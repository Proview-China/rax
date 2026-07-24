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

type lostHistoryDerivationReplyRepositoryV1 struct {
	*memory.Backend
	once sync.Once
}

func (r *lostHistoryDerivationReplyRepositoryV1) CreateHistoryDerivationCandidateFactV1(ctx context.Context, fact contract.HistoryDerivationCandidateFactV1) (contract.HistoryDerivationCandidateFactV1, bool, error) {
	stored, replay, err := r.Backend.CreateHistoryDerivationCandidateFactV1(ctx, fact)
	if err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, err
	}
	lost := false
	r.once.Do(func() { lost = true })
	if lost {
		return contract.HistoryDerivationCandidateFactV1{}, false, contract.NewError(contract.ErrIndeterminate, "history_derivation_reply", "commit succeeded but reply was lost")
	}
	return stored, replay, nil
}

type countingHistoryTimelineV1 struct {
	ports.HistoryDerivationTimelineReaderV1
	calls atomic.Int64
}

func (r *countingHistoryTimelineV1) InspectByEvidence(ctx context.Context, id string) (contract.TimelineEventRecord, error) {
	r.calls.Add(1)
	return r.HistoryDerivationTimelineReaderV1.InspectByEvidence(ctx, id)
}

type driftingHistoryTimelineV1 struct {
	ports.HistoryDerivationTimelineReaderV1
	calls atomic.Int64
}

func (r *driftingHistoryTimelineV1) InspectByEvidence(ctx context.Context, id string) (contract.TimelineEventRecord, error) {
	record, err := r.HistoryDerivationTimelineReaderV1.InspectByEvidence(ctx, id)
	if err == nil && r.calls.Add(1) > 1 {
		record.Visibility = "tombstoned"
	}
	return record, err
}

func TestHistoryDerivationControllerCreatesCandidateOnlyFact(t *testing.T) {
	ctx := context.Background()
	backend, clock, request := preparedHistoryDerivationV1(t, ctx)
	controller, err := domain.NewHistoryDerivationCandidateControllerV1(backend, backend, backend, backend, testkit.HistoryDerivationOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	fact, replay, err := controller.CreateHistoryDerivationCandidateV1(ctx, request)
	if err != nil || replay {
		t.Fatalf("create = (%#v,%v,%v)", fact, replay, err)
	}
	if fact.Authority != contract.HistoryDerivationAuthorityV1 || len(fact.Sources) != 2 || fact.Output.ObjectID != request.OutputObjectID {
		t.Fatalf("fact = %#v", fact)
	}
	for i, source := range fact.Sources {
		if source.EvidenceRecordRef != request.Sources[i].EvidenceRecordRef {
			t.Fatal("source order changed")
		}
	}
}

func TestHistoryDerivationControllerFailsClosedForSpliceDriftAndCorruptOutput(t *testing.T) {
	for _, test := range []struct {
		name     string
		prepare  func(*ports.CreateHistoryDerivationCandidateRequestV1)
		timeline func(*memory.Backend) ports.HistoryDerivationTimelineReaderV1
		content  func(*memory.Backend) ports.ContentStore
		code     contract.ErrorCode
	}{
		{name: "event digest", prepare: func(r *ports.CreateHistoryDerivationCandidateRequestV1) {
			r.Sources[0].ExpectedEvidenceRecordDigest = "changed"
		}, code: contract.ErrRevisionConflict},
		{name: "projection digest", prepare: func(r *ports.CreateHistoryDerivationCandidateRequestV1) {
			r.Sources[0].ExpectedProjectionDigest = "changed"
		}, code: contract.ErrRevisionConflict},
		{name: "cross scope", prepare: func(r *ports.CreateHistoryDerivationCandidateRequestV1) { r.Scope.InstanceID = "other-instance" }, code: contract.ErrRevisionConflict},
		{name: "s1 s2 drift", timeline: func(b *memory.Backend) ports.HistoryDerivationTimelineReaderV1 {
			return &driftingHistoryTimelineV1{HistoryDerivationTimelineReaderV1: b}
		}, code: contract.ErrIndeterminate},
		{name: "corrupt output", content: func(b *memory.Backend) ports.ContentStore { return &auditContentV1{ContentStore: b, mode: "corrupt"} }, code: contract.ErrContentDigestMismatch},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			backend, clock, request := preparedHistoryDerivationV1(t, ctx)
			if test.prepare != nil {
				test.prepare(&request)
			}
			timeline := ports.HistoryDerivationTimelineReaderV1(backend)
			if test.timeline != nil {
				timeline = test.timeline(backend)
			}
			content := ports.ContentStore(backend)
			if test.content != nil {
				content = test.content(backend)
			}
			controller, err := domain.NewHistoryDerivationCandidateControllerV1(backend, timeline, backend, content, testkit.HistoryDerivationOwnerV1(), clock)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := controller.CreateHistoryDerivationCandidateV1(ctx, request); !contract.HasCode(err, test.code) {
				t.Fatalf("error = %v", err)
			}
			if _, err := backend.InspectHistoryDerivationCandidateByIDV1(ctx, ports.InspectHistoryDerivationCandidateByIDRequestV1{TenantID: request.Scope.TenantID, ScopeDigest: request.Scope.ExecutionScopeDigest, CandidateID: request.CandidateID, Owner: testkit.HistoryDerivationOwnerV1()}); !contract.HasCode(err, contract.ErrNotFound) {
				t.Fatalf("failed inspection created a fact: %v", err)
			}
		})
	}
}

func TestHistoryDerivationLostReplyInspectsOriginalWithoutSourceReplay(t *testing.T) {
	ctx := context.Background()
	backend, clock, request := preparedHistoryDerivationV1(t, ctx)
	repository := &lostHistoryDerivationReplyRepositoryV1{Backend: backend}
	timeline := &countingHistoryTimelineV1{HistoryDerivationTimelineReaderV1: backend}
	controller, err := domain.NewHistoryDerivationCandidateControllerV1(repository, timeline, backend, backend, testkit.HistoryDerivationOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := controller.CreateHistoryDerivationCandidateV1(ctx, request); !contract.HasCode(err, contract.ErrIndeterminate) {
		t.Fatalf("lost reply = %v", err)
	}
	calls := timeline.calls.Load()
	fact, replay, err := controller.CreateHistoryDerivationCandidateV1(ctx, request)
	if err != nil || !replay || fact.CandidateID != request.CandidateID || timeline.calls.Load() != calls {
		t.Fatalf("recovery=(%#v,%v,%v), calls=%d->%d", fact, replay, err, calls, timeline.calls.Load())
	}
	changed := request
	changed.Kind = contract.HistoryDerivationIndex
	if _, _, err := controller.CreateHistoryDerivationCandidateV1(ctx, changed); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("changed request accepted: %v", err)
	}
}

func TestHistoryDerivationRejectsTypedNilDependencies(t *testing.T) {
	var repository *memory.Backend
	backend := memory.New()
	clock := &testkit.Clock{Time: time.Now()}
	if _, err := domain.NewHistoryDerivationCandidateControllerV1(repository, backend, backend, backend, testkit.HistoryDerivationOwnerV1(), clock); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("typed nil accepted: %v", err)
	}
}

func preparedHistoryDerivationV1(t *testing.T, ctx context.Context) (*memory.Backend, *testkit.Clock, ports.CreateHistoryDerivationCandidateRequestV1) {
	t.Helper()
	backend := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 18, 30, 0, 0, time.UTC)}
	for i := uint64(1); i <= 2; i++ {
		if _, _, err := backend.PutProjection(ctx, testkit.TimelineEvent(i, i, contract.TrustObservation)); err != nil {
			t.Fatal(err)
		}
	}
	manager, err := domain.NewContentManager(backend, backend, clock, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, journal, err := manager.Put(ctx, domain.PutObjectRequest{
		JournalID: "journal-derived", ObjectID: "object-derived", SchemaVersion: "content/v1",
		Classification: "sensitive", OwnerID: "continuity", ScopeDigest: testkit.Scope().ExecutionScopeDigest,
		RetentionPolicyRef: "retention-1", Compression: "identity", EncryptionRef: "key-envelope-1", Data: []byte("derived history candidate"),
	})
	if err != nil || journal.State != contract.JournalClosed {
		t.Fatalf("put = (%#v,%v)", journal, err)
	}
	sources := make([]contract.HistoryDerivationSourceCoordinateV1, 0, 2)
	for i := uint64(1); i <= 2; i++ {
		record, err := backend.InspectByEvidence(ctx, "evidence-"+string(rune('0'+i)))
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, contract.HistoryDerivationSourceCoordinateV1{EvidenceRecordRef: record.EvidenceRecordRef, ExpectedEvidenceRecordDigest: record.EvidenceRecordDigest, ExpectedProjectionDigest: record.Candidate.Digest})
	}
	return backend, clock, ports.CreateHistoryDerivationCandidateRequestV1{
		CandidateID: "history-derivation-1", IdempotencyKey: "history-derivation-request-1", Scope: testkit.Scope(), Kind: contract.HistoryDerivationSummary,
		Sources: sources, OutputObjectID: manifest.ObjectID, ExpectedOutputManifestDigest: manifest.Digest,
	}
}
