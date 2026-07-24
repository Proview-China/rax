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

var fixed = time.Date(2026, 7, 18, 2, 0, 0, 0, time.UTC)

type localStub struct {
	mu     sync.Mutex
	values []LocalReadinessProjectionV1
	err    error
	calls  int
}

func (s *localStub) InspectToolMCPLocalReadinessV1(context.Context, string, core.Revision) (LocalReadinessProjectionV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return LocalReadinessProjectionV1{}, s.err
	}
	if len(s.values) == 0 {
		return LocalReadinessProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "local readiness absent")
	}
	i := s.calls - 1
	if i >= len(s.values) {
		i = len(s.values) - 1
	}
	return s.values[i], nil
}

type productionStub struct {
	mu     sync.Mutex
	values []ProductionReadinessProjectionV1
	err    error
	calls  int
}

func (s *productionStub) InspectToolMCPProductionReadinessV1(context.Context, string, core.Revision) (ProductionReadinessProjectionV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return ProductionReadinessProjectionV1{}, s.err
	}
	if len(s.values) == 0 {
		return ProductionReadinessProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "production readiness absent")
	}
	i := s.calls - 1
	if i >= len(s.values) {
		i = len(s.values) - 1
	}
	return s.values[i], nil
}

type catalog struct {
	mu      sync.Mutex
	values  map[string]assemblercontract.ComponentReleaseV1
	commits int
	lost    bool
}

func newCatalog() *catalog {
	return &catalog{values: map[string]assemblercontract.ComponentReleaseV1{}}
}
func (s *catalog) EnsureExactComponentReleaseV1(_ context.Context, v assemblercontract.ComponentReleaseV1) (assemblercontract.ComponentReleaseV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := v.Validate(); e != nil {
		return assemblercontract.ComponentReleaseV1{}, e
	}
	k := v.ReleaseID + "/" + strconv.FormatUint(uint64(v.Revision), 10)
	if old, ok := s.values[k]; ok {
		if old.ReleaseDigest != v.ReleaseDigest {
			return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "release conflict")
		}
		return assemblercontract.CloneComponentReleaseV1(old), nil
	}
	s.values[k] = assemblercontract.CloneComponentReleaseV1(v)
	s.commits++
	if s.lost {
		s.lost = false
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "lost reply")
	}
	return assemblercontract.CloneComponentReleaseV1(v), nil
}
func (s *catalog) InspectExactComponentReleaseV1(_ context.Context, r assemblercontract.ComponentReleaseRefV1) (assemblercontract.ComponentReleaseV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.values[r.ReleaseID+"/"+strconv.FormatUint(uint64(r.Revision), 10)]
	if !ok {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "missing")
	}
	if v.RefV1() != r {
		return assemblercontract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "ref drift")
	}
	return assemblercontract.CloneComponentReleaseV1(v), nil
}
func object(id string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id))}
}
func request(rev core.Revision) PublicationRequestV1 {
	return PublicationRequestV1{ReleaseID: "praxis.tool-mcp/release", Revision: rev, SourceRef: object("tool-mcp-source"), PublisherRef: object("tool-mcp-publisher"), TrustRef: object("tool-mcp-trust"), CertificationID: "praxis.tool-mcp/certification", ArtifactDigest: core.DigestBytes([]byte("tool-mcp-artifact")), CreatedUnixNano: fixed.Add(-time.Minute).UnixNano(), ExpiresUnixNano: fixed.Add(time.Hour).UnixNano()}
}
func localProjection(t *testing.T, rev core.Revision) LocalReadinessProjectionV1 {
	t.Helper()
	ids := []string{"g6a", "surface", "surface-binding", "input-contract", "controlled-provider", "mcp-discovery", "mcp-lifecycle", "official-sdk"}
	r := make([]assemblycontract.ObjectRefV1, len(ids))
	for i, id := range ids {
		r[i] = object("tool-mcp-" + id)
	}
	p, e := SealLocalReadinessV1(LocalReadinessProjectionV1{ComponentID: ComponentIDV1, ReleaseID: request(rev).ReleaseID, Revision: rev, ArtifactDigest: request(rev).ArtifactDigest, G6AP4CurrentRef: r[0], SurfaceCurrentRef: r[1], SurfaceBindingCurrentRef: r[2], InputContractCurrentRef: r[3], ControlledProviderAdapterRef: r[4], MCPDiscoveryRef: r[5], MCPLifecycleRef: r[6], OfficialSDKCallRef: r[7], CheckedUnixNano: fixed.UnixNano(), ExpiresUnixNano: fixed.Add(30 * time.Minute).UnixNano()})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func productionProjection(t *testing.T, rev core.Revision) ProductionReadinessProjectionV1 {
	t.Helper()
	ids := []string{"action-store", "binding-store", "surface-store", "mcp-store", "credential", "transport", "provider", "actual-point", "evidence", "settlement", "mcp-current", "mcp-inspect", "cleanup", "deployment", "certification"}
	r := make([]assemblycontract.ObjectRefV1, len(ids))
	for i, id := range ids {
		r[i] = object("tool-mcp-prod-" + id)
	}
	req := request(rev)
	p := ProductionReadinessProjectionV1{ComponentID: ComponentIDV1, ReleaseID: req.ReleaseID, Revision: rev, ArtifactDigest: req.ArtifactDigest, DurableActionStoreRef: r[0], DurableBindingStoreRef: r[1], DurableSurfaceStoreRef: r[2], DurableMCPStoreRef: r[3], CredentialCurrentRef: r[4], ProviderTransportRef: r[5], ProviderCurrentRef: r[6], ControlledActualPointRef: r[7], EvidenceGovernanceRef: r[8], SettlementCurrentRef: r[9], MCPLifecycleCurrentRef: r[10], MCPInspectRef: r[11], CleanupOwnerRef: r[12], DeploymentAttestationRef: r[13], CertificationFactRef: r[14], CheckedUnixNano: fixed.UnixNano(), ExpiresUnixNano: fixed.Add(20 * time.Minute).UnixNano()}
	candidate, e := buildReleasePayloadV1(req, assemblercontract.SupportProductionV1, &p, false)
	if e != nil {
		t.Fatal(e)
	}
	p.ManifestDigest, e = candidate.ComponentManifest.BindingDigestV2()
	if e != nil {
		t.Fatal(e)
	}
	p.CertificationFactRef = assemblycontract.ObjectRefV1{ID: req.CertificationID, Revision: req.Revision, Digest: candidate.CertificationRef.Digest}
	sealed, e := SealProductionReadinessV1(p)
	if e != nil {
		t.Fatal(e)
	}
	return sealed
}

func TestReferenceStandaloneAndProductionRemainDistinct(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		local                  *localStub
		prod                   *productionStub
		mode                   assemblercontract.SupportModeV1
		localReady, production bool
	}{{"reference", &localStub{}, &productionStub{}, assemblercontract.SupportReferenceOnlyV1, false, false}, {"standalone", &localStub{values: []LocalReadinessProjectionV1{localProjection(t, 2), localProjection(t, 2)}}, &productionStub{}, assemblercontract.SupportStandaloneV1, true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			store := newCatalog()
			p, e := NewPublisherV1(tc.local, tc.prod, store, func() time.Time { return fixed })
			if e != nil {
				t.Fatal(e)
			}
			got, e := p.Publish(context.Background(), request(map[bool]core.Revision{false: 1, true: 2}[tc.localReady]))
			if e != nil {
				t.Fatal(e)
			}
			if got.Release.SupportMode != tc.mode || got.LocalReady != tc.localReady || got.ProductionReady != tc.production {
				t.Fatalf("truth drift: %#v", got)
			}
			if got.Release.ComponentManifest.Conformance == "fully_controlled" {
				t.Fatal("non-production candidate self-certified")
			}
			if len(got.Release.CapabilityDescriptors) != 2 || len(got.Release.PortSpecs) != 2 || len(got.Release.FactoryDescriptors) != 2 {
				t.Fatal("assembly closure incomplete")
			}
		})
	}
	local := localProjection(t, 3)
	prod := productionProjection(t, 3)
	store := newCatalog()
	p, _ := NewPublisherV1(&localStub{values: []LocalReadinessProjectionV1{local, local}}, &productionStub{values: []ProductionReadinessProjectionV1{prod, prod}}, store, func() time.Time { return fixed })
	got, e := p.Publish(context.Background(), request(3))
	if e != nil {
		t.Fatal(e)
	}
	if !got.ProductionReady || got.Release.SupportMode != assemblercontract.SupportProductionV1 {
		t.Fatalf("production not published: %#v", got)
	}
	report, e := EvaluateConformanceCandidateV1(got, fixed)
	if e != nil || !report.ProductionClaimEligible {
		t.Fatalf("conformance=%#v err=%v", report, e)
	}
}

func TestProductionCannotExistWithoutLocalClosure(t *testing.T) {
	prod := productionProjection(t, 4)
	store := newCatalog()
	p, _ := NewPublisherV1(&localStub{}, &productionStub{values: []ProductionReadinessProjectionV1{prod}}, store, func() time.Time { return fixed })
	if _, e := p.Publish(context.Background(), request(4)); !core.HasCategory(e, core.ErrorPreconditionFailed) {
		t.Fatalf("production without local closure accepted: %v", e)
	}
	if store.commits != 0 {
		t.Fatal("invalid production reached catalog")
	}
}
func TestDriftExpiryUnavailableAndAliasingFailClosed(t *testing.T) {
	local := localProjection(t, 5)
	drift := local
	drift.MCPLifecycleRef = object("drifted-lifecycle")
	drift, _ = SealLocalReadinessV1(drift)
	store := newCatalog()
	p, _ := NewPublisherV1(&localStub{values: []LocalReadinessProjectionV1{local, drift}}, &productionStub{}, store, func() time.Time { return fixed })
	if _, e := p.Publish(context.Background(), request(5)); !core.HasCategory(e, core.ErrorConflict) {
		t.Fatalf("S1/S2 drift accepted: %v", e)
	}
	unavailable := core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "readiness unavailable")
	p, _ = NewPublisherV1(&localStub{err: unavailable}, &productionStub{}, newCatalog(), func() time.Time { return fixed })
	if _, e := p.Publish(context.Background(), request(6)); !core.HasCategory(e, core.ErrorUnavailable) {
		t.Fatalf("unavailable downgraded: %v", e)
	}
	alias := localProjection(t, 7)
	alias.OfficialSDKCallRef = alias.MCPLifecycleRef
	if _, e := SealLocalReadinessV1(alias); !core.HasCategory(e, core.ErrorConflict) {
		t.Fatalf("aliased proof accepted: %v", e)
	}
	expired := localProjection(t, 8)
	expired.ExpiresUnixNano = fixed.Add(time.Nanosecond).UnixNano()
	expired, _ = SealLocalReadinessV1(expired)
	clock := &sequenceClock{values: []time.Time{fixed, fixed.Add(2 * time.Nanosecond)}}
	p, _ = NewPublisherV1(&localStub{values: []LocalReadinessProjectionV1{expired}}, &productionStub{}, newCatalog(), clock.Now)
	if _, e := p.Publish(context.Background(), request(8)); !core.HasCategory(e, core.ErrorPreconditionFailed) {
		t.Fatalf("TTL crossing accepted: %v", e)
	}
}
func TestLostReplyAndSixtyFourPublishersLinearizeOnce(t *testing.T) {
	store := newCatalog()
	store.lost = true
	p, _ := NewPublisherV1(&localStub{}, &productionStub{}, store, func() time.Time { return fixed })
	if _, e := p.Publish(context.Background(), request(9)); e != nil {
		t.Fatal(e)
	}
	if store.commits != 1 {
		t.Fatalf("lost reply commits=%d", store.commits)
	}
	store = newCatalog()
	var failed atomic.Int64
	start := make(chan struct{})
	var group sync.WaitGroup
	for range 64 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			p, _ := NewPublisherV1(&localStub{}, &productionStub{}, store, func() time.Time { return fixed })
			if _, e := p.Publish(context.Background(), request(10)); e != nil {
				failed.Add(1)
			}
		}()
	}
	close(start)
	group.Wait()
	if failed.Load() != 0 || store.commits != 1 {
		t.Fatalf("failures=%d commits=%d", failed.Load(), store.commits)
	}
}
func TestTypedNilAndChangedContentFailClosed(t *testing.T) {
	var l *localStub
	if _, e := NewPublisherV1(l, &productionStub{}, newCatalog(), func() time.Time { return fixed }); e == nil {
		t.Fatal("typed nil accepted")
	}
	var production *productionStub
	if _, e := NewPublisherV1(&localStub{}, production, newCatalog(), func() time.Time { return fixed }); e == nil {
		t.Fatal("typed-nil production reader accepted")
	}
	var storeNil *catalog
	if _, e := NewPublisherV1(&localStub{}, &productionStub{}, storeNil, func() time.Time { return fixed }); e == nil {
		t.Fatal("typed-nil catalog accepted")
	}
	store := newCatalog()
	p, _ := NewPublisherV1(&localStub{}, &productionStub{}, store, func() time.Time { return fixed })
	if _, e := p.Publish(context.Background(), request(11)); e != nil {
		t.Fatal(e)
	}
	changed := request(11)
	changed.ArtifactDigest = core.DigestBytes([]byte("changed"))
	if _, e := p.Publish(context.Background(), changed); !core.HasCategory(e, core.ErrorConflict) {
		t.Fatalf("changed content accepted: %v", e)
	}
}

func TestClockRollbackFailsBeforeCatalogWrite(t *testing.T) {
	local := localProjection(t, 12)
	store := newCatalog()
	clock := &sequenceClock{values: []time.Time{fixed, fixed.Add(-time.Nanosecond)}}
	publisher, err := NewPublisherV1(&localStub{values: []LocalReadinessProjectionV1{local}}, &productionStub{}, store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = publisher.Publish(context.Background(), request(12)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback was accepted: %v", err)
	}
	if store.commits != 0 {
		t.Fatal("clock rollback reached the catalog")
	}
}

type sequenceClock struct {
	mu     sync.Mutex
	values []time.Time
	i      int
}

func (c *sequenceClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.i >= len(c.values) {
		return c.values[len(c.values)-1]
	}
	v := c.values[c.i]
	c.i++
	return v
}
