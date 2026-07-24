package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func (s *Store) CreateWorkspaceRewindCompositionV1(ctx context.Context, fact contract.WorkspaceRewindCompositionFactV1) (contract.WorkspaceRewindCompositionFactV1, error) {
	now := s.clock()
	if err := fact.ValidateCurrent(now); err != nil {
		return contract.WorkspaceRewindCompositionFactV1{}, err
	}
	body, err := encode(fact)
	if err != nil {
		return contract.WorkspaceRewindCompositionFactV1{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.WorkspaceRewindCompositionFactV1{}, err
	}
	defer tx.Rollback()
	if existing, found, err := inspectWorkspaceRewindByRequestTxV1(ctx, tx, fact.TenantID, fact.ScopeDigest, fact.RequestID); err != nil {
		return contract.WorkspaceRewindCompositionFactV1{}, err
	} else if found {
		if !contract.SameWorkspaceRewindCompositionV1(existing, fact) {
			return contract.WorkspaceRewindCompositionFactV1{}, ports.ErrConflict
		}
		return existing.Clone(), tx.Commit()
	}
	var boundRequest, boundDigest string
	err = tx.QueryRowContext(ctx, `SELECT request_id,request_digest FROM workspace_rewind_composition_facts WHERE tenant_id=? AND scope_digest=? AND idempotency_key=?`, fact.TenantID, fact.ScopeDigest, fact.IdempotencyKey).Scan(&boundRequest, &boundDigest)
	if err == nil && (boundRequest != fact.RequestID || boundDigest != fact.RequestDigest) {
		return contract.WorkspaceRewindCompositionFactV1{}, ports.ErrConflict
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return contract.WorkspaceRewindCompositionFactV1{}, err
	}
	if err := validateWorkspaceRewindClosureTxV1(ctx, tx, fact, now); err != nil {
		return contract.WorkspaceRewindCompositionFactV1{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_rewind_composition_facts(
		fact_id,revision,digest,tenant_id,scope_digest,request_id,idempotency_key,request_digest,
		planned_change_set_id,planned_change_set_revision,planned_change_set_digest,body)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, fact.Meta.ID, fact.Meta.Revision, fact.Meta.Digest,
		fact.TenantID, fact.ScopeDigest, fact.RequestID, fact.IdempotencyKey, fact.RequestDigest,
		fact.PlannedChangeSetRef.ID, fact.PlannedChangeSetRef.Revision, fact.PlannedChangeSetRef.Digest, body); err != nil {
		return contract.WorkspaceRewindCompositionFactV1{}, classifyWrite(err)
	}
	if err := tx.Commit(); err != nil {
		return contract.WorkspaceRewindCompositionFactV1{}, err
	}
	return fact.Clone(), nil
}

func (s *Store) InspectWorkspaceRewindCompositionV1(ctx context.Context, exact contract.Ref) (contract.WorkspaceRewindCompositionFactV1, error) {
	if err := exact.ValidateShape("workspace rewind composition"); err != nil {
		return contract.WorkspaceRewindCompositionFactV1{}, err
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM workspace_rewind_composition_facts WHERE fact_id=? AND revision=? AND digest=?`, exact.ID, exact.Revision, exact.Digest).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceRewindCompositionFactV1{}, ports.ErrNotFound
		}
		return contract.WorkspaceRewindCompositionFactV1{}, err
	}
	return decodeWorkspaceRewindCompositionV1(body)
}

func (s *Store) InspectWorkspaceRewindCompositionByRequestV1(ctx context.Context, tenantID, scopeDigest, requestID string) (contract.WorkspaceRewindCompositionFactV1, error) {
	if tenantID == "" || !contract.ValidDigest(scopeDigest) || requestID == "" {
		return contract.WorkspaceRewindCompositionFactV1{}, errors.New("workspace rewind request coordinates are invalid")
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM workspace_rewind_composition_facts WHERE tenant_id=? AND scope_digest=? AND request_id=?`, tenantID, scopeDigest, requestID).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceRewindCompositionFactV1{}, ports.ErrNotFound
		}
		return contract.WorkspaceRewindCompositionFactV1{}, err
	}
	return decodeWorkspaceRewindCompositionV1(body)
}

func inspectWorkspaceRewindByRequestTxV1(ctx context.Context, tx *sql.Tx, tenantID, scopeDigest, requestID string) (contract.WorkspaceRewindCompositionFactV1, bool, error) {
	var body []byte
	err := tx.QueryRowContext(ctx, `SELECT body FROM workspace_rewind_composition_facts WHERE tenant_id=? AND scope_digest=? AND request_id=?`, tenantID, scopeDigest, requestID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.WorkspaceRewindCompositionFactV1{}, false, nil
	}
	if err != nil {
		return contract.WorkspaceRewindCompositionFactV1{}, false, err
	}
	var fact contract.WorkspaceRewindCompositionFactV1
	if err := decode(body, &fact); err != nil || fact.ValidateShape() != nil {
		return contract.WorkspaceRewindCompositionFactV1{}, false, errors.New("stored workspace rewind composition is invalid")
	}
	return fact.Clone(), true, nil
}

func validateWorkspaceRewindClosureTxV1(ctx context.Context, tx *sql.Tx, fact contract.WorkspaceRewindCompositionFactV1, now time.Time) error {
	view, err := inspectWorkspaceViewCurrentTxV1(ctx, tx, fact.SourceWorkspaceViewRef, now)
	if err != nil {
		return err
	}
	if view.Lease.TenantID != fact.TenantID || view.Lease.ScopeDigest != fact.ScopeDigest || view.BaseRevision != fact.ExpectedBaseRevision || view.FileScopeDigest != fact.ExpectedFileScopeDigest {
		return ports.ErrConflict
	}
	if fact.Meta.ExpiresUnixNano > view.Meta.ExpiresUnixNano || fact.Meta.ExpiresUnixNano > view.Lease.ExpiresUnixNano {
		return ports.ErrConflict
	}
	keep, err := inspectWorkspaceChangeSetRefsTxV1(ctx, tx, fact.KeepChangeSetRefs)
	if err != nil {
		return err
	}
	drop, err := inspectWorkspaceChangeSetRefsTxV1(ctx, tx, fact.DropChangeSetRefs)
	if err != nil {
		return err
	}
	for _, source := range append(append([]contract.WorkspaceChangeSet(nil), keep...), drop...) {
		if source.Meta.ExpiresUnixNano < fact.Meta.ExpiresUnixNano {
			return ports.ErrConflict
		}
	}
	planned, err := inspectWorkspaceChangeSetHistoryTxV1(ctx, tx, fact.PlannedChangeSetRef)
	if err != nil {
		return err
	}
	changes, err := contract.ComposeWorkspaceRewindChangesV1(view, keep, drop)
	if err != nil || planned.ValidateCurrent(now) != nil || planned.State != contract.ChangeSetStaged ||
		planned.Meta.ExpiresUnixNano != fact.Meta.ExpiresUnixNano ||
		!contract.SameRef(planned.ViewRef, view.Meta.Ref()) || planned.BaseRevision != view.BaseRevision ||
		planned.BaseArtifactRef != view.BaseArtifactRef || !reflect.DeepEqual(planned.Changes, changes) {
		return ports.ErrConflict
	}
	return nil
}

func inspectWorkspaceViewCurrentTxV1(ctx context.Context, tx *sql.Tx, exact contract.Ref, now time.Time) (contract.WorkspaceView, error) {
	var revision uint64
	var digest string
	var body []byte
	if err := tx.QueryRowContext(ctx, `SELECT revision,digest,body FROM workspace_view_current WHERE view_id=?`, exact.ID).Scan(&revision, &digest, &body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceView{}, ports.ErrNotFound
		}
		return contract.WorkspaceView{}, err
	}
	if revision != exact.Revision || digest != exact.Digest {
		return contract.WorkspaceView{}, ports.ErrConflict
	}
	var view contract.WorkspaceView
	if err := decode(body, &view); err != nil || view.ValidateCurrent(now) != nil {
		return contract.WorkspaceView{}, ports.ErrConflict
	}
	return view, nil
}

func inspectWorkspaceChangeSetRefsTxV1(ctx context.Context, tx *sql.Tx, refs []contract.Ref) ([]contract.WorkspaceChangeSet, error) {
	result := make([]contract.WorkspaceChangeSet, len(refs))
	for i, ref := range refs {
		set, err := inspectWorkspaceChangeSetHistoryTxV1(ctx, tx, ref)
		if err != nil {
			return nil, err
		}
		result[i] = set
	}
	return result, nil
}

func inspectWorkspaceChangeSetHistoryTxV1(ctx context.Context, tx *sql.Tx, exact contract.Ref) (contract.WorkspaceChangeSet, error) {
	var body []byte
	if err := tx.QueryRowContext(ctx, `SELECT body FROM workspace_change_set_history WHERE change_set_id=? AND revision=? AND digest=?`, exact.ID, exact.Revision, exact.Digest).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceChangeSet{}, ports.ErrNotFound
		}
		return contract.WorkspaceChangeSet{}, err
	}
	var set contract.WorkspaceChangeSet
	if err := decode(body, &set); err != nil || set.ValidateShape() != nil {
		return contract.WorkspaceChangeSet{}, ports.ErrConflict
	}
	return set, nil
}

func decodeWorkspaceRewindCompositionV1(body []byte) (contract.WorkspaceRewindCompositionFactV1, error) {
	var fact contract.WorkspaceRewindCompositionFactV1
	if err := decode(body, &fact); err != nil {
		return contract.WorkspaceRewindCompositionFactV1{}, err
	}
	if err := fact.ValidateShape(); err != nil {
		return contract.WorkspaceRewindCompositionFactV1{}, err
	}
	return fact.Clone(), nil
}

var _ ports.WorkspaceRewindCompositionRepositoryV1 = (*Store)(nil)
