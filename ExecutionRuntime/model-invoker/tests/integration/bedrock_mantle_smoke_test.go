//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/bedrockmantle"
)

func TestBedrockMantleResponsesSmoke(t *testing.T) {
	if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv("PRAXIS_BEDROCK_MANTLE_SMOKE") != "confirmed" {
		t.Skip("Bedrock Mantle live smoke requires global and provider confirmations")
	}
	region, project, model := os.Getenv("AWS_REGION"), os.Getenv("BEDROCK_SMOKE_PROJECT_REF"), os.Getenv("BEDROCK_MANTLE_SMOKE_MODEL")
	if region == "" || project == "" || model == "" {
		t.Fatal("AWS_REGION, BEDROCK_SMOKE_PROJECT_REF, and BEDROCK_MANTLE_SMOKE_MODEL are required")
	}
	adapter, err := provider.New(provider.Config{Region: region, ProjectRef: project, CredentialMode: provider.CredentialDefaultChain})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Invoke(context.Background(), modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolResponses, Model: model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: praxis-bedrock-mantle-ok")}, Budget: modelinvoker.Budget{MaxOutputTokens: 16, Timeout: 30 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	if !hasExactProviderSmokeMarker(response.Text(), "praxis-bedrock-mantle-ok") {
		t.Fatal("Bedrock Mantle response did not match the exact marker")
	}
}
