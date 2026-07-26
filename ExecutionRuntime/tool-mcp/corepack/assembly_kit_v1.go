package corepack

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

const (
	CorePackAssemblyContractVersionV1 = "praxis.tool-mcp.core-pack-assembly/v1"
	CorePackAssemblyPreviewV1         = CorePackAssemblyModeV1("reference_preview")
	CorePackAssemblyAdmittedV1        = CorePackAssemblyModeV1("admitted")
	CorePackUnsupportedReasonV1       = "praxis.core-tool/provider-not-bound-v1"
)

type CorePackAssemblyModeV1 string

type CorePackAssemblyRequestV1 struct {
	ContractVersion          string                                                `json:"contract_version"`
	Mode                     CorePackAssemblyModeV1                                `json:"mode"`
	Catalog                  CatalogConfigV1                                       `json:"catalog"`
	Surface                  SurfaceConfigV1                                       `json:"surface"`
	VerificationRequest      toolcontract.ToolPackageVerifyRequestV1               `json:"verification_request,omitempty"`
	VerificationIssuance     toolcontract.ToolPackageVerificationCurrentIssuanceV1 `json:"verification_issuance,omitempty"`
	AdmissionCommand         toolcontract.ToolPackageAdmissionCommandV1            `json:"admission_command,omitempty"`
	RequestedExpiresUnixNano int64                                                 `json:"requested_expires_unix_nano"`
}

type CorePackAssemblyResultV1 struct {
	ContractVersion   string                           `json:"contract_version"`
	Mode              CorePackAssemblyModeV1           `json:"mode"`
	Catalog           CatalogV1                        `json:"catalog"`
	RegistrySnapshot  sdk.RegistrySnapshotRefV1        `json:"registry_snapshot"`
	PackageRecord     registry.Record                  `json:"package_record"`
	PackageAssembly   *sdk.PackageAssemblyV1           `json:"package_assembly,omitempty"`
	Surface           toolcontract.ToolSurfaceManifest `json:"surface"`
	ReferenceOnly     bool                             `json:"reference_only"`
	Admitted          bool                             `json:"admitted"`
	Executable        bool                             `json:"executable"`
	UnsupportedReason string                           `json:"unsupported_reason"`
	ExpiresUnixNano   int64                            `json:"expires_unix_nano"`
	Digest            core.Digest                      `json:"digest"`
}

type CorePackAssemblyKitV1 struct {
	registry     *registry.Registry
	registrySDK  *sdk.SDKV1
	verification *sdk.PackageVerificationV1
	clock        sdk.ClockV1
}

// CorePackAssemblyFactoryV1 is an owner-local declarative factory. It exposes
// assembly only; deliberately no executable Tool provider can be resolved from
// it in V1.
type CorePackAssemblyFactoryV1 struct{ kit *CorePackAssemblyKitV1 }

func NewCorePackAssemblyFactoryV1(kit *CorePackAssemblyKitV1) (*CorePackAssemblyFactoryV1, error) {
	if kit == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Core Pack Assembly Kit is required")
	}
	return &CorePackAssemblyFactoryV1{kit: kit}, nil
}

func (f *CorePackAssemblyFactoryV1) BuildV1(ctx context.Context, request CorePackAssemblyRequestV1) (CorePackAssemblyResultV1, error) {
	if f == nil || f.kit == nil {
		return CorePackAssemblyResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Core Pack Assembly Factory is unavailable")
	}
	return f.kit.AssembleV1(ctx, request)
}

func NewCorePackAssemblyKitV1(target *registry.Registry, registrySDK *sdk.SDKV1, verification *sdk.PackageVerificationV1, clock sdk.ClockV1) (*CorePackAssemblyKitV1, error) {
	if target == nil || registrySDK == nil || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Core Pack Assembly Kit dependencies are required")
	}
	return &CorePackAssemblyKitV1{registry: target, registrySDK: registrySDK, verification: verification, clock: clock}, nil
}

func (k *CorePackAssemblyKitV1) AssembleV1(ctx context.Context, request CorePackAssemblyRequestV1) (CorePackAssemblyResultV1, error) {
	now, err := k.readyV1(ctx)
	if err != nil {
		return CorePackAssemblyResultV1{}, err
	}
	if err = request.validateCurrentV1(now); err != nil {
		return CorePackAssemblyResultV1{}, err
	}
	catalog, err := BuildCatalogV1(request.Catalog)
	if err != nil {
		return CorePackAssemblyResultV1{}, err
	}
	packageRecord, err := RegisterV1(k.registry, catalog, now)
	if err != nil {
		return CorePackAssemblyResultV1{}, err
	}
	var assembly *sdk.PackageAssemblyV1
	if request.Mode == CorePackAssemblyPreviewV1 {
		if packageRecord.State != registry.StateSubmitted {
			return CorePackAssemblyResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "preview requires a submitted Package")
		}
	} else {
		packageRecord, err = k.admitV1(ctx, now, catalog, request)
		if err != nil {
			return CorePackAssemblyResultV1{}, err
		}
	}
	s1, err := k.registrySDK.InspectRegistrySnapshotV1(ctx)
	if err != nil {
		return CorePackAssemblyResultV1{}, err
	}
	snapshotRef := sdk.RegistrySnapshotRefV1{Revision: s1.Revision, Digest: s1.Digest}
	if request.Mode == CorePackAssemblyAdmittedV1 {
		resolved, resolveErr := k.registrySDK.ResolvePackageForAssemblyV1(ctx, catalog.Package.ID, snapshotRef)
		if resolveErr != nil {
			return CorePackAssemblyResultV1{}, resolveErr
		}
		assembly = &resolved
	}
	surfaceConfig := request.Surface
	surfaceConfig.RegistrySnapshotDigest = s1.Digest
	surface, err := BuildSurfaceV1(catalog, surfaceConfig)
	if err != nil {
		return CorePackAssemblyResultV1{}, err
	}
	s2, err := k.registrySDK.InspectRegistrySnapshotV1(ctx)
	if err != nil {
		return CorePackAssemblyResultV1{}, err
	}
	if s1.Revision != s2.Revision || s1.Digest != s2.Digest {
		return CorePackAssemblyResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Registry Snapshot changed during Core Pack assembly")
	}
	finished, err := k.readyV1(ctx)
	if err != nil {
		return CorePackAssemblyResultV1{}, err
	}
	if !finished.Before(time.Unix(0, request.RequestedExpiresUnixNano)) {
		return CorePackAssemblyResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Core Pack assembly expired before publication")
	}
	result := CorePackAssemblyResultV1{
		ContractVersion: CorePackAssemblyContractVersionV1, Mode: request.Mode,
		Catalog: catalog.Clone(), RegistrySnapshot: snapshotRef, PackageRecord: packageRecord,
		PackageAssembly: assembly, Surface: surface, ReferenceOnly: request.Mode == CorePackAssemblyPreviewV1,
		Admitted: request.Mode == CorePackAssemblyAdmittedV1, Executable: false,
		UnsupportedReason: CorePackUnsupportedReasonV1, ExpiresUnixNano: minExpiryV1(request.RequestedExpiresUnixNano, surface.ExpiresUnixNano),
	}
	if request.Mode == CorePackAssemblyAdmittedV1 {
		result.ExpiresUnixNano = minExpiryV1(result.ExpiresUnixNano, request.VerificationIssuance.RequestedExpiresUnixNano)
	}
	return sealCorePackAssemblyResultV1(result)
}

func (k *CorePackAssemblyKitV1) admitV1(ctx context.Context, now time.Time, catalog CatalogV1, request CorePackAssemblyRequestV1) (registry.Record, error) {
	if k.verification == nil {
		return registry.Record{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "admitted assembly requires Package Verification")
	}
	fact, err := k.verification.VerifyPackageV1(ctx, request.VerificationRequest)
	if err != nil {
		return registry.Record{}, err
	}
	if fact.Ref != request.VerificationIssuance.Fact || fact.Package.ID != string(catalog.Package.ID) || fact.Package.Revision != catalog.Package.Revision || fact.Package.Digest != catalog.Package.Digest {
		return registry.Record{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Package verification Fact differs from the Core Pack")
	}
	current, err := k.verification.ResolvePackageVerificationCurrentV1(ctx, request.VerificationIssuance)
	if err != nil {
		return registry.Record{}, err
	}
	inspected, err := k.verification.InspectPackageVerificationCurrentV1(ctx, current.Ref)
	if err != nil {
		return registry.Record{}, err
	}
	if inspected.Ref != current.Ref || inspected.ProjectionDigest != current.ProjectionDigest || inspected.ValidateCurrent(current.Ref, now) != nil || request.AdmissionCommand.VerificationCurrent != current.Ref || request.AdmissionCommand.ExpectedRegistryRevision != current.CurrentPackageRegistry.RegistryRevision {
		return registry.Record{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Package verification/current/admission closure drifted")
	}
	record, err := k.verification.AdmitVerifiedPackageV1(ctx, request.AdmissionCommand)
	if err != nil {
		if !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
			return registry.Record{}, err
		}
		_, record, err = k.registrySDK.InspectPackageV1(ctx, fact.Package)
		if err != nil {
			return registry.Record{}, err
		}
	}
	manifest, exact, err := k.registrySDK.InspectPackageV1(ctx, fact.Package)
	if err != nil {
		return registry.Record{}, err
	}
	if manifest.Digest != catalog.Package.Digest || exact.State != registry.StateActive || record.ObjectDigest != exact.ObjectDigest {
		return registry.Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "verified Core Pack is not active and exact")
	}
	return exact, nil
}

func (r CorePackAssemblyRequestV1) validateCurrentV1(now time.Time) error {
	if r.ContractVersion != CorePackAssemblyContractVersionV1 || now.IsZero() || r.RequestedExpiresUnixNano <= now.UnixNano() || r.Surface.ExpiresAt.UnixNano() > r.RequestedExpiresUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingExpired, "Core Pack assembly request is invalid or expired")
	}
	if r.Mode != CorePackAssemblyPreviewV1 && r.Mode != CorePackAssemblyAdmittedV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Core Pack assembly mode is invalid")
	}
	if r.Mode == CorePackAssemblyAdmittedV1 {
		if r.VerificationRequest.ValidateCurrent(now) != nil || r.VerificationIssuance.ValidateCurrent(now) != nil || r.AdmissionCommand.Validate() != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "admitted Core Pack coordinates are invalid")
		}
	}
	return nil
}

func (k *CorePackAssemblyKitV1) readyV1(ctx context.Context) (time.Time, error) {
	if ctx == nil {
		return time.Time{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Core Pack Assembly context is required")
	}
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	if k == nil || k.registry == nil || k.registrySDK == nil || k.clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Core Pack Assembly Kit is unavailable")
	}
	now := k.clock().UTC()
	if now.IsZero() {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Core Pack Assembly clock is invalid")
	}
	return now, nil
}

func sealCorePackAssemblyResultV1(result CorePackAssemblyResultV1) (CorePackAssemblyResultV1, error) {
	result = result.Clone()
	result.Digest = ""
	digest, err := toolcontract.Seal("praxis.tool-mcp.core-pack-assembly", CorePackAssemblyContractVersionV1, "CorePackAssemblyResultV1", result)
	if err != nil {
		return CorePackAssemblyResultV1{}, err
	}
	result.Digest = digest
	return result, result.Validate()
}

func (r CorePackAssemblyResultV1) Validate() error {
	if r.ContractVersion != CorePackAssemblyContractVersionV1 || r.Catalog.Validate() != nil || r.RegistrySnapshot.Validate() != nil || r.PackageRecord.Validate() != nil || r.Surface.Validate() != nil || r.Digest.Validate() != nil || r.Executable || r.UnsupportedReason != CorePackUnsupportedReasonV1 || r.ExpiresUnixNano <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Core Pack Assembly result is incomplete")
	}
	if r.Surface.RegistrySnapshotDigest != r.RegistrySnapshot.Digest || (r.Mode == CorePackAssemblyPreviewV1) != r.ReferenceOnly || (r.Mode == CorePackAssemblyAdmittedV1) != r.Admitted {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Core Pack Assembly result mode or Snapshot drifted")
	}
	if r.ReferenceOnly && (r.PackageAssembly != nil || r.PackageRecord.State != registry.StateSubmitted) {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "preview result contains admitted state")
	}
	if r.Admitted && (r.PackageAssembly == nil || r.PackageAssembly.Validate() != nil || r.PackageRecord.State != registry.StateActive) {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "admitted result lacks an active exact Package closure")
	}
	copy := r.Clone()
	copy.Digest = ""
	digest, err := toolcontract.Seal("praxis.tool-mcp.core-pack-assembly", CorePackAssemblyContractVersionV1, "CorePackAssemblyResultV1", copy)
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Core Pack Assembly result digest drifted")
	}
	return nil
}

func (r CorePackAssemblyResultV1) Clone() CorePackAssemblyResultV1 {
	r.Catalog = r.Catalog.Clone()
	r.Surface.Entries = append([]toolcontract.ToolSurfaceEntry(nil), r.Surface.Entries...)
	for i := range r.Surface.Entries {
		r.Surface.Entries[i].EffectKinds = append([]runtimeports.NamespacedNameV2(nil), r.Surface.Entries[i].EffectKinds...)
	}
	if r.PackageAssembly != nil {
		a := clonePackageAssemblyForKitV1(*r.PackageAssembly)
		r.PackageAssembly = &a
	}
	return r
}

func clonePackageAssemblyForKitV1(a sdk.PackageAssemblyV1) sdk.PackageAssemblyV1 {
	b, _ := json.Marshal(a)
	var out sdk.PackageAssemblyV1
	_ = json.Unmarshal(b, &out)
	return out
}

func minExpiryV1(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
