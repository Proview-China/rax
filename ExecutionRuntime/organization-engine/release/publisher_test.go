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
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var fixedNow = time.Date(2026, 7, 18, 2, 0, 0, 0, time.UTC)

type localReader struct {
	mu     sync.Mutex
	values []LocalReadinessProjectionV1
	err    error
	reads  int
}

func (r *localReader) InspectOrganizationLocalReadinessV1(context.Context, string, core.Revision) (LocalReadinessProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	if r.err != nil {
		return LocalReadinessProjectionV1{}, r.err
	}
	if len(r.values) == 0 {
		return LocalReadinessProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "Organization SQLite readiness is absent")
	}
	value := r.values[0]
	if len(r.values) > 1 {
		r.values = r.values[1:]
	}
	return value, nil
}

type productionReader struct {
	mu     sync.Mutex
	values []ProductionReadinessProjectionV1
	err    error
	reads  int
}

func (r *productionReader) InspectOrganizationProductionReadinessV1(context.Context, string, core.Revision) (ProductionReadinessProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	if r.err != nil {
		return ProductionReadinessProjectionV1{}, r.err
	}
	if len(r.values) == 0 {
		return ProductionReadinessProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "Organization production readiness is absent")
	}
	value := r.values[0]
	if len(r.values) > 1 {
		r.values = r.values[1:]
	}
	return value, nil
}

type releaseCatalog struct {
	mu        sync.RWMutex
	values    map[string]assemblercontract.ComponentReleaseV1
	commits   int
	lostReply atomic.Bool
}

func newReleaseCatalog() *releaseCatalog {
	return &releaseCatalog{values: map[string]assemblercontract.ComponentReleaseV1{}}
}

func releaseKey(releaseID string, revision core.Revision) string {
	return releaseID + "\x00" + strconv.FormatUint(uint64(revision), 10)
}

func (s *releaseCatalog) EnsureExactComponentReleaseV1(ctx context.Context, candidate assemblercontract.ComponentReleaseV1) (assemblercontract.ComponentReleaseV1, error) {
	if err := ctx.Err(); err != nil {
		return assemblercontract.ComponentReleaseV1{}, err
	}
	if err := candidate.Validate(); err != nil {
		return assemblercontract.ComponentReleaseV1{}, err
	}
	s.mu.Lock()
	key := releaseKey(candidate.ReleaseID, candidate.Revision)
	existing, exists := s.values[key]
	if exists && existing.ReleaseDigest != candidate.ReleaseDigest {
		s.mu.Unlock()
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Organization release identity contains different content")
	}
	if !exists {
		s.values[key] = assemblercontract.CloneComponentReleaseV1(candidate)
		s.commits++
		existing = s.values[key]
	}
	s.mu.Unlock()
	if s.lostReply.CompareAndSwap(true, false) {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Organization catalog reply was lost")
	}
	return assemblercontract.CloneComponentReleaseV1(existing), nil
}

func (s *releaseCatalog) InspectExactComponentReleaseV1(ctx context.Context, ref assemblercontract.ComponentReleaseRefV1) (assemblercontract.ComponentReleaseV1, error) {
	if err := ctx.Err(); err != nil {
		return assemblercontract.ComponentReleaseV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return assemblercontract.ComponentReleaseV1{}, err
	}
	s.mu.RLock()
	value, ok := s.values[releaseKey(ref.ReleaseID, ref.Revision)]
	s.mu.RUnlock()
	if !ok {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "Organization release is absent")
	}
	if value.RefV1() != ref {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Organization release exact ref drifted")
	}
	return assemblercontract.CloneComponentReleaseV1(value), nil
}

func (s *releaseCatalog) commitCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.commits
}

func objectRef(id string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
}

func publicationRequest(revision core.Revision) PublicationRequestV1 {
	return PublicationRequestV1{
		ReleaseID:       "praxis.organization/release",
		Revision:        revision,
		SourceRef:       objectRef("organization-source"),
		PublisherRef:    objectRef("organization-publisher"),
		TrustRef:        objectRef("organization-trust"),
		CertificationID: "organization-certification",
		ArtifactDigest:  core.DigestBytes([]byte("organization-artifact")),
		CreatedUnixNano: fixedNow.Add(-time.Minute).UnixNano(),
		ExpiresUnixNano: fixedNow.Add(time.Hour).UnixNano(),
	}
}

func localReadiness(t *testing.T, revision core.Revision) LocalReadinessProjectionV1 {
	t.Helper()
	refs := []assemblycontract.ObjectRefV1{objectRef("sqlite-resource"), objectRef("sqlite-schema"), objectRef("sqlite-integrity"), objectRef("sqlite-restart"), objectRef("organization-history-reader"), objectRef("organization-current-reader")}
	value, err := SealLocalReadinessV1(LocalReadinessProjectionV1{
		ComponentID:          ComponentIDV1,
		ReleaseID:            publicationRequest(revision).ReleaseID,
		Revision:             revision,
		ArtifactDigest:       publicationRequest(revision).ArtifactDigest,
		BackendKind:          SQLiteBackendKindV1,
		SQLiteResourceRef:    refs[0],
		SchemaEvidenceRef:    refs[1],
		IntegrityEvidenceRef: refs[2],
		RestartEvidenceRef:   refs[3],
		HistoricalReaderRef:  refs[4],
		CurrentReaderRef:     refs[5],
		CheckedUnixNano:      fixedNow.Add(-time.Minute).UnixNano(),
		ExpiresUnixNano:      fixedNow.Add(30 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func productionReadiness(t *testing.T, local LocalReadinessProjectionV1) ProductionReadinessProjectionV1 {
	t.Helper()
	request := publicationRequest(local.Revision)
	value := ProductionReadinessProjectionV1{
		ComponentID:                 ComponentIDV1,
		ReleaseID:                   request.ReleaseID,
		Revision:                    request.Revision,
		ArtifactDigest:              request.ArtifactDigest,
		ManifestDigest:              core.DigestBytes([]byte("placeholder-manifest")),
		LocalReadinessRef:           local.ExactRefV1(),
		ResourceBindingSetRef:       objectRef("organization-resource-binding"),
		CleanupCurrentRef:           objectRef("organization-cleanup-current"),
		DeploymentAttestationRef:    objectRef("organization-deployment"),
		ExecutableFactoryBindingRef: objectRef("organization-executable-factory"),
		CertificationFactRef:        objectRef(request.CertificationID),
		CheckedUnixNano:             fixedNow.Add(-time.Minute).UnixNano(),
		ExpiresUnixNano:             fixedNow.Add(20 * time.Minute).UnixNano(),
	}
	draft, err := buildReleasePayloadV1(request, assemblercontract.SupportProductionV1, &local, &value, false)
	if err != nil {
		t.Fatal(err)
	}
	value.ManifestDigest, err = draft.ComponentManifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	value.CertificationFactRef = draft.CertificationRef
	value, err = SealProductionReadinessV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestPublicationModesAndOwnerBoundaries(t *testing.T) {
	local := localReadiness(t, 2)
	production := productionReadiness(t, local)
	tests := []struct {
		name       string
		local      *localReader
		production *productionReader
		request    PublicationRequestV1
		mode       assemblercontract.SupportModeV1
		prod       bool
	}{
		{"reference", &localReader{}, &productionReader{}, publicationRequest(1), assemblercontract.SupportReferenceOnlyV1, false},
		{"standalone", &localReader{values: []LocalReadinessProjectionV1{local, local}}, &productionReader{}, publicationRequest(2), assemblercontract.SupportStandaloneV1, false},
		{"production", &localReader{values: []LocalReadinessProjectionV1{local, local}}, &productionReader{values: []ProductionReadinessProjectionV1{production, production}}, publicationRequest(2), assemblercontract.SupportProductionV1, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			publisher, err := NewPublisherV1(test.local, test.production, newReleaseCatalog(), func() time.Time { return fixedNow })
			if err != nil {
				t.Fatal(err)
			}
			result, err := publisher.Publish(context.Background(), test.request)
			if err != nil {
				t.Fatal(err)
			}
			if result.Release.SupportMode != test.mode || result.ProductionReady != test.prod {
				t.Fatalf("mode=%q production=%v", result.Release.SupportMode, result.ProductionReady)
			}
			if len(result.Release.FactoryDescriptors) != 1 || len(result.Release.PortSpecs) != 1 || len(result.Release.CapabilityDescriptors) != 1 {
				t.Fatal("declarative construction closure is incomplete")
			}
			if result.LocalReady {
				found := false
				for _, evidence := range result.Release.EvidenceRefs {
					if evidence == result.LocalReadiness.ExactRefV1() {
						found = true
					}
				}
				if !found {
					t.Fatal("release does not bind the exact SQLite local readiness projection")
				}
			}
			if len(result.Release.ComponentManifest.Dependencies) != 0 || len(result.Release.ComponentManifest.RequiredCapabilities) != 0 {
				t.Fatal("Organization readiness acquired an external Review consumer dependency")
			}
			owners := result.Release.ComponentManifest.Owners
			if len(owners) != 3 || owners[0].OwnerComponentID != runtimeports.RuntimeSharedEngineComponentIDV1 || owners[1].OwnerComponentID != runtimeports.RuntimeSharedEngineComponentIDV1 || owners[2].OwnerComponentID != ComponentIDV1 {
				t.Fatalf("fact ownership drifted: %#v", owners)
			}
			report, err := EvaluateConformanceCandidateV1(result, fixedNow)
			if err != nil || !report.ReleaseValid || !report.ConstructionClosureValid || !report.DescriptorOnly || report.ProductionClaimEligible != test.prod {
				t.Fatalf("report=%#v err=%v", report, err)
			}
		})
	}
}

func TestMemoryReadinessCannotBecomeStandalone(t *testing.T) {
	value := localReadiness(t, 3)
	value.BackendKind = "memory"
	value.Digest, _ = LocalReadinessDigestV1(value)
	if err := value.ValidateCurrent(fixedNow); err == nil {
		t.Fatal("memory readiness was accepted as SQLite")
	}
	catalog := newReleaseCatalog()
	publisher, _ := NewPublisherV1(&localReader{values: []LocalReadinessProjectionV1{value}}, &productionReader{}, catalog, func() time.Time { return fixedNow })
	if _, err := publisher.Publish(context.Background(), publicationRequest(3)); err == nil || catalog.commitCount() != 0 {
		t.Fatalf("invalid local readiness reached catalog: %v", err)
	}
}

func TestLostReplyRecoversByExactInspect(t *testing.T) {
	catalog := newReleaseCatalog()
	catalog.lostReply.Store(true)
	publisher, _ := NewPublisherV1(&localReader{}, &productionReader{}, catalog, func() time.Time { return fixedNow })
	result, err := publisher.Publish(context.Background(), publicationRequest(4))
	if err != nil {
		t.Fatal(err)
	}
	if result.Release.SupportMode != assemblercontract.SupportReferenceOnlyV1 || catalog.commitCount() != 1 {
		t.Fatalf("lost reply recovery wrote %d releases", catalog.commitCount())
	}
}

func TestSameRevisionCannotPromoteOrChangeContent(t *testing.T) {
	catalog := newReleaseCatalog()
	reference, _ := NewPublisherV1(&localReader{}, &productionReader{}, catalog, func() time.Time { return fixedNow })
	request := publicationRequest(5)
	first, err := reference.Publish(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	local := localReadiness(t, 5)
	standalone, _ := NewPublisherV1(&localReader{values: []LocalReadinessProjectionV1{local, local}}, &productionReader{}, catalog, func() time.Time { return fixedNow })
	if _, err = standalone.Publish(context.Background(), request); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same revision promotion was accepted: %v", err)
	}
	stored, err := catalog.InspectExactComponentReleaseV1(context.Background(), first.Release.RefV1())
	if err != nil || stored.SupportMode != assemblercontract.SupportReferenceOnlyV1 || catalog.commitCount() != 1 {
		t.Fatalf("first release was replaced: %#v err=%v", stored, err)
	}
}

func TestReadinessDriftExpiryUnavailableAndClockRollbackFailClosed(t *testing.T) {
	local := localReadiness(t, 6)
	drift := local
	drift.CurrentReaderRef = objectRef("different-current-reader")
	drift, _ = SealLocalReadinessV1(drift)
	tests := []struct {
		name  string
		local *localReader
		clock func() time.Time
	}{
		{"drift", &localReader{values: []LocalReadinessProjectionV1{local, drift}}, func() time.Time { return fixedNow }},
		{"unavailable", &localReader{err: core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "SQLite reader unavailable")}, func() time.Time { return fixedNow }},
		{"expired", &localReader{values: []LocalReadinessProjectionV1{local}}, func() time.Time { return fixedNow.Add(time.Hour) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := newReleaseCatalog()
			publisher, _ := NewPublisherV1(test.local, &productionReader{}, catalog, test.clock)
			if _, err := publisher.Publish(context.Background(), publicationRequest(6)); err == nil || catalog.commitCount() != 0 {
				t.Fatalf("unsafe readiness reached catalog: %v", err)
			}
		})
	}
	var calls atomic.Int64
	clock := func() time.Time {
		if calls.Add(1) == 1 {
			return fixedNow
		}
		return fixedNow.Add(-time.Second)
	}
	catalog := newReleaseCatalog()
	publisher, _ := NewPublisherV1(&localReader{}, &productionReader{}, catalog, clock)
	if _, err := publisher.Publish(context.Background(), publicationRequest(7)); err == nil || catalog.commitCount() != 0 {
		t.Fatalf("clock rollback reached catalog: %v", err)
	}
}

func TestClockCursorRejectsMidPublicationRegressionBeforeCatalogWrite(t *testing.T) {
	local := localReadiness(t, 14)
	observations := []time.Time{fixedNow, fixedNow.Add(5 * time.Second), fixedNow.Add(3 * time.Second)}
	var index atomic.Int64
	clock := func() time.Time {
		position := int(index.Add(1)) - 1
		if position >= len(observations) {
			return observations[len(observations)-1]
		}
		return observations[position]
	}
	catalog := newReleaseCatalog()
	publisher, err := NewPublisherV1(
		&localReader{values: []LocalReadinessProjectionV1{local, local}},
		&productionReader{},
		catalog,
		clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = publisher.Publish(context.Background(), publicationRequest(14)); err == nil || !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("mid-publication clock regression was not rejected: %v", err)
	}
	if catalog.commitCount() != 0 {
		t.Fatalf("clock regression wrote %d catalog records", catalog.commitCount())
	}
}

func TestProductionRequiresExactOrganizationOwnedClosure(t *testing.T) {
	local := localReadiness(t, 8)
	production := productionReadiness(t, local)
	production.ResourceBindingSetRef = objectRef("different-resource-binding")
	// Keeping the old digest proves that a field splice cannot self-authorize.
	catalog := newReleaseCatalog()
	publisher, _ := NewPublisherV1(&localReader{values: []LocalReadinessProjectionV1{local}}, &productionReader{values: []ProductionReadinessProjectionV1{production}}, catalog, func() time.Time { return fixedNow })
	if _, err := publisher.Publish(context.Background(), publicationRequest(8)); err == nil || catalog.commitCount() != 0 {
		t.Fatalf("spliced production readiness reached catalog: %v", err)
	}

	withoutLocal, _ := NewPublisherV1(&localReader{}, &productionReader{values: []ProductionReadinessProjectionV1{production}}, newReleaseCatalog(), func() time.Time { return fixedNow })
	if _, err := withoutLocal.Publish(context.Background(), publicationRequest(8)); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("production without SQLite closure was accepted: %v", err)
	}

	aliased := productionReadiness(t, local)
	aliased.CleanupCurrentRef = assemblycontract.ObjectRefV1{ID: aliased.ResourceBindingSetRef.ID, Revision: 2, Digest: core.DigestBytes([]byte("different-revision-same-proof-id"))}
	if _, err := SealProductionReadinessV1(aliased); err == nil {
		t.Fatal("two production proof roles shared one stable proof identity")
	}
}

func TestProductionS1S2DriftAndTTLCrossingFailBeforeWrite(t *testing.T) {
	local := localReadiness(t, 12)
	production := productionReadiness(t, local)
	drift := production
	drift.CleanupCurrentRef = objectRef("different-cleanup-current")
	drift, err := SealProductionReadinessV1(drift)
	if err != nil {
		t.Fatal(err)
	}
	catalog := newReleaseCatalog()
	publisher, _ := NewPublisherV1(
		&localReader{values: []LocalReadinessProjectionV1{local, local}},
		&productionReader{values: []ProductionReadinessProjectionV1{production, drift}},
		catalog,
		func() time.Time { return fixedNow },
	)
	if _, err = publisher.Publish(context.Background(), publicationRequest(12)); err == nil || catalog.commitCount() != 0 {
		t.Fatalf("production S1/S2 drift reached catalog: %v", err)
	}

	local = localReadiness(t, 13)
	var calls atomic.Int64
	clock := func() time.Time {
		switch calls.Add(1) {
		case 1, 2:
			return fixedNow
		default:
			return time.Unix(0, local.ExpiresUnixNano)
		}
	}
	catalog = newReleaseCatalog()
	publisher, _ = NewPublisherV1(&localReader{values: []LocalReadinessProjectionV1{local, local}}, &productionReader{}, catalog, clock)
	if _, err = publisher.Publish(context.Background(), publicationRequest(13)); err == nil || catalog.commitCount() != 0 {
		t.Fatalf("TTL crossing reached catalog: %v", err)
	}
}

func TestConcurrentPublishLinearizesOneExactRelease(t *testing.T) {
	catalog := newReleaseCatalog()
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	digests := make(chan core.Digest, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			publisher, err := NewPublisherV1(&localReader{}, &productionReader{}, catalog, func() time.Time { return fixedNow })
			if err != nil {
				errs <- err
				return
			}
			result, err := publisher.Publish(context.Background(), publicationRequest(9))
			if err != nil {
				errs <- err
				return
			}
			digests <- result.Release.ReleaseDigest
		}()
	}
	wg.Wait()
	close(errs)
	close(digests)
	for err := range errs {
		t.Fatal(err)
	}
	var expected core.Digest
	for digest := range digests {
		if expected == "" {
			expected = digest
		} else if digest != expected {
			t.Fatal("concurrent publishers returned different release content")
		}
	}
	if catalog.commitCount() != 1 {
		t.Fatalf("commits=%d", catalog.commitCount())
	}
}

func TestTypedNilDependenciesAndNilContextFailClosed(t *testing.T) {
	var local *localReader
	if _, err := NewPublisherV1(local, &productionReader{}, newReleaseCatalog(), func() time.Time { return fixedNow }); err == nil {
		t.Fatal("typed-nil local reader accepted")
	}
	var catalog *releaseCatalog
	if _, err := NewPublisherV1(&localReader{}, &productionReader{}, catalog, func() time.Time { return fixedNow }); err == nil {
		t.Fatal("typed-nil catalog accepted")
	}
	var production *productionReader
	if _, err := NewPublisherV1(&localReader{}, production, newReleaseCatalog(), func() time.Time { return fixedNow }); err == nil {
		t.Fatal("typed-nil production reader accepted")
	}
	publisher, _ := NewPublisherV1(&localReader{}, &productionReader{}, newReleaseCatalog(), func() time.Time { return fixedNow })
	if _, err := publisher.Publish(nil, publicationRequest(10)); err == nil {
		t.Fatal("nil context accepted")
	}
}

func TestConformanceRejectsSupportAndProofSplices(t *testing.T) {
	publisher, _ := NewPublisherV1(&localReader{}, &productionReader{}, newReleaseCatalog(), func() time.Time { return fixedNow })
	result, err := publisher.Publish(context.Background(), publicationRequest(11))
	if err != nil {
		t.Fatal(err)
	}
	result.Release.SupportMode = assemblercontract.SupportProductionV1
	if _, err = EvaluateConformanceCandidateV1(result, fixedNow); err == nil {
		t.Fatal("support mode splice passed conformance")
	}
}
