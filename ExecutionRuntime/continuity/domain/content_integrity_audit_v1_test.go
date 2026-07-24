package domain_test

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

type auditContentV1 struct {
	ports.ContentStore
	mode     string
	getCalls atomic.Int32
}

func (s *auditContentV1) HasChunk(ctx context.Context, ref contract.ChunkRef) (bool, error) {
	if s.mode == "missing" {
		return false, nil
	}
	if s.mode == "unavailable" {
		return false, contract.NewError(contract.ErrUnavailable, "chunk", "injected unavailable")
	}
	return s.ContentStore.HasChunk(ctx, ref)
}

func (s *auditContentV1) GetChunk(ctx context.Context, ref contract.ChunkRef) ([]byte, error) {
	value, err := s.ContentStore.GetChunk(ctx, ref)
	if err != nil {
		return nil, err
	}
	call := s.getCalls.Add(1)
	if s.mode == "corrupt" || (s.mode == "drift" && call > 1) {
		value = append([]byte{}, value...)
		value[0] ^= 0xff
	}
	return value, nil
}

type countingMetadataV1 struct {
	ports.MetadataStore
	calls atomic.Int32
}

type auditMetadataModeV1 struct {
	ports.MetadataStore
	mode string
}

func (m auditMetadataModeV1) InspectObject(ctx context.Context, id string) (contract.ObjectManifest, bool, error) {
	if m.mode == "metadata_absent" {
		return contract.ObjectManifest{}, false, contract.NewError(contract.ErrNotFound, "object", "injected absent")
	}
	manifest, visible, err := m.MetadataStore.InspectObject(ctx, id)
	if m.mode == "visibility_mismatch" {
		visible = false
	}
	return manifest, visible, err
}

func (m auditMetadataModeV1) InspectJournal(ctx context.Context, id string) (contract.WriteJournal, error) {
	if m.mode == "journal_absent" {
		return contract.WriteJournal{}, contract.NewError(contract.ErrNotFound, "journal", "injected absent")
	}
	return m.MetadataStore.InspectJournal(ctx, id)
}

func (s *countingMetadataV1) InspectObject(ctx context.Context, id string) (contract.ObjectManifest, bool, error) {
	s.calls.Add(1)
	return s.MetadataStore.InspectObject(ctx, id)
}

func (s *countingMetadataV1) InspectJournal(ctx context.Context, id string) (contract.WriteJournal, error) {
	s.calls.Add(1)
	return s.MetadataStore.InspectJournal(ctx, id)
}

type lostContentAuditReplyRepositoryV1 struct {
	*memory.Backend
	once sync.Once
}

func (r *lostContentAuditReplyRepositoryV1) CreateContentIntegrityAuditFactV1(ctx context.Context, fact contract.ContentIntegrityAuditFactV1) (contract.ContentIntegrityAuditFactV1, bool, error) {
	stored, replay, err := r.Backend.CreateContentIntegrityAuditFactV1(ctx, fact)
	if err != nil {
		return contract.ContentIntegrityAuditFactV1{}, false, err
	}
	lost := false
	r.once.Do(func() { lost = true })
	if lost {
		return contract.ContentIntegrityAuditFactV1{}, false, contract.NewError(contract.ErrIndeterminate, "audit_reply", "commit succeeded but reply was lost")
	}
	return stored, replay, nil
}

func TestContentIntegrityAuditHealthyMissingCorruptAndUnavailable(t *testing.T) {
	for _, test := range []struct {
		name           string
		mode           string
		classification contract.ContentIntegrityClassificationV1
		status         contract.ContentIntegrityStatusV1
	}{
		{name: "healthy", classification: contract.ContentIntegrityHealthy, status: contract.ContentIntegrityStatusHealthy},
		{name: "missing", mode: "missing", classification: contract.ContentIntegrityDanglingReference, status: contract.ContentIntegrityStatusAttentionRequired},
		{name: "corrupt", mode: "corrupt", classification: contract.ContentIntegrityCorruptContent, status: contract.ContentIntegrityStatusAttentionRequired},
		{name: "unavailable", mode: "unavailable", classification: contract.ContentIntegrityIndeterminate, status: contract.ContentIntegrityStatusIndeterminate},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			backend, clock, request := preparedAuditObjectV1(t, ctx)
			content := &auditContentV1{ContentStore: backend, mode: test.mode}
			controller, err := domain.NewContentIntegrityAuditControllerV1(backend, backend, content, testkit.ContentIntegrityAuditOwnerV1(), clock)
			if err != nil {
				t.Fatal(err)
			}
			fact, replay, err := controller.CreateContentIntegrityAuditV1(ctx, request)
			if err != nil || replay || fact.Status != test.status || fact.Findings[0].Classification != test.classification {
				t.Fatalf("audit = (%#v,%v,%v)", fact, replay, err)
			}
			if err := fact.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestContentIntegrityAuditS1S2DriftFailsWithoutFact(t *testing.T) {
	ctx := context.Background()
	backend, clock, request := preparedAuditObjectV1(t, ctx)
	content := &auditContentV1{ContentStore: backend, mode: "drift"}
	controller, err := domain.NewContentIntegrityAuditControllerV1(backend, backend, content, testkit.ContentIntegrityAuditOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := controller.CreateContentIntegrityAuditV1(ctx, request); !contract.HasCode(err, contract.ErrIndeterminate) {
		t.Fatalf("S1/S2 drift must fail closed, got %v", err)
	}
	if _, err := backend.InspectContentIntegrityAuditByIDV1(ctx, ports.InspectContentIntegrityAuditByIDRequestV1{
		TenantID: request.Scope.TenantID, ScopeDigest: request.Scope.ExecutionScopeDigest,
		AuditID: request.AuditID, Owner: testkit.ContentIntegrityAuditOwnerV1(),
	}); !contract.HasCode(err, contract.ErrNotFound) {
		t.Fatalf("drift created a fact: %v", err)
	}
}

func TestContentIntegrityAuditClosedMetadataClassifications(t *testing.T) {
	for _, test := range []struct {
		mode           string
		classification contract.ContentIntegrityClassificationV1
	}{
		{mode: "metadata_absent", classification: contract.ContentIntegrityMetadataAbsent},
		{mode: "journal_absent", classification: contract.ContentIntegrityJournalAbsent},
		{mode: "visibility_mismatch", classification: contract.ContentIntegrityIndeterminate},
	} {
		t.Run(test.mode, func(t *testing.T) {
			ctx := context.Background()
			backend, clock, request := preparedAuditObjectV1(t, ctx)
			controller, err := domain.NewContentIntegrityAuditControllerV1(backend, auditMetadataModeV1{MetadataStore: backend, mode: test.mode}, backend, testkit.ContentIntegrityAuditOwnerV1(), clock)
			if err != nil {
				t.Fatal(err)
			}
			fact, _, err := controller.CreateContentIntegrityAuditV1(ctx, request)
			if err != nil || fact.Findings[0].Classification != test.classification {
				t.Fatalf("classification = (%s,%v)", fact.Findings[0].Classification, err)
			}
		})
	}
}

func TestContentIntegrityAuditLostReplyInspectsOriginalWithoutRescan(t *testing.T) {
	ctx := context.Background()
	backend, clock, request := preparedAuditObjectV1(t, ctx)
	repository := &lostContentAuditReplyRepositoryV1{Backend: backend}
	metadata := &countingMetadataV1{MetadataStore: backend}
	controller, err := domain.NewContentIntegrityAuditControllerV1(repository, metadata, backend, testkit.ContentIntegrityAuditOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := controller.CreateContentIntegrityAuditV1(ctx, request); !contract.HasCode(err, contract.ErrIndeterminate) {
		t.Fatalf("lost reply = %v", err)
	}
	calls := metadata.calls.Load()
	fact, replay, err := controller.CreateContentIntegrityAuditV1(ctx, request)
	if err != nil || !replay || fact.AuditID != request.AuditID || metadata.calls.Load() != calls {
		t.Fatalf("recovery = (%#v,%v,%v), calls=%d->%d", fact, replay, err, calls, metadata.calls.Load())
	}
	changed := request
	changed.Subjects = append([]contract.ContentIntegritySubjectV1{}, request.Subjects...)
	changed.Subjects[0].ExpectedManifestDigest = "different-manifest-digest"
	if _, _, err := controller.CreateContentIntegrityAuditV1(ctx, changed); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("same ID changed request accepted: %v", err)
	}
}

func TestContentIntegrityAuditRejectsTypedNilDependencies(t *testing.T) {
	var repository *memory.Backend
	backend := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 17, 0, 0, 0, time.UTC)}
	if _, err := domain.NewContentIntegrityAuditControllerV1(repository, backend, backend, testkit.ContentIntegrityAuditOwnerV1(), clock); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("typed-nil repository accepted: %v", err)
	}
}

func preparedAuditObjectV1(t *testing.T, ctx context.Context) (*memory.Backend, *testkit.Clock, ports.CreateContentIntegrityAuditRequestV1) {
	t.Helper()
	backend := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 17, 0, 0, 0, time.UTC)}
	manager, err := domain.NewContentManager(backend, backend, clock, 64, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := testkit.ContentIntegrityAuditRequestV1()
	manifest, journal, err := manager.Put(ctx, domain.PutObjectRequest{
		JournalID: request.Subjects[0].JournalID, ObjectID: request.Subjects[0].ObjectID,
		SchemaVersion: "content/v1", Classification: "sensitive", OwnerID: "continuity",
		ScopeDigest: request.Scope.ExecutionScopeDigest, RetentionPolicyRef: "retention-1",
		Compression: "identity", EncryptionRef: "key-envelope-1", Data: []byte("content-integrity-audit"),
	})
	if err != nil || journal.State != contract.JournalClosed {
		t.Fatalf("prepare content = (%#v,%v)", journal, err)
	}
	request.Subjects[0].ExpectedManifestDigest = manifest.Digest
	return backend, clock, request
}
