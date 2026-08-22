package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/kube"
	"github.com/zxh326/kite/pkg/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const multipleResourcesYAML = `apiVersion: v1
kind: Namespace
metadata:
  name: magik-tunnel-manager
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: magik-tunnel-connector-data
  namespace: magik-tunnel-manager
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 256Mi
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: magik-tunnel-connector
  namespace: magik-tunnel-manager
`

func TestDecodeYAMLDocuments(t *testing.T) {
	objects, err := decodeYAMLDocuments("---\n\n" + multipleResourcesYAML + "---\n")
	if err != nil {
		t.Fatalf("decodeYAMLDocuments() error = %v", err)
	}
	if len(objects) != 3 {
		t.Fatalf("decoded %d objects, want 3", len(objects))
	}
	wantKinds := []string{"Namespace", "PersistentVolumeClaim", "ServiceAccount"}
	for i, wantKind := range wantKinds {
		if objects[i].GetKind() != wantKind {
			t.Fatalf("object %d kind = %q, want %q", i+1, objects[i].GetKind(), wantKind)
		}
	}
}

func TestDecodeYAMLDocumentsReportsDocumentNumber(t *testing.T) {
	_, err := decodeYAMLDocuments(`apiVersion: v1
kind: Namespace
metadata:
  name: valid
---
apiVersion: v1
kind: ConfigMap
metadata: [invalid
`)
	if err == nil {
		t.Fatal("decodeYAMLDocuments() error = nil, want malformed YAML error")
	}
	if !strings.Contains(err.Error(), "document 2") {
		t.Fatalf("error = %q, want document number", err)
	}
}

func TestApplyResourceCreatesMultipleDocumentsInOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core types to scheme: %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	cs := &cluster.ClientSet{
		Name: "test-cluster",
		K8sClient: &kube.K8sClient{
			Client: fakeClient,
		},
	}

	body, err := json.Marshal(ApplyResourceRequest{YAML: multipleResourcesYAML, CreateOnly: true})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/resources/apply", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("cluster", cs)
	c.Set("user", model.AnonymousUser)

	NewResourceApplyHandler().ApplyResource(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response struct {
		Count     int               `json:"count"`
		Resources []appliedResource `json:"resources"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Count != 3 || len(response.Resources) != 3 {
		t.Fatalf("response = %+v, want three resources", response)
	}

	assertUnstructuredExists(t, fakeClient, schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, "", "magik-tunnel-manager")
	assertUnstructuredExists(t, fakeClient, schema.GroupVersionKind{Version: "v1", Kind: "PersistentVolumeClaim"}, "magik-tunnel-manager", "magik-tunnel-connector-data")
	assertUnstructuredExists(t, fakeClient, schema.GroupVersionKind{Version: "v1", Kind: "ServiceAccount"}, "magik-tunnel-manager", "magik-tunnel-connector")
}

func assertUnstructuredExists(t *testing.T, k8sClient client.Client, gvk schema.GroupVersionKind, namespace, name string) {
	t.Helper()
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		t.Fatalf("get %s %s/%s: %v", gvk.Kind, namespace, name, err)
	}
}
