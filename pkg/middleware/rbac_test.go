package middleware

import (
	"testing"
)

func TestUrl2NamespaceResource(t *testing.T) {
	testCases := []struct {
		name             string
		url              string
		wantNamespace    string
		wantResource     string
		wantResourceName string
	}{
		{
			name:             "valid URL with namespace and resource",
			url:              "/api/v1/pods/default/pods",
			wantNamespace:    "default",
			wantResource:     "pods",
			wantResourceName: "pods",
		},
		{
			name:             "valid URL with all namespace and specific resource",
			url:              "/api/v1/pvs/_all/some-pv",
			wantNamespace:    "_all",
			wantResource:     "pvs",
			wantResourceName: "some-pv",
		},
		{
			name:             "valid URL with namespace only",
			url:              "/api/v1/pods/default",
			wantNamespace:    "default",
			wantResource:     "pods",
			wantResourceName: "",
		},
		{
			name:             "invalid URL - too short (3 parts)",
			url:              "/api/v1",
			wantNamespace:    "",
			wantResource:     "",
			wantResourceName: "",
		},
		{
			name:             "invalid URL - missing namespace",
			url:              "/api/v1/pods",
			wantNamespace:    "_all",
			wantResource:     "pods",
			wantResourceName: "",
		},
		{
			name:             "URL with resource name",
			url:              "/api/v1/pods/default/my-pod",
			wantNamespace:    "default",
			wantResource:     "pods",
			wantResourceName: "my-pod",
		},
		{
			name:             "URL with sub-resource (history) — resource name is still extracted",
			url:              "/api/v1/pods/default/some-pods/history",
			wantNamespace:    "default",
			wantResource:     "pods",
			wantResourceName: "some-pods",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotNamespace, gotResource, gotResourceName := url2namespaceresource(tc.url)
			if gotNamespace != tc.wantNamespace || gotResource != tc.wantResource || gotResourceName != tc.wantResourceName {
				t.Errorf("url2namespaceresource(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.url, gotNamespace, gotResource, gotResourceName,
					tc.wantNamespace, tc.wantResource, tc.wantResourceName)
			}
		})
	}
}
