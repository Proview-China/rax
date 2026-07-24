package releasecandidate_test

import (
	"context"
	"sync"
	"testing"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/releasecandidate"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var testNow = time.Date(2026, 7, 18, 1, 20, 0, 0, time.UTC)

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

func planArtifacts() []assemblercontract.PlanArtifactV1 {
	return []assemblercontract.PlanArtifactV1{
		{Role: assemblercontract.ArtifactHarnessBootstrapV1, Ref: ref("plan/harness-bootstrap")},
		{Role: assemblercontract.ArtifactProfileV1, Ref: ref("plan/profile")},
		{Role: assemblercontract.ArtifactRuntimePolicyV1, Ref: ref("plan/runtime-policy")},
		{Role: assemblercontract.ArtifactHarnessStackV1, Ref: ref("plan/harness-stack")},
		{Role: assemblercontract.ArtifactSemanticRouteV1, Ref: ref("plan/semantic-route")},
		{Role: assemblercontract.ArtifactContextPlanV1, Ref: ref("plan/context")},
		{Role: assemblercontract.ArtifactToolSurfaceV1, Ref: ref("plan/tool-surface")},
		{Role: assemblercontract.ArtifactCapabilityGrantV1, Ref: ref("plan/capability-grant")},
		{Role: assemblercontract.ArtifactExpectedInjectionV1, Ref: ref("plan/expected-injection")},
	}
}

func validRequest() releasecandidate.RequestV1 {
	return releasecandidate.RequestV1{
		ReleaseID: "release/harness/assembly-candidate", Revision: 1,
		ArtifactDigest: digest("harness-artifact-v1"), SourceRef: ref("source/harness/v1"),
		PublisherRef: ref("publisher/harness-owner"), TrustRef: ref("trust/harness-owner"),
		EvidenceRefs:  []assemblycontract.ObjectRefV1{ref("evidence/harness-assembly-current"), ref("evidence/harness-route-current"), ref("evidence/harness-commit-gate")},
		PlanArtifacts: planArtifacts(), TTL: time.Hour,
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

func TestBuildV1ClosesReferenceOnlyReleaseAndDescriptorFactory(t *testing.T) {
	candidate := build(t)
	release := candidate.Release
	if release.SupportMode != assemblercontract.SupportReferenceOnlyV1 || candidate.Readiness.ProductionEligible || candidate.Readiness.State != "assembly_candidate" {
		t.Fatalf("candidate crossed production boundary: mode=%q readiness=%+v", release.SupportMode, candidate.Readiness)
	}
	conformance := candidate.Conformance
	if !conformance.ReferenceOnly || !conformance.OwnerLocalAssemblyCurrent || !conformance.OwnerLocalRouteCurrent || !conformance.OwnerLocalCommitGate || conformance.PersistentStores || conformance.ProductionRoute || conformance.ActualPointGuard || conformance.ProductionContinuation || conformance.ExecutableFactory || conformance.ProductionSLA {
		t.Fatalf("owner-local conformance drifted: %+v", conformance)
	}
	if release.ComponentManifest.ComponentID != releasecandidate.ComponentIDV1 || release.ComponentManifest.Kind != releasecandidate.ComponentKindV1 || release.ComponentManifest.Conformance != runtimeports.ConformanceRestrictedControlled || release.ComponentManifest.ResidualClass != runtimeports.ResidualInspectable {
		t.Fatalf("manifest boundary drifted: %+v", release.ComponentManifest)
	}
	if len(release.ModuleDescriptors) != 1 || len(release.CapabilityDescriptors) != 1 || len(release.PortSpecs) != 1 || len(release.FactoryDescriptors) != 1 || len(release.ComponentManifest.Owners) != 3 || len(release.RequiredPlanArtifacts) != 9 {
		t.Fatalf("descriptor, owner, or plan closure incomplete")
	}
	factory := release.FactoryDescriptors[0]
	if factory.FactoryID != releasecandidate.FactoryIDV1 || factory.ModuleRef != releasecandidate.ModuleIDV1 || factory.OutputCapability != releasecandidate.CapabilityV1 || factory.ArtifactDigest != release.ArtifactDigest {
		t.Fatalf("factory descriptor drifted: %+v", factory)
	}
	certified, err := assemblercontract.ComponentReleaseCertificationDigestV1(release)
	if err != nil || certified != release.CertificationRef.Digest {
		t.Fatalf("candidate certification is not exact: %q %v", certified, err)
	}
}

func TestProductionPromotionFailsClosed(t *testing.T) {
	release := build(t).Release
	release.SupportMode = assemblercontract.SupportProductionV1
	certified, err := assemblercontract.ComponentReleaseCertificationDigestV1(release)
	if err != nil {
		t.Fatal(err)
	}
	release.CertificationRef.Digest = certified
	if _, err = assemblercontract.SealComponentReleaseV1(release); !core.HasReason(err, core.ReasonBindingNotCertified) {
		t.Fatalf("reference Harness release promoted: %v", err)
	}
}

type releaseStore struct {
	mu            sync.Mutex
	release       assemblercontract.ComponentReleaseV1
	ensureCalls   int
	inspectCalls  int
	loseNextReply bool
	driftReturn   bool
	inspectLive   bool
}

func (s *releaseStore) EnsureExactComponentReleaseV1(_ context.Context, value assemblercontract.ComponentReleaseV1) (assemblercontract.ComponentReleaseV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCalls++
	if s.release.ReleaseDigest != "" && s.release.ReleaseDigest != value.ReleaseDigest {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "release changed")
	}
	s.release = assemblercontract.CloneComponentReleaseV1(value)
	if s.loseNextReply {
		s.loseNextReply = false
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "publish reply lost")
	}
	if s.driftReturn {
		drifted := assemblercontract.CloneComponentReleaseV1(value)
		drifted.ReleaseID = "release/harness/drifted"
		drifted, _ = assemblercontract.SealComponentReleaseV1(drifted)
		return drifted, nil
	}
	return assemblercontract.CloneComponentReleaseV1(value), nil
}

func (s *releaseStore) InspectExactComponentReleaseV1(ctx context.Context, expected assemblercontract.ComponentReleaseRefV1) (assemblercontract.ComponentReleaseV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inspectCalls++
	s.inspectLive = ctx.Err() == nil
	if s.release.RefV1() != expected {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "release exact ref drifted")
	}
	return assemblercontract.CloneComponentReleaseV1(s.release), nil
}

func TestPublisherLostReplyUsesExactInspectWithoutMutationRetry(t *testing.T) {
	builder, err := releasecandidate.NewBuilderV1(&fixedClock{now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	store := &releaseStore{loseNextReply: true}
	publisher, err := releasecandidate.NewPublisherV1(builder, store, store)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := publisher.PublishV1(ctx, validRequest()); err != nil {
		t.Fatal(err)
	}
	if store.ensureCalls != 1 || store.inspectCalls != 1 || !store.inspectLive {
		t.Fatalf("lost reply recovery drifted: ensure=%d inspect=%d live=%v", store.ensureCalls, store.inspectCalls, store.inspectLive)
	}
}

func TestTypedNilPublishedDriftAndPostPublishTimeFailClosed(t *testing.T) {
	var nilClock *fixedClock
	if _, err := releasecandidate.NewBuilderV1(nilClock); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("typed nil clock accepted: %v", err)
	}
	builder, err := releasecandidate.NewBuilderV1(&fixedClock{now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	var nilStore *releaseStore
	if _, err := releasecandidate.NewPublisherV1(builder, nilStore, nilStore); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil publisher accepted: %v", err)
	}
	store := &releaseStore{driftReturn: true}
	publisher, err := releasecandidate.NewPublisherV1(builder, store, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = publisher.PublishV1(context.Background(), validRequest()); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("published drift accepted: %v", err)
	}
	for _, tt := range []struct {
		name   string
		final  time.Time
		reason core.ReasonCode
	}{{"ttl", testNow.Add(time.Second), core.ReasonCapabilityExpired}, {"rollback", testNow.Add(-time.Nanosecond), core.ReasonClockRegression}} {
		t.Run(tt.name, func(t *testing.T) {
			builder, err := releasecandidate.NewBuilderV1(&sequenceClock{values: []time.Time{testNow, testNow, tt.final}})
			if err != nil {
				t.Fatal(err)
			}
			store := &releaseStore{}
			publisher, _ := releasecandidate.NewPublisherV1(builder, store, store)
			request := validRequest()
			request.TTL = time.Second
			if _, err := publisher.PublishV1(context.Background(), request); !core.HasReason(err, tt.reason) {
				t.Fatalf("want %s, got %v", tt.reason, err)
			}
		})
	}
}

func TestRequestPlanTTLConformanceAndProofDriftFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*releasecandidate.RequestV1)
		reason core.ReasonCode
	}{
		{"missing-evidence", func(r *releasecandidate.RequestV1) { r.EvidenceRefs = nil }, core.ReasonEvidenceUnavailable},
		{"duplicate-evidence", func(r *releasecandidate.RequestV1) { r.EvidenceRefs = append(r.EvidenceRefs, r.EvidenceRefs[0]) }, core.ReasonEvidenceConflict},
		{"artifact", func(r *releasecandidate.RequestV1) { r.ArtifactDigest = "" }, core.ReasonInvalidDigest},
		{"missing-plan", func(r *releasecandidate.RequestV1) { r.PlanArtifacts = r.PlanArtifacts[:8] }, core.ReasonPlanInvalid},
		{"duplicate-plan", func(r *releasecandidate.RequestV1) { r.PlanArtifacts[8].Role = r.PlanArtifacts[0].Role }, core.ReasonDuplicateCanonicalKey},
		{"ttl-short", func(r *releasecandidate.RequestV1) { r.TTL = time.Nanosecond }, core.ReasonCapabilityExpired},
		{"ttl-drift", func(r *releasecandidate.RequestV1) { r.TTL = time.Second + time.Nanosecond }, core.ReasonCapabilityExpired},
		{"ttl-long", func(r *releasecandidate.RequestV1) { r.TTL = 25 * time.Hour }, core.ReasonCapabilityExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := validRequest()
			tt.mutate(&request)
			builder, _ := releasecandidate.NewBuilderV1(&fixedClock{now: testNow})
			if _, err := builder.BuildV1(request); !core.HasReason(err, tt.reason) {
				t.Fatalf("want %s, got %v", tt.reason, err)
			}
		})
	}
	candidate := build(t)
	candidate.Readiness.MissingProductionProofs[0] = "harness.fake-proof"
	if err := candidate.ValidateCurrentV1(testNow.Add(time.Minute)); !core.HasReason(err, core.ReasonBindingNotCertified) {
		t.Fatalf("proof drift accepted: %v", err)
	}
	candidate = build(t)
	candidate.Conformance.ExecutableFactory = true
	if err := candidate.ValidateCurrentV1(testNow.Add(time.Minute)); !core.HasReason(err, core.ReasonBindingNotCertified) {
		t.Fatalf("factory conformance drift accepted: %v", err)
	}
	candidate = build(t)
	candidate.Release.ComponentManifest.ArtifactDigest = digest("artifact-drift")
	if err := candidate.ValidateCurrentV1(testNow.Add(time.Minute)); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("artifact drift accepted: %v", err)
	}
}

func TestPublisher64ConcurrentDeterministic(t *testing.T) {
	builder, _ := releasecandidate.NewBuilderV1(&fixedClock{now: testNow})
	store := &releaseStore{}
	publisher, _ := releasecandidate.NewPublisherV1(builder, store, store)
	const workers = 64
	values := make(chan core.Digest, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for index := 0; index < workers; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			candidate, err := publisher.PublishV1(context.Background(), validRequest())
			if err != nil {
				errs <- err
				return
			}
			values <- candidate.Release.ReleaseDigest
		}()
	}
	wg.Wait()
	close(values)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var expected core.Digest
	for value := range values {
		if expected == "" {
			expected = value
		}
		if value != expected {
			t.Fatalf("concurrent release drift: want=%q got=%q", expected, value)
		}
	}
	if store.ensureCalls != workers {
		t.Fatalf("unexpected ensure calls: %d", store.ensureCalls)
	}
}
