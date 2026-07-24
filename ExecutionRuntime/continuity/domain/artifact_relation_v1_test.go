package domain_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

type artifactSourceReaderV1 struct {
	mu     sync.Mutex
	values []contract.ArtifactRelationSourceProjectionV1
	err    error
	calls  int
}

func (r *artifactSourceReaderV1) InspectArtifactRelationSourceV1(context.Context, ports.ArtifactRelationSourceRequestV1) (contract.ArtifactRelationSourceProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.err != nil {
		return contract.ArtifactRelationSourceProjectionV1{}, r.err
	}
	index := r.calls - 1
	if index >= len(r.values) {
		index = len(r.values) - 1
	}
	return r.values[index].Clone(), nil
}

func TestArtifactRelationCreateExactReplayAndIndexes(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)}
	record := projectArtifactTimelineEventV1(t, ctx, backend, clock)
	source := testkit.ArtifactSourceProjectionV1(record.EvidenceRecordRef, record.EvidenceRecordDigest)
	reader := &artifactSourceReaderV1{values: []contract.ArtifactRelationSourceProjectionV1{source, source}}
	controller, err := domain.NewArtifactRelationControllerV1(backend, backend, reader, testkit.ArtifactRelationOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	request := testkit.ArtifactRelationRequestV1(source)
	fact, replay, err := controller.CreateArtifactRelationV1(ctx, request)
	if err != nil || replay {
		t.Fatalf("create = (%#v,%v,%v)", fact, replay, err)
	}
	if reader.calls != 2 || fact.Revision != 1 || fact.SourceProjection.Artifact.StorageRef != source.Artifact.StorageRef {
		t.Fatalf("S1/S2 or fact derivation failed: calls=%d fact=%#v", reader.calls, fact)
	}

	// A retry after a lost reply inspects the stable Relation ID first. It does
	// not need to call the now-unavailable external owner again.
	reader.err = contract.NewError(contract.ErrUnavailable, "artifact_owner", "unavailable")
	replayed, replay, err := controller.CreateArtifactRelationV1(ctx, request)
	if err != nil || !replay || replayed.Ref() != fact.Ref() || reader.calls != 2 {
		t.Fatalf("lost reply replay = (%#v,%v,%v), calls=%d", replayed, replay, err, reader.calls)
	}
	byArtifact, err := controller.ListArtifactRelationsV1(ctx, ports.ListArtifactRelationsRequestV1{ArtifactFactRef: source.Artifact.ArtifactFactRef})
	if err != nil || len(byArtifact) != 1 || byArtifact[0].Ref() != fact.Ref() {
		t.Fatalf("artifact index = (%#v,%v)", byArtifact, err)
	}
	byRelated, err := controller.ListRelatedArtifactRelationsV1(ctx, ports.ListRelatedArtifactRelationsRequestV1{RelatedFactRef: source.RelatedFactRef})
	if err != nil || len(byRelated) != 1 || byRelated[0].Ref() != fact.Ref() {
		t.Fatalf("related index = (%#v,%v)", byRelated, err)
	}
	byArtifact[0].SourceProjection.Artifact.StorageRef = "mutated"
	inspected, err := controller.InspectArtifactRelationV1(ctx, ports.InspectArtifactRelationRequestV1{Ref: fact.Ref()})
	if err != nil || inspected.SourceProjection.Artifact.StorageRef != source.Artifact.StorageRef {
		t.Fatal("Artifact Relation result aliases repository state")
	}
}

func TestArtifactRelationRejectsS1S2DriftAndWritesNothing(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)}
	record := projectArtifactTimelineEventV1(t, ctx, backend, clock)
	source := testkit.ArtifactSourceProjectionV1(record.EvidenceRecordRef, record.EvidenceRecordDigest)
	drifted := source.Clone()
	drifted.Artifact.StorageDigest = "changed-storage-digest"
	reader := &artifactSourceReaderV1{values: []contract.ArtifactRelationSourceProjectionV1{source, drifted}}
	controller, err := domain.NewArtifactRelationControllerV1(backend, backend, reader, testkit.ArtifactRelationOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	request := testkit.ArtifactRelationRequestV1(source)
	if _, _, err := controller.CreateArtifactRelationV1(ctx, request); !contract.HasCode(err, contract.ErrIndeterminate) {
		t.Fatalf("S1/S2 drift error = %v", err)
	}
	if _, err := backend.InspectArtifactRelationByIDV1(ctx, ports.InspectArtifactRelationByIDRequestV1{
		TenantID: request.Scope.TenantID, ScopeDigest: request.Scope.ExecutionScopeDigest,
		RelationID: request.RelationID, Owner: testkit.ArtifactRelationOwnerV1(),
	}); !contract.HasCode(err, contract.ErrNotFound) {
		t.Fatalf("drift wrote a relation: %v", err)
	}
}

func TestArtifactRelationRejectsEverySourceClosureDrift(t *testing.T) {
	mutations := map[string]func(*contract.ArtifactRelationSourceProjectionV1){
		"artifact exact ref": func(value *contract.ArtifactRelationSourceProjectionV1) {
			value.Artifact.ArtifactFactRef.Digest = "changed-artifact-digest"
		},
		"related exact ref": func(value *contract.ArtifactRelationSourceProjectionV1) {
			value.RelatedFactRef.Digest = "changed-related-digest"
		},
		"storage": func(value *contract.ArtifactRelationSourceProjectionV1) {
			value.Artifact.StorageDigest = "changed-storage-digest"
		},
		"parent": func(value *contract.ArtifactRelationSourceProjectionV1) {
			value.Artifact.ParentRevisionRef.Digest = "changed-parent-digest"
		},
		"source projection": func(value *contract.ArtifactRelationSourceProjectionV1) {
			value.SourceProjectionRef.Digest = "changed-source-digest"
		},
		"evidence": func(value *contract.ArtifactRelationSourceProjectionV1) {
			value.EvidenceRecordDigest = "changed-evidence-digest"
			value.Artifact.OriginEvidenceDigest = "changed-evidence-digest"
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			backend := memory.New()
			clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)}
			record := projectArtifactTimelineEventV1(t, ctx, backend, clock)
			source := testkit.ArtifactSourceProjectionV1(record.EvidenceRecordRef, record.EvidenceRecordDigest)
			drifted := source.Clone()
			mutate(&drifted)
			reader := &artifactSourceReaderV1{values: []contract.ArtifactRelationSourceProjectionV1{source, drifted}}
			controller, err := domain.NewArtifactRelationControllerV1(backend, backend, reader, testkit.ArtifactRelationOwnerV1(), clock)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := controller.CreateArtifactRelationV1(ctx, testkit.ArtifactRelationRequestV1(source)); err == nil {
				t.Fatal("source closure drift was accepted")
			}
			if _, err := backend.InspectArtifactRelationByIDV1(ctx, ports.InspectArtifactRelationByIDRequestV1{
				TenantID: testkit.Scope().TenantID, ScopeDigest: testkit.Scope().ExecutionScopeDigest,
				RelationID: "artifact-relation-1", Owner: testkit.ArtifactRelationOwnerV1(),
			}); !contract.HasCode(err, contract.ErrNotFound) {
				t.Fatalf("source closure drift wrote a relation: %v", err)
			}
		})
	}
}

func TestArtifactRelationRejectsOwnerRouteEventAndTenantSplice(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)}
	record := projectArtifactTimelineEventV1(t, ctx, backend, clock)
	source := testkit.ArtifactSourceProjectionV1(record.EvidenceRecordRef, record.EvidenceRecordDigest)

	t.Run("owner route mismatch", func(t *testing.T) {
		wrong := source.Clone()
		wrong.RelatedFactRef.Digest = "other-owner-result"
		controller, _ := domain.NewArtifactRelationControllerV1(backend, backend, &artifactSourceReaderV1{values: []contract.ArtifactRelationSourceProjectionV1{wrong}}, testkit.ArtifactRelationOwnerV1(), clock)
		if _, _, err := controller.CreateArtifactRelationV1(ctx, testkit.ArtifactRelationRequestV1(source)); !contract.HasCode(err, contract.ErrRevisionConflict) {
			t.Fatalf("owner route mismatch = %v", err)
		}
	})

	t.Run("event does not bind related fact", func(t *testing.T) {
		isolated := memory.New()
		timeline, _ := domain.NewReferenceTimeline(isolated, clock, time.Minute)
		candidate := testkit.Candidate(1, 1, contract.TrustObservation)
		candidate.ObjectRefs = []string{source.Artifact.ArtifactFactRef.ID}
		candidate.Digest, _ = candidate.CanonicalDigest()
		if _, _, err := timeline.Project(ctx, candidate); err != nil {
			t.Fatal(err)
		}
		controller, _ := domain.NewArtifactRelationControllerV1(isolated, isolated, &artifactSourceReaderV1{values: []contract.ArtifactRelationSourceProjectionV1{source}}, testkit.ArtifactRelationOwnerV1(), clock)
		if _, _, err := controller.CreateArtifactRelationV1(ctx, testkit.ArtifactRelationRequestV1(source)); !contract.HasCode(err, contract.ErrEvidenceNotInspectable) {
			t.Fatalf("unbound event = %v", err)
		}
	})

	t.Run("cross tenant request", func(t *testing.T) {
		request := testkit.ArtifactRelationRequestV1(source)
		request.RelatedFactRef.TenantID = "tenant-2"
		controller, _ := domain.NewArtifactRelationControllerV1(backend, backend, &artifactSourceReaderV1{values: []contract.ArtifactRelationSourceProjectionV1{source}}, testkit.ArtifactRelationOwnerV1(), clock)
		if _, _, err := controller.CreateArtifactRelationV1(ctx, request); !contract.HasCode(err, contract.ErrRevisionConflict) {
			t.Fatalf("cross tenant request = %v", err)
		}
	})
}

func TestArtifactRelationControllerRejectsTypedNil(t *testing.T) {
	var typedNil *artifactSourceReaderV1
	if _, err := domain.NewArtifactRelationControllerV1(memory.New(), memory.New(), typedNil, testkit.ArtifactRelationOwnerV1(), &testkit.Clock{Time: time.Now()}); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("typed nil source reader = %v", err)
	}
}

func TestArtifactRelationUnknownReaderErrorIsClosedIndeterminate(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)}
	record := projectArtifactTimelineEventV1(t, ctx, backend, clock)
	source := testkit.ArtifactSourceProjectionV1(record.EvidenceRecordRef, record.EvidenceRecordDigest)
	reader := &artifactSourceReaderV1{err: errors.New("opaque provider error")}
	controller, err := domain.NewArtifactRelationControllerV1(backend, backend, reader, testkit.ArtifactRelationOwnerV1(), clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := controller.CreateArtifactRelationV1(ctx, testkit.ArtifactRelationRequestV1(source)); !contract.HasCode(err, contract.ErrIndeterminate) {
		t.Fatalf("unclassified reader error = %v", err)
	}
}

func projectArtifactTimelineEventV1(t *testing.T, ctx context.Context, backend *memory.Backend, clock *testkit.Clock) contract.TimelineEventRecord {
	t.Helper()
	timeline, err := domain.NewReferenceTimeline(backend, clock, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	candidate := testkit.Candidate(1, 1, contract.TrustObservation)
	source := testkit.ArtifactSourceProjectionV1(candidate.Evidence.RecordRef, candidate.Evidence.RecordDigest)
	candidate.ObjectRefs = []string{source.Artifact.ArtifactFactRef.ID, source.RelatedFactRef.ID}
	candidate.Digest, _ = candidate.CanonicalDigest()
	record, _, err := timeline.Project(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	return record
}
