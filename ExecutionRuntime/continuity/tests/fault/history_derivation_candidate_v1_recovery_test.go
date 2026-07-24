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

type derivationFaultTimelineV1 struct {
	ports.HistoryDerivationTimelineReaderV1
	calls atomic.Int32
}

func (r *derivationFaultTimelineV1) InspectByEvidence(ctx context.Context, id string) (contract.TimelineEventRecord, error) {
	r.calls.Add(1)
	return r.HistoryDerivationTimelineReaderV1.InspectByEvidence(ctx, id)
}

func TestHistoryDerivationLostReplyOnlyInspectsOriginalCandidate(t *testing.T) {
	ctx := context.Background()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 19, 0, 0, 0, time.UTC)}
	backend := memory.New()
	event := testkit.TimelineEvent(1, 1, contract.TrustObservation)
	if _, _, err := backend.PutProjection(ctx, event); err != nil {
		t.Fatal(err)
	}
	manager, err := domain.NewContentManager(backend, backend, clock, 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, _, err := manager.Put(ctx, domain.PutObjectRequest{JournalID: "journal-derived", ObjectID: "object-derived", SchemaVersion: "content/v1", Classification: "sensitive", OwnerID: "continuity", ScopeDigest: testkit.Scope().ExecutionScopeDigest, RetentionPolicyRef: "retention-1", Compression: "identity", EncryptionRef: "key-envelope-1", Data: []byte("derived output")})
	if err != nil {
		t.Fatal(err)
	}
	request := ports.CreateHistoryDerivationCandidateRequestV1{CandidateID: "derivation-1", IdempotencyKey: "request-1", Scope: testkit.Scope(), Kind: contract.HistoryDerivationSummary, Sources: []contract.HistoryDerivationSourceCoordinateV1{{EvidenceRecordRef: event.EvidenceRecordRef, ExpectedEvidenceRecordDigest: event.EvidenceRecordDigest, ExpectedProjectionDigest: event.Candidate.Digest}}, OutputObjectID: manifest.ObjectID, ExpectedOutputManifestDigest: manifest.Digest}
	timeline := &derivationFaultTimelineV1{HistoryDerivationTimelineReaderV1: backend}
	controller, err := domain.NewHistoryDerivationCandidateControllerV1(backend, timeline, backend, backend, testkit.HistoryDerivationOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	fake, err := fakes.NewHistoryDerivationCandidateGovernanceV1(controller)
	if err != nil {
		t.Fatal(err)
	}
	fake.LoseNextSuccessfulCreateReply(contract.NewError(contract.ErrIndeterminate, "reply", "durable reply lost"))
	if _, _, err := fake.CreateHistoryDerivationCandidateV1(ctx, request); !contract.HasCode(err, contract.ErrIndeterminate) {
		t.Fatalf("lost reply=%v", err)
	}
	calls := timeline.calls.Load()
	fact, replay, err := fake.CreateHistoryDerivationCandidateV1(ctx, request)
	if err != nil || !replay || fact.CandidateID != request.CandidateID || timeline.calls.Load() != calls {
		t.Fatalf("recover=(%#v,%v,%v),calls=%d->%d", fact, replay, err, calls, timeline.calls.Load())
	}
}
