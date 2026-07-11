package bedrockmantle

import (
	"context"
	"net/http"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/anthropicmessages"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openairesponses"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	anthropicbedrock "github.com/anthropics/anthropic-sdk-go/bedrock"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	awsv4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	openaisdk "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

type openAIClient struct {
	responses responses.ResponseService
	chat      openaisdk.ChatCompletionService
	store     bool
}
type responsesClient struct{ native *openAIClient }
type chatClient struct{ native *openAIClient }
type messagesClient struct{ messages anthropicsdk.MessageService }

func newSDKClients(config Config) (openaichat.Client, openairesponses.Client, anthropicmessages.Client, error) {
	key, credentials, err := credentialSources(config)
	if err != nil {
		return nil, nil, nil, err
	}
	httpClient := adaptercore.CloneHTTPClientWithoutRedirects(config.HTTPClient)
	next := httpClient.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	httpClient.Transport = authTransport{next: next, region: config.Region, apiKey: key, credentials: credentials, signer: awsv4.NewSigner()}
	httpClient = adaptercore.CloneHTTPClientWithResponseCapture(httpClient)
	root := strings.TrimRight(config.rootEndpoint(), "/")
	openAIOpts := []openaioption.RequestOption{openaioption.WithBaseURL(root + "/openai/v1/"), openaioption.WithAPIKey("praxis-bedrock-managed"), openaioption.WithMaxRetries(0), openaioption.WithHTTPClient(httpClient)}
	openAI := &openAIClient{responses: responses.NewResponseService(openAIOpts...), chat: openaisdk.NewChatCompletionService(openAIOpts...), store: config.storesResponses()}
	mantle, err := anthropicbedrock.NewMantleClient(context.Background(), anthropicbedrock.MantleClientConfig{AWSRegion: config.Region, BaseURL: root + "/anthropic", SkipAuth: true}, anthropicoption.WithoutEnvironmentDefaults(), anthropicoption.WithAPIKey(""), anthropicoption.WithMaxRetries(0), anthropicoption.WithHTTPClient(httpClient))
	if err != nil {
		return nil, nil, nil, err
	}
	return chatClient{native: openAI}, responsesClient{native: openAI}, messagesClient{messages: mantle.Messages}, nil
}

func (c responsesClient) Create(ctx context.Context, params responses.ResponseNewParams) (*responses.Response, http.Header, error) {
	params.Store = param.NewOpt(c.native.store)
	var raw *http.Response
	value, err := c.native.responses.New(ctx, params, openaioption.WithResponseInto(&raw))
	return value, adaptercore.ResponseHeaders(raw), err
}
func (c responsesClient) Stream(ctx context.Context, params responses.ResponseNewParams) (openairesponses.EventStream, http.Header) {
	params.Store = param.NewOpt(c.native.store)
	var raw *http.Response
	stream := c.native.responses.NewStreaming(ctx, params, openaioption.WithResponseInto(&raw))
	return stream, adaptercore.ResponseHeaders(raw)
}
func (c chatClient) Create(ctx context.Context, params openaisdk.ChatCompletionNewParams) (*openaisdk.ChatCompletion, http.Header, error) {
	var raw *http.Response
	value, err := c.native.chat.New(ctx, params, openaioption.WithResponseInto(&raw))
	return value, adaptercore.ResponseHeaders(raw), err
}
func (c chatClient) Stream(ctx context.Context, params openaisdk.ChatCompletionNewParams) (openaichat.EventStream, http.Header) {
	var raw *http.Response
	stream := c.native.chat.NewStreaming(ctx, params, openaioption.WithResponseInto(&raw))
	return stream, adaptercore.ResponseHeaders(raw)
}
func (c messagesClient) Create(ctx context.Context, params anthropicsdk.MessageNewParams) (*anthropicsdk.Message, http.Header, error) {
	var raw *http.Response
	value, err := c.messages.New(ctx, params, anthropicoption.WithResponseInto(&raw))
	return value, adaptercore.ResponseHeaders(raw), err
}
func (c messagesClient) Stream(ctx context.Context, params anthropicsdk.MessageNewParams) (anthropicmessages.EventStream, http.Header) {
	var raw *http.Response
	stream := c.messages.NewStreaming(ctx, params, anthropicoption.WithResponseInto(&raw))
	return stream, adaptercore.ResponseHeaders(raw)
}

var _ openaichat.Client = chatClient{}
var _ openairesponses.Client = responsesClient{}
var _ anthropicmessages.Client = messagesClient{}
