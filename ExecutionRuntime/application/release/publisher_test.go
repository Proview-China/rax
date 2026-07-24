package release

import (
	"context"
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var fixed = time.Date(2026, 7, 18, 6, 0, 0, 0, time.UTC)

type localReader struct {
	mu     sync.Mutex
	values []LocalReadinessProjectionV1
	err    error
	calls  int
}

func (s *localReader) InspectApplicationLocalReadinessV1(context.Context, string, core.Revision) (LocalReadinessProjectionV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return LocalReadinessProjectionV1{}, s.err
	}
	if len(s.values) == 0 {
		return LocalReadinessProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "absent")
	}
	i := s.calls - 1
	if i >= len(s.values) {
		i = len(s.values) - 1
	}
	return s.values[i], nil
}

type productionReader struct {
	mu     sync.Mutex
	values []ProductionReadinessProjectionV1
	err    error
	calls  int
}

func (s *productionReader) InspectApplicationProductionReadinessV1(context.Context, string, core.Revision) (ProductionReadinessProjectionV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return ProductionReadinessProjectionV1{}, s.err
	}
	if len(s.values) == 0 {
		return ProductionReadinessProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "absent")
	}
	i := s.calls - 1
	if i >= len(s.values) {
		i = len(s.values) - 1
	}
	return s.values[i], nil
}

type catalog struct {
	mu      sync.Mutex
	data    map[string]assemblercontract.ComponentReleaseV1
	commits int
	lost    bool
}

func newCatalog() *catalog { return &catalog{data: map[string]assemblercontract.ComponentReleaseV1{}} }
func (s *catalog) EnsureExactComponentReleaseV1(_ context.Context, v assemblercontract.ComponentReleaseV1) (assemblercontract.ComponentReleaseV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := v.Validate(); e != nil {
		return assemblercontract.ComponentReleaseV1{}, e
	}
	k := v.ReleaseID + strconv.FormatUint(uint64(v.Revision), 10)
	if old, ok := s.data[k]; ok {
		if old.ReleaseDigest != v.ReleaseDigest {
			return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "changed")
		}
		return assemblercontract.CloneComponentReleaseV1(old), nil
	}
	s.data[k] = assemblercontract.CloneComponentReleaseV1(v)
	s.commits++
	if s.lost {
		s.lost = false
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "lost")
	}
	return assemblercontract.CloneComponentReleaseV1(v), nil
}
func (s *catalog) InspectExactComponentReleaseV1(_ context.Context, r assemblercontract.ComponentReleaseRefV1) (assemblercontract.ComponentReleaseV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[r.ReleaseID+strconv.FormatUint(uint64(r.Revision), 10)]
	if !ok {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "missing")
	}
	if v.RefV1() != r {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "drift")
	}
	return assemblercontract.CloneComponentReleaseV1(v), nil
}
func obj(id string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
}
func request(rev core.Revision) PublicationRequestV1 {
	return PublicationRequestV1{ReleaseID: "praxis.application/release", Revision: rev, SourceRef: obj("source"), PublisherRef: obj("publisher"), TrustRef: obj("trust"), CertificationID: "certification", ArtifactDigest: core.DigestBytes([]byte("artifact")), CreatedUnixNano: fixed.Add(-time.Minute).UnixNano(), ExpiresUnixNano: fixed.Add(time.Hour).UnixNano()}
}
func local(t *testing.T, rev core.Revision) LocalReadinessProjectionV1 {
	t.Helper()
	p, e := SealLocalV1(LocalReadinessProjectionV1{ReleaseID: request(rev).ReleaseID, Revision: rev, ArtifactDigest: request(rev).ArtifactDigest, CommandWorkflowRef: obj("command-workflow"), RunCoordinationRef: obj("run"), GovernedOperationRef: obj("governed"), G6ARef: obj("g6a"), ContextRefreshRef: obj("context"), CheckpointRef: obj("checkpoint"), CheckedUnixNano: fixed.UnixNano(), ExpiresUnixNano: fixed.Add(30 * time.Minute).UnixNano()})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func production(t *testing.T, rev core.Revision) ProductionReadinessProjectionV1 {
	t.Helper()
	names := []string{"command", "journal", "attempt", "run", "g6a", "context", "checkpoint", "outbox-worker", "recovery-worker", "governance", "settlement", "execution", "cleanup", "root", "deployment", "certification"}
	rs := make([]assemblycontract.ObjectRefV1, len(names))
	for i, n := range names {
		rs[i] = obj(n)
	}
	r := request(rev)
	p := ProductionReadinessProjectionV1{ReleaseID: r.ReleaseID, Revision: rev, ArtifactDigest: r.ArtifactDigest, DurableCommandOutboxStoreRef: rs[0], DurableWorkflowJournalStoreRef: rs[1], DurableOperationAttemptStoreRef: rs[2], DurableRunStoreRef: rs[3], DurableG6AStoreRef: rs[4], DurableContextRefreshStoreRef: rs[5], DurableCheckpointStoreRef: rs[6], OutboxWorkerRef: rs[7], RecoveryWorkerRef: rs[8], RuntimeGovernanceGatewayRef: rs[9], RunSettlementGatewayRef: rs[10], ExecutionGatewayRef: rs[11], CleanupOwnerRef: rs[12], ProductionRootRef: rs[13], DeploymentAttestationRef: rs[14], CertificationFactRef: rs[15], CheckedUnixNano: fixed.UnixNano(), ExpiresUnixNano: fixed.Add(20 * time.Minute).UnixNano()}
	candidate, e := buildPayload(r, assemblercontract.SupportProductionV1, &p, false)
	if e != nil {
		t.Fatal(e)
	}
	p.ManifestDigest, e = candidate.ComponentManifest.BindingDigestV2()
	if e != nil {
		t.Fatal(e)
	}
	p.CertificationFactRef = assemblycontract.ObjectRefV1{ID: r.CertificationID, Revision: r.Revision, Digest: candidate.CertificationRef.Digest}
	p, e = SealProductionV1(p)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestTruthfulModesAndProduction(t *testing.T) {
	for _, tc := range []struct {
		rev  core.Revision
		l    *localReader
		mode assemblercontract.SupportModeV1
	}{{1, &localReader{}, assemblercontract.SupportReferenceOnlyV1}, {2, &localReader{values: []LocalReadinessProjectionV1{local(t, 2), local(t, 2)}}, assemblercontract.SupportStandaloneV1}} {
		p, _ := NewPublisherV1(tc.l, &productionReader{}, newCatalog(), func() time.Time { return fixed })
		got, e := p.Publish(context.Background(), request(tc.rev))
		if e != nil {
			t.Fatal(e)
		}
		if got.Release.SupportMode != tc.mode || got.ProductionReady {
			t.Fatalf("mode drift %#v", got)
		}
		if len(got.Release.CapabilityDescriptors) != 7 || len(got.Release.PortSpecs) != 6 || len(got.Release.FactoryDescriptors) != 6 {
			t.Fatal("shared engine closure incomplete")
		}
		owners := got.Release.ComponentManifest.Owners
		if owners[0].OwnerComponentID == ComponentIDV1 || owners[1].OwnerComponentID == ComponentIDV1 || owners[2].OwnerComponentID != ComponentIDV1 {
			t.Fatal("Application self-assigned Runtime Effect/Settlement ownership")
		}
		if len(got.Release.ComponentManifest.RequiredCapabilities) != 1 || got.Release.ComponentManifest.RequiredCapabilities[0].ProviderComponent != runtimeports.RuntimeSharedEngineComponentIDV1 || len(got.Release.Dependencies) != 1 || got.Release.ComponentManifest.Dependencies[0].ComponentID != runtimeports.RuntimeSharedEngineComponentIDV1 {
			t.Fatal("Runtime governance dependency is not explicit")
		}
	}
	l := local(t, 3)
	prod := production(t, 3)
	p, _ := NewPublisherV1(&localReader{values: []LocalReadinessProjectionV1{l, l}}, &productionReader{values: []ProductionReadinessProjectionV1{prod, prod}}, newCatalog(), func() time.Time { return fixed })
	got, e := p.Publish(context.Background(), request(3))
	if e != nil {
		t.Fatal(e)
	}
	report, e := EvaluateV1(got, fixed)
	if e != nil || !report.ProductionEligible {
		t.Fatalf("production report %#v err=%v", report, e)
	}
}
func TestNoPartialCoordinatorCanPromoteProduction(t *testing.T) {
	prod := production(t, 4)
	store := newCatalog()
	p, _ := NewPublisherV1(&localReader{}, &productionReader{values: []ProductionReadinessProjectionV1{prod}}, store, func() time.Time { return fixed })
	if _, e := p.Publish(context.Background(), request(4)); !core.HasCategory(e, core.ErrorPreconditionFailed) {
		t.Fatalf("production without all local engines: %v", e)
	}
	if store.commits != 0 {
		t.Fatal("partial coordinator reached catalog")
	}
}
func TestDriftUnavailableTTLClockAliasTypedNil(t *testing.T) {
	l := local(t, 5)
	changed := l
	changed.CheckpointRef = obj("changed")
	changed, _ = SealLocalV1(changed)
	store := newCatalog()
	p, _ := NewPublisherV1(&localReader{values: []LocalReadinessProjectionV1{l, changed}}, &productionReader{}, store, func() time.Time { return fixed })
	if _, e := p.Publish(context.Background(), request(5)); !core.HasCategory(e, core.ErrorConflict) {
		t.Fatalf("drift accepted: %v", e)
	}
	unavailable := core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "unavailable")
	p, _ = NewPublisherV1(&localReader{err: unavailable}, &productionReader{}, newCatalog(), func() time.Time { return fixed })
	if _, e := p.Publish(context.Background(), request(6)); !core.HasCategory(e, core.ErrorUnavailable) {
		t.Fatalf("unavailable downgraded: %v", e)
	}
	l = local(t, 7)
	l.ExpiresUnixNano = fixed.Add(time.Nanosecond).UnixNano()
	l, _ = SealLocalV1(l)
	clock := &seqClock{values: []time.Time{fixed, fixed.Add(2 * time.Nanosecond)}}
	p, _ = NewPublisherV1(&localReader{values: []LocalReadinessProjectionV1{l}}, &productionReader{}, newCatalog(), clock.Now)
	if _, e := p.Publish(context.Background(), request(7)); !core.HasCategory(e, core.ErrorPreconditionFailed) {
		t.Fatalf("TTL accepted: %v", e)
	}
	clock = &seqClock{values: []time.Time{fixed, fixed.Add(-time.Nanosecond)}}
	p, _ = NewPublisherV1(&localReader{values: []LocalReadinessProjectionV1{local(t, 8)}}, &productionReader{}, newCatalog(), clock.Now)
	if _, e := p.Publish(context.Background(), request(8)); !core.HasReason(e, core.ReasonClockRegression) {
		t.Fatalf("rollback accepted: %v", e)
	}
	l = local(t, 9)
	l.CheckpointRef = l.ContextRefreshRef
	if _, e := SealLocalV1(l); !core.HasCategory(e, core.ErrorConflict) {
		t.Fatalf("alias accepted: %v", e)
	}
	var typed *localReader
	if _, e := NewPublisherV1(typed, &productionReader{}, newCatalog(), func() time.Time { return fixed }); e == nil {
		t.Fatal("typed nil accepted")
	}
}
func TestLostReply64ConcurrentAndChangedContent(t *testing.T) {
	store := newCatalog()
	store.lost = true
	p, _ := NewPublisherV1(&localReader{}, &productionReader{}, store, func() time.Time { return fixed })
	if _, e := p.Publish(context.Background(), request(10)); e != nil {
		t.Fatal(e)
	}
	if store.commits != 1 {
		t.Fatal("lost reply recreated")
	}
	store = newCatalog()
	var failures atomic.Int64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			p, _ := NewPublisherV1(&localReader{}, &productionReader{}, store, func() time.Time { return fixed })
			if _, e := p.Publish(context.Background(), request(11)); e != nil {
				failures.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if failures.Load() != 0 || store.commits != 1 {
		t.Fatalf("fail=%d commits=%d", failures.Load(), store.commits)
	}
	p, _ = NewPublisherV1(&localReader{}, &productionReader{}, store, func() time.Time { return fixed })
	changed := request(11)
	changed.ArtifactDigest = core.DigestBytes([]byte("changed"))
	if _, e := p.Publish(context.Background(), changed); !core.HasCategory(e, core.ErrorConflict) {
		t.Fatalf("changed content accepted: %v", e)
	}
}

type seqClock struct {
	mu     sync.Mutex
	values []time.Time
	i      int
}

func (c *seqClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.i >= len(c.values) {
		return c.values[len(c.values)-1]
	}
	v := c.values[c.i]
	c.i++
	return v
}
