package sqlite

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const reviewGovernanceProjectionDispatchAuthorityFactV3 = "dispatch-authority-fact-v3"

func loadDispatchAuthorityFactV3(ctx context.Context, source queryRower, expected ports.AuthorityBindingRefV2) (ports.DispatchAuthorityFactV3, error) {
	raw := reviewGovernanceProjectionRefV1{ID: expected.Ref, Revision: expected.Revision, Digest: expected.Digest}
	tenant, payload, rowDigest, err := loadReviewGovernanceProjectionPayloadV1(ctx, source, reviewGovernanceProjectionDispatchAuthorityFactV3, raw)
	if err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	value, err := decodeRow[ports.DispatchAuthorityFactV3](payload, rowDigest, "DispatchAuthorityFactV3")
	if err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	if value.Scope.Identity.TenantID != tenant || value.Ref != expected || value.Validate() != nil {
		return ports.DispatchAuthorityFactV3{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "dispatch authority V3 historical row drifted")
	}
	return cloneStrict(value)
}

func (s *Store) InspectCurrentAuthorityFactV3(ctx context.Context, expected ports.AuthorityBindingRefV2) (ports.DispatchAuthorityFactV3, error) {
	if err := expected.Validate(); err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	defer func() { _ = tx.Rollback() }()
	value, err := loadDispatchAuthorityFactV3(ctx, tx, expected)
	if err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	current, highest, err := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionDispatchAuthorityFactV3, value.Scope.Identity.TenantID, expected.Ref)
	if err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	want := reviewGovernanceProjectionRefV1{ID: expected.Ref, Revision: expected.Revision, Digest: expected.Digest}
	if current != want || highest != expected.Revision {
		return ports.DispatchAuthorityFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "dispatch authority V3 current full Ref drifted")
	}
	return cloneStrict(value)
}
func (s *Store) InspectHistoricalAuthorityFactV3(ctx context.Context, expected ports.AuthorityBindingRefV2) (ports.DispatchAuthorityFactV3, error) {
	if err := contextError(ctx, "Inspect historical dispatch authority V3"); err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	return loadDispatchAuthorityFactV3(ctx, s.db, expected)
}

func (s *Store) CommitAuthorityFactV3(ctx context.Context, request ports.DispatchAuthorityFactPublishRequestV3) (ports.DispatchAuthorityFactPublishReceiptV3, error) {
	if err := request.Validate(); err != nil {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, err
	}
	value := request.Value.Clone()
	raw := reviewGovernanceProjectionRefV1{ID: value.Ref.Ref, Revision: value.Ref.Revision, Digest: value.Ref.Digest}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, loadErr := loadDispatchAuthorityFactV3(ctx, tx, value.Ref); loadErr == nil {
		if !reflect.DeepEqual(existing, value) {
			return ports.DispatchAuthorityFactPublishReceiptV3{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "dispatch authority V3 exact Ref binds different content")
		}
		return ports.DispatchAuthorityFactPublishReceiptV3{Ref: value.Ref, Created: false}, nil
	} else if !core.HasCategory(loadErr, core.ErrorNotFound) {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, loadErr
	}
	tenant := value.Scope.Identity.TenantID
	current, highest, currentErr := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionDispatchAuthorityFactV3, tenant, value.Ref.Ref)
	if request.Previous == nil {
		if !core.HasCategory(currentErr, core.ErrorNotFound) || value.Ref.Revision != 1 {
			return ports.DispatchAuthorityFactPublishReceiptV3{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "dispatch authority V3 initial publish lost create-once precondition")
		}
	} else {
		if currentErr != nil {
			return ports.DispatchAuthorityFactPublishReceiptV3{}, currentErr
		}
		previousRaw := reviewGovernanceProjectionRefV1{ID: request.Previous.Ref, Revision: request.Previous.Revision, Digest: request.Previous.Digest}
		if current != previousRaw || highest != request.Previous.Revision || raw.ID != previousRaw.ID || raw.Revision != previousRaw.Revision+1 {
			return ports.DispatchAuthorityFactPublishReceiptV3{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "dispatch authority V3 full-ref CAS drifted")
		}
		prior, loadErr := loadDispatchAuthorityFactV3(ctx, tx, *request.Previous)
		if loadErr != nil {
			return ports.DispatchAuthorityFactPublishReceiptV3{}, loadErr
		}
		if !ports.SameDispatchAuthorityStableIdentityV3(prior, value) || value.CheckedUnixNano < prior.CheckedUnixNano || prior.State != ports.AuthorityFactActive {
			return ports.DispatchAuthorityFactPublishReceiptV3{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "dispatch authority V3 identity, state or sealed clock regressed")
		}
	}
	if s.consumeStageFailure() {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected dispatch authority V3 staged failure")
	}
	if err := insertReviewGovernanceProjectionV1(ctx, tx, reviewGovernanceProjectionDispatchAuthorityFactV3, "DispatchAuthorityFactV3", tenant, raw, value); err != nil {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, err
	}
	if request.Previous == nil {
		err = insertReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionDispatchAuthorityFactV3, tenant, raw)
	} else {
		err = advanceReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionDispatchAuthorityFactV3, tenant, reviewGovernanceProjectionRefV1{ID: request.Previous.Ref, Revision: request.Previous.Revision, Digest: request.Previous.Digest}, raw)
	}
	if err != nil {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, err
	}
	if s.consumeLostReply() {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "dispatch authority V3 publish reply was lost")
	}
	return ports.DispatchAuthorityFactPublishReceiptV3{Ref: value.Ref, Created: true}, nil
}

var _ control.DispatchAuthorityCurrentFactPortV3 = (*Store)(nil)
