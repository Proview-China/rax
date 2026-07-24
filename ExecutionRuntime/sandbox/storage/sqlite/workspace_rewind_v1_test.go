package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceRewindCompositionV1DurableIdempotentAndHistorical(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedNow
	store, err := OpenWithClock(ctx, t.TempDir()+"/sandbox.db", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	request := seedWorkspaceRewindV1(t, ctx, store)
	composer, err := kernel.NewWorkspaceRewindComposerV1(store, store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	first, err := composer.ComposeWorkspaceRewindV1(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := composer.ComposeWorkspaceRewindV1(ctx, request)
	if err != nil || replay.Composition.Meta.Ref() != first.Composition.Meta.Ref() || replay.ChangeSet.Meta.Ref() != first.ChangeSet.Meta.Ref() {
		t.Fatalf("idempotent replay = %+v err=%v", replay, err)
	}
	if len(first.ChangeSet.Changes) != 1 || first.ChangeSet.Changes[0].Path != "src/generated/keep.go" {
		t.Fatalf("planned ChangeSet did not preserve only the keep selection: %+v", first.ChangeSet.Changes)
	}

	now = request.RequestedNotAfter.Add(time.Second)
	historical, err := composer.InspectWorkspaceRewindV1(ctx, request)
	if err != nil || historical.Composition.Meta.Ref() != first.Composition.Meta.Ref() || historical.ChangeSet.Meta.Ref() != first.ChangeSet.Meta.Ref() {
		t.Fatalf("expired exact history must remain readable: %+v err=%v", historical, err)
	}
	if _, err := composer.ComposeWorkspaceRewindV1(ctx, driftWorkspaceRewindRequestV1(t, request)); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("same request identity with changed content error=%v", err)
	}
}

func TestWorkspaceRewindCompositionV1LostReplyAndConcurrentSingleWinner(t *testing.T) {
	ctx := context.Background()
	store, err := OpenWithClock(ctx, t.TempDir()+"/sandbox.db", func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	request := seedWorkspaceRewindV1(t, ctx, store)
	lost := &workspaceRewindLostReplyRepositoryV1{WorkspaceRewindCompositionRepositoryV1: store, lose: true}
	composer, err := kernel.NewWorkspaceRewindComposerV1(store, lost, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := composer.ComposeWorkspaceRewindV1(ctx, request)
	if err != nil || lost.creates != 1 {
		t.Fatalf("lost reply recovery = %+v err=%v creates=%d", recovered, err, lost.creates)
	}

	const workers = 64
	results := make(chan ports.WorkspaceRewindCompositionResultV1, workers)
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			result, composeErr := composer.ComposeWorkspaceRewindV1(ctx, request)
			results <- result
			errs <- composeErr
		}()
	}
	group.Wait()
	close(results)
	close(errs)
	for composeErr := range errs {
		if composeErr != nil {
			t.Fatalf("concurrent replay error=%v", composeErr)
		}
	}
	for result := range results {
		if result.Composition.Meta.Ref() != recovered.Composition.Meta.Ref() || result.ChangeSet.Meta.Ref() != recovered.ChangeSet.Meta.Ref() {
			t.Fatalf("concurrent replay returned another winner: %+v", result)
		}
	}
	if lost.creates != 1 {
		t.Fatalf("completed replay called create again: %d", lost.creates)
	}
}

func TestWorkspaceRewindCompositionV1RepositoryCannotBypassOwnerClosure(t *testing.T) {
	ctx := context.Background()
	store, err := OpenWithClock(ctx, t.TempDir()+"/sandbox.db", func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	request := seedWorkspaceRewindV1(t, ctx, store)
	view, err := store.InspectWorkspaceViewCurrentV1(ctx, request.SourceWorkspaceViewRef)
	if err != nil {
		t.Fatal(err)
	}
	drop, err := store.InspectWorkspaceChangeSetHistoryV1(ctx, request.DropChangeSetRefs[0])
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, request.RequestedNotAfter, request.PlannedChangeSetID, view, drop.Changes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWorkspaceChangeSetV1(ctx, wrong); err != nil {
		t.Fatal(err)
	}
	fact, err := contract.NewWorkspaceRewindCompositionFactV1(testkit.FixedNow, request.RequestedNotAfter, request, view, wrong)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWorkspaceRewindCompositionV1(ctx, fact); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("direct repository accepted a planned ChangeSet that differs from keep/drop closure: %v", err)
	}
}

type workspaceRewindLostReplyRepositoryV1 struct {
	ports.WorkspaceRewindCompositionRepositoryV1
	mu      sync.Mutex
	lose    bool
	creates int
}

func (r *workspaceRewindLostReplyRepositoryV1) CreateWorkspaceRewindCompositionV1(ctx context.Context, fact contract.WorkspaceRewindCompositionFactV1) (contract.WorkspaceRewindCompositionFactV1, error) {
	r.mu.Lock()
	r.creates++
	lose := r.lose
	r.lose = false
	r.mu.Unlock()
	created, err := r.WorkspaceRewindCompositionRepositoryV1.CreateWorkspaceRewindCompositionV1(ctx, fact)
	if err == nil && lose {
		return contract.WorkspaceRewindCompositionFactV1{}, errors.New("injected workspace rewind composition reply loss")
	}
	return created, err
}

func seedWorkspaceRewindV1(t *testing.T, ctx context.Context, store *Store) contract.ComposeWorkspaceRewindRequestV1 {
	t.Helper()
	view := testkit.WorkspaceView()
	view.BaseRevision = testkit.Ref("workspace-base-revision").Digest
	if _, err := store.CreateWorkspaceViewV1(ctx, view); err != nil {
		t.Fatal(err)
	}
	keep, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, testkit.FixedNow.Add(time.Hour), "changeset-keep", view, []contract.WorkspaceChange{{
		Kind: contract.WorkspaceAdd, Path: "src/generated/keep.go", BlobRef: refPointerV1(testkit.Ref("blob-keep")),
	}})
	if err != nil {
		t.Fatal(err)
	}
	drop, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, testkit.FixedNow.Add(time.Hour), "changeset-drop", view, []contract.WorkspaceChange{{
		Kind: contract.WorkspaceAdd, Path: "src/generated/drop.go", BlobRef: refPointerV1(testkit.Ref("blob-drop")),
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, set := range []contract.WorkspaceChangeSet{keep, drop} {
		if _, err := store.CreateWorkspaceChangeSetV1(ctx, set); err != nil {
			t.Fatal(err)
		}
	}
	request, err := contract.SealComposeWorkspaceRewindRequestV1(contract.ComposeWorkspaceRewindRequestV1{
		RequestID:               "rewind-request",
		IdempotencyKey:          "rewind-idempotency",
		PlannedChangeSetID:      "changeset-rewind-planned",
		SourceWorkspaceViewRef:  view.Meta.Ref(),
		ExpectedBaseRevision:    view.BaseRevision,
		ExpectedFileScopeDigest: view.FileScopeDigest,
		KeepChangeSetRefs:       []contract.Ref{keep.Meta.Ref()},
		DropChangeSetRefs:       []contract.Ref{drop.Meta.Ref()},
		RequestedNotAfter:       testkit.FixedNow.Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func driftWorkspaceRewindRequestV1(t *testing.T, request contract.ComposeWorkspaceRewindRequestV1) contract.ComposeWorkspaceRewindRequestV1 {
	t.Helper()
	request.IdempotencyKey = "rewind-idempotency-drift"
	sealed, err := contract.SealComposeWorkspaceRewindRequestV1(request)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

var _ ports.WorkspaceRewindCompositionRepositoryV1 = (*workspaceRewindLostReplyRepositoryV1)(nil)
