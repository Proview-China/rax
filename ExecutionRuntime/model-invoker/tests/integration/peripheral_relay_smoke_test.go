//go:build integration

package integration_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation/nativehttp"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation/specs"
)

// TestPeripheralRelayFileListSmoke is a no-generation, low-cost live probe for
// provider-managed file storage exposed by an explicitly authorized relay.
// Credentials are accepted only through the process environment and are never
// written by the test.
func TestPeripheralRelayFileListSmoke(t *testing.T) {
	if os.Getenv("PRAXIS_LIVE_PERIPHERAL_RELAY") != "1" {
		t.Skip("set PRAXIS_LIVE_PERIPHERAL_RELAY=1 to enable the authorized live probe")
	}
	provider := modelinvoker.ProviderID(os.Getenv("PRAXIS_PERIPHERAL_PROVIDER"))
	baseURL, apiKey := os.Getenv("PRAXIS_PERIPHERAL_BASE_URL"), os.Getenv("PRAXIS_PERIPHERAL_API_KEY")
	if provider == "" || baseURL == "" || apiKey == "" {
		t.Fatal("provider, base URL, and API key are required")
	}

	var catalog []nativehttp.Spec
	query := map[string][]string{"limit": {"1"}}
	switch os.Getenv("PRAXIS_PERIPHERAL_CATALOG") {
	case "openai":
		catalog = specs.OpenAI(nil)
	case "anthropic":
		catalog = specs.Anthropic(nil)
	case "gemini":
		catalog = specs.Gemini(nil)
		query = map[string][]string{"pageSize": {"1"}}
	case "xai":
		catalog = specs.XAI(nil)
	default:
		t.Fatal("unsupported peripheral catalog")
	}
	auth := nativehttp.AuthBearer
	headerName := ""
	if value := os.Getenv("PRAXIS_PERIPHERAL_HEADER"); value != "" {
		auth, headerName = nativehttp.AuthHeader, value
	}
	staticHeaders := http.Header{}
	if name, value := os.Getenv("PRAXIS_PERIPHERAL_STATIC_HEADER"), os.Getenv("PRAXIS_PERIPHERAL_STATIC_VALUE"); name != "" && value != "" {
		staticHeaders.Set(name, value)
	}
	p, err := nativehttp.New(nativehttp.Config{
		Provider: provider, BaseURL: baseURL, Trust: nativehttp.TrustRelay,
		Auth: auth, APIKey: apiKey, HeaderName: headerName, StaticHeaders: staticHeaders, Specs: catalog,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := operation.NewRegistry(p)
	if err != nil {
		t.Fatal(err)
	}
	invoker, err := operation.NewInvoker(registry)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := invoker.Invoke(ctx, operation.Request{
		Provider: provider, Kind: operation.FileList,
		Query: query, Budget: operation.Budget{MaxResponseBytes: 1 << 20},
	})
	if err != nil {
		var typed *modelinvoker.Error
		if errors.As(err, &typed) {
			t.Fatalf("relay file-list capability probe failed: kind=%s status=%d code=%s retryable=%t", typed.Kind, typed.HTTPStatus, typed.Code, typed.Retryable)
		}
		t.Fatalf("relay file-list capability probe failed: %v", err)
	}
	if result.Provider != provider || result.Kind != operation.FileList {
		t.Fatalf("unexpected normalized identity: %+v", result)
	}
}
