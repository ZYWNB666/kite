package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
)

// ProxyKubeconfigHandler returns the kubeconfig for clusters that the
// authenticated API-key user is allowed to proxy through kite-proxy.
//
// This endpoint is intentionally available only to API-key users so that
// kite-proxy can fetch credentials without a browser session.  The response
// must be treated as sensitive; kite-proxy MUST keep it in memory only.
//
// Query params:
//
//	cluster (optional) – restrict the response to a single named cluster.
func ProxyKubeconfigHandler(cm *cluster.ClusterManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := c.MustGet("user").(model.User)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		// Only API-key users may call this endpoint.
		if user.Provider != "api_key" {
			c.JSON(http.StatusForbidden, gin.H{"error": "this endpoint is only available to API-key users"})
			return
		}

		clusterFilter := c.Query("cluster")

		// Build result: cluster name -> kubeconfig YAML string.
		type clusterKubeconfig struct {
			Name       string `json:"name"`
			Kubeconfig string `json:"kubeconfig"`
		}

		var results []clusterKubeconfig

		clusters, _, _ := cm.GetAllClusters()
		for name, cs := range clusters {
			if clusterFilter != "" && name != clusterFilter {
				continue
			}
			// Check that user has proxy permission for this cluster.
			// We use AllNamespaces ("*") for the namespace check – cluster-level
			// proxy access is sufficient here; namespace filtering is kite-proxy's job.
			if !rbac.CanProxy(user, name, "*") && !rbac.CanProxy(user, name, "_all") {
				continue
			}
			kc := cs.GetKubeconfig()
			if kc == "" {
				// In-cluster config – no kubeconfig to return.
				continue
			}
			results = append(results, clusterKubeconfig{
				Name:       name,
				Kubeconfig: kc,
			})
		}

		if len(results) == 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "no clusters available for proxy or proxy not permitted"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"clusters": results})
	}
}
