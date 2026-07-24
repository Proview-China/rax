package fakes_test

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"reflect"
	"testing"
	"time"
)

func TestDispatchAuthorityCurrentStoreV3CASHistoryCloneAndLostReply(t *testing.T) {
	now := time.Unix(2_820_000_000, 0)
	first := fakeDispatchAuthorityV3(t, now)
	store := fakes.NewDispatchAuthorityCurrentStoreV3()
	receipt, err := store.CommitAuthorityFactV3(context.Background(), ports.DispatchAuthorityFactPublishRequestV3{Value: first})
	if err != nil || !receipt.Created {
		t.Fatalf("create failed: %+v %v", receipt, err)
	}
	replay, err := store.CommitAuthorityFactV3(context.Background(), ports.DispatchAuthorityFactPublishRequestV3{Value: first})
	if err != nil || replay.Created {
		t.Fatalf("replay failed: %+v %v", replay, err)
	}
	next := fakeNextDispatchAuthorityV3(t, first, now.Add(time.Second), true)
	if _, err := store.CommitAuthorityFactV3(context.Background(), ports.DispatchAuthorityFactPublishRequestV3{Previous: &first.Ref, Value: next}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectCurrentAuthorityFactV3(context.Background(), first.Ref); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale full Ref accepted: %v", err)
	}
	old, err := store.InspectHistoricalAuthorityFactV3(context.Background(), first.Ref)
	if err != nil || !reflect.DeepEqual(old, first) {
		t.Fatalf("history rewritten: %+v %v", old, err)
	}
	old.Scope.SandboxLease.Epoch++
	clean, _ := store.InspectHistoricalAuthorityFactV3(context.Background(), first.Ref)
	if !reflect.DeepEqual(clean, first) {
		t.Fatal("V3 fake leaked mutable alias")
	}
	store.SetAfterCommitHookV3(func() error { return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost") })
	third := fakeNextDispatchAuthorityV3(t, next, now.Add(2*time.Second), true)
	if _, err := store.CommitAuthorityFactV3(context.Background(), ports.DispatchAuthorityFactPublishRequestV3{Previous: &next.Ref, Value: third}); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost reply not surfaced: %v", err)
	}
	if got, err := store.InspectHistoricalAuthorityFactV3(context.Background(), third.Ref); err != nil || !reflect.DeepEqual(got, third) {
		t.Fatalf("lost reply exact Inspect failed: %+v %v", got, err)
	}
}
func TestReviewActorAuthorityStoreV2CASHistoryAndNoRunDrift(t *testing.T) {
	now := time.Unix(2_820_000_000, 0)
	first := fakeActorProjectionV2(t, now)
	store := fakes.NewReviewActorAuthorityCurrentStoreV2()
	if _, err := store.CommitActorAuthorityV2(context.Background(), ports.ReviewActorAuthorityCurrentPublishRequestV2{Value: first}); err != nil {
		t.Fatal(err)
	}
	next := fakeNextActorProjectionV2(t, first, now.Add(time.Second), true)
	if _, err := store.CommitActorAuthorityV2(context.Background(), ports.ReviewActorAuthorityCurrentPublishRequestV2{Previous: &first.Ref, Value: next}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveActorAuthorityV2(context.Background(), first.Subject); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale subject resolved: %v", err)
	}
	if old, err := store.InspectHistoricalActorAuthorityV2(context.Background(), first.Ref); err != nil || !reflect.DeepEqual(old, first) {
		t.Fatalf("actor history lost: %+v %v", old, err)
	}
	drift := next.Clone()
	drift.Ref.Revision++
	drift.Ref.ID = ""
	drift.Subject.Target.RunID = "other"
	drift.Fact.RunID = "other"
	drift.Fact.Ref.Revision++
	drift.Fact.Ref.Epoch++
	drift.Fact.Scope.AuthorityEpoch = drift.Fact.Ref.Epoch
	var err error
	drift.Fact, err = ports.SealDispatchAuthorityFactV3(drift.Fact)
	if err != nil {
		t.Fatal(err)
	}
	drift.Subject.ActorAuthority = drift.Fact.Ref
	drift, err = ports.SealReviewActorAuthorityCurrentProjectionV2(drift)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitActorAuthorityV2(context.Background(), ports.ReviewActorAuthorityCurrentPublishRequestV2{Previous: &next.Ref, Value: drift}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("cross-run stable history accepted: %v", err)
	}
}
func fakeDispatchAuthorityV3(t *testing.T, now time.Time) ports.DispatchAuthorityFactV3 {
	t.Helper()
	d := func(v string) core.Digest {
		x, e := core.CanonicalJSONDigest("fake-authority-v3", "v1", "value", v)
		if e != nil {
			t.Fatal(e)
		}
		return x
	}
	tenant := core.TenantID("tenant-fake-actor")
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: tenant, ID: "agent", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: d("plan")}, Instance: core.InstanceRef{ID: "instance", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox", Epoch: 1}, AuthorityEpoch: 1}
	f, e := ports.SealDispatchAuthorityFactV3(ports.DispatchAuthorityFactV3{Ref: ports.AuthorityBindingRefV2{Ref: "authority", Revision: 1, Epoch: 1}, Scope: scope, RunID: "run", ActionScopeDigest: d("action"), State: ports.AuthorityFactActive, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()})
	if e != nil {
		t.Fatal(e)
	}
	return f
}
func fakeNextDispatchAuthorityV3(t *testing.T, f ports.DispatchAuthorityFactV3, now time.Time, active bool) ports.DispatchAuthorityFactV3 {
	t.Helper()
	f = f.Clone()
	f.Ref.Revision++
	f.Ref.Epoch++
	f.Scope.AuthorityEpoch = f.Ref.Epoch
	f.CheckedUnixNano = now.UnixNano()
	f.ExpiresUnixNano = now.Add(30 * time.Second).UnixNano()
	f.State = ports.AuthorityFactActive
	if !active {
		f.State = ports.AuthorityFactRevoked
	}
	f, e := ports.SealDispatchAuthorityFactV3(f)
	if e != nil {
		t.Fatal(e)
	}
	return f
}
func fakeActorProjectionV2(t *testing.T, now time.Time) ports.ReviewActorAuthorityCurrentProjectionV2 {
	t.Helper()
	f := fakeDispatchAuthorityV3(t, now)
	d, _ := core.CanonicalJSONDigest("fake-actor-v2", "v1", "value", "target")
	s := ports.ReviewActorAuthorityCurrentSubjectV2{Target: ports.ReviewDecisionTargetRefV1{TenantID: f.Scope.Identity.TenantID, ID: "target", Revision: 1, Digest: d, RunID: f.RunID}, ActorAuthority: f.Ref, ActionScopeDigest: f.ActionScopeDigest}
	p, e := ports.SealReviewActorAuthorityCurrentProjectionV2(ports.ReviewActorAuthorityCurrentProjectionV2{Ref: ports.ReviewActorAuthorityCurrentProjectionRefV2{Revision: 1}, Subject: s, Fact: f, State: ports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func fakeNextActorProjectionV2(t *testing.T, p ports.ReviewActorAuthorityCurrentProjectionV2, now time.Time, active bool) ports.ReviewActorAuthorityCurrentProjectionV2 {
	t.Helper()
	p = p.Clone()
	p.Ref.Revision++
	p.Fact = fakeNextDispatchAuthorityV3(t, p.Fact, now, active)
	p.Subject.ActorAuthority = p.Fact.Ref
	p.CheckedUnixNano = now.UnixNano()
	p.ExpiresUnixNano = now.Add(20 * time.Second).UnixNano()
	p.State = ports.ReviewDecisionGovernanceProjectionActiveV1
	p.Current = active
	if !active {
		p.State = ports.ReviewDecisionGovernanceProjectionRevokedV1
	}
	p, e := ports.SealReviewActorAuthorityCurrentProjectionV2(p)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
