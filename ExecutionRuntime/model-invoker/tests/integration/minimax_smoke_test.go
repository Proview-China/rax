//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/minimax"
)

func TestMiniMaxLiveSmoke(t *testing.T) {
	if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv("PRAXIS_MINIMAX_LIVE_TESTS") != "1" {
		t.Skip("MiniMax live smoke requires two explicit confirmations")
	}
	key, model := os.Getenv("MINIMAX_API_KEY"), os.Getenv("MINIMAX_SMOKE_MODEL")
	if key == "" || model == "" {
		t.Skip("MINIMAX_API_KEY and MINIMAX_SMOKE_MODEL are required")
	}
	adapter, err := minimax.New(minimax.Config{APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	request := modelinvoker.Request{
		Provider: minimax.ProviderID, Protocol: modelinvoker.ProtocolMessages,
		Endpoint: "https://api.minimax.io/anthropic", Model: model,
		Input:  []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: praxis-minimax-ok")},
		Budget: modelinvoker.Budget{MaxOutputTokens: 32},
	}
	response, err := adapter.Invoke(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Text() == "" {
		t.Fatal("MiniMax returned empty text")
	}
}
