package resources

import (
	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
)

const requestedNamespacesContextKey = "resource-requested-namespaces"

type requestedNamespaceSelection struct {
	all    bool
	values map[string]struct{}
}

func requestedNamespaces(c *gin.Context) requestedNamespaceSelection {
	if cached, ok := c.Get(requestedNamespacesContextKey); ok {
		return cached.(requestedNamespaceSelection)
	}

	selection := requestedNamespaceSelection{all: true}
	if namespaces, restricted := common.ParseRequestedNamespaces(c.Query("namespaces")); restricted {
		selection.all = false
		selection.values = make(map[string]struct{})
		for _, namespace := range namespaces {
			selection.values[namespace] = struct{}{}
		}
	}

	c.Set(requestedNamespacesContextKey, selection)
	return selection
}

func matchesRequestedNamespace(c *gin.Context, namespace string) bool {
	selection := requestedNamespaces(c)
	if selection.all || namespace == "" {
		return true
	}
	_, ok := selection.values[namespace]
	return ok
}
