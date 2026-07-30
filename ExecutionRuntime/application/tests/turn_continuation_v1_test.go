package application_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var _ applicationports.TurnContinuationPortV1 = (*blackboxTurnContinuationOwnerV1)(nil)

type blackboxTurnContinuationOwnerV1 struct {
	mu              sync.Mutex
	now             time.Time
	current         contract.TurnContinuationCurrentV1
	present         bool
	loseCommitReply bool
	beginCalls      uint32
	commitCalls     uint32
	inspectCalls    uint32
}

func (o *blackboxTurnContinuationOwnerV1) BeginTurnContinuationV1(_ context.Context, start contract.TurnContinuationStartRequestV1) (contract.TurnContinuationCurrentV1, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.beginCalls++
	if o.present {
		return contract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Attempt already exists; Inspect original")
	}
	pending, err := contract.SealTurnContinuationPendingV1(start, o.now.UnixNano(), o.now.Add(24*time.Second).UnixNano())
	if err != nil {
		return contract.TurnContinuationCurrentV1{}, err
	}
	o.current, o.present = pending, true
	return pending.Clone(), nil
}

func (o *blackboxTurnContinuationOwnerV1) CommitTurnContinuationV1(_ context.Context, request contract.TurnContinuationCommitRequestV1) (contract.TurnContinuationCurrentV1, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.commitCalls++
	if !o.present || o.current.Digest != request.Pending.Digest || o.current.ActiveContext != request.ExpectedActiveContext {
		return contract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "pending or ActiveContext CAS conflict")
	}
	next, err := contract.SealTurnContinuationContextCurrentV1(request, o.now.UnixNano(), o.now.Add(8*time.Second).UnixNano())
	if err != nil {
		return contract.TurnContinuationCurrentV1{}, err
	}
	o.current = next
	if o.loseCommitReply {
		o.loseCommitReply = false
		return contract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected lost Commit reply")
	}
	return next.Clone(), nil
}

func (o *blackboxTurnContinuationOwnerV1) InspectTurnContinuationV1(_ context.Context, request contract.TurnContinuationInspectRequestV1) (contract.TurnContinuationCurrentV1, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.inspectCalls++
	if !o.present || request.Validate() != nil || request.AttemptRef != o.current.Start.AttemptRefV1() {
		return contract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "TurnContinuation Attempt not found")
	}
	return o.current.Clone(), nil
}

func TestTurnContinuationBlackBoxLostCommitReplyInspectsOriginalAttemptV1(t *testing.T) {
	base := time.Unix(1_810_010_000, 0)
	start := blackboxTurnContinuationStartV1(t, base)
	owner := &blackboxTurnContinuationOwnerV1{now: base.Add(time.Second), loseCommitReply: true}
	pending, err := owner.BeginTurnContinuationV1(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	if pending.State != contract.TurnContinuationPendingV1 || pending.ModelTurnAllowedV1(owner.now) == nil {
		t.Fatal("Begin did not fail-close at continuation_pending")
	}
	refresh := blackboxAppliedContextRefreshV1(t, start, base)
	commit, err := contract.SealTurnContinuationCommitRequestV1(contract.TurnContinuationCommitRequestV1{
		Pending: pending, AppliedContextRefresh: refresh, RequestedNotAfterUnixNano: base.Add(12 * time.Second).UnixNano(),
	}, owner.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.CommitTurnContinuationV1(context.Background(), commit); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("Commit did not report injected unknown outcome: %v", err)
	}
	inspect, err := contract.SealTurnContinuationInspectRequestV1(contract.TurnContinuationInspectRequestV1{AttemptRef: start.AttemptRefV1()})
	if err != nil {
		t.Fatal(err)
	}
	current, err := owner.InspectTurnContinuationV1(context.Background(), inspect)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.ModelTurnAllowedV1(base.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if current.ActiveContext.FrameRef != refresh.FrameRef || current.ActiveContext.Revision != start.ExpectedActiveContext.Revision+1 || current.PreviousDigest != pending.Digest {
		t.Fatalf("Inspect lost the exact ActiveContext CAS: %#v", current)
	}
	if owner.beginCalls != 1 || owner.commitCalls != 1 || owner.inspectCalls != 1 {
		t.Fatalf("lost reply was redispatched: begin=%d commit=%d inspect=%d", owner.beginCalls, owner.commitCalls, owner.inspectCalls)
	}
}

func blackboxTurnContinuationStartV1(t *testing.T, now time.Time) contract.TurnContinuationStartRequestV1 {
	t.Helper()
	digest := func(seed string) core.Digest { return core.DigestBytes([]byte(seed)) }
	exact := func(kind, id string) contract.ContextRefreshExactRefV1 {
		return contract.ContextRefreshExactRefV1{Kind: runtimeports.NamespacedNameV2(kind), ID: id, Revision: 1, Digest: digest(id)}
	}
	scope, runID := digest("blackbox-scope"), core.AgentRunID("blackbox-run")
	sessionDigest, turnDigest := digest("blackbox-session"), digest("blackbox-turn")
	checked, expires := now.Add(-time.Second).UnixNano(), now.Add(time.Minute).UnixNano()
	source, err := contract.SealContextTurnSourceCurrentV1(contract.ContextTurnSourceCurrentV1{
		ExecutionScopeDigest: scope, RunID: runID,
		Session:              contract.SingleCallSessionCoordinateV1{ID: "blackbox-session", Revision: 2, Digest: sessionDigest, Phase: contract.SingleCallSessionWaitingActionV1, CheckedUnixNano: checked, ExpiresUnixNano: expires},
		SessionApplicability: contract.SingleCallSessionApplicabilitySourceCoordinateV1{Kind: contract.SingleCallSessionSourceKindV1, ID: "session:" + string(sessionDigest), Revision: 2, Digest: sessionDigest},
		Turn:                 contract.SingleCallTurnCoordinateV1{ID: "turn:" + string(turnDigest), Ordinal: 3, Revision: 3, Digest: turnDigest},
		TurnApplicability:    contract.SingleCallTurnApplicabilitySourceCoordinateV1{Kind: contract.SingleCallTurnSourceKindV1, ID: "turn:" + string(turnDigest), Revision: 3, Digest: turnDigest},
		CheckedUnixNano:      checked, ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := contract.SealHarnessActiveContextRefV1(contract.HarnessActiveContextRefV1{
		Revision: 5, ExecutionScopeDigest: scope, RunID: runID, SessionID: source.Session.ID, TurnOrdinal: source.Turn.Ordinal,
		ManifestRef: exact("context/manifest", "blackbox-manifest-3"), FrameRef: exact("context/frame", "blackbox-frame-3"), GenerationRef: exact("context/generation", "blackbox-generation-3"), ContextCurrentPointerRef: exact("context/generation-current", "blackbox-current-3"), UpdatedUnixNano: now.Add(-2 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	startInput := contract.TurnContinuationStartRequestV1{
		ExecutionScopeDigest: scope, RunID: runID, Source: source,
		SettledToolResult:     contract.SingleCallToolActionResultRefV2{ID: "blackbox-result", Revision: 1, Digest: digest("blackbox-result"), RequestID: "blackbox-request", RequestRevision: 1, RequestDigest: digest("blackbox-request"), ActionCoordinateDigest: digest("blackbox-action"), ToolResultID: "blackbox-tool-result", ToolResultRevision: 1, ToolResultDigest: digest("blackbox-tool-result")},
		ExpectedActiveContext: active, TargetTurn: source.Turn.Ordinal + 1, RequestedNotAfterUnixNano: now.Add(30 * time.Second).UnixNano(),
	}
	attemptID, err := contract.DeriveTurnContinuationAttemptIDV1(startInput)
	if err != nil {
		t.Fatal(err)
	}
	startInput.ExpectedContextRefreshAttempt = contract.ContextRefreshExactRefV1{Kind: contract.ContextTurnRefreshApplicationAttemptKindV1, ID: attemptID, Revision: 1, Digest: digest("blackbox-context-attempt")}
	start, err := contract.SealTurnContinuationStartRequestV1(startInput)
	if err != nil {
		t.Fatal(err)
	}
	return start
}

func blackboxAppliedContextRefreshV1(t *testing.T, start contract.TurnContinuationStartRequestV1, now time.Time) contract.ContextTurnRefreshResultV1 {
	t.Helper()
	digest := func(seed string) core.Digest { return core.DigestBytes([]byte(seed)) }
	exact := func(kind, id string) contract.ContextRefreshExactRefV1 {
		return contract.ContextRefreshExactRefV1{Kind: runtimeports.NamespacedNameV2(kind), ID: id, Revision: 1, Digest: digest(id)}
	}
	settlement, pointer := exact("context/apply-settlement", "blackbox-apply-4"), exact("context/generation-current", "blackbox-current-4")
	result, err := contract.SealContextTurnRefreshResultV1(contract.ContextTurnRefreshResultV1{
		AttemptRef:             start.ExpectedContextRefreshAttempt,
		PendingDomainResultRef: exact("context/pending-result", "blackbox-pending-4"), TransitionProofRef: exact("context/transition-proof", "blackbox-proof-4"), ManifestRef: exact("context/manifest", "blackbox-manifest-4"), FrameRef: exact("context/frame", "blackbox-frame-4"), GenerationRef: exact("context/generation", "blackbox-generation-4"),
		StableSourceSetDigest: digest("blackbox-stable"), S1AssociationSetDigest: digest("blackbox-s1"), S2AssociationSetDigest: digest("blackbox-s2"), ApplySettlementRef: &settlement, CurrentPointerRef: &pointer,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(18 * time.Second).UnixNano(), State: contract.ContextTurnRefreshAppliedStateV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
