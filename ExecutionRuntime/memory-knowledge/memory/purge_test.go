package memory

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func purgeRequest(target contract.Ref) PurgeRequest {
	return PurgeRequest{ID: "memory-purge", TargetRef: target, ScopeRef: ref("scope"), OperationRef: ref("purge-operation"), RequestedByRef: ref("requester"), RetentionDecisionRef: ref("retention-clear"), LegalHoldInspectionRef: ref("legal-hold-clear"), TTL: time.Hour}
}

func TestMemoryPreparePurgeFreezesExactContentAndDoesNotDelete(t *testing.T) {
	f := newFixture(t)
	_, record := createMemoryRecord(t, f, "record", 1)
	projection := SealProjection(Projection{Ref: contract.Ref{ID: "projection", Revision: 1}, TenantID: f.access.TenantID, RecordRef: record.Ref, Kind: "lexical", BuilderVersion: "v1", State: ProjectionReady, Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1}, CreatedAt: f.now, ExpiresAt: f.now.Add(time.Hour)})
	if _, err := f.store.PutProjection(f.access, projection, contract.ExpectAbsent()); err != nil {
		t.Fatal(err)
	}
	tombstone := f.candidate("tombstone", CandidateTombstone, "", record.Ref, 2)
	admission := f.submitReady(tombstone, "tombstone-admission")
	removed, _, err := f.store.Tombstone(f.access, CommitRequest{TenantID: f.access.TenantID, AttemptID: "tombstone-attempt", ResultID: "tombstone-result", RecordID: record.Ref.ID, CandidateRef: tombstone.Ref(), AdmissionRef: admission.Ref, OperationRef: ref("tombstone-operation"), ExpectedRevision: contract.ExpectRevision(record.Ref.Revision)})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := f.store.PreparePurge(f.access, purgeRequest(removed.Ref))
	if err != nil || len(intent.ContentRefs) != 1 || intent.ContentRefs[0] != *record.ContentRef || len(intent.ProjectionRefs) != 1 || intent.ExecutionSupport != contract.PurgeExecutionBlockedV1 {
		t.Fatalf("intent=%+v err=%v", intent, err)
	}
	if !f.content.Has(*record.ContentRef) {
		t.Fatal("PreparePurge performed physical deletion")
	}
	inspected, err := f.store.InspectPurge(f.access, intent.Ref)
	if err != nil || !contract.SameRef(inspected.Ref, intent.Ref) {
		t.Fatalf("inspect=%+v err=%v", inspected, err)
	}
	tampered := intent
	tampered.ContentRefs[0].Digest = "sha256:tampered"
	if err := tampered.Validate(f.now); !errors.Is(err, contract.ErrInvalidArgument) && !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("tampered purge accepted: %v", err)
	}
}

func TestMemoryPreparePurgeRejectsCurrentAndLegalHold(t *testing.T) {
	f := newFixture(t)
	_, active := createMemoryRecord(t, f, "active", 1)
	if _, err := f.store.PreparePurge(f.access, purgeRequest(active.Ref)); !errors.Is(err, contract.ErrNotCurrent) {
		t.Fatalf("active record purge prepared: %v", err)
	}
	candidate := f.candidate("held", CandidateCreate, "held", contract.Ref{}, 2)
	candidate.LegalHoldRef = ref("legal-hold")
	candidate = SealCandidate(candidate)
	admission := f.submitReady(candidate, "held-admission")
	held, _, err := f.commit(candidate, admission, "held-attempt", "held-result", "held", contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	tombstone := f.candidate("held-tombstone", CandidateTombstone, "", held.Ref, 3)
	tombstone.LegalHoldRef = held.LegalHoldRef
	tombstone = SealCandidate(tombstone)
	admission = f.submitReady(tombstone, "held-tombstone-admission")
	removed, _, err := f.store.Tombstone(f.access, CommitRequest{TenantID: f.access.TenantID, AttemptID: "held-tombstone-attempt", ResultID: "held-tombstone-result", RecordID: held.Ref.ID, CandidateRef: tombstone.Ref(), AdmissionRef: admission.Ref, OperationRef: ref("held-tombstone-operation"), ExpectedRevision: contract.ExpectRevision(held.Ref.Revision)})
	if err != nil {
		t.Fatal(err)
	}
	request := purgeRequest(removed.Ref)
	request.ID = "held-purge"
	if _, err := f.store.PreparePurge(f.access, request); !errors.Is(err, contract.ErrScopeDenied) {
		t.Fatalf("legal hold purge prepared: %v", err)
	}
}
