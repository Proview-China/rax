// Package releasecandidate publishes Harness's declarative, reference-only
// ComponentReleaseV1. The factory is a descriptor only: this package exposes
// no executable factory, Host construction, or production promotion API.
package releasecandidate

import (
	"context"
	"reflect"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	assemblerports "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ComponentIDV1     runtimeports.ComponentIDV2        = "components/harness"
	ComponentKindV1   runtimeports.ComponentKindV2      = "praxis/harness"
	GovernanceV1      runtimeports.GovernanceCategoryV2 = "praxis/core"
	CapabilityV1      runtimeports.CapabilityNameV2     = "praxis.harness/run-loop"
	ContractNameV1    runtimeports.NamespacedNameV2     = "praxis.harness/contract"
	SemanticVersionV1                                   = "1.0.0"
	ModuleIDV1                                          = "module/harness"
	FactoryIDV1                                         = "factory/harness"
	PortIDV1                                            = "port/harness/run-loop"
	MinimumTTL                                          = time.Second
	MaximumTTL                                          = 24 * time.Hour
)

type ClockV1 interface{ Now() time.Time }

type RequestV1 struct {
	ReleaseID      string
	Revision       core.Revision
	ArtifactDigest core.Digest
	SourceRef      assemblycontract.ObjectRefV1
	PublisherRef   assemblycontract.ObjectRefV1
	TrustRef       assemblycontract.ObjectRefV1
	EvidenceRefs   []assemblycontract.ObjectRefV1
	PlanArtifacts  []assemblercontract.PlanArtifactV1
	TTL            time.Duration
}

// ConformanceV1 records the exact live owner-local boundary. The three local
// surfaces are implemented, but none is accepted as a production proof.
type ConformanceV1 struct {
	ReferenceOnly             bool
	OwnerLocalAssemblyCurrent bool
	OwnerLocalRouteCurrent    bool
	OwnerLocalCommitGate      bool
	PersistentStores          bool
	ProductionRoute           bool
	ActualPointGuard          bool
	ProductionContinuation    bool
	ExecutableFactory         bool
	ProductionSLA             bool
}

func OwnerLocalConformanceV1() ConformanceV1 {
	return ConformanceV1{
		ReferenceOnly:             true,
		OwnerLocalAssemblyCurrent: true,
		OwnerLocalRouteCurrent:    true,
		OwnerLocalCommitGate:      true,
	}
}

func (c ConformanceV1) Validate() error {
	if !c.ReferenceOnly || !c.OwnerLocalAssemblyCurrent || !c.OwnerLocalRouteCurrent || !c.OwnerLocalCommitGate ||
		c.PersistentStores || c.ProductionRoute || c.ActualPointGuard || c.ProductionContinuation || c.ExecutableFactory || c.ProductionSLA {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Harness owner-local conformance cannot claim production stores, route, actual-point, continuation, factory, or SLA")
	}
	return nil
}

type ProofRequirementV1 string

const (
	ProofDurableSessionEventV1   ProofRequirementV1 = "harness.durable-session-event-store"
	ProofDurableAssemblyV1       ProofRequirementV1 = "harness.durable-assembly-current-store"
	ProofDurableRouteV1          ProofRequirementV1 = "harness.durable-route-current-store"
	ProofProductionRouteV1       ProofRequirementV1 = "harness.production-route-wiring-current"
	ProofModelActualPointV1      ProofRequirementV1 = "harness.model-actual-point-guard"
	ProofToolConsumerV1          ProofRequirementV1 = "harness.tool-consumer-current"
	ProofApplicationCurrentV1    ProofRequirementV1 = "harness.application-coordination-current"
	ProofContextContinuationV1   ProofRequirementV1 = "harness.context-refresh-continuation"
	ProofExecutableFactoryV1     ProofRequirementV1 = "harness.executable-factory-binding"
	ProofCleanupV1               ProofRequirementV1 = "harness.cleanup-conformance"
	ProofDeploymentAttestationV1 ProofRequirementV1 = "harness.deployment-attestation-current"
	ProofCompositionRootV1       ProofRequirementV1 = "harness.production-composition-root"
)

var requiredProductionProofsV1 = []ProofRequirementV1{
	ProofDurableSessionEventV1, ProofDurableAssemblyV1, ProofDurableRouteV1,
	ProofProductionRouteV1, ProofModelActualPointV1, ProofToolConsumerV1,
	ProofApplicationCurrentV1, ProofContextContinuationV1, ProofExecutableFactoryV1,
	ProofCleanupV1, ProofDeploymentAttestationV1, ProofCompositionRootV1,
}

var requiredPlanRolesV1 = []assemblercontract.PlanArtifactRoleV1{
	assemblercontract.ArtifactHarnessBootstrapV1,
	assemblercontract.ArtifactProfileV1,
	assemblercontract.ArtifactRuntimePolicyV1,
	assemblercontract.ArtifactHarnessStackV1,
	assemblercontract.ArtifactSemanticRouteV1,
	assemblercontract.ArtifactContextPlanV1,
	assemblercontract.ArtifactToolSurfaceV1,
	assemblercontract.ArtifactCapabilityGrantV1,
	assemblercontract.ArtifactExpectedInjectionV1,
}

type ReadinessV1 struct {
	State                    string
	ProductionEligible       bool
	RequiredProductionProofs []ProofRequirementV1
	MissingProductionProofs  []ProofRequirementV1
	CheckedUnixNano          int64
	ExpiresUnixNano          int64
}

type CandidateV1 struct {
	Release     assemblercontract.ComponentReleaseV1
	Conformance ConformanceV1
	Readiness   ReadinessV1
}

type BuilderV1 struct{ clock ClockV1 }

func NewBuilderV1(clock ClockV1) (*BuilderV1, error) {
	if nilLikeV1(clock) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Harness release candidate clock is unavailable")
	}
	return &BuilderV1{clock: clock}, nil
}

func (b *BuilderV1) BuildV1(request RequestV1) (CandidateV1, error) {
	if b == nil || nilLikeV1(b.clock) {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Harness release candidate builder is unavailable")
	}
	baseline := b.clock.Now()
	if baseline.IsZero() {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Harness release candidate clock is unavailable")
	}
	if request.TTL < MinimumTTL || request.TTL > MaximumTTL || request.TTL%time.Second != 0 {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCapabilityExpired, "Harness release candidate TTL is outside its exact bounded window")
	}
	if err := request.ArtifactDigest.Validate(); err != nil {
		return CandidateV1{}, err
	}
	for _, ref := range []assemblycontract.ObjectRefV1{request.SourceRef, request.PublisherRef, request.TrustRef} {
		if err := ref.Validate(); err != nil {
			return CandidateV1{}, err
		}
	}
	if err := validateEvidenceV1(request.EvidenceRefs); err != nil {
		return CandidateV1{}, err
	}
	if err := validatePlanArtifactsV1(request.PlanArtifacts); err != nil {
		return CandidateV1{}, err
	}
	conformance := OwnerLocalConformanceV1()
	if err := conformance.Validate(); err != nil {
		return CandidateV1{}, err
	}
	release, err := buildReleaseV1(request, baseline)
	if err != nil {
		return CandidateV1{}, err
	}
	fresh := b.clock.Now()
	if fresh.Before(baseline) {
		return CandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Harness release candidate clock regressed during assembly")
	}
	if fresh.UnixNano() >= release.ExpiresUnixNano {
		return CandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Harness release candidate expired during assembly")
	}
	proofs := append([]ProofRequirementV1(nil), requiredProductionProofsV1...)
	candidate := CandidateV1{
		Release: release, Conformance: conformance,
		Readiness: ReadinessV1{
			State: "assembly_candidate", ProductionEligible: false,
			RequiredProductionProofs: proofs, MissingProductionProofs: append([]ProofRequirementV1(nil), proofs...),
			CheckedUnixNano: fresh.UnixNano(), ExpiresUnixNano: release.ExpiresUnixNano,
		},
	}
	return candidate, candidate.ValidateCurrentV1(fresh)
}

func (c CandidateV1) ValidateCurrentV1(now time.Time) error {
	if c.Release.SupportMode != assemblercontract.SupportReferenceOnlyV1 || c.Readiness.State != "assembly_candidate" || c.Readiness.ProductionEligible {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Harness candidate cannot self-promote to production")
	}
	if err := c.Conformance.Validate(); err != nil {
		return err
	}
	if !sameProofsV1(c.Readiness.RequiredProductionProofs, requiredProductionProofsV1) || !sameProofsV1(c.Readiness.MissingProductionProofs, requiredProductionProofsV1) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Harness production proof boundary drifted")
	}
	if now.IsZero() || now.UnixNano() < c.Release.CreatedUnixNano || now.UnixNano() < c.Readiness.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Harness release candidate clock regressed")
	}
	if now.UnixNano() >= c.Release.ExpiresUnixNano || now.UnixNano() >= c.Readiness.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Harness release candidate expired")
	}
	return c.Release.Validate()
}

type PublisherV1 struct {
	builder *BuilderV1
	sink    assemblerports.ComponentReleasePublisherV1
	reader  assemblerports.ComponentReleaseReaderV1
}

func NewPublisherV1(builder *BuilderV1, sink assemblerports.ComponentReleasePublisherV1, reader assemblerports.ComponentReleaseReaderV1) (*PublisherV1, error) {
	if builder == nil || nilLikeV1(sink) || nilLikeV1(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Harness release publisher dependencies are unavailable")
	}
	return &PublisherV1{builder: builder, sink: sink, reader: reader}, nil
}

func (p *PublisherV1) PublishV1(ctx context.Context, request RequestV1) (CandidateV1, error) {
	if p == nil || p.builder == nil || nilLikeV1(p.sink) || nilLikeV1(p.reader) {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Harness release publisher is unavailable")
	}
	candidate, err := p.builder.BuildV1(request)
	if err != nil {
		return CandidateV1{}, err
	}
	published, err := p.sink.EnsureExactComponentReleaseV1(ctx, candidate.Release)
	if err != nil && core.HasCategory(err, core.ErrorIndeterminate) {
		published, err = p.reader.InspectExactComponentReleaseV1(context.WithoutCancel(ctx), candidate.Release.RefV1())
	}
	if err != nil {
		return CandidateV1{}, err
	}
	if published.ReleaseDigest != candidate.Release.ReleaseDigest || published.RefV1() != candidate.Release.RefV1() {
		return CandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Harness published release drifted from its exact candidate")
	}
	if err := published.Validate(); err != nil {
		return CandidateV1{}, err
	}
	candidate.Release = published
	return candidate, candidate.ValidateCurrentV1(p.builder.clock.Now())
}

func buildReleaseV1(request RequestV1, now time.Time) (assemblercontract.ComponentReleaseV1, error) {
	runRequest := schemaV1("run-loop-request")
	runResult := schemaV1("run-loop-result")
	cleanupRequest := schemaV1("cleanup-request")
	cleanupResult := schemaV1("cleanup-result")
	schemas := []runtimeports.SchemaRefV2{cleanupRequest, cleanupResult, runRequest, runResult}
	owners := []runtimeports.OwnerAssignmentV2{
		{Role: runtimeports.OwnerEffect, OwnerComponentID: ComponentIDV1},
		{Role: runtimeports.OwnerSettlement, OwnerComponentID: ComponentIDV1},
		{Role: runtimeports.OwnerCleanup, OwnerComponentID: ComponentIDV1},
	}
	capabilityTTL := uint64(request.TTL / time.Second)
	manifest := runtimeports.ComponentManifestV2{
		ContractVersion: runtimeports.BindingContractVersionV2, ComponentID: ComponentIDV1,
		Kind: ComponentKindV1, GovernanceCategory: GovernanceV1, SemanticVersion: SemanticVersionV1,
		ArtifactDigest: request.ArtifactDigest,
		Contract:       runtimeports.ContractBindingV2{Name: ContractNameV1, Version: SemanticVersionV1, Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}},
		Schemas:        schemas, Locality: runtimeports.LocalityHostControlPlane,
		Dependencies: []runtimeports.ComponentDependencyV2{}, RequiredCapabilities: []runtimeports.CapabilityRequirementV2{},
		ProvidedCapabilities: []runtimeports.ProvidedCapabilityV2{{Capability: CapabilityV1, TTLSeconds: capabilityTTL, Schemas: []runtimeports.SchemaRefV2{runRequest, runResult}}},
		Conformance:          runtimeports.ConformanceRestrictedControlled, ResidualClass: runtimeports.ResidualInspectable,
		Owners: owners, Credentials: []runtimeports.CredentialRequirementV2{}, OfflinePolicy: runtimeports.OfflineDenied,
		Extensions: []runtimeports.GovernanceExtensionV2{}, Annotations: []runtimeports.DisplayAnnotationV2{},
	}
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		return assemblercontract.ComponentReleaseV1{}, err
	}
	module := assemblycontract.ModuleDescriptorV1{
		ContractVersion: assemblycontract.ContractVersionV1, ModuleID: ModuleIDV1, Namespace: "praxis.harness",
		SemanticVersion: SemanticVersionV1, ArtifactDigest: request.ArtifactDigest,
		PublisherRef: request.PublisherRef, SourceRef: request.SourceRef,
		ComponentManifestRef: assemblycontract.ObjectRefV1{ID: string(ComponentIDV1), Revision: request.Revision, Digest: manifestDigest},
		Compatibility:        assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"},
		Capabilities:         []runtimeports.CapabilityNameV2{CapabilityV1}, Schemas: schemas,
		Locality: manifest.Locality, ResidualClass: manifest.ResidualClass, Owners: owners,
		CredentialRequirements: []runtimeports.NamespacedNameV2{},
	}
	capability := assemblycontract.CapabilityDescriptorV1{
		ContractVersion: assemblycontract.ContractVersionV1, Capability: CapabilityV1, Version: SemanticVersionV1,
		Schemas: []runtimeports.SchemaRefV2{runRequest, runResult}, Provided: true, TTLSeconds: capabilityTTL,
		EffectClass: "harness-run-loop-reference", OwnerCapability: CapabilityV1, Conformance: manifest.Conformance,
	}
	port := assemblycontract.PortSpecV1{
		ContractVersion: assemblycontract.ContractVersionV1, PortID: PortIDV1, OwnerCapability: CapabilityV1,
		RequestSchema: runRequest, ResponseSchema: runResult, OperationClass: "harness-run-loop-reference",
		Idempotency: "exact-run-attempt-inspect", FailureSemantics: "fail-closed-on-stale-route-gate-or-current",
		Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"},
	}
	factory := assemblycontract.ModuleFactoryDescriptorV1{
		ContractVersion: assemblycontract.ContractVersionV1, FactoryID: FactoryIDV1, ModuleRef: ModuleIDV1,
		ArtifactDigest: request.ArtifactDigest, ConstructionMode: assemblycontract.ConstructionTrustedInProcessGoV1,
		InputSchema: runRequest, OutputCapability: CapabilityV1, Lifecycle: assemblycontract.LifecycleGenerationV1,
		CleanupContractRef: assemblycontract.CleanupContractRefV1{
			Ref:             assemblycontract.ObjectRefV1{ID: "contract/harness/cleanup", Revision: 1, Digest: core.DigestBytes([]byte("contract/harness/cleanup@1"))},
			OwnerCapability: CapabilityV1, RequestSchema: cleanupRequest, ResultSchema: cleanupResult,
		},
		TrustRef: request.TrustRef,
	}
	release := assemblercontract.ComponentReleaseV1{
		ReleaseID: request.ReleaseID, Revision: request.Revision, SupportMode: assemblercontract.SupportReferenceOnlyV1,
		ComponentManifest: manifest, ModuleDescriptors: []assemblycontract.ModuleDescriptorV1{module},
		CapabilityDescriptors: []assemblycontract.CapabilityDescriptorV1{capability}, SlotSpecs: []assemblycontract.SlotSpecV1{},
		SlotContributions: []assemblycontract.SlotContributionV1{}, PortSpecs: []assemblycontract.PortSpecV1{port}, HookFaces: []assemblycontract.HookFaceSpecV1{},
		PhaseContributions: []assemblycontract.PhaseContributionV1{}, Dependencies: []assemblycontract.DependencySpecV1{},
		FactoryDescriptors: []assemblycontract.ModuleFactoryDescriptorV1{factory}, ProviderBindingCandidates: []assemblycontract.ProviderBindingCandidateV1{},
		RequiredPlanArtifacts: append([]assemblercontract.PlanArtifactV1(nil), request.PlanArtifacts...),
		SourceRef:             request.SourceRef, ArtifactDigest: request.ArtifactDigest,
		CertificationRef: assemblycontract.ObjectRefV1{ID: "certification/harness/assembly-candidate", Revision: request.Revision, Digest: runtimeports.EvidenceGenesisDigestV2},
		EvidenceRefs:     append([]assemblycontract.ObjectRefV1(nil), request.EvidenceRefs...),
		CreatedUnixNano:  now.UnixNano(), ExpiresUnixNano: now.Add(request.TTL).UnixNano(),
	}
	certificationDigest, err := assemblercontract.ComponentReleaseCertificationDigestV1(release)
	if err != nil {
		return assemblercontract.ComponentReleaseV1{}, err
	}
	release.CertificationRef.Digest = certificationDigest
	return assemblercontract.SealComponentReleaseV1(release)
}

func validateEvidenceV1(refs []assemblycontract.ObjectRefV1) error {
	if len(refs) == 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Harness release candidate requires owner-local assembly, route, and gate evidence")
	}
	seen := make(map[assemblycontract.ObjectRefV1]struct{}, len(refs))
	for _, ref := range refs {
		if err := ref.Validate(); err != nil {
			return err
		}
		if _, exists := seen[ref]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Harness release evidence contains a duplicate exact ref")
		}
		seen[ref] = struct{}{}
	}
	return nil
}

func validatePlanArtifactsV1(values []assemblercontract.PlanArtifactV1) error {
	if len(values) != len(requiredPlanRolesV1) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "Harness release candidate requires the complete declarative plan artifact set")
	}
	seen := make(map[assemblercontract.PlanArtifactRoleV1]struct{}, len(values))
	for _, value := range values {
		if err := value.Ref.Validate(); err != nil {
			return err
		}
		if _, exists := seen[value.Role]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Harness release candidate duplicates a plan artifact role")
		}
		seen[value.Role] = struct{}{}
	}
	for _, role := range requiredPlanRolesV1 {
		if _, exists := seen[role]; !exists {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "Harness release candidate is missing a required plan artifact role")
		}
	}
	return nil
}

func schemaV1(name string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.harness", Name: name, Version: SemanticVersionV1, MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("praxis.harness/schema/" + name + "@1.0.0"))}
}

func sameProofsV1(left, right []ProofRequirementV1) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func nilLikeV1(value any) bool {
	if value == nil {
		return true
	}
	kind := reflect.ValueOf(value).Kind()
	switch kind {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflect.ValueOf(value).IsNil()
	default:
		return false
	}
}
