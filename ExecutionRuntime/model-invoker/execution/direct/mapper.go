package direct

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type toolResultPayload struct {
	CallID          string                `json:"call_id"`
	Name            string                `json:"name,omitempty"`
	Output          string                `json:"output"`
	IsError         bool                  `json:"is_error,omitempty"`
	Executed        *bool                 `json:"executed,omitempty"`
	ResultOrigin    union.EventOrigin     `json:"result_origin,omitempty"`
	SyntheticReason string                `json:"synthetic_reason,omitempty"`
	SideEffectState union.SideEffectState `json:"side_effect_state,omitempty"`
}

func mapRequest(invocation execution.Invocation, config Config) (modelinvoker.Request, error) {
	request := modelinvoker.Request{Model: config.Model, Stream: invocation.Request.ExecutionPolicy.Stream, AllowDegradation: false}
	for index, instruction := range invocation.Request.Instructions {
		text, err := textContent(instruction.Content)
		if err != nil {
			return modelinvoker.Request{}, fmt.Errorf("%w: instruction %d: %v", ErrMapping, index, err)
		}
		role := modelinvoker.RoleDeveloper
		if instruction.Authority == "runtime_policy" {
			role = modelinvoker.RoleSystem
		}
		request.Instructions = append(request.Instructions, modelinvoker.Instruction{Role: role, Text: text})
	}
	for index, input := range invocation.Request.Input {
		switch input.Kind {
		case "message":
			role, err := mapRole(input.Role)
			if err != nil {
				return modelinvoker.Request{}, fmt.Errorf("%w: input %d: %v", ErrUnsupportedInput, index, err)
			}
			text, err := textContent(input.Content)
			if err != nil {
				return modelinvoker.Request{}, fmt.Errorf("%w: input %d: %v", ErrUnsupportedInput, index, err)
			}
			request.Input = append(request.Input, modelinvoker.MessageInput(role, text))
		case "tool_result":
			var result toolResultPayload
			if len(input.Payload) == 0 || json.Unmarshal(input.Payload, &result) != nil || strings.TrimSpace(result.CallID) == "" {
				return modelinvoker.Request{}, fmt.Errorf("%w: input %d tool result is invalid", ErrUnsupportedInput, index)
			}
			request.Input = append(request.Input, modelinvoker.NamedFunctionResultInput(result.CallID, result.Name, result.Output, result.IsError))
		default:
			return modelinvoker.Request{}, fmt.Errorf("%w: input %d kind %q", ErrUnsupportedInput, index, input.Kind)
		}
	}
	allowedToolIDs := make(map[string]struct{}, len(invocation.Request.ToolPolicy.AllowedToolIDs))
	for _, toolID := range invocation.Request.ToolPolicy.AllowedToolIDs {
		allowedToolIDs[toolID] = struct{}{}
	}
	resolvedAllowedToolIDs := make(map[string]struct{}, len(allowedToolIDs))
	for index, tool := range invocation.Request.Tools {
		if len(allowedToolIDs) != 0 {
			if _, allowed := allowedToolIDs[tool.ID]; !allowed {
				continue
			}
			resolvedAllowedToolIDs[tool.ID] = struct{}{}
		}
		if tool.Kind != "function" || (tool.ExecutionOwner != union.ExecutionOwnerPraxis && tool.ExecutionOwner != union.ExecutionOwnerExternal) {
			return modelinvoker.Request{}, fmt.Errorf("%w: tool %d must be a caller-hosted function", ErrUnsupportedTool, index)
		}
		if !jsonObject(tool.InputSchema) {
			return modelinvoker.Request{}, fmt.Errorf("%w: tool %d input schema is not an object", ErrUnsupportedTool, index)
		}
		strict := true
		request.Tools = append(request.Tools, modelinvoker.Tool{Name: tool.Name, Description: tool.Description, Parameters: append(json.RawMessage(nil), tool.InputSchema...), Strict: &strict})
	}
	if len(resolvedAllowedToolIDs) != len(allowedToolIDs) {
		return modelinvoker.Request{}, fmt.Errorf("%w: tool policy references an undeclared caller tool", ErrUnsupportedTool)
	}
	if len(request.Tools) == 0 {
		request.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceNone}
	} else {
		request.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceAuto}
	}
	if len(invocation.Request.OutputContract.JSONSchema) > 0 {
		if !jsonObject(invocation.Request.OutputContract.JSONSchema) {
			return modelinvoker.Request{}, fmt.Errorf("%w: output schema is not a JSON object", ErrMapping)
		}
		strict := true
		request.Output = modelinvoker.OutputConstraint{Type: modelinvoker.OutputJSONSchema, Name: "praxis_union_output", Schema: append(json.RawMessage(nil), invocation.Request.OutputContract.JSONSchema...), Strict: &strict}
	}
	if effort := strings.TrimSpace(invocation.Request.ReasoningIntent.Effort); effort != "" {
		reasoning := modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffort(effort)}
		if summary := strings.TrimSpace(invocation.Request.ReasoningIntent.Summary); summary != "" && summary != "none" {
			reasoning.Summary = modelinvoker.ReasoningSummary(summary)
		}
		if invocation.Request.ReasoningIntent.BudgetTokens > 0 {
			budget := invocation.Request.ReasoningIntent.BudgetTokens
			reasoning.BudgetTokens = &budget
		}
		request.Reasoning = &reasoning
	}
	request.Budget = modelinvoker.Budget{MaxOutputTokens: invocation.Request.Budget.MaxOutputTokens, Timeout: invocation.Request.Budget.MaxWallTime}
	request.Metadata = make(modelinvoker.Metadata, len(invocation.Request.Metadata))
	for key, value := range invocation.Request.Metadata {
		request.Metadata[key] = value
	}
	mode := invocation.Request.SessionIntent.Mode
	if mode != "" && mode != "stateless" && mode != "new" {
		return modelinvoker.Request{}, fmt.Errorf("%w: mode %q requires an explicit continuation bridge", ErrUnsupportedSession, mode)
	}
	return request, nil
}

func mapRole(value string) (modelinvoker.Role, error) {
	switch value {
	case "user":
		return modelinvoker.RoleUser, nil
	case "assistant":
		return modelinvoker.RoleAssistant, nil
	default:
		return "", fmt.Errorf("role %q is unsupported", value)
	}
}

func textContent(parts []union.ContentPart) (string, error) {
	var builder strings.Builder
	for _, part := range parts {
		switch part.Kind {
		case "text":
			builder.WriteString(part.Text)
		case "json":
			if len(part.JSON) == 0 || !json.Valid(part.JSON) {
				return "", fmt.Errorf("JSON content is invalid")
			}
			builder.Write(part.JSON)
		default:
			return "", fmt.Errorf("content kind %q requires an asset transport", part.Kind)
		}
	}
	if strings.TrimSpace(builder.String()) == "" {
		return "", fmt.Errorf("text content is empty")
	}
	return builder.String(), nil
}

func jsonObject(payload []byte) bool {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(trimmed, &object) == nil && object != nil
}
