package contract_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestGovernedSessionV3CreateSharesV2ConflictDomainAndDeepClones(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	v2, _ := testkit.GovernedFactsV2(now)
	v3 := sealCreatingSessionV3(t, v2)
	store := fakes.NewGovernedStoreV2()
	if _, err := store.CreateSessionV3(context.Background(), v3); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSessionV2(context.Background(), v2); err == nil {
		t.Fatal("V2 was allowed to occupy an existing V3 session key")
	}
	got, err := store.InspectSessionV3(context.Background(), v3.Run, v3.ID)
	if err != nil {
		t.Fatal(err)
	}
	got.Run.Scope.SandboxLease.ID = "mutated"
	again, _ := store.InspectSessionV3(context.Background(), v3.Run, v3.ID)
	if again.Run.Scope.SandboxLease.ID == "mutated" {
		t.Fatal("V3 store returned an aliased SandboxLease")
	}

	other := fakes.NewGovernedStoreV2()
	if _, err := other.CreateSessionV2(context.Background(), v2); err != nil {
		t.Fatal(err)
	}
	if _, err := other.CreateSessionV3(context.Background(), v3); err == nil {
		t.Fatal("V3 was allowed to occupy an existing V2 session key")
	}
}

func TestGovernedSessionV3CreateLostReplyUsesExactInspectAndReplay(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	v2, _ := testkit.GovernedFactsV2(now)
	creating := sealCreatingSessionV3(t, v2)
	store := fakes.NewGovernedStoreV2()
	store.LoseNextSessionV3CreateReply = true
	if _, err := store.CreateSessionV3(context.Background(), creating); err == nil {
		t.Fatal("injected V3 create reply loss was not reported")
	}
	inspected, err := store.InspectSessionV3(context.Background(), creating.Run, creating.ID)
	if err != nil || !reflect.DeepEqual(inspected, creating) {
		t.Fatalf("lost create reply did not expose the exact stored session: %#v / %v", inspected, err)
	}
	replayed, err := store.CreateSessionV3(context.Background(), creating)
	if err != nil || !reflect.DeepEqual(replayed, creating) {
		t.Fatalf("exact create replay failed: %#v / %v", replayed, err)
	}

	changed := creating.Clone()
	changed.UpdatedUnixNano++
	changed = sealSessionV3(t, changed)
	if _, err := store.CreateSessionV3(context.Background(), changed); err == nil {
		t.Fatal("same V3 create key with different canonical content was accepted")
	}
}

func TestGovernedSessionV3TypedNilPortIsUnavailable(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	steps := governedSessionV3Steps(t, now)
	request := sealCASV3(t, steps[0], steps[1])
	var store *fakes.GovernedStoreV2
	var port harnessports.SessionFactPortV3 = store
	if _, err := port.CreateSessionV3(context.Background(), steps[0]); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("typed-nil Create error = %v", err)
	}
	if _, err := port.InspectSessionV3(context.Background(), steps[0].Run, steps[0].ID); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("typed-nil Inspect error = %v", err)
	}
	if _, err := port.CompareAndSwapSessionV3(context.Background(), request); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("typed-nil CAS error = %v", err)
	}
}

func TestGovernedSessionV3LostReplyReplayRequiresExactSuccessorAndExpectedDigest(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	store := fakes.NewGovernedStoreV2()
	steps := governedSessionV3Steps(t, now)
	if _, err := store.CreateSessionV3(context.Background(), steps[0]); err != nil {
		t.Fatal(err)
	}
	for index := 1; index < len(steps); index++ {
		request := sealCASV3(t, steps[index-1], steps[index])
		if index == len(steps)-1 {
			store.LoseNextSessionV3CASReply = true
			if _, err := store.CompareAndSwapSessionV3(context.Background(), request); err == nil {
				t.Fatal("injected final CAS reply loss was not reported")
			}
		}
		got, err := store.CompareAndSwapSessionV3(context.Background(), request)
		if err != nil || got.Digest != steps[index].Digest || got.Revision != steps[index].Revision {
			t.Fatalf("step %d exact CAS/replay failed: %#v / %v", index, got, err)
		}
	}
	current, _ := store.InspectSessionV3(context.Background(), steps[0].Run, steps[0].ID)
	if current.ApplicationBinding == nil || current.ApplicationBinding.IdentityRef != steps[len(steps)-1].ApplicationBinding.IdentityRef {
		t.Fatal("waiting_action successor did not atomically retain the complete binding")
	}

	alternative := steps[len(steps)-1].Clone()
	alternative.UpdatedUnixNano++
	alternative = sealSessionV3(t, alternative)
	if err := contract.ValidateSessionTransitionV3(steps[len(steps)-2], alternative); err != nil {
		t.Fatalf("alternative successor fixture is not independently valid: %v", err)
	}
	stale := sealCASV3(t, steps[len(steps)-2], alternative)
	if _, err := store.CompareAndSwapSessionV3(context.Background(), stale); err == nil {
		t.Fatal("valid but non-exact successor bypassed no-ABA CAS")
	}

	for name, mutate := range map[string]func(*contract.GovernedSessionV3){
		"pending-action": func(next *contract.GovernedSessionV3) {
			changed, err := contract.NewPendingActionV2("action-g6a-drift", next.PendingAction.Capability, next.PendingAction.Payload, next.PendingAction.SourceCandidate)
			if err != nil {
				t.Fatal(err)
			}
			next.PendingAction = &changed
			next.ApplicationBinding.PendingAction = changed
		},
		"identity-ref": func(next *contract.GovernedSessionV3) {
			next.ApplicationBinding.IdentityRef.ID = next.ApplicationBinding.DomainResultFactRef.FactID
		},
		"domain-result-ref": func(next *contract.GovernedSessionV3) {
			next.ApplicationBinding.DomainResultFactRef.FactID = next.ApplicationBinding.IdentityRef.ID
		},
		"model-turn-settlement": func(next *contract.GovernedSessionV3) {
			changed := next.ApplicationBinding.ModelTurnSettlementRef
			changed.ID = "settlement-g6a-drift"
			changed.Digest = testkit.Digest("settlement-g6a-drift")
			next.ApplicationBinding.ModelTurnSettlementRef = changed
			next.Execution.Settlement = &changed
		},
	} {
		t.Run("lost-reply-binding-drift-"+name, func(t *testing.T) {
			next := steps[len(steps)-1].Clone()
			mutate(&next)
			next.Digest = ""
			sealed, err := contract.SealGovernedSessionV3(next)
			if err != nil {
				return
			}
			request, err := contract.SealSessionCASRequestV3(contract.SessionCASRequestV3{Run: steps[len(steps)-2].Run, SessionID: steps[len(steps)-2].ID, ExpectedRevision: steps[len(steps)-2].Revision, ExpectedDigest: steps[len(steps)-2].Digest, Next: sealed})
			if err != nil {
				t.Fatalf("valid drift fixture failed CAS sealing: %v", err)
			}
			if _, err := store.CompareAndSwapSessionV3(context.Background(), request); err == nil {
				t.Fatal("ApplicationBinding drift was accepted after lost reply")
			}
		})
	}
}

func TestGovernedSessionV3StoreDeepClonesBindingAndSettlement(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	steps := governedSessionV3Steps(t, now)
	store := fakes.NewGovernedStoreV2()
	if _, err := store.CreateSessionV3(context.Background(), steps[0]); err != nil {
		t.Fatal(err)
	}
	for index := 1; index < len(steps); index++ {
		request := sealCASV3(t, steps[index-1], steps[index])
		returned, err := store.CompareAndSwapSessionV3(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if index != len(steps)-1 {
			continue
		}
		request.Next.ApplicationBinding.PendingAction.Payload.Inline[0] ^= 1
		request.Next.Execution.Settlement.Evidence[0].RecordDigest = testkit.Digest("mutated-input-evidence")
		request.Next.Execution.Settlement.Attempt.Delegation.ID = "mutated-input-delegation"
		request.Next.Execution.Settlement.DomainResultSchema.ContentDigest = testkit.Digest("mutated-input-schema")
		returned.PendingAction.Payload.Inline[0] ^= 1
		returned.ApplicationBinding.PendingAction.Payload.Inline[0] ^= 1
		returned.Execution.Settlement.Evidence[0].RecordDigest = testkit.Digest("mutated-return-evidence")
		returned.Execution.Settlement.Attempt.Delegation.ID = "mutated-return-delegation"
		returned.Execution.Settlement.DomainResultSchema.ContentDigest = testkit.Digest("mutated-return-schema")
	}
	stored, err := store.InspectSessionV3(context.Background(), steps[0].Run, steps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	want := steps[len(steps)-1]
	if !reflect.DeepEqual(stored, want) {
		t.Fatal("V3 store input/return mutation polluted nested PendingAction, Evidence, Delegation or DomainResultSchema")
	}
}

func TestGovernedSessionV3RejectsWaitingActionLineageReplacement(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	steps := governedSessionV3Steps(t, now)
	current := steps[len(steps)-2]
	next := steps[len(steps)-1].Clone()
	next.PendingAction.SourceCandidate.Digest = testkit.Digest("other-candidate")
	next.ApplicationBinding.PendingAction = *next.PendingAction
	next.ApplicationBinding.IdentityRef.PendingActionRequestDigest = next.PendingAction.RequestDigest
	next.Digest = ""
	if _, err := contract.SealGovernedSessionV3(next); err == nil {
		// A self-invalid successor must already fail sealing; if future contracts
		// make it intrinsically valid, transition lineage must still reject it.
		t.Fatal("spliced waiting_action successor unexpectedly sealed")
	}

	next = steps[len(steps)-1].Clone()
	next.Execution.Admission.OperationDigest = testkit.Digest("other-operation")
	next.Execution.Prepared.OperationDigest = next.Execution.Admission.OperationDigest
	next.Execution.Enforcement.OperationDigest = next.Execution.Admission.OperationDigest
	next.Execution.Settlement.Attempt.OperationDigest = next.Execution.Admission.OperationDigest
	next.Digest = ""
	resealed, err := contract.SealGovernedSessionV3(next)
	if err == nil {
		if err := contract.ValidateSessionTransitionV3(current, resealed); err == nil {
			t.Fatal("whole-attempt replacement crossed waiting_settlement to waiting_action")
		}
	}
}

func TestGovernedSessionV3ConcurrentCASLinearizesExactSuccessor(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	steps := governedSessionV3Steps(t, now)
	store := fakes.NewGovernedStoreV2()
	if _, err := store.CreateSessionV3(context.Background(), steps[0]); err != nil {
		t.Fatal(err)
	}
	request := sealCASV3(t, steps[0], steps[1])
	const workers = 64
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			got, err := store.CompareAndSwapSessionV3(context.Background(), request)
			if err == nil && got.Digest != request.Next.Digest {
				err = coreConflictForTest("concurrent CAS returned another successor")
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

	conflictStore := fakes.NewGovernedStoreV2()
	if _, err := conflictStore.CreateSessionV3(context.Background(), steps[0]); err != nil {
		t.Fatal(err)
	}
	start = make(chan struct{})
	errs = make(chan error, workers)
	var successes int
	var successMu sync.Mutex
	requests := make([]contract.SessionCASRequestV3, workers)
	for index := range workers {
		next := steps[1].Clone()
		next.Candidate.ID = "candidate-competing-" + string(rune('A'+index))
		next.Candidate.Digest = testkit.Digest(next.Candidate.ID)
		next = sealSessionV3(t, next)
		requests[index] = sealCASV3(t, steps[0], next)
	}
	for index := range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := conflictStore.CompareAndSwapSessionV3(context.Background(), requests[index])
			if err == nil {
				successMu.Lock()
				successes++
				successMu.Unlock()
			}
			errs <- nil
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	if successes != 1 {
		t.Fatalf("competing successors succeeded %d times", successes)
	}
}

func coreConflictForTest(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, message)
}

func governedSessionV3Steps(t *testing.T, now time.Time) []contract.GovernedSessionV3 {
	t.Helper()
	v2, candidate := testkit.GovernedFactsV2(now)
	identity, fact, _, pending := identityFactFixtureV1(t, now, "")
	candidateRef, _ := candidate.RefV2()
	creating := sealCreatingSessionV3(t, v2)
	waitingDispatch := creating.Clone()
	waitingDispatch.Revision++
	waitingDispatch.Phase = contract.SessionWaitingModelDispatchV2
	waitingDispatch.Turn = 1
	waitingDispatch.Candidate = &candidateRef
	waitingDispatch.UpdatedUnixNano++
	waitingDispatch = sealSessionV3(t, waitingDispatch)

	reservation := contract.ModelDispatchReservationRefV2{ID: "reservation-g6a", Digest: testkit.Digest("reservation-g6a"), AttemptID: "attempt-governed", IntentDigest: testkit.Digest("intent-g6a"), CandidateDigest: candidateRef.Digest, ReservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	reserved := waitingDispatch.Clone()
	reserved.Revision++
	reserved.Phase = contract.SessionModelDispatchReservedV2
	reserved.DomainReservation = &reservation
	reserved.UpdatedUnixNano++
	reserved = sealSessionV3(t, reserved)

	prepared := testkit.GovernedAttemptRefsV2(now, candidate, "")
	inflight := reserved.Clone()
	inflight.Revision++
	inflight.Phase = contract.SessionModelInFlightV2
	inflight.Execution = &prepared
	inflight.UpdatedUnixNano++
	inflight = sealSessionV3(t, inflight)

	observed := testkit.GovernedAttemptRefsV2(now, candidate, runtimeports.ProviderAttemptObservedV2)
	waitingSettlement := inflight.Clone()
	waitingSettlement.Revision++
	waitingSettlement.Phase = contract.SessionWaitingSettlementV2
	waitingSettlement.Execution = &observed
	waitingSettlement.UpdatedUnixNano++
	waitingSettlement = sealSessionV3(t, waitingSettlement)

	factRef, _ := fact.RefV3()
	identityRef, _ := identity.RefV1(fact.ContentDigest)
	settled := observed
	delegation := settled.Delegation
	observation := *settled.Observation
	settled.Settlement = &runtimeports.OperationSettlementRefV3{ID: "settlement-g6a", Revision: 1, Digest: testkit.Digest("settlement-g6a"), Attempt: runtimeports.OperationDispatchAttemptRefV3{OperationDigest: settled.Admission.OperationDigest, EffectID: settled.Admission.EffectID, IntentRevision: settled.Admission.IntentRevision, IntentDigest: settled.Admission.IntentDigest, PermitID: settled.PermitID, PermitRevision: settled.PermitRevision, PermitDigest: settled.PermitDigest, AttemptID: settled.AttemptID, Delegation: &delegation}, Disposition: runtimeports.OperationSettlementAppliedV3, Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: "custom.model/settlement-owner", ManifestDigest: testkit.Digest("settlement-owner")}, Observation: &observation, Evidence: []runtimeports.EvidenceRecordRefV2{observation.Evidence}, DomainResultSchema: &fact.Schema, DomainResultDigest: fact.ContentDigest}
	binding := contract.PendingActionApplicationBindingV1{PendingAction: pending, IdentityRef: identityRef, DomainResultFactRef: factRef, ModelTurnSettlementRef: *settled.Settlement}
	waitingAction := waitingSettlement.Clone()
	waitingAction.Revision++
	waitingAction.Phase = contract.SessionWaitingActionV2
	waitingAction.Candidate = nil
	waitingAction.DomainReservation = nil
	waitingAction.Execution = &settled
	waitingAction.PendingAction = &pending
	waitingAction.ApplicationBinding = &binding
	waitingAction.UpdatedUnixNano++
	waitingAction = sealSessionV3(t, waitingAction)
	return []contract.GovernedSessionV3{creating, waitingDispatch, reserved, inflight, waitingSettlement, waitingAction}
}

func sealCreatingSessionV3(t *testing.T, v2 contract.GovernedSessionV2) contract.GovernedSessionV3 {
	t.Helper()
	return sealSessionV3(t, contract.GovernedSessionV3{ID: v2.ID, Revision: v2.Revision, Run: v2.Run, Endpoint: v2.Endpoint, Phase: v2.Phase, Turn: v2.Turn, CreatedUnixNano: v2.CreatedUnixNano, UpdatedUnixNano: v2.UpdatedUnixNano})
}

func sealSessionV3(t *testing.T, value contract.GovernedSessionV3) contract.GovernedSessionV3 {
	t.Helper()
	sealed, err := contract.SealGovernedSessionV3(value)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func sealCASV3(t *testing.T, current, next contract.GovernedSessionV3) contract.SessionCASRequestV3 {
	t.Helper()
	request, err := contract.SealSessionCASRequestV3(contract.SessionCASRequestV3{Run: current.Run, SessionID: current.ID, ExpectedRevision: current.Revision, ExpectedDigest: current.Digest, Next: next})
	if err != nil {
		t.Fatal(err)
	}
	return request
}
