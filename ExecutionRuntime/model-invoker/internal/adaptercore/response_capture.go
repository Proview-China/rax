package adaptercore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

const (
	// DefaultMaxResponseBodyBytes is the hard, decompressed response-body limit
	// shared by every provider adapter. It is intentionally not configurable so
	// an invocation can never disable the process-wide memory safety boundary.
	DefaultMaxResponseBodyBytes int64 = 8 << 20

	// ResponseBodyLimitErrorCode is stable for errors.As/error-envelope checks at
	// provider-neutral public boundaries.
	ResponseBodyLimitErrorCode = "response_body_limit_exceeded"
)

type responseCaptureContextKey struct{}

// CapturedResponse is a defensive snapshot of one HTTP response. Streaming
// captures intentionally leave Body empty because consuming it would advance
// the provider stream.
type CapturedResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// RedirectError reports a rejected 3xx response without retaining its URL or
// Location header.
type RedirectError struct {
	StatusCode int
}

func (e *RedirectError) Error() string {
	return fmt.Sprintf("HTTP redirect response rejected (status %d)", e.StatusCode)
}

// ResponseCapture stores one response for a request context. Its mutable state
// is protected so the same context remains race-safe if a caller misuses it for
// concurrent requests.
type ResponseCapture struct {
	mu        sync.RWMutex
	streaming bool
	provider  modelinvoker.ProviderID
	response  CapturedResponse
	limitErr  error
}

// WithResponseCapture binds a fresh capture to ctx. The returned capture is
// isolated from captures attached to all other contexts.
func WithResponseCapture(ctx context.Context, streaming bool, providers ...modelinvoker.ProviderID) (context.Context, *ResponseCapture) {
	var provider modelinvoker.ProviderID
	if len(providers) > 0 {
		provider = providers[0]
	}
	capture := &ResponseCapture{streaming: streaming, provider: provider}
	return context.WithValue(ctx, responseCaptureContextKey{}, capture), capture
}

// Snapshot returns a deep copy safe for use after the HTTP response is closed.
func (c *ResponseCapture) Snapshot() CapturedResponse {
	if c == nil {
		return CapturedResponse{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneCapturedResponse(c.response)
}

// Err returns the stable response-body limit error observed by this request,
// if any. Some SDK stream decoders stringify reader errors, so adapters use
// this side channel to restore the controlled provider-neutral error envelope.
func (c *ResponseCapture) Err() error {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.limitErr
}

// StreamWithResponseCapture restores the controlled body-limit error when an
// SDK decoder stringifies or replaces its reader error. It leaves all normal
// stream events and errors untouched.
func StreamWithResponseCapture(stream modelinvoker.Stream, capture *ResponseCapture) modelinvoker.Stream {
	if stream == nil || capture == nil {
		return stream
	}
	return &responseCaptureStream{inner: stream, capture: capture}
}

type responseCaptureStream struct {
	inner   modelinvoker.Stream
	capture *ResponseCapture
	current modelinvoker.StreamEvent
}

func (s *responseCaptureStream) Next() bool {
	if !s.inner.Next() {
		s.current = modelinvoker.StreamEvent{}
		return false
	}
	s.current = s.inner.Event()
	if s.current.Type == modelinvoker.StreamEventError {
		var invocationError *modelinvoker.Error
		if errors.As(s.capture.Err(), &invocationError) && invocationError != nil {
			copy := *invocationError
			s.current.Error = &copy
		}
	}
	return true
}

func (s *responseCaptureStream) Event() modelinvoker.StreamEvent { return s.current }

func (s *responseCaptureStream) Err() error {
	if err := s.capture.Err(); err != nil {
		return err
	}
	return s.inner.Err()
}

func (s *responseCaptureStream) Close() error { return s.inner.Close() }

// CloneHTTPClientWithResponseCapture returns an independent client whose
// transport records responses only when a ResponseCapture is present in the
// request context. Its transport also rejects every 3xx response. Compose it
// with CloneHTTPClientWithoutRedirects so the outer http.Client cannot follow
// redirects before the response reaches this transport boundary.
func CloneHTTPClientWithResponseCapture(source *http.Client) *http.Client {
	if source == nil {
		source = http.DefaultClient
	}
	cloned := *source
	transport := source.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	cloned.Transport = responseCaptureTransport{next: transport}
	return &cloned
}

type responseCaptureTransport struct {
	next http.RoundTripper
}

func (t responseCaptureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.next.RoundTrip(request)
	if response == nil {
		return response, err
	}
	capture, _ := request.Context().Value(responseCaptureContextKey{}).(*ResponseCapture)
	if response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest {
		if capture != nil {
			capture.store(CapturedResponse{StatusCode: response.StatusCode, Header: response.Header.Clone()})
		}
		if response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, &RedirectError{StatusCode: response.StatusCode}
	}

	var provider modelinvoker.ProviderID
	if capture != nil {
		provider = capture.provider
	}
	if response.Body != nil {
		response.Body = newLimitedResponseBody(response.Body, provider, capture)
	}
	if capture != nil && (capture.streaming || response.Body == nil) {
		capture.store(CapturedResponse{StatusCode: response.StatusCode, Header: response.Header.Clone()})
	} else if capture != nil {
		body, readErr := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		capture.store(CapturedResponse{
			StatusCode: response.StatusCode,
			Header:     response.Header.Clone(),
			Body:       body,
		})
		if IsResponseBodyLimitError(readErr) {
			return nil, readErr
		}
		response.Body = &replayResponseBody{
			Reader:      bytes.NewReader(body),
			terminalErr: readErr,
			closeErr:    closeErr,
		}
	}
	return response, err
}

// IsResponseBodyLimitError reports whether err crossed the shared hard body
// limit. Provider SDK wrappers may add layers, so callers must use errors.As.
func IsResponseBodyLimitError(err error) bool {
	var invocationError *modelinvoker.Error
	return errors.As(err, &invocationError) && invocationError != nil &&
		invocationError.Code == ResponseBodyLimitErrorCode
}

func responseBodyLimitError(provider modelinvoker.ProviderID) *modelinvoker.Error {
	return &modelinvoker.Error{
		Kind:      modelinvoker.ErrorProvider,
		Provider:  provider,
		Operation: "read_response",
		Code:      ResponseBodyLimitErrorCode,
		Message:   fmt.Sprintf("provider response exceeded the %d-byte safety limit", DefaultMaxResponseBodyBytes),
		Retryable: false,
	}
}

type limitedResponseBody struct {
	inner     io.ReadCloser
	provider  modelinvoker.ProviderID
	capture   *ResponseCapture
	remaining int64
	exceeded  bool
	closeOnce sync.Once
	closeErr  error
}

func newLimitedResponseBody(inner io.ReadCloser, provider modelinvoker.ProviderID, capture *ResponseCapture) io.ReadCloser {
	return &limitedResponseBody{inner: inner, provider: provider, capture: capture, remaining: DefaultMaxResponseBodyBytes}
}

func (b *limitedResponseBody) Read(destination []byte) (int, error) {
	if b.exceeded {
		return 0, b.limitError()
	}
	if len(destination) == 0 {
		return 0, nil
	}
	if b.remaining > 0 {
		if int64(len(destination)) > b.remaining {
			destination = destination[:b.remaining]
		}
		read, err := b.inner.Read(destination)
		b.remaining -= int64(read)
		return read, err
	}

	var probe [1]byte
	read, err := b.inner.Read(probe[:])
	if read == 0 {
		return 0, err
	}
	b.exceeded = true
	_ = b.Close()
	return 0, b.limitError()
}

func (b *limitedResponseBody) Close() error {
	b.closeOnce.Do(func() {
		b.closeErr = b.inner.Close()
	})
	return b.closeErr
}

func (b *limitedResponseBody) limitError() error {
	err := responseBodyLimitError(b.provider)
	if b.capture != nil {
		b.capture.storeLimitError(err)
	}
	return err
}

func (c *ResponseCapture) storeLimitError(err error) {
	if c == nil || err == nil {
		return
	}
	c.mu.Lock()
	if c.limitErr == nil {
		c.limitErr = err
	}
	c.mu.Unlock()
}

func (c *ResponseCapture) store(response CapturedResponse) {
	c.mu.Lock()
	c.response = cloneCapturedResponse(response)
	c.mu.Unlock()
}

func cloneCapturedResponse(response CapturedResponse) CapturedResponse {
	return CapturedResponse{
		StatusCode: response.StatusCode,
		Header:     response.Header.Clone(),
		Body:       append([]byte(nil), response.Body...),
	}
}

// replayResponseBody preserves a transport read error after replaying the
// bytes captured before that error.
type replayResponseBody struct {
	*bytes.Reader
	terminalErr error
	closeErr    error
}

func (b *replayResponseBody) Read(destination []byte) (int, error) {
	read, err := b.Reader.Read(destination)
	if err == io.EOF && b.terminalErr != nil {
		err = b.terminalErr
		b.terminalErr = nil
	}
	return read, err
}

func (b *replayResponseBody) Close() error {
	return b.closeErr
}
