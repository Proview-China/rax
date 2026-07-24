package service

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type exactTraceReaderStubV2 struct {
	value contract.TraceFactV1
	err   error
}

func (s exactTraceReaderStubV2) InspectTraceExactV1(context.Context, core.TenantID, reviewport.ExactFactRefV1) (contract.TraceFactV1, error) {
	return s.value, s.err
}

func TestBoundedRecoveryContextV1DetachesCancellationAndStopsAtTTL(t *testing.T) {
	baseline := time.Unix(1_900_000_000, 0)
	now := baseline
	s := &Service{clock: func() time.Time { return now }}
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	recovery, cancel, ok := s.boundedRecoveryContextV1(parent, baseline, baseline.Add(25*time.Millisecond).UnixNano())
	if !ok {
		t.Fatal("valid short recovery window rejected")
	}
	defer cancel()
	if err := recovery.Err(); err != nil {
		t.Fatalf("caller cancellation leaked into detached recovery: %v", err)
	}
	started := time.Now()
	<-recovery.Done()
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("recovery exceeded subject TTL: %v", elapsed)
	}
}

func TestBoundedRecoveryContextV1RejectsTTLAndClockRollback(t *testing.T) {
	baseline := time.Unix(1_900_000_000, 0)
	tests := []struct {
		name string
		now  time.Time
	}{
		{name: "expired", now: baseline.Add(time.Second)},
		{name: "rollback", now: baseline.Add(-time.Nanosecond)},
		{name: "zero", now: time.Time{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := &Service{clock: func() time.Time { return test.now }}
			expiry := baseline.Add(time.Second).UnixNano()
			if recovery, cancel, ok := s.boundedRecoveryContextV1(context.Background(), baseline, expiry); ok {
				cancel()
				t.Fatalf("invalid recovery window accepted: %#v", recovery)
			}
		})
	}
}

func TestInspectTraceBatchExactV2RejectsNotFoundAndExactDrift(t *testing.T) {
	want := contract.TraceFactV1{
		FactIdentityV1: contract.FactIdentityV1{
			TenantID: "tenant-a",
			ID:       "trace-a",
			Revision: 1,
			Digest:   core.DigestBytes([]byte("trace-a")),
		},
	}
	notFound := core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "missing")
	if err := inspectTraceBatchExactV2(context.Background(), exactTraceReaderStubV2{err: notFound}, []contract.TraceFactV1{want}); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("NotFound was not preserved: %v", err)
	}
	drift := want
	drift.ID = "trace-b"
	if err := inspectTraceBatchExactV2(context.Background(), exactTraceReaderStubV2{value: drift}, []contract.TraceFactV1{want}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("exact drift was accepted: %v", err)
	}
}

func TestRecoveryStillCurrentV1DetectsCrossingAndRollback(t *testing.T) {
	baseline := time.Unix(1_900_000_000, 0)
	now := baseline.Add(time.Millisecond)
	s := &Service{clock: func() time.Time { return now }}
	expiry := baseline.Add(2 * time.Millisecond).UnixNano()
	if !s.recoveryStillCurrentV1(baseline, expiry) {
		t.Fatal("valid recovery point rejected")
	}
	now = baseline.Add(2 * time.Millisecond)
	if s.recoveryStillCurrentV1(baseline, expiry) {
		t.Fatal("TTL crossing accepted")
	}
	now = baseline.Add(-time.Nanosecond)
	if s.recoveryStillCurrentV1(baseline, baseline.Add(time.Minute).UnixNano()) {
		t.Fatal("clock rollback accepted")
	}
}
