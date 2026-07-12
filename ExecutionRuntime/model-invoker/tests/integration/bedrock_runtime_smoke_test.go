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
	if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv("PRAXIS_BEDROCK_RUNTIME_SMOKE") != "confirmed" {
		t.Skip("Bedrock Runtime live smoke requires global and provider confirmations")
	}
	region, model := os.Getenv("AWS_REGION"), os.Getenv("BEDROCK_RUNTIME_SMOKE_MODEL")
	if region == "" || model == "" {
		t.Fatal("AWS_REGION and BEDROCK_RUNTIME_SMOKE_MODEL are required")
	}
	adapter, err := provider.New(provider.Config{Region: region, CredentialMode: provider.CredentialDefaultChain})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Invoke(context.Background(), modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolBedrockConverse, Model: model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: praxis-bedrock-runtime-ok")}, Budget: modelinvoker.Budget{MaxOutputTokens: 16, Timeout: 30 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	if !hasExactProviderSmokeMarker(response.Text(), "praxis-bedrock-runtime-ok") {
		t.Fatal("Bedrock Runtime response did not match the exact marker")
	}
}
