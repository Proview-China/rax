package contract

import (
	"errors"
	"strings"
	"time"
)

const (
	CheckpointPhaseDomainResultTypeURLV2      = "praxis.sandbox/checkpoint-phase-domain-result/v2"
	CheckpointPhaseDomainResultDigestDomainV2 = "praxis.sandbox/checkpoint-phase-domain-result/body/v2"
)

// CheckpointPhaseDomainResultV2 is the Sandbox-owned result after Provider
// execution, independent Inspect, and Runtime Evidence consumption. It is
// immutable and deliberately contains no Runtime Settlement or Apply result.
type CheckpointPhaseDomainResultV2 struct {
	Meta                 Meta                       `json:"meta"`
	ReservationRef       Ref                        `json:"reservation_ref"`
	TenantID             string                     `json:"tenant_id"`
	ParticipantRef       Ref                        `json:"participant_ref"`
	CheckpointAttemptRef Ref                        `json:"checkpoint_attempt_ref"`
	Phase                CheckpointPhase            `json:"phase"`
	PreviousPresence     CheckpointPresence         `json:"previous_presence"`
	PreviousPhase        *CheckpointPhaseClosureRef `json:"previous_phase,omitempty"`
	OperationID          string                     `json:"operation_id"`
	EffectID             string                     `json:"effect_id"`
	AttemptID            string                     `json:"attempt_id"`
	State                CheckpointPhaseState       `json:"state"`
	ProviderAttemptRef   Ref                        `json:"provider_attempt_ref"`
	ProviderObservation  Ref                        `json:"provider_observation_ref"`
	ProviderReceipt      Ref                        `json:"provider_receipt_ref"`
	EvidenceConsumption  Ref                        `json:"evidence_consumption_ref"`
}

// CheckpointPhaseResultCurrentProjectionV2 is produced by the Sandbox
// actual-point adapter from exact Provider Inspect and consumed Runtime
// Evidence current. Caller booleans and raw state strings are absent.
type CheckpointPhaseResultCurrentProjectionV2 struct {
	ReservationRef      Ref                  `json:"reservation_ref"`
	State               CheckpointPhaseState `json:"state"`
	ProviderAttemptRef  Ref                  `json:"provider_attempt_ref"`
	ProviderObservation Ref                  `json:"provider_observation_ref"`
	ProviderReceipt     Ref                  `json:"provider_receipt_ref"`
	EvidenceConsumption Ref                  `json:"evidence_consumption_ref"`
	CheckedUnixNano     int64                `json:"checked_unix_nano"`
	ExpiresUnixNano     int64                `json:"expires_unix_nano"`
	ProjectionDigest    string               `json:"projection_digest"`
}

func SealCheckpointPhaseResultCurrentProjectionV2(value CheckpointPhaseResultCurrentProjectionV2) (CheckpointPhaseResultCurrentProjectionV2, error) {
	value.ProjectionDigest = ""
	digest, err := Digest("praxis.sandbox/checkpoint-phase-result-current/body/v2", value)
	if err != nil {
		return CheckpointPhaseResultCurrentProjectionV2{}, err
	}
	value.ProjectionDigest = digest
	return value, value.ValidateShape()
}

func (v CheckpointPhaseResultCurrentProjectionV2) ValidateShape() error {
	if v.ReservationRef.ValidateShape("checkpoint phase result reservation") != nil || !checkpointResultStateValidV2(v.State) || v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano || !ValidDigest(v.ProjectionDigest) {
		return errors.New("checkpoint phase result current projection is incomplete")
	}
	for name, ref := range map[string]Ref{"provider attempt": v.ProviderAttemptRef, "provider observation": v.ProviderObservation, "provider receipt": v.ProviderReceipt, "evidence consumption": v.EvidenceConsumption} {
		if err := ref.ValidateShape(name); err != nil {
			return err
		}
	}
	copy := v
	copy.ProjectionDigest = ""
	digest, err := Digest("praxis.sandbox/checkpoint-phase-result-current/body/v2", copy)
	if err != nil || digest != v.ProjectionDigest {
		return errors.New("checkpoint phase result current projection digest mismatch")
	}
	return nil
}

func checkpointResultStateValidV2(state CheckpointPhaseState) bool {
	switch state {
	case CheckpointPhasePrepared, CheckpointPhaseFailed, CheckpointPhaseNotApplied, CheckpointPhaseUnknown, CheckpointPhaseCommitted, CheckpointPhaseAborted, CheckpointPhaseIndeterminate:
		return true
	default:
		return false
	}
}

func (v CheckpointPhaseResultCurrentProjectionV2) ValidateCurrent(now time.Time) error {
	if err := v.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < v.CheckedUnixNano || now.UnixNano() >= v.ExpiresUnixNano {
		return errors.New("checkpoint phase result current projection is stale")
	}
	return nil
}

type RecordCheckpointPhaseDomainResultRequestV2 struct {
	ReservationRef           Ref    `json:"reservation_ref"`
	ExpectedProjectionDigest string `json:"expected_projection_digest"`
	RequestedNotAfter        int64  `json:"requested_not_after_unix_nano"`
}

func (v RecordCheckpointPhaseDomainResultRequestV2) ValidateCurrent(now time.Time) error {
	if v.ReservationRef.ValidateShape("checkpoint DomainResult reservation") != nil || !ValidDigest(v.ExpectedProjectionDigest) || v.RequestedNotAfter <= 0 || now.IsZero() || now.UnixNano() >= v.RequestedNotAfter {
		return errors.New("checkpoint DomainResult record request is incomplete or stale")
	}
	return nil
}

func SealCheckpointPhaseDomainResultV2(value CheckpointPhaseDomainResultV2) (CheckpointPhaseDomainResultV2, error) {
	var err error
	value, err = cloneCheckpoint(value)
	if err != nil {
		return CheckpointPhaseDomainResultV2{}, err
	}
	value.Meta.ContractVersion = ContractFamily
	value.Meta.Digest = ""
	digest, err := Digest(CheckpointPhaseDomainResultDigestDomainV2, value)
	if err != nil {
		return CheckpointPhaseDomainResultV2{}, err
	}
	value.Meta.Digest = digest
	return value, value.ValidateShape()
}

func (v CheckpointPhaseDomainResultV2) ValidateShape() error {
	if v.Meta.ValidateShape() != nil || strings.TrimSpace(v.TenantID) == "" || strings.TrimSpace(v.OperationID) == "" || strings.TrimSpace(v.EffectID) == "" || strings.TrimSpace(v.AttemptID) == "" || v.Phase.Validate() != nil || v.State.ValidateFor(v.Phase) != nil || v.PreviousPresence.Validate() != nil {
		return errors.New("checkpoint phase DomainResult coordinates are incomplete")
	}
	for name, ref := range map[string]Ref{
		"reservation": v.ReservationRef, "participant": v.ParticipantRef, "checkpoint attempt": v.CheckpointAttemptRef,
		"provider attempt": v.ProviderAttemptRef, "provider observation": v.ProviderObservation,
		"provider receipt": v.ProviderReceipt, "evidence consumption": v.EvidenceConsumption,
	} {
		if err := ref.ValidateShape(name); err != nil {
			return err
		}
	}
	if err := validateCheckpointAttemptIdentity(v.AttemptID, v.CheckpointAttemptRef); err != nil {
		return err
	}
	if v.Phase == CheckpointPhasePrepare {
		if v.PreviousPresence != CheckpointAbsent || v.PreviousPhase != nil {
			return errors.New("checkpoint prepare DomainResult cannot carry PreviousPhase")
		}
	} else if v.PreviousPresence != CheckpointPresent || v.PreviousPhase == nil || v.PreviousPhase.ValidateShape() != nil || v.PreviousPhase.Phase != CheckpointPhasePrepare || v.PreviousPhase.State != CheckpointPhasePrepared {
		return errors.New("checkpoint successor DomainResult requires the exact prepared closure")
	}
	copy := v
	copy.Meta.Digest = ""
	digest, err := Digest(CheckpointPhaseDomainResultDigestDomainV2, copy)
	if err != nil || digest != v.Meta.Digest {
		return errors.New("checkpoint phase DomainResult digest mismatch")
	}
	return nil
}

func (v CheckpointPhaseDomainResultV2) ValidateCurrent(now time.Time) error {
	if err := v.ValidateShape(); err != nil {
		return err
	}
	return v.Meta.ValidateCurrent(now)
}

func (v CheckpointPhaseDomainResultV2) ExactRef() SnapshotArtifactExactRefV2 {
	return SnapshotArtifactExactRefV2{TypeURL: CheckpointPhaseDomainResultTypeURLV2, Version: 2, ID: v.Meta.ID, Revision: v.Meta.Revision, DigestAlgorithm: SnapshotArtifactDigestSHA256, DigestDomain: CheckpointPhaseDomainResultDigestDomainV2, Digest: v.Meta.Digest, ExpiresUnixNano: v.Meta.ExpiresUnixNano}
}

type CheckpointPhaseApplySettlementV2 struct {
	DomainResultRef      SnapshotArtifactExactRefV2 `json:"domain_result_ref"`
	RuntimeSettlementRef Ref                        `json:"runtime_settlement_ref"`
}

func (v CheckpointPhaseApplySettlementV2) ValidateShape() error {
	if v.DomainResultRef.ValidateShape("checkpoint phase DomainResult") != nil || v.DomainResultRef.TypeURL != CheckpointPhaseDomainResultTypeURLV2 || v.RuntimeSettlementRef.ValidateShape("Runtime checkpoint Settlement") != nil {
		return errors.New("checkpoint phase ApplySettlement closure is incomplete")
	}
	return nil
}

type CheckpointPhaseSettlementCurrentProjectionV2 struct {
	DomainResultRef      SnapshotArtifactExactRefV2 `json:"domain_result_ref"`
	RuntimeSettlementRef Ref                        `json:"runtime_settlement_ref"`
	CheckedUnixNano      int64                      `json:"checked_unix_nano"`
	ExpiresUnixNano      int64                      `json:"expires_unix_nano"`
	ProjectionDigest     string                     `json:"projection_digest"`
}

func SealCheckpointPhaseSettlementCurrentProjectionV2(value CheckpointPhaseSettlementCurrentProjectionV2) (CheckpointPhaseSettlementCurrentProjectionV2, error) {
	value.ProjectionDigest = ""
	digest, err := Digest("praxis.sandbox/checkpoint-phase-settlement-current/body/v2", value)
	if err != nil {
		return CheckpointPhaseSettlementCurrentProjectionV2{}, err
	}
	value.ProjectionDigest = digest
	return value, value.ValidateShape()
}

func (v CheckpointPhaseSettlementCurrentProjectionV2) ValidateShape() error {
	if v.DomainResultRef.ValidateShape("checkpoint phase DomainResult") != nil || v.DomainResultRef.TypeURL != CheckpointPhaseDomainResultTypeURLV2 || v.RuntimeSettlementRef.ValidateShape("Runtime checkpoint Settlement") != nil || v.CheckedUnixNano <= 0 || v.ExpiresUnixNano <= v.CheckedUnixNano || !ValidDigest(v.ProjectionDigest) {
		return errors.New("checkpoint phase Settlement current projection is incomplete")
	}
	copy := v
	copy.ProjectionDigest = ""
	digest, err := Digest("praxis.sandbox/checkpoint-phase-settlement-current/body/v2", copy)
	if err != nil || digest != v.ProjectionDigest {
		return errors.New("checkpoint phase Settlement current projection digest mismatch")
	}
	return nil
}

func (v CheckpointPhaseSettlementCurrentProjectionV2) ValidateCurrent(now time.Time) error {
	if err := v.ValidateShape(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < v.CheckedUnixNano || now.UnixNano() >= v.ExpiresUnixNano {
		return errors.New("checkpoint phase Settlement current projection is stale")
	}
	return nil
}
