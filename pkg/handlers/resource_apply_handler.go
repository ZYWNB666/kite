package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
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

// ApplyResource applies a YAML resource to the cluster
func (h *ResourceApplyHandler) ApplyResource(c *gin.Context) {
	cs := c.MustGet("cluster").(*cluster.ClientSet)
	user := c.MustGet("user").(model.User)

	var req ApplyResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Decode YAML into unstructured object
	decodeUniversal := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}

	_, _, err := decodeUniversal.Decode([]byte(req.YAML), nil, obj)
	if err != nil {
		klog.Errorf("Failed to decode YAML: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid YAML format: " + err.Error()})
		return
	}

	resource := strings.ToLower(obj.GetKind()) + "s"

	ctx := c.Request.Context()

	existingObj := &unstructured.Unstructured{}
	existingObj.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	existingObj.SetName(obj.GetName())
	existingObj.SetNamespace(obj.GetNamespace())

	err = cs.K8sClient.Get(ctx, client.ObjectKey{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}, existingObj)
	verb := string(common.VerbCreate)
	switch {
	case err == nil:
		verb = string(common.VerbUpdate)
	case apierrors.IsNotFound(err):
		// Creating a new resource keeps the create verb.
	default:
		klog.Errorf("Failed to get resource before authorization: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get resource: " + err.Error()})
		return
	}
	if !rbac.CanAccess(user, resource, verb, cs.Name, obj.GetNamespace(), obj.GetName()) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": rbac.NoAccess(user.Key(), verb, resource, obj.GetNamespace(), cs.Name)})
		return
	}

	defer func() {
		previousYAML := []byte{}
		if existingObj.GetResourceVersion() != "" {
			existingObj.SetManagedFields(nil)
			previousYAML, _ = syaml.Marshal(existingObj)
		}
		errMessage := ""
		if err != nil {
			errMessage = err.Error()
		}
		model.DB.Create(&model.ResourceHistory{
			ClusterName:   cs.Name,
			ResourceType:  resource,
			ResourceName:  obj.GetName(),
			Namespace:     obj.GetNamespace(),
			OperationType: "apply",
			ResourceYAML:  req.YAML,
			PreviousYAML:  string(previousYAML),
			OperatorID:    user.ID,
			Success:       err == nil,
			ErrorMessage:  errMessage,
		})
	}()

	switch {
	case apierrors.IsNotFound(err):
		if err := cs.K8sClient.Create(ctx, obj); err != nil {
			klog.Errorf("Failed to create resource: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create resource: " + err.Error()})
			return
		}
		err = nil // Clear error after successful creation
	case err == nil:
		if req.CreateOnly {
			err = apierrors.NewAlreadyExists(schema.GroupResource{Resource: resource}, obj.GetName())
			c.JSON(http.StatusConflict, gin.H{"error": obj.GetKind() + " \"" + obj.GetName() + "\" already exists"})
			return
		}
		obj.SetResourceVersion(existingObj.GetResourceVersion())
		if err := cs.K8sClient.Update(ctx, obj); err != nil {
			klog.Errorf("Failed to update resource: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update resource: " + err.Error()})
			return
		}
	default:
		klog.Errorf("Failed to get resource: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get resource: " + err.Error()})
		return
	}

	klog.Infof("Successfully applied resource: %s/%s", obj.GetKind(), obj.GetName())
	c.JSON(http.StatusOK, gin.H{
		"message":   "Resource applied successfully",
		"kind":      obj.GetKind(),
		"name":      obj.GetName(),
		"namespace": obj.GetNamespace(),
	})
}
