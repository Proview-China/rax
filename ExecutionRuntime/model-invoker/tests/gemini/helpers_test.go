package gemini_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/gemini"
)

const testAPIKey = "gemini-explicit-test-key"

func baseRequest() modelinvoker.Request {
	return modelinvoker.Request{
		Provider: provider.ProviderID,
		Protocol: modelinvoker.ProtocolGenerateContent,
		Model:    "gemini-test-model",
		Input:    []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Hello")},
		Budget:   modelinvoker.Budget{MaxOutputTokens: 128},
	}
}

func newAdapter(t testing.TB, baseURL string, client *http.Client) *provider.Adapter {
	t.Helper()
	adapter, err := provider.New(provider.Config{APIKey: testAPIKey, BaseURL: baseURL, HTTPClient: client})
	if err != nil {
		t.Fatalf("configure Gemini adapter: %v", err)
	}
	return adapter
}

func weatherTools() []modelinvoker.Tool {
	return []modelinvoker.Tool{{
		Name:        "get_weather",
		Description: "Get the current weather",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"],"additionalProperties":false}`),
	}}
}

func mustFixture(t testing.TB, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}

func decodeBody(t testing.TB, request *http.Request) map[string]any {
	t.Helper()
	defer request.Body.Close()
	var body map[string]any
	decoder := json.NewDecoder(request.Body)
	if err := decoder.Decode(&body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("request body contains trailing JSON: %v", err)
	}
	return body
}

func object(t testing.TB, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value %T is not an object: %#v", value, value)
	}
	return result
}

func array(t testing.TB, value any) []any {
	t.Helper()
	result, ok := value.([]any)
	if !ok {
		t.Fatalf("value %T is not an array: %#v", value, value)
	}
	return result
}

func wantValue(t testing.TB, object map[string]any, key string, want any) {
	t.Helper()
	got, exists := object[key]
	if !exists || got != want {
		t.Fatalf("%s = %#v (exists %v), want %#v", key, got, exists, want)
	}
}

func writeJSON(writer http.ResponseWriter, status int, data []byte) {
	writer.Header().Set("content-type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(data)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
