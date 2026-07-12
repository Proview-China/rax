//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/zai"
)

func TestZAILiveSmoke(t *testing.T) {
	if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv("PRAXIS_ZAI_LIVE_TESTS") != "1" {
		t.Skip("Z.AI live smoke requires two explicit confirmations")
	}
	key, model := os.Getenv("ZAI_API_KEY"), os.Getenv("ZAI_SMOKE_MODEL")
	if key == "" || model == "" {
		t.Fatal("enabled Z.AI live smoke requires ZAI_API_KEY and ZAI_SMOKE_MODEL")
	}
	adapter, err := zai.New(zai.Config{APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	r := modelinvoker.Request{Provider: zai.ProviderID, Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: "https://api.z.ai/api/paas/v4", Model: model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: praxis-zai-ok")}, Budget: modelinvoker.Budget{MaxOutputTokens: 32}}
	response, err := adapter.Invoke(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !hasExactProviderSmokeMarker(response.Text(), "praxis-zai-ok") {
		t.Fatal("Z.AI response did not match the exact marker")
	}
}
