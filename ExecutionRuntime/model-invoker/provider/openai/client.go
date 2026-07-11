package openai

import (
	"context"
	"net/http"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openairesponses"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
)

type responseEventStream interface {
	Next() bool
	Current() responses.ResponseStreamEventUnion
	Err() error
	Close() error
}

type chatEventStream interface {
	Next() bool
	Current() openaisdk.ChatCompletionChunk
	Err() error
	Close() error
}

type nativeClient interface {
	createResponse(context.Context, responses.ResponseNewParams) (*responses.Response, http.Header, error)
	streamResponse(context.Context, responses.ResponseNewParams) (responseEventStream, http.Header)
	createChatCompletion(context.Context, openaisdk.ChatCompletionNewParams) (*openaisdk.ChatCompletion, http.Header, error)
	streamChatCompletion(context.Context, openaisdk.ChatCompletionNewParams) (chatEventStream, http.Header)
}

type sdkClient struct {
	responses responses.ResponseService
	chat      openaisdk.ChatCompletionService
}

type chatDriverClient struct {
	native nativeClient
}

type responsesDriverClient struct {
	native nativeClient
}

func (c responsesDriverClient) Create(ctx context.Context, params responses.ResponseNewParams) (*responses.Response, http.Header, error) {
	return c.native.createResponse(ctx, params)
}

func (c responsesDriverClient) Stream(ctx context.Context, params responses.ResponseNewParams) (openairesponses.EventStream, http.Header) {
	stream, headers := c.native.streamResponse(ctx, params)
	return stream, headers
}

func (c chatDriverClient) Create(ctx context.Context, params openaisdk.ChatCompletionNewParams) (*openaisdk.ChatCompletion, http.Header, error) {
	return c.native.createChatCompletion(ctx, params)
}

func (c chatDriverClient) Stream(ctx context.Context, params openaisdk.ChatCompletionNewParams) (openaichat.EventStream, http.Header) {
	stream, headers := c.native.streamChatCompletion(ctx, params)
	return stream, headers
}

func newSDKClient(config Config) nativeClient {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL + "/"
	}
	httpClient := adaptercore.CloneHTTPClientWithoutRedirects(config.HTTPClient)
	httpClient = adaptercore.CloneHTTPClientWithResponseCapture(httpClient)
	opts := []option.RequestOption{
		option.WithBaseURL(baseURL),
		option.WithAPIKey(config.APIKey),
		option.WithMaxRetries(0),
		option.WithHTTPClient(httpClient),
	}
	return &sdkClient{
		responses: responses.NewResponseService(opts...),
		chat:      openaisdk.NewChatCompletionService(opts...),
	}
}

func (c *sdkClient) createResponse(ctx context.Context, params responses.ResponseNewParams) (*responses.Response, http.Header, error) {
	var raw *http.Response
	response, err := c.responses.New(ctx, params, option.WithResponseInto(&raw))
	return response, adaptercore.ResponseHeaders(raw), err
}

func (c *sdkClient) streamResponse(ctx context.Context, params responses.ResponseNewParams) (responseEventStream, http.Header) {
	var raw *http.Response
	stream := c.responses.NewStreaming(ctx, params, option.WithResponseInto(&raw))
	return stream, adaptercore.ResponseHeaders(raw)
}

func (c *sdkClient) createChatCompletion(ctx context.Context, params openaisdk.ChatCompletionNewParams) (*openaisdk.ChatCompletion, http.Header, error) {
	var raw *http.Response
	response, err := c.chat.New(ctx, params, option.WithResponseInto(&raw))
	return response, adaptercore.ResponseHeaders(raw), err
}

func (c *sdkClient) streamChatCompletion(ctx context.Context, params openaisdk.ChatCompletionNewParams) (chatEventStream, http.Header) {
	var raw *http.Response
	stream := c.chat.NewStreaming(ctx, params, option.WithResponseInto(&raw))
	return stream, adaptercore.ResponseHeaders(raw)
}

var (
	_ responseEventStream    = (*ssestream.Stream[responses.ResponseStreamEventUnion])(nil)
	_ chatEventStream        = (*ssestream.Stream[openaisdk.ChatCompletionChunk])(nil)
	_ nativeClient           = (*sdkClient)(nil)
	_ openaichat.Client      = chatDriverClient{}
	_ openairesponses.Client = responsesDriverClient{}
)
