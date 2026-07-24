package api_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/api"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/storage/sqlite"
)

func TestServiceV1SubmitReplayExecuteWatchAndClone(t *testing.T) {
	fixture := newAPIFixture(t)
	request := apiRequest(t, fixture.now, "request-1", "key-1", `{"backend":"wasm"}`)
	queued, err := fixture.service.Submit(context.Background(), request)
	if err != nil || queued.State != api.OperationQueuedV1 || queued.Revision != 1 {
		t.Fatalf("submit=%#v err=%v", queued, err)
	}
	replay, err := fixture.service.Submit(context.Background(), request)
	if err != nil || replay.Ref() != queued.Ref() {
		t.Fatalf("replay=%#v err=%v", replay, err)
	}
	completed, err := fixture.service.Execute(context.Background(), request.RequestID)
	if err != nil || completed.State != api.OperationSucceededV1 || completed.Revision != 3 || fixture.handler.executeCalls.Load() != 1 {
		t.Fatalf("completed=%#v err=%v execute_calls=%d", completed, err, fixture.handler.executeCalls.Load())
	}
	items, cursor, err := fixture.service.Watch(context.Background(), 0, 16)
	if err != nil || len(items) != 3 || cursor < 3 || items[0].State != api.OperationQueuedV1 || items[1].State != api.OperationRunningV1 || items[2].State != api.OperationSucceededV1 {
		t.Fatalf("watch=%#v cursor=%d err=%v", items, cursor, err)
	}
	none, sameCursor, err := fixture.service.Watch(context.Background(), cursor, 16)
	if err != nil || len(none) != 0 || sameCursor != cursor {
		t.Fatalf("watch tail=%#v cursor=%d err=%v", none, sameCursor, err)
	}
	completed.Request.Payload[0] = '['
	completed.Result.Payload[0] = '['
	inspected, err := fixture.service.Inspect(context.Background(), request.RequestID)
	if err != nil || string(inspected.Request.Payload) != `{"backend":"wasm"}` || string(inspected.Result.Payload) != `{"accepted":true}` {
		t.Fatalf("store aliased caller memory: %#v err=%v", inspected, err)
	}
}

func TestServiceV1IdempotencyConflictAndLostCreateReplyInspect(t *testing.T) {
	fixture := newAPIFixture(t)
	first := apiRequest(t, fixture.now, "request-1", "stable-key", `{"backend":"container"}`)
	if _, err := fixture.service.Submit(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	changed := apiRequest(t, fixture.now, "request-2", "stable-key", `{"backend":"wasm"}`)
	if _, err := fixture.service.Submit(context.Background(), changed); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("changed idempotent request accepted: %v", err)
	}

	lost := &lostCreateStore{OperationStoreV1: fixture.store, lose: true}
	service, err := api.NewServiceV1(lost, fixture.handler, func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}
	request := apiRequest(t, fixture.now, "request-lost", "lost-key", `{"backend":"host"}`)
	if _, err := service.Submit(context.Background(), request); err == nil {
		t.Fatal("lost create reply was reported as success")
	}
	recovered, err := service.InspectByIdempotency(context.Background(), request.TenantID, request.IdempotencyKey)
	if err != nil || recovered.Request.Digest != request.Digest || recovered.State != api.OperationQueuedV1 {
		t.Fatalf("lost create Inspect=%#v err=%v", recovered, err)
	}
}

func TestServiceV1ConcurrentExecuteSingleWinner(t *testing.T) {
	fixture := newAPIFixture(t)
	request := apiRequest(t, fixture.now, "request-race", "key-race", `{"backend":"microvm"}`)
	if _, err := fixture.service.Submit(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int64
	var conflicts atomic.Int64
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fact, err := fixture.service.Execute(context.Background(), request.RequestID)
			if err == nil && fact.State == api.OperationSucceededV1 {
				successes.Add(1)
				return
			}
			if errors.Is(err, api.ErrConflict) {
				conflicts.Add(1)
				return
			}
			t.Errorf("unexpected concurrent Execute result: fact=%#v err=%v", fact, err)
		}()
	}
	wg.Wait()
	if successes.Load() != 1 || conflicts.Load() != 63 || fixture.handler.executeCalls.Load() != 1 {
		t.Fatalf("successes=%d conflicts=%d handler=%d", successes.Load(), conflicts.Load(), fixture.handler.executeCalls.Load())
	}
}

func TestServiceV1CancellationNeverClaimsRunningEffectStopped(t *testing.T) {
	fixture := newAPIFixture(t)
	queuedRequest := apiRequest(t, fixture.now, "queued-cancel", "queued-cancel", `{"kind":"release"}`)
	if _, err := fixture.service.Submit(context.Background(), queuedRequest); err != nil {
		t.Fatal(err)
	}
	cancelled, err := fixture.service.Cancel(context.Background(), queuedRequest.RequestID)
	if err != nil || cancelled.State != api.OperationCancelledV1 || fixture.handler.executeCalls.Load() != 0 {
		t.Fatalf("queued cancel=%#v err=%v calls=%d", cancelled, err, fixture.handler.executeCalls.Load())
	}

	runningRequest := apiRequest(t, fixture.now, "running-cancel", "running-cancel", `{"kind":"fence"}`)
	if _, err := fixture.service.Submit(context.Background(), runningRequest); err != nil {
		t.Fatal(err)
	}
	fixture.handler.started = make(chan struct{})
	fixture.handler.release = make(chan struct{})
	done := make(chan struct {
		fact api.OperationFactV1
		err  error
	}, 1)
	go func() {
		fact, err := fixture.service.Execute(context.Background(), runningRequest.RequestID)
		done <- struct {
			fact api.OperationFactV1
			err  error
		}{fact, err}
	}()
	<-fixture.handler.started
	marked, err := fixture.service.Cancel(context.Background(), runningRequest.RequestID)
	if err != nil || marked.State != api.OperationRunningV1 || !marked.CancellationRequested {
		t.Fatalf("running cancel=%#v err=%v", marked, err)
	}
	close(fixture.handler.release)
	result := <-done
	if result.err != nil || result.fact.State != api.OperationSucceededV1 || !result.fact.CancellationRequested {
		t.Fatalf("running completion=%#v err=%v", result.fact, result.err)
	}
}

func TestServiceV1UnknownOutcomeOnlyReconcilesOriginal(t *testing.T) {
	fixture := newAPIFixture(t)
	fixture.handler.executeErr = errors.New("lost execute reply")
	fixture.handler.reconcileOutcome = fixture.handler.outcome
	request := apiRequest(t, fixture.now, "request-lost-execute", "lost-execute", `{"kind":"open"}`)
	if _, err := fixture.service.Submit(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	fact, err := fixture.service.Execute(context.Background(), request.RequestID)
	if err != nil || fact.State != api.OperationSucceededV1 || fixture.handler.executeCalls.Load() != 1 || fixture.handler.reconcileCalls.Load() != 1 {
		t.Fatalf("reconciled=%#v err=%v execute=%d reconcile=%d", fact, err, fixture.handler.executeCalls.Load(), fixture.handler.reconcileCalls.Load())
	}

	unknownFixture := newAPIFixture(t)
	unknownFixture.handler.executeErr = errors.New("lost execute reply")
	unknownFixture.handler.reconcileErr = errors.New("inspect unavailable")
	unknown := apiRequest(t, unknownFixture.now, "request-unknown", "unknown", `{"kind":"activate"}`)
	if _, err := unknownFixture.service.Submit(context.Background(), unknown); err != nil {
		t.Fatal(err)
	}
	fact, err = unknownFixture.service.Execute(context.Background(), unknown.RequestID)
	if err != nil || fact.State != api.OperationIndeterminateV1 || unknownFixture.handler.executeCalls.Load() != 1 || unknownFixture.handler.reconcileCalls.Load() != 1 {
		t.Fatalf("unknown=%#v err=%v execute=%d reconcile=%d", fact, err, unknownFixture.handler.executeCalls.Load(), unknownFixture.handler.reconcileCalls.Load())
	}
}

func TestServiceV1ExpiryCanonicalJSONAndTypedNil(t *testing.T) {
	fixture := newAPIFixture(t)
	request := apiRequest(t, fixture.now, "request-expired", "expired", `{"a":1}`)
	fixture.now = time.Unix(0, request.RequestedNotAfterUnixNano)
	if _, err := fixture.service.Submit(context.Background(), request); !errors.Is(err, api.ErrStale) || fixture.handler.executeCalls.Load() != 0 {
		t.Fatalf("expired submit err=%v calls=%d", err, fixture.handler.executeCalls.Load())
	}
	bad := request
	bad.Payload = []byte(`{"a":1} {"b":2}`)
	if _, err := api.SealOperationRequestV1(bad); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
	duplicate := request
	duplicate.Payload = []byte(`{"a":1,"a":2}`)
	if _, err := api.SealOperationRequestV1(duplicate); err == nil {
		t.Fatal("duplicate JSON member was accepted")
	}
	var typedNilStore *lostCreateStore
	if _, err := api.NewServiceV1(typedNilStore, fixture.handler, time.Now); err == nil {
		t.Fatal("typed-nil store was accepted")
	}
	var typedNilHandler *fakeHandler
	if _, err := api.NewServiceV1(fixture.store, typedNilHandler, time.Now); err == nil {
		t.Fatal("typed-nil handler was accepted")
	}
}

type apiFixture struct {
	now     time.Time
	store   *sqlite.Store
	handler *fakeHandler
	service *api.ServiceV1
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	now := time.Unix(2_000_000_000, 0)
	store, err := sqlite.OpenWithClock(context.Background(), filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	result, err := api.SealResultV1(api.ResultV1{Schema: "praxis.sandbox.api/test-result/v1", Revision: 1, Payload: []byte(`{"accepted":true}`)})
	if err != nil {
		t.Fatal(err)
	}
	handler := &fakeHandler{outcome: api.HandlerOutcomeV1{State: api.OperationSucceededV1, Result: &result}}
	fixture := &apiFixture{now: now, store: store, handler: handler}
	service, err := api.NewServiceV1(store, handler, func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}
	fixture.service = service
	return fixture
}

func apiRequest(t *testing.T, now time.Time, id, key, payload string) api.OperationRequestV1 {
	t.Helper()
	request, err := api.SealOperationRequestV1(api.OperationRequestV1{
		RequestID: id, IdempotencyKey: key, TenantID: "tenant-1", Action: api.ActionLifecycleV1,
		PayloadSchema: "praxis.sandbox.api/test-request/v1", PayloadRevision: 1, Payload: []byte(payload),
		RequestedUnixNano: now.Add(-time.Second).UnixNano(), RequestedNotAfterUnixNano: now.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

type fakeHandler struct {
	executeCalls     atomic.Int64
	reconcileCalls   atomic.Int64
	started          chan struct{}
	release          chan struct{}
	startOnce        sync.Once
	outcome          api.HandlerOutcomeV1
	reconcileOutcome api.HandlerOutcomeV1
	executeErr       error
	reconcileErr     error
}

func (h *fakeHandler) Execute(context.Context, api.OperationRequestV1) (api.HandlerOutcomeV1, error) {
	h.executeCalls.Add(1)
	if h.started != nil {
		h.startOnce.Do(func() { close(h.started) })
	}
	if h.release != nil {
		<-h.release
	}
	return h.outcome, h.executeErr
}

func (h *fakeHandler) Reconcile(context.Context, api.OperationRequestV1) (api.HandlerOutcomeV1, error) {
	h.reconcileCalls.Add(1)
	if h.reconcileOutcome.State == "" {
		return h.outcome, h.reconcileErr
	}
	return h.reconcileOutcome, h.reconcileErr
}

type lostCreateStore struct {
	api.OperationStoreV1
	mu   sync.Mutex
	lose bool
}

func (s *lostCreateStore) CreateOnce(ctx context.Context, tenant, key string, fact api.OperationFactV1) (api.OperationFactV1, bool, error) {
	stored, created, err := s.OperationStoreV1.CreateOnce(ctx, tenant, key, fact)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil && created && s.lose {
		s.lose = false
		return api.OperationFactV1{}, false, errors.New("injected lost create reply")
	}
	return stored, created, err
}
