package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func securityTestUser(verbs []string, resources []string, resourceNames ...string) model.User {
	return model.User{Roles: []common.Role{{
		Name:          "security-test",
		Clusters:      []string{"cluster-a"},
		Namespaces:    []string{"team-a"},
		Resources:     resources,
		ResourceNames: resourceNames,
		Verbs:         verbs,
	}}}
}

func TestConfigMapListHidesGatewayProviderFromReadOnlyUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	protected := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: "gateway-config", Namespace: "team-a",
		Labels: map[string]string{gatewayProviderLabel: "envoy"},
	}}
	public := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: "public-config", Namespace: "team-a",
	}}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/configmaps/team-a", nil)
	c.Params = gin.Params{{Key: "namespace", Value: "team-a"}}
	c.Set("cluster", &cluster.ClientSet{
		Name: "cluster-a",
		K8sClient: &kube.K8sClient{Client: ctrlfake.NewClientBuilder().
			WithScheme(scheme).WithObjects(protected, public).Build()},
	})
	c.Set("user", securityTestUser([]string{"get"}, []string{"configmaps"}))

	handler := NewGenericResourceHandler[*corev1.ConfigMap, *corev1.ConfigMapList](common.ConfigMaps)
	handler.List(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %q", recorder.Code, recorder.Body.String())
	}
	var got corev1.ConfigMapList
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].Name != public.Name {
		t.Fatalf("items = %#v, want only %q", got.Items, public.Name)
	}
}

func TestGatewayProviderConfigMapVisibleWithNamedWriteAccess(t *testing.T) {
	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      "gateway-config",
		Namespace: "team-a",
		Labels:    map[string]string{gatewayProviderLabel: "envoy"},
	}}
	user := securityTestUser(
		[]string{"get", "update"},
		[]string{"configmaps"},
		configMap.Name,
	)

	if !canViewResourceObject(user, "configmaps", "cluster-a", "team-a", configMap.Name, configMap) {
		t.Fatal("selected ConfigMap should be visible after write access is granted")
	}
	if canViewResourceObject(user, "configmaps", "cluster-a", "team-a", "other", configMap) {
		t.Fatal("write access to another name must not expose the protected ConfigMap")
	}
}

func TestSecretGetRedactsContentForReadOnlyUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "team-a"},
		Data:       map[string][]byte{"password": []byte("super-secret")},
		StringData: map[string]string{"token": "plain-secret"},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/secrets/team-a/credentials", nil)
	c.Params = gin.Params{
		{Key: "namespace", Value: "team-a"},
		{Key: "name", Value: "credentials"},
	}
	c.Set("cluster", &cluster.ClientSet{
		Name: "cluster-a",
		K8sClient: &kube.K8sClient{Client: ctrlfake.NewClientBuilder().
			WithScheme(scheme).WithObjects(secret).Build()},
	})
	c.Set("user", securityTestUser([]string{"get"}, []string{"secrets"}))

	handler := NewGenericResourceHandler[*corev1.Secret, *corev1.SecretList](common.Secrets)
	handler.Get(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %q", recorder.Code, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, exists := response["data"]; exists {
		t.Fatalf("read-only response exposes data: %s", recorder.Body.String())
	}
	if _, exists := response["stringData"]; exists {
		t.Fatalf("read-only response exposes stringData: %s", recorder.Body.String())
	}
}

func TestWatchSecurityFiltersConfigMapsAndRedactsSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newContext := func(user model.User) *gin.Context {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/resources/team-a/_watch", nil)
		c.Params = gin.Params{{Key: "namespace", Value: "team-a"}}
		c.Set("cluster", &cluster.ClientSet{Name: "cluster-a"})
		c.Set("user", user)
		return c
	}

	viewer := securityTestUser([]string{"get"}, []string{"*"})
	protectedConfigMap := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "gateway-config", "namespace": "team-a",
			"labels": map[string]any{gatewayProviderLabel: "envoy"},
		},
	}}
	if canStreamWatchObject(newContext(viewer), "configmaps", protectedConfigMap) {
		t.Fatal("watch must filter a protected ConfigMap for a read-only user")
	}

	editor := securityTestUser([]string{"get", "update"}, []string{"configmaps"}, "gateway-config")
	if !canStreamWatchObject(newContext(editor), "configmaps", protectedConfigMap) {
		t.Fatal("watch should retain a protected ConfigMap with named write access")
	}

	secret := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "credentials", "namespace": "team-a"},
		"data":     map[string]any{"password": "c3VwZXItc2VjcmV0"},
	}}
	transformed, err := transformWatchObject(
		newContext(viewer),
		resourceWatchStreamOptions{Resource: "secrets"},
		"MODIFIED",
		secret,
	)
	if err != nil {
		t.Fatalf("transformWatchObject() error = %v", err)
	}
	got := transformed.(*unstructured.Unstructured)
	if _, found, _ := unstructured.NestedMap(got.Object, "data"); found {
		t.Fatalf("read-only watch exposes Secret data: %#v", got.Object)
	}
	if _, found, _ := unstructured.NestedMap(secret.Object, "data"); !found {
		t.Fatal("watch transformation unexpectedly mutated the shared source object")
	}
}

func TestCustomResourceDetailsRequireWriteAccessExceptLWS(t *testing.T) {
	viewer := securityTestUser([]string{"get"}, []string{"*"})
	ordinary := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{
		Name: "widgets.example.io",
	}}
	lws := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{
		Name: leaderWorkerSetCRD,
	}}

	if canViewCustomResourceDetails(viewer, "cluster-a", ordinary.Name, "team-a", "demo", ordinary) {
		t.Fatal("read-only user must not view ordinary custom-resource details")
	}
	if !canViewCustomResourceDetails(viewer, "cluster-a", lws.Name, "team-a", "demo", lws) {
		t.Fatal("LWS details must remain visible to read-only users")
	}

	editor := securityTestUser([]string{"get", "update"}, []string{ordinary.Name}, "demo")
	if !canViewCustomResourceDetails(editor, "cluster-a", ordinary.Name, "team-a", "demo", ordinary) {
		t.Fatal("named write access should expose custom-resource details")
	}
}

func TestReduceCustomResourceToMetadataRemovesDetails(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.io/v1",
		"kind":       "Widget",
		"metadata": map[string]any{
			"name": "demo", "namespace": "team-a",
			"annotations": map[string]any{"sensitive": "value"},
		},
		"spec":   map[string]any{"password": "secret"},
		"status": map[string]any{"internal": "detail"},
	}}

	reduceCustomResourceToMetadata(obj)

	if _, exists := obj.Object["spec"]; exists {
		t.Fatalf("reduced object still exposes spec: %#v", obj.Object)
	}
	if _, exists := obj.Object["status"]; exists {
		t.Fatalf("reduced object still exposes status: %#v", obj.Object)
	}
	if annotations := obj.GetAnnotations(); len(annotations) != 0 {
		t.Fatalf("reduced object still exposes annotations: %#v", annotations)
	}
	if obj.GetName() != "demo" || obj.GetNamespace() != "team-a" {
		t.Fatalf("reduced identity = %s/%s", obj.GetNamespace(), obj.GetName())
	}
}

func TestCRListRedactsDetailsAndGetForbidsReadOnlyUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scheme := runtime.NewScheme()
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	gv := schema.GroupVersion{Group: "example.io", Version: "v1"}
	scheme.AddKnownTypeWithName(gv.WithKind("Widget"), &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(gv.WithKind("WidgetList"), &unstructured.UnstructuredList{})
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "widgets.example.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural: "widgets", Kind: "Widget", ListKind: "WidgetList",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name: "v1", Served: true, Storage: true,
			}},
		},
	}
	widget := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.io/v1",
		"kind":       "Widget",
		"metadata": map[string]any{
			"name": "demo", "namespace": "team-a",
		},
		"spec":   map[string]any{"credential": "sensitive"},
		"status": map[string]any{"internal": "detail"},
	}}
	client := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(crd, widget).Build()
	viewer := securityTestUser([]string{"get"}, []string{"widgets.example.io"})

	t.Run("list retains identity only", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/widgets.example.io/team-a", nil)
		c.Params = gin.Params{
			{Key: "crd", Value: crd.Name},
			{Key: "namespace", Value: "team-a"},
		}
		c.Set("cluster", &cluster.ClientSet{Name: "cluster-a", K8sClient: &kube.K8sClient{Client: client}})
		c.Set("user", viewer)

		NewCRHandler().List(c)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %q", recorder.Code, recorder.Body.String())
		}
		body := recorder.Body.String()
		if !json.Valid(recorder.Body.Bytes()) || !containsAll(body, `"name":"demo"`, `"namespace":"team-a"`) {
			t.Fatalf("list response lost resource identity: %s", body)
		}
		if containsAny(body, "credential", "sensitive", `"spec"`, `"status"`) {
			t.Fatalf("read-only list exposes custom-resource details: %s", body)
		}
	})

	t.Run("get is forbidden", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/widgets.example.io/team-a/demo", nil)
		c.Params = gin.Params{
			{Key: "crd", Value: crd.Name},
			{Key: "namespace", Value: "team-a"},
			{Key: "name", Value: "demo"},
		}
		c.Set("cluster", &cluster.ClientSet{Name: "cluster-a", K8sClient: &kube.K8sClient{Client: client}})
		c.Set("user", viewer)

		NewCRHandler().Get(c)

		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body = %q", recorder.Code, recorder.Body.String())
		}
	})
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

func containsAny(value string, parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(value, part) {
			return true
		}
	}
	return false
}
