package corepack

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type SurfaceConfigV1 struct {
	ID                     string
	Owner                  core.OwnerRef
	ResolvedPlanDigest     core.Digest
	ProfileDigest          core.Digest
	CapabilityGrantDigest  core.Digest
	RegistrySnapshotDigest core.Digest
	CreatedAt              time.Time
	ExpiresAt              time.Time
}

func BuildSurfaceV1(catalog CatalogV1, config SurfaceConfigV1) (contract.ToolSurfaceManifest, error) {
	if err := catalog.Validate(); err != nil {
		return contract.ToolSurfaceManifest{}, err
	}
	if config.Owner.Validate() != nil || config.ResolvedPlanDigest.Validate() != nil ||
		config.ProfileDigest.Validate() != nil || config.CapabilityGrantDigest.Validate() != nil ||
		config.RegistrySnapshotDigest.Validate() != nil || config.CreatedAt.IsZero() ||
		!config.CreatedAt.Before(config.ExpiresAt) {
		return contract.ToolSurfaceManifest{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Core Tool Pack surface config is incomplete")
	}
	entries := make([]contract.ToolSurfaceEntry, 0, len(catalog.Tools))
	for i := range catalog.Tools {
		definition := catalog.Definitions[i]
		capability := catalog.Capabilities[i]
		tool := catalog.Tools[i]
		mechanism, err := contract.Seal("praxis.core-tool.surface", contract.CoreToolPackContractVersionV1, "LocalMechanism", struct {
			Tool      contract.ObjectRef `json:"tool"`
			Mechanism string             `json:"mechanism"`
		}{objectRefV1(tool.ID, tool.Revision, tool.Digest), string(tool.Mechanism)})
		if err != nil {
			return contract.ToolSurfaceManifest{}, err
		}
		entries = append(entries, contract.ToolSurfaceEntry{
			Capability: objectRefV1(capability.ID, capability.Revision, capability.Digest),
			Tool:       objectRefV1(tool.ID, tool.Revision, tool.Digest),
			ModelName:  definition.ModelName, InputSchema: capability.InputSchema,
			DescriptionDigest: catalog.Materials[i].Ref.DescriptionDigest,
			Visibility:        contract.SurfaceVisible, Allowed: true,
			Admission: contract.AdmissionRequired, MechanismDigest: mechanism,
			EffectKinds: []runtimeports.NamespacedNameV2{EffectKindV1},
		})
	}
	return contract.SealSurface(contract.ToolSurfaceManifest{
		ID: config.ID, Revision: 1, Owner: config.Owner,
		ResolvedPlanDigest: config.ResolvedPlanDigest, ProfileDigest: config.ProfileDigest,
		CapabilityGrantDigest: config.CapabilityGrantDigest, RegistrySnapshotDigest: config.RegistrySnapshotDigest,
		Entries: entries, Dialect: contract.ModelToolDialectFunctionCallingV1,
		CreatedUnixNano: config.CreatedAt.UTC().UnixNano(), ExpiresUnixNano: config.ExpiresAt.UTC().UnixNano(),
	})
}
