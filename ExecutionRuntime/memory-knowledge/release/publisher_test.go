package release

import (
	"context"
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var now = time.Date(2026, 7, 18, 4, 0, 0, 0, time.UTC)

type localReader struct {
	mu     sync.Mutex
	values []LocalReadinessProjectionV1
	err    error
	calls  int
}

func (s *localReader) InspectMemoryKnowledgeLocalReadinessV1(context.Context, string, core.Revision) (LocalReadinessProjectionV1, error) {
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

func (s *productionReader) InspectMemoryKnowledgeProductionReadinessV1(context.Context, string, core.Revision) (ProductionReadinessProjectionV1, error) {
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
func req(rev core.Revision) PublicationRequestV1 {
	return PublicationRequestV1{ReleaseID: "praxis.memory-knowledge/release", Revision: rev, SourceRef: obj("source"), PublisherRef: obj("publisher"), TrustRef: obj("trust"), CertificationID: "certification", ArtifactDigest: core.DigestBytes([]byte("artifact")), CreatedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
}
func local(t *testing.T, rev core.Revision) LocalReadinessProjectionV1 {
	t.Helper()
	rs := []assemblycontract.ObjectRefV1{obj("memory"), obj("knowledge"), obj("retrieval"), obj("memory-context"), obj("knowledge-context"), obj("purge")}
	p, e := SealLocalV1(LocalReadinessProjectionV1{ReleaseID: req(rev).ReleaseID, Revision: rev, ArtifactDigest: req(rev).ArtifactDigest, MemoryOwnerRef: rs[0], KnowledgeOwnerRef: rs[1], RetrievalRef: rs[2], MemoryContextSourceRef: rs[3], KnowledgeContextSourceRef: rs[4], PurgeInspectRef: rs[5], CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Minute).UnixNano()})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func production(t *testing.T, rev core.Revision) ProductionReadinessProjectionV1 {
	t.Helper()
	names := []string{"memory-fact", "memory-content", "knowledge-fact", "knowledge-content", "authority", "credential", "index", "context", "settlement", "purge-effect", "cleanup", "deployment", "certification"}
	rs := make([]assemblycontract.ObjectRefV1, len(names))
	for i, n := range names {
		rs[i] = obj(n)
	}
	r := req(rev)
	p := ProductionReadinessProjectionV1{ReleaseID: r.ReleaseID, Revision: rev, ArtifactDigest: r.ArtifactDigest, DurableMemoryFactStoreRef: rs[0], DurableMemoryContentStoreRef: rs[1], DurableKnowledgeFactStoreRef: rs[2], DurableKnowledgeContentStoreRef: rs[3], AuthorityPolicyCurrentRef: rs[4], CredentialCurrentRef: rs[5], RetrievalIndexCurrentRef: rs[6], ContextSourceCurrentRef: rs[7], SettlementCurrentRef: rs[8], PurgeEffectRef: rs[9], CleanupOwnerRef: rs[10], DeploymentAttestationRef: rs[11], CertificationFactRef: rs[12], CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(20 * time.Minute).UnixNano()}
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
func TestModesAndProductionCertification(t *testing.T) {
	for _, tc := range []struct {
		rev  core.Revision
		l    *localReader
		mode assemblercontract.SupportModeV1
	}{{1, &localReader{}, assemblercontract.SupportReferenceOnlyV1}, {2, &localReader{values: []LocalReadinessProjectionV1{local(t, 2), local(t, 2)}}, assemblercontract.SupportStandaloneV1}} {
		p, _ := NewPublisherV1(tc.l, &productionReader{}, newCatalog(), func() time.Time { return now })
		got, e := p.Publish(context.Background(), req(tc.rev))
		if e != nil {
			t.Fatal(e)
		}
		if got.Release.SupportMode != tc.mode || got.ProductionReady {
			t.Fatalf("mode drift %#v", got)
		}
		if len(got.Release.PortSpecs) != 2 || len(got.Release.FactoryDescriptors) != 2 {
			t.Fatal("construction closure missing")
		}
	}
	l := local(t, 3)
	prod := production(t, 3)
	p, _ := NewPublisherV1(&localReader{values: []LocalReadinessProjectionV1{l, l}}, &productionReader{values: []ProductionReadinessProjectionV1{prod, prod}}, newCatalog(), func() time.Time { return now })
	got, e := p.Publish(context.Background(), req(3))
	if e != nil {
		t.Fatal(e)
	}
	report, e := EvaluateV1(got, now)
	if e != nil || !report.ProductionEligible {
		t.Fatalf("production=%#v err=%v", report, e)
	}
}
func TestProductionRequiresLocalAndDriftFails(t *testing.T) {
	prod := production(t, 4)
	store := newCatalog()
	p, _ := NewPublisherV1(&localReader{}, &productionReader{values: []ProductionReadinessProjectionV1{prod}}, store, func() time.Time { return now })
	if _, e := p.Publish(context.Background(), req(4)); !core.HasCategory(e, core.ErrorPreconditionFailed) {
		t.Fatalf("production without local: %v", e)
	}
	l := local(t, 5)
	changed := l
	changed.PurgeInspectRef = obj("changed-purge")
	changed, _ = SealLocalV1(changed)
	p, _ = NewPublisherV1(&localReader{values: []LocalReadinessProjectionV1{l, changed}}, &productionReader{}, store, func() time.Time { return now })
	if _, e := p.Publish(context.Background(), req(5)); !core.HasCategory(e, core.ErrorConflict) {
		t.Fatalf("drift accepted: %v", e)
	}
}
func TestUnavailableTTLClockTypedNilFailClosed(t *testing.T) {
	unavailable := core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "unavailable")
	p, _ := NewPublisherV1(&localReader{err: unavailable}, &productionReader{}, newCatalog(), func() time.Time { return now })
	if _, e := p.Publish(context.Background(), req(6)); !core.HasCategory(e, core.ErrorUnavailable) {
		t.Fatalf("downgraded: %v", e)
	}
	var typed *localReader
	if _, e := NewPublisherV1(typed, &productionReader{}, newCatalog(), func() time.Time { return now }); e == nil {
		t.Fatal("typed nil accepted")
	}
	l := local(t, 7)
	l.ExpiresUnixNano = now.Add(time.Nanosecond).UnixNano()
	l, _ = SealLocalV1(l)
	clock := &clockSeq{values: []time.Time{now, now.Add(2 * time.Nanosecond)}}
	p, _ = NewPublisherV1(&localReader{values: []LocalReadinessProjectionV1{l}}, &productionReader{}, newCatalog(), clock.Now)
	if _, e := p.Publish(context.Background(), req(7)); !core.HasCategory(e, core.ErrorPreconditionFailed) {
		t.Fatalf("TTL accepted: %v", e)
	}
	clock = &clockSeq{values: []time.Time{now, now.Add(-time.Nanosecond)}}
	p, _ = NewPublisherV1(&localReader{values: []LocalReadinessProjectionV1{local(t, 8)}}, &productionReader{}, newCatalog(), clock.Now)
	if _, e := p.Publish(context.Background(), req(8)); !core.HasReason(e, core.ReasonClockRegression) {
		t.Fatalf("rollback accepted: %v", e)
	}
}
func TestLostReplyAnd64ConcurrentLinearizeOnce(t *testing.T) {
	store := newCatalog()
	store.lost = true
	p, _ := NewPublisherV1(&localReader{}, &productionReader{}, store, func() time.Time { return now })
	if _, e := p.Publish(context.Background(), req(9)); e != nil {
		t.Fatal(e)
	}
	if store.commits != 1 {
		t.Fatal("lost reply recreated")
	}
	store = newCatalog()
	var fail atomic.Int64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			p, _ := NewPublisherV1(&localReader{}, &productionReader{}, store, func() time.Time { return now })
			if _, e := p.Publish(context.Background(), req(10)); e != nil {
				fail.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if fail.Load() != 0 || store.commits != 1 {
		t.Fatalf("fail=%d commits=%d", fail.Load(), store.commits)
	}
}
func TestAliasingAndChangedContentFail(t *testing.T) {
	l := local(t, 11)
	l.PurgeInspectRef = l.RetrievalRef
	if _, e := SealLocalV1(l); !core.HasCategory(e, core.ErrorConflict) {
		t.Fatalf("alias accepted: %v", e)
	}
	store := newCatalog()
	p, _ := NewPublisherV1(&localReader{}, &productionReader{}, store, func() time.Time { return now })
	if _, e := p.Publish(context.Background(), req(12)); e != nil {
		t.Fatal(e)
	}
	changed := req(12)
	changed.ArtifactDigest = core.DigestBytes([]byte("changed"))
	if _, e := p.Publish(context.Background(), changed); !core.HasCategory(e, core.ErrorConflict) {
		t.Fatalf("changed content accepted: %v", e)
	}
}

type clockSeq struct {
	mu     sync.Mutex
	values []time.Time
	i      int
}

func (c *clockSeq) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.i >= len(c.values) {
		return c.values[len(c.values)-1]
	}
	v := c.values[c.i]
	c.i++
	return v
}
