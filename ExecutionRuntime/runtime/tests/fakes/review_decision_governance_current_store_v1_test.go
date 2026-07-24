package fakes_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/tests/testsupport"
)

func TestReviewDecisionGovernanceCurrentStoreV1HistoryCASAndDeepClone(t *testing.T) {
	t.Parallel()
	fixture := testsupport.ReviewDecisionGovernanceFixture()
	store := fakes.NewReviewDecisionGovernanceCurrentStoreV1()
	ctx := context.Background()
	first := ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: fixture.Policy}
	if _, err := store.CommitPolicyV1(ctx, first); err != nil {
		t.Fatal(err)
	}
	next := fixture.NextPolicy(fixture.Now.Add(time.Second))
	previous := fixture.Policy.Ref
	if _, err := store.CommitPolicyV1(ctx, ports.ReviewDecisionPolicyCurrentPublishRequestV1{Previous: &previous, Value: next}); err != nil {
		t.Fatal(err)
	}
	current, err := store.ResolvePolicyV1(ctx, next.Subject)
	if err != nil || current != next.Ref {
		t.Fatalf("current index drifted: %+v %v", current, err)
	}
	old, err := store.InspectHistoricalPolicyV1(ctx, fixture.Policy.Ref)
	if err != nil || old.Ref != fixture.Policy.Ref {
		t.Fatalf("historical projection was lost: %+v %v", old, err)
	}
	if receipt, err := store.CommitPolicyV1(ctx, ports.ReviewDecisionPolicyCurrentPublishRequestV1{Previous: &previous, Value: next}); err != nil || receipt.Created {
		t.Fatalf("canonical replay was not idempotent: %+v %v", receipt, err)
	}
	different := fixture.NextPolicy(fixture.Now.Add(2 * time.Second))
	if _, err := store.CommitPolicyV1(ctx, ports.ReviewDecisionPolicyCurrentPublishRequestV1{Previous: &previous, Value: different}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale CAS did not conflict: %v", err)
	}

	if _, err := store.CommitScopeV1(ctx, ports.ReviewDecisionScopeCurrentPublishRequestV1{Value: fixture.Scope}); err != nil {
		t.Fatal(err)
	}
	read, err := store.InspectHistoricalScopeV1(ctx, fixture.Scope.Ref)
	if err != nil {
		t.Fatal(err)
	}
	read.Fact.Scope.SandboxLease.ID = "mutated"
	again, err := store.InspectHistoricalScopeV1(ctx, fixture.Scope.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if again.Fact.Scope.SandboxLease.ID == "mutated" {
		t.Fatal("historical Scope returned a mutable alias")
	}
}

func TestReviewDecisionGovernanceCurrentStoreV1ConcurrentCanonicalSingleCreate(t *testing.T) {
	t.Parallel()
	fixture := testsupport.ReviewDecisionGovernanceFixture()
	store := fakes.NewReviewDecisionGovernanceCurrentStoreV1()
	request := ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: fixture.Policy}
	var created, replayed, unexpected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipt, err := store.CommitPolicyV1(context.Background(), request)
			if err != nil {
				unexpected.Add(1)
				return
			}
			if receipt.Created {
				created.Add(1)
			} else {
				replayed.Add(1)
			}
		}()
	}
	wg.Wait()
	if created.Load() != 1 || replayed.Load() != 63 || unexpected.Load() != 0 {
		t.Fatalf("64-way canonical publish created=%d replayed=%d unexpected=%d", created.Load(), replayed.Load(), unexpected.Load())
	}

	drift := fixture.Policy
	drift.CheckedUnixNano++
	drift.Ref.Digest, drift.ProjectionDigest = "", ""
	digest, err := ports.DigestReviewDecisionPolicyCurrentProjectionV1(drift)
	if err != nil {
		t.Fatal(err)
	}
	drift.Ref.Digest, drift.ProjectionDigest = digest, digest
	if _, err := store.CommitPolicyV1(context.Background(), ports.ReviewDecisionPolicyCurrentPublishRequestV1{Value: drift}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same ID/revision different digest did not conflict: %v", err)
	}
}
