package direct

import (
	"context"
	"fmt"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

type Config struct {
	Identity   union.VersionedIdentity
	Backend    Backend
	RouteID    upstream.RouteID
	Invocation upstream.InvocationContext
	Model      string
}

func (config Config) validate() error {
	if err := config.Identity.Validate("direct.identity"); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if config.Backend == nil || strings.TrimSpace(string(config.RouteID)) == "" || strings.TrimSpace(config.Model) == "" {
		return fmt.Errorf("%w: backend, RouteID and exact model are required", ErrInvalidConfig)
	}
	if config.Invocation == (upstream.InvocationContext{}) {
		return fmt.Errorf("%w: an explicit invocation context is required", ErrInvalidConfig)
	}
	return nil
}

type Adapter struct {
	config Config
}

func New(config Config) (*Adapter, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Adapter{config: config}, nil
}

func (adapter *Adapter) Describe(_ context.Context) (execution.AdapterDescriptor, error) {
	if adapter == nil {
		return execution.AdapterDescriptor{}, ErrInvalidConfig
	}
	return execution.AdapterDescriptor{
		Identity: adapter.config.Identity, Origin: union.EventOriginProvider,
		ExecutionKinds: []union.ExecutionKind{union.ExecutionKindModel},
	}, nil
}
