package fault_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

type lostIntegrityAuditReplyV1 struct {
	*memory.Backend
	once sync.Once
}

func (r *lostIntegrityAuditReplyV1) CreateContentIntegrityAuditFactV1(ctx context.Context, fact contract.ContentIntegrityAuditFactV1) (contract.ContentIntegrityAuditFactV1, bool, error) {
	stored, replay, err := r.Backend.CreateContentIntegrityAuditFactV1(ctx, fact)
	if err != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, err
	}
	lost := false
	r.once.Do(func() { lost = true })
	if lost {
		return contract.ContentIntegrityAuditFactV1{}, false, contract.NewError(contract.ErrIndeterminate, "audit_reply", "durable commit reply lost")
	}
	return stored, replay, nil
}

type faultCountingMetadataV1 struct {
	ports.MetadataStore
	calls atomic.Int32
}

func (m *faultCountingMetadataV1) InspectObject(ctx context.Context, id string) (contract.ObjectManifest, bool, error) {
	m.calls.Add(1)
	return m.MetadataStore.InspectObject(ctx, id)
}

func (m *faultCountingMetadataV1) InspectJournal(ctx context.Context, id string) (contract.WriteJournal, error) {
	m.calls.Add(1)
	return m.MetadataStore.InspectJournal(ctx, id)
}

func TestContentIntegrityAuditLostReplyOnlyInspectsOriginalAudit(t *testing.T) {
	ctx := context.Background()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 17, 0, 0, 0, time.UTC)}
	backend := memory.New()
	request := testkit.ContentIntegrityAuditRequestV1()
	manager, err := domain.NewContentManager(backend, backend, clock, 64, nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, _, err := manager.Put(ctx, domain.PutObjectRequest{
		JournalID: request.Subjects[0].JournalID, ObjectID: request.Subjects[0].ObjectID,
		SchemaVersion: "content/v1", Classification: "sensitive", OwnerID: "continuity",
		ScopeDigest: request.Scope.ExecutionScopeDigest, RetentionPolicyRef: "retention-1",
		Compression: "identity", EncryptionRef: "key-envelope-1", Data: []byte("lost-reply-content"),
	})
	if err != nil {
		t.Fatal(err)
	}
	request.Subjects[0].ExpectedManifestDigest = manifest.Digest
	repository := &lostIntegrityAuditReplyV1{Backend: backend}
	metadata := &faultCountingMetadataV1{MetadataStore: backend}
	controller, err := domain.NewContentIntegrityAuditControllerV1(repository, metadata, backend, testkit.ContentIntegrityAuditOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := controller.CreateContentIntegrityAuditV1(ctx, request); !contract.HasCode(err, contract.ErrIndeterminate) {
		t.Fatalf("lost reply error = %v", err)
	}
	calls := metadata.calls.Load()
	fact, replay, err := controller.CreateContentIntegrityAuditV1(ctx, request)
	if err != nil || !replay || fact.AuditID != request.AuditID || metadata.calls.Load() != calls {
		t.Fatalf("recover = (%#v,%v,%v), calls=%d->%d", fact, replay, err, calls, metadata.calls.Load())
	}
}
