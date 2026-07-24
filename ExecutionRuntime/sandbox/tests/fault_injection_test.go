package sandbox_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestFaultObservationOrderingAndConflict(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewMemoryStore()
	_ = store.SeedProjection(testkit.Projection())
	controller, _ := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
	reservation := testkit.Reservation(contract.EffectAllocate, 1, "source")
	if err := controller.Reserve(ctx, reservation); err != nil {
		t.Fatal(err)
	}
	first := testkit.Observation(reservation, 1, "source-first")
	if accepted, err := controller.RecordObservation(ctx, first); err != nil || !accepted {
		t.Fatalf("first observation = %v, %v", accepted, err)
	}
	if accepted, err := controller.RecordObservation(ctx, first); err != nil || accepted {
		t.Fatalf("idempotent observation = %v, %v", accepted, err)
	}
	conflict := testkit.Observation(reservation, 1, "source-conflict")
	if _, err := controller.RecordObservation(ctx, conflict); !errors.Is(err, ports.ErrSourceConflict) {
		t.Fatalf("same source sequence conflict error = %v", err)
	}
	second := testkit.Observation(reservation, 2, "source-second")
	if accepted, err := controller.RecordObservation(ctx, second); err != nil || !accepted {
		t.Fatalf("second observation = %v, %v", accepted, err)
	}
	stale := testkit.Observation(reservation, 1, "source-stale")
	if _, err := controller.RecordObservation(ctx, stale); !errors.Is(err, ports.ErrStale) {
		t.Fatalf("stale source error = %v", err)
	}
}

func TestFaultUnknownSettlementDoesNotApplyLifecycleFact(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewMemoryStore()
	_ = store.SeedProjection(testkit.Projection())
	controller, _ := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
	reservation := testkit.Reservation(contract.EffectAllocate, 1, "unknown")
	if err := controller.Reserve(ctx, reservation); err != nil {
		t.Fatal(err)
	}
	observation := testkit.Observation(reservation, 1, "unknown")
	if _, err := controller.RecordObservation(ctx, observation); err != nil {
		t.Fatal(err)
	}
	inspection := testkit.Inspection(reservation, observation, contract.DispositionUnknown, "unknown")
	if err := controller.RecordInspection(ctx, inspection); err != nil {
		t.Fatal(err)
	}
	result := testkit.Result(reservation, inspection, contract.DomainResultPayload{}, "unknown")
	if err := controller.CommitDomainResult(ctx, result); err != nil {
		t.Fatal(err)
	}
	projection, err := controller.ApplySettlement(ctx, result.Meta.ID, testkit.Settlement(result, "unknown"))
	if err != nil {
		t.Fatal(err)
	}
	if projection.Allocated {
		t.Fatal("unknown outcome applied allocation")
	}
	resolutionInspection := testkit.Inspection(reservation, observation, contract.DispositionConfirmedNotApplied, "unknown-resolution")
	if err := controller.RecordInspection(ctx, resolutionInspection); err != nil {
		t.Fatalf("inspect original unknown attempt: %v", err)
	}
	retry := testkit.Reservation(contract.EffectAllocate, projection.Meta.Revision, "unknown-retry")
	retry.OperationID = reservation.OperationID
	if err := controller.Reserve(ctx, retry); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatalf("unknown outcome admitted a new attempt: %v", err)
	}
	if _, err := store.GetReservation(ctx, retry.Meta.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("unknown retry wrote reservation: %v", err)
	}
	for _, feature := range []ports.Feature{ports.FeatureExternalLifecycle, ports.FeatureRuntimeAdapter, ports.FeatureApplicationAdapter, ports.FeatureCheckpointRestore, ports.FeatureAssemblyBinding, ports.FeatureWorkspaceCommit, ports.FeatureRemoteBackend, ports.FeatureGovernedAPI, ports.FeatureSnapshotArtifactCapture} {
		if !ports.Supported(feature) || ports.RequireSupported(feature) != nil {
			t.Fatalf("implemented feature %q was not reported supported", feature)
		}
	}
	if ports.Supported(ports.FeatureSnapshotArtifactOwner) || !errors.Is(ports.RequireSupported(ports.FeatureSnapshotArtifactOwner), ports.ErrUnsupported) {
		t.Fatal("terminal Snapshot Artifact lifecycle was reported supported before Retention/purge closure")
	}
}

func TestFaultConcurrentExactSettlementReplayConverges(t *testing.T) {
	ctx := context.Background()
	base := testkit.NewMemoryStore()
	_ = base.SeedProjection(testkit.Projection())
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	store := &casBarrierStore{FactStore: base, entered: entered, release: release}
	controller, _ := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
	result := commitAppliedResult(t, ctx, controller, contract.EffectAllocate, 1, 1, "exact-race", contract.DomainResultPayload{AllocationConfirmed: true})
	settlement := testkit.Settlement(result, "exact-race")

	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			projection, err := controller.ApplySettlement(ctx, result.Meta.ID, settlement)
			if err != nil {
				t.Errorf("exact replay error = %v", err)
				return
			}
			if projection.Meta.Revision != 2 || !projection.Allocated {
				t.Errorf("exact replay projection = %#v", projection)
				return
			}
			successes.Add(1)
		}()
	}
	<-entered
	<-entered
	close(release)
	wg.Wait()
	if successes.Load() != 2 {
		t.Fatalf("exact settlement successes = %d, want 2", successes.Load())
	}
}

func TestFaultSettlementCASLostReplyAndLateExactReplay(t *testing.T) {
	ctx := context.Background()
	base := testkit.NewMemoryStore()
	_ = base.SeedProjection(testkit.Projection())
	store := &lostReplyStore{FactStore: base}
	controller, _ := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
	allocate := commitAppliedResult(t, ctx, controller, contract.EffectAllocate, 1, 1, "lost-allocate", contract.DomainResultPayload{AllocationConfirmed: true})
	allocateSettlement := testkit.Settlement(allocate, "lost-allocate")
	projection, err := controller.ApplySettlement(ctx, allocate.Meta.ID, allocateSettlement)
	if err != nil || projection.Meta.Revision != 2 || !projection.Allocated {
		t.Fatalf("lost CAS reply did not inspect durable binding: %#v, %v", projection, err)
	}
	projection = settleApplied(t, ctx, controller, contract.EffectActivate, 2, 2, "after-lost", contract.DomainResultPayload{ActivationConfirmed: true})
	replayed, err := controller.ApplySettlement(ctx, allocate.Meta.ID, allocateSettlement)
	if err != nil {
		t.Fatalf("late exact lost-reply replay: %v", err)
	}
	if replayed.Meta.Revision != projection.Meta.Revision || !replayed.Activated {
		t.Fatalf("late exact replay returned stale projection: %#v", replayed)
	}
}

func TestNoGoOpaqueSettlementRefCannotAliasAnotherDomainResult(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewMemoryStore()
	_ = store.SeedProjection(testkit.Projection())
	controller, _ := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
	allocate := commitAppliedResult(t, ctx, controller, contract.EffectAllocate, 1, 1, "alias-allocate", contract.DomainResultPayload{AllocationConfirmed: true})
	allocateSettlement := testkit.Settlement(allocate, "shared-opaque")
	if _, err := controller.ApplySettlement(ctx, allocate.Meta.ID, allocateSettlement); err != nil {
		t.Fatal(err)
	}
	activate := commitAppliedResult(t, ctx, controller, contract.EffectActivate, 2, 2, "alias-activate", contract.DomainResultPayload{ActivationConfirmed: true})
	aliased := testkit.Settlement(activate, "alias-activate")
	aliased.OpaqueRef = allocateSettlement.OpaqueRef
	if _, err := controller.ApplySettlement(ctx, activate.Meta.ID, aliased); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("opaque settlement alias error = %v", err)
	}
	projection, err := store.GetProjection(ctx, testkit.Lease().LeaseID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Activated || projection.Meta.Revision != 2 {
		t.Fatalf("opaque alias mutated projection: %#v", projection)
	}
}

func TestFaultConcurrentSettlementCASHasSingleWinner(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewMemoryStore()
	_ = store.SeedProjection(testkit.Projection())
	controller, _ := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
	first := commitAppliedResult(t, ctx, controller, contract.EffectAllocate, 1, 1, "race-one", contract.DomainResultPayload{AllocationConfirmed: true})
	second := commitAppliedResult(t, ctx, controller, contract.EffectAllocate, 1, 2, "race-two", contract.DomainResultPayload{AllocationConfirmed: true})

	var successes atomic.Int32
	var wg sync.WaitGroup
	for suffix, result := range map[string]contract.SandboxDomainResultFact{"one": first, "two": second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := controller.ApplySettlement(ctx, result.Meta.ID, testkit.Settlement(result, suffix)); err == nil {
				successes.Add(1)
			} else if !errors.Is(err, ports.ErrStale) {
				t.Errorf("ApplySettlement error = %v", err)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("settlement successes = %d, want 1", successes.Load())
	}
}

func TestNoGoHistoricalTerminalIsInspectableAndRecoverableButNotExecutable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewMemoryStore()
	projection := testkit.Projection()
	projection.Allocated = true
	projection.Activated = true
	projection.ExecutionQuiesced = true
	projection.EnvironmentClosed = true
	projection.Fenced = true
	projection.Released = true
	projection.Cleanup = testkit.CompleteCleanup()
	projection.Meta = expiredMeta(projection.Meta)
	projection.Lease.ExpiresUnixNano = testkit.FixedNow.Add(-time.Hour).UnixNano()
	if err := projection.ValidateShape(); err != nil {
		t.Fatalf("historical terminal shape rejected: %v", err)
	}
	if err := projection.ValidateCurrent(testkit.FixedNow); err == nil {
		t.Fatal("expired terminal projection was treated as currently usable")
	}
	if err := store.SeedProjection(projection); err != nil {
		t.Fatal(err)
	}

	reservation := testkit.Reservation(contract.EffectInspect, projection.Meta.Revision, "historical-inspect")
	reservation.Meta = expiredMeta(reservation.Meta)
	reservation.Lease = projection.Lease
	if err := reservation.ValidateShape(); err != nil {
		t.Fatalf("historical reservation shape rejected: %v", err)
	}
	if err := reservation.ValidateCurrent(testkit.FixedNow); err == nil {
		t.Fatal("expired historical reservation was eligible for new execution")
	}
	if err := store.CreateReservation(ctx, reservation); err != nil {
		t.Fatal(err)
	}
	observation := testkit.Observation(reservation, 1, "historical-inspect")
	observation.Meta = expiredMeta(observation.Meta)
	if err := observation.ValidateShape(); err != nil {
		t.Fatalf("historical observation shape rejected: %v", err)
	}
	if accepted, err := store.AppendObservation(ctx, reservation.Meta.ID, observation); err != nil || !accepted {
		t.Fatalf("seed historical observation = %v, %v", accepted, err)
	}

	controller, _ := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
	inspection := testkit.Inspection(reservation, observation, contract.DispositionConfirmedApplied, "historical-inspect")
	if err := controller.RecordInspection(ctx, inspection); err != nil {
		t.Fatalf("current inspection of historical terminal failed: %v", err)
	}
	result := testkit.Result(reservation, inspection, contract.DomainResultPayload{}, "historical-inspect")
	if err := controller.CommitDomainResult(ctx, result); err != nil {
		t.Fatalf("historical recovery result failed: %v", err)
	}
	recovered, err := controller.ApplySettlement(ctx, result.Meta.ID, testkit.Settlement(result, "historical-inspect"))
	if err != nil {
		t.Fatalf("opaque settlement recovery failed: %v", err)
	}
	if recovered.Meta.Revision != projection.Meta.Revision+1 {
		t.Fatalf("recovery revision = %d", recovered.Meta.Revision)
	}
	if err := recovered.ValidateShape(); err != nil {
		t.Fatalf("recovered historical projection shape rejected: %v", err)
	}
	if err := recovered.ValidateCurrent(testkit.FixedNow); err == nil {
		t.Fatal("recovery renewed expired execution eligibility")
	}

	newExecution := testkit.Reservation(contract.EffectOpen, recovered.Meta.Revision, "expired-new-execution")
	newExecution.Lease = recovered.Lease
	if err := controller.Reserve(ctx, newExecution); err == nil {
		t.Fatal("expired historical lease authorized a new execution reservation")
	}
}

func expiredMeta(meta contract.Meta) contract.Meta {
	meta.CreatedUnixNano = testkit.FixedNow.Add(-3 * time.Hour).UnixNano()
	meta.UpdatedUnixNano = testkit.FixedNow.Add(-2 * time.Hour).UnixNano()
	meta.ExpiresUnixNano = testkit.FixedNow.Add(-time.Hour).UnixNano()
	return meta
}

type casBarrierStore struct {
	ports.FactStore
	entered chan<- struct{}
	release <-chan struct{}
}

func (s *casBarrierStore) CompareAndSwapProjection(ctx context.Context, expectedRevision uint64, projection contract.EnvironmentProjection) error {
	s.entered <- struct{}{}
	<-s.release
	return s.FactStore.CompareAndSwapProjection(ctx, expectedRevision, projection)
}

type lostReplyStore struct {
	ports.FactStore
	injected atomic.Bool
}

func (s *lostReplyStore) CompareAndSwapProjection(ctx context.Context, expectedRevision uint64, projection contract.EnvironmentProjection) error {
	if err := s.FactStore.CompareAndSwapProjection(ctx, expectedRevision, projection); err != nil {
		return err
	}
	if s.injected.CompareAndSwap(false, true) {
		return errors.New("injected CAS reply loss")
	}
	return nil
}
