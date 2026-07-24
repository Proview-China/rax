package control_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/tests/testsupport"
)

func TestReviewDecisionGovernanceCurrentGatewayV1ExactPublishReadAndLostReply(t *testing.T) {
	t.Parallel()
	fixture := testsupport.ReviewDecisionGovernanceFixture()
	store := fakes.NewReviewDecisionGovernanceCurrentStoreV1()
	source := fakes.NewReviewDecisionGovernanceSourceStoreV1()
	testsupport.SeedReviewDecisionGovernanceSourcesV1(source, fixture)
	ctx, cancel := context.WithCancel(context.Background())
	wrapped := &cancelAfterCommitFactPortV1{ReviewDecisionGovernanceCurrentFactPortV1: store, cancel: cancel}
	gateway, err := control.NewReviewDecisionGovernanceCurrentGatewayV1(wrapped, source, source, source, source, func() time.Time { return fixture.Now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := gateway.PublishReviewDecisionPolicyCurrentV1(ctx, ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: fixture.Policy})
	if err != nil || receipt.Ref != fixture.Policy.Ref {
		t.Fatalf("lost reply exact recovery failed: receipt=%+v err=%v", receipt, err)
	}
	ctx = context.Background()
	if _, err := gateway.PublishReviewDecisionAuthorityCurrentV1(ctx, ports.ReviewDecisionAuthorityCurrentPublishRequestV1{Value: fixture.Authority}); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.PublishReviewDecisionScopeCurrentV1(ctx, ports.ReviewDecisionScopeCurrentPublishRequestV1{Value: fixture.Scope}); err != nil {
		t.Fatal(err)
	}
	policyRef, err := gateway.ResolveCurrentReviewDecisionPolicyV1(ctx, ports.ReviewDecisionPolicyCurrentResolveRequestV1{Subject: fixture.Policy.Subject})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.InspectCurrentReviewDecisionPolicyV1(ctx, fixture.Policy.Subject, policyRef); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.InspectHistoricalReviewDecisionPolicyV1(ctx, fixture.Policy.Ref); err != nil {
		t.Fatal(err)
	}
}

func TestReviewDecisionGovernanceCurrentGatewayV1RejectsProofAndS1S2Drift(t *testing.T) {
	t.Parallel()
	fixture := testsupport.ReviewDecisionGovernanceFixture()
	store := fakes.NewReviewDecisionGovernanceCurrentStoreV1()
	source := fakes.NewReviewDecisionGovernanceSourceStoreV1()
	testsupport.SeedReviewDecisionGovernanceSourcesV1(source, fixture)
	badTarget := fixture.Target
	badTarget.Digest = core.DigestBytes([]byte("drift"))
	source.PutTargetV1(badTarget)
	gateway, err := control.NewReviewDecisionGovernanceCurrentGatewayV1(store, source, source, source, source, func() time.Time { return fixture.Now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.PublishReviewDecisionPolicyCurrentV1(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: fixture.Policy}); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("Target proof drift did not fail closed: %v", err)
	}
	if _, err := store.InspectHistoricalPolicyV1(context.Background(), fixture.Policy.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("proof failure leaked a projection: %v", err)
	}
	badAssignment := fixture.Assignment
	badAssignment.Digest = core.DigestBytes([]byte("assignment-drift"))
	source.PutTargetV1(fixture.Target)
	source.PutAssignmentV1(badAssignment)
	if _, err := gateway.PublishReviewDecisionAuthorityCurrentV1(context.Background(), ports.ReviewDecisionAuthorityCurrentPublishRequestV1{Value: fixture.Authority}); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("Assignment proof drift did not fail closed: %v", err)
	}
	if _, err := store.InspectHistoricalAuthorityV1(context.Background(), fixture.Authority.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("Assignment proof failure leaked a projection: %v", err)
	}

	source.PutTargetV1(fixture.Target)
	source.PutAssignmentV1(fixture.Assignment)
	mutating := &mutatingPolicySourceV1{ReviewDecisionGovernanceSourceStoreV1: source}
	gateway, err = control.NewReviewDecisionGovernanceCurrentGatewayV1(store, source, mutating, source, source, func() time.Time { return fixture.Now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.PublishReviewDecisionPolicyCurrentV1(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: fixture.Policy}); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("Policy S1/S2 drift did not fail closed: %v", err)
	}
	if _, err := store.InspectHistoricalPolicyV1(context.Background(), fixture.Policy.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("S1/S2 failure leaked a projection: %v", err)
	}
}

func TestReviewDecisionGovernanceCurrentGatewayV1RejectsClockRollback(t *testing.T) {
	t.Parallel()
	fixture := testsupport.ReviewDecisionGovernanceFixture()
	clock := &sequenceClockV1{values: []time.Time{fixture.Now, fixture.Now.Add(2 * time.Second), fixture.Now.Add(time.Second)}}
	store := fakes.NewReviewDecisionGovernanceCurrentStoreV1()
	source := fakes.NewReviewDecisionGovernanceSourceStoreV1()
	testsupport.SeedReviewDecisionGovernanceSourcesV1(source, fixture)
	gateway, err := control.NewReviewDecisionGovernanceCurrentGatewayV1(store, source, source, source, source, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.PublishReviewDecisionPolicyCurrentV1(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: fixture.Policy}); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback did not fail closed: %v", err)
	}
	if _, err := store.InspectHistoricalPolicyV1(context.Background(), fixture.Policy.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("rollback leaked a projection: %v", err)
	}
}

func TestReviewDecisionGovernanceCurrentGatewayV1RejectsExpiredProjectionWithoutWrite(t *testing.T) {
	t.Parallel()
	fixture := testsupport.ReviewDecisionGovernanceFixture()
	store, _, gateway := reviewDecisionGatewayFixtureV1(t, fixture, func() time.Time { return fixture.Now.Add(31 * time.Second) })
	if _, err := gateway.PublishReviewDecisionPolicyCurrentV1(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: fixture.Policy}); err == nil {
		t.Fatal("expired active Policy projection was published")
	}
	if _, err := store.InspectHistoricalPolicyV1(context.Background(), fixture.Policy.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("expired publish leaked history: %v", err)
	}
}

func reviewDecisionGatewayFixtureV1(t *testing.T, fixture testsupport.ReviewDecisionGovernanceFixtureV1, clock func() time.Time) (*fakes.ReviewDecisionGovernanceCurrentStoreV1, *fakes.ReviewDecisionGovernanceSourceStoreV1, *control.ReviewDecisionGovernanceCurrentGatewayV1) {
	t.Helper()
	store := fakes.NewReviewDecisionGovernanceCurrentStoreV1()
	source := fakes.NewReviewDecisionGovernanceSourceStoreV1()
	testsupport.SeedReviewDecisionGovernanceSourcesV1(source, fixture)
	gateway, err := control.NewReviewDecisionGovernanceCurrentGatewayV1(store, source, source, source, source, clock)
	if err != nil {
		t.Fatal(err)
	}
	return store, source, gateway
}

type mutatingPolicySourceV1 struct {
	*fakes.ReviewDecisionGovernanceSourceStoreV1
	calls atomic.Int32
}

func (s *mutatingPolicySourceV1) InspectReviewPolicy(ctx context.Context, ref string) (ports.ReviewPolicyFactV2, error) {
	v, err := s.ReviewDecisionGovernanceSourceStoreV1.InspectReviewPolicy(ctx, ref)
	if err != nil {
		return v, err
	}
	if s.calls.Add(1) == 2 {
		v.Active = false
		v.Digest, _ = v.DigestV2()
		s.PutPolicyV1(v)
	}
	return v, nil
}

type sequenceClockV1 struct {
	mu     sync.Mutex
	values []time.Time
	index  int
}

type cancelAfterCommitFactPortV1 struct {
	control.ReviewDecisionGovernanceCurrentFactPortV1
	cancel context.CancelFunc
}

func (s *cancelAfterCommitFactPortV1) CommitPolicyV1(ctx context.Context, request ports.ReviewDecisionPolicyCurrentPublishRequestV1) (ports.ReviewDecisionPolicyCurrentPublishReceiptV1, error) {
	_, err := s.ReviewDecisionGovernanceCurrentFactPortV1.CommitPolicyV1(ctx, request)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
	}
	s.cancel()
	return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "lost reply after commit")
}

func (c *sequenceClockV1) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.index >= len(c.values) {
		return c.values[len(c.values)-1]
	}
	value := c.values[c.index]
	c.index++
	return value
}
