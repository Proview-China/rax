package release

import (
	"context"
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"reflect"
	"time"
)

const releaseVersionV1 = "1.0.0"

type PublisherV1 struct {
	local      LocalReadinessReaderV1
	production ProductionReadinessReaderV1
	catalog    ComponentReleaseCatalogPortV1
	clock      func() time.Time
}

func NewPublisherV1(local LocalReadinessReaderV1, production ProductionReadinessReaderV1, catalog ComponentReleaseCatalogPortV1, clock func() time.Time) (*PublisherV1, error) {
	if nilLike(local) || nilLike(production) || nilLike(catalog) || nilLike(clock) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "tool/MCP release publisher dependencies are incomplete")
	}
	return &PublisherV1{local: local, production: production, catalog: catalog, clock: clock}, nil
}

func (p *PublisherV1) Publish(ctx context.Context, request PublicationRequestV1) (PublicationResultV1, error) {
	if p == nil || ctx == nil {
		return PublicationResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "tool/MCP publisher or context is nil")
	}
	start := p.clock()
	if e := request.Validate(start); e != nil {
		return PublicationResultV1{}, e
	}
	local, e := p.local.InspectToolMCPLocalReadinessV1(ctx, request.ReleaseID, request.Revision)
	hasLocal := e == nil
	if e != nil && !core.HasCategory(e, core.ErrorNotFound) {
		return PublicationResultV1{}, e
	}
	if hasLocal {
		if local.ReleaseID != request.ReleaseID || local.Revision != request.Revision || local.ArtifactDigest != request.ArtifactDigest {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "tool/MCP local readiness targets another release")
		}
		if e = local.ValidateCurrent(p.fresh(start)); e != nil {
			return PublicationResultV1{}, e
		}
	}
	production, e := p.production.InspectToolMCPProductionReadinessV1(ctx, request.ReleaseID, request.Revision)
	hasProduction := e == nil
	if e != nil && !core.HasCategory(e, core.ErrorNotFound) {
		return PublicationResultV1{}, e
	}
	if hasProduction && !hasLocal {
		return PublicationResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "production readiness cannot replace owner-local construction closure")
	}
	if hasProduction {
		if production.ReleaseID != request.ReleaseID || production.Revision != request.Revision || production.ArtifactDigest != request.ArtifactDigest {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "tool/MCP production readiness targets another release")
		}
		if e = production.ValidateCurrent(p.fresh(start)); e != nil {
			return PublicationResultV1{}, e
		}
	}
	release, e := buildReleasePayloadV1(request, mode(hasLocal, hasProduction), optionalProduction(hasProduction, production), true)
	if e != nil {
		return PublicationResultV1{}, e
	}
	if hasLocal {
		second, er := p.local.InspectToolMCPLocalReadinessV1(ctx, request.ReleaseID, request.Revision)
		if er != nil || second != local {
			if er != nil {
				return PublicationResultV1{}, er
			}
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "tool/MCP local readiness drifted before publication")
		}
		if er = second.ValidateCurrent(p.fresh(start)); er != nil {
			return PublicationResultV1{}, er
		}
	}
	if hasProduction {
		second, er := p.production.InspectToolMCPProductionReadinessV1(ctx, request.ReleaseID, request.Revision)
		if er != nil || second != production {
			if er != nil {
				return PublicationResultV1{}, er
			}
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "tool/MCP production readiness drifted before publication")
		}
		if er = second.ValidateCurrent(p.fresh(start)); er != nil {
			return PublicationResultV1{}, er
		}
	}
	written, er := p.catalog.EnsureExactComponentReleaseV1(ctx, release)
	if er != nil {
		inspected, ie := p.catalog.InspectExactComponentReleaseV1(ctx, release.RefV1())
		if ie != nil {
			return PublicationResultV1{}, er
		}
		written = inspected
	}
	if written.ReleaseDigest != release.ReleaseDigest {
		return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "tool/MCP catalog returned different release content")
	}
	done := p.fresh(start)
	if !done.Before(time.Unix(0, written.ExpiresUnixNano)) {
		return PublicationResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "tool/MCP release expired during publication")
	}
	result := PublicationResultV1{Release: written, LocalReady: hasLocal, ProductionReady: hasProduction}
	if hasLocal {
		v := local
		result.LocalReadiness = &v
	}
	if hasProduction {
		v := production
		result.ProductionReadiness = &v
	}
	return result, nil
}
func (p *PublisherV1) fresh(previous time.Time) time.Time {
	n := p.clock()
	if n.IsZero() || n.Before(previous) {
		return time.Time{}
	}
	return n
}
func mode(local, production bool) assemblercontract.SupportModeV1 {
	if production {
		return assemblercontract.SupportProductionV1
	}
	if local {
		return assemblercontract.SupportStandaloneV1
	}
	return assemblercontract.SupportReferenceOnlyV1
}
func optionalProduction(ok bool, p ProductionReadinessProjectionV1) *ProductionReadinessProjectionV1 {
	if !ok {
		return nil
	}
	return &p
}

func buildReleaseV1(request PublicationRequestV1, support assemblercontract.SupportModeV1, readiness *ProductionReadinessProjectionV1) (assemblercontract.ComponentReleaseV1, error) {
	return buildReleasePayloadV1(request, support, readiness, true)
}
func buildReleasePayloadV1(request PublicationRequestV1, support assemblercontract.SupportModeV1, readiness *ProductionReadinessProjectionV1, requireCertification bool) (assemblercontract.ComponentReleaseV1, error) {
	actionRequest := schema("single-call-action-request-v2", "praxis.application/single-call-tool-action-request/v2")
	actionResult := schema("tool-result-v2", toolcontract.ResultContractVersionV2+"/ToolResultV2")
	mcpRequest := schema("mcp-execution-command-v1", toolcontract.MCPExecutionCommandContractVersionV1+"/MCPExecutionCommandFactV1")
	mcpResult := schema("mcp-protocol-receipt-v1", toolcontract.MCPProtocolReceiptContractVersionV1+"/MCPProtocolReceiptV1")
	schemas := []runtimeports.SchemaRefV2{actionRequest, actionResult, mcpRequest, mcpResult}
	conf := runtimeports.ConformanceContainedObserveOnly
	residual := runtimeports.ResidualPotentiallyStale
	if support == assemblercontract.SupportStandaloneV1 {
		conf = runtimeports.ConformanceRestrictedControlled
		residual = runtimeports.ResidualInspectable
	}
	if support == assemblercontract.SupportProductionV1 {
		conf = runtimeports.ConformanceFullyControlled
		residual = runtimeports.ResidualNone
	}
	owners := []runtimeports.OwnerAssignmentV2{{Role: runtimeports.OwnerEffect, OwnerComponentID: ComponentIDV1}, {Role: runtimeports.OwnerSettlement, OwnerComponentID: ComponentIDV1}, {Role: runtimeports.OwnerCleanup, OwnerComponentID: ComponentIDV1}}
	credential := runtimeports.CredentialRequirementV2{CredentialClass: "praxis.tool-mcp/provider-credential", ScopeDigest: core.DigestBytes([]byte("praxis.tool-mcp/provider-credential-scope/v1")), MaximumTTLSeconds: 30}
	manifest := runtimeports.ComponentManifestV2{ContractVersion: runtimeports.BindingContractVersionV2, ComponentID: ComponentIDV1, Kind: ComponentKindV1, GovernanceCategory: "praxis/core", SemanticVersion: releaseVersionV1, ArtifactDigest: request.ArtifactDigest, Contract: runtimeports.ContractBindingV2{Name: "praxis.tool-mcp/component-release", Version: releaseVersionV1, Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}, Schemas: schemas, Locality: runtimeports.LocalityInstanceDataPlane, Dependencies: []runtimeports.ComponentDependencyV2{}, RequiredCapabilities: []runtimeports.CapabilityRequirementV2{}, ProvidedCapabilities: []runtimeports.ProvidedCapabilityV2{{Capability: ToolActionCapabilityV1, TTLSeconds: 30, Schemas: []runtimeports.SchemaRefV2{actionRequest, actionResult}}, {Capability: MCPCallCapabilityV1, TTLSeconds: 30, Schemas: []runtimeports.SchemaRefV2{mcpRequest, mcpResult}}}, Conformance: conf, ResidualClass: residual, Owners: owners, Credentials: []runtimeports.CredentialRequirementV2{credential}, OfflinePolicy: runtimeports.OfflineDenied, Extensions: []runtimeports.GovernanceExtensionV2{}, Annotations: []runtimeports.DisplayAnnotationV2{}}
	manifestDigest, e := manifest.BindingDigestV2()
	if e != nil {
		return assemblercontract.ComponentReleaseV1{}, e
	}
	if readiness != nil && requireCertification && (readiness.ManifestDigest != manifestDigest || readiness.ArtifactDigest != request.ArtifactDigest) {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "tool/MCP production readiness certifies another manifest")
	}
	module := assemblycontract.ModuleDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, ModuleID: "praxis.tool-mcp/module", Namespace: "praxis.tool-mcp", SemanticVersion: releaseVersionV1, ArtifactDigest: request.ArtifactDigest, PublisherRef: request.PublisherRef, SourceRef: request.SourceRef, ComponentManifestRef: assemblycontract.ObjectRefV1{ID: string(ComponentIDV1), Revision: request.Revision, Digest: manifestDigest}, Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, Capabilities: []runtimeports.CapabilityNameV2{ToolActionCapabilityV1, MCPCallCapabilityV1}, Schemas: schemas, Locality: manifest.Locality, ResidualClass: residual, Owners: owners, CredentialRequirements: []runtimeports.NamespacedNameV2{credential.CredentialClass}}
	caps := []assemblycontract.CapabilityDescriptorV1{{ContractVersion: assemblycontract.ContractVersionV1, Capability: ToolActionCapabilityV1, Version: releaseVersionV1, Schemas: []runtimeports.SchemaRefV2{actionRequest, actionResult}, Provided: true, TTLSeconds: 30, EffectClass: "governed-external-effect", OwnerCapability: ToolActionCapabilityV1, Conformance: conf}, {ContractVersion: assemblycontract.ContractVersionV1, Capability: MCPCallCapabilityV1, Version: releaseVersionV1, Schemas: []runtimeports.SchemaRefV2{mcpRequest, mcpResult}, Provided: true, TTLSeconds: 30, EffectClass: "governed-remote-protocol-effect", OwnerCapability: MCPCallCapabilityV1, Conformance: conf}}
	ports := []assemblycontract.PortSpecV1{port("praxis.tool-mcp/port/single-call-action-v2", ToolActionCapabilityV1, actionRequest, actionResult, "praxis.tool/action"), port("praxis.tool-mcp/port/mcp-tools-call-v1", MCPCallCapabilityV1, mcpRequest, mcpResult, "praxis.mcp/tools-call")}
	factories := []assemblycontract.ModuleFactoryDescriptorV1{factory("praxis.tool-mcp/factory/single-call-action-v2", module.ModuleID, request.ArtifactDigest, actionRequest, ToolActionCapabilityV1, actionResult, request.TrustRef), factory("praxis.tool-mcp/factory/mcp-tools-call-v1", module.ModuleID, request.ArtifactDigest, mcpRequest, MCPCallCapabilityV1, mcpResult, request.TrustRef)}
	expires := request.ExpiresUnixNano
	evidence := []assemblycontract.ObjectRefV1{}
	if readiness != nil {
		if readiness.ExpiresUnixNano < expires {
			expires = readiness.ExpiresUnixNano
		}
		evidence = append(evidence, readiness.evidenceRefs()...)
	}
	release := assemblercontract.ComponentReleaseV1{ReleaseID: request.ReleaseID, Revision: request.Revision, SupportMode: support, ComponentManifest: manifest, ModuleDescriptors: []assemblycontract.ModuleDescriptorV1{module}, CapabilityDescriptors: caps, SlotSpecs: []assemblycontract.SlotSpecV1{}, SlotContributions: []assemblycontract.SlotContributionV1{}, PortSpecs: ports, HookFaces: []assemblycontract.HookFaceSpecV1{}, PhaseContributions: []assemblycontract.PhaseContributionV1{}, Dependencies: []assemblycontract.DependencySpecV1{}, FactoryDescriptors: factories, ProviderBindingCandidates: []assemblycontract.ProviderBindingCandidateV1{}, RequiredPlanArtifacts: []assemblercontract.PlanArtifactV1{}, SourceRef: request.SourceRef, ArtifactDigest: request.ArtifactDigest, EvidenceRefs: evidence, CreatedUnixNano: request.CreatedUnixNano, ExpiresUnixNano: expires}
	if support == assemblercontract.SupportProductionV1 {
		release.CertificationRef = assemblycontract.ObjectRefV1{ID: request.CertificationID, Revision: request.Revision, Digest: runtimeports.EvidenceGenesisDigestV2}
		d, er := assemblercontract.ComponentReleaseCertificationDigestV1(release)
		if er != nil {
			return assemblercontract.ComponentReleaseV1{}, er
		}
		if requireCertification && (readiness.CertificationFactRef.ID != request.CertificationID || readiness.CertificationFactRef.Revision != request.Revision || readiness.CertificationFactRef.Digest != d) {
			return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "tool/MCP release lacks independent exact certification")
		}
		if requireCertification {
			release.CertificationRef = readiness.CertificationFactRef
		} else {
			release.CertificationRef.Digest = d
		}
	}
	return assemblercontract.SealComponentReleaseV1(release)
}
func schema(name, canonical string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.tool-mcp", Name: name, Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte(canonical))}
}
func ref(id string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
}
func port(id string, owner runtimeports.CapabilityNameV2, req, res runtimeports.SchemaRefV2, effect runtimeports.NamespacedNameV2) assemblycontract.PortSpecV1 {
	return assemblycontract.PortSpecV1{ContractVersion: assemblycontract.ContractVersionV1, PortID: id, OwnerCapability: owner, RequestSchema: req, ResponseSchema: res, OperationClass: "start-or-inspect", EffectKind: effect, ConflictDomainRule: "tenant-run-action-effect", Governance: assemblycontract.GovernanceRequirementsV1{ReviewRequired: true, FenceRequired: true, AuthorityRequired: true, ScopeRequired: true, BudgetRequired: true}, Idempotency: "same-command-start-or-inspect", CancelSupported: false, OperationScopeRef: &assemblycontract.OperationScopeRefV1{Ref: ref(id + "/scope"), ScopeKind: assemblycontract.RuntimeOperationScopeKindV1, ScopeDigest: core.DigestBytes([]byte(id + "/scope"))}, InspectContractRef: &assemblycontract.InspectContractRefV1{Ref: ref(id + "/inspect"), OwnerCapability: owner, RequestSchema: req, ObservationSchema: res}, DomainResultContractRef: &assemblycontract.DomainResultContractRefV1{Ref: ref(id + "/domain-result"), OwnerCapability: owner, Schema: res}, RuntimeOperationSettlementRefContract: &assemblycontract.RuntimeOperationSettlementRefContractV1{Ref: ref(id + "/settlement"), RuntimeOwnerCapability: assemblycontract.RuntimeOperationSettlementCapabilityV1, Schema: res}, ApplySettlementContractRef: &assemblycontract.ApplySettlementContractRefV1{Ref: ref(id + "/apply"), OwnerCapability: owner, RequestSchema: req, ResultSchema: res}, FailureSemantics: "unknown-outcome-inspect-only", Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}
}
func factory(id, module string, artifact core.Digest, input runtimeports.SchemaRefV2, output runtimeports.CapabilityNameV2, result runtimeports.SchemaRefV2, trust assemblycontract.ObjectRefV1) assemblycontract.ModuleFactoryDescriptorV1 {
	return assemblycontract.ModuleFactoryDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, FactoryID: id, ModuleRef: module, ArtifactDigest: artifact, ConstructionMode: assemblycontract.ConstructionTrustedInProcessGoV1, InputSchema: input, OutputCapability: output, Lifecycle: assemblycontract.LifecycleRunV1, CleanupContractRef: assemblycontract.CleanupContractRefV1{Ref: ref(id + "/cleanup"), OwnerCapability: output, RequestSchema: input, ResultSchema: result}, TrustRef: trust}
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
