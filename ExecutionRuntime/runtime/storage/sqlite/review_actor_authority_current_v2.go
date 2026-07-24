package sqlite

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"reflect"
)

const reviewGovernanceProjectionReviewActorAuthorityV2 = "review-actor-authority-v2"

func loadReviewActorAuthorityProjectionV2(ctx context.Context, source queryRower, ref reviewGovernanceProjectionRefV1) (ports.ReviewActorAuthorityCurrentProjectionV2, error) {
	tenant, payload, rowDigest, err := loadReviewGovernanceProjectionPayloadV1(ctx, source, reviewGovernanceProjectionReviewActorAuthorityV2, ref)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	value, err := decodeRow[ports.ReviewActorAuthorityCurrentProjectionV2](payload, rowDigest, "ReviewActorAuthorityCurrentProjectionV2")
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	if value.Subject.Target.TenantID != tenant || value.Ref.ID != ref.ID || value.Ref.Revision != ref.Revision || value.Ref.Digest != ref.Digest || value.Validate() != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review actor authority V2 historical row drifted")
	}
	return cloneStrict(value)
}
func (s *Store) ResolveActorAuthorityV2(ctx context.Context, subject ports.ReviewActorAuthorityCurrentSubjectV2) (ports.ReviewActorAuthorityCurrentProjectionRefV2, error) {
	if err := subject.Validate(); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, err
	}
	id, err := ports.DeriveReviewActorAuthorityCurrentProjectionIDV2(subject)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, highest, err := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionReviewActorAuthorityV2, subject.Target.TenantID, id)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, err
	}
	if highest != current.Revision {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review actor authority V2 current/highest drifted")
	}
	value, err := loadReviewActorAuthorityProjectionV2(ctx, tx, current)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, err
	}
	if value.Subject != subject {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review actor authority V2 current subject drifted")
	}
	return ports.ReviewActorAuthorityCurrentProjectionRefV2{ID: current.ID, Revision: current.Revision, Digest: current.Digest}, nil
}
func (s *Store) InspectCurrentActorAuthorityV2(ctx context.Context, subject ports.ReviewActorAuthorityCurrentSubjectV2, expected ports.ReviewActorAuthorityCurrentProjectionRefV2) (ports.ReviewActorAuthorityCurrentProjectionV2, error) {
	if err := subject.Validate(); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	want := reviewGovernanceProjectionRefV1{ID: expected.ID, Revision: expected.Revision, Digest: expected.Digest}
	current, highest, err := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionReviewActorAuthorityV2, subject.Target.TenantID, expected.ID)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	if current != want || highest != expected.Revision {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review actor authority V2 current full Ref drifted")
	}
	value, err := loadReviewActorAuthorityProjectionV2(ctx, tx, want)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	if value.Subject != subject {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review actor authority V2 exact subject drifted")
	}
	return cloneStrict(value)
}
func (s *Store) InspectHistoricalActorAuthorityV2(ctx context.Context, ref ports.ReviewActorAuthorityCurrentProjectionRefV2) (ports.ReviewActorAuthorityCurrentProjectionV2, error) {
	if err := contextError(ctx, "Inspect historical Review actor authority V2"); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	return loadReviewActorAuthorityProjectionV2(ctx, s.db, reviewGovernanceProjectionRefV1{ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest})
}
func (s *Store) CommitActorAuthorityV2(ctx context.Context, request ports.ReviewActorAuthorityCurrentPublishRequestV2) (ports.ReviewActorAuthorityCurrentPublishReceiptV2, error) {
	if err := request.Validate(); err != nil {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	value := request.Value.Clone()
	raw := reviewGovernanceProjectionRefV1{ID: value.Ref.ID, Revision: value.Ref.Revision, Digest: value.Ref.Digest}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, loadErr := loadReviewActorAuthorityProjectionV2(ctx, tx, raw); loadErr == nil {
		if !reflect.DeepEqual(existing, value) {
			return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review actor authority V2 exact Ref binds different content")
		}
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{Ref: value.Ref, Created: false}, nil
	} else if !core.HasCategory(loadErr, core.ErrorNotFound) {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, loadErr
	}
	tenant := value.Subject.Target.TenantID
	current, highest, currentErr := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionReviewActorAuthorityV2, tenant, value.Ref.ID)
	if request.Previous == nil {
		if !core.HasCategory(currentErr, core.ErrorNotFound) || value.Ref.Revision != 1 {
			return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review actor authority V2 initial publish lost create-once precondition")
		}
	} else {
		if currentErr != nil {
			return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, currentErr
		}
		previous := reviewGovernanceProjectionRefV1{ID: request.Previous.ID, Revision: request.Previous.Revision, Digest: request.Previous.Digest}
		if current != previous || highest != request.Previous.Revision || raw.ID != previous.ID || raw.Revision != previous.Revision+1 {
			return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review actor authority V2 full-ref CAS drifted")
		}
		prior, loadErr := loadReviewActorAuthorityProjectionV2(ctx, tx, previous)
		if loadErr != nil {
			return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, loadErr
		}
		if !ports.SameReviewActorAuthorityStableIdentityV2(prior.Subject, value.Subject) || value.CheckedUnixNano < prior.CheckedUnixNano || prior.State != ports.ReviewDecisionGovernanceProjectionActiveV1 {
			return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review actor authority V2 identity, state or clock regressed")
		}
	}
	if s.consumeStageFailure() {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review actor authority V2 staged failure")
	}
	if err := insertReviewGovernanceProjectionV1(ctx, tx, reviewGovernanceProjectionReviewActorAuthorityV2, "ReviewActorAuthorityCurrentProjectionV2", tenant, raw, value); err != nil {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	if request.Previous == nil {
		err = insertReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionReviewActorAuthorityV2, tenant, raw)
	} else {
		err = advanceReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionReviewActorAuthorityV2, tenant, reviewGovernanceProjectionRefV1{ID: request.Previous.ID, Revision: request.Previous.Revision, Digest: request.Previous.Digest}, raw)
	}
	if err != nil {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	if s.consumeLostReply() {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Review actor authority V2 publish reply was lost")
	}
	return ports.ReviewActorAuthorityCurrentPublishReceiptV2{Ref: value.Ref, Created: true}, nil
}

var _ control.ReviewActorAuthorityCurrentFactPortV2 = (*Store)(nil)
