package providercontract

import (
	"context"
	"errors"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

// Case describes the SDK-free contract shared by all provider adapters.
type Case struct {
	Provider            modelinvoker.Provider
	Request             modelinvoker.Request
	UnsupportedProtocol modelinvoker.Protocol
}

// BehaviorCase exercises the successful non-streaming and streaming paths of
// a concrete adapter against its provider-specific loopback fixture.
type BehaviorCase struct {
	Provider         modelinvoker.Provider
	InvokeRequest    modelinvoker.Request
	StreamRequest    modelinvoker.Request
	ExpectedEndpoint string
	NativeCalls      func() int64
}

// Run verifies identity, capability completeness, cancellation, and the
// provider/protocol/state/options isolation that must hold before any network
// request is sent.
func Run(t *testing.T, test Case) {
	t.Helper()
	if test.Provider == nil {
		t.Fatal("provider is nil")
	}
	id := test.Provider.ID()
	protocol := test.Provider.DefaultProtocol()
	if id == "" || protocol == modelinvoker.ProtocolAuto {
		t.Fatalf("invalid provider identity %q/%q", id, protocol)
	}
	if test.Request.Provider != id || test.Request.Protocol != protocol {
		t.Fatalf("base request identity %q/%q does not match provider %q/%q", test.Request.Provider, test.Request.Protocol, id, protocol)
	}
	if test.UnsupportedProtocol == modelinvoker.ProtocolAuto || test.UnsupportedProtocol == protocol {
		t.Fatalf("unsupported protocol %q is not a distinct concrete protocol", test.UnsupportedProtocol)
	}

	t.Run("capability contract is complete", func(t *testing.T) {
		contract, err := test.Provider.Capabilities(context.Background(), modelinvoker.CapabilityQuery{
			Protocol: protocol,
			Endpoint: test.Request.Endpoint,
			Model:    test.Request.Model,
		})
		if err != nil {
			t.Fatalf("Capabilities() error = %v", err)
		}
		for _, capability := range adaptercore.KnownCapabilities() {
			support, ok := contract[capability]
			if !ok {
				t.Errorf("capability %q is absent", capability)
				continue
			}
			if !validSupportLevel(support.Level) {
				t.Errorf("capability %q has invalid support level %q", capability, support.Level)
			}
			if len(support.Limitations) == 0 {
				t.Errorf("capability %q has no traceable limitation or mapping note", capability)
			}
			if support.Level != modelinvoker.SupportUnsupported && !containsProtocol(support.Protocols, protocol) {
				t.Errorf("capability %q protocols = %q, want %q", capability, support.Protocols, protocol)
			}
			if support.Level != modelinvoker.SupportUnsupported && !containsString(support.Models, test.Request.Model) {
				t.Errorf("capability %q models = %q, want %q", capability, support.Models, test.Request.Model)
			}
		}
	})

	t.Run("capabilities honor cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := test.Provider.Capabilities(ctx, modelinvoker.CapabilityQuery{Protocol: protocol, Model: test.Request.Model})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Capabilities() error = %v, want context.Canceled", err)
		}
	})

	t.Run("capabilities reject foreign protocol", func(t *testing.T) {
		_, err := test.Provider.Capabilities(context.Background(), modelinvoker.CapabilityQuery{
			Protocol: test.UnsupportedProtocol,
			Model:    test.Request.Model,
		})
		if err == nil {
			t.Fatal("Capabilities() accepted an unsupported protocol")
		}
	})

	t.Run("capabilities reject endpoint mismatch", func(t *testing.T) {
		_, err := test.Provider.Capabilities(context.Background(), modelinvoker.CapabilityQuery{
			Protocol: protocol,
			Endpoint: "https://contract-mismatch.invalid/v1",
			Model:    test.Request.Model,
		})
		if err == nil {
			t.Fatal("Capabilities() accepted an endpoint mismatch")
		}
	})

	t.Run("wrong provider is rejected", func(t *testing.T) {
		request := test.Request
		request.Provider = "contract-other-provider"
		if _, err := test.Provider.Invoke(context.Background(), request); err == nil {
			t.Fatal("Invoke() accepted a foreign provider")
		}
	})

	t.Run("wrong protocol is rejected", func(t *testing.T) {
		request := test.Request
		request.Protocol = test.UnsupportedProtocol
		if _, err := test.Provider.Invoke(context.Background(), request); err == nil {
			t.Fatal("Invoke() accepted a foreign protocol")
		}
	})

	t.Run("endpoint mismatch is rejected", func(t *testing.T) {
		request := test.Request
		request.Endpoint = "https://contract-mismatch.invalid/v1"
		if _, err := test.Provider.Invoke(context.Background(), request); err == nil {
			t.Fatal("Invoke() accepted an endpoint mismatch")
		}
	})

	t.Run("foreign state is rejected", func(t *testing.T) {
		request := test.Request
		request.State = &modelinvoker.State{
			Kind:     modelinvoker.StateProviderContinuation,
			Provider: "contract-other-provider",
			Protocol: protocol,
			Payload:  modelinvoker.NewRawPayload([]byte(`{"version":1}`)),
		}
		if _, err := test.Provider.Invoke(context.Background(), request); err == nil {
			t.Fatal("Invoke() accepted foreign continuation state")
		}
	})

	t.Run("cross protocol state is rejected", func(t *testing.T) {
		request := test.Request
		request.State = &modelinvoker.State{
			Kind:     modelinvoker.StateProviderContinuation,
			Provider: id,
			Protocol: test.UnsupportedProtocol,
			Payload:  modelinvoker.NewRawPayload([]byte(`{"version":1}`)),
		}
		if _, err := test.Provider.Invoke(context.Background(), request); err == nil {
			t.Fatal("Invoke() accepted cross-protocol continuation state")
		}
	})

	t.Run("foreign options are rejected", func(t *testing.T) {
		request := test.Request
		request.ProviderOptions = modelinvoker.ProviderOptions{
			"contract-other-provider": []byte(`{}`),
		}
		if _, err := test.Provider.Invoke(context.Background(), request); err == nil {
			t.Fatal("Invoke() accepted foreign provider options")
		}
	})

	t.Run("invoke honors cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := test.Provider.Invoke(ctx, test.Request)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Invoke() error = %v, want context.Canceled", err)
		}
	})

	t.Run("stream honors cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		stream, err := test.Provider.Stream(ctx, test.Request)
		if stream != nil {
			_ = stream.Close()
			t.Fatal("Stream() returned a stream for a cancelled context")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Stream() error = %v, want context.Canceled", err)
		}
	})

	t.Run("stream rejects foreign identity", func(t *testing.T) {
		request := test.Request
		request.Provider = "contract-other-provider"
		stream, err := test.Provider.Stream(context.Background(), request)
		if stream != nil {
			_ = stream.Close()
			t.Fatal("Stream() returned a stream for a foreign provider")
		}
		if err == nil {
			t.Fatal("Stream() accepted a foreign provider")
		}
	})
}

// RunBehavior applies the same observable response and stream invariants to
// each real Provider adapter without importing a provider SDK.
func RunBehavior(t *testing.T, test BehaviorCase) {
	t.Helper()
	if test.Provider == nil || test.NativeCalls == nil {
		t.Fatal("behavior contract requires a provider and native call counter")
	}
	before := test.NativeCalls()

	t.Run("non-stream response", func(t *testing.T) {
		response, err := test.Provider.Invoke(context.Background(), test.InvokeRequest)
		if err != nil {
			t.Fatalf("Invoke() error = %v", err)
		}
		validateResponse(t, response, test.InvokeRequest, test.ExpectedEndpoint)
		if response.RawRequest.Empty() || response.RawResponse.Empty() {
			t.Fatalf("Invoke() omitted controlled raw payloads: request=%v response=%v", response.RawRequest.Empty(), response.RawResponse.Empty())
		}
	})

	t.Run("stream order and terminal", func(t *testing.T) {
		stream, err := test.Provider.Stream(context.Background(), test.StreamRequest)
		if err != nil {
			t.Fatalf("Stream() error = %v", err)
		}
		if stream == nil {
			t.Fatal("Stream() returned nil")
		}
		var sequence int64
		started := 0
		usage := 0
		terminal := 0
		var final *modelinvoker.Response
		for stream.Next() {
			event := stream.Event()
			sequence++
			if event.Sequence != sequence {
				t.Fatalf("stream sequence = %d, want %d", event.Sequence, sequence)
			}
			switch event.Type {
			case modelinvoker.StreamEventResponseStarted:
				started++
			case modelinvoker.StreamEventUsage:
				usage++
			case modelinvoker.StreamEventResponseCompleted:
				terminal++
				final = event.Response
			case modelinvoker.StreamEventError:
				terminal++
				final = event.Response
			}
		}
		if err := stream.Err(); err != nil {
			t.Fatalf("stream.Err() = %v", err)
		}
		if started != 1 || usage == 0 || terminal != 1 || final == nil {
			t.Fatalf("stream lifecycle started=%d usage=%d terminal=%d final=%v", started, usage, terminal, final != nil)
		}
		validateResponse(t, *final, test.StreamRequest, test.ExpectedEndpoint)
		if final.RawRequest.Empty() || final.RawResponse.Empty() || len(final.NativeEvents) == 0 {
			t.Fatalf("terminal stream omitted audit data: request=%v response=%v native=%d", final.RawRequest.Empty(), final.RawResponse.Empty(), len(final.NativeEvents))
		}
		if stream.Next() {
			t.Fatal("stream produced an event after terminal state")
		}
		if err := stream.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		if err := stream.Close(); err != nil {
			t.Fatalf("second Close() error = %v", err)
		}
	})

	if delta := test.NativeCalls() - before; delta != 2 {
		t.Fatalf("behavior contract made %d native calls, want 2", delta)
	}
}

func validateResponse(t *testing.T, response modelinvoker.Response, request modelinvoker.Request, endpoint string) {
	t.Helper()
	if response.Provider != request.Provider || response.Protocol != request.Protocol || response.Model == "" {
		t.Fatalf("response identity = %q/%q/%q, want %q/%q/non-empty", response.Provider, response.Protocol, response.Model, request.Provider, request.Protocol)
	}
	if response.Status != modelinvoker.ResponseStatusCompleted && response.Status != modelinvoker.ResponseStatusIncomplete {
		t.Fatalf("response status = %q", response.Status)
	}
	if response.MappingReport.Provider != request.Provider || response.MappingReport.Protocol != request.Protocol {
		t.Fatalf("mapping identity = %q/%q", response.MappingReport.Provider, response.MappingReport.Protocol)
	}
	if endpoint != "" && response.MappingReport.Endpoint != endpoint {
		t.Fatalf("mapping endpoint = %q, want %q", response.MappingReport.Endpoint, endpoint)
	}
	if response.RequestID == "" {
		t.Fatal("response request ID is empty")
	}
}

func validSupportLevel(level modelinvoker.SupportLevel) bool {
	switch level {
	case modelinvoker.SupportNative, modelinvoker.SupportCompatible, modelinvoker.SupportPartial, modelinvoker.SupportUnsupported:
		return true
	default:
		return false
	}
}

func containsProtocol(protocols []modelinvoker.Protocol, target modelinvoker.Protocol) bool {
	for _, protocol := range protocols {
		if protocol == target {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
