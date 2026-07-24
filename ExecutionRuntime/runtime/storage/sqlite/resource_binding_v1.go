package sqlite

import (
	"context"
	"database/sql"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func loadResourceHandleHistoryV1(ctx context.Context, source queryRower, ref ports.ResourceHandleRefV1) (ports.ResourceHandleCurrentV1, error) {
	var digest, rowDigest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT projection_digest,row_digest,canonical_json FROM runtime_resource_handle_history WHERE owner_domain=? AND owner_id=? AND handle_id=? AND revision=?`, ref.Owner.Domain, ref.Owner.ID, ref.ID, ref.Revision).Scan(&digest, &rowDigest, &payload)
	if err == sql.ErrNoRows {
		return ports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Resource handle history is absent")
	}
	if err != nil {
		return ports.ResourceHandleCurrentV1{}, mapDBError(ctx, err, false)
	}
	projection, err := decodeRow[ports.ResourceHandleCurrentV1](payload, rowDigest, "ResourceHandleCurrentV1")
	if err != nil {
		return ports.ResourceHandleCurrentV1{}, err
	}
	if projection.Ref != ref || string(projection.ProjectionDigest) != digest {
		return ports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Resource handle history coordinates drifted")
	}
	return projection, projection.Validate()
}

func loadResourceHandleCurrentV1(ctx context.Context, source queryRower, ref ports.ResourceHandleRefV1) (ports.ResourceHandleCurrentV1, error) {
	var revision, highest uint64
	var digest string
	err := source.QueryRowContext(ctx, `SELECT revision,projection_digest,highest_revision FROM runtime_resource_handle_current WHERE owner_domain=? AND owner_id=? AND handle_id=?`, ref.Owner.Domain, ref.Owner.ID, ref.ID).Scan(&revision, &digest, &highest)
	if err == sql.ErrNoRows {
		return ports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Resource handle current is absent")
	}
	if err != nil {
		return ports.ResourceHandleCurrentV1{}, mapDBError(ctx, err, false)
	}
	if core.Revision(revision) != ref.Revision || core.Revision(highest) != ref.Revision || core.Digest(digest) != ref.Digest {
		return ports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Resource handle exact current Ref drifted")
	}
	return loadResourceHandleHistoryV1(ctx, source, ref)
}

func insertResourceHandleV1(ctx context.Context, tx *sql.Tx, value ports.ResourceHandleCurrentV1) error {
	payload, err := marshalStrict(value)
	if err != nil {
		return err
	}
	rowDigest, err := digestRow("ResourceHandleCurrentV1", value)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO runtime_resource_handle_history(owner_domain,owner_id,handle_id,revision,projection_digest,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?)`, value.Ref.Owner.Domain, value.Ref.Owner.ID, value.Ref.ID, value.Ref.Revision, string(value.ProjectionDigest), string(rowDigest), payload); err != nil {
		return mapDBError(ctx, err, true)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_resource_handle_current(owner_domain,owner_id,handle_id,revision,projection_digest,highest_revision) VALUES(?,?,?,?,?,?)`, value.Ref.Owner.Domain, value.Ref.Owner.ID, value.Ref.ID, value.Ref.Revision, string(value.ProjectionDigest), value.Ref.Revision)
	return mapDBError(ctx, err, true)
}

func loadResourceBindingSetHistoryV1(ctx context.Context, source queryRower, ref ports.ResourceBindingSetRefV1) (ports.ResourceBindingSetV1, error) {
	var digest, rowDigest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT projection_digest,row_digest,canonical_json FROM runtime_resource_binding_set_history WHERE set_id=? AND revision=?`, ref.ID, ref.Revision).Scan(&digest, &rowDigest, &payload)
	if err == sql.ErrNoRows {
		return ports.ResourceBindingSetV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Resource BindingSet history is absent")
	}
	if err != nil {
		return ports.ResourceBindingSetV1{}, mapDBError(ctx, err, false)
	}
	set, err := decodeRow[ports.ResourceBindingSetV1](payload, rowDigest, "ResourceBindingSetV1")
	if err != nil {
		return ports.ResourceBindingSetV1{}, err
	}
	if set.Ref != ref || string(set.ProjectionDigest) != digest {
		return ports.ResourceBindingSetV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Resource BindingSet history coordinates drifted")
	}
	return set, set.Validate()
}

func loadResourceBindingSetCurrentV1(ctx context.Context, source queryRower, ref ports.ResourceBindingSetRefV1) (ports.ResourceBindingSetV1, error) {
	var revision, highest uint64
	var digest string
	err := source.QueryRowContext(ctx, `SELECT revision,projection_digest,highest_revision FROM runtime_resource_binding_set_current WHERE set_id=?`, ref.ID).Scan(&revision, &digest, &highest)
	if err == sql.ErrNoRows {
		return ports.ResourceBindingSetV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Resource BindingSet current is absent")
	}
	if err != nil {
		return ports.ResourceBindingSetV1{}, mapDBError(ctx, err, false)
	}
	if core.Revision(revision) != ref.Revision || core.Revision(highest) != ref.Revision || core.Digest(digest) != ref.Digest {
		return ports.ResourceBindingSetV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Resource BindingSet exact current Ref drifted")
	}
	return loadResourceBindingSetHistoryV1(ctx, source, ref)
}

func insertResourceBindingSetV1(ctx context.Context, tx *sql.Tx, set ports.ResourceBindingSetV1) error {
	payload, err := marshalStrict(set)
	if err != nil {
		return err
	}
	rowDigest, err := digestRow("ResourceBindingSetV1", set)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO runtime_resource_binding_set_history(set_id,revision,projection_digest,row_digest,canonical_json) VALUES(?,?,?,?,?)`, set.Ref.ID, set.Ref.Revision, string(set.ProjectionDigest), string(rowDigest), payload); err != nil {
		return mapDBError(ctx, err, true)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_resource_binding_set_current(set_id,revision,projection_digest,highest_revision) VALUES(?,?,?,?)`, set.Ref.ID, set.Ref.Revision, string(set.ProjectionDigest), set.Ref.Revision)
	return mapDBError(ctx, err, true)
}

func (s *Store) EnsureResourceHandleCurrentV1(ctx context.Context, value ports.ResourceHandleCurrentV1) (ports.ResourceHandleCurrentV1, error) {
	if err := contextError(ctx, "Ensure Resource handle current"); err != nil {
		return ports.ResourceHandleCurrentV1{}, err
	}
	if s == nil || s.db == nil {
		return ports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "runtime Binding sqlite is unavailable")
	}
	staged, err := cloneStrict(value)
	if err != nil {
		return ports.ResourceHandleCurrentV1{}, err
	}
	now := s.clock()
	if err := staged.ValidateCurrent(staged.Ref, now); err != nil {
		return ports.ResourceHandleCurrentV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ResourceHandleCurrentV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var revision uint64
	err = tx.QueryRowContext(ctx, `SELECT revision FROM runtime_resource_handle_current WHERE owner_domain=? AND owner_id=? AND handle_id=?`, staged.Ref.Owner.Domain, staged.Ref.Owner.ID, staged.Ref.ID).Scan(&revision)
	if err == nil {
		current, inspectErr := loadResourceHandleCurrentV1(ctx, tx, staged.Ref)
		if inspectErr == nil && current.ProjectionDigest == staged.ProjectionDigest {
			return cloneStrict(current)
		}
		return ports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Resource handle identity already contains different content")
	}
	if err != sql.ErrNoRows {
		return ports.ResourceHandleCurrentV1{}, mapDBError(ctx, err, false)
	}
	if s.consumeStageFailure() {
		return ports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Resource handle staged failure")
	}
	if err := insertResourceHandleV1(ctx, tx, staged); err != nil {
		return ports.ResourceHandleCurrentV1{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.ResourceHandleCurrentV1{}, err
	}
	if s.consumeLostReply() {
		return ports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Resource handle Ensure reply loss")
	}
	return cloneStrict(staged)
}

func (s *Store) InspectResourceHandleCurrentV1(ctx context.Context, exact ports.ResourceHandleRefV1) (ports.ResourceHandleCurrentV1, error) {
	if err := contextError(ctx, "Inspect Resource handle current"); err != nil {
		return ports.ResourceHandleCurrentV1{}, err
	}
	if s == nil || s.db == nil || exact.Validate() != nil {
		return ports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Resource handle exact lookup is invalid")
	}
	value, err := loadResourceHandleCurrentV1(ctx, s.db, exact)
	if err != nil {
		return ports.ResourceHandleCurrentV1{}, err
	}
	if err := value.ValidateCurrent(exact, s.clock()); err != nil {
		return ports.ResourceHandleCurrentV1{}, err
	}
	return cloneStrict(value)
}

func (s *Store) EnsureResourceBindingSetCurrentV1(ctx context.Context, value ports.ResourceBindingSetV1) (ports.ResourceBindingSetV1, error) {
	if err := contextError(ctx, "Ensure Resource BindingSet current"); err != nil {
		return ports.ResourceBindingSetV1{}, err
	}
	if s == nil || s.db == nil {
		return ports.ResourceBindingSetV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "runtime Binding sqlite is unavailable")
	}
	staged, err := cloneStrict(value)
	if err != nil {
		return ports.ResourceBindingSetV1{}, err
	}
	now := s.clock()
	if err := staged.ValidateCurrent(staged.Ref, now); err != nil {
		return ports.ResourceBindingSetV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.ResourceBindingSetV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var revision uint64
	err = tx.QueryRowContext(ctx, `SELECT revision FROM runtime_resource_binding_set_current WHERE set_id=?`, staged.Ref.ID).Scan(&revision)
	if err == nil {
		current, inspectErr := loadResourceBindingSetCurrentV1(ctx, tx, staged.Ref)
		if inspectErr == nil && current.ProjectionDigest == staged.ProjectionDigest {
			return cloneStrict(current)
		}
		return ports.ResourceBindingSetV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Resource BindingSet identity already contains different content")
	}
	if err != sql.ErrNoRows {
		return ports.ResourceBindingSetV1{}, mapDBError(ctx, err, false)
	}
	for _, binding := range staged.Bindings {
		handle, inspectErr := loadResourceHandleCurrentV1(ctx, tx, binding.Handle)
		if inspectErr != nil {
			return ports.ResourceBindingSetV1{}, inspectErr
		}
		if handle.CleanupContract != binding.CleanupContract || handle.DeploymentAttestation != binding.DeploymentAttestation || handle.Ref != binding.Handle || handle.ValidateCurrent(binding.Handle, now) != nil {
			return ports.ResourceBindingSetV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Resource BindingSet member is not exact current")
		}
	}
	if s.consumeStageFailure() {
		return ports.ResourceBindingSetV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Resource BindingSet staged failure")
	}
	if err := insertResourceBindingSetV1(ctx, tx, staged); err != nil {
		return ports.ResourceBindingSetV1{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.ResourceBindingSetV1{}, err
	}
	if s.consumeLostReply() {
		return ports.ResourceBindingSetV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Resource BindingSet Ensure reply loss")
	}
	return cloneStrict(staged)
}

func (s *Store) InspectResourceBindingSetCurrentV1(ctx context.Context, exact ports.ResourceBindingSetRefV1) (ports.ResourceBindingSetV1, error) {
	if err := contextError(ctx, "Inspect Resource BindingSet current"); err != nil {
		return ports.ResourceBindingSetV1{}, err
	}
	if s == nil || s.db == nil || exact.Validate() != nil {
		return ports.ResourceBindingSetV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Resource BindingSet exact lookup is invalid")
	}
	set, err := loadResourceBindingSetCurrentV1(ctx, s.db, exact)
	if err != nil {
		return ports.ResourceBindingSetV1{}, err
	}
	if err := set.ValidateCurrent(exact, s.clock()); err != nil {
		return ports.ResourceBindingSetV1{}, err
	}
	for _, binding := range set.Bindings {
		handle, inspectErr := s.InspectResourceHandleCurrentV1(ctx, binding.Handle)
		if inspectErr != nil || handle.CleanupContract != binding.CleanupContract || handle.DeploymentAttestation != binding.DeploymentAttestation {
			return ports.ResourceBindingSetV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Resource BindingSet member currentness drifted")
		}
	}
	return cloneStrict(set)
}

var _ ports.ResourceOwnerRepositoryV1 = (*Store)(nil)
