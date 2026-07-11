package openaichat

import (
	"context"
	"net/http"

	openaisdk "github.com/openai/openai-go/v3"
)

// EventStream is the narrow SDK stream seam used by the protocol driver.
type EventStream interface {
	Next() bool
	Current() openaisdk.ChatCompletionChunk
	Err() error
	Close() error
}

// Client is the narrow SDK seam used by the protocol driver. It is confined
// to this internal concrete protocol package and cannot cross the public API.
type Client interface {
	Create(context.Context, openaisdk.ChatCompletionNewParams) (*openaisdk.ChatCompletion, http.Header, error)
	Stream(context.Context, openaisdk.ChatCompletionNewParams) (EventStream, http.Header)
}
