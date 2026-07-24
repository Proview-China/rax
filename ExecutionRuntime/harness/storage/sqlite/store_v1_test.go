package sqlite

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var stateTestNowV1 = time.Date(2026, 7, 18, 4, 0, 0, 0, time.UTC)

func TestStatePlaneV1SessionRestartLostReplyHistoryAndProof(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/harness.db"
	store := openStateTestV1(t, path, func() time.Time { return stateTestNowV1 })
	creating := stateTestSessionV1(t, "restart", stateTestNowV1)
	store.faultMu.Lock()
	store.loseNextReply = true
	store.faultMu.Unlock()
	if _, err := store.CreateSessionV4(ctx, creating); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost create reply = %v", err)
	}
	inspected, err := store.InspectSessionV4(ctx, creating.Run, creating.ID)
	if err != nil || !reflect.DeepEqual(inspected, creating) {
		t.Fatalf("create recovery = %#v, %v", inspected, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openStateTestV1(t, path, func() time.Time { return stateTestNowV1 })
	defer store.Close()
	if replayed, err := store.CreateSessionV4(ctx, creating); err != nil || !reflect.DeepEqual(replayed, creating) {
		t.Fatalf("exact create replay = %#v, %v", replayed, err)
	}
	changedCreate := creating.Clone()
	changedCreate.CreatedUnixNano--
	changedCreate, err = contract.SealGovernedSessionV4(changedCreate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateSessionV4(ctx, changedCreate); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("changed create replay = %v", err)
	}
	terminal := stateTestTerminalV1(t, creating, stateTestNowV1)
	request := stateTestCASV1(t, creating, terminal)
	store.faultMu.Lock()
	store.loseNextReply = true
	store.faultMu.Unlock()
	if _, err := store.CompareAndSwapSessionV4(ctx, request); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost CAS reply = %v", err)
	}
	if replayed, err := store.CompareAndSwapSessionV4(ctx, request); err != nil || !reflect.DeepEqual(replayed, terminal) {
		t.Fatalf("exact CAS replay = %#v, %v", replayed, err)
	}
	alternative := terminal.Clone()
	alternative.CreatedUnixNano--
	alternative, err = contract.SealGovernedSessionV4(alternative)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompareAndSwapSessionV4(ctx, stateTestCASV1(t, creating, alternative)); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("changed CAS replay = %v", err)
	}
	proof, err := store.InspectCurrentDurableSessionEventV1(ctx, "harness-state-test")
	if err != nil || proof.SessionHistoryCount != 2 || proof.SessionCurrentCount != 1 || proof.EventCount != 0 {
		t.Fatalf("proof = %#v, %v", proof, err)
	}
	if exact, err := store.InspectDurableSessionEventCurrentV1(ctx, proof.RefV1()); err != nil || exact != proof {
		t.Fatalf("exact proof = %#v, %v", exact, err)
	}
	if err := store.IntegrityCheckV1(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestStatePlaneV1EventExactSequenceRestartAndNoOutcomeUpgrade(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/harness.db"
	store := openStateTestV1(t, path, func() time.Time { return stateTestNowV1 })
	first := stateTestEventV1(1, 1, contract.EventRunCompleted, stateTestNowV1.Add(-time.Second))
	store.faultMu.Lock()
	store.loseNextReply = true
	store.faultMu.Unlock()
	if err := store.AppendCandidate(ctx, first); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost event reply = %v", err)
	}
	got, err := store.InspectCandidate(ctx, first.SourceComponentID, first.SourceEpoch, first.SourceSequence)
	if err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("event recovery = %#v, %v", got, err)
	}
	if err := store.AppendCandidate(ctx, first); err != nil {
		t.Fatalf("event replay = %v", err)
	}
	changed := first
	changed.Kind = contract.EventRunFailed
	if err := store.AppendCandidate(ctx, changed); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("changed duplicate = %v", err)
	}
	if err := store.AppendCandidate(ctx, stateTestEventV1(1, 3, contract.EventRunFailed, stateTestNowV1)); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("sequence gap = %v", err)
	}
	second := stateTestEventV1(1, 2, contract.EventRunFailed, stateTestNowV1)
	if err := store.AppendCandidate(ctx, second); err != nil {
		t.Fatal(err)
	}
	otherEpoch := stateTestEventV1(2, 1, contract.EventRunStarted, stateTestNowV1)
	if err := store.AppendCandidate(ctx, otherEpoch); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openStateTestV1(t, path, func() time.Time { return stateTestNowV1 })
	defer store.Close()
	got, err = store.InspectCandidate(ctx, second.SourceComponentID, second.SourceEpoch, second.SourceSequence)
	if err != nil || !reflect.DeepEqual(got, second) {
		t.Fatalf("restart event = %#v, %v", got, err)
	}
	proof, err := store.InspectCurrentDurableSessionEventV1(ctx, "harness-state-test")
	if err != nil || proof.EventCount != 3 || proof.EventSourceCount != 2 || proof.SessionCurrentCount != 0 {
		t.Fatalf("event proof = %#v, %v", proof, err)
	}
	// A completion claim remains an immutable Harness source candidate. This
	// State Plane has no Runtime settlement/outcome method or table.
	var runtimeTables int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name LIKE '%runtime%outcome%'`).Scan(&runtimeTables); err != nil || runtimeTables != 0 {
		t.Fatalf("Runtime outcome surface leaked: count=%d err=%v", runtimeTables, err)
	}
}

func TestStatePlaneV1SixtyFourIndependentStoresHaveOneSessionCASWinner(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/harness.db"
	creating := stateTestSessionV1(t, "race", stateTestNowV1)
	seed := openStateTestV1(t, path, func() time.Time { return stateTestNowV1.Add(time.Minute) })
	if _, err := seed.CreateSessionV4(ctx, creating); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var winners, conflicts, unexpected atomic.Int64
	var wait sync.WaitGroup
	for index := 0; index < 64; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			store, err := OpenV1(ctx, ConfigV1{Path: path, StoreID: "harness-state-test", ProofTTL: time.Minute, Clock: func() time.Time { return stateTestNowV1.Add(time.Minute) }})
			if err != nil {
				unexpected.Add(1)
				return
			}
			defer store.Close()
			terminal := stateTestTerminalAtV1(t, creating, stateTestNowV1.Add(time.Duration(index+1)*time.Nanosecond))
			request := stateTestCASV1(t, creating, terminal)
			<-start
			_, err = store.CompareAndSwapSessionV4(ctx, request)
			if err == nil {
				winners.Add(1)
			} else if core.HasCategory(err, core.ErrorConflict) {
				conflicts.Add(1)
			} else {
				unexpected.Add(1)
			}
		}(index)
	}
	close(start)
	wait.Wait()
	if winners.Load() != 1 || conflicts.Load() != 63 || unexpected.Load() != 0 {
		t.Fatalf("winners=%d conflicts=%d unexpected=%d", winners.Load(), conflicts.Load(), unexpected.Load())
	}
}

func TestStatePlaneV1CorruptionSchemaClockTTLAndTypedNilFailClosed(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/harness.db"
	now := stateTestNowV1
	clock := func() time.Time { return now }
	store, err := OpenV1(ctx, ConfigV1{Path: path, StoreID: "harness-state-test", ProofTTL: time.Second, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	creating := stateTestSessionV1(t, "corrupt", now)
	if _, err = store.CreateSessionV4(ctx, creating); err != nil {
		t.Fatal(err)
	}
	event := stateTestEventV1(1, 1, contract.EventRunStarted, now)
	if err = store.AppendCandidate(ctx, event); err != nil {
		t.Fatal(err)
	}
	now = now.Add(-time.Nanosecond)
	if err = store.AppendCandidate(ctx, event); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("idempotent event under clock rollback = %v", err)
	}
	now = stateTestNowV1
	if _, err = store.db.Exec(`UPDATE harness_session_history_v4 SET canonical_json=json_set(canonical_json,'$.unknown_field',1) WHERE session_id=?`, creating.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectSessionV4(ctx, creating.Run, creating.ID); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("session row corruption = %v", err)
	}
	if _, err = store.db.Exec(`UPDATE harness_event_candidate_v1 SET row_digest=? WHERE source_component_id=?`, "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", event.SourceComponentID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectCandidate(ctx, event.SourceComponentID, event.SourceEpoch, event.SourceSequence); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("event row corruption = %v", err)
	}
	now = now.Add(time.Second)
	if _, err = store.InspectCurrentDurableSessionEventV1(ctx, "harness-state-test"); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("proof TTL = %v", err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	// Restore row corruption so reopen reaches proof refresh, then prove a
	// persisted clock high-water rejects rollback.
	repair := openStateTestV1(t, path, func() time.Time { return stateTestNowV1.Add(time.Second) })
	cleanSession, rowDigest, _ := encodeRowV1("GovernedSessionV4", creating)
	cleanEvent, eventRowDigest, _ := encodeRowV1("EventCandidateV1", event)
	if _, err = repair.db.Exec(`UPDATE harness_session_history_v4 SET canonical_json=?,row_digest=? WHERE session_id=?`, cleanSession, rowDigest, creating.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = repair.db.Exec(`UPDATE harness_event_candidate_v1 SET canonical_json=?,row_digest=? WHERE source_component_id=?`, cleanEvent, eventRowDigest, event.SourceComponentID); err != nil {
		t.Fatal(err)
	}
	_ = repair.Close()
	store, err = OpenV1(ctx, ConfigV1{Path: path, StoreID: "harness-state-test", ProofTTL: time.Second, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	rollback := stateTestNowV1.Add(-time.Nanosecond)
	if reopened, err := OpenV1(ctx, ConfigV1{Path: path, StoreID: "harness-state-test", ProofTTL: time.Second, Clock: func() time.Time { return rollback }}); !core.HasReason(err, core.ReasonClockRegression) {
		if reopened != nil {
			_ = reopened.Close()
		}
		t.Fatalf("clock rollback reopen = %v", err)
	}

	var nilStore *StoreV1
	if _, err := nilStore.InspectSessionV4(ctx, contract.RunRef{}, "session"); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("typed-nil session = %v", err)
	}
	if err := nilStore.AppendCandidate(ctx, contract.Event{}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("typed-nil event = %v", err)
	}
	if _, err := nilStore.InspectCurrentDurableSessionEventV1(ctx, "store"); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("typed-nil proof = %v", err)
	}
}

func TestStatePlaneV1SchemaDigestDriftFailsClosedOnReopen(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/harness.db"
	store := openStateTestV1(t, path, func() time.Time { return stateTestNowV1 })
	if _, err := store.db.Exec(`UPDATE harness_state_schema_v1 SET digest=? WHERE version=?`, "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", schemaVersionV1); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := OpenV1(ctx, ConfigV1{Path: path, StoreID: "harness-state-test", ProofTTL: time.Minute, Clock: func() time.Time { return stateTestNowV1 }}); !core.HasCategory(err, core.ErrorConflict) {
		if reopened != nil {
			_ = reopened.Close()
		}
		t.Fatalf("schema drift = %v", err)
	}
}

func openStateTestV1(t *testing.T, path string, clock func() time.Time) *StoreV1 {
	t.Helper()
	store, err := OpenV1(context.Background(), ConfigV1{Path: path, StoreID: "harness-state-test", ProofTTL: time.Minute, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func stateTestSessionV1(t *testing.T, suffix string, now time.Time) contract.GovernedSessionV4 {
	t.Helper()
	v2, _ := testkit.GovernedFactsV2(now)
	v2.ID = "session-" + suffix
	sealed, err := contract.SealGovernedSessionV4(contract.GovernedSessionV4{ID: v2.ID, Revision: 1, Run: v2.Run, Endpoint: v2.Endpoint, Phase: contract.SessionCreatingV2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func stateTestTerminalV1(t *testing.T, creating contract.GovernedSessionV4, now time.Time) contract.GovernedSessionV4 {
	return stateTestTerminalAtV1(t, creating, now)
}

func stateTestTerminalAtV1(t *testing.T, creating contract.GovernedSessionV4, updated time.Time) contract.GovernedSessionV4 {
	t.Helper()
	next := creating.Clone()
	next.Revision = 2
	next.Phase = contract.SessionTerminalV2
	next.CompletionClaim = contract.ClaimCancelled
	next.UpdatedUnixNano = updated.UnixNano()
	sealed, err := contract.SealGovernedSessionV4(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func stateTestCASV1(t *testing.T, current, next contract.GovernedSessionV4) contract.SessionCASRequestV4 {
	t.Helper()
	sealed, err := contract.SealSessionCASRequestV4(contract.SessionCASRequestV4{Run: current.Run, SessionID: current.ID, ExpectedRevision: current.Revision, ExpectedDigest: current.Digest, Next: next})
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func stateTestEventV1(epoch core.Epoch, sequence uint64, kind contract.EventKind, observed time.Time) contract.Event {
	payload := testkit.Payload("praxis.harness.test-event/v1", map[string]any{"sequence": sequence, "kind": kind})
	return contract.Event{SourceComponentID: "components/harness/test-source", SourceEpoch: epoch, SourceSequence: sequence, RunID: "run-event", Kind: kind, Payload: payload, ObservedAt: observed.UTC()}
}
