package ports

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

func TestCloneWorkspaceReadAdmissionAttemptBindingV2DoesNotAliasExpectedFileRef(t *testing.T) {
	fixture := workspaceReadAdmissionAttemptBindingV2ShapeFixture(t)
	inputExpected := fixture.WorkspaceReadCommand.ExpectedFileRef
	cloned := cloneWorkspaceReadAdmissionAttemptBindingV2(fixture)
	if cloned.WorkspaceReadCommand.ExpectedFileRef == inputExpected {
		t.Fatal("clone retained the caller's ExpectedFileRef pointer")
	}
	original := *cloned.WorkspaceReadCommand.ExpectedFileRef
	inputExpected.ID = "caller-mutated"
	if *cloned.WorkspaceReadCommand.ExpectedFileRef != original {
		t.Fatal("caller mutation changed the cloned V2 value")
	}
	cloned.WorkspaceReadCommand.ExpectedFileRef.ID = "output-mutated"
	if inputExpected.ID == "output-mutated" {
		t.Fatal("cloned output mutation changed caller input")
	}
}

func workspaceReadAdmissionAttemptBindingV2ShapeFixture(t *testing.T) WorkspaceReadAdmissionAttemptBindingV2 {
	t.Helper()
	// This fixture only proves canonical cloning; Seal is not authority and
	// cannot create an owner fact or execution eligibility.
	expected := contract.Ref{
		ID: "file", Revision: 1,
		Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	value := WorkspaceReadAdmissionAttemptBindingV2{
		ContractVersion: WorkspaceReadAdmissionAttemptBindingContractVersionV2,
		TypeURL:         WorkspaceReadAdmissionAttemptBindingTypeURLV2,
	}
	// Build from a valid fixture by using the canonical helper's own clone path.
	// The remaining fields are filled by a storage-backed integration fixture;
	// this test calls clone directly because invalid shape must not obscure the
	// alias property under test.
	value.WorkspaceReadCommand.ExpectedFileRef = &expected
	value.RuntimeAttempt.Delegation = nil
	return value
}
