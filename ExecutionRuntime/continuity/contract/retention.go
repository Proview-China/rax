package contract

type RetentionState string

const (
	RetentionActive          RetentionState = "active"
	RetentionExpired         RetentionState = "retention_expired"
	RetentionTombstoned      RetentionState = "tombstoned"
	RetentionLegalHold       RetentionState = "legal_hold"
	RetentionPrivacyRequired RetentionState = "privacy_erasure_required"
)

type RetentionFact struct {
	ObjectID              string         `json:"object_id"`
	PolicyRef             string         `json:"policy_ref"`
	Classification        string         `json:"classification"`
	State                 RetentionState `json:"state"`
	PreviousState         RetentionState `json:"previous_state,omitempty"`
	TombstoneRef          string         `json:"tombstone_ref,omitempty"`
	HoldRef               string         `json:"hold_ref,omitempty"`
	TransitionEvidenceRef string         `json:"transition_evidence_ref,omitempty"`
	Revision              uint64         `json:"revision"`
	UpdatedUnixNano       int64          `json:"updated_unix_nano"`
}

func (f RetentionFact) Validate() error {
	for field, value := range map[string]string{
		"object_id": f.ObjectID, "policy_ref": f.PolicyRef, "classification": f.Classification,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if f.Revision == 0 || f.UpdatedUnixNano <= 0 {
		return NewError(ErrInvalidArgument, "retention", "revision and update time are required")
	}
	if f.Revision > 1 {
		if err := ValidateToken("transition_evidence_ref", f.TransitionEvidenceRef); err != nil {
			return err
		}
	}
	switch f.State {
	case RetentionActive, RetentionExpired, RetentionPrivacyRequired:
	case RetentionTombstoned:
		if err := ValidateToken("tombstone_ref", f.TombstoneRef); err != nil {
			return err
		}
	case RetentionLegalHold:
		if err := ValidateToken("hold_ref", f.HoldRef); err != nil {
			return err
		}
		if f.PreviousState == "" || f.PreviousState == RetentionLegalHold {
			return NewError(ErrInvalidArgument, "previous_state", "legal hold must preserve the prior state")
		}
	default:
		return NewError(ErrInvalidArgument, "retention_state", "unknown state")
	}
	return nil
}

func AdvanceRetention(current RetentionFact, next RetentionState, evidenceRef string) (RetentionFact, error) {
	if err := current.Validate(); err != nil {
		return RetentionFact{}, err
	}
	if current.State == RetentionLegalHold {
		if current.PreviousState == "" || next != current.PreviousState {
			return RetentionFact{}, NewError(ErrRetentionBlocked, "legal_hold", "hold must be explicitly released to its previous state")
		}
		if err := ValidateToken("transition_evidence_ref", evidenceRef); err != nil {
			return RetentionFact{}, err
		}
		nextFact := current
		nextFact.State = next
		nextFact.PreviousState = ""
		nextFact.HoldRef = ""
		nextFact.TransitionEvidenceRef = evidenceRef
		nextFact.Revision++
		return nextFact, nil
	}
	allowed := false
	switch current.State {
	case RetentionActive:
		allowed = next == RetentionExpired || next == RetentionTombstoned || next == RetentionLegalHold || next == RetentionPrivacyRequired
	case RetentionExpired:
		allowed = next == RetentionTombstoned || next == RetentionLegalHold || next == RetentionPrivacyRequired
	case RetentionTombstoned:
		allowed = next == RetentionLegalHold || next == RetentionPrivacyRequired
	case RetentionPrivacyRequired:
		allowed = next == RetentionTombstoned || next == RetentionLegalHold
	}
	if !allowed {
		return RetentionFact{}, NewError(ErrInvalidArgument, "retention_transition", "illegal state transition")
	}
	if err := ValidateToken("transition_evidence_ref", evidenceRef); err != nil {
		return RetentionFact{}, err
	}
	nextFact := current
	nextFact.PreviousState = current.State
	nextFact.State = next
	nextFact.Revision++
	nextFact.TransitionEvidenceRef = evidenceRef
	if next == RetentionTombstoned {
		nextFact.TombstoneRef = evidenceRef
	}
	if next == RetentionLegalHold {
		nextFact.HoldRef = evidenceRef
	}
	return nextFact, nil
}
