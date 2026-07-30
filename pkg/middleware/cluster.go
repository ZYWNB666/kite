package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
)

const (
	ClusterNameHeader = "x-cluster-name"
	ClusterNameKey    = "cluster-name"
	K8sClientKey      = "k8s-client"
	PromClientKey     = "prom-client"
)

// ClusterMiddleware extracts cluster name from header and injects clients into context
func ClusterMiddleware(cm *cluster.ClusterManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		headerCluster := c.GetHeader(ClusterNameHeader)
		queryCluster, hasQueryCluster := c.GetQuery(ClusterNameHeader)
		if headerCluster != "" && hasQueryCluster && queryCluster != "" && headerCluster != queryCluster {
			c.JSON(http.StatusBadRequest, gin.H{"error": "conflicting cluster names"})
			c.Abort()
			return
		}

		clusterName := headerCluster
		if clusterName == "" && hasQueryCluster {
			clusterName = queryCluster
		}
		if clusterName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cluster name is required"})
			c.Abort()
			return
		}
		cluster, err := cm.GetClientSet(clusterName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			c.Abort()
			return
		}
		c.Set("cluster", cluster)
		c.Set(ClusterNameKey, cluster.Name)
		c.Next()
	}
}
