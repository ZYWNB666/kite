package resources

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	gatewayProviderLabel = "gateway.magikcompute.ai/provider"
	leaderWorkerSetCRD   = "leaderworkersets.leaderworkerset.x-k8s.io"
)

// hasResourceWriteAccess distinguishes users who can only inspect a namespace
// from users who are allowed to operate on the concrete resource. In
// particular, route-adjustment roles grant update for the selected ConfigMaps.
func hasResourceWriteAccess(user model.User, resource, clusterName, namespace, resourceName string) bool {
	return rbac.CanAccess(
		user,
		resource,
		string(common.VerbUpdate),
		clusterName,
		namespace,
		resourceName,
	)
}

func isGatewayProviderConfigMap(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	_, exists := obj.GetLabels()[gatewayProviderLabel]
	return exists
}

func canViewResourceObject(
	user model.User,
	resource, clusterName, namespace, resourceName string,
	obj metav1.Object,
) bool {
	if resource != string(common.ConfigMaps) || !isGatewayProviderConfigMap(obj) {
		return true
	}
	return hasResourceWriteAccess(user, resource, clusterName, namespace, resourceName)
}

func redactSecretContent(object any) {
	switch secret := object.(type) {
	case *corev1.Secret:
		secret.Data = nil
		secret.StringData = nil
	case *unstructured.Unstructured:
		unstructured.RemoveNestedField(secret.Object, "data")
		unstructured.RemoveNestedField(secret.Object, "stringData")
		unstructured.RemoveNestedField(secret.Object, "binaryData")
	}
}

func isLeaderWorkerSet(crd *apiextensionsv1.CustomResourceDefinition) bool {
	return crd != nil && crd.Name == leaderWorkerSetCRD
}

func canViewCustomResourceDetails(
	user model.User,
	clusterName, crdName, namespace, resourceName string,
	crd *apiextensionsv1.CustomResourceDefinition,
) bool {
	return isLeaderWorkerSet(crd) || hasResourceWriteAccess(
		user,
		crdName,
		clusterName,
		namespace,
		resourceName,
	)
}

// reduceCustomResourceToMetadata retains only the fields needed to identify an
// item in a list/watch stream. A read-only user must not be able to reconstruct
// resource details from a collection response.
func reduceCustomResourceToMetadata(obj *unstructured.Unstructured) {
	if obj == nil {
		return
	}
	metadata := map[string]any{
		"name":            obj.GetName(),
		"namespace":       obj.GetNamespace(),
		"uid":             string(obj.GetUID()),
		"resourceVersion": obj.GetResourceVersion(),
		"generation":      obj.GetGeneration(),
	}
	if creationTimestamp := obj.GetCreationTimestamp(); !creationTimestamp.IsZero() {
		metadata["creationTimestamp"] = creationTimestamp.UTC().Format(time.RFC3339)
	}
	if deletionTimestamp := obj.GetDeletionTimestamp(); deletionTimestamp != nil {
		metadata["deletionTimestamp"] = deletionTimestamp.UTC().Format(time.RFC3339)
	}
	obj.Object = map[string]any{
		"apiVersion": obj.GetAPIVersion(),
		"kind":       obj.GetKind(),
		"metadata":   metadata,
	}
}

func denyCustomResourceDetails(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"error": "resource details are not available to read-only users",
	})
}
