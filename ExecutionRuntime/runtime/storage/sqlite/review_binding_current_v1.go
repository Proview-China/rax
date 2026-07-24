package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func loadAssociationHistory(ctx context.Context, source queryRower, expected ports.ReviewBindingConsumerAssociationRefV1) (ports.ReviewBindingConsumerAssociationCurrentProjectionV1, error) {
	var digest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT digest,canonical_json FROM runtime_review_binding_association_history WHERE id=? AND revision=?`, expected.ID, expected.Revision).Scan(&digest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review Binding Consumer association history is absent")
	}
	if err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, mapDBError(ctx, err, false)
	}
	projection, err := decodeRow[ports.ReviewBindingConsumerAssociationCurrentProjectionV1](payload, digest, "ReviewBindingConsumerAssociationCurrentProjectionV1")
	if err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if projection.Ref != expected || projection.Validate() != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding Consumer association history drifted")
	}
	return projection, nil
}

func loadCurrentAssociation(ctx context.Context, source queryRower, expected ports.ReviewBindingConsumerAssociationRefV1) (ports.ReviewBindingConsumerAssociationCurrentProjectionV1, error) {
	var revision uint64
	var digest string
	err := source.QueryRowContext(ctx, `SELECT revision,digest FROM runtime_review_binding_association_current WHERE id=?`, expected.ID).Scan(&revision, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review Binding Consumer association current index is absent")
	}
	if err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, mapDBError(ctx, err, false)
	}
	if uint64(expected.Revision) != revision || string(expected.Digest) != digest {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Binding Consumer association current index drifted")
	}
	return loadAssociationHistory(ctx, source, expected)
}

func insertAssociation(ctx context.Context, tx *sql.Tx, projection ports.ReviewBindingConsumerAssociationCurrentProjectionV1) error {
	payload, err := marshalStrict(projection)
	if err != nil {
		return err
	}
	rowDigest, err := digestRow("ReviewBindingConsumerAssociationCurrentProjectionV1", projection)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_review_binding_association_history(id,revision,digest,canonical_json) VALUES(?,?,?,?)`, projection.Ref.ID, projection.Ref.Revision, string(rowDigest), payload); err != nil {
		return mapDBError(ctx, err, true)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_review_binding_association_current(id,revision,digest) VALUES(?,?,?)`, projection.Ref.ID, projection.Ref.Revision, string(projection.Ref.Digest))
	return mapDBError(ctx, err, true)
}

func advanceAssociation(ctx context.Context, tx *sql.Tx, expected ports.ReviewBindingConsumerAssociationRefV1, next ports.ReviewBindingConsumerAssociationCurrentProjectionV1) error {
	payload, err := marshalStrict(next)
	if err != nil {
		return err
	}
	rowDigest, err := digestRow("ReviewBindingConsumerAssociationCurrentProjectionV1", next)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_review_binding_association_history(id,revision,digest,canonical_json) VALUES(?,?,?,?)`, next.Ref.ID, next.Ref.Revision, string(rowDigest), payload); err != nil {
		return mapDBError(ctx, err, true)
	}
	result, err := tx.ExecContext(ctx, `UPDATE runtime_review_binding_association_current SET revision=?,digest=? WHERE id=? AND revision=? AND digest=?`, next.Ref.Revision, string(next.Ref.Digest), expected.ID, expected.Revision, string(expected.Digest))
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Binding Consumer association CAS lost its current precondition")
	}
	return nil
}

// CreateReviewBindingConsumerAssociationV1 is a Runtime Binding Owner method,
// not a consumer-facing public port.
func (s *Store) CreateReviewBindingConsumerAssociationV1(ctx context.Context, projection ports.ReviewBindingConsumerAssociationCurrentProjectionV1) (ports.ReviewBindingConsumerAssociationCurrentProjectionV1, error) {
	if err := projection.Validate(); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if projection.Ref.Revision != 1 || !projection.Current {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "Review Binding Consumer association must start current at revision one")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	now := s.clock()
	if err := projection.ValidateCurrent(projection.Ref, projection.Consumer, projection.Source, now); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if _, err := readAuthoritativeClosure(ctx, tx, projection.Source, projection, now); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	staged, err := cloneStrict(projection)
	if err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if s.consumeStageFailure() {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review Binding association staged failure")
	}
	if err := insertAssociation(ctx, tx, staged); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if s.consumeLostReply() {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Review Binding association Create reply loss")
	}
	return cloneStrict(staged)
}

// CompareAndSwapReviewBindingConsumerAssociationV1 is Owner-only. It refuses
// to advance an association referenced by an active projection; such changes
// require the compound association/projection transaction.
func (s *Store) CompareAndSwapReviewBindingConsumerAssociationV1(ctx context.Context, expected ports.ReviewBindingConsumerAssociationRefV1, next ports.ReviewBindingConsumerAssociationCurrentProjectionV1) (ports.ReviewBindingConsumerAssociationCurrentProjectionV1, error) {
	if err := expected.Validate(); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if err := next.Validate(); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	previous, err := loadCurrentAssociation(ctx, tx, expected)
	if err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if next.Ref.ID != expected.ID || next.Ref.Revision != expected.Revision+1 || !next.Current || previous.Consumer != next.Consumer || previous.Source != next.Source || previous.CheckedUnixNano > next.CheckedUnixNano {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding Consumer association CAS coordinates drifted")
	}
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM runtime_review_binding_projection_history h JOIN runtime_review_binding_projection_current c ON c.tenant_id=h.tenant_id AND c.id=h.id AND c.revision=h.revision WHERE json_extract(h.canonical_json,'$.consumer_association.ref.id')=? AND json_extract(h.canonical_json,'$.consumer_association.ref.revision')=? AND json_extract(h.canonical_json,'$.consumer_association.ref.digest')=?`, expected.ID, expected.Revision, string(expected.Digest)).Scan(&active); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, mapDBError(ctx, err, false)
	}
	if active != 0 {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "active Review projections require compound association CAS")
	}
	now := s.clock()
	if err := next.ValidateCurrent(next.Ref, next.Consumer, next.Source, now); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if _, err := readAuthoritativeClosure(ctx, tx, next.Source, next, now); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	staged, err := cloneStrict(next)
	if err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if s.consumeStageFailure() {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review Binding association staged failure")
	}
	if err := advanceAssociation(ctx, tx, expected, staged); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if s.consumeLostReply() {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Review Binding association CAS reply loss")
	}
	return cloneStrict(staged)
}

func loadProjectionHistory(ctx context.Context, source queryRower, tenant core.TenantID, expected ports.ReviewBindingProjectionRefV1) (ports.ReviewBindingCurrentProjectionV1, error) {
	var digest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT digest,canonical_json FROM runtime_review_binding_projection_history WHERE tenant_id=? AND id=? AND revision=?`, string(tenant), expected.ID, expected.Revision).Scan(&digest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review Binding projection history is absent")
	}
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, mapDBError(ctx, err, false)
	}
	projection, err := decodeRow[ports.ReviewBindingCurrentProjectionV1](payload, digest, "ReviewBindingCurrentProjectionV1")
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	if projection.Subject.TenantID != tenant || projection.Ref != expected || projection.Validate() != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding projection history tenant or exact coordinates drifted")
	}
	return projection.CloneV1(), nil
}

func loadProjectionCurrentRef(ctx context.Context, source queryRower, tenant core.TenantID, id string) (ports.ReviewBindingProjectionRefV1, core.Revision, error) {
	var revision, highest uint64
	var digest string
	err := source.QueryRowContext(ctx, `SELECT revision,digest,highest_revision FROM runtime_review_binding_projection_current WHERE tenant_id=? AND id=?`, string(tenant), id).Scan(&revision, &digest, &highest)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.ReviewBindingProjectionRefV1{}, 0, core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review Binding projection current index is absent")
	}
	if err != nil {
		return ports.ReviewBindingProjectionRefV1{}, 0, mapDBError(ctx, err, false)
	}
	return ports.ReviewBindingProjectionRefV1{ID: id, Revision: core.Revision(revision), Digest: core.Digest(digest)}, core.Revision(highest), nil
}

func insertProjectionHistory(ctx context.Context, tx *sql.Tx, projection ports.ReviewBindingCurrentProjectionV1) error {
	payload, err := marshalStrict(projection)
	if err != nil {
		return err
	}
	digest, err := digestRow("ReviewBindingCurrentProjectionV1", projection)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_review_binding_projection_history(tenant_id,id,revision,digest,canonical_json) VALUES(?,?,?,?,?)`, string(projection.Subject.TenantID), projection.Ref.ID, projection.Ref.Revision, string(digest), payload)
	return mapDBError(ctx, err, true)
}

func insertProjectionCurrent(ctx context.Context, tx *sql.Tx, projection ports.ReviewBindingCurrentProjectionV1) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO runtime_review_binding_projection_current(tenant_id,id,revision,digest,highest_revision) VALUES(?,?,?,?,?)`, string(projection.Subject.TenantID), projection.Ref.ID, projection.Ref.Revision, string(projection.Ref.Digest), projection.Ref.Revision)
	return mapDBError(ctx, err, true)
}

func advanceProjectionCurrent(ctx context.Context, tx *sql.Tx, expected, next ports.ReviewBindingCurrentProjectionV1) error {
	result, err := tx.ExecContext(ctx, `UPDATE runtime_review_binding_projection_current SET revision=?,digest=?,highest_revision=? WHERE tenant_id=? AND id=? AND revision=? AND digest=? AND highest_revision=?`, next.Ref.Revision, string(next.Ref.Digest), next.Ref.Revision, string(next.Subject.TenantID), expected.Ref.ID, expected.Ref.Revision, string(expected.Ref.Digest), expected.Ref.Revision)
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Binding projection CAS lost current/highest precondition")
	}
	return nil
}

func loadReceipt(ctx context.Context, source queryRower, expected ports.ReviewBindingProjectionPublishRefV1) (ports.ReviewBindingProjectionPublishReceiptV1, core.TenantID, error) {
	var tenant, digest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT tenant_id,digest,canonical_json FROM runtime_review_binding_publish_receipts WHERE publish_id=?`, expected.ID).Scan(&tenant, &digest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, "", core.NewError(core.ErrorNotFound, core.ReasonOwnerMissing, "Review Binding Publish receipt is absent")
	}
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, "", mapDBError(ctx, err, false)
	}
	receipt, err := decodeRow[ports.ReviewBindingProjectionPublishReceiptV1](payload, digest, "ReviewBindingProjectionPublishReceiptV1")
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, "", err
	}
	if receipt.PublishRef != expected || receipt.Validate() != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, "", core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding Publish receipt drifted")
	}
	return receipt, core.TenantID(tenant), nil
}

func insertReceipt(ctx context.Context, tx *sql.Tx, tenant core.TenantID, receipt ports.ReviewBindingProjectionPublishReceiptV1) error {
	payload, err := marshalStrict(receipt)
	if err != nil {
		return err
	}
	digest, err := digestRow("ReviewBindingProjectionPublishReceiptV1", receipt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_review_binding_publish_receipts(publish_id,tenant_id,digest,canonical_json) VALUES(?,?,?,?)`, receipt.PublishRef.ID, string(tenant), string(digest), payload)
	return mapDBError(ctx, err, true)
}

func (s *Store) ResolveCurrentReviewBindingV1(ctx context.Context, request ports.ResolveReviewBindingCurrentRequestV1) (ports.ReviewBindingProjectionRefV1, error) {
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingProjectionRefV1{}, err
	}
	id, err := ports.DeriveReviewBindingProjectionIDV1(ports.ReviewBindingProjectionIdentityInputV1{Source: request.Source, Subject: request.Subject})
	if err != nil {
		return ports.ReviewBindingProjectionRefV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewBindingProjectionRefV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	ref, highest, err := loadProjectionCurrentRef(ctx, tx, request.Subject.TenantID, id)
	if err != nil {
		return ports.ReviewBindingProjectionRefV1{}, err
	}
	if highest != ref.Revision {
		return ports.ReviewBindingProjectionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Binding current/highest index drifted")
	}
	if _, err := inspectCurrentProjection(ctx, tx, request.Source, request.Subject, ref, s.clock()); err != nil {
		return ports.ReviewBindingProjectionRefV1{}, err
	}
	return ref, nil
}

func (s *Store) InspectReviewBindingProjectionV1(ctx context.Context, request ports.InspectReviewBindingProjectionRequestV1) (ports.ReviewBindingCurrentProjectionV1, error) {
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	projection, err := loadProjectionHistory(ctx, s.db, request.ExpectedSubject.TenantID, request.Ref)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	if projection.Source != request.ExpectedSource || projection.Subject != request.ExpectedSubject {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding historical Inspect coordinates drifted")
	}
	return projection.CloneV1(), nil
}

func (s *Store) InspectCurrentReviewBindingV1(ctx context.Context, request ports.InspectCurrentReviewBindingRequestV1) (ports.ReviewBindingCurrentProjectionV1, error) {
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	return inspectCurrentProjection(ctx, tx, request.ExpectedSource, request.ExpectedSubject, request.ExpectedRef, s.clock())
}

func inspectCurrentProjection(ctx context.Context, tx *sql.Tx, source ports.ReviewComponentBindingRefV2, subject ports.ReviewBindingSubjectV1, expected ports.ReviewBindingProjectionRefV1, now time.Time) (ports.ReviewBindingCurrentProjectionV1, error) {
	current, highest, err := loadProjectionCurrentRef(ctx, tx, subject.TenantID, expected.ID)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	if current != expected || highest != expected.Revision {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Binding current/highest index drifted")
	}
	projection, err := loadProjectionHistory(ctx, tx, subject.TenantID, expected)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	if projection.Source != source || projection.Subject != subject {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding current projection coordinates drifted")
	}
	association, err := loadCurrentAssociation(ctx, tx, projection.ConsumerAssociation.Ref)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	closure, err := readAuthoritativeClosure(ctx, tx, source, association, now)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	closureDigest, err := closure.DigestV1()
	if err != nil || closureDigest != projection.ClosureDigest || !reflect.DeepEqual(closure.Members, projection.Members) || closure.BindingSet.ID != projection.BindingSetID || closure.BindingSet.Revision != projection.BindingSetRevision || closure.BindingSet.Digest != projection.BindingSetDigest || closure.BindingSet.SemanticDigest != projection.BindingSetSemanticDigest || closure.BindingSet.ExpiresUnixNano != projection.BindingSetExpiresUnixNano || closure.SelectedGrant != projection.SelectedGrant || closure.ConsumerAssociation != projection.ConsumerAssociation || closure.ConsumerBinding != projection.ConsumerBinding || closure.ExpiresUnixNano != projection.ExpiresUnixNano {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding authoritative closure drifted without projection advance")
	}
	if err := projection.ValidateCurrent(expected, source, subject, now); err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	return projection.CloneV1(), nil
}

func (s *Store) InspectCurrentReviewBindingConsumerAssociationV1(ctx context.Context, expected ports.ReviewBindingConsumerAssociationRefV1) (ports.ReviewBindingConsumerAssociationCurrentProjectionV1, error) {
	if err := expected.Validate(); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	projection, err := loadCurrentAssociation(ctx, tx, expected)
	if err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	now := s.clock()
	if err := projection.ValidateCurrent(expected, projection.Consumer, projection.Source, now); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	consumer, err := inspectProviderBindingCurrent(ctx, tx, projection.Consumer, now)
	if err != nil || consumer.Ref != projection.Consumer {
		if err != nil {
			return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
		}
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding Consumer current proof drifted")
	}
	return cloneStrict(projection)
}

func (s *Store) CreateReviewBindingProjectionV1(ctx context.Context, request ports.CreateReviewBindingProjectionRequestV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if receipt, tenant, loadErr := loadReceipt(ctx, tx, request.PublishRef); loadErr == nil {
		if tenant != request.Input.Subject.TenantID {
			return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding receipt tenant drifted")
		}
		return receipt, nil
	} else if !core.HasCategory(loadErr, core.ErrorNotFound) {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, loadErr
	}
	id, err := ports.DeriveReviewBindingProjectionIDV1(ports.ReviewBindingProjectionIdentityInputV1{Source: request.Input.Source, Subject: request.Input.Subject})
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if _, _, loadErr := loadProjectionCurrentRef(ctx, tx, request.Input.Subject.TenantID, id); loadErr == nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Binding projection identity already exists")
	} else if !core.HasCategory(loadErr, core.ErrorNotFound) {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, loadErr
	}
	association, err := loadCurrentAssociation(ctx, tx, request.Input.Association)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	projection, err := buildProjection(ctx, tx, request.Input.Source, request.Input.Subject, association, 1, s.clock())
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	receipt, err := ports.SealReviewBindingProjectionPublishReceiptV1(ports.ReviewBindingProjectionPublishReceiptV1{PublishRef: request.PublishRef, Projection: projection.Ref, CurrentIndex: projection.Ref, HighestRevision: projection.Ref.Revision})
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	projection, receipt, err = stageProjectionReceipt(projection, receipt)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if s.consumeStageFailure() {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review Binding Create staged failure")
	}
	if err := insertProjectionHistory(ctx, tx, projection); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if err := insertProjectionCurrent(ctx, tx, projection); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if err := insertReceipt(ctx, tx, projection.Subject.TenantID, receipt); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if s.consumeLostReply() {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Review Binding Create reply loss")
	}
	return receipt, nil
}

func (s *Store) CompareAndSwapReviewBindingProjectionV1(ctx context.Context, request ports.CompareAndSwapReviewBindingProjectionRequestV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if receipt, tenant, loadErr := loadReceipt(ctx, tx, request.PublishRef); loadErr == nil {
		if tenant != request.Input.Subject.TenantID {
			return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding CAS receipt tenant drifted")
		}
		return receipt, nil
	} else if !core.HasCategory(loadErr, core.ErrorNotFound) {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, loadErr
	}
	current, highest, err := loadProjectionCurrentRef(ctx, tx, request.Input.Subject.TenantID, request.Input.ExpectedCurrent.ID)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if current != request.Input.ExpectedCurrent || highest != current.Revision {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Binding projection CAS precondition drifted")
	}
	previous, err := loadProjectionHistory(ctx, tx, request.Input.Subject.TenantID, current)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if previous.Source != request.Input.Source || previous.Subject != request.Input.Subject {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding projection CAS identity drifted")
	}
	if request.Input.Association != previous.ConsumerAssociation.Ref {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "association advance requires Runtime Binding Owner compound CAS")
	}
	association, err := loadCurrentAssociation(ctx, tx, request.Input.Association)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	now := s.clock()
	next, buildErr := buildProjection(ctx, tx, request.Input.Source, request.Input.Subject, association, current.Revision+1, now)
	if buildErr != nil {
		next, buildErr = buildTerminalProjection(ctx, tx, previous, request.Input.Source, request.Input.Subject, current.Revision+1, now)
		if buildErr != nil {
			return ports.ReviewBindingProjectionPublishReceiptV1{}, buildErr
		}
	}
	if next.ClosureDigest == previous.ClosureDigest && next.State == previous.State {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Review Binding projection CAS has no authoritative state change")
	}
	receipt, err := ports.SealReviewBindingProjectionPublishReceiptV1(ports.ReviewBindingProjectionPublishReceiptV1{PublishRef: request.PublishRef, Projection: next.Ref, CurrentIndex: next.Ref, HighestRevision: next.Ref.Revision})
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	next, receipt, err = stageProjectionReceipt(next, receipt)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if s.consumeStageFailure() {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review Binding CAS staged failure")
	}
	if err := insertProjectionHistory(ctx, tx, next); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if err := advanceProjectionCurrent(ctx, tx, previous, next); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if err := insertReceipt(ctx, tx, next.Subject.TenantID, receipt); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if s.consumeLostReply() {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Review Binding CAS reply loss")
	}
	return receipt, nil
}

func (s *Store) CompareAndSwapReviewBindingAssociationProjectionV1(ctx context.Context, request control.CompareAndSwapReviewBindingAssociationProjectionRequestV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	tenant := request.Projection.Input.Subject.TenantID
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if receipt, receiptTenant, loadErr := loadReceipt(ctx, tx, request.Projection.PublishRef); loadErr == nil {
		if receiptTenant != tenant {
			return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding compound receipt tenant drifted")
		}
		return receipt, nil
	} else if !core.HasCategory(loadErr, core.ErrorNotFound) {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, loadErr
	}
	previousAssociation, err := loadCurrentAssociation(ctx, tx, request.ExpectedAssociation)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if previousAssociation.Consumer != request.NextAssociation.Consumer || previousAssociation.Source != request.NextAssociation.Source || previousAssociation.CheckedUnixNano > request.NextAssociation.CheckedUnixNano {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding compound CAS changed stable Consumer or Source coordinates")
	}
	current, highest, err := loadProjectionCurrentRef(ctx, tx, tenant, request.Projection.Input.ExpectedCurrent.ID)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if current != request.Projection.Input.ExpectedCurrent || highest != current.Revision {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Binding compound projection precondition drifted")
	}
	previous, err := loadProjectionHistory(ctx, tx, tenant, current)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if previous.ConsumerAssociation.Ref != request.ExpectedAssociation || previous.Source != request.Projection.Input.Source || previous.Subject != request.Projection.Input.Subject {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding compound previous closure drifted")
	}
	now := s.clock()
	if err := request.NextAssociation.ValidateCurrent(request.NextAssociation.Ref, request.NextAssociation.Consumer, request.NextAssociation.Source, now); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	next, err := buildProjection(ctx, tx, request.Projection.Input.Source, request.Projection.Input.Subject, request.NextAssociation, current.Revision+1, now)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if next.ClosureDigest == previous.ClosureDigest {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Review Binding compound CAS has no authoritative state change")
	}
	receipt, err := ports.SealReviewBindingProjectionPublishReceiptV1(ports.ReviewBindingProjectionPublishReceiptV1{PublishRef: request.Projection.PublishRef, Projection: next.Ref, CurrentIndex: next.Ref, HighestRevision: next.Ref.Revision})
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	nextAssociation, err := cloneStrict(request.NextAssociation)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	next, receipt, err = stageProjectionReceipt(next, receipt)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if s.consumeStageFailure() {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review Binding compound staged failure")
	}
	if err := advanceAssociation(ctx, tx, request.ExpectedAssociation, nextAssociation); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if err := insertProjectionHistory(ctx, tx, next); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if err := advanceProjectionCurrent(ctx, tx, previous, next); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if err := insertReceipt(ctx, tx, tenant, receipt); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if s.consumeLostReply() {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Review Binding compound CAS reply loss")
	}
	return receipt, nil
}

func (s *Store) InspectReviewBindingProjectionPublishV1(ctx context.Context, expected ports.ReviewBindingProjectionPublishRefV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	if err := expected.Validate(); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	receipt, _, err := loadReceipt(ctx, s.db, expected)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	return receipt, nil
}

func buildProjection(ctx context.Context, source queryRower, component ports.ReviewComponentBindingRefV2, subject ports.ReviewBindingSubjectV1, association ports.ReviewBindingConsumerAssociationCurrentProjectionV1, revision core.Revision, now time.Time) (ports.ReviewBindingCurrentProjectionV1, error) {
	if now.IsZero() {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Binding projection clock is unavailable")
	}
	closure, err := readAuthoritativeClosure(ctx, source, component, association, now)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	return ports.SealReviewBindingCurrentProjectionV1(ports.ReviewBindingCurrentProjectionV1{
		Ref: ports.ReviewBindingProjectionRefV1{Revision: revision}, Source: component, Subject: subject,
		State: ports.ReviewBindingCurrentActiveV1, Current: true,
		BindingSetID: closure.BindingSet.ID, BindingSetRevision: closure.BindingSet.Revision,
		BindingSetDigest: closure.BindingSet.Digest, BindingSetSemanticDigest: closure.BindingSet.SemanticDigest,
		BindingSetExpiresUnixNano: closure.BindingSet.ExpiresUnixNano,
		Members:                   closure.Members, SelectedGrant: closure.SelectedGrant,
		ConsumerAssociation: closure.ConsumerAssociation, ConsumerBinding: closure.ConsumerBinding,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: closure.ExpiresUnixNano,
	})
}

func buildTerminalProjection(ctx context.Context, source queryRower, previous ports.ReviewBindingCurrentProjectionV1, component ports.ReviewComponentBindingRefV2, subject ports.ReviewBindingSubjectV1, revision core.Revision, now time.Time) (ports.ReviewBindingCurrentProjectionV1, error) {
	set, err := loadBindingSet(ctx, source, component.BindingSetID)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	state := ports.ReviewBindingCurrentStateV1("")
	switch {
	case set.State == control.BindingSetRevoked:
		state = ports.ReviewBindingCurrentRevokedV1
	case set.State == control.BindingSetExpired:
		state = ports.ReviewBindingCurrentExpiredV1
	case set.Revision != component.BindingSetRevision:
		state = ports.ReviewBindingCurrentSupersededV1
	default:
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Review Binding authoritative closure changed without terminal source transition")
	}
	if now.IsZero() || now.UnixNano() < previous.CheckedUnixNano || !now.Before(time.Unix(0, previous.ExpiresUnixNano)) {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Review Binding terminal transition crossed immutable TTL")
	}
	next := previous.CloneV1()
	next.Ref.Revision, next.Ref.Digest = revision, ""
	next.State, next.Current = state, false
	next.CheckedUnixNano, next.ProjectionDigest = now.UnixNano(), ""
	return ports.SealReviewBindingCurrentProjectionV1(next)
}

func readAuthoritativeClosure(ctx context.Context, source queryRower, component ports.ReviewComponentBindingRefV2, association ports.ReviewBindingConsumerAssociationCurrentProjectionV1, now time.Time) (ports.ReviewBindingAuthoritativeClosureInputV1, error) {
	if err := component.Validate(); err != nil {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, err
	}
	set, err := loadBindingSet(ctx, source, component.BindingSetID)
	if err != nil {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, err
	}
	if set.Revision != component.BindingSetRevision || set.State != control.BindingSetActive || !now.Before(time.Unix(0, set.ExpiresUnixNano)) {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Review Binding source BindingSet is not current")
	}
	setDigest, err := control.BindingSetDigestV2(set)
	if err != nil {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, err
	}
	semanticDigest, err := control.BindingSetSemanticDigestV2(set)
	if err != nil {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, err
	}
	members := make([]ports.ReviewBindingMemberCurrentRefV1, 0, len(set.Members))
	facts := make([]control.BindingFactV2, 0, len(set.Members))
	probes := make([]control.BindingCurrentProbeV2, 0, len(set.Members))
	minimum := set.ExpiresUnixNano
	selected := ports.ReviewBindingSelectedGrantRefV1{}
	selectedCount := 0
	for _, member := range set.Members {
		fact, inspectErr := loadBinding(ctx, source, member.BindingID)
		if inspectErr != nil {
			return ports.ReviewBindingAuthoritativeClosureInputV1{}, inspectErr
		}
		if fact.Validate() != nil || fact.State != control.BindingBound || fact.BindingSetID != set.ID || fact.Revision != member.BindingRevision || fact.ComponentID != member.ComponentID || fact.ManifestDigest != member.ManifestDigest || fact.Manifest.ArtifactDigest != member.ArtifactDigest {
			return ports.ReviewBindingAuthoritativeClosureInputV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding member Fact drifted from its Set")
		}
		setGrantDigest, setGrantErr := control.BindingGrantSetDigestV2(member.Grants)
		factGrantDigest, factGrantErr := control.BindingGrantSetDigestV2(fact.Grants)
		if setGrantErr != nil || factGrantErr != nil || setGrantDigest != factGrantDigest || !reflect.DeepEqual(member.Grants, fact.Grants) {
			return ports.ReviewBindingAuthoritativeClosureInputV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding Set and Fact Grant closures drifted")
		}
		factDigest, digestErr := core.CanonicalJSONDigest("praxis.runtime.binding", ports.BindingContractVersionV2, "BindingFactV2", fact)
		if digestErr != nil {
			return ports.ReviewBindingAuthoritativeClosureInputV1{}, digestErr
		}
		grantMin := fact.ExpiresUnixNano
		for _, grant := range fact.Grants {
			if grant.ExpiresUnixNano < grantMin {
				grantMin = grant.ExpiresUnixNano
			}
			if member.ComponentID == component.ComponentID && member.ManifestDigest == component.ManifestDigest && member.ArtifactDigest == component.ArtifactDigest && grant.Capability == component.Capability {
				grantDigest, digestErr := core.CanonicalJSONDigest("praxis.runtime.binding", ports.BindingContractVersionV2, "CapabilityGrantV2", grant)
				if digestErr != nil {
					return ports.ReviewBindingAuthoritativeClosureInputV1{}, digestErr
				}
				selected = ports.ReviewBindingSelectedGrantRefV1{ComponentID: member.ComponentID, BindingID: member.BindingID, BindingRevision: member.BindingRevision, Capability: grant.Capability, SetGrantDigest: grantDigest, FactGrantDigest: grantDigest, ExpiresUnixNano: grant.ExpiresUnixNano}
				selectedCount++
			}
		}
		minimum = minExpiry(minimum, fact.ExpiresUnixNano, grantMin)
		members = append(members, ports.ReviewBindingMemberCurrentRefV1{ComponentID: member.ComponentID, BindingID: member.BindingID, BindingRevision: member.BindingRevision, BindingFactDigest: factDigest, ManifestDigest: member.ManifestDigest, ArtifactDigest: member.ArtifactDigest, SetGrantSetDigest: setGrantDigest, FactGrantSetDigest: factGrantDigest, BindingFactExpiresUnixNano: fact.ExpiresUnixNano, SetGrantMinExpiresUnixNano: grantMin, FactGrantMinExpiresUnixNano: grantMin})
		facts = append(facts, fact)
		probes = append(probes, control.BindingCurrentProbeV2{ComponentID: member.ComponentID, ManifestDigest: member.ManifestDigest})
	}
	if selectedCount != 1 {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingNotCertified, "Review Binding Source capability is missing or ambiguous")
	}
	if err := control.ValidateBindingSetCurrentV2(set, facts, probes, now); err != nil {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, err
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ComponentID < members[j].ComponentID })
	consumer, err := inspectProviderBindingCurrent(ctx, source, association.Consumer, now)
	if err != nil {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, err
	}
	if err := association.ValidateCurrent(association.Ref, association.Consumer, component, now); err != nil {
		return ports.ReviewBindingAuthoritativeClosureInputV1{}, err
	}
	minimum = minExpiry(minimum, selected.ExpiresUnixNano, association.ExpiresUnixNano, consumer.ExpiresUnixNano)
	closure := ports.ReviewBindingAuthoritativeClosureInputV1{
		Source:     component,
		BindingSet: ports.ReviewBindingSetExactRefV1{ID: set.ID, Revision: set.Revision, Digest: setDigest, SemanticDigest: semanticDigest, ExpiresUnixNano: set.ExpiresUnixNano},
		Members:    members, SelectedGrant: selected, ConsumerAssociation: association, ConsumerBinding: consumer, ExpiresUnixNano: minimum,
	}
	return closure, closure.Validate()
}

func inspectProviderBindingCurrent(ctx context.Context, source queryRower, expected ports.ProviderBindingRefV2, now time.Time) (ports.ProviderBindingCurrentProjectionV2, error) {
	if err := expected.Validate(); err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	set, err := loadBindingSet(ctx, source, expected.BindingSetID)
	if err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	setDigest, err := control.BindingSetDigestV2(set)
	if err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	semanticDigest, err := control.BindingSetSemanticDigestV2(set)
	if err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	if set.Revision != expected.BindingSetRevision || set.State != control.BindingSetActive || !now.Before(time.Unix(0, set.ExpiresUnixNano)) {
		return ports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "provider BindingSet is not current")
	}
	var selectedMember control.BindingMemberV2
	found := false
	for _, member := range set.Members {
		if member.ComponentID == expected.ComponentID {
			selectedMember, found = member, true
			break
		}
	}
	if !found || selectedMember.ManifestDigest != expected.ManifestDigest || selectedMember.ArtifactDigest != expected.ArtifactDigest {
		return ports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "provider Binding member identity drifted")
	}
	selectedFact, err := loadBinding(ctx, source, selectedMember.BindingID)
	if err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	if selectedFact.State != control.BindingBound || selectedFact.BindingSetID != set.ID || selectedFact.Revision != selectedMember.BindingRevision || selectedFact.ManifestDigest != selectedMember.ManifestDigest || selectedFact.Manifest.ArtifactDigest != selectedMember.ArtifactDigest || !now.Before(time.Unix(0, selectedFact.ExpiresUnixNano)) {
		return ports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "provider Binding Fact drifted from current set")
	}
	probes := make([]control.BindingCurrentProbeV2, 0, len(set.Members))
	facts := make([]control.BindingFactV2, 0, len(set.Members))
	currentExpiry := set.ExpiresUnixNano
	for _, member := range set.Members {
		fact, inspectErr := loadBinding(ctx, source, member.BindingID)
		if inspectErr != nil {
			return ports.ProviderBindingCurrentProjectionV2{}, inspectErr
		}
		if !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
			return ports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "provider Binding member expired")
		}
		memberGrantDigest, memberErr := control.BindingGrantSetDigestV2(member.Grants)
		factGrantDigest, factErr := control.BindingGrantSetDigestV2(fact.Grants)
		if memberErr != nil || factErr != nil || memberGrantDigest != factGrantDigest || !reflect.DeepEqual(member.Grants, fact.Grants) {
			return ports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "provider Binding Grant closure drifted")
		}
		currentExpiry = minExpiry(currentExpiry, fact.ExpiresUnixNano)
		facts = append(facts, fact)
		probes = append(probes, control.BindingCurrentProbeV2{ComponentID: member.ComponentID, ManifestDigest: member.ManifestDigest})
	}
	if err := control.ValidateBindingSetCurrentV2(set, facts, probes, now); err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	var grant ports.CapabilityGrantV2
	grantFound := false
	for _, candidate := range selectedMember.Grants {
		if candidate.Capability == expected.Capability {
			grant, grantFound = candidate, true
			break
		}
	}
	if !grantFound || !now.Before(time.Unix(0, grant.ExpiresUnixNano)) {
		return ports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorForbidden, core.ReasonUnknownCapability, "provider capability Grant is missing or expired")
	}
	grantDigest, err := core.CanonicalJSONDigest("praxis.runtime.binding", ports.ProviderBindingCurrentnessContractVersionV2, "CapabilityGrantV2", grant)
	if err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	expires := minExpiry(currentExpiry, selectedFact.ExpiresUnixNano, grant.ExpiresUnixNano)
	projection, err := ports.SealProviderBindingCurrentProjectionV2(ports.ProviderBindingCurrentProjectionV2{
		ContractVersion: ports.ProviderBindingCurrentnessContractVersionV2, Ref: expected, State: ports.ProviderBindingCurrentActiveV2,
		BindingSetDigest: setDigest, BindingSetSemanticDigest: semanticDigest,
		BindingID: selectedMember.BindingID, BindingRevision: selectedMember.BindingRevision,
		GrantDigest: grantDigest, IssuedUnixNano: grant.ObservedUnixNano, ExpiresUnixNano: expires,
	})
	if err != nil {
		return ports.ProviderBindingCurrentProjectionV2{}, err
	}
	return projection, projection.ValidateCurrent(expected, now)
}

func minExpiry(first int64, rest ...int64) int64 {
	minimum := first
	for _, value := range rest {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func stageProjectionReceipt(projection ports.ReviewBindingCurrentProjectionV1, receipt ports.ReviewBindingProjectionPublishReceiptV1) (ports.ReviewBindingCurrentProjectionV1, ports.ReviewBindingProjectionPublishReceiptV1, error) {
	stagedProjection, err := cloneStrict(projection)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	stagedReceipt, err := cloneStrict(receipt)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	return stagedProjection, stagedReceipt, nil
}
