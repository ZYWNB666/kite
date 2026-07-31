package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
)

type auditLogOperator struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name,omitempty"`
	Provider string `json:"provider,omitempty"`
}

type auditLogListItem struct {
	ID              uint              `json:"id"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
	ClusterName     string            `json:"clusterName"`
	ResourceType    string            `json:"resourceType"`
	ResourceName    string            `json:"resourceName"`
	Namespace       string            `json:"namespace"`
	OperationType   string            `json:"operationType"`
	OperationSource string            `json:"operationSource"`
	Success         bool              `json:"success"`
	ErrorMessage    string            `json:"errorMessage"`
	OperatorID      uint              `json:"operatorId"`
	Operator        *auditLogOperator `json:"operator,omitempty"`
}

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
	// Do not reference the YAML columns here, even for a presence check. Without
	// that guarantee the database may read every large YAML value before applying
	// ORDER BY/LIMIT. GetAuditLogDetail is the only audit endpoint that loads YAML.
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
			resource_histories.operator_id`).
		Preload("Operator", func(operatorQuery *gorm.DB) *gorm.DB {
			return operatorQuery.Select("id", "username", "name", "provider")
		}).
		Order("resource_histories.created_at DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&history).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	items := make([]auditLogListItem, 0, len(history))
	for i := range history {
		entry := history[i]
		item := auditLogListItem{
			ID:              entry.ID,
			CreatedAt:       entry.CreatedAt,
			UpdatedAt:       entry.UpdatedAt,
			ClusterName:     entry.ClusterName,
			ResourceType:    entry.ResourceType,
			ResourceName:    entry.ResourceName,
			Namespace:       entry.Namespace,
			OperationType:   entry.OperationType,
			OperationSource: entry.OperationSource,
			Success:         entry.Success,
			ErrorMessage:    entry.ErrorMessage,
			OperatorID:      entry.OperatorID,
		}
		if entry.Operator != nil {
			item.Operator = &auditLogOperator{
				ID:       entry.Operator.ID,
				Username: entry.Operator.Username,
				Name:     entry.Operator.Name,
				Provider: entry.Operator.Provider,
			}
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  items,
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
