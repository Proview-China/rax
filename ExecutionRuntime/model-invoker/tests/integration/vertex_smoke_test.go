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
	if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv("PRAXIS_VERTEX_SMOKE") != "confirmed" {
		t.Skip("Vertex live smoke requires global and provider confirmations")
	}
	project, location, model, deployment := os.Getenv("GOOGLE_CLOUD_PROJECT"), os.Getenv("GOOGLE_CLOUD_LOCATION"), os.Getenv("VERTEX_SMOKE_MODEL"), os.Getenv("VERTEX_SMOKE_DEPLOYMENT_REF")
	if project == "" || location == "" || model == "" || deployment == "" {
		t.Fatal("GOOGLE_CLOUD_PROJECT, GOOGLE_CLOUD_LOCATION, VERTEX_SMOKE_MODEL, and VERTEX_SMOKE_DEPLOYMENT_REF are required")
	}
	adapter, err := provider.New(provider.Config{Project: project, Location: location, DeploymentMode: provider.DeploymentServerless, DeploymentRef: deployment, CredentialMode: provider.CredentialADC})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Invoke(context.Background(), modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolGenerateContent, Model: model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: praxis-vertex-ok")}, Budget: modelinvoker.Budget{MaxOutputTokens: 16, Timeout: 30 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	if !hasExactProviderSmokeMarker(response.Text(), "praxis-vertex-ok") {
		t.Fatal("Vertex response did not match the exact marker")
	}
}
