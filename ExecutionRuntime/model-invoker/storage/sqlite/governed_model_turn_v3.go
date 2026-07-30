package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

type governedModelTurnRowV3 struct {
	turnID                  string
	revision                uint64
	factDigest              string
	attemptDigest           string
	preparedID              string
	preparedRevision        uint64
	preparedDigest          string
	currentID               string
	currentRevision         uint64
	currentDigest           string
	currentCheckedUnixNano  int64
	currentExpiresUnixNano  int64
	currentNotAfterUnixNano int64
	materialID              string
	materialRevision        uint64
	materialDigest          string
	attemptRequestDigest    string
	routeCallDigest         string
	dispatchSequence        uint64
	providerAttemptOrdinal  uint64
	expiresUnixNano         int64
	canonicalJSON           []byte
}

func (s *Store) EnsurePreparedGovernedModelTurnV3(
	ctx context.Context,
	outcome modelinvoker.GovernedModelTurnOutcomeV3,
) (modelinvoker.GovernedModelTurnMutationV3, error) {
	if err := contextErrorV1(ctx, "ensure_turn_v3"); err != nil {
		return modelinvoker.GovernedModelTurnMutationV3{}, err
	}
	if err := outcome.Validate(); err != nil ||
		outcome.State != modelinvoker.GovernedModelTurnPreparedV3 ||
		outcome.Revision != 1 {
		return modelinvoker.GovernedModelTurnMutationV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorInvalid,
			"ensure_turn_v3",
			"prepared turn V3 is invalid",
			err,
		)
	}
	wire, err := modelinvoker.EncodeGovernedModelTurnOutcomeV3(outcome)
	if err != nil {
		return modelinvoker.GovernedModelTurnMutationV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorInvalid,
			"ensure_turn_v3",
			"prepared turn V3 cannot be encoded",
			err,
		)
	}
	tx, err := s.beginV1(ctx, "ensure_turn_v3")
	if err != nil {
		return modelinvoker.GovernedModelTurnMutationV3{}, err
	}
	defer func() { _ = tx.Rollback() }()

	stored, err := loadGovernedModelTurnRowV3(
		ctx,
		tx,
		`WHERE turn_id=? AND revision=?`,
		outcome.ID,
		outcome.Revision,
	)
	if err == nil {
		if err := stored.validateAgainstV3(outcome, wire); err != nil {
			return modelinvoker.GovernedModelTurnMutationV3{}, err
		}
		return modelinvoker.GovernedModelTurnMutationV3{
			Outcome: outcome.CloneV3(),
			Applied: false,
		}, nil
	}
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) !=
		modelinvoker.GovernedModelInvocationErrorNotFound {
		return modelinvoker.GovernedModelTurnMutationV3{}, err
	}

	attempt := outcome.AttemptRefV3()
	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO governed_model_turn_v3_history(
		   turn_id,revision,fact_digest,attempt_digest,
		   prepared_id,prepared_revision,prepared_digest,
		   current_id,current_revision,current_digest,
		   current_checked_unix_nano,current_expires_unix_nano,current_not_after_unix_nano,
		   material_id,material_revision,material_digest,
		   attempt_request_digest,route_call_digest,
		   dispatch_sequence,provider_attempt_ordinal,
		   expires_unix_nano,canonical_json
		 ) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		outcome.ID,
		outcome.Revision,
		string(outcome.Digest),
		string(attempt.Digest),
		outcome.PreparedRef.ID,
		outcome.PreparedRef.Revision,
		string(outcome.PreparedRef.Digest),
		outcome.CurrentRef.ID,
		outcome.CurrentRef.Revision,
		string(outcome.CurrentRef.Digest),
		outcome.CurrentRef.CheckedUnixNano,
		outcome.CurrentRef.ExpiresUnixNano,
		outcome.CurrentRef.NotAfterUnixNano,
		outcome.MaterialRef.ID,
		outcome.MaterialRef.Revision,
		string(outcome.MaterialRef.Digest),
		string(outcome.AttemptRequestDigest),
		string(outcome.RouteCallDigest),
		outcome.DispatchSequence,
		outcome.ProviderAttemptOrdinal,
		outcome.ExpiresUnixNano,
		wire,
	); err != nil {
		return modelinvoker.GovernedModelTurnMutationV3{}, mapDBErrorV1(
			ctx,
			"ensure_turn_v3",
			err,
			true,
		)
	}
	if err := tx.Commit(); err != nil {
		return modelinvoker.GovernedModelTurnMutationV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorIndeterminate,
			"ensure_turn_v3",
			"prepared turn V3 commit outcome is unknown",
			err,
		)
	}
	return modelinvoker.GovernedModelTurnMutationV3{
		Outcome: outcome.CloneV3(),
		Applied: true,
	}, nil
}

func (s *Store) InspectGovernedModelTurnAttemptV3(
	ctx context.Context,
	ref modelinvoker.GovernedModelTurnAttemptRefV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	if err := contextErrorV1(ctx, "inspect_turn_attempt_v3"); err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, err
	}
	if err := ref.Validate(); err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorInvalid,
			"inspect_turn_attempt_v3",
			"turn V3 AttemptRef is invalid",
			err,
		)
	}
	row, err := loadGovernedModelTurnRowV3(
		ctx,
		s.db,
		`WHERE attempt_digest=?`,
		string(ref.Digest),
	)
	if err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, err
	}
	outcome, err := row.decodeV3()
	if err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, err
	}
	if err := outcome.ValidateAgainstAttemptRefV3(ref); err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"inspect_turn_attempt_v3",
			"stored turn V3 differs from original AttemptRef",
			err,
		)
	}
	return outcome.CloneV3(), nil
}

func (s *Store) InspectExactGovernedModelTurnV3(
	ctx context.Context,
	ref modelinvoker.GovernedModelTurnRefV3,
) (modelinvoker.GovernedModelTurnOutcomeV3, error) {
	if err := contextErrorV1(ctx, "inspect_turn_v3"); err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, err
	}
	if err := ref.Validate(); err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorInvalid,
			"inspect_turn_v3",
			"exact turn V3 Ref is invalid",
			err,
		)
	}
	row, err := loadGovernedModelTurnRowV3(
		ctx,
		s.db,
		`WHERE turn_id=? AND revision=?`,
		ref.ID,
		ref.Revision,
	)
	if err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, err
	}
	outcome, err := row.decodeV3()
	if err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, err
	}
	if outcome.RefV3() != ref {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"inspect_turn_v3",
			"stored turn V3 failed exact Ref revalidation",
			nil,
		)
	}
	return outcome.CloneV3(), nil
}

func loadGovernedModelTurnRowV3(
	ctx context.Context,
	query interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	clause string,
	args ...any,
) (governedModelTurnRowV3, error) {
	const columns = `
SELECT turn_id,revision,fact_digest,attempt_digest,
       prepared_id,prepared_revision,prepared_digest,
       current_id,current_revision,current_digest,
       current_checked_unix_nano,current_expires_unix_nano,current_not_after_unix_nano,
       material_id,material_revision,material_digest,
       attempt_request_digest,route_call_digest,
       dispatch_sequence,provider_attempt_ordinal,
       expires_unix_nano,canonical_json
FROM governed_model_turn_v3_history `
	var row governedModelTurnRowV3
	err := query.QueryRowContext(ctx, columns+clause, args...).Scan(
		&row.turnID,
		&row.revision,
		&row.factDigest,
		&row.attemptDigest,
		&row.preparedID,
		&row.preparedRevision,
		&row.preparedDigest,
		&row.currentID,
		&row.currentRevision,
		&row.currentDigest,
		&row.currentCheckedUnixNano,
		&row.currentExpiresUnixNano,
		&row.currentNotAfterUnixNano,
		&row.materialID,
		&row.materialRevision,
		&row.materialDigest,
		&row.attemptRequestDigest,
		&row.routeCallDigest,
		&row.dispatchSequence,
		&row.providerAttemptOrdinal,
		&row.expiresUnixNano,
		&row.canonicalJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return governedModelTurnRowV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorNotFound,
			"inspect_turn_v3",
			"turn V3 history is absent",
			nil,
		)
	}
	if err != nil {
		return governedModelTurnRowV3{}, mapDBErrorV1(
			ctx,
			"inspect_turn_v3",
			err,
			false,
		)
	}
	return row, nil
}

func (r governedModelTurnRowV3) decodeV3() (
	modelinvoker.GovernedModelTurnOutcomeV3,
	error,
) {
	outcome, err := modelinvoker.DecodeGovernedModelTurnOutcomeV3(r.canonicalJSON)
	if err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"inspect_turn_v3",
			"stored turn V3 canonical payload is invalid",
			err,
		)
	}
	wire, err := modelinvoker.EncodeGovernedModelTurnOutcomeV3(outcome)
	if err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, errorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"inspect_turn_v3",
			"stored turn V3 cannot be re-encoded",
			err,
		)
	}
	if err := r.validateAgainstV3(outcome, wire); err != nil {
		return modelinvoker.GovernedModelTurnOutcomeV3{}, err
	}
	return outcome.CloneV3(), nil
}

func (r governedModelTurnRowV3) validateAgainstV3(
	outcome modelinvoker.GovernedModelTurnOutcomeV3,
	wire []byte,
) error {
	attempt := outcome.AttemptRefV3()
	if r.turnID != outcome.ID ||
		r.revision != uint64(outcome.Revision) ||
		r.factDigest != string(outcome.Digest) ||
		r.attemptDigest != string(attempt.Digest) ||
		r.preparedID != outcome.PreparedRef.ID ||
		r.preparedRevision != uint64(outcome.PreparedRef.Revision) ||
		r.preparedDigest != string(outcome.PreparedRef.Digest) ||
		r.currentID != outcome.CurrentRef.ID ||
		r.currentRevision != uint64(outcome.CurrentRef.Revision) ||
		r.currentDigest != string(outcome.CurrentRef.Digest) ||
		r.currentCheckedUnixNano != outcome.CurrentRef.CheckedUnixNano ||
		r.currentExpiresUnixNano != outcome.CurrentRef.ExpiresUnixNano ||
		r.currentNotAfterUnixNano != outcome.CurrentRef.NotAfterUnixNano ||
		r.materialID != outcome.MaterialRef.ID ||
		r.materialRevision != uint64(outcome.MaterialRef.Revision) ||
		r.materialDigest != string(outcome.MaterialRef.Digest) ||
		r.attemptRequestDigest != string(outcome.AttemptRequestDigest) ||
		r.routeCallDigest != string(outcome.RouteCallDigest) ||
		r.dispatchSequence != outcome.DispatchSequence ||
		r.providerAttemptOrdinal != uint64(outcome.ProviderAttemptOrdinal) ||
		r.expiresUnixNano != outcome.ExpiresUnixNano ||
		!bytes.Equal(r.canonicalJSON, wire) {
		return errorV1(
			modelinvoker.GovernedModelInvocationErrorConflict,
			"inspect_turn_v3",
			"stored turn V3 exact coordinates drifted",
			nil,
		)
	}
	return nil
}

var _ modelinvoker.GovernedModelTurnRepositoryV3 = (*Store)(nil)
