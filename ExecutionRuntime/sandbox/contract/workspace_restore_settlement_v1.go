package contract

import (
	"errors"
	"strings"
)

const (
	WorkspaceRestoreSettlementTypeURLV1      = "praxis.sandbox/workspace-restore-settlement/v1"
	WorkspaceRestoreSettlementDigestDomainV1 = "praxis.sandbox/workspace-restore-settlement/body/v1"
)

// RuntimeRestoreStageSettlementRefV1 is deliberately opaque. It carries only
// Runtime identity/digest coordinates and the exact Sandbox Fact it binds; it
// contains no outcome, disposition, activation, or rollback semantics.
type RuntimeRestoreStageSettlementRefV1 struct {
	ID              string                     `json:"id"`
	Revision        uint64                     `json:"revision"`
	Digest          string                     `json:"digest"`
	OperationDigest string                     `json:"operation_digest"`
	EffectID        string                     `json:"effect_id"`
	DomainResult    SnapshotArtifactExactRefV2 `json:"domain_result"`
}

func (r RuntimeRestoreStageSettlementRefV1) ValidateShape() error {
	if strings.TrimSpace(r.ID) == "" || r.Revision != 1 || !ValidDigest(strings.TrimPrefix(r.Digest, "sha256:")) || !ValidDigest(strings.TrimPrefix(r.OperationDigest, "sha256:")) || strings.TrimSpace(r.EffectID) == "" || r.DomainResult.ValidateShape("workspace restore Stage DomainResult") != nil || r.DomainResult.TypeURL != WorkspaceRestoreFactTypeURLV1 {
		return errors.New("opaque Runtime Restore Stage Settlement ref is incomplete")
	}
	return nil
}

type WorkspaceRestoreApplySettlementFactV1 struct {
	Meta              Meta                               `json:"meta"`
	TenantID          string                             `json:"tenant_id"`
	StageFactRef      SnapshotArtifactExactRefV2         `json:"stage_fact_ref"`
	RuntimeSettlement RuntimeRestoreStageSettlementRefV1 `json:"runtime_settlement"`
}

func SealWorkspaceRestoreApplySettlementFactV1(value WorkspaceRestoreApplySettlementFactV1) (WorkspaceRestoreApplySettlementFactV1, error) {
	value.Meta.ContractVersion = ContractFamily
	value.Meta.Digest = ""
	digest, err := Digest(WorkspaceRestoreSettlementDigestDomainV1, value)
	if err != nil {
		return WorkspaceRestoreApplySettlementFactV1{}, err
	}
	value.Meta.Digest = digest
	return value, value.ValidateShape()
}

func (v WorkspaceRestoreApplySettlementFactV1) ValidateShape() error {
	if v.Meta.ValidateShape() != nil || strings.TrimSpace(v.TenantID) == "" || v.StageFactRef.ValidateShape("workspace restore Stage Fact") != nil || v.StageFactRef.TypeURL != WorkspaceRestoreFactTypeURLV1 || v.RuntimeSettlement.ValidateShape() != nil || v.RuntimeSettlement.DomainResult != v.StageFactRef {
		return errors.New("workspace restore ApplySettlement Fact is incomplete or crosses DomainResult")
	}
	copy := v
	copy.Meta.Digest = ""
	digest, err := Digest(WorkspaceRestoreSettlementDigestDomainV1, copy)
	if err != nil || digest != v.Meta.Digest {
		return errors.New("workspace restore ApplySettlement Fact digest mismatch")
	}
	return nil
}

func (v WorkspaceRestoreApplySettlementFactV1) ExactRef() SnapshotArtifactExactRefV2 {
	return SnapshotArtifactExactRefV2{TypeURL: WorkspaceRestoreSettlementTypeURLV1, Version: 1, ID: v.Meta.ID, Revision: v.Meta.Revision, DigestAlgorithm: SnapshotArtifactDigestSHA256, DigestDomain: WorkspaceRestoreSettlementDigestDomainV1, Digest: v.Meta.Digest, ExpiresUnixNano: v.Meta.ExpiresUnixNano}
}
