package contract

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestBypassDecisionV1IsIndependentCurrentPolicyFact(t *testing.T) {
	now := time.Unix(1710000000, 0)
	value := bypassFixtureV1(t, now)
	sealed, err := SealBypassDecisionV1(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := sealed.ValidateCurrent(sealed.Target, sealed.Case, sealed.PolicyCurrentProjection, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	drift := sealed.PolicyCurrentProjection
	drift.Revision++
	if err := sealed.ValidateCurrent(sealed.Target, sealed.Case, drift, now.Add(time.Second)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("Policy projection drift must fail closed, got %v", err)
	}
	if sealed.State == "accepted" {
		t.Fatal("BypassDecision must never encode an accepted Verdict")
	}
}

func TestBypassDecisionV1RejectsPartialEffectAndCrossTenant(t *testing.T) {
	now := time.Unix(1710000000, 0)
	partial := bypassFixtureV1(t, now)
	partial.IntentRevision = 0
	if _, err := SealBypassDecisionV1(partial); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("partial Effect binding accepted: %v", err)
	}
	cross := bypassFixtureV1(t, now)
	cross.Case.TenantID = "other"
	if _, err := SealBypassDecisionV1(cross); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("cross-tenant Case accepted: %v", err)
	}
}

func TestBypassDecisionV1TerminalAndTTLFailClosed(t *testing.T) {
	now := time.Unix(1710000000, 0)
	active, err := SealBypassDecisionV1(bypassFixtureV1(t, now))
	if err != nil {
		t.Fatal(err)
	}
	if err := active.ValidateCurrent(active.Target, active.Case, active.PolicyCurrentProjection, time.Unix(0, active.ExpiresUnixNano)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("TTL boundary accepted: %v", err)
	}
	revoked := bypassFixtureV1(t, now)
	revoked.State = BypassDecisionRevokedV1
	revoked.InvalidationReason = core.ReasonReviewVerdictStale
	revoked.UpdatedUnixNano++
	sealed, err := SealBypassDecisionV1(revoked)
	if err != nil {
		t.Fatal(err)
	}
	if err := sealed.ValidateCurrent(sealed.Target, sealed.Case, sealed.PolicyCurrentProjection, now.Add(time.Second)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("revoked decision accepted as current: %v", err)
	}
}

func bypassFixtureV1(t *testing.T, now time.Time) BypassDecisionV1 {
	t.Helper()
	tenant := core.TenantID("tenant-bypass")
	digest := func(s string) core.Digest { return core.DigestBytes([]byte(s)) }
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: tenant, ID: "agent-bypass", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-bypass", PlanDigest: digest("plan")},
		Instance:       core.InstanceRef{ID: "instance-bypass", Epoch: 3},
		AuthorityEpoch: 2,
	}
	policy := runtimeports.ReviewPolicyBindingRefV2{Ref: "policy-bypass", Revision: 4, Digest: digest("policy")}
	policyCurrent := runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1{ID: "policy-current-1", Revision: 5, Digest: digest("policy-current")}
	expires := now.Add(time.Minute).UnixNano()
	proof, err := SealBypassExternalCurrentProofV1(BypassExternalCurrentProofV1{
		Policy:          policyCurrent,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return BypassDecisionV1{
		FactIdentityV1: FactIdentityV1{TenantID: tenant, ID: "bypass-1", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		Target:         BypassTargetExactRefV1{TenantID: tenant, ID: "target-1", Revision: 2, Digest: digest("target")},
		Case:           BypassCaseExactRefV1{TenantID: tenant, ID: "case-1", Revision: 3, Digest: digest("case")},
		IntentID:       "intent-1", IntentRevision: 2, SubjectDigest: digest("subject"), PayloadRevision: 7, PayloadDigest: digest("payload"),
		Scope: scope, RunID: "run-bypass", ActionScopeDigest: digest("action-scope"), Policy: policy,
		PolicyCurrentProjection: policyCurrent,
		PolicyDecisionRef:       "policy-decision-1",
		ActorAuthority:          runtimeports.AuthorityBindingRefV2{Ref: "authority-1", Revision: 8, Digest: digest("authority"), Epoch: 2},
		CurrentScope:            runtimeports.ExecutionScopeBindingRefV2{Ref: "scope-current-1", Revision: 9, Digest: digest("scope")},
		TargetEvidenceSetDigest: digest("evidence"), Profile: ProfileYOLOV1, Risk: RiskLowV1, EffectClass: EffectObserveOnlyV1, Environment: EnvironmentProductionV1,
		RouteDecisionDigest: digest("route"), ExternalProof: proof, State: BypassDecisionActiveV1, ExpiresUnixNano: expires,
	}
}
