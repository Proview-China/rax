package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
)

// VerifiedAgentPackageClosureReaderV1 must reread both exact Owner records and
// verify their full immutable closure. It is intentionally read-only.
type VerifiedAgentPackageClosureReaderV1 interface {
	LoadVerifiedAgentPackageClosureV1(context.Context, contract.AgentPackageRefV1) (contract.VerifiedAgentPackageClosureV1, error)
}

type AgentPackageSelectionCurrentReaderV1 interface {
	InspectAgentPackageSelectionCurrentV1(context.Context, string) (contract.AgentPackageSelectionCurrentV1, error)
	InspectAgentPackageSelectionExactV1(context.Context, contract.AgentPackageSelectionCurrentRefV1) (contract.AgentPackageSelectionCurrentV1, error)
}

// AgentPackageSelectionCurrentRepositoryV1 owns the append-only history and the
// single current CAS pointer for a selection ID.
type AgentPackageSelectionCurrentRepositoryV1 interface {
	AgentPackageSelectionCurrentReaderV1
	CompareAndSwapAgentPackageSelectionCurrentV1(
		context.Context,
		contract.AgentPackageSelectionCurrentRefV1,
		contract.AgentPackageSelectionCurrentV1,
	) (contract.AgentPackageSelectionCurrentV1, error)
}
