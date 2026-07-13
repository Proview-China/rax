package profilecompiler_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

var testNow = time.Date(2026, 7, 13, 1, 0, 0, 0, time.UTC)

func representativeRegistry(t *testing.T) (*profile.Registry, []profile.SemanticRouteProfile) {
	t.Helper()
	profiles, err := profile.RepresentativeProfiles(testNow)
	if err != nil {
		t.Fatalf("RepresentativeProfiles() error = %v", err)
	}
	registry, err := profile.NewRegistry(testNow, profiles...)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry, profiles
}

func compilerForTest(t *testing.T) (*profile.Compiler, []profile.SemanticRouteProfile) {
	t.Helper()
	registry, profiles := representativeRegistry(t)
	compiler, err := profile.NewCompiler(registry, testNow)
	if err != nil {
		t.Fatalf("NewCompiler() error = %v", err)
	}
	return compiler, profiles
}

func canonicalRequest(id profile.ProfileID) union.UnifiedExecutionRequest {
	return union.UnifiedExecutionRequest{
		SemanticVersion: union.SemanticVersionV1,
		ExecutionID:     union.ExecutionID("exec." + string(id)),
		ProfileSelector: union.ProfileSelector{
			Exact: &union.VersionedIdentity{ID: string(id), Version: "v1candidate"},
		},
		ExecutionKind: union.ExecutionKindAuto,
		ToolPolicy: union.ToolPolicy{
			DefaultApproval: "on_side_effect", Parallelism: 1, MaxActions: 8,
		},
		OutputContract: union.OutputContract{
			AcceptedContentKinds: []string{"json"}, CompletionMode: "final",
			JSONSchema: json.RawMessage("{\"type\":\"object\",\"additionalProperties\":false}"),
		},
		SessionIntent: union.SessionIntent{Mode: "new"},
		ExecutionPolicy: union.ExecutionPolicy{
			Sandbox: "workspace_write", CWDReference: "/workspace", NetworkPolicy: "denied",
			UserPresence: "present", Foreground: "required", InteractionMode: "interactive", MaxConcurrency: 1,
		},
		Budget: union.Budget{MaxWallTime: 120 * time.Second, MaxToolActions: 8},
		DegradationPolicy: union.DegradationPolicy{
			Default: union.DegradationDefaultReject,
		},
		IntentGraph: canonicalIntentGraph(),
	}
}

func canonicalIntentGraph() union.IntentGraph {
	return union.IntentGraph{Nodes: []union.IntentNode{
		{
			ID: "i4", Kind: union.IntentProduceStructured, Target: "summary",
			DependsOn: []union.IntentID{"i3"}, Required: true,
			AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact, union.SemanticFidelityTransformed},
		},
		{
			ID: "i2", Kind: union.IntentCreateFile, Target: "/workspace/internal/config/config_test.go",
			DependsOn: []union.IntentID{"i1"}, Required: true,
			Specification:    json.RawMessage("{\"content_digest\":\"sha256:fixture\"}"),
			AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact},
		},
		{
			ID: "i1", Kind: union.IntentModifyFile, Target: "/workspace/internal/config/config.go",
			Required: true, Specification: json.RawMessage("{\"before_hash\":\"H_CONFIG_0\",\"replace\":[\"legacy\",\"strict\"]}"),
			AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact},
		},
		{
			ID: "i3", Kind: union.IntentExecuteCode, Target: "./internal/config",
			DependsOn: []union.IntentID{"i1", "i2"}, Required: true,
			Specification:    json.RawMessage("{\"argv\":[\"go\",\"test\",\"./internal/config\"],\"cwd\":\"/workspace\",\"network\":false,\"timeout_ms\":120000}"),
			AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact},
		},
	}}
}

func paperCompileInput(id profile.ProfileID) profile.CompileInput {
	return profile.CompileInput{
		Request: canonicalRequest(id),
		ActualManifest: profile.InjectionManifest{
			SchemaVersion: "v1candidate", ProbeStatus: profile.ManifestProbeNotRun,
		},
		PaperOnly: true,
	}
}

func actualFromExpected(expected profile.InjectionManifest) profile.InjectionManifest {
	actual := expected.Clone()
	actual.ProbeStatus = profile.ManifestProbeObserved
	for index := range actual.Fields {
		actual.Fields[index].Evidence = profile.ManifestEvidence{
			Source: profile.ManifestEvidenceObserved, Confidence: 100,
			Reference: "fixture://actual/" + actual.Fields[index].Path,
		}
	}
	return actual
}

func profileByID(t *testing.T, profiles []profile.SemanticRouteProfile, id profile.ProfileID) profile.SemanticRouteProfile {
	t.Helper()
	for _, candidate := range profiles {
		if candidate.ID == id {
			return candidate
		}
	}
	t.Fatalf("Profile %q not found", id)
	return profile.SemanticRouteProfile{}
}

func assertProfileErrorCode(t *testing.T, err error, want profile.ErrorCode) *profile.Error {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %q", want)
	}
	typed, ok := err.(*profile.Error)
	if !ok || typed.Code != want {
		t.Fatalf("error = %#v, want code %q", err, want)
	}
	return typed
}
