package assemblycontract

import (
	"encoding/json"
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const digestDomainV1 = "praxis.harness.assembly"

func normalizedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	if result == nil {
		result = []string{}
	}
	return result
}

func normalizedRefs(values []ObjectRefV1) []ObjectRefV1 {
	result := append([]ObjectRefV1(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	if result == nil {
		result = []ObjectRefV1{}
	}
	return result
}

func SlotSpecDigestV1(value SlotSpecV1) (core.Digest, error) {
	value.Digest = ""
	value.ContributionKinds = append([]SlotContributionKindV1(nil), value.ContributionKinds...)
	value.Dependencies = normalizedStrings(value.Dependencies)
	value.MutationSafeNormalize()
	return core.CanonicalJSONDigest(digestDomainV1, ContractVersionV1, "SlotSpecV1", value)
}

func (s *SlotSpecV1) MutationSafeNormalize() {
	sort.Slice(s.ContributionKinds, func(i, j int) bool { return s.ContributionKinds[i] < s.ContributionKinds[j] })
	if s.ContributionKinds == nil {
		s.ContributionKinds = []SlotContributionKindV1{}
	}
}

func SlotContributionDigestV1(value SlotContributionV1) (core.Digest, error) {
	value.Digest = ""
	value.Dependencies = normalizedStrings(value.Dependencies)
	return core.CanonicalJSONDigest(digestDomainV1, ContractVersionV1, "SlotContributionV1", value)
}

func HookFaceSpecDigestV1(value HookFaceSpecV1) (core.Digest, error) {
	value.Digest = ""
	value.MutationMask = normalizedStrings(value.MutationMask)
	return core.CanonicalJSONDigest(digestDomainV1, ContractVersionV1, "HookFaceSpecV1", value)
}

func PhaseContributionDigestV1(value PhaseContributionV1) (core.Digest, error) {
	value.Digest = ""
	value.Dependencies = normalizedStrings(value.Dependencies)
	value.WriteSet = normalizedStrings(value.WriteSet)
	return core.CanonicalJSONDigest(digestDomainV1, ContractVersionV1, "PhaseContributionV1", value)
}

func ProviderBindingCandidateDigestV1(value ProviderBindingCandidateV1) (core.Digest, error) {
	value.Digest = ""
	return core.CanonicalJSONDigest(digestDomainV1, ContractVersionV1, "ProviderBindingCandidateV1", value)
}

func SealAssemblyInputV1(value AssemblyInputV1) (AssemblyInputV1, error) {
	value = cloneCanonicalValue(value)
	value.ContractVersion = ContractVersionV1
	for index := range value.Modules {
		value.Modules[index].ContractVersion = ContractVersionV1
	}
	for index := range value.Capabilities {
		value.Capabilities[index].ContractVersion = ContractVersionV1
	}
	for index := range value.Slots {
		value.Slots[index].ContractVersion = ContractVersionV1
		digest, err := SlotSpecDigestV1(value.Slots[index])
		if err != nil {
			return AssemblyInputV1{}, err
		}
		value.Slots[index].Digest = digest
	}
	for index := range value.SlotContributions {
		value.SlotContributions[index].ContractVersion = ContractVersionV1
		digest, err := SlotContributionDigestV1(value.SlotContributions[index])
		if err != nil {
			return AssemblyInputV1{}, err
		}
		value.SlotContributions[index].Digest = digest
	}
	for index := range value.PortSpecs {
		value.PortSpecs[index].ContractVersion = ContractVersionV1
	}
	for index := range value.HookFaces {
		value.HookFaces[index].ContractVersion = ContractVersionV1
		digest, err := HookFaceSpecDigestV1(value.HookFaces[index])
		if err != nil {
			return AssemblyInputV1{}, err
		}
		value.HookFaces[index].Digest = digest
	}
	for index := range value.PhaseContributions {
		value.PhaseContributions[index].ContractVersion = ContractVersionV1
		digest, err := PhaseContributionDigestV1(value.PhaseContributions[index])
		if err != nil {
			return AssemblyInputV1{}, err
		}
		value.PhaseContributions[index].Digest = digest
	}
	for index := range value.Dependencies {
		value.Dependencies[index].ContractVersion = ContractVersionV1
	}
	for index := range value.Factories {
		value.Factories[index].ContractVersion = ContractVersionV1
	}
	for index := range value.ProviderBindingCandidates {
		value.ProviderBindingCandidates[index].ContractVersion = ContractVersionV1
		digest, err := ProviderBindingCandidateDigestV1(value.ProviderBindingCandidates[index])
		if err != nil {
			return AssemblyInputV1{}, err
		}
		value.ProviderBindingCandidates[index].Digest = digest
	}
	value = NormalizeAssemblyInputV1(value)
	digest, err := AssemblyInputDigestV1(value)
	if err != nil {
		return AssemblyInputV1{}, err
	}
	value.Digest = digest
	if err := value.Validate(); err != nil {
		return AssemblyInputV1{}, err
	}
	return value, nil
}

func NormalizeAssemblyInputV1(value AssemblyInputV1) AssemblyInputV1 {
	value = cloneCanonicalValue(value)
	value.CurrentFacts = normalizedRefs(value.CurrentFacts)
	value.RouteBindings = normalizedRefs(value.RouteBindings)
	value.EvidenceRefs = normalizedRefs(value.EvidenceRefs)
	sort.Slice(value.ComponentManifests, func(i, j int) bool {
		return value.ComponentManifests[i].ComponentID < value.ComponentManifests[j].ComponentID
	})
	sort.Slice(value.Modules, func(i, j int) bool { return value.Modules[i].ModuleID < value.Modules[j].ModuleID })
	for index := range value.Modules {
		sort.Slice(value.Modules[index].Capabilities, func(i, j int) bool {
			return value.Modules[index].Capabilities[i] < value.Modules[index].Capabilities[j]
		})
		sort.Slice(value.Modules[index].Schemas, func(i, j int) bool {
			return value.Modules[index].Schemas[i].Key() < value.Modules[index].Schemas[j].Key()
		})
		sort.Slice(value.Modules[index].Owners, func(i, j int) bool {
			if value.Modules[index].Owners[i].Role != value.Modules[index].Owners[j].Role {
				return value.Modules[index].Owners[i].Role < value.Modules[index].Owners[j].Role
			}
			return value.Modules[index].Owners[i].OwnerComponentID < value.Modules[index].Owners[j].OwnerComponentID
		})
		sort.Slice(value.Modules[index].CredentialRequirements, func(i, j int) bool {
			return value.Modules[index].CredentialRequirements[i] < value.Modules[index].CredentialRequirements[j]
		})
	}
	sort.Slice(value.Capabilities, func(i, j int) bool { return value.Capabilities[i].Capability < value.Capabilities[j].Capability })
	for index := range value.Capabilities {
		sort.Slice(value.Capabilities[index].Schemas, func(i, j int) bool {
			return value.Capabilities[index].Schemas[i].Key() < value.Capabilities[index].Schemas[j].Key()
		})
	}
	sort.Slice(value.Slots, func(i, j int) bool { return value.Slots[i].SlotID < value.Slots[j].SlotID })
	for index := range value.Slots {
		value.Slots[index].Dependencies = normalizedStrings(value.Slots[index].Dependencies)
		value.Slots[index].MutationSafeNormalize()
	}
	sort.Slice(value.SlotContributions, func(i, j int) bool {
		return value.SlotContributions[i].ContributionID < value.SlotContributions[j].ContributionID
	})
	for index := range value.SlotContributions {
		value.SlotContributions[index].Dependencies = normalizedStrings(value.SlotContributions[index].Dependencies)
	}
	sort.Slice(value.PortSpecs, func(i, j int) bool { return value.PortSpecs[i].PortID < value.PortSpecs[j].PortID })
	for index := range value.PortSpecs {
		sort.Slice(value.PortSpecs[index].RunStartRequirementRefs, func(i, j int) bool {
			left, right := value.PortSpecs[index].RunStartRequirementRefs[i], value.PortSpecs[index].RunStartRequirementRefs[j]
			if left.RequirementID != right.RequirementID {
				return left.RequirementID < right.RequirementID
			}
			return left.Ref.ID < right.Ref.ID
		})
		sort.Slice(value.PortSpecs[index].RunSettlementRequirementRefs, func(i, j int) bool {
			left, right := value.PortSpecs[index].RunSettlementRequirementRefs[i], value.PortSpecs[index].RunSettlementRequirementRefs[j]
			if left.RequirementID != right.RequirementID {
				return left.RequirementID < right.RequirementID
			}
			return left.Ref.ID < right.Ref.ID
		})
	}
	sort.Slice(value.HookFaces, func(i, j int) bool { return value.HookFaces[i].HookFaceID < value.HookFaces[j].HookFaceID })
	for index := range value.HookFaces {
		value.HookFaces[index].MutationMask = normalizedStrings(value.HookFaces[index].MutationMask)
	}
	sort.Slice(value.PhaseContributions, func(i, j int) bool {
		return value.PhaseContributions[i].ContributionID < value.PhaseContributions[j].ContributionID
	})
	for index := range value.PhaseContributions {
		value.PhaseContributions[index].Dependencies = normalizedStrings(value.PhaseContributions[index].Dependencies)
		value.PhaseContributions[index].WriteSet = normalizedStrings(value.PhaseContributions[index].WriteSet)
	}
	sort.Slice(value.Dependencies, func(i, j int) bool {
		if value.Dependencies[i].FromRef != value.Dependencies[j].FromRef {
			return value.Dependencies[i].FromRef < value.Dependencies[j].FromRef
		}
		return value.Dependencies[i].ToRef < value.Dependencies[j].ToRef
	})
	sort.Slice(value.Factories, func(i, j int) bool { return value.Factories[i].FactoryID < value.Factories[j].FactoryID })
	sort.Slice(value.ProviderBindingCandidates, func(i, j int) bool {
		return value.ProviderBindingCandidates[i].CandidateID < value.ProviderBindingCandidates[j].CandidateID
	})
	value.Policy.AllowResidualClasses = normalizedStrings(value.Policy.AllowResidualClasses)
	return value
}

func cloneCanonicalValue[T any](value T) T {
	payload, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var result T
	if err := json.Unmarshal(payload, &result); err != nil {
		return value
	}
	return result
}

func AssemblyInputDigestV1(value AssemblyInputV1) (core.Digest, error) {
	value.Digest = ""
	value = NormalizeAssemblyInputV1(value)
	type manifestRef struct {
		ComponentID string      `json:"component_id"`
		Digest      core.Digest `json:"digest"`
	}
	summaries := make([]manifestRef, 0, len(value.ComponentManifests))
	for _, manifest := range value.ComponentManifests {
		digest, err := manifest.BindingDigestV2()
		if err != nil {
			return "", err
		}
		summaries = append(summaries, manifestRef{ComponentID: string(manifest.ComponentID), Digest: digest})
	}
	view := struct {
		ContractVersion           string                       `json:"contract_version"`
		InputID                   string                       `json:"input_id"`
		Revision                  core.Revision                `json:"revision"`
		OwnerRef                  string                       `json:"owner_ref"`
		ScopeRef                  string                       `json:"scope_ref"`
		CreatedUnixNano           int64                        `json:"created_unix_nano"`
		Plan                      AssemblyPlanRefsV1           `json:"plan"`
		CurrentFacts              []ObjectRefV1                `json:"current_facts"`
		RouteBindings             []ObjectRefV1                `json:"route_bindings"`
		ComponentManifests        []manifestRef                `json:"component_manifests"`
		Modules                   []ModuleDescriptorV1         `json:"modules"`
		Capabilities              []CapabilityDescriptorV1     `json:"capabilities"`
		Slots                     []SlotSpecV1                 `json:"slots"`
		SlotContributions         []SlotContributionV1         `json:"slot_contributions"`
		PortSpecs                 []PortSpecV1                 `json:"port_specs"`
		HookFaces                 []HookFaceSpecV1             `json:"hookfaces"`
		PhaseContributions        []PhaseContributionV1        `json:"phase_contributions"`
		Dependencies              []DependencySpecV1           `json:"dependencies"`
		Factories                 []ModuleFactoryDescriptorV1  `json:"factories"`
		ProviderBindingCandidates []ProviderBindingCandidateV1 `json:"provider_binding_candidates"`
		Policy                    AssemblyPolicyV1             `json:"policy"`
		PreviousGenerationRef     *ObjectRefV1                 `json:"previous_generation_ref,omitempty"`
		EvidenceRefs              []ObjectRefV1                `json:"evidence_refs"`
	}{value.ContractVersion, value.InputID, value.Revision, value.OwnerRef, value.ScopeRef, value.CreatedUnixNano, value.Plan, value.CurrentFacts, value.RouteBindings, summaries, value.Modules, value.Capabilities, value.Slots, value.SlotContributions, value.PortSpecs, value.HookFaces, value.PhaseContributions, value.Dependencies, value.Factories, value.ProviderBindingCandidates, value.Policy, value.PreviousGenerationRef, value.EvidenceRefs}
	return core.CanonicalJSONDigest(digestDomainV1, ContractVersionV1, "AssemblyInputV1", view)
}

func CatalogDigestV1(slots []SlotSpecV1, hookfaces []HookFaceSpecV1) (core.Digest, error) {
	slotsCopy := cloneCanonicalValue(slots)
	hookCopy := cloneCanonicalValue(hookfaces)
	for index := range slotsCopy {
		slotsCopy[index].Dependencies = normalizedStrings(slotsCopy[index].Dependencies)
		slotsCopy[index].ContributionKinds = append([]SlotContributionKindV1(nil), slotsCopy[index].ContributionKinds...)
		slotsCopy[index].MutationSafeNormalize()
	}
	for index := range hookCopy {
		hookCopy[index].MutationMask = normalizedStrings(hookCopy[index].MutationMask)
	}
	sort.Slice(slotsCopy, func(i, j int) bool { return slotsCopy[i].SlotID < slotsCopy[j].SlotID })
	sort.Slice(hookCopy, func(i, j int) bool { return hookCopy[i].HookFaceID < hookCopy[j].HookFaceID })
	return core.CanonicalJSONDigest(digestDomainV1, CatalogVersionV1, "AssemblyCatalogV1", struct {
		Slots     []SlotSpecV1     `json:"slots"`
		HookFaces []HookFaceSpecV1 `json:"hookfaces"`
	}{slotsCopy, hookCopy})
}

func ManifestDigestV1(value AssemblyManifestV1) (core.Digest, error) {
	value.Digest = ""
	type manifestRef struct {
		ComponentID string      `json:"component_id"`
		Digest      core.Digest `json:"digest"`
	}
	manifestRefs := make([]manifestRef, 0, len(value.ComponentManifests))
	for _, manifest := range value.ComponentManifests {
		digest, err := manifest.BindingDigestV2()
		if err != nil {
			return "", err
		}
		manifestRefs = append(manifestRefs, manifestRef{ComponentID: string(manifest.ComponentID), Digest: digest})
	}
	view := struct {
		ContractVersion           string                       `json:"contract_version"`
		InputDigest               core.Digest                  `json:"input_digest"`
		CatalogDigest             core.Digest                  `json:"catalog_digest"`
		Plan                      AssemblyPlanRefsV1           `json:"plan"`
		CurrentFacts              []ObjectRefV1                `json:"current_facts"`
		RouteBindings             []ObjectRefV1                `json:"route_bindings"`
		Policy                    AssemblyPolicyV1             `json:"policy"`
		ComponentManifests        []manifestRef                `json:"component_manifests"`
		Modules                   []ModuleDescriptorV1         `json:"modules"`
		Capabilities              []CapabilityDescriptorV1     `json:"capabilities"`
		Slots                     []SlotSpecV1                 `json:"slots"`
		SlotContributions         []SlotContributionV1         `json:"slot_contributions"`
		PortSpecs                 []PortSpecV1                 `json:"port_specs"`
		HookFaces                 []HookFaceSpecV1             `json:"hookfaces"`
		PhaseContributions        []PhaseContributionV1        `json:"phase_contributions"`
		Dependencies              []DependencySpecV1           `json:"dependencies"`
		Factories                 []ModuleFactoryDescriptorV1  `json:"factories"`
		ProviderBindingCandidates []ProviderBindingCandidateV1 `json:"provider_binding_candidates"`
		Residuals                 []ResidualReportV1           `json:"residuals"`
	}{value.ContractVersion, value.InputDigest, value.CatalogDigest, value.Plan, value.CurrentFacts, value.RouteBindings, value.Policy, manifestRefs, value.Modules, value.Capabilities, value.Slots, value.SlotContributions, value.PortSpecs, value.HookFaces, value.PhaseContributions, value.Dependencies, value.Factories, value.ProviderBindingCandidates, value.Residuals}
	return core.CanonicalJSONDigest(digestDomainV1, ContractVersionV1, "AssemblyManifestV1", view)
}

func GraphDigestV1(value CompiledHarnessGraphV1) (core.Digest, error) {
	value.Digest = ""
	return core.CanonicalJSONDigest(digestDomainV1, ContractVersionV1, "CompiledHarnessGraphV1", value)
}

func DiagnosticsDigestV1(values []AssemblyDiagnosticV1) (core.Digest, error) {
	copyValues := append([]AssemblyDiagnosticV1(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool {
		if copyValues[i].Code != copyValues[j].Code {
			return copyValues[i].Code < copyValues[j].Code
		}
		return copyValues[i].ObjectPath < copyValues[j].ObjectPath
	})
	return core.CanonicalJSONDigest(digestDomainV1, ContractVersionV1, "AssemblyDiagnosticsV1", copyValues)
}

func ResidualsDigestV1(values []ResidualReportV1) (core.Digest, error) {
	copyValues := append([]ResidualReportV1(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool {
		if copyValues[i].ResidualClass != copyValues[j].ResidualClass {
			return copyValues[i].ResidualClass < copyValues[j].ResidualClass
		}
		return copyValues[i].Owner < copyValues[j].Owner
	})
	return core.CanonicalJSONDigest(digestDomainV1, ContractVersionV1, "AssemblyResidualsV1", copyValues)
}

func GenerationDigestV1(value AssemblyGenerationV1) (core.Digest, error) {
	value.Digest = ""
	return core.CanonicalJSONDigest(digestDomainV1, ContractVersionV1, "AssemblyGenerationV1", value)
}
func HandoffDigestV1(value AssemblyHandoffV1) (core.Digest, error) {
	value.Digest = ""
	return core.CanonicalJSONDigest(digestDomainV1, ContractVersionV1, "AssemblyHandoffV1", value)
}
func BindingConformanceDigestV1(value AssemblyBindingConformanceV1) (core.Digest, error) {
	value.Digest = ""
	value.SchemaDigests = append([]core.Digest(nil), value.SchemaDigests...)
	sort.Slice(value.SchemaDigests, func(i, j int) bool { return value.SchemaDigests[i] < value.SchemaDigests[j] })
	return core.CanonicalJSONDigest(digestDomainV1, ContractVersionV1, "AssemblyBindingConformanceV1", value)
}

func SealBindingConformanceV1(value AssemblyBindingConformanceV1, nowUnixNano int64) (AssemblyBindingConformanceV1, error) {
	value.ContractVersion = ContractVersionV1
	value.SchemaDigests = append([]core.Digest(nil), value.SchemaDigests...)
	sort.Slice(value.SchemaDigests, func(i, j int) bool { return value.SchemaDigests[i] < value.SchemaDigests[j] })
	digest, err := BindingConformanceDigestV1(value)
	if err != nil {
		return AssemblyBindingConformanceV1{}, err
	}
	value.Digest = digest
	if err := value.Validate(nowUnixNano); err != nil {
		return AssemblyBindingConformanceV1{}, err
	}
	return value, nil
}
