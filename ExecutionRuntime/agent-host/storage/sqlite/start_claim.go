package sqlite

import (
	"context"
	"database/sql"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

const hostStartClaimRowV1 = "HostStartClaimV1"

func (s *Store) ClaimOrInspectHostStartV1(ctx context.Context, desired contract.HostStartClaimV1) (contract.HostStartClaimV1, error) {
	if err := s.writeReady(ctx); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	if err := desired.ValidateHistoricalV1(); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	if desired.HostContractVersion == contract.HostLifecycleContractVersionV3 {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorPrecondition, "host_start_v3_atomic_port_required", "HostStart V3 must create its Claim and Input sidecar through the atomic V3 port")
	}
	payload, rowDigest, err := encodeRow(hostStartClaimRowV1, desired)
	if err != nil {
		return contract.HostStartClaimV1{}, err
	}
	tx, err := s.beginMutation(ctx)
	if err != nil {
		return contract.HostStartClaimV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_host_start_claims(host_id,start_id,digest,row_digest,canonical_json) VALUES(?,?,?,?,?)`, desired.HostID, desired.StartID, string(desired.Digest), rowDigest, payload); err != nil {
		return contract.HostStartClaimV1{}, mapDBError(ctx, err, true)
	}
	actual, err := inspectHostStartClaim(ctx, tx, desired.HostID, desired.StartID)
	if err != nil {
		return contract.HostStartClaimV1{}, err
	}
	if !contract.SameHostStartClaimV1(actual, desired) {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorConflict, "host_start_claim_conflict", "HostID and StartID are permanently bound to another exact claim")
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	return actual, nil
}

func (s *Store) InspectHostStartClaimV1(ctx context.Context, hostID, startID string) (contract.HostStartClaimV1, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	if err := contract.ValidateIdentifierV1("host id", hostID); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	if err := contract.ValidateIdentifierV1("start id", startID); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	return inspectHostStartClaim(ctx, s.db, hostID, startID)
}

func (s *Store) InspectHostStartClaimCurrentV1(ctx context.Context, expected contract.HostStartClaimRefV1) (contract.HostStartClaimV1, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	actual, err := inspectHostStartClaim(ctx, s.db, expected.HostID, expected.StartID)
	if err != nil {
		return contract.HostStartClaimV1{}, err
	}
	actualRef, err := actual.CurrentRefV1()
	if err != nil {
		return contract.HostStartClaimV1{}, err
	}
	if actualRef != expected {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorConflict, "host_start_claim_ref_drift", "HostStart Claim exact Ref drifted")
	}
	return actual, nil
}

type queryRowContext interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func inspectHostStartClaim(ctx context.Context, query queryRowContext, hostID, startID string) (contract.HostStartClaimV1, error) {
	var payload []byte
	var rowDigest string
	err := query.QueryRowContext(ctx, `SELECT canonical_json,row_digest FROM agent_host_start_claims WHERE host_id=? AND start_id=?`, hostID, startID).Scan(&payload, &rowDigest)
	if err != nil {
		if err == sql.ErrNoRows {
			return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorNotFound, "host_start_claim_missing", "host start claim does not exist")
		}
		return contract.HostStartClaimV1{}, mapDBError(ctx, err, false)
	}
	value, err := decodeRow[contract.HostStartClaimV1](payload, rowDigest, hostStartClaimRowV1)
	if err != nil {
		return contract.HostStartClaimV1{}, err
	}
	if value.HostID != hostID || value.StartID != startID || value.ValidateHistoricalV1() != nil {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorConflict, "host_start_claim_row_drift", "host start claim row identity drifted")
	}
	return value, nil
}
