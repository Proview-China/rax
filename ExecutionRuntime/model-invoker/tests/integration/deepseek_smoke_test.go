//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/deepseek"
)

func TestDeepSeekLiveSmoke(t *testing.T) {
	if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv("PRAXIS_DEEPSEEK_LIVE_TESTS") != "1" {
		t.Skip("DeepSeek live smoke requires two explicit confirmations")
	}
	key := os.Getenv("DEEPSEEK_API_KEY")
	if key == "" {
		t.Fatal("enabled DeepSeek live smoke requires DEEPSEEK_API_KEY")
	}
	model := os.Getenv("DEEPSEEK_SMOKE_MODEL")
	if model != "deepseek-v4-flash" && model != "deepseek-v4-pro" {
		t.Fatal("enabled DeepSeek live smoke requires an explicitly approved current v4 model")
	}
	adapter, err := deepseek.New(deepseek.Config{APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	request := modelinvoker.Request{Provider: deepseek.ProviderID, Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: "https://api.deepseek.com", Model: model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: praxis-deepseek-ok")}, Budget: modelinvoker.Budget{MaxOutputTokens: 32}}
	response, err := adapter.Invoke(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !hasExactProviderSmokeMarker(response.Text(), "praxis-deepseek-ok") {
		t.Fatal("DeepSeek response did not match the exact marker")
	}
}
