package surface

import (
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type Selection struct {
	Capability        contract.CapabilityDescriptor
	Tool              contract.ToolDescriptor
	ModelName         string
	DescriptionDigest core.Digest
	Visible           bool
	Allowed           bool
	PreApproved       bool
}

type CompileRequest struct {
	Owner                  core.OwnerRef
	ResolvedPlanDigest     core.Digest
	ProfileDigest          core.Digest
	CapabilityGrantDigest  core.Digest
	RegistrySnapshotDigest core.Digest
	Dialect                runtimeports.NamespacedNameV2
	Selections             []Selection
	Revision               core.Revision
	CreatedAt              time.Time
	ExpiresAt              time.Time
}

func Compile(request CompileRequest) (contract.ToolSurfaceManifest, error) {
	if request.Owner.Validate() != nil || request.Revision == 0 || request.CreatedAt.IsZero() || !request.ExpiresAt.After(request.CreatedAt) || runtimeports.ValidateNamespacedNameV2(request.Dialect) != nil {
		return contract.ToolSurfaceManifest{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "surface compile request is incomplete")
	}
	for _, digest := range []core.Digest{request.ResolvedPlanDigest, request.ProfileDigest, request.CapabilityGrantDigest, request.RegistrySnapshotDigest} {
		if digest.Validate() != nil {
			return contract.ToolSurfaceManifest{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidDigest, "surface compile input digest is invalid")
		}
	}
	if len(request.Selections) == 0 || len(request.Selections) > contract.MaxSurfaceEntries {
		return contract.ToolSurfaceManifest{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "surface selection count is invalid")
	}
	selections := append([]Selection(nil), request.Selections...)
	sort.Slice(selections, func(i, j int) bool {
		if selections[i].ModelName != selections[j].ModelName {
			return selections[i].ModelName < selections[j].ModelName
		}
		if selections[i].Capability.ID != selections[j].Capability.ID {
			return selections[i].Capability.ID < selections[j].Capability.ID
		}
		return selections[i].Tool.ID < selections[j].Tool.ID
	})
	entries := make([]contract.ToolSurfaceEntry, 0, len(selections))
	for _, selection := range selections {
		if err := selection.Tool.ValidateAgainst(selection.Capability); err != nil {
			return contract.ToolSurfaceManifest{}, err
		}
		if !selection.Visible && (selection.Allowed || selection.PreApproved) || selection.PreApproved && !selection.Allowed {
			return contract.ToolSurfaceManifest{}, core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "surface visible, allowed and pre-approved sets are inconsistent")
		}
		mechanismDigest, err := contract.Seal("praxis.tool-mcp.surface", contract.SurfaceContractVersion, "Mechanism", struct {
			Mechanism contract.ToolMechanism `json:"mechanism"`
			Artifact  core.Digest            `json:"artifact_digest"`
		}{selection.Tool.Mechanism, selection.Tool.ArtifactDigest})
		if err != nil {
			return contract.ToolSurfaceManifest{}, err
		}
		visibility := contract.SurfaceHidden
		if selection.Visible {
			visibility = contract.SurfaceVisible
		}
		admission := contract.AdmissionRequired
		if selection.PreApproved {
			admission = contract.AdmissionPreApproved
		}
		entries = append(entries, contract.ToolSurfaceEntry{
			Capability: contract.ObjectRef{ID: string(selection.Capability.ID), Revision: selection.Capability.Revision, Digest: selection.Capability.Digest},
			Tool:       contract.ObjectRef{ID: string(selection.Tool.ID), Revision: selection.Tool.Revision, Digest: selection.Tool.Digest},
			ModelName:  selection.ModelName, InputSchema: selection.Tool.InputSchema, DescriptionDigest: selection.DescriptionDigest,
			Visibility: visibility, Allowed: selection.Allowed, Admission: admission, MechanismDigest: mechanismDigest, EffectKinds: append([]runtimeports.NamespacedNameV2(nil), selection.Tool.EffectKinds...),
		})
	}
	id, err := contract.StableID("surface", string(request.ResolvedPlanDigest), string(request.ProfileDigest), string(request.RegistrySnapshotDigest), string(request.Dialect))
	if err != nil {
		return contract.ToolSurfaceManifest{}, err
	}
	return contract.SealSurface(contract.ToolSurfaceManifest{
		ID: id, Revision: request.Revision, Owner: request.Owner, ResolvedPlanDigest: request.ResolvedPlanDigest, ProfileDigest: request.ProfileDigest,
		CapabilityGrantDigest: request.CapabilityGrantDigest, RegistrySnapshotDigest: request.RegistrySnapshotDigest, Entries: entries, Dialect: request.Dialect,
		CreatedUnixNano: request.CreatedAt.UTC().UnixNano(), ExpiresUnixNano: request.ExpiresAt.UTC().UnixNano(),
	})
}
