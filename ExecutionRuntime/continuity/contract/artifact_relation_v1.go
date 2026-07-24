package contract

import "time"

const (
	ArtifactRelationContractV1   = "praxis.continuity/artifact-relation-governance/v1"
	ArtifactRelationFactSchemaV1 = "praxis.continuity/artifact-relation-fact/v1"
	ArtifactRelationCapabilityV1 = "artifact-relation-governance-v1"
)

type ArtifactRelationKindV1 string

const (
	ArtifactRelationContextFrame    ArtifactRelationKindV1 = "context_frame"
	ArtifactRelationWorkspaceChange ArtifactRelationKindV1 = "workspace_change"
	ArtifactRelationReview          ArtifactRelationKindV1 = "review"
	ArtifactRelationToolResult      ArtifactRelationKindV1 = "tool_result"
	ArtifactRelationEffect          ArtifactRelationKindV1 = "effect"
	ArtifactRelationCheckpoint      ArtifactRelationKindV1 = "checkpoint"
)

func (k ArtifactRelationKindV1) Validate() error {
	switch k {
	case ArtifactRelationContextFrame, ArtifactRelationWorkspaceChange,
		ArtifactRelationReview, ArtifactRelationToolResult,
		ArtifactRelationEffect, ArtifactRelationCheckpoint:
		return nil
	default:
		return NewError(ErrInvalidArgument, "artifact_relation_kind", "unsupported relation kind")
	}
}

// ArtifactRefV1 is an owner-sealed historical descriptor. Continuity indexes
// it but does not own the artifact body, revision semantics, or currentness.
type ArtifactRefV1 struct {
	ArtifactFactRef         ExactFactRefV2  `json:"artifact_fact_ref"`
	StorageRef              string          `json:"storage_ref"`
	StorageDigest           string          `json:"storage_digest"`
	ParentRevisionRef       *ExactFactRefV2 `json:"parent_revision_ref,omitempty"`
	OriginEvidenceRecordRef string          `json:"origin_evidence_record_ref"`
	OriginEvidenceDigest    string          `json:"origin_evidence_digest"`
}

func (r ArtifactRefV1) Validate() error {
	if err := r.ArtifactFactRef.Validate(); err != nil {
		return err
	}
	if err := ValidateToken("storage_ref", r.StorageRef); err != nil {
		return err
	}
	if err := ValidateDigest("storage_digest", r.StorageDigest); err != nil {
		return err
	}
	if err := ValidateToken("origin_evidence_record_ref", r.OriginEvidenceRecordRef); err != nil {
		return err
	}
	if err := ValidateDigest("origin_evidence_digest", r.OriginEvidenceDigest); err != nil {
		return err
	}
	if r.ParentRevisionRef == nil {
		return nil
	}
	parent := *r.ParentRevisionRef
	if err := parent.Validate(); err != nil {
		return err
	}
	current := r.ArtifactFactRef
	if parent.TenantID != current.TenantID || parent.ID != current.ID ||
		parent.ContractVersion != current.ContractVersion || parent.SchemaRef != current.SchemaRef ||
		parent.Owner != current.Owner || parent.Revision >= current.Revision {
		return NewError(ErrRevisionConflict, "parent_revision_ref", "parent must be a lower revision of the same owner artifact")
	}
	return nil
}

func (r ArtifactRefV1) Clone() ArtifactRefV1 {
	result := r
	result.ParentRevisionRef = cloneExactRefV2(r.ParentRevisionRef)
	return result
}

// ArtifactRelationSourceProjectionV1 is returned by a typed artifact owner
// reader. The request cannot carry these owner-sealed fields as trusted data.
type ArtifactRelationSourceProjectionV1 struct {
	SourceProjectionRef  ExactFactRefV2         `json:"source_projection_ref"`
	Artifact             ArtifactRefV1          `json:"artifact"`
	RelatedFactRef       ExactFactRefV2         `json:"related_fact_ref"`
	Kind                 ArtifactRelationKindV1 `json:"kind"`
	EvidenceRecordRef    string                 `json:"evidence_record_ref"`
	EvidenceRecordDigest string                 `json:"evidence_record_digest"`
	ExecutionScopeDigest string                 `json:"execution_scope_digest"`
}

func (p ArtifactRelationSourceProjectionV1) Validate() error {
	if err := p.SourceProjectionRef.Validate(); err != nil {
		return err
	}
	if err := p.Artifact.Validate(); err != nil {
		return err
	}
	if err := p.RelatedFactRef.Validate(); err != nil {
		return err
	}
	if err := p.Kind.Validate(); err != nil {
		return err
	}
	if err := ValidateToken("evidence_record_ref", p.EvidenceRecordRef); err != nil {
		return err
	}
	if err := ValidateDigest("evidence_record_digest", p.EvidenceRecordDigest); err != nil {
		return err
	}
	if err := ValidateDigest("execution_scope_digest", p.ExecutionScopeDigest); err != nil {
		return err
	}
	tenant := p.SourceProjectionRef.TenantID
	if p.Artifact.ArtifactFactRef.TenantID != tenant || p.RelatedFactRef.TenantID != tenant {
		return NewError(ErrRevisionConflict, "artifact_relation_tenant", "source, artifact, and related fact must share a tenant")
	}
	sourceOwner, artifactOwner := p.SourceProjectionRef.Owner, p.Artifact.ArtifactFactRef.Owner
	if sourceOwner.BindingSetID != artifactOwner.BindingSetID || sourceOwner.BindingRevision != artifactOwner.BindingRevision ||
		sourceOwner.ComponentID != artifactOwner.ComponentID || sourceOwner.ManifestDigest != artifactOwner.ManifestDigest ||
		sourceOwner.ArtifactDigest != artifactOwner.ArtifactDigest {
		return NewError(ErrRevisionConflict, "artifact_source_owner", "source projection is not sealed by the artifact owner binding")
	}
	if p.SourceProjectionRef.ScopeDigest != p.ExecutionScopeDigest {
		return NewError(ErrRevisionConflict, "source_projection_scope", "source projection belongs to another execution scope")
	}
	if p.EvidenceRecordRef != p.Artifact.OriginEvidenceRecordRef || p.EvidenceRecordDigest != p.Artifact.OriginEvidenceDigest {
		return NewError(ErrRevisionConflict, "artifact_evidence", "source projection changed artifact origin evidence")
	}
	return nil
}

func (p ArtifactRelationSourceProjectionV1) Clone() ArtifactRelationSourceProjectionV1 {
	result := p
	result.Artifact = p.Artifact.Clone()
	return result
}

type ArtifactRelationFactV1 struct {
	ContractVersion  string                             `json:"contract_version"`
	SchemaRef        string                             `json:"schema_ref"`
	RelationID       string                             `json:"relation_id"`
	Revision         uint64                             `json:"revision"`
	Digest           string                             `json:"digest"`
	IdempotencyKey   string                             `json:"idempotency_key"`
	Scope            Scope                              `json:"scope"`
	Owner            OwnerBinding                       `json:"owner"`
	SourceProjection ArtifactRelationSourceProjectionV1 `json:"source_projection"`
	CreatedUnixNano  int64                              `json:"created_unix_nano"`
}

func (f ArtifactRelationFactV1) CanonicalDigest() (string, error) {
	copy := f.Clone()
	copy.Digest = ""
	return CanonicalDigest(copy)
}

func (f ArtifactRelationFactV1) Validate() error {
	if f.ContractVersion != ArtifactRelationContractV1 || f.SchemaRef != ArtifactRelationFactSchemaV1 {
		return NewError(ErrInvalidArgument, "artifact_relation_contract", "unsupported contract or schema")
	}
	if err := ValidateToken("relation_id", f.RelationID); err != nil {
		return err
	}
	if err := ValidateToken("idempotency_key", f.IdempotencyKey); err != nil {
		return err
	}
	if f.Revision != 1 || f.CreatedUnixNano <= 0 {
		return NewError(ErrInvalidArgument, "artifact_relation_fact", "immutable relation requires revision one and creation time")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	if err := validateArtifactRelationOwnerV1(f.Owner); err != nil {
		return err
	}
	if err := f.SourceProjection.Validate(); err != nil {
		return err
	}
	if f.SourceProjection.SourceProjectionRef.TenantID != f.Scope.TenantID ||
		f.SourceProjection.ExecutionScopeDigest != f.Scope.ExecutionScopeDigest {
		return NewError(ErrRevisionConflict, "artifact_relation_scope", "relation source belongs to another tenant or execution scope")
	}
	expected, err := f.CanonicalDigest()
	if err != nil {
		return err
	}
	if f.Digest == "" || f.Digest != expected {
		return NewError(ErrRevisionConflict, "artifact_relation_digest", "canonical digest mismatch")
	}
	return nil
}

func (f ArtifactRelationFactV1) Ref() ArtifactRelationRefV1 {
	return ArtifactRelationRefV1(ExactFactRefV2{
		ContractVersion: f.ContractVersion, SchemaRef: f.SchemaRef, Owner: f.Owner,
		TenantID: f.Scope.TenantID, ID: f.RelationID, Revision: f.Revision,
		Digest: f.Digest, ScopeDigest: f.Scope.ExecutionScopeDigest,
	})
}

func (f ArtifactRelationFactV1) Clone() ArtifactRelationFactV1 {
	result := f
	result.SourceProjection = f.SourceProjection.Clone()
	return result
}

type ArtifactRelationRefV1 ExactFactRefV2

func (r ArtifactRelationRefV1) Validate() error {
	value := ExactFactRefV2(r)
	if err := value.Validate(); err != nil {
		return err
	}
	if value.ContractVersion != ArtifactRelationContractV1 || value.SchemaRef != ArtifactRelationFactSchemaV1 || value.Revision != 1 {
		return NewError(ErrInvalidArgument, "artifact_relation_ref", "wrong contract, schema, or revision")
	}
	return validateArtifactRelationOwnerV1(value.Owner)
}

func (r ArtifactRelationRefV1) Exact() ExactFactRefV2 { return ExactFactRefV2(r) }

func validateArtifactRelationOwnerV1(owner OwnerBinding) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	if owner.ComponentID != ContinuityComponentID || owner.Capability != ArtifactRelationCapabilityV1 || owner.FactKind != "artifact_relation_fact_v1" {
		return NewError(ErrInvalidArgument, "owner_binding", "wrong Continuity Artifact Relation owner")
	}
	return nil
}

func ArtifactRelationSourceDigestV1(value ArtifactRelationSourceProjectionV1) (string, error) {
	copy := value.Clone()
	return CanonicalDigest(copy)
}

func NewArtifactRelationFactV1(relationID, idempotencyKey string, scope Scope, owner OwnerBinding, source ArtifactRelationSourceProjectionV1, now time.Time) (ArtifactRelationFactV1, error) {
	fact := ArtifactRelationFactV1{
		ContractVersion: ArtifactRelationContractV1, SchemaRef: ArtifactRelationFactSchemaV1,
		RelationID: relationID, Revision: 1, IdempotencyKey: idempotencyKey,
		Scope: scope, Owner: owner, SourceProjection: source.Clone(), CreatedUnixNano: now.UnixNano(),
	}
	digest, err := fact.CanonicalDigest()
	if err != nil {
		return ArtifactRelationFactV1{}, err
	}
	fact.Digest = digest
	if err := fact.Validate(); err != nil {
		return ArtifactRelationFactV1{}, err
	}
	return fact, nil
}
