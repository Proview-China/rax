//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/azureopenai"
)

func TestAzureOpenAIResponsesSmoke(t *testing.T) {
	if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv("PRAXIS_AZURE_OPENAI_SMOKE") != "confirmed" {
		t.Skip("Azure OpenAI live smoke requires global and provider confirmations")
	}
	endpoint, region, deployment, key := os.Getenv("AZURE_OPENAI_ENDPOINT"), os.Getenv("AZURE_OPENAI_REGION"), os.Getenv("AZURE_OPENAI_DEPLOYMENT"), os.Getenv("AZURE_OPENAI_API_KEY")
	if endpoint == "" || region == "" || deployment == "" || key == "" {
		t.Fatal("AZURE_OPENAI_ENDPOINT, AZURE_OPENAI_REGION, AZURE_OPENAI_DEPLOYMENT, and AZURE_OPENAI_API_KEY are required")
	}
	adapter, err := provider.New(provider.Config{ResourceEndpoint: endpoint, Region: region, DeploymentName: deployment, CredentialMode: provider.CredentialAPIKey, APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Invoke(context.Background(), modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolResponses, Model: deployment, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: praxis-azure-openai-ok")}, Budget: modelinvoker.Budget{MaxOutputTokens: 16, Timeout: 30 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	if !hasExactProviderSmokeMarker(response.Text(), "praxis-azure-openai-ok") {
		t.Fatal("Azure OpenAI response did not match the exact marker")
	}
}
