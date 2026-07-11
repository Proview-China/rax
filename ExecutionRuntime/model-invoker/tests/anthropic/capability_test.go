package anthropic_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/anthropic"
)

func TestCapabilitiesContractAndBoundaries(t *testing.T) {
	adapter, err := provider.New(provider.Config{APIKey: "capability-test-key"})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.ID() != provider.ProviderID || adapter.DefaultProtocol() != modelinvoker.ProtocolMessages {
		t.Fatalf("identity = %q/%q", adapter.ID(), adapter.DefaultProtocol())
	}

	query := modelinvoker.CapabilityQuery{Protocol: modelinvoker.ProtocolMessages, Model: "claude-test-model"}
	contract, err := adapter.Capabilities(context.Background(), query)
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	levels := map[modelinvoker.Capability]modelinvoker.SupportLevel{
		modelinvoker.CapabilityTextGeneration:       modelinvoker.SupportNative,
		modelinvoker.CapabilityStreaming:            modelinvoker.SupportNative,
		modelinvoker.CapabilityToolCalling:          modelinvoker.SupportNative,
		modelinvoker.CapabilityParallelToolCalling:  modelinvoker.SupportNative,
		modelinvoker.CapabilityFunctionErrorResult:  modelinvoker.SupportNative,
		modelinvoker.CapabilityProviderContinuation: modelinvoker.SupportNative,
		modelinvoker.CapabilityPromptCaching:        modelinvoker.SupportUnsupported,
		modelinvoker.CapabilityUsageReporting:       modelinvoker.SupportNative,
		modelinvoker.CapabilityStructuredOutput:     modelinvoker.SupportCompatible,
		modelinvoker.CapabilityReasoning:            modelinvoker.SupportCompatible,
		modelinvoker.CapabilityReasoningSummary:     modelinvoker.SupportCompatible,
		modelinvoker.CapabilityServerState:          modelinvoker.SupportUnsupported,
		modelinvoker.CapabilityVisionInput:          modelinvoker.SupportUnsupported,
		modelinvoker.CapabilityAudioInput:           modelinvoker.SupportUnsupported,
		modelinvoker.CapabilityVideoInput:           modelinvoker.SupportUnsupported,
		modelinvoker.CapabilityFileInput:            modelinvoker.SupportUnsupported,
		modelinvoker.CapabilityBatch:                modelinvoker.SupportUnsupported,
		modelinvoker.CapabilityRealtime:             modelinvoker.SupportUnsupported,
		modelinvoker.CapabilityHostedTools:          modelinvoker.SupportUnsupported,
	}
	for capability, want := range levels {
		support, ok := contract[capability]
		if !ok || support.Level != want {
			t.Errorf("capability %q = %#v, want level %q", capability, support, want)
		}
		if want != modelinvoker.SupportUnsupported || capability == modelinvoker.CapabilityServerState {
			if len(support.Protocols) != 1 || support.Protocols[0] != query.Protocol ||
				len(support.Models) != 1 || support.Models[0] != query.Model {
				t.Errorf("capability %q query bounds = %#v", capability, support)
			}
		}
	}

	_, err = adapter.Capabilities(context.Background(), modelinvoker.CapabilityQuery{
		Protocol: modelinvoker.ProtocolResponses, Model: query.Model,
	})
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorInvalidRequest {
		t.Fatalf("unsupported protocol error = %v", err)
	}
	_, err = adapter.Capabilities(context.Background(), modelinvoker.CapabilityQuery{
		Protocol: modelinvoker.ProtocolMessages, Endpoint: "https://other.example.test", Model: query.Model,
	})
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping {
		t.Fatalf("endpoint mismatch error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = adapter.Capabilities(cancelled, query)
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorCancelled {
		t.Fatalf("cancelled capability error = %v", err)
	}
}

func TestSelectionErrorsPreserveAnthropicKindOperationAndMessage(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "selection-test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		mutate     func(*modelinvoker.Request)
		wantKind   modelinvoker.ErrorKind
		wantDetail string
	}{
		{name: "provider", mutate: func(request *modelinvoker.Request) { request.Provider = "other" }, wantKind: modelinvoker.ErrorInvalidRequest, wantDetail: `request provider "other" does not match "anthropic"`},
		{name: "protocol", mutate: func(request *modelinvoker.Request) { request.Protocol = modelinvoker.ProtocolResponses }, wantKind: modelinvoker.ErrorInvalidRequest, wantDetail: `unsupported protocol "responses"`},
		{name: "endpoint", mutate: func(request *modelinvoker.Request) { request.Endpoint = "https://other.example.test" }, wantKind: modelinvoker.ErrorMapping, wantDetail: "request endpoint does not match the configured Anthropic endpoint"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := baseRequest()
			test.mutate(&request)
			_, err := adapter.Invoke(context.Background(), request)
			var invocationError *modelinvoker.Error
			if !errors.As(err, &invocationError) || invocationError.Kind != test.wantKind ||
				invocationError.Operation != "validate" || !strings.Contains(invocationError.Message, test.wantDetail) {
				t.Fatalf("selection error = %#v (%v)", invocationError, err)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("selection failures made %d HTTP calls", calls.Load())
	}
}
