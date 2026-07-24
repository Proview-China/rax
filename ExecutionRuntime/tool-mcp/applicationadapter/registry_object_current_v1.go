package applicationadapter

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

type RegistryObjectCurrentReaderV1 struct {
	registry *registry.Registry
	clock    ClockV1
}

func NewRegistryObjectCurrentReaderV1(source *registry.Registry, clock ClockV1) (*RegistryObjectCurrentReaderV1, error) {
	if source == nil {
		return nil, bindingInvalidV1("Tool Registry exact source is required")
	}
	if isNilFlowDependencyV1(clock) {
		return nil, bindingInvalidV1("Tool Registry current clock is required")
	}
	return &RegistryObjectCurrentReaderV1{registry: source, clock: clock}, nil
}

func (r *RegistryObjectCurrentReaderV1) ResolveExactToolCapabilityCurrentV1(ctx context.Context, object toolcontract.ObjectRef) (toolcontract.CapabilityDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	return r.readCapabilityV1(ctx, object, toolcontract.ToolRegistryObjectCurrentRefV1{})
}

func (r *RegistryObjectCurrentReaderV1) InspectExactToolCapabilityCurrentV1(ctx context.Context, object toolcontract.ObjectRef, expected toolcontract.ToolRegistryObjectCurrentRefV1) (toolcontract.CapabilityDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	if err := expected.Validate(); err != nil {
		return toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, err
	}
	if expected.Kind != toolcontract.ToolRegistryCapabilityCurrentKindV1 {
		return toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, bindingConflictV1("Tool Registry capability current Ref has another Kind")
	}
	return r.readCapabilityV1(ctx, object, expected)
}

func (r *RegistryObjectCurrentReaderV1) ResolveExactToolDescriptorCurrentV1(ctx context.Context, object toolcontract.ObjectRef) (toolcontract.ToolDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	return r.readToolV1(ctx, object, toolcontract.ToolRegistryObjectCurrentRefV1{})
}

func (r *RegistryObjectCurrentReaderV1) InspectExactToolDescriptorCurrentV1(ctx context.Context, object toolcontract.ObjectRef, expected toolcontract.ToolRegistryObjectCurrentRefV1) (toolcontract.ToolDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	if err := expected.Validate(); err != nil {
		return toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, err
	}
	if expected.Kind != toolcontract.ToolRegistryDescriptorCurrentKindV1 {
		return toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, bindingConflictV1("Tool Registry descriptor current Ref has another Kind")
	}
	return r.readToolV1(ctx, object, expected)
}

func (r *RegistryObjectCurrentReaderV1) readCapabilityV1(ctx context.Context, object toolcontract.ObjectRef, expected toolcontract.ToolRegistryObjectCurrentRefV1) (toolcontract.CapabilityDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	if err := r.readyRegistryCurrentV1(ctx); err != nil {
		return toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, err
	}
	if err := object.Validate(); err != nil {
		return toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, err
	}
	descriptor, record, ok := r.registry.ResolveCapability(object.ID)
	if !ok {
		return toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, bindingNotFoundV1("exact Tool Capability is absent")
	}
	if err := descriptor.Validate(); err != nil {
		return toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, bindingConflictV1("stored Tool Capability is non-canonical")
	}
	descriptorRef := toolcontract.ObjectRef{ID: string(descriptor.ID), Revision: descriptor.Revision, Digest: descriptor.Digest}
	if descriptorRef != object {
		return toolcontract.CapabilityDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, bindingConflictV1("exact Tool Capability object drifted")
	}
	projection, err := r.sealRegistryCurrentV1(ctx, record, descriptorRef, descriptor.Owner, expected)
	return descriptor, projection, err
}

func (r *RegistryObjectCurrentReaderV1) readToolV1(ctx context.Context, object toolcontract.ObjectRef, expected toolcontract.ToolRegistryObjectCurrentRefV1) (toolcontract.ToolDescriptor, toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	if err := r.readyRegistryCurrentV1(ctx); err != nil {
		return toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, err
	}
	if err := object.Validate(); err != nil {
		return toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, err
	}
	descriptor, record, ok := r.registry.ResolveTool(object.ID)
	if !ok {
		return toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, bindingNotFoundV1("exact Tool Descriptor is absent")
	}
	if err := descriptor.Validate(); err != nil {
		return toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, bindingConflictV1("stored Tool Descriptor is non-canonical")
	}
	descriptorRef := toolcontract.ObjectRef{ID: string(descriptor.ID), Revision: descriptor.Revision, Digest: descriptor.Digest}
	if descriptorRef != object {
		return toolcontract.ToolDescriptor{}, toolcontract.ToolRegistryObjectCurrentProjectionV1{}, bindingConflictV1("exact Tool Descriptor object drifted")
	}
	projection, err := r.sealRegistryCurrentV1(ctx, record, descriptorRef, descriptor.Owner, expected)
	return descriptor, projection, err
}

func (r *RegistryObjectCurrentReaderV1) sealRegistryCurrentV1(ctx context.Context, record registry.Record, object toolcontract.ObjectRef, owner core.OwnerRef, expected toolcontract.ToolRegistryObjectCurrentRefV1) (toolcontract.ToolRegistryObjectCurrentProjectionV1, error) {
	if err := record.Validate(); err != nil {
		return toolcontract.ToolRegistryObjectCurrentProjectionV1{}, bindingConflictV1("Tool Registry record is non-canonical")
	}
	if record.State != registry.StateActive || record.ID != object.ID || record.ObjectRevision != object.Revision || record.ObjectDigest != object.Digest {
		return toolcontract.ToolRegistryObjectCurrentProjectionV1{}, bindingConflictV1("Tool Registry record is inactive or differs from exact object")
	}
	source, err := toolcontract.SealToolRegistryRecordSourceV1(toolcontract.ToolRegistryRecordSourceV1{
		Kind: record.Kind, ID: record.ID, ObjectRevision: record.ObjectRevision, ObjectDigest: record.ObjectDigest,
		State: string(record.State), RegistryRevision: record.RegistryRevision, UpdatedUnixNano: record.UpdatedUnixNano,
	})
	if err != nil {
		return toolcontract.ToolRegistryObjectCurrentProjectionV1{}, err
	}
	if err := contextErrRegistryCurrentV1(ctx); err != nil {
		return toolcontract.ToolRegistryObjectCurrentProjectionV1{}, err
	}
	now := r.clock.Now()
	if now.IsZero() || now.UnixNano() < record.UpdatedUnixNano {
		return toolcontract.ToolRegistryObjectCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Registry current clock regressed")
	}
	projection, err := toolcontract.SealToolRegistryObjectCurrentProjectionV1(toolcontract.ToolRegistryObjectCurrentProjectionV1{
		Source: source, Object: object, RegistryOwner: owner,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(toolcontract.MaxToolRegistryObjectCurrentTTLV1).UnixNano(),
	})
	if err != nil {
		return toolcontract.ToolRegistryObjectCurrentProjectionV1{}, err
	}
	if expected != (toolcontract.ToolRegistryObjectCurrentRefV1{}) && projection.Ref != expected {
		return toolcontract.ToolRegistryObjectCurrentProjectionV1{}, bindingConflictV1("Tool Registry exact current Ref drifted")
	}
	if err := contextErrRegistryCurrentV1(ctx); err != nil {
		return toolcontract.ToolRegistryObjectCurrentProjectionV1{}, err
	}
	final := r.clock.Now()
	if final.Before(now) {
		return toolcontract.ToolRegistryObjectCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Registry current clock regressed after read")
	}
	if err := projection.ValidateCurrent(final); err != nil {
		return toolcontract.ToolRegistryObjectCurrentProjectionV1{}, err
	}
	return projection, nil
}

func (r *RegistryObjectCurrentReaderV1) readyRegistryCurrentV1(ctx context.Context) error {
	if err := contextErrRegistryCurrentV1(ctx); err != nil {
		return err
	}
	if r == nil || r.registry == nil || isNilFlowDependencyV1(r.clock) {
		return bindingUnavailableV1("Tool Registry current Reader is unavailable")
	}
	return nil
}

func contextErrRegistryCurrentV1(ctx context.Context) error {
	if ctx == nil {
		return bindingInvalidV1("Tool Registry current context is required")
	}
	return ctx.Err()
}

var _ toolcontract.ToolRegistryObjectCurrentReaderV1 = (*RegistryObjectCurrentReaderV1)(nil)

func bindingNotFoundV1(message string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, message)
}
