package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func (s *Store) CreateWorkspaceViewV1(ctx context.Context, value contract.WorkspaceView) (contract.WorkspaceView, error) {
	if err := value.ValidateCurrent(s.clock()); err != nil {
		return contract.WorkspaceView{}, err
	}
	body, err := encode(value)
	if err != nil {
		return contract.WorkspaceView{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.WorkspaceView{}, err
	}
	defer tx.Rollback()
	var existingBody []byte
	if err := tx.QueryRowContext(ctx, `SELECT body FROM workspace_view_current WHERE view_id=?`, value.Meta.ID).Scan(&existingBody); err == nil {
		var existing contract.WorkspaceView
		if decode(existingBody, &existing) != nil || !contract.SameRef(existing.Meta.Ref(), value.Meta.Ref()) {
			return contract.WorkspaceView{}, ports.ErrConflict
		}
		return existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return contract.WorkspaceView{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_view_history(view_id,revision,digest,body) VALUES(?,?,?,?)`, value.Meta.ID, value.Meta.Revision, value.Meta.Digest, body); err != nil {
		return contract.WorkspaceView{}, classifyWrite(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_view_current(view_id,revision,digest,body) VALUES(?,?,?,?)`, value.Meta.ID, value.Meta.Revision, value.Meta.Digest, body); err != nil {
		return contract.WorkspaceView{}, classifyWrite(err)
	}
	if err := tx.Commit(); err != nil {
		return contract.WorkspaceView{}, err
	}
	return cloneWorkspaceV1(value)
}

func (s *Store) CreateWorkspaceChangeSetV1(ctx context.Context, value contract.WorkspaceChangeSet) (contract.WorkspaceChangeSet, error) {
	if err := value.ValidateCurrent(s.clock()); err != nil || value.Meta.Revision != 1 || value.State != contract.ChangeSetStaged {
		if err != nil {
			return contract.WorkspaceChangeSet{}, err
		}
		return contract.WorkspaceChangeSet{}, ports.ErrConflict
	}
	body, err := encode(value)
	if err != nil {
		return contract.WorkspaceChangeSet{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.WorkspaceChangeSet{}, err
	}
	defer tx.Rollback()
	var viewRevision uint64
	var viewDigest string
	if err := tx.QueryRowContext(ctx, `SELECT revision,digest FROM workspace_view_current WHERE view_id=?`, value.ViewRef.ID).Scan(&viewRevision, &viewDigest); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceChangeSet{}, ports.ErrNotFound
		}
		return contract.WorkspaceChangeSet{}, err
	}
	if viewRevision != value.ViewRef.Revision || viewDigest != value.ViewRef.Digest {
		return contract.WorkspaceChangeSet{}, ports.ErrConflict
	}
	var existingBody []byte
	if err := tx.QueryRowContext(ctx, `SELECT body FROM workspace_change_set_current WHERE change_set_id=?`, value.Meta.ID).Scan(&existingBody); err == nil {
		var existing contract.WorkspaceChangeSet
		if decode(existingBody, &existing) != nil || !contract.SameRef(existing.Meta.Ref(), value.Meta.Ref()) {
			return contract.WorkspaceChangeSet{}, ports.ErrConflict
		}
		return existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return contract.WorkspaceChangeSet{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_change_set_history(change_set_id,revision,digest,body) VALUES(?,?,?,?)`, value.Meta.ID, value.Meta.Revision, value.Meta.Digest, body); err != nil {
		return contract.WorkspaceChangeSet{}, classifyWrite(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_change_set_current(change_set_id,revision,digest,body) VALUES(?,?,?,?)`, value.Meta.ID, value.Meta.Revision, value.Meta.Digest, body); err != nil {
		return contract.WorkspaceChangeSet{}, classifyWrite(err)
	}
	if err := tx.Commit(); err != nil {
		return contract.WorkspaceChangeSet{}, err
	}
	return cloneWorkspaceV1(value)
}

func (s *Store) InspectWorkspaceViewCurrentV1(ctx context.Context, expected contract.Ref) (contract.WorkspaceView, error) {
	if err := expected.ValidateShape("expected workspace view"); err != nil {
		return contract.WorkspaceView{}, err
	}
	var revision uint64
	var digest string
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT revision,digest,body FROM workspace_view_current WHERE view_id=?`, expected.ID).Scan(&revision, &digest, &body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceView{}, ports.ErrNotFound
		}
		return contract.WorkspaceView{}, err
	}
	if revision != expected.Revision || digest != expected.Digest {
		return contract.WorkspaceView{}, ports.ErrConflict
	}
	var value contract.WorkspaceView
	if err := decode(body, &value); err != nil {
		return contract.WorkspaceView{}, err
	}
	return value, value.ValidateCurrent(s.clock())
}

func (s *Store) InspectWorkspaceChangeSetCurrentV1(ctx context.Context, expected contract.Ref) (contract.WorkspaceChangeSet, error) {
	if err := expected.ValidateShape("expected workspace change set"); err != nil {
		return contract.WorkspaceChangeSet{}, err
	}
	var revision uint64
	var digest string
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT revision,digest,body FROM workspace_change_set_current WHERE change_set_id=?`, expected.ID).Scan(&revision, &digest, &body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceChangeSet{}, ports.ErrNotFound
		}
		return contract.WorkspaceChangeSet{}, err
	}
	if revision != expected.Revision || digest != expected.Digest {
		return contract.WorkspaceChangeSet{}, ports.ErrConflict
	}
	var value contract.WorkspaceChangeSet
	if err := decode(body, &value); err != nil {
		return contract.WorkspaceChangeSet{}, err
	}
	return value, value.ValidateCurrent(s.clock())
}

func (s *Store) InspectWorkspaceChangeSetOwnerCurrentByIDV1(ctx context.Context, id string) (contract.WorkspaceChangeSet, error) {
	if id == "" {
		return contract.WorkspaceChangeSet{}, errors.New("workspace change set ID is empty")
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM workspace_change_set_current WHERE change_set_id=?`, id).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceChangeSet{}, ports.ErrNotFound
		}
		return contract.WorkspaceChangeSet{}, err
	}
	var value contract.WorkspaceChangeSet
	if err := decode(body, &value); err != nil {
		return contract.WorkspaceChangeSet{}, err
	}
	return value, value.ValidateCurrent(s.clock())
}

func (s *Store) InspectWorkspaceViewHistoryV1(ctx context.Context, exact contract.Ref) (contract.WorkspaceView, error) {
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM workspace_view_history WHERE view_id=? AND revision=? AND digest=?`, exact.ID, exact.Revision, exact.Digest).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceView{}, ports.ErrNotFound
		}
		return contract.WorkspaceView{}, err
	}
	var value contract.WorkspaceView
	if err := decode(body, &value); err != nil {
		return contract.WorkspaceView{}, err
	}
	return value, value.ValidateShape()
}

func (s *Store) InspectWorkspaceChangeSetHistoryV1(ctx context.Context, exact contract.Ref) (contract.WorkspaceChangeSet, error) {
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM workspace_change_set_history WHERE change_set_id=? AND revision=? AND digest=?`, exact.ID, exact.Revision, exact.Digest).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceChangeSet{}, ports.ErrNotFound
		}
		return contract.WorkspaceChangeSet{}, err
	}
	var value contract.WorkspaceChangeSet
	if err := decode(body, &value); err != nil {
		return contract.WorkspaceChangeSet{}, err
	}
	return value, value.ValidateShape()
}

func (s *Store) CompareAndSwapWorkspaceChangeSetV1(ctx context.Context, expected contract.Ref, next contract.WorkspaceChangeSet) error {
	if err := expected.ValidateShape("expected workspace change set"); err != nil {
		return err
	}
	if err := next.ValidateCurrent(s.clock()); err != nil || next.Meta.ID != expected.ID || next.Meta.Revision != expected.Revision+1 {
		if err != nil {
			return err
		}
		return ports.ErrConflict
	}
	body, err := encode(next)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var revision uint64
	var digest string
	if err := tx.QueryRowContext(ctx, `SELECT revision,digest FROM workspace_change_set_current WHERE change_set_id=?`, expected.ID).Scan(&revision, &digest); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ports.ErrNotFound
		}
		return err
	}
	if revision == next.Meta.Revision && digest == next.Meta.Digest {
		return nil
	}
	if revision != expected.Revision || digest != expected.Digest {
		return ports.ErrConflict
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_change_set_history(change_set_id,revision,digest,body) VALUES(?,?,?,?)`, next.Meta.ID, next.Meta.Revision, next.Meta.Digest, body); err != nil {
		return classifyWrite(err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE workspace_change_set_current SET revision=?,digest=?,body=? WHERE change_set_id=? AND revision=? AND digest=?`, next.Meta.Revision, next.Meta.Digest, body, expected.ID, expected.Revision, expected.Digest)
	if err != nil {
		return classifyWrite(err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return ports.ErrConflict
	}
	return tx.Commit()
}

func cloneWorkspaceV1[T any](value T) (T, error) {
	var result T
	body, err := encode(value)
	if err != nil {
		return result, err
	}
	if err := decode(body, &result); err != nil {
		return result, err
	}
	return result, nil
}

var _ ports.WorkspaceOwnerStoreV1 = (*Store)(nil)
