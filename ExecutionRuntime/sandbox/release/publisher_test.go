package release

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var testNowV1 = time.Date(2026, 7, 18, 0, 30, 0, 0, time.UTC)

type readinessStubV1 struct {
	mu     sync.Mutex
	values []SandboxProductionReadinessProjectionV1
	err    error
	calls  int
}

func (s *readinessStubV1) InspectSandboxProductionReadinessV1(_ context.Context, _ string, _ core.Revision) (SandboxProductionReadinessProjectionV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return SandboxProductionReadinessProjectionV1{}, s.err
	}
	if len(s.values) == 0 {
		return SandboxProductionReadinessProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "readiness absent")
	}
	index := s.calls - 1
	if index >= len(s.values) {
		index = len(s.values) - 1
	}
	return s.values[index], nil
}

type catalogStoreV1 struct {
	mu            sync.Mutex
	values        map[string]assemblercontract.ComponentReleaseV1
	commits       int
	ensureCalls   int
	lostReplyOnce bool
}

func newCatalogStoreV1() *catalogStoreV1 {
	return &catalogStoreV1{values: map[string]assemblercontract.ComponentReleaseV1{}}
}

func (s *catalogStoreV1) EnsureExactComponentReleaseV1(_ context.Context, candidate assemblercontract.ComponentReleaseV1) (assemblercontract.ComponentReleaseV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCalls++
	if err := candidate.Validate(); err != nil {
		return assemblercontract.ComponentReleaseV1{}, err
	}
	key := candidate.ReleaseID + "/" + strconv.FormatUint(uint64(candidate.Revision), 10)
	if existing, ok := s.values[key]; ok {
		if existing.ReleaseDigest != candidate.ReleaseDigest {
			return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "release content conflict")
		}
		return assemblercontract.CloneComponentReleaseV1(existing), nil
	}
	s.values[key] = assemblercontract.CloneComponentReleaseV1(candidate)
	s.commits++
	if s.lostReplyOnce {
		s.lostReplyOnce = false
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "lost release create reply")
	}
	return assemblercontract.CloneComponentReleaseV1(candidate), nil
}

func (s *catalogStoreV1) InspectExactComponentReleaseV1(_ context.Context, ref assemblercontract.ComponentReleaseRefV1) (assemblercontract.ComponentReleaseV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[ref.ReleaseID+"/"+strconv.FormatUint(uint64(ref.Revision), 10)]
	if !ok {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "release absent")
	}
	if value.RefV1() != ref {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "release ref drift")
	}
	return assemblercontract.CloneComponentReleaseV1(value), nil
}

func TestMissingReadinessPublishesStandaloneAssemblyCandidate(t *testing.T) {
	reader := &readinessStubV1{}
	store := newCatalogStoreV1()
	publisher := mustPublisherV1(t, reader, store, func() time.Time { return testNowV1 })
	result, err := publisher.Publish(context.Background(), publicationRequestV1(1))
	if err != nil {
		t.Fatal(err)
	}
	if result.ProductionReady || result.Readiness != nil || result.Release.SupportMode != assemblercontract.SupportStandaloneV1 {
		t.Fatalf("missing production proof was promoted: %#v", result)
	}
	if result.Release.ComponentManifest.Conformance == "fully_controlled" || result.Release.ComponentManifest.ResidualClass == "none" {
		t.Fatal("standalone candidate self-certified production conformance")
	}
	if len(result.Release.ModuleDescriptors) != 1 || len(result.Release.CapabilityDescriptors) != 2 || len(result.Release.PortSpecs) != 2 || len(result.Release.FactoryDescriptors) != 2 {
		t.Fatal("assembly candidate omitted the host adapter construction closure")
	}
	if len(result.Release.SlotContributions) != 1 || result.Release.SlotContributions[0].SlotRef != "sandbox.execution" || result.Release.SlotContributions[0].Kind != assemblycontract.SlotContributionOwnerV1 || result.Release.SlotContributions[0].CapabilityRef != ExecutionCapabilityV1 || len(result.Release.ProviderBindingCandidates) != 0 || len(result.Release.PhaseContributions) != 0 {
		t.Fatalf("Sandbox execution contribution is incomplete or fabricated a provider/phase: %+v", result.Release)
	}
	report, err := EvaluateConformanceCandidateV1(result, testNowV1)
	if err != nil || !report.CatalogEligible || report.ProductionClaimEligible {
		t.Fatalf("standalone conformance truth drifted: %#v %v", report, err)
	}
}

func TestExactReadinessPublishesCertifiedProductionRelease(t *testing.T) {
	readiness := readinessProjectionV1(t, 2, testNowV1)
	reader := &readinessStubV1{values: []SandboxProductionReadinessProjectionV1{readiness, readiness}}
	store := newCatalogStoreV1()
	publisher := mustPublisherV1(t, reader, store, func() time.Time { return testNowV1 })
	result, err := publisher.Publish(context.Background(), publicationRequestV1(2))
	if err != nil {
		t.Fatal(err)
	}
	if !result.ProductionReady || result.Release.SupportMode != assemblercontract.SupportProductionV1 || result.Release.ComponentManifest.Conformance != "fully_controlled" || result.Release.ComponentManifest.ResidualClass != "none" {
		t.Fatalf("exact production readiness was not represented: %#v", result.Release)
	}
	if err := result.Release.Validate(); err != nil {
		t.Fatal(err)
	}
	report, err := EvaluateConformanceCandidateV1(result, testNowV1)
	if err != nil || !report.ProductionClaimEligible || !report.ConstructionClosureValid {
		t.Fatalf("production conformance candidate is incomplete: %#v %v", report, err)
	}
}

func TestReadinessDriftAndTTLFailBeforeCatalogWrite(t *testing.T) {
	first := readinessProjectionV1(t, 3, testNowV1)
	second := first
	second.ProviderInspectRef = refV1("provider-inspect-drift")
	second = mustSealReadinessV1(t, second)
	store := newCatalogStoreV1()
	publisher := mustPublisherV1(t, &readinessStubV1{values: []SandboxProductionReadinessProjectionV1{first, second}}, store, func() time.Time { return testNowV1 })
	if _, err := publisher.Publish(context.Background(), publicationRequestV1(3)); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("readiness S1/S2 drift was accepted: %v", err)
	}
	if store.commits != 0 {
		t.Fatal("drifted readiness reached the release catalog")
	}

	clock := &sequenceClockV1{values: []time.Time{testNowV1, testNowV1, testNowV1.Add(31 * time.Minute)}}
	expiring := readinessProjectionV1(t, 4, testNowV1)
	expiring.ExpiresUnixNano = testNowV1.Add(30 * time.Minute).UnixNano()
	expiring = mustSealReadinessV1(t, expiring)
	store = newCatalogStoreV1()
	publisher = mustPublisherV1(t, &readinessStubV1{values: []SandboxProductionReadinessProjectionV1{expiring, expiring}}, store, clock.Now)
	if _, err := publisher.Publish(context.Background(), publicationRequestV1(4)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("TTL crossing was accepted: %v", err)
	}
	if store.commits != 0 {
		t.Fatal("expired readiness reached the release catalog")
	}
}

func TestProductionArtifactManifestAndCertificationAreExact(t *testing.T) {
	base := readinessProjectionV1(t, 8, testNowV1)
	for _, test := range []struct {
		name   string
		mutate func(*SandboxProductionReadinessProjectionV1)
	}{
		{name: "artifact", mutate: func(value *SandboxProductionReadinessProjectionV1) {
			value.ArtifactDigest = core.DigestBytes([]byte("other-artifact"))
		}},
		{name: "manifest", mutate: func(value *SandboxProductionReadinessProjectionV1) {
			value.ManifestDigest = core.DigestBytes([]byte("other-manifest"))
		}},
		{name: "certification", mutate: func(value *SandboxProductionReadinessProjectionV1) {
			value.CertificationFactRef.Digest = core.DigestBytes([]byte("other-certification"))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			projection := base
			test.mutate(&projection)
			projection = mustSealReadinessV1(t, projection)
			store := newCatalogStoreV1()
			publisher := mustPublisherV1(t, &readinessStubV1{values: []SandboxProductionReadinessProjectionV1{projection, projection}}, store, func() time.Time { return testNowV1 })
			if _, err := publisher.Publish(context.Background(), publicationRequestV1(8)); err == nil {
				t.Fatal("drifted independent production proof was accepted")
			}
			if store.commits != 0 {
				t.Fatal("drifted production proof reached the catalog")
			}
		})
	}
}

func TestUnavailableAndClockRollbackNeverDowngradeToStandalone(t *testing.T) {
	store := newCatalogStoreV1()
	unavailable := core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "readiness backend unavailable")
	publisher := mustPublisherV1(t, &readinessStubV1{err: unavailable}, store, func() time.Time { return testNowV1 })
	if _, err := publisher.Publish(context.Background(), publicationRequestV1(10)); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("unknown readiness was downgraded to standalone: %v", err)
	}
	if store.commits != 0 {
		t.Fatal("unknown readiness wrote a release")
	}

	projection := readinessProjectionV1(t, 11, testNowV1)
	clock := &sequenceClockV1{values: []time.Time{testNowV1, testNowV1.Add(-time.Nanosecond)}}
	store = newCatalogStoreV1()
	publisher = mustPublisherV1(t, &readinessStubV1{values: []SandboxProductionReadinessProjectionV1{projection, projection}}, store, clock.Now)
	if _, err := publisher.Publish(context.Background(), publicationRequestV1(11)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback was accepted: %v", err)
	}
	if store.commits != 0 {
		t.Fatal("clock rollback wrote a release")
	}
}

func TestLostCreateReplyRecoversByExactInspect(t *testing.T) {
	store := newCatalogStoreV1()
	store.lostReplyOnce = true
	publisher := mustPublisherV1(t, &readinessStubV1{}, store, func() time.Time { return testNowV1 })
	result, err := publisher.Publish(context.Background(), publicationRequestV1(5))
	if err != nil {
		t.Fatal(err)
	}
	if store.commits != 1 || result.Release.ReleaseDigest == "" {
		t.Fatalf("lost create reply was re-created or not recovered: commits=%d", store.commits)
	}
}

func TestSixtyFourConcurrentPublishersLinearizeOneRelease(t *testing.T) {
	store := newCatalogStoreV1()
	var failures atomic.Int64
	start := make(chan struct{})
	var group sync.WaitGroup
	for range 64 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			publisher := mustPublisherV1(t, &readinessStubV1{}, store, func() time.Time { return testNowV1 })
			if _, err := publisher.Publish(context.Background(), publicationRequestV1(6)); err != nil {
				failures.Add(1)
			}
		}()
	}
	close(start)
	group.Wait()
	if failures.Load() != 0 || store.commits != 1 {
		t.Fatalf("concurrent exact publication did not linearize once: failures=%d commits=%d", failures.Load(), store.commits)
	}
}

func TestTypedNilAndProofAliasFailClosed(t *testing.T) {
	var typedNil *readinessStubV1
	if _, err := NewPublisherV1(typedNil, newCatalogStoreV1(), func() time.Time { return testNowV1 }); err == nil {
		t.Fatal("typed-nil readiness reader was accepted")
	}
	var typedCatalog *catalogStoreV1
	if _, err := NewPublisherV1(&readinessStubV1{}, typedCatalog, func() time.Time { return testNowV1 }); err == nil {
		t.Fatal("typed-nil release catalog was accepted")
	}
	projection := readinessProjectionV1(t, 7, testNowV1)
	projection.CleanupOwnerRef = projection.ProviderInspectRef
	if _, err := SealSandboxProductionReadinessV1(projection); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("one proof aliased two production roles: %v", err)
	}
}

func TestSameReleaseRevisionRejectsChangedArtifactAndReturnedValueCannotMutateStore(t *testing.T) {
	store := newCatalogStoreV1()
	publisher := mustPublisherV1(t, &readinessStubV1{}, store, func() time.Time { return testNowV1 })
	first, err := publisher.Publish(context.Background(), publicationRequestV1(9))
	if err != nil {
		t.Fatal(err)
	}
	original := first.Release.RefV1()
	first.Release.ModuleDescriptors[0].ModuleID = "caller-mutated"
	inspected, err := store.InspectExactComponentReleaseV1(context.Background(), original)
	if err != nil || inspected.ModuleDescriptors[0].ModuleID == "caller-mutated" {
		t.Fatalf("returned release aliased catalog state: %v", err)
	}
	changed := publicationRequestV1(9)
	changed.ArtifactDigest = core.DigestBytes([]byte("changed-artifact"))
	if _, err := publisher.Publish(context.Background(), changed); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same release revision accepted changed artifact: %v", err)
	}
	if store.commits != 1 {
		t.Fatalf("changed content committed: %d", store.commits)
	}
}

type sequenceClockV1 struct {
	mu     sync.Mutex
	values []time.Time
	index  int
}

func (c *sequenceClockV1) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.index >= len(c.values) {
		return c.values[len(c.values)-1]
	}
	value := c.values[c.index]
	c.index++
	return value
}

func publicationRequestV1(revision core.Revision) PublicationRequestV1 {
	return PublicationRequestV1{
		ReleaseID: "praxis.sandbox/release", Revision: revision,
		SourceRef: refV1("sandbox-release-source"), PublisherRef: refV1("sandbox-release-publisher"), TrustRef: refV1("sandbox-release-trust"),
		CertificationID: "praxis.sandbox/release-certification", ArtifactDigest: core.DigestBytes([]byte("sandbox-artifact-v1")),
		CreatedUnixNano: testNowV1.Add(-time.Minute).UnixNano(), ExpiresUnixNano: testNowV1.Add(time.Hour).UnixNano(),
	}
}

func readinessProjectionV1(t *testing.T, revision core.Revision, now time.Time) SandboxProductionReadinessProjectionV1 {
	t.Helper()
	ids := []string{"composition", "fact-store", "current-store", "lease-reader", "policy-reader", "sandbox-reader", "enforcement", "evidence", "settlement", "provider-transport", "provider-inspect", "journal", "cleanup", "deployment", "certification"}
	refs := make([]assemblycontract.ObjectRefV1, len(ids))
	for index, id := range ids {
		refs[index] = refV1("sandbox-" + id)
	}
	request := publicationRequestV1(revision)
	standalone, err := buildReleasePayloadV1(request, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	manifest := standalone.ComponentManifest
	manifest.Conformance = "fully_controlled"
	manifest.ResidualClass = "none"
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	projection := SandboxProductionReadinessProjectionV1{
		ComponentID: ComponentIDV1, ReleaseID: "praxis.sandbox/release", Revision: revision,
		ArtifactDigest: request.ArtifactDigest, ManifestDigest: manifestDigest,
		CompositionRef: refs[0], DurableFactStoreRef: refs[1], DurableCurrentStoreRef: refs[2],
		LeaseCurrentReaderRef: refs[3], PolicyCurrentReaderRef: refs[4], SandboxCurrentReaderRef: refs[5],
		EnforcementGatewayRef: refs[6], EvidenceGovernanceRef: refs[7], SettlementCurrentRef: refs[8],
		ProviderTransportRef: refs[9], ProviderInspectRef: refs[10], DataPlaneJournalRef: refs[11],
		CleanupOwnerRef: refs[12], DeploymentAttestationRef: refs[13], CertificationFactRef: refs[14],
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Minute).UnixNano(),
	}
	candidate, err := buildReleasePayloadV1(request, &projection, false)
	if err != nil {
		t.Fatal(err)
	}
	projection.CertificationFactRef = assemblycontract.ObjectRefV1{ID: request.CertificationID, Revision: request.Revision, Digest: candidate.CertificationRef.Digest}
	return mustSealReadinessV1(t, projection)
}

func refV1(id string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
}

func mustSealReadinessV1(t *testing.T, value SandboxProductionReadinessProjectionV1) SandboxProductionReadinessProjectionV1 {
	t.Helper()
	sealed, err := SealSandboxProductionReadinessV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func mustPublisherV1(t *testing.T, readiness ProductionReadinessReaderV1, catalog ComponentReleaseCatalogPortV1, now func() time.Time) *PublisherV1 {
	t.Helper()
	publisher, err := NewPublisherV1(readiness, catalog, now)
	if err != nil {
		t.Fatal(err)
	}
	return publisher
}
