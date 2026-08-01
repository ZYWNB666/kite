package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUrl2NamespaceResource(t *testing.T) {
	testCases := []struct {
		name             string
		url              string
		matchedRoute     string
		wantNamespace    string
		wantResource     string
		wantResourceName string
	}{
		{
			name:             "valid URL with namespace and resource",
			url:              "/api/v1/pods/default/pods",
			matchedRoute:     "/api/v1/pods/:namespace/:name",
			wantNamespace:    "default",
			wantResource:     "pods",
			wantResourceName: "pods",
		},
		{
			name:             "valid URL with all namespace and specific resource",
			url:              "/api/v1/pvs/_all/some-pv",
			matchedRoute:     "/api/v1/pvs/_all/:name",
			wantNamespace:    "_all",
			wantResource:     "pvs",
			wantResourceName: "some-pv",
		},
		{
			name:             "valid URL with namespace only",
			url:              "/api/v1/pods/default",
			matchedRoute:     "/api/v1/pods/:namespace",
			wantNamespace:    "default",
			wantResource:     "pods",
			wantResourceName: "",
		},
		{
			name:             "invalid URL - too short",
			url:              "/api/v1",
			matchedRoute:     "",
			wantNamespace:    "",
			wantResource:     "",
			wantResourceName: "",
		},
		{
			name:             "URL without namespace",
			url:              "/api/v1/pods",
			matchedRoute:     "/api/v1/pods",
			wantNamespace:    "_all",
			wantResource:     "pods",
			wantResourceName: "",
		},
		{
			name:             "URL with resource name",
			url:              "/api/v1/pods/default/my-pod",
			matchedRoute:     "/api/v1/pods/:namespace/:name",
			wantNamespace:    "default",
			wantResource:     "pods",
			wantResourceName: "my-pod",
		},
		{
			name:             "sub-resource route still extracts resource name",
			url:              "/api/v1/pods/default/some-pod/history",
			matchedRoute:     "/api/v1/pods/:namespace/:name/history",
			wantNamespace:    "default",
			wantResource:     "pods",
			wantResourceName: "some-pod",
		},
		{
			name:             "watch collection route has no resource name",
			url:              "/api/v1/services/default/_watch",
			matchedRoute:     "/api/v1/services/:namespace/_watch",
			wantNamespace:    "default",
			wantResource:     "services",
			wantResourceName: "",
		},
		{
			name:             "watch remains a legal resource name",
			url:              "/api/v1/services/default/watch",
			matchedRoute:     "/api/v1/services/:namespace/:name",
			wantNamespace:    "default",
			wantResource:     "services",
			wantResourceName: "watch",
		},
		{
			name:             "history remains a legal resource name",
			url:              "/api/v1/configmaps/default/history",
			matchedRoute:     "/api/v1/configmaps/:namespace/:name",
			wantNamespace:    "default",
			wantResource:     "configmaps",
			wantResourceName: "history",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotNamespace, gotResource, gotResourceName := url2namespaceresource(
				tc.url,
				tc.matchedRoute,
			)
			if gotNamespace != tc.wantNamespace ||
				gotResource != tc.wantResource ||
				gotResourceName != tc.wantResourceName {
				t.Errorf(
					"url2namespaceresource(%q, %q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.url,
					tc.matchedRoute,
					gotNamespace,
					gotResource,
					gotResourceName,
					tc.wantNamespace,
					tc.wantResource,
					tc.wantResourceName,
				)
			}
		})
	}
}

func TestUrl2NamespaceResourceUsesMatchedGinRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		namespace, resource, resourceName := url2namespaceresource(
			c.Request.URL.Path,
			c.FullPath(),
		)
		c.Header("X-Test-Namespace", namespace)
		c.Header("X-Test-Resource", resource)
		c.Header("X-Test-Resource-Name", resourceName)
		c.Next()
	})
	router.GET("/api/v1/services/:namespace/_watch", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.PUT("/api/v1/services/:namespace/:name", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	tests := []struct {
		name             string
		method           string
		path             string
		wantResourceName string
	}{
		{
			name:             "collection watch",
			method:           http.MethodGet,
			path:             "/api/v1/services/default/_watch",
			wantResourceName: "",
		},
		{
			name:             "object named watch",
			method:           http.MethodPut,
			path:             "/api/v1/services/default/watch",
			wantResourceName: "watch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, tt.path, nil)
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
			}
			if got := recorder.Header().Get("X-Test-Namespace"); got != "default" {
				t.Fatalf("namespace = %q, want default", got)
			}
			if got := recorder.Header().Get("X-Test-Resource"); got != "services" {
				t.Fatalf("resource = %q, want services", got)
			}
			if got := recorder.Header().Get("X-Test-Resource-Name"); got != tt.wantResourceName {
				t.Fatalf("resource name = %q, want %q", got, tt.wantResourceName)
			}
		})
	}
}

func TestRBACMiddlewareAuthorizesRequestedNamespacesForNamespacedCRD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const crdName = "leaderworkersets.leaderworkerset.x-k8s.io"

	tests := []struct {
		name           string
		method         string
		scope          apiextensionsv1.ResourceScope
		roleNamespaces []string
		roleVerbs      []string
		query          string
		wantStatus     int
	}{
		{
			name:           "allows every explicitly authorized namespace",
			method:         http.MethodGet,
			scope:          apiextensionsv1.NamespaceScoped,
			roleNamespaces: []string{"team-a", "team-b"},
			roleVerbs:      []string{"get"},
			query:          "?namespaces=team-a,team-b",
			wantStatus:     http.StatusNoContent,
		},
		{
			name:           "rejects when one requested namespace is unauthorized",
			method:         http.MethodGet,
			scope:          apiextensionsv1.NamespaceScoped,
			roleNamespaces: []string{"team-a"},
			roleVerbs:      []string{"get"},
			query:          "?namespaces=team-a,team-b",
			wantStatus:     http.StatusForbidden,
		},
		{
			name:           "retains all-namespaces authorization without a selection",
			method:         http.MethodGet,
			scope:          apiextensionsv1.NamespaceScoped,
			roleNamespaces: []string{"team-a", "team-b"},
			roleVerbs:      []string{"get"},
			wantStatus:     http.StatusForbidden,
		},
		{
			name:           "does not bypass cluster-scoped authorization",
			method:         http.MethodGet,
			scope:          apiextensionsv1.ClusterScoped,
			roleNamespaces: []string{"team-a"},
			roleVerbs:      []string{"get"},
			query:          "?namespaces=team-a",
			wantStatus:     http.StatusForbidden,
		},
		{
			name:           "retains wildcard all-namespaces authorization",
			method:         http.MethodGet,
			scope:          apiextensionsv1.NamespaceScoped,
			roleNamespaces: []string{"*"},
			roleVerbs:      []string{"get"},
			wantStatus:     http.StatusNoContent,
		},
		{
			name:           "does not apply namespace selection to writes",
			method:         http.MethodPost,
			scope:          apiextensionsv1.NamespaceScoped,
			roleNamespaces: []string{"team-a"},
			roleVerbs:      []string{"create"},
			query:          "?namespaces=team-a",
			wantStatus:     http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := apiextensionsv1.AddToScheme(scheme); err != nil {
				t.Fatalf("AddToScheme() error = %v", err)
			}
			crd := &apiextensionsv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: crdName},
				Spec: apiextensionsv1.CustomResourceDefinitionSpec{
					Group: "leaderworkerset.x-k8s.io",
					Names: apiextensionsv1.CustomResourceDefinitionNames{
						Plural: "leaderworkersets",
						Kind:   "LeaderWorkerSet",
					},
					Scope: tt.scope,
					Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
						Name: "v1", Served: true, Storage: true,
					}},
				},
			}
			user := model.User{
				Username: "alice",
				Roles: []common.Role{{
					Name:       "lws-reader",
					Clusters:   []string{"test"},
					Resources:  []string{crdName},
					Namespaces: tt.roleNamespaces,
					Verbs:      tt.roleVerbs,
				}},
			}
			clientSet := &cluster.ClientSet{
				Name: "test",
				K8sClient: &kube.K8sClient{
					Client: ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(crd).Build(),
				},
			}

			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("user", user)
				c.Set("cluster", clientSet)
				c.Next()
			})
			router.Use(RBACMiddleware())
			router.GET("/api/v1/:crd/_all/_watch", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})
			router.POST("/api/v1/:crd/_all/_watch", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(
				tt.method,
				"/api/v1/"+crdName+"/_all/_watch"+tt.query,
				nil,
			)
			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %q", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
		})
	}
}
