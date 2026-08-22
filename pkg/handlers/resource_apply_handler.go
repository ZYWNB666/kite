package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	syaml "sigs.k8s.io/yaml"
)

type ResourceApplyHandler struct {
}

func NewResourceApplyHandler() *ResourceApplyHandler {
	return &ResourceApplyHandler{}
}

type ApplyResourceRequest struct {
	YAML       string `json:"yaml" binding:"required"`
	CreateOnly bool   `json:"createOnly"`
}

type appliedResource struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type preparedResource struct {
	object       *unstructured.Unstructured
	existing     *unstructured.Unstructured
	resource     string
	documentYAML string
	verb         string
}

// ApplyResource applies one or more YAML documents to the cluster. Documents
// are processed in input order so later resources can depend on earlier ones.
func (h *ResourceApplyHandler) ApplyResource(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)

	var req ApplyResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	objects, err := decodeYAMLDocuments(req.YAML)
	if err != nil {
		klog.Errorf("Failed to decode YAML: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid YAML format: " + err.Error()})
		return
	}

	ctx := c.Request.Context()
	prepared := make([]preparedResource, 0, len(objects))
	for i, obj := range objects {
		resource := strings.ToLower(obj.GetKind()) + "s"
		existing := &unstructured.Unstructured{}
		existing.SetGroupVersionKind(obj.GroupVersionKind())
		err = cs.K8sClient.Get(ctx, client.ObjectKey{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}, existing)

		verb := string(common.VerbCreate)
		switch {
		case err == nil:
			verb = string(common.VerbUpdate)
		case apierrors.IsNotFound(err):
			// Creating a new resource keeps the create verb.
		default:
			klog.Errorf("Failed to get resource in document %d before authorization: %v", i+1, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("Document %d: failed to get resource: %v", i+1, err),
			})
			return
		}

		if !rbac.CanAccess(user, resource, verb, cs.Name, obj.GetNamespace(), obj.GetName()) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": fmt.Sprintf("Document %d: %s", i+1, rbac.NoAccess(user.Key(), verb, resource, obj.GetNamespace(), cs.Name)),
			})
			return
		}
		if req.CreateOnly && err == nil {
			c.JSON(http.StatusConflict, gin.H{
				"error": fmt.Sprintf("Document %d: %s \"%s\" already exists", i+1, obj.GetKind(), obj.GetName()),
			})
			return
		}

		documentYAML, _ := syaml.Marshal(obj)
		prepared = append(prepared, preparedResource{
			object:       obj,
			existing:     existing,
			resource:     resource,
			documentYAML: string(documentYAML),
			verb:         verb,
		})
	}

	results := make([]appliedResource, 0, len(prepared))
	for i := range prepared {
		item := &prepared[i]
		applyErr := applyPreparedResource(ctx, cs, item)
		recordApplyHistory(cs, user, item, applyErr)
		if applyErr != nil {
			klog.Errorf("Failed to apply resource in document %d: %v", i+1, applyErr)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   fmt.Sprintf("Document %d: failed to apply %s %q: %v", i+1, item.object.GetKind(), item.object.GetName(), applyErr),
				"applied": results,
				"count":   len(results),
			})
			return
		}

		klog.Infof("Successfully applied resource: %s/%s", item.object.GetKind(), item.object.GetName())
		results = append(results, appliedResource{
			Kind:      item.object.GetKind(),
			Name:      item.object.GetName(),
			Namespace: item.object.GetNamespace(),
		})
	}

	first := results[0]
	message := "Resource applied successfully"
	if len(results) > 1 {
		message = fmt.Sprintf("%d resources applied successfully", len(results))
	}
	c.JSON(http.StatusOK, gin.H{
		"message":   message,
		"kind":      first.Kind,
		"name":      first.Name,
		"namespace": first.Namespace,
		"resources": results,
		"count":     len(results),
	})
}

func decodeYAMLDocuments(content string) ([]*unstructured.Unstructured, error) {
	decoder := yamlutil.NewYAMLOrJSONDecoder(strings.NewReader(content), 4096)
	objects := make([]*unstructured.Unstructured, 0, 1)
	document := 0
	for {
		document++
		var raw runtime.RawExtension
		if err := decoder.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("document %d: %w", document, err)
		}
		if len(raw.Raw) == 0 {
			continue
		}

		obj := &unstructured.Unstructured{}
		if _, _, err := unstructured.UnstructuredJSONScheme.Decode(raw.Raw, nil, obj); err != nil {
			return nil, fmt.Errorf("document %d: %w", document, err)
		}
		if obj.GetAPIVersion() == "" || obj.GetKind() == "" {
			return nil, fmt.Errorf("document %d: apiVersion and kind are required", document)
		}
		objects = append(objects, obj)
	}
	if len(objects) == 0 {
		return nil, errors.New("no Kubernetes resources found")
	}
	return objects, nil
}

func applyPreparedResource(ctx context.Context, cs *cluster.ClientSet, item *preparedResource) error {
	if item.verb == string(common.VerbCreate) {
		return cs.K8sClient.Create(ctx, item.object)
	}

	item.object.SetResourceVersion(item.existing.GetResourceVersion())
	return cs.K8sClient.Update(ctx, item.object)
}

func recordApplyHistory(cs *cluster.ClientSet, user model.User, item *preparedResource, applyErr error) {
	if model.DB == nil {
		return
	}
	previousYAML := []byte{}
	if item.existing.GetResourceVersion() != "" {
		item.existing.SetManagedFields(nil)
		previousYAML, _ = syaml.Marshal(item.existing)
	}
	errMessage := ""
	if applyErr != nil {
		errMessage = applyErr.Error()
	}
	model.DB.Create(&model.ResourceHistory{
		ClusterName:   cs.Name,
		ResourceType:  item.resource,
		ResourceName:  item.object.GetName(),
		Namespace:     item.object.GetNamespace(),
		OperationType: "apply",
		ResourceYAML:  item.documentYAML,
		PreviousYAML:  string(previousYAML),
		OperatorID:    user.ID,
		Success:       applyErr == nil,
		ErrorMessage:  errMessage,
	})
}
