package registry

import (
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

type ControlRegistryV2 struct {
	mu        sync.RWMutex
	sealed    bool
	factories map[contract.ControlAdapterFactoryRefV2]ports.ControlAdapterFactoryV2
}

func NewControlV2() *ControlRegistryV2 {
	return &ControlRegistryV2{factories: make(map[contract.ControlAdapterFactoryRefV2]ports.ControlAdapterFactoryV2)}
}

func (r *ControlRegistryV2) RegisterControlAdapterFactoryV2(factory ports.ControlAdapterFactoryV2) error {
	if r == nil {
		return contract.NewError(contract.ErrorUnavailable, "control_registry_missing", "control adapter registry is nil")
	}
	if contract.IsTypedNilV1(factory) {
		return contract.NewError(contract.ErrorInvalidArgument, "control_adapter_factory_missing", "control adapter factory is required")
	}
	descriptor := factory.DescriptorV2()
	if err := descriptor.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return contract.NewError(contract.ErrorPrecondition, "control_registry_sealed", "control adapter registry is sealed")
	}
	if _, exists := r.factories[descriptor.Ref]; exists {
		return contract.NewError(contract.ErrorConflict, "control_adapter_factory_duplicate", "exact control adapter factory is already registered")
	}
	r.factories[descriptor.Ref] = factory
	return nil
}

func (r *ControlRegistryV2) SealControlAdapterRegistryV2() error {
	if r == nil {
		return contract.NewError(contract.ErrorUnavailable, "control_registry_missing", "control adapter registry is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.factories) == 0 {
		return contract.NewError(contract.ErrorPrecondition, "control_registry_empty", "control adapter registry cannot seal empty")
	}
	r.sealed = true
	return nil
}

func (r *ControlRegistryV2) ResolveControlAdapterFactoryV2(expected contract.ControlAdapterFactoryRefV2) (ports.ControlAdapterFactoryV2, error) {
	if r == nil {
		return nil, contract.NewError(contract.ErrorUnavailable, "control_registry_missing", "control adapter registry is nil")
	}
	if err := expected.Validate(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.sealed {
		return nil, contract.NewError(contract.ErrorPrecondition, "control_registry_unsealed", "control adapter registry must be sealed")
	}
	factory, exists := r.factories[expected]
	if !exists {
		return nil, contract.NewError(contract.ErrorNotFound, "control_adapter_factory_not_bound", "exact executable control adapter factory is not registered")
	}
	return factory, nil
}
