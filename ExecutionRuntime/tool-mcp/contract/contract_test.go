package contract_test

import (
	"testing"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestStableIDAndDigestAreDeterministic(t *testing.T) {
	first, err := contract.StableID("action", "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	second, err := contract.StableID("action", "a", "b")
	if err != nil || first != second {
		t.Fatalf("stable IDs differ: %q %q %v", first, second, err)
	}
	capability := testkit.Capability()
	if err := capability.Validate(); err != nil {
		t.Fatal(err)
	}
	mutated := capability
	mutated.Risk = contract.RiskHigh
	if err := mutated.Validate(); err == nil {
		t.Fatal("mutated sealed capability was accepted")
	}
}

func TestToolCannotWeakenCapabilityEffects(t *testing.T) {
	capability := testkit.Capability()
	capability.EffectKinds = []runtimeports.NamespacedNameV2{"praxis.tool/cancel", "praxis.tool/execute"}
	var err error
	capability, err = contract.SealCapability(capability)
	if err != nil {
		t.Fatal(err)
	}
	tool := testkit.Tool()
	tool.Capability = contract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
	tool, err = contract.SealTool(tool)
	if err != nil {
		t.Fatal(err)
	}
	if err := tool.ValidateAgainst(capability); err == nil {
		t.Fatal("tool that omits required cancel effect was accepted")
	}
}

func TestPackageAndActionContractsValidate(t *testing.T) {
	if err := testkit.Package().Validate(); err != nil {
		t.Fatal(err)
	}
	candidate := testkit.Candidate()
	if err := candidate.Validate(); err != nil {
		t.Fatal(err)
	}
	candidate.ConflictDomain = "tenant/other"
	if err := candidate.Validate(); err == nil {
		t.Fatal("candidate digest drift was accepted")
	}
}

func TestActionCandidateRequiresPendingActionDigest(t *testing.T) {
	candidate := testkit.Candidate()
	candidate.PendingActionDigest = ""
	candidate.Digest = ""
	if _, err := contract.SealActionCandidate(candidate); err == nil {
		t.Fatal("ActionCandidate without Harness PendingAction RequestDigest was sealed")
	}
}

func FuzzDecodeStableID(f *testing.F) {
	f.Add("valid_part")
	f.Add("")
	f.Fuzz(func(t *testing.T, value string) {
		id, err := contract.StableID("test", value)
		if err == nil && contract.ValidateStableID(id) != nil {
			t.Fatalf("StableID returned invalid id %q", id)
		}
	})
}
