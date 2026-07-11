package bedrockruntime

import (
	"net/http"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
)

type dialect struct{}

func (dialect) ValidateRequest(request modelinvoker.Request) error {
	if request.Protocol == modelinvoker.ProtocolBedrockConverse && request.State != nil {
		return providerError(modelinvoker.ErrorMapping, "validate", "Bedrock Converse continuation state is unsupported")
	}
	return nil
}
func (dialect) ClassifyFailure(failure protocol.Failure) protocol.ErrorClassification {
	kind, retryable := modelinvoker.ErrorProvider, false
	switch failure.HTTPStatus {
	case 401:
		kind = modelinvoker.ErrorAuthentication
	case 403:
		kind = modelinvoker.ErrorPermission
	case 408:
		kind, retryable = modelinvoker.ErrorTimeout, true
	case 429:
		kind, retryable = modelinvoker.ErrorRateLimit, true
	case 500, 502, 503, 504:
		kind, retryable = modelinvoker.ErrorProviderUnavailable, true
	}
	return protocol.ErrorClassification{Kind: kind, Code: failure.Code, Message: "Bedrock operation failed", Retryable: retryable, RetryAfter: failure.RetryAfter}
}
func (dialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	metadata := make(modelinvoker.ProviderMetadata)
	for key, values := range headers {
		lower := strings.ToLower(key)
		if lower == "x-amzn-requestid" || strings.HasPrefix(lower, "x-amzn-bedrock-") {
			metadata[lower] = strings.Join(values, ",")
		}
	}
	return metadata
}

var _ protocol.Dialect = dialect{}
