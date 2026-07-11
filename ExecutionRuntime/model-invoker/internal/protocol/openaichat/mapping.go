package openaichat

import (
	"fmt"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

func buildParams(request modelinvoker.Request, extension RequestMapper) (openaisdk.ChatCompletionNewParams, modelinvoker.RawPayload, []modelinvoker.MappingDecision, error) {
	params := openaisdk.ChatCompletionNewParams{Model: openaisdk.ChatModel(request.Model)}
	decisions := make([]modelinvoker.MappingDecision, 0)
	messages := make([]openaisdk.ChatCompletionMessageParamUnion, 0, len(request.Instructions)+len(request.Input))

	for _, instruction := range request.Instructions {
		message, err := textMessage(instruction.Role, instruction.Text)
		if err != nil {
			return params, modelinvoker.RawPayload{}, decisions, err
		}
		messages = append(messages, message)
	}
	var pendingCalls *openaisdk.ChatCompletionAssistantMessageParam
	flushCalls := func() {
		if pendingCalls != nil {
			messages = append(messages, openaisdk.ChatCompletionMessageParamUnion{OfAssistant: pendingCalls})
			pendingCalls = nil
		}
	}
	for _, item := range request.Input {
		switch item.Type {
		case modelinvoker.InputTypeMessage:
			flushCalls()
			message, err := textMessage(item.Message.Role, item.Message.Text)
			if err != nil {
				return params, modelinvoker.RawPayload{}, decisions, err
			}
			messages = append(messages, message)
		case modelinvoker.InputTypeFunctionCall:
			if pendingCalls == nil {
				pendingCalls = &openaisdk.ChatCompletionAssistantMessageParam{}
			}
			pendingCalls.ToolCalls = append(pendingCalls.ToolCalls, openaisdk.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openaisdk.ChatCompletionMessageFunctionToolCallParam{
					ID: item.FunctionCall.ID,
					Function: openaisdk.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name: item.FunctionCall.Name, Arguments: string(item.FunctionCall.Arguments),
					},
				},
			})
		case modelinvoker.InputTypeFunctionResult:
			flushCalls()
			if item.FunctionResult.IsError {
				if !request.AllowDegradation {
					return params, modelinvoker.RawPayload{}, decisions, mappingError("chat_completions.map_input", "tool messages have no portable is_error marker; allow degradation to pass the output text")
				}
				decisions = append(decisions, degradation(modelinvoker.CapabilityFunctionErrorResult, "function result is_error marker omitted; output text preserved"))
			}
			messages = append(messages, openaisdk.ToolMessage(item.FunctionResult.Output, item.FunctionResult.CallID))
		default:
			return params, modelinvoker.RawPayload{}, decisions, mappingError("chat_completions.map_input", fmt.Sprintf("unsupported input type %q", item.Type))
		}
	}
	flushCalls()
	params.Messages = messages

	for _, tool := range request.Tools {
		schema, err := schemaObject(tool.Parameters)
		if err != nil {
			return params, modelinvoker.RawPayload{}, decisions, mappingError("chat_completions.map_tools", err.Error())
		}
		definition := shared.FunctionDefinitionParam{Name: tool.Name, Parameters: shared.FunctionParameters(schema)}
		if tool.Strict != nil {
			definition.Strict = openaisdk.Bool(*tool.Strict)
		}
		if tool.Strict != nil && *tool.Strict {
			if err := validateStrictSchema(schema, "$tool."+tool.Name); err != nil {
				return params, modelinvoker.RawPayload{}, decisions, mappingError("chat_completions.map_tools", err.Error())
			}
		}
		if tool.Description != "" {
			definition.Description = openaisdk.String(tool.Description)
		}
		params.Tools = append(params.Tools, openaisdk.ChatCompletionFunctionTool(definition))
	}
	if err := mapToolChoice(&params, request.ToolChoice); err != nil {
		return params, modelinvoker.RawPayload{}, decisions, err
	}
	if request.ParallelToolCalls != nil {
		params.ParallelToolCalls = openaisdk.Bool(*request.ParallelToolCalls)
	}
	if request.Budget.MaxOutputTokens > 0 {
		params.MaxCompletionTokens = openaisdk.Int(request.Budget.MaxOutputTokens)
	}
	params.Metadata = openaisdk.Metadata(request.Metadata)
	if err := mapOutput(&params, request.Output); err != nil {
		return params, modelinvoker.RawPayload{}, decisions, err
	}
	if request.Reasoning != nil {
		params.ReasoningEffort = shared.ReasoningEffort(request.Reasoning.Effort)
		if request.Reasoning.Summary != "" {
			if !request.AllowDegradation {
				return params, modelinvoker.RawPayload{}, decisions, mappingError("chat_completions.map_reasoning", "reasoning summary is not supported by Chat Completions")
			}
			decisions = append(decisions, degradation(modelinvoker.CapabilityReasoningSummary, "reasoning summary request omitted; effort preserved"))
		}
	}
	if request.Stream {
		params.StreamOptions.IncludeUsage = openaisdk.Bool(true)
	}
	if extension != nil {
		mapped, extensionDecisions, err := extension.MapChatRequest(request, params)
		if err != nil {
			return params, modelinvoker.RawPayload{}, decisions, err
		}
		params = mapped
		decisions = append(decisions, extensionDecisions...)
	}

	raw, err := adaptercore.MarshalAuditRequest(params, request.Stream)
	if err != nil {
		return params, modelinvoker.RawPayload{}, decisions, mappingError("chat_completions.serialize", err.Error())
	}
	return params, raw, decisions, nil
}

func textMessage(role modelinvoker.Role, text string) (openaisdk.ChatCompletionMessageParamUnion, error) {
	switch role {
	case modelinvoker.RoleSystem:
		return openaisdk.SystemMessage(text), nil
	case modelinvoker.RoleDeveloper:
		return openaisdk.DeveloperMessage(text), nil
	case modelinvoker.RoleUser:
		return openaisdk.UserMessage(text), nil
	case modelinvoker.RoleAssistant:
		return openaisdk.AssistantMessage(text), nil
	default:
		return openaisdk.ChatCompletionMessageParamUnion{}, mappingError("chat_completions.map_role", fmt.Sprintf("unsupported role %q", role))
	}
}

func mapToolChoice(params *openaisdk.ChatCompletionNewParams, choice modelinvoker.ToolChoice) error {
	switch choice.Mode {
	case modelinvoker.ToolChoiceAuto:
		if len(params.Tools) > 0 {
			params.ToolChoice.OfAuto = openaisdk.String(string(openaisdk.ChatCompletionToolChoiceOptionAutoAuto))
		}
	case modelinvoker.ToolChoiceNone:
		params.ToolChoice.OfAuto = openaisdk.String(string(openaisdk.ChatCompletionToolChoiceOptionAutoNone))
	case modelinvoker.ToolChoiceRequired:
		params.ToolChoice.OfAuto = openaisdk.String(string(openaisdk.ChatCompletionToolChoiceOptionAutoRequired))
	case modelinvoker.ToolChoiceFunction:
		params.ToolChoice = openaisdk.ToolChoiceOptionFunctionToolChoice(openaisdk.ChatCompletionNamedToolChoiceFunctionParam{Name: choice.Name})
	default:
		return mappingError("chat_completions.map_tool_choice", fmt.Sprintf("unsupported tool choice %q", choice.Mode))
	}
	return nil
}

func mapOutput(params *openaisdk.ChatCompletionNewParams, output modelinvoker.OutputConstraint) error {
	switch output.Type {
	case modelinvoker.OutputText:
		return nil
	case modelinvoker.OutputJSONObject:
		object := shared.NewResponseFormatJSONObjectParam()
		params.ResponseFormat.OfJSONObject = &object
	case modelinvoker.OutputJSONSchema:
		schema, err := schemaObject(output.Schema)
		if err != nil {
			return mappingError("chat_completions.map_output", err.Error())
		}
		if output.Strict != nil && *output.Strict {
			if err := validateStrictSchema(schema, "$output"); err != nil {
				return mappingError("chat_completions.map_output", err.Error())
			}
		}
		nativeSchema := shared.ResponseFormatJSONSchemaJSONSchemaParam{Name: output.Name, Schema: schema}
		if output.Description != "" {
			nativeSchema.Description = openaisdk.String(output.Description)
		}
		if output.Strict != nil {
			nativeSchema.Strict = openaisdk.Bool(*output.Strict)
		}
		params.ResponseFormat.OfJSONSchema = &shared.ResponseFormatJSONSchemaParam{JSONSchema: nativeSchema}
	default:
		return mappingError("chat_completions.map_output", fmt.Sprintf("unsupported output constraint %q", output.Type))
	}
	return nil
}
