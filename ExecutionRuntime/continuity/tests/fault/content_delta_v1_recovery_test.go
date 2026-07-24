package fault_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

type deltaFaultMetadataV1 struct {
	ports.MetadataStore
	calls atomic.Int32
}

func (m *deltaFaultMetadataV1) InspectObject(ctx context.Context, id string) (contract.ObjectManifest, bool, error) {
	m.calls.Add(1)
	return m.MetadataStore.InspectObject(ctx, id)
}

func TestContentDeltaLostReplyOnlyInspectsOriginalDelta(t *testing.T) {
	ctx := context.Background()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC)}
	backend := memory.New()
	manager, err := domain.NewContentManager(backend, backend, clock, 4, nil)
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
	metadata := &deltaFaultMetadataV1{MetadataStore: backend}
	controller, err := domain.NewContentDeltaControllerV1(backend, metadata, backend, testkit.ContentDeltaOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	fake, err := fakes.NewContentDeltaGovernanceV1(controller)
	if err != nil {
		t.Fatal(err)
	}
	fake.LoseNextSuccessfulCreateReply(contract.NewError(contract.ErrIndeterminate, "delta_reply", "durable reply lost"))
	if _, _, err := fake.CreateContentDeltaV1(ctx, request); !contract.HasCode(err, contract.ErrIndeterminate) {
		t.Fatalf("lost reply error = %v", err)
	}
	calls := metadata.calls.Load()
	fact, replay, err := fake.CreateContentDeltaV1(ctx, request)
	if err != nil || !replay || fact.DeltaID != request.DeltaID || metadata.calls.Load() != calls {
		t.Fatalf("recover = (%#v,%v,%v), calls=%d->%d", fact, replay, err, calls, metadata.calls.Load())
	}
}
