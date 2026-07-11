//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/bedrockruntime"
)

func TestBedrockRuntimeConverseSmoke(t *testing.T) {
	if os.Getenv("PRAXIS_BEDROCK_RUNTIME_SMOKE") != "confirmed" {
		t.Fatal("set PRAXIS_BEDROCK_RUNTIME_SMOKE=confirmed only after approving one real cloud request")
	}
	region, model := os.Getenv("AWS_REGION"), os.Getenv("BEDROCK_RUNTIME_SMOKE_MODEL")
	if region == "" || model == "" {
		t.Fatal("AWS_REGION and BEDROCK_RUNTIME_SMOKE_MODEL are required")
	}
	adapter, err := provider.New(provider.Config{Region: region, CredentialMode: provider.CredentialDefaultChain})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Invoke(context.Background(), modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolBedrockConverse, Model: model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly OK.")}, Budget: modelinvoker.Budget{MaxOutputTokens: 16, Timeout: 30 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Text() == "" {
		t.Fatal("empty Bedrock Runtime response")
	}
}
