package common

import "testing"

func TestRequestedResourceMetadata(t *testing.T) {
	tests := []struct {
		resource      ResourceType
		group         string
		clusterScoped bool
	}{
		{ResourceQuotas, "", false},
		{LimitRanges, "", false},
		{PodDisruptionBudgets, "policy", false},
		{PriorityClasses, "scheduling.k8s.io", true},
		{RuntimeClasses, "node.k8s.io", true},
		{Leases, "coordination.k8s.io", false},
		{MutatingWebhookConfigs, "admissionregistration.k8s.io", true},
		{ValidatingWebhookConfigs, "admissionregistration.k8s.io", true},
		{AdmissionPolicies, "admissionregistration.k8s.io", true},
		{AdmissionPolicyBindings, "admissionregistration.k8s.io", true},
		{EndpointSlices, "discovery.k8s.io", false},
		{Endpoints, "", false},
		{IngressClasses, "networking.k8s.io", true},
		{GatewayClasses, "gateway.networking.k8s.io", true},
	}

	for _, tt := range tests {
		t.Run(string(tt.resource), func(t *testing.T) {
			meta := LookupResource(string(tt.resource))
			if meta == nil {
				t.Fatalf("LookupResource(%q) returned nil", tt.resource)
			}
			if meta.Group != tt.group {
				t.Fatalf("Group = %q, want %q", meta.Group, tt.group)
			}
			if meta.ClusterScoped != tt.clusterScoped {
				t.Fatalf("ClusterScoped = %v, want %v", meta.ClusterScoped, tt.clusterScoped)
			}
		})
	}
}
