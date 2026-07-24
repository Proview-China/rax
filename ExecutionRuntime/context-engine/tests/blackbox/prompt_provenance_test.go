package blackbox_test

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestPromptUpstreamProvenanceOfflineBlackBoxV1(t *testing.T) {
	for name, fixture := range map[string]testkit.PromptProvenanceFixtureV1{
		"official_coding_agent": testkit.PromptProvenanceV1(),
		"opaque_sdk_preset":     testkit.PromptPresetReferenceProvenanceV1(),
	} {
		t.Run(name, func(t *testing.T) {
			request, err := kernel.SealVerifyPromptUpstreamProvenanceRequestV1(context.Background(), contract.VerifyPromptUpstreamProvenanceRequestV1{
				Provenance: fixture.Provenance, ArtifactBytes: fixture.Artifacts, LicenseBytes: fixture.License, GeneratedBytes: fixture.Generated,
				CheckedUnixNano: testkit.Now, MaxInputBytes: contract.MaxPromptUpstreamInputBytesV1,
			})
			if err != nil {
				t.Fatal(err)
			}
			report, err := kernel.VerifyPromptUpstreamProvenanceV1(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if report.ProvenanceRef.Digest != fixture.Provenance.ProvenanceDigest || report.ClosureDigest != fixture.Provenance.Closure.ClosureDigest || len(report.VerifiedContentRefs) != len(fixture.Generated) {
				t.Fatalf("offline provenance report drifted: %#v", report)
			}
		})
	}
}
