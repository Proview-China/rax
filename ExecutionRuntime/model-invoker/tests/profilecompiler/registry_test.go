package profilecompiler_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestRepresentativeRegistryCarriesSixExactSelectionKeys(t *testing.T) {
	registry, profiles := representativeRegistry(t)
	if got := len(registry.IDs()); got != 6 || len(profiles) != 6 {
		t.Fatalf("representative Profiles = registry %d profiles %d, want 6", got, len(profiles))
	}
	for _, candidate := range profiles {
		if candidate.Selection.BaseRouteID == "" || candidate.Selection.ModelID == "" ||
			candidate.Selection.ModelRevision == "" || candidate.Selection.ExecutionSurface == "" {
			t.Fatalf("Profile %q has incomplete exact key: %#v", candidate.ID, candidate.Selection)
		}
		if candidate.Selection.ExecutionSurface == profile.ExecutionSurfaceDirectAPI {
			if len(candidate.Selection.HarnessStack) != 0 {
				t.Fatalf("Direct Profile %q has a Harness stack", candidate.ID)
			}
		} else if len(candidate.Selection.HarnessStack) == 0 {
			t.Fatalf("Harness Profile %q has no component stack", candidate.ID)
		}
		resolved, err := registry.Resolve(profile.ProfileSelector{ID: candidate.ID})
		if err != nil || resolved.ID != candidate.ID {
			t.Fatalf("Resolve(%q) = %#v, %v", candidate.ID, resolved.ID, err)
		}
	}
}

func TestN01AmbiguousSelectorFailsWithoutCompilation(t *testing.T) {
	compiler, _ := compilerForTest(t)
	request := canonicalRequest(profile.ProfileOpenAIDirect)
	request.ProfileSelector = union.ProfileSelector{Constraints: map[string]string{"provider": "openai"}}
	_, err := compiler.Compile(profile.CompileInput{
		Request: request,
		ActualManifest: profile.InjectionManifest{
			SchemaVersion: "v1candidate", ProbeStatus: profile.ManifestProbeNotRun,
		},
		PaperOnly: true,
	})
	typed := assertProfileErrorCode(t, err, profile.ErrorResolutionFailed)
	if len(typed.Candidates) != 2 {
		t.Fatalf("ambiguous candidates = %#v, want OpenAI Direct and Codex", typed.Candidates)
	}
}

func TestHarnessStackDigestIsPartOfConstraintResolution(t *testing.T) {
	registry, profiles := representativeRegistry(t)
	codex := profileByID(t, profiles, profile.ProfileCodex)
	digest, err := codex.Selection.HarnessStackDigest()
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := registry.Resolve(profile.ProfileSelector{Constraints: profile.SelectionConstraints{
		Provider: "openai", ExecutionSurface: profile.ExecutionSurfaceAppServer, HarnessStackDigest: digest,
	}})
	if err != nil || resolved.ID != profile.ProfileCodex {
		t.Fatalf("Harness stack resolution = %q, %v", resolved.ID, err)
	}
	if _, err := registry.Resolve(profile.ProfileSelector{Constraints: profile.SelectionConstraints{
		Provider: "openai", ExecutionSurface: profile.ExecutionSurfaceAppServer,
		HarnessStackDigest: profile.DigestString("wrong-stack"),
	}}); err == nil {
		t.Fatal("wrong Harness stack digest resolved a Profile")
	}
}
