package anthropicmessages

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

const continuationVersion = 1

var nativeToolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type continuationMessage struct {
	Version int               `json:"version"`
	Role    string            `json:"role"`
	Content []json.RawMessage `json:"content"`
}

// MarshalJSON keeps provider continuation state limited to the native blocks
// that Anthropic requires callers to return. Ordinary assistant text remains in
// the normalized response and must not become an unrestricted native-input
// channel on the next request.
func (m continuationMessage) MarshalJSON() ([]byte, error) {
	validation, err := validateContinuationContent(m.Content)
	if err != nil {
		return nil, err
	}
	content := make([]json.RawMessage, 0, len(m.Content))
	for index, block := range validation.blocks {
		if block.resumable {
			raw, err := canonicalContinuationBlock(m.Content[index], block)
			if err != nil {
				return nil, err
			}
			content = append(content, raw)
		}
	}
	type wire continuationMessage
	return json.Marshal(wire{Version: m.Version, Role: m.Role, Content: content})
}

type continuationBlockInfo struct {
	kind      string
	id        string
	name      string
	caller    string
	callerSet bool
	thinking  bool
	resumable bool
}

type continuationContentValidation struct {
	blocks       []continuationBlockInfo
	hasThinking  bool
	hasResumable bool
}

type callResolver struct {
	byID   map[string]string
	byName map[string][]string
	used   map[string]struct{}
}

func newCallResolver() *callResolver {
	return &callResolver{
		byID: make(map[string]string), byName: make(map[string][]string), used: make(map[string]struct{}),
	}
}

func (r *callResolver) add(id, name string) error {
	if id == "" || name == "" {
		return nil
	}
	if existing, ok := r.byID[id]; ok {
		if existing != name {
			return fmt.Errorf("tool use ID %q is associated with both %q and %q", id, existing, name)
		}
		return nil
	}
	r.byID[id] = name
	r.byName[name] = append(r.byName[name], id)
	return nil
}

func (r *callResolver) resolve(callID, name string) (string, error) {
	if callID != "" {
		expected, ok := r.byID[callID]
		if !ok {
			return "", fmt.Errorf("function result references unknown call ID %q", callID)
		}
		if name != "" && expected != name {
			return "", fmt.Errorf("function result name %q does not match call %q name %q", name, callID, expected)
		}
		if _, used := r.used[callID]; used {
			return "", fmt.Errorf("function result repeats call ID %q", callID)
		}
		r.used[callID] = struct{}{}
		return callID, nil
	}

	ids := r.byName[name]
	match := ""
	for _, id := range ids {
		if _, used := r.used[id]; used {
			continue
		}
		if match != "" {
			return "", fmt.Errorf("function result name %q matches multiple unresolved tool calls", name)
		}
		match = id
	}
	if match == "" {
		return "", fmt.Errorf("function result name %q has no matching unresolved tool call ID", name)
	}
	r.used[match] = struct{}{}
	return match, nil
}

func buildMessageParams(binding protocol.Binding, request modelinvoker.Request, stream bool, extension RequestMapper) (anthropicsdk.MessageNewParams, modelinvoker.RawPayload, []modelinvoker.MappingDecision, error) {
	params := anthropicsdk.MessageNewParams{
		Model:     anthropicsdk.Model(request.Model),
		MaxTokens: request.Budget.MaxOutputTokens,
	}
	if params.MaxTokens <= 0 {
		return params, modelinvoker.RawPayload{}, nil, mappingError("messages.map", "Anthropic Messages requires max output tokens greater than zero")
	}
	decisions := make([]modelinvoker.MappingDecision, 0)

	for _, instruction := range request.Instructions {
		switch instruction.Role {
		case modelinvoker.RoleSystem:
			params.System = append(params.System, anthropicsdk.TextBlockParam{Text: instruction.Text})
		case modelinvoker.RoleDeveloper:
			if !request.AllowDegradation {
				return params, modelinvoker.RawPayload{}, decisions, mappingError("messages.map_instructions", "developer instructions require explicit degradation because Anthropic has only one top-level system tier")
			}
			params.System = append(params.System, anthropicsdk.TextBlockParam{Text: instruction.Text})
			decisions = append(decisions, degradation(modelinvoker.CapabilityTextGeneration,
				"developer instruction was promoted into Anthropic's top-level system prompt"))
		default:
			return params, modelinvoker.RawPayload{}, decisions, mappingError("messages.map_instructions", fmt.Sprintf("unsupported instruction role %q", instruction.Role))
		}
	}

	resolver := newCallResolver()
	if request.State != nil {
		continuation, stateHasThinking, err := decodeContinuation(binding, request, resolver)
		if err != nil {
			return params, modelinvoker.RawPayload{}, decisions, err
		}
		params.Messages = append(params.Messages, continuation)
		if stateHasThinking && (request.Reasoning == nil || request.Reasoning.Effort == modelinvoker.ReasoningEffortNone) {
			return params, modelinvoker.RawPayload{}, decisions, mappingError("messages.map_state", "a continuation containing thinking blocks must continue with thinking enabled")
		}
	}

	for index, item := range request.Input {
		switch item.Type {
		case modelinvoker.InputTypeMessage:
			if item.Message == nil {
				return params, modelinvoker.RawPayload{}, decisions, mappingError("messages.map_input", fmt.Sprintf("input %d message is nil", index))
			}
			var role anthropicsdk.MessageParamRole
			switch item.Message.Role {
			case modelinvoker.RoleUser:
				role = anthropicsdk.MessageParamRoleUser
			case modelinvoker.RoleAssistant:
				role = anthropicsdk.MessageParamRoleAssistant
			case modelinvoker.RoleSystem, modelinvoker.RoleDeveloper:
				return params, modelinvoker.RawPayload{}, decisions, mappingError("messages.map_input", fmt.Sprintf("input message role %q cannot appear in Anthropic message history; use Instructions", item.Message.Role))
			default:
				return params, modelinvoker.RawPayload{}, decisions, mappingError("messages.map_input", fmt.Sprintf("unsupported message role %q", item.Message.Role))
			}
			appendMessageBlock(&params.Messages, role, anthropicsdk.NewTextBlock(item.Message.Text))
		case modelinvoker.InputTypeFunctionCall:
			if item.FunctionCall == nil {
				return params, modelinvoker.RawPayload{}, decisions, mappingError("messages.map_input", fmt.Sprintf("input %d function call is nil", index))
			}
			arguments, err := jsonObject(item.FunctionCall.Arguments)
			if err != nil {
				return params, modelinvoker.RawPayload{}, decisions, mappingError("messages.map_input", fmt.Sprintf("function call %q arguments: %v", item.FunctionCall.Name, err))
			}
			if err := resolver.add(item.FunctionCall.ID, item.FunctionCall.Name); err != nil {
				return params, modelinvoker.RawPayload{}, decisions, mappingError("messages.map_input", err.Error())
			}
			appendMessageBlock(&params.Messages, anthropicsdk.MessageParamRoleAssistant,
				anthropicsdk.NewToolUseBlock(item.FunctionCall.ID, arguments, item.FunctionCall.Name))
		case modelinvoker.InputTypeFunctionResult:
			if item.FunctionResult == nil {
				return params, modelinvoker.RawPayload{}, decisions, mappingError("messages.map_input", fmt.Sprintf("input %d function result is nil", index))
			}
			callID, err := resolver.resolve(item.FunctionResult.CallID, item.FunctionResult.Name)
			if err != nil {
				return params, modelinvoker.RawPayload{}, decisions, mappingError("messages.map_input", err.Error())
			}
			appendMessageBlock(&params.Messages, anthropicsdk.MessageParamRoleUser,
				anthropicsdk.NewToolResultBlock(callID, item.FunctionResult.Output, item.FunctionResult.IsError))
		default:
			return params, modelinvoker.RawPayload{}, decisions, mappingError("messages.map_input", fmt.Sprintf("unsupported input type %q", item.Type))
		}
	}

	for _, tool := range request.Tools {
		if !nativeToolNamePattern.MatchString(tool.Name) {
			return params, modelinvoker.RawPayload{}, decisions, mappingError("messages.map_tools", fmt.Sprintf("tool name %q is invalid for Anthropic", tool.Name))
		}
		var schema anthropicsdk.ToolInputSchemaParam
		if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
			return params, modelinvoker.RawPayload{}, decisions, mappingError("messages.map_tools", fmt.Sprintf("tool %q schema: %v", tool.Name, err))
		}
		native := anthropicsdk.ToolUnionParamOfTool(schema, tool.Name)
		if tool.Description != "" {
			native.OfTool.Description = anthropicsdk.String(tool.Description)
		}
		if tool.Strict != nil {
			native.OfTool.Strict = anthropicsdk.Bool(*tool.Strict)
		}
		params.Tools = append(params.Tools, native)
	}

	if err := mapToolChoice(&params, request); err != nil {
		return params, modelinvoker.RawPayload{}, decisions, err
	}
	if err := mapOutput(&params, request, &decisions); err != nil {
		return params, modelinvoker.RawPayload{}, decisions, err
	}
	if err := mapReasoning(&params, request, &decisions); err != nil {
		return params, modelinvoker.RawPayload{}, decisions, err
	}
	if err := mapMetadata(&params, request, &decisions); err != nil {
		return params, modelinvoker.RawPayload{}, decisions, err
	}
	if extension != nil {
		mapped, extensionDecisions, err := extension.MapMessagesRequest(request, params)
		if err != nil {
			return params, modelinvoker.RawPayload{}, decisions, err
		}
		params = mapped
		decisions = append(decisions, extensionDecisions...)
	}

	raw, err := adaptercore.MarshalAuditRequest(params, stream)
	if err != nil {
		return params, modelinvoker.RawPayload{}, decisions, mappingError("messages.audit", "could not serialize Anthropic request: "+err.Error())
	}
	return params, raw, decisions, nil
}

func decodeContinuation(binding protocol.Binding, request modelinvoker.Request, resolver *callResolver) (anthropicsdk.MessageParam, bool, error) {
	state := request.State
	if state.Kind != modelinvoker.StateProviderContinuation {
		return anthropicsdk.MessageParam{}, false, mappingError("messages.map_state", fmt.Sprintf("unsupported state kind %q", state.Kind))
	}
	if state.Provider != binding.Provider || state.Protocol != binding.Protocol {
		return anthropicsdk.MessageParam{}, false, mappingError("messages.map_state", "provider continuation does not belong to this Messages binding")
	}
	payload := state.Payload.Bytes()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope == nil {
		if err == nil {
			err = fmt.Errorf("value must be a JSON object")
		}
		return anthropicsdk.MessageParam{}, false, mappingError("messages.map_state", "invalid Anthropic continuation payload: "+err.Error())
	}
	if err := rejectContinuationCacheControl(payload, "continuation payload"); err != nil {
		return anthropicsdk.MessageParam{}, false, mappingError("messages.map_state", err.Error())
	}
	if err := rejectUnknownContinuationFields(envelope, "continuation payload", "version", "role", "content"); err != nil {
		return anthropicsdk.MessageParam{}, false, mappingError("messages.map_state", err.Error())
	}
	var wire continuationMessage
	if err := json.Unmarshal(payload, &wire); err != nil {
		return anthropicsdk.MessageParam{}, false, mappingError("messages.map_state", "invalid Anthropic continuation payload: "+err.Error())
	}
	if wire.Version != continuationVersion {
		return anthropicsdk.MessageParam{}, false, mappingError("messages.map_state", fmt.Sprintf("unsupported Anthropic continuation version %d", wire.Version))
	}
	if wire.Role != "assistant" || len(wire.Content) == 0 {
		return anthropicsdk.MessageParam{}, false, mappingError("messages.map_state", "Anthropic continuation must contain a non-empty assistant message")
	}
	validation, err := validateContinuationContent(wire.Content)
	if err != nil {
		return anthropicsdk.MessageParam{}, false, mappingError("messages.map_state", err.Error())
	}
	if err := validateContinuationStateFields(wire.Content); err != nil {
		return anthropicsdk.MessageParam{}, false, mappingError("messages.map_state", err.Error())
	}
	if !validation.hasResumable {
		return anthropicsdk.MessageParam{}, false, mappingError("messages.map_state", "Anthropic continuation contains no thinking or tool-use state")
	}
	message := anthropicsdk.MessageParam{Role: anthropicsdk.MessageParamRoleAssistant}
	for index, raw := range wire.Content {
		info := validation.blocks[index]
		if !info.resumable {
			return anthropicsdk.MessageParam{}, false, mappingError("messages.map_state", fmt.Sprintf("continuation content %d type %q is not allowed in provider continuation state", index, info.kind))
		}
		if info.kind == "tool_use" {
			if !info.callerSet {
				return anthropicsdk.MessageParam{}, false, mappingError("messages.map_state", fmt.Sprintf("continuation content %d field %q is required", index, "caller"))
			}
			if err := resolver.add(info.id, info.name); err != nil {
				return anthropicsdk.MessageParam{}, false, mappingError("messages.map_state", err.Error())
			}
		}
		var block anthropicsdk.ContentBlockParamUnion
		if err := json.Unmarshal(raw, &block); err != nil {
			return anthropicsdk.MessageParam{}, false, mappingError("messages.map_state", fmt.Sprintf("continuation content %d cannot be mapped: %v", index, err))
		}
		message.Content = append(message.Content, block)
	}
	return message, validation.hasThinking, nil
}

func validateContinuationContent(content []json.RawMessage) (continuationContentValidation, error) {
	validation := continuationContentValidation{blocks: make([]continuationBlockInfo, len(content))}
	for index, raw := range content {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			if err == nil {
				err = fmt.Errorf("value must be a JSON object")
			}
			return continuationContentValidation{}, fmt.Errorf("continuation content %d is invalid: %v", index, err)
		}
		if err := rejectContinuationCacheControl(raw, fmt.Sprintf("continuation content %d", index)); err != nil {
			return continuationContentValidation{}, err
		}

		kind, err := continuationStringField(fields, "type", index, true)
		if err != nil {
			return continuationContentValidation{}, err
		}
		info := continuationBlockInfo{kind: kind}
		switch kind {
		case "text":
			if _, err := continuationStringField(fields, "text", index, false); err != nil {
				return continuationContentValidation{}, err
			}
		case "thinking":
			if _, err := continuationStringField(fields, "thinking", index, true); err != nil {
				return continuationContentValidation{}, err
			}
			if _, err := continuationStringField(fields, "signature", index, true); err != nil {
				return continuationContentValidation{}, err
			}
			info.thinking = true
			info.resumable = true
		case "redacted_thinking":
			if _, err := continuationStringField(fields, "data", index, true); err != nil {
				return continuationContentValidation{}, err
			}
			info.thinking = true
			info.resumable = true
		case "tool_use":
			info.id, err = continuationStringField(fields, "id", index, true)
			if err != nil {
				return continuationContentValidation{}, err
			}
			info.name, err = continuationStringField(fields, "name", index, true)
			if err != nil {
				return continuationContentValidation{}, err
			}
			if !nativeToolNamePattern.MatchString(info.name) {
				return continuationContentValidation{}, fmt.Errorf("continuation content %d tool name is invalid for Anthropic", index)
			}
			input, exists := fields["input"]
			if !exists {
				return continuationContentValidation{}, fmt.Errorf("continuation content %d field %q is required", index, "input")
			}
			if _, err := jsonObject(input); err != nil {
				return continuationContentValidation{}, fmt.Errorf("continuation content %d tool input: %v", index, err)
			}
			info.caller = "direct"
			if caller, exists := fields["caller"]; exists {
				info.callerSet = true
				if err := rejectContinuationCacheControl(caller, fmt.Sprintf("continuation content %d caller", index)); err != nil {
					return continuationContentValidation{}, err
				}
				var callerFields map[string]json.RawMessage
				if err := json.Unmarshal(caller, &callerFields); err != nil || callerFields == nil {
					if err == nil {
						err = fmt.Errorf("value must be a JSON object")
					}
					return continuationContentValidation{}, fmt.Errorf("continuation content %d caller is invalid: %v", index, err)
				}
				info.caller, err = continuationStringField(callerFields, "type", index, true)
				if err != nil {
					return continuationContentValidation{}, err
				}
			}
			if info.caller != "direct" {
				return continuationContentValidation{}, fmt.Errorf("continuation content %d tool caller %q is unsupported; only direct is allowed", index, info.caller)
			}
			info.resumable = true
		default:
			return continuationContentValidation{}, fmt.Errorf("continuation content %d type %q is unsupported", index, kind)
		}
		validation.blocks[index] = info
		validation.hasThinking = validation.hasThinking || info.thinking
		validation.hasResumable = validation.hasResumable || info.resumable
	}
	return validation, nil
}

func canonicalContinuationBlock(raw json.RawMessage, info continuationBlockInfo) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		if err == nil {
			err = fmt.Errorf("value must be a JSON object")
		}
		return nil, err
	}
	canonical := make(map[string]json.RawMessage)
	canonical["type"] = fields["type"]
	switch info.kind {
	case "thinking":
		canonical["thinking"] = fields["thinking"]
		canonical["signature"] = fields["signature"]
	case "redacted_thinking":
		canonical["data"] = fields["data"]
	case "tool_use":
		canonical["id"] = fields["id"]
		canonical["name"] = fields["name"]
		canonical["input"] = fields["input"]
		canonical["caller"] = json.RawMessage(`{"type":"direct"}`)
	default:
		return nil, fmt.Errorf("continuation block type %q is not resumable", info.kind)
	}
	encoded, err := json.Marshal(canonical)
	return json.RawMessage(encoded), err
}

func validateContinuationStateFields(content []json.RawMessage) error {
	for index, raw := range content {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			if err == nil {
				err = fmt.Errorf("value must be a JSON object")
			}
			return fmt.Errorf("continuation content %d is invalid: %v", index, err)
		}
		kind, err := continuationStringField(fields, "type", index, true)
		if err != nil {
			return err
		}
		location := fmt.Sprintf("continuation content %d", index)
		switch kind {
		case "thinking":
			if err := rejectUnknownContinuationFields(fields, location, "type", "thinking", "signature"); err != nil {
				return err
			}
		case "redacted_thinking":
			if err := rejectUnknownContinuationFields(fields, location, "type", "data"); err != nil {
				return err
			}
		case "tool_use":
			if err := rejectUnknownContinuationFields(fields, location, "type", "id", "name", "input", "caller"); err != nil {
				return err
			}
			caller, exists := fields["caller"]
			if !exists {
				return fmt.Errorf("continuation content %d field %q is required", index, "caller")
			}
			var callerFields map[string]json.RawMessage
			if err := json.Unmarshal(caller, &callerFields); err != nil || callerFields == nil {
				if err == nil {
					err = fmt.Errorf("value must be a JSON object")
				}
				return fmt.Errorf("continuation content %d caller is invalid: %v", index, err)
			}
			if err := rejectUnknownContinuationFields(callerFields, location+" caller", "type"); err != nil {
				return err
			}
		case "text":
			return fmt.Errorf("continuation content %d type %q is not allowed in provider continuation state", index, kind)
		default:
			return fmt.Errorf("continuation content %d type %q is unsupported", index, kind)
		}
	}
	return nil
}

func rejectContinuationCacheControl(raw json.RawMessage, location string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		if err == nil {
			err = fmt.Errorf("value must be a JSON object")
		}
		return fmt.Errorf("%s is invalid: %v", location, err)
	}
	if _, exists := fields["cache_control"]; exists {
		return fmt.Errorf("%s uses unsupported cache_control", location)
	}
	return nil
}

func rejectUnknownContinuationFields(fields map[string]json.RawMessage, location string, allowed ...string) error {
	allow := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allow[field] = struct{}{}
	}
	unknown := make([]string, 0)
	for field := range fields {
		if _, exists := allow[field]; !exists {
			unknown = append(unknown, field)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("%s field %q is unsupported", location, unknown[0])
}

func continuationStringField(fields map[string]json.RawMessage, name string, index int, nonEmpty bool) (string, error) {
	raw, exists := fields[name]
	if !exists {
		return "", fmt.Errorf("continuation content %d field %q is required", index, name)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("continuation content %d field %q must be a string", index, name)
	}
	if nonEmpty && strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("continuation content %d field %q must not be empty", index, name)
	}
	return value, nil
}

func appendMessageBlock(messages *[]anthropicsdk.MessageParam, role anthropicsdk.MessageParamRole, block anthropicsdk.ContentBlockParamUnion) {
	if len(*messages) > 0 && (*messages)[len(*messages)-1].Role == role {
		(*messages)[len(*messages)-1].Content = append((*messages)[len(*messages)-1].Content, block)
		return
	}
	*messages = append(*messages, anthropicsdk.MessageParam{Role: role, Content: []anthropicsdk.ContentBlockParamUnion{block}})
}

func mapToolChoice(params *anthropicsdk.MessageNewParams, request modelinvoker.Request) error {
	if request.Reasoning != nil && request.Reasoning.Effort != modelinvoker.ReasoningEffortNone &&
		(request.ToolChoice.Mode == modelinvoker.ToolChoiceRequired || request.ToolChoice.Mode == modelinvoker.ToolChoiceFunction) {
		return mappingError("messages.map_tool_choice", "Anthropic thinking is incompatible with forced tool choice")
	}

	switch request.ToolChoice.Mode {
	case modelinvoker.ToolChoiceAuto:
		if request.ParallelToolCalls != nil {
			choice := anthropicsdk.ToolChoiceAutoParam{DisableParallelToolUse: anthropicsdk.Bool(!*request.ParallelToolCalls)}
			params.ToolChoice = anthropicsdk.ToolChoiceUnionParam{OfAuto: &choice}
		}
	case modelinvoker.ToolChoiceNone:
		choice := anthropicsdk.NewToolChoiceNoneParam()
		params.ToolChoice = anthropicsdk.ToolChoiceUnionParam{OfNone: &choice}
	case modelinvoker.ToolChoiceRequired:
		choice := anthropicsdk.ToolChoiceAnyParam{}
		if request.ParallelToolCalls != nil {
			choice.DisableParallelToolUse = anthropicsdk.Bool(!*request.ParallelToolCalls)
		}
		params.ToolChoice = anthropicsdk.ToolChoiceUnionParam{OfAny: &choice}
	case modelinvoker.ToolChoiceFunction:
		params.ToolChoice = anthropicsdk.ToolChoiceParamOfTool(request.ToolChoice.Name)
		if request.ParallelToolCalls != nil {
			params.ToolChoice.OfTool.DisableParallelToolUse = anthropicsdk.Bool(!*request.ParallelToolCalls)
		}
	default:
		return mappingError("messages.map_tool_choice", fmt.Sprintf("unsupported tool choice %q", request.ToolChoice.Mode))
	}
	return nil
}

func mapOutput(params *anthropicsdk.MessageNewParams, request modelinvoker.Request, decisions *[]modelinvoker.MappingDecision) error {
	switch request.Output.Type {
	case modelinvoker.OutputText:
		return nil
	case modelinvoker.OutputJSONObject:
		return mappingError("messages.map_output", "Anthropic has no native json_object output mode")
	case modelinvoker.OutputJSONSchema:
		if request.Output.Strict != nil && !*request.Output.Strict {
			return mappingError("messages.map_output", "Anthropic JSON Schema output cannot explicitly disable constrained decoding")
		}
		schema, err := jsonObject(request.Output.Schema)
		if err != nil {
			return mappingError("messages.map_output", err.Error())
		}
		if request.Output.Description != "" {
			if current, ok := schema["description"].(string); ok && current != request.Output.Description {
				return mappingError("messages.map_output", "output description conflicts with schema description")
			}
			schema["description"] = request.Output.Description
		}
		params.OutputConfig.Format = anthropicsdk.JSONOutputFormatParam{Schema: schema}
		*decisions = append(*decisions, transformed(modelinvoker.CapabilityStructuredOutput,
			fmt.Sprintf("output schema name %q remains a Praxis identifier; Anthropic receives the schema itself", request.Output.Name)))
		return nil
	default:
		return mappingError("messages.map_output", fmt.Sprintf("unsupported output constraint %q", request.Output.Type))
	}
}

func mapReasoning(params *anthropicsdk.MessageNewParams, request modelinvoker.Request, decisions *[]modelinvoker.MappingDecision) error {
	reasoning := request.Reasoning
	if reasoning == nil {
		return nil
	}

	display := "omitted"
	switch reasoning.Summary {
	case "":
	case modelinvoker.ReasoningSummaryAuto:
		display = "summarized"
	case modelinvoker.ReasoningSummaryConcise, modelinvoker.ReasoningSummaryDetailed:
		if !request.AllowDegradation {
			return mappingError("messages.map_reasoning", "Anthropic cannot control concise or detailed thinking-summary style without explicit degradation")
		}
		display = "summarized"
		*decisions = append(*decisions, degradation(modelinvoker.CapabilityReasoningSummary,
			fmt.Sprintf("reasoning summary style %q degraded to Anthropic summarized thinking", reasoning.Summary)))
	default:
		return mappingError("messages.map_reasoning", fmt.Sprintf("unsupported reasoning summary %q", reasoning.Summary))
	}

	if reasoning.BudgetTokens != nil {
		if *reasoning.BudgetTokens < 1024 || *reasoning.BudgetTokens >= request.Budget.MaxOutputTokens {
			return mappingError("messages.map_reasoning", "thinking budget must be at least 1024 and less than max output tokens")
		}
		params.Thinking = anthropicsdk.ThinkingConfigParamOfEnabled(*reasoning.BudgetTokens)
		params.Thinking.OfEnabled.Display = anthropicsdk.ThinkingConfigEnabledDisplay(display)
		return mapReasoningEffort(params, request, decisions)
	}

	if reasoning.Effort == modelinvoker.ReasoningEffortNone {
		if reasoning.Summary != "" {
			return mappingError("messages.map_reasoning", "disabled reasoning cannot request a thinking summary")
		}
		disabled := anthropicsdk.NewThinkingConfigDisabledParam()
		params.Thinking = anthropicsdk.ThinkingConfigParamUnion{OfDisabled: &disabled}
		return nil
	}

	adaptive := anthropicsdk.ThinkingConfigAdaptiveParam{Display: anthropicsdk.ThinkingConfigAdaptiveDisplay(display)}
	params.Thinking = anthropicsdk.ThinkingConfigParamUnion{OfAdaptive: &adaptive}
	return mapReasoningEffort(params, request, decisions)
}

func mapReasoningEffort(params *anthropicsdk.MessageNewParams, request modelinvoker.Request, decisions *[]modelinvoker.MappingDecision) error {
	reasoning := request.Reasoning
	switch reasoning.Effort {
	case "":
	case modelinvoker.ReasoningEffortMinimal:
		if !request.AllowDegradation {
			return mappingError("messages.map_reasoning", "minimal reasoning effort requires explicit degradation to low")
		}
		params.OutputConfig.Effort = anthropicsdk.OutputConfigEffortLow
		*decisions = append(*decisions, degradation(modelinvoker.CapabilityReasoning,
			"minimal reasoning effort degraded to Anthropic low effort"))
	case modelinvoker.ReasoningEffortLow:
		params.OutputConfig.Effort = anthropicsdk.OutputConfigEffortLow
	case modelinvoker.ReasoningEffortMedium:
		params.OutputConfig.Effort = anthropicsdk.OutputConfigEffortMedium
	case modelinvoker.ReasoningEffortHigh:
		params.OutputConfig.Effort = anthropicsdk.OutputConfigEffortHigh
	case modelinvoker.ReasoningEffortXHigh:
		params.OutputConfig.Effort = anthropicsdk.OutputConfigEffortXhigh
	case modelinvoker.ReasoningEffortMax:
		params.OutputConfig.Effort = anthropicsdk.OutputConfigEffortMax
	default:
		return mappingError("messages.map_reasoning", fmt.Sprintf("unsupported reasoning effort %q", reasoning.Effort))
	}
	return nil
}

func mapMetadata(params *anthropicsdk.MessageNewParams, request modelinvoker.Request, decisions *[]modelinvoker.MappingDecision) error {
	for key, value := range request.Metadata {
		if key == "user_id" {
			params.Metadata.UserID = anthropicsdk.String(value)
			continue
		}
		if !request.AllowDegradation {
			return mappingError("messages.map_metadata", fmt.Sprintf("Anthropic cannot represent metadata key %q", key))
		}
		*decisions = append(*decisions, degradation(modelinvoker.CapabilityTextGeneration,
			fmt.Sprintf("metadata key %q was not sent to Anthropic", key)))
	}
	return nil
}

func jsonObject(raw json.RawMessage) (map[string]any, error) {
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("value must be a JSON object")
	}
	return object, nil
}
