package compatprovider

import (
	"context"
	"net/http"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/anthropicmessages"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openairesponses"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	openaisdk "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

type openAIClient struct {
	chat      openaisdk.ChatCompletionService
	responses responses.ResponseService
}

func newOpenAIClient(apiKey, endpoint string, client *http.Client) *openAIClient {
	httpClient := adaptercore.CloneHTTPClientWithoutRedirects(client)
	httpClient = adaptercore.CloneHTTPClientWithResponseCapture(httpClient)
	opts := []openaioption.RequestOption{
		openaioption.WithBaseURL(endpoint),
		openaioption.WithAPIKey(apiKey),
		openaioption.WithMaxRetries(0),
		openaioption.WithHTTPClient(httpClient),
	}
	return &openAIClient{
		chat:      openaisdk.NewChatCompletionService(opts...),
		responses: responses.NewResponseService(opts...),
	}
}

func (c *openAIClient) Create(ctx context.Context, params openaisdk.ChatCompletionNewParams) (*openaisdk.ChatCompletion, http.Header, error) {
	var raw *http.Response
	result, err := c.chat.New(ctx, params, openaioption.WithResponseInto(&raw))
	return result, adaptercore.ResponseHeaders(raw), err
}

func (c *openAIClient) Stream(ctx context.Context, params openaisdk.ChatCompletionNewParams) (openaichat.EventStream, http.Header) {
	var raw *http.Response
	stream := c.chat.NewStreaming(ctx, params, openaioption.WithResponseInto(&raw))
	return stream, adaptercore.ResponseHeaders(raw)
}

func (c *openAIClient) CreateResponse(ctx context.Context, params responses.ResponseNewParams) (*responses.Response, http.Header, error) {
	var raw *http.Response
	result, err := c.responses.New(ctx, params, openaioption.WithResponseInto(&raw))
	return result, adaptercore.ResponseHeaders(raw), err
}

func (c *openAIClient) StreamResponse(ctx context.Context, params responses.ResponseNewParams) (openairesponses.EventStream, http.Header) {
	var raw *http.Response
	stream := c.responses.NewStreaming(ctx, params, openaioption.WithResponseInto(&raw))
	return stream, adaptercore.ResponseHeaders(raw)
}

type responsesClient struct{ native *openAIClient }

func (c responsesClient) Create(ctx context.Context, params responses.ResponseNewParams) (*responses.Response, http.Header, error) {
	return c.native.CreateResponse(ctx, params)
}
func (c responsesClient) Stream(ctx context.Context, params responses.ResponseNewParams) (openairesponses.EventStream, http.Header) {
	return c.native.StreamResponse(ctx, params)
}

type anthropicClient struct{ messages anthropicsdk.MessageService }

func newAnthropicClient(apiKey, endpoint string, client *http.Client, useAuthToken bool) *anthropicClient {
	httpClient := adaptercore.CloneHTTPClientWithoutRedirects(client)
	httpClient = adaptercore.CloneHTTPClientWithResponseCapture(httpClient)
	opts := []anthropicoption.RequestOption{
		anthropicoption.WithoutEnvironmentDefaults(),
		anthropicoption.WithBaseURL(endpoint),
		anthropicoption.WithMaxRetries(0),
		anthropicoption.WithHTTPClient(httpClient),
	}
	if useAuthToken {
		opts = append(opts, anthropicoption.WithAuthToken(apiKey))
	} else {
		opts = append(opts, anthropicoption.WithAPIKey(apiKey))
	}
	clientValue := anthropicsdk.NewClient(opts...)
	return &anthropicClient{messages: clientValue.Messages}
}

func (c *anthropicClient) Create(ctx context.Context, params anthropicsdk.MessageNewParams) (*anthropicsdk.Message, http.Header, error) {
	var raw *http.Response
	result, err := c.messages.New(ctx, params, anthropicoption.WithResponseInto(&raw))
	return result, adaptercore.ResponseHeaders(raw), err
}

func (c *anthropicClient) Stream(ctx context.Context, params anthropicsdk.MessageNewParams) (anthropicmessages.EventStream, http.Header) {
	var raw *http.Response
	stream := c.messages.NewStreaming(ctx, params, anthropicoption.WithResponseInto(&raw))
	return stream, adaptercore.ResponseHeaders(raw)
}

var (
	_ openaichat.Client        = (*openAIClient)(nil)
	_ openairesponses.Client   = responsesClient{}
	_ anthropicmessages.Client = (*anthropicClient)(nil)
)
