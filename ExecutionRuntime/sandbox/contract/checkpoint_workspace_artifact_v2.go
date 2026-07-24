package contract

import (
	"errors"
	"strings"
	"time"
)

const CheckpointWorkspaceArtifactInspectionContractVersionV2 = "praxis.sandbox/checkpoint-workspace-artifact-inspection/v2"

// CheckpointWorkspaceArtifactObservationV2 is the exact opaque Provider
// observation required to independently inspect a local checkpoint artifact.
// It grants no filesystem authority and contains no backend path.
type CheckpointWorkspaceArtifactObservationV2 struct {
	Provider         string `json:"provider"`
	ArtifactID       string `json:"artifact_id"`
	SubjectDigest    string `json:"subject_digest"`
	ContentDigest    string `json:"content_digest"`
	ContentLength    uint64 `json:"content_length"`
	State            string `json:"state"`
	CheckpointPhase  string `json:"checkpoint_phase"`
	RecordedUnixNano int64  `json:"recorded_unix_nano"`
	ExpiresUnixNano  int64  `json:"expires_unix_nano"`
}

func (v CheckpointWorkspaceArtifactObservationV2) ValidateCurrent(now time.Time) error {
	if v.Provider != "host_workspace" && v.Provider != "containerd_oci" {
		return errors.New("checkpoint artifact is not a workspace-capable local Provider")
	}
	if !strings.HasPrefix(v.ArtifactID, "praxis-checkpoint:") || !ValidDigest(v.SubjectDigest) || !ValidDigest(v.ContentDigest) || v.ContentLength == 0 || v.State != "prepared" || v.CheckpointPhase != "checkpoint_prepare" || v.RecordedUnixNano <= 0 || v.ExpiresUnixNano <= v.RecordedUnixNano || now.IsZero() || now.UnixNano() < v.RecordedUnixNano || now.UnixNano() >= v.ExpiresUnixNano {
		return errors.New("checkpoint workspace artifact observation is incomplete or stale")
	}
	if strings.TrimPrefix(v.ArtifactID, "praxis-checkpoint:") != strings.TrimPrefix(v.SubjectDigest, "sha256:") {
		return errors.New("checkpoint workspace artifact identity drifted")
	}
	return nil
}

type InspectCheckpointWorkspaceArtifactRequestV2 struct {
	Observation       CheckpointWorkspaceArtifactObservationV2 `json:"observation"`
	SnapshotID        string                                   `json:"snapshot_id"`
	TenantID          string                                   `json:"tenant_id"`
	SourceScopeDigest string                                   `json:"source_scope_digest"`
}

func (r InspectCheckpointWorkspaceArtifactRequestV2) ValidateCurrent(now time.Time) error {
	if err := r.Observation.ValidateCurrent(now); err != nil {
		return err
	}
	if strings.TrimSpace(r.SnapshotID) == "" || strings.TrimSpace(r.TenantID) == "" || !ValidDigest(r.SourceScopeDigest) {
		return errors.New("checkpoint workspace artifact request identity is incomplete")
	}
	return nil
}

type CheckpointWorkspaceArtifactInspectionV2 struct {
	ContractVersion string                                   `json:"contract_version"`
	Observation     CheckpointWorkspaceArtifactObservationV2 `json:"observation"`
	Bundle          WorkspaceSnapshotBundleV1                `json:"bundle"`
	CheckedUnixNano int64                                    `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                    `json:"expires_unix_nano"`
	Digest          string                                   `json:"digest"`
}

func SealCheckpointWorkspaceArtifactInspectionV2(value CheckpointWorkspaceArtifactInspectionV2, now time.Time) (CheckpointWorkspaceArtifactInspectionV2, error) {
	value.ContractVersion = CheckpointWorkspaceArtifactInspectionContractVersionV2
	value.Bundle = value.Bundle.Clone()
	value.Digest = ""
	digest, err := Digest("praxis.sandbox/checkpoint-workspace-artifact-inspection/body/v2", value)
	if err != nil {
		return CheckpointWorkspaceArtifactInspectionV2{}, err
	}
	value.Digest = digest
	return value, value.ValidateCurrent(now)
}

func (v CheckpointWorkspaceArtifactInspectionV2) ValidateCurrent(now time.Time) error {
	if v.ContractVersion != CheckpointWorkspaceArtifactInspectionContractVersionV2 || v.Observation.ValidateCurrent(now) != nil || v.Bundle.ValidateShape() != nil || v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= 0 || now.IsZero() || now.UnixNano() < v.CheckedUnixNano || now.UnixNano() >= v.ExpiresUnixNano || v.ExpiresUnixNano > v.Observation.ExpiresUnixNano || !ValidDigest(v.Digest) {
		return errors.New("checkpoint workspace artifact inspection is incomplete or stale")
	}
	copy := v
	copy.Bundle = v.Bundle.Clone()
	copy.Digest = ""
	digest, err := Digest("praxis.sandbox/checkpoint-workspace-artifact-inspection/body/v2", copy)
	if err != nil || digest != v.Digest {
		return errors.New("checkpoint workspace artifact inspection digest drifted")
	}
	return nil
}
