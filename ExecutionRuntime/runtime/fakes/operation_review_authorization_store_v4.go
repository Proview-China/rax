package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationReviewAuthorizationStoreV4 is deterministic reference/test
// infrastructure. It makes no production durability, backend or SLA claim.
type OperationReviewAuthorizationStoreV4 struct {
	mu            sync.Mutex
	clock         func() time.Time
	facts         map[string]ports.OperationReviewAuthorizationFactV4
	loseCreate    bool
	loseCAS       bool
	createCommits uint64
}

func NewOperationReviewAuthorizationStoreV4(clock func() time.Time) *OperationReviewAuthorizationStoreV4 {
	return &OperationReviewAuthorizationStoreV4{clock: clock, facts: make(map[string]ports.OperationReviewAuthorizationFactV4)}
}

func (s *OperationReviewAuthorizationStoreV4) LoseNextCreateReplyV4() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseCreate = true
}

func (s *OperationReviewAuthorizationStoreV4) LoseNextCASReplyV4() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseCAS = true
}

func (s *OperationReviewAuthorizationStoreV4) CreateCommitCountV4() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createCommits
}

func (s *OperationReviewAuthorizationStoreV4) CreateOperationReviewAuthorizationV4(_ context.Context, fact ports.OperationReviewAuthorizationFactV4) (ports.OperationReviewAuthorizationFactV4, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fact.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if current, exists := s.facts[fact.ID]; exists {
		if current.Digest == fact.Digest {
			return cloneOperationReviewAuthorizationFactV4(current), nil
		}
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review Authorization ID contains different content")
	}
	if s.clock == nil || fact.Revision != 1 || fact.State != ports.OperationReviewAuthorizationActiveV4 {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "new Review Authorization must be active revision one")
	}
	now := s.clock()
	if now.IsZero() || fact.CreatedUnixNano > now.UnixNano() || fact.UpdatedUnixNano != fact.CreatedUnixNano || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "new Review Authorization time is future, inconsistent or expired")
	}
	s.facts[fact.ID] = cloneOperationReviewAuthorizationFactV4(fact)
	s.createCommits++
	if s.loseCreate {
		s.loseCreate = false
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected lost Review Authorization create reply")
	}
	return cloneOperationReviewAuthorizationFactV4(fact), nil
}

func (s *OperationReviewAuthorizationStoreV4) InspectOperationReviewAuthorizationV4(_ context.Context, id string) (ports.OperationReviewAuthorizationFactV4, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, exists := s.facts[id]
	if !exists {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "Review Authorization is absent")
	}
	return cloneOperationReviewAuthorizationFactV4(fact), nil
}

func (s *OperationReviewAuthorizationStoreV4) CompareAndSwapOperationReviewAuthorizationV4(_ context.Context, request ports.OperationReviewAuthorizationCASRequestV4) (ports.OperationReviewAuthorizationFactV4, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := request.Next.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	current, exists := s.facts[request.Next.ID]
	if !exists {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "Review Authorization is absent")
	}
	if current.Digest == request.Next.Digest {
		return cloneOperationReviewAuthorizationFactV4(current), nil
	}
	if current.Revision != request.ExpectedRevision {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Authorization revision changed")
	}
	if s.clock == nil {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review Authorization store clock is required")
	}
	if err := ports.ValidateOperationReviewAuthorizationTransitionV4(current, request.Next, s.clock()); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	s.facts[current.ID] = cloneOperationReviewAuthorizationFactV4(request.Next)
	if s.loseCAS {
		s.loseCAS = false
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected lost Review Authorization CAS reply")
	}
	return cloneOperationReviewAuthorizationFactV4(request.Next), nil
}

func cloneOperationReviewAuthorizationFactV4(fact ports.OperationReviewAuthorizationFactV4) ports.OperationReviewAuthorizationFactV4 {
	fact.Review.DecisionEvidence = append([]ports.EvidenceRecordRefV2{}, fact.Review.DecisionEvidence...)
	if fact.Review.Satisfaction != nil {
		satisfaction := *fact.Review.Satisfaction
		satisfaction.Evidence = append([]ports.EvidenceRecordRefV2{}, satisfaction.Evidence...)
		fact.Review.Satisfaction = &satisfaction
	}
	if fact.Intent.Operation.ExecutionScope.SandboxLease != nil {
		lease := *fact.Intent.Operation.ExecutionScope.SandboxLease
		fact.Intent.Operation.ExecutionScope.SandboxLease = &lease
	}
	if fact.Review.Operation.ExecutionScope.SandboxLease != nil {
		lease := *fact.Review.Operation.ExecutionScope.SandboxLease
		fact.Review.Operation.ExecutionScope.SandboxLease = &lease
	}
	if fact.Fence.Scope.SandboxLease != nil {
		lease := *fact.Fence.Scope.SandboxLease
		fact.Fence.Scope.SandboxLease = &lease
	}
	return fact
}

var _ ports.OperationReviewAuthorizationFactPortV4 = (*OperationReviewAuthorizationStoreV4)(nil)
