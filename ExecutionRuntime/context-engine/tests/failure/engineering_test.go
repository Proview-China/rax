package failure_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/sdk"
)

type engineeringFailureEvaluatorV1 struct {
	ref contract.ContextEvaluatorRefV1
	err error
}

func (f *engineeringFailureEvaluatorV1) RefV1() contract.ContextEvaluatorRefV1 { return f.ref }

func (f *engineeringFailureEvaluatorV1) EvaluateContextV1(ctx context.Context, input contract.ContextEvaluationInputV1) (contract.ContextEvaluationObservationV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ContextEvaluationObservationV1{}, err
	}
	if f.err != nil {
		return contract.ContextEvaluationObservationV1{}, f.err
	}
	return testkit.EngineeringObservationV1(input), nil
}

func TestContextEngineeringEvaluatorFaultsReturnZeroV1(t *testing.T) {
	request := engineeringFailurePreparationV1(t)
	for name, injected := range map[string]error{
		"unknown": contract.ErrUnknown, "unavailable": contract.ErrUnavailable,
		"canceled": context.Canceled, "deadline": context.DeadlineExceeded,
	} {
		t.Run(name, func(t *testing.T) {
			evaluator := &engineeringFailureEvaluatorV1{ref: testkit.EngineeringEvaluatorRefV1(), err: injected}
			got, err := sdk.EvaluateContextWithV1(context.Background(), evaluator, request, engineeringFailureMetaV1(sdk.EngineeringAdmitEvaluationV1, "failure-"+name))
			if !errors.Is(err, injected) || !reflect.DeepEqual(got, sdk.AdmitContextEvaluationResponseV1{}) {
				t.Fatalf("fault produced result: %#v %v", got, err)
			}
		})
	}
}

func TestContextEngineeringS2DriftAndTTLReturnZeroV1(t *testing.T) {
	request := engineeringFailurePreparationV1(t)
	prepared, err := sdk.PrepareContextEvaluationV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	observation := testkit.EngineeringObservationV1(prepared.Input)

	drift := request
	drift.Outcomes = append([]contract.ContextOutcomeFactV1(nil), request.Outcomes...)
	drift.Outcomes[1].Metrics.CacheReadTokens++
	drift.Meta.RequestDigest = ""
	drift, err = sdk.SealPrepareContextEvaluationRequestV1(context.Background(), drift)
	if err != nil {
		t.Fatal(err)
	}
	driftRequest, err := sdk.SealAdmitContextEvaluationRequestV1(context.Background(), sdk.AdmitContextEvaluationRequestV1{
		Meta: engineeringFailureMetaV1(sdk.EngineeringAdmitEvaluationV1, "failure-s2-drift"), Preparation: drift,
		Input: prepared.Input, Observation: observation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := sdk.AdmitContextEvaluationV1(context.Background(), driftRequest); !errors.Is(err, contract.ErrConflict) || !reflect.DeepEqual(got, sdk.AdmitContextEvaluationResponseV1{}) {
		t.Fatalf("S2 drift produced result: %#v %v", got, err)
	}

	expiredObservation := observation
	expiredObservation.ObservedUnixNano = prepared.Input.ExpiresUnixNano
	expiredObservation.ExpiresUnixNano = prepared.Input.ExpiresUnixNano + 1
	expiredObservation.ObservationDigest = ""
	expiredObservation, err = contract.SealContextEvaluationObservationV1(expiredObservation)
	if err != nil {
		t.Fatal(err)
	}
	expiredRequest, err := sdk.SealAdmitContextEvaluationRequestV1(context.Background(), sdk.AdmitContextEvaluationRequestV1{
		Meta: engineeringFailureMetaV1(sdk.EngineeringAdmitEvaluationV1, "failure-ttl-crossing"), Preparation: request,
		Input: prepared.Input, Observation: expiredObservation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := sdk.AdmitContextEvaluationV1(context.Background(), expiredRequest); !errors.Is(err, contract.ErrExpired) || !reflect.DeepEqual(got, sdk.AdmitContextEvaluationResponseV1{}) {
		t.Fatalf("TTL crossing produced result: %#v %v", got, err)
	}
}

func engineeringFailurePreparationV1(t *testing.T) sdk.PrepareContextEvaluationRequestV1 {
	t.Helper()
	baseline, candidate, policy, outcomes := testkit.EngineeringOutcomesV1()
	request, err := sdk.SealPrepareContextEvaluationRequestV1(context.Background(), sdk.PrepareContextEvaluationRequestV1{
		Meta: engineeringFailureMetaV1(sdk.EngineeringPrepareEvaluationV1, "failure-evaluation-prepare"), EvaluationID: "failure-evaluation",
		EvaluatorRef: testkit.EngineeringEvaluatorRefV1(), Outcomes: outcomes, BaselineRecipeRef: baseline,
		CandidateRecipeRef: candidate, PolicyRef: policy, CheckedUnixNano: testkit.Now,
		NotAfterUnixNano: testkit.Now + int64(30*time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func engineeringFailureMetaV1(op sdk.ContextEngineeringOperationV1, id string) sdk.ContextEngineeringRequestMetaV1 {
	return sdk.ContextEngineeringRequestMetaV1{ContractVersion: sdk.ContextEngineeringSDKContractVersionV1, RequestID: id, Operation: op, Limits: sdk.DefaultContextEngineeringLimitsV1()}
}
