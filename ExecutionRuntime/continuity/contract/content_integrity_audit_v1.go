package contract

import (
	"sort"
	"time"
)

const (
	ContentIntegrityAuditContractV1   = "praxis.continuity/content-integrity-audit/v1"
	ContentIntegrityAuditFactSchemaV1 = "praxis.continuity/content-integrity-audit-fact/v1"
	ContentIntegrityAuditCapabilityV1 = "continuity/content-integrity-audit-v1"
)

type ContentIntegrityClassificationV1 string

const (
	ContentIntegrityHealthy           ContentIntegrityClassificationV1 = "healthy"
	ContentIntegrityWriteIncomplete   ContentIntegrityClassificationV1 = "write_incomplete"
	ContentIntegrityMetadataAbsent    ContentIntegrityClassificationV1 = "metadata_absent"
	ContentIntegrityJournalAbsent     ContentIntegrityClassificationV1 = "journal_absent"
	ContentIntegrityDanglingReference ContentIntegrityClassificationV1 = "dangling_reference"
	ContentIntegrityCorruptContent    ContentIntegrityClassificationV1 = "corrupt_content"
	ContentIntegrityIndeterminate     ContentIntegrityClassificationV1 = "indeterminate"
)

func (v ContentIntegrityClassificationV1) Validate() error {
	switch v {
	case ContentIntegrityHealthy, ContentIntegrityWriteIncomplete,
		ContentIntegrityMetadataAbsent, ContentIntegrityJournalAbsent,
		ContentIntegrityDanglingReference, ContentIntegrityCorruptContent,
		ContentIntegrityIndeterminate:
		return nil
	default:
		return NewError(ErrInvalidArgument, "content_integrity_classification", "unknown classification")
	}
}

type ContentIntegrityStatusV1 string

const (
	ContentIntegrityStatusHealthy           ContentIntegrityStatusV1 = "healthy"
	ContentIntegrityStatusAttentionRequired ContentIntegrityStatusV1 = "attention_required"
	ContentIntegrityStatusIndeterminate     ContentIntegrityStatusV1 = "indeterminate"
)

func (v ContentIntegrityStatusV1) Validate() error {
	switch v {
	case ContentIntegrityStatusHealthy, ContentIntegrityStatusAttentionRequired, ContentIntegrityStatusIndeterminate:
		return nil
	default:
		return NewError(ErrInvalidArgument, "content_integrity_status", "unknown status")
	}
}

type ContentIntegrityChunkStatusV1 string

const (
	ContentIntegrityChunkHealthy       ContentIntegrityChunkStatusV1 = "healthy"
	ContentIntegrityChunkMissing       ContentIntegrityChunkStatusV1 = "missing"
	ContentIntegrityChunkCorrupt       ContentIntegrityChunkStatusV1 = "corrupt"
	ContentIntegrityChunkIndeterminate ContentIntegrityChunkStatusV1 = "indeterminate"
)

func (v ContentIntegrityChunkStatusV1) Validate() error {
	switch v {
	case ContentIntegrityChunkHealthy, ContentIntegrityChunkMissing,
		ContentIntegrityChunkCorrupt, ContentIntegrityChunkIndeterminate:
		return nil
	default:
		return NewError(ErrInvalidArgument, "content_integrity_chunk_status", "unknown chunk status")
	}
}

// ContentIntegritySubjectV1 is coordinate-only. It carries no caller-asserted
// visibility, journal state, chunk state, or cleanup conclusion.
type ContentIntegritySubjectV1 struct {
	ObjectID               string `json:"object_id"`
	JournalID              string `json:"journal_id"`
	ExpectedManifestDigest string `json:"expected_manifest_digest,omitempty"`
}

func (s ContentIntegritySubjectV1) Validate() error {
	if err := ValidateToken("object_id", s.ObjectID); err != nil {
		return err
	}
	if err := ValidateToken("journal_id", s.JournalID); err != nil {
		return err
	}
	if s.ExpectedManifestDigest != "" {
		return ValidateDigest("expected_manifest_digest", s.ExpectedManifestDigest)
	}
	return nil
}

func NormalizeContentIntegritySubjectsV1(values []ContentIntegritySubjectV1) ([]ContentIntegritySubjectV1, error) {
	if len(values) == 0 || len(values) > MaxReferenceCount {
		return nil, NewError(ErrInvalidArgument, "content_integrity_subjects", "one or more bounded subjects are required")
	}
	result := append([]ContentIntegritySubjectV1{}, values...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].ObjectID != result[j].ObjectID {
			return result[i].ObjectID < result[j].ObjectID
		}
		return result[i].JournalID < result[j].JournalID
	})
	for i := range result {
		if err := result[i].Validate(); err != nil {
			return nil, err
		}
		if i > 0 && result[i-1].ObjectID == result[i].ObjectID && result[i-1].JournalID == result[i].JournalID {
			return nil, NewError(ErrInvalidArgument, "content_integrity_subjects", "duplicate object and journal coordinates")
		}
	}
	return result, nil
}

type ContentIntegrityChunkFindingV1 struct {
	Chunk      ChunkRef                      `json:"chunk"`
	Status     ContentIntegrityChunkStatusV1 `json:"status"`
	DetailCode string                        `json:"detail_code"`
}

func (f ContentIntegrityChunkFindingV1) Validate() error {
	if err := f.Chunk.Validate(); err != nil {
		return err
	}
	if err := f.Status.Validate(); err != nil {
		return err
	}
	return ValidateToken("detail_code", f.DetailCode)
}

type ContentIntegrityFindingV1 struct {
	Subject         ContentIntegritySubjectV1        `json:"subject"`
	ManifestDigest  string                           `json:"manifest_digest,omitempty"`
	Visible         bool                             `json:"visible"`
	JournalRevision uint64                           `json:"journal_revision,omitempty"`
	JournalState    JournalState                     `json:"journal_state,omitempty"`
	Chunks          []ContentIntegrityChunkFindingV1 `json:"chunks"`
	Classification  ContentIntegrityClassificationV1 `json:"classification"`
	DetailCode      string                           `json:"detail_code"`
}

func (f ContentIntegrityFindingV1) Validate() error {
	if err := f.Subject.Validate(); err != nil {
		return err
	}
	if err := f.Classification.Validate(); err != nil {
		return err
	}
	if err := ValidateToken("detail_code", f.DetailCode); err != nil {
		return err
	}
	if f.ManifestDigest != "" {
		if err := ValidateDigest("manifest_digest", f.ManifestDigest); err != nil {
			return err
		}
	}
	if (f.JournalRevision == 0) != (f.JournalState == "") {
		return NewError(ErrInvalidArgument, "journal_observation", "revision and state must be present together")
	}
	if f.JournalState != "" && !validJournalState(f.JournalState) {
		return NewError(ErrInvalidArgument, "journal_state", "unknown state")
	}
	if len(f.Chunks) > MaxReferenceCount {
		return NewError(ErrInvalidArgument, "chunk_findings", "too many chunk findings")
	}
	for _, chunk := range f.Chunks {
		if err := chunk.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (f ContentIntegrityFindingV1) Clone() ContentIntegrityFindingV1 {
	result := f
	result.Chunks = append([]ContentIntegrityChunkFindingV1{}, f.Chunks...)
	return result
}

type ContentIntegrityAuditFactV1 struct {
	ContractVersion string                      `json:"contract_version"`
	SchemaRef       string                      `json:"schema_ref"`
	AuditID         string                      `json:"audit_id"`
	Revision        uint64                      `json:"revision"`
	IdempotencyKey  string                      `json:"idempotency_key"`
	RequestDigest   string                      `json:"request_digest"`
	Scope           Scope                       `json:"scope"`
	Owner           OwnerBinding                `json:"owner"`
	Subjects        []ContentIntegritySubjectV1 `json:"subjects"`
	Findings        []ContentIntegrityFindingV1 `json:"findings"`
	Status          ContentIntegrityStatusV1    `json:"status"`
	CreatedUnixNano int64                       `json:"created_unix_nano"`
	Digest          string                      `json:"digest"`
}

func (f ContentIntegrityAuditFactV1) CanonicalDigest() (string, error) {
	copy := f.Clone()
	copy.Digest = ""
	return CanonicalDigest(copy)
}

func (f ContentIntegrityAuditFactV1) Validate() error {
	if f.ContractVersion != ContentIntegrityAuditContractV1 || f.SchemaRef != ContentIntegrityAuditFactSchemaV1 {
		return NewError(ErrInvalidArgument, "content_integrity_audit_contract", "unsupported contract or schema")
	}
	if err := ValidateToken("audit_id", f.AuditID); err != nil {
		return err
	}
	if err := ValidateToken("idempotency_key", f.IdempotencyKey); err != nil {
		return err
	}
	if err := ValidateDigest("request_digest", f.RequestDigest); err != nil {
		return err
	}
	if f.Revision != 1 || f.CreatedUnixNano <= 0 {
		return NewError(ErrInvalidArgument, "content_integrity_audit_fact", "immutable audit requires revision one and creation time")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	if err := validateContentIntegrityAuditOwnerV1(f.Owner); err != nil {
		return err
	}
	if err := f.Status.Validate(); err != nil {
		return err
	}
	subjects, err := NormalizeContentIntegritySubjectsV1(f.Subjects)
	if err != nil {
		return err
	}
	if len(subjects) != len(f.Findings) {
		return NewError(ErrInvalidArgument, "content_integrity_findings", "one finding per subject is required")
	}
	for i := range subjects {
		if err := f.Findings[i].Validate(); err != nil {
			return err
		}
		if f.Findings[i].Subject != subjects[i] {
			return NewError(ErrInvalidArgument, "content_integrity_findings", "findings must match normalized subjects")
		}
	}
	if contentIntegrityAggregateStatusV1(f.Findings) != f.Status {
		return NewError(ErrInvalidArgument, "content_integrity_status", "aggregate status does not match findings")
	}
	expected, err := f.CanonicalDigest()
	if err != nil {
		return err
	}
	if f.Digest == "" || f.Digest != expected {
		return NewError(ErrRevisionConflict, "content_integrity_audit_digest", "canonical digest mismatch")
	}
	return nil
}

func (f ContentIntegrityAuditFactV1) Ref() ContentIntegrityAuditRefV1 {
	return ContentIntegrityAuditRefV1(ExactFactRefV2{
		ContractVersion: f.ContractVersion, SchemaRef: f.SchemaRef, Owner: f.Owner,
		TenantID: f.Scope.TenantID, ID: f.AuditID, Revision: f.Revision,
		Digest: f.Digest, ScopeDigest: f.Scope.ExecutionScopeDigest,
	})
}

func (f ContentIntegrityAuditFactV1) Clone() ContentIntegrityAuditFactV1 {
	result := f
	result.Subjects = append([]ContentIntegritySubjectV1{}, f.Subjects...)
	result.Findings = make([]ContentIntegrityFindingV1, len(f.Findings))
	for i := range f.Findings {
		result.Findings[i] = f.Findings[i].Clone()
	}
	return result
}

type ContentIntegrityAuditRefV1 ExactFactRefV2

func (r ContentIntegrityAuditRefV1) Validate() error {
	value := ExactFactRefV2(r)
	if err := value.Validate(); err != nil {
		return err
	}
	if value.ContractVersion != ContentIntegrityAuditContractV1 || value.SchemaRef != ContentIntegrityAuditFactSchemaV1 || value.Revision != 1 {
		return NewError(ErrInvalidArgument, "content_integrity_audit_ref", "wrong contract, schema, or revision")
	}
	return validateContentIntegrityAuditOwnerV1(value.Owner)
}

func (r ContentIntegrityAuditRefV1) Exact() ExactFactRefV2 { return ExactFactRefV2(r) }

func NewContentIntegrityAuditFactV1(auditID, idempotencyKey, requestDigest string, scope Scope, owner OwnerBinding, subjects []ContentIntegritySubjectV1, findings []ContentIntegrityFindingV1, now time.Time) (ContentIntegrityAuditFactV1, error) {
	normalized, err := NormalizeContentIntegritySubjectsV1(subjects)
	if err != nil {
		return ContentIntegrityAuditFactV1{}, err
	}
	fact := ContentIntegrityAuditFactV1{
		ContractVersion: ContentIntegrityAuditContractV1, SchemaRef: ContentIntegrityAuditFactSchemaV1,
		AuditID: auditID, Revision: 1, IdempotencyKey: idempotencyKey, RequestDigest: requestDigest,
		Scope: scope, Owner: owner, Subjects: normalized, Findings: cloneContentIntegrityFindingsV1(findings),
		Status: contentIntegrityAggregateStatusV1(findings), CreatedUnixNano: now.UnixNano(),
	}
	digest, err := fact.CanonicalDigest()
	if err != nil {
		return ContentIntegrityAuditFactV1{}, err
	}
	fact.Digest = digest
	if err := fact.Validate(); err != nil {
		return ContentIntegrityAuditFactV1{}, err
	}
	return fact, nil
}

func validateContentIntegrityAuditOwnerV1(owner OwnerBinding) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	if owner.ComponentID != ContinuityComponentID || owner.Capability != ContentIntegrityAuditCapabilityV1 || owner.FactKind != "content_integrity_audit_fact_v1" {
		return NewError(ErrInvalidArgument, "owner_binding", "wrong Continuity Content Integrity Audit owner")
	}
	return nil
}

func cloneContentIntegrityFindingsV1(values []ContentIntegrityFindingV1) []ContentIntegrityFindingV1 {
	result := make([]ContentIntegrityFindingV1, len(values))
	for i := range values {
		result[i] = values[i].Clone()
	}
	return result
}

func contentIntegrityAggregateStatusV1(findings []ContentIntegrityFindingV1) ContentIntegrityStatusV1 {
	status := ContentIntegrityStatusHealthy
	for _, finding := range findings {
		if finding.Classification == ContentIntegrityIndeterminate {
			return ContentIntegrityStatusIndeterminate
		}
		if finding.Classification != ContentIntegrityHealthy {
			status = ContentIntegrityStatusAttentionRequired
		}
	}
	return status
}
