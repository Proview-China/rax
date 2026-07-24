package knowledge

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func buildPublishedView(t *testing.T, f *fixture) (View, Projection) {
	t.Helper()
	projection, err := f.store.PutProjection(f.access, ProjectionInput{
		TenantID: f.access.TenantID, ID: "projection-export", Kind: string(contract.IndexLexical), RecordRefs: []contract.Ref{f.record.Ref},
		BuilderVersion: "lexical-v1", Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1}, State: ProjectionReady, TTL: time.Hour,
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	ready, err := f.store.CreateSnapshot(f.access, SnapshotInput{
		TenantID: f.access.TenantID, ID: "snapshot-export", Version: "v1", SourceRefs: []contract.Ref{f.source.Ref},
		PackageRefs: []contract.Ref{f.pkg.Ref}, RecordRefs: []contract.Ref{f.record.Ref}, ProjectionRefs: []contract.Ref{projection.Ref},
		Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1},
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	_, snapshot, err := f.store.PublishSnapshot(f.access, ready.Ref, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	view, err := f.store.CreateView(f.access, ViewInput{
		TenantID: f.access.TenantID, ID: "view-export", SnapshotRef: snapshot.Ref, AuthorityRef: f.access.AuthorityRef, PolicyRef: f.access.PolicyRef,
		ProjectionRefs: []contract.Ref{projection.Ref}, Scopes: []string{f.record.Scope}, AllowedLicenses: []string{f.record.License},
		SensitivityMax: "internal", Purpose: "export", CurrentOnly: true, TTL: time.Hour,
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	return view, projection
}

func TestKnowledgeExportExactRefsAndWithdrawFailClosed(t *testing.T) {
	f := newFixture(t, true)
	view, _ := buildPublishedView(t, f)
	manifest, err := f.store.ExportView(f.access, view.Ref, "knowledge-export", time.Minute)
	if err != nil || len(manifest.Entries) != 1 || !contract.SameRef(manifest.Entries[0].RecordRef, f.record.Ref) || manifest.Entries[0].ContentRef == nil || *manifest.Entries[0].ContentRef != f.record.ContentRef {
		t.Fatalf("manifest=%+v err=%v", manifest, err)
	}
	if _, _, err := f.store.WithdrawSource(f.access, f.source.Ref.ID, "withdrawn", contract.ExpectRevision(f.source.Ref.Revision)); err != nil {
		t.Fatal(err)
	}
	after, err := f.store.ExportView(f.access, view.Ref, "knowledge-export-after", time.Minute)
	if err != nil || len(after.Entries) != 0 {
		t.Fatalf("withdrawn source remained exportable: %+v %v", after, err)
	}
	tampered := manifest
	tampered.Entries[0].License = "forged"
	if err := tampered.Validate(*f.now); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("tampered export accepted: %v", err)
	}
}

func TestKnowledgeReindexPublishesExactDescriptor(t *testing.T) {
	f := newFixture(t, true)
	input := ProjectionInput{TenantID: f.access.TenantID, ID: "projection-reindex", Kind: string(contract.IndexGraph), RecordRefs: []contract.Ref{f.record.Ref}, BuilderVersion: "graph-v1", Coverage: contract.Coverage{Status: contract.CoveragePartial, Expected: 2, Available: 1, DroppedReasons: []string{"relationship_unverified"}}, State: ProjectionPartial, TTL: time.Hour}
	descriptor := contract.IndexDescriptorV1{Ref: contract.Ref{ID: "index-reindex", Revision: 1}, Kind: contract.IndexGraph, ViewRef: ref("view-reindex"), BoundaryRef: ref("snapshot-reindex"), BuilderRef: ref("builder-reindex"), BuilderVersion: "graph-v1", IndexVersion: "v1", State: contract.IndexPartial, Coverage: input.Coverage, CreatedAt: *f.now, ExpiresAt: f.now.Add(time.Hour)}
	projection, index, err := f.store.ReindexLocal(f.access, input, contract.ExpectAbsent(), descriptor, contract.ExpectAbsent())
	if err != nil || len(index.RecordRefs) != 1 || !contract.SameRef(index.RecordRefs[0], f.record.Ref) || len(index.Coverage.ProjectionRefs) != 1 || !contract.SameRef(index.Coverage.ProjectionRefs[0], projection.Ref) {
		t.Fatalf("projection=%+v index=%+v err=%v", projection, index, err)
	}
	listed, err := f.store.ListIndexDescriptors(f.access)
	if err != nil || len(listed) != 1 || !contract.SameRef(listed[0].Ref, index.Ref) {
		t.Fatalf("listed=%+v err=%v", listed, err)
	}
	wrong := descriptor
	wrong.Ref = contract.Ref{ID: "wrong-index", Revision: 1}
	wrong.Kind = contract.IndexVector
	if _, _, err := f.store.ReindexLocal(f.access, input, contract.ExpectAbsent(), wrong, contract.ExpectAbsent()); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("kind substitution accepted: %v", err)
	}
	conflictingInput := input
	conflictingInput.ID = "projection-conflict"
	conflictingDescriptor := descriptor
	conflictingDescriptor.Ref = contract.Ref{ID: "index-conflict", Revision: 1}
	if _, _, err := f.store.ReindexLocal(f.access, conflictingInput, contract.ExpectAbsent(), conflictingDescriptor, contract.ExpectAbsent()); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("semantic index conflict accepted: %v", err)
	}
	projections, err := f.store.ListProjections(f.access)
	if err != nil || len(projections) != 1 {
		t.Fatalf("failed descriptor left orphan projection: %+v %v", projections, err)
	}
}
