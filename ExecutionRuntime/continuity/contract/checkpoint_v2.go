package contract

import (
	"cmp"
	"sort"
)

const (
	CheckpointManifestGovernanceContractV2 = "praxis.continuity/checkpoint-manifest-governance/v2"
	CheckpointManifestFactSchemaV2         = "praxis.continuity/checkpoint-manifest-fact/v2"
	CheckpointManifestSealSchemaV2         = "praxis.continuity/checkpoint-manifest-seal-fact/v2"
	CheckpointManifestCapabilityV2         = "checkpoint-manifest-governance-v2"
	ContinuityComponentID                  = "praxis/continuity"
)

// ExactFactRefV2 is a neutral, versioned cross-owner reference. It deliberately
// carries no copied owner state, outcome, verdict, or payload.
type ExactFactRefV2 struct {
	ContractVersion string       `json:"contract_version"`
	SchemaRef       string       `json:"schema_ref"`
	Owner           OwnerBinding `json:"owner"`
	TenantID        string       `json:"tenant_id"`
	ID              string       `json:"id"`
	Revision        uint64       `json:"revision"`
	Digest          string       `json:"digest"`
	ScopeDigest     string       `json:"scope_digest"`
}

// ExactFactIdentityKeyV2 is the complete comparable identity of an exact
// cross-owner reference. It is deliberately structural: every OwnerBinding
// field participates, and delimiter-bearing token values cannot alias.
type ExactFactIdentityKeyV2 struct {
	ContractVersion string
	SchemaRef       string
	Owner           OwnerBinding
	TenantID        string
	ID              string
	Revision        uint64
	Digest          string
	ScopeDigest     string
}

func (r ExactFactRefV2) Validate() error {
	for field, value := range map[string]string{
		"contract_version": r.ContractVersion,
		"schema_ref":       r.SchemaRef,
		"tenant_id":        r.TenantID,
		"id":               r.ID,
		"scope_digest":     r.ScopeDigest,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := ValidateDigest("digest", r.Digest); err != nil {
		return err
	}
	if r.Revision == 0 {
		return NewError(ErrInvalidArgument, "revision", "must be non-zero")
	}
	return r.Owner.Validate()
}

func (r ExactFactRefV2) IdentityKey() ExactFactIdentityKeyV2 {
	return ExactFactIdentityKeyV2{
		ContractVersion: r.ContractVersion,
		SchemaRef:       r.SchemaRef,
		Owner:           r.Owner,
		TenantID:        r.TenantID,
		ID:              r.ID,
		Revision:        r.Revision,
		Digest:          r.Digest,
		ScopeDigest:     r.ScopeDigest,
	}
}

func (r ExactFactRefV2) Equal(other ExactFactRefV2) bool {
	return r == other
}

type CheckpointManifestRefV2 ExactFactRefV2

func (r CheckpointManifestRefV2) Validate() error {
	value := ExactFactRefV2(r)
	if err := value.Validate(); err != nil {
		return err
	}
	if value.ContractVersion != CheckpointManifestGovernanceContractV2 || value.SchemaRef != CheckpointManifestFactSchemaV2 {
		return NewError(ErrInvalidArgument, "manifest_ref", "wrong contract or schema")
	}
	return validateContinuityOwnerV2(value.Owner, "checkpoint_manifest_fact_v2")
}

func (r CheckpointManifestRefV2) Exact() ExactFactRefV2 { return ExactFactRefV2(r) }

type CheckpointManifestSealRefV2 ExactFactRefV2

func (r CheckpointManifestSealRefV2) Validate() error {
	value := ExactFactRefV2(r)
	if err := value.Validate(); err != nil {
		return err
	}
	if value.ContractVersion != CheckpointManifestGovernanceContractV2 || value.SchemaRef != CheckpointManifestSealSchemaV2 {
		return NewError(ErrInvalidArgument, "manifest_seal_ref", "wrong contract or schema")
	}
	if value.Revision != 1 {
		return NewError(ErrInvalidArgument, "manifest_seal_ref", "seal revision must be 1")
	}
	return validateContinuityOwnerV2(value.Owner, "checkpoint_manifest_seal_fact_v2")
}

func (r CheckpointManifestSealRefV2) Exact() ExactFactRefV2 { return ExactFactRefV2(r) }

type TimelineCutV2 struct {
	LedgerScopeDigest string         `json:"ledger_scope_digest"`
	LedgerSequence    uint64         `json:"ledger_sequence"`
	EvidenceRecordRef ExactFactRefV2 `json:"evidence_record_ref"`
}

func (c TimelineCutV2) Validate() error {
	if err := ValidateDigest("ledger_scope_digest", c.LedgerScopeDigest); err != nil {
		return err
	}
	if c.LedgerSequence == 0 {
		return NewError(ErrInvalidArgument, "ledger_sequence", "must be non-zero")
	}
	return c.EvidenceRecordRef.Validate()
}

type AttemptSettlementClosureV2 struct {
	AttemptRef    ExactFactRefV2   `json:"attempt_ref"`
	Begun         bool             `json:"begun"`
	SettlementRef *ExactFactRefV2  `json:"settlement_ref,omitempty"`
	InspectionRef *ExactFactRefV2  `json:"inspection_ref,omitempty"`
	ResidualRefs  []ExactFactRefV2 `json:"residual_refs"`
}

func (c AttemptSettlementClosureV2) Validate() error {
	if err := c.AttemptRef.Validate(); err != nil {
		return err
	}
	if c.SettlementRef != nil {
		if !c.Begun {
			return NewError(ErrInvalidArgument, "settlement_ref", "settlement cannot precede Begin")
		}
		if err := c.SettlementRef.Validate(); err != nil {
			return err
		}
	}
	if c.InspectionRef != nil {
		if err := c.InspectionRef.Validate(); err != nil {
			return err
		}
	}
	residuals, err := normalizeExactRefsV2(c.ResidualRefs, "attempt_residual_refs")
	if err != nil {
		return err
	}
	if c.Begun && c.SettlementRef == nil && (c.InspectionRef == nil || len(residuals) == 0) {
		return NewError(ErrCheckpointIndeterminate, "attempt_closure", "begun attempt without settlement requires exact inspection and residual")
	}
	return nil
}

func (c AttemptSettlementClosureV2) clone() AttemptSettlementClosureV2 {
	result := c
	result.SettlementRef = cloneExactRefV2(c.SettlementRef)
	result.InspectionRef = cloneExactRefV2(c.InspectionRef)
	result.ResidualRefs = append([]ExactFactRefV2{}, c.ResidualRefs...)
	return result
}

type ParticipantClosureRefV2 struct {
	ParticipantID      string           `json:"participant_id"`
	Required           bool             `json:"required"`
	RuntimeClosureRef  ExactFactRefV2   `json:"runtime_closure_ref"`
	ParticipantFactRef ExactFactRefV2   `json:"participant_fact_ref"`
	SnapshotRef        *ExactFactRefV2  `json:"snapshot_ref,omitempty"`
	CoverageRef        *ExactFactRefV2  `json:"coverage_ref,omitempty"`
	EvidenceRefs       []ExactFactRefV2 `json:"evidence_refs"`
	ResidualRefs       []ExactFactRefV2 `json:"residual_refs"`
}

func (c ParticipantClosureRefV2) Validate() error {
	if err := ValidateToken("participant_id", c.ParticipantID); err != nil {
		return err
	}
	if err := c.ParticipantFactRef.Validate(); err != nil {
		return err
	}
	if err := c.RuntimeClosureRef.Validate(); err != nil {
		return err
	}
	for _, ref := range []*ExactFactRefV2{c.SnapshotRef, c.CoverageRef} {
		if ref != nil {
			if err := ref.Validate(); err != nil {
				return err
			}
		}
	}
	if _, err := normalizeExactRefsV2(c.EvidenceRefs, "participant_evidence_refs"); err != nil {
		return err
	}
	_, err := normalizeExactRefsV2(c.ResidualRefs, "participant_residual_refs")
	return err
}

func (c ParticipantClosureRefV2) completeForSeal() bool {
	return c.RuntimeClosureRef.Validate() == nil && c.SnapshotRef != nil && c.CoverageRef != nil && len(c.EvidenceRefs) > 0
}

func (c ParticipantClosureRefV2) clone() ParticipantClosureRefV2 {
	result := c
	result.SnapshotRef = cloneExactRefV2(c.SnapshotRef)
	result.CoverageRef = cloneExactRefV2(c.CoverageRef)
	result.EvidenceRefs = append([]ExactFactRefV2{}, c.EvidenceRefs...)
	result.ResidualRefs = append([]ExactFactRefV2{}, c.ResidualRefs...)
	return result
}

type ManifestDiagnosticSeverityV2 string

const (
	ManifestDiagnosticInfoV2     ManifestDiagnosticSeverityV2 = "info"
	ManifestDiagnosticWarningV2  ManifestDiagnosticSeverityV2 = "warning"
	ManifestDiagnosticBlockingV2 ManifestDiagnosticSeverityV2 = "blocking"
)

type CheckpointManifestDiagnosticV2 struct {
	DiagnosticID  string                       `json:"diagnostic_id"`
	DiagnosticRef ExactFactRefV2               `json:"diagnostic_ref"`
	Code          string                       `json:"code"`
	Severity      ManifestDiagnosticSeverityV2 `json:"severity"`
	SubjectRef    *ExactFactRefV2              `json:"subject_ref,omitempty"`
	InspectionRef *ExactFactRefV2              `json:"inspection_ref,omitempty"`
	ResidualRefs  []ExactFactRefV2             `json:"residual_refs"`
}

func (d CheckpointManifestDiagnosticV2) Validate() error {
	for field, value := range map[string]string{"diagnostic_id": d.DiagnosticID, "diagnostic_code": d.Code} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := d.DiagnosticRef.Validate(); err != nil {
		return err
	}
	switch d.Severity {
	case ManifestDiagnosticInfoV2, ManifestDiagnosticWarningV2, ManifestDiagnosticBlockingV2:
	default:
		return NewError(ErrInvalidArgument, "diagnostic_severity", "unknown severity")
	}
	for _, ref := range []*ExactFactRefV2{d.SubjectRef, d.InspectionRef} {
		if ref != nil {
			if err := ref.Validate(); err != nil {
				return err
			}
		}
	}
	residuals, err := normalizeExactRefsV2(d.ResidualRefs, "diagnostic_residual_refs")
	if err != nil {
		return err
	}
	if d.Severity == ManifestDiagnosticBlockingV2 && d.InspectionRef == nil && len(residuals) == 0 {
		return NewError(ErrInvalidArgument, "diagnostic", "blocking diagnostic requires inspection or residual")
	}
	return nil
}

func (d CheckpointManifestDiagnosticV2) clone() CheckpointManifestDiagnosticV2 {
	result := d
	result.SubjectRef = cloneExactRefV2(d.SubjectRef)
	result.InspectionRef = cloneExactRefV2(d.InspectionRef)
	result.ResidualRefs = append([]ExactFactRefV2{}, d.ResidualRefs...)
	return result
}

const ManifestCollecting ManifestState = "collecting"

type CheckpointManifestFactV2 struct {
	ContractVersion              string                           `json:"contract_version"`
	SchemaRef                    string                           `json:"schema_ref"`
	ManifestID                   string                           `json:"manifest_id"`
	Revision                     uint64                           `json:"revision"`
	Digest                       string                           `json:"digest"`
	Owner                        OwnerBinding                     `json:"owner"`
	Scope                        Scope                            `json:"scope"`
	State                        ManifestState                    `json:"state"`
	IdempotencyKey               string                           `json:"idempotency_key"`
	CheckpointAttemptRef         ExactFactRefV2                   `json:"checkpoint_attempt_ref"`
	BarrierRef                   ExactFactRefV2                   `json:"barrier_ref"`
	EffectCutRef                 ExactFactRefV2                   `json:"effect_cut_ref"`
	TimelineCut                  TimelineCutV2                    `json:"timeline_cut"`
	ContextGenerationRef         ExactFactRefV2                   `json:"context_generation_ref"`
	ContextFrameRefs             []ExactFactRefV2                 `json:"context_frame_refs"`
	AttemptSettlementClosures    []AttemptSettlementClosureV2     `json:"attempt_settlement_closures"`
	MemoryRefs                   []ExactFactRefV2                 `json:"memory_refs"`
	KnowledgeRefs                []ExactFactRefV2                 `json:"knowledge_refs"`
	ParticipantClosures          []ParticipantClosureRefV2        `json:"participant_closures"`
	RuntimeParticipantSetDigest  string                           `json:"runtime_participant_set_digest"`
	RequiredParticipantSetDigest string                           `json:"required_participant_set_digest"`
	FrozenRefSetDigest           string                           `json:"frozen_ref_set_digest"`
	Diagnostics                  []CheckpointManifestDiagnosticV2 `json:"diagnostics"`
	ResidualRefs                 []ExactFactRefV2                 `json:"residual_refs"`
	CreatedUnixNano              int64                            `json:"created_unix_nano"`
	UpdatedUnixNano              int64                            `json:"updated_unix_nano"`
}

func (m CheckpointManifestFactV2) Validate() error {
	if m.ContractVersion != CheckpointManifestGovernanceContractV2 || m.SchemaRef != CheckpointManifestFactSchemaV2 {
		return NewError(ErrInvalidArgument, "manifest", "wrong contract or schema")
	}
	if err := validateContinuityOwnerV2(m.Owner, "checkpoint_manifest_fact_v2"); err != nil {
		return err
	}
	if err := ValidateToken("manifest_id", m.ManifestID); err != nil {
		return err
	}
	if err := ValidateToken("idempotency_key", m.IdempotencyKey); err != nil {
		return err
	}
	if err := m.Scope.Validate(); err != nil {
		return err
	}
	if m.Revision == 0 || m.CreatedUnixNano <= 0 || m.UpdatedUnixNano < m.CreatedUnixNano {
		return NewError(ErrInvalidArgument, "manifest", "invalid revision or timestamps")
	}
	for _, ref := range []ExactFactRefV2{m.CheckpointAttemptRef, m.BarrierRef, m.EffectCutRef, m.ContextGenerationRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if err := m.TimelineCut.Validate(); err != nil {
		return err
	}
	if err := ValidateDigest("required_participant_set_digest", m.RequiredParticipantSetDigest); err != nil {
		return err
	}
	if err := ValidateDigest("runtime_participant_set_digest", m.RuntimeParticipantSetDigest); err != nil {
		return err
	}
	if err := ValidateDigest("frozen_ref_set_digest", m.FrozenRefSetDigest); err != nil {
		return err
	}
	if len(m.ContextFrameRefs) == 0 || len(m.ParticipantClosures) == 0 {
		return NewError(ErrInvalidArgument, "manifest_closure", "context frame and participant closures are required")
	}
	if _, err := normalizeExactRefsV2(m.ContextFrameRefs, "context_frame_refs"); err != nil {
		return err
	}
	if _, err := normalizeExactRefsV2(m.MemoryRefs, "memory_refs"); err != nil {
		return err
	}
	if _, err := normalizeExactRefsV2(m.KnowledgeRefs, "knowledge_refs"); err != nil {
		return err
	}
	closures, err := normalizeAttemptClosuresV2(m.AttemptSettlementClosures)
	if err != nil {
		return err
	}
	participants, err := normalizeParticipantClosuresV2(m.ParticipantClosures)
	if err != nil {
		return err
	}
	requiredDigest, err := RequiredParticipantSetDigestV2(participants)
	if err != nil {
		return err
	}
	if m.RequiredParticipantSetDigest != requiredDigest {
		return NewError(ErrRevisionConflict, "required_participant_set_digest", "does not match required participant closures")
	}
	diagnostics, err := normalizeManifestDiagnosticsV2(m.Diagnostics)
	if err != nil {
		return err
	}
	residuals, err := AggregateCheckpointManifestResidualRefsV2(m)
	if err != nil {
		return err
	}
	if err := ValidateCheckpointManifestScopeClosureV2(m); err != nil {
		return err
	}
	frozenDigest, err := FrozenRefSetDigestV2(m)
	if err != nil {
		return err
	}
	if m.FrozenRefSetDigest != frozenDigest {
		return NewError(ErrRevisionConflict, "frozen_ref_set_digest", "does not match exact reference closure")
	}
	hasBlocking := false
	for _, diagnostic := range diagnostics {
		hasBlocking = hasBlocking || diagnostic.Severity == ManifestDiagnosticBlockingV2
	}
	hasUnsettledBegin := false
	for _, closure := range closures {
		hasUnsettledBegin = hasUnsettledBegin || (closure.Begun && closure.SettlementRef == nil)
	}
	switch m.State {
	case ManifestCollecting:
	case ManifestVerifiedCandidate:
		if hasBlocking || hasUnsettledBegin || len(residuals) != 0 {
			return NewError(ErrCheckpointIndeterminate, "manifest_state", "verified_candidate cannot contain blocking diagnostics, unsettled Begin, or any nested residual")
		}
		for _, participant := range participants {
			if participant.Required && !participant.completeForSeal() {
				return NewError(ErrCheckpointPartial, "participant_closure", "required participant closure is incomplete")
			}
		}
	case ManifestDiagnosticPartial, ManifestDiagnosticIndeterminate, ManifestRejected:
		if !hasBlocking && len(residuals) == 0 && !hasUnsettledBegin {
			return NewError(ErrInvalidArgument, "manifest_state", "diagnostic or rejected finalization requires an explicit blocking diagnostic, unknown Begin, or residual")
		}
	default:
		return NewError(ErrInvalidArgument, "manifest_state", "unknown state")
	}
	expected, err := m.CanonicalDigest()
	if err != nil {
		return err
	}
	if m.Digest == "" || m.Digest != expected {
		return NewError(ErrRevisionConflict, "manifest_digest", "canonical digest mismatch")
	}
	return nil
}

func (m CheckpointManifestFactV2) CanonicalDigest() (string, error) {
	copy := m.Clone()
	copy.Digest = ""
	var err error
	if copy.ContextFrameRefs, err = normalizeExactRefsV2(copy.ContextFrameRefs, "context_frame_refs"); err != nil {
		return "", err
	}
	if copy.MemoryRefs, err = normalizeExactRefsV2(copy.MemoryRefs, "memory_refs"); err != nil {
		return "", err
	}
	if copy.KnowledgeRefs, err = normalizeExactRefsV2(copy.KnowledgeRefs, "knowledge_refs"); err != nil {
		return "", err
	}
	if copy.AttemptSettlementClosures, err = normalizeAttemptClosuresV2(copy.AttemptSettlementClosures); err != nil {
		return "", err
	}
	if copy.ParticipantClosures, err = normalizeParticipantClosuresV2(copy.ParticipantClosures); err != nil {
		return "", err
	}
	if copy.Diagnostics, err = normalizeManifestDiagnosticsV2(copy.Diagnostics); err != nil {
		return "", err
	}
	if copy.ResidualRefs, err = normalizeExactRefsV2(copy.ResidualRefs, "manifest_residual_refs"); err != nil {
		return "", err
	}
	return CanonicalDigest(copy)
}

func (m CheckpointManifestFactV2) Clone() CheckpointManifestFactV2 {
	result := m
	result.ContextFrameRefs = append([]ExactFactRefV2{}, m.ContextFrameRefs...)
	result.MemoryRefs = append([]ExactFactRefV2{}, m.MemoryRefs...)
	result.KnowledgeRefs = append([]ExactFactRefV2{}, m.KnowledgeRefs...)
	result.AttemptSettlementClosures = make([]AttemptSettlementClosureV2, len(m.AttemptSettlementClosures))
	for i := range m.AttemptSettlementClosures {
		result.AttemptSettlementClosures[i] = m.AttemptSettlementClosures[i].clone()
	}
	result.ParticipantClosures = make([]ParticipantClosureRefV2, len(m.ParticipantClosures))
	for i := range m.ParticipantClosures {
		result.ParticipantClosures[i] = m.ParticipantClosures[i].clone()
	}
	result.Diagnostics = make([]CheckpointManifestDiagnosticV2, len(m.Diagnostics))
	for i := range m.Diagnostics {
		result.Diagnostics[i] = m.Diagnostics[i].clone()
	}
	result.ResidualRefs = append([]ExactFactRefV2{}, m.ResidualRefs...)
	return result
}

func (m CheckpointManifestFactV2) Ref() CheckpointManifestRefV2 {
	return CheckpointManifestRefV2(ExactFactRefV2{
		ContractVersion: m.ContractVersion, SchemaRef: m.SchemaRef, Owner: m.Owner,
		TenantID: m.Scope.TenantID,
		ID:       m.ManifestID, Revision: m.Revision, Digest: m.Digest,
		ScopeDigest: m.Scope.ExecutionScopeDigest,
	})
}

func AdvanceCheckpointManifestStateV2(current, next ManifestState) error {
	if current == ManifestCollecting {
		switch next {
		case ManifestCollecting, ManifestVerifiedCandidate, ManifestDiagnosticPartial, ManifestDiagnosticIndeterminate, ManifestRejected:
			return nil
		}
	}
	return NewError(ErrRevisionConflict, "manifest_state", "terminal manifest fact cannot transition")
}

type CheckpointManifestSealFactV2 struct {
	ContractVersion              string                    `json:"contract_version"`
	SchemaRef                    string                    `json:"schema_ref"`
	SealID                       string                    `json:"seal_id"`
	Revision                     uint64                    `json:"revision"`
	Digest                       string                    `json:"digest"`
	Owner                        OwnerBinding              `json:"owner"`
	TenantID                     string                    `json:"tenant_id"`
	ScopeDigest                  string                    `json:"scope_digest"`
	IdempotencyKey               string                    `json:"idempotency_key"`
	ManifestRef                  CheckpointManifestRefV2   `json:"manifest_ref"`
	CheckpointAttemptRef         ExactFactRefV2            `json:"checkpoint_attempt_ref"`
	BarrierRef                   ExactFactRefV2            `json:"barrier_ref"`
	EffectCutRef                 ExactFactRefV2            `json:"effect_cut_ref"`
	FrozenRefSetDigest           string                    `json:"frozen_ref_set_digest"`
	RequiredParticipantSetDigest string                    `json:"required_participant_set_digest"`
	RuntimeParticipantSetDigest  string                    `json:"runtime_participant_set_digest"`
	ParticipantClosures          []ParticipantClosureRefV2 `json:"participant_closures"`
	ContextClosureDigest         string                    `json:"context_closure_digest"`
	ArtifactClosureDigest        string                    `json:"artifact_closure_digest"`
	CreatedUnixNano              int64                     `json:"created_unix_nano"`
}

func (s CheckpointManifestSealFactV2) Validate() error {
	if s.ContractVersion != CheckpointManifestGovernanceContractV2 || s.SchemaRef != CheckpointManifestSealSchemaV2 {
		return NewError(ErrInvalidArgument, "manifest_seal", "wrong contract or schema")
	}
	if err := validateContinuityOwnerV2(s.Owner, "checkpoint_manifest_seal_fact_v2"); err != nil {
		return err
	}
	for field, value := range map[string]string{
		"seal_id": s.SealID, "tenant_id": s.TenantID, "scope_digest": s.ScopeDigest, "idempotency_key": s.IdempotencyKey,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if s.Revision != 1 || s.CreatedUnixNano <= 0 {
		return NewError(ErrInvalidArgument, "manifest_seal", "revision must be 1 and creation time must be positive")
	}
	if err := s.ManifestRef.Validate(); err != nil {
		return err
	}
	for _, ref := range []ExactFactRefV2{s.CheckpointAttemptRef, s.BarrierRef, s.EffectCutRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	if err := ValidateDigest("frozen_ref_set_digest", s.FrozenRefSetDigest); err != nil {
		return err
	}
	if err := ValidateDigest("required_participant_set_digest", s.RequiredParticipantSetDigest); err != nil {
		return err
	}
	if err := ValidateDigest("runtime_participant_set_digest", s.RuntimeParticipantSetDigest); err != nil {
		return err
	}
	if err := ValidateDigest("context_closure_digest", s.ContextClosureDigest); err != nil {
		return err
	}
	if err := ValidateDigest("artifact_closure_digest", s.ArtifactClosureDigest); err != nil {
		return err
	}
	if len(s.ParticipantClosures) == 0 {
		return NewError(ErrInvalidArgument, "participant_closures", "seal requires participant closures")
	}
	participants, err := normalizeParticipantClosuresV2(s.ParticipantClosures)
	if err != nil {
		return err
	}
	for _, participant := range participants {
		if participant.Required && !participant.completeForSeal() {
			return NewError(ErrCheckpointPartial, "participant_closure", "required participant closure is incomplete")
		}
		if len(participant.ResidualRefs) != 0 {
			return NewError(ErrCheckpointIndeterminate, "participant_residual_refs", "sealed participant closure cannot contain residuals")
		}
	}
	if err := ValidateCheckpointManifestSealScopeClosureV2(s); err != nil {
		return err
	}
	requiredDigest, err := RequiredParticipantSetDigestV2(participants)
	if err != nil {
		return err
	}
	if s.RequiredParticipantSetDigest != requiredDigest {
		return NewError(ErrRevisionConflict, "required_participant_set_digest", "does not match sealed participant closures")
	}
	expected, err := s.CanonicalDigest()
	if err != nil {
		return err
	}
	if s.Digest == "" || s.Digest != expected {
		return NewError(ErrRevisionConflict, "manifest_seal_digest", "canonical digest mismatch")
	}
	return nil
}

func (s CheckpointManifestSealFactV2) CanonicalDigest() (string, error) {
	copy := s.Clone()
	copy.Digest = ""
	var err error
	copy.ParticipantClosures, err = normalizeParticipantClosuresV2(copy.ParticipantClosures)
	if err != nil {
		return "", err
	}
	return CanonicalDigest(copy)
}

func (s CheckpointManifestSealFactV2) Clone() CheckpointManifestSealFactV2 {
	result := s
	result.ParticipantClosures = make([]ParticipantClosureRefV2, len(s.ParticipantClosures))
	for i := range s.ParticipantClosures {
		result.ParticipantClosures[i] = s.ParticipantClosures[i].clone()
	}
	return result
}

func (s CheckpointManifestSealFactV2) Ref() CheckpointManifestSealRefV2 {
	return CheckpointManifestSealRefV2(ExactFactRefV2{
		ContractVersion: s.ContractVersion, SchemaRef: s.SchemaRef, Owner: s.Owner,
		TenantID: s.TenantID,
		ID:       s.SealID, Revision: s.Revision, Digest: s.Digest, ScopeDigest: s.ScopeDigest,
	})
}

func ParticipantClosuresDigestV2(values []ParticipantClosureRefV2) (string, error) {
	normalized, err := normalizeParticipantClosuresV2(values)
	if err != nil {
		return "", err
	}
	return CanonicalDigest(normalized)
}

// ContextClosureDigestV2 freezes the exact Context Generation and Context
// Frame set. It carries no Context payload or currentness conclusion.
func ContextClosureDigestV2(manifest CheckpointManifestFactV2) (string, error) {
	frames, err := normalizeExactRefsV2(manifest.ContextFrameRefs, "context_frame_refs")
	if err != nil {
		return "", err
	}
	if err := manifest.ContextGenerationRef.Validate(); err != nil {
		return "", err
	}
	return CanonicalDigest(struct {
		Generation ExactFactRefV2   `json:"context_generation_ref"`
		Frames     []ExactFactRefV2 `json:"context_frame_refs"`
	}{Generation: manifest.ContextGenerationRef, Frames: frames})
}

// ArtifactClosureDigestV2 freezes only exact Data Plane artifact references:
// Memory, Knowledge, Participant Snapshot and coverage. Participant governance
// identity remains separately bound by ParticipantClosuresDigestV2.
func ArtifactClosureDigestV2(manifest CheckpointManifestFactV2) (string, error) {
	refs := append([]ExactFactRefV2{}, manifest.MemoryRefs...)
	refs = append(refs, manifest.KnowledgeRefs...)
	for _, participant := range manifest.ParticipantClosures {
		if participant.SnapshotRef != nil {
			refs = append(refs, *participant.SnapshotRef)
		}
		if participant.CoverageRef != nil {
			refs = append(refs, *participant.CoverageRef)
		}
	}
	return ExactRefSetDigestV2(refs)
}

func ExactRefSetDigestV2(values []ExactFactRefV2) (string, error) {
	normalized, err := normalizeExactRefsV2(values, "exact_ref_set")
	if err != nil {
		return "", err
	}
	return CanonicalDigest(normalized)
}

func AggregateCheckpointManifestResidualRefsV2(manifest CheckpointManifestFactV2) ([]ExactFactRefV2, error) {
	all := append([]ExactFactRefV2{}, manifest.ResidualRefs...)
	for _, closure := range manifest.AttemptSettlementClosures {
		all = append(all, closure.ResidualRefs...)
	}
	for _, participant := range manifest.ParticipantClosures {
		all = append(all, participant.ResidualRefs...)
	}
	for _, diagnostic := range manifest.Diagnostics {
		all = append(all, diagnostic.ResidualRefs...)
	}
	unique := make(map[ExactFactIdentityKeyV2]ExactFactRefV2, len(all))
	for _, ref := range all {
		if err := ref.Validate(); err != nil {
			return nil, err
		}
		unique[ref.IdentityKey()] = ref
	}
	result := make([]ExactFactRefV2, 0, len(unique))
	for _, ref := range unique {
		result = append(result, ref)
	}
	sort.Slice(result, func(i, j int) bool {
		return compareExactFactIdentityKeyV2(result[i].IdentityKey(), result[j].IdentityKey()) < 0
	})
	return result, nil
}

func ValidateCheckpointManifestScopeClosureV2(manifest CheckpointManifestFactV2) error {
	tenantID := manifest.Scope.TenantID
	scopeDigest := manifest.Scope.ExecutionScopeDigest
	refs := []ExactFactRefV2{
		manifest.CheckpointAttemptRef, manifest.BarrierRef, manifest.EffectCutRef,
		manifest.TimelineCut.EvidenceRecordRef, manifest.ContextGenerationRef,
	}
	refs = append(refs, manifest.ContextFrameRefs...)
	refs = append(refs, manifest.MemoryRefs...)
	refs = append(refs, manifest.KnowledgeRefs...)
	for _, closure := range manifest.AttemptSettlementClosures {
		refs = append(refs, closure.AttemptRef)
		if closure.SettlementRef != nil {
			refs = append(refs, *closure.SettlementRef)
		}
		if closure.InspectionRef != nil {
			refs = append(refs, *closure.InspectionRef)
		}
		refs = append(refs, closure.ResidualRefs...)
	}
	for _, participant := range manifest.ParticipantClosures {
		refs = append(refs, participant.RuntimeClosureRef)
		refs = append(refs, participant.ParticipantFactRef)
		if participant.SnapshotRef != nil {
			refs = append(refs, *participant.SnapshotRef)
		}
		if participant.CoverageRef != nil {
			refs = append(refs, *participant.CoverageRef)
		}
		refs = append(refs, participant.EvidenceRefs...)
		refs = append(refs, participant.ResidualRefs...)
	}
	for _, diagnostic := range manifest.Diagnostics {
		refs = append(refs, diagnostic.DiagnosticRef)
		if diagnostic.SubjectRef != nil {
			refs = append(refs, *diagnostic.SubjectRef)
		}
		if diagnostic.InspectionRef != nil {
			refs = append(refs, *diagnostic.InspectionRef)
		}
		refs = append(refs, diagnostic.ResidualRefs...)
	}
	refs = append(refs, manifest.ResidualRefs...)
	return validateExactRefScopeSetV2(tenantID, scopeDigest, refs)
}

func ValidateCheckpointManifestSealScopeClosureV2(seal CheckpointManifestSealFactV2) error {
	refs := []ExactFactRefV2{
		seal.ManifestRef.Exact(), seal.CheckpointAttemptRef, seal.BarrierRef, seal.EffectCutRef,
	}
	for _, participant := range seal.ParticipantClosures {
		refs = append(refs, participant.RuntimeClosureRef)
		refs = append(refs, participant.ParticipantFactRef)
		if participant.SnapshotRef != nil {
			refs = append(refs, *participant.SnapshotRef)
		}
		if participant.CoverageRef != nil {
			refs = append(refs, *participant.CoverageRef)
		}
		refs = append(refs, participant.EvidenceRefs...)
		refs = append(refs, participant.ResidualRefs...)
	}
	return validateExactRefScopeSetV2(seal.TenantID, seal.ScopeDigest, refs)
}

func ValidateCheckpointManifestSealBindingV2(manifest CheckpointManifestFactV2, seal CheckpointManifestSealFactV2) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	if err := seal.Validate(); err != nil {
		return err
	}
	if manifest.State != ManifestVerifiedCandidate {
		return NewError(ErrCheckpointPartial, "manifest_state", "only verified_candidate may be sealed")
	}
	if !manifest.Ref().Exact().Equal(seal.ManifestRef.Exact()) ||
		!manifest.CheckpointAttemptRef.Equal(seal.CheckpointAttemptRef) ||
		!manifest.BarrierRef.Equal(seal.BarrierRef) ||
		!manifest.EffectCutRef.Equal(seal.EffectCutRef) ||
		manifest.Scope.TenantID != seal.TenantID ||
		manifest.Scope.ExecutionScopeDigest != seal.ScopeDigest ||
		manifest.FrozenRefSetDigest != seal.FrozenRefSetDigest ||
		manifest.RuntimeParticipantSetDigest != seal.RuntimeParticipantSetDigest ||
		manifest.RequiredParticipantSetDigest != seal.RequiredParticipantSetDigest {
		return NewError(ErrRevisionConflict, "manifest_seal_binding", "seal changed an exact manifest binding")
	}
	manifestClosures, err := ParticipantClosuresDigestV2(manifest.ParticipantClosures)
	if err != nil {
		return err
	}
	sealClosures, err := ParticipantClosuresDigestV2(seal.ParticipantClosures)
	if err != nil {
		return err
	}
	if manifestClosures != sealClosures {
		return NewError(ErrRevisionConflict, "participant_closures", "seal changed participant closure set")
	}
	contextDigest, err := ContextClosureDigestV2(manifest)
	if err != nil {
		return err
	}
	artifactDigest, err := ArtifactClosureDigestV2(manifest)
	if err != nil {
		return err
	}
	if seal.ContextClosureDigest != contextDigest || seal.ArtifactClosureDigest != artifactDigest {
		return NewError(ErrRevisionConflict, "manifest_closure_digest", "seal changed context or artifact closure")
	}
	if !sameContinuityOwnerBindingV2(manifest.Owner, seal.Owner) {
		return NewError(ErrRevisionConflict, "owner_binding", "seal owner binding drifted from manifest owner")
	}
	return nil
}

func validateExactRefScopeSetV2(tenantID, scopeDigest string, refs []ExactFactRefV2) error {
	for _, ref := range refs {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.TenantID != tenantID || ref.ScopeDigest != scopeDigest {
			return NewError(ErrRevisionConflict, "exact_ref_scope", "cross-tenant or cross-scope reference splice")
		}
	}
	return nil
}

func sameContinuityOwnerBindingV2(manifest, seal OwnerBinding) bool {
	return manifest.BindingSetID == seal.BindingSetID &&
		manifest.BindingRevision == seal.BindingRevision &&
		manifest.ComponentID == seal.ComponentID &&
		manifest.ManifestDigest == seal.ManifestDigest &&
		manifest.ArtifactDigest == seal.ArtifactDigest &&
		manifest.Capability == seal.Capability
}

func RequiredParticipantSetDigestV2(values []ParticipantClosureRefV2) (string, error) {
	normalized, err := normalizeParticipantClosuresV2(values)
	if err != nil {
		return "", err
	}
	required := make([]string, 0, len(normalized))
	for _, participant := range normalized {
		if participant.Required {
			required = append(required, participant.ParticipantID)
		}
	}
	if len(required) == 0 {
		return "", NewError(ErrInvalidArgument, "required_participant_set", "at least one required participant is needed")
	}
	return CanonicalDigest(required)
}

func FrozenRefSetDigestV2(manifest CheckpointManifestFactV2) (string, error) {
	frames, err := normalizeExactRefsV2(manifest.ContextFrameRefs, "context_frame_refs")
	if err != nil {
		return "", err
	}
	attempts, err := normalizeAttemptClosuresV2(manifest.AttemptSettlementClosures)
	if err != nil {
		return "", err
	}
	memoryRefs, err := normalizeExactRefsV2(manifest.MemoryRefs, "memory_refs")
	if err != nil {
		return "", err
	}
	knowledgeRefs, err := normalizeExactRefsV2(manifest.KnowledgeRefs, "knowledge_refs")
	if err != nil {
		return "", err
	}
	participants, err := normalizeParticipantClosuresV2(manifest.ParticipantClosures)
	if err != nil {
		return "", err
	}
	diagnostics, err := normalizeManifestDiagnosticsV2(manifest.Diagnostics)
	if err != nil {
		return "", err
	}
	residuals, err := normalizeExactRefsV2(manifest.ResidualRefs, "manifest_residual_refs")
	if err != nil {
		return "", err
	}
	closure := struct {
		CheckpointAttemptRef        ExactFactRefV2                   `json:"checkpoint_attempt_ref"`
		BarrierRef                  ExactFactRefV2                   `json:"barrier_ref"`
		EffectCutRef                ExactFactRefV2                   `json:"effect_cut_ref"`
		TimelineCut                 TimelineCutV2                    `json:"timeline_cut"`
		ContextGenerationRef        ExactFactRefV2                   `json:"context_generation_ref"`
		ContextFrameRefs            []ExactFactRefV2                 `json:"context_frame_refs"`
		AttemptSettlementClosures   []AttemptSettlementClosureV2     `json:"attempt_settlement_closures"`
		MemoryRefs                  []ExactFactRefV2                 `json:"memory_refs"`
		KnowledgeRefs               []ExactFactRefV2                 `json:"knowledge_refs"`
		ParticipantClosures         []ParticipantClosureRefV2        `json:"participant_closures"`
		RuntimeParticipantSetDigest string                           `json:"runtime_participant_set_digest"`
		Diagnostics                 []CheckpointManifestDiagnosticV2 `json:"diagnostics"`
		ResidualRefs                []ExactFactRefV2                 `json:"residual_refs"`
	}{
		CheckpointAttemptRef: manifest.CheckpointAttemptRef, BarrierRef: manifest.BarrierRef,
		EffectCutRef: manifest.EffectCutRef, TimelineCut: manifest.TimelineCut,
		ContextGenerationRef: manifest.ContextGenerationRef, ContextFrameRefs: frames,
		AttemptSettlementClosures: attempts, MemoryRefs: memoryRefs, KnowledgeRefs: knowledgeRefs,
		ParticipantClosures: participants, RuntimeParticipantSetDigest: manifest.RuntimeParticipantSetDigest, Diagnostics: diagnostics, ResidualRefs: residuals,
	}
	return CanonicalDigest(closure)
}

func validateContinuityOwnerV2(owner OwnerBinding, factKind string) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	if owner.ComponentID != ContinuityComponentID || owner.Capability != CheckpointManifestCapabilityV2 || owner.FactKind != factKind {
		return NewError(ErrInvalidArgument, "owner_binding", "wrong Continuity owner, capability, or fact kind")
	}
	return nil
}

func normalizeExactRefsV2(values []ExactFactRefV2, field string) ([]ExactFactRefV2, error) {
	if len(values) > MaxReferenceCount {
		return nil, NewError(ErrInvalidArgument, field, "too many references")
	}
	result := append([]ExactFactRefV2{}, values...)
	sort.Slice(result, func(i, j int) bool {
		return compareExactFactIdentityKeyV2(result[i].IdentityKey(), result[j].IdentityKey()) < 0
	})
	for i, ref := range result {
		if err := ref.Validate(); err != nil {
			return nil, err
		}
		if i > 0 && result[i-1].IdentityKey() == ref.IdentityKey() {
			return nil, NewError(ErrInvalidArgument, field, "duplicate exact reference")
		}
	}
	return result, nil
}

func normalizeAttemptClosuresV2(values []AttemptSettlementClosureV2) ([]AttemptSettlementClosureV2, error) {
	if len(values) > MaxReferenceCount {
		return nil, NewError(ErrInvalidArgument, "attempt_settlement_closures", "too many closures")
	}
	result := make([]AttemptSettlementClosureV2, len(values))
	for i := range values {
		result[i] = values[i].clone()
	}
	sort.Slice(result, func(i, j int) bool {
		return compareExactFactIdentityKeyV2(result[i].AttemptRef.IdentityKey(), result[j].AttemptRef.IdentityKey()) < 0
	})
	for i := range result {
		if err := result[i].Validate(); err != nil {
			return nil, err
		}
		var err error
		result[i].ResidualRefs, err = normalizeExactRefsV2(result[i].ResidualRefs, "attempt_residual_refs")
		if err != nil {
			return nil, err
		}
		if i > 0 && result[i-1].AttemptRef.IdentityKey() == result[i].AttemptRef.IdentityKey() {
			return nil, NewError(ErrInvalidArgument, "attempt_settlement_closures", "duplicate attempt ref")
		}
	}
	return result, nil
}

func normalizeParticipantClosuresV2(values []ParticipantClosureRefV2) ([]ParticipantClosureRefV2, error) {
	if len(values) > MaxReferenceCount {
		return nil, NewError(ErrInvalidArgument, "participant_closures", "too many closures")
	}
	result := make([]ParticipantClosureRefV2, len(values))
	for i := range values {
		result[i] = values[i].clone()
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ParticipantID < result[j].ParticipantID })
	for i := range result {
		if err := result[i].Validate(); err != nil {
			return nil, err
		}
		var err error
		result[i].EvidenceRefs, err = normalizeExactRefsV2(result[i].EvidenceRefs, "participant_evidence_refs")
		if err != nil {
			return nil, err
		}
		result[i].ResidualRefs, err = normalizeExactRefsV2(result[i].ResidualRefs, "participant_residual_refs")
		if err != nil {
			return nil, err
		}
		if i > 0 && result[i-1].ParticipantID == result[i].ParticipantID {
			return nil, NewError(ErrInvalidArgument, "participant_closures", "duplicate participant id")
		}
	}
	return result, nil
}

func normalizeManifestDiagnosticsV2(values []CheckpointManifestDiagnosticV2) ([]CheckpointManifestDiagnosticV2, error) {
	if len(values) > MaxReferenceCount {
		return nil, NewError(ErrInvalidArgument, "diagnostics", "too many diagnostics")
	}
	result := make([]CheckpointManifestDiagnosticV2, len(values))
	for i := range values {
		result[i] = values[i].clone()
	}
	sort.Slice(result, func(i, j int) bool { return result[i].DiagnosticID < result[j].DiagnosticID })
	for i := range result {
		if err := result[i].Validate(); err != nil {
			return nil, err
		}
		var err error
		result[i].ResidualRefs, err = normalizeExactRefsV2(result[i].ResidualRefs, "diagnostic_residual_refs")
		if err != nil {
			return nil, err
		}
		if i > 0 && result[i-1].DiagnosticID == result[i].DiagnosticID {
			return nil, NewError(ErrInvalidArgument, "diagnostics", "duplicate diagnostic id")
		}
	}
	return result, nil
}

func cloneExactRefV2(value *ExactFactRefV2) *ExactFactRefV2 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func compareExactFactIdentityKeyV2(left, right ExactFactIdentityKeyV2) int {
	comparisons := [...]int{
		cmp.Compare(left.TenantID, right.TenantID),
		cmp.Compare(left.ScopeDigest, right.ScopeDigest),
		cmp.Compare(left.ContractVersion, right.ContractVersion),
		cmp.Compare(left.SchemaRef, right.SchemaRef),
		cmp.Compare(left.Owner.BindingSetID, right.Owner.BindingSetID),
		cmp.Compare(left.Owner.BindingRevision, right.Owner.BindingRevision),
		cmp.Compare(left.Owner.ComponentID, right.Owner.ComponentID),
		cmp.Compare(left.Owner.ManifestDigest, right.Owner.ManifestDigest),
		cmp.Compare(left.Owner.ArtifactDigest, right.Owner.ArtifactDigest),
		cmp.Compare(left.Owner.Capability, right.Owner.Capability),
		cmp.Compare(left.Owner.FactKind, right.Owner.FactKind),
		cmp.Compare(left.ID, right.ID),
		cmp.Compare(left.Revision, right.Revision),
		cmp.Compare(left.Digest, right.Digest),
	}
	for _, comparison := range comparisons {
		if comparison != 0 {
			return comparison
		}
	}
	return 0
}
