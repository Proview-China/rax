package conformance

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/resultgrounding"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ExerciseReviewArtifactCurrentReaderV2 is a reusable Owner conformance
// probe. It checks new-S1 resolve, same-ref S2, exact historical read and
// immutable deep-clone behavior without granting the consumer a publisher.
func ExerciseReviewArtifactCurrentReaderV2(ctx context.Context, reader runtimeports.ReviewArtifactCurrentReaderV2, subject runtimeports.ReviewArtifactCurrentSubjectV2, now time.Time) error {
	ref, err := reader.ResolveCurrentReviewArtifactV2(ctx, subject)
	if err != nil {
		return err
	}
	first, err := reader.InspectCurrentReviewArtifactV2(ctx, subject, ref)
	if err != nil {
		return err
	}
	if err := first.ValidateCurrent(ref, subject, now); err != nil {
		return err
	}
	clone := first.Clone()
	if len(clone.Subject.Anchors) > 0 && len(clone.Subject.Anchors[0].Payload.Inline) > 0 {
		clone.Subject.Anchors[0].Payload.Inline[0] ^= 0xff
	}
	second, err := reader.InspectCurrentReviewArtifactV2(ctx, subject, ref)
	if err != nil {
		return err
	}
	historical, err := reader.InspectHistoricalReviewArtifactV2(ctx, ref)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(first, historical) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Artifact Owner Reader is not exact or immutable")
	}
	return nil
}

func ExerciseReviewEnvironmentCurrentReaderV2(ctx context.Context, reader runtimeports.ReviewEnvironmentCurrentReaderV2, subject runtimeports.ReviewEnvironmentCurrentSubjectV2, now time.Time) error {
	ref, err := reader.ResolveCurrentReviewEnvironmentV2(ctx, subject)
	if err != nil {
		return err
	}
	first, err := reader.InspectCurrentReviewEnvironmentV2(ctx, subject, ref)
	if err != nil {
		return err
	}
	if err := first.ValidateCurrent(ref, subject, now); err != nil {
		return err
	}
	second, err := reader.InspectCurrentReviewEnvironmentV2(ctx, subject, ref)
	if err != nil {
		return err
	}
	historical, err := reader.InspectHistoricalReviewEnvironmentV2(ctx, ref)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(first, historical) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Environment Owner Reader is not exact or immutable")
	}
	return nil
}

func ExerciseReviewValidationScopeCurrentReaderV2(ctx context.Context, reader runtimeports.ReviewValidationScopeCurrentReaderV2, subject runtimeports.ReviewValidationScopeCurrentSubjectV2, now time.Time) error {
	ref, err := reader.ResolveCurrentReviewValidationScopeV2(ctx, subject)
	if err != nil {
		return err
	}
	first, err := reader.InspectCurrentReviewValidationScopeV2(ctx, subject, ref)
	if err != nil {
		return err
	}
	if err := first.ValidateCurrent(ref, subject, now); err != nil {
		return err
	}
	second, err := reader.InspectCurrentReviewValidationScopeV2(ctx, subject, ref)
	if err != nil {
		return err
	}
	historical, err := reader.InspectHistoricalReviewValidationScopeV2(ctx, ref)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(first, historical) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Validation Scope Owner Reader is not exact or immutable")
	}
	return nil
}

func ExerciseResultBundleCurrentGroundingReaderV2(ctx context.Context, reader resultgrounding.ResultBundleCurrentGroundingReaderV2, request resultgrounding.ResultBundleCurrentGroundingRequestV2, now time.Time) error {
	first, err := reader.InspectResultBundleCurrentGroundingV2(ctx, request)
	if err != nil {
		return err
	}
	if err := first.ValidateCurrent(request.Bundle, now); err != nil {
		return err
	}
	clone := first.Clone()
	if len(clone.Artifacts) > 0 && len(clone.Artifacts[0].Subject.Anchors) > 0 && len(clone.Artifacts[0].Subject.Anchors[0].Payload.Inline) > 0 {
		clone.Artifacts[0].Subject.Anchors[0].Payload.Inline[0] ^= 0xff
	}
	if reflect.DeepEqual(clone, first) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "result grounding aggregate does not expose an isolated deep clone")
	}
	second, err := reader.InspectResultBundleCurrentGroundingV2(ctx, request)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(first, second) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "result grounding same-current replay changed its exact aggregate")
	}
	return nil
}

// ExerciseResultBundleGroundingFailureV2 is the reusable negative probe for a
// caller-supplied fault fixture. It proves the reader returned no partial
// aggregate and preserved the expected closed category/reason.
func ExerciseResultBundleGroundingFailureV2(ctx context.Context, reader resultgrounding.ResultBundleCurrentGroundingReaderV2, request resultgrounding.ResultBundleCurrentGroundingRequestV2, category core.ErrorCategory, reason core.ReasonCode) error {
	value, err := reader.InspectResultBundleCurrentGroundingV2(ctx, request)
	if err == nil {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "result grounding fault fixture returned a partial success")
	}
	if !reflect.DeepEqual(value, resultgrounding.ResultBundleCurrentGroundingProjectionV2{}) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "result grounding fault fixture leaked a partial aggregate")
	}
	if category != "" && !core.HasCategory(err, category) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "result grounding fault fixture changed its closed error category")
	}
	if reason != "" && !core.HasReason(err, reason) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "result grounding fault fixture changed its closed reason")
	}
	return nil
}
