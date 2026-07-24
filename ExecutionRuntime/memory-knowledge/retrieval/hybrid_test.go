package retrieval

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func hybridHit(id string, projection contract.Ref, score int) contract.RetrievalHit {
	record := ref(id, 1)
	content := contract.ContentRef{ID: "content-" + id, Digest: "sha256:content-" + id, Length: 10, MediaType: "text/plain"}
	return contract.RetrievalHit{
		RecordRef: record, Score: score, MatchReason: "channel", Scope: "user", Subject: id,
		ProjectionRefs: []contract.Ref{projection}, Citation: contract.Citation{Domain: contract.OwnerMemory, RecordRef: record, SourceRefs: []contract.Ref{ref("source-"+id, 1)}, ContentRef: content, RangeEnd: 10, Current: true, SummaryDigest: "sha256:summary"},
	}
}

func hybridRequest(t *testing.T, now time.Time) HybridRequestV1 {
	t.Helper()
	request, err := SealHybridRequestV1(HybridRequestV1{
		Query:    contract.RetrievalQuery{ID: "query", Revision: 1, Domain: contract.OwnerMemory, ViewRef: ref("view", 1), Purpose: "assist", Text: "alpha", Scopes: []string{"user"}, SensitivityMax: "internal", Limit: 2, RequestedAt: now, ExpiresAt: now.Add(time.Hour)},
		Channels: []ChannelBudgetV1{{Kind: contract.IndexVector, Limit: 3, Weight: 1}, {Kind: contract.IndexLexical, Limit: 3, Weight: 1}}, RRFK: 60, MaxCandidates: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func channel(t *testing.T, now time.Time, kind contract.IndexKind, projection contract.Ref, hits []contract.RetrievalHit) ChannelObservationV1 {
	t.Helper()
	observation, err := SealChannelObservationV1(ChannelObservationV1{
		Kind: kind, ProjectionRef: projection, ViewRef: ref("view", 1), WatermarkRef: ref("watermark", 9), Hits: hits,
		Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: len(hits), Available: len(hits), ProjectionRefs: []contract.Ref{projection}}, ObservedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func TestHybridRRFDeterministicBudgetCoverageAndCursor(t *testing.T) {
	now := time.Date(2026, 7, 17, 7, 0, 0, 0, time.UTC)
	request := hybridRequest(t, now)
	lexicalRef, vectorRef := ref("lexical", 1), ref("vector", 1)
	lexical := channel(t, now, contract.IndexLexical, lexicalRef, []contract.RetrievalHit{hybridHit("a", lexicalRef, 100), hybridHit("b", lexicalRef, 90), hybridHit("c", lexicalRef, 80)})
	vector := channel(t, now, contract.IndexVector, vectorRef, []contract.RetrievalHit{hybridHit("b", vectorRef, 100), hybridHit("a", vectorRef, 90), hybridHit("c", vectorRef, 80)})
	result, err := MergeHybridV1(now, request, []ChannelObservationV1{vector, lexical})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 2 || result.Hits[0].RecordRef.ID != "a" || result.Hits[1].RecordRef.ID != "b" || result.NextCursor == "" || result.Coverage.Status != contract.CoverageComplete {
		t.Fatalf("unexpected hybrid result: %+v", result)
	}
	reversed, err := MergeHybridV1(now, request, []ChannelObservationV1{lexical, vector})
	if err != nil || reversed.ResultDigest != result.ResultDigest {
		t.Fatalf("insertion order changed result: %s %s %v", result.ResultDigest, reversed.ResultDigest, err)
	}
	pageRequest := request
	pageRequest.Query.Cursor = result.NextCursor
	pageRequest.Digest = ""
	pageRequest, err = SealHybridRequestV1(pageRequest)
	if err != nil {
		t.Fatal(err)
	}
	page, err := MergeHybridV1(now, pageRequest, []ChannelObservationV1{lexical, vector})
	if err != nil || len(page.Hits) != 1 || page.Hits[0].RecordRef.ID != "c" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	drift := pageRequest
	drift.Query.ViewRef = ref("other-view", 1)
	drift.Digest = ""
	drift, err = SealHybridRequestV1(drift)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MergeHybridV1(now, drift, []ChannelObservationV1{lexical, vector}); !errors.Is(err, contract.ErrNotCurrent) {
		t.Fatalf("cursor drift accepted: %v", err)
	}
}

func TestHybridRejectsStaleTamperRevisionAndContentDrift(t *testing.T) {
	now := time.Date(2026, 7, 17, 7, 0, 0, 0, time.UTC)
	request := hybridRequest(t, now)
	lexicalRef, vectorRef := ref("lexical", 1), ref("vector", 1)
	lexical := channel(t, now, contract.IndexLexical, lexicalRef, []contract.RetrievalHit{hybridHit("a", lexicalRef, 100)})
	vectorHit := hybridHit("a", vectorRef, 100)
	vector := channel(t, now, contract.IndexVector, vectorRef, []contract.RetrievalHit{vectorHit})
	tampered := lexical
	tampered.Hits[0].Subject = "tampered"
	if _, err := MergeHybridV1(now, request, []ChannelObservationV1{tampered, vector}); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("tamper accepted: %v", err)
	}
	stale := channel(t, now, contract.IndexLexical, lexicalRef, []contract.RetrievalHit{hybridHit("a", lexicalRef, 100)})
	if _, err := MergeHybridV1(now.Add(2*time.Minute), request, []ChannelObservationV1{stale, vector}); err == nil {
		t.Fatal("stale observation accepted")
	}
	revisionDrift := vector
	revisionDrift.Hits[0] = hybridHit("a", vectorRef, 100)
	revisionDrift.Hits[0].RecordRef = ref("a", 2)
	revisionDrift.Hits[0].Citation.RecordRef = revisionDrift.Hits[0].RecordRef
	revisionDrift, _ = SealChannelObservationV1(revisionDrift)
	if _, err := MergeHybridV1(now, request, []ChannelObservationV1{lexical, revisionDrift}); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("revision drift accepted: %v", err)
	}
	contentDrift := vector
	contentDrift.Hits[0].Citation.ContentRef.Digest = "sha256:other"
	contentDrift, _ = SealChannelObservationV1(contentDrift)
	if _, err := MergeHybridV1(now, request, []ChannelObservationV1{lexical, contentDrift}); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("content drift accepted: %v", err)
	}
}
