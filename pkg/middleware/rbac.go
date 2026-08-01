package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/rbac"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
)

func RBACMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(model.User)
		cs := c.MustGet("cluster").(*cluster.ClientSet)

		verbs := method2verb(c.Request.Method)
		ns, resource, resourceName := url2namespaceresource(
			c.Request.URL.Path,
			c.FullPath(),
		)
		if ns == "" || resource == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid resource URL"})
			return
		}
		if resource == string(common.Namespaces) && verbs == "get" {
			// if user has roles, allow access to list namespaces resource
			// don't worry about security here, we will filter namespaces in the list namespace handler
			// this is just to allow users to list namespaces they have access to
			c.Next()
			return
		}

		canAccess := canAccessResourceRequest(
			c,
			user,
			cs,
			resource,
			verbs,
			ns,
			resourceName,
		)
		if canAccess {
			c.Next()
		} else {
			c.AbortWithStatusJSON(http.StatusForbidden,
				gin.H{"error": rbac.NoAccess(user.Key(), verbs, resource, ns, cs.Name)})
		}
	}
}

func canAccessResourceRequest(
	c *gin.Context,
	user model.User,
	cs *cluster.ClientSet,
	resource string,
	verb string,
	namespace string,
	resourceName string,
) bool {
	if verb != string(common.VerbGet) || namespace != common.AllNamespaces || resourceName != "" {
		return rbac.CanAccess(user, resource, verb, cs.Name, namespace, resourceName)
	}

	namespaces, restricted := common.ParseRequestedNamespaces(c.Query("namespaces"))
	if !restricted || !isNamespaceScopedResource(c.Request.Context(), cs, resource) {
		return rbac.CanAccess(user, resource, verb, cs.Name, namespace, resourceName)
	}

	for _, requestedNamespace := range namespaces {
		if !rbac.CanAccess(user, resource, verb, cs.Name, requestedNamespace, resourceName) {
			return false
		}
	}
	return true
}

func isNamespaceScopedResource(ctx context.Context, cs *cluster.ClientSet, resource string) bool {
	if resourceMeta := common.LookupResource(resource); resourceMeta != nil {
		return !resourceMeta.ClusterScoped
	}
	if cs == nil || cs.K8sClient == nil {
		return false
	}

	var crd apiextensionsv1.CustomResourceDefinition
	if err := cs.K8sClient.Get(ctx, types.NamespacedName{Name: resource}, &crd); err != nil {
		return false
	}
	return crd.Spec.Scope == apiextensionsv1.NamespaceScoped
}

func method2verb(method string) string {
	switch method {
	case http.MethodPost:
		return string(common.VerbCreate)
	case http.MethodPut, http.MethodPatch:
		return string(common.VerbUpdate)
	default:
		return strings.ToLower(method)
	}
}

// url2namespaceresource converts a URL path to namespace, resource type, and optional resource name.
// matchedRoute is Gin's registered route pattern. Using it to identify :name
// avoids confusing legal object names such as "watch" or "history" with
// collection/sub-resource path segments.
// For example:
//
// - /api/v1/pods/default/nginx     => default, pods, nginx
// - /api/v1/pvs/_all/some-pv      => _all, pvs, some-pv
// - /api/v1/pods/default           => default, pods, ""
// - /api/v1/pods                   => "", pods, ""
func url2namespaceresource(url, matchedRoute string) (namespace string, resource string, resourceName string) {
	if common.Base != "" {
		url = strings.TrimPrefix(url, common.Base)
		matchedRoute = strings.TrimPrefix(matchedRoute, common.Base)
	}
	// Split the URL into its components
	parts := strings.Split(url, "/")
	if len(parts) < 4 {
		return
	}
	resource = parts[3] // The resource type is always the third part
	if len(parts) > 4 {
		namespace = parts[4]
	} else {
		namespace = common.AllNamespaces // All namespaces
	}
	routeParts := strings.Split(matchedRoute, "/")
	if len(parts) > 5 && len(routeParts) > 5 && routeParts[5] == ":name" {
		resourceName = parts[5]
	}
	return
}
