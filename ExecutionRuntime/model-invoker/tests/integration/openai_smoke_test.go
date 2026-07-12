//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	openaiadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/openai"
)

// TestOpenAIResponsesSmoke is intentionally excluded from ordinary test runs.
// It performs one real request only when the caller explicitly supplies all
// three environment variables documented in the module README.
func TestOpenAIResponsesSmoke(t *testing.T) {
	if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv("PRAXIS_OPENAI_SMOKE") != "confirmed" {
		t.Skip("OpenAI live smoke requires global and provider confirmations")
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	model := os.Getenv("OPENAI_SMOKE_MODEL")
	if apiKey == "" || model == "" {
		t.Fatal("OPENAI_API_KEY and OPENAI_SMOKE_MODEL are required")
	}

	adapter, err := openaiadapter.New(openaiadapter.Config{APIKey: apiKey})
	if err != nil {
		t.Fatalf("configure OpenAI adapter: %v", err)
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
		Provider: openaiadapter.ProviderID,
		Protocol: modelinvoker.ProtocolResponses,
		Model:    model,
		Input:    []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: praxis-openai-ok")},
		Budget:   modelinvoker.Budget{MaxOutputTokens: 16, Timeout: 30 * time.Second},
	})
	if err != nil {
		t.Fatalf("real OpenAI smoke request: %v", err)
	}
	if response.ID == "" || response.RequestID == "" || !hasExactProviderSmokeMarker(response.Text(), "praxis-openai-ok") {
		t.Fatalf("incomplete smoke response: id=%q request_id=%q text=%q", response.ID, response.RequestID, response.Text())
	}
}
