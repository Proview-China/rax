package registry

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPToolMappingAdmissionResultV1 struct {
	Mapping    Record `json:"mapping"`
	Capability Record `json:"capability"`
	Tool       Record `json:"tool"`
}

func (r *Registry) InspectMCPToolMappingManifestV1(ctx context.Context, exact toolcontract.MCPToolMappingManifestRefV1) (toolcontract.MCPToolMappingManifestV1, error) {
	if ctx == nil {
		return toolcontract.MCPToolMappingManifestV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Tool Mapping exact Inspect context is required")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPToolMappingManifestV1{}, err
	}
	if r == nil || exact.Validate() != nil {
		return toolcontract.MCPToolMappingManifestV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Tool Mapping exact Inspect is invalid")
	}
	value, _, ok := r.InspectMCPToolMapping(exact)
	if !ok {
		return toolcontract.MCPToolMappingManifestV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Tool Mapping Manifest not found")
	}
	return value, nil
}

var _ toolcontract.MCPToolMappingManifestExactReaderV1 = (*Registry)(nil)

type MCPToolMappingAdmissionServiceV1 struct {
	registry  *Registry
	snapshots toolcontract.MCPCapabilitySnapshotExactReaderV3
	materials toolcontract.MCPToolDiscoveryMaterialExactReaderV1
	clock     func() time.Time
}

func NewMCPToolMappingAdmissionServiceV1(registry *Registry, snapshots toolcontract.MCPCapabilitySnapshotExactReaderV3, materials toolcontract.MCPToolDiscoveryMaterialExactReaderV1, clock func() time.Time) (*MCPToolMappingAdmissionServiceV1, error) {
	if registry == nil || nilLikeMCPToolMappingV1(snapshots) || nilLikeMCPToolMappingV1(materials) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Tool Mapping Admission dependencies are required")
	}
	return &MCPToolMappingAdmissionServiceV1{registry: registry, snapshots: snapshots, materials: materials, clock: clock}, nil
}

func (s *MCPToolMappingAdmissionServiceV1) AdmitMCPToolMappingV1(ctx context.Context, request toolcontract.MCPToolMappingAdmissionRequestV1) (MCPToolMappingAdmissionResultV1, error) {
	if ctx == nil {
		return MCPToolMappingAdmissionResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Tool Mapping Admission context is required")
	}
	if err := ctx.Err(); err != nil {
		return MCPToolMappingAdmissionResultV1{}, err
	}
	if s == nil || s.registry == nil || nilLikeMCPToolMappingV1(s.snapshots) || nilLikeMCPToolMappingV1(s.materials) || s.clock == nil {
		return MCPToolMappingAdmissionResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Tool Mapping Admission is unavailable")
	}
	s1 := s.clock()
	if err := request.ValidateCurrent(s1); err != nil {
		return MCPToolMappingAdmissionResultV1{}, err
	}
	mapping, _, ok := s.registry.InspectMCPToolMapping(request.Mapping)
	if !ok {
		return MCPToolMappingAdmissionResultV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Tool Mapping Manifest not found")
	}
	firstSnapshot, err := s.snapshots.InspectMCPCapabilitySnapshotV3(ctx, mapping.Snapshot)
	if err != nil {
		return MCPToolMappingAdmissionResultV1{}, err
	}
	firstMaterial, err := s.materials.InspectExactMCPToolDiscoveryMaterialV1(ctx, mapping.SourceMaterial)
	if err != nil {
		return MCPToolMappingAdmissionResultV1{}, err
	}
	if err := s.validateClosureV1(mapping, firstSnapshot, firstMaterial, s1); err != nil {
		return MCPToolMappingAdmissionResultV1{}, err
	}
	secondSnapshot, err := s.snapshots.InspectMCPCapabilitySnapshotV3(ctx, mapping.Snapshot)
	if err != nil {
		return MCPToolMappingAdmissionResultV1{}, err
	}
	secondMaterial, err := s.materials.InspectExactMCPToolDiscoveryMaterialV1(ctx, mapping.SourceMaterial)
	if err != nil {
		return MCPToolMappingAdmissionResultV1{}, err
	}
	s2 := s.clock()
	if s2.IsZero() || s2.Before(s1) {
		return MCPToolMappingAdmissionResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Tool Mapping Admission clock regressed")
	}
	if !reflect.DeepEqual(firstSnapshot, secondSnapshot) || !reflect.DeepEqual(firstMaterial, secondMaterial) {
		return MCPToolMappingAdmissionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Tool Mapping source drifted between S1 and S2")
	}
	if err := s.validateClosureV1(mapping, secondSnapshot, secondMaterial, s2); err != nil {
		return MCPToolMappingAdmissionResultV1{}, err
	}
	return s.registry.admitMCPToolMappingCASV1(ctx, request, secondSnapshot, secondMaterial, s2)
}

func (s *MCPToolMappingAdmissionServiceV1) validateClosureV1(mapping toolcontract.MCPToolMappingManifestV1, snapshot toolcontract.MCPCapabilitySnapshotV3, material toolcontract.MCPToolDiscoveryMaterialV1, now time.Time) error {
	if snapshot.ValidateCurrent(now) != nil || snapshot.ObjectRef() != mapping.Snapshot || material.Validate() != nil || material.Ref != mapping.SourceMaterial {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Tool Mapping source is not exact-current")
	}
	capability, capabilityRecord, capabilityOK := s.registry.ResolveCapability(mapping.Capability.ID)
	tool, toolRecord, toolOK := s.registry.ResolveTool(mapping.Tool.ID)
	if !capabilityOK || !toolOK || capabilityRecord.ObjectRevision != mapping.Capability.Revision || capabilityRecord.ObjectDigest != mapping.Capability.Digest || toolRecord.ObjectRevision != mapping.Tool.Revision || toolRecord.ObjectDigest != mapping.Tool.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Tool Mapping Registry targets drifted")
	}
	return mapping.ValidateAgainst(snapshot, material, capability, tool)
}

func (r *Registry) admitMCPToolMappingCASV1(ctx context.Context, request toolcontract.MCPToolMappingAdmissionRequestV1, snapshot toolcontract.MCPCapabilitySnapshotV3, material toolcontract.MCPToolDiscoveryMaterialV1, now time.Time) (MCPToolMappingAdmissionResultV1, error) {
	if err := request.ValidateCurrent(now); err != nil {
		return MCPToolMappingAdmissionResultV1{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return MCPToolMappingAdmissionResultV1{}, err
	}
	mapping, ok := r.mcpMappings[request.Mapping.ID]
	if !ok || mapping.Ref != request.Mapping {
		return MCPToolMappingAdmissionResultV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Tool Mapping Manifest not found at CAS")
	}
	capability, capabilityOK := r.capabilities[mapping.Capability.ID]
	tool, toolOK := r.tools[mapping.Tool.ID]
	if !capabilityOK || !toolOK || mapping.ValidateAgainst(snapshot, material, capability, tool) != nil {
		return MCPToolMappingAdmissionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Tool Mapping CAS closure drifted")
	}
	mappingKey, capabilityKey, toolKey := key("mcp-tool-mapping", mapping.Ref.ID), key("capability", mapping.Capability.ID), key("tool", mapping.Tool.ID)
	mappingRecord, capabilityRecord, toolRecord := r.records[mappingKey], r.records[capabilityKey], r.records[toolKey]
	if mappingRecord.State == StateAdmitted && capabilityRecord.State == StateAdmitted && toolRecord.State == StateAdmitted && mappingRecord.RegistryRevision == capabilityRecord.RegistryRevision && mappingRecord.RegistryRevision == toolRecord.RegistryRevision && mappingRecord.UpdatedUnixNano == capabilityRecord.UpdatedUnixNano && mappingRecord.UpdatedUnixNano == toolRecord.UpdatedUnixNano {
		return MCPToolMappingAdmissionResultV1{Mapping: mappingRecord, Capability: capabilityRecord, Tool: toolRecord}, nil
	}
	if mappingRecord.RegistryRevision != request.ExpectedMappingRegistryRevision || capabilityRecord.RegistryRevision != request.ExpectedCapabilityRegistryRevision || toolRecord.RegistryRevision != request.ExpectedToolRegistryRevision || mappingRecord.State != StateSubmitted || capabilityRecord.State != StateSubmitted || toolRecord.State != StateSubmitted || mappingRecord.ObjectRevision != mapping.Ref.Revision || mappingRecord.ObjectDigest != mapping.Ref.Digest || capabilityRecord.ObjectRevision != mapping.Capability.Revision || capabilityRecord.ObjectDigest != mapping.Capability.Digest || toolRecord.ObjectRevision != mapping.Tool.Revision || toolRecord.ObjectDigest != mapping.Tool.Digest {
		return MCPToolMappingAdmissionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Tool Mapping Admission expected Registry closure drifted")
	}
	if now.UnixNano() < mappingRecord.UpdatedUnixNano || now.UnixNano() < capabilityRecord.UpdatedUnixNano || now.UnixNano() < toolRecord.UpdatedUnixNano || !now.Before(time.Unix(0, snapshot.ExpiresUnixNano)) || !now.Before(time.Unix(0, request.RequestedExpiresUnixNano)) {
		return MCPToolMappingAdmissionResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Tool Mapping Admission time is invalid or expired")
	}
	r.revision++
	for _, record := range []*Record{&mappingRecord, &capabilityRecord, &toolRecord} {
		record.State = StateAdmitted
		record.RegistryRevision = r.revision
		record.UpdatedUnixNano = now.UnixNano()
	}
	r.records[mappingKey], r.records[capabilityKey], r.records[toolKey] = mappingRecord, capabilityRecord, toolRecord
	return MCPToolMappingAdmissionResultV1{Mapping: mappingRecord, Capability: capabilityRecord, Tool: toolRecord}, nil
}

func nilLikeMCPToolMappingV1(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
