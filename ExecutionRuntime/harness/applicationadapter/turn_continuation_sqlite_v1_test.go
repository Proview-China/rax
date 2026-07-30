package applicationadapter

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type sqliteTurnContinuationFixtureV1 struct {
	now     time.Time
	start   applicationcontract.TurnContinuationStartRequestV1
	refresh applicationcontract.ContextTurnRefreshResultV1
}

type sqliteTurnContinuationClockV1 struct{ unixNano atomic.Int64 }

func newSQLiteTurnContinuationClockV1(now time.Time) *sqliteTurnContinuationClockV1 {
	clock := &sqliteTurnContinuationClockV1{}
	clock.Set(now)
	return clock
}

func (c *sqliteTurnContinuationClockV1) Now() time.Time    { return time.Unix(0, c.unixNano.Load()) }
func (c *sqliteTurnContinuationClockV1) Set(now time.Time) { c.unixNano.Store(now.UnixNano()) }

func sqliteTurnContinuationDigestV1(seed string) core.Digest { return core.DigestBytes([]byte(seed)) }

func sqliteTurnContinuationExactRefV1(kind, id string) applicationcontract.ContextRefreshExactRefV1 {
	return applicationcontract.ContextRefreshExactRefV1{Kind: runtimeports.NamespacedNameV2(kind), ID: id, Revision: 1, Digest: sqliteTurnContinuationDigestV1(id)}
}

func newSQLiteTurnContinuationFixtureV1(t *testing.T) sqliteTurnContinuationFixtureV1 {
	t.Helper()
	now := time.Unix(1_810_000_000, 0)
	scope := sqliteTurnContinuationDigestV1("scope")
	runID := core.AgentRunID("run-1")
	sessionDigest := sqliteTurnContinuationDigestV1("session")
	turnDigest := sqliteTurnContinuationDigestV1("turn-7")
	checked := now.Add(-time.Second).UnixNano()
	expires := now.Add(time.Minute).UnixNano()
	source, err := applicationcontract.SealContextTurnSourceCurrentV1(applicationcontract.ContextTurnSourceCurrentV1{
		ExecutionScopeDigest: scope,
		RunID:                runID,
		Session:              applicationcontract.SingleCallSessionCoordinateV1{ID: "session-1", Revision: 4, Digest: sessionDigest, Phase: applicationcontract.SingleCallSessionWaitingActionV1, CheckedUnixNano: checked, ExpiresUnixNano: expires},
		SessionApplicability: applicationcontract.SingleCallSessionApplicabilitySourceCoordinateV1{Kind: applicationcontract.SingleCallSessionSourceKindV1, ID: "session:" + string(sessionDigest), Revision: 4, Digest: sessionDigest},
		Turn:                 applicationcontract.SingleCallTurnCoordinateV1{ID: "turn:" + string(turnDigest), Ordinal: 7, Revision: 7, Digest: turnDigest},
		TurnApplicability:    applicationcontract.SingleCallTurnApplicabilitySourceCoordinateV1{Kind: applicationcontract.SingleCallTurnSourceKindV1, ID: "turn:" + string(turnDigest), Revision: 7, Digest: turnDigest},
		CheckedUnixNano:      checked,
		ExpiresUnixNano:      expires,
	})
	mustSQLiteTurnContinuationV1(t, err)
	active, err := applicationcontract.SealHarnessActiveContextRefV1(applicationcontract.HarnessActiveContextRefV1{
		Revision:                 11,
		ExecutionScopeDigest:     scope,
		RunID:                    runID,
		SessionID:                source.Session.ID,
		TurnOrdinal:              source.Turn.Ordinal,
		ManifestRef:              sqliteTurnContinuationExactRefV1("context/manifest", "manifest-7"),
		FrameRef:                 sqliteTurnContinuationExactRefV1("context/frame", "frame-7"),
		GenerationRef:            sqliteTurnContinuationExactRefV1("context/generation", "generation-7"),
		ContextCurrentPointerRef: sqliteTurnContinuationExactRefV1("context/generation-current", "current-7"),
		UpdatedUnixNano:          now.Add(-2 * time.Second).UnixNano(),
	})
	mustSQLiteTurnContinuationV1(t, err)
	tool := applicationcontract.SingleCallToolActionResultRefV2{
		ID: "single-call-result-1", Revision: 1, Digest: sqliteTurnContinuationDigestV1("single-call-result"),
		RequestID: "single-call-request-1", RequestRevision: 1, RequestDigest: sqliteTurnContinuationDigestV1("single-call-request"),
		ActionCoordinateDigest: sqliteTurnContinuationDigestV1("action-coordinate"), ToolResultID: "tool-result-1", ToolResultRevision: 3, ToolResultDigest: sqliteTurnContinuationDigestV1("tool-result"),
	}
	startInput := applicationcontract.TurnContinuationStartRequestV1{
		ExecutionScopeDigest:      scope,
		RunID:                     runID,
		Source:                    source,
		SettledToolResult:         tool,
		ExpectedActiveContext:     active,
		TargetTurn:                source.Turn.Ordinal + 1,
		RequestedNotAfterUnixNano: now.Add(30 * time.Second).UnixNano(),
	}
	attemptID, err := applicationcontract.DeriveTurnContinuationAttemptIDV1(startInput)
	mustSQLiteTurnContinuationV1(t, err)
	memoryItem, err := applicationcontract.SealContextOwnerSourceItemV1(applicationcontract.ContextOwnerSourceItemV1{
		Rank: 0, ItemDigest: sqliteTurnContinuationDigestV1("memory-item"), RecordRef: sqliteTurnContinuationExactRefV1("memory/record", "memory-record"),
		StableOwnerChain: []applicationcontract.ContextRefreshExactRefV1{sqliteTurnContinuationExactRefV1("memory/source", "memory-source")},
		ContentRef:       applicationcontract.ContextOwnerContentRefV1{ID: "memory-content", Digest: sqliteTurnContinuationDigestV1("memory-content"), Length: 12, MediaType: "text/plain"},
		TokenEstimate:    3, Sensitivity: "internal", CitationDigest: sqliteTurnContinuationDigestV1("memory-citation"), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
	})
	mustSQLiteTurnContinuationV1(t, err)
	memory, err := applicationcontract.SealContextOwnerSourceEnvelopeV1(applicationcontract.ContextOwnerSourceEnvelopeV1{
		ID: "memory-envelope", Owner: applicationcontract.ContextOwnerMemoryV1, SourceSession: source.Session, SessionApplicability: source.SessionApplicability, SourceTurn: source.Turn, TurnApplicability: source.TurnApplicability,
		AttemptInspectionRef: sqliteTurnContinuationExactRefV1("memory/inspection", "memory-inspection"), CurrentProjectionRef: sqliteTurnContinuationExactRefV1("memory/projection", "memory-projection"), StableClosureDigest: sqliteTurnContinuationDigestV1("memory-closure"),
		Items: []applicationcontract.ContextOwnerSourceItemV1{memoryItem}, Phase: applicationcontract.ContextSourceCheckS1V1, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
	})
	mustSQLiteTurnContinuationV1(t, err)
	memoryRequest, err := applicationcontract.SealContextOwnerSourceRequestV1(applicationcontract.ContextOwnerSourceRequestV1{
		Owner: applicationcontract.ContextOwnerMemoryV1, SourceSession: source.Session, SessionApplicability: source.SessionApplicability, SourceTurn: source.Turn, TurnApplicability: source.TurnApplicability,
		OwnerRequest: []byte(`{"owner":"memory"}`), Phase: applicationcontract.ContextSourceCheckS1V1, RequestedNotAfterNano: now.Add(30 * time.Second).UnixNano(),
	})
	mustSQLiteTurnContinuationV1(t, err)
	prepare, err := applicationcontract.SealContextTurnRefreshPrepareRequestV1(applicationcontract.ContextTurnRefreshPrepareRequestV1{
		ID: attemptID, ExecutionScopeDigest: scope, RunID: runID, SourceSession: source.Session, SessionApplicability: source.SessionApplicability,
		SourceTurn: source.Turn, TurnApplicability: source.TurnApplicability, ExpectedTargetTurn: source.Turn.Ordinal + 1,
		OpaqueContextRequest: []byte(`{"turn":"next"}`), MemoryRequest: &memoryRequest, Memory: &memory, RequestedNotAfterNano: now.Add(30 * time.Second).UnixNano(),
	})
	mustSQLiteTurnContinuationV1(t, err)
	startInput.ExpectedContextRefreshAttempt, err = prepare.AttemptRefV1()
	mustSQLiteTurnContinuationV1(t, err)
	start, err := applicationcontract.SealTurnContinuationStartRequestV1(startInput)
	mustSQLiteTurnContinuationV1(t, err)
	applySettlement := sqliteTurnContinuationExactRefV1("context/apply-settlement", "apply-8")
	currentPointer := sqliteTurnContinuationExactRefV1("context/generation-current", "current-8")
	refresh, err := applicationcontract.SealContextTurnRefreshResultV1(applicationcontract.ContextTurnRefreshResultV1{
		AttemptRef:             start.ExpectedContextRefreshAttempt,
		PendingDomainResultRef: sqliteTurnContinuationExactRefV1("context/pending-result", "pending-8"),
		TransitionProofRef:     sqliteTurnContinuationExactRefV1("context/transition-proof", "proof-8"),
		ManifestRef:            sqliteTurnContinuationExactRefV1("context/manifest", "manifest-8"),
		FrameRef:               sqliteTurnContinuationExactRefV1("context/frame", "frame-8"),
		GenerationRef:          sqliteTurnContinuationExactRefV1("context/generation", "generation-8"),
		StableSourceSetDigest:  sqliteTurnContinuationDigestV1("stable-source"),
		S1AssociationSetDigest: sqliteTurnContinuationDigestV1("s1-association"),
		S2AssociationSetDigest: sqliteTurnContinuationDigestV1("s2-association"),
		ApplySettlementRef:     &applySettlement,
		CurrentPointerRef:      &currentPointer,
		CheckedUnixNano:        now.Add(time.Second).UnixNano(),
		ExpiresUnixNano:        now.Add(20 * time.Second).UnixNano(),
		State:                  applicationcontract.ContextTurnRefreshAppliedStateV1,
	})
	mustSQLiteTurnContinuationV1(t, err)
	return sqliteTurnContinuationFixtureV1{now: now, start: start, refresh: refresh}
}

func (f sqliteTurnContinuationFixtureV1) commitV1(t *testing.T, pending applicationcontract.TurnContinuationCurrentV1, refresh applicationcontract.ContextTurnRefreshResultV1) applicationcontract.TurnContinuationCommitRequestV1 {
	t.Helper()
	commit, err := applicationcontract.SealTurnContinuationCommitRequestV1(applicationcontract.TurnContinuationCommitRequestV1{
		Pending: pending, AppliedContextRefresh: refresh, RequestedNotAfterUnixNano: f.now.Add(15 * time.Second).UnixNano(),
	}, f.now.Add(2*time.Second))
	mustSQLiteTurnContinuationV1(t, err)
	return commit
}

func (f sqliteTurnContinuationFixtureV1) alternateRefreshV1(t *testing.T) applicationcontract.ContextTurnRefreshResultV1 {
	t.Helper()
	refresh := f.refresh
	refresh.PendingDomainResultRef = sqliteTurnContinuationExactRefV1("context/pending-result", "pending-alt")
	refresh.TransitionProofRef = sqliteTurnContinuationExactRefV1("context/transition-proof", "proof-alt")
	refresh.ManifestRef = sqliteTurnContinuationExactRefV1("context/manifest", "manifest-alt")
	refresh.FrameRef = sqliteTurnContinuationExactRefV1("context/frame", "frame-alt")
	refresh.GenerationRef = sqliteTurnContinuationExactRefV1("context/generation", "generation-alt")
	applySettlement := sqliteTurnContinuationExactRefV1("context/apply-settlement", "apply-alt")
	currentPointer := sqliteTurnContinuationExactRefV1("context/generation-current", "current-alt")
	refresh.ApplySettlementRef = &applySettlement
	refresh.CurrentPointerRef = &currentPointer
	refresh.Digest = ""
	sealed, err := applicationcontract.SealContextTurnRefreshResultV1(refresh)
	mustSQLiteTurnContinuationV1(t, err)
	return sealed
}

func openSQLiteTurnContinuationTestStoreV1(t *testing.T, path string, clock *sqliteTurnContinuationClockV1) *SQLiteTurnContinuationStoreV1 {
	t.Helper()
	store, err := OpenSQLiteTurnContinuationStoreV1(context.Background(), SQLiteTurnContinuationStoreConfigV1{Path: path, Clock: clock.Now})
	mustSQLiteTurnContinuationV1(t, err)
	return store
}

func inspectSQLiteTurnContinuationRequestV1(t *testing.T, start applicationcontract.TurnContinuationStartRequestV1) applicationcontract.TurnContinuationInspectRequestV1 {
	t.Helper()
	request, err := applicationcontract.SealTurnContinuationInspectRequestV1(applicationcontract.TurnContinuationInspectRequestV1{AttemptRef: start.AttemptRefV1()})
	mustSQLiteTurnContinuationV1(t, err)
	return request
}

func TestSQLiteTurnContinuationBeginCommitInspectIdempotencyAndDeepCopyV1(t *testing.T) {
	fixture := newSQLiteTurnContinuationFixtureV1(t)
	clock := newSQLiteTurnContinuationClockV1(fixture.now)
	store := openSQLiteTurnContinuationTestStoreV1(t, filepath.Join(t.TempDir(), "continuation.db"), clock)
	defer store.Close()
	ctx := context.Background()

	pending, err := store.BeginTurnContinuationV1(ctx, fixture.start)
	mustSQLiteTurnContinuationV1(t, err)
	repeated, err := store.BeginTurnContinuationV1(ctx, fixture.start)
	mustSQLiteTurnContinuationV1(t, err)
	if repeated.Digest != pending.Digest || repeated.State != applicationcontract.TurnContinuationPendingV1 {
		t.Fatalf("same Start was not Begin-idempotent: %#v", repeated)
	}

	drifted := fixture.start
	drifted.AttemptID = ""
	drifted.Digest = ""
	drifted.RequestedNotAfterUnixNano--
	drifted, err = applicationcontract.SealTurnContinuationStartRequestV1(drifted)
	mustSQLiteTurnContinuationV1(t, err)
	if drifted.AttemptID != fixture.start.AttemptID || drifted.Digest == fixture.start.Digest {
		t.Fatal("test did not create same-ID different-digest Start")
	}
	if _, err = store.BeginTurnContinuationV1(ctx, drifted); !core.HasCategory(err, core.ErrorConflict) || !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same-ID different-digest Start was not rejected: %v", err)
	}

	inspect := inspectSQLiteTurnContinuationRequestV1(t, fixture.start)
	inspected, err := store.InspectTurnContinuationV1(ctx, inspect)
	mustSQLiteTurnContinuationV1(t, err)
	if inspected.Digest != pending.Digest {
		t.Fatal("Inspect did not return exact pending current")
	}
	wrongInspect := inspect
	wrongInspect.AttemptRef.Digest = sqliteTurnContinuationDigestV1("wrong-original-attempt")
	wrongInspect.Digest = ""
	wrongInspect, err = applicationcontract.SealTurnContinuationInspectRequestV1(wrongInspect)
	mustSQLiteTurnContinuationV1(t, err)
	if _, err = store.InspectTurnContinuationV1(ctx, wrongInspect); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Inspect accepted a rebound AttemptRef: %v", err)
	}

	commit := fixture.commitV1(t, pending, fixture.refresh)
	clock.Set(fixture.now.Add(2 * time.Second))
	current, err := store.CommitTurnContinuationV1(ctx, commit)
	mustSQLiteTurnContinuationV1(t, err)
	repeatedCurrent, err := store.CommitTurnContinuationV1(ctx, commit)
	mustSQLiteTurnContinuationV1(t, err)
	if repeatedCurrent.Digest != current.Digest || repeatedCurrent.State != applicationcontract.TurnContinuationContextCurrentV1 {
		t.Fatal("same Commit was not idempotent")
	}
	beginAfterCommit, err := store.BeginTurnContinuationV1(ctx, fixture.start)
	mustSQLiteTurnContinuationV1(t, err)
	if beginAfterCommit.Digest != current.Digest {
		t.Fatal("same Start after Commit did not recover the final current")
	}
	alternateCommit := fixture.commitV1(t, pending, fixture.alternateRefreshV1(t))
	if _, err = store.CommitTurnContinuationV1(ctx, alternateCommit); !core.HasCategory(err, core.ErrorConflict) || !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("different Commit was not rejected: %v", err)
	}

	current.AppliedContextRefresh.CurrentPointerRef.ID = "caller-splice"
	current.AppliedContextRefresh.ApplySettlementRef.ID = "caller-splice"
	current.Start.ExpectedContextRefreshAttempt.ID = "caller-splice"
	recovered, err := store.InspectTurnContinuationV1(ctx, inspect)
	mustSQLiteTurnContinuationV1(t, err)
	if recovered.Digest != repeatedCurrent.Digest || recovered.AppliedContextRefresh.CurrentPointerRef.ID == "caller-splice" || recovered.Start.ExpectedContextRefreshAttempt.ID == "caller-splice" {
		t.Fatal("returned current was not deeply cloned")
	}
	if err = recovered.ModelTurnAllowedV1(clock.Now()); err != nil {
		t.Fatal(err)
	}
	if err = store.IntegrityCheckV1(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteTurnContinuationLostReplyAndRestartRecoverOnlyByOriginalAttemptV1(t *testing.T) {
	fixture := newSQLiteTurnContinuationFixtureV1(t)
	clock := newSQLiteTurnContinuationClockV1(fixture.now)
	path := filepath.Join(t.TempDir(), "continuation.db")
	store := openSQLiteTurnContinuationTestStoreV1(t, path, clock)
	ctx := context.Background()
	inspect := inspectSQLiteTurnContinuationRequestV1(t, fixture.start)

	store.LoseNextBeginReplyForTestingV1()
	if _, err := store.BeginTurnContinuationV1(ctx, fixture.start); !core.HasCategory(err, core.ErrorIndeterminate) || !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("lost Begin reply was not indeterminate: %v", err)
	}
	pending, err := store.InspectTurnContinuationV1(ctx, inspect)
	mustSQLiteTurnContinuationV1(t, err)
	if pending.State != applicationcontract.TurnContinuationPendingV1 {
		t.Fatalf("original Attempt Inspect did not recover pending: %s", pending.State)
	}

	commit := fixture.commitV1(t, pending, fixture.refresh)
	clock.Set(fixture.now.Add(2 * time.Second))
	store.LoseNextCommitReplyForTestingV1()
	if _, err = store.CommitTurnContinuationV1(ctx, commit); !core.HasCategory(err, core.ErrorIndeterminate) || !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("lost Commit reply was not indeterminate: %v", err)
	}
	current, err := store.InspectTurnContinuationV1(ctx, inspect)
	mustSQLiteTurnContinuationV1(t, err)
	if current.State != applicationcontract.TurnContinuationContextCurrentV1 || current.CommitRequestDigest != commit.Digest {
		t.Fatalf("original Attempt Inspect did not recover committed current: %#v", current)
	}
	mustSQLiteTurnContinuationV1(t, store.Close())

	clock.Set(fixture.now.Add(3 * time.Second))
	restarted := openSQLiteTurnContinuationTestStoreV1(t, path, clock)
	defer restarted.Close()
	recovered, err := restarted.InspectTurnContinuationV1(ctx, inspect)
	mustSQLiteTurnContinuationV1(t, err)
	if recovered.Digest != current.Digest || recovered.CommitRequestDigest != commit.Digest {
		t.Fatal("restart did not recover the exact committed current")
	}
	wrong := inspect
	wrong.AttemptRef.Digest = sqliteTurnContinuationDigestV1("rebound-after-restart")
	wrong.Digest = ""
	wrong, err = applicationcontract.SealTurnContinuationInspectRequestV1(wrong)
	mustSQLiteTurnContinuationV1(t, err)
	if _, err = restarted.InspectTurnContinuationV1(ctx, wrong); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("restart recovery accepted a rebound AttemptRef: %v", err)
	}
}

func TestSQLiteTurnContinuationConcurrentBeginAndCommitV1(t *testing.T) {
	fixture := newSQLiteTurnContinuationFixtureV1(t)
	clock := newSQLiteTurnContinuationClockV1(fixture.now)
	store := openSQLiteTurnContinuationTestStoreV1(t, filepath.Join(t.TempDir(), "continuation.db"), clock)
	defer store.Close()
	ctx := context.Background()
	const workers = 32

	beginValues := make(chan applicationcontract.TurnContinuationCurrentV1, workers)
	beginErrors := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			value, err := store.BeginTurnContinuationV1(ctx, fixture.start)
			if err != nil {
				beginErrors <- err
				return
			}
			beginValues <- value
		}()
	}
	wait.Wait()
	close(beginValues)
	close(beginErrors)
	for err := range beginErrors {
		t.Fatal(err)
	}
	var pending applicationcontract.TurnContinuationCurrentV1
	for value := range beginValues {
		if pending.Digest == "" {
			pending = value
		}
		if value.Digest != pending.Digest || value.State != applicationcontract.TurnContinuationPendingV1 {
			t.Fatal("concurrent Begin created more than one pending current")
		}
	}
	commit := fixture.commitV1(t, pending, fixture.refresh)
	clock.Set(fixture.now.Add(2 * time.Second))
	commitValues := make(chan applicationcontract.TurnContinuationCurrentV1, workers)
	commitErrors := make(chan error, workers)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			value, err := store.CommitTurnContinuationV1(ctx, commit)
			if err != nil {
				commitErrors <- err
				return
			}
			commitValues <- value
		}()
	}
	wait.Wait()
	close(commitValues)
	close(commitErrors)
	for err := range commitErrors {
		t.Fatal(err)
	}
	var current applicationcontract.TurnContinuationCurrentV1
	for value := range commitValues {
		if current.Digest == "" {
			current = value
		}
		if value.Digest != current.Digest || value.CommitRequestDigest != commit.Digest {
			t.Fatal("concurrent same Commit published more than one successor")
		}
	}
}

func TestSQLiteTurnContinuationConcurrentDifferentCommitHasOneWinnerV1(t *testing.T) {
	fixture := newSQLiteTurnContinuationFixtureV1(t)
	clock := newSQLiteTurnContinuationClockV1(fixture.now)
	store := openSQLiteTurnContinuationTestStoreV1(t, filepath.Join(t.TempDir(), "continuation.db"), clock)
	defer store.Close()
	ctx := context.Background()
	pending, err := store.BeginTurnContinuationV1(ctx, fixture.start)
	mustSQLiteTurnContinuationV1(t, err)
	commits := []applicationcontract.TurnContinuationCommitRequestV1{fixture.commitV1(t, pending, fixture.refresh), fixture.commitV1(t, pending, fixture.alternateRefreshV1(t))}
	clock.Set(fixture.now.Add(2 * time.Second))

	type outcome struct {
		index int
		value applicationcontract.TurnContinuationCurrentV1
		err   error
	}
	outcomes := make(chan outcome, 16)
	var wait sync.WaitGroup
	for index := range 16 {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			value, err := store.CommitTurnContinuationV1(ctx, commits[index%2])
			outcomes <- outcome{index: index % 2, value: value, err: err}
		}(index)
	}
	wait.Wait()
	close(outcomes)
	winner := -1
	conflicts := 0
	for result := range outcomes {
		if result.err == nil {
			if winner < 0 {
				winner = result.index
			}
			if winner != result.index || result.value.CommitRequestDigest != commits[winner].Digest {
				t.Fatal("different Commit requests both won")
			}
			continue
		}
		if !core.HasCategory(result.err, core.ErrorConflict) {
			t.Fatalf("losing Commit returned non-conflict: %v", result.err)
		}
		conflicts++
	}
	if winner < 0 || conflicts == 0 {
		t.Fatalf("different Commit race lacked a single winner/conflicts: winner=%d conflicts=%d", winner, conflicts)
	}
	current, err := store.InspectTurnContinuationV1(ctx, inspectSQLiteTurnContinuationRequestV1(t, fixture.start))
	mustSQLiteTurnContinuationV1(t, err)
	if current.CommitRequestDigest != commits[winner].Digest {
		t.Fatal("Inspect did not recover the race winner")
	}
}

func TestSQLiteTurnContinuationRejectsStaleActiveContextAndBodySpliceV1(t *testing.T) {
	fixture := newSQLiteTurnContinuationFixtureV1(t)
	ctx := context.Background()

	t.Run("stale ActiveContext indexed CAS", func(t *testing.T) {
		clock := newSQLiteTurnContinuationClockV1(fixture.now)
		store := openSQLiteTurnContinuationTestStoreV1(t, filepath.Join(t.TempDir(), "stale.db"), clock)
		defer store.Close()
		pending, err := store.BeginTurnContinuationV1(ctx, fixture.start)
		mustSQLiteTurnContinuationV1(t, err)
		_, err = store.db.ExecContext(ctx, `UPDATE harness_turn_continuation_current_v1 SET active_context_revision=active_context_revision+1 WHERE attempt_id=?`, fixture.start.AttemptID)
		mustSQLiteTurnContinuationV1(t, err)
		clock.Set(fixture.now.Add(2 * time.Second))
		if _, err = store.CommitTurnContinuationV1(ctx, fixture.commitV1(t, pending, fixture.refresh)); !core.HasCategory(err, core.ErrorConflict) || !core.HasReason(err, core.ReasonEvidenceConflict) {
			t.Fatalf("stale indexed ActiveContext was not rejected: %v", err)
		}
	})

	t.Run("canonical body splice", func(t *testing.T) {
		clock := newSQLiteTurnContinuationClockV1(fixture.now)
		store := openSQLiteTurnContinuationTestStoreV1(t, filepath.Join(t.TempDir(), "splice.db"), clock)
		defer store.Close()
		_, err := store.BeginTurnContinuationV1(ctx, fixture.start)
		mustSQLiteTurnContinuationV1(t, err)
		_, err = store.db.ExecContext(ctx, `UPDATE harness_turn_continuation_current_v1 SET canonical_json=? WHERE attempt_id=?`, []byte(`{"spliced":true}`), fixture.start.AttemptID)
		mustSQLiteTurnContinuationV1(t, err)
		if _, err = store.InspectTurnContinuationV1(ctx, inspectSQLiteTurnContinuationRequestV1(t, fixture.start)); !core.HasCategory(err, core.ErrorConflict) || !core.HasReason(err, core.ReasonInvalidCanonicalForm) {
			t.Fatalf("spliced canonical body was not rejected: %v", err)
		}
	})
}

func TestSQLiteTurnContinuationClockAndTTLCounterexamplesV1(t *testing.T) {
	fixture := newSQLiteTurnContinuationFixtureV1(t)
	ctx := context.Background()
	t.Run("migration rejects zero clock", func(t *testing.T) {
		clock := newSQLiteTurnContinuationClockV1(time.Time{})
		_, err := OpenSQLiteTurnContinuationStoreV1(ctx, SQLiteTurnContinuationStoreConfigV1{Path: filepath.Join(t.TempDir(), "zero.db"), Clock: clock.Now})
		if !core.HasCategory(err, core.ErrorPreconditionFailed) || !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("zero clock was not rejected: %v", err)
		}
	})
	t.Run("Begin rejects regressed clock", func(t *testing.T) {
		clock := newSQLiteTurnContinuationClockV1(fixture.now)
		store := openSQLiteTurnContinuationTestStoreV1(t, filepath.Join(t.TempDir(), "regressed.db"), clock)
		defer store.Close()
		clock.Set(time.Unix(0, fixture.start.Source.CheckedUnixNano-1))
		_, err := store.BeginTurnContinuationV1(ctx, fixture.start)
		if !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("regressed Begin clock was not rejected: %v", err)
		}
	})
	t.Run("exclusive pending and Commit deadlines", func(t *testing.T) {
		clock := newSQLiteTurnContinuationClockV1(fixture.now)
		store := openSQLiteTurnContinuationTestStoreV1(t, filepath.Join(t.TempDir(), "ttl.db"), clock)
		defer store.Close()
		pending, err := store.BeginTurnContinuationV1(ctx, fixture.start)
		mustSQLiteTurnContinuationV1(t, err)
		commit := fixture.commitV1(t, pending, fixture.refresh)
		clock.Set(time.Unix(0, commit.RequestedNotAfterUnixNano))
		if _, err = store.CommitTurnContinuationV1(ctx, commit); !core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("Commit remained current at its exclusive deadline: %v", err)
		}
		clock.Set(time.Unix(0, pending.ExpiresUnixNano))
		if _, err = store.InspectTurnContinuationV1(ctx, inspectSQLiteTurnContinuationRequestV1(t, fixture.start)); !core.HasReason(err, core.ReasonBindingExpired) {
			t.Fatalf("pending remained current at its exclusive deadline: %v", err)
		}
	})
}

func TestSQLiteTurnContinuationResamplesClockAfterWriterWaitV1(t *testing.T) {
	fixture := newSQLiteTurnContinuationFixtureV1(t)
	clock := newSQLiteTurnContinuationClockV1(fixture.now)
	store := openSQLiteTurnContinuationTestStoreV1(t, filepath.Join(t.TempDir(), "toctou.db"), clock)
	defer store.Close()
	ctx := context.Background()

	store.mu.Lock()
	beginSampled := make(chan struct{})
	var beginSampleOnce sync.Once
	store.clock = func() time.Time {
		beginSampleOnce.Do(func() { close(beginSampled) })
		return clock.Now()
	}
	beginResult := make(chan error, 1)
	go func() {
		_, err := store.BeginTurnContinuationV1(ctx, fixture.start)
		beginResult <- err
	}()
	<-beginSampled
	clock.Set(time.Unix(0, fixture.start.RequestedNotAfterUnixNano))
	store.mu.Unlock()
	if err := <-beginResult; !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("Begin used its pre-lock clock sample past the deadline: %v", err)
	}

	clock.Set(fixture.now)
	store.clock = clock.Now
	pending, err := store.BeginTurnContinuationV1(ctx, fixture.start)
	mustSQLiteTurnContinuationV1(t, err)
	commit := fixture.commitV1(t, pending, fixture.refresh)
	clock.Set(fixture.now.Add(2 * time.Second))
	store.mu.Lock()
	commitSampled := make(chan struct{})
	var commitSampleOnce sync.Once
	store.clock = func() time.Time {
		commitSampleOnce.Do(func() { close(commitSampled) })
		return clock.Now()
	}
	commitResult := make(chan error, 1)
	go func() {
		_, err := store.CommitTurnContinuationV1(ctx, commit)
		commitResult <- err
	}()
	<-commitSampled
	clock.Set(time.Unix(0, commit.RequestedNotAfterUnixNano))
	store.mu.Unlock()
	if err = <-commitResult; !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("Commit used its pre-lock clock sample past the deadline: %v", err)
	}
	store.clock = clock.Now
	inspected, err := store.InspectTurnContinuationV1(ctx, inspectSQLiteTurnContinuationRequestV1(t, fixture.start))
	mustSQLiteTurnContinuationV1(t, err)
	if inspected.State != applicationcontract.TurnContinuationPendingV1 {
		t.Fatal("expired Commit mutated the pending current")
	}
}

func TestSQLiteTurnContinuationRejectsExtraSchemaVersionV1(t *testing.T) {
	fixture := newSQLiteTurnContinuationFixtureV1(t)
	clock := newSQLiteTurnContinuationClockV1(fixture.now)
	path := filepath.Join(t.TempDir(), "schema.db")
	store := openSQLiteTurnContinuationTestStoreV1(t, path, clock)
	_, err := store.db.ExecContext(context.Background(), `INSERT INTO harness_turn_continuation_schema_v1(version,digest,applied_unix_nano) VALUES(2,?,?)`, string(sqliteTurnContinuationDigestV1("unknown-schema")), fixture.now.UnixNano())
	mustSQLiteTurnContinuationV1(t, err)
	mustSQLiteTurnContinuationV1(t, store.Close())
	_, err = OpenSQLiteTurnContinuationStoreV1(context.Background(), SQLiteTurnContinuationStoreConfigV1{Path: path, Clock: clock.Now})
	if !core.HasCategory(err, core.ErrorConflict) || !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("extra schema version was not rejected: %v", err)
	}
}

func mustSQLiteTurnContinuationV1(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
