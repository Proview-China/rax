package runtimeadapter_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
)

type bindingStoreV4 struct {
	mu     sync.Mutex
	values map[string]runtimeports.OperationSettlementDomainResultFactRefV4
	fail   bool
}

func (s *bindingStoreV4) CreateDomainResultRuntimeBindingV4(_ context.Context, value runtimeports.OperationSettlementDomainResultFactRefV4) (runtimeports.OperationSettlementDomainResultFactRefV4, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.values[value.ID]; ok {
		if runtimeports.SameOperationSettlementDomainResultFactRefV4(existing, value) {
			return existing, nil
		}
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, errors.New("conflict")
	}
	s.values[value.ID] = value
	if s.fail {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, errors.New("lost reply")
	}
	return value, nil
}

func (s *bindingStoreV4) InspectDomainResultRuntimeBindingV4(_ context.Context, id string) (runtimeports.OperationSettlementDomainResultFactRefV4, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[id]
	if !ok {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, errors.New("not found")
	}
	return value, nil
}

func TestDomainResultCurrentAdapterV4BindsOwnerFactAndRecoversLostReply(t *testing.T) {
	adapter, request, bindings := domainResultFixtureV4(t)
	bindings.fail = true
	ref, err := adapter.BindDomainResultRuntimeV4(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	current, err := adapter.InspectOperationSettlementDomainResultCurrentV4(context.Background(), request.EffectKind, ref)
	if err != nil {
		t.Fatal(err)
	}
	if current.Fact.ID != request.ResultID || current.EffectKind != request.EffectKind || current.ExpiresUnixNano <= current.CheckedUnixNano {
		t.Fatalf("unexpected current projection: %#v", current)
	}
	tampered := ref
	tampered.Attempt.PermitDigest = digestV4("other-permit")
	if _, err := adapter.InspectOperationSettlementDomainResultCurrentV4(context.Background(), request.EffectKind, tampered); err == nil {
		t.Fatal("tampered Runtime attempt was accepted")
	}
}

func TestDomainResultCurrentAdapterV4FailsClosedOnExpiredOrCrossEffectFact(t *testing.T) {
	adapter, request, _ := domainResultFixtureV4(t)
	ref, err := adapter.BindDomainResultRuntimeV4(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.InspectOperationSettlementDomainResultCurrentV4(context.Background(), "praxis.sandbox/activate", ref); err == nil {
		t.Fatal("cross-effect DomainResult was accepted")
	}
}

func domainResultFixtureV4(t *testing.T) (*runtimeadapter.DomainResultCurrentAdapterV4, runtimeadapter.BindDomainResultRuntimeV4Request, *bindingStoreV4) {
	t.Helper()
	store := testkit.NewMemoryStore()
	reservation := testkit.Reservation(contract.EffectAllocate, 1, "domain-current")
	operation := testkit.RuntimeOperation(reservation)
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	reservation.OperationSubjectDigest = string(operationDigest)
	observation := testkit.Observation(reservation, 1, "domain-current")
	inspection := testkit.Inspection(reservation, observation, contract.DispositionConfirmedApplied, "domain-current")
	result := testkit.Result(reservation, inspection, contract.DomainResultPayload{AllocationConfirmed: true}, "domain-current")
	if err := store.CreateReservation(context.Background(), reservation); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateDomainResult(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	bindings := &bindingStoreV4{values: make(map[string]runtimeports.OperationSettlementDomainResultFactRefV4)}
	owner := runtimeports.ProviderBindingRefV2{
		BindingSetID: "sandbox-bindings", BindingSetRevision: 1,
		ComponentID: "praxis.sandbox/controller", ManifestDigest: digestV4("sandbox-manifest"),
		ArtifactDigest: digestV4("sandbox-artifact"), Capability: "praxis.sandbox/domain-result-owner",
	}
	schema := runtimeports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "domain-result", Version: "1.0.0", MediaType: "application/json", ContentDigest: digestV4("domain-result-schema")}
	adapter, err := runtimeadapter.NewDomainResultCurrentAdapterV4(store, bindings, owner, schema, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	attempt := runtimeports.OperationDispatchAttemptRefV3{
		OperationDigest: operationDigest, EffectID: runtimecore.EffectIntentID(reservation.EffectID),
		IntentRevision: runtimecore.Revision(reservation.IntentRevision), IntentDigest: prefixedDigest(reservation.IntentDigest),
		PermitID: "permit-domain-current", PermitRevision: 1, PermitDigest: digestV4("permit"), AttemptID: reservation.AttemptID,
	}
	return adapter, runtimeadapter.BindDomainResultRuntimeV4Request{EffectKind: runtimeports.EffectKindV2(reservation.Kind), ResultID: result.Meta.ID, Operation: operation, Attempt: attempt}, bindings
}

func digestV4(value string) runtimecore.Digest {
	digest, err := runtimecore.CanonicalJSONDigest("sandbox-test", "1.0.0", "fixture", value)
	if err != nil {
		panic(err)
	}
	return digest
}
