package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/api"
)

func (s *Store) CreateOnce(ctx context.Context, tenantID, idempotencyKey string, fact api.OperationFactV1) (api.OperationFactV1, bool, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(idempotencyKey) == "" || tenantID != fact.Request.TenantID || idempotencyKey != fact.Request.IdempotencyKey || fact.Revision != 1 || fact.State != api.OperationQueuedV1 {
		return api.OperationFactV1{}, false, api.ErrConflict
	}
	if err := fact.ValidateShape(); err != nil {
		return api.OperationFactV1{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return api.OperationFactV1{}, false, err
	}
	defer tx.Rollback()

	existing, err := readAPIOperationByIdempotency(ctx, tx, tenantID, idempotencyKey)
	if err == nil {
		if err := tx.Commit(); err != nil {
			return api.OperationFactV1{}, false, err
		}
		return existing, false, nil
	}
	if !errors.Is(err, api.ErrNotFound) {
		return api.OperationFactV1{}, false, err
	}
	if _, err := readAPIOperationCurrent(ctx, tx, fact.ID); err == nil {
		return api.OperationFactV1{}, false, api.ErrConflict
	} else if !errors.Is(err, api.ErrNotFound) {
		return api.OperationFactV1{}, false, err
	}

	body, err := encode(fact)
	if err != nil {
		return api.OperationFactV1{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_api_operation_history(operation_id,revision,digest,body) VALUES(?,?,?,?)`, fact.ID, fact.Revision, fact.Digest, body); err != nil {
		return api.OperationFactV1{}, false, classifyAPIWrite(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_api_operation_current(operation_id,tenant_id,idempotency_key,revision,digest,body) VALUES(?,?,?,?,?,?)`, fact.ID, tenantID, idempotencyKey, fact.Revision, fact.Digest, body); err != nil {
		return api.OperationFactV1{}, false, classifyAPIWrite(err)
	}
	if err := tx.Commit(); err != nil {
		return api.OperationFactV1{}, false, err
	}
	cloned, err := cloneAPIOperation(fact)
	return cloned, true, err
}

func (s *Store) InspectCurrent(ctx context.Context, id string) (api.OperationFactV1, error) {
	return readAPIOperationCurrent(ctx, s.db, id)
}

func (s *Store) InspectByIdempotency(ctx context.Context, tenantID, idempotencyKey string) (api.OperationFactV1, error) {
	return readAPIOperationByIdempotency(ctx, s.db, tenantID, idempotencyKey)
}

func (s *Store) CompareAndSwap(ctx context.Context, expected api.OperationRefV1, next api.OperationFactV1) (api.OperationFactV1, error) {
	if err := expected.Validate(); err != nil {
		return api.OperationFactV1{}, err
	}
	if err := next.ValidateShape(); err != nil {
		return api.OperationFactV1{}, err
	}
	if next.ID != expected.ID || next.Revision != expected.Revision+1 {
		return api.OperationFactV1{}, api.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return api.OperationFactV1{}, err
	}
	defer tx.Rollback()
	current, err := readAPIOperationCurrent(ctx, tx, expected.ID)
	if err != nil {
		return api.OperationFactV1{}, err
	}
	if current.Ref() != expected || next.Request.Digest != current.Request.Digest || next.CreatedUnixNano != current.CreatedUnixNano || next.ExpiresUnixNano != current.ExpiresUnixNano {
		return api.OperationFactV1{}, api.ErrConflict
	}
	body, err := encode(next)
	if err != nil {
		return api.OperationFactV1{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_api_operation_history(operation_id,revision,digest,body) VALUES(?,?,?,?)`, next.ID, next.Revision, next.Digest, body); err != nil {
		return api.OperationFactV1{}, classifyAPIWrite(err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE sandbox_api_operation_current SET revision=?,digest=?,body=? WHERE operation_id=? AND revision=? AND digest=?`, next.Revision, next.Digest, body, expected.ID, expected.Revision, expected.Digest)
	if err != nil {
		return api.OperationFactV1{}, classifyAPIWrite(err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return api.OperationFactV1{}, api.ErrConflict
	}
	if err := tx.Commit(); err != nil {
		return api.OperationFactV1{}, err
	}
	return cloneAPIOperation(next)
}

func (s *Store) ListAfter(ctx context.Context, after uint64, limit int) ([]api.OperationFactV1, uint64, error) {
	if limit <= 0 || limit > 1024 {
		return nil, 0, errors.New("sandbox API watch limit must be within 1..1024")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT cursor,body FROM sandbox_api_operation_history WHERE cursor>? ORDER BY cursor ASC LIMIT ?`, after, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]api.OperationFactV1, 0, limit)
	cursor := after
	for rows.Next() {
		var body []byte
		if err := rows.Scan(&cursor, &body); err != nil {
			return nil, 0, err
		}
		var fact api.OperationFactV1
		if err := decode(body, &fact); err != nil {
			return nil, 0, err
		}
		if err := fact.ValidateShape(); err != nil {
			return nil, 0, fmt.Errorf("stored Sandbox API operation is invalid: %w", err)
		}
		items = append(items, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, cursor, nil
}

func readAPIOperationCurrent(ctx context.Context, db queryer, id string) (api.OperationFactV1, error) {
	var body []byte
	if err := db.QueryRowContext(ctx, `SELECT body FROM sandbox_api_operation_current WHERE operation_id=?`, id).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return api.OperationFactV1{}, api.ErrNotFound
		}
		return api.OperationFactV1{}, err
	}
	return decodeAPIOperation(body)
}

func readAPIOperationByIdempotency(ctx context.Context, db queryer, tenantID, idempotencyKey string) (api.OperationFactV1, error) {
	var body []byte
	if err := db.QueryRowContext(ctx, `SELECT body FROM sandbox_api_operation_current WHERE tenant_id=? AND idempotency_key=?`, tenantID, idempotencyKey).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return api.OperationFactV1{}, api.ErrNotFound
		}
		return api.OperationFactV1{}, err
	}
	return decodeAPIOperation(body)
}

func decodeAPIOperation(body []byte) (api.OperationFactV1, error) {
	var fact api.OperationFactV1
	if err := decode(body, &fact); err != nil {
		return api.OperationFactV1{}, err
	}
	if err := fact.ValidateShape(); err != nil {
		return api.OperationFactV1{}, fmt.Errorf("stored Sandbox API operation is invalid: %w", err)
	}
	return fact, nil
}

func cloneAPIOperation(value api.OperationFactV1) (api.OperationFactV1, error) {
	body, err := encode(value)
	if err != nil {
		return api.OperationFactV1{}, err
	}
	cloned, err := decodeAPIOperation(body)
	return cloned, err
}

func classifyAPIWrite(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return api.ErrConflict
	}
	return err
}
