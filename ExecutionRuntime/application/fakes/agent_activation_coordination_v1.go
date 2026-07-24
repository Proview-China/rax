package fakes

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// AgentActivationCoordinationStoreV1 is a thread-safe reference Fact store.
// It is intentionally in-memory and grants no production durability claim.
type AgentActivationCoordinationStoreV1 struct {
	mu            sync.Mutex
	facts         map[string]contract.AgentActivationCoordinationFactV1
	loseEnsure    bool
	loseCAS       bool
	ensureCommits uint64
	casCommits    uint64
}

func NewAgentActivationCoordinationStoreV1() *AgentActivationCoordinationStoreV1 {
	return &AgentActivationCoordinationStoreV1{facts: make(map[string]contract.AgentActivationCoordinationFactV1)}
}

func (s *AgentActivationCoordinationStoreV1) EnsureAgentActivationCoordinationV1(ctx context.Context, fact contract.AgentActivationCoordinationFactV1) (contract.AgentActivationCoordinationFactV1, error) {
	if err := lifecycleContextErrorV1(ctx); err != nil {
		return contract.AgentActivationCoordinationFactV1{}, err
	}
	if err := fact.Validate(); err != nil {
		return contract.AgentActivationCoordinationFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.facts[fact.ActivationID]; ok {
		if existing.Digest != fact.Digest {
			return contract.AgentActivationCoordinationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Agent activation coordination identity already binds different content")
		}
		return cloneActivationFactV1(existing), nil
	}
	s.facts[fact.ActivationID] = cloneActivationFactV1(fact)
	s.ensureCommits++
	if s.loseEnsure {
		s.loseEnsure = false
		return contract.AgentActivationCoordinationFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Agent activation coordination Ensure reply loss")
	}
	return cloneActivationFactV1(fact), nil
}

func (s *AgentActivationCoordinationStoreV1) InspectAgentActivationCoordinationV1(ctx context.Context, activationID string) (contract.AgentActivationCoordinationFactV1, error) {
	if err := lifecycleContextErrorV1(ctx); err != nil {
		return contract.AgentActivationCoordinationFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.facts[activationID]
	if !ok {
		return contract.AgentActivationCoordinationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Agent activation coordination Fact is absent")
	}
	return cloneActivationFactV1(fact), nil
}

func (s *AgentActivationCoordinationStoreV1) CompareAndSwapAgentActivationCoordinationV1(ctx context.Context, request applicationports.AgentActivationCoordinationCASRequestV1) (contract.AgentActivationCoordinationFactV1, error) {
	if err := lifecycleContextErrorV1(ctx); err != nil {
		return contract.AgentActivationCoordinationFactV1{}, err
	}
	if request.ActivationID == "" || request.ExpectedRevision == 0 || request.ExpectedDigest.Validate() != nil || request.Next.ActivationID != request.ActivationID {
		return contract.AgentActivationCoordinationFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "Agent activation coordination CAS coordinates are incomplete")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.facts[request.ActivationID]
	if !ok {
		return contract.AgentActivationCoordinationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Agent activation coordination Fact is absent")
	}
	if current.Revision != request.ExpectedRevision || current.Digest != request.ExpectedDigest {
		return contract.AgentActivationCoordinationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Agent activation coordination CAS predecessor changed")
	}
	if err := contract.ValidateAgentActivationCoordinationTransitionV1(current, request.Next); err != nil {
		return contract.AgentActivationCoordinationFactV1{}, err
	}
	s.facts[request.ActivationID] = cloneActivationFactV1(request.Next)
	s.casCommits++
	if s.loseCAS {
		s.loseCAS = false
		return contract.AgentActivationCoordinationFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Agent activation coordination CAS reply loss")
	}
	return cloneActivationFactV1(request.Next), nil
}

func (s *AgentActivationCoordinationStoreV1) LoseNextEnsureReplyV1() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseEnsure = true
}

func (s *AgentActivationCoordinationStoreV1) LoseNextCASReplyV1() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseCAS = true
}

func (s *AgentActivationCoordinationStoreV1) CountsV1() (uint64, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureCommits, s.casCommits
}

type AgentActivationStepResultFactoryV1 func(contract.AgentActivationStepRequestV1) (contract.AgentActivationStepResultV1, error)

type activationStepRecordV1 struct {
	request contract.AgentActivationStepRequestV1
	result  contract.AgentActivationStepResultV1
}

// AgentActivationStepV1 is a deterministic Owner-step reference port. It
// models stable-attempt start/inspect only; it performs no external Effect.
type AgentActivationStepV1 struct {
	mu         sync.Mutex
	step       contract.AgentActivationStepV1
	factory    AgentActivationStepResultFactoryV1
	records    map[string]activationStepRecordV1
	loseReply  bool
	startCalls uint64
	commits    uint64
	inspects   uint64
}

func NewAgentActivationStepV1(step contract.AgentActivationStepV1, factory AgentActivationStepResultFactoryV1) (*AgentActivationStepV1, error) {
	if step.Validate() != nil || nilFunctionV1(factory) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Agent activation reference step and result factory are required")
	}
	return &AgentActivationStepV1{step: step, factory: factory, records: make(map[string]activationStepRecordV1)}, nil
}

func (s *AgentActivationStepV1) StartOrInspectAgentActivationStepV1(ctx context.Context, request contract.AgentActivationStepRequestV1) (contract.AgentActivationStepResultV1, error) {
	if err := lifecycleContextErrorV1(ctx); err != nil {
		return contract.AgentActivationStepResultV1{}, err
	}
	if err := request.Validate(); err != nil {
		return contract.AgentActivationStepResultV1{}, err
	}
	if request.Step != s.step {
		return contract.AgentActivationStepResultV1{}, core.NewError(core.ErrorForbidden, core.ReasonComponentMismatch, "Agent activation request reached another Owner step")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startCalls++
	if existing, ok := s.records[request.AttemptID]; ok {
		if existing.request.RequestDigest != request.RequestDigest {
			return contract.AgentActivationStepResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Agent activation step attempt already binds another request")
		}
		return cloneActivationStepResultV1(existing.result), nil
	}
	result, err := s.factory(request)
	if err != nil {
		return contract.AgentActivationStepResultV1{}, err
	}
	if err := result.ValidateFor(request, time.Unix(0, result.CheckedUnixNano)); err != nil {
		return contract.AgentActivationStepResultV1{}, err
	}
	s.records[request.AttemptID] = activationStepRecordV1{request: request, result: cloneActivationStepResultV1(result)}
	s.commits++
	if s.loseReply {
		s.loseReply = false
		return contract.AgentActivationStepResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Agent activation Owner step reply loss")
	}
	return cloneActivationStepResultV1(result), nil
}

func (s *AgentActivationStepV1) InspectAgentActivationStepV1(ctx context.Context, request contract.AgentActivationStepRequestV1) (contract.AgentActivationStepResultV1, error) {
	if err := lifecycleContextErrorV1(ctx); err != nil {
		return contract.AgentActivationStepResultV1{}, err
	}
	if err := request.Validate(); err != nil {
		return contract.AgentActivationStepResultV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inspects++
	existing, ok := s.records[request.AttemptID]
	if !ok {
		return contract.AgentActivationStepResultV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "Agent activation Owner step result is absent")
	}
	if existing.request.RequestDigest != request.RequestDigest {
		return contract.AgentActivationStepResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Agent activation Owner step Inspect request drifted")
	}
	return cloneActivationStepResultV1(existing.result), nil
}

func (s *AgentActivationStepV1) LoseNextStartReplyV1() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseReply = true
}

func (s *AgentActivationStepV1) CountsV1() (uint64, uint64, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startCalls, s.commits, s.inspects
}

func cloneActivationFactV1(value contract.AgentActivationCoordinationFactV1) contract.AgentActivationCoordinationFactV1 {
	payload, _ := json.Marshal(value)
	var clone contract.AgentActivationCoordinationFactV1
	_ = json.Unmarshal(payload, &clone)
	return clone
}

func cloneActivationStepResultV1(value contract.AgentActivationStepResultV1) contract.AgentActivationStepResultV1 {
	payload, _ := json.Marshal(value)
	var clone contract.AgentActivationStepResultV1
	_ = json.Unmarshal(payload, &clone)
	return clone
}

var _ applicationports.AgentActivationCoordinationFactPortV1 = (*AgentActivationCoordinationStoreV1)(nil)
var _ applicationports.AgentActivationStepPortV1 = (*AgentActivationStepV1)(nil)
