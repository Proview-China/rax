package contract

import (
	"errors"
	"slices"
	"strings"
	"time"
)

type PrepareWorkspaceCheckpointParticipantRequestV2 struct {
	StableID                string                     `json:"stable_id"`
	TenantID                string                     `json:"tenant_id"`
	ScopeDigest             string                     `json:"scope_digest"`
	RunID                   string                     `json:"run_id"`
	CheckpointAttemptRef    Ref                        `json:"checkpoint_attempt_ref"`
	BarrierRef              Ref                        `json:"barrier_ref"`
	EffectCutRef            Ref                        `json:"effect_cut_ref"`
	ParticipantID           string                     `json:"participant_id"`
	ParticipantDigest       string                     `json:"participant_digest"`
	PreparedPhaseFactRef    Ref                        `json:"prepared_phase_fact_ref"`
	SnapshotArtifactFactRef SnapshotArtifactExactRefV2 `json:"snapshot_artifact_fact_ref"`
	CoveragePolicyRef       Ref                        `json:"coverage_policy_ref"`
	RequestedNotAfter       int64                      `json:"requested_not_after_unix_nano"`
}

func (r PrepareWorkspaceCheckpointParticipantRequestV2) ValidateCurrent(now time.Time) error {
	if strings.TrimSpace(r.StableID) == "" || strings.TrimSpace(r.TenantID) == "" || !ValidDigest(r.ScopeDigest) || strings.TrimSpace(r.RunID) == "" || strings.TrimSpace(r.ParticipantID) == "" || !ValidDigest(r.ParticipantDigest) || r.RequestedNotAfter <= 0 || now.IsZero() || now.UnixNano() >= r.RequestedNotAfter {
		return errors.New("prepare workspace checkpoint request is incomplete or stale")
	}
	for name, ref := range map[string]Ref{"checkpoint attempt": r.CheckpointAttemptRef, "barrier": r.BarrierRef, "effect cut": r.EffectCutRef, "prepared phase": r.PreparedPhaseFactRef, "coverage policy": r.CoveragePolicyRef} {
		if err := ref.ValidateShape("prepare workspace checkpoint " + name); err != nil {
			return err
		}
	}
	if err := r.SnapshotArtifactFactRef.ValidateCurrent("prepare workspace checkpoint snapshot", now); err != nil {
		return err
	}
	if r.SnapshotArtifactFactRef.TypeURL != SnapshotArtifactFactTypeURL || r.SnapshotArtifactFactRef.DigestDomain != SnapshotArtifactFactDomain {
		return errors.New("prepare workspace checkpoint requires an exact snapshot artifact fact")
	}
	return nil
}

type WorkspaceCheckpointPreparationCurrentProjectionV2 struct {
	TenantID                string                         `json:"tenant_id"`
	ScopeDigest             string                         `json:"scope_digest"`
	RunID                   string                         `json:"run_id"`
	CheckpointAttemptRef    Ref                            `json:"checkpoint_attempt_ref"`
	BarrierRef              Ref                            `json:"barrier_ref"`
	EffectCutRef            Ref                            `json:"effect_cut_ref"`
	ParticipantID           string                         `json:"participant_id"`
	ParticipantDigest       string                         `json:"participant_digest"`
	PreparedPhaseFactRef    Ref                            `json:"prepared_phase_fact_ref"`
	SnapshotArtifactFactRef SnapshotArtifactExactRefV2     `json:"snapshot_artifact_fact_ref"`
	SnapshotAggregateRef    SnapshotArtifactAggregateRefV2 `json:"snapshot_aggregate_ref"`
	CoveragePolicyRef       Ref                            `json:"coverage_policy_ref"`
	Included                []string                       `json:"included"`
	DeclaredExcluded        []string                       `json:"declared_excluded"`
	ResidualRefs            []Ref                          `json:"residual_refs"`
	CheckedUnixNano         int64                          `json:"checked_unix_nano"`
	ExpiresUnixNano         int64                          `json:"expires_unix_nano"`
	ProjectionDigest        string                         `json:"projection_digest"`
}

func (p WorkspaceCheckpointPreparationCurrentProjectionV2) Clone() WorkspaceCheckpointPreparationCurrentProjectionV2 {
	p.Included = append([]string(nil), p.Included...)
	p.DeclaredExcluded = append([]string(nil), p.DeclaredExcluded...)
	p.ResidualRefs = append([]Ref(nil), p.ResidualRefs...)
	return p
}

func SealWorkspaceCheckpointPreparationCurrentProjectionV2(value WorkspaceCheckpointPreparationCurrentProjectionV2, now time.Time) (WorkspaceCheckpointPreparationCurrentProjectionV2, error) {
	value = value.Clone()
	slices.Sort(value.Included)
	slices.Sort(value.DeclaredExcluded)
	slices.SortFunc(value.ResidualRefs, compareWorkspaceCheckpointRefV2)
	value.ProjectionDigest = ""
	digest, err := Digest("praxis.sandbox/workspace-checkpoint-preparation-current/body/v2", value)
	if err != nil {
		return WorkspaceCheckpointPreparationCurrentProjectionV2{}, err
	}
	value.ProjectionDigest = digest
	return value, value.ValidateCurrent(now)
}

func (p WorkspaceCheckpointPreparationCurrentProjectionV2) ValidateCurrent(now time.Time) error {
	if strings.TrimSpace(p.TenantID) == "" || !ValidDigest(p.ScopeDigest) || strings.TrimSpace(p.RunID) == "" || strings.TrimSpace(p.ParticipantID) == "" || !ValidDigest(p.ParticipantDigest) || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || !ValidDigest(p.ProjectionDigest) || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || now.UnixNano() >= p.ExpiresUnixNano {
		return errors.New("workspace checkpoint preparation projection is incomplete or stale")
	}
	for name, ref := range map[string]Ref{"checkpoint attempt": p.CheckpointAttemptRef, "barrier": p.BarrierRef, "effect cut": p.EffectCutRef, "prepared phase": p.PreparedPhaseFactRef, "coverage policy": p.CoveragePolicyRef} {
		if err := ref.ValidateShape("workspace checkpoint preparation " + name); err != nil {
			return err
		}
	}
	if err := p.SnapshotArtifactFactRef.ValidateCurrent("workspace checkpoint preparation snapshot", now); err != nil {
		return err
	}
	if p.SnapshotArtifactFactRef.TypeURL != SnapshotArtifactFactTypeURL || p.SnapshotArtifactFactRef.DigestDomain != SnapshotArtifactFactDomain || p.ExpiresUnixNano > p.SnapshotArtifactFactRef.ExpiresUnixNano {
		return errors.New("workspace checkpoint preparation snapshot binding drifted")
	}
	if err := p.SnapshotAggregateRef.ValidateShape(); err != nil {
		return err
	}
	if p.SnapshotAggregateRef.TenantID != p.TenantID || p.SnapshotAggregateRef.DataDomain != WorkspaceSnapshotDataDomain || p.SnapshotAggregateRef.ExpiresUnixNano < p.ExpiresUnixNano {
		return errors.New("workspace checkpoint preparation snapshot aggregate binding drifted")
	}
	if len(p.Included) == 0 || ValidateSortedUnique(p.Included, "workspace checkpoint preparation included coverage") != nil || ValidateSortedUnique(p.DeclaredExcluded, "workspace checkpoint preparation declared exclusions") != nil {
		return errors.New("workspace checkpoint preparation coverage is empty or non-canonical")
	}
	if !slices.IsSortedFunc(p.ResidualRefs, compareWorkspaceCheckpointRefV2) {
		return errors.New("workspace checkpoint preparation residual refs are non-canonical")
	}
	for index, ref := range p.ResidualRefs {
		if err := ref.ValidateShape("workspace checkpoint preparation residual"); err != nil {
			return err
		}
		if index > 0 && SameRef(p.ResidualRefs[index-1], ref) {
			return errors.New("workspace checkpoint preparation residual refs contain duplicates")
		}
	}
	copy := p.Clone()
	copy.ProjectionDigest = ""
	digest, err := Digest("praxis.sandbox/workspace-checkpoint-preparation-current/body/v2", copy)
	if err != nil || digest != p.ProjectionDigest {
		return errors.New("workspace checkpoint preparation projection digest mismatch")
	}
	return nil
}

func (p WorkspaceCheckpointPreparationCurrentProjectionV2) MatchesRequest(r PrepareWorkspaceCheckpointParticipantRequestV2) bool {
	return p.TenantID == r.TenantID && p.ScopeDigest == r.ScopeDigest && p.RunID == r.RunID &&
		SameRef(p.CheckpointAttemptRef, r.CheckpointAttemptRef) && SameRef(p.BarrierRef, r.BarrierRef) &&
		SameRef(p.EffectCutRef, r.EffectCutRef) && p.ParticipantID == r.ParticipantID &&
		p.ParticipantDigest == r.ParticipantDigest && SameRef(p.PreparedPhaseFactRef, r.PreparedPhaseFactRef) &&
		SameSnapshotArtifactExactRef(p.SnapshotArtifactFactRef, r.SnapshotArtifactFactRef) &&
		SameRef(p.CoveragePolicyRef, r.CoveragePolicyRef)
}

type InspectWorkspaceCheckpointPreparedRequestV2 struct {
	TenantID            string `json:"tenant_id"`
	ScopeDigest         string `json:"scope_digest"`
	CheckpointAttemptID string `json:"checkpoint_attempt_id"`
	ParticipantID       string `json:"participant_id"`
}

func (r InspectWorkspaceCheckpointPreparedRequestV2) Validate() error {
	if strings.TrimSpace(r.TenantID) == "" || !ValidDigest(r.ScopeDigest) || strings.TrimSpace(r.CheckpointAttemptID) == "" || strings.TrimSpace(r.ParticipantID) == "" {
		return errors.New("workspace checkpoint prepared inspect coordinate is incomplete")
	}
	return nil
}

type InspectWorkspaceCheckpointFactRequestV2 struct {
	TenantID    string                     `json:"tenant_id"`
	ScopeDigest string                     `json:"scope_digest"`
	ExpectedRef SnapshotArtifactExactRefV2 `json:"expected_ref"`
}

func (r InspectWorkspaceCheckpointFactRequestV2) Validate(typeURL, digestDomain, name string) error {
	if strings.TrimSpace(r.TenantID) == "" || !ValidDigest(r.ScopeDigest) {
		return errors.New(name + " inspect scope is incomplete")
	}
	if err := r.ExpectedRef.ValidateShape(name + " inspect"); err != nil {
		return err
	}
	if r.ExpectedRef.TypeURL != typeURL || r.ExpectedRef.DigestDomain != digestDomain {
		return errors.New(name + " inspect exact ref has the wrong type")
	}
	return nil
}
