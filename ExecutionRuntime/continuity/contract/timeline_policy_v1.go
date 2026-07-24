package contract

import "time"

const TimelineProjectionPolicyContractVersionV1 = "praxis.continuity.timeline-projection-policy/v1"

type TimelineProjectionPolicyStateV1 string

const (
	TimelineProjectionPolicyActiveV1  TimelineProjectionPolicyStateV1 = "active"
	TimelineProjectionPolicyRevokedV1 TimelineProjectionPolicyStateV1 = "revoked"
	TimelineProjectionPolicyExpiredV1 TimelineProjectionPolicyStateV1 = "expired"
)

type TimelineProjectionPolicyRefV1 struct {
	PolicyID    string `json:"policy_id"`
	Revision    uint64 `json:"revision"`
	ScopeDigest string `json:"scope_digest"`
	Digest      string `json:"digest"`
}

func (r TimelineProjectionPolicyRefV1) Validate() error {
	for field, value := range map[string]string{"policy_id": r.PolicyID, "scope_digest": r.ScopeDigest, "policy_digest": r.Digest} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if r.Revision == 0 {
		return NewError(ErrInvalidArgument, "policy_revision", "must be non-zero")
	}
	return nil
}

// TimelineProjectionPolicyCurrentV1 proves only that an opaque policy identity
// is current for a scope. It deliberately does not invent policy decisions.
type TimelineProjectionPolicyCurrentV1 struct {
	ContractVersion string                          `json:"contract_version"`
	Ref             TimelineProjectionPolicyRefV1   `json:"ref"`
	State           TimelineProjectionPolicyStateV1 `json:"state"`
	CheckedUnixNano int64                           `json:"checked_unix_nano"`
	ExpiresUnixNano int64                           `json:"expires_unix_nano"`
}

func (p TimelineProjectionPolicyCurrentV1) CanonicalDigest() (string, error) {
	copy := p
	copy.Ref.Digest = ""
	return CanonicalDigest(copy)
}

func (p TimelineProjectionPolicyCurrentV1) Validate() error {
	if p.ContractVersion != TimelineProjectionPolicyContractVersionV1 {
		return NewError(ErrInvalidArgument, "contract_version", "unsupported timeline policy version")
	}
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	switch p.State {
	case TimelineProjectionPolicyActiveV1, TimelineProjectionPolicyRevokedV1, TimelineProjectionPolicyExpiredV1:
	default:
		return NewError(ErrInvalidArgument, "policy_state", "unknown timeline policy state")
	}
	if p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return NewError(ErrInvalidArgument, "policy_ttl", "natural checked and expiry are inconsistent")
	}
	expected, err := p.CanonicalDigest()
	if err != nil {
		return err
	}
	if p.Ref.Digest != expected {
		return NewError(ErrProjectionConflict, "policy_digest", "policy projection canonical digest drifted")
	}
	return nil
}

func (p TimelineProjectionPolicyCurrentV1) ValidateCurrent(expected TimelineProjectionPolicyRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if p.Ref != expected {
		return NewError(ErrRevisionConflict, "policy_ref", "expected policy revision is stale")
	}
	if p.State != TimelineProjectionPolicyActiveV1 {
		return NewError(ErrPreconditionFailed, "policy_state", "policy is not active")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano || now.UnixNano() >= p.ExpiresUnixNano {
		return NewError(ErrPreconditionFailed, "policy_ttl", "policy is outside its natural current window")
	}
	return nil
}

func SealTimelineProjectionPolicyCurrentV1(p TimelineProjectionPolicyCurrentV1) (TimelineProjectionPolicyCurrentV1, error) {
	if p.ContractVersion != "" && p.ContractVersion != TimelineProjectionPolicyContractVersionV1 {
		return TimelineProjectionPolicyCurrentV1{}, NewError(ErrInvalidArgument, "contract_version", "unsupported timeline policy version")
	}
	p.ContractVersion = TimelineProjectionPolicyContractVersionV1
	provided := p.Ref.Digest
	p.Ref.Digest = ""
	digest, err := p.CanonicalDigest()
	if err != nil {
		return TimelineProjectionPolicyCurrentV1{}, err
	}
	if provided != "" && provided != digest {
		return TimelineProjectionPolicyCurrentV1{}, NewError(ErrProjectionConflict, "policy_digest", "supplied policy digest drifted")
	}
	p.Ref.Digest = digest
	return p, p.Validate()
}

func ValidateTimelineProjectionPolicySuccessorV1(current, next TimelineProjectionPolicyCurrentV1) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if next.Ref.PolicyID != current.Ref.PolicyID || next.Ref.ScopeDigest != current.Ref.ScopeDigest || next.Ref.Revision != current.Ref.Revision+1 {
		return NewError(ErrRevisionConflict, "policy_ref", "policy identity, scope or revision drifted")
	}
	if current.State != TimelineProjectionPolicyActiveV1 {
		return NewError(ErrRevisionConflict, "policy_state", "terminal policy cannot advance")
	}
	if next.CheckedUnixNano < current.CheckedUnixNano {
		return NewError(ErrPreconditionFailed, "policy_clock", "policy checked time regressed")
	}
	return nil
}
