package core

import (
	"time"
)

// TimelinePoint preserves both authoritative ledger order and provider-local
// source order. Wall-clock timestamps are evidence, never the ordering key.
type TimelinePoint struct {
	LedgerScope    string    `json:"ledger_scope"`
	LedgerSequence uint64    `json:"ledger_sequence"`
	SourceID       string    `json:"source_id"`
	SourceEpoch    Epoch     `json:"source_epoch"`
	SourceSequence uint64    `json:"source_sequence"`
	EventID        string    `json:"event_id"`
	CausationID    string    `json:"causation_id"`
	CorrelationID  string    `json:"correlation_id"`
	ObservedAt     time.Time `json:"observed_at"`
	IngestedAt     time.Time `json:"ingested_at"`
}

func (p TimelinePoint) Validate() error {
	if blank(p.LedgerScope) || p.LedgerSequence == 0 || blank(p.SourceID) || p.SourceEpoch == 0 || p.SourceSequence == 0 || blank(p.EventID) {
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "timeline point requires ledger and source identities with non-zero sequences")
	}
	if p.ObservedAt.IsZero() || p.IngestedAt.IsZero() {
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "timeline point requires observed and ingested timestamps")
	}
	return nil
}

type CheckpointConsistency string

const (
	CheckpointConsistent    CheckpointConsistency = "consistent"
	CheckpointPartial       CheckpointConsistency = "partial"
	CheckpointIndeterminate CheckpointConsistency = "indeterminate"
	CheckpointRejected      CheckpointConsistency = "rejected"
)

type CheckpointParticipantState string

const (
	CheckpointParticipantPrepared  CheckpointParticipantState = "prepared"
	CheckpointParticipantCommitted CheckpointParticipantState = "committed"
	CheckpointParticipantAborted   CheckpointParticipantState = "aborted"
	CheckpointParticipantUnknown   CheckpointParticipantState = "unknown"
)

type EffectWatermarks struct {
	Accepted   uint64 `json:"effect_intent_accept_watermark"`
	Dispatched uint64 `json:"effect_dispatch_watermark"`
	Settled    uint64 `json:"effect_settlement_watermark"`
	Remote     uint64 `json:"remote_continuation_watermark"`
}

func (w EffectWatermarks) Validate() error {
	if w.Dispatched > w.Accepted || w.Settled > w.Dispatched {
		return NewError(ErrorPreconditionFailed, ReasonCheckpointInconsistent, "effect watermarks must satisfy accepted >= dispatched >= settled")
	}
	return nil
}

type CheckpointParticipantSnapshot struct {
	ComponentID    string                     `json:"component_id"`
	ComponentKind  string                     `json:"component_kind"`
	Required       bool                       `json:"required"`
	State          CheckpointParticipantState `json:"state"`
	SnapshotRef    string                     `json:"snapshot_ref,omitempty"`
	SnapshotDigest Digest                     `json:"snapshot_digest,omitempty"`
}

func (p CheckpointParticipantSnapshot) Validate() error {
	if blank(p.ComponentID) || blank(p.ComponentKind) {
		return NewError(ErrorInvalidArgument, ReasonInvalidReference, "checkpoint participant identity is required")
	}
	switch p.State {
	case CheckpointParticipantPrepared, CheckpointParticipantCommitted:
		if blank(p.SnapshotRef) {
			return NewError(ErrorPreconditionFailed, ReasonCheckpointInconsistent, "prepared participant requires a snapshot reference")
		}
		if err := p.SnapshotDigest.Validate(); err != nil {
			return err
		}
	case CheckpointParticipantAborted, CheckpointParticipantUnknown:
		return nil
	default:
		return NewError(ErrorInvalidArgument, ReasonCheckpointInconsistent, "unknown checkpoint participant state")
	}
	return nil
}

type CheckpointSet struct {
	ID             string                          `json:"checkpoint_id"`
	Epoch          Epoch                           `json:"checkpoint_epoch"`
	BarrierID      string                          `json:"barrier_id"`
	Scope          ExecutionScope                  `json:"scope"`
	PlanDigest     Digest                          `json:"plan_digest"`
	ProfileDigest  Digest                          `json:"profile_digest"`
	ContextDigest  Digest                          `json:"context_digest"`
	AuthorityEpoch Epoch                           `json:"authority_epoch"`
	Effects        EffectWatermarks                `json:"effect_watermarks"`
	EventWatermark TimelinePoint                   `json:"event_watermark"`
	Participants   []CheckpointParticipantSnapshot `json:"participants"`
	Consistency    CheckpointConsistency           `json:"consistency_status"`
	CreatedAt      time.Time                       `json:"created_at"`
}

func (c CheckpointSet) Validate() error {
	if blank(c.ID) || c.Epoch == 0 || blank(c.BarrierID) || c.CreatedAt.IsZero() {
		return NewError(ErrorInvalidArgument, ReasonCheckpointInconsistent, "checkpoint identity, epoch, barrier and creation time are required")
	}
	if err := c.Scope.Validate(); err != nil {
		return err
	}
	for _, digest := range []Digest{c.PlanDigest, c.ProfileDigest, c.ContextDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if c.PlanDigest != c.Scope.Lineage.PlanDigest || c.AuthorityEpoch != c.Scope.AuthorityEpoch {
		return NewError(ErrorPreconditionFailed, ReasonCheckpointInconsistent, "checkpoint plan and authority must match the execution scope")
	}
	if err := c.Effects.Validate(); err != nil {
		return err
	}
	if err := c.EventWatermark.Validate(); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(c.Participants))
	if c.Consistency == CheckpointConsistent && len(c.Participants) == 0 {
		return NewError(ErrorPreconditionFailed, ReasonCheckpointInconsistent, "consistent checkpoint requires at least one participant")
	}
	for _, participant := range c.Participants {
		if err := participant.Validate(); err != nil {
			return err
		}
		if _, exists := seen[participant.ComponentID]; exists {
			return NewError(ErrorConflict, ReasonCheckpointInconsistent, "duplicate checkpoint participant")
		}
		seen[participant.ComponentID] = struct{}{}
		if c.Consistency == CheckpointConsistent && participant.Required && participant.State != CheckpointParticipantCommitted {
			return NewError(ErrorPreconditionFailed, ReasonCheckpointInconsistent, "consistent checkpoint requires every required participant to commit")
		}
	}
	switch c.Consistency {
	case CheckpointConsistent, CheckpointPartial, CheckpointIndeterminate, CheckpointRejected:
		return nil
	default:
		return NewError(ErrorInvalidArgument, ReasonCheckpointInconsistent, "unknown checkpoint consistency")
	}
}

type RestoreRequest struct {
	Checkpoint            CheckpointSet `json:"checkpoint"`
	NewInstance           InstanceRef   `json:"new_instance"`
	CurrentPlanDigest     Digest        `json:"current_plan_digest"`
	CurrentProfileDigest  Digest        `json:"current_profile_digest"`
	CurrentAuthorityEpoch Epoch         `json:"current_authority_epoch"`
}

func (r RestoreRequest) Validate() error {
	if err := r.Checkpoint.Validate(); err != nil {
		return err
	}
	if r.Checkpoint.Consistency != CheckpointConsistent {
		return NewError(ErrorPreconditionFailed, ReasonCheckpointInconsistent, "automatic restore requires a consistent checkpoint")
	}
	if err := r.NewInstance.Validate(); err != nil {
		return err
	}
	if r.NewInstance.ID == r.Checkpoint.Scope.Instance.ID || r.NewInstance.Epoch <= r.Checkpoint.Scope.Instance.Epoch {
		return NewError(ErrorPreconditionFailed, ReasonRestoreIncompatible, "restore must create a new instance id and higher epoch")
	}
	if r.CurrentPlanDigest != r.Checkpoint.PlanDigest || r.CurrentProfileDigest != r.Checkpoint.ProfileDigest || r.CurrentAuthorityEpoch == 0 {
		return NewError(ErrorPreconditionFailed, ReasonRestoreIncompatible, "restore plan, profile or authority is incompatible")
	}
	return nil
}
