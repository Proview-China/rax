package contract

import (
	"errors"
	"slices"
	"strings"
	"time"
)

const (
	WorkspaceCheckpointCoverageTypeURL         = "praxis.sandbox/workspace-checkpoint-coverage-fact-ref/v2"
	WorkspaceCheckpointCoverageDigestDomain    = "praxis.sandbox/workspace-checkpoint-coverage-fact/body/v2"
	WorkspaceCheckpointParticipantTypeURL      = "praxis.sandbox/workspace-checkpoint-participant-fact-ref/v2"
	WorkspaceCheckpointParticipantDigestDomain = "praxis.sandbox/workspace-checkpoint-participant-fact/body/v2"
	WorkspaceSnapshotDataDomain                = "workspace"
)

type WorkspaceCheckpointCoverageState string

const WorkspaceCheckpointCoverageComplete WorkspaceCheckpointCoverageState = "complete"

// WorkspaceCheckpointCoverageFactV2 states exactly what the Sandbox workspace
// snapshot covers. Declared exclusions are explicit; unresolved failures stay
// ResidualRefs and cannot produce a complete fact.
type WorkspaceCheckpointCoverageFactV2 struct {
	Meta                    Meta                             `json:"meta"`
	TenantID                string                           `json:"tenant_id"`
	ScopeDigest             string                           `json:"scope_digest"`
	RunID                   string                           `json:"run_id"`
	CheckpointAttemptRef    Ref                              `json:"checkpoint_attempt_ref"`
	BarrierRef              Ref                              `json:"barrier_ref"`
	EffectCutRef            Ref                              `json:"effect_cut_ref"`
	ParticipantID           string                           `json:"participant_id"`
	ParticipantDigest       string                           `json:"participant_digest"`
	PreparedPhaseFactRef    Ref                              `json:"prepared_phase_fact_ref"`
	SnapshotArtifactFactRef SnapshotArtifactExactRefV2       `json:"snapshot_artifact_fact_ref"`
	SnapshotAggregateRef    SnapshotArtifactAggregateRefV2   `json:"snapshot_aggregate_ref"`
	CoveragePolicyRef       Ref                              `json:"coverage_policy_ref"`
	Included                []string                         `json:"included"`
	DeclaredExcluded        []string                         `json:"declared_excluded"`
	ResidualRefs            []Ref                            `json:"residual_refs"`
	State                   WorkspaceCheckpointCoverageState `json:"state"`
	RequestedNotAfter       int64                            `json:"requested_not_after_unix_nano"`
}

func (v WorkspaceCheckpointCoverageFactV2) Clone() WorkspaceCheckpointCoverageFactV2 {
	v.Included = append([]string(nil), v.Included...)
	v.DeclaredExcluded = append([]string(nil), v.DeclaredExcluded...)
	v.ResidualRefs = append([]Ref(nil), v.ResidualRefs...)
	return v
}

func SealWorkspaceCheckpointCoverageFactV2(value WorkspaceCheckpointCoverageFactV2) (WorkspaceCheckpointCoverageFactV2, error) {
	value.Included = append([]string(nil), value.Included...)
	value.DeclaredExcluded = append([]string(nil), value.DeclaredExcluded...)
	value.ResidualRefs = append([]Ref(nil), value.ResidualRefs...)
	slices.Sort(value.Included)
	slices.Sort(value.DeclaredExcluded)
	slices.SortFunc(value.ResidualRefs, compareWorkspaceCheckpointRefV2)
	value.Meta.Digest = ""
	if err := validateWorkspaceCheckpointCoverageBodyV2(value); err != nil {
		return WorkspaceCheckpointCoverageFactV2{}, err
	}
	digest, err := Digest(WorkspaceCheckpointCoverageDigestDomain, value)
	if err != nil {
		return WorkspaceCheckpointCoverageFactV2{}, err
	}
	value.Meta.Digest = digest
	return value, value.ValidateShape()
}

func (v WorkspaceCheckpointCoverageFactV2) ValidateShape() error {
	if err := v.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := validateWorkspaceCheckpointCoverageBodyV2(v); err != nil {
		return err
	}
	copy := v
	copy.Included = append([]string(nil), v.Included...)
	copy.DeclaredExcluded = append([]string(nil), v.DeclaredExcluded...)
	copy.ResidualRefs = append([]Ref(nil), v.ResidualRefs...)
	copy.Meta.Digest = ""
	digest, err := Digest(WorkspaceCheckpointCoverageDigestDomain, copy)
	if err != nil || digest != v.Meta.Digest {
		return errors.New("workspace checkpoint coverage digest mismatch")
	}
	return nil
}

func (v WorkspaceCheckpointCoverageFactV2) ValidateCurrent(now time.Time) error {
	if err := v.ValidateShape(); err != nil {
		return err
	}
	return v.Meta.ValidateCurrent(now)
}

func (v WorkspaceCheckpointCoverageFactV2) ExactRef() SnapshotArtifactExactRefV2 {
	return SnapshotArtifactExactRefV2{TypeURL: WorkspaceCheckpointCoverageTypeURL, Version: SnapshotArtifactVersion, ID: v.Meta.ID, Revision: v.Meta.Revision, DigestAlgorithm: SnapshotArtifactDigestSHA256, DigestDomain: WorkspaceCheckpointCoverageDigestDomain, Digest: v.Meta.Digest, ExpiresUnixNano: v.Meta.ExpiresUnixNano}
}

func validateWorkspaceCheckpointCoverageBodyV2(v WorkspaceCheckpointCoverageFactV2) error {
	if strings.TrimSpace(v.TenantID) == "" || !ValidDigest(v.ScopeDigest) || strings.TrimSpace(v.RunID) == "" || strings.TrimSpace(v.ParticipantID) == "" || !ValidDigest(v.ParticipantDigest) || v.State != WorkspaceCheckpointCoverageComplete || v.RequestedNotAfter <= 0 {
		return errors.New("workspace checkpoint coverage fact is incomplete")
	}
	for name, ref := range map[string]Ref{"checkpoint attempt": v.CheckpointAttemptRef, "barrier": v.BarrierRef, "effect cut": v.EffectCutRef, "prepared phase": v.PreparedPhaseFactRef, "coverage policy": v.CoveragePolicyRef} {
		if err := ref.ValidateShape("workspace checkpoint " + name); err != nil {
			return err
		}
	}
	if err := v.SnapshotArtifactFactRef.ValidateShape("workspace checkpoint snapshot artifact"); err != nil {
		return err
	}
	if v.SnapshotArtifactFactRef.TypeURL != SnapshotArtifactFactTypeURL || v.SnapshotArtifactFactRef.DigestDomain != SnapshotArtifactFactDomain {
		return errors.New("workspace checkpoint coverage binds a non-artifact fact")
	}
	if err := v.SnapshotAggregateRef.ValidateShape(); err != nil {
		return err
	}
	if v.SnapshotAggregateRef.TenantID != v.TenantID || v.SnapshotAggregateRef.DataDomain != WorkspaceSnapshotDataDomain {
		return errors.New("workspace checkpoint coverage snapshot aggregate binding drifted")
	}
	if len(v.Included) == 0 || ValidateSortedUnique(v.Included, "workspace checkpoint included coverage") != nil || ValidateSortedUnique(v.DeclaredExcluded, "workspace checkpoint declared exclusions") != nil {
		return errors.New("workspace checkpoint coverage sets are empty or non-canonical")
	}
	if len(v.ResidualRefs) != 0 {
		return errors.New("complete workspace checkpoint coverage cannot contain residuals")
	}
	if v.Meta.ExpiresUnixNano > v.RequestedNotAfter || v.Meta.ExpiresUnixNano > v.SnapshotArtifactFactRef.ExpiresUnixNano || v.Meta.ExpiresUnixNano > v.SnapshotAggregateRef.ExpiresUnixNano {
		return errors.New("workspace checkpoint coverage TTL exceeds an upstream bound")
	}
	return nil
}

type WorkspaceCheckpointParticipantState string

const WorkspaceCheckpointParticipantPrepared WorkspaceCheckpointParticipantState = "prepared"

type WorkspaceCheckpointParticipantFactV2 struct {
	Meta                    Meta                                `json:"meta"`
	TenantID                string                              `json:"tenant_id"`
	ScopeDigest             string                              `json:"scope_digest"`
	RunID                   string                              `json:"run_id"`
	CheckpointAttemptRef    Ref                                 `json:"checkpoint_attempt_ref"`
	BarrierRef              Ref                                 `json:"barrier_ref"`
	EffectCutRef            Ref                                 `json:"effect_cut_ref"`
	ParticipantID           string                              `json:"participant_id"`
	ParticipantDigest       string                              `json:"participant_digest"`
	PreparedPhaseFactRef    Ref                                 `json:"prepared_phase_fact_ref"`
	SnapshotArtifactFactRef SnapshotArtifactExactRefV2          `json:"snapshot_artifact_fact_ref"`
	SnapshotAggregateRef    SnapshotArtifactAggregateRefV2      `json:"snapshot_aggregate_ref"`
	CoverageFactRef         SnapshotArtifactExactRefV2          `json:"coverage_fact_ref"`
	State                   WorkspaceCheckpointParticipantState `json:"state"`
	RequestedNotAfter       int64                               `json:"requested_not_after_unix_nano"`
}

func SealWorkspaceCheckpointParticipantFactV2(value WorkspaceCheckpointParticipantFactV2) (WorkspaceCheckpointParticipantFactV2, error) {
	value.Meta.Digest = ""
	if err := validateWorkspaceCheckpointParticipantBodyV2(value); err != nil {
		return WorkspaceCheckpointParticipantFactV2{}, err
	}
	digest, err := Digest(WorkspaceCheckpointParticipantDigestDomain, value)
	if err != nil {
		return WorkspaceCheckpointParticipantFactV2{}, err
	}
	value.Meta.Digest = digest
	return value, value.ValidateShape()
}

func (v WorkspaceCheckpointParticipantFactV2) ValidateShape() error {
	if err := v.Meta.ValidateShape(); err != nil {
		return err
	}
	if err := validateWorkspaceCheckpointParticipantBodyV2(v); err != nil {
		return err
	}
	copy := v
	copy.Meta.Digest = ""
	digest, err := Digest(WorkspaceCheckpointParticipantDigestDomain, copy)
	if err != nil || digest != v.Meta.Digest {
		return errors.New("workspace checkpoint participant digest mismatch")
	}
	return nil
}

func (v WorkspaceCheckpointParticipantFactV2) ValidateCurrent(now time.Time) error {
	if err := v.ValidateShape(); err != nil {
		return err
	}
	return v.Meta.ValidateCurrent(now)
}

func (v WorkspaceCheckpointParticipantFactV2) ExactRef() SnapshotArtifactExactRefV2 {
	return SnapshotArtifactExactRefV2{TypeURL: WorkspaceCheckpointParticipantTypeURL, Version: SnapshotArtifactVersion, ID: v.Meta.ID, Revision: v.Meta.Revision, DigestAlgorithm: SnapshotArtifactDigestSHA256, DigestDomain: WorkspaceCheckpointParticipantDigestDomain, Digest: v.Meta.Digest, ExpiresUnixNano: v.Meta.ExpiresUnixNano}
}

func validateWorkspaceCheckpointParticipantBodyV2(v WorkspaceCheckpointParticipantFactV2) error {
	if strings.TrimSpace(v.TenantID) == "" || !ValidDigest(v.ScopeDigest) || strings.TrimSpace(v.RunID) == "" || strings.TrimSpace(v.ParticipantID) == "" || !ValidDigest(v.ParticipantDigest) || v.State != WorkspaceCheckpointParticipantPrepared || v.RequestedNotAfter <= 0 {
		return errors.New("workspace checkpoint participant fact is incomplete")
	}
	for name, ref := range map[string]Ref{"checkpoint attempt": v.CheckpointAttemptRef, "barrier": v.BarrierRef, "effect cut": v.EffectCutRef, "prepared phase": v.PreparedPhaseFactRef} {
		if err := ref.ValidateShape("workspace checkpoint participant " + name); err != nil {
			return err
		}
	}
	if err := v.SnapshotArtifactFactRef.ValidateShape("workspace checkpoint participant snapshot"); err != nil {
		return err
	}
	if err := v.CoverageFactRef.ValidateShape("workspace checkpoint participant coverage"); err != nil {
		return err
	}
	if err := v.SnapshotAggregateRef.ValidateShape(); err != nil {
		return err
	}
	if v.SnapshotArtifactFactRef.TypeURL != SnapshotArtifactFactTypeURL || v.CoverageFactRef.TypeURL != WorkspaceCheckpointCoverageTypeURL || v.CoverageFactRef.DigestDomain != WorkspaceCheckpointCoverageDigestDomain {
		return errors.New("workspace checkpoint participant fact types drifted")
	}
	if v.SnapshotAggregateRef.TenantID != v.TenantID || v.SnapshotAggregateRef.DataDomain != WorkspaceSnapshotDataDomain {
		return errors.New("workspace checkpoint participant snapshot aggregate binding drifted")
	}
	if v.Meta.ExpiresUnixNano > v.RequestedNotAfter || v.Meta.ExpiresUnixNano > v.SnapshotArtifactFactRef.ExpiresUnixNano || v.Meta.ExpiresUnixNano > v.SnapshotAggregateRef.ExpiresUnixNano || v.Meta.ExpiresUnixNano > v.CoverageFactRef.ExpiresUnixNano {
		return errors.New("workspace checkpoint participant TTL exceeds an upstream bound")
	}
	return nil
}

type WorkspaceCheckpointPreparedBundleV2 struct {
	Coverage    WorkspaceCheckpointCoverageFactV2    `json:"coverage"`
	Participant WorkspaceCheckpointParticipantFactV2 `json:"participant"`
}

func (b WorkspaceCheckpointPreparedBundleV2) Clone() WorkspaceCheckpointPreparedBundleV2 {
	b.Coverage = b.Coverage.Clone()
	return b
}

func (b WorkspaceCheckpointPreparedBundleV2) ValidateShape() error {
	if b.Coverage.ValidateShape() != nil || b.Participant.ValidateShape() != nil {
		return errors.New("workspace checkpoint prepared bundle is invalid")
	}
	if b.Coverage.TenantID != b.Participant.TenantID || b.Coverage.ScopeDigest != b.Participant.ScopeDigest || b.Coverage.RunID != b.Participant.RunID || !SameRef(b.Coverage.CheckpointAttemptRef, b.Participant.CheckpointAttemptRef) || !SameRef(b.Coverage.BarrierRef, b.Participant.BarrierRef) || !SameRef(b.Coverage.EffectCutRef, b.Participant.EffectCutRef) || b.Coverage.ParticipantID != b.Participant.ParticipantID || b.Coverage.ParticipantDigest != b.Participant.ParticipantDigest || !SameRef(b.Coverage.PreparedPhaseFactRef, b.Participant.PreparedPhaseFactRef) || !SameSnapshotArtifactExactRef(b.Coverage.SnapshotArtifactFactRef, b.Participant.SnapshotArtifactFactRef) || !SameSnapshotArtifactAggregateRef(b.Coverage.SnapshotAggregateRef, b.Participant.SnapshotAggregateRef) || !SameSnapshotArtifactExactRef(b.Coverage.ExactRef(), b.Participant.CoverageFactRef) {
		return errors.New("workspace checkpoint prepared bundle binding drifted")
	}
	return nil
}

func compareWorkspaceCheckpointRefV2(a, b Ref) int {
	if result := strings.Compare(a.ID, b.ID); result != 0 {
		return result
	}
	if a.Revision < b.Revision {
		return -1
	}
	if a.Revision > b.Revision {
		return 1
	}
	return strings.Compare(a.Digest, b.Digest)
}
