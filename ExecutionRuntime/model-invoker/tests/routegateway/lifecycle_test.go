package routegateway_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func TestFactoryFailureCanBeRetried(t *testing.T) {
	state := &callState{}
	failure := &failOnceFactory{fakeFactory: fakeFactory{id: "openai", state: state}}
	gateway := gatewayWithOverrideFactory(t, failure, state)
	defer gateway.Close()
	if _, err := gateway.Resolve(context.Background(), openAICall()); err == nil {
		t.Fatal("first factory failure error = nil")
	}
	if _, err := gateway.Resolve(context.Background(), openAICall()); err != nil {
		t.Fatalf("retry after factory failure: %v", err)
	}
	if failure.calls.Load() != 2 {
		t.Fatalf("factory calls = %d, want 2", failure.calls.Load())
	}
}

func TestSingleflightWaitHonorsDeadline(t *testing.T) {
	state := &callState{}
	entered, release := make(chan struct{}), make(chan struct{})
	factory := &blockingFactory{fakeFactory: fakeFactory{id: "openai", state: state}, entered: entered, release: release}
	gateway := gatewayWithOverrideFactory(t, factory, state)
	defer gateway.Close()
	creatorDone := make(chan error, 1)
	go func() {
		_, err := gateway.Resolve(context.Background(), openAICall())
		creatorDone <- err
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err := gateway.Resolve(ctx, openAICall())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait error = %v", err)
	}
	close(release)
	if err := <-creatorDone; err != nil {
		t.Fatal(err)
	}
	if factory.calls.Load() != 1 {
		t.Fatalf("factory calls = %d, want 1", factory.calls.Load())
	}
}

func TestGatewayCloseSurfacesRedactedAdapterCloseFailure(t *testing.T) {
	state := &callState{}
	secret := "CLOSE-MUST-NOT-LEAK"
	factory := closeErrorFactory{fakeFactory: fakeFactory{id: "openai", state: state}, closeErr: errors.New(secret)}
	gateway := gatewayWithOverrideFactory(t, factory, state)
	if _, err := gateway.Resolve(context.Background(), openAICall()); err != nil {
		t.Fatal(err)
	}
	err := gateway.Close()
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe or missing Close error: %v", err)
	}
}

func TestFactoryRegistryRejectsTypedNil(t *testing.T) {
	var factory *typedNilFactory
	if _, err := routegateway.NewFactoryRegistry(factory); err == nil {
		t.Fatal("typed-nil factory error = nil")
	}
}

func FuzzSecretMaterialFormattingNeverLeaks(f *testing.F) {
	f.Add([]byte("secret-value"), "version-1")
	f.Fuzz(func(t *testing.T, secret []byte, version string) {
		if len(secret) < 4 || strings.TrimSpace(version) == "" || strings.ContainsAny(version, "\r\n\x00") || len(version) > 256 {
			t.Skip()
		}
		material, err := routegateway.NewSecretMaterial("test.profile", upstream.CredentialAPIKey, version, time.Time{}, map[upstream.CredentialPurpose][]byte{upstream.CredentialPurposeAPIKey: secret})
		if err != nil {
			t.Skip()
		}
		formatted := fmt.Sprintf("%v %#v %+v", material, material, material)
		if strings.Contains(formatted, string(secret)) {
			t.Fatal("secret appeared in formatted material")
		}
	})
}

type failOnceFactory struct {
	fakeFactory
	calls atomic.Int64
}

func (f *failOnceFactory) Build(ctx context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	if f.calls.Add(1) == 1 {
		return routegateway.FactoryResult{}, errors.New("temporary failure")
	}
	return f.fakeFactory.Build(ctx, input)
}

type blockingFactory struct {
	fakeFactory
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	calls   atomic.Int64
}

func (f *blockingFactory) Build(ctx context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	f.calls.Add(1)
	f.once.Do(func() { close(f.entered) })
	select {
	case <-ctx.Done():
		return routegateway.FactoryResult{}, ctx.Err()
	case <-f.release:
	}
	return f.fakeFactory.Build(ctx, input)
}

type closeErrorFactory struct {
	fakeFactory
	closeErr error
}

func (f closeErrorFactory) Build(_ context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	f.state.factory.Add(1)
	return routegateway.FactoryResult{Provider: &fakeProvider{id: f.id, state: f.state}, Closer: closeError{err: f.closeErr}, Endpoint: input.Endpoint}, nil
}

type closeError struct{ err error }

func (c closeError) Close() error { return c.err }

type typedNilFactory struct{}

func (*typedNilFactory) ID() string                         { return "nil" }
func (*typedNilFactory) Version() string                    { return "v1" }
func (*typedNilFactory) AdapterID() modelinvoker.ProviderID { return "nil" }
func (*typedNilFactory) Build(context.Context, routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	return routegateway.FactoryResult{}, nil
}

func gatewayWithOverrideFactory(t *testing.T, override routegateway.AdapterFactory, state *callState) *routegateway.Gateway {
	t.Helper()
	routeCatalog := defaultCatalog(t)
	builtins, err := routegateway.NewBuiltinFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	factories := make([]routegateway.AdapterFactory, 0, len(builtins.IDs()))
	for _, id := range builtins.IDs() {
		if id == override.AdapterID() {
			factories = append(factories, override)
		} else {
			factories = append(factories, fakeFactory{id: id, state: state})
		}
	}
	registry, err := routegateway.NewFactoryRegistry(factories...)
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := routegateway.New(routeCatalog, countingBinding{state: state}, countingSecret{state: state, version: "v1"}, registry, routegateway.WithClock(func() time.Time { return gatewayNow }))
	if err != nil {
		t.Fatal(err)
	}
	return gateway
}
