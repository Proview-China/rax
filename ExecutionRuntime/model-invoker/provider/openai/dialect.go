package openai

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
)

var nativeToolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type chatDialect struct{}

type responsesDialect struct{ chatDialect }

func (responsesDialect) ValidateRequest(request modelinvoker.Request) error {
	if request.State != nil && request.State.Kind != modelinvoker.StateServerContinuation {
		return mappingError("validate", "Responses requires server continuation state")
	}
	if err := validateOpenAIRequestSemantics(request); err != nil {
		return err
	}
	for _, options := range request.ProviderOptions {
		var object map[string]json.RawMessage
		if json.Unmarshal(options, &object) != nil || len(object) != 0 {
			return providerError(modelinvoker.ErrorMapping, "validate", "provider options are not defined in the current implementation slice", nil)
		}
	}
	return nil
}

func (chatDialect) ValidateRequest(request modelinvoker.Request) error {
	if request.State != nil {
		return mappingError("validate", "server continuation state is not supported by Chat Completions")
	}
	if err := validateOpenAIRequestSemantics(request); err != nil {
		return err
	}
	for _, options := range request.ProviderOptions {
		var object map[string]json.RawMessage
		if json.Unmarshal(options, &object) != nil || len(object) != 0 {
			return providerError(modelinvoker.ErrorMapping, "validate", "provider options are not defined in the current implementation slice", nil)
		}
	}
	return nil
}

func (chatDialect) ClassifyFailure(failure protocol.Failure) protocol.ErrorClassification {
	kind, retryable := classifyAPIError(failure.HTTPStatus, failure.Type, failure.Code)
	if failure.Source == protocol.FailureSourceTransport {
		kind = modelinvoker.ErrorProviderUnavailable
		retryable = true
	}
	if failure.Source == protocol.FailureSourceProtocol {
		kind = modelinvoker.ErrorProvider
		retryable = false
	}
	if failure.Source == protocol.FailureSourceStream && failure.Code == "invalid_stream_error" {
		kind = modelinvoker.ErrorStreamInterrupted
		retryable = false
	}
	message := failure.Message
	if message == "" {
		message = "provider operation failed"
	}
	code := failure.Code
	if code == "" {
		code = failure.Type
	}
	return protocol.ErrorClassification{
		Kind: kind, Code: code, Message: message, Retryable: retryable, RetryAfter: failure.RetryAfter,
	}
}

func (chatDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	return headerMetadata(headers)
}

func headerMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	metadata := make(modelinvoker.ProviderMetadata)
	for key, values := range headers {
		lower := strings.ToLower(key)
		if lower == "openai-processing-ms" || lower == "openai-version" || strings.HasPrefix(lower, "x-ratelimit-") {
			metadata[lower] = strings.Join(values, ",")
		}
	}
	return metadata
}

var _ protocol.Dialect = chatDialect{}
var _ protocol.Dialect = responsesDialect{}
