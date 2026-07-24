package knowledge

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestKnowledgeFaultLostReplyInspectsOriginalAttempt(t *testing.T) {
	f := newFixture(t, false)

	before, result, err := f.store.InspectCommit(f.access, f.attempt.Ref.ID, f.attempt.OperationRef)
	if err != nil || result != nil || before.Outcome != InspectionNotApplied {
		t.Fatalf("begin inspection: %+v %+v %v", before, result, err)
	}

	// Simulate the caller losing the successful CommitAttempt response.
	if _, err := f.store.CommitAttempt(f.access, f.attempt.Ref.ID); err != nil {
		t.Fatal(err)
	}
	after, inspected, err := f.store.InspectCommit(f.access, f.attempt.Ref.ID, f.attempt.OperationRef)
	if err != nil || inspected == nil || after.Outcome != InspectionApplied || !contract.SameRef(after.Ref, inspected.InspectionRef) {
		t.Fatalf("lost reply recovery: inspection=%+v result=%+v err=%v", after, inspected, err)
	}
	retry, err := f.store.CommitAttempt(f.access, f.attempt.Ref.ID)
	if err != nil || !contract.SameRef(retry.Ref, inspected.Ref) || retry.CASAfter != 1 {
		t.Fatalf("original attempt retry reapplied: result=%+v err=%v", retry, err)
	}
	if _, _, err := f.store.InspectCommit(f.access, f.attempt.Ref.ID, ref("another-operation")); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("inspection accepted another operation: %v", err)
	}
}

func TestKnowledgeFaultExpiryAndInvalidCASFailClosed(t *testing.T) {
	f := newFixture(t, true)
	contentRef, err := f.content.Put([]byte("expires exactly"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := f.store.SubmitCandidate(f.access, CandidateInput{
		TenantID: f.access.TenantID, ID: "candidate-expiring", ProducerID: "producer-expiring", SourceEpoch: 1, SourceSequence: 1,
		Kind: CandidateCorrection, TargetRef: f.record.Ref, PayloadDigest: "sha256:expiring", TTL: time.Minute,
		Draft: RecordDraft{ID: f.record.Ref.ID, PackageRef: f.pkg.Ref, ContentRef: contentRef, SourceRefs: []contract.Ref{f.source.Ref},
			Scope: f.record.Scope, Subject: f.record.Subject, Sensitivity: f.record.Sensitivity, License: f.record.License,
			TrustState: f.record.TrustState, ValidFrom: *f.now, ValidTo: f.now.Add(time.Hour)},
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	*f.now = candidate.ExpiresAt
	if _, err := f.store.AdmitCandidate(f.access, candidate.Ref, AdmissionCommitReady, "too late", time.Hour, contract.ExpectAbsent()); !errors.Is(err, contract.ErrNotCurrent) {
		t.Fatalf("candidate accepted at exact expiry: %v", err)
	}

	if _, err := f.store.RegisterSource(f.access, SourceInput{}, contract.ExpectedRevision{}); !errors.Is(err, contract.ErrInvalidArgument) {
		t.Fatalf("ambiguous expected revision accepted: %v", err)
	}
}
