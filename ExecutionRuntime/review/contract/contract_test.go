package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestTargetCurrentnessAndDrift(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	target := testkit.Target(now)
	current := contract.TargetCurrentnessV1{TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, PayloadRevision: target.PayloadRevision, PayloadDigest: target.PayloadDigest, Scope: target.Scope, ActionScopeDigest: target.ActionScopeDigest, Policy: target.Policy, ActorAuthority: target.ActorAuthority, CurrentScope: target.CurrentScope, EvidenceSetDigest: target.EvidenceSetDigest, ContextFrameDigest: target.ContextFrameDigest, Now: now.Add(time.Minute)}
	if err := target.ValidateCurrent(current); err != nil {
		t.Fatal(err)
	}
	current.PayloadDigest = testkit.Digest("drift")
	if err := target.ValidateCurrent(current); !core.HasReason(err, core.ReasonReviewCandidateConflict) {
		t.Fatalf("expected candidate conflict, got %v", err)
	}
}

func TestDomainResultSettlementLayering(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	target := testkit.Target(now)
	c := contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{TenantID: "tenant-a", ID: "case-a", Revision: 4}, CurrentRoundID: "round-a", CurrentAssignment: "assignment-a", TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest}
	round := testkit.Round(now, c, contract.RouteAutoV1)
	assignment := testkit.Assignment(now, c, round, contract.RouteAutoV1)
	result := testkit.DomainResult(now, c, round, assignment)
	settlement := testkit.RuntimeSettlement(result)
	applied, err := contract.ApplyRuntimeSettlementV1("apply-a", result, settlement, now.Add(time.Second).UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if applied.State != contract.DomainApplyAppliedV1 {
		t.Fatalf("unexpected apply state %s", applied.State)
	}
	settlement.DomainResultDigest = testkit.Digest("other-result")
	if _, err := contract.ApplyRuntimeSettlementV1("apply-b", result, settlement, now.Add(time.Second).UnixNano()); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("expected exact domain result rejection, got %v", err)
	}
}

func TestAutoAttestationRequiresApplySettlement(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	target := testkit.Target(now)
	c := contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{TenantID: "tenant-a", ID: "case-a", Revision: 4}, CurrentRoundID: "round-a", CurrentAssignment: "assignment-a", TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest}
	round := testkit.Round(now, c, contract.RouteHumanV1)
	assignment := testkit.Assignment(now, c, round, contract.RouteHumanV1)
	human := testkit.HumanAttestation(now, c, round, assignment, contract.ResolutionAcceptV1, "human")
	human.Route = contract.RouteAutoV1
	human.Digest = ""
	if _, err := contract.SealAttestationV1(human); !core.HasReason(err, core.ReasonEffectSettlementMissing) {
		t.Fatalf("expected missing settlement, got %v", err)
	}
}
