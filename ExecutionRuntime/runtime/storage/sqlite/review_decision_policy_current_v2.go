package sqlite

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const reviewGovernanceProjectionReviewDecisionPolicyV2 = "review-decision-policy-v2"

func loadReviewDecisionPolicyProjectionV2(ctx context.Context, source queryRower, ref reviewGovernanceProjectionRefV1) (ports.ReviewDecisionPolicyCurrentProjectionV2, error) {
	tenant, payload, rowDigest, err := loadReviewGovernanceProjectionPayloadV1(ctx, source, reviewGovernanceProjectionReviewDecisionPolicyV2, ref)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	value, err := decodeRow[ports.ReviewDecisionPolicyCurrentProjectionV2](payload, rowDigest, "ReviewDecisionPolicyCurrentProjectionV2")
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	if value.Subject.TenantID != tenant || value.Ref.ID != ref.ID || value.Ref.Revision != ref.Revision || value.Ref.Digest != ref.Digest || value.Validate() != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review decision policy V2 historical row drifted")
	}
	return cloneStrict(value)
}

func (s *Store) ResolvePolicyV2(ctx context.Context, subject ports.ReviewDecisionPolicyApplicabilitySubjectV2) (ports.ReviewDecisionPolicyCurrentProjectionRefV2, error) {
	if err := subject.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, err
	}
	id, err := ports.DeriveReviewDecisionPolicyCurrentProjectionIDV2(subject)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, highest, err := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionReviewDecisionPolicyV2, subject.TenantID, id)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, err
	}
	if highest != current.Revision {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review decision policy V2 current/highest drifted")
	}
	value, err := loadReviewDecisionPolicyProjectionV2(ctx, tx, current)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, err
	}
	if !ports.SameReviewDecisionPolicyApplicabilitySubjectV2(value.Subject, subject) {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review decision policy V2 current subject drifted")
	}
	return ports.ReviewDecisionPolicyCurrentProjectionRefV2{ID: current.ID, Revision: current.Revision, Digest: current.Digest}, nil
}

func (s *Store) InspectCurrentPolicyV2(ctx context.Context, subject ports.ReviewDecisionPolicyApplicabilitySubjectV2, expected ports.ReviewDecisionPolicyCurrentProjectionRefV2) (ports.ReviewDecisionPolicyCurrentProjectionV2, error) {
	if err := subject.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	want := reviewGovernanceProjectionRefV1{ID: expected.ID, Revision: expected.Revision, Digest: expected.Digest}
	current, highest, err := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionReviewDecisionPolicyV2, subject.TenantID, expected.ID)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	if current != want || highest != expected.Revision {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review decision policy V2 current full Ref drifted")
	}
	value, err := loadReviewDecisionPolicyProjectionV2(ctx, tx, want)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	if !ports.SameReviewDecisionPolicyApplicabilitySubjectV2(value.Subject, subject) {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review decision policy V2 exact subject drifted")
	}
	return cloneStrict(value)
}

func (s *Store) InspectHistoricalPolicyV2(ctx context.Context, ref ports.ReviewDecisionPolicyCurrentProjectionRefV2) (ports.ReviewDecisionPolicyCurrentProjectionV2, error) {
	if err := contextError(ctx, "Inspect historical Review decision policy V2"); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	return loadReviewDecisionPolicyProjectionV2(ctx, s.db, reviewGovernanceProjectionRefV1{ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest})
}

func (s *Store) CommitPolicyV2(ctx context.Context, request ports.ReviewDecisionPolicyCurrentPublishRequestV2) (ports.ReviewDecisionPolicyCurrentPublishReceiptV2, error) {
	if err := request.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
	}
	value := request.Value.Clone()
	raw := reviewGovernanceProjectionRefV1{ID: value.Ref.ID, Revision: value.Ref.Revision, Digest: value.Ref.Digest}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, loadErr := loadReviewDecisionPolicyProjectionV2(ctx, tx, raw); loadErr == nil {
		if !reflect.DeepEqual(existing, value) {
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review decision policy V2 exact Ref binds different content")
		}
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{Ref: value.Ref, Created: false}, nil
	} else if !core.HasCategory(loadErr, core.ErrorNotFound) {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, loadErr
	}
	current, highest, currentErr := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionReviewDecisionPolicyV2, value.Subject.TenantID, value.Ref.ID)
	if request.Previous == nil {
		if !core.HasCategory(currentErr, core.ErrorNotFound) || value.Ref.Revision != 1 {
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review decision policy V2 initial publish lost create-once precondition")
		}
	} else {
		if currentErr != nil {
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, currentErr
		}
		previous := reviewGovernanceProjectionRefV1{ID: request.Previous.ID, Revision: request.Previous.Revision, Digest: request.Previous.Digest}
		if current != previous || highest != previous.Revision || raw.ID != previous.ID || raw.Revision != previous.Revision+1 {
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review decision policy V2 full-ref CAS drifted")
		}
		prior, loadErr := loadReviewDecisionPolicyProjectionV2(ctx, tx, previous)
		if loadErr != nil {
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, loadErr
		}
		if !ports.SameReviewDecisionPolicyProjectionIdentityV2(prior.Subject, value.Subject) || value.CheckedUnixNano < prior.CheckedUnixNano {
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review decision policy V2 stable identity or clock regressed")
		}
	}
	if s.consumeStageFailure() {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review decision policy V2 staged failure")
	}
	if err := insertReviewGovernanceProjectionV1(ctx, tx, reviewGovernanceProjectionReviewDecisionPolicyV2, "ReviewDecisionPolicyCurrentProjectionV2", value.Subject.TenantID, raw, value); err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
	}
	if request.Previous == nil {
		err = insertReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionReviewDecisionPolicyV2, value.Subject.TenantID, raw)
	} else {
		err = advanceReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionReviewDecisionPolicyV2, value.Subject.TenantID, reviewGovernanceProjectionRefV1{ID: request.Previous.ID, Revision: request.Previous.Revision, Digest: request.Previous.Digest}, raw)
	}
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
	}
	if s.consumeLostReply() {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Review decision policy V2 publish reply was lost")
	}
	return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{Ref: value.Ref, Created: true}, nil
}

var _ control.ReviewDecisionPolicyCurrentFactPortV2 = (*Store)(nil)
