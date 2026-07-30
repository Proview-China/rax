package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolports "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/ports"
)

const settledApplicationResultSchemaV1 = `
CREATE TABLE IF NOT EXISTS single_call_application_result_v2 (
    result_id TEXT PRIMARY KEY,
    result_revision INTEGER NOT NULL,
    result_digest TEXT NOT NULL,
    request_id TEXT NOT NULL,
    request_revision INTEGER NOT NULL,
    request_digest TEXT NOT NULL,
    action_coordinate_digest TEXT NOT NULL,
    scope_digest TEXT NOT NULL,
    body_json BLOB NOT NULL,
    row_digest TEXT NOT NULL,
    UNIQUE(result_id, result_revision, result_digest),
    UNIQUE(request_id, request_revision, request_digest, action_coordinate_digest, scope_digest)
) STRICT;
`

// OpenApplicationResultStoreV2 installs the durable Application V2 result
// table used by the existing ApplicationResultStoreV2 close seam.
func OpenApplicationResultStoreV2(ctx context.Context, config ConfigV1) (*StoreV1, error) {
	store, err := OpenV1(ctx, config)
	if err != nil {
		return nil, err
	}
	if err = store.initializeSettledApplicationResultSchemaV1(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func (s *StoreV1) initializeSettledApplicationResultSchemaV1(ctx context.Context) error {
	if err := s.writeReadyV1(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, settledApplicationResultSchemaV1); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	if err = tx.Commit(); err != nil {
		return indeterminateV1("Application V2 result schema commit outcome is unknown")
	}
	return nil
}

// CreateSingleCallApplicationResultV2 is the real create-once seam called by
// SingleCallToolActionAdapterV2 after Tool settlement has closed. There is no
// separate whole-fact Ensure or handoff-side write API.
func (s *StoreV1) CreateSingleCallApplicationResultV2(
	ctx context.Context,
	request applicationcontract.SingleCallToolActionRequestV2,
	result applicationcontract.SingleCallToolActionResultV2,
) (applicationcontract.SingleCallToolActionResultV2, error) {
	if err := s.writeReadyV1(ctx); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	record := toolports.ApplicationResultExactRecordV2{Request: request, Result: result}
	key, err := validateSettledApplicationResultRecordV1(record)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	body, err := json.Marshal(record)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, invalidV1("Application V2 result JSON encode failed")
	}
	rowDigest, err := rowDigestV1("SingleCallApplicationResultRecordV2", record)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	ref := result.RefV2()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	insert, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO single_call_application_result_v2
(result_id,result_revision,result_digest,request_id,request_revision,request_digest,action_coordinate_digest,scope_digest,body_json,row_digest)
VALUES(?,?,?,?,?,?,?,?,?,?)`,
		ref.ID, int64(ref.Revision), string(ref.Digest),
		key.RequestID, int64(key.RequestRevision), string(key.RequestDigest),
		string(key.ActionCoordinateDigest), string(key.ScopeDigest),
		body, string(rowDigest),
	)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, mapDBErrorV1(ctx, err, true)
	}
	affected, err := insert.RowsAffected()
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, mapDBErrorV1(ctx, err, true)
	}
	if affected == 0 {
		winner, inspectErr := inspectApplicationResultWinnerV1(ctx, tx, ref.ID, key)
		if inspectErr != nil {
			return applicationcontract.SingleCallToolActionResultV2{}, inspectErr
		}
		if !reflect.DeepEqual(winner, record) {
			return applicationcontract.SingleCallToolActionResultV2{}, conflictV1("Application V2 result key already binds different exact content")
		}
	}
	if err = tx.Commit(); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, indeterminateV1("Application V2 result commit outcome is unknown")
	}
	cloned, err := cloneSettledApplicationResultRecordV1(record)
	return cloned.Result, err
}

func (s *StoreV1) InspectSingleCallApplicationResultRecordV2(
	ctx context.Context,
	key applicationcontract.SingleCallToolActionInspectKeyV2,
) (toolports.ApplicationResultExactRecordV2, error) {
	if err := s.readReadyV1(ctx); err != nil {
		return toolports.ApplicationResultExactRecordV2{}, err
	}
	if err := key.Validate(); err != nil {
		return toolports.ApplicationResultExactRecordV2{}, err
	}
	record, err := inspectApplicationResultByKeyV1(ctx, s.db, key)
	if err != nil {
		return toolports.ApplicationResultExactRecordV2{}, err
	}
	return cloneSettledApplicationResultRecordV1(record)
}

func (s *StoreV1) InspectApplicationResultExactV2(
	ctx context.Context,
	original applicationcontract.SingleCallToolActionResultRefV2,
) (toolports.ApplicationResultExactRecordV2, error) {
	if err := s.readReadyV1(ctx); err != nil {
		return toolports.ApplicationResultExactRecordV2{}, err
	}
	if err := original.Validate(); err != nil {
		return toolports.ApplicationResultExactRecordV2{}, err
	}
	record, err := inspectApplicationResultByRefV1(ctx, s.db, original.ID)
	if err != nil {
		return toolports.ApplicationResultExactRecordV2{}, err
	}
	if record.Result.RefV2() != original {
		return toolports.ApplicationResultExactRecordV2{}, conflictV1("stored Application V2 exact result ref drifted")
	}
	return cloneSettledApplicationResultRecordV1(record)
}

type applicationResultQueryerV1 interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func inspectApplicationResultByKeyV1(
	ctx context.Context,
	queryer applicationResultQueryerV1,
	key applicationcontract.SingleCallToolActionInspectKeyV2,
) (toolports.ApplicationResultExactRecordV2, error) {
	row := queryer.QueryRowContext(ctx, `
SELECT result_id,result_revision,result_digest,request_id,request_revision,request_digest,action_coordinate_digest,scope_digest,body_json,row_digest
FROM single_call_application_result_v2
WHERE request_id=? AND request_revision=? AND request_digest=? AND action_coordinate_digest=? AND scope_digest=?`,
		key.RequestID, int64(key.RequestRevision), string(key.RequestDigest),
		string(key.ActionCoordinateDigest), string(key.ScopeDigest),
	)
	return decodeApplicationResultRowV1(ctx, row)
}

func inspectApplicationResultWinnerV1(
	ctx context.Context,
	queryer applicationResultQueryerV1,
	resultID string,
	key applicationcontract.SingleCallToolActionInspectKeyV2,
) (toolports.ApplicationResultExactRecordV2, error) {
	row := queryer.QueryRowContext(ctx, `
SELECT result_id,result_revision,result_digest,request_id,request_revision,request_digest,action_coordinate_digest,scope_digest,body_json,row_digest
FROM single_call_application_result_v2
WHERE result_id=? OR
      (request_id=? AND request_revision=? AND request_digest=? AND action_coordinate_digest=? AND scope_digest=?)
ORDER BY CASE WHEN result_id=? THEN 0 ELSE 1 END
LIMIT 1`,
		resultID,
		key.RequestID, int64(key.RequestRevision), string(key.RequestDigest),
		string(key.ActionCoordinateDigest), string(key.ScopeDigest),
		resultID,
	)
	return decodeApplicationResultRowV1(ctx, row)
}

func inspectApplicationResultByRefV1(
	ctx context.Context,
	queryer applicationResultQueryerV1,
	resultID string,
) (toolports.ApplicationResultExactRecordV2, error) {
	row := queryer.QueryRowContext(ctx, `
SELECT result_id,result_revision,result_digest,request_id,request_revision,request_digest,action_coordinate_digest,scope_digest,body_json,row_digest
FROM single_call_application_result_v2 WHERE result_id=?`, resultID)
	return decodeApplicationResultRowV1(ctx, row)
}

func decodeApplicationResultRowV1(
	ctx context.Context,
	row scanRowV1,
) (toolports.ApplicationResultExactRecordV2, error) {
	var resultID, resultDigest, requestID, requestDigest, actionDigest, scopeDigest, storedRowDigest string
	var resultRevision, requestRevision int64
	var body []byte
	if err := row.Scan(
		&resultID, &resultRevision, &resultDigest,
		&requestID, &requestRevision, &requestDigest,
		&actionDigest, &scopeDigest, &body, &storedRowDigest,
	); err != nil {
		return toolports.ApplicationResultExactRecordV2{}, mapDBErrorV1(ctx, err, false)
	}
	var record toolports.ApplicationResultExactRecordV2
	if core.DecodeStrictJSON(body, &record) != nil || !canonicalJSONBytesV1(body, record) {
		return toolports.ApplicationResultExactRecordV2{}, conflictV1("stored Application V2 result row is non-canonical")
	}
	key, err := validateSettledApplicationResultRecordV1(record)
	if err != nil {
		return toolports.ApplicationResultExactRecordV2{}, conflictV1("stored Application V2 result record is invalid")
	}
	ref := record.Result.RefV2()
	rowDigest, err := rowDigestV1("SingleCallApplicationResultRecordV2", record)
	if err != nil || string(rowDigest) != storedRowDigest ||
		resultID != ref.ID || resultRevision != int64(ref.Revision) || resultDigest != string(ref.Digest) ||
		requestID != key.RequestID || requestRevision != int64(key.RequestRevision) ||
		requestDigest != string(key.RequestDigest) || actionDigest != string(key.ActionCoordinateDigest) ||
		scopeDigest != string(key.ScopeDigest) {
		return toolports.ApplicationResultExactRecordV2{}, conflictV1("stored Application V2 result row digest drifted")
	}
	return record, nil
}

func validateSettledApplicationResultRecordV1(
	record toolports.ApplicationResultExactRecordV2,
) (applicationcontract.SingleCallToolActionInspectKeyV2, error) {
	if err := record.Request.Validate(); err != nil {
		return applicationcontract.SingleCallToolActionInspectKeyV2{}, err
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(record.Request)
	if err != nil {
		return applicationcontract.SingleCallToolActionInspectKeyV2{}, err
	}
	if err = record.ValidateForKey(key); err != nil {
		return applicationcontract.SingleCallToolActionInspectKeyV2{}, err
	}
	return key, nil
}

func cloneSettledApplicationResultRecordV1(
	record toolports.ApplicationResultExactRecordV2,
) (toolports.ApplicationResultExactRecordV2, error) {
	body, err := json.Marshal(record)
	if err != nil {
		return toolports.ApplicationResultExactRecordV2{}, invalidV1("Application V2 result clone encode failed")
	}
	var cloned toolports.ApplicationResultExactRecordV2
	if err = core.DecodeStrictJSON(body, &cloned); err != nil {
		return toolports.ApplicationResultExactRecordV2{}, invalidV1("Application V2 result clone decode failed")
	}
	return cloned, nil
}

var _ toolports.ApplicationResultExactReaderV2 = (*StoreV1)(nil)
