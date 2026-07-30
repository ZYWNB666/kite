package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
)

func ListAuditLogs(c *gin.Context) {
	page := 1
	size := 20

	if p := strings.TrimSpace(c.Query("page")); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page parameter"})
			return
		}
	}
	if s := strings.TrimSpace(c.Query("size")); s != "" {
		if parsed, err := strconv.Atoi(s); err == nil && parsed > 0 && parsed <= 100 {
			size = parsed
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid size parameter"})
			return
		}
	}

	operatorName := strings.TrimSpace(c.Query("operatorName"))
	search := strings.TrimSpace(c.Query("search"))
	operation := strings.TrimSpace(c.Query("operation"))
	clusterName := strings.TrimSpace(c.Query("cluster"))
	resourceType := strings.TrimSpace(c.Query("resourceType"))
	resourceName := strings.TrimSpace(c.Query("resourceName"))
	namespace := strings.TrimSpace(c.Query("namespace"))

	query := model.DB.Model(&model.ResourceHistory{})
	if operatorName != "" {
		query = query.
			Joins("JOIN users ON users.id = resource_histories.operator_id").
			Where("users.name = ?", operatorName)
	}
	if clusterName != "" {
		query = query.Where("cluster_name = ?", clusterName)
	}
	if resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}
	if resourceName != "" {
		query = query.Where("resource_name = ?", resourceName)
	}
	if namespace != "" {
		query = query.Where("namespace = ?", namespace)
	}
	if search != "" {
		like := "%" + search + "%"
		query = query.Where("resource_name LIKE ?", like)
	}
	if operation != "" {
		query = query.Where("operation_type = ?", operation)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	history := []model.ResourceHistory{}
	if err := query.
		Select(`resource_histories.id,
			resource_histories.created_at,
			resource_histories.updated_at,
			resource_histories.cluster_name,
			resource_histories.resource_type,
			resource_histories.resource_name,
			resource_histories.namespace,
			resource_histories.operation_type,
			resource_histories.operation_source,
			resource_histories.success,
			resource_histories.error_message,
			resource_histories.operator_id,
			CASE
				WHEN resource_histories.resource_yaml <> '' OR resource_histories.previous_yaml <> ''
				THEN true
				ELSE false
			END AS has_yaml_diff`).
		Preload("Operator").
		Order("resource_histories.created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&history).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  history,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

func GetAuditLogDetail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid audit log id"})
		return
	}

	var history model.ResourceHistory
	result := model.DB.
		Select("id", "resource_yaml", "previous_yaml").
		First(&history, uint(id))
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "audit log not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load audit log"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           history.ID,
		"resourceYaml": history.ResourceYAML,
		"previousYaml": history.PreviousYAML,
	})
}
