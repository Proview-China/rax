package memory_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestTimelineProjectionPolicyRepositoryV1HistoryCurrentLostReplyAndScope(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0)
	currentTime := now
	backend := memory.NewWithClock(func() time.Time { return currentTime })
	first := memoryTimelinePolicyV1(t, "policy-a", "scope-a", 1, now, now.Add(time.Minute))
	created, duplicate, err := backend.CreateTimelineProjectionPolicyV1(ctx, first)
	if err != nil || duplicate || created.Ref != first.Ref {
		t.Fatalf("create failed: created=%+v duplicate=%v err=%v", created, duplicate, err)
	}
	// A repeated exact create models a lost create reply and must only Inspect.
	replayed, duplicate, err := backend.CreateTimelineProjectionPolicyV1(ctx, first)
	if err != nil || !duplicate || replayed.Ref != first.Ref {
		t.Fatalf("lost create reply did not converge: replayed=%+v duplicate=%v err=%v", replayed, duplicate, err)
	}
	second := memoryTimelinePolicyV1(t, "policy-a", "scope-a", 2, now.Add(time.Second), now.Add(2*time.Minute))
	if _, err = backend.CompareAndSwapTimelineProjectionPolicyV1(ctx, first.Ref, second); err != nil {
		t.Fatal(err)
	}
	currentTime = now.Add(time.Second)
	if historical, err := backend.InspectTimelineProjectionPolicyV1(ctx, first.Ref); err != nil || historical.Ref != first.Ref {
		t.Fatalf("history was overwritten: historical=%+v err=%v", historical, err)
	}
	if err = backend.ValidateTimelineProjectionPolicyCurrentV1(ctx, first); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("old exact ref must no longer be current: %v", err)
	}
	if err = backend.ValidateTimelineProjectionPolicyCurrentV1(ctx, second); err != nil {
		t.Fatal(err)
	}
	if _, err = backend.CompareAndSwapTimelineProjectionPolicyV1(ctx, first.Ref, second); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("progressed replay must not ABA: %v", err)
	}
	other := memoryTimelinePolicyV1(t, "policy-a", "scope-b", 1, now, now.Add(time.Minute))
	if _, _, err = backend.CreateTimelineProjectionPolicyV1(ctx, other); err != nil {
		t.Fatal(err)
	}
	if got, err := backend.InspectTimelineProjectionPolicyCurrentV1(ctx, "policy-a", "scope-b"); err != nil || got.Ref != other.Ref {
		t.Fatalf("same ID in another scope crossed current index: got=%+v err=%v", got, err)
	}
}

func TestTimelineProjectionPolicyRepositoryV1ConcurrentCASOneWinner(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0)
	backend := memory.NewWithClock(func() time.Time { return now })
	first := memoryTimelinePolicyV1(t, "policy-race", "scope-a", 1, now, now.Add(time.Minute))
	if _, _, err := backend.CreateTimelineProjectionPolicyV1(ctx, first); err != nil {
		t.Fatal(err)
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			next := memoryTimelinePolicyV1(t, "policy-race", "scope-a", 2, now.Add(time.Second), now.Add(time.Duration(120+i)*time.Second))
			if _, err := backend.CompareAndSwapTimelineProjectionPolicyV1(ctx, first.Ref, next); err == nil {
				winners.Add(1)
			} else if !contract.HasCode(err, contract.ErrRevisionConflict) {
				t.Errorf("unexpected CAS error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("expected one CAS winner, got %d", winners.Load())
	}
}

func TestTimelineProjectionPolicyRepositoryV1ExpiryBoundary(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	current := now
	backend := memory.NewWithClock(func() time.Time { return current })
	policy := memoryTimelinePolicyV1(t, "policy-expiry", "scope-a", 1, now, now.Add(time.Second))
	if _, _, err := backend.CreateTimelineProjectionPolicyV1(context.Background(), policy); err != nil {
		t.Fatal(err)
	}
	current = now.Add(time.Second)
	if err := backend.ValidateTimelineProjectionPolicyCurrentV1(context.Background(), policy); !contract.HasCode(err, contract.ErrPreconditionFailed) {
		t.Fatalf("exact expiry must fail closed: %v", err)
	}
}

func memoryTimelinePolicyV1(t *testing.T, id, scope string, revision uint64, checked, expires time.Time) contract.TimelineProjectionPolicyCurrentV1 {
	t.Helper()
	value, err := contract.SealTimelineProjectionPolicyCurrentV1(contract.TimelineProjectionPolicyCurrentV1{Ref: contract.TimelineProjectionPolicyRefV1{PolicyID: id, Revision: revision, ScopeDigest: scope}, State: contract.TimelineProjectionPolicyActiveV1, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
