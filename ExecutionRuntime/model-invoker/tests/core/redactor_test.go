package core_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

func TestAdapterCoreRedactorScrubsEscapedFormsAndFormatsSafely(t *testing.T) {
	secret := `sk-"a/b? c&<`
	redactor := adaptercore.NewRedactor(secret)
	encoded, err := json.Marshal(secret)
	if err != nil {
		t.Fatal(err)
	}
	forms := []string{
		secret,
		string(encoded[1 : len(encoded)-1]),
		url.QueryEscape(secret),
		url.PathEscape(secret),
	}
	for _, value := range []any{redactor, &redactor} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			formatted := fmt.Sprintf(format, value)
			assertNoSecretForms(t, formatted, forms)
		}
	}
	redacted := redactor.String(strings.Join(forms, " | "))
	assertNoSecretForms(t, redacted, forms)
	if !strings.Contains(redacted, "[REDACTED]") {
		t.Fatalf("redacted string = %q", redacted)
	}
}

func TestAdapterCoreRedactorDeepCopiesPublicResults(t *testing.T) {
	secret := `sk-"a/b? c&<`
	redactor := adaptercore.NewRedactor(secret)
	original := modelinvoker.Response{
		ID: secret, Provider: modelinvoker.ProviderID(secret), Protocol: modelinvoker.Protocol(secret), Model: secret,
		Status: modelinvoker.ResponseStatus(secret), StopReason: modelinvoker.StopReason(secret), StopSequence: secret,
		Output: []modelinvoker.OutputItem{
			{Type: modelinvoker.OutputItemText, Text: secret, ReasoningSummary: secret},
			{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{
				ID: secret, Name: secret, Arguments: json.RawMessage(`{"key":"sk-\"a/b? c&\u003c"}`),
			}},
		},
		RequestID: secret,
		Metadata:  modelinvoker.Metadata{secret: secret},
		State: &modelinvoker.State{
			Kind: modelinvoker.StateKind(secret), Provider: modelinvoker.ProviderID(secret), Protocol: modelinvoker.Protocol(secret),
			ID: secret, Payload: modelinvoker.NewRawPayload([]byte(secret)),
		},
		ProviderMetadata: modelinvoker.ProviderMetadata{secret: secret},
		MappingReport: modelinvoker.MappingReport{
			Provider: modelinvoker.ProviderID(secret), Protocol: modelinvoker.Protocol(secret), Endpoint: "https://example.test/" + url.PathEscape(secret),
			Decisions: []modelinvoker.MappingDecision{{Capability: modelinvoker.Capability(secret), Action: modelinvoker.MappingAction(secret), Detail: secret}},
		},
		RawRequest:   modelinvoker.NewRawPayload([]byte(secret)),
		RawResponse:  modelinvoker.NewRawPayload([]byte(`{"error":"sk-\"a/b? c&\u003c"}`)),
		NativeEvents: []modelinvoker.RawPayload{modelinvoker.NewRawPayload([]byte(url.QueryEscape(secret)))},
	}

	result := redactor.Response(original)
	assertResponseHasNoSecret(t, result, secret)
	if original.ID != secret || original.Output[1].FunctionCall.ID != secret || original.Metadata[secret] != secret || string(original.RawRequest.Bytes()) != secret {
		t.Fatal("redaction mutated the source response")
	}
	result.Output[1].FunctionCall.Arguments[0] = 'x'
	result.Metadata["changed"] = "changed"
	result.State.ID = "changed"
	result.MappingReport.Decisions[0].Detail = "changed"
	result.NativeEvents[0] = modelinvoker.NewRawPayload([]byte("changed"))
	if original.Output[1].FunctionCall.Arguments[0] == 'x' || original.State.ID != secret || original.MappingReport.Decisions[0].Detail != secret || string(original.NativeEvents[0].Bytes()) != url.QueryEscape(secret) {
		t.Fatal("redacted response shares mutable storage with its source")
	}
}

func TestAdapterCoreRedactorErrorsAndStreamEventsDoNotExposeCauses(t *testing.T) {
	secret := "sk-secret-cause"
	redactor := adaptercore.NewRedactor(secret)
	cause := &redactorUnsafeError{message: "unsafe " + secret}
	original := &modelinvoker.Error{
		Kind: modelinvoker.ErrorProvider, Provider: "provider", Operation: "operation-" + secret,
		Code: secret, Message: "message " + secret, RequestID: secret,
		MappingReport: modelinvoker.MappingReport{Endpoint: "https://example.test/" + url.PathEscape(secret)},
		Err:           cause,
	}
	redacted, ok := redactor.Error(original).(*modelinvoker.Error)
	if !ok {
		t.Fatalf("redacted error type = %T", redactor.Error(original))
	}
	assertNoSecretForms(t, fmt.Sprintf("%v %+v %#v", redacted, redacted, redacted), []string{secret, url.QueryEscape(secret), url.PathEscape(secret)})
	var exposed *redactorUnsafeError
	if errors.As(redacted, &exposed) || errors.Is(redacted, cause) {
		t.Fatal("redacted error exposed its original cause")
	}

	cancelled := redactor.Error(&modelinvoker.Error{Kind: modelinvoker.ErrorCancelled, Err: context.Canceled})
	deadline := redactor.Error(&modelinvoker.Error{Kind: modelinvoker.ErrorTimeout, Err: context.DeadlineExceeded})
	if !errors.Is(cancelled, context.Canceled) || !errors.Is(deadline, context.DeadlineExceeded) {
		t.Fatal("redactor did not preserve context sentinels")
	}

	event := redactor.StreamEvent(modelinvoker.StreamEvent{
		Type: modelinvoker.StreamEventError, ResponseID: secret, TextDelta: secret, ReasoningDelta: secret,
		ArgumentsDelta: secret, FunctionCall: &modelinvoker.FunctionCall{ID: secret, Name: secret, Arguments: json.RawMessage(`{"key":"sk-secret-cause"}`)},
		Response: &modelinvoker.Response{ID: secret, RawResponse: modelinvoker.NewRawPayload([]byte(secret))},
		Error:    original, Raw: modelinvoker.NewRawPayload([]byte(secret)),
	})
	serialized := fmt.Sprintf("%#v|%s|%s|%s|%s|%s", event, event.Raw.Bytes(), event.Response.RawResponse.Bytes(), event.FunctionCall.Arguments, event.Error, event.Response.ID)
	assertNoSecretForms(t, serialized, []string{secret})
	if errors.As(event.Error, &exposed) {
		t.Fatal("stream event exposed its original cause")
	}
}

func TestAdapterCoreRedactingStreamBlocksSecretsSplitAcrossEvents(t *testing.T) {
	secret := "sk-cross-event-secret"
	split := 8
	redactor := adaptercore.NewRedactor(secret)
	inner := &fakeStream{events: []modelinvoker.StreamEvent{
		{
			Type: modelinvoker.StreamEventTextDelta, Sequence: 10,
			TextDelta: "prefix " + secret[:split], Raw: modelinvoker.NewRawPayload([]byte(`{"delta":"` + secret[:split] + `"}`)),
		},
		{
			Type: modelinvoker.StreamEventTextDelta, Sequence: 11,
			TextDelta: secret[split:] + " suffix", Raw: modelinvoker.NewRawPayload([]byte(`{"delta":"` + secret[split:] + `"}`)),
		},
		{
			Type: modelinvoker.StreamEventReasoningDelta, Sequence: 12,
			ReasoningDelta: secret[:split], Raw: modelinvoker.NewRawPayload([]byte(`{"reasoning":"` + secret[:split] + `"}`)),
		},
		{
			Type: modelinvoker.StreamEventReasoningDelta, Sequence: 13,
			ReasoningDelta: secret[split:], Raw: modelinvoker.NewRawPayload([]byte(`{"reasoning":"` + secret[split:] + `"}`)),
		},
		{
			Type: modelinvoker.StreamEventFunctionArgumentsDelta, Sequence: 14, ArgumentsDelta: `{"token":"` + secret[:split],
			FunctionCall: &modelinvoker.FunctionCall{ID: "call_1", Name: "tool", Arguments: json.RawMessage(`{"token":"` + secret[:split])},
			Raw:          modelinvoker.NewRawPayload([]byte(`{"arguments":"` + secret[:split] + `"}`)),
		},
		{
			Type: modelinvoker.StreamEventFunctionArgumentsDelta, Sequence: 15, ArgumentsDelta: secret[split:] + `"}`,
			FunctionCall: &modelinvoker.FunctionCall{ID: "call_1", Name: "tool", Arguments: json.RawMessage(`{"token":"` + secret + `"}`)},
			Raw:          modelinvoker.NewRawPayload([]byte(`{"arguments":"` + secret[split:] + `"}`)),
		},
		{
			Type: modelinvoker.StreamEventResponseCompleted, Sequence: 16,
			Raw: modelinvoker.NewRawPayload([]byte(`{"done":true}`)),
			Response: &modelinvoker.Response{
				Output:       []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemText, Text: secret}},
				RawResponse:  modelinvoker.NewRawPayload([]byte(secret)),
				NativeEvents: []modelinvoker.RawPayload{modelinvoker.NewRawPayload([]byte(secret[:split])), modelinvoker.NewRawPayload([]byte(secret[split:]))},
			},
		},
	}}
	stream := redactor.Stream(inner)
	defer stream.Close()

	var text, reasoning, arguments, raw string
	var terminal *modelinvoker.Response
	var previousSequence int64
	for stream.Next() {
		event := stream.Event()
		if event.Sequence <= previousSequence {
			t.Fatalf("sequence = %d after %d", event.Sequence, previousSequence)
		}
		previousSequence = event.Sequence
		text += event.TextDelta
		reasoning += event.ReasoningDelta
		arguments += event.ArgumentsDelta
		raw += string(event.Raw.Bytes())
		if event.Response != nil {
			terminal = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if text != "prefix [REDACTED] suffix" || reasoning != "[REDACTED]" || arguments != `{"token":"[REDACTED]"}` {
		t.Fatalf("redacted semantic streams = text:%q reasoning:%q arguments:%q", text, reasoning, arguments)
	}
	if terminal == nil || string(terminal.RawResponse.Bytes()) != "[REDACTED]" || len(terminal.NativeEvents) != 2 {
		t.Fatalf("terminal audit = %#v", terminal)
	}
	var native string
	for _, event := range terminal.NativeEvents {
		native += string(event.Bytes())
	}
	assertNoSecretForms(t, text+reasoning+arguments+raw+native+string(terminal.RawResponse.Bytes()), []string{secret, url.QueryEscape(secret), url.PathEscape(secret)})
}

func TestAdapterCoreRedactingStreamFlushesSafeSuffixBeforeTerminalAndErrorEOF(t *testing.T) {
	for _, test := range []struct {
		name   string
		events []modelinvoker.StreamEvent
		err    error
	}{
		{
			name: "terminal event",
			events: []modelinvoker.StreamEvent{
				{Type: modelinvoker.StreamEventTextDelta, Sequence: 1, TextDelta: "ordinary s"},
				{Type: modelinvoker.StreamEventError, Sequence: 2, Error: &modelinvoker.Error{Kind: modelinvoker.ErrorProvider, Message: "failed"}},
			},
		},
		{
			name:   "error EOF",
			events: []modelinvoker.StreamEvent{{Type: modelinvoker.StreamEventTextDelta, Sequence: 1, TextDelta: "ordinary s"}},
			err:    errors.New("stream failed"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream := adaptercore.NewRedactor("secret-value").Stream(&fakeStream{events: test.events, err: test.err})
			defer stream.Close()
			var text string
			var previousSequence int64
			for stream.Next() {
				event := stream.Event()
				if event.Sequence <= previousSequence {
					t.Fatalf("sequence = %d after %d", event.Sequence, previousSequence)
				}
				previousSequence = event.Sequence
				text += event.TextDelta
			}
			if text != "ordinary s" {
				t.Fatalf("flushed text = %q", text)
			}
			if test.err != nil && (stream.Err() == nil || stream.Err().Error() != test.err.Error()) {
				t.Fatalf("EOF error = %v", stream.Err())
			}
		})
	}
}

func TestAdapterCoreRedactingStreamFormattingNeverExposesPatternsOrPendingFragments(t *testing.T) {
	secret := "stream-format-left--stream-format-right"
	split := len("stream-format-left--")
	stream := adaptercore.NewRedactor(secret).Stream(&fakeStream{events: []modelinvoker.StreamEvent{
		{Type: modelinvoker.StreamEventTextDelta, Sequence: 1, TextDelta: secret[:split]},
		{Type: modelinvoker.StreamEventTextDelta, Sequence: 2, TextDelta: secret[split:]},
		{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 3, Response: &modelinvoker.Response{}},
	}})
	defer stream.Close()

	assertSafe := func(stage string) {
		t.Helper()
		for _, format := range []string{"%v", "%+v", "%#v"} {
			formatted := fmt.Sprintf(format, stream)
			for _, forbidden := range []string{secret, secret[:split], secret[split:]} {
				if strings.Contains(formatted, forbidden) {
					t.Fatalf("%s stream format %s exposed %q: %s", stage, format, forbidden, formatted)
				}
			}
		}
	}

	assertSafe("before read")
	if !stream.Next() {
		t.Fatal("first Next() = false")
	}
	assertSafe("with pending fragment")
	for stream.Next() {
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	assertSafe("after terminal")
}

type redactorUnsafeError struct {
	message string
}

func (e *redactorUnsafeError) Error() string { return e.message }

func assertResponseHasNoSecret(t *testing.T, response modelinvoker.Response, secret string) {
	t.Helper()
	encoded, _ := json.Marshal(secret)
	forms := []string{secret, string(encoded[1 : len(encoded)-1]), url.QueryEscape(secret), url.PathEscape(secret)}
	parts := []string{
		response.ID, string(response.Provider), string(response.Protocol), response.Model, string(response.Status), string(response.StopReason), response.StopSequence,
		response.RequestID, string(response.RawRequest.Bytes()), string(response.RawResponse.Bytes()), response.MappingReport.Endpoint,
	}
	for key, value := range response.Metadata {
		parts = append(parts, key, value)
	}
	for key, value := range response.ProviderMetadata {
		parts = append(parts, key, value)
	}
	if response.State != nil {
		parts = append(parts, string(response.State.Kind), string(response.State.Provider), string(response.State.Protocol), response.State.ID, string(response.State.Payload.Bytes()))
	}
	for _, item := range response.Output {
		parts = append(parts, string(item.Type), item.Text, item.ReasoningSummary)
		if item.FunctionCall != nil {
			parts = append(parts, item.FunctionCall.ID, item.FunctionCall.Name, string(item.FunctionCall.Arguments))
		}
	}
	for _, decision := range response.MappingReport.Decisions {
		parts = append(parts, string(decision.Capability), string(decision.Action), decision.Detail)
	}
	for _, raw := range response.NativeEvents {
		parts = append(parts, string(raw.Bytes()))
	}
	assertNoSecretForms(t, strings.Join(parts, "|"), forms)
}

func assertNoSecretForms(t *testing.T, value string, forms []string) {
	t.Helper()
	for _, form := range forms {
		if form != "" && strings.Contains(value, form) {
			t.Fatalf("value contains secret form %q: %s", form, value)
		}
	}
}
