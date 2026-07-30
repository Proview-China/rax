// Package runtimeadapter exposes read-only Model-owned projections to Runtime.
// It contains no Runtime guard, Provider dispatch or routegateway logic.
package runtimeadapter

import (
	"context"
	"reflect"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ModelProviderBoundaryCurrentAdapterV1 struct {
	repository modelinvoker.GovernedModelTurnProviderBoundaryRepositoryV3
	clock      func() time.Time
}

func NewModelProviderBoundaryCurrentAdapterV1(
	repository modelinvoker.GovernedModelTurnProviderBoundaryRepositoryV3,
	clock func() time.Time,
) (*ModelProviderBoundaryCurrentAdapterV1, error) {
	if nilLikeV1(repository) || clock == nil {
		return nil, core.NewError(
			core.ErrorInvalidArgument,
			core.ReasonComponentMissing,
			"Model provider boundary repository and clock are required",
		)
	}
	return &ModelProviderBoundaryCurrentAdapterV1{
		repository: repository,
		clock:      clock,
	}, nil
}

func (a *ModelProviderBoundaryCurrentAdapterV1) InspectCurrentModelProviderBoundaryV1(
	ctx context.Context,
	ref runtimeports.ModelProviderBoundaryCurrentRefV1,
) (runtimeports.ModelProviderBoundaryCurrentProjectionV1, error) {
	if a == nil || ctx == nil || ctx.Err() != nil ||
		nilLikeV1(a.repository) || a.clock == nil {
		return runtimeports.ModelProviderBoundaryCurrentProjectionV1{},
			core.NewError(
				core.ErrorUnavailable,
				core.ReasonComponentMissing,
				"Model provider boundary current adapter is unavailable",
			)
	}
	if err := ref.Validate(); err != nil {
		return runtimeports.ModelProviderBoundaryCurrentProjectionV1{}, err
	}
	fact, err := a.repository.InspectGovernedModelTurnProviderBoundaryAttemptV3(
		ctx,
		ref,
	)
	if err != nil {
		return runtimeports.ModelProviderBoundaryCurrentProjectionV1{}, err
	}
	projection, err := fact.RuntimeProjectionV1()
	if err != nil ||
		!runtimeports.SameModelProviderBoundaryCurrentRefV1(projection.Ref, ref) ||
		projection.ProjectionDigest != fact.ProjectionDigest {
		return runtimeports.ModelProviderBoundaryCurrentProjectionV1{},
			core.NewError(
				core.ErrorConflict,
				core.ReasonInvalidDigest,
				"Model provider boundary current projection drifted",
			)
	}
	now := a.clock()
	if now.IsZero() || now.UnixNano() < projection.CheckedUnixNano ||
		!now.Before(time.Unix(0, projection.ExpiresUnixNano)) ||
		projection.ExpiresUnixNano > fact.ExpiresUnixNano ||
		projection.ExpiresUnixNano > ref.ExpiresUnixNano {
		return runtimeports.ModelProviderBoundaryCurrentProjectionV1{},
			core.NewError(
				core.ErrorPreconditionFailed,
				core.ReasonBindingExpired,
				"Model provider boundary current projection is stale",
			)
	}
	return projection, nil
}

func nilLikeV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

var _ runtimeports.ModelProviderBoundaryCurrentReaderV1 = (*ModelProviderBoundaryCurrentAdapterV1)(nil)
