package memory

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/reference"
)

type fixture struct {
	t       *testing.T
	now     time.Time
	clock   contract.ClockFunc
	content *reference.Store
	store   *Store
	access  Access
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	f := &fixture{t: t, now: now, content: reference.NewStore()}
	f.clock = func() time.Time { return f.now }
	f.store = NewStore(f.clock, f.content)
	f.access = Access{TenantID: "tenant-a", IdentityID: "identity-a", AuthorityRef: ref("authority-a"), AuthorityEpoch: 7, PolicyRef: ref("policy-a")}
	return f
}

func ref(id string) contract.Ref {
	return contract.Ref{ID: id, Revision: 1, Digest: "sha256:" + id}
}

func (f *fixture) candidate(id string, kind CandidateKind, text string, target contract.Ref, sequence uint64) Candidate {
	f.t.Helper()
	var content *contract.ContentRef
	if kind != CandidateTombstone {
		got, err := f.content.Put([]byte(text), "text/plain")
		if err != nil {
			f.t.Fatal(err)
		}
		content = &got
	}
	c := Candidate{
		Envelope: contract.Envelope{ContractVersion: contract.VersionV1, SchemaRef: "praxis.memory/candidate-v1", ID: id, Revision: 1, TenantID: f.access.TenantID, IdentityID: f.access.IdentityID, IdentityEpoch: 3, AuthorityRef: f.access.AuthorityRef, AuthorityEpoch: f.access.AuthorityEpoch, PolicyRef: f.access.PolicyRef, Purpose: "assist", ActionScopeDigest: "sha256:scope", CreatedAt: f.now, UpdatedAt: f.now, ExpiresAt: f.now.Add(time.Hour), CorrelationID: "correlation"},
		Kind:     kind, ProducerRef: ref("producer"), SourceEpoch: 9, SourceSequence: sequence, Scope: "identity_private", Subject: "preference", ContentRef: content, SourceRefs: []contract.Ref{ref("source")}, EvidenceRefs: []contract.Ref{ref("evidence")}, Sensitivity: "internal", TargetRecordRef: target,
	}
	return SealCandidate(c)
}

func (f *fixture) submitReady(c Candidate, admissionID string) AdmissionFact {
	f.t.Helper()
	if _, err := f.store.SubmitCandidate(f.access, c); err != nil {
		f.t.Fatal(err)
	}
	a, err := f.store.Admit(f.access, AdmissionRequest{ID: admissionID, CandidateRef: c.Ref(), Decision: AdmissionCommitReady, ExpiresAt: f.now.Add(time.Hour), ExpectedRevision: contract.ExpectAbsent()})
	if err != nil {
		f.t.Fatal(err)
	}
	return a
}

func (f *fixture) commit(c Candidate, a AdmissionFact, attempt, result, record string, expected contract.ExpectedRevision) (Record, contract.DomainResultFact, error) {
	return f.store.Commit(f.access, CommitRequest{TenantID: f.access.TenantID, AttemptID: attempt, ResultID: result, RecordID: record, CandidateRef: c.Ref(), AdmissionRef: a.Ref, OperationRef: ref("operation-" + attempt), ExpectedRevision: expected})
}

func TestStateMachineAdmissionFourStates(t *testing.T) {
	decisions := []AdmissionDecision{AdmissionRejected, AdmissionMerged, AdmissionReviewRequired, AdmissionCommitReady}
	for i, decision := range decisions {
		t.Run(string(decision), func(t *testing.T) {
			f := newFixture(t)
			c := f.candidate("candidate", CandidateCreate, "remember blue", contract.Ref{}, uint64(i+1))
			if _, err := f.store.SubmitCandidate(f.access, c); err != nil {
				t.Fatal(err)
			}
			req := AdmissionRequest{ID: "admission", CandidateRef: c.Ref(), Decision: decision, ExpiresAt: f.now.Add(time.Minute), ExpectedRevision: contract.ExpectAbsent()}
			if decision == AdmissionMerged {
				req.MergeTarget = ref("merge-target")
			}
			fact, err := f.store.Admit(f.access, req)
			if err != nil {
				t.Fatal(err)
			}
			if fact.Decision != decision || fact.Owner != contract.OwnerMemory {
				t.Fatalf("unexpected fact: %#v", fact)
			}
			_, _, err = f.commit(c, fact, "attempt", "result", "record", contract.ExpectAbsent())
			if decision == AdmissionCommitReady && err != nil {
				t.Fatalf("commit_ready rejected: %v", err)
			}
			if decision != AdmissionCommitReady && !errors.Is(err, contract.ErrCandidateRejected) {
				t.Fatalf("decision %s committed or wrong error: %v", decision, err)
			}
		})
	}
}

func TestStateMachineAdmissionReviewTransitionCAS(t *testing.T) {
	f := newFixture(t)
	c := f.candidate("candidate", CandidateCreate, "review me", contract.Ref{}, 1)
	if _, err := f.store.SubmitCandidate(f.access, c); err != nil {
		t.Fatal(err)
	}
	first, err := f.store.Admit(f.access, AdmissionRequest{ID: "admission", CandidateRef: c.Ref(), Decision: AdmissionReviewRequired, ExpiresAt: f.now.Add(time.Hour), ExpectedRevision: contract.ExpectAbsent()})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := f.store.Admit(f.access, AdmissionRequest{ID: "admission", CandidateRef: c.Ref(), Decision: AdmissionCommitReady, ExpiresAt: f.now.Add(time.Hour), ExpectedRevision: contract.ExpectRevision(first.Ref.Revision)})
	if err != nil || ready.Ref.Revision != 2 {
		t.Fatalf("review transition failed: %#v %v", ready, err)
	}
	if _, err := f.store.Admit(f.access, AdmissionRequest{ID: "admission", CandidateRef: c.Ref(), Decision: AdmissionRejected, ExpiresAt: f.now.Add(time.Hour), ExpectedRevision: contract.ExpectRevision(2)}); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatalf("terminal admission transitioned: %v", err)
	}
	if _, _, err := f.commit(c, first, "old-attempt", "old-result", "record", contract.ExpectAbsent()); !errors.Is(err, contract.ErrNotCurrent) {
		t.Fatalf("stale admission revision committed: %v", err)
	}
}

func TestCASCandidateExactIdempotencyAndSourceSequenceConflict(t *testing.T) {
	f := newFixture(t)
	c := f.candidate("candidate", CandidateCreate, "alpha", contract.Ref{}, 1)
	first, err := f.store.SubmitCandidate(f.access, c)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.store.SubmitCandidate(f.access, c)
	if err != nil || !contract.SameRef(first.Ref(), second.Ref()) {
		t.Fatalf("exact replay must be idempotent: %#v %v", second, err)
	}
	changed := f.candidate("different-id", CandidateCreate, "beta", contract.Ref{}, 1)
	if _, err := f.store.SubmitCandidate(f.access, changed); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("same producer epoch sequence accepted different evidence: %v", err)
	}
	otherTenant := f.access
	otherTenant.TenantID = "tenant-b"
	changed.Envelope.TenantID = otherTenant.TenantID
	changed = SealCandidate(changed)
	if _, err := f.store.SubmitCandidate(otherTenant, changed); err != nil {
		t.Fatalf("source sequence key must be tenant isolated: %v", err)
	}
}

func TestCASRecordCorrectionAndImmutableHistory(t *testing.T) {
	f := newFixture(t)
	create := f.candidate("create", CandidateCreate, "old text", contract.Ref{}, 1)
	a1 := f.submitReady(create, "admission-create")
	r1, result1, err := f.commit(create, a1, "attempt-create", "result-create", "record", contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	if result1.State != contract.DomainResultReady || result1.CASBefore != 0 || result1.CASAfter != 1 {
		t.Fatalf("bad result fact: %#v", result1)
	}
	correction := f.candidate("correction", CandidateCorrection, "new text", r1.Ref, 2)
	a2 := f.submitReady(correction, "admission-correction")
	r2, result2, err := f.store.Correct(f.access, CommitRequest{TenantID: f.access.TenantID, AttemptID: "attempt-correction", ResultID: "result-correction", RecordID: "record", CandidateRef: correction.Ref(), AdmissionRef: a2.Ref, OperationRef: ref("operation-correction"), ExpectedRevision: contract.ExpectRevision(1)})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Ref.Revision != 2 || !contract.SameRef(r2.Corrects, r1.Ref) || result2.CASAfter != 2 {
		t.Fatalf("bad correction: %#v %#v", r2, result2)
	}
	historical, err := f.store.InspectRecord(f.access, r1.Ref)
	if err != nil || historical.Status != RecordActive || historical.ContentRef == nil {
		t.Fatalf("historical revision was overwritten: %#v %v", historical, err)
	}
	_, _, err = f.commit(correction, a2, "attempt-loser", "result-loser", "record", contract.ExpectRevision(1))
	if !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatalf("stale CAS won: %v", err)
	}
}

func TestCASConcurrentCorrectionSingleWinner(t *testing.T) {
	f := newFixture(t)
	create := f.candidate("create", CandidateCreate, "base", contract.Ref{}, 1)
	a0 := f.submitReady(create, "admission-create")
	r1, _, err := f.commit(create, a0, "attempt-create", "result-create", "record", contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	c1 := f.candidate("correction-1", CandidateCorrection, "one", r1.Ref, 2)
	c2 := f.candidate("correction-2", CandidateCorrection, "two", r1.Ref, 3)
	a1 := f.submitReady(c1, "admission-1")
	a2 := f.submitReady(c2, "admission-2")
	requests := []struct {
		c Candidate
		a AdmissionFact
		i string
	}{{c1, a1, "1"}, {c2, a2, "2"}}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, item := range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := f.store.Correct(f.access, CommitRequest{TenantID: f.access.TenantID, AttemptID: "attempt-" + item.i, ResultID: "result-" + item.i, RecordID: "record", CandidateRef: item.c.Ref(), AdmissionRef: item.a.Ref, OperationRef: ref("operation-" + item.i), ExpectedRevision: contract.ExpectRevision(1)})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	winners, conflicts := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, contract.ErrRevisionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("CAS winners=%d conflicts=%d", winners, conflicts)
	}
}

func TestFaultUnknownOnlyInspectOriginalAttempt(t *testing.T) {
	f := newFixture(t)
	c := f.candidate("candidate", CandidateCreate, "lost reply", contract.Ref{}, 1)
	a := f.submitReady(c, "admission")
	f.store.afterCAS = func(CommitInspection) error { return errors.New("lost response") }
	_, _, err := f.commit(c, a, "attempt", "result", "record", contract.ExpectAbsent())
	if !errors.Is(err, contract.ErrUnknownOutcome) {
		t.Fatalf("want unknown outcome, got %v", err)
	}
	inspection, result, err := f.store.InspectCommit(f.access, "attempt")
	if err != nil || inspection.State != InspectionApplied || result.AttemptID != "attempt" || !contract.SameRef(result.InspectionRef, inspection.Ref) {
		t.Fatalf("inspect did not recover original attempt: %#v %#v %v", inspection, result, err)
	}
	f.store.afterCAS = nil
	record, replay, err := f.commit(c, a, "attempt", "result", "record", contract.ExpectAbsent())
	if err != nil || record.Ref.Revision != 1 || !contract.SameRef(replay.Ref, result.Ref) {
		t.Fatalf("exact attempt replay created a new commit: %#v %#v %v", record, replay, err)
	}
}

func TestCASApplySettlementOpaqueAndUnique(t *testing.T) {
	f := newFixture(t)
	c := f.candidate("candidate", CandidateCreate, "settle", contract.Ref{}, 1)
	a := f.submitReady(c, "admission")
	_, result, err := f.commit(c, a, "attempt", "result", "record", contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	settlement := contract.RuntimeSettlementRef{Ref: ref("runtime-settlement")}
	req := SettlementRequest{TenantID: f.access.TenantID, Association: contract.DomainResultAssociation{DomainResultRef: result.Ref}, Settlement: settlement, ExpectedRevision: contract.ExpectAbsent()}
	first, err := f.store.ApplySettlement(f.access, req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.store.ApplySettlement(f.access, req)
	if err != nil || !contract.SameRef(first.Ref, second.Ref) {
		t.Fatalf("exact settlement replay not idempotent: %#v %v", second, err)
	}
	req.Settlement = contract.RuntimeSettlementRef{Ref: ref("different-settlement")}
	if _, err := f.store.ApplySettlement(f.access, req); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatalf("replacement settlement accepted: %v", err)
	}
	if result.State != contract.DomainResultReady {
		t.Fatalf("domain result mutated by settlement: %s", result.State)
	}
	c2 := f.candidate("candidate-2", CandidateCreate, "second", contract.Ref{}, 2)
	a2 := f.submitReady(c2, "admission-2")
	_, result2, err := f.commit(c2, a2, "attempt-2", "result-2", "record-2", contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ApplySettlement(f.access, SettlementRequest{TenantID: f.access.TenantID, Association: contract.DomainResultAssociation{DomainResultRef: result2.Ref}, Settlement: settlement, ExpectedRevision: contract.ExpectAbsent()}); !errors.Is(err, contract.ErrSettlementMismatch) {
		t.Fatalf("settlement ref reused across results: %v", err)
	}
}

func TestCASApplySettlementRejectsWrongAssociationDigest(t *testing.T) {
	f := newFixture(t)
	c := f.candidate("candidate", CandidateCreate, "association", contract.Ref{}, 1)
	a := f.submitReady(c, "admission")
	_, result, err := f.commit(c, a, "attempt", "result", "record", contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	wrong := result.Ref
	wrong.Digest = "sha256:wrong-association"
	_, err = f.store.ApplySettlement(f.access, SettlementRequest{
		TenantID: f.access.TenantID, Association: contract.DomainResultAssociation{DomainResultRef: wrong},
		Settlement: contract.RuntimeSettlementRef{Ref: ref("runtime-settlement")}, ExpectedRevision: contract.ExpectAbsent(),
	})
	if !errors.Is(err, contract.ErrSettlementMismatch) {
		t.Fatalf("wrong association digest accepted: %v", err)
	}
}

func TestStateMachineTombstoneHasNoBody(t *testing.T) {
	f := newFixture(t)
	create := f.candidate("create", CandidateCreate, "secret text", contract.Ref{}, 1)
	a1 := f.submitReady(create, "admission-create")
	r1, _, err := f.commit(create, a1, "attempt-create", "result-create", "record", contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	tombstone := f.candidate("forget", CandidateTombstone, "", r1.Ref, 2)
	a2 := f.submitReady(tombstone, "admission-forget")
	r2, _, err := f.store.Tombstone(f.access, CommitRequest{TenantID: f.access.TenantID, AttemptID: "attempt-forget", ResultID: "result-forget", RecordID: "record", CandidateRef: tombstone.Ref(), AdmissionRef: a2.Ref, OperationRef: ref("operation-forget"), ExpectedRevision: contract.ExpectRevision(1)})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Status != RecordTombstoned || r2.ContentRef != nil {
		t.Fatalf("tombstone retained body: %#v", r2)
	}
}

func TestStateMachineTTLBoundary(t *testing.T) {
	f := newFixture(t)
	c := f.candidate("candidate", CandidateCreate, "expires", contract.Ref{}, 1)
	c.Envelope.ExpiresAt = f.now
	c = SealCandidate(c)
	if _, err := f.store.SubmitCandidate(f.access, c); !errors.Is(err, contract.ErrNotCurrent) {
		t.Fatalf("now == expires must be expired: %v", err)
	}
}

func TestConformanceMemoryOwnerDeepCopies(t *testing.T) {
	f := newFixture(t)
	c := f.candidate("candidate", CandidateCreate, "immutable", contract.Ref{}, 1)
	stored, err := f.store.SubmitCandidate(f.access, c)
	if err != nil {
		t.Fatal(err)
	}
	stored.SourceRefs[0] = ref("mutated")
	replayed, err := f.store.SubmitCandidate(f.access, c)
	if err != nil {
		t.Fatal(err)
	}
	if !contract.SameRef(replayed.SourceRefs[0], ref("source")) || replayed.Envelope.Digest != c.Envelope.Digest {
		t.Fatalf("caller mutated owner state: %#v", replayed)
	}
	if replayed.Ref().Digest == "" || contract.OwnerMemory == contract.OwnerKnowledge {
		t.Fatal("owner/ref conformance failure")
	}
}
