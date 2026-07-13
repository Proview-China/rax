package relaycompat

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

type relayDialect struct {
	protocol      modelinvoker.Protocol
	allowedModels []string
}

func (dialect relayDialect) ValidateRequest(request modelinvoker.Request) error {
	if request.Protocol != dialect.protocol {
		return providerError(modelinvoker.ErrorInvalidRequest, "validate", "request protocol does not match the relay Route")
	}
	if !allowedModel(dialect.allowedModels, request.Model) {
		return providerError(modelinvoker.ErrorMapping, "validate", "model is outside the relay Route exact allowlist")
	}
	if len(request.ProviderOptions) != 0 {
		return providerError(modelinvoker.ErrorMapping, "validate", "third-party relay routes do not accept provider-specific options")
	}
	return nil
}

func (relayDialect) ClassifyFailure(failure protocol.Failure) protocol.ErrorClassification {
	kind, retryable := modelinvoker.ErrorProvider, false
	switch failure.HTTPStatus {
	case 400, 422:
		kind = modelinvoker.ErrorInvalidRequest
	case 401:
		kind = modelinvoker.ErrorAuthentication
	case 402:
		kind = modelinvoker.ErrorBilling
	case 403, 404:
		kind = modelinvoker.ErrorPermission
	case 408:
		kind, retryable = modelinvoker.ErrorTimeout, true
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
	return protocol.ErrorClassification{Kind: kind, Code: code, Message: "third-party relay operation failed", Retryable: retryable, RetryAfter: failure.RetryAfter}
}

func (relayDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	metadata := modelinvoker.ProviderMetadata{}
	for name, values := range headers {
		lower := strings.ToLower(name)
		if lower == "x-request-id" || lower == "request-id" || lower == "x-goog-request-id" || lower == "cf-ray" || lower == "retry-after" || strings.HasPrefix(lower, "x-ratelimit-") || strings.HasPrefix(lower, "x-goog-quota-") {
			metadata[lower] = strings.Join(values, ",")
		}
	}
	return metadata
}

func capabilityContract(protocolID modelinvoker.Protocol, allowedModels []string) compatprovider.CapabilityBuilder {
	models := append([]string(nil), allowedModels...)
	return func(_ context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
		if query.Protocol != protocolID {
			return nil, providerError(modelinvoker.ErrorInvalidRequest, "capabilities", "capability protocol does not match the relay Route")
		}
		if !allowedModel(models, query.Model) {
			return nil, providerError(modelinvoker.ErrorMapping, "capabilities", "model is outside the relay Route exact allowlist")
		}
		contract := adaptercore.UnsupportedContract("not declared by the selected third-party relay compatibility Route")
		adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible,
			"normalized through an explicitly configured third-party relay protocol",
			modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming,
			modelinvoker.CapabilityToolCalling, modelinvoker.CapabilityUsageReporting,
		)
		switch query.Protocol {
		case modelinvoker.ProtocolResponses:
			adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible, "Responses continuation is preserved when the relay returns it", modelinvoker.CapabilityServerState)
		case modelinvoker.ProtocolMessages, modelinvoker.ProtocolGenerateContent:
			adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible, "protocol-native tool errors are normalized", modelinvoker.CapabilityFunctionErrorResult)
		case modelinvoker.ProtocolChatCompletions:
			contract[modelinvoker.CapabilityFunctionErrorResult] = adaptercore.QuerySupport(query, modelinvoker.SupportPartial, "Chat Completions preserves tool result text but not a portable is_error marker")
		}
		return contract, nil
	}
}

func allowedModel(models []string, target string) bool {
	for _, model := range models {
		if model == target {
			return true
		}
	}
	return false
}

func (dialect relayDialect) String() string {
	return fmt.Sprintf("third-party-relay/%s", dialect.protocol)
}

var _ protocol.Dialect = relayDialect{}
