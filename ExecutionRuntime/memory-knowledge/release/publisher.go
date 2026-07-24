package release

import (
	"context"
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	mkcontract "github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"reflect"
	"time"
)

type PublisherV1 struct {
	local      LocalReadinessReaderV1
	production ProductionReadinessReaderV1
	catalog    CatalogPortV1
	clock      func() time.Time
}

func NewPublisherV1(l LocalReadinessReaderV1, p ProductionReadinessReaderV1, c CatalogPortV1, clock func() time.Time) (*PublisherV1, error) {
	if nilLike(l) || nilLike(p) || nilLike(c) || nilLike(clock) {
		return nil, invalid("publisher dependencies are incomplete")
	}
	return &PublisherV1{l, p, c, clock}, nil
}
func (p *PublisherV1) Publish(ctx context.Context, r PublicationRequestV1) (PublicationResultV1, error) {
	if p == nil || ctx == nil {
		return PublicationResultV1{}, invalid("publisher or context is nil")
	}
	start := p.clock()
	if e := r.Validate(start); e != nil {
		return PublicationResultV1{}, e
	}
	l, e := p.local.InspectMemoryKnowledgeLocalReadinessV1(ctx, r.ReleaseID, r.Revision)
	hasL := e == nil
	if e != nil && !core.HasCategory(e, core.ErrorNotFound) {
		return PublicationResultV1{}, e
	}
	if hasL {
		if l.ReleaseID != r.ReleaseID || l.Revision != r.Revision || l.ArtifactDigest != r.ArtifactDigest {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "local readiness targets another release")
		}
		if e = l.ValidateCurrent(p.fresh(start)); e != nil {
			return PublicationResultV1{}, e
		}
	}
	prod, e := p.production.InspectMemoryKnowledgeProductionReadinessV1(ctx, r.ReleaseID, r.Revision)
	hasP := e == nil
	if e != nil && !core.HasCategory(e, core.ErrorNotFound) {
		return PublicationResultV1{}, e
	}
	if hasP && !hasL {
		return PublicationResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "production requires local closure")
	}
	if hasP {
		if prod.ReleaseID != r.ReleaseID || prod.Revision != r.Revision || prod.ArtifactDigest != r.ArtifactDigest {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "production readiness targets another release")
		}
		if e = prod.ValidateCurrent(p.fresh(start)); e != nil {
			return PublicationResultV1{}, e
		}
	}
	support := assemblercontract.SupportReferenceOnlyV1
	if hasL {
		support = assemblercontract.SupportStandaloneV1
	}
	if hasP {
		support = assemblercontract.SupportProductionV1
	}
	release, e := buildPayload(r, support, optional(hasP, prod), true)
	if e != nil {
		return PublicationResultV1{}, e
	}
	if hasL {
		s, e := p.local.InspectMemoryKnowledgeLocalReadinessV1(ctx, r.ReleaseID, r.Revision)
		if e != nil {
			return PublicationResultV1{}, e
		}
		if s != l {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "local readiness drifted")
		}
		if e = s.ValidateCurrent(p.fresh(start)); e != nil {
			return PublicationResultV1{}, e
		}
	}
	if hasP {
		s, e := p.production.InspectMemoryKnowledgeProductionReadinessV1(ctx, r.ReleaseID, r.Revision)
		if e != nil {
			return PublicationResultV1{}, e
		}
		if s != prod {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "production readiness drifted")
		}
		if e = s.ValidateCurrent(p.fresh(start)); e != nil {
			return PublicationResultV1{}, e
		}
	}
	written, we := p.catalog.EnsureExactComponentReleaseV1(ctx, release)
	if we != nil {
		inspected, ie := p.catalog.InspectExactComponentReleaseV1(ctx, release.RefV1())
		if ie != nil {
			return PublicationResultV1{}, we
		}
		written = inspected
	}
	if written.ReleaseDigest != release.ReleaseDigest {
		return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "catalog returned another release")
	}
	result := PublicationResultV1{Release: written, LocalReady: hasL, ProductionReady: hasP}
	if hasL {
		x := l
		result.Local = &x
	}
	if hasP {
		x := prod
		result.Production = &x
	}
	return result, nil
}
func (p *PublisherV1) fresh(old time.Time) time.Time {
	n := p.clock()
	if n.IsZero() || n.Before(old) {
		return time.Time{}
	}
	return n
}
func optional(ok bool, p ProductionReadinessProjectionV1) *ProductionReadinessProjectionV1 {
	if !ok {
		return nil
	}
	return &p
}
func buildPayload(r PublicationRequestV1, support assemblercontract.SupportModeV1, prod *ProductionReadinessProjectionV1, requireCert bool) (assemblercontract.ComponentReleaseV1, error) {
	memoryReq := schema("memory-commit-request", mkcontract.VersionV1+"/MemoryCommitRequest")
	memoryRes := schema("memory-domain-result", mkcontract.VersionV1+"/MemoryDomainResult")
	knowledgeReq := schema("knowledge-commit-request", mkcontract.VersionV1+"/KnowledgeCommitRequest")
	knowledgeRes := schema("knowledge-domain-result", mkcontract.VersionV1+"/KnowledgeDomainResult")
	schemas := []runtimeports.SchemaRefV2{memoryReq, memoryRes, knowledgeReq, knowledgeRes}
	conf := runtimeports.ConformanceContainedObserveOnly
	res := runtimeports.ResidualPotentiallyStale
	if support == assemblercontract.SupportStandaloneV1 {
		conf = runtimeports.ConformanceRestrictedControlled
		res = runtimeports.ResidualInspectable
	}
	if support == assemblercontract.SupportProductionV1 {
		conf = runtimeports.ConformanceFullyControlled
		res = runtimeports.ResidualNone
	}
	owners := []runtimeports.OwnerAssignmentV2{{Role: runtimeports.OwnerEffect, OwnerComponentID: ComponentIDV1}, {Role: runtimeports.OwnerSettlement, OwnerComponentID: ComponentIDV1}, {Role: runtimeports.OwnerCleanup, OwnerComponentID: ComponentIDV1}}
	manifest := runtimeports.ComponentManifestV2{ContractVersion: runtimeports.BindingContractVersionV2, ComponentID: ComponentIDV1, Kind: ComponentKindV1, GovernanceCategory: "praxis/core", SemanticVersion: "1.0.0", ArtifactDigest: r.ArtifactDigest, Contract: runtimeports.ContractBindingV2{Name: "praxis.memory-knowledge/component-release", Version: "1.0.0", Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}, Schemas: schemas, Locality: runtimeports.LocalityExternalStatePlane, Dependencies: []runtimeports.ComponentDependencyV2{}, RequiredCapabilities: []runtimeports.CapabilityRequirementV2{}, ProvidedCapabilities: []runtimeports.ProvidedCapabilityV2{{Capability: MemoryCapabilityV1, TTLSeconds: 30, Schemas: []runtimeports.SchemaRefV2{memoryReq, memoryRes}}, {Capability: KnowledgeCapabilityV1, TTLSeconds: 30, Schemas: []runtimeports.SchemaRefV2{knowledgeReq, knowledgeRes}}}, Conformance: conf, ResidualClass: res, Owners: owners, Credentials: []runtimeports.CredentialRequirementV2{}, OfflinePolicy: runtimeports.OfflineDenied, Extensions: []runtimeports.GovernanceExtensionV2{}, Annotations: []runtimeports.DisplayAnnotationV2{}}
	md, e := manifest.BindingDigestV2()
	if e != nil {
		return assemblercontract.ComponentReleaseV1{}, e
	}
	if requireCert && prod != nil && (prod.ManifestDigest != md || prod.ArtifactDigest != r.ArtifactDigest) {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "production manifest drift")
	}
	module := assemblycontract.ModuleDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, ModuleID: "praxis.memory-knowledge/module", Namespace: "praxis.memory-knowledge", SemanticVersion: "1.0.0", ArtifactDigest: r.ArtifactDigest, PublisherRef: r.PublisherRef, SourceRef: r.SourceRef, ComponentManifestRef: assemblycontract.ObjectRefV1{ID: string(ComponentIDV1), Revision: r.Revision, Digest: md}, Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, Capabilities: []runtimeports.CapabilityNameV2{MemoryCapabilityV1, KnowledgeCapabilityV1}, Schemas: schemas, Locality: manifest.Locality, ResidualClass: res, Owners: owners, CredentialRequirements: []runtimeports.NamespacedNameV2{}}
	caps := []assemblycontract.CapabilityDescriptorV1{capability(MemoryCapabilityV1, memoryReq, memoryRes, conf), capability(KnowledgeCapabilityV1, knowledgeReq, knowledgeRes, conf)}
	ports := []assemblycontract.PortSpecV1{port("praxis.memory-knowledge/port/memory-owner", MemoryCapabilityV1, memoryReq, memoryRes, "praxis.memory/commit"), port("praxis.memory-knowledge/port/knowledge-owner", KnowledgeCapabilityV1, knowledgeReq, knowledgeRes, "praxis.knowledge/commit")}
	factories := []assemblycontract.ModuleFactoryDescriptorV1{factory("praxis.memory-knowledge/factory/memory-owner", module.ModuleID, r.ArtifactDigest, memoryReq, memoryRes, MemoryCapabilityV1, r.TrustRef), factory("praxis.memory-knowledge/factory/knowledge-owner", module.ModuleID, r.ArtifactDigest, knowledgeReq, knowledgeRes, KnowledgeCapabilityV1, r.TrustRef)}
	expires := r.ExpiresUnixNano
	evidence := []assemblycontract.ObjectRefV1{}
	if prod != nil {
		if prod.ExpiresUnixNano < expires {
			expires = prod.ExpiresUnixNano
		}
		evidence = prod.evidence()
	}
	release := assemblercontract.ComponentReleaseV1{ReleaseID: r.ReleaseID, Revision: r.Revision, SupportMode: support, ComponentManifest: manifest, ModuleDescriptors: []assemblycontract.ModuleDescriptorV1{module}, CapabilityDescriptors: caps, SlotSpecs: []assemblycontract.SlotSpecV1{}, SlotContributions: []assemblycontract.SlotContributionV1{}, PortSpecs: ports, HookFaces: []assemblycontract.HookFaceSpecV1{}, PhaseContributions: []assemblycontract.PhaseContributionV1{}, Dependencies: []assemblycontract.DependencySpecV1{}, FactoryDescriptors: factories, ProviderBindingCandidates: []assemblycontract.ProviderBindingCandidateV1{}, RequiredPlanArtifacts: []assemblercontract.PlanArtifactV1{}, SourceRef: r.SourceRef, ArtifactDigest: r.ArtifactDigest, EvidenceRefs: evidence, CreatedUnixNano: r.CreatedUnixNano, ExpiresUnixNano: expires}
	if prod != nil {
		release.CertificationRef = assemblycontract.ObjectRefV1{ID: r.CertificationID, Revision: r.Revision, Digest: runtimeports.EvidenceGenesisDigestV2}
		d, e := assemblercontract.ComponentReleaseCertificationDigestV1(release)
		if e != nil {
			return assemblercontract.ComponentReleaseV1{}, e
		}
		if requireCert && (prod.CertificationFactRef.ID != r.CertificationID || prod.CertificationFactRef.Revision != r.Revision || prod.CertificationFactRef.Digest != d) {
			return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "independent certification missing")
		}
		if requireCert {
			release.CertificationRef = prod.CertificationFactRef
		} else {
			release.CertificationRef.Digest = d
		}
	}
	return assemblercontract.SealComponentReleaseV1(release)
}
func schema(name, body string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.memory-knowledge", Name: name, Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte(body))}
}
func ref(id string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
}
func capability(id runtimeports.CapabilityNameV2, a, b runtimeports.SchemaRefV2, c runtimeports.ConformanceLevel) assemblycontract.CapabilityDescriptorV1 {
	return assemblycontract.CapabilityDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, Capability: id, Version: "1.0.0", Schemas: []runtimeports.SchemaRefV2{a, b}, Provided: true, TTLSeconds: 30, EffectClass: "governed-state-effect", OwnerCapability: id, Conformance: c}
}
func port(id string, owner runtimeports.CapabilityNameV2, a, b runtimeports.SchemaRefV2, effect runtimeports.NamespacedNameV2) assemblycontract.PortSpecV1 {
	return assemblycontract.PortSpecV1{ContractVersion: assemblycontract.ContractVersionV1, PortID: id, OwnerCapability: owner, RequestSchema: a, ResponseSchema: b, OperationClass: "owner-commit-or-inspect", EffectKind: effect, ConflictDomainRule: "tenant-owner-subject", Governance: assemblycontract.GovernanceRequirementsV1{ReviewRequired: true, FenceRequired: true, AuthorityRequired: true, ScopeRequired: true, BudgetRequired: true}, Idempotency: "same-attempt-commit-or-inspect", OperationScopeRef: &assemblycontract.OperationScopeRefV1{Ref: ref(id + "/scope"), ScopeKind: assemblycontract.RuntimeOperationScopeKindV1, ScopeDigest: core.DigestBytes([]byte(id + "/scope"))}, InspectContractRef: &assemblycontract.InspectContractRefV1{Ref: ref(id + "/inspect"), OwnerCapability: owner, RequestSchema: a, ObservationSchema: b}, DomainResultContractRef: &assemblycontract.DomainResultContractRefV1{Ref: ref(id + "/domain"), OwnerCapability: owner, Schema: b}, RuntimeOperationSettlementRefContract: &assemblycontract.RuntimeOperationSettlementRefContractV1{Ref: ref(id + "/settlement"), RuntimeOwnerCapability: assemblycontract.RuntimeOperationSettlementCapabilityV1, Schema: b}, ApplySettlementContractRef: &assemblycontract.ApplySettlementContractRefV1{Ref: ref(id + "/apply"), OwnerCapability: owner, RequestSchema: a, ResultSchema: b}, FailureSemantics: "unknown-outcome-inspect-only", Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}
}
func factory(id, module string, artifact core.Digest, a, b runtimeports.SchemaRefV2, out runtimeports.CapabilityNameV2, trust assemblycontract.ObjectRefV1) assemblycontract.ModuleFactoryDescriptorV1 {
	return assemblycontract.ModuleFactoryDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, FactoryID: id, ModuleRef: module, ArtifactDigest: artifact, ConstructionMode: assemblycontract.ConstructionTrustedInProcessGoV1, InputSchema: a, OutputCapability: out, Lifecycle: assemblycontract.LifecycleGenerationV1, CleanupContractRef: assemblycontract.CleanupContractRefV1{Ref: ref(id + "/cleanup"), OwnerCapability: out, RequestSchema: a, ResultSchema: b}, TrustRef: trust}
}
func nilLike(v any) bool {
	if v == nil {
		return true
	}
	x := reflect.ValueOf(v)
	switch x.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return x.IsNil()
	}
	return false
}
