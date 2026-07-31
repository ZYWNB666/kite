package resources

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRegisterRoutesAddsWatchToSupportedResources(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1"))

	routes := router.Routes()
	assertRouteCount := func(path string, want int) {
		t.Helper()
		count := 0
		for _, route := range routes {
			if route.Method == "GET" && route.Path == path {
				count++
			}
		}
		if count != want {
			t.Fatalf("GET %s registered %d times, want %d", path, count, want)
		}
	}

	assertRouteCount("/api/v1/services/:namespace/_watch", 1)
	assertRouteCount("/api/v1/configmaps/:namespace/_watch", 1)
	assertRouteCount("/api/v1/pods/:namespace/_watch", 1)
	assertRouteCount("/api/v1/nodes/_all/_watch", 0)
	assertRouteCount("/api/v1/:crd/:namespace/_watch", 1)
	assertRouteCount("/api/v1/services/:namespace/watch", 0)
	assertRouteCount("/api/v1/:crd/:namespace/watch", 0)
}

func TestReduceWatchObjectPreservesConfigMapAndSecretKeys(t *testing.T) {
	tests := []struct {
		resource string
		field    string
	}{
		{resource: string(common.ConfigMaps), field: "data"},
		{resource: string(common.Secrets), field: "data"},
	}

	for _, tt := range tests {
		t.Run(tt.resource, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: map[string]any{
				tt.field: map[string]any{
					"alpha": "sensitive-value",
					"beta":  "another-value",
				},
			}}

			reduceWatchObject(tt.resource, obj)

			values, found, err := unstructured.NestedMap(obj.Object, tt.field)
			if err != nil || !found {
				t.Fatalf("NestedMap() = (%v, %v), want values", found, err)
			}
			if values["alpha"] != "" || values["beta"] != "" {
				t.Fatalf("reduced values = %#v, want empty values with keys preserved", values)
			}
		})
	}
}

func TestWatchedPodWithMetricsKeepsStableIdentityWhenReduced(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "demo",
			Namespace:       "default",
			UID:             types.UID("pod-uid"),
			ResourceVersion: "42",
		},
	}

	result := watchedPodWithMetrics(pod, nil, true)
	if result.UID != pod.UID || result.ResourceVersion != pod.ResourceVersion {
		t.Fatalf(
			"reduced identity = (%q, %q), want (%q, %q)",
			result.UID,
			result.ResourceVersion,
			pod.UID,
			pod.ResourceVersion,
		)
	}
}

func TestCRWatchReturnsDiscoveryFailureAsSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scheme := runtime.NewScheme()
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/prometheuses.monitoring.coreos.com/_all/_watch",
		nil,
	)
	c.Params = gin.Params{
		{Key: "crd", Value: "prometheuses.monitoring.coreos.com"},
		{Key: "namespace", Value: common.AllNamespaces},
	}
	c.Set("cluster", &cluster.ClientSet{
		Name: "test",
		K8sClient: &kube.K8sClient{
			Client: ctrlfake.NewClientBuilder().WithScheme(scheme).Build(),
		},
	})

	NewCRHandler().Watch(c)

	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "event: watch-error") ||
		!strings.Contains(body, `"fatal":true`) ||
		!strings.Contains(body, "CustomResourceDefinition not found") {
		t.Fatalf("watch response = %q, want fatal CRD discovery SSE error", body)
	}
}

func TestServeResourceWatchEndsAtConnectionLifetime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "services"}
	service := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]any{
			"name":            "demo",
			"namespace":       "default",
			"uid":             "uid-1",
			"resourceVersion": "10",
		},
	}}
	dynamicClient := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "ServiceList"},
		service,
	)
	fakeWatcher := watch.NewRaceFreeFake()
	defer fakeWatcher.Stop()
	dynamicClient.PrependWatchReactor(
		"services",
		func(ktesting.Action) (bool, watch.Interface, error) {
			return true, fakeWatcher, nil
		},
	)

	hub := kube.NewResourceWatchHub(dynamicClient)
	defer hub.Close()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/services/default/_watch",
		nil,
	)
	c.Params = gin.Params{{Key: "namespace", Value: "default"}}
	c.Set("cluster", &cluster.ClientSet{
		Name: "test",
		K8sClient: &kube.K8sClient{
			WatchHub: hub,
		},
	})
	c.Set("user", model.User{})

	started := time.Now()
	serveResourceWatch(c, resourceWatchStreamOptions{
		GVR:                gvr,
		Resource:           string(common.Services),
		ConnectionLifetime: 200 * time.Millisecond,
	})
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("watch ran for %s, want bounded connection lifetime", elapsed)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "event: snapshot") ||
		!strings.Contains(body, "event: ready") {
		t.Fatalf("watch response = %q, want snapshot and ready events", body)
	}
}
