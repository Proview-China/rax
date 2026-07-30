package contract

import (
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type turnContinuationFixtureV1 struct {
	now     time.Time
	prepare ContextTurnRefreshPrepareRequestV1
	start   TurnContinuationStartRequestV1
	pending TurnContinuationCurrentV1
	refresh ContextTurnRefreshResultV1
	commit  TurnContinuationCommitRequestV1
	current TurnContinuationCurrentV1
}

func continuationDigestV1(seed string) core.Digest { return core.DigestBytes([]byte(seed)) }

func continuationExactRefV1(kind, id string) ContextRefreshExactRefV1 {
	return ContextRefreshExactRefV1{Kind: runtimeports.NamespacedNameV2(kind), ID: id, Revision: 1, Digest: continuationDigestV1(id)}
}

func newTurnContinuationFixtureV1(t *testing.T) turnContinuationFixtureV1 {
	t.Helper()
	now := time.Unix(1_810_000_000, 0)
	scope := continuationDigestV1("scope")
	runID := core.AgentRunID("run-1")
	sessionDigest := continuationDigestV1("session")
	turnDigest := continuationDigestV1("turn-7")
	checked := now.Add(-time.Second).UnixNano()
	expires := now.Add(time.Minute).UnixNano()
	source, err := SealContextTurnSourceCurrentV1(ContextTurnSourceCurrentV1{
		ExecutionScopeDigest: scope,
		RunID:                runID,
		Session:              SingleCallSessionCoordinateV1{ID: "session-1", Revision: 4, Digest: sessionDigest, Phase: SingleCallSessionWaitingActionV1, CheckedUnixNano: checked, ExpiresUnixNano: expires},
		SessionApplicability: SingleCallSessionApplicabilitySourceCoordinateV1{Kind: SingleCallSessionSourceKindV1, ID: "session:" + string(sessionDigest), Revision: 4, Digest: sessionDigest},
		Turn:                 SingleCallTurnCoordinateV1{ID: "turn:" + string(turnDigest), Ordinal: 7, Revision: 7, Digest: turnDigest},
		TurnApplicability:    SingleCallTurnApplicabilitySourceCoordinateV1{Kind: SingleCallTurnSourceKindV1, ID: "turn:" + string(turnDigest), Revision: 7, Digest: turnDigest},
		CheckedUnixNano:      checked,
		ExpiresUnixNano:      expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := SealHarnessActiveContextRefV1(HarnessActiveContextRefV1{
		Revision:                 11,
		ExecutionScopeDigest:     scope,
		RunID:                    runID,
		SessionID:                source.Session.ID,
		TurnOrdinal:              source.Turn.Ordinal,
		ManifestRef:              continuationExactRefV1("context/manifest", "manifest-7"),
		FrameRef:                 continuationExactRefV1("context/frame", "frame-7"),
		GenerationRef:            continuationExactRefV1("context/generation", "generation-7"),
		ContextCurrentPointerRef: continuationExactRefV1("context/generation-current", "current-7"),
		UpdatedUnixNano:          now.Add(-2 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	tool := SingleCallToolActionResultRefV2{
		ID: "single-call-result-1", Revision: 1, Digest: continuationDigestV1("single-call-result"),
		RequestID: "single-call-request-1", RequestRevision: 1, RequestDigest: continuationDigestV1("single-call-request"),
		ActionCoordinateDigest: continuationDigestV1("action-coordinate"), ToolResultID: "tool-result-1", ToolResultRevision: 3, ToolResultDigest: continuationDigestV1("tool-result"),
	}
	startInput := TurnContinuationStartRequestV1{
		ExecutionScopeDigest: scope, RunID: runID, Source: source, SettledToolResult: tool, ExpectedActiveContext: active,
		TargetTurn: source.Turn.Ordinal + 1, RequestedNotAfterUnixNano: now.Add(30 * time.Second).UnixNano(),
	}
	attemptID, err := DeriveTurnContinuationAttemptIDV1(startInput)
	if err != nil {
		t.Fatal(err)
	}
	memoryItem, err := SealContextOwnerSourceItemV1(ContextOwnerSourceItemV1{
		Rank: 0, ItemDigest: continuationDigestV1("memory-item"), RecordRef: continuationExactRefV1("memory/record", "memory-record"),
		StableOwnerChain: []ContextRefreshExactRefV1{continuationExactRefV1("memory/source", "memory-source")},
		ContentRef:       ContextOwnerContentRefV1{ID: "memory-content", Digest: continuationDigestV1("memory-content"), Length: 12, MediaType: "text/plain"},
		TokenEstimate:    3, Sensitivity: "internal", CitationDigest: continuationDigestV1("memory-citation"), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	memory, err := SealContextOwnerSourceEnvelopeV1(ContextOwnerSourceEnvelopeV1{
		ID: "memory-envelope", Owner: ContextOwnerMemoryV1, SourceSession: source.Session, SessionApplicability: source.SessionApplicability, SourceTurn: source.Turn, TurnApplicability: source.TurnApplicability,
		AttemptInspectionRef: continuationExactRefV1("memory/inspection", "memory-inspection"), CurrentProjectionRef: continuationExactRefV1("memory/projection", "memory-projection"), StableClosureDigest: continuationDigestV1("memory-closure"),
		Items: []ContextOwnerSourceItemV1{memoryItem}, Phase: ContextSourceCheckS1V1, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	memoryRequest, err := SealContextOwnerSourceRequestV1(ContextOwnerSourceRequestV1{
		Owner: ContextOwnerMemoryV1, SourceSession: source.Session, SessionApplicability: source.SessionApplicability, SourceTurn: source.Turn, TurnApplicability: source.TurnApplicability,
		OwnerRequest: []byte(`{"owner":"memory"}`), Phase: ContextSourceCheckS1V1, RequestedNotAfterNano: now.Add(30 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	prepare, err := SealContextTurnRefreshPrepareRequestV1(ContextTurnRefreshPrepareRequestV1{
		ID: attemptID, ExecutionScopeDigest: scope, RunID: runID, SourceSession: source.Session, SessionApplicability: source.SessionApplicability,
		SourceTurn: source.Turn, TurnApplicability: source.TurnApplicability, ExpectedTargetTurn: source.Turn.Ordinal + 1,
		OpaqueContextRequest: []byte(`{"turn":"next"}`), MemoryRequest: &memoryRequest, Memory: &memory, RequestedNotAfterNano: now.Add(30 * time.Second).UnixNano(),
	})
	if err != nil || prepare.ValidateCurrent(now) != nil {
		t.Fatalf("seal real Context prepare: %#v %v", prepare, err)
	}
	startInput.ExpectedContextRefreshAttempt, err = prepare.AttemptRefV1()
	if err != nil {
		t.Fatal(err)
	}
	start, err := SealTurnContinuationStartRequestV1(startInput)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := SealTurnContinuationPendingV1(start, now.UnixNano(), now.Add(25*time.Second).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	applySettlement := continuationExactRefV1("context/apply-settlement", "apply-8")
	currentPointer := continuationExactRefV1("context/generation-current", "current-8")
	refresh, err := SealContextTurnRefreshResultV1(ContextTurnRefreshResultV1{
		AttemptRef:             start.ExpectedContextRefreshAttempt,
		PendingDomainResultRef: continuationExactRefV1("context/pending-result", "pending-8"),
		TransitionProofRef:     continuationExactRefV1("context/transition-proof", "proof-8"),
		ManifestRef:            continuationExactRefV1("context/manifest", "manifest-8"),
		FrameRef:               continuationExactRefV1("context/frame", "frame-8"),
		GenerationRef:          continuationExactRefV1("context/generation", "generation-8"),
		StableSourceSetDigest:  continuationDigestV1("stable-source"),
		S1AssociationSetDigest: continuationDigestV1("s1-association"),
		S2AssociationSetDigest: continuationDigestV1("s2-association"),
		ApplySettlementRef:     &applySettlement,
		CurrentPointerRef:      &currentPointer,
		CheckedUnixNano:        now.Add(time.Second).UnixNano(),
		ExpiresUnixNano:        now.Add(20 * time.Second).UnixNano(),
		State:                  ContextTurnRefreshAppliedStateV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	commitNow := now.Add(2 * time.Second)
	commit, err := SealTurnContinuationCommitRequestV1(TurnContinuationCommitRequestV1{
		Pending: pending, AppliedContextRefresh: refresh, RequestedNotAfterUnixNano: now.Add(15 * time.Second).UnixNano(),
	}, commitNow)
	if err != nil {
		t.Fatal(err)
	}
	current, err := SealTurnContinuationContextCurrentV1(commit, commitNow.UnixNano(), now.Add(10*time.Second).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	return turnContinuationFixtureV1{now: now, prepare: prepare, start: start, pending: pending, refresh: refresh, commit: commit, current: current}
}

func TestTurnContinuationPendingThenExactActiveContextCASV1(t *testing.T) {
	fixture := newTurnContinuationFixtureV1(t)
	prepareRef, err := fixture.prepare.AttemptRefV1()
	if err != nil || prepareRef != fixture.start.ExpectedContextRefreshAttempt {
		t.Fatalf("Begin did not freeze the sealed Context prepare exact ref: %#v %v", prepareRef, err)
	}
	if fixture.pending.State != TurnContinuationPendingV1 || fixture.pending.ActiveContext != fixture.start.ExpectedActiveContext {
		t.Fatalf("pending state changed ActiveContext: %#v", fixture.pending)
	}
	if err := fixture.pending.ModelTurnAllowedV1(fixture.now.Add(time.Second)); err == nil {
		t.Fatal("continuation_pending allowed the next Model Turn")
	}
	if err := fixture.current.ModelTurnAllowedV1(fixture.now.Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if fixture.current.ActiveContext.Revision != fixture.start.ExpectedActiveContext.Revision+1 || fixture.current.ActiveContext.TurnOrdinal != fixture.start.TargetTurn || fixture.current.ActiveContext.FrameRef != fixture.refresh.FrameRef || fixture.current.PreviousDigest != fixture.pending.Digest {
		t.Fatalf("ActiveContext CAS lost exact target refs: %#v", fixture.current)
	}
	if fixture.start.ExpectedActiveContext.FrameRef == fixture.current.ActiveContext.FrameRef {
		t.Fatal("target ContextFrame was not switched")
	}
}

func TestTurnContinuationRejectsSplicedOrPreparedOnlyContextV1(t *testing.T) {
	fixture := newTurnContinuationFixtureV1(t)
	tampered := fixture.current.Clone()
	tampered.ActiveContext.FrameRef = continuationExactRefV1("context/frame", "spliced-frame")
	tampered.ActiveContext.Digest = ""
	var err error
	tampered.ActiveContext, err = SealHarnessActiveContextRefV1(tampered.ActiveContext)
	if err != nil {
		t.Fatal(err)
	}
	tampered.Digest = ""
	tampered.Digest, err = tampered.DigestV1()
	if err != nil {
		t.Fatal(err)
	}
	if err := tampered.ValidateCurrent(fixture.now.Add(3 * time.Second)); err == nil {
		t.Fatal("a valid but spliced ContextFrame was accepted")
	}

	prepared := fixture.refresh
	prepared.State = ContextTurnRefreshPreparedStateV1
	prepared.ApplySettlementRef = nil
	prepared.CurrentPointerRef = nil
	prepared.S2AssociationSetDigest = ""
	prepared.Digest = ""
	prepared, err = SealContextTurnRefreshResultV1(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = SealTurnContinuationCommitRequestV1(TurnContinuationCommitRequestV1{Pending: fixture.pending, AppliedContextRefresh: prepared, RequestedNotAfterUnixNano: fixture.now.Add(15 * time.Second).UnixNano()}, fixture.now.Add(2*time.Second)); err == nil {
		t.Fatal("prepared_pending Context result was accepted for ActiveContext CAS")
	}

	splicedAttempt := fixture.refresh
	splicedAttempt.AttemptRef.Digest = continuationDigestV1("same-id-different-prepare")
	splicedAttempt.Digest = ""
	splicedAttempt, err = SealContextTurnRefreshResultV1(splicedAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = SealTurnContinuationCommitRequestV1(TurnContinuationCommitRequestV1{Pending: fixture.pending, AppliedContextRefresh: splicedAttempt, RequestedNotAfterUnixNano: fixture.now.Add(15 * time.Second).UnixNano()}, fixture.now.Add(2*time.Second)); err == nil {
		t.Fatal("same Context Attempt ID/revision with a different exact digest was accepted")
	}
}

func TestTurnContinuationCurrentnessAndOriginalInspectV1(t *testing.T) {
	fixture := newTurnContinuationFixtureV1(t)
	if err := fixture.start.ValidateCurrent(time.Unix(0, fixture.start.RequestedNotAfterUnixNano)); err == nil {
		t.Fatal("start request remained current at its exclusive deadline")
	}
	if err := fixture.pending.ValidateCurrent(time.Unix(0, fixture.pending.ExpiresUnixNano)); err == nil {
		t.Fatal("pending current remained current at its exclusive deadline")
	}
	if err := fixture.current.ModelTurnAllowedV1(time.Unix(0, fixture.current.ExpiresUnixNano)); err == nil {
		t.Fatal("expired context_current allowed a Model Turn")
	}
	if _, err := SealTurnContinuationContextCurrentV1(fixture.commit, fixture.now.Add(2*time.Second).UnixNano(), fixture.commit.RequestedNotAfterUnixNano+1); err == nil {
		t.Fatal("context_current exceeded its exact Commit currentness bound")
	}
	inspect, err := SealTurnContinuationInspectRequestV1(TurnContinuationInspectRequestV1{AttemptRef: fixture.start.AttemptRefV1()})
	if err != nil || inspect.AttemptRef.ID != fixture.start.AttemptID || inspect.AttemptRef.Digest != fixture.start.Digest {
		t.Fatalf("original Attempt Inspect is not exact: %#v %v", inspect, err)
	}

	rebounded := fixture.start
	rebounded.AttemptID = ""
	rebounded.Digest = ""
	rebounded.RequestedNotAfterUnixNano--
	rebounded, err = SealTurnContinuationStartRequestV1(rebounded)
	if err != nil {
		t.Fatal(err)
	}
	if rebounded.AttemptID != fixture.start.AttemptID || rebounded.Digest == fixture.start.Digest {
		t.Fatal("same semantic Attempt did not retain its ID while request currentness changed")
	}
}

func TestTurnContinuationCanonicalSealIsRaceSafeV1(t *testing.T) {
	fixture := newTurnContinuationFixtureV1(t)
	const workers = 64
	digests := make(chan core.Digest, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			sealed, err := SealTurnContinuationContextCurrentV1(fixture.commit, fixture.now.Add(2*time.Second).UnixNano(), fixture.now.Add(10*time.Second).UnixNano())
			if err != nil {
				errs <- err
				return
			}
			digests <- sealed.Digest
		}()
	}
	wait.Wait()
	close(digests)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	for digest := range digests {
		if digest != fixture.current.Digest {
			t.Fatalf("concurrent seal returned %s, want %s", digest, fixture.current.Digest)
		}
	}
}
