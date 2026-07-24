package sqlite

import (
	"context"
	"database/sql"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

const cleanupClosureRowV2 = "HostCleanupClosureFactV2"

func (s *Store) EnsureHostCleanupClosureV2(ctx context.Context, desired contract.HostCleanupClosureFactV2) (contract.HostCleanupClosureFactV2, error) {
	if err := s.writeReady(ctx); err != nil {
		return contract.HostCleanupClosureFactV2{}, err
	}
	if err := desired.Validate(); err != nil {
		return contract.HostCleanupClosureFactV2{}, err
	}
	planRef, err := desired.Plan.RefV2()
	if err != nil {
		return contract.HostCleanupClosureFactV2{}, err
	}
	payload, rowDigest, err := encodeRow(cleanupClosureRowV2, desired)
	if err != nil {
		return contract.HostCleanupClosureFactV2{}, err
	}
	tx, err := s.beginMutation(ctx)
	if err != nil {
		return contract.HostCleanupClosureFactV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	claim, err := inspectHostStartClaim(ctx, tx, desired.Plan.HostID, desired.Plan.StartID)
	if err != nil {
		if contract.HasCode(err, contract.ErrorNotFound) {
			return contract.HostCleanupClosureFactV2{}, contract.NewError(contract.ErrorPrecondition, "host_start_claim_missing", "cleanup closure requires its permanent HostStart Claim")
		}
		return contract.HostCleanupClosureFactV2{}, err
	}
	claimRef, err := claim.CurrentRefV1()
	if err != nil {
		return contract.HostCleanupClosureFactV2{}, err
	}
	if claimRef != desired.StartClaimRef {
		return contract.HostCleanupClosureFactV2{}, contract.NewError(contract.ErrorConflict, "cleanup_closure_start_claim_drift", "cleanup closure Start Claim Ref drifted")
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_host_cleanup_closures_v2(closure_id,host_id,start_id,revision,digest,plan_id,plan_revision,plan_digest,coverage_digest,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, desired.ClosureID, desired.Plan.HostID, desired.Plan.StartID, desired.Revision, string(desired.ContentDigest), planRef.ID, planRef.Revision, string(planRef.Digest), string(desired.CoverageDigest), rowDigest, payload); err != nil {
		return contract.HostCleanupClosureFactV2{}, mapDBError(ctx, err, true)
	}
	actual, err := inspectHostCleanupClosureByIDV2(ctx, tx, desired.ClosureID)
	if err != nil {
		return contract.HostCleanupClosureFactV2{}, err
	}
	wantRef, _ := desired.RefV2()
	gotRef, _ := actual.RefV2()
	if gotRef != wantRef {
		return contract.HostCleanupClosureFactV2{}, contract.NewError(contract.ErrorConflict, "cleanup_closure_conflict", "cleanup closure already exists with different content")
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return contract.HostCleanupClosureFactV2{}, err
	}
	return actual, nil
}

func (s *Store) InspectHostCleanupClosureV2(ctx context.Context, expected contract.HostCleanupClosureRefV2) (contract.HostCleanupClosureFactV2, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.HostCleanupClosureFactV2{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.HostCleanupClosureFactV2{}, err
	}
	actual, err := inspectHostCleanupClosureByIDV2(ctx, s.db, expected.ClosureID)
	if err != nil {
		return contract.HostCleanupClosureFactV2{}, err
	}
	ref, err := actual.RefV2()
	if err != nil {
		return contract.HostCleanupClosureFactV2{}, err
	}
	if ref != expected {
		return contract.HostCleanupClosureFactV2{}, contract.NewError(contract.ErrorConflict, "cleanup_closure_exact_ref_drift", "cleanup closure exact Ref drifted")
	}
	return actual, nil
}

func (s *Store) InspectHostCleanupClosureForStartV2(ctx context.Context, hostID, startID string) (contract.HostCleanupClosureFactV2, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.HostCleanupClosureFactV2{}, err
	}
	for field, value := range map[string]string{"host id": hostID, "start id": startID} {
		if err := contract.ValidateIdentifierV1(field, value); err != nil {
			return contract.HostCleanupClosureFactV2{}, err
		}
	}
	return inspectHostCleanupClosureForStartV2(ctx, s.db, hostID, startID)
}

func inspectHostCleanupClosureByIDV2(ctx context.Context, query queryRowContext, id string) (contract.HostCleanupClosureFactV2, error) {
	return decodeHostCleanupClosureRowV2(ctx, query.QueryRowContext(ctx, `SELECT canonical_json,row_digest FROM agent_host_cleanup_closures_v2 WHERE closure_id=?`, id), func(v contract.HostCleanupClosureFactV2) bool { return v.ClosureID == id })
}

func inspectHostCleanupClosureForStartV2(ctx context.Context, query queryRowContext, hostID, startID string) (contract.HostCleanupClosureFactV2, error) {
	return decodeHostCleanupClosureRowV2(ctx, query.QueryRowContext(ctx, `SELECT canonical_json,row_digest FROM agent_host_cleanup_closures_v2 WHERE host_id=? AND start_id=?`, hostID, startID), func(v contract.HostCleanupClosureFactV2) bool {
		return v.Plan.HostID == hostID && v.Plan.StartID == startID
	})
}

type rowScanner interface{ Scan(...any) error }

func decodeHostCleanupClosureRowV2(ctx context.Context, row rowScanner, identity func(contract.HostCleanupClosureFactV2) bool) (contract.HostCleanupClosureFactV2, error) {
	var payload []byte
	var rowDigest string
	if err := row.Scan(&payload, &rowDigest); err != nil {
		if err == sql.ErrNoRows {
			return contract.HostCleanupClosureFactV2{}, contract.NewError(contract.ErrorNotFound, "cleanup_closure_missing", "cleanup closure does not exist")
		}
		return contract.HostCleanupClosureFactV2{}, mapDBError(ctx, err, false)
	}
	value, err := decodeRow[contract.HostCleanupClosureFactV2](payload, rowDigest, cleanupClosureRowV2)
	if err != nil {
		return contract.HostCleanupClosureFactV2{}, err
	}
	if !identity(value) || value.Validate() != nil {
		return contract.HostCleanupClosureFactV2{}, contract.NewError(contract.ErrorConflict, "cleanup_closure_row_drift", "cleanup closure row identity or canonical content drifted")
	}
	return contract.CloneHostCleanupClosureFactV2(value), nil
}

func (s *Store) InspectCleanupPlanV2(ctx context.Context, expected contract.ExactRefV1) (contract.CleanupPlanV2, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.CleanupPlanV2{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.CleanupPlanV2{}, err
	}
	if expected.Kind != "praxis.agent-host/cleanup-plan-v2" {
		return contract.CleanupPlanV2{}, contract.NewError(contract.ErrorInvalidArgument, "cleanup_plan_ref_kind_invalid", "cleanup Plan Ref kind is unsupported")
	}
	var payload []byte
	var rowDigest string
	err := s.db.QueryRowContext(ctx, `SELECT canonical_json,row_digest FROM agent_host_cleanup_closures_v2 WHERE plan_id=? AND plan_revision=? AND plan_digest=?`, expected.ID, expected.Revision, string(expected.Digest)).Scan(&payload, &rowDigest)
	if err != nil {
		if err == sql.ErrNoRows {
			return contract.CleanupPlanV2{}, contract.NewError(contract.ErrorNotFound, "cleanup_plan_missing", "cleanup Plan does not exist in the Closure store")
		}
		return contract.CleanupPlanV2{}, mapDBError(ctx, err, false)
	}
	closure, err := decodeRow[contract.HostCleanupClosureFactV2](payload, rowDigest, cleanupClosureRowV2)
	if err != nil {
		return contract.CleanupPlanV2{}, err
	}
	if err = closure.Validate(); err != nil {
		return contract.CleanupPlanV2{}, contract.NewError(contract.ErrorConflict, "cleanup_closure_row_drift", "cleanup Closure row drifted")
	}
	actual, err := closure.Plan.RefV2()
	if err != nil || actual != expected {
		return contract.CleanupPlanV2{}, contract.NewError(contract.ErrorConflict, "cleanup_plan_exact_ref_drift", "embedded cleanup Plan exact Ref drifted")
	}
	return contract.CloneHostCleanupClosureFactV2(closure).Plan, nil
}

var _ hostports.HostCleanupClosureFactPortV2 = (*Store)(nil)
var _ hostports.CleanupPlanCurrentReaderV2 = (*Store)(nil)
