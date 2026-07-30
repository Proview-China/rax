package ports

import (
	"context"

	builderports "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// HostDeploymentResourceCurrentReaderV2 is deliberately narrower than the
// Runtime resource Owner. It exposes no create, update, cleanup or settlement
// operation to the Host deployment Owner.
type HostDeploymentResourceCurrentReaderV2 interface {
	InspectResourceHandleCurrentV1(context.Context, runtimeports.ResourceHandleRefV1) (runtimeports.ResourceHandleCurrentV1, error)
}

// HostDeploymentServiceCurrentReaderV2 is a read-only exact projection seam.
// Its first implementation may be fixture-only; it is not a production service
// registry or readiness grant.
type HostDeploymentServiceCurrentReaderV2 interface {
	InspectHostServiceBindingCurrentV2(context.Context, contract.HostServiceBindingRefV1) (contract.HostServiceBindingCurrentV2, error)
}

// HostDeploymentCurrentRepositoryV2 is the private durable Host Owner seam.
// Method names are intentionally different from the public Reader so a raw
// SQLite Store cannot accidentally satisfy the public currentness boundary.
type HostDeploymentCurrentRepositoryV2 interface {
	InspectStoredHostDeploymentExactV2(context.Context, contract.HostDeploymentCurrentRefV2) (contract.HostDeploymentCurrentV2, error)
	InspectStoredHostDeploymentCurrentV2(context.Context, string, string) (contract.HostDeploymentCurrentV2, error)
	CompareAndSwapStoredHostDeploymentCurrentV2(
		context.Context,
		contract.HostDeploymentCurrentRefV2,
		contract.HostDeploymentCurrentV2,
	) (contract.HostDeploymentCurrentV2, error)
}

// HostDeploymentCurrentReaderV2 always revalidates Builder selection,
// verified closure and resource/service currentness. It cannot be implemented
// by a raw persistence Store.
type HostDeploymentCurrentReaderV2 interface {
	InspectHostDeploymentCurrentV2(context.Context, contract.HostDeploymentCurrentRefV2) (contract.HostDeploymentCurrentV2, error)
}

type HostDeploymentCurrentOwnerV2 interface {
	HostDeploymentCurrentReaderV2
	PublishHostDeploymentCurrentV2(
		context.Context,
		contract.PublishHostDeploymentCurrentRequestV2,
	) (contract.HostDeploymentCurrentV2, error)
}

// HostDeploymentBuilderEvidenceV2 names the two Builder-owned read surfaces
// required by the Host. The Host may derive and compare package lineage but
// may persist only AgentPackageSelectionCurrentRefV1.
type HostDeploymentBuilderEvidenceV2 interface {
	builderports.AgentPackageSelectionCurrentReaderV1
	builderports.VerifiedAgentPackageClosureReaderV1
}
