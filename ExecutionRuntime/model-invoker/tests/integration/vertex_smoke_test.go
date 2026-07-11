//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/vertex"
)

func TestVertexGenerateContentSmoke(t *testing.T) {
	if os.Getenv("PRAXIS_VERTEX_SMOKE") != "confirmed" {
		t.Fatal("set PRAXIS_VERTEX_SMOKE=confirmed only after approving one real cloud request")
	}
	project, location, model, deployment := os.Getenv("GOOGLE_CLOUD_PROJECT"), os.Getenv("GOOGLE_CLOUD_LOCATION"), os.Getenv("VERTEX_SMOKE_MODEL"), os.Getenv("VERTEX_SMOKE_DEPLOYMENT_REF")
	if project == "" || location == "" || model == "" || deployment == "" {
		t.Fatal("GOOGLE_CLOUD_PROJECT, GOOGLE_CLOUD_LOCATION, VERTEX_SMOKE_MODEL, and VERTEX_SMOKE_DEPLOYMENT_REF are required")
	}
	adapter, err := provider.New(provider.Config{Project: project, Location: location, DeploymentMode: provider.DeploymentServerless, DeploymentRef: deployment, CredentialMode: provider.CredentialADC})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Invoke(context.Background(), modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolGenerateContent, Model: model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly OK.")}, Budget: modelinvoker.Budget{MaxOutputTokens: 16, Timeout: 30 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Text() == "" {
		t.Fatal("empty Vertex response")
	}
}
