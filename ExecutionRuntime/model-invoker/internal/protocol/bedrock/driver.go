package bedrock

import (
	"context"
	"errors"
	"fmt"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	awsruntime "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// Driver owns Bedrock Runtime Converse and InvokeModel wire semantics.
type Driver struct {
	base   *protocol.Base
	client Client
}

func New(binding protocol.Binding, dialect protocol.Dialect, client Client) (*Driver, error) {
	if binding.Protocol != modelinvoker.ProtocolBedrockConverse && binding.Protocol != modelinvoker.ProtocolBedrockInvoke {
		return nil, fmt.Errorf("create Bedrock driver: unsupported protocol %q", binding.Protocol)
	}
	if protocol.IsNil(client) {
		return nil, fmt.Errorf("create Bedrock driver: client is nil")
	}
	base, err := protocol.NewBase(binding, dialect)
	if err != nil {
		return nil, fmt.Errorf("create Bedrock driver: %w", err)
	}
	return &Driver{base: base, client: client}, nil
}

func (d *Driver) Binding() protocol.Binding {
	if d == nil || d.base == nil {
		return protocol.Binding{}
	}
	return d.base.Binding()
}

func (d *Driver) Invoke(ctx context.Context, request modelinvoker.Request) (modelinvoker.Response, error) {
	if d == nil || d.base == nil || protocol.IsNil(d.client) {
		return modelinvoker.Response{}, providerError(request.Provider, modelinvoker.ErrorProviderUnavailable, "invoke", "Bedrock driver is not initialized")
	}
	if ctx == nil {
		return modelinvoker.Response{}, providerError(request.Provider, modelinvoker.ErrorInvalidRequest, "invoke", "context is nil")
	}
	request.Stream = false
	if err := d.base.Validate(request); err != nil {
		return modelinvoker.Response{}, err
	}
	switch d.Binding().Protocol {
	case modelinvoker.ProtocolBedrockConverse:
		input, rawRequest, decisions, err := buildConverseInput(request)
		if err != nil {
			return failedResponse(request, rawRequest, decisions), d.base.StampError(ctx, request, err, "bedrock_converse.map")
		}
		output, callErr := d.client.Converse(ctx, input)
		if callErr != nil {
			return failedResponse(request, rawRequest, decisions), d.base.StampError(ctx, request, safeFailure(request.Provider, "bedrock_converse", callErr), "bedrock_converse")
		}
		response, normalizeErr := normalizeConverseOutput(request, output, rawRequest, decisions)
		return d.base.StampResponse(request, response), d.base.StampError(ctx, request, normalizeErr, "bedrock_converse.normalize")
	case modelinvoker.ProtocolBedrockInvoke:
		input, rawRequest, err := buildInvokeInput(request)
		if err != nil {
			return failedResponse(request, rawRequest, nil), d.base.StampError(ctx, request, err, "bedrock_invoke_model.map")
		}
		output, callErr := d.client.InvokeModel(ctx, input)
		if callErr != nil {
			return failedResponse(request, rawRequest, nil), d.base.StampError(ctx, request, safeFailure(request.Provider, "bedrock_invoke_model", callErr), "bedrock_invoke_model")
		}
		if output == nil {
			return failedResponse(request, rawRequest, nil), d.base.StampError(ctx, request, providerError(request.Provider, modelinvoker.ErrorProvider, "bedrock_invoke_model", "Bedrock InvokeModel returned nil output"), "bedrock_invoke_model")
		}
		response := modelinvoker.Response{Provider: request.Provider, Protocol: request.Protocol, Model: request.Model, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonOther,
			RawRequest: rawRequest, RawResponse: modelinvoker.NewRawPayload(output.Body), MappingReport: modelinvoker.MappingReport{Provider: request.Provider, Protocol: request.Protocol, Endpoint: request.Endpoint,
				Decisions: []modelinvoker.MappingDecision{{Capability: modelinvoker.CapabilityTextGeneration, Action: modelinvoker.MappingTransformed, Detail: "InvokeModel response retained as provider-native JSON"}}}}
		return d.base.StampResponse(request, response), nil
	default:
		return modelinvoker.Response{}, providerError(request.Provider, modelinvoker.ErrorInvalidRequest, "invoke", "unsupported Bedrock protocol")
	}
}

func (d *Driver) Stream(ctx context.Context, request modelinvoker.Request) (modelinvoker.Stream, error) {
	if d == nil || d.base == nil || protocol.IsNil(d.client) {
		return nil, providerError(request.Provider, modelinvoker.ErrorProviderUnavailable, "stream", "Bedrock driver is not initialized")
	}
	if ctx == nil {
		return nil, providerError(request.Provider, modelinvoker.ErrorInvalidRequest, "stream", "context is nil")
	}
	request.Stream = true
	if err := d.base.Validate(request); err != nil {
		return nil, err
	}
	switch d.Binding().Protocol {
	case modelinvoker.ProtocolBedrockConverse:
		input, rawRequest, decisions, err := buildConverseInput(request)
		if err != nil {
			return nil, d.base.StampError(ctx, request, err, "bedrock_converse.map")
		}
		streamInput := &awsruntime.ConverseStreamInput{ModelId: input.ModelId, Messages: input.Messages, System: input.System, InferenceConfig: input.InferenceConfig, ToolConfig: input.ToolConfig, RequestMetadata: input.RequestMetadata}
		native, callErr := d.client.ConverseStream(ctx, streamInput)
		if callErr != nil {
			return nil, d.base.StampError(ctx, request, safeFailure(request.Provider, "bedrock_converse.stream", callErr), "bedrock_converse.stream")
		}
		if protocol.IsNil(native) {
			return nil, providerError(request.Provider, modelinvoker.ErrorStreamInterrupted, "bedrock_converse.stream", "Bedrock returned a nil stream")
		}
		return d.base.BindStream(ctx, request, newConverseStream(ctx, request, native, rawRequest, decisions)), nil
	case modelinvoker.ProtocolBedrockInvoke:
		input, rawRequest, err := buildInvokeInput(request)
		if err != nil {
			return nil, d.base.StampError(ctx, request, err, "bedrock_invoke_model.map")
		}
		streamInput := &awsruntime.InvokeModelWithResponseStreamInput{ModelId: input.ModelId, Body: input.Body, Accept: input.Accept, ContentType: input.ContentType, GuardrailIdentifier: input.GuardrailIdentifier, GuardrailVersion: input.GuardrailVersion}
		native, callErr := d.client.InvokeModelWithResponseStream(ctx, streamInput)
		if callErr != nil {
			return nil, d.base.StampError(ctx, request, safeFailure(request.Provider, "bedrock_invoke_model.stream", callErr), "bedrock_invoke_model.stream")
		}
		if protocol.IsNil(native) {
			return nil, providerError(request.Provider, modelinvoker.ErrorStreamInterrupted, "bedrock_invoke_model.stream", "Bedrock returned a nil stream")
		}
		return d.base.BindStream(ctx, request, newInvokeStream(ctx, request, native, rawRequest)), nil
	default:
		return nil, providerError(request.Provider, modelinvoker.ErrorInvalidRequest, "stream", "unsupported Bedrock protocol")
	}
}

func safeFailure(provider modelinvoker.ProviderID, operation string, err error) *modelinvoker.Error {
	result := &modelinvoker.Error{Kind: modelinvoker.ErrorProvider, Provider: provider, Operation: operation, Message: "Bedrock operation failed"}
	if errors.Is(err, context.Canceled) {
		result.Kind, result.Message = modelinvoker.ErrorCancelled, "Bedrock operation was cancelled"
		return result
	}
	if errors.Is(err, context.DeadlineExceeded) {
		result.Kind, result.Message, result.Retryable = modelinvoker.ErrorTimeout, "Bedrock operation timed out", true
		return result
	}
	var responseError *smithyhttp.ResponseError
	if errors.As(err, &responseError) && responseError != nil {
		result.HTTPStatus = responseError.HTTPStatusCode()
		classifyHTTPStatus(result)
	}
	var apiError smithy.APIError
	if errors.As(err, &apiError) && apiError != nil {
		result.Code = safeAWSCode(apiError.ErrorCode())
		classifyAWSCode(result, result.Code)
	}
	return result
}

func classifyHTTPStatus(result *modelinvoker.Error) {
	switch result.HTTPStatus {
	case 400:
		result.Kind = modelinvoker.ErrorInvalidRequest
	case 401:
		result.Kind = modelinvoker.ErrorAuthentication
	case 403, 404:
		result.Kind = modelinvoker.ErrorPermission
	case 408:
		result.Kind, result.Retryable = modelinvoker.ErrorTimeout, true
	case 429:
		result.Kind, result.Retryable = modelinvoker.ErrorRateLimit, true
	case 500, 502, 503, 504:
		result.Kind, result.Retryable = modelinvoker.ErrorProviderUnavailable, true
	}
}

func classifyAWSCode(result *modelinvoker.Error, code string) {
	switch strings.ToLower(code) {
	case "validationexception":
		result.Kind = modelinvoker.ErrorInvalidRequest
	case "unrecognizedclientexception", "invalidsignatureexception", "expiredtokenexception":
		result.Kind = modelinvoker.ErrorAuthentication
	case "accessdeniedexception", "resourcenotfoundexception":
		result.Kind = modelinvoker.ErrorPermission
	case "throttlingexception", "servicequotaexceededexception":
		result.Kind, result.Retryable = modelinvoker.ErrorRateLimit, true
	case "modeltimeoutexception":
		result.Kind, result.Retryable = modelinvoker.ErrorTimeout, true
	case "modelnotreadyexception", "serviceunavailableexception", "internalserverexception":
		result.Kind, result.Retryable = modelinvoker.ErrorProviderUnavailable, true
	}
}

func safeAWSCode(value string) string {
	if len(value) > 128 {
		return ""
	}
	for _, character := range value {
		if !((character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' || character == '-' || character == '.') {
			return ""
		}
	}
	return value
}

func providerError(provider modelinvoker.ProviderID, kind modelinvoker.ErrorKind, operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Provider: provider, Operation: operation, Message: message}
}

var _ protocol.Driver = (*Driver)(nil)
