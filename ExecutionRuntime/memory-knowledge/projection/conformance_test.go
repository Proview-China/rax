package projection_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/projection"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/projection/graph"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/projection/lexical"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/projection/skill"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/projection/vector"
)

func projectionRef(id string) contract.Ref {
	return contract.Ref{ID: id, Revision: 1, Digest: "sha256:" + id}
}

func metadata(now time.Time, id, kind string) projection.MetadataV1 {
	return projection.MetadataV1{
		Owner: contract.OwnerMemory, ProjectionRef: projectionRef(kind), RecordRef: projectionRef(id),
		ContentRef: contract.ContentRef{ID: "content-" + id, Digest: "sha256:content-" + id, Length: 100, MediaType: "text/plain"},
		SourceRefs: []contract.Ref{projectionRef("source-" + id)}, EvidenceRefs: []contract.Ref{projectionRef("evidence-" + id)},
		Scope: "identity_private", Subject: id, Sensitivity: "internal", CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
}

func TestFourProjectionReferenceBackendsDeterministicAndCanonical(t *testing.T) {
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	detail := contract.ContentRef{ID: "detail", Digest: "sha256:detail", Length: 10, MediaType: "text/plain"}
	skillA, err := skill.SealEntryV1(skill.EntryV1{Ref: contract.Ref{ID: "skill-a", Revision: 1}, Metadata: metadata(now, "record-a", "skill-index"), Title: "Deploy service", Description: "repeatable deployment", Keywords: []string{"deploy", "service"}, UseWhen: []string{"release service"}, DoNotUseWhen: []string{"rollback"}, DetailRef: detail})
	if err != nil {
		t.Fatal(err)
	}
	skillB, err := skill.SealEntryV1(skill.EntryV1{Ref: contract.Ref{ID: "skill-b", Revision: 1}, Metadata: metadata(now, "record-b", "skill-index"), Title: "Inspect logs", Description: "debug failure", Keywords: []string{"logs"}, UseWhen: []string{"debug"}, DetailRef: detail})
	if err != nil {
		t.Fatal(err)
	}
	hits1, err := skill.Search(now, "deploy service", 10, []skill.EntryV1{skillB, skillA})
	if err != nil || len(hits1) != 1 || hits1[0].RecordRef.ID != "record-a" {
		t.Fatalf("skill=%+v %v", hits1, err)
	}
	hits2, _ := skill.Search(now, "deploy service", 10, []skill.EntryV1{skillA, skillB})
	if !reflect.DeepEqual(hits1, hits2) {
		t.Fatal("skill insertion order changed output")
	}
	if denied, _ := skill.Search(now, "rollback deploy", 10, []skill.EntryV1{skillA}); len(denied) != 0 {
		t.Fatal("do-not-use condition ignored")
	}

	lexA, err := lexical.BuildEntryV1(contract.Ref{ID: "lex-a", Revision: 1}, metadata(now, "record-a", "lexical-index"), "alpha alpha beta")
	if err != nil {
		t.Fatal(err)
	}
	lexB, err := lexical.BuildEntryV1(contract.Ref{ID: "lex-b", Revision: 1}, metadata(now, "record-b", "lexical-index"), "alpha gamma")
	if err != nil {
		t.Fatal(err)
	}
	lexHits, err := lexical.Search(now, "alpha", 10, []lexical.EntryV1{lexB, lexA})
	if err != nil || len(lexHits) != 2 || lexHits[0].RecordRef.ID != "record-a" {
		t.Fatalf("lexical=%+v %v", lexHits, err)
	}

	model := projectionRef("embedding-model")
	vecA, err := vector.SealEntryV1(vector.EntryV1{Ref: contract.Ref{ID: "vec-a", Revision: 1}, Metadata: metadata(now, "record-a", "vector-index"), ModelRef: model, Dimension: 2, ChunkEnd: 100, Vector: []float64{1, 0}})
	if err != nil {
		t.Fatal(err)
	}
	vecB, err := vector.SealEntryV1(vector.EntryV1{Ref: contract.Ref{ID: "vec-b", Revision: 1}, Metadata: metadata(now, "record-b", "vector-index"), ModelRef: model, Dimension: 2, ChunkEnd: 100, Vector: []float64{0.5, 0.5}})
	if err != nil {
		t.Fatal(err)
	}
	vecHits, err := vector.Search(now, []float64{1, 0}, model, 10, []vector.EntryV1{vecB, vecA})
	if err != nil || len(vecHits) != 2 || vecHits[0].RecordRef.ID != "record-a" {
		t.Fatalf("vector=%+v %v", vecHits, err)
	}

	edgeA, err := graph.SealEdgeV1(graph.EdgeV1{Ref: contract.Ref{ID: "edge-a", Revision: 1}, Metadata: metadata(now, "record-a", "graph-index"), From: "service-a", Relation: "depends_on", To: "db-a", ConfidenceBPS: 9000, ValidFrom: now.Add(-time.Hour), ValidTo: now.Add(time.Hour), TransactionAt: now})
	if err != nil {
		t.Fatal(err)
	}
	edgeB, err := graph.SealEdgeV1(graph.EdgeV1{Ref: contract.Ref{ID: "edge-b", Revision: 1}, Metadata: metadata(now, "record-b", "graph-index"), From: "service-b", Relation: "depends_on", To: "db-b", ConfidenceBPS: 8000, ValidFrom: now.Add(-time.Hour), ValidTo: now.Add(time.Hour), TransactionAt: now})
	if err != nil {
		t.Fatal(err)
	}
	graphHits, err := graph.Search(now, []string{"service-a"}, "depends_on", 10, []graph.EdgeV1{edgeB, edgeA})
	if err != nil || len(graphHits) != 1 || graphHits[0].RecordRef.ID != "record-a" {
		t.Fatalf("graph=%+v %v", graphHits, err)
	}

	tamperedSkill := skillA
	tamperedSkill.Title = "tampered"
	if err := tamperedSkill.Validate(now); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("skill tamper: %v", err)
	}
	tamperedLexical := lexA
	tamperedLexical.TermFrequency["alpha"]++
	if err := tamperedLexical.Validate(now); err == nil {
		t.Fatal("lexical tamper accepted")
	}
	tamperedVector := vecA
	tamperedVector.Vector[0] = 0.1
	if err := tamperedVector.Validate(now); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("vector tamper: %v", err)
	}
	tamperedGraph := edgeA
	tamperedGraph.ConfidenceBPS--
	if err := tamperedGraph.Validate(now); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("graph tamper: %v", err)
	}
}

func TestProjectionEntriesExpireFailClosed(t *testing.T) {
	now := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	entry, err := lexical.BuildEntryV1(contract.Ref{ID: "lex", Revision: 1}, metadata(now, "record", "lexical-index"), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lexical.Search(now.Add(2*time.Hour), "alpha", 1, []lexical.EntryV1{entry}); err == nil {
		t.Fatal("expired projection accepted")
	}
}
