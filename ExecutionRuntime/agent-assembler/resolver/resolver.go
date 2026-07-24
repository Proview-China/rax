package resolver

import (
	"context"
	"reflect"
	"sort"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/mapper"
	assemblerports "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/ports"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type Resolver struct {
	facts   assemblerports.ResolutionFactsReaderV1
	catalog assemblerports.ComponentReleaseCatalogReaderV1
	plans   assemblerports.ResolvedAgentPlanRepositoryV1
	clock   func() time.Time
}

func New(facts assemblerports.ResolutionFactsReaderV1, catalog assemblerports.ComponentReleaseCatalogReaderV1, plans assemblerports.ResolvedAgentPlanRepositoryV1, clock func() time.Time) (*Resolver, error) {
	if isNil(facts) || isNil(catalog) || isNil(plans) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "assembler requires exact facts, catalog, plan repository, and clock")
	}
	return &Resolver{facts: facts, catalog: catalog, plans: plans, clock: clock}, nil
}

func (r *Resolver) Resolve(ctx context.Context, request assemblercontract.ResolveRequestV1) (assemblercontract.ResolveResultV1, error) {
	if err := ctx.Err(); err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	if err := request.FactsRef.Validate(); err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	if err := request.CatalogRef.Validate(); err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	factsS1, err := r.facts.InspectExactResolutionFactsV1(ctx, request.FactsRef)
	if err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	if err := factsS1.Validate(); err != nil || factsS1.RefV1() != request.FactsRef {
		return assemblercontract.ResolveResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "resolution facts exact reader returned invalid or different facts")
	}
	catalogS1, err := r.catalog.InspectExactComponentReleaseCatalogV1(ctx, request.CatalogRef)
	if err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	if err := catalogS1.Validate(); err != nil || catalogS1.RefV1() != request.CatalogRef {
		return assemblercontract.ResolveResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "component catalog exact reader returned invalid or different snapshot")
	}
	definitionCatalog := definitionValidationCatalog(catalogS1.Governance)
	if err := request.Definition.Validate(definitionCatalog); err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	if request.Definition.RefV1() != factsS1.DefinitionRef {
		return assemblercontract.ResolveResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "resolution facts do not bind the exact definition")
	}
	nowS1 := r.clock().UnixNano()
	if err := validateCurrentWindows(nowS1, request.Definition, factsS1, catalogS1); err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	if err := validateDefinitionFacts(request.Definition, factsS1); err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	selected, err := selectReleases(request.Definition, catalogS1, nowS1)
	if err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	planID, err := assemblercontract.DerivePlanIDV1(request.Definition.RefV1(), factsS1.RefV1(), catalogS1.RefV1())
	if err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	binding, err := mapper.BindingPlanV2(planID, request.Definition, selected, catalogS1.GovernanceDigest)
	if err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	planRefs, err := resolvePlanArtifacts(selected)
	if err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	profile := toAssemblyRef(request.Definition.ProfileSelectionRef)
	if planRefs.Profile != profile {
		return assemblercontract.ResolveResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "resolved profile artifact differs from the definition selection")
	}
	validUntil := minimumValidity(request.Definition.EffectiveWindow.NotAfterUnixNano, factsS1.ExpiresUnixNano, catalogS1.ExpiresUnixNano, selected)
	components := make([]assemblercontract.ResolvedComponentV1, 0, len(selected))
	evidence := append([]assemblycontract.ObjectRefV1{}, factsS1.EvidenceRefs...)
	for _, release := range selected {
		components = append(components, assemblercontract.ResolvedComponentV1{RequirementID: string(release.ComponentManifest.ComponentID), ReleaseRef: release.RefV1(), Manifest: release.ComponentManifest})
		evidence = append(evidence, release.EvidenceRefs...)
	}
	plan := assemblercontract.ResolvedAgentPlanV1{
		PlanID: planID, Revision: 1, DefinitionRef: request.Definition.RefV1(), IdentityRef: factsS1.IdentityRef,
		ProfileRef: profile, PolicyRefs: append([]assemblycontract.ObjectRefV1{}, factsS1.PolicyRefs...), SandboxRequirementRef: factsS1.SandboxRequirementRef,
		ComponentReleases: components, BindingPlan: binding, AssemblyPlanRefs: planRefs, HarnessBootstrapRef: planRefs.HarnessBootstrapPlan,
		ResolutionFactsRef: factsS1.RefV1(), CatalogRef: catalogS1.RefV1(), Residuals: []assemblycontract.ResidualReportV1{}, EvidenceRefs: evidence,
		CreatedUnixNano: factsS1.FrozenUnixNano, ValidUntilUnixNano: validUntil,
	}
	plan, err = assemblercontract.SealResolvedAgentPlanV1(plan)
	if err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	factsS2, err := r.facts.InspectExactResolutionFactsV1(ctx, request.FactsRef)
	if err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	catalogS2, err := r.catalog.InspectExactComponentReleaseCatalogV1(ctx, request.CatalogRef)
	if err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	nowS2 := r.clock().UnixNano()
	if nowS2 < nowS1 {
		return assemblercontract.ResolveResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "assembler clock regressed between current reads")
	}
	if factsS2.RefV1() != factsS1.RefV1() || catalogS2.RefV1() != catalogS1.RefV1() || !reflect.DeepEqual(factsS1, factsS2) || !reflect.DeepEqual(catalogS1, catalogS2) {
		return assemblercontract.ResolveResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "resolution facts or catalog drifted during resolve")
	}
	if err := validateCurrentWindows(nowS2, request.Definition, factsS2, catalogS2); err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	assemblyInput, err := mapper.AssemblyInputV1(plan, factsS2, selected)
	if err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	stored, err := r.plans.EnsureExactResolvedAgentPlanV1(ctx, plan)
	if err != nil {
		inspected, inspectErr := r.plans.InspectExactResolvedAgentPlanV1(ctx, plan.RefV1())
		if inspectErr != nil || inspected.Validate() != nil || !reflect.DeepEqual(inspected, plan) {
			return assemblercontract.ResolveResultV1{}, err
		}
		stored = inspected
	}
	if stored.Validate() != nil || !reflect.DeepEqual(stored, plan) {
		return assemblercontract.ResolveResultV1{}, core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "plan repository returned different exact content")
	}
	if err := r.ensureCurrent(ctx, request.Definition.DefinitionID, stored, factsS2.FrozenUnixNano, stored.ValidUntilUnixNano); err != nil {
		return assemblercontract.ResolveResultV1{}, err
	}
	return assemblercontract.CloneResolveResultV1(assemblercontract.ResolveResultV1{Plan: stored, BindingPlan: binding, AssemblyInput: assemblyInput}), nil
}

func (r *Resolver) ensureCurrent(ctx context.Context, definitionID string, plan assemblercontract.ResolvedAgentPlanV1, checked, expires int64) error {
	current, inspectErr := r.plans.InspectCurrentResolvedAgentPlanV1(ctx, definitionID)
	var expected *assemblercontract.CurrentResolvedPlanRefV1
	nextRevision := core.Revision(1)
	var previous *assemblercontract.CurrentResolvedPlanRefV1
	if inspectErr == nil {
		if err := current.Validate(); err != nil {
			return err
		}
		if current.PlanRef == plan.RefV1() {
			return nil
		}
		currentRef := current.RefV1()
		expected = &currentRef
		previous = &currentRef
		nextRevision = current.Revision + 1
	} else if core.HasCategory(inspectErr, core.ErrorNotFound) {
	} else {
		return inspectErr
	}
	next, err := assemblercontract.SealCurrentResolvedPlanV1(assemblercontract.CurrentResolvedPlanV1{
		DefinitionID: definitionID, Revision: nextRevision, PlanRef: plan.RefV1(), PreviousRef: previous,
		UpdatedUnixNano: checked, CheckedUnixNano: checked, ExpiresUnixNano: expires,
	})
	if err != nil {
		return err
	}
	_, err = r.plans.CompareAndSwapCurrentResolvedAgentPlanV1(ctx, expected, next)
	if err == nil {
		return nil
	}
	current, inspectErr = r.plans.InspectCurrentResolvedAgentPlanV1(ctx, definitionID)
	if inspectErr == nil && current.Validate() == nil && reflect.DeepEqual(current, next) {
		return nil
	}
	return err
}

func definitionValidationCatalog(catalog runtimeports.GovernanceCatalogV2) definitioncontract.ValidationCatalogV1 {
	result := definitioncontract.ValidationCatalogV1{}
	for _, registration := range catalog.Registrations {
		result.Kinds = append(result.Kinds, string(registration.Kind))
		for _, capability := range registration.Capabilities {
			result.Capabilities = append(result.Capabilities, string(capability))
		}
		for _, extension := range registration.ExtensionPolicies {
			result.RegisteredExtensionKeys = append(result.RegisteredExtensionKeys, string(extension.Key))
		}
	}
	result.Kinds = uniqueStrings(result.Kinds)
	result.Capabilities = uniqueStrings(result.Capabilities)
	result.RegisteredExtensionKeys = uniqueStrings(result.RegisteredExtensionKeys)
	return result
}

func validateCurrentWindows(now int64, definition definitioncontract.AgentDefinitionV1, facts assemblercontract.ResolutionFactsSnapshotV1, catalog assemblercontract.ComponentReleaseCatalogSnapshotV1) error {
	if now < facts.FrozenUnixNano || now < catalog.CheckedUnixNano || now < definition.EffectiveWindow.NotBeforeUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "current observation precedes a sealed lower time bound")
	}
	if now >= facts.ExpiresUnixNano || now >= catalog.ExpiresUnixNano || now >= definition.EffectiveWindow.NotAfterUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "definition or resolution current facts expired")
	}
	return nil
}

func validateDefinitionFacts(definition definitioncontract.AgentDefinitionV1, facts assemblercontract.ResolutionFactsSnapshotV1) error {
	if facts.IdentityRef != toAssemblyRef(definition.IdentityRef) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "identity resolution fact differs from definition")
	}
	required := []definitioncontract.ObjectRefV1{definition.IdentityRef, definition.ProfileSelectionRef, definition.ProvenanceRef, definition.ApprovalRef,
		definition.PolicyRefs.Runtime, definition.PolicyRefs.Authority, definition.PolicyRefs.Review, definition.PolicyRefs.Budget, definition.PolicyRefs.Sandbox,
		definition.PolicyRefs.Context, definition.PolicyRefs.Continuity, definition.PolicyRefs.ToolMCP, definition.PolicyRefs.MemoryKnowledge}
	available := map[assemblycontract.ObjectRefV1]struct{}{}
	for _, ref := range facts.CurrentFacts {
		available[ref] = struct{}{}
	}
	for _, ref := range required {
		if _, ok := available[toAssemblyRef(ref)]; !ok {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "resolution facts omit a required exact definition ref")
		}
	}
	return nil
}

func selectReleases(definition definitioncontract.AgentDefinitionV1, catalog assemblercontract.ComponentReleaseCatalogSnapshotV1, now int64) ([]assemblercontract.ComponentReleaseV1, error) {
	selected := make([]assemblercontract.ComponentReleaseV1, 0, len(definition.Components))
	for _, requirement := range definition.Components {
		candidates := []assemblercontract.ComponentReleaseV1{}
		for _, release := range catalog.Releases {
			manifest := release.ComponentManifest
			if string(manifest.ComponentID) != requirement.ComponentID || string(manifest.Kind) != requirement.Kind {
				continue
			}
			if release.SupportMode != assemblercontract.SupportProductionV1 || now >= release.ExpiresUnixNano {
				continue
			}
			if !containsVersion(requirement.SemanticVersion, manifest.SemanticVersion) || string(manifest.Contract.Name) != requirement.ContractName || !containsVersion(requirement.ContractVersion, manifest.Contract.Version) {
				continue
			}
			if runtimeports.LocalityV2(requirement.LocalityConstraint) != manifest.Locality {
				continue
			}
			if !providesAll(manifest, requirement.RequiredCapabilities) {
				continue
			}
			if err := credentialsSatisfied(definition.SecretRefs, manifest.Credentials); err != nil {
				continue
			}
			candidates = append(candidates, release)
		}
		if len(candidates) == 0 {
			if !requirement.Required {
				continue
			}
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "required production component release is absent")
		}
		sort.Slice(candidates, func(i, j int) bool {
			left, _ := core.ParseSemanticVersion(candidates[i].ComponentManifest.SemanticVersion)
			right, _ := core.ParseSemanticVersion(candidates[j].ComponentManifest.SemanticVersion)
			comparison := core.CompareSemanticVersion(left, right)
			if comparison != 0 {
				return comparison > 0
			}
			return candidates[i].ReleaseID < candidates[j].ReleaseID
		})
		if len(candidates) > 1 && candidates[0].ComponentManifest.SemanticVersion == candidates[1].ComponentManifest.SemanticVersion {
			return nil, core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "multiple exact component releases satisfy the same highest version")
		}
		selected = append(selected, candidates[0])
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].ComponentManifest.ComponentID < selected[j].ComponentManifest.ComponentID
	})
	if err := validateSelectedGraph(definition, selected); err != nil {
		return nil, err
	}
	if err := validateUniqueCapabilityProviders(definition, selected); err != nil {
		return nil, err
	}
	return selected, nil
}

func validateSelectedGraph(definition definitioncontract.AgentDefinitionV1, releases []assemblercontract.ComponentReleaseV1) error {
	selected := map[runtimeports.ComponentIDV2]struct{}{}
	declared := map[string]map[string]struct{}{}
	for _, requirement := range definition.Components {
		set := map[string]struct{}{}
		for _, dep := range requirement.DependencyIDs {
			set[dep] = struct{}{}
		}
		declared[requirement.ComponentID] = set
	}
	for _, release := range releases {
		selected[release.ComponentManifest.ComponentID] = struct{}{}
	}
	for _, release := range releases {
		for _, dependency := range release.ComponentManifest.Dependencies {
			if dependency.Optional {
				continue
			}
			if _, ok := selected[dependency.ComponentID]; !ok {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "release required dependency is not selected")
			}
			if _, ok := declared[string(release.ComponentManifest.ComponentID)][string(dependency.ComponentID)]; !ok {
				return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "release dependency is absent from definition dependency graph")
			}
		}
	}
	return nil
}

func validateUniqueCapabilityProviders(definition definitioncontract.AgentDefinitionV1, releases []assemblercontract.ComponentReleaseV1) error {
	providers := map[string][]runtimeports.ComponentIDV2{}
	for _, release := range releases {
		for _, capability := range release.ComponentManifest.ProvidedCapabilities {
			providers[string(capability.Capability)] = append(providers[string(capability.Capability)], release.ComponentManifest.ComponentID)
		}
	}
	for _, requirement := range definition.Components {
		selectedRequirement := false
		for _, release := range releases {
			if string(release.ComponentManifest.ComponentID) == requirement.ComponentID {
				selectedRequirement = true
				break
			}
		}
		if !selectedRequirement {
			continue
		}
		for _, capability := range requirement.RequiredCapabilities {
			values := providers[capability]
			if len(values) != 1 || string(values[0]) != requirement.ComponentID {
				return core.NewError(core.ErrorConflict, core.ReasonUnknownCapability, "required capability does not resolve to one exact declared provider")
			}
		}
	}
	return nil
}

func resolvePlanArtifacts(releases []assemblercontract.ComponentReleaseV1) (assemblycontract.AssemblyPlanRefsV1, error) {
	artifacts := map[assemblercontract.PlanArtifactRoleV1]assemblycontract.ObjectRefV1{}
	for _, release := range releases {
		for _, artifact := range release.RequiredPlanArtifacts {
			if existing, ok := artifacts[artifact.Role]; ok && existing != artifact.Ref {
				return assemblycontract.AssemblyPlanRefsV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "multiple releases publish a required plan artifact role")
			}
			artifacts[artifact.Role] = artifact.Ref
		}
	}
	required := []assemblercontract.PlanArtifactRoleV1{assemblercontract.ArtifactHarnessBootstrapV1, assemblercontract.ArtifactProfileV1, assemblercontract.ArtifactRuntimePolicyV1, assemblercontract.ArtifactHarnessStackV1, assemblercontract.ArtifactSemanticRouteV1, assemblercontract.ArtifactContextPlanV1, assemblercontract.ArtifactToolSurfaceV1, assemblercontract.ArtifactCapabilityGrantV1, assemblercontract.ArtifactExpectedInjectionV1}
	for _, role := range required {
		if _, ok := artifacts[role]; !ok {
			return assemblycontract.AssemblyPlanRefsV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "required sealed plan artifact is absent")
		}
	}
	return assemblycontract.AssemblyPlanRefsV1{HarnessBootstrapPlan: artifacts[assemblercontract.ArtifactHarnessBootstrapV1], Profile: artifacts[assemblercontract.ArtifactProfileV1], RuntimePolicy: artifacts[assemblercontract.ArtifactRuntimePolicyV1], HarnessStack: artifacts[assemblercontract.ArtifactHarnessStackV1], SemanticRoute: artifacts[assemblercontract.ArtifactSemanticRouteV1], ContextPlan: artifacts[assemblercontract.ArtifactContextPlanV1], ToolSurface: artifacts[assemblercontract.ArtifactToolSurfaceV1], CapabilityGrant: artifacts[assemblercontract.ArtifactCapabilityGrantV1], ExpectedInjectionManifest: artifacts[assemblercontract.ArtifactExpectedInjectionV1]}, nil
}

func containsVersion(value definitioncontract.VersionRangeV1, candidate string) bool {
	return (runtimeports.VersionRangeV2{MinimumInclusive: value.MinimumInclusive, MaximumExclusive: value.MaximumExclusive}).Contains(candidate)
}
func providesAll(manifest runtimeports.ComponentManifestV2, required []string) bool {
	provided := map[string]struct{}{}
	for _, capability := range manifest.ProvidedCapabilities {
		provided[string(capability.Capability)] = struct{}{}
	}
	for _, capability := range required {
		if _, ok := provided[capability]; !ok {
			return false
		}
	}
	return true
}
func credentialsSatisfied(secrets []definitioncontract.SecretRefV1, required []runtimeports.CredentialRequirementV2) error {
	for _, credential := range required {
		found := false
		for _, secret := range secrets {
			if secret.Class == string(credential.CredentialClass) && secret.RequestedScopeDigest == credential.ScopeDigest {
				found = true
				break
			}
		}
		if !found {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonCredentialLeaseMissing, "release credential requirement is unresolved")
		}
	}
	return nil
}
func minimumValidity(definition, facts, catalog int64, releases []assemblercontract.ComponentReleaseV1) int64 {
	result := definition
	for _, value := range []int64{facts, catalog} {
		if value < result {
			result = value
		}
	}
	for _, release := range releases {
		if release.ExpiresUnixNano < result {
			result = release.ExpiresUnixNano
		}
	}
	return result
}
func toAssemblyRef(value definitioncontract.ObjectRefV1) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: value.ID, Revision: value.Revision, Digest: value.Digest}
}
func uniqueStrings(values []string) []string {
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	}
	return false
}
