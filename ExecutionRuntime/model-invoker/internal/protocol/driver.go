package protocol

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

// Driver is the SDK-neutral invocation surface owned by a wire protocol.
// Concrete protocol packages may use provider SDK types internally, but those
// types cannot occur in this interface.
type Driver interface {
	Binding() Binding
	Invoke(context.Context, modelinvoker.Request) (modelinvoker.Response, error)
	Stream(context.Context, modelinvoker.Request) (modelinvoker.Stream, error)
}

// Dialect owns provider-specific validation, failure classification, and
// response-header metadata. Protocol drivers own wire mapping and extraction.
type Dialect interface {
	ValidateRequest(modelinvoker.Request) error
	ClassifyFailure(Failure) ErrorClassification
	ProviderMetadata(http.Header) modelinvoker.ProviderMetadata
}

// Base centralizes binding validation and identity enforcement for concrete
// protocol drivers. It is immutable after construction.
type Base struct {
	binding Binding
	dialect Dialect
}

func NewBase(binding Binding, dialect Dialect) (*Base, error) {
	if err := binding.Validate(); err != nil {
		return nil, fmt.Errorf("create protocol base: %w", err)
	}
	if IsNil(dialect) {
		return nil, fmt.Errorf("create protocol base: dialect is nil")
	}
	return &Base{binding: binding.Clone(), dialect: dialect}, nil
}

func (b *Base) Binding() Binding {
	if b == nil {
		return Binding{}
	}
	return b.binding.Clone()
}

// Validate applies binding selection checks before the provider dialect.
func (b *Base) Validate(request modelinvoker.Request) error {
	if b == nil || IsNil(b.dialect) {
		return bindingError("", modelinvoker.ErrorProviderUnavailable, "validate", "protocol base is not initialized")
	}
	if err := b.binding.ValidateRequest(request); err != nil {
		return err
	}
	if err := b.dialect.ValidateRequest(request); err != nil {
		return b.binding.StampError(nil, request, err, "validate")
	}
	return nil
}

func (b *Base) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	if b == nil || IsNil(b.dialect) {
		return nil
	}
	metadata := b.dialect.ProviderMetadata(headers.Clone())
	if metadata == nil {
		return nil
	}
	clone := make(modelinvoker.ProviderMetadata, len(metadata))
	for key, value := range metadata {
		clone[key] = value
	}
	return clone
}

func (b *Base) RequestID(headers http.Header) string {
	if b == nil {
		return ""
	}
	for _, name := range b.binding.RequestIDHeaders {
		if value := strings.TrimSpace(headers.Get(name)); value != "" && len(value) <= 1024 && !strings.ContainsAny(value, "\r\n") {
			return value
		}
	}
	return ""
}

func (b *Base) StampResponse(request modelinvoker.Request, response modelinvoker.Response) modelinvoker.Response {
	if b == nil {
		return modelinvoker.Response{Model: request.Model, Status: modelinvoker.ResponseStatusFailed}
	}
	return b.binding.StampResponse(request, response)
}

func (b *Base) StampError(ctx context.Context, request modelinvoker.Request, err error, operation string) error {
	if b == nil {
		return bindingError("", modelinvoker.ErrorProviderUnavailable, operation, "protocol base is not initialized")
	}
	return b.binding.StampError(ctx, request, err, operation)
}

func (b *Base) BindStream(ctx context.Context, request modelinvoker.Request, stream modelinvoker.Stream) modelinvoker.Stream {
	if b == nil {
		return nil
	}
	return b.binding.BindStream(ctx, request, stream)
}

// IsNil recognizes both nil interfaces and interfaces containing typed nils.
// Protocol-specific constructors use it to reject nil SDK clients before any
// request can reach a native call.
func IsNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
