package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RunSettlementStoreV2 is a deterministic in-memory implementation of the
// single Run Fact Owner transaction boundary. It is conformance/test-only and
// deliberately makes no production durability, availability, or SLA claim.
type RunSettlementStoreV2 struct {
	mu                    sync.Mutex
	clock                 func() time.Time
	runs                  map[string]core.AgentRunRecord
	active                map[string]core.AgentRunID
	plans                 map[string]ports.RunSettlementPlanFactV2
	certifications        map[string]ports.RunSettlementPlanCertificationAssociationV3
	startConfirmations    map[string]ports.RunStartConfirmationFactV3
	closures              map[string]map[uint64]control.RunSettlementClosureFactV2
	currentClosures       map[string]control.RunSettlementClosurePointerFactV2
	decisions             map[string]control.RunSettlementDecisionFactV2
	progress              map[string]control.RunTerminationProgressFactV2
	reports               map[string]control.RunTerminationReportV2
	loseNextBundleReply   bool
	loseNextClosureReply  bool
	loseNextCommitReply   bool
	loseNextProgressReply bool
	loseNextReportReply   bool
}

func NewRunSettlementStoreV2(clock func() time.Time) *RunSettlementStoreV2 {
	if clock == nil {
		clock = time.Now
	}
	return &RunSettlementStoreV2{
		clock: clock, runs: map[string]core.AgentRunRecord{}, active: map[string]core.AgentRunID{},
		plans: map[string]ports.RunSettlementPlanFactV2{}, certifications: map[string]ports.RunSettlementPlanCertificationAssociationV3{}, startConfirmations: map[string]ports.RunStartConfirmationFactV3{}, closures: map[string]map[uint64]control.RunSettlementClosureFactV2{}, currentClosures: map[string]control.RunSettlementClosurePointerFactV2{},
		decisions: map[string]control.RunSettlementDecisionFactV2{}, progress: map[string]control.RunTerminationProgressFactV2{}, reports: map[string]control.RunTerminationReportV2{},
	}
}

func (s *RunSettlementStoreV2) CreateRunBundleV3(ctx context.Context, request control.RunBundleCreateRequestV3) (control.RunBundleV3, error) {
	if err := contextError(ctx); err != nil {
		return control.RunBundleV3{}, err
	}
	if err := request.Validate(); err != nil {
		return control.RunBundleV3{}, err
	}
	association := request.Certification
	key := runKey(request.Run.Scope, request.Run.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if currentRun, exists := s.runs[key]; exists {
		currentPlan, planExists := s.plans[key]
		currentCertification, certificationExists := s.certifications[key]
		if !planExists || !certificationExists {
			return control.RunBundleV3{}, core.NewError(core.ErrorInternal, core.ReasonRunCompletionAtomicityBroken, "certified Run bundle is partially persisted")
		}
		if sameRunRecordV2(currentRun, request.Run) && sameRunSettlementPlanV2(currentPlan, request.Plan) && currentCertification == association {
			return control.RunBundleV3{Run: cloneRun(currentRun), Plan: cloneRunSettlementPlanV2(currentPlan), Certification: currentCertification}, nil
		}
		return control.RunBundleV3{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "certified Run bundle already binds different content")
	}
	if _, exists := s.plans[key]; exists {
		return control.RunBundleV3{}, core.NewError(core.ErrorInternal, core.ReasonRunCompletionAtomicityBroken, "Run Plan exists without its certified bundle")
	}
	if _, exists := s.certifications[key]; exists {
		return control.RunBundleV3{}, core.NewError(core.ErrorInternal, core.ReasonRunCompletionAtomicityBroken, "Run certification exists without its certified bundle")
	}
	if _, active := s.active[executionKey(request.Run.Scope)]; active {
		return control.RunBundleV3{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "execution scope already has an active Run")
	}
	run := cloneRun(request.Run)
	plan := cloneRunSettlementPlanV2(request.Plan)
	s.runs[key], s.plans[key], s.certifications[key], s.active[executionKey(request.Run.Scope)] = run, plan, association, run.ID
	if s.loseNextBundleReply {
		s.loseNextBundleReply = false
		return control.RunBundleV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected certified Run bundle reply loss")
	}
	return control.RunBundleV3{Run: cloneRun(run), Plan: cloneRunSettlementPlanV2(plan), Certification: association}, nil
}

func (s *RunSettlementStoreV2) InspectRunBundleV3(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (control.RunBundleV3, error) {
	if err := contextError(ctx); err != nil {
		return control.RunBundleV3{}, err
	}
	if err := scope.Validate(); err != nil {
		return control.RunBundleV3{}, err
	}
	key := runKey(scope, runID)
	s.mu.Lock()
	defer s.mu.Unlock()
	run, runExists := s.runs[key]
	plan, planExists := s.plans[key]
	certification, certificationExists := s.certifications[key]
	if !runExists && !planExists && !certificationExists {
		return control.RunBundleV3{}, core.NewError(core.ErrorNotFound, core.ReasonRunConflict, "certified Run bundle does not exist")
	}
	if !runExists || !planExists || !certificationExists {
		return control.RunBundleV3{}, core.NewError(core.ErrorInternal, core.ReasonRunCompletionAtomicityBroken, "certified Run bundle is partially persisted")
	}
	return control.RunBundleV3{Run: cloneRun(run), Plan: cloneRunSettlementPlanV2(plan), Certification: certification}, nil
}

func (s *RunSettlementStoreV2) CommitRunStartV3(ctx context.Context, request control.CommitRunStartRequestV3) (ports.RunStartConfirmationEnvelopeV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.RunStartConfirmationEnvelopeV3{}, err
	}
	if err := request.NextRun.Validate(); err != nil {
		return ports.RunStartConfirmationEnvelopeV3{}, err
	}
	if err := request.Confirmation.Validate(); err != nil {
		return ports.RunStartConfirmationEnvelopeV3{}, err
	}
	key := runKey(request.NextRun.Scope, request.NextRun.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.runs[key]
	if !exists {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorNotFound, core.ReasonRunConflict, "pending Run does not exist")
	}
	certification, certified := s.certifications[key]
	if !certified {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementPlanConflict, "Run start requires a certified V3 bundle")
	}
	if existing, confirmed := s.startConfirmations[key]; confirmed {
		envelope := ports.RunStartConfirmationEnvelopeV3{Run: cloneRun(current), Certification: certification, Confirmation: cloneRunStartConfirmationV3(existing)}
		left, leftErr := existing.Digest, existing.Validate()
		right, rightErr := request.Confirmation.Digest, request.Confirmation.Validate()
		if leftErr == nil && rightErr == nil && left == right && envelope.Validate() == nil {
			return envelope, nil
		}
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "Run already binds another immutable start confirmation")
	}
	identity, _ := ports.RunIdentityDigestV2(request.NextRun)
	if current.Status != core.RunPending || current.Revision != request.ExpectedRunRevision || request.NextRun.Status != core.RunRunning || request.NextRun.Revision != current.Revision+1 || identity != request.Confirmation.RunIdentityDigest || request.NextRun.ID != request.Confirmation.RunID || request.NextRun.Revision != request.Confirmation.RunRevision || request.NextRun.StartedAt.UnixNano() != request.Confirmation.StartedUnixNano || !ports.SameExecutionScopeV2(current.Scope, request.NextRun.Scope) {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "Run start commit does not bind the exact pending Run transition")
	}
	s.runs[key] = cloneRun(request.NextRun)
	s.startConfirmations[key] = cloneRunStartConfirmationV3(request.Confirmation)
	envelope := ports.RunStartConfirmationEnvelopeV3{Run: cloneRun(request.NextRun), Certification: certification, Confirmation: cloneRunStartConfirmationV3(request.Confirmation)}
	return envelope, envelope.Validate()
}

func (s *RunSettlementStoreV2) InspectRunStartConfirmationV3(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (ports.RunStartConfirmationEnvelopeV3, error) {
	if err := contextError(ctx); err != nil {
		return ports.RunStartConfirmationEnvelopeV3{}, err
	}
	if err := scope.Validate(); err != nil {
		return ports.RunStartConfirmationEnvelopeV3{}, err
	}
	key := runKey(scope, runID)
	s.mu.Lock()
	defer s.mu.Unlock()
	run, runExists := s.runs[key]
	confirmation, proofExists := s.startConfirmations[key]
	certification, certificationExists := s.certifications[key]
	if !runExists || !proofExists || !certificationExists {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorNotFound, core.ReasonRunConflict, "Run start confirmation does not exist")
	}
	envelope := ports.RunStartConfirmationEnvelopeV3{Run: cloneRun(run), Certification: certification, Confirmation: cloneRunStartConfirmationV3(confirmation)}
	if err := envelope.Validate(); err != nil {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorInternal, core.ReasonRunCompletionAtomicityBroken, "Run and start confirmation atomicity is broken")
	}
	return envelope, nil
}

func (s *RunSettlementStoreV2) LoseNextBundleReply()   { s.setReplyLoss(&s.loseNextBundleReply) }
func (s *RunSettlementStoreV2) LoseNextClosureReply()  { s.setReplyLoss(&s.loseNextClosureReply) }
func (s *RunSettlementStoreV2) LoseNextCommitReply()   { s.setReplyLoss(&s.loseNextCommitReply) }
func (s *RunSettlementStoreV2) LoseNextProgressReply() { s.setReplyLoss(&s.loseNextProgressReply) }
func (s *RunSettlementStoreV2) LoseNextReportReply()   { s.setReplyLoss(&s.loseNextReportReply) }

func (s *RunSettlementStoreV2) setReplyLoss(target *bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	*target = true
}

func (s *RunSettlementStoreV2) CreateRunBundleV2(ctx context.Context, request control.RunBundleCreateRequestV2) (control.RunBundleV2, error) {
	if err := contextError(ctx); err != nil {
		return control.RunBundleV2{}, err
	}
	if err := request.Validate(); err != nil {
		return control.RunBundleV2{}, err
	}
	key := runKey(request.Run.Scope, request.Run.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if currentRun, runExists := s.runs[key]; runExists {
		currentPlan, planExists := s.plans[key]
		if !planExists {
			return control.RunBundleV2{}, core.NewError(core.ErrorInternal, core.ReasonRunCompletionAtomicityBroken, "Run exists without its create-time settlement Plan")
		}
		if sameRunRecordV2(currentRun, request.Run) && sameRunSettlementPlanV2(currentPlan, request.Plan) {
			return control.RunBundleV2{Run: cloneRun(currentRun), Plan: cloneRunSettlementPlanV2(currentPlan)}, nil
		}
		return control.RunBundleV2{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "Run bundle identity already binds different content")
	}
	if _, planExists := s.plans[key]; planExists {
		return control.RunBundleV2{}, core.NewError(core.ErrorInternal, core.ReasonRunCompletionAtomicityBroken, "settlement Plan exists without its Run")
	}
	instanceKey := executionKey(request.Run.Scope)
	if _, active := s.active[instanceKey]; active {
		return control.RunBundleV2{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "execution scope already has an active Run")
	}
	run := cloneRun(request.Run)
	plan := cloneRunSettlementPlanV2(request.Plan)
	s.runs[key], s.plans[key], s.active[instanceKey] = run, plan, run.ID
	if s.loseNextBundleReply {
		s.loseNextBundleReply = false
		return control.RunBundleV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected atomic Run bundle reply loss")
	}
	return control.RunBundleV2{Run: cloneRun(run), Plan: cloneRunSettlementPlanV2(plan)}, nil
}

// CreateRun remains a restricted legacy compatibility primitive. V2 callers
// must use CreateRunBundleV2 so a dispatchable Run can never lack its Plan.
func (s *RunSettlementStoreV2) CreateRun(ctx context.Context, initial core.AgentRunRecord) (core.AgentRunRecord, error) {
	if err := contextError(ctx); err != nil {
		return core.AgentRunRecord{}, err
	}
	if err := initial.Validate(); err != nil {
		return core.AgentRunRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runKey(initial.Scope, initial.ID)
	if _, exists := s.runs[key]; exists {
		return core.AgentRunRecord{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "legacy Run fact already exists")
	}
	if _, exists := s.active[executionKey(initial.Scope)]; exists {
		return core.AgentRunRecord{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "execution scope already has an active Run")
	}
	s.runs[key] = cloneRun(initial)
	s.active[executionKey(initial.Scope)] = initial.ID
	return cloneRun(initial), nil
}

func (s *RunSettlementStoreV2) InspectRun(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (core.AgentRunRecord, error) {
	if err := contextError(ctx); err != nil {
		return core.AgentRunRecord{}, err
	}
	if err := scope.Validate(); err != nil {
		return core.AgentRunRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, exists := s.runs[runKey(scope, runID)]
	if !exists || !ports.SameExecutionScopeV2(run.Scope, scope) {
		return core.AgentRunRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Run fact does not exist")
	}
	return cloneRun(run), nil
}

func (s *RunSettlementStoreV2) InspectActiveRun(ctx context.Context, scope core.ExecutionScope) (core.AgentRunRecord, error) {
	if err := contextError(ctx); err != nil {
		return core.AgentRunRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id, exists := s.active[executionKey(scope)]
	if !exists {
		return core.AgentRunRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "execution scope has no active Run")
	}
	run, exists := s.runs[runKey(scope, id)]
	if !exists {
		return core.AgentRunRecord{}, core.NewError(core.ErrorInternal, core.ReasonRunCompletionAtomicityBroken, "active Run index has no Run fact")
	}
	return cloneRun(run), nil
}

func (s *RunSettlementStoreV2) CompareAndSwapRun(ctx context.Context, request control.RunFactCASRequest) (core.AgentRunRecord, error) {
	if err := contextError(ctx); err != nil {
		return core.AgentRunRecord{}, err
	}
	if err := request.Validate(); err != nil {
		return core.AgentRunRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runKey(request.Next.Scope, request.Next.ID)
	current, exists := s.runs[key]
	if !exists {
		return core.AgentRunRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Run fact does not exist")
	}
	if current.Revision != request.ExpectedRevision {
		if sameRunRecordV2(current, request.Next) {
			return cloneRun(current), nil
		}
		return core.AgentRunRecord{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Run fact revision does not match CAS precondition")
	}
	if err := control.ValidateRunFactTransition(current, request.Next); err != nil {
		return core.AgentRunRecord{}, err
	}
	next := cloneRun(request.Next)
	s.runs[key] = next
	if next.Status == core.RunTerminal {
		delete(s.active, executionKey(next.Scope))
	} else {
		s.active[executionKey(next.Scope)] = next.ID
	}
	return cloneRun(next), nil
}

func (s *RunSettlementStoreV2) InspectRunSettlementPlanV2(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (ports.RunSettlementPlanFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.RunSettlementPlanFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	plan, exists := s.plans[runKey(scope, runID)]
	if !exists || !ports.SameExecutionScopeV2(plan.ExecutionScope, scope) {
		return ports.RunSettlementPlanFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonRunSettlementPlanConflict, "Run settlement Plan does not exist")
	}
	return cloneRunSettlementPlanV2(plan), nil
}

func (s *RunSettlementStoreV2) CreateRunSettlementClosureAttemptV2(ctx context.Context, proposed control.RunSettlementClosureFactV2) (control.RunSettlementClosureAttemptResultV2, error) {
	if err := contextError(ctx); err != nil {
		return control.RunSettlementClosureAttemptResultV2{}, err
	}
	if err := proposed.Validate(); err != nil {
		return control.RunSettlementClosureAttemptResultV2{}, err
	}
	key := runKey(proposed.ExecutionScope, proposed.RunID)
	s.mu.Lock()
	defer s.mu.Unlock()
	attempts := s.closures[key]
	if existing, exists := attempts[proposed.Attempt]; exists {
		if sameRunSettlementClosureV2(existing, proposed) {
			pointer := s.currentClosures[key]
			return control.RunSettlementClosureAttemptResultV2{Closure: cloneRunSettlementClosureV2(existing), Pointer: pointer}, nil
		}
		return control.RunSettlementClosureAttemptResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementClosureConflict, "Closure attempt already binds different immutable content")
	}
	if _, decided := s.decisions[key]; decided {
		return control.RunSettlementClosureAttemptResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRunCompletionConflict, "terminal Decision forbids a new Closure attempt")
	}
	run, runExists := s.runs[key]
	plan, planExists := s.plans[key]
	if !runExists || !planExists || run.Status != core.RunStopping || run.Revision != proposed.RunRevision {
		return control.RunSettlementClosureAttemptResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementClosureConflict, "Closure requires the exact stopping Run and create-time Plan")
	}
	planRef, _ := plan.RefV2()
	if proposed.Plan != planRef {
		return control.RunSettlementClosureAttemptResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementPlanConflict, "Closure Plan ref drifted")
	}
	if err := validateClosureParticipantsAgainstPlanV2(proposed, plan, s.clock()); err != nil {
		return control.RunSettlementClosureAttemptResultV2{}, err
	}
	current, hasCurrent := s.currentClosures[key]
	if !hasCurrent {
		if proposed.Attempt != 1 || proposed.PreviousClosureDigest != ports.EvidenceGenesisDigestV2 {
			return control.RunSettlementClosureAttemptResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementClosureConflict, "first Closure must be attempt one with genesis predecessor")
		}
	} else {
		previous := attempts[current.Current.Attempt]
		previousDigest, _ := previous.DigestV2()
		if proposed.Attempt != current.Current.Attempt+1 || proposed.PreviousClosureDigest != previousDigest {
			return control.RunSettlementClosureAttemptResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementClosureConflict, "Closure attempt does not extend the current immutable chain")
		}
	}
	if attempts == nil {
		attempts = map[uint64]control.RunSettlementClosureFactV2{}
		s.closures[key] = attempts
	}
	ref, _ := proposed.RefV2()
	pointer := control.RunSettlementClosurePointerFactV2{ContractVersion: ports.RunSettlementContractVersionV2, Revision: 1, RunID: proposed.RunID, RunIdentityDigest: proposed.RunIdentityDigest, ExecutionScopeDigest: proposed.ExecutionScopeDigest, Current: ref, UpdatedUnixNano: proposed.CreatedUnixNano}
	if hasCurrent {
		pointer.Revision = current.Revision + 1
	}
	attempts[proposed.Attempt] = cloneRunSettlementClosureV2(proposed)
	s.currentClosures[key] = pointer
	if s.loseNextClosureReply {
		s.loseNextClosureReply = false
		return control.RunSettlementClosureAttemptResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Closure attempt create reply loss")
	}
	return control.RunSettlementClosureAttemptResultV2{Closure: cloneRunSettlementClosureV2(proposed), Pointer: pointer}, nil
}

func (s *RunSettlementStoreV2) InspectRunSettlementClosureAttemptV2(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID, attempt uint64) (control.RunSettlementClosureFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.RunSettlementClosureFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	closure, exists := s.closures[runKey(scope, runID)][attempt]
	if !exists {
		return control.RunSettlementClosureFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonRunSettlementClosureConflict, "Run settlement Closure attempt does not exist")
	}
	return cloneRunSettlementClosureV2(closure), nil
}

func (s *RunSettlementStoreV2) InspectCurrentRunSettlementClosureV2(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (control.RunSettlementClosureAttemptResultV2, error) {
	if err := contextError(ctx); err != nil {
		return control.RunSettlementClosureAttemptResultV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runKey(scope, runID)
	pointer, exists := s.currentClosures[key]
	if !exists {
		return control.RunSettlementClosureAttemptResultV2{}, core.NewError(core.ErrorNotFound, core.ReasonRunSettlementClosureConflict, "current Closure attempt does not exist")
	}
	closure, exists := s.closures[key][pointer.Current.Attempt]
	if !exists {
		return control.RunSettlementClosureAttemptResultV2{}, core.NewError(core.ErrorInternal, core.ReasonRunCompletionAtomicityBroken, "current Closure pointer has no immutable attempt")
	}
	return control.RunSettlementClosureAttemptResultV2{Closure: cloneRunSettlementClosureV2(closure), Pointer: pointer}, nil
}

func (s *RunSettlementStoreV2) CommitRunCompletionV2(ctx context.Context, request control.CommitRunCompletionRequestV2) (control.CommitRunCompletionResultV2, error) {
	if err := contextError(ctx); err != nil {
		return control.CommitRunCompletionResultV2{}, err
	}
	if err := request.Decision.Validate(); err != nil {
		return control.CommitRunCompletionResultV2{}, err
	}
	if err := request.InitialProgress.Validate(); err != nil {
		return control.CommitRunCompletionResultV2{}, err
	}
	if err := request.ExecutionScope.Validate(); err != nil {
		return control.CommitRunCompletionResultV2{}, err
	}
	key := runKey(request.ExecutionScope, request.Decision.RunID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if result, found, err := s.inspectCommittedLocked(key, request.Decision); found || err != nil {
		return result, err
	}
	pointer, pointerExists := s.currentClosures[key]
	closure, closureExists := s.closures[key][request.Decision.Closure.Attempt]
	plan, planExists := s.plans[key]
	if !closureExists || !planExists {
		return control.CommitRunCompletionResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementClosureConflict, "completion requires immutable Plan and Closure")
	}
	if !pointerExists || pointer.Revision != request.Decision.ClosurePointerRevision || pointer.Current != request.Decision.Closure {
		return control.CommitRunCompletionResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementClosureConflict, "Decision does not bind the current Closure attempt")
	}
	run, runExists := s.runs[key]
	if !runExists || run.Status != core.RunStopping || run.Revision != request.ExpectedRunRevision || request.Decision.ExpectedRunRevision != request.ExpectedRunRevision {
		return control.CommitRunCompletionResultV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "completion requires the exact stopping Run revision")
	}
	planRef, _ := plan.RefV2()
	closureRef, _ := closure.RefV2()
	decisionRef, _ := request.Decision.RefV2()
	if request.Decision.Plan != planRef || request.Decision.Closure != closureRef || request.Decision.RunIdentityDigest != closure.RunIdentityDigest || request.Decision.ExecutionScopeDigest != closure.ExecutionScopeDigest || !ports.SameExecutionScopeV2(request.ExecutionScope, run.Scope) || request.InitialProgress.RunID != run.ID || request.InitialProgress.ExecutionScopeDigest != closure.ExecutionScopeDigest || !ports.SameExecutionScopeV2(request.InitialProgress.ExecutionScope, run.Scope) || request.InitialProgress.Decision != decisionRef || request.InitialProgress.Revision != 1 {
		return control.CommitRunCompletionResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunCompletionConflict, "Decision, Run, Plan, Closure and initial Progress do not match")
	}
	if err := control.ValidateRunSettlementDecisionAgainstClosureV2(request.Decision, closure, plan); err != nil {
		return control.CommitRunCompletionResultV2{}, err
	}
	if err := validateCommitRequirementsAgainstPlanV2(request.Decision, request.InitialProgress, plan); err != nil {
		return control.CommitRunCompletionResultV2{}, err
	}
	nextRun := cloneRun(run)
	nextRun.Status = core.RunTerminal
	nextRun.Revision++
	nextRun.EndedAt = time.Unix(0, request.Decision.CreatedUnixNano)
	nextRun.Outcome = request.Decision.Outcome
	if err := control.ValidateRunFactTransition(run, nextRun); err != nil {
		return control.CommitRunCompletionResultV2{}, err
	}
	s.runs[runKey(run.Scope, run.ID)] = nextRun
	s.decisions[key] = cloneRunSettlementDecisionV2(request.Decision)
	s.progress[key] = cloneRunTerminationProgressV2(request.InitialProgress)
	delete(s.active, executionKey(run.Scope))
	result := control.CommitRunCompletionResultV2{Run: cloneRun(nextRun), Decision: cloneRunSettlementDecisionV2(request.Decision), Progress: cloneRunTerminationProgressV2(request.InitialProgress)}
	if s.loseNextCommitReply {
		s.loseNextCommitReply = false
		return control.CommitRunCompletionResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected atomic Run completion reply loss")
	}
	return result, nil
}

func (s *RunSettlementStoreV2) inspectCommittedLocked(key string, expected control.RunSettlementDecisionFactV2) (control.CommitRunCompletionResultV2, bool, error) {
	decision, decisionExists := s.decisions[key]
	progress, progressExists := s.progress[key]
	run, runExists := s.runs[key]
	if !decisionExists && !progressExists && (!runExists || run.Status != core.RunTerminal) {
		return control.CommitRunCompletionResultV2{}, false, nil
	}
	if !decisionExists || !progressExists || !runExists || run.Status != core.RunTerminal || run.Outcome != decision.Outcome {
		return control.CommitRunCompletionResultV2{}, true, core.NewError(core.ErrorInternal, core.ReasonRunCompletionAtomicityBroken, "Decision, terminal Run and Progress are only partially visible")
	}
	if !sameRunSettlementDecisionV2(decision, expected) {
		return control.CommitRunCompletionResultV2{}, true, core.NewError(core.ErrorConflict, core.ReasonRunCompletionConflict, "terminal Run already binds a different immutable Decision")
	}
	return control.CommitRunCompletionResultV2{Run: cloneRun(run), Decision: cloneRunSettlementDecisionV2(decision), Progress: cloneRunTerminationProgressV2(progress)}, true, nil
}

func (s *RunSettlementStoreV2) InspectRunSettlementDecisionV2(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (control.RunSettlementDecisionFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.RunSettlementDecisionFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	decision, exists := s.decisions[runKey(scope, runID)]
	if !exists {
		return control.RunSettlementDecisionFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonRunCompletionConflict, "Run settlement Decision does not exist")
	}
	return cloneRunSettlementDecisionV2(decision), nil
}

func (s *RunSettlementStoreV2) InspectRunTerminationProgressV2(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (control.RunTerminationProgressFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.RunTerminationProgressFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	progress, exists := s.progress[runKey(scope, runID)]
	if !exists {
		return control.RunTerminationProgressFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonTerminationProgressConflict, "Run termination Progress does not exist")
	}
	return cloneRunTerminationProgressV2(progress), nil
}

func (s *RunSettlementStoreV2) CompareAndSwapRunTerminationProgressV2(ctx context.Context, request control.RunTerminationProgressCASRequestV2) (control.RunTerminationProgressFactV2, error) {
	if err := contextError(ctx); err != nil {
		return control.RunTerminationProgressFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := runKey(request.Next.ExecutionScope, request.Next.RunID)
	current, exists := s.progress[key]
	if !exists {
		return control.RunTerminationProgressFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonTerminationProgressConflict, "Run termination Progress does not exist")
	}
	if current.Revision != request.ExpectedRevision {
		if sameRunTerminationProgressV2(current, request.Next) {
			return cloneRunTerminationProgressV2(current), nil
		}
		return control.RunTerminationProgressFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "termination Progress revision does not match CAS precondition")
	}
	if err := control.ValidateRunTerminationProgressTransitionV2(current, request.Next, s.clock()); err != nil {
		return control.RunTerminationProgressFactV2{}, err
	}
	s.progress[key] = cloneRunTerminationProgressV2(request.Next)
	if s.loseNextProgressReply {
		s.loseNextProgressReply = false
		return control.RunTerminationProgressFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected termination Progress CAS reply loss")
	}
	return cloneRunTerminationProgressV2(request.Next), nil
}

func (s *RunSettlementStoreV2) CreateRunTerminationReportV2(ctx context.Context, report control.RunTerminationReportV2) (control.RunTerminationReportV2, error) {
	if err := contextError(ctx); err != nil {
		return control.RunTerminationReportV2{}, err
	}
	if err := report.Validate(); err != nil {
		return control.RunTerminationReportV2{}, err
	}
	key := runKey(report.ExecutionScope, report.RunID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.reports[key]; ok {
		left, _ := existing.DigestV2()
		right, _ := report.DigestV2()
		if left == right {
			return existing, nil
		}
		return control.RunTerminationReportV2{}, core.NewError(core.ErrorConflict, core.ReasonTerminationReportIncomplete, "termination Report already binds different content")
	}
	progress, progressOK := s.progress[key]
	decision, decisionOK := s.decisions[key]
	run, runOK := s.runs[key]
	if !progressOK || !decisionOK || !runOK || run.Status != core.RunTerminal {
		return control.RunTerminationReportV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonTerminationReportIncomplete, "termination Report does not bind exact terminal facts")
	}
	expected, expectedErr := control.BuildRunTerminationReportV2(run, decision, progress)
	actualDigest, actualErr := report.DigestV2()
	expectedDigest, digestErr := expected.DigestV2()
	if expectedErr != nil || actualErr != nil || digestErr != nil || actualDigest != expectedDigest {
		return control.RunTerminationReportV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonTerminationReportIncomplete, "termination Report differs from the Fact Owner reconstructed terminal facts")
	}
	s.reports[key] = report
	if s.loseNextReportReply {
		s.loseNextReportReply = false
		return control.RunTerminationReportV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected termination Report reply loss")
	}
	return report, nil
}

func (s *RunSettlementStoreV2) InspectRunTerminationReportV2(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (control.RunTerminationReportV2, error) {
	if err := contextError(ctx); err != nil {
		return control.RunTerminationReportV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	report, ok := s.reports[runKey(scope, runID)]
	if !ok {
		return control.RunTerminationReportV2{}, core.NewError(core.ErrorNotFound, core.ReasonTerminationReportIncomplete, "termination Report does not exist")
	}
	return report, nil
}

func cloneRunSettlementPlanV2(f ports.RunSettlementPlanFactV2) ports.RunSettlementPlanFactV2 {
	f.ExecutionScope = cloneScope(f.ExecutionScope)
	f.Requirements = append([]ports.RunSettlementRequirementV2{}, f.Requirements...)
	return f
}

func cloneRunSettlementClosureV2(f control.RunSettlementClosureFactV2) control.RunSettlementClosureFactV2 {
	f.ExecutionScope = cloneScope(f.ExecutionScope)
	f.Execution.ExecutionScope = cloneScope(f.Execution.ExecutionScope)
	f.Participants = append([]control.RunSettlementClosureParticipantV2{}, f.Participants...)
	if f.Claim != nil {
		claim := *f.Claim
		claim.ExecutionScope = cloneScope(claim.ExecutionScope)
		f.Claim = &claim
	}
	return f
}

func cloneRunSettlementDecisionV2(f control.RunSettlementDecisionFactV2) control.RunSettlementDecisionFactV2 {
	f.Resolutions = append([]control.RunSettlementResolutionV2{}, f.Resolutions...)
	cloneSettlementResolutionPointersV2(f.Resolutions)
	if f.Claim != nil {
		claim := *f.Claim
		claim.ExecutionScope = cloneScope(claim.ExecutionScope)
		f.Claim = &claim
	}
	return f
}

func cloneRunTerminationProgressV2(f control.RunTerminationProgressFactV2) control.RunTerminationProgressFactV2 {
	f.ExecutionScope = cloneScope(f.ExecutionScope)
	f.Items = append([]control.RunSettlementResolutionV2{}, f.Items...)
	cloneSettlementResolutionPointersV2(f.Items)
	return f
}

func cloneRunStartConfirmationV3(f ports.RunStartConfirmationFactV3) ports.RunStartConfirmationFactV3 {
	f.ExecutionScope = cloneScope(f.ExecutionScope)
	if f.Attempt.Observation != nil {
		observation := *f.Attempt.Observation
		f.Attempt.Observation = &observation
	}
	if f.Attempt.Settlement != nil {
		settlement := *f.Attempt.Settlement
		settlement.Evidence = append([]ports.EvidenceRecordRefV2{}, settlement.Evidence...)
		if settlement.Observation != nil {
			observation := *settlement.Observation
			settlement.Observation = &observation
		}
		if settlement.InspectionEffect != nil {
			inspection := *settlement.InspectionEffect
			if inspection.Delegation != nil {
				delegation := *inspection.Delegation
				inspection.Delegation = &delegation
			}
			settlement.InspectionEffect = &inspection
		}
		if settlement.InspectionSettlement != nil {
			inspection := *settlement.InspectionSettlement
			inspection.Evidence = append([]ports.EvidenceRecordRefV2{}, inspection.Evidence...)
			if inspection.Observation != nil {
				observation := *inspection.Observation
				inspection.Observation = &observation
			}
			if inspection.DomainResultSchema != nil {
				schema := *inspection.DomainResultSchema
				inspection.DomainResultSchema = &schema
			}
			settlement.InspectionSettlement = &inspection
		}
		if settlement.DomainResultSchema != nil {
			schema := *settlement.DomainResultSchema
			settlement.DomainResultSchema = &schema
		}
		f.Attempt.Settlement = &settlement
	}
	return f
}

func cloneSettlementResolutionPointersV2(values []control.RunSettlementResolutionV2) {
	for index := range values {
		if values[index].Participant != nil {
			ref := *values[index].Participant
			values[index].Participant = &ref
		}
	}
}

func validateClosureParticipantsAgainstPlanV2(closure control.RunSettlementClosureFactV2, plan ports.RunSettlementPlanFactV2, now time.Time) error {
	expected := make(map[ports.NamespacedNameV2]ports.RunSettlementRequirementV2)
	for _, requirement := range plan.Requirements {
		if requirement.Kind == ports.RunRequirementExecutionTruth || requirement.Kind == ports.RunRequirementEffects {
			continue
		}
		expected[requirement.ID] = requirement
	}
	if len(closure.Participants) != len(expected) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementParticipantMissing, "Closure does not freeze every required participant")
	}
	for _, participant := range closure.Participants {
		requirement, exists := expected[participant.RequirementID]
		requirementDigest, _ := requirement.DigestV2()
		planRef, _ := plan.RefV2()
		fact := participant.ParticipantFact
		if !exists || requirementDigest != participant.RequirementDigest || fact.RunID != plan.RunID || fact.RunIdentityDigest != plan.RunIdentityDigest || fact.ExecutionScopeDigest != plan.ExecutionScopeDigest || fact.Plan != planRef || fact.RequirementID != requirement.ID || fact.RequirementDigest != requirementDigest || fact.SubjectDigest != requirement.SubjectDigest || fact.Owner != requirement.Owner || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementParticipantStale, "Closure participant does not match its Plan requirement")
		}
		if err := participant.PolicyFact.ValidateCurrent(requirement.Policy, plan, requirement.ID, now); err != nil {
			return err
		}
		if fact.Disposition == ports.RunSettlementOperationNotRequired && (fact.Policy == nil || *fact.Policy != requirement.Policy || !participant.PolicyFact.AllowOperationNotRequired) {
			return core.NewError(core.ErrorForbidden, core.ReasonRunSettlementRequirementInvalid, "Closure not-required participant lacks exact current Policy authorization")
		}
	}
	return nil
}

func validateCommitRequirementsAgainstPlanV2(decision control.RunSettlementDecisionFactV2, progress control.RunTerminationProgressFactV2, plan ports.RunSettlementPlanFactV2) error {
	completion := make(map[ports.NamespacedNameV2]ports.RunSettlementRequirementV2)
	termination := make(map[ports.NamespacedNameV2]ports.RunSettlementRequirementV2)
	for _, requirement := range plan.Requirements {
		if requirement.Phase == ports.RunSettlementPhaseCompletion {
			completion[requirement.ID] = requirement
		} else {
			termination[requirement.ID] = requirement
		}
	}
	if len(decision.Resolutions) != len(completion) || len(progress.Items) != len(termination) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "Decision and Progress do not cover the exact Plan barriers")
	}
	for _, resolution := range decision.Resolutions {
		requirement, exists := completion[resolution.RequirementID]
		if !exists || resolution.Kind != requirement.Kind || resolution.Policy != requirement.Policy {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "Decision resolution drifted from its Plan")
		}
	}
	for _, resolution := range progress.Items {
		requirement, exists := termination[resolution.RequirementID]
		if !exists || resolution.Kind != requirement.Kind || resolution.Policy != requirement.Policy {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "termination Progress drifted from its Plan")
		}
	}
	return nil
}

func sameRunRecordV2(left, right core.AgentRunRecord) bool {
	leftDigest, leftErr := ports.RunIdentityDigestV2(left)
	rightDigest, rightErr := ports.RunIdentityDigestV2(right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest && left.Status == right.Status && left.Revision == right.Revision && left.StartedAt.Equal(right.StartedAt) && left.EndedAt.Equal(right.EndedAt) && left.Outcome == right.Outcome
}

func sameRunSettlementPlanV2(left, right ports.RunSettlementPlanFactV2) bool {
	leftDigest, leftErr := left.DigestV2()
	rightDigest, rightErr := right.DigestV2()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func sameRunSettlementClosureV2(left, right control.RunSettlementClosureFactV2) bool {
	leftDigest, leftErr := left.DigestV2()
	rightDigest, rightErr := right.DigestV2()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func sameRunSettlementDecisionV2(left, right control.RunSettlementDecisionFactV2) bool {
	leftDigest, leftErr := left.DigestV2()
	rightDigest, rightErr := right.DigestV2()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func sameRunTerminationProgressV2(left, right control.RunTerminationProgressFactV2) bool {
	leftDigest, leftErr := left.DigestV2()
	rightDigest, rightErr := right.DigestV2()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

var _ control.RunSettlementFactPortV2 = (*RunSettlementStoreV2)(nil)
