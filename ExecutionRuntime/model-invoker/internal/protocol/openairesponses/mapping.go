package openairesponses

import (
	"fmt"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

func buildParams(request modelinvoker.Request, extension RequestMapper) (responses.ResponseNewParams, modelinvoker.RawPayload, []modelinvoker.MappingDecision, error) {
	params := responses.ResponseNewParams{Model: openaisdk.ChatModel(request.Model)}
	items := make(responses.ResponseInputParam, 0, len(request.Instructions)+len(request.Input))
	decisions := make([]modelinvoker.MappingDecision, 0)

	for _, instruction := range request.Instructions {
		role, err := responseRole(instruction.Role)
		if err != nil {
			return params, modelinvoker.RawPayload{}, decisions, err
		}
		items = append(items, responses.ResponseInputItemParamOfMessage(instruction.Text, role))
	}
	for _, item := range request.Input {
		switch item.Type {
		case modelinvoker.InputTypeMessage:
			role, err := responseRole(item.Message.Role)
			if err != nil {
				return params, modelinvoker.RawPayload{}, decisions, err
			}
			items = append(items, responses.ResponseInputItemParamOfMessage(item.Message.Text, role))
		case modelinvoker.InputTypeFunctionCall:
			items = append(items, responses.ResponseInputItemParamOfFunctionCall(
				string(item.FunctionCall.Arguments), item.FunctionCall.ID, item.FunctionCall.Name,
			))
		case modelinvoker.InputTypeFunctionResult:
			if item.FunctionResult.IsError {
				if !request.AllowDegradation {
					return params, modelinvoker.RawPayload{}, decisions, mappingError("responses.map_input", "OpenAI function_call_output has no portable is_error marker; allow degradation to pass the output text")
				}
				decisions = append(decisions, degradation(modelinvoker.CapabilityFunctionErrorResult, "function result is_error marker omitted; output text preserved"))
			}
			items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(
				item.FunctionResult.CallID, item.FunctionResult.Output,
			))
		default:
			return params, modelinvoker.RawPayload{}, decisions, mappingError("responses.map_input", fmt.Sprintf("unsupported input type %q", item.Type))
		}
	}
	params.Input.OfInputItemList = items

	for _, tool := range request.Tools {
		schema, err := schemaObject(tool.Parameters)
		if err != nil {
			return params, modelinvoker.RawPayload{}, decisions, mappingError("responses.map_tools", err.Error())
		}
		if tool.Strict != nil && *tool.Strict {
			if err := validateStrictSchema(schema, "$tool."+tool.Name); err != nil {
				return params, modelinvoker.RawPayload{}, decisions, mappingError("responses.map_tools", err.Error())
			}
		}
		nativeFunction := responses.FunctionToolParam{Name: tool.Name, Parameters: schema}
		if tool.Strict != nil {
			nativeFunction.Strict = openaisdk.Bool(*tool.Strict)
		}
		native := responses.ToolUnionParam{OfFunction: &nativeFunction}
		if tool.Description != "" {
			native.OfFunction.Description = openaisdk.String(tool.Description)
		}
		params.Tools = append(params.Tools, native)
	}
	if err := mapToolChoice(&params, request.ToolChoice); err != nil {
		return params, modelinvoker.RawPayload{}, decisions, err
	}
	if request.ParallelToolCalls != nil {
		params.ParallelToolCalls = openaisdk.Bool(*request.ParallelToolCalls)
	}
	if request.Budget.MaxOutputTokens > 0 {
		params.MaxOutputTokens = openaisdk.Int(request.Budget.MaxOutputTokens)
	}
	if request.State != nil {
		params.PreviousResponseID = openaisdk.String(request.State.ID)
	}
	params.Metadata = openaisdk.Metadata(request.Metadata)
	if err := mapOutput(&params, request.Output); err != nil {
		return params, modelinvoker.RawPayload{}, decisions, err
	}
	if request.Reasoning != nil {
		params.Reasoning = shared.ReasoningParam{
			Effort:  shared.ReasoningEffort(request.Reasoning.Effort),
			Summary: shared.ReasoningSummary(request.Reasoning.Summary),
		}
	}
	if extension != nil {
		mapped, extensionDecisions, err := extension.MapResponsesRequest(request, params)
		if err != nil {
			return params, modelinvoker.RawPayload{}, decisions, err
		}
		params = mapped
		decisions = append(decisions, extensionDecisions...)
	}

	raw, err := adaptercore.MarshalAuditRequest(params, request.Stream)
	if err != nil {
		return params, modelinvoker.RawPayload{}, decisions, mappingError("responses.serialize", err.Error())
	}
	return params, raw, decisions, nil
}

func responseRole(role modelinvoker.Role) (responses.EasyInputMessageRole, error) {
	switch role {
	case modelinvoker.RoleSystem:
		return responses.EasyInputMessageRoleSystem, nil
	case modelinvoker.RoleDeveloper:
		return responses.EasyInputMessageRoleDeveloper, nil
	case modelinvoker.RoleUser:
		return responses.EasyInputMessageRoleUser, nil
	case modelinvoker.RoleAssistant:
		return responses.EasyInputMessageRoleAssistant, nil
	default:
		return "", mappingError("responses.map_role", fmt.Sprintf("unsupported role %q", role))
	}
}

func mapToolChoice(params *responses.ResponseNewParams, choice modelinvoker.ToolChoice) error {
	switch choice.Mode {
	case modelinvoker.ToolChoiceAuto:
		if len(params.Tools) > 0 {
			params.ToolChoice.OfToolChoiceMode = openaisdk.Opt(responses.ToolChoiceOptionsAuto)
		}
	case modelinvoker.ToolChoiceNone:
		params.ToolChoice.OfToolChoiceMode = openaisdk.Opt(responses.ToolChoiceOptionsNone)
	case modelinvoker.ToolChoiceRequired:
		params.ToolChoice.OfToolChoiceMode = openaisdk.Opt(responses.ToolChoiceOptionsRequired)
	case modelinvoker.ToolChoiceFunction:
		params.ToolChoice.OfFunctionTool = &responses.ToolChoiceFunctionParam{Name: choice.Name}
	default:
		return mappingError("responses.map_tool_choice", fmt.Sprintf("unsupported tool choice %q", choice.Mode))
	}
	return nil
}

func mapOutput(params *responses.ResponseNewParams, output modelinvoker.OutputConstraint) error {
	switch output.Type {
	case modelinvoker.OutputText:
		return nil
	case modelinvoker.OutputJSONObject:
		object := shared.NewResponseFormatJSONObjectParam()
		params.Text.Format.OfJSONObject = &object
	case modelinvoker.OutputJSONSchema:
		schema, err := schemaObject(output.Schema)
		if err != nil {
			return mappingError("responses.map_output", err.Error())
		}
		if output.Strict != nil && *output.Strict {
			if err := validateStrictSchema(schema, "$output"); err != nil {
				return mappingError("responses.map_output", err.Error())
			}
		}
		format := responses.ResponseFormatTextConfigParamOfJSONSchema(output.Name, schema)
		if output.Strict != nil {
			format.OfJSONSchema.Strict = openaisdk.Bool(*output.Strict)
		}
		if output.Description != "" {
			format.OfJSONSchema.Description = openaisdk.String(output.Description)
		}
		params.Text.Format = format
	default:
		return mappingError("responses.map_output", fmt.Sprintf("unsupported output constraint %q", output.Type))
	}
	return nil
}
