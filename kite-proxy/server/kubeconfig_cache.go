package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// kubeconfigEntry holds an in-memory kubeconfig for a single cluster.
// The raw kubeconfig YAML is intentionally NOT stored here; only the
// parsed rest.Config is kept so that secrets are not retained as plain text.
type kubeconfigEntry struct {
	restConfig *rest.Config
}

// kubeconfigCache is an in-memory store of cluster → *rest.Config.
// kubeconfigs are NEVER written to disk.
type kubeconfigCache struct {
	mu      sync.RWMutex
	entries map[string]*kubeconfigEntry
}

// globalCache is the singleton in-memory kubeconfig store.
var globalCache = &kubeconfigCache{
	entries: make(map[string]*kubeconfigEntry),
}

// Get returns the rest.Config for clusterName, fetching it from the kite
// server if it is not already cached.
func (c *kubeconfigCache) Get(clusterName string) (*rest.Config, error) {
	c.mu.RLock()
	if entry, ok := c.entries[clusterName]; ok {
		c.mu.RUnlock()
		return entry.restConfig, nil
	}
	c.mu.RUnlock()

	// Not cached – fetch from kite server.
	restCfg, err := fetchRestConfig(clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch kubeconfig for cluster %q: %w", clusterName, err)
	}

	c.mu.Lock()
	c.entries[clusterName] = &kubeconfigEntry{restConfig: restCfg}
	c.mu.Unlock()

	klog.Infof("Loaded kubeconfig for cluster %q into memory cache", clusterName)
	return restCfg, nil
}

// Clear removes all cached kubeconfigs (e.g. after config change).
func (c *kubeconfigCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*kubeconfigEntry)
	klog.Info("Kubeconfig cache cleared")
}

// ClearCluster removes the cached kubeconfig for a single cluster.
func (c *kubeconfigCache) ClearCluster(clusterName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, clusterName)
}

// ListCached returns the names of all clusters that have a cached kubeconfig.
func (c *kubeconfigCache) ListCached() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	names := make([]string, 0, len(c.entries))
	for name := range c.entries {
		names = append(names, name)
	}
	return names
}

// fetchRestConfig calls the kite server's proxy kubeconfig endpoint and returns
// a parsed rest.Config for the requested cluster.
// The raw YAML is used only transiently inside this function and is not stored.
func fetchRestConfig(clusterName string) (*rest.Config, error) {
	cfg := GetConfig()
	if cfg.KiteURL == "" {
		return nil, fmt.Errorf("kite server URL is not configured")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("kite API key is not configured")
	}

	url := fmt.Sprintf("%s/api/v1/proxy/kubeconfig?cluster=%s", cfg.KiteURL, clusterName)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", cfg.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to kite server failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kite server returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Clusters []struct {
			Name       string `json:"name"`
			Kubeconfig string `json:"kubeconfig"`
		} `json:"clusters"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode kite response: %w", err)
	}

	for _, cl := range result.Clusters {
		if cl.Name == clusterName {
			restCfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(cl.Kubeconfig))
			if err != nil {
				return nil, fmt.Errorf("failed to parse kubeconfig for cluster %q: %w", clusterName, err)
			}
			// Raw YAML (cl.Kubeconfig) is discarded here – only the parsed config is kept.
			return restCfg, nil
		}
	}

	return nil, fmt.Errorf("cluster %q not found in kite response", clusterName)
}

// FetchAvailableClusters calls the kite server and returns the list of
// cluster names that are available for proxying (based on RBAC permissions).
func FetchAvailableClusters() ([]ClusterInfo, error) {
	cfg := GetConfig()
	if cfg.KiteURL == "" {
		return nil, fmt.Errorf("kite server URL is not configured")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("kite API key is not configured")
	}

	url := fmt.Sprintf("%s/api/v1/proxy/kubeconfig", cfg.KiteURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", cfg.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to kite server failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("no proxy permission or no accessible clusters")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kite server returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Clusters []struct {
			Name string `json:"name"`
		} `json:"clusters"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode kite response: %w", err)
	}

	clusters := make([]ClusterInfo, 0, len(result.Clusters))
	for _, cl := range result.Clusters {
		cached := false
		globalCache.mu.RLock()
		_, cached = globalCache.entries[cl.Name]
		globalCache.mu.RUnlock()
		clusters = append(clusters, ClusterInfo{
			Name:   cl.Name,
			Cached: cached,
		})
	}
	return clusters, nil
}

// ClusterInfo is a lightweight struct used in API responses.
type ClusterInfo struct {
	Name   string `json:"name"`
	Cached bool   `json:"cached"`
}
