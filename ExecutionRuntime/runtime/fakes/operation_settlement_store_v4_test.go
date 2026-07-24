package fakes

import "testing"

func TestOperationSettlementV4GuardKeyUsesTenantAndEffectPartition(t *testing.T) {
	left := operationSettlementGuardKeyV4("tenant-a", "shared-effect")
	samePartition := operationSettlementGuardKeyV4("tenant-a", "shared-effect")
	otherTenant := operationSettlementGuardKeyV4("tenant-b", "shared-effect")
	if left != samePartition {
		t.Fatal("same tenant/effect did not share one terminal guard key")
	}
	if left == otherTenant {
		t.Fatal("cross-tenant equal Effect IDs collided in the terminal guard key")
	}
}
