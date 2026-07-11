package vertex

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/anthropicmessages"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/geminigenerate"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	anthropicvertex "github.com/anthropics/anthropic-sdk-go/vertex"
	openaisdk "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/genai"
)

type geminiClient struct{ models *genai.Models }
type chatClient struct {
	chat openaisdk.ChatCompletionService
}
type messagesClient struct{ messages anthropicsdk.MessageService }

func newSDKClients(config Config) (geminigenerate.Client, openaichat.Client, anthropicmessages.Client, error) {
	httpClient := adaptercore.CloneHTTPClientWithoutRedirects(config.HTTPClient)
	next := httpClient.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	var googleCredentials *google.Credentials
	if config.CredentialMode == CredentialAPIKey {
		var source credentialSource
		if config.APIKeyProvider != nil {
			source = renewableAPIKey{source: config.APIKeyProvider}
		} else {
			source = staticAPIKey(config.APIKey)
		}
		httpClient.Transport = authTransport{next: next, source: source}
	} else if config.AccessTokenProvider != nil {
		googleCredentials = &google.Credentials{ProjectID: config.Project, TokenSource: oauthTokenSource{source: config.AccessTokenProvider}}
		httpClient.Transport = &oauth2.Transport{Source: googleCredentials.TokenSource, Base: next}
	} else {
		var err error
		googleCredentials, err = google.FindDefaultCredentials(context.Background(), "https://www.googleapis.com/auth/cloud-platform")
		if err != nil {
			return nil, nil, nil, fmt.Errorf("vertex: load application default credentials: %w", err)
		}
		httpClient.Transport = &oauth2.Transport{Source: googleCredentials.TokenSource, Base: next}
	}
	httpClient = adaptercore.CloneHTTPClientWithResponseCapture(httpClient)
	clientConfig := &genai.ClientConfig{Backend: genai.BackendVertexAI, Project: config.Project, Location: config.Location, HTTPClient: httpClient, HTTPOptions: genai.HTTPOptions{BaseURL: strings.TrimRight(config.rootEndpoint(), "/") + "/", APIVersion: "v1"}}
	genaiClient, err := genai.NewClient(context.Background(), clientConfig)
	if err != nil || genaiClient == nil || genaiClient.Models == nil {
		return nil, nil, nil, fmt.Errorf("vertex: initialize Google Gen AI client")
	}
	chatBase := strings.TrimRight(config.rootEndpoint(), "/") + "/v1beta1/projects/" + config.Project + "/locations/" + config.Location + "/endpoints/openapi/"
	chat := openaisdk.NewChatCompletionService(openaioption.WithBaseURL(chatBase), openaioption.WithAPIKey("praxis-vertex-managed"), openaioption.WithMaxRetries(0), openaioption.WithHTTPClient(httpClient))
	var messages anthropicmessages.Client
	if config.CredentialMode == CredentialADC {
		vertexOption := anthropicvertex.WithCredentials(context.Background(), config.Location, config.Project, googleCredentials)
		anthropic := anthropicsdk.NewClient(
			anthropicoption.WithoutEnvironmentDefaults(), anthropicoption.WithAPIKey(""), anthropicoption.WithMaxRetries(0),
			vertexOption,
			anthropicoption.WithBaseURL(strings.TrimRight(config.rootEndpoint(), "/")+"/"),
			anthropicoption.WithHTTPClient(httpClient),
		)
		messages = messagesClient{messages: anthropic.Messages}
	}
	return &geminiClient{models: genaiClient.Models}, chatClient{chat: chat}, messages, nil
}
func (c *geminiClient) GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, http.Header, error) {
	response, err := c.models.GenerateContent(ctx, model, contents, config)
	var headers http.Header
	if response != nil && response.SDKHTTPResponse != nil {
		headers = response.SDKHTTPResponse.Headers.Clone()
	}
	return response, headers, err
}
func (c *geminiClient) GenerateContentStream(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (geminigenerate.EventStream, http.Header, error) {
	stream := newGeminiStream(c.models.GenerateContentStream(ctx, model, contents, config))
	var headers http.Header
	if current := stream.Current(); current != nil && current.SDKHTTPResponse != nil {
		headers = current.SDKHTTPResponse.Headers.Clone()
	}
	if err := stream.Err(); err != nil {
		_ = stream.Close()
		return nil, headers, err
	}
	return stream, headers, nil
}

type geminiStream struct {
	next                          func() (*genai.GenerateContentResponse, error, bool)
	stop                          func()
	current                       *genai.GenerateContentResponse
	err                           error
	prefetched, exhausted, closed bool
}

func newGeminiStream(sequence iter.Seq2[*genai.GenerateContentResponse, error]) *geminiStream {
	next, stop := iter.Pull2(sequence)
	s := &geminiStream{next: next, stop: stop}
	s.prefetch()
	return s
}
func (s *geminiStream) prefetch() {
	if s.closed || s.exhausted || s.prefetched || s.err != nil {
		return
	}
	v, e, ok := s.next()
	if !ok {
		s.exhausted = true
		return
	}
	if e != nil {
		s.err = e
		return
	}
	if v == nil {
		s.err = fmt.Errorf("vertex: SDK stream returned nil response")
		return
	}
	s.current = v
	s.prefetched = true
}
func (s *geminiStream) Next() bool {
	if s == nil || s.closed || s.err != nil || s.exhausted {
		return false
	}
	if s.prefetched {
		s.prefetched = false
		return true
	}
	v, e, ok := s.next()
	if !ok {
		s.exhausted = true
		return false
	}
	if e != nil {
		s.err = e
		return false
	}
	if v == nil {
		s.err = fmt.Errorf("vertex: SDK stream returned nil response")
		return false
	}
	s.current = v
	return true
}
func (s *geminiStream) Current() *genai.GenerateContentResponse {
	if s == nil {
		return nil
	}
	return s.current
}
func (s *geminiStream) Err() error {
	if s == nil {
		return fmt.Errorf("vertex: stream is nil")
	}
	return s.err
}
func (s *geminiStream) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	if s.stop != nil {
		s.stop()
	}
	return nil
}
func (c chatClient) Create(ctx context.Context, p openaisdk.ChatCompletionNewParams) (*openaisdk.ChatCompletion, http.Header, error) {
	var raw *http.Response
	v, err := c.chat.New(ctx, p, openaioption.WithResponseInto(&raw))
	return v, adaptercore.ResponseHeaders(raw), err
}
func (c chatClient) Stream(ctx context.Context, p openaisdk.ChatCompletionNewParams) (openaichat.EventStream, http.Header) {
	var raw *http.Response
	s := c.chat.NewStreaming(ctx, p, openaioption.WithResponseInto(&raw))
	return s, adaptercore.ResponseHeaders(raw)
}
func (c messagesClient) Create(ctx context.Context, p anthropicsdk.MessageNewParams) (*anthropicsdk.Message, http.Header, error) {
	var raw *http.Response
	v, err := c.messages.New(ctx, p, anthropicoption.WithResponseInto(&raw))
	return v, adaptercore.ResponseHeaders(raw), err
}
func (c messagesClient) Stream(ctx context.Context, p anthropicsdk.MessageNewParams) (anthropicmessages.EventStream, http.Header) {
	var raw *http.Response
	s := c.messages.NewStreaming(ctx, p, anthropicoption.WithResponseInto(&raw))
	return s, adaptercore.ResponseHeaders(raw)
}

var _ geminigenerate.Client = (*geminiClient)(nil)
var _ openaichat.Client = chatClient{}
var _ anthropicmessages.Client = messagesClient{}
