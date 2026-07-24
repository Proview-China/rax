package sdk

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

// ToolAliasResolutionV1 is an assembly-only projection. Surface compilation
// consumes Tool; Alias remains causation metadata and is never read by a Run.
type ToolAliasResolutionV1 struct {
	RegistrySnapshot RegistrySnapshotRefV1       `json:"registry_snapshot"`
	Alias            toolcontract.ToolAliasV1    `json:"alias"`
	AliasRecord      registry.Record             `json:"alias_record"`
	Tool             toolcontract.ToolDescriptor `json:"tool"`
	ToolRecord       registry.Record             `json:"tool_record"`
}

func (r ToolAliasResolutionV1) Validate() error {
	if r.RegistrySnapshot.Validate() != nil || r.Alias.Validate() != nil || r.AliasRecord.Validate() != nil || r.Tool.Validate() != nil || r.ToolRecord.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Alias resolution is incomplete")
	}
	if r.AliasRecord.Kind != "tool-alias" || r.AliasRecord.ID != r.Alias.Ref.ID || r.AliasRecord.ObjectRevision != r.Alias.Ref.Revision || r.AliasRecord.ObjectDigest != r.Alias.Ref.Digest || r.AliasRecord.State != registry.StateActive {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Alias resolution does not bind an active exact Alias")
	}
	if r.ToolRecord.Kind != "tool" || r.ToolRecord.ID != string(r.Tool.ID) || r.ToolRecord.ObjectRevision != r.Tool.Revision || r.ToolRecord.ObjectDigest != r.Tool.Digest || r.ToolRecord.State != registry.StateActive || r.Alias.Tool != (toolcontract.ObjectRef{ID: string(r.Tool.ID), Revision: r.Tool.Revision, Digest: r.Tool.Digest}) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Alias resolution does not bind an active exact Tool")
	}
	if r.AliasRecord.RegistryRevision > r.RegistrySnapshot.Revision || r.ToolRecord.RegistryRevision > r.RegistrySnapshot.Revision {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Alias resolution is newer than the exact Registry Snapshot")
	}
	return nil
}

func (s *SDKV1) RegisterToolAliasV1(ctx context.Context, alias toolcontract.ToolAliasV1, expectedCurrent *toolcontract.ToolAliasRefV1) (registry.Record, error) {
	now, err := s.readyV1(ctx)
	if err != nil {
		return registry.Record{}, err
	}
	if err := alias.ValidateAt(now); err != nil {
		return registry.Record{}, err
	}
	return s.registry.SubmitToolAlias(alias, expectedCurrent, now)
}

func (s *SDKV1) InspectToolAliasV1(ctx context.Context, exact toolcontract.ToolAliasRefV1) (toolcontract.ToolAliasV1, registry.Record, error) {
	if _, err := s.readyV1(ctx); err != nil {
		return toolcontract.ToolAliasV1{}, registry.Record{}, err
	}
	if exact.Validate() != nil {
		return toolcontract.ToolAliasV1{}, registry.Record{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Alias exact Ref is invalid")
	}
	value, record, ok := s.registry.InspectToolAlias(exact)
	if !ok {
		return toolcontract.ToolAliasV1{}, registry.Record{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Tool Alias was not found")
	}
	if value.Validate() != nil || value.Ref != exact || record.Validate() != nil || record.Kind != "tool-alias" || record.ID != exact.ID || record.ObjectRevision != exact.Revision || record.ObjectDigest != exact.Digest {
		return toolcontract.ToolAliasV1{}, registry.Record{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Alias exact history drifted")
	}
	return value, record, nil
}

func (s *SDKV1) ResolveToolAliasForAssemblyV1(ctx context.Context, owner core.OwnerRef, alias runtimeports.NamespacedNameV2, snapshotRef RegistrySnapshotRefV1) (ToolAliasResolutionV1, error) {
	started, err := s.readyV1(ctx)
	if err != nil {
		return ToolAliasResolutionV1{}, err
	}
	id, err := toolcontract.DeriveToolAliasIDV1(owner, alias)
	if err != nil || snapshotRef.Validate() != nil {
		return ToolAliasResolutionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Alias assembly coordinates are invalid")
	}
	if err := s.validateSnapshotExactV1(snapshotRef); err != nil {
		return ToolAliasResolutionV1{}, err
	}
	value, aliasRecord, ok := s.registry.ResolveToolAlias(id)
	if !ok || value.Validate() != nil || value.Owner != owner || value.Alias != alias || aliasRecord.Validate() != nil || aliasRecord.State != registry.StateActive || aliasRecord.ObjectRevision != value.Ref.Revision || aliasRecord.ObjectDigest != value.Ref.Digest {
		return ToolAliasResolutionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "Tool Alias is not an active exact Registry object")
	}
	tool, toolRecord, ok := s.registry.ResolveTool(value.Tool.ID)
	if !ok || tool.Validate() != nil || toolRecord.Validate() != nil || toolRecord.State != registry.StateActive || string(tool.ID) != value.Tool.ID || tool.Revision != value.Tool.Revision || tool.Digest != value.Tool.Digest || toolRecord.ObjectRevision != value.Tool.Revision || toolRecord.ObjectDigest != value.Tool.Digest {
		return ToolAliasResolutionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "Tool Alias target is not an active exact Tool")
	}
	result := ToolAliasResolutionV1{RegistrySnapshot: snapshotRef, Alias: value, AliasRecord: aliasRecord, Tool: tool, ToolRecord: toolRecord}
	if err := result.Validate(); err != nil {
		return ToolAliasResolutionV1{}, err
	}
	if err := s.validateSnapshotExactV1(snapshotRef); err != nil {
		return ToolAliasResolutionV1{}, err
	}
	finished, err := s.readyV1(ctx)
	if err != nil {
		return ToolAliasResolutionV1{}, err
	}
	if finished.Before(started) {
		return ToolAliasResolutionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Alias assembly clock regressed")
	}
	return cloneToolAliasResolutionV1(result), nil
}

func cloneToolAliasResolutionV1(value ToolAliasResolutionV1) ToolAliasResolutionV1 {
	value.Tool.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), value.Tool.EffectKinds...)
	value.Tool.Residuals = append([]toolcontract.Residual(nil), value.Tool.Residuals...)
	return value
}
