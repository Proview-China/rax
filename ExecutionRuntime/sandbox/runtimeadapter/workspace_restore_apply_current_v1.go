package runtimeadapter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// WorkspaceRestoreApplySettlementCurrentAdapterV1 exposes only the exact
// Sandbox-owned ApplySettlement fact required by Runtime Activation. It does
// not interpret Runtime outcome or activate the target Instance.
type WorkspaceRestoreApplySettlementCurrentAdapterV1 struct {
	apply   *WorkspaceRestoreSettlementAdapterV1
	owner   sandboxports.WorkspaceRestoreSettlementOwnerPortV1
	binding runtimeports.ProviderBindingRefV2
	clock   func() time.Time
}

func NewWorkspaceRestoreApplySettlementCurrentAdapterV1(apply *WorkspaceRestoreSettlementAdapterV1, owner sandboxports.WorkspaceRestoreSettlementOwnerPortV1, binding runtimeports.ProviderBindingRefV2, clock func() time.Time) (*WorkspaceRestoreApplySettlementCurrentAdapterV1, error) {
	if apply == nil || nilLikeV4(owner) || binding.Validate() != nil || clock == nil {
		return nil, errors.New("Sandbox Restore ApplySettlement adapter, Owner binding, and clock are required")
	}
	return &WorkspaceRestoreApplySettlementCurrentAdapterV1{apply: apply, owner: owner, binding: binding, clock: clock}, nil
}

func (a *WorkspaceRestoreApplySettlementCurrentAdapterV1) ApplyWorkspaceRestoreStageSettlementCurrentV1(ctx context.Context, settlement runtimeports.RestoreStageSettlementRefV1, stageFact contract.SnapshotArtifactExactRefV2) (runtimeports.RestoreStageApplySettlementCurrentProjectionV1, error) {
	if a == nil {
		return runtimeports.RestoreStageApplySettlementCurrentProjectionV1{}, errors.New("Sandbox Restore ApplySettlement current adapter is nil")
	}
	fact, err := a.apply.ApplyWorkspaceRestoreStageSettlementV1(ctx, settlement, stageFact)
	if err != nil {
		return runtimeports.RestoreStageApplySettlementCurrentProjectionV1{}, err
	}
	return a.projectV1(fact, settlement, a.clock())
}

func (a *WorkspaceRestoreApplySettlementCurrentAdapterV1) InspectRestoreStageApplySettlementCurrentV1(ctx context.Context, expected runtimeports.RestoreStageApplySettlementRefV1) (runtimeports.RestoreStageApplySettlementCurrentProjectionV1, error) {
	if a == nil || expected.Validate() != nil {
		return runtimeports.RestoreStageApplySettlementCurrentProjectionV1{}, errors.New("Sandbox Restore ApplySettlement current coordinates are invalid")
	}
	fact, err := a.owner.InspectWorkspaceRestoreApplySettlementV1(ctx, string(expected.TenantID), expected.ID)
	if err != nil {
		return runtimeports.RestoreStageApplySettlementCurrentProjectionV1{}, err
	}
	projection, err := a.projectV1(fact, expected.RuntimeSettlement, a.clock())
	if err != nil {
		return runtimeports.RestoreStageApplySettlementCurrentProjectionV1{}, err
	}
	if !runtimeports.SameRestoreStageApplySettlementRefV1(projection.Fact, expected) {
		return runtimeports.RestoreStageApplySettlementCurrentProjectionV1{}, fmt.Errorf("%w: Sandbox ApplySettlement exact ref drifted", sandboxports.ErrConflict)
	}
	return projection, nil
}

func (a *WorkspaceRestoreApplySettlementCurrentAdapterV1) projectV1(fact contract.WorkspaceRestoreApplySettlementFactV1, settlement runtimeports.RestoreStageSettlementRefV1, now time.Time) (runtimeports.RestoreStageApplySettlementCurrentProjectionV1, error) {
	if fact.ValidateShape() != nil || settlement.Validate() != nil || now.IsZero() || fact.RuntimeSettlement.ID != settlement.ID || fact.RuntimeSettlement.Revision != uint64(settlement.Revision) || !sameDigestV1(fact.RuntimeSettlement.Digest, string(settlement.Digest)) || !sameDigestV1(fact.RuntimeSettlement.OperationDigest, string(settlement.OperationDigest)) || fact.RuntimeSettlement.EffectID != string(settlement.EffectID) {
		return runtimeports.RestoreStageApplySettlementCurrentProjectionV1{}, fmt.Errorf("%w: Sandbox ApplySettlement and Runtime Settlement drifted", sandboxports.ErrConflict)
	}
	domain := settlement.DomainResult
	if fact.StageFactRef.TypeURL != contract.WorkspaceRestoreFactTypeURLV1 || fact.StageFactRef.ID != domain.ID || fact.StageFactRef.Revision != uint64(domain.Revision) || !sameDigestV1(fact.StageFactRef.Digest, string(domain.Digest)) || fact.TenantID != string(domain.TenantID) {
		return runtimeports.RestoreStageApplySettlementCurrentProjectionV1{}, fmt.Errorf("%w: Sandbox ApplySettlement binds another Stage Fact", sandboxports.ErrConflict)
	}
	ref := runtimeports.RestoreStageApplySettlementRefV1{
		Owner: a.binding, ID: fact.Meta.ID, Revision: runtimecore.Revision(fact.Meta.Revision), Digest: runtimecore.Digest("sha256:" + strings.TrimPrefix(fact.Meta.Digest, "sha256:")),
		TenantID: settlement.DomainResult.TenantID, DomainResult: settlement.DomainResult, RuntimeSettlement: settlement,
	}
	if err := ref.Validate(); err != nil {
		return runtimeports.RestoreStageApplySettlementCurrentProjectionV1{}, err
	}
	checked := fact.Meta.UpdatedUnixNano
	if checked <= 0 {
		checked = fact.Meta.CreatedUnixNano
	}
	return runtimeports.SealRestoreStageApplySettlementCurrentProjectionV1(runtimeports.RestoreStageApplySettlementCurrentProjectionV1{Fact: ref, CheckedUnixNano: checked, ExpiresUnixNano: fact.Meta.ExpiresUnixNano}, now)
}

var _ runtimeports.RestoreStageApplySettlementCurrentReaderV1 = (*WorkspaceRestoreApplySettlementCurrentAdapterV1)(nil)
