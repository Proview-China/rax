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
	if os.Getenv("PRAXIS_AZURE_OPENAI_SMOKE") != "confirmed" {
		t.Fatal("set PRAXIS_AZURE_OPENAI_SMOKE=confirmed only after approving one real cloud request")
	}
	endpoint, region, deployment, key := os.Getenv("AZURE_OPENAI_ENDPOINT"), os.Getenv("AZURE_OPENAI_REGION"), os.Getenv("AZURE_OPENAI_DEPLOYMENT"), os.Getenv("AZURE_OPENAI_API_KEY")
	if endpoint == "" || region == "" || deployment == "" || key == "" {
		t.Fatal("AZURE_OPENAI_ENDPOINT, AZURE_OPENAI_REGION, AZURE_OPENAI_DEPLOYMENT, and AZURE_OPENAI_API_KEY are required")
	}
	adapter, err := provider.New(provider.Config{ResourceEndpoint: endpoint, Region: region, DeploymentName: deployment, CredentialMode: provider.CredentialAPIKey, APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Invoke(context.Background(), modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolResponses, Model: deployment, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly OK.")}, Budget: modelinvoker.Budget{MaxOutputTokens: 16, Timeout: 30 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Text() == "" {
		t.Fatal("empty Azure OpenAI response")
	}
}
