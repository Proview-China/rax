package bedrock

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsruntime "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	awstypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

type invokeOptions struct {
	Body                json.RawMessage `json:"body"`
	Accept              string          `json:"accept,omitempty"`
	ContentType         string          `json:"content_type,omitempty"`
	GuardrailIdentifier string          `json:"guardrail_identifier,omitempty"`
	GuardrailVersion    string          `json:"guardrail_version,omitempty"`
}

func buildConverseInput(request modelinvoker.Request) (*awsruntime.ConverseInput, modelinvoker.RawPayload, []modelinvoker.MappingDecision, error) {
	if request.State != nil {
		return nil, modelinvoker.RawPayload{}, nil, mappingError("bedrock_converse.map", "server or provider continuation state is not supported")
	}
	if request.Output.Type != modelinvoker.OutputText {
		return nil, modelinvoker.RawPayload{}, nil, mappingError("bedrock_converse.map", "portable structured output is model-specific in Bedrock Converse")
	}
	if request.Reasoning != nil {
		return nil, modelinvoker.RawPayload{}, nil, mappingError("bedrock_converse.map", "portable reasoning controls are model-specific in Bedrock Converse")
	}
	if request.ParallelToolCalls != nil && *request.ParallelToolCalls && !request.AllowDegradation {
		return nil, modelinvoker.RawPayload{}, nil, mappingError("bedrock_converse.map", "parallel tool calls require explicit degradation because support is model-specific")
	}
	if err := requireEmptyProviderOptions(request); err != nil {
		return nil, modelinvoker.RawPayload{}, nil, err
	}

	input := &awsruntime.ConverseInput{ModelId: aws.String(request.Model)}
	for _, instruction := range request.Instructions {
		input.System = append(input.System, &awstypes.SystemContentBlockMemberText{Value: instruction.Text})
	}
	for index, item := range request.Input {
		message, err := mapInputItem(item)
		if err != nil {
			return nil, modelinvoker.RawPayload{}, nil, mappingError("bedrock_converse.map", fmt.Sprintf("input %d: %s", index, err))
		}
		input.Messages = append(input.Messages, message)
	}
	if request.Budget.MaxOutputTokens > 0 {
		if request.Budget.MaxOutputTokens > math.MaxInt32 {
			return nil, modelinvoker.RawPayload{}, nil, mappingError("bedrock_converse.map", "max output tokens exceed Bedrock Converse int32 limit")
		}
		input.InferenceConfig = &awstypes.InferenceConfiguration{MaxTokens: aws.Int32(int32(request.Budget.MaxOutputTokens))}
	}
	input.RequestMetadata = cloneMetadata(request.Metadata)
	toolConfig, err := mapTools(request)
	if err != nil {
		return nil, modelinvoker.RawPayload{}, nil, err
	}
	input.ToolConfig = toolConfig

	audit := map[string]any{
		"model_id": request.Model, "messages": len(input.Messages), "system_blocks": len(input.System),
		"tools": len(request.Tools), "stream": request.Stream, "request_metadata": input.RequestMetadata,
	}
	raw, _ := json.Marshal(audit)
	decisions := []modelinvoker.MappingDecision{
		{Capability: modelinvoker.CapabilityTextGeneration, Action: modelinvoker.MappingTransformed, Detail: "Praxis input mapped to Bedrock Converse messages"},
		{Capability: modelinvoker.CapabilityUsageReporting, Action: modelinvoker.MappingExact},
	}
	if len(request.Tools) > 0 {
		decisions = append(decisions, modelinvoker.MappingDecision{Capability: modelinvoker.CapabilityToolCalling, Action: modelinvoker.MappingTransformed, Detail: "Praxis tools mapped to Bedrock toolConfig"})
	}
	if request.ParallelToolCalls != nil && *request.ParallelToolCalls {
		decisions = append(decisions, modelinvoker.MappingDecision{Capability: modelinvoker.CapabilityParallelToolCalling, Action: modelinvoker.MappingDegraded, Detail: "parallel behavior remains model-selected"})
	}
	return input, modelinvoker.NewRawPayload(raw), decisions, nil
}

func mapInputItem(item modelinvoker.InputItem) (awstypes.Message, error) {
	switch item.Type {
	case modelinvoker.InputTypeMessage:
		if item.Message == nil {
			return awstypes.Message{}, fmt.Errorf("message is nil")
		}
		role := awstypes.ConversationRoleUser
		if item.Message.Role == modelinvoker.RoleAssistant {
			role = awstypes.ConversationRoleAssistant
		} else if item.Message.Role != modelinvoker.RoleUser {
			return awstypes.Message{}, fmt.Errorf("role %q must be expressed through Request.Instructions", item.Message.Role)
		}
		return awstypes.Message{Role: role, Content: []awstypes.ContentBlock{&awstypes.ContentBlockMemberText{Value: item.Message.Text}}}, nil
	case modelinvoker.InputTypeFunctionCall:
		if item.FunctionCall == nil {
			return awstypes.Message{}, fmt.Errorf("function call is nil")
		}
		var arguments any
		if err := json.Unmarshal(item.FunctionCall.Arguments, &arguments); err != nil {
			return awstypes.Message{}, fmt.Errorf("function call arguments are invalid JSON")
		}
		return awstypes.Message{Role: awstypes.ConversationRoleAssistant, Content: []awstypes.ContentBlock{
			&awstypes.ContentBlockMemberToolUse{Value: awstypes.ToolUseBlock{ToolUseId: aws.String(item.FunctionCall.ID), Name: aws.String(item.FunctionCall.Name), Input: document.NewLazyDocument(arguments)}},
		}}, nil
	case modelinvoker.InputTypeFunctionResult:
		if item.FunctionResult == nil {
			return awstypes.Message{}, fmt.Errorf("function result is nil")
		}
		status := awstypes.ToolResultStatusSuccess
		if item.FunctionResult.IsError {
			status = awstypes.ToolResultStatusError
		}
		return awstypes.Message{Role: awstypes.ConversationRoleUser, Content: []awstypes.ContentBlock{
			&awstypes.ContentBlockMemberToolResult{Value: awstypes.ToolResultBlock{ToolUseId: aws.String(item.FunctionResult.CallID), Status: status, Content: []awstypes.ToolResultContentBlock{
				&awstypes.ToolResultContentBlockMemberText{Value: item.FunctionResult.Output},
			}}},
		}}, nil
	default:
		return awstypes.Message{}, fmt.Errorf("unsupported input type %q", item.Type)
	}
}

func mapTools(request modelinvoker.Request) (*awstypes.ToolConfiguration, error) {
	if len(request.Tools) == 0 {
		return nil, nil
	}
	configuration := &awstypes.ToolConfiguration{}
	for index, tool := range request.Tools {
		var schema any
		if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
			return nil, mappingError("bedrock_converse.map", fmt.Sprintf("tool %d schema is invalid", index))
		}
		configuration.Tools = append(configuration.Tools, &awstypes.ToolMemberToolSpec{Value: awstypes.ToolSpecification{
			Name: aws.String(tool.Name), Description: optionalString(tool.Description), Strict: tool.Strict,
			InputSchema: &awstypes.ToolInputSchemaMemberJson{Value: document.NewLazyDocument(schema)},
		}})
	}
	switch request.ToolChoice.Mode {
	case modelinvoker.ToolChoiceAuto:
		configuration.ToolChoice = &awstypes.ToolChoiceMemberAuto{Value: awstypes.AutoToolChoice{}}
	case modelinvoker.ToolChoiceRequired:
		configuration.ToolChoice = &awstypes.ToolChoiceMemberAny{Value: awstypes.AnyToolChoice{}}
	case modelinvoker.ToolChoiceFunction:
		configuration.ToolChoice = &awstypes.ToolChoiceMemberTool{Value: awstypes.SpecificToolChoice{Name: aws.String(request.ToolChoice.Name)}}
	case modelinvoker.ToolChoiceNone:
		return nil, mappingError("bedrock_converse.map", "tool_choice none cannot be sent with Bedrock tools")
	default:
		return nil, mappingError("bedrock_converse.map", "unsupported tool choice")
	}
	return configuration, nil
}

func buildInvokeInput(request modelinvoker.Request) (*awsruntime.InvokeModelInput, modelinvoker.RawPayload, error) {
	if request.State != nil || len(request.Tools) > 0 || request.Reasoning != nil || request.Output.Type != modelinvoker.OutputText {
		return nil, modelinvoker.RawPayload{}, mappingError("bedrock_invoke_model.map", "InvokeModel raw mode does not infer portable tools, state, reasoning, or structured output")
	}
	raw, ok := request.ProviderOptions[request.Provider]
	if !ok {
		return nil, modelinvoker.RawPayload{}, mappingError("bedrock_invoke_model.map", "provider options with a raw JSON body are required")
	}
	var options invokeOptions
	if err := json.Unmarshal(raw, &options); err != nil || len(options.Body) == 0 || !json.Valid(options.Body) {
		return nil, modelinvoker.RawPayload{}, mappingError("bedrock_invoke_model.map", "provider options body must be valid JSON")
	}
	contentType := options.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	accept := options.Accept
	if accept == "" {
		accept = "application/json"
	}
	if contentType != "application/json" || strings.ContainsAny(accept+contentType, "\r\n") {
		return nil, modelinvoker.RawPayload{}, mappingError("bedrock_invoke_model.map", "InvokeModel content type must be application/json and headers must be safe")
	}
	if (options.GuardrailIdentifier == "") != (options.GuardrailVersion == "") {
		return nil, modelinvoker.RawPayload{}, mappingError("bedrock_invoke_model.map", "guardrail identifier and version must be configured together")
	}
	input := &awsruntime.InvokeModelInput{
		ModelId: aws.String(request.Model), Body: append([]byte(nil), options.Body...),
		Accept: aws.String(accept), ContentType: aws.String(contentType),
		GuardrailIdentifier: optionalString(options.GuardrailIdentifier), GuardrailVersion: optionalString(options.GuardrailVersion),
	}
	return input, modelinvoker.NewRawPayload(options.Body), nil
}

func requireEmptyProviderOptions(request modelinvoker.Request) error {
	for namespace, raw := range request.ProviderOptions {
		if namespace != request.Provider {
			return mappingError("bedrock_converse.map", "provider options namespace does not match Bedrock Runtime")
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil || len(object) != 0 {
			return mappingError("bedrock_converse.map", "provider options are reserved for model-specific slices")
		}
	}
	return nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return aws.String(value)
}

func cloneMetadata(source modelinvoker.Metadata) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func normalizeConverseOutput(request modelinvoker.Request, output *awsruntime.ConverseOutput, rawRequest modelinvoker.RawPayload, decisions []modelinvoker.MappingDecision) (modelinvoker.Response, error) {
	if output == nil {
		return failedResponse(request, rawRequest, decisions), fmt.Errorf("Bedrock Converse returned nil output")
	}
	response := modelinvoker.Response{
		Provider: request.Provider, Protocol: request.Protocol, Model: request.Model, Status: modelinvoker.ResponseStatusCompleted,
		StopReason: normalizeStopReason(string(output.StopReason)), RawRequest: rawRequest,
		MappingReport: modelinvoker.MappingReport{Provider: request.Provider, Protocol: request.Protocol, Endpoint: request.Endpoint, Decisions: append([]modelinvoker.MappingDecision(nil), decisions...)},
	}
	if message, ok := output.Output.(*awstypes.ConverseOutputMemberMessage); ok {
		for _, block := range message.Value.Content {
			switch value := block.(type) {
			case *awstypes.ContentBlockMemberText:
				response.Output = append(response.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemText, Text: value.Value})
			case *awstypes.ContentBlockMemberToolUse:
				encoded := []byte("null")
				if value.Value.Input != nil {
					var err error
					encoded, err = value.Value.Input.MarshalSmithyDocument()
					if err != nil || !json.Valid(encoded) {
						return failedResponse(request, rawRequest, decisions), fmt.Errorf("decode Bedrock tool input")
					}
				}
				response.Output = append(response.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: aws.ToString(value.Value.ToolUseId), Name: aws.ToString(value.Value.Name), Arguments: encoded}})
			}
		}
	}
	response.Usage = normalizeUsage(output.Usage)
	raw, _ := json.Marshal(map[string]any{"stop_reason": output.StopReason, "output_items": len(response.Output), "usage": response.Usage})
	response.RawResponse = modelinvoker.NewRawPayload(raw)
	return response, nil
}

func failedResponse(request modelinvoker.Request, rawRequest modelinvoker.RawPayload, decisions []modelinvoker.MappingDecision) modelinvoker.Response {
	return modelinvoker.Response{Provider: request.Provider, Protocol: request.Protocol, Model: request.Model, Status: modelinvoker.ResponseStatusFailed, RawRequest: rawRequest,
		MappingReport: modelinvoker.MappingReport{Provider: request.Provider, Protocol: request.Protocol, Endpoint: request.Endpoint, Decisions: append([]modelinvoker.MappingDecision(nil), decisions...)}}
}

func normalizeUsage(value *awstypes.TokenUsage) modelinvoker.Usage {
	if value == nil {
		return modelinvoker.Usage{}
	}
	return modelinvoker.Usage{InputTokens: int64(aws.ToInt32(value.InputTokens)), OutputTokens: int64(aws.ToInt32(value.OutputTokens)), TotalTokens: int64(aws.ToInt32(value.TotalTokens)), CacheReadTokens: int64(aws.ToInt32(value.CacheReadInputTokens)), CacheWriteTokens: int64(aws.ToInt32(value.CacheWriteInputTokens))}
}

func normalizeStopReason(value string) modelinvoker.StopReason {
	switch value {
	case "end_turn":
		return modelinvoker.StopReasonEndTurn
	case "max_tokens":
		return modelinvoker.StopReasonMaxOutputTokens
	case "stop_sequence":
		return modelinvoker.StopReasonStopSequence
	case "tool_use":
		return modelinvoker.StopReasonToolCall
	case "content_filtered", "guardrail_intervened":
		return modelinvoker.StopReasonContentFilter
	default:
		return modelinvoker.StopReasonOther
	}
}

func mappingError(operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: modelinvoker.ErrorMapping, Operation: operation, Message: message}
}
