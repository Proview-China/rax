package conformance_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/framestore"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

func TestFrameStoreImplementsOwnerReadPortsV1(t *testing.T) {
	var _ contextports.ContextFrameExactCurrentReaderV1 = (*framestore.SQLiteV1)(nil)
	var _ contextports.ContextParentFrameSourceBindingReaderV1 = (*framestore.SQLiteV1)(nil)
	var _ contextports.ContextFrameMetadataReaderV1 = (*framestore.SQLiteV1)(nil)
	var _ contextports.ContextManifestMetadataReaderV1 = (*framestore.SQLiteV1)(nil)
	var _ contextports.ContextGenerationMetadataReaderV1 = (*framestore.SQLiteV1)(nil)
	var _ contextports.ContextGenerationCurrentPointerReaderV1 = (*framestore.SQLiteV1)(nil)

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
	if _, err := store.CommitCurrentV1(context.Background(), "conformance-commit", fixture.State, nil, fixture.Now.UnixNano()); err != nil {
		t.Fatal(err)
	}
	if got := store.ContextOwnerRefV1(); got != fixture.Owner {
		t.Fatalf("Owner binding drifted: %+v", got)
	}
}
