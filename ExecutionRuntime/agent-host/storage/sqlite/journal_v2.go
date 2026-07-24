package sqlite

import (
	"context"
	"database/sql"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

const hostJournalRowV2 = "HostJournalV2"

func (s *Store) CreateHostJournalV2(ctx context.Context, desired contract.HostJournalV2) (contract.HostJournalV2, error) {
	if err := s.writeReady(ctx); err != nil {
		return contract.HostJournalV2{}, err
	}
	if err := desired.Validate(); err != nil {
		return contract.HostJournalV2{}, err
	}
	payload, rowDigest, err := encodeRow(hostJournalRowV2, desired)
	if err != nil {
		return contract.HostJournalV2{}, err
	}
	tx, err := s.beginMutation(ctx)
	if err != nil {
		return contract.HostJournalV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = validateJournalClaim(ctx, tx, desired); err != nil {
		return contract.HostJournalV2{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_host_journal_v2(host_id,start_id,revision,digest,row_digest,canonical_json) VALUES(?,?,?,?,?,?)`, desired.HostID, desired.StartID, desired.Revision, string(desired.Digest), rowDigest, payload); err != nil {
		return contract.HostJournalV2{}, mapDBError(ctx, err, true)
	}
	actual, err := inspectHostJournalV2(ctx, tx, desired.HostID, desired.StartID)
	if err != nil {
		return contract.HostJournalV2{}, err
	}
	if actual.Digest != desired.Digest {
		return contract.HostJournalV2{}, contract.NewError(contract.ErrorConflict, "host_journal_v2_conflict", "HostV2 Journal already exists with different content")
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return contract.HostJournalV2{}, err
	}
	return actual, nil
}

func (s *Store) CompareAndSwapHostJournalV2(ctx context.Context, expected contract.ExactRefV1, next contract.HostJournalV2) (contract.HostJournalV2, error) {
	if err := s.writeReady(ctx); err != nil {
		return contract.HostJournalV2{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.HostJournalV2{}, err
	}
	if err := next.Validate(); err != nil {
		return contract.HostJournalV2{}, err
	}
	payload, rowDigest, err := encodeRow(hostJournalRowV2, next)
	if err != nil {
		return contract.HostJournalV2{}, err
	}
	tx, err := s.beginMutation(ctx)
	if err != nil {
		return contract.HostJournalV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := inspectHostJournalV2(ctx, tx, next.HostID, next.StartID)
	if err != nil {
		return contract.HostJournalV2{}, err
	}
	currentRef, err := current.RefV2()
	if err != nil {
		return contract.HostJournalV2{}, err
	}
	if currentRef != expected {
		return contract.HostJournalV2{}, contract.NewError(contract.ErrorConflict, "host_journal_v2_cas_conflict", "HostV2 Journal expected Ref drifted")
	}
	if err = contract.ValidateHostJournalSuccessorV2(current, next); err != nil {
		return contract.HostJournalV2{}, err
	}
	if err = validateJournalClaim(ctx, tx, next); err != nil {
		return contract.HostJournalV2{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE agent_host_journal_v2 SET revision=?,digest=?,row_digest=?,canonical_json=? WHERE host_id=? AND start_id=? AND revision=? AND digest=?`, next.Revision, string(next.Digest), rowDigest, payload, next.HostID, next.StartID, current.Revision, string(current.Digest))
	if err != nil {
		return contract.HostJournalV2{}, mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return contract.HostJournalV2{}, mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return contract.HostJournalV2{}, contract.NewError(contract.ErrorConflict, "host_journal_v2_cas_conflict", "HostV2 Journal changed concurrently")
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return contract.HostJournalV2{}, err
	}
	return next, nil
}

func (s *Store) InspectHostJournalV2(ctx context.Context, hostID, startID string) (contract.HostJournalV2, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.HostJournalV2{}, err
	}
	if err := contract.ValidateIdentifierV1("host id", hostID); err != nil {
		return contract.HostJournalV2{}, err
	}
	if err := contract.ValidateIdentifierV1("start id", startID); err != nil {
		return contract.HostJournalV2{}, err
	}
	return inspectHostJournalV2(ctx, s.db, hostID, startID)
}

func inspectHostJournalV2(ctx context.Context, query queryRowContext, hostID, startID string) (contract.HostJournalV2, error) {
	var payload []byte
	var rowDigest string
	err := query.QueryRowContext(ctx, `SELECT canonical_json,row_digest FROM agent_host_journal_v2 WHERE host_id=? AND start_id=?`, hostID, startID).Scan(&payload, &rowDigest)
	if err != nil {
		if err == sql.ErrNoRows {
			return contract.HostJournalV2{}, contract.NewError(contract.ErrorNotFound, "host_journal_v2_missing", "HostV2 Journal does not exist")
		}
		return contract.HostJournalV2{}, mapDBError(ctx, err, false)
	}
	value, err := decodeRow[contract.HostJournalV2](payload, rowDigest, hostJournalRowV2)
	if err != nil {
		return contract.HostJournalV2{}, err
	}
	if value.HostID != hostID || value.StartID != startID || value.Validate() != nil {
		return contract.HostJournalV2{}, contract.NewError(contract.ErrorConflict, "host_journal_v2_row_drift", "HostV2 Journal row identity drifted")
	}
	return value, nil
}

func validateJournalClaim(ctx context.Context, query queryRowContext, journal contract.HostJournalV2) error {
	claim, err := inspectHostStartClaim(ctx, query, journal.HostID, journal.StartID)
	if err != nil {
		if contract.HasCode(err, contract.ErrorNotFound) {
			return contract.NewError(contract.ErrorPrecondition, "host_start_claim_missing", "HostV2 Journal requires its permanent HostStart Claim")
		}
		return err
	}
	claimRef, err := claim.RefV1()
	if err != nil {
		return err
	}
	if claimRef != journal.StartClaimRef || claim.ConfigDigest != journal.ConfigDigest {
		return contract.NewError(contract.ErrorConflict, "host_journal_claim_drift", "HostV2 Journal does not bind the exact HostStart Claim")
	}
	return nil
}
