// Package releasecandidate exposes Model Invoker's declarative assembly
// candidate. It has no production promotion or provider construction API.
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
	ComponentIDV1              runtimeports.ComponentIDV2        = "components/model-invoker"
	ComponentKindV1            runtimeports.ComponentKindV2      = "praxis/model-invoker"
	GovernanceV1               runtimeports.GovernanceCategoryV2 = "praxis/core"
	CapabilityV1               runtimeports.CapabilityNameV2     = "praxis.model-invoker/prepared-invocation-current"
	ContractNameV1             runtimeports.NamespacedNameV2     = "praxis.model-invoker/contract"
	SemanticVersionV1                                            = "1.0.0"
	ModuleIDV1                                                   = "module/model-invoker"
	FactoryIDV1                                                  = "factory/model-invoker/prepared-invocation-current"
	PortIDV1                                                     = "port/model-invoker/prepared-invocation-current"
	ReadinessContractVersionV1                                   = "praxis.model-invoker/component-readiness/v1"
	ReadinessObjectKindV1                                        = "ModelInvokerReadinessV1"
	MinimumTTL                                                   = time.Second
	MaximumTTL                                                   = 24 * time.Hour
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
	ProofDurablePreparedHistoricalV1 ProofRequirementV1 = "model-invoker.prepared-historical.durable"
	ProofDurablePreparedCurrentV1    ProofRequirementV1 = "model-invoker.prepared-current.durable"
	ProofRouteCurrentV1              ProofRequirementV1 = "model-invoker.route.current"
	ProofProfileCurrentV1            ProofRequirementV1 = "model-invoker.profile.current"
	ProofRegistryCurrentV1           ProofRequirementV1 = "model-invoker.registry-deployment.current"
	ProofProviderCurrentV1           ProofRequirementV1 = "model-invoker.provider-deployment.current"
	ProofDurableCommitAckV1          ProofRequirementV1 = "model-invoker.commit-gate-ack.durable"
	ProofHarnessBridgeV1             ProofRequirementV1 = "model-invoker.harness-bridge.deployment"
	ProofDispatchActualPointV1       ProofRequirementV1 = "model-invoker.dispatch-actual-point.conformance"
	ProofCleanupSettlementV1         ProofRequirementV1 = "model-invoker.cleanup-settlement.conformance"
	ProofDeploymentCertificationV1   ProofRequirementV1 = "model-invoker.deployment-root.certification"
)

var requiredProductionProofsV1 = []ProofRequirementV1{
	ProofDurablePreparedHistoricalV1, ProofDurablePreparedCurrentV1,
	ProofRouteCurrentV1, ProofProfileCurrentV1, ProofRegistryCurrentV1,
	ProofProviderCurrentV1, ProofDurableCommitAckV1, ProofHarnessBridgeV1,
	ProofDispatchActualPointV1, ProofCleanupSettlementV1, ProofDeploymentCertificationV1,
}

func RequiredProductionProofsV1() []ProofRequirementV1 {
	return append([]ProofRequirementV1(nil), requiredProductionProofsV1...)
}

type ReadinessV1 struct {
	ContractVersion          string                                  `json:"contract_version"`
	ObjectKind               string                                  `json:"object_kind"`
	ReleaseRef               assemblercontract.ComponentReleaseRefV1 `json:"release_ref"`
	State                    string                                  `json:"state"`
	ProductionEligible       bool                                    `json:"production_eligible"`
	RequiredProductionProofs []ProofRequirementV1                    `json:"required_production_proofs"`
	MissingProductionProofs  []ProofRequirementV1                    `json:"missing_production_proofs"`
	CheckedUnixNano          int64                                   `json:"checked_unix_nano"`
	ExpiresUnixNano          int64                                   `json:"expires_unix_nano"`
	Digest                   core.Digest                             `json:"digest"`
}

func ReadinessDigestV1(value ReadinessV1) (core.Digest, error) {
	value.Digest = ""
	return core.CanonicalJSONDigest("praxis.model-invoker.component-readiness", ReadinessContractVersionV1, ReadinessObjectKindV1, value)
}

func SealReadinessV1(value ReadinessV1) (ReadinessV1, error) {
	value.ContractVersion = ReadinessContractVersionV1
	value.ObjectKind = ReadinessObjectKindV1
	value.Digest = ""
	if err := value.validateShapeV1(); err != nil {
		return ReadinessV1{}, err
	}
	digest, err := ReadinessDigestV1(value)
	if err != nil {
		return ReadinessV1{}, err
	}
	value.Digest = digest
	return value, value.ValidateCurrentV1(time.Unix(0, value.CheckedUnixNano))
}

func (value ReadinessV1) validateShapeV1() error {
	if value.ContractVersion != ReadinessContractVersionV1 || value.ObjectKind != ReadinessObjectKindV1 || value.ReleaseRef.Validate() != nil || value.State != "assembly_candidate" || value.ProductionEligible || value.CheckedUnixNano <= 0 || value.ExpiresUnixNano <= value.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Model Invoker readiness cannot claim production or has invalid coordinates")
	}
	if !sameProofsV1(value.RequiredProductionProofs, requiredProductionProofsV1) || !sameProofsV1(value.MissingProductionProofs, requiredProductionProofsV1) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Model Invoker production P0 proof boundary drifted")
	}
	return nil
}

func (value ReadinessV1) ValidateCurrentV1(now time.Time) error {
	if err := value.validateShapeV1(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < value.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model Invoker readiness clock regressed")
	}
	if now.UnixNano() >= value.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Model Invoker readiness expired")
	}
	digest, err := ReadinessDigestV1(value)
	if err != nil || digest != value.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model Invoker readiness digest drifted")
	}
	return nil
}

type CandidateV1 struct {
	Release   assemblercontract.ComponentReleaseV1
	Readiness ReadinessV1
}

type BuilderV1 struct{ clock ClockV1 }

func NewBuilderV1(clock ClockV1) (*BuilderV1, error) {
	if nilLikeV1(clock) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Model Invoker release candidate clock is unavailable")
	}
	return &BuilderV1{clock: clock}, nil
}

func (builder *BuilderV1) BuildV1(request RequestV1) (CandidateV1, error) {
	if builder == nil || nilLikeV1(builder.clock) {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Model Invoker release candidate builder is unavailable")
	}
	started := builder.clock.Now()
	if started.IsZero() {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Model Invoker release candidate clock returned zero")
	}
	if err := validateRequestV1(request); err != nil {
		return CandidateV1{}, err
	}
	release, err := buildReleaseV1(request, started)
	if err != nil {
		return CandidateV1{}, err
	}
	fresh := builder.clock.Now()
	if fresh.Before(started) {
		return CandidateV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model Invoker release candidate clock regressed")
	}
	proofs := RequiredProductionProofsV1()
	readiness, err := SealReadinessV1(ReadinessV1{ReleaseRef: release.RefV1(), State: "assembly_candidate", RequiredProductionProofs: proofs, MissingProductionProofs: append([]ProofRequirementV1(nil), proofs...), CheckedUnixNano: fresh.UnixNano(), ExpiresUnixNano: release.ExpiresUnixNano})
	if err != nil {
		return CandidateV1{}, err
	}
	candidate := CandidateV1{Release: release, Readiness: readiness}
	return candidate, candidate.ValidateCurrentV1(fresh)
}

func (candidate CandidateV1) ValidateCurrentV1(now time.Time) error {
	if candidate.Release.SupportMode != assemblercontract.SupportReferenceOnlyV1 || candidate.Readiness.ReleaseRef != candidate.Release.RefV1() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Model Invoker candidate cannot self-promote or splice release identity")
	}
	if err := candidate.Release.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < candidate.Release.CreatedUnixNano || now.UnixNano() >= candidate.Release.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Model Invoker release candidate is not current")
	}
	return candidate.Readiness.ValidateCurrentV1(now)
}

type PublisherV1 struct {
	builder *BuilderV1
	sink    assemblerports.ComponentReleasePublisherV1
	reader  assemblerports.ComponentReleaseReaderV1
}

func NewPublisherV1(builder *BuilderV1, sink assemblerports.ComponentReleasePublisherV1, reader assemblerports.ComponentReleaseReaderV1) (*PublisherV1, error) {
	if builder == nil || nilLikeV1(sink) || nilLikeV1(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Model Invoker release publisher requires builder and exact catalog ports")
	}
	return &PublisherV1{builder: builder, sink: sink, reader: reader}, nil
}

func (publisher *PublisherV1) PublishV1(ctx context.Context, request RequestV1) (CandidateV1, error) {
	if publisher == nil || publisher.builder == nil || nilLikeV1(publisher.sink) || nilLikeV1(publisher.reader) {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Model Invoker release publisher is unavailable")
	}
	if ctx == nil {
		return CandidateV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model Invoker release publication context is required")
	}
	candidate, err := publisher.builder.BuildV1(request)
	if err != nil {
		return CandidateV1{}, err
	}
	published, err := publisher.sink.EnsureExactComponentReleaseV1(ctx, candidate.Release)
	if err != nil && core.HasCategory(err, core.ErrorIndeterminate) {
		published, err = publisher.reader.InspectExactComponentReleaseV1(context.WithoutCancel(ctx), candidate.Release.RefV1())
	}
	if err != nil {
		return CandidateV1{}, err
	}
	if published.RefV1() != candidate.Release.RefV1() || published.ReleaseDigest != candidate.Release.ReleaseDigest {
		return CandidateV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model Invoker published release drifted")
	}
	if err := published.Validate(); err != nil {
		return CandidateV1{}, err
	}
	candidate.Release = published
	return candidate, candidate.ValidateCurrentV1(publisher.builder.clock.Now())
}

func validateRequestV1(request RequestV1) error {
	if request.ReleaseID == "" || request.Revision == 0 || request.TTL < MinimumTTL || request.TTL > MaximumTTL || request.TTL%time.Second != 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCapabilityExpired, "Model Invoker release request identity or TTL is invalid")
	}
	if err := request.ArtifactDigest.Validate(); err != nil {
		return err
	}
	for _, ref := range []assemblycontract.ObjectRefV1{request.SourceRef, request.PublisherRef, request.TrustRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if len(request.EvidenceRefs) == 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Model Invoker release candidate requires exact evidence")
	}
	seen := map[assemblycontract.ObjectRefV1]struct{}{}
	for _, ref := range request.EvidenceRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
		if _, ok := seen[ref]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Model Invoker release evidence is duplicated")
		}
		seen[ref] = struct{}{}
	}
	return nil
}

func buildReleaseV1(request RequestV1, now time.Time) (assemblercontract.ComponentReleaseV1, error) {
	requestSchema, resultSchema := schemaV1("prepared-current-request"), schemaV1("prepared-current-result")
	cleanupRequest, cleanupResult := schemaV1("cleanup-request"), schemaV1("cleanup-result")
	schemas := []runtimeports.SchemaRefV2{cleanupRequest, cleanupResult, requestSchema, resultSchema}
	owners := []runtimeports.OwnerAssignmentV2{{Role: runtimeports.OwnerEffect, OwnerComponentID: ComponentIDV1}, {Role: runtimeports.OwnerSettlement, OwnerComponentID: ComponentIDV1}, {Role: runtimeports.OwnerCleanup, OwnerComponentID: ComponentIDV1}}
	ttl := uint64(request.TTL / time.Second)
	manifest := runtimeports.ComponentManifestV2{ContractVersion: runtimeports.BindingContractVersionV2, ComponentID: ComponentIDV1, Kind: ComponentKindV1, GovernanceCategory: GovernanceV1, SemanticVersion: SemanticVersionV1, ArtifactDigest: request.ArtifactDigest, Contract: runtimeports.ContractBindingV2{Name: ContractNameV1, Version: SemanticVersionV1, Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}, Schemas: schemas, Locality: runtimeports.LocalityHostControlPlane, ProvidedCapabilities: []runtimeports.ProvidedCapabilityV2{{Capability: CapabilityV1, TTLSeconds: ttl, Schemas: []runtimeports.SchemaRefV2{requestSchema, resultSchema}}}, Conformance: runtimeports.ConformanceRestrictedControlled, ResidualClass: runtimeports.ResidualInspectable, Owners: owners, OfflinePolicy: runtimeports.OfflineDenied}
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		return assemblercontract.ComponentReleaseV1{}, err
	}
	module := assemblycontract.ModuleDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, ModuleID: ModuleIDV1, Namespace: "praxis.model-invoker", SemanticVersion: SemanticVersionV1, ArtifactDigest: request.ArtifactDigest, PublisherRef: request.PublisherRef, SourceRef: request.SourceRef, ComponentManifestRef: assemblycontract.ObjectRefV1{ID: string(ComponentIDV1), Revision: request.Revision, Digest: manifestDigest}, Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, Capabilities: []runtimeports.CapabilityNameV2{CapabilityV1}, Schemas: schemas, Locality: manifest.Locality, ResidualClass: manifest.ResidualClass, Owners: owners}
	capability := assemblycontract.CapabilityDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, Capability: CapabilityV1, Version: SemanticVersionV1, Schemas: []runtimeports.SchemaRefV2{requestSchema, resultSchema}, Provided: true, TTLSeconds: ttl, EffectClass: "model-prepared-current-read", OwnerCapability: CapabilityV1, Conformance: manifest.Conformance}
	port := assemblycontract.PortSpecV1{ContractVersion: assemblycontract.ContractVersionV1, PortID: PortIDV1, OwnerCapability: CapabilityV1, RequestSchema: requestSchema, ResponseSchema: resultSchema, OperationClass: "prepared-model-invocation-current-read", Idempotency: "exact-ref-read", FailureSemantics: "fail-closed-on-stale-drift-or-absent", Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}
	factory := assemblycontract.ModuleFactoryDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, FactoryID: FactoryIDV1, ModuleRef: ModuleIDV1, ArtifactDigest: request.ArtifactDigest, ConstructionMode: assemblycontract.ConstructionTrustedInProcessGoV1, InputSchema: requestSchema, OutputCapability: CapabilityV1, Lifecycle: assemblycontract.LifecycleGenerationV1, CleanupContractRef: assemblycontract.CleanupContractRefV1{Ref: assemblycontract.ObjectRefV1{ID: "contract/model-invoker/cleanup", Revision: 1, Digest: core.DigestBytes([]byte("contract/model-invoker/cleanup@1"))}, OwnerCapability: CapabilityV1, RequestSchema: cleanupRequest, ResultSchema: cleanupResult}, TrustRef: request.TrustRef}
	release := assemblercontract.ComponentReleaseV1{ReleaseID: request.ReleaseID, Revision: request.Revision, SupportMode: assemblercontract.SupportReferenceOnlyV1, ComponentManifest: manifest, ModuleDescriptors: []assemblycontract.ModuleDescriptorV1{module}, CapabilityDescriptors: []assemblycontract.CapabilityDescriptorV1{capability}, SlotSpecs: []assemblycontract.SlotSpecV1{}, SlotContributions: []assemblycontract.SlotContributionV1{}, PortSpecs: []assemblycontract.PortSpecV1{port}, HookFaces: []assemblycontract.HookFaceSpecV1{}, PhaseContributions: []assemblycontract.PhaseContributionV1{}, Dependencies: []assemblycontract.DependencySpecV1{}, FactoryDescriptors: []assemblycontract.ModuleFactoryDescriptorV1{factory}, ProviderBindingCandidates: []assemblycontract.ProviderBindingCandidateV1{}, RequiredPlanArtifacts: []assemblercontract.PlanArtifactV1{}, SourceRef: request.SourceRef, ArtifactDigest: request.ArtifactDigest, CertificationRef: assemblycontract.ObjectRefV1{ID: "certification/model-invoker/assembly-candidate", Revision: request.Revision, Digest: runtimeports.EvidenceGenesisDigestV2}, EvidenceRefs: append([]assemblycontract.ObjectRefV1(nil), request.EvidenceRefs...), CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(request.TTL).UnixNano()}
	certification, err := assemblercontract.ComponentReleaseCertificationDigestV1(release)
	if err != nil {
		return assemblercontract.ComponentReleaseV1{}, err
	}
	release.CertificationRef.Digest = certification
	return assemblercontract.SealComponentReleaseV1(release)
}

func schemaV1(name string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.model-invoker", Name: name, Version: SemanticVersionV1, MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("praxis.model-invoker/schema/" + name + "@1.0.0"))}
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
	}
	return false
}
