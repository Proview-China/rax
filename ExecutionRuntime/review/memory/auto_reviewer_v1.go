package memory

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func sameAutoReviewerAttemptSubjectV1(left, right contract.AutoReviewerAttemptV1) bool {
	return left.TenantID == right.TenantID && left.ID == right.ID && left.IdempotencyKey == right.IdempotencyKey && left.Case == right.Case && left.Round == right.Round && left.Assignment == right.Assignment && left.Target == right.Target && left.Rubric == right.Rubric && left.ContextFrameDigest == right.ContextFrameDigest && left.ReviewerID == right.ReviewerID && left.ReviewerAuthority == right.ReviewerAuthority && left.ReviewerBinding == right.ReviewerBinding && left.RouteID == right.RouteID && left.Operation == right.Operation && left.OperationDigest == right.OperationDigest && left.InvocationEffect == right.InvocationEffect && left.ResultSchema == right.ResultSchema && left.RoundOrdinal == right.RoundOrdinal && left.MaxCostMicros == right.MaxCostMicros && left.CreatedUnixNano == right.CreatedUnixNano && left.ExpiresUnixNano == right.ExpiresUnixNano
}

func (s *Store) validateAutoReviewerSubjectLockedV1(attempt contract.AutoReviewerAttemptV1) error {
	caseValue, err := inspectExact(s.caseHistory, key(attempt.TenantID, attempt.Case.ID), reviewport.ExactV1(attempt.Case.ID, attempt.Case.Revision, attempt.Case.Digest), func(v contract.ReviewCaseV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, "review case")
	if err != nil {
		return err
	}
	round, ok := s.rounds[key(attempt.TenantID, attempt.Round.ID)]
	if !ok {
		return notFound("review round")
	}
	assignment, err := inspectExact(s.assignmentHistory, key(attempt.TenantID, attempt.Assignment.ID), reviewport.ExactV1(attempt.Assignment.ID, attempt.Assignment.Revision, attempt.Assignment.Digest), func(v contract.ReviewerAssignmentV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, "review assignment")
	if err != nil {
		return err
	}
	target, err := inspectExact(s.targetHistory, key(attempt.TenantID, attempt.Target.ID), reviewport.ExactV1(attempt.Target.ID, attempt.Target.Revision, attempt.Target.Digest), func(v contract.TargetSnapshotV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, "review target")
	if err != nil {
		return err
	}
	rubric, ok := s.rubricHistory[key(attempt.TenantID, attempt.Rubric.ID)][attempt.Rubric.Revision]
	if !ok {
		return notFound("review Rubric")
	}
	if round.Revision != attempt.Round.Revision || round.Digest != attempt.Round.Digest || rubric.Digest != attempt.Rubric.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto reviewer exact Round or Rubric drifted")
	}
	currentCase, currentOK := s.cases[key(attempt.TenantID, attempt.Case.ID)]
	currentTarget, targetCurrentOK := s.targets[key(attempt.TenantID, attempt.Target.ID)]
	currentAssignment, assignmentCurrentOK := s.assignments[key(attempt.TenantID, attempt.Assignment.ID)]
	currentRubric, rubricCurrentOK := s.rubricCurrent[key(attempt.TenantID, attempt.Rubric.ID)]
	if !runtimeports.SameExecutionScopeV2(target.Scope, attempt.Operation.ExecutionScope) || target.RunID != attempt.Operation.RunID {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "auto reviewer Operation drifted from the exact Target scope")
	}
	if !currentOK || currentCase.Revision != caseValue.Revision || currentCase.Digest != caseValue.Digest || !targetCurrentOK || currentTarget.Revision != target.Revision || currentTarget.Digest != target.Digest || !assignmentCurrentOK || currentAssignment.Revision != assignment.Revision || currentAssignment.Digest != assignment.Digest || !rubricCurrentOK || currentRubric != attempt.Rubric || caseValue.State != contract.CaseReviewingV1 || caseValue.CurrentRoundID != round.ID || caseValue.CurrentAssignment != assignment.ID || caseValue.TargetID != target.ID || caseValue.TargetRevision != target.Revision || caseValue.TargetDigest != target.Digest || round.CaseID != caseValue.ID || round.CaseRevision > caseValue.Revision || round.TargetID != target.ID || round.TargetRevision != target.Revision || round.TargetDigest != target.Digest || round.Route != contract.RouteAutoV1 || round.AssignmentID != assignment.ID || round.ContextFrameDigest != attempt.ContextFrameDigest || round.RubricDigest != rubric.Digest || assignment.CaseID != caseValue.ID || assignment.CaseRevision != round.CaseRevision || assignment.RoundID != round.ID || assignment.RoundRevision != round.Revision || assignment.RoundDigest != round.Digest || assignment.TargetID != target.ID || assignment.TargetRevision != target.Revision || assignment.TargetDigest != target.Digest || assignment.Route != contract.RouteAutoV1 || assignment.State != contract.AssignmentClaimedV1 || assignment.ReviewerID != attempt.ReviewerID || assignment.ReviewerAuthority != attempt.ReviewerAuthority || assignment.ReviewerBinding != attempt.ReviewerBinding {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "auto reviewer Attempt drifted from exact Review facts")
	}
	return nil
}

func (s *Store) BeginAutoReviewerAttemptV1(ctx context.Context, mutation reviewport.BeginAutoReviewerAttemptMutationV1) (contract.AutoReviewerAttemptV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.AutoReviewerAttemptV1{}, err
	}
	attempt := mutation.Attempt
	if err := attempt.Validate(); err != nil {
		return contract.AutoReviewerAttemptV1{}, err
	}
	if attempt.Revision != 1 || attempt.State != contract.AutoReviewerAttemptPreparedV1 {
		return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "auto reviewer Attempt must start prepared at revision one")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	itemKey := key(attempt.TenantID, attempt.ID)
	if previous, ok := s.autoReviewerAttempts[itemKey]; ok {
		if previous.Digest == attempt.Digest {
			return clone(previous)
		}
		return contract.AutoReviewerAttemptV1{}, exists("auto reviewer Attempt")
	}
	if previousID, ok := s.autoReviewerKeys[key(attempt.TenantID, attempt.IdempotencyKey)]; ok {
		return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "auto reviewer idempotency key binds another Attempt: "+previousID)
	}
	if err := s.validateAutoReviewerSubjectLockedV1(attempt); err != nil {
		return contract.AutoReviewerAttemptV1{}, err
	}
	var priorRounds uint32
	for _, previous := range s.autoReviewerAttempts {
		if previous.TenantID == attempt.TenantID && previous.Target.ID == attempt.Target.ID {
			if previous.Round.ID == attempt.Round.ID || previous.Assignment.ID == attempt.Assignment.ID {
				return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "auto reviewer Round or Assignment already has an Attempt")
			}
			priorRounds++
		}
	}
	if attempt.RoundOrdinal != priorRounds+1 {
		return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "auto reviewer round ordinal does not follow Review-owned history")
	}
	rubric := s.rubricHistory[key(attempt.TenantID, attempt.Rubric.ID)][attempt.Rubric.Revision]
	if attempt.RoundOrdinal > rubric.Termination.MaxRounds {
		return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBudgetBindingStale, "auto reviewer maximum round count was reached")
	}
	copyValue, err := clone(attempt)
	if err != nil {
		return contract.AutoReviewerAttemptV1{}, err
	}
	s.autoReviewerAttempts[itemKey] = copyValue
	appendHistory(s.autoReviewerHistory, itemKey, attempt.Revision, copyValue)
	s.autoReviewerKeys[key(attempt.TenantID, attempt.IdempotencyKey)] = attempt.ID
	return clone(copyValue)
}

func (s *Store) advanceAutoReviewerAttemptLockedV1(expectedRef contract.ExactResourceRefV1, next contract.AutoReviewerAttemptV1, allowedFrom map[contract.AutoReviewerAttemptStateV1]bool) (contract.AutoReviewerAttemptV1, bool, error) {
	itemKey := key(next.TenantID, next.ID)
	current, ok := s.autoReviewerAttempts[itemKey]
	if !ok {
		return contract.AutoReviewerAttemptV1{}, false, notFound("auto reviewer Attempt")
	}
	if current.Digest == next.Digest {
		value, err := clone(current)
		return value, false, err
	}
	if current.ExactRef() != expectedRef {
		return contract.AutoReviewerAttemptV1{}, false, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "auto reviewer Attempt current ref drifted")
	}
	if !allowedFrom[current.State] || next.Revision != current.Revision+1 || !sameAutoReviewerAttemptSubjectV1(current, next) || next.UpdatedUnixNano <= current.UpdatedUnixNano {
		return contract.AutoReviewerAttemptV1{}, false, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "auto reviewer Attempt transition is invalid")
	}
	if err := validateAutoReviewerInvocationSourceV1(current, next); err != nil {
		return contract.AutoReviewerAttemptV1{}, false, err
	}
	if _, exists := s.autoReviewerHistory[itemKey][next.Revision]; exists {
		return contract.AutoReviewerAttemptV1{}, false, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "auto reviewer Attempt history revision already exists")
	}
	copyValue, err := clone(next)
	if err != nil {
		return contract.AutoReviewerAttemptV1{}, false, err
	}
	s.autoReviewerAttempts[itemKey] = copyValue
	appendHistory(s.autoReviewerHistory, itemKey, next.Revision, copyValue)
	value, err := clone(copyValue)
	return value, true, err
}

func validateAutoReviewerInvocationSourceV1(current, next contract.AutoReviewerAttemptV1) error {
	want := current.InvocationAttempt
	if current.State == contract.AutoReviewerAttemptPreparedV1 {
		ref := current.ExactRef()
		want = &ref
	}
	if want == nil || next.InvocationAttempt == nil || *next.InvocationAttempt != *want {
		return core.NewError(core.ErrorConflict, core.ReasonEffectUnknownOutcome, "auto reviewer successor lost the exact original invocation Attempt")
	}
	return nil
}

func (s *Store) MarkAutoReviewerWaitingInspectV1(ctx context.Context, mutation reviewport.MarkAutoReviewerWaitingInspectMutationV1) (reviewport.AutoReviewerInvocationStartClaimReceiptV1, error) {
	if err := checkContext(ctx); err != nil {
		return reviewport.AutoReviewerInvocationStartClaimReceiptV1{}, err
	}
	if err := mutation.Next.Validate(); err != nil {
		return reviewport.AutoReviewerInvocationStartClaimReceiptV1{}, err
	}
	if mutation.Next.State != contract.AutoReviewerAttemptWaitingInspectV1 {
		return reviewport.AutoReviewerInvocationStartClaimReceiptV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "auto reviewer invocation start claim must enter waiting_inspect")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt, applied, err := s.advanceAutoReviewerAttemptLockedV1(mutation.Expected, mutation.Next, map[contract.AutoReviewerAttemptStateV1]bool{contract.AutoReviewerAttemptPreparedV1: true})
	return reviewport.AutoReviewerInvocationStartClaimReceiptV1{Attempt: attempt, Applied: applied}, err
}

func (s *Store) RecordAutoReviewerObservationV1(ctx context.Context, mutation reviewport.RecordAutoReviewerObservationMutationV1) (contract.AutoReviewerAttemptV1, contract.ReviewerInvocationResultFactV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, err
	}
	next, observation, result := mutation.Next, mutation.Observation, mutation.DomainResult
	if err := next.Validate(); err != nil {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, err
	}
	if err := observation.Validate(); err != nil {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, err
	}
	if err := result.Validate(); err != nil {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, err
	}
	if next.State != contract.AutoReviewerAttemptObservedV1 || next.Observation == nil || next.DomainResult == nil || *next.Observation != observation.Ref() || *next.DomainResult != result.ExactRef() {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto reviewer observed transition lacks exact outputs")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.autoReviewerAttempts[key(next.TenantID, next.ID)]
	if !ok {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, notFound("auto reviewer Attempt")
	}
	if current.Digest == next.Digest {
		storedObservation, observationOK := s.autoReviewerObservations[key(next.TenantID, observation.ID)]
		storedResult, resultOK := s.domainResults[key(next.TenantID, result.ID)]
		if !observationOK || !resultOK || storedObservation.Digest != observation.Digest || storedResult.Digest != result.Digest {
			return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto reviewer canonical replay closure drifted")
		}
		attemptCopy, _ := clone(current)
		resultCopy, _ := clone(storedResult)
		return attemptCopy, resultCopy, nil
	}
	if current.ExactRef() != mutation.Expected || next.Revision != current.Revision+1 || !sameAutoReviewerAttemptSubjectV1(current, next) || current.State != contract.AutoReviewerAttemptWaitingInspectV1 || next.UpdatedUnixNano <= current.UpdatedUnixNano {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "auto reviewer observed transition lost its exact current Attempt")
	}
	if err := validateAutoReviewerInvocationSourceV1(current, next); err != nil {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, err
	}
	if err := s.validateAutoReviewerSubjectLockedV1(current); err != nil {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, err
	}
	if observation.TenantID != current.TenantID || observation.AttemptID != current.ID || next.InvocationAttempt == nil || observation.AttemptRevision != next.InvocationAttempt.Revision || observation.AttemptDigest != next.InvocationAttempt.Digest || observation.OperationDigest != current.OperationDigest || observation.RuntimeAttempt.OperationDigest != current.OperationDigest || observation.RuntimeAttempt.EffectID != current.InvocationEffect.EffectID || observation.RuntimeAttempt.IntentRevision != current.InvocationEffect.EffectRevision || observation.RuntimeAttempt.Delegation == nil || *observation.RuntimeAttempt.Delegation != observation.ProviderObservation.Delegation || observation.ResultSchema != current.ResultSchema || result.TenantID != current.TenantID || result.CaseID != current.Case.ID || result.CaseRevision != current.Case.Revision || result.RoundID != current.Round.ID || result.RoundRevision != current.Round.Revision || result.RoundDigest != current.Round.Digest || result.AssignmentID != current.Assignment.ID || result.AssignmentRevision != current.Assignment.Revision || result.AssignmentDigest != current.Assignment.Digest || result.TargetID != current.Target.ID || result.TargetRevision != current.Target.Revision || result.TargetDigest != current.Target.Digest || result.AttemptID != observation.RuntimeAttempt.AttemptID || result.ResultSchema != current.ResultSchema || result.ResultDigest != observation.Output.Digest || len(result.ObservationRefs) != 1 || result.ObservationRefs[0] != observation.ID {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto reviewer Observation or DomainResult provenance drifted")
	}
	rubric := s.rubricHistory[key(current.TenantID, current.Rubric.ID)][current.Rubric.Revision]
	if rubric.Digest != current.Rubric.Digest {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto reviewer exact Rubric drifted")
	}
	if err := rubric.ValidateAutoReviewerOutputV1(observation.Output); err != nil {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, err
	}
	if observation.Tokens > rubric.Termination.MaxTokens || observation.CostMicros > current.MaxCostMicros || observation.ObservedUnixNano-current.CreatedUnixNano > rubric.Termination.MaxDurationNanos {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBudgetBindingStale, "auto reviewer invocation exceeded an exact termination budget")
	}
	if _, ok := s.autoReviewerObservations[key(current.TenantID, observation.ID)]; ok {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, exists("auto reviewer Observation")
	}
	if _, ok := s.domainResults[key(current.TenantID, result.ID)]; ok {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, exists("review domain result")
	}
	if _, ok := s.autoReviewerHistory[key(current.TenantID, current.ID)][next.Revision]; ok {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "auto reviewer Attempt history revision already exists")
	}
	observationCopy, err := clone(observation)
	if err != nil {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, err
	}
	resultCopy, err := clone(result)
	if err != nil {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, err
	}
	nextCopy, err := clone(next)
	if err != nil {
		return contract.AutoReviewerAttemptV1{}, contract.ReviewerInvocationResultFactV1{}, err
	}
	s.autoReviewerObservations[key(current.TenantID, observation.ID)] = observationCopy
	s.domainResults[key(current.TenantID, result.ID)] = resultCopy
	s.autoReviewerAttempts[key(current.TenantID, current.ID)] = nextCopy
	appendHistory(s.autoReviewerHistory, key(current.TenantID, current.ID), next.Revision, nextCopy)
	attemptOut, _ := clone(nextCopy)
	resultOut, _ := clone(resultCopy)
	return attemptOut, resultOut, nil
}

func (s *Store) TerminateAutoReviewerAttemptV1(ctx context.Context, mutation reviewport.TerminateAutoReviewerAttemptMutationV1) (contract.AutoReviewerAttemptV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.AutoReviewerAttemptV1{}, err
	}
	if err := mutation.Next.Validate(); err != nil {
		return contract.AutoReviewerAttemptV1{}, err
	}
	if mutation.Next.State != contract.AutoReviewerAttemptFailedClosedV1 && mutation.Next.State != contract.AutoReviewerAttemptEscalatedV1 {
		return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "auto reviewer termination state is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, _, err := s.advanceAutoReviewerAttemptLockedV1(mutation.Expected, mutation.Next, map[contract.AutoReviewerAttemptStateV1]bool{contract.AutoReviewerAttemptPreparedV1: true, contract.AutoReviewerAttemptWaitingInspectV1: true})
	return value, err
}

func (s *Store) InspectAutoReviewerAttemptExactV1(ctx context.Context, tenant core.TenantID, ref contract.ExactResourceRefV1) (contract.AutoReviewerAttemptV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.AutoReviewerAttemptV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return inspectExact(s.autoReviewerHistory, key(tenant, ref.ID), reviewport.ExactV1(ref.ID, ref.Revision, ref.Digest), func(v contract.AutoReviewerAttemptV1) contract.FactIdentityV1 { return v.FactIdentityV1 }, "auto reviewer Attempt")
}

func (s *Store) InspectAutoReviewerAttemptCurrentV1(ctx context.Context, tenant core.TenantID, id string) (contract.AutoReviewerAttemptV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.AutoReviewerAttemptV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.autoReviewerAttempts[key(tenant, id)]
	if !ok {
		return contract.AutoReviewerAttemptV1{}, notFound("auto reviewer Attempt")
	}
	return clone(value)
}

func (s *Store) InspectAutoReviewerAttemptByIdempotencyV1(ctx context.Context, tenant core.TenantID, idempotency string) (contract.AutoReviewerAttemptV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.AutoReviewerAttemptV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.autoReviewerKeys[key(tenant, idempotency)]
	if !ok {
		return contract.AutoReviewerAttemptV1{}, notFound("auto reviewer Attempt")
	}
	return clone(s.autoReviewerAttempts[key(tenant, id)])
}

func (s *Store) InspectAutoReviewerObservationExactV1(ctx context.Context, tenant core.TenantID, ref contract.AutoReviewerInvocationObservationRefV1) (contract.AutoReviewerInvocationObservationV1, error) {
	if err := checkContext(ctx); err != nil {
		return contract.AutoReviewerInvocationObservationV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.AutoReviewerInvocationObservationV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.autoReviewerObservations[key(tenant, ref.ID)]
	if !ok {
		return contract.AutoReviewerInvocationObservationV1{}, notFound("auto reviewer Observation")
	}
	if value.Revision != ref.Revision || value.Digest != ref.Digest {
		return contract.AutoReviewerInvocationObservationV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto reviewer Observation exact ref drifted")
	}
	return clone(value)
}

func (s *Store) InspectAutoReviewTerminationCurrentV1(ctx context.Context, request reviewport.AutoReviewTerminationCurrentRequestV1) (reviewport.AutoReviewTerminationCurrentProjectionV1, error) {
	if err := checkContext(ctx); err != nil {
		return reviewport.AutoReviewTerminationCurrentProjectionV1{}, err
	}
	if err := request.Validate(); err != nil {
		return reviewport.AutoReviewTerminationCurrentProjectionV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inspectAutoReviewTerminationCurrentLockedV1(request)
}

func (s *Store) inspectAutoReviewTerminationCurrentLockedV1(request reviewport.AutoReviewTerminationCurrentRequestV1) (reviewport.AutoReviewTerminationCurrentProjectionV1, error) {
	target, targetOK := s.targets[key(request.TenantID, request.Target.ID)]
	caseValue, caseOK := s.cases[key(request.TenantID, request.Case.ID)]
	round, roundOK := s.rounds[key(request.TenantID, request.ExpectedRound.ID)]
	rubricRef, rubricCurrentOK := s.rubricCurrent[key(request.TenantID, request.Rubric.ID)]
	rubric, rubricHistoryOK := s.rubricHistory[key(request.TenantID, request.Rubric.ID)][request.Rubric.Revision]
	if !targetOK || !caseOK || !roundOK || !rubricCurrentOK || !rubricHistoryOK {
		return reviewport.AutoReviewTerminationCurrentProjectionV1{}, notFound("auto review termination current input")
	}
	targetRef := contract.ExactResourceRefV1{ID: target.ID, Revision: target.Revision, Digest: target.Digest}
	caseRef := contract.ExactResourceRefV1{ID: caseValue.ID, Revision: caseValue.Revision, Digest: caseValue.Digest}
	roundRef := contract.ExactResourceRefV1{ID: round.ID, Revision: round.Revision, Digest: round.Digest}
	if targetRef != request.Target || caseRef != request.Case || roundRef != request.ExpectedRound || rubricRef != request.Rubric || rubric.ExactRef() != request.Rubric || caseValue.TargetID != target.ID || caseValue.TargetRevision != target.Revision || caseValue.TargetDigest != target.Digest || caseValue.CurrentRoundID != round.ID || round.CaseID != caseValue.ID || round.TargetID != target.ID || round.TargetRevision != target.Revision || round.TargetDigest != target.Digest || round.Route != contract.RouteAutoV1 {
		return reviewport.AutoReviewTerminationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "auto review termination current inputs drifted")
	}
	now := time.Unix(0, request.CheckedUnixNano)
	for _, current := range []struct {
		created int64
		expires int64
	}{{target.CreatedUnixNano, target.ExpiresUnixNano}, {caseValue.CreatedUnixNano, caseValue.ExpiresUnixNano}, {round.CreatedUnixNano, round.ExpiresUnixNano}, {rubric.CreatedUnixNano, rubric.ExpiresUnixNano}} {
		if err := contract.ValidateNow(now, current.created, current.expires); err != nil {
			return reviewport.AutoReviewTerminationCurrentProjectionV1{}, err
		}
	}

	var roundCount, highestOrdinal, rejectionCount, repeatedFinding uint32
	findingCounts := make(map[core.Digest]uint32)
	for _, attempt := range s.autoReviewerAttempts {
		if attempt.TenantID != request.TenantID || attempt.Target.ID != request.Target.ID {
			continue
		}
		roundCount++
		if attempt.RoundOrdinal > highestOrdinal {
			highestOrdinal = attempt.RoundOrdinal
		}
		if attempt.State != contract.AutoReviewerAttemptObservedV1 || attempt.Observation == nil {
			continue
		}
		observation, ok := s.autoReviewerObservations[key(request.TenantID, attempt.Observation.ID)]
		if !ok || observation.Revision != attempt.Observation.Revision || observation.Digest != attempt.Observation.Digest {
			return reviewport.AutoReviewTerminationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "auto review termination Observation history drifted")
		}
		if observation.Output.Resolution == contract.ResolutionRejectV1 {
			rejectionCount++
		}
		seenInRound := make(map[core.Digest]struct{}, len(observation.Output.Findings))
		for _, finding := range observation.Output.Findings {
			signature, err := core.CanonicalJSONDigest("praxis.review.auto-review-termination-current", reviewport.AutoReviewTerminationCurrentContractVersionV1, "AutoReviewRepeatedFindingSignatureV1", struct {
				Category string `json:"category"`
				Anchor   string `json:"anchor"`
				Claim    string `json:"claim"`
			}{finding.Category, finding.Anchor, finding.Claim})
			if err != nil {
				return reviewport.AutoReviewTerminationCurrentProjectionV1{}, err
			}
			seenInRound[signature] = struct{}{}
		}
		for signature := range seenInRound {
			findingCounts[signature]++
			if findingCounts[signature] > repeatedFinding {
				repeatedFinding = findingCounts[signature]
			}
		}
	}
	if roundCount == 0 || highestOrdinal != roundCount {
		return reviewport.AutoReviewTerminationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "auto review termination round history is non-contiguous")
	}
	expires := target.ExpiresUnixNano
	for _, value := range []int64{caseValue.ExpiresUnixNano, round.ExpiresUnixNano, rubric.ExpiresUnixNano} {
		if value < expires {
			expires = value
		}
	}
	return reviewport.SealAutoReviewTerminationCurrentProjectionV1(reviewport.AutoReviewTerminationCurrentProjectionV1{
		TenantID: request.TenantID, Target: request.Target, Case: request.Case, Rubric: request.Rubric, ExpectedRound: request.ExpectedRound,
		RoundCount: roundCount, HighestRoundOrdinal: highestOrdinal, RepeatedFindingCount: repeatedFinding, RepeatedRejectionCount: rejectionCount,
		CheckedUnixNano: request.CheckedUnixNano, ExpiresUnixNano: expires,
	})
}
