// Package releasecandidate exposes Review's declarative ComponentReleaseV1
// assembly candidate. It intentionally has no production promotion or Host
// construction API: the incomplete REV-D11 Evidence/current composition and
// host root must be closed outside Review before production can be certified.
package releasecandidate

import (
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ComponentIDV1     runtimeports.ComponentIDV2        = "components/review"
	ComponentKindV1   runtimeports.ComponentKindV2      = "praxis/review"
	CapabilityV1      runtimeports.CapabilityNameV2     = "praxis.review/decision-current"
	ContractNameV1    runtimeports.NamespacedNameV2     = "praxis.review/contract"
	GovernanceV1      runtimeports.GovernanceCategoryV2 = "praxis/core"
	SemanticVersionV1                                   = "1.0.0"
	ModuleIDV1                                          = "module/review"
	FactoryIDV1                                         = "factory/review"
	PortIDV1                                            = "port/review/decision-current"
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

type ProofRequirementV1 string

const (
	ProofDecisionCurrentV1   ProofRequirementV1 = "review.decision.current"
	ProofVerdictCurrentV1    ProofRequirementV1 = "review.verdict.current"
	ProofPolicyCurrentV1     ProofRequirementV1 = "review.policy.current"
	ProofEvidenceCurrentV1   ProofRequirementV1 = "review.evidence.current"
	ProofAuthorityCurrentV1  ProofRequirementV1 = "review.authority.current"
	ProofScopeCurrentV1      ProofRequirementV1 = "review.scope.current"
	ProofDurableStoreV1      ProofRequirementV1 = "review.durable-store.conformance"
	ProofRemoteEffectV1      ProofRequirementV1 = "review.remote-effect.conformance"
	ProofHumanInterventionV1 ProofRequirementV1 = "review.human-intervention.conformance"
	ProofCleanupV1           ProofRequirementV1 = "review.cleanup.conformance"
	ProofCompositionRootV1   ProofRequirementV1 = "praxis.composition-root.current"
)

var requiredProductionProofsV1 = []ProofRequirementV1{
	ProofDecisionCurrentV1, ProofVerdictCurrentV1, ProofPolicyCurrentV1,
	ProofEvidenceCurrentV1, ProofAuthorityCurrentV1, ProofScopeCurrentV1,
	ProofDurableStoreV1, ProofRemoteEffectV1, ProofHumanInterventionV1,
	ProofCleanupV1, ProofCompositionRootV1,
}

type ReadinessV1 struct {
	State                    string
	ProductionEligible       bool
	RequiredProductionProofs []ProofRequirementV1
	MissingProductionProofs  []ProofRequirementV1
	ProofAssessments         []ProofAssessmentV1
	CheckedUnixNano          int64
	ExpiresUnixNano          int64
}

type CandidateV1 struct {
	Release   assemblercontract.ComponentReleaseV1
	Readiness ReadinessV1
}

type BuilderV1 struct{ clock ClockV1 }

func NewBuilderV1(clock ClockV1) (*BuilderV1, error) {
	if nilcheck.IsNil(clock) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Review release candidate clock is unavailable")
	}
	return &BuilderV1{clock: clock}, nil
}

func (b *BuilderV1) BuildV1(request RequestV1) (CandidateV1, error) {
	if b == nil || nilcheck.IsNil(b.clock) {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Review release candidate builder is unavailable")
	}
	started := b.clock.Now()
	if started.IsZero() {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Review release candidate clock is unavailable")
	}
	if request.TTL < MinimumTTL || request.TTL > MaximumTTL || request.TTL%time.Second != 0 {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCapabilityExpired, "Review release candidate TTL is outside its bounded window")
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
		return CandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Review release candidate requires artifact and conformance evidence")
	}
	seenEvidence := make(map[assemblycontract.ObjectRefV1]struct{}, len(request.EvidenceRefs))
	for _, ref := range request.EvidenceRefs {
		if err := ref.Validate(); err != nil {
			return CandidateV1{}, err
		}
		if _, exists := seenEvidence[ref]; exists {
			return CandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review release candidate evidence set contains a duplicate exact ref")
		}
		seenEvidence[ref] = struct{}{}
	}

	release, err := buildReleaseV1(request, started)
	if err != nil {
		return CandidateV1{}, err
	}
	fresh := b.clock.Now()
	if fresh.Before(started) {
		return CandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review release candidate clock regressed during assembly")
	}
	if fresh.UnixNano() >= release.ExpiresUnixNano {
		return CandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Review release candidate expired during assembly")
	}
	proofs := append([]ProofRequirementV1(nil), requiredProductionProofsV1...)
	assessments := currentProofAssessmentsV1()
	candidate := CandidateV1{Release: release, Readiness: ReadinessV1{
		State: "assembly_candidate", ProductionEligible: false,
		RequiredProductionProofs: proofs,
		MissingProductionProofs:  append([]ProofRequirementV1(nil), proofs...),
		ProofAssessments:         assessments,
		CheckedUnixNano:          fresh.UnixNano(), ExpiresUnixNano: release.ExpiresUnixNano,
	}}
	return candidate, candidate.ValidateCurrentV1(fresh)
}

func (c CandidateV1) ValidateCurrentV1(now time.Time) error {
	if c.Release.SupportMode != assemblercontract.SupportReferenceOnlyV1 || c.Readiness.State != "assembly_candidate" || c.Readiness.ProductionEligible {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Review candidate cannot self-promote to production")
	}
	if !sameProofsV1(c.Readiness.RequiredProductionProofs, requiredProductionProofsV1) || !sameProofsV1(c.Readiness.MissingProductionProofs, requiredProductionProofsV1) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Review production proof boundary drifted")
	}
	if !sameProofAssessmentsV1(c.Readiness.ProofAssessments, currentProofAssessmentsV1()) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Review production proof assessment drifted")
	}
	if now.IsZero() || now.UnixNano() < c.Release.CreatedUnixNano || now.UnixNano() < c.Readiness.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review release candidate currentness clock regressed")
	}
	if now.UnixNano() >= c.Release.ExpiresUnixNano || now.UnixNano() >= c.Readiness.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Review release candidate is expired")
	}
	return c.Release.Validate()
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

func buildReleaseV1(request RequestV1, now time.Time) (assemblercontract.ComponentReleaseV1, error) {
	requestSchema := schemaV1("decision-current-request")
	responseSchema := schemaV1("decision-current-result")
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
		ContractVersion: assemblycontract.ContractVersionV1, ModuleID: ModuleIDV1, Namespace: "praxis.review",
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
		EffectClass: "review-decision-current-read", OwnerCapability: CapabilityV1, Conformance: manifest.Conformance,
	}
	port := assemblycontract.PortSpecV1{
		ContractVersion: assemblycontract.ContractVersionV1, PortID: PortIDV1, OwnerCapability: CapabilityV1,
		RequestSchema: requestSchema, ResponseSchema: responseSchema, OperationClass: "review-decision-current-read",
		Idempotency: "exact-ref-read", FailureSemantics: "fail-closed-on-stale-or-drift",
		Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"},
	}
	factory := assemblycontract.ModuleFactoryDescriptorV1{
		ContractVersion: assemblycontract.ContractVersionV1, FactoryID: FactoryIDV1, ModuleRef: ModuleIDV1,
		ArtifactDigest: request.ArtifactDigest, ConstructionMode: assemblycontract.ConstructionTrustedInProcessGoV1,
		InputSchema: requestSchema, OutputCapability: CapabilityV1, Lifecycle: assemblycontract.LifecycleGenerationV1,
		CleanupContractRef: assemblycontract.CleanupContractRefV1{Ref: assemblycontract.ObjectRefV1{ID: "contract/review/cleanup", Revision: 1, Digest: core.DigestBytes([]byte("contract/review/cleanup@1"))}, OwnerCapability: CapabilityV1, RequestSchema: cleanupRequest, ResultSchema: cleanupResult},
		TrustRef:           request.TrustRef,
	}
	release := assemblercontract.ComponentReleaseV1{
		ReleaseID: request.ReleaseID, Revision: request.Revision, SupportMode: assemblercontract.SupportReferenceOnlyV1,
		ComponentManifest: manifest, ModuleDescriptors: []assemblycontract.ModuleDescriptorV1{module},
		CapabilityDescriptors: []assemblycontract.CapabilityDescriptorV1{capability}, SlotSpecs: []assemblycontract.SlotSpecV1{},
		SlotContributions: []assemblycontract.SlotContributionV1{}, PortSpecs: []assemblycontract.PortSpecV1{port}, HookFaces: []assemblycontract.HookFaceSpecV1{},
		PhaseContributions: []assemblycontract.PhaseContributionV1{}, Dependencies: []assemblycontract.DependencySpecV1{},
		FactoryDescriptors: []assemblycontract.ModuleFactoryDescriptorV1{factory}, ProviderBindingCandidates: []assemblycontract.ProviderBindingCandidateV1{},
		RequiredPlanArtifacts: []assemblercontract.PlanArtifactV1{}, SourceRef: request.SourceRef, ArtifactDigest: request.ArtifactDigest,
		CertificationRef: assemblycontract.ObjectRefV1{ID: "certification/review/assembly-candidate", Revision: request.Revision, Digest: runtimeports.EvidenceGenesisDigestV2},
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
	return runtimeports.SchemaRefV2{Namespace: "praxis.review", Name: name, Version: SemanticVersionV1, MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("praxis.review/schema/" + name + "@1.0.0"))}
}
