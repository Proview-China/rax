package assemblycompiler

import (
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
)

func resolveSlots(input assemblycontract.AssemblyInputV1, order []string) []assemblycontract.ResolvedSlotV1 {
	rank := map[string]int{}
	for position, id := range order {
		rank[id] = position
	}
	result := make([]assemblycontract.ResolvedSlotV1, 0, len(input.Slots))
	for _, slot := range input.Slots {
		values := []assemblycontract.SlotContributionV1{}
		for _, contribution := range input.SlotContributions {
			if contribution.SlotRef == slot.SlotID || (slot.SlotID == "domain.*" && strings.HasPrefix(contribution.SlotRef, "domain.") && len(contribution.SlotRef) > len("domain.")) || (slot.SlotID != "domain.*" && strings.HasSuffix(slot.SlotID, ".*") && strings.HasPrefix(contribution.SlotRef, strings.TrimSuffix(slot.SlotID, "*")) && len(contribution.SlotRef) > len(slot.SlotID)-1) {
				values = append(values, contribution)
			}
		}
		sort.Slice(values, func(i, j int) bool {
			if rank[values[i].ContributionID] != rank[values[j].ContributionID] {
				return rank[values[i].ContributionID] < rank[values[j].ContributionID]
			}
			if values[i].Priority != values[j].Priority {
				return values[i].Priority < values[j].Priority
			}
			if values[i].ModuleRef != values[j].ModuleRef {
				return values[i].ModuleRef < values[j].ModuleRef
			}
			return values[i].ContributionID < values[j].ContributionID
		})
		refs := make([]string, 0, len(values))
		for _, value := range values {
			refs = append(refs, value.ContributionID)
		}
		result = append(result, assemblycontract.ResolvedSlotV1{SlotSpecDigest: slot.Digest, SlotID: slot.SlotID, OwnerCapability: slot.OwnerCapability, Contributions: refs})
	}
	return result
}

func resolvePhases(input assemblycontract.AssemblyInputV1, index indexes, order []string) []assemblycontract.ResolvedPhaseV1 {
	rank := map[string]int{}
	for position, id := range order {
		rank[id] = position
	}
	byHook := map[string][]assemblycontract.PhaseContributionV1{}
	for _, contribution := range input.PhaseContributions {
		byHook[contribution.HookFaceRef] = append(byHook[contribution.HookFaceRef], contribution)
	}
	result := make([]assemblycontract.ResolvedPhaseV1, 0, len(input.HookFaces))
	for _, hook := range input.HookFaces {
		values := byHook[hook.HookFaceID]
		sort.Slice(values, func(i, j int) bool {
			if rank[values[i].ContributionID] != rank[values[j].ContributionID] {
				return rank[values[i].ContributionID] < rank[values[j].ContributionID]
			}
			if values[i].Priority != values[j].Priority {
				return values[i].Priority < values[j].Priority
			}
			if values[i].ModuleRef != values[j].ModuleRef {
				return values[i].ModuleRef < values[j].ModuleRef
			}
			return values[i].ContributionID < values[j].ContributionID
		})
		refs := make([]string, 0, len(values))
		for _, value := range values {
			refs = append(refs, value.ContributionID)
		}
		result = append(result, assemblycontract.ResolvedPhaseV1{HookFaceID: hook.HookFaceID, PhaseID: hook.PhaseID, Capability: hook.Kind, Contributions: refs})
	}
	return result
}
