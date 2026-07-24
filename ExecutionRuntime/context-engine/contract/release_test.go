package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestRecipeLifecyclePresenceAndStatesV1(t *testing.T) {
	recipe := testkit.Recipe()
	recipeDigest, _ := recipe.DigestValue()
	recipeRef := contract.FactRef{ID: recipe.ID, Revision: recipe.Revision, Digest: recipeDigest}
	draft := lifecycleFactV1("draft-1", recipeRef, nil, contract.ContextRecipeDraftV1)
	if err := draft.Validate(); err != nil {
		t.Fatal(err)
	}
	draftDigest, _ := draft.DigestValue()
	draftRef := contract.FactRef{ID: draft.ID, Revision: 1, Digest: draftDigest}
	validated := lifecycleFactV1("validated-1", recipeRef, &draftRef, contract.ContextRecipeValidatedV1)
	validated.ValidationReportRef = releaseRefV1("validation-report")
	if err := validated.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := validated
	bad.EvaluationRef = releaseRefV1("premature-evaluation")
	if err := bad.Validate(); err == nil {
		t.Fatal("validated state accepted evaluation ref")
	}
	bad = draft
	bad.PreviousLifecycleRef = &draftRef
	if err := bad.Validate(); err == nil {
		t.Fatal("draft accepted previous lifecycle")
	}
}

func lifecycleFactV1(id string, recipe contract.FactRef, previous *contract.FactRef, state contract.ContextRecipeLifecycleStateV1) contract.ContextRecipeLifecycleFactV1 {
	return contract.ContextRecipeLifecycleFactV1{ContractVersion: contract.Version, ID: id, Revision: 1, RecipeRef: recipe, PreviousLifecycleRef: previous, State: state, Evidence: []contract.EvidenceRef{}, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + int64(time.Minute)}
}

func releaseRefV1(id string) *contract.FactRef {
	value := contract.FactRef{ID: id, Revision: 1, Digest: testkit.D(id)}
	return &value
}
