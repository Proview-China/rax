package kernel

import (
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// NewWorkspaceReadPhysicalExecutorForTestV1 exposes dependency injection only
// in the go test build. Production has only NewWorkspaceReadPhysicalExecutorV1,
// which constructs the private Kernel-to-Data-Plane bridge itself.
func NewWorkspaceReadPhysicalExecutorForTestV1(
	commands workspaceReadPublishedCommandCurrentReaderV2,
	associations runtimeports.PreparedDomainCommandAssociationCurrentReaderV1,
	workspaces sandboxports.WorkspaceCurrentReaderV1,
	sandboxCurrent runtimeports.OperationDispatchSandboxCurrentReaderV4,
	enforcement runtimeports.OperationDispatchEnforcementGovernancePortV4,
	store sandboxports.WorkspaceReadOwnerStoreV1,
	actualPoint WorkspaceReadActualPointV1,
	clock func() time.Time,
) (*WorkspaceReadPhysicalExecutorV1, error) {
	return newWorkspaceReadPhysicalExecutorV1(
		commands, associations, workspaces, sandboxCurrent, enforcement,
		store, actualPoint, clock,
	)
}

func NewWorkspaceReadActualPointAdapterForTestV2(
	client dataplaneadapter.Client,
) (WorkspaceReadActualPointV1, error) {
	return newWorkspaceReadActualPointAdapterV2(client)
}

func AuthorizeWorkspaceReadPhysicalJournalEvidenceForTestV2(
	journal contract.WorkspaceReadPhysicalJournalRefV2,
) (WorkspaceReadPhysicalJournalEvidenceForTestV2, error) {
	return authorizeWorkspaceReadPhysicalJournalEvidenceV2(journal)
}

type WorkspaceReadPhysicalJournalEvidenceForTestV2 = workspaceReadPhysicalJournalEvidenceV2
