package routegateway

import (
	"context"
	"errors"
	"reflect"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// GovernedModelTurnActualBoundaryDependenciesV3 is deliberately closed over
// public, read-only Owner ports plus one composite Boundary store.
// Missing authoritative Context/Tool pair readers make Authorizer construction
// fail, so this path cannot silently fall back to stored RouteCall bytes.
type GovernedModelTurnProviderBoundaryStoreV3 interface {
	modelinvoker.GovernedModelTurnProviderBoundaryRepositoryV3
	modelinvoker.GovernedModelTurnProviderBoundaryTurnAttemptReaderV3
}

type GovernedModelTurnActualBoundaryDependenciesV3 struct {
	preparedHistory modelinvoker.PreparedModelInvocationReaderV1
	preparedCurrent modelinvoker.PreparedModelInvocationCurrentReaderV1
	commitGate      modelinvoker.PreparedModelInvocationCommitGateV1
	materials       modelinvoker.InvocationMaterialReaderV2
	authorizer      *modelinvoker.InvocationMaterialAuthorizerV2
	turns           modelinvoker.GovernedModelTurnRepositoryV3
	boundaries      GovernedModelTurnProviderBoundaryStoreV3
}

func NewGovernedModelTurnActualBoundaryDependenciesV3(
	preparedHistory modelinvoker.PreparedModelInvocationReaderV1,
	preparedCurrent modelinvoker.PreparedModelInvocationCurrentReaderV1,
	commitGate modelinvoker.PreparedModelInvocationCommitGateV1,
	materials modelinvoker.InvocationMaterialReaderV2,
	authorizer *modelinvoker.InvocationMaterialAuthorizerV2,
	turns modelinvoker.GovernedModelTurnRepositoryV3,
	boundaries GovernedModelTurnProviderBoundaryStoreV3,
) (GovernedModelTurnActualBoundaryDependenciesV3, error) {
	dependencies := GovernedModelTurnActualBoundaryDependenciesV3{
		preparedHistory: preparedHistory,
		preparedCurrent: preparedCurrent,
		commitGate:      commitGate,
		materials:       materials,
		authorizer:      authorizer,
		turns:           turns,
		boundaries:      boundaries,
	}
	if err := dependencies.validate(); err != nil {
		return GovernedModelTurnActualBoundaryDependenciesV3{}, err
	}
	return dependencies, nil
}

func (d GovernedModelTurnActualBoundaryDependenciesV3) validate() error {
	for _, dependency := range []any{
		d.preparedHistory,
		d.preparedCurrent,
		d.commitGate,
		d.materials,
		d.authorizer,
		d.turns,
		d.boundaries,
	} {
		if nilInterface(dependency) {
			return gatewayError(
				modelinvoker.ErrorInvalidRequest,
				"governed_turn_v3_actual_boundary_dependencies_required",
				"governed model turn V3 actual boundary requires every exact Owner reader",
				nil,
			)
		}
	}
	return nil
}

// GovernedModelTurnActualBoundaryCommandV3 starts from an already persisted,
// exact V3 Turn winner. RuntimeRequest may omit ModelBoundary on its first
// call; the Model boundary builder derives and seals it.
type GovernedModelTurnActualBoundaryCommandV3 struct {
	TurnRef        modelinvoker.GovernedModelTurnRefV3
	RuntimeRequest runtimeports.InspectCurrentModelProviderActualPointRequestV1
}

// GovernedModelTurnActualBoundaryResultV3 is an in-process pre-invoke result.
// Until exact credential current, prepared injection, and durable Provider
// attempt contracts exist, Boundary and Invoke remain zero.
type GovernedModelTurnActualBoundaryResultV3 struct {
	Turn     modelinvoker.GovernedModelTurnOutcomeV3
	Boundary modelinvoker.GovernedModelTurnProviderBoundaryPersistenceResultV3
	Invoke   InvokeResult
}

// InvokeGovernedModelTurnActualBoundaryV3 currently proves only the reversible
// pre-invoke coordinator. It replays exact Turn/Context/Tool/ACK inputs and
// prepares an adapter, then deterministically fails closed before writing a
// Boundary or invoking a Provider. This is not a production dispatch port.
func (g *Gateway) InvokeGovernedModelTurnActualBoundaryV3(
	ctx context.Context,
	command GovernedModelTurnActualBoundaryCommandV3,
) (
	result GovernedModelTurnActualBoundaryResultV3,
	err error,
) {
	if g == nil || g.now == nil || g.governedTurnActualBoundaryV3 == nil {
		return result, governedGatewayErrorV1(
			modelinvoker.GovernedModelInvocationErrorUnavailable,
			"turn_v3_actual_boundary",
			"governed model turn V3 actual boundary is unavailable",
			nil,
		)
	}
	if ctx == nil || ctx.Err() != nil || command.TurnRef.Validate() != nil {
		return result, governedGatewayErrorV1(
			modelinvoker.GovernedModelInvocationErrorInvalid,
			"turn_v3_actual_boundary",
			"live context and exact V3 Turn Ref are required",
			nil,
		)
	}
	dependencies := g.governedTurnActualBoundaryV3

	turn, err := dependencies.turns.InspectExactGovernedModelTurnV3(ctx, command.TurnRef)
	if err != nil {
		return result, err
	}
	if turn.Validate() != nil || turn.RefV3() != command.TurnRef {
		return result, governedGatewayErrorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"turn_v3_exact",
			"V3 Turn exact reader returned invalid or drifted content",
			nil,
		)
	}
	result.Turn = turn.CloneV3()

	// Inspect the original Turn attempt before crossing Commit or resolving any
	// Provider credential/factory. A replay never re-prepares a Provider, even
	// when its caller only retained the original command without ModelBoundary.
	existing, inspectErr :=
		dependencies.boundaries.InspectGovernedModelTurnProviderBoundaryTurnAttemptV3(
			ctx,
			turn.AttemptRefV3(),
		)
	if inspectErr == nil {
		expectedRequest := command.RuntimeRequest
		if reflect.DeepEqual(
			expectedRequest.ModelBoundary,
			runtimeports.ModelProviderBoundaryCurrentRefV1{},
		) {
			expectedRequest.ModelBoundary = existing.RuntimeRequest.ModelBoundary
		} else if expectedRequest.ModelBoundary.Validate() != nil ||
			!runtimeports.SameModelProviderBoundaryCurrentRefV1(
				expectedRequest.ModelBoundary,
				existing.RuntimeRequest.ModelBoundary,
			) {
			return result, governedGatewayErrorV1(
				modelinvoker.GovernedModelInvocationErrorConflict,
				"turn_v3_boundary_existing",
				"caller V3 provider boundary differs from the immutable winner",
				nil,
			)
		}
		if existing.Validate() != nil ||
			existing.TurnRef != turn.RefV3() ||
			!reflect.DeepEqual(existing.RuntimeRequest, expectedRequest) {
			return result, governedGatewayErrorV1(
				modelinvoker.GovernedModelInvocationErrorConflict,
				"turn_v3_boundary_existing",
				"existing V3 provider boundary belongs to another exact request",
				nil,
			)
		}
		result.Boundary = modelinvoker.GovernedModelTurnProviderBoundaryPersistenceResultV3{
			Fact:        existing,
			Disposition: modelinvoker.GovernedModelTurnProviderBoundaryPersistenceExistingV3,
		}
		return result, governedGatewayErrorV1(
			modelinvoker.GovernedModelInvocationErrorIndeterminate,
			"turn_v3_boundary_existing",
			"original V3 provider boundary already exists; Provider replay is forbidden",
			nil,
		)
	}
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(inspectErr) !=
		modelinvoker.GovernedModelInvocationErrorNotFound {
		return result, inspectErr
	}
	if err := modelinvoker.ValidateModelProviderActualPointRequestDraftV3(
		command.RuntimeRequest,
	); err != nil {
		return result, err
	}

	s1 := g.now()
	prepared1, current1, material1, err :=
		g.readGovernedModelTurnInputsV3(ctx, turn, s1)
	if err != nil {
		return result, err
	}
	authorization1, err :=
		modelinvoker.InspectCurrentInvocationMaterialAuthorizationClosureV3(
			ctx,
			dependencies.authorizer,
			prepared1,
			current1,
			material1,
			s1,
		)
	if err != nil {
		return result, err
	}
	ack1, err := modelinvoker.CrossPreparedModelInvocationCommitGateV1(
		ctx,
		dependencies.commitGate,
		turn.PreparedRef,
		turn.CurrentRef,
	)
	if err != nil {
		return result, err
	}
	if ack1.PreparedRef != turn.PreparedRef ||
		ack1.CurrentRef != turn.CurrentRef ||
		ack1.ValidateCurrent(current1, s1) != nil {
		return result, governedGatewayErrorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"turn_v3_s1_ack",
			"V3 S1 Commit ACK is not exact and current",
			nil,
		)
	}

	s2 := g.now()
	if s2.IsZero() || s2.Before(s1) {
		return result, governedGatewayErrorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"turn_v3_s2_clock",
			"clock regressed before V3 S2",
			nil,
		)
	}
	prepared2, current2, material2, err :=
		g.readGovernedModelTurnInputsV3(ctx, turn, s2)
	if err != nil {
		return result, err
	}
	authorization2, err :=
		modelinvoker.InspectCurrentInvocationMaterialAuthorizationClosureV3(
			ctx,
			dependencies.authorizer,
			prepared2,
			current2,
			material2,
			s2,
		)
	if err != nil {
		return result, err
	}
	ack2, err := modelinvoker.InspectPreparedModelInvocationCommitAckV1(
		ctx,
		dependencies.commitGate,
		ack1.Ref(),
	)
	if err != nil {
		return result, err
	}
	if ack2.PreparedRef != turn.PreparedRef ||
		ack2.CurrentRef != turn.CurrentRef ||
		ack2.ValidateCurrent(current2, s2) != nil {
		return result, governedGatewayErrorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"turn_v3_s2_ack",
			"V3 S2 Commit ACK is not exact and current",
			nil,
		)
	}
	if prepared1.Ref() != prepared2.Ref() ||
		current1.Ref() != current2.Ref() ||
		material1.RefV2() != material2.RefV2() ||
		ack1 != ack2 ||
		!sameInvocationMaterialAuthorizationClosureV3(
			authorization1,
			authorization2,
		) {
		return result, governedGatewayErrorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"turn_v3_s2",
			"V3 exact inputs drifted between S1 and S2",
			nil,
		)
	}

	prepareAt := g.now()
	if prepareAt.IsZero() || prepareAt.Before(s2) {
		return result, governedGatewayErrorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"turn_v3_provider_prepare_clock",
			"clock regressed before reversible Provider preparation",
			nil,
		)
	}
	preparedProvider, err := g.prepareAt(ctx, material2.Call, prepareAt)
	if err != nil {
		return result, err
	}
	defer func() {
		err = retireAndJoinGovernedModelTurnV3ProviderReleaseError(
			err,
			preparedProvider.lease,
		)
	}()
	actualRouteDigest, err :=
		modelinvoker.DigestGovernedRouteSelectionV1(preparedProvider.resolution.Route)
	if err != nil || actualRouteDigest != material2.RouteDigest ||
		preparedProvider.request.Model != material2.Call.Request.Model {
		return result, governedGatewayErrorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"turn_v3_provider_prepare",
			"prepared Provider route differs from exact invocation material",
			err,
		)
	}
	return result, governedGatewayErrorV1(
		modelinvoker.GovernedModelInvocationErrorUnavailable,
		"turn_v3_credential_current_contract_required",
		"exact credential current proof is unavailable; V3 Provider invoke is disabled",
		nil,
	)
}

func retireAndJoinGovernedModelTurnV3ProviderReleaseError(
	primary error,
	lease *adapterLease,
) error {
	if lease == nil {
		return primary
	}
	releaseErr := lease.retire()
	if releaseErr == nil {
		return primary
	}
	return errors.Join(
		primary,
		gatewayError(
			modelinvoker.ErrorProviderUnavailable,
			"turn_v3_provider_release_failed",
			"reversible V3 Provider adapter release failed",
			releaseErr,
		),
	)
}

func (g *Gateway) readGovernedModelTurnInputsV3(
	ctx context.Context,
	turn modelinvoker.GovernedModelTurnOutcomeV3,
	now time.Time,
) (
	modelinvoker.PreparedModelInvocationFactV1,
	modelinvoker.PreparedModelInvocationCurrentProjectionV1,
	modelinvoker.InvocationMaterialV2,
	error,
) {
	var historical modelinvoker.PreparedModelInvocationFactV1
	var current modelinvoker.PreparedModelInvocationCurrentProjectionV1
	var material modelinvoker.InvocationMaterialV2
	if now.IsZero() {
		return historical, current, material, governedGatewayErrorV1(
			modelinvoker.GovernedModelInvocationErrorInvalid,
			"turn_v3_read_inputs",
			"validation clock is zero",
			nil,
		)
	}
	dependencies := g.governedTurnActualBoundaryV3
	historical, err := dependencies.preparedHistory.InspectExactPreparedModelInvocationV1(
		ctx,
		turn.PreparedRef,
	)
	if err != nil {
		return historical, current, material, err
	}
	current, err = dependencies.preparedCurrent.InspectExactPreparedModelInvocationCurrentV1(
		ctx,
		turn.CurrentRef,
	)
	if err != nil {
		return historical, current, material, err
	}
	material, err = dependencies.materials.InspectExactInvocationMaterialV2(
		ctx,
		turn.MaterialRef,
	)
	if err != nil {
		return historical, current, material, err
	}
	if historical.Ref() != turn.PreparedRef ||
		current.Ref() != turn.CurrentRef ||
		material.RefV2() != turn.MaterialRef ||
		current.ValidateAgainstFact(historical) != nil ||
		current.ValidateCurrent(turn.CurrentRef, now) != nil ||
		material.ValidateAgainstPreparedV2(historical, current, now) != nil ||
		turn.AttemptRequestDigest != historical.UnifiedRequestDigest ||
		turn.RouteCallDigest != material.RouteCallDigest {
		return modelinvoker.PreparedModelInvocationFactV1{},
			modelinvoker.PreparedModelInvocationCurrentProjectionV1{},
			modelinvoker.InvocationMaterialV2{},
			governedGatewayErrorV1(
				modelinvoker.GovernedModelInvocationErrorConflict,
				"turn_v3_read_inputs",
				"V3 exact durable inputs drifted",
				nil,
			)
	}
	return historical.Clone(), current.Clone(), material.CloneV2(), nil
}

func sameInvocationMaterialAuthorizationClosureV3(
	left modelinvoker.InvocationMaterialCurrentAuthorizationClosureV3,
	right modelinvoker.InvocationMaterialCurrentAuthorizationClosureV3,
) bool {
	leftAuthorization := left.Authorization
	rightAuthorization := right.Authorization
	return leftAuthorization.PreparedRef == rightAuthorization.PreparedRef &&
		leftAuthorization.CurrentRef == rightAuthorization.CurrentRef &&
		leftAuthorization.RouteCallDigest == rightAuthorization.RouteCallDigest &&
		reflect.DeepEqual(
			leftAuthorization.SourceLineage,
			rightAuthorization.SourceLineage,
		) &&
		leftAuthorization.ProviderInjectionRef ==
			rightAuthorization.ProviderInjectionRef &&
		leftAuthorization.RouteRef == rightAuthorization.RouteRef &&
		leftAuthorization.ProfileRef == rightAuthorization.ProfileRef &&
		leftAuthorization.ExpiresUnixNano ==
			rightAuthorization.ExpiresUnixNano &&
		reflect.DeepEqual(left.ContextPair, right.ContextPair) &&
		reflect.DeepEqual(left.ToolPair, right.ToolPair)
}
