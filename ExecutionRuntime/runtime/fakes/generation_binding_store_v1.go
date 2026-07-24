package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// GenerationBindingAssociationStoreV1 is an in-memory reference Fact Owner.
// It is deterministic test infrastructure and makes no production durability,
// transaction, backend, process-topology or SLA claim.
type GenerationBindingAssociationStoreV1 struct {
	mu            sync.Mutex
	clock         func() time.Time
	facts         map[string]ports.GenerationBindingAssociationFactV1
	loseCreate    bool
	loseCAS       bool
	createCommits uint64
}

func NewGenerationBindingAssociationStoreV1(clock func() time.Time) *GenerationBindingAssociationStoreV1 {
	return &GenerationBindingAssociationStoreV1{clock: clock, facts: make(map[string]ports.GenerationBindingAssociationFactV1)}
}

func (s *GenerationBindingAssociationStoreV1) LoseNextCreateReplyV1() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseCreate = true
}

func (s *GenerationBindingAssociationStoreV1) LoseNextCASReplyV1() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseCAS = true
}

func (s *GenerationBindingAssociationStoreV1) CreateCommitCountV1() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createCommits
}

func (s *GenerationBindingAssociationStoreV1) CreateGenerationBindingAssociationV1(_ context.Context, fact ports.GenerationBindingAssociationFactV1) (ports.GenerationBindingAssociationFactV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fact.Validate(); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if current, exists := s.facts[fact.ID]; exists {
		if current.Digest == fact.Digest {
			return cloneGenerationBindingAssociationFactV1(current), nil
		}
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "association ID already contains different content")
	}
	if fact.Revision != 1 || fact.State != ports.GenerationBindingAssociationActiveV1 || s.clock == nil {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "new association must be active revision one")
	}
	now := s.clock()
	if now.IsZero() || fact.CreatedUnixNano > now.UnixNano() || fact.UpdatedUnixNano != fact.CreatedUnixNano || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "new association time is future, inconsistent or expired")
	}
	s.facts[fact.ID] = cloneGenerationBindingAssociationFactV1(fact)
	s.createCommits++
	if s.loseCreate {
		s.loseCreate = false
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected lost association create reply")
	}
	return cloneGenerationBindingAssociationFactV1(fact), nil
}

func (s *GenerationBindingAssociationStoreV1) InspectGenerationBindingAssociationV1(_ context.Context, id string) (ports.GenerationBindingAssociationFactV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, exists := s.facts[id]
	if !exists {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "generation binding association is absent")
	}
	return cloneGenerationBindingAssociationFactV1(fact), nil
}

func (s *GenerationBindingAssociationStoreV1) CompareAndSwapGenerationBindingAssociationV1(_ context.Context, request ports.GenerationBindingAssociationCASRequestV1) (ports.GenerationBindingAssociationFactV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := request.Next.Validate(); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	current, exists := s.facts[request.Next.ID]
	if !exists {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "generation binding association is absent")
	}
	if current.Digest == request.Next.Digest {
		return cloneGenerationBindingAssociationFactV1(current), nil
	}
	if request.ExpectedRevision != current.Revision {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "association revision changed")
	}
	if s.clock == nil {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "association store clock is required")
	}
	if err := ports.ValidateGenerationBindingAssociationTransitionV1(current, request.Next, s.clock()); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	s.facts[current.ID] = cloneGenerationBindingAssociationFactV1(request.Next)
	if s.loseCAS {
		s.loseCAS = false
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected lost association CAS reply")
	}
	return cloneGenerationBindingAssociationFactV1(request.Next), nil
}

func cloneGenerationBindingAssociationFactV1(fact ports.GenerationBindingAssociationFactV1) ports.GenerationBindingAssociationFactV1 {
	fact.Candidate.Generation.ComponentManifests = append([]ports.GenerationComponentManifestRefV1{}, fact.Candidate.Generation.ComponentManifests...)
	if fact.Candidate.Activation.Operation.ExecutionScope.SandboxLease != nil {
		lease := *fact.Candidate.Activation.Operation.ExecutionScope.SandboxLease
		fact.Candidate.Activation.Operation.ExecutionScope.SandboxLease = &lease
	}
	return fact
}

var _ ports.GenerationBindingAssociationFactPortV1 = (*GenerationBindingAssociationStoreV1)(nil)
