package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	WorkspaceRestoreAttemptTypeURLV1      = "praxis.sandbox/workspace-restore-attempt/v1"
	WorkspaceRestoreAttemptDigestDomainV1 = "praxis.sandbox/workspace-restore-attempt/body/v1"
	WorkspaceRestoreFactTypeURLV1         = "praxis.sandbox/workspace-restore-stage-fact/v1"
	WorkspaceRestoreFactDigestDomainV1    = "praxis.sandbox/workspace-restore-stage-fact/body/v1"
)

type WorkspaceRestoreAttemptStateV1 string

const (
	WorkspaceRestoreAttemptPreparedV1          WorkspaceRestoreAttemptStateV1 = "prepared"
	WorkspaceRestoreAttemptGovernedV1          WorkspaceRestoreAttemptStateV1 = "governed"
	WorkspaceRestoreAttemptInvocationV1        WorkspaceRestoreAttemptStateV1 = "invocation_recorded"
	WorkspaceRestoreAttemptReconcileRequiredV1 WorkspaceRestoreAttemptStateV1 = "reconcile_required"
	WorkspaceRestoreAttemptStagedV1            WorkspaceRestoreAttemptStateV1 = "staged"
	WorkspaceRestoreAttemptPartialV1           WorkspaceRestoreAttemptStateV1 = "partial"
)

type WorkspaceRestoreStageStateV1 string

const (
	WorkspaceRestoreStageCompleteV1 WorkspaceRestoreStageStateV1 = "complete"
	WorkspaceRestoreStagePartialV1  WorkspaceRestoreStageStateV1 = "partial"
)

type WorkspaceRestoreStageRequestV1 struct {
	TenantID                string                     `json:"tenant_id"`
	DispatchAttemptID       string                     `json:"dispatch_attempt_id"`
	RuntimeRestoreAttempt   SnapshotArtifactExactRefV2 `json:"runtime_restore_attempt"`
	RestoreEligibility      SnapshotArtifactExactRefV2 `json:"restore_eligibility"`
	Target                  RuntimeLeaseBinding        `json:"target"`
	SnapshotArtifactFactRef SnapshotArtifactExactRefV2 `json:"snapshot_artifact_fact_ref"`
	RequestedNotAfter       int64                      `json:"requested_not_after_unix_nano"`
}

func (r WorkspaceRestoreStageRequestV1) ValidateShape() error {
	if strings.TrimSpace(r.TenantID) == "" || strings.TrimSpace(r.DispatchAttemptID) == "" || r.RequestedNotAfter <= 0 {
		return errors.New("workspace restore stage request identity or TTL is invalid")
	}
	for name, ref := range map[string]SnapshotArtifactExactRefV2{
		"runtime restore attempt": r.RuntimeRestoreAttempt,
		"restore eligibility":     r.RestoreEligibility,
		"snapshot artifact fact":  r.SnapshotArtifactFactRef,
	} {
		if err := ref.ValidateShape(name); err != nil {
			return err
		}
	}
	if r.RuntimeRestoreAttempt.TypeURL != "praxis.runtime/restore-attempt/v2" || r.RestoreEligibility.TypeURL != "praxis.runtime/restore-eligibility/v2" || r.SnapshotArtifactFactRef.TypeURL != SnapshotArtifactFactTypeURL {
		return errors.New("workspace restore request exact ref kinds are invalid")
	}
	if err := r.Target.ValidateShape(); err != nil {
		return err
	}
	if r.TenantID != r.Target.TenantID {
		return errors.New("workspace restore request tenant does not match target")
	}
	return nil
}

func (r WorkspaceRestoreStageRequestV1) ValidateCurrent(now time.Time) error {
	if err := r.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() >= r.RequestedNotAfter {
		return errors.New("workspace restore stage request TTL is stale")
	}
	for name, ref := range map[string]SnapshotArtifactExactRefV2{
		"runtime restore attempt": r.RuntimeRestoreAttempt,
		"restore eligibility":     r.RestoreEligibility,
		"snapshot artifact fact":  r.SnapshotArtifactFactRef,
	} {
		if err := ref.ValidateCurrent(name, now); err != nil {
			return err
		}
	}
	if err := r.Target.ValidateCurrent(now); err != nil {
		return err
	}
	return nil
}

func (r WorkspaceRestoreStageRequestV1) StableKeyDigest() (string, error) {
	return Digest("praxis.sandbox/workspace-restore-stage-stable-key/v1", struct {
		TenantID          string
		DispatchAttemptID string
		Attempt           SnapshotArtifactExactRefV2
		Target            RuntimeLeaseBinding
	}{r.TenantID, r.DispatchAttemptID, r.RuntimeRestoreAttempt, r.Target})
}

type WorkspaceRestoreBundleCurrentProjectionV1 struct {
	TenantID                string                       `json:"tenant_id"`
	SnapshotArtifactFactRef SnapshotArtifactExactRefV2   `json:"snapshot_artifact_fact_ref"`
	StorageArtifactRef      SnapshotStorageArtifactRefV2 `json:"storage_artifact_ref"`
	Bundle                  WorkspaceSnapshotBundleV1    `json:"bundle"`
	CheckedUnixNano         int64                        `json:"checked_unix_nano"`
	ExpiresUnixNano         int64                        `json:"expires_unix_nano"`
	ProjectionDigest        string                       `json:"projection_digest"`
}

func SealWorkspaceRestoreBundleCurrentProjectionV1(value WorkspaceRestoreBundleCurrentProjectionV1) (WorkspaceRestoreBundleCurrentProjectionV1, error) {
	value.Bundle = value.Bundle.Clone()
	value.ProjectionDigest = ""
	digest, err := Digest("praxis.sandbox/workspace-restore-bundle-current/body/v1", value)
	if err != nil {
		return WorkspaceRestoreBundleCurrentProjectionV1{}, err
	}
	value.ProjectionDigest = digest
	return value, value.ValidateShape()
}

func (v WorkspaceRestoreBundleCurrentProjectionV1) ValidateShape() error {
	if strings.TrimSpace(v.TenantID) == "" || v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano || !ValidDigest(v.ProjectionDigest) {
		return errors.New("workspace restore bundle current projection is incomplete")
	}
	if err := v.SnapshotArtifactFactRef.ValidateShape("snapshot artifact fact"); err != nil || v.SnapshotArtifactFactRef.TypeURL != SnapshotArtifactFactTypeURL {
		return errors.New("workspace restore bundle projection artifact ref is invalid")
	}
	if err := v.StorageArtifactRef.ValidateShape(); err != nil || v.Bundle.ValidateShape() != nil {
		return errors.New("workspace restore bundle projection payload is invalid")
	}
	if v.TenantID != v.StorageArtifactRef.TenantID || v.TenantID != v.Bundle.TenantID || v.StorageArtifactRef.ContentDigest != digestBytesV1Must(v.Bundle) {
		return errors.New("workspace restore bundle projection crosses tenant or content")
	}
	copy := v
	copy.Bundle = v.Bundle.Clone()
	copy.ProjectionDigest = ""
	digest, err := Digest("praxis.sandbox/workspace-restore-bundle-current/body/v1", copy)
	if err != nil || digest != v.ProjectionDigest {
		return errors.New("workspace restore bundle current projection digest mismatch")
	}
	return nil
}

func (v WorkspaceRestoreBundleCurrentProjectionV1) ValidateCurrent(now time.Time) error {
	if err := v.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < v.CheckedUnixNano || now.UnixNano() >= v.ExpiresUnixNano {
		return errors.New("workspace restore bundle current projection is stale")
	}
	return nil
}

// WorkspaceRestoreGovernanceCurrentProjectionV1 is produced by a typed
// cross-owner Reader. Caller booleans are deliberately absent.
type WorkspaceRestoreGovernanceCurrentProjectionV1 struct {
	TenantID               string                     `json:"tenant_id"`
	RuntimeRestoreAttempt  SnapshotArtifactExactRefV2 `json:"runtime_restore_attempt"`
	RestoreEligibility     SnapshotArtifactExactRefV2 `json:"restore_eligibility"`
	Target                 RuntimeLeaseBinding        `json:"target"`
	ActionAdmissionRef     SnapshotArtifactExactRefV2 `json:"action_admission_ref"`
	ReviewAuthorizationRef SnapshotArtifactExactRefV2 `json:"review_authorization_ref"`
	DispatchPermitRef      SnapshotArtifactExactRefV2 `json:"dispatch_permit_ref"`
	BeginRef               SnapshotArtifactExactRefV2 `json:"begin_ref"`
	EnforcementRef         SnapshotArtifactExactRefV2 `json:"enforcement_ref"`
	CheckedUnixNano        int64                      `json:"checked_unix_nano"`
	ExpiresUnixNano        int64                      `json:"expires_unix_nano"`
	ProjectionDigest       string                     `json:"projection_digest"`
}

func SealWorkspaceRestoreGovernanceCurrentProjectionV1(value WorkspaceRestoreGovernanceCurrentProjectionV1) (WorkspaceRestoreGovernanceCurrentProjectionV1, error) {
	value.ProjectionDigest = ""
	digest, err := Digest("praxis.sandbox/workspace-restore-governance-current/body/v1", value)
	if err != nil {
		return WorkspaceRestoreGovernanceCurrentProjectionV1{}, err
	}
	value.ProjectionDigest = digest
	return value, value.ValidateShape()
}

func (v WorkspaceRestoreGovernanceCurrentProjectionV1) ValidateShape() error {
	if strings.TrimSpace(v.TenantID) == "" || v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano || !ValidDigest(v.ProjectionDigest) || v.Target.ValidateShape() != nil || v.TenantID != v.Target.TenantID {
		return errors.New("workspace restore governance current projection is incomplete")
	}
	refs := []SnapshotArtifactExactRefV2{v.RuntimeRestoreAttempt, v.RestoreEligibility, v.ActionAdmissionRef, v.ReviewAuthorizationRef, v.DispatchPermitRef, v.BeginRef, v.EnforcementRef}
	for _, ref := range refs {
		if err := ref.ValidateShape("workspace restore governance ref"); err != nil {
			return err
		}
	}
	copy := v
	copy.ProjectionDigest = ""
	digest, err := Digest("praxis.sandbox/workspace-restore-governance-current/body/v1", copy)
	if err != nil || digest != v.ProjectionDigest {
		return errors.New("workspace restore governance current projection digest mismatch")
	}
	return nil
}

func (v WorkspaceRestoreGovernanceCurrentProjectionV1) ValidateCurrent(now time.Time) error {
	if err := v.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < v.CheckedUnixNano || now.UnixNano() >= v.ExpiresUnixNano {
		return errors.New("workspace restore governance current projection is stale")
	}
	return nil
}

func (v WorkspaceRestoreGovernanceCurrentProjectionV1) MatchesRequest(r WorkspaceRestoreStageRequestV1) bool {
	return v.TenantID == r.TenantID && SameSnapshotArtifactExactRef(v.RuntimeRestoreAttempt, r.RuntimeRestoreAttempt) && SameSnapshotArtifactExactRef(v.RestoreEligibility, r.RestoreEligibility) && SameRuntimeLeaseBinding(v.Target, r.Target)
}

type WorkspaceRestoreAttemptV1 struct {
	Meta                       Meta                                           `json:"meta"`
	StableKeyDigest            string                                         `json:"stable_key_digest"`
	Request                    WorkspaceRestoreStageRequestV1                 `json:"request"`
	BundleProjectionDigest     string                                         `json:"bundle_projection_digest"`
	BundleDigest               string                                         `json:"bundle_digest"`
	GovernanceProjectionDigest string                                         `json:"governance_projection_digest"`
	Governance                 *WorkspaceRestoreGovernanceCurrentProjectionV1 `json:"governance,omitempty"`
	State                      WorkspaceRestoreAttemptStateV1                 `json:"state"`
	RootRef                    *WorkspaceRootRefV1                            `json:"root_ref,omitempty"`
	StageFactRef               *SnapshotArtifactExactRefV2                    `json:"stage_fact_ref,omitempty"`
	ProviderStageAttemptRef    *SnapshotArtifactExactRefV2                    `json:"provider_stage_attempt_ref,omitempty"`
}

func (v WorkspaceRestoreAttemptV1) Clone() WorkspaceRestoreAttemptV1 {
	if v.Governance != nil {
		value := *v.Governance
		v.Governance = &value
	}
	if v.RootRef != nil {
		value := *v.RootRef
		v.RootRef = &value
	}
	if v.StageFactRef != nil {
		value := *v.StageFactRef
		v.StageFactRef = &value
	}
	if v.ProviderStageAttemptRef != nil {
		value := *v.ProviderStageAttemptRef
		v.ProviderStageAttemptRef = &value
	}
	return v
}

func SealWorkspaceRestoreAttemptV1(value WorkspaceRestoreAttemptV1) (WorkspaceRestoreAttemptV1, error) {
	value.Meta.ContractVersion = ContractFamily
	value.Meta.Digest = ""
	digest, err := Digest(WorkspaceRestoreAttemptDigestDomainV1, value)
	if err != nil {
		return WorkspaceRestoreAttemptV1{}, err
	}
	value.Meta.Digest = digest
	return value, value.ValidateShape()
}

func (v WorkspaceRestoreAttemptV1) ValidateShape() error {
	if err := v.Meta.ValidateShape(); err != nil || v.Request.ValidateShape() != nil || !ValidDigest(v.StableKeyDigest) || !ValidDigest(v.BundleProjectionDigest) || !ValidDigest(v.BundleDigest) {
		return errors.New("workspace restore attempt is incomplete")
	}
	if v.Meta.ID != v.Request.DispatchAttemptID {
		return errors.New("workspace restore attempt does not bind the Runtime dispatch attempt")
	}
	stable, err := v.Request.StableKeyDigest()
	if err != nil || stable != v.StableKeyDigest {
		return errors.New("workspace restore attempt stable key drifted")
	}
	switch v.State {
	case WorkspaceRestoreAttemptPreparedV1:
		if v.GovernanceProjectionDigest != "" || v.Governance != nil || v.RootRef != nil || v.StageFactRef != nil || v.ProviderStageAttemptRef != nil {
			return errors.New("prepared workspace restore attempt carries governance or later refs")
		}
	case WorkspaceRestoreAttemptGovernedV1, WorkspaceRestoreAttemptInvocationV1:
		if !validWorkspaceRestoreAttemptGovernanceV1(v) || v.RootRef != nil || v.StageFactRef != nil || v.ProviderStageAttemptRef != nil {
			return errors.New("non-final workspace restore attempt carries final refs")
		}
	case WorkspaceRestoreAttemptReconcileRequiredV1:
		if !validWorkspaceRestoreAttemptGovernanceV1(v) || v.RootRef != nil || v.StageFactRef != nil || v.ProviderStageAttemptRef == nil || v.ProviderStageAttemptRef.ValidateShape("provider stage attempt") != nil {
			return errors.New("reconcile-required workspace restore attempt lacks original provider attempt")
		}
	case WorkspaceRestoreAttemptStagedV1, WorkspaceRestoreAttemptPartialV1:
		if !validWorkspaceRestoreAttemptGovernanceV1(v) || v.RootRef == nil || v.RootRef.ValidateShape() != nil || v.RootRef.BundleDigest != v.BundleDigest || v.StageFactRef == nil || v.StageFactRef.ValidateShape("workspace restore stage fact") != nil || v.StageFactRef.TypeURL != WorkspaceRestoreFactTypeURLV1 || v.ProviderStageAttemptRef == nil || *v.ProviderStageAttemptRef != v.RootRef.StageAttemptRef {
			return errors.New("final workspace restore attempt lacks exact root ref")
		}
	default:
		return fmt.Errorf("unsupported workspace restore attempt state %q", v.State)
	}
	copy := v
	copy.Meta.Digest = ""
	digest, err := Digest(WorkspaceRestoreAttemptDigestDomainV1, copy)
	if err != nil || digest != v.Meta.Digest {
		return errors.New("workspace restore attempt digest mismatch")
	}
	return nil
}

func validWorkspaceRestoreAttemptGovernanceV1(v WorkspaceRestoreAttemptV1) bool {
	return ValidDigest(v.GovernanceProjectionDigest) && v.Governance != nil && v.Governance.ValidateShape() == nil && v.Governance.ProjectionDigest == v.GovernanceProjectionDigest && v.Governance.MatchesRequest(v.Request)
}

type WorkspaceRestoreProviderRequestV1 struct {
	StageAttemptRef       SnapshotArtifactExactRefV2 `json:"stage_attempt_ref"`
	RuntimeRestoreAttempt SnapshotArtifactExactRefV2 `json:"runtime_restore_attempt"`
	Target                RuntimeLeaseBinding        `json:"target"`
	Bundle                WorkspaceSnapshotBundleV1  `json:"bundle"`
}

func (r WorkspaceRestoreProviderRequestV1) Clone() WorkspaceRestoreProviderRequestV1 {
	r.Bundle = r.Bundle.Clone()
	return r
}

func (r WorkspaceRestoreProviderRequestV1) ValidateShape() error {
	if err := r.StageAttemptRef.ValidateShape("workspace stage attempt"); err != nil || r.StageAttemptRef.TypeURL != WorkspaceRestoreAttemptTypeURLV1 {
		return errors.New("workspace provider stage attempt ref is invalid")
	}
	if err := r.RuntimeRestoreAttempt.ValidateShape("runtime restore attempt"); err != nil || r.RuntimeRestoreAttempt.TypeURL != "praxis.runtime/restore-attempt/v2" {
		return errors.New("workspace provider runtime attempt ref is invalid")
	}
	if err := r.Target.ValidateShape(); err != nil || r.Bundle.ValidateShape() != nil || r.Target.TenantID != r.Bundle.TenantID {
		return errors.New("workspace provider target or bundle is invalid")
	}
	return nil
}

func (r WorkspaceRestoreProviderRequestV1) ValidateCurrent(now time.Time) error {
	if err := r.ValidateShape(); err != nil {
		return err
	}
	if err := r.StageAttemptRef.ValidateCurrent("workspace stage attempt", now); err != nil {
		return err
	}
	if err := r.RuntimeRestoreAttempt.ValidateCurrent("runtime restore attempt", now); err != nil {
		return err
	}
	return r.Target.ValidateCurrent(now)
}

type WorkspaceRestoreProviderResultV1 struct {
	RootRef WorkspaceRootRefV1 `json:"root_ref"`
	Created bool               `json:"created"`
}

func (v WorkspaceRestoreAttemptV1) ExactRef() SnapshotArtifactExactRefV2 {
	return SnapshotArtifactExactRefV2{TypeURL: WorkspaceRestoreAttemptTypeURLV1, Version: 1, ID: v.Meta.ID, Revision: v.Meta.Revision, DigestAlgorithm: SnapshotArtifactDigestSHA256, DigestDomain: WorkspaceRestoreAttemptDigestDomainV1, Digest: v.Meta.Digest, ExpiresUnixNano: v.Meta.ExpiresUnixNano}
}

type WorkspaceRestoreStageFactV1 struct {
	Meta                    Meta                                          `json:"meta"`
	TenantID                string                                        `json:"tenant_id"`
	AttemptRef              SnapshotArtifactExactRefV2                    `json:"attempt_ref"`
	RuntimeRestoreAttempt   SnapshotArtifactExactRefV2                    `json:"runtime_restore_attempt"`
	RestoreEligibility      SnapshotArtifactExactRefV2                    `json:"restore_eligibility"`
	Target                  RuntimeLeaseBinding                           `json:"target"`
	SnapshotArtifactFactRef SnapshotArtifactExactRefV2                    `json:"snapshot_artifact_fact_ref"`
	BundleDigest            string                                        `json:"bundle_digest"`
	RootRef                 WorkspaceRootRefV1                            `json:"root_ref"`
	Governance              WorkspaceRestoreGovernanceCurrentProjectionV1 `json:"governance"`
	State                   WorkspaceRestoreStageStateV1                  `json:"state"`
	Residuals               []WorkspaceSnapshotExcludedV1                 `json:"residuals"`
}

func (v WorkspaceRestoreStageFactV1) Clone() WorkspaceRestoreStageFactV1 {
	v.Residuals = append([]WorkspaceSnapshotExcludedV1(nil), v.Residuals...)
	return v
}

func SealWorkspaceRestoreStageFactV1(value WorkspaceRestoreStageFactV1) (WorkspaceRestoreStageFactV1, error) {
	value.Residuals = append([]WorkspaceSnapshotExcludedV1(nil), value.Residuals...)
	value.Meta.ContractVersion = ContractFamily
	value.Meta.Digest = ""
	digest, err := Digest(WorkspaceRestoreFactDigestDomainV1, value)
	if err != nil {
		return WorkspaceRestoreStageFactV1{}, err
	}
	value.Meta.Digest = digest
	return value, value.ValidateShape()
}

func (v WorkspaceRestoreStageFactV1) ValidateShape() error {
	if err := v.Meta.ValidateShape(); err != nil || strings.TrimSpace(v.TenantID) == "" || v.AttemptRef.ValidateShape("workspace restore attempt") != nil || v.RuntimeRestoreAttempt.ValidateShape("runtime restore attempt") != nil || v.RestoreEligibility.ValidateShape("restore eligibility") != nil || v.Target.ValidateShape() != nil || v.SnapshotArtifactFactRef.ValidateShape("snapshot artifact") != nil || !ValidDigest(v.BundleDigest) || v.RootRef.ValidateShape() != nil || v.Governance.ValidateShape() != nil {
		return errors.New("workspace restore stage fact is incomplete")
	}
	if v.TenantID != v.Target.TenantID || v.TenantID != v.RootRef.TenantID || v.BundleDigest != v.RootRef.BundleDigest || v.Governance.TenantID != v.TenantID || v.Governance.RuntimeRestoreAttempt != v.RuntimeRestoreAttempt || v.Governance.RestoreEligibility != v.RestoreEligibility || v.Governance.Target != v.Target || v.AttemptRef.ID != v.Governance.EnforcementRef.ID {
		return errors.New("workspace restore stage fact crosses tenant or bundle")
	}
	for _, residual := range v.Residuals {
		if err := residual.ValidateShape(); err != nil {
			return err
		}
	}
	switch v.State {
	case WorkspaceRestoreStageCompleteV1:
		if len(v.Residuals) != 0 {
			return errors.New("complete workspace restore stage carries residuals")
		}
	case WorkspaceRestoreStagePartialV1:
		if len(v.Residuals) == 0 {
			return errors.New("partial workspace restore stage lacks residuals")
		}
	default:
		return fmt.Errorf("unsupported workspace restore stage state %q", v.State)
	}
	copy := v
	copy.Meta.Digest = ""
	digest, err := Digest(WorkspaceRestoreFactDigestDomainV1, copy)
	if err != nil || digest != v.Meta.Digest {
		return errors.New("workspace restore stage fact digest mismatch")
	}
	return nil
}

func (v WorkspaceRestoreStageFactV1) ExactRef() SnapshotArtifactExactRefV2 {
	return SnapshotArtifactExactRefV2{TypeURL: WorkspaceRestoreFactTypeURLV1, Version: 1, ID: v.Meta.ID, Revision: v.Meta.Revision, DigestAlgorithm: SnapshotArtifactDigestSHA256, DigestDomain: WorkspaceRestoreFactDigestDomainV1, Digest: v.Meta.Digest, ExpiresUnixNano: v.Meta.ExpiresUnixNano}
}

func digestBytesV1Must(bundle WorkspaceSnapshotBundleV1) string {
	encoded, err := EncodeWorkspaceSnapshotBundleV1(bundle)
	if err != nil {
		return ""
	}
	digest := sha256DigestV1(encoded)
	return digest
}

func sha256DigestV1(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
