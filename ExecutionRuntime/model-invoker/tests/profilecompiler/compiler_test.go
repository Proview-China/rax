package profilecompiler_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestSixRepresentativeRoutesCompileToDeterministicGoldens(t *testing.T) {
	compiler, _ := compilerForTest(t)
	goldens := map[profile.ProfileID]map[union.IntentID]string{
		profile.ProfileOpenAIDirect: {
			"i1": "openai.caller.apply_patch", "i2": "openai.caller.apply_patch",
			"i3": "openai.caller.process", "i4": "openai.responses.json_schema",
		},
		profile.ProfileCodex: {
			"i1": "codex.apply_patch", "i2": "codex.apply_patch",
			"i3": "codex.shell", "i4": "codex.output_schema",
		},
		profile.ProfileClaudeSDK: {
			"i1": "claude.edit", "i2": "claude.write",
			"i3": "claude.bash", "i4": "claude.output_format",
		},
		profile.ProfileGeminiCLI: {
			"i1": "gemini.replace", "i2": "gemini.write_file",
			"i3": "gemini.run_shell_command", "i4": "praxis.gemini.schema_repair",
		},
		profile.ProfileKimiCLI: {
			"i1": "kimi.edit", "i2": "kimi.write",
			"i3": "kimi.shell", "i4": "praxis.kimi.schema_repair",
		},
		profile.ProfileQwenSDK: {
			"i1": "qwen.edit", "i2": "qwen.write",
			"i3": "qwen.bash", "i4": "praxis.qwen.schema_repair",
		},
	}
	for profileID, expected := range goldens {
		profileID, expected := profileID, expected
		t.Run(string(profileID), func(t *testing.T) {
			first, err := compiler.Compile(paperCompileInput(profileID))
			if err != nil {
				t.Fatalf("first Compile() error = %v", err)
			}
			second, err := compiler.Compile(paperCompileInput(profileID))
			if err != nil {
				t.Fatalf("second Compile() error = %v", err)
			}
			if first.Plan.Digest == "" || first.Plan.Digest != second.Plan.Digest ||
				first.Plan.RouteFingerprint != second.Plan.RouteFingerprint {
				t.Fatalf("non-deterministic plan: first=%q/%q second=%q/%q",
					first.Plan.Digest, first.Plan.RouteFingerprint, second.Plan.Digest, second.Plan.RouteFingerprint)
			}
			if err := first.Plan.Validate(); err != nil {
				t.Fatalf("PreparedExecutionPlan.Validate() error = %v", err)
			}
			got := map[union.IntentID]string{}
			for _, decision := range first.MappingReport.Decisions {
				intentID := union.IntentID(strings.TrimPrefix(decision.SourcePath, "intent_graph."))
				got[intentID] = strings.TrimPrefix(decision.TargetPath, "mechanisms.")
			}
			for intentID, mechanismID := range expected {
				if got[intentID] != mechanismID {
					t.Errorf("Intent %s primary = %q, want %q", intentID, got[intentID], mechanismID)
				}
			}
			if len(first.Plan.Residuals) != 1 || first.Plan.Residuals[0].Kind != "probe_not_run" {
				t.Fatalf("paper-only residuals = %#v", first.Plan.Residuals)
			}
		})
	}
}

func TestN02ExtraExecutableToolFailsBeforeMechanismCompilation(t *testing.T) {
	compiler, profiles := compilerForTest(t)
	selected := profileByID(t, profiles, profile.ProfileCodex)
	actual := actualFromExpected(selected.HarnessCapability.ExpectedManifest)
	actual.Fields = append(actual.Fields, profile.ManifestField{
		Path: "tools.unexpected.executable", State: profile.ManifestFieldPresent, Value: "true",
		Evidence: profile.ManifestEvidence{
			Source: profile.ManifestEvidenceObserved, Confidence: 100, Reference: "fixture://unexpected",
		},
	})
	input := paperCompileInput(profile.ProfileCodex)
	input.PaperOnly = false
	input.ActualManifest = actual
	_, err := compiler.Compile(input)
	assertProfileErrorCode(t, err, profile.ErrorManifestDrift)
}

func TestN08CurrentKimiRejectsLegacyWire(t *testing.T) {
	compiler, _ := compilerForTest(t)
	input := paperCompileInput(profile.ProfileKimiCLI)
	input.RequiredNativeFeatures = []string{"legacy_wire"}
	_, err := compiler.Compile(input)
	assertProfileErrorCode(t, err, profile.ErrorProfileIncompatible)
}

func TestN09QwenBareAndCoreToolsAreMutuallyExclusive(t *testing.T) {
	compiler, _ := compilerForTest(t)
	input := paperCompileInput(profile.ProfileQwenSDK)
	input.RequiredNativeFeatures = []string{"bare", "core_tools"}
	_, err := compiler.Compile(input)
	assertProfileErrorCode(t, err, profile.ErrorProfileIncompatible)
}

func TestN13AutomaticModelFallbackRequiresNewResolution(t *testing.T) {
	compiler, _ := compilerForTest(t)
	input := paperCompileInput(profile.ProfileOpenAIDirect)
	input.AutomaticModelFallback = true
	_, err := compiler.Compile(input)
	assertProfileErrorCode(t, err, profile.ErrorProfileIncompatible)
}

func TestN14UnavailableComputerUseIsCapabilityRejected(t *testing.T) {
	compiler, _ := compilerForTest(t)
	input := paperCompileInput(profile.ProfileOpenAIDirect)
	input.Request.IntentGraph = union.IntentGraph{Nodes: []union.IntentNode{{
		ID: "computer", Kind: union.IntentComputerUse, Target: "desktop",
		Required: true, AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact},
	}}}
	_, err := compiler.Compile(input)
	assertProfileErrorCode(t, err, profile.ErrorCapabilityRejected)
}

func TestFileAdministrationAndToolUnionPrimitivesRequireExplicitPolicyThenCompile(t *testing.T) {
	_, profiles := compilerForTest(t)
	selected := profileByID(t, profiles, profile.ProfileOpenAIDirect)
	deniedRegistry, err := profile.NewRegistry(testNow, selected)
	if err != nil {
		t.Fatal(err)
	}
	deniedCompiler, _ := profile.NewCompiler(deniedRegistry, testNow)
	request := canonicalRequest(profile.ProfileOpenAIDirect)
	request.IntentGraph = administrativeIntentGraph()
	_, err = deniedCompiler.Compile(profile.CompileInput{
		Request: request, PaperOnly: true,
		ActualManifest: profile.InjectionManifest{SchemaVersion: "v1candidate", ProbeStatus: profile.ManifestProbeNotRun},
	})
	assertProfileErrorCode(t, err, profile.ErrorPolicyRejected)

	selected.DefaultPolicy.Filesystem.WritablePaths = profile.PathSetConstraint{Specified: true, Values: []string{"/workspace"}}
	selected.DefaultPolicy.Filesystem.AllowDelete = true
	selected.DefaultPolicy.Filesystem.AllowMove = true
	registry, err := profile.NewRegistry(testNow, selected)
	if err != nil {
		t.Fatal(err)
	}
	compiler, _ := profile.NewCompiler(registry, testNow)
	compiled, err := compiler.Compile(profile.CompileInput{
		Request: request, PaperOnly: true,
		ActualManifest: profile.InjectionManifest{SchemaVersion: "v1candidate", ProbeStatus: profile.ManifestProbeNotRun},
	})
	if err != nil {
		t.Fatalf("Compile(widened policy): %v", err)
	}
	want := map[union.IntentID]string{
		"delete-file": "openai.caller.delete_path", "move-file": "openai.caller.move_path",
		"create-dir": "openai.caller.directory", "delete-dir": "openai.caller.delete_path",
		"call-tool": "openai.caller.function",
	}
	for _, decision := range compiled.MappingReport.Decisions {
		intentID := union.IntentID(strings.TrimPrefix(decision.SourcePath, "intent_graph."))
		if expected := want[intentID]; expected != "" && decision.TargetPath != "mechanisms."+expected {
			t.Errorf("%s mapped to %q, want %q", intentID, decision.TargetPath, expected)
		}
		delete(want, intentID)
	}
	if len(want) != 0 {
		t.Fatalf("missing union primitive mappings: %#v", want)
	}
}

func administrativeIntentGraph() union.IntentGraph {
	return union.IntentGraph{Nodes: []union.IntentNode{
		{ID: "delete-file", Kind: union.IntentDeleteFile, Target: "/workspace/old.txt", Required: true, AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact}},
		{ID: "move-file", Kind: union.IntentMoveFile, Target: "/workspace/source.txt", Required: true, Specification: json.RawMessage(`{"destination":"/workspace/destination.txt"}`), AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact}},
		{ID: "create-dir", Kind: union.IntentCreateDirectory, Target: "/workspace/new-dir", Required: true, AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact}},
		{ID: "delete-dir", Kind: union.IntentDeleteDirectory, Target: "/workspace/old-dir", Required: true, AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact}},
		{ID: "call-tool", Kind: union.IntentCallTool, Target: "workspace.inspect", Required: true, AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact}},
	}}
}

func TestHardFilterRunsBeforeScoreAndSelectsAllowedFallback(t *testing.T) {
	compiler, profiles := compilerForTest(t)
	selected := profileByID(t, profiles, profile.ProfileOpenAIDirect)
	input := paperCompileInput(profile.ProfileOpenAIDirect)
	input.PolicyLayers = []profile.PolicyLayer{{
		ID: "task.deny-native-schema", Scope: profile.PolicyScopeTask,
		Identity:           selected.DefaultPolicy.Identity,
		DeniedMechanismIDs: []string{"openai.responses.json_schema"},
	}}
	compiled, err := compiler.Compile(input)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	for _, decision := range compiled.MappingReport.Decisions {
		if decision.SourcePath == "intent_graph.i4" {
			if decision.TargetPath != "mechanisms.praxis.schema.repair" {
				t.Fatalf("I4 target = %q, want hard-filtered Praxis repair", decision.TargetPath)
			}
			return
		}
	}
	t.Fatal("I4 mapping decision missing")
}

func TestExactActualManifestCompilesAndContributesToFingerprint(t *testing.T) {
	compiler, profiles := compilerForTest(t)
	selected := profileByID(t, profiles, profile.ProfileClaudeSDK)
	input := paperCompileInput(profile.ProfileClaudeSDK)
	input.PaperOnly = false
	input.ActualManifest = actualFromExpected(selected.HarnessCapability.ExpectedManifest)
	compiled, err := compiler.Compile(input)
	if err != nil {
		t.Fatalf("Compile(actual manifest) error = %v", err)
	}
	if !compiled.ManifestEvaluation.Allowed || len(compiled.ManifestEvaluation.Differences) != 0 ||
		compiled.Plan.Metadata["actual_manifest_digest"] == "" {
		t.Fatalf("actual manifest result = %#v", compiled.ManifestEvaluation)
	}
}
