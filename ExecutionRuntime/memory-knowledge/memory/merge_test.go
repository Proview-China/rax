package memory

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func createMemoryRecord(t *testing.T, f *fixture, id string, sequence uint64) (Candidate, Record) {
	t.Helper()
	candidate := f.candidate("candidate-"+id, CandidateCreate, "content "+id, contract.Ref{}, sequence)
	admission := f.submitReady(candidate, "admission-"+id)
	record, _, err := f.commit(candidate, admission, "attempt-"+id, "result-"+id, id, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	return candidate, record
}
func TestMemoryMergeCreatesNamedFactAndInvalidatesMergedSource(t *testing.T) {
	f := newFixture(t)
	_, target := createMemoryRecord(t, f, "record-a", 1)
	_, other := createMemoryRecord(t, f, "record-b", 2)
	content, err := f.content.Put([]byte("merged content"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	candidate := Candidate{Envelope: contract.Envelope{ContractVersion: contract.VersionV1, SchemaRef: "praxis.memory/candidate-v1", ID: "candidate-merge", Revision: 1, TenantID: f.access.TenantID, IdentityID: f.access.IdentityID, IdentityEpoch: 3, AuthorityRef: f.access.AuthorityRef, AuthorityEpoch: f.access.AuthorityEpoch, PolicyRef: f.access.PolicyRef, Purpose: target.Purpose, ActionScopeDigest: target.ActionScopeDigest, CreatedAt: f.now, UpdatedAt: f.now, ExpiresAt: f.now.Add(time.Hour), CorrelationID: "merge"}, Kind: CandidateMerge, ProducerRef: ref("consolidator"), SourceEpoch: 12, SourceSequence: 3, Scope: target.Scope, Subject: target.Subject, ContentRef: &content, SourceRefs: []contract.Ref{ref("merge-source")}, EvidenceRefs: []contract.Ref{ref("merge-evidence")}, Sensitivity: target.Sensitivity, TargetRecordRef: target.Ref, MergeSourceRefs: []contract.Ref{other.Ref, target.Ref}}
	candidate = SealCandidate(candidate)
	if _, err = f.store.SubmitCandidate(f.access, candidate); err != nil {
		t.Fatal(err)
	}
	admission, err := f.store.Admit(f.access, AdmissionRequest{ID: "admission-merge", CandidateRef: candidate.Ref(), Decision: AdmissionMerged, MergeTarget: target.Ref, Reason: "duplicate memories", ExpiresAt: f.now.Add(time.Hour), ExpectedRevision: contract.ExpectAbsent()})
	if err != nil {
		t.Fatal(err)
	}
	merged, _, err := f.store.Merge(f.access, CommitRequest{TenantID: f.access.TenantID, AttemptID: "attempt-merge", ResultID: "result-merge", RecordID: target.Ref.ID, CandidateRef: candidate.Ref(), AdmissionRef: admission.Ref, OperationRef: ref("operation-merge"), ExpectedRevision: contract.ExpectRevision(target.Ref.Revision)})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.MergeSourceRefs) != 2 {
		t.Fatalf("merge record=%+v", merged)
	}
	f.store.mu.RLock()
	mergedInto := f.store.tenants[f.access.TenantID].mergedInto[other.Ref.ID]
	f.store.mu.RUnlock()
	if !contract.SameRef(mergedInto, merged.Ref) {
		t.Fatalf("source remained current: %+v", mergedInto)
	}
	factRef := contract.Ref{ID: "memory/merge/" + candidate.Envelope.ID, Revision: 1}
	f.store.mu.RLock()
	factRef.Digest = f.store.tenants[f.access.TenantID].mergeFacts[factRef.ID].Ref.Digest
	f.store.mu.RUnlock()
	fact, err := f.store.InspectMerge(f.access, factRef)
	if err != nil || !contract.SameRef(fact.TargetRef, merged.Ref) {
		t.Fatalf("fact=%+v %v", fact, err)
	}
}
func TestMemoryMergeRejectsStaleOrDuplicateSources(t *testing.T) {
	f := newFixture(t)
	_, target := createMemoryRecord(t, f, "record-a", 1)
	candidate := f.candidate("bad-merge", CandidateCorrection, "merged", target.Ref, 2)
	candidate.Kind = CandidateMerge
	candidate.MergeSourceRefs = []contract.Ref{target.Ref, target.Ref}
	candidate = SealCandidate(candidate)
	if err := candidate.Validate(f.now); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("duplicate merge accepted: %v", err)
	}
}
