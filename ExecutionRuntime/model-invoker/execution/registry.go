package execution

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type RegisteredAdapter struct {
	Descriptor AdapterDescriptor
	Adapter    Adapter
}

type Registry struct {
	mu       sync.RWMutex
	adapters map[string]RegisteredAdapter
}

func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]RegisteredAdapter)}
}

func (registry *Registry) Register(ctx context.Context, adapter Adapter) error {
	if registry == nil || adapter == nil || ctx == nil {
		return ErrInvalidAdapter
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	descriptor, err := adapter.Describe(ctx)
	if err != nil {
		return fmt.Errorf("describe execution adapter: %w", err)
	}
	if err := descriptor.Validate(); err != nil {
		return err
	}
	key := descriptor.Identity.ID
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.adapters == nil {
		registry.adapters = make(map[string]RegisteredAdapter)
	}
	if _, exists := registry.adapters[key]; exists {
		return fmt.Errorf("%w: adapter identity is immutable", ErrAdapterAlreadyRegistered)
	}
	descriptor.ExecutionKinds = append([]union.ExecutionKind(nil), descriptor.ExecutionKinds...)
	registry.adapters[key] = RegisteredAdapter{Descriptor: descriptor, Adapter: adapter}
	return nil
}

func (registry *Registry) Resolve(id string) (RegisteredAdapter, error) {
	if registry == nil {
		return RegisteredAdapter{}, ErrAdapterNotFound
	}
	registry.mu.RLock()
	registered, exists := registry.adapters[id]
	registry.mu.RUnlock()
	if !exists {
		return RegisteredAdapter{}, ErrAdapterNotFound
	}
	registered.Descriptor.ExecutionKinds = append([]union.ExecutionKind(nil), registered.Descriptor.ExecutionKinds...)
	return registered, nil
}

func (registry *Registry) Descriptors() []AdapterDescriptor {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	descriptors := make([]AdapterDescriptor, 0, len(registry.adapters))
	for _, registered := range registry.adapters {
		descriptor := registered.Descriptor
		descriptor.ExecutionKinds = append([]union.ExecutionKind(nil), descriptor.ExecutionKinds...)
		descriptors = append(descriptors, descriptor)
	}
	registry.mu.RUnlock()
	sort.Slice(descriptors, func(left, right int) bool {
		return descriptors[left].Identity.ID < descriptors[right].Identity.ID
	})
	return descriptors
}
