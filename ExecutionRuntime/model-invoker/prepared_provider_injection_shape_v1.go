package modelinvoker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	PreparedProviderInjectionShapeContractVersionV1 = "praxis.model-invoker.prepared-provider-injection-shape/v1"

	preparedProviderInjectionCanonicalDomainV1 = "praxis.model-invoker.prepared-provider-injection"
)

// PreparedProviderTriStateV1 preserves the distinction between an omitted
// provider control and controls explicitly set to false or true.
type PreparedProviderTriStateV1 string

const (
	PreparedProviderTriStateUnspecifiedV1 PreparedProviderTriStateV1 = "unspecified"
	PreparedProviderTriStateFalseV1       PreparedProviderTriStateV1 = "false"
	PreparedProviderTriStateTrueV1        PreparedProviderTriStateV1 = "true"
)

// PreparedProviderToolV1 is the closed provider-facing tool shape. Parameters
// contains one detached, recursively strict canonical JSON object.
type PreparedProviderToolV1 struct {
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	Parameters  json.RawMessage            `json:"parameters"`
	Strict      PreparedProviderTriStateV1 `json:"strict"`
}

// PreparedProviderToolChoiceV1 makes the default auto mode explicit in the
// canonical document. Name is populated only for function mode.
type PreparedProviderToolChoiceV1 struct {
	Mode string `json:"mode"`
	Name string `json:"name"`
}

// PreparedProviderOptionsV1 binds at most one exact selected-provider
// namespace. Present distinguishes absent options from a present empty object.
type PreparedProviderOptionsV1 struct {
	Provider      ProviderID      `json:"provider"`
	Present       bool            `json:"present"`
	CanonicalJSON json.RawMessage `json:"canonical_json"`
}

// PreparedProviderInjectionShapeV1 is the complete and exclusive canonical
// body for ActualProviderInjectionDigest. It grants no execution authority.
type PreparedProviderInjectionShapeV1 struct {
	ContractVersion   string                       `json:"contract_version"`
	Provider          ProviderID                   `json:"provider"`
	Protocol          Protocol                     `json:"protocol"`
	OrderedTools      []PreparedProviderToolV1     `json:"ordered_tools"`
	ToolChoice        PreparedProviderToolChoiceV1 `json:"tool_choice"`
	ParallelToolCalls PreparedProviderTriStateV1   `json:"parallel_tool_calls"`
	ProviderOptions   PreparedProviderOptionsV1    `json:"provider_options"`
}

// BuildPreparedProviderInjectionShapeV1 builds a detached canonical snapshot
// of only the provider-injection fields in request. It performs no Provider,
// Harness, Runtime, Context or Tool operation.
func BuildPreparedProviderInjectionShapeV1(request Request) (PreparedProviderInjectionShapeV1, error) {
	var shape PreparedProviderInjectionShapeV1
	if !utf8.ValidString(string(request.Provider)) || strings.TrimSpace(string(request.Provider)) == "" {
		return shape, preparedProviderInjectionInvalidV1("selected provider is required and must be valid UTF-8")
	}
	if !request.Protocol.valid() {
		return shape, preparedProviderInjectionInvalidV1("prepared provider protocol must be concrete and valid")
	}

	tools := make([]PreparedProviderToolV1, 0, len(request.Tools))
	names := make(map[string]struct{}, len(request.Tools))
	for index, tool := range request.Tools {
		if !utf8.ValidString(tool.Name) || strings.TrimSpace(tool.Name) == "" {
			return shape, preparedProviderInjectionInvalidV1(fmt.Sprintf("tool %d name is required and must be valid UTF-8", index))
		}
		if !utf8.ValidString(tool.Description) {
			return shape, preparedProviderInjectionInvalidV1(fmt.Sprintf("tool %d description must be valid UTF-8", index))
		}
		if _, duplicate := names[tool.Name]; duplicate {
			return shape, preparedProviderInjectionInvalidV1(fmt.Sprintf("tool %d duplicates name %q", index, tool.Name))
		}
		names[tool.Name] = struct{}{}
		parameters, err := canonicalPreparedProviderJSONObjectV1(tool.Parameters)
		if err != nil {
			return shape, fmt.Errorf("tool %d parameters: %w", index, err)
		}
		tools = append(tools, PreparedProviderToolV1{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  parameters,
			Strict:      preparedProviderTriStateV1(tool.Strict),
		})
	}

	choice, err := canonicalPreparedProviderToolChoiceV1(request.ToolChoice, names, len(tools))
	if err != nil {
		return shape, err
	}
	options, err := canonicalPreparedProviderOptionsV1(request.Provider, request.ProviderOptions)
	if err != nil {
		return shape, err
	}
	shape = PreparedProviderInjectionShapeV1{
		ContractVersion:   PreparedProviderInjectionShapeContractVersionV1,
		Provider:          request.Provider,
		Protocol:          request.Protocol,
		OrderedTools:      tools,
		ToolChoice:        choice,
		ParallelToolCalls: preparedProviderTriStateV1(request.ParallelToolCalls),
		ProviderOptions:   options,
	}
	return shape, nil
}

// ComputeActualProviderInjectionDigestV1 computes the exact digest that a
// PreparedModelInvocationFactV1 carries as ActualProviderInjectionDigest.
func ComputeActualProviderInjectionDigestV1(request Request) (core.Digest, error) {
	shape, err := BuildPreparedProviderInjectionShapeV1(request)
	if err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest(
		preparedProviderInjectionCanonicalDomainV1,
		"v1",
		"PreparedProviderInjectionShapeV1",
		shape,
	)
}

func preparedProviderTriStateV1(value *bool) PreparedProviderTriStateV1 {
	if value == nil {
		return PreparedProviderTriStateUnspecifiedV1
	}
	if *value {
		return PreparedProviderTriStateTrueV1
	}
	return PreparedProviderTriStateFalseV1
}

func canonicalPreparedProviderToolChoiceV1(choice ToolChoice, names map[string]struct{}, toolCount int) (PreparedProviderToolChoiceV1, error) {
	result := PreparedProviderToolChoiceV1{Name: ""}
	switch choice.Mode {
	case ToolChoiceAuto:
		result.Mode = "auto"
		if choice.Name != "" {
			return PreparedProviderToolChoiceV1{}, preparedProviderInjectionInvalidV1("auto tool choice must not include a function name")
		}
	case ToolChoiceNone:
		result.Mode = "none"
		if choice.Name != "" {
			return PreparedProviderToolChoiceV1{}, preparedProviderInjectionInvalidV1("none tool choice must not include a function name")
		}
	case ToolChoiceRequired:
		result.Mode = "required"
		if toolCount == 0 || choice.Name != "" {
			return PreparedProviderToolChoiceV1{}, preparedProviderInjectionInvalidV1("required tool choice needs tools and no function name")
		}
	case ToolChoiceFunction:
		result.Mode = "function"
		if !utf8.ValidString(choice.Name) || strings.TrimSpace(choice.Name) == "" {
			return PreparedProviderToolChoiceV1{}, preparedProviderInjectionInvalidV1("function tool choice requires a valid UTF-8 non-empty name")
		}
		if _, declared := names[choice.Name]; !declared {
			return PreparedProviderToolChoiceV1{}, preparedProviderInjectionInvalidV1("function tool choice references an undeclared tool")
		}
		result.Name = choice.Name
	default:
		return PreparedProviderToolChoiceV1{}, preparedProviderInjectionInvalidV1(fmt.Sprintf("unknown tool choice mode %q", choice.Mode))
	}
	return result, nil
}

func canonicalPreparedProviderOptionsV1(provider ProviderID, options ProviderOptions) (PreparedProviderOptionsV1, error) {
	result := PreparedProviderOptionsV1{Provider: provider, Present: false, CanonicalJSON: nil}
	if len(options) == 0 {
		return result, nil
	}
	if len(options) != 1 {
		return PreparedProviderOptionsV1{}, preparedProviderInjectionInvalidV1("provider options must contain at most one selected-provider namespace")
	}
	for namespace, raw := range options {
		if !utf8.ValidString(string(namespace)) || namespace != provider {
			return PreparedProviderOptionsV1{}, preparedProviderInjectionInvalidV1("provider options namespace must exactly match the selected provider")
		}
		canonical, err := canonicalPreparedProviderJSONObjectV1(raw)
		if err != nil {
			return PreparedProviderOptionsV1{}, fmt.Errorf("provider options: %w", err)
		}
		result.Present = true
		result.CanonicalJSON = canonical
	}
	return result, nil
}

func canonicalPreparedProviderJSONObjectV1(raw json.RawMessage) (json.RawMessage, error) {
	if !utf8.Valid(raw) {
		return nil, preparedProviderInjectionInvalidV1("JSON object must be valid UTF-8")
	}
	if err := validatePreparedProviderJSONStringScalarsV1(raw); err != nil {
		return nil, err
	}
	var strictObject map[string]json.RawMessage
	if err := core.DecodeStrictJSON(raw, &strictObject); err != nil {
		return nil, err
	}
	if strictObject == nil {
		return nil, preparedProviderInjectionInvalidV1("JSON value must be one object")
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, preparedProviderInjectionInvalidV1("JSON object cannot be decoded canonically")
	}
	if _, object := value.(map[string]any); !object {
		return nil, preparedProviderInjectionInvalidV1("JSON value must be one object")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, preparedProviderInjectionInvalidV1("JSON object contains trailing data")
	}
	canonical, err := json.Marshal(value)
	if err != nil || len(canonical) > core.MaxCanonicalDocumentBytes {
		return nil, preparedProviderInjectionInvalidV1("JSON object cannot be represented within canonical limits")
	}
	return append(json.RawMessage(nil), canonical...), nil
}

func validatePreparedProviderJSONStringScalarsV1(raw []byte) error {
	for index := 0; index < len(raw); {
		if raw[index] != '"' {
			index++
			continue
		}
		index++
		for {
			if index >= len(raw) {
				return preparedProviderInjectionInvalidV1("JSON string is unterminated")
			}
			if raw[index] == '"' {
				index++
				break
			}
			switch raw[index] {
			case '\\':
				if index+1 >= len(raw) {
					return preparedProviderInjectionInvalidV1("JSON string escape is incomplete")
				}
				if raw[index+1] != 'u' {
					index += 2
					continue
				}
				unit, ok := preparedProviderUnicodeEscapeV1(raw, index)
				if !ok {
					return preparedProviderInjectionInvalidV1("JSON string Unicode escape is invalid")
				}
				switch {
				case unit >= 0xd800 && unit <= 0xdbff:
					next := index + 6
					low, paired := preparedProviderUnicodeEscapeV1(raw, next)
					if !paired || low < 0xdc00 || low > 0xdfff {
						return preparedProviderInjectionInvalidV1("JSON string contains an unpaired high surrogate escape")
					}
					index += 12
				case unit >= 0xdc00 && unit <= 0xdfff:
					return preparedProviderInjectionInvalidV1("JSON string contains an unpaired low surrogate escape")
				default:
					index += 6
				}
			default:
				_, size := utf8.DecodeRune(raw[index:])
				index += size
			}
		}
	}
	return nil
}

func preparedProviderUnicodeEscapeV1(raw []byte, start int) (uint16, bool) {
	if start < 0 || start+6 > len(raw) || raw[start] != '\\' || raw[start+1] != 'u' {
		return 0, false
	}
	var value uint16
	for index := start + 2; index < start+6; index++ {
		nibble, ok := preparedProviderHexNibbleV1(raw[index])
		if !ok {
			return 0, false
		}
		value = value<<4 | uint16(nibble)
	}
	return value, true
}

func preparedProviderHexNibbleV1(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}

func preparedProviderInjectionInvalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, message)
}
