package openairesponses

import (
	"context"
	"net/http"

	"github.com/openai/openai-go/v3/responses"
)

type EventStream interface {
	Next() bool
	Current() responses.ResponseStreamEventUnion
	Err() error
	Close() error
}

type Client interface {
	Create(context.Context, responses.ResponseNewParams) (*responses.Response, http.Header, error)
	Stream(context.Context, responses.ResponseNewParams) (EventStream, http.Header)
}
