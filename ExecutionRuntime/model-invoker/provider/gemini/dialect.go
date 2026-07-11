package gemini

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
)

var nativeToolNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.:-]{0,127}$`)

type generateDialect struct{}

func (generateDialect) ValidateRequest(request modelinvoker.Request) error {
	if request.State != nil && request.State.Kind != modelinvoker.StateProviderContinuation {
		return mappingError("validate", "Gemini GenerateContent accepts only provider continuation state")
	}
	if roleError := containsConversationOnlyRoleError(request); roleError != "" {
		return mappingError("validate", roleError)
	}
	for index, tool := range request.Tools {
		if !nativeToolNamePattern.MatchString(tool.Name) {
			return mappingError("validate", fmt.Sprintf("tool %d name is not valid for Gemini", index))
		}
	}
	if request.ToolChoice.Mode == modelinvoker.ToolChoiceFunction && !nativeToolNamePattern.MatchString(request.ToolChoice.Name) {
		return mappingError("validate", "function tool choice name is not valid for Gemini")
	}
	for index, input := range request.Input {
		switch input.Type {
		case modelinvoker.InputTypeFunctionCall:
			if input.FunctionCall != nil && !nativeToolNamePattern.MatchString(input.FunctionCall.Name) {
				return mappingError("validate", fmt.Sprintf("input %d function call name is not valid for Gemini", index))
			}
		case modelinvoker.InputTypeFunctionResult:
			if input.FunctionResult != nil && input.FunctionResult.Name != "" && !nativeToolNamePattern.MatchString(input.FunctionResult.Name) {
				return mappingError("validate", fmt.Sprintf("input %d function result name is not valid for Gemini", index))
			}
		}
	}
	for namespace, raw := range request.ProviderOptions {
		if namespace != ProviderID {
			return mappingError("validate", fmt.Sprintf("provider options namespace %q cannot be consumed by Gemini", namespace))
		}
		if !optionsAreEmpty(raw) {
			return mappingError("validate", "Gemini provider options are not defined for the GenerateContent slice")
		}
	}
	return nil
}

func (generateDialect) ClassifyFailure(failure protocol.Failure) protocol.ErrorClassification {
	kind, retryable := classifyAPIError(failure.HTTPStatus, failure.Type)
	for _, signal := range failure.Signals {
		if signal.Key == "billing" && signal.Value == "true" {
			kind, retryable = modelinvoker.ErrorBilling, false
		}
	}
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

func (generateDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	metadata := make(modelinvoker.ProviderMetadata)
	for key, values := range headers {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "x-ratelimit-") || strings.HasPrefix(lower, "x-goog-quota-") {
			metadata[lower] = strings.Join(values, ",")
		}
	}
	return metadata
}

func optionsAreEmpty(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && len(object) == 0
}

func containsConversationOnlyRoleError(request modelinvoker.Request) string {
	for _, item := range request.Input {
		if item.Type != modelinvoker.InputTypeMessage || item.Message == nil {
			continue
		}
		if item.Message.Role == modelinvoker.RoleSystem || item.Message.Role == modelinvoker.RoleDeveloper {
			return fmt.Sprintf("message role %q is not supported in Gemini contents; use Request.Instructions", item.Message.Role)
		}
	}
	return ""
}

var _ protocol.Dialect = generateDialect{}
