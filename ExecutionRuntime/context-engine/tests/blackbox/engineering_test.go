package blackbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/sdk"
)

func TestContextEngineeringSDKBlackBoxV1(t *testing.T) {
	asset := testkit.PromptAssetV1()
	validateRequest, err := sdk.SealValidatePromptAssetEngineeringRequestV1(context.Background(), sdk.ValidatePromptAssetEngineeringRequestV1{
		Meta:  engineeringBlackBoxMetaV1(sdk.EngineeringValidatePromptAssetV1, "blackbox-prompt-validate"),
		Asset: asset,
	})
	if err != nil {
		t.Fatal(err)
	}
	validated, err := sdk.ValidatePromptAssetEngineeringV1(context.Background(), validateRequest)
	if err != nil || !validated.Valid || validated.AssetRef == nil {
		t.Fatalf("prompt validation failed: %#v %v", validated, err)
	}

	previewRequest, err := sdk.SealPreviewPromptCandidatesEngineeringRequestV1(context.Background(), sdk.PreviewPromptCandidatesEngineeringRequestV1{
		Meta: engineeringBlackBoxMetaV1(sdk.EngineeringPreviewPromptV1, "blackbox-prompt-preview"), Asset: asset,
		Build: testkit.PromptBuildRequestV1(asset),
	})
	if err != nil {
		t.Fatal(err)
	}
	previewed, err := sdk.PreviewPromptCandidatesEngineeringV1(context.Background(), previewRequest)
	if err != nil || previewed.Candidates.PromptAssetRef != *validated.AssetRef || len(previewed.Candidates.Candidates) != len(asset.Fragments) {
		t.Fatalf("prompt preview failed: %#v %v", previewed, err)
	}

	baseline, candidate, policy, outcomes := testkit.EngineeringOutcomesV1()
	prepareRequest, err := sdk.SealPrepareContextEvaluationRequestV1(context.Background(), sdk.PrepareContextEvaluationRequestV1{
		Meta: engineeringBlackBoxMetaV1(sdk.EngineeringPrepareEvaluationV1, "blackbox-evaluation-prepare"), EvaluationID: "blackbox-evaluation",
		EvaluatorRef: testkit.EngineeringEvaluatorRefV1(), Outcomes: outcomes, BaselineRecipeRef: baseline,
		CandidateRecipeRef: candidate, PolicyRef: policy, CheckedUnixNano: testkit.Now,
		NotAfterUnixNano: testkit.Now + int64(30*time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := sdk.PrepareContextEvaluationV1(context.Background(), prepareRequest)
	if err != nil {
		t.Fatal(err)
	}
	admitRequest, err := sdk.SealAdmitContextEvaluationRequestV1(context.Background(), sdk.AdmitContextEvaluationRequestV1{
		Meta:        engineeringBlackBoxMetaV1(sdk.EngineeringAdmitEvaluationV1, "blackbox-evaluation-admit"),
		Preparation: prepareRequest, Input: prepared.Input, Observation: testkit.EngineeringObservationV1(prepared.Input),
	})
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := sdk.AdmitContextEvaluationV1(context.Background(), admitRequest)
	if err != nil || admitted.Evaluation.Disposition != contract.ContextEvaluationBetterV1 {
		t.Fatalf("evaluation admission failed: %#v %v", admitted, err)
	}
	feedbackRequest, err := sdk.SealBuildContextFeedbackRequestV1(context.Background(), sdk.BuildContextFeedbackRequestV1{
		Meta: engineeringBlackBoxMetaV1(sdk.EngineeringBuildFeedbackV1, "blackbox-feedback-build"), FeedbackCandidateID: "blackbox-feedback",
		Outcomes: outcomes, Evaluation: admitted.Evaluation, ChangeDigest: testkit.D("blackbox-change"),
		Evidence: []contract.EvidenceRef{testkit.Evidence("blackbox-feedback")}, CreatedUnixNano: admitted.Evaluation.CreatedUnixNano + 1,
		NotAfterUnixNano: admitted.Evaluation.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	feedback, err := sdk.BuildContextFeedbackEngineeringV1(context.Background(), feedbackRequest)
	if err != nil || feedback.Feedback.EvaluationRef != admitted.EvaluationRef || feedback.Feedback.State != contract.ContextFeedbackEvaluatedV1 {
		t.Fatalf("feedback build failed: %#v %v", feedback, err)
	}
}

func engineeringBlackBoxMetaV1(op sdk.ContextEngineeringOperationV1, id string) sdk.ContextEngineeringRequestMetaV1 {
	return sdk.ContextEngineeringRequestMetaV1{
		ContractVersion: sdk.ContextEngineeringSDKContractVersionV1, RequestID: id,
		Operation: op, Limits: sdk.DefaultContextEngineeringLimitsV1(),
	}
}
