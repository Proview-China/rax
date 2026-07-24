package registry_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/registry"
)

type factory struct{}

func (factory) StartOrInspectConstructionV1(context.Context, ports.ConstructRequestV1) (ports.ComponentHandleV1, error) {
	return nil, contract.NewError(contract.ErrorUnavailable, "unused", "unused")
}
func (factory) InspectConstructionV1(context.Context, ports.ConstructRequestV1) (ports.ComponentHandleV1, error) {
	return nil, contract.NewError(contract.ErrorUnavailable, "unused", "unused")
}

type nilFactory struct{}

func (*nilFactory) StartOrInspectConstructionV1(context.Context, ports.ConstructRequestV1) (ports.ComponentHandleV1, error) {
	return nil, nil
}
func (*nilFactory) InspectConstructionV1(context.Context, ports.ConstructRequestV1) (ports.ComponentHandleV1, error) {
	return nil, nil
}

func factoryKey(t *testing.T, id, capability string) contract.FactoryKeyV1 {
	t.Helper()
	digest, _ := contract.DigestJSONV1("artifact-" + id)
	return contract.FactoryKeyV1{ComponentID: id, ArtifactDigest: digest, Contract: "praxis.fixture/v1", Capability: capability}
}

func TestRegistryExactClosedByPlanAndAliasRejected(t *testing.T) {
	r := registry.NewV1()
	key := factoryKey(t, "component-a", "praxis.fixture/read")
	if err := r.RegisterV1(key, factory{}); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterV1(key, factory{}); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatal("duplicate accepted")
	}
	alternative := key
	alternative.Capability = "praxis.fixture/write"
	if err := r.RegisterV1(alternative, factory{}); err != nil {
		t.Fatalf("exact alternate capability rejected: %v", err)
	}
	if err := r.SealV1(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ResolveV1(key); err != nil {
		t.Fatal(err)
	}
	drift := key
	drift.Contract = "praxis.fixture/v2"
	if _, err := r.ResolveV1(drift); !contract.HasCode(err, contract.ErrorNotFound) {
		t.Fatal("contract drift resolved")
	}
	if err := r.RegisterV1(factoryKey(t, "component-b", "praxis.fixture/read"), factory{}); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatal("sealed registry accepted registration")
	}
}

func TestRegistryRejectsTypedNilFactory(t *testing.T) {
	r := registry.NewV1()
	var value *nilFactory
	if err := r.RegisterV1(factoryKey(t, "component-a", "praxis.fixture/read"), value); !contract.HasCode(err, contract.ErrorInvalidArgument) {
		t.Fatal("typed nil factory accepted")
	}
}

func TestRegistryConcurrentExactResolve(t *testing.T) {
	r := registry.NewV1()
	key := factoryKey(t, "component-a", "praxis.fixture/read")
	if err := r.RegisterV1(key, factory{}); err != nil {
		t.Fatal(err)
	}
	if err := r.SealV1(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errors := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := r.ResolveV1(key); errors <- err }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
}
