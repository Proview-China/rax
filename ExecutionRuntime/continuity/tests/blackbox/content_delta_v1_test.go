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

func TestContentDeltaSQLiteBlackboxCreateReopenInspect(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "continuity.db")
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC)}
	store, err := continuitysqlite.OpenWithClock(ctx, path, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	content := memory.New()
	manager, err := domain.NewContentManager(store, content, clock, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	put := func(journalID, objectID, data string) contract.ObjectManifest {
		manifest, _, err := manager.Put(ctx, domain.PutObjectRequest{
			JournalID: journalID, ObjectID: objectID, SchemaVersion: "content/v1",
			Classification: "sensitive", OwnerID: "continuity", ScopeDigest: testkit.Scope().ExecutionScopeDigest,
			RetentionPolicyRef: "retention-1", Compression: "identity", EncryptionRef: "key-envelope-1", Data: []byte(data),
		})
		if err != nil {
			t.Fatal(err)
		}
		return manifest
	}
	base := put("journal-base", "object-base", "AAAABBBBCCCC")
	target := put("journal-target", "object-target", "AAAABBBBDDDD")
	request := ports.CreateContentDeltaRequestV1{
		DeltaID: "content-delta-1", IdempotencyKey: "content-delta-request-1", Scope: testkit.Scope(),
		BaseObjectID: base.ObjectID, ExpectedBaseManifestDigest: base.Digest,
		TargetObjectID: target.ObjectID, ExpectedTargetManifestDigest: target.Digest,
	}
	controller, err := domain.NewContentDeltaControllerV1(store, store, content, testkit.ContentDeltaOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	fact, replay, err := controller.CreateContentDeltaV1(ctx, request)
	if err != nil || replay || fact.SharedBytes != 8 {
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
	inspected, err := store.InspectContentDeltaV1(ctx, ports.InspectContentDeltaRequestV1{Ref: fact.Ref()})
	if err != nil || inspected.Ref() != fact.Ref() || inspected.SharedBytes != 8 {
		t.Fatalf("reopen inspect = (%#v,%v)", inspected, err)
	}
}
