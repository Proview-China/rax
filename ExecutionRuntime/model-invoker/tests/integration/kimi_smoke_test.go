//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/kimi"
)

func TestKimiLiveSmoke(t *testing.T) {
	if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv("PRAXIS_KIMI_LIVE_TESTS") != "1" {
		t.Skip("Kimi live smoke requires two explicit confirmations")
	}
	key, model := os.Getenv("MOONSHOT_API_KEY"), os.Getenv("KIMI_SMOKE_MODEL")
	if key == "" || model == "" {
		t.Fatal("enabled Kimi live smoke requires MOONSHOT_API_KEY and KIMI_SMOKE_MODEL")
	}
	adapter, err := kimi.New(kimi.Config{APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	r := modelinvoker.Request{Provider: kimi.ProviderID, Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: "https://api.moonshot.cn/v1", Model: model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: praxis-kimi-ok")}, Budget: modelinvoker.Budget{MaxOutputTokens: 32}}
	response, err := adapter.Invoke(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !hasExactProviderSmokeMarker(response.Text(), "praxis-kimi-ok") {
		t.Fatal("Kimi response did not match the exact marker")
	}
}
