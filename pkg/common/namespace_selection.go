package common

import "strings"

// ParseRequestedNamespaces returns a deduplicated namespace selection and
// whether the request should be restricted to it. Empty input, empty values,
// and _all retain the normal all-namespaces behavior.
func ParseRequestedNamespaces(raw string) ([]string, bool) {
	seen := make(map[string]struct{})
	namespaces := make([]string, 0)
	for _, value := range strings.Split(raw, ",") {
		namespace := strings.TrimSpace(value)
		if namespace == "" {
			continue
		}
		if namespace == AllNamespaces {
			return nil, false
		}
		if _, exists := seen[namespace]; exists {
			continue
		}
		seen[namespace] = struct{}{}
		namespaces = append(namespaces, namespace)
	}
	return namespaces, len(namespaces) > 0
}
