package localcompat_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/localcompat"
)

func TestAnonymousLocalChatDoesNotInheritOpenAIEnvironmentCredentials(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "environment-secret")
	t.Setenv("OPENAI_ORG_ID", "environment-org")
	t.Setenv("OPENAI_PROJECT_ID", "environment-project")
	t.Setenv("OPENAI_CUSTOM_HEADERS", "Authorization: Bearer custom-secret\nX-API-Key: custom-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		for _, name := range []string{"Authorization", "X-API-Key", "OpenAI-Organization", "OpenAI-Project", "Cookie", "Proxy-Authorization"} {
			if value := r.Header.Get(name); value != "" {
				t.Errorf("anonymous local request leaked %s=%q", name, value)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"chat-local","object":"chat.completion","created":1,"model":"local-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"local-ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()

	adapter, err := localcompat.New(localcompat.Config{
		Product: localcompat.ProductGeneric, Trust: localcompat.TrustLocal, BaseURL: server.URL + "/v1",
		Protocol: modelinvoker.ProtocolChatCompletions, AllowedModels: []string{"local-model"},
		SupportedCapabilities: []modelinvoker.Capability{
			modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming, modelinvoker.CapabilityUsageReporting,
		},
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := modelinvoker.Request{
		Provider: localcompat.ProviderGeneric, Protocol: modelinvoker.ProtocolChatCompletions,
		Endpoint: server.URL + "/v1", Model: "local-model",
		Input:  []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
		Budget: modelinvoker.Budget{MaxOutputTokens: 8},
	}
	registry, _ := modelinvoker.NewRegistry(adapter)
	invoker, _ := modelinvoker.NewInvoker(registry)
	response, err := invoker.Invoke(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Text() != "local-ok" || response.Provider != localcompat.ProviderGeneric {
		t.Fatalf("unexpected normalized response: %+v", response)
	}
}

func TestLocalProductsHaveDistinctIdentityAndFailClosed(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	definitions := localcompat.Definitions()
	if len(definitions) != 3 {
		t.Fatalf("local product registry count drifted: %d", len(definitions))
	}
	for _, item := range definitions {
		adapter, err := localcompat.New(localcompat.Config{
			Product: item.Product, Trust: localcompat.TrustLocal, BaseURL: server.URL + "/v1",
			Protocol: modelinvoker.ProtocolResponses, AllowedModels: []string{"m"},
			SupportedCapabilities: []modelinvoker.Capability{modelinvoker.CapabilityTextGeneration}, HTTPClient: server.Client(),
		})
		if err != nil {
			t.Fatalf("%s: %v", item.Product, err)
		}
		if adapter.ID() != item.Provider {
			t.Fatalf("%s ID = %q, want %q", item.Product, adapter.ID(), item.Provider)
		}
	}

	invalid := []localcompat.Config{
		{Product: localcompat.ProductGeneric, Trust: localcompat.TrustLocal, BaseURL: "http://example.com/v1", Protocol: modelinvoker.ProtocolChatCompletions, AllowedModels: []string{"m"}, SupportedCapabilities: []modelinvoker.Capability{modelinvoker.CapabilityTextGeneration}},
		{Product: localcompat.ProductGeneric, Trust: localcompat.TrustEnterprise, BaseURL: "http://model.corp/v1", Protocol: modelinvoker.ProtocolChatCompletions, AllowedModels: []string{"m"}, SupportedCapabilities: []modelinvoker.Capability{modelinvoker.CapabilityTextGeneration}},
		{Product: localcompat.ProductGeneric, Trust: localcompat.TrustLocal, BaseURL: server.URL + "/v1", Protocol: modelinvoker.ProtocolMessages, AllowedModels: []string{"m"}, SupportedCapabilities: []modelinvoker.Capability{modelinvoker.CapabilityTextGeneration}},
		{Product: localcompat.ProductGeneric, Trust: localcompat.TrustLocal, BaseURL: server.URL + "/v1", Protocol: modelinvoker.ProtocolChatCompletions, AllowedModels: []string{"m"}},
	}
	for index, config := range invalid {
		if _, err := localcompat.New(config); err == nil {
			t.Fatalf("invalid config %d was accepted", index)
		}
	}
}

func TestEnterpriseEndpointRequiresHTTPSAndUsesConfiguredCredential(t *testing.T) {
	const secret = "enterprise-key"
	t.Setenv("OPENAI_CUSTOM_HEADERS", "Authorization: Bearer wrong-key\nX-API-Key: inherited-key\nOpenAI-Organization: inherited-org")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Errorf("configured enterprise credential missing")
		}
		if r.Header.Get("X-API-Key") != "" || r.Header.Get("OpenAI-Organization") != "" {
			t.Errorf("enterprise request inherited ambient credential headers")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"chat-enterprise","model":"m","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()
	adapter, err := localcompat.New(localcompat.Config{
		Product: localcompat.ProductGeneric, Trust: localcompat.TrustEnterprise, BaseURL: server.URL + "/v1", APIKey: secret,
		Protocol: modelinvoker.ProtocolChatCompletions, AllowedModels: []string{"m"}, SupportedCapabilities: []modelinvoker.Capability{
			modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityUsageReporting,
		}, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := modelinvoker.Request{Provider: localcompat.ProviderGeneric, Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: server.URL + "/v1", Model: "m", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "x")}, Budget: modelinvoker.Budget{MaxOutputTokens: 8}}
	registry, _ := modelinvoker.NewRegistry(adapter)
	invoker, _ := modelinvoker.NewInvoker(registry)
	response, err := invoker.Invoke(context.Background(), request)
	if err != nil || response.Text() != "ok" {
		t.Fatalf("Invoke() = %+v, %v", response, err)
	}
	if strings.Contains(fmt.Sprintf("%#v", adapter), secret) || strings.Contains(response.RawRequest.String(), secret) {
		t.Fatal("enterprise credential leaked")
	}
}
