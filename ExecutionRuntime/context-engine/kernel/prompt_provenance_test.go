package kernel_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestVerifyPromptUpstreamProvenanceExactDeterministicV1(t *testing.T) {
	fixture := testkit.PromptProvenanceV1()
	request := sealPromptProvenanceRequestV1(t, fixture)
	report, err := kernel.VerifyPromptUpstreamProvenanceV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.VerifiedArtifactIDs) != 1 || len(report.VerifiedContentRefs) != 2 || report.SourceSetDigest != fixture.Provenance.SourceSetDigest || report.GeneratedSetDigest != fixture.Provenance.GeneratedSetDigest || report.ClosureDigest != fixture.Provenance.Closure.ClosureDigest {
		t.Fatalf("verification report drifted: %#v", report)
	}
	again, err := kernel.VerifyPromptUpstreamProvenanceV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if again.ReportDigest != report.ReportDigest {
		t.Fatal("same exact request produced a different report")
	}
}

func TestSealPromptUpstreamProvenanceRequestDeepCopiesV1(t *testing.T) {
	fixture := testkit.PromptProvenanceV1()
	request := contract.VerifyPromptUpstreamProvenanceRequestV1{
		Provenance: fixture.Provenance, ArtifactBytes: fixture.Artifacts, LicenseBytes: fixture.License, GeneratedBytes: fixture.Generated,
		CheckedUnixNano: testkit.Now, MaxInputBytes: contract.MaxPromptUpstreamInputBytesV1,
	}
	sealed, err := kernel.SealVerifyPromptUpstreamProvenanceRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.ArtifactBytes[0].Bytes[0] ^= 0xff
	request.LicenseBytes[0] ^= 0xff
	request.GeneratedBytes[0].Bytes[0] ^= 0xff
	if _, err := kernel.VerifyPromptUpstreamProvenanceV1(context.Background(), sealed); err != nil {
		t.Fatalf("sealed request aliased caller bytes: %v", err)
	}
	sealed.ArtifactBytes[0].Bytes[0] ^= 0xff
	if _, err := kernel.VerifyPromptUpstreamProvenanceV1(context.Background(), sealed); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("tampered sealed bytes error = %v", err)
	}
}

func TestVerifyPromptUpstreamPresetReferenceDoesNotInventBodyV1(t *testing.T) {
	fixture := testkit.PromptPresetReferenceProvenanceV1()
	request := sealPromptProvenanceRequestV1(t, fixture)
	report, err := kernel.VerifyPromptUpstreamProvenanceV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.VerifiedContentRefs) != 0 {
		t.Fatal("opaque SDK preset verification invented generated content")
	}
}

func TestVerifyPromptUpstreamProvenanceFailClosedV1(t *testing.T) {
	base := sealPromptProvenanceRequestV1(t, testkit.PromptProvenanceV1())
	tests := map[string]struct {
		mutate func(*contract.VerifyPromptUpstreamProvenanceRequestV1)
		want   error
	}{
		"artifact_bytes":  {func(v *contract.VerifyPromptUpstreamProvenanceRequestV1) { v.ArtifactBytes[0].Bytes[0] ^= 0xff }, contract.ErrConflict},
		"license_bytes":   {func(v *contract.VerifyPromptUpstreamProvenanceRequestV1) { v.LicenseBytes[0] ^= 0xff }, contract.ErrConflict},
		"generated_bytes": {func(v *contract.VerifyPromptUpstreamProvenanceRequestV1) { v.GeneratedBytes[0].Bytes[0] ^= 0xff }, contract.ErrConflict},
		"request_digest": {func(v *contract.VerifyPromptUpstreamProvenanceRequestV1) {
			v.RequestDigest = testkit.D("other-request")
		}, contract.ErrConflict},
		"expired": {func(v *contract.VerifyPromptUpstreamProvenanceRequestV1) {
			v.CheckedUnixNano = v.Provenance.ExpiresUnixNano
		}, contract.ErrExpired},
		"before_created": {func(v *contract.VerifyPromptUpstreamProvenanceRequestV1) {
			v.CheckedUnixNano = v.Provenance.CreatedUnixNano - 1
		}, contract.ErrExpired},
		"limit":            {func(v *contract.VerifyPromptUpstreamProvenanceRequestV1) { v.MaxInputBytes = 1 }, contract.ErrLimitExceeded},
		"missing_artifact": {func(v *contract.VerifyPromptUpstreamProvenanceRequestV1) { v.ArtifactBytes = nil }, contract.ErrInvalid},
	}
	for name, item := range tests {
		t.Run(name, func(t *testing.T) {
			request := clonePromptProvenanceRequestV1(base)
			item.mutate(&request)
			report, err := kernel.VerifyPromptUpstreamProvenanceV1(context.Background(), request)
			if !errors.Is(err, item.want) {
				t.Fatalf("error = %v, want %v", err, item.want)
			}
			if !zeroPromptProvenanceReportV1(report) {
				t.Fatal("failure returned a partial report")
			}
		})
	}
}

func TestVerifyPromptUpstreamProvenanceMidCancelReturnsZeroV1(t *testing.T) {
	fixture := testkit.PromptProvenanceV1()
	large := make([]byte, 512*1024)
	for index := range large {
		large[index] = byte(index % 251)
	}
	fixture.Artifacts[0].Bytes = large
	fixture.Provenance.Artifacts[0].ByteLength = uint64(len(large))
	fixture.Provenance.Artifacts[0].ContentDigest = contract.DigestBytes(large)
	fixture.Provenance.Artifacts[0].ExtractedRanges = []contract.PromptUpstreamRangeV1{{Start: 0, End: uint64(len(large)), Digest: contract.DigestBytes(large)}}
	sealedProvenance, err := contract.SealPromptUpstreamProvenanceV1(fixture.Provenance)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Provenance = sealedProvenance
	request := sealPromptProvenanceRequestV1(t, fixture)
	ctx := &cancelAfterChecksContextV1{remaining: 5}
	report, err := kernel.VerifyPromptUpstreamProvenanceV1(ctx, request)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-hash cancellation error = %v", err)
	}
	if !zeroPromptProvenanceReportV1(report) {
		t.Fatal("cancellation returned a partial report")
	}
}

func TestVerifyPromptUpstreamProvenanceConcurrentDeterministicV1(t *testing.T) {
	request := sealPromptProvenanceRequestV1(t, testkit.PromptProvenanceV1())
	const workers = 64
	digests := make(chan contract.Digest, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			report, err := kernel.VerifyPromptUpstreamProvenanceV1(context.Background(), request)
			if err != nil {
				errs <- err
				return
			}
			digests <- report.ReportDigest
		}()
	}
	wait.Wait()
	close(errs)
	close(digests)
	for err := range errs {
		t.Fatal(err)
	}
	var expected contract.Digest
	for digest := range digests {
		if expected == "" {
			expected = digest
		}
		if digest != expected {
			t.Fatal("concurrent verification was not deterministic")
		}
	}
}

func sealPromptProvenanceRequestV1(t *testing.T, fixture testkit.PromptProvenanceFixtureV1) contract.VerifyPromptUpstreamProvenanceRequestV1 {
	t.Helper()
	request, err := kernel.SealVerifyPromptUpstreamProvenanceRequestV1(context.Background(), contract.VerifyPromptUpstreamProvenanceRequestV1{
		Provenance: fixture.Provenance, ArtifactBytes: fixture.Artifacts, LicenseBytes: fixture.License, GeneratedBytes: fixture.Generated,
		CheckedUnixNano: testkit.Now, MaxInputBytes: contract.MaxPromptUpstreamInputBytesV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func clonePromptProvenanceRequestV1(v contract.VerifyPromptUpstreamProvenanceRequestV1) contract.VerifyPromptUpstreamProvenanceRequestV1 {
	copy := v
	copy.ArtifactBytes = append([]contract.PromptUpstreamArtifactBytesV1(nil), v.ArtifactBytes...)
	for index := range copy.ArtifactBytes {
		copy.ArtifactBytes[index].Bytes = append([]byte(nil), v.ArtifactBytes[index].Bytes...)
	}
	copy.LicenseBytes = append([]byte(nil), v.LicenseBytes...)
	copy.GeneratedBytes = append([]contract.PromptGeneratedContentBytesV1(nil), v.GeneratedBytes...)
	for index := range copy.GeneratedBytes {
		copy.GeneratedBytes[index].Bytes = append([]byte(nil), v.GeneratedBytes[index].Bytes...)
	}
	return copy
}

type cancelAfterChecksContextV1 struct {
	remaining int64
}

func (v *cancelAfterChecksContextV1) Deadline() (time.Time, bool) { return time.Time{}, false }
func (v *cancelAfterChecksContextV1) Done() <-chan struct{}       { return nil }
func (v *cancelAfterChecksContextV1) Value(any) any               { return nil }
func (v *cancelAfterChecksContextV1) Err() error {
	if atomic.AddInt64(&v.remaining, -1) <= 0 {
		return context.Canceled
	}
	return nil
}

func zeroPromptProvenanceReportV1(v contract.PromptUpstreamVerificationReportV1) bool {
	return v.ProvenanceRef == (contract.PromptUpstreamProvenanceRefV1{}) && v.SourceSetDigest == "" && v.GeneratedSetDigest == "" && v.ClosureDigest == "" && len(v.VerifiedArtifactIDs) == 0 && len(v.VerifiedContentRefs) == 0 && v.CheckedUnixNano == 0 && v.ExpiresUnixNano == 0 && v.ReportDigest == ""
}
