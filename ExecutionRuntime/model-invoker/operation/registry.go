package operation

import (
	"reflect"
	"sort"
	"strings"
	"sync"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

type Registry struct {
	mu        sync.RWMutex
	providers map[modelinvoker.ProviderID]Provider
}

func NewRegistry(providers ...Provider) (*Registry, error) {
	r := &Registry{providers: make(map[modelinvoker.ProviderID]Provider, len(providers))}
	for _, provider := range providers {
		if err := r.Register(provider); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) Register(provider Provider) error {
	if r == nil || nilProvider(provider) {
		return operationError("", modelinvoker.ErrorInvalidRequest, "register", "", "operation provider is nil")
	}
	id := provider.ID()
	if strings.TrimSpace(string(id)) == "" {
		return operationError("", modelinvoker.ErrorInvalidRequest, "register", "", "operation provider ID is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[id]; ok {
		return operationError(id, modelinvoker.ErrorDuplicateProvider, "register", "", "operation provider is already registered")
	}
	r.providers[id] = provider
	return nil
}

func (r *Registry) Get(id modelinvoker.ProviderID) (Provider, error) {
	if r == nil {
		return nil, operationError(id, modelinvoker.ErrorUnknownProvider, "resolve", "", "operation registry is nil")
	}
	r.mu.RLock()
	provider, ok := r.providers[id]
	r.mu.RUnlock()
	if !ok {
		return nil, operationError(id, modelinvoker.ErrorUnknownProvider, "resolve", "", "operation provider is not registered")
	}
	return provider, nil
}

func (r *Registry) IDs() []modelinvoker.ProviderID {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	ids := make([]modelinvoker.ProviderID, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	r.mu.RUnlock()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func nilProvider(provider Provider) bool {
	if provider == nil {
		return true
	}
	v := reflect.ValueOf(provider)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}
