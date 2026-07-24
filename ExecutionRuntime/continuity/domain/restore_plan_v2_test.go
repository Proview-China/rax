package domain_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

type restorePlanClock func() time.Time

func (c restorePlanClock) Now() time.Time { return c() }

func TestRestorePlanControllerV2CreateCASHistoryAndLostReplyInspect(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_752_577_200, 0)
	backend := memory.NewWithClock(func() time.Time { return now })
	controller, err := domain.NewRestorePlanControllerV2(backend, restorePlanClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	initial := testkit.RestorePlanV2(now)
	created, replay, err := controller.CreateRestorePlanV2(ctx, ports.CreateRestorePlanRequestV2{Candidate: initial, ExpectAbsent: true})
	if err != nil || replay {
		t.Fatalf("create = (%v,%v,%v)", created.Ref(), replay, err)
	}
	_, replay, err = controller.CreateRestorePlanV2(ctx, ports.CreateRestorePlanRequestV2{Candidate: initial, ExpectAbsent: true})
	if err != nil || !replay {
		t.Fatalf("create replay = (%v,%v)", replay, err)
	}

	now = now.Add(time.Second)
	next, replay, err := controller.CompareAndSwapRestorePlanV2(ctx, ports.CompareAndSwapRestorePlanRequestV2{Expected: created.Ref(), NextState: contract.RestorePlanCheckpointInspectedV2})
	if err != nil || replay || next.Revision != 2 {
		t.Fatalf("CAS = (%v,%v,%v)", next.Ref(), replay, err)
	}
	// A progressed lost reply is recovered by the original exact CAS/Inspect;
	// it never creates a replacement Plan identity.
	recovered, replay, err := controller.CompareAndSwapRestorePlanV2(ctx, ports.CompareAndSwapRestorePlanRequestV2{Expected: created.Ref(), NextState: contract.RestorePlanCheckpointInspectedV2})
	if err != nil || !replay || recovered.Ref() != next.Ref() {
		t.Fatalf("CAS recovery = (%v,%v,%v)", recovered.Ref(), replay, err)
	}
	historical, err := controller.InspectRestorePlanV2(ctx, ports.InspectRestorePlanRequestV2{Ref: created.Ref()})
	if err != nil || historical.State != contract.RestorePlanDraftV2 {
		t.Fatalf("historical = (%s,%v)", historical.State, err)
	}
	current, err := controller.InspectCurrentRestorePlanV2(ctx, currentRestorePlanRequest(initial))
	if err != nil || current.Ref() != next.Ref() {
		t.Fatalf("current = (%v,%v)", current.Ref(), err)
	}
}

func TestRestorePlanControllerV2ConcurrentCASHasOneBranchWinner(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_752_577_200, 0)
	backend := memory.NewWithClock(func() time.Time { return now })
	controller, err := domain.NewRestorePlanControllerV2(backend, restorePlanClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	plan := testkit.RestorePlanV2(now)
	current, _, err := controller.CreateRestorePlanV2(ctx, ports.CreateRestorePlanRequestV2{Candidate: plan, ExpectAbsent: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []contract.RestorePlanStateV2{contract.RestorePlanCheckpointInspectedV2, contract.RestorePlanCompatibilityInspectedV2} {
		now = now.Add(time.Second)
		current, _, err = controller.CompareAndSwapRestorePlanV2(ctx, ports.CompareAndSwapRestorePlanRequestV2{Expected: current.Ref(), NextState: state})
		if err != nil {
			t.Fatal(err)
		}
	}
	now = now.Add(time.Second)
	var success atomic.Int32
	var conflicts atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		state := contract.RestorePlanAdmittedV2
		if i%2 == 1 {
			state = contract.RestorePlanRejectedV2
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, replay, err := controller.CompareAndSwapRestorePlanV2(ctx, ports.CompareAndSwapRestorePlanRequestV2{Expected: current.Ref(), NextState: state})
			switch {
			case err == nil && !replay:
				success.Add(1)
			case err == nil && replay:
				// Same branch retries are exact idempotent replays.
			case contract.HasCode(err, contract.ErrRevisionConflict):
				conflicts.Add(1)
			default:
				t.Errorf("CAS error = %v", err)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 1 || conflicts.Load() == 0 {
		t.Fatalf("success=%d conflicts=%d, want one linearization winner and opposite-branch conflicts", success.Load(), conflicts.Load())
	}
}

func TestRestorePlanControllerV2ExpiryAndTypedNilFailClosed(t *testing.T) {
	var typedNil *memory.Backend
	if _, err := domain.NewRestorePlanControllerV2(typedNil, restorePlanClock(time.Now)); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("typed-nil constructor error = %v", err)
	}

	ctx := context.Background()
	now := time.Unix(1_752_577_200, 0)
	backend := memory.NewWithClock(func() time.Time { return now })
	controller, err := domain.NewRestorePlanControllerV2(backend, restorePlanClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	plan := testkit.RestorePlanV2(now)
	created, _, err := controller.CreateRestorePlanV2(ctx, ports.CreateRestorePlanRequestV2{Candidate: plan, ExpectAbsent: true})
	if err != nil {
		t.Fatal(err)
	}
	now = time.Unix(0, plan.ExpiresUnixNano)
	if _, err := controller.InspectCurrentRestorePlanV2(ctx, currentRestorePlanRequest(plan)); !contract.HasCode(err, contract.ErrRestoreIncompatible) {
		t.Fatalf("expired current error = %v", err)
	}
	expired, replay, err := controller.CompareAndSwapRestorePlanV2(ctx, ports.CompareAndSwapRestorePlanRequestV2{Expected: created.Ref(), NextState: contract.RestorePlanExpiredV2})
	if err != nil || replay || expired.State != contract.RestorePlanExpiredV2 {
		t.Fatalf("expire = (%s,%v,%v)", expired.State, replay, err)
	}
	if _, err := controller.InspectRestorePlanV2(ctx, ports.InspectRestorePlanRequestV2{Ref: created.Ref()}); err != nil {
		t.Fatalf("expired Plan history is no longer inspectable: %v", err)
	}
}

func currentRestorePlanRequest(plan contract.RestorePlanFactV2) ports.InspectCurrentRestorePlanRequestV2 {
	return ports.InspectCurrentRestorePlanRequestV2{
		TenantID: plan.Scope.TenantID, ScopeDigest: plan.Scope.ExecutionScopeDigest,
		PlanID: plan.PlanID, Owner: plan.Owner,
	}
}
