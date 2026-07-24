package contract

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// PortableFunctionToolProfileVersionV1 identifies the deliberately small
// function-tool expression shared by the current Model Invoker adapters. It
// is an expression profile only; it grants no execution authority.
const PortableFunctionToolProfileVersionV1 = "praxis.tool-mcp.portable-function-tool/v1"

var portableFunctionToolNameV1 = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)

var portableFunctionSchemaKeywordsV1 = map[string]struct{}{
	"$id": {}, "$defs": {}, "$ref": {}, "$anchor": {},
	"type": {}, "format": {}, "title": {}, "description": {}, "enum": {},
	"items": {}, "prefixItems": {}, "minItems": {}, "maxItems": {},
	"minimum": {}, "maximum": {}, "anyOf": {}, "oneOf": {},
	"properties": {}, "additionalProperties": {}, "required": {},
	"propertyOrdering": {},
}

func ValidatePortableFunctionToolNameV1(name string) error {
	if name != strings.TrimSpace(name) || !portableFunctionToolNameV1.MatchString(name) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "portable function Tool name must match [A-Za-z_][A-Za-z0-9_-]{0,63}")
	}
	return nil
}

// ValidatePortableFunctionToolSchemaV1 validates the strict JSON Schema
// intersection used by the current OpenAI-compatible, Anthropic and Gemini
// Model Invoker mappings. The profile is versioned here so vendor drift cannot
// silently broaden or narrow an already sealed Tool Surface.
func ValidatePortableFunctionToolSchemaV1(raw json.RawMessage) error {
	var value any
	if err := core.DecodeStrictJSON(raw, &value); err != nil {
		return err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "portable function Tool schema must be one JSON object")
	}
	nodes := 0
	return validatePortableFunctionSchemaNodeV1(root, "$", true, 0, &nodes)
}

func validatePortableFunctionSchemaNodeV1(node map[string]any, path string, requireObject bool, depth int, nodes *int) error {
	*nodes++
	if depth > MaxToolDefinitionSchemaDepthV1 || *nodes > MaxToolDefinitionSchemaNodesV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "portable function Tool schema depth or node count exceeds limit")
	}
	for key := range node {
		if _, ok := portableFunctionSchemaKeywordsV1[key]; !ok {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownCapability, fmt.Sprintf("portable function Tool schema %s uses unsupported keyword %q", path, key))
		}
	}
	object, isObject := schemaTypeIncludesObjectV1(node["type"])
	if requireObject && (!isObject || !object) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, fmt.Sprintf("portable function Tool schema %s must declare type object", path))
	}
	if object {
		if additional, ok := node["additionalProperties"].(bool); !ok || additional {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, fmt.Sprintf("portable strict function Tool schema %s must set additionalProperties=false", path))
		}
		properties, ok := node["properties"].(map[string]any)
		if !ok {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, fmt.Sprintf("portable strict function Tool schema %s must define properties", path))
		}
		required, err := requiredPropertySetV1(node["required"])
		if err != nil || len(required) != len(properties) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, fmt.Sprintf("portable strict function Tool schema %s must require every property exactly once", path))
		}
		for name := range properties {
			if _, ok := required[name]; !ok {
				return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, fmt.Sprintf("portable strict function Tool schema %s must require property %q", path, name))
			}
		}
	}
	if properties, ok := node["properties"]; ok {
		values, ok := properties.(map[string]any)
		if !ok {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, fmt.Sprintf("portable function Tool schema %s.properties must be an object", path))
		}
		for name, value := range values {
			child, ok := value.(map[string]any)
			if !ok {
				return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, fmt.Sprintf("portable function Tool schema %s.properties.%s must be a schema object", path, name))
			}
			if err := validatePortableFunctionSchemaNodeV1(child, path+".properties."+name, false, depth+1, nodes); err != nil {
				return err
			}
		}
	}
	if definitions, ok := node["$defs"]; ok {
		values, ok := definitions.(map[string]any)
		if !ok {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, fmt.Sprintf("portable function Tool schema %s.$defs must be an object", path))
		}
		for name, value := range values {
			child, ok := value.(map[string]any)
			if !ok {
				return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, fmt.Sprintf("portable function Tool schema %s.$defs.%s must be a schema object", path, name))
			}
			if err := validatePortableFunctionSchemaNodeV1(child, path+".$defs."+name, false, depth+1, nodes); err != nil {
				return err
			}
		}
	}
	if items, ok := node["items"]; ok {
		child, ok := items.(map[string]any)
		if !ok {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, fmt.Sprintf("portable function Tool schema %s.items must be a schema object", path))
		}
		if err := validatePortableFunctionSchemaNodeV1(child, path+".items", false, depth+1, nodes); err != nil {
			return err
		}
	}
	for _, key := range []string{"prefixItems", "anyOf", "oneOf"} {
		if variants, ok := node[key]; ok {
			values, ok := variants.([]any)
			if !ok || len(values) == 0 {
				return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, fmt.Sprintf("portable function Tool schema %s.%s must be a non-empty schema array", path, key))
			}
			for index, value := range values {
				child, ok := value.(map[string]any)
				if !ok {
					return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, fmt.Sprintf("portable function Tool schema %s.%s[%d] must be a schema object", path, key, index))
				}
				if err := validatePortableFunctionSchemaNodeV1(child, fmt.Sprintf("%s.%s[%d]", path, key, index), false, depth+1, nodes); err != nil {
					return err
				}
			}
		}
	}
	if additional, ok := node["additionalProperties"]; ok && !object {
		if child, ok := additional.(map[string]any); ok {
			if err := validatePortableFunctionSchemaNodeV1(child, path+".additionalProperties", false, depth+1, nodes); err != nil {
				return err
			}
		} else if _, ok := additional.(bool); !ok {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, fmt.Sprintf("portable function Tool schema %s.additionalProperties must be boolean or a schema object", path))
		}
	}
	if ref, ok := node["$ref"]; ok {
		if _, ok := ref.(string); !ok {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, fmt.Sprintf("portable function Tool schema %s.$ref must be a string", path))
		}
		for key := range node {
			if key != "$ref" && !strings.HasPrefix(key, "$") {
				return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, fmt.Sprintf("portable function Tool schema %s has unsupported $ref sibling %q", path, key))
			}
		}
	}
	return nil
}

func schemaTypeIncludesObjectV1(value any) (bool, bool) {
	switch typed := value.(type) {
	case string:
		return typed == "object", true
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok && text == "object" {
				return true, true
			}
		}
		return false, true
	case nil:
		return false, false
	default:
		return false, true
	}
}

func requiredPropertySetV1(value any) (map[string]struct{}, error) {
	values, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("required must be an array")
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		name, ok := value.(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("required contains a non-string or empty name")
		}
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("required contains a duplicate name")
		}
		result[name] = struct{}{}
	}
	return result, nil
}
