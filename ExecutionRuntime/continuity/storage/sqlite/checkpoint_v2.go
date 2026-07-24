package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func (s *Store) CreateCheckpointManifestFactV2(ctx context.Context, fact contract.CheckpointManifestFactV2) (contract.CheckpointManifestFactV2, bool, error) {
	if err := fact.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	if fact.Revision != 1 || fact.State != contract.ManifestCollecting {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrInvalidArgument, "manifest_create", "revision 1 collecting fact is required")
	}
	body, _, err := encode(fact)
	if err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	tenant, scope, id := fact.Scope.TenantID, fact.Scope.ExecutionScopeDigest, fact.ManifestID
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.CheckpointManifestFactV2{}, false, unavailable("begin manifest create", err)
	}
	defer tx.Rollback()
	var bound string
	err = tx.QueryRowContext(ctx, "SELECT manifest_id FROM checkpoint_manifest_idempotency WHERE tenant_id=? AND scope_digest=? AND idempotency_key=?", tenant, scope, fact.IdempotencyKey).Scan(&bound)
	if err == nil && bound != id {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "request already created another manifest in this tenant scope")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return contract.CheckpointManifestFactV2{}, false, unavailable("inspect manifest idempotency", err)
	}
	current, found, err := inspectCurrentManifestTx(ctx, tx, tenant, scope, id)
	if err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	if found {
		initial, err := inspectManifestRevisionTx(ctx, tx, tenant, scope, id, 1)
		if err == nil && initial.Ref().Exact().Equal(fact.Ref().Exact()) {
			return initial, true, tx.Commit()
		}
		_ = current
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "manifest_id", "create-once manifest identity changed")
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO checkpoint_manifest_history(tenant_id,scope_digest,manifest_id,revision,ref_digest,body) VALUES(?,?,?,?,?,?)", tenant, scope, id, 1, fact.Ref().Exact().Digest, body); err != nil {
		return contract.CheckpointManifestFactV2{}, false, unavailable("insert manifest history", err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO checkpoint_manifest_current(tenant_id,scope_digest,manifest_id,revision) VALUES(?,?,?,1)", tenant, scope, id); err != nil {
		return contract.CheckpointManifestFactV2{}, false, unavailable("insert manifest current", err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO checkpoint_manifest_idempotency(tenant_id,scope_digest,idempotency_key,manifest_id) VALUES(?,?,?,?)", tenant, scope, fact.IdempotencyKey, id); err != nil {
		return contract.CheckpointManifestFactV2{}, false, unavailable("insert manifest idempotency", err)
	}
	if err = tx.Commit(); err != nil {
		return contract.CheckpointManifestFactV2{}, false, unavailable("commit manifest create", err)
	}
	return fact.Clone(), false, nil
}

func (s *Store) CompareAndSwapCheckpointManifestFactV2(ctx context.Context, expected contract.CheckpointManifestRefV2, next contract.CheckpointManifestFactV2) (contract.CheckpointManifestFactV2, bool, error) {
	if err := expected.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	if err := next.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	ref := expected.Exact()
	if ref.TenantID != next.Scope.TenantID || ref.ScopeDigest != next.Scope.ExecutionScopeDigest || ref.ID != next.ManifestID {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "manifest_key", "tenant, scope, or manifest ID changed")
	}
	body, _, err := encode(next)
	if err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.CheckpointManifestFactV2{}, false, unavailable("begin manifest CAS", err)
	}
	defer tx.Rollback()
	current, found, err := inspectCurrentManifestTx(ctx, tx, ref.TenantID, ref.ScopeDigest, ref.ID)
	if err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	if !found {
		return contract.CheckpointManifestFactV2{}, false, notFound("manifest_key", "manifest not found in tenant scope")
	}
	if current.Revision == ref.Revision+1 && current.Ref().Exact().Equal(next.Ref().Exact()) {
		return current, true, tx.Commit()
	}
	if !current.Ref().Exact().Equal(ref) {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "manifest_ref", "CAS expected ref is not current")
	}
	if next.Revision != current.Revision+1 {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "manifest_revision", "CAS must advance exactly one revision")
	}
	if err := validateManifestMutation(current, next); err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	if err := contract.AdvanceCheckpointManifestStateV2(current.State, next.State); err != nil {
		return contract.CheckpointManifestFactV2{}, false, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO checkpoint_manifest_history(tenant_id,scope_digest,manifest_id,revision,ref_digest,body) VALUES(?,?,?,?,?,?)", ref.TenantID, ref.ScopeDigest, ref.ID, next.Revision, next.Ref().Exact().Digest, body); err != nil {
		return contract.CheckpointManifestFactV2{}, false, unavailable("insert manifest revision", err)
	}
	result, err := tx.ExecContext(ctx, "UPDATE checkpoint_manifest_current SET revision=? WHERE tenant_id=? AND scope_digest=? AND manifest_id=? AND revision=?", next.Revision, ref.TenantID, ref.ScopeDigest, ref.ID, ref.Revision)
	if err != nil {
		return contract.CheckpointManifestFactV2{}, false, unavailable("update manifest current", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return contract.CheckpointManifestFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "manifest_ref", "manifest current CAS lost")
	}
	if err = tx.Commit(); err != nil {
		return contract.CheckpointManifestFactV2{}, false, unavailable("commit manifest CAS", err)
	}
	return next.Clone(), false, nil
}

func (s *Store) InspectCheckpointManifestV2(ctx context.Context, request ports.InspectCheckpointManifestRequestV2) (contract.CheckpointManifestFactV2, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	if err := request.Ref.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	ref := request.Ref.Exact()
	var body []byte
	if err := s.db.QueryRowContext(ctx, "SELECT body FROM checkpoint_manifest_history WHERE tenant_id=? AND scope_digest=? AND manifest_id=? AND revision=?", ref.TenantID, ref.ScopeDigest, ref.ID, ref.Revision).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.CheckpointManifestFactV2{}, notFound("manifest_revision", "manifest revision not found")
		}
		return contract.CheckpointManifestFactV2{}, unavailable("inspect manifest", err)
	}
	fact, err := decodeManifest(body)
	if err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	if !fact.Ref().Exact().Equal(ref) {
		return contract.CheckpointManifestFactV2{}, contract.NewError(contract.ErrRevisionConflict, "manifest_ref", "exact manifest ref or owner mismatch")
	}
	return fact, nil
}

func (s *Store) InspectCurrentCheckpointManifestV2(ctx context.Context, request ports.InspectCurrentCheckpointManifestRequestV2) (contract.CheckpointManifestFactV2, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	if err := request.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	var body []byte
	err := s.db.QueryRowContext(ctx, `SELECT h.body FROM checkpoint_manifest_current c
		JOIN checkpoint_manifest_history h ON h.tenant_id=c.tenant_id AND h.scope_digest=c.scope_digest AND h.manifest_id=c.manifest_id AND h.revision=c.revision
		WHERE c.tenant_id=? AND c.scope_digest=? AND c.manifest_id=?`, request.TenantID, request.ScopeDigest, request.ManifestID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.CheckpointManifestFactV2{}, notFound("manifest_key", "manifest not found in tenant scope")
	}
	if err != nil {
		return contract.CheckpointManifestFactV2{}, unavailable("inspect current manifest", err)
	}
	fact, err := decodeManifest(body)
	if err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	if fact.Owner != request.Owner {
		return contract.CheckpointManifestFactV2{}, contract.NewError(contract.ErrRevisionConflict, "owner_binding", "current manifest owner mismatch")
	}
	return fact, nil
}

func inspectCurrentManifestTx(ctx context.Context, tx *sql.Tx, tenant, scope, id string) (contract.CheckpointManifestFactV2, bool, error) {
	var body []byte
	err := tx.QueryRowContext(ctx, `SELECT h.body FROM checkpoint_manifest_current c
		JOIN checkpoint_manifest_history h ON h.tenant_id=c.tenant_id AND h.scope_digest=c.scope_digest AND h.manifest_id=c.manifest_id AND h.revision=c.revision
		WHERE c.tenant_id=? AND c.scope_digest=? AND c.manifest_id=?`, tenant, scope, id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.CheckpointManifestFactV2{}, false, nil
	}
	if err != nil {
		return contract.CheckpointManifestFactV2{}, false, unavailable("inspect current manifest", err)
	}
	fact, err := decodeManifest(body)
	return fact, true, err
}

func inspectManifestRevisionTx(ctx context.Context, tx *sql.Tx, tenant, scope, id string, revision uint64) (contract.CheckpointManifestFactV2, error) {
	var body []byte
	if err := tx.QueryRowContext(ctx, "SELECT body FROM checkpoint_manifest_history WHERE tenant_id=? AND scope_digest=? AND manifest_id=? AND revision=?", tenant, scope, id, revision).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.CheckpointManifestFactV2{}, notFound("manifest_revision", "manifest revision not found")
		}
		return contract.CheckpointManifestFactV2{}, unavailable("inspect manifest revision", err)
	}
	return decodeManifest(body)
}

func decodeManifest(body []byte) (contract.CheckpointManifestFactV2, error) {
	var fact contract.CheckpointManifestFactV2
	if err := decode(body, &fact); err != nil {
		return contract.CheckpointManifestFactV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return contract.CheckpointManifestFactV2{}, contract.NewError(contract.ErrContentDigestMismatch, "checkpoint_manifest", "stored manifest failed validation")
	}
	return fact.Clone(), nil
}

func (s *Store) CreateCheckpointManifestSealFactV2(ctx context.Context, seal contract.CheckpointManifestSealFactV2) (contract.CheckpointManifestSealFactV2, bool, error) {
	if err := seal.Validate(); err != nil {
		return contract.CheckpointManifestSealFactV2{}, false, err
	}
	body, _, err := encode(seal)
	if err != nil {
		return contract.CheckpointManifestSealFactV2{}, false, err
	}
	manifestRef := seal.ManifestRef.Exact()
	identityDigest, err := contract.CanonicalDigest(manifestRef.IdentityKey())
	if err != nil {
		return contract.CheckpointManifestSealFactV2{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.CheckpointManifestSealFactV2{}, false, unavailable("begin manifest seal", err)
	}
	defer tx.Rollback()
	current, found, err := inspectCurrentManifestTx(ctx, tx, manifestRef.TenantID, manifestRef.ScopeDigest, manifestRef.ID)
	if err != nil {
		return contract.CheckpointManifestSealFactV2{}, false, err
	}
	if !found {
		return contract.CheckpointManifestSealFactV2{}, false, notFound("manifest_key", "seal manifest not found in tenant scope")
	}
	if !current.Ref().Exact().Equal(manifestRef) {
		return contract.CheckpointManifestSealFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "manifest_ref", "seal manifest ref is not current")
	}
	if err := contract.ValidateCheckpointManifestSealBindingV2(current, seal); err != nil {
		return contract.CheckpointManifestSealFactV2{}, false, err
	}
	sealRef := seal.Ref().Exact()
	var existingBody []byte
	err = tx.QueryRowContext(ctx, "SELECT body FROM checkpoint_manifest_seals WHERE tenant_id=? AND scope_digest=? AND seal_id=?", sealRef.TenantID, sealRef.ScopeDigest, sealRef.ID).Scan(&existingBody)
	if err == nil {
		existing, err := decodeSeal(existingBody)
		if err != nil {
			return contract.CheckpointManifestSealFactV2{}, false, err
		}
		if existing.Ref().Exact().Equal(sealRef) {
			return existing, true, tx.Commit()
		}
		return contract.CheckpointManifestSealFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "seal_id", "immutable seal content changed")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return contract.CheckpointManifestSealFactV2{}, false, unavailable("inspect seal", err)
	}
	var conflictID string
	if err := tx.QueryRowContext(ctx, "SELECT seal_id FROM checkpoint_manifest_seals WHERE tenant_id=? AND scope_digest=? AND idempotency_key=?", sealRef.TenantID, sealRef.ScopeDigest, seal.IdempotencyKey).Scan(&conflictID); err == nil {
		return contract.CheckpointManifestSealFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "request already created another seal in this tenant scope")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return contract.CheckpointManifestSealFactV2{}, false, unavailable("inspect seal idempotency", err)
	}
	if err := tx.QueryRowContext(ctx, "SELECT seal_id FROM checkpoint_manifest_seals WHERE manifest_identity_digest=?", identityDigest).Scan(&conflictID); err == nil {
		return contract.CheckpointManifestSealFactV2{}, false, contract.NewError(contract.ErrRevisionConflict, "manifest_ref", "exact manifest revision already has a seal")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return contract.CheckpointManifestSealFactV2{}, false, unavailable("inspect manifest seal", err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO checkpoint_manifest_seals(tenant_id,scope_digest,seal_id,idempotency_key,manifest_identity_digest,ref_digest,body) VALUES(?,?,?,?,?,?,?)", sealRef.TenantID, sealRef.ScopeDigest, sealRef.ID, seal.IdempotencyKey, identityDigest, sealRef.Digest, body); err != nil {
		return contract.CheckpointManifestSealFactV2{}, false, unavailable("insert manifest seal", err)
	}
	if err = tx.Commit(); err != nil {
		return contract.CheckpointManifestSealFactV2{}, false, unavailable("commit manifest seal", err)
	}
	return seal.Clone(), false, nil
}

func (s *Store) InspectCheckpointManifestSealV2(ctx context.Context, request ports.InspectCheckpointManifestSealRequestV2) (contract.CheckpointManifestSealFactV2, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.CheckpointManifestSealFactV2{}, err
	}
	if err := request.Ref.Validate(); err != nil {
		return contract.CheckpointManifestSealFactV2{}, err
	}
	ref := request.Ref.Exact()
	var body []byte
	if err := s.db.QueryRowContext(ctx, "SELECT body FROM checkpoint_manifest_seals WHERE tenant_id=? AND scope_digest=? AND seal_id=?", ref.TenantID, ref.ScopeDigest, ref.ID).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.CheckpointManifestSealFactV2{}, notFound("seal_key", "manifest seal not found in tenant scope")
		}
		return contract.CheckpointManifestSealFactV2{}, unavailable("inspect manifest seal", err)
	}
	seal, err := decodeSeal(body)
	if err != nil {
		return contract.CheckpointManifestSealFactV2{}, err
	}
	if !seal.Ref().Exact().Equal(ref) {
		return contract.CheckpointManifestSealFactV2{}, contract.NewError(contract.ErrRevisionConflict, "manifest_seal_ref", "exact seal ref or owner mismatch")
	}
	return seal, nil
}

func decodeSeal(body []byte) (contract.CheckpointManifestSealFactV2, error) {
	var seal contract.CheckpointManifestSealFactV2
	if err := decode(body, &seal); err != nil {
		return contract.CheckpointManifestSealFactV2{}, err
	}
	if err := seal.Validate(); err != nil {
		return contract.CheckpointManifestSealFactV2{}, contract.NewError(contract.ErrContentDigestMismatch, "checkpoint_manifest_seal", "stored seal failed validation")
	}
	return seal.Clone(), nil
}

func validateManifestMutation(current, next contract.CheckpointManifestFactV2) error {
	currentFrames, err := contract.ExactRefSetDigestV2(current.ContextFrameRefs)
	if err != nil {
		return err
	}
	nextFrames, err := contract.ExactRefSetDigestV2(next.ContextFrameRefs)
	if err != nil {
		return err
	}
	if current.Owner != next.Owner || current.Scope != next.Scope || current.IdempotencyKey != next.IdempotencyKey ||
		!current.CheckpointAttemptRef.Equal(next.CheckpointAttemptRef) || !current.BarrierRef.Equal(next.BarrierRef) ||
		!current.EffectCutRef.Equal(next.EffectCutRef) || current.TimelineCut != next.TimelineCut ||
		!current.ContextGenerationRef.Equal(next.ContextGenerationRef) || currentFrames != nextFrames ||
		current.RequiredParticipantSetDigest != next.RequiredParticipantSetDigest || current.CreatedUnixNano != next.CreatedUnixNano {
		return contract.NewError(contract.ErrRevisionConflict, "manifest_identity", "immutable manifest identity changed")
	}
	return nil
}

var _ ports.CheckpointManifestRepositoryV2 = (*Store)(nil)
