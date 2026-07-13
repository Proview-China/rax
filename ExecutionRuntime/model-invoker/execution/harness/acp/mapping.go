package acp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func mapACPInvocation(invocation execution.Invocation, config AdapterConfig) (json.RawMessage, json.RawMessage, error) {
	request := invocation.Request
	mode := request.SessionIntent.Mode
	if mode != "" && mode != "new" && mode != "stateless" {
		return nil, nil, fmt.Errorf("%w: session mode %q needs an explicit load-session bridge", ErrMapping, mode)
	}
	if request.SessionIntent.SessionID != "" || request.SessionIntent.TurnID != "" {
		return nil, nil, fmt.Errorf("%w: existing session identities need an explicit load-session bridge", ErrMapping)
	}
	if len(request.Context) != 0 {
		return nil, nil, fmt.Errorf("%w: explicit context references need a materialization bridge", ErrMapping)
	}
	if len(request.Tools) != 0 {
		return nil, nil, fmt.Errorf("%w: caller-hosted tools require an explicit MCP bridge", ErrMapping)
	}
	if cwd := strings.TrimSpace(request.ExecutionPolicy.CWDReference); cwd != "" && filepath.IsAbs(cwd) && filepath.Clean(cwd) != filepath.Clean(config.Client.Process.WorkingDirectory) {
		return nil, nil, fmt.Errorf("%w: execution cwd differs from the pinned Harness cwd", ErrMapping)
	}

	blocks := make([]any, 0, len(request.Input)+1)
	if len(request.Instructions) != 0 {
		var instructions strings.Builder
		for index, instruction := range request.Instructions {
			text, err := acpTextParts(instruction.Content)
			if err != nil {
				return nil, nil, fmt.Errorf("%w: instruction %d: %v", ErrMapping, index, err)
			}
			if instructions.Len() != 0 {
				instructions.WriteString("\n\n")
			}
			fmt.Fprintf(&instructions, "[%s:%s]\n%s", instruction.Authority, instruction.Scope, text)
		}
		blocks = append(blocks, map[string]any{"type": "text", "text": instructions.String()})
	}
	for index, input := range request.Input {
		if input.Kind != "message" || input.Role != "user" {
			return nil, nil, fmt.Errorf("%w: input %d must be a new user message", ErrMapping, index)
		}
		for _, part := range input.Content {
			switch part.Kind {
			case "text":
				if part.Text == "" {
					return nil, nil, fmt.Errorf("%w: input %d contains empty text", ErrMapping, index)
				}
				blocks = append(blocks, map[string]any{"type": "text", "text": part.Text})
			case "json":
				if len(part.JSON) == 0 || !json.Valid(part.JSON) {
					return nil, nil, fmt.Errorf("%w: input %d contains invalid JSON", ErrMapping, index)
				}
				blocks = append(blocks, map[string]any{"type": "text", "text": string(part.JSON)})
			default:
				return nil, nil, fmt.Errorf("%w: ACP content kind %q needs an explicit content bridge", ErrMapping, part.Kind)
			}
		}
	}
	if len(request.OutputContract.JSONSchema) != 0 {
		if !validJSONObject(request.OutputContract.JSONSchema) {
			return nil, nil, fmt.Errorf("%w: output schema must be an object", ErrMapping)
		}
		blocks = append(blocks, map[string]any{
			"type": "text", "text": "Return only JSON matching this Praxis output schema:\n" + string(request.OutputContract.JSONSchema),
		})
	}
	if len(blocks) == 0 {
		return nil, nil, fmt.Errorf("%w: at least one prompt content block is required", ErrMapping)
	}

	var session map[string]any
	if json.Unmarshal(config.SessionOptions, &session) != nil || session == nil {
		return nil, nil, fmt.Errorf("%w: session options are invalid", ErrMapping)
	}
	session["cwd"] = config.Client.Process.WorkingDirectory
	if _, exists := session["mcpServers"]; !exists {
		session["mcpServers"] = []any{}
	}
	sessionRaw, err := json.Marshal(session)
	if err != nil {
		return nil, nil, err
	}
	promptRaw, err := json.Marshal(blocks)
	if err != nil {
		return nil, nil, err
	}
	return sessionRaw, promptRaw, nil
}

func acpTextParts(parts []union.ContentPart) (string, error) {
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
