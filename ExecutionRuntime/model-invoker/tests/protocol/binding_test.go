package protocol_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
)

func TestNewBindingAndBaseRejectUnsafeZeroValues(t *testing.T) {
	tests := []struct {
		name     string
		provider modelinvoker.ProviderID
		protocol modelinvoker.Protocol
		endpoint string
		headers  []string
	}{
		{name: "empty provider", protocol: modelinvoker.ProtocolChatCompletions, endpoint: testEndpoint},
		{name: "automatic protocol", provider: testProvider, endpoint: testEndpoint},
		{name: "empty endpoint", provider: testProvider, protocol: modelinvoker.ProtocolChatCompletions},
		{name: "non-loopback plain HTTP", provider: testProvider, protocol: modelinvoker.ProtocolChatCompletions, endpoint: "http://gateway.example.test/v1"},
		{name: "endpoint user info", provider: testProvider, protocol: modelinvoker.ProtocolChatCompletions, endpoint: "https://user@gateway.example.test/v1"},
		{name: "endpoint query", provider: testProvider, protocol: modelinvoker.ProtocolChatCompletions, endpoint: testEndpoint + "?token=x"},
		{name: "endpoint fragment", provider: testProvider, protocol: modelinvoker.ProtocolChatCompletions, endpoint: testEndpoint + "#fragment"},
		{name: "uppercase request ID header", provider: testProvider, protocol: modelinvoker.ProtocolChatCompletions, endpoint: testEndpoint, headers: []string{"X-Request-ID"}},
		{name: "duplicate request ID header", provider: testProvider, protocol: modelinvoker.ProtocolChatCompletions, endpoint: testEndpoint, headers: []string{"x-request-id", "x-request-id"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := protocol.NewBinding(test.provider, test.protocol, test.endpoint, test.headers...); err == nil {
				t.Fatal("NewBinding() error = nil")
			}
		})
	}

	dialect := &fakeDialect{}
	if _, err := protocol.NewBase(protocol.Binding{}, dialect); err == nil {
		t.Fatal("NewBase(zero Binding) error = nil")
	}
	if _, err := protocol.NewBase(protocol.Binding{
		Provider: testProvider, Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: testEndpoint + "/",
	}, dialect); err == nil {
		t.Fatal("NewBase(non-canonical Binding) error = nil")
	}
	if _, err := protocol.NewBase(mustBinding(t), nil); err == nil {
		t.Fatal("NewBase(nil Dialect) error = nil")
	}
	var typedNil *fakeDialect
	if _, err := protocol.NewBase(mustBinding(t), typedNil); err == nil {
		t.Fatal("NewBase(typed-nil Dialect) error = nil")
	}
	if !protocol.IsNil(nil) || !protocol.IsNil(typedNil) || protocol.IsNil(dialect) || protocol.IsNil(struct{}{}) {
		t.Fatal("IsNil() did not distinguish nil, typed nil, and concrete values")
	}
}

func TestBindingCanonicalizationAndCopiesAreDefensive(t *testing.T) {
	headers := []string{"x-request-id", "request-id"}
	binding, err := protocol.NewBinding(testProvider, modelinvoker.ProtocolChatCompletions, "https://GATEWAY.example.test:443/v1///", headers...)
	if err != nil {
		t.Fatal(err)
	}
	headers[0] = "mutated"
	if binding.Endpoint != testEndpoint || binding.RequestIDHeaders[0] != "x-request-id" {
		t.Fatalf("canonical Binding = %#v", binding)
	}

	base, err := protocol.NewBase(binding, &fakeDialect{})
	if err != nil {
		t.Fatal(err)
	}
	first := base.Binding()
	first.RequestIDHeaders[0] = "mutated"
	second := base.Binding()
	if second.RequestIDHeaders[0] != "x-request-id" || second.Endpoint != testEndpoint {
		t.Fatalf("Base.Binding() shared state: %#v", second)
	}
	if got := binding.EffectiveEndpoint(""); got != testEndpoint {
		t.Fatalf("EffectiveEndpoint(empty) = %q", got)
	}
	if got := binding.EffectiveEndpoint("https://GATEWAY.example.test:443/v1/"); got != testEndpoint {
		t.Fatalf("EffectiveEndpoint(canonical equivalent) = %q", got)
	}
}

func TestBaseRejectsSelectionAndStateMismatchBeforeNativeCall(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*modelinvoker.Request)
	}{
		{name: "provider", mutate: func(request *modelinvoker.Request) { request.Provider = "other-host" }},
		{name: "protocol", mutate: func(request *modelinvoker.Request) { request.Protocol = modelinvoker.ProtocolMessages }},
		{name: "endpoint scheme", mutate: func(request *modelinvoker.Request) { request.Endpoint = "http://127.0.0.1/v1" }},
		{name: "endpoint host", mutate: func(request *modelinvoker.Request) { request.Endpoint = "https://other.example.test/v1" }},
		{name: "endpoint path", mutate: func(request *modelinvoker.Request) { request.Endpoint = "https://gateway.example.test/v2" }},
		{name: "state provider", mutate: func(request *modelinvoker.Request) {
			request.State = &modelinvoker.State{Kind: modelinvoker.StateServerContinuation, Provider: "other-host", Protocol: request.Protocol, ID: "previous"}
		}},
		{name: "state protocol", mutate: func(request *modelinvoker.Request) {
			request.State = &modelinvoker.State{Kind: modelinvoker.StateServerContinuation, Provider: request.Provider, Protocol: modelinvoker.ProtocolMessages, ID: "previous"}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dialect := &fakeDialect{}
			base, err := protocol.NewBase(mustBinding(t), dialect)
			if err != nil {
				t.Fatal(err)
			}
			driver := &fakeDriver{base: base}
			request := validRequest(testEndpoint)
			test.mutate(&request)
			if _, err := driver.Invoke(context.Background(), request); err == nil {
				t.Fatal("Invoke() error = nil")
			}
			if driver.nativeCalls != 0 || dialect.validateCalls != 0 {
				t.Fatalf("native/dialect calls = %d/%d, want 0/0", driver.nativeCalls, dialect.validateCalls)
			}
		})
	}

	for _, endpoint := range []string{"", "https://GATEWAY.example.test:443/v1/"} {
		dialect := &fakeDialect{}
		base, err := protocol.NewBase(mustBinding(t), dialect)
		if err != nil {
			t.Fatal(err)
		}
		driver := &fakeDriver{base: base, invokeResponse: modelinvoker.Response{Status: modelinvoker.ResponseStatusCompleted}}
		response, err := driver.Invoke(context.Background(), validRequest(endpoint))
		if err != nil {
			t.Fatalf("Invoke(endpoint=%q) error = %v", endpoint, err)
		}
		if driver.nativeCalls != 1 || dialect.validateCalls != 1 || response.MappingReport.Endpoint != testEndpoint {
			t.Fatalf("successful selection calls/endpoint = %d/%d/%q", driver.nativeCalls, dialect.validateCalls, response.MappingReport.Endpoint)
		}
	}
}

func TestBindingStampsEveryInvokeIdentityWithoutMutatingInputs(t *testing.T) {
	binding := mustBinding(t)
	request := validRequest("")
	original := modelinvoker.Response{
		Provider: "openai",
		Protocol: modelinvoker.ProtocolResponses,
		State: &modelinvoker.State{
			Kind: modelinvoker.StateServerContinuation, Provider: "anthropic", Protocol: modelinvoker.ProtocolMessages,
			ID: "previous", Payload: modelinvoker.NewRawPayload([]byte(`{"state":true}`)),
		},
		MappingReport: modelinvoker.MappingReport{
			Provider: "gemini", Protocol: modelinvoker.ProtocolGenerateContent, Endpoint: "https://wrong.example.test",
			Decisions: []modelinvoker.MappingDecision{{Capability: modelinvoker.CapabilityTextGeneration, Action: modelinvoker.MappingTransformed, Detail: "preserve-me"}},
		},
		RawRequest:  modelinvoker.NewRawPayload([]byte(`{"request":true}`)),
		RawResponse: modelinvoker.NewRawPayload([]byte(`{"response":true}`)),
	}
	stamped := binding.StampResponse(request, original)
	if stamped.Provider != testProvider || stamped.Protocol != modelinvoker.ProtocolChatCompletions || stamped.Model != request.Model {
		t.Fatalf("stamped Response identity = %#v", stamped)
	}
	if stamped.State == nil || stamped.State.Provider != testProvider || stamped.State.Protocol != modelinvoker.ProtocolChatCompletions {
		t.Fatalf("stamped State = %#v", stamped.State)
	}
	if stamped.MappingReport.Provider != testProvider || stamped.MappingReport.Protocol != modelinvoker.ProtocolChatCompletions || stamped.MappingReport.Endpoint != testEndpoint {
		t.Fatalf("stamped MappingReport = %#v", stamped.MappingReport)
	}
	if got := stamped.MappingReport.Decisions[0].Detail; got != "preserve-me" {
		t.Fatalf("mapping decision detail = %q", got)
	}
	stamped.MappingReport.Decisions[0].Detail = "mutated"
	if original.MappingReport.Decisions[0].Detail != "preserve-me" || original.State.Provider != "anthropic" {
		t.Fatalf("StampResponse mutated input: %#v", original)
	}

	secret := "sdk-request-secret"
	native := &fakeSDKError{
		Request:    &http.Request{Header: http.Header{"Authorization": []string{"Bearer " + secret}}},
		Credential: fakeCredential{APIKey: secret}, message: "native " + secret,
	}
	originalErr := &modelinvoker.Error{
		Kind: modelinvoker.ErrorMapping, Provider: "openai", Message: "safe failure", Err: native,
		MappingReport: modelinvoker.MappingReport{Provider: "openai", Protocol: modelinvoker.ProtocolResponses, Endpoint: "https://wrong.example.test"},
	}
	stampedErr := binding.StampError(context.Background(), request, originalErr, "fake.invoke")
	var invocationError *modelinvoker.Error
	if !errors.As(stampedErr, &invocationError) || invocationError.Provider != testProvider || invocationError.Operation != "fake.invoke" || invocationError.Err != nil || invocationError.MappingReport.Endpoint != testEndpoint {
		t.Fatalf("stamped Error = %#v", stampedErr)
	}
	var exposed *fakeSDKError
	if errors.As(stampedErr, &exposed) {
		t.Fatal("StampError retained native SDK error")
	}
	if originalErr.Provider != "openai" || originalErr.Err != native {
		t.Fatal("StampError mutated input error")
	}
	encoded, err := json.Marshal(stampedErr)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprintf("%#v|%s", stampedErr, encoded), secret) {
		t.Fatal("stamped Error retained native request or credential data")
	}
}

func TestBaseValidationMetadataAndRequestIDAreDefensive(t *testing.T) {
	sharedMetadata := modelinvoker.ProviderMetadata{"region": "test-region"}
	dialect := &fakeDialect{metadata: sharedMetadata, mutateHeaders: true}
	base, err := protocol.NewBase(mustBinding(t), dialect)
	if err != nil {
		t.Fatal(err)
	}
	if err := base.Validate(validRequest("")); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	headers := http.Header{
		"X-Request-Id": []string{"primary"},
		"Request-Id":   []string{"secondary"},
		"X-Test":       []string{"original"},
	}
	metadata := base.ProviderMetadata(headers)
	if headers.Get("X-Test") != "original" || metadata["region"] != "test-region" {
		t.Fatalf("metadata/header = %#v / %#v", metadata, headers)
	}
	metadata["region"] = "caller-mutated"
	if sharedMetadata["region"] != "test-region" {
		t.Fatal("ProviderMetadata returned dialect-owned map")
	}
	if got := base.RequestID(headers); got != "primary" {
		t.Fatalf("RequestID() = %q, want primary", got)
	}
	if dialect.validateCalls != 1 || dialect.metadataCalls != 1 {
		t.Fatalf("dialect validate/metadata calls = %d/%d", dialect.validateCalls, dialect.metadataCalls)
	}

	native := &fakeSDKError{message: "dialect-native-secret"}
	dialect.validateErr = native
	err = base.Validate(validRequest(""))
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError.Provider != testProvider || invocationError.Err != nil {
		t.Fatalf("dialect validation error = %#v", err)
	}
	var exposed *fakeSDKError
	if errors.As(err, &exposed) || strings.Contains(fmt.Sprintf("%#v", err), "dialect-native-secret") {
		t.Fatal("Base.Validate retained dialect-native error")
	}
}

func TestBaseStampErrorPreservesNil(t *testing.T) {
	base, err := protocol.NewBase(mustBinding(t), &fakeDialect{})
	if err != nil {
		t.Fatal(err)
	}
	if got := base.StampError(context.Background(), validRequest(""), nil, "fake.invoke"); got != nil {
		t.Fatalf("StampError(nil) = %#v (%T), want nil", got, got)
	}
}

func TestBindingStampErrorPreservesNilInterface(t *testing.T) {
	var got error = mustBinding(t).StampError(context.Background(), validRequest(""), nil, "fake.invoke")
	if got != nil {
		t.Fatalf("Binding.StampError(nil) = %#v (%T), want nil error interface", got, got)
	}
}

func TestNilBaseFailsClosedInsteadOfPassingThroughUnsafeValues(t *testing.T) {
	var base *protocol.Base
	request := validRequest("")
	secret := "nil-base-native-secret"
	native := &fakeSDKError{
		Request:    &http.Request{Header: http.Header{"Authorization": []string{"Bearer " + secret}}},
		Credential: fakeCredential{APIKey: secret}, message: secret,
	}
	poisoned := modelinvoker.Response{
		Provider: "openai", Protocol: modelinvoker.ProtocolResponses, Status: modelinvoker.ResponseStatusCompleted,
		State: &modelinvoker.State{Kind: modelinvoker.StateServerContinuation, Provider: "openai", Protocol: modelinvoker.ProtocolResponses, ID: "previous"},
		MappingReport: modelinvoker.MappingReport{
			Provider: "openai", Protocol: modelinvoker.ProtocolResponses, Endpoint: "https://wrong.example.test",
			Decisions: []modelinvoker.MappingDecision{{Capability: modelinvoker.CapabilityTextGeneration, Action: modelinvoker.MappingExact}},
		},
		RawResponse: modelinvoker.NewRawPayload([]byte(secret)),
	}
	response := base.StampResponse(request, poisoned)
	if response.Status != modelinvoker.ResponseStatusFailed || response.Model != request.Model || response.Provider != "" || response.Protocol != "" ||
		response.State != nil || len(response.MappingReport.Decisions) != 0 || !response.RawResponse.Empty() {
		t.Fatalf("nil Base passed through poisoned Response: %#v", response)
	}
	if poisoned.Provider != "openai" || poisoned.RawResponse.Empty() {
		t.Fatal("nil Base mutated input while failing closed")
	}

	err := base.StampError(context.Background(), request, native, "fake.invoke")
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError.Kind != modelinvoker.ErrorProviderUnavailable || invocationError.Err != nil {
		t.Fatalf("nil Base StampError = %#v", err)
	}
	var exposed *fakeSDKError
	if errors.As(err, &exposed) || strings.Contains(fmt.Sprintf("%#v", err), secret) {
		t.Fatal("nil Base passed through SDK cause")
	}

	inner := &fakeStream{events: []modelinvoker.StreamEvent{{Type: modelinvoker.StreamEventNative}}}
	if stream := base.BindStream(context.Background(), request, inner); stream != nil {
		t.Fatalf("nil Base BindStream = %#v, want nil", stream)
	}
	if inner.nextCalls != 0 || inner.closeCalls != 0 {
		t.Fatalf("nil Base touched unsafe stream: next/close = %d/%d", inner.nextCalls, inner.closeCalls)
	}
}

func TestBindingStampEventCoversNestedResponseAndError(t *testing.T) {
	binding := mustBinding(t)
	request := validRequest("")
	event := modelinvoker.StreamEvent{
		Type: modelinvoker.StreamEventError, Sequence: 17,
		Response: &modelinvoker.Response{
			Provider: "openai", Protocol: modelinvoker.ProtocolResponses,
			State:         &modelinvoker.State{Kind: modelinvoker.StateServerContinuation, Provider: "openai", Protocol: modelinvoker.ProtocolResponses, ID: "previous"},
			MappingReport: modelinvoker.MappingReport{Provider: "openai", Protocol: modelinvoker.ProtocolResponses, Endpoint: "https://wrong.example.test"},
		},
		Error: &modelinvoker.Error{
			Kind: modelinvoker.ErrorProvider, Provider: "openai", Message: "failed", Err: errors.New("native"),
			MappingReport: modelinvoker.MappingReport{Provider: "openai", Protocol: modelinvoker.ProtocolResponses, Endpoint: "https://wrong.example.test"},
		},
		Raw: modelinvoker.NewRawPayload([]byte(`{"event":true}`)),
	}
	stamped := binding.StampEvent(context.Background(), request, event)
	if stamped.Sequence != event.Sequence || !reflect.DeepEqual(stamped.Raw.Bytes(), event.Raw.Bytes()) {
		t.Fatalf("StampEvent changed sequence/raw: %#v", stamped)
	}
	if stamped.Response == nil || stamped.Response.Provider != testProvider || stamped.Response.State.Provider != testProvider || stamped.Response.MappingReport.Endpoint != testEndpoint {
		t.Fatalf("stamped event Response = %#v", stamped.Response)
	}
	if stamped.Error == nil || stamped.Error.Provider != testProvider || stamped.Error.Err != nil || stamped.Error.MappingReport.Endpoint != testEndpoint {
		t.Fatalf("stamped event Error = %#v", stamped.Error)
	}
	if event.Response.Provider != "openai" || event.Error.Provider != "openai" || event.Error.Err == nil {
		t.Fatal("StampEvent mutated input event")
	}
}
