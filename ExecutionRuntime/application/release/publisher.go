package release

import (
	"context"
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"reflect"
	"strings"
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
		return nil, invalid("publisher dependencies incomplete")
	}
	return &PublisherV1{l, p, c, clock}, nil
}
func (p *PublisherV1) Publish(ctx context.Context, r PublicationRequestV1) (PublicationResultV1, error) {
	if p == nil || ctx == nil {
		return PublicationResultV1{}, invalid("publisher or context nil")
	}
	start := p.clock()
	if e := r.Validate(start); e != nil {
		return PublicationResultV1{}, e
	}
	l, e := p.local.InspectApplicationLocalReadinessV1(ctx, r.ReleaseID, r.Revision)
	hasL := e == nil
	if e != nil && !core.HasCategory(e, core.ErrorNotFound) {
		return PublicationResultV1{}, e
	}
	if hasL {
		if l.ReleaseID != r.ReleaseID || l.Revision != r.Revision || l.ArtifactDigest != r.ArtifactDigest {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "local readiness target drift")
		}
		if e = l.ValidateCurrent(p.fresh(start)); e != nil {
			return PublicationResultV1{}, e
		}
	}
	prod, e := p.production.InspectApplicationProductionReadinessV1(ctx, r.ReleaseID, r.Revision)
	hasP := e == nil
	if e != nil && !core.HasCategory(e, core.ErrorNotFound) {
		return PublicationResultV1{}, e
	}
	if hasP && !hasL {
		return PublicationResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "production requires complete local closure")
	}
	if hasP {
		if prod.ReleaseID != r.ReleaseID || prod.Revision != r.Revision || prod.ArtifactDigest != r.ArtifactDigest {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "production readiness target drift")
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
		s, e := p.local.InspectApplicationLocalReadinessV1(ctx, r.ReleaseID, r.Revision)
		if e != nil {
			return PublicationResultV1{}, e
		}
		if s != l {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "local readiness S1/S2 drift")
		}
		if e = s.ValidateCurrent(p.fresh(start)); e != nil {
			return PublicationResultV1{}, e
		}
	}
	if hasP {
		s, e := p.production.InspectApplicationProductionReadinessV1(ctx, r.ReleaseID, r.Revision)
		if e != nil {
			return PublicationResultV1{}, e
		}
		if s != prod {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "production readiness S1/S2 drift")
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
		return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "catalog content drift")
	}
	out := PublicationResultV1{Release: written, LocalReady: hasL, ProductionReady: hasP}
	if hasL {
		x := l
		out.Local = &x
	}
	if hasP {
		x := prod
		out.Production = &x
	}
	return out, nil
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
	schemas := make([]runtimeports.SchemaRefV2, len(capabilities)*2)
	for i, c := range capabilities {
		schemas[i*2] = schema(string(c) + "-request")
		schemas[i*2+1] = schema(string(c) + "-result")
	}
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
	runtimeComponent := runtimeports.RuntimeSharedEngineComponentIDV1
	runtimeCapability := runtimeports.CapabilityNameV2("praxis.runtime/execution-governance")
	owners := []runtimeports.OwnerAssignmentV2{{Role: runtimeports.OwnerEffect, OwnerComponentID: runtimeComponent}, {Role: runtimeports.OwnerSettlement, OwnerComponentID: runtimeComponent}, {Role: runtimeports.OwnerCleanup, OwnerComponentID: ComponentIDV1}}
	provided := make([]runtimeports.ProvidedCapabilityV2, len(capabilities))
	descriptors := make([]assemblycontract.CapabilityDescriptorV1, len(capabilities))
	ports := make([]assemblycontract.PortSpecV1, len(capabilities))
	factories := make([]assemblycontract.ModuleFactoryDescriptorV1, len(capabilities))
	for i, c := range capabilities {
		pair := schemas[i*2 : i*2+2]
		provided[i] = runtimeports.ProvidedCapabilityV2{Capability: c, TTLSeconds: 30, Schemas: pair}
		descriptors[i] = assemblycontract.CapabilityDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, Capability: c, Version: "1.0.0", Schemas: pair, Provided: true, TTLSeconds: 30, EffectClass: "governed-application-coordination", OwnerCapability: c, Conformance: conf}
		id := "praxis.application/port/" + string(c)
		ports[i] = port(id, c, pair[0], pair[1])
		factories[i] = factory("praxis.application/factory/"+string(c), r.ArtifactDigest, pair[0], pair[1], c, r.TrustRef)
	}
	descriptors = append(descriptors, assemblycontract.CapabilityDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, Capability: runtimeCapability, Version: "1.0.0", Schemas: append([]runtimeports.SchemaRefV2{}, schemas[4:6]...), Required: true, TTLSeconds: 30, EffectClass: "runtime-governance-required", OwnerCapability: runtimeCapability, Conformance: conf})
	manifest := runtimeports.ComponentManifestV2{ContractVersion: runtimeports.BindingContractVersionV2, ComponentID: ComponentIDV1, Kind: ComponentKindV1, GovernanceCategory: "praxis/core", SemanticVersion: "1.0.0", ArtifactDigest: r.ArtifactDigest, Contract: runtimeports.ContractBindingV2{Name: "praxis.application/shared-engine", Version: "1.0.0", Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}, Schemas: schemas, Locality: runtimeports.LocalityHostControlPlane, Dependencies: []runtimeports.ComponentDependencyV2{{ComponentID: runtimeComponent, Optional: false}}, RequiredCapabilities: []runtimeports.CapabilityRequirementV2{{Capability: runtimeCapability, ProviderComponent: runtimeComponent, Optional: false}}, ProvidedCapabilities: provided, Conformance: conf, ResidualClass: res, Owners: owners, Credentials: []runtimeports.CredentialRequirementV2{}, OfflinePolicy: runtimeports.OfflineDenied, Extensions: []runtimeports.GovernanceExtensionV2{}, Annotations: []runtimeports.DisplayAnnotationV2{}}
	md, e := manifest.BindingDigestV2()
	if e != nil {
		return assemblercontract.ComponentReleaseV1{}, e
	}
	if requireCert && prod != nil && (prod.ManifestDigest != md || prod.ArtifactDigest != r.ArtifactDigest) {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "production manifest drift")
	}
	module := assemblycontract.ModuleDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, ModuleID: "praxis.application/module/shared-engine", Namespace: "praxis.application", SemanticVersion: "1.0.0", ArtifactDigest: r.ArtifactDigest, PublisherRef: r.PublisherRef, SourceRef: r.SourceRef, ComponentManifestRef: assemblycontract.ObjectRefV1{ID: string(ComponentIDV1), Revision: r.Revision, Digest: md}, Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, Capabilities: append([]runtimeports.CapabilityNameV2{}, capabilities...), Schemas: schemas, Locality: manifest.Locality, ResidualClass: res, Owners: owners, CredentialRequirements: []runtimeports.NamespacedNameV2{}}
	expires := r.ExpiresUnixNano
	evidence := []assemblycontract.ObjectRefV1{}
	if prod != nil {
		if prod.ExpiresUnixNano < expires {
			expires = prod.ExpiresUnixNano
		}
		evidence = prod.evidence()
	}
	release := assemblercontract.ComponentReleaseV1{ReleaseID: r.ReleaseID, Revision: r.Revision, SupportMode: support, ComponentManifest: manifest, ModuleDescriptors: []assemblycontract.ModuleDescriptorV1{module}, CapabilityDescriptors: descriptors, SlotSpecs: []assemblycontract.SlotSpecV1{}, SlotContributions: []assemblycontract.SlotContributionV1{}, PortSpecs: ports, HookFaces: []assemblycontract.HookFaceSpecV1{}, PhaseContributions: []assemblycontract.PhaseContributionV1{}, Dependencies: []assemblycontract.DependencySpecV1{{ContractVersion: assemblycontract.ContractVersionV1, FromRef: module.ModuleID, ToRef: string(runtimeComponent), Relation: "requires", Required: true, VersionRange: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, Capability: runtimeCapability, FailureMode: "fail-closed"}}, FactoryDescriptors: factories, ProviderBindingCandidates: []assemblycontract.ProviderBindingCandidateV1{}, RequiredPlanArtifacts: []assemblercontract.PlanArtifactV1{}, SourceRef: r.SourceRef, ArtifactDigest: r.ArtifactDigest, EvidenceRefs: evidence, CreatedUnixNano: r.CreatedUnixNano, ExpiresUnixNano: expires}
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
func schema(name string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.application", Name: strings.NewReplacer("/", "-", ".", "-").Replace(name), Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte(name))}
}
func ref(id string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
}
func port(id string, owner runtimeports.CapabilityNameV2, a, b runtimeports.SchemaRefV2) assemblycontract.PortSpecV1 {
	return assemblycontract.PortSpecV1{ContractVersion: assemblycontract.ContractVersionV1, PortID: id, OwnerCapability: owner, RequestSchema: a, ResponseSchema: b, OperationClass: "coordination-command", Idempotency: "same-command-owner-inspect", FailureSemantics: "fail-closed", Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}
}
func factory(id string, artifact core.Digest, a, b runtimeports.SchemaRefV2, out runtimeports.CapabilityNameV2, trust assemblycontract.ObjectRefV1) assemblycontract.ModuleFactoryDescriptorV1 {
	return assemblycontract.ModuleFactoryDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, FactoryID: id, ModuleRef: "praxis.application/module/shared-engine", ArtifactDigest: artifact, ConstructionMode: assemblycontract.ConstructionTrustedInProcessGoV1, InputSchema: a, OutputCapability: out, Lifecycle: assemblycontract.LifecycleGenerationV1, CleanupContractRef: assemblycontract.CleanupContractRefV1{Ref: ref(id + "/cleanup"), OwnerCapability: out, RequestSchema: a, ResultSchema: b}, TrustRef: trust}
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
