package failure_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestPromptUpstreamProvenanceByteDriftAndCancelReturnZeroV1(t *testing.T) {
	fixture := testkit.PromptProvenanceV1()
	request, err := kernel.SealVerifyPromptUpstreamProvenanceRequestV1(context.Background(), contract.VerifyPromptUpstreamProvenanceRequestV1{
		Provenance: fixture.Provenance, ArtifactBytes: fixture.Artifacts, LicenseBytes: fixture.License, GeneratedBytes: fixture.Generated,
		CheckedUnixNano: testkit.Now, MaxInputBytes: contract.MaxPromptUpstreamInputBytesV1,
	})
	if err != nil {
		t.Fatal(err)
	}

	tampered := request
	tampered.LicenseBytes = append([]byte(nil), request.LicenseBytes...)
	tampered.LicenseBytes[0] ^= 0xff
	if report, err := kernel.VerifyPromptUpstreamProvenanceV1(context.Background(), tampered); !errors.Is(err, contract.ErrConflict) || report.ReportDigest != "" {
		t.Fatalf("license drift report=%#v err=%v", report, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if report, err := kernel.VerifyPromptUpstreamProvenanceV1(ctx, request); !errors.Is(err, context.Canceled) || report.ReportDigest != "" {
		t.Fatalf("cancel report=%#v err=%v", report, err)
	}
}
