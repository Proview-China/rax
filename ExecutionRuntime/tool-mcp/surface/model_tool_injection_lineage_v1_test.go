package surface

import (
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func TestFreezeRouteCallV1AllMutableBranchesAreIndependent(t *testing.T) {
	strict := true
	parallel := false
	outputStrict := true
	budgetTokens := int64(32)
	remainingQuota := int64(7)
	original := modelinvoker.RouteCall{
		RouteID: "openai.direct.payg.responses",
		Invocation: upstream.InvocationContext{
			Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService,
			Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground,
		},
		EntitlementState: &upstream.EntitlementState{RemainingQuota: &remainingQuota},
		Request: modelinvoker.Request{
			Input: []modelinvoker.InputItem{
				modelinvoker.MessageInput(modelinvoker.RoleUser, "stable"),
				modelinvoker.FunctionCallInput("call", "tool.example", json.RawMessage(`{"value":"stable"}`)),
				modelinvoker.NamedFunctionResultInput("call", "tool.example", "stable", false),
			},
			Instructions: []modelinvoker.Instruction{{Role: modelinvoker.RoleSystem, Text: "stable"}},
			Tools: []modelinvoker.Tool{{
				Name: "tool.example", Description: "stable",
				Parameters: json.RawMessage(`{"type":"object"}`), Strict: &strict,
			}},
			ParallelToolCalls: &parallel,
			Output: modelinvoker.OutputConstraint{
				Type: modelinvoker.OutputJSONSchema, Name: "stable", Description: "stable",
				Schema: json.RawMessage(`{"type":"object"}`), Strict: &outputStrict,
			},
			Reasoning: &modelinvoker.Reasoning{
				Effort: modelinvoker.ReasoningEffortLow, BudgetTokens: &budgetTokens,
			},
			State: &modelinvoker.State{
				Kind: modelinvoker.StateProviderContinuation, Provider: "provider",
				Protocol: modelinvoker.ProtocolResponses, ID: "state",
				Payload: modelinvoker.NewRawPayload([]byte("stable")),
			},
			Metadata: modelinvoker.Metadata{"trace": "stable"},
			ProviderOptions: modelinvoker.ProviderOptions{
				"provider": json.RawMessage(`{"value":"stable"}`),
			},
		},
	}
	frozen := freezeRouteCallV1(original)
	baseline := freezeRouteCallV1(frozen)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for index := 0; index < 1_000; index++ {
			*original.EntitlementState.RemainingQuota = int64(index)
			original.Request.Input[0].Message.Text = "mutated"
			original.Request.Input[1].FunctionCall.Arguments[0] = byte('a' + index%26)
			original.Request.Input[2].FunctionResult.Output = "mutated"
			original.Request.Instructions[0].Text = "mutated"
			original.Request.Tools[0].Description = "mutated"
			original.Request.Tools[0].Parameters[0] = byte('a' + index%26)
			*original.Request.Tools[0].Strict = index%2 == 0
			*original.Request.ParallelToolCalls = index%2 == 0
			original.Request.Output.Schema[0] = byte('a' + index%26)
			*original.Request.Output.Strict = index%2 == 0
			original.Request.Reasoning.Effort = modelinvoker.ReasoningEffortHigh
			*original.Request.Reasoning.BudgetTokens = int64(index)
			original.Request.State.Payload = modelinvoker.NewRawPayload([]byte{byte(index)})
			original.Request.Metadata["trace"] = "mutated"
			original.Request.ProviderOptions["provider"][0] = byte('a' + index%26)
		}
	}()
	for index := 0; index < 1_000; index++ {
		if !reflect.DeepEqual(frozen, baseline) ||
			!reflect.DeepEqual(frozen.Request.State.Payload.Bytes(), baseline.Request.State.Payload.Bytes()) {
			t.Fatal("frozen RouteCall changed while the caller mutated an input branch")
		}
	}
	wg.Wait()
	if !reflect.DeepEqual(frozen, baseline) ||
		!reflect.DeepEqual(frozen.Request.State.Payload.Bytes(), baseline.Request.State.Payload.Bytes()) {
		t.Fatal("frozen RouteCall retained a caller-owned mutable branch")
	}
}
