package failure_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/framestore"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
)

func TestFrameStoreStagedFailureRollsBackAndLostReplyRequiresInspectV1(t *testing.T) {
	fixture, err := testfixture.NewFrameStoreFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	store, err := framestore.OpenSQLiteV1(context.Background(), framestore.SQLiteConfigV1{
		Path: t.TempDir() + "/frame.db", Owner: fixture.Owner, Clock: func() time.Time { return fixture.Now },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.FailNextStageForTestingV1()
	if _, err := store.CommitCurrentV1(context.Background(), "stage-failure", fixture.State, nil, fixture.Now.UnixNano()); !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("staged failure classification drifted: %v", err)
	}
	if _, err := store.InspectCommitV1(context.Background(), "stage-failure"); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("rolled-back operation became visible: %v", err)
	}
	store.LoseNextReplyForTestingV1()
	if _, err := store.CommitCurrentV1(context.Background(), "lost-reply", fixture.State, nil, fixture.Now.UnixNano()); !errors.Is(err, contract.ErrUnknown) {
		t.Fatalf("lost reply classification drifted: %v", err)
	}
	if _, err := store.CommitCurrentV1(context.Background(), "lost-reply", fixture.State, nil, fixture.Now.UnixNano()); !errors.Is(err, contract.ErrInspectOnly) {
		t.Fatalf("lost reply retry must be inspect-only: %v", err)
	}
	if _, err := store.InspectCommitV1(context.Background(), "lost-reply"); err != nil {
		t.Fatalf("inspect did not recover exact committed operation: %v", err)
	}
}
