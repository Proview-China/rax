package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

type CreateHistoryDerivationCandidateRequestV1 struct {
	CandidateID                  string                                         `json:"candidate_id"`
	IdempotencyKey               string                                         `json:"idempotency_key"`
	Scope                        contract.Scope                                 `json:"scope"`
	Kind                         contract.HistoryDerivationKindV1               `json:"kind"`
	Sources                      []contract.HistoryDerivationSourceCoordinateV1 `json:"sources"`
	OutputObjectID               string                                         `json:"output_object_id"`
	ExpectedOutputManifestDigest string                                         `json:"expected_output_manifest_digest"`
}

func (r CreateHistoryDerivationCandidateRequestV1) Validate() error {
	if err := contract.ValidateToken("candidate_id", r.CandidateID); err != nil {
		return err
	}
	if err := contract.ValidateToken("idempotency_key", r.IdempotencyKey); err != nil {
		return err
	}
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if err := r.Kind.Validate(); err != nil {
		return err
	}
	if _, err := contract.NormalizeHistoryDerivationSourcesV1(r.Sources); err != nil {
		return err
	}
	if err := contract.ValidateToken("output_object_id", r.OutputObjectID); err != nil {
		return err
	}
	return contract.ValidateDigest("expected_output_manifest_digest", r.ExpectedOutputManifestDigest)
}

func (r CreateHistoryDerivationCandidateRequestV1) CanonicalDigest() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	copy := r
	copy.Sources = append([]contract.HistoryDerivationSourceCoordinateV1{}, r.Sources...)
	return contract.CanonicalDigest(copy)
}

type InspectHistoryDerivationCandidateRequestV1 struct {
	Ref contract.HistoryDerivationCandidateRefV1 `json:"ref"`
}

type InspectHistoryDerivationCandidateByIDRequestV1 struct {
	TenantID    string                `json:"tenant_id"`
	ScopeDigest string                `json:"scope_digest"`
	CandidateID string                `json:"candidate_id"`
	Owner       contract.OwnerBinding `json:"owner"`
}

type HistoryDerivationTimelineReaderV1 interface {
	InspectByEvidence(context.Context, string) (contract.TimelineEventRecord, error)
}

type HistoryDerivationCandidateReaderV1 interface {
	InspectHistoryDerivationCandidateV1(context.Context, InspectHistoryDerivationCandidateRequestV1) (contract.HistoryDerivationCandidateFactV1, error)
	InspectHistoryDerivationCandidateByIDV1(context.Context, InspectHistoryDerivationCandidateByIDRequestV1) (contract.HistoryDerivationCandidateFactV1, error)
}

type HistoryDerivationCandidateGovernancePortV1 interface {
	HistoryDerivationCandidateReaderV1
	CreateHistoryDerivationCandidateV1(context.Context, CreateHistoryDerivationCandidateRequestV1) (contract.HistoryDerivationCandidateFactV1, bool, error)
}
