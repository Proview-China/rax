package packageverify

import (
	"context"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

// AdmissionV1 binds the Tool-owned verification service to the Tool Registry's
// strong verification-aware Admission port. It is production-neutral
// composition, not a production backend or Package Enable implementation.
type AdmissionV1 struct {
	service *ServiceV1
	target  VerifiedPackageAdmissionRegistryV1
}

func NewAdmissionV1(service *ServiceV1, target VerifiedPackageAdmissionRegistryV1) (*AdmissionV1, error) {
	if service == nil || isNilV1(target) {
		return nil, packageInvalidV1("Package Admission dependencies are nil")
	}
	return &AdmissionV1{service: service, target: target}, nil
}

func (a *AdmissionV1) AdmitPackageV1(ctx context.Context, command toolcontract.ToolPackageAdmissionCommandV1) (registry.Record, error) {
	if a == nil || a.service == nil || isNilV1(a.target) {
		return registry.Record{}, packageInvalidV1("Package Admission is unavailable")
	}
	return a.service.AdmitPackageV1(ctx, a.target, command)
}
