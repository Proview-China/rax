package localcompat

import (
	"context"
	"net/http"
	"slices"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/compatprovider"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
)

type localDialect struct {
	provider modelinvoker.ProviderID
	protocol modelinvoker.Protocol
	models   []string
}

func (d localDialect) ValidateRequest(request modelinvoker.Request) error {
	if request.Provider != d.provider || request.Protocol != d.protocol {
		return localError(d.provider, modelinvoker.ErrorInvalidRequest, "validate", "request does not match local compatible binding")
	}
	if !slices.Contains(d.models, request.Model) {
		return localError(d.provider, modelinvoker.ErrorMapping, "validate", "model is outside local endpoint exact allowlist")
	}
	if len(request.ProviderOptions) != 0 {
		return localError(d.provider, modelinvoker.ErrorMapping, "validate", "local compatible route does not accept provider-specific options")
	}
	return nil
}

func (d localDialect) ClassifyFailure(failure protocol.Failure) protocol.ErrorClassification {
	kind, retryable := modelinvoker.ErrorProvider, false
	switch failure.HTTPStatus {
	case 400, 405, 409, 413, 415, 422:
		kind = modelinvoker.ErrorInvalidRequest
	case 401:
		kind = modelinvoker.ErrorAuthentication
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
	return protocol.ErrorClassification{Kind: kind, Code: failure.Code, Message: "local compatible operation failed", Retryable: retryable, RetryAfter: failure.RetryAfter}
}

func (localDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	out := modelinvoker.ProviderMetadata{}
	for name, values := range headers {
		lower := strings.ToLower(name)
		if lower == "x-request-id" || lower == "request-id" || lower == "retry-after" || strings.HasPrefix(lower, "x-ratelimit-") {
			out[lower] = strings.Join(values, ",")
		}
	}
	return out
}

func capabilityContract(provider modelinvoker.ProviderID, protocolID modelinvoker.Protocol, models []string, capabilities []modelinvoker.Capability) compatprovider.CapabilityBuilder {
	modelAllowlist := append([]string(nil), models...)
	declared := append([]modelinvoker.Capability(nil), capabilities...)
	return func(_ context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
		if query.Protocol != protocolID || !slices.Contains(modelAllowlist, query.Model) {
			return nil, localError(provider, modelinvoker.ErrorMapping, "capabilities", "query is outside local compatible binding")
		}
		contract := adaptercore.UnsupportedContract("not declared by the self-hosted endpoint administrator")
		adaptercore.SetSupport(contract, query, modelinvoker.SupportCompatible, "explicitly attested by self-hosted endpoint configuration", declared...)
		return contract, nil
	}
}

var _ protocol.Dialect = localDialect{}
