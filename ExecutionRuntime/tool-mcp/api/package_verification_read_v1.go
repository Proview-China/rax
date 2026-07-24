package api

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type PackageVerificationHistoryReadPortV1 interface {
	InspectExactToolPackageVerificationObservationV1(context.Context, toolcontract.ToolPackageVerificationObservationRefV1) (toolcontract.ToolPackageVerificationObservationV1, error)
	InspectExactToolPackageVerificationFactV1(context.Context, toolcontract.ToolPackageVerificationFactRefV1) (toolcontract.ToolPackageVerificationFactV1, error)
}

type PackageVerificationCurrentReadPortV1 interface {
	InspectCurrentToolPackageVerificationV1(context.Context, toolcontract.ToolPackageVerificationCurrentRefV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error)
}

// PackageVerificationReadV1 is a transport-neutral exact read API. It does not
// choose HTTP/gRPC, expose material bytes, or provide verification/admission
// writes.
type PackageVerificationReadV1 struct {
	history PackageVerificationHistoryReadPortV1
	current PackageVerificationCurrentReadPortV1
}

func NewPackageVerificationReadV1(history PackageVerificationHistoryReadPortV1, current PackageVerificationCurrentReadPortV1) (*PackageVerificationReadV1, error) {
	if nilLikeMCPDiscoveryReadV2(history) || nilLikeMCPDiscoveryReadV2(current) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Package Verification read dependencies are required")
	}
	return &PackageVerificationReadV1{history: history, current: current}, nil
}

func (a *PackageVerificationReadV1) InspectPackageVerificationObservationV1(ctx context.Context, exact toolcontract.ToolPackageVerificationObservationRefV1) (toolcontract.ToolPackageVerificationObservationV1, error) {
	if exact.Validate() != nil {
		return toolcontract.ToolPackageVerificationObservationV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Package Verification Observation Ref is invalid")
	}
	return inspectPackageExactTwiceV1(ctx, a, func(ctx context.Context) (toolcontract.ToolPackageVerificationObservationV1, error) {
		return a.history.InspectExactToolPackageVerificationObservationV1(ctx, exact)
	}, func(value toolcontract.ToolPackageVerificationObservationV1) bool {
		return value.Ref == exact && value.Validate() == nil
	})
}

func (a *PackageVerificationReadV1) InspectPackageVerificationFactV1(ctx context.Context, exact toolcontract.ToolPackageVerificationFactRefV1) (toolcontract.ToolPackageVerificationFactV1, error) {
	if exact.Validate() != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Package Verification Fact Ref is invalid")
	}
	return inspectPackageExactTwiceV1(ctx, a, func(ctx context.Context) (toolcontract.ToolPackageVerificationFactV1, error) {
		return a.history.InspectExactToolPackageVerificationFactV1(ctx, exact)
	}, func(value toolcontract.ToolPackageVerificationFactV1) bool {
		return value.Ref == exact && value.Validate() == nil
	})
}

func (a *PackageVerificationReadV1) InspectPackageVerificationCurrentV1(ctx context.Context, exact toolcontract.ToolPackageVerificationCurrentRefV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
	if exact.Validate() != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Package Verification Current Ref is invalid")
	}
	return inspectPackageExactTwiceV1(ctx, a, func(ctx context.Context) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
		return a.current.InspectCurrentToolPackageVerificationV1(ctx, exact)
	}, func(value toolcontract.ToolPackageVerificationCurrentProjectionV1) bool {
		return value.Ref == exact && value.Validate() == nil
	})
}

func inspectPackageExactTwiceV1[T any](ctx context.Context, api *PackageVerificationReadV1, read func(context.Context) (T, error), valid func(T) bool) (T, error) {
	var zero T
	if ctx == nil {
		return zero, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Package Verification read context is required")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if api == nil || nilLikeMCPDiscoveryReadV2(api.history) || nilLikeMCPDiscoveryReadV2(api.current) {
		return zero, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Package Verification read API is unavailable")
	}
	first, err := read(ctx)
	if err != nil {
		return zero, err
	}
	second, err := read(ctx)
	if err != nil {
		return zero, err
	}
	if !valid(first) || !valid(second) || !reflect.DeepEqual(first, second) {
		return zero, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Package Verification changed during exact read")
	}
	return second, nil
}
