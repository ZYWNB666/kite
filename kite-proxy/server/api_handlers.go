package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// handleGetConfig returns the current config (API key is masked).
func handleGetConfig(c *gin.Context) {
	cfg := GetConfig()
	maskedKey := ""
	if cfg.APIKey != "" {
		if len(cfg.APIKey) > 8 {
			maskedKey = cfg.APIKey[:4] + "****" + cfg.APIKey[len(cfg.APIKey)-4:]
		} else {
			maskedKey = "****"
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"kiteURL":      cfg.KiteURL,
		"apiKeyMasked": maskedKey,
		"configured":   cfg.KiteURL != "" && cfg.APIKey != "",
	})
}

// handleSetConfig updates the kite server URL and API key.
func handleSetConfig(c *gin.Context) {
	var req struct {
		KiteURL string `json:"kiteURL" binding:"required"`
		APIKey  string `json:"apiKey"  binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	SetConfig(Config{
		Port:    GetConfig().Port,
		KiteURL: req.KiteURL,
		APIKey:  req.APIKey,
	})

	klog.Infof("Configuration updated: kiteURL=%s", req.KiteURL)
	c.JSON(http.StatusOK, gin.H{"message": "configuration saved"})
}

// handleListClusters fetches available clusters from the kite server.
func handleListClusters(c *gin.Context) {
	clusters, err := FetchAvailableClusters()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"clusters": clusters})
}

// handleGetKubeconfig returns a local kubeconfig that points kubectl at
// this kite-proxy instance.  Users can pipe the output into a file and
// use it with KUBECONFIG env variable.
func handleGetKubeconfig(c *gin.Context) {
	clusters, err := FetchAvailableClusters()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	cfg := GetConfig()
	port := cfg.Port
	if port == "" {
		port = "8090"
	}
	baseURL := fmt.Sprintf("http://localhost:%s", port)

	// Build a multi-cluster kubeconfig YAML.
	var sb stringBuilder
	sb.line("apiVersion: v1")
	sb.line("kind: Config")
	sb.line("preferences: {}")
	sb.line("")
	sb.line("clusters:")
	for _, cl := range clusters {
		sb.linef("- cluster:")
		sb.linef("    server: %s/proxy/%s", baseURL, cl.Name)
		sb.linef("    insecure-skip-tls-verify: true")
		sb.linef("  name: kite-proxy-%s", cl.Name)
	}
	sb.line("")
	sb.line("users:")
	sb.line("- name: kite-proxy-user")
	sb.line("  user: {}")
	sb.line("")
	sb.line("contexts:")
	for _, cl := range clusters {
		sb.linef("- context:")
		sb.linef("    cluster: kite-proxy-%s", cl.Name)
		sb.linef("    user: kite-proxy-user")
		sb.linef("  name: kite-proxy-%s", cl.Name)
	}
	if len(clusters) > 0 {
		sb.line("")
		sb.linef("current-context: kite-proxy-%s", clusters[0].Name)
	}

	c.Header("Content-Type", "application/x-yaml")
	c.Header("Content-Disposition", `attachment; filename="kubeconfig-kite-proxy.yaml"`)
	c.String(http.StatusOK, sb.String())
}

// handleClearCache removes all cached kubeconfigs from memory.
func handleClearCache(c *gin.Context) {
	globalCache.Clear()
	c.JSON(http.StatusOK, gin.H{"message": "cache cleared"})
}

// handleStatus returns health and status information.
func handleStatus(c *gin.Context) {
	cfg := GetConfig()
	c.JSON(http.StatusOK, gin.H{
		"status":        "ok",
		"configured":    cfg.KiteURL != "" && cfg.APIKey != "",
		"cachedClusters": globalCache.ListCached(),
	})
}

// handlePrewarm fetches (and caches) the kubeconfig for a specific cluster.
func handlePrewarm(c *gin.Context) {
	clusterName := c.Param("cluster")
	if clusterName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster name is required"})
		return
	}
	globalCache.ClearCluster(clusterName)
	if _, err := globalCache.Get(clusterName); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("cluster %q warmed up", clusterName)})
}

// stringBuilder is a tiny helper to build multi-line strings.
type stringBuilder struct {
	buf []byte
}

func (s *stringBuilder) line(text string) {
	s.buf = append(s.buf, text...)
	s.buf = append(s.buf, '\n')
}

func (s *stringBuilder) linef(format string, args ...interface{}) {
	s.line(fmt.Sprintf(format, args...))
}

func (s *stringBuilder) String() string {
	return string(s.buf)
}
