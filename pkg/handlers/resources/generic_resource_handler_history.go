package resources

import (
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	"k8s.io/kubectl/pkg/describe"
)

func (h *GenericResourceHandler[T, V]) registerCustomRoutes(group *gin.RouterGroup) {
	if h.name == string(common.CRDs) {
		group.GET("/_all/_summaries", h.listCRDSummaries)
	}
}

func (h *GenericResourceHandler[T, V]) ListHistory(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	namespace := c.Param("namespace")
	resourceName := c.Param("name")
	user := c.MustGet("user").(model.User)
	if h.name == string(common.Secrets) && !hasResourceWriteAccess(user, h.name, cs.Name, namespace, resourceName) {
		c.JSON(http.StatusForbidden, gin.H{"error": rbac.NoAccess(user.Key(), string(common.VerbUpdate), h.name, namespace, cs.Name)})
		return
	}
	if h.name == string(common.ConfigMaps) {
		_, err := h.GetResource(c, namespace, resourceName)
		if err != nil && !hasResourceWriteAccess(user, h.name, cs.Name, namespace, resourceName) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pageSize parameter"})
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page parameter"})
		return
	}

	var total int64
	if err := model.DB.Model(&model.ResourceHistory{}).Where("cluster_name = ? AND resource_type = ? AND resource_name = ? AND namespace = ?", cs.Name, h.name, resourceName, namespace).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	history := []model.ResourceHistory{}
	if err := model.DB.Preload("Operator").Where("cluster_name = ? AND resource_type = ? AND resource_name = ? AND namespace = ?", cs.Name, h.name, resourceName, namespace).Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&history).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))
	hasNextPage := page < totalPages
	hasPrevPage := page > 1

	response := gin.H{
		"data": history,
		"pagination": gin.H{
			"page":        page,
			"pageSize":    pageSize,
			"total":       total,
			"totalPages":  totalPages,
			"hasNextPage": hasNextPage,
			"hasPrevPage": hasPrevPage,
		},
	}

	c.JSON(http.StatusOK, response)
}

func (h *GenericResourceHandler[T, V]) Describe(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	if h.name == string(common.ConfigMaps) {
		if _, err := h.GetResource(c, c.Param("namespace"), c.Param("name")); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
	}
	gk := h.getGroupKind()
	describer, ok := describe.DescriberFor(gk, cs.K8sClient.Configuration)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "no describer found for this resource"})
		return
	}
	namespace := c.Param("namespace")
	name := c.Param("name")
	out, err := describer.Describe(namespace, name, describe.DescriberSettings{
		ShowEvents: true,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": out})
}
