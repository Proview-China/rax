package assemblycontract

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func validateContract(version string) error {
	if version != ContractVersionV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMismatch, "assembly contract version is not supported")
	}
	return nil
}

func validateID(value string) error {
	if value == "" || len(value) > MaxReferenceBytes || strings.TrimSpace(value) != value {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "assembly reference is empty, padded or too long")
	}
	for _, character := range []byte(value) {
		if character < 0x21 || character > 0x7e {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "assembly reference must use visible ASCII")
		}
	}
	return nil
}

func validateMutationPath(value string) error {
	if err := validateID(value); err != nil {
		return err
	}
	if value == "candidate.declared-write-set" || strings.Contains(value, "*") || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") || !strings.Contains(value, ".") {
		return core.NewError(core.ErrorForbidden, core.ReasonPlanInvalid, "mutation path must be one exact bounded field path, not a wildcard or sentinel")
	}
	for _, part := range strings.Split(value, ".") {
		if part == "" {
			return core.NewError(core.ErrorForbidden, core.ReasonPlanInvalid, "mutation path contains an empty segment")
		}
		for _, character := range []byte(part) {
			if !((character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' || character == '_') {
				return core.NewError(core.ErrorForbidden, core.ReasonPlanInvalid, "mutation path must use canonical lowercase field segments")
			}
		}
	}
	return nil
}

func (r ObjectRefV1) Validate() error {
	if err := validateID(r.ID); err != nil {
		return err
	}
	if r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "assembly reference revision is required")
	}
	return r.Digest.Validate()
}

func (r CompatibilityV1) Validate() error {
	return (runtimeports.VersionRangeV2{MinimumInclusive: r.MinimumInclusive, MaximumExclusive: r.MaximumExclusive}).Validate()
}

func validateRefs(refs []ObjectRefV1, required bool) error {
	if required && len(refs) == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "required assembly reference set is empty")
	}
	if len(refs) > MaxAssemblyEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "assembly reference set exceeds its bound")
	}
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if err := ref.Validate(); err != nil {
			return err
		}
		if _, ok := seen[ref.ID]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "assembly reference set contains a duplicate id")
		}
		seen[ref.ID] = struct{}{}
	}
	return nil
}

func (p AssemblyPlanRefsV1) Validate() error {
	refs := []ObjectRefV1{p.ResolvedAgentPlan, p.HarnessBootstrapPlan, p.Profile, p.RuntimePolicy, p.HarnessStack, p.SemanticRoute, p.ContextPlan, p.ToolSurface, p.CapabilityGrant, p.ExpectedInjectionManifest}
	for _, ref := range refs {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (m ModuleDescriptorV1) Validate() error {
	if err := validateContract(m.ContractVersion); err != nil {
		return err
	}
	for _, value := range []string{m.ModuleID, m.Namespace, m.SemanticVersion} {
		if err := validateID(value); err != nil {
			return err
		}
	}
	version, err := core.ParseSemanticVersion(m.SemanticVersion)
	if err != nil || version.String() != m.SemanticVersion {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidSemanticVersion, "module semantic version must be canonical SemVer")
	}
	if err := m.ArtifactDigest.Validate(); err != nil {
		return err
	}
	for _, ref := range []ObjectRefV1{m.PublisherRef, m.SourceRef, m.ComponentManifestRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if err := m.Compatibility.Validate(); err != nil {
		return err
	}
	if len(m.Capabilities) > MaxAssemblyEntries || len(m.Schemas) > MaxAssemblyEntries || len(m.Owners) == 0 || len(m.Owners) > MaxAssemblyEntries || len(m.CredentialRequirements) > MaxAssemblyEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "module descriptor set exceeds its bound or has no owner")
	}
	capabilitySet := map[runtimeports.CapabilityNameV2]struct{}{}
	for _, capability := range m.Capabilities {
		if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(capability)); err != nil {
			return err
		}
		if _, exists := capabilitySet[capability]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "module capability is duplicated")
		}
		capabilitySet[capability] = struct{}{}
	}
	schemaSet := map[string]struct{}{}
	for _, schema := range m.Schemas {
		if err := schema.Validate(); err != nil {
			return err
		}
		if _, exists := schemaSet[schema.Key()]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "module schema is duplicated")
		}
		schemaSet[schema.Key()] = struct{}{}
	}
	ownerSet := map[string]struct{}{}
	for _, owner := range m.Owners {
		if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(owner.OwnerComponentID)); err != nil {
			return err
		}
		if owner.Role != runtimeports.OwnerEffect && owner.Role != runtimeports.OwnerSettlement && owner.Role != runtimeports.OwnerCleanup {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonOwnerMissing, "module owner role is invalid")
		}
		key := string(owner.Role) + "\x00" + string(owner.OwnerComponentID)
		if _, exists := ownerSet[key]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "module owner is duplicated")
		}
		ownerSet[key] = struct{}{}
	}
	credentialSet := map[runtimeports.NamespacedNameV2]struct{}{}
	for _, credential := range m.CredentialRequirements {
		if err := runtimeports.ValidateNamespacedNameV2(credential); err != nil {
			return err
		}
		if _, exists := credentialSet[credential]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "module credential requirement is duplicated")
		}
		credentialSet[credential] = struct{}{}
	}
	return nil
}

func (c CapabilityDescriptorV1) Validate() error {
	if err := validateContract(c.ContractVersion); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(c.Capability)); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(c.OwnerCapability)); err != nil {
		return err
	}
	version, err := core.ParseSemanticVersion(c.Version)
	if err != nil || version.String() != c.Version {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidSemanticVersion, "capability version must be canonical SemVer")
	}
	if !c.Required && !c.Provided {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownCapability, "capability must be required or provided")
	}
	if c.TTLSeconds == 0 || c.TTLSeconds > runtimeports.MaxCapabilityTTLSeconds {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCapabilityExpired, "capability TTL is invalid")
	}
	if err := validateID(c.EffectClass); err != nil {
		return err
	}
	if c.Conformance != runtimeports.ConformanceFullyControlled && c.Conformance != runtimeports.ConformanceRestrictedControlled && c.Conformance != runtimeports.ConformanceContainedObserveOnly && c.Conformance != runtimeports.ConformanceRejected {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMismatch, "capability conformance is invalid")
	}
	if len(c.Schemas) == 0 || len(c.Schemas) > MaxAssemblyEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownSchema, "capability schema set is empty or too large")
	}
	seenSchemas := map[string]struct{}{}
	for _, schema := range c.Schemas {
		if err := schema.Validate(); err != nil {
			return err
		}
		if _, exists := seenSchemas[schema.Key()]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "capability schema is duplicated")
		}
		seenSchemas[schema.Key()] = struct{}{}
	}
	return nil
}

func validLifecycle(value LifecycleScopeV1) bool {
	return value == LifecycleGenerationV1 || value == LifecycleInstanceV1 || value == LifecycleRunV1 || value == LifecycleSessionV1
}

func validCardinality(value CardinalityV1) bool {
	return value == CardinalityExactlyOneV1 || value == CardinalityZeroOrOneV1 || value == CardinalityZeroOrManyV1 || value == CardinalityOwnerSourcesV1 || value == CardinalityActiveBindingV1
}

func validSlotKind(value SlotContributionKindV1) bool {
	return value == SlotContributionOwnerV1 || value == SlotContributionSourceV1 || value == SlotContributionProviderV1 || value == SlotContributionReferenceV1
}

func validPhaseCapability(value PhaseCapabilityV1) bool {
	return value == PhaseObserverV1 || value == PhaseFilterV1 || value == PhaseGateV1 || value == PhasePortV1
}

func (s SlotSpecV1) Validate() error {
	if err := validateContract(s.ContractVersion); err != nil {
		return err
	}
	if err := validateID(s.SlotID); err != nil {
		return err
	}
	if !validLifecycle(s.LifecycleScope) || !validCardinality(s.Cardinality) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "slot lifecycle or cardinality is invalid")
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(s.OwnerCapability)); err != nil {
		return err
	}
	if len(s.ContributionKinds) == 0 || len(s.ContributionKinds) > 4 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "slot contribution kind set is empty or too large")
	}
	seen := map[SlotContributionKindV1]struct{}{}
	for _, kind := range s.ContributionKinds {
		if !validSlotKind(kind) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "slot contribution kind is invalid")
		}
		if _, ok := seen[kind]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "slot contribution kind is duplicated")
		}
		seen[kind] = struct{}{}
	}
	if err := s.InputSchema.Validate(); err != nil {
		return err
	}
	if err := s.OutputSchema.Validate(); err != nil {
		return err
	}
	for _, value := range []string{s.EffectClass, s.ConcurrencyPolicy, s.FailurePolicy, s.DegradationPolicy} {
		if err := validateID(value); err != nil {
			return err
		}
	}
	if len(s.Dependencies) > MaxAssemblyEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "slot dependency set exceeds its bound")
	}
	for _, dependency := range s.Dependencies {
		if err := validateID(dependency); err != nil {
			return err
		}
	}
	if err := s.Digest.Validate(); err != nil {
		return err
	}
	digest, err := SlotSpecDigestV1(s)
	if err != nil {
		return err
	}
	if digest != s.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "slot digest does not match canonical content")
	}
	return nil
}

func (c SlotContributionV1) Validate() error {
	if err := validateContract(c.ContractVersion); err != nil {
		return err
	}
	for _, value := range []string{c.ContributionID, c.ModuleRef, c.SlotRef} {
		if err := validateID(value); err != nil {
			return err
		}
	}
	if !validSlotKind(c.Kind) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "slot contribution kind is invalid")
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(c.CapabilityRef)); err != nil {
		return err
	}
	if c.PortSpecRef != "" {
		if err := validateID(c.PortSpecRef); err != nil {
			return err
		}
	}
	if c.ProviderCandidateRef != "" {
		if err := validateID(c.ProviderCandidateRef); err != nil {
			return err
		}
	}
	if len(c.Dependencies) > MaxAssemblyEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "slot contribution dependency set exceeds its bound")
	}
	for _, dependency := range c.Dependencies {
		if err := validateID(dependency); err != nil {
			return err
		}
	}
	if err := c.Digest.Validate(); err != nil {
		return err
	}
	digest, err := SlotContributionDigestV1(c)
	if err != nil {
		return err
	}
	if digest != c.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "slot contribution digest does not match canonical content")
	}
	return nil
}

func (p PortSpecV1) Validate() error {
	if err := validateContract(p.ContractVersion); err != nil {
		return err
	}
	if err := validateID(p.PortID); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(p.OwnerCapability)); err != nil {
		return err
	}
	if err := p.RequestSchema.Validate(); err != nil {
		return err
	}
	if err := p.ResponseSchema.Validate(); err != nil {
		return err
	}
	for _, value := range []string{p.OperationClass, p.Idempotency, p.FailureSemantics} {
		if err := validateID(value); err != nil {
			return err
		}
	}
	if err := p.Compatibility.Validate(); err != nil {
		return err
	}
	if len(p.RunStartRequirementRefs) > MaxAssemblyEntries || len(p.RunSettlementRequirementRefs) > MaxAssemblyEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "typed Run requirement set exceeds its bound")
	}
	startSeen := map[string]struct{}{}
	for _, ref := range p.RunStartRequirementRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
		key := string(ref.RequirementID) + "\x00" + ref.Ref.ID
		if _, exists := startSeen[key]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Run Start requirement is duplicated")
		}
		startSeen[key] = struct{}{}
	}
	settlementSeen := map[string]struct{}{}
	for _, ref := range p.RunSettlementRequirementRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
		key := string(ref.RequirementID) + "\x00" + ref.Ref.ID
		if _, exists := settlementSeen[key]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Run Settlement requirement is duplicated")
		}
		settlementSeen[key] = struct{}{}
	}
	if p.EffectKind == "" {
		if p.ConflictDomainRule != "" || p.Governance != (GovernanceRequirementsV1{}) || p.CancelSupported || p.OperationScopeRef != nil || p.InspectContractRef != nil || p.DomainResultContractRef != nil || p.RuntimeOperationSettlementRefContract != nil || p.ApplySettlementContractRef != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "effect-free port cannot declare effect recovery state")
		}
		return nil
	}
	if err := runtimeports.ValidateNamespacedNameV2(p.EffectKind); err != nil {
		return err
	}
	if err := validateID(p.ConflictDomainRule); err != nil {
		return err
	}
	if !p.Governance.FenceRequired || !p.Governance.AuthorityRequired || !p.Governance.ScopeRequired || !p.Governance.BudgetRequired {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "effectful port lacks governance requirements")
	}
	if p.OperationScopeRef == nil || p.InspectContractRef == nil || p.DomainResultContractRef == nil || p.RuntimeOperationSettlementRefContract == nil || p.ApplySettlementContractRef == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "effectful port requires OperationScope, Inspect, DomainResult, Runtime Settlement ref and ApplySettlement contracts")
	}
	if err := p.OperationScopeRef.Validate(); err != nil {
		return err
	}
	if err := p.InspectContractRef.Validate(); err != nil {
		return err
	}
	if err := p.DomainResultContractRef.Validate(); err != nil {
		return err
	}
	if err := p.RuntimeOperationSettlementRefContract.Validate(); err != nil {
		return err
	}
	if err := p.ApplySettlementContractRef.Validate(); err != nil {
		return err
	}
	if p.InspectContractRef.OwnerCapability != p.OwnerCapability || p.DomainResultContractRef.OwnerCapability != p.OwnerCapability || p.ApplySettlementContractRef.OwnerCapability != p.OwnerCapability {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonSettlementOwnerMismatch, "effect contracts do not preserve the PortSpec domain owner")
	}
	if p.DomainResultContractRef.Schema.Key() != p.ResponseSchema.Key() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownSchema, "DomainResult contract schema must equal the Port response schema")
	}
	return nil
}

func (r OperationScopeRefV1) Validate() error {
	if err := r.Ref.Validate(); err != nil {
		return err
	}
	if r.ScopeKind != RuntimeOperationScopeKindV1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "effectful Port OperationScope must remain Runtime-owned")
	}
	return r.ScopeDigest.Validate()
}

func (r InspectContractRefV1) Validate() error {
	if err := r.Ref.Validate(); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(r.OwnerCapability)); err != nil {
		return err
	}
	if err := r.RequestSchema.Validate(); err != nil {
		return err
	}
	return r.ObservationSchema.Validate()
}

func (r DomainResultContractRefV1) Validate() error {
	if err := r.Ref.Validate(); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(r.OwnerCapability)); err != nil {
		return err
	}
	return r.Schema.Validate()
}

func (r RuntimeOperationSettlementRefContractV1) Validate() error {
	if err := r.Ref.Validate(); err != nil {
		return err
	}
	if r.RuntimeOwnerCapability != RuntimeOperationSettlementCapabilityV1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonSettlementOwnerMismatch, "Runtime Operation Settlement ref contract must remain Runtime-owned")
	}
	return r.Schema.Validate()
}

func (r ApplySettlementContractRefV1) Validate() error {
	if err := r.Ref.Validate(); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(r.OwnerCapability)); err != nil {
		return err
	}
	if err := r.RequestSchema.Validate(); err != nil {
		return err
	}
	return r.ResultSchema.Validate()
}

func (r RunStartRequirementRefV1) Validate() error {
	if err := r.Ref.Validate(); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(r.RequirementID); err != nil {
		return err
	}
	return runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(r.OwnerCapability))
}

func (r RunSettlementRequirementRefV1) Validate() error {
	if err := r.Ref.Validate(); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(r.RequirementID); err != nil {
		return err
	}
	return runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(r.OwnerCapability))
}

func (r CleanupContractRefV1) Validate() error {
	if err := r.Ref.Validate(); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(r.OwnerCapability)); err != nil {
		return err
	}
	if err := r.RequestSchema.Validate(); err != nil {
		return err
	}
	return r.ResultSchema.Validate()
}

func (h HookFaceSpecV1) Validate() error {
	if err := validateContract(h.ContractVersion); err != nil {
		return err
	}
	for _, value := range []string{h.HookFaceID, h.PhaseID, h.AuthorityCeiling, h.EffectClass, h.TimeoutPolicy, h.FailurePolicy, h.ConcurrencyPolicy, h.ReceiptPolicy} {
		if err := validateID(value); err != nil {
			return err
		}
	}
	if !validPhaseCapability(h.Kind) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "hookface capability is invalid")
	}
	if err := h.InputSchema.Validate(); err != nil {
		return err
	}
	if err := h.OutputSchema.Validate(); err != nil {
		return err
	}
	if h.Kind == PhaseFilterV1 && len(h.MutationMask) == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "filter hookface requires an explicit bounded mutation mask")
	}
	if h.Kind != PhaseFilterV1 && len(h.MutationMask) != 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "only filter hookfaces may declare a mutation mask")
	}
	if len(h.MutationMask) > MaxWriteSetEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "hookface mutation mask exceeds its bound")
	}
	for _, path := range h.MutationMask {
		if err := validateMutationPath(path); err != nil {
			return err
		}
	}
	if err := h.Digest.Validate(); err != nil {
		return err
	}
	digest, err := HookFaceSpecDigestV1(h)
	if err != nil {
		return err
	}
	if digest != h.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "hookface digest does not match canonical content")
	}
	return nil
}

func (p PhaseContributionV1) Validate() error {
	if err := validateContract(p.ContractVersion); err != nil {
		return err
	}
	for _, value := range []string{p.ContributionID, p.HookFaceRef, p.ModuleRef} {
		if err := validateID(value); err != nil {
			return err
		}
	}
	if err := p.HandlerDescriptorRef.Validate(); err != nil {
		return err
	}
	if !validPhaseCapability(p.Capability) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "phase contribution capability is invalid")
	}
	if p.Capability == PhaseFilterV1 && len(p.WriteSet) == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "filter contribution requires an explicit bounded write set")
	}
	if p.Capability != PhaseFilterV1 && len(p.WriteSet) != 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "only filter contributions may declare a write set")
	}
	if p.Async && p.Capability != PhaseObserverV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "only observer contributions may be asynchronous")
	}
	if len(p.WriteSet) > MaxWriteSetEntries || len(p.Dependencies) > MaxAssemblyEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "phase contribution set exceeds its bound")
	}
	for _, path := range p.WriteSet {
		if err := validateMutationPath(path); err != nil {
			return err
		}
	}
	for _, dependency := range p.Dependencies {
		if err := validateID(dependency); err != nil {
			return err
		}
	}
	if err := p.Digest.Validate(); err != nil {
		return err
	}
	digest, err := PhaseContributionDigestV1(p)
	if err != nil {
		return err
	}
	if digest != p.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "phase contribution digest does not match canonical content")
	}
	return nil
}

func (d DependencySpecV1) Validate() error {
	if err := validateContract(d.ContractVersion); err != nil {
		return err
	}
	for _, value := range []string{d.FromRef, d.ToRef, d.Relation, d.FailureMode} {
		if err := validateID(value); err != nil {
			return err
		}
	}
	if d.FromRef == d.ToRef {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDependencyCycle, "assembly dependency cannot target itself")
	}
	if err := d.VersionRange.Validate(); err != nil {
		return err
	}
	if d.Capability != "" {
		return runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(d.Capability))
	}
	return nil
}

func (f ModuleFactoryDescriptorV1) Validate() error {
	if err := validateContract(f.ContractVersion); err != nil {
		return err
	}
	for _, value := range []string{f.FactoryID, f.ModuleRef} {
		if err := validateID(value); err != nil {
			return err
		}
	}
	if err := f.ArtifactDigest.Validate(); err != nil {
		return err
	}
	if f.ConstructionMode != ConstructionTrustedInProcessGoV1 {
		return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownCapability, "Wave 1 only accepts static trusted in-process Go factory descriptors")
	}
	if err := f.InputSchema.Validate(); err != nil {
		return err
	}
	if err := runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(f.OutputCapability)); err != nil {
		return err
	}
	if !validLifecycle(f.Lifecycle) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "factory lifecycle is invalid")
	}
	if err := f.CleanupContractRef.Validate(); err != nil {
		return err
	}
	return f.TrustRef.Validate()
}

func (c ProviderBindingCandidateV1) Validate() error {
	if err := validateContract(c.ContractVersion); err != nil {
		return err
	}
	for _, value := range []string{c.CandidateID, c.ModuleRef, c.SlotRef, c.PortSpecRef} {
		if err := validateID(value); err != nil {
			return err
		}
	}
	if err := c.ProviderRef.Validate(); err != nil {
		return err
	}
	if err := c.Digest.Validate(); err != nil {
		return err
	}
	digest, err := ProviderBindingCandidateDigestV1(c)
	if err != nil {
		return err
	}
	if digest != c.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "provider candidate digest does not match canonical content")
	}
	return nil
}

func (i AssemblyInputV1) Validate() error {
	if err := validateContract(i.ContractVersion); err != nil {
		return err
	}
	for _, value := range []string{i.InputID, i.OwnerRef, i.ScopeRef} {
		if err := validateID(value); err != nil {
			return err
		}
	}
	if i.Revision == 0 || i.CreatedUnixNano <= 0 || i.Policy.MaximumPriority <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "assembly input revision, creation time and priority ceiling are required")
	}
	if err := i.Plan.Validate(); err != nil {
		return err
	}
	for _, refs := range [][]ObjectRefV1{i.CurrentFacts, i.RouteBindings, i.EvidenceRefs} {
		if err := validateRefs(refs, false); err != nil {
			return err
		}
	}
	if i.PreviousGenerationRef != nil {
		if err := i.PreviousGenerationRef.Validate(); err != nil {
			return err
		}
	}
	allowedResiduals := map[string]struct{}{}
	for _, class := range i.Policy.AllowResidualClasses {
		switch runtimeports.ResidualClassV2(class) {
		case runtimeports.ResidualInspectable, runtimeports.ResidualCompensatable, runtimeports.ResidualExternallyOwned, runtimeports.ResidualPotentiallyStale:
		default:
			return core.NewError(core.ErrorInvalidArgument, core.ReasonRemoteResidualUnresolved, "Assembly policy contains an unknown residual class")
		}
		if _, exists := allowedResiduals[class]; exists {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Assembly residual class is duplicated")
		}
		allowedResiduals[class] = struct{}{}
	}
	if len(i.ComponentManifests) == 0 || len(i.ComponentManifests) > MaxAssemblyEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "assembly component manifest set is empty or too large")
	}
	for _, manifest := range i.ComponentManifests {
		if err := manifest.Validate(); err != nil {
			return err
		}
	}
	if len(i.Modules) == 0 || len(i.Slots) == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "assembly modules and slot catalog are required")
	}
	if len(i.Modules) > MaxAssemblyEntries || len(i.Capabilities) > MaxAssemblyEntries || len(i.Slots) > MaxAssemblyEntries || len(i.SlotContributions) > MaxAssemblyEntries || len(i.PortSpecs) > MaxAssemblyEntries || len(i.HookFaces) > MaxAssemblyEntries || len(i.PhaseContributions) > MaxAssemblyEntries || len(i.Dependencies) > MaxAssemblyEntries || len(i.Factories) > MaxAssemblyEntries || len(i.ProviderBindingCandidates) > MaxAssemblyEntries {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "assembly input collection exceeds its bound")
	}
	for _, value := range i.Modules {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range i.Capabilities {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range i.Slots {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range i.SlotContributions {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range i.PortSpecs {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range i.HookFaces {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range i.PhaseContributions {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range i.Dependencies {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range i.Factories {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range i.ProviderBindingCandidates {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	if err := i.Digest.Validate(); err != nil {
		return err
	}
	digest, err := AssemblyInputDigestV1(i)
	if err != nil {
		return err
	}
	if digest != i.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "assembly input digest does not match canonical content")
	}
	return nil
}

func (h AssemblyHandoffV1) Validate() error {
	if err := validateContract(h.ContractVersion); err != nil {
		return err
	}
	if err := h.GenerationRef.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{h.ManifestDigest, h.GraphDigest, h.CatalogDigest, h.Digest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := runtimeports.ValidateNamespacedNameV2(h.RequiredExtension); err != nil {
		return err
	}
	for _, candidate := range h.ProviderCandidates {
		if err := candidate.Validate(); err != nil {
			return err
		}
	}
	digest, err := HandoffDigestV1(h)
	if err != nil {
		return err
	}
	if digest != h.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "assembly handoff digest does not match canonical content")
	}
	return nil
}

func (c AssemblyBindingConformanceV1) Validate(nowUnixNano int64) error {
	if err := validateContract(c.ContractVersion); err != nil {
		return err
	}
	if err := c.HandoffRef.Validate(); err != nil {
		return err
	}
	if err := c.GenerationRef.Validate(); err != nil {
		return err
	}
	for _, digest := range []core.Digest{c.ManifestDigest, c.GraphDigest, c.BindingSetDigest, c.BindingSetSemanticDigest, c.Digest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if c.Association == nil {
		if c.Binding.BindingSetID == "" || c.Binding.BindingSetRevision == 0 || c.Binding.ComponentID == "" || c.Binding.ManifestDigest.Validate() != nil || c.Binding.ArtifactDigest.Validate() != nil || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(c.Binding.Capability)) != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingNotCertified, "binding conformance requires an exact public Runtime binding reference")
		}
		if err := c.CapabilityDigest.Validate(); err != nil {
			return err
		}
		if len(c.SchemaDigests) == 0 || len(c.SchemaDigests) > MaxAssemblyEntries {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownSchema, "binding conformance schema digest set is empty or too large")
		}
		for _, digest := range c.SchemaDigests {
			if err := digest.Validate(); err != nil {
				return err
			}
		}
	} else {
		if err := c.Association.Validate(); err != nil {
			return err
		}
		if c.GovernanceExtension == nil || c.GovernanceExtension.Validate() != nil || c.BindingSetID == "" || c.BindingSetRevision == 0 {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonBindingNotCertified, "association conformance requires exact Runtime association, BindingSet and governance extension references")
		}
		for _, digest := range []core.Digest{c.InputDigest, c.CatalogDigest, c.ComponentManifestSetDigest, c.GenerationProjectionDigest, c.BindingSetCurrentnessDigest, c.BindingSetProjectionDigest, c.ActivationOperationDigest, c.ActivationCurrentnessDigest, c.ActivationProjectionDigest} {
			if err := digest.Validate(); err != nil {
				return err
			}
		}
		if c.Binding.BindingSetID != "" || c.Binding.BindingSetRevision != 0 || c.Binding.ComponentID != "" || c.CapabilityDigest != "" || len(c.SchemaDigests) != 0 {
			return core.NewError(core.ErrorConflict, core.ReasonBindingNotCertified, "association conformance must not synthesize a provider Binding or capability/schema certification")
		}
	}
	if c.ObservedUnixNano <= 0 || c.ExpiresUnixNano <= c.ObservedUnixNano || nowUnixNano <= 0 || c.ExpiresUnixNano <= nowUnixNano || !c.Current {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "binding conformance is stale, expired or non-current")
	}
	digest, err := BindingConformanceDigestV1(c)
	if err != nil {
		return err
	}
	if digest != c.Digest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "binding conformance digest does not match canonical content")
	}
	return nil
}
