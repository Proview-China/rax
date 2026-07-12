package azureopenai_test

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
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/azureopenai"
)

type rotatingToken struct{ calls atomic.Int32 }

func (p *rotatingToken) AccessToken(context.Context) (string, time.Time, error) {
	n := p.calls.Add(1)
	return fmt.Sprintf("token-%d", n), time.Now().Add(time.Hour), nil
}

func TestV1AndLegacyBindingsKeepURLAuthAndDeploymentSeparate(t *testing.T) {
	captured := make(chan struct{ path, query, key, auth string }, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured <- struct{ path, query, key, auth string }{r.URL.Path, r.URL.RawQuery, r.Header.Get("api-key"), r.Header.Get("Authorization")}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/openai/v1/responses":
			_, _ = fmt.Fprint(w, `{"id":"resp","model":"deploy-a","status":"completed","output":[{"id":"msg","type":"message","role":"assistant","content":[{"type":"output_text","text":"responses ok","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
		default:
			_, _ = fmt.Fprint(w, `{"id":"chat","model":"deploy-a","choices":[{"index":0,"finish_reason":"stop","message":{"content":"chat ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
		}
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{ResourceEndpoint: server.URL, Region: "eastus", DeploymentName: "deploy-a", CredentialMode: provider.CredentialAPIKey, APIKey: "azure-key", EnableLegacy: true, LegacyAPIVersion: "2024-10-21", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		protocol                    modelinvoker.Protocol
		endpoint, path, query, want string
	}{{modelinvoker.ProtocolResponses, server.URL + "/openai/v1", "/openai/v1/responses", "", "responses ok"}, {modelinvoker.ProtocolChatCompletions, server.URL + "/openai/v1", "/openai/v1/chat/completions", "", "chat ok"}, {modelinvoker.ProtocolChatCompletions, server.URL + "/openai/deployments/deploy-a", "/openai/deployments/deploy-a/chat/completions", "api-version=2024-10-21", "chat ok"}}
	for _, test := range tests {
		r := modelinvoker.Request{Provider: provider.ProviderID, Protocol: test.protocol, Endpoint: test.endpoint, Model: "deploy-a", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}}
		response, err := adapter.Invoke(context.Background(), r)
		if err != nil {
			t.Fatalf("%s: %v", test.endpoint, err)
		}
		native := <-captured
		if native.path != test.path || native.query != test.query || native.key != "azure-key" || native.auth != "" {
			t.Fatalf("native = %#v", native)
		}
		if response.Text() != test.want {
			t.Fatalf("response = %#v", response)
		}
	}
	r := modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolResponses, Endpoint: server.URL + "/openai/v1", Model: "model-id-not-deployment", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "x")}}
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("model ID accepted instead of deployment name")
	}
}

func TestEntraProviderRefreshesWithoutLeakingIntoV1Query(t *testing.T) {
	tokens := &rotatingToken{}
	auth := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("v1 query = %q", r.URL.RawQuery)
		}
		auth <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"chat","model":"deploy-a","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{ResourceEndpoint: server.URL, Region: "eastus", DeploymentName: "deploy-a", CredentialMode: provider.CredentialEntraID, AccessTokenProvider: tokens, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	r := modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: server.URL + "/openai/v1", Model: "deploy-a", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "x")}}
	for range 2 {
		if _, err := adapter.Invoke(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
	if first, second := <-auth, <-auth; first != "Bearer token-1" || second != "Bearer token-2" {
		t.Fatalf("auth = %q / %q", first, second)
	}
	if tokens.calls.Load() != 2 {
		t.Fatalf("token refresh calls = %d", tokens.calls.Load())
	}
}

func TestAzureProjectsNativeModelToDeploymentForInvokeAndStream(t *testing.T) {
	const deployment = "deploy-a"
	const nativeModel = "gpt-native-version-different-from-deployment"
	for _, protocolID := range []modelinvoker.Protocol{modelinvoker.ProtocolChatCompletions, modelinvoker.ProtocolResponses} {
		for _, streaming := range []bool{false, true} {
			name := string(protocolID) + "_invoke"
			if streaming {
				name = string(protocolID) + "_stream"
			}
			t.Run(name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					if streaming {
						writer.Header().Set("Content-Type", "text/event-stream")
						if protocolID == modelinvoker.ProtocolResponses {
							_, _ = fmt.Fprintf(writer, "data: %s\n\ndata: %s\n\n",
								`{"type":"response.created","sequence_number":1,"response":{"id":"resp","model":"`+nativeModel+`","status":"in_progress","output":[]}}`,
								`{"type":"response.completed","sequence_number":2,"response":{"id":"resp","model":"`+nativeModel+`","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}}`)
						} else {
							_, _ = fmt.Fprintf(writer, "data: %s\n\n",
								`{"id":"chat","object":"chat.completion.chunk","model":"`+nativeModel+`","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`)
						}
						return
					}
					writer.Header().Set("Content-Type", "application/json")
					if protocolID == modelinvoker.ProtocolResponses {
						_, _ = fmt.Fprint(writer, `{"id":"resp","model":"`+nativeModel+`","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`)
					} else {
						_, _ = fmt.Fprint(writer, `{"id":"chat","model":"`+nativeModel+`","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
					}
				}))
				defer server.Close()
				adapter, err := provider.New(provider.Config{ResourceEndpoint: server.URL, Region: "eastus", DeploymentName: deployment, CredentialMode: provider.CredentialAPIKey, APIKey: "azure-key", HTTPClient: server.Client()})
				if err != nil {
					t.Fatal(err)
				}
				request := modelinvoker.Request{Provider: provider.ProviderID, Protocol: protocolID, Endpoint: server.URL + "/openai/v1", Model: deployment, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}}
				if !streaming {
					response, err := adapter.Invoke(context.Background(), request)
					if err != nil {
						t.Fatal(err)
					}
					if response.Model != deployment || !strings.Contains(string(response.RawResponse.Bytes()), nativeModel) {
						t.Fatalf("Azure projected/native response = %#v raw=%q", response, response.RawResponse.Bytes())
					}
					return
				}
				stream, err := adapter.Stream(context.Background(), request)
				if err != nil {
					t.Fatal(err)
				}
				defer stream.Close()
				var responses int
				for stream.Next() {
					if response := stream.Event().Response; response != nil {
						responses++
						if response.Model != deployment {
							t.Fatalf("Azure stream portable Model = %q, want deployment %q", response.Model, deployment)
						}
					}
				}
				if err := stream.Err(); err != nil {
					t.Fatal(err)
				}
				if responses == 0 {
					t.Fatal("Azure stream emitted no portable response")
				}
			})
		}
	}
}
