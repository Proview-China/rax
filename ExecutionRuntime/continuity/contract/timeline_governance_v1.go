package contract

const TimelineGovernanceContractVersionV1 = "praxis.continuity.timeline-governance/v1"

type TimelineEvidenceRecordRefV1 struct {
	LedgerScopeDigest string `json:"ledger_scope_digest"`
	Sequence          uint64 `json:"sequence"`
	RecordDigest      string `json:"record_digest"`
}

func (r TimelineEvidenceRecordRefV1) Validate() error {
	if err := ValidateDigest("ledger_scope_digest", r.LedgerScopeDigest); err != nil {
		return err
	}
	if err := ValidateDigest("record_digest", r.RecordDigest); err != nil {
		return err
	}
	if r.Sequence == 0 {
		return NewError(ErrInvalidArgument, "sequence", "must be non-zero")
	}
	return nil
}

type TimelineOwnerFactRefV1 struct {
	Owner           OwnerBinding `json:"owner"`
	FactKind        string       `json:"fact_kind"`
	FactID          string       `json:"fact_id"`
	Revision        uint64       `json:"revision"`
	FactDigest      string       `json:"fact_digest"`
	PayloadSchema   string       `json:"payload_schema"`
	PayloadDigest   string       `json:"payload_digest"`
	PayloadRevision uint64       `json:"payload_revision"`
	ScopeDigest     string       `json:"scope_digest"`
}

func (r TimelineOwnerFactRefV1) Validate() error {
	if err := r.Owner.Validate(); err != nil {
		return err
	}
	for field, value := range map[string]string{
		"fact_kind": r.FactKind, "fact_id": r.FactID, "fact_digest": r.FactDigest,
		"payload_schema": r.PayloadSchema, "payload_digest": r.PayloadDigest,
		"scope_digest": r.ScopeDigest,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if r.Revision == 0 || r.PayloadRevision == 0 {
		return NewError(ErrInvalidArgument, "owner_fact_ref", "revision and payload revision must be non-zero")
	}
	return nil
}

// TimelineProjectionRequestV1 is deliberately coordinate-only. Semantic,
// payload, sequence, digest, trust and currentness fields are all derived from
// Owner Readers after create-once admission.
type TimelineProjectionRequestV1 struct {
	ContractVersion   string                       `json:"contract_version"`
	AttemptID         string                       `json:"attempt_id"`
	IdempotencyKey    string                       `json:"idempotency_key"`
	EvidenceSource    EvidenceSourceKey            `json:"evidence_source"`
	ExpectedRecord    *TimelineEvidenceRecordRefV1 `json:"expected_record,omitempty"`
	OwnerFact         *TimelineOwnerFactRefV1      `json:"owner_fact,omitempty"`
	ProjectionPolicy  string                       `json:"projection_policy_ref"`
	ScopeDigest       string                       `json:"scope_digest"`
	RequestedNotAfter int64                        `json:"requested_not_after"`
	Digest            string                       `json:"digest"`
}

func (r TimelineProjectionRequestV1) Clone() TimelineProjectionRequestV1 {
	copy := r
	if r.ExpectedRecord != nil {
		value := *r.ExpectedRecord
		copy.ExpectedRecord = &value
	}
	if r.OwnerFact != nil {
		value := *r.OwnerFact
		copy.OwnerFact = &value
	}
	return copy
}

func (r TimelineProjectionRequestV1) CanonicalDigest() (string, error) {
	copy := r.Clone()
	copy.Digest = ""
	return CanonicalDigest(copy)
}

func (r TimelineProjectionRequestV1) Validate() error {
	if r.ContractVersion != TimelineGovernanceContractVersionV1 {
		return NewError(ErrInvalidArgument, "contract_version", "unsupported version")
	}
	for field, value := range map[string]string{
		"attempt_id": r.AttemptID, "idempotency_key": r.IdempotencyKey,
		"projection_policy_ref": r.ProjectionPolicy, "scope_digest": r.ScopeDigest,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := r.EvidenceSource.Validate(); err != nil {
		return err
	}
	if r.ExpectedRecord != nil {
		if err := r.ExpectedRecord.Validate(); err != nil {
			return err
		}
	}
	if r.OwnerFact != nil {
		if err := r.OwnerFact.Validate(); err != nil {
			return err
		}
		if r.OwnerFact.ScopeDigest != r.ScopeDigest {
			return NewError(ErrProjectionConflict, "owner_fact", "owner fact belongs to another scope")
		}
	}
	if r.RequestedNotAfter < 0 {
		return NewError(ErrInvalidArgument, "requested_not_after", "negative caller bound is forbidden")
	}
	expected, err := r.CanonicalDigest()
	if err != nil {
		return err
	}
	if r.Digest == "" || r.Digest != expected {
		return NewError(ErrProjectionConflict, "request_digest", "canonical digest mismatch")
	}
	return nil
}

type TimelineProjectionAttemptStateV1 string

const (
	TimelineAttemptProposedV1          TimelineProjectionAttemptStateV1 = "proposed"
	TimelineAttemptInspectingV1        TimelineProjectionAttemptStateV1 = "inspecting"
	TimelineAttemptAdmittedV1          TimelineProjectionAttemptStateV1 = "admitted"
	TimelineAttemptReconcileRequiredV1 TimelineProjectionAttemptStateV1 = "reconcile_required"
	TimelineAttemptVisibleV1           TimelineProjectionAttemptStateV1 = "visible"
	TimelineAttemptRejectedV1          TimelineProjectionAttemptStateV1 = "rejected"
	TimelineAttemptExpiredV1           TimelineProjectionAttemptStateV1 = "expired"
	TimelineAttemptIndeterminateV1     TimelineProjectionAttemptStateV1 = "indeterminate"
)

type TimelineProjectionAttemptRefV1 struct {
	AttemptID   string `json:"attempt_id"`
	Revision    uint64 `json:"revision"`
	Digest      string `json:"digest"`
	ScopeDigest string `json:"scope_digest"`
}

func (r TimelineProjectionAttemptRefV1) Validate() error {
	if err := ValidateToken("attempt_id", r.AttemptID); err != nil {
		return err
	}
	if err := ValidateDigest("attempt_digest", r.Digest); err != nil {
		return err
	}
	if err := ValidateToken("scope_digest", r.ScopeDigest); err != nil {
		return err
	}
	if r.Revision == 0 {
		return NewError(ErrInvalidArgument, "attempt_revision", "must be non-zero")
	}
	return nil
}

type TimelineEventRefV1 struct {
	EventID           string `json:"event_id"`
	EvidenceRecordRef string `json:"evidence_record_ref"`
	LedgerScopeDigest string `json:"ledger_scope_digest"`
	LedgerSequence    uint64 `json:"ledger_sequence"`
	Digest            string `json:"digest"`
}

func (r TimelineEventRefV1) Validate() error {
	for field, value := range map[string]string{
		"event_id": r.EventID, "evidence_record_ref": r.EvidenceRecordRef,
		"ledger_scope_digest": r.LedgerScopeDigest, "event_digest": r.Digest,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if r.LedgerSequence == 0 {
		return NewError(ErrInvalidArgument, "ledger_sequence", "must be non-zero")
	}
	return nil
}

type TimelineProjectionAttemptFactV1 struct {
	ContractVersion            string                           `json:"contract_version"`
	Ref                        TimelineProjectionAttemptRefV1   `json:"ref"`
	Request                    TimelineProjectionRequestV1      `json:"request"`
	State                      TimelineProjectionAttemptStateV1 `json:"state"`
	EvidenceProjectionRef      string                           `json:"evidence_projection_ref,omitempty"`
	EvidenceProjectionDigest   string                           `json:"evidence_projection_digest,omitempty"`
	EvidenceCurrentIndexRef    string                           `json:"evidence_current_index_ref,omitempty"`
	EvidenceCurrentIndexDigest string                           `json:"evidence_current_index_digest,omitempty"`
	OwnerProjectionDigest      string                           `json:"owner_projection_digest,omitempty"`
	PolicyProjectionDigest     string                           `json:"policy_projection_digest,omitempty"`
	CheckedUnixNano            int64                            `json:"checked_unix_nano,omitempty"`
	NotAfterUnixNano           int64                            `json:"not_after_unix_nano,omitempty"`
	Event                      *TimelineEventRefV1              `json:"event,omitempty"`
}

func (f TimelineProjectionAttemptFactV1) Clone() TimelineProjectionAttemptFactV1 {
	copy := f
	copy.Request = f.Request.Clone()
	if f.Event != nil {
		value := *f.Event
		copy.Event = &value
	}
	return copy
}

func (f TimelineProjectionAttemptFactV1) CanonicalDigest() (string, error) {
	copy := f.Clone()
	copy.Ref.Digest = ""
	return CanonicalDigest(copy)
}

func (f TimelineProjectionAttemptFactV1) Validate() error {
	if f.ContractVersion != TimelineGovernanceContractVersionV1 {
		return NewError(ErrInvalidArgument, "contract_version", "unsupported version")
	}
	if err := f.Ref.Validate(); err != nil {
		return err
	}
	if err := f.Request.Validate(); err != nil {
		return err
	}
	if f.Ref.AttemptID != f.Request.AttemptID || f.Ref.ScopeDigest != f.Request.ScopeDigest {
		return NewError(ErrProjectionConflict, "attempt_ref", "request and attempt identity drifted")
	}
	switch f.State {
	case TimelineAttemptProposedV1, TimelineAttemptInspectingV1:
		if f.Event != nil || f.CheckedUnixNano != 0 || f.NotAfterUnixNano != 0 {
			return NewError(ErrProjectionConflict, "attempt_state", "pre-admission attempt carries result fields")
		}
	case TimelineAttemptAdmittedV1:
		if err := f.validateAdmissionFields(); err != nil {
			return err
		}
		if f.Event != nil {
			return NewError(ErrProjectionConflict, "event", "non-visible attempt cannot carry event")
		}
	case TimelineAttemptReconcileRequiredV1:
		if f.CheckedUnixNano != 0 || f.NotAfterUnixNano != 0 || f.EvidenceProjectionRef != "" || f.EvidenceProjectionDigest != "" || f.EvidenceCurrentIndexRef != "" || f.EvidenceCurrentIndexDigest != "" || f.OwnerProjectionDigest != "" || f.PolicyProjectionDigest != "" {
			if err := f.validateAdmissionFields(); err != nil {
				return err
			}
		}
		if f.Event != nil {
			return NewError(ErrProjectionConflict, "event", "reconcile attempt cannot guess an event")
		}
	case TimelineAttemptVisibleV1:
		if err := f.validateAdmissionFields(); err != nil {
			return err
		}
		if f.Event == nil || f.Event.Validate() != nil {
			return NewError(ErrProjectionConflict, "event", "visible attempt requires exact event ref")
		}
	case TimelineAttemptRejectedV1, TimelineAttemptExpiredV1, TimelineAttemptIndeterminateV1:
		if f.Event != nil {
			return NewError(ErrProjectionConflict, "event", "terminal failed attempt cannot carry event")
		}
	default:
		return NewError(ErrInvalidArgument, "attempt_state", "unknown state")
	}
	expected, err := f.CanonicalDigest()
	if err != nil {
		return err
	}
	if f.Ref.Digest != expected {
		return NewError(ErrProjectionConflict, "attempt_digest", "canonical digest mismatch")
	}
	return nil
}

func (f TimelineProjectionAttemptFactV1) validateAdmissionFields() error {
	for field, value := range map[string]string{
		"evidence_projection_ref":       f.EvidenceProjectionRef,
		"evidence_projection_digest":    f.EvidenceProjectionDigest,
		"evidence_current_index_ref":    f.EvidenceCurrentIndexRef,
		"evidence_current_index_digest": f.EvidenceCurrentIndexDigest,
		"policy_projection_digest":      f.PolicyProjectionDigest,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if f.Request.OwnerFact != nil && f.OwnerProjectionDigest == "" {
		return NewError(ErrProjectionConflict, "owner_projection_digest", "owner projection is required")
	}
	if f.Request.OwnerFact == nil && f.OwnerProjectionDigest != "" {
		return NewError(ErrProjectionConflict, "owner_projection_digest", "non-authoritative attempt cannot carry owner projection")
	}
	if f.CheckedUnixNano <= 0 || f.NotAfterUnixNano <= f.CheckedUnixNano {
		return NewError(ErrProjectionConflict, "attempt_ttl", "checked and not-after times are inconsistent")
	}
	return nil
}

type TimelineProjectionCurrentV1 struct {
	ContractVersion            string                         `json:"contract_version"`
	Event                      TimelineEventRefV1             `json:"event"`
	Attempt                    TimelineProjectionAttemptRefV1 `json:"attempt"`
	EvidenceProjectionRef      string                         `json:"evidence_projection_ref"`
	EvidenceProjectionDigest   string                         `json:"evidence_projection_digest"`
	EvidenceCurrentIndexRef    string                         `json:"evidence_current_index_ref"`
	EvidenceCurrentIndexDigest string                         `json:"evidence_current_index_digest"`
	OwnerProjectionDigest      string                         `json:"owner_projection_digest,omitempty"`
	PolicyProjectionDigest     string                         `json:"policy_projection_digest"`
	CheckedUnixNano            int64                          `json:"checked_unix_nano"`
	NotAfterUnixNano           int64                          `json:"not_after_unix_nano"`
	Digest                     string                         `json:"digest"`
}

func (p TimelineProjectionCurrentV1) CanonicalDigest() (string, error) {
	copy := p
	copy.Digest = ""
	return CanonicalDigest(copy)
}

func (p TimelineProjectionCurrentV1) Validate() error {
	if p.ContractVersion != TimelineGovernanceContractVersionV1 || p.Event.Validate() != nil || p.Attempt.Validate() != nil {
		return NewError(ErrInvalidArgument, "projection_current", "version, event and attempt refs are required")
	}
	for field, value := range map[string]string{
		"evidence_projection_ref":       p.EvidenceProjectionRef,
		"evidence_projection_digest":    p.EvidenceProjectionDigest,
		"evidence_current_index_ref":    p.EvidenceCurrentIndexRef,
		"evidence_current_index_digest": p.EvidenceCurrentIndexDigest,
		"policy_projection_digest":      p.PolicyProjectionDigest,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if p.CheckedUnixNano <= 0 || p.NotAfterUnixNano <= p.CheckedUnixNano {
		return NewError(ErrInvalidArgument, "projection_ttl", "checked and not-after times are inconsistent")
	}
	expected, err := p.CanonicalDigest()
	if err != nil {
		return err
	}
	if p.Digest == "" || p.Digest != expected {
		return NewError(ErrProjectionConflict, "projection_digest", "canonical digest mismatch")
	}
	return nil
}

func SealTimelineProjectionRequestV1(r TimelineProjectionRequestV1) (TimelineProjectionRequestV1, error) {
	r.Digest = ""
	digest, err := r.CanonicalDigest()
	if err != nil {
		return TimelineProjectionRequestV1{}, err
	}
	r.Digest = digest
	return r, r.Validate()
}

func SealTimelineProjectionAttemptV1(f TimelineProjectionAttemptFactV1) (TimelineProjectionAttemptFactV1, error) {
	f.Ref.Digest = ""
	digest, err := f.CanonicalDigest()
	if err != nil {
		return TimelineProjectionAttemptFactV1{}, err
	}
	f.Ref.Digest = digest
	return f, f.Validate()
}

func SealTimelineProjectionCurrentV1(p TimelineProjectionCurrentV1) (TimelineProjectionCurrentV1, error) {
	p.Digest = ""
	digest, err := p.CanonicalDigest()
	if err != nil {
		return TimelineProjectionCurrentV1{}, err
	}
	p.Digest = digest
	return p, p.Validate()
}

func AdvanceTimelineProjectionAttemptV1(current, next TimelineProjectionAttemptStateV1) error {
	allowed := map[TimelineProjectionAttemptStateV1]map[TimelineProjectionAttemptStateV1]bool{
		TimelineAttemptProposedV1:          {TimelineAttemptInspectingV1: true, TimelineAttemptRejectedV1: true, TimelineAttemptExpiredV1: true, TimelineAttemptIndeterminateV1: true, TimelineAttemptReconcileRequiredV1: true},
		TimelineAttemptInspectingV1:        {TimelineAttemptAdmittedV1: true, TimelineAttemptRejectedV1: true, TimelineAttemptExpiredV1: true, TimelineAttemptIndeterminateV1: true, TimelineAttemptReconcileRequiredV1: true},
		TimelineAttemptAdmittedV1:          {TimelineAttemptVisibleV1: true, TimelineAttemptReconcileRequiredV1: true, TimelineAttemptExpiredV1: true, TimelineAttemptIndeterminateV1: true},
		TimelineAttemptReconcileRequiredV1: {TimelineAttemptAdmittedV1: true, TimelineAttemptVisibleV1: true, TimelineAttemptIndeterminateV1: true},
	}
	if allowed[current][next] {
		return nil
	}
	return NewError(ErrRevisionConflict, "attempt_state", "illegal or terminal transition")
}
