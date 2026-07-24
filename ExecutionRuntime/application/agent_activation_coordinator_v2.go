package application

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type AgentActivationCoordinatorV2 struct {
	facts applicationports.AgentActivationCoordinationFactPortV2
	steps applicationports.AgentActivationStepPortsV2
	clock func() time.Time
}

func NewAgentActivationCoordinatorV2(facts applicationports.AgentActivationCoordinationFactPortV2, steps applicationports.AgentActivationStepPortsV2, clock func() time.Time) (*AgentActivationCoordinatorV2, error) {
	if activationNilV2(facts) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Agent activation V2 Fact port and clock are required")
	}
	for _, step := range steps.OrderedV2() {
		if activationNilV2(step) {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "All Agent activation V2 step ports are required")
		}
	}
	return &AgentActivationCoordinatorV2{facts: facts, steps: steps, clock: clock}, nil
}

func (c *AgentActivationCoordinatorV2) StartOrInspectAgentActivationV2(ctx context.Context, request contract.AgentActivationStartRequestV2) (contract.AgentActivationResultV2, error) {
	if c == nil || ctx == nil || ctx.Err() != nil {
		return contract.AgentActivationResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "Agent activation V2 coordinator or context is unavailable")
	}
	if err := request.Validate(); err != nil {
		return contract.AgentActivationResultV2{}, err
	}
	fact, err := c.facts.InspectAgentActivationCoordinationV2(ctx, request.ActivationID)
	if err != nil {
		if !core.HasCategory(err, core.ErrorNotFound) {
			return contract.AgentActivationResultV2{}, err
		}
		cursor := activationClockCursorV2{read: c.clock}
		now, err := cursor.observe()
		if err != nil {
			return contract.AgentActivationResultV2{}, err
		}
		first, err := newActivationIntentEventV2(request, contract.AgentActivationPreflightV2, nil, 1, now)
		if err != nil {
			return contract.AgentActivationResultV2{}, err
		}
		initial, err := contract.NewAgentActivationCoordinationFactV2(request, first, now)
		if err != nil {
			return contract.AgentActivationResultV2{}, err
		}
		receipt, createErr := c.facts.CreateAgentActivationCoordinationV2(ctx, initial)
		if createErr == nil {
			fact = receipt.Fact
		} else {
			inspected, inspectErr := c.facts.InspectAgentActivationCoordinationV2(context.WithoutCancel(ctx), request.ActivationID)
			if inspectErr != nil || inspected.Digest != initial.Digest {
				return contract.AgentActivationResultV2{}, createErr
			}
			fact = inspected
		}
	}
	if fact.Request.RequestDigest != request.RequestDigest {
		return contract.AgentActivationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Agent activation V2 ID binds another Start request")
	}
	last := time.Time{}
	if len(fact.Events) != 0 {
		last = time.Unix(0, fact.Events[len(fact.Events)-1].RecordedUnixNano)
	}
	cursor := activationClockCursorV2{read: c.clock, last: last}
	return c.resumeActivationV2(ctx, fact, &cursor)
}

func (c *AgentActivationCoordinatorV2) InspectAgentActivationV2(ctx context.Context, request contract.AgentActivationStartRequestV2) (contract.AgentActivationResultV2, error) {
	if c == nil || ctx == nil || ctx.Err() != nil {
		return contract.AgentActivationResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "Agent activation V2 coordinator or context is unavailable")
	}
	if err := request.Validate(); err != nil {
		return contract.AgentActivationResultV2{}, err
	}
	fact, err := c.facts.InspectAgentActivationCoordinationV2(ctx, request.ActivationID)
	if err != nil {
		return contract.AgentActivationResultV2{}, err
	}
	if fact.Request.RequestDigest != request.RequestDigest {
		return contract.AgentActivationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Agent activation V2 Inspect request drifted")
	}
	if fact.Result == nil {
		return contract.AgentActivationResultV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "Agent activation V2 is not complete")
	}
	now := c.clock()
	if err := fact.Result.ValidateFor(request, now); err != nil {
		return contract.AgentActivationResultV2{}, err
	}
	return *fact.Result, nil
}

func (c *AgentActivationCoordinatorV2) resumeActivationV2(ctx context.Context, fact contract.AgentActivationCoordinationFactV2, cursor *activationClockCursorV2) (contract.AgentActivationResultV2, error) {
	for {
		if fact.Result != nil {
			now, err := cursor.observe()
			if err != nil {
				return contract.AgentActivationResultV2{}, err
			}
			if err = fact.Result.ValidateFor(fact.Request, now); err != nil {
				return contract.AgentActivationResultV2{}, err
			}
			return *fact.Result, nil
		}
		stepIndex, state, predecessor, invocation, err := activationPositionV2(fact)
		if err != nil {
			return contract.AgentActivationResultV2{}, err
		}
		step := contract.AgentActivationStepOrderV2()[stepIndex]
		owner := c.steps.OrderedV2()[stepIndex]
		switch state {
		case contract.AgentActivationStepResultRecordedV2:
			now, err := cursor.observe()
			if err != nil {
				return contract.AgentActivationResultV2{}, err
			}
			intent, err := newActivationIntentEventV2(fact.Request, step, predecessor, uint32(len(fact.Events)+1), now)
			if err != nil {
				return contract.AgentActivationResultV2{}, err
			}
			fact, _, err = c.appendActivationEventV2(ctx, fact, intent, nil)
			if err != nil {
				return contract.AgentActivationResultV2{}, err
			}
		case contract.AgentActivationStepIntentRecordedV2:
			last := fact.Events[len(fact.Events)-1]
			prepared, err := owner.PrepareAgentActivationStepV2(ctx, applicationports.AgentActivationStepPreparationV2{
				Coordination: fact.RefV2(), InvocationSequence: uint32(len(fact.Events) + 1), InvocationEventDigest: last.Digest,
				Step: step, Predecessor: predecessor, Start: fact.Request, RequestedNotAfterUnixNano: fact.Request.RequestedNotAfterUnixNano,
			})
			if err != nil {
				return contract.AgentActivationResultV2{}, err
			}
			if err = validatePreparedActivationRequestV2(fact, step, predecessor, last, prepared); err != nil {
				return contract.AgentActivationResultV2{}, err
			}
			now, err := cursor.observe()
			if err != nil {
				return contract.AgentActivationResultV2{}, err
			}
			event, err := contract.SealAgentActivationStepEventV2(contract.AgentActivationStepEventV2{Sequence: uint32(len(fact.Events) + 1), Step: step, State: contract.AgentActivationStepInvocationRecordedV2, AttemptID: prepared.AttemptID, RequestDigest: prepared.RequestDigest, Request: &prepared, RecordedUnixNano: now.UnixNano()})
			if err != nil {
				return contract.AgentActivationResultV2{}, err
			}
			var owned bool
			fact, owned, err = c.appendActivationEventV2(ctx, fact, event, nil)
			if err != nil {
				return contract.AgentActivationResultV2{}, err
			}
			if !owned {
				// Recovery of invocation_recorded never owns Start permission.
				continue
			}
			result, startErr := owner.StartOrInspectAgentActivationStepV2(ctx, prepared)
			if startErr != nil {
				now, clockErr := cursor.observe()
				if clockErr != nil {
					return contract.AgentActivationResultV2{}, clockErr
				}
				unknown, eventErr := contract.SealAgentActivationStepEventV2(contract.AgentActivationStepEventV2{Sequence: uint32(len(fact.Events) + 1), Step: step, State: contract.AgentActivationStepOutcomeUnknownV2, AttemptID: prepared.AttemptID, RequestDigest: prepared.RequestDigest, Request: &prepared, RecordedUnixNano: now.UnixNano()})
				if eventErr != nil {
					return contract.AgentActivationResultV2{}, eventErr
				}
				fact, _, err = c.appendActivationEventV2(context.WithoutCancel(ctx), fact, unknown, nil)
				if err != nil {
					return contract.AgentActivationResultV2{}, err
				}
				inspected, inspectErr := owner.InspectAgentActivationStepV2(context.WithoutCancel(ctx), prepared)
				if inspectErr != nil {
					return contract.AgentActivationResultV2{}, startErr
				}
				result = inspected
			}
			fact, err = c.recordActivationResultV2(ctx, fact, prepared, result, cursor)
			if err != nil {
				return contract.AgentActivationResultV2{}, err
			}
		case contract.AgentActivationStepInvocationRecordedV2, contract.AgentActivationStepOutcomeUnknownV2:
			if invocation == nil {
				return contract.AgentActivationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Persisted activation invocation request is absent")
			}
			result, inspectErr := owner.InspectAgentActivationStepV2(context.WithoutCancel(ctx), *invocation)
			if inspectErr != nil {
				return contract.AgentActivationResultV2{}, inspectErr
			}
			fact, err = c.recordActivationResultV2(ctx, fact, *invocation, result, cursor)
			if err != nil {
				return contract.AgentActivationResultV2{}, err
			}
		default:
			return contract.AgentActivationResultV2{}, core.NewError(core.ErrorInternal, core.ReasonInvalidState, "Agent activation V2 coordination position is invalid")
		}
	}
}

func (c *AgentActivationCoordinatorV2) recordActivationResultV2(ctx context.Context, fact contract.AgentActivationCoordinationFactV2, request contract.AgentActivationStepRequestV2, result contract.AgentActivationStepResultV2, cursor *activationClockCursorV2) (contract.AgentActivationCoordinationFactV2, error) {
	now, err := cursor.observe()
	if err != nil {
		return contract.AgentActivationCoordinationFactV2{}, err
	}
	if err = result.ValidateFor(request, now); err != nil {
		return contract.AgentActivationCoordinationFactV2{}, err
	}
	event, err := contract.SealAgentActivationStepEventV2(contract.AgentActivationStepEventV2{Sequence: uint32(len(fact.Events) + 1), Step: request.Step, State: contract.AgentActivationStepResultRecordedV2, AttemptID: request.AttemptID, RequestDigest: request.RequestDigest, Request: &request, Result: &result, RecordedUnixNano: now.UnixNano()})
	if err != nil {
		return contract.AgentActivationCoordinationFactV2{}, err
	}
	var final *contract.AgentActivationResultV2
	if request.Step == contract.AgentActivationReadyInspectV2 {
		candidate := fact
		candidate.Events = append(append([]contract.AgentActivationStepEventV2{}, fact.Events...), event)
		built, buildErr := buildAgentActivationResultV2(candidate, fact.Request, now)
		if buildErr != nil {
			return contract.AgentActivationCoordinationFactV2{}, buildErr
		}
		final = &built
	}
	written, _, err := c.appendActivationEventV2(ctx, fact, event, final)
	return written, err
}

func (c *AgentActivationCoordinatorV2) appendActivationEventV2(ctx context.Context, current contract.AgentActivationCoordinationFactV2, event contract.AgentActivationStepEventV2, result *contract.AgentActivationResultV2) (contract.AgentActivationCoordinationFactV2, bool, error) {
	next := current
	next.Revision++
	next.Events = append(append([]contract.AgentActivationStepEventV2{}, current.Events...), event)
	next.Result = result
	next.UpdatedUnixNano = event.RecordedUnixNano
	next.Digest = ""
	next, err := contract.SealAgentActivationCoordinationFactV2(next)
	if err != nil {
		return contract.AgentActivationCoordinationFactV2{}, false, err
	}
	receipt, err := c.facts.CompareAndSwapAgentActivationCoordinationV2(ctx, applicationports.AgentActivationCoordinationCASRequestV2{ActivationID: current.ActivationID, ExpectedRevision: current.Revision, ExpectedDigest: current.Digest, Next: next})
	if err == nil {
		if !receipt.Applied || receipt.Fact.Digest != next.Digest || receipt.Fact.Revision != next.Revision {
			return contract.AgentActivationCoordinationFactV2{}, false, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Agent activation V2 CAS did not return the exact applied successor")
		}
		return receipt.Fact, true, nil
	}
	inspected, inspectErr := c.facts.InspectAgentActivationCoordinationV2(context.WithoutCancel(ctx), current.ActivationID)
	if inspectErr == nil && strictActivationSuccessorV2(current, inspected) {
		return inspected, false, nil
	}
	return contract.AgentActivationCoordinationFactV2{}, false, err
}

func validatePreparedActivationRequestV2(fact contract.AgentActivationCoordinationFactV2, step contract.AgentActivationStepV2, predecessor *contract.AgentActivationStepResultRefV2, intent contract.AgentActivationStepEventV2, request contract.AgentActivationStepRequestV2) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if request.Coordination != fact.RefV2() || request.InvocationSequence != intent.Sequence+1 || request.InvocationEventDigest != intent.Digest || request.Step != step || request.Inputs.ProposedScope != fact.Request.ProposedScope || !sameActivationRefV2(request.Inputs.Predecessor, predecessor) || request.RequestedNotAfterUnixNano != fact.Request.RequestedNotAfterUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Prepared activation request drifted from the exact coordination state")
	}
	return nil
}

func newActivationIntentEventV2(request contract.AgentActivationStartRequestV2, step contract.AgentActivationStepV2, predecessor *contract.AgentActivationStepResultRefV2, sequence uint32, now time.Time) (contract.AgentActivationStepEventV2, error) {
	baseDigest, err := contract.AgentActivationStepIntentBaseDigestV2(request, step, predecessor)
	if err != nil {
		return contract.AgentActivationStepEventV2{}, err
	}
	attempt, err := contract.DeriveAgentActivationStepAttemptIDV2(request.ActivationID, request.RequestDigest, step)
	if err != nil {
		return contract.AgentActivationStepEventV2{}, err
	}
	return contract.SealAgentActivationStepEventV2(contract.AgentActivationStepEventV2{Sequence: sequence, Step: step, State: contract.AgentActivationStepIntentRecordedV2, AttemptID: attempt, RequestDigest: baseDigest, RecordedUnixNano: now.UnixNano()})
}

func activationPositionV2(fact contract.AgentActivationCoordinationFactV2) (int, contract.AgentActivationStepEventStateV2, *contract.AgentActivationStepResultRefV2, *contract.AgentActivationStepRequestV2, error) {
	if err := fact.Validate(); err != nil {
		return 0, "", nil, nil, err
	}
	completed := 0
	var predecessor *contract.AgentActivationStepResultRefV2
	var invocation *contract.AgentActivationStepRequestV2
	for _, event := range fact.Events {
		if event.Request != nil {
			copy := *event.Request
			invocation = &copy
		}
		if event.State == contract.AgentActivationStepResultRecordedV2 {
			completed++
			copy := event.Result.Ref
			predecessor = &copy
			invocation = nil
		}
	}
	last := fact.Events[len(fact.Events)-1]
	if last.State == contract.AgentActivationStepResultRecordedV2 && completed < len(contract.AgentActivationStepOrderV2()) {
		return completed, last.State, predecessor, nil, nil
	}
	if completed >= len(contract.AgentActivationStepOrderV2()) {
		return completed - 1, last.State, predecessor, invocation, nil
	}
	return completed, last.State, predecessor, invocation, nil
}

func buildAgentActivationResultV2(fact contract.AgentActivationCoordinationFactV2, request contract.AgentActivationStartRequestV2, now time.Time) (contract.AgentActivationResultV2, error) {
	results := map[contract.AgentActivationStepV2]contract.AgentActivationStepResultV2{}
	expires := request.RequestedNotAfterUnixNano
	for _, event := range fact.Events {
		if event.State == contract.AgentActivationStepResultRecordedV2 {
			results[event.Step] = *event.Result
			if event.Result.Proof.ExpiresUnixNano < expires {
				expires = event.Result.Proof.ExpiresUnixNano
			}
		}
	}
	commit, active, open, ready := results[contract.AgentActivationCommitV2], results[contract.AgentActivationSandboxActivateV2], results[contract.AgentActivationExecutionOpenV2], results[contract.AgentActivationReadyInspectV2]
	if commit.Proof.CommittedScope == nil || commit.Proof.Lease == nil || active.Proof.Lease == nil || open.Proof.Lease == nil || ready.Proof.Lease == nil || open.Proof.EndpointCurrent == nil || ready.Proof.SecondaryCurrent == nil || ready.Proof.EndpointCurrent == nil || *commit.Proof.Lease != *active.Proof.Lease || *commit.Proof.Lease != *open.Proof.Lease || *commit.Proof.Lease != *ready.Proof.Lease || ready.Proof.PrimaryCurrent != commit.Proof.PrimaryCurrent || *ready.Proof.SecondaryCurrent != active.Proof.PrimaryCurrent {
		return contract.AgentActivationResultV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Agent activation V2 step results do not form one exact ready closure")
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(*commit.Proof.CommittedScope)
	if err != nil {
		return contract.AgentActivationResultV2{}, err
	}
	result, err := contract.SealAgentActivationResultV2(contract.AgentActivationResultV2{ExecutionScope: *commit.Proof.CommittedScope, ExecutionScopeDigest: scopeDigest, ActivationCurrent: commit.Proof.PrimaryCurrent, SandboxActiveCurrent: active.Proof.PrimaryCurrent, ExecutionOpenCurrent: open.Proof.PrimaryCurrent, EndpointCurrent: *open.Proof.EndpointCurrent, ExecutionReadyCurrent: *ready.Proof.EndpointCurrent, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}, request)
	if err != nil {
		return contract.AgentActivationResultV2{}, err
	}
	if err = result.ValidateFor(request, now); err != nil {
		return contract.AgentActivationResultV2{}, err
	}
	return result, nil
}

func strictActivationSuccessorV2(current, next contract.AgentActivationCoordinationFactV2) bool {
	if current.Validate() != nil || next.Validate() != nil || next.ActivationID != current.ActivationID || next.Request.RequestDigest != current.Request.RequestDigest || next.Revision <= current.Revision || len(next.Events) <= len(current.Events) {
		return false
	}
	for index := range current.Events {
		if next.Events[index].Digest != current.Events[index].Digest {
			return false
		}
	}
	return true
}

func sameActivationRefV2(left, right *contract.AgentActivationStepResultRefV2) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
func activationNilV2(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

type activationClockCursorV2 struct {
	read func() time.Time
	last time.Time
}

func (c *activationClockCursorV2) observe() (time.Time, error) {
	if c == nil || c.read == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Agent activation V2 clock is unavailable")
	}
	now := c.read()
	if now.IsZero() || (!c.last.IsZero() && now.Before(c.last)) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Agent activation V2 clock regressed")
	}
	c.last = now
	return now, nil
}

var _ applicationports.AgentActivationPortV2 = (*AgentActivationCoordinatorV2)(nil)
