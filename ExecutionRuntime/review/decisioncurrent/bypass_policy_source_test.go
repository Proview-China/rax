package decisioncurrent

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestBypassPolicySourceV1ExactNotRequiredS1S2(t *testing.T) {
	decision, reader, clock := bypassPolicyFixtureV1(t)
	source, err := NewBypassPolicySourceV1(reader, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := source.ReadBypassCurrentV1(context.Background(), decision, clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if proof.Digest != decision.ExternalProof.Digest || reader.resolveCalls != 1 || reader.inspectCalls != 2 {
		t.Fatalf("Bypass Policy cut was not one exact S1/S2: proof=%+v resolve=%d inspect=%d", proof, reader.resolveCalls, reader.inspectCalls)
	}
}

func TestBypassPolicySourceV1RejectsRequiredOrDriftedPolicy(t *testing.T) {
	for _, name := range []string{"required", "s2_drift"} {
		t.Run(name, func(t *testing.T) {
			decision, reader, clock := bypassPolicyFixtureV1(t)
			switch name {
			case "required":
				fact := reader.value.Fact
				fact.OperationNotRequired = false
				fact.Digest, _ = fact.DigestV2()
				subject := reader.value.Subject
				subject.Policy = runtimeports.ReviewPolicyBindingRefV2{Ref: fact.Ref, Revision: fact.Revision, Digest: fact.Digest}
				reader.value = sealExternalPolicyV1(t, subject, fact, time.Unix(0, reader.value.CheckedUnixNano), time.Unix(0, reader.value.ExpiresUnixNano))
				decision.Policy = runtimeports.ReviewPolicyBindingRefV2{Ref: fact.Ref, Revision: fact.Revision, Digest: fact.Digest}
				decision.PolicyCurrentProjection = reader.value.Ref
				proof, err := contract.SealBypassExternalCurrentProofV1(contract.BypassExternalCurrentProofV1{Policy: reader.value.Ref, CheckedUnixNano: reader.value.CheckedUnixNano, ExpiresUnixNano: reader.value.ExpiresUnixNano})
				if err != nil {
					t.Fatal(err)
				}
				decision.ExternalProof = proof
				decision, err = contract.SealBypassDecisionV1(decision)
				if err != nil {
					t.Fatal(err)
				}
			case "s2_drift":
				reader.driftOnInspect = 2
			}
			source, _ := NewBypassPolicySourceV1(reader, clock.Now)
			_, err := source.ReadBypassCurrentV1(context.Background(), decision, clock.Now())
			if name == "required" && !core.HasCategory(err, core.ErrorForbidden) {
				t.Fatalf("Policy-required route was not forbidden: %v", err)
			}
			if name == "s2_drift" && !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("Policy S2 drift was not conflict: %v", err)
			}
		})
	}
}

func TestBypassPolicySourceV1LostReplyDetachedAndClockRollback(t *testing.T) {
	t.Run("lost exact reply", func(t *testing.T) {
		decision, reader, clock := bypassPolicyFixtureV1(t)
		ctx, cancel := context.WithCancel(context.Background())
		reader.loseInspectReply, reader.cancel = true, cancel
		source, _ := NewBypassPolicySourceV1(reader, clock.Now)
		if _, err := source.ReadBypassCurrentV1(ctx, decision, clock.Now()); err != nil {
			t.Fatal(err)
		}
		if reader.inspectCalls != 3 || ctx.Err() == nil {
			t.Fatalf("lost exact reply did not recover once on detached context: calls=%d ctx=%v", reader.inspectCalls, ctx.Err())
		}
	})
	t.Run("clock rollback", func(t *testing.T) {
		decision, reader, clock := bypassPolicyFixtureV1(t)
		reader.rollbackClockOnInspect, reader.clock = 1, clock
		source, _ := NewBypassPolicySourceV1(reader, clock.Now)
		if _, err := source.ReadBypassCurrentV1(context.Background(), decision, clock.Now()); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock rollback was not rejected: %v", err)
		}
	})
}

func bypassPolicyFixtureV1(t *testing.T) (contract.BypassDecisionV1, *policyReaderV1, *externalClockV1) {
	t.Helper()
	base := time.Unix(2_300_000_000, 0)
	clock := &externalClockV1{value: base.Add(time.Second)}
	tenant := core.TenantID("tenant-bypass-policy")
	digest := externalDigestV1
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: tenant, ID: "agent-bypass-policy", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-bypass-policy", PlanDigest: digest("plan-bypass")},
		Instance:       core.InstanceRef{ID: "instance-bypass-policy", Epoch: 2},
		AuthorityEpoch: 3,
	}
	target := runtimeports.ReviewDecisionTargetRefV1{TenantID: tenant, ID: "target-bypass-policy", Revision: 4, Digest: digest("target-bypass"), RunID: "run-bypass-policy"}
	authority := runtimeports.AuthorityBindingRefV2{Ref: "authority-bypass", Revision: 5, Digest: digest("authority-bypass"), Epoch: scope.AuthorityEpoch}
	currentScope := runtimeports.ExecutionScopeBindingRefV2{Ref: "scope-bypass", Revision: 6, Digest: digest("scope-bypass")}
	fact := runtimeports.ReviewPolicyFactV2{
		Ref: "policy-bypass", Revision: 7, SubjectDigest: target.Digest, Scope: scope, RunID: target.RunID,
		CurrentScope: currentScope, RiskClass: "review/low", ActorAuthorityRef: authority.Ref,
		ReviewerAuthorityRef: "policy/not-applicable", OperationNotRequired: true,
		PolicyDecisionRef: "policy-decision-bypass", Active: true, ExpiresUnixNano: base.Add(10 * time.Minute).UnixNano(),
	}
	fact.Digest, _ = fact.DigestV2()
	policy := runtimeports.ReviewPolicyBindingRefV2{Ref: fact.Ref, Revision: fact.Revision, Digest: fact.Digest}
	subject := runtimeports.ReviewDecisionPolicyCurrentSubjectV1{Target: target, Policy: policy}
	projection := sealExternalPolicyV1(t, subject, fact, base, base.Add(5*time.Minute))
	proof, err := contract.SealBypassExternalCurrentProofV1(contract.BypassExternalCurrentProofV1{Policy: projection.Ref, CheckedUnixNano: projection.CheckedUnixNano, ExpiresUnixNano: projection.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := contract.SealBypassDecisionV1(contract.BypassDecisionV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: tenant, ID: "bypass-policy-1", Revision: 1, CreatedUnixNano: base.Add(time.Second).UnixNano(), UpdatedUnixNano: base.Add(time.Second).UnixNano()},
		Target:         contract.BypassTargetExactRefV1{TenantID: tenant, ID: target.ID, Revision: target.Revision, Digest: target.Digest},
		Case:           contract.BypassCaseExactRefV1{TenantID: tenant, ID: "case-bypass-policy", Revision: 2, Digest: digest("case-bypass")},
		IntentID:       "intent-bypass", IntentRevision: 2, SubjectDigest: digest("subject-bypass"),
		PayloadRevision: 3, PayloadDigest: digest("payload-bypass"), Scope: scope, RunID: target.RunID,
		ActionScopeDigest: digest("action-bypass"), Policy: policy, PolicyCurrentProjection: projection.Ref,
		PolicyDecisionRef: fact.PolicyDecisionRef, ActorAuthority: authority, CurrentScope: currentScope,
		TargetEvidenceSetDigest: digest("evidence-bypass"), Profile: contract.ProfileYOLOV1,
		Risk: contract.RiskLowV1, EffectClass: contract.EffectObserveOnlyV1, Environment: contract.EnvironmentProductionV1,
		RouteDecisionDigest: digest("route-bypass"), ExternalProof: proof, State: contract.BypassDecisionActiveV1,
		ExpiresUnixNano: projection.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	return decision, &policyReaderV1{value: projection, clock: clock}, clock
}
