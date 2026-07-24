package sqlite_test

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func TestSQLiteArtifactRelationReopenExactIndexesAndNoAlias(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "continuity.db")
	store := openStore(t, path)
	now := time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)
	source := testkit.ArtifactSourceProjectionV1("evidence-1", "evidence-digest-1")
	fact, err := contract.NewArtifactRelationFactV1("relation-1", "request-1", testkit.Scope(), testkit.ArtifactRelationOwnerV1(), source, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, replay, err := store.CreateArtifactRelationFactV1(ctx, fact); err != nil || replay {
		t.Fatalf("create = (%v,%v)", replay, err)
	}
	if _, replay, err := store.CreateArtifactRelationFactV1(ctx, fact); err != nil || !replay {
		t.Fatalf("lost create reply = (%v,%v)", replay, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openStore(t, path)
	defer store.Close()
	inspected, err := store.InspectArtifactRelationV1(ctx, ports.InspectArtifactRelationRequestV1{Ref: fact.Ref()})
	if err != nil || inspected.Ref() != fact.Ref() {
		t.Fatalf("reopen exact inspect = (%#v,%v)", inspected, err)
	}
	byArtifact, err := store.ListArtifactRelationsV1(ctx, ports.ListArtifactRelationsRequestV1{ArtifactFactRef: source.Artifact.ArtifactFactRef})
	if err != nil || len(byArtifact) != 1 || byArtifact[0].Ref() != fact.Ref() {
		t.Fatalf("artifact index = (%#v,%v)", byArtifact, err)
	}
	byRelated, err := store.ListRelatedArtifactRelationsV1(ctx, ports.ListRelatedArtifactRelationsRequestV1{RelatedFactRef: source.RelatedFactRef})
	if err != nil || len(byRelated) != 1 || byRelated[0].Ref() != fact.Ref() {
		t.Fatalf("related index = (%#v,%v)", byRelated, err)
	}
	byArtifact[0].SourceProjection.Artifact.ParentRevisionRef.Digest = "mutated"
	again, err := store.InspectArtifactRelationV1(ctx, ports.InspectArtifactRelationRequestV1{Ref: fact.Ref()})
	if err != nil || again.SourceProjection.Artifact.ParentRevisionRef.Digest == "mutated" {
		t.Fatal("SQLite Artifact Relation aliases decoded caller memory")
	}
}

func TestSQLiteArtifactRelationConcurrentDifferentContentSingleWinner(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, filepath.Join(t.TempDir(), "continuity.db"))
	defer store.Close()
	now := time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)
	var winners atomic.Int32
	var conflicts atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			source := testkit.ArtifactSourceProjectionV1("evidence-1", "evidence-digest-1")
			source.Artifact.StorageDigest = decimal(i)
			fact, err := contract.NewArtifactRelationFactV1("relation-race", "request-race", testkit.Scope(), testkit.ArtifactRelationOwnerV1(), source, now)
			if err != nil {
				unexpected.Add(1)
				return
			}
			_, replay, err := store.CreateArtifactRelationFactV1(ctx, fact)
			switch {
			case err == nil && !replay:
				winners.Add(1)
			case contract.HasCode(err, contract.ErrRevisionConflict):
				conflicts.Add(1)
			default:
				unexpected.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if winners.Load() != 1 || conflicts.Load() != 63 || unexpected.Load() != 0 {
		t.Fatalf("SQLite create-once closure winners=%d conflicts=%d unexpected=%d", winners.Load(), conflicts.Load(), unexpected.Load())
	}
}
