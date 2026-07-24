package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

type CreateContentDeltaRequestV1 struct {
	DeltaID                      string         `json:"delta_id"`
	IdempotencyKey               string         `json:"idempotency_key"`
	Scope                        contract.Scope `json:"scope"`
	BaseObjectID                 string         `json:"base_object_id"`
	ExpectedBaseManifestDigest   string         `json:"expected_base_manifest_digest"`
	TargetObjectID               string         `json:"target_object_id"`
	ExpectedTargetManifestDigest string         `json:"expected_target_manifest_digest"`
}

func (r CreateContentDeltaRequestV1) Validate() error {
	for field, value := range map[string]string{
		"delta_id": r.DeltaID, "idempotency_key": r.IdempotencyKey,
		"base_object_id": r.BaseObjectID, "target_object_id": r.TargetObjectID,
	} {
		if err := contract.ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := contract.ValidateDigest("expected_base_manifest_digest", r.ExpectedBaseManifestDigest); err != nil {
		return err
	}
	if err := contract.ValidateDigest("expected_target_manifest_digest", r.ExpectedTargetManifestDigest); err != nil {
		return err
	}
	if r.BaseObjectID == r.TargetObjectID {
		return contract.NewError(contract.ErrInvalidArgument, "content_delta_objects", "base and target object ids must differ")
	}
	return r.Scope.Validate()
}

func (r CreateContentDeltaRequestV1) CanonicalDigest() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return contract.CanonicalDigest(r)
}

type InspectContentDeltaRequestV1 struct {
	Ref contract.ContentDeltaRefV1 `json:"ref"`
}

type InspectContentDeltaByIDRequestV1 struct {
	TenantID    string                `json:"tenant_id"`
	ScopeDigest string                `json:"scope_digest"`
	DeltaID     string                `json:"delta_id"`
	Owner       contract.OwnerBinding `json:"owner"`
}

type ContentDeltaReaderV1 interface {
	InspectContentDeltaV1(context.Context, InspectContentDeltaRequestV1) (contract.ContentDeltaFactV1, error)
	InspectContentDeltaByIDV1(context.Context, InspectContentDeltaByIDRequestV1) (contract.ContentDeltaFactV1, error)
}

type ContentDeltaGovernancePortV1 interface {
	ContentDeltaReaderV1
	CreateContentDeltaV1(context.Context, CreateContentDeltaRequestV1) (contract.ContentDeltaFactV1, bool, error)
}
