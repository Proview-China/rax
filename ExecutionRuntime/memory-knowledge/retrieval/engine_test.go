package retrieval

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestDeterministicRetrievalDifferentInsertionOrder(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	view := ref("view", 1)
	watermark := ref("watermark", 3)
	query := contract.RetrievalQuery{
		ID: "query", Revision: 1, Domain: contract.OwnerMemory, ViewRef: view,
		Purpose: "answer", Text: "alpha beta", Scopes: []string{"user"},
		SensitivityMax: "internal", Limit: 10, RequestedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	docs := []Document{
		doc(contract.OwnerMemory, "record-b", "alpha beta", "user", "public"),
		doc(contract.OwnerMemory, "record-a", "alpha beta", "user", "public"),
		doc(contract.OwnerMemory, "record-c", "alpha", "user", "public"),
	}
	first, err := Search(now, query, watermark, docs, contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1})
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(docs)
	for range 100 {
		got, err := Search(now, query, watermark, docs, contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, got) {
			t.Fatalf("retrieval changed with insertion order:\nfirst=%+v\ngot=%+v", first, got)
		}
	}
	if first.Hits[0].RecordRef.ID != "record-a" || first.Hits[1].RecordRef.ID != "record-b" {
		t.Fatalf("tie break is not record ID ascending: %+v", first.Hits)
	}
}

func TestCursorBindsWatermarkAndPartialCoverageIsOrthogonalToHits(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	query := contract.RetrievalQuery{
		ID: "query", Revision: 1, Domain: contract.OwnerKnowledge, ViewRef: ref("view", 1),
		Purpose: "answer", Text: "missing", Scopes: []string{"domain"},
		SensitivityMax: "public", Limit: 1, RequestedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	partial := contract.Coverage{Status: contract.CoveragePartial, Expected: 2, Available: 1, DroppedReasons: []string{"stale", "stale"}}
	result, err := Search(now, query, ref("watermark", 1), nil, partial)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 0 || result.Coverage.Status != contract.CoveragePartial {
		t.Fatalf("zero hits and partial coverage were conflated: %+v", result)
	}

	query.Text = "alpha"
	query.Limit = 1
	docs := []Document{doc(contract.OwnerKnowledge, "a", "alpha", "domain", "public"), doc(contract.OwnerKnowledge, "b", "alpha", "domain", "public")}
	page, err := Search(now, query, ref("watermark", 1), docs, partial)
	if err != nil {
		t.Fatal(err)
	}
	if page.NextCursor == "" {
		t.Fatal("expected next cursor")
	}
	query.Cursor = page.NextCursor
	if _, err := Search(now, query, ref("watermark", 2), docs, partial); err == nil {
		t.Fatal("cursor must fail when watermark changes")
	}
}

func TestFiltersBeforeRankingAndCitationIsExact(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	query := contract.RetrievalQuery{
		ID: "query", Revision: 1, Domain: contract.OwnerMemory, ViewRef: ref("view", 1),
		Purpose: "answer", Text: "alpha", Scopes: []string{"user"}, SensitivityMax: "internal",
		Limit: 10, RequestedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	allowed := doc(contract.OwnerMemory, "allowed", "alpha", "user", "public")
	denied := doc(contract.OwnerMemory, "denied", "alpha alpha alpha", "organization", "restricted")
	result, err := Search(now, query, ref("watermark", 1), []Document{denied, allowed}, contract.Coverage{Status: contract.CoverageComplete})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].RecordRef.ID != "allowed" {
		t.Fatalf("scope/sensitivity filter did not run before rank: %+v", result.Hits)
	}
	citation := result.Hits[0].Citation
	if !contract.SameRef(citation.RecordRef, allowed.RecordRef) || citation.RangeEnd != allowed.ContentRef.Length || len(citation.SourceRefs) == 0 {
		t.Fatalf("citation is not exact: %+v", citation)
	}
}

func TestCitationMissingSourceFailsClosed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	query := contract.RetrievalQuery{
		ID: "query", Revision: 1, Domain: contract.OwnerMemory, ViewRef: ref("view", 1),
		Purpose: "answer", Text: "alpha", Scopes: []string{"user"}, SensitivityMax: "public",
		Limit: 1, RequestedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	invalid := doc(contract.OwnerMemory, "record", "alpha", "user", "public")
	invalid.SourceRefs = nil
	if _, err := Search(now, query, ref("watermark", 1), []Document{invalid}, contract.Coverage{Status: contract.CoverageComplete}); err == nil {
		t.Fatal("document without citation source must fail closed")
	}
}

func TestSearchUntrustedCanonicalInputNeverPanics(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	query := contract.RetrievalQuery{
		ID: "query", Revision: 1, Domain: contract.OwnerMemory, ViewRef: ref("view", 1),
		Purpose: "answer", Text: "alpha", Scopes: []string{"user"}, SensitivityMax: "public",
		Limit: 1, RequestedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(10000, 1, 1, 1, 0, 0, 0, time.UTC),
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("untrusted retrieval query panicked: %v", recovered)
		}
	}()
	_, err := Search(now, query, ref("watermark", 1), nil, contract.Coverage{Status: contract.CoverageComplete})
	if !errors.Is(err, contract.ErrInvalidArgument) {
		t.Fatalf("uncanonicalizable query error = %v, want invalid argument", err)
	}
}

func ref(id string, revision uint64) contract.Ref {
	return contract.Ref{ID: id, Revision: revision, Digest: contract.MustDigest(struct {
		ID       string
		Revision uint64
	}{id, revision})}
}

func doc(domain contract.OwnerDomain, id, text, scope, sensitivity string) Document {
	return Document{
		Domain: domain, RecordRef: ref(id, 1),
		ContentRef: contract.ContentRef{ID: "content-" + id, Digest: contract.MustDigest(text), Length: int64(len(text)), MediaType: "text/plain"},
		Text:       text, Scope: scope, Subject: "subject", Sensitivity: sensitivity, Current: true,
		SourceRefs: []contract.Ref{ref("source-"+id, 1)}, EvidenceRefs: []contract.Ref{ref("evidence-"+id, 1)},
		ProjectionRefs: []contract.Ref{ref("projection", 1)},
	}
}
