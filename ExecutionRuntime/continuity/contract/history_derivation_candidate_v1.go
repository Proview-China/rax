package contract

import "time"

const (
	HistoryDerivationContractV1   = "praxis.continuity/history-derivation-candidate/v1"
	HistoryDerivationFactSchemaV1 = "praxis.continuity/history-derivation-candidate-fact/v1"
	HistoryDerivationCapabilityV1 = "continuity/history-derivation-candidate-v1"
	HistoryDerivationAuthorityV1  = "candidate_only"
)

type HistoryDerivationKindV1 string

const (
	HistoryDerivationProjection HistoryDerivationKindV1 = "projection"
	HistoryDerivationSummary    HistoryDerivationKindV1 = "summary"
	HistoryDerivationIndex      HistoryDerivationKindV1 = "index"
)

func (k HistoryDerivationKindV1) Validate() error {
	switch k {
	case HistoryDerivationProjection, HistoryDerivationSummary, HistoryDerivationIndex:
		return nil
	default:
		return NewError(ErrInvalidArgument, "history_derivation_kind", "unknown derivation kind")
	}
}

type HistoryDerivationSourceCoordinateV1 struct {
	EvidenceRecordRef            string `json:"evidence_record_ref"`
	ExpectedEvidenceRecordDigest string `json:"expected_evidence_record_digest"`
	ExpectedProjectionDigest     string `json:"expected_projection_digest"`
}

func (s HistoryDerivationSourceCoordinateV1) Validate() error {
	if err := ValidateToken("evidence_record_ref", s.EvidenceRecordRef); err != nil {
		return err
	}
	if err := ValidateDigest("expected_evidence_record_digest", s.ExpectedEvidenceRecordDigest); err != nil {
		return err
	}
	return ValidateDigest("expected_projection_digest", s.ExpectedProjectionDigest)
}

func NormalizeHistoryDerivationSourcesV1(values []HistoryDerivationSourceCoordinateV1) ([]HistoryDerivationSourceCoordinateV1, error) {
	if len(values) == 0 || len(values) > MaxReferenceCount {
		return nil, NewError(ErrInvalidArgument, "history_derivation_sources", "one or more bounded ordered sources are required")
	}
	result := append([]HistoryDerivationSourceCoordinateV1{}, values...)
	seen := make(map[string]struct{}, len(result))
	for i := range result {
		if err := result[i].Validate(); err != nil {
			return nil, err
		}
		if _, ok := seen[result[i].EvidenceRecordRef]; ok {
			return nil, NewError(ErrInvalidArgument, "history_derivation_sources", "duplicate Evidence Record ref")
		}
		seen[result[i].EvidenceRecordRef] = struct{}{}
	}
	return result, nil
}

type HistoryDerivationEventRefV1 struct {
	EvidenceRecordRef    string `json:"evidence_record_ref"`
	EvidenceRecordDigest string `json:"evidence_record_digest"`
	LedgerScopeDigest    string `json:"ledger_scope_digest"`
	LedgerSequence       uint64 `json:"ledger_sequence"`
	CandidateID          string `json:"candidate_id"`
	CandidateRevision    uint64 `json:"candidate_revision"`
	ProjectionDigest     string `json:"projection_digest"`
	Visibility           string `json:"visibility"`
}

func (r HistoryDerivationEventRefV1) Validate() error {
	for field, value := range map[string]string{
		"evidence_record_ref": r.EvidenceRecordRef, "ledger_scope_digest": r.LedgerScopeDigest,
		"candidate_id": r.CandidateID, "visibility": r.Visibility,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := ValidateDigest("evidence_record_digest", r.EvidenceRecordDigest); err != nil {
		return err
	}
	if err := ValidateDigest("projection_digest", r.ProjectionDigest); err != nil {
		return err
	}
	if r.LedgerSequence == 0 || r.CandidateRevision == 0 {
		return NewError(ErrInvalidArgument, "history_derivation_event_ref", "ledger sequence and candidate revision are required")
	}
	if r.Visibility != "visible" && r.Visibility != "tombstoned" {
		return NewError(ErrInvalidArgument, "visibility", "unknown Event visibility")
	}
	return nil
}

func HistoryDerivationEventRefFromRecordV1(record TimelineEventRecord) HistoryDerivationEventRefV1 {
	return HistoryDerivationEventRefV1{
		EvidenceRecordRef: record.EvidenceRecordRef, EvidenceRecordDigest: record.EvidenceRecordDigest,
		LedgerScopeDigest: record.LedgerScopeDigest, LedgerSequence: record.LedgerSequence,
		CandidateID: record.Candidate.CandidateID, CandidateRevision: record.Candidate.Revision,
		ProjectionDigest: record.Candidate.Digest, Visibility: record.Visibility,
	}
}

type HistoryDerivationCandidateFactV1 struct {
	ContractVersion string                        `json:"contract_version"`
	SchemaRef       string                        `json:"schema_ref"`
	CandidateID     string                        `json:"candidate_id"`
	Revision        uint64                        `json:"revision"`
	IdempotencyKey  string                        `json:"idempotency_key"`
	RequestDigest   string                        `json:"request_digest"`
	Scope           Scope                         `json:"scope"`
	Owner           OwnerBinding                  `json:"owner"`
	Kind            HistoryDerivationKindV1       `json:"kind"`
	Sources         []HistoryDerivationEventRefV1 `json:"sources"`
	SourceSetDigest string                        `json:"source_set_digest"`
	Output          ContentObjectRefV1            `json:"output"`
	Authority       string                        `json:"authority"`
	CreatedUnixNano int64                         `json:"created_unix_nano"`
	Digest          string                        `json:"digest"`
}

func (f HistoryDerivationCandidateFactV1) CanonicalDigest() (string, error) {
	copy := f.Clone()
	copy.Digest = ""
	return CanonicalDigest(copy)
}

func (f HistoryDerivationCandidateFactV1) Validate() error {
	if f.ContractVersion != HistoryDerivationContractV1 || f.SchemaRef != HistoryDerivationFactSchemaV1 {
		return NewError(ErrInvalidArgument, "history_derivation_contract", "unsupported contract or schema")
	}
	if err := ValidateToken("candidate_id", f.CandidateID); err != nil {
		return err
	}
	if err := ValidateToken("idempotency_key", f.IdempotencyKey); err != nil {
		return err
	}
	if err := ValidateDigest("request_digest", f.RequestDigest); err != nil {
		return err
	}
	if f.Revision != 1 || f.CreatedUnixNano <= 0 || f.Authority != HistoryDerivationAuthorityV1 {
		return NewError(ErrInvalidArgument, "history_derivation_fact", "immutable revision one and candidate-only authority are required")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	if err := validateHistoryDerivationOwnerV1(f.Owner); err != nil {
		return err
	}
	if err := f.Kind.Validate(); err != nil {
		return err
	}
	if err := f.Output.Validate(); err != nil {
		return err
	}
	if f.Output.ScopeDigest != f.Scope.ExecutionScopeDigest {
		return NewError(ErrRevisionConflict, "history_derivation_scope", "output belongs to another execution scope")
	}
	if len(f.Sources) == 0 || len(f.Sources) > MaxReferenceCount {
		return NewError(ErrInvalidArgument, "history_derivation_sources", "one or more bounded sources are required")
	}
	seen := make(map[string]struct{}, len(f.Sources))
	for _, source := range f.Sources {
		if err := source.Validate(); err != nil {
			return err
		}
		if _, ok := seen[source.EvidenceRecordRef]; ok {
			return NewError(ErrInvalidArgument, "history_derivation_sources", "duplicate Evidence Record ref")
		}
		seen[source.EvidenceRecordRef] = struct{}{}
	}
	sourceDigest, err := CanonicalDigest(f.Sources)
	if err != nil {
		return err
	}
	if f.SourceSetDigest != sourceDigest {
		return NewError(ErrRevisionConflict, "source_set_digest", "ordered source set digest mismatch")
	}
	expected, err := f.CanonicalDigest()
	if err != nil {
		return err
	}
	if f.Digest == "" || f.Digest != expected {
		return NewError(ErrRevisionConflict, "history_derivation_digest", "canonical digest mismatch")
	}
	return nil
}

func (f HistoryDerivationCandidateFactV1) Ref() HistoryDerivationCandidateRefV1 {
	return HistoryDerivationCandidateRefV1(ExactFactRefV2{
		ContractVersion: f.ContractVersion, SchemaRef: f.SchemaRef, Owner: f.Owner,
		TenantID: f.Scope.TenantID, ID: f.CandidateID, Revision: f.Revision,
		Digest: f.Digest, ScopeDigest: f.Scope.ExecutionScopeDigest,
	})
}

func (f HistoryDerivationCandidateFactV1) Clone() HistoryDerivationCandidateFactV1 {
	result := f
	result.Sources = append([]HistoryDerivationEventRefV1{}, f.Sources...)
	return result
}

type HistoryDerivationCandidateRefV1 ExactFactRefV2

func (r HistoryDerivationCandidateRefV1) Validate() error {
	value := ExactFactRefV2(r)
	if err := value.Validate(); err != nil {
		return err
	}
	if value.ContractVersion != HistoryDerivationContractV1 || value.SchemaRef != HistoryDerivationFactSchemaV1 || value.Revision != 1 {
		return NewError(ErrInvalidArgument, "history_derivation_ref", "wrong contract, schema, or revision")
	}
	return validateHistoryDerivationOwnerV1(value.Owner)
}

func (r HistoryDerivationCandidateRefV1) Exact() ExactFactRefV2 { return ExactFactRefV2(r) }

func NewHistoryDerivationCandidateFactV1(candidateID, idempotencyKey, requestDigest string, scope Scope, owner OwnerBinding, kind HistoryDerivationKindV1, sources []HistoryDerivationEventRefV1, output ContentObjectRefV1, now time.Time) (HistoryDerivationCandidateFactV1, error) {
	sourceDigest, err := CanonicalDigest(sources)
	if err != nil {
		return HistoryDerivationCandidateFactV1{}, err
	}
	fact := HistoryDerivationCandidateFactV1{
		ContractVersion: HistoryDerivationContractV1, SchemaRef: HistoryDerivationFactSchemaV1,
		CandidateID: candidateID, Revision: 1, IdempotencyKey: idempotencyKey, RequestDigest: requestDigest,
		Scope: scope, Owner: owner, Kind: kind, Sources: append([]HistoryDerivationEventRefV1{}, sources...),
		SourceSetDigest: sourceDigest, Output: output, Authority: HistoryDerivationAuthorityV1,
		CreatedUnixNano: now.UnixNano(),
	}
	digest, err := fact.CanonicalDigest()
	if err != nil {
		return HistoryDerivationCandidateFactV1{}, err
	}
	fact.Digest = digest
	if err := fact.Validate(); err != nil {
		return HistoryDerivationCandidateFactV1{}, err
	}
	return fact, nil
}

func validateHistoryDerivationOwnerV1(owner OwnerBinding) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	if owner.ComponentID != ContinuityComponentID || owner.Capability != HistoryDerivationCapabilityV1 || owner.FactKind != "history_derivation_candidate_fact_v1" {
		return NewError(ErrInvalidArgument, "owner_binding", "wrong Continuity History Derivation owner")
	}
	return nil
}
