package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func preparedStoreError(kind modelinvoker.PreparedModelInvocationRepositoryErrorKindV1, op, message string, err error) error {
	return &modelinvoker.PreparedModelInvocationRepositoryErrorV1{Kind: kind, Operation: op, Message: message, Err: err}
}

func (s *Store) EnsurePreparedModelInvocationV1(ctx context.Context, fact modelinvoker.PreparedModelInvocationFactV1) (modelinvoker.PreparedModelInvocationFactV1, error) {
	if ctx == nil || ctx.Err() != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate, "ensure_historical", "context ended before linearization", context.Cause(ctx))
	}
	if err := fact.Validate(); err != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorInvalid, "ensure_historical", "fact is invalid", err)
	}
	wire, err := json.Marshal(fact)
	if err != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorInvalid, "ensure_historical", "fact cannot be encoded", err)
	}
	tx, err := s.beginV1(ctx, "ensure_prepared_historical")
	if err != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var digest string
	var stored []byte
	err = tx.QueryRowContext(ctx, `SELECT fact_digest,canonical_json FROM prepared_model_invocation_history WHERE prepared_id=? AND revision=?`, fact.ID, fact.Revision).Scan(&digest, &stored)
	if err == nil {
		if digest != string(fact.Digest) || !bytes.Equal(stored, wire) {
			return modelinvoker.PreparedModelInvocationFactV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorConflict, "ensure_historical", "exact history contains different canonical content", nil)
		}
		return fact.Clone(), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return modelinvoker.PreparedModelInvocationFactV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorUnavailable, "ensure_historical", "history read failed", err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO prepared_model_invocation_history(prepared_id,revision,fact_digest,canonical_json) VALUES(?,?,?,?)`, fact.ID, fact.Revision, string(fact.Digest), wire); err != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate, "ensure_historical", "history write outcome is unknown", err)
	}
	if err = tx.Commit(); err != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate, "ensure_historical", "history commit outcome is unknown", err)
	}
	return fact.Clone(), nil
}

func (s *Store) InspectExactPreparedModelInvocationV1(ctx context.Context, ref modelinvoker.PreparedModelInvocationRefV1) (modelinvoker.PreparedModelInvocationFactV1, error) {
	if ctx == nil || ctx.Err() != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate, "inspect_historical", "context ended", context.Cause(ctx))
	}
	if err := ref.Validate(); err != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorInvalid, "inspect_historical", "ref is invalid", err)
	}
	var digest string
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT fact_digest,canonical_json FROM prepared_model_invocation_history WHERE prepared_id=? AND revision=?`, ref.ID, ref.Revision).Scan(&digest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return modelinvoker.PreparedModelInvocationFactV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorAuthoritativeAbsent, "inspect_historical", "exact history is absent", nil)
	}
	if err != nil {
		return modelinvoker.PreparedModelInvocationFactV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorRetentionUnreadable, "inspect_historical", "history is unreadable", err)
	}
	var fact modelinvoker.PreparedModelInvocationFactV1
	if err := core.DecodeStrictJSON(payload, &fact); err != nil || fact.Validate() != nil || fact.Ref() != ref || digest != string(fact.Digest) {
		return modelinvoker.PreparedModelInvocationFactV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorConflict, "inspect_historical", "stored history failed exact revalidation", err)
	}
	return fact.Clone(), nil
}

func (s *Store) EnsurePreparedModelInvocationCurrentV1(ctx context.Context, current modelinvoker.PreparedModelInvocationCurrentProjectionV1) (modelinvoker.PreparedModelInvocationCurrentProjectionV1, error) {
	if ctx == nil || ctx.Err() != nil {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate, "ensure_current", "context ended", context.Cause(ctx))
	}
	if err := current.Validate(); err != nil {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorInvalid, "ensure_current", "current is invalid", err)
	}
	wire, err := json.Marshal(current)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorInvalid, "ensure_current", "current cannot be encoded", err)
	}
	tx, err := s.beginV1(ctx, "ensure_prepared_current")
	if err != nil {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var hd string
	if err := tx.QueryRowContext(ctx, `SELECT fact_digest FROM prepared_model_invocation_history WHERE prepared_id=? AND revision=?`, current.Prepared.ID, current.Prepared.Revision).Scan(&hd); err != nil || hd != string(current.Prepared.Digest) {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorConflict, "ensure_current", "prepared history is absent or drifted", err)
	}
	var digest string
	var stored []byte
	err = tx.QueryRowContext(ctx, `SELECT current_digest,canonical_json FROM prepared_model_invocation_current WHERE current_id=?`, current.ID).Scan(&digest, &stored)
	if err == nil {
		if digest != string(current.Digest) || !bytes.Equal(stored, wire) {
			return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorConflict, "ensure_current", "current contains different canonical content", nil)
		}
		return current.Clone(), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorUnavailable, "ensure_current", "current read failed", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO prepared_model_invocation_current(current_id,revision,current_digest,prepared_id,prepared_revision,prepared_digest,canonical_json) VALUES(?,?,?,?,?,?,?)`, current.ID, current.Revision, string(current.Digest), current.Prepared.ID, current.Prepared.Revision, string(current.Prepared.Digest), wire)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate, "ensure_current", "current write outcome is unknown", err)
	}
	if err = tx.Commit(); err != nil {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate, "ensure_current", "current commit outcome is unknown", err)
	}
	return current.Clone(), nil
}

func (s *Store) InspectExactPreparedModelInvocationCurrentV1(ctx context.Context, ref modelinvoker.PreparedModelInvocationCurrentRefV1) (modelinvoker.PreparedModelInvocationCurrentProjectionV1, error) {
	if ctx == nil || ctx.Err() != nil {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorIndeterminate, "inspect_current", "context ended", context.Cause(ctx))
	}
	if err := ref.Validate(); err != nil {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorInvalid, "inspect_current", "ref is invalid", err)
	}
	var digest, preparedID, preparedDigest string
	var revision, preparedRevision uint64
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT revision,current_digest,prepared_id,prepared_revision,prepared_digest,canonical_json FROM prepared_model_invocation_current WHERE current_id=?`, ref.ID).Scan(&revision, &digest, &preparedID, &preparedRevision, &preparedDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorAuthoritativeAbsent, "inspect_current", "exact current is absent", nil)
	}
	if err != nil {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorRetentionUnreadable, "inspect_current", "current is unreadable", err)
	}
	var current modelinvoker.PreparedModelInvocationCurrentProjectionV1
	if err := core.DecodeStrictJSON(payload, &current); err != nil || current.Validate() != nil || current.Ref() != ref || revision != uint64(ref.Revision) || digest != string(ref.Digest) || preparedID != ref.Prepared.ID || preparedRevision != uint64(ref.Prepared.Revision) || preparedDigest != string(ref.Prepared.Digest) {
		return modelinvoker.PreparedModelInvocationCurrentProjectionV1{}, preparedStoreError(modelinvoker.PreparedModelInvocationRepositoryErrorConflict, "inspect_current", "stored current failed exact revalidation", err)
	}
	return current.Clone(), nil
}

var _ modelinvoker.PreparedModelInvocationRepositoryV1 = (*Store)(nil)
var _ modelinvoker.PreparedModelInvocationReaderV1 = (*Store)(nil)
var _ modelinvoker.PreparedModelInvocationCurrentRepositoryV1 = (*Store)(nil)
