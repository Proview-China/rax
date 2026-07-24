package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

// CreateArtifactRelationRequestV1 contains coordinates only. Storage, parent,
// evidence digest, and source projection fields must come from the typed owner
// reader and cannot be asserted by the caller.
type CreateArtifactRelationRequestV1 struct {
	RelationID                  string                          `json:"relation_id"`
	IdempotencyKey              string                          `json:"idempotency_key"`
	Scope                       contract.Scope                  `json:"scope"`
	ArtifactFactRef             contract.ExactFactRefV2         `json:"artifact_fact_ref"`
	RelatedFactRef              contract.ExactFactRefV2         `json:"related_fact_ref"`
	Kind                        contract.ArtifactRelationKindV1 `json:"kind"`
	EvidenceRecordRef           string                          `json:"evidence_record_ref"`
	ExpectedSourceProjectionRef *contract.ExactFactRefV2        `json:"expected_source_projection_ref,omitempty"`
}

func (r CreateArtifactRelationRequestV1) Validate() error {
	if err := contract.ValidateToken("relation_id", r.RelationID); err != nil {
		return err
	}
	if err := contract.ValidateToken("idempotency_key", r.IdempotencyKey); err != nil {
		return err
	}
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if err := r.ArtifactFactRef.Validate(); err != nil {
		return err
	}
	if err := r.RelatedFactRef.Validate(); err != nil {
		return err
	}
	if err := r.Kind.Validate(); err != nil {
		return err
	}
	if err := contract.ValidateToken("evidence_record_ref", r.EvidenceRecordRef); err != nil {
		return err
	}
	if r.ArtifactFactRef.TenantID != r.Scope.TenantID || r.RelatedFactRef.TenantID != r.Scope.TenantID {
		return contract.NewError(contract.ErrRevisionConflict, "artifact_relation_tenant", "request references another tenant")
	}
	if r.ExpectedSourceProjectionRef != nil {
		if err := r.ExpectedSourceProjectionRef.Validate(); err != nil {
			return err
		}
		if r.ExpectedSourceProjectionRef.TenantID != r.Scope.TenantID || r.ExpectedSourceProjectionRef.ScopeDigest != r.Scope.ExecutionScopeDigest {
			return contract.NewError(contract.ErrRevisionConflict, "source_projection_ref", "expected source projection belongs to another tenant or scope")
		}
	}
	return nil
}

type ArtifactRelationSourceRequestV1 struct {
	ArtifactFactRef             contract.ExactFactRefV2         `json:"artifact_fact_ref"`
	RelatedFactRef              contract.ExactFactRefV2         `json:"related_fact_ref"`
	Kind                        contract.ArtifactRelationKindV1 `json:"kind"`
	EvidenceRecordRef           string                          `json:"evidence_record_ref"`
	ExecutionScopeDigest        string                          `json:"execution_scope_digest"`
	ExpectedSourceProjectionRef *contract.ExactFactRefV2        `json:"expected_source_projection_ref,omitempty"`
}

// ArtifactRelationSourceReaderV1 is a consumer-side typed routing seam. A
// production implementation must route to the actual artifact owner; a
// generic caller-owned projection is not accepted.
type ArtifactRelationSourceReaderV1 interface {
	InspectArtifactRelationSourceV1(context.Context, ArtifactRelationSourceRequestV1) (contract.ArtifactRelationSourceProjectionV1, error)
}

type InspectArtifactRelationRequestV1 struct {
	Ref contract.ArtifactRelationRefV1 `json:"ref"`
}

type InspectArtifactRelationByIDRequestV1 struct {
	TenantID    string                `json:"tenant_id"`
	ScopeDigest string                `json:"scope_digest"`
	RelationID  string                `json:"relation_id"`
	Owner       contract.OwnerBinding `json:"owner"`
}

type ArtifactTimelineReaderV1 interface {
	InspectByEvidence(context.Context, string) (contract.TimelineEventRecord, error)
}

type ListArtifactRelationsRequestV1 struct {
	ArtifactFactRef contract.ExactFactRefV2 `json:"artifact_fact_ref"`
}

type ListRelatedArtifactRelationsRequestV1 struct {
	RelatedFactRef contract.ExactFactRefV2 `json:"related_fact_ref"`
}

type ArtifactRelationReaderV1 interface {
	InspectArtifactRelationV1(context.Context, InspectArtifactRelationRequestV1) (contract.ArtifactRelationFactV1, error)
	InspectArtifactRelationByIDV1(context.Context, InspectArtifactRelationByIDRequestV1) (contract.ArtifactRelationFactV1, error)
	ListArtifactRelationsV1(context.Context, ListArtifactRelationsRequestV1) ([]contract.ArtifactRelationFactV1, error)
	ListRelatedArtifactRelationsV1(context.Context, ListRelatedArtifactRelationsRequestV1) ([]contract.ArtifactRelationFactV1, error)
}

type ArtifactRelationGovernancePortV1 interface {
	ArtifactRelationReaderV1
	CreateArtifactRelationV1(context.Context, CreateArtifactRelationRequestV1) (contract.ArtifactRelationFactV1, bool, error)
}
