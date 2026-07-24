package release

import (
	"context"
	"reflect"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	organizationcontract "github.com/Proview-China/rax/ExecutionRuntime/organization-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type PublisherV1 struct {
	local      LocalReadinessReaderV1
	production ProductionReadinessReaderV1
	catalog    ComponentReleaseCatalogPortV1
	clock      func() time.Time
}

func NewPublisherV1(local LocalReadinessReaderV1, production ProductionReadinessReaderV1, catalog ComponentReleaseCatalogPortV1, clock func() time.Time) (*PublisherV1, error) {
	if nilLike(local) || nilLike(production) || nilLike(catalog) || nilLike(clock) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Organization release publisher dependencies are incomplete")
	}
	return &PublisherV1{local: local, production: production, catalog: catalog, clock: clock}, nil
}

func (p *PublisherV1) Publish(ctx context.Context, request PublicationRequestV1) (PublicationResultV1, error) {
	if p == nil || ctx == nil {
		return PublicationResultV1{}, invalid("Organization publisher or context is nil")
	}
	start := p.clock()
	if err := request.Validate(start); err != nil {
		return PublicationResultV1{}, err
	}
	lastObserved := start
	fresh := func() time.Time {
		now := p.clock()
		if now.IsZero() || now.Before(lastObserved) {
			return time.Time{}
		}
		lastObserved = now
		return now
	}
	local, err := p.local.InspectOrganizationLocalReadinessV1(ctx, request.ReleaseID, request.Revision)
	hasLocal := err == nil
	if err != nil && !core.HasCategory(err, core.ErrorNotFound) {
		return PublicationResultV1{}, err
	}
	if hasLocal {
		if local.ReleaseID != request.ReleaseID || local.Revision != request.Revision || local.ArtifactDigest != request.ArtifactDigest {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Organization local readiness targets another release")
		}
		if err = local.ValidateCurrent(fresh()); err != nil {
			return PublicationResultV1{}, err
		}
	}
	production, err := p.production.InspectOrganizationProductionReadinessV1(ctx, request.ReleaseID, request.Revision)
	hasProduction := err == nil
	if err != nil && !core.HasCategory(err, core.ErrorNotFound) {
		return PublicationResultV1{}, err
	}
	if hasProduction && !hasLocal {
		return PublicationResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Organization production readiness cannot replace SQLite local readiness")
	}
	if hasProduction {
		if production.ReleaseID != request.ReleaseID || production.Revision != request.Revision || production.ArtifactDigest != request.ArtifactDigest || production.LocalReadinessRef != local.ExactRefV1() {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Organization production readiness targets another release or local readiness")
		}
		if err = production.ValidateCurrent(fresh()); err != nil {
			return PublicationResultV1{}, err
		}
	}
	release, err := buildReleasePayloadV1(request, supportMode(hasLocal, hasProduction), optionalLocal(hasLocal, local), optionalProduction(hasProduction, production), true)
	if err != nil {
		return PublicationResultV1{}, err
	}
	if hasLocal {
		second, readErr := p.local.InspectOrganizationLocalReadinessV1(ctx, request.ReleaseID, request.Revision)
		if readErr != nil {
			return PublicationResultV1{}, readErr
		}
		if second != local {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Organization local readiness drifted before publication")
		}
		if err = second.ValidateCurrent(fresh()); err != nil {
			return PublicationResultV1{}, err
		}
	}
	if hasProduction {
		second, readErr := p.production.InspectOrganizationProductionReadinessV1(ctx, request.ReleaseID, request.Revision)
		if readErr != nil {
			return PublicationResultV1{}, readErr
		}
		if second != production {
			return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Organization production readiness drifted before publication")
		}
		if err = second.ValidateCurrent(fresh()); err != nil {
			return PublicationResultV1{}, err
		}
	}
	beforeWrite := fresh()
	if beforeWrite.IsZero() {
		return PublicationResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Organization clock regressed before catalog publication")
	}
	if !beforeWrite.Before(time.Unix(0, release.ExpiresUnixNano)) {
		return PublicationResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Organization release expired before catalog publication")
	}
	written, writeErr := p.catalog.EnsureExactComponentReleaseV1(ctx, release)
	if writeErr != nil {
		inspected, inspectErr := p.catalog.InspectExactComponentReleaseV1(ctx, release.RefV1())
		if inspectErr != nil {
			return PublicationResultV1{}, writeErr
		}
		written = inspected
	}
	if written.ReleaseDigest != release.ReleaseDigest {
		return PublicationResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Organization catalog returned different release content")
	}
	done := fresh()
	if done.IsZero() || !done.Before(time.Unix(0, written.ExpiresUnixNano)) {
		return PublicationResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Organization release expired or clock regressed during publication")
	}
	result := PublicationResultV1{Release: written, LocalReady: hasLocal, ProductionReady: hasProduction}
	if hasLocal {
		copy := local
		result.LocalReadiness = &copy
	}
	if hasProduction {
		copy := production
		result.ProductionReadiness = &copy
	}
	return result, nil
}

func supportMode(local, production bool) assemblercontract.SupportModeV1 {
	if production {
		return assemblercontract.SupportProductionV1
	}
	if local {
		return assemblercontract.SupportStandaloneV1
	}
	return assemblercontract.SupportReferenceOnlyV1
}

func optionalLocal(ok bool, value LocalReadinessProjectionV1) *LocalReadinessProjectionV1 {
	if !ok {
		return nil
	}
	return &value
}

func optionalProduction(ok bool, value ProductionReadinessProjectionV1) *ProductionReadinessProjectionV1 {
	if !ok {
		return nil
	}
	return &value
}

func buildReleaseV1(request PublicationRequestV1, support assemblercontract.SupportModeV1, local *LocalReadinessProjectionV1, production *ProductionReadinessProjectionV1) (assemblercontract.ComponentReleaseV1, error) {
	return buildReleasePayloadV1(request, support, local, production, true)
}

func buildReleasePayloadV1(request PublicationRequestV1, support assemblercontract.SupportModeV1, local *LocalReadinessProjectionV1, production *ProductionReadinessProjectionV1, requireCertification bool) (assemblercontract.ComponentReleaseV1, error) {
	requestSchema := schema("review-eligibility-source-v1", organizationcontract.ContractVersionV1+"/ReviewEligibilitySourceV1")
	responseSchema := schema("review-eligibility-current-v1", organizationcontract.ContractVersionV1+"/ReviewEligibilityCurrentProjectionV1")
	schemas := []runtimeports.SchemaRefV2{requestSchema, responseSchema}
	conformance := runtimeports.ConformanceContainedObserveOnly
	residual := runtimeports.ResidualPotentiallyStale
	if support == assemblercontract.SupportStandaloneV1 {
		conformance = runtimeports.ConformanceRestrictedControlled
		residual = runtimeports.ResidualInspectable
	}
	if support == assemblercontract.SupportProductionV1 {
		conformance = runtimeports.ConformanceFullyControlled
		residual = runtimeports.ResidualNone
	}
	owners := []runtimeports.OwnerAssignmentV2{
		{Role: runtimeports.OwnerEffect, OwnerComponentID: runtimeports.RuntimeSharedEngineComponentIDV1},
		{Role: runtimeports.OwnerSettlement, OwnerComponentID: runtimeports.RuntimeSharedEngineComponentIDV1},
		{Role: runtimeports.OwnerCleanup, OwnerComponentID: ComponentIDV1},
	}
	manifest := runtimeports.ComponentManifestV2{
		ContractVersion:      runtimeports.BindingContractVersionV2,
		ComponentID:          ComponentIDV1,
		Kind:                 ComponentKindV1,
		GovernanceCategory:   "praxis/core",
		SemanticVersion:      ReleaseSemanticVersionV1,
		ArtifactDigest:       request.ArtifactDigest,
		Contract:             runtimeports.ContractBindingV2{Name: ContractNameV1, Version: ReleaseSemanticVersionV1, Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}},
		Schemas:              schemas,
		Locality:             runtimeports.LocalityHostControlPlane,
		Dependencies:         []runtimeports.ComponentDependencyV2{},
		RequiredCapabilities: []runtimeports.CapabilityRequirementV2{},
		ProvidedCapabilities: []runtimeports.ProvidedCapabilityV2{{Capability: CapabilityV1, TTLSeconds: 30, Schemas: schemas}},
		Conformance:          conformance,
		ResidualClass:        residual,
		Owners:               owners,
		Credentials:          []runtimeports.CredentialRequirementV2{},
		OfflinePolicy:        runtimeports.OfflineDenied,
		Extensions:           []runtimeports.GovernanceExtensionV2{},
		Annotations:          []runtimeports.DisplayAnnotationV2{},
	}
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		return assemblercontract.ComponentReleaseV1{}, err
	}
	if production != nil && requireCertification && (production.ManifestDigest != manifestDigest || production.ArtifactDigest != request.ArtifactDigest) {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Organization production readiness certifies another manifest")
	}
	module := assemblycontract.ModuleDescriptorV1{
		ContractVersion:        assemblycontract.ContractVersionV1,
		ModuleID:               ModuleIDV1,
		Namespace:              "praxis.organization",
		SemanticVersion:        ReleaseSemanticVersionV1,
		ArtifactDigest:         request.ArtifactDigest,
		PublisherRef:           request.PublisherRef,
		SourceRef:              request.SourceRef,
		ComponentManifestRef:   assemblycontract.ObjectRefV1{ID: string(ComponentIDV1), Revision: request.Revision, Digest: manifestDigest},
		Compatibility:          assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"},
		Capabilities:           []runtimeports.CapabilityNameV2{CapabilityV1},
		Schemas:                schemas,
		Locality:               manifest.Locality,
		ResidualClass:          residual,
		Owners:                 owners,
		CredentialRequirements: []runtimeports.NamespacedNameV2{},
	}
	capability := assemblycontract.CapabilityDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, Capability: CapabilityV1, Version: ReleaseSemanticVersionV1, Schemas: schemas, Provided: true, TTLSeconds: 30, EffectClass: "read-only-current", OwnerCapability: CapabilityV1, Conformance: conformance}
	port := assemblycontract.PortSpecV1{ContractVersion: assemblycontract.ContractVersionV1, PortID: PortIDV1, OwnerCapability: CapabilityV1, RequestSchema: requestSchema, ResponseSchema: responseSchema, OperationClass: "inspect-current", Idempotency: "exact-ref-read", CancelSupported: false, FailureSemantics: "unavailable-fail-closed", Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}
	factory := assemblycontract.ModuleFactoryDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, FactoryID: FactoryIDV1, ModuleRef: ModuleIDV1, ArtifactDigest: request.ArtifactDigest, ConstructionMode: assemblycontract.ConstructionTrustedInProcessGoV1, InputSchema: requestSchema, OutputCapability: CapabilityV1, Lifecycle: assemblycontract.LifecycleGenerationV1, CleanupContractRef: assemblycontract.CleanupContractRefV1{Ref: ref(FactoryIDV1 + "/cleanup"), OwnerCapability: CapabilityV1, RequestSchema: requestSchema, ResultSchema: responseSchema}, TrustRef: request.TrustRef}
	expires := request.ExpiresUnixNano
	evidence := []assemblycontract.ObjectRefV1{}
	if local != nil {
		if local.ExpiresUnixNano < expires {
			expires = local.ExpiresUnixNano
		}
		evidence = append(evidence, local.ExactRefV1())
		evidence = append(evidence, local.refs()...)
	}
	if production != nil {
		if production.ExpiresUnixNano < expires {
			expires = production.ExpiresUnixNano
		}
		evidence = append(evidence, production.evidenceRefs()...)
	}
	release := assemblercontract.ComponentReleaseV1{
		ReleaseID:                 request.ReleaseID,
		Revision:                  request.Revision,
		SupportMode:               support,
		ComponentManifest:         manifest,
		ModuleDescriptors:         []assemblycontract.ModuleDescriptorV1{module},
		CapabilityDescriptors:     []assemblycontract.CapabilityDescriptorV1{capability},
		SlotSpecs:                 []assemblycontract.SlotSpecV1{},
		SlotContributions:         []assemblycontract.SlotContributionV1{},
		PortSpecs:                 []assemblycontract.PortSpecV1{port},
		HookFaces:                 []assemblycontract.HookFaceSpecV1{},
		PhaseContributions:        []assemblycontract.PhaseContributionV1{},
		Dependencies:              []assemblycontract.DependencySpecV1{},
		FactoryDescriptors:        []assemblycontract.ModuleFactoryDescriptorV1{factory},
		ProviderBindingCandidates: []assemblycontract.ProviderBindingCandidateV1{},
		RequiredPlanArtifacts:     []assemblercontract.PlanArtifactV1{},
		SourceRef:                 request.SourceRef,
		ArtifactDigest:            request.ArtifactDigest,
		EvidenceRefs:              evidence,
		CreatedUnixNano:           request.CreatedUnixNano,
		ExpiresUnixNano:           expires,
	}
	if support == assemblercontract.SupportProductionV1 {
		release.CertificationRef = assemblycontract.ObjectRefV1{ID: request.CertificationID, Revision: request.Revision, Digest: runtimeports.EvidenceGenesisDigestV2}
		certificationDigest, digestErr := assemblercontract.ComponentReleaseCertificationDigestV1(release)
		if digestErr != nil {
			return assemblercontract.ComponentReleaseV1{}, digestErr
		}
		if requireCertification && (production == nil || production.CertificationFactRef.ID != request.CertificationID || production.CertificationFactRef.Revision != request.Revision || production.CertificationFactRef.Digest != certificationDigest) {
			return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Organization release lacks independent exact certification")
		}
		if requireCertification {
			release.CertificationRef = production.CertificationFactRef
		} else {
			release.CertificationRef.Digest = certificationDigest
		}
	}
	return assemblercontract.SealComponentReleaseV1(release)
}

func schema(name, canonical string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.organization", Name: name, Version: ReleaseSemanticVersionV1, MediaType: "application/json", ContentDigest: core.DigestBytes([]byte(canonical))}
}

func ref(id string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
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
