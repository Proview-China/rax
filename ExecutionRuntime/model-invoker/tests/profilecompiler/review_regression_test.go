package profilecompiler_test

import (
	"encoding/json"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func compilerWithProfile(t *testing.T, selected profile.SemanticRouteProfile) *profile.Compiler {
	t.Helper()
	registry, err := profile.NewRegistry(testNow, selected)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	compiler, err := profile.NewCompiler(registry, testNow)
	if err != nil {
		t.Fatalf("NewCompiler: %v", err)
	}
	return compiler
}

func TestMechanismScoresAndPreferenceWeightsAreBounded(t *testing.T) {
	_, profiles := compilerForTest(t)
	selected := profileByID(t, profiles, profile.ProfileOpenAIDirect)
	selected.HarnessCapability.AvailableMechanisms[0].Score.ModelAffinity = 101
	if _, err := profile.NewRegistry(testNow, selected); err == nil {
		t.Fatal("registry accepted an out-of-range mechanism score")
	}

	base := profileByID(t, profiles, profile.ProfileOpenAIDirect).DefaultPolicy
	_, err := profile.MergeRuntimePolicy(base, profile.PolicyLayer{
		ID: "task.overflow", Scope: profile.PolicyScopeTask, Identity: base.Identity,
		MechanismPreferenceWeight: map[string]int{"openai.caller.apply_patch": int(^uint(0) >> 1)},
	})
	if err == nil {
		t.Fatal("policy merge accepted an overflow-scale preference weight")
	}
}

func TestProfileRegistryRejectsDefaultPolicyBoundToAnotherRoute(t *testing.T) {
	_, profiles := compilerForTest(t)
	selected := profileByID(t, profiles, profile.ProfileOpenAIDirect)
	selected.DefaultPolicy.Identity.ModelID = "different-model"
	if _, err := profile.NewRegistry(testNow, selected); err == nil {
		t.Fatal("registry accepted a default policy bound to another model")
	}
}

func TestCompilerEnforcesFilesystemDenyRoots(t *testing.T) {
	_, profiles := compilerForTest(t)
	selected := profileByID(t, profiles, profile.ProfileOpenAIDirect)
	selected.DefaultPolicy.Filesystem.WritablePaths = profile.PathSetConstraint{Specified: true, Values: []string{"/workspace"}}
	selected.DefaultPolicy.Filesystem.DeniedPaths = []string{"/workspace/private"}
	compiler := compilerWithProfile(t, selected)
	input := paperCompileInput(profile.ProfileOpenAIDirect)
	input.Request.IntentGraph = union.IntentGraph{Nodes: []union.IntentNode{{
		ID: "private", Kind: union.IntentModifyFile, Target: "/workspace/private/config.go", Required: true,
		AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact},
	}}}
	_, err := compiler.Compile(input)
	typed := assertProfileErrorCode(t, err, profile.ErrorPolicyRejected)
	if typed.Path != "/workspace/private/config.go" {
		t.Fatalf("denied path error = %#v", typed)
	}
}

func TestMoveCompilationRequiresPolicyAllowedExactDestination(t *testing.T) {
	_, profiles := compilerForTest(t)
	selected := profileByID(t, profiles, profile.ProfileOpenAIDirect)
	selected.DefaultPolicy.Filesystem.WritablePaths = profile.PathSetConstraint{Specified: true, Values: []string{"/workspace"}}
	selected.DefaultPolicy.Filesystem.DeniedPaths = []string{"/workspace/private"}
	selected.DefaultPolicy.Filesystem.AllowMove = true
	compiler := compilerWithProfile(t, selected)

	for _, test := range []struct {
		name string
		spec json.RawMessage
		path string
	}{
		{name: "missing destination", spec: nil, path: "move"},
		{name: "outside writable roots", spec: json.RawMessage(`{"destination":"/outside/destination.txt"}`), path: "/outside/destination.txt"},
		{name: "explicitly denied destination", spec: json.RawMessage(`{"destination":"/workspace/private/destination.txt"}`), path: "/workspace/private/destination.txt"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := paperCompileInput(profile.ProfileOpenAIDirect)
			input.Request.IntentGraph = union.IntentGraph{Nodes: []union.IntentNode{{
				ID: "move", Kind: union.IntentMoveFile, Target: "/workspace/source.txt", Specification: test.spec,
				Required: true, AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact},
			}}}
			_, err := compiler.Compile(input)
			typed := assertProfileErrorCode(t, err, profile.ErrorPolicyRejected)
			if typed.Path != test.path {
				t.Fatalf("move rejection path = %q, want %q", typed.Path, test.path)
			}
		})
	}

	valid := paperCompileInput(profile.ProfileOpenAIDirect)
	valid.Request.IntentGraph = union.IntentGraph{Nodes: []union.IntentNode{{
		ID: "move", Kind: union.IntentMoveFile, Target: "/workspace/source.txt",
		Specification: json.RawMessage(`{"destination":"/workspace/destination.txt"}`),
		Required:      true, AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact},
	}}}
	if _, err := compiler.Compile(valid); err != nil {
		t.Fatalf("valid move compilation: %v", err)
	}
}

func TestEitherFinalFallbackCapCanDisableFallbacks(t *testing.T) {
	_, profiles := compilerForTest(t)
	base := profileByID(t, profiles, profile.ProfileOpenAIDirect)
	for _, mutate := range []func(*profile.RuntimePolicy){
		func(policy *profile.RuntimePolicy) { policy.MaxFallbacks = 0; policy.RetryFallback.MaxFallbacks = 3 },
		func(policy *profile.RuntimePolicy) { policy.MaxFallbacks = 3; policy.RetryFallback.MaxFallbacks = 0 },
	} {
		selected := base.Clone()
		mutate(&selected.DefaultPolicy)
		compiled, err := compilerWithProfile(t, selected).Compile(paperCompileInput(profile.ProfileOpenAIDirect))
		if err != nil {
			t.Fatal(err)
		}
		plansPerIntent := make(map[union.IntentID]int)
		for _, plan := range compiled.Plan.Mechanisms {
			plansPerIntent[plan.IntentID]++
			if len(plan.FallbackPlanIDs) != 0 {
				t.Fatalf("zero fallback cap emitted fallback IDs: %#v", plan)
			}
		}
		for intentID, count := range plansPerIntent {
			if count != 1 {
				t.Fatalf("intent %q has %d mechanisms with zero fallback cap", intentID, count)
			}
		}
	}
}
