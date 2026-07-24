package release

import (
	"context"
	"reflect"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	releaseSemanticVersionV1  = "1.0.0"
	releaseContractNameV1     = "praxis.sandbox/component-release"
	moduleIDV1                = "praxis.sandbox/module/lifecycle-v4"
	factoryIDV1               = "praxis.sandbox/factory/lifecycle-v4"
	portIDV1                  = "praxis.sandbox/port/lifecycle-v4"
	cleanupContractIDV1       = "praxis.sandbox/cleanup/lifecycle-v4"
	executionFactoryIDV1      = "praxis.sandbox/factory/execution-v1"
	executionPortIDV1         = "praxis.sandbox/port/execution-v1"
	executionCleanupIDV1      = "praxis.sandbox/cleanup/execution-v1"
	executionContributionIDV1 = "praxis.sandbox/contribution/execution-owner-v1"
)

type PublisherV1 struct {
	readiness ProductionReadinessReaderV1
	catalog   ComponentReleaseCatalogPortV1
	now       func() time.Time
}

func NewPublisherV1(readiness ProductionReadinessReaderV1, catalog ComponentReleaseCatalogPortV1, now func() time.Time) (*PublisherV1, error) {
	if nilLike(readiness) || nilLike(catalog) || nilLike(now) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "sandbox release publisher requires readiness, catalog, and clock ports")
	}
	return &PublisherV1{readiness: readiness, catalog: catalog, now: now}, nil
}

func (p *PublisherV1) Publish(ctx context.Context, request PublicationRequestV1) (PublicationResultV1, error) {
	if p == nil || ctx == nil {
		return PublicationResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "sandbox release publisher or context is nil")
	}
	now := p.now()
	if err := request.Validate(now); err != nil {
		return PublicationResultV1{}, err
	}

	var readiness *SandboxProductionReadinessProjectionV1
	projection, err := p.readiness.InspectSandboxProductionReadinessV1(ctx, request.ReleaseID, request.Revision)
	if err == nil {
		if projection.ReleaseID != request.ReleaseID || projection.Revision != request.Revision {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "sandbox readiness projection targets another release")
		}
		fresh := p.now()
		if fresh.Before(now) {
			return PublicationResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "sandbox release clock regressed during readiness inspection")
		}
		if err := projection.ValidateCurrent(fresh); err != nil {
			return PublicationResultV1{}, err
		}
		readiness = &projection
	} else if !isKnownAbsent(err) {
		return PublicationResultV1{}, err
	}

	release, err := buildReleaseV1(request, readiness)
	if err != nil {
		return PublicationResultV1{}, err
	}
	if readiness != nil {
		second, inspectErr := p.readiness.InspectSandboxProductionReadinessV1(ctx, request.ReleaseID, request.Revision)
		if inspectErr != nil {
			return PublicationResultV1{}, inspectErr
		}
		fresh := p.now()
		if fresh.Before(now) {
			return PublicationResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "sandbox release clock regressed before publication")
		}
		if err := second.ValidateCurrent(fresh); err != nil {
			return PublicationResultV1{}, err
		}
		if second != *readiness {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "sandbox production readiness drifted before publication")
		}
	}
	written, writeErr := p.catalog.EnsureExactComponentReleaseV1(ctx, release)
	if writeErr != nil {
		inspected, inspectErr := p.catalog.InspectExactComponentReleaseV1(ctx, release.RefV1())
		if inspectErr != nil {
			return PublicationResultV1{}, writeErr
		}
		if inspected.ReleaseDigest != release.ReleaseDigest {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "sandbox release catalog recovered different content")
		}
		written = inspected
	}
	if written.ReleaseDigest != release.ReleaseDigest {
		return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "sandbox release catalog returned different content")
	}
	completed := p.now()
	if completed.Before(now) {
		return PublicationResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "sandbox release clock regressed after catalog publication")
	}
	if !completed.Before(time.Unix(0, written.ExpiresUnixNano)) {
		return PublicationResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "sandbox release expired during publication")
	}
	return PublicationResultV1{Release: written, ProductionReady: readiness != nil, Readiness: cloneReadiness(readiness)}, nil
}

func buildReleaseV1(request PublicationRequestV1, readiness *SandboxProductionReadinessProjectionV1) (assemblercontract.ComponentReleaseV1, error) {
	return buildReleasePayloadV1(request, readiness, true)
}

func buildReleasePayloadV1(request PublicationRequestV1, readiness *SandboxProductionReadinessProjectionV1, requireIndependentCertification bool) (assemblercontract.ComponentReleaseV1, error) {
	requestSchema := schemaRefV1("lifecycle-request-v4", contract.SandboxLifecycleContractVersionV4+"/SandboxLifecycleRequestV4")
	resultSchema := schemaRefV1("lifecycle-result-v4", contract.SandboxLifecycleContractVersionV4+"/SandboxLifecycleResultV4")
	schemas := []runtimeports.SchemaRefV2{requestSchema, resultSchema}
	owners := []runtimeports.OwnerAssignmentV2{
		{Role: runtimeports.OwnerEffect, OwnerComponentID: ComponentIDV1},
		{Role: runtimeports.OwnerSettlement, OwnerComponentID: ComponentIDV1},
		{Role: runtimeports.OwnerCleanup, OwnerComponentID: ComponentIDV1},
	}
	support := assemblercontract.SupportStandaloneV1
	conformance := runtimeports.ConformanceRestrictedControlled
	residual := runtimeports.ResidualInspectable
	expires := request.ExpiresUnixNano
	evidence := []assemblycontract.ObjectRefV1{}
	if readiness != nil {
		support = assemblercontract.SupportProductionV1
		conformance = runtimeports.ConformanceFullyControlled
		residual = runtimeports.ResidualNone
		if readiness.ExpiresUnixNano < expires {
			expires = readiness.ExpiresUnixNano
		}
		evidence = append(evidence, readiness.evidenceReferences()...)
	}
	manifest := runtimeports.ComponentManifestV2{
		ContractVersion: runtimeports.BindingContractVersionV2,
		ComponentID:     ComponentIDV1, Kind: ComponentKindV1, GovernanceCategory: "praxis/core",
		SemanticVersion: releaseSemanticVersionV1, ArtifactDigest: request.ArtifactDigest,
		Contract: runtimeports.ContractBindingV2{Name: releaseContractNameV1, Version: releaseSemanticVersionV1, Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}},
		Schemas:  schemas, Locality: runtimeports.LocalityInstanceDataPlane,
		Dependencies: []runtimeports.ComponentDependencyV2{}, RequiredCapabilities: []runtimeports.CapabilityRequirementV2{},
		ProvidedCapabilities: []runtimeports.ProvidedCapabilityV2{
			{Capability: LifecycleCapabilityV1, TTLSeconds: 30, Schemas: schemas},
			{Capability: ExecutionCapabilityV1, TTLSeconds: 30, Schemas: schemas},
		},
		Conformance: conformance, ResidualClass: residual, Owners: owners,
		Credentials: []runtimeports.CredentialRequirementV2{}, OfflinePolicy: runtimeports.OfflineDenied,
		Extensions: []runtimeports.GovernanceExtensionV2{}, Annotations: []runtimeports.DisplayAnnotationV2{},
	}
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		return assemblercontract.ComponentReleaseV1{}, err
	}
	if readiness != nil && (readiness.ArtifactDigest != request.ArtifactDigest || readiness.ManifestDigest != manifestDigest) {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "sandbox readiness certifies another artifact or manifest")
	}
	module := assemblycontract.ModuleDescriptorV1{
		ContractVersion: assemblycontract.ContractVersionV1, ModuleID: moduleIDV1, Namespace: "praxis.sandbox",
		SemanticVersion: releaseSemanticVersionV1, ArtifactDigest: request.ArtifactDigest,
		PublisherRef: request.PublisherRef, SourceRef: request.SourceRef,
		ComponentManifestRef: assemblycontract.ObjectRefV1{ID: string(ComponentIDV1), Revision: request.Revision, Digest: manifestDigest},
		Compatibility:        assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"},
		Capabilities:         []runtimeports.CapabilityNameV2{ExecutionCapabilityV1, LifecycleCapabilityV1}, Schemas: schemas,
		Locality: manifest.Locality, ResidualClass: residual, Owners: owners,
		CredentialRequirements: []runtimeports.NamespacedNameV2{},
	}
	capability := assemblycontract.CapabilityDescriptorV1{
		ContractVersion: assemblycontract.ContractVersionV1, Capability: LifecycleCapabilityV1,
		Version: releaseSemanticVersionV1, Schemas: schemas, Provided: true, TTLSeconds: 30,
		EffectClass: "governed-external-effect", OwnerCapability: LifecycleOwnerV1, Conformance: conformance,
	}
	executionCapability := assemblycontract.CapabilityDescriptorV1{
		ContractVersion: assemblycontract.ContractVersionV1, Capability: ExecutionCapabilityV1,
		Version: releaseSemanticVersionV1, Schemas: schemas, Provided: true, TTLSeconds: 30,
		EffectClass: "governed-external-effect", OwnerCapability: ExecutionOwnerV1, Conformance: conformance,
	}
	port := assemblycontract.PortSpecV1{
		ContractVersion: assemblycontract.ContractVersionV1, PortID: portIDV1, OwnerCapability: LifecycleOwnerV1,
		RequestSchema: requestSchema, ResponseSchema: resultSchema, OperationClass: "sandbox-lifecycle-start-or-inspect",
		EffectKind: "praxis.sandbox/environment-lifecycle", ConflictDomainRule: "tenant-instance-effect-kind",
		Governance:  assemblycontract.GovernanceRequirementsV1{ReviewRequired: true, FenceRequired: true, AuthorityRequired: true, ScopeRequired: true, BudgetRequired: true},
		Idempotency: "same-request-start-or-inspect", CancelSupported: false,
		OperationScopeRef:                     &assemblycontract.OperationScopeRefV1{Ref: contractRefV1("operation-scope"), ScopeKind: assemblycontract.RuntimeOperationScopeKindV1, ScopeDigest: core.DigestBytes([]byte("praxis.sandbox/environment-lifecycle-scope/v1"))},
		InspectContractRef:                    &assemblycontract.InspectContractRefV1{Ref: contractRefV1("inspect"), OwnerCapability: LifecycleOwnerV1, RequestSchema: requestSchema, ObservationSchema: resultSchema},
		DomainResultContractRef:               &assemblycontract.DomainResultContractRefV1{Ref: contractRefV1("domain-result"), OwnerCapability: LifecycleOwnerV1, Schema: resultSchema},
		RuntimeOperationSettlementRefContract: &assemblycontract.RuntimeOperationSettlementRefContractV1{Ref: contractRefV1("runtime-settlement"), RuntimeOwnerCapability: assemblycontract.RuntimeOperationSettlementCapabilityV1, Schema: resultSchema},
		ApplySettlementContractRef:            &assemblycontract.ApplySettlementContractRefV1{Ref: contractRefV1("apply-settlement"), OwnerCapability: LifecycleOwnerV1, RequestSchema: requestSchema, ResultSchema: resultSchema},
		FailureSemantics:                      "unknown-outcome-inspect-only", Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"},
	}
	executionPort := port
	executionPort.PortID = executionPortIDV1
	executionPort.OwnerCapability = ExecutionOwnerV1
	executionPort.OperationClass = "sandbox-execution-start-or-inspect"
	executionPort.EffectKind = "praxis.sandbox/environment-execution"
	executionPort.ConflictDomainRule = "tenant-instance-sandbox-execution"
	executionPort.InspectContractRef = &assemblycontract.InspectContractRefV1{Ref: contractRefV1("execution-inspect"), OwnerCapability: ExecutionOwnerV1, RequestSchema: requestSchema, ObservationSchema: resultSchema}
	executionPort.DomainResultContractRef = &assemblycontract.DomainResultContractRefV1{Ref: contractRefV1("execution-domain-result"), OwnerCapability: ExecutionOwnerV1, Schema: resultSchema}
	executionPort.ApplySettlementContractRef = &assemblycontract.ApplySettlementContractRefV1{Ref: contractRefV1("execution-apply-settlement"), OwnerCapability: ExecutionOwnerV1, RequestSchema: requestSchema, ResultSchema: resultSchema}
	factory := assemblycontract.ModuleFactoryDescriptorV1{
		ContractVersion: assemblycontract.ContractVersionV1, FactoryID: factoryIDV1, ModuleRef: moduleIDV1,
		ArtifactDigest: request.ArtifactDigest, ConstructionMode: assemblycontract.ConstructionTrustedInProcessGoV1,
		InputSchema: requestSchema, OutputCapability: LifecycleCapabilityV1, Lifecycle: assemblycontract.LifecycleInstanceV1,
		CleanupContractRef: assemblycontract.CleanupContractRefV1{
			Ref:             assemblycontract.ObjectRefV1{ID: cleanupContractIDV1, Revision: 1, Digest: core.DigestBytes([]byte(cleanupContractIDV1 + "/v1"))},
			OwnerCapability: LifecycleOwnerV1, RequestSchema: requestSchema, ResultSchema: resultSchema,
		},
		TrustRef: request.TrustRef,
	}
	executionFactory := factory
	executionFactory.FactoryID = executionFactoryIDV1
	executionFactory.OutputCapability = ExecutionCapabilityV1
	executionFactory.CleanupContractRef = assemblycontract.CleanupContractRefV1{
		Ref:             assemblycontract.ObjectRefV1{ID: executionCleanupIDV1, Revision: 1, Digest: core.DigestBytes([]byte(executionCleanupIDV1 + "/v1"))},
		OwnerCapability: ExecutionOwnerV1, RequestSchema: requestSchema, ResultSchema: resultSchema,
	}
	executionContribution := assemblycontract.SlotContributionV1{
		ContractVersion: assemblycontract.ContractVersionV1,
		ContributionID:  executionContributionIDV1,
		ModuleRef:       moduleIDV1,
		SlotRef:         "sandbox.execution",
		Kind:            assemblycontract.SlotContributionOwnerV1,
		CapabilityRef:   ExecutionCapabilityV1,
		PortSpecRef:     executionPortIDV1,
	}
	executionContribution.Digest, err = assemblycontract.SlotContributionDigestV1(executionContribution)
	if err != nil {
		return assemblercontract.ComponentReleaseV1{}, err
	}
	if err := executionContribution.Validate(); err != nil {
		return assemblercontract.ComponentReleaseV1{}, err
	}
	release := assemblercontract.ComponentReleaseV1{
		ReleaseID: request.ReleaseID, Revision: request.Revision, SupportMode: support,
		ComponentManifest: manifest, ModuleDescriptors: []assemblycontract.ModuleDescriptorV1{module},
		CapabilityDescriptors: []assemblycontract.CapabilityDescriptorV1{executionCapability, capability},
		SlotSpecs:             []assemblycontract.SlotSpecV1{}, SlotContributions: []assemblycontract.SlotContributionV1{executionContribution},
		PortSpecs: []assemblycontract.PortSpecV1{executionPort, port}, HookFaces: []assemblycontract.HookFaceSpecV1{},
		PhaseContributions: []assemblycontract.PhaseContributionV1{}, Dependencies: []assemblycontract.DependencySpecV1{},
		FactoryDescriptors:        []assemblycontract.ModuleFactoryDescriptorV1{executionFactory, factory},
		ProviderBindingCandidates: []assemblycontract.ProviderBindingCandidateV1{},
		RequiredPlanArtifacts:     []assemblercontract.PlanArtifactV1{}, SourceRef: request.SourceRef,
		ArtifactDigest: request.ArtifactDigest, EvidenceRefs: evidence,
		CreatedUnixNano: request.CreatedUnixNano, ExpiresUnixNano: expires,
	}
	if support == assemblercontract.SupportProductionV1 {
		release.CertificationRef = assemblycontract.ObjectRefV1{ID: request.CertificationID, Revision: request.Revision, Digest: runtimeports.EvidenceGenesisDigestV2}
		certificationDigest, err := assemblercontract.ComponentReleaseCertificationDigestV1(release)
		if err != nil {
			return assemblercontract.ComponentReleaseV1{}, err
		}
		if requireIndependentCertification && (readiness.CertificationFactRef.ID != request.CertificationID || readiness.CertificationFactRef.Revision != request.Revision || readiness.CertificationFactRef.Digest != certificationDigest) {
			return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "sandbox release lacks an independently exact certification fact")
		}
		if requireIndependentCertification {
			release.CertificationRef = readiness.CertificationFactRef
		} else {
			release.CertificationRef.Digest = certificationDigest
		}
	} else {
		release.CertificationRef = assemblycontract.ObjectRefV1{}
	}
	return assemblercontract.SealComponentReleaseV1(release)
}

func schemaRefV1(name, canonical string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.sandbox", Name: name, Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte(canonical))}
}

func contractRefV1(name string) assemblycontract.ObjectRefV1 {
	id := "praxis.sandbox/contract/" + name + "/v1"
	return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
}

func isKnownAbsent(err error) bool {
	return core.HasCategory(err, core.ErrorNotFound)
}

func nilLike(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func cloneReadiness(value *SandboxProductionReadinessProjectionV1) *SandboxProductionReadinessProjectionV1 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
