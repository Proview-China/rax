package decisioncurrent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestBoundedDetachedExactReadPreservesOriginalUnknownAndTTL(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	recovery, cancel, ok := boundedDetachedRecoveryV1(context.Background(), now, now.Add(20*time.Millisecond).UnixNano())
	if !ok {
		t.Fatal("bounded recovery was not constructed")
	}
	defer cancel()
	original := core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "original lost reply")
	calls := 0
	started := time.Now()
	_, recovered, err := retryExactReadV1(context.Background(), recovery, func() time.Time { return now }, func(ctx context.Context) (string, error) {
		calls++
		if calls == 1 {
			return "", original
		}
		<-ctx.Done()
		return "", core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "blocked retry")
	})
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("detached exact retry exceeded subject TTL: %s", elapsed)
	}
	if !recovered || !errors.Is(err, original) || calls != 2 {
		t.Fatalf("recovered=%v err=%v calls=%d", recovered, err, calls)
	}
}

func TestBoundedDetachedExactReadClockRollbackFailsClosed(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	recovery, cancel, ok := boundedDetachedRecoveryV1(context.Background(), now, now.Add(time.Second).UnixNano())
	if !ok {
		t.Fatal("bounded recovery was not constructed")
	}
	defer cancel()
	clockCalls := 0
	clock := func() time.Time {
		clockCalls++
		if clockCalls <= 3 {
			return now
		}
		return now.Add(-time.Nanosecond)
	}
	readCalls := 0
	_, _, err := retryExactReadV1(context.Background(), recovery, clock, func(context.Context) (string, error) {
		readCalls++
		if readCalls == 1 {
			return "", core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost reply")
		}
		return "value", nil
	})
	if !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback during recovery was not fail closed: %v", err)
	}
}

func TestDetachedRecoveryUsesShortestSnapshotTTL(t *testing.T) {
	now := time.Unix(2_400_300_000, 0)
	parent, cancel, ok := boundedDetachedRecoveryV1(context.Background(), now, now.Add(time.Second).UnixNano())
	if !ok {
		t.Fatal("recovery context was not constructed")
	}
	defer cancel()
	snapshot, snapshotCancel, ok := tightenDetachedRecoveryV1(parent, now, now.Add(20*time.Millisecond).UnixNano())
	if !ok {
		t.Fatal("snapshot recovery context was not tightened")
	}
	defer snapshotCancel()
	started := time.Now()
	<-snapshot.Done()
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("snapshot TTL did not shorten recovery: %s", elapsed)
	}
}
