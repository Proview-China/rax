package realtime

import (
	"reflect"
	"sort"
	"strings"
	"sync"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

// Registry is the provider-neutral selection boundary for realtime sessions.
// It prevents callers from reaching a concrete WebSocket implementation as
// the canonical invocation path.
type Registry struct {
	mu        sync.RWMutex
	providers map[modelinvoker.ProviderID]Provider
}

func NewRegistry(providers ...Provider) (*Registry, error) {
	registry := &Registry{providers: make(map[modelinvoker.ProviderID]Provider, len(providers))}
	for _, provider := range providers {
		if err := registry.Register(provider); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (registry *Registry) Register(provider Provider) error {
	if registry == nil || nilInterface(provider) {
		return realtimeError("", modelinvoker.ErrorInvalidRequest, "register", "realtime provider is nil", nil)
	}
	id := provider.ID()
	if strings.TrimSpace(string(id)) == "" {
		return realtimeError("", modelinvoker.ErrorInvalidRequest, "register", "realtime provider ID is required", nil)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.providers[id]; exists {
		return realtimeError(id, modelinvoker.ErrorDuplicateProvider, "register", "realtime provider is already registered", nil)
	}
	registry.providers[id] = provider
	return nil
}

func (registry *Registry) Get(id modelinvoker.ProviderID) (Provider, error) {
	if registry == nil {
		return nil, realtimeError(id, modelinvoker.ErrorUnknownProvider, "resolve", "realtime registry is nil", nil)
	}
	registry.mu.RLock()
	provider, ok := registry.providers[id]
	registry.mu.RUnlock()
	if !ok {
		return nil, realtimeError(id, modelinvoker.ErrorUnknownProvider, "resolve", "realtime provider is not registered", nil)
	}
	return provider, nil
}

func (registry *Registry) IDs() []modelinvoker.ProviderID {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	ids := make([]modelinvoker.ProviderID, 0, len(registry.providers))
	for id := range registry.providers {
		ids = append(ids, id)
	}
	registry.mu.RUnlock()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflection := reflect.ValueOf(value)
	switch reflection.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflection.IsNil()
	default:
		return false
	}
}
