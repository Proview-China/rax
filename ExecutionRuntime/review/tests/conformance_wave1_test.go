package review_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestConformanceWave1LifecycleHistoryAndSettlement(t *testing.T) {
	resolved := newResolvedFlow(t, 5*time.Minute)
	if _, err := resolved.store.InspectRoundExactV1(resolved.ctx, resolved.round.TenantID, reviewport.ExactV1(resolved.round.ID, resolved.round.Revision, resolved.round.Digest)); err != nil {
		t.Fatal(err)
	}
	if _, err := resolved.store.InspectAssignmentExactV1(resolved.ctx, resolved.assignment.TenantID, reviewport.ExactV1(resolved.assignment.ID, resolved.assignment.Revision, resolved.assignment.Digest)); err != nil {
		t.Fatal(err)
	}
	if _, err := resolved.store.InspectAttestationExactV1(resolved.ctx, resolved.attestation.TenantID, reviewport.ExactV1(resolved.attestation.ID, resolved.attestation.Revision, resolved.attestation.Digest)); err != nil {
		t.Fatal(err)
	}
	if _, err := resolved.store.InspectVerdictExactV1(resolved.ctx, resolved.verdict.TenantID, reviewport.ExactV1(resolved.verdict.ID, resolved.verdict.Revision, resolved.verdict.Digest)); err != nil {
		t.Fatal(err)
	}
	resolved.clock.Advance(time.Second)
	revoked, currentVerdict, err := resolved.owner.RevokeV1(resolved.ctx, resolved.resolved.TenantID, resolved.resolved.ID, reviewport.ExpectedV1(resolved.resolved.Revision, resolved.resolved.Digest), core.ReasonReviewVerdictStale, testkit.Trace(resolved.clock.Now(), resolved.resolved, contract.TraceRevokedV1, 5, resolved.verdict.ID))
	if err != nil {
		t.Fatal(err)
	}
	if currentVerdict == nil || currentVerdict.Revision != resolved.verdict.Revision+1 {
		t.Fatal("Invalidate did not append Verdict revision")
	}
	if _, err := resolved.store.InspectCaseExactV1(resolved.ctx, resolved.resolved.TenantID, reviewport.ExactV1(resolved.resolved.ID, resolved.resolved.Revision, resolved.resolved.Digest)); err != nil {
		t.Fatal(err)
	}
	if _, err := resolved.store.InspectCaseExactV1(resolved.ctx, revoked.TenantID, reviewport.ExactV1(revoked.ID, revoked.Revision, revoked.Digest)); err != nil {
		t.Fatal(err)
	}
	if _, err := resolved.store.InspectVerdictExactV1(resolved.ctx, resolved.verdict.TenantID, reviewport.ExactV1(resolved.verdict.ID, resolved.verdict.Revision, resolved.verdict.Digest)); err != nil {
		t.Fatal(err)
	}
	if _, err := resolved.store.InspectVerdictExactV1(resolved.ctx, currentVerdict.TenantID, reviewport.ExactV1(currentVerdict.ID, currentVerdict.Revision, currentVerdict.Digest)); err != nil {
		t.Fatal(err)
	}

	auto := newReviewingFlow(t, contract.RouteAutoV1)
	auto.clock.Advance(time.Second)
	result := testkit.DomainResult(auto.clock.Now(), auto.caseValue, auto.round, auto.assignment)
	if _, err := auto.store.CreateDomainResultV1(auto.ctx, result); err != nil {
		t.Fatal(err)
	}
	apply, err := contract.ApplyRuntimeSettlementV1("apply-conformance", result, testkit.RuntimeSettlement(result), auto.clock.Now().Add(time.Nanosecond).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auto.store.CreateApplySettlementV1(auto.ctx, apply); err != nil {
		t.Fatal(err)
	}
	if _, err := auto.store.InspectDomainResultExactV1(auto.ctx, result.TenantID, reviewport.ExactV1(result.ID, result.Revision, result.Digest)); err != nil {
		t.Fatal(err)
	}
	if _, err := auto.store.InspectApplySettlementExactV1(auto.ctx, apply.TenantID, reviewport.ExactV1(apply.ID, apply.Revision, apply.Digest)); err != nil {
		t.Fatal(err)
	}
}
