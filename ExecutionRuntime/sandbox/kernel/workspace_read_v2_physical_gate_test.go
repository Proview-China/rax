package kernel

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	ownerworkspaceread "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type rawWorkspaceReadActualPointV1 struct{ calls atomic.Uint64 }

func (p *rawWorkspaceReadActualPointV1) ReadWorkspaceFileV1(context.Context, WorkspaceReadActualPointRequestV1) (WorkspaceReadActualPointResultV1, error) {
	p.calls.Add(1)
	return WorkspaceReadActualPointResultV1{}, nil
}

type workspaceReadConstructorCommandsV2 struct {
	workspaceReadPublishedCommandCurrentReaderV2
}
type workspaceReadConstructorAssociationsV2 struct {
	runtimeports.PreparedDomainCommandAssociationCurrentReaderV1
}
type workspaceReadConstructorWorkspacesV2 struct {
	sandboxports.WorkspaceCurrentReaderV1
}
type workspaceReadConstructorSandboxCurrentV2 struct {
	runtimeports.OperationDispatchSandboxCurrentReaderV4
}
type workspaceReadConstructorEnforcementV2 struct {
	runtimeports.OperationDispatchEnforcementGovernancePortV4
}
type workspaceReadConstructorStoreV2 struct {
	workspaceReadAuthorizedOwnerStoreV2
	ownerworkspaceread.PostActualRepositoryV2
}

func TestWorkspaceReadPhysicalExecutorRejectsRawV1ActualPointBypass(t *testing.T) {
	t.Parallel()
	raw := &rawWorkspaceReadActualPointV1{}
	_, err := newWorkspaceReadPhysicalExecutorV1(
		&workspaceReadConstructorCommandsV2{},
		&workspaceReadConstructorAssociationsV2{},
		&workspaceReadConstructorWorkspacesV2{},
		&workspaceReadConstructorSandboxCurrentV2{},
		&workspaceReadConstructorEnforcementV2{},
		&workspaceReadConstructorStoreV2{},
		raw,
		time.Now,
	)
	if err == nil {
		t.Fatal("raw V1 actual point bypass was accepted")
	}
	if raw.calls.Load() != 0 {
		t.Fatalf("rejected raw V1 actual point was called %d times", raw.calls.Load())
	}
}

var _ WorkspaceReadActualPointV1 = (*rawWorkspaceReadActualPointV1)(nil)
