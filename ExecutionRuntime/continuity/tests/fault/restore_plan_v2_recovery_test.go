package fault_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

type faultRestoreClock func() time.Time

func (c faultRestoreClock) Now() time.Time { return c() }

func TestRestorePlanV2LostReplyOnlyInspectsOriginalIdentity(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_752_577_200, 0)
	backend := memory.NewWithClock(func() time.Time { return now })
	controller, err := domain.NewRestorePlanControllerV2(backend, faultRestoreClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	plan := testkit.RestorePlanV2(now)
	// Simulate a durable create whose reply was lost.
	if _, _, err := backend.CreateRestorePlanFactV2(ctx, plan); err != nil {
		t.Fatal(err)
	}
	inspected, err := controller.InspectRestorePlanV2(ctx, ports.InspectRestorePlanRequestV2{Ref: plan.Ref()})
	if err != nil || inspected.Ref() != plan.Ref() {
		t.Fatalf("create recovery = (%v,%v)", inspected.Ref(), err)
	}
	alternate := plan.Clone()
	alternate.PlanID = "restore-plan-v2-alternate"
	testkit.RefreshRestorePlanV2(&alternate)
	if _, _, err := controller.CreateRestorePlanV2(ctx, ports.CreateRestorePlanRequestV2{Candidate: alternate, ExpectAbsent: true}); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("lost reply allowed replacement Plan identity: %v", err)
	}

	now = now.Add(time.Second)
	next := plan.Clone()
	next.Revision = 2
	next.State = contract.RestorePlanCheckpointInspectedV2
	next.UpdatedUnixNano = now.UnixNano()
	testkit.RefreshRestorePlanV2(&next)
	// Simulate a durable CAS whose reply was lost.
	if _, _, err := backend.CompareAndSwapRestorePlanFactV2(ctx, plan.Ref(), next); err != nil {
		t.Fatal(err)
	}
	current, err := controller.InspectCurrentRestorePlanV2(ctx, ports.InspectCurrentRestorePlanRequestV2{
		TenantID: plan.Scope.TenantID, ScopeDigest: plan.Scope.ExecutionScopeDigest, PlanID: plan.PlanID, Owner: plan.Owner,
	})
	if err != nil || current.Ref() != next.Ref() {
		t.Fatalf("CAS recovery = (%v,%v)", current.Ref(), err)
	}
	if current.PlanID != plan.PlanID || current.ProposedInstance.InstanceID == current.SourceInstanceRef.ID {
		t.Fatal("recovery changed Plan identity or reused source Instance")
	}
}
