package kernel

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
)

const MaxContextModelInputLineageCurrentTTLV1 = 30 * time.Second

type ContextModelInputLineageCurrentReaderV1 struct {
	owner   contract.OwnerRef
	exact   contract.ContextModelInputMaterialExactReaderV1
	current contract.ContextModelInputMaterialCurrentReaderV1
	frames  contextports.ContextFrameExactCurrentReaderV1
	clock   func() time.Time
	maxTTL  time.Duration
}

type contextFrameOwnerBoundReaderV1 interface {
	ContextOwnerRefV1() contract.OwnerRef
}

func NewContextModelInputLineageCurrentReaderV1(
	owner contract.OwnerRef,
	exact contract.ContextModelInputMaterialExactReaderV1,
	current contract.ContextModelInputMaterialCurrentReaderV1,
	frames contextports.ContextFrameExactCurrentReaderV1,
	clock func() time.Time,
	maxTTL time.Duration,
) (*ContextModelInputLineageCurrentReaderV1, error) {
	if owner.Validate() != nil || nilLikeModelInputLineageV1(exact) || nilLikeModelInputLineageV1(current) || nilLikeModelInputLineageV1(frames) || clock == nil || maxTTL <= 0 || maxTTL > MaxContextModelInputLineageCurrentTTLV1 {
		return nil, fmt.Errorf("%w: context model input lineage current reader dependencies", contract.ErrInvalid)
	}
	ownerBound, ok := frames.(contextFrameOwnerBoundReaderV1)
	if !ok || ownerBound.ContextOwnerRefV1() != owner {
		return nil, fmt.Errorf("%w: context model input lineage Frame reader Owner binding", contract.ErrConflict)
	}
	return &ContextModelInputLineageCurrentReaderV1{
		owner: owner, exact: exact, current: current, frames: frames, clock: clock, maxTTL: maxTTL,
	}, nil
}

var _ contextports.ContextModelInputLineageCurrentReaderV1 = (*ContextModelInputLineageCurrentReaderV1)(nil)

type contextModelInputLineageSnapshotV1 struct {
	Material contract.ContextModelInputMaterialV1          `json:"material"`
	Current  contract.ContextModelInputMaterialV1          `json:"current"`
	Frame    contract.ContextFrameExactCurrentProjectionV1 `json:"frame"`
}

func (r *ContextModelInputLineageCurrentReaderV1) InspectContextModelInputLineageCurrentV1(ctx context.Context, request contract.ContextModelInputLineageCurrentRequestV1) (contract.ContextModelInputLineageCurrentProjectionV1, error) {
	if ctx == nil {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, fmt.Errorf("%w: context model input lineage request", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, err
	}
	if r == nil || request.Validate() != nil || request.Source.Owner != r.owner || request.Source.Kind != contract.ContextInvocationSourceModelInputMaterialV1 {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, fmt.Errorf("%w: context model input lineage request binding", contract.ErrConflict)
	}
	checked := r.clock()
	if checked.IsZero() {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, fmt.Errorf("%w: context model input lineage clock", contract.ErrInvalid)
	}
	if checked.UnixNano() < request.CheckedUnixNano {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, fmt.Errorf("%w: context model input lineage clock rollback", contract.ErrConflict)
	}
	if checked.UnixNano() >= request.NotAfterUnixNano {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, fmt.Errorf("%w: context model input lineage request lifetime", contract.ErrExpired)
	}

	s1, err := r.readSnapshot(ctx, request.Source, checked.UnixNano())
	if err != nil {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, err
	}
	expires := minModelInputLineageExpiryV1(
		request.NotAfterUnixNano,
		checked.Add(r.maxTTL).UnixNano(),
		s1.Material.ExpiresUnixNano,
		s1.Current.ExpiresUnixNano,
		s1.Frame.ExpiresUnixNano,
	)
	if checked.UnixNano() >= expires {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, fmt.Errorf("%w: context model input lineage S1 lifetime", contract.ErrExpired)
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, err
	}

	s2, err := r.readSnapshot(ctx, request.Source, checked.UnixNano())
	if err != nil {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, err
	}
	s1Digest, err := contract.DigestJSON(s1)
	if err != nil {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, err
	}
	s2Digest, err := contract.DigestJSON(s2)
	if err != nil {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, err
	}
	if s1Digest != s2Digest {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, fmt.Errorf("%w: context model input lineage S1/S2 drift", contract.ErrConflict)
	}
	expires2 := minModelInputLineageExpiryV1(
		request.NotAfterUnixNano,
		checked.Add(r.maxTTL).UnixNano(),
		s2.Material.ExpiresUnixNano,
		s2.Current.ExpiresUnixNano,
		s2.Frame.ExpiresUnixNano,
	)
	if expires2 != expires {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, fmt.Errorf("%w: context model input lineage TTL drift", contract.ErrConflict)
	}
	completed := r.clock()
	if completed.IsZero() || completed.Before(checked) {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, fmt.Errorf("%w: context model input lineage clock regression", contract.ErrConflict)
	}
	if completed.UnixNano() >= expires {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, fmt.Errorf("%w: context model input lineage TTL crossing", contract.ErrExpired)
	}
	if s2.Material.Validate() != nil || s2.Current.Validate() != nil || s2.Material.Ref != s2.Current.Ref || s2.Material.FrameRef != s2.Frame.FrameRef || s2.Frame.ValidateAt(completed.UnixNano()) != nil {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, fmt.Errorf("%w: context model input lineage completed currentness", contract.ErrConflict)
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, err
	}

	materialSource, err := contract.ContextModelInputMaterialExactSourceV1(r.owner, s2.Material.Ref)
	if err != nil {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, err
	}
	frameSource, err := contract.ContextFrameExactSourceV1(r.owner, s2.Material.FrameRef)
	if err != nil {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, err
	}
	if materialSource.Digest == frameSource.Digest {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, fmt.Errorf("%w: context model input lineage source digest collision", contract.ErrConflict)
	}
	projection, err := contract.SealContextModelInputLineageCurrentProjectionV1(contract.ContextModelInputLineageCurrentProjectionV1{
		Material: materialSource, Frame: frameSource, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires,
	}, completed.UnixNano())
	if err != nil {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, err
	}
	if err := projection.ValidateAgainst(request, completed.UnixNano()); err != nil {
		return contract.ContextModelInputLineageCurrentProjectionV1{}, err
	}
	return projection, nil
}

func (r *ContextModelInputLineageCurrentReaderV1) readSnapshot(ctx context.Context, source contract.ContextInvocationExactSourceRefV1, observedUnixNano int64) (contextModelInputLineageSnapshotV1, error) {
	if err := ctx.Err(); err != nil {
		return contextModelInputLineageSnapshotV1{}, err
	}
	materialRef, err := source.MaterialRefV1()
	if err != nil {
		return contextModelInputLineageSnapshotV1{}, err
	}
	material, err := r.exact.ReadContextModelInputMaterialExactV1(ctx, materialRef, observedUnixNano)
	if err != nil {
		return contextModelInputLineageSnapshotV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return contextModelInputLineageSnapshotV1{}, err
	}
	current, err := r.current.ReadContextModelInputMaterialCurrentV1(ctx, materialRef.ID, observedUnixNano)
	if err != nil {
		return contextModelInputLineageSnapshotV1{}, err
	}
	if material.Validate() != nil || current.Validate() != nil || material.Ref != materialRef || current.Ref != materialRef || !reflect.DeepEqual(material, current) {
		return contextModelInputLineageSnapshotV1{}, fmt.Errorf("%w: context model input material is not exact current", contract.ErrConflict)
	}
	if err := ctx.Err(); err != nil {
		return contextModelInputLineageSnapshotV1{}, err
	}
	frame, err := r.frames.InspectContextFrameExactCurrentV1(ctx, material.FrameRef, observedUnixNano)
	if err != nil {
		return contextModelInputLineageSnapshotV1{}, err
	}
	if frame.ValidateAt(observedUnixNano) != nil || frame.FrameRef != material.FrameRef {
		return contextModelInputLineageSnapshotV1{}, fmt.Errorf("%w: context model input frame is not exact current", contract.ErrConflict)
	}
	return contextModelInputLineageSnapshotV1{Material: material.Clone(), Current: current.Clone(), Frame: frame}, nil
}

func minModelInputLineageExpiryV1(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func nilLikeModelInputLineageV1(value any) bool {
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
