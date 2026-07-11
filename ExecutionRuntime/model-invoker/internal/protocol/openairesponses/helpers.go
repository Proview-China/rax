package openairesponses

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func driverError(kind modelinvoker.ErrorKind, operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Operation: operation, Message: message}
}

func mappingError(operation, message string) *modelinvoker.Error {
	return driverError(modelinvoker.ErrorMapping, operation, message)
}

func mappingErrorWithRequestID(operation, message, requestID string) *modelinvoker.Error {
	err := mappingError(operation, message)
	err.RequestID = requestID
	return err
}

func degradation(capability modelinvoker.Capability, detail string) modelinvoker.MappingDecision {
	return modelinvoker.MappingDecision{Capability: capability, Action: modelinvoker.MappingDegraded, Detail: detail}
}

func schemaObject(raw json.RawMessage) (map[string]any, error) {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil || schema == nil {
		return nil, fmt.Errorf("schema must be a JSON object")
	}
	return schema, nil
}

func validateStrictSchema(schema map[string]any, path string) error {
	if schemaType, _ := schema["type"].(string); schemaType == "object" {
		additional, ok := schema["additionalProperties"].(bool)
		if !ok || additional {
			return fmt.Errorf("%s strict object must set additionalProperties to false", path)
		}
		properties, _ := schema["properties"].(map[string]any)
		requiredValues, _ := schema["required"].([]any)
		required := make(map[string]struct{}, len(requiredValues))
		for _, value := range requiredValues {
			if name, ok := value.(string); ok {
				required[name] = struct{}{}
			}
		}
		for name, property := range properties {
			if _, ok := required[name]; !ok {
				return fmt.Errorf("%s strict object must require property %q", path, name)
			}
			if child, ok := property.(map[string]any); ok {
				if err := validateStrictSchema(child, path+"."+name); err != nil {
					return err
				}
			}
		}
	}
	if items, ok := schema["items"].(map[string]any); ok {
		if err := validateStrictSchema(items, path+"[]"); err != nil {
			return err
		}
	}
	for _, keyword := range []string{"anyOf", "oneOf", "allOf"} {
		if variants, ok := schema[keyword].([]any); ok {
			for index, variant := range variants {
				if child, ok := variant.(map[string]any); ok {
					if err := validateStrictSchema(child, fmt.Sprintf("%s.%s[%d]", path, keyword, index)); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func validFunctionCall(call modelinvoker.FunctionCall) bool {
	if strings.TrimSpace(call.ID) == "" || !toolNamePattern.MatchString(call.Name) {
		return false
	}
	_, err := schemaObject(call.Arguments)
	return err == nil
}

func cloneCall(call modelinvoker.FunctionCall) *modelinvoker.FunctionCall {
	copy := call
	copy.Arguments = append(json.RawMessage(nil), call.Arguments...)
	return &copy
}
