package anthropicmessages

import (
	"context"
	"net/http"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

type EventStream interface {
	Next() bool
	Current() anthropicsdk.MessageStreamEventUnion
	Err() error
	Close() error
}

type Client interface {
	Create(context.Context, anthropicsdk.MessageNewParams) (*anthropicsdk.Message, http.Header, error)
	Stream(context.Context, anthropicsdk.MessageNewParams) (EventStream, http.Header)
}
