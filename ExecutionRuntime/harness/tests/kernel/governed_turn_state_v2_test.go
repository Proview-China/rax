package kernel_test

import (
	"context"
	"sync"
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

func TestGovernedTurnStateV2AttachesOnlyExactRuntimeFactsAndRecoversLostCAS(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	store := fakes.NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	loop, _ := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: store, Candidates: store, Clock: func() time.Time { return now }, CandidateTTL: time.Minute})
	_, candidate := testkit.GovernedFactsV2(now)
	preparedCandidate, err := loop.PrepareInitialCandidateV2(context.Background(), prepareRequestV2(candidate))
	if err != nil {
		t.Fatal(err)
	}
	state := &kernel.GovernedTurnStateCoordinatorV2{Sessions: store}
	candidateRef, _ := candidate.RefV2()
	reserved, reservation := reserveSessionForKernelTestV3(t, store, preparedCandidate.Session, candidateRef, now)
	attempt := testkit.GovernedAttemptRefsV2(now, candidate, "")
	store.LoseNextSessionCASReply = true
	inflight, err := state.AttachPreparedAttemptV2(context.Background(), harnessports.AttachPreparedAttemptRequestV2{Run: candidate.Run, SessionID: reserved.ID, ExpectedSessionRevision: reserved.Revision, Candidate: candidateRef, Reservation: reservation, Attempt: attempt, UpdatedUnixNano: now.Add(time.Second).UnixNano()})
	if err != nil || inflight.Phase != contract.SessionModelInFlightV2 || inflight.Revision != reserved.Revision+1 {
		t.Fatalf("prepared Runtime refs were not attached exactly: %#v err=%v", inflight, err)
	}
	replayed, err := state.AttachPreparedAttemptV2(context.Background(), harnessports.AttachPreparedAttemptRequestV2{Run: candidate.Run, SessionID: reserved.ID, ExpectedSessionRevision: reserved.Revision, Candidate: candidateRef, Reservation: reservation, Attempt: attempt, UpdatedUnixNano: now.Add(time.Second).UnixNano()})
	if err != nil || replayed.Revision != reserved.Revision+1 {
		t.Fatalf("prepared attachment replay changed state: %#v err=%v", replayed, err)
	}

	observed := testkit.GovernedAttemptRefsV2(now, candidate, runtimeports.ProviderAttemptObservedV2)
	pending, err := contract.NewPendingActionV2("action-governed", "custom.tool/execute", candidate.Input, candidateRef)
	if err != nil {
		t.Fatal(err)
	}
	store.LoseNextSessionCASReply = true
	waitingSettlement, err := state.AttachObservedAttemptV2(context.Background(), harnessports.AttachObservedAttemptRequestV2{Run: candidate.Run, SessionID: inflight.ID, ExpectedSessionRevision: inflight.Revision, Attempt: observed, UpdatedUnixNano: now.Add(2 * time.Second).UnixNano()})
	if err != nil || waitingSettlement.Phase != contract.SessionWaitingSettlementV2 || waitingSettlement.Revision != inflight.Revision+1 || waitingSettlement.Execution == nil || waitingSettlement.Execution.Observation == nil {
		t.Fatalf("observed Runtime refs were not attached exactly: %#v err=%v", waitingSettlement, err)
	}
	forged := observed
	forged.PermitDigest = core.DigestBytes([]byte("other-permit"))
	if _, err := state.AttachObservedAttemptV2(context.Background(), harnessports.AttachObservedAttemptRequestV2{Run: candidate.Run, SessionID: inflight.ID, ExpectedSessionRevision: inflight.Revision, Attempt: forged, UpdatedUnixNano: now.Add(2 * time.Second).UnixNano()}); err == nil {
		t.Fatal("attempt sidecar drift was accepted")
	}
	turn := contract.SettledTurnResultV2{ContractVersion: contract.SettledTurnResultContractV2, Candidate: candidateRef, State: contract.SettledTurnActionRequiredV2, Action: &pending}
	settled, domain := testkit.GovernedSettledAttemptRefsV2(now, candidate, turn, runtimeports.OperationSettlementAppliedV3)
	store.LoseNextSessionCASReply = true
	waiting, err := state.ApplySettledTurnV2(context.Background(), harnessports.ApplySettledTurnRequestV2{Run: candidate.Run, SessionID: waitingSettlement.ID, ExpectedSessionRevision: waitingSettlement.Revision, Attempt: settled, DomainResult: domain, UpdatedUnixNano: now.Add(3 * time.Second).UnixNano()})
	if err != nil || waiting.Phase != contract.SessionWaitingActionV2 || waiting.PendingAction == nil {
		t.Fatalf("exact settlement did not apply action: %#v err=%v", waiting, err)
	}
	changedDomain := domain
	changedDomain.Schema.Name = "other"
	if _, err := state.ApplySettledTurnV2(context.Background(), harnessports.ApplySettledTurnRequestV2{Run: candidate.Run, SessionID: waitingSettlement.ID, ExpectedSessionRevision: waitingSettlement.Revision, Attempt: settled, DomainResult: changedDomain, UpdatedUnixNano: now.Add(3 * time.Second).UnixNano()}); err == nil {
		t.Fatal("same settlement replay replaced its exact DomainResult")
	}
	changedPending, err := contract.NewPendingActionV2("action-other", pending.Capability, pending.Payload, candidateRef)
	if err != nil {
		t.Fatal(err)
	}
	changedTurn := contract.SettledTurnResultV2{ContractVersion: contract.SettledTurnResultContractV2, Candidate: candidateRef, State: contract.SettledTurnActionRequiredV2, Action: &changedPending}
	changedSidecar, err := contract.NewSettledTurnDomainResultV2(changedTurn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.ApplySettledTurnV2(context.Background(), harnessports.ApplySettledTurnRequestV2{Run: candidate.Run, SessionID: waitingSettlement.ID, ExpectedSessionRevision: waitingSettlement.Revision, Attempt: settled, DomainResult: changedSidecar, UpdatedUnixNano: now.Add(3 * time.Second).UnixNano()}); err == nil {
		t.Fatal("same settlement replay replaced its exact action sidecar")
	}
}

func TestGovernedTurnStateV2UnknownAttemptStaysReconcilingUntilObserved(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	store := fakes.NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	loop, _ := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: store, Candidates: store, Clock: func() time.Time { return now }, CandidateTTL: time.Minute})
	_, candidate := testkit.GovernedFactsV2(now)
	prepared, _ := loop.PrepareInitialCandidateV2(context.Background(), prepareRequestV2(candidate))
	state := &kernel.GovernedTurnStateCoordinatorV2{Sessions: store}
	candidateRef, _ := candidate.RefV2()
	reserved, reservation := reserveSessionForKernelTestV3(t, store, prepared.Session, candidateRef, now)
	attempt := testkit.GovernedAttemptRefsV2(now, candidate, "")
	inflight, _ := state.AttachPreparedAttemptV2(context.Background(), harnessports.AttachPreparedAttemptRequestV2{Run: candidate.Run, SessionID: reserved.ID, ExpectedSessionRevision: reserved.Revision, Candidate: candidateRef, Reservation: reservation, Attempt: attempt, UpdatedUnixNano: now.Add(time.Second).UnixNano()})
	store.LoseNextSessionCASReply = true
	reconciling, err := state.MarkAttemptReconcilingV2(context.Background(), harnessports.MarkAttemptReconcilingRequestV2{Run: candidate.Run, SessionID: inflight.ID, ExpectedSessionRevision: inflight.Revision, UpdatedUnixNano: now.Add(2 * time.Second).UnixNano()})
	if err != nil || reconciling.Phase != contract.SessionReconcilingV2 {
		t.Fatalf("unknown attempt did not enter recoverable reconciliation: %#v err=%v", reconciling, err)
	}
	unknown := testkit.GovernedAttemptRefsV2(now, candidate, runtimeports.ProviderAttemptUnknownV2)
	if _, err := state.AttachObservedAttemptV2(context.Background(), harnessports.AttachObservedAttemptRequestV2{Run: candidate.Run, SessionID: reconciling.ID, ExpectedSessionRevision: reconciling.Revision, Attempt: unknown, UpdatedUnixNano: now.Add(3 * time.Second).UnixNano()}); err == nil {
		t.Fatal("unknown provider observation closed reconciliation without independent settlement")
	}
	stored, err := store.InspectSessionV2(context.Background(), candidate.Run, reconciling.ID)
	if err != nil || stored.Phase != contract.SessionReconcilingV2 || stored.CompletionClaim != "" {
		t.Fatalf("unknown attempt escaped reconciliation: %#v err=%v", stored, err)
	}
	failure := contract.NewSettledTurnFailureV2(candidateRef, "custom.model/inspect-not-applied", []byte("independent inspect confirmed failure"))
	settled, domain := testkit.GovernedSettledAttemptRefsV2(now, candidate, failure, runtimeports.OperationSettlementFailedV3)
	missingInspect := settled
	missingInspect.Observation = nil
	missingSettlement := *settled.Settlement
	missingSettlement.Observation = nil
	missingInspect.Settlement = &missingSettlement
	if _, err := state.ApplySettledTurnV2(context.Background(), harnessports.ApplySettledTurnRequestV2{Run: candidate.Run, SessionID: reconciling.ID, ExpectedSessionRevision: reconciling.Revision, Attempt: missingInspect, DomainResult: domain, UpdatedUnixNano: now.Add(4 * time.Second).UnixNano()}); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("post-prepared unknown settled without Inspect provenance: %v", err)
	}
	testkit.AddUnknownInspectProvenanceV2(&settled)
	wrongInspect := settled
	wrongSettlement := *settled.Settlement
	wrongInspectionSettlement := *wrongSettlement.InspectionSettlement
	wrongInspectionSettlement.Attempt.AttemptID = "inspect-attempt-swapped"
	wrongSettlement.InspectionSettlement = &wrongInspectionSettlement
	wrongInspect.Settlement = &wrongSettlement
	if _, err := state.ApplySettledTurnV2(context.Background(), harnessports.ApplySettledTurnRequestV2{Run: candidate.Run, SessionID: reconciling.ID, ExpectedSessionRevision: reconciling.Revision, Attempt: wrongInspect, DomainResult: domain, UpdatedUnixNano: now.Add(4 * time.Second).UnixNano()}); err == nil {
		t.Fatal("post-prepared unknown accepted swapped Inspect settlement")
	}
	terminal, err := state.ApplySettledTurnV2(context.Background(), harnessports.ApplySettledTurnRequestV2{Run: candidate.Run, SessionID: reconciling.ID, ExpectedSessionRevision: reconciling.Revision, Attempt: settled, DomainResult: domain, UpdatedUnixNano: now.Add(4 * time.Second).UnixNano()})
	if err != nil || terminal.CompletionClaim != contract.ClaimFailed {
		t.Fatalf("independently settled unknown attempt did not close exactly: %#v err=%v", terminal, err)
	}
}

func TestGovernedTurnStateV2ReconcilingValidatesBeforeSessionBackend(t *testing.T) {
	sessions := &countingSessionPortV2{}
	state := &kernel.GovernedTurnStateCoordinatorV2{Sessions: sessions}
	if _, err := state.MarkAttemptReconcilingV2(context.Background(), harnessports.MarkAttemptReconcilingRequestV2{}); err == nil {
		t.Fatal("zero reconciliation request reached the session backend")
	}
	if sessions.calls != 0 {
		t.Fatalf("invalid reconciliation touched session backend %d times", sessions.calls)
	}
}

func TestGovernedTurnStateV2ConcurrentPreparedAttachmentLinearizesOnce(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	store := fakes.NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	loop, _ := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: store, Candidates: store, Clock: func() time.Time { return now }, CandidateTTL: time.Minute})
	_, candidate := testkit.GovernedFactsV2(now)
	prepared, err := loop.PrepareInitialCandidateV2(context.Background(), prepareRequestV2(candidate))
	if err != nil {
		t.Fatal(err)
	}
	candidateRef, _ := candidate.RefV2()
	reserved, reservation := reserveSessionForKernelTestV3(t, store, prepared.Session, candidateRef, now)
	attempt := testkit.GovernedAttemptRefsV2(now, candidate, "")
	request := harnessports.AttachPreparedAttemptRequestV2{
		Run: candidate.Run, SessionID: reserved.ID, ExpectedSessionRevision: reserved.Revision,
		Candidate: candidateRef, Reservation: reservation, Attempt: attempt, UpdatedUnixNano: now.Add(time.Second).UnixNano(),
	}
	state := &kernel.GovernedTurnStateCoordinatorV2{Sessions: store}
	const workers = 64
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			<-start
			got, err := state.AttachPreparedAttemptV2(context.Background(), request)
			if err == nil && (got.Revision != reserved.Revision+1 || got.Phase != contract.SessionModelInFlightV2) {
				err = core.NewError(core.ErrorConflict, core.ReasonInvalidState, "concurrent attachment returned another state")
			}
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	stored, err := store.InspectSessionV2(context.Background(), candidate.Run, reserved.ID)
	if err != nil || stored.Revision != reserved.Revision+1 {
		t.Fatalf("concurrent attachment advanced more than once: %#v err=%v", stored, err)
	}
}

func TestGovernedTurnStateV2ReplayRejectsSameRefsWithClaimOrTimeDrift(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	store := fakes.NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	loop, _ := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: store, Candidates: store, Clock: func() time.Time { return now }, CandidateTTL: time.Minute})
	_, candidate := testkit.GovernedFactsV2(now)
	prepared, _ := loop.PrepareInitialCandidateV2(context.Background(), prepareRequestV2(candidate))
	state := &kernel.GovernedTurnStateCoordinatorV2{Sessions: store}
	candidateRef, _ := candidate.RefV2()
	reserved, reservation := reserveSessionForKernelTestV3(t, store, prepared.Session, candidateRef, now)
	attempt := testkit.GovernedAttemptRefsV2(now, candidate, "")
	inflight, _ := state.AttachPreparedAttemptV2(context.Background(), harnessports.AttachPreparedAttemptRequestV2{
		Run: candidate.Run, SessionID: reserved.ID, ExpectedSessionRevision: reserved.Revision,
		Candidate: candidateRef, Reservation: reservation, Attempt: attempt, UpdatedUnixNano: now.Add(time.Second).UnixNano(),
	})
	observed := testkit.GovernedAttemptRefsV2(now, candidate, runtimeports.ProviderAttemptObservedV2)
	original := harnessports.AttachObservedAttemptRequestV2{Run: candidate.Run, SessionID: inflight.ID, ExpectedSessionRevision: inflight.Revision, Attempt: observed, UpdatedUnixNano: now.Add(2 * time.Second).UnixNano()}
	waitingSettlement, err := state.AttachObservedAttemptV2(context.Background(), original)
	if err != nil || waitingSettlement.Phase != contract.SessionWaitingSettlementV2 {
		t.Fatalf("waiting settlement fixture failed: %#v err=%v", waitingSettlement, err)
	}
	changedTime := original
	changedTime.UpdatedUnixNano++
	if _, err := state.AttachObservedAttemptV2(context.Background(), changedTime); err == nil {
		t.Fatal("same Runtime refs replaced an already attached timestamp")
	}
	preparedTimeDrift := harnessports.AttachPreparedAttemptRequestV2{
		Run: candidate.Run, SessionID: reserved.ID, ExpectedSessionRevision: reserved.Revision,
		Candidate: candidateRef, Reservation: reservation, Attempt: attempt, UpdatedUnixNano: now.Add(time.Second).UnixNano() + 1,
	}
	if _, err := state.AttachPreparedAttemptV2(context.Background(), preparedTimeDrift); err == nil {
		t.Fatal("same prepared refs replaced their committed attachment timestamp")
	}
	output := candidate.Input
	turn := contract.SettledTurnResultV2{ContractVersion: contract.SettledTurnResultContractV2, Candidate: candidateRef, State: contract.SettledTurnCompletedV2, Output: &output}
	settled, domain := testkit.GovernedSettledAttemptRefsV2(now, candidate, turn, runtimeports.OperationSettlementAppliedV3)
	apply := harnessports.ApplySettledTurnRequestV2{Run: candidate.Run, SessionID: waitingSettlement.ID, ExpectedSessionRevision: waitingSettlement.Revision, Attempt: settled, DomainResult: domain, UpdatedUnixNano: now.Add(3 * time.Second).UnixNano()}
	terminal, err := state.ApplySettledTurnV2(context.Background(), apply)
	if err != nil || terminal.CompletionClaim != contract.ClaimCompleted {
		t.Fatalf("settled terminal fixture failed: %#v err=%v", terminal, err)
	}
	changedSettlement := apply
	changedSettlement.UpdatedUnixNano++
	if _, err := state.ApplySettledTurnV2(context.Background(), changedSettlement); err == nil {
		t.Fatal("same settlement refs replaced their committed timestamp")
	}
	failedTurn := contract.NewSettledTurnFailureV2(candidateRef, "custom.model/failed", []byte("different terminal claim"))
	failedDomain, err := contract.NewSettledTurnDomainResultV2(failedTurn)
	if err != nil {
		t.Fatal(err)
	}
	changedClaim := apply
	changedClaim.DomainResult = failedDomain
	if _, err := state.ApplySettledTurnV2(context.Background(), changedClaim); err == nil {
		t.Fatal("same settlement refs replaced the derived terminal claim")
	}
}

func TestGovernedSessionFactPortV2RejectsNakedInFlightCAS(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	store := fakes.NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	loop, _ := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: store, Candidates: store, Clock: func() time.Time { return now }, CandidateTTL: time.Minute})
	_, candidate := testkit.GovernedFactsV2(now)
	prepared, err := loop.PrepareInitialCandidateV2(context.Background(), prepareRequestV2(candidate))
	if err != nil {
		t.Fatal(err)
	}
	naked := prepared.Session
	naked.Revision++
	naked.Phase = contract.SessionModelInFlightV2
	naked.UpdatedUnixNano++
	if _, err := store.CompareAndSwapSessionV2(context.Background(), harnessports.SessionCASRequestV2{ExpectedRevision: prepared.Session.Revision, Next: naked}); err == nil {
		t.Fatal("raw Session CAS entered model_in_flight without Runtime governed refs")
	}
	stored, err := store.InspectSessionV2(context.Background(), candidate.Run, prepared.Session.ID)
	if err != nil || stored.Revision != prepared.Session.Revision || stored.Phase != contract.SessionWaitingModelDispatchV2 {
		t.Fatalf("rejected naked CAS changed authoritative Session: %#v err=%v", stored, err)
	}
}

func TestGovernedSessionFactPortV2RejectsNakedSettlementToActionOrTerminalCAS(t *testing.T) {
	now := time.Unix(2_000_000_000, 0)
	store := fakes.NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	loop, _ := kernel.NewGovernedLoopV2(kernel.GovernedLoopConfigV2{Sessions: store, Candidates: store, Clock: func() time.Time { return now }, CandidateTTL: time.Minute})
	_, candidate := testkit.GovernedFactsV2(now)
	prepared, err := loop.PrepareInitialCandidateV2(context.Background(), prepareRequestV2(candidate))
	if err != nil {
		t.Fatal(err)
	}
	state := &kernel.GovernedTurnStateCoordinatorV2{Sessions: store}
	candidateRef, _ := candidate.RefV2()
	reserved, reservation := reserveSessionForKernelTestV3(t, store, prepared.Session, candidateRef, now)
	inflight, err := state.AttachPreparedAttemptV2(context.Background(), harnessports.AttachPreparedAttemptRequestV2{
		Run: candidate.Run, SessionID: reserved.ID, ExpectedSessionRevision: reserved.Revision,
		Candidate: candidateRef, Reservation: reservation, Attempt: testkit.GovernedAttemptRefsV2(now, candidate, ""), UpdatedUnixNano: now.Add(time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	waitingSettlement, err := state.AttachObservedAttemptV2(context.Background(), harnessports.AttachObservedAttemptRequestV2{
		Run: candidate.Run, SessionID: inflight.ID, ExpectedSessionRevision: inflight.Revision,
		Attempt: testkit.GovernedAttemptRefsV2(now, candidate, runtimeports.ProviderAttemptObservedV2), UpdatedUnixNano: now.Add(2 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := contract.NewPendingActionV2("action-naked", "custom.tool/execute", candidate.Input, candidateRef)
	if err != nil {
		t.Fatal(err)
	}
	nakedAction := waitingSettlement
	nakedAction.Revision++
	nakedAction.Phase = contract.SessionWaitingActionV2
	nakedAction.Candidate = nil
	nakedAction.PendingAction = &pending
	nakedAction.UpdatedUnixNano++
	if _, err := store.CompareAndSwapSessionV2(context.Background(), harnessports.SessionCASRequestV2{ExpectedRevision: waitingSettlement.Revision, Next: nakedAction}); err == nil {
		t.Fatal("raw Session CAS derived an action from Observation without exact Settlement")
	}
	nakedTerminal := waitingSettlement
	nakedTerminal.Revision++
	nakedTerminal.Phase = contract.SessionTerminalV2
	nakedTerminal.Candidate = nil
	nakedTerminal.CompletionClaim = contract.ClaimCompleted
	nakedTerminal.UpdatedUnixNano++
	if _, err := store.CompareAndSwapSessionV2(context.Background(), harnessports.SessionCASRequestV2{ExpectedRevision: waitingSettlement.Revision, Next: nakedTerminal}); err == nil {
		t.Fatal("raw Session CAS derived a completion claim from Observation without exact Settlement")
	}
	stored, err := store.InspectSessionV2(context.Background(), candidate.Run, waitingSettlement.ID)
	if err != nil || stored.Revision != waitingSettlement.Revision || stored.Phase != contract.SessionWaitingSettlementV2 {
		t.Fatalf("rejected naked settlement CAS changed authoritative Session: %#v err=%v", stored, err)
	}
}

func reserveSessionForKernelTestV3(t *testing.T, store *fakes.GovernedStoreV2, waiting contract.GovernedSessionV2, candidate contract.CandidateRefV2, now time.Time) (contract.GovernedSessionV2, contract.ModelDispatchReservationRefV2) {
	t.Helper()
	reservation := contract.ModelDispatchReservationRefV2{ID: "reservation-" + waiting.ID, Digest: testkit.Digest("reservation-" + waiting.ID), AttemptID: "application-attempt-" + waiting.ID, IntentDigest: testkit.Digest("intent-" + waiting.ID), CandidateDigest: candidate.Digest, ReservedUnixNano: now.Add(time.Millisecond).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	next := waiting
	next.Revision++
	next.Phase = contract.SessionModelDispatchReservedV2
	next.DomainReservation = &reservation
	next.UpdatedUnixNano = reservation.ReservedUnixNano
	stored, err := store.CompareAndSwapSessionV2(context.Background(), harnessports.SessionCASRequestV2{ExpectedRevision: waiting.Revision, Next: next})
	if err != nil {
		t.Fatal(err)
	}
	return stored, reservation
}
