package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openairesponses"
)

const (
	ProviderID      modelinvoker.ProviderID = "openai"
	defaultBaseURL                          = "https://api.openai.com/v1"
	requestIDHeader                         = "x-request-id"
)

type Adapter struct {
	client                 nativeClient
	chatDriver             *openaichat.Driver
	chatPublicBinding      protocol.Binding
	responsesDriver        *openairesponses.Driver
	responsesPublicBinding protocol.Binding
	baseURL                string
	redactor               adaptercore.Redactor
}

func (Adapter) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "openai.Adapter([REDACTED])")
}

func (Adapter) GoString() string {
	return "openai.Adapter([REDACTED])"
}

func New(config Config) (*Adapter, error) {
	redactor := adaptercore.NewRedactor(config.APIKey)
	if err := config.validate(); err != nil {
		return nil, redactor.Error(&modelinvoker.Error{
			Kind:      modelinvoker.ErrorInvalidRequest,
			Provider:  ProviderID,
			Operation: "configure",
			Message:   err.Error(),
			Err:       err,
		})
	}
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	adapter, err := newWithClient(newSDKClient(config), baseURL, redactor)
	if err != nil {
		return nil, redactor.Error(providerError(modelinvoker.ErrorInvalidRequest, "configure", err.Error(), nil))
	}
	return adapter, nil
}

func newWithClient(client nativeClient, baseURL string, redactor adaptercore.Redactor) (*Adapter, error) {
	endpoint := adaptercore.NormalizeEndpoint(baseURL)
	chatBinding, err := protocol.NewBinding(ProviderID, modelinvoker.ProtocolChatCompletions, endpoint, requestIDHeader)
	if err != nil {
		return nil, fmt.Errorf("configure Chat Completions binding: %w", err)
	}
	chatDriver, err := openaichat.New(chatBinding, chatDialect{}, chatDriverClient{native: client})
	if err != nil {
		return nil, err
	}
	responsesBinding, err := protocol.NewBinding(ProviderID, modelinvoker.ProtocolResponses, endpoint, requestIDHeader)
	if err != nil {
		return nil, fmt.Errorf("configure Responses binding: %w", err)
	}
	responsesDriver, err := openairesponses.New(responsesBinding, responsesDialect{}, responsesDriverClient{native: client})
	if err != nil {
		return nil, err
	}
	chatPublicBinding := chatBinding.Clone()
	chatPublicBinding.Endpoint = redactor.String(chatBinding.Endpoint)
	responsesPublicBinding := responsesBinding.Clone()
	responsesPublicBinding.Endpoint = redactor.String(responsesBinding.Endpoint)
	return &Adapter{
		client: client, chatDriver: chatDriver, chatPublicBinding: chatPublicBinding,
		responsesDriver: responsesDriver, responsesPublicBinding: responsesPublicBinding,
		baseURL: endpoint, redactor: redactor,
	}, nil
}

func (a *Adapter) ID() modelinvoker.ProviderID { return ProviderID }

func (a *Adapter) DefaultProtocol() modelinvoker.Protocol {
	return modelinvoker.ProtocolResponses
}

func (a *Adapter) publicBinding(protocolID modelinvoker.Protocol) (protocol.Binding, string, bool) {
	if a == nil {
		return protocol.Binding{}, "", false
	}
	switch protocolID {
	case modelinvoker.ProtocolResponses:
		if a.responsesDriver != nil {
			return a.responsesPublicBinding, "responses.create", true
		}
	case modelinvoker.ProtocolChatCompletions:
		if a.chatDriver != nil {
			return a.chatPublicBinding, "chat_completions.create", true
		}
	}
	return protocol.Binding{}, "", false
}

func (a *Adapter) Capabilities(ctx context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	if a == nil || a.client == nil {
		return nil, providerError(modelinvoker.ErrorProviderUnavailable, "capabilities", "adapter is not initialized", nil)
	}
	if ctx == nil {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "context is nil", nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, adaptercore.ContextError(ProviderID, "capabilities", err)
	}
	if strings.TrimSpace(query.Model) == "" {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "model is required", nil)
	}
	if query.Endpoint != "" && adaptercore.NormalizeEndpoint(query.Endpoint) != a.baseURL {
		return nil, mappingError("capabilities", "capability endpoint does not match the configured OpenAI endpoint")
	}
	contract := adaptercore.UnsupportedContract("outside the implemented OpenAI Responses and Chat Completions semantic slice")
	nativeMapping := fmt.Sprintf("adapter mapping is native; OpenAI validates model %q availability and feature support at invocation time", query.Model)
	switch query.Protocol {
	case modelinvoker.ProtocolResponses:
		adaptercore.SetSupport(contract, query, modelinvoker.SupportNative, nativeMapping,
			modelinvoker.CapabilityTextGeneration,
			modelinvoker.CapabilityStreaming,
			modelinvoker.CapabilityToolCalling,
			modelinvoker.CapabilityParallelToolCalling,
			modelinvoker.CapabilityStructuredOutput,
			modelinvoker.CapabilityReasoning,
			modelinvoker.CapabilityReasoningSummary,
			modelinvoker.CapabilityServerState,
			modelinvoker.CapabilityUsageReporting,
		)
		contract[modelinvoker.CapabilityFunctionErrorResult] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial,
			"function_call_output has no portable is_error marker; output text can be preserved only with explicit degradation")
	case modelinvoker.ProtocolChatCompletions:
		adaptercore.SetSupport(contract, query, modelinvoker.SupportNative, nativeMapping,
			modelinvoker.CapabilityTextGeneration,
			modelinvoker.CapabilityStreaming,
			modelinvoker.CapabilityToolCalling,
			modelinvoker.CapabilityParallelToolCalling,
			modelinvoker.CapabilityStructuredOutput,
			modelinvoker.CapabilityReasoning,
			modelinvoker.CapabilityUsageReporting,
		)
		contract[modelinvoker.CapabilityReasoningSummary] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial,
			"Chat Completions maps reasoning effort but cannot return a Responses-style reasoning summary")
		contract[modelinvoker.CapabilityFunctionErrorResult] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial,
			"tool messages have no portable is_error marker; output text can be preserved only with explicit degradation")
		contract[modelinvoker.CapabilityServerState] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported,
			"previous_response_id is a Responses protocol feature")
	default:
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", fmt.Sprintf("unsupported protocol %q", query.Protocol), nil)
	}
	return contract, nil
}

func (a *Adapter) Invoke(ctx context.Context, request modelinvoker.Request) (result modelinvoker.Response, resultErr error) {
	redactor := adaptercore.Redactor{}
	if a != nil {
		redactor = a.redactor
	}
	publicBinding, operation, bindAfterRedaction := a.publicBinding(request.Protocol)
	stampResponseAfterRedaction := false
	defer func() {
		if bindAfterRedaction {
			if stampResponseAfterRedaction {
				result = publicBinding.StampResponse(request, result)
			}
			resultErr = publicBinding.StampError(ctx, request, resultErr, operation)
		}
	}()
	defer func() {
		result = redactor.Response(result)
		resultErr = redactor.Error(resultErr)
	}()
	if ctx == nil {
		return modelinvoker.Response{}, providerError(modelinvoker.ErrorInvalidRequest, "invoke", "context is nil", nil)
	}
	request.Stream = false
	if err := a.validateRequest(request); err != nil {
		return modelinvoker.Response{}, err
	}
	switch request.Protocol {
	case modelinvoker.ProtocolResponses:
		stampResponseAfterRedaction = true
		return a.responsesDriver.Invoke(ctx, request)
	case modelinvoker.ProtocolChatCompletions:
		stampResponseAfterRedaction = true
		return a.chatDriver.Invoke(ctx, request)
	default:
		return modelinvoker.Response{}, providerError(modelinvoker.ErrorInvalidRequest, "invoke", fmt.Sprintf("unsupported protocol %q", request.Protocol), nil)
	}
}

func (a *Adapter) Stream(ctx context.Context, request modelinvoker.Request) (result modelinvoker.Stream, resultErr error) {
	redactor := adaptercore.Redactor{}
	if a != nil {
		redactor = a.redactor
	}
	publicBinding, operation, bindAfterRedaction := a.publicBinding(request.Protocol)
	if operation == "responses.create" {
		operation = "responses.stream"
	} else if operation == "chat_completions.create" {
		operation = "chat_completions.stream"
	}
	defer func() {
		if bindAfterRedaction {
			result = publicBinding.BindStream(ctx, request, result)
			resultErr = publicBinding.StampError(ctx, request, resultErr, operation)
		}
	}()
	defer func() {
		result = redactor.Stream(result)
		resultErr = redactor.Error(resultErr)
	}()
	if ctx == nil {
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "stream", "context is nil", nil)
	}
	request.Stream = true
	if err := a.validateRequest(request); err != nil {
		return nil, err
	}
	switch request.Protocol {
	case modelinvoker.ProtocolResponses:
		return a.responsesDriver.Stream(ctx, request)
	case modelinvoker.ProtocolChatCompletions:
		return a.chatDriver.Stream(ctx, request)
	default:
		return nil, providerError(modelinvoker.ErrorInvalidRequest, "stream", fmt.Sprintf("unsupported protocol %q", request.Protocol), nil)
	}
}

func (a *Adapter) validateRequest(request modelinvoker.Request) error {
	if a == nil || a.client == nil {
		return providerError(modelinvoker.ErrorProviderUnavailable, "validate", "adapter is not initialized", nil)
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if err := adaptercore.ValidateSelection(
		request,
		ProviderID,
		a.baseURL,
		modelinvoker.ProtocolResponses,
		modelinvoker.ProtocolChatCompletions,
	); err != nil {
		kind := modelinvoker.ErrorMapping
		if request.Provider != ProviderID || (request.Protocol != modelinvoker.ProtocolResponses && request.Protocol != modelinvoker.ProtocolChatCompletions) {
			kind = modelinvoker.ErrorInvalidRequest
		}
		return providerError(kind, "validate", err.Error(), nil)
	}
	if request.State != nil {
		if request.Protocol != modelinvoker.ProtocolResponses {
			return mappingError("validate", "server continuation state is not supported by Chat Completions")
		}
		if request.State.Kind != modelinvoker.StateServerContinuation {
			return mappingError("validate", "OpenAI Responses requires server continuation state")
		}
	}
	if err := validateOpenAIRequestSemantics(request); err != nil {
		return err
	}
	for _, options := range request.ProviderOptions {
		var object map[string]json.RawMessage
		if json.Unmarshal(options, &object) != nil || len(object) != 0 {
			return providerError(modelinvoker.ErrorMapping, "validate", "OpenAI provider options are not defined in the first implementation slice", nil)
		}
	}
	return nil
}

func validateOpenAIRequestSemantics(request modelinvoker.Request) error {
	for index, tool := range request.Tools {
		if !nativeToolNamePattern.MatchString(tool.Name) {
			return mappingError("validate", fmt.Sprintf("tool %d name is not valid for OpenAI", index))
		}
	}
	if request.ToolChoice.Mode == modelinvoker.ToolChoiceFunction && !nativeToolNamePattern.MatchString(request.ToolChoice.Name) {
		return mappingError("validate", "function tool choice name is not valid for OpenAI")
	}
	if request.Output.Type == modelinvoker.OutputJSONSchema && !nativeToolNamePattern.MatchString(request.Output.Name) {
		return mappingError("validate", "JSON Schema output name is not valid for OpenAI")
	}
	for index, input := range request.Input {
		switch input.Type {
		case modelinvoker.InputTypeFunctionCall:
			if input.FunctionCall == nil || strings.TrimSpace(input.FunctionCall.ID) == "" || !nativeToolNamePattern.MatchString(input.FunctionCall.Name) {
				return mappingError("validate", fmt.Sprintf("input %d OpenAI function call requires an ID and valid name", index))
			}
		case modelinvoker.InputTypeFunctionResult:
			if input.FunctionResult == nil || strings.TrimSpace(input.FunctionResult.CallID) == "" {
				return mappingError("validate", fmt.Sprintf("input %d OpenAI function result requires a call ID", index))
			}
			if input.FunctionResult.Name != "" && !nativeToolNamePattern.MatchString(input.FunctionResult.Name) {
				return mappingError("validate", fmt.Sprintf("input %d function result name is not valid for OpenAI", index))
			}
		}
	}
	if len(request.Metadata) > 16 {
		return mappingError("validate", "OpenAI metadata must contain at most 16 entries")
	}
	for key, value := range request.Metadata {
		if len(key) < 1 || len(key) > 64 || len(value) > 512 {
			return mappingError("validate", "OpenAI metadata keys must be 1-64 bytes and values at most 512 bytes")
		}
	}
	if request.Reasoning != nil {
		if request.Reasoning.BudgetTokens != nil {
			return mappingError("validate", "OpenAI does not support an explicit reasoning token budget")
		}
		if request.Reasoning.Effort == modelinvoker.ReasoningEffortMax {
			return mappingError("validate", "OpenAI does not support max reasoning effort")
		}
	}
	return nil
}

var _ modelinvoker.Provider = (*Adapter)(nil)
