package modelinvokeradapter

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// InvocationContextOwnerBindingAdapterV1 projects the authoritative Context
// model-input lineage into Model's owner-neutral public contract. It never
// writes Context or Model facts.
type InvocationContextOwnerBindingAdapterV1 struct {
	owner   contract.OwnerRef
	lineage contextports.ContextModelInputLineageCurrentReaderV1
	clock   func() time.Time
}

func NewInvocationContextOwnerBindingAdapterV1(
	owner contract.OwnerRef,
	lineage contextports.ContextModelInputLineageCurrentReaderV1,
	clock func() time.Time,
) (*InvocationContextOwnerBindingAdapterV1, error) {
	if owner.Validate() != nil || nilLikeContextOwnerBindingV1(lineage) || clock == nil {
		return nil, fmt.Errorf("%w: invocation context owner binding adapter dependencies", contract.ErrInvalid)
	}
	return &InvocationContextOwnerBindingAdapterV1{
		owner: owner, lineage: lineage, clock: clock,
	}, nil
}

var _ modelinvoker.InvocationContextOwnerBindingReaderV1 = (*InvocationContextOwnerBindingAdapterV1)(nil)

func (a *InvocationContextOwnerBindingAdapterV1) InspectCurrentInvocationContextOwnerBindingV1(
	ctx context.Context,
	request modelinvoker.InvocationContextOwnerBindingRequestV1,
) (modelinvoker.InvocationContextOwnerBindingProjectionV1, error) {
	if ctx == nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, fmt.Errorf("%w: invocation context owner binding context", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, err
	}
	if a == nil || a.owner.Validate() != nil || nilLikeContextOwnerBindingV1(a.lineage) || a.clock == nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, fmt.Errorf("%w: invocation context owner binding adapter", contract.ErrUnavailable)
	}
	if request.Validate() != nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, fmt.Errorf("%w: invocation context owner binding request", contract.ErrInvalid)
	}
	if request.MaterialLookup.Kind != string(contract.ContextInvocationSourceModelInputMaterialV1) {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, fmt.Errorf("%w: invocation context material kind", contract.ErrConflict)
	}

	contextRequest, err := a.contextRequestV1(request)
	if err != nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, err
	}
	started := a.clock()
	if started.IsZero() {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, fmt.Errorf("%w: invocation context owner binding clock", contract.ErrInvalid)
	}
	if started.UnixNano() < request.CheckedUnixNano {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, fmt.Errorf("%w: invocation context owner binding clock rollback", contract.ErrConflict)
	}
	if started.UnixNano() >= request.NotAfterUnixNano {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, fmt.Errorf("%w: invocation context owner binding request lifetime", contract.ErrExpired)
	}

	s1, err := a.lineage.InspectContextModelInputLineageCurrentV1(ctx, contextRequest)
	if err != nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, err
	}
	observed := a.clock()
	if err := validateAdapterClockV1(started, observed, request.NotAfterUnixNano); err != nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, err
	}
	if err := a.validateAuthoritativeProjectionV1(contextRequest, s1, observed); err != nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, err
	}

	s2, err := a.lineage.InspectContextModelInputLineageCurrentV1(ctx, contextRequest)
	if err != nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, err
	}
	completed := a.clock()
	if err := validateAdapterClockV1(observed, completed, request.NotAfterUnixNano); err != nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, err
	}
	if err := a.validateAuthoritativeProjectionV1(contextRequest, s2, completed); err != nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, err
	}
	if s1 != s2 {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, fmt.Errorf("%w: invocation context lineage S1/S2 drift", contract.ErrConflict)
	}
	if err := ctx.Err(); err != nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, err
	}

	expires := minContextOwnerBindingExpiryV1(
		request.NotAfterUnixNano,
		s1.ExpiresUnixNano,
		s2.ExpiresUnixNano,
	)
	if completed.UnixNano() >= expires {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, fmt.Errorf("%w: invocation context owner binding TTL crossing", contract.ErrExpired)
	}

	contextOwner := modelinvoker.ContextOwnerRef{
		ComponentID:   s2.Material.Owner.ComponentID,
		BindingDigest: core.Digest(s2.Material.Owner.BindingDigest),
	}
	_, neutralOwner, err := modelinvoker.MapContextOwnerRefToNeutralOwnerV1(contextOwner)
	if err != nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, err
	}
	material, err := modelExactSourceV1(s2.Material, neutralOwner)
	if err != nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, err
	}
	frame, err := modelExactSourceV1(s2.Frame, neutralOwner)
	if err != nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, err
	}

	projection, err := modelinvoker.SealInvocationContextOwnerBindingProjectionV1(
		modelinvoker.InvocationContextOwnerBindingProjectionV1{
			ContextOwner:         contextOwner,
			Material:             material,
			Frame:                frame,
			ContextLineageDigest: core.Digest(s2.Digest),
			CheckedUnixNano:      completed.UnixNano(),
			ExpiresUnixNano:      expires,
		},
		request,
		completed,
	)
	if err != nil {
		return modelinvoker.InvocationContextOwnerBindingProjectionV1{}, err
	}
	return projection, nil
}

func (a *InvocationContextOwnerBindingAdapterV1) contextRequestV1(
	request modelinvoker.InvocationContextOwnerBindingRequestV1,
) (contract.ContextModelInputLineageCurrentRequestV1, error) {
	material := contract.ContextModelInputMaterialRefV1{
		ID:       request.MaterialLookup.ID,
		Revision: uint64(request.MaterialLookup.Revision),
		Digest:   contract.Digest(request.MaterialLookup.Digest),
	}
	source, err := contract.ContextModelInputMaterialExactSourceV1(a.owner, material)
	if err != nil {
		return contract.ContextModelInputLineageCurrentRequestV1{}, err
	}
	return contract.SealContextModelInputLineageCurrentRequestV1(
		contract.ContextModelInputLineageCurrentRequestV1{
			Source:           source,
			CheckedUnixNano:  request.CheckedUnixNano,
			NotAfterUnixNano: request.NotAfterUnixNano,
		},
	)
}

func (a *InvocationContextOwnerBindingAdapterV1) validateAuthoritativeProjectionV1(
	request contract.ContextModelInputLineageCurrentRequestV1,
	projection contract.ContextModelInputLineageCurrentProjectionV1,
	now time.Time,
) error {
	if now.IsZero() {
		return fmt.Errorf("%w: authoritative context lineage clock", contract.ErrInvalid)
	}
	if now.UnixNano() < projection.CheckedUnixNano {
		return fmt.Errorf("%w: authoritative context lineage clock rollback", contract.ErrConflict)
	}
	if now.UnixNano() >= projection.ExpiresUnixNano {
		return fmt.Errorf("%w: authoritative context lineage lifetime", contract.ErrExpired)
	}
	if projection.ValidateAgainst(request, now.UnixNano()) != nil {
		return fmt.Errorf("%w: authoritative context lineage projection", contract.ErrConflict)
	}
	if projection.Material.Owner != a.owner ||
		projection.Frame.Owner != a.owner ||
		projection.Material.Kind != contract.ContextInvocationSourceModelInputMaterialV1 ||
		projection.Frame.Kind != contract.ContextInvocationSourceFrameV1 ||
		projection.Material != request.Source {
		return fmt.Errorf("%w: authoritative context lineage coordinate", contract.ErrConflict)
	}
	return nil
}

func modelExactSourceV1(
	source contract.ContextInvocationExactSourceRefV1,
	neutralOwner core.OwnerRef,
) (modelinvoker.InvocationMaterialExactSourceRefV1, error) {
	mapped := modelinvoker.InvocationMaterialExactSourceRefV1{
		Owner:    neutralOwner,
		Kind:     string(source.Kind),
		ID:       source.ID,
		Revision: core.Revision(source.Revision),
		Digest:   core.Digest(source.Digest),
	}
	if mapped.Validate() != nil ||
		mapped.Kind != string(source.Kind) ||
		mapped.ID != source.ID ||
		uint64(mapped.Revision) != source.Revision ||
		contract.Digest(mapped.Digest) != source.Digest {
		return modelinvoker.InvocationMaterialExactSourceRefV1{}, fmt.Errorf("%w: lossless context exact source mapping", contract.ErrConflict)
	}
	return mapped, nil
}

func validateAdapterClockV1(previous, current time.Time, notAfterUnixNano int64) error {
	if current.IsZero() {
		return fmt.Errorf("%w: invocation context owner binding clock", contract.ErrInvalid)
	}
	if current.Before(previous) {
		return fmt.Errorf("%w: invocation context owner binding clock rollback", contract.ErrConflict)
	}
	if current.UnixNano() >= notAfterUnixNano {
		return fmt.Errorf("%w: invocation context owner binding request lifetime", contract.ErrExpired)
	}
	return nil
}

func minContextOwnerBindingExpiryV1(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func nilLikeContextOwnerBindingV1(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}
