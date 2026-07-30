package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

// HostStartPackageSelectionBindingCurrentReaderV1 reads one exact immutable
// Host-owned association. It exposes no Claim or deployment mutation.
type HostStartPackageSelectionBindingCurrentReaderV1 interface {
	InspectHostStartPackageSelectionBindingV1(
		context.Context,
		contract.HostStartPackageSelectionBindingRefV1,
	) (contract.HostStartPackageSelectionBindingV1, error)
}

// HostStartPackageSelectionBindingForClaimReaderV1 resolves the sole immutable
// association for an already-inspected exact Claim. Callers cannot nominate a
// Binding or Deployment V2 Ref through this surface.
type HostStartPackageSelectionBindingForClaimReaderV1 interface {
	InspectHostStartPackageSelectionBindingForClaimV1(
		context.Context,
		contract.HostStartClaimRefV1,
	) (contract.HostStartPackageSelectionBindingV1, error)
}

type HostStartPackageSelectionBindingReadersV1 interface {
	HostStartPackageSelectionBindingCurrentReaderV1
	HostStartPackageSelectionBindingForClaimReaderV1
}

// HostStartClaimPackageSelectionPortV1 owns the one-commit ClaimV1 +
// InputV3-sidecar + immutable Package Selection association operation.
// ClaimOrInspectHostStartV3 intentionally remains a weaker, unchanged port.
type HostStartClaimPackageSelectionPortV1 interface {
	HostStartPackageSelectionBindingReadersV1
	ClaimOrInspectHostStartPackageSelectionV1(
		context.Context,
		contract.HostStartClaimV1,
		contract.HostStartClaimInputV3,
		contract.HostStartPackageSelectionBindingV1,
	) (contract.HostStartPackageSelectionBindingV1, error)
}
