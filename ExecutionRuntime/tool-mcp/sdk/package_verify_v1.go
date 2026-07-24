package sdk

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

type PackageVerificationPortV1 interface {
	VerifyV1(context.Context, toolcontract.ToolPackageVerifyRequestV1) (toolcontract.ToolPackageVerificationFactV1, error)
	ResolveCurrentToolPackageVerificationV1(context.Context, toolcontract.ToolPackageVerificationCurrentIssuanceV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error)
	InspectCurrentToolPackageVerificationV1(context.Context, toolcontract.ToolPackageVerificationCurrentRefV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error)
}

type PackageVerificationHistoryReaderV1 interface {
	InspectExactToolPackageVerificationObservationV1(context.Context, toolcontract.ToolPackageVerificationObservationRefV1) (toolcontract.ToolPackageVerificationObservationV1, error)
	InspectExactToolPackageVerificationFactV1(context.Context, toolcontract.ToolPackageVerificationFactRefV1) (toolcontract.ToolPackageVerificationFactV1, error)
}

type PackageVerifiedAdmissionPortV1 interface {
	AdmitPackageV1(context.Context, toolcontract.ToolPackageAdmissionCommandV1) (registry.Record, error)
}

// PackageVerificationV1 exposes only already-sealed exact Package
// verification coordinates. It has no URL, tag, Registry client, raw key,
// credential, --insecure, Fetch, Install or Enable input.
type PackageVerificationV1 struct {
	verification PackageVerificationPortV1
	history      PackageVerificationHistoryReaderV1
	admission    PackageVerifiedAdmissionPortV1
}

func NewPackageVerificationV1(verification PackageVerificationPortV1, history PackageVerificationHistoryReaderV1, admission PackageVerifiedAdmissionPortV1) (*PackageVerificationV1, error) {
	if nilLikeV1(verification) || nilLikeV1(history) || nilLikeV1(admission) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Package Verification SDK dependencies are required")
	}
	return &PackageVerificationV1{verification: verification, history: history, admission: admission}, nil
}

func (s *PackageVerificationV1) VerifyPackageV1(ctx context.Context, request toolcontract.ToolPackageVerifyRequestV1) (toolcontract.ToolPackageVerificationFactV1, error) {
	if err := readyPackageVerificationSDKV1(ctx, s); err != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, err
	}
	return s.verification.VerifyV1(ctx, request)
}

func (s *PackageVerificationV1) ResolvePackageVerificationCurrentV1(ctx context.Context, issuance toolcontract.ToolPackageVerificationCurrentIssuanceV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
	if err := readyPackageVerificationSDKV1(ctx, s); err != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, err
	}
	return s.verification.ResolveCurrentToolPackageVerificationV1(ctx, issuance)
}

func (s *PackageVerificationV1) InspectPackageVerificationObservationV1(ctx context.Context, exact toolcontract.ToolPackageVerificationObservationRefV1) (toolcontract.ToolPackageVerificationObservationV1, error) {
	if err := readyPackageVerificationSDKV1(ctx, s); err != nil {
		return toolcontract.ToolPackageVerificationObservationV1{}, err
	}
	return s.history.InspectExactToolPackageVerificationObservationV1(ctx, exact)
}

func (s *PackageVerificationV1) InspectPackageVerificationFactV1(ctx context.Context, exact toolcontract.ToolPackageVerificationFactRefV1) (toolcontract.ToolPackageVerificationFactV1, error) {
	if err := readyPackageVerificationSDKV1(ctx, s); err != nil {
		return toolcontract.ToolPackageVerificationFactV1{}, err
	}
	return s.history.InspectExactToolPackageVerificationFactV1(ctx, exact)
}

func (s *PackageVerificationV1) InspectPackageVerificationCurrentV1(ctx context.Context, exact toolcontract.ToolPackageVerificationCurrentRefV1) (toolcontract.ToolPackageVerificationCurrentProjectionV1, error) {
	if err := readyPackageVerificationSDKV1(ctx, s); err != nil {
		return toolcontract.ToolPackageVerificationCurrentProjectionV1{}, err
	}
	return s.verification.InspectCurrentToolPackageVerificationV1(ctx, exact)
}

func (s *PackageVerificationV1) AdmitVerifiedPackageV1(ctx context.Context, command toolcontract.ToolPackageAdmissionCommandV1) (registry.Record, error) {
	if err := readyPackageVerificationSDKV1(ctx, s); err != nil {
		return registry.Record{}, err
	}
	return s.admission.AdmitPackageV1(ctx, command)
}

func readyPackageVerificationSDKV1(ctx context.Context, s *PackageVerificationV1) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Package Verification SDK context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || nilLikeV1(s.verification) || nilLikeV1(s.history) || nilLikeV1(s.admission) {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Package Verification SDK is unavailable")
	}
	return nil
}
