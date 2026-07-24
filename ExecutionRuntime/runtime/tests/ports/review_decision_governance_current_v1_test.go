package ports_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewDecisionGovernanceCurrentV1CanonicalGolden(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_800_000_000, 0)
	policy := reviewDecisionPolicyProjectionV1(t, now)
	authority := reviewDecisionAuthorityProjectionV1(t, now)
	scope := reviewDecisionScopeProjectionV1(t, now)

	golden := map[string]string{
		"policy_id":        "review-policy-current-b1aa79ab58d4dad52bd3b89fc64b105cff74d018b7f6f2c5222b264bc543c7bc",
		"policy_digest":    "sha256:2cb08078fed853ff4b84dfe5854fb818b1df2c0a71dde20cc94720741eeba833",
		"authority_id":     "review-authority-current-430150b4a84b646a7f6b6e07c2c2c5e933849967d7948846cbba2768c5a39920",
		"authority_digest": "sha256:06c816b3bf419d93d80484845f56d8acb97e1356a7fecffdbae6b10a3407ce33",
		"scope_id":         "review-scope-current-76a79d343bddd7adb7b28cacf9215d3e06d50763fcd7d5bb3580f5804ec71e29",
		"scope_digest":     "sha256:f8711c24cdf724ae7197359d9354e5db0171efddeeb0e6fbe26c01e956d1bde2",
	}
	actual := map[string]string{
		"policy_id": string(policy.Ref.ID), "policy_digest": string(policy.ProjectionDigest),
		"authority_id": string(authority.Ref.ID), "authority_digest": string(authority.ProjectionDigest),
		"scope_id": string(scope.Ref.ID), "scope_digest": string(scope.ProjectionDigest),
	}
	for key, want := range golden {
		if actual[key] != want {
			t.Fatalf("%s canonical golden drifted: got %q want %q", key, actual[key], want)
		}
	}
}

func TestReviewDecisionGovernanceCurrentV1RejectsSubjectAndDigestDrift(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_800_000_000, 0)

	policy := reviewDecisionPolicyProjectionV1(t, now)
	policy.Subject.Target.Digest = reviewDecisionDigestV1("other-target")
	if err := policy.Validate(); err == nil {
		t.Fatal("policy accepted Target drift")
	}

	authority := reviewDecisionAuthorityProjectionV1(t, now)
	authority.Subject.Assignment.ReviewerID = "other-reviewer"
	if err := authority.Validate(); err == nil {
		t.Fatal("authority accepted Assignment/Reviewer drift")
	}
	authority = reviewDecisionAuthorityProjectionV1(t, now)
	authority.Subject.Assignment.TenantID = "other-tenant"
	if err := authority.Validate(); err == nil {
		t.Fatalf("authority accepted cross-tenant Assignment: %v", err)
	}

	scope := reviewDecisionScopeProjectionV1(t, now)
	scope.Subject.ActionScopeDigest = reviewDecisionDigestV1("other-action")
	if err := scope.Validate(); err == nil {
		t.Fatal("scope accepted action-scope drift without a new sealed projection")
	}
}

func TestReviewDecisionGovernanceCurrentV1TTLAndClockAreClosed(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_800_000_000, 0)
	policy := reviewDecisionPolicyProjectionV1(t, now)
	authority := reviewDecisionAuthorityProjectionV1(t, now)
	scope := reviewDecisionScopeProjectionV1(t, now)

	checks := []struct {
		name     string
		validate func(time.Time) error
	}{
		{"policy", func(at time.Time) error { return policy.ValidateCurrent(policy.Ref, policy.Subject, at) }},
		{"authority", func(at time.Time) error { return authority.ValidateCurrent(authority.Ref, authority.Subject, at) }},
		{"scope", func(at time.Time) error { return scope.ValidateCurrent(scope.Ref, scope.Subject, at) }},
	}
	for _, check := range checks {
		check := check
		t.Run(check.name, func(t *testing.T) {
			if err := check.validate(now.Add(time.Second)); err != nil {
				t.Fatalf("current projection rejected: %v", err)
			}
			if err := check.validate(now.Add(-time.Nanosecond)); !core.HasReason(err, core.ReasonClockRegression) {
				t.Fatalf("clock rollback did not fail closed: %v", err)
			}
			if err := check.validate(now.Add(30 * time.Second)); err == nil {
				t.Fatal("TTL boundary did not fail closed")
			}
		})
	}
}

func TestReviewDecisionGovernanceCurrentV1PublishRevisionIsExact(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_800_000_000, 0)
	policy := reviewDecisionPolicyProjectionV1(t, now)
	if err := (ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: policy}).Validate(); err != nil {
		t.Fatalf("initial publish rejected: %v", err)
	}
	previous := policy.Ref
	policy.Ref.Revision = 2
	policy.ProjectionDigest = ""
	policy.Ref.Digest = ""
	digest, err := ports.DigestReviewDecisionPolicyCurrentProjectionV1(policy)
	if err != nil {
		t.Fatal(err)
	}
	policy.Ref.Digest, policy.ProjectionDigest = digest, digest
	if err := (ports.ReviewDecisionPolicyCurrentPublishRequestV1{Previous: &previous, Value: policy}).Validate(); err != nil {
		t.Fatalf("exact +1 publish rejected: %v", err)
	}
	policy.Ref.Revision = 4
	policy.Ref.Digest, policy.ProjectionDigest = "", ""
	digest, err = ports.DigestReviewDecisionPolicyCurrentProjectionV1(policy)
	if err != nil {
		t.Fatal(err)
	}
	policy.Ref.Digest, policy.ProjectionDigest = digest, digest
	if err := (ports.ReviewDecisionPolicyCurrentPublishRequestV1{Previous: &previous, Value: policy}).Validate(); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("revision gap did not conflict: %v", err)
	}
}

func TestReviewDecisionGovernanceCurrentV1StateCurrentTruthTableAndHistorical(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_800_000_000, 0)
	states := []ports.ReviewDecisionGovernanceProjectionStateV1{
		ports.ReviewDecisionGovernanceProjectionRevokedV1,
		ports.ReviewDecisionGovernanceProjectionExpiredV1,
		ports.ReviewDecisionGovernanceProjectionSupersededV1,
	}
	for _, state := range states {
		state := state
		t.Run(string(state), func(t *testing.T) {
			policy := terminalReviewDecisionPolicyProjectionV1(t, now, state)
			if err := policy.Validate(); err != nil {
				t.Fatalf("terminal Policy historical projection rejected: %v", err)
			}
			if err := policy.ValidateCurrent(policy.Ref, policy.Subject, now.Add(time.Second)); err == nil {
				t.Fatal("terminal Policy historical projection became current")
			}
			policy.Current = true
			if err := policy.Validate(); err == nil {
				t.Fatal("terminal Policy with Current=true sealed")
			}

			authority := terminalReviewDecisionAuthorityProjectionV1(t, now, state)
			if err := authority.Validate(); err != nil {
				t.Fatalf("terminal Authority historical projection rejected: %v", err)
			}
			if err := authority.ValidateCurrent(authority.Ref, authority.Subject, now.Add(time.Second)); err == nil {
				t.Fatal("terminal Authority historical projection became current")
			}
			authority.Current = true
			if err := authority.Validate(); err == nil {
				t.Fatal("terminal Authority with Current=true sealed")
			}

			scope := terminalReviewDecisionScopeProjectionV1(t, now, state)
			if err := scope.Validate(); err != nil {
				t.Fatalf("terminal Scope historical projection rejected: %v", err)
			}
			if err := scope.ValidateCurrent(scope.Ref, scope.Subject, now.Add(time.Second)); err == nil {
				t.Fatal("terminal Scope historical projection became current")
			}
			scope.Current = true
			if err := scope.Validate(); err == nil {
				t.Fatal("terminal Scope with Current=true sealed")
			}
		})
	}

	policy := reviewDecisionPolicyProjectionV1(t, now)
	policy.Current = false
	if err := policy.Validate(); err == nil {
		t.Fatal("active Policy with Current=false sealed")
	}
	authority := reviewDecisionAuthorityProjectionV1(t, now)
	authority.Current = false
	if err := authority.Validate(); err == nil {
		t.Fatal("active Authority with Current=false sealed")
	}
	scope := reviewDecisionScopeProjectionV1(t, now)
	scope.Current = false
	if err := scope.Validate(); err == nil {
		t.Fatal("active Scope with Current=false sealed")
	}
}

func reviewDecisionPolicyProjectionV1(t *testing.T, now time.Time) ports.ReviewDecisionPolicyCurrentProjectionV1 {
	t.Helper()
	target, scope := reviewDecisionTargetAndScopeV1()
	policyRef := ports.ReviewPolicyBindingRefV2{Ref: "policy-1", Revision: 3, Digest: reviewDecisionDigestV1("policy-placeholder")}
	fact := ports.ReviewPolicyFactV2{Ref: policyRef.Ref, Revision: policyRef.Revision, SubjectDigest: target.Digest, Scope: scope, RunID: target.RunID, CurrentScope: ports.ExecutionScopeBindingRefV2{Ref: "scope-1", Revision: 4, Digest: reviewDecisionDigestV1("scope")}, RiskClass: "praxis/high", ActorAuthorityRef: "authority-actor", ReviewerAuthorityRef: "authority-reviewer", PolicyDecisionRef: "decision-1", Active: true, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	factDigest, err := fact.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	fact.Digest, policyRef.Digest = factDigest, factDigest
	subject := ports.ReviewDecisionPolicyCurrentSubjectV1{Target: target, Policy: policyRef}
	id, err := ports.DeriveReviewDecisionPolicyCurrentProjectionIDV1(subject, fact.Ref)
	if err != nil {
		t.Fatal(err)
	}
	p := ports.ReviewDecisionPolicyCurrentProjectionV1{ContractVersion: ports.ReviewDecisionGovernanceCurrentContractVersionV1, Ref: ports.ReviewDecisionPolicyCurrentProjectionRefV1{ID: id, Revision: 1}, Subject: subject, Fact: fact, State: ports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()}
	digest, err := ports.DigestReviewDecisionPolicyCurrentProjectionV1(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	return p
}

func reviewDecisionAuthorityProjectionV1(t *testing.T, now time.Time) ports.ReviewDecisionAuthorityCurrentProjectionV1 {
	t.Helper()
	target, scope := reviewDecisionTargetAndScopeV1()
	action := reviewDecisionDigestV1("action")
	binding := ports.AuthorityBindingRefV2{Ref: "authority-reviewer", Revision: 5, Digest: reviewDecisionDigestV1("authority"), Epoch: scope.AuthorityEpoch}
	subject := ports.ReviewDecisionAuthorityCurrentSubjectV1{Role: ports.ReviewDecisionAuthorityReviewerV1, Target: target, Assignment: ports.ReviewDecisionAssignmentRefV1{TenantID: target.TenantID, ID: "assignment-1", Revision: 2, Digest: reviewDecisionDigestV1("assignment"), ReviewerID: "reviewer-1"}, Authority: binding, ActionScopeDigest: action}
	fact := ports.DispatchAuthorityFactV2{Ref: binding.Ref, Digest: binding.Digest, Revision: binding.Revision, Scope: scope, ActionScopeDigest: action, State: ports.AuthorityFactActive, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	id, err := ports.DeriveReviewDecisionAuthorityCurrentProjectionIDV1(subject, fact.Ref)
	if err != nil {
		t.Fatal(err)
	}
	p := ports.ReviewDecisionAuthorityCurrentProjectionV1{ContractVersion: ports.ReviewDecisionGovernanceCurrentContractVersionV1, Ref: ports.ReviewDecisionAuthorityCurrentProjectionRefV1{ID: id, Revision: 1}, Subject: subject, Fact: fact, State: ports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()}
	digest, err := ports.DigestReviewDecisionAuthorityCurrentProjectionV1(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	return p
}

func reviewDecisionScopeProjectionV1(t *testing.T, now time.Time) ports.ReviewDecisionScopeCurrentProjectionV1 {
	t.Helper()
	target, scope := reviewDecisionTargetAndScopeV1()
	fact := ports.ExecutionScopeCurrentFactV2{Ref: "scope-1", Revision: 4, Scope: scope, CapabilityGrantDigest: reviewDecisionDigestV1("grant"), ActivationSource: reviewDecisionGovernanceSourceV1("activation"), InstanceSource: reviewDecisionGovernanceSourceV1("instance"), AuthoritySource: reviewDecisionGovernanceSourceV1("authority"), BindingSource: reviewDecisionGovernanceSourceV1("binding"), RunSource: reviewDecisionGovernanceSourceV1("run"), ActiveRunID: target.RunID, RunState: "running", ProjectionWatermark: 9, State: ports.ExecutionScopeFactActive, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	factDigest, err := fact.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	fact.Digest = factDigest
	current, err := fact.BindingRefV2()
	if err != nil {
		t.Fatal(err)
	}
	subject := ports.ReviewDecisionScopeCurrentSubjectV1{TenantID: target.TenantID, Target: target, RunID: target.RunID, Scope: scope, CurrentScope: current, ActionScopeDigest: reviewDecisionDigestV1("action")}
	id, err := ports.DeriveReviewDecisionScopeCurrentProjectionIDV1(subject, fact.Ref)
	if err != nil {
		t.Fatal(err)
	}
	p := ports.ReviewDecisionScopeCurrentProjectionV1{ContractVersion: ports.ReviewDecisionGovernanceCurrentContractVersionV1, Ref: ports.ReviewDecisionScopeCurrentProjectionRefV1{ID: id, Revision: 1}, Subject: subject, Fact: fact, State: ports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()}
	digest, err := ports.DigestReviewDecisionScopeCurrentProjectionV1(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	return p
}

func terminalReviewDecisionPolicyProjectionV1(t *testing.T, now time.Time, state ports.ReviewDecisionGovernanceProjectionStateV1) ports.ReviewDecisionPolicyCurrentProjectionV1 {
	t.Helper()
	p := reviewDecisionPolicyProjectionV1(t, now)
	p.State, p.Current, p.Fact.Active = state, false, false
	factDigest, err := p.Fact.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	p.Fact.Digest, p.Subject.Policy.Digest = factDigest, factDigest
	p.Ref.Digest, p.ProjectionDigest = "", ""
	digest, err := ports.DigestReviewDecisionPolicyCurrentProjectionV1(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p
}

func terminalReviewDecisionAuthorityProjectionV1(t *testing.T, now time.Time, state ports.ReviewDecisionGovernanceProjectionStateV1) ports.ReviewDecisionAuthorityCurrentProjectionV1 {
	t.Helper()
	p := reviewDecisionAuthorityProjectionV1(t, now)
	p.State, p.Current = state, false
	if state == ports.ReviewDecisionGovernanceProjectionExpiredV1 {
		p.Fact.State = ports.AuthorityFactExpired
	} else {
		p.Fact.State = ports.AuthorityFactRevoked
	}
	p.Ref.Digest, p.ProjectionDigest = "", ""
	digest, err := ports.DigestReviewDecisionAuthorityCurrentProjectionV1(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p
}

func terminalReviewDecisionScopeProjectionV1(t *testing.T, now time.Time, state ports.ReviewDecisionGovernanceProjectionStateV1) ports.ReviewDecisionScopeCurrentProjectionV1 {
	t.Helper()
	p := reviewDecisionScopeProjectionV1(t, now)
	p.State, p.Current = state, false
	if state == ports.ReviewDecisionGovernanceProjectionExpiredV1 {
		p.Fact.State = ports.ExecutionScopeFactExpired
	} else {
		p.Fact.State = ports.ExecutionScopeFactRevoked
	}
	factDigest, err := p.Fact.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	p.Fact.Digest = factDigest
	current, err := p.Fact.BindingRefV2()
	if err != nil {
		t.Fatal(err)
	}
	p.Subject.CurrentScope = current
	p.Ref.Digest, p.ProjectionDigest = "", ""
	digest, err := ports.DigestReviewDecisionScopeCurrentProjectionV1(p)
	if err != nil {
		t.Fatal(err)
	}
	p.Ref.Digest, p.ProjectionDigest = digest, digest
	return p
}

func reviewDecisionTargetAndScopeV1() (ports.ReviewDecisionTargetRefV1, core.ExecutionScope) {
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 2}, Lineage: core.LineageRef{ID: "lineage-1", PlanDigest: reviewDecisionDigestV1("plan")}, Instance: core.InstanceRef{ID: "instance-1", Epoch: 3}, AuthorityEpoch: 4}
	return ports.ReviewDecisionTargetRefV1{TenantID: "tenant-1", ID: "target-1", Revision: 7, Digest: reviewDecisionDigestV1("target"), RunID: "run-1"}, scope
}

func reviewDecisionGovernanceSourceV1(ref string) ports.GovernanceSourceFactRefV2 {
	return ports.GovernanceSourceFactRefV2{Ref: ref, Revision: 1, Digest: reviewDecisionDigestV1(ref)}
}

func reviewDecisionDigestV1(value string) core.Digest { return core.DigestBytes([]byte(value)) }
