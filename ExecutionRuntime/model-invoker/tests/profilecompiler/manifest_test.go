package profilecompiler_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
)

func TestManifestEvidenceAndP0ThroughP3Classification(t *testing.T) {
	_, profiles := representativeRegistry(t)
	selected := profileByID(t, profiles, profile.ProfileCodex)
	actual := actualFromExpected(selected.HarnessCapability.ExpectedManifest)
	for index := range actual.Fields {
		if actual.Fields[index].Path == "auth.route" {
			actual.Fields[index].Value = "unexpected_auth_route"
		}
	}
	actual.Fields = append(actual.Fields,
		profile.ManifestField{
			Path: "tools.unexpected.executable", State: profile.ManifestFieldPresent, Value: "true",
			Evidence: profile.ManifestEvidence{
				Source: profile.ManifestEvidenceObserved, Confidence: 100, Reference: "fixture://unexpected",
			},
		},
		profile.ManifestField{
			Path: "telemetry.label", State: profile.ManifestFieldPresent, Value: "new",
			Evidence: profile.ManifestEvidence{
				Source: profile.ManifestEvidenceReported, Confidence: 100, Reference: "fixture://telemetry",
			},
		},
	)
	evaluation, err := profile.CompareManifests(
		selected.HarnessCapability.ExpectedManifest, actual, profile.ContextSemanticStable, nil,
	)
	if err != nil {
		t.Fatalf("CompareManifests() error = %v", err)
	}
	if evaluation.Allowed {
		t.Fatal("semantic_stable accepted an extra executable tool")
	}
	levels := map[string]profile.ManifestDriftLevel{}
	for _, difference := range evaluation.Differences {
		levels[difference.Path] = difference.Level
	}
	if levels["auth.route"] != profile.ManifestDriftP0 ||
		levels["tools.unexpected.executable"] != profile.ManifestDriftP1 ||
		levels["telemetry.label"] != profile.ManifestDriftP3 {
		t.Fatalf("manifest levels = %#v", levels)
	}
}

func TestP2DriftRequiresExplicitSemanticStableAllowance(t *testing.T) {
	_, profiles := representativeRegistry(t)
	selected := profileByID(t, profiles, profile.ProfileClaudeSDK)
	actual := actualFromExpected(selected.HarnessCapability.ExpectedManifest)
	for index := range actual.Fields {
		if actual.Fields[index].Path == "event.fidelity" {
			actual.Fields[index].Value = "reduced"
		}
	}
	denied, err := profile.CompareManifests(
		selected.HarnessCapability.ExpectedManifest, actual, profile.ContextSemanticStable, nil,
	)
	if err != nil || denied.Allowed {
		t.Fatalf("unacknowledged P2 result = %#v, %v", denied, err)
	}
	allowed, err := profile.CompareManifests(
		selected.HarnessCapability.ExpectedManifest, actual, profile.ContextSemanticStable, []string{"event.fidelity"},
	)
	if err != nil || !allowed.Allowed || len(allowed.Residuals) != 1 {
		t.Fatalf("acknowledged P2 result = %#v, %v", allowed, err)
	}
}
