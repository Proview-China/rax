package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceCurrentHistoryCASAndLostReply(t *testing.T) {
	ctx := context.Background()
	store, err := OpenWithClock(ctx, t.TempDir()+"/sandbox.db", func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	view := testkit.WorkspaceView()
	createdView, err := store.CreateWorkspaceViewV1(ctx, view)
	if err != nil || !contract.SameRef(createdView.Meta.Ref(), view.Meta.Ref()) {
		t.Fatalf("create view: %#v %v", createdView, err)
	}
	change := contract.WorkspaceChange{Kind: contract.WorkspaceAdd, Path: "src/generated/store.go", BlobRef: refPointerV1(testkit.Ref("blob-store"))}
	set, err := kernel.StageWorkspaceChangeSet(testkit.FixedNow, testkit.FixedNow.Add(time.Hour), "changeset-store", view, []contract.WorkspaceChange{change})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWorkspaceChangeSetV1(ctx, set); err != nil {
		t.Fatal(err)
	}
	reservation := testkit.Reservation(contract.EffectWorkspaceCommit, 1, "workspace-store")
	observation := testkit.Observation(reservation, 1, "workspace-store")
	inspection := testkit.Inspection(reservation, observation, contract.DispositionConfirmedApplied, "workspace-store")
	inspection.WorkspaceChangeSetRef = refPointerV1(set.Meta.Ref())
	result := testkit.Result(reservation, inspection, contract.DomainResultPayload{WorkspaceChangeSetRef: refPointerV1(set.Meta.Ref())}, "workspace-store")
	settlement := testkit.Settlement(result, "workspace-store")
	committed, err := contract.ApplyWorkspaceCommitSettlement(testkit.FixedNow, set, result, settlement, "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSwapWorkspaceChangeSetV1(ctx, set.Meta.Ref(), committed); err != nil {
		t.Fatal(err)
	}
	// Lost CAS reply: exact replay observes the same winner and cannot append ABA.
	if err := store.CompareAndSwapWorkspaceChangeSetV1(ctx, set.Meta.Ref(), committed); err != nil {
		t.Fatalf("lost reply replay: %v", err)
	}
	current, err := store.InspectWorkspaceChangeSetCurrentV1(ctx, committed.Meta.Ref())
	if err != nil || current.State != contract.ChangeSetCommitted {
		t.Fatalf("current: %#v %v", current, err)
	}
	historical, err := store.InspectWorkspaceChangeSetHistoryV1(ctx, set.Meta.Ref())
	if err != nil || historical.State != contract.ChangeSetStaged {
		t.Fatalf("historical: %#v %v", historical, err)
	}
	if _, err := store.InspectWorkspaceChangeSetCurrentV1(ctx, set.Meta.Ref()); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("stale current ref error=%v", err)
	}
	ownerCurrent, err := store.InspectWorkspaceChangeSetOwnerCurrentByIDV1(ctx, set.Meta.ID)
	if err != nil || !contract.SameRef(ownerCurrent.Meta.Ref(), committed.Meta.Ref()) {
		t.Fatalf("Owner recovery current: %#v %v", ownerCurrent, err)
	}
	drift, err := contract.ApplyWorkspaceCommitSettlement(testkit.FixedNow, set, result, settlement, "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompareAndSwapWorkspaceChangeSetV1(ctx, set.Meta.Ref(), drift); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("different CAS winner error=%v", err)
	}
}

func refPointerV1(value contract.Ref) *contract.Ref { return &value }
