// Package resource provides typed convenience operations for provider-managed
// files and retrieval stores. It delegates transport and policy enforcement to
// the operation Invoker instead of creating a second HTTP stack.
package resource

import (
	"context"
	"fmt"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
)

type Kind string

const (
	File  Kind = "file"
	Store Kind = "store"
)

type Request struct {
	Provider       modelinvoker.ProviderID
	Kind           Kind
	Model          string
	ID             string
	ParentID       string
	Body           modelinvoker.RawPayload
	ContentType    string
	Query          map[string][]string
	Metadata       modelinvoker.Metadata
	IdempotencyKey string
	Budget         operation.Budget
}

type Client struct{ invoker *operation.Invoker }

func NewClient(invoker *operation.Invoker) (*Client, error) {
	if invoker == nil {
		return nil, fmt.Errorf("resource operation invoker is required")
	}
	return &Client{invoker: invoker}, nil
}

func (c *Client) Create(ctx context.Context, request Request) (operation.Result, error) {
	return c.invoke(ctx, request, operation.FileCreate, operation.StoreCreate)
}
func (c *Client) List(ctx context.Context, request Request) (operation.Result, error) {
	return c.invoke(ctx, request, operation.FileList, operation.StoreList)
}
func (c *Client) Get(ctx context.Context, request Request) (operation.Result, error) {
	return c.invoke(ctx, request, operation.FileGet, operation.StoreGet)
}
func (c *Client) Delete(ctx context.Context, request Request) (operation.Result, error) {
	return c.invoke(ctx, request, operation.FileDelete, operation.StoreDelete)
}
func (c *Client) Content(ctx context.Context, request Request) (operation.Result, error) {
	if c == nil || c.invoker == nil {
		return operation.Result{}, fmt.Errorf("resource client is not initialized")
	}
	if request.Kind != File {
		return operation.Result{}, fmt.Errorf("content is available only for file resources")
	}
	return c.invoker.Invoke(ctx, toOperation(request, operation.FileContent))
}
func (c *Client) Search(ctx context.Context, request Request) (operation.Result, error) {
	if c == nil || c.invoker == nil {
		return operation.Result{}, fmt.Errorf("resource client is not initialized")
	}
	if request.Kind != Store {
		return operation.Result{}, fmt.Errorf("search is available only for store resources")
	}
	return c.invoker.Invoke(ctx, toOperation(request, operation.StoreSearch))
}

func (c *Client) invoke(ctx context.Context, request Request, fileKind, storeKind operation.Kind) (operation.Result, error) {
	if c == nil || c.invoker == nil {
		return operation.Result{}, fmt.Errorf("resource client is not initialized")
	}
	kind := fileKind
	switch request.Kind {
	case File:
	case Store:
		kind = storeKind
	default:
		return operation.Result{}, fmt.Errorf("resource kind %q is unsupported", request.Kind)
	}
	return c.invoker.Invoke(ctx, toOperation(request, kind))
}

func toOperation(request Request, kind operation.Kind) operation.Request {
	return operation.Request{
		Provider: request.Provider, Kind: kind, Model: request.Model, ResourceID: request.ID, ParentID: request.ParentID,
		Body: request.Body, ContentType: request.ContentType, Query: cloneQuery(request.Query), Metadata: cloneMetadata(request.Metadata),
		IdempotencyKey: request.IdempotencyKey, Budget: request.Budget,
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
