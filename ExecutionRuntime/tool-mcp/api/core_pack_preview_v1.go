package api

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/corepack"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

const CorePackPreviewContractVersionV1 = "praxis.tool-mcp.core-pack-preview/v1"

type CorePackPreviewConfigV1 struct {
	ContractVersion          string        `json:"contract_version"`
	Owner                    core.OwnerRef `json:"owner"`
	ArtifactDigest           core.Digest   `json:"artifact_digest"`
	SignatureDigest          core.Digest   `json:"signature_digest"`
	ProvenanceDigest         core.Digest   `json:"provenance_digest"`
	SurfaceID                string        `json:"surface_id"`
	ResolvedPlanDigest       core.Digest   `json:"resolved_plan_digest"`
	ProfileDigest            core.Digest   `json:"profile_digest"`
	CapabilityGrantDigest    core.Digest   `json:"capability_grant_digest"`
	CreatedUnixNano          int64         `json:"created_unix_nano"`
	SurfaceExpiresUnixNano   int64         `json:"surface_expires_unix_nano"`
	RequestedExpiresUnixNano int64         `json:"requested_expires_unix_nano"`
}

func DecodeCorePackPreviewConfigV1(payload []byte) (CorePackPreviewConfigV1, error) {
	var config CorePackPreviewConfigV1
	if err := core.DecodeStrictJSON(payload, &config); err != nil {
		return CorePackPreviewConfigV1{}, err
	}
	if err := config.Validate(); err != nil {
		return CorePackPreviewConfigV1{}, err
	}
	return config, nil
}

func (c CorePackPreviewConfigV1) Validate() error {
	if c.ContractVersion != CorePackPreviewContractVersionV1 || c.Owner.Validate() != nil || c.ArtifactDigest.Validate() != nil || c.SignatureDigest.Validate() != nil || c.ProvenanceDigest.Validate() != nil || c.ResolvedPlanDigest.Validate() != nil || c.ProfileDigest.Validate() != nil || c.CapabilityGrantDigest.Validate() != nil || contract.ValidateStableID(c.SurfaceID) != nil || c.CreatedUnixNano <= 0 || c.SurfaceExpiresUnixNano <= c.CreatedUnixNano || c.RequestedExpiresUnixNano < c.SurfaceExpiresUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Core Pack Preview config is incomplete")
	}
	return nil
}

func (c CorePackPreviewConfigV1) ValidateCurrent(now time.Time) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < c.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Core Pack Preview clock regressed")
	}
	if now.UnixNano() >= c.SurfaceExpiresUnixNano || now.UnixNano() >= c.RequestedExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Core Pack Preview config expired")
	}
	return nil
}

type CorePackPreviewDeclarationV1 struct {
	Order             uint32                          `json:"order"`
	ModelName         string                          `json:"model_name"`
	CapabilityRef     contract.ObjectRef              `json:"capability_ref"`
	ToolRef           contract.ObjectRef              `json:"tool_ref"`
	InputSchemaRef    runtimeports.SchemaRefV2        `json:"input_schema_ref"`
	DescriptionDigest core.Digest                     `json:"description_digest"`
	EffectKinds       []runtimeports.NamespacedNameV2 `json:"effect_kinds"`
	Risk              contract.RiskClass              `json:"risk"`
	ReviewProfile     runtimeports.NamespacedNameV2   `json:"review_profile"`
}

type CorePackPreviewResultV1 struct {
	ContractVersion   string                         `json:"contract_version"`
	AssemblyDigest    core.Digest                    `json:"assembly_digest"`
	PackageRef        contract.ObjectRef             `json:"package_ref"`
	PackageState      registry.State                 `json:"package_state"`
	RegistrySnapshot  sdk.RegistrySnapshotRefV1      `json:"registry_snapshot_ref"`
	SurfaceRef        contract.ObjectRef             `json:"surface_ref"`
	Declarations      []CorePackPreviewDeclarationV1 `json:"declarations"`
	ReferenceOnly     bool                           `json:"reference_only"`
	Admitted          bool                           `json:"admitted"`
	Executable        bool                           `json:"executable"`
	UnsupportedReason string                         `json:"unsupported_reason"`
	ExpiresUnixNano   int64                          `json:"expires_unix_nano"`
	Digest            core.Digest                    `json:"digest"`
	proof             *corepack.CorePackAssemblyResultV1
}

type CorePackPreviewAssemblyPortV1 interface {
	BuildV1(context.Context, corepack.CorePackAssemblyRequestV1) (corepack.CorePackAssemblyResultV1, error)
}
type CorePackPreviewV1 struct {
	factory CorePackPreviewAssemblyPortV1
	clock   func() time.Time
}

func NewCorePackPreviewV1(factory CorePackPreviewAssemblyPortV1, clock func() time.Time) (*CorePackPreviewV1, error) {
	if nilLikeCorePackPreviewV1(factory) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Core Pack Preview dependencies are required")
	}
	return &CorePackPreviewV1{factory: factory, clock: clock}, nil
}

func (p *CorePackPreviewV1) PreviewV1(ctx context.Context, config CorePackPreviewConfigV1) (CorePackPreviewResultV1, error) {
	if p == nil || nilLikeCorePackPreviewV1(p.factory) || p.clock == nil {
		return CorePackPreviewResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Core Pack Preview is unavailable")
	}
	if ctx == nil {
		return CorePackPreviewResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Core Pack Preview context is required")
	}
	if err := ctx.Err(); err != nil {
		return CorePackPreviewResultV1{}, err
	}
	now := p.clock().UTC()
	if err := config.ValidateCurrent(now); err != nil {
		return CorePackPreviewResultV1{}, err
	}
	assembled, err := p.factory.BuildV1(ctx, corepack.CorePackAssemblyRequestV1{
		ContractVersion: corepack.CorePackAssemblyContractVersionV1, Mode: corepack.CorePackAssemblyPreviewV1,
		Catalog:                  corepack.CatalogConfigV1{Owner: config.Owner, ArtifactDigest: config.ArtifactDigest, SignatureDigest: config.SignatureDigest, ProvenanceDigest: config.ProvenanceDigest, CreatedAt: time.Unix(0, config.CreatedUnixNano).UTC()},
		Surface:                  corepack.SurfaceConfigV1{ID: config.SurfaceID, Owner: config.Owner, ResolvedPlanDigest: config.ResolvedPlanDigest, ProfileDigest: config.ProfileDigest, CapabilityGrantDigest: config.CapabilityGrantDigest, RegistrySnapshotDigest: core.DigestBytes([]byte("preview-snapshot-placeholder-v1")), CreatedAt: time.Unix(0, config.CreatedUnixNano).UTC(), ExpiresAt: time.Unix(0, config.SurfaceExpiresUnixNano).UTC()},
		RequestedExpiresUnixNano: config.RequestedExpiresUnixNano,
	})
	if err != nil {
		return CorePackPreviewResultV1{}, err
	}
	if err = assembled.Validate(); err != nil {
		return CorePackPreviewResultV1{}, err
	}
	if assembled.Mode != corepack.CorePackAssemblyPreviewV1 || !assembled.ReferenceOnly || assembled.Admitted || assembled.Executable || assembled.PackageRecord.State != registry.StateSubmitted {
		return CorePackPreviewResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Core Pack Preview factory returned a non-preview assembly")
	}
	result := CorePackPreviewResultV1{ContractVersion: CorePackPreviewContractVersionV1, AssemblyDigest: assembled.Digest, PackageRef: contract.ObjectRef{ID: string(assembled.Catalog.Package.ID), Revision: assembled.Catalog.Package.Revision, Digest: assembled.Catalog.Package.Digest}, PackageState: assembled.PackageRecord.State, RegistrySnapshot: assembled.RegistrySnapshot, SurfaceRef: contract.ObjectRef{ID: assembled.Surface.ID, Revision: assembled.Surface.Revision, Digest: assembled.Surface.Digest}, ReferenceOnly: true, UnsupportedReason: assembled.UnsupportedReason, ExpiresUnixNano: assembled.ExpiresUnixNano}
	for i, entry := range assembled.Surface.Entries {
		result.Declarations = append(result.Declarations, CorePackPreviewDeclarationV1{Order: entry.Order, ModelName: entry.ModelName, CapabilityRef: entry.Capability, ToolRef: entry.Tool, InputSchemaRef: entry.InputSchema, DescriptionDigest: entry.DescriptionDigest, EffectKinds: append([]runtimeports.NamespacedNameV2(nil), entry.EffectKinds...), Risk: assembled.Catalog.Capabilities[i].Risk, ReviewProfile: assembled.Catalog.Capabilities[i].ReviewProfile})
	}
	proof := assembled.Clone()
	result.proof = &proof
	return sealCorePackPreviewResultV1(result)
}

func (r CorePackPreviewResultV1) Validate() error {
	if r.ContractVersion != CorePackPreviewContractVersionV1 || r.Digest.Validate() != nil || r.AssemblyDigest.Validate() != nil || r.PackageRef.Validate() != nil || r.RegistrySnapshot.Validate() != nil || r.SurfaceRef.Validate() != nil || len(r.Declarations) != 5 || !r.ReferenceOnly || r.Admitted || r.Executable || r.PackageState != registry.StateSubmitted || r.UnsupportedReason != corepack.CorePackUnsupportedReasonV1 || r.ExpiresUnixNano <= 0 || r.proof == nil || r.proof.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Core Pack Preview result is incomplete")
	}
	p := r.proof
	if r.AssemblyDigest != p.Digest || r.RegistrySnapshot != p.RegistrySnapshot || r.ExpiresUnixNano != p.ExpiresUnixNano || r.PackageRef != (contract.ObjectRef{ID: string(p.Catalog.Package.ID), Revision: p.Catalog.Package.Revision, Digest: p.Catalog.Package.Digest}) || r.SurfaceRef != (contract.ObjectRef{ID: p.Surface.ID, Revision: p.Surface.Revision, Digest: p.Surface.Digest}) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Core Pack Preview summary differs from the Assembly proof")
	}
	for i, d := range r.Declarations {
		e, c := p.Surface.Entries[i], p.Catalog.Capabilities[i]
		if d.Order != e.Order || d.ModelName != e.ModelName || d.CapabilityRef != e.Capability || d.ToolRef != e.Tool || d.InputSchemaRef != e.InputSchema || d.DescriptionDigest != e.DescriptionDigest || d.Risk != c.Risk || d.ReviewProfile != c.ReviewProfile || len(d.EffectKinds) != len(e.EffectKinds) {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Core Pack Preview declaration differs from the Assembly proof")
		}
		for j := range d.EffectKinds {
			if d.EffectKinds[j] != e.EffectKinds[j] {
				return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Core Pack Preview effects differ from the Assembly proof")
			}
		}
	}
	copy := r.Clone()
	copy.Digest = ""
	digest, err := contract.Seal("praxis.tool-mcp.core-pack-preview", CorePackPreviewContractVersionV1, "CorePackPreviewResultV1", copy)
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Core Pack Preview result digest drifted")
	}
	return nil
}

func (r CorePackPreviewResultV1) Clone() CorePackPreviewResultV1 {
	r.Declarations = append([]CorePackPreviewDeclarationV1(nil), r.Declarations...)
	for i := range r.Declarations {
		r.Declarations[i].EffectKinds = append([]runtimeports.NamespacedNameV2(nil), r.Declarations[i].EffectKinds...)
	}
	if r.proof != nil {
		p := r.proof.Clone()
		r.proof = &p
	}
	return r
}

func sealCorePackPreviewResultV1(result CorePackPreviewResultV1) (CorePackPreviewResultV1, error) {
	result = result.Clone()
	result.Digest = ""
	digest, err := contract.Seal("praxis.tool-mcp.core-pack-preview", CorePackPreviewContractVersionV1, "CorePackPreviewResultV1", result)
	if err != nil {
		return CorePackPreviewResultV1{}, err
	}
	result.Digest = digest
	return result, result.Validate()
}

func nilLikeCorePackPreviewV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
