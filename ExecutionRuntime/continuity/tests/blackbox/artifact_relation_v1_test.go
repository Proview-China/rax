package blackbox_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	continuitysqlite "github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/sqlite"
)

type blackboxArtifactSourceReaderV1 struct {
	source contract.ArtifactRelationSourceProjectionV1
}

func (r blackboxArtifactSourceReaderV1) InspectArtifactRelationSourceV1(context.Context, ports.ArtifactRelationSourceRequestV1) (contract.ArtifactRelationSourceProjectionV1, error) {
	return r.source.Clone(), nil
}

func TestArtifactRelationSQLiteBlackboxCreateReopenInspect(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "continuity.db")
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)}
	store, err := continuitysqlite.OpenWithClock(ctx, path, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	timeline, err := domain.NewReferenceTimeline(store, clock, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	candidate := testkit.Candidate(1, 1, contract.TrustObservation)
	source := testkit.ArtifactSourceProjectionV1(candidate.Evidence.RecordRef, candidate.Evidence.RecordDigest)
	candidate.ObjectRefs = []string{source.Artifact.ArtifactFactRef.ID, source.RelatedFactRef.ID}
	candidate.Digest, _ = candidate.CanonicalDigest()
	if _, _, err := timeline.Project(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	controller, err := domain.NewArtifactRelationControllerV1(store, store, blackboxArtifactSourceReaderV1{source: source}, testkit.ArtifactRelationOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	fact, replay, err := controller.CreateArtifactRelationV1(ctx, testkit.ArtifactRelationRequestV1(source))
	if err != nil || replay {
		t.Fatalf("create = (%#v,%v,%v)", fact, replay, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = continuitysqlite.OpenWithClock(ctx, path, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	inspected, err := store.InspectArtifactRelationV1(ctx, ports.InspectArtifactRelationRequestV1{Ref: fact.Ref()})
	if err != nil || inspected.Ref() != fact.Ref() {
		t.Fatalf("reopen inspect = (%#v,%v)", inspected, err)
	}
	if _, err := store.InspectByEvidence(ctx, source.EvidenceRecordRef); err != nil {
		t.Fatalf("origin Timeline Event was not durable: %v", err)
	}
}
