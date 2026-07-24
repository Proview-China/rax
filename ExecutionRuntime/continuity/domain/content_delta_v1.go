package domain

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type ContentDeltaControllerV1 struct {
	repository contentDeltaRepositoryV1
	metadata   ports.MetadataStore
	content    ports.ContentStore
	owner      contract.OwnerBinding
	clock      Clock
}

type contentDeltaRepositoryV1 interface {
	ports.ContentDeltaReaderV1
	CreateContentDeltaFactV1(context.Context, contract.ContentDeltaFactV1) (contract.ContentDeltaFactV1, bool, error)
}

func NewContentDeltaControllerV1(repository contentDeltaRepositoryV1, metadata ports.MetadataStore, content ports.ContentStore, owner contract.OwnerBinding, clock Clock) (*ContentDeltaControllerV1, error) {
	if nilInterfaceV1(repository) || nilInterfaceV1(metadata) || nilInterfaceV1(content) || nilInterfaceV1(clock) {
		return nil, contract.NewError(contract.ErrInvalidArgument, "content_delta_controller", "repository, metadata, content, and clock are required")
	}
	if err := owner.Validate(); err != nil || owner.ComponentID != contract.ContinuityComponentID || owner.Capability != contract.ContentDeltaCapabilityV1 || owner.FactKind != "content_delta_fact_v1" {
		return nil, contract.NewError(contract.ErrInvalidArgument, "owner_binding", "invalid Continuity Content Delta owner")
	}
	return &ContentDeltaControllerV1{repository: repository, metadata: metadata, content: content, owner: owner, clock: clock}, nil
}

func (c *ContentDeltaControllerV1) CreateContentDeltaV1(ctx context.Context, request ports.CreateContentDeltaRequestV1) (contract.ContentDeltaFactV1, bool, error) {
	if c == nil || nilInterfaceV1(c.repository) || nilInterfaceV1(c.metadata) || nilInterfaceV1(c.content) || nilInterfaceV1(c.clock) {
		return contract.ContentDeltaFactV1{}, false, contract.NewError(contract.ErrUnsupported, "content_delta_controller", "controller is not configured")
	}
	if err := request.Validate(); err != nil {
		return contract.ContentDeltaFactV1{}, false, err
	}
	requestDigest, err := request.CanonicalDigest()
	if err != nil {
		return contract.ContentDeltaFactV1{}, false, err
	}
	byID := ports.InspectContentDeltaByIDRequestV1{
		TenantID: request.Scope.TenantID, ScopeDigest: request.Scope.ExecutionScopeDigest,
		DeltaID: request.DeltaID, Owner: c.owner,
	}
	if existing, inspectErr := c.repository.InspectContentDeltaByIDV1(ctx, byID); inspectErr == nil {
		if existing.RequestDigest == requestDigest && existing.IdempotencyKey == request.IdempotencyKey {
			return existing.Clone(), true, nil
		}
		return contract.ContentDeltaFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "delta_id", "create-once Content Delta changed request")
	} else if !contract.HasCode(inspectErr, contract.ErrNotFound) {
		return contract.ContentDeltaFactV1{}, false, normalizeContentReadBoundaryErrorV1(inspectErr, "content_delta_repository")
	}

	s1, err := c.inspectSourceV1(ctx, request)
	if err != nil {
		return contract.ContentDeltaFactV1{}, false, err
	}
	s2, err := c.inspectSourceV1(ctx, request)
	if err != nil {
		return contract.ContentDeltaFactV1{}, false, err
	}
	if !sameCanonicalV1(s1, s2) {
		return contract.ContentDeltaFactV1{}, false, contract.NewError(contract.ErrIndeterminate, "content_delta_s1_s2", "base or target content changed during inspection")
	}
	fact, err := contract.NewContentDeltaFactV1(request.DeltaID, request.IdempotencyKey, requestDigest, request.Scope, c.owner, s2, c.clock.Now())
	if err != nil {
		return contract.ContentDeltaFactV1{}, false, err
	}
	stored, replay, err := c.repository.CreateContentDeltaFactV1(ctx, fact)
	if err != nil {
		return contract.ContentDeltaFactV1{}, false, normalizeContentReadBoundaryErrorV1(err, "content_delta_repository")
	}
	return stored.Clone(), replay, nil
}

func (c *ContentDeltaControllerV1) InspectContentDeltaV1(ctx context.Context, request ports.InspectContentDeltaRequestV1) (contract.ContentDeltaFactV1, error) {
	if c == nil || nilInterfaceV1(c.repository) {
		return contract.ContentDeltaFactV1{}, contract.NewError(contract.ErrUnsupported, "content_delta_reader", "reader is not configured")
	}
	return c.repository.InspectContentDeltaV1(ctx, request)
}

func (c *ContentDeltaControllerV1) InspectContentDeltaByIDV1(ctx context.Context, request ports.InspectContentDeltaByIDRequestV1) (contract.ContentDeltaFactV1, error) {
	if c == nil || nilInterfaceV1(c.repository) {
		return contract.ContentDeltaFactV1{}, contract.NewError(contract.ErrUnsupported, "content_delta_reader", "reader is not configured")
	}
	return c.repository.InspectContentDeltaByIDV1(ctx, request)
}

func (c *ContentDeltaControllerV1) inspectSourceV1(ctx context.Context, request ports.CreateContentDeltaRequestV1) (contract.ContentDeltaSourceProjectionV1, error) {
	base, baseChunks, err := c.inspectObjectV1(ctx, request.BaseObjectID, request.ExpectedBaseManifestDigest, request.Scope.ExecutionScopeDigest)
	if err != nil {
		return contract.ContentDeltaSourceProjectionV1{}, err
	}
	target, targetChunks, err := c.inspectObjectV1(ctx, request.TargetObjectID, request.ExpectedTargetManifestDigest, request.Scope.ExecutionScopeDigest)
	if err != nil {
		return contract.ContentDeltaSourceProjectionV1{}, err
	}
	projection := contract.ContentDeltaSourceProjectionV1{Base: base, BaseChunks: baseChunks, Target: target, TargetChunks: targetChunks}
	if err := projection.Validate(); err != nil {
		return contract.ContentDeltaSourceProjectionV1{}, err
	}
	return projection.Clone(), nil
}

func (c *ContentDeltaControllerV1) inspectObjectV1(ctx context.Context, objectID, expectedManifestDigest, scopeDigest string) (contract.ContentObjectRefV1, []contract.ChunkRef, error) {
	return inspectContentObjectExactV1(ctx, c.metadata, c.content, objectID, expectedManifestDigest, scopeDigest)
}

func normalizeContentReadBoundaryErrorV1(err error, field string) error {
	for _, code := range []contract.ErrorCode{
		contract.ErrInvalidArgument, contract.ErrContentDigestMismatch,
		contract.ErrCrossStoreIndeterminate, contract.ErrRevisionConflict,
		contract.ErrNotFound, contract.ErrUnsupported, contract.ErrPreconditionFailed,
		contract.ErrUnavailable, contract.ErrIndeterminate,
	} {
		if contract.HasCode(err, code) {
			return err
		}
	}
	return contract.NewError(contract.ErrIndeterminate, field, "boundary returned an unclassified result; inspect the original delta coordinates")
}

var _ ports.ContentDeltaGovernancePortV1 = (*ContentDeltaControllerV1)(nil)
