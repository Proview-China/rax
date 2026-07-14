package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestManifestAcceptsOnlyCurrentGovernedEvidence(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)
	manifest := testkit.Manifest(now, ports.ConformanceFullyControlled)
	if err := manifest.Validate(now); err != nil {
		t.Fatal(err)
	}

	expired := manifest
	expired.EvidenceExpiresAt = now
	if err := expired.Validate(now); !core.HasReason(err, core.ReasonCapabilityExpired) {
		t.Fatalf("expired manifest evidence must be rejected: %v", err)
	}

	ungoverned := manifest
	ungoverned.Conformance = ports.ConformanceContainedObserveOnly
	if err := ungoverned.Validate(now); !core.HasReason(err, core.ReasonComponentMismatch) {
		t.Fatalf("observe-only harness must not execute: %v", err)
	}
}

func TestBootstrapRejectsDigestDriftAndDuplicateResiduals(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)
	bootstrap := testkit.Manifest(now, ports.ConformanceRestrictedControlled).Bootstrap
	bootstrap.ProfileDigest = "sha256:drift"
	if err := bootstrap.Validate(now); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("invalid compiled digest must fail closed: %v", err)
	}

	bootstrap = testkit.Manifest(now, ports.ConformanceRestrictedControlled).Bootstrap
	bootstrap.AllowedResiduals = []string{"native_prompt", "native_prompt"}
	if err := bootstrap.Validate(now); !core.HasReason(err, core.ReasonPlanInvalid) {
		t.Fatalf("duplicate residual must fail closed: %v", err)
	}
}

func TestManifestCloneBreaksMutableSliceAliases(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)
	manifest := testkit.Manifest(now, ports.ConformanceFullyControlled)
	manifest.Bootstrap.AllowedResiduals = []string{"native_session"}
	clone := contract.CloneManifest(manifest)
	manifest.Capabilities[0] = "drifted"
	manifest.OpaqueBoundaries[0] = "drifted"
	manifest.Bootstrap.AllowedResiduals[0] = "drifted"
	if clone.Capabilities[0] != "interaction_loop" || clone.OpaqueBoundaries[0] != "provider_session" || clone.Bootstrap.AllowedResiduals[0] != "native_session" {
		t.Fatalf("manifest clone retained mutable aliases: %+v", clone)
	}
}

func TestOpaquePayloadRejectsContentReplacement(t *testing.T) {
	t.Parallel()
	payload := testkit.Payload("test.payload/v1", map[string]string{"value": "original"})
	payload.Payload = []byte(`{"value":"replaced"}`)
	if err := contract.ValidateOpaque(payload); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("payload replacement must fail digest verification: %v", err)
	}
}
