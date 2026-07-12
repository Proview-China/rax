//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/mimo"
)

func TestMiMoLiveSmoke(t *testing.T) {
	if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv("PRAXIS_MIMO_LIVE_TESTS") != "1" {
		t.Skip("MiMo live smoke requires two explicit confirmations")
	}
	key, model := os.Getenv("MIMO_API_KEY"), os.Getenv("MIMO_SMOKE_MODEL")
	if key == "" || model == "" {
		t.Fatal("enabled MiMo live smoke requires MIMO_API_KEY and MIMO_SMOKE_MODEL")
	}
	adapter, err := mimo.New(mimo.Config{APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	request := modelinvoker.Request{
		Provider: mimo.ProviderID, Protocol: modelinvoker.ProtocolMessages,
		Endpoint: "https://api.xiaomimimo.com/anthropic", Model: model,
		Input:  []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: praxis-mimo-ok")},
		Budget: modelinvoker.Budget{MaxOutputTokens: 32},
	}
	response, err := adapter.Invoke(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !hasExactProviderSmokeMarker(response.Text(), "praxis-mimo-ok") {
		t.Fatal("MiMo response did not match the exact marker")
	}
}
