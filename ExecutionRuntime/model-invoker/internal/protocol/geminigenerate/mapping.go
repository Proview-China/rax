package geminigenerate

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"google.golang.org/genai"
)

type mappedRequest struct {
	contents  []*genai.Content
	config    *genai.GenerateContentConfig
	raw       modelinvoker.RawPayload
	decisions []modelinvoker.MappingDecision
	envelope  continuationEnvelope
}

func buildMappedRequest(request modelinvoker.Request) (mappedRequest, error) {
	mapped := mappedRequest{config: &genai.GenerateContentConfig{}}
	envelope, err := decodeContinuation(request.State)
	if err != nil {
		return mapped, mappingError("generate_content.map_state", err.Error())
	}
	contents, err := cloneContents(envelope.Contents)
	if err != nil {
		return mapped, mappingError("generate_content.map_state", "failed to clone Gemini continuation contents")
	}
	envelope.Contents = contents
	mapped.contents = contents
	mapped.envelope = envelope

	if err := mapInputs(request, &mapped); err != nil {
		return mapped, err
	}
	if err := mapInstructions(request, &mapped); err != nil {
		return mapped, err
	}
	if err := mapTools(request, &mapped); err != nil {
		return mapped, err
	}
	if err := mapOutput(request, &mapped); err != nil {
		return mapped, err
	}
	if err := mapReasoning(request, &mapped); err != nil {
		return mapped, err
	}
	if request.Budget.MaxOutputTokens > 0 {
		if request.Budget.MaxOutputTokens > math.MaxInt32 {
			return mapped, mappingError("generate_content.map_budget", "max output tokens exceed Gemini int32 range")
		}
		mapped.config.MaxOutputTokens = int32(request.Budget.MaxOutputTokens)
	}
	if len(request.Metadata) > 0 {
		if !request.AllowDegradation {
			return mapped, mappingError("generate_content.map_metadata", "Gemini Developer API does not support request metadata labels")
		}
		mapped.decisions = append(mapped.decisions, degradation(
			modelinvoker.CapabilityTextGeneration,
			"request metadata omitted because labels are available only on the Vertex backend",
		))
	}

	if len(mapped.contents) == 0 {
		return mapped, mappingError("generate_content.map_input", "Gemini request contains no content after mapping")
	}
	mapped.envelope.Contents = mapped.contents
	audit := map[string]any{
		"model":    request.Model,
		"contents": mapped.contents,
		"config":   mapped.config,
	}
	raw, err := adaptercore.MarshalAuditRequest(audit, request.Stream)
	if err != nil {
		return mapped, mappingError("generate_content.serialize", "failed to serialize Gemini audit request")
	}
	mapped.raw = raw
	return mapped, nil
}

func mapInputs(request modelinvoker.Request, mapped *mappedRequest) error {
	for inputIndex, item := range request.Input {
		switch item.Type {
		case modelinvoker.InputTypeMessage:
			role := genai.RoleUser
			switch item.Message.Role {
			case modelinvoker.RoleUser:
			case modelinvoker.RoleAssistant:
				role = genai.RoleModel
			default:
				return mappingError("generate_content.map_input", fmt.Sprintf("message role %q is not a Gemini conversation role; use Request.Instructions", item.Message.Role))
			}
			appendContentPart(&mapped.contents, role, &genai.Part{Text: item.Message.Text})
		case modelinvoker.InputTypeFunctionCall:
			var arguments map[string]any
			if err := json.Unmarshal(item.FunctionCall.Arguments, &arguments); err != nil || arguments == nil {
				return mappingError("generate_content.map_function_call", fmt.Sprintf("input %d function call arguments must be a JSON object", inputIndex))
			}
			native := &genai.FunctionCall{
				ID: item.FunctionCall.ID, Name: item.FunctionCall.Name, Args: arguments,
			}
			contentIndex, partIndex := nextPartPosition(mapped.contents, genai.RoleModel)
			if _, err := addInputContinuationCall(&mapped.envelope, contentIndex, partIndex, native); err != nil {
				return mappingError("generate_content.map_function_call", err.Error())
			}
			appendContentPart(&mapped.contents, genai.RoleModel, &genai.Part{FunctionCall: native})
		case modelinvoker.InputTypeFunctionResult:
			id, call, err := resolveFunctionResult(&mapped.envelope, item.FunctionResult)
			if err != nil {
				return mappingError("generate_content.map_function_result", err.Error())
			}
			response := map[string]any{"output": item.FunctionResult.Output}
			if item.FunctionResult.IsError {
				response = map[string]any{"error": item.FunctionResult.Output}
			}
			appendContentPart(&mapped.contents, genai.RoleUser, &genai.Part{FunctionResponse: &genai.FunctionResponse{
				ID: call.NativeID, Name: call.Name, Response: response,
			}})
			call.Responded = true
			mapped.envelope.Calls[id] = call
		default:
			return mappingError("generate_content.map_input", fmt.Sprintf("unsupported input type %q", item.Type))
		}
	}
	return nil
}

func appendContentPart(contents *[]*genai.Content, role string, part *genai.Part) {
	if len(*contents) > 0 && (*contents)[len(*contents)-1].Role == role {
		last := (*contents)[len(*contents)-1]
		last.Parts = append(last.Parts, part)
		return
	}
	*contents = append(*contents, &genai.Content{Role: role, Parts: []*genai.Part{part}})
}

func nextPartPosition(contents []*genai.Content, role string) (int, int) {
	if len(contents) > 0 && contents[len(contents)-1].Role == role {
		return len(contents) - 1, len(contents[len(contents)-1].Parts)
	}
	return len(contents), 0
}

func mapInstructions(request modelinvoker.Request, mapped *mappedRequest) error {
	if len(request.Instructions) == 0 {
		return nil
	}
	parts := make([]*genai.Part, 0, len(request.Instructions))
	for _, instruction := range request.Instructions {
		if instruction.Role == modelinvoker.RoleDeveloper {
			if !request.AllowDegradation {
				return mappingError("generate_content.map_instructions", "Gemini has no separate developer-instruction priority tier")
			}
			mapped.decisions = append(mapped.decisions, degradation(
				modelinvoker.CapabilityTextGeneration,
				"developer instruction collapsed into Gemini systemInstruction",
			))
		}
		parts = append(parts, &genai.Part{Text: instruction.Text})
	}
	mapped.config.SystemInstruction = &genai.Content{Role: genai.RoleUser, Parts: parts}
	return nil
}

func mapTools(request modelinvoker.Request, mapped *mappedRequest) error {
	if len(request.Tools) == 0 {
		return nil
	}
	declarations := make([]*genai.FunctionDeclaration, 0, len(request.Tools))
	strictTrue := false
	strictFalse := false
	for _, tool := range request.Tools {
		schema, err := decodeJSONSchema(tool.Parameters, true)
		if err != nil {
			return mappingError("generate_content.map_tools", fmt.Sprintf("tool %q: %s", tool.Name, err))
		}
		declarations = append(declarations, &genai.FunctionDeclaration{
			Name: tool.Name, Description: tool.Description, ParametersJsonSchema: schema,
		})
		if tool.Strict != nil {
			strictTrue = strictTrue || *tool.Strict
			strictFalse = strictFalse || !*tool.Strict
		}
	}
	if strictTrue && strictFalse {
		if !request.AllowDegradation {
			return mappingError("generate_content.map_tools", "Gemini strict function decoding is global and cannot preserve mixed strict=true/false declarations")
		}
		mapped.decisions = append(mapped.decisions, degradation(
			modelinvoker.CapabilityToolCalling,
			"global constrained decoding applied to tools with mixed strictness",
		))
	}
	mapped.config.Tools = []*genai.Tool{{FunctionDeclarations: declarations}}

	mode := genai.FunctionCallingConfigModeAuto
	var allowed []string
	switch request.ToolChoice.Mode {
	case modelinvoker.ToolChoiceAuto:
		if strictTrue {
			mode = genai.FunctionCallingConfigModeValidated
		}
	case modelinvoker.ToolChoiceNone:
		mode = genai.FunctionCallingConfigModeNone
	case modelinvoker.ToolChoiceRequired:
		mode = genai.FunctionCallingConfigModeAny
	case modelinvoker.ToolChoiceFunction:
		mode = genai.FunctionCallingConfigModeAny
		allowed = []string{request.ToolChoice.Name}
	default:
		return mappingError("generate_content.map_tools", fmt.Sprintf("unsupported tool choice %q", request.ToolChoice.Mode))
	}
	mapped.config.ToolConfig = &genai.ToolConfig{FunctionCallingConfig: &genai.FunctionCallingConfig{
		Mode: mode, AllowedFunctionNames: allowed,
	}}
	if request.ParallelToolCalls != nil && !*request.ParallelToolCalls {
		if !request.AllowDegradation {
			return mappingError("generate_content.map_tools", "Gemini has no request switch that prohibits parallel function calls")
		}
		mapped.decisions = append(mapped.decisions, degradation(
			modelinvoker.CapabilityParallelToolCalling,
			"parallel_tool_calls=false omitted because Gemini exposes no disable switch",
		))
	}
	return nil
}

func mapOutput(request modelinvoker.Request, mapped *mappedRequest) error {
	switch request.Output.Type {
	case modelinvoker.OutputText:
		return nil
	case modelinvoker.OutputJSONObject:
		mapped.config.ResponseMIMEType = "application/json"
		return nil
	case modelinvoker.OutputJSONSchema:
		schema, err := decodeJSONSchema(request.Output.Schema, false)
		if err != nil {
			return mappingError("generate_content.map_output", err.Error())
		}
		schema, err = cloneSchema(schema)
		if err != nil {
			return mappingError("generate_content.map_output", "failed to clone output schema")
		}
		if _, exists := schema["title"]; !exists && request.Output.Name != "" {
			schema["title"] = request.Output.Name
		}
		if _, exists := schema["description"]; !exists && request.Output.Description != "" {
			schema["description"] = request.Output.Description
		}
		mapped.config.ResponseMIMEType = "application/json"
		mapped.config.ResponseJsonSchema = schema
		if request.Output.Strict != nil && !*request.Output.Strict {
			mapped.decisions = append(mapped.decisions, transformation(
				modelinvoker.CapabilityStructuredOutput,
				"Gemini structured output always applies schema-constrained generation",
			))
		}
		return nil
	default:
		return mappingError("generate_content.map_output", fmt.Sprintf("unsupported output constraint %q", request.Output.Type))
	}
}

func mapReasoning(request modelinvoker.Request, mapped *mappedRequest) error {
	if request.Reasoning == nil {
		return nil
	}
	if request.Reasoning.BudgetTokens != nil && request.Reasoning.Effort != "" {
		return mappingError(
			"generate_content.map_reasoning",
			"Gemini thinkingBudget and thinkingLevel target different model families and cannot be sent together",
		)
	}
	config := &genai.ThinkingConfig{}
	if request.Reasoning.BudgetTokens != nil {
		if *request.Reasoning.BudgetTokens > math.MaxInt32 {
			return mappingError("generate_content.map_reasoning", "reasoning budget exceeds Gemini int32 range")
		}
		value := int32(*request.Reasoning.BudgetTokens)
		config.ThinkingBudget = &value
	}
	if request.Reasoning.Effort != "" {
		switch request.Reasoning.Effort {
		case modelinvoker.ReasoningEffortNone:
			return mappingError("generate_content.map_reasoning", "portable reasoning effort none cannot be guaranteed across Gemini thinking models")
		case modelinvoker.ReasoningEffortMinimal:
			config.ThinkingLevel = genai.ThinkingLevelMinimal
		case modelinvoker.ReasoningEffortLow:
			config.ThinkingLevel = genai.ThinkingLevelLow
		case modelinvoker.ReasoningEffortMedium:
			config.ThinkingLevel = genai.ThinkingLevelMedium
		case modelinvoker.ReasoningEffortHigh:
			config.ThinkingLevel = genai.ThinkingLevelHigh
		case modelinvoker.ReasoningEffortMax:
			if !request.AllowDegradation {
				return mappingError("generate_content.map_reasoning", "Gemini has no max thinking level")
			}
			config.ThinkingLevel = genai.ThinkingLevelHigh
			mapped.decisions = append(mapped.decisions, degradation(
				modelinvoker.CapabilityReasoning,
				"reasoning effort max reduced to Gemini thinking level HIGH",
			))
		case modelinvoker.ReasoningEffortXHigh:
			if !request.AllowDegradation {
				return mappingError("generate_content.map_reasoning", "Gemini has no xhigh thinking level")
			}
			config.ThinkingLevel = genai.ThinkingLevelHigh
			mapped.decisions = append(mapped.decisions, degradation(
				modelinvoker.CapabilityReasoning,
				"reasoning effort xhigh reduced to Gemini thinking level HIGH",
			))
		default:
			return mappingError("generate_content.map_reasoning", fmt.Sprintf("unsupported reasoning effort %q", request.Reasoning.Effort))
		}
	}
	if request.Reasoning.Summary != "" {
		if !request.AllowDegradation {
			return mappingError(
				"generate_content.map_reasoning",
				"Gemini thought parts are provider-native reasoning and cannot be promised as a portable reasoning summary",
			)
		}
		config.IncludeThoughts = true
		mapped.decisions = append(mapped.decisions, degradation(
			modelinvoker.CapabilityReasoningSummary,
			fmt.Sprintf("reasoning summary %s mapped to Gemini includeThoughts=true; returned thought parts remain provider-native reasoning", request.Reasoning.Summary),
		))
	}
	mapped.config.ThinkingConfig = config
	return nil
}

func transformation(capability modelinvoker.Capability, detail string) modelinvoker.MappingDecision {
	return modelinvoker.MappingDecision{Capability: capability, Action: modelinvoker.MappingTransformed, Detail: detail}
}

func degradation(capability modelinvoker.Capability, detail string) modelinvoker.MappingDecision {
	return modelinvoker.MappingDecision{Capability: capability, Action: modelinvoker.MappingDegraded, Detail: detail}
}

func optionsAreEmpty(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && len(object) == 0
}

func containsConversationOnlyRoleError(request modelinvoker.Request) string {
	for _, item := range request.Input {
		if item.Type != modelinvoker.InputTypeMessage || item.Message == nil {
			continue
		}
		if item.Message.Role == modelinvoker.RoleSystem || item.Message.Role == modelinvoker.RoleDeveloper {
			return fmt.Sprintf("message role %q is not supported in Gemini contents; use Request.Instructions", item.Message.Role)
		}
	}
	return ""
}

func normalizedEndpoint(request modelinvoker.Request, configured string) string {
	return adaptercore.EffectiveEndpoint(strings.TrimSpace(request.Endpoint), configured)
}
