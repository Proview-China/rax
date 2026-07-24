package runtimeadapter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type WorkspaceRestoreRuntimeSettlementReaderV1 interface {
	InspectRestoreStageSettlementV1(context.Context, string) (runtimeports.RestoreStageSettlementFactV1, error)
}

type WorkspaceRestoreSettlementAdapterV1 struct {
	runtime WorkspaceRestoreRuntimeSettlementReaderV1
	owner   ports.WorkspaceRestoreSettlementOwnerPortV1
}

func NewWorkspaceRestoreSettlementAdapterV1(runtime WorkspaceRestoreRuntimeSettlementReaderV1, owner ports.WorkspaceRestoreSettlementOwnerPortV1) (*WorkspaceRestoreSettlementAdapterV1, error) {
	if nilLikeV4(runtime) || nilLikeV4(owner) {
		return nil, errors.New("Runtime Restore Stage Settlement Reader and Sandbox Settlement Owner are required")
	}
	return &WorkspaceRestoreSettlementAdapterV1{runtime: runtime, owner: owner}, nil
}

func (a *WorkspaceRestoreSettlementAdapterV1) ApplyWorkspaceRestoreStageSettlementV1(ctx context.Context, expected runtimeports.RestoreStageSettlementRefV1, stageFact contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreApplySettlementFactV1, error) {
	if a == nil || nilLikeV4(ctx) || expected.Validate() != nil || stageFact.ValidateShape("workspace restore Stage Fact") != nil || stageFact.TypeURL != contract.WorkspaceRestoreFactTypeURLV1 {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, errors.New("workspace restore Runtime Settlement apply coordinates are invalid")
	}
	fact, err := a.runtime.InspectRestoreStageSettlementV1(ctx, expected.ID)
	if err != nil {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, err
	}
	if fact.Validate() != nil || fact.RefV1() != expected {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, fmt.Errorf("%w: Runtime Restore Stage Settlement exact Inspect drifted", ports.ErrConflict)
	}
	domain := expected.DomainResult
	if domain.ID != stageFact.ID || uint64(domain.Revision) != stageFact.Revision || !sameDigestV1(string(domain.Digest), stageFact.Digest) || string(domain.TenantID) != string(fact.Submission.RestoreAttempt.TenantID) {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, fmt.Errorf("%w: Runtime Settlement binds another Sandbox Stage Fact", ports.ErrConflict)
	}
	opaque := contract.RuntimeRestoreStageSettlementRefV1{ID: expected.ID, Revision: uint64(expected.Revision), Digest: strings.TrimPrefix(string(expected.Digest), "sha256:"), OperationDigest: strings.TrimPrefix(string(expected.OperationDigest), "sha256:"), EffectID: string(expected.EffectID), DomainResult: stageFact}
	if err := opaque.ValidateShape(); err != nil {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, err
	}
	return a.owner.ApplyWorkspaceRestoreSettlementV1(ctx, opaque)
}
