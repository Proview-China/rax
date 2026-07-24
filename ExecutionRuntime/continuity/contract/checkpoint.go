package contract

import "sort"

type ParticipantState string

const (
	ParticipantPrepared    ParticipantState = "prepared"
	ParticipantUnsupported ParticipantState = "unsupported"
	ParticipantPartial     ParticipantState = "partial"
	ParticipantUnknown     ParticipantState = "unknown"
	ParticipantCommitted   ParticipantState = "committed"
	ParticipantAborted     ParticipantState = "aborted"
)

type SnapshotBinding struct {
	ParticipantID    string           `json:"participant_id"`
	Required         bool             `json:"required"`
	State            ParticipantState `json:"state"`
	SnapshotRef      string           `json:"snapshot_ref,omitempty"`
	SnapshotRevision uint64           `json:"snapshot_revision,omitempty"`
	SnapshotDigest   string           `json:"snapshot_digest,omitempty"`
	CoverageSchema   string           `json:"coverage_schema,omitempty"`
	CoverageDigest   string           `json:"coverage_digest,omitempty"`
	StorageRef       string           `json:"storage_ref,omitempty"`
	EncryptionRef    string           `json:"encryption_ref,omitempty"`
	EvidenceRef      string           `json:"evidence_ref,omitempty"`
	InspectFactRef   string           `json:"inspect_fact_ref,omitempty"`
	ResidualRefs     []ResidualRef    `json:"residual_refs"`
}

func (s SnapshotBinding) Validate() error {
	if err := ValidateToken("participant_id", s.ParticipantID); err != nil {
		return err
	}
	switch s.State {
	case ParticipantPrepared, ParticipantCommitted:
		for field, value := range map[string]string{
			"snapshot_ref": s.SnapshotRef, "snapshot_digest": s.SnapshotDigest,
			"coverage_schema": s.CoverageSchema, "coverage_digest": s.CoverageDigest,
			"storage_ref": s.StorageRef, "evidence_ref": s.EvidenceRef,
			"inspect_fact_ref": s.InspectFactRef,
		} {
			if err := ValidateToken(field, value); err != nil {
				return err
			}
		}
		if s.SnapshotRevision == 0 {
			return NewError(ErrInvalidArgument, "snapshot_revision", "must be non-zero")
		}
	case ParticipantUnsupported, ParticipantPartial, ParticipantUnknown, ParticipantAborted:
	default:
		return NewError(ErrInvalidArgument, "participant_state", "unknown state")
	}
	for _, residual := range s.ResidualRefs {
		if err := residual.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type ManifestState string

const (
	ManifestVerifiedCandidate       ManifestState = "verified_candidate"
	ManifestDiagnosticPartial       ManifestState = "diagnostic_partial"
	ManifestDiagnosticIndeterminate ManifestState = "diagnostic_indeterminate"
	ManifestRejected                ManifestState = "rejected"
)

type CheckpointManifest struct {
	ContractVersion     string            `json:"contract_version"`
	CheckpointID        string            `json:"checkpoint_id"`
	Epoch               uint64            `json:"epoch"`
	BarrierID           string            `json:"barrier_id"`
	BarrierRevision     uint64            `json:"barrier_revision"`
	Scope               Scope             `json:"scope"`
	RuntimeStateRef     string            `json:"runtime_state_ref"`
	RunSessionRef       string            `json:"run_session_ref"`
	PlanDigest          string            `json:"plan_digest"`
	ProfileDigest       string            `json:"profile_digest"`
	BindingDigest       string            `json:"binding_digest"`
	ContextGeneration   string            `json:"context_generation"`
	ContextMaterialized bool              `json:"context_materialized"`
	ToolSurfaceDigest   string            `json:"tool_surface_digest"`
	AuthorityDigest     string            `json:"authority_digest"`
	EvidenceWatermark   uint64            `json:"evidence_watermark"`
	EffectCutRef        string            `json:"effect_cut_ref"`
	EffectCutAccepted   bool              `json:"effect_cut_accepted"`
	Participants        []SnapshotBinding `json:"participants"`
	ResidualRefs        []ResidualRef     `json:"residual_refs"`
	Revision            uint64            `json:"revision"`
	CreatedUnixNano     int64             `json:"created_unix_nano"`
	Digest              string            `json:"digest"`
}

func (m CheckpointManifest) CanonicalDigest() (string, error) {
	copy := m
	copy.Digest = ""
	copy.Participants = make([]SnapshotBinding, len(m.Participants))
	for i, participant := range m.Participants {
		copy.Participants[i] = participant
		residuals, err := NormalizeResiduals(participant.ResidualRefs)
		if err != nil {
			return "", err
		}
		copy.Participants[i].ResidualRefs = residuals
	}
	sort.Slice(copy.Participants, func(i, j int) bool {
		return copy.Participants[i].ParticipantID < copy.Participants[j].ParticipantID
	})
	residuals, err := NormalizeResiduals(m.ResidualRefs)
	if err != nil {
		return "", err
	}
	copy.ResidualRefs = residuals
	return CanonicalDigest(copy)
}

func ValidateCheckpointManifest(m CheckpointManifest) (ManifestState, error) {
	if m.ContractVersion != ContractVersion {
		return ManifestRejected, NewError(ErrInvalidArgument, "contract_version", "unsupported version")
	}
	for field, value := range map[string]string{
		"checkpoint_id": m.CheckpointID, "barrier_id": m.BarrierID,
		"runtime_state_ref": m.RuntimeStateRef, "run_session_ref": m.RunSessionRef,
		"plan_digest": m.PlanDigest, "profile_digest": m.ProfileDigest,
		"binding_digest": m.BindingDigest, "context_generation": m.ContextGeneration,
		"tool_surface_digest": m.ToolSurfaceDigest, "authority_digest": m.AuthorityDigest,
		"effect_cut_ref": m.EffectCutRef,
	} {
		if err := ValidateToken(field, value); err != nil {
			return ManifestRejected, err
		}
	}
	if err := m.Scope.Validate(); err != nil {
		return ManifestRejected, err
	}
	if m.Epoch == 0 || m.BarrierRevision == 0 || m.EvidenceWatermark == 0 || m.Revision == 0 || m.CreatedUnixNano <= 0 {
		return ManifestRejected, NewError(ErrInvalidArgument, "manifest", "epochs, revisions, watermark, and creation time are required")
	}
	if len(m.Participants) == 0 {
		return ManifestRejected, NewError(ErrInvalidArgument, "participants", "at least one participant is required")
	}
	seen := map[string]struct{}{}
	state := ManifestVerifiedCandidate
	for _, participant := range m.Participants {
		if err := participant.Validate(); err != nil {
			return ManifestRejected, err
		}
		if _, ok := seen[participant.ParticipantID]; ok {
			return ManifestRejected, NewError(ErrInvalidArgument, "participants", "duplicate participant")
		}
		seen[participant.ParticipantID] = struct{}{}
		if participant.Required {
			switch participant.State {
			case ParticipantUnknown:
				state = ManifestDiagnosticIndeterminate
			case ParticipantUnsupported, ParticipantPartial, ParticipantAborted:
				if state != ManifestDiagnosticIndeterminate {
					state = ManifestDiagnosticPartial
				}
			}
		}
	}
	for _, residual := range m.ResidualRefs {
		if err := residual.Validate(); err != nil {
			return ManifestRejected, err
		}
	}
	if !m.EffectCutAccepted {
		state = ManifestDiagnosticIndeterminate
	}
	if !m.ContextMaterialized {
		if len(m.ResidualRefs) == 0 {
			return ManifestRejected, NewError(ErrCheckpointIndeterminate, "context_generation", "unmaterialized context requires an explicit residual")
		}
		state = ManifestDiagnosticIndeterminate
	}
	expected, err := m.CanonicalDigest()
	if err != nil {
		return ManifestRejected, err
	}
	if m.Digest == "" || m.Digest != expected {
		return ManifestRejected, NewError(ErrInvalidArgument, "manifest_digest", "canonical digest mismatch")
	}
	return state, nil
}
