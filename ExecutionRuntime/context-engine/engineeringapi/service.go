package engineeringapi

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/sdk"
)

// ContextEngineeringAPIV1 is an owner-local, transport-neutral development
// surface. It grants no Store, listener, capability, provider, publication, or
// production-root authority.
type ContextEngineeringAPIV1 interface {
	ValidatePromptAsset(context.Context, sdk.ValidatePromptAssetEngineeringRequestV1) (sdk.ValidatePromptAssetEngineeringResponseV1, error)
	PreviewPromptCandidates(context.Context, sdk.PreviewPromptCandidatesEngineeringRequestV1) (sdk.PreviewPromptCandidatesEngineeringResponseV1, error)
	PrepareContextEvaluation(context.Context, sdk.PrepareContextEvaluationRequestV1) (sdk.PrepareContextEvaluationResponseV1, error)
	AdmitContextEvaluation(context.Context, sdk.AdmitContextEvaluationRequestV1) (sdk.AdmitContextEvaluationResponseV1, error)
	BuildContextFeedback(context.Context, sdk.BuildContextFeedbackRequestV1) (sdk.BuildContextFeedbackResponseV1, error)
	ExecuteJSON(context.Context, sdk.ContextEngineeringOperationV1, []byte) ([]byte, error)
}

type ServiceV1 struct{}

func (ServiceV1) ValidatePromptAsset(ctx context.Context, request sdk.ValidatePromptAssetEngineeringRequestV1) (sdk.ValidatePromptAssetEngineeringResponseV1, error) {
	return sdk.ValidatePromptAssetEngineeringV1(ctx, request)
}

func (ServiceV1) PreviewPromptCandidates(ctx context.Context, request sdk.PreviewPromptCandidatesEngineeringRequestV1) (sdk.PreviewPromptCandidatesEngineeringResponseV1, error) {
	return sdk.PreviewPromptCandidatesEngineeringV1(ctx, request)
}

func (ServiceV1) PrepareContextEvaluation(ctx context.Context, request sdk.PrepareContextEvaluationRequestV1) (sdk.PrepareContextEvaluationResponseV1, error) {
	return sdk.PrepareContextEvaluationV1(ctx, request)
}

func (ServiceV1) AdmitContextEvaluation(ctx context.Context, request sdk.AdmitContextEvaluationRequestV1) (sdk.AdmitContextEvaluationResponseV1, error) {
	return sdk.AdmitContextEvaluationV1(ctx, request)
}

func (ServiceV1) BuildContextFeedback(ctx context.Context, request sdk.BuildContextFeedbackRequestV1) (sdk.BuildContextFeedbackResponseV1, error) {
	return sdk.BuildContextFeedbackEngineeringV1(ctx, request)
}

func (s ServiceV1) ExecuteJSON(ctx context.Context, operation sdk.ContextEngineeringOperationV1, payload []byte) ([]byte, error) {
	switch operation {
	case sdk.EngineeringValidatePromptAssetV1:
		request, err := sdk.DecodeValidatePromptAssetEngineeringRequestV1(ctx, payload)
		if err != nil {
			return nil, err
		}
		response, err := s.ValidatePromptAsset(ctx, request)
		if err != nil {
			return nil, err
		}
		return sdk.EncodeValidatePromptAssetEngineeringResponseV1(ctx, response)
	case sdk.EngineeringPreviewPromptV1:
		request, err := sdk.DecodePreviewPromptCandidatesEngineeringRequestV1(ctx, payload)
		if err != nil {
			return nil, err
		}
		response, err := s.PreviewPromptCandidates(ctx, request)
		if err != nil {
			return nil, err
		}
		return sdk.EncodePreviewPromptCandidatesEngineeringResponseV1(ctx, response)
	case sdk.EngineeringPrepareEvaluationV1:
		request, err := sdk.DecodePrepareContextEvaluationRequestV1(ctx, payload)
		if err != nil {
			return nil, err
		}
		response, err := s.PrepareContextEvaluation(ctx, request)
		if err != nil {
			return nil, err
		}
		return sdk.EncodePrepareContextEvaluationResponseV1(ctx, response)
	case sdk.EngineeringAdmitEvaluationV1:
		request, err := sdk.DecodeAdmitContextEvaluationRequestV1(ctx, payload)
		if err != nil {
			return nil, err
		}
		response, err := s.AdmitContextEvaluation(ctx, request)
		if err != nil {
			return nil, err
		}
		return sdk.EncodeAdmitContextEvaluationResponseV1(ctx, response)
	case sdk.EngineeringBuildFeedbackV1:
		request, err := sdk.DecodeBuildContextFeedbackRequestV1(ctx, payload)
		if err != nil {
			return nil, err
		}
		response, err := s.BuildContextFeedback(ctx, request)
		if err != nil {
			return nil, err
		}
		return sdk.EncodeBuildContextFeedbackResponseV1(ctx, response)
	default:
		return nil, sdk.UnsupportedEngineeringOperationErrorV1(operation)
	}
}

var _ ContextEngineeringAPIV1 = ServiceV1{}
