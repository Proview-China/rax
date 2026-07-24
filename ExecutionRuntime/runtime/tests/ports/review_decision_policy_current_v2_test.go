package ports_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewDecisionPolicyCurrentV2SealNonCircularAndCurrentness(t *testing.T) {
	now := time.Unix(2_700_000_000, 0)
	p := reviewDecisionPolicyProjectionV2(t, now, 1)
	var bypassCompatible ports.ReviewDecisionPolicyCurrentProjectionRefV1 = p.Ref
	if bypassCompatible != p.Ref {
		t.Fatal("V2 exact ref no longer aliases the existing Bypass-compatible V1 shape")
	}
	if err := p.ValidateCurrent(p.Ref, p.Subject, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	separate := p.Subject
	lease := *p.Subject.Scope.SandboxLease
	separate.Scope.SandboxLease = &lease
	if !ports.SameReviewDecisionPolicyApplicabilitySubjectV2(p.Subject, separate) {
		t.Fatal("semantically equal SandboxLease pointers compared by allocation identity")
	}
	cloned := p.Clone()
	cloned.Subject.Scope.SandboxLease.Epoch++
	if cloned.Subject.Scope.SandboxLease.Epoch == p.Subject.Scope.SandboxLease.Epoch {
		t.Fatal("projection Clone leaked mutable SandboxLease alias")
	}
	raw, err := json.Marshal(p.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "target_digest") {
		t.Fatalf("non-circular subject leaked target digest: %s", raw)
	}
	nextSubject := p.Subject
	nextSubject.Policy.Revision++
	nextSubject.Policy.Digest = policyDigestV2(t, "policy-next")
	if id, err := ports.DeriveReviewDecisionPolicyCurrentProjectionIDV2(nextSubject); err != nil || id != p.Ref.ID {
		t.Fatalf("policy revision changed stable applicability ID: %q %v", id, err)
	}
	driftSubject := nextSubject
	driftSubject.PayloadRevision++
	if id, err := ports.DeriveReviewDecisionPolicyCurrentProjectionIDV2(driftSubject); err != nil || id == p.Ref.ID {
		t.Fatalf("exact applicability drift reused stable ID: %q %v", id, err)
	}
	if err := p.ValidateCurrent(p.Ref, p.Subject, time.Unix(0, p.ExpiresUnixNano)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("TTL boundary accepted: %v", err)
	}
}

func TestReviewDecisionPolicyCurrentV2HardNegatives(t *testing.T) {
	p := reviewDecisionPolicyProjectionV2(t, time.Unix(2_700_000_000, 0), 1)
	cases := map[string]func(*ports.ReviewDecisionPolicyCurrentProjectionV2){
		"subject digest": func(v *ports.ReviewDecisionPolicyCurrentProjectionV2) {
			v.Subject.IntentSubjectDigest = policyDigestV2(t, "drift")
		},
		"payload digest": func(v *ports.ReviewDecisionPolicyCurrentProjectionV2) { v.Subject.PayloadDigest = "" },
		"policy fact subject": func(v *ports.ReviewDecisionPolicyCurrentProjectionV2) {
			v.Fact.SubjectDigest = policyDigestV2(t, "other")
		},
		"actor authority":    func(v *ports.ReviewDecisionPolicyCurrentProjectionV2) { v.Subject.ActorAuthority.Ref = "other" },
		"scope":              func(v *ports.ReviewDecisionPolicyCurrentProjectionV2) { v.Subject.Scope.Instance.Epoch++ },
		"truth table":        func(v *ports.ReviewDecisionPolicyCurrentProjectionV2) { v.Current = false },
		"ttl exceeds source": func(v *ports.ReviewDecisionPolicyCurrentProjectionV2) { v.ExpiresUnixNano = v.Fact.ExpiresUnixNano + 1 },
		"digest":             func(v *ports.ReviewDecisionPolicyCurrentProjectionV2) { v.ProjectionDigest = policyDigestV2(t, "bad") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := p.Clone()
			mutate(&changed)
			if changed.Validate() == nil {
				t.Fatal("invalid projection accepted")
			}
		})
	}
}

func TestReviewDecisionPolicyCurrentV2PublishShapeAndTerminal(t *testing.T) {
	now := time.Unix(2_700_000_000, 0)
	first := reviewDecisionPolicyProjectionV2(t, now, 1)
	if err := (ports.ReviewDecisionPolicyCurrentPublishRequestV2{Value: first}).Validate(); err != nil {
		t.Fatal(err)
	}
	next := first
	next.Ref.Revision = 2
	next.Subject.Policy.Revision++
	next.Fact = reviewPolicyFactV2(t, next.Subject, now.Add(time.Second), false)
	next.Subject.Policy.Digest = next.Fact.Digest
	next.State = ports.ReviewDecisionGovernanceProjectionRevokedV1
	next.Current = false
	next.CheckedUnixNano = now.Add(time.Second).UnixNano()
	next.ExpiresUnixNano = now.Add(20 * time.Second).UnixNano()
	var err error
	next, err = ports.SealReviewDecisionPolicyCurrentProjectionV2(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := (ports.ReviewDecisionPolicyCurrentPublishRequestV2{Previous: &first.Ref, Value: next}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := next.ValidateCurrent(next.Ref, next.Subject, now.Add(2*time.Second)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("terminal projection accepted: %v", err)
	}
	gap := next
	gap.Ref.Revision = 4
	gap, err = ports.SealReviewDecisionPolicyCurrentProjectionV2(gap)
	if err != nil {
		t.Fatal(err)
	}
	if err := (ports.ReviewDecisionPolicyCurrentPublishRequestV2{Previous: &next.Ref, Value: gap}).Validate(); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("revision gap accepted: %v", err)
	}
}

func reviewDecisionPolicyProjectionV2(t *testing.T, now time.Time, revision core.Revision) ports.ReviewDecisionPolicyCurrentProjectionV2 {
	t.Helper()
	subject := reviewDecisionPolicySubjectV2(t)
	fact := reviewPolicyFactV2(t, subject, now, true)
	subject.Policy.Digest = fact.Digest
	p, err := ports.SealReviewDecisionPolicyCurrentProjectionV2(ports.ReviewDecisionPolicyCurrentProjectionV2{Ref: ports.ReviewDecisionPolicyCurrentProjectionRefV2{Revision: revision}, Subject: subject, Fact: fact, State: ports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func reviewDecisionPolicySubjectV2(t *testing.T) ports.ReviewDecisionPolicyApplicabilitySubjectV2 {
	t.Helper()
	tenant := core.TenantID("tenant-policy-v2")
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: tenant, ID: "agent", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: policyDigestV2(t, "plan")}, Instance: core.InstanceRef{ID: "instance", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox", Epoch: 1}, AuthorityEpoch: 1}
	return ports.ReviewDecisionPolicyApplicabilitySubjectV2{TenantID: tenant, TargetID: "target", TargetRevision: 3, IntentID: "intent", IntentRevision: 2, IntentSubjectDigest: policyDigestV2(t, "subject"), PayloadRevision: 4, PayloadDigest: policyDigestV2(t, "payload"), RunID: "run", Scope: scope, CurrentScope: ports.ExecutionScopeBindingRefV2{Ref: "scope", Revision: 1, Digest: policyDigestV2(t, "scope")}, ActionScopeDigest: policyDigestV2(t, "action"), ActorAuthority: ports.AuthorityBindingRefV2{Ref: "actor", Revision: 1, Digest: policyDigestV2(t, "actor"), Epoch: 1}, Policy: ports.ReviewPolicyBindingRefV2{Ref: "policy", Revision: 1, Digest: policyDigestV2(t, "policy")}}
}
func reviewPolicyFactV2(t *testing.T, s ports.ReviewDecisionPolicyApplicabilitySubjectV2, now time.Time, active bool) ports.ReviewPolicyFactV2 {
	t.Helper()
	f := ports.ReviewPolicyFactV2{Ref: s.Policy.Ref, Revision: s.Policy.Revision, SubjectDigest: s.IntentSubjectDigest, Scope: s.Scope, RunID: s.RunID, CurrentScope: s.CurrentScope, RiskClass: "review.test/controlled", ActorAuthorityRef: s.ActorAuthority.Ref, ReviewerAuthorityRef: "reviewer", PolicyDecisionRef: "decision", Active: active, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	d, err := f.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	f.Digest = d
	return f
}
func policyDigestV2(t *testing.T, value string) core.Digest {
	t.Helper()
	d, err := core.CanonicalJSONDigest("review-policy-v2-test", "v1", "value", value)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
