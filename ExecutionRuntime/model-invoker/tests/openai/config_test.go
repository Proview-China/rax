package openai_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	openaiadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/openai"
)

func TestPublicConfigAndNewSafetyValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  openaiadapter.Config
		wantErr bool
	}{
		{name: "default endpoint", config: openaiadapter.Config{APIKey: "test-key"}},
		{name: "HTTPS endpoint", config: openaiadapter.Config{APIKey: "test-key", BaseURL: "https://example.com/v1"}},
		{name: "IPv4 loopback HTTP", config: openaiadapter.Config{APIKey: "test-key", BaseURL: "http://127.0.0.1:1234/v1"}},
		{name: "IPv6 loopback HTTP", config: openaiadapter.Config{APIKey: "test-key", BaseURL: "http://[::1]:1234/v1"}},
		{name: "localhost HTTP", config: openaiadapter.Config{APIKey: "test-key", BaseURL: "http://localhost:1234/v1"}},
		{name: "custom HTTP client", config: openaiadapter.Config{APIKey: "test-key", HTTPClient: &http.Client{}}},
		{name: "missing key", config: openaiadapter.Config{}, wantErr: true},
		{name: "blank key", config: openaiadapter.Config{APIKey: " \t"}, wantErr: true},
		{name: "relative endpoint", config: openaiadapter.Config{APIKey: "test-key", BaseURL: "/v1"}, wantErr: true},
		{name: "unsupported scheme", config: openaiadapter.Config{APIKey: "test-key", BaseURL: "ftp://example.com/v1"}, wantErr: true},
		{name: "remote insecure HTTP", config: openaiadapter.Config{APIKey: "test-key", BaseURL: "http://example.com/v1"}, wantErr: true},
		{name: "embedded credentials", config: openaiadapter.Config{APIKey: "secret-value", BaseURL: "https://user:pass@example.com/v1"}, wantErr: true},
		{name: "query", config: openaiadapter.Config{APIKey: "test-key", BaseURL: "https://example.com/v1?token=value"}, wantErr: true},
		{name: "fragment", config: openaiadapter.Config{APIKey: "test-key", BaseURL: "https://example.com/v1#fragment"}, wantErr: true},
		{name: "malformed endpoint", config: openaiadapter.Config{APIKey: "test-key", BaseURL: "://bad"}, wantErr: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			adapter, err := openaiadapter.New(test.config)
			if test.wantErr {
				got := assertPublicErrorKind(t, err, modelinvoker.ErrorInvalidRequest)
				if got.Operation != "configure" {
					t.Fatalf("operation = %q, want configure", got.Operation)
				}
				if test.config.APIKey != "" && strings.Contains(err.Error(), test.config.APIKey) {
					t.Fatal("configuration error leaked API key")
				}
				return
			}
			if err != nil || adapter == nil {
				t.Fatalf("New() = (%v, %v), want adapter and nil error", adapter, err)
			}
			if adapter.ID() != openaiadapter.ProviderID || adapter.DefaultProtocol() != modelinvoker.ProtocolResponses {
				t.Fatalf("adapter identity/default protocol = %q/%q", adapter.ID(), adapter.DefaultProtocol())
			}
		})
	}
}

func TestPublicConfigValidationAndFormattingDoNotExposeAPIKey(t *testing.T) {
	secret := "://sk-config-secret"
	config := openaiadapter.Config{APIKey: secret, BaseURL: secret}
	for _, format := range []string{"%v", "%+v", "%#v"} {
		formatted := fmt.Sprintf(format, config)
		if strings.Contains(formatted, secret) {
			t.Fatalf("Config format %s leaked API key: %s", format, formatted)
		}
	}
	_, err := openaiadapter.New(config)
	if err == nil {
		t.Fatal("New() accepted malformed BaseURL")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(fmt.Sprintf("%#v", err), secret) {
		t.Fatalf("configuration error leaked API key: %#v", err)
	}
}

func TestConfiguredClientIgnoresOpenAIEnvironmentDefaults(t *testing.T) {
	for key, value := range map[string]string{
		"OPENAI_API_KEY":        "environment-key",
		"OPENAI_ADMIN_KEY":      "environment-admin-key",
		"OPENAI_ORG_ID":         "environment-organization",
		"OPENAI_PROJECT_ID":     "environment-project",
		"OPENAI_WEBHOOK_SECRET": "environment-webhook-secret",
		"OPENAI_BASE_URL":       "http://127.0.0.1:1/environment/v1/",
		"OPENAI_CUSTOM_HEADERS": "X-Environment-Leak: present\nX-Second-Environment-Leak: present",
	} {
		t.Setenv(key, value)
	}

	tests := []struct {
		name         string
		protocol     modelinvoker.Protocol
		path         string
		responseBody string
	}{
		{
			name:         "responses",
			protocol:     modelinvoker.ProtocolResponses,
			path:         "/configured/v1/responses",
			responseBody: `{"id":"resp_environment","model":"test-model","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`,
		},
		{
			name:         "chat completions",
			protocol:     modelinvoker.ProtocolChatCompletions,
			path:         "/configured/v1/chat/completions",
			responseBody: `{"id":"chat_environment","model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			captured := make(chan *http.Request, 1)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				captured <- request.Clone(request.Context())
				writer.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(writer, test.responseBody)
			}))
			t.Cleanup(server.Close)

			adapter, err := openaiadapter.New(openaiadapter.Config{
				APIKey:     "configured-key",
				BaseURL:    server.URL + "/configured/v1",
				HTTPClient: server.Client(),
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			request := modelinvoker.Request{
				Provider: openaiadapter.ProviderID,
				Protocol: test.protocol,
				Model:    "test-model",
				Input:    []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
			}
			if _, err := adapter.Invoke(context.Background(), request); err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}

			got := <-captured
			if got.URL.Path != test.path {
				t.Fatalf("request path = %q, want %q", got.URL.Path, test.path)
			}
			if authorization := got.Header.Get("Authorization"); authorization != "Bearer configured-key" {
				t.Fatalf("Authorization = %q, want configured key", authorization)
			}
			for _, header := range []string{
				"OpenAI-Organization",
				"OpenAI-Project",
				"X-Environment-Leak",
				"X-Second-Environment-Leak",
			} {
				if values := got.Header.Values(header); len(values) != 0 {
					t.Fatalf("environment header %s leaked into request: %q", header, values)
				}
			}
		})
	}
}
