package plancompat

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/compatprovider"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
)

type planDialect struct {
	kind          Kind
	protocol      modelinvoker.Protocol
	allowedModels []string
}

func (dialect planDialect) ValidateRequest(request modelinvoker.Request) error {
	provider := providerID(dialect.kind)
	if !approvedModel(dialect.allowedModels, request.Model) {
		return providerError(provider, modelinvoker.ErrorMapping, "validate", "model is outside the approved subscription-plan text slice")
	}
	if request.Protocol != dialect.protocol {
		return providerError(provider, modelinvoker.ErrorInvalidRequest, "validate", "request protocol does not match the subscription route")
	}
	if request.Output.Type != modelinvoker.OutputText {
		return providerError(provider, modelinvoker.ErrorUnsupportedCapability, "validate", "structured output is not approved for this subscription slice")
	}
	if request.Reasoning != nil {
		return providerError(provider, modelinvoker.ErrorUnsupportedCapability, "validate", "portable reasoning controls are not approved for this subscription slice")
	}
	if request.State != nil {
		return providerError(provider, modelinvoker.ErrorUnsupportedCapability, "validate", "continuation state is not approved for this subscription slice")
	}
	if request.ParallelToolCalls != nil && *request.ParallelToolCalls {
		return providerError(provider, modelinvoker.ErrorUnsupportedCapability, "validate", "parallel tool control is not approved for this subscription slice")
	}
	if len(request.ProviderOptions) != 0 {
		return providerError(provider, modelinvoker.ErrorMapping, "validate", "provider options are not accepted by restricted subscription routes")
	}
	return nil
}

func (dialect planDialect) ClassifyFailure(failure protocol.Failure) protocol.ErrorClassification {
	kind, retryable := modelinvoker.ErrorProvider, false
	switch failure.HTTPStatus {
	case 400:
		kind = modelinvoker.ErrorInvalidRequest
	case 401:
		kind = modelinvoker.ErrorAuthentication
	case 402:
		kind = modelinvoker.ErrorBilling
	case 403, 404:
		kind = modelinvoker.ErrorPermission
	case 429:
		kind, retryable = modelinvoker.ErrorRateLimit, true
	case 500, 502, 503, 504:
		kind, retryable = modelinvoker.ErrorProviderUnavailable, true
	}
	if failure.Source == protocol.FailureSourceTransport {
		kind, retryable = modelinvoker.ErrorProviderUnavailable, true
	}
	if failure.Source == protocol.FailureSourceProtocol {
		kind, retryable = modelinvoker.ErrorProvider, false
	}
	code := failure.Code
	if code == "" {
		code = failure.Type
	}
	return protocol.ErrorClassification{Kind: kind, Code: code, Message: "subscription plan operation failed", Retryable: retryable, RetryAfter: failure.RetryAfter}
}

func (planDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	metadata := modelinvoker.ProviderMetadata{}
	for name, values := range headers {
		lower := strings.ToLower(name)
		if lower == "x-request-id" || lower == "request-id" || lower == "x-dashscope-request-id" || lower == "retry-after" || strings.HasPrefix(lower, "x-ratelimit-") {
			metadata[lower] = strings.Join(values, ",")
		}
	}
	return metadata
}

func capabilityContract(kind Kind, allowedModels []string) compatprovider.CapabilityBuilder {
	models := append([]string(nil), allowedModels...)
	return func(_ context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
		if !approvedModel(models, query.Model) {
			return nil, providerError(providerID(kind), modelinvoker.ErrorMapping, "capabilities", "model is outside the approved subscription-plan text slice")
		}
		if query.Protocol != modelinvoker.ProtocolChatCompletions && query.Protocol != modelinvoker.ProtocolMessages {
			return nil, providerError(providerID(kind), modelinvoker.ErrorInvalidRequest, "capabilities", "unsupported subscription protocol")
		}
		contract := adaptercore.UnsupportedContract("outside the approved interactive-coding subscription slice")
		adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible, "mapped through the official subscription compatibility endpoint", modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming, modelinvoker.CapabilityToolCalling, modelinvoker.CapabilityUsageReporting)
		contract[modelinvoker.CapabilityParallelToolCalling] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported, "parallel tool control is not approved in the restricted slice")
		contract[modelinvoker.CapabilityStructuredOutput] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported, "structured output is not approved in the restricted slice")
		contract[modelinvoker.CapabilityReasoning] = adaptercore.QuerySupport(query, modelinvoker.SupportUnsupported, "portable reasoning controls require plan-specific fixtures")
		if query.Protocol == modelinvoker.ProtocolMessages {
			contract[modelinvoker.CapabilityFunctionErrorResult] = adaptercore.QuerySupport(query, modelinvoker.SupportCompatible, "Messages preserves tool_result is_error")
		} else {
			contract[modelinvoker.CapabilityFunctionErrorResult] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "Chat preserves result text but not the portable is_error marker")
		}
		return contract, nil
	}
}

func approvedModel(allowedModels []string, model string) bool {
	for _, allowed := range allowedModels {
		if model == allowed {
			return true
		}
	}
	return false
}

func (dialect planDialect) String() string {
	return fmt.Sprintf("%s/%s", dialect.kind, dialect.protocol)
}

var _ protocol.Dialect = planDialect{}
