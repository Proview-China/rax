package control_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type actorTargetProofV2 struct {
	mu     sync.Mutex
	values []ports.ReviewDecisionTargetRefV1
	calls  int
}

func (s *actorTargetProofV2) InspectReviewDecisionTargetProofV1(ctx context.Context, expected ports.ReviewDecisionTargetRefV1) (ports.ReviewDecisionTargetRefV1, error) {
	if err := ctx.Err(); err != nil {
		return ports.ReviewDecisionTargetRefV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "target context ended")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.values) == 0 {
		return ports.ReviewDecisionTargetRefV1{}, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "target missing")
	}
	i := s.calls
	if i >= len(s.values) {
		i = len(s.values) - 1
	}
	s.calls++
	return s.values[i], nil
}
func (s *actorTargetProofV2) InspectReviewDecisionAssignmentProofV1(context.Context, ports.ReviewDecisionAssignmentRefV1) (ports.ReviewDecisionAssignmentRefV1, error) {
	return ports.ReviewDecisionAssignmentRefV1{}, core.NewError(core.ErrorForbidden, core.ReasonOwnerMissing, "actor path has no Assignment")
}

type actorAuthoritySequenceV3 struct {
	mu     sync.Mutex
	values []ports.DispatchAuthorityFactV3
	calls  int
}

func (s *actorAuthoritySequenceV3) InspectCurrentDispatchAuthorityV3(context.Context, ports.AuthorityBindingRefV2) (ports.DispatchAuthorityFactV3, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.calls
	if i >= len(s.values) {
		i = len(s.values) - 1
	}
	s.calls++
	return s.values[i].Clone(), nil
}
func (s *actorAuthoritySequenceV3) InspectHistoricalDispatchAuthorityV3(context.Context, ports.AuthorityBindingRefV2) (ports.DispatchAuthorityFactV3, error) {
	return s.values[0].Clone(), nil
}

func TestDispatchAuthorityCurrentGatewayV3LostReplyExactAndCurrent(t *testing.T) {
	now := time.Unix(2_810_000_000, 0)
	fact := controlDispatchAuthorityV3(t, now)
	store := fakes.NewDispatchAuthorityCurrentStoreV3()
	clockNow := now
	gateway, err := control.NewDispatchAuthorityCurrentGatewayV3(store, func() time.Time { return clockNow })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	store.SetAfterCommitHookV3(func() error {
		cancel()
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost")
	})
	receipt, err := gateway.PublishDispatchAuthorityFactV3(ctx, ports.DispatchAuthorityFactPublishRequestV3{Value: fact})
	if err != nil || receipt.Created || receipt.Ref != fact.Ref {
		t.Fatalf("V3 lost reply recovery failed: %+v %v", receipt, err)
	}
	store.SetAfterCommitHookV3(nil)
	clockNow = now.Add(time.Second)
	got, err := gateway.InspectCurrentDispatchAuthorityV3(context.Background(), fact.Ref)
	if err != nil || got.Ref != fact.Ref {
		t.Fatalf("V3 exact current failed: %+v %v", got, err)
	}
	expiredGateway, _ := control.NewDispatchAuthorityCurrentGatewayV3(store, func() time.Time { return time.Unix(0, fact.ExpiresUnixNano) })
	if _, err := expiredGateway.InspectCurrentDispatchAuthorityV3(context.Background(), fact.Ref); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("V3 pure time expiry accepted: %v", err)
	}
	if historical, err := store.InspectHistoricalAuthorityFactV3(context.Background(), fact.Ref); err != nil || historical.Ref != fact.Ref {
		t.Fatalf("time expiry mutated history: %+v %v", historical, err)
	}
	canceled, cancelRead := context.WithCancel(context.Background())
	cancelRead()
	if _, err := gateway.InspectCurrentDispatchAuthorityV3(canceled, fact.Ref); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("canceled V3 read error degraded: %v", err)
	}
	lostExpiryStore := fakes.NewDispatchAuthorityCurrentStoreV3()
	lostExpiryTimes := []time.Time{now, now, now.Add(time.Second), time.Unix(0, fact.ExpiresUnixNano)}
	lostExpiryGateway, err := control.NewDispatchAuthorityCurrentGatewayV3(lostExpiryStore, func() time.Time {
		value := lostExpiryTimes[0]
		lostExpiryTimes = lostExpiryTimes[1:]
		return value
	})
	if err != nil {
		t.Fatal(err)
	}
	lostCtx, lostCancel := context.WithCancel(context.Background())
	lostExpiryStore.SetAfterCommitHookV3(func() error {
		lostCancel()
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost across TTL")
	})
	if _, err := lostExpiryGateway.PublishDispatchAuthorityFactV3(lostCtx, ports.DispatchAuthorityFactPublishRequestV3{Value: fact}); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("V3 lost reply TTL crossing accepted: %v", err)
	}
	if historical, err := lostExpiryStore.InspectHistoricalAuthorityFactV3(context.Background(), fact.Ref); err != nil || historical.Ref != fact.Ref {
		t.Fatalf("V3 lost reply TTL crossing lost exact history: %+v %v", historical, err)
	}
	var typedNil *fakes.DispatchAuthorityCurrentStoreV3
	if _, err := control.NewDispatchAuthorityCurrentGatewayV3(typedNil, time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("V3 typed nil accepted: %v", err)
	}
}

func TestReviewActorAuthorityGatewayV2ExactS1S2LostReplyAndNoAssignment(t *testing.T) {
	now := time.Unix(2_810_000_000, 0)
	projection := controlActorProjectionV2(t, now)
	authorityStore := fakes.NewDispatchAuthorityCurrentStoreV3()
	authorityGateway, _ := control.NewDispatchAuthorityCurrentGatewayV3(authorityStore, func() time.Time { return now.Add(time.Second) })
	if _, err := authorityGateway.PublishDispatchAuthorityFactV3(context.Background(), ports.DispatchAuthorityFactPublishRequestV3{Value: projection.Fact}); err != nil {
		t.Fatal(err)
	}
	actorStore := fakes.NewReviewActorAuthorityCurrentStoreV2()
	proof := &actorTargetProofV2{values: []ports.ReviewDecisionTargetRefV1{projection.Subject.Target}}
	gateway, err := control.NewReviewActorAuthorityCurrentGatewayV2(actorStore, proof, authorityGateway, func() time.Time { return now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	actorStore.SetAfterCommitHookV2(func() error {
		cancel()
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost")
	})
	receipt, err := gateway.PublishReviewActorAuthorityCurrentV2(ctx, ports.ReviewActorAuthorityCurrentPublishRequestV2{Value: projection})
	if err != nil || receipt.Created || receipt.Ref != projection.Ref {
		t.Fatalf("actor lost reply recovery failed: %+v %v", receipt, err)
	}
	actorStore.SetAfterCommitHookV2(nil)
	ref, err := gateway.ResolveCurrentReviewActorAuthorityV2(context.Background(), ports.ReviewActorAuthorityCurrentResolveRequestV2{Subject: projection.Subject})
	if err != nil || ref != projection.Ref {
		t.Fatalf("actor Resolve failed: %+v %v", ref, err)
	}
	got, err := gateway.InspectCurrentReviewActorAuthorityV2(context.Background(), projection.Subject, projection.Ref)
	if err != nil || got.Ref != projection.Ref {
		t.Fatalf("actor current failed: %+v %v", got, err)
	}
}

func TestReviewActorAuthorityGatewayV2DriftTTLRollbackAndTypedNil(t *testing.T) {
	now := time.Unix(2_810_000_000, 0)
	projection := controlActorProjectionV2(t, now)
	actorStore := fakes.NewReviewActorAuthorityCurrentStoreV2()
	if _, err := actorStore.CommitActorAuthorityV2(context.Background(), ports.ReviewActorAuthorityCurrentPublishRequestV2{Value: projection}); err != nil {
		t.Fatal(err)
	}
	drift := projection.Fact.Clone()
	drift.RunID = "run-b"
	drift, err := ports.SealDispatchAuthorityFactV3(drift)
	if err != nil {
		t.Fatal(err)
	}
	source := &actorAuthoritySequenceV3{values: []ports.DispatchAuthorityFactV3{projection.Fact, drift}}
	gateway, _ := control.NewReviewActorAuthorityCurrentGatewayV2(actorStore, &actorTargetProofV2{values: []ports.ReviewDecisionTargetRefV1{projection.Subject.Target}}, source, func() time.Time { return now.Add(time.Second) })
	if _, err := gateway.InspectCurrentReviewActorAuthorityV2(context.Background(), projection.Subject, projection.Ref); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("actor S1/S2 run drift accepted: %v", err)
	}
	ttlGateway, _ := control.NewReviewActorAuthorityCurrentGatewayV2(actorStore, &actorTargetProofV2{values: []ports.ReviewDecisionTargetRefV1{projection.Subject.Target}}, &actorAuthoritySequenceV3{values: []ports.DispatchAuthorityFactV3{projection.Fact}}, func() time.Time { return time.Unix(0, projection.ExpiresUnixNano) })
	if _, err := ttlGateway.InspectCurrentReviewActorAuthorityV2(context.Background(), projection.Subject, projection.Ref); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("actor TTL boundary accepted: %v", err)
	}
	if historical, err := actorStore.InspectHistoricalActorAuthorityV2(context.Background(), projection.Ref); err != nil || historical.Ref != projection.Ref {
		t.Fatalf("actor time expiry mutated history: %+v %v", historical, err)
	}
	times := []time.Time{now, now.Add(5 * time.Second), now.Add(4 * time.Second)}
	rollbackGateway, _ := control.NewReviewActorAuthorityCurrentGatewayV2(actorStore, &actorTargetProofV2{values: []ports.ReviewDecisionTargetRefV1{projection.Subject.Target}}, &actorAuthoritySequenceV3{values: []ports.DispatchAuthorityFactV3{projection.Fact}}, func() time.Time {
		v := times[0]
		if len(times) > 1 {
			times = times[1:]
		}
		return v
	})
	if _, err := rollbackGateway.InspectCurrentReviewActorAuthorityV2(context.Background(), projection.Subject, projection.Ref); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("actor clock rollback accepted: %v", err)
	}
	var typedNil *actorTargetProofV2
	if _, err := control.NewReviewActorAuthorityCurrentGatewayV2(actorStore, typedNil, source, time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("actor typed nil accepted: %v", err)
	}
}

func TestReviewActorAuthorityGatewayV2LostReplyTTLCrossingKeepsExactHistory(t *testing.T) {
	now := time.Unix(2_810_000_000, 0)
	projection := controlActorProjectionV2(t, now)
	store := fakes.NewReviewActorAuthorityCurrentStoreV2()
	times := []time.Time{now, now, now.Add(time.Second), time.Unix(0, projection.ExpiresUnixNano)}
	gateway, err := control.NewReviewActorAuthorityCurrentGatewayV2(
		store,
		&actorTargetProofV2{values: []ports.ReviewDecisionTargetRefV1{projection.Subject.Target}},
		&actorAuthoritySequenceV3{values: []ports.DispatchAuthorityFactV3{projection.Fact}},
		func() time.Time {
			value := times[0]
			times = times[1:]
			return value
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	store.SetAfterCommitHookV2(func() error {
		cancel()
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost across TTL")
	})
	if _, err := gateway.PublishReviewActorAuthorityCurrentV2(ctx, ports.ReviewActorAuthorityCurrentPublishRequestV2{Value: projection}); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("actor lost reply TTL crossing accepted: %v", err)
	}
	historical, err := store.InspectHistoricalActorAuthorityV2(context.Background(), projection.Ref)
	if err != nil || !reflect.DeepEqual(historical, projection) {
		t.Fatalf("actor lost reply TTL crossing lost exact history: %+v %v", historical, err)
	}
}

func controlDispatchAuthorityV3(t *testing.T, now time.Time) ports.DispatchAuthorityFactV3 {
	t.Helper()
	d := func(v string) core.Digest {
		x, e := core.CanonicalJSONDigest("control-authority-v3", "v1", "value", v)
		if e != nil {
			t.Fatal(e)
		}
		return x
	}
	tenant := core.TenantID("tenant-control-actor")
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: tenant, ID: "agent", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: d("plan")}, Instance: core.InstanceRef{ID: "instance", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox", Epoch: 1}, AuthorityEpoch: 1}
	f, e := ports.SealDispatchAuthorityFactV3(ports.DispatchAuthorityFactV3{Ref: ports.AuthorityBindingRefV2{Ref: "authority", Revision: 1, Epoch: 1}, Scope: scope, RunID: "run-a", ActionScopeDigest: d("action"), State: ports.AuthorityFactActive, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()})
	if e != nil {
		t.Fatal(e)
	}
	return f
}
func controlActorProjectionV2(t *testing.T, now time.Time) ports.ReviewActorAuthorityCurrentProjectionV2 {
	t.Helper()
	fact := controlDispatchAuthorityV3(t, now)
	d, _ := core.CanonicalJSONDigest("control-actor-v2", "v1", "value", "target")
	subject := ports.ReviewActorAuthorityCurrentSubjectV2{Target: ports.ReviewDecisionTargetRefV1{TenantID: fact.Scope.Identity.TenantID, ID: "target", Revision: 1, Digest: d, RunID: fact.RunID}, ActorAuthority: fact.Ref, ActionScopeDigest: fact.ActionScopeDigest}
	p, e := ports.SealReviewActorAuthorityCurrentProjectionV2(ports.ReviewActorAuthorityCurrentProjectionV2{Ref: ports.ReviewActorAuthorityCurrentProjectionRefV2{Revision: 1}, Subject: subject, Fact: fact, State: ports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
