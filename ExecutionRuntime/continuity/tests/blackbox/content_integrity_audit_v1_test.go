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

func TestContentIntegrityAuditSQLiteBlackboxCreateReopenInspect(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "continuity.db")
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 17, 0, 0, 0, time.UTC)}
	store, err := continuitysqlite.OpenWithClock(ctx, path, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	content := memory.New()
	request := testkit.ContentIntegrityAuditRequestV1()
	manager, err := domain.NewContentManager(store, content, clock, 64, nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, _, err := manager.Put(ctx, domain.PutObjectRequest{
		JournalID: request.Subjects[0].JournalID, ObjectID: request.Subjects[0].ObjectID,
		SchemaVersion: "content/v1", Classification: "sensitive", OwnerID: "continuity",
		ScopeDigest: request.Scope.ExecutionScopeDigest, RetentionPolicyRef: "retention-1",
		Compression: "identity", EncryptionRef: "key-envelope-1", Data: []byte("durable-content-integrity-audit"),
	})
	if err != nil {
		t.Fatal(err)
	}
	request.Subjects[0].ExpectedManifestDigest = manifest.Digest
	controller, err := domain.NewContentIntegrityAuditControllerV1(store, store, content, testkit.ContentIntegrityAuditOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	fact, replay, err := controller.CreateContentIntegrityAuditV1(ctx, request)
	if err != nil || replay || fact.Status != contract.ContentIntegrityStatusHealthy {
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
	inspected, err := store.InspectContentIntegrityAuditV1(ctx, ports.InspectContentIntegrityAuditRequestV1{Ref: fact.Ref()})
	if err != nil || inspected.Ref() != fact.Ref() || inspected.Findings[0].Classification != contract.ContentIntegrityHealthy {
		t.Fatalf("reopen inspect = (%#v,%v)", inspected, err)
	}
}
