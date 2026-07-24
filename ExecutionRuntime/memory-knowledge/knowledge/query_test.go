package knowledge

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestKnowledgeBlackBoxDeterministicQueryCitationCoverage(t *testing.T) {
	f := newFixture(t, true)
	projection, err := f.store.PutProjection(f.access, ProjectionInput{
		TenantID: f.access.TenantID, ID: "projection-a", Kind: "lexical", RecordRefs: []contract.Ref{f.record.Ref},
		BuilderVersion: "deterministic-lexical-v1", Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1},
		State: ProjectionReady, TTL: time.Hour,
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	ready, err := f.store.CreateSnapshot(f.access, SnapshotInput{
		TenantID: f.access.TenantID, ID: "snapshot-query", Version: "v1", SourceRefs: []contract.Ref{f.source.Ref},
		PackageRefs: []contract.Ref{f.pkg.Ref}, RecordRefs: []contract.Ref{f.record.Ref}, ProjectionRefs: []contract.Ref{projection.Ref},
		Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1},
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	_, published, err := f.store.PublishSnapshot(f.access, ready.Ref, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	view, err := f.store.CreateView(f.access, ViewInput{
		TenantID: f.access.TenantID, ID: "view-a", SnapshotRef: published.Ref,
		AuthorityRef: f.access.AuthorityRef, PolicyRef: f.access.PolicyRef, ProjectionRefs: []contract.Ref{projection.Ref},
		Scopes: []string{"project-a"}, AllowedLicenses: []string{"internal-use"}, SensitivityMax: "internal",
		Purpose: "answer", CurrentOnly: true, TTL: time.Hour,
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	query := contract.RetrievalQuery{
		ID: "query-a", Revision: 1, Domain: contract.OwnerKnowledge, ViewRef: view.Ref, Purpose: view.Purpose,
		Text: "alpha knowledge", Scopes: []string{"project-a"}, SensitivityMax: "internal", Limit: 10,
		RequestedAt: *f.now, ExpiresAt: f.now.Add(time.Hour),
	}
	first, err := f.store.Query(f.access, query, f.content)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.store.Query(f.access, query, f.content)
	if err != nil {
		t.Fatal(err)
	}
	if first.ResultDigest != second.ResultDigest || first.EvidenceDigest != second.EvidenceDigest {
		t.Fatalf("retrieval was not deterministic: first=%s second=%s", first.ResultDigest, second.ResultDigest)
	}
	if len(first.Hits) != 1 || first.Coverage.Status != contract.CoverageComplete {
		t.Fatalf("unexpected result/coverage: %+v", first)
	}
	hit := first.Hits[0]
	if !contract.SameRef(hit.RecordRef, f.record.Ref) || !contract.SameRef(hit.SnapshotRef, published.Ref) || !contract.SameRef(hit.PackageRef, f.pkg.Ref) {
		t.Fatalf("watermark refs missing: %+v", hit)
	}
	if len(hit.Citation.SourceRefs) != 1 || !contract.SameRef(hit.Citation.SourceRefs[0], f.source.Ref) || !contract.SameRef(hit.Citation.RecordRef, f.record.Ref) {
		t.Fatalf("citation missing exact source/record: %+v", hit.Citation)
	}

	_, _, err = f.store.WithdrawSource(f.access, f.source.Ref.ID, "source revoked", contract.ExpectRevision(f.source.Ref.Revision))
	if err != nil {
		t.Fatal(err)
	}
	partial, err := f.store.Query(f.access, query, f.content)
	if err != nil {
		t.Fatal(err)
	}
	if len(partial.Hits) != 0 || partial.Coverage.Status != contract.CoverageNone || len(partial.Coverage.DroppedReasons) == 0 {
		t.Fatalf("withdrawal was hidden: %+v", partial)
	}

	*f.now = view.ExpiresAt
	if _, err := f.store.Query(f.access, query, f.content); !errors.Is(err, contract.ErrNotCurrent) {
		t.Fatalf("TTL boundary now==expires accepted: %v", err)
	}
}
