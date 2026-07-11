package anthropic_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/anthropic"
)

func TestConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		config  provider.Config
		wantErr string
	}{
		{name: "missing key", config: provider.Config{}, wantErr: "API key is required"},
		{name: "relative URL", config: provider.Config{APIKey: "test", BaseURL: "/v1"}, wantErr: "absolute HTTP"},
		{name: "credentials", config: provider.Config{APIKey: "test", BaseURL: "https://user@example.com"}, wantErr: "credentials"},
		{name: "query", config: provider.Config{APIKey: "test", BaseURL: "https://example.com?q=1"}, wantErr: "query or fragment"},
		{name: "remote HTTP", config: provider.Config{APIKey: "test", BaseURL: "http://example.com"}, wantErr: "loopback"},
		{name: "loopback HTTP", config: provider.Config{APIKey: "test", BaseURL: "http://127.0.0.1:8080", HTTPClient: http.DefaultClient}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter, err := provider.New(test.config)
			if test.wantErr == "" {
				if err != nil || adapter == nil {
					t.Fatalf("New() = (%v, %v), want adapter", adapter, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("New() error = %v, want substring %q", err, test.wantErr)
			}
			if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorInvalidRequest {
				t.Fatalf("kind = %q, want invalid_request", modelinvoker.ErrorKindOf(err))
			}
		})
	}
}

func TestConfigValidationAndFormattingDoNotExposeAPIKey(t *testing.T) {
	secret := "://anthropic-config-secret"
	config := provider.Config{APIKey: secret, BaseURL: secret}
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if formatted := fmt.Sprintf(format, config); strings.Contains(formatted, secret) {
			t.Fatalf("Config format %s leaked API key: %s", format, formatted)
		}
	}
	_, err := provider.New(config)
	if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(fmt.Sprintf("%#v", err), secret) {
		t.Fatalf("configuration error leaked API key: %#v", err)
	}

	formatSecret := "anthropic-format/a b"
	adapter, err := provider.New(provider.Config{
		APIKey: formatSecret, BaseURL: "https://example.test/" + url.PathEscape(formatSecret),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{adapter, *adapter} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			formatted := fmt.Sprintf(format, value)
			if strings.Contains(formatted, formatSecret) || strings.Contains(formatted, url.PathEscape(formatSecret)) || strings.Contains(formatted, url.QueryEscape(formatSecret)) {
				t.Fatalf("Adapter format %s leaked API key: %s", format, formatted)
			}
		}
	}
}
