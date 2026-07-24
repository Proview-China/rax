package runtimeadapter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
)

type currentGraph struct {
	attempt         contract.DomainAttemptFact
	expectedAttempt runtimeports.OperationDispatchSandboxFactRefV4
	reservation     contract.DomainReservation
	lease           contract.RuntimeLeaseBindingFact
	projection      contract.EnvironmentProjection
	requirement     contract.ExecutionRequirement
	policy          contract.PolicyProjection
	placement       contract.PlacementCandidate
	backend         contract.BackendDescriptor
	slot            contract.SlotCandidate
	operation       runtimeports.OperationSubjectV3
	generation      runtimeports.GenerationBindingAssociationFactV1
}

func TestIntegrationExactCurrentReaderV4ReturnsSealedMinimumTTLProjection(t *testing.T) {
	t.Parallel()
	if !ports.Supported(ports.FeatureExactCurrentReader) {
		t.Fatal("exact-current Reader support matrix is false")
	}
	reader, graph := exactCurrentReader(t, func(g *currentGraph) {
		g.backend.Meta.ExpiresUnixNano = testkit.FixedNow.Add(2 * time.Hour).UnixNano()
	})
	got, err := reader.InspectOperationDispatchSandboxCurrentV4(context.Background(), graph.operation, runtimecore.EffectIntentID(graph.reservation.EffectID), runtimeFactRef(graph.attempt.Meta))
	if err != nil {
		t.Fatal(err)
	}
	if err := got.ValidateCurrent(graph.operation, runtimecore.EffectIntentID(graph.reservation.EffectID), runtimecore.Revision(graph.reservation.IntentRevision), prefixedDigest(graph.reservation.IntentDigest), graph.reservation.AttemptID, runtimeProvider(graph.reservation.ProviderBinding), testkit.FixedNow); err != nil {
		t.Fatalf("current projection: %v", err)
	}
	if got.ExpiresUnixNano != graph.backend.Meta.ExpiresUnixNano {
		t.Fatalf("projection expiry = %d, want minimum backend TTL %d", got.ExpiresUnixNano, graph.backend.Meta.ExpiresUnixNano)
	}
	if got.Attempt != runtimeFactRef(graph.attempt.Meta) || got.Reservation.ID != graph.reservation.Meta.ID || got.RuntimeLease.Ref.ID != graph.lease.Meta.ID || got.Generation.ID != graph.generation.ID {
		t.Fatalf("projection lost exact owner refs: %#v", got)
	}
	tampered := got
	tampered.ExpiresUnixNano++
	if err := tampered.Validate(); err == nil {
		t.Fatal("projection TTL tamper retained old digest")
	}
}

func TestIntegrationExactCurrentReaderV4BindsAttemptTTLIntoProjection(t *testing.T) {
	t.Parallel()
	reader, graph := exactCurrentReader(t, func(g *currentGraph) {
		g.attempt.Meta.ExpiresUnixNano = testkit.FixedNow.Add(time.Hour).UnixNano()
		g.expectedAttempt = runtimeFactRef(g.attempt.Meta)
	})
	got, err := reader.InspectOperationDispatchSandboxCurrentV4(context.Background(), graph.operation, runtimecore.EffectIntentID(graph.reservation.EffectID), graph.expectedAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempt != runtimeFactRef(graph.attempt.Meta) || got.ExpiresUnixNano != graph.attempt.Meta.ExpiresUnixNano {
		t.Fatalf("attempt ref/TTL not bound into projection: %#v", got)
	}
}

func TestBlackboxExactCurrentReaderV4FailsClosedOnEveryExactAxis(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*currentGraph){
		"revoked attempt":     func(g *currentGraph) { g.attempt.State = contract.CurrentFactRevoked },
		"unknown attempt":     func(g *currentGraph) { g.attempt.State = contract.CurrentFactIndeterminate },
		"expired attempt":     func(g *currentGraph) { g.attempt.Meta.ExpiresUnixNano = testkit.FixedNow.UnixNano() },
		"attempt revision":    func(g *currentGraph) { g.attempt.Meta.Revision++ },
		"attempt digest":      func(g *currentGraph) { g.attempt.Meta.Digest = testkit.Ref("tampered-attempt").Digest },
		"attempt reservation": func(g *currentGraph) { g.attempt.ReservationRef = testkit.Ref("other-reservation") },
		"attempt lease":       func(g *currentGraph) { g.attempt.RuntimeLeaseBindingRef = testkit.Ref("other-lease") },
		"attempt intent":      func(g *currentGraph) { g.attempt.IntentDigest = testkit.Ref("other-intent").Digest },
		"revoked reservation": func(g *currentGraph) { g.reservation.State = contract.CurrentFactRevoked },
		"unknown reservation": func(g *currentGraph) { g.reservation.State = contract.CurrentFactIndeterminate },
		"expired reservation": func(g *currentGraph) { g.reservation.Meta.ExpiresUnixNano = testkit.FixedNow.UnixNano() },
		"tenant": func(g *currentGraph) {
			g.reservation.Lease.TenantID = "other-tenant"
			g.lease.Binding = g.reservation.Lease
			g.projection.Lease = g.reservation.Lease
		},
		"lease epoch":       func(g *currentGraph) { g.lease.Binding.LeaseEpoch++ },
		"instance epoch":    func(g *currentGraph) { g.lease.Binding.InstanceEpoch++ },
		"fence epoch":       func(g *currentGraph) { g.lease.Binding.FenceEpoch++ },
		"observed revision": func(g *currentGraph) { g.lease.Binding.ObservedRevision++ },
		"scope":             func(g *currentGraph) { g.policy.ScopeDigest = testkit.Ref("other-scope").Digest },
		"provider":          func(g *currentGraph) { g.slot.ProviderBinding.ArtifactDigest = testkit.Ref("other-provider").Digest },
		"generation provider": func(g *currentGraph) {
			g.reservation.ProviderBinding.BindingSetRevision++
			g.slot.ProviderBinding = g.reservation.ProviderBinding
		},
		"placement":          func(g *currentGraph) { g.placement.BackendRef = testkit.Ref("other-backend") },
		"reservation digest": func(g *currentGraph) { g.reservation.OperationSubjectDigest = testkit.Ref("tampered-operation").Digest },
		"expired lease fact": func(g *currentGraph) { g.lease.Meta.ExpiresUnixNano = testkit.FixedNow.UnixNano() },
		"expired lease value": func(g *currentGraph) {
			g.lease.Binding.ExpiresUnixNano = testkit.FixedNow.UnixNano()
			g.reservation.Lease.ExpiresUnixNano = g.lease.Binding.ExpiresUnixNano
			g.projection.Lease.ExpiresUnixNano = g.lease.Binding.ExpiresUnixNano
		},
		"expired projection":  func(g *currentGraph) { g.projection.Meta.ExpiresUnixNano = testkit.FixedNow.UnixNano() },
		"expired requirement": func(g *currentGraph) { g.requirement.Meta.ExpiresUnixNano = testkit.FixedNow.UnixNano() },
		"expired policy":      func(g *currentGraph) { g.policy.Meta.ExpiresUnixNano = testkit.FixedNow.UnixNano() },
		"expired placement":   func(g *currentGraph) { g.placement.Meta.ExpiresUnixNano = testkit.FixedNow.UnixNano() },
		"expired backend":     func(g *currentGraph) { g.backend.Meta.ExpiresUnixNano = testkit.FixedNow.UnixNano() },
		"expired slot":        func(g *currentGraph) { g.slot.Meta.ExpiresUnixNano = testkit.FixedNow.UnixNano() },
		"expired generation":  func(g *currentGraph) { g.generation.ExpiresUnixNano = testkit.FixedNow.UnixNano() },
		"revoked generation":  func(g *currentGraph) { g.generation.State = runtimeports.GenerationBindingAssociationRevokedV1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			reader, graph := exactCurrentReader(t, mutate)
			got, err := reader.InspectOperationDispatchSandboxCurrentV4(context.Background(), graph.operation, runtimecore.EffectIntentID(graph.reservation.EffectID), graph.expectedAttempt)
			if err == nil {
				t.Fatalf("drift returned current projection: %#v", got)
			}
			if got.ProjectionDigest != "" {
				t.Fatalf("drift returned a partial sealed projection: %#v", got)
			}
		})
	}
}

func TestBlackboxExactCurrentReaderV4RejectsWrongRequestCoordinates(t *testing.T) {
	t.Parallel()
	reader, graph := exactCurrentReader(t, nil)
	tests := map[string]func(*runtimeports.OperationSubjectV3, *runtimecore.EffectIntentID, *runtimeports.OperationDispatchSandboxFactRefV4){
		"operation": func(operation *runtimeports.OperationSubjectV3, _ *runtimecore.EffectIntentID, _ *runtimeports.OperationDispatchSandboxFactRefV4) {
			operation.CustomOperationID = "other-operation"
		},
		"effect": func(_ *runtimeports.OperationSubjectV3, effect *runtimecore.EffectIntentID, _ *runtimeports.OperationDispatchSandboxFactRefV4) {
			*effect = "other-effect"
		},
		"attempt id": func(_ *runtimeports.OperationSubjectV3, _ *runtimecore.EffectIntentID, attempt *runtimeports.OperationDispatchSandboxFactRefV4) {
			attempt.ID = "other-attempt"
		},
		"attempt revision": func(_ *runtimeports.OperationSubjectV3, _ *runtimecore.EffectIntentID, attempt *runtimeports.OperationDispatchSandboxFactRefV4) {
			attempt.Revision++
		},
		"attempt digest": func(_ *runtimeports.OperationSubjectV3, _ *runtimecore.EffectIntentID, attempt *runtimeports.OperationDispatchSandboxFactRefV4) {
			attempt.Digest = prefixedDigest(testkit.Ref("wrong-attempt").Digest)
		},
		"attempt ttl": func(_ *runtimeports.OperationSubjectV3, _ *runtimecore.EffectIntentID, attempt *runtimeports.OperationDispatchSandboxFactRefV4) {
			attempt.ExpiresUnixNano--
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			operation := graph.operation
			effect := runtimecore.EffectIntentID(graph.reservation.EffectID)
			attempt := graph.expectedAttempt
			mutate(&operation, &effect, &attempt)
			got, err := reader.InspectOperationDispatchSandboxCurrentV4(context.Background(), operation, effect, attempt)
			if err == nil || got.ProjectionDigest != "" {
				t.Fatalf("wrong coordinate returned projection=%#v err=%v", got, err)
			}
		})
	}
}

func TestFaultExactCurrentReaderLostReplyReturnsZeroAndNeverRetries(t *testing.T) {
	t.Parallel()
	baseReader, graph := exactCurrentReader(t, nil)
	_ = baseReader
	base := graphStore(t, graph)
	store := &failingCurrentStore{ExactCurrentStore: base, err: errors.New("injected lost reply")}
	reader, err := runtimeadapter.NewCurrentReaderV4(store, &testkit.GenerationReader{Fact: graph.generation}, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	got, err := reader.InspectOperationDispatchSandboxCurrentV4(context.Background(), graph.operation, runtimecore.EffectIntentID(graph.reservation.EffectID), runtimeFactRef(graph.attempt.Meta))
	if err == nil || got.ProjectionDigest != "" || store.backendReads != 1 {
		t.Fatalf("lost reply result=%#v err=%v backendReads=%d", got, err, store.backendReads)
	}
}

func TestWhiteboxExactCurrentReaderIndependentlyReadsAttemptAndReservation(t *testing.T) {
	t.Parallel()
	_, graph := exactCurrentReader(t, nil)
	store := &countingCurrentStore{ExactCurrentStore: graphStore(t, graph)}
	reader, err := runtimeadapter.NewCurrentReaderV4(store, &testkit.GenerationReader{Fact: graph.generation}, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.InspectOperationDispatchSandboxCurrentV4(context.Background(), graph.operation, runtimecore.EffectIntentID(graph.reservation.EffectID), graph.expectedAttempt); err != nil {
		t.Fatal(err)
	}
	if store.attemptReads != 1 || store.reservationReads != 1 {
		t.Fatalf("attemptReads=%d reservationReads=%d, want independent single reads", store.attemptReads, store.reservationReads)
	}
}

func TestFaultExactCurrentReaderAttemptLostReplyStopsBeforeReservation(t *testing.T) {
	t.Parallel()
	_, graph := exactCurrentReader(t, nil)
	store := &countingCurrentStore{ExactCurrentStore: graphStore(t, graph), attemptErr: errors.New("injected attempt lost reply")}
	reader, err := runtimeadapter.NewCurrentReaderV4(store, &testkit.GenerationReader{Fact: graph.generation}, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	got, err := reader.InspectOperationDispatchSandboxCurrentV4(context.Background(), graph.operation, runtimecore.EffectIntentID(graph.reservation.EffectID), graph.expectedAttempt)
	if err == nil || got.ProjectionDigest != "" || store.attemptReads != 1 || store.reservationReads != 0 {
		t.Fatalf("lost attempt reply projection=%#v err=%v attemptReads=%d reservationReads=%d", got, err, store.attemptReads, store.reservationReads)
	}
}

type countingCurrentStore struct {
	ports.ExactCurrentStore
	attemptErr       error
	attemptReads     int
	reservationReads int
}

func (s *countingCurrentStore) GetAttempt(ctx context.Context, id string) (contract.DomainAttemptFact, error) {
	s.attemptReads++
	if s.attemptErr != nil {
		return contract.DomainAttemptFact{}, s.attemptErr
	}
	return s.ExactCurrentStore.GetAttempt(ctx, id)
}

func (s *countingCurrentStore) InspectReservationByAttempt(ctx context.Context, operationID, effectID, attemptID string) (contract.DomainReservation, error) {
	s.reservationReads++
	return s.ExactCurrentStore.InspectReservationByAttempt(ctx, operationID, effectID, attemptID)
}

type failingCurrentStore struct {
	ports.ExactCurrentStore
	err          error
	backendReads int
}

func (s *failingCurrentStore) GetBackend(context.Context, string) (contract.BackendDescriptor, error) {
	s.backendReads++
	return contract.BackendDescriptor{}, s.err
}

func exactCurrentReader(t *testing.T, mutate func(*currentGraph)) (*runtimeadapter.CurrentReaderV4, currentGraph) {
	t.Helper()
	reservation := testkit.Reservation(contract.EffectAllocate, 1, "current-reader")
	operation := testkit.RuntimeOperation(reservation)
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	reservation.OperationSubjectDigest = string(operationDigest)
	reservation.Lease.ScopeDigest = string(operation.ExecutionScopeDigest)
	lease := testkit.LeaseFact()
	lease.Binding = reservation.Lease
	reservation.RuntimeLeaseBindingRef = lease.Meta.Ref()
	attempt := testkit.Attempt(reservation)
	projection := testkit.Projection()
	projection.Lease = reservation.Lease
	requirement, policy, placement, backend, slot := testkit.Requirement(), testkit.Policy(), testkit.Candidate(), testkit.Backend(), testkit.Slot()
	policy.ScopeDigest = reservation.Lease.ScopeDigest
	generation := testkit.GenerationAssociation(operation, testkit.FixedNow.Add(6*time.Hour))
	reservation.GenerationBindingRef = contract.Ref{ID: generation.ID, Revision: uint64(generation.Revision), Digest: string(generation.Digest)}
	graph := currentGraph{attempt: attempt, expectedAttempt: runtimeFactRef(attempt.Meta), reservation: reservation, lease: lease, projection: projection, requirement: requirement, policy: policy, placement: placement, backend: backend, slot: slot, operation: operation, generation: generation}
	if mutate != nil {
		mutate(&graph)
	}
	store := graphStore(t, graph)
	reader, err := runtimeadapter.NewCurrentReaderV4(store, &testkit.GenerationReader{Fact: graph.generation}, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	return reader, graph
}

func graphStore(t *testing.T, graph currentGraph) *testkit.MemoryStore {
	t.Helper()
	store := testkit.NewMemoryStore()
	if err := store.SeedProjection(graph.projection); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedExactCurrentFacts(graph.attempt, graph.lease, graph.requirement, graph.policy, graph.placement, graph.backend, graph.slot); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateReservation(context.Background(), graph.reservation); err != nil {
		t.Fatal(err)
	}
	return store
}

func runtimeFactRef(meta contract.Meta) runtimeports.OperationDispatchSandboxFactRefV4 {
	return runtimeports.OperationDispatchSandboxFactRefV4{ID: meta.ID, Revision: runtimecore.Revision(meta.Revision), Digest: prefixedDigest(meta.Digest), ExpiresUnixNano: meta.ExpiresUnixNano}
}

func runtimeProvider(value contract.ProviderBindingRef) runtimeports.ProviderBindingRefV2 {
	return runtimeports.ProviderBindingRefV2{BindingSetID: value.BindingSetID, BindingSetRevision: runtimecore.Revision(value.BindingSetRevision), ComponentID: runtimeports.ComponentIDV2(value.ComponentID), ManifestDigest: prefixedDigest(value.ManifestDigest), ArtifactDigest: prefixedDigest(value.ArtifactDigest), Capability: runtimeports.CapabilityNameV2(value.Capability)}
}

func prefixedDigest(value string) runtimecore.Digest {
	if len(value) >= len("sha256:") && value[:len("sha256:")] == "sha256:" {
		return runtimecore.Digest(value)
	}
	return runtimecore.Digest("sha256:" + value)
}
