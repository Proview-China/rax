package lifecycle

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/journal"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestHostV3SQLiteMultiHandleWithSingleProcessReferencePipelineRestartLostReplyAndStop(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := newHostV3TestClock(time.Unix(1_990_000_000, 0))
	testDir := t.TempDir()
	dbPath := filepath.Join(testDir, "host-v3.db")
	projectionPath := filepath.Join(testDir, "owner-projections.db")
	owner := core.OwnerRef{Domain: "praxis.agent-host", ID: "host-v3-reference"}
	deployment := hostV3DeploymentFixture(t, clock.Peek())
	pipelineStore := openHostV3Store(t, ctx, dbPath, owner, clock)
	pipeline := newReferenceHostV3Pipeline(t, clock, owner, pipelineStore, projectionPath)
	const hostHandles = 8
	stores := make([]*sqlite.Store, hostHandles)
	hosts := make([]*HostV3, hostHandles)
	for index := range hostHandles {
		stores[index] = openHostV3Store(t, ctx, dbPath, owner, clock)
		hosts[index] = newSQLiteHostV3(t, stores[index], deployment, pipeline, clock)
	}
	request := hostV3StartRequestFixture(t, deployment, clock.Peek(), "start-concurrent")
	pipeline.loseStartReply.Store(true)

	const workers = 32
	results := make([]contract.StartResultV3, workers)
	errs := make([]error, workers)
	var wait sync.WaitGroup
	for index := range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index], errs[index] = hosts[index%len(hosts)].StartV3(ctx, request)
		}()
	}
	wait.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("start[%d]: %v", index, err)
		}
		if results[index].ResultDigest != results[0].ResultDigest {
			t.Fatalf("start[%d] returned another result", index)
		}
	}
	if calls := pipeline.startCalls.Load(); calls != 1 {
		t.Fatalf("owner pipeline Start calls=%d, want 1", calls)
	}
	if results[0].Ready.ID != results[0].Availability.ID || results[0].Ready.Revision != results[0].Availability.Revision || results[0].Ready.Epoch != results[0].Availability.Epoch || results[0].Ready.ExpiresUnixNano != results[0].Availability.ExpiresUnixNano {
		t.Fatalf("Ready/Availability coordinates drifted: ready=%+v availability=%+v", results[0].Ready, results[0].Availability)
	}
	claim, err := stores[0].InspectHostStartClaimCurrentV1(ctx, results[0].StartClaim)
	if err != nil {
		t.Fatal(err)
	}
	versionCoordinator, err := journal.NewCoordinatorV2(stores[0], clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = versionCoordinator.EnsureAcceptedV2(ctx, claim); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("V3 claim crossed into V2 Journal acceptance: %v", err)
	}

	inspectRequest := hostV3InspectRequestFixture(t, results[0].StartClaim, clock.Peek())
	inspected, err := hosts[1].InspectV3(ctx, inspectRequest)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Phase != contract.HostInspectReadyV3 || !inspected.HasReady || !inspected.HasAvailability || !inspected.HasCleanupClosure {
		t.Fatalf("inspect=%+v", inspected)
	}

	for _, store := range stores {
		if err = store.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err = pipeline.Close(); err != nil {
		t.Fatal(err)
	}
	if err = pipelineStore.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openHostV3Store(t, ctx, dbPath, owner, clock)
	defer reopened.Close()
	reopenedPipelineStore := openHostV3Store(t, ctx, dbPath, owner, clock)
	defer reopenedPipelineStore.Close()
	pipeline = newReferenceHostV3Pipeline(t, clock, owner, reopenedPipelineStore, projectionPath)
	defer pipeline.Close()
	host := newSQLiteHostV3(t, reopened, deployment, pipeline, clock)
	restarted, err := host.InspectV3(ctx, hostV3InspectRequestFixture(t, results[0].StartClaim, clock.Peek()))
	if err != nil || restarted.Phase != contract.HostInspectReadyV3 {
		t.Fatalf("restart inspect=%+v err=%v", restarted, err)
	}

	stop := hostV3StopRequestFixture(t, results[0], clock.Peek())
	pipeline.loseStopReply.Store(true)
	stopped, err := host.StopV3(ctx, stop)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.State != contract.CleanupClosedV1 || stopped.CleanupClosure != results[0].CleanupClosure {
		t.Fatalf("stop=%+v", stopped)
	}
	if calls := pipeline.stopCalls.Load(); calls != 1 {
		t.Fatalf("owner pipeline Stop calls=%d, want 1", calls)
	}
	replayed, err := host.StopV3(ctx, stop)
	if err != nil || replayed.ResultDigest != stopped.ResultDigest || pipeline.stopCalls.Load() != 1 {
		t.Fatalf("stop replay=%+v err=%v calls=%d", replayed, err, pipeline.stopCalls.Load())
	}
}

func TestHostV3FailsClosedOnSpliceExpiryAndUnknown(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := newHostV3TestClock(time.Unix(1_990_100_000, 0))
	owner := core.OwnerRef{Domain: "praxis.agent-host", ID: "host-v3-reference"}
	deployment := hostV3DeploymentFixture(t, clock.Peek())
	store := openHostV3Store(t, ctx, filepath.Join(t.TempDir(), "host-v3.db"), owner, clock)
	defer store.Close()
	pipeline := newReferenceHostV3Pipeline(t, clock, owner, store, filepath.Join(t.TempDir(), "owner-projections.db"))
	defer pipeline.Close()
	host := newSQLiteHostV3(t, store, deployment, pipeline, clock)
	start := hostV3StartRequestFixture(t, deployment, clock.Peek(), "start-splice")
	result, err := host.StartV3(ctx, start)
	if err != nil {
		t.Fatal(err)
	}

	splicedClaim := result.StartClaim
	splicedClaim.Digest = hostV3Digest(t, "spliced-claim")
	if _, err = host.InspectV3(ctx, hostV3InspectRequestFixture(t, splicedClaim, clock.Peek())); !contract.HasCode(err, contract.ErrorConflict) && !contract.HasCode(err, contract.ErrorNotFound) {
		t.Fatalf("claim splice err=%v", err)
	}
	splicedStop := hostV3StopRequestFixture(t, result, clock.Peek())
	splicedStop.CleanupClosure.Digest = hostV3Digest(t, "spliced-closure")
	splicedStop.RequestDigest = ""
	splicedStop, err = contract.SealStopRequestV3(splicedStop)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = host.StopV3(ctx, splicedStop); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("closure splice err=%v", err)
	}

	expired := hostV3StartRequestFixture(t, deployment, clock.Peek(), "start-expired")
	clock.Set(time.Unix(0, expired.RequestedNotAfterUnixNano))
	if _, err = host.StartV3(ctx, expired); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("expired start err=%v", err)
	}

	clock.Set(time.Unix(1_990_100_100, 0))
	unknownPipeline := newReferenceHostV3Pipeline(t, clock, owner, store, filepath.Join(t.TempDir(), "unknown-owner-projections.db"))
	defer unknownPipeline.Close()
	unknownPipeline.unknownStart.Store(true)
	unknownHost := newSQLiteHostV3(t, store, deployment, unknownPipeline, clock)
	unknown := hostV3StartRequestFixture(t, deployment, clock.Peek(), "start-unknown")
	if _, err = unknownHost.StartV3(ctx, unknown); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("unknown Start err=%v", err)
	}
	if unknownPipeline.inspectCalls.Load() == 0 {
		t.Fatal("unknown Start did not Inspect the original operation")
	}
}

type hostV3TestClock struct{ nanos atomic.Int64 }

func newHostV3TestClock(value time.Time) *hostV3TestClock {
	clock := &hostV3TestClock{}
	clock.nanos.Store(value.UnixNano())
	return clock
}
func (c *hostV3TestClock) Now() time.Time      { return time.Unix(0, c.nanos.Add(int64(time.Millisecond))) }
func (c *hostV3TestClock) Peek() time.Time     { return time.Unix(0, c.nanos.Load()) }
func (c *hostV3TestClock) Set(value time.Time) { c.nanos.Store(value.UnixNano()) }

type hostV3DeploymentReader struct {
	value contract.HostDeploymentCurrentV1
}

// referenceHostV3ProjectionStore is test-only durable evidence for the narrow
// Owner pipeline. It proves restart recovery without claiming that the real
// Ready/Cleanup Owners are already composed for production.
type referenceHostV3ProjectionStore struct{ db *sql.DB }

func openReferenceHostV3ProjectionStore(path string) (*referenceHostV3ProjectionStore, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dsn := (&url.URL{Scheme: "file", Path: abs}).String() + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS host_v3_reference_projection(kind TEXT NOT NULL, subject_key TEXT NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(kind,subject_key))`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &referenceHostV3ProjectionStore{db: db}, nil
}
func (s *referenceHostV3ProjectionStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
func (s *referenceHostV3ProjectionStore) store(ctx context.Context, kind, key string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO host_v3_reference_projection(kind,subject_key,payload) VALUES(?,?,?)`, kind, key, payload); err != nil {
		return contract.NewError(contract.ErrorUnknownOutcome, "reference_projection_write_unknown", "reference Owner projection write outcome is unknown")
	}
	var actual []byte
	if err = s.db.QueryRowContext(ctx, `SELECT payload FROM host_v3_reference_projection WHERE kind=? AND subject_key=?`, kind, key).Scan(&actual); err != nil {
		return contract.NewError(contract.ErrorUnavailable, "reference_projection_read_failed", "reference Owner projection cannot be read")
	}
	if string(actual) != string(payload) {
		return contract.NewError(contract.ErrorConflict, "reference_projection_splice", "reference Owner projection already binds another exact result")
	}
	return nil
}
func (s *referenceHostV3ProjectionStore) load(ctx context.Context, kind, key string, target any) (bool, error) {
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM host_v3_reference_projection WHERE kind=? AND subject_key=?`, kind, key).Scan(&payload)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, contract.NewError(contract.ErrorUnavailable, "reference_projection_read_failed", "reference Owner projection cannot be read")
	}
	if err = json.Unmarshal(payload, target); err != nil {
		return false, contract.NewError(contract.ErrorConflict, "reference_projection_corrupt", "reference Owner projection is corrupt")
	}
	return true, nil
}
func (s *referenceHostV3ProjectionStore) StoreStart(ctx context.Context, key string, value contract.HostV3OwnerStartProjectionV1) error {
	return s.store(ctx, "start", key, value)
}
func (s *referenceHostV3ProjectionStore) LoadStart(ctx context.Context, key string) (contract.HostV3OwnerStartProjectionV1, bool, error) {
	var value contract.HostV3OwnerStartProjectionV1
	ok, err := s.load(ctx, "start", key, &value)
	if err == nil && ok {
		err = value.Validate()
	}
	return value, ok, err
}
func (s *referenceHostV3ProjectionStore) StoreStop(ctx context.Context, key string, value contract.HostV3OwnerStopProjectionV1) error {
	return s.store(ctx, "stop", key, value)
}
func (s *referenceHostV3ProjectionStore) LoadStop(ctx context.Context, key string) (contract.HostV3OwnerStopProjectionV1, bool, error) {
	var value contract.HostV3OwnerStopProjectionV1
	ok, err := s.load(ctx, "stop", key, &value)
	if err == nil && ok {
		err = value.Validate()
	}
	return value, ok, err
}

func (r hostV3DeploymentReader) InspectHostDeploymentCurrentV1(_ context.Context, expected contract.HostDeploymentCurrentRefV1) (contract.HostDeploymentCurrentV1, error) {
	if r.value.Ref != expected {
		return contract.HostDeploymentCurrentV1{}, contract.NewError(contract.ErrorConflict, "deployment_splice", "another deployment current was requested")
	}
	return r.value, nil
}

type referenceHostV3Pipeline struct {
	mu             sync.Mutex
	clock          *hostV3TestClock
	owner          core.OwnerRef
	coordinator    *journal.CoordinatorV2
	projections    *referenceHostV3ProjectionStore
	startCalls     atomic.Int64
	stopCalls      atomic.Int64
	inspectCalls   atomic.Int64
	loseStartReply atomic.Bool
	loseStopReply  atomic.Bool
	unknownStart   atomic.Bool
}

func newReferenceHostV3Pipeline(t *testing.T, clock *hostV3TestClock, owner core.OwnerRef, journalStore *sqlite.Store, projectionPath string) *referenceHostV3Pipeline {
	t.Helper()
	coordinator, err := journal.NewCoordinatorV2(journalStore, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	projections, err := openReferenceHostV3ProjectionStore(projectionPath)
	if err != nil {
		t.Fatal(err)
	}
	return &referenceHostV3Pipeline{clock: clock, owner: owner, coordinator: coordinator, projections: projections}
}
func (p *referenceHostV3Pipeline) Close() error { return p.projections.Close() }
func hostV3Key(hostID, startID string) string   { return hostID + "\x00" + startID }

func (p *referenceHostV3Pipeline) StartOrInspectHostV3(ctx context.Context, request contract.StartRequestV3, binding contract.HostStartClaimInputBindingV3, current contract.HostJournalV2) (contract.HostV3OwnerStartProjectionV1, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := hostV3Key(binding.Input.HostID, binding.Input.StartID)
	projection, ok, err := p.projections.LoadStart(ctx, key)
	if err != nil {
		return contract.HostV3OwnerStartProjectionV1{}, err
	}
	if ok {
		latest, err := p.coordinator.InspectV2(ctx, binding.Input.HostID, binding.Input.StartID)
		if err != nil {
			return contract.HostV3OwnerStartProjectionV1{}, err
		}
		return p.refreshStartProjection(projection, latest)
	}
	if p.unknownStart.Load() {
		return contract.HostV3OwnerStartProjectionV1{}, contract.NewError(contract.ErrorUnknownOutcome, "reference_start_unknown", "reference owner outcome is deliberately hidden")
	}
	p.startCalls.Add(1)
	readyJournal, err := p.advance(ctx, current, contract.HostReadyV2)
	if err != nil {
		return contract.HostV3OwnerStartProjectionV1{}, err
	}
	projection, err = p.newStartProjection(request.RequestDigest, binding, readyJournal)
	if err != nil {
		return contract.HostV3OwnerStartProjectionV1{}, err
	}
	if err = p.projections.StoreStart(ctx, key, projection); err != nil {
		return contract.HostV3OwnerStartProjectionV1{}, err
	}
	if p.loseStartReply.Swap(false) {
		return contract.HostV3OwnerStartProjectionV1{}, contract.NewError(contract.ErrorUnknownOutcome, "reference_start_reply_lost", "reference start committed but reply was lost")
	}
	return projection, nil
}

func (p *referenceHostV3Pipeline) InspectHostV3(ctx context.Context, binding contract.HostStartClaimInputBindingV3, _ contract.HostJournalV2) (contract.HostV3OwnerStartProjectionV1, error) {
	p.inspectCalls.Add(1)
	p.mu.Lock()
	defer p.mu.Unlock()
	projection, ok, err := p.projections.LoadStart(ctx, hostV3Key(binding.Input.HostID, binding.Input.StartID))
	if err != nil {
		return contract.HostV3OwnerStartProjectionV1{}, err
	}
	if !ok {
		return contract.HostV3OwnerStartProjectionV1{}, contract.NewError(contract.ErrorNotFound, "reference_start_missing", "reference start projection is not visible")
	}
	latest, err := p.coordinator.InspectV2(ctx, binding.Input.HostID, binding.Input.StartID)
	if err != nil {
		return contract.HostV3OwnerStartProjectionV1{}, err
	}
	return p.refreshStartProjection(projection, latest)
}

func (p *referenceHostV3Pipeline) StopOrInspectHostV3(ctx context.Context, request contract.StopRequestV3, binding contract.HostStartClaimInputBindingV3, current contract.HostJournalV2) (contract.HostV3OwnerStopProjectionV1, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := hostV3Key(binding.Input.HostID, binding.Input.StartID)
	projection, ok, err := p.projections.LoadStop(ctx, key)
	if err != nil {
		return contract.HostV3OwnerStopProjectionV1{}, err
	}
	if ok {
		return projection, nil
	}
	p.stopCalls.Add(1)
	closed, err := p.advance(ctx, current, contract.HostClosedV2)
	if err != nil {
		return contract.HostV3OwnerStopProjectionV1{}, err
	}
	journalRef, _ := closed.RefV2()
	resultRef := contract.ExactRefV1{Kind: "praxis.agent-host/cleanup-result-v3", ID: "cleanup-result-" + request.StartID, Revision: 1, Digest: hostV3DigestNoTest("cleanup-result-" + string(request.RequestDigest))}
	projection, err = contract.SealHostV3OwnerStopProjectionV1(contract.HostV3OwnerStopProjectionV1{RequestDigest: request.RequestDigest, Journal: journalRef, CleanupClosure: request.CleanupClosure, CleanupResult: resultRef, State: contract.CleanupClosedV1, CheckedUnixNano: p.clock.Now().UnixNano()})
	if err != nil {
		return contract.HostV3OwnerStopProjectionV1{}, err
	}
	if err = p.projections.StoreStop(ctx, key, projection); err != nil {
		return contract.HostV3OwnerStopProjectionV1{}, err
	}
	if p.loseStopReply.Swap(false) {
		return contract.HostV3OwnerStopProjectionV1{}, contract.NewError(contract.ErrorUnknownOutcome, "reference_stop_reply_lost", "reference stop committed but reply was lost")
	}
	return projection, nil
}

func (p *referenceHostV3Pipeline) InspectStopHostV3(ctx context.Context, request contract.StopRequestV3, binding contract.HostStartClaimInputBindingV3, _ contract.HostJournalV2) (contract.HostV3OwnerStopProjectionV1, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	projection, ok, err := p.projections.LoadStop(ctx, hostV3Key(binding.Input.HostID, binding.Input.StartID))
	if err != nil {
		return contract.HostV3OwnerStopProjectionV1{}, err
	}
	if !ok || projection.RequestDigest != request.RequestDigest || projection.CleanupClosure != request.CleanupClosure {
		return contract.HostV3OwnerStopProjectionV1{}, contract.NewError(contract.ErrorNotFound, "reference_stop_missing", "reference stop projection is not visible")
	}
	return projection, nil
}

func (p *referenceHostV3Pipeline) newStartProjection(requestDigest contract.DigestV1, binding contract.HostStartClaimInputBindingV3, current contract.HostJournalV2) (contract.HostV3OwnerStartProjectionV1, error) {
	checked := p.clock.Now()
	expires := checked.Add(30 * time.Minute).UnixNano()
	if binding.ClaimRef.ExpiresUnixNano < expires {
		expires = binding.ClaimRef.ExpiresUnixNano
	}
	factRef := contract.SystemReadyFactRefV2{ID: "ready-fact-" + binding.Input.StartID, Revision: 1, Digest: core.DigestBytes([]byte("ready-fact-" + binding.Input.StartID)), ExpiresUnixNano: expires}
	ready, err := contract.SealSystemReadyCurrentV2(contract.SystemReadyCurrentV2{Ref: contract.SystemReadyCurrentRefV2{ID: contract.DeriveSystemReadyCurrentIDV2(binding.Input.HostID, binding.Input.StartID), Revision: 1, Epoch: 1}, FactRef: factRef, HostID: binding.Input.HostID, StartID: binding.Input.StartID, State: contract.SystemReadyCurrentReadyV2, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		return contract.HostV3OwnerStartProjectionV1{}, err
	}
	availability, err := ready.ToAgentExecutionAvailabilityV1(p.owner)
	if err != nil {
		return contract.HostV3OwnerStartProjectionV1{}, err
	}
	journalRef, _ := current.RefV2()
	closure := contract.ExactRefV1{Kind: contract.HostCleanupClosureRefKindV2, ID: "cleanup-closure-" + binding.Input.StartID, Revision: 1, Digest: hostV3DigestNoTest("cleanup-closure-" + binding.Input.StartID)}
	return contract.SealHostV3OwnerStartProjectionV1(contract.HostV3OwnerStartProjectionV1{HostID: binding.Input.HostID, StartID: binding.Input.StartID, RequestDigest: requestDigest, Journal: journalRef, CleanupClosure: closure, Ready: ready.Ref, Availability: availability.Ref, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires})
}
func (p *referenceHostV3Pipeline) refreshStartProjection(value contract.HostV3OwnerStartProjectionV1, current contract.HostJournalV2) (contract.HostV3OwnerStartProjectionV1, error) {
	ref, err := current.RefV2()
	if err != nil {
		return contract.HostV3OwnerStartProjectionV1{}, err
	}
	value.Journal, value.ProjectionDigest = ref, ""
	return contract.SealHostV3OwnerStartProjectionV1(value)
}
func (p *referenceHostV3Pipeline) advance(ctx context.Context, current contract.HostJournalV2, target contract.HostPhaseV2) (contract.HostJournalV2, error) {
	sequence := []contract.HostPhaseV2{contract.HostAcceptedV2, contract.HostValidatingV2, contract.HostResolvingV2, contract.HostCompilingV2, contract.HostBindingV2, contract.HostConstructingControlV2, contract.HostActivatingV2, contract.HostAssociatingGenerationV2, contract.HostVerifyingV2, contract.HostReadyV2, contract.HostDrainingV2, contract.HostReconcilingV2, contract.HostClosedV2}
	index, targetIndex := -1, -1
	for i, phase := range sequence {
		if phase == current.Phase {
			index = i
		}
		if phase == target {
			targetIndex = i
		}
	}
	if index < 0 || targetIndex < index {
		return current, contract.NewError(contract.ErrorConflict, "reference_phase_drift", "reference pipeline cannot reverse lifecycle phase")
	}
	for index < targetIndex {
		index++
		next := current
		next.Revision++
		next.Phase = sequence[index]
		next.UpdatedUnixNano = p.clock.Now().UnixNano()
		next.Digest = ""
		var err error
		next, err = contract.SealHostJournalV2(next)
		if err != nil {
			return contract.HostJournalV2{}, err
		}
		current, err = p.coordinator.AdvanceV2(ctx, current, next)
		if err != nil {
			return contract.HostJournalV2{}, err
		}
	}
	return current, nil
}

func openHostV3Store(t *testing.T, ctx context.Context, path string, owner core.OwnerRef, clock *hostV3TestClock) *sqlite.Store {
	t.Helper()
	store, err := sqlite.Open(ctx, sqlite.Config{Path: path, Owner: owner, Clock: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	return store
}
func newSQLiteHostV3(t *testing.T, store *sqlite.Store, deployment contract.HostDeploymentCurrentV1, pipeline *referenceHostV3Pipeline, clock *hostV3TestClock) *HostV3 {
	t.Helper()
	coordinator, err := journal.NewCoordinatorV2(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	host, err := NewHostV3(ConfigV3{Claims: store, Journal: coordinator, Deployment: hostV3DeploymentReader{value: deployment}, Pipeline: pipeline, Clock: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	return host
}
func hostV3DeploymentFixture(t *testing.T, now time.Time) contract.HostDeploymentCurrentV1 {
	t.Helper()
	expires := now.Add(4 * time.Hour).UnixNano()
	owner := core.OwnerRef{Domain: "praxis.deployment", ID: "host-v3-owner"}
	handle := runtimeports.ResourceHandleRefV1{Owner: owner, ID: "state-runtime", Revision: 1, Digest: core.DigestBytes([]byte("state-runtime")), Kind: "praxis/sqlite", ScopeDigest: core.DigestBytes([]byte("state-scope")), ExpiresUnixNano: expires}
	roles := []contract.HostServiceBindingRoleV1{contract.HostServiceDefinitionSourceV1, contract.HostServiceCatalogV1, contract.HostServiceResolutionFactsV1, contract.HostServiceSecretBrokerV1, contract.HostServiceCredentialRegistryV1, contract.HostServiceProviderRegistryV1, contract.HostServiceRuntimeV1, contract.HostServiceApplicationV1, contract.HostServiceHarnessV1, contract.HostServiceListenV1, contract.HostServiceDiagnosticsV1, contract.HostServiceShutdownV1}
	services := make([]contract.HostServiceBindingRefV1, 0, len(roles))
	for _, role := range roles {
		id := string(role)
		services = append(services, contract.HostServiceBindingRefV1{Role: role, ConfiguredID: id, BindingRef: contract.ExactRefV1{Kind: "praxis.agent-host/fixture", ID: id, Revision: 1, Digest: hostV3Digest(t, id)}, Capability: "praxis.host/" + id, ExpiresUnixNano: expires})
	}
	value, err := contract.SealHostDeploymentCurrentV1(contract.HostDeploymentCurrentV1{Ref: contract.HostDeploymentCurrentRefV1{HostID: "host-v3", DeploymentID: "deployment-v3", Revision: 1, BootstrapDigest: hostV3Digest(t, "bootstrap"), ExpiresUnixNano: expires}, ResourceHandles: []runtimeports.ResourceHandleRefV1{handle}, ServiceBindings: services, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func hostV3StartRequestFixture(t *testing.T, deployment contract.HostDeploymentCurrentV1, now time.Time, startID string) contract.StartRequestV3 {
	t.Helper()
	value, err := contract.SealStartRequestV3(contract.StartRequestV3{StartID: startID, DeploymentCurrentRef: deployment.Ref, Config: contract.HostConfigV1{ContractVersion: contract.ContractVersionV1, HostID: deployment.Ref.HostID, DefinitionSourceRef: "definition-source", StatePlaneBindings: []string{"state-runtime"}, ProviderEndpointRefs: []string{"provider"}, SecretBrokerRef: "secret", CatalogRef: "catalog", ResolutionFactsRef: "resolution", RuntimeServiceRefs: []string{"runtime"}, ListenRef: "listen", DiagnosticsPolicyRef: "diagnostics"}, DefinitionSourceCurrent: contract.ExactRefV1{Kind: "praxis.agent-definition/source", ID: "definition-source", Revision: 1, Digest: hostV3Digest(t, "definition-source")}, RequestedAtUnixNano: now.Add(-time.Second).UnixNano(), RequestedNotAfterUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func hostV3InspectRequestFixture(t *testing.T, claim contract.HostStartClaimRefV1, now time.Time) contract.InspectRequestV3 {
	t.Helper()
	value, err := contract.SealInspectRequestV3(contract.InspectRequestV3{HostID: claim.HostID, StartID: claim.StartID, StartClaim: claim, RequestedAtUnixNano: now.Add(-time.Second).UnixNano(), RequestedNotAfterUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func hostV3StopRequestFixture(t *testing.T, start contract.StartResultV3, now time.Time) contract.StopRequestV3 {
	t.Helper()
	value, err := contract.SealStopRequestV3(contract.StopRequestV3{HostID: start.HostID, StartID: start.StartID, StartClaim: start.StartClaim, CleanupClosure: start.CleanupClosure, RequestedAtUnixNano: now.Add(-time.Second).UnixNano(), RequestedNotAfterUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func hostV3Digest(t *testing.T, value string) contract.DigestV1 {
	t.Helper()
	return hostV3DigestNoTest(value)
}
func hostV3DigestNoTest(value string) contract.DigestV1 {
	digest, _ := contract.DigestJSONV1(value)
	return digest
}
