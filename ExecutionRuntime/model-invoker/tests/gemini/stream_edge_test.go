package gemini_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func TestIDLessFunctionCallTerminalSnapshotDedup(t *testing.T) {
	const (
		callWithoutSignature = `{"functionCall":{"name":"get_weather","args":{"city":"Rome"}}}`
		callWithSignature    = `{"functionCall":{"name":"get_weather","args":{"city":"Rome"}},"thoughtSignature":"c2lnX2NhbGw="}`
	)
	tests := []struct {
		name          string
		initialParts  string
		terminalParts string
		wantCalls     int
		wantSignature bool
	}{
		{name: "without thought signature", initialParts: callWithoutSignature, terminalParts: callWithoutSignature, wantCalls: 1},
		{name: "with thought signature", initialParts: callWithSignature, terminalParts: callWithSignature, wantCalls: 1, wantSignature: true},
		{name: "signature added by terminal snapshot", initialParts: callWithoutSignature, terminalParts: callWithSignature, wantCalls: 1, wantSignature: true},
		{
			name:          "same chunk parallel identical calls",
			initialParts:  callWithoutSignature + "," + callWithoutSignature,
			terminalParts: callWithoutSignature + "," + callWithoutSignature,
			wantCalls:     2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int64
			streamBody := fmt.Sprintf(
				"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[%s]},\"index\":0}],\"responseId\":\"resp_idless\"}\n\n"+
					"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[%s]},\"finishReason\":\"STOP\",\"index\":0}],\"responseId\":\"resp_idless\"}\n\n",
				test.initialParts,
				test.terminalParts,
			)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				writer.Header().Set("content-type", "text/event-stream")
				_, _ = writer.Write([]byte(streamBody))
			}))
			defer server.Close()

			request := baseRequest()
			request.Tools = weatherTools()
			stream, err := newAdapter(t, server.URL, server.Client()).Stream(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			var final *modelinvoker.Response
			completedCalls := 0
			for stream.Next() {
				event := stream.Event()
				if event.Type == modelinvoker.StreamEventFunctionCallCompleted {
					completedCalls++
				}
				if event.Type == modelinvoker.StreamEventResponseCompleted {
					final = event.Response
				}
			}
			if err := stream.Err(); err != nil {
				t.Fatalf("stream Err() = %v", err)
			}
			if err := stream.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if final == nil || final.State == nil {
				t.Fatalf("terminal response = %#v", final)
			}
			calls := final.FunctionCalls()
			if len(calls) != test.wantCalls || completedCalls != test.wantCalls {
				t.Fatalf("calls/events = %d/%d, want %d; calls = %#v", len(calls), completedCalls, test.wantCalls, calls)
			}
			ids := make(map[string]struct{}, len(calls))
			for _, call := range calls {
				if call.ID == "" || call.Name != "get_weather" || string(call.Arguments) != `{"city":"Rome"}` {
					t.Fatalf("normalized call = %#v", call)
				}
				ids[call.ID] = struct{}{}
			}
			if len(ids) != test.wantCalls {
				t.Fatalf("semantic call IDs are not distinct: %#v", calls)
			}
			payload := string(final.State.Payload.Bytes())
			if strings.Contains(payload, "c2lnX2NhbGw=") != test.wantSignature {
				t.Fatalf("continuation signature presence = %v, payload = %s", strings.Contains(payload, "c2lnX2NhbGw="), payload)
			}
			if len(final.NativeEvents) != 2 {
				t.Fatalf("NativeEvents = %d, repeated snapshot must remain auditable", len(final.NativeEvents))
			}
			if requests.Load() != 1 {
				t.Fatalf("HTTP requests = %d, stream was replayed", requests.Load())
			}
		})
	}
}

func TestNativeIDFunctionCallTerminalSnapshotSignatureMerge(t *testing.T) {
	const (
		signatureA = "c2lnX25hdGl2ZV9h"
		signatureB = "c2lnX25hdGl2ZV9i"
	)
	tests := []struct {
		name              string
		initialSignature  string
		terminalSignature string
		wantError         bool
	}{
		{name: "signature added by terminal snapshot", terminalSignature: signatureA},
		{name: "conflicting terminal signature", initialSignature: signatureA, terminalSignature: signatureB, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			part := func(signature string) string {
				result := `{"functionCall":{"id":"call_native_01","name":"get_weather","args":{"city":"Rome"}}`
				if signature != "" {
					result += `,"thoughtSignature":"` + signature + `"`
				}
				return result + `}`
			}
			streamBody := fmt.Sprintf(
				"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[%s]},\"index\":0}],\"responseId\":\"resp_native\"}\n\n"+
					"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[%s]},\"finishReason\":\"STOP\",\"index\":0}],\"responseId\":\"resp_native\"}\n\n",
				part(test.initialSignature),
				part(test.terminalSignature),
			)
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				writer.Header().Set("content-type", "text/event-stream")
				_, _ = writer.Write([]byte(streamBody))
			}))
			defer server.Close()

			request := baseRequest()
			request.Tools = weatherTools()
			stream, err := newAdapter(t, server.URL, server.Client()).Stream(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			var final *modelinvoker.Response
			completedCalls := 0
			errorEvents := 0
			for stream.Next() {
				switch event := stream.Event(); event.Type {
				case modelinvoker.StreamEventFunctionCallCompleted:
					completedCalls++
				case modelinvoker.StreamEventResponseCompleted:
					final = event.Response
				case modelinvoker.StreamEventError:
					errorEvents++
				}
			}
			if requests.Load() != 1 || completedCalls != 1 {
				t.Fatalf("requests/completed calls = %d/%d", requests.Load(), completedCalls)
			}
			if test.wantError {
				if modelinvoker.ErrorKindOf(stream.Err()) != modelinvoker.ErrorProvider ||
					!strings.Contains(stream.Err().Error(), "changed its thought signature") || errorEvents != 1 || final != nil {
					t.Fatalf("conflicting signature result = final %#v, errors %d, err %v", final, errorEvents, stream.Err())
				}
				return
			}
			if stream.Err() != nil || errorEvents != 0 || final == nil || final.State == nil {
				t.Fatalf("terminal result = final %#v, errors %d, err %v", final, errorEvents, stream.Err())
			}
			calls := final.FunctionCalls()
			payload := string(final.State.Payload.Bytes())
			if len(calls) != 1 || calls[0].ID != "call_native_01" ||
				!strings.Contains(payload, test.terminalSignature) {
				t.Fatalf("terminal calls/state = %#v / %s", calls, payload)
			}
		})
	}
}

func TestParentContextCancellationAfterStreamStart(t *testing.T) {
	var requests atomic.Int64
	handlerCancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.Header().Set("content-type", "text/event-stream")
		flusher, ok := writer.(http.Flusher)
		if !ok {
			return
		}
		_, _ = writer.Write([]byte("data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"started\"}]},\"index\":0}],\"responseId\":\"resp_parent_cancel\"}\n\n"))
		flusher.Flush()
		<-request.Context().Done()
		close(handlerCancelled)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := newAdapter(t, server.URL, server.Client()).Stream(ctx, baseRequest())
	if err != nil {
		t.Fatal(err)
	}
	textSeen := false
	for !textSeen && stream.Next() {
		if event := stream.Event(); event.Type == modelinvoker.StreamEventTextDelta {
			textSeen = event.TextDelta == "started"
		}
	}
	if !textSeen {
		t.Fatal("stream did not start before cancellation")
	}
	cancel()

	var partial *modelinvoker.Response
	errorEvents := 0
	for stream.Next() {
		if event := stream.Event(); event.Type == modelinvoker.StreamEventError {
			errorEvents++
			partial = event.Response
		}
	}
	if modelinvoker.ErrorKindOf(stream.Err()) != modelinvoker.ErrorCancelled || !errors.Is(stream.Err(), context.Canceled) {
		t.Fatalf("stream error = %v", stream.Err())
	}
	if errorEvents != 1 || partial == nil || partial.Status != modelinvoker.ResponseStatusCancelled || partial.Text() != "started" {
		t.Fatalf("cancelled partial response/events = %#v/%d", partial, errorEvents)
	}
	select {
	case <-handlerCancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("parent cancellation did not stop the HTTP iterator")
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if stream.Next() {
		t.Fatal("cancelled/closed stream produced another event")
	}
	if requests.Load() != 1 {
		t.Fatalf("HTTP requests = %d, cancelled stream was replayed", requests.Load())
	}
}

func TestStreamContinuationSurvivesPlainTextToolResultTurn(t *testing.T) {
	toolStream := mustFixture(t, "testdata/stream-tool-thinking.sse")
	textFixture := mustFixture(t, "testdata/response-text.json")
	plainTextStream := `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Weather result accepted."}]},"finishReason":"STOP","index":0}],"responseId":"resp_stream_after_tool"}` + "\n\n"
	thirdRequest := make(chan map[string]any, 1)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch requests.Add(1) {
		case 1:
			writer.Header().Set("content-type", "text/event-stream")
			_, _ = writer.Write(toolStream)
		case 2:
			writer.Header().Set("content-type", "text/event-stream")
			_, _ = writer.Write([]byte(plainTextStream))
		default:
			data, _ := io.ReadAll(request.Body)
			var body map[string]any
			_ = json.Unmarshal(data, &body)
			thirdRequest <- body
			writeJSON(writer, http.StatusOK, textFixture)
		}
	}))
	defer server.Close()
	adapter := newAdapter(t, server.URL, server.Client())

	firstRequest := baseRequest()
	firstRequest.Tools = weatherTools()
	firstStream, err := adapter.Stream(context.Background(), firstRequest)
	if err != nil {
		t.Fatalf("first Stream() error = %v", err)
	}
	firstResponse := completedGeminiStreamResponse(t, firstStream)
	if err := firstStream.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	firstCalls := firstResponse.FunctionCalls()
	if firstResponse.State == nil || len(firstCalls) != 1 {
		t.Fatalf("first stream response = %#v", firstResponse)
	}

	secondRequest := baseRequest()
	secondRequest.Input = []modelinvoker.InputItem{
		modelinvoker.FunctionResultInput(firstCalls[0].ID, "21 C", false),
	}
	secondRequest.Tools = weatherTools()
	secondRequest.State = firstResponse.State
	secondStream, err := adapter.Stream(context.Background(), secondRequest)
	if err != nil {
		t.Fatalf("second Stream() error = %v", err)
	}
	secondResponse := completedGeminiStreamResponse(t, secondStream)
	if err := secondStream.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if secondResponse.Text() != "Weather result accepted." || secondResponse.State == nil ||
		secondResponse.State.ID != secondResponse.ID {
		t.Fatalf("plain-text tool-result response = %#v", secondResponse)
	}
	stateBefore := string(secondResponse.State.Payload.Bytes())
	if !strings.Contains(stateBefore, "thoughtSignature") || !strings.Contains(stateBefore, "Weather result accepted.") {
		t.Fatalf("updated stream continuation = %s", stateBefore)
	}
	mutatedState := secondResponse.State.Payload.Bytes()
	mutatedState[0] ^= 0xff
	if string(secondResponse.State.Payload.Bytes()) != stateBefore {
		t.Fatal("updated stream continuation is not defensive")
	}

	third := baseRequest()
	third.Input = []modelinvoker.InputItem{
		modelinvoker.MessageInput(modelinvoker.RoleUser, "Continue with tomorrow."),
	}
	third.Tools = weatherTools()
	third.State = secondResponse.State
	thirdResponse, err := adapter.Invoke(context.Background(), third)
	if err != nil {
		t.Fatalf("third Invoke() error = %v", err)
	}
	if thirdResponse.State == nil || requests.Load() != 3 {
		t.Fatalf("third response/state/requests = %#v/%d", thirdResponse.State, requests.Load())
	}
	if string(secondResponse.State.Payload.Bytes()) != stateBefore {
		t.Fatal("third-turn mapping mutated the prior stream continuation")
	}

	thirdBody := <-thirdRequest
	contents := array(t, thirdBody["contents"])
	if len(contents) != 5 {
		t.Fatalf("third stream-chain contents = %#v", contents)
	}
	modelParts := array(t, object(t, contents[1])["parts"])
	if len(modelParts) != 3 || object(t, modelParts[1])["thoughtSignature"] != "c2lnX3N0cmVhbV90aG91Z2h0" ||
		object(t, modelParts[2])["thoughtSignature"] != "c2lnX3N0cmVhbV9jYWxs" {
		t.Fatalf("third stream-chain model parts = %#v", modelParts)
	}
	functionResponse := object(t, object(t, array(t, object(t, contents[2])["parts"])[0])["functionResponse"])
	wantValue(t, object(t, functionResponse["response"]), "output", "21 C")
	continuedText := object(t, array(t, object(t, contents[3])["parts"])[0])
	wantValue(t, continuedText, "text", "Weather result accepted.")
	nextUser := object(t, array(t, object(t, contents[4])["parts"])[0])
	wantValue(t, nextUser, "text", "Continue with tomorrow.")
}

func TestPlainTextStreamWithoutProviderHistoryDoesNotCreateContinuation(t *testing.T) {
	plainTextStream := `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"plain"}]},"finishReason":"STOP","index":0}],"responseId":"resp_plain"}` + "\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("content-type", "text/event-stream")
		_, _ = writer.Write([]byte(plainTextStream))
	}))
	defer server.Close()

	stream, err := newAdapter(t, server.URL, server.Client()).Stream(context.Background(), baseRequest())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	response := completedGeminiStreamResponse(t, stream)
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if response.State != nil {
		t.Fatalf("plain text stream created continuation state: %#v", response.State)
	}
}

func TestPartialToolContinuationSurvivesProviderErrorAndEOF(t *testing.T) {
	tests := []struct {
		name       string
		trailer    string
		wantKind   modelinvoker.ErrorKind
		wantEvents int
	}{
		{
			name:       "provider error",
			trailer:    `{"error":{"code":503,"message":"temporarily unavailable","status":"UNAVAILABLE"}}` + "\n\n",
			wantKind:   modelinvoker.ErrorProviderUnavailable,
			wantEvents: 2,
		},
		{name: "unexpected EOF", wantKind: modelinvoker.ErrorStreamInterrupted, wantEvents: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			followupBody := make(chan map[string]any, 1)
			var streamRequests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if strings.Contains(request.URL.Path, ":streamGenerateContent") {
					streamRequests.Add(1)
					writer.Header().Set("content-type", "text/event-stream")
					writer.Header().Set("x-goog-request-id", "req_partial")
					chunk := `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"private thought","thought":true,"thoughtSignature":"c2lnX3Rob3VnaHQ="},{"functionCall":{"name":"get_weather","args":{"city":"Oslo"}},"thoughtSignature":"c2lnX2NhbGw="}]},"index":0}],"responseId":"resp_partial"}` + "\n\n"
					_, _ = writer.Write([]byte(chunk + test.trailer))
					return
				}
				data, _ := io.ReadAll(request.Body)
				var body map[string]any
				_ = json.Unmarshal(data, &body)
				followupBody <- body
				writeJSON(writer, http.StatusOK, mustFixture(t, "testdata/response-text.json"))
			}))
			defer server.Close()
			adapter := newAdapter(t, server.URL, server.Client())

			request := baseRequest()
			request.Tools = weatherTools()
			stream, err := adapter.Stream(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			var partial *modelinvoker.Response
			for stream.Next() {
				if event := stream.Event(); event.Type == modelinvoker.StreamEventError {
					partial = event.Response
				}
			}
			if modelinvoker.ErrorKindOf(stream.Err()) != test.wantKind {
				t.Fatalf("stream error = %v, want %q", stream.Err(), test.wantKind)
			}
			if err := stream.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if partial == nil || partial.State == nil || partial.Status != modelinvoker.ResponseStatusFailed ||
				partial.RequestID != "req_partial" || len(partial.NativeEvents) != test.wantEvents {
				t.Fatalf("partial response = %#v", partial)
			}
			calls := partial.FunctionCalls()
			if len(calls) != 1 || calls[0].ID == "" || calls[0].Name != "get_weather" {
				t.Fatalf("partial calls = %#v", calls)
			}
			stateBefore := partial.State.Payload.Bytes()
			if !bytes.Contains(stateBefore, []byte("c2lnX3Rob3VnaHQ=")) || !bytes.Contains(stateBefore, []byte("c2lnX2NhbGw=")) {
				t.Fatalf("partial continuation lost thought signatures: %s", stateBefore)
			}
			mutated := partial.State.Payload.Bytes()
			mutated[0] ^= 0xff
			if !bytes.Equal(partial.State.Payload.Bytes(), stateBefore) {
				t.Fatal("partial continuation payload is not defensive")
			}

			followup := baseRequest()
			followup.Input = []modelinvoker.InputItem{modelinvoker.FunctionResultInput(calls[0].ID, "21 C", false)}
			followup.Tools = weatherTools()
			followup.State = partial.State
			response, err := adapter.Invoke(context.Background(), followup)
			if err != nil {
				t.Fatalf("continuation Invoke() error = %v", err)
			}
			if response.Text() != "Hello from Gemini." {
				t.Fatalf("followup text = %q", response.Text())
			}
			body := <-followupBody
			contents := array(t, body["contents"])
			if len(contents) != 3 {
				t.Fatalf("followup contents = %#v", contents)
			}
			modelParts := array(t, object(t, contents[1])["parts"])
			if object(t, modelParts[0])["thoughtSignature"] != "c2lnX3Rob3VnaHQ=" ||
				object(t, modelParts[1])["thoughtSignature"] != "c2lnX2NhbGw=" {
				t.Fatalf("followup signatures = %#v", modelParts)
			}
			functionResponse := object(t, object(t, array(t, object(t, contents[2])["parts"])[0])["functionResponse"])
			wantValue(t, functionResponse, "name", "get_weather")
			if _, exists := functionResponse["id"]; exists {
				t.Fatalf("ID-less native call gained response ID: %#v", functionResponse)
			}
			if !bytes.Equal(partial.State.Payload.Bytes(), stateBefore) {
				t.Fatal("continuation mapping mutated the caller state")
			}
			if streamRequests.Load() != 1 {
				t.Fatalf("stream requests = %d, error path replayed", streamRequests.Load())
			}
		})
	}
}

func completedGeminiStreamResponse(t testing.TB, stream modelinvoker.Stream) *modelinvoker.Response {
	t.Helper()
	var response *modelinvoker.Response
	for stream.Next() {
		if event := stream.Event(); event.Type == modelinvoker.StreamEventResponseCompleted {
			response = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream Err() = %v", err)
	}
	if response == nil {
		t.Fatal("stream did not produce a completed response")
	}
	return response
}
