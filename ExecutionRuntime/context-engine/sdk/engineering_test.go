package sdk

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestContextEngineeringPromptValidatePreviewV1(t *testing.T) {
	asset := testkit.PromptAssetV1()
	validate := sealEngineeringPromptValidateV1(t, asset)
	validated, err := ValidatePromptAssetEngineeringV1(context.Background(), validate)
	if err != nil || !validated.Valid || validated.AssetRef == nil || *validated.AssetRef != testkit.PromptAssetRefV1(asset) || len(validated.Diagnostics) != 0 {
		t.Fatalf("validate drift: %#v %v", validated, err)
	}
	previewRequest, err := SealPreviewPromptCandidatesEngineeringRequestV1(context.Background(), PreviewPromptCandidatesEngineeringRequestV1{
		Meta: engineeringMetaV1(EngineeringPreviewPromptV1, "prompt-preview"), Asset: asset, Build: testkit.PromptBuildRequestV1(asset),
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := PreviewPromptCandidatesEngineeringV1(context.Background(), previewRequest)
	if err != nil || len(first.Candidates.Candidates) != len(asset.Fragments) {
		t.Fatalf("preview drift: %#v %v", first, err)
	}
	first.Candidates.Candidates[0].ID = "caller-mutated"
	again, err := PreviewPromptCandidatesEngineeringV1(context.Background(), previewRequest)
	if err != nil || again.Candidates.Candidates[0].ID == "caller-mutated" {
		t.Fatalf("preview alias escaped: %#v %v", again, err)
	}
	invalid := asset
	invalid.ContentDigest = testkit.D("drift")
	bad := sealEngineeringPromptValidateV1(t, invalid)
	report, err := ValidatePromptAssetEngineeringV1(context.Background(), bad)
	if err != nil || report.Valid || report.AssetRef != nil || len(report.Diagnostics) != 1 {
		t.Fatalf("invalid prompt report drift: %#v %v", report, err)
	}
}

func TestContextEngineeringEvaluationFullChainV1(t *testing.T) {
	preparation := engineeringPreparationV1(t)
	prepared, err := PrepareContextEvaluationV1(context.Background(), preparation)
	if err != nil {
		t.Fatal(err)
	}
	observation := testkit.EngineeringObservationV1(prepared.Input)
	admitRequest, err := SealAdmitContextEvaluationRequestV1(context.Background(), AdmitContextEvaluationRequestV1{
		Meta: engineeringMetaV1(EngineeringAdmitEvaluationV1, "evaluation-admit"), Preparation: preparation, Input: prepared.Input, Observation: observation,
	})
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := AdmitContextEvaluationV1(context.Background(), admitRequest)
	if err != nil {
		t.Fatal(err)
	}
	feedbackRequest, err := SealBuildContextFeedbackRequestV1(context.Background(), BuildContextFeedbackRequestV1{
		Meta: engineeringMetaV1(EngineeringBuildFeedbackV1, "feedback-build"), FeedbackCandidateID: "feedback-sdk-v1",
		Outcomes: preparation.Outcomes, Evaluation: admitted.Evaluation, ChangeDigest: testkit.D("change"),
		Evidence: []contract.EvidenceRef{testkit.Evidence("feedback-sdk")}, CreatedUnixNano: admitted.Evaluation.CreatedUnixNano + 1, NotAfterUnixNano: admitted.Evaluation.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	feedback, err := BuildContextFeedbackEngineeringV1(context.Background(), feedbackRequest)
	if err != nil || feedback.Feedback.EvaluationRef != admitted.EvaluationRef || feedback.Feedback.State != contract.ContextFeedbackEvaluatedV1 {
		t.Fatalf("feedback chain drift: %#v %v", feedback, err)
	}

	encoded, err := EncodePrepareContextEvaluationRequestV1(context.Background(), preparation)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePrepareContextEvaluationRequestV1(context.Background(), encoded)
	if err != nil || !reflect.DeepEqual(decoded, preparation) {
		t.Fatalf("prepare codec drift: %#v %v", decoded, err)
	}
	if _, err := EncodeAdmitContextEvaluationResponseV1(context.Background(), admitted); err != nil {
		t.Fatalf("admit response encode failed: %v; meta=%+v limits=%+v evaluation_ref=%+v", err, admitted.Meta, admitted.limits, admitted.EvaluationRef)
	}
	admitted.Evaluation.QualityScorePPM++
	if _, err := EncodeAdmitContextEvaluationResponseV1(context.Background(), admitted); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("tampered response encoded: %v", err)
	}
}

func TestContextEngineeringStrictCodecAndLimitsV1(t *testing.T) {
	preparation := engineeringPreparationV1(t)
	payload, err := EncodePrepareContextEvaluationRequestV1(context.Background(), preparation)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := bytes.Replace(payload, []byte(`"input_tokens":100`), []byte(`"input_tokens":100,"input_tokens":100`), 1)
	if _, err := DecodePrepareContextEvaluationRequestV1(context.Background(), duplicate); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("recursive duplicate key accepted: %v", err)
	}
	unknown := bytes.Replace(payload, []byte(`"evaluation_id":"evaluation-sdk-v1"`), []byte(`"evaluation_id":"evaluation-sdk-v1","unknown":true`), 1)
	if _, err := DecodePrepareContextEvaluationRequestV1(context.Background(), unknown); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("unknown field accepted: %v", err)
	}
	nullOutcomes := preparation
	nullOutcomes.Outcomes = nil
	if _, err := SealPrepareContextEvaluationRequestV1(context.Background(), nullOutcomes); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("null outcomes accepted: %v", err)
	}
	limited := preparation
	limited.Meta.RequestDigest = ""
	limited.Meta.Limits.MaxOutcomes = 1
	if _, err := SealPrepareContextEvaluationRequestV1(context.Background(), limited); !errors.Is(err, contract.ErrLimitExceeded) {
		t.Fatalf("outcome limit bypassed: %v", err)
	}
	limited = preparation
	limited.Meta.RequestDigest = ""
	limited.Meta.Limits.MaxCanonicalBytes = 1
	if _, err := SealPrepareContextEvaluationRequestV1(context.Background(), limited); !errors.Is(err, contract.ErrLimitExceeded) {
		t.Fatalf("canonical limit bypassed: %v", err)
	}
}

func TestContextEngineeringS2UnknownCancelAndLocalEvaluatorV1(t *testing.T) {
	preparation := engineeringPreparationV1(t)
	prepared, err := PrepareContextEvaluationV1(context.Background(), preparation)
	if err != nil {
		t.Fatal(err)
	}
	observation := testkit.EngineeringObservationV1(prepared.Input)
	drift := preparation
	drift.Outcomes = append([]contract.ContextOutcomeFactV1(nil), preparation.Outcomes...)
	drift.Outcomes[1].Metrics.CostMicros++
	drift.Meta.RequestDigest = ""
	drift, err = SealPrepareContextEvaluationRequestV1(context.Background(), drift)
	if err != nil {
		t.Fatal(err)
	}
	admit, err := SealAdmitContextEvaluationRequestV1(context.Background(), AdmitContextEvaluationRequestV1{Meta: engineeringMetaV1(EngineeringAdmitEvaluationV1, "admit-drift"), Preparation: drift, Input: prepared.Input, Observation: observation})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := AdmitContextEvaluationV1(context.Background(), admit); !errors.Is(err, contract.ErrConflict) || !reflect.DeepEqual(result, AdmitContextEvaluationResponseV1{}) {
		t.Fatalf("S2 drift produced evaluation: %#v %v", result, err)
	}
	for name, injected := range map[string]error{"unknown": contract.ErrUnknown, "unavailable": contract.ErrUnavailable} {
		t.Run(name, func(t *testing.T) {
			evaluator := &engineeringEvaluatorFakeV1{ref: testkit.EngineeringEvaluatorRefV1(), err: injected}
			result, err := EvaluateContextWithV1(context.Background(), evaluator, preparation, engineeringMetaV1(EngineeringAdmitEvaluationV1, "admit-"+name))
			if !errors.Is(err, injected) || !reflect.DeepEqual(result, AdmitContextEvaluationResponseV1{}) {
				t.Fatalf("injected evaluator produced result: %#v %v", result, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := PrepareContextEvaluationV1(ctx, preparation); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(result, PrepareContextEvaluationResponseV1{}) {
		t.Fatalf("cancel produced response: %#v %v", result, err)
	}
	evaluator := &engineeringEvaluatorFakeV1{ref: testkit.EngineeringEvaluatorRefV1()}
	result, err := EvaluateContextWithV1(context.Background(), evaluator, preparation, engineeringMetaV1(EngineeringAdmitEvaluationV1, "admit-local"))
	if err != nil || result.Evaluation.Disposition != contract.ContextEvaluationBetterV1 {
		t.Fatalf("local evaluator chain failed: %#v %v", result, err)
	}
}

func TestContextEngineering64ConcurrentDeterministicV1(t *testing.T) {
	request := engineeringPreparationV1(t)
	want, err := PrepareContextEvaluationV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 64)
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := PrepareContextEvaluationV1(context.Background(), request)
			if err == nil && !reflect.DeepEqual(got, want) {
				err = errors.New("evaluation preparation drift")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

type engineeringEvaluatorFakeV1 struct {
	ref contract.ContextEvaluatorRefV1
	err error
}

func (f *engineeringEvaluatorFakeV1) RefV1() contract.ContextEvaluatorRefV1 { return f.ref }

func (f *engineeringEvaluatorFakeV1) EvaluateContextV1(ctx context.Context, input contract.ContextEvaluationInputV1) (contract.ContextEvaluationObservationV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ContextEvaluationObservationV1{}, err
	}
	if f.err != nil {
		return contract.ContextEvaluationObservationV1{}, f.err
	}
	return testkit.EngineeringObservationV1(input), nil
}

func sealEngineeringPromptValidateV1(t *testing.T, asset contract.PromptAssetV1) ValidatePromptAssetEngineeringRequestV1 {
	t.Helper()
	request, err := SealValidatePromptAssetEngineeringRequestV1(context.Background(), ValidatePromptAssetEngineeringRequestV1{Meta: engineeringMetaV1(EngineeringValidatePromptAssetV1, "prompt-validate"), Asset: asset})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func engineeringPreparationV1(t *testing.T) PrepareContextEvaluationRequestV1 {
	t.Helper()
	baseline, candidate, policy, outcomes := testkit.EngineeringOutcomesV1()
	request, err := SealPrepareContextEvaluationRequestV1(context.Background(), PrepareContextEvaluationRequestV1{
		Meta: engineeringMetaV1(EngineeringPrepareEvaluationV1, "evaluation-prepare"), EvaluationID: "evaluation-sdk-v1",
		EvaluatorRef: testkit.EngineeringEvaluatorRefV1(), Outcomes: outcomes, BaselineRecipeRef: baseline,
		CandidateRecipeRef: candidate, PolicyRef: policy, CheckedUnixNano: testkit.Now, NotAfterUnixNano: testkit.Now + int64(30*time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func engineeringMetaV1(op ContextEngineeringOperationV1, id string) ContextEngineeringRequestMetaV1 {
	return ContextEngineeringRequestMetaV1{ContractVersion: ContextEngineeringSDKContractVersionV1, RequestID: id, Operation: op, Limits: DefaultContextEngineeringLimitsV1()}
}
