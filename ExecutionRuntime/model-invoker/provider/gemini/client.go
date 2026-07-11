package gemini

import (
	"context"
	"fmt"
	"iter"
	"net/http"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/geminigenerate"
	"google.golang.org/genai"
)

type sdkClient struct {
	models *genai.Models
}

func newSDKClient(config Config) (geminigenerate.Client, error) {
	httpClient := adaptercore.CloneHTTPClientWithoutRedirects(config.HTTPClient)
	httpClient = adaptercore.CloneHTTPClientWithResponseCapture(httpClient)
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:     config.APIKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: httpClient,
		HTTPOptions: genai.HTTPOptions{
			BaseURL:    config.effectiveBaseURL(),
			APIVersion: config.effectiveAPIVersion(),
		},
	})
	if err != nil {
		return nil, err
	}
	if client == nil || client.Models == nil {
		return nil, fmt.Errorf("gemini SDK returned an uninitialized client")
	}
	return &sdkClient{models: client.Models}, nil
}

func (c *sdkClient) GenerateContent(
	ctx context.Context,
	model string,
	contents []*genai.Content,
	config *genai.GenerateContentConfig,
) (*genai.GenerateContentResponse, http.Header, error) {
	response, err := c.models.GenerateContent(ctx, model, contents, config)
	var headers http.Header
	if response != nil && response.SDKHTTPResponse != nil && response.SDKHTTPResponse.Headers != nil {
		headers = response.SDKHTTPResponse.Headers.Clone()
	}
	return response, headers, err
}

func (c *sdkClient) GenerateContentStream(
	ctx context.Context,
	model string,
	contents []*genai.Content,
	config *genai.GenerateContentConfig,
) (geminigenerate.EventStream, http.Header, error) {
	sequence := c.models.GenerateContentStream(ctx, model, contents, config)
	stream := newSDKResponseStream(sequence)
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

type sdkResponseStream struct {
	next       func() (*genai.GenerateContentResponse, error, bool)
	stop       func()
	current    *genai.GenerateContentResponse
	err        error
	prefetched bool
	exhausted  bool
	closed     bool
}

func newSDKResponseStream(sequence iter.Seq2[*genai.GenerateContentResponse, error]) *sdkResponseStream {
	next, stop := iter.Pull2(sequence)
	stream := &sdkResponseStream{next: next, stop: stop}
	stream.prefetch()
	return stream
}

func (s *sdkResponseStream) prefetch() {
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
		s.err = fmt.Errorf("gemini SDK stream returned a nil response")
		return
	}
	s.current = response
	s.prefetched = true
}

func (s *sdkResponseStream) Next() bool {
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
		s.err = fmt.Errorf("gemini SDK stream returned a nil response")
		return false
	}
	s.current = response
	return true
}

func (s *sdkResponseStream) Current() *genai.GenerateContentResponse {
	if s == nil {
		return nil
	}
	return s.current
}

func (s *sdkResponseStream) Err() error {
	if s == nil {
		return fmt.Errorf("gemini SDK stream is nil")
	}
	return s.err
}

func (s *sdkResponseStream) Close() error {
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
	_ geminigenerate.Client      = (*sdkClient)(nil)
	_ geminigenerate.EventStream = (*sdkResponseStream)(nil)
)
