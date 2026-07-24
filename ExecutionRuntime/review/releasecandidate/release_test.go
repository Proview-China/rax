package releasecandidate_test

import (
	"sync"
	"testing"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/releasecandidate"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var testNow = time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)

type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time { return c.now }

type sequenceClock struct {
	mu     sync.Mutex
	values []time.Time
	index  int
}

func (c *sequenceClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.index >= len(c.values) {
		return c.values[len(c.values)-1]
	}
	value := c.values[c.index]
	c.index++
	return value
}

func digest(value string) core.Digest { return core.DigestBytes([]byte(value)) }
func ref(value string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: value, Revision: 1, Digest: digest(value)}
}
func validRequest() releasecandidate.RequestV1 {
	return releasecandidate.RequestV1{
		ReleaseID: "release/review/assembly-candidate", Revision: 1,
		ArtifactDigest: digest("review-artifact-v1"), SourceRef: ref("source/review/v1"),
		PublisherRef: ref("publisher/review-owner"), TrustRef: ref("trust/review-owner"),
		EvidenceRefs: []assemblycontract.ObjectRefV1{ref("evidence/review-unit"), ref("evidence/review-sqlite")},
		TTL:          time.Hour,
	}
}
func build(t *testing.T) releasecandidate.CandidateV1 {
	t.Helper()
	builder, err := releasecandidate.NewBuilderV1(&fixedClock{now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := builder.BuildV1(validRequest())
	if err != nil {
		t.Fatal(err)
	}
	return candidate
}

func TestBuildV1ProducesExactReferenceOnlyAssemblyCandidate(t *testing.T) {
	candidate := build(t)
	release := candidate.Release
	if release.SupportMode != assemblercontract.SupportReferenceOnlyV1 || candidate.Readiness.ProductionEligible || candidate.Readiness.State != "assembly_candidate" {
		t.Fatalf("candidate crossed production boundary: release=%q readiness=%+v", release.SupportMode, candidate.Readiness)
	}
	if release.ComponentManifest.ComponentID != releasecandidate.ComponentIDV1 || release.ComponentManifest.Kind != releasecandidate.ComponentKindV1 || release.ComponentManifest.Conformance != runtimeports.ConformanceRestrictedControlled || release.ComponentManifest.ResidualClass != runtimeports.ResidualInspectable {
		t.Fatalf("manifest boundary drifted: %+v", release.ComponentManifest)
	}
	if len(release.ModuleDescriptors) != 1 || len(release.CapabilityDescriptors) != 1 || len(release.PortSpecs) != 1 || len(release.FactoryDescriptors) != 1 {
		t.Fatalf("descriptor closure is incomplete: modules=%d capabilities=%d ports=%d factories=%d", len(release.ModuleDescriptors), len(release.CapabilityDescriptors), len(release.PortSpecs), len(release.FactoryDescriptors))
	}
	if len(release.ComponentManifest.Owners) != 3 || len(release.ModuleDescriptors[0].Owners) != 3 {
		t.Fatalf("owner closure is incomplete")
	}
	factory := release.FactoryDescriptors[0]
	if factory.FactoryID != releasecandidate.FactoryIDV1 || factory.ModuleRef != releasecandidate.ModuleIDV1 || factory.OutputCapability != releasecandidate.CapabilityV1 || factory.ArtifactDigest != release.ArtifactDigest {
		t.Fatalf("declarative factory descriptor drifted: %+v", factory)
	}
	certified, err := assemblercontract.ComponentReleaseCertificationDigestV1(release)
	if err != nil || certified != release.CertificationRef.Digest {
		t.Fatalf("candidate certification is not exact: digest=%q err=%v", certified, err)
	}
	if err := candidate.ValidateCurrentV1(testNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
}

func TestProductionPromotionAlwaysFailsClosed(t *testing.T) {
	release := build(t).Release
	release.SupportMode = assemblercontract.SupportProductionV1
	certified, err := assemblercontract.ComponentReleaseCertificationDigestV1(release)
	if err != nil {
		t.Fatal(err)
	}
	release.CertificationRef.Digest = certified
	if _, err = assemblercontract.SealComponentReleaseV1(release); !core.HasReason(err, core.ReasonBindingNotCertified) {
		t.Fatalf("reference-only Review release was promoted to production: %v", err)
	}
}

func TestBuilderRejectsTypedNilAndClockRegression(t *testing.T) {
	var clock *fixedClock
	if _, err := releasecandidate.NewBuilderV1(clock); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("typed nil clock accepted: %v", err)
	}
	builder, err := releasecandidate.NewBuilderV1(&sequenceClock{values: []time.Time{testNow, testNow.Add(-time.Nanosecond)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = builder.BuildV1(validRequest()); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback accepted: %v", err)
	}
}

func TestFaultsTTLDriftAndExpiryFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*releasecandidate.RequestV1)
		reason core.ReasonCode
	}{
		{"missing-evidence", func(r *releasecandidate.RequestV1) { r.EvidenceRefs = nil }, core.ReasonEvidenceUnavailable},
		{"zero-artifact", func(r *releasecandidate.RequestV1) { r.ArtifactDigest = "" }, core.ReasonInvalidDigest},
		{"ttl-too-short", func(r *releasecandidate.RequestV1) { r.TTL = time.Nanosecond }, core.ReasonCapabilityExpired},
		{"ttl-subsecond-drift", func(r *releasecandidate.RequestV1) { r.TTL = time.Second + time.Nanosecond }, core.ReasonCapabilityExpired},
		{"ttl-too-long", func(r *releasecandidate.RequestV1) { r.TTL = 25 * time.Hour }, core.ReasonCapabilityExpired},
		{"duplicate-evidence", func(r *releasecandidate.RequestV1) { r.EvidenceRefs = append(r.EvidenceRefs, r.EvidenceRefs[0]) }, core.ReasonEvidenceConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := validRequest()
			tt.mutate(&request)
			builder, err := releasecandidate.NewBuilderV1(&fixedClock{now: testNow})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = builder.BuildV1(request); !core.HasReason(err, tt.reason) {
				t.Fatalf("want %s, got %v", tt.reason, err)
			}
		})
	}
	candidate := build(t)
	if err := candidate.ValidateCurrentV1(testNow.Add(-time.Nanosecond)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("creation drift accepted: %v", err)
	}
	if err := candidate.ValidateCurrentV1(time.Unix(0, candidate.Release.ExpiresUnixNano)); !core.HasReason(err, core.ReasonCapabilityExpired) {
		t.Fatalf("TTL crossing accepted: %v", err)
	}
	candidate.Release.ComponentManifest.ArtifactDigest = digest("drift")
	if err := candidate.ValidateCurrentV1(testNow.Add(time.Minute)); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("artifact drift accepted: %v", err)
	}
}

func TestBuildV1IsDeterministicUnder64ConcurrentCalls(t *testing.T) {
	builder, err := releasecandidate.NewBuilderV1(&fixedClock{now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	const workers = 64
	digests := make(chan core.Digest, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			candidate, err := builder.BuildV1(validRequest())
			if err != nil {
				errs <- err
				return
			}
			digests <- candidate.Release.ReleaseDigest
		}()
	}
	wg.Wait()
	close(errs)
	close(digests)
	for err := range errs {
		t.Fatal(err)
	}
	var expected core.Digest
	for value := range digests {
		if expected == "" {
			expected = value
		}
		if value != expected {
			t.Fatalf("concurrent build drifted: want=%q got=%q", expected, value)
		}
	}
}
