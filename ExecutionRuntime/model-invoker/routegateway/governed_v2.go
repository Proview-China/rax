package routegateway

import (
	"context"
	"errors"
	"strings"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type GovernedModelTurnDependenciesV2 struct {
	PreparedHistory modelinvoker.PreparedModelInvocationReaderV1
	PreparedCurrent modelinvoker.PreparedModelInvocationCurrentReaderV1
	CommitGate      modelinvoker.PreparedModelInvocationCommitGateV1
	Materials       modelinvoker.InvocationMaterialReaderV1
	Turns           modelinvoker.GovernedModelTurnRepositoryV2
}

func (d GovernedModelTurnDependenciesV2) validate() error {
	if nilInterface(d.PreparedHistory) || nilInterface(d.PreparedCurrent) || nilInterface(d.CommitGate) || nilInterface(d.Materials) || nilInterface(d.Turns) {
		return gatewayError(modelinvoker.ErrorInvalidRequest, "governed_turn_v2_dependencies_required", "governed model turn requires durable exact Prepared, material, gate and turn repositories", nil)
	}
	return nil
}

func (g *Gateway) StartOrInspectGovernedModelTurnV2(ctx context.Context, command modelinvoker.GovernedModelTurnCommandV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	if g == nil || g.now == nil || g.governedTurnV2 == nil {
		return modelinvoker.GovernedModelTurnOutcomeV2{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorUnavailable, "start_turn_v2", "governed model turn is unavailable", nil)
	}
	if ctx == nil || ctx.Err() != nil {
		return modelinvoker.GovernedModelTurnOutcomeV2{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "start_turn_v2", "live context is required", nil)
	}
	baseline := g.now()
	if baseline.IsZero() {
		return modelinvoker.GovernedModelTurnOutcomeV2{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "start_turn_v2", "clock returned zero", nil)
	}
	historical0, current0, material0, err := g.readGovernedTurnInputsV2(ctx, command, baseline)
	if err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV2{}, err
	}
	if err := current0.ValidateCurrent(command.CurrentRef, baseline); err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV2{}, err
	}
	prepared, err := modelinvoker.NewPreparedGovernedModelTurnV2(command, baseline)
	if err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV2{}, err
	}
	created, err := g.governedTurnV2.Turns.CreateGovernedModelTurnV2(ctx, prepared)
	if err != nil {
		if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate {
			return modelinvoker.GovernedModelTurnOutcomeV2{}, err
		}
		recovered, inspectErr := g.governedTurnV2.Turns.InspectExactGovernedModelTurnV2(context.WithoutCancel(ctx), prepared.RefV2())
		if inspectErr != nil {
			return modelinvoker.GovernedModelTurnOutcomeV2{}, errors.Join(err, inspectErr)
		}
		created = modelinvoker.GovernedModelTurnMutationV2{Outcome: recovered}
	}
	attempt := created.Outcome
	if attempt.State != modelinvoker.GovernedModelTurnPreparedV2 {
		return turnResultForStateV2(attempt)
	}
	s1 := g.now()
	if s1.IsZero() || s1.Before(baseline) {
		return attempt, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "turn_s1_clock", "clock regressed before S1", nil)
	}
	historical1, current1, material1, err := g.readGovernedTurnInputsV2(ctx, command, s1)
	if err != nil {
		return attempt, err
	}
	if historical0.Ref() != historical1.Ref() || current0.Ref() != current1.Ref() || material0.RefV1() != material1.RefV1() {
		return attempt, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "turn_s1", "durable exact inputs drifted", nil)
	}
	ack, err := modelinvoker.CrossPreparedModelInvocationCommitGateV1(ctx, g.governedTurnV2.CommitGate, command.PreparedRef, command.CurrentRef)
	if err != nil {
		if deterministicBeforeProviderBoundaryV2(err) {
			return g.rejectGovernedTurnBeforeBoundaryV2(ctx, attempt, "commit_gate_rejected", g.now(), err)
		}
		return attempt, closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "turn_gate", "commit gate did not return exact ACK", err)
	}
	preparedProvider, err := g.prepareAt(ctx, material1.Call, g.now())
	if err != nil {
		if deterministicBeforeProviderBoundaryV2(err) {
			return g.rejectGovernedTurnBeforeBoundaryV2(ctx, attempt, "provider_prepare_rejected", g.now(), err)
		}
		return attempt, closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorUnavailable, "provider_prepare_unavailable", "provider preparation is retryable before boundary", err)
	}
	release := true
	defer func() {
		if release {
			_ = preparedProvider.lease.release()
		}
	}()
	actualRouteDigest, err := modelinvoker.DigestGovernedRouteSelectionV1(preparedProvider.resolution.Route)
	if err != nil || actualRouteDigest != material1.RouteDigest {
		return g.rejectGovernedTurnBeforeBoundaryV2(ctx, attempt, "route_selection_drift", g.now(), errors.Join(err, errors.New("resolved route differs from authorized material")))
	}
	s2 := g.now()
	if s2.IsZero() || s2.Before(s1) {
		return g.rejectGovernedTurnBeforeBoundaryV2(ctx, attempt, "turn_s2_clock", s2, nil)
	}
	historical2, current2, material2, err := g.readGovernedTurnInputsV2(ctx, command, s2)
	if err != nil {
		return attempt, err
	}
	ack2, err := g.governedTurnV2.CommitGate.InspectExactAck(ctx, ack.Ref())
	if err != nil {
		return attempt, err
	}
	if historical1.Ref() != historical2.Ref() || current1.Ref() != current2.Ref() || material1.RefV1() != material2.RefV1() || ack.Ref() != ack2.Ref() {
		return attempt, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "turn_s2", "durable exact inputs or ACK drifted", nil)
	}
	fresh := g.now()
	if fresh.IsZero() || fresh.Before(s2) || !fresh.Before(time.Unix(0, material2.ExpiresUnixNano)) || !fresh.Before(time.Unix(0, attempt.ExpiresUnixNano)) || current2.ValidateCurrent(command.CurrentRef, fresh) != nil || ack2.ValidateCurrent(current2, fresh) != nil {
		return g.rejectGovernedTurnBeforeBoundaryV2(ctx, attempt, "actual_point_not_current", fresh, nil)
	}
	receipt, err := modelinvoker.SealPreparedModelInvocationDispatchReceiptAgainstV1(historical2, current2, ack2, modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1{PreparedRef: command.PreparedRef, CurrentRef: command.CurrentRef, AckRef: ack2.Ref(), DispatchSequence: command.DispatchSequence, BoundaryKind: modelinvoker.GovernedModelTurnProviderBoundaryKindV2, ProviderAttemptOrdinal: command.ProviderAttemptOrdinal, AttemptRequestDigest: command.AttemptRequestDigest, ActualToolSurfaceDigest: historical2.ActualToolSurfaceDigest, ActualProviderInjectionDigest: historical2.ActualProviderInjectionDigest, CheckedUnixNano: fresh.UnixNano()}, fresh)
	if err != nil {
		return attempt, err
	}
	boundary := attempt.CloneV2()
	boundary.Revision = 2
	boundary.State = modelinvoker.GovernedModelTurnProviderBoundaryCrossedV2
	boundary.UpdatedUnixNano = fresh.UnixNano()
	boundary.ExpiresUnixNano = minTimeUnixNanoV1(attempt.ExpiresUnixNano, material2.ExpiresUnixNano, current2.ExpiresUnixNano, current2.NotAfterUnixNano, ack2.ExpiresUnixNano, ack2.NotAfterUnixNano)
	ackRef := ack2.Ref()
	boundary.AckRef = &ackRef
	boundary.DispatchReceipt = &receipt
	boundary.Digest = ""
	boundary, err = modelinvoker.SealGovernedModelTurnOutcomeV2(boundary)
	if err != nil {
		return attempt, err
	}
	mutation, err := g.governedTurnV2.Turns.CompareAndSwapGovernedModelTurnV2(ctx, modelinvoker.GovernedModelTurnCASV2{Expected: attempt.RefV2(), Next: boundary})
	if err != nil {
		if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) == modelinvoker.GovernedModelInvocationErrorIndeterminate {
			recovered, inspectErr := g.governedTurnV2.Turns.InspectExactGovernedModelTurnV2(context.WithoutCancel(ctx), boundary.RefV2())
			if inspectErr == nil {
				return recovered, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "turn_boundary_reply_lost", "provider boundary may have been committed; exact Inspect recovered it and provider dispatch is forbidden", err)
			}
			return attempt, errors.Join(err, inspectErr)
		}
		return attempt, err
	}
	if !mutation.Applied {
		return turnResultForStateV2(mutation.Outcome)
	}
	boundary = mutation.Outcome
	physical := g.now()
	if physical.IsZero() || physical.Before(fresh) || !physical.Before(time.Unix(0, boundary.ExpiresUnixNano)) {
		return g.finishGovernedTurnAfterBoundaryV2(ctx, boundary, "physical_point_not_current", physical, nil)
	}
	historical3, current3, material3, err := g.readGovernedTurnInputsV2(ctx, command, physical)
	if err != nil {
		return g.finishGovernedTurnAfterBoundaryV2(context.WithoutCancel(ctx), boundary, "physical_owner_read_failed", physical, err)
	}
	ack3, err := g.governedTurnV2.CommitGate.InspectExactAck(ctx, ack2.Ref())
	if err != nil {
		return g.finishGovernedTurnAfterBoundaryV2(context.WithoutCancel(ctx), boundary, "physical_ack_read_failed", physical, err)
	}
	if historical3.Ref() != historical2.Ref() || current3.Ref() != current2.Ref() || material3.RefV1() != material2.RefV1() || ack3.Ref() != ack2.Ref() || current3.ValidateCurrent(command.CurrentRef, physical) != nil || ack3.ValidateCurrent(current3, physical) != nil {
		return g.finishGovernedTurnAfterBoundaryV2(context.WithoutCancel(ctx), boundary, "physical_owner_drift", physical, nil)
	}
	actualPoint := g.now()
	if actualPoint.IsZero() || actualPoint.Before(physical) ||
		!actualPoint.Before(time.Unix(0, boundary.ExpiresUnixNano)) ||
		current3.ValidateCurrent(command.CurrentRef, actualPoint) != nil ||
		material3.ValidateAgainstPreparedV1(historical3, current3, actualPoint) != nil ||
		ack3.ValidateCurrent(current3, actualPoint) != nil ||
		historical3.RouteDigest != material3.RouteDigest ||
		historical3.ProfileDigest != material3.ProfileDigest ||
		material3.Call.Request.Model != preparedProvider.request.Model ||
		actualRouteDigest != material3.RouteDigest {
		return g.finishGovernedTurnAfterBoundaryV2(context.WithoutCancel(ctx), boundary, "actual_point_s3_not_current", actualPoint, nil)
	}
	invokeResult, invokeErr := g.invokePrepared(ctx, preparedProvider)
	release = false
	if invokeErr != nil {
		return g.finishGovernedTurnAfterBoundaryV2(context.WithoutCancel(ctx), boundary, "provider_outcome_unknown", g.now(), invokeErr)
	}
	observation, err := observeGovernedModelTurnV2(boundary, invokeResult, historical3.Ref(), material3, g.now())
	if err != nil {
		return g.finishGovernedTurnAfterBoundaryV2(context.WithoutCancel(ctx), boundary, "provider_observation_invalid", g.now(), err)
	}
	observed := boundary.CloneV2()
	observed.Revision = 3
	observed.State = modelinvoker.GovernedModelTurnObservedV2
	observed.UpdatedUnixNano = observation.ObservedUnixNano
	observed.Observation = &observation
	observed.Digest = ""
	observed, err = modelinvoker.SealGovernedModelTurnOutcomeV2(observed)
	if err != nil {
		return boundary, err
	}
	request := modelinvoker.GovernedModelTurnCASV2{Expected: boundary.RefV2(), Next: observed}
	if observation.OutcomeKind == modelinvoker.GovernedModelTurnToolCallCandidateV2 {
		mutation, err = g.governedTurnV2.Turns.CompareAndSwapObservedGovernedModelTurnV2(ctx, request)
	} else {
		mutation, err = g.governedTurnV2.Turns.CompareAndSwapGovernedModelTurnV2(ctx, request)
	}
	if err != nil {
		if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) == modelinvoker.GovernedModelInvocationErrorIndeterminate {
			recovered, inspectErr := g.governedTurnV2.Turns.InspectExactGovernedModelTurnV2(context.WithoutCancel(ctx), observed.RefV2())
			if inspectErr == nil {
				return recovered, nil
			}
			return boundary, errors.Join(err, inspectErr)
		}
		return boundary, err
	}
	return mutation.Outcome, nil
}
func (g *Gateway) InspectExactGovernedModelTurnV2(ctx context.Context, ref modelinvoker.GovernedModelTurnRefV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	if g == nil || g.governedTurnV2 == nil {
		return modelinvoker.GovernedModelTurnOutcomeV2{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorUnavailable, "inspect_turn_v2", "governed model turn unavailable", nil)
	}
	return g.governedTurnV2.Turns.InspectExactGovernedModelTurnV2(ctx, ref)
}
func (g *Gateway) readGovernedTurnInputsV2(ctx context.Context, command modelinvoker.GovernedModelTurnCommandV2, now time.Time) (modelinvoker.PreparedModelInvocationFactV1, modelinvoker.PreparedModelInvocationCurrentProjectionV1, modelinvoker.InvocationMaterialV1, error) {
	if now.IsZero() {
		return modelinvoker.PreparedModelInvocationFactV1{}, modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, modelinvoker.InvocationMaterialV1{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "read_turn_inputs_v2", "validation clock is zero", nil)
	}
	h, err := g.governedTurnV2.PreparedHistory.InspectExactPreparedModelInvocationV1(ctx, command.PreparedRef)
	if err != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, modelinvoker.InvocationMaterialV1{}, err
	}
	c, err := g.governedTurnV2.PreparedCurrent.InspectExactPreparedModelInvocationCurrentV1(ctx, command.CurrentRef)
	if err != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, modelinvoker.InvocationMaterialV1{}, err
	}
	m, err := g.governedTurnV2.Materials.InspectExactInvocationMaterialV1(ctx, command.MaterialRef)
	if err != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, modelinvoker.InvocationMaterialV1{}, err
	}
	if h.Ref() != command.PreparedRef || c.Ref() != command.CurrentRef || c.ValidateAgainstFact(h) != nil || c.ValidateCurrent(command.CurrentRef, now) != nil || m.RefV1() != command.MaterialRef || m.ValidateAgainstPreparedV1(h, c, now) != nil || command.AttemptRequestDigest != h.UnifiedRequestDigest || command.RouteCallDigest != m.RouteCallDigest {
		return modelinvoker.PreparedModelInvocationFactV1{}, modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, modelinvoker.InvocationMaterialV1{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "read_turn_inputs_v2", "exact durable inputs drifted", nil)
	}
	return h.Clone(), c.Clone(), m.CloneV1(), nil
}
func (g *Gateway) rejectGovernedTurnBeforeBoundaryV2(ctx context.Context, current modelinvoker.GovernedModelTurnOutcomeV2, code string, now time.Time, cause error) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	next := current.CloneV2()
	next.Revision = 2
	next.State = modelinvoker.GovernedModelTurnRejectedNoEffectV2
	next.FailureCode = code
	if !now.IsZero() && now.UnixNano() >= next.UpdatedUnixNano {
		next.UpdatedUnixNano = now.UnixNano()
	}
	next.Digest = ""
	sealed, err := modelinvoker.SealGovernedModelTurnOutcomeV2(next)
	if err != nil {
		return current, err
	}
	mutation, err := g.governedTurnV2.Turns.CompareAndSwapGovernedModelTurnV2(ctx, modelinvoker.GovernedModelTurnCASV2{Expected: current.RefV2(), Next: sealed})
	if err != nil {
		if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) == modelinvoker.GovernedModelInvocationErrorIndeterminate {
			recovered, inspectErr := g.governedTurnV2.Turns.InspectExactGovernedModelTurnV2(context.WithoutCancel(ctx), sealed.RefV2())
			if inspectErr == nil {
				if cause == nil {
					cause = errors.New(code)
				}
				return recovered, closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "turn_rejected_no_effect", "provider boundary was not crossed", cause)
			}
			return current, errors.Join(err, inspectErr)
		}
		return current, errors.Join(cause, err)
	}
	if cause == nil {
		cause = errors.New(code)
	}
	return mutation.Outcome, closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "turn_rejected_no_effect", "provider boundary was not crossed", cause)
}
func (g *Gateway) finishGovernedTurnAfterBoundaryV2(ctx context.Context, boundary modelinvoker.GovernedModelTurnOutcomeV2, code string, now time.Time, cause error) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	terminal := boundary.CloneV2()
	terminal.Revision = 3
	terminal.State = modelinvoker.GovernedModelTurnUnknownV2
	terminal.FailureCode = code
	if !now.IsZero() && now.UnixNano() >= terminal.UpdatedUnixNano {
		terminal.UpdatedUnixNano = now.UnixNano()
	}
	terminal.Digest = ""
	sealed, err := modelinvoker.SealGovernedModelTurnOutcomeV2(terminal)
	if err != nil {
		return boundary, err
	}
	mutation, err := g.governedTurnV2.Turns.CompareAndSwapGovernedModelTurnV2(ctx, modelinvoker.GovernedModelTurnCASV2{Expected: boundary.RefV2(), Next: sealed})
	if err != nil {
		if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) == modelinvoker.GovernedModelInvocationErrorIndeterminate {
			recovered, inspectErr := g.governedTurnV2.Turns.InspectExactGovernedModelTurnV2(context.WithoutCancel(ctx), sealed.RefV2())
			if inspectErr == nil {
				if cause == nil {
					cause = errors.New(code)
				}
				return recovered, closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "turn_unknown", "provider outcome requires exact Inspect", cause)
			}
			return boundary, errors.Join(err, inspectErr)
		}
		return boundary, errors.Join(cause, err)
	}
	if cause == nil {
		cause = errors.New(code)
	}
	return mutation.Outcome, closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "turn_unknown", "provider outcome requires exact Inspect", cause)
}
func observeGovernedModelTurnV2(boundary modelinvoker.GovernedModelTurnOutcomeV2, result InvokeResult, prepared modelinvoker.PreparedModelInvocationRefV1, material modelinvoker.InvocationMaterialV1, now time.Time) (modelinvoker.GovernedModelTurnObservationV2, error) {
	if now.IsZero() || !now.Before(time.Unix(0, boundary.ExpiresUnixNano)) {
		return modelinvoker.GovernedModelTurnObservationV2{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "observe_turn_v2", "observation crossed TTL", nil)
	}
	response := result.Response
	if response.Status != modelinvoker.ResponseStatusCompleted || strings.TrimSpace(response.ID) == "" || strings.TrimSpace(response.Model) == "" {
		return modelinvoker.GovernedModelTurnObservationV2{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "observe_turn_v2", "provider response is not completed", nil)
	}
	routeDigest, err := modelinvoker.DigestGovernedRouteSelectionV1(result.Resolution.Route)
	if err != nil || routeDigest != material.RouteDigest {
		return modelinvoker.GovernedModelTurnObservationV2{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "observe_turn_v2", "provider route selection drifted after the physical boundary", err)
	}
	if response.Model != material.Call.Request.Model {
		return modelinvoker.GovernedModelTurnObservationV2{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "observe_turn_v2", "provider response model differs from the frozen invocation material", nil)
	}
	observation := modelinvoker.GovernedModelTurnObservationV2{TurnRef: boundary.RefV2(), RouteSelectionDigest: routeDigest, Provider: response.Provider, Protocol: response.Protocol, ResponseID: response.ID, Model: response.Model, Status: response.Status, StopReason: response.StopReason, Usage: response.Usage, ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: boundary.ExpiresUnixNano}
	calls := response.FunctionCalls()
	switch response.StopReason {
	case modelinvoker.StopReasonEndTurn:
		if len(calls) != 0 || strings.TrimSpace(response.Text()) == "" {
			return modelinvoker.GovernedModelTurnObservationV2{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "observe_turn_v2", "completed_text response is invalid", nil)
		}
		observation.OutcomeKind = modelinvoker.GovernedModelTurnCompletedTextV2
		observation.CompletedText = response.Text()
	case modelinvoker.StopReasonToolCall:
		if len(calls) != 1 || strings.TrimSpace(response.Text()) != "" {
			return modelinvoker.GovernedModelTurnObservationV2{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "observe_turn_v2", "tool_call response must contain exactly one call and no text", nil)
		}
		allowed := false
		for _, tool := range material.Call.Request.Tools {
			if tool.Name == calls[0].Name {
				allowed = true
				break
			}
		}
		if !allowed || (material.Call.Request.ToolChoice.Mode == modelinvoker.ToolChoiceFunction && material.Call.Request.ToolChoice.Name != calls[0].Name) {
			return modelinvoker.GovernedModelTurnObservationV2{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "observe_turn_v2", "provider returned an unauthorized tool call", nil)
		}
		candidate, err := modelinvoker.FinalizeToolCallCandidateObservationV1(prepared.InvocationDigest, response)
		if err != nil {
			return modelinvoker.GovernedModelTurnObservationV2{}, err
		}
		projection, err := modelinvoker.NewToolCallCandidateObservationProjectionV1(prepared.InvocationID, boundary.DispatchSequence, response.ID, candidate)
		if err != nil {
			return modelinvoker.GovernedModelTurnObservationV2{}, err
		}
		observation.OutcomeKind = modelinvoker.GovernedModelTurnToolCallCandidateV2
		observation.ToolCallProjection = &projection
	default:
		return modelinvoker.GovernedModelTurnObservationV2{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "observe_turn_v2", "unsupported provider stop reason", nil)
	}
	return modelinvoker.SealGovernedModelTurnObservationV2(observation)
}

func deterministicBeforeProviderBoundaryV2(err error) bool {
	var providerErr *modelinvoker.Error
	if errors.As(err, &providerErr) && providerErr != nil {
		switch providerErr.Kind {
		case modelinvoker.ErrorInvalidRequest, modelinvoker.ErrorUnsupportedCapability, modelinvoker.ErrorPolicyRejected, modelinvoker.ErrorPermission, modelinvoker.ErrorAuthentication, modelinvoker.ErrorUnknownProvider, modelinvoker.ErrorDuplicateProvider, modelinvoker.ErrorMapping:
			return true
		}
		return false
	}
	return core.HasCategory(err, core.ErrorInvalidArgument) || core.HasCategory(err, core.ErrorConflict) || core.HasCategory(err, core.ErrorNotFound)
}
func turnResultForStateV2(outcome modelinvoker.GovernedModelTurnOutcomeV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	switch outcome.State {
	case modelinvoker.GovernedModelTurnObservedV2:
		return outcome, nil
	case modelinvoker.GovernedModelTurnRejectedNoEffectV2:
		return outcome, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_turn_v2", "turn was rejected before provider execution", nil)
	default:
		return outcome, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "inspect_turn_v2", "provider outcome is inspect-only", nil)
	}
}

var _ modelinvoker.GovernedModelTurnPortV2 = (*Gateway)(nil)
