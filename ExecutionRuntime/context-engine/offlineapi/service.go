package offlineapi

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/sdk"
)

// ContextOfflineAPIV1 is an owner-local, transport-neutral read-only surface.
// It neither exposes a Store nor grants any production capability.
type ContextOfflineAPIV1 interface {
	ValidateRecipe(context.Context, sdk.ValidateRecipeRequestV1) (sdk.ValidateRecipeResponseV1, error)
	CompareRecipes(context.Context, sdk.CompareRecipesRequestV1) (sdk.CompareRecipesResponseV1, error)
	CompileFrame(context.Context, sdk.CompileFrameRequestV1) (sdk.CompileFrameResponseV1, error)
	PreviewFrame(context.Context, sdk.PreviewFrameRequestV1) (sdk.PreviewFrameResponseV1, error)
	InspectFrameExact(context.Context, sdk.InspectFrameExactRequestV1) (sdk.InspectFrameExactResponseV1, error)
	InspectCachePlan(context.Context, sdk.InspectCachePlanRequestV1) (sdk.InspectCachePlanResponseV1, error)
	ExecuteJSON(context.Context, sdk.OfflineSDKOperationV1, []byte) ([]byte, error)
}

type ServiceV1 struct{}

func (ServiceV1) ValidateRecipe(ctx context.Context, request sdk.ValidateRecipeRequestV1) (sdk.ValidateRecipeResponseV1, error) {
	return sdk.ValidateRecipeV1(ctx, request)
}

func (ServiceV1) CompareRecipes(ctx context.Context, request sdk.CompareRecipesRequestV1) (sdk.CompareRecipesResponseV1, error) {
	return sdk.CompareRecipesV1(ctx, request)
}

func (ServiceV1) CompileFrame(ctx context.Context, request sdk.CompileFrameRequestV1) (sdk.CompileFrameResponseV1, error) {
	return sdk.CompileFrameV1(ctx, request)
}

func (ServiceV1) PreviewFrame(ctx context.Context, request sdk.PreviewFrameRequestV1) (sdk.PreviewFrameResponseV1, error) {
	return sdk.PreviewFrameV1(ctx, request)
}

func (ServiceV1) InspectFrameExact(ctx context.Context, request sdk.InspectFrameExactRequestV1) (sdk.InspectFrameExactResponseV1, error) {
	return sdk.InspectFrameExactV1(ctx, request)
}

func (ServiceV1) InspectCachePlan(ctx context.Context, request sdk.InspectCachePlanRequestV1) (sdk.InspectCachePlanResponseV1, error) {
	return sdk.InspectCachePlanV1(ctx, request)
}

func (s ServiceV1) ExecuteJSON(ctx context.Context, operation sdk.OfflineSDKOperationV1, payload []byte) ([]byte, error) {
	switch operation {
	case sdk.OfflineValidateRecipeV1:
		request, err := sdk.DecodeValidateRecipeRequestV1(ctx, payload)
		if err != nil {
			return nil, err
		}
		response, err := s.ValidateRecipe(ctx, request)
		if err != nil {
			return nil, err
		}
		return sdk.EncodeValidateRecipeResponseV1(ctx, response)
	case sdk.OfflineCompareRecipesV1:
		request, err := sdk.DecodeCompareRecipesRequestV1(ctx, payload)
		if err != nil {
			return nil, err
		}
		response, err := s.CompareRecipes(ctx, request)
		if err != nil {
			return nil, err
		}
		return sdk.EncodeCompareRecipesResponseV1(ctx, response)
	case sdk.OfflineCompileFrameV1:
		request, err := sdk.DecodeCompileFrameRequestV1(ctx, payload)
		if err != nil {
			return nil, err
		}
		response, err := s.CompileFrame(ctx, request)
		if err != nil {
			return nil, err
		}
		return sdk.EncodeCompileFrameResponseV1(ctx, response)
	case sdk.OfflinePreviewFrameV1:
		request, err := sdk.DecodePreviewFrameRequestV1(ctx, payload)
		if err != nil {
			return nil, err
		}
		response, err := s.PreviewFrame(ctx, request)
		if err != nil {
			return nil, err
		}
		return sdk.EncodePreviewFrameResponseV1(ctx, response)
	case sdk.OfflineInspectFrameExactV1:
		request, err := sdk.DecodeInspectFrameExactRequestV1(ctx, payload)
		if err != nil {
			return nil, err
		}
		response, err := s.InspectFrameExact(ctx, request)
		if err != nil {
			return nil, err
		}
		return sdk.EncodeInspectFrameExactResponseV1(ctx, response)
	case sdk.OfflineInspectCachePlanV1:
		request, err := sdk.DecodeInspectCachePlanRequestV1(ctx, payload)
		if err != nil {
			return nil, err
		}
		response, err := s.InspectCachePlan(ctx, request)
		if err != nil {
			return nil, err
		}
		return sdk.EncodeInspectCachePlanResponseV1(ctx, response)
	default:
		return nil, sdk.UnsupportedOperationErrorV1(operation)
	}
}

var _ ContextOfflineAPIV1 = ServiceV1{}
