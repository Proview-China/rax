package review_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/caseengine"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/verdictowner"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type typedNilExternalCurrentReaderV1 struct{}

func (*typedNilExternalCurrentReaderV1) InspectDecisionExternalCurrentV1(context.Context, reviewport.DecisionExternalCurrentRequestV1) (reviewport.DecisionExternalCurrentProjectionV1, error) {
	panic("typed-nil external reader must be rejected at construction")
}

type typedNilDecisionCurrentReaderV1 struct{}

func (*typedNilDecisionCurrentReaderV1) InspectDecisionCurrentV1(context.Context, reviewport.DecisionCurrentRequestV1) (contract.DecisionCurrentSnapshotV1, error) {
	panic("typed-nil decision reader must be rejected at construction")
}

type replyLostStore struct {
	reviewport.StoreV1
	lost atomic.Bool
}

func (s *replyLostStore) RecordAttestationV1(ctx context.Context, m reviewport.RecordAttestationMutationV1) (contract.ReviewCaseV1, contract.AttestationV1, error) {
	c, a, err := s.StoreV1.RecordAttestationV1(ctx, m)
	if err == nil && s.lost.CompareAndSwap(false, true) {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "simulated reply loss")
	}
	return c, a, err
}

func TestFaultAttestationReplyLossInspectsSameIdempotencyKey(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	wrapped := &replyLostStore{StoreV1: f.store}
	engine, _ := caseengine.New(wrapped, f.clock.Now)
	f.clock.Advance(1)
	att := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-loss")
	trace := testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 3, att.ID)
	expected := reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest)
	c, replayed, err := engine.RecordAttestationV1(f.ctx, expected, att, trace)
	if err != nil {
		t.Fatalf("lost reply exact Inspect did not recover: %v", err)
	}
	if replayed.Digest != att.Digest || c.State != contract.CaseAttestedV1 {
		t.Fatalf("recovery did not inspect original attestation")
	}
}

func TestFaultConcurrentCASHasOneWinner(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	current := f.caseValue
	next := current
	next.Revision++
	next.State = contract.CaseAttestedV1
	next.UpdatedUnixNano++
	next.Digest = ""
	next, _ = contract.SealReviewCaseV1(next)
	const workers = 32
	var wins atomic.Int32
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			if _, err := f.store.AdvanceCaseForTestV1(context.Background(), reviewport.ExpectedV1(current.Revision, current.Digest), next); err == nil {
				wins.Add(1)
			} else if !core.HasReason(err, core.ReasonRevisionConflict) {
				t.Errorf("unexpected CAS error: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("CAS winners=%d want 1", wins.Load())
	}
}

func TestFaultIdempotencyPayloadMismatch(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(1)
	a := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-conflict")
	trace := testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 3, a.ID)
	expected := reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest)
	if _, _, err := f.engine.RecordAttestationV1(f.ctx, expected, a, trace); err != nil {
		t.Fatal(err)
	}
	b := a
	b.ID = "attestation-other"
	b.ReasonCodes = []string{"review.test/different"}
	b.Digest = ""
	b, _ = contract.SealAttestationV1(b)
	if _, _, err := f.engine.RecordAttestationV1(f.ctx, expected, b, trace); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("expected idempotency mismatch, got %v", err)
	}
}

func TestFaultVerdictReplayPayloadMismatch(t *testing.T) {
	r := newResolvedFlow(t, 5*time.Minute)
	_, _, err := r.owner.DecideV1(r.ctx, verdictowner.DecideCommandV1{TenantID: r.resolved.TenantID, CaseID: r.resolved.ID, Expected: reviewport.ExpectedV1(r.verdict.CaseRevision, r.verdict.CaseDigest), AttestationID: "different-attestation", VerdictID: r.verdict.ID})
	if !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("expected Verdict replay mismatch, got %v", err)
	}
}

func TestFaultConstructorsRejectTypedNilDependencies(t *testing.T) {
	clock := testkit.NewClock(time.Unix(1_750_400_000, 0))
	var typedNilStore *memory.Store
	if _, err := caseengine.New(typedNilStore, clock.Now); err == nil {
		t.Fatal("case engine accepted a typed-nil Store")
	}

	store := memory.NewStore()
	var typedNilExternal *typedNilExternalCurrentReaderV1
	if _, err := memory.NewDecisionCurrentSourceV1(store, typedNilExternal, time.Now); err == nil {
		t.Fatal("decision current source accepted a typed-nil external reader")
	}

	var typedNilCurrent *typedNilDecisionCurrentReaderV1
	if _, err := verdictowner.New(store, typedNilCurrent, clock.Now); err == nil {
		t.Fatal("verdict owner accepted a typed-nil current reader")
	}
	if _, err := verdictowner.New(typedNilStore, &typedNilDecisionCurrentReaderV1{}, clock.Now); err == nil {
		t.Fatal("verdict owner accepted a typed-nil Store")
	}
}
