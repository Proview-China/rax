package bedrockmantle_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/bedrockmantle"
)

type rotatingKey struct{ calls atomic.Int32 }

func (p *rotatingKey) APIKey(context.Context) (string, time.Time, error) {
	n := p.calls.Add(1)
	return fmt.Sprintf("ABSK-test-%d", n), time.Now().Add(time.Hour), nil
}

func TestAllMantleBindingsUseBedrockIdentityAndLocalSDKFake(t *testing.T) {
	keys := &rotatingKey{}
	captured := make(chan struct{ path, key, authorization string }, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured <- struct{ path, key, authorization string }{r.URL.Path, r.Header.Get("x-api-key"), r.Header.Get("Authorization")}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-amzn-requestid", "mantle-request")
		switch r.URL.Path {
		case "/openai/v1/responses":
			_, _ = fmt.Fprint(w, `{"id":"resp","model":"deployment","status":"completed","output":[{"id":"msg","type":"message","role":"assistant","content":[{"type":"output_text","text":"responses ok","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
		case "/openai/v1/chat/completions":
			_, _ = fmt.Fprint(w, `{"id":"chat","model":"deployment","choices":[{"index":0,"finish_reason":"stop","message":{"content":"chat ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
		case "/anthropic/v1/messages":
			_, _ = fmt.Fprint(w, `{"id":"msg","type":"message","role":"assistant","model":"claude","content":[{"type":"text","text":"messages ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{Region: "us-east-1", ProjectRef: "project-a", BaseURL: server.URL, CredentialMode: provider.CredentialAPIKey, APIKeyProvider: keys, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		protocol       modelinvoker.Protocol
		endpoint, want string
		budget         int64
	}{{modelinvoker.ProtocolResponses, server.URL + "/openai/v1", "responses ok", 0}, {modelinvoker.ProtocolChatCompletions, server.URL + "/openai/v1", "chat ok", 0}, {modelinvoker.ProtocolMessages, server.URL + "/anthropic/v1", "messages ok", 32}}
	for _, test := range tests {
		request := modelinvoker.Request{Provider: provider.ProviderID, Protocol: test.protocol, Endpoint: test.endpoint, Model: "deployment", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}, Budget: modelinvoker.Budget{MaxOutputTokens: test.budget}}
		response, err := adapter.Invoke(context.Background(), request)
		if err != nil {
			t.Fatalf("%s: %v", test.protocol, err)
		}
		if response.Text() != test.want || response.Provider != provider.ProviderID {
			t.Fatalf("%s response = %#v", test.protocol, response)
		}
		native := <-captured
		if native.key == "" || native.authorization != "" || !strings.HasPrefix(native.key, "ABSK-test-") {
			t.Fatalf("%s auth = %#v", test.protocol, native)
		}
	}
	if keys.calls.Load() != 3 {
		t.Fatalf("renewable key calls = %d, want 3", keys.calls.Load())
	}
}

func TestStoreFalseRejectsContinuationAndOpenAIKey(t *testing.T) {
	store := false
	if _, err := provider.New(provider.Config{Region: "us-east-1", ProjectRef: "project", CredentialMode: provider.CredentialAPIKey, APIKey: "sk-proj-wrong", StoreResponses: &store}); err == nil {
		t.Fatal("OpenAI key accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected call") }))
	defer server.Close()
	adapter, err := provider.New(provider.Config{Region: "us-east-1", ProjectRef: "project", BaseURL: server.URL, CredentialMode: provider.CredentialAPIKey, APIKey: "ABSK-test", StoreResponses: &store, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	r := modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolResponses, Endpoint: server.URL + "/openai/v1", Model: "deployment", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "x")}, State: &modelinvoker.State{Kind: modelinvoker.StateServerContinuation, Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolResponses, ID: "prev"}}
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("store=false continuation accepted")
	}
}

func TestMantleSigV4UsesDistinctServiceScope(t *testing.T) {
	authorization := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"chat","model":"deployment","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{Region: "us-east-1", ProjectRef: "project", BaseURL: server.URL, CredentialMode: provider.CredentialSigV4, AccessKeyID: "AKIATEST", SecretAccessKey: "test-secret", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: server.URL + "/openai/v1", Model: "deployment", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}}
	if _, err := adapter.Invoke(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if got := <-authorization; !strings.Contains(got, "/bedrock-mantle/aws4_request") {
		t.Fatalf("SigV4 authorization scope = %q", got)
	}
}
