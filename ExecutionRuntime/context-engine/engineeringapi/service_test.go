package engineeringapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/engineeringapi"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/sdk"
)

func TestEngineeringAPIFiveTypedAndJSONOperationsV1(t *testing.T) {
	ctx := context.Background()
	service := engineeringapi.ServiceV1{}
	asset := testkit.PromptAssetV1()

	validate := sealValidateV1(t, asset)
	wantValidate, err := service.ValidatePromptAsset(ctx, validate)
	if err != nil {
		t.Fatal(err)
	}
	validateJSON := executeV1(t, service, sdk.EngineeringValidatePromptAssetV1, mustEncodeV1(t, func() ([]byte, error) { return sdk.EncodeValidatePromptAssetEngineeringRequestV1(ctx, validate) }))
	var gotValidate sdk.ValidatePromptAssetEngineeringResponseV1
	decodeResponseV1(t, validateJSON, &gotValidate)
	if gotValidate.Meta != wantValidate.Meta || gotValidate.Valid != wantValidate.Valid || !reflect.DeepEqual(gotValidate.AssetRef, wantValidate.AssetRef) {
		t.Fatal("validate typed/json drift")
	}

	preview := sealPreviewV1(t, asset)
	wantPreview, err := service.PreviewPromptCandidates(ctx, preview)
	if err != nil {
		t.Fatal(err)
	}
	previewJSON := executeV1(t, service, sdk.EngineeringPreviewPromptV1, mustEncodeV1(t, func() ([]byte, error) { return sdk.EncodePreviewPromptCandidatesEngineeringRequestV1(ctx, preview) }))
	var gotPreview sdk.PreviewPromptCandidatesEngineeringResponseV1
	decodeResponseV1(t, previewJSON, &gotPreview)
	if gotPreview.Meta != wantPreview.Meta || gotPreview.Candidates.ProjectionDigest != wantPreview.Candidates.ProjectionDigest {
		t.Fatal("preview typed/json drift")
	}

	prepare := sealPrepareV1(t)
	wantPrepare, err := service.PrepareContextEvaluation(ctx, prepare)
	if err != nil {
		t.Fatal(err)
	}
	prepareJSON := executeV1(t, service, sdk.EngineeringPrepareEvaluationV1, mustEncodeV1(t, func() ([]byte, error) { return sdk.EncodePrepareContextEvaluationRequestV1(ctx, prepare) }))
	var gotPrepare sdk.PrepareContextEvaluationResponseV1
	decodeResponseV1(t, prepareJSON, &gotPrepare)
	if gotPrepare.Meta != wantPrepare.Meta || gotPrepare.Input.InputDigest != wantPrepare.Input.InputDigest {
		t.Fatal("prepare typed/json drift")
	}

	admit := sealAdmitV1(t, prepare, wantPrepare.Input)
	wantAdmit, err := service.AdmitContextEvaluation(ctx, admit)
	if err != nil {
		t.Fatal(err)
	}
	admitJSON := executeV1(t, service, sdk.EngineeringAdmitEvaluationV1, mustEncodeV1(t, func() ([]byte, error) { return sdk.EncodeAdmitContextEvaluationRequestV1(ctx, admit) }))
	var gotAdmit sdk.AdmitContextEvaluationResponseV1
	decodeResponseV1(t, admitJSON, &gotAdmit)
	if gotAdmit.Meta != wantAdmit.Meta || gotAdmit.EvaluationRef != wantAdmit.EvaluationRef {
		t.Fatal("admit typed/json drift")
	}

	feedback := sealFeedbackV1(t, prepare.Outcomes, wantAdmit.Evaluation)
	wantFeedback, err := service.BuildContextFeedback(ctx, feedback)
	if err != nil {
		t.Fatal(err)
	}
	feedbackJSON := executeV1(t, service, sdk.EngineeringBuildFeedbackV1, mustEncodeV1(t, func() ([]byte, error) { return sdk.EncodeBuildContextFeedbackRequestV1(ctx, feedback) }))
	var gotFeedback sdk.BuildContextFeedbackResponseV1
	decodeResponseV1(t, feedbackJSON, &gotFeedback)
	if gotFeedback.Meta != wantFeedback.Meta || gotFeedback.FeedbackRef != wantFeedback.FeedbackRef {
		t.Fatal("feedback typed/json drift")
	}
}

func TestEngineeringAPIJSONStrictUnknownAndCancelV1(t *testing.T) {
	ctx := context.Background()
	service := engineeringapi.ServiceV1{}
	request := sealPrepareV1(t)
	payload := mustEncodeV1(t, func() ([]byte, error) { return sdk.EncodePrepareContextEvaluationRequestV1(ctx, request) })
	duplicate := bytes.Replace(payload, []byte(`"evaluation_id":"engineering-api-evaluation"`), []byte(`"evaluation_id":"engineering-api-evaluation","evaluation_id":"duplicate"`), 1)
	if response, err := service.ExecuteJSON(ctx, sdk.EngineeringPrepareEvaluationV1, duplicate); !errors.Is(err, contract.ErrInvalid) || response != nil {
		t.Fatalf("duplicate key accepted: %q %v", response, err)
	}
	if response, err := service.ExecuteJSON(ctx, "future", payload); !errors.Is(err, contract.ErrUnsupported) || response != nil {
		t.Fatalf("unknown operation accepted: %q %v", response, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if response, err := service.ExecuteJSON(canceled, sdk.EngineeringPrepareEvaluationV1, payload); !errors.Is(err, context.Canceled) || response != nil {
		t.Fatalf("canceled operation produced response: %q %v", response, err)
	}
}

func sealValidateV1(t *testing.T, asset contract.PromptAssetV1) sdk.ValidatePromptAssetEngineeringRequestV1 {
	t.Helper()
	request, err := sdk.SealValidatePromptAssetEngineeringRequestV1(context.Background(), sdk.ValidatePromptAssetEngineeringRequestV1{Meta: engineeringMetaV1(sdk.EngineeringValidatePromptAssetV1, "engineering-api-validate"), Asset: asset})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func sealPreviewV1(t *testing.T, asset contract.PromptAssetV1) sdk.PreviewPromptCandidatesEngineeringRequestV1 {
	t.Helper()
	request, err := sdk.SealPreviewPromptCandidatesEngineeringRequestV1(context.Background(), sdk.PreviewPromptCandidatesEngineeringRequestV1{Meta: engineeringMetaV1(sdk.EngineeringPreviewPromptV1, "engineering-api-preview"), Asset: asset, Build: testkit.PromptBuildRequestV1(asset)})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func sealPrepareV1(t *testing.T) sdk.PrepareContextEvaluationRequestV1 {
	t.Helper()
	baseline, candidate, policy, outcomes := testkit.EngineeringOutcomesV1()
	request, err := sdk.SealPrepareContextEvaluationRequestV1(context.Background(), sdk.PrepareContextEvaluationRequestV1{
		Meta: engineeringMetaV1(sdk.EngineeringPrepareEvaluationV1, "engineering-api-prepare"), EvaluationID: "engineering-api-evaluation",
		EvaluatorRef: testkit.EngineeringEvaluatorRefV1(), Outcomes: outcomes, BaselineRecipeRef: baseline, CandidateRecipeRef: candidate,
		PolicyRef: policy, CheckedUnixNano: testkit.Now, NotAfterUnixNano: testkit.Now + int64(30*time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func sealAdmitV1(t *testing.T, preparation sdk.PrepareContextEvaluationRequestV1, input contract.ContextEvaluationInputV1) sdk.AdmitContextEvaluationRequestV1 {
	t.Helper()
	request, err := sdk.SealAdmitContextEvaluationRequestV1(context.Background(), sdk.AdmitContextEvaluationRequestV1{
		Meta: engineeringMetaV1(sdk.EngineeringAdmitEvaluationV1, "engineering-api-admit"), Preparation: preparation,
		Input: input, Observation: testkit.EngineeringObservationV1(input),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func sealFeedbackV1(t *testing.T, outcomes []contract.ContextOutcomeFactV1, evaluation contract.ContextEvaluationFactV1) sdk.BuildContextFeedbackRequestV1 {
	t.Helper()
	request, err := sdk.SealBuildContextFeedbackRequestV1(context.Background(), sdk.BuildContextFeedbackRequestV1{
		Meta: engineeringMetaV1(sdk.EngineeringBuildFeedbackV1, "engineering-api-feedback"), FeedbackCandidateID: "engineering-api-feedback-candidate",
		Outcomes: outcomes, Evaluation: evaluation, ChangeDigest: testkit.D("engineering-api-change"), Evidence: []contract.EvidenceRef{testkit.Evidence("engineering-api-feedback")},
		CreatedUnixNano: evaluation.CreatedUnixNano + 1, NotAfterUnixNano: evaluation.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func engineeringMetaV1(operation sdk.ContextEngineeringOperationV1, requestID string) sdk.ContextEngineeringRequestMetaV1 {
	return sdk.ContextEngineeringRequestMetaV1{ContractVersion: sdk.ContextEngineeringSDKContractVersionV1, RequestID: requestID, Operation: operation, Limits: sdk.DefaultContextEngineeringLimitsV1()}
}

func mustEncodeV1(t *testing.T, encode func() ([]byte, error)) []byte {
	t.Helper()
	payload, err := encode()
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func executeV1(t *testing.T, service engineeringapi.ServiceV1, operation sdk.ContextEngineeringOperationV1, payload []byte) []byte {
	t.Helper()
	response, err := service.ExecuteJSON(context.Background(), operation, payload)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decodeResponseV1(t *testing.T, payload []byte, target any) {
	t.Helper()
	if !json.Valid(payload) {
		t.Fatalf("invalid JSON response: %q", payload)
	}
	if err := json.Unmarshal(payload, target); err != nil {
		t.Fatal(err)
	}
}
