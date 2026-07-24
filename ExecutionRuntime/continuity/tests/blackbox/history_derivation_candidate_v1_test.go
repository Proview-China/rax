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
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
	continuitysqlite "github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/sqlite"
)

func TestHistoryDerivationSQLiteBlackboxCreateReopenInspect(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "continuity.db")
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 19, 0, 0, 0, time.UTC)}
	store, err := continuitysqlite.OpenWithClock(ctx, path, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	event := testkit.TimelineEvent(1, 1, contract.TrustObservation)
	if _, _, err := store.PutProjection(ctx, event); err != nil {
		t.Fatal(err)
	}
	content := memory.New()
	manager, err := domain.NewContentManager(store, content, clock, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, _, err := manager.Put(ctx, domain.PutObjectRequest{JournalID: "journal-derived", ObjectID: "object-derived", SchemaVersion: "content/v1", Classification: "sensitive", OwnerID: "continuity", ScopeDigest: testkit.Scope().ExecutionScopeDigest, RetentionPolicyRef: "retention-1", Compression: "identity", EncryptionRef: "key-envelope-1", Data: []byte("derived output")})
	if err != nil {
		t.Fatal(err)
	}
	request := ports.CreateHistoryDerivationCandidateRequestV1{CandidateID: "derivation-1", IdempotencyKey: "request-1", Scope: testkit.Scope(), Kind: contract.HistoryDerivationSummary, Sources: []contract.HistoryDerivationSourceCoordinateV1{{EvidenceRecordRef: event.EvidenceRecordRef, ExpectedEvidenceRecordDigest: event.EvidenceRecordDigest, ExpectedProjectionDigest: event.Candidate.Digest}}, OutputObjectID: manifest.ObjectID, ExpectedOutputManifestDigest: manifest.Digest}
	controller, err := domain.NewHistoryDerivationCandidateControllerV1(store, store, store, content, testkit.HistoryDerivationOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	fact, replay, err := controller.CreateHistoryDerivationCandidateV1(ctx, request)
	if err != nil || replay {
		t.Fatalf("create=(%#v,%v,%v)", fact, replay, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = continuitysqlite.OpenWithClock(ctx, path, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.InspectHistoryDerivationCandidateV1(ctx, ports.InspectHistoryDerivationCandidateRequestV1{Ref: fact.Ref()})
	if err != nil || got.Ref() != fact.Ref() {
		t.Fatalf("reopen=(%#v,%v)", got, err)
	}
	history, err := store.InspectByEvidence(ctx, event.EvidenceRecordRef)
	if err != nil || history.Candidate.Digest != event.Candidate.Digest {
		t.Fatalf("source Event mutated=(%#v,%v)", history, err)
	}
}
