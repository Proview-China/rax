package memory

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestBlackBoxDeterministicQueryCitationCoverage(t *testing.T) {
	f := newFixture(t)
	c := f.candidate("candidate", CandidateCreate, "blue preference blue", contract.Ref{}, 1)
	a := f.submitReady(c, "admission")
	record, _, err := f.commit(c, a, "attempt", "result", "record", contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	projection := SealProjection(Projection{Ref: contract.Ref{ID: "lexical", Revision: 1}, TenantID: f.access.TenantID, RecordRef: record.Ref, Kind: "lexical", BuilderVersion: "deterministic-v1", State: ProjectionPartial, Coverage: contract.Coverage{Status: contract.CoveragePartial, Expected: 2, Available: 1}, CreatedAt: f.now, ExpiresAt: f.now.Add(time.Hour)})
	projection, err = f.store.PutProjection(f.access, projection, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	watermark, err := f.store.CurrentWatermark(f.access)
	if err != nil {
		t.Fatal(err)
	}
	view := SealView(View{Ref: contract.Ref{ID: "view", Revision: 1}, TenantID: f.access.TenantID, PrincipalID: f.access.IdentityID, AuthorityRef: f.access.AuthorityRef, AuthorityEpoch: f.access.AuthorityEpoch, PolicyRef: f.access.PolicyRef, Purpose: "assist", Scopes: []string{"identity_private"}, SensitivityMax: "internal", WatermarkRef: watermark.Ref, ProjectionRefs: []contract.Ref{projection.Ref}, CreatedAt: f.now, ExpiresAt: f.now.Add(time.Hour)})
	view, err = f.store.PublishView(f.access, view, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	query := contract.RetrievalQuery{ID: "query", Revision: 1, Domain: contract.OwnerMemory, ViewRef: view.Ref, Purpose: "assist", Text: "blue", Scopes: []string{"identity_private"}, SensitivityMax: "internal", Limit: 10, RequestedAt: f.now, ExpiresAt: f.now.Add(time.Minute)}
	one, err := f.store.Query(f.access, query)
	if err != nil {
		t.Fatal(err)
	}
	two, err := f.store.Query(f.access, query)
	if err != nil {
		t.Fatal(err)
	}
	if one.ResultDigest != two.ResultDigest || one.EvidenceDigest != two.EvidenceDigest {
		t.Fatalf("query is not deterministic: %s/%s %s/%s", one.ResultDigest, two.ResultDigest, one.EvidenceDigest, two.EvidenceDigest)
	}
	if len(one.Hits) != 1 || !contract.SameRef(one.Hits[0].Citation.RecordRef, record.Ref) || one.Hits[0].Citation.ContentRef.Digest != record.ContentRef.Digest {
		t.Fatalf("citation does not bind exact record/content: %#v", one.Hits)
	}
	if one.Coverage.Status != contract.CoveragePartial || len(one.Coverage.ProjectionRefs) != 1 {
		t.Fatalf("partial coverage hidden: %#v", one.Coverage)
	}
}

func TestBlackBoxTombstoneWatermarkDoesNotFallBack(t *testing.T) {
	f := newFixture(t)
	create := f.candidate("create", CandidateCreate, "forgotten secret", contract.Ref{}, 1)
	a1 := f.submitReady(create, "admission-create")
	r1, _, err := f.commit(create, a1, "attempt-create", "result-create", "record", contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	tombstone := f.candidate("forget", CandidateTombstone, "", r1.Ref, 2)
	a2 := f.submitReady(tombstone, "admission-forget")
	if _, _, err := f.store.Tombstone(f.access, CommitRequest{TenantID: f.access.TenantID, AttemptID: "attempt-forget", ResultID: "result-forget", RecordID: "record", CandidateRef: tombstone.Ref(), AdmissionRef: a2.Ref, OperationRef: ref("operation-forget"), ExpectedRevision: contract.ExpectRevision(1)}); err != nil {
		t.Fatal(err)
	}
	watermark, _ := f.store.CurrentWatermark(f.access)
	view := SealView(View{Ref: contract.Ref{ID: "view", Revision: 1}, TenantID: f.access.TenantID, PrincipalID: f.access.IdentityID, AuthorityRef: f.access.AuthorityRef, AuthorityEpoch: f.access.AuthorityEpoch, PolicyRef: f.access.PolicyRef, Purpose: "assist", Scopes: []string{"identity_private"}, SensitivityMax: "internal", WatermarkRef: watermark.Ref, CreatedAt: f.now, ExpiresAt: f.now.Add(time.Hour)})
	view, err = f.store.PublishView(f.access, view, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.store.Query(f.access, contract.RetrievalQuery{ID: "query", Revision: 1, Domain: contract.OwnerMemory, ViewRef: view.Ref, Purpose: "assist", Text: "forgotten", Scopes: []string{"identity_private"}, SensitivityMax: "internal", Limit: 10, RequestedAt: f.now, ExpiresAt: f.now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 0 {
		t.Fatalf("tombstone leaked older active revision: %#v", result.Hits)
	}
}

func TestBlackBoxOwnerTenantAuthorityIsolation(t *testing.T) {
	f := newFixture(t)
	c := f.candidate("candidate", CandidateCreate, "isolated", contract.Ref{}, 1)
	a := f.submitReady(c, "admission")
	record, _, err := f.commit(c, a, "attempt", "result", "record", contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	wrong := f.access
	wrong.AuthorityEpoch++
	if _, err := f.store.InspectRecord(wrong, record.Ref); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("stale authority read historical record: %v", err)
	}
	if _, _, err := f.store.InspectCommit(wrong, "attempt"); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("stale authority inspected commit: %v", err)
	}
	query := contract.RetrievalQuery{Domain: contract.OwnerKnowledge}
	if _, err := f.store.Query(f.access, query); !errors.Is(err, contract.ErrScopeDenied) {
		t.Fatalf("knowledge owner query entered memory: %v", err)
	}
}
