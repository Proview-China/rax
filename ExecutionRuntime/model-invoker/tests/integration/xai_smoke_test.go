//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/xai"
)

func TestXAILiveSmoke(t *testing.T) {
	if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv("PRAXIS_XAI_LIVE_TESTS") != "1" {
		t.Skip("xAI live smoke requires two explicit confirmations")
	}
	key, model := os.Getenv("XAI_API_KEY"), os.Getenv("XAI_SMOKE_MODEL")
	if key == "" || model == "" {
		t.Fatal("enabled xAI live smoke requires XAI_API_KEY and XAI_SMOKE_MODEL")
	}
	adapter, err := xai.New(xai.Config{APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	request := modelinvoker.Request{
		Provider: xai.ProviderID, Protocol: modelinvoker.ProtocolResponses, Endpoint: "https://api.x.ai/v1", Model: model,
		Input:  []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: praxis-xai-ok")},
		Budget: modelinvoker.Budget{MaxOutputTokens: 32},
	}
	response, err := adapter.Invoke(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !hasExactProviderSmokeMarker(response.Text(), "praxis-xai-ok") {
		t.Fatal("xAI response did not match the exact marker")
	}
}
