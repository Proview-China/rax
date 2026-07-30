package kernel

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

const MaxContextModelInputSourceCurrentTTLV1 = 30 * time.Second

type ContextModelInputSourceCurrentReaderV1 struct {
	owner      contract.OwnerRef
	exact      contract.ContextModelInputMaterialExactReaderV1
	current    contract.ContextModelInputMaterialCurrentReaderV1
	frames     contextports.ContextFrameExactCurrentReaderV1
	frameOwner contextModelInputSourceFrameOwnerV1
	clock      func() time.Time
	maxTTL     time.Duration
}

type contextModelInputSourceFrameOwnerV1 interface {
	ContextOwnerRefV1() contract.OwnerRef
}

func NewContextModelInputSourceCurrentReaderV1(
	owner contract.OwnerRef,
	exact contract.ContextModelInputMaterialExactReaderV1,
	current contract.ContextModelInputMaterialCurrentReaderV1,
	frames contextports.ContextFrameExactCurrentReaderV1,
	clock func() time.Time,
	maxTTL time.Duration,
) (*ContextModelInputSourceCurrentReaderV1, error) {
	if owner.Validate() != nil || nilLikeContextModelInputSourceV1(exact) ||
		nilLikeContextModelInputSourceV1(current) || nilLikeContextModelInputSourceV1(frames) ||
		clock == nil || maxTTL <= 0 || maxTTL > MaxContextModelInputSourceCurrentTTLV1 {
		return nil, fmt.Errorf("%w: context model input source current reader dependencies", contract.ErrInvalid)
	}
	ownerBound, ok := frames.(contextModelInputSourceFrameOwnerV1)
	if !ok || ownerBound.ContextOwnerRefV1() != owner {
		return nil, fmt.Errorf("%w: context model input source Frame reader Owner binding", contract.ErrConflict)
	}
	return &ContextModelInputSourceCurrentReaderV1{
		owner: owner, exact: exact, current: current, frames: frames,
		frameOwner: ownerBound, clock: clock, maxTTL: maxTTL,
	}, nil
}

var _ contextports.ContextModelInputSourceCurrentReaderV1 = (*ContextModelInputSourceCurrentReaderV1)(nil)

func (r *ContextModelInputSourceCurrentReaderV1) ContextOwnerRefV1() contract.OwnerRef {
	if r == nil {
		return contract.OwnerRef{}
	}
	return r.owner
}

type contextModelInputSourceSnapshotV1 struct {
	Owner    contract.OwnerRef
	Material contract.ContextModelInputMaterialV1
	Current  contract.ContextModelInputMaterialV1
	Frame    contract.ContextFrameExactCurrentProjectionV1
}

func (r *ContextModelInputSourceCurrentReaderV1) InspectContextModelInputSourceCurrentV1(
	ctx context.Context,
	request contract.ContextModelInputSourceCurrentRequestV1,
) (contract.ContextModelInputSourceCurrentProjectionV1, error) {
	if ctx == nil {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, fmt.Errorf("%w: context model input source context", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, err
	}
	if r == nil || request.Validate() != nil || request.Owner != r.owner {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, fmt.Errorf("%w: context model input source request binding", contract.ErrConflict)
	}
	checked := r.clock()
	if checked.IsZero() {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, fmt.Errorf("%w: context model input source clock", contract.ErrInvalid)
	}
	if checked.UnixNano() < request.CheckedUnixNano {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, fmt.Errorf("%w: context model input source clock rollback", contract.ErrConflict)
	}
	if checked.UnixNano() >= request.NotAfterUnixNano {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, fmt.Errorf("%w: context model input source request lifetime", contract.ErrExpired)
	}

	s1, err := r.readContextModelInputSourceSnapshotV1(ctx, request, checked.UnixNano())
	if err != nil {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, err
	}
	s2, err := r.readContextModelInputSourceSnapshotV1(ctx, request, checked.UnixNano())
	if err != nil {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, err
	}
	if !sameContextModelInputSourceFactsV1(s1, s2) {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, fmt.Errorf("%w: context model input source S1/S2 drift", contract.ErrConflict)
	}
	expires := minContextModelInputSourceExpiryV1(
		request.NotAfterUnixNano,
		checked.Add(r.maxTTL).UnixNano(),
		s1.Material.ExpiresUnixNano,
		s1.Current.ExpiresUnixNano,
		s1.Frame.ExpiresUnixNano,
		s2.Material.ExpiresUnixNano,
		s2.Current.ExpiresUnixNano,
		s2.Frame.ExpiresUnixNano,
	)
	completed := r.clock()
	if completed.IsZero() || completed.Before(checked) {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, fmt.Errorf("%w: context model input source clock regression", contract.ErrConflict)
	}
	if completed.UnixNano() >= expires {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, fmt.Errorf("%w: context model input source TTL crossing", contract.ErrExpired)
	}
	if s2.Material.Validate() != nil || s2.Current.Validate() != nil ||
		s2.Frame.ValidateAt(completed.UnixNano()) != nil ||
		completed.UnixNano() < s2.Material.CheckedUnixNano ||
		completed.UnixNano() >= s2.Material.ExpiresUnixNano {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, fmt.Errorf("%w: context model input source completed currentness", contract.ErrConflict)
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, err
	}
	projection, err := contract.SealContextModelInputSourceCurrentProjectionV1(
		contract.ContextModelInputSourceCurrentProjectionV1{
			Owner:           r.owner,
			MaterialSource:  request.Material,
			Material:        s2.Material,
			FrameSource:     request.Frame,
			Frame:           s2.Frame,
			CheckedUnixNano: completed.UnixNano(),
			ExpiresUnixNano: expires,
		},
		completed.UnixNano(),
	)
	if err != nil {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, err
	}
	if err := projection.ValidateAgainst(request, completed.UnixNano()); err != nil {
		return contract.ContextModelInputSourceCurrentProjectionV1{}, err
	}
	return projection, nil
}

func (r *ContextModelInputSourceCurrentReaderV1) readContextModelInputSourceSnapshotV1(
	ctx context.Context,
	request contract.ContextModelInputSourceCurrentRequestV1,
	observedUnixNano int64,
) (contextModelInputSourceSnapshotV1, error) {
	if err := ctx.Err(); err != nil {
		return contextModelInputSourceSnapshotV1{}, err
	}
	if r.frameOwner.ContextOwnerRefV1() != r.owner {
		return contextModelInputSourceSnapshotV1{}, fmt.Errorf("%w: context model input source Owner drift", contract.ErrConflict)
	}
	materialRef, err := request.Material.MaterialRefV1()
	if err != nil {
		return contextModelInputSourceSnapshotV1{}, err
	}
	frameRef, err := request.Frame.FrameRefV1()
	if err != nil {
		return contextModelInputSourceSnapshotV1{}, err
	}
	material, err := r.exact.ReadContextModelInputMaterialExactV1(ctx, materialRef, observedUnixNano)
	if err != nil {
		return contextModelInputSourceSnapshotV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return contextModelInputSourceSnapshotV1{}, err
	}
	current, err := r.current.ReadContextModelInputMaterialCurrentV1(ctx, materialRef.ID, observedUnixNano)
	if err != nil {
		return contextModelInputSourceSnapshotV1{}, err
	}
	if material.Validate() != nil || current.Validate() != nil ||
		material.Ref != materialRef || current.Ref != materialRef ||
		!reflect.DeepEqual(material, current) || material.FrameRef != frameRef ||
		observedUnixNano < material.CheckedUnixNano ||
		observedUnixNano >= material.ExpiresUnixNano {
		return contextModelInputSourceSnapshotV1{}, fmt.Errorf("%w: context model input Material is not exact current", contract.ErrConflict)
	}
	if err := ctx.Err(); err != nil {
		return contextModelInputSourceSnapshotV1{}, err
	}
	frame, err := r.frames.InspectContextFrameExactCurrentV1(ctx, frameRef, observedUnixNano)
	if err != nil {
		return contextModelInputSourceSnapshotV1{}, err
	}
	frameValidationUnixNano := observedUnixNano
	if frame.CheckedUnixNano > frameValidationUnixNano {
		frameValidationUnixNano = frame.CheckedUnixNano
	}
	if frame.FrameRef != frameRef || frame.ValidateAt(frameValidationUnixNano) != nil ||
		r.frameOwner.ContextOwnerRefV1() != r.owner {
		return contextModelInputSourceSnapshotV1{}, fmt.Errorf("%w: context model input Frame is not exact current", contract.ErrConflict)
	}
	return contextModelInputSourceSnapshotV1{
		Owner: r.owner, Material: material.Clone(), Current: current.Clone(), Frame: frame,
	}, nil
}

func sameContextModelInputSourceFactsV1(left, right contextModelInputSourceSnapshotV1) bool {
	return left.Owner == right.Owner &&
		reflect.DeepEqual(left.Material, right.Material) &&
		reflect.DeepEqual(left.Current, right.Current) &&
		left.Frame.FrameRef == right.Frame.FrameRef &&
		left.Frame.Current == right.Frame.Current
}

func minContextModelInputSourceExpiryV1(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func nilLikeContextModelInputSourceV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
