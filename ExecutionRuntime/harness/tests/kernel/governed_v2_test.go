package kernel_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestGovernedLoopV2PreparesOnlyCandidateAndRecoversEveryLostReply(t *testing.T) {
	for _, fault := range []string{"none", "session_create", "candidate_create", "session_cas"} {
		t.Run(fault, func(t *testing.T) {
			now := time.Unix(1_800_000_000, 0)
			store := fakes.NewGovernedStoreV2()
			store.Clock = func() time.Time { return now }
			switch fault {
			case "session_create":
				store.LoseNextSessionCreateReply = true
			case "candidate_create":
				store.LoseNextCandidateCreateReply = true
			case "session_cas":
				store.LoseNextSessionCASReply = true
			}
			loop, err := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: store, Candidates: store, Clock: func() time.Time { return now }, CandidateTTL: time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			_, candidate := testkit.GovernedFactsV2(now)
			request := prepareRequestV2(candidate)
			result, err := loop.PrepareInitialCandidateV2(context.Background(), request)
			if err != nil {
				t.Fatalf("prepare failed at %s: %v", fault, err)
			}
			if result.Session.Phase != contract.SessionWaitingModelDispatchV2 || result.Session.Revision != 2 || result.Candidate.ID != candidate.ID {
				t.Fatalf("unexpected preparation result: %#v", result)
			}
			// A fresh coordinator may replay only the identical preallocated facts.
			restarted, err := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: store, Candidates: store, Clock: func() time.Time { return now }, CandidateTTL: time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			replayed, err := restarted.PrepareInitialCandidateV2(context.Background(), request)
			if err != nil {
				t.Fatalf("restart replay failed: %v", err)
			}
			if replayed.Session.Revision != 2 || replayed.Session.Phase != contract.SessionWaitingModelDispatchV2 {
				t.Fatalf("restart changed session: %#v", replayed.Session)
			}
		})
	}
}

func TestGovernedLoopV2ActionContinuationConsumesExactPendingOnce(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	store := fakes.NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	loop, _ := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: store, Candidates: store, Clock: func() time.Time { return now }, CandidateTTL: time.Minute})
	_, initial := testkit.GovernedFactsV2(now)
	prepared, err := loop.PrepareInitialCandidateV2(context.Background(), prepareRequestV2(initial))
	if err != nil {
		t.Fatal(err)
	}
	current := prepared.Session
	ref, _ := initial.RefV2()
	reserved, _ := reserveSessionForKernelTestV3(t, store, current, ref, now)
	inflight := reserved
	inflight.Revision, inflight.Phase, inflight.UpdatedUnixNano = reserved.Revision+1, contract.SessionModelInFlightV2, now.Add(time.Second).UnixNano()
	preparedAttempt := testkit.GovernedAttemptRefsV2(now, initial, "")
	inflight.Execution = &preparedAttempt
	if _, err := store.CompareAndSwapSessionV2(context.Background(), harnessports.SessionCASRequestV2{ExpectedRevision: reserved.Revision, Next: inflight}); err != nil {
		t.Fatal(err)
	}
	pending, err := contract.NewPendingActionV2("action-1", "custom/tool", initial.Input, ref)
	if err != nil {
		t.Fatal(err)
	}
	state := &kernel.GovernedTurnStateCoordinatorV2{Sessions: store}
	observedAttempt := testkit.GovernedAttemptRefsV2(now, initial, runtimeports.ProviderAttemptObservedV2)
	waitingSettlement, err := state.AttachObservedAttemptV2(context.Background(), harnessports.AttachObservedAttemptRequestV2{Run: initial.Run, SessionID: inflight.ID, ExpectedSessionRevision: inflight.Revision, Attempt: observedAttempt, UpdatedUnixNano: now.Add(2 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	turn := contract.SettledTurnResultV2{ContractVersion: contract.SettledTurnResultContractV2, Candidate: ref, State: contract.SettledTurnActionRequiredV2, Action: &pending}
	settledAttempt, domain := testkit.GovernedSettledAttemptRefsV2(now, initial, turn, runtimeports.OperationSettlementAppliedV3)
	waiting, err := state.ApplySettledTurnV2(context.Background(), harnessports.ApplySettledTurnRequestV2{Run: initial.Run, SessionID: waitingSettlement.ID, ExpectedSessionRevision: waitingSettlement.Revision, Attempt: settledAttempt, DomainResult: domain, UpdatedUnixNano: now.Add(3 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	continuation := contract.ContinuationRefV2{Kind: contract.CandidateActionTurnV2, PendingRef: pending.Ref, PendingDigest: pending.RequestDigest, SettlementRef: "tool-settlement-1", SettlementDigest: testkit.Digest("tool-settlement"), EvidenceRef: "tool-evidence-1", EvidenceDigest: testkit.Digest("tool-evidence")}
	request := kernel.PrepareContinuationCandidateRequestV2{Run: initial.Run, SessionID: waiting.ID, ExpectedSessionRevision: waiting.Revision, CandidateID: "candidate-2", Input: initial.Input, ContextRef: initial.ContextRef, ContextDigest: initial.ContextDigest, Continuation: continuation, Provider: initial.Provider, CreatedUnixNano: now.Add(4 * time.Second).UnixNano(), ExpiresUnixNano: now.Add(64 * time.Second).UnixNano()}
	store.LoseNextCandidateCreateReply = true
	store.LoseNextSessionCASReply = true
	result, err := loop.PrepareContinuationCandidateV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Session.Phase != contract.SessionWaitingModelDispatchV2 || result.Session.Turn != 2 || result.Session.PendingAction != nil || result.Candidate.Kind != contract.CandidateActionTurnV2 {
		t.Fatalf("continuation did not consume pending action: %#v", result)
	}
	// Restart/replay returns the exact facts and does not consume twice.
	replayed, err := loop.PrepareContinuationCandidateV2(context.Background(), request)
	if err != nil || replayed.Session.Revision != waiting.Revision+1 {
		t.Fatalf("continuation replay changed state: %#v err=%v", replayed, err)
	}
	changed := request
	changed.Continuation.SettlementDigest = testkit.Digest("different-settlement")
	if _, err := loop.PrepareContinuationCandidateV2(context.Background(), changed); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("settlement swap was accepted: %v", err)
	}
}

func TestGovernedLoopV2RejectsWrongPendingContinuation(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	store := fakes.NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	loop, _ := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: store, Candidates: store, Clock: func() time.Time { return now }, CandidateTTL: time.Minute})
	_, initial := testkit.GovernedFactsV2(now)
	prepared, _ := loop.PrepareInitialCandidateV2(context.Background(), prepareRequestV2(initial))
	inflight := prepared.Session
	inflight.Revision, inflight.Phase, inflight.UpdatedUnixNano = 3, contract.SessionModelInFlightV2, now.Add(time.Second).UnixNano()
	preparedAttempt := testkit.GovernedAttemptRefsV2(now, initial, "")
	inflight.Execution = &preparedAttempt
	_, _ = store.CompareAndSwapSessionV2(context.Background(), harnessports.SessionCASRequestV2{ExpectedRevision: 2, Next: inflight})
	ref, _ := initial.RefV2()
	pending, _ := contract.NewPendingActionV2("action-1", "custom/tool", initial.Input, ref)
	state := &kernel.GovernedTurnStateCoordinatorV2{Sessions: store}
	observedAttempt := testkit.GovernedAttemptRefsV2(now, initial, runtimeports.ProviderAttemptObservedV2)
	waitingSettlement, _ := state.AttachObservedAttemptV2(context.Background(), harnessports.AttachObservedAttemptRequestV2{Run: initial.Run, SessionID: inflight.ID, ExpectedSessionRevision: inflight.Revision, Attempt: observedAttempt, UpdatedUnixNano: now.Add(2 * time.Second).UnixNano()})
	turn := contract.SettledTurnResultV2{ContractVersion: contract.SettledTurnResultContractV2, Candidate: ref, State: contract.SettledTurnActionRequiredV2, Action: &pending}
	settledAttempt, domain := testkit.GovernedSettledAttemptRefsV2(now, initial, turn, runtimeports.OperationSettlementAppliedV3)
	waiting, _ := state.ApplySettledTurnV2(context.Background(), harnessports.ApplySettledTurnRequestV2{Run: initial.Run, SessionID: waitingSettlement.ID, ExpectedSessionRevision: waitingSettlement.Revision, Attempt: settledAttempt, DomainResult: domain, UpdatedUnixNano: now.Add(3 * time.Second).UnixNano()})
	request := kernel.PrepareContinuationCandidateRequestV2{Run: initial.Run, SessionID: waiting.ID, ExpectedSessionRevision: waiting.Revision, CandidateID: "candidate-2", Input: initial.Input, ContextRef: initial.ContextRef, ContextDigest: initial.ContextDigest, Continuation: contract.ContinuationRefV2{Kind: contract.CandidateActionTurnV2, PendingRef: "action-other", PendingDigest: pending.RequestDigest, SettlementRef: "settlement-1", SettlementDigest: testkit.Digest("settlement"), EvidenceRef: "evidence-1", EvidenceDigest: testkit.Digest("evidence")}, Provider: initial.Provider, CreatedUnixNano: now.Add(4 * time.Second).UnixNano(), ExpiresUnixNano: now.Add(64 * time.Second).UnixNano()}
	if _, err := loop.PrepareContinuationCandidateV2(context.Background(), request); err == nil {
		t.Fatal("wrong pending action ref was accepted")
	}
}

func TestGovernedLoopV2RejectsConflictingCandidateDuringRecovery(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	store := fakes.NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	session, candidate := testkit.GovernedFactsV2(now)
	if _, err := store.CreateSessionV2(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	conflict := candidate
	conflict.ContextRef = "context-conflict"
	if _, err := store.CreateCandidateV2(context.Background(), conflict); err != nil {
		t.Fatal(err)
	}
	loop, err := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: store, Candidates: store, Clock: func() time.Time { return now }, CandidateTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loop.PrepareInitialCandidateV2(context.Background(), prepareRequestV2(candidate)); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("conflicting candidate was accepted: %v", err)
	}
}

func TestGovernedLoopV2RequiresPreallocatedStableLifetime(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	store := fakes.NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	loop, _ := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: store, Candidates: store, Clock: func() time.Time { return now }, CandidateTTL: time.Minute})
	_, candidate := testkit.GovernedFactsV2(now)
	request := prepareRequestV2(candidate)
	request.ExpiresUnixNano = now.Add(2 * time.Minute).UnixNano()
	if _, err := loop.PrepareInitialCandidateV2(context.Background(), request); err == nil {
		t.Fatal("unbounded candidate lifetime accepted")
	}
	request = prepareRequestV2(candidate)
	request.ExpiresUnixNano = now.UnixNano()
	if _, err := loop.PrepareInitialCandidateV2(context.Background(), request); err == nil {
		t.Fatal("expired candidate accepted")
	}
}

func TestGovernedLoopV2ContinuationValidatesBeforeSessionBackend(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	sessions := &countingSessionPortV2{}
	loop, err := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: sessions, Candidates: fakes.NewGovernedStoreV2(), Clock: func() time.Time { return now }, CandidateTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loop.PrepareContinuationCandidateV2(context.Background(), kernel.PrepareContinuationCandidateRequestV2{}); err == nil {
		t.Fatal("zero continuation request reached the session backend")
	}
	if sessions.calls != 0 {
		t.Fatalf("invalid continuation touched session backend %d times", sessions.calls)
	}
}

type countingSessionPortV2 struct{ calls int }

func (p *countingSessionPortV2) CreateSessionV2(context.Context, contract.GovernedSessionV2) (contract.GovernedSessionV2, error) {
	p.calls++
	return contract.GovernedSessionV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "unexpected backend call")
}

func (p *countingSessionPortV2) InspectSessionV2(context.Context, contract.RunRef, string) (contract.GovernedSessionV2, error) {
	p.calls++
	return contract.GovernedSessionV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "unexpected backend call")
}

func (p *countingSessionPortV2) CompareAndSwapSessionV2(context.Context, harnessports.SessionCASRequestV2) (contract.GovernedSessionV2, error) {
	p.calls++
	return contract.GovernedSessionV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "unexpected backend call")
}

func prepareRequestV2(candidate contract.ModelTurnCandidateV2) kernel.PrepareInitialCandidateRequestV2 {
	return kernel.PrepareInitialCandidateRequestV2{Run: candidate.Run, Endpoint: candidate.Endpoint, SessionID: candidate.SessionRef, CandidateID: candidate.ID, Input: candidate.Input, ContextRef: candidate.ContextRef, ContextDigest: candidate.ContextDigest, Provider: candidate.Provider, CreatedUnixNano: candidate.CreatedUnixNano, ExpiresUnixNano: candidate.ExpiresUnixNano}
}
