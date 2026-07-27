package resources

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (h *GenericResourceHandler[T, V]) WatchSupported() bool {
	switch h.name {
	case string(common.PodMetrics), string(common.NodeMetrics):
		return false
	default:
		return true
	}
}

func (h *GenericResourceHandler[T, V]) Watch(c *gin.Context) {
	if !h.WatchSupported() {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "watch is not supported for this resource"})
		return
	}

	resource := common.MustLookupResource(h.name)
	serveResourceWatch(c, resourceWatchStreamOptions{
		GVR: schema.GroupVersionResource{
			Group:    resource.Group,
			Version:  resource.Version,
			Resource: string(resource.Plural),
		},
		Resource:      h.name,
		ClusterScoped: resource.ClusterScoped,
		Reduce:        c.Query("reduce") == "true",
	})
}
