package ports_test

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type operationSettlementGovernanceCompileV3 struct{}

func (operationSettlementGovernanceCompileV3) SettleOperationEffectV3(context.Context, ports.OperationEffectIntentV3, ports.OperationSettlementSubmissionV3) (ports.OperationSettlementRefV3, error) {
	return ports.OperationSettlementRefV3{}, nil
}

func (operationSettlementGovernanceCompileV3) InspectOperationSettlementV3(context.Context, ports.OperationSubjectV3, core.EffectIntentID) (ports.OperationSettlementRefV3, error) {
	return ports.OperationSettlementRefV3{}, nil
}

func TestOperationSettlementGovernancePortV3EmbedsCurrentReaderWithoutChangingMethodSet(t *testing.T) {
	var governance ports.OperationSettlementGovernancePortV3 = operationSettlementGovernanceCompileV3{}
	var current ports.OperationSettlementCurrentReaderV3 = governance
	if current == nil {
		t.Fatal("Operation Settlement governance must expose the additive current reader")
	}
}
