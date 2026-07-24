package memory

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestMemoryExportIsExactMetadataOnlyAndCopyIsolated(t *testing.T) {
	f := newFixture(t)
	_, record := createMemoryRecord(t, f, "record-export", 1)
	watermark, err := f.store.CurrentWatermark(f.access)
	if err != nil {
		t.Fatal(err)
	}
	view := SealView(View{
		Ref: contract.Ref{ID: "view-export", Revision: 1}, TenantID: f.access.TenantID,
		PrincipalID: f.access.IdentityID, AuthorityRef: f.access.AuthorityRef, AuthorityEpoch: f.access.AuthorityEpoch,
		PolicyRef: f.access.PolicyRef, Purpose: "export", Scopes: []string{record.Scope}, SensitivityMax: "internal",
		WatermarkRef: watermark.Ref, CreatedAt: f.now, ExpiresAt: f.now.Add(time.Hour),
	})
	view, err = f.store.PublishView(f.access, view, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := f.store.ExportView(f.access, view.Ref, "memory-export", time.Minute)
	if err != nil || len(manifest.Entries) != 1 || !contract.SameRef(manifest.Entries[0].RecordRef, record.Ref) || manifest.Entries[0].ContentRef == nil || *manifest.Entries[0].ContentRef != *record.ContentRef {
		t.Fatalf("manifest=%+v err=%v", manifest, err)
	}
	manifest.Entries[0].SourceRefs[0] = ref("tampered")
	again, err := f.store.ExportView(f.access, view.Ref, "memory-export-2", time.Minute)
	if err != nil || contract.SameRef(again.Entries[0].SourceRefs[0], manifest.Entries[0].SourceRefs[0]) {
		t.Fatalf("export aliased caller memory: %+v %v", again, err)
	}
	tampered := again
	tampered.Entries[0].Scope = "other"
	if err := tampered.Validate(f.now); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("tampered manifest accepted: %v", err)
	}
}

func TestMemoryReindexPublishesProjectionThenExactDescriptor(t *testing.T) {
	f := newFixture(t)
	_, record := createMemoryRecord(t, f, "record-reindex", 1)
	watermark, _ := f.store.CurrentWatermark(f.access)
	projection := SealProjection(Projection{
		Ref: contract.Ref{ID: "projection-reindex", Revision: 1}, TenantID: f.access.TenantID, RecordRef: record.Ref,
		Kind: string(contract.IndexLexical), BuilderVersion: "lexical-v1", State: ProjectionReady,
		Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1}, CreatedAt: f.now, ExpiresAt: f.now.Add(time.Hour),
	})
	descriptor := contract.IndexDescriptorV1{
		Ref: contract.Ref{ID: "index-reindex", Revision: 1}, Kind: contract.IndexLexical,
		ViewRef: ref("view-reindex"), BoundaryRef: watermark.Ref, BuilderRef: ref("builder-reindex"),
		BuilderVersion: "lexical-v1", IndexVersion: "v1", State: contract.IndexReady,
		Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1}, CreatedAt: f.now, ExpiresAt: f.now.Add(time.Hour),
	}
	published, index, err := f.store.ReindexLocal(f.access, projection, contract.ExpectAbsent(), descriptor, contract.ExpectAbsent())
	if err != nil || !contract.SameRef(index.RecordRefs[0], record.Ref) || len(index.Coverage.ProjectionRefs) != 1 || !contract.SameRef(index.Coverage.ProjectionRefs[0], published.Ref) {
		t.Fatalf("projection=%+v index=%+v err=%v", published, index, err)
	}
	listed, err := f.store.ListIndexDescriptors(f.access)
	if err != nil || len(listed) != 1 || !contract.SameRef(listed[0].Ref, index.Ref) {
		t.Fatalf("listed=%+v err=%v", listed, err)
	}
	wrong := descriptor
	wrong.Ref = contract.Ref{ID: "index-wrong", Revision: 1}
	wrong.Kind = contract.IndexGraph
	if _, _, err := f.store.ReindexLocal(f.access, projection, contract.ExpectAbsent(), wrong, contract.ExpectAbsent()); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("kind substitution accepted: %v", err)
	}
	conflictingProjection := projection
	conflictingProjection.Ref = contract.Ref{ID: "projection-conflict", Revision: 1}
	conflictingProjection = SealProjection(conflictingProjection)
	conflictingDescriptor := descriptor
	conflictingDescriptor.Ref = contract.Ref{ID: "index-conflict", Revision: 1}
	if _, _, err := f.store.ReindexLocal(f.access, conflictingProjection, contract.ExpectAbsent(), conflictingDescriptor, contract.ExpectAbsent()); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("semantic index conflict accepted: %v", err)
	}
	projections, err := f.store.ListProjections(f.access)
	if err != nil || len(projections) != 1 {
		t.Fatalf("failed descriptor left orphan projection: %+v %v", projections, err)
	}
}
