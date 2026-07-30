package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-run-service/contract"
)

func digestV1(t *testing.T, value string) contract.DigestV1 {
	t.Helper()
	digest, err := contract.DigestJSONV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func refV1(t *testing.T, kind, id string, revision uint64) contract.ExactRefWireV1 {
	t.Helper()
	return contract.ExactRefWireV1{Kind: kind, ID: id, Revision: contract.NewWireUint64V1(revision), Digest: digestV1(t, id)}
}

func streamV1(t *testing.T) contract.ExactRefWireV1 {
	return refV1(t, contract.AgentRunEventStreamRefKindV1, "stream-1", 1)
}

func eventV1(t *testing.T, stream contract.ExactRefWireV1, sequence uint64, now time.Time) contract.AgentRunEventV1 {
	t.Helper()
	wireSequence := contract.NewWireUint64V1(sequence)
	id, err := contract.AgentRunEventIDV1(stream, wireSequence)
	if err != nil {
		t.Fatal(err)
	}
	event, err := contract.SealAgentRunEventV1(contract.AgentRunEventV1{
		StreamRef: stream, Sequence: wireSequence,
		EventRef:       contract.ExactRefWireV1{Kind: contract.AgentRunEventRefKindV1, ID: id, Revision: wireSequence},
		SourceOwnerRef: refV1(t, "praxis.runtime/owner", "runtime-owner-1", 1),
		SubjectRef:     refV1(t, "praxis.runtime/run", "run-1", 1),
		Kind:           "runtime.run_observed", PayloadVersion: "runtime-event-v1",
		PayloadDigest:    digestV1(t, "payload"),
		OccurredUnixNano: contract.NewWireUnixNanoV1(now.Add(-time.Second).UnixNano()),
		ObservedUnixNano: contract.NewWireUnixNanoV1(now.UnixNano()),
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func targetV1(t *testing.T) contract.AgentRunTargetV1 {
	t.Helper()
	target, err := contract.SealAgentRunTargetV1(contract.AgentRunTargetV1{
		HostID: "host-1", StartID: "start-1",
		HostStartClaim: refV1(t, contract.AgentRunHostStartClaimKindV1, "start-claim-1", 1),
		ExecutionScope: refV1(t, contract.AgentRunExecutionScopeKindV1, "scope-1", 1),
		RunCurrent:     refV1(t, contract.AgentRunCurrentKindV1, "run-1", 1),
		AuthorityEpoch: contract.NewWireUint64V1(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func watchRequestV1(t *testing.T, stream, retention contract.ExactRefWireV1, after uint64, now time.Time) contract.AgentRunWatchRequestV1 {
	t.Helper()
	var cursor *contract.AgentRunEventCursorV1
	if after > 0 {
		value, err := contract.SealAgentRunEventCursorV1(contract.AgentRunEventCursorV1{StreamRef: stream, Sequence: contract.NewWireUint64V1(after)})
		if err != nil {
			t.Fatal(err)
		}
		cursor = &value
	}
	request, err := contract.SealAgentRunWatchRequestV1(contract.AgentRunWatchRequestV1{
		RequestID: "watch-request-1", TraceID: "watch-trace-1",
		Target: targetV1(t), StreamRef: stream, RetentionCurrentRef: retention,
		AfterSequence: contract.NewWireUint64V1(after), Cursor: cursor, Limit: contract.NewWireUint64V1(100),
		RequestedUnixNano: contract.NewWireUnixNanoV1(now.UnixNano()),
		NotAfterUnixNano:  contract.NewWireUnixNanoV1(now.Add(time.Minute).UnixNano()),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestEventStreamResumeRestartAndRetentionGapV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	path := filepath.Join(t.TempDir(), "events.sqlite")
	store, err := OpenEventStreamV1(context.Background(), EventStreamConfigV1{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	stream := streamV1(t)
	retention1 := refV1(t, contract.AgentRunEventRetentionCurrentRefKindV1, "retention-1", 1)
	if err := store.AppendEventV1(context.Background(), eventV1(t, stream, 1, now), retention1); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEventV1(context.Background(), eventV1(t, stream, 2, now), retention1); err != nil {
		t.Fatal(err)
	}
	result, err := store.WatchAgentRunV1(context.Background(), watchRequestV1(t, stream, retention1, 0, now))
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != contract.AgentRunWatchEventsV1 || len(result.Events) != 2 {
		t.Fatalf("watch result=%+v", result)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := OpenEventStreamV1(context.Background(), EventStreamConfigV1{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	resumed, err := restarted.WatchAgentRunV1(context.Background(), watchRequestV1(t, stream, retention1, 1, now))
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Disposition != contract.AgentRunWatchEventsV1 || len(resumed.Events) != 1 || resumed.Events[0].Sequence != contract.NewWireUint64V1(2) {
		t.Fatalf("resumed result=%+v", resumed)
	}
	retention2 := refV1(t, contract.AgentRunEventRetentionCurrentRefKindV1, retention1.ID, 2)
	if err := restarted.AdvanceRetentionV1(context.Background(), stream, retention1, retention2, contract.NewWireUint64V1(2)); err != nil {
		t.Fatal(err)
	}
	gap, err := restarted.WatchAgentRunV1(context.Background(), watchRequestV1(t, stream, retention2, 0, now))
	if err != nil {
		t.Fatal(err)
	}
	if gap.Disposition != contract.AgentRunWatchResyncRequiredV1 || gap.Fault == nil || gap.Fault.RetryDirective != contract.RetryInspectV1 {
		t.Fatalf("gap result=%+v", gap)
	}
}

func TestEventRetentionRejectsSuccessorSpliceAndTailOverflowV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	store, err := OpenEventStreamV1(context.Background(), EventStreamConfigV1{Path: filepath.Join(t.TempDir(), "retention.sqlite"), Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stream := streamV1(t)
	current := refV1(t, contract.AgentRunEventRetentionCurrentRefKindV1, "retention-1", 1)
	if err := store.AppendEventV1(context.Background(), eventV1(t, stream, 1, now), current); err != nil {
		t.Fatal(err)
	}
	splicedID := refV1(t, contract.AgentRunEventRetentionCurrentRefKindV1, "other-retention", 2)
	if err := store.AdvanceRetentionV1(context.Background(), stream, current, splicedID, contract.NewWireUint64V1(1)); !contract.HasCode(err, contract.FaultRevisionConflictV1) {
		t.Fatalf("id splice error=%v want REVISION_CONFLICT", err)
	}
	skippedRevision := refV1(t, contract.AgentRunEventRetentionCurrentRefKindV1, current.ID, 3)
	if err := store.AdvanceRetentionV1(context.Background(), stream, current, skippedRevision, contract.NewWireUint64V1(1)); !contract.HasCode(err, contract.FaultRevisionConflictV1) {
		t.Fatalf("revision splice error=%v want REVISION_CONFLICT", err)
	}
	next := refV1(t, contract.AgentRunEventRetentionCurrentRefKindV1, current.ID, 2)
	if err := store.AdvanceRetentionV1(context.Background(), stream, current, next, contract.NewWireUint64V1(3)); !contract.HasCode(err, contract.FaultPreconditionFailedV1) {
		t.Fatalf("tail overflow error=%v want PRECONDITION_FAILED", err)
	}
}

func TestEventRetentionCommitLostReplyReturnsUnknownAndPersistsV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	store, err := OpenEventStreamV1(context.Background(), EventStreamConfigV1{Path: filepath.Join(t.TempDir(), "retention-lost.sqlite"), Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stream := streamV1(t)
	current := refV1(t, contract.AgentRunEventRetentionCurrentRefKindV1, "retention-1", 1)
	next := refV1(t, contract.AgentRunEventRetentionCurrentRefKindV1, current.ID, 2)
	if err := store.AppendEventV1(context.Background(), eventV1(t, stream, 1, now), current); err != nil {
		t.Fatal(err)
	}
	store.commitV1 = func(tx *sql.Tx) error {
		if err := tx.Commit(); err != nil {
			return err
		}
		return context.Canceled
	}
	if err := store.AdvanceRetentionV1(context.Background(), stream, current, next, contract.NewWireUint64V1(1)); !contract.HasCode(err, contract.FaultUnknownOutcomeV1) {
		t.Fatalf("commit-lost error=%v want UNKNOWN_OUTCOME", err)
	}
	store.commitV1 = func(tx *sql.Tx) error { return tx.Commit() }
	result, err := store.WatchAgentRunV1(context.Background(), watchRequestV1(t, stream, next, 0, now))
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != contract.AgentRunWatchEventsV1 {
		t.Fatalf("committed retention was not visible after lost reply: %+v", result)
	}
}

func TestEventStreamOpenAndMissingWatchFaultsV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	for _, config := range []EventStreamConfigV1{{Path: ""}, {Path: " spaced "}, {Path: "valid.sqlite", BusyTimeout: time.Minute + time.Nanosecond}} {
		if _, err := OpenEventStreamV1(context.Background(), config); !contract.HasCode(err, contract.FaultInvalidArgumentV1) {
			t.Fatalf("config=%+v error=%v want INVALID_ARGUMENT", config, err)
		}
	}
	store, err := OpenEventStreamV1(context.Background(), EventStreamConfigV1{Path: filepath.Join(t.TempDir(), "missing.sqlite"), Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stream := streamV1(t)
	retention := refV1(t, contract.AgentRunEventRetentionCurrentRefKindV1, "retention-1", 1)
	if _, err := store.WatchAgentRunV1(context.Background(), watchRequestV1(t, stream, retention, 0, now)); !contract.HasCode(err, contract.FaultNotFoundV1) {
		t.Fatalf("missing stream error=%v want NOT_FOUND", err)
	}
}

func TestEventAppendCrossHandleReplayAndSpliceV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	path := filepath.Join(t.TempDir(), "event-race.sqlite")
	first, err := OpenEventStreamV1(context.Background(), EventStreamConfigV1{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := OpenEventStreamV1(context.Background(), EventStreamConfigV1{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	stream := streamV1(t)
	retention := refV1(t, contract.AgentRunEventRetentionCurrentRefKindV1, "retention-1", 1)
	event := eventV1(t, stream, 1, now)
	stores := []*EventStreamV1{first, second}
	const workers = 64
	var group sync.WaitGroup
	errors := make(chan error, workers)
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			if err := stores[index%2].AppendEventV1(context.Background(), event, retention); err != nil {
				errors <- err
			}
		}(index)
	}
	group.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("exact replay failed: %v", err)
	}
	spliced := event
	spliced.PayloadDigest = digestV1(t, "different-payload")
	spliced.EventRef.Digest, spliced.EventDigest = "", ""
	spliced, err = contract.SealAgentRunEventV1(spliced)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.AppendEventV1(context.Background(), spliced, retention); !contract.HasCode(err, contract.FaultRevisionConflictV1) {
		t.Fatalf("event splice error=%v want REVISION_CONFLICT", err)
	}
}

func TestEventRetentionCrossHandleCASLinearizesV1(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	path := filepath.Join(t.TempDir(), "retention-race.sqlite")
	first, err := OpenEventStreamV1(context.Background(), EventStreamConfigV1{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := OpenEventStreamV1(context.Background(), EventStreamConfigV1{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	stream := streamV1(t)
	current := refV1(t, contract.AgentRunEventRetentionCurrentRefKindV1, "retention-1", 1)
	if err := first.AppendEventV1(context.Background(), eventV1(t, stream, 1, now), current); err != nil {
		t.Fatal(err)
	}
	nextA := refV1(t, contract.AgentRunEventRetentionCurrentRefKindV1, current.ID, 2)
	nextB := nextA
	nextB.Digest = digestV1(t, "alternate-successor")
	var successes atomic.Int64
	var conflicts atomic.Int64
	var group sync.WaitGroup
	for index, candidate := range []contract.ExactRefWireV1{nextA, nextB} {
		group.Add(1)
		go func(index int, candidate contract.ExactRefWireV1) {
			defer group.Done()
			err := []*EventStreamV1{first, second}[index].AdvanceRetentionV1(context.Background(), stream, current, candidate, contract.NewWireUint64V1(1))
			switch {
			case err == nil:
				successes.Add(1)
			case contract.HasCode(err, contract.FaultRevisionConflictV1):
				conflicts.Add(1)
			default:
				t.Errorf("unexpected retention error: %v", err)
			}
		}(index, candidate)
	}
	group.Wait()
	if successes.Load() != 1 || conflicts.Load() != 1 {
		t.Fatalf("successes=%d conflicts=%d want 1/1", successes.Load(), conflicts.Load())
	}
}
