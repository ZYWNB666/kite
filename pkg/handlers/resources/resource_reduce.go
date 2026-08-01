package resources

import (
	"github.com/zxh326/kite/pkg/common"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// reduceResourceObject removes fields that list and table views do not use.
// The full object remains available when reduce is not requested.
func reduceResourceObject(resource string, object any) {
	if resource != string(common.CRDs) {
		return
	}

	if crd, ok := object.(*apiextensionsv1.CustomResourceDefinition); ok {
		reduceCustomResourceDefinition(crd)
	}
}

func reduceCustomResourceDefinition(crd *apiextensionsv1.CustomResourceDefinition) {
	crd.Spec.Conversion = nil
	for i := range crd.Spec.Versions {
		crd.Spec.Versions[i].Schema = nil
		crd.Spec.Versions[i].Subresources = nil
		crd.Spec.Versions[i].SelectableFields = nil
	}
	crd.Status.AcceptedNames = apiextensionsv1.CustomResourceDefinitionNames{}
	crd.Status.StoredVersions = nil
}

func reduceUnstructuredCustomResourceDefinition(obj *unstructured.Unstructured) {
	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err == nil && found {
		delete(spec, "conversion")
		if versions, ok := spec["versions"].([]any); ok {
			for _, version := range versions {
				versionMap, ok := version.(map[string]any)
				if !ok {
					continue
				}
				delete(versionMap, "schema")
				delete(versionMap, "subresources")
				delete(versionMap, "selectableFields")
			}
		}
		_ = unstructured.SetNestedMap(obj.Object, spec, "spec")
	}

	status, found, err := unstructured.NestedMap(obj.Object, "status")
	if err == nil && found {
		delete(status, "acceptedNames")
		delete(status, "storedVersions")
		_ = unstructured.SetNestedMap(obj.Object, status, "status")
	}
}
