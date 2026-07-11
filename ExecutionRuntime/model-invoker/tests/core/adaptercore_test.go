package core_test

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

func TestAdapterCoreEndpointAndSelection(t *testing.T) {
	if got := adaptercore.NormalizeEndpoint("https://example.test/v1/"); got != "https://example.test/v1" {
		t.Fatalf("NormalizeEndpoint() = %q", got)
	}
	if got := adaptercore.EffectiveEndpoint("", "https://example.test/v1/"); got != "https://example.test/v1" {
		t.Fatalf("EffectiveEndpoint() = %q", got)
	}
	if !adaptercore.IsLoopbackHost("localhost") || !adaptercore.IsLoopbackHost("127.0.0.1") || adaptercore.IsLoopbackHost("example.test") {
		t.Fatal("IsLoopbackHost() returned an invalid classification")
	}
	request := Request{
		Provider: "provider", Protocol: ProtocolMessages,
		Endpoint: "https://example.test/v1/", Model: "model",
		Input:           []InputItem{MessageInput(RoleUser, "hello")},
		ProviderOptions: ProviderOptions{"provider": json.RawMessage(`{}`)},
	}
	if err := adaptercore.ValidateSelection(request, "provider", "https://example.test/v1", ProtocolMessages); err != nil {
		t.Fatalf("ValidateSelection() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Request)
		want   string
	}{
		{name: "provider", mutate: func(r *Request) { r.Provider = "other" }, want: "provider"},
		{name: "protocol", mutate: func(r *Request) { r.Protocol = ProtocolGenerateContent }, want: "protocol"},
		{name: "endpoint", mutate: func(r *Request) { r.Endpoint = "https://other.test/v1" }, want: "endpoint"},
		{name: "options", mutate: func(r *Request) { r.ProviderOptions = ProviderOptions{"other": json.RawMessage(`{}`)} }, want: "namespace"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := request
			test.mutate(&candidate)
			if err := adaptercore.ValidateSelection(candidate, "provider", "https://example.test/v1", ProtocolMessages); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateSelection() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAdapterCoreAuditPayload(t *testing.T) {
	payload, err := adaptercore.MarshalAuditRequest(map[string]any{"model": "test"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(payload.Bytes(), &object); err != nil {
		t.Fatal(err)
	}
	if object["model"] != "test" || object["stream"] != true {
		t.Fatalf("audit object = %#v", object)
	}
	bytes := payload.Bytes()
	bytes[0] = 'x'
	if !json.Valid(payload.Bytes()) {
		t.Fatal("RawPayload.Bytes did not return a defensive copy")
	}
	if _, err := adaptercore.MarshalAuditRequest(nil, true); err == nil {
		t.Fatal("MarshalAuditRequest(nil) did not reject a non-object request")
	}

	exact, err := adaptercore.RawPayload(`{"exact":true}`, map[string]any{"fallback": true})
	if err != nil {
		t.Fatal(err)
	}
	if string(exact.Bytes()) != `{"exact":true}` {
		t.Fatalf("exact raw = %s", exact.Bytes())
	}
	fallback, err := adaptercore.RawPayload("", map[string]any{"fallback": true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fallback.Bytes()), `"fallback":true`) {
		t.Fatalf("fallback raw = %s", fallback.Bytes())
	}
	if _, err := adaptercore.RawPayload("", make(chan int)); err == nil {
		t.Fatal("RawPayload() swallowed fallback serialization failure")
	}
}

func TestAdapterCoreHeaders(t *testing.T) {
	headers := http.Header{
		"Request-Id":   []string{"req_primary"},
		"X-Request-Id": []string{"req_secondary"},
		"Retry-After":  []string{"0.125"},
	}
	if got := adaptercore.RequestID(headers, "request-id", "x-request-id"); got != "req_primary" {
		t.Fatalf("RequestID() = %q", got)
	}
	if got := adaptercore.RetryAfter(headers); got != 125*time.Millisecond {
		t.Fatalf("RetryAfter() = %v", got)
	}
	headers.Set("retry-after", "1.5")
	if got := adaptercore.RetryAfter(headers); got != 1500*time.Millisecond {
		t.Fatalf("RetryAfter(seconds) = %v", got)
	}
	now := time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC)
	headers.Set("retry-after", now.Add(2*time.Second).Format(http.TimeFormat))
	if got := adaptercore.RetryAfterAt(headers, now); got != 2*time.Second {
		t.Fatalf("RetryAfterAt(date) = %v", got)
	}
	headers.Set("retry-after", "1e100")
	if got := adaptercore.RetryAfterAt(headers, now); got != 0 {
		t.Fatalf("RetryAfterAt(overflow) = %v", got)
	}
	response := &http.Response{Header: http.Header{"X-Test": []string{"original"}}}
	cloned := adaptercore.ResponseHeaders(response)
	cloned.Set("X-Test", "mutated")
	if response.Header.Get("X-Test") != "original" {
		t.Fatal("ResponseHeaders() did not return a defensive copy")
	}
}

func TestAdapterCoreContextErrorAllowsOnlyContextSentinels(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantKind  ErrorKind
		wantCause error
	}{
		{name: "cancelled", err: fmt.Errorf("wrapped: %w", context.Canceled), wantKind: ErrorCancelled, wantCause: context.Canceled},
		{name: "deadline", err: fmt.Errorf("wrapped: %w", context.DeadlineExceeded), wantKind: ErrorTimeout, wantCause: context.DeadlineExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := adaptercore.ContextError("test-provider", "capabilities", test.err)
			if got.Kind != test.wantKind || got.Provider != "test-provider" || got.Operation != "capabilities" {
				t.Fatalf("ContextError() = %#v", got)
			}
			if !errors.Is(got, test.wantCause) || errors.Is(got, test.err) {
				t.Fatalf("ContextError() unwrap chain = %#v", got.Err)
			}
		})
	}

	native := errors.New("sdk secret")
	got := adaptercore.ContextError("test-provider", "capabilities", native)
	if got.Kind != ErrorProvider || got.Err != nil || errors.Is(got, native) || strings.Contains(got.Error(), native.Error()) {
		t.Fatalf("unknown ContextError() exposed native error: %#v (%v)", got, got)
	}
}

func TestAdapterCoreHTTPClientCloneRejectsRedirectsWithoutMutation(t *testing.T) {
	var originalRedirectCalls atomic.Int64
	original := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		originalRedirectCalls.Add(1)
		return nil
	}}
	cloned := adaptercore.CloneHTTPClientWithoutRedirects(original)
	if cloned == original || original.CheckRedirect == nil {
		t.Fatal("HTTP client was not independently cloned")
	}

	secondHopCalls := atomic.Int64{}
	secondHop := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		secondHopCalls.Add(1)
	}))
	t.Cleanup(secondHop.Close)
	firstHop := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, secondHop.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(firstHop.Close)

	response, err := cloned.Get(firstHop.URL)
	if err != nil {
		t.Fatalf("redirect-rejecting client returned error: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusTemporaryRedirect || secondHopCalls.Load() != 0 {
		t.Fatalf("redirect status/second hops = %d/%d", response.StatusCode, secondHopCalls.Load())
	}
	if originalRedirectCalls.Load() != 0 {
		t.Fatal("cloned client invoked the caller's CheckRedirect callback")
	}
}

func TestAdapterCoreResponseCaptureIsContextIsolatedAndConcurrent(t *testing.T) {
	client := adaptercore.CloneHTTPClientWithResponseCapture(&http.Client{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Capture", request.URL.Query().Get("id"))
		_, _ = fmt.Fprint(writer, request.URL.Query().Get("body"))
	}))
	t.Cleanup(server.Close)

	type result struct {
		id       string
		body     string
		capture  adaptercore.CapturedResponse
		readBody string
		err      error
	}
	results := make(chan result, 32)
	var group sync.WaitGroup
	for index := range 32 {
		group.Add(1)
		go func() {
			defer group.Done()
			id := fmt.Sprintf("request-%d", index)
			body := fmt.Sprintf("body-%d", index)
			ctx, capture := adaptercore.WithResponseCapture(context.Background(), false)
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"?id="+id+"&body="+body, nil)
			if err != nil {
				results <- result{err: err}
				return
			}
			response, err := client.Do(request)
			if err != nil {
				results <- result{err: err}
				return
			}
			read, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			results <- result{id: id, body: body, capture: capture.Snapshot(), readBody: string(read), err: readErr}
		}()
	}
	group.Wait()
	close(results)
	for got := range results {
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.capture.Header.Get("X-Capture") != got.id || string(got.capture.Body) != got.body || got.readBody != got.body {
			t.Fatalf("isolated capture = %#v / read %q, want %q/%q", got.capture, got.readBody, got.id, got.body)
		}
	}
}

func TestAdapterCoreResponseCaptureHTTPClientCloneDoesNotMutateSource(t *testing.T) {
	transport := http.DefaultTransport
	redirect := func(*http.Request, []*http.Request) error { return nil }
	original := &http.Client{Transport: transport, CheckRedirect: redirect, Timeout: 3 * time.Second}
	cloned := adaptercore.CloneHTTPClientWithResponseCapture(original)
	if cloned == original || cloned.Transport == original.Transport {
		t.Fatal("response-capture client did not receive an independent transport wrapper")
	}
	if original.Transport != transport || original.CheckRedirect == nil || original.Timeout != 3*time.Second {
		t.Fatal("response-capture clone mutated the source client")
	}
	if cloned.CheckRedirect == nil || cloned.Timeout != original.Timeout {
		t.Fatal("response-capture clone did not preserve unrelated client policy")
	}
}

func TestAdapterCoreStreamingResponseCaptureDoesNotReadBody(t *testing.T) {
	body := &countingReadCloser{Reader: strings.NewReader("stream-data")}
	client := adaptercore.CloneHTTPClientWithResponseCapture(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"X-Stream": []string{"yes"}}, Body: body}, nil
	})})
	ctx, capture := adaptercore.WithResponseCapture(context.Background(), true)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if body.reads.Load() != 0 {
		t.Fatalf("stream body was read %d times during capture", body.reads.Load())
	}
	snapshot := capture.Snapshot()
	if snapshot.Header.Get("X-Stream") != "yes" || len(snapshot.Body) != 0 {
		t.Fatalf("stream capture = %#v", snapshot)
	}
	_, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if body.reads.Load() == 0 {
		t.Fatal("stream body was not readable by the caller")
	}
}

func TestAdapterCoreResponseBodyHardLimitIsSharedAndExact(t *testing.T) {
	t.Run("non-streaming capture", func(t *testing.T) {
		body := &trackedReadCloser{Reader: io.LimitReader(repeatingByteReader('x'), adaptercore.DefaultMaxResponseBodyBytes+1)}
		client := adaptercore.CloneHTTPClientWithResponseCapture(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}, nil
		})})
		ctx, capture := adaptercore.WithResponseCapture(context.Background(), false, "provider")
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://body-limit.invalid/secret-url", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if response != nil {
			_ = response.Body.Close()
			t.Fatal("oversized response unexpectedly crossed the transport boundary")
		}
		assertResponseBodyLimitError(t, err, "provider")
		if snapshot := capture.Snapshot(); int64(len(snapshot.Body)) != adaptercore.DefaultMaxResponseBodyBytes {
			t.Fatalf("captured body length = %d, want %d", len(snapshot.Body), adaptercore.DefaultMaxResponseBodyBytes)
		}
		if body.reads.Load() == 0 || body.closes.Load() != 1 {
			t.Fatalf("oversized body reads/closes = %d/%d", body.reads.Load(), body.closes.Load())
		}
	})

	t.Run("streaming body", func(t *testing.T) {
		body := &trackedReadCloser{Reader: io.LimitReader(repeatingByteReader('s'), adaptercore.DefaultMaxResponseBodyBytes+1)}
		client := adaptercore.CloneHTTPClientWithResponseCapture(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}, nil
		})})
		ctx, capture := adaptercore.WithResponseCapture(context.Background(), true, "provider")
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://stream-limit.invalid/secret-url", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		read, err := io.ReadAll(response.Body)
		assertResponseBodyLimitError(t, err, "provider")
		if int64(len(read)) != adaptercore.DefaultMaxResponseBodyBytes || len(capture.Snapshot().Body) != 0 {
			t.Fatalf("stream read/capture lengths = %d/%d", len(read), len(capture.Snapshot().Body))
		}
		if body.closes.Load() != 1 {
			t.Fatalf("stream body closes = %d", body.closes.Load())
		}
	})

	t.Run("exact limit succeeds", func(t *testing.T) {
		body := io.NopCloser(io.LimitReader(repeatingByteReader('e'), adaptercore.DefaultMaxResponseBodyBytes))
		client := adaptercore.CloneHTTPClientWithResponseCapture(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}, nil
		})})
		ctx, capture := adaptercore.WithResponseCapture(context.Background(), false, "provider")
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://exact-limit.invalid", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		read, err := io.ReadAll(response.Body)
		if err != nil || int64(len(read)) != adaptercore.DefaultMaxResponseBodyBytes || int64(len(capture.Snapshot().Body)) != adaptercore.DefaultMaxResponseBodyBytes {
			t.Fatalf("exact-limit read/capture/error = %d/%d/%v", len(read), len(capture.Snapshot().Body), err)
		}
	})

	t.Run("decompressed bytes are limited", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Encoding", "gzip")
			compressed := gzip.NewWriter(writer)
			_, _ = io.CopyN(compressed, repeatingByteReader('z'), adaptercore.DefaultMaxResponseBodyBytes+1)
			_ = compressed.Close()
		}))
		t.Cleanup(server.Close)
		client := adaptercore.CloneHTTPClientWithResponseCapture(server.Client())
		ctx, capture := adaptercore.WithResponseCapture(context.Background(), false, "provider")
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
		assertResponseBodyLimitError(t, err, "provider")
		if int64(len(capture.Snapshot().Body)) != adaptercore.DefaultMaxResponseBodyBytes {
			t.Fatalf("decompressed capture length = %d", len(capture.Snapshot().Body))
		}
	})
}

func TestAdapterCoreRejectsHugeRedirectBeforeReadingOrFollowing(t *testing.T) {
	var secondHopCalls atomic.Int64
	body := &trackedReadCloser{Reader: repeatingByteReader('r')}
	original := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "second-hop.invalid" {
			secondHopCalls.Add(1)
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
		}
		return &http.Response{
			StatusCode: http.StatusTemporaryRedirect,
			Header:     http.Header{"Location": []string{"https://second-hop.invalid/secret"}},
			Body:       body,
		}, nil
	})}
	client := adaptercore.CloneHTTPClientWithoutRedirects(original)
	client = adaptercore.CloneHTTPClientWithResponseCapture(client)
	ctx, capture := adaptercore.WithResponseCapture(context.Background(), false, "provider")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://first-hop.invalid/v1", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if response != nil {
		_ = response.Body.Close()
		t.Fatal("redirect returned a response")
	}
	var redirect *adaptercore.RedirectError
	if !errors.As(err, &redirect) || redirect.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("redirect error = %#v", err)
	}
	if body.reads.Load() != 0 || body.closes.Load() != 1 || secondHopCalls.Load() != 0 {
		t.Fatalf("redirect body reads/closes/second hops = %d/%d/%d", body.reads.Load(), body.closes.Load(), secondHopCalls.Load())
	}
	if snapshot := capture.Snapshot(); snapshot.StatusCode != http.StatusTemporaryRedirect || len(snapshot.Body) != 0 || snapshot.Header.Get("Location") == "" {
		t.Fatalf("redirect capture = %#v", snapshot)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type countingReadCloser struct {
	io.Reader
	reads atomic.Int64
}

func (r *countingReadCloser) Read(destination []byte) (int, error) {
	r.reads.Add(1)
	return r.Reader.Read(destination)
}

func (*countingReadCloser) Close() error { return nil }

type repeatingByteReader byte

func (r repeatingByteReader) Read(destination []byte) (int, error) {
	for index := range destination {
		destination[index] = byte(r)
	}
	return len(destination), nil
}

type trackedReadCloser struct {
	io.Reader
	reads  atomic.Int64
	closes atomic.Int64
}

func (r *trackedReadCloser) Read(destination []byte) (int, error) {
	r.reads.Add(1)
	return r.Reader.Read(destination)
}

func (r *trackedReadCloser) Close() error {
	r.closes.Add(1)
	return nil
}

func assertResponseBodyLimitError(t *testing.T, err error, provider ProviderID) {
	t.Helper()
	if !adaptercore.IsResponseBodyLimitError(err) {
		t.Fatalf("error = %T %v, want response-body limit", err, err)
	}
	var invocationError *Error
	if !errors.As(err, &invocationError) || invocationError == nil || invocationError.Provider != provider ||
		invocationError.Kind != ErrorProvider || invocationError.Code != adaptercore.ResponseBodyLimitErrorCode || invocationError.Retryable {
		t.Fatalf("response-body limit error = %#v", invocationError)
	}
	if strings.Contains(invocationError.Error(), "body-limit.invalid") || strings.Contains(invocationError.Error(), "secret-url") {
		t.Fatalf("response-body limit error retained URL: %v", invocationError)
	}
}

func TestAdapterCoreCapabilityContractIsCompleteAndDefensive(t *testing.T) {
	capabilities := adaptercore.KnownCapabilities()
	if len(capabilities) == 0 {
		t.Fatal("KnownCapabilities() is empty")
	}
	first := capabilities[0]
	capabilities[0] = "mutated"
	if adaptercore.KnownCapabilities()[0] != first {
		t.Fatal("KnownCapabilities() returned shared storage")
	}
	contract := adaptercore.UnsupportedContract("not supported")
	for _, capability := range adaptercore.KnownCapabilities() {
		if support, ok := contract[capability]; !ok || support.Level != SupportUnsupported {
			t.Fatalf("capability %q = %#v", capability, support)
		}
	}
	query := CapabilityQuery{Protocol: ProtocolMessages, Model: "model"}
	adaptercore.SetSupport(contract, query, SupportNative, "native", CapabilityTextGeneration)
	support := contract[CapabilityTextGeneration]
	if support.Level != SupportNative || len(support.Protocols) != 1 || support.Protocols[0] != ProtocolMessages || len(support.Models) != 1 || support.Models[0] != "model" {
		t.Fatalf("query support = %#v", support)
	}
}
