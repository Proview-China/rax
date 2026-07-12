package routegateway

import (
	"reflect"
	"sort"
	"strings"
	"sync"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

type FactoryRegistry struct {
	mu        sync.RWMutex
	factories map[modelinvoker.ProviderID]AdapterFactory
}

func NewFactoryRegistry(factories ...AdapterFactory) (*FactoryRegistry, error) {
	registry := &FactoryRegistry{factories: make(map[modelinvoker.ProviderID]AdapterFactory, len(factories))}
	for _, factory := range factories {
		if err := registry.Register(factory); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *FactoryRegistry) Register(factory AdapterFactory) error {
	if r == nil || nilInterface(factory) {
		return gatewayError(modelinvoker.ErrorInvalidRequest, "factory_nil", "adapter factory is required", nil)
	}
	adapterID := factory.AdapterID()
	if strings.TrimSpace(string(adapterID)) == "" || !safeVersion(factory.ID()) || !safeVersion(factory.Version()) {
		return gatewayError(modelinvoker.ErrorInvalidRequest, "factory_invalid", "adapter factory ID, version, and AdapterID are required", nil)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.factories == nil {
		r.factories = make(map[modelinvoker.ProviderID]AdapterFactory)
	}
	if _, exists := r.factories[adapterID]; exists {
		return gatewayError(modelinvoker.ErrorDuplicateProvider, "factory_duplicate", "adapter factory is already registered", nil)
	}
	r.factories[adapterID] = factory
	return nil
}

func (r *FactoryRegistry) Get(adapterID modelinvoker.ProviderID) (AdapterFactory, error) {
	if r == nil {
		return nil, gatewayError(modelinvoker.ErrorUnknownProvider, "factory_registry_nil", "factory registry is not initialized", nil)
	}
	r.mu.RLock()
	factory, ok := r.factories[adapterID]
	r.mu.RUnlock()
	if !ok {
		return nil, gatewayError(modelinvoker.ErrorUnknownProvider, "factory_not_found", "catalog AdapterID has no registered factory", nil)
	}
	return factory, nil
}

func (r *FactoryRegistry) IDs() []modelinvoker.ProviderID {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	ids := make([]modelinvoker.ProviderID, 0, len(r.factories))
	for id := range r.factories {
		ids = append(ids, id)
	}
	r.mu.RUnlock()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func providerIdentityError(expected modelinvoker.ProviderID, provider modelinvoker.Provider) error {
	if nilInterface(provider) {
		return gatewayError(modelinvoker.ErrorProviderUnavailable, "factory_provider_nil", "adapter factory returned a nil provider", nil)
	}
	if provider.ID() != expected {
		return gatewayError(modelinvoker.ErrorMapping, "factory_provider_mismatch", "adapter factory returned a provider with the wrong identity", nil)
	}
	return nil
}
