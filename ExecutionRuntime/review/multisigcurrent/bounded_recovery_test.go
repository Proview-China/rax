package multisigcurrent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestBoundedCurrentRecoveryV2CapsAtFiveSecondsAndKnownTTL(t *testing.T) {
	now := time.Now()
	ctx, cancel, ok := boundedCurrentRecoveryContextV2(context.Background(), now)
	if !ok {
		t.Fatal("bounded recovery rejected a valid clock")
	}
	defer cancel()
	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		t.Fatal("detached recovery has no deadline")
	}
	remaining := deadline.Sub(time.Now())
	if remaining <= 0 || remaining > maximumDetachedCurrentRecoveryV2 {
		t.Fatalf("detached recovery deadline=%s", remaining)
	}

	const ttl = 25 * time.Millisecond
	ttlCtx, ttlCancel, ok := boundedCurrentRecoveryContextV2(context.Background(), now, now.Add(ttl).UnixNano())
	if !ok {
		t.Fatal("bounded recovery rejected a live TTL")
	}
	defer ttlCancel()
	ttlDeadline, hasTTLDeadline := ttlCtx.Deadline()
	if !hasTTLDeadline || ttlDeadline.After(time.Now().Add(ttl)) {
		t.Fatalf("detached recovery escaped the known TTL: %s", ttlDeadline)
	}
}

func TestBoundedCurrentRecoveryV2RejectsZeroRollbackAndExpiredTTL(t *testing.T) {
	now := time.Now()
	for name, test := range map[string]struct {
		clock    time.Time
		expiries []int64
	}{
		"zero-clock":  {},
		"expired-ttl": {clock: now, expiries: []int64{now.Add(-time.Nanosecond).UnixNano()}},
	} {
		t.Run(name, func(t *testing.T) {
			if ctx, cancel, ok := boundedCurrentRecoveryContextV2(context.Background(), test.clock, test.expiries...); ok {
				cancel()
				t.Fatalf("invalid recovery accepted: %v", ctx)
			}
		})
	}
}

func TestExactReadV2MapsContextAndPreservesOriginalUnknown(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	var calls atomic.Int64
	_, recovered, err := exactReadV2(context.Background(), clock, func(context.Context) (string, error) {
		if calls.Add(1) == 1 {
			return "", context.Canceled
		}
		return "", core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "recovery unavailable")
	})
	if !recovered || calls.Load() != 2 || !core.HasCategory(err, core.ErrorIndeterminate) || !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("raw cancellation was not preserved as original Unknown: recovered=%v calls=%d err=%v", recovered, calls.Load(), err)
	}
}

func TestExactReadV2RejectsRollbackBeforeDetachedRetry(t *testing.T) {
	base := time.Now()
	times := []time.Time{base, base, base.Add(2 * time.Second), base.Add(time.Second)}
	var calls atomic.Int64
	clock := func() time.Time {
		index := int(calls.Add(1)) - 1
		if index >= len(times) {
			return times[len(times)-1]
		}
		return times[index]
	}
	_, recovered, err := exactReadV2(context.Background(), clock, func(context.Context) (string, error) {
		return "", core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost reply")
	})
	if !recovered || !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("rollback before detached retry was accepted: recovered=%v err=%v", recovered, err)
	}
}

func TestExactReadV2RecoveryIsClippedByKnownTTL(t *testing.T) {
	now := time.Now()
	var calls atomic.Int64
	original := core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost reply")
	started := time.Now()
	_, recovered, err := exactReadV2(context.Background(), func() time.Time { return now }, func(ctx context.Context) (string, error) {
		if calls.Add(1) == 1 {
			return "", original
		}
		<-ctx.Done()
		return "", ctx.Err()
	}, now.Add(20*time.Millisecond).UnixNano())
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("detached retry exceeded known TTL: %s", elapsed)
	}
	if !recovered || !core.HasCategory(err, core.ErrorIndeterminate) || !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("TTL-clipped retry did not preserve original Unknown: recovered=%v err=%v", recovered, err)
	}
}
