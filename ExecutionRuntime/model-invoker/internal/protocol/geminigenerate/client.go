package geminigenerate

import (
	"context"
	"net/http"

	"google.golang.org/genai"
)

type EventStream interface {
	Next() bool
	Current() *genai.GenerateContentResponse
	Err() error
	Close() error
}

type Client interface {
	GenerateContent(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, http.Header, error)
	GenerateContentStream(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (EventStream, http.Header, error)
}
