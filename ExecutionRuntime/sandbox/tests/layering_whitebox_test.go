package sandbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWhiteBoxDomainResultDoesNotMutateBeforeRuntimeSettlement(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewMemoryStore()
	if err := store.SeedProjection(testkit.Projection()); err != nil {
		t.Fatal(err)
	}
	controller, _ := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
	result := commitAppliedResult(t, ctx, controller, contract.EffectAllocate, 1, 1, "layering", contract.DomainResultPayload{AllocationConfirmed: true})

	before, err := store.GetProjection(ctx, testkit.Lease().LeaseID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Allocated || before.Meta.Revision != 1 {
		t.Fatalf("DomainResultFact mutated projection before Runtime settlement: %#v", before)
	}

	bad := testkit.Settlement(result, "bad")
	bad.DomainResultRef = testkit.Ref("another-domain-result")
	if _, err := controller.ApplySettlement(ctx, result.Meta.ID, bad); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatalf("mismatched settlement error = %v", err)
	}
	bad = testkit.Settlement(result, "bad-attempt")
	bad.AttemptID = "another-attempt"
	if _, err := controller.ApplySettlement(ctx, result.Meta.ID, bad); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatalf("mismatched settlement attempt error = %v", err)
	}

	after, err := controller.ApplySettlement(ctx, result.Meta.ID, testkit.Settlement(result, "good"))
	if err != nil {
		t.Fatal(err)
	}
	if !after.Allocated || after.Meta.Revision != 2 || after.LastSettlementRef.ID == "" {
		t.Fatalf("exact settlement was not applied: %#v", after)
	}
	replayed, err := controller.ApplySettlement(ctx, result.Meta.ID, testkit.Settlement(result, "good"))
	if err != nil || replayed.Meta.Revision != after.Meta.Revision {
		t.Fatalf("exact settlement replay was not idempotent: %#v, %v", replayed, err)
	}
}

func TestWhiteBoxCannotCommitResultBeforeInspection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewMemoryStore()
	_ = store.SeedProjection(testkit.Projection())
	controller, _ := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
	reservation := testkit.Reservation(contract.EffectAllocate, 1, "order")
	if err := controller.Reserve(ctx, reservation); err != nil {
		t.Fatal(err)
	}
	missingInspection := testkit.Inspection(reservation, testkit.Observation(reservation, 1, "order"), contract.DispositionConfirmedApplied, "missing")
	result := testkit.Result(reservation, missingInspection, contract.DomainResultPayload{AllocationConfirmed: true}, "order")
	if err := controller.CommitDomainResult(ctx, result); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("CommitDomainResult error = %v, want not found", err)
	}
}

func TestWhiteBoxRejectsActivationBeforeAllocation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewMemoryStore()
	_ = store.SeedProjection(testkit.Projection())
	controller, _ := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
	result := commitAppliedResult(t, ctx, controller, contract.EffectActivate, 1, 1, "activate-too-early", contract.DomainResultPayload{ActivationConfirmed: true})
	if _, err := controller.ApplySettlement(ctx, result.Meta.ID, testkit.Settlement(result, "activate-too-early")); !errors.Is(err, kernel.ErrInvalidTransition) {
		t.Fatalf("activation-before-allocation error = %v", err)
	}
}

func TestWhiteBoxRejectsLeaseDriftAtDomainResult(t *testing.T) {
	tests := leaseDriftCases()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store := testkit.NewMemoryStore()
			_ = store.SeedProjection(testkit.Projection())
			controller, _ := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
			reservation := testkit.Reservation(contract.EffectAllocate, 1, "result-"+test.name)
			if err := controller.Reserve(ctx, reservation); err != nil {
				t.Fatal(err)
			}
			observation := testkit.Observation(reservation, 1, "result-"+test.name)
			if _, err := controller.RecordObservation(ctx, observation); err != nil {
				t.Fatal(err)
			}
			inspection := testkit.Inspection(reservation, observation, contract.DispositionConfirmedApplied, "result-"+test.name)
			if err := controller.RecordInspection(ctx, inspection); err != nil {
				t.Fatal(err)
			}
			result := testkit.Result(reservation, inspection, contract.DomainResultPayload{AllocationConfirmed: true}, "result-"+test.name)
			test.mutate(&result.Lease)
			if err := controller.CommitDomainResult(ctx, result); !errors.Is(err, kernel.ErrInvalidTransition) {
				t.Fatalf("%s drift error = %v", test.name, err)
			}
		})
	}
}

func TestNoGoReserveRejectsProjectionAndLeaseDriftBeforeOwnerWrite(t *testing.T) {
	for _, test := range leaseDriftCases() {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store := testkit.NewMemoryStore()
			if err := store.SeedProjection(testkit.Projection()); err != nil {
				t.Fatal(err)
			}
			controller, _ := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
			reservation := testkit.Reservation(contract.EffectAllocate, 1, "reserve-"+test.name)
			test.mutate(&reservation.Lease)
			if err := controller.Reserve(ctx, reservation); err == nil {
				t.Fatalf("%s drift was reserved", test.name)
			}
			if _, err := store.GetReservation(ctx, reservation.Meta.ID); !errors.Is(err, ports.ErrNotFound) {
				t.Fatalf("%s drift wrote reservation: %v", test.name, err)
			}
		})
	}

	t.Run("projection revision", func(t *testing.T) {
		ctx := context.Background()
		store := testkit.NewMemoryStore()
		_ = store.SeedProjection(testkit.Projection())
		controller, _ := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
		reservation := testkit.Reservation(contract.EffectAllocate, 2, "stale-revision")
		if err := controller.Reserve(ctx, reservation); !errors.Is(err, ports.ErrStale) {
			t.Fatalf("stale revision error = %v", err)
		}
		if _, err := store.GetReservation(ctx, reservation.Meta.ID); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("stale revision wrote reservation: %v", err)
		}
	})

	t.Run("projection ttl", func(t *testing.T) {
		ctx := context.Background()
		store := testkit.NewMemoryStore()
		projection := testkit.Projection()
		projection.Meta.CreatedUnixNano = testkit.FixedNow.Add(-3 * time.Hour).UnixNano()
		projection.Meta.UpdatedUnixNano = testkit.FixedNow.Add(-2 * time.Hour).UnixNano()
		projection.Meta.ExpiresUnixNano = testkit.FixedNow.Add(-time.Hour).UnixNano()
		_ = store.SeedProjection(projection)
		controller, _ := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
		reservation := testkit.Reservation(contract.EffectAllocate, 1, "expired-projection")
		if err := controller.Reserve(ctx, reservation); err == nil {
			t.Fatal("expired projection authorized a reservation")
		}
		if _, err := store.GetReservation(ctx, reservation.Meta.ID); !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("expired projection wrote reservation: %v", err)
		}
	})
}

func TestNoGoFencedProjectionRejectsAllocateAndActivate(t *testing.T) {
	tests := []struct {
		name       string
		kind       contract.EffectKind
		projection func() contract.EnvironmentProjection
		payload    contract.DomainResultPayload
	}{
		{name: "allocate", kind: contract.EffectAllocate, projection: func() contract.EnvironmentProjection {
			p := testkit.Projection()
			p.Fenced = true
			return p
		}, payload: contract.DomainResultPayload{AllocationConfirmed: true}},
		{name: "activate", kind: contract.EffectActivate, projection: func() contract.EnvironmentProjection {
			p := testkit.Projection()
			p.Allocated = true
			p.Fenced = true
			return p
		}, payload: contract.DomainResultPayload{ActivationConfirmed: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store := testkit.NewMemoryStore()
			if err := store.SeedProjection(test.projection()); err != nil {
				t.Fatal(err)
			}
			controller, _ := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
			result := commitAppliedResult(t, ctx, controller, test.kind, 1, 1, "fenced-"+test.name, test.payload)
			if _, err := controller.ApplySettlement(ctx, result.Meta.ID, testkit.Settlement(result, "fenced-"+test.name)); !errors.Is(err, kernel.ErrInvalidTransition) {
				t.Fatalf("fenced %s error = %v", test.name, err)
			}
		})
	}
}

type leaseDriftCase struct {
	name   string
	mutate func(*contract.RuntimeLeaseBinding)
}

func leaseDriftCases() []leaseDriftCase {
	return []leaseDriftCase{
		{name: "tenant", mutate: func(v *contract.RuntimeLeaseBinding) { v.TenantID += "-drift" }},
		{name: "instance", mutate: func(v *contract.RuntimeLeaseBinding) { v.InstanceID += "-drift" }},
		{name: "instance-epoch", mutate: func(v *contract.RuntimeLeaseBinding) { v.InstanceEpoch++ }},
		{name: "lease", mutate: func(v *contract.RuntimeLeaseBinding) { v.LeaseID += "-drift" }},
		{name: "lease-epoch", mutate: func(v *contract.RuntimeLeaseBinding) { v.LeaseEpoch++ }},
		{name: "fence", mutate: func(v *contract.RuntimeLeaseBinding) { v.FenceEpoch++ }},
		{name: "scope", mutate: func(v *contract.RuntimeLeaseBinding) { v.ScopeDigest = testkit.Ref("scope-drift").Digest }},
		{name: "binding-revision", mutate: func(v *contract.RuntimeLeaseBinding) { v.ObservedRevision++ }},
		{name: "ttl", mutate: func(v *contract.RuntimeLeaseBinding) { v.ExpiresUnixNano += int64(time.Hour) }},
	}
}
