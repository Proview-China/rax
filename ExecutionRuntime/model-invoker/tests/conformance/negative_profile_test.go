package conformance_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
)

var negativeTestTime = time.Date(2026, 7, 13, 3, 0, 0, 0, time.UTC)

func representativeProfileRegistry(t *testing.T) *profile.Registry {
	t.Helper()
	profiles, err := profile.RepresentativeProfiles(negativeTestTime)
	if err != nil {
		t.Fatalf("RepresentativeProfiles: %v", err)
	}
	registry, err := profile.NewRegistry(negativeTestTime, profiles...)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return registry
}

func TestN01ProfileZeroMatchFailsClosed(t *testing.T) {
	registry := representativeProfileRegistry(t)

	_, err := registry.Resolve(profile.ProfileSelector{Constraints: profile.SelectionConstraints{
		Provider: "provider-that-does-not-exist",
	}})
	var typed *profile.Error
	if !errors.As(err, &typed) || typed.Code != profile.ErrorResolutionFailed {
		t.Fatalf("Resolve error = %#v, want %q", err, profile.ErrorResolutionFailed)
	}
	if len(typed.Candidates) != 0 {
		t.Fatalf("zero-match candidates = %#v, want none", typed.Candidates)
	}
}

func TestN02ProfileMultipleMatchesFailAsAmbiguous(t *testing.T) {
	registry := representativeProfileRegistry(t)

	_, err := registry.Resolve(profile.ProfileSelector{Constraints: profile.SelectionConstraints{
		Provider: "openai",
	}})
	var typed *profile.Error
	if !errors.As(err, &typed) || typed.Code != profile.ErrorResolutionFailed {
		t.Fatalf("Resolve error = %#v, want %q", err, profile.ErrorResolutionFailed)
	}
	if len(typed.Candidates) != 2 {
		t.Fatalf("ambiguous candidates = %#v, want exactly two OpenAI routes", typed.Candidates)
	}
}

func TestN08ManifestP0DriftIsNeverAllowed(t *testing.T) {
	expected := profile.InjectionManifest{
		SchemaVersion: "v1", ProbeStatus: profile.ManifestProbeReported,
		Fields: []profile.ManifestField{{Path: "auth.route", State: profile.ManifestFieldPresent, Value: "chatgpt_subscription"}},
	}
	actual := profile.InjectionManifest{
		SchemaVersion: "v1", ProbeStatus: profile.ManifestProbeObserved,
		Fields: []profile.ManifestField{{
			Path: "auth.route", State: profile.ManifestFieldPresent, Value: "api_key",
			Evidence: profile.ManifestEvidence{Source: profile.ManifestEvidenceObserved, Confidence: 100, Reference: "fixture://negative/N08/auth-route"},
		}},
	}

	evaluation, err := profile.CompareManifests(expected, actual, profile.ContextVendorDefault, []string{"auth.route"})
	if err != nil {
		t.Fatalf("CompareManifests: %v", err)
	}
	if evaluation.Allowed || len(evaluation.Differences) != 1 {
		t.Fatalf("P0 evaluation = %#v", evaluation)
	}
	difference := evaluation.Differences[0]
	if difference.Level != profile.ManifestDriftP0 || difference.Kind != profile.ManifestDiffValueChanged {
		t.Fatalf("P0 difference = %#v", difference)
	}
}

func TestN09OpaqueManifestFieldIsNotAbsent(t *testing.T) {
	expected := profile.InjectionManifest{
		SchemaVersion: "v1", ProbeStatus: profile.ManifestProbeReported,
		Fields: []profile.ManifestField{{Path: "instructions.system", State: profile.ManifestFieldAbsent}},
	}
	actual := profile.InjectionManifest{
		SchemaVersion: "v1", ProbeStatus: profile.ManifestProbeObserved,
		Fields: []profile.ManifestField{{
			Path: "instructions.system", State: profile.ManifestFieldOpaque,
			Evidence: profile.ManifestEvidence{Source: profile.ManifestEvidenceOpaque, Confidence: 0, Reference: "fixture://negative/N09/opaque"},
		}},
	}

	evaluation, err := profile.CompareManifests(expected, actual, profile.ContextSemanticStable, nil)
	if err != nil {
		t.Fatalf("CompareManifests: %v", err)
	}
	if evaluation.Allowed || len(evaluation.Differences) != 1 {
		t.Fatalf("opaque evaluation = %#v", evaluation)
	}
	difference := evaluation.Differences[0]
	if difference.Kind != profile.ManifestDiffBecameOpaque || difference.Actual == nil || difference.Actual.State != profile.ManifestFieldOpaque {
		t.Fatalf("opaque field was collapsed into absence: %#v", difference)
	}
}
