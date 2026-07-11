package azureopenai

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openairesponses"
	openaisdk "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"net/http"
	"strings"
)

type sdkClient struct {
	chat      openaisdk.ChatCompletionService
	responses responses.ResponseService
}
type chatClient struct{ native *sdkClient }
type responsesClient struct{ native *sdkClient }

func credential(c Config) credentialSource {
	if c.CredentialMode == CredentialEntraID {
		return entraToken{source: c.AccessTokenProvider}
	}
	if c.APIKeyProvider != nil {
		return renewableAPIKey{source: c.APIKeyProvider}
	}
	return staticAPIKey(c.APIKey)
}
func newHTTPClient(c Config, apiVersion string) *http.Client {
	client := adaptercore.CloneHTTPClientWithoutRedirects(c.HTTPClient)
	next := client.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	client.Transport = authTransport{next: next, source: credential(c), apiVersion: apiVersion}
	return adaptercore.CloneHTTPClientWithResponseCapture(client)
}
func newV1Clients(c Config) (openaichat.Client, openairesponses.Client) {
	base := strings.TrimRight(c.root(), "/") + "/openai/v1/"
	client := newHTTPClient(c, "")
	opts := []openaioption.RequestOption{openaioption.WithBaseURL(base), openaioption.WithAPIKey("praxis-azure-managed"), openaioption.WithMaxRetries(0), openaioption.WithHTTPClient(client)}
	native := &sdkClient{chat: openaisdk.NewChatCompletionService(opts...), responses: responses.NewResponseService(opts...)}
	return chatClient{native: native}, responsesClient{native: native}
}
func newLegacyChatClient(c Config) openaichat.Client {
	base := strings.TrimRight(c.root(), "/") + "/openai/deployments/" + urlPathSegment(c.DeploymentName) + "/"
	client := newHTTPClient(c, c.LegacyAPIVersion)
	opts := []openaioption.RequestOption{openaioption.WithBaseURL(base), openaioption.WithAPIKey("praxis-azure-managed"), openaioption.WithMaxRetries(0), openaioption.WithHTTPClient(client)}
	return chatClient{native: &sdkClient{chat: openaisdk.NewChatCompletionService(opts...)}}
}
func urlPathSegment(value string) string { return strings.ReplaceAll(value, " ", "%20") }
func (c chatClient) Create(ctx context.Context, p openaisdk.ChatCompletionNewParams) (*openaisdk.ChatCompletion, http.Header, error) {
	var raw *http.Response
	v, err := c.native.chat.New(ctx, p, openaioption.WithResponseInto(&raw))
	return v, adaptercore.ResponseHeaders(raw), err
}
func (c chatClient) Stream(ctx context.Context, p openaisdk.ChatCompletionNewParams) (openaichat.EventStream, http.Header) {
	var raw *http.Response
	s := c.native.chat.NewStreaming(ctx, p, openaioption.WithResponseInto(&raw))
	return s, adaptercore.ResponseHeaders(raw)
}
func (c responsesClient) Create(ctx context.Context, p responses.ResponseNewParams) (*responses.Response, http.Header, error) {
	var raw *http.Response
	v, err := c.native.responses.New(ctx, p, openaioption.WithResponseInto(&raw))
	return v, adaptercore.ResponseHeaders(raw), err
}
func (c responsesClient) Stream(ctx context.Context, p responses.ResponseNewParams) (openairesponses.EventStream, http.Header) {
	var raw *http.Response
	s := c.native.responses.NewStreaming(ctx, p, openaioption.WithResponseInto(&raw))
	return s, adaptercore.ResponseHeaders(raw)
}

var _ openaichat.Client = chatClient{}
var _ openairesponses.Client = responsesClient{}
