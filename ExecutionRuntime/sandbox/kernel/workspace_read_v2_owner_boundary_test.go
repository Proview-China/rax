package kernel

import (
	"testing"
	"time"

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
	_, err := NewWorkspaceReadPhysicalExecutorV1(
		&noopWorkspaceReadCommandReaderV1{},
		&noopWorkspaceReadAssociationReaderV1{},
		&noopWorkspaceCurrentReaderV1{},
		&noopWorkspaceReadSandboxReaderV1{},
		&noopWorkspaceReadEnforcementV4{},
		&v1OnlyWorkspaceReadStore{},
		&countingWorkspaceReadActualPointV2{},
		time.Now,
	)
	if err == nil {
		t.Fatal("V1-only owner store exposed a physical execution bypass")
	}
}
