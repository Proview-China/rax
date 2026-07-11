package anthropic

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
)

var nativeToolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type messagesDialect struct{}

func (messagesDialect) ValidateRequest(request modelinvoker.Request) error {
	if request.State != nil && request.State.Kind != modelinvoker.StateProviderContinuation {
		return mappingError("validate", "Anthropic Messages accepts only provider continuation state")
	}
	if request.Budget.MaxOutputTokens <= 0 {
		return mappingError("validate", "Anthropic Messages requires max output tokens greater than zero")
	}
	for index, tool := range request.Tools {
		if !nativeToolNamePattern.MatchString(tool.Name) {
			return mappingError("validate", fmt.Sprintf("tool %d name is not valid for Anthropic", index))
		}
	}
	if request.ToolChoice.Mode == modelinvoker.ToolChoiceFunction && !nativeToolNamePattern.MatchString(request.ToolChoice.Name) {
		return mappingError("validate", "function tool choice name is not valid for Anthropic")
	}
	for index, item := range request.Input {
		if item.Type == modelinvoker.InputTypeFunctionCall {
			if item.FunctionCall == nil || strings.TrimSpace(item.FunctionCall.ID) == "" || !nativeToolNamePattern.MatchString(item.FunctionCall.Name) {
				return mappingError("validate", fmt.Sprintf("input %d Anthropic function call requires an ID and valid name", index))
			}
		}
		if item.Type == modelinvoker.InputTypeFunctionResult && item.FunctionResult != nil &&
			item.FunctionResult.Name != "" && !nativeToolNamePattern.MatchString(item.FunctionResult.Name) {
			return mappingError("validate", fmt.Sprintf("input %d function result name is not valid for Anthropic", index))
		}
	}
	return validateProviderOptions(request)
}

func (messagesDialect) ClassifyFailure(failure protocol.Failure) protocol.ErrorClassification {
	kind, retryable := classifyAPIError(failure.HTTPStatus, failure.Type)
	if failure.Source == protocol.FailureSourceTransport {
		kind = modelinvoker.ErrorProviderUnavailable
		retryable = true
	}
	if failure.Source == protocol.FailureSourceProtocol {
		kind = modelinvoker.ErrorProvider
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

func (messagesDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	metadata := make(modelinvoker.ProviderMetadata)
	for key, values := range headers {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "anthropic-ratelimit-") {
			metadata[lower] = strings.Join(values, ",")
		}
	}
	return metadata
}

var _ protocol.Dialect = messagesDialect{}
