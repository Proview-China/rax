package direct

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
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

	ToolCallObservationRepository modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1
	GovernedInvocationBindings    modelinvoker.GovernedModelInvocationBindingReaderV1
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
	if config.ToolCallObservationRepository != nil && projectionRepositoryUnavailableV1(config.ToolCallObservationRepository) {
		return fmt.Errorf("%w: tool call observation repository is typed-nil", ErrInvalidConfig)
	}
	if config.GovernedInvocationBindings != nil && nilInterfaceDirectV1(config.GovernedInvocationBindings) {
		return fmt.Errorf("%w: governed invocation Binding Reader is typed-nil", ErrInvalidConfig)
	}
	if config.GovernedInvocationBindings != nil {
		if _, ok := config.Backend.(GovernedBackendV1); !ok {
			return fmt.Errorf("%w: governed invocation Binding Reader requires a governed Backend", ErrInvalidConfig)
		}
	}
	return nil
}

func projectionRepositoryUnavailableV1(repository modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1) bool {
	return modelinvoker.IsToolCallCandidateObservationProjectionRepositoryUnavailableV1(repository)
}

type Adapter struct {
	config           Config
	governedMu       sync.Mutex
	governedPrepared map[union.ExecutionID]modelinvoker.GovernedModelInvocationBindingV1
}

func New(config Config) (*Adapter, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Adapter{config: config, governedPrepared: make(map[union.ExecutionID]modelinvoker.GovernedModelInvocationBindingV1)}, nil
}

func nilInterfaceDirectV1(value any) bool {
	if value == nil {
		return true
	}
	kind := reflect.ValueOf(value).Kind()
	switch kind {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflect.ValueOf(value).IsNil()
	}
	return false
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
