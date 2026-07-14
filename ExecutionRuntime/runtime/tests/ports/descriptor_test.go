package ports_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestBoundCapabilityRequiresEvidence(t *testing.T) {
	t.Parallel()
	digest, err := core.DigestJSON("model-invoker")
	if err != nil {
		t.Fatal(err)
	}
	descriptor := ports.ComponentDescriptor{
		ID: "model-invoker", Kind: ports.ComponentModelInvoker, Version: "v1",
		ArtifactDigest: digest, ContractVersion: ports.ContractVersion,
		Capabilities: []ports.Capability{{Name: "invoke", State: ports.CapabilityBound}},
	}
	if err := descriptor.Validate(); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("bound capability without evidence must fail, got %v", err)
	}
	descriptor.Capabilities[0].EvidenceDigest = digest
	descriptor.Capabilities[0].EvidenceExpiry = time.Now().Add(time.Hour)
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("evidence-backed capability should validate: %v", err)
	}
}

func TestDuplicateCapabilityIsRejected(t *testing.T) {
	t.Parallel()
	digest, err := core.DigestJSON("harness")
	if err != nil {
		t.Fatal(err)
	}
	descriptor := ports.ComponentDescriptor{
		ID: "harness", Kind: ports.ComponentHarness, Version: "v1",
		ArtifactDigest: digest, ContractVersion: ports.ContractVersion,
		Capabilities: []ports.Capability{
			{Name: "run", State: ports.CapabilityDeclared},
			{Name: "run", State: ports.CapabilityDeclared},
		},
	}
	if err := descriptor.Validate(); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("duplicate capability must conflict, got %v", err)
	}
}
