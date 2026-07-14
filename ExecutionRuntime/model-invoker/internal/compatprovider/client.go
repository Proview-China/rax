package compatprovider

import (
	"context"
	"fmt"
	"iter"
	"net/http"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/anthropicmessages"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/geminigenerate"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openairesponses"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	openaisdk "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"google.golang.org/genai"
)

type openAIClient struct {
	chat      openaisdk.ChatCompletionService
	responses responses.ResponseService
}

func newOpenAIClient(apiKey, endpoint string, client *http.Client, userAgent string, allowAnonymous bool) *openAIClient {
	httpClient := adaptercore.CloneHTTPClientWithoutRedirects(client)
	httpClient = withUserAgent(httpClient, userAgent)
	httpClient = withPinnedOpenAIAuth(httpClient, apiKey, allowAnonymous)
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

type authStrippingTransport struct {
	next   http.RoundTripper
	apiKey string
}

func (transport authStrippingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	for _, name := range []string{"Authorization", "Proxy-Authorization", "Cookie", "X-API-Key", "Api-Key", "OpenAI-Organization", "OpenAI-Project"} {
		clone.Header.Del(name)
	}
	if transport.apiKey != "" {
		clone.Header.Set("Authorization", "Bearer "+transport.apiKey)
	}
	return transport.next.RoundTrip(clone)
}

func withPinnedOpenAIAuth(client *http.Client, apiKey string, allowAnonymous bool) *http.Client {
	if apiKey == "" && !allowAnonymous {
		return client
	}
	next := client.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	client.Transport = authStrippingTransport{next: next, apiKey: apiKey}
	return client
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

func newAnthropicClient(apiKey, endpoint string, client *http.Client, useAuthToken bool, userAgent string) *anthropicClient {
	httpClient := adaptercore.CloneHTTPClientWithoutRedirects(client)
	httpClient = withUserAgent(httpClient, userAgent)
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

type userAgentTransport struct {
	next      http.RoundTripper
	userAgent string
}

func (transport userAgentTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("User-Agent", transport.userAgent)
	return transport.next.RoundTrip(clone)
}

func withUserAgent(client *http.Client, userAgent string) *http.Client {
	if userAgent == "" {
		return client
	}
	next := client.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	client.Transport = userAgentTransport{next: next, userAgent: userAgent}
	return client
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

type generateClient struct{ models *genai.Models }

func newGenerateClient(apiKey, baseURL, apiVersion string, client *http.Client, userAgent string) (*generateClient, error) {
	httpClient := adaptercore.CloneHTTPClientWithoutRedirects(client)
	httpClient = withUserAgent(httpClient, userAgent)
	httpClient = adaptercore.CloneHTTPClientWithResponseCapture(httpClient)
	sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: apiKey, Backend: genai.BackendGeminiAPI, HTTPClient: httpClient,
		HTTPOptions: genai.HTTPOptions{BaseURL: baseURL, APIVersion: apiVersion},
	})
	if err != nil {
		return nil, err
	}
	if sdk == nil || sdk.Models == nil {
		return nil, fmt.Errorf("GenerateContent SDK returned an uninitialized client")
	}
	return &generateClient{models: sdk.Models}, nil
}

func (c *generateClient) GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, http.Header, error) {
	response, err := c.models.GenerateContent(ctx, model, contents, config)
	var headers http.Header
	if response != nil && response.SDKHTTPResponse != nil && response.SDKHTTPResponse.Headers != nil {
		headers = response.SDKHTTPResponse.Headers.Clone()
	}
	return response, headers, err
}

func (c *generateClient) GenerateContentStream(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (geminigenerate.EventStream, http.Header, error) {
	stream := newGenerateResponseStream(c.models.GenerateContentStream(ctx, model, contents, config))
	var headers http.Header
	if current := stream.Current(); current != nil && current.SDKHTTPResponse != nil && current.SDKHTTPResponse.Headers != nil {
		headers = current.SDKHTTPResponse.Headers.Clone()
	}
	if err := stream.Err(); err != nil {
		_ = stream.Close()
		return nil, headers, err
	}
	return stream, headers, nil
}

type generateResponseStream struct {
	next       func() (*genai.GenerateContentResponse, error, bool)
	stop       func()
	current    *genai.GenerateContentResponse
	err        error
	prefetched bool
	exhausted  bool
	closed     bool
}

func newGenerateResponseStream(sequence iter.Seq2[*genai.GenerateContentResponse, error]) *generateResponseStream {
	next, stop := iter.Pull2(sequence)
	stream := &generateResponseStream{next: next, stop: stop}
	stream.prefetch()
	return stream
}

func (s *generateResponseStream) prefetch() {
	if s.closed || s.exhausted || s.prefetched || s.err != nil {
		return
	}
	response, err, ok := s.next()
	if !ok {
		s.exhausted = true
		return
	}
	if err != nil {
		s.err = err
		return
	}
	if response == nil {
		s.err = fmt.Errorf("GenerateContent SDK stream returned a nil response")
		return
	}
	s.current, s.prefetched = response, true
}

func (s *generateResponseStream) Next() bool {
	if s == nil || s.closed || s.err != nil || s.exhausted {
		return false
	}
	if s.prefetched {
		s.prefetched = false
		return true
	}
	response, err, ok := s.next()
	if !ok {
		s.exhausted = true
		return false
	}
	if err != nil {
		s.err = err
		return false
	}
	if response == nil {
		s.err = fmt.Errorf("GenerateContent SDK stream returned a nil response")
		return false
	}
	s.current = response
	return true
}

func (s *generateResponseStream) Current() *genai.GenerateContentResponse {
	if s == nil {
		return nil
	}
	return s.current
}

func (s *generateResponseStream) Err() error {
	if s == nil {
		return fmt.Errorf("GenerateContent SDK stream is nil")
	}
	return s.err
}

func (s *generateResponseStream) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	if s.stop != nil {
		s.stop()
	}
	return nil
}

var (
	_ openaichat.Client          = (*openAIClient)(nil)
	_ openairesponses.Client     = responsesClient{}
	_ anthropicmessages.Client   = (*anthropicClient)(nil)
	_ geminigenerate.Client      = (*generateClient)(nil)
	_ geminigenerate.EventStream = (*generateResponseStream)(nil)
)
