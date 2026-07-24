package preparedinvocation_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestHistoricalAndCurrentCreateOnceExactRead(t *testing.T) {
	store := modelinvoker.NewInMemoryPreparedModelInvocationStoreV1()
	fact := sealedFact()
	for index := 0; index < 2; index++ {
		got, err := modelinvoker.EnsurePreparedModelInvocationFactV1(context.Background(), store, fact)
		if err != nil || !reflect.DeepEqual(got, fact) {
			t.Fatalf("Historical Ensure %d = %#v, %v", index, got, err)
		}
	}
	readFact, err := store.InspectExactPreparedModelInvocationV1(context.Background(), fact.Ref())
	if err != nil || !reflect.DeepEqual(readFact, fact) {
		t.Fatalf("Historical read = %#v, %v", readFact, err)
	}

	current := sealedCurrent(fact)
	for index := 0; index < 2; index++ {
		got, err := modelinvoker.EnsurePreparedModelInvocationCurrentProjectionV1(context.Background(), store, current)
		if err != nil || !reflect.DeepEqual(got, current) {
			t.Fatalf("Current Ensure %d = %#v, %v", index, got, err)
		}
	}
	readCurrent, err := store.InspectExactPreparedModelInvocationCurrentV1(context.Background(), current.Ref())
	if err != nil || !reflect.DeepEqual(readCurrent, current) {
		t.Fatalf("Current read = %#v, %v", readCurrent, err)
	}
	stats := store.StatsV1()
	if stats.HistoricalEnsureCalls != 2 || stats.HistoricalRecords != 1 || stats.CurrentEnsureCalls != 2 || stats.CurrentRecords != 1 {
		t.Fatalf("stats = %#v", stats)
	}
}

func TestRepositoryRejectsSameIdentityChangedContent(t *testing.T) {
	store := modelinvoker.NewInMemoryPreparedModelInvocationStoreV1()
	fact := sealedFact()
	if _, err := store.EnsurePreparedModelInvocationV1(context.Background(), fact); err != nil {
		t.Fatal(err)
	}
	changedDraft := draftFact()
	changedDraft.RouteDigest = digest("another-route")
	changed, err := modelinvoker.SealPreparedModelInvocationFactV1(changedDraft)
	if err != nil || changed.ID != fact.ID || changed.Digest == fact.Digest {
		t.Fatalf("changed fact = %#v, %v", changed, err)
	}
	if _, err := store.EnsurePreparedModelInvocationV1(context.Background(), changed); modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err) != modelinvoker.PreparedModelInvocationRepositoryErrorConflict {
		t.Fatalf("Historical conflict = %v", err)
	}

	current := sealedCurrent(fact)
	if _, err := store.EnsurePreparedModelInvocationCurrentV1(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	changedCurrentDraft := current
	changedCurrentDraft.ID, changedCurrentDraft.Digest = "", ""
	changedCurrentDraft.ExpiresUnixNano--
	changedCurrent, err := modelinvoker.SealPreparedModelInvocationCurrentV1(changedCurrentDraft)
	if err != nil || changedCurrent.ID != current.ID || changedCurrent.Digest == current.Digest {
		t.Fatalf("changed Current = %#v, %v", changedCurrent, err)
	}
	if _, err := store.EnsurePreparedModelInvocationCurrentV1(context.Background(), changedCurrent); modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err) != modelinvoker.PreparedModelInvocationRepositoryErrorConflict {
		t.Fatalf("Current conflict = %v", err)
	}
}

func TestRepositoryAuthoritativeAbsentAndCurrentRequiresHistorical(t *testing.T) {
	store := &modelinvoker.InMemoryPreparedModelInvocationStoreV1{}
	fact := sealedFact()
	if _, err := store.InspectExactPreparedModelInvocationV1(context.Background(), fact.Ref()); modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err) != modelinvoker.PreparedModelInvocationRepositoryErrorAuthoritativeAbsent {
		t.Fatalf("Historical absent = %v", err)
	}
	current := sealedCurrent(fact)
	if _, err := store.InspectExactPreparedModelInvocationCurrentV1(context.Background(), current.Ref()); modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err) != modelinvoker.PreparedModelInvocationRepositoryErrorAuthoritativeAbsent {
		t.Fatalf("Current absent = %v", err)
	}
	if _, err := store.EnsurePreparedModelInvocationCurrentV1(context.Background(), current); modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err) != modelinvoker.PreparedModelInvocationRepositoryErrorConflict {
		t.Fatalf("orphan Current = %v", err)
	}
}

type lostHistoricalReply struct {
	inner modelinvoker.PreparedModelInvocationRepositoryV1
	calls atomic.Uint64
}

func (r *lostHistoricalReply) EnsurePreparedModelInvocationV1(ctx context.Context, fact modelinvoker.PreparedModelInvocationFactV1) (modelinvoker.PreparedModelInvocationFactV1, error) {
	call := r.calls.Add(1)
	ensured, err := r.inner.EnsurePreparedModelInvocationV1(ctx, fact)
	if err != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, err
	}
	if call == 1 {
		return modelinvoker.PreparedModelInvocationFactV1{}, &modelinvoker.PreparedModelInvocationRepositoryErrorV1{Kind: modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate, Message: "lost reply"}
	}
	return ensured, nil
}

type lostCurrentReply struct {
	inner modelinvoker.PreparedModelInvocationCurrentRepositoryV1
	calls atomic.Uint64
}

func (r *lostCurrentReply) EnsurePreparedModelInvocationCurrentV1(ctx context.Context, current modelinvoker.PreparedModelInvocationCurrentProjectionV1) (modelinvoker.PreparedModelInvocationCurrentProjectionV1, error) {
	call := r.calls.Add(1)
	ensured, err := r.inner.EnsurePreparedModelInvocationCurrentV1(ctx, current)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, err
	}
	if call == 1 {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, &modelinvoker.PreparedModelInvocationRepositoryErrorV1{Kind: modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate, Message: "lost reply"}
	}
	return ensured, nil
}

func (r *lostCurrentReply) InspectExactPreparedModelInvocationCurrentV1(ctx context.Context, ref modelinvoker.PreparedModelInvocationCurrentRefV1) (modelinvoker.PreparedModelInvocationCurrentProjectionV1, error) {
	return r.inner.InspectExactPreparedModelInvocationCurrentV1(ctx, ref)
}

func TestAtomicEnsureLostReplyRetriesSameCanonicalOnce(t *testing.T) {
	store := modelinvoker.NewInMemoryPreparedModelInvocationStoreV1()
	fact := sealedFact()
	historical := &lostHistoricalReply{inner: store}
	gotFact, err := modelinvoker.EnsurePreparedModelInvocationFactV1(context.Background(), historical, fact)
	if err != nil || gotFact.Ref() != fact.Ref() || historical.calls.Load() != 2 || store.StatsV1().HistoricalRecords != 1 {
		t.Fatalf("Historical lost reply = %#v/%v calls=%d stats=%#v", gotFact, err, historical.calls.Load(), store.StatsV1())
	}
	current := sealedCurrent(fact)
	currentRepository := &lostCurrentReply{inner: store}
	gotCurrent, err := modelinvoker.EnsurePreparedModelInvocationCurrentProjectionV1(context.Background(), currentRepository, current)
	if err != nil || gotCurrent.Ref() != current.Ref() || currentRepository.calls.Load() != 2 || store.StatsV1().CurrentRecords != 1 {
		t.Fatalf("Current lost reply = %#v/%v calls=%d stats=%#v", gotCurrent, err, currentRepository.calls.Load(), store.StatsV1())
	}
}

type typedNilHistoricalRepository struct{}

func (*typedNilHistoricalRepository) EnsurePreparedModelInvocationV1(context.Context, modelinvoker.PreparedModelInvocationFactV1) (modelinvoker.PreparedModelInvocationFactV1, error) {
	panic("typed-nil repository invoked")
}

type typedNilCurrentRepository struct{}

func (*typedNilCurrentRepository) EnsurePreparedModelInvocationCurrentV1(context.Context, modelinvoker.PreparedModelInvocationCurrentProjectionV1) (modelinvoker.PreparedModelInvocationCurrentProjectionV1, error) {
	panic("typed-nil repository invoked")
}

func (*typedNilCurrentRepository) InspectExactPreparedModelInvocationCurrentV1(context.Context, modelinvoker.PreparedModelInvocationCurrentRefV1) (modelinvoker.PreparedModelInvocationCurrentProjectionV1, error) {
	panic("typed-nil repository invoked")
}

func TestTypedNilRepositoriesFailClosed(t *testing.T) {
	var historical *typedNilHistoricalRepository
	if _, err := modelinvoker.EnsurePreparedModelInvocationFactV1(context.Background(), historical, sealedFact()); modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err) != modelinvoker.PreparedModelInvocationRepositoryErrorInvalid {
		t.Fatalf("typed-nil Historical = %v", err)
	}
	var current *typedNilCurrentRepository
	if _, err := modelinvoker.EnsurePreparedModelInvocationCurrentProjectionV1(context.Background(), current, sealedCurrent(sealedFact())); modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err) != modelinvoker.PreparedModelInvocationRepositoryErrorInvalid {
		t.Fatalf("typed-nil Current = %v", err)
	}
}

type fixedHistoricalRepository struct {
	fact modelinvoker.PreparedModelInvocationFactV1
	err  error
}

func (r fixedHistoricalRepository) EnsurePreparedModelInvocationV1(context.Context, modelinvoker.PreparedModelInvocationFactV1) (modelinvoker.PreparedModelInvocationFactV1, error) {
	return r.fact, r.err
}

func TestCanonicalProducerRejectsDifferentValidReturn(t *testing.T) {
	want := sealedFact()
	draft := draftFact()
	draft.InvocationID = "invocation-2"
	draft.InvocationDigest = digest("request-2")
	draft.UnifiedRequestDigest = draft.InvocationDigest
	other, err := modelinvoker.SealPreparedModelInvocationFactV1(draft)
	if err != nil {
		t.Fatal(err)
	}
	_, err = modelinvoker.EnsurePreparedModelInvocationFactV1(context.Background(), fixedHistoricalRepository{fact: other}, want)
	if modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err) != modelinvoker.PreparedModelInvocationRepositoryErrorConflict {
		t.Fatalf("different valid return = %v", err)
	}
}

func TestConcurrentCreateOnceAndConflicts(t *testing.T) {
	store := modelinvoker.NewInMemoryPreparedModelInvocationStoreV1()
	fact := sealedFact()
	const workers = 64
	var group sync.WaitGroup
	errorsChannel := make(chan error, workers)
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			got, err := store.EnsurePreparedModelInvocationV1(context.Background(), fact)
			if err == nil && got.Ref() != fact.Ref() {
				err = errors.New("different exact ref")
			}
			errorsChannel <- err
		}()
	}
	group.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	if stats := store.StatsV1(); stats.HistoricalEnsureCalls != workers || stats.HistoricalRecords != 1 {
		t.Fatalf("concurrent stats = %#v", stats)
	}

	current := sealedCurrent(fact)
	errorsChannel = make(chan error, workers)
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			got, err := store.EnsurePreparedModelInvocationCurrentV1(context.Background(), current)
			if err == nil && got.Ref() != current.Ref() {
				err = errors.New("different Current exact ref")
			}
			errorsChannel <- err
		}()
	}
	group.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	if stats := store.StatsV1(); stats.CurrentEnsureCalls != workers || stats.CurrentRecords != 1 {
		t.Fatalf("concurrent Current stats = %#v", stats)
	}
}

func TestCanceledContextIsIndeterminateAndDoesNotCreate(t *testing.T) {
	store := modelinvoker.NewInMemoryPreparedModelInvocationStoreV1()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := store.EnsurePreparedModelInvocationV1(ctx, sealedFact())
	if modelinvoker.PreparedModelInvocationRepositoryErrorKindOfV1(err) != modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate || !errors.Is(err, context.Canceled) || store.StatsV1().HistoricalRecords != 0 {
		t.Fatalf("canceled Ensure = %v, stats=%#v", err, store.StatsV1())
	}
	if core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("canceled Ensure misclassified: %v", err)
	}
}
