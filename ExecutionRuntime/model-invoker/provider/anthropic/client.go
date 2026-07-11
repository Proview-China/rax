package anthropic

import (
	"context"
	"net/http"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/anthropicmessages"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
)

type sdkClient struct {
	messages anthropicsdk.MessageService
}

func newSDKClient(config Config) anthropicmessages.Client {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	httpClient := adaptercore.CloneHTTPClientWithoutRedirects(config.HTTPClient)
	httpClient = adaptercore.CloneHTTPClientWithResponseCapture(httpClient)
	opts := []option.RequestOption{
		option.WithoutEnvironmentDefaults(),
		option.WithBaseURL(baseURL),
		option.WithAPIKey(config.APIKey),
		option.WithMaxRetries(0),
		option.WithHTTPClient(httpClient),
	}

	client := anthropicsdk.NewClient(opts...)
	return &sdkClient{messages: client.Messages}
}

func (c *sdkClient) Create(ctx context.Context, params anthropicsdk.MessageNewParams) (*anthropicsdk.Message, http.Header, error) {
	var raw *http.Response
	message, err := c.messages.New(ctx, params, option.WithResponseInto(&raw))
	return message, adaptercore.ResponseHeaders(raw), err
}

func (c *sdkClient) Stream(ctx context.Context, params anthropicsdk.MessageNewParams) (anthropicmessages.EventStream, http.Header) {
	var raw *http.Response
	stream := c.messages.NewStreaming(ctx, params, option.WithResponseInto(&raw))
	return stream, adaptercore.ResponseHeaders(raw)
}

var (
	_ anthropicmessages.EventStream = (*ssestream.Stream[anthropicsdk.MessageStreamEventUnion])(nil)
	_ anthropicmessages.Client      = (*sdkClient)(nil)
)
