//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/qwen"
)

func TestQwenLiveSmoke(t *testing.T) {
	if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv("PRAXIS_QWEN_LIVE_TESTS") != "1" {
		t.Skip("Qwen live smoke requires two explicit confirmations")
	}
	key, workspace, model := os.Getenv("DASHSCOPE_API_KEY"), os.Getenv("QWEN_SMOKE_WORKSPACE_ID"), os.Getenv("QWEN_SMOKE_MODEL")
	region := qwen.Region(os.Getenv("QWEN_SMOKE_REGION"))
	if key == "" || workspace == "" || model == "" || region == "" {
		t.Skip("DASHSCOPE_API_KEY, QWEN_SMOKE_WORKSPACE_ID, QWEN_SMOKE_REGION and QWEN_SMOKE_MODEL are required")
	}
	adapter, err := qwen.New(qwen.Config{APIKey: key, Region: region, WorkspaceID: workspace})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "https://" + workspace + "." + string(region) + ".maas.aliyuncs.com/compatible-mode/v1"
	request := modelinvoker.Request{
		Provider: qwen.ProviderID, Protocol: modelinvoker.ProtocolResponses,
		Endpoint: endpoint, Model: model,
		Input:  []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: praxis-qwen-ok")},
		Budget: modelinvoker.Budget{MaxOutputTokens: 32},
	}
	response, err := adapter.Invoke(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Text() == "" {
		t.Fatal("Qwen returned empty text")
	}
}
