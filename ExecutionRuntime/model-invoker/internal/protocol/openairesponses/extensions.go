package openairesponses

import (
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/openai/openai-go/v3/responses"
)

// RequestMapper lets a provider-owned Responses compatibility dialect map
// documented fields whose semantics differ from OpenAI's native service.
type RequestMapper interface {
	MapResponsesRequest(modelinvoker.Request, responses.ResponseNewParams) (responses.ResponseNewParams, []modelinvoker.MappingDecision, error)
}

// ResponseMapper preserves provider-owned Responses semantics that differ
// from OpenAI, such as an explicitly stateless store=false contract.
type ResponseMapper interface {
	MapResponsesResponse(modelinvoker.Request, *responses.Response, *modelinvoker.Response) error
}
