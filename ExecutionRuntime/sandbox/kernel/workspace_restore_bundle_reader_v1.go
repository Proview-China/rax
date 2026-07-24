package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// WorkspaceRestoreBundleReaderV1 derives the restorable payload exclusively
// from the Sandbox SnapshotArtifact Fact and encrypted Content Store exact ref.
type WorkspaceRestoreBundleReaderV1 struct {
	artifacts ports.SnapshotArtifactOwnerPortV2
	content   ports.SnapshotContentStoreV2
	clock     func() time.Time
	maxTTL    time.Duration
}

func NewWorkspaceRestoreBundleReaderV1(artifacts ports.SnapshotArtifactOwnerPortV2, content ports.SnapshotContentStoreV2, clock func() time.Time, maxTTL time.Duration) (*WorkspaceRestoreBundleReaderV1, error) {
	if nilInterface(artifacts) || nilInterface(content) || clock == nil || maxTTL <= 0 {
		return nil, errors.New("workspace restore bundle Reader dependencies are required")
	}
	return &WorkspaceRestoreBundleReaderV1{artifacts: artifacts, content: content, clock: clock, maxTTL: maxTTL}, nil
}

func (r *WorkspaceRestoreBundleReaderV1) InspectWorkspaceRestoreBundleCurrentV1(ctx context.Context, request contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreBundleCurrentProjectionV1, error) {
	now := r.clock()
	if err := request.ValidateCurrent(now); err != nil {
		return contract.WorkspaceRestoreBundleCurrentProjectionV1{}, err
	}
	projection, fact, err := r.inspect(ctx, request)
	if err != nil {
		return contract.WorkspaceRestoreBundleCurrentProjectionV1{}, err
	}
	if err := fact.ValidateCurrent(now); err != nil || projection.StorageArtifactRef.ValidateCurrent(now) != nil || projection.ValidateCurrent(now) != nil {
		return contract.WorkspaceRestoreBundleCurrentProjectionV1{}, fmt.Errorf("%w: workspace snapshot artifact or content is not current", ports.ErrStale)
	}
	return projection, nil
}

func (r *WorkspaceRestoreBundleReaderV1) InspectWorkspaceRestoreBundleExactV1(ctx context.Context, request contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreBundleCurrentProjectionV1, error) {
	if err := request.ValidateShape(); err != nil {
		return contract.WorkspaceRestoreBundleCurrentProjectionV1{}, err
	}
	projection, _, err := r.inspect(ctx, request)
	return projection, err
}

func (r *WorkspaceRestoreBundleReaderV1) inspect(ctx context.Context, request contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreBundleCurrentProjectionV1, contract.SnapshotArtifactFactV2, error) {
	fact, err := r.artifacts.InspectArtifactFact(ctx, &contract.InspectSnapshotArtifactFactRequestV2{ExpectedRef: request.SnapshotArtifactFactRef})
	if err != nil {
		return contract.WorkspaceRestoreBundleCurrentProjectionV1{}, contract.SnapshotArtifactFactV2{}, err
	}
	if fact.ValidateShape() != nil || fact.State != contract.SnapshotArtifactAvailable || fact.TenantID != request.TenantID || fact.ExactRef() != request.SnapshotArtifactFactRef {
		return contract.WorkspaceRestoreBundleCurrentProjectionV1{}, contract.SnapshotArtifactFactV2{}, fmt.Errorf("%w: workspace snapshot Artifact Fact crosses exact request", ports.ErrConflict)
	}
	content, err := r.content.InspectSnapshotContentV2(ctx, &contract.InspectSnapshotContentRequestV2{ExpectedRef: fact.StorageArtifactRef})
	if err != nil {
		return contract.WorkspaceRestoreBundleCurrentProjectionV1{}, contract.SnapshotArtifactFactV2{}, err
	}
	if content.StorageRef != fact.StorageArtifactRef {
		return contract.WorkspaceRestoreBundleCurrentProjectionV1{}, contract.SnapshotArtifactFactV2{}, fmt.Errorf("%w: workspace snapshot Content Store returned another exact ref", ports.ErrConflict)
	}
	bundle, err := contract.DecodeWorkspaceSnapshotBundleV1(content.Content)
	if err != nil {
		return contract.WorkspaceRestoreBundleCurrentProjectionV1{}, contract.SnapshotArtifactFactV2{}, fmt.Errorf("%w: workspace snapshot bundle is not canonical: %v", ports.ErrConflict, err)
	}
	checked := fact.Meta.UpdatedUnixNano
	if content.StorageRef.CreatedUnixNano > checked {
		checked = content.StorageRef.CreatedUnixNano
	}
	expires := minimumWorkspaceRestoreTimeV1(fact.Meta.ExpiresUnixNano, fact.StorageArtifactRef.ExpiresUnixNano, request.SnapshotArtifactFactRef.ExpiresUnixNano, checked+int64(r.maxTTL))
	projection, err := contract.SealWorkspaceRestoreBundleCurrentProjectionV1(contract.WorkspaceRestoreBundleCurrentProjectionV1{TenantID: request.TenantID, SnapshotArtifactFactRef: fact.ExactRef(), StorageArtifactRef: fact.StorageArtifactRef, Bundle: bundle, CheckedUnixNano: checked, ExpiresUnixNano: expires})
	if err != nil {
		return contract.WorkspaceRestoreBundleCurrentProjectionV1{}, contract.SnapshotArtifactFactV2{}, err
	}
	return projection, fact, nil
}

var _ ports.WorkspaceRestoreBundleCurrentReaderV1 = (*WorkspaceRestoreBundleReaderV1)(nil)
