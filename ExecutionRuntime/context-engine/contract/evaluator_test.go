package contract_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestEvaluationInputAndObservationSealV1(t *testing.T) {
	baseline, candidate, policy, outcomes := testkit.EngineeringOutcomesV1()
	refs := engineeringOutcomeRefsV1(t, outcomes)
	input, err := contract.SealContextEvaluationInputV1(contract.ContextEvaluationInputV1{
		EvaluationID: "evaluation-1", EvaluatorRef: testkit.EngineeringEvaluatorRefV1(), OutcomeRefs: []contract.FactRef{refs[1], refs[0]},
		BaselineRecipeRef: baseline, CandidateRecipeRef: candidate, PolicyRef: policy, CheckedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + int64(30*time.Second),
	})
	if err != nil || input.OutcomeRefs[0].ID != "outcome-baseline" {
		t.Fatalf("input seal drift: %#v %v", input, err)
	}
	observation := testkit.EngineeringObservationV1(input)
	if err := observation.Validate(); err != nil {
		t.Fatal(err)
	}
	drift := observation
	drift.InputDigest = testkit.D("other-input")
	if err := drift.Validate(); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("observation digest drift accepted: %v", err)
	}
}

func TestEvaluatorNominalAndInputFailClosedV1(t *testing.T) {
	if err := (contract.ContextEvaluatorRefV1{}).Validate(); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("zero evaluator ref accepted: %v", err)
	}
	ref := testkit.EngineeringRefV1("same")
	input := contract.ContextEvaluationInputV1{EvaluationID: "evaluation", EvaluatorRef: testkit.EngineeringEvaluatorRefV1(), OutcomeRefs: []contract.FactRef{testkit.EngineeringRefV1("outcome")}, BaselineRecipeRef: ref, CandidateRecipeRef: ref, PolicyRef: testkit.EngineeringRefV1("policy"), CheckedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 1}
	if _, err := contract.SealContextEvaluationInputV1(input); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("same baseline/candidate accepted: %v", err)
	}
}

func engineeringOutcomeRefsV1(t *testing.T, outcomes []contract.ContextOutcomeFactV1) []contract.FactRef {
	t.Helper()
	refs := make([]contract.FactRef, len(outcomes))
	for index, outcome := range outcomes {
		digest, err := outcome.DigestValue()
		if err != nil {
			t.Fatal(err)
		}
		refs[index] = contract.FactRef{ID: outcome.ID, Revision: outcome.Revision, Digest: digest}
	}
	return refs
}
