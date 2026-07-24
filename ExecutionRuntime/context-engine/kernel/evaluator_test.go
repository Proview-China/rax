package kernel_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestEvaluatorPrepareAdmitFeedbackExactChainV1(t *testing.T) {
	baseline, candidate, policy, outcomes := testkit.EngineeringOutcomesV1()
	input, err := kernel.PrepareContextEvaluationInputV1(context.Background(), "evaluation-kernel-v1", testkit.EngineeringEvaluatorRefV1(), outcomes, baseline, candidate, policy, testkit.Now, testkit.Now+int64(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	observation := testkit.EngineeringObservationV1(input)
	evaluation, evaluationRef, err := kernel.AdmitContextEvaluationObservationV1(context.Background(), outcomes, input, observation)
	if err != nil {
		t.Fatal(err)
	}
	feedback, feedbackRef, err := kernel.BuildContextFeedbackCandidateV1(context.Background(), "feedback-kernel-v1", outcomes, evaluation, testkit.D("change"), []contract.EvidenceRef{testkit.Evidence("feedback")}, evaluation.CreatedUnixNano+1, evaluation.ExpiresUnixNano)
	if err != nil {
		t.Fatal(err)
	}
	if feedback.EvaluationRef != evaluationRef || feedback.RiskScorePPM != evaluation.RiskScorePPM || feedback.BaseRecipeRef != baseline || feedbackRef.ID != feedback.ID {
		t.Fatalf("feedback exact chain drift: %#v %#v", feedback, feedbackRef)
	}
	observation.Evidence[0] = testkit.Evidence("caller-mutated")
	if evaluation.Evidence[0] == observation.Evidence[0] {
		t.Fatal("observation/evaluation alias escaped")
	}
}

func TestEvaluatorKernelHardDriftMatrixV1(t *testing.T) {
	baseline, candidate, policy, outcomes := testkit.EngineeringOutcomesV1()
	input, err := kernel.PrepareContextEvaluationInputV1(context.Background(), "evaluation-drift-v1", testkit.EngineeringEvaluatorRefV1(), outcomes, baseline, candidate, policy, testkit.Now, testkit.Now+int64(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	baseObservation := testkit.EngineeringObservationV1(input)
	tests := map[string]func(*contract.ContextEvaluationObservationV1){
		"evaluator": func(v *contract.ContextEvaluationObservationV1) { v.EvaluatorRef.Digest = testkit.D("other") },
		"input":     func(v *contract.ContextEvaluationObservationV1) { v.InputDigest = testkit.D("other") },
		"policy":    func(v *contract.ContextEvaluationObservationV1) { v.PolicyRef = testkit.EngineeringRefV1("other") },
		"outcomes":  func(v *contract.ContextEvaluationObservationV1) { v.OutcomeRefs = v.OutcomeRefs[:1] },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := baseObservation
			value.OutcomeRefs = append([]contract.FactRef(nil), baseObservation.OutcomeRefs...)
			mutate(&value)
			value, _ = contract.SealContextEvaluationObservationV1(value)
			result, ref, err := kernel.AdmitContextEvaluationObservationV1(context.Background(), outcomes, input, value)
			if err == nil || !reflect.DeepEqual(result, contract.ContextEvaluationFactV1{}) || ref != (contract.FactRef{}) {
				t.Fatalf("drift accepted: %#v %#v %v", result, ref, err)
			}
		})
	}
	driftedOutcomes := append([]contract.ContextOutcomeFactV1(nil), outcomes...)
	driftedOutcomes[1].Metrics.CostMicros++
	if result, ref, err := kernel.AdmitContextEvaluationObservationV1(context.Background(), driftedOutcomes, input, baseObservation); !errors.Is(err, contract.ErrConflict) || !reflect.DeepEqual(result, contract.ContextEvaluationFactV1{}) || ref != (contract.FactRef{}) {
		t.Fatalf("S2 drift accepted: %#v %#v %v", result, ref, err)
	}
	if _, err := kernel.PrepareContextEvaluationInputV1(context.Background(), "one-side", testkit.EngineeringEvaluatorRefV1(), outcomes[:1], baseline, candidate, policy, testkit.Now, testkit.Now+int64(30*time.Second)); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("one-sided evaluation accepted: %v", err)
	}
}

func TestEvaluatorKernelCancelReturnsZeroV1(t *testing.T) {
	baseline, candidate, policy, outcomes := testkit.EngineeringOutcomesV1()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	input, err := kernel.PrepareContextEvaluationInputV1(ctx, "evaluation-cancel", testkit.EngineeringEvaluatorRefV1(), outcomes, baseline, candidate, policy, testkit.Now, testkit.Now+int64(30*time.Second))
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(input, contract.ContextEvaluationInputV1{}) {
		t.Fatalf("cancel returned input: %#v %v", input, err)
	}
}
