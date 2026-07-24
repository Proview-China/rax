package fakes

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// SingleCallToolActionCoordinationStoreV1 is a deterministic test Fact store.
// It is not a production backend, durability claim or service-level promise.
type SingleCallToolActionCoordinationStoreV1 struct {
	mu                   sync.Mutex
	facts                map[string]contract.SingleCallToolActionCoordinationFactV1
	LoseNextCreateReply  bool
	LoseNextCASReply     bool
	LoseNextInspectReply bool
	CreateCommits        uint64
	CASCommits           uint64
	AfterCASCommit       func(contract.SingleCallToolActionCoordinationStateV1)
}

func NewSingleCallToolActionCoordinationStoreV1() *SingleCallToolActionCoordinationStoreV1 {
	return &SingleCallToolActionCoordinationStoreV1{facts: make(map[string]contract.SingleCallToolActionCoordinationFactV1)}
}

func (s *SingleCallToolActionCoordinationStoreV1) CreateSingleCallToolActionCoordinationV1(_ context.Context, fact contract.SingleCallToolActionCoordinationFactV1) (contract.SingleCallToolActionCoordinationFactV1, error) {
	if err := fact.Validate(); err != nil {
		return contract.SingleCallToolActionCoordinationFactV1{}, err
	}
	key := singleCallToolActionKeyV1(fact.Request.ExecutionScope, fact.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.facts[key]; ok {
		if current.Request.Digest != fact.Request.Digest {
			return contract.SingleCallToolActionCoordinationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "single-call coordination ID already binds different content")
		}
		return cloneSingleCallCoordinationV1(current), nil
	}
	s.facts[key] = cloneSingleCallCoordinationV1(fact)
	s.CreateCommits++
	if s.LoseNextCreateReply {
		s.LoseNextCreateReply = false
		return contract.SingleCallToolActionCoordinationFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected single-call create reply loss")
	}
	return cloneSingleCallCoordinationV1(fact), nil
}

func (s *SingleCallToolActionCoordinationStoreV1) InspectSingleCallToolActionCoordinationV1(_ context.Context, scope core.ExecutionScope, id string) (contract.SingleCallToolActionCoordinationFactV1, error) {
	if err := scope.Validate(); err != nil {
		return contract.SingleCallToolActionCoordinationFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.LoseNextInspectReply {
		s.LoseNextInspectReply = false
		return contract.SingleCallToolActionCoordinationFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected single-call Inspect reply loss")
	}
	fact, ok := s.facts[singleCallToolActionKeyV1(scope, id)]
	if !ok {
		return contract.SingleCallToolActionCoordinationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "single-call coordination fact not found")
	}
	return cloneSingleCallCoordinationV1(fact), nil
}

func (s *SingleCallToolActionCoordinationStoreV1) CompareAndSwapSingleCallToolActionCoordinationV1(_ context.Context, request applicationports.SingleCallToolActionCoordinationCASRequestV1) (contract.SingleCallToolActionCoordinationFactV1, error) {
	if err := request.Scope.Validate(); err != nil {
		return contract.SingleCallToolActionCoordinationFactV1{}, err
	}
	if request.ExpectedRevision == 0 || request.Next.ID != request.ID || request.Next.Revision != request.ExpectedRevision+1 || !runtimeports.SameExecutionScopeV2(request.Scope, request.Next.Request.ExecutionScope) {
		return contract.SingleCallToolActionCoordinationFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "single-call coordination CAS key or revision is invalid")
	}
	key := singleCallToolActionKeyV1(request.Scope, request.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.facts[key]
	if !ok {
		return contract.SingleCallToolActionCoordinationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "single-call coordination fact not found")
	}
	if current.Revision != request.ExpectedRevision {
		if current.Revision == request.Next.Revision && current.Digest == request.Next.Digest {
			return cloneSingleCallCoordinationV1(current), nil
		}
		return contract.SingleCallToolActionCoordinationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "single-call coordination revision changed")
	}
	if err := contract.ValidateSingleCallToolActionCoordinationTransitionV1(current, request.Next); err != nil {
		return contract.SingleCallToolActionCoordinationFactV1{}, err
	}
	s.facts[key] = cloneSingleCallCoordinationV1(request.Next)
	s.CASCommits++
	if s.AfterCASCommit != nil {
		s.AfterCASCommit(request.Next.State)
	}
	if s.LoseNextCASReply {
		s.LoseNextCASReply = false
		return contract.SingleCallToolActionCoordinationFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected single-call CAS reply loss")
	}
	return cloneSingleCallCoordinationV1(request.Next), nil
}

func (s *SingleCallToolActionCoordinationStoreV1) Counts() (uint64, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.CreateCommits, s.CASCommits
}

func singleCallToolActionKeyV1(scope core.ExecutionScope, id string) string {
	digest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	return string(digest) + "\x00" + id
}

func cloneSingleCallCoordinationV1(value contract.SingleCallToolActionCoordinationFactV1) contract.SingleCallToolActionCoordinationFactV1 {
	payload, _ := json.Marshal(value)
	var clone contract.SingleCallToolActionCoordinationFactV1
	_ = json.Unmarshal(payload, &clone)
	return clone
}
