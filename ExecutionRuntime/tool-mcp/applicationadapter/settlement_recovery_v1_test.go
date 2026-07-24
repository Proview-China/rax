package applicationadapter

import (
	"context"
	"testing"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

type cancellationHonoringSettlementInspectorV1 struct {
	want runtimeports.InspectCurrentOperationSettlementRequestV4
	seen bool
}

func (s *cancellationHonoringSettlementInspectorV1) InspectCurrentOperationSettlementV4(ctx context.Context, got runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error) {
	if err := ctx.Err(); err != nil {
		return runtimeports.OperationInspectionSettlementRefV4{}, err
	}
	if _, ok := ctx.Deadline(); !ok {
		panic("lost-reply recovery Inspect is not bounded")
	}
	if !runtimeports.SameOperationSubjectV3(got.Operation, s.want.Operation) || got.EffectID != s.want.EffectID {
		panic("lost-reply recovery changed the exact settlement key")
	}
	s.seen = true
	return runtimeports.OperationInspectionSettlementRefV4{}, nil
}

func TestSettlementLostReplyRecoveryIgnoresCallerCancellationOnlyForBoundedExactInspect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fixture := testkit.BoundaryFixture(testkit.FixedTime)
	want := runtimeports.InspectCurrentOperationSettlementRequestV4{Operation: fixture.Operation, EffectID: fixture.Attempt.EffectID}
	inspector := &cancellationHonoringSettlementInspectorV1{want: want}
	if _, err := inspectSettlementAfterLostReplyV1(ctx, inspector, want, time.Second); err != nil {
		t.Fatalf("bounded exact Inspect inherited caller cancellation: %v", err)
	}
	if !inspector.seen {
		t.Fatal("bounded exact Inspect was not called")
	}
}
