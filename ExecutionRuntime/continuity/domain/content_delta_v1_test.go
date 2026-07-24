package domain_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

type lostContentDeltaReplyRepositoryV1 struct {
	*memory.Backend
	once sync.Once
}

func (r *lostContentDeltaReplyRepositoryV1) CreateContentDeltaFactV1(ctx context.Context, fact contract.ContentDeltaFactV1) (contract.ContentDeltaFactV1, bool, error) {
	stored, replay, err := r.Backend.CreateContentDeltaFactV1(ctx, fact)
	if err != nil {
		return contract.ContentDeltaFactV1{}, false, err
	}
	lost := false
	r.once.Do(func() { lost = true })
	if lost {
		return contract.ContentDeltaFactV1{}, false, contract.NewError(contract.ErrIndeterminate, "delta_reply", "commit succeeded but reply was lost")
	}
	return stored, replay, nil
}

func TestContentDeltaControllerDerivesSharedRecipe(t *testing.T) {
	ctx := context.Background()
	backend, clock, request := preparedDeltaObjectsV1(t, ctx)
	controller, err := domain.NewContentDeltaControllerV1(backend, backend, backend, testkit.ContentDeltaOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	fact, replay, err := controller.CreateContentDeltaV1(ctx, request)
	if err != nil || replay {
		t.Fatalf("create = (%#v,%v,%v)", fact, replay, err)
	}
	if fact.SharedBytes != 8 || fact.AddedBytes != 4 || fact.RemovedBytes != 4 || len(fact.TargetRecipe) != 3 {
		t.Fatalf("derived delta = %#v", fact)
	}
	if fact.TargetRecipe[0].Kind != contract.ContentDeltaReuse || fact.TargetRecipe[2].Kind != contract.ContentDeltaAdd {
		t.Fatalf("recipe = %#v", fact.TargetRecipe)
	}
}

func TestContentDeltaControllerFailsClosedForMissingCorruptAndExpectedDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mode   string
		mutate func(*ports.CreateContentDeltaRequestV1)
		code   contract.ErrorCode
	}{
		{name: "missing", mode: "missing", code: contract.ErrCrossStoreIndeterminate},
		{name: "corrupt", mode: "corrupt", code: contract.ErrContentDigestMismatch},
		{name: "expected-drift", mutate: func(r *ports.CreateContentDeltaRequestV1) {
			r.ExpectedTargetManifestDigest = "different-manifest-digest"
		}, code: contract.ErrRevisionConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			backend, clock, request := preparedDeltaObjectsV1(t, ctx)
			if test.mutate != nil {
				test.mutate(&request)
			}
			content := ports.ContentStore(backend)
			if test.mode != "" {
				content = &auditContentV1{ContentStore: backend, mode: test.mode}
			}
			controller, err := domain.NewContentDeltaControllerV1(backend, backend, content, testkit.ContentDeltaOwnerV1(), clock)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := controller.CreateContentDeltaV1(ctx, request); !contract.HasCode(err, test.code) {
				t.Fatalf("error = %v", err)
			}
			if _, err := backend.InspectContentDeltaByIDV1(ctx, ports.InspectContentDeltaByIDRequestV1{
				TenantID: request.Scope.TenantID, ScopeDigest: request.Scope.ExecutionScopeDigest,
				DeltaID: request.DeltaID, Owner: testkit.ContentDeltaOwnerV1(),
			}); !contract.HasCode(err, contract.ErrNotFound) {
				t.Fatalf("failed inspection created a Delta Fact: %v", err)
			}
		})
	}
}

func TestContentDeltaControllerRejectsCrossScopeSplice(t *testing.T) {
	ctx := context.Background()
	backend, clock, request := preparedDeltaObjectsV1(t, ctx)
	request.Scope.ExecutionScopeDigest = "other-execution-scope"
	controller, err := domain.NewContentDeltaControllerV1(backend, backend, backend, testkit.ContentDeltaOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := controller.CreateContentDeltaV1(ctx, request); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("cross-scope splice accepted: %v", err)
	}
}

func TestContentDeltaLostReplyInspectsOriginalWithoutContentReplay(t *testing.T) {
	ctx := context.Background()
	backend, clock, request := preparedDeltaObjectsV1(t, ctx)
	repository := &lostContentDeltaReplyRepositoryV1{Backend: backend}
	metadata := &countingMetadataV1{MetadataStore: backend}
	controller, err := domain.NewContentDeltaControllerV1(repository, metadata, backend, testkit.ContentDeltaOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := controller.CreateContentDeltaV1(ctx, request); !contract.HasCode(err, contract.ErrIndeterminate) {
		t.Fatalf("lost reply = %v", err)
	}
	calls := metadata.calls.Load()
	fact, replay, err := controller.CreateContentDeltaV1(ctx, request)
	if err != nil || !replay || fact.DeltaID != request.DeltaID || metadata.calls.Load() != calls {
		t.Fatalf("recovery = (%#v,%v,%v), calls=%d->%d", fact, replay, err, calls, metadata.calls.Load())
	}
	changed := request
	changed.TargetObjectID = "object-other"
	if _, _, err := controller.CreateContentDeltaV1(ctx, changed); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("changed request accepted: %v", err)
	}
}

func TestContentDeltaRejectsTypedNilDependencies(t *testing.T) {
	var repository *memory.Backend
	backend := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC)}
	if _, err := domain.NewContentDeltaControllerV1(repository, backend, backend, testkit.ContentDeltaOwnerV1(), clock); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("typed-nil repository accepted: %v", err)
	}
}

func preparedDeltaObjectsV1(t *testing.T, ctx context.Context) (*memory.Backend, *testkit.Clock, ports.CreateContentDeltaRequestV1) {
	t.Helper()
	backend := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC)}
	manager, err := domain.NewContentManager(backend, backend, clock, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	put := func(journalID, objectID string, data []byte) contract.ObjectManifest {
		manifest, journal, err := manager.Put(ctx, domain.PutObjectRequest{
			JournalID: journalID, ObjectID: objectID, SchemaVersion: "content/v1",
			Classification: "sensitive", OwnerID: "continuity", ScopeDigest: testkit.Scope().ExecutionScopeDigest,
			RetentionPolicyRef: "retention-1", Compression: "identity", EncryptionRef: "key-envelope-1", Data: data,
		})
		if err != nil || journal.State != contract.JournalClosed {
			t.Fatalf("put %s = (%#v,%v)", objectID, journal, err)
		}
		return manifest
	}
	base := put("journal-base", "object-base", []byte("AAAABBBBCCCC"))
	target := put("journal-target", "object-target", []byte("AAAABBBBDDDD"))
	return backend, clock, ports.CreateContentDeltaRequestV1{
		DeltaID: "content-delta-1", IdempotencyKey: "content-delta-request-1", Scope: testkit.Scope(),
		BaseObjectID: base.ObjectID, ExpectedBaseManifestDigest: base.Digest,
		TargetObjectID: target.ObjectID, ExpectedTargetManifestDigest: target.Digest,
	}
}
