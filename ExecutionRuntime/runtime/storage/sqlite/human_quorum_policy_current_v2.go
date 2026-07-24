package sqlite

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const reviewGovernanceProjectionHumanQuorumPolicyV2 = "human-quorum-policy-v2"

func loadHumanQuorumPolicyProjectionV2(ctx context.Context, source queryRower, ref reviewGovernanceProjectionRefV1) (ports.HumanQuorumPolicyCurrentProjectionV2, error) {
	tenant, payload, rowDigest, err := loadReviewGovernanceProjectionPayloadV1(ctx, source, reviewGovernanceProjectionHumanQuorumPolicyV2, ref)
	if err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionV2{}, err
	}
	value, err := decodeRow[ports.HumanQuorumPolicyCurrentProjectionV2](payload, rowDigest, "HumanQuorumPolicyCurrentProjectionV2")
	if err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionV2{}, err
	}
	if value.Subject.TenantID != tenant || value.Ref.ID != ref.ID || value.Ref.Revision != ref.Revision || value.Ref.Digest != ref.Digest || value.Validate() != nil {
		return ports.HumanQuorumPolicyCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "human quorum policy historical projection row drifted")
	}
	return cloneStrict(value)
}

func (s *Store) ResolveCurrentHumanQuorumPolicyV2(ctx context.Context, request ports.HumanQuorumPolicyCurrentResolveRequestV2) (ports.HumanQuorumPolicyCurrentProjectionRefV2, error) {
	if err := request.Validate(); err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionRefV2{}, err
	}
	baseline, err := s.humanQuorumPolicyClockV2(time.Time{})
	if err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionRefV2{}, err
	}
	id, err := ports.DeriveHumanQuorumPolicyCurrentProjectionIDV2(request.Subject)
	if err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionRefV2{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionRefV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, highest, err := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionHumanQuorumPolicyV2, request.Subject.TenantID, id)
	if err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionRefV2{}, err
	}
	if highest != current.Revision {
		return ports.HumanQuorumPolicyCurrentProjectionRefV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "human quorum policy current/highest index drifted")
	}
	value, err := loadHumanQuorumPolicyProjectionV2(ctx, tx, current)
	if err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionRefV2{}, err
	}
	now, err := s.humanQuorumPolicyClockV2(baseline)
	if err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionRefV2{}, err
	}
	ref := ports.HumanQuorumPolicyCurrentProjectionRefV2{ID: current.ID, Revision: current.Revision, Digest: current.Digest}
	if err := value.ValidateCurrent(ref, request.Subject, now); err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionRefV2{}, err
	}
	return ref, nil
}

func (s *Store) InspectCurrentHumanQuorumPolicyV2(ctx context.Context, subject ports.HumanQuorumPolicyCurrentSubjectV2, expected ports.HumanQuorumPolicyCurrentProjectionRefV2) (ports.HumanQuorumPolicyCurrentProjectionV2, error) {
	if err := subject.Validate(); err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionV2{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionV2{}, err
	}
	baseline, err := s.humanQuorumPolicyClockV2(time.Time{})
	if err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionV2{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	want := reviewGovernanceProjectionRefV1{ID: expected.ID, Revision: expected.Revision, Digest: expected.Digest}
	current, highest, err := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionHumanQuorumPolicyV2, subject.TenantID, expected.ID)
	if err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionV2{}, err
	}
	if current != want || highest != expected.Revision {
		return ports.HumanQuorumPolicyCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "human quorum policy current full Ref drifted")
	}
	value, err := loadHumanQuorumPolicyProjectionV2(ctx, tx, want)
	if err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionV2{}, err
	}
	now, err := s.humanQuorumPolicyClockV2(baseline)
	if err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionV2{}, err
	}
	if err := value.ValidateCurrent(expected, subject, now); err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionV2{}, err
	}
	return cloneStrict(value)
}

func (s *Store) InspectHistoricalHumanQuorumPolicyV2(ctx context.Context, ref ports.HumanQuorumPolicyCurrentProjectionRefV2) (ports.HumanQuorumPolicyCurrentProjectionV2, error) {
	if err := contextError(ctx, "Inspect historical human quorum policy"); err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.HumanQuorumPolicyCurrentProjectionV2{}, err
	}
	return loadHumanQuorumPolicyProjectionV2(ctx, s.db, reviewGovernanceProjectionRefV1{ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest})
}

func (s *Store) PublishHumanQuorumPolicyCurrentV2(ctx context.Context, request ports.HumanQuorumPolicyCurrentPublishRequestV2) (ports.HumanQuorumPolicyCurrentPublishReceiptV2, error) {
	if err := request.Validate(); err != nil {
		return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, err
	}
	baseline, err := s.humanQuorumPolicyClockV2(time.Time{})
	if err != nil {
		return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, err
	}
	value := request.Value.Clone()
	raw := reviewGovernanceProjectionRefV1{ID: value.Ref.ID, Revision: value.Ref.Revision, Digest: value.Ref.Digest}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, loadErr := loadHumanQuorumPolicyProjectionV2(ctx, tx, raw); loadErr == nil {
		if !reflect.DeepEqual(existing, value) {
			return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "human quorum policy exact Ref binds different content")
		}
		return ports.HumanQuorumPolicyCurrentPublishReceiptV2{Ref: value.Ref, Created: false}, nil
	} else if !core.HasCategory(loadErr, core.ErrorNotFound) {
		return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, loadErr
	}
	current, highest, currentErr := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionHumanQuorumPolicyV2, value.Subject.TenantID, value.Ref.ID)
	if request.Previous == nil {
		if !core.HasCategory(currentErr, core.ErrorNotFound) || value.Ref.Revision != 1 {
			return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "human quorum policy initial publish lost create-once precondition")
		}
	} else {
		if currentErr != nil {
			return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, currentErr
		}
		previous := reviewGovernanceProjectionRefV1{ID: request.Previous.ID, Revision: request.Previous.Revision, Digest: request.Previous.Digest}
		if current != previous || highest != previous.Revision || raw.ID != previous.ID || raw.Revision != previous.Revision+1 {
			return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "human quorum policy full-ref CAS or revision drifted")
		}
		prior, loadErr := loadHumanQuorumPolicyProjectionV2(ctx, tx, previous)
		if loadErr != nil {
			return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, loadErr
		}
		if prior.Subject != value.Subject || value.CheckedUnixNano < prior.CheckedUnixNano {
			return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonClockRegression, "human quorum policy subject or sealed clock regressed")
		}
	}
	now, err := s.humanQuorumPolicyClockV2(baseline)
	if err != nil {
		return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, err
	}
	if now.UnixNano() < value.CheckedUnixNano || !now.Before(time.Unix(0, value.ExpiresUnixNano)) {
		return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human quorum policy publish crossed its sealed current window")
	}
	if s.consumeStageFailure() {
		return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected human quorum policy staged failure")
	}
	if err := insertReviewGovernanceProjectionV1(ctx, tx, reviewGovernanceProjectionHumanQuorumPolicyV2, "HumanQuorumPolicyCurrentProjectionV2", value.Subject.TenantID, raw, value); err != nil {
		return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, err
	}
	if request.Previous == nil {
		err = insertReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionHumanQuorumPolicyV2, value.Subject.TenantID, raw)
	} else {
		previous := reviewGovernanceProjectionRefV1{ID: request.Previous.ID, Revision: request.Previous.Revision, Digest: request.Previous.Digest}
		err = advanceReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionHumanQuorumPolicyV2, value.Subject.TenantID, previous, raw)
	}
	if err != nil {
		return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, err
	}
	if s.consumeLostReply() {
		return ports.HumanQuorumPolicyCurrentPublishReceiptV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "human quorum policy publish reply was lost")
	}
	return ports.HumanQuorumPolicyCurrentPublishReceiptV2{Ref: value.Ref, Created: true}, nil
}

func (s *Store) humanQuorumPolicyClockV2(baseline time.Time) (time.Time, error) {
	now := s.clock()
	if now.IsZero() || !baseline.IsZero() && now.Before(baseline) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "human quorum policy clock regressed")
	}
	return now, nil
}
