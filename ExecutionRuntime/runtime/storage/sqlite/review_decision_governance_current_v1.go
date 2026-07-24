package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	reviewGovernanceProjectionPolicyV1    = "policy"
	reviewGovernanceProjectionAuthorityV1 = "authority"
	reviewGovernanceProjectionScopeV1     = "scope"
)

type reviewGovernanceProjectionRefV1 struct {
	ID       string
	Revision core.Revision
	Digest   core.Digest
}

func loadReviewGovernanceProjectionCurrentV1(ctx context.Context, source queryRower, kind string, tenant core.TenantID, id string) (reviewGovernanceProjectionRefV1, core.Revision, error) {
	if err := contextError(ctx, "Inspect Review governance projection current index"); err != nil {
		return reviewGovernanceProjectionRefV1{}, 0, err
	}
	var revision, highest, historyHighest uint64
	var digest string
	err := source.QueryRowContext(ctx, `SELECT c.revision,c.projection_digest,c.highest_revision,
  (SELECT COALESCE(MAX(h.revision),0) FROM runtime_review_governance_projection_history h
   WHERE h.kind=c.kind AND h.projection_id=c.projection_id)
FROM runtime_review_governance_projection_current c
WHERE c.kind=? AND c.tenant_id=? AND c.projection_id=?`, kind, string(tenant), id).Scan(&revision, &digest, &highest, &historyHighest)
	if errors.Is(err, sql.ErrNoRows) {
		return reviewGovernanceProjectionRefV1{}, 0, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review governance projection current index is absent")
	}
	if err != nil {
		return reviewGovernanceProjectionRefV1{}, 0, mapDBError(ctx, err, false)
	}
	if revision == 0 || highest == 0 || historyHighest == 0 || revision != highest || highest != historyHighest || digest == "" {
		return reviewGovernanceProjectionRefV1{}, 0, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review governance projection current/highest/history index drifted")
	}
	return reviewGovernanceProjectionRefV1{ID: id, Revision: core.Revision(revision), Digest: core.Digest(digest)}, core.Revision(highest), nil
}

func loadReviewGovernanceProjectionPayloadV1(ctx context.Context, source queryRower, kind string, ref reviewGovernanceProjectionRefV1) (core.TenantID, []byte, string, error) {
	if err := contextError(ctx, "Inspect Review governance projection history"); err != nil {
		return "", nil, "", err
	}
	var tenant, projectionDigest, rowDigest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT tenant_id,projection_digest,row_digest,canonical_json FROM runtime_review_governance_projection_history WHERE kind=? AND projection_id=? AND revision=?`, kind, ref.ID, ref.Revision).Scan(&tenant, &projectionDigest, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, "", core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review governance historical projection is absent")
	}
	if err != nil {
		return "", nil, "", mapDBError(ctx, err, false)
	}
	if projectionDigest != string(ref.Digest) {
		return "", nil, "", core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review governance historical projection digest drifted")
	}
	return core.TenantID(tenant), payload, rowDigest, nil
}

func insertReviewGovernanceProjectionV1(ctx context.Context, tx *sql.Tx, kind, discriminator string, tenant core.TenantID, ref reviewGovernanceProjectionRefV1, value any) error {
	payload, err := marshalStrict(value)
	if err != nil {
		return err
	}
	rowDigest, err := digestRow(discriminator, value)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_review_governance_projection_history(kind,tenant_id,projection_id,revision,projection_digest,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?)`, kind, string(tenant), ref.ID, ref.Revision, string(ref.Digest), string(rowDigest), payload)
	return mapDBError(ctx, err, true)
}

func insertReviewGovernanceProjectionCurrentV1(ctx context.Context, tx *sql.Tx, kind string, tenant core.TenantID, ref reviewGovernanceProjectionRefV1) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO runtime_review_governance_projection_current(kind,tenant_id,projection_id,revision,projection_digest,highest_revision) VALUES(?,?,?,?,?,?)`, kind, string(tenant), ref.ID, ref.Revision, string(ref.Digest), ref.Revision)
	return mapDBError(ctx, err, true)
}

func advanceReviewGovernanceProjectionCurrentV1(ctx context.Context, tx *sql.Tx, kind string, tenant core.TenantID, expected, next reviewGovernanceProjectionRefV1) error {
	result, err := tx.ExecContext(ctx, `UPDATE runtime_review_governance_projection_current SET revision=?,projection_digest=?,highest_revision=? WHERE kind=? AND tenant_id=? AND projection_id=? AND revision=? AND projection_digest=? AND highest_revision=?`, next.Revision, string(next.Digest), next.Revision, kind, string(tenant), expected.ID, expected.Revision, string(expected.Digest), expected.Revision)
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review governance projection current/highest full-ref CAS lost its precondition")
	}
	return nil
}

func (s *Store) ResolvePolicyV1(ctx context.Context, subject ports.ReviewDecisionPolicyCurrentSubjectV1) (ports.ReviewDecisionPolicyCurrentProjectionRefV1, error) {
	if err := subject.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV1{}, err
	}
	id, err := ports.DeriveReviewDecisionPolicyCurrentProjectionIDV1(subject, subject.Policy.Ref)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	ref, highest, err := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionPolicyV1, subject.Target.TenantID, id)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV1{}, err
	}
	if highest != ref.Revision {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Policy projection current/highest index drifted")
	}
	value, err := loadPolicyProjectionV1(ctx, tx, ref)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV1{}, err
	}
	if value.Subject != subject {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Policy projection Resolve subject drifted")
	}
	return ports.ReviewDecisionPolicyCurrentProjectionRefV1{ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest}, nil
}

func (s *Store) InspectCurrentPolicyV1(ctx context.Context, subject ports.ReviewDecisionPolicyCurrentSubjectV1, expected ports.ReviewDecisionPolicyCurrentProjectionRefV1) (ports.ReviewDecisionPolicyCurrentProjectionV1, error) {
	if err := subject.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	raw := reviewGovernanceProjectionRefV1{expected.ID, expected.Revision, expected.Digest}
	current, highest, err := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionPolicyV1, subject.Target.TenantID, expected.ID)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	if current != raw || highest != expected.Revision {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Policy projection current index drifted")
	}
	value, err := loadPolicyProjectionV1(ctx, tx, raw)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	if value.Subject != subject {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Policy projection subject drifted")
	}
	return cloneStrict(value)
}

func (s *Store) InspectHistoricalPolicyV1(ctx context.Context, ref ports.ReviewDecisionPolicyCurrentProjectionRefV1) (ports.ReviewDecisionPolicyCurrentProjectionV1, error) {
	if err := ref.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	return loadPolicyProjectionV1(ctx, s.db, reviewGovernanceProjectionRefV1{ref.ID, ref.Revision, ref.Digest})
}

func loadPolicyProjectionV1(ctx context.Context, source queryRower, ref reviewGovernanceProjectionRefV1) (ports.ReviewDecisionPolicyCurrentProjectionV1, error) {
	tenant, payload, rowDigest, err := loadReviewGovernanceProjectionPayloadV1(ctx, source, reviewGovernanceProjectionPolicyV1, ref)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	value, err := decodeRow[ports.ReviewDecisionPolicyCurrentProjectionV1](payload, rowDigest, "ReviewDecisionPolicyCurrentProjectionV1")
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	if value.Subject.Target.TenantID != tenant || value.Ref.ID != ref.ID || value.Ref.Revision != ref.Revision || value.Ref.Digest != ref.Digest || value.Validate() != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review Policy historical projection row drifted")
	}
	return cloneStrict(value)
}

func (s *Store) CommitPolicyV1(ctx context.Context, request ports.ReviewDecisionPolicyCurrentPublishRequestV1) (ports.ReviewDecisionPolicyCurrentPublishReceiptV1, error) {
	if err := request.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
	}
	created, err := commitGovernanceProjectionV1(ctx, s, reviewGovernanceProjectionPolicyV1, "ReviewDecisionPolicyCurrentProjectionV1", request.Value.Subject.Target.TenantID, reviewGovernanceProjectionRefV1{request.Value.Ref.ID, request.Value.Ref.Revision, request.Value.Ref.Digest}, policyPreviousV1(request.Previous), request.Value, func(v any) bool {
		stored, ok := v.(ports.ReviewDecisionPolicyCurrentProjectionV1)
		return ok && reflect.DeepEqual(stored, request.Value)
	}, func(ctx context.Context, source queryRower, ref reviewGovernanceProjectionRefV1) (any, error) {
		return loadPolicyProjectionV1(ctx, source, ref)
	})
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
	}
	return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{Ref: request.Value.Ref, Created: created}, nil
}

func policyPreviousV1(ref *ports.ReviewDecisionPolicyCurrentProjectionRefV1) *reviewGovernanceProjectionRefV1 {
	if ref == nil {
		return nil
	}
	return &reviewGovernanceProjectionRefV1{ref.ID, ref.Revision, ref.Digest}
}

func (s *Store) ResolveAuthorityV1(ctx context.Context, subject ports.ReviewDecisionAuthorityCurrentSubjectV1) (ports.ReviewDecisionAuthorityCurrentProjectionRefV1, error) {
	if err := subject.Validate(); err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionRefV1{}, err
	}
	id, err := ports.DeriveReviewDecisionAuthorityCurrentProjectionIDV1(subject, subject.Authority.Ref)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionRefV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionRefV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	ref, highest, err := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionAuthorityV1, subject.Target.TenantID, id)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionRefV1{}, err
	}
	if highest != ref.Revision {
		return ports.ReviewDecisionAuthorityCurrentProjectionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Authority projection current/highest index drifted")
	}
	value, err := loadAuthorityProjectionV1(ctx, tx, ref)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionRefV1{}, err
	}
	if !reflect.DeepEqual(value.Subject, subject) {
		return ports.ReviewDecisionAuthorityCurrentProjectionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Authority projection Resolve subject drifted")
	}
	return ports.ReviewDecisionAuthorityCurrentProjectionRefV1{ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest}, nil
}

func (s *Store) InspectCurrentAuthorityV1(ctx context.Context, subject ports.ReviewDecisionAuthorityCurrentSubjectV1, expected ports.ReviewDecisionAuthorityCurrentProjectionRefV1) (ports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
	if err := subject.Validate(); err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	raw := reviewGovernanceProjectionRefV1{expected.ID, expected.Revision, expected.Digest}
	current, highest, err := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionAuthorityV1, subject.Target.TenantID, expected.ID)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	if current != raw || highest != expected.Revision {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Authority projection current index drifted")
	}
	value, err := loadAuthorityProjectionV1(ctx, tx, raw)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	if !reflect.DeepEqual(value.Subject, subject) {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Authority projection subject drifted")
	}
	return cloneStrict(value)
}

func (s *Store) InspectHistoricalAuthorityV1(ctx context.Context, ref ports.ReviewDecisionAuthorityCurrentProjectionRefV1) (ports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
	if err := ref.Validate(); err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	return loadAuthorityProjectionV1(ctx, s.db, reviewGovernanceProjectionRefV1{ref.ID, ref.Revision, ref.Digest})
}

func loadAuthorityProjectionV1(ctx context.Context, source queryRower, ref reviewGovernanceProjectionRefV1) (ports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
	tenant, payload, rowDigest, err := loadReviewGovernanceProjectionPayloadV1(ctx, source, reviewGovernanceProjectionAuthorityV1, ref)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	value, err := decodeRow[ports.ReviewDecisionAuthorityCurrentProjectionV1](payload, rowDigest, "ReviewDecisionAuthorityCurrentProjectionV1")
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	if value.Subject.Target.TenantID != tenant || value.Ref.ID != ref.ID || value.Ref.Revision != ref.Revision || value.Ref.Digest != ref.Digest || value.Validate() != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review Authority historical projection row drifted")
	}
	return cloneStrict(value)
}

func (s *Store) CommitAuthorityV1(ctx context.Context, request ports.ReviewDecisionAuthorityCurrentPublishRequestV1) (ports.ReviewDecisionAuthorityCurrentPublishReceiptV1, error) {
	if err := request.Validate(); err != nil {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
	}
	created, err := commitGovernanceProjectionV1(ctx, s, reviewGovernanceProjectionAuthorityV1, "ReviewDecisionAuthorityCurrentProjectionV1", request.Value.Subject.Target.TenantID, reviewGovernanceProjectionRefV1{request.Value.Ref.ID, request.Value.Ref.Revision, request.Value.Ref.Digest}, authorityPreviousV1(request.Previous), request.Value, func(v any) bool {
		stored, ok := v.(ports.ReviewDecisionAuthorityCurrentProjectionV1)
		return ok && reflect.DeepEqual(stored, request.Value)
	}, func(ctx context.Context, source queryRower, ref reviewGovernanceProjectionRefV1) (any, error) {
		return loadAuthorityProjectionV1(ctx, source, ref)
	})
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
	}
	return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{Ref: request.Value.Ref, Created: created}, nil
}

func authorityPreviousV1(ref *ports.ReviewDecisionAuthorityCurrentProjectionRefV1) *reviewGovernanceProjectionRefV1 {
	if ref == nil {
		return nil
	}
	return &reviewGovernanceProjectionRefV1{ref.ID, ref.Revision, ref.Digest}
}

func (s *Store) ResolveScopeV1(ctx context.Context, subject ports.ReviewDecisionScopeCurrentSubjectV1) (ports.ReviewDecisionScopeCurrentProjectionRefV1, error) {
	if err := subject.Validate(); err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionRefV1{}, err
	}
	id, err := ports.DeriveReviewDecisionScopeCurrentProjectionIDV1(subject, subject.CurrentScope.Ref)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionRefV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionRefV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	ref, highest, err := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionScopeV1, subject.TenantID, id)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionRefV1{}, err
	}
	if highest != ref.Revision {
		return ports.ReviewDecisionScopeCurrentProjectionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Scope projection current/highest index drifted")
	}
	value, err := loadScopeProjectionV1(ctx, tx, ref)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionRefV1{}, err
	}
	if !reflect.DeepEqual(value.Subject, subject) {
		return ports.ReviewDecisionScopeCurrentProjectionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Scope projection Resolve subject drifted")
	}
	return ports.ReviewDecisionScopeCurrentProjectionRefV1{ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest}, nil
}

func (s *Store) InspectCurrentScopeV1(ctx context.Context, subject ports.ReviewDecisionScopeCurrentSubjectV1, expected ports.ReviewDecisionScopeCurrentProjectionRefV1) (ports.ReviewDecisionScopeCurrentProjectionV1, error) {
	if err := subject.Validate(); err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	raw := reviewGovernanceProjectionRefV1{expected.ID, expected.Revision, expected.Digest}
	current, highest, err := loadReviewGovernanceProjectionCurrentV1(ctx, tx, reviewGovernanceProjectionScopeV1, subject.TenantID, expected.ID)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	if current != raw || highest != expected.Revision {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Scope projection current index drifted")
	}
	value, err := loadScopeProjectionV1(ctx, tx, raw)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	if !reflect.DeepEqual(value.Subject, subject) {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Scope projection subject drifted")
	}
	return cloneStrict(value)
}

func (s *Store) InspectHistoricalScopeV1(ctx context.Context, ref ports.ReviewDecisionScopeCurrentProjectionRefV1) (ports.ReviewDecisionScopeCurrentProjectionV1, error) {
	if err := ref.Validate(); err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	return loadScopeProjectionV1(ctx, s.db, reviewGovernanceProjectionRefV1{ref.ID, ref.Revision, ref.Digest})
}

func loadScopeProjectionV1(ctx context.Context, source queryRower, ref reviewGovernanceProjectionRefV1) (ports.ReviewDecisionScopeCurrentProjectionV1, error) {
	tenant, payload, rowDigest, err := loadReviewGovernanceProjectionPayloadV1(ctx, source, reviewGovernanceProjectionScopeV1, ref)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	value, err := decodeRow[ports.ReviewDecisionScopeCurrentProjectionV1](payload, rowDigest, "ReviewDecisionScopeCurrentProjectionV1")
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	if value.Subject.TenantID != tenant || value.Ref.ID != ref.ID || value.Ref.Revision != ref.Revision || value.Ref.Digest != ref.Digest || value.Validate() != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review Scope historical projection row drifted")
	}
	return cloneStrict(value)
}

func (s *Store) CommitScopeV1(ctx context.Context, request ports.ReviewDecisionScopeCurrentPublishRequestV1) (ports.ReviewDecisionScopeCurrentPublishReceiptV1, error) {
	if err := request.Validate(); err != nil {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
	}
	created, err := commitGovernanceProjectionV1(ctx, s, reviewGovernanceProjectionScopeV1, "ReviewDecisionScopeCurrentProjectionV1", request.Value.Subject.TenantID, reviewGovernanceProjectionRefV1{request.Value.Ref.ID, request.Value.Ref.Revision, request.Value.Ref.Digest}, scopePreviousV1(request.Previous), request.Value, func(v any) bool {
		stored, ok := v.(ports.ReviewDecisionScopeCurrentProjectionV1)
		return ok && reflect.DeepEqual(stored, request.Value)
	}, func(ctx context.Context, source queryRower, ref reviewGovernanceProjectionRefV1) (any, error) {
		return loadScopeProjectionV1(ctx, source, ref)
	})
	if err != nil {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
	}
	return ports.ReviewDecisionScopeCurrentPublishReceiptV1{Ref: request.Value.Ref, Created: created}, nil
}

func scopePreviousV1(ref *ports.ReviewDecisionScopeCurrentProjectionRefV1) *reviewGovernanceProjectionRefV1 {
	if ref == nil {
		return nil
	}
	return &reviewGovernanceProjectionRefV1{ref.ID, ref.Revision, ref.Digest}
}

type governanceProjectionLoaderV1 func(context.Context, queryRower, reviewGovernanceProjectionRefV1) (any, error)

func commitGovernanceProjectionV1(ctx context.Context, s *Store, kind, discriminator string, tenant core.TenantID, ref reviewGovernanceProjectionRefV1, previous *reviewGovernanceProjectionRefV1, value any, same func(any) bool, load governanceProjectionLoaderV1) (bool, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, loadErr := load(ctx, tx, ref); loadErr == nil {
		if !same(existing) {
			return false, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review governance projection exact Ref binds different content")
		}
		return false, nil
	} else if !core.HasCategory(loadErr, core.ErrorNotFound) {
		return false, loadErr
	}
	current, highest, currentErr := loadReviewGovernanceProjectionCurrentV1(ctx, tx, kind, tenant, ref.ID)
	if previous == nil {
		if !core.HasCategory(currentErr, core.ErrorNotFound) || ref.Revision != 1 {
			return false, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review governance initial projection lost create-once precondition")
		}
	} else {
		if currentErr != nil {
			return false, currentErr
		}
		if current != *previous || highest != previous.Revision || ref.ID != previous.ID || ref.Revision != previous.Revision+1 {
			return false, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review governance projection full-ref CAS or monotonic revision drifted")
		}
	}
	if s.consumeStageFailure() {
		return false, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review governance projection staged failure")
	}
	if err := insertReviewGovernanceProjectionV1(ctx, tx, kind, discriminator, tenant, ref, value); err != nil {
		return false, err
	}
	if previous == nil {
		err = insertReviewGovernanceProjectionCurrentV1(ctx, tx, kind, tenant, ref)
	} else {
		err = advanceReviewGovernanceProjectionCurrentV1(ctx, tx, kind, tenant, *previous, ref)
	}
	if err != nil {
		return false, err
	}
	if err := commit(ctx, tx); err != nil {
		return false, err
	}
	if s.consumeLostReply() {
		return false, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Review governance projection commit reply was lost")
	}
	return true, nil
}

var _ control.ReviewDecisionGovernanceCurrentFactPortV1 = (*Store)(nil)
