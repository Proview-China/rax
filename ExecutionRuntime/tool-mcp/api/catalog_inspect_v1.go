package api

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

type InspectRegistryObjectRequestV1 struct {
	Kind  string                 `json:"kind"`
	Exact toolcontract.ObjectRef `json:"exact"`
}

func (r InspectRegistryObjectRequestV1) Validate() error {
	if !validRegistryKindV1(r.Kind, false) || r.Exact.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Registry object inspect request is invalid")
	}
	return nil
}

// RegistryObjectProjectionV1 is a closed typed union. Exactly one object must
// be present and must bind the exact Registry Record returned by the same
// owner read. It is a read projection, not authority or an admission fact.
type RegistryObjectProjectionV1 struct {
	ContractVersion  string                             `json:"contract_version"`
	Kind             string                             `json:"kind"`
	Record           registry.Record                    `json:"record"`
	Capability       *toolcontract.CapabilityDescriptor `json:"capability,omitempty"`
	Tool             *toolcontract.ToolDescriptor       `json:"tool,omitempty"`
	Package          *toolcontract.ToolPackageManifest  `json:"package,omitempty"`
	ToolAlias        *toolcontract.ToolAliasV1          `json:"tool_alias,omitempty"`
	ProjectionDigest core.Digest                        `json:"projection_digest"`
}

func (p RegistryObjectProjectionV1) Validate() error {
	if p.ContractVersion != CatalogContractVersionV1 || !validRegistryKindV1(p.Kind, false) || p.Record.Validate() != nil || p.Record.Kind != p.Kind || p.ProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Registry object projection is incomplete")
	}
	set := 0
	if p.Capability != nil {
		set++
		if p.Kind != "capability" || p.Capability.Validate() != nil || !recordMatchesRegistryObjectV1(p.Record, string(p.Capability.ID), p.Capability.Revision, p.Capability.Digest) {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Registry Capability projection drifted")
		}
	}
	if p.Tool != nil {
		set++
		if p.Kind != "tool" || p.Tool.Validate() != nil || !recordMatchesRegistryObjectV1(p.Record, string(p.Tool.ID), p.Tool.Revision, p.Tool.Digest) {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Registry Tool projection drifted")
		}
	}
	if p.Package != nil {
		set++
		if p.Kind != "package" || p.Package.Validate() != nil || !recordMatchesRegistryObjectV1(p.Record, string(p.Package.ID), p.Package.Revision, p.Package.Digest) {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Registry Package projection drifted")
		}
	}
	if p.ToolAlias != nil {
		set++
		if p.Kind != "tool-alias" || p.ToolAlias.Validate() != nil || !recordMatchesRegistryObjectV1(p.Record, p.ToolAlias.Ref.ID, p.ToolAlias.Ref.Revision, p.ToolAlias.Ref.Digest) {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Registry Tool Alias projection drifted")
		}
	}
	if set != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Registry object projection must contain exactly one typed object")
	}
	digest, err := p.ComputeDigest()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Registry object projection digest drifted")
	}
	return nil
}

func (p RegistryObjectProjectionV1) ComputeDigest() (core.Digest, error) {
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.tool-mcp.api", CatalogContractVersionV1, "RegistryObjectProjectionV1", p)
}

func sealRegistryObjectProjectionV1(p RegistryObjectProjectionV1) (RegistryObjectProjectionV1, error) {
	p.ContractVersion = CatalogContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.ComputeDigest()
	if err != nil {
		return RegistryObjectProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate()
}

func (c *CatalogV1) InspectRegistryObjectV1(ctx context.Context, request InspectRegistryObjectRequestV1) (RegistryObjectProjectionV1, error) {
	if c == nil || nilLikeCatalogV1(c.sdk) {
		return RegistryObjectProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Catalog API is unavailable")
	}
	if ctx == nil {
		return RegistryObjectProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Catalog API context is required")
	}
	if err := ctx.Err(); err != nil {
		return RegistryObjectProjectionV1{}, err
	}
	if err := request.Validate(); err != nil {
		return RegistryObjectProjectionV1{}, err
	}
	first, err := c.inspectRegistryObjectOnceV1(ctx, request)
	if err != nil {
		return RegistryObjectProjectionV1{}, err
	}
	if err = ctx.Err(); err != nil {
		return RegistryObjectProjectionV1{}, err
	}
	second, err := c.inspectRegistryObjectOnceV1(ctx, request)
	if err != nil {
		return RegistryObjectProjectionV1{}, err
	}
	if first.ProjectionDigest != second.ProjectionDigest {
		return RegistryObjectProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Registry object changed during exact Inspect")
	}
	return cloneRegistryObjectProjectionV1(second), nil
}

func (c *CatalogV1) inspectRegistryObjectOnceV1(ctx context.Context, request InspectRegistryObjectRequestV1) (RegistryObjectProjectionV1, error) {
	projection := RegistryObjectProjectionV1{Kind: request.Kind}
	var err error
	switch request.Kind {
	case "capability":
		var value toolcontract.CapabilityDescriptor
		value, projection.Record, err = c.sdk.InspectCapabilityV1(ctx, request.Exact)
		projection.Capability = &value
	case "tool":
		var value toolcontract.ToolDescriptor
		value, projection.Record, err = c.sdk.InspectToolV1(ctx, request.Exact)
		projection.Tool = &value
	case "package":
		var value toolcontract.ToolPackageManifest
		value, projection.Record, err = c.sdk.InspectPackageV1(ctx, request.Exact)
		projection.Package = &value
	case "tool-alias":
		var value toolcontract.ToolAliasV1
		value, projection.Record, err = c.sdk.InspectToolAliasV1(ctx, toolcontract.ToolAliasRefV1{ID: request.Exact.ID, Revision: request.Exact.Revision, Digest: request.Exact.Digest})
		projection.ToolAlias = &value
	default:
		return RegistryObjectProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Registry object kind is unsupported")
	}
	if err != nil {
		return RegistryObjectProjectionV1{}, err
	}
	return sealRegistryObjectProjectionV1(projection)
}

func recordMatchesRegistryObjectV1(record registry.Record, id string, revision core.Revision, digest core.Digest) bool {
	return record.ID == id && record.ObjectRevision == revision && record.ObjectDigest == digest
}

func cloneRegistryObjectProjectionV1(p RegistryObjectProjectionV1) RegistryObjectProjectionV1 {
	if p.Capability != nil {
		value := *p.Capability
		value.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), value.EffectKinds...)
		p.Capability = &value
	}
	if p.Tool != nil {
		value := *p.Tool
		value.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), value.EffectKinds...)
		value.Residuals = append([]toolcontract.Residual(nil), value.Residuals...)
		p.Tool = &value
	}
	if p.Package != nil {
		value := *p.Package
		value.Signatures = append([]core.Digest(nil), value.Signatures...)
		value.Descriptors = append([]toolcontract.PackageDescriptorRef(nil), value.Descriptors...)
		value.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), value.EffectKinds...)
		p.Package = &value
	}
	if p.ToolAlias != nil {
		value := *p.ToolAlias
		p.ToolAlias = &value
	}
	return p
}
