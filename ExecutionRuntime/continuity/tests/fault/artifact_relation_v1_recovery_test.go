package fault_test

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

type lostArtifactReplyRepositoryV1 struct {
	*memory.Backend
	once sync.Once
}

func (r *lostArtifactReplyRepositoryV1) CreateArtifactRelationFactV1(ctx context.Context, fact contract.ArtifactRelationFactV1) (contract.ArtifactRelationFactV1, bool, error) {
	stored, replay, err := r.Backend.CreateArtifactRelationFactV1(ctx, fact)
	if err != nil {
		return contract.ArtifactRelationFactV1{}, false, err
	}
	lost := false
	r.once.Do(func() { lost = true })
	if lost {
		return contract.ArtifactRelationFactV1{}, false, contract.NewError(contract.ErrIndeterminate, "artifact_relation_reply", "commit succeeded but reply was lost")
	}
	return stored, replay, nil
}

type recoverableArtifactSourceReaderV1 struct {
	source contract.ArtifactRelationSourceProjectionV1
	err    error
	calls  int
}

func (r *recoverableArtifactSourceReaderV1) InspectArtifactRelationSourceV1(context.Context, ports.ArtifactRelationSourceRequestV1) (contract.ArtifactRelationSourceProjectionV1, error) {
	r.calls++
	if r.err != nil {
		return contract.ArtifactRelationSourceProjectionV1{}, r.err
	}
	return r.source.Clone(), nil
}

func TestArtifactRelationLostReplyInspectsOriginalRelationWithoutOwnerReplay(t *testing.T) {
	ctx := context.Background()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)}
	backend := memory.New()
	timeline, err := domain.NewReferenceTimeline(backend, clock, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	candidate := testkit.Candidate(1, 1, contract.TrustObservation)
	source := testkit.ArtifactSourceProjectionV1(candidate.Evidence.RecordRef, candidate.Evidence.RecordDigest)
	candidate.ObjectRefs = []string{source.Artifact.ArtifactFactRef.ID, source.RelatedFactRef.ID}
	candidate.Digest, _ = candidate.CanonicalDigest()
	if _, _, err := timeline.Project(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	repository := &lostArtifactReplyRepositoryV1{Backend: backend}
	reader := &recoverableArtifactSourceReaderV1{source: source}
	controller, err := domain.NewArtifactRelationControllerV1(repository, backend, reader, testkit.ArtifactRelationOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	request := testkit.ArtifactRelationRequestV1(source)
	if _, _, err := controller.CreateArtifactRelationV1(ctx, request); !contract.HasCode(err, contract.ErrIndeterminate) {
		t.Fatalf("lost reply error = %v", err)
	}
	if reader.calls != 2 {
		t.Fatalf("initial S1/S2 calls = %d", reader.calls)
	}
	reader.err = contract.NewError(contract.ErrUnavailable, "artifact_owner", "unavailable after commit")
	fact, replay, err := controller.CreateArtifactRelationV1(ctx, request)
	if err != nil || !replay || fact.RelationID != request.RelationID || reader.calls != 2 {
		t.Fatalf("recovery = (%#v,%v,%v), reader calls=%d", fact, replay, err, reader.calls)
	}
}
