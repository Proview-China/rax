package core_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	. "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

type fakeProvider struct {
	id               ProviderID
	defaultProtocol  Protocol
	capabilitiesFunc func(context.Context, CapabilityQuery) (CapabilityContract, error)
	invokeFunc       func(context.Context, Request) (Response, error)
	streamFunc       func(context.Context, Request) (Stream, error)
}

func (p *fakeProvider) ID() ProviderID { return p.id }

func (p *fakeProvider) DefaultProtocol() Protocol { return p.defaultProtocol }

func (p *fakeProvider) Capabilities(ctx context.Context, query CapabilityQuery) (CapabilityContract, error) {
	if p.capabilitiesFunc != nil {
		return p.capabilitiesFunc(ctx, query)
	}
	return nativeContract(), nil
}

func (p *fakeProvider) Invoke(ctx context.Context, request Request) (Response, error) {
	if p.invokeFunc != nil {
		return p.invokeFunc(ctx, request)
	}
	return Response{Status: ResponseStatusCompleted}, nil
}

func (p *fakeProvider) Stream(ctx context.Context, request Request) (Stream, error) {
	if p.streamFunc != nil {
		return p.streamFunc(ctx, request)
	}
	return &fakeStream{}, nil
}

func newFakeProvider(id ProviderID) *fakeProvider {
	return &fakeProvider{id: id, defaultProtocol: ProtocolResponses}
}

func TestRegistryRegisterResolveAndListDeterministically(t *testing.T) {
	registry, err := NewRegistry(newFakeProvider("zeta"), newFakeProvider("alpha"))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if err := registry.Register(newFakeProvider("middle")); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if got := registry.Len(); got != 3 {
		t.Fatalf("Len() = %d, want 3", got)
	}
	wantIDs := []ProviderID{"alpha", "middle", "zeta"}
	if got := registry.IDs(); !reflect.DeepEqual(got, wantIDs) {
		t.Fatalf("IDs() = %#v, want %#v", got, wantIDs)
	}
	provider, err := registry.Get("middle")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if provider.ID() != "middle" {
		t.Fatalf("Get().ID() = %q, want middle", provider.ID())
	}
}

func TestRegistryAcceptsEveryDeclaredProviderProtocol(t *testing.T) {
	protocols := []Protocol{ProtocolResponses, ProtocolChatCompletions, ProtocolMessages, ProtocolGenerateContent, ProtocolBedrockConverse, ProtocolBedrockInvoke}
	for _, protocol := range protocols {
		t.Run(string(protocol), func(t *testing.T) {
			provider := &fakeProvider{id: ProviderID(protocol), defaultProtocol: protocol}
			if _, err := NewRegistry(provider); err != nil {
				t.Fatalf("NewRegistry() protocol %q error = %v", protocol, err)
			}
		})
	}
}

func TestRegistryRejectsInvalidProvidersAndDuplicates(t *testing.T) {
	var typedNil *fakeProvider
	tests := []struct {
		name     string
		registry *Registry
		provider Provider
		wantKind ErrorKind
	}{
		{name: "nil registry", registry: nil, provider: newFakeProvider("provider"), wantKind: ErrorInvalidRequest},
		{name: "nil provider", registry: &Registry{}, provider: nil, wantKind: ErrorInvalidRequest},
		{name: "typed nil provider", registry: &Registry{}, provider: typedNil, wantKind: ErrorInvalidRequest},
		{name: "empty id", registry: &Registry{}, provider: newFakeProvider(" "), wantKind: ErrorInvalidRequest},
		{name: "invalid default protocol", registry: &Registry{}, provider: &fakeProvider{id: "provider", defaultProtocol: ProtocolAuto}, wantKind: ErrorInvalidRequest},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.registry.Register(test.provider)
			if ErrorKindOf(err) != test.wantKind {
				t.Fatalf("ErrorKindOf(Register()) = %q, want %q (err=%v)", ErrorKindOf(err), test.wantKind, err)
			}
		})
	}

	registry, err := NewRegistry(newFakeProvider("duplicate"))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	err = registry.Register(newFakeProvider("duplicate"))
	if ErrorKindOf(err) != ErrorDuplicateProvider {
		t.Fatalf("duplicate ErrorKind = %q, want %q", ErrorKindOf(err), ErrorDuplicateProvider)
	}
	if !errors.Is(err, &Error{Kind: ErrorDuplicateProvider, Provider: "duplicate"}) {
		t.Fatalf("duplicate error does not support errors.Is: %v", err)
	}

	if _, err := NewRegistry(newFakeProvider("same"), newFakeProvider("same")); ErrorKindOf(err) != ErrorDuplicateProvider {
		t.Fatalf("NewRegistry duplicate ErrorKind = %q, want %q", ErrorKindOf(err), ErrorDuplicateProvider)
	}
}

func TestRegistryUnknownAndNilReads(t *testing.T) {
	var nilRegistry *Registry
	if got := nilRegistry.Len(); got != 0 {
		t.Fatalf("nil Len() = %d, want 0", got)
	}
	if got := nilRegistry.IDs(); got != nil {
		t.Fatalf("nil IDs() = %#v, want nil", got)
	}
	if _, err := nilRegistry.Get("missing"); ErrorKindOf(err) != ErrorUnknownProvider {
		t.Fatalf("nil Get() ErrorKind = %q, want %q", ErrorKindOf(err), ErrorUnknownProvider)
	}

	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if _, err := registry.Get("missing"); ErrorKindOf(err) != ErrorUnknownProvider {
		t.Fatalf("missing Get() ErrorKind = %q, want %q", ErrorKindOf(err), ErrorUnknownProvider)
	}
}

func TestRegistryConcurrentRegisterGetAndList(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	const providers = 128
	start := make(chan struct{})
	errorsChannel := make(chan error, providers)
	var writers sync.WaitGroup
	for index := 0; index < providers; index++ {
		writers.Add(1)
		go func(index int) {
			defer writers.Done()
			<-start
			id := ProviderID(fmt.Sprintf("provider-%03d", index))
			if err := registry.Register(newFakeProvider(id)); err != nil {
				errorsChannel <- err
				return
			}
			provider, err := registry.Get(id)
			if err != nil {
				errorsChannel <- err
				return
			}
			if provider.ID() != id {
				errorsChannel <- fmt.Errorf("resolved %q, want %q", provider.ID(), id)
			}
		}(index)
	}

	readersDone := make(chan struct{})
	var readers sync.WaitGroup
	for index := 0; index < 8; index++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-readersDone:
					return
				default:
					_ = registry.Len()
					_ = registry.IDs()
				}
			}
		}()
	}

	close(start)
	writers.Wait()
	close(readersDone)
	readers.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		t.Errorf("concurrent registry operation: %v", err)
	}
	if got := registry.Len(); got != providers {
		t.Fatalf("Len() = %d, want %d", got, providers)
	}
	ids := registry.IDs()
	for index := 1; index < len(ids); index++ {
		if ids[index-1] >= ids[index] {
			t.Fatalf("IDs() not strictly sorted at %d: %#v", index, ids)
		}
	}
}
