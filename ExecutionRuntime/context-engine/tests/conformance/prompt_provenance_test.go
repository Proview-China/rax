package conformance_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestPromptUpstreamProvenanceDoesNotOwnModelRouteOrPublicationV1(t *testing.T) {
	fixture := testkit.PromptProvenanceV1()
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
	payload, err := json.Marshal(struct {
		Provenance contract.PromptUpstreamProvenanceV1         `json:"provenance"`
		Report     contract.PromptUpstreamVerificationReportV1 `json:"report"`
	}{Provenance: fixture.Provenance, Report: report})
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(payload))
	for _, forbidden := range []string{"model_family", "route_id", "provider_driver_kind", "prompt_injected_values", "published", "actual_injection", "runtime_settlement", "continuation"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("cross-owner field %q escaped into provenance/report", forbidden)
		}
	}
}
