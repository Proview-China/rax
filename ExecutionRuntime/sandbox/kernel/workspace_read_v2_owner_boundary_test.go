package kernel

import (
	"testing"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type v1OnlyWorkspaceReadStore struct {
	sandboxports.WorkspaceReadOwnerStoreV1
}

type noopWorkspaceReadCommandReaderV1 struct {
	sandboxports.WorkspaceReadCommandCurrentReaderV1
}

type noopWorkspaceCurrentReaderV1 struct {
	sandboxports.WorkspaceCurrentReaderV1
}

type noopWorkspaceReadAssociationReaderV1 struct {
	runtimeports.PreparedDomainCommandAssociationCurrentReaderV1
}

type noopWorkspaceReadSandboxReaderV1 struct {
	runtimeports.OperationDispatchSandboxCurrentReaderV4
}

type noopWorkspaceReadEnforcementV4 struct {
	runtimeports.OperationDispatchEnforcementGovernancePortV4
}

func TestWorkspaceReadPhysicalExecutorV1RejectsV1OnlyOwnerStore(t *testing.T) {
	var rawOnly any = &noopWorkspaceReadCommandReaderV1{}
	if _, ok := rawOnly.(sandboxports.WorkspaceReadPublishedCommandCurrentReaderV2); ok {
		t.Fatal("V1-only Command reader satisfied the published-current gate")
	}
}

var _ sandboxports.WorkspaceReadOwnerStoreV1 = (*v1OnlyWorkspaceReadStore)(nil)
var _ sandboxports.WorkspaceReadCommandCurrentReaderV1 = (*noopWorkspaceReadCommandReaderV1)(nil)
