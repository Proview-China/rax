package contract

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

type TrustClass string

const (
	TrustObservation       TrustClass = "observation"
	TrustLateObservation   TrustClass = "late_observation"
	TrustReceipt           TrustClass = "receipt"
	TrustAttestation       TrustClass = "attestation"
	TrustClaim             TrustClass = "claim"
	TrustAuthoritativeFact TrustClass = "authoritative_fact"
)

type EvidenceSourceKey struct {
	RegistrationID string `json:"registration_id"`
	SourceEpoch    uint64 `json:"source_epoch"`
	SourceSequence uint64 `json:"source_sequence"`
}

func (k EvidenceSourceKey) Validate() error {
	if err := ValidateToken("registration_id", k.RegistrationID); err != nil {
		return err
	}
	if k.SourceEpoch == 0 || k.SourceSequence == 0 {
		return NewError(ErrInvalidArgument, "evidence_source_key", "epoch and sequence must be non-zero")
	}
	return nil
}

func (k EvidenceSourceKey) String() string {
	return fmt.Sprintf("%s/%d/%d", k.RegistrationID, k.SourceEpoch, k.SourceSequence)
}

type EvidenceAdmission struct {
	RecordRef         string            `json:"record_ref"`
	LedgerScopeDigest string            `json:"ledger_scope_digest"`
	LedgerSequence    uint64            `json:"ledger_sequence"`
	RecordDigest      string            `json:"record_digest"`
	SourceKey         EvidenceSourceKey `json:"source_key"`
	TrustClass        TrustClass        `json:"trust_class"`
	ObservedUnixNano  int64             `json:"observed_unix_nano"`
	RecordedUnixNano  int64             `json:"recorded_unix_nano"`
	PayloadRef        string            `json:"payload_ref"`
	PayloadSchema     string            `json:"payload_schema"`
	PayloadDigest     string            `json:"payload_digest"`
	PayloadRevision   uint64            `json:"payload_revision"`
	AdmittedByLedger  bool              `json:"admitted_by_ledger"`
	InspectedByOwner  bool              `json:"inspected_by_owner"`
}

func (a EvidenceAdmission) Validate() error {
	if !a.AdmittedByLedger || !a.InspectedByOwner {
		return NewError(ErrEvidenceNotInspectable, "evidence", "record must be admitted and independently inspectable")
	}
	for field, value := range map[string]string{
		"record_ref": a.RecordRef, "ledger_scope_digest": a.LedgerScopeDigest,
		"record_digest": a.RecordDigest, "payload_ref": a.PayloadRef,
		"payload_schema": a.PayloadSchema, "payload_digest": a.PayloadDigest,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if a.LedgerSequence == 0 || a.PayloadRevision == 0 {
		return NewError(ErrInvalidArgument, "evidence", "ledger sequence and payload revision must be non-zero")
	}
	if a.ObservedUnixNano <= 0 || a.RecordedUnixNano <= 0 {
		return NewError(ErrInvalidArgument, "evidence_time", "timestamps must be positive")
	}
	if err := a.SourceKey.Validate(); err != nil {
		return err
	}
	switch a.TrustClass {
	case TrustObservation, TrustLateObservation, TrustReceipt, TrustAttestation, TrustClaim, TrustAuthoritativeFact:
		return nil
	default:
		return NewError(ErrInvalidArgument, "trust_class", "unknown trust class")
	}
}

type TimelineProjectionCandidate struct {
	ContractVersion     string                  `json:"contract_version"`
	CandidateID         string                  `json:"candidate_id"`
	Revision            uint64                  `json:"revision"`
	Digest              string                  `json:"digest"`
	Scope               Scope                   `json:"scope"`
	Owner               OwnerBinding            `json:"owner"`
	Evidence            EvidenceAdmission       `json:"evidence"`
	OwnerFactRef        *FactRef                `json:"owner_fact_ref,omitempty"`
	OwnerFactExactRef   *TimelineOwnerFactRefV1 `json:"owner_fact_exact_ref,omitempty"`
	SemanticKind        string                  `json:"semantic_kind"`
	CustomClass         string                  `json:"custom_class,omitempty"`
	ParentRefs          []string                `json:"parent_refs"`
	CausationRefs       []string                `json:"causation_refs"`
	CorrelationID       string                  `json:"correlation_id,omitempty"`
	ObjectRefs          []string                `json:"object_refs"`
	ProjectionPolicyRef string                  `json:"projection_policy_ref"`
}

func (c TimelineProjectionCandidate) CanonicalDigest() (string, error) {
	parents, err := NormalizeOrdered(c.ParentRefs)
	if err != nil {
		return "", err
	}
	causation, err := NormalizeOrdered(c.CausationRefs)
	if err != nil {
		return "", err
	}
	objects, err := NormalizeSet(c.ObjectRefs)
	if err != nil {
		return "", err
	}
	copy := c
	copy.Digest = ""
	copy.ParentRefs = parents
	copy.CausationRefs = causation
	copy.ObjectRefs = objects
	return CanonicalDigest(copy)
}

func (c TimelineProjectionCandidate) Validate() error {
	if c.ContractVersion != ContractVersion {
		return NewError(ErrInvalidArgument, "contract_version", "unsupported version")
	}
	if err := ValidateToken("candidate_id", c.CandidateID); err != nil {
		return err
	}
	if c.Revision == 0 {
		return NewError(ErrInvalidArgument, "revision", "must be non-zero")
	}
	if err := c.Scope.Validate(); err != nil {
		return err
	}
	if err := c.Owner.Validate(); err != nil {
		return err
	}
	if err := c.Evidence.Validate(); err != nil {
		return err
	}
	if err := ValidateToken("semantic_kind", c.SemanticKind); err != nil {
		return err
	}
	if err := ValidateToken("projection_policy_ref", c.ProjectionPolicyRef); err != nil {
		return err
	}
	if c.CorrelationID != "" {
		if err := ValidateToken("correlation_id", c.CorrelationID); err != nil {
			return err
		}
	}
	for _, refs := range [][]string{c.ParentRefs, c.CausationRefs} {
		normalized, err := NormalizeOrdered(refs)
		if err != nil || len(normalized) != len(refs) {
			if err != nil {
				return err
			}
		}
	}
	if _, err := NormalizeSet(c.ObjectRefs); err != nil {
		return err
	}
	if Contains(c.ParentRefs, c.CandidateID) {
		return NewError(ErrInvalidArgument, "parent_refs", "self reference is forbidden")
	}
	if c.Evidence.TrustClass == TrustAuthoritativeFact {
		if (c.OwnerFactRef == nil) == (c.OwnerFactExactRef == nil) {
			return NewError(ErrEvidenceNotInspectable, "owner_fact_ref", "authoritative projection requires an owner fact")
		}
		if c.OwnerFactRef != nil {
			if err := c.OwnerFactRef.Validate(); err != nil {
				return err
			}
		} else if err := c.OwnerFactExactRef.Validate(); err != nil {
			return err
		}
	} else if c.OwnerFactRef != nil || c.OwnerFactExactRef != nil {
		return NewError(ErrEvidenceNotInspectable, "owner_fact_ref", "non-authoritative projection cannot carry an owner fact")
	}
	expected, err := c.CanonicalDigest()
	if err != nil {
		return err
	}
	if c.Digest == "" || c.Digest != expected {
		return NewError(ErrProjectionConflict, "digest", "candidate digest does not match canonical content")
	}
	return nil
}

func (c TimelineProjectionCandidate) Clone() TimelineProjectionCandidate {
	copy := c
	copy.ParentRefs = append([]string{}, c.ParentRefs...)
	copy.CausationRefs = append([]string{}, c.CausationRefs...)
	copy.ObjectRefs = append([]string{}, c.ObjectRefs...)
	if c.OwnerFactRef != nil {
		ref := *c.OwnerFactRef
		copy.OwnerFactRef = &ref
	}
	if c.OwnerFactExactRef != nil {
		ref := *c.OwnerFactExactRef
		copy.OwnerFactExactRef = &ref
	}
	return copy
}

type TimelineEventRecord struct {
	Candidate            TimelineProjectionCandidate `json:"candidate"`
	EvidenceRecordRef    string                      `json:"evidence_record_ref"`
	LedgerScopeDigest    string                      `json:"ledger_scope_digest"`
	LedgerSequence       uint64                      `json:"ledger_sequence"`
	EvidenceRecordDigest string                      `json:"evidence_record_digest"`
	TrustClass           TrustClass                  `json:"trust_class"`
	ProjectionRevision   uint64                      `json:"projection_revision"`
	Visibility           string                      `json:"visibility"`
	TombstoneRef         string                      `json:"tombstone_ref,omitempty"`
}

// TimelineProjectionTombstoneRequestV1 carries only coordinates for an
// immutable Continuity-owned visibility fact. It never authorizes physical
// deletion or mutation of the historical Event.
type TimelineProjectionTombstoneRequestV1 struct {
	TombstoneID        string `json:"tombstone_id"`
	EvidenceRecordRef  string `json:"evidence_record_ref"`
	SourceTombstoneRef string `json:"source_tombstone_ref"`
	PolicyBasisRef     string `json:"policy_basis_ref"`
	IdempotencyKey     string `json:"idempotency_key"`
}

func (r TimelineProjectionTombstoneRequestV1) Validate() error {
	for field, value := range map[string]string{
		"tombstone_id":         r.TombstoneID,
		"evidence_record_ref":  r.EvidenceRecordRef,
		"source_tombstone_ref": r.SourceTombstoneRef,
		"policy_basis_ref":     r.PolicyBasisRef,
		"idempotency_key":      r.IdempotencyKey,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	return nil
}

// TimelineProjectionTombstoneFactV1 is append-only. Query combines this fact
// with Event history through a visibility overlay; the Event bytes and
// revision remain unchanged.
type TimelineProjectionTombstoneFactV1 struct {
	ContractVersion    string `json:"contract_version"`
	TombstoneID        string `json:"tombstone_id"`
	EvidenceRecordRef  string `json:"evidence_record_ref"`
	SourceTombstoneRef string `json:"source_tombstone_ref"`
	PolicyBasisRef     string `json:"policy_basis_ref"`
	IdempotencyKey     string `json:"idempotency_key"`
	ScopeDigest        string `json:"scope_digest"`
	Revision           uint64 `json:"revision"`
	CreatedUnixNano    int64  `json:"created_unix_nano"`
	Digest             string `json:"digest"`
}

func (f TimelineProjectionTombstoneFactV1) CanonicalDigest() (string, error) {
	copy := f
	copy.Digest = ""
	return CanonicalDigest(copy)
}

func (f TimelineProjectionTombstoneFactV1) Validate() error {
	if f.ContractVersion != ContractVersion {
		return NewError(ErrInvalidArgument, "contract_version", "unsupported version")
	}
	request := TimelineProjectionTombstoneRequestV1{
		TombstoneID: f.TombstoneID, EvidenceRecordRef: f.EvidenceRecordRef,
		SourceTombstoneRef: f.SourceTombstoneRef, PolicyBasisRef: f.PolicyBasisRef,
		IdempotencyKey: f.IdempotencyKey,
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if err := ValidateToken("scope_digest", f.ScopeDigest); err != nil {
		return err
	}
	if f.Revision != 1 || f.CreatedUnixNano <= 0 {
		return NewError(ErrInvalidArgument, "tombstone_fact", "revision one and creation time are required")
	}
	expected, err := f.CanonicalDigest()
	if err != nil {
		return err
	}
	if f.Digest == "" || f.Digest != expected {
		return NewError(ErrProjectionConflict, "tombstone_digest", "canonical digest mismatch")
	}
	return nil
}

func (r TimelineEventRecord) Validate() error {
	if err := r.Candidate.Validate(); err != nil {
		return err
	}
	if r.EvidenceRecordRef != r.Candidate.Evidence.RecordRef ||
		r.LedgerScopeDigest != r.Candidate.Evidence.LedgerScopeDigest ||
		r.LedgerSequence != r.Candidate.Evidence.LedgerSequence ||
		r.EvidenceRecordDigest != r.Candidate.Evidence.RecordDigest ||
		r.TrustClass != r.Candidate.Evidence.TrustClass {
		return NewError(ErrProjectionConflict, "evidence", "projection changed evidence-owned fields")
	}
	if r.ProjectionRevision == 0 {
		return NewError(ErrInvalidArgument, "projection_revision", "must be non-zero")
	}
	if r.Visibility != "visible" && r.Visibility != "tombstoned" {
		return NewError(ErrInvalidArgument, "visibility", "unknown visibility")
	}
	return nil
}

func (r TimelineEventRecord) Clone() TimelineEventRecord {
	copy := r
	copy.Candidate = r.Candidate.Clone()
	return copy
}

type TimelineQuery struct {
	LedgerScopeDigest  string   `json:"ledger_scope_digest"`
	TenantID           string   `json:"tenant_id,omitempty"`
	IdentityID         string   `json:"identity_id,omitempty"`
	LineageID          string   `json:"lineage_id,omitempty"`
	InstanceID         string   `json:"instance_id,omitempty"`
	RunID              string   `json:"run_id,omitempty"`
	SemanticKinds      []string `json:"semantic_kinds"`
	ObjectRef          string   `json:"object_ref,omitempty"`
	TurnRef            string   `json:"turn_ref,omitempty"`
	StepRef            string   `json:"step_ref,omitempty"`
	ActionRef          string   `json:"action_ref,omitempty"`
	ArtifactRef        string   `json:"artifact_ref,omitempty"`
	EffectRef          string   `json:"effect_ref,omitempty"`
	ReviewCaseRef      string   `json:"review_case_ref,omitempty"`
	CheckpointRef      string   `json:"checkpoint_ref,omitempty"`
	ParentRef          string   `json:"parent_ref,omitempty"`
	CausationRef       string   `json:"causation_ref,omitempty"`
	CorrelationID      string   `json:"correlation_id,omitempty"`
	RecordedAfter      int64    `json:"recorded_after,omitempty"`
	RecordedBefore     int64    `json:"recorded_before,omitempty"`
	IncludeTombstoned  bool     `json:"include_tombstoned"`
	AuthorityWatermark string   `json:"authority_watermark"`
	PolicyWatermark    string   `json:"policy_watermark"`
	PageLimit          int      `json:"page_limit"`
	Cursor             string   `json:"-"`
}

func (q TimelineQuery) Validate() error {
	for field, value := range map[string]string{
		"ledger_scope_digest": q.LedgerScopeDigest,
		"authority_watermark": q.AuthorityWatermark,
		"policy_watermark":    q.PolicyWatermark,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	for field, value := range map[string]string{
		"object_ref": q.ObjectRef, "turn_ref": q.TurnRef, "step_ref": q.StepRef,
		"action_ref": q.ActionRef, "artifact_ref": q.ArtifactRef,
		"effect_ref": q.EffectRef, "review_case_ref": q.ReviewCaseRef,
		"checkpoint_ref": q.CheckpointRef, "parent_ref": q.ParentRef,
		"causation_ref": q.CausationRef, "correlation_id": q.CorrelationID,
	} {
		if value != "" {
			if err := ValidateToken(field, value); err != nil {
				return err
			}
		}
	}
	if q.PageLimit <= 0 || q.PageLimit > 1000 {
		return NewError(ErrInvalidArgument, "page_limit", "must be between 1 and 1000")
	}
	if q.RecordedBefore != 0 && q.RecordedAfter > q.RecordedBefore {
		return NewError(ErrInvalidArgument, "recorded_range", "after must not exceed before")
	}
	_, err := NormalizeSet(q.SemanticKinds)
	return err
}

func (q TimelineQuery) Digest() (string, error) {
	kinds, err := NormalizeSet(q.SemanticKinds)
	if err != nil {
		return "", err
	}
	q.SemanticKinds = kinds
	q.Cursor = ""
	return CanonicalDigest(q)
}

// TimelineEventMatchesQuery is the contract-level filter predicate used both
// by stores and by untrusted-reader validation at SDK/API boundaries.
func TimelineEventMatchesQuery(record TimelineEventRecord, query TimelineQuery) bool {
	scope := record.Candidate.Scope
	if record.LedgerScopeDigest != query.LedgerScopeDigest ||
		query.TenantID != "" && query.TenantID != scope.TenantID ||
		query.IdentityID != "" && query.IdentityID != scope.IdentityID ||
		query.LineageID != "" && query.LineageID != scope.LineageID ||
		query.InstanceID != "" && query.InstanceID != scope.InstanceID ||
		query.RunID != "" && query.RunID != scope.RunID ||
		query.ObjectRef != "" && !Contains(record.Candidate.ObjectRefs, query.ObjectRef) ||
		query.TurnRef != "" && !Contains(record.Candidate.ObjectRefs, query.TurnRef) ||
		query.StepRef != "" && !Contains(record.Candidate.ObjectRefs, query.StepRef) ||
		query.ActionRef != "" && !Contains(record.Candidate.ObjectRefs, query.ActionRef) ||
		query.ArtifactRef != "" && !Contains(record.Candidate.ObjectRefs, query.ArtifactRef) ||
		query.EffectRef != "" && !Contains(record.Candidate.ObjectRefs, query.EffectRef) ||
		query.ReviewCaseRef != "" && !Contains(record.Candidate.ObjectRefs, query.ReviewCaseRef) ||
		query.CheckpointRef != "" && !Contains(record.Candidate.ObjectRefs, query.CheckpointRef) ||
		query.ParentRef != "" && !Contains(record.Candidate.ParentRefs, query.ParentRef) ||
		query.CausationRef != "" && !Contains(record.Candidate.CausationRefs, query.CausationRef) ||
		query.CorrelationID != "" && query.CorrelationID != record.Candidate.CorrelationID ||
		query.RecordedAfter != 0 && record.Candidate.Evidence.RecordedUnixNano <= query.RecordedAfter ||
		query.RecordedBefore != 0 && record.Candidate.Evidence.RecordedUnixNano > query.RecordedBefore ||
		!query.IncludeTombstoned && record.Visibility == "tombstoned" {
		return false
	}
	return len(query.SemanticKinds) == 0 || Contains(query.SemanticKinds, record.Candidate.SemanticKind)
}

type TimelineCursor struct {
	LedgerScopeDigest  string `json:"ledger_scope_digest"`
	AfterSequence      uint64 `json:"after_sequence"`
	QueryDigest        string `json:"query_digest"`
	AuthorityWatermark string `json:"authority_watermark"`
	PolicyWatermark    string `json:"policy_watermark"`
	ProjectionSchema   string `json:"projection_schema"`
	PageLimit          int    `json:"page_limit"`
	IssuedUnixNano     int64  `json:"issued_unix_nano"`
	ExpiresUnixNano    int64  `json:"expires_unix_nano"`
	State              string `json:"state"`
	Digest             string `json:"digest"`
}

func (c TimelineCursor) canonicalDigest() (string, error) {
	copy := c
	copy.Digest = ""
	return CanonicalDigest(copy)
}

func (c TimelineCursor) Encode() (string, error) {
	digest, err := c.canonicalDigest()
	if err != nil {
		return "", err
	}
	c.Digest = digest
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func DecodeTimelineCursor(token string) (TimelineCursor, error) {
	var cursor TimelineCursor
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return cursor, NewError(ErrCursorInvalidated, "cursor", "invalid encoding")
	}
	if err := json.Unmarshal(b, &cursor); err != nil {
		return cursor, NewError(ErrCursorInvalidated, "cursor", "invalid payload")
	}
	expected, err := cursor.canonicalDigest()
	if err != nil || cursor.Digest == "" || cursor.Digest != expected {
		return TimelineCursor{}, NewError(ErrCursorInvalidated, "cursor", "digest mismatch")
	}
	canonical, err := cursor.Encode()
	if err != nil || canonical != token {
		return TimelineCursor{}, NewError(ErrCursorInvalidated, "cursor", "noncanonical encoding")
	}
	return cursor, nil
}

func (c TimelineCursor) ValidateFor(q TimelineQuery, now time.Time) error {
	queryDigest, err := q.Digest()
	if err != nil {
		return err
	}
	if c.State != "active" || c.LedgerScopeDigest != q.LedgerScopeDigest ||
		c.QueryDigest != queryDigest || c.AuthorityWatermark != q.AuthorityWatermark ||
		c.PolicyWatermark != q.PolicyWatermark || c.ProjectionSchema != ProjectionSchema ||
		c.PageLimit != q.PageLimit {
		return NewError(ErrCursorInvalidated, "cursor", "query, authority, policy, or schema drift")
	}
	if now.UnixNano() >= c.ExpiresUnixNano {
		return NewError(ErrCursorExpired, "cursor", "expired")
	}
	return nil
}

type TimelinePage struct {
	Records    []TimelineEventRecord `json:"records"`
	NextCursor string                `json:"next_cursor,omitempty"`
	Exhausted  bool                  `json:"exhausted"`
}
