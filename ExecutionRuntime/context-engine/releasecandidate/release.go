// Package releasecandidate publishes Context Engine's declarative,
// reference-only ComponentReleaseV1. It exposes no Host factory or production
// promotion API.
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
	ComponentIDV1     runtimeports.ComponentIDV2        = "components/context"
	ComponentKindV1   runtimeports.ComponentKindV2      = "praxis/context"
	GovernanceV1      runtimeports.GovernanceCategoryV2 = "praxis/core"
	CapabilityV1      runtimeports.CapabilityNameV2     = "praxis.context/execute"
	ContractNameV1    runtimeports.NamespacedNameV2     = "praxis.context/contract"
	SemanticVersionV1                                   = "1.0.0"
	ModuleIDV1                                          = "module/context"
	FactoryIDV1                                         = "factory/context"
	PortIDV1                                            = "port/context/turn-refresh"
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
	TTL            time.Duration
}

type ConformanceV1 struct {
	ReferenceOnly       bool
	ProductionSLA       bool
	DurableState        bool
	DurableCache        bool
	ProviderCalls       uint64
	HarnessContinuation bool
}

func OwnerLocalConformanceV1() ConformanceV1 {
	return ConformanceV1{ReferenceOnly: true}
}

func (c ConformanceV1) Validate() error {
	if !c.ReferenceOnly || c.ProductionSLA || c.DurableState || c.DurableCache || c.ProviderCalls != 0 || c.HarnessContinuation {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Context owner-local conformance cannot claim production state, cache, provider, or continuation")
	}
	return nil
}

type ProofRequirementV1 string

const (
	ProofDurableStateV1     ProofRequirementV1 = "context.durable-state"
	ProofDurableCacheV1     ProofRequirementV1 = "context.durable-cache"
	ProofSourceCurrentV1    ProofRequirementV1 = "context.source-current"
	ProofProviderCurrentV1  ProofRequirementV1 = "context.provider-current"
	ProofInjectionCurrentV1 ProofRequirementV1 = "context.injection-current"
	ProofRefreshCurrentV1   ProofRequirementV1 = "context.refresh-current"
	ProofContinuationV1     ProofRequirementV1 = "context.harness-continuation"
	ProofCleanupV1          ProofRequirementV1 = "context.cleanup-conformance"
	ProofDeploymentV1       ProofRequirementV1 = "context.deployment-root"
)

var requiredProductionProofsV1 = []ProofRequirementV1{
	ProofDurableStateV1, ProofDurableCacheV1, ProofSourceCurrentV1,
	ProofProviderCurrentV1, ProofInjectionCurrentV1, ProofRefreshCurrentV1,
	ProofContinuationV1, ProofCleanupV1, ProofDeploymentV1,
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
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Context release candidate clock is unavailable")
	}
	return &BuilderV1{clock: clock}, nil
}

func (b *BuilderV1) BuildV1(request RequestV1) (CandidateV1, error) {
	if b == nil || nilLikeV1(b.clock) {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Context release candidate builder is unavailable")
	}
	baseline := b.clock.Now()
	if baseline.IsZero() {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Context release candidate clock is unavailable")
	}
	if request.TTL < MinimumTTL || request.TTL > MaximumTTL || request.TTL%time.Second != 0 {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCapabilityExpired, "Context release candidate TTL is outside its exact bounded window")
	}
	if err := request.ArtifactDigest.Validate(); err != nil {
		return CandidateV1{}, err
	}
	for _, ref := range []assemblycontract.ObjectRefV1{request.SourceRef, request.PublisherRef, request.TrustRef} {
		if err := ref.Validate(); err != nil {
			return CandidateV1{}, err
		}
	}
	if len(request.EvidenceRefs) == 0 {
		return CandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Context release candidate requires owner-local evidence")
	}
	seen := make(map[assemblycontract.ObjectRefV1]struct{}, len(request.EvidenceRefs))
	for _, ref := range request.EvidenceRefs {
		if err := ref.Validate(); err != nil {
			return CandidateV1{}, err
		}
		if _, exists := seen[ref]; exists {
			return CandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Context release evidence contains a duplicate exact ref")
		}
		seen[ref] = struct{}{}
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
		return CandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Context release candidate clock regressed during assembly")
	}
	if fresh.UnixNano() >= release.ExpiresUnixNano {
		return CandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Context release candidate expired during assembly")
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
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Context candidate cannot self-promote to production")
	}
	if err := c.Conformance.Validate(); err != nil {
		return err
	}
	if !sameProofsV1(c.Readiness.RequiredProductionProofs, requiredProductionProofsV1) || !sameProofsV1(c.Readiness.MissingProductionProofs, requiredProductionProofsV1) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Context production proof boundary drifted")
	}
	if now.IsZero() || now.UnixNano() < c.Release.CreatedUnixNano || now.UnixNano() < c.Readiness.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Context release candidate clock regressed")
	}
	if now.UnixNano() >= c.Release.ExpiresUnixNano || now.UnixNano() >= c.Readiness.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Context release candidate expired")
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
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Context release publisher dependencies are unavailable")
	}
	return &PublisherV1{builder: builder, sink: sink, reader: reader}, nil
}

func (p *PublisherV1) PublishV1(ctx context.Context, request RequestV1) (CandidateV1, error) {
	if p == nil || p.builder == nil || nilLikeV1(p.sink) || nilLikeV1(p.reader) {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Context release publisher is unavailable")
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
		return CandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Context published release drifted from its exact candidate")
	}
	if err := published.Validate(); err != nil {
		return CandidateV1{}, err
	}
	candidate.Release = published
	return candidate, candidate.ValidateCurrentV1(p.builder.clock.Now())
}

func buildReleaseV1(request RequestV1, now time.Time) (assemblercontract.ComponentReleaseV1, error) {
	requestSchema := schemaV1("turn-refresh-request")
	responseSchema := schemaV1("turn-refresh-result")
	cleanupRequest := schemaV1("cleanup-request")
	cleanupResult := schemaV1("cleanup-result")
	schemas := []runtimeports.SchemaRefV2{cleanupRequest, cleanupResult, requestSchema, responseSchema}
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
		ProvidedCapabilities: []runtimeports.ProvidedCapabilityV2{{Capability: CapabilityV1, TTLSeconds: capabilityTTL, Schemas: []runtimeports.SchemaRefV2{requestSchema, responseSchema}}},
		Conformance:          runtimeports.ConformanceRestrictedControlled, ResidualClass: runtimeports.ResidualInspectable,
		Owners: owners, Credentials: []runtimeports.CredentialRequirementV2{}, OfflinePolicy: runtimeports.OfflineDenied,
		Extensions: []runtimeports.GovernanceExtensionV2{}, Annotations: []runtimeports.DisplayAnnotationV2{},
	}
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		return assemblercontract.ComponentReleaseV1{}, err
	}
	module := assemblycontract.ModuleDescriptorV1{
		ContractVersion: assemblycontract.ContractVersionV1, ModuleID: ModuleIDV1, Namespace: "praxis.context",
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
		Schemas: []runtimeports.SchemaRefV2{requestSchema, responseSchema}, Provided: true, TTLSeconds: capabilityTTL,
		EffectClass: "context-turn-refresh-reference", OwnerCapability: CapabilityV1, Conformance: manifest.Conformance,
	}
	port := assemblycontract.PortSpecV1{
		ContractVersion: assemblycontract.ContractVersionV1, PortID: PortIDV1, OwnerCapability: CapabilityV1,
		RequestSchema: requestSchema, ResponseSchema: responseSchema, OperationClass: "context-turn-refresh-reference",
		Idempotency: "exact-attempt-inspect", FailureSemantics: "fail-closed-on-stale-or-drift",
		Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"},
	}
	factory := assemblycontract.ModuleFactoryDescriptorV1{
		ContractVersion: assemblycontract.ContractVersionV1, FactoryID: FactoryIDV1, ModuleRef: ModuleIDV1,
		ArtifactDigest: request.ArtifactDigest, ConstructionMode: assemblycontract.ConstructionTrustedInProcessGoV1,
		InputSchema: requestSchema, OutputCapability: CapabilityV1, Lifecycle: assemblycontract.LifecycleGenerationV1,
		CleanupContractRef: assemblycontract.CleanupContractRefV1{
			Ref:             assemblycontract.ObjectRefV1{ID: "contract/context/cleanup", Revision: 1, Digest: core.DigestBytes([]byte("contract/context/cleanup@1"))},
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
		RequiredPlanArtifacts: []assemblercontract.PlanArtifactV1{}, SourceRef: request.SourceRef, ArtifactDigest: request.ArtifactDigest,
		CertificationRef: assemblycontract.ObjectRefV1{ID: "certification/context/assembly-candidate", Revision: request.Revision, Digest: runtimeports.EvidenceGenesisDigestV2},
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

func schemaV1(name string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.context", Name: name, Version: SemanticVersionV1, MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("praxis.context/schema/" + name + "@1.0.0"))}
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
