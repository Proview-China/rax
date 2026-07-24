package contract

import (
	"errors"
	"fmt"
	"strings"
)

const (
	WorkspaceRootRefTypeURLV1      = "praxis.sandbox/workspace-root-ref/v1"
	WorkspaceRootRefDigestDomainV1 = "praxis.sandbox/workspace-root-ref/body/v1"
	WorkspaceRootRefVersionV1      = uint32(1)
)

// WorkspaceRootRefV1 is intentionally opaque outside Sandbox/Agent Host. ID is
// a stable handle, never an absolute or relative host path.
type WorkspaceRootRefV1 struct {
	TypeURL               string                     `json:"type_url"`
	Version               uint32                     `json:"version"`
	ID                    string                     `json:"id"`
	Revision              uint64                     `json:"revision"`
	TenantID              string                     `json:"tenant_id"`
	RestoreAttemptID      string                     `json:"restore_attempt_id"`
	RuntimeRestoreAttempt SnapshotArtifactExactRefV2 `json:"runtime_restore_attempt"`
	StageAttemptRef       SnapshotArtifactExactRefV2 `json:"stage_attempt_ref"`
	Target                RuntimeLeaseBinding        `json:"target"`
	BundleDigest          string                     `json:"bundle_digest"`
	DigestAlgorithm       string                     `json:"digest_algorithm"`
	DigestDomain          string                     `json:"digest_domain"`
	Digest                string                     `json:"digest"`
}

func SealWorkspaceRootRefV1(value WorkspaceRootRefV1) (WorkspaceRootRefV1, error) {
	value.TypeURL = WorkspaceRootRefTypeURLV1
	value.Version = WorkspaceRootRefVersionV1
	value.Revision = 1
	value.DigestAlgorithm = SnapshotArtifactDigestSHA256
	value.DigestDomain = WorkspaceRootRefDigestDomainV1
	value.Digest = ""
	if err := validateWorkspaceRootRefBodyV1(value); err != nil {
		return WorkspaceRootRefV1{}, err
	}
	digest, err := Digest(WorkspaceRootRefDigestDomainV1, value)
	if err != nil {
		return WorkspaceRootRefV1{}, err
	}
	value.Digest = digest
	return value, value.ValidateShape()
}

func (v WorkspaceRootRefV1) ValidateShape() error {
	if err := validateWorkspaceRootRefBodyV1(v); err != nil {
		return err
	}
	if !ValidDigest(v.Digest) {
		return errors.New("workspace root ref digest is invalid")
	}
	copy := v
	copy.Digest = ""
	digest, err := Digest(WorkspaceRootRefDigestDomainV1, copy)
	if err != nil || digest != v.Digest {
		return errors.New("workspace root ref digest mismatch")
	}
	return nil
}

func validateWorkspaceRootRefBodyV1(v WorkspaceRootRefV1) error {
	if v.TypeURL != WorkspaceRootRefTypeURLV1 || v.Version != WorkspaceRootRefVersionV1 || v.Revision != 1 || v.DigestAlgorithm != SnapshotArtifactDigestSHA256 || v.DigestDomain != WorkspaceRootRefDigestDomainV1 {
		return errors.New("workspace root ref contract is invalid")
	}
	if strings.TrimSpace(v.ID) == "" || strings.ContainsAny(v.ID, `/\\`) || strings.TrimSpace(v.TenantID) == "" || strings.TrimSpace(v.RestoreAttemptID) == "" || !ValidDigest(v.BundleDigest) {
		return errors.New("workspace root ref identity is incomplete or non-opaque")
	}
	if err := v.Target.ValidateShape(); err != nil {
		return fmt.Errorf("workspace root target: %w", err)
	}
	if err := v.StageAttemptRef.ValidateShape("workspace stage attempt"); err != nil || v.StageAttemptRef.TypeURL != WorkspaceRestoreAttemptTypeURLV1 {
		return errors.New("workspace root stage attempt ref is invalid")
	}
	if err := v.RuntimeRestoreAttempt.ValidateShape("runtime restore attempt"); err != nil || v.RuntimeRestoreAttempt.TypeURL != "praxis.runtime/restore-attempt/v2" || v.RuntimeRestoreAttempt.ID != v.RestoreAttemptID {
		return errors.New("workspace root Runtime restore attempt ref is invalid")
	}
	if v.TenantID != v.Target.TenantID {
		return errors.New("workspace root tenant does not match target lease")
	}
	return nil
}

func SameWorkspaceRootRefV1(a, b WorkspaceRootRefV1) bool {
	return a == b
}
