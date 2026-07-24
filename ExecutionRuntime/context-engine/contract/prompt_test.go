package contract_test

import (
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestPromptAssetV1SealsExactContentAndNominalRef(t *testing.T) {
	asset := testkit.PromptAssetV1()
	if err := asset.Validate(); err != nil {
		t.Fatal(err)
	}
	ref, err := asset.RefV1()
	if err != nil || ref.ID != asset.ID || ref.Revision != asset.Revision {
		t.Fatalf("prompt ref drift: %#v %v", ref, err)
	}
	roles := []struct {
		role  contract.PromptFragmentRoleV1
		kind  contract.FragmentKind
		trust contract.TrustClass
	}{
		{contract.PromptFragmentInstructionV1, contract.FragmentInstruction, contract.TrustAuthoritativeInstruction},
		{contract.PromptFragmentExampleV1, contract.FragmentConversation, contract.TrustRestrictedMaterial},
		{contract.PromptFragmentPolicyV1, contract.FragmentPolicySnapshot, contract.TrustRestrictedMaterial},
	}
	for _, item := range roles {
		kind, trust, err := item.role.KindAndTrustV1()
		if err != nil || kind != item.kind || trust != item.trust {
			t.Fatalf("role mapping drift: %q %q %q %v", item.role, kind, trust, err)
		}
	}
	if _, _, err := contract.PromptFragmentRoleV1("provider-system").KindAndTrustV1(); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("provider role string accepted: %v", err)
	}
}

func TestPromptAssetV1FailClosedMatrix(t *testing.T) {
	base := testkit.PromptAssetV1()
	tests := map[string]func(*contract.PromptAssetV1){
		"content_digest":        func(v *contract.PromptAssetV1) { v.ContentDigest = testkit.D("drift") },
		"fragment_order":        func(v *contract.PromptAssetV1) { v.Fragments[0], v.Fragments[1] = v.Fragments[1], v.Fragments[0] },
		"fragment_duplicate":    func(v *contract.PromptAssetV1) { v.Fragments[1].ID = v.Fragments[0].ID },
		"fragment_zero_content": func(v *contract.PromptAssetV1) { v.Fragments[0].Content.Length = 0 },
		"fragment_unknown_role": func(v *contract.PromptAssetV1) { v.Fragments[0].Role = "provider-system" },
		"evidence_not_closed":   func(v *contract.PromptAssetV1) { v.Fragments[0].Evidence = testkit.Evidence("outside") },
		"evidence_duplicate":    func(v *contract.PromptAssetV1) { v.Evidence[1] = v.Evidence[0] },
		"render_duplicate":      func(v *contract.PromptAssetV1) { v.RenderCompatibility[1] = v.RenderCompatibility[0] },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := base
			value.Fragments = append([]contract.PromptFragmentSpecV1(nil), base.Fragments...)
			value.Evidence = append([]contract.EvidenceRef(nil), base.Evidence...)
			value.RenderCompatibility = append([]contract.FactRef(nil), base.RenderCompatibility...)
			mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("invalid prompt asset accepted")
			}
		})
	}
}

func TestPromptBuildRequestV1DigestAndLifetime(t *testing.T) {
	request := testkit.PromptBuildRequestV1(testkit.PromptAssetV1())
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.Execution.Turn++
	if err := request.Validate(); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("request drift accepted: %v", err)
	}
	bad := testkit.PromptBuildRequestV1(testkit.PromptAssetV1())
	bad.NotAfterUnixNano = bad.CreatedUnixNano
	bad.RequestDigest = ""
	if _, err := contract.SealBuildPromptCandidatesRequestV1(bad); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("empty request lifetime accepted: %v", err)
	}
}

func TestPromptLifecycleNominalPresenceV1(t *testing.T) {
	assetRef := testkit.PromptAssetRefV1(testkit.PromptAssetV1())
	draft := promptLifecycleV1("prompt-draft", assetRef, nil, contract.ContextPromptDraftV1)
	if err := draft.Validate(); err != nil {
		t.Fatal(err)
	}
	digest, _ := draft.DigestValue()
	draftRef := contract.FactRef{ID: draft.ID, Revision: 1, Digest: digest}
	validated := promptLifecycleV1("prompt-validated", assetRef, &draftRef, contract.ContextPromptValidatedV1)
	validated.ValidationReportRef = promptRefPtrV1("prompt-validation")
	if err := validated.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := validated
	bad.EvaluationRef = promptRefPtrV1("premature")
	if err := bad.Validate(); err == nil {
		t.Fatal("validated prompt accepted premature evaluation")
	}
}

func promptLifecycleV1(id string, asset contract.PromptAssetRefV1, previous *contract.FactRef, state contract.ContextPromptLifecycleStateV1) contract.ContextPromptLifecycleFactV1 {
	return contract.ContextPromptLifecycleFactV1{ContractVersion: contract.Version, ID: id, Revision: 1, PromptAssetRef: asset, PreviousLifecycleRef: previous, State: state, Evidence: []contract.EvidenceRef{}, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 1_000}
}

func promptRefPtrV1(id string) *contract.FactRef {
	ref := contract.FactRef{ID: id, Revision: 1, Digest: testkit.D(id)}
	return &ref
}
