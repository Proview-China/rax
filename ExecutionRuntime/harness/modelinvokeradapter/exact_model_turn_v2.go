package modelinvokeradapter

import (
	"context"
	"errors"
	"reflect"
	"time"

	bridgecontract "github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ExactModelTurnClosureReadersV2 struct {
	Materials   modelinvoker.InvocationMaterialReaderV2
	ContextPair modelinvoker.InvocationMaterialContextPairExactReaderV2
	ToolPair    modelinvoker.InvocationMaterialToolPairExactReaderV2
}

type ExactModelTurnAdapterConfigV2 struct {
	Readers    ExactModelTurnClosureReadersV2
	Dispatches harnessports.ModelTurnDispatchRepositoryV2
	Model      modelinvoker.GovernedModelTurnPortV3
	Clock      func() time.Time
}

// ExactModelTurnAdapterV2 is a Harness-owned owner-local bridge. It verifies
// four immutable Context/Tool source roles around the Model Owner call and
// persists only Harness dispatch references. It cannot call a Provider,
// execute a Tool, create PendingAction, or advance a Harness Turn.
type ExactModelTurnAdapterV2 struct {
	readers    ExactModelTurnClosureReadersV2
	dispatches harnessports.ModelTurnDispatchRepositoryV2
	model      modelinvoker.GovernedModelTurnPortV3
	clock      func() time.Time
}

func NewExactModelTurnAdapterV2(config ExactModelTurnAdapterConfigV2) (*ExactModelTurnAdapterV2, error) {
	for _, dependency := range []any{
		config.Readers.Materials,
		config.Readers.ContextPair,
		config.Readers.ToolPair,
		config.Dispatches,
		config.Model,
	} {
		if nilLikeExactModelTurnV2(dependency) {
			return nil, exactModelTurnErrorV2(core.ErrorInvalidArgument, core.ReasonComponentMissing, "exact model-turn V2 requires all owner readers, sidecar, and Model V3 port")
		}
	}
	if config.Clock == nil {
		return nil, exactModelTurnErrorV2(core.ErrorInvalidArgument, core.ReasonComponentMissing, "exact model-turn V2 clock is required")
	}
	return &ExactModelTurnAdapterV2{
		readers:    config.Readers,
		dispatches: config.Dispatches,
		model:      config.Model,
		clock:      config.Clock,
	}, nil
}

func (a *ExactModelTurnAdapterV2) StartOrInspectExactModelTurnV2(
	ctx context.Context,
	envelope bridgecontract.ModelTurnExactEnvelopeV2,
) (bridgecontract.ModelTurnDispatchFactV2, error) {
	if err := a.preflightV2(ctx, envelope); err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	ref, err := bridgecontract.DeriveModelTurnDispatchRefV2(envelope)
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	current, err := a.dispatches.InspectExactModelTurnDispatchV2(ctx, ref)
	if err != nil {
		if !core.HasCategory(err, core.ErrorNotFound) {
			return bridgecontract.ModelTurnDispatchFactV2{}, err
		}
		attempt, createErr := bridgecontract.NewModelTurnDispatchAttemptFactV2(ref)
		if createErr != nil {
			return bridgecontract.ModelTurnDispatchFactV2{}, createErr
		}
		current, err = a.dispatches.EnsureModelTurnDispatchAttemptV2(ctx, attempt)
		if err != nil {
			if !core.HasCategory(err, core.ErrorIndeterminate) &&
				!core.HasCategory(err, core.ErrorUnavailable) &&
				!core.HasCategory(err, core.ErrorConflict) {
				return bridgecontract.ModelTurnDispatchFactV2{}, err
			}
			recoveryContext, cancel := exactModelTurnRecoveryContextV2(
				ctx,
				ref.Envelope.RequestedNotAfterUnixNano,
			)
			recovered, inspectErr := a.dispatches.InspectExactModelTurnDispatchV2(recoveryContext, ref)
			cancel()
			if inspectErr != nil {
				return bridgecontract.ModelTurnDispatchFactV2{}, errors.Join(err, inspectErr)
			}
			current = recovered
		}
	}
	if err := current.Validate(); err != nil || current.Ref != ref {
		if err != nil {
			return bridgecontract.ModelTurnDispatchFactV2{}, err
		}
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorConflict, core.ReasonBindingDrift, "exact model-turn V2 sidecar returned another dispatch")
	}
	return a.resumeV2(ctx, current)
}

func (a *ExactModelTurnAdapterV2) InspectExactModelTurnV2(
	ctx context.Context,
	ref bridgecontract.ModelTurnDispatchRefV2,
) (bridgecontract.ModelTurnDispatchFactV2, error) {
	if a == nil || nilLikeExactModelTurnV2(a.dispatches) {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorUnavailable, core.ReasonComponentMissing, "exact model-turn V2 sidecar is unavailable")
	}
	if err := exactModelTurnContextV2(ctx); err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	fact, err := a.dispatches.InspectExactModelTurnDispatchV2(ctx, ref)
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	if err := fact.Validate(); err != nil || fact.Ref != ref {
		if err != nil {
			return bridgecontract.ModelTurnDispatchFactV2{}, err
		}
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorConflict, core.ReasonBindingDrift, "exact model-turn V2 Inspect returned another dispatch")
	}
	return fact.CloneV2(), nil
}

func (a *ExactModelTurnAdapterV2) resumeV2(
	ctx context.Context,
	current bridgecontract.ModelTurnDispatchFactV2,
) (bridgecontract.ModelTurnDispatchFactV2, error) {
	s1Time, err := freshExactModelTurnTimeV2(a.clock, time.Time{})
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	s1, err := a.inspectClosureV2(ctx, current.Ref.Envelope, s1Time)
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}

	var outcome modelinvoker.GovernedModelTurnOutcomeV3
	switch current.State {
	case bridgecontract.ModelTurnDispatchAttemptBoundV2:
		outcome, err = a.model.StartOrInspectGovernedModelTurnV3(ctx, current.Ref.Envelope.Command)
		if err != nil {
			recoveryContext, cancel := exactModelTurnRecoveryContextV2(
				ctx,
				current.NotAfterUnixNano,
				s1.ExpiresUnixNano,
			)
			recovered, inspectErr := a.model.InspectGovernedModelTurnAttemptV3(
				recoveryContext,
				current.Attempt,
			)
			cancel()
			if inspectErr != nil {
				return bridgecontract.ModelTurnDispatchFactV2{}, errors.Join(err, inspectErr)
			}
			outcome = recovered
		}
	case bridgecontract.ModelTurnDispatchOutcomeBoundV2:
		if current.Outcome == nil {
			return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidState, "outcome-bound exact model-turn V2 sidecar lacks outcome")
		}
		outcome, err = a.model.InspectExactGovernedModelTurnV3(ctx, *current.Outcome)
		if err != nil {
			return bridgecontract.ModelTurnDispatchFactV2{}, err
		}
	default:
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidState, "exact model-turn V2 sidecar state is unsupported")
	}

	s2Time, err := freshExactModelTurnTimeV2(a.clock, s1Time)
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	s2, err := a.inspectClosureV2(ctx, current.Ref.Envelope, s2Time)
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	if s1.ClosureDigest != s2.ClosureDigest ||
		s1.MaterialRef != s2.MaterialRef ||
		s1.SourceLineage != s2.SourceLineage {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorConflict, core.ReasonBindingDrift, "exact model-turn V2 four-source closure drifted between S1 and S2")
	}
	finalNow, err := freshExactModelTurnTimeV2(a.clock, s2Time)
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	expires := minExactModelTurnExpiryV2(
		current.NotAfterUnixNano,
		s1.ExpiresUnixNano,
		s2.ExpiresUnixNano,
		outcome.ExpiresUnixNano,
	)
	if finalNow.UnixNano() >= expires ||
		validateModelTurnOutcomeCurrentV2(outcome, current.Attempt, finalNow) != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "exact model-turn V2 closure crossed its common TTL")
	}
	if current.State == bridgecontract.ModelTurnDispatchOutcomeBoundV2 {
		if current.Outcome == nil || *current.Outcome != outcome.RefV3() {
			return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorConflict, core.ReasonBindingDrift, "exact model-turn V2 stored outcome drifted")
		}
		return current.CloneV2(), nil
	}

	next, err := bridgecontract.BindModelTurnDispatchOutcomeV2(current, outcome.RefV3())
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	stored, err := a.dispatches.BindModelTurnDispatchOutcomeV2(ctx, next)
	if err != nil {
		if !core.HasCategory(err, core.ErrorIndeterminate) &&
			!core.HasCategory(err, core.ErrorUnavailable) &&
			!core.HasCategory(err, core.ErrorConflict) {
			return bridgecontract.ModelTurnDispatchFactV2{}, err
		}
		recoveryContext, cancel := exactModelTurnRecoveryContextV2(ctx, expires)
		recovered, inspectErr := a.dispatches.InspectExactModelTurnDispatchV2(recoveryContext, current.Ref)
		cancel()
		if inspectErr != nil {
			return bridgecontract.ModelTurnDispatchFactV2{}, errors.Join(err, inspectErr)
		}
		stored = recovered
	}
	if err := stored.Validate(); err != nil || !reflect.DeepEqual(stored, next) {
		if err != nil {
			return bridgecontract.ModelTurnDispatchFactV2{}, err
		}
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "exact model-turn V2 sidecar stored another outcome")
	}
	postBindNow, err := freshExactModelTurnTimeV2(a.clock, finalNow)
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	if postBindNow.UnixNano() >= expires ||
		validateModelTurnOutcomeCurrentV2(outcome, current.Attempt, postBindNow) != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "exact model-turn V2 closure crossed its common TTL after durable Bind")
	}
	return stored.CloneV2(), nil
}

type exactModelTurnClosureSnapshotV2 struct {
	MaterialRef     modelinvoker.InvocationMaterialRefV2
	SourceLineage   modelinvoker.InvocationMaterialSourceLineageV2
	ClosureDigest   core.Digest
	ExpiresUnixNano int64
}

func (a *ExactModelTurnAdapterV2) inspectClosureV2(
	ctx context.Context,
	envelope bridgecontract.ModelTurnExactEnvelopeV2,
	now time.Time,
) (exactModelTurnClosureSnapshotV2, error) {
	if err := exactModelTurnContextV2(ctx); err != nil {
		return exactModelTurnClosureSnapshotV2{}, err
	}
	material, err := a.readers.Materials.InspectExactInvocationMaterialV2(ctx, envelope.Material)
	if err != nil {
		return exactModelTurnClosureSnapshotV2{}, err
	}
	if err := material.Validate(); err != nil || material.RefV2() != envelope.Material ||
		now.IsZero() || now.UnixNano() < material.CreatedUnixNano ||
		!now.Before(time.Unix(0, material.ExpiresUnixNano)) {
		if err != nil {
			return exactModelTurnClosureSnapshotV2{}, err
		}
		return exactModelTurnClosureSnapshotV2{}, exactModelTurnErrorV2(core.ErrorConflict, core.ReasonBindingDrift, "exact InvocationMaterialV2 is not current")
	}
	lineage := material.Authorization.SourceLineage
	contextPair, err := a.readers.ContextPair.InspectExactInvocationContextPairV2(
		ctx,
		lineage.ContextFrame,
		lineage.ContextMaterial,
		lineage.ContextMappedInputDigest,
	)
	if err != nil {
		return exactModelTurnClosureSnapshotV2{}, err
	}
	if err = contextPair.ValidateCurrentV2(
		lineage.ContextFrame,
		lineage.ContextMaterial,
		lineage.ContextMappedInputDigest,
		now,
	); err != nil {
		return exactModelTurnClosureSnapshotV2{}, err
	}
	toolPair, err := a.readers.ToolPair.InspectExactInvocationToolPairV2(
		ctx,
		lineage.ToolInjectionMaterial,
		lineage.ToolSurface,
		lineage.RequestToolsDigest,
	)
	if err != nil {
		return exactModelTurnClosureSnapshotV2{}, err
	}
	if err = toolPair.ValidateCurrentV2(
		lineage.ToolInjectionMaterial,
		lineage.ToolSurface,
		lineage.ExpectedInjectionDigest,
		lineage.CompiledToolsDigest,
		lineage.RequestToolsDigest,
		now,
	); err != nil {
		return exactModelTurnClosureSnapshotV2{}, err
	}
	expires := minExactModelTurnExpiryV2(
		envelope.RequestedNotAfterUnixNano,
		material.ExpiresUnixNano,
		contextPair.ExpiresUnixNano,
		toolPair.ExpiresUnixNano,
	)
	if now.UnixNano() >= expires {
		return exactModelTurnClosureSnapshotV2{}, exactModelTurnErrorV2(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "exact model-turn V2 owner closure expired")
	}
	closureDigest, err := core.CanonicalJSONDigest(
		"praxis.harness.exact-model-turn",
		bridgecontract.ModelTurnDispatchContractVersionV2,
		"ExactModelTurnFourSourceClosureV2",
		struct {
			Material                 modelinvoker.InvocationMaterialRefV2           `json:"material"`
			SourceLineage            modelinvoker.InvocationMaterialSourceLineageV2 `json:"source_lineage"`
			ContextProjectionDigest  core.Digest                                    `json:"context_projection_digest"`
			ContextCheckedUnixNano   int64                                          `json:"context_checked_unix_nano"`
			ContextExpiresUnixNano   int64                                          `json:"context_expires_unix_nano"`
			ToolProjectionDigest     core.Digest                                    `json:"tool_projection_digest"`
			ToolCheckedUnixNano      int64                                          `json:"tool_checked_unix_nano"`
			ToolExpiresUnixNano      int64                                          `json:"tool_expires_unix_nano"`
			ContextMappedInputDigest core.Digest                                    `json:"context_mapped_input_digest"`
			ExpectedInjectionDigest  core.Digest                                    `json:"expected_injection_digest"`
			CompiledToolsDigest      core.Digest                                    `json:"compiled_tools_digest"`
			RequestToolsDigest       core.Digest                                    `json:"request_tools_digest"`
		}{
			Material:                 material.RefV2(),
			SourceLineage:            lineage,
			ContextProjectionDigest:  contextPair.ProjectionDigest,
			ContextCheckedUnixNano:   contextPair.CheckedUnixNano,
			ContextExpiresUnixNano:   contextPair.ExpiresUnixNano,
			ToolProjectionDigest:     toolPair.ProjectionDigest,
			ToolCheckedUnixNano:      toolPair.CheckedUnixNano,
			ToolExpiresUnixNano:      toolPair.ExpiresUnixNano,
			ContextMappedInputDigest: contextPair.ContextMappedInputDigest,
			ExpectedInjectionDigest:  toolPair.ExpectedInjectionDigest,
			CompiledToolsDigest:      toolPair.CompiledToolsDigest,
			RequestToolsDigest:       toolPair.RequestToolsDigest,
		},
	)
	if err != nil {
		return exactModelTurnClosureSnapshotV2{}, err
	}
	return exactModelTurnClosureSnapshotV2{
		MaterialRef:     material.RefV2(),
		SourceLineage:   lineage,
		ClosureDigest:   closureDigest,
		ExpiresUnixNano: expires,
	}, nil
}

func (a *ExactModelTurnAdapterV2) preflightV2(
	ctx context.Context,
	envelope bridgecontract.ModelTurnExactEnvelopeV2,
) error {
	if a == nil || nilLikeExactModelTurnV2(a.readers.Materials) ||
		nilLikeExactModelTurnV2(a.readers.ContextPair) ||
		nilLikeExactModelTurnV2(a.readers.ToolPair) ||
		nilLikeExactModelTurnV2(a.dispatches) ||
		nilLikeExactModelTurnV2(a.model) || a.clock == nil {
		return exactModelTurnErrorV2(core.ErrorUnavailable, core.ReasonComponentMissing, "exact model-turn V2 adapter is unavailable")
	}
	if err := exactModelTurnContextV2(ctx); err != nil {
		return err
	}
	return envelope.Validate()
}

func validateModelTurnOutcomeCurrentV2(
	outcome modelinvoker.GovernedModelTurnOutcomeV3,
	attempt modelinvoker.GovernedModelTurnAttemptRefV3,
	now time.Time,
) error {
	if outcome.ValidateAgainstAttemptRefV3(attempt) != nil || now.IsZero() ||
		now.UnixNano() < outcome.CurrentRef.CheckedUnixNano ||
		!now.Before(time.Unix(0, outcome.ExpiresUnixNano)) ||
		!now.Before(time.Unix(0, outcome.CurrentRef.ExpiresUnixNano)) ||
		!now.Before(time.Unix(0, outcome.CurrentRef.NotAfterUnixNano)) ||
		!now.Before(time.Unix(0, outcome.MaterialRef.ExpiresUnixNano)) {
		return exactModelTurnErrorV2(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Model V3 outcome is not exact-current")
	}
	return nil
}

func minExactModelTurnExpiryV2(values ...int64) int64 {
	var minimum int64
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}

func exactModelTurnRecoveryContextV2(
	ctx context.Context,
	expiresUnixNano ...int64,
) (context.Context, context.CancelFunc) {
	deadline := time.Unix(0, minExactModelTurnExpiryV2(expiresUnixNano...))
	return context.WithDeadline(context.WithoutCancel(ctx), deadline)
}

func freshExactModelTurnTimeV2(clock func() time.Time, previous time.Time) (time.Time, error) {
	now := clock()
	if now.IsZero() || (!previous.IsZero() && now.Before(previous)) {
		return time.Time{}, exactModelTurnErrorV2(core.ErrorPreconditionFailed, core.ReasonClockRegression, "exact model-turn V2 clock regressed")
	}
	return now, nil
}

func exactModelTurnContextV2(ctx context.Context) error {
	if ctx == nil {
		return exactModelTurnErrorV2(core.ErrorInvalidArgument, core.ReasonInvalidReference, "exact model-turn V2 context is required")
	}
	if err := ctx.Err(); err != nil {
		return exactModelTurnErrorV2(core.ErrorUnavailable, core.ReasonInvalidState, "exact model-turn V2 context is canceled")
	}
	return nil
}

func exactModelTurnErrorV2(category core.ErrorCategory, reason core.ReasonCode, message string) error {
	return core.NewError(category, reason, message)
}

func nilLikeExactModelTurnV2(value any) bool {
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

var _ harnessports.ExactModelTurnPortV2 = (*ExactModelTurnAdapterV2)(nil)
