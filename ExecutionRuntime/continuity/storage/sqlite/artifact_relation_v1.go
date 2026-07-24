package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func (s *Store) CreateArtifactRelationFactV1(ctx context.Context, fact contract.ArtifactRelationFactV1) (contract.ArtifactRelationFactV1, bool, error) {
	if err := fact.Validate(); err != nil {
		return contract.ArtifactRelationFactV1{}, false, err
	}
	body, _, err := encode(fact)
	if err != nil {
		return contract.ArtifactRelationFactV1{}, false, err
	}
	artifactDigest, err := contract.CanonicalDigest(fact.SourceProjection.Artifact.ArtifactFactRef.IdentityKey())
	if err != nil {
		return contract.ArtifactRelationFactV1{}, false, err
	}
	relatedDigest, err := contract.CanonicalDigest(fact.SourceProjection.RelatedFactRef.IdentityKey())
	if err != nil {
		return contract.ArtifactRelationFactV1{}, false, err
	}
	tenant, scope, id := fact.Scope.TenantID, fact.Scope.ExecutionScopeDigest, fact.RelationID
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.ArtifactRelationFactV1{}, false, unavailable("begin Artifact Relation create", err)
	}
	defer tx.Rollback()
	var boundID string
	err = tx.QueryRowContext(ctx, "SELECT relation_id FROM artifact_relation_facts WHERE tenant_id=? AND scope_digest=? AND idempotency_key=?", tenant, scope, fact.IdempotencyKey).Scan(&boundID)
	if err == nil && boundID != id {
		return contract.ArtifactRelationFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "request already created another Artifact Relation")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return contract.ArtifactRelationFactV1{}, false, unavailable("inspect Artifact Relation idempotency", err)
	}
	existing, found, err := inspectArtifactRelationByIDTxV1(ctx, tx, tenant, scope, id)
	if err != nil {
		return contract.ArtifactRelationFactV1{}, false, err
	}
	if found {
		if existing.Ref().Exact().Equal(fact.Ref().Exact()) && existing.IdempotencyKey == fact.IdempotencyKey {
			return existing.Clone(), true, tx.Commit()
		}
		return contract.ArtifactRelationFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "relation_id", "create-once Artifact Relation changed content")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO artifact_relation_facts(
		tenant_id,scope_digest,relation_id,idempotency_key,artifact_identity_digest,
		related_identity_digest,ref_digest,body) VALUES(?,?,?,?,?,?,?,?)`,
		tenant, scope, id, fact.IdempotencyKey, artifactDigest, relatedDigest, fact.Digest, body); err != nil {
		return contract.ArtifactRelationFactV1{}, false, unavailable("insert Artifact Relation", err)
	}
	if err = tx.Commit(); err != nil {
		return contract.ArtifactRelationFactV1{}, false, unavailable("commit Artifact Relation", err)
	}
	return fact.Clone(), false, nil
}

func (s *Store) InspectArtifactRelationV1(ctx context.Context, request ports.InspectArtifactRelationRequestV1) (contract.ArtifactRelationFactV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.ArtifactRelationFactV1{}, err
	}
	if err := request.Ref.Validate(); err != nil {
		return contract.ArtifactRelationFactV1{}, err
	}
	ref := request.Ref.Exact()
	fact, err := s.inspectArtifactRelationByIDV1(ctx, ref.TenantID, ref.ScopeDigest, ref.ID)
	if err != nil {
		return contract.ArtifactRelationFactV1{}, err
	}
	if !fact.Ref().Exact().Equal(ref) {
		return contract.ArtifactRelationFactV1{}, contract.NewError(contract.ErrRevisionConflict, "artifact_relation_ref", "exact Artifact Relation ref mismatch")
	}
	return fact.Clone(), nil
}

func (s *Store) InspectArtifactRelationByIDV1(ctx context.Context, request ports.InspectArtifactRelationByIDRequestV1) (contract.ArtifactRelationFactV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.ArtifactRelationFactV1{}, err
	}
	if err := validateArtifactRelationByIDRequestV1(request); err != nil {
		return contract.ArtifactRelationFactV1{}, err
	}
	fact, err := s.inspectArtifactRelationByIDV1(ctx, request.TenantID, request.ScopeDigest, request.RelationID)
	if err != nil {
		return contract.ArtifactRelationFactV1{}, err
	}
	if fact.Owner != request.Owner {
		return contract.ArtifactRelationFactV1{}, contract.NewError(contract.ErrRevisionConflict, "owner_binding", "Artifact Relation Owner mismatch")
	}
	return fact.Clone(), nil
}

func (s *Store) inspectArtifactRelationByIDV1(ctx context.Context, tenant, scope, id string) (contract.ArtifactRelationFactV1, error) {
	var body []byte
	if err := s.db.QueryRowContext(ctx, "SELECT body FROM artifact_relation_facts WHERE tenant_id=? AND scope_digest=? AND relation_id=?", tenant, scope, id).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.ArtifactRelationFactV1{}, notFound("artifact_relation_id", "Artifact Relation not found")
		}
		return contract.ArtifactRelationFactV1{}, unavailable("inspect Artifact Relation", err)
	}
	return decodeArtifactRelationV1(body)
}

func (s *Store) ListArtifactRelationsV1(ctx context.Context, request ports.ListArtifactRelationsRequestV1) ([]contract.ArtifactRelationFactV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return nil, err
	}
	if err := request.ArtifactFactRef.Validate(); err != nil {
		return nil, err
	}
	digest, err := contract.CanonicalDigest(request.ArtifactFactRef.IdentityKey())
	if err != nil {
		return nil, err
	}
	return s.listArtifactRelationsV1(ctx, "artifact_identity_digest", digest)
}

func (s *Store) ListRelatedArtifactRelationsV1(ctx context.Context, request ports.ListRelatedArtifactRelationsRequestV1) ([]contract.ArtifactRelationFactV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return nil, err
	}
	if err := request.RelatedFactRef.Validate(); err != nil {
		return nil, err
	}
	digest, err := contract.CanonicalDigest(request.RelatedFactRef.IdentityKey())
	if err != nil {
		return nil, err
	}
	return s.listArtifactRelationsV1(ctx, "related_identity_digest", digest)
}

func (s *Store) listArtifactRelationsV1(ctx context.Context, column, digest string) ([]contract.ArtifactRelationFactV1, error) {
	query := "SELECT body FROM artifact_relation_facts WHERE " + column + "=? ORDER BY tenant_id,scope_digest,relation_id"
	rows, err := s.db.QueryContext(ctx, query, digest)
	if err != nil {
		return nil, unavailable("list Artifact Relations", err)
	}
	defer rows.Close()
	result := make([]contract.ArtifactRelationFactV1, 0)
	for rows.Next() {
		var body []byte
		if err := rows.Scan(&body); err != nil {
			return nil, unavailable("scan Artifact Relation", err)
		}
		fact, err := decodeArtifactRelationV1(body)
		if err != nil {
			return nil, err
		}
		result = append(result, fact.Clone())
	}
	if err := rows.Err(); err != nil {
		return nil, unavailable("iterate Artifact Relations", err)
	}
	return result, nil
}

func inspectArtifactRelationByIDTxV1(ctx context.Context, tx *sql.Tx, tenant, scope, id string) (contract.ArtifactRelationFactV1, bool, error) {
	var body []byte
	err := tx.QueryRowContext(ctx, "SELECT body FROM artifact_relation_facts WHERE tenant_id=? AND scope_digest=? AND relation_id=?", tenant, scope, id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.ArtifactRelationFactV1{}, false, nil
	}
	if err != nil {
		return contract.ArtifactRelationFactV1{}, false, unavailable("inspect Artifact Relation", err)
	}
	fact, err := decodeArtifactRelationV1(body)
	return fact, true, err
}

func decodeArtifactRelationV1(body []byte) (contract.ArtifactRelationFactV1, error) {
	var fact contract.ArtifactRelationFactV1
	if err := decode(body, &fact); err != nil {
		return contract.ArtifactRelationFactV1{}, err
	}
	if err := fact.Validate(); err != nil {
		return contract.ArtifactRelationFactV1{}, contract.NewError(contract.ErrContentDigestMismatch, "artifact_relation", "stored Artifact Relation failed validation")
	}
	return fact.Clone(), nil
}

func validateArtifactRelationByIDRequestV1(request ports.InspectArtifactRelationByIDRequestV1) error {
	for field, value := range map[string]string{"tenant_id": request.TenantID, "scope_digest": request.ScopeDigest, "relation_id": request.RelationID} {
		if err := contract.ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := request.Owner.Validate(); err != nil {
		return err
	}
	if request.Owner.ComponentID != contract.ContinuityComponentID || request.Owner.Capability != contract.ArtifactRelationCapabilityV1 || request.Owner.FactKind != "artifact_relation_fact_v1" {
		return contract.NewError(contract.ErrInvalidArgument, "owner_binding", "wrong Continuity Artifact Relation owner")
	}
	return nil
}

var _ ports.ArtifactRelationReaderV1 = (*Store)(nil)
