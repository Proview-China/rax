package domain_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestContentWriteReadAndIntegrity(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}
	manager, err := domain.NewContentManager(backend, backend, clock, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("abcdefghijk")
	unencrypted := putRequest(data)
	unencrypted.EncryptionRef = ""
	if _, _, err := manager.Put(ctx, unencrypted); err == nil {
		t.Fatal("sensitive unencrypted content was accepted")
	}
	manifest, journal, err := manager.Put(ctx, putRequest(data))
	if err != nil || journal.State != contract.JournalClosed || len(manifest.Chunks) != 3 {
		t.Fatalf("put: manifest=%#v journal=%#v err=%v", manifest, journal, err)
	}
	got, _, err := manager.Read(ctx, manifest.ObjectID)
	if err != nil || string(got) != string(data) {
		t.Fatalf("read: %q err=%v", got, err)
	}
	got[0] = 'X'
	again, _, err := manager.Read(ctx, manifest.ObjectID)
	if err != nil || string(again) != string(data) {
		t.Fatalf("reference backend leaked mutable content: %q err=%v", again, err)
	}
	backend.CorruptChunkForTest(manifest.Chunks[0].Digest)
	if _, _, err := manager.Read(ctx, manifest.ObjectID); !contract.HasCode(err, contract.ErrContentDigestMismatch) {
		t.Fatalf("corrupt chunk should fail closed, got %v", err)
	}
}

func TestContentUnknownRecoversExactJournal(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}
	fired := false
	fault := func(state contract.JournalState, _ contract.WriteJournal) error {
		if state == contract.JournalContentStaged && !fired {
			fired = true
			return errors.New("lost response after durable content stage")
		}
		return nil
	}
	manager, _ := domain.NewContentManager(backend, backend, clock, 4, fault)
	data := []byte("recover-this-content")
	_, journal, err := manager.Put(ctx, putRequest(data))
	if !contract.HasCode(err, contract.ErrCrossStoreIndeterminate) || journal.State != contract.JournalContentStaged {
		t.Fatalf("fault should leave inspectable content_staged journal: %#v err=%v", journal, err)
	}
	recovered, err := manager.Recover(ctx, journal.JournalID, data)
	if err != nil || recovered.State != contract.JournalClosed || recovered.JournalID != journal.JournalID || recovered.LastInspectionRef == "" {
		t.Fatalf("exact recovery failed: %#v err=%v", recovered, err)
	}
	if _, err := manager.Recover(ctx, journal.JournalID, []byte("different-content")); !contract.HasCode(err, contract.ErrContentDigestMismatch) {
		t.Fatalf("changed recovery payload should fail, got %v", err)
	}
}

func TestRetentionManagerTombstoneHoldAndUnsupportedPurge(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}
	manager, _ := domain.NewRetentionManager(backend, clock)
	if _, err := manager.Create(ctx, "object-1", "policy-1", "sensitive"); err != nil {
		t.Fatal(err)
	}
	held, err := manager.Transition(ctx, "object-1", contract.RetentionLegalHold, "hold-fact-1")
	if err != nil || held.State != contract.RetentionLegalHold {
		t.Fatalf("hold: %#v err=%v", held, err)
	}
	if _, err := manager.Transition(ctx, "object-1", contract.RetentionTombstoned, "tombstone-1"); !contract.HasCode(err, contract.ErrRetentionBlocked) {
		t.Fatalf("hold should block tombstone, got %v", err)
	}
	if err := manager.PhysicalPurge(ctx, "object-1"); !contract.HasCode(err, contract.ErrUnsupported) {
		t.Fatalf("physical purge should be unsupported, got %v", err)
	}
}

func TestRetentionManagerRecoversCreateAndCASLostRepliesByExactInspect(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	store := &retentionLostReplyStore{RetentionStore: backend, loseCreate: true, loseCAS: true}
	clock := &testkit.Clock{Time: time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)}
	manager, err := domain.NewRetentionManager(store, clock)
	if err != nil {
		t.Fatal(err)
	}
	created, err := manager.Create(ctx, "retention-lost-object", "retention-lost-policy", "sensitive")
	if err != nil || created.Revision != 1 || created.State != contract.RetentionActive {
		t.Fatalf("create lost reply did not recover exact fact: fact=%+v err=%v", created, err)
	}
	clock.Time = clock.Time.Add(time.Second)
	transitioned, err := manager.Transition(ctx, created.ObjectID, contract.RetentionExpired, "retention-expiry-evidence")
	if err != nil || transitioned.Revision != 2 || transitioned.State != contract.RetentionExpired {
		t.Fatalf("CAS lost reply did not recover exact fact: fact=%+v err=%v", transitioned, err)
	}
	if store.createCalls != 1 || store.casCalls != 1 || store.inspectCalls != 3 {
		t.Fatalf("lost reply recovery repeated mutation or skipped Inspect: create=%d cas=%d inspect=%d", store.createCalls, store.casCalls, store.inspectCalls)
	}
}

func TestRetentionManagerRejectsTypedNilDependenciesAndChangedRecovery(t *testing.T) {
	var nilStore *memory.Backend
	if _, err := domain.NewRetentionManager(nilStore, &testkit.Clock{Time: time.Now()}); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("typed-nil Retention Store was accepted: %v", err)
	}
	var nilClock *testkit.Clock
	if _, err := domain.NewRetentionManager(memory.New(), nilClock); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("typed-nil Retention clock was accepted: %v", err)
	}

	backend := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 18, 9, 5, 0, 0, time.UTC)}
	first, err := domain.NewRetentionManager(backend, clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Create(context.Background(), "retention-conflict-object", "retention-policy-a", "internal"); err != nil {
		t.Fatal(err)
	}
	second, err := domain.NewRetentionManager(backend, clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.Create(context.Background(), "retention-conflict-object", "retention-policy-b", "internal"); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("changed same-ID Retention create recovered another fact: %v", err)
	}
}

type retentionLostReplyStore struct {
	ports.RetentionStore
	loseCreate, loseCAS                 bool
	createCalls, casCalls, inspectCalls int
}

func (s *retentionLostReplyStore) CreateRetention(ctx context.Context, fact contract.RetentionFact) error {
	s.createCalls++
	if err := s.RetentionStore.CreateRetention(ctx, fact); err != nil {
		return err
	}
	if s.loseCreate {
		s.loseCreate = false
		return errors.New("retention create reply lost")
	}
	return nil
}

func (s *retentionLostReplyStore) CASRetention(ctx context.Context, expected uint64, fact contract.RetentionFact) error {
	s.casCalls++
	if err := s.RetentionStore.CASRetention(ctx, expected, fact); err != nil {
		return err
	}
	if s.loseCAS {
		s.loseCAS = false
		return errors.New("retention CAS reply lost")
	}
	return nil
}

func (s *retentionLostReplyStore) InspectRetention(ctx context.Context, objectID string) (contract.RetentionFact, error) {
	s.inspectCalls++
	return s.RetentionStore.InspectRetention(ctx, objectID)
}

func putRequest(data []byte) domain.PutObjectRequest {
	return domain.PutObjectRequest{
		JournalID: "journal-1", ObjectID: "object-1", SchemaVersion: "content/v1",
		Classification: "sensitive", OwnerID: "continuity", ScopeDigest: "scope-1",
		RetentionPolicyRef: "retention-1", Compression: "identity", EncryptionRef: "key-envelope-1",
		Data: data,
	}
}
