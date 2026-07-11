package anthropicmessages

import (
	"encoding/json"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

// RequestMapper lets a provider-owned compatibility dialect remove or
// transform Anthropic fields that the provider documents differently.
type RequestMapper interface {
	MapMessagesRequest(modelinvoker.Request, anthropicsdk.MessageNewParams) (anthropicsdk.MessageNewParams, []modelinvoker.MappingDecision, error)
}

// ContinuationMapper canonicalizes provider-emitted content blocks before the
// strict portable continuation envelope is built. It never runs on caller-
// supplied State, which remains subject to the normal fail-closed validation.
type ContinuationMapper interface {
	MapMessagesContinuation(modelinvoker.Request, []json.RawMessage) ([]json.RawMessage, error)
}

type StopReasonMapping struct {
	Status     modelinvoker.ResponseStatus
	StopReason modelinvoker.StopReason
	Error      error
}

// StopReasonMapper owns provider-documented Messages terminal reasons that
// are not part of Anthropic's native stop-reason vocabulary.
type StopReasonMapper interface {
	MapMessagesStopReason(modelinvoker.Request, string) (StopReasonMapping, bool)
}
