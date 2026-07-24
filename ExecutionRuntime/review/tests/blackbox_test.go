package review_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/verdictowner"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type unsupportedExternalCurrentReaderV1 struct{}

func (unsupportedExternalCurrentReaderV1) InspectDecisionExternalCurrentV1(context.Context, reviewport.DecisionExternalCurrentRequestV1) (reviewport.DecisionExternalCurrentProjectionV1, error) {
	return reviewport.DecisionExternalCurrentProjectionV1{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "production external current readers are unsupported")
}

func TestBlackboxHumanVerdictCurrentnessAndRevoke(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(time.Second)
	att := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-human")
	c, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), att, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 3, att.ID))
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	owner := newVerdictOwner(t, f, nil)
	resolved, v, err := owner.DecideV1(f.ctx, verdictowner.DecideCommandV1{TenantID: c.TenantID, CaseID: c.ID, Expected: reviewport.ExpectedV1(c.Revision, c.Digest), AttestationID: att.ID, VerdictID: "verdict-a", Trace: testkit.Trace(f.clock.Now(), c, contract.TraceVerdictV1, 4, "verdict-a")})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.State != contract.CaseResolvedV1 || v.State != contract.VerdictAcceptedV1 {
		t.Fatalf("unexpected resolved facts: %s %s", resolved.State, v.State)
	}
	if _, err := owner.InspectCurrentV1(f.ctx, v.TenantID, v.ID); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	revoked, rv, err := owner.RevokeV1(f.ctx, resolved.TenantID, resolved.ID, reviewport.ExpectedV1(resolved.Revision, resolved.Digest), core.ReasonReviewVerdictStale, testkit.Trace(f.clock.Now(), resolved, contract.TraceRevokedV1, 5, v.ID))
	if err != nil {
		t.Fatal(err)
	}
	if revoked.State != contract.CaseRevokedV1 || rv == nil || rv.State != contract.VerdictRevokedV1 {
		t.Fatalf("revoke did not invalidate exact case and verdict")
	}
}

func TestBlackboxLegacyV3AutoSettlementFailsClosedBeforeAttestationV1(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteAutoV1)
	f.clock.Advance(time.Second)
	result := testkit.DomainResult(f.clock.Now(), f.caseValue, f.round, f.assignment)
	if _, err := f.store.CreateDomainResultV1(f.ctx, result); err != nil {
		t.Fatal(err)
	}
	settlement := testkit.RuntimeSettlement(result)
	apply, err := contract.ApplyRuntimeSettlementV1("apply-a", result, settlement, f.clock.Now().Add(time.Nanosecond).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CreateApplySettlementV1(f.ctx, apply); err != nil {
		t.Fatal(err)
	}
	att := testkit.AutoAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, result, apply)
	_, _, err = f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), att, testkit.AttestedTrace(f.clock.Now(), f.caseValue, 3, att.ID, att.ID, apply.ID, result.ID))
	if !core.HasReason(err, core.ReasonEffectSettlementMissing) {
		t.Fatalf("legacy Runtime V3 Auto settlement reached production Attestation: %v", err)
	}
	if _, inspectErr := f.store.InspectAttestationV1(f.ctx, att.TenantID, att.ID); !core.HasCategory(inspectErr, core.ErrorNotFound) {
		t.Fatalf("rejected V3 Auto settlement leaked Attestation: %v", inspectErr)
	}
}

func TestBlackboxVerdictShortestTTLExpiresCase(t *testing.T) {
	r := newResolvedFlow(t, 2*time.Minute)
	r.clock.Advance(2 * time.Minute)
	expired, v, err := r.owner.ExpireV1(r.ctx, r.resolved.TenantID, r.resolved.ID, reviewport.ExpectedV1(r.resolved.Revision, r.resolved.Digest), core.ReasonReviewVerdictStale, testkit.Trace(r.clock.Now(), r.resolved, contract.TraceExpiredV1, 5, r.verdict.ID))
	if err != nil {
		t.Fatal(err)
	}
	if expired.State != contract.CaseExpiredV1 || v == nil || v.State != contract.VerdictExpiredV1 {
		t.Fatalf("shortest TTL did not expire case and verdict")
	}
}

func TestBlackboxSupersedeInvalidatesVerdict(t *testing.T) {
	r := newResolvedFlow(t, 5*time.Minute)
	r.clock.Advance(time.Second)
	superseded, v, err := r.owner.SupersedeV1(r.ctx, r.resolved.TenantID, r.resolved.ID, reviewport.ExpectedV1(r.resolved.Revision, r.resolved.Digest), core.ReasonReviewCandidateConflict, testkit.Trace(r.clock.Now(), r.resolved, contract.TraceSupersededV1, 5, r.verdict.ID))
	if err != nil {
		t.Fatal(err)
	}
	if superseded.State != contract.CaseSupersededV1 || v == nil || v.State != contract.VerdictSupersededV1 {
		t.Fatalf("supersede did not invalidate exact verdict")
	}
}

func TestBlackboxFindingOwnerCompoundCreateOnceInspect(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(time.Second)
	evidence := []runtimeports.ReviewEvidenceRefV2{testkit.Evidence("finding")}
	finding, err := contract.SealFindingV1(contract.FindingV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: f.caseValue.TenantID, ID: "finding-a", Revision: 1, CreatedUnixNano: f.clock.Now().UnixNano(), UpdatedUnixNano: f.clock.Now().UnixNano()}, CaseID: f.caseValue.ID, CaseRevision: f.caseValue.Revision, RoundID: f.round.ID, RoundRevision: f.round.Revision, RoundDigest: f.round.Digest, TargetID: f.caseValue.TargetID, TargetRevision: f.caseValue.TargetRevision, TargetDigest: f.caseValue.TargetDigest, Category: "review.test/correctness", Priority: "high", Anchor: "file.go:1", Claim: "candidate violates invariant", Impact: "execution must remain blocked", Evidence: evidence, Status: contract.FindingOpenV1, ExpiresUnixNano: f.clock.Now().Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	trace := testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceFindingV1, 70, finding.ID)
	mutation := reviewport.CreateFindingWithTraceMutationV2{Finding: finding, Trace: trace}
	created, err := f.engine.CreateFindingWithTraceV2(f.ctx, mutation)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := f.engine.CreateFindingWithTraceV2(f.ctx, mutation)
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := f.engine.InspectFindingV1(f.ctx, finding.TenantID, finding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if created.Digest != replayed.Digest || created.Digest != inspected.Digest {
		t.Fatalf("finding create-once/Inspect lost exact fact")
	}
	if _, err := f.store.InspectTraceExactV1(f.ctx, trace.TenantID, reviewport.ExactV1(trace.ID, trace.Revision, trace.Digest)); err != nil {
		t.Fatalf("compound Engine Finding lost exact FindingObserved Trace: %v", err)
	}
}

func TestBlackboxExternalCurrentUnsupportedNeverReachesVerdictCAS(t *testing.T) {
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-unsupported-current")
	attested, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), attestation, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 3, attestation.ID))
	if err != nil {
		t.Fatal(err)
	}
	current, err := memory.NewDecisionCurrentSourceV1(f.store, unsupportedExternalCurrentReaderV1{}, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	countingStore := &decideCallCountingStore{StoreV1: f.store}
	owner, err := verdictowner.New(countingStore, current, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = owner.DecideV1(f.ctx, verdictowner.DecideCommandV1{TenantID: attested.TenantID, CaseID: attested.ID, Expected: reviewport.ExpectedV1(attested.Revision, attested.Digest), AttestationID: attestation.ID, VerdictID: "verdict-unsupported-current", Trace: testkit.Trace(f.clock.Now(), attested, contract.TraceVerdictV1, 4, "verdict-unsupported-current")})
	if !core.HasCategory(err, core.ErrorCapabilityUnavailable) || !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("unsupported external current reader did not fail closed: %v", err)
	}
	if countingStore.decideCalls.Load() != 0 {
		t.Fatalf("unsupported external current reached Verdict CAS: %d", countingStore.decideCalls.Load())
	}
	assertCurrentCaseExact(t, f.store, attested)
	if _, inspectErr := f.store.InspectVerdictV1(f.ctx, attested.TenantID, "verdict-unsupported-current"); !core.HasCategory(inspectErr, core.ErrorNotFound) {
		t.Fatalf("unsupported external current leaked Verdict: %v", inspectErr)
	}
}
