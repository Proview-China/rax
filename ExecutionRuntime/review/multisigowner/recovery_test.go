package multisigowner

import (
	"context"
	"testing"
	"time"
)

func TestBoundedRecoveryContextV2DetachesCancellationButStopsAtSubjectTTL(t *testing.T) {
	baseline := time.Unix(1_900_000_000, 0)
	now := baseline
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	recovery, cancel, ok := boundedRecoveryContextV2(parent, func() time.Time { return now }, baseline, baseline.Add(25*time.Millisecond).UnixNano())
	if !ok {
		t.Fatal("valid short recovery window rejected")
	}
	defer cancel()
	if err := recovery.Err(); err != nil {
		t.Fatalf("caller cancellation leaked into detached recovery: %v", err)
	}
	started := time.Now()
	<-recovery.Done()
	elapsed := time.Since(started)
	if elapsed > 250*time.Millisecond {
		t.Fatalf("recovery exceeded the subject TTL bound: %v", elapsed)
	}
}

func TestBoundedRecoveryContextV2RejectsTTLAndClockRollback(t *testing.T) {
	baseline := time.Unix(1_900_000_000, 0)
	tests := []struct {
		name   string
		now    time.Time
		expiry int64
	}{
		{name: "expired", now: baseline.Add(time.Second), expiry: baseline.Add(time.Second).UnixNano()},
		{name: "rollback", now: baseline.Add(-time.Nanosecond), expiry: baseline.Add(time.Minute).UnixNano()},
		{name: "zero-clock", now: time.Time{}, expiry: baseline.Add(time.Minute).UnixNano()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if recovery, cancel, ok := boundedRecoveryContextV2(context.Background(), func() time.Time { return test.now }, baseline, test.expiry); ok {
				cancel()
				t.Fatalf("invalid recovery window accepted: %#v", recovery)
			}
		})
	}
}

func TestRecoveryStillCurrentV2DetectsTTLCrossingAndRollback(t *testing.T) {
	baseline := time.Unix(1_900_000_000, 0)
	now := baseline.Add(time.Millisecond)
	clock := func() time.Time { return now }
	expiry := baseline.Add(2 * time.Millisecond).UnixNano()
	if !recoveryStillCurrentV2(clock, baseline, expiry) {
		t.Fatal("valid recovery point rejected")
	}
	now = baseline.Add(2 * time.Millisecond)
	if recoveryStillCurrentV2(clock, baseline, expiry) {
		t.Fatal("TTL crossing accepted")
	}
	now = baseline.Add(-time.Nanosecond)
	if recoveryStillCurrentV2(clock, baseline, baseline.Add(time.Minute).UnixNano()) {
		t.Fatal("clock rollback accepted")
	}
}
