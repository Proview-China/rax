package fakes

import (
	"context"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/admission"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type activationFacts struct {
	attempt admission.ActivationAttempt
}

func (s *FactStore) CreateActivationAttempt(ctx context.Context, attempt admission.ActivationAttempt) (admission.ActivationAttempt, error) {
	if err := contextError(ctx); err != nil {
		return admission.ActivationAttempt{}, err
	}
	if err := attempt.Validate(s.clock()); err != nil {
		return admission.ActivationAttempt{}, err
	}
	if attempt.Stage != admission.StageProposed || attempt.Recovery != admission.RecoveryNormal || attempt.Revision != 1 {
		return admission.ActivationAttempt{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "new activation attempt must be proposed, normal and revision one")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.evidenceAvailable {
		return admission.ActivationAttempt{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "activation journal is unavailable")
	}
	if _, exists := s.activations[attempt.ID]; exists {
		return admission.ActivationAttempt{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "activation attempt already exists")
	}
	s.activations[attempt.ID] = &activationFacts{attempt: cloneActivationAttempt(attempt)}
	return cloneActivationAttempt(attempt), nil
}

func (s *FactStore) InspectActivationAttempt(ctx context.Context, id string) (admission.ActivationAttempt, error) {
	if err := contextError(ctx); err != nil {
		return admission.ActivationAttempt{}, err
	}
	if strings.TrimSpace(id) == "" {
		return admission.ActivationAttempt{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "activation attempt id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	facts, exists := s.activations[id]
	if !exists {
		return admission.ActivationAttempt{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "activation attempt does not exist")
	}
	return cloneActivationAttempt(facts.attempt), nil
}

func (s *FactStore) CompareAndSwapActivation(ctx context.Context, next admission.ActivationAttempt, transition admission.TransitionContext) (admission.ActivationAttempt, error) {
	if err := contextError(ctx); err != nil {
		return admission.ActivationAttempt{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.evidenceAvailable {
		return admission.ActivationAttempt{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "activation journal is unavailable")
	}
	facts, exists := s.activations[next.ID]
	if !exists {
		return admission.ActivationAttempt{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "activation attempt does not exist")
	}
	if err := admission.ValidateTransition(facts.attempt, next, transition, s.clock()); err != nil {
		return admission.ActivationAttempt{}, err
	}
	facts.attempt = cloneActivationAttempt(next)
	return cloneActivationAttempt(next), nil
}

func (s *FactStore) CommitActivation(ctx context.Context, request admission.ActivationCommitRequest) (admission.ActivationCommitResult, error) {
	if err := contextError(ctx); err != nil {
		return admission.ActivationCommitResult{}, err
	}
	if strings.TrimSpace(request.AttemptID) == "" || strings.TrimSpace(request.IdentityLeaseID) == "" || request.ExpectedAttemptRevision == 0 || request.ExpectedIdentityLeaseRevision == 0 || request.AuthorityEpoch == 0 {
		return admission.ActivationCommitResult{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "activation commit identities and revisions are required")
	}
	if err := request.SandboxLease.Validate(); err != nil {
		return admission.ActivationCommitResult{}, err
	}
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.evidenceAvailable {
		return admission.ActivationCommitResult{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "activation commit journal is unavailable")
	}
	facts, exists := s.activations[request.AttemptID]
	if !exists {
		return admission.ActivationCommitResult{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "activation attempt does not exist")
	}
	current := facts.attempt
	if current.Revision != request.ExpectedAttemptRevision || current.Stage != admission.StageSandboxReserved || current.Recovery != admission.RecoveryNormal {
		return admission.ActivationCommitResult{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "activation attempt is not ready to commit")
	}
	if current.SandboxReservation.State != admission.OperationConfirmedApplied || current.SandboxReservation.Reference != string(request.SandboxLease.ID) {
		return admission.ActivationCommitResult{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "confirmed quarantined sandbox reservation is required")
	}
	leaseFacts, exists := s.leaseIDs[request.IdentityLeaseID]
	if !exists {
		return admission.ActivationCommitResult{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "identity lease does not exist")
	}
	expireLeaseIfNeeded(leaseFacts, now)
	lease := leaseFacts.lease
	if lease.Revision != request.ExpectedIdentityLeaseRevision || lease.State != control.IdentityLeaseReserved || lease.Identity != current.Scope.Identity || lease.Lineage != current.Scope.Lineage || lease.AuthorityEpoch != request.AuthorityEpoch || lease.ActivationAttemptID != current.ID || current.IdentityLeaseID != lease.ID {
		return admission.ActivationCommitResult{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonIdentityLeaseStateInvalid, "identity lease does not match the activation attempt")
	}

	next := cloneActivationAttempt(current)
	next.Stage = admission.StageCommitted
	next.Scope.SandboxLease = &request.SandboxLease
	next.IdentityLeaseState = control.IdentityLeaseActive
	next.IdentityLeaseRevision = lease.Revision + 1
	next.Revision++
	next.UpdatedAt = now
	if err := admission.ValidateTransition(current, next, admission.TransitionContext{}, now); err != nil {
		return admission.ActivationCommitResult{}, err
	}
	lease.State = control.IdentityLeaseActive
	lease.Revision++
	leaseFacts.lease = lease
	facts.attempt = next
	return admission.ActivationCommitResult{Attempt: cloneActivationAttempt(next), IdentityLease: lease}, nil
}

func cloneActivationAttempt(attempt admission.ActivationAttempt) admission.ActivationAttempt {
	clone := attempt
	if attempt.Scope.SandboxLease != nil {
		lease := *attempt.Scope.SandboxLease
		clone.Scope.SandboxLease = &lease
	}
	if attempt.Snapshot != nil {
		snapshot := *attempt.Snapshot
		clone.Snapshot = &snapshot
	}
	return clone
}
