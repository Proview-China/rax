//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	geminiadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/gemini"
)

// TestGeminiGenerateContentSmoke is excluded from ordinary test runs and makes
// exactly one real request only after all three explicit approval gates exist.
func TestGeminiGenerateContentSmoke(t *testing.T) {
	if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv("PRAXIS_GEMINI_SMOKE") != "confirmed" {
		t.Skip("Gemini live smoke requires global and provider confirmations")
	}
	apiKey := os.Getenv("GEMINI_API_KEY")
	model := os.Getenv("GEMINI_SMOKE_MODEL")
	if apiKey == "" || model == "" {
		t.Fatal("GEMINI_API_KEY and GEMINI_SMOKE_MODEL are required")
	}

	adapter, err := geminiadapter.New(geminiadapter.Config{APIKey: apiKey})
	if err != nil {
		t.Fatalf("configure Gemini adapter: %v", err)
	}
	registry, err := modelinvoker.NewRegistry(adapter)
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}
	invoker, err := modelinvoker.NewInvoker(registry)
	if err != nil {
		t.Fatalf("create invoker: %v", err)
	}

	response, err := invoker.Invoke(context.Background(), modelinvoker.Request{
		Provider: geminiadapter.ProviderID,
		Protocol: modelinvoker.ProtocolGenerateContent,
		Model:    model,
		Input:    []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: praxis-gemini-ok")},
		Budget:   modelinvoker.Budget{MaxOutputTokens: 16, Timeout: 30 * time.Second},
	})
	if err != nil {
		t.Fatalf("real Gemini smoke request: %v", err)
	}
	if response.ID == "" || !hasExactProviderSmokeMarker(response.Text(), "praxis-gemini-ok") {
		t.Fatalf("incomplete smoke response: id=%q request_id=%q text=%q", response.ID, response.RequestID, response.Text())
	}
}
