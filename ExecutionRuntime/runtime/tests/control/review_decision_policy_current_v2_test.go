package control_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type policySourceV2 struct {
	mu     sync.Mutex
	values []ports.ReviewPolicyFactV2
	calls  int
}

func (s *policySourceV2) InspectReviewPolicy(ctx context.Context, _ string) (ports.ReviewPolicyFactV2, error) {
	if err := ctx.Err(); err != nil {
		return ports.ReviewPolicyFactV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "policy source context ended")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.values) == 0 {
		return ports.ReviewPolicyFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "policy missing")
	}
	i := s.calls
	if i >= len(s.values) {
		i = len(s.values) - 1
	}
	s.calls++
	return s.values[i], nil
}

func TestReviewDecisionPolicyCurrentGatewayV2ExactS1S2AndLostReply(t *testing.T) {
	now := time.Unix(2_710_000_000, 0)
	projection := controlPolicyProjectionV2(t, now)
	source := &policySourceV2{values: []ports.ReviewPolicyFactV2{projection.Fact}}
	store := fakes.NewReviewDecisionPolicyCurrentStoreV2()
	clockNow := now
	gateway, err := control.NewReviewDecisionPolicyCurrentGatewayV2(store, source, func() time.Time { return clockNow })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	store.SetAfterCommitHookV2(func() error {
		cancel()
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost reply")
	})
	receipt, err := gateway.PublishReviewDecisionPolicyCurrentV2(ctx, ports.ReviewDecisionPolicyCurrentPublishRequestV2{Value: projection})
	if err != nil || receipt.Created || receipt.Ref != projection.Ref {
		t.Fatalf("lost reply exact recovery failed: %+v %v", receipt, err)
	}
	store.SetAfterCommitHookV2(nil)
	ref, err := gateway.ResolveCurrentReviewDecisionPolicyV2(context.Background(), ports.ReviewDecisionPolicyCurrentResolveRequestV2{Subject: projection.Subject})
	if err != nil || ref != projection.Ref {
		t.Fatalf("resolve failed: %+v %v", ref, err)
	}
	clockNow = now.Add(time.Second)
	got, err := gateway.InspectCurrentReviewDecisionPolicyV2(context.Background(), projection.Subject, projection.Ref)
	if err != nil || got != projection {
		t.Fatalf("current S1/S2 failed: %+v %v", got, err)
	}
}

func TestReviewDecisionPolicyCurrentGatewayV2DriftRollbackTTLAndTypedNil(t *testing.T) {
	now := time.Unix(2_710_000_000, 0)
	projection := controlPolicyProjectionV2(t, now)
	store := fakes.NewReviewDecisionPolicyCurrentStoreV2()
	if _, err := store.CommitPolicyV2(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV2{Value: projection}); err != nil {
		t.Fatal(err)
	}
	drift := projection.Fact
	drift.PolicyDecisionRef = "other"
	d, _ := drift.DigestV2()
	drift.Digest = d
	source := &policySourceV2{values: []ports.ReviewPolicyFactV2{projection.Fact, drift}}
	times := []time.Time{now, now.Add(time.Second), now.Add(2 * time.Second)}
	var mu sync.Mutex
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		v := times[0]
		if len(times) > 1 {
			times = times[1:]
		}
		return v
	}
	gateway, err := control.NewReviewDecisionPolicyCurrentGatewayV2(store, source, clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.InspectCurrentReviewDecisionPolicyV2(context.Background(), projection.Subject, projection.Ref); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("S1/S2 drift accepted: %v", err)
	}
	source = &policySourceV2{values: []ports.ReviewPolicyFactV2{projection.Fact}}
	rollback := []time.Time{now, now.Add(5 * time.Second), now.Add(4 * time.Second)}
	gateway, _ = control.NewReviewDecisionPolicyCurrentGatewayV2(store, source, func() time.Time {
		v := rollback[0]
		if len(rollback) > 1 {
			rollback = rollback[1:]
		}
		return v
	})
	if _, err := gateway.InspectCurrentReviewDecisionPolicyV2(context.Background(), projection.Subject, projection.Ref); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback accepted: %v", err)
	}
	gateway, _ = control.NewReviewDecisionPolicyCurrentGatewayV2(store, source, func() time.Time { return time.Unix(0, projection.ExpiresUnixNano) })
	if _, err := gateway.InspectCurrentReviewDecisionPolicyV2(context.Background(), projection.Subject, projection.Ref); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("TTL boundary accepted: %v", err)
	}
	resolveTimes := []time.Time{now, now.Add(5 * time.Second), now.Add(4 * time.Second)}
	resolveGateway, err := control.NewReviewDecisionPolicyCurrentGatewayV2(store, &policySourceV2{values: []ports.ReviewPolicyFactV2{projection.Fact}}, func() time.Time {
		value := resolveTimes[0]
		if len(resolveTimes) > 1 {
			resolveTimes = resolveTimes[1:]
		}
		return value
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolveGateway.ResolveCurrentReviewDecisionPolicyV2(context.Background(), ports.ReviewDecisionPolicyCurrentResolveRequestV2{Subject: projection.Subject}); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("Resolve clock rollback accepted: %v", err)
	}
	var typedNil *policySourceV2
	if _, err := control.NewReviewDecisionPolicyCurrentGatewayV2(store, typedNil, time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed nil accepted: %v", err)
	}

	lostStore := fakes.NewReviewDecisionPolicyCurrentStoreV2()
	lostStore.SetAfterCommitHookV2(func() error { return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost") })
	lostTimes := []time.Time{now, now, now.Add(time.Second), time.Unix(0, projection.ExpiresUnixNano)}
	lostGateway, err := control.NewReviewDecisionPolicyCurrentGatewayV2(lostStore, &policySourceV2{values: []ports.ReviewPolicyFactV2{projection.Fact}}, func() time.Time {
		value := lostTimes[0]
		if len(lostTimes) > 1 {
			lostTimes = lostTimes[1:]
		}
		return value
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lostGateway.PublishReviewDecisionPolicyCurrentV2(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV2{Value: projection}); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("lost-reply recovery ignored TTL crossing: %v", err)
	}
	if stored, err := lostStore.InspectHistoricalPolicyV2(context.Background(), projection.Ref); err != nil || stored != projection {
		t.Fatalf("TTL-crossed lost reply did not preserve exact history: %+v %v", stored, err)
	}
}

func controlPolicyProjectionV2(t *testing.T, now time.Time) ports.ReviewDecisionPolicyCurrentProjectionV2 {
	t.Helper()
	tenant := core.TenantID("tenant-control-v2")
	digest := func(v string) core.Digest {
		d, e := core.CanonicalJSONDigest("control-policy-v2", "v1", "value", v)
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: tenant, ID: "agent", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: digest("plan")}, Instance: core.InstanceRef{ID: "instance", Epoch: 1}, AuthorityEpoch: 1}
	subject := ports.ReviewDecisionPolicyApplicabilitySubjectV2{TenantID: tenant, TargetID: "target", TargetRevision: 1, IntentID: "intent", IntentRevision: 1, IntentSubjectDigest: digest("subject"), PayloadRevision: 1, PayloadDigest: digest("payload"), RunID: "run", Scope: scope, CurrentScope: ports.ExecutionScopeBindingRefV2{Ref: "scope", Revision: 1, Digest: digest("scope")}, ActionScopeDigest: digest("action"), ActorAuthority: ports.AuthorityBindingRefV2{Ref: "actor", Revision: 1, Digest: digest("actor"), Epoch: 1}, Policy: ports.ReviewPolicyBindingRefV2{Ref: "policy", Revision: 1, Digest: digest("placeholder")}}
	fact := ports.ReviewPolicyFactV2{Ref: "policy", Revision: 1, SubjectDigest: subject.IntentSubjectDigest, Scope: scope, RunID: "run", CurrentScope: subject.CurrentScope, RiskClass: "review.test/controlled", ActorAuthorityRef: "actor", ReviewerAuthorityRef: "reviewer", PolicyDecisionRef: "decision", Active: true, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	factDigest, e := fact.DigestV2()
	if e != nil {
		t.Fatal(e)
	}
	fact.Digest = factDigest
	subject.Policy.Digest = factDigest
	p, e := ports.SealReviewDecisionPolicyCurrentProjectionV2(ports.ReviewDecisionPolicyCurrentProjectionV2{Ref: ports.ReviewDecisionPolicyCurrentProjectionRefV2{Revision: 1}, Subject: subject, Fact: fact, State: ports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
