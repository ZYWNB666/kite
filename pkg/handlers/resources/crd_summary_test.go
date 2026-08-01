package resources

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestListCRDSummariesExcludesLargeDefinitionFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scheme := runtime.NewScheme()
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "leaderworkersets.leaderworkerset.x-k8s.io",
			Annotations: map[string]string{"large": strings.Repeat("a", 10000)},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "leaderworkerset.x-k8s.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural: "leaderworkersets",
				Kind:   "LeaderWorkerSet",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name:    "v1",
				Served:  true,
				Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type:        "object",
						Description: strings.Repeat("s", 10000),
					},
				},
				AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{{
					Name: "Ready", Type: "integer", JSONPath: ".status.readyReplicas",
				}},
			}},
		},
		Status: apiextensionsv1.CustomResourceDefinitionStatus{
			Conditions: []apiextensionsv1.CustomResourceDefinitionCondition{{
				Type: apiextensionsv1.Established, Status: apiextensionsv1.ConditionTrue,
			}},
		},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/crds/_all/_summaries", nil)
	c.Set("cluster", &cluster.ClientSet{
		Name: "test",
		K8sClient: &kube.K8sClient{
			Client: ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(crd).Build(),
		},
	})
	c.Set("user", model.User{})

	handler := NewGenericResourceHandler[
		*apiextensionsv1.CustomResourceDefinition,
		*apiextensionsv1.CustomResourceDefinitionList,
	](common.CRDs)
	handler.listCRDSummaries(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %q", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, excluded := range []string{"openAPIV3Schema", "schema", "metadata", "annotations", strings.Repeat("s", 100)} {
		if strings.Contains(body, excluded) {
			t.Fatalf("summary response contains excluded CRD field %q", excluded)
		}
	}
	for _, required := range []string{"LeaderWorkerSet", "leaderworkerset.x-k8s.io", "additionalPrinterColumns", ".status.readyReplicas", `"established":true`} {
		if !strings.Contains(body, required) {
			t.Fatalf("summary response is missing %q: %s", required, body)
		}
	}
	if recorder.Body.Len() > 2048 {
		t.Fatalf("summary response size = %d bytes, want <= 2048", recorder.Body.Len())
	}
}
