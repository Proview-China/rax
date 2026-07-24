package memory

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func lifecycleCandidate(f *fixture, id string, kind CandidateKind, target Record, sequence uint64) Candidate {
	c := Candidate{
		Envelope: contract.Envelope{
			ContractVersion: contract.VersionV1, SchemaRef: "praxis.memory/candidate-v1", ID: id, Revision: 1,
			TenantID: f.access.TenantID, IdentityID: f.access.IdentityID, IdentityEpoch: 3,
			AuthorityRef: f.access.AuthorityRef, AuthorityEpoch: f.access.AuthorityEpoch, PolicyRef: f.access.PolicyRef,
			Purpose: target.Purpose, ActionScopeDigest: target.ActionScopeDigest, CreatedAt: f.now, UpdatedAt: f.now,
			ExpiresAt: f.now.Add(time.Hour), CorrelationID: "lifecycle",
		},
		Kind: kind, ProducerRef: ref("lifecycle-controller"), SourceEpoch: 11, SourceSequence: sequence,
		Scope: target.Scope, Subject: target.Subject, SourceRefs: []contract.Ref{target.Ref},
		EvidenceRefs: []contract.Ref{ref("lifecycle-evidence")}, Sensitivity: target.Sensitivity, TargetRecordRef: target.Ref,
	}
	if kind == CandidatePin {
		c.RetentionRef = ref("retention-policy")
	}
	return SealCandidate(c)
}

func commitLifecycle(t *testing.T, f *fixture, current Record, kind CandidateKind, sequence uint64) (Record, error) {
	t.Helper()
	candidate := lifecycleCandidate(f, "candidate-"+string(kind), kind, current, sequence)
	admission := f.submitReady(candidate, "admission-"+string(kind))
	request := CommitRequest{
		TenantID: f.access.TenantID, AttemptID: "attempt-" + string(kind), ResultID: "result-" + string(kind),
		RecordID: current.Ref.ID, CandidateRef: candidate.Ref(), AdmissionRef: admission.Ref,
		OperationRef: ref("operation-" + string(kind)), ExpectedRevision: contract.ExpectRevision(current.Ref.Revision),
	}
	var record Record
	var err error
	switch kind {
	case CandidatePin:
		record, _, err = f.store.Pin(f.access, request)
	case CandidateArchive:
		record, _, err = f.store.Archive(f.access, request)
	case CandidateForget:
		record, _, err = f.store.Forget(f.access, request)
	}
	return record, err
}

func TestMemoryLifecyclePinArchiveForgetPreservesHistory(t *testing.T) {
	f := newFixture(t)
	create := f.candidate("candidate-create", CandidateCreate, "remember lifecycle", contract.Ref{}, 1)
	admission := f.submitReady(create, "admission-create")
	active, _, err := f.commit(create, admission, "attempt-create", "result-create", "record-a", contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := commitLifecycle(t, f, active, CandidatePin, 2)
	if err != nil || !pinned.Pinned || pinned.ContentRef == nil || !contract.SameRef(pinned.RetentionRef, ref("retention-policy")) {
		t.Fatalf("pin=%+v err=%v", pinned, err)
	}
	archived, err := commitLifecycle(t, f, pinned, CandidateArchive, 3)
	if err != nil || archived.Status != RecordArchived || !archived.Pinned || archived.ContentRef == nil {
		t.Fatalf("archive=%+v err=%v", archived, err)
	}
	forgotten, err := commitLifecycle(t, f, archived, CandidateForget, 4)
	if err != nil || forgotten.Status != RecordTombstoned || forgotten.ContentRef != nil {
		t.Fatalf("forget=%+v err=%v", forgotten, err)
	}
	historical, err := f.store.InspectRecord(f.access, active.Ref)
	if err != nil || historical.Status != RecordActive || historical.ContentRef == nil {
		t.Fatalf("history lost: %+v %v", historical, err)
	}
}

func TestMemoryForgetFailsClosedOnLegalHoldAndMetadataDrift(t *testing.T) {
	f := newFixture(t)
	create := f.candidate("candidate-create", CandidateCreate, "held memory", contract.Ref{}, 1)
	create.LegalHoldRef = ref("legal-hold")
	create = SealCandidate(create)
	admission := f.submitReady(create, "admission-create")
	held, _, err := f.commit(create, admission, "attempt-create", "result-create", "record-held", contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	forget := lifecycleCandidate(f, "candidate-forget", CandidateForget, held, 2)
	ready := f.submitReady(forget, "admission-forget")
	request := CommitRequest{TenantID: f.access.TenantID, AttemptID: "attempt-forget", ResultID: "result-forget", RecordID: held.Ref.ID, CandidateRef: forget.Ref(), AdmissionRef: ready.Ref, OperationRef: ref("operation-forget"), ExpectedRevision: contract.ExpectRevision(held.Ref.Revision)}
	if _, _, err := f.store.Forget(f.access, request); !errors.Is(err, contract.ErrScopeDenied) {
		t.Fatalf("legal hold bypassed: %v", err)
	}
	drift := lifecycleCandidate(f, "candidate-drift", CandidateArchive, held, 3)
	drift.Subject = "different"
	drift = SealCandidate(drift)
	ready = f.submitReady(drift, "admission-drift")
	request = CommitRequest{TenantID: f.access.TenantID, AttemptID: "attempt-drift", ResultID: "result-drift", RecordID: held.Ref.ID, CandidateRef: drift.Ref(), AdmissionRef: ready.Ref, OperationRef: ref("operation-drift"), ExpectedRevision: contract.ExpectRevision(held.Ref.Revision)}
	if _, _, err := f.store.Archive(f.access, request); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("metadata drift accepted: %v", err)
	}
}
