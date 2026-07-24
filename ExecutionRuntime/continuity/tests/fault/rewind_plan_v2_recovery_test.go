package fault_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

type rewindFaultClock func() time.Time

func (c rewindFaultClock) Now() time.Time { return c() }

type rewindLostReplyRepositoryV2 struct {
	delegate   ports.RewindPlanRepositoryV2
	loseCreate atomic.Bool
	loseCAS    atomic.Bool
	creates    atomic.Int64
	cas        atomic.Int64
}

func (r *rewindLostReplyRepositoryV2) CreateRewindPlanFactV2(ctx context.Context, plan contract.RewindPlanFactV2) (contract.RewindPlanFactV2, bool, error) {
	r.creates.Add(1)
	created, replay, err := r.delegate.CreateRewindPlanFactV2(ctx, plan)
	if err == nil && r.loseCreate.CompareAndSwap(true, false) {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrUnavailable, "fault", "create reply lost after durable write")
	}
	return created, replay, err
}

func (r *rewindLostReplyRepositoryV2) CompareAndSwapRewindPlanFactV2(ctx context.Context, expected contract.RewindPlanRefV2, next contract.RewindPlanFactV2) (contract.RewindPlanFactV2, bool, error) {
	r.cas.Add(1)
	updated, replay, err := r.delegate.CompareAndSwapRewindPlanFactV2(ctx, expected, next)
	if err == nil && r.loseCAS.CompareAndSwap(true, false) {
		return contract.RewindPlanFactV2{}, false, contract.NewError(contract.ErrUnavailable, "fault", "CAS reply lost after durable write")
	}
	return updated, replay, err
}

func (r *rewindLostReplyRepositoryV2) InspectRewindPlanV2(ctx context.Context, request ports.InspectRewindPlanRequestV2) (contract.RewindPlanFactV2, error) {
	return r.delegate.InspectRewindPlanV2(ctx, request)
}

func (r *rewindLostReplyRepositoryV2) InspectCurrentRewindPlanV2(ctx context.Context, request ports.InspectCurrentRewindPlanRequestV2) (contract.RewindPlanFactV2, error) {
	return r.delegate.InspectCurrentRewindPlanV2(ctx, request)
}

func TestRewindPlanV2LostRepliesInspectExactWithoutRepeatingMutation(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_752_577_200, 0)
	backend := memory.NewWithClock(func() time.Time { return now })
	fault := &rewindLostReplyRepositoryV2{delegate: backend}
	fault.loseCreate.Store(true)
	controller, err := domain.NewRewindPlanControllerV2(fault, rewindFaultClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	created, replay, err := controller.CreateRewindPlanV2(ctx, ports.CreateRewindPlanRequestV2{Candidate: testkit.RewindPlanV2(now), ExpectAbsent: true})
	if err != nil || !replay || fault.creates.Load() != 1 {
		t.Fatalf("create recovery = (%v,%v,calls=%d)", replay, err, fault.creates.Load())
	}
	now = now.Add(time.Second)
	fault.loseCAS.Store(true)
	updated, replay, err := controller.CompareAndSwapRewindPlanV2(ctx, ports.CompareAndSwapRewindPlanRequestV2{Expected: created.Ref(), NextState: contract.RewindPlanWorkspaceInspectedV2})
	if err != nil || !replay || updated.Revision != 2 || fault.cas.Load() != 1 {
		t.Fatalf("CAS recovery = (%d,%v,%v,calls=%d)", updated.Revision, replay, err, fault.cas.Load())
	}
	current, err := controller.InspectCurrentRewindPlanV2(ctx, ports.InspectCurrentRewindPlanRequestV2{TenantID: updated.Scope.TenantID, ScopeDigest: updated.Scope.ExecutionScopeDigest, PlanID: updated.PlanID, Owner: updated.Owner})
	if err != nil || current.Ref() != updated.Ref() {
		t.Fatalf("current = (%v,%v)", current.Ref(), err)
	}
}

func TestRewindPlanV2LostReplyChangedSameIDConflicts(t *testing.T) {
	now := time.Unix(1_752_577_200, 0)
	backend := memory.NewWithClock(func() time.Time { return now })
	fault := &rewindLostReplyRepositoryV2{delegate: backend}
	fault.loseCreate.Store(true)
	controller, _ := domain.NewRewindPlanControllerV2(fault, rewindFaultClock(func() time.Time { return now }))
	plan := testkit.RewindPlanV2(now)
	if _, _, err := controller.CreateRewindPlanV2(context.Background(), ports.CreateRewindPlanRequestV2{Candidate: plan, ExpectAbsent: true}); err != nil {
		t.Fatal(err)
	}
	changed := plan.Clone()
	changed.DropChangeSetRefs[0] = testkit.ExactRefV2("workspace-change-drop-drift", "praxis/sandbox", "workspace_change_set_v1")
	testkit.RefreshRewindPlanV2(&changed)
	if _, _, err := controller.CreateRewindPlanV2(context.Background(), ports.CreateRewindPlanRequestV2{Candidate: changed, ExpectAbsent: true}); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("changed same-ID error = %v", err)
	}
}
