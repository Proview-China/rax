package union

import "strings"

const SemanticVersionV1 = "praxis.execution-union/v1"

type (
	ExecutionID        string
	SessionID          string
	TurnID             string
	IntentID           string
	MechanismPlanID    string
	MechanismAttemptID string
	EffectID           string
	VerificationID     string
	ItemID             string
	ActionID           string
	ArtifactID         string
	EventID            string
	ApprovalID         string
)

type VersionedIdentity struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

func (identity VersionedIdentity) Validate(field string) error {
	if strings.TrimSpace(identity.ID) == "" {
		return validationError(field+".id", "must not be empty")
	}
	if strings.TrimSpace(identity.Version) == "" {
		return validationError(field+".version", "must not be empty")
	}
	return nil
}

type NativeIdentity struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Value     string `json:"value"`
}

func (identity NativeIdentity) Validate() error {
	if strings.TrimSpace(identity.Namespace) == "" {
		return validationError("native_identity.namespace", "must not be empty")
	}
	if strings.TrimSpace(identity.Kind) == "" {
		return validationError("native_identity.kind", "must not be empty")
	}
	if strings.TrimSpace(identity.Value) == "" {
		return validationError("native_identity.value", "must not be empty")
	}
	return nil
}
