package knowledge

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestKnowledgeQualityRejectsTamperedStoredDomainResult(t *testing.T) {
	f := newFixture(t, true)
	tampered := f.result
	tampered.CleanupState = "tampered"
	f.store.tenants[f.access.TenantID].results[tampered.Ref.ID] = tampered

	association := contract.DomainResultAssociation{DomainResultRef: f.result.Ref}
	_, err := f.store.ApplySettlement(
		f.access,
		association,
		contract.RuntimeSettlementRef{Ref: ref("settlement-tampered-result")},
		contract.ExpectAbsent(),
	)
	if !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("tampered domain result accepted by settlement: %v", err)
	}
	if _, err := f.store.CommitAttempt(f.access, f.attempt.Ref.ID); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("tampered domain result returned by commit retry: %v", err)
	}
	if _, _, err := f.store.InspectCommit(f.access, f.attempt.Ref.ID, f.attempt.OperationRef); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("tampered domain result returned by inspection: %v", err)
	}
}

func TestKnowledgeQualityRejectsSameRefSubstitutedDomainResult(t *testing.T) {
	f := newFixture(t, true)
	substitute := f.result
	substitute.AttemptID = "different-attempt-with-same-ref"
	f.store.tenants[f.access.TenantID].results[substitute.Ref.ID] = substitute

	_, err := f.store.ApplySettlement(
		f.access,
		contract.DomainResultAssociation{DomainResultRef: f.result.Ref},
		contract.RuntimeSettlementRef{Ref: ref("settlement-substituted-result")},
		contract.ExpectAbsent(),
	)
	if !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("same-ref substituted domain result accepted: %v", err)
	}
}

func TestKnowledgeQualityConcurrentLostReplyIsSingleApply(t *testing.T) {
	f := newFixture(t, false)

	const callers = 32
	results := make(chan contract.DomainResultFact, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := f.store.CommitAttempt(f.access, f.attempt.Ref.ID)
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent original-attempt recovery failed: %v", err)
		}
	}
	var exact contract.Ref
	for result := range results {
		if exact.ID == "" {
			exact = result.Ref
		}
		if !contract.SameRef(exact, result.Ref) || result.CASBefore != 0 || result.CASAfter != 1 {
			t.Fatalf("concurrent retry was not exact-idempotent: %+v", result)
		}
	}
	inspection, inspected, err := f.store.InspectCommit(f.access, f.attempt.Ref.ID, f.attempt.OperationRef)
	if err != nil || inspected == nil || inspection.Outcome != InspectionApplied || !contract.SameRef(inspected.Ref, exact) {
		t.Fatalf("lost-reply inspection did not recover exact result: inspection=%+v result=%+v err=%v", inspection, inspected, err)
	}
	if got := len(f.store.tenants[f.access.TenantID].records[f.candidate.Draft.ID]); got != 1 {
		t.Fatalf("same attempt applied %d record revisions", got)
	}
}

func TestKnowledgeQualityUntrustedCanonicalInputNeverPanics(t *testing.T) {
	f := newFixture(t, true)
	outOfRange := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("source time", func(t *testing.T) {
		assertNoPanic(t, func() error {
			_, err := f.store.RegisterSource(f.access, SourceInput{
				TenantID: f.access.TenantID, ID: "source-untrusted-time", Version: "v1", AssetRef: f.assetRef,
				ContentDigest: "sha256:untrusted", AuthorityRef: f.access.AuthorityRef, PolicyRef: f.access.PolicyRef,
				License: "internal-use", Scope: "project-a", Sensitivity: "internal", State: SourceAvailable,
				AcquiredAt: outOfRange, ValidFrom: *f.now, ValidTo: f.now.Add(time.Hour),
			}, contract.ExpectAbsent())
			return err
		})
	})

	t.Run("candidate time", func(t *testing.T) {
		assertNoPanic(t, func() error {
			_, err := f.store.SubmitCandidate(f.access, CandidateInput{
				TenantID: f.access.TenantID, ID: "candidate-untrusted-time", ProducerID: "producer-untrusted",
				SourceEpoch: 1, SourceSequence: 1, Kind: CandidateCorrection, TargetRef: f.record.Ref,
				PayloadDigest: "sha256:untrusted-time", TTL: time.Hour,
				Draft: RecordDraft{
					ID: f.record.Ref.ID, PackageRef: f.pkg.Ref, ContentRef: f.contentRef, SourceRefs: []contract.Ref{f.source.Ref},
					Scope: f.record.Scope, Subject: f.record.Subject, Sensitivity: f.record.Sensitivity,
					License: f.record.License, TrustState: f.record.TrustState,
					ValidFrom: outOfRange, ValidTo: outOfRange.Add(time.Hour),
				},
			}, contract.ExpectAbsent())
			return err
		})
	})
}

func assertNoPanic(t *testing.T, call func() error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("untrusted input panicked: %v", recovered)
		}
	}()
	if err := call(); !errors.Is(err, contract.ErrInvalidArgument) {
		t.Fatalf("untrusted canonical input error = %v, want invalid argument", err)
	}
}
