package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func (s *Store) CreateHistoryDerivationCandidateFactV1(ctx context.Context, fact contract.HistoryDerivationCandidateFactV1) (contract.HistoryDerivationCandidateFactV1, bool, error) {
	if err := fact.Validate(); err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, err
	}
	body, _, err := encode(fact)
	if err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, err
	}
	tenant, scope, id := fact.Scope.TenantID, fact.Scope.ExecutionScopeDigest, fact.CandidateID
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, unavailable("begin History Derivation Candidate create", err)
	}
	defer tx.Rollback()
	var boundID string
	err = tx.QueryRowContext(ctx, "SELECT candidate_id FROM history_derivation_candidate_facts WHERE tenant_id=? AND scope_digest=? AND idempotency_key=?", tenant, scope, fact.IdempotencyKey).Scan(&boundID)
	if err == nil && boundID != id {
		return contract.HistoryDerivationCandidateFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "request already created another History Derivation Candidate")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return contract.HistoryDerivationCandidateFactV1{}, false, unavailable("inspect History Derivation Candidate idempotency", err)
	}
	existing, found, err := inspectHistoryDerivationByIDTxV1(ctx, tx, tenant, scope, id)
	if err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, err
	}
	if found {
		if existing.Ref().Exact().Equal(fact.Ref().Exact()) && existing.IdempotencyKey == fact.IdempotencyKey && existing.RequestDigest == fact.RequestDigest {
			return existing.Clone(), true, tx.Commit()
		}
		return contract.HistoryDerivationCandidateFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "candidate_id", "create-once History Derivation Candidate changed content")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO history_derivation_candidate_facts(
		tenant_id,scope_digest,candidate_id,idempotency_key,request_digest,kind,source_set_digest,output_object_id,ref_digest,body)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, tenant, scope, id, fact.IdempotencyKey, fact.RequestDigest, fact.Kind, fact.SourceSetDigest, fact.Output.ObjectID, fact.Digest, body); err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, unavailable("insert History Derivation Candidate", err)
	}
	if err = tx.Commit(); err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, unavailable("commit History Derivation Candidate", err)
	}
	return fact.Clone(), false, nil
}

func (s *Store) InspectHistoryDerivationCandidateV1(ctx context.Context, request ports.InspectHistoryDerivationCandidateRequestV1) (contract.HistoryDerivationCandidateFactV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, err
	}
	if err := request.Ref.Validate(); err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, err
	}
	ref := request.Ref.Exact()
	fact, err := s.inspectHistoryDerivationByIDV1(ctx, ref.TenantID, ref.ScopeDigest, ref.ID)
	if err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, err
	}
	if !fact.Ref().Exact().Equal(ref) {
		return contract.HistoryDerivationCandidateFactV1{}, contract.NewError(contract.ErrRevisionConflict, "history_derivation_ref", "exact History Derivation Candidate ref mismatch")
	}
	return fact.Clone(), nil
}

func (s *Store) InspectHistoryDerivationCandidateByIDV1(ctx context.Context, request ports.InspectHistoryDerivationCandidateByIDRequestV1) (contract.HistoryDerivationCandidateFactV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, err
	}
	if err := validateHistoryDerivationByIDRequestV1(request); err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, err
	}
	fact, err := s.inspectHistoryDerivationByIDV1(ctx, request.TenantID, request.ScopeDigest, request.CandidateID)
	if err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, err
	}
	if fact.Owner != request.Owner {
		return contract.HistoryDerivationCandidateFactV1{}, contract.NewError(contract.ErrRevisionConflict, "owner_binding", "History Derivation Candidate Owner mismatch")
	}
	return fact.Clone(), nil
}

func (s *Store) inspectHistoryDerivationByIDV1(ctx context.Context, tenant, scope, id string) (contract.HistoryDerivationCandidateFactV1, error) {
	var body []byte
	if err := s.db.QueryRowContext(ctx, "SELECT body FROM history_derivation_candidate_facts WHERE tenant_id=? AND scope_digest=? AND candidate_id=?", tenant, scope, id).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.HistoryDerivationCandidateFactV1{}, notFound("history_derivation_id", "History Derivation Candidate not found")
		}
		return contract.HistoryDerivationCandidateFactV1{}, unavailable("inspect History Derivation Candidate", err)
	}
	return decodeHistoryDerivationCandidateV1(body)
}

func inspectHistoryDerivationByIDTxV1(ctx context.Context, tx *sql.Tx, tenant, scope, id string) (contract.HistoryDerivationCandidateFactV1, bool, error) {
	var body []byte
	err := tx.QueryRowContext(ctx, "SELECT body FROM history_derivation_candidate_facts WHERE tenant_id=? AND scope_digest=? AND candidate_id=?", tenant, scope, id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.HistoryDerivationCandidateFactV1{}, false, nil
	}
	if err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, false, unavailable("inspect History Derivation Candidate", err)
	}
	fact, err := decodeHistoryDerivationCandidateV1(body)
	return fact, true, err
}

func decodeHistoryDerivationCandidateV1(body []byte) (contract.HistoryDerivationCandidateFactV1, error) {
	var fact contract.HistoryDerivationCandidateFactV1
	if err := decode(body, &fact); err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, err
	}
	if err := fact.Validate(); err != nil {
		return contract.HistoryDerivationCandidateFactV1{}, contract.NewError(contract.ErrContentDigestMismatch, "history_derivation_candidate", "stored History Derivation Candidate failed validation")
	}
	return fact.Clone(), nil
}

func validateHistoryDerivationByIDRequestV1(request ports.InspectHistoryDerivationCandidateByIDRequestV1) error {
	for field, value := range map[string]string{"tenant_id": request.TenantID, "scope_digest": request.ScopeDigest, "candidate_id": request.CandidateID} {
		if err := contract.ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := request.Owner.Validate(); err != nil {
		return err
	}
	if request.Owner.ComponentID != contract.ContinuityComponentID || request.Owner.Capability != contract.HistoryDerivationCapabilityV1 || request.Owner.FactKind != "history_derivation_candidate_fact_v1" {
		return contract.NewError(contract.ErrInvalidArgument, "owner_binding", "wrong Continuity History Derivation owner")
	}
	return nil
}

var _ ports.HistoryDerivationCandidateReaderV1 = (*Store)(nil)
