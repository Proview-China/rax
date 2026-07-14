// Package job provides typed lifecycle operations for asynchronous video and
// batch jobs. It delegates all transport, capability, and trust checks to the
// operation Invoker.
package job

import (
	"context"
	"fmt"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
)

type Kind string

const (
	Video Kind = "video"
	Batch Kind = "batch"
)

type Request struct {
	Provider         modelinvoker.ProviderID
	Kind             Kind
	Model            string
	ID               string
	Body             modelinvoker.RawPayload
	ContentType      string
	Query            map[string][]string
	Metadata         modelinvoker.Metadata
	IdempotencyKey   string
	Budget           operation.Budget
	AllowDegradation bool
}

type Client struct{ invoker *operation.Invoker }

func NewClient(invoker *operation.Invoker) (*Client, error) {
	if invoker == nil {
		return nil, fmt.Errorf("job operation invoker is required")
	}
	return &Client{invoker: invoker}, nil
}

func (c *Client) Create(ctx context.Context, request Request) (operation.Result, error) {
	return c.invoke(ctx, request, operation.VideoGenerate, operation.BatchCreate)
}
func (c *Client) Get(ctx context.Context, request Request) (operation.Result, error) {
	return c.invoke(ctx, request, operation.VideoGet, operation.BatchGet)
}
func (c *Client) List(ctx context.Context, request Request) (operation.Result, error) {
	if request.Kind != Batch {
		return operation.Result{}, fmt.Errorf("list is available only for batch jobs")
	}
	return c.invoker.Invoke(ctx, toOperation(request, operation.BatchList))
}
func (c *Client) Results(ctx context.Context, request Request) (operation.Result, error) {
	if request.Kind != Video {
		return operation.Result{}, fmt.Errorf("synchronous results are available only for video jobs; batch results require StreamResults")
	}
	return c.invoker.Invoke(ctx, toOperation(request, operation.VideoContent))
}
func (c *Client) StreamResults(ctx context.Context, request Request) (operation.Stream, error) {
	if c == nil || c.invoker == nil {
		return nil, fmt.Errorf("job client is not initialized")
	}
	if request.Kind != Batch {
		return nil, fmt.Errorf("stream results are available only for batch jobs")
	}
	return c.invoker.Stream(ctx, toOperation(request, operation.BatchResults))
}
func (c *Client) Cancel(ctx context.Context, request Request) (operation.Result, error) {
	if request.Kind != Batch {
		return operation.Result{}, fmt.Errorf("cancel is available only for batch jobs")
	}
	return c.invoker.Invoke(ctx, toOperation(request, operation.BatchCancel))
}
func (c *Client) Delete(ctx context.Context, request Request) (operation.Result, error) {
	switch request.Kind {
	case Video:
		return c.invoker.Invoke(ctx, toOperation(request, operation.VideoDelete))
	case Batch:
		return c.invoker.Invoke(ctx, toOperation(request, operation.BatchDelete))
	default:
		return operation.Result{}, fmt.Errorf("job kind %q is unsupported", request.Kind)
	}
}

func (c *Client) invoke(ctx context.Context, request Request, videoKind, batchKind operation.Kind) (operation.Result, error) {
	if c == nil || c.invoker == nil {
		return operation.Result{}, fmt.Errorf("job client is not initialized")
	}
	kind := videoKind
	switch request.Kind {
	case Video:
	case Batch:
		kind = batchKind
	default:
		return operation.Result{}, fmt.Errorf("job kind %q is unsupported", request.Kind)
	}
	return c.invoker.Invoke(ctx, toOperation(request, kind))
}

func toOperation(request Request, kind operation.Kind) operation.Request {
	return operation.Request{
		Provider: request.Provider, Kind: kind, Model: request.Model, ResourceID: request.ID,
		Body: request.Body, ContentType: request.ContentType, Query: cloneQuery(request.Query), Metadata: cloneMetadata(request.Metadata),
		IdempotencyKey: request.IdempotencyKey, Budget: request.Budget, AllowDegradation: request.AllowDegradation,
	}
}

func cloneQuery(input map[string][]string) map[string][]string {
	if input == nil {
		return nil
	}
	out := make(map[string][]string, len(input))
	for key, values := range input {
		out[key] = append([]string(nil), values...)
	}
	return out
}
func cloneMetadata(input modelinvoker.Metadata) modelinvoker.Metadata {
	if input == nil {
		return nil
	}
	out := make(modelinvoker.Metadata, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
