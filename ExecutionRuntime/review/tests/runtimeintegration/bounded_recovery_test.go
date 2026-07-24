package runtimeintegration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestFaultOperationReviewCurrentReaderV4BoundedRecoveryPreservesOriginalUnknown(t *testing.T) {
	t.Run("blocking_inspect_uses_snapshot_ttl", func(t *testing.T) {
		fixture := newFixtureV4(t, "accepted", false)
		logicalNow := fixture.now
		snapshot := cloneTestSnapshot(fixture.snapshot)
		snapshot.ExpiresUnixNano = logicalNow.Add(20 * time.Millisecond).UnixNano()
		original := core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "original V4 Inspect outcome is unknown")
		source := &boundedRecoverySourceV4{snapshot: snapshot, original: original, blockRecovery: true}
		reader := mustReaderV4(t, source, func() time.Time { return logicalNow })
		started := time.Now()
		_, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent)
		if elapsed := time.Since(started); elapsed >= time.Second {
			t.Fatalf("V4 recovery exceeded snapshot TTL bound: %v", elapsed)
		}
		if err != original || source.count() != 2 {
			t.Fatalf("V4 blocking recovery replaced original Unknown or retried more than once: calls=%d err=%v", source.count(), err)
		}
	})

	t.Run("not_found_preserves_original_unknown", func(t *testing.T) {
		fixture := newFixtureV4(t, "accepted", false)
		original := core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "original V4 Inspect outcome is unknown")
		source := &boundedRecoverySourceV4{snapshot: fixture.snapshot, original: original, recoveryErr: core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "V4 exact snapshot is not found")}
		reader := mustReaderV4(t, source, func() time.Time { return fixture.now })
		_, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent)
		if err != original || source.count() != 2 {
			t.Fatalf("V4 NotFound replaced original Unknown: calls=%d err=%v", source.count(), err)
		}
	})

	t.Run("expired_snapshot_skips_retry", func(t *testing.T) {
		fixture := newFixtureV4(t, "accepted", false)
		snapshot := cloneTestSnapshot(fixture.snapshot)
		snapshot.ExpiresUnixNano = fixture.now.UnixNano()
		original := core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "original V4 Inspect outcome is unknown")
		source := &boundedRecoverySourceV4{snapshot: snapshot, original: original}
		reader := mustReaderV4(t, source, func() time.Time { return fixture.now })
		_, err := reader.InspectOperationReviewCurrentV4(context.Background(), fixture.intent)
		if err != original || source.count() != 1 {
			t.Fatalf("V4 expired snapshot started recovery: calls=%d err=%v", source.count(), err)
		}
	})
}

func TestFaultOperationReviewCurrentReaderV5BoundedRecoveryPreservesOriginalUnknown(t *testing.T) {
	t.Run("blocking_inspect_uses_snapshot_ttl", func(t *testing.T) {
		fixture := newQuorumFixtureV5(t)
		logicalNow := fixture.now.Add(6 * time.Second)
		snapshot := fixture.snapshot
		snapshot.ExpiresUnixNano = logicalNow.Add(20 * time.Millisecond).UnixNano()
		original := core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "original V5 Inspect outcome is unknown")
		source := &boundedRecoverySourceV5{snapshot: snapshot, original: original, blockRecovery: true}
		reader, err := runtimeadapter.NewReaderV5(source, func() time.Time { return logicalNow })
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		_, err = reader.InspectOperationReviewCurrentV5(context.Background(), fixture.request)
		if elapsed := time.Since(started); elapsed >= time.Second {
			t.Fatalf("V5 recovery exceeded snapshot TTL bound: %v", elapsed)
		}
		if err != original || source.count() != 2 {
			t.Fatalf("V5 blocking recovery replaced original Unknown or retried more than once: calls=%d err=%v", source.count(), err)
		}
	})

	t.Run("not_found_preserves_original_unknown", func(t *testing.T) {
		fixture := newQuorumFixtureV5(t)
		original := core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "original V5 Inspect outcome is unknown")
		source := &boundedRecoverySourceV5{snapshot: fixture.snapshot, original: original, recoveryErr: core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "V5 exact snapshot is not found")}
		reader, err := runtimeadapter.NewReaderV5(source, func() time.Time { return fixture.now.Add(6 * time.Second) })
		if err != nil {
			t.Fatal(err)
		}
		_, err = reader.InspectOperationReviewCurrentV5(context.Background(), fixture.request)
		if err != original || source.count() != 2 {
			t.Fatalf("V5 NotFound replaced original Unknown: calls=%d err=%v", source.count(), err)
		}
	})

	t.Run("expired_snapshot_skips_retry", func(t *testing.T) {
		fixture := newQuorumFixtureV5(t)
		logicalNow := fixture.now.Add(6 * time.Second)
		snapshot := fixture.snapshot
		snapshot.ExpiresUnixNano = logicalNow.UnixNano()
		original := core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "original V5 Inspect outcome is unknown")
		source := &boundedRecoverySourceV5{snapshot: snapshot, original: original}
		reader, err := runtimeadapter.NewReaderV5(source, func() time.Time { return logicalNow })
		if err != nil {
			t.Fatal(err)
		}
		_, err = reader.InspectOperationReviewCurrentV5(context.Background(), fixture.request)
		if err != original || source.count() != 1 {
			t.Fatalf("V5 expired snapshot started recovery: calls=%d err=%v", source.count(), err)
		}
	})
}

type boundedRecoverySourceV4 struct {
	mu            sync.Mutex
	snapshot      runtimeadapter.CurrentFactSnapshotV4
	original      error
	recoveryErr   error
	blockRecovery bool
	calls         int
}

func (s *boundedRecoverySourceV4) InspectReviewCurrentFactsV4(ctx context.Context, _ runtimeadapter.ExactCurrentRequestV4) (runtimeadapter.CurrentFactSnapshotV4, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	if call == 1 {
		return cloneTestSnapshot(s.snapshot), s.original
	}
	if s.blockRecovery {
		<-ctx.Done()
		return runtimeadapter.CurrentFactSnapshotV4{}, ctx.Err()
	}
	if s.recoveryErr != nil {
		return runtimeadapter.CurrentFactSnapshotV4{}, s.recoveryErr
	}
	return cloneTestSnapshot(s.snapshot), nil
}

func (s *boundedRecoverySourceV4) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type boundedRecoverySourceV5 struct {
	mu            sync.Mutex
	snapshot      runtimeadapter.CurrentFactSnapshotV5
	original      error
	recoveryErr   error
	blockRecovery bool
	calls         int
}

func (s *boundedRecoverySourceV5) InspectReviewCurrentFactsV5(ctx context.Context, _ runtimeadapter.ExactCurrentRequestV5) (runtimeadapter.CurrentFactSnapshotV5, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	if call == 1 {
		return s.snapshot, s.original
	}
	if s.blockRecovery {
		<-ctx.Done()
		return runtimeadapter.CurrentFactSnapshotV5{}, ctx.Err()
	}
	if s.recoveryErr != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, s.recoveryErr
	}
	return s.snapshot, nil
}

func (s *boundedRecoverySourceV5) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}
