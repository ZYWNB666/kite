package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/model"
	"gorm.io/gorm"
)

func TestNormalizeReportLink(t *testing.T) {
	got, err := normalizeReportLink("  https://docs.example.com/report?id=1  ")
	if err != nil {
		t.Fatalf("normalizeReportLink() error = %v", err)
	}
	if got != "https://docs.example.com/report?id=1" {
		t.Fatalf("normalizeReportLink() = %q", got)
	}

	for _, invalid := range []string{"", "   ", "not-a-url", "ftp://example.com/report"} {
		t.Run(invalid, func(t *testing.T) {
			if _, err := normalizeReportLink(invalid); err == nil {
				t.Fatalf("normalizeReportLink(%q) unexpectedly succeeded", invalid)
			}
		})
	}
}

func TestNormalizeTargetResources(t *testing.T) {
	got, err := normalizeTargetResources([]string{"gateway-config", " gateway-config ", "routes.prod"})
	if err != nil {
		t.Fatalf("normalizeTargetResources() error = %v", err)
	}
	if len(got) != 2 || got[0] != "gateway-config" || got[1] != "routes.prod" {
		t.Fatalf("normalizeTargetResources() = %#v", got)
	}

	for _, invalid := range []string{"*", ".*", "gateway|other", ""} {
		t.Run(invalid, func(t *testing.T) {
			if _, err := normalizeTargetResources([]string{invalid}); err == nil {
				t.Fatalf("normalizeTargetResources(%q) unexpectedly succeeded", invalid)
			}
		})
	}
}

func TestNormalizeUpdateNamespacesRejectsRouteAdjustmentNamespace(t *testing.T) {
	for _, namespaces := range [][]string{
		{routeAdjustmentNamespace},
		{"default", routeAdjustmentNamespace},
		{" " + routeAdjustmentNamespace + " "},
	} {
		if _, err := normalizeUpdateNamespaces(namespaces); err == nil {
			t.Fatalf("normalizeUpdateNamespaces(%#v) unexpectedly succeeded", namespaces)
		}
	}

	got, err := normalizeUpdateNamespaces([]string{" default ", "default", "production"})
	if err != nil {
		t.Fatalf("normalizeUpdateNamespaces() error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"default", "production"}) {
		t.Fatalf("normalizeUpdateNamespaces() = %#v", got)
	}
}

func TestCreateAccessRequestRejectsReservedNamespaceForUpdates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, requestType := range []string{model.RequestTypeFullUpdate, model.RequestTypeCanaryUpdate} {
		t.Run(requestType, func(t *testing.T) {
			body := fmt.Sprintf(`{
				"cluster":"cluster-a",
				"namespaces":["default","%s"],
				"requestType":"%s",
				"reportLink":"https://example.com/report",
				"durationHours":1,
				"riskLevel":"low",
				"reason":"test",
				"approverUid":"approver"
			}`, routeAdjustmentNamespace, requestType)

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/access-requests", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("user", model.User{Model: model.Model{ID: 1}})
			c.Set("cluster", &cluster.ClientSet{Name: "cluster-a"})

			CreateAccessRequest(c)

			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "仅允许通过路由调整申请访问") {
				t.Fatalf("status/body = %d %q", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestBuildTempRoleScopesRouteAdjustmentToSelectedConfigMaps(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour)
	req := &model.AccessRequest{
		Model:           model.Model{ID: 186},
		RequesterName:   "admin",
		Cluster:         "yunqiao",
		Namespace:       "envoy-gateway-system",
		RequestType:     model.RequestTypeRouteAdjust,
		TargetResources: model.SliceString{"magik-proxy-xingtai1-k3-test2", "magik-proxy-xingtai1-k3-test1"},
		ExpiresAt:       &expiresAt,
	}

	role, err := buildTempRole(req)
	if err != nil {
		t.Fatalf("buildTempRole() error = %v", err)
	}
	if !reflect.DeepEqual(role.Resources, model.SliceString{"configmaps"}) {
		t.Fatalf("role.Resources = %#v, want configmaps", role.Resources)
	}
	if !reflect.DeepEqual(role.ResourceNames, req.TargetResources) {
		t.Fatalf("role.ResourceNames = %#v, want %#v", role.ResourceNames, req.TargetResources)
	}
	if !reflect.DeepEqual(role.Verbs, model.SliceString{"get", "list", "create", "update", "patch"}) {
		t.Fatalf("role.Verbs = %#v", role.Verbs)
	}
}

func TestBuildTempRoleRejectsUnknownRequestType(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour)
	_, err := buildTempRole(&model.AccessRequest{
		Model:         model.Model{ID: 187},
		Cluster:       "yunqiao",
		Namespace:     "envoy-gateway-system",
		RequestType:   "",
		DurationHours: 1,
		ExpiresAt:     &expiresAt,
	})
	if err == nil {
		t.Fatal("buildTempRole() unexpectedly granted permissions to an unknown request type")
	}
}

func TestBuildTempRoleRejectsReservedNamespaceForUpdates(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour)
	for _, requestType := range []string{model.RequestTypeFullUpdate, model.RequestTypeCanaryUpdate} {
		t.Run(requestType, func(t *testing.T) {
			_, err := buildTempRole(&model.AccessRequest{
				Model:       model.Model{ID: 188},
				Cluster:     "yunqiao",
				Namespace:   routeAdjustmentNamespace,
				RequestType: requestType,
				ExpiresAt:   &expiresAt,
			})
			if err == nil {
				t.Fatal("buildTempRole() unexpectedly granted update access to the reserved namespace")
			}
		})
	}
}

func TestReconcileActiveRouteAdjustmentRolesRepairsWildcardRole(t *testing.T) {
	originalDB := model.DB
	testDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:route-role-%d?mode=memory&cache=shared", time.Now().UnixNano())))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := testDB.AutoMigrate(&model.Role{}, &model.AccessRequest{}); err != nil {
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

	role := model.Role{
		Name:       "temp-access-req-186",
		Clusters:   model.SliceString{"yunqiao"},
		Namespaces: model.SliceString{"envoy-gateway-system"},
		Resources:  model.SliceString{"*"},
		Verbs:      model.SliceString{"*"},
	}
	if err := testDB.Create(&role).Error; err != nil {
		t.Fatalf("create wildcard role: %v", err)
	}
	expiresAt := time.Now().Add(time.Hour)
	req := model.AccessRequest{
		Model:           model.Model{ID: 186},
		RequesterName:   "admin",
		Cluster:         "yunqiao",
		Namespace:       "envoy-gateway-system",
		RequestType:     model.RequestTypeRouteAdjust,
		TargetResources: model.SliceString{"route-a", "route-b"},
		Status:          model.AccessRequestApproved,
		ExpiresAt:       &expiresAt,
		RoleID:          &role.ID,
	}
	if err := testDB.Create(&req).Error; err != nil {
		t.Fatalf("create access request: %v", err)
	}

	reconcileActiveRouteAdjustmentRoles()

	var repaired model.Role
	if err := testDB.First(&repaired, role.ID).Error; err != nil {
		t.Fatalf("load repaired role: %v", err)
	}
	if !reflect.DeepEqual(repaired.Resources, model.SliceString{"configmaps"}) {
		t.Fatalf("repaired.Resources = %#v", repaired.Resources)
	}
	if !reflect.DeepEqual(repaired.ResourceNames, req.TargetResources) {
		t.Fatalf("repaired.ResourceNames = %#v, want %#v", repaired.ResourceNames, req.TargetResources)
	}
	if !reflect.DeepEqual(repaired.Verbs, model.SliceString{"get", "list", "create", "update", "patch"}) {
		t.Fatalf("repaired.Verbs = %#v", repaired.Verbs)
	}
}

func TestCreateAccessRequestRequiresClusterContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"cluster":"cluster-a",
		"namespaces":["envoy-gateway-system"],
		"requestType":"route_adjust",
		"targetResources":["gateway-config"],
		"durationHours":1,
		"riskLevel":"low",
		"reason":"test",
		"approverUid":"approver"
	}`)

	t.Run("missing context", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/access-requests", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", model.User{Model: model.Model{ID: 1}})

		CreateAccessRequest(c)

		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "申请集群上下文缺失") {
			t.Fatalf("status/body = %d %q", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("mismatched context", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/access-requests", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", model.User{Model: model.Model{ID: 1}})
		c.Set("cluster", &cluster.ClientSet{Name: "cluster-b"})

		CreateAccessRequest(c)

		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "申请集群与请求集群不一致") {
			t.Fatalf("status/body = %d %q", recorder.Code, recorder.Body.String())
		}
	})
}

func TestFeishuCallbackResponsePreservesRequestFields(t *testing.T) {
	req := &model.AccessRequest{
		RequesterName:   "requester",
		Cluster:         "cluster-a",
		Namespace:       "envoy-gateway-system",
		RequestType:     model.RequestTypeRouteAdjust,
		ReportLink:      "https://docs.example.com/report",
		TargetResources: model.SliceString{"gateway-config", "routes.prod"},
		DurationHours:   4,
		RiskLevel:       "medium",
		Reason:          "adjust route",
		ApproverName:    "approver",
		Status:          model.AccessRequestApproved,
	}
	card := buildAccessRequestResultCard(req)

	v2Body, err := json.Marshal(buildFeishuCardCallbackResponse("2.0", "success", "ok", card))
	if err != nil {
		t.Fatalf("marshal v2 response: %v", err)
	}
	for _, expected := range []string{
		`"type":"raw"`,
		"https://docs.example.com/report",
		"gateway-config",
		"routes.prod",
		"路由调整",
	} {
		if !strings.Contains(string(v2Body), expected) {
			t.Fatalf("v2 callback response missing %q: %s", expected, v2Body)
		}
	}

	legacy := buildFeishuCardCallbackResponse("", "success", "ok", card)
	if _, ok := legacy["elements"]; !ok {
		t.Fatal("legacy callback response must expose raw card elements at the root")
	}
	if _, ok := legacy["card"]; ok {
		t.Fatal("legacy callback response must not wrap raw card fields in card.data")
	}
}
