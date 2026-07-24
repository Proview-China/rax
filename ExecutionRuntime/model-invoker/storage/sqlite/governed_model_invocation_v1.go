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

type currentRowV1 struct {
	revision core.Revision
	digest   core.Digest
	highest  core.Revision
}

type historyRowV1 struct {
	fact          modelinvoker.GovernedModelInvocationFactV1
	factDigest    core.Digest
	attemptDigest core.Digest
	rowDigest     core.Digest
	wire          json.RawMessage
}

func (s *Store) CreateGovernedModelInvocationV1(ctx context.Context, fact modelinvoker.GovernedModelInvocationFactV1) (modelinvoker.GovernedModelInvocationMutationV1, error) {
	if err := contextErrorV1(ctx, "create"); err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, err
	}
	if err := fact.Validate(); err != nil || fact.Revision != 1 || fact.State != modelinvoker.GovernedModelInvocationPreparedV1 {
		return modelinvoker.GovernedModelInvocationMutationV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "create", "create requires sealed prepared revision one", err)
	}
	wire, rowDigest, attemptDigest, err := encodeFactV1(fact)
	if err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, err
	}
	tx, err := s.beginV1(ctx, "create")
	if err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var guardedID string
	err = tx.QueryRowContext(ctx, `SELECT invocation_id FROM governed_model_invocation_attempt_guard WHERE attempt_digest=?`, string(attemptDigest)).Scan(&guardedID)
	guardExists := err == nil
	if err == nil && guardedID != fact.ID {
		return modelinvoker.GovernedModelInvocationMutationV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "create", "logical provider attempt contains different canonical content", nil)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return modelinvoker.GovernedModelInvocationMutationV1{}, mapDBErrorV1(ctx, "create", err, false)
	}
	current, currentErr := loadCurrentRowV1(ctx, tx, fact.ID)
	if currentErr == nil {
		if !guardExists || guardedID != fact.ID {
			return modelinvoker.GovernedModelInvocationMutationV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "create", "invocation current index lost its logical attempt guard", nil)
		}
		first, loadErr := loadHistoryV1(ctx, tx, fact.ID, 1)
		if loadErr != nil {
			return modelinvoker.GovernedModelInvocationMutationV1{}, loadErr
		}
		if first.fact.RefV1() != fact.RefV1() || !bytes.Equal(first.wire, wire) || first.attemptDigest != attemptDigest {
			return modelinvoker.GovernedModelInvocationMutationV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "create", "invocation ID contains different canonical content", nil)
		}
		stored, loadErr := loadHistoryV1(ctx, tx, fact.ID, current.revision)
		if loadErr != nil {
			return modelinvoker.GovernedModelInvocationMutationV1{}, loadErr
		}
		return modelinvoker.GovernedModelInvocationMutationV1{Fact: stored.fact.CloneV1(), Applied: false}, nil
	}
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(currentErr) != modelinvoker.GovernedModelInvocationErrorNotFound {
		return modelinvoker.GovernedModelInvocationMutationV1{}, currentErr
	}
	if guardExists {
		return modelinvoker.GovernedModelInvocationMutationV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "create", "attempt guard has no current invocation", nil)
	}
	if err := insertHistoryV1(ctx, tx, fact, attemptDigest, rowDigest, wire); err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO governed_model_invocation_current(invocation_id,revision,fact_digest,highest_revision) VALUES(?,?,?,?)`, fact.ID, fact.Revision, string(fact.Digest), fact.Revision); err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, mapDBErrorV1(ctx, "create", err, true)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO governed_model_invocation_attempt_guard(attempt_digest,invocation_id) VALUES(?,?)`, string(attemptDigest), fact.ID); err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, mapDBErrorV1(ctx, "create", err, true)
	}
	if err := tx.Commit(); err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "create", "sqlite commit outcome is unknown", err)
	}
	return modelinvoker.GovernedModelInvocationMutationV1{Fact: fact.CloneV1(), Applied: true}, nil
}

func (s *Store) CompareAndSwapGovernedModelInvocationV1(ctx context.Context, request modelinvoker.GovernedModelInvocationCASV1) (modelinvoker.GovernedModelInvocationMutationV1, error) {
	if err := contextErrorV1(ctx, "cas"); err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, err
	}
	if err := request.Validate(); err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, err
	}
	wire, rowDigest, attemptDigest, err := encodeFactV1(request.Next)
	if err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, err
	}
	tx, err := s.beginV1(ctx, "cas")
	if err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := loadCurrentRowV1(ctx, tx, request.Next.ID)
	if err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, err
	}
	stored, err := loadHistoryV1(ctx, tx, request.Next.ID, current.revision)
	if err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, err
	}
	if err := requireAttemptGuardV1(ctx, tx, stored.attemptDigest, stored.fact.ID); err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, err
	}
	if stored.fact.RefV1() == request.Next.RefV1() {
		if !bytes.Equal(stored.wire, wire) {
			return modelinvoker.GovernedModelInvocationMutationV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "cas", "same Ref contains different canonical content", nil)
		}
		return modelinvoker.GovernedModelInvocationMutationV1{Fact: stored.fact.CloneV1(), Applied: false}, nil
	}
	if stored.fact.RefV1() != request.Expected || current.digest != request.Expected.Digest || current.highest != request.Expected.Revision {
		return modelinvoker.GovernedModelInvocationMutationV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "cas", "current exact Ref differs from CAS precondition", nil)
	}
	if err := modelinvoker.ValidateGovernedModelInvocationTransitionV1(stored.fact, request.Next); err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "cas", "transition is invalid", err)
	}
	if attemptDigest != stored.attemptDigest {
		return modelinvoker.GovernedModelInvocationMutationV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "cas", "attempt coordinate drifted", nil)
	}
	if err := insertHistoryV1(ctx, tx, request.Next, attemptDigest, rowDigest, wire); err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE governed_model_invocation_current SET revision=?,fact_digest=?,highest_revision=? WHERE invocation_id=? AND revision=? AND fact_digest=? AND highest_revision=?`, request.Next.Revision, string(request.Next.Digest), request.Next.Revision, request.Next.ID, current.revision, string(current.digest), current.highest)
	if err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, mapDBErrorV1(ctx, "cas", err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, mapDBErrorV1(ctx, "cas", err, true)
	}
	if affected != 1 {
		return modelinvoker.GovernedModelInvocationMutationV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "cas", "current index CAS lost its exact precondition", nil)
	}
	if err := tx.Commit(); err != nil {
		return modelinvoker.GovernedModelInvocationMutationV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "cas", "sqlite commit outcome is unknown", err)
	}
	return modelinvoker.GovernedModelInvocationMutationV1{Fact: request.Next.CloneV1(), Applied: true}, nil
}

func (s *Store) InspectExactGovernedModelInvocationV1(ctx context.Context, ref modelinvoker.GovernedModelInvocationRefV1) (modelinvoker.GovernedModelInvocationFactV1, error) {
	if err := contextErrorV1(ctx, "inspect_exact"); err != nil {
		return modelinvoker.GovernedModelInvocationFactV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return modelinvoker.GovernedModelInvocationFactV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "inspect_exact", "exact Ref is invalid", err)
	}
	if s == nil || s.db == nil {
		return modelinvoker.GovernedModelInvocationFactV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorUnavailable, "inspect_exact", "sqlite repository is unavailable", nil)
	}
	stored, err := loadHistoryV1(ctx, s.db, ref.ID, ref.Revision)
	if err != nil {
		return modelinvoker.GovernedModelInvocationFactV1{}, err
	}
	if stored.fact.RefV1() != ref {
		return modelinvoker.GovernedModelInvocationFactV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_exact", "exact Ref differs from immutable history", nil)
	}
	return stored.fact.CloneV1(), nil
}

func (s *Store) InspectCurrentGovernedModelInvocationV1(ctx context.Context, id string) (modelinvoker.GovernedModelInvocationFactV1, error) {
	if err := contextErrorV1(ctx, "inspect_current"); err != nil {
		return modelinvoker.GovernedModelInvocationFactV1{}, err
	}
	if s == nil || s.db == nil {
		return modelinvoker.GovernedModelInvocationFactV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorUnavailable, "inspect_current", "sqlite repository is unavailable", nil)
	}
	if id == "" {
		return modelinvoker.GovernedModelInvocationFactV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "inspect_current", "invocation ID is required", nil)
	}
	current, err := loadCurrentRowV1(ctx, s.db, id)
	if err != nil {
		return modelinvoker.GovernedModelInvocationFactV1{}, err
	}
	stored, err := loadHistoryV1(ctx, s.db, id, current.revision)
	if err != nil {
		return modelinvoker.GovernedModelInvocationFactV1{}, err
	}
	if err := requireAttemptGuardV1(ctx, s.db, stored.attemptDigest, stored.fact.ID); err != nil {
		return modelinvoker.GovernedModelInvocationFactV1{}, err
	}
	if stored.fact.Digest != current.digest {
		return modelinvoker.GovernedModelInvocationFactV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_current", "current index digest drifted", nil)
	}
	return stored.fact.CloneV1(), nil
}

func loadCurrentRowV1(ctx context.Context, source interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (currentRowV1, error) {
	var revision, highest, historyHighest uint64
	var digest string
	err := source.QueryRowContext(ctx, `SELECT c.revision,c.fact_digest,c.highest_revision,(SELECT COALESCE(MAX(h.revision),0) FROM governed_model_invocation_history h WHERE h.invocation_id=c.invocation_id) FROM governed_model_invocation_current c WHERE c.invocation_id=?`, id).Scan(&revision, &digest, &highest, &historyHighest)
	if errors.Is(err, sql.ErrNoRows) {
		return currentRowV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorNotFound, "inspect_current", "invocation current index is absent", nil)
	}
	if err != nil {
		return currentRowV1{}, mapDBErrorV1(ctx, "inspect_current", err, false)
	}
	if revision == 0 || highest == 0 || historyHighest == 0 || revision != highest || highest != historyHighest || digest == "" {
		return currentRowV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_current", "current/highest/history index drifted", nil)
	}
	return currentRowV1{revision: core.Revision(revision), digest: core.Digest(digest), highest: core.Revision(highest)}, nil
}

func loadHistoryV1(ctx context.Context, source interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string, revision core.Revision) (historyRowV1, error) {
	var factDigest, attemptDigest, rowDigest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT fact_digest,attempt_digest,row_digest,canonical_json FROM governed_model_invocation_history WHERE invocation_id=? AND revision=?`, id, revision).Scan(&factDigest, &attemptDigest, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return historyRowV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorNotFound, "inspect_exact", "invocation history is absent", nil)
	}
	if err != nil {
		return historyRowV1{}, mapDBErrorV1(ctx, "inspect_exact", err, false)
	}
	var fact modelinvoker.GovernedModelInvocationFactV1
	if err := core.DecodeStrictJSON(payload, &fact); err != nil {
		return historyRowV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_exact", "stored Fact failed strict decoding", err)
	}
	computedRow, computedAttempt, err := rowDigestsV1(fact)
	if err != nil || fact.Validate() != nil || fact.ID != id || fact.Revision != revision || fact.Digest != core.Digest(factDigest) || computedRow != core.Digest(rowDigest) || computedAttempt != core.Digest(attemptDigest) {
		return historyRowV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_exact", "stored Fact failed exact revalidation", err)
	}
	return historyRowV1{fact: fact.CloneV1(), factDigest: fact.Digest, attemptDigest: computedAttempt, rowDigest: computedRow, wire: append(json.RawMessage(nil), payload...)}, nil
}

func insertHistoryV1(ctx context.Context, tx *sql.Tx, fact modelinvoker.GovernedModelInvocationFactV1, attemptDigest, rowDigest core.Digest, wire json.RawMessage) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO governed_model_invocation_history(invocation_id,revision,fact_digest,attempt_digest,row_digest,canonical_json) VALUES(?,?,?,?,?,?)`, fact.ID, fact.Revision, string(fact.Digest), string(attemptDigest), string(rowDigest), wire)
	return mapDBErrorV1(ctx, "write_history", err, true)
}

func requireAttemptGuardV1(ctx context.Context, source interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, attemptDigest core.Digest, invocationID string) error {
	var storedID string
	err := source.QueryRowContext(ctx, `SELECT invocation_id FROM governed_model_invocation_attempt_guard WHERE attempt_digest=?`, string(attemptDigest)).Scan(&storedID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && storedID != invocationID) {
		return errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "attempt_guard", "logical attempt guard drifted", nil)
	}
	if err != nil {
		return mapDBErrorV1(ctx, "attempt_guard", err, false)
	}
	return nil
}

func encodeFactV1(fact modelinvoker.GovernedModelInvocationFactV1) (json.RawMessage, core.Digest, core.Digest, error) {
	if err := fact.Validate(); err != nil {
		return nil, "", "", err
	}
	payload, err := json.Marshal(fact)
	if err != nil {
		return nil, "", "", errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "encode", "Fact is not JSON serializable", err)
	}
	var exact modelinvoker.GovernedModelInvocationFactV1
	if err := core.DecodeStrictJSON(payload, &exact); err != nil || exact.RefV1() != fact.RefV1() {
		return nil, "", "", errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "encode", "Fact failed strict round trip", err)
	}
	rowDigest, attemptDigest, err := rowDigestsV1(exact)
	return payload, rowDigest, attemptDigest, err
}

func rowDigestsV1(fact modelinvoker.GovernedModelInvocationFactV1) (core.Digest, core.Digest, error) {
	rowDigest, err := core.CanonicalJSONDigest("praxis.model-invoker.sqlite", "v1", "GovernedModelInvocationRowV1", fact)
	if err != nil {
		return "", "", err
	}
	attemptDigest, err := core.CanonicalJSONDigest("praxis.model-invoker.sqlite", "v1", "GovernedModelInvocationAttemptV1", struct {
		PreparedRef          modelinvoker.PreparedModelInvocationRefV1 `json:"prepared_ref"`
		DispatchSequence     uint64                                    `json:"dispatch_sequence"`
		ProviderAttemptOrder uint32                                    `json:"provider_attempt_ordinal"`
	}{fact.PreparedRef, fact.DispatchSequence, fact.ProviderAttemptOrdinal})
	if err != nil {
		return "", "", err
	}
	return rowDigest, attemptDigest, nil
}

var _ modelinvoker.GovernedModelInvocationRepositoryV1 = (*Store)(nil)
