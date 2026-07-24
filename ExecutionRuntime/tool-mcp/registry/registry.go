package registry

import (
	"sort"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type State string

const (
	StateSubmitted  State = "submitted"
	StateAdmitted   State = "admitted"
	StateActive     State = "active"
	StateDeprecated State = "deprecated"
	StateRevoked    State = "revoked"
)

type Record struct {
	Kind             string        `json:"kind"`
	ID               string        `json:"id"`
	ObjectRevision   core.Revision `json:"object_revision"`
	ObjectDigest     core.Digest   `json:"object_digest"`
	State            State         `json:"state"`
	RegistryRevision core.Revision `json:"registry_revision"`
	UpdatedUnixNano  int64         `json:"updated_unix_nano"`
}

func (r Record) Validate() error {
	if !contract.ValidObjectID(r.ID) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "registry id is invalid")
	}
	if r.ObjectRevision == 0 || r.RegistryRevision == 0 || r.UpdatedUnixNano <= 0 || r.ObjectDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "registry record is incomplete")
	}
	switch r.State {
	case StateSubmitted, StateAdmitted, StateActive, StateDeprecated, StateRevoked:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "registry state is invalid")
	}
}

type Snapshot struct {
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
	Records  []Record      `json:"records"`
}

type Registry struct {
	mu                sync.RWMutex
	revision          core.Revision
	capabilities      map[string]contract.CapabilityDescriptor
	tools             map[string]contract.ToolDescriptor
	packages          map[string]contract.ToolPackageManifest
	aliases           map[string]map[core.Revision]contract.ToolAliasV1
	aliasRecords      map[string]map[core.Revision]Record
	aliasCurrent      map[string]contract.ToolAliasRefV1
	mcpMappings       map[string]contract.MCPToolMappingManifestV1
	packageAdmissions map[string]contract.ToolPackageVerificationCurrentRefV1
	records           map[string]Record
}

func New() *Registry {
	return &Registry{
		capabilities:      make(map[string]contract.CapabilityDescriptor),
		tools:             make(map[string]contract.ToolDescriptor),
		packages:          make(map[string]contract.ToolPackageManifest),
		aliases:           make(map[string]map[core.Revision]contract.ToolAliasV1),
		aliasRecords:      make(map[string]map[core.Revision]Record),
		aliasCurrent:      make(map[string]contract.ToolAliasRefV1),
		mcpMappings:       make(map[string]contract.MCPToolMappingManifestV1),
		packageAdmissions: make(map[string]contract.ToolPackageVerificationCurrentRefV1),
		records:           make(map[string]Record),
	}
}

func key(kind, id string) string { return kind + ":" + id }

func (r *Registry) SubmitCapability(value contract.CapabilityDescriptor, now time.Time) (Record, error) {
	if err := value.Validate(); err != nil {
		return Record{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.submitLocked("capability", string(value.ID), value.Revision, value.Digest, now, func() { r.capabilities[string(value.ID)] = cloneCapability(value) })
}

func (r *Registry) SubmitTool(value contract.ToolDescriptor, now time.Time) (Record, error) {
	if err := value.Validate(); err != nil {
		return Record{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	capability, ok := r.capabilities[value.Capability.ID]
	if !ok || value.ValidateAgainst(capability) != nil {
		return Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "tool capability is absent or differs")
	}
	return r.submitLocked("tool", string(value.ID), value.Revision, value.Digest, now, func() { r.tools[string(value.ID)] = cloneTool(value) })
}

func (r *Registry) SubmitPackage(value contract.ToolPackageManifest, now time.Time) (Record, error) {
	if err := value.Validate(); err != nil {
		return Record{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, descriptor := range value.Descriptors {
		tool, ok := r.tools[string(descriptor.ToolID)]
		if !ok || tool.Revision != descriptor.Revision || tool.Digest != descriptor.Digest {
			return Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "package references an absent or drifting tool")
		}
	}
	return r.submitLocked("package", string(value.ID), value.Revision, value.Digest, now, func() { r.packages[string(value.ID)] = clonePackage(value) })
}

func (r *Registry) SubmitMCPToolMapping(value contract.MCPToolMappingManifestV1, now time.Time) (Record, error) {
	if err := value.Validate(); err != nil {
		return Record{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	capability, capabilityOK := r.capabilities[value.Capability.ID]
	tool, toolOK := r.tools[value.Tool.ID]
	if !capabilityOK || !toolOK || capability.Revision != value.Capability.Revision || capability.Digest != value.Capability.Digest || tool.Revision != value.Tool.Revision || tool.Digest != value.Tool.Digest || tool.Owner != value.Owner || tool.Mechanism != contract.MechanismMCP || tool.ValidateAgainst(capability) != nil {
		return Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "MCP Tool Mapping target descriptors are absent or drifted")
	}
	return r.submitLocked("mcp-tool-mapping", value.Ref.ID, value.Ref.Revision, value.Ref.Digest, now, func() { r.mcpMappings[value.Ref.ID] = value })
}

// SubmitToolAlias creates or advances one assembly-time Alias under the same
// Registry lock and revision domain as Tool records. Alias facts never make a
// Tool active and cannot target an inactive or drifting Tool.
func (r *Registry) SubmitToolAlias(value contract.ToolAliasV1, expectedCurrent *contract.ToolAliasRefV1, now time.Time) (Record, error) {
	if err := value.ValidateAt(now); err != nil {
		return Record{}, err
	}
	if expectedCurrent != nil && expectedCurrent.Validate() != nil {
		return Record{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Alias expected current Ref is invalid")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	tool, ok := r.tools[value.Tool.ID]
	toolRecord := r.records[key("tool", value.Tool.ID)]
	if !ok || tool.Revision != value.Tool.Revision || tool.Digest != value.Tool.Digest || toolRecord.State != StateActive || toolRecord.ObjectRevision != value.Tool.Revision || toolRecord.ObjectDigest != value.Tool.Digest {
		return Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "Tool Alias target is not an active exact Tool")
	}
	id := value.Ref.ID
	currentRef, exists := r.aliasCurrent[id]
	if exists {
		current := r.aliases[id][currentRef.Revision]
		currentRecord := r.aliasRecords[id][currentRef.Revision]
		if value.Ref == currentRef {
			if current != value {
				return Record{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Alias current Ref binds different content")
			}
			return currentRecord, nil
		}
		if _, duplicatedRevision := r.aliases[id][value.Ref.Revision]; duplicatedRevision || value.Ref.Revision <= currentRef.Revision {
			return Record{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Tool Alias revision cannot roll back or change history")
		}
		if expectedCurrent == nil || *expectedCurrent != currentRef {
			return Record{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Tool Alias expected current Ref differs")
		}
		if currentRecord.State != StateActive {
			return Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "Tool Alias current mapping is not active")
		}
		if value.Ref.Revision != currentRef.Revision+1 || value.Owner != current.Owner || value.Alias != current.Alias || value.CreatedUnixNano < current.CreatedUnixNano {
			return Record{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Tool Alias successor identity or revision drifted")
		}
	} else if expectedCurrent != nil || value.Ref.Revision != 1 {
		return Record{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Tool Alias create requires revision 1 and no expected current")
	}
	if r.aliases[id] == nil {
		r.aliases[id] = make(map[core.Revision]contract.ToolAliasV1)
		r.aliasRecords[id] = make(map[core.Revision]Record)
	}
	r.revision++
	record := Record{
		Kind: "tool-alias", ID: id, ObjectRevision: value.Ref.Revision, ObjectDigest: value.Ref.Digest,
		State: StateActive, RegistryRevision: r.revision, UpdatedUnixNano: now.UTC().UnixNano(),
	}
	r.aliases[id][value.Ref.Revision] = value
	r.aliasRecords[id][value.Ref.Revision] = record
	r.aliasCurrent[id] = value.Ref
	r.records[key("tool-alias", id)] = record
	return record, nil
}

func (r *Registry) submitLocked(kind, id string, revision core.Revision, digest core.Digest, now time.Time, store func()) (Record, error) {
	if now.IsZero() {
		return Record{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "registry time is required")
	}
	k := key(kind, id)
	if existing, ok := r.records[k]; ok {
		if existing.ObjectRevision == revision && existing.ObjectDigest == digest {
			return existing, nil
		}
		return Record{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "registry object id already binds different content")
	}
	r.revision++
	record := Record{Kind: kind, ID: id, ObjectRevision: revision, ObjectDigest: digest, State: StateSubmitted, RegistryRevision: r.revision, UpdatedUnixNano: now.UTC().UnixNano()}
	store()
	r.records[k] = record
	return record, nil
}

func (r *Registry) Transition(kind, id string, expected core.Revision, target State, now time.Time) (Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(kind, id)
	current, ok := r.records[k]
	if !ok {
		return Record{}, core.NewError(core.ErrorNotFound, core.ReasonUnknownCapability, "registry object not found")
	}
	if current.RegistryRevision != expected {
		return Record{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "registry CAS revision differs")
	}
	if current.State == target {
		return current, nil
	}
	if kind == "package" && (target == StateAdmitted || target == StateActive) {
		return Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "Package requires verification-aware Admission and separate governed Enable")
	}
	if kind == "mcp-tool-mapping" && (target == StateAdmitted || target == StateActive) {
		return Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "MCP Tool Mapping requires mapping-aware Admission")
	}
	if kind == "tool" && (target == StateAdmitted || target == StateActive) {
		if tool, exists := r.tools[id]; exists && tool.Mechanism == contract.MechanismMCP {
			return Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "MCP Tool requires an admitted exact Mapping Manifest")
		}
	}
	if !allowedTransition(current.State, target) {
		return Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "registry transition is not monotonic")
	}
	if target == StateAdmitted || target == StateActive {
		if err := r.dependenciesReadyLocked(kind, id, target); err != nil {
			return Record{}, err
		}
	}
	if now.IsZero() || now.UTC().UnixNano() < current.UpdatedUnixNano {
		return Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "registry clock regressed")
	}
	r.revision++
	current.State = target
	current.RegistryRevision = r.revision
	current.UpdatedUnixNano = now.UTC().UnixNano()
	r.records[k] = current
	if kind == "tool-alias" {
		if exact, ok := r.aliasCurrent[id]; ok {
			r.aliasRecords[id][exact.Revision] = current
		}
	}
	return current, nil
}

// AdmitVerifiedPackageV1 is the only production-neutral Package Admission
// write. It persists the exact Verification current Ref under the same Registry
// lock and successor revision as the submitted -> admitted transition. Active
// enablement remains a separate governed operation.
func (r *Registry) AdmitVerifiedPackageV1(request contract.ToolPackageVerifiedAdmissionRequestV1, now time.Time) (Record, error) {
	if err := request.ValidateCurrent(now); err != nil {
		return Record{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id := request.PackageCurrent.Package.ID
	k := key("package", id)
	current, ok := r.records[k]
	manifest, manifestOK := r.packages[id]
	if !ok || !manifestOK || current.Kind != "package" || current.ID != id || current.ObjectRevision != request.PackageCurrent.Package.Revision || current.ObjectDigest != request.PackageCurrent.Package.Digest || manifest.Revision != request.PackageCurrent.Package.Revision || manifest.Digest != request.PackageCurrent.Package.Digest {
		return Record{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "verified Package Admission source drifted")
	}
	if exact, admitted := r.packageAdmissions[id]; admitted {
		if exact != request.VerificationCurrent.Ref || current.State != StateAdmitted {
			return Record{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Package Admission already binds different Verification")
		}
		return current, nil
	}
	if current.RegistryRevision != request.ExpectedRegistryRevision || current.RegistryRevision != request.PackageCurrent.RegistryRevision || current.State != StateSubmitted {
		return Record{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "verified Package Admission CAS source differs")
	}
	if err := r.dependenciesReadyLocked("package", id, StateAdmitted); err != nil {
		return Record{}, err
	}
	if now.IsZero() || now.UnixNano() < current.UpdatedUnixNano {
		return Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "verified Package Admission clock regressed")
	}
	r.revision++
	current.State = StateAdmitted
	current.RegistryRevision = r.revision
	current.UpdatedUnixNano = now.UnixNano()
	r.records[k] = current
	r.packageAdmissions[id] = request.VerificationCurrent.Ref
	return current, nil
}

func (r *Registry) InspectVerifiedPackageAdmissionV1(pkg contract.ObjectRef, verification contract.ToolPackageVerificationCurrentRefV1) (Record, bool) {
	if pkg.Validate() != nil || verification.Validate() != nil {
		return Record{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	exact, ok := r.packageAdmissions[pkg.ID]
	record := r.records[key("package", pkg.ID)]
	if !ok || exact != verification || record.State != StateAdmitted || record.ObjectRevision != pkg.Revision || record.ObjectDigest != pkg.Digest {
		return Record{}, false
	}
	return record, true
}

func (r *Registry) dependenciesReadyLocked(kind, id string, target State) error {
	required := StateAdmitted
	if target == StateActive {
		required = StateActive
	}
	switch kind {
	case "tool":
		tool, ok := r.tools[id]
		if !ok {
			return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "tool descriptor is absent")
		}
		capability := r.records[key("capability", tool.Capability.ID)]
		if !stateAtLeast(capability.State, required) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "tool capability is not admitted at the required state")
		}
	case "package":
		pkg, ok := r.packages[id]
		if !ok {
			return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "package manifest is absent")
		}
		for _, descriptor := range pkg.Descriptors {
			tool := r.records[key("tool", string(descriptor.ToolID))]
			if !stateAtLeast(tool.State, required) {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "package tool is not admitted at the required state")
			}
		}
	}
	return nil
}

func stateAtLeast(actual, required State) bool {
	if actual == StateRevoked {
		return false
	}
	if required == StateAdmitted {
		return actual == StateAdmitted || actual == StateActive || actual == StateDeprecated
	}
	return actual == StateActive
}

func allowedTransition(from, to State) bool {
	switch from {
	case StateSubmitted:
		return to == StateAdmitted || to == StateRevoked
	case StateAdmitted:
		return to == StateActive || to == StateRevoked
	case StateActive:
		return to == StateDeprecated || to == StateRevoked
	case StateDeprecated:
		return to == StateRevoked
	default:
		return false
	}
}

func (r *Registry) ResolveCapability(id string) (contract.CapabilityDescriptor, Record, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.capabilities[id]
	if !ok {
		return contract.CapabilityDescriptor{}, Record{}, false
	}
	return cloneCapability(value), r.records[key("capability", id)], true
}

func (r *Registry) ResolveTool(id string) (contract.ToolDescriptor, Record, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.tools[id]
	if !ok {
		return contract.ToolDescriptor{}, Record{}, false
	}
	return cloneTool(value), r.records[key("tool", id)], true
}

func (r *Registry) ResolvePackage(id string) (contract.ToolPackageManifest, Record, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.packages[id]
	if !ok {
		return contract.ToolPackageManifest{}, Record{}, false
	}
	return clonePackage(value), r.records[key("package", id)], true
}

// InspectExactPackageRecordV1 returns a Package and its Registry record under
// one read lock. It is an owner-local exact seam for current projections; it
// does not resolve aliases or mutate Registry state.
func (r *Registry) InspectExactPackageRecordV1(exact contract.ObjectRef, expectedRegistryRevision core.Revision, expectedRecordDigest core.Digest) (contract.ToolPackageManifest, Record, bool) {
	if exact.Validate() != nil || expectedRegistryRevision == 0 || expectedRecordDigest.Validate() != nil {
		return contract.ToolPackageManifest{}, Record{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.packages[exact.ID]
	record := r.records[key("package", exact.ID)]
	if !ok || value.Revision != exact.Revision || value.Digest != exact.Digest || record.Kind != "package" || record.ID != exact.ID || record.ObjectRevision != exact.Revision || record.ObjectDigest != exact.Digest || record.RegistryRevision != expectedRegistryRevision {
		return contract.ToolPackageManifest{}, Record{}, false
	}
	source, err := contract.SealToolPackageRegistryRecordSourceV1(contract.ToolPackageRegistryRecordSourceV1{
		Kind: record.Kind, ID: record.ID, ObjectRevision: record.ObjectRevision, ObjectDigest: record.ObjectDigest,
		State: string(record.State), RegistryRevision: record.RegistryRevision, UpdatedUnixNano: record.UpdatedUnixNano,
	})
	if err != nil || source.Digest != expectedRecordDigest {
		return contract.ToolPackageManifest{}, Record{}, false
	}
	return clonePackage(value), record, true
}

func (r *Registry) InspectMCPToolMapping(exact contract.MCPToolMappingManifestRefV1) (contract.MCPToolMappingManifestV1, Record, bool) {
	if exact.Validate() != nil {
		return contract.MCPToolMappingManifestV1{}, Record{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.mcpMappings[exact.ID]
	record := r.records[key("mcp-tool-mapping", exact.ID)]
	if !ok || value.Ref != exact || record.ObjectRevision != exact.Revision || record.ObjectDigest != exact.Digest {
		return contract.MCPToolMappingManifestV1{}, Record{}, false
	}
	return value, record, true
}

func (r *Registry) InspectToolAlias(exact contract.ToolAliasRefV1) (contract.ToolAliasV1, Record, bool) {
	if exact.Validate() != nil {
		return contract.ToolAliasV1{}, Record{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.aliases[exact.ID][exact.Revision]
	if !ok || value.Ref != exact {
		return contract.ToolAliasV1{}, Record{}, false
	}
	return value, r.aliasRecords[exact.ID][exact.Revision], true
}

func (r *Registry) ResolveToolAlias(id string) (contract.ToolAliasV1, Record, bool) {
	if contract.ValidateStableID(id) != nil {
		return contract.ToolAliasV1{}, Record{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	exact, ok := r.aliasCurrent[id]
	if !ok {
		return contract.ToolAliasV1{}, Record{}, false
	}
	value, ok := r.aliases[id][exact.Revision]
	if !ok || value.Ref != exact {
		return contract.ToolAliasV1{}, Record{}, false
	}
	return value, r.aliasRecords[id][exact.Revision], true
}

func cloneCapability(value contract.CapabilityDescriptor) contract.CapabilityDescriptor {
	value.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), value.EffectKinds...)
	return value
}

func cloneTool(value contract.ToolDescriptor) contract.ToolDescriptor {
	value.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), value.EffectKinds...)
	value.Residuals = append([]contract.Residual(nil), value.Residuals...)
	return value
}

func clonePackage(value contract.ToolPackageManifest) contract.ToolPackageManifest {
	value.Signatures = append([]core.Digest(nil), value.Signatures...)
	value.Descriptors = append([]contract.PackageDescriptorRef(nil), value.Descriptors...)
	value.EffectKinds = append([]runtimeports.NamespacedNameV2(nil), value.EffectKinds...)
	return value
}

func (r *Registry) Snapshot() (Snapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	records := make([]Record, 0, len(r.records))
	for _, record := range r.records {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Kind != records[j].Kind {
			return records[i].Kind < records[j].Kind
		}
		return records[i].ID < records[j].ID
	})
	digest, err := contract.Seal("praxis.tool-mcp.registry", "v1", "Snapshot", records)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Revision: r.revision, Digest: digest, Records: records}, nil
}
