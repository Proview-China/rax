package openaichat

import (
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	openaisdk "github.com/openai/openai-go/v3"
)

// RequestMapper is an optional provider-dialect seam for fields that are part
// of an officially documented Chat Completions compatibility contract but not
// represented by the upstream OpenAI SDK parameter type.
type RequestMapper interface {
	MapChatRequest(modelinvoker.Request, openaisdk.ChatCompletionNewParams) (openaisdk.ChatCompletionNewParams, []modelinvoker.MappingDecision, error)
}

// ResponseMapper preserves documented provider extensions after the portable
// Chat Completions response has been normalized.
type ResponseMapper interface {
	MapChatResponse(modelinvoker.Request, *openaisdk.ChatCompletion, *modelinvoker.Response) error
}

// StreamMapper preserves documented provider extensions from each selected
// Chat Completions choice. It returns a portable reasoning delta when present.
type StreamMapper interface {
	MapChatChunk(modelinvoker.Request, openaisdk.ChatCompletionChunkChoice) (string, []modelinvoker.MappingDecision, error)
}

// StreamDeltaMapper handles compatibility APIs whose Chat fields are
// cumulative snapshots rather than ordinary deltas. Previous portable text
// and reasoning are passed per stream, so provider adapters remain stateless
// and safe for concurrent use.
type StreamDeltaMapper interface {
	MapChatStreamDelta(modelinvoker.Request, openaisdk.ChatCompletionChunkChoice, string, string) (string, string, []modelinvoker.MappingDecision, error)
}

type StreamMetadata struct {
	RequestID        string
	ProviderMetadata modelinvoker.ProviderMetadata
}

// StreamMetadataMapper preserves provider-documented stream envelope metadata
// such as a request ID carried in JSON rather than an HTTP header.
type StreamMetadataMapper interface {
	MapChatStreamMetadata(modelinvoker.Request, openaisdk.ChatCompletionChunk) (StreamMetadata, error)
}

type FinishReasonMapping struct {
	Status     modelinvoker.ResponseStatus
	StopReason modelinvoker.StopReason
	Error      error
}

// FinishReasonMapper owns provider-documented terminal reasons that are not
// part of the portable OpenAI Chat Completions finish-reason vocabulary.
type FinishReasonMapper interface {
	MapChatFinishReason(modelinvoker.Request, string) (FinishReasonMapping, bool)
}
