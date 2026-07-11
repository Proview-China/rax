package modelinvoker

import (
	"reflect"
	"sort"
	"strings"
	"sync"
)

type Registry struct {
	mu        sync.RWMutex
	providers map[ProviderID]Provider
}

func NewRegistry(providers ...Provider) (*Registry, error) {
	registry := &Registry{providers: make(map[ProviderID]Provider, len(providers))}
	for _, provider := range providers {
		if err := registry.Register(provider); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *Registry) Register(provider Provider) error {
	if r == nil {
		return &Error{Kind: ErrorInvalidRequest, Operation: "register_provider", Message: "registry is nil"}
	}
	if nilProvider(provider) {
		return &Error{Kind: ErrorInvalidRequest, Operation: "register_provider", Message: "provider is nil"}
	}
	id := provider.ID()
	if strings.TrimSpace(string(id)) == "" {
		return &Error{Kind: ErrorInvalidRequest, Operation: "register_provider", Message: "provider ID is required"}
	}
	protocol := provider.DefaultProtocol()
	if !protocol.valid() {
		return &Error{Kind: ErrorInvalidRequest, Provider: id, Operation: "register_provider", Message: "provider default protocol is invalid"}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.providers == nil {
		r.providers = make(map[ProviderID]Provider)
	}
	if _, exists := r.providers[id]; exists {
		return &Error{Kind: ErrorDuplicateProvider, Provider: id, Operation: "register_provider", Message: "provider is already registered"}
	}
	r.providers[id] = provider
	return nil
}

func (r *Registry) Get(id ProviderID) (Provider, error) {
	if r == nil {
		return nil, &Error{Kind: ErrorUnknownProvider, Provider: id, Operation: "resolve_provider", Message: "registry is nil"}
	}
	r.mu.RLock()
	provider, ok := r.providers[id]
	r.mu.RUnlock()
	if !ok {
		return nil, &Error{Kind: ErrorUnknownProvider, Provider: id, Operation: "resolve_provider", Message: "provider is not registered"}
	}
	return provider, nil
}

func (r *Registry) IDs() []ProviderID {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	ids := make([]ProviderID, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	r.mu.RUnlock()
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	length := len(r.providers)
	r.mu.RUnlock()
	return length
}

func nilProvider(provider Provider) bool {
	if provider == nil {
		return true
	}
	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
