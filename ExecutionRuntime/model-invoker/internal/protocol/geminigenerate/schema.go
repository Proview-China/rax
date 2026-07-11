package geminigenerate

import (
	"encoding/json"
	"fmt"
	"strings"
)

var supportedSchemaKeywords = map[string]struct{}{
	"$id": {}, "$defs": {}, "$ref": {}, "$anchor": {},
	"type": {}, "format": {}, "title": {}, "description": {}, "enum": {},
	"items": {}, "prefixItems": {}, "minItems": {}, "maxItems": {},
	"minimum": {}, "maximum": {}, "anyOf": {}, "oneOf": {},
	"properties": {}, "additionalProperties": {}, "required": {},
	"propertyOrdering": {},
}

func decodeJSONSchema(raw json.RawMessage, requireObject bool) (map[string]any, error) {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil || schema == nil {
		if err == nil {
			err = fmt.Errorf("schema is null")
		}
		return nil, fmt.Errorf("schema must be a JSON object: %w", err)
	}
	if err := validateSchemaNode(schema, "$", requireObject); err != nil {
		return nil, err
	}
	return schema, nil
}

func validateSchemaNode(node map[string]any, path string, requireObject bool) error {
	for key := range node {
		if _, ok := supportedSchemaKeywords[key]; !ok {
			return fmt.Errorf("%s uses unsupported Gemini JSON Schema keyword %q", path, key)
		}
	}
	if requireObject {
		value, ok := node["type"]
		if !ok || !isObjectType(value) {
			return fmt.Errorf("%s must declare type object", path)
		}
	}
	if properties, ok := node["properties"]; ok {
		object, ok := properties.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.properties must be an object", path)
		}
		for name, value := range object {
			child, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.properties.%s must be a schema object", path, name)
			}
			if err := validateSchemaNode(child, path+".properties."+name, false); err != nil {
				return err
			}
		}
	}
	if definitions, ok := node["$defs"]; ok {
		object, ok := definitions.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.$defs must be an object", path)
		}
		for name, value := range object {
			child, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.$defs.%s must be a schema object", path, name)
			}
			if err := validateSchemaNode(child, path+".$defs."+name, false); err != nil {
				return err
			}
		}
	}
	if items, ok := node["items"]; ok {
		child, ok := items.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.items must be a schema object", path)
		}
		if err := validateSchemaNode(child, path+".items", false); err != nil {
			return err
		}
	}
	for _, key := range []string{"prefixItems", "anyOf", "oneOf"} {
		if value, ok := node[key]; ok {
			children, ok := value.([]any)
			if !ok || len(children) == 0 {
				return fmt.Errorf("%s.%s must be a non-empty schema array", path, key)
			}
			for index, value := range children {
				child, ok := value.(map[string]any)
				if !ok {
					return fmt.Errorf("%s.%s[%d] must be a schema object", path, key, index)
				}
				if err := validateSchemaNode(child, fmt.Sprintf("%s.%s[%d]", path, key, index), false); err != nil {
					return err
				}
			}
		}
	}
	if additional, ok := node["additionalProperties"]; ok {
		switch value := additional.(type) {
		case bool:
		case map[string]any:
			if err := validateSchemaNode(value, path+".additionalProperties", false); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s.additionalProperties must be a boolean or schema object", path)
		}
	}
	if ref, ok := node["$ref"]; ok {
		if _, ok := ref.(string); !ok {
			return fmt.Errorf("%s.$ref must be a string", path)
		}
		for key := range node {
			if key != "$ref" && !strings.HasPrefix(key, "$") {
				return fmt.Errorf("%s with $ref must not include sibling keyword %q", path, key)
			}
		}
	}
	return nil
}

func isObjectType(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.EqualFold(typed, "object")
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.EqualFold(text, "object") {
				return true
			}
		}
	}
	return false
}

func cloneSchema(schema map[string]any) (map[string]any, error) {
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var clone map[string]any
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, err
	}
	return clone, nil
}
