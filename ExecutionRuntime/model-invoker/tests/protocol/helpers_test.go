package protocol_test

import (
	"context"
	"net/http"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
)

const (
	testProvider modelinvoker.ProviderID = "acme-hosted"
	testEndpoint                         = "https://gateway.example.test/v1"
)

func mustBinding(t *testing.T) protocol.Binding {
	t.Helper()
	binding, err := protocol.NewBinding(testProvider, modelinvoker.ProtocolChatCompletions, testEndpoint, "x-request-id", "request-id")
	if err != nil {
		t.Fatalf("NewBinding() error = %v", err)
	}
	return binding
}

func validRequest(endpoint string) modelinvoker.Request {
	return modelinvoker.Request{
		Provider: testProvider,
		Protocol: modelinvoker.ProtocolChatCompletions,
		Endpoint: endpoint,
		Model:    "test-model",
		Input:    []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
	}
}

type fakeDialect struct {
	validateErr    error
	classification protocol.ErrorClassification
	metadata       modelinvoker.ProviderMetadata
	validateCalls  int
	classifyCalls  int
	metadataCalls  int
	lastFailure    protocol.Failure
	mutateHeaders  bool
	mutateFailure  bool
}

func (d *fakeDialect) ValidateRequest(modelinvoker.Request) error {
	d.validateCalls++
	return d.validateErr
}

func (d *fakeDialect) ClassifyFailure(failure protocol.Failure) protocol.ErrorClassification {
	d.classifyCalls++
	d.lastFailure = failure.Clone()
	if d.mutateFailure && len(failure.Signals) > 0 {
		failure.Signals[0].Value = "dialect-mutated"
		bytes := failure.Raw.Bytes()
		if len(bytes) > 0 {
			bytes[0] = 'X'
		}
	}
	return d.classification
}

func (d *fakeDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	d.metadataCalls++
	if d.mutateHeaders {
		headers.Set("X-Test", "dialect-mutated")
	}
	return d.metadata
}

type fakeDriver struct {
	base           *protocol.Base
	invokeResponse modelinvoker.Response
	invokeErr      error
	stream         modelinvoker.Stream
	streamErr      error
	nativeCalls    int
}

func (d *fakeDriver) Binding() protocol.Binding {
	if d == nil || d.base == nil {
		return protocol.Binding{}
	}
	return d.base.Binding()
}

func (d *fakeDriver) Invoke(ctx context.Context, request modelinvoker.Request) (modelinvoker.Response, error) {
	if err := d.base.Validate(request); err != nil {
		return modelinvoker.Response{}, err
	}
	d.nativeCalls++
	response := d.base.StampResponse(request, d.invokeResponse)
	if d.invokeErr == nil {
		return response, nil
	}
	return response, d.base.StampError(ctx, request, d.invokeErr, "fake.invoke")
}

func (d *fakeDriver) Stream(ctx context.Context, request modelinvoker.Request) (modelinvoker.Stream, error) {
	if err := d.base.Validate(request); err != nil {
		return nil, err
	}
	d.nativeCalls++
	if d.streamErr != nil {
		return nil, d.base.StampError(ctx, request, d.streamErr, "fake.stream")
	}
	return d.base.BindStream(ctx, request, d.stream), nil
}

type fakeStream struct {
	events     []modelinvoker.StreamEvent
	index      int
	nextCalls  int
	terminal   error
	closeErr   error
	closeCalls int
}

func (s *fakeStream) Next() bool {
	s.nextCalls++
	if s.index >= len(s.events) {
		return false
	}
	s.index++
	return true
}

func (s *fakeStream) Event() modelinvoker.StreamEvent {
	if s.index == 0 || s.index > len(s.events) {
		return modelinvoker.StreamEvent{}
	}
	return s.events[s.index-1]
}

func (s *fakeStream) Err() error { return s.terminal }

func (s *fakeStream) Close() error {
	s.closeCalls++
	return s.closeErr
}

type fakeCredential struct {
	APIKey string
}

type fakeSDKError struct {
	Request    *http.Request
	Credential fakeCredential
	message    string
}

func (e *fakeSDKError) Error() string { return e.message }

var (
	_ protocol.Dialect    = (*fakeDialect)(nil)
	_ protocol.Driver     = (*fakeDriver)(nil)
	_ modelinvoker.Stream = (*fakeStream)(nil)
	_ error               = (*fakeSDKError)(nil)
)
