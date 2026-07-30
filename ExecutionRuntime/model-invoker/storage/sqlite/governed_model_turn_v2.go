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

type turnCurrentV2 struct {
	revision, highest core.Revision
	digest            core.Digest
}
type turnStoredV2 struct {
	outcome modelinvoker.GovernedModelTurnOutcomeV2
	wire    json.RawMessage
	attempt core.Digest
}

func (s *Store) CreateGovernedModelTurnV2(ctx context.Context, outcome modelinvoker.GovernedModelTurnOutcomeV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	if err := contextErrorV1(ctx, "create_turn_v2"); err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, err
	}
	if err := outcome.Validate(); err != nil || outcome.State != modelinvoker.GovernedModelTurnPreparedV2 || outcome.Revision != 1 {
		return modelinvoker.GovernedModelTurnMutationV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "create_turn_v2", "create requires prepared revision one", err)
	}
	wire, attempt, err := encodeTurnV2(outcome)
	if err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, err
	}
	tx, err := s.beginV1(ctx, "create_turn_v2")
	if err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var guarded string
	guardErr := tx.QueryRowContext(ctx, `SELECT turn_id FROM governed_model_turn_attempt_guard WHERE attempt_digest=?`, string(attempt)).Scan(&guarded)
	guardExists := guardErr == nil
	if guardErr != nil && !errors.Is(guardErr, sql.ErrNoRows) {
		return modelinvoker.GovernedModelTurnMutationV2{}, mapDBErrorV1(ctx, "create_turn_v2", guardErr, false)
	}
	if guardExists && guarded != outcome.ID {
		return modelinvoker.GovernedModelTurnMutationV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "create_turn_v2", "logical attempt contains different material", nil)
	}
	current, currentErr := loadTurnCurrentV2(ctx, tx, outcome.ID)
	if currentErr == nil {
		if !guardExists {
			return modelinvoker.GovernedModelTurnMutationV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "create_turn_v2", "turn lost attempt guard", nil)
		}
		first, err := loadTurnHistoryV2(ctx, tx, outcome.ID, 1)
		if err != nil {
			return modelinvoker.GovernedModelTurnMutationV2{}, err
		}
		if first.outcome.RefV2() != outcome.RefV2() || !bytes.Equal(first.wire, wire) {
			return modelinvoker.GovernedModelTurnMutationV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "create_turn_v2", "turn contains different canonical content", nil)
		}
		stored, err := loadTurnHistoryV2(ctx, tx, outcome.ID, current.revision)
		return modelinvoker.GovernedModelTurnMutationV2{Outcome: stored.outcome.CloneV2(), Applied: false}, err
	}
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(currentErr) != modelinvoker.GovernedModelInvocationErrorNotFound {
		return modelinvoker.GovernedModelTurnMutationV2{}, currentErr
	}
	if guardExists {
		return modelinvoker.GovernedModelTurnMutationV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "create_turn_v2", "attempt guard has no turn", nil)
	}
	if err := insertTurnHistoryV2(ctx, tx, outcome, attempt, wire); err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO governed_model_turn_current(turn_id,revision,fact_digest,highest_revision) VALUES(?,?,?,?)`, outcome.ID, outcome.Revision, string(outcome.Digest), outcome.Revision); err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, mapDBErrorV1(ctx, "create_turn_v2", err, true)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO governed_model_turn_attempt_guard(attempt_digest,turn_id) VALUES(?,?)`, string(attempt), outcome.ID); err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, mapDBErrorV1(ctx, "create_turn_v2", err, true)
	}
	if err = tx.Commit(); err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "create_turn_v2", "commit outcome is unknown", err)
	}
	return modelinvoker.GovernedModelTurnMutationV2{Outcome: outcome.CloneV2(), Applied: true}, nil
}
func (s *Store) CompareAndSwapGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return s.casTurnV2(ctx, request, false)
}
func (s *Store) CompareAndSwapObservedGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	if request.Next.State != modelinvoker.GovernedModelTurnObservedV2 || request.Next.Observation == nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "cas_observed_turn_v2", "observed transition and projection are required", nil)
	}
	return s.casTurnV2(ctx, request, true)
}
func (s *Store) casTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2, withProjection bool) (modelinvoker.GovernedModelTurnMutationV2, error) {
	if err := contextErrorV1(ctx, "cas_turn_v2"); err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, err
	}
	if err := request.Validate(); err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, err
	}
	if request.Next.State == modelinvoker.GovernedModelTurnObservedV2 && request.Next.Observation != nil && request.Next.Observation.ToolCallProjection != nil && !withProjection {
		return modelinvoker.GovernedModelTurnMutationV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "cas_turn_v2", "tool-call observation requires atomic projection CAS", nil)
	}
	wire, attempt, err := encodeTurnV2(request.Next)
	if err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, err
	}
	tx, err := s.beginV1(ctx, "cas_turn_v2")
	if err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := loadTurnCurrentV2(ctx, tx, request.Next.ID)
	if err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, err
	}
	stored, err := loadTurnHistoryV2(ctx, tx, request.Next.ID, current.revision)
	if err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, err
	}
	if stored.attempt != attempt {
		return modelinvoker.GovernedModelTurnMutationV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "cas_turn_v2", "attempt lineage drifted", nil)
	}
	if stored.outcome.RefV2() == request.Next.RefV2() {
		if !bytes.Equal(stored.wire, wire) {
			return modelinvoker.GovernedModelTurnMutationV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "cas_turn_v2", "same Ref has different content", nil)
		}
		return modelinvoker.GovernedModelTurnMutationV2{Outcome: stored.outcome.CloneV2(), Applied: false}, nil
	}
	if stored.outcome.RefV2() != request.Expected || current.digest != request.Expected.Digest || current.highest != request.Expected.Revision {
		return modelinvoker.GovernedModelTurnMutationV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "cas_turn_v2", "current exact Ref changed", nil)
	}
	if err := modelinvoker.ValidateGovernedModelTurnTransitionV2(stored.outcome, request.Next); err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "cas_turn_v2", "transition is invalid", err)
	}
	if err := insertTurnHistoryV2(ctx, tx, request.Next, attempt, wire); err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE governed_model_turn_current SET revision=?,fact_digest=?,highest_revision=? WHERE turn_id=? AND revision=? AND fact_digest=? AND highest_revision=?`, request.Next.Revision, string(request.Next.Digest), request.Next.Revision, request.Next.ID, current.revision, string(current.digest), current.highest)
	if err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, mapDBErrorV1(ctx, "cas_turn_v2", err, true)
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return modelinvoker.GovernedModelTurnMutationV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "cas_turn_v2", "CAS lost exact precondition", nil)
	}
	if withProjection {
		if request.Next.Observation.ToolCallProjection == nil {
			return modelinvoker.GovernedModelTurnMutationV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "cas_observed_turn_v2", "tool-call projection is required", nil)
		}
		projection := *request.Next.Observation.ToolCallProjection
		payload, err := json.Marshal(projection)
		if err != nil {
			return modelinvoker.GovernedModelTurnMutationV2{}, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO governed_model_turn_tool_call_projection(turn_id,turn_revision,projection_id,projection_revision,projection_digest,observation_digest,canonical_json) VALUES(?,?,?,?,?,?,?)`, request.Next.ID, request.Next.Revision, projection.Ref.ID, projection.Ref.Revision, string(projection.Ref.Digest), string(projection.Observation.Digest), payload)
		if err != nil {
			return modelinvoker.GovernedModelTurnMutationV2{}, mapDBErrorV1(ctx, "cas_observed_turn_v2", err, true)
		}
	}
	if err = tx.Commit(); err != nil {
		return modelinvoker.GovernedModelTurnMutationV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "cas_turn_v2", "commit outcome is unknown", err)
	}
	return modelinvoker.GovernedModelTurnMutationV2{Outcome: request.Next.CloneV2(), Applied: true}, nil
}
func (s *Store) InspectExactGovernedModelTurnV2(ctx context.Context, ref modelinvoker.GovernedModelTurnRefV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	if err := contextErrorV1(ctx, "inspect_turn_v2"); err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "inspect_turn_v2", "exact Ref invalid", err)
	}
	stored, err := loadTurnHistoryV2(ctx, s.db, ref.ID, ref.Revision)
	if err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV2{}, err
	}
	if stored.outcome.RefV2() != ref {
		return modelinvoker.GovernedModelTurnOutcomeV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_turn_v2", "exact Ref drifted", nil)
	}
	return stored.outcome.CloneV2(), nil
}
func (s *Store) InspectCurrentGovernedModelTurnV2(ctx context.Context, id string) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	if err := contextErrorV1(ctx, "inspect_current_turn_v2"); err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV2{}, err
	}
	current, err := loadTurnCurrentV2(ctx, s.db, id)
	if err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV2{}, err
	}
	stored, err := loadTurnHistoryV2(ctx, s.db, id, current.revision)
	if err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV2{}, err
	}
	if stored.outcome.Digest != current.digest {
		return modelinvoker.GovernedModelTurnOutcomeV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_current_turn_v2", "current digest drifted", nil)
	}
	return stored.outcome.CloneV2(), nil
}
func (s *Store) InspectExactGovernedModelTurnToolCallProjectionV2(ctx context.Context, ref modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	if err := contextErrorV1(ctx, "inspect_turn_projection_v2"); err != nil {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "inspect_turn_projection_v2", "projection Ref invalid", err)
	}
	var turnID, digest, observation string
	var turnRevision uint64
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT turn_id,turn_revision,projection_digest,observation_digest,canonical_json FROM governed_model_turn_tool_call_projection WHERE projection_id=? AND projection_revision=?`, ref.ID, ref.Revision).Scan(&turnID, &turnRevision, &digest, &observation, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorNotFound, "inspect_turn_projection_v2", "projection absent", nil)
	}
	if err != nil {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, mapDBErrorV1(ctx, "inspect_turn_projection_v2", err, false)
	}
	projection, err := modelinvoker.DecodeToolCallCandidateObservationProjectionV1(payload)
	if err != nil || projection.Ref != ref || digest != string(ref.Digest) || observation != string(ref.ObservationDigest) || turnID == "" || turnRevision == 0 {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_turn_projection_v2", "projection failed exact revalidation", err)
	}
	turn, turnErr := loadTurnHistoryV2(ctx, s.db, turnID, core.Revision(turnRevision))
	if turnErr != nil || turn.outcome.State != modelinvoker.GovernedModelTurnObservedV2 || turn.outcome.Observation == nil || turn.outcome.Observation.ToolCallProjection == nil || turn.outcome.Observation.TurnRef.ID != turnID || turn.outcome.Revision != core.Revision(turnRevision) || turn.outcome.Observation.ToolCallProjection.Ref != ref {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_turn_projection_v2", "projection is not exactly linked to turn history", turnErr)
	}
	return projection.Clone(), nil
}
func loadTurnCurrentV2(ctx context.Context, source interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (turnCurrentV2, error) {
	var revision, highest, history uint64
	var digest string
	err := source.QueryRowContext(ctx, `SELECT c.revision,c.fact_digest,c.highest_revision,(SELECT COALESCE(MAX(h.revision),0) FROM governed_model_turn_history h WHERE h.turn_id=c.turn_id) FROM governed_model_turn_current c WHERE c.turn_id=?`, id).Scan(&revision, &digest, &highest, &history)
	if errors.Is(err, sql.ErrNoRows) {
		return turnCurrentV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorNotFound, "inspect_current_turn_v2", "turn absent", nil)
	}
	if err != nil {
		return turnCurrentV2{}, mapDBErrorV1(ctx, "inspect_current_turn_v2", err, false)
	}
	if revision == 0 || revision != highest || highest != history {
		return turnCurrentV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_current_turn_v2", "current/history drifted", nil)
	}
	return turnCurrentV2{core.Revision(revision), core.Revision(highest), core.Digest(digest)}, nil
}
func loadTurnHistoryV2(ctx context.Context, source interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string, revision core.Revision) (turnStoredV2, error) {
	var digest, attempt string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT fact_digest,attempt_digest,canonical_json FROM governed_model_turn_history WHERE turn_id=? AND revision=?`, id, revision).Scan(&digest, &attempt, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return turnStoredV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorNotFound, "inspect_turn_v2", "turn history absent", nil)
	}
	if err != nil {
		return turnStoredV2{}, mapDBErrorV1(ctx, "inspect_turn_v2", err, false)
	}
	var outcome modelinvoker.GovernedModelTurnOutcomeV2
	if err := core.DecodeStrictJSON(payload, &outcome); err != nil || outcome.Validate() != nil || outcome.ID != id || outcome.Revision != revision || digest != string(outcome.Digest) {
		return turnStoredV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_turn_v2", "stored turn failed exact revalidation", err)
	}
	computed, err := turnAttemptDigestV2(outcome)
	if err != nil || attempt != string(computed) {
		return turnStoredV2{}, errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "inspect_turn_v2", "stored attempt drifted", err)
	}
	return turnStoredV2{outcome.CloneV2(), append(json.RawMessage(nil), payload...), computed}, nil
}
func encodeTurnV2(outcome modelinvoker.GovernedModelTurnOutcomeV2) (json.RawMessage, core.Digest, error) {
	if err := outcome.Validate(); err != nil {
		return nil, "", err
	}
	wire, err := json.Marshal(outcome)
	if err != nil {
		return nil, "", err
	}
	attempt, err := turnAttemptDigestV2(outcome)
	return wire, attempt, err
}
func turnAttemptDigestV2(outcome modelinvoker.GovernedModelTurnOutcomeV2) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.model-invoker.sqlite", "v2", "GovernedModelTurnAttemptV2", struct {
		Prepared modelinvoker.PreparedModelInvocationRefV1 `json:"prepared"`
		Sequence uint64                                    `json:"sequence"`
		Ordinal  uint32                                    `json:"ordinal"`
	}{outcome.PreparedRef, outcome.DispatchSequence, outcome.ProviderAttemptOrdinal})
}
func insertTurnHistoryV2(ctx context.Context, tx *sql.Tx, outcome modelinvoker.GovernedModelTurnOutcomeV2, attempt core.Digest, wire json.RawMessage) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO governed_model_turn_history(turn_id,revision,fact_digest,attempt_digest,canonical_json) VALUES(?,?,?,?,?)`, outcome.ID, outcome.Revision, string(outcome.Digest), string(attempt), wire)
	return mapDBErrorV1(ctx, "write_turn_history_v2", err, true)
}

var _ modelinvoker.GovernedModelTurnRepositoryV2 = (*Store)(nil)
