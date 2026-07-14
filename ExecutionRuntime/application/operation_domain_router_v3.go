package application

import (
	"context"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OperationDomainAdapterRegistrationV3 struct {
	StepKind   runtimeports.NamespacedNameV2
	Descriptor contract.StepDescriptorRefV2
	Adapter    runtimeports.ProviderBindingRefV2
	Port       applicationports.OperationDomainStatePortV3
}

func (r OperationDomainAdapterRegistrationV3) Validate(now time.Time) error {
	if r.Port == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "operation domain adapter Port is required")
	}
	if err := (applicationports.OperationDomainResolveRequestV3{StepKind: r.StepKind, Descriptor: r.Descriptor, DomainAdapter: r.Adapter}).Validate(now); err != nil {
		return err
	}
	return r.Adapter.Validate()
}

type OperationDomainRouterV3 struct {
	mu          sync.RWMutex
	clock       func() time.Time
	currentness applicationports.OperationDomainAdapterCurrentnessPortV3
	entries     map[runtimeports.NamespacedNameV2]OperationDomainAdapterRegistrationV3
}

func NewOperationDomainRouterV3(clock func() time.Time, currentness ...applicationports.OperationDomainAdapterCurrentnessPortV3) (*OperationDomainRouterV3, error) {
	if clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "operation domain router Clock is required")
	}
	var reader applicationports.OperationDomainAdapterCurrentnessPortV3
	if len(currentness) > 1 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMismatch, "operation domain router accepts exactly one currentness reader")
	}
	if len(currentness) == 1 {
		reader = currentness[0]
	}
	return &OperationDomainRouterV3{clock: clock, currentness: reader, entries: make(map[runtimeports.NamespacedNameV2]OperationDomainAdapterRegistrationV3)}, nil
}

func (r *OperationDomainRouterV3) RegisterOperationDomainV3(ctx context.Context, registration OperationDomainAdapterRegistrationV3) error {
	if err := registration.Validate(r.clock()); err != nil {
		return err
	}
	if err := r.validateCurrentAdapterV3(ctx, registration.Adapter); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[registration.StepKind]; exists {
		return core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "operation domain StepKind already has one adapter owner")
	}
	r.entries[registration.StepKind] = registration
	return nil
}

func (r *OperationDomainRouterV3) ResolveOperationDomainV3(ctx context.Context, request applicationports.OperationDomainResolveRequestV3) (applicationports.OperationDomainStatePortV3, error) {
	if err := request.Validate(r.clock()); err != nil {
		return nil, err
	}
	r.mu.RLock()
	entry, exists := r.entries[request.StepKind]
	r.mu.RUnlock()
	if !exists {
		return nil, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "operation domain StepKind has no registered adapter")
	}
	if entry.Descriptor != request.Descriptor || entry.Adapter != request.DomainAdapter {
		return nil, core.NewError(core.ErrorConflict, core.ReasonComponentMismatch, "operation domain descriptor drifted from adapter registration")
	}
	if err := entry.Validate(r.clock()); err != nil {
		return nil, err
	}
	if err := r.validateCurrentAdapterV3(ctx, entry.Adapter); err != nil {
		return nil, err
	}
	return entry.Port, nil
}

func (r *OperationDomainRouterV3) validateCurrentAdapterV3(ctx context.Context, adapter runtimeports.ProviderBindingRefV2) error {
	if r.currentness == nil {
		return core.NewError(core.ErrorForbidden, core.ReasonComponentMissing, "operation domain adapter currentness reader is required")
	}
	current, err := r.currentness.InspectOperationDomainAdapterCurrentV3(ctx, adapter)
	if err != nil {
		return err
	}
	return current.ValidateCurrentFor(adapter, r.clock())
}

var _ applicationports.OperationDomainResolverV3 = (*OperationDomainRouterV3)(nil)
