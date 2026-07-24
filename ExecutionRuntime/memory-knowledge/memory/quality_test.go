package memory

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestCandidateCanonicalDigestTamperRejected(t *testing.T) {
	f := newFixture(t)
	candidate := f.candidate("candidate", CandidateCreate, "sealed body", contract.Ref{}, 1)
	candidate.Subject = "tampered-after-seal"
	if _, err := f.store.SubmitCandidate(f.access, candidate); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("candidate with stale canonical digest accepted: %v", err)
	}
}

func TestDomainResultTamperRejectedOnInspectAndSettlement(t *testing.T) {
	t.Run("inspect rejects tampered persisted fact", func(t *testing.T) {
		f := newFixture(t)
		candidate := f.candidate("candidate", CandidateCreate, "tamper", contract.Ref{}, 1)
		admission := f.submitReady(candidate, "admission")
		_, _, err := f.commit(candidate, admission, "attempt", "result", "record", contract.ExpectAbsent())
		if err != nil {
			t.Fatal(err)
		}

		f.store.mu.Lock()
		stored := f.store.tenants[f.access.TenantID].attempts["attempt"]
		stored.result.SubjectRef.ID = "tampered-record"
		f.store.tenants[f.access.TenantID].attempts["attempt"] = stored
		f.store.mu.Unlock()

		if _, _, err := f.store.InspectCommit(f.access, "attempt"); !errors.Is(err, contract.ErrEvidenceConflict) {
			t.Fatalf("tampered domain result escaped inspection: %v", err)
		}
	})

	t.Run("settlement rejects tampered persisted fact", func(t *testing.T) {
		f := newFixture(t)
		candidate := f.candidate("candidate", CandidateCreate, "tamper", contract.Ref{}, 1)
		admission := f.submitReady(candidate, "admission")
		_, result, err := f.commit(candidate, admission, "attempt", "result", "record", contract.ExpectAbsent())
		if err != nil {
			t.Fatal(err)
		}

		f.store.mu.Lock()
		stored := f.store.tenants[f.access.TenantID].results[result.Ref.ID]
		stored.CASAfter++
		f.store.tenants[f.access.TenantID].results[result.Ref.ID] = stored
		f.store.mu.Unlock()

		_, err = f.store.ApplySettlement(f.access, settlementRequest(f, result.Ref, "settlement"))
		if !errors.Is(err, contract.ErrEvidenceConflict) {
			t.Fatalf("tampered domain result accepted by settlement: %v", err)
		}
	})
}

func TestDomainResultAssociationExactBinding(t *testing.T) {
	f := newFixture(t)
	candidate := f.candidate("candidate", CandidateCreate, "association", contract.Ref{}, 1)
	admission := f.submitReady(candidate, "admission")
	_, result, err := f.commit(candidate, admission, "attempt", "result", "record", contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}

	wrongRevision := result.Ref
	wrongRevision.Revision++
	if _, err := f.store.ApplySettlement(f.access, settlementRequest(f, wrongRevision, "wrong-revision")); !errors.Is(err, contract.ErrSettlementMismatch) {
		t.Fatalf("wrong domain result revision accepted: %v", err)
	}

	replacement, err := contract.NewDomainResultFact(
		contract.OwnerMemory,
		result.Ref.ID,
		"replacement-attempt",
		ref("replacement-operation"),
		ref("replacement-subject"),
		ref("replacement-inspection"),
		0,
		1,
		[]contract.Ref{ref("replacement-evidence")},
		contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1},
		"local_complete",
		nil,
		f.now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Ref.ID != result.Ref.ID || contract.SameRef(replacement.Ref, result.Ref) {
		t.Fatalf("test replacement must share ID but differ in canonical fact ref: %#v %#v", result.Ref, replacement.Ref)
	}

	f.store.mu.Lock()
	f.store.tenants[f.access.TenantID].results[result.Ref.ID] = replacement
	f.store.mu.Unlock()
	if _, err := f.store.ApplySettlement(f.access, settlementRequest(f, result.Ref, "same-id-replacement")); !errors.Is(err, contract.ErrSettlementMismatch) {
		t.Fatalf("association rebound to a different domain result under the same ID: %v", err)
	}
}

func TestUntrustedMemoryInputNeverPanics(t *testing.T) {
	f := newFixture(t)
	invalidTime := time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)

	assertNoPanic(t, "candidate seal and submit", func() {
		candidate := f.candidate("candidate", CandidateCreate, "invalid time", contract.Ref{}, 1)
		candidate.Envelope.CreatedAt = invalidTime
		candidate = SealCandidate(candidate)
		if candidate.Envelope.Digest != "" {
			t.Fatal("uncanonicalizable candidate was sealed")
		}
		if _, err := f.store.SubmitCandidate(f.access, candidate); err == nil {
			t.Fatal("uncanonicalizable candidate was accepted")
		}
	})

	assertNoPanic(t, "view publish", func() {
		watermark, err := f.store.CurrentWatermark(f.access)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.store.PublishView(f.access, View{
			Ref: contract.Ref{ID: "view", Revision: 1}, TenantID: f.access.TenantID,
			PrincipalID: f.access.IdentityID, AuthorityRef: f.access.AuthorityRef,
			AuthorityEpoch: f.access.AuthorityEpoch, PolicyRef: f.access.PolicyRef,
			Purpose: "assist", Scopes: []string{"identity_private"}, SensitivityMax: "internal",
			WatermarkRef: watermark.Ref, CreatedAt: invalidTime, ExpiresAt: f.now.Add(time.Hour),
		}, contract.ExpectAbsent())
		if err == nil {
			t.Fatal("uncanonicalizable view was accepted")
		}
	})

	assertNoPanic(t, "projection seal", func() {
		projection := SealProjection(Projection{
			Ref: contract.Ref{ID: "projection", Revision: 1}, TenantID: f.access.TenantID,
			RecordRef: ref("record"), Kind: "lexical", BuilderVersion: "v1",
			State: ProjectionReady, CreatedAt: invalidTime, ExpiresAt: f.now.Add(time.Hour),
		})
		if projection.Ref.Digest != "" {
			t.Fatal("uncanonicalizable projection was sealed")
		}
	})
}

func TestConcurrentLostReplyExactReplayDoesNotRepeatCAS(t *testing.T) {
	f := newFixture(t)
	candidate := f.candidate("candidate", CandidateCreate, "lost reply replay", contract.Ref{}, 1)
	admission := f.submitReady(candidate, "admission")
	request := CommitRequest{
		TenantID: f.access.TenantID, AttemptID: "attempt", ResultID: "result", RecordID: "record",
		CandidateRef: candidate.Ref(), AdmissionRef: admission.Ref, OperationRef: ref("operation"),
		ExpectedRevision: contract.ExpectAbsent(),
	}
	f.store.afterCAS = func(CommitInspection) error { return errors.New("lost reply") }
	if _, _, err := f.store.Commit(f.access, request); !errors.Is(err, contract.ErrUnknownOutcome) {
		t.Fatalf("want unknown outcome after durable CAS, got %v", err)
	}
	f.store.afterCAS = nil

	const replays = 32
	type replayResult struct {
		record contract.Ref
		result contract.Ref
		err    error
	}
	results := make(chan replayResult, replays)
	var wg sync.WaitGroup
	for range replays {
		wg.Add(1)
		go func() {
			defer wg.Done()
			record, result, err := f.store.Commit(f.access, request)
			results <- replayResult{record: record.Ref, result: result.Ref, err: err}
		}()
	}
	wg.Wait()
	close(results)
	for replay := range results {
		if replay.err != nil || replay.record.Revision != 1 || replay.result.ID != request.ResultID {
			t.Fatalf("exact lost-reply replay diverged: %#v", replay)
		}
	}

	f.store.mu.RLock()
	tenant := f.store.tenants[f.access.TenantID]
	recordVersions := len(tenant.records[request.RecordID])
	attempts := len(tenant.attempts)
	domainResults := len(tenant.results)
	f.store.mu.RUnlock()
	if recordVersions != 1 || attempts != 1 || domainResults != 1 {
		t.Fatalf("replay repeated authoritative mutation: records=%d attempts=%d results=%d", recordVersions, attempts, domainResults)
	}
}

func settlementRequest(f *fixture, resultRef contract.Ref, settlementID string) SettlementRequest {
	return SettlementRequest{
		TenantID:         f.access.TenantID,
		Association:      contract.DomainResultAssociation{DomainResultRef: resultRef},
		Settlement:       contract.RuntimeSettlementRef{Ref: ref(settlementID)},
		ExpectedRevision: contract.ExpectAbsent(),
	}
}

func assertNoPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("untrusted input panicked: %v", recovered)
			}
		}()
		fn()
	})
}
