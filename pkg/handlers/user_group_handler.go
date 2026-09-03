package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	"gorm.io/gorm"
)

type userGroupRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	MemberIDs   []uint `json:"memberIds"`
}

func ListUserGroups(c *gin.Context) {
	groups, err := model.ListUserGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list user groups"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"groups": groups})
}

func GetUserGroup(c *gin.Context) {
	id, ok := parseUserGroupID(c)
	if !ok {
		return
	}
	group, err := model.GetUserGroupByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user group"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"group": group})
}

func CreateUserGroup(c *gin.Context) {
	var req userGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	group := &model.UserGroup{
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
	}
	if err := model.CreateUserGroup(group, req.MemberIDs); err != nil {
		writeUserGroupError(c, err, "create")
		return
	}
	rbac.TriggerSync()
	c.JSON(http.StatusCreated, gin.H{"group": group})
}

func UpdateUserGroup(c *gin.Context) {
	id, ok := parseUserGroupID(c)
	if !ok {
		return
	}
	var req userGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	group, err := model.GetUserGroupByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user group"})
		return
	}
	if err := model.UpdateUserGroup(group, req.Name, strings.TrimSpace(req.Description), req.MemberIDs); err != nil {
		writeUserGroupError(c, err, "update")
		return
	}
	rbac.TriggerSync()
	c.JSON(http.StatusOK, gin.H{"group": group})
}

func DeleteUserGroup(c *gin.Context) {
	id, ok := parseUserGroupID(c)
	if !ok {
		return
	}
	group, err := model.GetUserGroupByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user group not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user group"})
		return
	}
	if err := model.DeleteUserGroup(group); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user group"})
		return
	}
	rbac.TriggerSync()
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func parseUserGroupID(c *gin.Context) (uint, bool) {
	var id uint
	if _, err := fmt.Sscanf(c.Param("id"), "%d", &id); err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user group id"})
		return 0, false
	}
	return id, true
}

func writeUserGroupError(c *gin.Context, err error, operation string) {
	if errors.Is(err, model.ErrInvalidGroupMember) || strings.Contains(err.Error(), "group name is required") {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "unique") || strings.Contains(lower, "duplicate") {
		c.JSON(http.StatusConflict, gin.H{"error": "a user group with this name already exists"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to " + operation + " user group"})
}
