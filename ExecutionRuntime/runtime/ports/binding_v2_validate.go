package ports

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	MaxOpaqueDeclaredBytes  = 64 << 20
	MaxCapabilityTTLSeconds = 30 * 24 * 60 * 60
)

func ValidateNamespacedNameV2(value NamespacedNameV2) error {
	raw := string(value)
	if len(raw) < 3 || len(raw) > 128 || strings.Count(raw, "/") != 1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidNamespace, "name must be one bounded namespace/name ASCII pair")
	}
	parts := strings.Split(raw, "/")
	if !validLowerASCIIName(parts[0], true) || !validLowerASCIIName(parts[1], false) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidNamespace, "namespace and name must use canonical lowercase ASCII")
	}
	return nil
}

func (r VersionRangeV2) Validate() error {
	minimum, err := core.ParseSemanticVersion(r.MinimumInclusive)
	if err != nil {
		return err
	}
	maximum, err := core.ParseSemanticVersion(r.MaximumExclusive)
	if err != nil {
		return err
	}
	if minimum.String() != r.MinimumInclusive || maximum.String() != r.MaximumExclusive || len(minimum.Build) != 0 || len(maximum.Build) != 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidSemanticVersion, "version range endpoints must be canonical and omit build metadata")
	}
	if core.CompareSemanticVersion(minimum, maximum) >= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidSemanticVersion, "version range must be non-empty")
	}
	return nil
}

func (r VersionRangeV2) Contains(value string) bool {
	if r.Validate() != nil {
		return false
	}
	version, err := core.ParseSemanticVersion(value)
	if err != nil || version.String() != value {
		return false
	}
	minimum, _ := core.ParseSemanticVersion(r.MinimumInclusive)
	maximum, _ := core.ParseSemanticVersion(r.MaximumExclusive)
	return core.CompareSemanticVersion(version, minimum) >= 0 && core.CompareSemanticVersion(version, maximum) < 0
}

func (s SchemaRefV2) Validate() error {
	if !validLowerASCIIName(s.Namespace, true) || !validLowerASCIIName(s.Name, false) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidNamespace, "schema namespace and name must use canonical lowercase ASCII")
	}
	version, err := core.ParseSemanticVersion(s.Version)
	if err != nil || version.String() != s.Version {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidSemanticVersion, "schema version must be canonical SemVer")
	}
	if !validMediaType(s.MediaType) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "schema media type must use canonical lowercase ASCII")
	}
	return s.ContentDigest.Validate()
}

func (p OpaquePayloadV2) Validate() error {
	if err := p.Schema.Validate(); err != nil {
		return err
	}
	if err := p.ContentDigest.Validate(); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(p.LimitPolicy.Policy); err != nil {
		return err
	}
	if err := p.LimitPolicy.Digest.Validate(); err != nil {
		return err
	}
	if p.Length == 0 || p.Length > MaxOpaqueDeclaredBytes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "opaque payload declared length is zero or exceeds policy ceiling")
	}
	inline := p.Inline != nil
	reference := p.Ref != ""
	if inline == reference {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "opaque payload requires exactly one inline body or reference")
	}
	if inline {
		if len(p.Inline) > MaxOpaqueInlineBytes || uint64(len(p.Inline)) != p.Length {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "opaque inline body violates its bounded declared length")
		}
		if core.DigestBytes(p.Inline) != p.ContentDigest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "opaque inline body does not match its content digest")
		}
	} else if len(p.Ref) > MaxOpaqueReferenceBytes || strings.TrimSpace(p.Ref) != p.Ref {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "opaque reference is padded or too long")
	}
	return nil
}

func (m ComponentManifestV2) Validate() error {
	if m.ContractVersion != BindingContractVersionV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "binding manifest requires the v2 contract discriminator")
	}
	if err := ValidateNamespacedNameV2(NamespacedNameV2(m.ComponentID)); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(NamespacedNameV2(m.Kind)); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(NamespacedNameV2(m.GovernanceCategory)); err != nil {
		return err
	}
	semanticVersion, err := core.ParseSemanticVersion(m.SemanticVersion)
	if err != nil || semanticVersion.String() != m.SemanticVersion {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidSemanticVersion, "component semantic version must be canonical SemVer")
	}
	if err := m.ArtifactDigest.Validate(); err != nil {
		return err
	}
	if err := validateContractBindingV2(m.Contract); err != nil {
		return err
	}
	if !validLocalityV2(m.Locality) || !validConformance(m.Conformance) || !validResidualClassV2(m.ResidualClass) || !validOfflinePolicyV2(m.OfflinePolicy) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "manifest locality, conformance, residual class and offline policy must be explicit")
	}
	if err := validateManifestSetLimits(m); err != nil {
		return err
	}
	knownSchemas, err := validateSchemaSetV2(m.Schemas)
	if err != nil {
		return err
	}
	dependencies := make(map[ComponentIDV2]bool, len(m.Dependencies))
	for _, dependency := range m.Dependencies {
		if err := ValidateNamespacedNameV2(NamespacedNameV2(dependency.ComponentID)); err != nil {
			return err
		}
		if dependency.ComponentID == m.ComponentID {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonDependencyCycle, "component cannot depend on itself")
		}
		if _, exists := dependencies[dependency.ComponentID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "component dependency set contains a duplicate")
		}
		dependencies[dependency.ComponentID] = dependency.Optional
	}
	requiredCapabilities := make(map[string]struct{}, len(m.RequiredCapabilities))
	for _, requirement := range m.RequiredCapabilities {
		if err := ValidateNamespacedNameV2(NamespacedNameV2(requirement.Capability)); err != nil {
			return err
		}
		if err := ValidateNamespacedNameV2(NamespacedNameV2(requirement.ProviderComponent)); err != nil {
			return err
		}
		key := string(requirement.ProviderComponent) + "\x00" + string(requirement.Capability)
		if _, exists := requiredCapabilities[key]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonUnknownCapability, "required capability set contains a duplicate")
		}
		requiredCapabilities[key] = struct{}{}
		dependencyOptional, exists := dependencies[requirement.ProviderComponent]
		if !exists || dependencyOptional != requirement.Optional {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "capability dependency requires a component dependency with identical optionality")
		}
	}
	providedCapabilities := make(map[CapabilityNameV2]struct{}, len(m.ProvidedCapabilities))
	for _, capability := range m.ProvidedCapabilities {
		if err := ValidateNamespacedNameV2(NamespacedNameV2(capability.Capability)); err != nil {
			return err
		}
		if capability.TTLSeconds == 0 || capability.TTLSeconds > MaxCapabilityTTLSeconds {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonCapabilityExpired, "provided capability TTL is zero or exceeds the bounded maximum")
		}
		if _, exists := providedCapabilities[capability.Capability]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonUnknownCapability, "provided capability set contains a duplicate")
		}
		providedCapabilities[capability.Capability] = struct{}{}
		if _, err := validateSchemaSetV2(capability.Schemas); err != nil {
			return err
		}
		for _, schema := range capability.Schemas {
			if _, exists := knownSchemas[schema.Key()]; !exists {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownSchema, "capability schema is absent from the manifest schema set")
			}
		}
	}
	if err := validateOwnersV2(m.Owners); err != nil {
		return err
	}
	if err := validateCredentialsV2(m.Credentials); err != nil {
		return err
	}
	if err := validateExtensionsV2(m.Extensions); err != nil {
		return err
	}
	return validateAnnotationsV2(m.Annotations)
}

func (m ComponentManifestV2) BindingDigestV2() (core.Digest, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	normalized := normalizeManifestV2(m)
	normalized.Annotations = []DisplayAnnotationV2{}
	return core.CanonicalJSONDigest("praxis.runtime.binding", BindingContractVersionV2, "ComponentManifestV2", normalized)
}

func DecodeComponentManifestV2(payload []byte) (ComponentManifestV2, error) {
	var manifest ComponentManifestV2
	if err := core.DecodeStrictJSON(payload, &manifest); err != nil {
		return ComponentManifestV2{}, err
	}
	if err := manifest.Validate(); err != nil {
		return ComponentManifestV2{}, err
	}
	return manifest, nil
}

func (c GovernanceCatalogV2) Validate() error {
	if len(c.Registrations) == 0 || len(c.Registrations) > MaxManifestSetEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "governance catalog must contain a bounded registration set")
	}
	seen := make(map[ComponentKindV2]struct{}, len(c.Registrations))
	for _, registration := range c.Registrations {
		if err := registration.Validate(); err != nil {
			return err
		}
		if _, exists := seen[registration.Kind]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonUnknownGovernanceCategory, "governance catalog contains a duplicate kind")
		}
		seen[registration.Kind] = struct{}{}
	}
	return nil
}

func (c GovernanceCatalogV2) DigestV2() (core.Digest, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	normalized := c
	normalized.Registrations = append([]GovernanceRegistrationV2(nil), c.Registrations...)
	for index := range normalized.Registrations {
		normalized.Registrations[index] = normalizeRegistrationV2(normalized.Registrations[index])
	}
	sort.Slice(normalized.Registrations, func(i, j int) bool { return normalized.Registrations[i].Kind < normalized.Registrations[j].Kind })
	return core.CanonicalJSONDigest("praxis.runtime.binding", BindingContractVersionV2, "GovernanceCatalogV2", normalized)
}

func (r GovernanceRegistrationV2) Validate() error {
	if err := ValidateNamespacedNameV2(NamespacedNameV2(r.Kind)); err != nil {
		return err
	}
	if err := ValidateNamespacedNameV2(NamespacedNameV2(r.Category)); err != nil {
		return err
	}
	if len(r.Capabilities) > MaxManifestSetEntries || len(r.Schemas) > MaxManifestSetEntries || len(r.ExtensionPolicies) > MaxManifestSetEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "governance registration set exceeds its bound")
	}
	if err := validateNamespacedSetV2(r.Capabilities); err != nil {
		return err
	}
	if _, err := validateSchemaSetV2(r.Schemas); err != nil {
		return err
	}
	extensions := make([]NamespacedNameV2, 0, len(r.ExtensionPolicies))
	for _, policy := range r.ExtensionPolicies {
		extensions = append(extensions, policy.Key)
	}
	if err := validateNamespacedSetV2(extensions); err != nil {
		return err
	}
	if len(r.AllowedLocalities) == 0 || len(r.AllowedConformance) == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownGovernanceCategory, "governance registration must explicitly allow locality and conformance")
	}
	localities := make(map[LocalityV2]struct{}, len(r.AllowedLocalities))
	for _, locality := range r.AllowedLocalities {
		if !validLocalityV2(locality) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "governance registration contains an unknown locality")
		}
		if _, exists := localities[locality]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "governance locality set contains a duplicate")
		}
		localities[locality] = struct{}{}
	}
	conformance := make(map[ConformanceLevel]struct{}, len(r.AllowedConformance))
	for _, level := range r.AllowedConformance {
		if !validConformance(level) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "governance registration contains an unknown conformance level")
		}
		if _, exists := conformance[level]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "governance conformance set contains a duplicate")
		}
		conformance[level] = struct{}{}
	}
	return nil
}

func ValidateManifestAgainstCatalogV2(manifest ComponentManifestV2, catalog GovernanceCatalogV2) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	if err := catalog.Validate(); err != nil {
		return err
	}
	var registration *GovernanceRegistrationV2
	for index := range catalog.Registrations {
		if catalog.Registrations[index].Kind == manifest.Kind {
			registration = &catalog.Registrations[index]
			break
		}
	}
	if registration == nil || registration.Category != manifest.GovernanceCategory {
		return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownGovernanceCategory, "component kind has no matching governance registration")
	}
	if !containsLocality(registration.AllowedLocalities, manifest.Locality) || !containsConformance(registration.AllowedConformance, manifest.Conformance) {
		return core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "manifest locality or conformance is not allowed by its governance registration")
	}
	knownCapabilities := make(map[CapabilityNameV2]struct{}, len(registration.Capabilities))
	for _, capability := range registration.Capabilities {
		knownCapabilities[capability] = struct{}{}
	}
	for _, capability := range manifest.ProvidedCapabilities {
		if _, exists := knownCapabilities[capability.Capability]; !exists {
			return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownCapability, "manifest declares a capability outside its governance registration")
		}
	}
	knownSchemas := make(map[string]struct{}, len(registration.Schemas))
	for _, schema := range registration.Schemas {
		knownSchemas[schema.Key()] = struct{}{}
	}
	for _, schema := range manifest.Schemas {
		if _, exists := knownSchemas[schema.Key()]; !exists {
			return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownSchema, "manifest uses a schema outside its governance registration")
		}
	}
	knownExtensions := make(map[NamespacedNameV2]struct{}, len(registration.ExtensionPolicies))
	for _, extension := range registration.ExtensionPolicies {
		knownExtensions[extension.Key] = struct{}{}
	}
	for _, extension := range manifest.Extensions {
		if _, exists := knownExtensions[extension.Key]; !exists && extension.Required {
			return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownRequiredExtension, "required governance extension is unknown")
		}
	}
	return nil
}

func (p BindingPlanV2) Validate() error {
	if err := validateBindingPlanStructureV2(p); err != nil {
		return err
	}
	digest, err := bindingPlanDigestV2(p)
	if err != nil || digest != p.PlanDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "binding plan digest is missing or drifted from its canonical requirements")
	}
	return nil
}

func validateBindingPlanStructureV2(p BindingPlanV2) error {
	if p.ID == "" || len(p.ID) > 128 || strings.TrimSpace(p.ID) != p.ID || len(p.Requirements) == 0 || len(p.Requirements) > MaxManifestSetEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "binding plan identity and bounded requirements are required")
	}
	if err := p.PlanDigest.Validate(); err != nil {
		return err
	}
	if err := p.GovernanceDigest.Validate(); err != nil {
		return err
	}
	seen := make(map[ComponentIDV2]struct{}, len(p.Requirements))
	for _, requirement := range p.Requirements {
		if err := ValidateNamespacedNameV2(NamespacedNameV2(requirement.ComponentID)); err != nil {
			return err
		}
		if err := ValidateNamespacedNameV2(NamespacedNameV2(requirement.Kind)); err != nil {
			return err
		}
		if err := requirement.SemanticVersion.Validate(); err != nil {
			return err
		}
		if err := ValidateNamespacedNameV2(requirement.ContractName); err != nil {
			return err
		}
		if err := requirement.Contract.Validate(); err != nil {
			return err
		}
		if err := requirement.ArtifactDigest.Validate(); err != nil {
			return err
		}
		if !requirement.Required && !requirement.AllowResidual {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "optional binding requirement must explicitly allow residual")
		}
		if err := validateNamespacedSetV2(requirement.RequiredCapabilities); err != nil {
			return err
		}
		if _, exists := seen[requirement.ComponentID]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "binding plan contains a duplicate component")
		}
		seen[requirement.ComponentID] = struct{}{}
	}
	return nil
}

// BindingPlanDigestV2 returns the canonical semantic identity of a sealed
// Binding Plan. Requirements and required-capability lists are sets: order is
// normalized, duplicates are rejected, and nil/empty capability sets are
// equivalent. PlanDigest itself is excluded from the hash.
func BindingPlanDigestV2(p BindingPlanV2) (core.Digest, error) {
	p.PlanDigest = EvidenceGenesisDigestV2
	if err := validateBindingPlanStructureV2(p); err != nil {
		return "", err
	}
	return bindingPlanDigestV2(p)
}

func SealBindingPlanV2(p BindingPlanV2) (BindingPlanV2, error) {
	p.PlanDigest = EvidenceGenesisDigestV2
	if err := validateBindingPlanStructureV2(p); err != nil {
		return BindingPlanV2{}, err
	}
	digest, err := bindingPlanDigestV2(p)
	if err != nil {
		return BindingPlanV2{}, err
	}
	p.PlanDigest = digest
	return p, p.Validate()
}

func bindingPlanDigestV2(p BindingPlanV2) (core.Digest, error) {
	p.PlanDigest = ""
	p.Requirements = append([]BindingRequirementV2{}, p.Requirements...)
	for index := range p.Requirements {
		p.Requirements[index].RequiredCapabilities = append([]CapabilityNameV2{}, p.Requirements[index].RequiredCapabilities...)
		if p.Requirements[index].RequiredCapabilities == nil {
			p.Requirements[index].RequiredCapabilities = []CapabilityNameV2{}
		}
		sort.Slice(p.Requirements[index].RequiredCapabilities, func(i, j int) bool {
			return p.Requirements[index].RequiredCapabilities[i] < p.Requirements[index].RequiredCapabilities[j]
		})
	}
	sort.Slice(p.Requirements, func(i, j int) bool { return p.Requirements[i].ComponentID < p.Requirements[j].ComponentID })
	return core.CanonicalJSONDigest("praxis.runtime.binding", BindingContractVersionV2, "BindingPlanV2", p)
}

func DecodeGovernanceCatalogV2(payload []byte) (GovernanceCatalogV2, error) {
	var catalog GovernanceCatalogV2
	if err := core.DecodeStrictJSON(payload, &catalog); err != nil {
		return GovernanceCatalogV2{}, err
	}
	if err := catalog.Validate(); err != nil {
		return GovernanceCatalogV2{}, err
	}
	return catalog, nil
}

func validateContractBindingV2(contract ContractBindingV2) error {
	if err := ValidateNamespacedNameV2(contract.Name); err != nil {
		return err
	}
	version, err := core.ParseSemanticVersion(contract.Version)
	if err != nil || version.String() != contract.Version {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidSemanticVersion, "contract version must be canonical SemVer")
	}
	if err := contract.Compatible.Validate(); err != nil {
		return err
	}
	if !contract.Compatible.Contains(contract.Version) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidSemanticVersion, "contract version is outside its declared compatibility range")
	}
	return nil
}

// Validate exposes the complete Contract binding validator to versioned Fact
// owners. A BindingSet must not rely on a previously validated Manifest after
// persistence or renewal.
func (contract ContractBindingV2) Validate() error {
	return validateContractBindingV2(contract)
}

// ValidateOwnerAssignmentsV2 validates the canonical one-owner-per-role set.
func ValidateOwnerAssignmentsV2(owners []OwnerAssignmentV2) error {
	return validateOwnersV2(owners)
}

// ValidateCapabilityGrantStructureV2 validates a persisted grant set without
// trusting an earlier Manifest validation. Manifest declaration matching is
// additionally performed by the Binding Fact owner.
func ValidateCapabilityGrantStructureV2(grants []CapabilityGrantV2) error {
	if len(grants) == 0 || len(grants) > MaxManifestSetEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownCapability, "bounded capability grants are required")
	}
	var previous CapabilityNameV2
	for index, grant := range grants {
		if err := ValidateNamespacedNameV2(NamespacedNameV2(grant.Capability)); err != nil {
			return err
		}
		if index > 0 && grant.Capability <= previous {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "capability grants must be sorted and unique")
		}
		previous = grant.Capability
		if err := grant.EvidenceDigest.Validate(); err != nil {
			return err
		}
		if grant.ObservedUnixNano <= 0 || grant.ExpiresUnixNano <= grant.ObservedUnixNano || grant.ExpiresUnixNano-grant.ObservedUnixNano > int64(MaxCapabilityTTLSeconds)*int64(time.Second) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "capability grant has an invalid bounded TTL")
		}
	}
	return nil
}

func validateManifestSetLimits(m ComponentManifestV2) error {
	counts := []int{len(m.Schemas), len(m.Dependencies), len(m.RequiredCapabilities), len(m.ProvidedCapabilities), len(m.Owners), len(m.Credentials), len(m.Extensions), len(m.Annotations)}
	for _, count := range counts {
		if count > MaxManifestSetEntries {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "manifest set exceeds its bounded entry count")
		}
	}
	return nil
}

func validateSchemaSetV2(schemas []SchemaRefV2) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(schemas))
	for _, schema := range schemas {
		if err := schema.Validate(); err != nil {
			return nil, err
		}
		if _, exists := seen[schema.Key()]; exists {
			return nil, core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "schema set contains a duplicate")
		}
		seen[schema.Key()] = struct{}{}
	}
	return seen, nil
}

func validateOwnersV2(owners []OwnerAssignmentV2) error {
	if len(owners) != 3 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerMissing, "effect, settlement and cleanup each require exactly one owner")
	}
	seen := make(map[OwnerRoleV2]struct{}, 3)
	for _, owner := range owners {
		if owner.Role != OwnerEffect && owner.Role != OwnerSettlement && owner.Role != OwnerCleanup {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonOwnerConflict, "owner role is unknown")
		}
		if err := ValidateNamespacedNameV2(NamespacedNameV2(owner.OwnerComponentID)); err != nil {
			return err
		}
		if _, exists := seen[owner.Role]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "owner role has multiple owners")
		}
		seen[owner.Role] = struct{}{}
	}
	return nil
}

func validateCredentialsV2(credentials []CredentialRequirementV2) error {
	seen := make(map[NamespacedNameV2]struct{}, len(credentials))
	for _, credential := range credentials {
		if err := ValidateNamespacedNameV2(credential.CredentialClass); err != nil {
			return err
		}
		if err := credential.ScopeDigest.Validate(); err != nil {
			return err
		}
		if credential.MaximumTTLSeconds == 0 || credential.MaximumTTLSeconds > MaxCapabilityTTLSeconds {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonCapabilityExpired, "credential maximum TTL is zero or exceeds the bounded maximum")
		}
		if _, exists := seen[credential.CredentialClass]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "credential requirement set contains a duplicate")
		}
		seen[credential.CredentialClass] = struct{}{}
	}
	return nil
}

func validateExtensionsV2(extensions []GovernanceExtensionV2) error {
	seen := make(map[NamespacedNameV2]struct{}, len(extensions))
	for _, extension := range extensions {
		if err := ValidateNamespacedNameV2(extension.Key); err != nil {
			return err
		}
		if err := extension.Payload.Validate(); err != nil {
			return err
		}
		if _, exists := seen[extension.Key]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "governance extension set contains a duplicate")
		}
		seen[extension.Key] = struct{}{}
	}
	return nil
}

func validateAnnotationsV2(annotations []DisplayAnnotationV2) error {
	seen := make(map[string]struct{}, len(annotations))
	for _, annotation := range annotations {
		if annotation.Key == "" || len(annotation.Key) > 128 || len(annotation.Value) > 4096 || strings.TrimSpace(annotation.Key) != annotation.Key {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "display annotation is empty, padded or too long")
		}
		if _, exists := seen[annotation.Key]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "display annotation set contains a duplicate")
		}
		seen[annotation.Key] = struct{}{}
	}
	return nil
}

func normalizeManifestV2(m ComponentManifestV2) ComponentManifestV2 {
	normalized := m
	normalized.Schemas = append([]SchemaRefV2{}, m.Schemas...)
	normalized.Dependencies = append([]ComponentDependencyV2{}, m.Dependencies...)
	normalized.RequiredCapabilities = append([]CapabilityRequirementV2{}, m.RequiredCapabilities...)
	normalized.ProvidedCapabilities = append([]ProvidedCapabilityV2{}, m.ProvidedCapabilities...)
	normalized.Owners = append([]OwnerAssignmentV2{}, m.Owners...)
	normalized.Credentials = append([]CredentialRequirementV2{}, m.Credentials...)
	normalized.Extensions = append([]GovernanceExtensionV2{}, m.Extensions...)
	normalized.Annotations = append([]DisplayAnnotationV2{}, m.Annotations...)
	for index := range normalized.ProvidedCapabilities {
		normalized.ProvidedCapabilities[index].Schemas = append([]SchemaRefV2{}, normalized.ProvidedCapabilities[index].Schemas...)
		sort.Slice(normalized.ProvidedCapabilities[index].Schemas, func(i, j int) bool {
			return normalized.ProvidedCapabilities[index].Schemas[i].Key() < normalized.ProvidedCapabilities[index].Schemas[j].Key()
		})
	}
	sort.Slice(normalized.Schemas, func(i, j int) bool { return normalized.Schemas[i].Key() < normalized.Schemas[j].Key() })
	sort.Slice(normalized.Dependencies, func(i, j int) bool {
		return normalized.Dependencies[i].ComponentID < normalized.Dependencies[j].ComponentID
	})
	sort.Slice(normalized.RequiredCapabilities, func(i, j int) bool {
		left := string(normalized.RequiredCapabilities[i].ProviderComponent) + "\x00" + string(normalized.RequiredCapabilities[i].Capability)
		right := string(normalized.RequiredCapabilities[j].ProviderComponent) + "\x00" + string(normalized.RequiredCapabilities[j].Capability)
		return left < right
	})
	sort.Slice(normalized.ProvidedCapabilities, func(i, j int) bool {
		return normalized.ProvidedCapabilities[i].Capability < normalized.ProvidedCapabilities[j].Capability
	})
	sort.Slice(normalized.Owners, func(i, j int) bool { return normalized.Owners[i].Role < normalized.Owners[j].Role })
	sort.Slice(normalized.Credentials, func(i, j int) bool {
		return normalized.Credentials[i].CredentialClass < normalized.Credentials[j].CredentialClass
	})
	sort.Slice(normalized.Extensions, func(i, j int) bool { return normalized.Extensions[i].Key < normalized.Extensions[j].Key })
	sort.Slice(normalized.Annotations, func(i, j int) bool { return normalized.Annotations[i].Key < normalized.Annotations[j].Key })
	return normalized
}

func normalizeRegistrationV2(r GovernanceRegistrationV2) GovernanceRegistrationV2 {
	normalized := r
	normalized.Capabilities = append([]CapabilityNameV2{}, r.Capabilities...)
	normalized.Schemas = append([]SchemaRefV2{}, r.Schemas...)
	normalized.ExtensionPolicies = append([]ExtensionPolicyV2{}, r.ExtensionPolicies...)
	normalized.AllowedLocalities = append([]LocalityV2{}, r.AllowedLocalities...)
	normalized.AllowedConformance = append([]ConformanceLevel{}, r.AllowedConformance...)
	sort.Slice(normalized.Capabilities, func(i, j int) bool { return normalized.Capabilities[i] < normalized.Capabilities[j] })
	sort.Slice(normalized.Schemas, func(i, j int) bool { return normalized.Schemas[i].Key() < normalized.Schemas[j].Key() })
	sort.Slice(normalized.ExtensionPolicies, func(i, j int) bool { return normalized.ExtensionPolicies[i].Key < normalized.ExtensionPolicies[j].Key })
	sort.Slice(normalized.AllowedLocalities, func(i, j int) bool { return normalized.AllowedLocalities[i] < normalized.AllowedLocalities[j] })
	sort.Slice(normalized.AllowedConformance, func(i, j int) bool { return normalized.AllowedConformance[i] < normalized.AllowedConformance[j] })
	return normalized
}

func validateNamespacedSetV2[T ~string](values []T) error {
	seen := make(map[T]struct{}, len(values))
	for _, value := range values {
		if err := ValidateNamespacedNameV2(NamespacedNameV2(value)); err != nil {
			return err
		}
		if _, exists := seen[value]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "namespaced set contains a duplicate")
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validLowerASCIIName(value string, namespace bool) bool {
	if value == "" || len(value) > 63 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	last := value[len(value)-1]
	if !((last >= 'a' && last <= 'z') || (last >= '0' && last <= '9')) {
		return false
	}
	for _, character := range []byte(value) {
		allowed := (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-'
		if namespace {
			allowed = allowed || character == '.'
		} else {
			allowed = allowed || character == '_' || character == '.'
		}
		if !allowed {
			return false
		}
	}
	return true
}

func validMediaType(value string) bool {
	if value == "" || len(value) > 128 || value != strings.ToLower(value) || strings.Count(value, "/") != 1 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range []byte(value) {
		if !((character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || strings.ContainsRune("!#$&^_.+-/", rune(character))) {
			return false
		}
	}
	return true
}

func validLocalityV2(value LocalityV2) bool {
	switch value {
	case LocalityHostControlPlane, LocalityInstanceDataPlane, LocalityExternalStatePlane, LocalityRemoteProvider:
		return true
	default:
		return false
	}
}

func validResidualClassV2(value ResidualClassV2) bool {
	switch value {
	case ResidualNone, ResidualInspectable, ResidualCompensatable, ResidualExternallyOwned, ResidualPotentiallyStale:
		return true
	default:
		return false
	}
}

func validOfflinePolicyV2(value OfflinePolicyModeV2) bool {
	return value == OfflineDenied || value == OfflineObserveOnly || value == OfflineCachedAuthorityOnly
}

func validConformance(value ConformanceLevel) bool {
	switch value {
	case ConformanceFullyControlled, ConformanceRestrictedControlled, ConformanceContainedObserveOnly, ConformanceRejected:
		return true
	default:
		return false
	}
}

func containsLocality(values []LocalityV2, target LocalityV2) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsConformance(values []ConformanceLevel, target ConformanceLevel) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// EncodeComponentManifestV2 preserves every contract-defined extension. It is
// not a canonical representation; BindingDigestV2 is the sole identity hash.
func EncodeComponentManifestV2(manifest ComponentManifestV2) ([]byte, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(manifest)
	if err != nil || len(payload) > core.MaxCanonicalDocumentBytes {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "manifest encoding exceeds its bound")
	}
	return bytes.Clone(payload), nil
}

func expiryFromGrant(grant CapabilityGrantV2) time.Time {
	return time.Unix(0, grant.ExpiresUnixNano)
}
