package core_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	. "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

type fakeStream struct {
	events     []StreamEvent
	index      int
	nextCalls  int
	err        error
	closeCalls int
}

func (s *fakeStream) Next() bool {
	s.nextCalls++
	if s.index >= len(s.events) {
		return false
	}
	s.index++
	return true
}

func (s *fakeStream) Event() StreamEvent {
	if s.index == 0 || s.index > len(s.events) {
		return StreamEvent{}
	}
	return s.events[s.index-1]
}

func (s *fakeStream) Err() error { return s.err }

func (s *fakeStream) Close() error {
	s.closeCalls++
	return nil
}

func newTestInvoker(t *testing.T, provider Provider, options ...InvokerOption) *Invoker {
	t.Helper()
	registry, err := NewRegistry(provider)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	invoker, err := NewInvoker(registry, options...)
	if err != nil {
		t.Fatalf("NewInvoker() error = %v", err)
	}
	return invoker
}

func TestNewInvokerValidatesConfiguration(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	tests := []struct {
		name     string
		registry *Registry
		options  []InvokerOption
	}{
		{name: "nil registry"},
		{name: "nil option", registry: registry, options: []InvokerOption{nil}},
		{name: "nil sleeper", registry: registry, options: []InvokerOption{WithSleeper(nil)}},
		{name: "zero attempts", registry: registry, options: []InvokerOption{WithRetryPolicy(RetryPolicy{Multiplier: 2})}},
		{name: "negative initial backoff", registry: registry, options: []InvokerOption{WithRetryPolicy(RetryPolicy{MaxAttempts: 1, InitialBackoff: -1, Multiplier: 2})}},
		{name: "initial exceeds maximum", registry: registry, options: []InvokerOption{WithRetryPolicy(RetryPolicy{MaxAttempts: 1, InitialBackoff: 2, MaxBackoff: 1, Multiplier: 2})}},
		{name: "small multiplier", registry: registry, options: []InvokerOption{WithRetryPolicy(RetryPolicy{MaxAttempts: 1, Multiplier: .5})}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewInvoker(test.registry, test.options...)
			if ErrorKindOf(err) != ErrorInvalidRequest {
				t.Fatalf("ErrorKindOf(NewInvoker()) = %q, want %q (err=%v)", ErrorKindOf(err), ErrorInvalidRequest, err)
			}
		})
	}

	invoker, err := NewInvoker(registry)
	if err != nil {
		t.Fatalf("NewInvoker(default) error = %v", err)
	}
	if invoker == nil {
		t.Fatal("NewInvoker(default) returned nil invoker")
	}
}

func TestInvokerInvokeResolvesDefaultProtocolAndCompletesResponse(t *testing.T) {
	for _, protocol := range []Protocol{ProtocolResponses, ProtocolChatCompletions, ProtocolMessages, ProtocolGenerateContent, ProtocolBedrockConverse, ProtocolBedrockInvoke} {
		t.Run(string(protocol), func(t *testing.T) {
			var capabilityQuery CapabilityQuery
			var providerRequest Request
			provider := newFakeProvider("test")
			provider.defaultProtocol = protocol
			provider.capabilitiesFunc = func(_ context.Context, query CapabilityQuery) (CapabilityContract, error) {
				capabilityQuery = query
				return nativeContract(), nil
			}
			provider.invokeFunc = func(_ context.Context, request Request) (Response, error) {
				providerRequest = request
				return Response{ID: "response", Model: request.Model, Status: ResponseStatusCompleted}, nil
			}

			invoker := newTestInvoker(t, provider)
			request := validRequest()
			request.Stream = true
			response, err := invoker.Invoke(context.Background(), request)
			if err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}
			if providerRequest.Protocol != protocol || providerRequest.Stream {
				t.Fatalf("provider request protocol/stream = %q/%v, want %q/false", providerRequest.Protocol, providerRequest.Stream, protocol)
			}
			if capabilityQuery != (CapabilityQuery{Protocol: protocol, Endpoint: request.Endpoint, Model: request.Model}) {
				t.Fatalf("capability query = %#v", capabilityQuery)
			}
			if response.Provider != request.Provider || response.Protocol != protocol || response.Model != request.Model {
				t.Fatalf("completed response identity = provider=%q protocol=%q model=%q", response.Provider, response.Protocol, response.Model)
			}
			if response.MappingReport.Provider != request.Provider || response.MappingReport.Protocol != protocol || response.MappingReport.Endpoint != request.Endpoint {
				t.Fatalf("mapping report identity = %#v", response.MappingReport)
			}
			if len(response.MappingReport.Decisions) != 2 {
				t.Fatalf("mapping decisions = %#v, want text and usage", response.MappingReport.Decisions)
			}
			if request.Protocol != ProtocolAuto || !request.Stream {
				t.Fatalf("Invoke() mutated caller request: protocol=%q stream=%v", request.Protocol, request.Stream)
			}
		})
	}
}

func TestInvokerRejectsProviderResponseIdentityDrift(t *testing.T) {
	request := validRequest()
	cases := []Response{
		{Model: "other"},
	}
	for index, candidate := range cases {
		provider := newFakeProvider(request.Provider)
		provider.defaultProtocol = ProtocolResponses
		provider.capabilitiesFunc = func(context.Context, CapabilityQuery) (CapabilityContract, error) {
			return nativeContract(), nil
		}
		provider.invokeFunc = func(context.Context, Request) (Response, error) {
			return candidate, nil
		}
		if _, err := newTestInvoker(t, provider).Invoke(context.Background(), request); ErrorKindOf(err) != ErrorMapping {
			t.Fatalf("response identity drift %d was not rejected: %v", index, err)
		}
	}
}

func TestInvokerRevalidatesStateAfterResolvingDefaultProtocol(t *testing.T) {
	tests := []struct {
		name          string
		stateProtocol Protocol
		wantKind      ErrorKind
		wantCalls     int
	}{
		{name: "matching state", stateProtocol: ProtocolResponses, wantCalls: 1},
		{name: "cross-protocol state", stateProtocol: ProtocolMessages, wantKind: ErrorInvalidRequest},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capabilityCalls := 0
			invokeCalls := 0
			provider := newFakeProvider("test")
			provider.defaultProtocol = ProtocolResponses
			provider.capabilitiesFunc = func(context.Context, CapabilityQuery) (CapabilityContract, error) {
				capabilityCalls++
				return nativeContract(CapabilityServerState), nil
			}
			provider.invokeFunc = func(context.Context, Request) (Response, error) {
				invokeCalls++
				return Response{Status: ResponseStatusCompleted}, nil
			}

			request := validRequest()
			request.State = &State{
				Kind: StateServerContinuation, Provider: request.Provider,
				Protocol: test.stateProtocol, ID: "previous",
			}
			_, err := newTestInvoker(t, provider).Invoke(context.Background(), request)
			if got := ErrorKindOf(err); got != test.wantKind {
				t.Fatalf("ErrorKindOf(Invoke()) = %q, want %q (err=%v)", got, test.wantKind, err)
			}
			if capabilityCalls != test.wantCalls || invokeCalls != test.wantCalls {
				t.Fatalf("capability/invoke calls = %d/%d, want %d/%d", capabilityCalls, invokeCalls, test.wantCalls, test.wantCalls)
			}
		})
	}
}

func TestInvokerInvokeRetriesOnlyRetryableErrors(t *testing.T) {
	tests := []struct {
		name        string
		error       error
		maxAttempts int
		wantCalls   int
		wantSleeps  int
	}{
		{
			name:        "retryable",
			error:       &Error{Kind: ErrorRateLimit, Retryable: true, Message: "retry"},
			maxAttempts: 3,
			wantCalls:   3,
			wantSleeps:  2,
		},
		{
			name:        "non retryable",
			error:       &Error{Kind: ErrorInvalidRequest, Message: "stop"},
			maxAttempts: 3,
			wantCalls:   1,
			wantSleeps:  0,
		},
		{
			name:        "plain error is not retryable",
			error:       errors.New("plain provider failure"),
			maxAttempts: 3,
			wantCalls:   1,
			wantSleeps:  0,
		},
		{
			name:        "default single attempt",
			error:       &Error{Kind: ErrorProviderUnavailable, Retryable: true, Message: "retry"},
			maxAttempts: 1,
			wantCalls:   1,
			wantSleeps:  0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			sleeps := 0
			provider := newFakeProvider("test")
			provider.invokeFunc = func(context.Context, Request) (Response, error) {
				calls++
				return Response{}, test.error
			}
			options := []InvokerOption{WithSleeper(func(context.Context, time.Duration) error {
				sleeps++
				return nil
			})}
			if test.maxAttempts != 1 {
				options = append(options, WithRetryPolicy(RetryPolicy{MaxAttempts: test.maxAttempts, InitialBackoff: time.Millisecond, Multiplier: 2}))
			}
			invoker := newTestInvoker(t, provider, options...)
			_, _ = invoker.Invoke(context.Background(), validRequest())
			if calls != test.wantCalls || sleeps != test.wantSleeps {
				t.Fatalf("calls/sleeps = %d/%d, want %d/%d", calls, sleeps, test.wantCalls, test.wantSleeps)
			}
		})
	}
}

func TestInvokerRetryBackoffAndRetryAfterAreDeterministic(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter time.Duration
		maximum    time.Duration
		want       []time.Duration
	}{
		{name: "exponential", want: []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}},
		{name: "retry after wins", retryAfter: 50 * time.Millisecond, want: []time.Duration{50 * time.Millisecond, 50 * time.Millisecond}},
		{name: "retry after is a minimum beyond policy maximum", retryAfter: 50 * time.Millisecond, maximum: 30 * time.Millisecond, want: []time.Duration{50 * time.Millisecond, 50 * time.Millisecond}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			provider := newFakeProvider("test")
			provider.invokeFunc = func(context.Context, Request) (Response, error) {
				calls++
				if calls < 3 {
					return Response{}, &Error{Kind: ErrorRateLimit, Retryable: true, RetryAfter: test.retryAfter, Message: "retry"}
				}
				return Response{Status: ResponseStatusCompleted}, nil
			}
			var delays []time.Duration
			invoker := newTestInvoker(t, provider,
				WithRetryPolicy(RetryPolicy{MaxAttempts: 3, InitialBackoff: 10 * time.Millisecond, MaxBackoff: test.maximum, Multiplier: 2}),
				WithSleeper(func(_ context.Context, delay time.Duration) error {
					delays = append(delays, delay)
					return nil
				}),
			)
			if _, err := invoker.Invoke(context.Background(), validRequest()); err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}
			if !reflect.DeepEqual(delays, test.want) {
				t.Fatalf("retry delays = %v, want %v", delays, test.want)
			}
		})
	}
}

func TestInvokerCancellationDuringRetryWaitStopsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	provider := newFakeProvider("test")
	provider.invokeFunc = func(context.Context, Request) (Response, error) {
		calls++
		return Response{}, &Error{Kind: ErrorProviderUnavailable, Retryable: true, Message: "retry"}
	}
	sleeps := 0
	invoker := newTestInvoker(t, provider,
		WithRetryPolicy(RetryPolicy{MaxAttempts: 5, InitialBackoff: time.Second, Multiplier: 2}),
		WithSleeper(func(ctx context.Context, _ time.Duration) error {
			sleeps++
			cancel()
			return ctx.Err()
		}),
	)
	_, err := invoker.Invoke(ctx, validRequest())
	if ErrorKindOf(err) != ErrorCancelled {
		t.Fatalf("ErrorKindOf(Invoke()) = %q, want %q (err=%v)", ErrorKindOf(err), ErrorCancelled, err)
	}
	if calls != 1 || sleeps != 1 {
		t.Fatalf("calls/sleeps = %d/%d, want 1/1", calls, sleeps)
	}
}

func TestInvokerBudgetTimeoutCancelsProvider(t *testing.T) {
	provider := newFakeProvider("test")
	provider.invokeFunc = func(ctx context.Context, _ Request) (Response, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 100*time.Millisecond {
			t.Errorf("provider context deadline = %v, %v", deadline, ok)
		}
		<-ctx.Done()
		return Response{}, ctx.Err()
	}
	invoker := newTestInvoker(t, provider)
	request := validRequest()
	request.Budget.Timeout = 10 * time.Millisecond
	_, err := invoker.Invoke(context.Background(), request)
	if ErrorKindOf(err) != ErrorTimeout {
		t.Fatalf("ErrorKindOf(Invoke()) = %q, want %q (err=%v)", ErrorKindOf(err), ErrorTimeout, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout does not unwrap to context.DeadlineExceeded: %v", err)
	}
}

func TestInvokerRejectsNegativeBudgetBeforeProvider(t *testing.T) {
	provider := newFakeProvider("test")
	provider.capabilitiesFunc = func(context.Context, CapabilityQuery) (CapabilityContract, error) {
		t.Fatal("Capabilities called for invalid budget")
		return nil, nil
	}
	request := validRequest()
	request.Budget.Timeout = -time.Nanosecond
	if _, err := newTestInvoker(t, provider).Invoke(context.Background(), request); ErrorKindOf(err) != ErrorInvalidRequest {
		t.Fatalf("negative budget ErrorKind = %q, want %q (err=%v)", ErrorKindOf(err), ErrorInvalidRequest, err)
	}
}

func TestInvokerBudgetTimeoutIncludesCapabilityQuery(t *testing.T) {
	provider := newFakeProvider("test")
	provider.capabilitiesFunc = func(ctx context.Context, _ CapabilityQuery) (CapabilityContract, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 100*time.Millisecond {
			t.Errorf("capability context deadline = %v, %v", deadline, ok)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	provider.invokeFunc = func(context.Context, Request) (Response, error) {
		t.Fatal("provider Invoke called after capability timeout")
		return Response{}, nil
	}
	request := validRequest()
	request.Budget.Timeout = 10 * time.Millisecond
	_, err := newTestInvoker(t, provider).Invoke(context.Background(), request)
	if ErrorKindOf(err) != ErrorTimeout || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("capability timeout error = %#v", err)
	}
}

func TestInvokerStreamUsesDefaultProtocolCompletesEventsAndCancelsAtEOF(t *testing.T) {
	request := validRequest()
	inner := &fakeStream{events: []StreamEvent{
		{Type: StreamEventTextDelta, TextDelta: "hello"},
		{Type: StreamEventResponseCompleted, Response: &Response{ID: "response", Model: request.Model, Status: ResponseStatusCompleted}},
	}}
	provider := newFakeProvider("test")
	provider.defaultProtocol = ProtocolChatCompletions
	provider.capabilitiesFunc = func(context.Context, CapabilityQuery) (CapabilityContract, error) {
		return nativeContract(CapabilityStreaming), nil
	}
	var providerContext context.Context
	var providerRequest Request
	provider.streamFunc = func(ctx context.Context, request Request) (Stream, error) {
		providerContext = ctx
		providerRequest = request
		return inner, nil
	}

	invoker := newTestInvoker(t, provider)
	stream, err := invoker.Stream(context.Background(), request)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if !providerRequest.Stream || providerRequest.Protocol != ProtocolChatCompletions {
		t.Fatalf("provider stream request = stream:%v protocol:%q", providerRequest.Stream, providerRequest.Protocol)
	}
	if !stream.Next() || stream.Event().TextDelta != "hello" {
		t.Fatalf("first stream event = %#v", stream.Event())
	}
	if !stream.Next() {
		t.Fatal("second Next() = false")
	}
	completed := stream.Event()
	if completed.Response == nil {
		t.Fatal("completed response = nil")
	}
	if completed.Response.Provider != request.Provider || completed.Response.Protocol != ProtocolChatCompletions || completed.Response.Model != request.Model {
		t.Fatalf("completed response identity = %#v", completed.Response)
	}
	if completed.Response.MappingReport.Protocol != ProtocolChatCompletions {
		t.Fatalf("completed mapping report = %#v", completed.Response.MappingReport)
	}
	if stream.Next() {
		t.Fatal("Next() after EOF = true")
	}
	nextCallsAtEOF := inner.nextCalls
	if stream.Next() || inner.nextCalls != nextCallsAtEOF {
		t.Fatalf("repeated EOF reached inner stream: next calls %d -> %d", nextCallsAtEOF, inner.nextCalls)
	}
	select {
	case <-providerContext.Done():
	default:
		t.Fatal("provider context was not cancelled at EOF")
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if inner.closeCalls != 1 {
		t.Fatalf("inner Close() calls = %d, want 1", inner.closeCalls)
	}
}

func TestInvokerStreamNeverRetries(t *testing.T) {
	streamCalls := 0
	sleeps := 0
	provider := newFakeProvider("test")
	provider.capabilitiesFunc = func(context.Context, CapabilityQuery) (CapabilityContract, error) {
		return nativeContract(CapabilityStreaming), nil
	}
	provider.streamFunc = func(context.Context, Request) (Stream, error) {
		streamCalls++
		return nil, &Error{Kind: ErrorProviderUnavailable, Retryable: true, Message: "retryable but unsafe to replay"}
	}
	invoker := newTestInvoker(t, provider,
		WithRetryPolicy(RetryPolicy{MaxAttempts: 5, InitialBackoff: time.Millisecond, Multiplier: 2}),
		WithSleeper(func(context.Context, time.Duration) error { sleeps++; return nil }),
	)
	_, err := invoker.Stream(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Stream() error = nil")
	}
	if streamCalls != 1 || sleeps != 0 {
		t.Fatalf("stream calls/sleeps = %d/%d, want 1/0", streamCalls, sleeps)
	}
}

func TestInvokerStreamRejectsNilAndNormalizesIteratorErrors(t *testing.T) {
	provider := newFakeProvider("test")
	provider.capabilitiesFunc = func(context.Context, CapabilityQuery) (CapabilityContract, error) {
		return nativeContract(CapabilityStreaming), nil
	}
	provider.streamFunc = func(context.Context, Request) (Stream, error) { return nil, nil }
	invoker := newTestInvoker(t, provider)
	if _, err := invoker.Stream(context.Background(), validRequest()); ErrorKindOf(err) != ErrorStreamInterrupted {
		t.Fatalf("nil stream ErrorKind = %q, want %q", ErrorKindOf(err), ErrorStreamInterrupted)
	}
	var typedNil *fakeStream
	provider.streamFunc = func(context.Context, Request) (Stream, error) { return typedNil, nil }
	if _, err := invoker.Stream(context.Background(), validRequest()); ErrorKindOf(err) != ErrorStreamInterrupted {
		t.Fatalf("typed nil stream ErrorKind = %q, want %q", ErrorKindOf(err), ErrorStreamInterrupted)
	}

	inner := &fakeStream{err: context.Canceled}
	provider.streamFunc = func(context.Context, Request) (Stream, error) { return inner, nil }
	stream, err := invoker.Stream(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if stream.Next() {
		t.Fatal("empty stream Next() = true")
	}
	if got := ErrorKindOf(stream.Err()); got != ErrorCancelled {
		t.Fatalf("iterator ErrorKind = %q, want %q", got, ErrorCancelled)
	}
}

func TestTypedNilErrorsDoNotPanicRuntime(t *testing.T) {
	var typedNil *Error
	var err error = typedNil
	if kind := ErrorKindOf(err); kind != "" {
		t.Fatalf("typed nil ErrorKindOf = %q", kind)
	}

	provider := newFakeProvider("test")
	provider.invokeFunc = func(context.Context, Request) (Response, error) { return Response{}, typedNil }
	_, err = newTestInvoker(t, provider).Invoke(context.Background(), validRequest())
	if ErrorKindOf(err) != ErrorProvider {
		t.Fatalf("typed nil provider error = %#v", err)
	}
}

func TestInvokerFailureReturnsResponseAndCompleteMappingAudit(t *testing.T) {
	provider := newFakeProvider("test")
	provider.invokeFunc = func(context.Context, Request) (Response, error) {
		return Response{
			RawResponse: NewRawPayload([]byte(`{"error":"mapped"}`)),
			MappingReport: MappingReport{Decisions: []MappingDecision{{
				Capability: CapabilityTextGeneration, Action: MappingDegraded, Detail: "provider detail",
			}}},
		}, &Error{Kind: ErrorMapping, Message: "mapping failed"}
	}
	request := validRequest()
	request.Endpoint = "https://example.test/v1"
	response, err := newTestInvoker(t, provider).Invoke(context.Background(), request)
	if ErrorKindOf(err) != ErrorMapping || response.RawResponse.Empty() {
		t.Fatalf("failure response/error = %#v / %#v", response, err)
	}
	var invocationError *Error
	if !errors.As(err, &invocationError) || invocationError == nil {
		t.Fatalf("failure error type = %T", err)
	}
	if response.MappingReport.Endpoint != request.Endpoint || len(response.MappingReport.Decisions) != 3 {
		t.Fatalf("response mapping report = %#v", response.MappingReport)
	}
	if !reflect.DeepEqual(invocationError.MappingReport, response.MappingReport) {
		t.Fatalf("error mapping report = %#v, response = %#v", invocationError.MappingReport, response.MappingReport)
	}
}

func TestInvokerRejectsInvalidOrUnsupportedBeforeCallingProvider(t *testing.T) {
	invokeCalls := 0
	provider := newFakeProvider("test")
	provider.invokeFunc = func(context.Context, Request) (Response, error) {
		invokeCalls++
		return Response{}, nil
	}
	invoker := newTestInvoker(t, provider)

	invalid := validRequest()
	invalid.Model = ""
	if _, err := invoker.Invoke(context.Background(), invalid); ErrorKindOf(err) != ErrorInvalidRequest {
		t.Fatalf("invalid request ErrorKind = %q, want %q", ErrorKindOf(err), ErrorInvalidRequest)
	}

	unsupported := validRequest()
	unsupported.Output = OutputConstraint{Type: OutputJSONObject}
	if _, err := invoker.Invoke(context.Background(), unsupported); ErrorKindOf(err) != ErrorUnsupportedCapability {
		t.Fatalf("unsupported request ErrorKind = %q, want %q", ErrorKindOf(err), ErrorUnsupportedCapability)
	}
	if invokeCalls != 0 {
		t.Fatalf("provider Invoke() calls = %d, want 0", invokeCalls)
	}
}
