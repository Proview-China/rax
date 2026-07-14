package operation

import (
	"context"
	"sort"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

// KindProvider declares the exact operation kinds owned by a provider
// transport. Composite uses it to combine specialized transports without
// inferring routes or allowing overlapping implementations.
type KindProvider interface {
	Provider
	Kinds() []Kind
}

type Composite struct {
	id       modelinvoker.ProviderID
	byKind   map[Kind]KindProvider
	children []KindProvider
}

func NewComposite(id modelinvoker.ProviderID, providers ...KindProvider) (*Composite, error) {
	if id == "" || len(providers) == 0 {
		return nil, operationError(id, modelinvoker.ErrorInvalidRequest, "compose", "", "composite provider ID and children are required")
	}
	c := &Composite{id: id, byKind: map[Kind]KindProvider{}, children: append([]KindProvider(nil), providers...)}
	for _, provider := range providers {
		if nilProvider(provider) || provider.ID() != id {
			return nil, operationError(id, modelinvoker.ErrorMapping, "compose", "", "composite child identity does not match")
		}
		for _, kind := range provider.Kinds() {
			if _, known := knownKinds[kind]; !known {
				return nil, operationError(id, modelinvoker.ErrorInvalidRequest, "compose", string(kind), "composite child declared an unknown operation")
			}
			if _, exists := c.byKind[kind]; exists {
				return nil, operationError(id, modelinvoker.ErrorDuplicateProvider, "compose", string(kind), "composite children overlap an operation")
			}
			c.byKind[kind] = provider
		}
	}
	if len(c.byKind) == 0 {
		return nil, operationError(id, modelinvoker.ErrorInvalidRequest, "compose", "", "composite provider has no operations")
	}
	return c, nil
}

func (c *Composite) ID() modelinvoker.ProviderID {
	if c == nil {
		return ""
	}
	return c.id
}
func (c *Composite) Kinds() []Kind {
	if c == nil {
		return nil
	}
	kinds := make([]Kind, 0, len(c.byKind))
	for kind := range c.byKind {
		kinds = append(kinds, kind)
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })
	return kinds
}
func (c *Composite) Capabilities(ctx context.Context, query Query) (CapabilityContract, error) {
	if c == nil {
		return nil, operationError("", modelinvoker.ErrorProviderUnavailable, "capabilities", "", "composite provider is not initialized")
	}
	out := CapabilityContract{}
	for _, child := range c.children {
		declared := make(map[Kind]struct{}, len(child.Kinds()))
		for _, kind := range child.Kinds() {
			declared[kind] = struct{}{}
		}
		contract, err := child.Capabilities(ctx, query)
		if err != nil {
			return nil, err
		}
		for kind, capability := range contract {
			if _, ok := declared[kind]; !ok {
				return nil, operationError(c.id, modelinvoker.ErrorMapping, "capabilities", string(kind), "composite child advertised an undeclared operation")
			}
			out[kind] = capability
		}
	}
	return out, nil
}
func (c *Composite) Invoke(ctx context.Context, request Request) (Result, error) {
	child, err := c.child(request.Kind)
	if err != nil {
		return Result{}, err
	}
	return child.Invoke(ctx, request)
}
func (c *Composite) Stream(ctx context.Context, request Request) (Stream, error) {
	child, err := c.child(request.Kind)
	if err != nil {
		return nil, err
	}
	return child.Stream(ctx, request)
}
func (c *Composite) child(kind Kind) (KindProvider, error) {
	if c == nil {
		return nil, operationError("", modelinvoker.ErrorProviderUnavailable, "dispatch", string(kind), "composite provider is not initialized")
	}
	child, ok := c.byKind[kind]
	if !ok {
		return nil, operationError(c.id, modelinvoker.ErrorUnsupportedCapability, "dispatch", string(kind), "operation is not configured")
	}
	return child, nil
}

var _ KindProvider = (*Composite)(nil)
