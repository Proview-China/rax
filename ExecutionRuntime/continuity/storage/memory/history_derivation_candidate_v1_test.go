package memory_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestHistoryDerivationRepositoryConcurrentDifferentContentSingleWinner(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	var winners, conflicts, unexpected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fact := historyDerivationFactV1(t, testkit.Scope(), "derivation-race", "request-race", i)
			_, replay, err := backend.CreateHistoryDerivationCandidateFactV1(ctx, fact)
			switch {
			case err == nil && !replay:
				winners.Add(1)
			case contract.HasCode(err, contract.ErrRevisionConflict):
				conflicts.Add(1)
			default:
				unexpected.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if winners.Load() != 1 || conflicts.Load() != 63 || unexpected.Load() != 0 {
		t.Fatalf("winners=%d conflicts=%d unexpected=%d", winners.Load(), conflicts.Load(), unexpected.Load())
	}
}

func TestHistoryDerivationRepositoryTenantIsolationAndNoAlias(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	base := historyDerivationFactV1(t, testkit.Scope(), "same-id", "same-request", 1)
	if _, _, err := backend.CreateHistoryDerivationCandidateFactV1(ctx, base); err != nil {
		t.Fatal(err)
	}
	otherScope := testkit.Scope()
	otherScope.TenantID = "tenant-2"
	otherScope.ExecutionScopeDigest = "tenant-2-scope"
	other := historyDerivationFactV1(t, otherScope, "same-id", "same-request", 2)
	if _, _, err := backend.CreateHistoryDerivationCandidateFactV1(ctx, other); err != nil {
		t.Fatalf("cross tenant collision: %v", err)
	}
	got, err := backend.InspectHistoryDerivationCandidateV1(ctx, ports.InspectHistoryDerivationCandidateRequestV1{Ref: base.Ref()})
	if err != nil {
		t.Fatal(err)
	}
	got.Sources[0].ProjectionDigest = "mutated"
	again, err := backend.InspectHistoryDerivationCandidateV1(ctx, ports.InspectHistoryDerivationCandidateRequestV1{Ref: base.Ref()})
	if err != nil || again.Sources[0].ProjectionDigest == "mutated" {
		t.Fatal("stored fact aliases caller")
	}
}

func historyDerivationFactV1(t *testing.T, scope contract.Scope, id, request string, variant int) contract.HistoryDerivationCandidateFactV1 {
	t.Helper()
	event := testkit.TimelineEvent(uint64(variant+1), uint64(variant+1), contract.TrustObservation)
	event.Candidate.Scope = scope
	digest, err := event.Candidate.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	event.Candidate.Digest = digest
	event.EvidenceRecordRef = event.Candidate.Evidence.RecordRef
	event.LedgerScopeDigest = event.Candidate.Evidence.LedgerScopeDigest
	event.LedgerSequence = event.Candidate.Evidence.LedgerSequence
	event.EvidenceRecordDigest = event.Candidate.Evidence.RecordDigest
	output := testkit.ContentDeltaSourceV1(scope).Target
	fact, err := contract.NewHistoryDerivationCandidateFactV1(id, request, "request-digest", scope, testkit.HistoryDerivationOwnerV1(), contract.HistoryDerivationSummary, []contract.HistoryDerivationEventRefV1{contract.HistoryDerivationEventRefFromRecordV1(event)}, output, time.Date(2026, 7, 17, 19, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return fact
}
