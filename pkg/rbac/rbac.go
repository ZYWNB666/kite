package rbac

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"k8s.io/klog/v2"
)

// CanAccess checks if user/oidcGroup can access resource with verb in cluster/namespace.
// resourceName is the specific resource name (e.g. a pod name). Pass an empty string
// for list/create operations where no specific name is targeted.
func CanAccess(user model.User, resource, verb, cluster, namespace string, resourceName ...string) bool {
	name := ""
	if len(resourceName) > 0 {
		name = resourceName[0]
	}
	roles := GetUserRoles(user)
	for _, role := range roles {
		if match(role.Clusters, cluster) &&
			match(role.Namespaces, namespace) &&
			match(role.Resources, resource) &&
			match(role.Verbs, verb) &&
			matchResourceName(role.ResourceNames, name) {
			klog.V(1).Infof("RBAC Check - User: %s, OIDC Groups: %v, Resource: %s/%s, Verb: %s, Cluster: %s, Namespace: %s, Hit Role: %v",
				user.Key(), user.OIDCGroups, resource, name, verb, cluster, namespace, role.Name)
			return true
		}
	}
	klog.V(1).Infof("RBAC Check - User: %s, OIDC Groups: %v, Resource: %s/%s, Verb: %s, Cluster: %s, Namespace: %s, No Access",
		user.Key(), user.OIDCGroups, resource, name, verb, cluster, namespace)
	return false
}

// CanProxy checks if user is allowed to use kite-proxy for the given cluster/namespace.
func CanProxy(user model.User, cluster, namespace string) bool {
	roles := GetUserRoles(user)
	for _, role := range roles {
		if !role.AllowProxy {
			continue
		}
		if !match(role.Clusters, cluster) {
			continue
		}
		// Determine which namespaces are allowed for proxy.
		// If ProxyNamespaces is empty, fall back to the role's Namespaces list.
		proxyNS := role.ProxyNamespaces
		if len(proxyNS) == 0 {
			proxyNS = role.Namespaces
		}
		if match(proxyNS, namespace) {
			klog.V(1).Infof("Proxy Check - User: %s, Cluster: %s, Namespace: %s, Hit Role: %v",
				user.Key(), cluster, namespace, role.Name)
			return true
		}
	}
	klog.V(1).Infof("Proxy Check - User: %s, Cluster: %s, Namespace: %s, No Access",
		user.Key(), cluster, namespace)
	return false
}

// CanAccessCluster reports whether the user has any role granting access to the cluster.
func CanAccessCluster(user model.User, name string) bool {
	roles := GetUserRoles(user)
	for _, role := range roles {
		if match(role.Clusters, name) {
			return true
		}
	}
	return false
}

func CanAccessNamespace(user model.User, cluster, name string) bool {
	roles := GetUserRoles(user)
	for _, role := range roles {
		if match(role.Clusters, cluster) && match(role.Namespaces, name) {
			return true
		}
	}
	return false
}

// GetUserRoles returns all roles for a user/oidcGroups
func GetUserRoles(user model.User) []common.Role {
	// Only return cached roles if they exist and are non-empty
	if user.Roles != nil && len(user.Roles) > 0 {
		return user.Roles
	}
	rolesMap := make(map[string]common.Role)

	// Load roles from database (role_assignments table)
	dbRoles, err := model.GetUserRolesFromDB(user.Username)
	if err != nil {
		klog.Warningf("Failed to load database roles for user %s: %v", user.Username, err)
	} else {
		for _, role := range dbRoles {
			rolesMap[role.Name] = role
		}
	}

	// Load roles from RBAC config file
	rwlock.RLock()
	defer rwlock.RUnlock()
	for _, mapping := range RBACConfig.RoleMapping {
		if contains(mapping.Users, "*") || contains(mapping.Users, user.Key()) {
			if r := findRole(mapping.Name); r != nil {
				rolesMap[r.Name] = *r
			}
		}
		for _, group := range user.OIDCGroups {
			if contains(mapping.OIDCGroups, group) {
				if r := findRole(mapping.Name); r != nil {
					rolesMap[r.Name] = *r
				}
			}
		}
	}
	roles := make([]common.Role, 0, len(rolesMap))
	for _, role := range rolesMap {
		roles = append(roles, role)
	}
	return roles
}

func findRole(name string) *common.Role {
	rwlock.RLock()
	defer rwlock.RUnlock()
	for _, r := range RBACConfig.Roles {
		if r.Name == name {
			return &r
		}
	}
	return nil
}

func match(list []string, val string) bool {
	for _, v := range list {
		if len(v) > 1 && strings.HasPrefix(v, "!") {
			if v[1:] == val {
				return false
			}
		}
		if v == "*" || v == val {
			return true
		}

		re, err := regexp.Compile("^(?:" + v + ")$")
		if err != nil {
			klog.Error(err)
			continue
		}
		if re.MatchString(val) {
			return true
		}
	}
	return false
}

// matchResourceName checks whether resourceName is allowed by the role's ResourceNames list.
// An empty list or a list containing "*" means all names are allowed.
// When resourceName itself is empty (e.g. list operations), we always allow.
func matchResourceName(resourceNames []string, resourceName string) bool {
	if resourceName == "" || len(resourceNames) == 0 {
		return true
	}
	return match(resourceNames, resourceName)
}

func contains(list []string, val string) bool {
	return slices.Contains(list, val)
}

func NoAccess(user, verb, resource, ns, cluster string) string {
	if ns == "" {
		return fmt.Sprintf("user %s does not have permission to %s %s on cluster %s",
			user, verb, resource, cluster)
	}
	if ns == common.AllNamespaces {
		ns = "All"
	}
	return fmt.Sprintf("user %s does not have permission to %s %s in namespace %s on cluster %s",
		user, verb, resource, ns, cluster)
}

func UserHasRole(user model.User, roleName string) bool {
	roles := GetUserRoles(user)
	for _, role := range roles {
		if role.Name == roleName {
			return true
		}
	}
	return false
}
