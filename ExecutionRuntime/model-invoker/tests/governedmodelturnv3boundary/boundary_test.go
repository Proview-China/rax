package governedmodelturnv3boundary_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/runtimeadapter"
	modelsqlite "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	_ "modernc.org/sqlite"
)

var boundaryNow = time.Unix(1_900_000_000, 0)

type boundaryFixture struct {
	path     string
	store    *modelsqlite.Store
	prepared modelinvoker.PreparedModelInvocationFactV1
	current  modelinvoker.PreparedModelInvocationCurrentProjectionV1
	material modelinvoker.InvocationMaterialV2
	turn     modelinvoker.GovernedModelTurnOutcomeV3
	ack      modelinvoker.PreparedModelInvocationCommitAckV1
	receipt  modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1
	provider runtimeports.ProviderBindingRefV2
	request  runtimeports.InspectCurrentModelProviderActualPointRequestV1
	fact     modelinvoker.GovernedModelTurnProviderBoundaryFactV3
}

type exactAckReader struct {
	ack modelinvoker.PreparedModelInvocationCommitAckV1
}

type driftingTurnReader struct {
	outcome modelinvoker.GovernedModelTurnOutcomeV3
	calls   *atomic.Int64
}

func (r driftingTurnReader) EnsurePreparedGovernedModelTurnV3(
	context.Context,
	modelinvoker.GovernedModelTurnOutcomeV3,
) (modelinvoker.GovernedModelTurnMutationV3, error) {
	panic("drifting Turn reader must remain read-only")
}

func (r driftingTurnReader) InspectGovernedModelTurnAttemptV3(
	context.Context,
	modelinvoker.GovernedModelTurnAttemptRefV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	panic("Build must use exact Turn Inspect")
}

func (r driftingTurnReader) InspectExactGovernedModelTurnV3(
	context.Context,
	modelinvoker.GovernedModelTurnRefV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	if r.calls != nil {
		r.calls.Add(1)
	}
	return r.outcome, nil
}

func (r exactAckReader) InspectExactAck(
	_ context.Context,
	ref modelinvoker.PreparedModelInvocationCommitAckRefV1,
) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	if r.ack.Ref() != ref {
		return modelinvoker.PreparedModelInvocationCommitAckV1{},
			&modelinvoker.GovernedModelInvocationErrorV1{
				Kind:      modelinvoker.GovernedModelInvocationErrorConflict,
				Operation: "inspect_ack",
				Message:   "ACK drifted",
			}
	}
	return r.ack, nil
}

type lostReplyBoundaryRepository struct {
	inner       *modelsqlite.Store
	ensureCalls atomic.Int64
	inspectRefs chan modelinvoker.GovernedModelTurnProviderBoundaryRefV3
}

type conflictRecoveryBoundaryRepository struct {
	inner *modelsqlite.Store
}

type lyingAppliedBoundaryRepository struct {
	inner *modelsqlite.Store
}

type mismatchedMutationBoundaryRepository struct {
	inner *modelsqlite.Store
}

func (r *lostReplyBoundaryRepository) EnsureGovernedModelTurnProviderBoundaryFactV3(
	ctx context.Context,
	fact modelinvoker.GovernedModelTurnProviderBoundaryFactV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryMutationV3, error) {
	r.ensureCalls.Add(1)
	if _, err := r.inner.EnsureGovernedModelTurnProviderBoundaryFactV3(ctx, fact); err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{}, err
	}
	return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{},
		&modelinvoker.GovernedModelInvocationErrorV1{
			Kind:      modelinvoker.GovernedModelInvocationErrorIndeterminate,
			Operation: "ensure_provider_boundary_v3",
			Message:   "simulated lost reply",
		}
}

func (r *lostReplyBoundaryRepository) InspectGovernedModelTurnProviderBoundaryAttemptV3(
	ctx context.Context,
	ref runtimeports.ModelProviderBoundaryCurrentRefV1,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	return r.inner.InspectGovernedModelTurnProviderBoundaryAttemptV3(ctx, ref)
}

func (r *lostReplyBoundaryRepository) InspectExactGovernedModelTurnProviderBoundaryV3(
	ctx context.Context,
	ref modelinvoker.GovernedModelTurnProviderBoundaryRefV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	select {
	case r.inspectRefs <- ref:
	default:
	}
	return r.inner.InspectExactGovernedModelTurnProviderBoundaryV3(ctx, ref)
}

func (r *conflictRecoveryBoundaryRepository) EnsureGovernedModelTurnProviderBoundaryFactV3(
	ctx context.Context,
	fact modelinvoker.GovernedModelTurnProviderBoundaryFactV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryMutationV3, error) {
	if _, err := r.inner.EnsureGovernedModelTurnProviderBoundaryFactV3(ctx, fact); err != nil {
		return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{}, err
	}
	return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{},
		&modelinvoker.GovernedModelInvocationErrorV1{
			Kind:      modelinvoker.GovernedModelInvocationErrorConflict,
			Operation: "ensure_provider_boundary_v3",
			Message:   "simulated conflict after commit",
		}
}

func (r *conflictRecoveryBoundaryRepository) InspectGovernedModelTurnProviderBoundaryAttemptV3(
	ctx context.Context,
	ref runtimeports.ModelProviderBoundaryCurrentRefV1,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	return r.inner.InspectGovernedModelTurnProviderBoundaryAttemptV3(ctx, ref)
}

func (r *conflictRecoveryBoundaryRepository) InspectExactGovernedModelTurnProviderBoundaryV3(
	ctx context.Context,
	ref modelinvoker.GovernedModelTurnProviderBoundaryRefV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	return r.inner.InspectExactGovernedModelTurnProviderBoundaryV3(ctx, ref)
}

func (r *lyingAppliedBoundaryRepository) EnsureGovernedModelTurnProviderBoundaryFactV3(
	_ context.Context,
	fact modelinvoker.GovernedModelTurnProviderBoundaryFactV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryMutationV3, error) {
	return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{
		Fact:    fact,
		Applied: true,
	}, nil
}

func (r *lyingAppliedBoundaryRepository) InspectGovernedModelTurnProviderBoundaryAttemptV3(
	ctx context.Context,
	ref runtimeports.ModelProviderBoundaryCurrentRefV1,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	return r.inner.InspectGovernedModelTurnProviderBoundaryAttemptV3(ctx, ref)
}

func (r *lyingAppliedBoundaryRepository) InspectExactGovernedModelTurnProviderBoundaryV3(
	ctx context.Context,
	ref modelinvoker.GovernedModelTurnProviderBoundaryRefV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	return r.inner.InspectExactGovernedModelTurnProviderBoundaryV3(ctx, ref)
}

func (r *mismatchedMutationBoundaryRepository) EnsureGovernedModelTurnProviderBoundaryFactV3(
	context.Context,
	modelinvoker.GovernedModelTurnProviderBoundaryFactV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryMutationV3, error) {
	return modelinvoker.GovernedModelTurnProviderBoundaryMutationV3{
		Applied: true,
	}, nil
}

func (r *mismatchedMutationBoundaryRepository) InspectGovernedModelTurnProviderBoundaryAttemptV3(
	ctx context.Context,
	ref runtimeports.ModelProviderBoundaryCurrentRefV1,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	return r.inner.InspectGovernedModelTurnProviderBoundaryAttemptV3(ctx, ref)
}

func (r *mismatchedMutationBoundaryRepository) InspectExactGovernedModelTurnProviderBoundaryV3(
	ctx context.Context,
	ref modelinvoker.GovernedModelTurnProviderBoundaryRefV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	return r.inner.InspectExactGovernedModelTurnProviderBoundaryV3(ctx, ref)
}

type typedNilBoundaryRepository struct{}

type typedNilACKReader struct{}

func (*typedNilACKReader) InspectExactAck(
	context.Context,
	modelinvoker.PreparedModelInvocationCommitAckRefV1,
) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	panic("typed nil ACK reader must not be called")
}

type sequenceClock struct {
	mu     sync.Mutex
	values []time.Time
	index  int
}

func (c *sequenceClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.values) == 0 {
		return time.Time{}
	}
	index := c.index
	if index >= len(c.values) {
		index = len(c.values) - 1
	} else {
		c.index++
	}
	return c.values[index]
}

func (*typedNilBoundaryRepository) EnsureGovernedModelTurnProviderBoundaryFactV3(
	context.Context,
	modelinvoker.GovernedModelTurnProviderBoundaryFactV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryMutationV3, error) {
	panic("typed nil repository must not be called")
}
func (*typedNilBoundaryRepository) InspectGovernedModelTurnProviderBoundaryAttemptV3(
	context.Context,
	runtimeports.ModelProviderBoundaryCurrentRefV1,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	panic("typed nil repository must not be called")
}
func (*typedNilBoundaryRepository) InspectExactGovernedModelTurnProviderBoundaryV3(
	context.Context,
	modelinvoker.GovernedModelTurnProviderBoundaryRefV3,
) (modelinvoker.GovernedModelTurnProviderBoundaryFactV3, error) {
	panic("typed nil repository must not be called")
}

func TestRealSQLiteBoundaryRestartAndRuntimeProjection(t *testing.T) {
	fixture := newBoundaryFixture(t)
	ref := fixture.fact.RefV3()
	result, err := modelinvoker.EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		fixture.store,
		fixture.fact,
		func() time.Time { return boundaryNow.Add(time.Minute) },
	)
	if err != nil || !reflect.DeepEqual(result.Fact, fixture.fact) ||
		result.Disposition != modelinvoker.GovernedModelTurnProviderBoundaryPersistenceCreatedV3 {
		t.Fatalf("ensure result=%#v err=%v", result, err)
	}
	if err := fixture.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openBoundaryStore(t, fixture.path)
	defer reopened.Close()
	exact, err := reopened.InspectExactGovernedModelTurnProviderBoundaryV3(
		context.Background(),
		ref,
	)
	if err != nil || !reflect.DeepEqual(exact, fixture.fact) {
		t.Fatalf("restart exact=%#v err=%v", exact, err)
	}
	adapter, err := runtimeadapter.NewModelProviderBoundaryCurrentAdapterV1(
		reopened,
		func() time.Time { return boundaryNow.Add(time.Minute) },
	)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := adapter.InspectCurrentModelProviderBoundaryV1(
		context.Background(),
		ref.RuntimeBoundary,
	)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Ref != ref.RuntimeBoundary ||
		projection.Provider != fixture.provider ||
		projection.ProjectionDigest != fixture.fact.ProjectionDigest ||
		projection.ExpiresUnixNano != fixture.fact.ExpiresUnixNano {
		t.Fatalf("Runtime projection drifted: %#v", projection)
	}
}

func TestBoundaryCreateOnceUnder64ConcurrentWritersAndReaders(t *testing.T) {
	fixture := newBoundaryFixture(t)
	defer fixture.store.Close()
	const workers = 64
	var group sync.WaitGroup
	failures := make(chan error, workers)
	results := make(
		chan modelinvoker.GovernedModelTurnProviderBoundaryPersistenceResultV3,
		workers,
	)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := modelinvoker.EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
				context.Background(),
				fixture.store,
				fixture.fact,
				func() time.Time { return boundaryNow.Add(time.Minute) },
			)
			if err != nil {
				failures <- err
				return
			}
			results <- result
		}()
	}
	group.Wait()
	close(failures)
	close(results)
	for err := range failures {
		t.Fatal(err)
	}
	created := 0
	existing := 0
	recovered := 0
	for result := range results {
		if !reflect.DeepEqual(result.Fact, fixture.fact) {
			t.Fatal("concurrent writer returned a sibling Fact")
		}
		switch result.Disposition {
		case modelinvoker.GovernedModelTurnProviderBoundaryPersistenceCreatedV3:
			created++
		case modelinvoker.GovernedModelTurnProviderBoundaryPersistenceExistingV3:
			existing++
		case modelinvoker.GovernedModelTurnProviderBoundaryPersistenceRecoveredUnknownV3:
			recovered++
		default:
			t.Fatalf("unexpected disposition: %q", result.Disposition)
		}
	}
	if created != 1 || existing+recovered != workers-1 {
		t.Fatalf(
			"create-once dispositions created=%d existing=%d recovered=%d",
			created,
			existing,
			recovered,
		)
	}
	assertBoundaryRows(t, fixture.path, 1)

	adapter, err := runtimeadapter.NewModelProviderBoundaryCurrentAdapterV1(
		fixture.store,
		func() time.Time { return boundaryNow.Add(time.Minute) },
	)
	if err != nil {
		t.Fatal(err)
	}
	group = sync.WaitGroup{}
	readFailures := make(chan error, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			projection, err := adapter.InspectCurrentModelProviderBoundaryV1(
				context.Background(),
				fixture.fact.RuntimeRequest.ModelBoundary,
			)
			if err != nil || projection.ProjectionDigest != fixture.fact.ProjectionDigest {
				readFailures <- fmt.Errorf("read projection: %#v: %w", projection, err)
			}
		}()
	}
	group.Wait()
	close(readFailures)
	for err := range readFailures {
		t.Fatal(err)
	}
}

func TestSameBoundaryIdentityDifferentProviderIsConflict(t *testing.T) {
	fixture := newBoundaryFixture(t)
	defer fixture.store.Close()
	if _, err := fixture.store.EnsureGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		fixture.fact,
	); err != nil {
		t.Fatal(err)
	}
	changedProvider := fixture.provider
	changedProvider.BindingSetRevision++
	request := fixture.request
	request.Verifier = changedProvider
	request.ModelBoundary = runtimeports.ModelProviderBoundaryCurrentRefV1{}
	ref, err := modelinvoker.DeriveGovernedModelTurnProviderBoundaryRefV3(
		fixture.turn,
		fixture.receipt,
		request,
		changedProvider,
		boundaryNow.UnixNano(),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.ModelBoundary = ref.RuntimeBoundary
	changed, err := modelinvoker.BuildGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		boundaryReaders(fixture),
		modelinvoker.GovernedModelTurnProviderBoundaryDraftV3{
			TurnRef: fixture.turn.RefV3(), DispatchReceipt: fixture.receipt,
			RuntimeRequest: request, Provider: changedProvider,
			CheckedUnixNano: boundaryNow.UnixNano(),
		},
		boundaryNow,
	)
	if err != nil || changed.ID != fixture.fact.ID ||
		changed.Digest == fixture.fact.Digest {
		t.Fatalf("changed Fact identity/body invalid: %#v %v", changed, err)
	}
	if _, err := fixture.store.EnsureGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		changed,
	); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorConflict {
		t.Fatalf("same identity different Provider error=%v", err)
	}
}

func TestSameBoundaryIdentityRejectsRuntimeAndACKSplices(t *testing.T) {
	fixture := newBoundaryFixture(t)
	defer fixture.store.Close()
	if _, err := fixture.store.EnsureGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		fixture.fact,
	); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(
		*runtimeports.InspectCurrentModelProviderActualPointRequestV1,
		*runtimeports.ProviderBindingRefV2,
	){
		"operation": func(
			request *runtimeports.InspectCurrentModelProviderActualPointRequestV1,
			_ *runtimeports.ProviderBindingRefV2,
		) {
			request.Operation.CurrentProjectionRef = "run-current-2"
			request.Operation.CurrentProjectionRevision = 2
			request.Operation.CurrentProjectionDigest = digest("run-current-2")
			operationDigest, err := request.Operation.DigestV3()
			if err != nil {
				t.Fatal(err)
			}
			request.Attempt.OperationDigest = operationDigest
			request.Attempt.AttemptID = "runtime-attempt-operation"
		},
		"effect": func(
			request *runtimeports.InspectCurrentModelProviderActualPointRequestV1,
			_ *runtimeports.ProviderBindingRefV2,
		) {
			request.EffectID = "effect-boundary-2"
			request.Attempt.EffectID = request.EffectID
			request.Attempt.AttemptID = "runtime-attempt-effect"
		},
		"runtime-attempt": func(
			request *runtimeports.InspectCurrentModelProviderActualPointRequestV1,
			_ *runtimeports.ProviderBindingRefV2,
		) {
			request.Attempt.AttemptID = "runtime-attempt-2"
		},
		"ttl": func(
			request *runtimeports.InspectCurrentModelProviderActualPointRequestV1,
			_ *runtimeports.ProviderBindingRefV2,
		) {
			request.RequestedNotAfterUnixNano = boundaryNow.Add(5 * time.Minute).UnixNano()
		},
		"verifier-provider": func(
			request *runtimeports.InspectCurrentModelProviderActualPointRequestV1,
			provider *runtimeports.ProviderBindingRefV2,
		) {
			provider.BindingSetRevision++
			request.Verifier = *provider
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			request := fixture.request
			request.ModelBoundary = runtimeports.ModelProviderBoundaryCurrentRefV1{}
			provider := fixture.provider
			mutate(&request, &provider)
			changed := buildBoundaryFact(
				t,
				fixture,
				fixture.ack,
				fixture.receipt,
				request,
				provider,
			)
			if changed.ID != fixture.fact.ID || changed.Digest == fixture.fact.Digest {
				t.Fatalf("splice identity/body invalid: %#v", changed)
			}
			if _, err := fixture.store.EnsureGovernedModelTurnProviderBoundaryFactV3(
				context.Background(),
				changed,
			); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
				modelinvoker.GovernedModelInvocationErrorConflict {
				t.Fatalf("same identity splice error=%v", err)
			}
		})
	}

	ackDraft := fixture.ack
	ackDraft.Digest = ""
	ackDraft.GateImplementationRef.Digest = digest("commit-gate-2")
	alternateACK, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(ackDraft)
	if err != nil {
		t.Fatal(err)
	}
	receiptDraft := fixture.receipt
	receiptDraft.Digest = ""
	receiptDraft.AckRef = alternateACK.Ref()
	alternateReceipt, err := modelinvoker.SealPreparedModelInvocationDispatchReceiptAgainstV1(
		fixture.prepared,
		fixture.current,
		alternateACK,
		receiptDraft,
		boundaryNow,
	)
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.request
	request.ModelBoundary = runtimeports.ModelProviderBoundaryCurrentRefV1{}
	changed := buildBoundaryFact(
		t,
		fixture,
		alternateACK,
		alternateReceipt,
		request,
		fixture.provider,
	)
	if changed.ID != fixture.fact.ID || changed.Digest == fixture.fact.Digest {
		t.Fatalf("ACK splice identity/body invalid: %#v", changed)
	}
	if _, err := fixture.store.EnsureGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		changed,
	); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorConflict {
		t.Fatalf("same identity ACK splice error=%v", err)
	}
}

func TestBoundaryLostReplyInspectsPrederivedExactRefOnly(t *testing.T) {
	fixture := newBoundaryFixture(t)
	defer fixture.store.Close()
	repository := &lostReplyBoundaryRepository{
		inner:       fixture.store,
		inspectRefs: make(chan modelinvoker.GovernedModelTurnProviderBoundaryRefV3, 4),
	}
	result, err := modelinvoker.EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		repository,
		fixture.fact,
		func() time.Time { return boundaryNow.Add(time.Minute) },
	)
	if err != nil || !reflect.DeepEqual(result.Fact, fixture.fact) ||
		result.Disposition !=
			modelinvoker.GovernedModelTurnProviderBoundaryPersistenceRecoveredUnknownV3 {
		t.Fatalf("lost reply result=%#v err=%v", result, err)
	}
	if repository.ensureCalls.Load() != 1 {
		t.Fatalf("ensure calls=%d", repository.ensureCalls.Load())
	}
	close(repository.inspectRefs)
	for inspected := range repository.inspectRefs {
		if inspected != fixture.fact.RefV3() {
			t.Fatalf("lost reply changed exact Ref: %#v", inspected)
		}
	}
	assertBoundaryRows(t, fixture.path, 1)
}

func TestBoundaryPersistenceDispositionIsExactAndNotAuthority(t *testing.T) {
	t.Run("initial-existing", func(t *testing.T) {
		fixture := newBoundaryFixture(t)
		defer fixture.store.Close()
		if _, err := fixture.store.EnsureGovernedModelTurnProviderBoundaryFactV3(
			context.Background(),
			fixture.fact,
		); err != nil {
			t.Fatal(err)
		}
		result, err := modelinvoker.EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
			context.Background(),
			fixture.store,
			fixture.fact,
			func() time.Time { return boundaryNow.Add(time.Minute) },
		)
		if err != nil || !reflect.DeepEqual(result.Fact, fixture.fact) ||
			result.Disposition !=
				modelinvoker.GovernedModelTurnProviderBoundaryPersistenceExistingV3 {
			t.Fatalf("existing result=%#v err=%v", result, err)
		}
	})

	t.Run("conflict-recovery", func(t *testing.T) {
		fixture := newBoundaryFixture(t)
		defer fixture.store.Close()
		result, err := modelinvoker.EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
			context.Background(),
			&conflictRecoveryBoundaryRepository{inner: fixture.store},
			fixture.fact,
			func() time.Time { return boundaryNow.Add(time.Minute) },
		)
		if err != nil || !reflect.DeepEqual(result.Fact, fixture.fact) ||
			result.Disposition !=
				modelinvoker.GovernedModelTurnProviderBoundaryPersistenceRecoveredUnknownV3 {
			t.Fatalf("conflict recovery result=%#v err=%v", result, err)
		}
		assertBoundaryRows(t, fixture.path, 1)
	})

	t.Run("lying-applied-without-history", func(t *testing.T) {
		fixture := newBoundaryFixture(t)
		defer fixture.store.Close()
		result, err := modelinvoker.EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
			context.Background(),
			&lyingAppliedBoundaryRepository{inner: fixture.store},
			fixture.fact,
			func() time.Time { return boundaryNow.Add(time.Minute) },
		)
		if err == nil ||
			result != (modelinvoker.GovernedModelTurnProviderBoundaryPersistenceResultV3{}) {
			t.Fatalf("lying Applied result=%#v err=%v", result, err)
		}
		assertBoundaryRows(t, fixture.path, 0)
	})

	t.Run("mutation-body-mismatch", func(t *testing.T) {
		fixture := newBoundaryFixture(t)
		defer fixture.store.Close()
		result, err := modelinvoker.EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
			context.Background(),
			&mismatchedMutationBoundaryRepository{inner: fixture.store},
			fixture.fact,
			func() time.Time { return boundaryNow.Add(time.Minute) },
		)
		if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
			modelinvoker.GovernedModelInvocationErrorConflict ||
			result != (modelinvoker.GovernedModelTurnProviderBoundaryPersistenceResultV3{}) {
			t.Fatalf("mismatched mutation result=%#v err=%v", result, err)
		}
		assertBoundaryRows(t, fixture.path, 0)
	})

	t.Run("unknown-disposition-rejected", func(t *testing.T) {
		fixture := newBoundaryFixture(t)
		defer fixture.store.Close()
		result := modelinvoker.GovernedModelTurnProviderBoundaryPersistenceResultV3{
			Fact: fixture.fact,
			Disposition: modelinvoker.
				GovernedModelTurnProviderBoundaryPersistenceDispositionV3("future"),
		}
		if result.Validate() == nil {
			t.Fatal("unknown persistence disposition was accepted")
		}
	})
}

func TestBoundaryEnsureReturnRejectsTTLAndClockCrossing(t *testing.T) {
	t.Run("ensure-success-crosses-expiry", func(t *testing.T) {
		fixture := newBoundaryFixture(t)
		defer fixture.store.Close()
		clock := &sequenceClock{values: []time.Time{
			boundaryNow.Add(time.Minute),
			time.Unix(0, fixture.fact.ExpiresUnixNano),
		}}
		result, err := modelinvoker.EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
			context.Background(),
			fixture.store,
			fixture.fact,
			clock.Now,
		)
		if err == nil ||
			result != (modelinvoker.GovernedModelTurnProviderBoundaryPersistenceResultV3{}) {
			t.Fatalf("expired ensure result=%#v err=%v", result, err)
		}
		assertBoundaryRows(t, fixture.path, 1)
	})

	t.Run("existing-winner-crosses-expiry", func(t *testing.T) {
		fixture := newBoundaryFixture(t)
		defer fixture.store.Close()
		if _, err := fixture.store.EnsureGovernedModelTurnProviderBoundaryFactV3(
			context.Background(),
			fixture.fact,
		); err != nil {
			t.Fatal(err)
		}
		clock := &sequenceClock{values: []time.Time{
			boundaryNow.Add(time.Minute),
			time.Unix(0, fixture.fact.ExpiresUnixNano),
		}}
		result, err := modelinvoker.EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
			context.Background(),
			fixture.store,
			fixture.fact,
			clock.Now,
		)
		if err == nil ||
			result != (modelinvoker.GovernedModelTurnProviderBoundaryPersistenceResultV3{}) {
			t.Fatalf("expired historical result=%#v err=%v", result, err)
		}
	})

	t.Run("lost-reply-recovery-crosses-expiry", func(t *testing.T) {
		fixture := newBoundaryFixture(t)
		defer fixture.store.Close()
		repository := &lostReplyBoundaryRepository{
			inner:       fixture.store,
			inspectRefs: make(chan modelinvoker.GovernedModelTurnProviderBoundaryRefV3, 4),
		}
		clock := &sequenceClock{values: []time.Time{
			boundaryNow.Add(time.Minute),
			time.Unix(0, fixture.fact.ExpiresUnixNano),
		}}
		result, err := modelinvoker.EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
			context.Background(),
			repository,
			fixture.fact,
			clock.Now,
		)
		if err == nil ||
			result != (modelinvoker.GovernedModelTurnProviderBoundaryPersistenceResultV3{}) {
			t.Fatalf("expired recovery result=%#v err=%v", result, err)
		}
		if repository.ensureCalls.Load() != 1 {
			t.Fatalf("ensure calls=%d", repository.ensureCalls.Load())
		}
		assertBoundaryRows(t, fixture.path, 1)
	})

	t.Run("clock-rollback", func(t *testing.T) {
		fixture := newBoundaryFixture(t)
		defer fixture.store.Close()
		clock := &sequenceClock{values: []time.Time{
			boundaryNow.Add(time.Minute),
			boundaryNow,
		}}
		result, err := modelinvoker.EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
			context.Background(),
			fixture.store,
			fixture.fact,
			clock.Now,
		)
		if err == nil ||
			result != (modelinvoker.GovernedModelTurnProviderBoundaryPersistenceResultV3{}) {
			t.Fatalf("rollback result=%#v err=%v", result, err)
		}
	})
}

func TestBoundaryBuildRejectsExpiredTurnWithoutFact(t *testing.T) {
	fixture := newBoundaryFixture(t)
	defer fixture.store.Close()
	fact, err := modelinvoker.BuildGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		boundaryReaders(fixture),
		modelinvoker.GovernedModelTurnProviderBoundaryDraftV3{
			TurnRef: fixture.turn.RefV3(), DispatchReceipt: fixture.receipt,
			RuntimeRequest: fixture.request, Provider: fixture.provider,
			CheckedUnixNano: boundaryNow.UnixNano(),
		},
		time.Unix(0, fixture.turn.ExpiresUnixNano),
	)
	if err == nil || fact != (modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}) {
		t.Fatalf("expired Turn produced Fact=%#v err=%v", fact, err)
	}
	assertBoundaryRows(t, fixture.path, 0)
}

func TestBoundaryBuildRejectsExactTurnReaderSibling(t *testing.T) {
	fixture := newBoundaryFixture(t)
	defer fixture.store.Close()
	baseCommand := modelinvoker.GovernedModelTurnCommandV3{
		PreparedRef:            fixture.turn.PreparedRef,
		CurrentRef:             fixture.turn.CurrentRef,
		MaterialRef:            fixture.turn.MaterialRef,
		AttemptRequestDigest:   fixture.turn.AttemptRequestDigest,
		RouteCallDigest:        fixture.turn.RouteCallDigest,
		DispatchSequence:       fixture.turn.DispatchSequence,
		ProviderAttemptOrdinal: fixture.turn.ProviderAttemptOrdinal,
	}
	sameAttemptSibling, err := modelinvoker.NewPreparedGovernedModelTurnV3(
		baseCommand,
		boundaryNow.Add(-90*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if sameAttemptSibling.AttemptRefV3() != fixture.turn.AttemptRefV3() ||
		sameAttemptSibling.RefV3() == fixture.turn.RefV3() {
		t.Fatal("fixture did not create a same-Attempt sibling Turn")
	}
	differentCommand := baseCommand
	differentCommand.DispatchSequence++
	differentAttempt, err := modelinvoker.NewPreparedGovernedModelTurnV3(
		differentCommand,
		boundaryNow.Add(-90*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if differentAttempt.AttemptRefV3() == fixture.turn.AttemptRefV3() {
		t.Fatal("fixture did not create a different-Attempt Turn")
	}
	for name, returned := range map[string]modelinvoker.GovernedModelTurnOutcomeV3{
		"same-attempt-different-exact-ref": sameAttemptSibling,
		"different-attempt":                differentAttempt,
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int64
			readers := boundaryReaders(fixture)
			readers.TurnHistory = driftingTurnReader{outcome: returned, calls: &calls}
			fact, err := modelinvoker.BuildGovernedModelTurnProviderBoundaryFactV3(
				context.Background(),
				readers,
				modelinvoker.GovernedModelTurnProviderBoundaryDraftV3{
					TurnRef: fixture.turn.RefV3(), DispatchReceipt: fixture.receipt,
					RuntimeRequest: fixture.request, Provider: fixture.provider,
					CheckedUnixNano: boundaryNow.UnixNano(),
				},
				boundaryNow,
			)
			if err == nil || fact != (modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}) {
				t.Fatalf("drifting exact Turn Reader Fact=%#v err=%v", fact, err)
			}
			if calls.Load() != 1 {
				t.Fatalf("exact Turn Reader calls=%d want=1", calls.Load())
			}
		})
	}

	var invalidCalls atomic.Int64
	readers := boundaryReaders(fixture)
	readers.TurnHistory = driftingTurnReader{
		outcome: fixture.turn,
		calls:   &invalidCalls,
	}
	invalidRef := fixture.turn.RefV3()
	invalidRef.Digest = ""
	fact, err := modelinvoker.BuildGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		readers,
		modelinvoker.GovernedModelTurnProviderBoundaryDraftV3{
			TurnRef: invalidRef, DispatchReceipt: fixture.receipt,
			RuntimeRequest: fixture.request, Provider: fixture.provider,
			CheckedUnixNano: boundaryNow.UnixNano(),
		},
		boundaryNow,
	)
	if err == nil || fact != (modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}) {
		t.Fatalf("invalid requested Turn Ref Fact=%#v err=%v", fact, err)
	}
	if invalidCalls.Load() != 0 {
		t.Fatalf("invalid Turn Ref reached exact Reader %d times", invalidCalls.Load())
	}
	assertBoundaryRows(t, fixture.path, 0)
}

func TestBoundaryBuildAndEnsureRejectUnavailableInputsWithoutFact(t *testing.T) {
	fixture := newBoundaryFixture(t)
	defer fixture.store.Close()
	draft := modelinvoker.GovernedModelTurnProviderBoundaryDraftV3{
		TurnRef: fixture.turn.RefV3(), DispatchReceipt: fixture.receipt,
		RuntimeRequest: fixture.request, Provider: fixture.provider,
		CheckedUnixNano: boundaryNow.UnixNano(),
	}
	valid := boundaryReaders(fixture)
	var nilStore *modelsqlite.Store
	var nilACK *typedNilACKReader
	cases := map[string]modelinvoker.GovernedModelTurnProviderBoundaryVerificationReadersV3{
		"turn": func() modelinvoker.GovernedModelTurnProviderBoundaryVerificationReadersV3 {
			readers := valid
			readers.TurnHistory = nilStore
			return readers
		}(),
		"prepared-history": func() modelinvoker.GovernedModelTurnProviderBoundaryVerificationReadersV3 {
			readers := valid
			readers.PreparedHistory = nilStore
			return readers
		}(),
		"prepared-current": func() modelinvoker.GovernedModelTurnProviderBoundaryVerificationReadersV3 {
			readers := valid
			readers.PreparedCurrent = nilStore
			return readers
		}(),
		"ack": func() modelinvoker.GovernedModelTurnProviderBoundaryVerificationReadersV3 {
			readers := valid
			readers.AckHistory = nilACK
			return readers
		}(),
	}
	for name, readers := range cases {
		t.Run(name, func(t *testing.T) {
			fact, err := modelinvoker.BuildGovernedModelTurnProviderBoundaryFactV3(
				context.Background(),
				readers,
				draft,
				boundaryNow,
			)
			if err == nil || fact != (modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}) {
				t.Fatalf("unavailable reader Fact=%#v err=%v", fact, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fact, err := modelinvoker.BuildGovernedModelTurnProviderBoundaryFactV3(
		ctx,
		valid,
		draft,
		boundaryNow,
	)
	if err == nil || fact != (modelinvoker.GovernedModelTurnProviderBoundaryFactV3{}) {
		t.Fatalf("cancelled Build Fact=%#v err=%v", fact, err)
	}
	var nilRepository *typedNilBoundaryRepository
	result, err := modelinvoker.EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		nilRepository,
		fixture.fact,
		func() time.Time { return boundaryNow },
	)
	if err == nil ||
		result != (modelinvoker.GovernedModelTurnProviderBoundaryPersistenceResultV3{}) {
		t.Fatalf("typed nil repository result=%#v err=%v", result, err)
	}
	result, err = modelinvoker.EnsureOrInspectGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		fixture.store,
		fixture.fact,
		nil,
	)
	if err == nil ||
		result != (modelinvoker.GovernedModelTurnProviderBoundaryPersistenceResultV3{}) {
		t.Fatalf("nil clock result=%#v err=%v", result, err)
	}
	assertBoundaryRows(t, fixture.path, 0)
}

func TestBoundaryCurrentReaderRejectsExpiredRollbackTypedNilAndCancelled(t *testing.T) {
	fixture := newBoundaryFixture(t)
	defer fixture.store.Close()
	if _, err := fixture.store.EnsureGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		fixture.fact,
	); err != nil {
		t.Fatal(err)
	}
	for name, clock := range map[string]func() time.Time{
		"expired":  func() time.Time { return time.Unix(0, fixture.fact.ExpiresUnixNano) },
		"rollback": func() time.Time { return time.Unix(0, fixture.fact.CreatedUnixNano-1) },
	} {
		t.Run(name, func(t *testing.T) {
			adapter, err := runtimeadapter.NewModelProviderBoundaryCurrentAdapterV1(
				fixture.store,
				clock,
			)
			if err != nil {
				t.Fatal(err)
			}
			projection, err := adapter.InspectCurrentModelProviderBoundaryV1(
				context.Background(),
				fixture.fact.RuntimeRequest.ModelBoundary,
			)
			if err == nil || projection != (runtimeports.ModelProviderBoundaryCurrentProjectionV1{}) {
				t.Fatalf("projection=%#v err=%v", projection, err)
			}
		})
	}
	var typedNil *typedNilBoundaryRepository
	if _, err := runtimeadapter.NewModelProviderBoundaryCurrentAdapterV1(
		typedNil,
		func() time.Time { return boundaryNow },
	); err == nil {
		t.Fatal("typed nil repository accepted")
	}
	adapter, err := runtimeadapter.NewModelProviderBoundaryCurrentAdapterV1(
		fixture.store,
		func() time.Time { return boundaryNow.Add(time.Minute) },
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	projection, err := adapter.InspectCurrentModelProviderBoundaryV1(
		ctx,
		fixture.fact.RuntimeRequest.ModelBoundary,
	)
	if err == nil || projection != (runtimeports.ModelProviderBoundaryCurrentProjectionV1{}) {
		t.Fatalf("cancelled projection=%#v err=%v", projection, err)
	}
}

func TestBoundaryStrictJSONAndIndexedRowTamperFailClosed(t *testing.T) {
	fixture := newBoundaryFixture(t)
	if _, err := fixture.store.EnsureGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		fixture.fact,
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", fixture.path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`UPDATE governed_model_turn_v3_provider_boundary_history
		 SET provider_component_id=?
		 WHERE boundary_id=? AND revision=?`,
		"praxis.model/drift",
		fixture.fact.ID,
		fixture.fact.Revision,
	); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openBoundaryStore(t, fixture.path)
	defer reopened.Close()
	if _, err := reopened.InspectExactGovernedModelTurnProviderBoundaryV3(
		context.Background(),
		fixture.fact.RefV3(),
	); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorConflict {
		t.Fatalf("tampered row error=%v", err)
	}
	wire, err := modelinvoker.EncodeGovernedModelTurnProviderBoundaryFactV3(fixture.fact)
	if err != nil {
		t.Fatal(err)
	}
	for name, payload := range map[string][]byte{
		"unknown":  append([]byte(`{"unknown":true,`), wire[1:]...),
		"trailing": append(append([]byte(nil), wire...), []byte(` {}`)...),
		"duplicate": append(
			[]byte(`{"contract_version":"duplicate",`),
			wire[1:]...,
		),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := modelinvoker.DecodeGovernedModelTurnProviderBoundaryFactV3(payload); err == nil {
				t.Fatal("non-strict JSON accepted")
			}
		})
	}
}

func TestBoundarySchemaRejectsCurrentTableExtraIndexAndTrigger(t *testing.T) {
	for name, statement := range map[string]string{
		"current": `CREATE TABLE governed_model_turn_v3_provider_boundary_current(id TEXT)`,
		"index": `CREATE INDEX boundary_extra
			ON governed_model_turn_v3_provider_boundary_history(effect_id)`,
		"trigger": `CREATE TRIGGER boundary_trigger AFTER INSERT
			ON governed_model_turn_v3_provider_boundary_history BEGIN SELECT 1; END`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "model.db")
			store := openBoundaryStore(t, path)
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", path))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(statement); err != nil {
				_ = db.Close()
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := modelsqlite.Open(
				context.Background(),
				modelsqlite.Config{Path: path},
			)
			if reopened != nil {
				_ = reopened.Close()
			}
			if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
				modelinvoker.GovernedModelInvocationErrorConflict {
				t.Fatalf("schema drift error=%v", err)
			}
		})
	}
}

func TestBoundaryV5MigrationNeverRepairsDeclaredOrPartialSchema(t *testing.T) {
	for name, mutate := range map[string]func(*testing.T, *sql.DB){
		"ledger-missing-exact-index": func(t *testing.T, db *sql.DB) {
			t.Helper()
			if _, err := db.Exec(
				`DROP INDEX governed_model_turn_v3_provider_boundary_history_exact`,
			); err != nil {
				t.Fatal(err)
			}
		},
		"ledger-missing-table": func(t *testing.T, db *sql.DB) {
			t.Helper()
			if _, err := db.Exec(
				`DROP TABLE governed_model_turn_v3_provider_boundary_history`,
			); err != nil {
				t.Fatal(err)
			}
		},
		"no-ledger-partial-v5": func(t *testing.T, db *sql.DB) {
			t.Helper()
			if _, err := db.Exec(
				`DELETE FROM model_invoker_schema WHERE version=5`,
			); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(
				`DROP INDEX governed_model_turn_v3_provider_boundary_history_exact`,
			); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "model.db")
			store := openBoundaryStore(t, path)
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", path))
			if err != nil {
				t.Fatal(err)
			}
			mutate(t, db)
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := modelsqlite.Open(
				context.Background(),
				modelsqlite.Config{Path: path},
			)
			if reopened != nil {
				_ = reopened.Close()
			}
			if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
				modelinvoker.GovernedModelInvocationErrorConflict {
				t.Fatalf("schema repair was not rejected: %v", err)
			}
		})
	}
}

func TestBoundarySchemaRejectsCommentForgedCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model.db")
	store := openBoundaryStore(t, path)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", path))
	if err != nil {
		t.Fatal(err)
	}
	var tableDDL, indexDDL string
	if err := db.QueryRow(
		`SELECT sql FROM sqlite_master
		 WHERE type='table'
		   AND name='governed_model_turn_v3_provider_boundary_history'`,
	).Scan(&tableDDL); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.QueryRow(
		`SELECT sql FROM sqlite_master
		 WHERE type='index'
		   AND name='governed_model_turn_v3_provider_boundary_history_exact'`,
	).Scan(&indexDDL); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	forged := strings.Replace(
		tableDDL,
		"CHECK(revision > 0)",
		"CHECK(1 /* CHECK(revision > 0) */)",
		1,
	)
	if forged == tableDDL {
		_ = db.Close()
		t.Fatal("revision CHECK was not found")
	}
	if _, err := db.Exec(`DROP TABLE governed_model_turn_v3_provider_boundary_history`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(forged); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(indexDDL); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := modelsqlite.Open(
		context.Background(),
		modelsqlite.Config{Path: path},
	)
	if reopened != nil {
		_ = reopened.Close()
	}
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorConflict {
		t.Fatalf("comment-forged CHECK error=%v", err)
	}
}

func TestBoundaryV4DatabaseMigratesWithoutChangingHistoricalWinners(t *testing.T) {
	fixture := newBoundaryFixture(t)
	if err := fixture.store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", fixture.path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE governed_model_turn_v3_provider_boundary_history`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM model_invoker_schema WHERE version=5`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	var beforeTurn []byte
	if err := db.QueryRow(
		`SELECT canonical_json FROM governed_model_turn_v3_history
		 WHERE turn_id=? AND revision=?`,
		fixture.turn.ID,
		fixture.turn.Revision,
	).Scan(&beforeTurn); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	migrated := openBoundaryStore(t, fixture.path)
	gotTurn, err := migrated.InspectExactGovernedModelTurnV3(
		context.Background(),
		fixture.turn.RefV3(),
	)
	if err != nil || !reflect.DeepEqual(gotTurn, fixture.turn) {
		_ = migrated.Close()
		t.Fatalf("V4 Turn changed across V5 migration: %#v err=%v", gotTurn, err)
	}
	if _, err := migrated.EnsureGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		fixture.fact,
	); err != nil {
		_ = migrated.Close()
		t.Fatal(err)
	}
	if err := migrated.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", fmt.Sprintf("file:%s", fixture.path))
	if err != nil {
		t.Fatal(err)
	}
	var afterTurn []byte
	if err := db.QueryRow(
		`SELECT canonical_json FROM governed_model_turn_v3_history
		 WHERE turn_id=? AND revision=?`,
		fixture.turn.ID,
		fixture.turn.Revision,
	).Scan(&afterTurn); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeTurn, afterTurn) {
		_ = db.Close()
		t.Fatal("V4 Turn payload changed during V5 migration")
	}
	var ledgerRows int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM model_invoker_schema WHERE version=5`,
	).Scan(&ledgerRows); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if ledgerRows != 1 {
		t.Fatalf("V5 migration ledger rows=%d want=1", ledgerRows)
	}
	reopened := openBoundaryStore(t, fixture.path)
	defer reopened.Close()
	if _, err := reopened.InspectExactGovernedModelTurnProviderBoundaryV3(
		context.Background(),
		fixture.fact.RefV3(),
	); err != nil {
		t.Fatal(err)
	}
}

func newBoundaryFixture(t *testing.T) boundaryFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model.db")
	store := openBoundaryStore(t, path)
	prepared, current, material := invocationFixture(t)
	if _, err := store.EnsurePreparedModelInvocationV1(
		context.Background(),
		prepared,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsurePreparedModelInvocationCurrentV1(
		context.Background(),
		current,
	); err != nil {
		t.Fatal(err)
	}
	command := modelinvoker.GovernedModelTurnCommandV3{
		PreparedRef: prepared.Ref(), CurrentRef: current.Ref(),
		MaterialRef:          material.RefV2(),
		AttemptRequestDigest: prepared.UnifiedRequestDigest,
		RouteCallDigest:      material.RouteCallDigest,
		DispatchSequence:     1, ProviderAttemptOrdinal: 1,
	}
	turn, err := modelinvoker.NewPreparedGovernedModelTurnV3(
		command,
		boundaryNow.Add(-2*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsurePreparedGovernedModelTurnV3(
		context.Background(),
		turn,
	); err != nil {
		t.Fatal(err)
	}
	ack, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(
		modelinvoker.PreparedModelInvocationCommitAckV1{
			PreparedRef: turn.PreparedRef, CurrentRef: turn.CurrentRef,
			GateImplementationRef: modelinvoker.PreparedModelInvocationGateImplementationRefV1{
				Owner:           core.OwnerRef{Domain: "praxis.harness", ID: "commit-gate"},
				ContractVersion: "1.0.0", ID: "commit-gate",
				Revision: 1, Digest: digest("commit-gate"),
			},
			SurfaceBindingRef: modelinvoker.PreparedModelInvocationSurfaceBindingRefV1{
				Owner:           core.OwnerRef{Domain: "praxis.tool", ID: "surface"},
				ContractVersion: "1.0.0", ID: "surface",
				Revision: 1, Digest: digest("surface"),
			},
			CheckedUnixNano:  boundaryNow.Add(-time.Minute).UnixNano(),
			ExpiresUnixNano:  boundaryNow.Add(15 * time.Minute).UnixNano(),
			NotAfterUnixNano: current.NotAfterUnixNano,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := modelinvoker.SealPreparedModelInvocationDispatchReceiptAgainstV1(
		prepared,
		current,
		ack,
		modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1{
			PreparedRef: turn.PreparedRef, CurrentRef: turn.CurrentRef,
			AckRef: ack.Ref(), DispatchSequence: turn.DispatchSequence,
			BoundaryKind:                  modelinvoker.GovernedModelTurnProviderBoundaryKindV2,
			ProviderAttemptOrdinal:        turn.ProviderAttemptOrdinal,
			AttemptRequestDigest:          turn.AttemptRequestDigest,
			ActualToolSurfaceDigest:       prepared.ActualToolSurfaceDigest,
			ActualProviderInjectionDigest: prepared.ActualProviderInjectionDigest,
			CheckedUnixNano:               boundaryNow.UnixNano(),
		},
		boundaryNow,
	)
	if err != nil {
		t.Fatal(err)
	}
	provider := providerBinding()
	request := runtimeRequest(t, provider)
	ref, err := modelinvoker.DeriveGovernedModelTurnProviderBoundaryRefV3(
		turn, receipt, request, provider, boundaryNow.UnixNano(),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.ModelBoundary = ref.RuntimeBoundary
	fixture := boundaryFixture{
		path: path, store: store, prepared: prepared, current: current,
		material: material, turn: turn, ack: ack, receipt: receipt,
		provider: provider, request: request,
	}
	fact, err := modelinvoker.BuildGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		boundaryReaders(fixture),
		modelinvoker.GovernedModelTurnProviderBoundaryDraftV3{
			TurnRef: turn.RefV3(), DispatchReceipt: receipt,
			RuntimeRequest: request, Provider: provider,
			CheckedUnixNano: boundaryNow.UnixNano(),
		},
		boundaryNow,
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.fact = fact
	return fixture
}

func boundaryReaders(
	fixture boundaryFixture,
) modelinvoker.GovernedModelTurnProviderBoundaryVerificationReadersV3 {
	return modelinvoker.GovernedModelTurnProviderBoundaryVerificationReadersV3{
		TurnHistory: fixture.store, PreparedHistory: fixture.store,
		PreparedCurrent: fixture.store, AckHistory: exactAckReader{ack: fixture.ack},
	}
}

func buildBoundaryFact(
	t *testing.T,
	fixture boundaryFixture,
	ack modelinvoker.PreparedModelInvocationCommitAckV1,
	receipt modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1,
	request runtimeports.InspectCurrentModelProviderActualPointRequestV1,
	provider runtimeports.ProviderBindingRefV2,
) modelinvoker.GovernedModelTurnProviderBoundaryFactV3 {
	t.Helper()
	ref, err := modelinvoker.DeriveGovernedModelTurnProviderBoundaryRefV3(
		fixture.turn,
		receipt,
		request,
		provider,
		boundaryNow.UnixNano(),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.ModelBoundary = ref.RuntimeBoundary
	fact, err := modelinvoker.BuildGovernedModelTurnProviderBoundaryFactV3(
		context.Background(),
		modelinvoker.GovernedModelTurnProviderBoundaryVerificationReadersV3{
			TurnHistory: fixture.store, PreparedHistory: fixture.store,
			PreparedCurrent: fixture.store, AckHistory: exactAckReader{ack: ack},
		},
		modelinvoker.GovernedModelTurnProviderBoundaryDraftV3{
			TurnRef: fixture.turn.RefV3(), DispatchReceipt: receipt,
			RuntimeRequest: request, Provider: provider,
			CheckedUnixNano: boundaryNow.UnixNano(),
		},
		boundaryNow,
	)
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func invocationFixture(
	t *testing.T,
) (
	modelinvoker.PreparedModelInvocationFactV1,
	modelinvoker.PreparedModelInvocationCurrentProjectionV1,
	modelinvoker.InvocationMaterialV2,
) {
	t.Helper()
	strict, parallel := true, false
	call := modelinvoker.RouteCall{
		RouteID: "boundary.route",
		Invocation: upstream.InvocationContext{
			Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService,
			Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground,
		},
		Request: modelinvoker.Request{
			Model: "boundary-model",
			Input: []modelinvoker.InputItem{
				modelinvoker.MessageInput(modelinvoker.RoleUser, "read README"),
			},
			Tools: []modelinvoker.Tool{{
				Name: "workspace.read", Description: "read",
				Parameters: []byte(`{"type":"object"}`), Strict: &strict,
			}},
			ToolChoice:        modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceRequired},
			ParallelToolCalls: &parallel,
			Budget:            modelinvoker.Budget{MaxOutputTokens: 256, Timeout: time.Minute},
		},
	}
	requestTools, err := modelinvoker.DigestGovernedModelTurnRequestToolsV2(call)
	if err != nil {
		t.Fatal(err)
	}
	contextMapped, err := modelinvoker.DigestGovernedModelTurnContextV2(call)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := modelinvoker.SealPreparedModelInvocationFactV1(
		modelinvoker.PreparedModelInvocationFactV1{
			InvocationID: "boundary-invocation", InvocationDigest: digest("request"),
			UnifiedRequestDigest: digest("request"), RequestToolsDigest: requestTools,
			PreparedPlanDigest: digest("plan"), RouteDigest: digest("route"),
			ProfileDigest:                 digest("profile"),
			ActualToolSurfaceDigest:       digest("surface"),
			ActualProviderInjectionDigest: digest("provider-injection"),
			CapabilitySnapshotRef: modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{
				ContractVersion: "1.0.0", ID: "capabilities", Revision: 1,
				Digest: digest("capabilities"),
			},
			RegistrySnapshotRef: runtimeports.RegistrySnapshotRefV1{
				Owner:           core.OwnerRef{Domain: "praxis.registry", ID: "registry"},
				ContractVersion: "1.0.0", ID: "registry", Revision: 1,
				Digest: digest("registry"),
			},
			CreatedUnixNano:  boundaryNow.Add(-10 * time.Minute).UnixNano(),
			NotAfterUnixNano: boundaryNow.Add(time.Hour).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(
		modelinvoker.PreparedModelInvocationCurrentProjectionV1{
			Prepared: prepared.Ref(), CapabilitySnapshotRef: prepared.CapabilitySnapshotRef,
			RegistrySnapshotRef:           prepared.RegistrySnapshotRef,
			ActualToolSurfaceDigest:       prepared.ActualToolSurfaceDigest,
			ActualProviderInjectionDigest: prepared.ActualProviderInjectionDigest,
			CheckedUnixNano:               boundaryNow.Add(-5 * time.Minute).UnixNano(),
			ExpiresUnixNano:               boundaryNow.Add(30 * time.Minute).UnixNano(),
			NotAfterUnixNano:              prepared.NotAfterUnixNano,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	contextOwner := core.OwnerRef{Domain: "praxis.context", ID: "context"}
	toolOwner := core.OwnerRef{Domain: "praxis.tool", ID: "tool"}
	lineage, err := modelinvoker.SealInvocationMaterialSourceLineageV2(
		modelinvoker.InvocationMaterialSourceLineageV2{
			ContextFrame:             sourceRef(contextOwner, modelinvoker.InvocationMaterialContextFrameKindV2, "frame"),
			ContextMaterial:          sourceRef(contextOwner, modelinvoker.InvocationMaterialContextMaterialKindV2, "material"),
			ToolInjectionMaterial:    sourceRef(toolOwner, modelinvoker.InvocationMaterialToolInjectionMaterialKindV2, "injection"),
			ToolSurface:              sourceRef(toolOwner, modelinvoker.InvocationMaterialToolSurfaceKindV2, "surface"),
			ContextMappedInputDigest: contextMapped,
			ExpectedInjectionDigest:  prepared.ActualToolSurfaceDigest,
			CompiledToolsDigest:      digest("compiled-tools"),
			RequestToolsDigest:       requestTools,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	routeCallDigest, err := modelinvoker.DigestGovernedModelTurnRouteCallV2(call)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := modelinvoker.SealInvocationMaterialAuthorizationV2(
		modelinvoker.InvocationMaterialAuthorizationV2{
			PreparedRef: prepared.Ref(), CurrentRef: current.Ref(),
			RouteCallDigest: routeCallDigest, SourceLineage: lineage,
			ProviderInjectionRef: sourceRef(core.OwnerRef{Domain: "praxis.model", ID: "model"}, "model-source", "provider"),
			RouteRef:             sourceRef(core.OwnerRef{Domain: "praxis.model", ID: "model"}, "model-source", "route"),
			ProfileRef:           sourceRef(core.OwnerRef{Domain: "praxis.model", ID: "model"}, "model-source", "profile"),
			AuthorizedUnixNano:   boundaryNow.Add(-4 * time.Minute).UnixNano(),
			ExpiresUnixNano:      boundaryNow.Add(25 * time.Minute).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	material, err := modelinvoker.SealInvocationMaterialV2(
		modelinvoker.InvocationMaterialV2{
			PreparedRef: prepared.Ref(), UnifiedRequestDigest: prepared.UnifiedRequestDigest,
			PreparedPlanDigest: prepared.PreparedPlanDigest,
			RouteDigest:        prepared.RouteDigest, ProfileDigest: prepared.ProfileDigest,
			Authorization: authorization, Call: call,
			CreatedUnixNano: boundaryNow.Add(-3 * time.Minute).UnixNano(),
			ExpiresUnixNano: boundaryNow.Add(20 * time.Minute).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return prepared, current, material
}

func runtimeRequest(
	t *testing.T,
	provider runtimeports.ProviderBindingRefV2,
) runtimeports.InspectCurrentModelProviderActualPointRequestV1 {
	t.Helper()
	lease := &core.SandboxLeaseRef{ID: "lease-boundary", Epoch: 7}
	scope := core.ExecutionScope{
		Identity:     core.AgentIdentityRef{TenantID: "tenant", ID: "agent", Epoch: 1},
		Lineage:      core.LineageRef{ID: "lineage", PlanDigest: digest("lineage-plan")},
		Instance:     core.InstanceRef{ID: "instance", Epoch: 4},
		SandboxLease: lease, AuthorityEpoch: 3,
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := runtimeports.OperationSubjectV3{
		Kind: runtimeports.OperationScopeRunV3, ExecutionScope: scope,
		ExecutionScopeDigest: scopeDigest, RunID: "run-boundary",
		SubjectRevision: 1, CurrentProjectionRef: "run-current",
		CurrentProjectionDigest:   digest("run-current"),
		CurrentProjectionRevision: 1,
	}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	attempt := runtimeports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest, EffectID: "effect-boundary",
		IntentRevision: 1, IntentDigest: digest("intent"),
		PermitID: "permit-boundary", PermitRevision: 1,
		PermitDigest: digest("permit"), AttemptID: "runtime-attempt-boundary",
	}
	return runtimeports.InspectCurrentModelProviderActualPointRequestV1{
		Operation: operation, EffectID: attempt.EffectID,
		ExpectedEffectRevision: 3, PermitID: attempt.PermitID,
		ExpectedPermitFactRevision: 2, PermitDigest: attempt.PermitDigest,
		AdmissionDigest: digest("admission"),
		ReviewAuthorization: runtimeports.OperationReviewAuthorizationRefV4{
			ID: "review-authorization", Revision: 1, Digest: digest("review"),
		},
		Attempt: attempt, Verifier: provider, FenceDigest: digest("fence"),
		RequestedNotAfterUnixNano: boundaryNow.Add(10 * time.Minute).UnixNano(),
	}
}

func providerBinding() runtimeports.ProviderBindingRefV2 {
	return runtimeports.ProviderBindingRefV2{
		BindingSetID: "binding-set", BindingSetRevision: 1,
		ComponentID: "praxis.model/provider", ManifestDigest: digest("manifest"),
		ArtifactDigest: digest("artifact"),
		Capability:     runtimeports.ModelInvokeCapabilityV1,
	}
}

func sourceRef(
	owner core.OwnerRef,
	kind string,
	id string,
) modelinvoker.InvocationMaterialExactSourceRefV1 {
	return modelinvoker.InvocationMaterialExactSourceRefV1{
		Owner: owner, Kind: kind, ID: id, Revision: 1, Digest: digest(id),
	}
}

func digest(value string) core.Digest {
	return core.DigestBytes([]byte(value))
}

func openBoundaryStore(t *testing.T, path string) *modelsqlite.Store {
	t.Helper()
	store, err := modelsqlite.Open(
		context.Background(),
		modelsqlite.Config{Path: path},
	)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func assertBoundaryRows(t *testing.T, path string, expected int) {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM governed_model_turn_v3_provider_boundary_history`,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != expected {
		t.Fatalf("boundary rows=%d want=%d", count, expected)
	}
}
