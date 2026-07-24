package assemblyadapter

import (
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// validateManifestGraphV1 checks the compiler-owned structure in addition to
// its digest. A caller cannot make a forged Graph acceptable merely by
// recomputing Graph, Generation and Handoff digests together.
func validateManifestGraphV1(manifest assemblycontract.AssemblyManifestV1, graph assemblycontract.CompiledHarnessGraphV1) error {
	if err := validateManifestDeclarationsV1(manifest); err != nil {
		return err
	}
	order, err := expectedDependencyOrderV1(manifest)
	if err != nil {
		return err
	}
	if !equalStringsV1(graph.DependencyOrder, order) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "compiled Graph dependency order drifted from the Assembly Manifest")
	}
	rank := make(map[string]int, len(order))
	for index, id := range order {
		rank[id] = index
	}

	portRefs := make([]string, 0, len(manifest.PortSpecs))
	for _, port := range manifest.PortSpecs {
		portRefs = append(portRefs, port.PortID)
	}
	factoryRefs := make([]string, 0, len(manifest.Factories))
	for _, factory := range manifest.Factories {
		factoryRefs = append(factoryRefs, factory.FactoryID)
	}
	if !equalStringsV1(graph.PortSpecRefs, portRefs) || !equalStringsV1(graph.FactoryRefs, factoryRefs) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "compiled Graph Port or Factory references drifted from the Assembly Manifest")
	}

	if len(graph.Slots) != len(manifest.Slots) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "compiled Graph Slot set drifted from the Assembly Manifest")
	}
	for index, slot := range manifest.Slots {
		resolved := graph.Slots[index]
		if resolved.SlotID != slot.SlotID || resolved.SlotSpecDigest != slot.Digest || resolved.OwnerCapability != slot.OwnerCapability {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "compiled Graph Slot identity or owner drifted from the Assembly Manifest")
		}
		contributions := make([]assemblycontract.SlotContributionV1, 0)
		for _, contribution := range manifest.SlotContributions {
			if slotMatchesV1(slot.SlotID, contribution.SlotRef) {
				contributions = append(contributions, contribution)
			}
		}
		sort.Slice(contributions, func(i, j int) bool {
			if rank[contributions[i].ContributionID] != rank[contributions[j].ContributionID] {
				return rank[contributions[i].ContributionID] < rank[contributions[j].ContributionID]
			}
			if contributions[i].Priority != contributions[j].Priority {
				return contributions[i].Priority < contributions[j].Priority
			}
			if contributions[i].ModuleRef != contributions[j].ModuleRef {
				return contributions[i].ModuleRef < contributions[j].ModuleRef
			}
			return contributions[i].ContributionID < contributions[j].ContributionID
		})
		expected := make([]string, 0, len(contributions))
		for _, contribution := range contributions {
			expected = append(expected, contribution.ContributionID)
		}
		if !equalStringsV1(resolved.Contributions, expected) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "compiled Graph Slot contributions drifted from the Assembly Manifest")
		}
	}

	if len(graph.Phases) != len(manifest.HookFaces) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "compiled Graph Phase set drifted from the Assembly Manifest")
	}
	for index, hook := range manifest.HookFaces {
		resolved := graph.Phases[index]
		if resolved.HookFaceID != hook.HookFaceID || resolved.PhaseID != hook.PhaseID || resolved.Capability != hook.Kind {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "compiled Graph Phase identity or capability drifted from the Assembly Manifest")
		}
		contributions := make([]assemblycontract.PhaseContributionV1, 0)
		for _, contribution := range manifest.PhaseContributions {
			if contribution.HookFaceRef == hook.HookFaceID {
				contributions = append(contributions, contribution)
			}
		}
		sort.Slice(contributions, func(i, j int) bool {
			if rank[contributions[i].ContributionID] != rank[contributions[j].ContributionID] {
				return rank[contributions[i].ContributionID] < rank[contributions[j].ContributionID]
			}
			if contributions[i].Priority != contributions[j].Priority {
				return contributions[i].Priority < contributions[j].Priority
			}
			if contributions[i].ModuleRef != contributions[j].ModuleRef {
				return contributions[i].ModuleRef < contributions[j].ModuleRef
			}
			return contributions[i].ContributionID < contributions[j].ContributionID
		})
		expected := make([]string, 0, len(contributions))
		for _, contribution := range contributions {
			expected = append(expected, contribution.ContributionID)
		}
		if !equalStringsV1(resolved.Contributions, expected) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "compiled Graph Phase contributions drifted from the Assembly Manifest")
		}
	}
	return nil
}

func validateManifestDeclarationsV1(manifest assemblycontract.AssemblyManifestV1) error {
	if err := manifest.Plan.Validate(); err != nil {
		return err
	}
	if manifest.Policy.MaximumPriority <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonPlanInvalid, "Assembly Manifest priority ceiling is required")
	}
	collections := []int{len(manifest.CurrentFacts), len(manifest.RouteBindings), len(manifest.ComponentManifests), len(manifest.Modules), len(manifest.Capabilities), len(manifest.Slots), len(manifest.SlotContributions), len(manifest.PortSpecs), len(manifest.HookFaces), len(manifest.PhaseContributions), len(manifest.Dependencies), len(manifest.Factories), len(manifest.ProviderBindingCandidates), len(manifest.Residuals)}
	for _, size := range collections {
		if size > assemblycontract.MaxAssemblyEntries {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Assembly Manifest collection exceeds its bound")
		}
	}
	for _, refs := range [][]assemblycontract.ObjectRefV1{manifest.CurrentFacts, manifest.RouteBindings} {
		for _, ref := range refs {
			if err := ref.Validate(); err != nil {
				return err
			}
		}
	}
	for _, component := range manifest.ComponentManifests {
		if err := component.Validate(); err != nil {
			return err
		}
	}
	for _, value := range manifest.Modules {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range manifest.Capabilities {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range manifest.Slots {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range manifest.SlotContributions {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range manifest.PortSpecs {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range manifest.HookFaces {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range manifest.PhaseContributions {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range manifest.Dependencies {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range manifest.Factories {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	for _, value := range manifest.ProviderBindingCandidates {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	if err := validateModuleManifestRefsV1(manifest); err != nil {
		return err
	}
	if err := validateManifestResidualsV1(manifest); err != nil {
		return err
	}
	return validateManifestCanonicalOrderV1(manifest)
}

func validateManifestResidualsV1(manifest assemblycontract.AssemblyManifestV1) error {
	allowed := make(map[string]struct{}, len(manifest.Policy.AllowResidualClasses))
	for _, class := range manifest.Policy.AllowResidualClasses {
		allowed[class] = struct{}{}
	}
	modules := make(map[string]assemblycontract.ModuleDescriptorV1, len(manifest.Modules))
	for _, module := range manifest.Modules {
		modules[module.ModuleID] = module
	}
	type expectedResidualV1 struct {
		class   string
		owner   string
		inspect assemblycontract.InspectContractRefV1
		cleanup assemblycontract.CleanupContractRefV1
	}
	expected := make([]expectedResidualV1, 0, len(manifest.Residuals))
	for _, component := range manifest.ComponentManifests {
		if component.ResidualClass == runtimeports.ResidualNone {
			continue
		}
		class := string(component.ResidualClass)
		if _, ok := allowed[class]; !ok {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRemoteResidualUnresolved, "component residual class is absent from Assembly policy")
		}
		owner := string(component.ComponentID)
		inspect := assemblycontract.InspectContractRefV1{}
		cleanup := assemblycontract.CleanupContractRefV1{}
		for _, factory := range manifest.Factories {
			module, ok := modules[factory.ModuleRef]
			if ok && module.ComponentManifestRef.ID == owner {
				cleanup = factory.CleanupContractRef
				break
			}
		}
		for _, port := range manifest.PortSpecs {
			for _, contribution := range manifest.SlotContributions {
				if contribution.ModuleRef == "" || contribution.PortSpecRef != port.PortID {
					continue
				}
				module, ok := modules[contribution.ModuleRef]
				if ok && module.ComponentManifestRef.ID == owner && port.InspectContractRef != nil {
					inspect = *port.InspectContractRef
					break
				}
			}
			if inspect.Ref.ID != "" {
				break
			}
		}
		if inspect.Ref.ID == "" || cleanup.Ref.ID == "" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRemoteResidualUnresolved, "component residual lacks its module-owned Inspect or Cleanup contract")
		}
		if err := inspect.Validate(); err != nil {
			return err
		}
		if err := cleanup.Validate(); err != nil {
			return err
		}
		if inspect.OwnerCapability != cleanup.OwnerCapability {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonSettlementOwnerMismatch, "Residual Inspect and Cleanup contracts do not share one domain owner")
		}
		expected = append(expected, expectedResidualV1{class: class, owner: owner, inspect: inspect, cleanup: cleanup})
	}
	sort.Slice(expected, func(i, j int) bool {
		if expected[i].class != expected[j].class {
			return expected[i].class < expected[j].class
		}
		return expected[i].owner < expected[j].owner
	})
	if len(manifest.Residuals) != len(expected) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRemoteResidualUnresolved, "Assembly Manifest Residual set does not cover the component residual set exactly")
	}
	for index, residual := range manifest.Residuals {
		want := expected[index]
		if !validResidualScopeV1(residual.Scope) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Assembly Residual scope is not a canonical reference")
		}
		if err := residual.InspectContractRef.Validate(); err != nil {
			return err
		}
		if err := residual.CleanupContractRef.Validate(); err != nil {
			return err
		}
		if residual.ResidualClass != want.class || residual.Owner != want.owner || residual.InspectContractRef != want.inspect || residual.CleanupContractRef != want.cleanup || !residual.Allowed || residual.BlockingStage != "none" {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Assembly Residual Inspect, Cleanup, Owner or state drifted from compiler-owned declarations")
		}
	}
	return nil
}

func validResidualScopeV1(value string) bool {
	if value == "" || len(value) > assemblycontract.MaxReferenceBytes || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range []byte(value) {
		if character < 0x21 || character > 0x7e {
			return false
		}
	}
	return true
}

func validateModuleManifestRefsV1(manifest assemblycontract.AssemblyManifestV1) error {
	components := make(map[string]struct {
		digest core.Digest
		value  int
	}, len(manifest.ComponentManifests))
	for index, component := range manifest.ComponentManifests {
		digest, err := component.BindingDigestV2()
		if err != nil {
			return err
		}
		components[string(component.ComponentID)] = struct {
			digest core.Digest
			value  int
		}{digest: digest, value: index}
	}
	for _, module := range manifest.Modules {
		entry, ok := components[module.ComponentManifestRef.ID]
		if !ok {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "Assembly module ComponentManifest is absent")
		}
		component := manifest.ComponentManifests[entry.value]
		if module.ComponentManifestRef.Digest != entry.digest || module.ArtifactDigest != component.ArtifactDigest || module.Locality != component.Locality || module.ResidualClass != component.ResidualClass {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMismatch, "Assembly module identity, artifact, locality or residual drifted from its ComponentManifest")
		}
		provided := make(map[string]struct{}, len(component.ProvidedCapabilities))
		for _, capability := range component.ProvidedCapabilities {
			provided[string(capability.Capability)] = struct{}{}
		}
		for _, capability := range module.Capabilities {
			if _, ok := provided[string(capability)]; !ok {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownCapability, "Assembly module capability is absent from its ComponentManifest")
			}
		}
		schemas := make(map[string]struct{}, len(component.Schemas))
		for _, schema := range component.Schemas {
			schemas[schema.Key()] = struct{}{}
		}
		for _, schema := range module.Schemas {
			if _, ok := schemas[schema.Key()]; !ok {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonUnknownSchema, "Assembly module schema is absent from its ComponentManifest")
			}
		}
		owners := make(map[string]struct{}, len(component.Owners))
		for _, owner := range component.Owners {
			owners[string(owner.Role)+"\x00"+string(owner.OwnerComponentID)] = struct{}{}
		}
		for _, owner := range module.Owners {
			if _, ok := owners[string(owner.Role)+"\x00"+string(owner.OwnerComponentID)]; !ok {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonOwnerConflict, "Assembly module owner is absent from its ComponentManifest")
			}
		}
	}
	return nil
}

func validateManifestCanonicalOrderV1(manifest assemblycontract.AssemblyManifestV1) error {
	checks := [][]string{
		refIDsV1(manifest.CurrentFacts), refIDsV1(manifest.RouteBindings), componentIDsV1(manifest), moduleIDsV1(manifest), capabilityIDsV1(manifest), slotIDsV1(manifest), slotContributionIDsV1(manifest), portIDsV1(manifest), hookIDsV1(manifest), phaseContributionIDsV1(manifest), factoryIDsV1(manifest), candidateIDsV1(manifest),
	}
	for _, values := range checks {
		for index := 1; index < len(values); index++ {
			if values[index-1] >= values[index] {
				return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Assembly Manifest collection is not sorted and unique")
			}
		}
	}
	for index := 1; index < len(manifest.Dependencies); index++ {
		left, right := manifest.Dependencies[index-1], manifest.Dependencies[index]
		if left.FromRef > right.FromRef || left.FromRef == right.FromRef && left.ToRef >= right.ToRef {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Assembly Manifest dependency set is not sorted and unique")
		}
	}
	for index := 1; index < len(manifest.Residuals); index++ {
		left, right := manifest.Residuals[index-1], manifest.Residuals[index]
		if left.ResidualClass > right.ResidualClass || left.ResidualClass == right.ResidualClass && left.Owner >= right.Owner {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Assembly Manifest residual set is not sorted and unique")
		}
	}
	return nil
}

func expectedDependencyOrderV1(manifest assemblycontract.AssemblyManifestV1) ([]string, error) {
	nodes := map[string]struct{}{}
	for _, values := range [][]string{moduleIDsV1(manifest), slotContributionIDsV1(manifest), phaseContributionIDsV1(manifest), portIDsV1(manifest), factoryIDsV1(manifest), candidateIDsV1(manifest)} {
		for _, value := range values {
			nodes[value] = struct{}{}
		}
	}
	edges := make(map[string]map[string]struct{}, len(nodes))
	indegree := make(map[string]int, len(nodes))
	for node := range nodes {
		edges[node] = map[string]struct{}{}
		indegree[node] = 0
	}
	add := func(dependent, dependency string, required bool) error {
		if _, ok := nodes[dependent]; !ok {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "Assembly dependency dependent is absent")
		}
		if _, ok := nodes[dependency]; !ok {
			if required {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "required Assembly dependency is absent")
			}
			return nil
		}
		if _, exists := edges[dependency][dependent]; !exists {
			edges[dependency][dependent] = struct{}{}
			indegree[dependent]++
		}
		return nil
	}
	for _, dependency := range manifest.Dependencies {
		if err := add(dependency.FromRef, dependency.ToRef, dependency.Required); err != nil {
			return nil, err
		}
	}
	for _, contribution := range manifest.SlotContributions {
		for _, dependency := range contribution.Dependencies {
			if err := add(contribution.ContributionID, dependency, true); err != nil {
				return nil, err
			}
		}
	}
	for _, contribution := range manifest.PhaseContributions {
		for _, dependency := range contribution.Dependencies {
			if err := add(contribution.ContributionID, dependency, true); err != nil {
				return nil, err
			}
		}
	}
	ready := make([]string, 0)
	for node, degree := range indegree {
		if degree == 0 {
			ready = append(ready, node)
		}
	}
	sort.Strings(ready)
	order := make([]string, 0, len(nodes))
	for len(ready) > 0 {
		node := ready[0]
		ready = ready[1:]
		order = append(order, node)
		children := make([]string, 0, len(edges[node]))
		for child := range edges[node] {
			children = append(children, child)
		}
		sort.Strings(children)
		for _, child := range children {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
				sort.Strings(ready)
			}
		}
	}
	if len(order) != len(nodes) {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonDependencyCycle, "Assembly Manifest dependency graph contains a cycle")
	}
	return order, nil
}

func slotMatchesV1(spec, contribution string) bool {
	return contribution == spec || spec == "domain.*" && strings.HasPrefix(contribution, "domain.") && len(contribution) > len("domain.") || spec != "domain.*" && strings.HasSuffix(spec, ".*") && strings.HasPrefix(contribution, strings.TrimSuffix(spec, "*")) && len(contribution) > len(spec)-1
}

func equalStringsV1(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func refIDsV1(values []assemblycontract.ObjectRefV1) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID)
	}
	return result
}
func componentIDsV1(m assemblycontract.AssemblyManifestV1) []string {
	result := make([]string, 0, len(m.ComponentManifests))
	for _, value := range m.ComponentManifests {
		result = append(result, string(value.ComponentID))
	}
	return result
}
func moduleIDsV1(m assemblycontract.AssemblyManifestV1) []string {
	result := make([]string, 0, len(m.Modules))
	for _, value := range m.Modules {
		result = append(result, value.ModuleID)
	}
	return result
}
func capabilityIDsV1(m assemblycontract.AssemblyManifestV1) []string {
	result := make([]string, 0, len(m.Capabilities))
	for _, value := range m.Capabilities {
		result = append(result, string(value.Capability))
	}
	return result
}
func slotIDsV1(m assemblycontract.AssemblyManifestV1) []string {
	result := make([]string, 0, len(m.Slots))
	for _, value := range m.Slots {
		result = append(result, value.SlotID)
	}
	return result
}
func slotContributionIDsV1(m assemblycontract.AssemblyManifestV1) []string {
	result := make([]string, 0, len(m.SlotContributions))
	for _, value := range m.SlotContributions {
		result = append(result, value.ContributionID)
	}
	return result
}
func portIDsV1(m assemblycontract.AssemblyManifestV1) []string {
	result := make([]string, 0, len(m.PortSpecs))
	for _, value := range m.PortSpecs {
		result = append(result, value.PortID)
	}
	return result
}
func hookIDsV1(m assemblycontract.AssemblyManifestV1) []string {
	result := make([]string, 0, len(m.HookFaces))
	for _, value := range m.HookFaces {
		result = append(result, value.HookFaceID)
	}
	return result
}
func phaseContributionIDsV1(m assemblycontract.AssemblyManifestV1) []string {
	result := make([]string, 0, len(m.PhaseContributions))
	for _, value := range m.PhaseContributions {
		result = append(result, value.ContributionID)
	}
	return result
}
func factoryIDsV1(m assemblycontract.AssemblyManifestV1) []string {
	result := make([]string, 0, len(m.Factories))
	for _, value := range m.Factories {
		result = append(result, value.FactoryID)
	}
	return result
}
func candidateIDsV1(m assemblycontract.AssemblyManifestV1) []string {
	result := make([]string, 0, len(m.ProviderBindingCandidates))
	for _, value := range m.ProviderBindingCandidates {
		result = append(result, value.CandidateID)
	}
	return result
}
