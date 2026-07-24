package knowledge

import (
	"errors"
	"sync"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestKnowledgeCASConcurrentCorrectionAndSettlement(t *testing.T) {
	f := newFixture(t, true)
	_, _, first := correctionCandidate(t, f, "correction-one", 2, "alpha corrected one")
	_, _, second := correctionCandidate(t, f, "correction-two", 3, "alpha corrected two")

	type outcome struct {
		result contract.DomainResultFact
		err    error
	}
	outcomes := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, attempt := range []CommitAttempt{first, second} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			result, err := f.store.CommitAttempt(f.access, id)
			outcomes <- outcome{result: result, err: err}
		}(attempt.Ref.ID)
	}
	wg.Wait()
	close(outcomes)
	var winner contract.DomainResultFact
	successes, conflicts := 0, 0
	for outcome := range outcomes {
		if outcome.err == nil {
			successes++
			winner = outcome.result
		} else if errors.Is(outcome.err, contract.ErrRevisionConflict) {
			conflicts++
		} else {
			t.Fatalf("unexpected correction result: %v", outcome.err)
		}
	}
	if successes != 1 || conflicts != 1 || winner.CASBefore != 1 || winner.CASAfter != 2 {
		t.Fatalf("CAS was not linearized: successes=%d conflicts=%d winner=%+v", successes, conflicts, winner)
	}

	winnerAssociation := contract.DomainResultAssociation{DomainResultRef: winner.Ref}
	if _, err := f.store.ApplySettlement(f.access, winnerAssociation, contract.RuntimeSettlementRef{Ref: ref("settlement-stale")}, contract.ExpectRevision(1)); !errors.Is(err, contract.ErrRevisionConflict) {
		t.Fatalf("settlement expect-absent distinction: %v", err)
	}
	settlement := contract.RuntimeSettlementRef{Ref: ref("settlement-winner")}
	application, err := f.store.ApplySettlement(f.access, winnerAssociation, settlement, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	if application.Ref.Revision != 1 {
		t.Fatalf("unexpected application revision: %+v", application.Ref)
	}
	if _, err := f.store.ApplySettlement(f.access, winnerAssociation, contract.RuntimeSettlementRef{Ref: ref("settlement-other")}, contract.ExpectAbsent()); !errors.Is(err, contract.ErrSettlementMismatch) {
		t.Fatalf("same result accepted another settlement: %v", err)
	}
	if _, err := f.store.ApplySettlement(f.access, contract.DomainResultAssociation{DomainResultRef: f.result.Ref}, settlement, contract.ExpectAbsent()); !errors.Is(err, contract.ErrSettlementMismatch) {
		t.Fatalf("settlement reused across results: %v", err)
	}
}

func TestKnowledgeCASRejectsWrongDomainResultAssociation(t *testing.T) {
	f := newFixture(t, true)
	wrong := f.result.Ref
	wrong.Digest = "sha256:wrong-association"
	_, err := f.store.ApplySettlement(
		f.access,
		contract.DomainResultAssociation{DomainResultRef: wrong},
		contract.RuntimeSettlementRef{Ref: ref("settlement-wrong-association")},
		contract.ExpectAbsent(),
	)
	if !errors.Is(err, contract.ErrSettlementMismatch) {
		t.Fatalf("wrong association digest accepted: %v", err)
	}
}

func TestKnowledgeCASSnapshotPublicationUsesExactReadyRef(t *testing.T) {
	f := newFixture(t, true)
	firstReady, err := f.store.CreateSnapshot(f.access, SnapshotInput{
		TenantID: f.access.TenantID, ID: "snapshot-a", Version: "v1", SourceRefs: []contract.Ref{f.source.Ref},
		PackageRefs: []contract.Ref{f.pkg.Ref}, RecordRefs: []contract.Ref{f.record.Ref},
		Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1},
	}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	firstPointer, firstPublished, err := f.store.PublishSnapshot(f.access, firstReady.Ref, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	retryPointer, retryPublished, err := f.store.PublishSnapshot(f.access, firstReady.Ref, contract.ExpectAbsent())
	if err != nil || !contract.SameRef(firstPointer.Ref, retryPointer.Ref) || !contract.SameRef(firstPublished.Ref, retryPublished.Ref) {
		t.Fatalf("publish exact retry: pointer=%+v snapshot=%+v err=%v", retryPointer, retryPublished, err)
	}

	secondReady, err := f.store.CreateSnapshot(f.access, SnapshotInput{
		TenantID: f.access.TenantID, ID: "snapshot-a", Version: "v2", SourceRefs: []contract.Ref{f.source.Ref},
		PackageRefs: []contract.Ref{f.pkg.Ref}, RecordRefs: []contract.Ref{f.record.Ref},
		Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1},
	}, contract.ExpectRevision(firstPublished.Ref.Revision))
	if err != nil {
		t.Fatal(err)
	}
	secondPointer, secondPublished, err := f.store.PublishSnapshot(f.access, secondReady.Ref, contract.ExpectRevision(firstPointer.Ref.Revision))
	if err != nil {
		t.Fatal(err)
	}
	if contract.SameRef(secondPublished.Ref, firstPublished.Ref) || !contract.SameRef(secondPublished.BuiltFrom, secondReady.Ref) || secondPointer.Ref.Revision != firstPointer.Ref.Revision+1 {
		t.Fatalf("same snapshot ID swallowed new ready revision: first=%+v second=%+v pointer=%+v", firstPublished.Ref, secondPublished, secondPointer)
	}
	if _, _, err := f.store.PublishSnapshot(f.access, firstReady.Ref, contract.ExpectRevision(secondPointer.Ref.Revision)); !errors.Is(err, contract.ErrNotCurrent) {
		t.Fatalf("old ready snapshot was republished: %v", err)
	}
}
