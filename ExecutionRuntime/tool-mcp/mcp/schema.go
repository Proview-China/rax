package mcp

import (
	"encoding/json"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	MaxSchemaDepth = 32
	MaxSchemaNodes = 4096
)

func ValidateToolSchema(payload []byte) (core.Digest, error) {
	if len(payload) == 0 || len(payload) > MaxMessageBytes {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "MCP tool schema is empty or exceeds limit")
	}
	var value any
	if err := core.DecodeStrictJSON(payload, &value); err != nil {
		return "", err
	}
	root, ok := value.(map[string]any)
	if !ok || root["type"] != "object" {
		return "", invalid("MCP tool input schema root type must be object")
	}
	nodes := 0
	if err := validateSchemaValue(value, 0, &nodes); err != nil {
		return "", err
	}
	properties, _ := root["properties"].(map[string]any)
	if required, exists := root["required"]; exists {
		items, ok := required.([]any)
		if !ok {
			return "", invalid("MCP schema required must be an array")
		}
		seen := make(map[string]struct{}, len(items))
		for _, item := range items {
			name, ok := item.(string)
			if !ok || name == "" {
				return "", invalid("MCP schema required entries must be names")
			}
			if _, exists := properties[name]; !exists {
				return "", invalid("MCP schema required name is absent from properties")
			}
			if _, duplicate := seen[name]; duplicate {
				return "", invalid("MCP schema required names must be unique")
			}
			seen[name] = struct{}{}
		}
	}
	return core.DigestBytes(payload), nil
}

func validateSchemaValue(value any, depth int, nodes *int) error {
	*nodes++
	if depth > MaxSchemaDepth || *nodes > MaxSchemaNodes {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "MCP schema depth or node count exceeds limit")
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if len(key) > 256 {
				return invalid("MCP schema key exceeds limit")
			}
			if err := validateSchemaValue(child, depth+1, nodes); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := validateSchemaValue(child, depth+1, nodes); err != nil {
				return err
			}
		}
	case string:
		if len(typed) > 64<<10 {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "MCP schema string exceeds limit")
		}
	case json.Number:
		return nil
	}
	return nil
}
