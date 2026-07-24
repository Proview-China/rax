package memory_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/memory"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/reference"
)

func TestBlackBoxPublicMemoryPort(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	ref := func(id string) contract.Ref { return contract.Ref{ID: id, Revision: 1, Digest: "sha256:" + id} }
	content := reference.NewStore()
	contentRef, err := content.Put([]byte("public memory port"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	store := memory.NewStore(contract.ClockFunc(func() time.Time { return now }), content)
	access := memory.Access{TenantID: "tenant", IdentityID: "identity", AuthorityRef: ref("authority"), AuthorityEpoch: 1, PolicyRef: ref("policy")}
	candidate := memory.SealCandidate(memory.Candidate{
		Envelope: contract.Envelope{ContractVersion: contract.VersionV1, SchemaRef: "praxis.memory/candidate-v1", ID: "candidate", Revision: 1, TenantID: access.TenantID, IdentityID: access.IdentityID, IdentityEpoch: 1, AuthorityRef: access.AuthorityRef, AuthorityEpoch: access.AuthorityEpoch, PolicyRef: access.PolicyRef, Purpose: "assist", ActionScopeDigest: "sha256:scope", CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Hour)},
		Kind:     memory.CandidateCreate, ProducerRef: ref("producer"), SourceEpoch: 1, SourceSequence: 1, Scope: "identity_private", Subject: "subject", ContentRef: &contentRef, SourceRefs: []contract.Ref{ref("source")}, EvidenceRefs: []contract.Ref{ref("evidence")}, Sensitivity: "internal",
	})
	if _, err := store.SubmitCandidate(access, candidate); err != nil {
		t.Fatal(err)
	}
	admission, err := store.Admit(access, memory.AdmissionRequest{ID: "admission", CandidateRef: candidate.Ref(), Decision: memory.AdmissionCommitReady, ExpiresAt: now.Add(time.Hour), ExpectedRevision: contract.ExpectAbsent()})
	if err != nil {
		t.Fatal(err)
	}
	record, result, err := store.Commit(access, memory.CommitRequest{TenantID: access.TenantID, AttemptID: "attempt", ResultID: "result", RecordID: "record", CandidateRef: candidate.Ref(), AdmissionRef: admission.Ref, OperationRef: ref("operation"), ExpectedRevision: contract.ExpectAbsent()})
	if err != nil {
		t.Fatal(err)
	}
	if record.Owner != contract.OwnerMemory || result.Owner != contract.OwnerMemory || result.State != contract.DomainResultReady {
		t.Fatalf("public port crossed owner/settlement boundary: %#v %#v", record, result)
	}
	watermark, err := store.CurrentWatermark(access)
	if err != nil {
		t.Fatal(err)
	}
	view := memory.SealView(memory.View{Ref: contract.Ref{ID: "view", Revision: 1}, TenantID: access.TenantID, PrincipalID: access.IdentityID, AuthorityRef: access.AuthorityRef, AuthorityEpoch: access.AuthorityEpoch, PolicyRef: access.PolicyRef, Purpose: "assist", Scopes: []string{"identity_private"}, SensitivityMax: "internal", WatermarkRef: watermark.Ref, CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	view, err = store.PublishView(access, view, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Query(access, contract.RetrievalQuery{ID: "query", Revision: 1, Domain: contract.OwnerMemory, ViewRef: view.Ref, Purpose: "assist", Text: "memory", Scopes: []string{"identity_private"}, SensitivityMax: "internal", Limit: 1, RequestedAt: now, ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 1 || !contract.SameRef(got.Hits[0].Citation.RecordRef, record.Ref) {
		t.Fatalf("public query/citation mismatch: %#v", got)
	}
}
