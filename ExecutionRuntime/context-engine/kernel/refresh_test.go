package kernel_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/refreshstore"
)

type applyBarrierStoreV1 struct {
	contextports.ContextTurnRefreshOwnerBackendV1
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

type inspectErrorOwnerBackendV1 struct {
	contextports.ContextTurnRefreshOwnerBackendV1
	err        error
	applyCalls atomic.Int64
}

func (b *inspectErrorOwnerBackendV1) InspectContextTurnRefreshV1(context.Context, contract.InspectContextTurnRefreshRequestV1) (contract.ContextTurnRefreshResultV1, error) {
	return contract.ContextTurnRefreshResultV1{}, b.err
}

func (b *inspectErrorOwnerBackendV1) ApplyContextTurnRefreshCurrentCASV1(ctx context.Context, commit contract.ContextTurnRefreshCommitV1) (contract.ContextTurnRefreshResultV1, error) {
	b.applyCalls.Add(1)
	return b.ContextTurnRefreshOwnerBackendV1.ApplyContextTurnRefreshCurrentCASV1(ctx, commit)
}

func (s *applyBarrierStoreV1) ApplyContextTurnRefreshCurrentCASV1(ctx context.Context, commit contract.ContextTurnRefreshCommitV1) (contract.ContextTurnRefreshResultV1, error) {
	s.once.Do(func() { close(s.entered) })
	select {
	case <-ctx.Done():
		return contract.ContextTurnRefreshResultV1{}, ctx.Err()
	case <-s.release:
	}
	return s.ContextTurnRefreshOwnerBackendV1.ApplyContextTurnRefreshCurrentCASV1(ctx, commit)
}

func TestContextTurnRefreshPendingThenAtomicApplyAndInspect(t *testing.T) {
	fixture := mustRefreshFixture(t)
	parent := fixture.Parent.Frame
	prepared, err := fixture.Service.RefreshContextTurnV1(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Status != contract.ContextTurnRefreshPendingV1 {
		t.Fatalf("status=%s", prepared.Status)
	}
	pending, err := fixture.Service.InspectContextTurnRefreshV1(context.Background(), contract.InspectContextTurnRefreshRequestV1{AttemptRef: prepared.AttemptRef})
	if err != nil {
		t.Fatal(err)
	}
	if pending.Current != nil || pending.ApplySettlementRef != nil || pending.Status != contract.ContextTurnRefreshPendingV1 {
		t.Fatalf("pending became visible: %#v", pending)
	}
	record, err := fixture.Store.LoadContextTurnRefreshPendingRecordV1(context.Background(), prepared.AttemptRef)
	if err != nil {
		t.Fatal(err)
	}
	if record.Frame.StablePrefix != parent.StablePrefix || !sameContent(record.Frame.SemiStable, parent.SemiStable) {
		t.Fatal("stable or semi-stable prefix was not exact-reused")
	}
	if record.Frame.DynamicTail == parent.DynamicTail {
		t.Fatal("dynamic tail did not change")
	}
	if record.Frame.ParentFrame == nil {
		t.Fatal("child frame lost exact parent")
	}
	if fixture.Parent.Frame != parent {
		t.Fatal("parent frame mutated")
	}
	childReader, err := kernel.NewParentFrameCurrentReaderV1(fixture.Store, fixture.Store, fixture.Store, fixture.Store, fixture.Store, fixture.Parent.Content, fixture.Clock.Now, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = childReader.InspectContextParentFrameCurrentV1(context.Background(), record.Binding.Source); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("pending child became current: %v", err)
	}
	apply := sealApply(t, fixture, prepared)
	result, err := fixture.Service.ApplyContextTurnRefreshV1(context.Background(), apply)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != contract.ContextTurnRefreshAppliedV1 || result.Current == nil || result.ApplySettlementRef == nil {
		t.Fatalf("not atomically applied: %#v", result)
	}
	childCurrent, err := childReader.InspectContextParentFrameCurrentV1(context.Background(), record.Binding.Source)
	if err != nil {
		t.Fatal(err)
	}
	if childCurrent.FrameRef != result.FrameRef || childCurrent.GenerationRef != result.GenerationRef {
		t.Fatal("applied exact child is not owner-current readable")
	}
	inspected, err := fixture.Service.InspectContextTurnRefreshV1(context.Background(), contract.InspectContextTurnRefreshRequestV1{AttemptRef: prepared.AttemptRef})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inspected, result) {
		t.Fatalf("inspect drift\n got: %#v\nwant: %#v", inspected, result)
	}
}

func TestContextTurnRefreshS2TTLAndClockFailuresKeepCurrentInvisible(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testfixture.RefreshFixtureV1, contract.ContextTurnRefreshPreparedV1)
		want   error
	}{
		{"ttl_crossing", func(f *testfixture.RefreshFixtureV1, p contract.ContextTurnRefreshPreparedV1) {
			f.Clock.Set(time.Unix(0, p.ExpiresUnixNano))
		}, contract.ErrExpired},
		{"clock_rollback", func(f *testfixture.RefreshFixtureV1, _ contract.ContextTurnRefreshPreparedV1) {
			f.Clock.Set(f.Now.Add(-time.Nanosecond))
		}, contract.ErrConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := mustRefreshFixture(t)
			prepared, err := fixture.Service.RefreshContextTurnV1(context.Background(), fixture.Request)
			if err != nil {
				t.Fatal(err)
			}
			apply := sealApply(t, fixture, prepared)
			tt.mutate(fixture, prepared)
			if _, err = fixture.Service.ApplyContextTurnRefreshV1(context.Background(), apply); !errors.Is(err, tt.want) {
				t.Fatalf("err=%v want %v", err, tt.want)
			}
			assertPending(t, fixture, prepared)
		})
	}
}

func TestContextTurnRefreshS2ParentAndToolDriftKeepCurrentInvisible(t *testing.T) {
	t.Run("parent_current_pointer", func(t *testing.T) {
		fixture := mustRefreshFixture(t)
		prepared, err := fixture.Service.RefreshContextTurnV1(context.Background(), fixture.Request)
		if err != nil {
			t.Fatal(err)
		}
		next := fixture.Parent.Pointer
		next.Revision++
		next, err = contract.SealContextGenerationCurrentPointerV1(next)
		if err != nil {
			t.Fatal(err)
		}
		if err = fixture.Store.CompareAndSwapGenerationCurrentV1(context.Background(), fixture.Parent.Pointer, next); err != nil {
			t.Fatal(err)
		}
		if _, err = fixture.Service.ApplyContextTurnRefreshV1(context.Background(), sealApply(t, fixture, prepared)); !errors.Is(err, contract.ErrConflict) {
			t.Fatalf("err=%v", err)
		}
		assertPending(t, fixture, prepared)
	})
	t.Run("tool_source", func(t *testing.T) {
		fixture := mustRefreshFixture(t)
		prepared, err := fixture.Service.RefreshContextTurnV1(context.Background(), fixture.Request)
		if err != nil {
			t.Fatal(err)
		}
		changed := fixture.ToolProjection
		changed.ExpiresUnixNano--
		changed, err = contract.SealSettledActionContextSourceCurrentV1(changed, fixture.Now.UnixNano())
		if err != nil {
			t.Fatal(err)
		}
		fixture.ToolReader.SetProjection(changed)
		if _, err = fixture.Service.ApplyContextTurnRefreshV1(context.Background(), sealApply(t, fixture, prepared)); !errors.Is(err, contract.ErrConflict) {
			t.Fatalf("err=%v", err)
		}
		assertPending(t, fixture, prepared)
	})
}

func TestContextTurnRefreshCancelPreservedAndZeroState(t *testing.T) {
	fixture := mustRefreshFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.Service.RefreshContextTurnV1(ctx, fixture.Request); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	attempt := contract.FactRef{ID: fixture.Request.RefreshAttemptID, Revision: 1, Digest: fixture.Request.Digest}
	if _, err := fixture.Service.InspectContextTurnRefreshV1(context.Background(), contract.InspectContextTurnRefreshRequestV1{AttemptRef: attempt}); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("canceled refresh wrote state: %v", err)
	}
}

func TestContextTurnRefresh64ConcurrentApplyHasOneExactCurrent(t *testing.T) {
	fixture := mustRefreshFixture(t)
	prepared, err := fixture.Service.RefreshContextTurnV1(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	apply := sealApply(t, fixture, prepared)
	const workers = 64
	results := make(chan contract.ContextTurnRefreshResultV1, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, applyErr := fixture.Service.ApplyContextTurnRefreshV1(context.Background(), apply)
			if applyErr != nil {
				errs <- applyErr
				return
			}
			results <- result
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	inspectOnlyOrConflict := 0
	for applyErr := range errs {
		if errors.Is(applyErr, contract.ErrInspectOnly) || errors.Is(applyErr, contract.ErrConflict) {
			inspectOnlyOrConflict++
			continue
		}
		t.Fatalf("concurrent apply: %v", applyErr)
	}
	var first contract.ContextTurnRefreshResultV1
	count := 0
	for result := range results {
		if count == 0 {
			first = result
		} else if !reflect.DeepEqual(result, first) {
			t.Fatal("concurrent apply exposed multiple current results")
		}
		count++
	}
	if count != 1 || inspectOnlyOrConflict != workers-1 {
		t.Fatalf("success=%d inspect_only_or_conflict=%d", count, inspectOnlyOrConflict)
	}
	inspected, err := fixture.Service.InspectContextTurnRefreshV1(context.Background(), contract.InspectContextTurnRefreshRequestV1{AttemptRef: prepared.AttemptRef})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inspected, first) || inspected.Current == nil {
		t.Fatal("single exact current not inspectable")
	}
}

func TestContextTurnRefreshS2BarrierAuthoritativeDriftCASConflicts(t *testing.T) {
	fixture := mustRefreshFixture(t)
	barrier := &applyBarrierStoreV1{ContextTurnRefreshOwnerBackendV1: fixture.Store, entered: make(chan struct{}), release: make(chan struct{})}
	service, err := kernel.NewContextTurnRefreshServiceV1(barrier, fixture.ToolReader, fixture.Parent.Content, fixture.Clock.Now, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.RefreshContextTurnV1(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	record, err := fixture.Store.LoadContextTurnRefreshPendingRecordV1(context.Background(), prepared.AttemptRef)
	if err != nil {
		t.Fatal(err)
	}
	apply := sealApply(t, fixture, prepared)
	applyDone := make(chan error, 1)
	go func() {
		_, applyErr := service.ApplyContextTurnRefreshV1(context.Background(), apply)
		applyDone <- applyErr
	}()
	select {
	case <-barrier.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("Apply did not reach post-S2 barrier")
	}
	drift := fixture.Parent.Pointer
	drift.Revision++
	drift, err = contract.SealContextGenerationCurrentPointerV1(drift)
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.Store.CompareAndSwapGenerationCurrentV1(context.Background(), fixture.Parent.Pointer, drift); err != nil {
		t.Fatal(err)
	}
	close(barrier.release)
	if err = <-applyDone; !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("Apply CAS err=%v", err)
	}
	assertPending(t, fixture, prepared)
	if _, err = fixture.Store.ResolveExactSourceBinding(context.Background(), record.Binding.Source); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("failed CAS exposed child binding: %v", err)
	}
	if _, err = fixture.Store.FrameByExactRef(context.Background(), record.Pending.FrameRef, record.Frame.Execution.ScopeDigest); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("failed CAS exposed child frame: %v", err)
	}
	if _, err = fixture.Store.ManifestByExactRef(context.Background(), record.Pending.ManifestRef, record.Frame.Execution.ScopeDigest); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("failed CAS exposed child manifest: %v", err)
	}
	if _, err = fixture.Store.GenerationByExactRef(context.Background(), record.Pending.GenerationRef, record.Frame.Execution.ScopeDigest); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("failed CAS exposed child generation: %v", err)
	}
	current, err := fixture.Store.InspectCurrentGenerationPointer(context.Background(), contract.ContextGenerationCurrentPointerRequestV1{ExecutionScopeDigest: drift.ExecutionScopeDigest, RunID: drift.RunID, SessionRef: drift.SessionRef, Turn: drift.Turn})
	if err != nil {
		t.Fatal(err)
	}
	if current != drift {
		t.Fatal("legal writer drift was not authoritative")
	}
}

func TestContextTurnRefreshReserveCannotSeedShadowCurrent(t *testing.T) {
	fixture := mustRefreshFixture(t)
	prepared, err := fixture.Service.RefreshContextTurnV1(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	record, err := fixture.Store.LoadContextTurnRefreshPendingRecordV1(context.Background(), prepared.AttemptRef)
	if err != nil {
		t.Fatal(err)
	}
	empty := refreshstore.NewMemory()
	if _, err = empty.ReserveContextTurnRefreshV1(context.Background(), record); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("empty backend accepted request-seeded current: %v", err)
	}
	if _, err = empty.InspectCurrentGenerationPointer(context.Background(), contract.ContextGenerationCurrentPointerRequestV1{ExecutionScopeDigest: fixture.Parent.Pointer.ExecutionScopeDigest, RunID: fixture.Parent.Pointer.RunID, SessionRef: fixture.Parent.Pointer.SessionRef, Turn: fixture.Parent.Pointer.Turn}); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("Reserve created shadow current: %v", err)
	}
}

func TestContextTurnRefreshApplyFailsClosedOnOwnerInspectError(t *testing.T) {
	for _, test := range []struct {
		name  string
		cause error
	}{{"unavailable", contract.ErrUnavailable}, {"conflict", contract.ErrConflict}} {
		t.Run(test.name, func(t *testing.T) {
			fixture := mustRefreshFixture(t)
			prepared, err := fixture.Service.RefreshContextTurnV1(context.Background(), fixture.Request)
			if err != nil {
				t.Fatal(err)
			}
			forced := fmt.Errorf("forced owner inspect: %w", test.cause)
			backend := &inspectErrorOwnerBackendV1{ContextTurnRefreshOwnerBackendV1: fixture.Store, err: forced}
			service, err := kernel.NewContextTurnRefreshServiceV1(backend, fixture.ToolReader, fixture.Parent.Content, fixture.Clock.Now, 30*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.ApplyContextTurnRefreshV1(context.Background(), sealApply(t, fixture, prepared))
			if err != forced {
				t.Fatalf("Inspect error was not returned exactly: got %v want %v", err, forced)
			}
			if backend.applyCalls.Load() != 0 {
				t.Fatalf("Apply CAS calls=%d", backend.applyCalls.Load())
			}
			assertPending(t, fixture, prepared)
		})
	}
}

func TestContextTurnRefresh64CompetingAttemptsHaveSingleCASWinner(t *testing.T) {
	fixture := mustRefreshFixture(t)
	const workers = 64
	prepared := make([]contract.ContextTurnRefreshPreparedV1, workers)
	requests := make([]contract.ContextTurnRefreshRequestV1, workers)
	for i := 0; i < workers; i++ {
		request := fixture.Request
		request.IdempotencyKey = fmt.Sprintf("refresh-competitor-%d", i)
		var err error
		request, err = contract.SealContextTurnRefreshRequestV1(request)
		if err != nil {
			t.Fatal(err)
		}
		requests[i] = request
		prepared[i], err = fixture.Service.RefreshContextTurnV1(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	wins := make(chan contract.ContextTurnRefreshResultV1, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			apply, err := contract.SealApplyContextTurnRefreshRequestV1(contract.ApplyContextTurnRefreshRequestV1{AttemptRef: prepared[index].AttemptRef, PendingDomainResultRef: prepared[index].PendingDomainResultRef, ExpectedCurrent: requests[index].ExpectedCurrent, CheckedUnixNano: fixture.Now.UnixNano(), NotAfterUnixNano: prepared[index].ExpiresUnixNano}, fixture.Now.UnixNano())
			if err != nil {
				errs <- err
				return
			}
			result, err := fixture.Service.ApplyContextTurnRefreshV1(context.Background(), apply)
			if err != nil {
				errs <- err
				return
			}
			wins <- result
		}(i)
	}
	wg.Wait()
	close(wins)
	close(errs)
	winnerCount := 0
	for range wins {
		winnerCount++
	}
	conflicts := 0
	for err := range errs {
		if errors.Is(err, contract.ErrConflict) {
			conflicts++
			continue
		}
		t.Fatalf("unexpected contender error: %v", err)
	}
	if winnerCount != 1 || conflicts != workers-1 {
		t.Fatalf("winners=%d conflicts=%d", winnerCount, conflicts)
	}
}

func mustRefreshFixture(t *testing.T) *testfixture.RefreshFixtureV1 {
	t.Helper()
	fixture, err := testfixture.NewRefreshFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}
func sealApply(t *testing.T, fixture *testfixture.RefreshFixtureV1, prepared contract.ContextTurnRefreshPreparedV1) contract.ApplyContextTurnRefreshRequestV1 {
	t.Helper()
	request, err := contract.SealApplyContextTurnRefreshRequestV1(contract.ApplyContextTurnRefreshRequestV1{AttemptRef: prepared.AttemptRef, PendingDomainResultRef: prepared.PendingDomainResultRef, ExpectedCurrent: fixture.Request.ExpectedCurrent, CheckedUnixNano: fixture.Now.UnixNano(), NotAfterUnixNano: prepared.ExpiresUnixNano}, fixture.Now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	return request
}
func assertPending(t *testing.T, fixture *testfixture.RefreshFixtureV1, prepared contract.ContextTurnRefreshPreparedV1) {
	t.Helper()
	result, err := fixture.Service.InspectContextTurnRefreshV1(context.Background(), contract.InspectContextTurnRefreshRequestV1{AttemptRef: prepared.AttemptRef})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != contract.ContextTurnRefreshPendingV1 || result.Current != nil || result.ApplySettlementRef != nil {
		t.Fatalf("failed S2 published current: %#v", result)
	}
}
func sameContent(a, b *contract.ContentRef) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
