package contract_test

import (
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestContextRecipeComparisonContractRejectsDuplicatePathV1(t *testing.T) {
	recipe := testkit.Recipe()
	digest, err := recipe.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	before, after := testkit.D("before"), testkit.D("after")
	comparison := contract.ContextRecipeComparisonV1{
		ContractVersion: contract.Version,
		BaseRecipeRef:   contract.FactRef{ID: recipe.ID, Revision: recipe.Revision, Digest: digest},
		CandidateRef:    contract.FactRef{ID: "candidate", Revision: 2, Digest: testkit.D("candidate")},
		Changes: []contract.ContextRecipeChangeV1{
			{FieldPath: "budget", Kind: contract.ContextRecipeChangeModifiedV1, BeforeDigest: &before, AfterDigest: &after},
			{FieldPath: "budget", Kind: contract.ContextRecipeChangeReorderedV1, BeforeDigest: &before, AfterDigest: &after},
		},
		CheckedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 1,
		ComparisonDigest: testkit.D("comparison"),
	}
	if err := comparison.Validate(); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("want duplicate-path conflict, got %v", err)
	}
}

func TestContextRecipeChangePresenceV1(t *testing.T) {
	digest := testkit.D("value")
	for _, change := range []contract.ContextRecipeChangeV1{
		{FieldPath: "added", Kind: contract.ContextRecipeChangeAddedV1, BeforeDigest: &digest},
		{FieldPath: "removed", Kind: contract.ContextRecipeChangeRemovedV1, AfterDigest: &digest},
		{FieldPath: "modified", Kind: contract.ContextRecipeChangeModifiedV1, BeforeDigest: &digest, AfterDigest: &digest},
	} {
		if err := change.Validate(); !errors.Is(err, contract.ErrConflict) {
			t.Fatalf("want presence conflict for %#v, got %v", change, err)
		}
	}
}
