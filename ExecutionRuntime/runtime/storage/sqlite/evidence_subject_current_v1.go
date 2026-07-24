package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func loadEvidenceSubjectProjectionV1(ctx context.Context, source queryRower, ref ports.EvidenceSubjectProjectionRefV1) (ports.EvidenceSubjectCurrentProjectionV1, error) {
	var subjectDigest, storedDigest string
	var revision uint64
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT revision,subject_digest,digest,canonical_json FROM runtime_evidence_subject_projection_history WHERE projection_id=? AND revision=?`, ref.ProjectionID, ref.Revision).Scan(&revision, &subjectDigest, &storedDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.EvidenceSubjectCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "Evidence subject historical Projection does not exist")
	}
	if err != nil {
		return ports.EvidenceSubjectCurrentProjectionV1{}, mapDBError(ctx, err, false)
	}
	projection, err := decodeRow[ports.EvidenceSubjectCurrentProjectionV1](payload, storedDigest, "EvidenceSubjectCurrentProjectionV1")
	if err != nil {
		return ports.EvidenceSubjectCurrentProjectionV1{}, err
	}
	if uint64(projection.Ref.Revision) != revision || string(projection.SubjectKeyDigest) != subjectDigest || projection.Ref != ref || projection.Validate() != nil {
		return ports.EvidenceSubjectCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence subject historical Projection row drifted")
	}
	return projection, nil
}

func loadEvidenceSubjectCurrentIndexV1(ctx context.Context, source queryRower, subjectDigest core.Digest) (ports.EvidenceSubjectCurrentIndexRefV1, error) {
	var indexID, projectionID, projectionDigest, storedDigest string
	var revision, projectionRevision, ownerWatermark uint64
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT index_id,revision,projection_id,projection_revision,projection_digest,owner_watermark,digest,canonical_json FROM runtime_evidence_subject_current WHERE subject_digest=?`, string(subjectDigest)).Scan(&indexID, &revision, &projectionID, &projectionRevision, &projectionDigest, &ownerWatermark, &storedDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.EvidenceSubjectCurrentIndexRefV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "Evidence subject Current Index does not exist")
	}
	if err != nil {
		return ports.EvidenceSubjectCurrentIndexRefV1{}, mapDBError(ctx, err, false)
	}
	index, err := decodeRow[ports.EvidenceSubjectCurrentIndexRefV1](payload, storedDigest, "EvidenceSubjectCurrentIndexRefV1")
	if err != nil {
		return ports.EvidenceSubjectCurrentIndexRefV1{}, err
	}
	if index.IndexID != indexID || uint64(index.Revision) != revision || index.CurrentProjection.ProjectionID != projectionID || uint64(index.CurrentProjection.Revision) != projectionRevision || string(index.CurrentProjection.Digest) != projectionDigest || uint64(index.OwnerWatermark) != ownerWatermark || index.SubjectKeyDigest != subjectDigest || index.Validate() != nil {
		return ports.EvidenceSubjectCurrentIndexRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence subject Current Index row drifted")
	}
	return index, nil
}

func loadEvidenceSubjectMutationV1(ctx context.Context, source queryRower, mutationID string) (ports.EvidenceSubjectMutationCommitV1, error) {
	var stable, request, subject, storedDigest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT stable_key_digest,request_digest,subject_digest,digest,canonical_json FROM runtime_evidence_subject_mutation_commits WHERE mutation_id=?`, mutationID).Scan(&stable, &request, &subject, &storedDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.EvidenceSubjectMutationCommitV1{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceSourceMissing, "Evidence subject Mutation Commit does not exist")
	}
	if err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, mapDBError(ctx, err, false)
	}
	commit, err := decodeRow[ports.EvidenceSubjectMutationCommitV1](payload, storedDigest, "EvidenceSubjectMutationCommitV1")
	if err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	if commit.Key.MutationID != mutationID || string(commit.Key.StableKeyDigest) != stable || string(commit.Key.RequestDigest) != request || string(commit.Key.SubjectKeyDigest) != subject || commit.Validate() != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence subject Mutation Commit row drifted")
	}
	return commit, nil
}

func (s *Store) InspectEvidenceSubjectProjectionFactV1(ctx context.Context, ref ports.EvidenceSubjectProjectionRefV1) (ports.EvidenceSubjectCurrentProjectionV1, error) {
	if err := contextError(ctx, "Inspect Evidence subject Projection"); err != nil {
		return ports.EvidenceSubjectCurrentProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.EvidenceSubjectCurrentProjectionV1{}, err
	}
	projection, err := loadEvidenceSubjectProjectionV1(ctx, s.db, ref)
	if err != nil {
		return ports.EvidenceSubjectCurrentProjectionV1{}, err
	}
	return cloneStrict(projection)
}

func (s *Store) InspectEvidenceSubjectCurrentIndexV1(ctx context.Context, subjectDigest core.Digest) (ports.EvidenceSubjectCurrentIndexRefV1, error) {
	if err := contextError(ctx, "Inspect Evidence subject Current Index"); err != nil {
		return ports.EvidenceSubjectCurrentIndexRefV1{}, err
	}
	if err := subjectDigest.Validate(); err != nil {
		return ports.EvidenceSubjectCurrentIndexRefV1{}, err
	}
	index, err := loadEvidenceSubjectCurrentIndexV1(ctx, s.db, subjectDigest)
	if err != nil {
		return ports.EvidenceSubjectCurrentIndexRefV1{}, err
	}
	return cloneStrict(index)
}

func (s *Store) InspectEvidenceSubjectMutationV1(ctx context.Context, key ports.EvidenceSubjectMutationKeyV1) (ports.EvidenceSubjectMutationCommitV1, error) {
	if err := contextError(ctx, "Inspect Evidence subject Mutation"); err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	if err := key.Validate(); err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	commit, err := loadEvidenceSubjectMutationV1(ctx, s.db, key.MutationID)
	if err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	if !reflect.DeepEqual(commit.Key, key) {
		return ports.EvidenceSubjectMutationCommitV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Evidence subject Mutation key binds different content")
	}
	return cloneStrict(commit)
}

func (s *Store) PublishEvidenceSubjectMutationV1(ctx context.Context, bundle ports.EvidenceSubjectMutationCommitV1, projection ports.EvidenceSubjectCurrentProjectionV1, index ports.EvidenceSubjectCurrentIndexRefV1) (ports.EvidenceSubjectMutationCommitV1, error) {
	if err := contextError(ctx, "Publish Evidence subject Mutation"); err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	if err := bundle.Validate(); err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	if err := projection.Validate(); err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	if err := index.Validate(); err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	if bundle.NewProjection != projection.Ref || !reflect.DeepEqual(bundle.NewIndex, index) || index.CurrentProjection != projection.Ref {
		return ports.EvidenceSubjectMutationCommitV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence subject atomic bundle relationships drifted")
	}
	stagedCommit, err := cloneStrict(bundle)
	if err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	stagedProjection, err := cloneStrict(projection)
	if err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	stagedIndex, err := cloneStrict(index)
	if err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, loadErr := loadEvidenceSubjectMutationV1(ctx, tx, bundle.Key.MutationID); loadErr == nil {
		if existing.CommitDigest == bundle.CommitDigest && reflect.DeepEqual(existing.Key, bundle.Key) {
			return cloneStrict(existing)
		}
		return ports.EvidenceSubjectMutationCommitV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Evidence subject Mutation ID binds different content")
	} else if !core.HasCategory(loadErr, core.ErrorNotFound) {
		return ports.EvidenceSubjectMutationCommitV1{}, loadErr
	}
	current, loadErr := loadEvidenceSubjectCurrentIndexV1(ctx, tx, index.SubjectKeyDigest)
	if core.HasCategory(loadErr, core.ErrorNotFound) {
		if bundle.ExpectedPreviousIndex != nil || bundle.ExpectedPreviousProjection != nil || index.Revision != 1 || index.PreviousProjection != nil || projection.Ref.Revision != 1 || projection.PreviousProjection != nil {
			return ports.EvidenceSubjectMutationCommitV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Evidence subject first publish shape is invalid")
		}
	} else if loadErr != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, loadErr
	} else if bundle.ExpectedPreviousIndex == nil || bundle.ExpectedPreviousProjection == nil || !reflect.DeepEqual(current, *bundle.ExpectedPreviousIndex) || current.CurrentProjection != *bundle.ExpectedPreviousProjection || index.Revision != current.Revision+1 || projection.Ref.Revision != current.Revision+1 || index.PreviousProjection == nil || *index.PreviousProjection != current.CurrentProjection || projection.PreviousProjection == nil || *projection.PreviousProjection != current.CurrentProjection {
		return ports.EvidenceSubjectMutationCommitV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Evidence subject publish lost exact CAS or history")
	}
	if _, historyErr := loadEvidenceSubjectProjectionV1(ctx, tx, projection.Ref); historyErr == nil {
		return ports.EvidenceSubjectMutationCommitV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence subject immutable Projection history already exists without its Commit")
	} else if !core.HasCategory(historyErr, core.ErrorNotFound) {
		return ports.EvidenceSubjectMutationCommitV1{}, historyErr
	}
	if s.consumeStageFailure() {
		return ports.EvidenceSubjectMutationCommitV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Evidence subject sqlite staged failure")
	}
	if err := insertEvidenceSubjectProjectionV1(ctx, tx, stagedProjection); err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	if current.Revision == 0 {
		err = insertEvidenceSubjectCurrentV1(ctx, tx, stagedIndex)
	} else {
		err = updateEvidenceSubjectCurrentV1(ctx, tx, current, stagedIndex)
	}
	if err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	if err := insertEvidenceSubjectMutationV1(ctx, tx, stagedCommit); err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.EvidenceSubjectMutationCommitV1{}, err
	}
	if s.consumeLostReply() {
		return ports.EvidenceSubjectMutationCommitV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Evidence subject sqlite publish reply was lost")
	}
	return cloneStrict(stagedCommit)
}

func insertEvidenceSubjectProjectionV1(ctx context.Context, tx *sql.Tx, projection ports.EvidenceSubjectCurrentProjectionV1) error {
	payload, err := marshalStrict(projection)
	if err != nil {
		return err
	}
	digest, err := digestRow("EvidenceSubjectCurrentProjectionV1", projection)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_evidence_subject_projection_history(projection_id,revision,subject_digest,digest,canonical_json) VALUES(?,?,?,?,?)`, projection.Ref.ProjectionID, projection.Ref.Revision, string(projection.SubjectKeyDigest), string(digest), payload)
	return mapDBError(ctx, err, true)
}

func insertEvidenceSubjectCurrentV1(ctx context.Context, tx *sql.Tx, index ports.EvidenceSubjectCurrentIndexRefV1) error {
	payload, err := marshalStrict(index)
	if err != nil {
		return err
	}
	digest, err := digestRow("EvidenceSubjectCurrentIndexRefV1", index)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_evidence_subject_current(subject_digest,index_id,revision,projection_id,projection_revision,projection_digest,owner_watermark,digest,canonical_json) VALUES(?,?,?,?,?,?,?,?,?)`, string(index.SubjectKeyDigest), index.IndexID, index.Revision, index.CurrentProjection.ProjectionID, index.CurrentProjection.Revision, string(index.CurrentProjection.Digest), index.OwnerWatermark, string(digest), payload)
	return mapDBError(ctx, err, true)
}

func updateEvidenceSubjectCurrentV1(ctx context.Context, tx *sql.Tx, expected, next ports.EvidenceSubjectCurrentIndexRefV1) error {
	payload, err := marshalStrict(next)
	if err != nil {
		return err
	}
	digest, err := digestRow("EvidenceSubjectCurrentIndexRefV1", next)
	if err != nil {
		return err
	}
	expectedDigest, err := digestRow("EvidenceSubjectCurrentIndexRefV1", expected)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE runtime_evidence_subject_current SET index_id=?,revision=?,projection_id=?,projection_revision=?,projection_digest=?,owner_watermark=?,digest=?,canonical_json=? WHERE subject_digest=? AND index_id=? AND revision=? AND digest=?`, next.IndexID, next.Revision, next.CurrentProjection.ProjectionID, next.CurrentProjection.Revision, string(next.CurrentProjection.Digest), next.OwnerWatermark, string(digest), payload, string(expected.SubjectKeyDigest), expected.IndexID, expected.Revision, string(expectedDigest))
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Evidence subject Current Index CAS lost its full-ref precondition")
	}
	return nil
}

func insertEvidenceSubjectMutationV1(ctx context.Context, tx *sql.Tx, commit ports.EvidenceSubjectMutationCommitV1) error {
	payload, err := marshalStrict(commit)
	if err != nil {
		return err
	}
	digest, err := digestRow("EvidenceSubjectMutationCommitV1", commit)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_evidence_subject_mutation_commits(mutation_id,stable_key_digest,request_digest,subject_digest,digest,canonical_json) VALUES(?,?,?,?,?,?)`, commit.Key.MutationID, string(commit.Key.StableKeyDigest), string(commit.Key.RequestDigest), string(commit.Key.SubjectKeyDigest), string(digest), payload)
	return mapDBError(ctx, err, true)
}

var _ ports.EvidenceSubjectCurrentFactPortV1 = (*Store)(nil)
