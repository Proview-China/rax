package knowledge

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func syncAttempt(f *fixture, id string) SyncAttemptV1 {
	return SyncAttemptV1{Ref: contract.Ref{ID: id, Revision: 1}, TenantID: f.access.TenantID, JobAttemptRef: ref("job-attempt"), SourceSubjectRef: f.source.Ref, AuthorityRef: f.access.AuthorityRef, PolicyRef: f.access.PolicyRef, ScopeRef: ref("scope"), InputDigest: "sha256:input", State: SyncReserved, CreatedAt: *f.now, UpdatedAt: *f.now, ExpiresAt: f.now.Add(time.Hour)}
}
func nextSync(current SyncAttemptV1, state SyncState, now time.Time) SyncAttemptV1 {
	next := current
	next.Ref = contract.Ref{ID: current.Ref.ID, Revision: current.Ref.Revision + 1}
	next.State = state
	next.UpdatedAt = now
	next.Digest = ""
	switch state {
	case SyncAcquired:
		next.AcquireObservationRef = ref("acquire-observation")
	case SyncParsed:
		next.ParsedPackageRef = ref("parsed-package")
	case SyncNormalized:
		next.NormalizedPackageRef = ref("normalized-package")
	case SyncValidated:
		next.ValidationEvidenceRefs = []contract.Ref{ref("validation-evidence")}
	case SyncIndexed:
		next.RecordRefs = []contract.Ref{ref("record")}
		next.ProjectionRefs = []contract.Ref{ref("projection")}
	case SyncSnapshotReady:
		next.SnapshotRef = ref("snapshot")
	case SyncPublished:
		next.PointerRef = ref("pointer")
	}
	return next
}
func TestKnowledgeSyncJournalStagesCASAndNoEarlyPublish(t *testing.T) {
	f := newFixture(t, false)
	current, err := f.store.ReserveSync(f.access, syncAttempt(f, "sync"))
	if err != nil {
		t.Fatal(err)
	}
	stages := []SyncState{SyncAcquired, SyncParsed, SyncNormalized, SyncValidated, SyncIndexed, SyncSnapshotReady}
	for i, state := range stages {
		next := nextSync(current, state, f.now.Add(time.Duration(i+1)*time.Second))
		current, err = f.store.AdvanceSync(f.access, current.Ref, next)
		if err != nil {
			t.Fatalf("%s: %v", state, err)
		}
		f.store.mu.RLock()
		tenant := f.store.tenants[f.access.TenantID]
		if tenant.current != nil {
			t.Fatal("snapshot published before publish stage")
		}
		f.store.mu.RUnlock()
	}
	published := nextSync(current, SyncPublished, f.now.Add(10*time.Second))
	published, err = f.store.AdvanceSync(f.access, current.Ref, published)
	if err != nil || published.State != SyncPublished {
		t.Fatalf("publish journal: %+v %v", published, err)
	}
	historical, err := f.store.InspectSync(f.access, current.Ref)
	if err != nil || historical.State != SyncSnapshotReady {
		t.Fatalf("history=%+v %v", historical, err)
	}
}
func TestKnowledgeSyncUnknownOnlyInspectAndConcurrentCAS(t *testing.T) {
	f := newFixture(t, false)
	reserved, err := f.store.ReserveSync(f.access, syncAttempt(f, "sync-unknown"))
	if err != nil {
		t.Fatal(err)
	}
	unknown := nextSync(reserved, SyncUnknownOutcome, f.now.Add(time.Second))
	unknown, err = f.store.AdvanceSync(f.access, reserved.Ref, unknown)
	if err != nil {
		t.Fatal(err)
	}
	parsed := nextSync(unknown, SyncParsed, f.now.Add(2*time.Second))
	if _, err := f.store.AdvanceSync(f.access, unknown.Ref, parsed); !errors.Is(err, contract.ErrUnknownOutcome) {
		t.Fatalf("unknown skipped inspect: %v", err)
	}
	acquired := nextSync(unknown, SyncAcquired, f.now.Add(2*time.Second))
	errs := make(chan error, 64)
	for range 64 {
		go func() { _, err := f.store.AdvanceSync(f.access, unknown.Ref, acquired); errs <- err }()
	}
	for range 64 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent CAS: %v", err)
		}
	}
	wrong := acquired.Ref
	wrong.Digest = "sha256:wrong"
	if _, err := f.store.InspectSync(f.access, wrong); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("drift accepted: %v", err)
	}
}
func TestKnowledgeSyncCanonicalTamper(t *testing.T) {
	f := newFixture(t, false)
	reserved, err := f.store.ReserveSync(f.access, syncAttempt(f, "sync-tamper"))
	if err != nil {
		t.Fatal(err)
	}
	reserved.InputDigest = "sha256:tampered"
	if err := reserved.Validate(); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("tamper accepted: %v", err)
	}
}
