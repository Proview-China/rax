package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

// HostV3 is the production-facing declarative lifecycle surface. It is
// additive; HostV1 and the injectable HostV2 reference coordinator remain
// separate and share only the permanent HostStart conflict domain.
type HostV3 interface {
	StartV3(context.Context, contract.StartRequestV3) (contract.StartResultV3, error)
	InspectV3(context.Context, contract.InspectRequestV3) (contract.InspectResultV3, error)
	StopV3(context.Context, contract.StopRequestV3) (contract.StopResultV3, error)
}

// HostV3CurrentReaderV1 is the read-only Host lifecycle surface consumed by
// actual-point adapters. It deliberately cannot Start or Stop a Host.
type HostV3CurrentReaderV1 interface {
	InspectV3(context.Context, contract.InspectRequestV3) (contract.InspectResultV3, error)
}

// HostDeploymentCurrentReaderV1 is read-only. Opening, migrating and closing
// resource handles remains the Deployment/Bootstrap Owner's responsibility.
type HostDeploymentCurrentReaderV1 interface {
	InspectHostDeploymentCurrentV1(context.Context, contract.HostDeploymentCurrentRefV1) (contract.HostDeploymentCurrentV1, error)
}

// HostStartClaimInputCurrentReaderV3 reads the exact V3 sidecar associated
// with the version-neutral HostStart Claim. It cannot create or authorize a
// lifecycle Start by itself.
type HostStartClaimInputCurrentReaderV3 interface {
	InspectHostStartClaimInputV3(context.Context, contract.HostStartClaimRefV1) (contract.HostStartClaimInputBindingV3, error)
}

// HostStartClaimPortV3 owns the single transaction/linear point that creates
// HostStartClaimV1 and its InputV3 sidecar. It embeds V1 so V1/V2/V3 contend on
// the same HostID+StartID key and permanent conflict domain.
type HostStartClaimPortV3 interface {
	HostStartClaimPortV1
	HostStartClaimInputCurrentReaderV3
	ClaimOrInspectHostStartV3(context.Context, contract.HostStartClaimV1, contract.HostStartClaimInputV3) (contract.HostStartClaimInputBindingV3, error)
}

// HostV3OwnerPipeline is the narrow post-Claim seam. A production adapter may
// compose the V2 Owner stages only after an additive after-Claim refactor; it
// must never manufacture CleanupClosure, Ready or Availability coordinates.
// StartOrInspect may dispatch only while it owns the durable operation intent;
// Inspect methods are permanently read-only recovery surfaces.
type HostV3OwnerPipeline interface {
	StartOrInspectHostV3(context.Context, contract.StartRequestV3, contract.HostStartClaimInputBindingV3, contract.HostJournalV2) (contract.HostV3OwnerStartProjectionV1, error)
	InspectHostV3(context.Context, contract.HostStartClaimInputBindingV3, contract.HostJournalV2) (contract.HostV3OwnerStartProjectionV1, error)
	StopOrInspectHostV3(context.Context, contract.StopRequestV3, contract.HostStartClaimInputBindingV3, contract.HostJournalV2) (contract.HostV3OwnerStopProjectionV1, error)
	InspectStopHostV3(context.Context, contract.StopRequestV3, contract.HostStartClaimInputBindingV3, contract.HostJournalV2) (contract.HostV3OwnerStopProjectionV1, error)
}
