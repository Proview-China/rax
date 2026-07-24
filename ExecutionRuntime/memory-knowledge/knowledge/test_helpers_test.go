package knowledge

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/reference"
)

type fixture struct {
	store      *Store
	content    *reference.Store
	now        *time.Time
	access     Access
	assetRef   contract.Ref
	source     Source
	pkg        Package
	candidate  Candidate
	admission  Admission
	attempt    CommitAttempt
	result     contract.DomainResultFact
	record     Record
	contentRef contract.ContentRef
}

func ref(id string) contract.Ref {
	return contract.Ref{ID: id, Revision: 1, Digest: "sha256:" + id}
}

func newFixture(t *testing.T, commit bool) *fixture {
	t.Helper()
	now := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	clock := contract.ClockFunc(func() time.Time { return now })
	f := &fixture{
		store: NewStore(clock), content: reference.NewStore(), now: &now,
		access:   Access{TenantID: "tenant-a", AuthorityRef: ref("authority-a"), PolicyRef: ref("policy-a")},
		assetRef: ref("asset-a"),
	}
	var err error
	f.contentRef, err = f.content.Put([]byte("deterministic knowledge alpha beta"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	f.source, err = f.store.RegisterSource(f.access, SourceInput{
		TenantID: f.access.TenantID, ID: "source-a", Version: "v1", AssetRef: f.assetRef,
		ContentDigest: f.contentRef.Digest, AuthorityRef: f.access.AuthorityRef, PolicyRef: f.access.PolicyRef,
		License: "internal-use", Scope: "project-a", Sensitivity: "internal", State: SourceAvailable,
		Provenance: []contract.Ref{ref("provenance-a")}, AcquiredAt: now, ValidFrom: now.Add(-time.Hour), ValidTo: now.Add(24 * time.Hour),
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	f.pkg, err = f.store.PutPackage(f.access, PackageInput{
		TenantID: f.access.TenantID, ID: "package-a", Version: "v1", SourceRefs: []contract.Ref{f.source.Ref},
		AuthorityRef: f.access.AuthorityRef, PolicyRef: f.access.PolicyRef, License: "internal-use",
		Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1}, State: PackageReady,
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	f.candidate, err = f.store.SubmitCandidate(f.access, CandidateInput{
		TenantID: f.access.TenantID, ID: "candidate-a", ProducerID: "producer-a", SourceEpoch: 1, SourceSequence: 1,
		Kind: CandidateRecord, PayloadDigest: "sha256:payload-a", EvidenceRefs: []contract.Ref{ref("evidence-a")}, TTL: time.Hour,
		Draft: RecordDraft{ID: "record-a", PackageRef: f.pkg.Ref, ContentRef: f.contentRef, SourceRefs: []contract.Ref{f.source.Ref},
			EvidenceRefs: []contract.Ref{ref("record-evidence-a")}, Scope: "project-a", Subject: "alpha",
			Sensitivity: "internal", License: "internal-use", TrustState: "source_supported",
			ValidFrom: now.Add(-time.Hour), ValidTo: now.Add(12 * time.Hour)},
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	f.admission, err = f.store.AdmitCandidate(f.access, f.candidate.Ref, AdmissionCommitReady, "validated", time.Hour, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	f.attempt, err = f.store.BeginCommit(f.access, CommitRequest{
		TenantID: f.access.TenantID, AttemptID: "attempt-a", OperationRef: ref("operation-a"),
		CandidateRef: f.candidate.Ref, AdmissionRef: f.admission.Ref, ExpectedRecord: contract.ExpectAbsent(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if commit {
		f.result, err = f.store.CommitAttempt(f.access, f.attempt.Ref.ID)
		if err != nil {
			t.Fatal(err)
		}
		f.record, err = f.store.GetRecord(f.access, f.result.SubjectRef)
		if err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func correctionCandidate(t *testing.T, f *fixture, id string, sequence uint64, body string) (Candidate, Admission, CommitAttempt) {
	t.Helper()
	contentRef, err := f.content.Put([]byte(body), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := f.store.SubmitCandidate(f.access, CandidateInput{
		TenantID: f.access.TenantID, ID: id, ProducerID: "producer-a", SourceEpoch: 1, SourceSequence: sequence,
		Kind: CandidateCorrection, TargetRef: f.record.Ref, PayloadDigest: "sha256:" + id, TTL: time.Hour,
		Draft: RecordDraft{ID: f.record.Ref.ID, PackageRef: f.pkg.Ref, ContentRef: contentRef, SourceRefs: []contract.Ref{f.source.Ref},
			EvidenceRefs: []contract.Ref{ref("evidence-" + id)}, Scope: f.record.Scope, Subject: f.record.Subject,
			Sensitivity: f.record.Sensitivity, License: f.record.License, TrustState: f.record.TrustState,
			ValidFrom: *f.now, ValidTo: f.now.Add(12 * time.Hour)},
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	admission, err := f.store.AdmitCandidate(f.access, candidate.Ref, AdmissionCommitReady, "validated", time.Hour, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := f.store.BeginCommit(f.access, CommitRequest{
		TenantID: f.access.TenantID, AttemptID: "attempt-" + id, OperationRef: ref("operation-" + id),
		CandidateRef: candidate.Ref, AdmissionRef: admission.Ref, ExpectedRecord: contract.ExpectRevision(f.record.Ref.Revision),
	})
	if err != nil {
		t.Fatal(err)
	}
	return candidate, admission, attempt
}
