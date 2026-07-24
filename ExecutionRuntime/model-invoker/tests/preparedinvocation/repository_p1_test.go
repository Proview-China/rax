package preparedinvocation_test

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

type scriptedHistoricalRepository struct {
	errors []error
	calls  atomic.Uint64
}

func (r *scriptedHistoricalRepository) EnsurePreparedModelInvocationV1(_ context.Context, fact modelinvoker.PreparedModelInvocationFactV1) (modelinvoker.PreparedModelInvocationFactV1, error) {
	call := int(r.calls.Add(1)) - 1
	if call < len(r.errors) && r.errors[call] != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, r.errors[call]
	}
	return fact, nil
}

type scriptedCurrentRepository struct {
	errors []error
	calls  atomic.Uint64
}

func (r *scriptedCurrentRepository) EnsurePreparedModelInvocationCurrentV1(_ context.Context, current modelinvoker.PreparedModelInvocationCurrentProjectionV1) (modelinvoker.PreparedModelInvocationCurrentProjectionV1, error) {
	call := int(r.calls.Add(1)) - 1
	if call < len(r.errors) && r.errors[call] != nil {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, r.errors[call]
	}
	return current, nil
}

func (*scriptedCurrentRepository) InspectExactPreparedModelInvocationCurrentV1(context.Context, modelinvoker.PreparedModelInvocationCurrentRefV1) (modelinvoker.PreparedModelInvocationCurrentProjectionV1, error) {
	panic("canonical producer must not recover through Reader")
}

func repositoryError(kind modelinvoker.PreparedModelInvocationRepositoryErrorKindV1) error {
	return &modelinvoker.PreparedModelInvocationRepositoryErrorV1{Kind: kind, Message: string(kind)}
}

func TestRecoveryClassifiesOnlySecondEnsureOutcome(t *testing.T) {
	secondKinds := []modelinvoker.PreparedModelInvocationRepositoryErrorKindV1{
		modelinvoker.PreparedModelInvocationRepositoryErrorConflict,
		modelinvoker.PreparedModelInvocationRepositoryErrorUnavailable,
		modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate,
	}
	for _, secondKind := range secondKinds {
		t.Run("historical_"+string(secondKind), func(t *testing.T) {
			repository := &scriptedHistoricalRepository{errors: []error{
				repositoryError(modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate),
				repositoryError(secondKind),
			}}
			_, err := modelinvoker.EnsurePreparedModelInvocationFactV1(context.Background(), repository, sealedFact())
			if got := modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err); got != secondKind || repository.calls.Load() != 2 {
				t.Fatalf("second outcome = %q, calls=%d, err=%v", got, repository.calls.Load(), err)
			}
		})
		t.Run("current_"+string(secondKind), func(t *testing.T) {
			fact := sealedFact()
			repository := &scriptedCurrentRepository{errors: []error{
				repositoryError(modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate),
				repositoryError(secondKind),
			}}
			_, err := modelinvoker.EnsurePreparedModelInvocationCurrentProjectionV1(context.Background(), repository, sealedCurrent(fact))
			if got := modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err); got != secondKind || repository.calls.Load() != 2 {
				t.Fatalf("second outcome = %q, calls=%d, err=%v", got, repository.calls.Load(), err)
			}
		})
	}
}

type countingHistoricalRepository struct{ calls atomic.Uint64 }

func (r *countingHistoricalRepository) EnsurePreparedModelInvocationV1(context.Context, modelinvoker.PreparedModelInvocationFactV1) (modelinvoker.PreparedModelInvocationFactV1, error) {
	r.calls.Add(1)
	panic("repository invoked after context gate")
}

type countingCurrentRepository struct{ calls atomic.Uint64 }

func (r *countingCurrentRepository) EnsurePreparedModelInvocationCurrentV1(context.Context, modelinvoker.PreparedModelInvocationCurrentProjectionV1) (modelinvoker.PreparedModelInvocationCurrentProjectionV1, error) {
	r.calls.Add(1)
	panic("repository invoked after context gate")
}

func (r *countingCurrentRepository) InspectExactPreparedModelInvocationCurrentV1(context.Context, modelinvoker.PreparedModelInvocationCurrentRefV1) (modelinvoker.PreparedModelInvocationCurrentProjectionV1, error) {
	r.calls.Add(1)
	panic("reader invoked after context gate")
}

func TestPublicEnsureChecksContextBeforeRepository(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for name, ctx := range map[string]context.Context{"nil": nil, "canceled": canceled} {
		t.Run(name+"_historical", func(t *testing.T) {
			repository := &countingHistoricalRepository{}
			_, err := modelinvoker.EnsurePreparedModelInvocationFactV1(ctx, repository, sealedFact())
			want := modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate
			if ctx == nil {
				want = modelinvoker.PreparedModelInvocationRepositoryErrorInvalid
			}
			if got := modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err); got != want || repository.calls.Load() != 0 {
				t.Fatalf("kind=%q calls=%d err=%v", got, repository.calls.Load(), err)
			}
		})
		t.Run(name+"_current", func(t *testing.T) {
			repository := &countingCurrentRepository{}
			fact := sealedFact()
			_, err := modelinvoker.EnsurePreparedModelInvocationCurrentProjectionV1(ctx, repository, sealedCurrent(fact))
			want := modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate
			if ctx == nil {
				want = modelinvoker.PreparedModelInvocationRepositoryErrorInvalid
			}
			if got := modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err); got != want || repository.calls.Load() != 0 {
				t.Fatalf("kind=%q calls=%d err=%v", got, repository.calls.Load(), err)
			}
		})
	}
}

func TestStoreEnsureAndReadRejectNilAndCanceledContext(t *testing.T) {
	store := modelinvoker.NewInMemoryPreparedModelInvocationStoreV1()
	fact := sealedFact()
	current := sealedCurrent(fact)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	assertKind := func(t *testing.T, err error, want modelinvoker.PreparedModelInvocationRepositoryErrorKindV1) {
		t.Helper()
		if got := modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err); got != want {
			t.Fatalf("kind=%q want=%q err=%v", got, want, err)
		}
	}
	for name, ctx := range map[string]context.Context{"nil": nil, "canceled": canceled} {
		want := modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate
		if ctx == nil {
			want = modelinvoker.PreparedModelInvocationRepositoryErrorInvalid
		}
		t.Run(name+"_historical_ensure", func(t *testing.T) {
			_, err := store.EnsurePreparedModelInvocationV1(ctx, fact)
			assertKind(t, err, want)
		})
		t.Run(name+"_historical_read", func(t *testing.T) {
			_, err := store.InspectExactPreparedModelInvocationV1(ctx, fact.Ref())
			assertKind(t, err, want)
		})
		t.Run(name+"_current_ensure", func(t *testing.T) {
			_, err := store.EnsurePreparedModelInvocationCurrentV1(ctx, current)
			assertKind(t, err, want)
		})
		t.Run(name+"_current_read", func(t *testing.T) {
			_, err := store.InspectExactPreparedModelInvocationCurrentV1(ctx, current.Ref())
			assertKind(t, err, want)
		})
	}
	if stats := store.StatsV1(); stats.HistoricalRecords != 0 || stats.CurrentRecords != 0 {
		t.Fatalf("context failures created records: %#v", stats)
	}
}

type cancelOnErrCallContext struct {
	cancelAt uint64
	calls    atomic.Uint64
	done     chan struct{}
	once     sync.Once
}

func (ctx *cancelOnErrCallContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *cancelOnErrCallContext) Done() <-chan struct{}       { return ctx.done }
func (ctx *cancelOnErrCallContext) Value(any) any               { return nil }
func (ctx *cancelOnErrCallContext) Err() error {
	if ctx.calls.Add(1) >= ctx.cancelAt {
		ctx.once.Do(func() { close(ctx.done) })
		return context.Canceled
	}
	return nil
}

func newCancelOnErrCallContext(cancelAt uint64) *cancelOnErrCallContext {
	return &cancelOnErrCallContext{cancelAt: cancelAt, done: make(chan struct{})}
}

func TestExactReadersRecheckContextUnderAndAfterReadLock(t *testing.T) {
	store := modelinvoker.NewInMemoryPreparedModelInvocationStoreV1()
	fact := sealedFact()
	if _, err := store.EnsurePreparedModelInvocationV1(context.Background(), fact); err != nil {
		t.Fatal(err)
	}
	current := sealedCurrent(fact)
	if _, err := store.EnsurePreparedModelInvocationCurrentV1(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	for _, cancelAt := range []uint64{2, 3} {
		t.Run(fmt.Sprintf("historical_err_call_%d", cancelAt), func(t *testing.T) {
			_, err := store.InspectExactPreparedModelInvocationV1(newCancelOnErrCallContext(cancelAt), fact.Ref())
			if got := modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err); got != modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate {
				t.Fatalf("kind=%q err=%v", got, err)
			}
		})
		t.Run(fmt.Sprintf("current_err_call_%d", cancelAt), func(t *testing.T) {
			_, err := store.InspectExactPreparedModelInvocationCurrentV1(newCancelOnErrCallContext(cancelAt), current.Ref())
			if got := modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err); got != modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate {
				t.Fatalf("kind=%q err=%v", got, err)
			}
		})
	}
}

func TestConcurrentChangedContentHasOneCanonicalWinner(t *testing.T) {
	store := modelinvoker.NewInMemoryPreparedModelInvocationStoreV1()
	factA := sealedFact()
	draftB := draftFact()
	draftB.RouteDigest = digest("changed-route")
	factB, err := modelinvoker.SealPreparedModelInvocationFactV1(draftB)
	if err != nil || factA.ID != factB.ID || factA.Digest == factB.Digest {
		t.Fatalf("Historical contenders invalid: %#v %#v %v", factA, factB, err)
	}
	assertConcurrentWinner(t, 64, func(index int) (string, error) {
		candidate := factA
		if index%2 == 1 {
			candidate = factB
		}
		got, err := store.EnsurePreparedModelInvocationV1(context.Background(), candidate)
		return string(got.Digest), err
	}, string(factA.Digest), string(factB.Digest))
	if stats := store.StatsV1(); stats.HistoricalRecords != 1 {
		t.Fatalf("Historical records=%d", stats.HistoricalRecords)
	}

	winner, err := store.InspectExactPreparedModelInvocationV1(context.Background(), factA.Ref())
	if err != nil {
		winner, err = store.InspectExactPreparedModelInvocationV1(context.Background(), factB.Ref())
	}
	if err != nil {
		t.Fatalf("read Historical winner: %v", err)
	}
	currentA := sealedCurrent(winner)
	currentDraftB := currentA
	currentDraftB.ID, currentDraftB.Digest = "", ""
	currentDraftB.ExpiresUnixNano--
	currentB, err := modelinvoker.SealPreparedModelInvocationCurrentV1(currentDraftB)
	if err != nil || currentA.ID != currentB.ID || currentA.Digest == currentB.Digest {
		t.Fatalf("Current contenders invalid: %#v %#v %v", currentA, currentB, err)
	}
	assertConcurrentWinner(t, 64, func(index int) (string, error) {
		candidate := currentA
		if index%2 == 1 {
			candidate = currentB
		}
		got, err := store.EnsurePreparedModelInvocationCurrentV1(context.Background(), candidate)
		return string(got.Digest), err
	}, string(currentA.Digest), string(currentB.Digest))
	if stats := store.StatsV1(); stats.CurrentRecords != 1 {
		t.Fatalf("Current records=%d", stats.CurrentRecords)
	}
}

func assertConcurrentWinner(t *testing.T, workers int, invoke func(int) (string, error), candidateA, candidateB string) {
	t.Helper()
	start := make(chan struct{})
	results := make(chan struct {
		digest string
		err    error
	}, workers)
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			digest, err := invoke(index)
			results <- struct {
				digest string
				err    error
			}{digest: digest, err: err}
		}(index)
	}
	close(start)
	group.Wait()
	close(results)
	winner := ""
	successes, conflicts := 0, 0
	for result := range results {
		if result.err == nil {
			successes++
			if winner == "" {
				winner = result.digest
			}
			if result.digest != winner {
				t.Fatalf("multiple canonical winners: %q and %q", winner, result.digest)
			}
			continue
		}
		if got := modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(result.err); got != modelinvoker.PreparedModelInvocationRepositoryErrorConflict {
			t.Fatalf("loser kind=%q err=%v", got, result.err)
		}
		conflicts++
	}
	if successes != workers/2 || conflicts != workers/2 || (winner != candidateA && winner != candidateB) {
		t.Fatalf("successes=%d conflicts=%d winner=%q", successes, conflicts, winner)
	}
}

func TestStoredWireTamperIsRejectedByExactReaders(t *testing.T) {
	store := modelinvoker.NewInMemoryPreparedModelInvocationStoreV1()
	fact := sealedFact()
	if _, err := store.EnsurePreparedModelInvocationV1(context.Background(), fact); err != nil {
		t.Fatal(err)
	}
	tamperStoredWire(t, store, "historicalByID", []byte(`"route_digest":"sha256:`))
	if _, err := store.InspectExactPreparedModelInvocationV1(context.Background(), fact.Ref()); modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err) != modelinvoker.PreparedModelInvocationRepositoryErrorConflict {
		t.Fatalf("tampered Historical = %v", err)
	}

	store = modelinvoker.NewInMemoryPreparedModelInvocationStoreV1()
	if _, err := store.EnsurePreparedModelInvocationV1(context.Background(), fact); err != nil {
		t.Fatal(err)
	}
	current := sealedCurrent(fact)
	if _, err := store.EnsurePreparedModelInvocationCurrentV1(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	tamperStoredWire(t, store, "currentByID", []byte(`"actual_tool_surface_digest":"sha256:`))
	if _, err := store.InspectExactPreparedModelInvocationCurrentV1(context.Background(), current.Ref()); modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err) != modelinvoker.PreparedModelInvocationRepositoryErrorConflict {
		t.Fatalf("tampered Current = %v", err)
	}
}

func tamperStoredWire(t *testing.T, store *modelinvoker.InMemoryPreparedModelInvocationStoreV1, fieldName string, marker []byte) {
	t.Helper()
	storeValue := reflect.ValueOf(store).Elem()
	field := storeValue.FieldByName(fieldName)
	iterator := field.MapRange()
	if !iterator.Next() {
		t.Fatalf("%s is empty", fieldName)
	}
	stored := iterator.Value()
	wireField := stored.FieldByName("wire")
	wire := wireField.Bytes()
	position := bytes.Index(wire, marker)
	if position < 0 {
		t.Fatalf("marker %q missing from %s wire", marker, fieldName)
	}
	position += len(marker)
	if wire[position] == '0' {
		wire[position] = '1'
	} else {
		wire[position] = '0'
	}
}

func TestRepositoryReturnsDeepClonesWithoutAlias(t *testing.T) {
	store := modelinvoker.NewInMemoryPreparedModelInvocationStoreV1()
	fact := sealedFact()
	ensuredFact, err := store.EnsurePreparedModelInvocationV1(context.Background(), fact)
	if err != nil {
		t.Fatal(err)
	}
	ensuredFact.RouteDigest = digest("polluted")
	readFact, err := store.InspectExactPreparedModelInvocationV1(context.Background(), fact.Ref())
	if err != nil || !reflect.DeepEqual(readFact, fact) {
		t.Fatalf("Historical alias polluted store: %#v %v", readFact, err)
	}
	readFact.ProfileDigest = digest("polluted-again")
	readFactAgain, err := store.InspectExactPreparedModelInvocationV1(context.Background(), fact.Ref())
	if err != nil || !reflect.DeepEqual(readFactAgain, fact) {
		t.Fatalf("Historical read alias polluted store: %#v %v", readFactAgain, err)
	}

	current := sealedCurrent(fact)
	ensuredCurrent, err := store.EnsurePreparedModelInvocationCurrentV1(context.Background(), current)
	if err != nil {
		t.Fatal(err)
	}
	ensuredCurrent.ActualToolSurfaceDigest = digest("polluted")
	readCurrent, err := store.InspectExactPreparedModelInvocationCurrentV1(context.Background(), current.Ref())
	if err != nil || !reflect.DeepEqual(readCurrent, current) {
		t.Fatalf("Current alias polluted store: %#v %v", readCurrent, err)
	}
	readCurrent.ActualProviderInjectionDigest = digest("polluted-again")
	readCurrentAgain, err := store.InspectExactPreparedModelInvocationCurrentV1(context.Background(), current.Ref())
	if err != nil || !reflect.DeepEqual(readCurrentAgain, current) {
		t.Fatalf("Current read alias polluted store: %#v %v", readCurrentAgain, err)
	}
}
