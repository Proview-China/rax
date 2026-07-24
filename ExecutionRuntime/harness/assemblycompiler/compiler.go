package assemblycompiler

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type Compiler struct{}

func New() Compiler { return Compiler{} }

func (Compiler) Compile(input assemblycontract.AssemblyInputV1) (assemblycontract.CompileResultV1, error) {
	if err := input.Validate(); err != nil {
		return rejected("input_invalid", "assembly_input", "", input.OwnerRef, "valid sealed AssemblyInputV1", err.Error(), err)
	}
	actualInputDigest, err := assemblycontract.AssemblyInputDigestV1(input)
	if err != nil {
		return rejected("input_digest_failed", "assembly_input", "digest", input.OwnerRef, "canonical digest", err.Error(), err)
	}
	if actualInputDigest != input.Digest {
		err = core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidDigest, "assembly input digest does not match canonical content")
		return rejected("input_digest_mismatch", "assembly_input", "digest", input.OwnerRef, string(actualInputDigest), string(input.Digest), err)
	}
	if err := validateDeclaredDigests(input); err != nil {
		return rejected("declared_digest_mismatch", "assembly_input", "digest", input.OwnerRef, "canonical object digest", err.Error(), err)
	}

	builtinSlots := assemblycontract.SlotCatalogV1()
	builtinHookFaces := assemblycontract.HookFaceCatalogV1()
	expectedCatalog, err := assemblycontract.CatalogDigestV1(builtinSlots, builtinHookFaces)
	if err != nil {
		return assemblycontract.CompileResultV1{}, err
	}
	actualCatalog, err := assemblycontract.CatalogDigestV1(input.Slots, input.HookFaces)
	if err != nil {
		return assemblycontract.CompileResultV1{}, err
	}
	if actualCatalog != expectedCatalog {
		err = core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "assembly input attempted to replace the Harness-owned Slot/Phase catalog")
		return rejected("catalog_mismatch", "assembly_input", "slots_or_hookfaces", "praxis.harness/assembly", string(expectedCatalog), string(actualCatalog), err)
	}

	index, err := buildIndex(input)
	if err != nil {
		return rejected("index_conflict", "assembly_input", "collections", input.OwnerRef, "unique ids", err.Error(), err)
	}
	if err := validateManifestOwnership(input, index); err != nil {
		return rejected("owner_or_manifest_conflict", "module", "component_manifest_ref", input.OwnerRef, "matching Runtime manifest owner", err.Error(), err)
	}
	if err := validateReferences(input, index); err != nil {
		return rejected("reference_conflict", "contribution", "reference", input.OwnerRef, "known compatible reference", err.Error(), err)
	}
	if err := validateCardinality(input, index); err != nil {
		return rejected("cardinality_conflict", "slot", "contributions", "praxis.harness/assembly", "declared cardinality", err.Error(), err)
	}
	if err := validatePhaseConflicts(input, index); err != nil {
		return rejected("phase_conflict", "phase", "write_set_or_capability", "praxis.harness/assembly", "non-overlapping authorized contribution", err.Error(), err)
	}
	order, err := dependencyOrder(input, index)
	if err != nil {
		return rejected("dependency_conflict", "dependency", "dag", input.OwnerRef, "acyclic known dependency graph", err.Error(), err)
	}
	residuals, err := compileResiduals(input, index)
	if err != nil {
		return rejected("residual_not_allowed", "component_manifest", "residual_class", input.OwnerRef, "allowed explicit residual", err.Error(), err)
	}

	normalized := assemblycontract.NormalizeAssemblyInputV1(input)
	resolvedSlots := resolveSlots(normalized, order)
	resolvedPhases := resolvePhases(normalized, index, order)
	manifest := assemblycontract.AssemblyManifestV1{
		ContractVersion: assemblycontract.ContractVersionV1, InputDigest: input.Digest, CatalogDigest: actualCatalog,
		Plan: normalized.Plan, CurrentFacts: normalized.CurrentFacts, RouteBindings: normalized.RouteBindings, Policy: normalized.Policy,
		ComponentManifests: normalized.ComponentManifests, Modules: normalized.Modules, Capabilities: normalized.Capabilities,
		Slots: normalized.Slots, SlotContributions: normalized.SlotContributions, PortSpecs: normalized.PortSpecs,
		HookFaces: normalized.HookFaces, PhaseContributions: normalized.PhaseContributions, Dependencies: normalized.Dependencies,
		Factories: normalized.Factories, ProviderBindingCandidates: normalized.ProviderBindingCandidates, Residuals: residuals,
	}
	manifest.Digest, err = assemblycontract.ManifestDigestV1(manifest)
	if err != nil {
		return assemblycontract.CompileResultV1{}, err
	}

	portRefs := make([]string, 0, len(normalized.PortSpecs))
	for _, port := range normalized.PortSpecs {
		portRefs = append(portRefs, port.PortID)
	}
	factoryRefs := make([]string, 0, len(normalized.Factories))
	for _, factory := range normalized.Factories {
		factoryRefs = append(factoryRefs, factory.FactoryID)
	}
	graph := assemblycontract.CompiledHarnessGraphV1{
		ContractVersion: assemblycontract.ContractVersionV1, InputDigest: input.Digest, CatalogDigest: actualCatalog,
		DependencyOrder: order, Slots: resolvedSlots, Phases: resolvedPhases, PortSpecRefs: portRefs, FactoryRefs: factoryRefs,
	}
	graph.Digest, err = assemblycontract.GraphDigestV1(graph)
	if err != nil {
		return assemblycontract.CompileResultV1{}, err
	}

	diagnostics := []assemblycontract.AssemblyDiagnosticV1{{Severity: assemblycontract.DiagnosticInfoV1, Code: "assembly_sealed", ObjectPath: "assembly_generation", Owner: "praxis.harness/assembly", Expected: "pre-binding sealed", Actual: "pre-binding sealed", Remediation: "submit AssemblyHandoffV1 to the Runtime-owned Binding path"}}
	diagnosticDigest, err := assemblycontract.DiagnosticsDigestV1(diagnostics)
	if err != nil {
		return assemblycontract.CompileResultV1{}, err
	}
	residualDigest, err := assemblycontract.ResidualsDigestV1(residuals)
	if err != nil {
		return assemblycontract.CompileResultV1{}, err
	}
	generation := assemblycontract.AssemblyGenerationV1{
		ContractVersion: assemblycontract.ContractVersionV1,
		GenerationID:    "assembly-generation-" + strings.TrimPrefix(string(input.Digest), "sha256:")[:24], Revision: 1,
		CompilerVersion: assemblycontract.CompilerVersionV1, CreatedUnixNano: input.CreatedUnixNano, State: assemblycontract.AssemblyStateSealedV1,
		InputDigest: input.Digest, ManifestDigest: manifest.Digest, GraphDigest: graph.Digest, DiagnosticDigest: diagnosticDigest,
		ResidualReportDigest: residualDigest, PreviousGenerationRef: input.PreviousGenerationRef, EvidenceRefs: append([]assemblycontract.ObjectRefV1(nil), input.EvidenceRefs...),
	}
	generation.Digest, err = assemblycontract.GenerationDigestV1(generation)
	if err != nil {
		return assemblycontract.CompileResultV1{}, err
	}
	handoff := assemblycontract.AssemblyHandoffV1{
		ContractVersion: assemblycontract.ContractVersionV1,
		GenerationRef:   assemblycontract.ObjectRefV1{ID: generation.GenerationID, Revision: generation.Revision, Digest: generation.Digest},
		ManifestDigest:  manifest.Digest, GraphDigest: graph.Digest, CatalogDigest: actualCatalog,
		RequiredExtension:  runtimeports.NamespacedNameV2("praxis.harness/assembly-generation"),
		ProviderCandidates: append([]assemblycontract.ProviderBindingCandidateV1(nil), normalized.ProviderBindingCandidates...),
	}
	handoff.Digest, err = assemblycontract.HandoffDigestV1(handoff)
	if err != nil {
		return assemblycontract.CompileResultV1{}, err
	}
	if err := handoff.Validate(); err != nil {
		return assemblycontract.CompileResultV1{}, err
	}

	return assemblycontract.CompileResultV1{Generation: &generation, Manifest: &manifest, Graph: &graph, Handoff: &handoff, Diagnostics: diagnostics, Residuals: residuals}, nil
}

func rejected(code, objectPath, fieldPath, owner, expected, actual string, cause error) (assemblycontract.CompileResultV1, error) {
	diagnostic := assemblycontract.AssemblyDiagnosticV1{Severity: assemblycontract.DiagnosticErrorV1, Code: code, ObjectPath: objectPath, FieldPath: fieldPath, Owner: owner, Expected: expected, Actual: actual, Remediation: "correct the immutable input and compile a new generation"}
	return assemblycontract.CompileResultV1{Diagnostics: []assemblycontract.AssemblyDiagnosticV1{diagnostic}, Residuals: []assemblycontract.ResidualReportV1{}}, cause
}

func validateDeclaredDigests(input assemblycontract.AssemblyInputV1) error {
	for _, value := range input.Slots {
		digest, err := assemblycontract.SlotSpecDigestV1(value)
		if err != nil || digest != value.Digest {
			return fmt.Errorf("slot %s digest mismatch", value.SlotID)
		}
	}
	for _, value := range input.SlotContributions {
		digest, err := assemblycontract.SlotContributionDigestV1(value)
		if err != nil || digest != value.Digest {
			return fmt.Errorf("slot contribution %s digest mismatch", value.ContributionID)
		}
	}
	for _, value := range input.HookFaces {
		digest, err := assemblycontract.HookFaceSpecDigestV1(value)
		if err != nil || digest != value.Digest {
			return fmt.Errorf("hookface %s digest mismatch", value.HookFaceID)
		}
	}
	for _, value := range input.PhaseContributions {
		digest, err := assemblycontract.PhaseContributionDigestV1(value)
		if err != nil || digest != value.Digest {
			return fmt.Errorf("phase contribution %s digest mismatch", value.ContributionID)
		}
	}
	for _, value := range input.ProviderBindingCandidates {
		digest, err := assemblycontract.ProviderBindingCandidateDigestV1(value)
		if err != nil || digest != value.Digest {
			return fmt.Errorf("provider candidate %s digest mismatch", value.CandidateID)
		}
	}
	return nil
}

func compileResiduals(input assemblycontract.AssemblyInputV1, index indexes) ([]assemblycontract.ResidualReportV1, error) {
	allowed := make(map[string]struct{}, len(input.Policy.AllowResidualClasses))
	for _, class := range input.Policy.AllowResidualClasses {
		allowed[class] = struct{}{}
	}
	result := []assemblycontract.ResidualReportV1{}
	for _, manifest := range input.ComponentManifests {
		if manifest.ResidualClass == runtimeports.ResidualNone {
			continue
		}
		class := string(manifest.ResidualClass)
		if _, ok := allowed[class]; !ok {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonRemoteResidualUnresolved, "component residual class is not allowed by Assembly policy")
		}
		owner := string(manifest.ComponentID)
		inspect, cleanup := assemblycontract.InspectContractRefV1{}, assemblycontract.CleanupContractRefV1{}
		for _, factory := range input.Factories {
			if module, ok := index.modules[factory.ModuleRef]; ok && module.ComponentManifestRef.ID == owner {
				cleanup = factory.CleanupContractRef
				break
			}
		}
		for _, port := range input.PortSpecs {
			for _, contribution := range input.SlotContributions {
				if contribution.ModuleRef == "" || contribution.PortSpecRef != port.PortID {
					continue
				}
				module, ok := index.modules[contribution.ModuleRef]
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
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonRemoteResidualUnresolved, "allowed residual lacks module-owned Inspect or Cleanup contract")
		}
		if inspect.OwnerCapability != cleanup.OwnerCapability {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonSettlementOwnerMismatch, "Residual Inspect and Cleanup contracts do not share the same domain owner capability")
		}
		result = append(result, assemblycontract.ResidualReportV1{ResidualClass: class, Owner: owner, Scope: input.ScopeRef, InspectContractRef: inspect, CleanupContractRef: cleanup, Allowed: true, BlockingStage: "none"})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ResidualClass != result[j].ResidualClass {
			return result[i].ResidualClass < result[j].ResidualClass
		}
		return result[i].Owner < result[j].Owner
	})
	return result, nil
}
