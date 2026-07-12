package routegateway_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
)

func TestTrustedBuiltinCandidateRejectionSurvivesGatewayWithoutDoubleCloseOrLeak(t *testing.T) {
	const sentinel = "CANDIDATE-CLOSE-SECRET-MUST-NOT-LEAK"
	closeFailure := errors.New(sentinel)
	state := &callState{}
	factory := &candidateRejectFactory{fakeFactory: fakeFactory{id: "openai", state: state}, closeErr: closeFailure}
	gateway := gatewayWithOverrideFactory(t, factory, state)
	defer gateway.Close()

	_, err := gateway.Resolve(context.Background(), openAICall())
	if err == nil {
		t.Fatal("candidate rejection error = nil")
	}
	assertGatewayErrorCode(t, err, "factory_endpoint_receipt_missing")
	if !errors.Is(err, closeFailure) {
		t.Fatalf("Gateway candidate rejection lost close cause: %v", err)
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("Gateway candidate rejection leaked close cause: %v", err)
	}
	if factory.closes.Load() != 1 {
		t.Fatalf("candidate Close calls = %d, want 1", factory.closes.Load())
	}
}

func TestUntrustedFactoryBuildErrorIsHiddenButReturnedCloserIsClosedOnce(t *testing.T) {
	const sentinel = "UNTRUSTED-FACTORY-BUILD-SECRET"
	state := &callState{}
	factory := &untrustedBuildFailureFactory{fakeFactory: fakeFactory{id: "openai", state: state}, buildErr: errors.New(sentinel)}
	gateway := gatewayWithOverrideFactory(t, factory, state)
	defer gateway.Close()

	_, err := gateway.Resolve(context.Background(), openAICall())
	if err == nil || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("untrusted build error missing or leaked: %v", err)
	}
	assertGatewayErrorCode(t, err, "factory_build_failed")
	if factory.closes.Load() != 1 {
		t.Fatalf("untrusted build result Close calls = %d, want 1", factory.closes.Load())
	}
}

func TestBuildErrorDerivesProviderCloserAndClosesOnceWithoutLeak(t *testing.T) {
	const sentinel = "DERIVED-PROVIDER-CLOSE-SECRET-MUST-NOT-LEAK"
	closeFailure := errors.New(sentinel)
	state := &callState{}
	factory := &providerCloserBuildFailureFactory{
		fakeFactory: fakeFactory{id: "openai", state: state},
		buildErr:    errors.New("UNTRUSTED-BUILD-ERROR-SECRET"), closeErr: closeFailure,
	}
	gateway := gatewayWithOverrideFactory(t, factory, state)
	defer gateway.Close()

	_, err := gateway.Resolve(context.Background(), openAICall())
	if err == nil || !errors.Is(err, closeFailure) {
		t.Fatalf("build failure lost derived Provider Close cause: %v", err)
	}
	assertGatewayErrorCode(t, err, "factory_build_failed")
	if strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), "UNTRUSTED-BUILD-ERROR-SECRET") {
		t.Fatalf("build failure leaked Provider/Build cause: %v", err)
	}
	if factory.closes.Load() != 1 {
		t.Fatalf("derived Provider Close calls = %d, want 1", factory.closes.Load())
	}
}

func TestPostBuildCallerCancellationAndDeadlineCloseLateResultOnce(t *testing.T) {
	for _, test := range []struct {
		name string
		ctx  func() (context.Context, context.CancelFunc)
		end  func(context.Context, context.CancelFunc)
		want error
	}{
		{
			name: "cancellation", ctx: func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
			end: func(_ context.Context, cancel context.CancelFunc) { cancel() }, want: context.Canceled,
		},
		{
			name: "deadline", ctx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 20*time.Millisecond)
			},
			end: func(ctx context.Context, _ context.CancelFunc) { <-ctx.Done() }, want: context.DeadlineExceeded,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			const sentinel = "LATE-BUILD-CLOSE-SECRET-MUST-NOT-LEAK"
			state := &callState{}
			entered, release := make(chan struct{}), make(chan struct{})
			closeFailure := errors.New(sentinel)
			closeCalls := &atomic.Int64{}
			factory := &blockingBuildFactory{
				fakeFactory: fakeFactory{id: "openai", state: state}, entered: entered, release: release,
				closer: &countingFailureCloser{calls: closeCalls, err: closeFailure},
			}
			gateway := gatewayWithOverrideFactory(t, factory, state)
			ctx, cancel := test.ctx()
			defer cancel()
			result := make(chan error, 1)
			go func() {
				_, err := gateway.Resolve(ctx, openAICall())
				result <- err
			}()
			<-entered
			test.end(ctx, cancel)
			close(release)
			err := <-result
			if err == nil || !errors.Is(err, test.want) || !errors.Is(err, closeFailure) {
				t.Fatalf("post-build %s error lost context/close cause: %v", test.name, err)
			}
			if strings.Contains(err.Error(), sentinel) {
				t.Fatalf("post-build %s leaked close cause: %v", test.name, err)
			}
			if closeCalls.Load() != 1 {
				t.Fatalf("post-build %s Close calls = %d, want 1", test.name, closeCalls.Load())
			}
			if err := gateway.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProviderCallFailureJoinsStaleLeaseCloseFailure(t *testing.T) {
	for _, operation := range []string{"capabilities", "invoke"} {
		t.Run(operation, func(t *testing.T) {
			const sentinel = "CALL-RELEASE-CLOSE-SECRET-MUST-NOT-LEAK"
			state := &callState{}
			secret := &rotatingSecret{state: state, version: "call-release-v1"}
			factory := &blockingOperationFactory{
				fakeFactory: fakeFactory{id: "openai", state: state}, operation: operation,
				entered: make(chan struct{}), release: make(chan struct{}), closeErr: errors.New(sentinel),
			}
			gateway := gatewayWithOverrideFactoryAndSecret(t, factory, state, secret)
			result := make(chan error, 1)
			go func() {
				if operation == "capabilities" {
					_, err := gateway.Capabilities(context.Background(), openAICall())
					result <- err
					return
				}
				_, err := gateway.Invoke(context.Background(), openAICall())
				result <- err
			}()
			<-factory.entered
			secret.setVersion("call-release-v2")
			if _, err := gateway.Resolve(context.Background(), openAICall()); err != nil {
				t.Fatal(err)
			}
			close(factory.release)
			err := <-result
			if err == nil || !errors.Is(err, factory.closeErr) {
				t.Fatalf("%s failure lost stale lease close cause: %v", operation, err)
			}
			assertGatewayErrorCode(t, err, "provider_call_failed")
			if strings.Contains(err.Error(), sentinel) || strings.Contains(err.Error(), "CALL-FAILURE-SECRET") {
				t.Fatalf("%s failure leaked call/close cause: %v", operation, err)
			}
			if factory.closeCalls.Load() != 1 {
				t.Fatalf("%s stale adapter Close calls = %d, want 1", operation, factory.closeCalls.Load())
			}
			if err := gateway.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRotationCloseFailureIsAggregatedByGatewayCloseWithoutLeak(t *testing.T) {
	state := &callState{}
	secret := &rotatingSecret{state: state, version: "rotation-v1"}
	closeFailure := errors.New("ROTATION-SECRET-MUST-NOT-LEAK")
	factory := &rotationCloseFactory{fakeFactory: fakeFactory{id: "openai", state: state}, firstCloseErr: closeFailure}
	gateway := gatewayWithOverrideFactoryAndSecret(t, factory, state, secret)
	if _, err := gateway.Resolve(context.Background(), openAICall()); err != nil {
		t.Fatal(err)
	}
	secret.setVersion("rotation-v2")
	if _, err := gateway.Resolve(context.Background(), openAICall()); err != nil {
		t.Fatalf("rotation should continue after recording the close failure: %v", err)
	}
	err := gateway.Close()
	if err == nil || !errors.Is(err, closeFailure) || strings.Contains(err.Error(), "ROTATION-SECRET-MUST-NOT-LEAK") {
		t.Fatalf("rotation close failure was lost or leaked: %v", err)
	}
	if factory.firstCloseCalls.Load() != 1 {
		t.Fatalf("rotated adapter close calls = %d, want 1", factory.firstCloseCalls.Load())
	}
}

func TestConcurrentGatewayCloseWaitsForRotationCloseFailure(t *testing.T) {
	state := &callState{}
	secret := &rotatingSecret{state: state, version: "concurrent-v1"}
	entered, release := make(chan struct{}), make(chan struct{})
	factory := &blockingRotationCloseFactory{
		fakeFactory: fakeFactory{id: "openai", state: state}, entered: entered, release: release,
		closeErr: errors.New("CONCURRENT-ROTATION-SECRET-MUST-NOT-LEAK"),
	}
	gateway := gatewayWithOverrideFactoryAndSecret(t, factory, state, secret)
	if _, err := gateway.Resolve(context.Background(), openAICall()); err != nil {
		t.Fatal(err)
	}
	secret.setVersion("concurrent-v2")
	resolveDone := make(chan error, 1)
	go func() {
		_, err := gateway.Resolve(context.Background(), openAICall())
		resolveDone <- err
	}()
	<-entered
	closeDone := make(chan error, 1)
	go func() { closeDone <- gateway.Close() }()
	select {
	case err := <-closeDone:
		t.Fatalf("Gateway.Close returned before in-flight rotation close: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	closeErr := <-closeDone
	if closeErr == nil || strings.Contains(closeErr.Error(), "CONCURRENT-ROTATION-SECRET-MUST-NOT-LEAK") {
		t.Fatalf("concurrent rotation close failure was lost or leaked: %v", closeErr)
	}
	if err := <-resolveDone; err == nil {
		t.Fatal("rotation Resolve succeeded after concurrent Gateway.Close")
	}
	if factory.builds.Load() != 1 {
		t.Fatalf("factory builds after concurrent close = %d, want 1", factory.builds.Load())
	}
}

func TestConcurrentGatewayCloseWaitsForInFlightFactoryBuildAndAggregatesCloseFailure(t *testing.T) {
	state := &callState{}
	entered, release := make(chan struct{}), make(chan struct{})
	closeCalls := &atomic.Int64{}
	factory := &blockingBuildFactory{
		fakeFactory: fakeFactory{id: "openai", state: state}, entered: entered, release: release,
		closer: &countingFailureCloser{calls: closeCalls, err: errors.New("BUILD-CLOSE-SECRET-MUST-NOT-LEAK")},
	}
	gateway := gatewayWithOverrideFactory(t, factory, state)
	resolveDone := make(chan error, 1)
	go func() {
		_, err := gateway.Resolve(context.Background(), openAICall())
		resolveDone <- err
	}()
	<-entered
	closeDone := make(chan error, 1)
	go func() { closeDone <- gateway.Close() }()
	select {
	case err := <-closeDone:
		t.Fatalf("Gateway.Close returned before in-flight factory build: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	closeErr := <-closeDone
	if closeErr == nil || strings.Contains(closeErr.Error(), "BUILD-CLOSE-SECRET-MUST-NOT-LEAK") {
		t.Fatalf("in-flight build close failure was lost or leaked: %v", closeErr)
	}
	if err := <-resolveDone; err == nil {
		t.Fatal("factory-backed Resolve succeeded after concurrent Gateway.Close")
	}
	if closeCalls.Load() != 1 {
		t.Fatalf("late factory result close calls = %d, want 1", closeCalls.Load())
	}
}

func TestConcurrentStaleLeaseReleaseClosesOnceAndReturnsSafeFailure(t *testing.T) {
	state := &callState{}
	secret := &rotatingSecret{state: state, version: "lease-v1"}
	factory := &rotationCloseFactory{fakeFactory: fakeFactory{id: "openai", state: state}, firstCloseErr: errors.New("LEASE-SECRET-MUST-NOT-LEAK")}
	gateway := gatewayWithOverrideFactoryAndSecret(t, factory, state, secret)
	const streams = 16
	values := make([]*routegateway.Stream, 0, streams)
	for index := 0; index < streams; index++ {
		stream, err := gateway.Stream(context.Background(), openAICall())
		if err != nil {
			t.Fatal(err)
		}
		values = append(values, stream)
	}
	secret.setVersion("lease-v2")
	if _, err := gateway.Resolve(context.Background(), openAICall()); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, streams)
	var wait sync.WaitGroup
	for _, stream := range values {
		wait.Add(1)
		go func(stream *routegateway.Stream) {
			defer wait.Done()
			results <- stream.Close()
		}(stream)
	}
	wait.Wait()
	close(results)
	failures := 0
	for err := range results {
		if err == nil {
			continue
		}
		failures++
		if strings.Contains(err.Error(), "LEASE-SECRET-MUST-NOT-LEAK") {
			t.Fatalf("lease close failure leaked: %v", err)
		}
	}
	if failures != 1 || factory.firstCloseCalls.Load() != 1 {
		t.Fatalf("close failures/calls = %d/%d, want 1/1", failures, factory.firstCloseCalls.Load())
	}
	if err := gateway.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidFactoryEndpointNeverEntersPoolAndFailedResultIsClosed(t *testing.T) {
	for name, invalidEndpoint := range map[string]string{
		"cross_host":      "https://attacker.invalid/v1",
		"cross_base_path": "https://api.openai.com/other",
		"encoded_escape":  "https://api.openai.com/v1/%2e%2e/private",
	} {
		t.Run(name, func(t *testing.T) {
			state := &callState{}
			factory := &endpointSequenceFactory{
				fakeFactory: fakeFactory{id: "openai", state: state},
				endpoints:   []string{invalidEndpoint, ""},
				closeErrors: []error{errors.New("INVALID-CLOSE-SECRET"), nil},
			}
			gateway := gatewayWithOverrideFactory(t, factory, state)
			if _, err := gateway.Resolve(context.Background(), openAICall()); err == nil || strings.Contains(err.Error(), "INVALID-CLOSE-SECRET") {
				t.Fatalf("invalid endpoint error missing or leaked: %v", err)
			}
			if factory.calls.Load() != 1 || factory.closes.Load() != 1 {
				t.Fatalf("invalid result calls/closes = %d/%d, want 1/1", factory.calls.Load(), factory.closes.Load())
			}
			if _, err := gateway.Resolve(context.Background(), openAICall()); err != nil {
				t.Fatalf("valid retry after rejected result: %v", err)
			}
			if factory.calls.Load() != 2 {
				t.Fatalf("rejected adapter polluted pool; factory calls = %d, want 2", factory.calls.Load())
			}
			if err := gateway.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFactoryWithoutLifecycleCloserNeverEntersPool(t *testing.T) {
	state := &callState{}
	factory := &missingCloserFactory{fakeFactory: fakeFactory{id: "openai", state: state}}
	gateway := gatewayWithOverrideFactory(t, factory, state)
	if _, err := gateway.Resolve(context.Background(), openAICall()); err == nil {
		t.Fatal("missing lifecycle closer error = nil")
	}
	if factory.calls.Load() != 1 {
		t.Fatalf("missing-closer factory calls = %d, want 1", factory.calls.Load())
	}
	if _, err := gateway.Resolve(context.Background(), openAICall()); err != nil {
		t.Fatalf("valid retry after missing closer: %v", err)
	}
	if factory.calls.Load() != 2 {
		t.Fatalf("missing-closer adapter polluted pool; factory calls = %d, want 2", factory.calls.Load())
	}
	if err := gateway.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentWaitersCannotObserveInvalidFactoryEndpoint(t *testing.T) {
	state := &callState{}
	factory := &switchEndpointFactory{fakeFactory: fakeFactory{id: "openai", state: state}}
	factory.invalid.Store(true)
	gateway := gatewayWithOverrideFactory(t, factory, state)
	const workers = 24
	start := make(chan struct{})
	results := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := gateway.Capabilities(context.Background(), openAICall())
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	for err := range results {
		if err == nil {
			t.Fatal("concurrent waiter observed an invalid cached adapter")
		}
	}
	if state.capabilities.Load() != 0 {
		t.Fatalf("invalid adapter reached Provider %d times", state.capabilities.Load())
	}
	factory.invalid.Store(false)
	if _, err := gateway.Capabilities(context.Background(), openAICall()); err != nil {
		t.Fatalf("valid construction after concurrent rejection: %v", err)
	}
	if state.capabilities.Load() != 1 {
		t.Fatalf("valid Provider capability calls = %d, want 1", state.capabilities.Load())
	}
	if err := gateway.Close(); err != nil {
		t.Fatal(err)
	}
}

type rotationCloseFactory struct {
	fakeFactory
	builds          atomic.Int64
	firstCloseCalls atomic.Int64
	firstCloseErr   error
}

type candidateRejectFactory struct {
	fakeFactory
	closeErr error
	closes   atomic.Int64
}

func (factory *candidateRejectFactory) Build(ctx context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	factory.state.factory.Add(1)
	provider := &candidateRejectProvider{fakeProvider: &fakeProvider{id: factory.id, state: factory.state}, closeErr: factory.closeErr, closes: &factory.closes}
	_, err := adaptercore.FinalizeCandidateBinding(ctx, factory.id, modelinvoker.Protocol(input.Entry.Route.Protocol.ID), input.Endpoint, provider, nil)
	return routegateway.FactoryResult{}, err
}

type candidateRejectProvider struct {
	*fakeProvider
	closeErr error
	closes   *atomic.Int64
}

func (provider *candidateRejectProvider) Close() error {
	provider.closes.Add(1)
	return provider.closeErr
}

type untrustedBuildFailureFactory struct {
	fakeFactory
	buildErr error
	closes   atomic.Int64
}

type providerCloserBuildFailureFactory struct {
	fakeFactory
	buildErr error
	closeErr error
	closes   atomic.Int64
}

func (factory *providerCloserBuildFailureFactory) Build(_ context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	factory.state.factory.Add(1)
	provider := &providerBuildFailureCloser{
		fakeProvider: &fakeProvider{id: factory.id, state: factory.state}, closeErr: factory.closeErr, closes: &factory.closes,
	}
	return routegateway.FactoryResult{Provider: provider, Endpoint: input.Endpoint}, factory.buildErr
}

type providerBuildFailureCloser struct {
	*fakeProvider
	closeErr error
	closes   *atomic.Int64
}

func (provider *providerBuildFailureCloser) Close() error {
	provider.closes.Add(1)
	return provider.closeErr
}

func (factory *untrustedBuildFailureFactory) Build(_ context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	factory.state.factory.Add(1)
	return routegateway.FactoryResult{
		Provider: &fakeProvider{id: factory.id, state: factory.state},
		Closer:   closeCounter{calls: &factory.closes}, Endpoint: input.Endpoint,
	}, factory.buildErr
}

type blockingRotationCloseFactory struct {
	fakeFactory
	builds   atomic.Int64
	entered  chan struct{}
	release  chan struct{}
	closeErr error
}

type blockingBuildFactory struct {
	fakeFactory
	entered chan struct{}
	release chan struct{}
	closer  interface{ Close() error }
}

type blockingOperationFactory struct {
	fakeFactory
	operation  string
	entered    chan struct{}
	release    chan struct{}
	closeErr   error
	builds     atomic.Int64
	closeCalls atomic.Int64
}

func (factory *blockingOperationFactory) Build(_ context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	factory.state.factory.Add(1)
	if factory.builds.Add(1) == 1 {
		provider := &blockingOperationProvider{
			fakeProvider: &fakeProvider{id: factory.id, state: factory.state}, operation: factory.operation,
			entered: factory.entered, release: factory.release,
		}
		return routegateway.FactoryResult{
			Provider: provider, Closer: &countingFailureCloser{calls: &factory.closeCalls, err: factory.closeErr}, Endpoint: input.Endpoint,
		}, nil
	}
	return routegateway.FactoryResult{
		Provider: &fakeProvider{id: factory.id, state: factory.state}, Closer: countCloser{state: factory.state}, Endpoint: input.Endpoint,
	}, nil
}

type blockingOperationProvider struct {
	*fakeProvider
	operation string
	entered   chan struct{}
	release   chan struct{}
}

func (provider *blockingOperationProvider) wait() error {
	close(provider.entered)
	<-provider.release
	return &modelinvoker.Error{
		Kind: modelinvoker.ErrorProvider, Code: "provider_call_failed", Message: "CALL-FAILURE-SECRET",
	}
}

func (provider *blockingOperationProvider) Capabilities(ctx context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	if provider.operation == "capabilities" {
		return nil, provider.wait()
	}
	return provider.fakeProvider.Capabilities(ctx, query)
}

func (provider *blockingOperationProvider) Invoke(ctx context.Context, request modelinvoker.Request) (modelinvoker.Response, error) {
	if provider.operation == "invoke" {
		return modelinvoker.Response{}, provider.wait()
	}
	return provider.fakeProvider.Invoke(ctx, request)
}

func (factory *blockingBuildFactory) Build(_ context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	factory.state.factory.Add(1)
	close(factory.entered)
	<-factory.release
	return routegateway.FactoryResult{
		Provider: &fakeProvider{id: factory.id, state: factory.state}, Closer: factory.closer, Endpoint: input.Endpoint,
	}, nil
}

func (factory *blockingRotationCloseFactory) Build(_ context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	factory.state.factory.Add(1)
	build := factory.builds.Add(1)
	var closer interface{ Close() error } = countCloser{state: factory.state}
	if build == 1 {
		closer = &blockingFailureCloser{entered: factory.entered, release: factory.release, err: factory.closeErr}
	}
	return routegateway.FactoryResult{Provider: &fakeProvider{id: factory.id, state: factory.state}, Closer: closer, Endpoint: input.Endpoint}, nil
}

type blockingFailureCloser struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}
	err     error
}

func (closer *blockingFailureCloser) Close() error {
	closer.once.Do(func() { close(closer.entered) })
	<-closer.release
	return closer.err
}

func (factory *rotationCloseFactory) Build(_ context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	factory.state.factory.Add(1)
	build := factory.builds.Add(1)
	closer := countCloser{state: factory.state}
	var resultCloser interface{ Close() error } = closer
	if build == 1 {
		resultCloser = &countingFailureCloser{calls: &factory.firstCloseCalls, err: factory.firstCloseErr}
	}
	return routegateway.FactoryResult{Provider: &fakeProvider{id: factory.id, state: factory.state}, Closer: resultCloser, Endpoint: input.Endpoint}, nil
}

type countingFailureCloser struct {
	calls *atomic.Int64
	err   error
}

func (closer *countingFailureCloser) Close() error {
	closer.calls.Add(1)
	return closer.err
}

type endpointSequenceFactory struct {
	fakeFactory
	mu          sync.Mutex
	endpoints   []string
	closeErrors []error
	calls       atomic.Int64
	closes      atomic.Int64
}

func (factory *endpointSequenceFactory) Build(_ context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	factory.state.factory.Add(1)
	index := int(factory.calls.Add(1)) - 1
	factory.mu.Lock()
	endpoint := input.Endpoint
	if index < len(factory.endpoints) && factory.endpoints[index] != "" {
		endpoint = factory.endpoints[index]
	}
	var closeErr error
	if index < len(factory.closeErrors) {
		closeErr = factory.closeErrors[index]
	}
	factory.mu.Unlock()
	return routegateway.FactoryResult{
		Provider: &fakeProvider{id: factory.id, state: factory.state},
		Closer:   closeCounter{calls: &factory.closes, err: closeErr}, Endpoint: endpoint,
	}, nil
}

type closeCounter struct {
	calls *atomic.Int64
	err   error
}

func (closer closeCounter) Close() error { closer.calls.Add(1); return closer.err }

type switchEndpointFactory struct {
	fakeFactory
	invalid atomic.Bool
}

type missingCloserFactory struct {
	fakeFactory
	calls atomic.Int64
}

func (factory *missingCloserFactory) Build(_ context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	factory.state.factory.Add(1)
	if factory.calls.Add(1) == 1 {
		return routegateway.FactoryResult{Provider: &fakeProvider{id: factory.id, state: factory.state}, Endpoint: input.Endpoint}, nil
	}
	return routegateway.FactoryResult{Provider: &fakeProvider{id: factory.id, state: factory.state}, Closer: countCloser{state: factory.state}, Endpoint: input.Endpoint}, nil
}

func (factory *switchEndpointFactory) Build(_ context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	factory.state.factory.Add(1)
	time.Sleep(2 * time.Millisecond)
	endpoint := input.Endpoint
	if factory.invalid.Load() {
		endpoint = "https://attacker.invalid/v1"
	}
	return routegateway.FactoryResult{Provider: &fakeProvider{id: factory.id, state: factory.state}, Closer: countCloser{state: factory.state}, Endpoint: endpoint}, nil
}

func gatewayWithOverrideFactoryAndSecret(t *testing.T, override routegateway.AdapterFactory, state *callState, secret routegateway.SecretResolver) *routegateway.Gateway {
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
	gateway, err := routegateway.New(routeCatalog, countingBinding{state: state}, secret, registry, routegateway.WithClock(func() time.Time { return gatewayNow }))
	if err != nil {
		t.Fatal(err)
	}
	return gateway
}
