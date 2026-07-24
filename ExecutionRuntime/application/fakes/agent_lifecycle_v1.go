package fakes

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type AgentActivationResultFactoryV1 func(contract.AgentActivationStartRequestV1) (contract.AgentActivationResultV1, error)
type AgentTerminationResultFactoryV1 func(contract.AgentTerminationRequestV1) (contract.AgentTerminationResultV1, error)

type agentActivationRecordV1 struct {
	request contract.AgentActivationStartRequestV1
	result  contract.AgentActivationResultV1
}

type agentTerminationRecordV1 struct {
	request contract.AgentTerminationRequestV1
	result  contract.AgentTerminationResultV1
}

// AgentLifecycleV1 is an in-memory reference port for contract/conformance
// tests. It is not a production coordinator, Runtime owner, or durable backend.
type AgentLifecycleV1 struct {
	mu             sync.Mutex
	starts         map[string]agentActivationRecordV1
	stops          map[string]agentTerminationRecordV1
	startFactory   AgentActivationResultFactoryV1
	stopFactory    AgentTerminationResultFactoryV1
	loseStartReply bool
	loseStopReply  bool
	startCalls     uint64
	startCommits   uint64
	inspectCalls   uint64
	stopCalls      uint64
	stopCommits    uint64
}

func NewAgentLifecycleV1(start AgentActivationResultFactoryV1, stop AgentTerminationResultFactoryV1) (*AgentLifecycleV1, error) {
	if nilFunctionV1(start) || nilFunctionV1(stop) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Agent lifecycle reference result factories are required")
	}
	return &AgentLifecycleV1{
		starts: make(map[string]agentActivationRecordV1), stops: make(map[string]agentTerminationRecordV1),
		startFactory: start, stopFactory: stop,
	}, nil
}

func (f *AgentLifecycleV1) StartOrInspectAgentActivationV1(ctx context.Context, request contract.AgentActivationStartRequestV1) (contract.AgentActivationResultV1, error) {
	if err := lifecycleContextErrorV1(ctx); err != nil {
		return contract.AgentActivationResultV1{}, err
	}
	if err := request.Validate(); err != nil {
		return contract.AgentActivationResultV1{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls++
	if existing, ok := f.starts[request.ActivationID]; ok {
		if existing.request.RequestDigest != request.RequestDigest || existing.request.AttemptID != request.AttemptID || existing.request.IdempotencyKey != request.IdempotencyKey {
			return contract.AgentActivationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Agent activation identity already binds another exact request")
		}
		return cloneActivationResultV1(existing.result), nil
	}
	result, err := f.startFactory(request)
	if err != nil {
		return contract.AgentActivationResultV1{}, err
	}
	if err := result.ValidateFor(request, time.Unix(0, result.CheckedUnixNano)); err != nil {
		return contract.AgentActivationResultV1{}, err
	}
	f.starts[request.ActivationID] = agentActivationRecordV1{request: request, result: cloneActivationResultV1(result)}
	f.startCommits++
	if f.loseStartReply {
		f.loseStartReply = false
		return contract.AgentActivationResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Agent activation reply loss after commit")
	}
	return cloneActivationResultV1(result), nil
}

func (f *AgentLifecycleV1) InspectAgentActivationV1(ctx context.Context, request contract.AgentActivationStartRequestV1) (contract.AgentActivationResultV1, error) {
	if err := lifecycleContextErrorV1(ctx); err != nil {
		return contract.AgentActivationResultV1{}, err
	}
	if err := request.Validate(); err != nil {
		return contract.AgentActivationResultV1{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inspectCalls++
	existing, ok := f.starts[request.ActivationID]
	if !ok {
		return contract.AgentActivationResultV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Agent activation result is absent")
	}
	if existing.request.RequestDigest != request.RequestDigest || existing.request.AttemptID != request.AttemptID || existing.request.IdempotencyKey != request.IdempotencyKey {
		return contract.AgentActivationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Agent activation Inspect request drifted")
	}
	return cloneActivationResultV1(existing.result), nil
}

func (f *AgentLifecycleV1) StopOrInspectAgentV1(ctx context.Context, request contract.AgentTerminationRequestV1) (contract.AgentTerminationResultV1, error) {
	if err := lifecycleContextErrorV1(ctx); err != nil {
		return contract.AgentTerminationResultV1{}, err
	}
	if err := request.Validate(); err != nil {
		return contract.AgentTerminationResultV1{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCalls++
	if existing, ok := f.stops[request.StopID]; ok {
		if existing.request.RequestDigest != request.RequestDigest || existing.request.AttemptID != request.AttemptID || existing.request.IdempotencyKey != request.IdempotencyKey {
			return contract.AgentTerminationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Agent termination identity already binds another exact request")
		}
		return cloneTerminationResultV1(existing.result), nil
	}
	result, err := f.stopFactory(request)
	if err != nil {
		return contract.AgentTerminationResultV1{}, err
	}
	if err := result.ValidateFor(request, time.Unix(0, result.CheckedUnixNano)); err != nil {
		return contract.AgentTerminationResultV1{}, err
	}
	f.stops[request.StopID] = agentTerminationRecordV1{request: request, result: cloneTerminationResultV1(result)}
	f.stopCommits++
	if f.loseStopReply {
		f.loseStopReply = false
		return contract.AgentTerminationResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Agent termination reply loss after commit")
	}
	return cloneTerminationResultV1(result), nil
}

func (f *AgentLifecycleV1) LoseNextStartReplyV1() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loseStartReply = true
}

func (f *AgentLifecycleV1) LoseNextStopReplyV1() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loseStopReply = true
}

type AgentLifecycleCountsV1 struct {
	StartCalls   uint64
	StartCommits uint64
	InspectCalls uint64
	StopCalls    uint64
	StopCommits  uint64
}

func (f *AgentLifecycleV1) CountsV1() AgentLifecycleCountsV1 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return AgentLifecycleCountsV1{StartCalls: f.startCalls, StartCommits: f.startCommits, InspectCalls: f.inspectCalls, StopCalls: f.stopCalls, StopCommits: f.stopCommits}
}

func cloneActivationResultV1(value contract.AgentActivationResultV1) contract.AgentActivationResultV1 {
	payload, _ := json.Marshal(value)
	var clone contract.AgentActivationResultV1
	_ = json.Unmarshal(payload, &clone)
	return clone
}

func cloneTerminationResultV1(value contract.AgentTerminationResultV1) contract.AgentTerminationResultV1 {
	payload, _ := json.Marshal(value)
	var clone contract.AgentTerminationResultV1
	_ = json.Unmarshal(payload, &clone)
	return clone
}

func lifecycleContextErrorV1(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "Agent lifecycle context is nil or canceled")
	}
	return nil
}

func nilFunctionV1(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	return ref.Kind() == reflect.Func && ref.IsNil()
}

var _ applicationports.AgentLifecyclePortV1 = (*AgentLifecycleV1)(nil)
