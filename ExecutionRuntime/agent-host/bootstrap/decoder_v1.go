// Package bootstrap owns the bounded authoring surface for Host bootstrap
// configuration. Decoding is pure: it never opens resources or publishes a
// deployment current fact.
package bootstrap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"gopkg.in/yaml.v3"
)

const (
	MaxBootstrapBytesV1 = 1 << 20
	MaxBootstrapDepthV1 = 64
	MaxBootstrapNodesV1 = 100_000
)

var decimalIntegerV1 = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

func DecodeJSONV1(payload []byte) (contract.HostBootstrapConfigV1, error) {
	if len(payload) == 0 || len(payload) > MaxBootstrapBytesV1 {
		return contract.HostBootstrapConfigV1{}, decodeErrorV1(core.ReasonCanonicalLimitExceeded, "bootstrap JSON is empty or exceeds the V1 byte limit")
	}
	var value contract.HostBootstrapConfigV1
	if err := core.DecodeStrictJSON(payload, &value); err != nil {
		return contract.HostBootstrapConfigV1{}, err
	}
	return contract.SealHostBootstrapConfigV1(value)
}

func DecodeYAMLV1(payload []byte) (contract.HostBootstrapConfigV1, error) {
	if len(payload) == 0 || len(payload) > MaxBootstrapBytesV1 {
		return contract.HostBootstrapConfigV1{}, decodeErrorV1(core.ReasonCanonicalLimitExceeded, "bootstrap YAML is empty or exceeds the V1 byte limit")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(payload))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return contract.HostBootstrapConfigV1{}, decodeErrorV1(core.ReasonInvalidCanonicalForm, "bootstrap YAML cannot be parsed")
	}
	if len(document.Content) != 1 {
		return contract.HostBootstrapConfigV1{}, decodeErrorV1(core.ReasonInvalidCanonicalForm, "bootstrap YAML must contain one document with one root")
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		return contract.HostBootstrapConfigV1{}, decodeErrorV1(core.ReasonInvalidCanonicalForm, "multiple bootstrap YAML documents or trailing content are forbidden")
	}
	count := 0
	value, err := semanticNodeV1(document.Content[0], 0, &count)
	if err != nil {
		return contract.HostBootstrapConfigV1{}, err
	}
	if _, ok := value.(map[string]any); !ok {
		return contract.HostBootstrapConfigV1{}, decodeErrorV1(core.ReasonInvalidCanonicalForm, "bootstrap YAML root must be an object")
	}
	strictJSON, err := json.Marshal(value)
	if err != nil {
		return contract.HostBootstrapConfigV1{}, decodeErrorV1(core.ReasonInvalidCanonicalForm, "bootstrap YAML is not representable as JSON")
	}
	return DecodeJSONV1(strictJSON)
}

func semanticNodeV1(node *yaml.Node, depth int, count *int) (any, error) {
	if node == nil || depth > MaxBootstrapDepthV1 {
		return nil, decodeErrorV1(core.ReasonCanonicalLimitExceeded, "bootstrap YAML nesting exceeds the V1 limit")
	}
	*count++
	if *count > MaxBootstrapNodesV1 {
		return nil, decodeErrorV1(core.ReasonCanonicalLimitExceeded, "bootstrap YAML node count exceeds the V1 limit")
	}
	if node.Anchor != "" || node.Kind == yaml.AliasNode || node.Alias != nil {
		return nil, nodeErrorV1(node, "YAML anchors and aliases are forbidden")
	}
	switch node.Kind {
	case yaml.MappingNode:
		if node.Tag != "!!map" && node.Tag != "tag:yaml.org,2002:map" {
			return nil, nodeErrorV1(node, "custom YAML map tags are forbidden")
		}
		if len(node.Content)%2 != 0 {
			return nil, nodeErrorV1(node, "YAML mapping is malformed")
		}
		result := make(map[string]any, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || (key.Tag != "!!str" && key.Tag != "tag:yaml.org,2002:str") {
				return nil, nodeErrorV1(key, "YAML object keys must be strings")
			}
			if key.Value == "<<" || key.Tag == "!!merge" || key.Tag == "tag:yaml.org,2002:merge" {
				return nil, nodeErrorV1(key, "YAML merge keys are forbidden")
			}
			if _, exists := result[key.Value]; exists {
				return nil, nodeErrorV1(key, "duplicate YAML object key: "+key.Value)
			}
			child, err := semanticNodeV1(node.Content[index+1], depth+1, count)
			if err != nil {
				return nil, err
			}
			result[key.Value] = child
		}
		return result, nil
	case yaml.SequenceNode:
		if node.Tag != "!!seq" && node.Tag != "tag:yaml.org,2002:seq" {
			return nil, nodeErrorV1(node, "custom YAML sequence tags are forbidden")
		}
		result := make([]any, 0, len(node.Content))
		for _, item := range node.Content {
			child, err := semanticNodeV1(item, depth+1, count)
			if err != nil {
				return nil, err
			}
			result = append(result, child)
		}
		return result, nil
	case yaml.ScalarNode:
		return semanticScalarV1(node)
	default:
		return nil, nodeErrorV1(node, "unsupported YAML node kind")
	}
}

func semanticScalarV1(node *yaml.Node) (any, error) {
	tag := strings.TrimPrefix(strings.TrimPrefix(node.Tag, "tag:yaml.org,2002:"), "!!")
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
		return nil, nodeErrorV1(node, "boolean scalars must be lowercase true or false")
	case "int":
		if !decimalIntegerV1.MatchString(node.Value) {
			return nil, nodeErrorV1(node, "integer scalars must use canonical base-10 notation")
		}
		value, err := strconv.ParseInt(node.Value, 10, 64)
		if err != nil {
			return nil, nodeErrorV1(node, "integer scalar exceeds signed 64-bit range")
		}
		return value, nil
	case "null":
		if node.Value != "null" {
			return nil, nodeErrorV1(node, "null scalar must use the canonical null spelling")
		}
		return nil, nil
	case "float", "timestamp":
		return nil, nodeErrorV1(node, tag+" scalars are forbidden")
	default:
		return nil, nodeErrorV1(node, "custom or implicit YAML scalar tag is forbidden: "+node.Tag)
	}
}

func nodeErrorV1(node *yaml.Node, message string) error {
	if node != nil && node.Line > 0 {
		message = fmt.Sprintf("line %d column %d: %s", node.Line, node.Column, message)
	}
	return decodeErrorV1(core.ReasonInvalidCanonicalForm, message)
}

func decodeErrorV1(reason core.ReasonCode, message string) error {
	return core.NewError(core.ErrorInvalidArgument, reason, message)
}
