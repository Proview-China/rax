package codexappserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func mapCodexInvocation(invocation execution.Invocation, config AdapterConfig) (json.RawMessage, json.RawMessage, error) {
	request := invocation.Request
	mode := request.SessionIntent.Mode
	if mode != "" && mode != "new" && mode != "stateless" {
		return nil, nil, fmt.Errorf("%w: session mode %q needs an explicit resume bridge", ErrMapping, mode)
	}
	if request.SessionIntent.SessionID != "" || request.SessionIntent.TurnID != "" {
		return nil, nil, fmt.Errorf("%w: existing session identities need an explicit resume bridge", ErrMapping)
	}
	if len(request.Context) != 0 {
		return nil, nil, fmt.Errorf("%w: explicit context references need a materialization bridge", ErrMapping)
	}
	if cwd := strings.TrimSpace(request.ExecutionPolicy.CWDReference); cwd != "" && filepath.IsAbs(cwd) && filepath.Clean(cwd) != filepath.Clean(config.Client.Process.WorkingDirectory) {
		return nil, nil, fmt.Errorf("%w: execution cwd differs from the pinned Harness cwd", ErrMapping)
	}

	inputs := make([]any, 0, len(request.Input))
	for index, input := range request.Input {
		if input.Kind != "message" || input.Role != "user" {
			return nil, nil, fmt.Errorf("%w: input %d must be a new user message", ErrMapping, index)
		}
		mapped, err := mapCodexContent(input.Content)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: input %d: %v", ErrMapping, index, err)
		}
		inputs = append(inputs, mapped...)
	}
	if len(inputs) == 0 {
		return nil, nil, fmt.Errorf("%w: at least one user input is required", ErrMapping)
	}

	var instructions strings.Builder
	for index, instruction := range request.Instructions {
		text, err := codexTextParts(instruction.Content)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: instruction %d: %v", ErrMapping, index, err)
		}
		if instructions.Len() != 0 {
			instructions.WriteString("\n\n")
		}
		fmt.Fprintf(&instructions, "[%s:%s]\n%s", instruction.Authority, instruction.Scope, text)
	}

	dynamicTools := make([]any, 0, len(request.Tools))
	for index, tool := range request.Tools {
		if tool.Kind != "function" || (tool.ExecutionOwner != union.ExecutionOwnerPraxis && tool.ExecutionOwner != union.ExecutionOwnerExternal) {
			return nil, nil, fmt.Errorf("%w: tool %d must be a caller-hosted function", ErrMapping, index)
		}
		if !codexJSONObject(tool.InputSchema) {
			return nil, nil, fmt.Errorf("%w: tool %d input schema must be a JSON object", ErrMapping, index)
		}
		dynamicTools = append(dynamicTools, map[string]any{
			"type": "function", "name": tool.Name, "description": tool.Description, "inputSchema": json.RawMessage(tool.InputSchema),
		})
	}
	if len(dynamicTools) != 0 && !experimentalAPIEnabled(config.Client.Capabilities) {
		return nil, nil, fmt.Errorf("%w: dynamic tools require capabilities.experimentalApi", ErrMapping)
	}

	thread := map[string]any{
		"model": config.Model, "cwd": config.Client.Process.WorkingDirectory, "approvalPolicy": config.ApprovalPolicy,
		"serviceName": config.ServiceName, "ephemeral": config.Ephemeral,
	}
	if config.Sandbox != "" {
		thread["sandbox"] = config.Sandbox
	} else {
		thread["permissions"] = config.Permissions
	}
	if instructions.Len() != 0 {
		thread["developerInstructions"] = instructions.String()
	}
	if len(dynamicTools) != 0 {
		thread["dynamicTools"] = dynamicTools
	}

	turn := map[string]any{"input": inputs, "model": config.Model, "cwd": config.Client.Process.WorkingDirectory}
	if effort := strings.TrimSpace(request.ReasoningIntent.Effort); effort != "" {
		turn["effort"] = effort
	}
	if summary := strings.TrimSpace(request.ReasoningIntent.Summary); summary != "" && summary != "none" {
		turn["summary"] = summary
	}
	if len(request.OutputContract.JSONSchema) != 0 {
		if !codexJSONObject(request.OutputContract.JSONSchema) {
			return nil, nil, fmt.Errorf("%w: output schema must be a JSON object", ErrMapping)
		}
		turn["outputSchema"] = json.RawMessage(request.OutputContract.JSONSchema)
	}
	threadRaw, err := json.Marshal(thread)
	if err != nil {
		return nil, nil, err
	}
	turnRaw, err := json.Marshal(turn)
	if err != nil {
		return nil, nil, err
	}
	return threadRaw, turnRaw, nil
}

func mapCodexContent(parts []union.ContentPart) ([]any, error) {
	result := make([]any, 0, len(parts))
	for _, part := range parts {
		switch part.Kind {
		case "text":
			if part.Text == "" {
				return nil, fmt.Errorf("text content is empty")
			}
			result = append(result, map[string]any{"type": "text", "text": part.Text})
		case "json":
			if len(part.JSON) == 0 || !json.Valid(part.JSON) {
				return nil, fmt.Errorf("JSON content is invalid")
			}
			result = append(result, map[string]any{"type": "text", "text": string(part.JSON)})
		case "image":
			if !strings.HasPrefix(part.Reference, "data:") {
				return nil, fmt.Errorf("image content must be an inline data URL")
			}
			result = append(result, map[string]any{"type": "image", "url": part.Reference})
		case "local_image":
			if !filepath.IsAbs(part.Reference) {
				return nil, fmt.Errorf("local image path must be absolute")
			}
			result = append(result, map[string]any{"type": "localImage", "path": part.Reference})
		default:
			return nil, fmt.Errorf("content kind %q is unsupported", part.Kind)
		}
	}
	return result, nil
}

func codexTextParts(parts []union.ContentPart) (string, error) {
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
			return "", fmt.Errorf("content kind %q is unsupported in instructions", part.Kind)
		}
	}
	if strings.TrimSpace(builder.String()) == "" {
		return "", fmt.Errorf("instruction content is empty")
	}
	return builder.String(), nil
}

func experimentalAPIEnabled(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var capabilities struct {
		ExperimentalAPI bool `json:"experimentalApi"`
	}
	return json.Unmarshal(raw, &capabilities) == nil && capabilities.ExperimentalAPI
}

func codexJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(trimmed, &object) == nil && object != nil
}

func setJSONObjectField(raw json.RawMessage, key string, value any) (json.RawMessage, error) {
	var object map[string]any
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, fmt.Errorf("%w: native params are not an object", ErrMapping)
	}
	object[key] = value
	return json.Marshal(object)
}
