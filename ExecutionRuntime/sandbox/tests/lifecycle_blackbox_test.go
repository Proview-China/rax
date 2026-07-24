package sandbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
)

func TestBlackBoxLifecycleKeepsRunAndTerminationSeparate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewMemoryStore()
	if err := store.SeedProjection(testkit.Projection()); err != nil {
		t.Fatal(err)
	}
	controller, err := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}

	sequence := uint64(0)
	apply := func(kind contract.EffectKind, expectedRevision uint64, suffix string, payload contract.DomainResultPayload) contract.EnvironmentProjection {
		t.Helper()
		sequence++
		return settleApplied(t, ctx, controller, kind, expectedRevision, sequence, suffix, payload)
	}

	projection := apply(contract.EffectAllocate, 1, "allocate", contract.DomainResultPayload{AllocationConfirmed: true})
	projection = apply(contract.EffectActivate, projection.Meta.Revision, "activate", contract.DomainResultPayload{ActivationConfirmed: true})
	projection = apply(contract.EffectOpen, projection.Meta.Revision, "open", contract.DomainResultPayload{OpenConfirmed: true})

	sequence++
	prematureClose := commitAppliedResult(t, ctx, controller, contract.EffectClose, projection.Meta.Revision, sequence, "premature-close", contract.DomainResultPayload{EnvironmentClosed: true})
	if _, err := controller.ApplySettlement(ctx, prematureClose.Meta.ID, testkit.Settlement(prematureClose, "premature-close")); err == nil {
		t.Fatal("close applied before execution-quiesced was separately proven")
	}

	projection = apply(contract.EffectCancel, projection.Meta.Revision, "cancel", contract.DomainResultPayload{ExecutionQuiesced: true})
	if !projection.RunCompletion().ExecutionQuiesced {
		t.Fatal("execution-quiesced was not reported after cancel settlement")
	}
	if projection.Termination().EnvironmentClosed {
		t.Fatal("run completion incorrectly implied environment closed")
	}

	projection = apply(contract.EffectClose, projection.Meta.Revision, "close", contract.DomainResultPayload{EnvironmentClosed: true})
	if !projection.Termination().EnvironmentClosed || !projection.RunCompletion().ExecutionQuiesced {
		t.Fatalf("close corrupted run/termination separation: %#v", projection)
	}
	projection = apply(contract.EffectFence, projection.Meta.Revision, "fence", contract.DomainResultPayload{FenceConfirmed: true})
	projection = apply(contract.EffectRelease, projection.Meta.Revision, "release", contract.DomainResultPayload{ReleaseConfirmed: true})
	if projection.Termination().Cleanup.Complete() {
		t.Fatal("release incorrectly implied cleanup complete")
	}
	partialCleanup := testkit.CompleteCleanup()
	partialCleanup.RemoteContinuation = contract.CleanupIndeterminate
	projection = apply(contract.EffectCleanup, projection.Meta.Revision, "cleanup-partial", contract.DomainResultPayload{Cleanup: &partialCleanup})
	if projection.Termination().Cleanup.Complete() {
		t.Fatal("indeterminate cleanup was reported complete")
	}
	cleanup := testkit.CompleteCleanup()
	projection = apply(contract.EffectCleanup, projection.Meta.Revision, "cleanup", contract.DomainResultPayload{Cleanup: &cleanup})

	report := projection.Termination()
	if !report.EnvironmentClosed || !report.Fenced || !report.Released || !report.Cleanup.Complete() {
		t.Fatalf("termination report incomplete: %#v", report)
	}
}

func settleApplied(t *testing.T, ctx context.Context, controller *kernel.Controller, kind contract.EffectKind, expectedRevision, sequence uint64, suffix string, payload contract.DomainResultPayload) contract.EnvironmentProjection {
	t.Helper()
	result := commitAppliedResult(t, ctx, controller, kind, expectedRevision, sequence, suffix, payload)
	projection, err := controller.ApplySettlement(ctx, result.Meta.ID, testkit.Settlement(result, suffix))
	if err != nil {
		t.Fatalf("ApplySettlement(%s): %v", kind, err)
	}
	return projection
}

func commitAppliedResult(t *testing.T, ctx context.Context, controller *kernel.Controller, kind contract.EffectKind, expectedRevision, sequence uint64, suffix string, payload contract.DomainResultPayload) contract.SandboxDomainResultFact {
	t.Helper()
	reservation := testkit.Reservation(kind, expectedRevision, suffix)
	if err := controller.Reserve(ctx, reservation); err != nil {
		t.Fatalf("Reserve(%s): %v", kind, err)
	}
	observation := testkit.Observation(reservation, sequence, suffix)
	if accepted, err := controller.RecordObservation(ctx, observation); err != nil || !accepted {
		t.Fatalf("RecordObservation(%s) = %v, %v", kind, accepted, err)
	}
	inspection := testkit.Inspection(reservation, observation, contract.DispositionConfirmedApplied, suffix)
	if kind == contract.EffectCleanup {
		inspection.Cleanup = payload.Cleanup
	}
	if err := controller.RecordInspection(ctx, inspection); err != nil {
		t.Fatalf("RecordInspection(%s): %v", kind, err)
	}
	result := testkit.Result(reservation, inspection, payload, suffix)
	if err := controller.CommitDomainResult(ctx, result); err != nil {
		t.Fatalf("CommitDomainResult(%s): %v", kind, err)
	}
	return result
}
