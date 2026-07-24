package knowledge

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestKnowledgeStateMachineAdmissionAndSettlement(t *testing.T) {
	f := newFixture(t, false)

	// Candidate source keys are tenant-local exact-idempotency keys.
	retry, err := f.store.SubmitCandidate(f.access, CandidateInput{
		TenantID: f.access.TenantID, ID: f.candidate.Ref.ID, ProducerID: f.candidate.ProducerID,
		SourceEpoch: f.candidate.SourceEpoch, SourceSequence: f.candidate.SourceSequence,
		Kind: f.candidate.Kind, Draft: f.candidate.Draft, PayloadDigest: f.candidate.PayloadDigest,
		EvidenceRefs: f.candidate.EvidenceRefs, TTL: time.Hour,
	}, contract.ExpectAbsent())
	if err != nil || !contract.SameRef(retry.Ref, f.candidate.Ref) {
		t.Fatalf("exact candidate retry: ref=%+v err=%v", retry.Ref, err)
	}
	_, err = f.store.SubmitCandidate(f.access, CandidateInput{
		TenantID: f.access.TenantID, ID: "candidate-conflict", ProducerID: f.candidate.ProducerID,
		SourceEpoch: f.candidate.SourceEpoch, SourceSequence: f.candidate.SourceSequence,
		Kind: f.candidate.Kind, Draft: f.candidate.Draft, PayloadDigest: "sha256:different", TTL: time.Hour,
	}, contract.ExpectAbsent())
	if !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("same source sequence with new payload: %v", err)
	}

	inspection, result, err := f.store.InspectCommit(f.access, f.attempt.Ref.ID, f.attempt.OperationRef)
	if err != nil || inspection.Outcome != InspectionNotApplied || result != nil {
		t.Fatalf("pre-apply inspect: inspection=%+v result=%+v err=%v", inspection, result, err)
	}
	domainResult, err := f.store.CommitAttempt(f.access, f.attempt.Ref.ID)
	if err != nil {
		t.Fatal(err)
	}
	if domainResult.Owner != contract.OwnerKnowledge || domainResult.State != contract.DomainResultReady {
		t.Fatalf("unexpected domain result: %+v", domainResult)
	}
	inspection, inspectedResult, err := f.store.InspectCommit(f.access, f.attempt.Ref.ID, f.attempt.OperationRef)
	if err != nil || inspectedResult == nil || !contract.SameRef(inspection.Ref, domainResult.InspectionRef) || !contract.SameRef(inspectedResult.Ref, domainResult.Ref) {
		t.Fatalf("post-apply inspect mismatch: inspection=%+v result=%+v err=%v", inspection, inspectedResult, err)
	}

	settlement := contract.RuntimeSettlementRef{Ref: ref("runtime-settlement-a")}
	association := contract.DomainResultAssociation{DomainResultRef: domainResult.Ref}
	application, err := f.store.ApplySettlement(f.access, association, settlement, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	if application.Owner != contract.OwnerKnowledge || application.State != contract.DomainResultSettled || domainResult.State != contract.DomainResultReady {
		t.Fatalf("settlement layering violated: result=%+v application=%+v", domainResult, application)
	}
	retryApplication, err := f.store.ApplySettlement(f.access, association, settlement, contract.ExpectAbsent())
	if err != nil || !contract.SameRef(application.Ref, retryApplication.Ref) {
		t.Fatalf("settlement exact retry: %+v %v", retryApplication, err)
	}
}

func TestKnowledgeStateMachineAdmissionVersionChain(t *testing.T) {
	f := newFixture(t, true)
	candidate, _, _ := correctionCandidate(t, f, "candidate-review", 2, "reviewed alpha correction")

	// correctionCandidate already created a terminal admission; use a second
	// candidate to exercise review_required -> commit_ready.
	contentRef, err := f.content.Put([]byte("versioned admission"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	candidate, err = f.store.SubmitCandidate(f.access, CandidateInput{
		TenantID: f.access.TenantID, ID: "candidate-review-chain", ProducerID: "reviewer", SourceEpoch: 1, SourceSequence: 1,
		Kind: CandidateCorrection, TargetRef: f.record.Ref, PayloadDigest: "sha256:review-chain", TTL: time.Hour,
		Draft: RecordDraft{ID: f.record.Ref.ID, PackageRef: f.pkg.Ref, ContentRef: contentRef, SourceRefs: []contract.Ref{f.source.Ref},
			Scope: f.record.Scope, Subject: f.record.Subject, Sensitivity: f.record.Sensitivity, License: f.record.License,
			TrustState: f.record.TrustState, ValidFrom: *f.now, ValidTo: f.now.Add(time.Hour)},
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	review, err := f.store.AdmitCandidate(f.access, candidate.Ref, AdmissionReviewRequired, "needs human review", time.Hour, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	ready, err := f.store.AdmitCandidate(f.access, candidate.Ref, AdmissionCommitReady, "verdict bound", time.Hour, contract.ExpectRevision(review.Ref.Revision))
	if err != nil || ready.Ref.Revision != review.Ref.Revision+1 {
		t.Fatalf("admission transition: review=%+v ready=%+v err=%v", review, ready, err)
	}
	_, err = f.store.AdmitCandidate(f.access, candidate.Ref, AdmissionRejected, "late reversal", time.Hour, contract.ExpectRevision(ready.Ref.Revision))
	if !errors.Is(err, contract.ErrCandidateRejected) {
		t.Fatalf("terminal admission transitioned: %v", err)
	}
}

func TestKnowledgeConformanceOwnerAndAccessIsolation(t *testing.T) {
	f := newFixture(t, true)
	wrong := f.access
	wrong.AuthorityRef = ref("authority-other")
	if _, err := f.store.GetRecord(wrong, f.record.Ref); !errors.Is(err, contract.ErrScopeDenied) {
		t.Fatalf("authority isolation: %v", err)
	}
	otherTenant := f.access
	otherTenant.TenantID = "tenant-b"
	if _, err := f.store.GetRecord(otherTenant, f.record.Ref); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("tenant isolation: %v", err)
	}
	if f.record.Owner != contract.OwnerKnowledge || f.result.Owner != contract.OwnerKnowledge {
		t.Fatalf("owner isolation: record=%s result=%s", f.record.Owner, f.result.Owner)
	}

	got, err := f.store.GetSource(f.access, f.source.Ref)
	if err != nil {
		t.Fatal(err)
	}
	got.Provenance[0] = ref("mutated")
	again, err := f.store.GetSource(f.access, f.source.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if contract.SameRef(again.Provenance[0], got.Provenance[0]) {
		t.Fatal("store returned aliased source provenance")
	}
}

func TestKnowledgeRejectsOwnerLikeCandidateTrustState(t *testing.T) {
	f := newFixture(t, true)
	contentRef, err := f.content.Put([]byte("invalid trust assertion"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.store.SubmitCandidate(f.access, CandidateInput{
		TenantID: f.access.TenantID, ID: "candidate-invalid-trust", ProducerID: "producer-a",
		SourceEpoch: 1, SourceSequence: 2, Kind: CandidateCorrection, TargetRef: f.record.Ref,
		PayloadDigest: "sha256:invalid-trust", TTL: time.Hour,
		Draft: RecordDraft{
			ID: f.record.Ref.ID, PackageRef: f.pkg.Ref, ContentRef: contentRef,
			SourceRefs: []contract.Ref{f.source.Ref}, Scope: f.record.Scope, Subject: f.record.Subject,
			Sensitivity: f.record.Sensitivity, License: f.record.License, TrustState: TrustState("authoritative"),
			ValidFrom: *f.now, ValidTo: f.now.Add(time.Hour),
		},
	}, contract.ExpectAbsent())
	if !errors.Is(err, contract.ErrInvalidArgument) {
		t.Fatalf("owner-like candidate trust state accepted: %v", err)
	}
}
