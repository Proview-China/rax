package fakes

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// AgentActivationCoordinationStoreV2 is deterministic and thread-safe. It is
// a conformance fixture, not a production durability claim.
type AgentActivationCoordinationStoreV2 struct {
	mu                        sync.Mutex
	facts                     map[string]contract.AgentActivationCoordinationFactV2
	loseCreate, loseCAS       bool
	failCAS                   core.ErrorCategory
	createCommits, casCommits uint64
}

func NewAgentActivationCoordinationStoreV2() *AgentActivationCoordinationStoreV2 {
	return &AgentActivationCoordinationStoreV2{facts: map[string]contract.AgentActivationCoordinationFactV2{}}
}

func (s *AgentActivationCoordinationStoreV2) CreateAgentActivationCoordinationV2(ctx context.Context, fact contract.AgentActivationCoordinationFactV2) (applicationports.AgentActivationCoordinationCreateReceiptV2, error) {
	if s == nil {
		return applicationports.AgentActivationCoordinationCreateReceiptV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Agent activation V2 fake store is nil")
	}
	if err := activationContextV2(ctx); err != nil {
		return applicationports.AgentActivationCoordinationCreateReceiptV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return applicationports.AgentActivationCoordinationCreateReceiptV2{}, err
	}
	if fact.Revision != 1 {
		return applicationports.AgentActivationCoordinationCreateReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Agent activation V2 create requires revision one")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.facts[fact.ActivationID]; ok {
		if current.Digest != fact.Digest {
			return applicationports.AgentActivationCoordinationCreateReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Agent activation V2 ID already binds another initial Fact")
		}
		return applicationports.AgentActivationCoordinationCreateReceiptV2{Fact: cloneActivationFactV2(current), Created: false}, nil
	}
	s.facts[fact.ActivationID] = cloneActivationFactV2(fact)
	s.createCommits++
	if s.loseCreate {
		s.loseCreate = false
		return applicationports.AgentActivationCoordinationCreateReceiptV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Injected Agent activation V2 create reply loss")
	}
	return applicationports.AgentActivationCoordinationCreateReceiptV2{Fact: cloneActivationFactV2(fact), Created: true}, nil
}

func (s *AgentActivationCoordinationStoreV2) InspectAgentActivationCoordinationV2(ctx context.Context, id string) (contract.AgentActivationCoordinationFactV2, error) {
	if s == nil {
		return contract.AgentActivationCoordinationFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Agent activation V2 fake store is nil")
	}
	if err := activationContextV2(ctx); err != nil {
		return contract.AgentActivationCoordinationFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.facts[id]
	if !ok {
		return contract.AgentActivationCoordinationFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Agent activation V2 Fact is absent")
	}
	return cloneActivationFactV2(fact), nil
}

func (s *AgentActivationCoordinationStoreV2) CompareAndSwapAgentActivationCoordinationV2(ctx context.Context, request applicationports.AgentActivationCoordinationCASRequestV2) (applicationports.AgentActivationCoordinationCASReceiptV2, error) {
	if s == nil {
		return applicationports.AgentActivationCoordinationCASReceiptV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Agent activation V2 fake store is nil")
	}
	if err := activationContextV2(ctx); err != nil {
		return applicationports.AgentActivationCoordinationCASReceiptV2{}, err
	}
	if err := request.Validate(); err != nil {
		return applicationports.AgentActivationCoordinationCASReceiptV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failCAS != "" {
		category := s.failCAS
		s.failCAS = ""
		return applicationports.AgentActivationCoordinationCASReceiptV2{}, activationFakeFaultV2(category, "Injected Agent activation V2 CAS pre-commit failure")
	}
	current, ok := s.facts[request.ActivationID]
	if !ok {
		return applicationports.AgentActivationCoordinationCASReceiptV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Agent activation V2 Fact is absent")
	}
	// Expected mismatch always conflicts, even when Next equals current. This is
	// what prevents two coordinators from both owning a start token.
	if current.Revision != request.ExpectedRevision || current.Digest != request.ExpectedDigest {
		return applicationports.AgentActivationCoordinationCASReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Agent activation V2 CAS predecessor changed")
	}
	if err := contract.ValidateAgentActivationCoordinationTransitionV2(current, request.Next); err != nil {
		return applicationports.AgentActivationCoordinationCASReceiptV2{}, err
	}
	s.facts[request.ActivationID] = cloneActivationFactV2(request.Next)
	s.casCommits++
	if s.loseCAS {
		s.loseCAS = false
		return applicationports.AgentActivationCoordinationCASReceiptV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Injected Agent activation V2 CAS reply loss")
	}
	return applicationports.AgentActivationCoordinationCASReceiptV2{Fact: cloneActivationFactV2(request.Next), Applied: true}, nil
}

func (s *AgentActivationCoordinationStoreV2) LoseNextCreateReplyV2() {
	s.mu.Lock()
	s.loseCreate = true
	s.mu.Unlock()
}
func (s *AgentActivationCoordinationStoreV2) LoseNextCASReplyV2() {
	s.mu.Lock()
	s.loseCAS = true
	s.mu.Unlock()
}
func (s *AgentActivationCoordinationStoreV2) FailNextCASBeforeCommitV2(category core.ErrorCategory) {
	s.mu.Lock()
	s.failCAS = category
	s.mu.Unlock()
}
func (s *AgentActivationCoordinationStoreV2) CountsV2() (uint64, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createCommits, s.casCommits
}

type AgentActivationStepRequestFactoryV2 func(applicationports.AgentActivationStepPreparationV2) (contract.AgentActivationStepRequestV2, error)
type AgentActivationStepResultFactoryV2 func(contract.AgentActivationStepRequestV2) (contract.AgentActivationStepResultV2, error)

type activationStepRecordV2 struct {
	request contract.AgentActivationStepRequestV2
	result  contract.AgentActivationStepResultV2
}

type AgentActivationStepV2 struct {
	mu                                              sync.Mutex
	step                                            contract.AgentActivationStepV2
	requestFactory                                  AgentActivationStepRequestFactoryV2
	resultFactory                                   AgentActivationStepResultFactoryV2
	records                                         map[string]activationStepRecordV2
	loseReply                                       bool
	prepareCalls, startCalls, commits, inspectCalls uint64
}

func NewAgentActivationStepV2(step contract.AgentActivationStepV2, requestFactory AgentActivationStepRequestFactoryV2, resultFactory AgentActivationStepResultFactoryV2) (*AgentActivationStepV2, error) {
	if step.Validate() != nil || activationNilFunctionV2(requestFactory) || activationNilFunctionV2(resultFactory) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Agent activation V2 fake step dependencies are required")
	}
	return &AgentActivationStepV2{step: step, requestFactory: requestFactory, resultFactory: resultFactory, records: map[string]activationStepRecordV2{}}, nil
}

func (s *AgentActivationStepV2) PrepareAgentActivationStepV2(ctx context.Context, p applicationports.AgentActivationStepPreparationV2) (contract.AgentActivationStepRequestV2, error) {
	if s == nil {
		return contract.AgentActivationStepRequestV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Agent activation V2 fake step is nil")
	}
	if err := activationContextV2(ctx); err != nil {
		return contract.AgentActivationStepRequestV2{}, err
	}
	if err := p.Validate(); err != nil {
		return contract.AgentActivationStepRequestV2{}, err
	}
	if p.Step != s.step {
		return contract.AgentActivationStepRequestV2{}, core.NewError(core.ErrorForbidden, core.ReasonComponentMismatch, "Agent activation preparation reached another step")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prepareCalls++
	request, err := s.requestFactory(p)
	if err != nil {
		return contract.AgentActivationStepRequestV2{}, err
	}
	if err := request.Validate(); err != nil {
		return contract.AgentActivationStepRequestV2{}, err
	}
	return cloneActivationStepRequestV2(request), nil
}

func (s *AgentActivationStepV2) StartOrInspectAgentActivationStepV2(ctx context.Context, request contract.AgentActivationStepRequestV2) (contract.AgentActivationStepResultV2, error) {
	if s == nil {
		return contract.AgentActivationStepResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Agent activation V2 fake step is nil")
	}
	if err := activationContextV2(ctx); err != nil {
		return contract.AgentActivationStepResultV2{}, err
	}
	if err := request.Validate(); err != nil {
		return contract.AgentActivationStepResultV2{}, err
	}
	if request.Step != s.step {
		return contract.AgentActivationStepResultV2{}, core.NewError(core.ErrorForbidden, core.ReasonComponentMismatch, "Agent activation request reached another V2 step")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startCalls++
	if record, ok := s.records[request.AttemptID]; ok {
		if record.request.RequestDigest != request.RequestDigest {
			return contract.AgentActivationStepResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Agent activation V2 attempt already binds another request")
		}
		return cloneActivationStepResultV2(record.result), nil
	}
	result, err := s.resultFactory(request)
	if err != nil {
		return contract.AgentActivationStepResultV2{}, err
	}
	if err := result.ValidateFor(request, time.Unix(0, result.Proof.CheckedUnixNano)); err != nil {
		return contract.AgentActivationStepResultV2{}, err
	}
	s.records[request.AttemptID] = activationStepRecordV2{cloneActivationStepRequestV2(request), cloneActivationStepResultV2(result)}
	s.commits++
	if s.loseReply {
		s.loseReply = false
		return contract.AgentActivationStepResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Injected Agent activation V2 Owner reply loss")
	}
	return cloneActivationStepResultV2(result), nil
}

func (s *AgentActivationStepV2) InspectAgentActivationStepV2(ctx context.Context, request contract.AgentActivationStepRequestV2) (contract.AgentActivationStepResultV2, error) {
	if s == nil {
		return contract.AgentActivationStepResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Agent activation V2 fake step is nil")
	}
	if err := activationContextV2(ctx); err != nil {
		return contract.AgentActivationStepResultV2{}, err
	}
	if err := request.Validate(); err != nil {
		return contract.AgentActivationStepResultV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inspectCalls++
	record, ok := s.records[request.AttemptID]
	if !ok {
		return contract.AgentActivationStepResultV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "Agent activation V2 Owner result is absent")
	}
	if record.request.RequestDigest != request.RequestDigest {
		return contract.AgentActivationStepResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Agent activation V2 Inspect request drifted")
	}
	return cloneActivationStepResultV2(record.result), nil
}

func (s *AgentActivationStepV2) LoseNextStartReplyV2() {
	s.mu.Lock()
	s.loseReply = true
	s.mu.Unlock()
}
func (s *AgentActivationStepV2) CountsV2() (uint64, uint64, uint64, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prepareCalls, s.startCalls, s.commits, s.inspectCalls
}

func cloneActivationFactV2(v contract.AgentActivationCoordinationFactV2) contract.AgentActivationCoordinationFactV2 {
	out := v
	out.Events = make([]contract.AgentActivationStepEventV2, len(v.Events))
	for index, event := range v.Events {
		out.Events[index] = event
		if event.Request != nil {
			request := cloneActivationStepRequestV2(*event.Request)
			out.Events[index].Request = &request
		}
		if event.Result != nil {
			result := cloneActivationStepResultV2(*event.Result)
			out.Events[index].Result = &result
		}
	}
	if v.Result != nil {
		result := *v.Result
		if v.Result.ExecutionScope.SandboxLease != nil {
			lease := *v.Result.ExecutionScope.SandboxLease
			result.ExecutionScope.SandboxLease = &lease
		}
		out.Result = &result
	}
	return out
}
func cloneActivationStepRequestV2(v contract.AgentActivationStepRequestV2) contract.AgentActivationStepRequestV2 {
	out := v
	if v.Inputs.Predecessor != nil {
		value := *v.Inputs.Predecessor
		out.Inputs.Predecessor = &value
	}
	if v.Inputs.Authority != nil {
		value := *v.Inputs.Authority
		out.Inputs.Authority = &value
	}
	if v.Inputs.Policy != nil {
		value := *v.Inputs.Policy
		out.Inputs.Policy = &value
	}
	if v.Inputs.Dispatch != nil {
		value := *v.Inputs.Dispatch
		out.Inputs.Dispatch = &value
	}
	return out
}
func cloneActivationStepResultV2(v contract.AgentActivationStepResultV2) contract.AgentActivationStepResultV2 {
	out := v
	if v.Proof.SecondaryCurrent != nil {
		value := *v.Proof.SecondaryCurrent
		out.Proof.SecondaryCurrent = &value
	}
	if v.Proof.Lease != nil {
		value := *v.Proof.Lease
		out.Proof.Lease = &value
	}
	if v.Proof.CommittedScope != nil {
		value := *v.Proof.CommittedScope
		if value.SandboxLease != nil {
			lease := *value.SandboxLease
			value.SandboxLease = &lease
		}
		out.Proof.CommittedScope = &value
	}
	if v.Proof.EndpointCurrent != nil {
		value := *v.Proof.EndpointCurrent
		out.Proof.EndpointCurrent = &value
	}
	if v.Proof.Budget != nil {
		value := *v.Proof.Budget
		if v.Proof.Budget.BudgetCurrent != nil {
			current := *v.Proof.Budget.BudgetCurrent
			value.BudgetCurrent = &current
		}
		if v.Proof.Budget.NotRequiredPolicy != nil {
			current := *v.Proof.Budget.NotRequiredPolicy
			value.NotRequiredPolicy = &current
		}
		out.Proof.Budget = &value
	}
	return out
}

func activationContextV2(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "Agent activation V2 context is unavailable")
	}
	return nil
}
func activationNilFunctionV2(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Func && reflected.IsNil()
}
func activationFakeFaultV2(category core.ErrorCategory, message string) error {
	switch category {
	case core.ErrorConflict:
		return core.NewError(category, core.ReasonRevisionConflict, message)
	case core.ErrorUnavailable:
		return core.NewError(category, core.ReasonEvidenceUnavailable, message)
	default:
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, message)
	}
}

var _ applicationports.AgentActivationCoordinationFactPortV2 = (*AgentActivationCoordinationStoreV2)(nil)
var _ applicationports.AgentActivationStepPortV2 = (*AgentActivationStepV2)(nil)
