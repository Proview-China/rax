package sdk

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

const ContractVersionV1 = "praxis.tool-mcp.sdk/v1"

type ClockV1 func() time.Time

type RegistryPortV1 interface {
	SubmitCapability(contract.CapabilityDescriptor, time.Time) (registry.Record, error)
	SubmitTool(contract.ToolDescriptor, time.Time) (registry.Record, error)
	SubmitPackage(contract.ToolPackageManifest, time.Time) (registry.Record, error)
	SubmitToolAlias(contract.ToolAliasV1, *contract.ToolAliasRefV1, time.Time) (registry.Record, error)
	SubmitMCPToolMapping(contract.MCPToolMappingManifestV1, time.Time) (registry.Record, error)
	ResolveCapability(string) (contract.CapabilityDescriptor, registry.Record, bool)
	ResolveTool(string) (contract.ToolDescriptor, registry.Record, bool)
	ResolvePackage(string) (contract.ToolPackageManifest, registry.Record, bool)
	InspectToolAlias(contract.ToolAliasRefV1) (contract.ToolAliasV1, registry.Record, bool)
	ResolveToolAlias(string) (contract.ToolAliasV1, registry.Record, bool)
	InspectMCPToolMapping(contract.MCPToolMappingManifestRefV1) (contract.MCPToolMappingManifestV1, registry.Record, bool)
	Snapshot() (registry.Snapshot, error)
}

type RegistrySnapshotRefV1 struct {
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r RegistrySnapshotRefV1) Validate() error {
	// Revision zero is the exact, valid snapshot of an empty Registry.
	if r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "SDK Registry Snapshot Ref is invalid")
	}
	return nil
}

type SurfaceSelectionV1 struct {
	Capability        contract.ObjectRef `json:"capability"`
	Tool              contract.ObjectRef `json:"tool"`
	ModelName         string             `json:"model_name"`
	DescriptionDigest core.Digest        `json:"description_digest"`
	Visible           bool               `json:"visible"`
	Allowed           bool               `json:"allowed"`
	PreApproved       bool               `json:"pre_approved"`
}

func (s SurfaceSelectionV1) Validate() error {
	if s.Capability.Validate() != nil || s.Tool.Validate() != nil || strings.TrimSpace(s.ModelName) == "" || len(s.ModelName) > contract.MaxStringBytes || s.DescriptionDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "SDK Surface Selection is invalid")
	}
	if !s.Visible && (s.Allowed || s.PreApproved) || s.PreApproved && !s.Allowed {
		return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "SDK Surface Selection visibility and admission drifted")
	}
	return nil
}

type CompileSurfaceRequestV1 struct {
	Owner                    core.OwnerRef                 `json:"owner"`
	ResolvedPlanDigest       core.Digest                   `json:"resolved_plan_digest"`
	ProfileDigest            core.Digest                   `json:"profile_digest"`
	CapabilityGrantDigest    core.Digest                   `json:"capability_grant_digest"`
	RegistrySnapshot         RegistrySnapshotRefV1         `json:"registry_snapshot"`
	Dialect                  runtimeports.NamespacedNameV2 `json:"dialect"`
	Selections               []SurfaceSelectionV1          `json:"selections"`
	Revision                 core.Revision                 `json:"revision"`
	RequestedExpiresUnixNano int64                         `json:"requested_expires_unix_nano"`
}

func (r CompileSurfaceRequestV1) Validate(now time.Time) error {
	if now.IsZero() || r.Owner.Validate() != nil || r.ResolvedPlanDigest.Validate() != nil || r.ProfileDigest.Validate() != nil || r.CapabilityGrantDigest.Validate() != nil || r.RegistrySnapshot.Validate() != nil || runtimeports.ValidateNamespacedNameV2(r.Dialect) != nil || r.Revision == 0 || len(r.Selections) == 0 || len(r.Selections) > contract.MaxSurfaceEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "SDK Compile Surface request is incomplete")
	}
	if r.RequestedExpiresUnixNano <= now.UnixNano() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "SDK Compile Surface request expired")
	}
	for _, selection := range r.Selections {
		if err := selection.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type SDKV1 struct {
	registry RegistryPortV1
	clock    ClockV1
}

func NewV1(registryPort RegistryPortV1, clock ClockV1) (*SDKV1, error) {
	if nilLikeV1(registryPort) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool SDK dependencies are required")
	}
	return &SDKV1{registry: registryPort, clock: clock}, nil
}

func (s *SDKV1) RegisterCapabilityV1(ctx context.Context, descriptor contract.CapabilityDescriptor) (registry.Record, error) {
	now, err := s.readyV1(ctx)
	if err != nil {
		return registry.Record{}, err
	}
	if err := descriptor.Validate(); err != nil {
		return registry.Record{}, err
	}
	if descriptor.CreatedUnixNano > now.UnixNano() {
		return registry.Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Capability cannot be registered before its creation time")
	}
	return s.registry.SubmitCapability(descriptor, now)
}

func (s *SDKV1) RegisterToolV1(ctx context.Context, descriptor contract.ToolDescriptor) (registry.Record, error) {
	now, err := s.readyV1(ctx)
	if err != nil {
		return registry.Record{}, err
	}
	if err := descriptor.Validate(); err != nil {
		return registry.Record{}, err
	}
	if descriptor.CreatedUnixNano > now.UnixNano() {
		return registry.Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool cannot be registered before its creation time")
	}
	return s.registry.SubmitTool(descriptor, now)
}

func (s *SDKV1) RegisterPackageV1(ctx context.Context, manifest contract.ToolPackageManifest) (registry.Record, error) {
	now, err := s.readyV1(ctx)
	if err != nil {
		return registry.Record{}, err
	}
	if err := manifest.Validate(); err != nil {
		return registry.Record{}, err
	}
	if manifest.CreatedUnixNano > now.UnixNano() {
		return registry.Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Package cannot be registered before its creation time")
	}
	return s.registry.SubmitPackage(manifest, now)
}

func (s *SDKV1) RegisterMCPToolMappingV1(ctx context.Context, manifest contract.MCPToolMappingManifestV1) (registry.Record, error) {
	now, err := s.readyV1(ctx)
	if err != nil {
		return registry.Record{}, err
	}
	if err := manifest.Validate(); err != nil {
		return registry.Record{}, err
	}
	if manifest.CreatedUnixNano > now.UnixNano() {
		return registry.Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Tool Mapping cannot be registered before its creation time")
	}
	return s.registry.SubmitMCPToolMapping(manifest, now)
}

func (s *SDKV1) InspectCapabilityV1(ctx context.Context, exact contract.ObjectRef) (contract.CapabilityDescriptor, registry.Record, error) {
	if _, err := s.readyV1(ctx); err != nil {
		return contract.CapabilityDescriptor{}, registry.Record{}, err
	}
	return s.inspectCapabilityExactV1(exact)
}

func (s *SDKV1) inspectCapabilityExactV1(exact contract.ObjectRef) (contract.CapabilityDescriptor, registry.Record, error) {
	if exact.Validate() != nil {
		return contract.CapabilityDescriptor{}, registry.Record{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "exact Capability Ref is invalid")
	}
	value, record, ok := s.registry.ResolveCapability(exact.ID)
	if !ok {
		return contract.CapabilityDescriptor{}, registry.Record{}, core.NewError(core.ErrorNotFound, core.ReasonUnknownCapability, "Capability was not found")
	}
	if value.Revision != exact.Revision || value.Digest != exact.Digest || record.ObjectRevision != exact.Revision || record.ObjectDigest != exact.Digest {
		return contract.CapabilityDescriptor{}, registry.Record{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Capability differs from the exact Ref")
	}
	return value, record, nil
}

func (s *SDKV1) InspectToolV1(ctx context.Context, exact contract.ObjectRef) (contract.ToolDescriptor, registry.Record, error) {
	if _, err := s.readyV1(ctx); err != nil {
		return contract.ToolDescriptor{}, registry.Record{}, err
	}
	return s.inspectToolExactV1(exact)
}

func (s *SDKV1) inspectToolExactV1(exact contract.ObjectRef) (contract.ToolDescriptor, registry.Record, error) {
	if exact.Validate() != nil {
		return contract.ToolDescriptor{}, registry.Record{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "exact Tool Ref is invalid")
	}
	value, record, ok := s.registry.ResolveTool(exact.ID)
	if !ok {
		return contract.ToolDescriptor{}, registry.Record{}, core.NewError(core.ErrorNotFound, core.ReasonUnknownCapability, "Tool was not found")
	}
	if value.Revision != exact.Revision || value.Digest != exact.Digest || record.ObjectRevision != exact.Revision || record.ObjectDigest != exact.Digest {
		return contract.ToolDescriptor{}, registry.Record{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool differs from the exact Ref")
	}
	return value, record, nil
}

func (s *SDKV1) InspectPackageV1(ctx context.Context, exact contract.ObjectRef) (contract.ToolPackageManifest, registry.Record, error) {
	if _, err := s.readyV1(ctx); err != nil {
		return contract.ToolPackageManifest{}, registry.Record{}, err
	}
	if exact.Validate() != nil {
		return contract.ToolPackageManifest{}, registry.Record{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "exact Package Ref is invalid")
	}
	value, record, ok := s.registry.ResolvePackage(exact.ID)
	if !ok {
		return contract.ToolPackageManifest{}, registry.Record{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Package was not found")
	}
	if value.Revision != exact.Revision || value.Digest != exact.Digest || record.ObjectRevision != exact.Revision || record.ObjectDigest != exact.Digest {
		return contract.ToolPackageManifest{}, registry.Record{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Package differs from the exact Ref")
	}
	return value, record, nil
}

func (s *SDKV1) InspectMCPToolMappingV1(ctx context.Context, exact contract.MCPToolMappingManifestRefV1) (contract.MCPToolMappingManifestV1, registry.Record, error) {
	if _, err := s.readyV1(ctx); err != nil {
		return contract.MCPToolMappingManifestV1{}, registry.Record{}, err
	}
	if exact.Validate() != nil {
		return contract.MCPToolMappingManifestV1{}, registry.Record{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "exact MCP Tool Mapping Ref is invalid")
	}
	value, record, ok := s.registry.InspectMCPToolMapping(exact)
	if !ok {
		return contract.MCPToolMappingManifestV1{}, registry.Record{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Tool Mapping was not found")
	}
	return value, record, nil
}

func (s *SDKV1) InspectRegistrySnapshotV1(ctx context.Context) (registry.Snapshot, error) {
	if _, err := s.readyV1(ctx); err != nil {
		return registry.Snapshot{}, err
	}
	return s.registry.Snapshot()
}

func (s *SDKV1) ResolveCapabilityForAssemblyV1(ctx context.Context, id runtimeports.NamespacedNameV2, snapshotRef RegistrySnapshotRefV1) (contract.CapabilityDescriptor, registry.Record, error) {
	if _, err := s.readyV1(ctx); err != nil {
		return contract.CapabilityDescriptor{}, registry.Record{}, err
	}
	if runtimeports.ValidateNamespacedNameV2(id) != nil || snapshotRef.Validate() != nil {
		return contract.CapabilityDescriptor{}, registry.Record{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Capability assembly resolution coordinates are invalid")
	}
	if err := s.validateSnapshotExactV1(snapshotRef); err != nil {
		return contract.CapabilityDescriptor{}, registry.Record{}, err
	}
	value, record, ok := s.registry.ResolveCapability(string(id))
	if !ok {
		return contract.CapabilityDescriptor{}, registry.Record{}, core.NewError(core.ErrorNotFound, core.ReasonUnknownCapability, "Capability was not found for assembly")
	}
	if record.State != registry.StateActive || value.Revision != record.ObjectRevision || value.Digest != record.ObjectDigest {
		return contract.CapabilityDescriptor{}, registry.Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "Capability is not an active exact Registry object")
	}
	if err := s.validateSnapshotExactV1(snapshotRef); err != nil {
		return contract.CapabilityDescriptor{}, registry.Record{}, err
	}
	return value, record, nil
}

func (s *SDKV1) ResolveToolForAssemblyV1(ctx context.Context, id runtimeports.NamespacedNameV2, snapshotRef RegistrySnapshotRefV1) (contract.ToolDescriptor, registry.Record, error) {
	if _, err := s.readyV1(ctx); err != nil {
		return contract.ToolDescriptor{}, registry.Record{}, err
	}
	if runtimeports.ValidateNamespacedNameV2(id) != nil || snapshotRef.Validate() != nil {
		return contract.ToolDescriptor{}, registry.Record{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool assembly resolution coordinates are invalid")
	}
	if err := s.validateSnapshotExactV1(snapshotRef); err != nil {
		return contract.ToolDescriptor{}, registry.Record{}, err
	}
	value, record, ok := s.registry.ResolveTool(string(id))
	if !ok {
		return contract.ToolDescriptor{}, registry.Record{}, core.NewError(core.ErrorNotFound, core.ReasonUnknownCapability, "Tool was not found for assembly")
	}
	if record.State != registry.StateActive || value.Revision != record.ObjectRevision || value.Digest != record.ObjectDigest {
		return contract.ToolDescriptor{}, registry.Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "Tool is not an active exact Registry object")
	}
	if err := s.validateSnapshotExactV1(snapshotRef); err != nil {
		return contract.ToolDescriptor{}, registry.Record{}, err
	}
	return value, record, nil
}

func (s *SDKV1) validateSnapshotExactV1(expected RegistrySnapshotRefV1) error {
	snapshot, err := s.registry.Snapshot()
	if err != nil {
		return err
	}
	if snapshot.Revision != expected.Revision || snapshot.Digest != expected.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Registry Snapshot differs from the exact SDK request")
	}
	return nil
}

func (s *SDKV1) CompileToolSurfaceV1(ctx context.Context, request CompileSurfaceRequestV1) (contract.ToolSurfaceManifest, error) {
	now, err := s.readyV1(ctx)
	if err != nil {
		return contract.ToolSurfaceManifest{}, err
	}
	if err = request.Validate(now); err != nil {
		return contract.ToolSurfaceManifest{}, err
	}
	if err := s.validateSnapshotExactV1(request.RegistrySnapshot); err != nil {
		return contract.ToolSurfaceManifest{}, err
	}
	selections := make([]surface.Selection, 0, len(request.Selections))
	for _, selected := range request.Selections {
		capability, capabilityRecord, err := s.inspectCapabilityExactV1(selected.Capability)
		if err != nil {
			return contract.ToolSurfaceManifest{}, err
		}
		tool, toolRecord, err := s.inspectToolExactV1(selected.Tool)
		if err != nil {
			return contract.ToolSurfaceManifest{}, err
		}
		if capabilityRecord.State != registry.StateActive || toolRecord.State != registry.StateActive {
			return contract.ToolSurfaceManifest{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "Surface requires active exact Capability and Tool")
		}
		selections = append(selections, surface.Selection{
			Capability: capability, Tool: tool, ModelName: selected.ModelName, DescriptionDigest: selected.DescriptionDigest,
			Visible: selected.Visible, Allowed: selected.Allowed, PreApproved: selected.PreApproved,
		})
	}
	if err := s.validateSnapshotExactV1(request.RegistrySnapshot); err != nil {
		return contract.ToolSurfaceManifest{}, err
	}
	final, err := s.readyV1(ctx)
	if err != nil {
		return contract.ToolSurfaceManifest{}, err
	}
	if final.Before(now) {
		return contract.ToolSurfaceManifest{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool SDK clock regressed during Surface compilation")
	}
	if final.UnixNano() >= request.RequestedExpiresUnixNano {
		return contract.ToolSurfaceManifest{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Tool SDK crossed Surface compilation window")
	}
	return surface.Compile(surface.CompileRequest{
		Owner: request.Owner, ResolvedPlanDigest: request.ResolvedPlanDigest, ProfileDigest: request.ProfileDigest,
		CapabilityGrantDigest: request.CapabilityGrantDigest, RegistrySnapshotDigest: request.RegistrySnapshot.Digest,
		Dialect: request.Dialect, Selections: selections, Revision: request.Revision, CreatedAt: final,
		ExpiresAt: time.Unix(0, request.RequestedExpiresUnixNano),
	})
}

func (s *SDKV1) readyV1(ctx context.Context) (time.Time, error) {
	if s == nil || nilLikeV1(s.registry) || s.clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Tool SDK is unavailable")
	}
	if ctx == nil {
		return time.Time{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool SDK context is required")
	}
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	now := s.clock()
	if now.IsZero() {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool SDK clock is unavailable")
	}
	return now.UTC(), nil
}

func nilLikeV1(value any) bool {
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
