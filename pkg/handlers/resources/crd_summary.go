package resources

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

type crdVersionSummary struct {
	Name                     string                                           `json:"name"`
	Served                   bool                                             `json:"served"`
	Storage                  bool                                             `json:"storage"`
	AdditionalPrinterColumns []apiextensionsv1.CustomResourceColumnDefinition `json:"additionalPrinterColumns,omitempty"`
}

type crdSummary struct {
	Name        string              `json:"name"`
	Group       string              `json:"group"`
	Kind        string              `json:"kind"`
	Scope       string              `json:"scope"`
	CreatedAt   string              `json:"createdAt,omitempty"`
	Established bool                `json:"established"`
	Versions    []crdVersionSummary `json:"versions"`
}

func (h *GenericResourceHandler[T, V]) listCRDSummaries(c *gin.Context) {
	objectList, err := h.list(c)
	if err != nil {
		return
	}
	crdList, ok := any(objectList).(*apiextensionsv1.CustomResourceDefinitionList)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build CRD summaries"})
		return
	}

	summaries := make([]crdSummary, 0, len(crdList.Items))
	for i := range crdList.Items {
		crd := &crdList.Items[i]
		versions := make([]crdVersionSummary, 0, len(crd.Spec.Versions))
		for j := range crd.Spec.Versions {
			version := &crd.Spec.Versions[j]
			versions = append(versions, crdVersionSummary{
				Name:                     version.Name,
				Served:                   version.Served,
				Storage:                  version.Storage,
				AdditionalPrinterColumns: version.AdditionalPrinterColumns,
			})
		}

		established := false
		for _, condition := range crd.Status.Conditions {
			if condition.Type == apiextensionsv1.Established && condition.Status == apiextensionsv1.ConditionTrue {
				established = true
				break
			}
		}

		createdAt := ""
		if !crd.CreationTimestamp.IsZero() {
			createdAt = crd.CreationTimestamp.Time.UTC().Format(time.RFC3339)
		}
		summaries = append(summaries, crdSummary{
			Name:        crd.Name,
			Group:       crd.Spec.Group,
			Kind:        crd.Spec.Names.Kind,
			Scope:       string(crd.Spec.Scope),
			CreatedAt:   createdAt,
			Established: established,
			Versions:    versions,
		})
	}

	c.JSON(http.StatusOK, gin.H{"items": summaries})
}
