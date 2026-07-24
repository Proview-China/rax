package contract

import (
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func validateID(value string) error {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "assembler identity is empty, padded, or too long")
	}
	for _, b := range []byte(value) {
		if b < 0x21 || b > 0x7e {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "assembler identity must use visible ASCII")
		}
	}
	return nil
}

func validSupportMode(value SupportModeV1) bool {
	return value == SupportDisabledV1 || value == SupportReferenceOnlyV1 || value == SupportStandaloneV1 || value == SupportProductionV1
}

func validArtifactRole(value PlanArtifactRoleV1) bool {
	switch value {
	case ArtifactHarnessBootstrapV1, ArtifactProfileV1, ArtifactRuntimePolicyV1, ArtifactHarnessStackV1,
		ArtifactSemanticRouteV1, ArtifactContextPlanV1, ArtifactToolSurfaceV1, ArtifactCapabilityGrantV1,
		ArtifactExpectedInjectionV1:
		return true
	default:
		return false
	}
}

func (r ComponentReleaseRefV1) Validate() error {
	if err := validateID(r.ReleaseID); err != nil {
		return err
	}
	if r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "component release revision is required")
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	return runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(r.ComponentID))
}

func (r ComponentReleaseV1) RefV1() ComponentReleaseRefV1 {
	return ComponentReleaseRefV1{ReleaseID: r.ReleaseID, Revision: r.Revision, Digest: r.ReleaseDigest, ComponentID: r.ComponentManifest.ComponentID}
}

func (r ComponentReleaseV1) Validate() error {
	if r.ContractVersion != ReleaseContractVersionV1 || !validSupportMode(r.SupportMode) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMismatch, "component release contract or support mode is invalid")
	}
	if err := validateID(r.ReleaseID); err != nil {
		return err
	}
	if r.Revision == 0 || r.CreatedUnixNano <= 0 || r.ExpiresUnixNano <= r.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCapabilityExpired, "component release revision and validity window are required")
	}
	if err := r.ComponentManifest.Validate(); err != nil {
		return err
	}
	if err := r.ArtifactDigest.Validate(); err != nil || r.ArtifactDigest != r.ComponentManifest.ArtifactDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "component release artifact differs from its manifest")
	}
	if err := r.SourceRef.Validate(); err != nil {
		return err
	}
	if r.SupportMode == SupportProductionV1 {
		if r.ComponentManifest.Conformance != runtimeports.ConformanceFullyControlled || r.ComponentManifest.ResidualClass != runtimeports.ResidualNone {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "production release must be fully controlled and residual-free")
		}
		certifiedDigest, digestErr := ComponentReleaseCertificationDigestV1(r)
		if err := r.CertificationRef.Validate(); err != nil || digestErr != nil || r.CertificationRef.Digest != certifiedDigest || len(r.EvidenceRefs) == 0 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "production release requires exact certification and evidence")
		}
	}
	if len(r.ModuleDescriptors) == 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "component release requires at least one module")
	}
	if overLimit(r.ModuleDescriptors) || overLimit(r.CapabilityDescriptors) || overLimit(r.SlotSpecs) || overLimit(r.SlotContributions) || overLimit(r.PortSpecs) || overLimit(r.HookFaces) || overLimit(r.PhaseContributions) || overLimit(r.Dependencies) || overLimit(r.FactoryDescriptors) || overLimit(r.ProviderBindingCandidates) || overLimit(r.RequiredPlanArtifacts) || overLimit(r.EvidenceRefs) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "component release collection exceeds its bound")
	}
	for _, value := range r.ModuleDescriptors {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range r.CapabilityDescriptors {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range r.SlotSpecs {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range r.SlotContributions {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range r.PortSpecs {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range r.HookFaces {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range r.PhaseContributions {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range r.Dependencies {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range r.FactoryDescriptors {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range r.ProviderBindingCandidates {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	roles := map[PlanArtifactRoleV1]struct{}{}
	for _, value := range r.RequiredPlanArtifacts {
		if !validArtifactRole(value.Role) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "component release contains an unknown plan artifact role")
		}
		if err := value.Ref.Validate(); err != nil {
			return err
		}
		if _, exists := roles[value.Role]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "component release duplicates a plan artifact role")
		}
		roles[value.Role] = struct{}{}
	}
	for _, ref := range r.EvidenceRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if err := validateOwnerClosure(r.ComponentManifest.Owners); err != nil {
		return err
	}
	if r.SupportMode == SupportProductionV1 {
		if err := validateProductionReleaseClosure(r); err != nil {
			return err
		}
	}
	digest, err := ComponentReleaseDigestV1(r)
	if err != nil || digest != r.ReleaseDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "component release digest is missing or drifted")
	}
	return nil
}

func validateProductionReleaseClosure(release ComponentReleaseV1) error {
	manifest := release.ComponentManifest
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		return err
	}
	provided := make(map[runtimeports.CapabilityNameV2]runtimeports.ProvidedCapabilityV2, len(manifest.ProvidedCapabilities))
	for _, capability := range manifest.ProvidedCapabilities {
		if _, exists := provided[capability.Capability]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "production manifest capability is duplicated")
		}
		provided[capability.Capability] = capability
	}
	if len(provided) == 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "production release has no provided capability")
	}
	descriptors := map[runtimeports.CapabilityNameV2]assemblycontract.CapabilityDescriptorV1{}
	requiredDescriptors := map[runtimeports.CapabilityNameV2]struct{}{}
	descriptorKeys := map[runtimeports.CapabilityNameV2]struct{}{}
	for _, descriptor := range release.CapabilityDescriptors {
		if _, exists := descriptorKeys[descriptor.Capability]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "production capability descriptor identity is duplicated")
		}
		descriptorKeys[descriptor.Capability] = struct{}{}
		if descriptor.Required {
			requiredDescriptors[descriptor.Capability] = struct{}{}
		}
		if !descriptor.Provided {
			continue
		}
		if _, exists := descriptors[descriptor.Capability]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "production capability descriptor is duplicated")
		}
		descriptors[descriptor.Capability] = descriptor
	}
	requiredManifest := map[runtimeports.CapabilityNameV2]struct{}{}
	for _, requirement := range manifest.RequiredCapabilities {
		requiredManifest[requirement.Capability] = struct{}{}
	}
	if len(requiredDescriptors) != len(requiredManifest) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "production required capability descriptors differ from the manifest")
	}
	for capability := range requiredManifest {
		if _, exists := requiredDescriptors[capability]; !exists {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "production required capability descriptor is missing")
		}
	}
	moduleCapabilities := map[runtimeports.CapabilityNameV2]struct{}{}
	moduleSchemas := map[string]runtimeports.SchemaRefV2{}
	modules := map[string]assemblycontract.ModuleDescriptorV1{}
	for _, module := range release.ModuleDescriptors {
		if _, exists := modules[module.ModuleID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "production module id is duplicated")
		}
		modules[module.ModuleID] = module
		if module.ArtifactDigest != release.ArtifactDigest || module.SemanticVersion != manifest.SemanticVersion || module.Locality != manifest.Locality || module.ResidualClass != manifest.ResidualClass || module.ComponentManifestRef.ID != string(manifest.ComponentID) || module.ComponentManifestRef.Revision != release.Revision || module.ComponentManifestRef.Digest != manifestDigest || !sameOwnerAssignments(module.Owners, manifest.Owners) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "production module does not exactly bind its release manifest and artifact")
		}
		for _, capability := range module.Capabilities {
			if _, exists := moduleCapabilities[capability]; exists {
				return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "production module capability is duplicated or aliased")
			}
			moduleCapabilities[capability] = struct{}{}
		}
		for _, schema := range module.Schemas {
			if _, exists := moduleSchemas[schema.Key()]; exists {
				return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "production module schema is duplicated or aliased")
			}
			moduleSchemas[schema.Key()] = schema
		}
	}
	if !sameSchemaProjection(moduleSchemas, manifest.Schemas) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "production module schemas differ from the manifest")
	}
	factories := map[runtimeports.CapabilityNameV2]assemblycontract.ModuleFactoryDescriptorV1{}
	factoryIDs := map[string]struct{}{}
	for _, factory := range release.FactoryDescriptors {
		if _, exists := factoryIDs[factory.FactoryID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "production factory id is duplicated")
		}
		factoryIDs[factory.FactoryID] = struct{}{}
		if _, exists := factories[factory.OutputCapability]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "production factory output capability is duplicated")
		}
		module, exists := modules[factory.ModuleRef]
		if !exists || factory.ArtifactDigest != release.ArtifactDigest || factory.ArtifactDigest != module.ArtifactDigest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "production factory does not exactly bind a module, artifact, and cleanup owner")
		}
		factories[factory.OutputCapability] = factory
	}
	portsByOwner := map[runtimeports.CapabilityNameV2]assemblycontract.PortSpecV1{}
	portIDs := map[string]struct{}{}
	for _, port := range release.PortSpecs {
		if _, exists := portIDs[port.PortID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "production port id is duplicated")
		}
		portIDs[port.PortID] = struct{}{}
		if _, exists := portsByOwner[port.OwnerCapability]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "production port capability is duplicated or aliased")
		}
		portsByOwner[port.OwnerCapability] = port
	}
	usedPortOwners := map[runtimeports.CapabilityNameV2]struct{}{}
	for capability, manifestCapability := range provided {
		descriptor, descriptorOK := descriptors[capability]
		factory, factoryOK := factories[capability]
		port, portOK := portsByOwner[descriptor.OwnerCapability]
		_, moduleOK := moduleCapabilities[capability]
		if !descriptorOK || !factoryOK || !portOK || !moduleOK {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "production capability lacks exact descriptor, module, factory, or port closure")
		}
		if _, used := usedPortOwners[descriptor.OwnerCapability]; used {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "production capabilities alias one port owner")
		}
		usedPortOwners[descriptor.OwnerCapability] = struct{}{}
		if descriptor.Version != manifest.SemanticVersion || descriptor.TTLSeconds != manifestCapability.TTLSeconds || descriptor.Conformance != manifest.Conformance || !sameSchemas(descriptor.Schemas, manifestCapability.Schemas) || factory.CleanupContractRef.OwnerCapability != descriptor.OwnerCapability {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "production capability descriptor differs from its manifest capability")
		}
		if !schemaIn(port.RequestSchema, manifestCapability.Schemas) || !schemaIn(port.ResponseSchema, manifestCapability.Schemas) || !schemaIn(factory.InputSchema, manifestCapability.Schemas) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownSchema, "production port or factory schema is outside its capability descriptor")
		}
	}
	if len(descriptors) != len(provided) || len(moduleCapabilities) != len(provided) || len(factories) != len(provided) || len(portsByOwner) != len(provided) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "production capability closure contains undeclared capability aliases")
	}
	return nil
}

func sameOwnerAssignments(left, right []runtimeports.OwnerAssignmentV2) bool {
	if len(left) != len(right) {
		return false
	}
	set := make(map[runtimeports.OwnerAssignmentV2]struct{}, len(left))
	for _, owner := range left {
		set[owner] = struct{}{}
	}
	for _, owner := range right {
		if _, exists := set[owner]; !exists {
			return false
		}
	}
	return true
}

func sameSchemaProjection(left map[string]runtimeports.SchemaRefV2, right []runtimeports.SchemaRefV2) bool {
	if len(left) != len(right) {
		return false
	}
	for _, schema := range right {
		if existing, exists := left[schema.Key()]; !exists || existing != schema {
			return false
		}
	}
	return true
}

func sameSchemas(left, right []runtimeports.SchemaRefV2) bool {
	projection := make(map[string]runtimeports.SchemaRefV2, len(left))
	for _, schema := range left {
		if _, exists := projection[schema.Key()]; exists {
			return false
		}
		projection[schema.Key()] = schema
	}
	return sameSchemaProjection(projection, right)
}

func schemaIn(value runtimeports.SchemaRefV2, values []runtimeports.SchemaRefV2) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func validateOwnerClosure(owners []runtimeports.OwnerAssignmentV2) error {
	counts := map[runtimeports.OwnerRoleV2]int{}
	for _, owner := range owners {
		counts[owner.Role]++
	}
	for _, role := range []runtimeports.OwnerRoleV2{runtimeports.OwnerEffect, runtimeports.OwnerSettlement, runtimeports.OwnerCleanup} {
		if counts[role] != 1 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerConflict, "component release requires one exact effect, settlement, and cleanup owner")
		}
	}
	return nil
}

func overLimit[T any](values []T) bool { return len(values) > MaxEntriesV1 }

func (r ComponentReleaseCatalogRefV1) Validate() error {
	if err := validateID(r.CatalogID); err != nil {
		return err
	}
	if r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "catalog revision is required")
	}
	return r.Digest.Validate()
}

func (c ComponentReleaseCatalogSnapshotV1) RefV1() ComponentReleaseCatalogRefV1 {
	return ComponentReleaseCatalogRefV1{CatalogID: c.CatalogID, Revision: c.Revision, Digest: c.Digest}
}

func (c ComponentReleaseCatalogSnapshotV1) Validate() error {
	if c.ContractVersion != CatalogContractVersionV1 || c.Revision == 0 || c.CheckedUnixNano <= 0 || c.ExpiresUnixNano <= c.CheckedUnixNano || len(c.Releases) == 0 || len(c.Releases) > MaxEntriesV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "catalog identity, releases, and current window are required")
	}
	if err := validateID(c.CatalogID); err != nil {
		return err
	}
	if err := c.Governance.Validate(); err != nil {
		return err
	}
	governanceDigest, err := c.Governance.DigestV2()
	if err != nil || governanceDigest != c.GovernanceDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "catalog governance digest is missing or drifted")
	}
	type releaseKey struct {
		ID       string
		Revision core.Revision
	}
	seen := map[releaseKey]struct{}{}
	currentRevisionByID := map[string]core.Revision{}
	for _, release := range c.Releases {
		if err := release.Validate(); err != nil {
			return err
		}
		if err := runtimeports.ValidateManifestAgainstCatalogV2(release.ComponentManifest, c.Governance); err != nil {
			return err
		}
		if release.CreatedUnixNano > c.CheckedUnixNano || release.ExpiresUnixNano <= c.CheckedUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "catalog snapshot contains a release outside its checked current window")
		}
		key := releaseKey{ID: release.ReleaseID, Revision: release.Revision}
		if _, exists := seen[key]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "catalog contains a duplicate release identity")
		}
		seen[key] = struct{}{}
		if _, exists := currentRevisionByID[release.ReleaseID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "catalog snapshot contains multiple current revisions for one release id")
		} else {
			currentRevisionByID[release.ReleaseID] = release.Revision
		}
	}
	digest, err := ComponentReleaseCatalogDigestV1(c)
	if err != nil || digest != c.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "catalog digest is missing or drifted")
	}
	return nil
}

func (r ResolutionFactsRefV1) Validate() error {
	if err := validateID(r.FactsID); err != nil {
		return err
	}
	if r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "resolution facts revision is required")
	}
	return r.Digest.Validate()
}

func (f ResolutionFactsSnapshotV1) RefV1() ResolutionFactsRefV1 {
	return ResolutionFactsRefV1{FactsID: f.FactsID, Revision: f.Revision, Digest: f.Digest}
}

func (f ResolutionFactsSnapshotV1) Validate() error {
	if f.ContractVersion != FactsContractVersionV1 || f.Revision == 0 || f.FrozenUnixNano <= 0 || f.ExpiresUnixNano <= f.FrozenUnixNano || f.MaximumPriority <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "resolution facts identity and frozen current window are required")
	}
	if err := validateID(f.FactsID); err != nil {
		return err
	}
	if err := f.DefinitionRef.Validate(); err != nil {
		return err
	}
	for _, ref := range []assemblycontract.ObjectRefV1{f.IdentityRef, f.SandboxRequirementRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	for _, refs := range [][]assemblycontract.ObjectRefV1{f.PolicyRefs, f.CurrentFacts, f.RouteBindings, f.EvidenceRefs} {
		if len(refs) > MaxEntriesV1 {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "resolution fact reference set exceeds its bound")
		}
		seen := map[string]struct{}{}
		for _, ref := range refs {
			if err := ref.Validate(); err != nil {
				return err
			}
			if _, exists := seen[ref.ID]; exists {
				return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "resolution fact reference set contains duplicate id")
			}
			seen[ref.ID] = struct{}{}
		}
	}
	if len(f.PolicyRefs) == 0 || len(f.CurrentFacts) == 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "resolution facts require policy and current fact refs")
	}
	if err := validateID(f.OwnerRef); err != nil {
		return err
	}
	if err := validateID(f.ScopeRef); err != nil {
		return err
	}
	digest, err := ResolutionFactsDigestV1(f)
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "resolution facts digest is missing or drifted")
	}
	return nil
}

func (r ResolvedAgentPlanRefV1) Validate() error {
	if err := validateID(r.PlanID); err != nil {
		return err
	}
	if r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "resolved plan revision is required")
	}
	return r.Digest.Validate()
}

func (p ResolvedAgentPlanV1) RefV1() ResolvedAgentPlanRefV1 {
	return ResolvedAgentPlanRefV1{PlanID: p.PlanID, Revision: p.Revision, Digest: p.Digest}
}

func (p ResolvedAgentPlanV1) Validate() error {
	if p.ContractVersion != PlanContractVersionV1 || p.Revision == 0 || p.CreatedUnixNano <= 0 || p.ValidUntilUnixNano <= p.CreatedUnixNano || len(p.ComponentReleases) == 0 || len(p.ComponentReleases) > MaxEntriesV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "resolved plan identity, releases, and validity are required")
	}
	if err := validateID(p.PlanID); err != nil {
		return err
	}
	if err := p.DefinitionRef.Validate(); err != nil {
		return err
	}
	if err := p.IdentityRef.Validate(); err != nil || p.SandboxRequirementRef.Validate() != nil || p.HarnessBootstrapRef.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "resolved plan exact identity, sandbox, or bootstrap ref is invalid")
	}
	if err := p.ProfileRef.Validate(); err != nil {
		return err
	}
	if err := p.ResolutionFactsRef.Validate(); err != nil {
		return err
	}
	if err := p.CatalogRef.Validate(); err != nil {
		return err
	}
	if err := p.BindingPlan.Validate(); err != nil {
		return err
	}
	if err := validateBindingProjection(p); err != nil {
		return err
	}
	if err := p.AssemblyPlanRefs.Validate(); err != nil {
		return err
	}
	if p.AssemblyPlanRefs.ResolvedAgentPlan.ID != p.PlanID || p.AssemblyPlanRefs.ResolvedAgentPlan.Revision != p.Revision || p.AssemblyPlanRefs.ResolvedAgentPlan.Digest != p.Digest || p.AssemblyPlanRefs.HarnessBootstrapPlan != p.HarnessBootstrapRef || p.AssemblyPlanRefs.Profile != p.ProfileRef {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "resolved plan self and plan artifact refs are inconsistent")
	}
	seen := map[runtimeports.ComponentIDV2]struct{}{}
	for _, component := range p.ComponentReleases {
		if err := component.ReleaseRef.Validate(); err != nil || component.Manifest.Validate() != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "resolved component release projection is invalid")
		}
		if component.ReleaseRef.ComponentID != component.Manifest.ComponentID {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "resolved release ref and manifest component differ")
		}
		if _, exists := seen[component.Manifest.ComponentID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "resolved plan contains duplicate component")
		}
		seen[component.Manifest.ComponentID] = struct{}{}
	}
	for _, ref := range append(append([]assemblycontract.ObjectRefV1{}, p.PolicyRefs...), p.EvidenceRefs...) {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if len(p.Residuals) != 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRemoteResidualUnresolved, "production resolved plan must be residual-free")
	}
	digest, err := ResolvedAgentPlanDigestV1(p)
	if err != nil || digest != p.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "resolved plan digest is missing or drifted")
	}
	return nil
}

func validateBindingProjection(plan ResolvedAgentPlanV1) error {
	if plan.BindingPlan.ID != plan.PlanID+"-binding" || len(plan.BindingPlan.Requirements) != len(plan.ComponentReleases) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "resolved plan binding projection identity or cardinality drifted")
	}
	manifests := make(map[runtimeports.ComponentIDV2]runtimeports.ComponentManifestV2, len(plan.ComponentReleases))
	for _, component := range plan.ComponentReleases {
		manifests[component.Manifest.ComponentID] = component.Manifest
	}
	for _, requirement := range plan.BindingPlan.Requirements {
		manifest, ok := manifests[requirement.ComponentID]
		if !ok || requirement.Kind != manifest.Kind || requirement.ArtifactDigest != manifest.ArtifactDigest || requirement.ContractName != manifest.Contract.Name || !requirement.SemanticVersion.Contains(manifest.SemanticVersion) || !requirement.Contract.Contains(manifest.Contract.Version) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "binding requirement differs from its exact resolved manifest")
		}
		provided := make(map[runtimeports.CapabilityNameV2]struct{}, len(manifest.ProvidedCapabilities))
		for _, capability := range manifest.ProvidedCapabilities {
			provided[capability.Capability] = struct{}{}
		}
		for _, capability := range requirement.RequiredCapabilities {
			if _, exists := provided[capability]; !exists {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "binding requirement capability is absent from its exact resolved manifest")
			}
		}
	}
	return nil
}

func (c CurrentResolvedPlanV1) Validate() error {
	if err := validateID(c.DefinitionID); err != nil {
		return err
	}
	if err := c.PlanRef.Validate(); err != nil {
		return err
	}
	if c.Revision == 0 || c.UpdatedUnixNano <= 0 || c.CheckedUnixNano < c.UpdatedUnixNano || c.ExpiresUnixNano <= c.CheckedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingExpired, "resolved plan current window is invalid")
	}
	if c.Revision == 1 && c.PreviousRef != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "initial current projection cannot have a predecessor")
	}
	if c.Revision > 1 {
		if c.PreviousRef == nil || c.PreviousRef.DefinitionID != c.DefinitionID || c.PreviousRef.Revision+1 != c.Revision || c.PreviousRef.Validate() != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRevisionConflict, "current projection requires the exact immediately preceding projection")
		}
	}
	digest, err := CurrentResolvedPlanDigestV1(c)
	if err != nil || digest != c.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "resolved plan current digest is missing or drifted")
	}
	return nil
}

func (c CurrentResolvedPlanV1) RefV1() CurrentResolvedPlanRefV1 {
	return CurrentResolvedPlanRefV1{DefinitionID: c.DefinitionID, Revision: c.Revision, Digest: c.Digest}
}
func (r CurrentResolvedPlanRefV1) Validate() error {
	if err := validateID(r.DefinitionID); err != nil {
		return err
	}
	if r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "current projection revision is required")
	}
	return r.Digest.Validate()
}

func normalizeRefs(values []assemblycontract.ObjectRefV1) []assemblycontract.ObjectRefV1 {
	values = append([]assemblycontract.ObjectRefV1{}, values...)
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	if values == nil {
		return []assemblycontract.ObjectRefV1{}
	}
	return values
}
