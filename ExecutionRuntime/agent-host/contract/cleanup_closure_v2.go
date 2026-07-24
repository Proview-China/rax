package contract

import (
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	HostCleanupClosureContractVersionV2 = "praxis.agent-host/cleanup-closure/v2"
	HostCleanupPlanTemplateVersionV2    = "praxis.agent-host/cleanup-plan-template/v2"
	HostCleanupClosureRefKindV2         = "praxis.agent-host/cleanup-closure-v2"
	HostCleanupPlanTemplateRefKindV2    = "praxis.agent-host/cleanup-plan-template-v2"
	MaxHostCleanupClosureEntriesV2      = 4096
)

type HostCleanupClosureRefV2 struct {
	ClosureID      string     `json:"closure_id"`
	Revision       uint64     `json:"revision"`
	HostID         string     `json:"host_id"`
	StartID        string     `json:"start_id"`
	PlanRef        ExactRefV1 `json:"plan_ref"`
	CoverageDigest DigestV1   `json:"coverage_digest"`
	Digest         DigestV1   `json:"digest"`
}

func (r HostCleanupClosureRefV2) Validate() error {
	for field, value := range map[string]string{"closure id": r.ClosureID, "host id": r.HostID, "start id": r.StartID} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	if r.Revision != 1 {
		return NewError(ErrorInvalidArgument, "cleanup_closure_revision_invalid", "cleanup closure Ref revision must be one")
	}
	if err := r.PlanRef.Validate(); err != nil {
		return err
	}
	if err := r.CoverageDigest.Validate(); err != nil {
		return err
	}
	return r.Digest.Validate()
}

func DeriveHostCleanupClosureIDV2(hostID, startID string) (string, error) {
	if err := ValidateIdentifierV1("host id", hostID); err != nil {
		return "", err
	}
	if err := ValidateIdentifierV1("start id", startID); err != nil {
		return "", err
	}
	digest, err := DigestJSONV1(struct {
		HostID  string `json:"host_id"`
		StartID string `json:"start_id"`
	}{hostID, startID})
	if err != nil {
		return "", err
	}
	return "closure/" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

type HostCleanupClosureAssemblyCoordinateV2 struct {
	ScopeRef      string                         `json:"scope_ref"`
	AssemblyInput ExactRefV1                     `json:"assembly_input_ref"`
	Publication   ExactRefV1                     `json:"publication_ref"`
	Generation    ExactRefV1                     `json:"generation_ref"`
	Manifest      ExactRefV1                     `json:"manifest_ref"`
	Graph         ExactRefV1                     `json:"graph_ref"`
	Handoff       ExactRefV1                     `json:"handoff_ref"`
	OwnerCurrent  runtimeports.OwnerCurrentRefV1 `json:"owner_current_ref"`
}

func (v HostCleanupClosureAssemblyCoordinateV2) Validate() error {
	if err := ValidateIdentifierV1("assembly scope", v.ScopeRef); err != nil {
		return err
	}
	for _, ref := range []ExactRefV1{v.AssemblyInput, v.Publication, v.Generation, v.Manifest, v.Graph, v.Handoff} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return v.OwnerCurrent.Validate()
}

type HostCleanupClosureBindingCoordinateV2 struct {
	AttemptID          string                                       `json:"attempt_id"`
	RequestDigest      core.Digest                                  `json:"request_digest"`
	BindingSet         runtimeports.BindingAdmissionBindingSetRefV1 `json:"binding_set"`
	Bindings           []runtimeports.BindingAdmissionBindingRefV1  `json:"bindings"`
	ResourceBindingSet runtimeports.ResourceBindingSetRefV1         `json:"resource_binding_set"`
	CheckedUnixNano    int64                                        `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                                        `json:"expires_unix_nano"`
	ResultDigest       core.Digest                                  `json:"result_digest"`
}

func (v HostCleanupClosureBindingCoordinateV2) canonicalV2() HostCleanupClosureBindingCoordinateV2 {
	v.Bindings = append([]runtimeports.BindingAdmissionBindingRefV1{}, v.Bindings...)
	sort.Slice(v.Bindings, func(i, j int) bool { return v.Bindings[i].ComponentID < v.Bindings[j].ComponentID })
	return v
}

func (v HostCleanupClosureBindingCoordinateV2) Validate() error {
	if err := ValidateIdentifierV1("binding attempt id", v.AttemptID); err != nil {
		return err
	}
	if err := v.RequestDigest.Validate(); err != nil {
		return err
	}
	if err := v.BindingSet.Validate(); err != nil {
		return err
	}
	if err := v.ResourceBindingSet.Validate(); err != nil {
		return err
	}
	if v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano || v.BindingSet.ExpiresUnixNano != v.ExpiresUnixNano || v.ExpiresUnixNano > v.ResourceBindingSet.ExpiresUnixNano || len(v.Bindings) == 0 || len(v.Bindings) > MaxHostCleanupClosureEntriesV2 {
		return NewError(ErrorInvalidArgument, "cleanup_closure_binding_incomplete", "cleanup closure Binding coordinate is incomplete")
	}
	var previous runtimeports.ComponentIDV2
	for index, binding := range v.Bindings {
		if err := binding.Validate(); err != nil {
			return err
		}
		if index > 0 && binding.ComponentID <= previous {
			return NewError(ErrorConflict, "cleanup_closure_binding_not_canonical", "cleanup closure Bindings must be sorted and unique")
		}
		if binding.ExpiresUnixNano < v.ExpiresUnixNano {
			return NewError(ErrorPrecondition, "cleanup_closure_binding_expired", "cleanup closure exceeds a Binding current")
		}
		previous = binding.ComponentID
	}
	return v.ResultDigest.Validate()
}

type HostCleanupPlanTemplateRouteV2 struct {
	NodeID              string                                    `json:"node_id"`
	FactoryRef          ControlAdapterFactoryRefV2                `json:"factory_ref"`
	ComponentID         runtimeports.ComponentIDV2                `json:"component_id"`
	ArtifactDigest      core.Digest                               `json:"artifact_digest"`
	Capability          runtimeports.CapabilityNameV2             `json:"capability"`
	Binding             runtimeports.BindingAdmissionBindingRefV1 `json:"binding"`
	CleanupContractRef  ExactRefV1                                `json:"cleanup_contract_ref"`
	InspectPortBinding  ExactRefV1                                `json:"inspect_port_binding"`
	RequestSchemaDigest DigestV1                                  `json:"request_schema_digest"`
	ResultSchemaDigest  DigestV1                                  `json:"result_schema_digest"`
	ResourceClass       CleanupResourceClassV2                    `json:"resource_class"`
	RequiredBarrierIDs  []string                                  `json:"required_barrier_ids"`
}

func (r HostCleanupPlanTemplateRouteV2) canonicalV2() HostCleanupPlanTemplateRouteV2 {
	r.RequiredBarrierIDs = append([]string{}, r.RequiredBarrierIDs...)
	sort.Strings(r.RequiredBarrierIDs)
	return r
}

func (r HostCleanupPlanTemplateRouteV2) Validate() error {
	if err := ValidateIdentifierV1("cleanup route node id", r.NodeID); err != nil {
		return err
	}
	if err := r.FactoryRef.Validate(); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(r.ComponentID)); err != nil {
		return err
	}
	if err := r.ArtifactDigest.Validate(); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(r.Capability)); err != nil {
		return err
	}
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if r.Binding.ComponentID != r.ComponentID {
		return NewError(ErrorConflict, "cleanup_template_binding_component_drift", "cleanup route Binding component drifted")
	}
	for _, ref := range []ExactRefV1{r.CleanupContractRef, r.InspectPortBinding} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if err := r.RequestSchemaDigest.Validate(); err != nil {
		return err
	}
	if err := r.ResultSchemaDigest.Validate(); err != nil {
		return err
	}
	if !validCleanupResourceClassV2(r.ResourceClass) {
		return NewError(ErrorInvalidArgument, "cleanup_resource_class_invalid", "cleanup route resource class is unsupported")
	}
	for i, dependency := range r.RequiredBarrierIDs {
		if err := ValidateIdentifierV1("cleanup route dependency", dependency); err != nil {
			return err
		}
		if dependency == r.NodeID {
			return NewError(ErrorConflict, "cleanup_self_dependency", "cleanup route cannot depend on itself")
		}
		if i > 0 && r.RequiredBarrierIDs[i-1] >= dependency {
			return NewError(ErrorConflict, "cleanup_route_dependency_duplicate", "cleanup route dependencies must be sorted and unique")
		}
	}
	return nil
}

type HostCleanupPlanTemplateCurrentV2 struct {
	ContractVersion    string                               `json:"contract_version"`
	TemplateRef        ExactRefV1                           `json:"template_ref"`
	Routes             []HostCleanupPlanTemplateRouteV2     `json:"routes"`
	FixedBarriers      []CleanupNodeV2                      `json:"fixed_barriers"`
	ResourceBindingSet runtimeports.ResourceBindingSetRefV1 `json:"resource_binding_set"`
	CheckedUnixNano    int64                                `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                                `json:"expires_unix_nano"`
	Digest             DigestV1                             `json:"digest"`
}

func (v HostCleanupPlanTemplateCurrentV2) canonicalV2() HostCleanupPlanTemplateCurrentV2 {
	v.Routes = append([]HostCleanupPlanTemplateRouteV2{}, v.Routes...)
	for i := range v.Routes {
		v.Routes[i] = v.Routes[i].canonicalV2()
	}
	sort.Slice(v.Routes, func(i, j int) bool { return v.Routes[i].NodeID < v.Routes[j].NodeID })
	v.FixedBarriers = append([]CleanupNodeV2{}, v.FixedBarriers...)
	sort.Slice(v.FixedBarriers, func(i, j int) bool { return v.FixedBarriers[i].NodeID < v.FixedBarriers[j].NodeID })
	return v
}

func (v HostCleanupPlanTemplateCurrentV2) digestV2() (DigestV1, error) {
	v = v.canonicalV2()
	v.TemplateRef.Digest = ""
	v.Digest = ""
	return DigestJSONV1(struct {
		Domain string                           `json:"domain"`
		Type   string                           `json:"type"`
		Body   HostCleanupPlanTemplateCurrentV2 `json:"body"`
	}{"praxis.agent-host.cleanup-plan-template-v2", "HostCleanupPlanTemplateCurrentV2", v})
}

func SealHostCleanupPlanTemplateCurrentV2(v HostCleanupPlanTemplateCurrentV2) (HostCleanupPlanTemplateCurrentV2, error) {
	if v.ContractVersion != "" && v.ContractVersion != HostCleanupPlanTemplateVersionV2 {
		return HostCleanupPlanTemplateCurrentV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "cleanup Plan Template contract version drifted")
	}
	v.ContractVersion = HostCleanupPlanTemplateVersionV2
	v = v.canonicalV2()
	providedRef, provided := v.TemplateRef.Digest, v.Digest
	v.TemplateRef.Digest, v.Digest = "", ""
	digest, err := v.digestV2()
	if err != nil {
		return HostCleanupPlanTemplateCurrentV2{}, err
	}
	if (providedRef != "" && providedRef != digest) || (provided != "" && provided != digest) {
		return HostCleanupPlanTemplateCurrentV2{}, NewError(ErrorConflict, "cleanup_plan_template_digest_drift", "cleanup plan template supplied a wrong digest")
	}
	v.TemplateRef.Digest, v.Digest = digest, digest
	return v, v.Validate()
}

func (v HostCleanupPlanTemplateCurrentV2) Validate() error {
	if v.ContractVersion != HostCleanupPlanTemplateVersionV2 || v.TemplateRef.Kind != HostCleanupPlanTemplateRefKindV2 || v.TemplateRef.Revision == 0 || v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano || len(v.Routes) == 0 || len(v.Routes)+len(v.FixedBarriers) > MaxHostCleanupClosureEntriesV2 {
		return NewError(ErrorInvalidArgument, "cleanup_plan_template_incomplete", "cleanup plan template current is incomplete")
	}
	if err := v.TemplateRef.Validate(); err != nil {
		return err
	}
	if err := v.ResourceBindingSet.Validate(); err != nil {
		return err
	}
	if v.ExpiresUnixNano > v.ResourceBindingSet.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "cleanup_plan_template_ttl_drift", "cleanup plan template exceeds Resource BindingSet")
	}
	for i, route := range v.Routes {
		if err := route.Validate(); err != nil {
			return err
		}
		if i > 0 && v.Routes[i-1].NodeID >= route.NodeID {
			return NewError(ErrorConflict, "cleanup_plan_template_route_duplicate", "cleanup plan template routes must be sorted and unique")
		}
	}
	barriers := map[string]bool{}
	for index, barrier := range v.FixedBarriers {
		if err := barrier.Validate(); err != nil {
			return err
		}
		if barrier.Kind != CleanupBarrierNodeV2 || !isFixedCleanupBarrierV2(barrier.NodeID) {
			return NewError(ErrorConflict, "cleanup_template_fixed_barrier_invalid", "cleanup Plan Template has a non-fixed barrier")
		}
		if index > 0 && v.FixedBarriers[index-1].NodeID >= barrier.NodeID {
			return NewError(ErrorConflict, "cleanup_template_fixed_barrier_duplicate", "cleanup fixed barriers must be sorted and unique")
		}
		barriers[barrier.NodeID] = true
	}
	for _, id := range []string{CleanupBarrierHarnessCloseV2, CleanupBarrierSandboxFenceV2, CleanupBarrierSandboxReleaseV2, CleanupBarrierRuntimeCleanupAggregateV2} {
		if !barriers[id] {
			return NewError(ErrorPrecondition, "cleanup_template_fixed_barrier_missing", "cleanup Plan Template lacks a fixed barrier")
		}
	}
	expected, err := v.digestV2()
	if err != nil {
		return err
	}
	if expected != v.Digest || expected != v.TemplateRef.Digest {
		return NewError(ErrorConflict, "cleanup_plan_template_digest_drift", "cleanup plan template digest drifted")
	}
	return nil
}

func (v HostCleanupPlanTemplateCurrentV2) ValidateCurrent(now time.Time) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < v.CheckedUnixNano {
		return NewError(ErrorPrecondition, "clock_regression", "cleanup plan template clock regressed")
	}
	if now.UnixNano() >= v.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "cleanup_plan_template_expired", "cleanup plan template expired")
	}
	return nil
}

type HostCleanupClosureControlCoverageV2 struct {
	FactoryRef         ControlAdapterFactoryRefV2                `json:"factory_ref"`
	ComponentID        runtimeports.ComponentIDV2                `json:"component_id"`
	ArtifactDigest     core.Digest                               `json:"artifact_digest"`
	Capability         runtimeports.CapabilityNameV2             `json:"capability"`
	Binding            runtimeports.BindingAdmissionBindingRefV1 `json:"binding"`
	Generation         runtimeports.OwnerCurrentRefV1            `json:"generation"`
	ResourceBindingSet runtimeports.ResourceBindingSetRefV1      `json:"resource_binding_set"`
	ResourceHandles    []runtimeports.ResourceHandleRefV1        `json:"resource_handles"`
	CleanupNodeIDs     []string                                  `json:"cleanup_node_ids"`
}

func (c HostCleanupClosureControlCoverageV2) canonicalV2() HostCleanupClosureControlCoverageV2 {
	c.ResourceHandles = append([]runtimeports.ResourceHandleRefV1{}, c.ResourceHandles...)
	sort.Slice(c.ResourceHandles, func(i, j int) bool {
		return ownerHandleKeyV2(c.ResourceHandles[i]) < ownerHandleKeyV2(c.ResourceHandles[j])
	})
	c.CleanupNodeIDs = append([]string{}, c.CleanupNodeIDs...)
	sort.Strings(c.CleanupNodeIDs)
	return c
}

func (c HostCleanupClosureControlCoverageV2) Validate() error {
	if err := c.FactoryRef.Validate(); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(c.ComponentID)); err != nil {
		return err
	}
	if err := c.ArtifactDigest.Validate(); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(c.Capability)); err != nil {
		return err
	}
	if err := c.Binding.Validate(); err != nil {
		return err
	}
	if c.Binding.ComponentID != c.ComponentID {
		return NewError(ErrorConflict, "cleanup_control_binding_component_drift", "cleanup control Binding component drifted")
	}
	if err := c.Generation.Validate(); err != nil {
		return err
	}
	if err := c.ResourceBindingSet.Validate(); err != nil {
		return err
	}
	if len(c.ResourceHandles) == 0 || len(c.CleanupNodeIDs) == 0 {
		return NewError(ErrorInvalidArgument, "cleanup_control_coverage_incomplete", "cleanup control coverage requires resources and nodes")
	}
	for i, handle := range c.ResourceHandles {
		if err := handle.Validate(); err != nil {
			return err
		}
		if i > 0 && ownerHandleKeyV2(c.ResourceHandles[i-1]) >= ownerHandleKeyV2(handle) {
			return NewError(ErrorConflict, "cleanup_control_resource_duplicate", "cleanup control resources must be sorted and unique")
		}
	}
	for i, id := range c.CleanupNodeIDs {
		if err := ValidateIdentifierV1("cleanup node id", id); err != nil {
			return err
		}
		if i > 0 && c.CleanupNodeIDs[i-1] >= id {
			return NewError(ErrorConflict, "cleanup_control_node_duplicate", "cleanup control nodes must be sorted and unique")
		}
	}
	return nil
}

func ownerHandleKeyV2(r runtimeports.ResourceHandleRefV1) string {
	return r.Owner.Domain + "\x00" + string(r.Owner.ID) + "\x00" + r.ID
}

type HostCleanupClosureCoverageSourceKindV2 string

const (
	HostCleanupCoverageBindingV2  HostCleanupClosureCoverageSourceKindV2 = "binding_component"
	HostCleanupCoverageControlV2  HostCleanupClosureCoverageSourceKindV2 = "control_factory"
	HostCleanupCoverageResourceV2 HostCleanupClosureCoverageSourceKindV2 = "resource_handle"
	HostCleanupCoverageBarrierV2  HostCleanupClosureCoverageSourceKindV2 = "fixed_barrier"
)

type HostCleanupClosureCoverageEntryV2 struct {
	SourceKind     HostCleanupClosureCoverageSourceKindV2 `json:"source_kind"`
	SourceID       string                                 `json:"source_id"`
	SourceRevision uint64                                 `json:"source_revision"`
	SourceDigest   DigestV1                               `json:"source_digest"`
	ComponentID    string                                 `json:"component_id"`
	ResourceClass  CleanupResourceClassV2                 `json:"resource_class"`
	CleanupNodeID  string                                 `json:"cleanup_node_id"`
}

func (e HostCleanupClosureCoverageEntryV2) Validate() error {
	switch e.SourceKind {
	case HostCleanupCoverageBindingV2, HostCleanupCoverageControlV2, HostCleanupCoverageResourceV2, HostCleanupCoverageBarrierV2:
	default:
		return NewError(ErrorInvalidArgument, "cleanup_coverage_source_invalid", "cleanup coverage source kind is unsupported")
	}
	for field, value := range map[string]string{"source id": e.SourceID, "component id": e.ComponentID, "cleanup node id": e.CleanupNodeID} {
		if err := ValidateIdentifierV1(field, value); err != nil {
			return err
		}
	}
	if e.SourceRevision == 0 {
		return NewError(ErrorInvalidArgument, "cleanup_coverage_revision_invalid", "cleanup coverage source revision is required")
	}
	if err := e.SourceDigest.Validate(); err != nil {
		return err
	}
	if !validCleanupResourceClassV2(e.ResourceClass) {
		return NewError(ErrorInvalidArgument, "cleanup_resource_class_invalid", "cleanup coverage resource class is unsupported")
	}
	return nil
}

func cleanupCoverageKeyV2(e HostCleanupClosureCoverageEntryV2) string {
	return string(e.SourceKind) + "\x00" + e.SourceID + "\x00" + e.CleanupNodeID
}

type HostCleanupClosureFactV2 struct {
	ContractVersion string                                 `json:"contract_version"`
	ClosureID       string                                 `json:"closure_id"`
	Revision        uint64                                 `json:"revision"`
	StartClaimRef   HostStartClaimRefV1                    `json:"start_claim_ref"`
	Assembly        HostCleanupClosureAssemblyCoordinateV2 `json:"assembly"`
	Binding         HostCleanupClosureBindingCoordinateV2  `json:"binding"`
	PlanTemplate    HostCleanupPlanTemplateCurrentV2       `json:"cleanup_plan_template_current"`
	Controls        []HostCleanupClosureControlCoverageV2  `json:"controls"`
	Plan            CleanupPlanV2                          `json:"plan"`
	Coverage        []HostCleanupClosureCoverageEntryV2    `json:"coverage"`
	CoverageDigest  DigestV1                               `json:"coverage_digest"`
	CreatedUnixNano int64                                  `json:"created_unix_nano"`
	ContentDigest   DigestV1                               `json:"content_digest"`
}

func (f HostCleanupClosureFactV2) canonicalV2() HostCleanupClosureFactV2 {
	f.Binding = f.Binding.canonicalV2()
	f.PlanTemplate = f.PlanTemplate.canonicalV2()
	f.Controls = append([]HostCleanupClosureControlCoverageV2{}, f.Controls...)
	for i := range f.Controls {
		f.Controls[i] = f.Controls[i].canonicalV2()
	}
	sort.Slice(f.Controls, func(i, j int) bool { return f.Controls[i].FactoryRef.FactoryID < f.Controls[j].FactoryRef.FactoryID })
	f.Coverage = append([]HostCleanupClosureCoverageEntryV2{}, f.Coverage...)
	sort.Slice(f.Coverage, func(i, j int) bool { return cleanupCoverageKeyV2(f.Coverage[i]) < cleanupCoverageKeyV2(f.Coverage[j]) })
	f.Plan = f.Plan.canonicalV2()
	return f
}

func digestHostCleanupCoverageV2(entries []HostCleanupClosureCoverageEntryV2) (DigestV1, error) {
	copy := append([]HostCleanupClosureCoverageEntryV2{}, entries...)
	sort.Slice(copy, func(i, j int) bool { return cleanupCoverageKeyV2(copy[i]) < cleanupCoverageKeyV2(copy[j]) })
	return DigestJSONV1(struct {
		Domain string                              `json:"domain"`
		Type   string                              `json:"type"`
		Body   []HostCleanupClosureCoverageEntryV2 `json:"body"`
	}{"praxis.agent-host.cleanup-closure-coverage-v2", "HostCleanupClosureCoverageV2", copy})
}

func (f HostCleanupClosureFactV2) digestV2() (DigestV1, error) {
	f = f.canonicalV2()
	f.ContentDigest = ""
	return DigestJSONV1(struct {
		Domain string                   `json:"domain"`
		Type   string                   `json:"type"`
		Body   HostCleanupClosureFactV2 `json:"body"`
	}{"praxis.agent-host.cleanup-closure-v2", "HostCleanupClosureFactV2", f})
}

func SealHostCleanupClosureFactV2(f HostCleanupClosureFactV2) (HostCleanupClosureFactV2, error) {
	if f.ContractVersion != "" && f.ContractVersion != HostCleanupClosureContractVersionV2 {
		return HostCleanupClosureFactV2{}, NewError(ErrorInvalidArgument, "contract_version_mismatch", "cleanup Closure contract version drifted")
	}
	f.ContractVersion = HostCleanupClosureContractVersionV2
	f = f.canonicalV2()
	if f.ClosureID == "" {
		id, err := DeriveHostCleanupClosureIDV2(f.Plan.HostID, f.Plan.StartID)
		if err != nil {
			return HostCleanupClosureFactV2{}, err
		}
		f.ClosureID = id
	}
	coverage, err := digestHostCleanupCoverageV2(f.Coverage)
	if err != nil {
		return HostCleanupClosureFactV2{}, err
	}
	if f.CoverageDigest != "" && f.CoverageDigest != coverage {
		return HostCleanupClosureFactV2{}, NewError(ErrorConflict, "cleanup_coverage_digest_drift", "cleanup closure supplied a wrong coverage digest")
	}
	f.CoverageDigest = coverage
	provided := f.ContentDigest
	f.ContentDigest = ""
	digest, err := f.digestV2()
	if err != nil {
		return HostCleanupClosureFactV2{}, err
	}
	if provided != "" && provided != digest {
		return HostCleanupClosureFactV2{}, NewError(ErrorConflict, "cleanup_closure_digest_drift", "cleanup closure supplied a wrong content digest")
	}
	f.ContentDigest = digest
	return f, f.Validate()
}

func (f HostCleanupClosureFactV2) Validate() error {
	if f.ContractVersion != HostCleanupClosureContractVersionV2 || f.Revision != 1 || f.CreatedUnixNano <= 0 || len(f.Controls) == 0 || len(f.Coverage) == 0 || len(f.Controls) > MaxHostCleanupClosureEntriesV2 || len(f.Coverage) > MaxHostCleanupClosureEntriesV2 {
		return NewError(ErrorInvalidArgument, "cleanup_closure_incomplete", "cleanup closure Fact is incomplete")
	}
	if err := ValidateIdentifierV1("closure id", f.ClosureID); err != nil {
		return err
	}
	if err := f.StartClaimRef.Validate(); err != nil {
		return err
	}
	if err := f.Assembly.Validate(); err != nil {
		return err
	}
	if err := f.Binding.Validate(); err != nil {
		return err
	}
	if err := f.PlanTemplate.Validate(); err != nil {
		return err
	}
	if err := f.Plan.Validate(); err != nil {
		return err
	}
	if f.Plan.HostID != f.StartClaimRef.HostID || f.Plan.StartID != f.StartClaimRef.StartID {
		return NewError(ErrorConflict, "cleanup_closure_start_claim_drift", "cleanup plan does not belong to the claimed Host Start")
	}
	if f.CreatedUnixNano >= f.StartClaimRef.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "cleanup_closure_claim_expired", "cleanup closure was created after its Start Claim window")
	}
	if f.CreatedUnixNano < f.Binding.CheckedUnixNano || f.CreatedUnixNano < f.PlanTemplate.CheckedUnixNano || f.CreatedUnixNano >= f.Binding.ExpiresUnixNano || f.CreatedUnixNano >= f.PlanTemplate.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "cleanup_closure_current_window_invalid", "cleanup closure creation is outside an exact input current window")
	}
	if f.CreatedUnixNano >= f.Assembly.OwnerCurrent.ExpiresUnixNano {
		return NewError(ErrorPrecondition, "cleanup_closure_assembly_expired", "cleanup closure was created after Assembly current expiry")
	}
	expectedID, err := DeriveHostCleanupClosureIDV2(f.Plan.HostID, f.Plan.StartID)
	if err != nil {
		return err
	}
	if f.ClosureID != expectedID {
		return NewError(ErrorConflict, "cleanup_closure_id_drift", "cleanup closure ID is not deterministic")
	}
	if f.Binding.ResourceBindingSet != f.PlanTemplate.ResourceBindingSet {
		return NewError(ErrorConflict, "cleanup_closure_resource_set_drift", "cleanup template Resource BindingSet drifted")
	}
	nodes := map[string]CleanupNodeV2{}
	for _, node := range f.Plan.Nodes {
		nodes[node.NodeID] = node
	}
	bindings := map[runtimeports.ComponentIDV2]runtimeports.BindingAdmissionBindingRefV1{}
	for _, binding := range f.Binding.Bindings {
		bindings[binding.ComponentID] = binding
	}
	controlIDs := map[string]struct{}{}
	controlByFactory := map[string]HostCleanupClosureControlCoverageV2{}
	resourceByID := map[string]runtimeports.ResourceHandleRefV1{}
	for i, control := range f.Controls {
		if err := control.Validate(); err != nil {
			return err
		}
		if i > 0 && f.Controls[i-1].FactoryRef.FactoryID >= control.FactoryRef.FactoryID {
			return NewError(ErrorConflict, "cleanup_control_duplicate", "cleanup closure Controls must be sorted and unique")
		}
		if control.ResourceBindingSet != f.Binding.ResourceBindingSet {
			return NewError(ErrorConflict, "cleanup_control_resource_set_drift", "cleanup control Resource BindingSet drifted")
		}
		if control.Generation != f.Assembly.OwnerCurrent {
			return NewError(ErrorConflict, "cleanup_control_generation_drift", "cleanup control Generation current drifted")
		}
		binding, ok := bindings[control.ComponentID]
		if !ok || binding != control.Binding {
			return NewError(ErrorConflict, "cleanup_control_binding_drift", "cleanup control does not bind an admitted component")
		}
		for _, nodeID := range control.CleanupNodeIDs {
			node, ok := nodes[nodeID]
			if !ok || node.OwnerComponentID != string(control.ComponentID) {
				return NewError(ErrorConflict, "cleanup_control_node_drift", "cleanup control names an invalid Owner cleanup node")
			}
		}
		controlIDs[control.FactoryRef.FactoryID] = struct{}{}
		controlByFactory[control.FactoryRef.FactoryID] = control
		for _, handle := range control.ResourceHandles {
			if previous, exists := resourceByID[handle.ID]; exists && previous != handle {
				return NewError(ErrorConflict, "cleanup_resource_alias", "cleanup resource ID aliases different exact handles")
			}
			resourceByID[handle.ID] = handle
		}
	}
	if len(f.PlanTemplate.Routes)+len(f.PlanTemplate.FixedBarriers) != len(f.Plan.Nodes) {
		return NewError(ErrorPrecondition, "cleanup_template_plan_incomplete", "cleanup Plan Template must cover every embedded Plan node")
	}
	for _, route := range f.PlanTemplate.Routes {
		node, ok := nodes[route.NodeID]
		if !ok || node.OwnerComponentID != string(route.ComponentID) || node.CleanupContractRef != route.CleanupContractRef || node.InspectPortBinding != route.InspectPortBinding || node.RequestSchemaDigest != route.RequestSchemaDigest || node.ResultSchemaDigest != route.ResultSchemaDigest || node.ResourceClass != route.ResourceClass || len(node.RequiredBarrierIDs) != len(route.RequiredBarrierIDs) {
			return NewError(ErrorConflict, "cleanup_template_plan_drift", "cleanup Plan Template route does not match the embedded Plan")
		}
		for index := range node.RequiredBarrierIDs {
			if node.RequiredBarrierIDs[index] != route.RequiredBarrierIDs[index] {
				return NewError(ErrorConflict, "cleanup_template_plan_drift", "cleanup Plan Template dependency drifted")
			}
		}
		control, ok := controlByFactory[route.FactoryRef.FactoryID]
		if ok && (control.FactoryRef != route.FactoryRef || control.ComponentID != route.ComponentID || control.ArtifactDigest != route.ArtifactDigest || control.Capability != route.Capability || control.Binding != route.Binding) {
			return NewError(ErrorConflict, "cleanup_template_control_drift", "cleanup Plan Template route does not match a planned Control")
		}
	}
	for _, barrier := range f.PlanTemplate.FixedBarriers {
		node, ok := nodes[barrier.NodeID]
		if !ok || node.Digest != barrier.Digest {
			return NewError(ErrorConflict, "cleanup_template_fixed_barrier_drift", "cleanup Plan Template fixed barrier drifted")
		}
	}
	coverageDigest, err := digestHostCleanupCoverageV2(f.Coverage)
	if err != nil {
		return err
	}
	if coverageDigest != f.CoverageDigest {
		return NewError(ErrorConflict, "cleanup_coverage_digest_drift", "cleanup coverage digest drifted")
	}
	coveredBindings, coveredControls, coveredResources, coveredBarriers := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	bindingByID := map[string]runtimeports.BindingAdmissionBindingRefV1{}
	for _, binding := range f.Binding.Bindings {
		bindingByID[binding.ID] = binding
	}
	for i, entry := range f.Coverage {
		if err := entry.Validate(); err != nil {
			return err
		}
		if i > 0 && cleanupCoverageKeyV2(f.Coverage[i-1]) >= cleanupCoverageKeyV2(entry) {
			return NewError(ErrorConflict, "cleanup_coverage_duplicate", "cleanup coverage must be sorted and unique")
		}
		node, ok := nodes[entry.CleanupNodeID]
		if !ok || node.ResourceClass != entry.ResourceClass || node.OwnerComponentID != entry.ComponentID {
			return NewError(ErrorConflict, "cleanup_coverage_node_drift", "cleanup coverage does not match its Plan node")
		}
		switch entry.SourceKind {
		case HostCleanupCoverageBindingV2:
			binding, ok := bindingByID[entry.SourceID]
			if !ok || uint64(binding.Revision) != entry.SourceRevision || DigestV1(binding.Digest) != entry.SourceDigest || string(binding.ComponentID) != entry.ComponentID {
				return NewError(ErrorConflict, "cleanup_binding_coverage_drift", "cleanup Binding coverage source drifted")
			}
			coveredBindings[entry.SourceID] = true
		case HostCleanupCoverageControlV2:
			control, ok := controlByFactory[entry.SourceID]
			if !ok || uint64(control.FactoryRef.Revision) != entry.SourceRevision || DigestV1(control.FactoryRef.Digest) != entry.SourceDigest || string(control.ComponentID) != entry.ComponentID {
				return NewError(ErrorConflict, "cleanup_control_coverage_drift", "cleanup Control coverage source drifted")
			}
			coveredControls[entry.SourceID] = true
		case HostCleanupCoverageResourceV2:
			handle, ok := resourceByID[entry.SourceID]
			if !ok || uint64(handle.Revision) != entry.SourceRevision || DigestV1(handle.Digest) != entry.SourceDigest {
				return NewError(ErrorConflict, "cleanup_resource_coverage_drift", "cleanup ResourceHandle coverage source drifted")
			}
			coveredResources[entry.SourceID] = true
		case HostCleanupCoverageBarrierV2:
			barrier, ok := nodes[entry.SourceID]
			if !ok || entry.SourceID != entry.CleanupNodeID || entry.SourceRevision != 1 || barrier.Digest != entry.SourceDigest {
				return NewError(ErrorConflict, "cleanup_barrier_coverage_drift", "cleanup barrier coverage source drifted")
			}
			coveredBarriers[entry.SourceID] = true
		}
	}
	for _, binding := range f.Binding.Bindings {
		if !coveredBindings[binding.ID] {
			return NewError(ErrorPrecondition, "cleanup_binding_coverage_missing", "cleanup closure lacks Binding coverage")
		}
	}
	for id := range controlIDs {
		if !coveredControls[id] {
			return NewError(ErrorPrecondition, "cleanup_control_coverage_missing", "cleanup closure lacks Control coverage")
		}
	}
	for _, control := range f.Controls {
		for _, handle := range control.ResourceHandles {
			if !coveredResources[handle.ID] {
				return NewError(ErrorPrecondition, "cleanup_resource_coverage_missing", "cleanup closure lacks ResourceHandle coverage")
			}
		}
	}
	for _, id := range []string{CleanupBarrierHarnessCloseV2, CleanupBarrierSandboxFenceV2, CleanupBarrierSandboxReleaseV2, CleanupBarrierRuntimeCleanupAggregateV2} {
		if !coveredBarriers[id] {
			return NewError(ErrorPrecondition, "cleanup_barrier_coverage_missing", "cleanup closure lacks fixed barrier coverage")
		}
	}
	expected, err := f.digestV2()
	if err != nil {
		return err
	}
	if expected != f.ContentDigest {
		return NewError(ErrorConflict, "cleanup_closure_digest_drift", "cleanup closure content digest drifted")
	}
	return nil
}

func isFixedCleanupBarrierV2(id string) bool {
	switch id {
	case CleanupBarrierHarnessCloseV2, CleanupBarrierSandboxFenceV2, CleanupBarrierSandboxReleaseV2, CleanupBarrierRuntimeCleanupAggregateV2:
		return true
	default:
		return false
	}
}

func (f HostCleanupClosureFactV2) RefV2() (HostCleanupClosureRefV2, error) {
	if err := f.Validate(); err != nil {
		return HostCleanupClosureRefV2{}, err
	}
	planRef, err := f.Plan.RefV2()
	if err != nil {
		return HostCleanupClosureRefV2{}, err
	}
	return HostCleanupClosureRefV2{ClosureID: f.ClosureID, Revision: f.Revision, HostID: f.Plan.HostID, StartID: f.Plan.StartID, PlanRef: planRef, CoverageDigest: f.CoverageDigest, Digest: f.ContentDigest}, nil
}

// BuildHostCleanupClosureCandidateV2 is pure. It consumes exact Owner outputs,
// constructs the complete Plan and coverage, and never writes an Owner Fact.
func BuildHostCleanupClosureCandidateV2(start StartRequestV2, compiled CompiledAssemblyArtifactsV2, publication AssemblyPublicationResultV2, binding runtimeports.BindingAdmissionResultV1, template HostCleanupPlanTemplateCurrentV2, requests []ControlAdapterConstructRequestV2) (HostCleanupClosureFactV2, error) {
	if err := start.Validate(); err != nil {
		return HostCleanupClosureFactV2{}, err
	}
	created := start.RequestedAtUnixNano
	for _, watermark := range []int64{compiled.CheckedUnixNano, binding.CheckedUnixNano, template.CheckedUnixNano} {
		if watermark > created {
			created = watermark
		}
	}
	now := time.Unix(0, created)
	if err := compiled.ValidateAt(now); err != nil {
		return HostCleanupClosureFactV2{}, err
	}
	if err := publication.ValidateAt(now); err != nil {
		return HostCleanupClosureFactV2{}, err
	}
	if err := binding.Validate(); err != nil {
		return HostCleanupClosureFactV2{}, err
	}
	if err := template.ValidateCurrent(now); err != nil {
		return HostCleanupClosureFactV2{}, err
	}
	if binding.ResourceBindingSet != template.ResourceBindingSet {
		return HostCleanupClosureFactV2{}, NewError(ErrorConflict, "cleanup_builder_resource_set_drift", "Binding and Cleanup Plan Template Resource BindingSet drifted")
	}
	if publication.Generation != compiled.Compiled.GenerationRef || publication.Manifest != compiled.Compiled.ManifestRef || publication.Graph != compiled.Compiled.Graph.GraphRef || publication.Handoff != compiled.Compiled.HandoffRef {
		return HostCleanupClosureFactV2{}, NewError(ErrorConflict, "cleanup_builder_assembly_drift", "published Assembly does not match compiled artifacts")
	}
	claim, err := start.ClaimV1()
	if err != nil {
		return HostCleanupClosureFactV2{}, err
	}
	claimRef, err := claim.CurrentRefV1()
	if err != nil {
		return HostCleanupClosureFactV2{}, err
	}

	nodes := append([]CleanupNodeV2{}, template.FixedBarriers...)
	routeByComponent := map[runtimeports.ComponentIDV2][]HostCleanupPlanTemplateRouteV2{}
	routeByFactory := map[string][]HostCleanupPlanTemplateRouteV2{}
	for _, route := range template.Routes {
		node, sealErr := SealCleanupNodeV2(CleanupNodeV2{NodeID: route.NodeID, Kind: CleanupOwnerNodeV2, OwnerComponentID: string(route.ComponentID), CleanupContractRef: route.CleanupContractRef, ResourceClass: route.ResourceClass, RequiredBarrierIDs: route.RequiredBarrierIDs, InspectPortBinding: route.InspectPortBinding, RequestSchemaDigest: route.RequestSchemaDigest, ResultSchemaDigest: route.ResultSchemaDigest})
		if sealErr != nil {
			return HostCleanupClosureFactV2{}, sealErr
		}
		nodes = append(nodes, node)
		routeByComponent[route.ComponentID] = append(routeByComponent[route.ComponentID], route)
		routeByFactory[route.FactoryRef.FactoryID] = append(routeByFactory[route.FactoryRef.FactoryID], route)
	}
	planIDDigest, err := DigestJSONV1(struct {
		HostID   string     `json:"host_id"`
		StartID  string     `json:"start_id"`
		Template ExactRefV1 `json:"template_ref"`
	}{start.Config.HostID, start.StartID, template.TemplateRef})
	if err != nil {
		return HostCleanupClosureFactV2{}, err
	}
	plan, err := SealCleanupPlanV2(CleanupPlanV2{ContractVersion: CleanupContractVersionV2, PlanID: "plan/" + strings.TrimPrefix(string(planIDDigest), "sha256:"), Revision: 1, HostID: start.Config.HostID, StartID: start.StartID, Nodes: nodes})
	if err != nil {
		return HostCleanupClosureFactV2{}, err
	}

	controls := make([]HostCleanupClosureControlCoverageV2, 0, len(requests))
	coverage := make([]HostCleanupClosureCoverageEntryV2, 0, len(binding.Bindings)+len(requests)+len(template.FixedBarriers))
	for _, admitted := range binding.Bindings {
		routes := routeByComponent[admitted.ComponentID]
		if len(routes) == 0 {
			return HostCleanupClosureFactV2{}, NewError(ErrorPrecondition, "cleanup_binding_route_missing", "an admitted component lacks a cleanup route")
		}
		route := routes[0]
		coverage = append(coverage, HostCleanupClosureCoverageEntryV2{SourceKind: HostCleanupCoverageBindingV2, SourceID: admitted.ID, SourceRevision: uint64(admitted.Revision), SourceDigest: DigestV1(admitted.Digest), ComponentID: string(admitted.ComponentID), ResourceClass: route.ResourceClass, CleanupNodeID: route.NodeID})
	}
	for _, request := range requests {
		if err := request.ValidateCurrent(now); err != nil {
			return HostCleanupClosureFactV2{}, err
		}
		if request.HostID != start.Config.HostID || request.StartID != start.StartID || request.Descriptor.Generation != publication.OwnerCurrent || request.ResourceBindings.Ref != binding.ResourceBindingSet {
			return HostCleanupClosureFactV2{}, NewError(ErrorConflict, "cleanup_control_request_drift", "Control request does not belong to this exact Start/Assembly/Binding")
		}
		routes := routeByFactory[request.Descriptor.Ref.FactoryID]
		if len(routes) == 0 {
			routes = routeByComponent[request.Descriptor.ComponentID]
		}
		if len(routes) == 0 {
			return HostCleanupClosureFactV2{}, NewError(ErrorPrecondition, "cleanup_control_route_missing", "a planned Control lacks a cleanup route")
		}
		nodeIDs := make([]string, 0, len(routes))
		for _, route := range routes {
			nodeIDs = append(nodeIDs, route.NodeID)
		}
		control := HostCleanupClosureControlCoverageV2{FactoryRef: request.Descriptor.Ref, ComponentID: request.Descriptor.ComponentID, ArtifactDigest: request.Descriptor.ArtifactDigest, Capability: request.Descriptor.Capability, Binding: request.Descriptor.Binding, Generation: request.Descriptor.Generation, ResourceBindingSet: request.Descriptor.ResourceBindingSet, ResourceHandles: request.Descriptor.ResourceHandles, CleanupNodeIDs: nodeIDs}
		control = control.canonicalV2()
		if err := control.Validate(); err != nil {
			return HostCleanupClosureFactV2{}, err
		}
		controls = append(controls, control)
		primary := routes[0]
		coverage = append(coverage, HostCleanupClosureCoverageEntryV2{SourceKind: HostCleanupCoverageControlV2, SourceID: control.FactoryRef.FactoryID, SourceRevision: uint64(control.FactoryRef.Revision), SourceDigest: DigestV1(control.FactoryRef.Digest), ComponentID: string(control.ComponentID), ResourceClass: primary.ResourceClass, CleanupNodeID: primary.NodeID})
		for _, handle := range control.ResourceHandles {
			coverage = append(coverage, HostCleanupClosureCoverageEntryV2{SourceKind: HostCleanupCoverageResourceV2, SourceID: handle.ID, SourceRevision: uint64(handle.Revision), SourceDigest: DigestV1(handle.Digest), ComponentID: string(control.ComponentID), ResourceClass: primary.ResourceClass, CleanupNodeID: primary.NodeID})
		}
	}
	for _, barrier := range template.FixedBarriers {
		coverage = append(coverage, HostCleanupClosureCoverageEntryV2{SourceKind: HostCleanupCoverageBarrierV2, SourceID: barrier.NodeID, SourceRevision: 1, SourceDigest: barrier.Digest, ComponentID: barrier.OwnerComponentID, ResourceClass: barrier.ResourceClass, CleanupNodeID: barrier.NodeID})
	}
	assembly := HostCleanupClosureAssemblyCoordinateV2{ScopeRef: compiled.ScopeRef, AssemblyInput: compiled.InputRef, Publication: publication.Publication, Generation: publication.Generation, Manifest: publication.Manifest, Graph: publication.Graph, Handoff: publication.Handoff, OwnerCurrent: publication.OwnerCurrent}
	bound := HostCleanupClosureBindingCoordinateV2{AttemptID: binding.AttemptID, RequestDigest: binding.RequestDigest, BindingSet: binding.BindingSet, Bindings: binding.Bindings, ResourceBindingSet: binding.ResourceBindingSet, CheckedUnixNano: binding.CheckedUnixNano, ExpiresUnixNano: binding.ExpiresUnixNano, ResultDigest: binding.ResultDigest}
	return SealHostCleanupClosureFactV2(HostCleanupClosureFactV2{Revision: 1, StartClaimRef: claimRef, Assembly: assembly, Binding: bound, PlanTemplate: template, Controls: controls, Plan: plan, Coverage: coverage, CreatedUnixNano: created})
}

func CloneHostCleanupClosureFactV2(f HostCleanupClosureFactV2) HostCleanupClosureFactV2 {
	return f.canonicalV2()
}
