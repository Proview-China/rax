package assemblycompiler

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func validateManifestOwnership(input assemblycontract.AssemblyInputV1, index indexes) error {
	for _, module := range input.Modules {
		manifest, ok := index.manifests[module.ComponentManifestRef.ID]
		if !ok {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "module ComponentManifest ref is not present")
		}
		digest, err := manifest.BindingDigestV2()
		if err != nil {
			return err
		}
		if module.ComponentManifestRef.Digest != digest || module.ComponentManifestRef.ID != string(manifest.ComponentID) || module.ArtifactDigest != manifest.ArtifactDigest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "module and Runtime ComponentManifest identity or digest drifted")
		}
		if module.Locality != manifest.Locality || module.ResidualClass != manifest.ResidualClass {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "module locality or residual class differs from its Runtime ComponentManifest")
		}
		provided := map[string]struct{}{}
		for _, capability := range manifest.ProvidedCapabilities {
			provided[string(capability.Capability)] = struct{}{}
		}
		for _, capability := range module.Capabilities {
			if _, ok := provided[string(capability)]; !ok {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "module capability is absent from its Runtime ComponentManifest")
			}
		}
		manifestSchemas := map[string]struct{}{}
		for _, schema := range manifest.Schemas {
			manifestSchemas[schema.Key()] = struct{}{}
		}
		for _, schema := range module.Schemas {
			if _, ok := manifestSchemas[schema.Key()]; !ok {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownSchema, "module schema is absent from its Runtime ComponentManifest")
			}
		}
		owners := map[string]struct{}{}
		for _, owner := range manifest.Owners {
			owners[string(owner.Role)+"\x00"+string(owner.OwnerComponentID)] = struct{}{}
		}
		for _, owner := range module.Owners {
			if _, ok := owners[string(owner.Role)+"\x00"+string(owner.OwnerComponentID)]; !ok {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerConflict, "module owner is absent from its Runtime ComponentManifest")
			}
		}
	}
	return nil
}

func matchingSlot(ref string, index indexes) (assemblycontract.SlotSpecV1, bool) {
	if value, ok := index.slots[ref]; ok {
		return value, true
	}
	for id, value := range index.slots {
		if strings.HasSuffix(id, ".*") && strings.HasPrefix(ref, strings.TrimSuffix(id, "*")) && len(ref) > len(id)-1 {
			return value, true
		}
	}
	return assemblycontract.SlotSpecV1{}, false
}

func containsSlotKind(values []assemblycontract.SlotContributionKindV1, target assemblycontract.SlotContributionKindV1) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func validateReferences(input assemblycontract.AssemblyInputV1, index indexes) error {
	for _, module := range input.Modules {
		for _, capability := range module.Capabilities {
			if _, ok := index.capabilities[capability]; !ok {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "module capability has no descriptor")
			}
		}
	}
	for _, contribution := range input.SlotContributions {
		module, ok := index.modules[contribution.ModuleRef]
		if !ok {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "slot contribution module is unknown")
		}
		slot, ok := matchingSlot(contribution.SlotRef, index)
		if !ok {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "slot contribution references an unknown Harness Slot")
		}
		if !containsSlotKind(slot.ContributionKinds, contribution.Kind) {
			return core.NewError(core.ErrorForbidden, core.ReasonPlanInvalid, "slot contribution exceeds the SlotSpec contribution ceiling")
		}
		capability, ok := index.capabilities[contribution.CapabilityRef]
		if !ok {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "slot contribution capability is unknown")
		}
		if capability.OwnerCapability != slot.OwnerCapability {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerConflict, "slot contribution capability owner does not match SlotSpec owner")
		}
		declared := false
		for _, value := range module.Capabilities {
			if value == contribution.CapabilityRef {
				declared = true
				break
			}
		}
		if !declared {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "module did not declare the contribution capability")
		}
		if contribution.PortSpecRef != "" {
			port, ok := index.ports[contribution.PortSpecRef]
			if !ok {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "slot contribution PortSpec is unknown")
			}
			if port.OwnerCapability != slot.OwnerCapability || port.RequestSchema.Key() != slot.InputSchema.Key() || port.ResponseSchema.Key() != slot.OutputSchema.Key() {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownSchema, "PortSpec owner or schemas do not match the SlotSpec")
			}
		}
		if contribution.ProviderCandidateRef != "" {
			candidate, ok := index.candidates[contribution.ProviderCandidateRef]
			if !ok || candidate.ModuleRef != contribution.ModuleRef || candidate.SlotRef != contribution.SlotRef || candidate.PortSpecRef != contribution.PortSpecRef {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "provider candidate does not exactly bind the slot contribution")
			}
		}
		if contribution.Priority > input.Policy.MaximumPriority || contribution.Priority < -input.Policy.MaximumPriority {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "slot contribution priority exceeds Assembly policy")
		}
	}
	for _, phase := range input.PhaseContributions {
		if _, ok := index.modules[phase.ModuleRef]; !ok {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "phase contribution module is unknown")
		}
		hook, ok := index.hookfaces[phase.HookFaceRef]
		if !ok {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "phase contribution references an unknown Harness HookFace")
		}
		if hook.Kind != phase.Capability {
			return core.NewError(core.ErrorForbidden, core.ReasonPlanInvalid, "phase contribution capability exceeds HookFace kind")
		}
		if phase.Priority > input.Policy.MaximumPriority || phase.Priority < -input.Policy.MaximumPriority {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "phase contribution priority exceeds Assembly policy")
		}
		if phase.Capability == assemblycontract.PhaseFilterV1 {
			allowed := map[string]struct{}{}
			for _, path := range hook.MutationMask {
				allowed[path] = struct{}{}
			}
			for _, path := range phase.WriteSet {
				if _, ok := allowed[path]; !ok {
					return core.NewError(core.ErrorForbidden, core.ReasonPlanInvalid, "filter write-set exceeds HookFace mutation mask")
				}
			}
		}
	}
	for _, factory := range input.Factories {
		module, ok := index.modules[factory.ModuleRef]
		if !ok {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "factory module is unknown")
		}
		if factory.ArtifactDigest != module.ArtifactDigest {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "factory artifact digest does not match module")
		}
		if _, ok := index.capabilities[factory.OutputCapability]; !ok {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "factory output capability is unknown")
		}
	}
	return nil
}

func validateCardinality(input assemblycontract.AssemblyInputV1, index indexes) error {
	bySlot := map[string][]assemblycontract.SlotContributionV1{}
	for _, contribution := range input.SlotContributions {
		bySlot[contribution.SlotRef] = append(bySlot[contribution.SlotRef], contribution)
	}
	for _, spec := range input.Slots {
		if strings.HasSuffix(spec.SlotID, ".*") {
			continue
		}
		values := bySlot[spec.SlotID]
		if spec.Required && len(values) == 0 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "required Slot has no contribution")
		}
		if !spec.Required && len(values) == 0 {
			continue
		}
		switch spec.Cardinality {
		case assemblycontract.CardinalityExactlyOneV1, assemblycontract.CardinalityActiveBindingV1:
			if len(values) > 1 || (spec.Required && len(values) != 1) {
				return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "Slot requires exactly one active contribution")
			}
		case assemblycontract.CardinalityZeroOrOneV1:
			if len(values) > 1 {
				return core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "Slot allows at most one contribution")
			}
		case assemblycontract.CardinalityOwnerSourcesV1:
			owners := 0
			for _, value := range values {
				if value.Kind == assemblycontract.SlotContributionOwnerV1 {
					owners++
				}
			}
			if owners != 1 {
				return core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "owner+sources Slot requires exactly one owner contribution")
			}
		}
	}
	return nil
}

func validatePhaseConflicts(input assemblycontract.AssemblyInputV1, index indexes) error {
	writes := map[string]map[string]string{}
	for _, phase := range input.PhaseContributions {
		if phase.Capability != assemblycontract.PhaseFilterV1 {
			continue
		}
		if writes[phase.HookFaceRef] == nil {
			writes[phase.HookFaceRef] = map[string]string{}
		}
		for _, path := range phase.WriteSet {
			if prior, exists := writes[phase.HookFaceRef][path]; exists {
				return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "filter write-set overlaps without a registered composition rule: "+prior+" and "+phase.ContributionID)
			}
			writes[phase.HookFaceRef][path] = phase.ContributionID
		}
	}
	return nil
}
