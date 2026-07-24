// Package decoder implements the bounded AgentDefinition V1 authoring format.
// YAML is accepted only as a strict, side-effect-free representation of a JSON
// object tree; YAML graph and implicit scalar features are deliberately absent.
package decoder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"gopkg.in/yaml.v3"
)

const (
	MaxYAMLBytesV1 = 1 << 20
	MaxYAMLDepthV1 = 64
	MaxYAMLNodesV1 = 100_000
)

var decimalIntegerV1 = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

func DecodeYAMLV1(payload []byte, catalog contract.ValidationCatalogV1) (contract.AgentDefinitionSourceV1, error) {
	if len(payload) == 0 || len(payload) > MaxYAMLBytesV1 {
		return contract.AgentDefinitionSourceV1{}, decodeError(core.ReasonCanonicalLimitExceeded, "YAML input is empty or exceeds the V1 byte limit")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(payload))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return contract.AgentDefinitionSourceV1{}, decodeError(core.ReasonInvalidCanonicalForm, "YAML cannot be parsed")
	}
	if len(document.Content) != 1 {
		return contract.AgentDefinitionSourceV1{}, decodeError(core.ReasonInvalidCanonicalForm, "YAML must contain one document with one root")
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		return contract.AgentDefinitionSourceV1{}, decodeError(core.ReasonInvalidCanonicalForm, "multiple YAML documents or trailing content are forbidden")
	}
	count := 0
	value, err := semanticNodeV1(document.Content[0], 0, &count)
	if err != nil {
		return contract.AgentDefinitionSourceV1{}, err
	}
	if _, ok := value.(map[string]any); !ok {
		return contract.AgentDefinitionSourceV1{}, decodeError(core.ReasonInvalidCanonicalForm, "AgentDefinition YAML root must be an object")
	}
	strictJSON, err := json.Marshal(value)
	if err != nil {
		return contract.AgentDefinitionSourceV1{}, decodeError(core.ReasonInvalidCanonicalForm, "YAML semantic tree is not representable as JSON")
	}
	return DecodeJSONV1(strictJSON, catalog)
}

func DecodeJSONV1(payload []byte, catalog contract.ValidationCatalogV1) (contract.AgentDefinitionSourceV1, error) {
	if len(payload) == 0 || len(payload) > MaxYAMLBytesV1 {
		return contract.AgentDefinitionSourceV1{}, decodeError(core.ReasonCanonicalLimitExceeded, "JSON input is empty or exceeds the V1 byte limit")
	}
	var source contract.AgentDefinitionSourceV1
	if err := core.DecodeStrictJSON(payload, &source); err != nil {
		return contract.AgentDefinitionSourceV1{}, err
	}
	source = contract.NormalizeSourceV1(source)
	if err := contract.ValidateSourceV1(source, catalog); err != nil {
		return contract.AgentDefinitionSourceV1{}, err
	}
	return source, nil
}

func semanticNodeV1(node *yaml.Node, depth int, count *int) (any, error) {
	if node == nil || depth > MaxYAMLDepthV1 {
		return nil, decodeError(core.ReasonCanonicalLimitExceeded, "YAML nesting exceeds the V1 limit")
	}
	(*count)++
	if *count > MaxYAMLNodesV1 {
		return nil, decodeError(core.ReasonCanonicalLimitExceeded, "YAML node count exceeds the V1 limit")
	}
	if node.Anchor != "" || node.Kind == yaml.AliasNode || node.Alias != nil {
		return nil, nodeError(node, "YAML anchors and aliases are forbidden")
	}
	switch node.Kind {
	case yaml.MappingNode:
		if node.Tag != "!!map" && node.Tag != "tag:yaml.org,2002:map" {
			return nil, nodeError(node, "custom YAML map tags are forbidden")
		}
		if len(node.Content)%2 != 0 {
			return nil, nodeError(node, "YAML mapping is malformed")
		}
		result := make(map[string]any, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || (key.Tag != "!!str" && key.Tag != "tag:yaml.org,2002:str") {
				return nil, nodeError(key, "YAML object keys must be strings")
			}
			if key.Value == "<<" || key.Tag == "!!merge" || key.Tag == "tag:yaml.org,2002:merge" {
				return nil, nodeError(key, "YAML merge keys are forbidden")
			}
			if _, exists := result[key.Value]; exists {
				return nil, nodeError(key, "duplicate YAML object key: "+key.Value)
			}
			value, err := semanticNodeV1(node.Content[index+1], depth+1, count)
			if err != nil {
				return nil, err
			}
			result[key.Value] = value
		}
		return result, nil
	case yaml.SequenceNode:
		if node.Tag != "!!seq" && node.Tag != "tag:yaml.org,2002:seq" {
			return nil, nodeError(node, "custom YAML sequence tags are forbidden")
		}
		result := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			value, err := semanticNodeV1(child, depth+1, count)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		return result, nil
	case yaml.ScalarNode:
		return semanticScalarV1(node)
	default:
		return nil, nodeError(node, "unsupported YAML node kind")
	}
}

func semanticScalarV1(node *yaml.Node) (any, error) {
	tag := strings.TrimPrefix(node.Tag, "tag:yaml.org,2002:")
	tag = strings.TrimPrefix(tag, "!!")
	switch tag {
	case "str":
		return node.Value, nil
	case "bool":
		if node.Value == "true" {
			return true, nil
		}
		if node.Value == "false" {
			return false, nil
		}
		return nil, nodeError(node, "boolean scalars must be lowercase true or false")
	case "int":
		if !decimalIntegerV1.MatchString(node.Value) {
			return nil, nodeError(node, "integer scalars must use canonical base-10 notation")
		}
		value, err := strconv.ParseInt(node.Value, 10, 64)
		if err != nil {
			return nil, nodeError(node, "integer scalar exceeds signed 64-bit range")
		}
		return value, nil
	case "null":
		if node.Value != "null" {
			return nil, nodeError(node, "null scalar must use the canonical null spelling")
		}
		return nil, nil
	case "float", "timestamp":
		return nil, nodeError(node, tag+" scalars are forbidden")
	default:
		return nil, nodeError(node, "custom or implicit YAML scalar tag is forbidden: "+node.Tag)
	}
}

func nodeError(node *yaml.Node, message string) error {
	if node != nil && node.Line > 0 {
		message = fmt.Sprintf("line %d column %d: %s", node.Line, node.Column, message)
	}
	return decodeError(core.ReasonInvalidCanonicalForm, message)
}

func decodeError(reason core.ReasonCode, message string) error {
	return core.NewError(core.ErrorInvalidArgument, reason, message)
}
