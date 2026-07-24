package assemblycontract

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ContractVersionV1 = "praxis.harness.assembly/v1"
	CompilerVersionV1 = "praxis.harness.assembly.compiler/v1"
	CatalogVersionV1  = "praxis.harness.assembly.catalog/v1"

	MaxAssemblyEntries = 512
	MaxReferenceBytes  = 2048
	MaxWriteSetEntries = 128
)

type ObjectRefV1 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

type AssemblyPlanRefsV1 struct {
	ResolvedAgentPlan         ObjectRefV1 `json:"resolved_agent_plan"`
	HarnessBootstrapPlan      ObjectRefV1 `json:"harness_bootstrap_plan"`
	Profile                   ObjectRefV1 `json:"profile"`
	RuntimePolicy             ObjectRefV1 `json:"runtime_policy"`
	HarnessStack              ObjectRefV1 `json:"harness_stack"`
	SemanticRoute             ObjectRefV1 `json:"semantic_route"`
	ContextPlan               ObjectRefV1 `json:"context_plan"`
	ToolSurface               ObjectRefV1 `json:"tool_surface"`
	CapabilityGrant           ObjectRefV1 `json:"capability_grant"`
	ExpectedInjectionManifest ObjectRefV1 `json:"expected_injection_manifest"`
}

type CompatibilityV1 struct {
	MinimumInclusive string `json:"minimum_inclusive"`
	MaximumExclusive string `json:"maximum_exclusive"`
}

type ModuleDescriptorV1 struct {
	ContractVersion        string                           `json:"contract_version"`
	ModuleID               string                           `json:"module_id"`
	Namespace              string                           `json:"namespace"`
	SemanticVersion        string                           `json:"semantic_version"`
	ArtifactDigest         core.Digest                      `json:"artifact_digest"`
	PublisherRef           ObjectRefV1                      `json:"publisher_ref"`
	SourceRef              ObjectRefV1                      `json:"source_ref"`
	ComponentManifestRef   ObjectRefV1                      `json:"component_manifest_ref"`
	Compatibility          CompatibilityV1                  `json:"compatibility"`
	Capabilities           []runtimeports.CapabilityNameV2  `json:"capabilities"`
	Schemas                []runtimeports.SchemaRefV2       `json:"schemas"`
	Locality               runtimeports.LocalityV2          `json:"locality"`
	ResidualClass          runtimeports.ResidualClassV2     `json:"residual_class"`
	Owners                 []runtimeports.OwnerAssignmentV2 `json:"owners"`
	CredentialRequirements []runtimeports.NamespacedNameV2  `json:"credential_requirements,omitempty"`
}

type CapabilityDescriptorV1 struct {
	ContractVersion string                        `json:"contract_version"`
	Capability      runtimeports.CapabilityNameV2 `json:"capability"`
	Version         string                        `json:"version"`
	Schemas         []runtimeports.SchemaRefV2    `json:"schemas"`
	Required        bool                          `json:"required"`
	Provided        bool                          `json:"provided"`
	TTLSeconds      uint64                        `json:"ttl_seconds"`
	EffectClass     string                        `json:"effect_class"`
	OwnerCapability runtimeports.CapabilityNameV2 `json:"owner_capability"`
	Conformance     runtimeports.ConformanceLevel `json:"conformance"`
}

type LifecycleScopeV1 string

const (
	LifecycleGenerationV1 LifecycleScopeV1 = "generation"
	LifecycleInstanceV1   LifecycleScopeV1 = "instance"
	LifecycleRunV1        LifecycleScopeV1 = "run"
	LifecycleSessionV1    LifecycleScopeV1 = "session"
)

type CardinalityV1 string

const (
	CardinalityExactlyOneV1    CardinalityV1 = "exactly_one"
	CardinalityZeroOrOneV1     CardinalityV1 = "zero_or_one"
	CardinalityZeroOrManyV1    CardinalityV1 = "zero_or_many"
	CardinalityOwnerSourcesV1  CardinalityV1 = "one_owner_many_sources"
	CardinalityActiveBindingV1 CardinalityV1 = "one_active_binding"
)

type SlotContributionKindV1 string

const (
	SlotContributionOwnerV1     SlotContributionKindV1 = "owner"
	SlotContributionSourceV1    SlotContributionKindV1 = "source"
	SlotContributionProviderV1  SlotContributionKindV1 = "provider"
	SlotContributionReferenceV1 SlotContributionKindV1 = "reference"
)

type SlotSpecV1 struct {
	ContractVersion   string                        `json:"contract_version"`
	SlotID            string                        `json:"slot_id"`
	LifecycleScope    LifecycleScopeV1              `json:"lifecycle_scope"`
	Cardinality       CardinalityV1                 `json:"cardinality"`
	Required          bool                          `json:"required"`
	OwnerCapability   runtimeports.CapabilityNameV2 `json:"owner_capability"`
	ContributionKinds []SlotContributionKindV1      `json:"contribution_kinds"`
	InputSchema       runtimeports.SchemaRefV2      `json:"input_schema"`
	OutputSchema      runtimeports.SchemaRefV2      `json:"output_schema"`
	EffectClass       string                        `json:"effect_class"`
	ConcurrencyPolicy string                        `json:"concurrency_policy"`
	FailurePolicy     string                        `json:"failure_policy"`
	DegradationPolicy string                        `json:"degradation_policy"`
	Dependencies      []string                      `json:"dependencies,omitempty"`
	Dynamic           bool                          `json:"dynamic"`
	Digest            core.Digest                   `json:"digest"`
}

type SlotContributionV1 struct {
	ContractVersion      string                        `json:"contract_version"`
	ContributionID       string                        `json:"contribution_id"`
	ModuleRef            string                        `json:"module_ref"`
	SlotRef              string                        `json:"slot_ref"`
	Kind                 SlotContributionKindV1        `json:"kind"`
	CapabilityRef        runtimeports.CapabilityNameV2 `json:"capability_ref"`
	PortSpecRef          string                        `json:"port_spec_ref,omitempty"`
	ProviderCandidateRef string                        `json:"provider_candidate_ref,omitempty"`
	Priority             int32                         `json:"priority"`
	Dependencies         []string                      `json:"dependencies,omitempty"`
	Digest               core.Digest                   `json:"digest"`
}

type GovernanceRequirementsV1 struct {
	ReviewRequired    bool `json:"review_required"`
	FenceRequired     bool `json:"fence_required"`
	AuthorityRequired bool `json:"authority_required"`
	ScopeRequired     bool `json:"scope_required"`
	BudgetRequired    bool `json:"budget_required"`
}

const (
	RuntimeOperationScopeKindV1            runtimeports.NamespacedNameV2 = "praxis.runtime/operation-scope"
	RuntimeOperationSettlementCapabilityV1 runtimeports.CapabilityNameV2 = "praxis.runtime/operation-settlement"
)

type OperationScopeRefV1 struct {
	Ref         ObjectRefV1                   `json:"ref"`
	ScopeKind   runtimeports.NamespacedNameV2 `json:"scope_kind"`
	ScopeDigest core.Digest                   `json:"scope_digest"`
}

type InspectContractRefV1 struct {
	Ref               ObjectRefV1                   `json:"ref"`
	OwnerCapability   runtimeports.CapabilityNameV2 `json:"owner_capability"`
	RequestSchema     runtimeports.SchemaRefV2      `json:"request_schema"`
	ObservationSchema runtimeports.SchemaRefV2      `json:"observation_schema"`
}

type DomainResultContractRefV1 struct {
	Ref             ObjectRefV1                   `json:"ref"`
	OwnerCapability runtimeports.CapabilityNameV2 `json:"owner_capability"`
	Schema          runtimeports.SchemaRefV2      `json:"schema"`
}

// RuntimeOperationSettlementRefContractV1 describes the public Runtime ref
// shape only. It cannot carry or apply a domain result fact.
type RuntimeOperationSettlementRefContractV1 struct {
	Ref                    ObjectRefV1                   `json:"ref"`
	RuntimeOwnerCapability runtimeports.CapabilityNameV2 `json:"runtime_owner_capability"`
	Schema                 runtimeports.SchemaRefV2      `json:"schema"`
}

type ApplySettlementContractRefV1 struct {
	Ref             ObjectRefV1                   `json:"ref"`
	OwnerCapability runtimeports.CapabilityNameV2 `json:"owner_capability"`
	RequestSchema   runtimeports.SchemaRefV2      `json:"request_schema"`
	ResultSchema    runtimeports.SchemaRefV2      `json:"result_schema"`
}

type RunStartRequirementRefV1 struct {
	Ref             ObjectRefV1                   `json:"ref"`
	RequirementID   runtimeports.NamespacedNameV2 `json:"requirement_id"`
	OwnerCapability runtimeports.CapabilityNameV2 `json:"owner_capability"`
}

type RunSettlementRequirementRefV1 struct {
	Ref             ObjectRefV1                   `json:"ref"`
	RequirementID   runtimeports.NamespacedNameV2 `json:"requirement_id"`
	OwnerCapability runtimeports.CapabilityNameV2 `json:"owner_capability"`
}

type CleanupContractRefV1 struct {
	Ref             ObjectRefV1                   `json:"ref"`
	OwnerCapability runtimeports.CapabilityNameV2 `json:"owner_capability"`
	RequestSchema   runtimeports.SchemaRefV2      `json:"request_schema"`
	ResultSchema    runtimeports.SchemaRefV2      `json:"result_schema"`
}

type PortSpecV1 struct {
	ContractVersion                       string                                   `json:"contract_version"`
	PortID                                string                                   `json:"port_id"`
	OwnerCapability                       runtimeports.CapabilityNameV2            `json:"owner_capability"`
	RequestSchema                         runtimeports.SchemaRefV2                 `json:"request_schema"`
	ResponseSchema                        runtimeports.SchemaRefV2                 `json:"response_schema"`
	OperationClass                        string                                   `json:"operation_class"`
	EffectKind                            runtimeports.NamespacedNameV2            `json:"effect_kind,omitempty"`
	ConflictDomainRule                    string                                   `json:"conflict_domain_rule,omitempty"`
	Governance                            GovernanceRequirementsV1                 `json:"governance"`
	Idempotency                           string                                   `json:"idempotency"`
	CancelSupported                       bool                                     `json:"cancel_supported"`
	OperationScopeRef                     *OperationScopeRefV1                     `json:"operation_scope_ref,omitempty"`
	InspectContractRef                    *InspectContractRefV1                    `json:"inspect_contract_ref,omitempty"`
	DomainResultContractRef               *DomainResultContractRefV1               `json:"domain_result_contract_ref,omitempty"`
	RuntimeOperationSettlementRefContract *RuntimeOperationSettlementRefContractV1 `json:"runtime_operation_settlement_ref_contract,omitempty"`
	ApplySettlementContractRef            *ApplySettlementContractRefV1            `json:"apply_settlement_contract_ref,omitempty"`
	RunStartRequirementRefs               []RunStartRequirementRefV1               `json:"run_start_requirement_refs,omitempty"`
	RunSettlementRequirementRefs          []RunSettlementRequirementRefV1          `json:"run_settlement_requirement_refs,omitempty"`
	FailureSemantics                      string                                   `json:"failure_semantics"`
	Compatibility                         CompatibilityV1                          `json:"compatibility"`
}

type PhaseCapabilityV1 string

const (
	PhaseObserverV1 PhaseCapabilityV1 = "observer"
	PhaseFilterV1   PhaseCapabilityV1 = "filter"
	PhaseGateV1     PhaseCapabilityV1 = "gate"
	PhasePortV1     PhaseCapabilityV1 = "port"
)

type HookFaceSpecV1 struct {
	ContractVersion   string                   `json:"contract_version"`
	HookFaceID        string                   `json:"hookface_id"`
	PhaseID           string                   `json:"phase_id"`
	Kind              PhaseCapabilityV1        `json:"kind"`
	InputSchema       runtimeports.SchemaRefV2 `json:"input_schema"`
	OutputSchema      runtimeports.SchemaRefV2 `json:"output_schema"`
	AuthorityCeiling  string                   `json:"authority_ceiling"`
	MutationMask      []string                 `json:"mutation_mask,omitempty"`
	EffectClass       string                   `json:"effect_class"`
	TimeoutPolicy     string                   `json:"timeout_policy"`
	FailurePolicy     string                   `json:"failure_policy"`
	ConcurrencyPolicy string                   `json:"concurrency_policy"`
	ReceiptPolicy     string                   `json:"receipt_policy"`
	Digest            core.Digest              `json:"digest"`
}

type PhaseContributionV1 struct {
	ContractVersion      string            `json:"contract_version"`
	ContributionID       string            `json:"contribution_id"`
	HookFaceRef          string            `json:"hookface_ref"`
	HandlerDescriptorRef ObjectRefV1       `json:"handler_descriptor_ref"`
	ModuleRef            string            `json:"module_ref"`
	Capability           PhaseCapabilityV1 `json:"capability"`
	Dependencies         []string          `json:"dependencies,omitempty"`
	Priority             int32             `json:"priority"`
	WriteSet             []string          `json:"write_set,omitempty"`
	Async                bool              `json:"async"`
	Digest               core.Digest       `json:"digest"`
}

type DependencySpecV1 struct {
	ContractVersion string                        `json:"contract_version"`
	FromRef         string                        `json:"from_ref"`
	ToRef           string                        `json:"to_ref"`
	Relation        string                        `json:"relation"`
	Required        bool                          `json:"required"`
	VersionRange    CompatibilityV1               `json:"version_range"`
	Capability      runtimeports.CapabilityNameV2 `json:"capability,omitempty"`
	FailureMode     string                        `json:"failure_mode"`
}

type ConstructionModeV1 string

const ConstructionTrustedInProcessGoV1 ConstructionModeV1 = "trusted_in_process_go"

type ModuleFactoryDescriptorV1 struct {
	ContractVersion    string                        `json:"contract_version"`
	FactoryID          string                        `json:"factory_id"`
	ModuleRef          string                        `json:"module_ref"`
	ArtifactDigest     core.Digest                   `json:"artifact_digest"`
	ConstructionMode   ConstructionModeV1            `json:"construction_mode"`
	InputSchema        runtimeports.SchemaRefV2      `json:"input_schema"`
	OutputCapability   runtimeports.CapabilityNameV2 `json:"output_capability"`
	Lifecycle          LifecycleScopeV1              `json:"lifecycle"`
	CleanupContractRef CleanupContractRefV1          `json:"cleanup_contract_ref"`
	TrustRef           ObjectRefV1                   `json:"trust_ref"`
}

type ProviderBindingCandidateV1 struct {
	ContractVersion string      `json:"contract_version"`
	CandidateID     string      `json:"candidate_id"`
	ModuleRef       string      `json:"module_ref"`
	SlotRef         string      `json:"slot_ref"`
	PortSpecRef     string      `json:"port_spec_ref"`
	ProviderRef     ObjectRefV1 `json:"provider_ref"`
	Digest          core.Digest `json:"digest"`
}

type AssemblyPolicyV1 struct {
	AllowResidualClasses []string `json:"allow_residual_classes,omitempty"`
	MaximumPriority      int32    `json:"maximum_priority"`
}

type AssemblyInputV1 struct {
	ContractVersion           string                             `json:"contract_version"`
	InputID                   string                             `json:"input_id"`
	Revision                  core.Revision                      `json:"revision"`
	OwnerRef                  string                             `json:"owner_ref"`
	ScopeRef                  string                             `json:"scope_ref"`
	CreatedUnixNano           int64                              `json:"created_unix_nano"`
	Plan                      AssemblyPlanRefsV1                 `json:"plan"`
	CurrentFacts              []ObjectRefV1                      `json:"current_facts"`
	RouteBindings             []ObjectRefV1                      `json:"route_bindings"`
	ComponentManifests        []runtimeports.ComponentManifestV2 `json:"component_manifests"`
	Modules                   []ModuleDescriptorV1               `json:"modules"`
	Capabilities              []CapabilityDescriptorV1           `json:"capabilities"`
	Slots                     []SlotSpecV1                       `json:"slots"`
	SlotContributions         []SlotContributionV1               `json:"slot_contributions"`
	PortSpecs                 []PortSpecV1                       `json:"port_specs"`
	HookFaces                 []HookFaceSpecV1                   `json:"hookfaces"`
	PhaseContributions        []PhaseContributionV1              `json:"phase_contributions"`
	Dependencies              []DependencySpecV1                 `json:"dependencies"`
	Factories                 []ModuleFactoryDescriptorV1        `json:"factories"`
	ProviderBindingCandidates []ProviderBindingCandidateV1       `json:"provider_binding_candidates"`
	Policy                    AssemblyPolicyV1                   `json:"policy"`
	PreviousGenerationRef     *ObjectRefV1                       `json:"previous_generation_ref,omitempty"`
	EvidenceRefs              []ObjectRefV1                      `json:"evidence_refs,omitempty"`
	Digest                    core.Digest                        `json:"digest"`
}

type AssemblyStateV1 string

const (
	AssemblyStateRejectedV1 AssemblyStateV1 = "rejected"
	AssemblyStateSealedV1   AssemblyStateV1 = "sealed"
)

type DiagnosticSeverityV1 string

const (
	DiagnosticErrorV1   DiagnosticSeverityV1 = "error"
	DiagnosticWarningV1 DiagnosticSeverityV1 = "warning"
	DiagnosticInfoV1    DiagnosticSeverityV1 = "info"
)

type AssemblyDiagnosticV1 struct {
	Severity    DiagnosticSeverityV1 `json:"severity"`
	Code        string               `json:"code"`
	ObjectPath  string               `json:"object_path"`
	FieldPath   string               `json:"field_path"`
	Owner       string               `json:"owner"`
	Expected    string               `json:"expected,omitempty"`
	Actual      string               `json:"actual,omitempty"`
	EvidenceRef *ObjectRefV1         `json:"evidence_ref,omitempty"`
	Remediation string               `json:"remediation"`
}

type ResidualReportV1 struct {
	ResidualClass      string               `json:"residual_class"`
	Owner              string               `json:"owner"`
	Scope              string               `json:"scope"`
	InspectContractRef InspectContractRefV1 `json:"inspect_contract_ref"`
	CleanupContractRef CleanupContractRefV1 `json:"cleanup_contract_ref"`
	Allowed            bool                 `json:"allowed"`
	BlockingStage      string               `json:"blocking_stage"`
}

type ResolvedSlotV1 struct {
	SlotSpecDigest  core.Digest                   `json:"slot_spec_digest"`
	SlotID          string                        `json:"slot_id"`
	OwnerCapability runtimeports.CapabilityNameV2 `json:"owner_capability"`
	Contributions   []string                      `json:"contributions"`
}

type ResolvedPhaseV1 struct {
	HookFaceID    string            `json:"hookface_id"`
	PhaseID       string            `json:"phase_id"`
	Capability    PhaseCapabilityV1 `json:"capability"`
	Contributions []string          `json:"contributions"`
}

type AssemblyManifestV1 struct {
	ContractVersion           string                             `json:"contract_version"`
	InputDigest               core.Digest                        `json:"input_digest"`
	CatalogDigest             core.Digest                        `json:"catalog_digest"`
	Plan                      AssemblyPlanRefsV1                 `json:"plan"`
	CurrentFacts              []ObjectRefV1                      `json:"current_facts"`
	RouteBindings             []ObjectRefV1                      `json:"route_bindings"`
	Policy                    AssemblyPolicyV1                   `json:"policy"`
	ComponentManifests        []runtimeports.ComponentManifestV2 `json:"component_manifests"`
	Modules                   []ModuleDescriptorV1               `json:"modules"`
	Capabilities              []CapabilityDescriptorV1           `json:"capabilities"`
	Slots                     []SlotSpecV1                       `json:"slots"`
	SlotContributions         []SlotContributionV1               `json:"slot_contributions"`
	PortSpecs                 []PortSpecV1                       `json:"port_specs"`
	HookFaces                 []HookFaceSpecV1                   `json:"hookfaces"`
	PhaseContributions        []PhaseContributionV1              `json:"phase_contributions"`
	Dependencies              []DependencySpecV1                 `json:"dependencies"`
	Factories                 []ModuleFactoryDescriptorV1        `json:"factories"`
	ProviderBindingCandidates []ProviderBindingCandidateV1       `json:"provider_binding_candidates"`
	Residuals                 []ResidualReportV1                 `json:"residuals"`
	Digest                    core.Digest                        `json:"digest"`
}

type CompiledHarnessGraphV1 struct {
	ContractVersion string            `json:"contract_version"`
	InputDigest     core.Digest       `json:"input_digest"`
	CatalogDigest   core.Digest       `json:"catalog_digest"`
	DependencyOrder []string          `json:"dependency_order"`
	Slots           []ResolvedSlotV1  `json:"slots"`
	Phases          []ResolvedPhaseV1 `json:"phases"`
	PortSpecRefs    []string          `json:"port_spec_refs"`
	FactoryRefs     []string          `json:"factory_refs"`
	Digest          core.Digest       `json:"digest"`
}

type AssemblyGenerationV1 struct {
	ContractVersion       string          `json:"contract_version"`
	GenerationID          string          `json:"generation_id"`
	Revision              core.Revision   `json:"revision"`
	CompilerVersion       string          `json:"compiler_version"`
	CreatedUnixNano       int64           `json:"created_unix_nano"`
	State                 AssemblyStateV1 `json:"state"`
	InputDigest           core.Digest     `json:"input_digest"`
	ManifestDigest        core.Digest     `json:"manifest_digest"`
	GraphDigest           core.Digest     `json:"graph_digest"`
	DiagnosticDigest      core.Digest     `json:"diagnostic_digest"`
	ResidualReportDigest  core.Digest     `json:"residual_report_digest"`
	PreviousGenerationRef *ObjectRefV1    `json:"previous_generation_ref,omitempty"`
	EvidenceRefs          []ObjectRefV1   `json:"evidence_refs,omitempty"`
	Digest                core.Digest     `json:"digest"`
}

type AssemblyHandoffV1 struct {
	ContractVersion    string                        `json:"contract_version"`
	GenerationRef      ObjectRefV1                   `json:"generation_ref"`
	ManifestDigest     core.Digest                   `json:"manifest_digest"`
	GraphDigest        core.Digest                   `json:"graph_digest"`
	CatalogDigest      core.Digest                   `json:"catalog_digest"`
	RequiredExtension  runtimeports.NamespacedNameV2 `json:"required_extension"`
	ProviderCandidates []ProviderBindingCandidateV1  `json:"provider_candidates"`
	Digest             core.Digest                   `json:"digest"`
}

type AssemblyBindingConformanceV1 struct {
	ContractVersion string      `json:"contract_version"`
	HandoffRef      ObjectRefV1 `json:"handoff_ref"`
	GenerationRef   ObjectRefV1 `json:"generation_ref"`
	// Association is present only for the Runtime-owned Generation-Binding
	// association path. The report remains a read-only Harness object and is
	// never a Runtime Binding or Association Fact.
	Association                 *runtimeports.GenerationBindingAssociationRefV1  `json:"association,omitempty"`
	InputDigest                 core.Digest                                      `json:"input_digest,omitempty"`
	ManifestDigest              core.Digest                                      `json:"manifest_digest"`
	GraphDigest                 core.Digest                                      `json:"graph_digest"`
	CatalogDigest               core.Digest                                      `json:"catalog_digest,omitempty"`
	ComponentManifestSetDigest  core.Digest                                      `json:"component_manifest_set_digest,omitempty"`
	GovernanceExtension         *runtimeports.GenerationGovernanceExtensionRefV1 `json:"governance_extension,omitempty"`
	GenerationProjectionDigest  core.Digest                                      `json:"generation_projection_digest,omitempty"`
	Binding                     runtimeports.ProviderBindingRefV2                `json:"binding"`
	BindingSetID                string                                           `json:"binding_set_id,omitempty"`
	BindingSetRevision          core.Revision                                    `json:"binding_set_revision,omitempty"`
	BindingSetDigest            core.Digest                                      `json:"binding_set_digest"`
	BindingSetSemanticDigest    core.Digest                                      `json:"binding_set_semantic_digest"`
	BindingSetCurrentnessDigest core.Digest                                      `json:"binding_set_currentness_digest,omitempty"`
	BindingSetProjectionDigest  core.Digest                                      `json:"binding_set_projection_digest,omitempty"`
	ActivationOperationDigest   core.Digest                                      `json:"activation_operation_digest,omitempty"`
	ActivationCurrentnessDigest core.Digest                                      `json:"activation_currentness_digest,omitempty"`
	ActivationProjectionDigest  core.Digest                                      `json:"activation_projection_digest,omitempty"`
	CapabilityDigest            core.Digest                                      `json:"capability_digest"`
	SchemaDigests               []core.Digest                                    `json:"schema_digests"`
	ObservedUnixNano            int64                                            `json:"observed_unix_nano"`
	ExpiresUnixNano             int64                                            `json:"expires_unix_nano"`
	Current                     bool                                             `json:"current"`
	Diagnostics                 []AssemblyDiagnosticV1                           `json:"diagnostics,omitempty"`
	Digest                      core.Digest                                      `json:"digest"`
}

type CompileResultV1 struct {
	Generation  *AssemblyGenerationV1   `json:"generation,omitempty"`
	Manifest    *AssemblyManifestV1     `json:"manifest,omitempty"`
	Graph       *CompiledHarnessGraphV1 `json:"graph,omitempty"`
	Handoff     *AssemblyHandoffV1      `json:"handoff,omitempty"`
	Diagnostics []AssemblyDiagnosticV1  `json:"diagnostics"`
	Residuals   []ResidualReportV1      `json:"residuals"`
}

func (c AssemblyBindingConformanceV1) Expired(now time.Time) bool {
	return now.IsZero() || c.ExpiresUnixNano <= now.UnixNano()
}
