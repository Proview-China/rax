//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	anthropicadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/anthropic"
)

// TestAnthropicMessagesSmoke is intentionally excluded from ordinary test
// runs. It performs one real request only when the caller explicitly supplies
// the confirmation, API key, and model gates below.
func TestAnthropicMessagesSmoke(t *testing.T) {
	if os.Getenv("PRAXIS_ANTHROPIC_SMOKE") != "confirmed" {
		t.Fatal("set PRAXIS_ANTHROPIC_SMOKE=confirmed only after approving a real API request")
	}
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	model := os.Getenv("ANTHROPIC_SMOKE_MODEL")
	if apiKey == "" || model == "" {
		t.Fatal("ANTHROPIC_API_KEY and ANTHROPIC_SMOKE_MODEL are required")
	}

	adapter, err := anthropicadapter.New(anthropicadapter.Config{APIKey: apiKey})
	if err != nil {
		t.Fatalf("configure Anthropic adapter: %v", err)
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
		Provider: anthropicadapter.ProviderID,
		Protocol: modelinvoker.ProtocolMessages,
		Model:    model,
		Input:    []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly OK.")},
		Budget:   modelinvoker.Budget{MaxOutputTokens: 16, Timeout: 30 * time.Second},
	})
	if err != nil {
		t.Fatalf("real Anthropic smoke request: %v", err)
	}
	if response.ID == "" || response.RequestID == "" || response.Text() == "" {
		t.Fatalf("incomplete smoke response: id=%q request_id=%q text=%q", response.ID, response.RequestID, response.Text())
	}
}
