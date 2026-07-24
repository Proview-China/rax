package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func loadReviewEvidenceProjectionV1(ctx context.Context, source queryRower, ref ports.ReviewEvidenceApplicabilityRefV1) (ports.ReviewEvidenceApplicabilityProjectionV1, error) {
	var subjectDigest, storedDigest string
	var revision uint64
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT revision,subject_digest,digest,canonical_json FROM runtime_review_evidence_projection_history WHERE projection_id=? AND revision=?`, ref.ProjectionID, ref.Revision).Scan(&revision, &subjectDigest, &storedDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "Review evidence applicability historical Projection does not exist")
	}
	if err != nil {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, mapDBError(ctx, err, false)
	}
	projection, err := decodeRow[ports.ReviewEvidenceApplicabilityProjectionV1](payload, storedDigest, "ReviewEvidenceApplicabilityProjectionV1")
	if err != nil {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, err
	}
	if uint64(projection.Ref.Revision) != revision || string(projection.SubjectDigest) != subjectDigest || projection.Ref != ref || projection.Validate() != nil {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability historical Projection row drifted")
	}
	return projection, nil
}

func loadReviewEvidenceCurrentIndexV1(ctx context.Context, source queryRower, subjectDigest core.Digest) (ports.ReviewEvidenceApplicabilityCurrentIndexRefV1, error) {
	var indexID, projectionID, projectionDigest, storedDigest string
	var revision, projectionRevision, highest uint64
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT index_id,revision,projection_id,projection_revision,projection_digest,highest_revision,digest,canonical_json FROM runtime_review_evidence_current WHERE subject_digest=?`, string(subjectDigest)).Scan(&indexID, &revision, &projectionID, &projectionRevision, &projectionDigest, &highest, &storedDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.ReviewEvidenceApplicabilityCurrentIndexRefV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "Review evidence applicability Current Index does not exist")
	}
	if err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentIndexRefV1{}, mapDBError(ctx, err, false)
	}
	index, err := decodeRow[ports.ReviewEvidenceApplicabilityCurrentIndexRefV1](payload, storedDigest, "ReviewEvidenceApplicabilityCurrentIndexRefV1")
	if err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentIndexRefV1{}, err
	}
	if index.IndexID != indexID || uint64(index.Revision) != revision || index.CurrentProjection.ProjectionID != projectionID || uint64(index.CurrentProjection.Revision) != projectionRevision || string(index.CurrentProjection.Digest) != projectionDigest || uint64(index.HighestRevision) != highest || index.SubjectDigest != subjectDigest || index.Validate() != nil {
		return ports.ReviewEvidenceApplicabilityCurrentIndexRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability Current Index row drifted")
	}
	return index, nil
}

func loadReviewEvidenceReceiptV1(ctx context.Context, source queryRower, publishID string) (ports.ReviewEvidenceApplicabilityPublishReceiptV1, error) {
	var requestDigest, subjectDigest, projectionID, projectionDigest, storedDigest string
	var projectionRevision uint64
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT request_digest,subject_digest,projection_id,projection_revision,projection_digest,digest,canonical_json FROM runtime_review_evidence_publish_receipts WHERE publish_id=?`, publishID).Scan(&requestDigest, &subjectDigest, &projectionID, &projectionRevision, &projectionDigest, &storedDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "Review evidence applicability Publish receipt does not exist")
	}
	if err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, mapDBError(ctx, err, false)
	}
	receipt, err := decodeRow[ports.ReviewEvidenceApplicabilityPublishReceiptV1](payload, storedDigest, "ReviewEvidenceApplicabilityPublishReceiptV1")
	if err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if receipt.PublishID != publishID || string(receipt.RequestDigest) != requestDigest || string(receipt.Projection.SubjectDigest) != subjectDigest || receipt.Projection.ProjectionID != projectionID || uint64(receipt.Projection.Revision) != projectionRevision || string(receipt.Projection.Digest) != projectionDigest || receipt.Validate() != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability Publish receipt row drifted")
	}
	return receipt, nil
}

func (s *Store) InspectReviewEvidenceApplicabilityCurrentFactV1(ctx context.Context, subjectDigest core.Digest) (ports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	if err := contextError(ctx, "Inspect Review evidence applicability Current"); err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	if err := subjectDigest.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelSerializable})
	if err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, mapDBError(ctx, err, false)
	}
	defer func() { _ = tx.Rollback() }()
	index, err := loadReviewEvidenceCurrentIndexV1(ctx, tx, subjectDigest)
	if err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	projection, err := loadReviewEvidenceProjectionV1(ctx, tx, index.CurrentProjection)
	if err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	snapshot := ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{ContractVersion: ports.ReviewEvidenceCurrentContractVersionV1, Projection: projection, CurrentIndex: index}
	if err := snapshot.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	if err := tx.Commit(); err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, mapDBError(ctx, err, false)
	}
	return ports.CloneReviewEvidenceApplicabilityCurrentSnapshotV1(snapshot), nil
}

func (s *Store) InspectReviewEvidenceApplicabilityProjectionFactV1(ctx context.Context, ref ports.ReviewEvidenceApplicabilityRefV1) (ports.ReviewEvidenceApplicabilityProjectionV1, error) {
	if err := contextError(ctx, "Inspect Review evidence applicability Projection"); err != nil {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, err
	}
	projection, err := loadReviewEvidenceProjectionV1(ctx, s.db, ref)
	if err != nil {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, err
	}
	return ports.CloneReviewEvidenceApplicabilityProjectionV1(projection), nil
}

func (s *Store) InspectReviewEvidenceApplicabilityPublishFactV1(ctx context.Context, publishID string) (ports.ReviewEvidenceApplicabilityPublishReceiptV1, error) {
	if err := contextError(ctx, "Inspect Review evidence applicability Publish"); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if strings.TrimSpace(publishID) == "" {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review evidence applicability PublishID is required")
	}
	receipt, err := loadReviewEvidenceReceiptV1(ctx, s.db, publishID)
	if err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	return ports.CloneReviewEvidenceApplicabilityPublishReceiptV1(receipt), nil
}

func (s *Store) PublishReviewEvidenceApplicabilityFactV1(ctx context.Context, request ports.PublishReviewEvidenceApplicabilityRequestV1, receipt ports.ReviewEvidenceApplicabilityPublishReceiptV1) (ports.ReviewEvidenceApplicabilityPublishReceiptV1, error) {
	if err := contextError(ctx, "Publish Review evidence applicability"); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if err := receipt.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if receipt.PublishID != string(request.RequestDigest) || receipt.RequestDigest != request.RequestDigest || receipt.Projection != request.Projection.Ref || !reflect.DeepEqual(receipt.CurrentIndex, request.NextCurrentIndex) {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability receipt drifted from Publish request")
	}
	stagedProjection, err := cloneStrict(request.Projection)
	if err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	stagedIndex, err := cloneStrict(request.NextCurrentIndex)
	if err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	stagedReceipt, err := cloneStrict(receipt)
	if err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, loadErr := loadReviewEvidenceReceiptV1(ctx, tx, receipt.PublishID); loadErr == nil {
		if existing.RequestDigest == request.RequestDigest && existing.Projection == request.Projection.Ref && reflect.DeepEqual(existing.CurrentIndex, request.NextCurrentIndex) {
			return ports.CloneReviewEvidenceApplicabilityPublishReceiptV1(existing), nil
		}
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review evidence applicability PublishID binds different content")
	} else if !core.HasCategory(loadErr, core.ErrorNotFound) {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, loadErr
	}
	current, loadErr := loadReviewEvidenceCurrentIndexV1(ctx, tx, request.Projection.SubjectDigest)
	if core.HasCategory(loadErr, core.ErrorNotFound) {
		if request.ExpectedCurrentIndex != nil || request.Projection.Ref.Revision != 1 || request.Projection.Previous != nil {
			return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review evidence applicability first Publish shape is invalid")
		}
	} else if loadErr != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, loadErr
	} else if request.ExpectedCurrentIndex == nil || !reflect.DeepEqual(current, *request.ExpectedCurrentIndex) || request.Projection.Previous == nil || current.CurrentProjection != *request.Projection.Previous || request.Projection.Ref.Revision != current.Revision+1 {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review evidence applicability Publish lost exact CAS or history")
	}
	if _, historyErr := loadReviewEvidenceProjectionV1(ctx, tx, request.Projection.Ref); historyErr == nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability immutable Projection history exists without receipt")
	} else if !core.HasCategory(historyErr, core.ErrorNotFound) {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, historyErr
	}
	if s.consumeStageFailure() {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review evidence sqlite staged failure")
	}
	if err := insertReviewEvidenceProjectionV1(ctx, tx, stagedProjection); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if current.Revision == 0 {
		err = insertReviewEvidenceCurrentV1(ctx, tx, stagedIndex)
	} else {
		err = updateReviewEvidenceCurrentV1(ctx, tx, current, stagedIndex)
	}
	if err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if err := insertReviewEvidenceReceiptV1(ctx, tx, stagedReceipt); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if s.consumeLostReply() {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Review evidence sqlite Publish reply was lost")
	}
	return ports.CloneReviewEvidenceApplicabilityPublishReceiptV1(stagedReceipt), nil
}

func insertReviewEvidenceProjectionV1(ctx context.Context, tx *sql.Tx, projection ports.ReviewEvidenceApplicabilityProjectionV1) error {
	payload, err := marshalStrict(projection)
	if err != nil {
		return err
	}
	digest, err := digestRow("ReviewEvidenceApplicabilityProjectionV1", projection)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_review_evidence_projection_history(projection_id,revision,subject_digest,digest,canonical_json) VALUES(?,?,?,?,?)`, projection.Ref.ProjectionID, projection.Ref.Revision, string(projection.SubjectDigest), string(digest), payload)
	return mapDBError(ctx, err, true)
}

func insertReviewEvidenceCurrentV1(ctx context.Context, tx *sql.Tx, index ports.ReviewEvidenceApplicabilityCurrentIndexRefV1) error {
	payload, err := marshalStrict(index)
	if err != nil {
		return err
	}
	digest, err := digestRow("ReviewEvidenceApplicabilityCurrentIndexRefV1", index)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_review_evidence_current(subject_digest,index_id,revision,projection_id,projection_revision,projection_digest,highest_revision,digest,canonical_json) VALUES(?,?,?,?,?,?,?,?,?)`, string(index.SubjectDigest), index.IndexID, index.Revision, index.CurrentProjection.ProjectionID, index.CurrentProjection.Revision, string(index.CurrentProjection.Digest), index.HighestRevision, string(digest), payload)
	return mapDBError(ctx, err, true)
}

func updateReviewEvidenceCurrentV1(ctx context.Context, tx *sql.Tx, expected, next ports.ReviewEvidenceApplicabilityCurrentIndexRefV1) error {
	payload, err := marshalStrict(next)
	if err != nil {
		return err
	}
	nextDigest, err := digestRow("ReviewEvidenceApplicabilityCurrentIndexRefV1", next)
	if err != nil {
		return err
	}
	expectedDigest, err := digestRow("ReviewEvidenceApplicabilityCurrentIndexRefV1", expected)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE runtime_review_evidence_current SET index_id=?,revision=?,projection_id=?,projection_revision=?,projection_digest=?,highest_revision=?,digest=?,canonical_json=? WHERE subject_digest=? AND index_id=? AND revision=? AND digest=?`, next.IndexID, next.Revision, next.CurrentProjection.ProjectionID, next.CurrentProjection.Revision, string(next.CurrentProjection.Digest), next.HighestRevision, string(nextDigest), payload, string(expected.SubjectDigest), expected.IndexID, expected.Revision, string(expectedDigest))
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review evidence applicability Current Index CAS lost its full-ref precondition")
	}
	return nil
}

func insertReviewEvidenceReceiptV1(ctx context.Context, tx *sql.Tx, receipt ports.ReviewEvidenceApplicabilityPublishReceiptV1) error {
	payload, err := marshalStrict(receipt)
	if err != nil {
		return err
	}
	digest, err := digestRow("ReviewEvidenceApplicabilityPublishReceiptV1", receipt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_review_evidence_publish_receipts(publish_id,request_digest,subject_digest,projection_id,projection_revision,projection_digest,digest,canonical_json) VALUES(?,?,?,?,?,?,?,?)`, receipt.PublishID, string(receipt.RequestDigest), string(receipt.Projection.SubjectDigest), receipt.Projection.ProjectionID, receipt.Projection.Revision, string(receipt.Projection.Digest), string(digest), payload)
	return mapDBError(ctx, err, true)
}

var _ control.ReviewEvidenceApplicabilityFactPortV1 = (*Store)(nil)
