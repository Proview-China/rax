package fakes_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewDecisionPolicyCurrentStoreV2CASHistoryAndLostReply(t *testing.T) {
	now := time.Unix(2_740_000_000, 0)
	first := fakePolicyProjectionV2(t, now, 1)
	store := fakes.NewReviewDecisionPolicyCurrentStoreV2()
	receipt, err := store.CommitPolicyV2(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV2{Value: first})
	if err != nil || !receipt.Created {
		t.Fatalf("create failed: %+v %v", receipt, err)
	}
	replay, err := store.CommitPolicyV2(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV2{Value: first})
	if err != nil || replay.Created {
		t.Fatalf("canonical replay failed: %+v %v", replay, err)
	}
	next := fakeNextPolicyProjectionV2(t, first, now.Add(time.Second))
	if _, err := store.CommitPolicyV2(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV2{Previous: &first.Ref, Value: next}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectCurrentPolicyV2(context.Background(), first.Subject, first.Ref); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale current accepted: %v", err)
	}
	if _, err := store.ResolvePolicyV2(context.Background(), first.Subject); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale exact subject resolved: %v", err)
	}
	if _, err := store.ResolvePolicyV2(context.Background(), first.Subject); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale exact subject resolved: %v", err)
	}
	if old, err := store.InspectHistoricalPolicyV2(context.Background(), first.Ref); err != nil || !reflect.DeepEqual(old, first) {
		t.Fatalf("history rewritten: %+v %v", old, err)
	}
	aliased, err := store.InspectHistoricalPolicyV2(context.Background(), first.Ref)
	if err != nil {
		t.Fatal(err)
	}
	aliased.Subject.Scope.SandboxLease.Epoch++
	clean, err := store.InspectHistoricalPolicyV2(context.Background(), first.Ref)
	if err != nil || !reflect.DeepEqual(clean, first) {
		t.Fatalf("fake leaked mutable projection alias: %+v %v", clean, err)
	}
	store.SetAfterCommitHookV2(func() error { return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost") })
	third := fakeNextPolicyProjectionV2(t, next, now.Add(2*time.Second))
	if _, err := store.CommitPolicyV2(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV2{Previous: &next.Ref, Value: third}); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost reply not surfaced: %v", err)
	}
	if got, err := store.InspectHistoricalPolicyV2(context.Background(), third.Ref); err != nil || !reflect.DeepEqual(got, third) {
		t.Fatalf("lost reply exact recovery impossible: %+v %v", got, err)
	}
}

func fakePolicyProjectionV2(t *testing.T, now time.Time, revision core.Revision) ports.ReviewDecisionPolicyCurrentProjectionV2 {
	t.Helper()
	tenant := core.TenantID("tenant-fake-v2")
	d := func(v string) core.Digest {
		x, e := core.CanonicalJSONDigest("fake-policy-v2", "v1", "value", v)
		if e != nil {
			t.Fatal(e)
		}
		return x
	}
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: tenant, ID: "agent", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: d("plan")}, Instance: core.InstanceRef{ID: "instance", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox", Epoch: 1}, AuthorityEpoch: 1}
	s := ports.ReviewDecisionPolicyApplicabilitySubjectV2{TenantID: tenant, TargetID: "target", TargetRevision: 1, IntentID: "intent", IntentRevision: 1, IntentSubjectDigest: d("subject"), PayloadRevision: 1, PayloadDigest: d("payload"), RunID: "run", Scope: scope, CurrentScope: ports.ExecutionScopeBindingRefV2{Ref: "scope", Revision: 1, Digest: d("scope")}, ActionScopeDigest: d("action"), ActorAuthority: ports.AuthorityBindingRefV2{Ref: "actor", Revision: 1, Digest: d("actor"), Epoch: 1}, Policy: ports.ReviewPolicyBindingRefV2{Ref: "policy", Revision: revision, Digest: d("placeholder")}}
	f := ports.ReviewPolicyFactV2{Ref: "policy", Revision: revision, SubjectDigest: s.IntentSubjectDigest, Scope: scope, RunID: "run", CurrentScope: s.CurrentScope, RiskClass: "review.test/controlled", ActorAuthorityRef: "actor", ReviewerAuthorityRef: "reviewer", PolicyDecisionRef: "decision", Active: true, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	fd, e := f.DigestV2()
	if e != nil {
		t.Fatal(e)
	}
	f.Digest = fd
	s.Policy.Digest = fd
	p, e := ports.SealReviewDecisionPolicyCurrentProjectionV2(ports.ReviewDecisionPolicyCurrentProjectionV2{Ref: ports.ReviewDecisionPolicyCurrentProjectionRefV2{Revision: revision}, Subject: s, Fact: f, State: ports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func fakeNextPolicyProjectionV2(t *testing.T, p ports.ReviewDecisionPolicyCurrentProjectionV2, now time.Time) ports.ReviewDecisionPolicyCurrentProjectionV2 {
	t.Helper()
	p.Ref.Revision++
	p.Subject.Policy.Revision++
	p.Fact.Revision++
	p.Fact.ExpiresUnixNano = now.Add(time.Minute).UnixNano()
	p.Fact.Digest = ""
	fd, e := p.Fact.DigestV2()
	if e != nil {
		t.Fatal(e)
	}
	p.Fact.Digest = fd
	p.Subject.Policy.Digest = fd
	p.CheckedUnixNano = now.UnixNano()
	p.ExpiresUnixNano = now.Add(30 * time.Second).UnixNano()
	p, e = ports.SealReviewDecisionPolicyCurrentProjectionV2(p)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
