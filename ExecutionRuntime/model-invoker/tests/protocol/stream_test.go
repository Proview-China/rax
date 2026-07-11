package protocol_test

import (
	"context"
	"errors"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
)

func TestIdentityBoundStreamStampsEventsTerminalErrorAndClose(t *testing.T) {
	binding := mustBinding(t)
	request := validRequest("")
	native := &fakeSDKError{message: "native stream failure"}
	poisonedResponse := &modelinvoker.Response{
		Provider: "openai", Protocol: modelinvoker.ProtocolResponses,
		State: &modelinvoker.State{
			Kind: modelinvoker.StateServerContinuation, Provider: "openai", Protocol: modelinvoker.ProtocolResponses,
			ID: "previous", Payload: modelinvoker.NewRawPayload([]byte(`{"state":true}`)),
		},
		MappingReport: modelinvoker.MappingReport{
			Provider: "openai", Protocol: modelinvoker.ProtocolResponses, Endpoint: "https://wrong.example.test",
			Decisions: []modelinvoker.MappingDecision{{Capability: modelinvoker.CapabilityStreaming, Action: modelinvoker.MappingExact, Detail: "preserve-stream-decision"}},
		},
	}
	poisonedError := &modelinvoker.Error{
		Kind: modelinvoker.ErrorProviderUnavailable, Provider: "openai", Message: "stream failed", Err: native,
		MappingReport: modelinvoker.MappingReport{Provider: "openai", Protocol: modelinvoker.ProtocolResponses, Endpoint: "https://wrong.example.test"},
	}
	inner := &fakeStream{
		events: []modelinvoker.StreamEvent{
			{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 41, Response: poisonedResponse, Raw: modelinvoker.NewRawPayload([]byte(`{"done":true}`))},
			{Type: modelinvoker.StreamEventError, Sequence: 42, Response: poisonedResponse, Error: poisonedError, Raw: modelinvoker.NewRawPayload([]byte(`{"error":true}`))},
		},
		terminal: poisonedError,
		closeErr: &modelinvoker.Error{
			Kind: modelinvoker.ErrorStreamInterrupted, Provider: "openai", Message: "close failed", Err: native,
			MappingReport: modelinvoker.MappingReport{Provider: "openai", Protocol: modelinvoker.ProtocolResponses, Endpoint: "https://wrong.example.test"},
		},
	}
	stream := binding.BindStream(context.Background(), request, inner)
	if stream == nil {
		t.Fatal("BindStream() returned nil")
	}

	var sequences []int64
	for stream.Next() {
		event := stream.Event()
		sequences = append(sequences, event.Sequence)
		if event.Response == nil || event.Response.Provider != testProvider || event.Response.Protocol != modelinvoker.ProtocolChatCompletions ||
			event.Response.State == nil || event.Response.State.Provider != testProvider || event.Response.State.Protocol != modelinvoker.ProtocolChatCompletions ||
			event.Response.MappingReport.Provider != testProvider || event.Response.MappingReport.Endpoint != testEndpoint {
			t.Fatalf("stream Response identity = %#v", event.Response)
		}
		if event.Type == modelinvoker.StreamEventError {
			if event.Error == nil || event.Error.Provider != testProvider || event.Error.Err != nil || event.Error.MappingReport.Endpoint != testEndpoint {
				t.Fatalf("stream Error identity = %#v", event.Error)
			}
			var exposed *fakeSDKError
			if errors.As(event.Error, &exposed) {
				t.Fatal("stream Error event retained native SDK error")
			}
		}
	}
	if len(sequences) != 2 || sequences[0] != 41 || sequences[1] != 42 {
		t.Fatalf("stream sequences = %v", sequences)
	}
	terminalCalls := inner.nextCalls
	if stream.Next() || inner.nextCalls != terminalCalls {
		t.Fatalf("Next after terminal state reached inner stream: %d -> %d", terminalCalls, inner.nextCalls)
	}
	var terminal *modelinvoker.Error
	if !errors.As(stream.Err(), &terminal) || terminal.Provider != testProvider || terminal.Err != nil || terminal.MappingReport.Endpoint != testEndpoint {
		t.Fatalf("terminal stream error = %#v", stream.Err())
	}
	closeErr := stream.Close()
	var closed *modelinvoker.Error
	if !errors.As(closeErr, &closed) || closed.Provider != testProvider || closed.Operation != "stream_close" || closed.Err != nil || closed.MappingReport.Endpoint != testEndpoint {
		t.Fatalf("stream Close error = %#v", closeErr)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if inner.closeCalls != 1 {
		t.Fatalf("inner Close calls = %d, want 1", inner.closeCalls)
	}
	if poisonedResponse.Provider != "openai" || poisonedError.Provider != "openai" || poisonedError.Err != native {
		t.Fatal("identity-bound stream mutated inner event values")
	}
}

func TestIdentityBoundStreamContextCauseIsReducedToSentinel(t *testing.T) {
	custom := errors.New("custom stream cancellation cause")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(custom)
	inner := &fakeStream{terminal: custom}
	stream := mustBinding(t).BindStream(ctx, validRequest(""), inner)
	if stream.Next() {
		t.Fatal("empty stream Next() = true")
	}
	err := stream.Err()
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError.Kind != modelinvoker.ErrorCancelled || invocationError.Err != context.Canceled ||
		!errors.Is(err, context.Canceled) || errors.Is(err, custom) {
		t.Fatalf("cancelled stream error = %#v", err)
	}
}

func TestBindingMustWrapOutsideRedactionToRestoreIdentity(t *testing.T) {
	const identity = "identity-secret"
	binding, err := protocol.NewBinding(identity, modelinvoker.ProtocolChatCompletions, testEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	request := validRequest("")
	request.Provider = identity
	inner := &fakeStream{events: []modelinvoker.StreamEvent{{
		Type: modelinvoker.StreamEventResponseCompleted, Sequence: 1,
		Response: &modelinvoker.Response{
			Provider: identity, Protocol: modelinvoker.ProtocolChatCompletions,
			State:         &modelinvoker.State{Kind: modelinvoker.StateServerContinuation, Provider: identity, Protocol: modelinvoker.ProtocolChatCompletions, ID: "previous"},
			MappingReport: modelinvoker.MappingReport{Provider: identity, Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: testEndpoint},
			RawResponse:   modelinvoker.NewRawPayload([]byte(identity)),
		},
		Raw: modelinvoker.NewRawPayload([]byte(identity)),
	}}}
	redacted := adaptercore.NewRedactor(identity).Stream(inner)
	stream := binding.BindStream(context.Background(), request, redacted)
	if !stream.Next() {
		t.Fatalf("wrapped stream Next() = false, err = %v", stream.Err())
	}
	event := stream.Event()
	if event.Response == nil || event.Response.Provider != identity || event.Response.State.Provider != identity || event.Response.MappingReport.Provider != identity {
		t.Fatalf("Binding did not restore authoritative identity after redaction: %#v", event.Response)
	}
	if got := string(event.Raw.Bytes()); got != "[REDACTED]" {
		t.Fatalf("event Raw after redaction = %q", got)
	}
	if got := string(event.Response.RawResponse.Bytes()); got != "[REDACTED]" {
		t.Fatalf("response Raw after redaction = %q", got)
	}
}

func TestBindStreamRejectsNilAndTypedNilStreams(t *testing.T) {
	binding := mustBinding(t)
	request := validRequest("")
	if stream := binding.BindStream(context.Background(), request, nil); stream != nil {
		t.Fatalf("BindStream(nil) = %#v", stream)
	}
	var typedNil *fakeStream
	if stream := binding.BindStream(context.Background(), request, typedNil); stream != nil {
		t.Fatalf("BindStream(typed nil) = %#v", stream)
	}
}
