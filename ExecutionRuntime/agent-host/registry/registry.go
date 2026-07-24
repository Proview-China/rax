package registry

import (
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

type RegistryV1 struct {
	mu        sync.RWMutex
	sealed    bool
	factories map[string]entryV1
}

type entryV1 struct {
	key     contract.FactoryKeyV1
	factory ports.ComponentFactoryV1
}

func NewV1() *RegistryV1 { return &RegistryV1{factories: make(map[string]entryV1)} }

func (r *RegistryV1) RegisterV1(key contract.FactoryKeyV1, factory ports.ComponentFactoryV1) error {
	if err := key.Validate(); err != nil {
		return err
	}
	if contract.IsTypedNilV1(factory) {
		return contract.NewError(contract.ErrorInvalidArgument, "factory_missing", "factory is required")
	}
	id, err := key.CanonicalIDV1()
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return contract.NewError(contract.ErrorPrecondition, "registry_sealed", "factory registry is sealed")
	}
	if _, ok := r.factories[id]; ok {
		return contract.NewError(contract.ErrorConflict, "duplicate_factory", "exact factory key is already registered")
	}
	r.factories[id] = entryV1{key: key, factory: factory}
	return nil
}

func (r *RegistryV1) SealV1() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return nil
	}
	if len(r.factories) == 0 {
		return contract.NewError(contract.ErrorPrecondition, "registry_empty", "factory registry cannot seal empty")
	}
	r.sealed = true
	return nil
}

func (r *RegistryV1) ResolveV1(key contract.FactoryKeyV1) (ports.ComponentFactoryV1, error) {
	if err := key.Validate(); err != nil {
		return nil, err
	}
	id, err := key.CanonicalIDV1()
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.sealed {
		return nil, contract.NewError(contract.ErrorPrecondition, "registry_unsealed", "factory registry must be sealed before resolution")
	}
	entry, ok := r.factories[id]
	if !ok {
		return nil, contract.NewError(contract.ErrorNotFound, "factory_not_bound", "exact factory key is not registered")
	}
	return entry.factory, nil
}
