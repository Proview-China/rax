package routegateway_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
)

func TestGatewaySanitizesAndStampsEveryCapabilitiesErrorShape(t *testing.T) {
	for _, mode := range []string{"capabilities_plain", "capabilities_wrapped", "capabilities_joined"} {
		t.Run(mode, func(t *testing.T) {
			state := &callState{}
			factory := &forgedErrorFactory{fakeFactory: fakeFactory{id: "openai", state: state}, mode: mode}
			gateway := gatewayWithOverrideFactory(t, factory, state)
			defer gateway.Close()

			result, err := gateway.Capabilities(context.Background(), openAICall())
			if err == nil {
				t.Fatal("Capabilities error = nil")
			}
			assertSanitizedStampedError(t, err, result.Resolution.Route)
		})
	}
}

func TestGatewayStampsInvokeAndStreamProviderErrors(t *testing.T) {
	for _, operation := range []string{"invoke", "stream"} {
		t.Run(operation, func(t *testing.T) {
			state := &callState{}
			factory := &forgedErrorFactory{fakeFactory: fakeFactory{id: "openai", state: state}, mode: operation}
			gateway := gatewayWithOverrideFactory(t, factory, state)
			defer gateway.Close()

			if operation == "invoke" {
				result, err := gateway.Invoke(context.Background(), openAICall())
				if err == nil {
					t.Fatal("Invoke error = nil")
				}
				assertSanitizedStampedError(t, err, result.Resolution.Route)
				return
			}
			_, err := gateway.Stream(context.Background(), openAICall())
			if err == nil {
				t.Fatal("Stream error = nil")
			}
			var routeErr *modelinvoker.RouteError
			if !errors.As(err, &routeErr) || routeErr == nil {
				t.Fatalf("Stream error has no trusted RouteError: %v", err)
			}
			assertSanitizedStampedError(t, err, routeErr.Route)
		})
	}
}

func TestGatewayDiscardsProviderModelErrorPayloadForInvokeAndStream(t *testing.T) {
	for _, code := range []string{"response_model_missing", "response_model_mismatch"} {
		t.Run("invoke_"+code, func(t *testing.T) {
			state := &callState{}
			factory := &forgedErrorFactory{fakeFactory: fakeFactory{id: "openai", state: state}, mode: "invoke_" + code}
			gateway := gatewayWithOverrideFactory(t, factory, state)
			defer gateway.Close()

			result, err := gateway.Invoke(context.Background(), openAICall())
			if err == nil {
				t.Fatal("Invoke model error = nil")
			}
			assertGatewayErrorCode(t, err, code)
			if !reflect.DeepEqual(result.Response, modelinvoker.Response{}) {
				t.Fatalf("Provider model error returned untrusted Invoke response: %#v", result.Response)
			}
		})
		t.Run("stream_"+code, func(t *testing.T) {
			state := &callState{}
			factory := &forgedErrorFactory{fakeFactory: fakeFactory{id: "openai", state: state}, mode: "stream_event_" + code}
			gateway := gatewayWithOverrideFactory(t, factory, state)
			defer gateway.Close()

			stream, err := gateway.Stream(context.Background(), openAICall())
			if err != nil {
				t.Fatal(err)
			}
			if !stream.Next() || stream.Event().Error == nil {
				t.Fatalf("Provider model error stream event = %#v", stream.Event())
			}
			event := stream.Event()
			assertGatewayErrorCode(t, event.Error, code)
			if event.Response != nil || !event.Raw.Empty() {
				t.Fatalf("Provider model error exposed stream Response/Raw: %#v", event)
			}
		})
	}
}

func TestGatewayStampsForgedIdentityInsideStaleLeaseCloseFailure(t *testing.T) {
	state := &callState{}
	secret := &rotatingSecret{state: state, version: "forged-close-v1"}
	factory := &rotationCloseFactory{fakeFactory: fakeFactory{id: "openai", state: state}, firstCloseErr: forgedProviderError()}
	gateway := gatewayWithOverrideFactoryAndSecret(t, factory, state, secret)
	defer gateway.Close()

	stream, err := gateway.Stream(context.Background(), openAICall())
	if err != nil {
		t.Fatal(err)
	}
	secret.setVersion("forged-close-v2")
	if _, err := gateway.Resolve(context.Background(), openAICall()); err != nil {
		t.Fatal(err)
	}
	err = stream.Close()
	if err == nil {
		t.Fatal("stale lease Close error = nil")
	}
	assertSanitizedStampedError(t, err, stream.Resolution().Route)
}

func TestGatewaySecondaryModelMismatchEventIsStampedAndCloseCauseIsSafe(t *testing.T) {
	const sentinel = "MODEL-MISMATCH-STREAM-CLOSE-SECRET-MUST-NOT-LEAK"
	state := &callState{}
	closeFailure := errors.New(sentinel)
	factory := &secondaryModelMismatchFactory{
		fakeFactory: fakeFactory{id: "openai", state: state}, closeErr: closeFailure,
	}
	gateway := gatewayWithOverrideFactory(t, factory, state)
	defer gateway.Close()

	stream, err := gateway.Stream(context.Background(), openAICall())
	if err != nil {
		t.Fatal(err)
	}
	if !stream.Next() {
		t.Fatalf("secondary model mismatch produced no error event: %v", stream.Err())
	}
	event := stream.Event()
	if event.Type != modelinvoker.StreamEventError || event.Error == nil || event.Response != nil || !event.Raw.Empty() {
		t.Fatalf("secondary model mismatch event = %#v", event)
	}
	assertGatewayErrorCode(t, event.Error, "response_model_mismatch")
	selection := stream.Resolution().Route
	if event.Error.Provider != selection.AdapterID || event.Error.MappingReport.Provider != selection.AdapterID ||
		event.Error.MappingReport.Protocol != selection.Protocol || event.Error.MappingReport.Endpoint != selection.Endpoint {
		t.Fatalf("secondary model mismatch event identity = %#v; selection = %#v", event.Error, selection)
	}
	streamErr := stream.Err()
	if streamErr == nil || !errors.Is(streamErr, closeFailure) {
		t.Fatalf("secondary model mismatch lost safe stream Close cause: %v", streamErr)
	}
	if strings.Contains(streamErr.Error(), sentinel) || strings.Contains(event.Error.Error(), sentinel) {
		t.Fatalf("secondary model mismatch leaked stream Close cause: event=%v err=%v", event.Error, streamErr)
	}
	if factory.closeCalls.Load() != 1 {
		t.Fatalf("secondary mismatch stream Close calls = %d, want 1", factory.closeCalls.Load())
	}
}

func TestGatewayStreamCloseSanitizesStampsAndPreservesCloseCause(t *testing.T) {
	const sentinel = "EXPLICIT-STREAM-CLOSE-SECRET-MUST-NOT-LEAK"
	state := &callState{}
	closeFailure := errors.New(sentinel)
	factory := &secondaryModelMismatchFactory{
		fakeFactory: fakeFactory{id: "openai", state: state}, closeErr: closeFailure,
	}
	gateway := gatewayWithOverrideFactory(t, factory, state)
	defer gateway.Close()

	stream, err := gateway.Stream(context.Background(), openAICall())
	if err != nil {
		t.Fatal(err)
	}
	err = stream.Close()
	if err == nil || !errors.Is(err, closeFailure) {
		t.Fatalf("explicit Stream.Close lost safe cause: %v", err)
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("explicit Stream.Close leaked cause: %v", err)
	}
	assertSanitizedStampedError(t, err, stream.Resolution().Route)
	if factory.closeCalls.Load() != 1 {
		t.Fatalf("explicit Stream.Close calls = %d, want 1", factory.closeCalls.Load())
	}
}

func assertSanitizedStampedError(t *testing.T, err error, selection modelinvoker.RouteSelection) {
	t.Helper()
	if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "forged-provider") || strings.Contains(err.Error(), "forged.invalid") {
		t.Fatalf("Gateway error leaked untrusted text or identity: %v", err)
	}
	var found int
	walkErrors(err, func(invocationError *modelinvoker.Error) {
		found++
		if invocationError.Provider != selection.AdapterID || invocationError.MappingReport.Provider != selection.AdapterID ||
			invocationError.MappingReport.Protocol != selection.Protocol || invocationError.MappingReport.Endpoint != selection.Endpoint {
			t.Errorf("unstamped Gateway error = %#v; selection = %#v", invocationError, selection)
		}
	})
	if found == 0 {
		t.Fatalf("Gateway error has no normalized *modelinvoker.Error: %v", err)
	}
}

func walkErrors(err error, visit func(*modelinvoker.Error)) {
	if err == nil {
		return
	}
	if invocationError, ok := err.(*modelinvoker.Error); ok && invocationError != nil {
		visit(invocationError)
		walkErrors(invocationError.Err, visit)
		return
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			walkErrors(child, visit)
		}
		return
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		walkErrors(wrapped.Unwrap(), visit)
	}
}

type forgedErrorFactory struct {
	fakeFactory
	mode string
}

type secondaryModelMismatchFactory struct {
	fakeFactory
	closeErr   error
	closeCalls atomic.Int64
}

func (factory *secondaryModelMismatchFactory) Build(_ context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	factory.state.factory.Add(1)
	provider := &secondaryModelMismatchProvider{
		fakeProvider: &fakeProvider{id: factory.id, state: factory.state}, closeErr: factory.closeErr, closeCalls: &factory.closeCalls,
	}
	return routegateway.FactoryResult{Provider: provider, Closer: countCloser{state: factory.state}, Endpoint: input.Endpoint}, nil
}

type secondaryModelMismatchProvider struct {
	*fakeProvider
	closeErr   error
	closeCalls *atomic.Int64
}

func (provider *secondaryModelMismatchProvider) Stream(_ context.Context, request modelinvoker.Request) (modelinvoker.Stream, error) {
	return &secondaryModelMismatchStream{
		request: request, closeErr: provider.closeErr, closeCalls: provider.closeCalls,
	}, nil
}

type secondaryModelMismatchStream struct {
	request    modelinvoker.Request
	closeErr   error
	closeCalls *atomic.Int64
	sent       bool
}

func (stream *secondaryModelMismatchStream) Next() bool {
	if stream.sent {
		return false
	}
	stream.sent = true
	return true
}
func (stream *secondaryModelMismatchStream) Event() modelinvoker.StreamEvent {
	return modelinvoker.StreamEvent{
		Type: modelinvoker.StreamEventTextDelta, Sequence: 1, TextDelta: "untrusted",
		Response: &modelinvoker.Response{
			Provider: stream.request.Provider, Protocol: stream.request.Protocol, Model: "silently-mapped-model",
		},
		Raw: modelinvoker.NewRawPayload([]byte("UNTRUSTED-MISMATCH-RAW")),
	}
}
func (*secondaryModelMismatchStream) Err() error { return nil }
func (stream *secondaryModelMismatchStream) Close() error {
	stream.closeCalls.Add(1)
	return stream.closeErr
}

func (factory *forgedErrorFactory) Build(_ context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	factory.state.factory.Add(1)
	return routegateway.FactoryResult{
		Provider: &forgedErrorProvider{fakeProvider: &fakeProvider{id: factory.id, state: factory.state}, mode: factory.mode},
		Closer:   countCloser{state: factory.state}, Endpoint: input.Endpoint,
	}, nil
}

type forgedErrorProvider struct {
	*fakeProvider
	mode string
}

func (provider *forgedErrorProvider) Capabilities(ctx context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	switch provider.mode {
	case "capabilities_plain":
		return nil, errors.New("CAPABILITIES-PLAIN-SECRET")
	case "capabilities_wrapped":
		return nil, fmt.Errorf("CAPABILITIES-WRAPPER-SECRET: %w", forgedProviderError())
	case "capabilities_joined":
		return nil, errors.Join(forgedProviderError(), errors.New("CAPABILITIES-JOIN-SECRET"))
	default:
		return provider.fakeProvider.Capabilities(ctx, query)
	}
}

func (provider *forgedErrorProvider) Invoke(_ context.Context, request modelinvoker.Request) (modelinvoker.Response, error) {
	if strings.HasPrefix(provider.mode, "invoke_response_model_") {
		code := strings.TrimPrefix(provider.mode, "invoke_")
		return untrustedModelErrorResponse(request), providerModelError(code)
	}
	if provider.mode == "invoke" {
		return modelinvoker.Response{Model: request.Model, Status: modelinvoker.ResponseStatusFailed}, forgedProviderError()
	}
	return provider.fakeProvider.Invoke(context.Background(), request)
}

func (provider *forgedErrorProvider) Stream(_ context.Context, request modelinvoker.Request) (modelinvoker.Stream, error) {
	if strings.HasPrefix(provider.mode, "stream_event_response_model_") {
		code := strings.TrimPrefix(provider.mode, "stream_event_")
		return &providerModelErrorStream{event: modelinvoker.StreamEvent{
			Type: modelinvoker.StreamEventError, Response: responsePointer(untrustedModelErrorResponse(request)),
			Error: providerModelError(code), Raw: modelinvoker.NewRawPayload([]byte("UNTRUSTED-MODEL-ERROR-RAW")),
		}}, nil
	}
	if provider.mode == "stream" {
		return nil, forgedProviderError()
	}
	return provider.fakeProvider.Stream(context.Background(), request)
}

func untrustedModelErrorResponse(request modelinvoker.Request) modelinvoker.Response {
	return modelinvoker.Response{
		Provider: "forged-provider", Protocol: modelinvoker.ProtocolMessages, Model: request.Model,
		Status: modelinvoker.ResponseStatusFailed, RawResponse: modelinvoker.NewRawPayload([]byte("UNTRUSTED-MODEL-ERROR-RESPONSE")),
	}
}

func responsePointer(response modelinvoker.Response) *modelinvoker.Response { return &response }

func providerModelError(code string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: modelinvoker.ErrorMapping, Provider: "forged-provider", Operation: "provider_model", Code: code, Message: "UNTRUSTED-MODEL-ERROR-SECRET"}
}

type providerModelErrorStream struct {
	event modelinvoker.StreamEvent
	sent  bool
}

func (stream *providerModelErrorStream) Next() bool {
	if stream.sent {
		return false
	}
	stream.sent = true
	return true
}
func (stream *providerModelErrorStream) Event() modelinvoker.StreamEvent { return stream.event }
func (*providerModelErrorStream) Err() error                             { return nil }
func (*providerModelErrorStream) Close() error                           { return nil }

func forgedProviderError() *modelinvoker.Error {
	return &modelinvoker.Error{
		Kind: modelinvoker.ErrorProvider, Provider: "forged-provider", Operation: "forged", Code: "forged_code",
		Message: "FORGED-MESSAGE-SECRET", Retryable: true,
		MappingReport: modelinvoker.MappingReport{
			Provider: "forged-provider", Protocol: modelinvoker.ProtocolMessages, Endpoint: "https://forged.invalid/v1",
			Decisions: []modelinvoker.MappingDecision{{Detail: "decision retained"}},
		},
	}
}
