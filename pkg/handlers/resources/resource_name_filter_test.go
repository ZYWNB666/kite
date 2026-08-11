package resources

import (
	"testing"

	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
)

func TestCanReadResourceObjectHonorsResourceNames(t *testing.T) {
	user := model.User{Roles: []common.Role{{
		Name:          "route-adjust",
		Clusters:      []string{"cluster-a"},
		Namespaces:    []string{"envoy-gateway-system"},
		Resources:     []string{"configmaps"},
		ResourceNames: []string{"gateway.prod"},
		Verbs:         []string{"get", "update"},
	}}}

	if !canReadResourceObject(user, "configmaps", "cluster-a", "envoy-gateway-system", "gateway.prod") {
		t.Fatal("expected selected ConfigMap to be readable")
	}
	if canReadResourceObject(user, "configmaps", "cluster-a", "envoy-gateway-system", "gatewayXprod") {
		t.Fatal("unexpected access to an unselected ConfigMap")
	}
}
