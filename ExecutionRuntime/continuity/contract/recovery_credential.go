package contract

import (
	"sort"
	"time"
)

// RecoveryActionV1 is intentionally closed to the historical restore-plan
// shape. It does not grant dispatch, activation, or provider authority.
type RecoveryActionV1 string

const (
	RecoveryActionInspectV1 RecoveryActionV1 = "inspect"
	RecoveryActionStageV1   RecoveryActionV1 = "stage"
)

// RecoveryCredentialV1 is a secret-free, short-lived reference bundle owned
// by Continuity. A valid bundle may constrain a RestorePlan shape, but it is
// not a Runtime eligibility fact, permit, fence, or execution credential.
type RecoveryCredentialV1 struct {
	CredentialID         string             `json:"credential_id"`
	Revision             uint64             `json:"revision"`
	CheckpointDigest     string             `json:"checkpoint_digest"`
	ManifestDigest       string             `json:"manifest_digest"`
	RestorePlanDigest    string             `json:"restore_plan_digest"`
	SubjectScope         Scope              `json:"subject_scope"`
	AuthorityRef         string             `json:"authority_ref"`
	PolicyRef            string             `json:"policy_ref"`
	ReviewRef            string             `json:"review_ref"`
	AllowedActions       []RecoveryActionV1 `json:"allowed_actions"`
	ParticipantSetDigest string             `json:"participant_set_digest"`
	IssuedUnixNano       int64              `json:"issued_unix_nano"`
	ExpiresUnixNano      int64              `json:"expires_unix_nano"`
	RevocationRef        string             `json:"revocation_ref,omitempty"`
	Digest               string             `json:"digest"`
}

func (c RecoveryCredentialV1) Clone() RecoveryCredentialV1 {
	copy := c
	copy.AllowedActions = append([]RecoveryActionV1{}, c.AllowedActions...)
	return copy
}

func (c RecoveryCredentialV1) CanonicalDigest() (string, error) {
	copy := c.Clone()
	copy.Digest = ""
	actions, err := normalizeRecoveryActionsV1(copy.AllowedActions)
	if err != nil {
		return "", err
	}
	copy.AllowedActions = actions
	return CanonicalDigest(copy)
}

// Validate only validates use as an input to a historical RestorePlan shape.
// It never establishes Runtime current eligibility or authorizes execution.
func (c RecoveryCredentialV1) Validate(now time.Time) error {
	for field, value := range map[string]string{
		"credential_id": c.CredentialID,
		"authority_ref": c.AuthorityRef,
		"policy_ref":    c.PolicyRef,
		"review_ref":    c.ReviewRef,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	for field, value := range map[string]string{
		"checkpoint_digest":      c.CheckpointDigest,
		"manifest_digest":        c.ManifestDigest,
		"restore_plan_digest":    c.RestorePlanDigest,
		"participant_set_digest": c.ParticipantSetDigest,
	} {
		if err := ValidateDigest(field, value); err != nil {
			return err
		}
	}
	if c.Revision == 0 {
		return NewError(ErrInvalidArgument, "revision", "must be non-zero")
	}
	if err := c.SubjectScope.Validate(); err != nil {
		return err
	}
	if len(c.AllowedActions) == 0 {
		return NewError(ErrInvalidArgument, "allowed_actions", "at least one inspect or stage action is required")
	}
	if _, err := normalizeRecoveryActionsV1(c.AllowedActions); err != nil {
		return err
	}
	if c.IssuedUnixNano <= 0 || c.ExpiresUnixNano <= c.IssuedUnixNano {
		return NewError(ErrInvalidArgument, "credential_ttl", "issued and expiry times are inconsistent")
	}
	if c.RevocationRef != "" {
		if err := ValidateToken("revocation_ref", c.RevocationRef); err != nil {
			return err
		}
	}
	expected, err := c.CanonicalDigest()
	if err != nil {
		return err
	}
	if c.Digest == "" || c.Digest != expected {
		return NewError(ErrInvalidArgument, "credential_digest", "canonical digest mismatch")
	}
	if now.IsZero() {
		return NewError(ErrInvalidArgument, "now", "injected current time is required")
	}
	if now.UnixNano() < c.IssuedUnixNano || now.UnixNano() >= c.ExpiresUnixNano {
		return NewError(ErrRestoreIncompatible, "credential_ttl", "credential is not valid at the requested time")
	}
	if c.RevocationRef != "" {
		return NewError(ErrRestoreIncompatible, "revocation_ref", "credential is revoked")
	}
	return nil
}

// ValidateForPlan binds the credential to one exact, already-sealed plan
// shape. Success stops at shape validation and does not enable restore.
func (c RecoveryCredentialV1) ValidateForPlan(plan RestorePlan, now time.Time) error {
	if err := plan.Validate(now); err != nil {
		return err
	}
	if err := c.Validate(now); err != nil {
		return err
	}
	if plan.RecoveryCredentialRef != c.CredentialID || plan.Digest != c.RestorePlanDigest {
		return NewError(ErrRestoreIncompatible, "recovery_credential", "credential does not bind the exact restore plan shape")
	}
	return nil
}

func normalizeRecoveryActionsV1(values []RecoveryActionV1) ([]RecoveryActionV1, error) {
	if len(values) > MaxReferenceCount {
		return nil, NewError(ErrInvalidArgument, "allowed_actions", "too many actions")
	}
	result := append([]RecoveryActionV1{}, values...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	for i, value := range result {
		switch value {
		case RecoveryActionInspectV1, RecoveryActionStageV1:
		default:
			return nil, NewError(ErrInvalidArgument, "allowed_actions", "action is not inspect or stage")
		}
		if i > 0 && result[i-1] == value {
			return nil, NewError(ErrInvalidArgument, "allowed_actions", "duplicate action")
		}
	}
	return result, nil
}
