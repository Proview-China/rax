package knowledge

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func knowledgeJob(f *fixture, id string) contract.OwnerJobAttemptV1 {
	return contract.OwnerJobAttemptV1{
		Ref: contract.Ref{ID: id, Revision: 1}, Owner: contract.OwnerKnowledge, Kind: contract.JobKnowledgeSync,
		TenantID: f.access.TenantID, AuthorityRef: f.access.AuthorityRef, PolicyRef: f.access.PolicyRef, ScopeRef: ref("scope-a"),
		OperationRef: ref("operation-job"), AttemptRef: ref("attempt-job"), SubjectRef: f.source.Ref, InputDigest: "sha256:input",
		State: contract.JobReserved, CreatedAt: *f.now, UpdatedAt: *f.now, ExpiresAt: f.now.Add(time.Hour),
	}
}

func TestKnowledgeOwnerJobCASUnknownInspectAndIsolation(t *testing.T) {
	f := newFixture(t, false)
	reserved, err := f.store.ReserveJob(f.access, knowledgeJob(f, "job-a"))
	if err != nil {
		t.Fatal(err)
	}
	begun := reserved
	begun.Ref = contract.Ref{ID: begun.Ref.ID, Revision: 2}
	begun.State = contract.JobBegun
	begun.UpdatedAt = f.now.Add(time.Second)
	begun.Digest = ""
	begun, err = f.store.AdvanceJob(f.access, reserved.Ref, begun)
	if err != nil {
		t.Fatal(err)
	}
	unknown := begun
	unknown.Ref = contract.Ref{ID: unknown.Ref.ID, Revision: 3}
	unknown.State = contract.JobUnknownOutcome
	unknown.UpdatedAt = f.now.Add(2 * time.Second)
	unknown.Digest = ""
	unknown, err = f.store.AdvanceJob(f.access, begun.Ref, unknown)
	if err != nil {
		t.Fatal(err)
	}
	restart := unknown
	restart.Ref = contract.Ref{ID: restart.Ref.ID, Revision: 4}
	restart.State = contract.JobBegun
	restart.UpdatedAt = f.now.Add(3 * time.Second)
	restart.Digest = ""
	if _, err := f.store.AdvanceJob(f.access, unknown.Ref, restart); !errors.Is(err, contract.ErrUnknownOutcome) {
		t.Fatalf("unknown outcome restarted: %v", err)
	}
	got, err := f.store.InspectJob(f.access, unknown.Ref)
	if err != nil || !contract.SameRef(got.Ref, unknown.Ref) {
		t.Fatalf("inspect=%+v err=%v", got, err)
	}
	got.Residuals = append(got.Residuals, "mutated")
	again, err := f.store.InspectJob(f.access, unknown.Ref)
	if err != nil || len(again.Residuals) != 0 {
		t.Fatalf("stored job aliased: %+v %v", again, err)
	}
	wrong := unknown.Ref
	wrong.Digest = "sha256:wrong"
	if _, err := f.store.InspectJob(f.access, wrong); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("drift not rejected: %v", err)
	}
}

func TestKnowledgeOwnerJobConcurrentCAS(t *testing.T) {
	f := newFixture(t, false)
	reserved, err := f.store.ReserveJob(f.access, knowledgeJob(f, "job-concurrent"))
	if err != nil {
		t.Fatal(err)
	}
	next := reserved
	next.Ref = contract.Ref{ID: next.Ref.ID, Revision: 2}
	next.State = contract.JobBegun
	next.UpdatedAt = f.now.Add(time.Second)
	next.Digest = ""
	errs := make(chan error, 64)
	for range 64 {
		go func() { _, err := f.store.AdvanceJob(f.access, reserved.Ref, next); errs <- err }()
	}
	for range 64 {
		if err := <-errs; err != nil {
			t.Fatalf("idempotent CAS: %v", err)
		}
	}
}
