package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

type CreateContentIntegrityAuditRequestV1 struct {
	AuditID        string                               `json:"audit_id"`
	IdempotencyKey string                               `json:"idempotency_key"`
	Scope          contract.Scope                       `json:"scope"`
	Subjects       []contract.ContentIntegritySubjectV1 `json:"subjects"`
}

func (r CreateContentIntegrityAuditRequestV1) Validate() error {
	if err := contract.ValidateToken("audit_id", r.AuditID); err != nil {
		return err
	}
	if err := contract.ValidateToken("idempotency_key", r.IdempotencyKey); err != nil {
		return err
	}
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	_, err := contract.NormalizeContentIntegritySubjectsV1(r.Subjects)
	return err
}

func (r CreateContentIntegrityAuditRequestV1) CanonicalDigest() (string, error) {
	subjects, err := contract.NormalizeContentIntegritySubjectsV1(r.Subjects)
	if err != nil {
		return "", err
	}
	copy := r
	copy.Subjects = subjects
	return contract.CanonicalDigest(copy)
}

type InspectContentIntegrityAuditRequestV1 struct {
	Ref contract.ContentIntegrityAuditRefV1 `json:"ref"`
}

type InspectContentIntegrityAuditByIDRequestV1 struct {
	TenantID    string                `json:"tenant_id"`
	ScopeDigest string                `json:"scope_digest"`
	AuditID     string                `json:"audit_id"`
	Owner       contract.OwnerBinding `json:"owner"`
}

type ContentIntegrityAuditReaderV1 interface {
	InspectContentIntegrityAuditV1(context.Context, InspectContentIntegrityAuditRequestV1) (contract.ContentIntegrityAuditFactV1, error)
	InspectContentIntegrityAuditByIDV1(context.Context, InspectContentIntegrityAuditByIDRequestV1) (contract.ContentIntegrityAuditFactV1, error)
}

type ContentIntegrityAuditGovernancePortV1 interface {
	ContentIntegrityAuditReaderV1
	CreateContentIntegrityAuditV1(context.Context, CreateContentIntegrityAuditRequestV1) (contract.ContentIntegrityAuditFactV1, bool, error)
}
