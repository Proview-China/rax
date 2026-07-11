package vertex_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/vertex"
)

type rotatingAccessToken struct{ calls atomic.Int32 }

func (p *rotatingAccessToken) AccessToken(context.Context) (string, time.Time, error) {
	p.calls.Add(1)
	return "vertex-access-token", time.Now().Add(time.Hour), nil
}

func TestVertexGeminiUsesProjectLocationAPIKeyAndSDKHTTPFake(t *testing.T) {
	t.Setenv("GOOGLE_GENAI_USE_VERTEXAI", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GOOGLE_CLOUD_LOCATION", "")
	captured := make(chan struct {
		path, key string
		body      []byte
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured <- struct {
			path, key string
			body      []byte
		}{r.URL.Path, r.Header.Get("x-goog-api-key"), body}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-goog-request-id", "vertex-request")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"vertex ok"}]},"finishReason":"STOP","index":0}],"modelVersion":"gemini-test","responseId":"vertex-response","usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":2,"totalTokenCount":3}}`))
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{Project: "project-a", Location: "us-central1", DeploymentMode: provider.DeploymentServerless, DeploymentRef: "google-publisher", BaseURL: server.URL, CredentialMode: provider.CredentialAPIKey, APIKey: "vertex-test-key", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	r := modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolGenerateContent, Endpoint: server.URL + "/v1", Model: "gemini-test", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}}
	response, err := adapter.Invoke(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	native := <-captured
	if native.key != "vertex-test-key" || !strings.Contains(native.path, "projects/project-a/locations/us-central1") || !json.Valid(native.body) {
		t.Fatalf("native = %#v", native)
	}
	if response.Text() != "vertex ok" || response.Provider != provider.ProviderID || response.Usage.TotalTokens != 3 {
		t.Fatalf("response = %#v", response)
	}
}
func TestVertexCredentialProtocolAndEndpointBoundaries(t *testing.T) {
	if _, err := provider.New(provider.Config{Project: "p", Location: "l", DeploymentMode: provider.DeploymentServerless, DeploymentRef: "d", CredentialMode: provider.CredentialADC, APIKey: "mixed"}); err == nil {
		t.Fatal("mixed ADC/API key accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected call") }))
	defer server.Close()
	adapter, err := provider.New(provider.Config{Project: "p", Location: "l", DeploymentMode: provider.DeploymentProvisionedThroughput, DeploymentRef: "pt", BaseURL: server.URL, CredentialMode: provider.CredentialAPIKey, APIKey: "key", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	r := modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolMessages, Endpoint: server.URL + "/v1", Model: "claude", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "x")}, Budget: modelinvoker.Budget{MaxOutputTokens: 1}}
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("Vertex Claude accepted API-key-only credential mode")
	}
}

func TestVertexClaudeUsesOfficialMiddlewareWithSafeLocalTransport(t *testing.T) {
	tokens := &rotatingAccessToken{}
	captured := make(chan struct {
		path, auth string
		body       map[string]any
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured <- struct {
			path, auth string
			body       map[string]any
		}{r.URL.Path, r.Header.Get("Authorization"), body}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"claude vertex ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":2}}`))
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{Project: "project-a", Location: "us-central1", DeploymentMode: provider.DeploymentServerless, DeploymentRef: "anthropic-publisher", BaseURL: server.URL, CredentialMode: provider.CredentialADC, AccessTokenProvider: tokens, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := server.URL + "/v1/projects/project-a/locations/us-central1/publishers/anthropic"
	request := modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolMessages, Endpoint: endpoint, Model: "claude-test", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}, Budget: modelinvoker.Budget{MaxOutputTokens: 32}}
	response, err := adapter.Invoke(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	native := <-captured
	if native.path != "/v1/projects/project-a/locations/us-central1/publishers/anthropic/models/claude-test:rawPredict" || native.auth != "Bearer vertex-access-token" {
		t.Fatalf("native = %#v", native)
	}
	if _, exists := native.body["model"]; exists {
		t.Fatalf("Vertex middleware retained model in body: %#v", native.body)
	}
	if native.body["anthropic_version"] != "vertex-2023-10-16" || response.Text() != "claude vertex ok" {
		t.Fatalf("body/response = %#v / %#v", native.body, response)
	}
	if tokens.calls.Load() == 0 {
		t.Fatal("access token provider was not used")
	}
}
