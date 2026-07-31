package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAuditPaginationTestDB(t *testing.T) *bytes.Buffer {
	t.Helper()
	originalDB := model.DB
	var sqlLog bytes.Buffer
	testDB, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:audit-pagination-%d?mode=memory&cache=shared", time.Now().UnixNano())),
		&gorm.Config{
			Logger: logger.New(
				log.New(&sqlLog, "", 0),
				logger.Config{LogLevel: logger.Info},
			),
		},
	)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := testDB.AutoMigrate(&model.User{}, &model.ResourceHistory{}, &model.AccessRequest{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	model.DB = testDB
	t.Cleanup(func() {
		model.DB = originalDB
		sqlDB, dbErr := testDB.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return &sqlLog
}

func TestListAuditLogsPaginatesFiltersByNameAndOmitsYAML(t *testing.T) {
	sqlLog := setupAuditPaginationTestDB(t)
	gin.SetMode(gin.TestMode)

	alice := model.User{Username: "alice-login", Name: "Alice", Enabled: true}
	bob := model.User{Username: "bob-login", Name: "Bob", Enabled: true}
	if err := model.DB.Create(&alice).Error; err != nil {
		t.Fatalf("create Alice: %v", err)
	}
	if err := model.DB.Create(&bob).Error; err != nil {
		t.Fatalf("create Bob: %v", err)
	}

	now := time.Now().UTC()
	histories := []model.ResourceHistory{
		{
			CreatedAt:       now.Add(-3 * time.Minute),
			ClusterName:     "cluster-a",
			ResourceType:    "Deployment",
			ResourceName:    "alice-old",
			Namespace:       "default",
			OperationType:   "update",
			OperationSource: "manual",
			ResourceYAML:    "alice-old-current-SECRET-YAML",
			PreviousYAML:    "alice-old-previous-SECRET-YAML",
			Success:         true,
			OperatorID:      alice.ID,
		},
		{
			CreatedAt:       now.Add(-2 * time.Minute),
			ClusterName:     "cluster-a",
			ResourceType:    "Deployment",
			ResourceName:    "bob-entry",
			Namespace:       "default",
			OperationType:   "update",
			OperationSource: "manual",
			ResourceYAML:    "bob-SECRET-YAML",
			PreviousYAML:    "bob-previous-SECRET-YAML",
			Success:         true,
			OperatorID:      bob.ID,
		},
		{
			CreatedAt:       now.Add(-time.Minute),
			ClusterName:     "cluster-a",
			ResourceType:    "Deployment",
			ResourceName:    "alice-new",
			Namespace:       "default",
			OperationType:   "update",
			OperationSource: "manual",
			ResourceYAML:    "alice-new-current-SECRET-YAML",
			PreviousYAML:    "alice-new-previous-SECRET-YAML",
			Success:         true,
			OperatorID:      alice.ID,
		},
	}
	if err := model.DB.Create(&histories).Error; err != nil {
		t.Fatalf("create histories: %v", err)
	}

	router := gin.New()
	router.GET("/audit-logs", ListAuditLogs)
	router.GET("/audit-logs/:id", GetAuditLogDetail)

	listRecorder := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/audit-logs?page=1&size=1&operatorName=Alice", nil)
	sqlLog.Reset()
	router.ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list audit logs status=%d body=%s", listRecorder.Code, listRecorder.Body.String())
	}
	if strings.Contains(listRecorder.Body.String(), "SECRET-YAML") {
		t.Fatalf("audit list leaked YAML: %s", listRecorder.Body.String())
	}
	if strings.Contains(listRecorder.Body.String(), "resourceYaml") ||
		strings.Contains(listRecorder.Body.String(), "previousYaml") {
		t.Fatalf("audit list should omit YAML fields entirely: %s", listRecorder.Body.String())
	}
	if strings.Contains(sqlLog.String(), "resource_yaml") ||
		strings.Contains(sqlLog.String(), "previous_yaml") {
		t.Fatalf("audit list SQL must not access YAML columns: %s", sqlLog.String())
	}

	var listResponse struct {
		Data  []model.ResourceHistory `json:"data"`
		Total int64                   `json:"total"`
		Page  int                     `json:"page"`
		Size  int                     `json:"size"`
	}
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listResponse); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listResponse.Total != 2 || listResponse.Page != 1 || listResponse.Size != 1 || len(listResponse.Data) != 1 {
		t.Fatalf("unexpected pagination response: %+v", listResponse)
	}
	entry := listResponse.Data[0]
	if entry.ResourceName != "alice-new" {
		t.Fatalf("expected newest Alice entry, got %q", entry.ResourceName)
	}
	if entry.Operator == nil || entry.Operator.Name != "Alice" {
		t.Fatalf("expected operator name Alice, got %+v", entry.Operator)
	}
	if entry.ResourceYAML != "" || entry.PreviousYAML != "" {
		t.Fatalf("list must omit YAML: %+v", entry)
	}

	secondPageRecorder := httptest.NewRecorder()
	secondPageRequest := httptest.NewRequest(http.MethodGet, "/audit-logs?page=2&size=1&operatorName=Alice", nil)
	router.ServeHTTP(secondPageRecorder, secondPageRequest)
	if secondPageRecorder.Code != http.StatusOK {
		t.Fatalf("second page status=%d body=%s", secondPageRecorder.Code, secondPageRecorder.Body.String())
	}
	var secondPageResponse struct {
		Data []model.ResourceHistory `json:"data"`
	}
	if err := json.Unmarshal(secondPageRecorder.Body.Bytes(), &secondPageResponse); err != nil {
		t.Fatalf("decode second page response: %v", err)
	}
	if len(secondPageResponse.Data) != 1 || secondPageResponse.Data[0].ResourceName != "alice-old" {
		t.Fatalf("unexpected second page: %+v", secondPageResponse.Data)
	}

	detailRecorder := httptest.NewRecorder()
	detailRequest := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/audit-logs/%d", entry.ID), nil)
	router.ServeHTTP(detailRecorder, detailRequest)
	if detailRecorder.Code != http.StatusOK {
		t.Fatalf("audit detail status=%d body=%s", detailRecorder.Code, detailRecorder.Body.String())
	}
	var detailResponse struct {
		ID           uint   `json:"id"`
		ResourceYAML string `json:"resourceYaml"`
		PreviousYAML string `json:"previousYaml"`
	}
	if err := json.Unmarshal(detailRecorder.Body.Bytes(), &detailResponse); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detailResponse.ID != entry.ID || detailResponse.ResourceYAML != "alice-new-current-SECRET-YAML" || detailResponse.PreviousYAML != "alice-new-previous-SECRET-YAML" {
		t.Fatalf("unexpected detail response: %+v", detailResponse)
	}
}

func TestListAllAccessRequestsPaginates(t *testing.T) {
	_ = setupAuditPaginationTestDB(t)
	gin.SetMode(gin.TestMode)

	now := time.Now().UTC()
	for index := 1; index <= 5; index++ {
		req := model.AccessRequest{
			Model:         model.Model{CreatedAt: now.Add(time.Duration(index) * time.Minute)},
			RequesterID:   uint(index),
			RequesterName: fmt.Sprintf("user-%d", index),
			Cluster:       "cluster-a",
			Namespace:     "default",
			DurationHours: 1,
			RiskLevel:     "low",
			Status:        model.AccessRequestPending,
		}
		if err := model.DB.Create(&req).Error; err != nil {
			t.Fatalf("create access request %d: %v", index, err)
		}
	}

	router := gin.New()
	router.GET("/access-requests", ListAllAccessRequests)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/access-requests?page=2&size=2", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list access requests status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Requests []model.AccessRequest `json:"requests"`
		Total    int64                 `json:"total"`
		Page     int                   `json:"page"`
		Size     int                   `json:"size"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode access request response: %v", err)
	}
	if response.Total != 5 || response.Page != 2 || response.Size != 2 || len(response.Requests) != 2 {
		t.Fatalf("unexpected pagination response: %+v", response)
	}
	if response.Requests[0].RequesterName != "user-3" || response.Requests[1].RequesterName != "user-2" {
		t.Fatalf("unexpected page contents: %+v", response.Requests)
	}
}

func TestResourceHistoryHasCreatedAtIndex(t *testing.T) {
	_ = setupAuditPaginationTestDB(t)
	if !model.DB.Migrator().HasIndex(&model.ResourceHistory{}, "idx_resource_histories_created_at") {
		t.Fatal("resource history needs a standalone created_at index for audit pagination")
	}

	var plan []struct {
		Detail string `gorm:"column:detail"`
	}
	if err := model.DB.Raw(`
		EXPLAIN QUERY PLAN
		SELECT id, created_at
		FROM resource_histories
		ORDER BY created_at DESC
		LIMIT 20
	`).Scan(&plan).Error; err != nil {
		t.Fatalf("explain audit pagination query: %v", err)
	}
	planText := ""
	for _, row := range plan {
		planText += row.Detail + "\n"
	}
	if !strings.Contains(planText, "idx_resource_histories_created_at") {
		t.Fatalf("audit pagination query did not use created_at index:\n%s", planText)
	}
}
