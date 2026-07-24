package routegateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

type GovernedModelInvocationDependenciesV1 struct {
	PreparedHistory modelinvoker.PreparedModelInvocationReaderV1
	PreparedCurrent modelinvoker.PreparedModelInvocationCurrentReaderV1
	CommitGate      modelinvoker.PreparedModelInvocationCommitGateV1
	Invocations     modelinvoker.GovernedModelInvocationRepositoryV1
}

func (d GovernedModelInvocationDependenciesV1) validate() error {
	if nilInterface(d.PreparedHistory) || nilInterface(d.PreparedCurrent) || nilInterface(d.CommitGate) || nilInterface(d.Invocations) {
		return gatewayError(modelinvoker.ErrorInvalidRequest, "governed_dependencies_required", "governed Model invocation requires exact Prepared history/current, CommitGate and invocation repository", nil)
	}
	return nil
}

// StartOrInspectGovernedModelInvocationV1 is the only RouteGateway method that
// may claim a governed provider call. A successful provider-boundary CAS grants
// call rights to exactly that CAS winner. Replays and unknown outcomes inspect
// immutable history/current state and never call the provider again.
func (g *Gateway) StartOrInspectGovernedModelInvocationV1(ctx context.Context, command modelinvoker.GovernedModelInvocationCommandV1) (modelinvoker.GovernedModelInvocationResultV1, error) {
	if g == nil || g.now == nil || g.governedV1 == nil {
		return modelinvoker.GovernedModelInvocationResultV1{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorUnavailable, "start", "governed invocation capability is unavailable", nil)
	}
	if ctx == nil {
		return modelinvoker.GovernedModelInvocationResultV1{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "start", "context is nil", nil)
	}
	if err := ctx.Err(); err != nil {
		return modelinvoker.GovernedModelInvocationResultV1{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "start", "context ended before execution", err)
	}
	baseline := g.now()
	if baseline.IsZero() {
		return modelinvoker.GovernedModelInvocationResultV1{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "start", "clock returned zero", nil)
	}
	routeDigest, err := modelinvoker.DigestGovernedRouteCallV1(command.Call)
	if err != nil {
		return modelinvoker.GovernedModelInvocationResultV1{}, err
	}
	preparedFact, err := modelinvoker.NewPreparedGovernedModelInvocationForGatewayV1(command, routeDigest, baseline)
	if err != nil {
		return modelinvoker.GovernedModelInvocationResultV1{}, err
	}
	created, err := g.governedV1.Invocations.CreateGovernedModelInvocationV1(ctx, preparedFact)
	if err != nil {
		if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate {
			return modelinvoker.GovernedModelInvocationResultV1{}, err
		}
		recovered, inspectErr := g.governedV1.Invocations.InspectExactGovernedModelInvocationV1(context.WithoutCancel(ctx), preparedFact.RefV1())
		if inspectErr != nil {
			return modelinvoker.GovernedModelInvocationResultV1{}, errors.Join(err, inspectErr)
		}
		created = modelinvoker.GovernedModelInvocationMutationV1{Fact: recovered, Applied: false}
		if ctx.Err() != nil {
			return resultFromGovernedFactV1(recovered), err
		}
	}
	currentAttempt := created.Fact
	if currentAttempt.State != modelinvoker.GovernedModelInvocationPreparedV1 {
		return resultForCurrentGovernedStateV1(currentAttempt)
	}

	// S1 exact reads precede route/provider preparation and the Commit Gate.
	historicalS1, currentS1, err := g.readPreparedCurrentV1(ctx, command)
	if err != nil {
		return resultFromGovernedFactV1(currentAttempt), err
	}
	ack, err := modelinvoker.CrossPreparedModelInvocationCommitGateV1(ctx, g.governedV1.CommitGate, command.PreparedRef, command.CurrentRef)
	if err != nil {
		return resultFromGovernedFactV1(currentAttempt), closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "commit_gate", "Prepared Commit Gate did not produce an exact current ACK", err)
	}

	prepareTime := g.now()
	if prepareTime.IsZero() || prepareTime.Before(baseline) {
		return resultFromGovernedFactV1(currentAttempt), governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "prepare_provider", "clock regressed before provider preparation", nil)
	}
	providerCall, err := g.prepareAt(ctx, command.Call, prepareTime)
	if err != nil {
		return resultFromGovernedFactV1(currentAttempt), closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorUnavailable, "prepare_provider", "provider preparation failed before the boundary", err)
	}
	releaseProvider := true
	defer func() {
		if releaseProvider {
			_ = providerCall.lease.release()
		}
	}()

	// S2 exact reads occur after route/provider preparation and immediately
	// before sealing the dispatch receipt and crossing the repository boundary.
	historicalS2, currentS2, err := g.readPreparedCurrentV1(ctx, command)
	if err != nil {
		return resultFromGovernedFactV1(currentAttempt), err
	}
	ackS2, err := g.governedV1.CommitGate.InspectExactAck(ctx, ack.Ref())
	if err != nil {
		return resultFromGovernedFactV1(currentAttempt), closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "inspect_ack", "exact Commit ACK reread failed", err)
	}
	if historicalS1.Ref() != historicalS2.Ref() || historicalS1.Digest != historicalS2.Digest || currentS1.Ref() != currentS2.Ref() || currentS1.Digest != currentS2.Digest || ackS2.Ref() != ack.Ref() || ackS2.Digest != ack.Digest {
		return resultFromGovernedFactV1(currentAttempt), governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "actual_point", "Prepared history/current or Commit ACK drifted across S1/S2", nil)
	}
	fresh := g.now()
	if fresh.IsZero() || fresh.Before(prepareTime) {
		return resultFromGovernedFactV1(currentAttempt), governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "actual_point", "clock regressed across governed validation", nil)
	}
	if err := currentS2.ValidateCurrent(command.CurrentRef, fresh); err != nil {
		return resultFromGovernedFactV1(currentAttempt), closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "actual_point", "Prepared current projection is no longer current", err)
	}
	if err := ackS2.ValidateCurrent(currentS2, fresh); err != nil {
		return resultFromGovernedFactV1(currentAttempt), closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "actual_point", "Commit ACK is no longer current", err)
	}
	receipt, err := modelinvoker.SealPreparedModelInvocationDispatchReceiptAgainstV1(historicalS2, currentS2, ackS2, modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1{
		PreparedRef: command.PreparedRef, CurrentRef: command.CurrentRef, AckRef: ackS2.Ref(),
		DispatchSequence: command.DispatchSequence, BoundaryKind: modelinvoker.GovernedModelProviderBoundaryKindV1,
		ProviderAttemptOrdinal: command.ProviderAttemptOrdinal, AttemptRequestDigest: routeDigest,
		ActualToolSurfaceDigest: historicalS2.ActualToolSurfaceDigest, ActualProviderInjectionDigest: historicalS2.ActualProviderInjectionDigest,
		CheckedUnixNano: fresh.UnixNano(),
	}, fresh)
	if err != nil {
		return resultFromGovernedFactV1(currentAttempt), closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "dispatch_receipt", "dispatch validation receipt could not be sealed", err)
	}
	expires := minTimeUnixNanoV1(currentAttempt.ExpiresUnixNano, currentS2.ExpiresUnixNano, ackS2.ExpiresUnixNano, ackS2.NotAfterUnixNano)
	boundary := currentAttempt.CloneV1()
	boundary.Revision = 2
	boundary.State = modelinvoker.GovernedModelInvocationProviderBoundaryCrossedV1
	boundary.UpdatedUnixNano = fresh.UnixNano()
	boundary.ExpiresUnixNano = expires
	ackRef := ackS2.Ref()
	boundary.AckRef = &ackRef
	boundary.DispatchReceipt = &receipt
	boundary.Digest = ""
	boundary, err = modelinvoker.SealGovernedModelInvocationFactV1(boundary)
	if err != nil {
		return resultFromGovernedFactV1(currentAttempt), err
	}
	boundaryMutation, err := g.governedV1.Invocations.CompareAndSwapGovernedModelInvocationV1(ctx, modelinvoker.GovernedModelInvocationCASV1{Expected: currentAttempt.RefV1(), Next: boundary})
	if err != nil {
		if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) == modelinvoker.GovernedModelInvocationErrorIndeterminate {
			recovered, inspectErr := g.governedV1.Invocations.InspectExactGovernedModelInvocationV1(context.WithoutCancel(ctx), boundary.RefV1())
			if inspectErr == nil {
				return resultFromGovernedFactV1(recovered), err
			}
			return modelinvoker.GovernedModelInvocationResultV1{}, errors.Join(err, inspectErr)
		}
		return resultFromGovernedFactV1(currentAttempt), err
	}
	if !boundaryMutation.Applied {
		return resultForCurrentGovernedStateV1(boundaryMutation.Fact)
	}
	boundary = boundaryMutation.Fact

	// Successful boundary CAS means the provider may be called. A replay can no
	// longer obtain call rights. The physical point still rereads the exact
	// Prepared history/current and ACK with a fresh clock. Boundary ownership is
	// necessary but not sufficient: any drift, rollback or expiry leaves this
	// attempt inspect-only and calls no provider.
	physical := g.now()
	if physical.IsZero() || physical.Before(fresh) || !physical.Before(time.Unix(0, expires)) {
		terminal, terminalErr := g.finishGovernedWithoutObservationV1(ctx, boundary, modelinvoker.GovernedModelInvocationRejectedNoEffectV1, "physical_point_not_current", physical)
		if terminalErr != nil {
			return resultFromGovernedFactV1(boundary), terminalErr
		}
		return resultFromGovernedFactV1(terminal), governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "physical_point", "clock or TTL rejected provider execution", nil)
	}
	historicalPhysical, currentPhysical, physicalReadErr := g.readPreparedCurrentV1(ctx, command)
	if physicalReadErr != nil {
		terminal, terminalErr := g.finishGovernedWithoutObservationV1(context.WithoutCancel(ctx), boundary, modelinvoker.GovernedModelInvocationRejectedNoEffectV1, "physical_point_owner_read_failed", physical)
		if terminalErr != nil {
			return resultFromGovernedFactV1(boundary), errors.Join(physicalReadErr, terminalErr)
		}
		return resultFromGovernedFactV1(terminal), physicalReadErr
	}
	ackPhysical, physicalAckErr := g.governedV1.CommitGate.InspectExactAck(ctx, ackS2.Ref())
	if physicalAckErr != nil {
		closedErr := closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "physical_ack", "exact Commit ACK physical-point reread failed", physicalAckErr)
		terminal, terminalErr := g.finishGovernedWithoutObservationV1(context.WithoutCancel(ctx), boundary, modelinvoker.GovernedModelInvocationRejectedNoEffectV1, "physical_point_ack_read_failed", physical)
		if terminalErr != nil {
			return resultFromGovernedFactV1(boundary), errors.Join(closedErr, terminalErr)
		}
		return resultFromGovernedFactV1(terminal), closedErr
	}
	if historicalPhysical.Ref() != historicalS2.Ref() || historicalPhysical.Digest != historicalS2.Digest || currentPhysical.Ref() != currentS2.Ref() || currentPhysical.Digest != currentS2.Digest || ackPhysical.Ref() != ackS2.Ref() || ackPhysical.Digest != ackS2.Digest {
		terminal, terminalErr := g.finishGovernedWithoutObservationV1(ctx, boundary, modelinvoker.GovernedModelInvocationRejectedNoEffectV1, "physical_point_owner_drift", physical)
		if terminalErr != nil {
			return resultFromGovernedFactV1(boundary), terminalErr
		}
		return resultFromGovernedFactV1(terminal), governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "physical_point", "Prepared history/current or Commit ACK drifted at provider execution", nil)
	}
	actual := g.now()
	if actual.IsZero() || actual.Before(physical) || !actual.Before(time.Unix(0, expires)) {
		terminal, terminalErr := g.finishGovernedWithoutObservationV1(ctx, boundary, modelinvoker.GovernedModelInvocationRejectedNoEffectV1, "actual_provider_point_not_current", actual)
		if terminalErr != nil {
			return resultFromGovernedFactV1(boundary), terminalErr
		}
		return resultFromGovernedFactV1(terminal), governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "actual_provider_point", "clock or TTL changed during provider-point rereads", nil)
	}
	if err := currentPhysical.ValidateCurrent(command.CurrentRef, actual); err != nil {
		terminal, terminalErr := g.finishGovernedWithoutObservationV1(ctx, boundary, modelinvoker.GovernedModelInvocationRejectedNoEffectV1, "physical_point_current_expired", actual)
		if terminalErr != nil {
			return resultFromGovernedFactV1(boundary), terminalErr
		}
		return resultFromGovernedFactV1(terminal), closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "physical_point", "Prepared current projection expired at provider execution", err)
	}
	if err := ackPhysical.ValidateCurrent(currentPhysical, actual); err != nil {
		terminal, terminalErr := g.finishGovernedWithoutObservationV1(ctx, boundary, modelinvoker.GovernedModelInvocationRejectedNoEffectV1, "physical_point_ack_expired", actual)
		if terminalErr != nil {
			return resultFromGovernedFactV1(boundary), terminalErr
		}
		return resultFromGovernedFactV1(terminal), closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "physical_point", "Commit ACK expired at provider execution", err)
	}
	invokeResult, invokeErr := g.invokePrepared(ctx, providerCall)
	releaseProvider = false
	if invokeErr != nil {
		terminal, terminalErr := g.finishGovernedWithoutObservationV1(context.WithoutCancel(ctx), boundary, modelinvoker.GovernedModelInvocationUnknownV1, "provider_outcome_unknown", g.now())
		if terminalErr != nil {
			return resultFromGovernedFactV1(boundary), errors.Join(invokeErr, terminalErr)
		}
		return resultFromGovernedFactV1(terminal), closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "provider", "provider outcome is unknown", invokeErr)
	}
	observation, err := observationFromResponseV1(boundary, invokeResult, command.Call.Request.Output.Schema, actual, g.now())
	if err != nil {
		terminal, terminalErr := g.finishGovernedWithoutObservationV1(context.WithoutCancel(ctx), boundary, modelinvoker.GovernedModelInvocationUnknownV1, "provider_observation_invalid", g.now())
		if terminalErr != nil {
			return resultFromGovernedFactV1(boundary), errors.Join(err, terminalErr)
		}
		return resultFromGovernedFactV1(terminal), err
	}
	observed := boundary.CloneV1()
	observed.Revision = 3
	observed.State = modelinvoker.GovernedModelInvocationObservedV1
	observed.UpdatedUnixNano = observation.ObservedUnixNano
	observed.Observation = &observation
	observed.Digest = ""
	observed, err = modelinvoker.SealGovernedModelInvocationFactV1(observed)
	if err != nil {
		return resultFromGovernedFactV1(boundary), err
	}
	mutation, err := g.governedV1.Invocations.CompareAndSwapGovernedModelInvocationV1(ctx, modelinvoker.GovernedModelInvocationCASV1{Expected: boundary.RefV1(), Next: observed})
	if err != nil {
		if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) == modelinvoker.GovernedModelInvocationErrorIndeterminate {
			recovered, inspectErr := g.governedV1.Invocations.InspectExactGovernedModelInvocationV1(context.WithoutCancel(ctx), observed.RefV1())
			if inspectErr == nil {
				return resultFromGovernedFactV1(recovered), nil
			}
			return resultFromGovernedFactV1(boundary), errors.Join(err, inspectErr)
		}
		return resultFromGovernedFactV1(boundary), err
	}
	return resultFromGovernedFactV1(mutation.Fact), nil
}

func (g *Gateway) InspectExactModelInvocationV1(ctx context.Context, ref modelinvoker.GovernedModelInvocationRefV1) (modelinvoker.GovernedModelInvocationResultV1, error) {
	if g == nil || g.governedV1 == nil || nilInterface(g.governedV1.Invocations) {
		return modelinvoker.GovernedModelInvocationResultV1{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorUnavailable, "inspect_exact", "governed invocation repository is unavailable", nil)
	}
	fact, err := g.governedV1.Invocations.InspectExactGovernedModelInvocationV1(ctx, ref)
	if err != nil {
		return modelinvoker.GovernedModelInvocationResultV1{}, err
	}
	return resultFromGovernedFactV1(fact), nil
}

func (g *Gateway) readPreparedCurrentV1(ctx context.Context, command modelinvoker.GovernedModelInvocationCommandV1) (modelinvoker.PreparedModelInvocationFactV1, modelinvoker.PreparedModelInvocationCurrentProjectionV1, error) {
	historical, err := g.governedV1.PreparedHistory.InspectExactPreparedModelInvocationV1(ctx, command.PreparedRef)
	if err != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorUnavailable, "prepared_history", "exact Prepared history reread failed", err)
	}
	current, err := g.governedV1.PreparedCurrent.InspectExactPreparedModelInvocationCurrentV1(ctx, command.CurrentRef)
	if err != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorUnavailable, "prepared_current", "exact Prepared current reread failed", err)
	}
	if historical.Ref() != command.PreparedRef || current.Ref() != command.CurrentRef || current.ValidateAgainstFact(historical) != nil || historical.UnifiedRequestDigest != command.AttemptRequestDigest {
		return modelinvoker.PreparedModelInvocationFactV1{}, modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "prepared_read", "Prepared history/current/request exact lineage drifted", nil)
	}
	return historical.Clone(), current.Clone(), nil
}

func (g *Gateway) finishGovernedWithoutObservationV1(ctx context.Context, boundary modelinvoker.GovernedModelInvocationFactV1, state modelinvoker.GovernedModelInvocationStateV1, code string, now time.Time) (modelinvoker.GovernedModelInvocationFactV1, error) {
	terminal := boundary.CloneV1()
	terminal.Revision = 3
	terminal.State = state
	terminal.FailureCode = code
	if now.IsZero() || now.UnixNano() < boundary.UpdatedUnixNano {
		terminal.UpdatedUnixNano = boundary.UpdatedUnixNano
	} else {
		terminal.UpdatedUnixNano = now.UnixNano()
	}
	terminal.Digest = ""
	sealed, err := modelinvoker.SealGovernedModelInvocationFactV1(terminal)
	if err != nil {
		return modelinvoker.GovernedModelInvocationFactV1{}, err
	}
	mutation, err := g.governedV1.Invocations.CompareAndSwapGovernedModelInvocationV1(ctx, modelinvoker.GovernedModelInvocationCASV1{Expected: boundary.RefV1(), Next: sealed})
	if err != nil {
		if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) == modelinvoker.GovernedModelInvocationErrorIndeterminate {
			recovered, inspectErr := g.governedV1.Invocations.InspectExactGovernedModelInvocationV1(context.WithoutCancel(ctx), sealed.RefV1())
			if inspectErr == nil {
				return recovered, nil
			}
			return modelinvoker.GovernedModelInvocationFactV1{}, errors.Join(err, inspectErr)
		}
		return modelinvoker.GovernedModelInvocationFactV1{}, err
	}
	return mutation.Fact, nil
}

func observationFromResponseV1(boundary modelinvoker.GovernedModelInvocationFactV1, result InvokeResult, schemaDocument json.RawMessage, actualProviderPoint, now time.Time) (modelinvoker.GovernedModelInvocationObservationV1, error) {
	if actualProviderPoint.IsZero() || now.IsZero() || now.Before(actualProviderPoint) || now.UnixNano() < boundary.UpdatedUnixNano || !now.Before(time.Unix(0, boundary.ExpiresUnixNano)) {
		return modelinvoker.GovernedModelInvocationObservationV1{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "observe", "provider response crossed the governed TTL or clock regressed", nil)
	}
	response := result.Response
	if response.Status != modelinvoker.ResponseStatusCompleted || response.StopReason != modelinvoker.StopReasonEndTurn || strings.TrimSpace(response.ID) == "" || strings.TrimSpace(response.Model) == "" || len(response.Output) != 1 || response.Output[0].Type != modelinvoker.OutputItemText || strings.TrimSpace(response.Output[0].Text) == "" || response.Output[0].FunctionCall != nil || response.Output[0].ReasoningSummary != "" || len(response.FunctionCalls()) != 0 {
		return modelinvoker.GovernedModelInvocationObservationV1{}, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "observe", "provider response is not one completed tool-free result", nil)
	}
	if err := modelinvoker.ValidateGovernedStructuredOutputV1(schemaDocument, []byte(response.Output[0].Text)); err != nil {
		return modelinvoker.GovernedModelInvocationObservationV1{}, closeGovernedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "observe", "provider structured output does not satisfy the governed schema", err)
	}
	routeDigest, err := modelinvoker.DigestGovernedRouteSelectionV1(result.Resolution.Route)
	if err != nil {
		return modelinvoker.GovernedModelInvocationObservationV1{}, err
	}
	return modelinvoker.SealGovernedModelInvocationObservationV1(modelinvoker.GovernedModelInvocationObservationV1{
		InvocationRef: boundary.RefV1(), RouteID: result.Resolution.Route.RouteID, RouteSelectionDigest: routeDigest,
		Provider: response.Provider, Protocol: response.Protocol,
		ResponseID: response.ID, Model: response.Model, Status: response.Status, StopReason: response.StopReason,
		StructuredOutput: []byte(response.Text()), Usage: response.Usage, ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: boundary.ExpiresUnixNano,
	})
}

func resultFromGovernedFactV1(fact modelinvoker.GovernedModelInvocationFactV1) modelinvoker.GovernedModelInvocationResultV1 {
	clone := fact.CloneV1()
	result := modelinvoker.GovernedModelInvocationResultV1{Invocation: clone}
	if clone.Observation != nil {
		observation := clone.Observation.CloneV1()
		result.Observation = &observation
	}
	return result
}

func resultForCurrentGovernedStateV1(fact modelinvoker.GovernedModelInvocationFactV1) (modelinvoker.GovernedModelInvocationResultV1, error) {
	result := resultFromGovernedFactV1(fact)
	switch fact.State {
	case modelinvoker.GovernedModelInvocationObservedV1:
		return result, nil
	case modelinvoker.GovernedModelInvocationRejectedNoEffectV1:
		return result, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_current", "governed invocation was rejected before provider execution", nil)
	default:
		return result, governedGatewayErrorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "inspect_current", "provider outcome is not an accepted Observation; exact Inspect only", nil)
	}
}

func governedGatewayErrorV1(kind modelinvoker.GovernedModelInvocationErrorKindV1, operation, message string, err error) error {
	return &modelinvoker.GovernedModelInvocationErrorV1{Kind: kind, Operation: operation, Message: message, Err: err}
}

func closeGovernedGatewayErrorV1(kind modelinvoker.GovernedModelInvocationErrorKindV1, operation, message string, err error) error {
	if err == nil {
		return nil
	}
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != "" {
		return err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		kind = modelinvoker.GovernedModelInvocationErrorIndeterminate
	}
	return governedGatewayErrorV1(kind, operation, message, err)
}

func minTimeUnixNanoV1(values ...int64) int64 {
	result := int64(0)
	for _, value := range values {
		if value > 0 && (result == 0 || value < result) {
			result = value
		}
	}
	return result
}

var _ modelinvoker.GovernedModelInvocationPortV1 = (*Gateway)(nil)
