package assemblycompiler

import (
	"slices"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func compileValid(t *testing.T, input assemblycontract.AssemblyInputV1) assemblycontract.CompileResultV1 {
	t.Helper()
	result, err := New().Compile(input)
	if err != nil {
		t.Fatalf("compile: %v (%+v)", err, result.Diagnostics)
	}
	return result
}

func reseal(t *testing.T, input assemblycontract.AssemblyInputV1) assemblycontract.AssemblyInputV1 {
	t.Helper()
	sealed, err := assemblycontract.SealAssemblyInputV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func TestCompileSealsPreBindingArtifactsDeterministically(t *testing.T) {
	t.Parallel()
	input := assemblytestkit.ValidInput()
	first := compileValid(t, input)
	reordered := input
	reordered.SlotContributions = slices.Clone(input.SlotContributions)
	slices.Reverse(reordered.SlotContributions)
	reordered.Capabilities = slices.Clone(input.Capabilities)
	slices.Reverse(reordered.Capabilities)
	reordered.HookFaces = slices.Clone(input.HookFaces)
	slices.Reverse(reordered.HookFaces)
	second := compileValid(t, reseal(t, reordered))
	if first.Generation.State != assemblycontract.AssemblyStateSealedV1 || first.Manifest.Digest != second.Manifest.Digest || first.Graph.Digest != second.Graph.Digest || first.Generation.Digest != second.Generation.Digest {
		t.Fatal("semantically identical inputs did not produce identical sealed artifacts")
	}
	if first.Handoff.GenerationRef.Digest != first.Generation.Digest || first.Handoff.ManifestDigest != first.Manifest.Digest || first.Handoff.GraphDigest != first.Graph.Digest {
		t.Fatal("handoff does not bind exact artifact digests")
	}
}

func TestCompileRejectsCatalogCardinalityAndOwnerDrift(t *testing.T) {
	t.Parallel()
	t.Run("catalog", func(t *testing.T) {
		input := assemblytestkit.ValidInput()
		for index := range input.Slots {
			if input.Slots[index].SlotID == "kernel.loop" {
				input.Slots[index].Required = false
				break
			}
		}
		input = reseal(t, input)
		result, err := New().Compile(input)
		if err == nil || result.Graph != nil || result.Diagnostics[0].Code != "catalog_mismatch" {
			t.Fatalf("catalog drift accepted: %v %+v", err, result)
		}
	})
	t.Run("cardinality", func(t *testing.T) {
		input := assemblytestkit.ValidInput()
		duplicate := input.SlotContributions[0]
		duplicate.ContributionID = "praxis.fixture/kernel-duplicate"
		input.SlotContributions = append(input.SlotContributions, duplicate)
		input = reseal(t, input)
		result, err := New().Compile(input)
		if err == nil || result.Diagnostics[0].Code != "cardinality_conflict" {
			t.Fatalf("duplicate active slot accepted: %v %+v", err, result.Diagnostics)
		}
	})
	t.Run("owner", func(t *testing.T) {
		input := assemblytestkit.ValidInput()
		input.Capabilities[0].OwnerCapability = "praxis.wrong/owner"
		input = reseal(t, input)
		result, err := New().Compile(input)
		if err == nil || result.Diagnostics[0].Code != "reference_conflict" {
			t.Fatalf("owner drift accepted: %v %+v", err, result.Diagnostics)
		}
	})
}

func TestCompileRejectsSchemaCycleWriteSetAndUnknownResidual(t *testing.T) {
	t.Parallel()
	t.Run("schema", func(t *testing.T) {
		input := assemblytestkit.ValidInput()
		input.PortSpecs[0].RequestSchema = assemblycontract.SlotCatalogV1()[0].InputSchema
		input = reseal(t, input)
		result, err := New().Compile(input)
		if err == nil || result.Diagnostics[0].Code != "reference_conflict" {
			t.Fatalf("schema drift accepted: %v %+v", err, result.Diagnostics)
		}
	})
	t.Run("cycle", func(t *testing.T) {
		input := assemblytestkit.ValidInput()
		a, b := input.SlotContributions[0].ContributionID, input.SlotContributions[1].ContributionID
		input.SlotContributions[0].Dependencies = []string{b}
		input.SlotContributions[1].Dependencies = []string{a}
		input = reseal(t, input)
		result, err := New().Compile(input)
		if err == nil || !core.HasReason(err, core.ReasonDependencyCycle) || result.Diagnostics[0].Code != "dependency_conflict" {
			t.Fatalf("cycle accepted: %v %+v", err, result.Diagnostics)
		}
	})
	t.Run("write_set", func(t *testing.T) {
		input := assemblytestkit.ValidInput()
		var hook assemblycontract.HookFaceSpecV1
		for _, value := range input.HookFaces {
			if value.Kind == assemblycontract.PhaseFilterV1 {
				hook = value
				break
			}
		}
		module := input.Modules[0].ModuleID
		path := hook.MutationMask[0]
		input.PhaseContributions = []assemblycontract.PhaseContributionV1{{ContributionID: "praxis.fixture/filter-a", HookFaceRef: hook.HookFaceID, HandlerDescriptorRef: assemblytestkit.Ref("handler-a"), ModuleRef: module, Capability: assemblycontract.PhaseFilterV1, WriteSet: []string{path}}, {ContributionID: "praxis.fixture/filter-b", HookFaceRef: hook.HookFaceID, HandlerDescriptorRef: assemblytestkit.Ref("handler-b"), ModuleRef: module, Capability: assemblycontract.PhaseFilterV1, WriteSet: []string{path}}}
		input = reseal(t, input)
		result, err := New().Compile(input)
		if err == nil || result.Diagnostics[0].Code != "phase_conflict" {
			t.Fatalf("overlapping write set accepted: %v %+v", err, result.Diagnostics)
		}
	})
	t.Run("unknown_hookface", func(t *testing.T) {
		input := assemblytestkit.ValidInput()
		input.PhaseContributions = []assemblycontract.PhaseContributionV1{{ContributionID: "praxis.fixture/unknown-hook", HookFaceRef: "praxis.fixture/not-in-catalog", HandlerDescriptorRef: assemblytestkit.Ref("handler-unknown"), ModuleRef: input.Modules[0].ModuleID, Capability: assemblycontract.PhaseObserverV1}}
		input = reseal(t, input)
		result, err := New().Compile(input)
		if err == nil || result.Diagnostics[0].Code != "reference_conflict" {
			t.Fatalf("unknown HookFace accepted: %v %+v", err, result.Diagnostics)
		}
	})
	t.Run("residual", func(t *testing.T) {
		input := assemblytestkit.ValidInput()
		input.ComponentManifests[0].ResidualClass = runtimeports.ResidualInspectable
		input.Modules[0].ResidualClass = runtimeports.ResidualInspectable
		digest, _ := input.ComponentManifests[0].BindingDigestV2()
		input.Modules[0].ComponentManifestRef.Digest = digest
		input = reseal(t, input)
		result, err := New().Compile(input)
		if err == nil || result.Diagnostics[0].Code != "residual_not_allowed" {
			t.Fatalf("unapproved residual accepted: %v %+v", err, result.Diagnostics)
		}
	})
}

func TestCompileAllowedResidualRequiresOwnedInspectAndCleanup(t *testing.T) {
	t.Parallel()
	input := assemblytestkit.ValidInput()
	input.ComponentManifests[0].ResidualClass = runtimeports.ResidualInspectable
	input.Modules[0].ResidualClass = runtimeports.ResidualInspectable
	digest, err := input.ComponentManifests[0].BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	input.Modules[0].ComponentManifestRef.Digest = digest
	input.Policy.AllowResidualClasses = []string{string(runtimeports.ResidualInspectable)}
	input = reseal(t, input)
	result := compileValid(t, input)
	if len(result.Residuals) != 1 || !result.Residuals[0].Allowed || result.Residuals[0].InspectContractRef.Ref.ID == "" || result.Residuals[0].CleanupContractRef.Ref.ID == "" {
		t.Fatalf("allowed residual is incomplete: %+v", result.Residuals)
	}

	missingCleanup := input
	missingCleanup.Factories = nil
	missingCleanup = reseal(t, missingCleanup)
	rejected, err := New().Compile(missingCleanup)
	if err == nil || rejected.Diagnostics[0].Code != "residual_not_allowed" {
		t.Fatalf("residual without Cleanup accepted: %v %+v", err, rejected.Diagnostics)
	}
}

func TestCompileEffectContractsKeepRunRequirementsOptionalAndDistinct(t *testing.T) {
	t.Parallel()
	input := assemblytestkit.ValidInput()
	result := compileValid(t, input)
	if len(result.Manifest.PortSpecs[0].RunStartRequirementRefs) != 0 || len(result.Manifest.PortSpecs[0].RunSettlementRequirementRefs) != 0 {
		t.Fatal("effectful Port unexpectedly forced Run Start or Run Settlement requirements")
	}

	input.PortSpecs[0].RunStartRequirementRefs = []assemblycontract.RunStartRequirementRefV1{{Ref: assemblytestkit.Ref("run-start-requirement"), RequirementID: "praxis.fixture/run-start", OwnerCapability: input.PortSpecs[0].OwnerCapability}}
	input.PortSpecs[0].RunSettlementRequirementRefs = []assemblycontract.RunSettlementRequirementRefV1{{Ref: assemblytestkit.Ref("run-settlement-requirement"), RequirementID: "praxis.fixture/run-settlement", OwnerCapability: input.PortSpecs[0].OwnerCapability}}
	input = reseal(t, input)
	result = compileValid(t, input)
	port := result.Manifest.PortSpecs[0]
	if len(port.RunStartRequirementRefs) != 1 || len(port.RunSettlementRequirementRefs) != 1 || port.RunStartRequirementRefs[0].Ref.ID == port.RunSettlementRequirementRefs[0].Ref.ID {
		t.Fatalf("typed Run requirements were merged or lost: %+v", port)
	}

	missingCases := []struct {
		name   string
		mutate func(*assemblycontract.PortSpecV1)
	}{
		{"operation_scope", func(p *assemblycontract.PortSpecV1) { p.OperationScopeRef = nil }},
		{"inspect", func(p *assemblycontract.PortSpecV1) { p.InspectContractRef = nil }},
		{"domain_result", func(p *assemblycontract.PortSpecV1) { p.DomainResultContractRef = nil }},
		{"runtime_settlement_ref", func(p *assemblycontract.PortSpecV1) { p.RuntimeOperationSettlementRefContract = nil }},
		{"apply_settlement", func(p *assemblycontract.PortSpecV1) { p.ApplySettlementContractRef = nil }},
	}
	for _, test := range missingCases {
		t.Run(test.name, func(t *testing.T) {
			candidate := assemblytestkit.ValidInput()
			test.mutate(&candidate.PortSpecs[0])
			digest, err := assemblycontract.AssemblyInputDigestV1(candidate)
			if err != nil {
				t.Fatal(err)
			}
			candidate.Digest = digest
			rejected, err := New().Compile(candidate)
			if !core.HasReason(err, core.ReasonEffectSettlementMissing) || rejected.Diagnostics[0].Code != "input_invalid" {
				t.Fatalf("missing %s contract accepted: %v %+v", test.name, err, rejected.Diagnostics)
			}
		})
	}
}

func TestResidualCompilerNeverTreatsRunRequirementAsInspect(t *testing.T) {
	t.Parallel()
	input := assemblytestkit.ValidInput()
	input.ComponentManifests[0].ResidualClass = runtimeports.ResidualInspectable
	input.Modules[0].ResidualClass = runtimeports.ResidualInspectable
	input.Policy.AllowResidualClasses = []string{string(runtimeports.ResidualInspectable)}
	input.PortSpecs[0].RunSettlementRequirementRefs = []assemblycontract.RunSettlementRequirementRefV1{{Ref: assemblytestkit.Ref("not-an-inspect-contract"), RequirementID: "praxis.fixture/run-settlement", OwnerCapability: input.PortSpecs[0].OwnerCapability}}
	input.PortSpecs[0].InspectContractRef = nil
	index, err := buildIndex(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compileResiduals(input, index); !core.HasReason(err, core.ReasonRemoteResidualUnresolved) {
		t.Fatalf("Run Settlement requirement substituted for Inspect contract: %v", err)
	}
}

func TestResolveDomainWildcardRequiresDomainPrefix(t *testing.T) {
	t.Parallel()
	input := assemblytestkit.ValidInput()
	base := input.SlotContributions[0]
	base.ContributionID, base.SlotRef = "praxis.fixture/domain-good", "domain.context-cache"
	evil := base
	evil.ContributionID, evil.SlotRef = "praxis.fixture/domain-evil", "not-domain-but-long-enough"
	input.SlotContributions = append(input.SlotContributions, base, evil)
	resolved := resolveSlots(input, nil)
	for _, slot := range resolved {
		if slot.SlotID == "domain.*" {
			if len(slot.Contributions) != 1 || slot.Contributions[0] != base.ContributionID {
				t.Fatalf("domain wildcard misclassified contribution: %+v", slot.Contributions)
			}
			return
		}
	}
	t.Fatal("domain wildcard missing")
}

func TestCompilerRejectsUniversalFilterCandidate(t *testing.T) {
	t.Parallel()
	input := assemblytestkit.ValidInput()
	var hook assemblycontract.HookFaceSpecV1
	for _, value := range input.HookFaces {
		if value.Kind == assemblycontract.PhaseFilterV1 {
			hook = value
			break
		}
	}
	phase := assemblycontract.PhaseContributionV1{ContractVersion: assemblycontract.ContractVersionV1, ContributionID: "praxis.fixture/universal-filter", HookFaceRef: hook.HookFaceID, HandlerDescriptorRef: assemblytestkit.Ref("universal-handler"), ModuleRef: input.Modules[0].ModuleID, Capability: assemblycontract.PhaseFilterV1, WriteSet: []string{"candidate.declared-write-set"}}
	phase.Digest, _ = assemblycontract.PhaseContributionDigestV1(phase)
	input.PhaseContributions = []assemblycontract.PhaseContributionV1{phase}
	input.Digest, _ = assemblycontract.AssemblyInputDigestV1(input)
	result, err := New().Compile(input)
	if !core.HasReason(err, core.ReasonPlanInvalid) || result.Diagnostics[0].Code != "input_invalid" {
		t.Fatalf("universal Filter candidate accepted: %v %+v", err, result.Diagnostics)
	}
}

func TestCompileFaultInjectionNeverReturnsPartialGraph(t *testing.T) {
	t.Parallel()
	input := assemblytestkit.ValidInput()
	input.Digest = assemblytestkit.Digest("lost-or-corrupt-input")
	result, err := New().Compile(input)
	if err == nil || result.Graph != nil || result.Manifest != nil || result.Handoff != nil || len(result.Diagnostics) != 1 {
		t.Fatalf("corrupt input returned partial executable artifacts: %v %+v", err, result)
	}
}

func FuzzCompileTamperedInputNeverPanics(f *testing.F) {
	f.Add([]byte("seed"))
	f.Fuzz(func(t *testing.T, payload []byte) {
		input := assemblytestkit.ValidInput()
		input.Digest = core.DigestBytes(payload)
		_, _ = New().Compile(input)
	})
}
