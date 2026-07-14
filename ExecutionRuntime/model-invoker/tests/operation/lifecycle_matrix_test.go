package operation_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/job"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/resource"
)

type matrixStream struct {
	events []operation.StreamEvent
	index  int
	err    error
	closed bool
}

func (s *matrixStream) Next() bool {
	if s == nil || s.index >= len(s.events) {
		return false
	}
	s.index++
	return true
}
func (s *matrixStream) Event() operation.StreamEvent {
	if s == nil || s.index == 0 || s.index > len(s.events) {
		return operation.StreamEvent{}
	}
	return s.events[s.index-1]
}
func (s *matrixStream) Err() error {
	if s == nil {
		return nil
	}
	return s.err
}
func (s *matrixStream) Close() error {
	if s != nil {
		s.closed = true
	}
	return nil
}

type matrixProvider struct {
	id         modelinvoker.ProviderID
	kinds      []operation.Kind
	advertised operation.CapabilityContract
	mu         sync.Mutex
	requests   []operation.Request
	stream     operation.Stream
	invokeErr  error
	streamErr  error
}

func (p *matrixProvider) ID() modelinvoker.ProviderID { return p.id }
func (p *matrixProvider) Kinds() []operation.Kind     { return append([]operation.Kind(nil), p.kinds...) }
func (p *matrixProvider) Capabilities(context.Context, operation.Query) (operation.CapabilityContract, error) {
	if p.advertised != nil {
		return p.advertised, nil
	}
	out := operation.CapabilityContract{}
	for _, kind := range p.kinds {
		out[kind] = operation.Capability{Level: operation.SupportNative, Lifecycle: operation.LifecycleRequest}
	}
	return out, nil
}
func (p *matrixProvider) Invoke(_ context.Context, request operation.Request) (operation.Result, error) {
	p.mu.Lock()
	p.requests = append(p.requests, request)
	p.mu.Unlock()
	return operation.Result{Status: operation.StatusSucceeded}, p.invokeErr
}
func (p *matrixProvider) Stream(_ context.Context, request operation.Request) (operation.Stream, error) {
	p.mu.Lock()
	p.requests = append(p.requests, request)
	p.mu.Unlock()
	return p.stream, p.streamErr
}

func allLifecycleKinds() []operation.Kind {
	return []operation.Kind{
		operation.FileCreate, operation.FileList, operation.FileGet, operation.FileDelete, operation.FileContent,
		operation.StoreCreate, operation.StoreList, operation.StoreGet, operation.StoreDelete, operation.StoreSearch,
		operation.VideoGenerate, operation.VideoGet, operation.VideoDelete, operation.VideoContent,
		operation.BatchCreate, operation.BatchList, operation.BatchGet, operation.BatchCancel, operation.BatchDelete, operation.BatchResults,
	}
}

func TestResourceAndJobClientsCoverEveryLifecycleRoute(t *testing.T) {
	stream := &matrixStream{events: []operation.StreamEvent{{Type: operation.StreamStarted, Sequence: 1}, {Type: operation.StreamCompleted, Sequence: 2}}}
	provider := &matrixProvider{id: "matrix", kinds: allLifecycleKinds(), stream: stream}
	registry, _ := operation.NewRegistry(provider)
	invoker, _ := operation.NewInvoker(registry)
	resources, _ := resource.NewClient(invoker)
	jobs, _ := job.NewClient(invoker)

	resourceCalls := []struct {
		kind resource.Kind
		call func(context.Context, resource.Request) (operation.Result, error)
	}{
		{resource.File, resources.Create}, {resource.File, resources.List}, {resource.File, resources.Get}, {resource.File, resources.Delete}, {resource.File, resources.Content},
		{resource.Store, resources.Create}, {resource.Store, resources.List}, {resource.Store, resources.Get}, {resource.Store, resources.Delete}, {resource.Store, resources.Search},
	}
	query := map[string][]string{"limit": {"1"}}
	metadata := modelinvoker.Metadata{"trace": "original"}
	for _, item := range resourceCalls {
		_, err := item.call(context.Background(), resource.Request{Provider: "matrix", Kind: item.kind, ID: "id", ParentID: "parent", Query: query, Metadata: metadata})
		if err != nil {
			t.Fatalf("resource %s failed: %v", item.kind, err)
		}
	}

	jobCalls := []struct {
		kind job.Kind
		call func(context.Context, job.Request) (operation.Result, error)
	}{
		{job.Video, jobs.Create}, {job.Video, jobs.Get}, {job.Video, jobs.Results}, {job.Video, jobs.Delete},
		{job.Batch, jobs.Create}, {job.Batch, jobs.List}, {job.Batch, jobs.Get}, {job.Batch, jobs.Cancel}, {job.Batch, jobs.Delete},
	}
	for _, item := range jobCalls {
		_, err := item.call(context.Background(), job.Request{Provider: "matrix", Kind: item.kind, ID: "id", Query: query, Metadata: metadata})
		if err != nil {
			t.Fatalf("job %s failed: %v", item.kind, err)
		}
	}
	batchStream, err := jobs.StreamResults(context.Background(), job.Request{Provider: "matrix", Kind: job.Batch, ID: "id"})
	if err != nil || !batchStream.Next() || batchStream.Event().Type != operation.StreamStarted {
		t.Fatalf("batch results stream failed: event=%+v err=%v", batchStream.Event(), err)
	}
	if err := batchStream.Close(); err != nil || !stream.closed {
		t.Fatalf("batch stream did not close: %v", err)
	}

	query["limit"][0] = "mutated"
	metadata["trace"] = "mutated"
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.requests) != len(resourceCalls)+len(jobCalls)+1 {
		t.Fatalf("unexpected request count: %d", len(provider.requests))
	}
	if provider.requests[0].Query["limit"][0] != "1" || provider.requests[0].Metadata["trace"] != "original" {
		t.Fatal("resource query or metadata aliases caller-owned maps")
	}
	if provider.requests[len(resourceCalls)].Query["limit"][0] != "1" || provider.requests[len(resourceCalls)].Metadata["trace"] != "original" {
		t.Fatal("job query or metadata aliases caller-owned maps")
	}
}

func TestLifecycleClientsRejectWrongKindAndNilReceiverWithoutPanic(t *testing.T) {
	var resources *resource.Client
	var jobs *job.Client
	resourceNilCalls := []func() error{
		func() error { _, err := resources.Create(context.Background(), resource.Request{}); return err },
		func() error {
			_, err := resources.Content(context.Background(), resource.Request{Kind: resource.File})
			return err
		},
		func() error {
			_, err := resources.Search(context.Background(), resource.Request{Kind: resource.Store})
			return err
		},
	}
	jobNilCalls := []func() error{
		func() error { _, err := jobs.Create(context.Background(), job.Request{}); return err },
		func() error { _, err := jobs.List(context.Background(), job.Request{Kind: job.Batch}); return err },
		func() error { _, err := jobs.Results(context.Background(), job.Request{Kind: job.Video}); return err },
		func() error { _, err := jobs.Cancel(context.Background(), job.Request{Kind: job.Batch}); return err },
		func() error { _, err := jobs.Delete(context.Background(), job.Request{Kind: job.Video}); return err },
		func() error {
			_, err := jobs.StreamResults(context.Background(), job.Request{Kind: job.Batch})
			return err
		},
	}
	for index, call := range append(resourceNilCalls, jobNilCalls...) {
		if err := call(); err == nil {
			t.Fatalf("nil receiver call %d succeeded", index)
		}
	}

	provider := &matrixProvider{id: "matrix", kinds: allLifecycleKinds()}
	registry, _ := operation.NewRegistry(provider)
	invoker, _ := operation.NewInvoker(registry)
	resources, _ = resource.NewClient(invoker)
	jobs, _ = job.NewClient(invoker)
	wrongCalls := []func() error{
		func() error {
			_, err := resources.Create(context.Background(), resource.Request{Provider: "matrix", Kind: resource.Kind("other")})
			return err
		},
		func() error {
			_, err := resources.Content(context.Background(), resource.Request{Provider: "matrix", Kind: resource.Store})
			return err
		},
		func() error {
			_, err := resources.Search(context.Background(), resource.Request{Provider: "matrix", Kind: resource.File})
			return err
		},
		func() error {
			_, err := jobs.Create(context.Background(), job.Request{Provider: "matrix", Kind: job.Kind("other")})
			return err
		},
		func() error {
			_, err := jobs.List(context.Background(), job.Request{Provider: "matrix", Kind: job.Video})
			return err
		},
		func() error {
			_, err := jobs.Results(context.Background(), job.Request{Provider: "matrix", Kind: job.Batch})
			return err
		},
		func() error {
			_, err := jobs.Cancel(context.Background(), job.Request{Provider: "matrix", Kind: job.Video})
			return err
		},
		func() error {
			_, err := jobs.Delete(context.Background(), job.Request{Provider: "matrix", Kind: job.Kind("other")})
			return err
		},
		func() error {
			_, err := jobs.StreamResults(context.Background(), job.Request{Provider: "matrix", Kind: job.Video})
			return err
		},
	}
	for index, call := range wrongCalls {
		if err := call(); err == nil {
			t.Fatalf("wrong-kind call %d succeeded", index)
		}
	}
	if _, err := resource.NewClient(nil); err == nil {
		t.Fatal("nil resource invoker was accepted")
	}
	if _, err := job.NewClient(nil); err == nil {
		t.Fatal("nil job invoker was accepted")
	}
}

func TestCompositeDispatchesAndRejectsIdentityOverlapAndCapabilityDrift(t *testing.T) {
	stream := &matrixStream{events: []operation.StreamEvent{{Type: operation.StreamCompleted, Sequence: 1}}}
	files := &matrixProvider{id: "p", kinds: []operation.Kind{operation.FileGet}, stream: stream}
	media := &matrixProvider{id: "p", kinds: []operation.Kind{operation.ImageGenerate}}
	composite, err := operation.NewComposite("p", media, files)
	if err != nil {
		t.Fatal(err)
	}
	if composite.ID() != "p" || !reflect.DeepEqual(composite.Kinds(), []operation.Kind{operation.FileGet, operation.ImageGenerate}) {
		t.Fatalf("unexpected composite identity/kinds: %s %v", composite.ID(), composite.Kinds())
	}
	if contract, err := composite.Capabilities(context.Background(), operation.Query{}); err != nil || len(contract) != 2 {
		t.Fatalf("composite capabilities failed: %+v %v", contract, err)
	}
	if _, err := composite.Invoke(context.Background(), operation.Request{Provider: "p", Kind: operation.ImageGenerate}); err != nil {
		t.Fatal(err)
	}
	if got, err := composite.Stream(context.Background(), operation.Request{Provider: "p", Kind: operation.FileGet}); err != nil || got != stream {
		t.Fatalf("composite stream dispatch failed: %T %v", got, err)
	}
	if _, err := composite.Invoke(context.Background(), operation.Request{Provider: "p", Kind: operation.BatchGet}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorUnsupportedCapability {
		t.Fatalf("undeclared dispatch was not rejected: %v", err)
	}

	badIdentity := &matrixProvider{id: "other", kinds: []operation.Kind{operation.FileGet}}
	unknown := &matrixProvider{id: "p", kinds: []operation.Kind{operation.Kind("unknown")}}
	empty := &matrixProvider{id: "p"}
	for index, children := range [][]operation.KindProvider{{files, files}, {badIdentity}, {unknown}, {empty}} {
		if _, err := operation.NewComposite("p", children...); err == nil {
			t.Fatalf("invalid composite %d succeeded", index)
		}
	}
	drift := &matrixProvider{id: "p", kinds: []operation.Kind{operation.FileGet}, advertised: operation.CapabilityContract{operation.BatchGet: {Level: operation.SupportNative}}}
	badComposite, _ := operation.NewComposite("p", drift)
	if _, err := badComposite.Capabilities(context.Background(), operation.Query{}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping {
		t.Fatalf("undeclared capability drift was not rejected: %v", err)
	}

	var nilComposite *operation.Composite
	if nilComposite.ID() != "" || nilComposite.Kinds() != nil {
		t.Fatal("nil composite accessors are not safe")
	}
	if _, err := nilComposite.Invoke(context.Background(), operation.Request{Kind: operation.FileGet}); err == nil {
		t.Fatal("nil composite invoke succeeded")
	}
}

func TestInvokerStreamWrapperAndErrorNormalization(t *testing.T) {
	inner := &matrixStream{events: []operation.StreamEvent{{Type: operation.StreamTextDelta, Sequence: 1, Text: "x"}}, err: errors.New("terminal")}
	provider := &matrixProvider{id: "p", kinds: []operation.Kind{operation.ImageGenerate}, stream: inner}
	registry, _ := operation.NewRegistry(provider)
	invoker, _ := operation.NewInvoker(registry)
	stream, err := invoker.Stream(context.Background(), operation.Request{Provider: "p", Kind: operation.ImageGenerate})
	if err != nil || !stream.Next() || stream.Event().Type != operation.StreamStarted || stream.Event().Result == nil || stream.Event().Result.MappingReport.Action != operation.MappingExact {
		t.Fatalf("stream wrapper did not own the semantic start: event=%+v err=%v", stream.Event(), err)
	}
	if !stream.Next() || stream.Event().Text != "x" || stream.Event().Sequence != 2 {
		t.Fatalf("stream wrapper did not re-sequence the provider event: event=%+v", stream.Event())
	}
	if stream.Next() || modelinvoker.ErrorKindOf(stream.Err()) != modelinvoker.ErrorProvider {
		t.Fatalf("stream wrapper failed: event=%+v err=%v terminal=%v", stream.Event(), err, stream.Err())
	}
	if err := stream.Close(); err != nil || !inner.closed {
		t.Fatalf("stream close failed: %v", err)
	}

	provider.stream = nil
	if _, err := invoker.Stream(context.Background(), operation.Request{Provider: "p", Kind: operation.ImageGenerate}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorStreamInterrupted {
		t.Fatalf("nil provider stream was not rejected: %v", err)
	}
	provider.streamErr = context.Canceled
	if _, err := invoker.Stream(context.Background(), operation.Request{Provider: "p", Kind: operation.ImageGenerate}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorCancelled {
		t.Fatalf("stream cancellation was not normalized: %v", err)
	}

	provider.streamErr = nil
	provider.invokeErr = errors.New("opaque")
	if _, err := invoker.Invoke(context.Background(), operation.Request{Provider: "p", Kind: operation.ImageGenerate}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorProvider {
		t.Fatalf("opaque invoke error was not normalized: %v", err)
	}
	provider.invokeErr = &modelinvoker.Error{Kind: modelinvoker.ErrorRateLimit, Message: "limited"}
	if _, err := invoker.Invoke(context.Background(), operation.Request{Provider: "p", Kind: operation.ImageGenerate}); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorRateLimit {
		t.Fatalf("typed invoke error was not preserved: %v", err)
	}
}

func TestInvokerStreamSynthesizesOneAttestedTerminalAndRejectsResultDrift(t *testing.T) {
	inner := &matrixStream{events: []operation.StreamEvent{{Type: operation.StreamTextDelta, Sequence: 44, Text: "x"}}}
	provider := &matrixProvider{id: "p", kinds: []operation.Kind{operation.ImageGenerate}, stream: inner}
	registry, _ := operation.NewRegistry(provider)
	invoker, _ := operation.NewInvoker(registry)
	request := operation.Request{Provider: "p", Kind: operation.ImageGenerate, Model: "m"}
	stream, err := invoker.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var events []operation.StreamEvent
	for stream.Next() {
		events = append(events, stream.Event())
	}
	if stream.Err() != nil || len(events) != 3 ||
		events[0].Type != operation.StreamStarted ||
		events[1].Type != operation.StreamTextDelta ||
		events[2].Type != operation.StreamCompleted {
		t.Fatalf("unexpected semantic stream lifecycle: %+v err=%v", events, stream.Err())
	}
	for index, event := range events {
		if event.Sequence != int64(index+1) {
			t.Fatalf("event %d sequence = %d", index, event.Sequence)
		}
	}
	terminal := events[2].Result
	if terminal == nil || terminal.Provider != "p" || terminal.Kind != operation.ImageGenerate ||
		terminal.Model != "m" || terminal.MappingReport.Action != operation.MappingExact {
		t.Fatalf("terminal result was not attested: %+v", terminal)
	}
	if stream.Next() {
		t.Fatal("semantic stream emitted more than one terminal event")
	}

	provider.stream = &matrixStream{events: []operation.StreamEvent{{
		Type:   operation.StreamCompleted,
		Result: &operation.Result{Provider: "other"},
	}}}
	stream, _ = invoker.Stream(context.Background(), request)
	if !stream.Next() || stream.Event().Type != operation.StreamStarted {
		t.Fatal("stream did not start")
	}
	if stream.Next() || modelinvoker.ErrorKindOf(stream.Err()) != modelinvoker.ErrorMapping {
		t.Fatalf("conflicting streamed result identity was not rejected: %v", stream.Err())
	}
}
