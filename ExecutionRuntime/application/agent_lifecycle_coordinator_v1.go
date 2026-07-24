package application

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// AgentActivationCoordinatorV1 is a reference write-ahead coordinator. It
// composes neutral Owner step ports but owns no Runtime, Sandbox or Harness
// facts and makes no production durability claim.
type AgentActivationCoordinatorV1 struct {
	facts applicationports.AgentActivationCoordinationFactPortV1
	steps applicationports.AgentActivationStepPortsV1
	clock func() time.Time
	mu    sync.Mutex
}

func NewAgentActivationCoordinatorV1(facts applicationports.AgentActivationCoordinationFactPortV1, steps applicationports.AgentActivationStepPortsV1, clock func() time.Time) (*AgentActivationCoordinatorV1, error) {
	if lifecycleNilV1(facts) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Agent activation coordination Fact port and clock are required")
	}
	for _, step := range steps.OrderedV1() {
		if lifecycleNilV1(step) {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "all Agent activation step ports are required")
		}
	}
	return &AgentActivationCoordinatorV1{facts: facts, steps: steps, clock: clock}, nil
}

func (c *AgentActivationCoordinatorV1) StartOrInspectAgentActivationV1(ctx context.Context, request contract.AgentActivationStartRequestV1) (contract.AgentActivationResultV1, error) {
	if c == nil || ctx == nil || ctx.Err() != nil {
		return contract.AgentActivationResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "Agent activation coordinator or context is unavailable")
	}
	if err := request.Validate(); err != nil {
		return contract.AgentActivationResultV1{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	fact, err := c.facts.InspectAgentActivationCoordinationV1(ctx, request.ActivationID)
	if err != nil {
		if !core.HasCategory(err, core.ErrorNotFound) {
			return contract.AgentActivationResultV1{}, err
		}
		cursor := activationClockCursorV1{read: c.clock}
		first, err := c.newIntentEventV1(request, 0, "", &cursor)
		if err != nil {
			return contract.AgentActivationResultV1{}, err
		}
		first.Sequence = 1
		first, err = contract.SealAgentActivationStepEventV1(first)
		if err != nil {
			return contract.AgentActivationResultV1{}, err
		}
		fact, err = contract.SealAgentActivationCoordinationFactV1(contract.AgentActivationCoordinationFactV1{
			ActivationID: request.ActivationID, Revision: 1, Request: request, Events: []contract.AgentActivationStepEventV1{first},
		})
		if err != nil {
			return contract.AgentActivationResultV1{}, err
		}
		fact, err = c.ensureFactV1(ctx, fact)
		if err != nil {
			return contract.AgentActivationResultV1{}, err
		}
	}
	if fact.Request.RequestDigest != request.RequestDigest {
		return contract.AgentActivationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Agent activation coordination identity binds another exact Start request")
	}
	last := time.Time{}
	if len(fact.Events) != 0 {
		last = time.Unix(0, fact.Events[len(fact.Events)-1].RecordedUnixNano)
	}
	cursor := activationClockCursorV1{read: c.clock, last: last}
	return c.resumeActivationV1(ctx, fact, &cursor)
}

func (c *AgentActivationCoordinatorV1) InspectAgentActivationV1(ctx context.Context, request contract.AgentActivationStartRequestV1) (contract.AgentActivationResultV1, error) {
	if c == nil || ctx == nil || ctx.Err() != nil {
		return contract.AgentActivationResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "Agent activation coordinator or context is unavailable")
	}
	if err := request.Validate(); err != nil {
		return contract.AgentActivationResultV1{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	fact, err := c.facts.InspectAgentActivationCoordinationV1(ctx, request.ActivationID)
	if err != nil {
		return contract.AgentActivationResultV1{}, err
	}
	if fact.Request.RequestDigest != request.RequestDigest {
		return contract.AgentActivationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Agent activation Inspect request drifted")
	}
	if fact.Result == nil {
		return contract.AgentActivationResultV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "Agent activation coordination is not complete")
	}
	now := c.clock()
	if err := fact.Result.ValidateFor(request, now); err != nil {
		return contract.AgentActivationResultV1{}, err
	}
	return cloneAgentActivationResultV1(*fact.Result), nil
}

func (c *AgentActivationCoordinatorV1) resumeActivationV1(ctx context.Context, fact contract.AgentActivationCoordinationFactV1, cursor *activationClockCursorV1) (contract.AgentActivationResultV1, error) {
	for {
		if fact.Result != nil {
			now, err := cursor.observe()
			if err != nil {
				return contract.AgentActivationResultV1{}, err
			}
			if err := fact.Result.ValidateFor(fact.Request, now); err != nil {
				return contract.AgentActivationResultV1{}, err
			}
			return cloneAgentActivationResultV1(*fact.Result), nil
		}
		stepIndex, lastState, predecessor, err := coordinationPositionV1(fact)
		if err != nil {
			return contract.AgentActivationResultV1{}, err
		}
		stepRequest, err := activationStepRequestV1(fact.Request, stepIndex, predecessor)
		if err != nil {
			return contract.AgentActivationResultV1{}, err
		}
		switch lastState {
		case contract.AgentActivationStepResultRecordedV1:
			event, eventErr := c.newIntentEventV1(fact.Request, stepIndex, predecessor, cursor)
			if eventErr != nil {
				return contract.AgentActivationResultV1{}, eventErr
			}
			fact, _, err = c.appendEventV1(ctx, fact, event, nil)
			if err != nil {
				return contract.AgentActivationResultV1{}, err
			}
		case contract.AgentActivationStepIntentRecordedV1:
			event, eventErr := c.newStateEventV1(stepRequest, contract.AgentActivationStepInvocationRecordedV1, nil, cursor)
			if eventErr != nil {
				return contract.AgentActivationResultV1{}, eventErr
			}
			var appliedThisCall bool
			fact, appliedThisCall, err = c.appendEventV1(ctx, fact, event, nil)
			if err != nil {
				return contract.AgentActivationResultV1{}, err
			}
			if !appliedThisCall {
				// Another caller, or an unknown CAS reply, owns any possible
				// dispatch. A recovered invocation/unknown is permanently
				// Inspect-only for this call.
				continue
			}
			result, startErr := c.steps.OrderedV1()[stepIndex].StartOrInspectAgentActivationStepV1(ctx, stepRequest)
			if startErr != nil {
				unknown, eventErr := c.newStateEventV1(stepRequest, contract.AgentActivationStepOutcomeUnknownV1, nil, cursor)
				if eventErr != nil {
					return contract.AgentActivationResultV1{}, eventErr
				}
				fact, _, err = c.appendEventV1(context.WithoutCancel(ctx), fact, unknown, nil)
				if err != nil {
					return contract.AgentActivationResultV1{}, err
				}
				result, err = c.steps.OrderedV1()[stepIndex].InspectAgentActivationStepV1(context.WithoutCancel(ctx), stepRequest)
				if err != nil {
					return contract.AgentActivationResultV1{}, startErr
				}
			}
			fact, err = c.recordStepResultV1(context.WithoutCancel(ctx), fact, stepRequest, result, cursor)
			if err != nil {
				return contract.AgentActivationResultV1{}, err
			}
		case contract.AgentActivationStepInvocationRecordedV1, contract.AgentActivationStepOutcomeUnknownV1:
			result, inspectErr := c.steps.OrderedV1()[stepIndex].InspectAgentActivationStepV1(context.WithoutCancel(ctx), stepRequest)
			if inspectErr != nil {
				return contract.AgentActivationResultV1{}, inspectErr
			}
			fact, err = c.recordStepResultV1(context.WithoutCancel(ctx), fact, stepRequest, result, cursor)
			if err != nil {
				return contract.AgentActivationResultV1{}, err
			}
		default:
			return contract.AgentActivationResultV1{}, core.NewError(core.ErrorInternal, core.ReasonInvalidState, "Agent activation coordination position is invalid")
		}
	}
}

func (c *AgentActivationCoordinatorV1) recordStepResultV1(ctx context.Context, fact contract.AgentActivationCoordinationFactV1, request contract.AgentActivationStepRequestV1, result contract.AgentActivationStepResultV1, cursor *activationClockCursorV1) (contract.AgentActivationCoordinationFactV1, error) {
	now, err := cursor.observe()
	if err != nil {
		return contract.AgentActivationCoordinationFactV1{}, err
	}
	if err := result.ValidateFor(request, now); err != nil {
		return contract.AgentActivationCoordinationFactV1{}, err
	}
	event, err := contract.SealAgentActivationStepEventV1(contract.AgentActivationStepEventV1{
		Sequence: uint32(len(fact.Events) + 1), Step: request.Step, State: contract.AgentActivationStepResultRecordedV1,
		AttemptID: request.AttemptID, RequestDigest: request.RequestDigest, Result: &result, RecordedUnixNano: now.UnixNano(),
	})
	if err != nil {
		return contract.AgentActivationCoordinationFactV1{}, err
	}
	var final *contract.AgentActivationResultV1
	if request.Step == contract.AgentActivationReadyInspectV1 {
		candidate := fact
		candidate.Events = append(append([]contract.AgentActivationStepEventV1{}, fact.Events...), event)
		built, buildErr := buildAgentActivationResultV1(candidate, fact.Request, now)
		if buildErr != nil {
			return contract.AgentActivationCoordinationFactV1{}, buildErr
		}
		final = &built
	}
	next, _, err := c.appendEventV1(ctx, fact, event, final)
	return next, err
}

func (c *AgentActivationCoordinatorV1) newIntentEventV1(request contract.AgentActivationStartRequestV1, stepIndex int, predecessor core.Digest, cursor *activationClockCursorV1) (contract.AgentActivationStepEventV1, error) {
	stepRequest, err := activationStepRequestV1(request, stepIndex, predecessor)
	if err != nil {
		return contract.AgentActivationStepEventV1{}, err
	}
	return c.newStateEventV1(stepRequest, contract.AgentActivationStepIntentRecordedV1, nil, cursor)
}

func (c *AgentActivationCoordinatorV1) newStateEventV1(request contract.AgentActivationStepRequestV1, state contract.AgentActivationStepEventStateV1, result *contract.AgentActivationStepResultV1, cursor *activationClockCursorV1) (contract.AgentActivationStepEventV1, error) {
	now, err := cursor.observe()
	if err != nil {
		return contract.AgentActivationStepEventV1{}, err
	}
	return contract.AgentActivationStepEventV1{
		Step: request.Step, State: state, AttemptID: request.AttemptID, RequestDigest: request.RequestDigest,
		Result: result, RecordedUnixNano: now.UnixNano(),
	}, nil
}

func (c *AgentActivationCoordinatorV1) ensureFactV1(ctx context.Context, next contract.AgentActivationCoordinationFactV1) (contract.AgentActivationCoordinationFactV1, error) {
	written, err := c.facts.EnsureAgentActivationCoordinationV1(ctx, next)
	if err == nil {
		return written, nil
	}
	inspected, inspectErr := c.facts.InspectAgentActivationCoordinationV1(context.WithoutCancel(ctx), next.ActivationID)
	if inspectErr == nil && inspected.Digest == next.Digest {
		return inspected, nil
	}
	return contract.AgentActivationCoordinationFactV1{}, err
}

func (c *AgentActivationCoordinatorV1) appendEventV1(ctx context.Context, current contract.AgentActivationCoordinationFactV1, event contract.AgentActivationStepEventV1, result *contract.AgentActivationResultV1) (contract.AgentActivationCoordinationFactV1, bool, error) {
	event.Sequence = uint32(len(current.Events) + 1)
	sealedEvent, err := contract.SealAgentActivationStepEventV1(event)
	if err != nil {
		return contract.AgentActivationCoordinationFactV1{}, false, err
	}
	next := current
	next.Revision++
	next.Events = append(append([]contract.AgentActivationStepEventV1{}, current.Events...), sealedEvent)
	next.Result = result
	next.Digest = ""
	next, err = contract.SealAgentActivationCoordinationFactV1(next)
	if err != nil {
		return contract.AgentActivationCoordinationFactV1{}, false, err
	}
	written, err := c.facts.CompareAndSwapAgentActivationCoordinationV1(ctx, applicationports.AgentActivationCoordinationCASRequestV1{
		ActivationID: current.ActivationID, ExpectedRevision: current.Revision, ExpectedDigest: current.Digest, Next: next,
	})
	if err == nil {
		if written.Revision != next.Revision || written.Digest != next.Digest {
			return contract.AgentActivationCoordinationFactV1{}, false, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Agent activation CAS success did not return the exact successor")
		}
		return written, true, nil
	}
	inspected, inspectErr := c.facts.InspectAgentActivationCoordinationV1(context.WithoutCancel(ctx), current.ActivationID)
	if inspectErr == nil && strictAgentActivationSuccessorV1(current, inspected) {
		return inspected, false, nil
	}
	return contract.AgentActivationCoordinationFactV1{}, false, err
}

func strictAgentActivationSuccessorV1(current, inspected contract.AgentActivationCoordinationFactV1) bool {
	if current.Validate() != nil || inspected.Validate() != nil || inspected.ActivationID != current.ActivationID || inspected.Request.RequestDigest != current.Request.RequestDigest || inspected.Revision <= current.Revision || len(inspected.Events) <= len(current.Events) {
		return false
	}
	for index := range current.Events {
		if inspected.Events[index].Digest != current.Events[index].Digest {
			return false
		}
	}
	return true
}

func activationStepRequestV1(request contract.AgentActivationStartRequestV1, stepIndex int, predecessor core.Digest) (contract.AgentActivationStepRequestV1, error) {
	steps := contract.AgentActivationStepOrderV1()
	if stepIndex < 0 || stepIndex >= len(steps) {
		return contract.AgentActivationStepRequestV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Agent activation step index is out of range")
	}
	return contract.SealAgentActivationStepRequestV1(contract.AgentActivationStepRequestV1{
		ActivationID: request.ActivationID, StartRequestDigest: request.RequestDigest, Step: steps[stepIndex],
		PredecessorResultDigest: predecessor, RequestedNotAfterUnixNano: request.RequestedNotAfterUnixNano,
	})
}

func coordinationPositionV1(fact contract.AgentActivationCoordinationFactV1) (int, contract.AgentActivationStepEventStateV1, core.Digest, error) {
	if err := fact.Validate(); err != nil {
		return 0, "", "", err
	}
	completed := 0
	predecessor := core.Digest("")
	for _, event := range fact.Events {
		if event.State == contract.AgentActivationStepResultRecordedV1 {
			completed++
			predecessor = event.Result.ResultDigest
		}
	}
	last := fact.Events[len(fact.Events)-1]
	if last.State == contract.AgentActivationStepResultRecordedV1 {
		if completed >= len(contract.AgentActivationStepOrderV1()) {
			return completed - 1, last.State, predecessor, nil
		}
		return completed, last.State, predecessor, nil
	}
	return completed, last.State, predecessor, nil
}

func buildAgentActivationResultV1(fact contract.AgentActivationCoordinationFactV1, request contract.AgentActivationStartRequestV1, now time.Time) (contract.AgentActivationResultV1, error) {
	results := make(map[contract.AgentActivationStepV1]contract.AgentActivationStepResultV1)
	expires := request.RequestedNotAfterUnixNano
	for _, event := range fact.Events {
		if event.State != contract.AgentActivationStepResultRecordedV1 {
			continue
		}
		results[event.Step] = *event.Result
		if event.Result.ExpiresUnixNano < expires {
			expires = event.Result.ExpiresUnixNano
		}
	}
	allocate := results[contract.AgentActivationSandboxAllocateV1]
	commit := results[contract.AgentActivationCommitV1]
	active := results[contract.AgentActivationSandboxActivateV1]
	ready := results[contract.AgentActivationReadyInspectV1]
	if allocate.SandboxLease == nil || allocate.SandboxLeaseCurrent == nil || commit.ExecutionScope == nil || commit.ActivationCurrent == nil || active.SandboxActiveCurrent == nil || ready.ExecutionReadyCurrent == nil || *allocate.SandboxLease != *commit.SandboxLease || *allocate.SandboxLease != *active.SandboxLease {
		return contract.AgentActivationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation step results do not form one exact execution closure")
	}
	scopeDigest, err := executionScopeDigestV1(*commit.ExecutionScope)
	if err != nil {
		return contract.AgentActivationResultV1{}, err
	}
	return contract.SealAgentActivationResultV1(contract.AgentActivationResultV1{
		ActivationID: request.ActivationID, AttemptID: request.AttemptID, RequestDigest: request.RequestDigest,
		ExecutionScope: *commit.ExecutionScope, ExecutionScopeDigest: scopeDigest, ActivationCurrent: *commit.ActivationCurrent,
		SandboxLease: *allocate.SandboxLease, SandboxLeaseCurrent: *allocate.SandboxLeaseCurrent,
		SandboxActiveCurrent: *active.SandboxActiveCurrent, ExecutionReadyCurrent: *ready.ExecutionReadyCurrent,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	})
}

func executionScopeDigestV1(scope core.ExecutionScope) (core.Digest, error) {
	return runtimeports.ExecutionScopeDigestV2(scope)
}

type activationClockCursorV1 struct {
	read func() time.Time
	last time.Time
}

func (c *activationClockCursorV1) observe() (time.Time, error) {
	now := c.read()
	if now.IsZero() || (!c.last.IsZero() && now.Before(c.last)) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Agent activation coordination clock regressed")
	}
	c.last = now
	return now, nil
}

func cloneAgentActivationResultV1(value contract.AgentActivationResultV1) contract.AgentActivationResultV1 {
	clone := value
	if value.ExecutionScope.SandboxLease != nil {
		lease := *value.ExecutionScope.SandboxLease
		clone.ExecutionScope.SandboxLease = &lease
	}
	return clone
}

func lifecycleNilV1(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}

var _ applicationports.AgentActivationPortV1 = (*AgentActivationCoordinatorV1)(nil)
