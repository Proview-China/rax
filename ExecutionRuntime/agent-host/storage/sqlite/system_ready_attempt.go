package sqlite

import (
	"context"
	"database/sql"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

const systemReadyAttemptRowV2 = "SystemReadyAttemptFactV2"

func (s *Store) CreateSystemReadyAttemptV2(ctx context.Context, desired contract.SystemReadyAttemptFactV2) (contract.SystemReadyAttemptFactV2, error) {
	if err := s.writeReady(ctx); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	if err := desired.Validate(); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	payload, rowDigest, err := encodeRow(systemReadyAttemptRowV2, desired)
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	tx, err := s.beginMutation(ctx)
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = validateSystemReadyAttemptClaim(ctx, tx, desired); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	key := desired.StepKey
	inserted, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_host_system_ready_attempts_v2(host_id,start_id,attempt_id,revision,digest,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?)`, key.HostID, key.StartID, key.AttemptID, desired.Revision, string(desired.Digest), rowDigest, payload)
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, mapDBError(ctx, err, true)
	}
	affected, err := inserted.RowsAffected()
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, mapDBError(ctx, err, true)
	}
	actual, err := inspectSystemReadyAttemptV2(ctx, tx, key)
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	if actual.Digest != desired.Digest {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorConflict, "system_ready_attempt_conflict", "SystemReady attempt already exists with different content")
	}
	if affected != 1 {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorConflict, "system_ready_attempt_exists", "SystemReady attempt already exists; only the create winner may execute")
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	return actual.CloneV2(), nil
}

func (s *Store) CompareAndSwapSystemReadyAttemptV2(ctx context.Context, expected contract.ExactRefV1, next contract.SystemReadyAttemptFactV2) (contract.SystemReadyAttemptFactV2, error) {
	if err := s.writeReady(ctx); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	if err := next.Validate(); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	if next.RefV2().ID != expected.ID || next.RefV2().Kind != expected.Kind {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorConflict, "system_ready_attempt_cas_identity_drift", "SystemReady attempt CAS identity drifted")
	}
	payload, rowDigest, err := encodeRow(systemReadyAttemptRowV2, next)
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	tx, err := s.beginMutation(ctx)
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := inspectSystemReadyAttemptV2(ctx, tx, next.StepKey)
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	if current.RefV2() != expected {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorConflict, "system_ready_attempt_cas_conflict", "SystemReady attempt expected Ref drifted")
	}
	if err = contract.ValidateSystemReadyAttemptSuccessorV2(current, next); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	if err = validateSystemReadyAttemptClaim(ctx, tx, next); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	key := next.StepKey
	result, err := tx.ExecContext(ctx, `UPDATE agent_host_system_ready_attempts_v2 SET revision=?,digest=?,row_digest=?,canonical_json=? WHERE host_id=? AND start_id=? AND attempt_id=? AND revision=? AND digest=?`, next.Revision, string(next.Digest), rowDigest, payload, key.HostID, key.StartID, key.AttemptID, current.Revision, string(current.Digest))
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorConflict, "system_ready_attempt_cas_conflict", "SystemReady attempt changed concurrently")
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	return next.CloneV2(), nil
}

func (s *Store) InspectSystemReadyAttemptV2(ctx context.Context, key contract.SystemReadyAttemptStepKeyV2) (contract.SystemReadyAttemptFactV2, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	if err := key.Validate(); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	return inspectSystemReadyAttemptV2(ctx, s.db, key)
}

func inspectSystemReadyAttemptV2(ctx context.Context, query queryRowContext, key contract.SystemReadyAttemptStepKeyV2) (contract.SystemReadyAttemptFactV2, error) {
	var payload []byte
	var rowDigest string
	err := query.QueryRowContext(ctx, `SELECT canonical_json,row_digest FROM agent_host_system_ready_attempts_v2 WHERE host_id=? AND start_id=? AND attempt_id=?`, key.HostID, key.StartID, key.AttemptID).Scan(&payload, &rowDigest)
	if err != nil {
		if err == sql.ErrNoRows {
			return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorNotFound, "system_ready_attempt_missing", "SystemReady attempt does not exist")
		}
		return contract.SystemReadyAttemptFactV2{}, mapDBError(ctx, err, false)
	}
	value, err := decodeRow[contract.SystemReadyAttemptFactV2](payload, rowDigest, systemReadyAttemptRowV2)
	if err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	if value.StepKey != key || value.Validate() != nil {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorConflict, "system_ready_attempt_row_drift", "SystemReady attempt row identity drifted")
	}
	return value.CloneV2(), nil
}

func validateSystemReadyAttemptClaim(ctx context.Context, query queryRowContext, attempt contract.SystemReadyAttemptFactV2) error {
	claim, err := inspectHostStartClaim(ctx, query, attempt.StepKey.HostID, attempt.StepKey.StartID)
	if err != nil {
		if contract.HasCode(err, contract.ErrorNotFound) {
			return contract.NewError(contract.ErrorPrecondition, "host_start_claim_missing", "SystemReady attempt requires its permanent HostStart Claim")
		}
		return err
	}
	claimRef, refErr := claim.CurrentRefV1()
	if refErr != nil {
		return refErr
	}
	if claimRef != attempt.Request.Claim {
		return contract.NewError(contract.ErrorConflict, "system_ready_attempt_claim_drift", "SystemReady attempt does not bind the exact HostStart Claim")
	}
	return nil
}
