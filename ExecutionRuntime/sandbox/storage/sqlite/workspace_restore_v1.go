package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func (s *Store) CreateWorkspaceRestoreAttemptV1(ctx context.Context, value contract.WorkspaceRestoreAttemptV1) (bool, error) {
	if err := value.ValidateShape(); err != nil || value.Meta.Revision != 1 || value.State != contract.WorkspaceRestoreAttemptPreparedV1 {
		return false, errors.New("invalid initial workspace restore attempt")
	}
	body, err := encode(value)
	if err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	existing, err := readWorkspaceRestoreCurrentV1(ctx, tx, value.StableKeyDigest)
	if err == nil {
		if existing.ExactRef() == value.ExactRef() {
			return false, nil
		}
		return false, ports.ErrConflict
	}
	if !errors.Is(err, ports.ErrNotFound) {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_restore_attempt_history(stable_digest,attempt_id,revision,digest,body) VALUES(?,?,?,?,?)`, value.StableKeyDigest, value.Meta.ID, value.Meta.Revision, value.Meta.Digest, body); err != nil {
		return false, classifyWrite(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_restore_attempt_current(stable_digest,attempt_id,revision,digest,body) VALUES(?,?,?,?,?)`, value.StableKeyDigest, value.Meta.ID, value.Meta.Revision, value.Meta.Digest, body); err != nil {
		return false, classifyWrite(err)
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) CASWorkspaceRestoreAttemptV1(ctx context.Context, expected contract.SnapshotArtifactExactRefV2, next contract.WorkspaceRestoreAttemptV1) (bool, error) {
	if err := next.ValidateShape(); err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	current, err := readWorkspaceRestoreCurrentV1(ctx, tx, next.StableKeyDigest)
	if err != nil {
		return false, err
	}
	if current.ExactRef() != expected {
		if current.ExactRef() == next.ExactRef() {
			return false, nil
		}
		return false, ports.ErrConflict
	}
	if err := validateWorkspaceRestoreTransitionSQLiteV1(current, next, false); err != nil {
		return false, err
	}
	if err := writeWorkspaceRestoreAttemptCASV1(ctx, tx, current, next); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) CommitWorkspaceRestoreStageV1(ctx context.Context, expected contract.SnapshotArtifactExactRefV2, next contract.WorkspaceRestoreAttemptV1, fact contract.WorkspaceRestoreStageFactV1) (bool, error) {
	if err := next.ValidateShape(); err != nil || fact.ValidateShape() != nil || next.StageFactRef == nil || *next.StageFactRef != fact.ExactRef() || next.ProviderStageAttemptRef == nil || fact.AttemptRef != *next.ProviderStageAttemptRef || next.RootRef == nil || *next.RootRef != fact.RootRef {
		return false, errors.New("invalid workspace restore final commit closure")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	current, err := readWorkspaceRestoreCurrentV1(ctx, tx, next.StableKeyDigest)
	if err != nil {
		return false, err
	}
	if current.ExactRef() != expected {
		if current.ExactRef() == next.ExactRef() {
			stored, inspectErr := readWorkspaceRestoreStageFactV1(ctx, tx, fact.ExactRef())
			if inspectErr == nil && stored.ExactRef() == fact.ExactRef() {
				return false, nil
			}
		}
		return false, ports.ErrConflict
	}
	if err := validateWorkspaceRestoreTransitionSQLiteV1(current, next, true); err != nil {
		return false, err
	}
	factBody, err := encode(fact)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_restore_stage_facts(fact_id,revision,digest,body) VALUES(?,?,?,?)`, fact.Meta.ID, fact.Meta.Revision, fact.Meta.Digest, factBody); err != nil {
		return false, classifyWrite(err)
	}
	if err := writeWorkspaceRestoreAttemptCASV1(ctx, tx, current, next); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) InspectWorkspaceRestoreAttemptByStableKeyV1(ctx context.Context, stable string) (contract.WorkspaceRestoreAttemptV1, error) {
	value, err := readWorkspaceRestoreCurrentV1(ctx, s.db, stable)
	if err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, err
	}
	if err := value.ValidateShape(); err != nil || value.StableKeyDigest != stable {
		return contract.WorkspaceRestoreAttemptV1{}, fmt.Errorf("%w: stored workspace restore current drifted", ports.ErrConflict)
	}
	return value.Clone(), nil
}

func (s *Store) InspectWorkspaceRestoreAttemptV1(ctx context.Context, ref contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreAttemptV1, error) {
	var body []byte
	err := s.db.QueryRowContext(ctx, `SELECT body FROM workspace_restore_attempt_history WHERE attempt_id=? AND revision=? AND digest=?`, ref.ID, ref.Revision, ref.Digest).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.WorkspaceRestoreAttemptV1{}, ports.ErrNotFound
	}
	if err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, err
	}
	var value contract.WorkspaceRestoreAttemptV1
	if err := decode(body, &value); err != nil || value.ValidateShape() != nil || value.ExactRef() != ref {
		return contract.WorkspaceRestoreAttemptV1{}, fmt.Errorf("%w: stored workspace restore attempt drifted", ports.ErrConflict)
	}
	return value.Clone(), nil
}

func (s *Store) InspectWorkspaceRestoreStageFactV1(ctx context.Context, ref contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreStageFactV1, error) {
	value, err := readWorkspaceRestoreStageFactV1(ctx, s.db, ref)
	if err != nil {
		return contract.WorkspaceRestoreStageFactV1{}, err
	}
	if err := value.ValidateShape(); err != nil || value.ExactRef() != ref {
		return contract.WorkspaceRestoreStageFactV1{}, fmt.Errorf("%w: stored workspace restore stage fact drifted", ports.ErrConflict)
	}
	return value.Clone(), nil
}

func readWorkspaceRestoreCurrentV1(ctx context.Context, db queryer, stable string) (contract.WorkspaceRestoreAttemptV1, error) {
	var body []byte
	if err := db.QueryRowContext(ctx, `SELECT body FROM workspace_restore_attempt_current WHERE stable_digest=?`, stable).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceRestoreAttemptV1{}, ports.ErrNotFound
		}
		return contract.WorkspaceRestoreAttemptV1{}, err
	}
	var value contract.WorkspaceRestoreAttemptV1
	if err := decode(body, &value); err != nil {
		return contract.WorkspaceRestoreAttemptV1{}, err
	}
	return value, nil
}

func readWorkspaceRestoreStageFactV1(ctx context.Context, db queryer, ref contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreStageFactV1, error) {
	var body []byte
	if err := db.QueryRowContext(ctx, `SELECT body FROM workspace_restore_stage_facts WHERE fact_id=? AND revision=? AND digest=?`, ref.ID, ref.Revision, ref.Digest).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceRestoreStageFactV1{}, ports.ErrNotFound
		}
		return contract.WorkspaceRestoreStageFactV1{}, err
	}
	var value contract.WorkspaceRestoreStageFactV1
	if err := decode(body, &value); err != nil {
		return contract.WorkspaceRestoreStageFactV1{}, err
	}
	return value, nil
}

func writeWorkspaceRestoreAttemptCASV1(ctx context.Context, tx *sql.Tx, current, next contract.WorkspaceRestoreAttemptV1) error {
	body, err := encode(next)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_restore_attempt_history(stable_digest,attempt_id,revision,digest,body) VALUES(?,?,?,?,?)`, next.StableKeyDigest, next.Meta.ID, next.Meta.Revision, next.Meta.Digest, body); err != nil {
		return classifyWrite(err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE workspace_restore_attempt_current SET attempt_id=?,revision=?,digest=?,body=? WHERE stable_digest=? AND attempt_id=? AND revision=? AND digest=?`, next.Meta.ID, next.Meta.Revision, next.Meta.Digest, body, next.StableKeyDigest, current.Meta.ID, current.Meta.Revision, current.Meta.Digest)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ports.ErrConflict
	}
	return nil
}

func validateWorkspaceRestoreTransitionSQLiteV1(current, next contract.WorkspaceRestoreAttemptV1, final bool) error {
	if current.Meta.ID != next.Meta.ID || next.Meta.Revision != current.Meta.Revision+1 || current.Meta.CreatedUnixNano != next.Meta.CreatedUnixNano || current.Meta.ExpiresUnixNano != next.Meta.ExpiresUnixNano || next.Meta.UpdatedUnixNano < current.Meta.UpdatedUnixNano || current.StableKeyDigest != next.StableKeyDigest || current.Request != next.Request || current.BundleProjectionDigest != next.BundleProjectionDigest || current.BundleDigest != next.BundleDigest {
		return ports.ErrConflict
	}
	if final {
		if (next.State != contract.WorkspaceRestoreAttemptStagedV1 && next.State != contract.WorkspaceRestoreAttemptPartialV1) || !sameWorkspaceRestoreGovernanceSQLiteV1(current, next) || next.ProviderStageAttemptRef == nil {
			return ports.ErrConflict
		}
		if current.State == contract.WorkspaceRestoreAttemptInvocationV1 && *next.ProviderStageAttemptRef != current.ExactRef() {
			return ports.ErrConflict
		}
		if current.State == contract.WorkspaceRestoreAttemptReconcileRequiredV1 && (current.ProviderStageAttemptRef == nil || *next.ProviderStageAttemptRef != *current.ProviderStageAttemptRef) {
			return ports.ErrConflict
		}
		if current.State != contract.WorkspaceRestoreAttemptInvocationV1 && current.State != contract.WorkspaceRestoreAttemptReconcileRequiredV1 {
			return ports.ErrConflict
		}
		return nil
	}
	switch current.State {
	case contract.WorkspaceRestoreAttemptPreparedV1:
		if next.State != contract.WorkspaceRestoreAttemptGovernedV1 || current.Governance != nil || current.GovernanceProjectionDigest != "" || next.Governance == nil || next.ProviderStageAttemptRef != nil {
			return ports.ErrConflict
		}
	case contract.WorkspaceRestoreAttemptGovernedV1:
		if next.State != contract.WorkspaceRestoreAttemptInvocationV1 || !sameWorkspaceRestoreGovernanceSQLiteV1(current, next) || next.ProviderStageAttemptRef != nil {
			return ports.ErrConflict
		}
	case contract.WorkspaceRestoreAttemptInvocationV1:
		if next.State != contract.WorkspaceRestoreAttemptReconcileRequiredV1 || !sameWorkspaceRestoreGovernanceSQLiteV1(current, next) || next.ProviderStageAttemptRef == nil || *next.ProviderStageAttemptRef != current.ExactRef() {
			return ports.ErrConflict
		}
	default:
		return ports.ErrConflict
	}
	return nil
}

func sameWorkspaceRestoreGovernanceSQLiteV1(current, next contract.WorkspaceRestoreAttemptV1) bool {
	return current.GovernanceProjectionDigest == next.GovernanceProjectionDigest && current.Governance != nil && next.Governance != nil && *current.Governance == *next.Governance
}

var _ ports.WorkspaceRestoreStoreV1 = (*Store)(nil)
