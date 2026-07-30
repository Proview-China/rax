package registry

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

type componentFactoryEntryV2 struct {
	registration contract.ComponentFactoryRegistrationV2
	factory      ports.ComponentFactoryV2
}

// ComponentRegistryV2 is sealed before use. It admits typed, exact, closed
// descriptor/conformance declarations and detects descriptor drift on resolve.
// It does not identify fixture/internal/testkit implementations or establish
// implementation provenance, and it never marks a registration production
// eligible. Registration never invokes construction or inspects Owner state.
type ComponentRegistryV2 struct {
	mu      sync.RWMutex
	sealed  bool
	entries map[contract.DigestV1]componentFactoryEntryV2
}

func NewComponentV2() *ComponentRegistryV2 {
	return &ComponentRegistryV2{entries: make(map[contract.DigestV1]componentFactoryEntryV2)}
}

func (registry *ComponentRegistryV2) RegisterComponentFactoryV2(
	ctx context.Context,
	factory ports.ComponentFactoryV2,
	conformance contract.ComponentFactoryConformanceCurrentV2,
) (contract.ComponentFactoryRegistrationV2, error) {
	if registry == nil {
		return contract.ComponentFactoryRegistrationV2{}, contract.NewError(contract.ErrorUnavailable, "component_registry_missing", "component factory registry is nil")
	}
	if ctx == nil {
		return contract.ComponentFactoryRegistrationV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "component factory registry requires a context")
	}
	if err := ctx.Err(); err != nil {
		return contract.ComponentFactoryRegistrationV2{}, err
	}
	if contract.IsTypedNilV1(factory) {
		return contract.ComponentFactoryRegistrationV2{}, contract.NewError(contract.ErrorInvalidArgument, "component_factory_missing", "component factory is required")
	}
	descriptor := factory.DescriptorV2()
	if err := descriptor.Validate(); err != nil {
		return contract.ComponentFactoryRegistrationV2{}, err
	}
	if err := conformance.Validate(); err != nil {
		return contract.ComponentFactoryRegistrationV2{}, err
	}
	if conformance.FactoryRef != descriptor.Ref || conformance.DescriptorDigest != descriptor.DescriptorDigest {
		return contract.ComponentFactoryRegistrationV2{}, contract.NewError(contract.ErrorConflict, "component_factory_registry_conformance_drift", "component factory descriptor and conformance differ")
	}
	registration, err := contract.SealComponentFactoryRegistrationV2(descriptor, conformance)
	if err != nil {
		return contract.ComponentFactoryRegistrationV2{}, err
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.sealed {
		return contract.ComponentFactoryRegistrationV2{}, contract.NewError(contract.ErrorPrecondition, "component_registry_sealed", "component factory registry is sealed")
	}
	if _, exists := registry.entries[registration.Key.Digest]; exists {
		return contract.ComponentFactoryRegistrationV2{}, contract.NewError(contract.ErrorConflict, "component_factory_duplicate", "exact component factory registration already exists")
	}
	registry.entries[registration.Key.Digest] = componentFactoryEntryV2{registration: registration, factory: factory}
	return registration, nil
}

func (registry *ComponentRegistryV2) SealComponentFactoryRegistryV2(ctx context.Context) error {
	if registry == nil {
		return contract.NewError(contract.ErrorUnavailable, "component_registry_missing", "component factory registry is nil")
	}
	if ctx == nil {
		return contract.NewError(contract.ErrorInvalidArgument, "context_missing", "component factory registry requires a context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if len(registry.entries) == 0 {
		return contract.NewError(contract.ErrorPrecondition, "component_registry_empty", "component factory registry cannot seal empty")
	}
	registry.sealed = true
	return nil
}

func (registry *ComponentRegistryV2) InspectComponentFactoryRegistrationV2(
	ctx context.Context,
	expected contract.ComponentFactoryRegistryKeyV2,
) (contract.ComponentFactoryRegistrationV2, error) {
	entry, err := registry.inspectComponentFactoryEntryV2(ctx, expected)
	if err != nil {
		return contract.ComponentFactoryRegistrationV2{}, err
	}
	return entry.registration, nil
}

func (registry *ComponentRegistryV2) ResolveComponentFactoryV2(
	ctx context.Context,
	expected contract.ComponentFactoryRegistryKeyV2,
) (ports.ComponentFactoryV2, error) {
	entry, err := registry.inspectComponentFactoryEntryV2(ctx, expected)
	if err != nil {
		return nil, err
	}
	if contract.IsTypedNilV1(entry.factory) {
		return nil, contract.NewError(contract.ErrorUnavailable, "component_factory_missing", "component factory registration lost its executable")
	}
	currentDescriptor := entry.factory.DescriptorV2()
	if err := currentDescriptor.Validate(); err != nil {
		return nil, err
	}
	if currentDescriptor != entry.registration.Descriptor {
		return nil, contract.NewError(contract.ErrorConflict, "component_factory_descriptor_drift", "registered executable factory descriptor changed after registration")
	}
	return entry.factory, nil
}

func (registry *ComponentRegistryV2) inspectComponentFactoryEntryV2(
	ctx context.Context,
	expected contract.ComponentFactoryRegistryKeyV2,
) (componentFactoryEntryV2, error) {
	if registry == nil {
		return componentFactoryEntryV2{}, contract.NewError(contract.ErrorUnavailable, "component_registry_missing", "component factory registry is nil")
	}
	if ctx == nil {
		return componentFactoryEntryV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "component factory registry requires a context")
	}
	if err := ctx.Err(); err != nil {
		return componentFactoryEntryV2{}, err
	}
	if err := expected.Validate(); err != nil {
		return componentFactoryEntryV2{}, err
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if !registry.sealed {
		return componentFactoryEntryV2{}, contract.NewError(contract.ErrorPrecondition, "component_registry_unsealed", "component factory registry must be sealed")
	}
	entry, exists := registry.entries[expected.Digest]
	if !exists || entry.registration.Key != expected {
		return componentFactoryEntryV2{}, contract.NewError(contract.ErrorNotFound, "component_factory_not_bound", "exact component factory is not registered")
	}
	if err := entry.registration.Validate(); err != nil {
		return componentFactoryEntryV2{}, err
	}
	return entry, nil
}

var _ ports.ComponentFactoryRegistryV2 = (*ComponentRegistryV2)(nil)
