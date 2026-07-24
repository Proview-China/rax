package kernel

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// WorkspaceRewindComposerV1 is the Sandbox Owner boundary for turning an
// exact keep/drop selection into a new staged ChangeSet. It performs no
// filesystem I/O and grants no Runtime, Review, Fence, or Provider authority.
type WorkspaceRewindComposerV1 struct {
	workspace    ports.WorkspaceOwnerStoreV1
	compositions ports.WorkspaceRewindCompositionRepositoryV1
	clock        func() time.Time
}

func NewWorkspaceRewindComposerV1(
	workspace ports.WorkspaceOwnerStoreV1,
	compositions ports.WorkspaceRewindCompositionRepositoryV1,
	clock func() time.Time,
) (*WorkspaceRewindComposerV1, error) {
	if rewindNilLikeV1(workspace) || rewindNilLikeV1(compositions) || clock == nil {
		return nil, errors.New("workspace rewind composer requires Owner stores and clock")
	}
	return &WorkspaceRewindComposerV1{workspace: workspace, compositions: compositions, clock: clock}, nil
}

func (c *WorkspaceRewindComposerV1) ComposeWorkspaceRewindV1(ctx context.Context, request contract.ComposeWorkspaceRewindRequestV1) (ports.WorkspaceRewindCompositionResultV1, error) {
	if c == nil || rewindNilLikeV1(ctx) {
		return ports.WorkspaceRewindCompositionResultV1{}, errors.New("workspace rewind composer or context is nil")
	}
	if err := request.ValidateShape(); err != nil {
		return ports.WorkspaceRewindCompositionResultV1{}, err
	}
	if existing, err := c.inspectWorkspaceRewindV1(ctx, request); err == nil {
		return existing, nil
	} else if !errors.Is(err, ports.ErrNotFound) {
		return ports.WorkspaceRewindCompositionResultV1{}, err
	}

	now := c.clock()
	if err := request.ValidateCurrent(now); err != nil {
		return ports.WorkspaceRewindCompositionResultV1{}, err
	}
	view, keep, drop, err := c.inspectWorkspaceRewindInputsV1(ctx, request)
	if err != nil {
		return ports.WorkspaceRewindCompositionResultV1{}, err
	}
	changes, err := contract.ComposeWorkspaceRewindChangesV1(view, keep, drop)
	if err != nil {
		return ports.WorkspaceRewindCompositionResultV1{}, err
	}
	expires := workspaceRewindExpiryV1(request, view, keep, drop)
	planned, err := StageWorkspaceChangeSet(now, expires, request.PlannedChangeSetID, view, changes)
	if err != nil {
		return ports.WorkspaceRewindCompositionResultV1{}, err
	}
	if _, err := c.workspace.CreateWorkspaceChangeSetV1(ctx, planned); err != nil {
		recovered, inspectErr := c.workspace.InspectWorkspaceChangeSetHistoryV1(context.WithoutCancel(ctx), planned.Meta.Ref())
		if inspectErr != nil || !sameWorkspaceChangeSetV1(recovered, planned) {
			return ports.WorkspaceRewindCompositionResultV1{}, err
		}
		planned = recovered
	}
	fact, err := contract.NewWorkspaceRewindCompositionFactV1(now, expires, request, view, planned)
	if err != nil {
		return ports.WorkspaceRewindCompositionResultV1{}, err
	}
	created, err := c.compositions.CreateWorkspaceRewindCompositionV1(ctx, fact)
	if err != nil {
		recovered, inspectErr := c.inspectWorkspaceRewindV1(context.WithoutCancel(ctx), request)
		if inspectErr != nil || !contract.SameWorkspaceRewindCompositionV1(recovered.Composition, fact) || !sameWorkspaceChangeSetV1(recovered.ChangeSet, planned) {
			return ports.WorkspaceRewindCompositionResultV1{}, err
		}
		return recovered, nil
	}
	return ports.WorkspaceRewindCompositionResultV1{Composition: created, ChangeSet: planned}, nil
}

func (c *WorkspaceRewindComposerV1) InspectWorkspaceRewindV1(ctx context.Context, request contract.ComposeWorkspaceRewindRequestV1) (ports.WorkspaceRewindCompositionResultV1, error) {
	if c == nil || rewindNilLikeV1(ctx) {
		return ports.WorkspaceRewindCompositionResultV1{}, errors.New("workspace rewind composer or context is nil")
	}
	if err := request.ValidateShape(); err != nil {
		return ports.WorkspaceRewindCompositionResultV1{}, err
	}
	return c.inspectWorkspaceRewindV1(ctx, request)
}

func (c *WorkspaceRewindComposerV1) inspectWorkspaceRewindV1(ctx context.Context, request contract.ComposeWorkspaceRewindRequestV1) (ports.WorkspaceRewindCompositionResultV1, error) {
	// The request deliberately carries no caller-selected Tenant/current
	// projection. Derive the repository coordinates from the exact historical
	// source View; this does not grant currentness or re-read mutable input.
	view, err := c.workspace.InspectWorkspaceViewHistoryV1(ctx, request.SourceWorkspaceViewRef)
	if err != nil {
		return ports.WorkspaceRewindCompositionResultV1{}, err
	}
	fact, err := c.compositions.InspectWorkspaceRewindCompositionByRequestV1(ctx, view.Lease.TenantID, view.Lease.ScopeDigest, request.RequestID)
	if err != nil {
		return ports.WorkspaceRewindCompositionResultV1{}, err
	}
	if fact.RequestDigest != request.Digest || fact.IdempotencyKey != request.IdempotencyKey ||
		fact.SourceWorkspaceViewRef != request.SourceWorkspaceViewRef ||
		fact.ExpectedBaseRevision != request.ExpectedBaseRevision ||
		fact.ExpectedFileScopeDigest != request.ExpectedFileScopeDigest ||
		!reflect.DeepEqual(fact.KeepChangeSetRefs, request.KeepChangeSetRefs) ||
		!reflect.DeepEqual(fact.DropChangeSetRefs, request.DropChangeSetRefs) ||
		fact.PlannedChangeSetRef.ID != request.PlannedChangeSetID {
		return ports.WorkspaceRewindCompositionResultV1{}, ports.ErrConflict
	}
	planned, err := c.workspace.InspectWorkspaceChangeSetHistoryV1(ctx, fact.PlannedChangeSetRef)
	if err != nil {
		return ports.WorkspaceRewindCompositionResultV1{}, err
	}
	return ports.WorkspaceRewindCompositionResultV1{Composition: fact, ChangeSet: planned}, nil
}

func (c *WorkspaceRewindComposerV1) inspectWorkspaceRewindInputsV1(ctx context.Context, request contract.ComposeWorkspaceRewindRequestV1) (contract.WorkspaceView, []contract.WorkspaceChangeSet, []contract.WorkspaceChangeSet, error) {
	view, err := c.workspace.InspectWorkspaceViewCurrentV1(ctx, request.SourceWorkspaceViewRef)
	if err != nil {
		return contract.WorkspaceView{}, nil, nil, err
	}
	if view.BaseRevision != request.ExpectedBaseRevision || view.FileScopeDigest != request.ExpectedFileScopeDigest {
		return contract.WorkspaceView{}, nil, nil, ports.ErrConflict
	}
	read := func(refs []contract.Ref) ([]contract.WorkspaceChangeSet, error) {
		result := make([]contract.WorkspaceChangeSet, len(refs))
		for index, ref := range refs {
			value, readErr := c.workspace.InspectWorkspaceChangeSetHistoryV1(ctx, ref)
			if readErr != nil {
				return nil, readErr
			}
			result[index] = value
		}
		return result, nil
	}
	keep, err := read(request.KeepChangeSetRefs)
	if err != nil {
		return contract.WorkspaceView{}, nil, nil, err
	}
	drop, err := read(request.DropChangeSetRefs)
	if err != nil {
		return contract.WorkspaceView{}, nil, nil, err
	}
	return view, keep, drop, nil
}

func workspaceRewindExpiryV1(request contract.ComposeWorkspaceRewindRequestV1, view contract.WorkspaceView, keep, drop []contract.WorkspaceChangeSet) time.Time {
	expires := request.RequestedNotAfter
	for _, unixNano := range []int64{view.Meta.ExpiresUnixNano, view.Lease.ExpiresUnixNano} {
		candidate := time.Unix(0, unixNano)
		if candidate.Before(expires) {
			expires = candidate
		}
	}
	for _, set := range append(append([]contract.WorkspaceChangeSet(nil), keep...), drop...) {
		candidate := time.Unix(0, set.Meta.ExpiresUnixNano)
		if candidate.Before(expires) {
			expires = candidate
		}
	}
	return expires
}

func sameWorkspaceChangeSetV1(left, right contract.WorkspaceChangeSet) bool {
	return reflect.DeepEqual(left, right)
}

func rewindNilLikeV1(value any) bool {
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

var _ ports.WorkspaceRewindCompositionPortV1 = (*WorkspaceRewindComposerV1)(nil)
