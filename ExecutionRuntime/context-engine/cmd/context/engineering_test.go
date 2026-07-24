package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/sdk"
)

func TestCLIEngineeringFiveCommandsV1(t *testing.T) {
	for name, fixture := range engineeringCLIFixturesV1(t) {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(context.Background(), fixture.args, bytes.NewReader(fixture.payload), &stdout, &stderr); code != 0 {
				t.Fatalf("exit=%d stderr=%s", code, stderr.String())
			}
			if stderr.Len() != 0 || !json.Valid(bytes.TrimSpace(stdout.Bytes())) {
				t.Fatalf("unexpected streams stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestCLIEngineeringStrictAndCanceledV1(t *testing.T) {
	fixture := engineeringCLIFixturesV1(t)["evaluation_prepare"]
	duplicate := bytes.Replace(fixture.payload, []byte(`"evaluation_id":"engineering-cli-evaluation"`), []byte(`"evaluation_id":"engineering-cli-evaluation","evaluation_id":"duplicate"`), 1)
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), fixture.args, bytes.NewReader(duplicate), &stdout, &stderr); code != 2 {
		t.Fatalf("strict exit=%d stderr=%s", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"invalid_argument"`) || strings.Contains(stderr.String(), "engineering-cli-evaluation") {
		t.Fatalf("strict failure leaked payload: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stdout.Reset()
	stderr.Reset()
	if code := run(ctx, fixture.args, bytes.NewReader(fixture.payload), &stdout, &stderr); code != 5 {
		t.Fatalf("cancel exit=%d stderr=%s", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"canceled"`) {
		t.Fatalf("cancel produced success: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

type engineeringCLIFixtureV1 struct {
	args    []string
	payload []byte
}

func engineeringCLIFixturesV1(t *testing.T) map[string]engineeringCLIFixtureV1 {
	t.Helper()
	ctx := context.Background()
	asset := testkit.PromptAssetV1()
	validate := mustSealEngineeringV1(t, func() (sdk.ValidatePromptAssetEngineeringRequestV1, error) {
		return sdk.SealValidatePromptAssetEngineeringRequestV1(ctx, sdk.ValidatePromptAssetEngineeringRequestV1{Meta: engineeringCLIMetaV1(sdk.EngineeringValidatePromptAssetV1, "engineering-cli-validate"), Asset: asset})
	})
	preview := mustSealEngineeringV1(t, func() (sdk.PreviewPromptCandidatesEngineeringRequestV1, error) {
		return sdk.SealPreviewPromptCandidatesEngineeringRequestV1(ctx, sdk.PreviewPromptCandidatesEngineeringRequestV1{Meta: engineeringCLIMetaV1(sdk.EngineeringPreviewPromptV1, "engineering-cli-preview"), Asset: asset, Build: testkit.PromptBuildRequestV1(asset)})
	})
	baseline, candidate, policy, outcomes := testkit.EngineeringOutcomesV1()
	prepare := mustSealEngineeringV1(t, func() (sdk.PrepareContextEvaluationRequestV1, error) {
		return sdk.SealPrepareContextEvaluationRequestV1(ctx, sdk.PrepareContextEvaluationRequestV1{
			Meta: engineeringCLIMetaV1(sdk.EngineeringPrepareEvaluationV1, "engineering-cli-prepare"), EvaluationID: "engineering-cli-evaluation",
			EvaluatorRef: testkit.EngineeringEvaluatorRefV1(), Outcomes: outcomes, BaselineRecipeRef: baseline, CandidateRecipeRef: candidate,
			PolicyRef: policy, CheckedUnixNano: testkit.Now, NotAfterUnixNano: testkit.Now + int64(30*time.Second),
		})
	})
	prepared, err := sdk.PrepareContextEvaluationV1(ctx, prepare)
	if err != nil {
		t.Fatal(err)
	}
	admit := mustSealEngineeringV1(t, func() (sdk.AdmitContextEvaluationRequestV1, error) {
		return sdk.SealAdmitContextEvaluationRequestV1(ctx, sdk.AdmitContextEvaluationRequestV1{
			Meta: engineeringCLIMetaV1(sdk.EngineeringAdmitEvaluationV1, "engineering-cli-admit"), Preparation: prepare,
			Input: prepared.Input, Observation: testkit.EngineeringObservationV1(prepared.Input),
		})
	})
	admitted, err := sdk.AdmitContextEvaluationV1(ctx, admit)
	if err != nil {
		t.Fatal(err)
	}
	feedback := mustSealEngineeringV1(t, func() (sdk.BuildContextFeedbackRequestV1, error) {
		return sdk.SealBuildContextFeedbackRequestV1(ctx, sdk.BuildContextFeedbackRequestV1{
			Meta: engineeringCLIMetaV1(sdk.EngineeringBuildFeedbackV1, "engineering-cli-feedback"), FeedbackCandidateID: "engineering-cli-feedback-candidate",
			Outcomes: outcomes, Evaluation: admitted.Evaluation, ChangeDigest: testkit.D("engineering-cli-change"), Evidence: []contract.EvidenceRef{testkit.Evidence("engineering-cli-feedback")},
			CreatedUnixNano: admitted.Evaluation.CreatedUnixNano + 1, NotAfterUnixNano: admitted.Evaluation.ExpiresUnixNano,
		})
	})
	return map[string]engineeringCLIFixtureV1{
		"prompt_validate":    {args: []string{"prompt", "validate"}, payload: mustEncodeEngineeringV1(t, func() ([]byte, error) { return sdk.EncodeValidatePromptAssetEngineeringRequestV1(ctx, validate) })},
		"prompt_preview":     {args: []string{"prompt", "preview"}, payload: mustEncodeEngineeringV1(t, func() ([]byte, error) { return sdk.EncodePreviewPromptCandidatesEngineeringRequestV1(ctx, preview) })},
		"evaluation_prepare": {args: []string{"evaluation", "prepare"}, payload: mustEncodeEngineeringV1(t, func() ([]byte, error) { return sdk.EncodePrepareContextEvaluationRequestV1(ctx, prepare) })},
		"evaluation_admit":   {args: []string{"evaluation", "admit"}, payload: mustEncodeEngineeringV1(t, func() ([]byte, error) { return sdk.EncodeAdmitContextEvaluationRequestV1(ctx, admit) })},
		"feedback_build":     {args: []string{"feedback", "build"}, payload: mustEncodeEngineeringV1(t, func() ([]byte, error) { return sdk.EncodeBuildContextFeedbackRequestV1(ctx, feedback) })},
	}
}

func engineeringCLIMetaV1(operation sdk.ContextEngineeringOperationV1, requestID string) sdk.ContextEngineeringRequestMetaV1 {
	return sdk.ContextEngineeringRequestMetaV1{ContractVersion: sdk.ContextEngineeringSDKContractVersionV1, RequestID: requestID, Operation: operation, Limits: sdk.DefaultContextEngineeringLimitsV1()}
}

func mustSealEngineeringV1[T any](t *testing.T, seal func() (T, error)) T {
	t.Helper()
	value, err := seal()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func mustEncodeEngineeringV1(t *testing.T, encode func() ([]byte, error)) []byte {
	t.Helper()
	payload, err := encode()
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
