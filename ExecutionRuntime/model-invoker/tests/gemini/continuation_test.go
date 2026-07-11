package gemini_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/gemini"
)

func TestThoughtSignatureContinuationRoundTrip(t *testing.T) {
	toolFixture := mustFixture(t, "testdata/response-tool-thinking.json")
	textFixture := mustFixture(t, "testdata/response-text.json")
	var calls atomic.Int64
	secondRequest := make(chan map[string]any, 1)
	thirdRequest := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		call := calls.Add(1)
		data, _ := io.ReadAll(request.Body)
		if call == 2 || call == 3 {
			var body map[string]any
			_ = json.Unmarshal(data, &body)
			if call == 2 {
				secondRequest <- body
			} else {
				thirdRequest <- body
			}
		}
		writer.Header().Set("x-goog-request-id", fmt.Sprintf("req_continuation_%d", call))
		if call == 1 {
			writeJSON(writer, http.StatusOK, toolFixture)
			return
		}
		writeJSON(writer, http.StatusOK, textFixture)
	}))
	defer server.Close()
	adapter := newAdapter(t, server.URL, server.Client())

	first := baseRequest()
	first.Tools = weatherTools()
	firstResponse, err := adapter.Invoke(context.Background(), first)
	if err != nil {
		t.Fatalf("first Invoke() error = %v", err)
	}
	callsOut := firstResponse.FunctionCalls()
	if len(callsOut) != 1 || callsOut[0].ID != "call_weather_01" || callsOut[0].Name != "get_weather" ||
		string(callsOut[0].Arguments) != `{"city":"Paris"}` {
		t.Fatalf("first function calls = %#v", callsOut)
	}
	if firstResponse.StopReason != modelinvoker.StopReasonToolCall || firstResponse.State == nil {
		t.Fatalf("first stop/state = %q/%#v", firstResponse.StopReason, firstResponse.State)
	}
	state := firstResponse.State
	if state.Kind != modelinvoker.StateProviderContinuation || state.Provider != provider.ProviderID ||
		state.Protocol != modelinvoker.ProtocolGenerateContent || state.ID != firstResponse.ID {
		t.Fatalf("continuation identity = %#v", state)
	}
	if state.Payload.String() != "[REDACTED]" || !strings.Contains(string(state.Payload.Bytes()), "thoughtSignature") {
		t.Fatalf("continuation payload = %s / %q", state.Payload.String(), state.Payload.Bytes())
	}
	copy := state.Payload.Bytes()
	copy[0] = 'x'
	if state.Payload.Bytes()[0] == 'x' {
		t.Fatal("continuation payload did not return a defensive copy")
	}

	second := baseRequest()
	second.Input = []modelinvoker.InputItem{
		modelinvoker.NamedFunctionResultInput("", "get_weather", "22 C", false),
	}
	second.Tools = weatherTools()
	second.State = state
	secondResponse, err := adapter.Invoke(context.Background(), second)
	if err != nil {
		t.Fatalf("continuation Invoke() error = %v", err)
	}
	if secondResponse.Text() != "Hello from Gemini." || secondResponse.State == nil || calls.Load() != 2 {
		t.Fatalf("second response/calls = %q/%d", secondResponse.Text(), calls.Load())
	}
	if secondResponse.State.ID != secondResponse.ID {
		t.Fatalf("updated continuation identity = %#v", secondResponse.State)
	}
	updatedPayload := string(secondResponse.State.Payload.Bytes())
	if !strings.Contains(updatedPayload, "thoughtSignature") || !strings.Contains(updatedPayload, "Hello from Gemini.") {
		t.Fatalf("updated continuation lost provider history or plain text: %s", updatedPayload)
	}
	mutatedPayload := secondResponse.State.Payload.Bytes()
	mutatedPayload[0] ^= 0xff
	if string(secondResponse.State.Payload.Bytes()) != updatedPayload {
		t.Fatal("updated continuation payload is not defensive")
	}

	body := <-secondRequest
	contents := array(t, body["contents"])
	if len(contents) != 3 {
		t.Fatalf("continued contents count = %d, want original user/model + result", len(contents))
	}
	modelContent := object(t, contents[1])
	wantValue(t, modelContent, "role", "model")
	modelParts := array(t, modelContent["parts"])
	if len(modelParts) != 2 {
		t.Fatalf("continued model parts = %#v", modelParts)
	}
	if object(t, modelParts[0])["thoughtSignature"] != "c2lnX3Rob3VnaHRfMDE=" ||
		object(t, modelParts[1])["thoughtSignature"] != "c2lnX2NhbGxfMDE=" {
		t.Fatalf("thought signatures were not replayed exactly: %#v", modelParts)
	}
	functionResponse := object(t, object(t, array(t, object(t, contents[2])["parts"])[0])["functionResponse"])
	wantValue(t, functionResponse, "id", "call_weather_01")
	wantValue(t, functionResponse, "name", "get_weather")
	wantValue(t, object(t, functionResponse["response"]), "output", "22 C")

	third := baseRequest()
	third.Input = []modelinvoker.InputItem{
		modelinvoker.MessageInput(modelinvoker.RoleUser, "What about tomorrow?"),
	}
	third.Tools = weatherTools()
	third.State = secondResponse.State
	thirdResponse, err := adapter.Invoke(context.Background(), third)
	if err != nil {
		t.Fatalf("third continuation Invoke() error = %v", err)
	}
	if thirdResponse.Text() != "Hello from Gemini." || thirdResponse.State == nil || calls.Load() != 3 {
		t.Fatalf("third response/state/calls = %q/%#v/%d", thirdResponse.Text(), thirdResponse.State, calls.Load())
	}
	if string(secondResponse.State.Payload.Bytes()) != updatedPayload {
		t.Fatal("third-turn mapping mutated the prior continuation")
	}

	thirdBody := <-thirdRequest
	thirdContents := array(t, thirdBody["contents"])
	if len(thirdContents) != 5 {
		t.Fatalf("third-turn contents = %#v", thirdContents)
	}
	continuedText := object(t, array(t, object(t, thirdContents[3])["parts"])[0])
	wantValue(t, continuedText, "text", "Hello from Gemini.")
	nextUser := object(t, array(t, object(t, thirdContents[4])["parts"])[0])
	wantValue(t, nextUser, "text", "What about tomorrow?")
	replayedModelParts := array(t, object(t, thirdContents[1])["parts"])
	if object(t, replayedModelParts[0])["thoughtSignature"] != "c2lnX3Rob3VnaHRfMDE=" ||
		object(t, replayedModelParts[1])["thoughtSignature"] != "c2lnX2NhbGxfMDE=" {
		t.Fatalf("third turn lost thought signatures: %#v", replayedModelParts)
	}
	replayedResponse := object(t, object(t, array(t, object(t, thirdContents[2])["parts"])[0])["functionResponse"])
	wantValue(t, object(t, replayedResponse["response"]), "output", "22 C")
}

func TestPlainTextWithoutProviderHistoryDoesNotCreateContinuation(t *testing.T) {
	textFixture := mustFixture(t, "testdata/response-text.json")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, textFixture)
	}))
	defer server.Close()

	response, err := newAdapter(t, server.URL, server.Client()).Invoke(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if response.State != nil {
		t.Fatalf("plain text response created continuation state: %#v", response.State)
	}
}

func TestContinuationRejectsUncontrolledOrInconsistentPayload(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	adapter := newAdapter(t, server.URL, server.Client())

	tests := []struct {
		name    string
		payload string
	}{
		{name: "unknown envelope field", payload: `{"version":1,"contents":[],"calls":{},"vertexConfig":{}}`},
		{name: "trailing JSON", payload: `{"version":1,"contents":[],"calls":{}} {}`},
		{name: "unsupported inline media", payload: `{"version":1,"contents":[{"role":"model","parts":[{"inlineData":{"mimeType":"text/plain","data":"eA=="}}]}],"calls":{}}`},
		{name: "model function response", payload: `{"version":1,"contents":[{"role":"model","parts":[{"functionResponse":{"name":"lookup","response":{"output":"x"}}}]}],"calls":{}}`},
		{name: "model function call marked as thought", payload: `{"version":1,"contents":[{"role":"model","parts":[{"functionCall":{"id":"call_1","name":"lookup","args":{}},"thought":true,"thoughtSignature":"c2lnX2NhbGw="}]}],"calls":{"call_1":{"name":"lookup","native_id":"call_1"}}}`},
		{name: "model function call with text", payload: `{"version":1,"contents":[{"role":"model","parts":[{"text":"extra","functionCall":{"id":"call_1","name":"lookup","args":{}}}]}],"calls":{"call_1":{"name":"lookup","native_id":"call_1"}}}`},
		{name: "call index missing", payload: `{"version":1,"contents":[{"role":"model","parts":[{"functionCall":{"id":"call_1","name":"lookup","args":{}}}]}],"calls":{}}`},
		{name: "response flag mismatch", payload: `{"version":1,"contents":[{"role":"model","parts":[{"functionCall":{"id":"call_1","name":"lookup","args":{}}}]},{"role":"user","parts":[{"functionResponse":{"id":"call_1","name":"lookup","response":{"output":"x"}}}]}],"calls":{"call_1":{"name":"lookup","native_id":"call_1"}}}`},
		{name: "forged generated ID", payload: `{"version":1,"contents":[{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{}}}]}],"calls":{"not-generated":{"name":"lookup"}}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := baseRequest()
			request.State = &modelinvoker.State{
				Kind: modelinvoker.StateProviderContinuation, Provider: provider.ProviderID,
				Protocol: modelinvoker.ProtocolGenerateContent,
				Payload:  modelinvoker.NewRawPayload([]byte(test.payload)),
			}
			_, err := adapter.Invoke(context.Background(), request)
			if err == nil || modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping {
				t.Fatalf("Invoke() error = %v, want mapping error", err)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid continuation reached HTTP %d times", calls.Load())
	}
}
