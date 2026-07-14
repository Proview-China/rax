package application_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRunCoordinatorV3RejectsForgedSuccessfulCASReply(t *testing.T) {
	fx := newCoordinatorFixtureV3(t, "custom.run/forged-cas")
	initial := fx.initial
	initial.IntentValue.Kind = runtimeports.OperationEffectKindExecutionStartV3
	initial.DelegationPlan.RuntimeSessionRef, _ = runtimeports.DeriveRuntimeExecutionSessionRefV2(initial.DelegationPlan.EndpointID, initial.Operation.RunID)
	initial.Intent, _ = contract.NewOperationIntentRefV3(initial.IntentValue)
	fx.runtime.base = initial
	if _, err := fx.attempts.CreateGovernedOperationAttemptV3(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	create := runCreateRequestV3(t, initial, fx.now)
	runtime := newRunRuntimeV3(create, fx.now)
	underlying := fakes.NewRunCoordinationStoreV3()
	forged := &forgedSuccessfulRunCoordinationStoreV3{RunCoordinationFactPortV3: underlying}
	coordinator, err := application.NewRunCoordinatorV3(application.RunCoordinatorConfigV3{Facts: forged, Journals: fx.journals, Attempts: fx.attempts, Assembler: runtime, Lifecycle: runtime, Start: runtime, Claims: runtime, Clock: func() time.Time { return fx.now.Add(10 * time.Millisecond) }})
	if err != nil {
		t.Fatal(err)
	}
	_, err = coordinator.PrepareRunV3(context.Background(), application.PrepareRunCoordinationRequestV3{CoordinationID: "run-forged-cas", Plan: fx.plan, JournalID: initial.JournalID, StepID: initial.StepID, StartAttemptID: initial.ID, Create: create})
	if !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("same-revision changed successful CAS reply was trusted: %v", err)
	}
	persisted, err := underlying.InspectRunCoordinationV3(context.Background(), fx.plan.Target, "run-forged-cas")
	if err != nil || persisted.State != contract.RunCoordinationCreatePlannedV3 || persisted.Revision != 1 {
		t.Fatalf("forged reply advanced authoritative store: %#v err=%v", persisted, err)
	}
}

func TestRunCoordinatorV3PrepareAndSettledStartRecoverLostReplies(t *testing.T) {
	fx := newCoordinatorFixtureV3(t, "custom.run/start")
	initial := fx.initial
	initial.IntentValue.Kind = runtimeports.OperationEffectKindExecutionStartV3
	initial.DelegationPlan.RuntimeSessionRef, _ = runtimeports.DeriveRuntimeExecutionSessionRefV2(initial.DelegationPlan.EndpointID, initial.Operation.RunID)
	ref, err := contract.NewOperationIntentRefV3(initial.IntentValue)
	if err != nil {
		t.Fatal(err)
	}
	initial.Intent = ref
	if err := initial.Validate(); err != nil {
		t.Fatal(err)
	}
	fx.initial, fx.runtime.base = initial, initial
	if _, err := fx.attempts.CreateGovernedOperationAttemptV3(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	create := runCreateRequestV3(t, initial, fx.now)
	runtime := newRunRuntimeV3(create, fx.now)
	runtime.loseCreateReply, runtime.loseStartReply = true, true
	clockNow := fx.now.Add(10 * time.Millisecond)
	coordinationFacts := fakes.NewRunCoordinationStoreV3()
	coordinationFacts.LoseNextCreateReply = true
	coordinationFacts.LoseCASReplies = 1
	coordinator, err := application.NewRunCoordinatorV3(application.RunCoordinatorConfigV3{Facts: coordinationFacts, Journals: fx.journals, Attempts: fx.attempts, Assembler: runtime, Lifecycle: runtime, Start: runtime, Claims: runtime, Clock: func() time.Time { return clockNow }})
	if err != nil {
		t.Fatal(err)
	}
	pending := parallelRunCoordinationV3(t, 64, func() (contract.RunCoordinationFactV3, error) {
		return coordinator.PrepareRunV3(context.Background(), application.PrepareRunCoordinationRequestV3{CoordinationID: "run-coordination", Plan: fx.plan, JournalID: initial.JournalID, StepID: initial.StepID, StartAttemptID: initial.ID, Create: create})
	})
	if pending.State != contract.RunCoordinationPendingV3 || runtime.createMutations != 1 {
		t.Fatalf("pending create recovery failed: %#v mutations=%d", pending, runtime.createMutations)
	}
	observed, err := fx.coordinator.StartGovernedOperationV3(context.Background(), application.StartGovernedOperationRequestV3{Plan: fx.plan, Attempt: initial})
	if err != nil {
		t.Fatal(err)
	}
	settlement := settlementSubmissionForAttemptV3(t, observed.Attempt, fx.now)
	if _, err := fx.coordinator.SettleGovernedOperationV3(context.Background(), application.SettleGovernedOperationRequestV3{Plan: fx.plan, AttemptID: initial.ID, Submission: settlement}); err != nil {
		t.Fatal(err)
	}
	coordinationFacts.LoseCASReplies = 2
	running := parallelRunCoordinationV3(t, 64, func() (contract.RunCoordinationFactV3, error) {
		return coordinator.ConfirmRunStartedV3(context.Background(), application.RunCoordinationRequestV3{Plan: fx.plan, CoordinationID: pending.ID})
	})
	if running.State != contract.RunCoordinationRunningV3 || running.Lifecycle.Run.Status != core.RunRunning || runtime.startMutations != 1 || running.Lifecycle.Run.StartedAt.UnixNano() != running.StartRuntimeAttempt.Observation.ObservedUnixNano {
		t.Fatalf("settled start recovery failed: %#v mutations=%d", running, runtime.startMutations)
	}
	if running.Lifecycle.Run.Outcome != "" {
		t.Fatal("Application injected a caller outcome into running Run")
	}
	candidate := runClaimCandidateV3(t, running.Lifecycle.Run, fx.now)
	plannedClaim := running
	plannedClaim.Revision++
	plannedClaim.State = contract.RunCoordinationClaimPlannedV3
	plannedClaim.UpdatedUnixNano++
	plannedClaim.ClaimCandidate = &candidate
	if err := contract.ValidateRunCoordinationTransitionV3(running, plannedClaim); err != nil {
		t.Fatalf("valid claim plan transition rejected: %v", err)
	}
	claimResult := runClaimResultV3(candidate, running.Lifecycle.Run, running.Lifecycle.Certification, running.Lifecycle.Plan)
	associatedClaim := plannedClaim
	associatedClaim.Revision++
	associatedClaim.State = contract.RunCoordinationClaimAssociatedV3
	associatedClaim.UpdatedUnixNano++
	associatedClaim.ClaimResult = &claimResult
	if err := contract.ValidateRunCoordinationTransitionV3(plannedClaim, associatedClaim); err != nil {
		t.Fatalf("valid claim association transition rejected: %v", err)
	}
	forgedCandidate := candidate
	forgedCandidate.EventID = "forged-terminal-event"
	forgedAssociation := associatedClaim
	forgedAssociation.ClaimCandidate = &forgedCandidate
	if err := contract.ValidateRunCoordinationTransitionV3(plannedClaim, forgedAssociation); err == nil {
		t.Fatal("claim association replaced its write-ahead candidate")
	}
	forgedLifecycle := running
	forgedLifecycle.Revision++
	forgedLifecycle.State = contract.RunCoordinationStopPlannedV3
	forgedLifecycle.UpdatedUnixNano++
	envelope := *running.Lifecycle
	envelope.Plan.ID = "forged-plan"
	forgedLifecycle.Lifecycle = &envelope
	if err := contract.ValidateRunCoordinationTransitionV3(running, forgedLifecycle); err == nil {
		t.Fatal("stop transition replaced immutable lifecycle Plan")
	}
	forgedStart := running
	forgedStart.Revision++
	forgedStart.State = contract.RunCoordinationStopPlannedV3
	forgedStart.UpdatedUnixNano++
	operation := *running.StartOperation
	operation.CurrentProjectionDigest = core.DigestBytes([]byte("forged-start"))
	forgedStart.StartOperation = &operation
	if err := contract.ValidateRunCoordinationTransitionV3(running, forgedStart); err == nil {
		t.Fatal("stop transition replaced settled execution-start sidecar")
	}
	forgedTime := running
	forgedTime.Revision++
	forgedTime.State = contract.RunCoordinationStopPlannedV3
	forgedTime.UpdatedUnixNano++
	timeEnvelope := *running.Lifecycle
	timeEnvelope.Run.StartedAt = timeEnvelope.Run.StartedAt.Add(time.Second)
	forgedTime.Lifecycle = &timeEnvelope
	if err := contract.ValidateRunCoordinationTransitionV3(running, forgedTime); err == nil {
		t.Fatal("Run lifecycle replaced governed StartedAt")
	}
	forgedConfirmation := running
	forgedConfirmation.Revision++
	forgedConfirmation.State = contract.RunCoordinationStopPlannedV3
	forgedConfirmation.UpdatedUnixNano++
	confirmation := *running.StartConfirmation
	confirmation.ID = "forged-start-confirmation"
	confirmation, _ = runtimeports.SealRunStartConfirmationFactV3(confirmation)
	forgedConfirmation.StartConfirmation = &confirmation
	if err := contract.ValidateRunCoordinationTransitionV3(running, forgedConfirmation); err == nil {
		t.Fatal("Run lifecycle replaced exact StartConfirmation")
	}
	forgedCertification := running
	forgedCertification.Revision++
	forgedCertification.State = contract.RunCoordinationStopPlannedV3
	forgedCertification.UpdatedUnixNano++
	certificationEnvelope := *running.Lifecycle
	certificationEnvelope.Certification.Certification.Digest = core.DigestBytes([]byte("another-valid-certification"))
	forgedCertification.Lifecycle = &certificationEnvelope
	if err := contract.ValidateRunCoordinationTransitionV3(running, forgedCertification); err == nil {
		t.Fatal("Run lifecycle replaced the exact create-time Plan certification association")
	}
	wrongSession := claimResult
	wrongSession.Run.SessionRef = "runtime-session:sha256:" + string(core.DigestBytes([]byte("other-session")))
	forgedClaimResult := associatedClaim
	forgedClaimResult.ClaimResult = &wrongSession
	if err := contract.ValidateRunCoordinationTransitionV3(plannedClaim, forgedClaimResult); err == nil {
		t.Fatal("claim association accepted another Run session")
	}
	wrongCertification := claimResult
	wrongCertification.Certification.Certification.Digest = core.DigestBytes([]byte("another-claim-certification"))
	forgedClaimResult = associatedClaim
	forgedClaimResult.ClaimResult = &wrongCertification
	if err := contract.ValidateRunCoordinationTransitionV3(plannedClaim, forgedClaimResult); err == nil {
		t.Fatal("claim association accepted another Plan certification")
	}
	wrongPlan := claimResult
	wrongPlan.Plan.ID = "another-claim-plan"
	forgedClaimResult = associatedClaim
	forgedClaimResult.ClaimResult = &wrongPlan
	if err := contract.ValidateRunCoordinationTransitionV3(plannedClaim, forgedClaimResult); err == nil {
		t.Fatal("claim association accepted another lifecycle Plan")
	}
	runtime.loseClaimReply = true
	coordinationFacts.LoseCASReplies = 2
	claimed := parallelRunCoordinationV3(t, 64, func() (contract.RunCoordinationFactV3, error) {
		return coordinator.IngestTerminalClaimV3(context.Background(), application.IngestRunClaimRequestV3{Plan: fx.plan, CoordinationID: pending.ID, Candidate: candidate})
	})
	if claimed.State != contract.RunCoordinationClaimAssociatedV3 || runtime.claimMutations != 1 || runtime.claimInspectCalls == 0 || claimed.Lifecycle.Run.Status != core.RunRunning || claimed.Lifecycle.Run.Outcome != "" {
		t.Fatalf("claim was not evidence-only: %#v", claimed)
	}
	clockNow = time.Unix(0, fx.plan.ExpiresUnixNano).Add(time.Hour)
	runtime.loseBeginReply, runtime.loseStopReply = true, true
	coordinationFacts.LoseCASReplies = 3
	terminal := parallelRunCoordinationV3(t, 64, func() (contract.RunCoordinationFactV3, error) {
		return coordinator.StopAndSettleRunV3(context.Background(), application.RunCoordinationRequestV3{Plan: fx.plan, CoordinationID: pending.ID})
	})
	if terminal.State != contract.RunCoordinationTerminalCleanupV3 || runtime.beginMutations != 1 || runtime.stopMutations != 1 || terminal.Lifecycle.Progress.UnresolvedCount != 1 {
		t.Fatalf("unknown cleanup was falsely closed: %#v", terminal)
	}
	forgedTerminal := terminal
	forgedTerminal.Revision++
	forgedTerminal.UpdatedUnixNano++
	terminalEnvelope := *terminal.Lifecycle
	closure := *terminalEnvelope.Closure
	closure.Digest = core.DigestBytes([]byte("forged-closure"))
	decision := *terminalEnvelope.Decision
	decision.Closure = closure
	progress := *terminalEnvelope.Progress
	progress.Decision = decision
	terminalEnvelope.Closure, terminalEnvelope.Decision, terminalEnvelope.Progress = &closure, &decision, &progress
	forgedTerminal.Lifecycle = &terminalEnvelope
	if err := contract.ValidateRunCoordinationTransitionV3(terminal, forgedTerminal); err == nil {
		t.Fatal("terminal cleanup replaced Closure/Decision causal chain")
	}
	forgedOutcome := terminal
	forgedOutcome.Revision++
	forgedOutcome.UpdatedUnixNano++
	outcomeEnvelope := *terminal.Lifecycle
	outcomeEnvelope.Run.Outcome = core.OutcomeFailed
	outcomeDecision := *outcomeEnvelope.Decision
	outcomeDecision.Outcome = core.OutcomeFailed
	outcomeProgress := *outcomeEnvelope.Progress
	outcomeProgress.Decision = outcomeDecision
	outcomeEnvelope.Decision, outcomeEnvelope.Progress = &outcomeDecision, &outcomeProgress
	forgedOutcome.Lifecycle = &outcomeEnvelope
	if err := contract.ValidateRunCoordinationTransitionV3(terminal, forgedOutcome); err == nil {
		t.Fatal("terminal cleanup replaced Runtime Outcome")
	}
	runtime.loseReconcileReply = true
	coordinationFacts.LoseCASReplies = 1
	closed := parallelRunCoordinationV3(t, 64, func() (contract.RunCoordinationFactV3, error) {
		return coordinator.ReconcileRunTerminationV3(context.Background(), application.RunCoordinationRequestV3{Plan: fx.plan, CoordinationID: pending.ID})
	})
	if closed.State != contract.RunCoordinationTerminationClosedV3 || runtime.reconcileMutations != 1 || closed.Lifecycle.Report == nil {
		t.Fatalf("termination reconcile recovery failed: %#v", closed)
	}
	read, _ := coordinationFacts.InspectRunCoordinationV3(context.Background(), closed.Scope, closed.ID)
	read.StartAttempt.SettlementDomainResult.Inline[0] ^= 0xff
	read.ClaimCandidate.Causation = append(read.ClaimCandidate.Causation, runtimeports.EvidenceCausationRefV2{LedgerScopeDigest: core.DigestBytes([]byte("mutated")), EventID: "mutated"})
	originalDelegationID := read.StartConfirmation.Attempt.Settlement.Attempt.Delegation.ID
	read.StartConfirmation.Attempt.Settlement.Attempt.Delegation.ID = "mutated-nested-delegation"
	again, _ := coordinationFacts.InspectRunCoordinationV3(context.Background(), closed.Scope, closed.ID)
	if again.StartAttempt.SettlementDomainResult.Inline[0] == read.StartAttempt.SettlementDomainResult.Inline[0] || len(again.ClaimCandidate.Causation) != 0 || again.StartConfirmation.Attempt.Settlement.Attempt.Delegation.ID != originalDelegationID {
		t.Fatal("run coordination fake leaked mutable nested storage")
	}
}

type forgedSuccessfulRunCoordinationStoreV3 struct {
	applicationports.RunCoordinationFactPortV3
	forged atomic.Bool
}

func (s *forgedSuccessfulRunCoordinationStoreV3) CompareAndSwapRunCoordinationV3(ctx context.Context, request applicationports.RunCoordinationCASRequestV3) (contract.RunCoordinationFactV3, error) {
	if !s.forged.Swap(true) {
		forged := request.Next
		forged.UpdatedUnixNano++
		if err := forged.Validate(); err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
		return forged, nil
	}
	return s.RunCoordinationFactPortV3.CompareAndSwapRunCoordinationV3(ctx, request)
}

func TestRunCoordinatorV3RejectsExpiredPlanBeforeAnyRuntimeMutation(t *testing.T) {
	fx := newCoordinatorFixtureV3(t, "custom.run/expired")
	initial := fx.initial
	initial.IntentValue.Kind = runtimeports.OperationEffectKindExecutionStartV3
	initial.DelegationPlan.RuntimeSessionRef, _ = runtimeports.DeriveRuntimeExecutionSessionRefV2(initial.DelegationPlan.EndpointID, initial.Operation.RunID)
	initial.Intent, _ = contract.NewOperationIntentRefV3(initial.IntentValue)
	fx.runtime.base = initial
	if _, err := fx.attempts.CreateGovernedOperationAttemptV3(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	create := runCreateRequestV3(t, initial, fx.now)
	runtime := newRunRuntimeV3(create, fx.now)
	facts := fakes.NewRunCoordinationStoreV3()
	journal, err := fx.journals.InspectWorkflowJournalV2(context.Background(), fx.plan.Target, initial.JournalID)
	if err != nil {
		t.Fatal(err)
	}
	wal, err := contract.NewRunCoordinationFactV3("expired-run", fx.plan, journal, initial.StepID, initial, create, fx.now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facts.CreateRunCoordinationV3(context.Background(), wal); err != nil {
		t.Fatal(err)
	}
	coordinator, err := application.NewRunCoordinatorV3(application.RunCoordinatorConfigV3{Facts: facts, Journals: fx.journals, Attempts: fx.attempts, Assembler: runtime, Lifecycle: runtime, Start: runtime, Claims: runtime, Clock: func() time.Time { return time.Unix(0, fx.plan.ExpiresUnixNano) }})
	if err != nil {
		t.Fatal(err)
	}
	_, err = coordinator.PrepareRunV3(context.Background(), application.PrepareRunCoordinationRequestV3{CoordinationID: "expired-run", Plan: fx.plan, JournalID: initial.JournalID, StepID: initial.StepID, StartAttemptID: initial.ID, Create: create})
	if !core.HasReason(err, core.ReasonCapabilityExpired) || runtime.createMutations != 0 {
		t.Fatalf("expired Plan reached Runtime: err=%v mutations=%d", err, runtime.createMutations)
	}
	runtime2 := newRunRuntimeV3(create, fx.now)
	if _, err := runtime2.CreatePendingRunV3(context.Background(), create); err != nil {
		t.Fatal(err)
	}
	facts2 := fakes.NewRunCoordinationStoreV3()
	wal2 := wal
	wal2.ID = "expired-run-existing"
	if _, err := facts2.CreateRunCoordinationV3(context.Background(), wal2); err != nil {
		t.Fatal(err)
	}
	coordinator2, _ := application.NewRunCoordinatorV3(application.RunCoordinatorConfigV3{Facts: facts2, Journals: fx.journals, Attempts: fx.attempts, Assembler: runtime2, Lifecycle: runtime2, Start: runtime2, Claims: runtime2, Clock: func() time.Time { return time.Unix(0, fx.plan.ExpiresUnixNano) }})
	recovered, err := coordinator2.PrepareRunV3(context.Background(), application.PrepareRunCoordinationRequestV3{CoordinationID: wal2.ID, Plan: fx.plan, JournalID: initial.JournalID, StepID: initial.StepID, StartAttemptID: initial.ID, Create: create})
	if err != nil || recovered.State != contract.RunCoordinationPendingV3 || runtime2.createMutations != 1 {
		t.Fatalf("expired WAL did not recover already-created pending Run: %#v err=%v mutations=%d", recovered, err, runtime2.createMutations)
	}
	runtime3 := newRunRuntimeV3(create, fx.now)
	if _, err := runtime3.CreatePendingRunV3(context.Background(), create); err != nil {
		t.Fatal(err)
	}
	runtime3.forgeNextInspect = true
	facts3 := fakes.NewRunCoordinationStoreV3()
	wal3 := wal
	wal3.ID = "malformed-inspect"
	if _, err := facts3.CreateRunCoordinationV3(context.Background(), wal3); err != nil {
		t.Fatal(err)
	}
	coordinator3, _ := application.NewRunCoordinatorV3(application.RunCoordinatorConfigV3{Facts: facts3, Journals: fx.journals, Attempts: fx.attempts, Assembler: runtime3, Lifecycle: runtime3, Start: runtime3, Claims: runtime3, Clock: func() time.Time { return fx.now }})
	if _, err := coordinator3.PrepareRunV3(context.Background(), application.PrepareRunCoordinationRequestV3{CoordinationID: wal3.ID, Plan: fx.plan, JournalID: initial.JournalID, StepID: initial.StepID, StartAttemptID: initial.ID, Create: create}); err == nil || runtime3.createMutations != 1 {
		t.Fatalf("malformed lifecycle Inspect triggered a mutation: err=%v mutations=%d", err, runtime3.createMutations)
	}
}

func parallelRunCoordinationV3(t *testing.T, workers int, call func() (contract.RunCoordinationFactV3, error)) contract.RunCoordinationFactV3 {
	t.Helper()
	type result struct {
		fact contract.RunCoordinationFactV3
		err  error
	}
	out := make(chan result, workers)
	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() { defer wait.Done(); fact, err := call(); out <- result{fact, err} }()
	}
	wait.Wait()
	close(out)
	var latest contract.RunCoordinationFactV3
	for item := range out {
		if item.err != nil {
			t.Fatalf("concurrent Run coordination failed: %v", item.err)
		}
		if item.fact.Revision > latest.Revision {
			latest = item.fact
		}
	}
	return latest
}

type runRuntimeV3 struct {
	mu                 sync.Mutex
	create             runtimeports.CreatePendingRunRequestV3
	envelope           *runtimeports.RunLifecycleEnvelopeV3
	now                time.Time
	loseCreateReply    bool
	loseStartReply     bool
	loseBeginReply     bool
	loseStopReply      bool
	loseReconcileReply bool
	loseClaimReply     bool
	createMutations    int
	startMutations     int
	beginMutations     int
	stopMutations      int
	reconcileMutations int
	claimMutations     int
	claimInspectCalls  int
	claimResult        *runtimeports.RunClaimIngestResultV3
	startConfirmation  *runtimeports.RunStartConfirmationEnvelopeV3
	forgeNextInspect   bool
}

func newRunRuntimeV3(create runtimeports.CreatePendingRunRequestV3, now time.Time) *runRuntimeV3 {
	return &runRuntimeV3{create: create, now: now}
}

func (r *runRuntimeV3) CreatePendingRunV3(_ context.Context, request runtimeports.CreatePendingRunRequestV3) (runtimeports.RunLifecycleEnvelopeV3, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.RunLifecycleEnvelopeV3{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.envelope == nil {
		value := pendingRunEnvelopeV3(request)
		r.envelope = &value
		r.createMutations++
	}
	if r.loseCreateReply {
		r.loseCreateReply = false
		return runtimeports.RunLifecycleEnvelopeV3{}, unavailableV3("create pending reply lost")
	}
	return *r.envelope, nil
}
func (r *runRuntimeV3) InspectRunLifecycleV3(_ context.Context, scope core.ExecutionScope, id core.AgentRunID) (runtimeports.RunLifecycleEnvelopeV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.envelope == nil {
		return runtimeports.RunLifecycleEnvelopeV3{}, notFoundV3("run lifecycle")
	}
	if r.forgeNextInspect {
		r.forgeNextInspect = false
		forged := *r.envelope
		forged.Plan.ID = "forged-inspect-plan"
		return forged, nil
	}
	return *r.envelope, nil
}
func (r *runRuntimeV3) ConfirmRunStartedV3(_ context.Context, request runtimeports.ConfirmRunStartedRequestV3) (runtimeports.RunStartConfirmationEnvelopeV3, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.RunStartConfirmationEnvelopeV3{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.envelope.Run.Status == core.RunPending {
		run := r.envelope.Run
		run.Status = core.RunRunning
		run.Revision++
		run.StartedAt = time.Unix(0, request.Attempt.Observation.ObservedUnixNano)
		envelope := *r.envelope
		envelope.Phase = runtimeports.RunLifecycleRunningV3
		envelope.Run = run
		r.envelope = &envelope
		runIdentity, _ := runtimeports.RunIdentityDigestV2(run)
		scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(run.Scope)
		operationDigest, _ := request.Operation.DigestV3()
		confirmation, sealErr := runtimeports.SealRunStartConfirmationFactV3(runtimeports.RunStartConfirmationFactV3{ContractVersion: runtimeports.RunSettlementContractVersionV2, ID: "run-start-confirmation", Revision: 1, RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScope: run.Scope, ExecutionScopeDigest: scopeDigest, OperationDigest: operationDigest, Attempt: request.Attempt, RunRevision: run.Revision, StartedUnixNano: run.StartedAt.UnixNano()})
		if sealErr != nil {
			return runtimeports.RunStartConfirmationEnvelopeV3{}, sealErr
		}
		start := runtimeports.RunStartConfirmationEnvelopeV3{Run: run, Certification: r.envelope.Certification, Confirmation: confirmation}
		r.startConfirmation = &start
		r.startMutations++
	}
	if r.loseStartReply {
		r.loseStartReply = false
		return runtimeports.RunStartConfirmationEnvelopeV3{}, unavailableV3("confirm start reply lost")
	}
	return *r.startConfirmation, nil
}
func (r *runRuntimeV3) InspectRunStartV3(_ context.Context, _ core.ExecutionScope, _ core.AgentRunID) (runtimeports.RunStartConfirmationEnvelopeV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.envelope == nil || r.envelope.Run.Status == core.RunPending {
		return runtimeports.RunStartConfirmationEnvelopeV3{}, notFoundV3("run start")
	}
	return *r.startConfirmation, nil
}
func (r *runRuntimeV3) BeginStopRunV3(_ context.Context, request runtimeports.BeginStopRunRequestV3) (runtimeports.RunLifecycleEnvelopeV3, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.RunLifecycleEnvelopeV3{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.envelope.Phase == runtimeports.RunLifecycleRunningV3 {
		value := *r.envelope
		value.Phase = runtimeports.RunLifecycleStoppingV3
		value.Run.Status = core.RunStopping
		value.Run.Revision++
		r.envelope = &value
		r.beginMutations++
	}
	if r.loseBeginReply {
		r.loseBeginReply = false
		return runtimeports.RunLifecycleEnvelopeV3{}, unavailableV3("begin stop reply lost")
	}
	return *r.envelope, nil
}
func (r *runRuntimeV3) StopAndSettleRunV3(_ context.Context, request runtimeports.BeginStopRunRequestV3) (runtimeports.RunLifecycleEnvelopeV3, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.RunLifecycleEnvelopeV3{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.envelope.Phase == runtimeports.RunLifecycleStoppingV3 {
		value := terminalCleanupEnvelopeV3(*r.envelope, r.now)
		r.envelope = &value
		r.stopMutations++
	}
	if r.loseStopReply {
		r.loseStopReply = false
		return runtimeports.RunLifecycleEnvelopeV3{}, unavailableV3("stop settle reply lost")
	}
	return *r.envelope, nil
}
func (r *runRuntimeV3) ReconcileRunTerminationV3(_ context.Context, request runtimeports.RunTerminationRequestV3) (runtimeports.RunLifecycleEnvelopeV3, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.RunLifecycleEnvelopeV3{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.envelope.Phase == runtimeports.RunLifecycleTerminalCleanupV3 {
		value := closedTerminationEnvelopeV3(*r.envelope)
		r.envelope = &value
		r.reconcileMutations++
	}
	if r.loseReconcileReply {
		r.loseReconcileReply = false
		return runtimeports.RunLifecycleEnvelopeV3{}, unavailableV3("termination reconcile reply lost")
	}
	return *r.envelope, nil
}
func (r *runRuntimeV3) InspectRunTerminationV3(_ context.Context, _ runtimeports.RunTerminationRequestV3) (runtimeports.RunLifecycleEnvelopeV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.envelope == nil || (r.envelope.Phase != runtimeports.RunLifecycleTerminalCleanupV3 && r.envelope.Phase != runtimeports.RunLifecycleTerminationClosedV3) {
		return runtimeports.RunLifecycleEnvelopeV3{}, notFoundV3("termination")
	}
	return *r.envelope, nil
}
func (r *runRuntimeV3) IngestRunClaimV3(_ context.Context, request runtimeports.RunClaimIngestRequestV2) (runtimeports.RunClaimIngestResultV3, error) {
	if err := request.Validate(); err != nil {
		return runtimeports.RunClaimIngestResultV3{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.claimResult == nil {
		value := runClaimResultV3(request.Candidate, r.envelope.Run, r.envelope.Certification, r.envelope.Plan)
		r.claimResult = &value
		r.claimMutations++
	}
	if r.loseClaimReply {
		r.loseClaimReply = false
		return runtimeports.RunClaimIngestResultV3{}, unavailableV3("claim ingest reply lost")
	}
	return *r.claimResult, nil
}
func (r *runRuntimeV3) InspectRunClaimV3(context.Context, core.ExecutionScope, core.AgentRunID) (runtimeports.RunClaimIngestResultV3, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.claimInspectCalls++
	if r.claimResult == nil {
		return runtimeports.RunClaimIngestResultV3{}, notFoundV3("run claim")
	}
	return *r.claimResult, nil
}

func pendingRunEnvelopeV3(create runtimeports.CreatePendingRunRequestV3) runtimeports.RunLifecycleEnvelopeV3 {
	planRef, _ := create.Plan.RefV2()
	return runtimeports.RunLifecycleEnvelopeV3{ContractVersion: runtimeports.RunLifecycleContractVersionV3, Phase: runtimeports.RunLifecyclePendingPreparedV3, Run: create.Run, Plan: runtimeports.RunSettlementPlanLifecycleRefV3{RunSettlementPlanRefV2: planRef, RunID: create.Run.ID, RunIdentityDigest: create.Plan.RunIdentityDigest, ExecutionScopeDigest: create.Plan.ExecutionScopeDigest}, Certification: create.Certification, EffectIndex: runtimeports.RunEffectIndexRefV3{ID: create.EffectIndexID, Revision: 1, Digest: core.DigestBytes([]byte("effect-index")), RunID: create.Run.ID, RunIdentityDigest: create.Plan.RunIdentityDigest, ExecutionScopeDigest: create.Plan.ExecutionScopeDigest, Watermark: 1, HeadDigest: runtimeports.EvidenceGenesisDigestV2}}
}

func terminalCleanupEnvelopeV3(stopping runtimeports.RunLifecycleEnvelopeV3, now time.Time) runtimeports.RunLifecycleEnvelopeV3 {
	value := stopping
	value.Phase = runtimeports.RunLifecycleTerminalCleanupV3
	value.Run.Status, value.Run.Revision, value.Run.EndedAt, value.Run.Outcome = core.RunTerminal, value.Run.Revision+1, now.Add(time.Second), core.OutcomeIndeterminate
	value.EffectIndex.Revision++
	value.EffectIndex.Digest, value.EffectIndex.Frozen = core.DigestBytes([]byte("effect-index-frozen")), true
	closure := runtimeports.RunSettlementClosureRefV3{ID: "run-closure", RunID: value.Run.ID, RunIdentityDigest: value.Plan.RunIdentityDigest, ExecutionScopeDigest: value.Plan.ExecutionScopeDigest, Attempt: 1, Revision: 1, Digest: core.DigestBytes([]byte("closure"))}
	decision := runtimeports.RunSettlementDecisionRefV3{ID: "run-decision", RunID: value.Run.ID, RunIdentityDigest: value.Plan.RunIdentityDigest, ExecutionScopeDigest: value.Plan.ExecutionScopeDigest, Revision: 1, Digest: core.DigestBytes([]byte("decision")), Outcome: value.Run.Outcome, Closure: closure}
	progress := runtimeports.RunTerminationProgressRefV3{ID: "termination-progress", RunID: value.Run.ID, RunIdentityDigest: value.Plan.RunIdentityDigest, ExecutionScopeDigest: value.Plan.ExecutionScopeDigest, Revision: 1, Digest: core.DigestBytes([]byte("progress-unknown")), UnresolvedCount: 1, Decision: decision}
	value.Closure, value.Decision, value.Progress = &closure, &decision, &progress
	return value
}

func closedTerminationEnvelopeV3(pending runtimeports.RunLifecycleEnvelopeV3) runtimeports.RunLifecycleEnvelopeV3 {
	value := pending
	value.Phase = runtimeports.RunLifecycleTerminationClosedV3
	progress := *value.Progress
	progress.Revision++
	progress.Digest, progress.UnresolvedCount = core.DigestBytes([]byte("progress-closed")), 0
	value.Progress = &progress
	report := runtimeports.RunTerminationReportRefV3{ID: "termination-report", RunID: value.Run.ID, RunIdentityDigest: value.Plan.RunIdentityDigest, ExecutionScopeDigest: value.Plan.ExecutionScopeDigest, Revision: 1, Digest: core.DigestBytes([]byte("report")), Decision: *value.Decision, Progress: progress}
	value.Report = &report
	return value
}

func runClaimCandidateV3(t *testing.T, run core.AgentRunRecord, now time.Time) runtimeports.EvidenceEventCandidateV2 {
	t.Helper()
	d := func(v string) core.Digest { return core.DigestBytes([]byte(v)) }
	schema := runtimeports.SchemaRefV2{Namespace: "runtime", Name: "run-claim", Version: "2.0.0", MediaType: "application/octet-stream", ContentDigest: d("claim-schema")}
	value := runtimeports.EvidenceEventCandidateV2{ContractVersion: runtimeports.EvidenceContractVersionV2, LedgerScope: runtimeports.EvidenceLedgerScopeV2{Partition: runtimeports.EvidencePartitionRun, TenantID: run.Scope.Identity.TenantID, IdentityID: run.Scope.Identity.ID, LineageID: run.Scope.Lineage.ID, InstanceID: run.Scope.Instance.ID, RunID: run.ID}, EventID: "terminal-event", RegistrationID: "claim-registration", RegistrationRevision: 1, SourceConfigurationDigest: d("claim-config"), SourcePolicy: runtimeports.EvidenceSourcePolicyBindingRefV2{Ref: "claim-policy", Digest: d("claim-policy"), Revision: 1}, SourceID: "harness/run-claim", SourceEpoch: run.Scope.Instance.Epoch, SourceSequence: 1, TrustClass: runtimeports.EvidenceTrustClaim, ClaimKind: core.RunClaimCompleted, EventKind: "runtime/run-completion", CustomClass: "runtime/claim", ExecutionScope: run.Scope, Payload: runtimeports.EvidencePayloadRefV2{Schema: schema, ContentDigest: d("claim-payload"), Revision: 1, Length: 1, Ref: "memory://terminal-event"}, Causation: []runtimeports.EvidenceCausationRefV2{}, CorrelationID: string(run.ID), Producer: runtimeports.EvidenceProducerBindingRefV2{BindingSetID: "claim-binding", BindingSetRevision: 1, ComponentID: "runtime/harness", ManifestDigest: d("claim-manifest"), ArtifactDigest: d("claim-artifact"), Capability: "runtime/claim"}, Authority: runtimeports.AuthorityBindingRefV2{Ref: "claim-authority", Digest: d("claim-authority"), Revision: 1, Epoch: run.Scope.AuthorityEpoch}, ObservedUnixNano: now.Add(20 * time.Millisecond).UnixNano()}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	return value
}

func runClaimResultV3(candidate runtimeports.EvidenceEventCandidateV2, run core.AgentRunRecord, certification runtimeports.RunSettlementPlanCertificationAssociationV3, plan runtimeports.RunSettlementPlanLifecycleRefV3) runtimeports.RunClaimIngestResultV3 {
	candidateDigest, _ := candidate.DigestV2()
	ledgerDigest, _ := candidate.LedgerScope.DigestV2()
	ingested := candidate.ObservedUnixNano + 1
	record := runtimeports.EvidenceLedgerRecordV2{Ref: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: ledgerDigest, Sequence: 1, RecordDigest: core.DigestBytes([]byte("claim-record"))}, Candidate: candidate, CandidateDigest: candidateDigest, PreviousRecordDigest: runtimeports.EvidenceGenesisDigestV2, IngestedUnixNano: ingested}
	runIdentity, _ := runtimeports.RunIdentityDigestV2(run)
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(run.Scope)
	associationID, _ := runtimeports.RunClaimAssociationIDV2(run.ID, record.Ref)
	association := runtimeports.RunClaimAssociationFactV2{ContractVersion: runtimeports.RunClaimAssociationContractVersionV2, ID: associationID, Revision: 1, State: runtimeports.RunClaimAssociatedV2, RunID: run.ID, RunRevisionAtAssociation: run.Revision, RunIdentityDigest: runIdentity, ExecutionScope: run.Scope, ExecutionScopeDigest: scopeDigest, LineagePlanDigest: run.Scope.Lineage.PlanDigest, ClaimKind: candidate.ClaimKind, RegistrationID: candidate.RegistrationID, SourceID: candidate.SourceID, SourceEpoch: candidate.SourceEpoch, SourceSequence: candidate.SourceSequence, EventID: candidate.EventID, Evidence: record.Ref, CandidateDigest: candidateDigest, PayloadDigest: candidate.Payload.ContentDigest, ObservedUnixNano: candidate.ObservedUnixNano, EvidenceIngestedUnixNano: ingested, CreatedUnixNano: ingested}
	return runtimeports.RunClaimIngestResultV3{Certification: certification, Plan: plan, Run: run, Evidence: record, Association: association}
}

func runCreateRequestV3(t *testing.T, attempt contract.GovernedOperationAttemptFactV3, now time.Time) runtimeports.CreatePendingRunRequestV3 {
	t.Helper()
	runID := attempt.Operation.RunID
	session, err := runtimeports.DeriveRuntimeExecutionSessionRefV2(attempt.DelegationPlan.EndpointID, runID)
	if err != nil {
		t.Fatal(err)
	}
	run := core.AgentRunRecord{ID: runID, Scope: attempt.Scope, Status: core.RunPending, Revision: 1, SessionRef: session}
	runIdentity, _ := runtimeports.RunIdentityDigestV2(run)
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(run.Scope)
	provider := runtimeports.EvidenceProducerBindingRefV2(attempt.PlannedProvider)
	execution := runtimeports.RunExecutionSubjectV2{EndpointID: attempt.DelegationPlan.EndpointID, EndpointDigest: core.DigestBytes([]byte("endpoint")), SessionRef: session, Binding: provider}
	execution.SubjectDigest, _ = execution.DigestV2()
	schema := runtimeports.SchemaRefV2{Namespace: "runtime.run", Name: "settlement", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("settlement-schema"))}
	type reqDef struct {
		id    runtimeports.NamespacedNameV2
		kind  runtimeports.NamespacedNameV2
		phase runtimeports.RunSettlementRequirementPhaseV2
	}
	defs := []reqDef{{"runtime.req/budget", runtimeports.RunRequirementBudget, runtimeports.RunSettlementPhaseCompletion}, {"runtime.req/domain", runtimeports.RunRequirementDomainCommits, runtimeports.RunSettlementPhaseCompletion}, {"runtime.req/effects", runtimeports.RunRequirementEffects, runtimeports.RunSettlementPhaseCompletion}, {"runtime.req/execution", runtimeports.RunRequirementExecutionTruth, runtimeports.RunSettlementPhaseCompletion}, {"runtime.req/remote", runtimeports.RunRequirementRemoteContinuations, runtimeports.RunSettlementPhaseCompletion}, {"runtime.req/cleanup", runtimeports.RunRequirementCleanup, runtimeports.RunSettlementPhaseTerminationReport}, {"runtime.req/provider", runtimeports.RunRequirementProviderRetention, runtimeports.RunSettlementPhaseTerminationReport}, {"runtime.req/residual", runtimeports.RunRequirementResidual, runtimeports.RunSettlementPhaseTerminationReport}}
	requirements := make([]runtimeports.RunSettlementRequirementV2, 0, len(defs))
	for _, d := range defs {
		subject := core.DigestBytes([]byte(string(d.kind)))
		if d.kind == runtimeports.RunRequirementExecutionTruth {
			subject = execution.SubjectDigest
		}
		requirements = append(requirements, runtimeports.RunSettlementRequirementV2{ID: d.id, Kind: d.kind, Phase: d.phase, Owner: provider, Schema: schema, SubjectSelector: "runtime.run/subject", SubjectDigest: subject, Policy: runtimeports.RunSettlementPolicyBindingRefV2{Ref: "policy-" + string(d.id[12:]), Revision: 1, Digest: core.DigestBytes([]byte("policy-" + string(d.id))), SemanticDigest: core.DigestBytes([]byte("semantic-" + string(d.id)))}, EvidenceTrust: runtimeports.EvidenceTrustAttestation, EvidenceKind: "runtime.run/attestation"})
	}
	plan := runtimeports.RunSettlementPlanFactV2{ContractVersion: runtimeports.RunSettlementContractVersionV2, ID: "run-settlement-plan", Revision: 1, RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScope: run.Scope, ExecutionScopeDigest: scopeDigest, SessionRef: session, LineagePlanDigest: run.Scope.Lineage.PlanDigest, BindingSet: runtimeports.RunBindingSetRefV2{ID: attempt.PlannedProvider.BindingSetID, Revision: attempt.PlannedProvider.BindingSetRevision, Digest: core.DigestBytes([]byte("binding-set")), SemanticDigest: core.DigestBytes([]byte("binding-set-semantic"))}, Execution: execution, Claim: runtimeports.RunClaimRequirementV2{Mode: runtimeports.RunClaimRequiredV2}, Requirements: requirements, CreatedUnixNano: now.UnixNano()}
	planRef, err := plan.RefV2()
	if err != nil {
		t.Fatal(err)
	}
	certification, err := runtimeports.SealRunSettlementPlanCertificationFactV3(runtimeports.RunSettlementPlanCertificationFactV3{
		ContractVersion:      runtimeports.RunSettlementPlanAdmissionContractVersionV3,
		ID:                   "run-settlement-plan-certification",
		Revision:             1,
		RunID:                run.ID,
		RunIdentityDigest:    runIdentity,
		ExecutionScope:       run.Scope,
		ExecutionScopeDigest: scopeDigest,
		Plan:                 planRef,
		BindingSet:           plan.BindingSet,
		BaselinePolicy:       runtimeports.RunSettlementBaselinePolicyRefV3{ID: "run-settlement-baseline-policy", Revision: 1, Digest: core.DigestBytes([]byte("run-settlement-baseline-policy"))},
		Declarations: []runtimeports.RunSettlementDeclarationRefV3{{
			ID: "run-settlement-declaration", Revision: 1, Digest: core.DigestBytes([]byte("run-settlement-declaration")),
			BindingSetID: plan.BindingSet.ID, BindingSetRevision: plan.BindingSet.Revision, BindingRevision: 1, ComponentID: provider.ComponentID,
		}},
		CertificationOwner: provider,
		CreatedUnixNano:    now.UnixNano(),
		ExpiresUnixNano:    now.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	certificationRef, err := certification.RefV3()
	if err != nil {
		t.Fatal(err)
	}
	association, err := runtimeports.NewRunSettlementPlanCertificationAssociationV3(run, plan, certificationRef)
	if err != nil {
		t.Fatal(err)
	}
	request := runtimeports.CreatePendingRunRequestV3{Run: run, Plan: plan, Certification: association, EffectIndexID: "run-effect-index"}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	return request
}
