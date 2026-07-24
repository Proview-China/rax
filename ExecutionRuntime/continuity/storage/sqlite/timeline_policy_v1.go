package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func (s *Store) CreateTimelineProjectionPolicyV1(ctx context.Context, candidate contract.TimelineProjectionPolicyCurrentV1) (contract.TimelineProjectionPolicyCurrentV1, bool, error) {
	if err := candidate.Validate(); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, false, err
	}
	if candidate.Ref.Revision != 1 || candidate.State != contract.TimelineProjectionPolicyActiveV1 {
		return contract.TimelineProjectionPolicyCurrentV1{}, false, contract.NewError(contract.ErrRevisionConflict, "policy_create", "create requires active revision one")
	}
	body, _, err := encode(candidate)
	if err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, false, unavailable("begin policy create", err)
	}
	defer tx.Rollback()
	current, found, err := inspectCurrentPolicyTx(ctx, tx, candidate.Ref.ScopeDigest, candidate.Ref.PolicyID)
	if err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, false, err
	}
	if found {
		if current.Ref.Digest == candidate.Ref.Digest {
			return current, true, tx.Commit()
		}
		return contract.TimelineProjectionPolicyCurrentV1{}, false, contract.NewError(contract.ErrRevisionConflict, "policy_id", "create-once policy changed content")
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO timeline_policy_history(scope_digest,policy_id,revision,ref_digest,body) VALUES(?,?,?,?,?)", candidate.Ref.ScopeDigest, candidate.Ref.PolicyID, 1, candidate.Ref.Digest, body); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, false, unavailable("insert policy history", err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO timeline_policy_current(scope_digest,policy_id,revision) VALUES(?,?,1)", candidate.Ref.ScopeDigest, candidate.Ref.PolicyID); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, false, unavailable("insert policy current", err)
	}
	if err = tx.Commit(); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, false, unavailable("commit policy create", err)
	}
	return candidate, false, nil
}

func (s *Store) InspectTimelineProjectionPolicyV1(ctx context.Context, ref contract.TimelineProjectionPolicyRefV1) (contract.TimelineProjectionPolicyCurrentV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, "SELECT body FROM timeline_policy_history WHERE scope_digest=? AND policy_id=? AND revision=?", ref.ScopeDigest, ref.PolicyID, ref.Revision).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.TimelineProjectionPolicyCurrentV1{}, notFound("policy_ref", "policy revision not found")
		}
		return contract.TimelineProjectionPolicyCurrentV1{}, unavailable("inspect policy", err)
	}
	value, err := decodePolicy(body)
	if err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	if value.Ref != ref {
		return contract.TimelineProjectionPolicyCurrentV1{}, contract.NewError(contract.ErrRevisionConflict, "policy_ref", "policy digest drifted")
	}
	return value, nil
}

func (s *Store) InspectTimelineProjectionPolicyCurrentV1(ctx context.Context, policyID, scopeDigest string) (contract.TimelineProjectionPolicyCurrentV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	if err := contract.ValidateToken("policy_id", policyID); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	if err := contract.ValidateToken("scope_digest", scopeDigest); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	var body []byte
	err := s.db.QueryRowContext(ctx, `SELECT h.body FROM timeline_policy_current c
		JOIN timeline_policy_history h ON h.scope_digest=c.scope_digest AND h.policy_id=c.policy_id AND h.revision=c.revision
		WHERE c.scope_digest=? AND c.policy_id=?`, scopeDigest, policyID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.TimelineProjectionPolicyCurrentV1{}, notFound("policy_id", "current policy not found")
	}
	if err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, unavailable("inspect current policy", err)
	}
	return decodePolicy(body)
}

func inspectCurrentPolicyTx(ctx context.Context, tx *sql.Tx, scopeDigest, policyID string) (contract.TimelineProjectionPolicyCurrentV1, bool, error) {
	var body []byte
	err := tx.QueryRowContext(ctx, `SELECT h.body FROM timeline_policy_current c
		JOIN timeline_policy_history h ON h.scope_digest=c.scope_digest AND h.policy_id=c.policy_id AND h.revision=c.revision
		WHERE c.scope_digest=? AND c.policy_id=?`, scopeDigest, policyID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.TimelineProjectionPolicyCurrentV1{}, false, nil
	}
	if err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, false, unavailable("inspect current policy", err)
	}
	value, err := decodePolicy(body)
	return value, true, err
}

func decodePolicy(body []byte) (contract.TimelineProjectionPolicyCurrentV1, error) {
	var value contract.TimelineProjectionPolicyCurrentV1
	if err := decode(body, &value); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	if err := value.Validate(); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, contract.NewError(contract.ErrContentDigestMismatch, "timeline_policy", "stored policy failed validation")
	}
	return value, nil
}

func (s *Store) ValidateTimelineProjectionPolicyCurrentV1(ctx context.Context, expected contract.TimelineProjectionPolicyCurrentV1) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	current, err := s.InspectTimelineProjectionPolicyCurrentV1(ctx, expected.Ref.PolicyID, expected.Ref.ScopeDigest)
	if err != nil {
		return err
	}
	if current.Ref != expected.Ref {
		return contract.NewError(contract.ErrRevisionConflict, "policy_ref", "policy current index advanced")
	}
	return current.ValidateCurrent(expected.Ref, s.clock())
}

func (s *Store) CompareAndSwapTimelineProjectionPolicyV1(ctx context.Context, expected contract.TimelineProjectionPolicyRefV1, next contract.TimelineProjectionPolicyCurrentV1) (contract.TimelineProjectionPolicyCurrentV1, error) {
	if err := expected.Validate(); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	if err := next.Validate(); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	body, _, err := encode(next)
	if err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, unavailable("begin policy CAS", err)
	}
	defer tx.Rollback()
	current, found, err := inspectCurrentPolicyTx(ctx, tx, expected.ScopeDigest, expected.PolicyID)
	if err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	if !found {
		return contract.TimelineProjectionPolicyCurrentV1{}, notFound("policy_id", "current policy not found")
	}
	if current.Ref != expected {
		return contract.TimelineProjectionPolicyCurrentV1{}, contract.NewError(contract.ErrRevisionConflict, "policy_ref", "CAS expected ref is stale")
	}
	if err := contract.ValidateTimelineProjectionPolicySuccessorV1(current, next); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO timeline_policy_history(scope_digest,policy_id,revision,ref_digest,body) VALUES(?,?,?,?,?)", next.Ref.ScopeDigest, next.Ref.PolicyID, next.Ref.Revision, next.Ref.Digest, body); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, unavailable("insert policy history", err)
	}
	result, err := tx.ExecContext(ctx, "UPDATE timeline_policy_current SET revision=? WHERE scope_digest=? AND policy_id=? AND revision=?", next.Ref.Revision, next.Ref.ScopeDigest, next.Ref.PolicyID, expected.Revision)
	if err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, unavailable("update policy current", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return contract.TimelineProjectionPolicyCurrentV1{}, contract.NewError(contract.ErrRevisionConflict, "policy_ref", "policy current CAS lost")
	}
	if err = tx.Commit(); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, unavailable("commit policy CAS", err)
	}
	return next, nil
}

var _ ports.TimelineProjectionPolicyRepositoryV1 = (*Store)(nil)
