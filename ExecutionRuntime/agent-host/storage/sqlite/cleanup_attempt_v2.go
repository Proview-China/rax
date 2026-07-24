package sqlite

import (
	"context"
	"database/sql"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

const cleanupAttemptRowV2 = "CleanupAttemptV2"

func (s *Store) CreateCleanupAttemptV2(ctx context.Context, desired contract.CleanupAttemptV2) (contract.CleanupAttemptV2, error) {
	if err := s.writeReady(ctx); err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	if err := desired.Validate(); err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	payload, rowDigest, err := encodeRow(cleanupAttemptRowV2, desired)
	if err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	tx, err := s.beginMutation(ctx)
	if err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = validateCleanupAttemptClaim(ctx, tx, desired); err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_host_cleanup_attempts_v2(attempt_id,host_id,start_id,revision,digest,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?)`, desired.AttemptID, desired.HostID, desired.StartID, desired.Revision, string(desired.Digest), rowDigest, payload); err != nil {
		return contract.CleanupAttemptV2{}, mapDBError(ctx, err, true)
	}
	actual, err := inspectCleanupAttemptV2(ctx, tx, desired.AttemptID)
	if err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	if actual.Digest != desired.Digest {
		return contract.CleanupAttemptV2{}, contract.NewError(contract.ErrorConflict, "cleanup_attempt_conflict", "cleanup attempt already exists with different content")
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	return actual, nil
}

func (s *Store) CompareAndSwapCleanupAttemptV2(ctx context.Context, expected contract.ExactRefV1, next contract.CleanupAttemptV2) (contract.CleanupAttemptV2, error) {
	if err := s.writeReady(ctx); err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	if err := next.Validate(); err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	payload, rowDigest, err := encodeRow(cleanupAttemptRowV2, next)
	if err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	tx, err := s.beginMutation(ctx)
	if err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := inspectCleanupAttemptV2(ctx, tx, next.AttemptID)
	if err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	if cleanupAttemptRefV2(current) != expected {
		return contract.CleanupAttemptV2{}, contract.NewError(contract.ErrorConflict, "cleanup_attempt_cas_conflict", "cleanup attempt expected Ref drifted")
	}
	if err = contract.ValidateCleanupAttemptSuccessorV2(current, next); err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	if err = validateCleanupAttemptClaim(ctx, tx, next); err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE agent_host_cleanup_attempts_v2 SET revision=?,digest=?,row_digest=?,canonical_json=? WHERE attempt_id=? AND revision=? AND digest=?`, next.Revision, string(next.Digest), rowDigest, payload, next.AttemptID, current.Revision, string(current.Digest))
	if err != nil {
		return contract.CleanupAttemptV2{}, mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return contract.CleanupAttemptV2{}, mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return contract.CleanupAttemptV2{}, contract.NewError(contract.ErrorConflict, "cleanup_attempt_cas_conflict", "cleanup attempt changed concurrently")
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	return next, nil
}

func (s *Store) InspectCleanupAttemptV2(ctx context.Context, attemptID string) (contract.CleanupAttemptV2, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	if err := contract.ValidateIdentifierV1("attempt id", attemptID); err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	return inspectCleanupAttemptV2(ctx, s.db, attemptID)
}

func inspectCleanupAttemptV2(ctx context.Context, query queryRowContext, attemptID string) (contract.CleanupAttemptV2, error) {
	var payload []byte
	var rowDigest string
	err := query.QueryRowContext(ctx, `SELECT canonical_json,row_digest FROM agent_host_cleanup_attempts_v2 WHERE attempt_id=?`, attemptID).Scan(&payload, &rowDigest)
	if err != nil {
		if err == sql.ErrNoRows {
			return contract.CleanupAttemptV2{}, contract.NewError(contract.ErrorNotFound, "cleanup_attempt_missing", "cleanup attempt does not exist")
		}
		return contract.CleanupAttemptV2{}, mapDBError(ctx, err, false)
	}
	value, err := decodeRow[contract.CleanupAttemptV2](payload, rowDigest, cleanupAttemptRowV2)
	if err != nil {
		return contract.CleanupAttemptV2{}, err
	}
	if value.AttemptID != attemptID || value.Validate() != nil {
		return contract.CleanupAttemptV2{}, contract.NewError(contract.ErrorConflict, "cleanup_attempt_row_drift", "cleanup attempt row identity drifted")
	}
	return value, nil
}

func validateCleanupAttemptClaim(ctx context.Context, query queryRowContext, attempt contract.CleanupAttemptV2) error {
	if _, err := inspectHostStartClaim(ctx, query, attempt.HostID, attempt.StartID); err != nil {
		if contract.HasCode(err, contract.ErrorNotFound) {
			return contract.NewError(contract.ErrorPrecondition, "host_start_claim_missing", "cleanup attempt requires its permanent HostStart Claim")
		}
		return err
	}
	return nil
}

func cleanupAttemptRefV2(value contract.CleanupAttemptV2) contract.ExactRefV1 {
	return contract.ExactRefV1{Kind: "praxis.agent-host/cleanup-attempt-v2", ID: value.AttemptID, Revision: value.Revision, Digest: value.Digest}
}
