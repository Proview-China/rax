// Package releasecandidate publishes Continuity's declarative, reference-only
// ComponentReleaseV1. It has no Host construction or production promotion API.
package releasecandidate

import (
	"context"
	"reflect"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	assemblerports "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ComponentIDV1     runtimeports.ComponentIDV2        = "components/continuity"
	ComponentKindV1   runtimeports.ComponentKindV2      = "praxis/continuity"
	GovernanceV1      runtimeports.GovernanceCategoryV2 = "praxis/core"
	CapabilityV1      runtimeports.CapabilityNameV2     = "praxis.continuity/checkpoint-manifest-current"
	ContractNameV1    runtimeports.NamespacedNameV2     = "praxis.continuity/contract"
	SemanticVersionV1                                   = "1.0.0"
	ModuleIDV1                                          = "module/continuity"
	FactoryIDV1                                         = "factory/continuity"
	PortIDV1                                            = "port/continuity/checkpoint-manifest-current"
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
	ProofDurableCheckpointV1  ProofRequirementV1 = "continuity.durable.checkpoint-store"
	ProofDurableTimelineV1    ProofRequirementV1 = "continuity.durable.timeline-store"
	ProofDurableArtifactV1    ProofRequirementV1 = "continuity.durable.artifact-store"
	ProofDurableHistoryV1     ProofRequirementV1 = "continuity.durable.history-store"
	ProofDurableRestoreV1     ProofRequirementV1 = "continuity.durable.restore-store"
	ProofCurrentIndexesV1     ProofRequirementV1 = "continuity.current-indexes"
	ProofRemoteBlobV1         ProofRequirementV1 = "continuity.remote-blob-provider"
	ProofParticipantCaptureV1 ProofRequirementV1 = "continuity.participant-capture"
	ProofRestoreExecuteV1     ProofRequirementV1 = "continuity.restore-execute"
	ProofCleanupV1            ProofRequirementV1 = "continuity.cleanup-conformance"
	ProofDeploymentV1         ProofRequirementV1 = "continuity.deployment-root"
)

var requiredProductionProofsV1 = []ProofRequirementV1{
	ProofDurableCheckpointV1, ProofDurableTimelineV1, ProofDurableArtifactV1,
	ProofDurableHistoryV1, ProofDurableRestoreV1, ProofCurrentIndexesV1,
	ProofRemoteBlobV1, ProofParticipantCaptureV1, ProofRestoreExecuteV1,
	ProofCleanupV1, ProofDeploymentV1,
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
	Release     assemblercontract.ComponentReleaseV1
	Conformance conformance.Manifest
	Readiness   ReadinessV1
}

type BuilderV1 struct{ clock ClockV1 }

func NewBuilderV1(clock ClockV1) (*BuilderV1, error) {
	if nilLikeV1(clock) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Continuity release candidate clock is unavailable")
	}
	return &BuilderV1{clock: clock}, nil
}

func (b *BuilderV1) BuildV1(request RequestV1) (CandidateV1, error) {
	if b == nil || nilLikeV1(b.clock) {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Continuity release candidate builder is unavailable")
	}
	baseline := b.clock.Now()
	if baseline.IsZero() {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Continuity release candidate clock is unavailable")
	}
	if request.TTL < MinimumTTL || request.TTL > MaximumTTL || request.TTL%time.Second != 0 {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCapabilityExpired, "Continuity release candidate TTL is outside its exact bounded window")
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
		return CandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Continuity release candidate requires durable and conformance evidence")
	}
	seen := make(map[assemblycontract.ObjectRefV1]struct{}, len(request.EvidenceRefs))
	for _, ref := range request.EvidenceRefs {
		if err := ref.Validate(); err != nil {
			return CandidateV1{}, err
		}
		if _, exists := seen[ref]; exists {
			return CandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Continuity release candidate evidence contains a duplicate exact ref")
		}
		seen[ref] = struct{}{}
	}
	wave := conformance.Wave1Manifest()
	if err := conformance.Validate(wave); err != nil {
		return CandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Continuity Wave 1 conformance drifted")
	}
	release, err := buildReleaseV1(request, baseline)
	if err != nil {
		return CandidateV1{}, err
	}
	fresh := b.clock.Now()
	if fresh.Before(baseline) {
		return CandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Continuity release candidate clock regressed during assembly")
	}
	if fresh.UnixNano() >= release.ExpiresUnixNano {
		return CandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Continuity release candidate expired during assembly")
	}
	proofs := append([]ProofRequirementV1(nil), requiredProductionProofsV1...)
	candidate := CandidateV1{
		Release: release, Conformance: wave,
		Readiness: ReadinessV1{
			State: "assembly_candidate", ProductionEligible: false,
			RequiredProductionProofs: proofs, MissingProductionProofs: append([]ProofRequirementV1(nil), proofs...),
			ProofAssessments: currentProofAssessmentsV1(),
			CheckedUnixNano:  fresh.UnixNano(), ExpiresUnixNano: release.ExpiresUnixNano,
		},
	}
	return candidate, candidate.ValidateCurrentV1(fresh)
}

func (c CandidateV1) ValidateCurrentV1(now time.Time) error {
	if c.Release.SupportMode != assemblercontract.SupportReferenceOnlyV1 || c.Readiness.State != "assembly_candidate" || c.Readiness.ProductionEligible {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Continuity candidate cannot self-promote to production")
	}
	if err := conformance.Validate(c.Conformance); err != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Continuity candidate conformance drifted")
	}
	if !sameProofsV1(c.Readiness.RequiredProductionProofs, requiredProductionProofsV1) || !sameProofsV1(c.Readiness.MissingProductionProofs, requiredProductionProofsV1) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Continuity production proof boundary drifted")
	}
	if !sameProofAssessmentsV1(c.Readiness.ProofAssessments, currentProofAssessmentsV1()) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Continuity production proof assessment drifted or self-promoted")
	}
	if now.IsZero() || now.UnixNano() < c.Release.CreatedUnixNano || now.UnixNano() < c.Readiness.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Continuity release candidate currentness clock regressed")
	}
	if now.UnixNano() >= c.Release.ExpiresUnixNano || now.UnixNano() >= c.Readiness.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Continuity release candidate expired")
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
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Continuity release publisher requires builder, publisher, and exact reader")
	}
	return &PublisherV1{builder: builder, sink: sink, reader: reader}, nil
}

func (p *PublisherV1) PublishV1(ctx context.Context, request RequestV1) (CandidateV1, error) {
	if p == nil || p.builder == nil || nilLikeV1(p.sink) || nilLikeV1(p.reader) {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Continuity release publisher is unavailable")
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
		return CandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Continuity published release drifted from its exact candidate")
	}
	if err := published.Validate(); err != nil {
		return CandidateV1{}, err
	}
	candidate.Release = published
	return candidate, candidate.ValidateCurrentV1(p.builder.clock.Now())
}

func buildReleaseV1(request RequestV1, now time.Time) (assemblercontract.ComponentReleaseV1, error) {
	requestSchema := schemaV1("checkpoint-manifest-current-request")
	responseSchema := schemaV1("checkpoint-manifest-current-result")
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
		ContractVersion: assemblycontract.ContractVersionV1, ModuleID: ModuleIDV1, Namespace: "praxis.continuity",
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
		EffectClass: "continuity-checkpoint-manifest-read", OwnerCapability: CapabilityV1, Conformance: manifest.Conformance,
	}
	port := assemblycontract.PortSpecV1{
		ContractVersion: assemblycontract.ContractVersionV1, PortID: PortIDV1, OwnerCapability: CapabilityV1,
		RequestSchema: requestSchema, ResponseSchema: responseSchema, OperationClass: "checkpoint-manifest-current-read",
		Idempotency: "exact-ref-read", FailureSemantics: "fail-closed-on-stale-or-drift",
		Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"},
	}
	factory := assemblycontract.ModuleFactoryDescriptorV1{
		ContractVersion: assemblycontract.ContractVersionV1, FactoryID: FactoryIDV1, ModuleRef: ModuleIDV1,
		ArtifactDigest: request.ArtifactDigest, ConstructionMode: assemblycontract.ConstructionTrustedInProcessGoV1,
		InputSchema: requestSchema, OutputCapability: CapabilityV1, Lifecycle: assemblycontract.LifecycleGenerationV1,
		CleanupContractRef: assemblycontract.CleanupContractRefV1{
			Ref:             assemblycontract.ObjectRefV1{ID: "contract/continuity/cleanup", Revision: 1, Digest: core.DigestBytes([]byte("contract/continuity/cleanup@1"))},
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
		CertificationRef: assemblycontract.ObjectRefV1{ID: "certification/continuity/assembly-candidate", Revision: request.Revision, Digest: runtimeports.EvidenceGenesisDigestV2},
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
	return runtimeports.SchemaRefV2{Namespace: "praxis.continuity", Name: name, Version: SemanticVersionV1, MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("praxis.continuity/schema/" + name + "@1.0.0"))}
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
