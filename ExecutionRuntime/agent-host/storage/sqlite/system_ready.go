package sqlite

import (
	"context"
	"database/sql"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	systemReadyFactRowV2    = "SystemReadyFactV2"
	systemReadyCurrentRowV2 = "SystemReadyCurrentV2"
)

func (s *Store) CreateSystemReadyFactV2(ctx context.Context, desired contract.SystemReadyFactV2) (contract.SystemReadyFactV2, error) {
	if err := s.writeReady(ctx); err != nil {
		return contract.SystemReadyFactV2{}, err
	}
	if err := desired.Validate(); err != nil {
		return contract.SystemReadyFactV2{}, err
	}
	payload, rowDigest, err := encodeRow(systemReadyFactRowV2, desired)
	if err != nil {
		return contract.SystemReadyFactV2{}, err
	}
	tx, err := s.beginMutation(ctx)
	if err != nil {
		return contract.SystemReadyFactV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = validateReadyClaim(ctx, tx, desired); err != nil {
		return contract.SystemReadyFactV2{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_host_system_ready_facts_v2(id,revision,host_id,start_id,digest,expires_unix_nano,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?,?)`, desired.Ref.ID, desired.Ref.Revision, desired.HostID, desired.StartID, string(desired.Digest), desired.ExpiresUnixNano, rowDigest, payload); err != nil {
		return contract.SystemReadyFactV2{}, mapDBError(ctx, err, true)
	}
	actual, err := inspectSystemReadyFact(ctx, tx, desired.Ref.ID)
	if err != nil {
		return contract.SystemReadyFactV2{}, err
	}
	if actual.Ref != desired.Ref || actual.Digest != desired.Digest {
		return contract.SystemReadyFactV2{}, contract.NewError(contract.ErrorConflict, "system_ready_fact_conflict", "SystemReady Fact ID already binds another immutable fact")
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return contract.SystemReadyFactV2{}, err
	}
	return actual, nil
}

func (s *Store) InspectSystemReadyFactV2(ctx context.Context, ref contract.SystemReadyFactRefV2) (contract.SystemReadyFactV2, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.SystemReadyFactV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.SystemReadyFactV2{}, err
	}
	value, err := inspectSystemReadyFact(ctx, s.db, ref.ID)
	if err != nil {
		return contract.SystemReadyFactV2{}, err
	}
	if value.Ref != ref {
		return contract.SystemReadyFactV2{}, contract.NewError(contract.ErrorConflict, "system_ready_fact_ref_drift", "SystemReady Fact exact Ref drifted")
	}
	return value, nil
}

func (s *Store) CreateSystemReadyCurrentV2(ctx context.Context, desired contract.SystemReadyCurrentV2) (contract.SystemReadyCurrentV2, error) {
	if err := s.writeReady(ctx); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	if err := desired.Validate(); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	payload, rowDigest, err := encodeRow(systemReadyCurrentRowV2, desired)
	if err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	tx, err := s.beginMutation(ctx)
	if err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = validateReadyCurrentFact(ctx, tx, desired); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	current, inspectErr := inspectSystemReadyCurrentByID(ctx, tx, desired.Ref.ID)
	if inspectErr == nil {
		if current.Ref != desired.Ref || current.ProjectionDigest != desired.ProjectionDigest {
			return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "system_ready_current_conflict", "SystemReady Current ID already exists with different content")
		}
		if err = s.finishMutation(ctx, tx); err != nil {
			return contract.SystemReadyCurrentV2{}, err
		}
		return current, nil
	}
	if !contract.HasCode(inspectErr, contract.ErrorNotFound) {
		return contract.SystemReadyCurrentV2{}, inspectErr
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_host_system_ready_current_history_v2(id,revision,epoch,fact_id,digest,expires_unix_nano,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?,?)`, desired.Ref.ID, desired.Ref.Revision, desired.Ref.Epoch, desired.FactRef.ID, string(desired.Ref.Digest), desired.ExpiresUnixNano, rowDigest, payload); err != nil {
		return contract.SystemReadyCurrentV2{}, mapDBError(ctx, err, true)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_host_system_ready_current_v2(id,revision,epoch,digest) VALUES(?,?,?,?)`, desired.Ref.ID, desired.Ref.Revision, desired.Ref.Epoch, string(desired.Ref.Digest)); err != nil {
		return contract.SystemReadyCurrentV2{}, mapDBError(ctx, err, true)
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	return desired, nil
}

func (s *Store) CompareAndSwapSystemReadyCurrentV2(ctx context.Context, expected contract.SystemReadyCurrentRefV2, next contract.SystemReadyCurrentV2) (contract.SystemReadyCurrentV2, error) {
	if err := s.writeReady(ctx); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	if err := next.Validate(); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	payload, rowDigest, err := encodeRow(systemReadyCurrentRowV2, next)
	if err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	tx, err := s.beginMutation(ctx)
	if err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := inspectSystemReadyCurrentByID(ctx, tx, expected.ID)
	if err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	if current.Ref != expected {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "system_ready_current_cas_conflict", "SystemReady Current expected Ref drifted")
	}
	if err = validateReadyCurrentFact(ctx, tx, next); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	if err = contract.ValidateSystemReadyCurrentSuccessorV2(current, next); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_host_system_ready_current_history_v2(id,revision,epoch,fact_id,digest,expires_unix_nano,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?,?)`, next.Ref.ID, next.Ref.Revision, next.Ref.Epoch, next.FactRef.ID, string(next.Ref.Digest), next.ExpiresUnixNano, rowDigest, payload); err != nil {
		return contract.SystemReadyCurrentV2{}, mapDBError(ctx, err, true)
	}
	result, err := tx.ExecContext(ctx, `UPDATE agent_host_system_ready_current_v2 SET revision=?,epoch=?,digest=? WHERE id=? AND revision=? AND epoch=? AND digest=?`, next.Ref.Revision, next.Ref.Epoch, string(next.Ref.Digest), expected.ID, expected.Revision, expected.Epoch, string(expected.Digest))
	if err != nil {
		return contract.SystemReadyCurrentV2{}, mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return contract.SystemReadyCurrentV2{}, mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "system_ready_current_cas_conflict", "SystemReady Current changed concurrently")
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	return next, nil
}

func (s *Store) InspectSystemReadyCurrentV2(ctx context.Context, ref contract.SystemReadyCurrentRefV2) (contract.SystemReadyCurrentV2, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	current, err := inspectSystemReadyCurrentByID(ctx, s.db, ref.ID)
	if err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	if current.Ref != ref {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "system_ready_current_ref_drift", "SystemReady Current exact Ref drifted")
	}
	return current, nil
}

func (s *Store) InspectSystemReadyCurrentForAvailabilityV2(ctx context.Context, expected runtimeports.AgentExecutionAvailabilityRefV1) (contract.SystemReadyCurrentV2, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	if expected.Owner != s.owner {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "availability_owner_drift", "availability Ref belongs to another Owner")
	}
	current, err := inspectSystemReadyCurrentByID(ctx, s.db, expected.ID)
	if err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	projection, err := current.ToAgentExecutionAvailabilityV1(s.owner)
	if err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	if projection.Ref != expected {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "availability_projection_drift", "availability projection does not match the exact requested Ref")
	}
	return current, nil
}

func inspectSystemReadyFact(ctx context.Context, query queryRowContext, id string) (contract.SystemReadyFactV2, error) {
	var payload []byte
	var rowDigest string
	err := query.QueryRowContext(ctx, `SELECT canonical_json,row_digest FROM agent_host_system_ready_facts_v2 WHERE id=?`, id).Scan(&payload, &rowDigest)
	if err != nil {
		if err == sql.ErrNoRows {
			return contract.SystemReadyFactV2{}, contract.NewError(contract.ErrorNotFound, "system_ready_fact_missing", "SystemReady Fact does not exist")
		}
		return contract.SystemReadyFactV2{}, mapDBError(ctx, err, false)
	}
	value, err := decodeRow[contract.SystemReadyFactV2](payload, rowDigest, systemReadyFactRowV2)
	if err != nil {
		return contract.SystemReadyFactV2{}, err
	}
	if value.Ref.ID != id || value.Validate() != nil {
		return contract.SystemReadyFactV2{}, contract.NewError(contract.ErrorConflict, "system_ready_fact_row_drift", "SystemReady Fact row identity drifted")
	}
	return value, nil
}

func inspectSystemReadyCurrentByID(ctx context.Context, query queryRowContext, id string) (contract.SystemReadyCurrentV2, error) {
	var payload []byte
	var rowDigest string
	err := query.QueryRowContext(ctx, `SELECT h.canonical_json,h.row_digest FROM agent_host_system_ready_current_v2 c JOIN agent_host_system_ready_current_history_v2 h ON h.id=c.id AND h.revision=c.revision WHERE c.id=?`, id).Scan(&payload, &rowDigest)
	if err != nil {
		if err == sql.ErrNoRows {
			return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorNotFound, "system_ready_current_missing", "SystemReady Current does not exist")
		}
		return contract.SystemReadyCurrentV2{}, mapDBError(ctx, err, false)
	}
	value, err := decodeRow[contract.SystemReadyCurrentV2](payload, rowDigest, systemReadyCurrentRowV2)
	if err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	if value.Ref.ID != id || value.Validate() != nil {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "system_ready_current_row_drift", "SystemReady Current row identity drifted")
	}
	return value, nil
}

func validateReadyClaim(ctx context.Context, query queryRowContext, fact contract.SystemReadyFactV2) error {
	claim, err := inspectHostStartClaim(ctx, query, fact.HostID, fact.StartID)
	if err != nil {
		if contract.HasCode(err, contract.ErrorNotFound) {
			return contract.NewError(contract.ErrorPrecondition, "host_start_claim_missing", "SystemReady Fact requires its permanent HostStart Claim")
		}
		return err
	}
	currentRef, err := claim.CurrentRefV1()
	if err != nil {
		return err
	}
	if currentRef != fact.HostStartClaim {
		return contract.NewError(contract.ErrorConflict, "system_ready_claim_drift", "SystemReady Fact does not bind the exact HostStart Claim")
	}
	return nil
}

func validateReadyCurrentFact(ctx context.Context, query queryRowContext, current contract.SystemReadyCurrentV2) error {
	fact, err := inspectSystemReadyFact(ctx, query, current.FactRef.ID)
	if err != nil {
		if contract.HasCode(err, contract.ErrorNotFound) {
			return contract.NewError(contract.ErrorPrecondition, "system_ready_fact_missing", "SystemReady Current requires its immutable Fact")
		}
		return err
	}
	if fact.Ref != current.FactRef {
		return contract.NewError(contract.ErrorConflict, "system_ready_fact_ref_drift", "SystemReady Current names a non-exact Fact Ref")
	}
	if fact.HostID != current.HostID || fact.StartID != current.StartID {
		return contract.NewError(contract.ErrorConflict, "system_ready_subject_drift", "SystemReady Current subject does not match its immutable Fact")
	}
	return nil
}
