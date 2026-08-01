package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"reflect"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func (s *Store) createWorkspaceReadCommandV1(ctx context.Context, value contract.WorkspaceReadCommandV1) (contract.WorkspaceReadCommandV1, error) {
	if err := value.ValidateCurrent(s.clock()); err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	body, err := encode(value)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	bodySeal, err := workspaceReadCommandCanonicalBodySealV1(value.Meta.Ref(), body)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO workspace_read_command_current(command_id,revision,digest,body)
		 VALUES(?,?,?,?)
		 ON CONFLICT(command_id) DO NOTHING`,
		value.Meta.ID,
		value.Meta.Revision,
		value.Meta.Digest,
		body,
	)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, classifyWrite(err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	if rows == 1 {
		if _, err = tx.ExecContext(
			ctx,
			`INSERT INTO workspace_read_command_body_seal(
				command_id,revision,digest,canonical_body_digest
			 ) VALUES(?,?,?,?)`,
			value.Meta.ID,
			value.Meta.Revision,
			value.Meta.Digest,
			bodySeal,
		); err != nil {
			return contract.WorkspaceReadCommandV1{}, classifyWrite(err)
		}
	} else if rows != 0 {
		return contract.WorkspaceReadCommandV1{}, ports.ErrConflict
	}
	if err = tx.Commit(); err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	stored, err := s.InspectWorkspaceReadCommandExactV1(ctx, value.Meta.Ref())
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	return stored, stored.ValidateCurrent(s.clock())
}

func (s *Store) inspectWorkspaceReadCommandCurrentV1(ctx context.Context, exact contract.Ref) (contract.WorkspaceReadCommandV1, error) {
	if err := exact.ValidateShape("workspace read command"); err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	var rev uint64
	var digest string
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT revision,digest,body FROM workspace_read_command_current WHERE command_id=?`, exact.ID).Scan(&rev, &digest, &body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceReadCommandV1{}, ports.ErrNotFound
		}
		return contract.WorkspaceReadCommandV1{}, err
	}
	if rev != exact.Revision || digest != exact.Digest {
		return contract.WorkspaceReadCommandV1{}, ports.ErrConflict
	}
	var value contract.WorkspaceReadCommandV1
	if err := decode(body, &value); err != nil {
		return value, err
	}
	return value, value.ValidateCurrent(s.clock())
}

// InspectWorkspaceReadCommandExactV1 returns the immutable Command body sealed
// at creation. The stored expiry remains evidence in that body; it is not a
// currentness gate for historical inspection and is never refreshed here.
func (s *Store) InspectWorkspaceReadCommandExactV1(ctx context.Context, exact contract.Ref) (contract.WorkspaceReadCommandV1, error) {
	if ctx == nil {
		return contract.WorkspaceReadCommandV1{}, errors.New("workspace read exact Command context is required")
	}
	if err := ctx.Err(); err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	if s == nil || s.db == nil {
		return contract.WorkspaceReadCommandV1{}, errors.New("workspace read exact Command reader is unavailable")
	}
	return inspectWorkspaceReadCommandExactTxV1(ctx, s.db, exact)
}

func inspectWorkspaceReadCommandExactTxV1(ctx context.Context, source queryer, exact contract.Ref) (contract.WorkspaceReadCommandV1, error) {
	if err := exact.ValidateShape("workspace read command"); err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	var revision uint64
	var digest string
	var body []byte
	var sealRevision sql.NullInt64
	var sealDigest sql.NullString
	var canonicalBodyDigest sql.NullString
	if err := source.QueryRowContext(
		ctx,
		`SELECT command.revision,command.digest,command.body,
		        seal.revision,seal.digest,seal.canonical_body_digest
		   FROM workspace_read_command_current AS command
		   LEFT JOIN workspace_read_command_body_seal AS seal
		     ON seal.command_id=command.command_id
		  WHERE command.command_id=?`,
		exact.ID,
	).Scan(
		&revision,
		&digest,
		&body,
		&sealRevision,
		&sealDigest,
		&canonicalBodyDigest,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceReadCommandV1{}, ports.ErrNotFound
		}
		return contract.WorkspaceReadCommandV1{}, err
	}
	if revision != exact.Revision || digest != exact.Digest {
		return contract.WorkspaceReadCommandV1{}, ports.ErrConflict
	}
	if !sealRevision.Valid || sealRevision.Int64 <= 0 ||
		uint64(sealRevision.Int64) != exact.Revision ||
		!sealDigest.Valid || sealDigest.String != exact.Digest ||
		!canonicalBodyDigest.Valid ||
		!contract.ValidDigest(canonicalBodyDigest.String) {
		return contract.WorkspaceReadCommandV1{}, ports.ErrConflict
	}
	var command contract.WorkspaceReadCommandV1
	if err := decode(body, &command); err != nil {
		return contract.WorkspaceReadCommandV1{}, ports.ErrConflict
	}
	canonical, err := encode(command)
	if err != nil || !bytes.Equal(canonical, body) {
		return contract.WorkspaceReadCommandV1{}, ports.ErrConflict
	}
	expectedBodySeal, err := workspaceReadCommandCanonicalBodySealV1(exact, canonical)
	if err != nil || expectedBodySeal != canonicalBodyDigest.String {
		return contract.WorkspaceReadCommandV1{}, ports.ErrConflict
	}
	if err := command.ValidateShape(); err != nil {
		return contract.WorkspaceReadCommandV1{}, ports.ErrConflict
	}
	if command.Meta.Ref() != exact {
		return contract.WorkspaceReadCommandV1{}, ports.ErrConflict
	}
	return command, nil
}

func workspaceReadCommandCanonicalBodySealV1(
	exact contract.Ref,
	canonical []byte,
) (string, error) {
	if err := exact.ValidateShape("workspace read command body seal"); err != nil {
		return "", err
	}
	if len(canonical) == 0 {
		return "", errors.New("workspace read Command canonical body is required")
	}
	return contract.Digest(
		"workspace-read-command-canonical-body-row/v1",
		struct {
			Exact         contract.Ref `json:"exact"`
			CanonicalBody string       `json:"canonical_body"`
		}{
			Exact:         exact,
			CanonicalBody: string(canonical),
		},
	)
}

func (s *Store) ReserveWorkspaceReadV1(ctx context.Context, res contract.WorkspaceReadReservationV1, attempt contract.WorkspaceReadAttemptV1, binding ports.WorkspaceReadAdmissionAttemptBindingV1) (contract.WorkspaceReadExecutionProjectionV1, bool, error) {
	return s.reserveWorkspaceReadV1(ctx, res, attempt, binding, nil)
}

func (s *Store) reserveWorkspaceReadV1(ctx context.Context, res contract.WorkspaceReadReservationV1, attempt contract.WorkspaceReadAttemptV1, binding ports.WorkspaceReadAdmissionAttemptBindingV1, authorization *runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3) (contract.WorkspaceReadExecutionProjectionV1, bool, error) {
	now := s.clock()
	if err := res.ValidateCurrent(now); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, err
	}
	if err := attempt.ValidateCurrent(now); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, err
	}
	if err := binding.Validate(); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, err
	}
	if authorization != nil {
		authorizationDigest, err := authorization.DigestV3()
		if err != nil ||
			authorization.ValidateCurrent(now) != nil ||
			authorizationDigest != authorization.AuthorizationDigest ||
			authorization.AuthorizationDigest != binding.AuthorizationDigest ||
			authorization.Association != binding.Association ||
			authorization.DomainCommand != binding.DomainCommand {
			return contract.WorkspaceReadExecutionProjectionV1{}, false, ports.ErrConflict
		}
	}
	expectedAttemptExpiry := minWorkspaceReadStoreExpiryV1(res.Meta.ExpiresUnixNano, attempt.AdmissionReceipt.ExpiresUnixNano)
	if attempt.State != contract.WorkspaceReadStartedV1 ||
		attempt.StableKeyDigest != res.StableKeyDigest ||
		attempt.RequestDigest != res.RequestDigest ||
		attempt.PayloadDigest != res.PayloadDigest ||
		res.AttemptID != attempt.Meta.ID ||
		!contract.SameRef(attempt.Reservation, res.Meta.Ref()) ||
		attempt.AdmissionReceipt.StableKeyDigest != res.StableKeyDigest ||
		attempt.Meta.ExpiresUnixNano != expectedAttemptExpiry ||
		binding.Attempt.OwnerRef() != attempt.Meta.Ref() ||
		binding.Command != res.Command ||
		string(binding.AuthorizationDigest) != res.AuthorizationDigest ||
		string(binding.StableKeyDigest) != res.StableKeyDigest ||
		binding.AdmissionReceipt.ID != attempt.AdmissionReceipt.ID ||
		uint64(binding.AdmissionReceipt.Revision) != attempt.AdmissionReceipt.Revision ||
		string(binding.AdmissionReceipt.Digest) != attempt.AdmissionReceipt.Digest ||
		string(binding.AdmissionReceipt.StableKeyDigest) != attempt.AdmissionReceipt.StableKeyDigest ||
		binding.DomainCommand.ID != res.Command.ID ||
		uint64(binding.DomainCommand.Revision) != res.Command.Revision ||
		string(binding.DomainCommand.Digest) != "sha256:"+res.Command.Digest ||
		binding.CreatedUnixNano != attempt.Meta.CreatedUnixNano ||
		binding.ExpiresUnixNano != attempt.Meta.ExpiresUnixNano {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, ports.ErrConflict
	}
	rb, err := encode(res)
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, err
	}
	ab, err := encode(attempt)
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, err
	}
	bb, err := encode(binding)
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, err
	}
	var bindingV2 ports.WorkspaceReadAdmissionAttemptBindingV2
	var bindingV2Body []byte
	var runtimeAttemptDigest runtimecore.Digest
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, err
	}
	defer tx.Rollback()
	if authorization != nil {
		command, inspectErr := inspectWorkspaceReadCommandExactTxV1(ctx, tx, res.Command)
		if inspectErr != nil || command.ValidateCurrent(now) != nil {
			return contract.WorkspaceReadExecutionProjectionV1{}, false, ports.ErrConflict
		}
		bindingV2, err = sealWorkspaceReadAdmissionAttemptBindingV2(*authorization, binding, command)
		if err != nil || validateWorkspaceReadRuntimeAttemptAdmissionClosureV2(res, attempt, command, bindingV2) != nil {
			return contract.WorkspaceReadExecutionProjectionV1{}, false, ports.ErrConflict
		}
		bindingV2Body, err = encode(bindingV2)
		if err != nil {
			return contract.WorkspaceReadExecutionProjectionV1{}, false, err
		}
		runtimeAttemptDigest, err = ports.WorkspaceReadRuntimeAttemptDigestV2(bindingV2.RuntimeAttempt)
		if err != nil {
			return contract.WorkspaceReadExecutionProjectionV1{}, false, err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_read_reservation(stable_digest,reservation_id,body) VALUES(?,?,?)`, res.StableKeyDigest, res.Meta.ID, rb); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, classifyWrite(err)
	}
	originResult, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_read_attempt_origin(attempt_id,stable_digest,revision,digest,body) VALUES(?,?,?,?,?)`, attempt.Meta.ID, res.StableKeyDigest, attempt.Meta.Revision, attempt.Meta.Digest, ab)
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, classifyWrite(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_read_attempt_owner_incarnation(attempt_id,owner_incarnation_id,reserved_unix_nano) VALUES(?,?,?)`, attempt.Meta.ID, s.workspaceReadOwnerIncarnation, now.UnixNano()); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, classifyWrite(err)
	}
	bindingResult, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_read_admission_attempt_binding(admission_id,admission_revision,admission_digest,attempt_id,attempt_revision,attempt_digest,body) VALUES(?,?,?,?,?,?,?)`,
		binding.AdmissionReceipt.ID, binding.AdmissionReceipt.Revision, binding.AdmissionReceipt.Digest,
		binding.Attempt.ID, binding.Attempt.Revision, binding.Attempt.Digest, bb)
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, classifyWrite(err)
	}
	var bindingV2Result sql.Result
	if authorization != nil {
		delegationPresent := 0
		delegationID := ""
		var delegationRevision runtimecore.Revision
		var delegationDigest runtimecore.Digest
		if bindingV2.RuntimeAttempt.Delegation != nil {
			delegationPresent = 1
			delegationID = bindingV2.RuntimeAttempt.Delegation.ID
			delegationRevision = bindingV2.RuntimeAttempt.Delegation.Revision
			delegationDigest = bindingV2.RuntimeAttempt.Delegation.Digest
		}
		bindingV2Result, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_read_runtime_attempt_admission_binding_v2(
			runtime_attempt_digest,operation_digest,effect_id,intent_revision,intent_digest,
			permit_id,permit_revision,permit_digest,runtime_attempt_id,
			delegation_present,delegation_id,delegation_revision,delegation_digest,
			authorization_digest,
			association_id,association_revision,association_digest,
			domain_command_id,domain_command_revision,domain_command_digest,
			command_id,command_revision,command_digest,
			admission_id,admission_revision,admission_digest,
			workspace_attempt_id,workspace_attempt_revision,workspace_attempt_digest,
			binding_digest,body
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			runtimeAttemptDigest,
			bindingV2.RuntimeAttempt.OperationDigest,
			bindingV2.RuntimeAttempt.EffectID,
			bindingV2.RuntimeAttempt.IntentRevision,
			bindingV2.RuntimeAttempt.IntentDigest,
			bindingV2.RuntimeAttempt.PermitID,
			bindingV2.RuntimeAttempt.PermitRevision,
			bindingV2.RuntimeAttempt.PermitDigest,
			bindingV2.RuntimeAttempt.AttemptID,
			delegationPresent,
			delegationID,
			delegationRevision,
			delegationDigest,
			bindingV2.AuthorizationDigest,
			bindingV2.Association.ID,
			bindingV2.Association.Revision,
			bindingV2.Association.Digest,
			bindingV2.DomainCommand.ID,
			bindingV2.DomainCommand.Revision,
			bindingV2.DomainCommand.Digest,
			bindingV2.WorkspaceReadCommand.Meta.ID,
			bindingV2.WorkspaceReadCommand.Meta.Revision,
			bindingV2.WorkspaceReadCommand.Meta.Digest,
			bindingV2.AdmissionReceipt.ID,
			bindingV2.AdmissionReceipt.Revision,
			bindingV2.AdmissionReceipt.Digest,
			bindingV2.WorkspaceReadAttempt.ID,
			bindingV2.WorkspaceReadAttempt.Revision,
			bindingV2.WorkspaceReadAttempt.Digest,
			bindingV2.Digest,
			bindingV2Body,
		)
		if err != nil {
			return contract.WorkspaceReadExecutionProjectionV1{}, false, classifyWrite(err)
		}
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_read_attempt_current(stable_digest,attempt_id,revision,digest,body) VALUES(?,?,?,?,?)`, res.StableKeyDigest, attempt.Meta.ID, attempt.Meta.Revision, attempt.Meta.Digest, ab)
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, classifyWrite(err)
	}
	rows, err := originResult.RowsAffected()
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, err
	}
	projection, err := s.inspectWorkspaceReadTx(ctx, tx, res.StableKeyDigest)
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, err
	}
	var originBody []byte
	if err = tx.QueryRowContext(ctx, `SELECT body FROM workspace_read_attempt_origin WHERE attempt_id=?`, attempt.Meta.ID).Scan(&originBody); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, err
	}
	var origin contract.WorkspaceReadAttemptV1
	if err = decode(originBody, &origin); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, err
	}
	if projection.Reservation.Meta.Ref() != res.Meta.Ref() || projection.Reservation.RequestDigest != res.RequestDigest || projection.Reservation.PayloadDigest != res.PayloadDigest || projection.Reservation.Command != res.Command || origin.Meta.Ref() != attempt.Meta.Ref() || origin.StableKeyDigest != attempt.StableKeyDigest || origin.RequestDigest != attempt.RequestDigest || origin.PayloadDigest != attempt.PayloadDigest || origin.AdmissionReceipt != attempt.AdmissionReceipt {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, ports.ErrConflict
	}
	var storedBindingBody []byte
	if err = tx.QueryRowContext(ctx, `SELECT body FROM workspace_read_admission_attempt_binding WHERE admission_id=? AND admission_revision=? AND admission_digest=?`,
		binding.AdmissionReceipt.ID, binding.AdmissionReceipt.Revision, binding.AdmissionReceipt.Digest).Scan(&storedBindingBody); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, err
	}
	var storedBinding ports.WorkspaceReadAdmissionAttemptBindingV1
	if err = decode(storedBindingBody, &storedBinding); err != nil || storedBinding.Validate() != nil || storedBinding != binding {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, ports.ErrConflict
	}
	var bindingV2Rows int64
	if authorization != nil {
		storedBindingV2, inspectErr := inspectWorkspaceReadAdmissionForRuntimeAttemptTxV2(ctx, tx, bindingV2.RuntimeAttempt)
		if inspectErr != nil || !reflect.DeepEqual(storedBindingV2, bindingV2) {
			return contract.WorkspaceReadExecutionProjectionV1{}, false, ports.ErrConflict
		}
		if validateWorkspaceReadRuntimeAttemptAdmissionClosureV2(res, origin, bindingV2.WorkspaceReadCommand, bindingV2) != nil {
			return contract.WorkspaceReadExecutionProjectionV1{}, false, ports.ErrConflict
		}
		bindingV2Rows, err = bindingV2Result.RowsAffected()
		if err != nil {
			return contract.WorkspaceReadExecutionProjectionV1{}, false, err
		}
	}
	currentRows, currentRowsErr := result.RowsAffected()
	bindingRows, bindingRowsErr := bindingResult.RowsAffected()
	if currentRowsErr != nil || bindingRowsErr != nil || currentRows != rows || bindingRows != rows ||
		authorization != nil && bindingV2Rows != rows {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, ports.ErrConflict
	}
	if rows == 1 {
		var ownerIncarnation string
		if err = tx.QueryRowContext(ctx, `SELECT owner_incarnation_id FROM workspace_read_attempt_owner_incarnation WHERE attempt_id=?`, attempt.Meta.ID).Scan(&ownerIncarnation); err != nil || ownerIncarnation != s.workspaceReadOwnerIncarnation {
			return contract.WorkspaceReadExecutionProjectionV1{}, false, ports.ErrConflict
		}
	}
	if err = tx.Commit(); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, err
	}
	return projection, rows == 1, nil
}

var _ ports.WorkspaceReadCommandExactReaderV1 = (*Store)(nil)

func (s *Store) InspectWorkspaceReadAttemptForAdmissionV1(ctx context.Context, admission runtimeports.ControlledOperationProviderAdmissionReceiptRefV2) (ports.WorkspaceReadAdmissionAttemptBindingV1, error) {
	if err := admission.Validate(); err != nil || !admission.Admitted || admission.NoEffect {
		if err != nil {
			return ports.WorkspaceReadAdmissionAttemptBindingV1{}, err
		}
		return ports.WorkspaceReadAdmissionAttemptBindingV1{}, ports.ErrConflict
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM workspace_read_admission_attempt_binding WHERE admission_id=? AND admission_revision=? AND admission_digest=?`,
		admission.ID, admission.Revision, admission.Digest).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ports.WorkspaceReadAdmissionAttemptBindingV1{}, ports.ErrNotFound
		}
		return ports.WorkspaceReadAdmissionAttemptBindingV1{}, err
	}
	var binding ports.WorkspaceReadAdmissionAttemptBindingV1
	if err := decode(body, &binding); err != nil {
		return binding, err
	}
	if err := binding.Validate(); err != nil {
		return binding, err
	}
	if binding.AdmissionReceipt != admission {
		return ports.WorkspaceReadAdmissionAttemptBindingV1{}, ports.ErrConflict
	}
	return binding, nil
}

func (s *Store) CompleteWorkspaceReadV1(ctx context.Context, expected contract.Ref, observation contract.WorkspaceReadObservationV1) (contract.WorkspaceReadExecutionProjectionV1, error) {
	return s.finishWorkspaceReadV1(ctx, expected, &observation, "", "")
}
func (s *Store) MarkWorkspaceReadUnknownV1(ctx context.Context, expected contract.Ref, unknown string) (contract.WorkspaceReadExecutionProjectionV1, error) {
	return s.finishWorkspaceReadV1(ctx, expected, nil, unknown, "")
}
func (s *Store) FailWorkspaceReadV1(ctx context.Context, expected contract.Ref, failure string) (contract.WorkspaceReadExecutionProjectionV1, error) {
	return s.finishWorkspaceReadV1(ctx, expected, nil, "", failure)
}

// RecoverStartedWorkspaceReadAfterRestartV1 is an explicit Owner recovery
// action. Ordinary Inspect remains read-only. Recovery is permitted only when
// the durable origin was reserved by a different Store incarnation.
func (s *Store) RecoverStartedWorkspaceReadAfterRestartV1(ctx context.Context, exact contract.WorkspaceReadAttemptRefV1) (contract.WorkspaceReadExecutionProjectionV1, error) {
	if err := exact.Validate(); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	now := s.clock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	defer tx.Rollback()
	var stable, originalDigest, previousIncarnation string
	var originalRevision uint64
	if err = tx.QueryRowContext(ctx, `SELECT stable_digest,revision,digest FROM workspace_read_attempt_origin WHERE attempt_id=?`, exact.ID).Scan(&stable, &originalRevision, &originalDigest); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrNotFound
		}
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	if originalRevision != exact.Revision || originalDigest != exact.Digest {
		return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
	}
	if err = tx.QueryRowContext(ctx, `SELECT owner_incarnation_id FROM workspace_read_attempt_owner_incarnation WHERE attempt_id=?`, exact.ID).Scan(&previousIncarnation); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	if previousIncarnation == "" || previousIncarnation == s.workspaceReadOwnerIncarnation {
		return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
	}
	var currentRevision uint64
	var currentDigest string
	var currentBody []byte
	if err = tx.QueryRowContext(ctx, `SELECT revision,digest,body FROM workspace_read_attempt_current WHERE attempt_id=?`, exact.ID).Scan(&currentRevision, &currentDigest, &currentBody); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	var current contract.WorkspaceReadAttemptV1
	if err = decode(currentBody, &current); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	if current.State != contract.WorkspaceReadStartedV1 {
		var evidenceIncarnation string
		if current.State == contract.WorkspaceReadUnknownV1 {
			_ = tx.QueryRowContext(ctx, `SELECT current_owner_incarnation_id FROM workspace_read_recovery_evidence WHERE attempt_id=?`, exact.ID).Scan(&evidenceIncarnation)
			if evidenceIncarnation == s.workspaceReadOwnerIncarnation {
				return s.inspectWorkspaceReadTx(ctx, tx, stable)
			}
		}
		return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
	}
	if err = current.ValidateCurrent(now); err != nil || currentRevision != current.Meta.Revision || currentDigest != current.Meta.Digest || now.UnixNano() <= current.Meta.CreatedUnixNano {
		if err != nil {
			return contract.WorkspaceReadExecutionProjectionV1{}, err
		}
		return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
	}
	evidenceDigest, err := contract.Digest("workspace-read-restart-recovery", struct {
		Attempt                    contract.WorkspaceReadAttemptRefV1
		PreviousOwnerIncarnationID string
		CurrentOwnerIncarnationID  string
		RecoveredUnixNano          int64
	}{exact, previousIncarnation, s.workspaceReadOwnerIncarnation, now.UnixNano()})
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	next := current
	next.State = contract.WorkspaceReadUnknownV1
	next.UnknownDigest = evidenceDigest
	next.FailureDigest = ""
	next, err = contract.SealWorkspaceReadAttemptV1(next, current.Meta.ID, current.Meta.Revision+1, now, time.Unix(0, current.Meta.ExpiresUnixNano))
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	nextBody, err := encode(next)
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE workspace_read_attempt_current SET revision=?,digest=?,body=? WHERE attempt_id=? AND revision=? AND digest=?`, next.Meta.Revision, next.Meta.Digest, nextBody, exact.ID, current.Meta.Revision, current.Meta.Digest)
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO workspace_read_recovery_evidence(attempt_id,previous_owner_incarnation_id,current_owner_incarnation_id,recovered_unix_nano,evidence_digest) VALUES(?,?,?,?,?)`, exact.ID, previousIncarnation, s.workspaceReadOwnerIncarnation, now.UnixNano(), evidenceDigest); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, classifyWrite(err)
	}
	if err = tx.Commit(); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	return s.inspectWorkspaceReadStableV1(ctx, stable)
}

func (s *Store) finishWorkspaceReadV1(ctx context.Context, expected contract.Ref, observation *contract.WorkspaceReadObservationV1, unknown, failure string) (contract.WorkspaceReadExecutionProjectionV1, error) {
	if observation == nil && !contract.ValidDigest(unknown) && !contract.ValidDigest(failure) {
		return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
	}
	if observation != nil && (unknown != "" || failure != "") || unknown != "" && failure != "" {
		return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	defer tx.Rollback()
	var stable string
	var revision uint64
	var digest string
	var body []byte
	if err = tx.QueryRowContext(ctx, `SELECT stable_digest,revision,digest,body FROM workspace_read_attempt_current WHERE attempt_id=?`, expected.ID).Scan(&stable, &revision, &digest, &body); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	var current contract.WorkspaceReadAttemptV1
	if err = decode(body, &current); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	now := s.clock()
	if err = current.ValidateCurrent(now); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	var reservationBody []byte
	if err = tx.QueryRowContext(ctx, `SELECT body FROM workspace_read_reservation WHERE stable_digest=?`, stable).Scan(&reservationBody); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	var reservation contract.WorkspaceReadReservationV1
	if err = decode(reservationBody, &reservation); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	if err = reservation.ValidateCurrent(now); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	if !contract.SameRef(current.Reservation, reservation.Meta.Ref()) || current.Meta.ExpiresUnixNano != minWorkspaceReadStoreExpiryV1(reservation.Meta.ExpiresUnixNano, current.AdmissionReceipt.ExpiresUnixNano) {
		return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
	}
	if current.State != contract.WorkspaceReadStartedV1 {
		projection, inspectErr := s.inspectWorkspaceReadTx(ctx, tx, stable)
		if inspectErr != nil {
			return projection, inspectErr
		}
		if observation != nil && (projection.Observation == nil || projection.Observation.Meta.Ref() != observation.Meta.Ref() || projection.Observation.ProviderReceipt != observation.ProviderReceipt) {
			return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
		}
		if observation == nil && (current.UnknownDigest != unknown || current.FailureDigest != failure) {
			return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
		}
		return projection, nil
	}
	if revision != expected.Revision || digest != expected.Digest {
		return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
	}
	next := current
	next.State = contract.WorkspaceReadUnknownV1
	next.UnknownDigest = unknown
	next.FailureDigest = ""
	if failure != "" {
		next.State = contract.WorkspaceReadFailedV1
		next.UnknownDigest = ""
		next.FailureDigest = failure
	}
	if observation != nil {
		if err = observation.ValidateCurrent(now); err != nil {
			return contract.WorkspaceReadExecutionProjectionV1{}, err
		}
		command, workspace, inputErr := inspectWorkspaceReadCompletionInputsTxV1(ctx, tx, reservation, now)
		if inputErr != nil {
			return contract.WorkspaceReadExecutionProjectionV1{}, inputErr
		}
		if !contract.SameRef(observation.Reservation, reservation.Meta.Ref()) ||
			!contract.SameRef(observation.Command, command.Meta.Ref()) ||
			!contract.SameRef(observation.WorkspaceView, workspace.Meta.Ref()) ||
			observation.RelativePath != command.RelativePath ||
			observation.StartByte != command.StartByte ||
			observation.ReturnedBytes > command.MaxBytes ||
			observation.File.Revision != workspace.Meta.Revision ||
			observation.AdmissionReceipt != current.AdmissionReceipt ||
			observation.Meta.ExpiresUnixNano != minWorkspaceReadStoreExpiryV1(current.Meta.ExpiresUnixNano, reservation.Meta.ExpiresUnixNano, current.AdmissionReceipt.ExpiresUnixNano, observation.ProviderReceipt.ExpiresUnixNano) {
			return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
		}
		fileID, fileIDErr := contract.WorkspaceReadFileIDV1(workspace.Meta.ID, command.RelativePath)
		if fileIDErr != nil || observation.File.ID != fileID || command.ExpectedFileRef != nil && !contract.SameRef(observation.File, *command.ExpectedFileRef) {
			return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
		}
		next.State = contract.WorkspaceReadObservedV1
		next.UnknownDigest = ""
		next.FailureDigest = ""
		ref := observation.Meta.Ref()
		next.Observation = &ref
	}
	next, err = contract.SealWorkspaceReadAttemptV1(next, current.Meta.ID, current.Meta.Revision+1, now, time.Unix(0, current.Meta.ExpiresUnixNano))
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	nb, _ := encode(next)
	result, err := tx.ExecContext(ctx, `UPDATE workspace_read_attempt_current SET revision=?,digest=?,body=? WHERE attempt_id=? AND revision=? AND digest=?`, next.Meta.Revision, next.Meta.Digest, nb, expected.ID, expected.Revision, expected.Digest)
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
	}
	if observation != nil {
		ob, _ := encode(*observation)
		if _, err = tx.ExecContext(ctx, `INSERT INTO workspace_read_observation(observation_id,stable_digest,body) VALUES(?,?,?)`, observation.Meta.ID, stable, ob); err != nil {
			return contract.WorkspaceReadExecutionProjectionV1{}, classifyWrite(err)
		}
	}
	if err = tx.Commit(); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	var res contract.WorkspaceReadReservationV1
	_ = res
	return s.inspectWorkspaceReadStableV1(ctx, stable)
}

func inspectWorkspaceReadCompletionInputsTxV1(ctx context.Context, tx *sql.Tx, reservation contract.WorkspaceReadReservationV1, now time.Time) (contract.WorkspaceReadCommandV1, contract.WorkspaceView, error) {
	current, err := inspectStoredWorkspaceReadCommandOwnerCurrentByCommandTxV2(
		ctx,
		tx,
		reservation.Command,
	)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, contract.WorkspaceView{}, err
	}
	command, err := inspectWorkspaceReadCommandExactTxV1(ctx, tx, current.Command)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, contract.WorkspaceView{}, referencedWorkspaceReadCommandStorageErrorV2(err)
	}
	publication, err := inspectStoredWorkspaceReadCommandPublicationExactTxV2(
		ctx,
		tx,
		current.Publication,
	)
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, contract.WorkspaceView{}, referencedWorkspaceReadCommandStorageErrorV2(err)
	}
	history, err := inspectStoredWorkspaceReadCommandOwnerHistoryTxV2(
		ctx,
		tx,
		current.Meta.ID,
		current.Command,
	)
	if err != nil ||
		validateStoredWorkspaceReadCommandOwnerHistoryV2(
			command,
			publication,
			history,
			current,
		) != nil ||
		contract.ValidateWorkspaceReadCommandOwnerClosureV2(
			command,
			publication,
			current,
		) != nil ||
		current.ValidateCurrent(now) != nil ||
		command.ValidateCurrent(now) != nil ||
		current.Command != reservation.Command ||
		command.Meta.Ref() != reservation.Command ||
		command.WorkspaceView != reservation.WorkspaceView {
		return contract.WorkspaceReadCommandV1{}, contract.WorkspaceView{}, ports.ErrConflict
	}

	var workspaceRevision uint64
	var workspaceDigest string
	var workspaceBody []byte
	if err := tx.QueryRowContext(ctx, `SELECT revision,digest,body FROM workspace_view_history WHERE view_id=? AND revision=? AND digest=?`, reservation.WorkspaceView.ID, reservation.WorkspaceView.Revision, reservation.WorkspaceView.Digest).Scan(&workspaceRevision, &workspaceDigest, &workspaceBody); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceReadCommandV1{}, contract.WorkspaceView{}, ports.ErrNotFound
		}
		return contract.WorkspaceReadCommandV1{}, contract.WorkspaceView{}, err
	}
	if workspaceRevision != reservation.WorkspaceView.Revision || workspaceDigest != reservation.WorkspaceView.Digest {
		return contract.WorkspaceReadCommandV1{}, contract.WorkspaceView{}, ports.ErrConflict
	}
	var workspace contract.WorkspaceView
	if decode(workspaceBody, &workspace) != nil || workspace.ValidateCurrent(now) != nil || !contract.SameRef(workspace.Meta.Ref(), reservation.WorkspaceView) || workspace.FileScopeDigest != command.FileScopeDigest || !workspaceReadStoreAllowedV1(workspace, command.RelativePath) {
		return contract.WorkspaceReadCommandV1{}, contract.WorkspaceView{}, ports.ErrConflict
	}
	return command, workspace, nil
}

func workspaceReadStoreAllowedV1(workspace contract.WorkspaceView, relative string) bool {
	allowed := false
	for _, scope := range workspace.ReadScopes {
		if relative == scope || len(relative) > len(scope) && relative[:len(scope)+1] == scope+"/" {
			allowed = true
			break
		}
	}
	if !allowed {
		return false
	}
	for _, scope := range workspace.HiddenScopes {
		if relative == scope || len(relative) > len(scope) && relative[:len(scope)+1] == scope+"/" {
			return false
		}
	}
	return contract.ValidateLogicalPath(relative) == nil
}

func (s *Store) InspectBoundedWorkspaceReadV1(ctx context.Context, exact contract.WorkspaceReadAttemptRefV1) (contract.WorkspaceReadExecutionProjectionV1, error) {
	if err := exact.Validate(); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	var stable string
	var revision uint64
	var digest string
	if err := s.db.QueryRowContext(ctx, `SELECT stable_digest,revision,digest FROM workspace_read_attempt_origin WHERE attempt_id=?`, exact.ID).Scan(&stable, &revision, &digest); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrNotFound
		}
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	if revision != exact.Revision || digest != exact.Digest {
		return contract.WorkspaceReadExecutionProjectionV1{}, ports.ErrConflict
	}
	return s.inspectWorkspaceReadStableV1(ctx, stable)
}

func (s *Store) inspectWorkspaceReadStableV1(ctx context.Context, stable string) (contract.WorkspaceReadExecutionProjectionV1, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	defer tx.Rollback()
	return s.inspectWorkspaceReadTx(ctx, tx, stable)
}
func (s *Store) inspectWorkspaceReadTx(ctx context.Context, tx *sql.Tx, stable string) (contract.WorkspaceReadExecutionProjectionV1, error) {
	var rb, ab []byte
	if err := tx.QueryRowContext(ctx, `SELECT body FROM workspace_read_reservation WHERE stable_digest=?`, stable).Scan(&rb); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT body FROM workspace_read_attempt_current WHERE stable_digest=?`, stable).Scan(&ab); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, err
	}
	var p contract.WorkspaceReadExecutionProjectionV1
	if decode(rb, &p.Reservation) != nil || decode(ab, &p.Attempt) != nil {
		return p, errors.New("decode workspace read projection")
	}
	p.AdmissionReceipt = p.Attempt.AdmissionReceipt
	if p.Attempt.Observation != nil {
		var observationID string
		var observationStable string
		var ob []byte
		if err := tx.QueryRowContext(ctx, `SELECT observation_id,stable_digest,body FROM workspace_read_observation WHERE stable_digest=?`, stable).Scan(&observationID, &observationStable, &ob); err != nil {
			return p, err
		}
		var o contract.WorkspaceReadObservationV1
		if err := decode(ob, &o); err != nil {
			return p, err
		}
		if observationStable != stable ||
			o.Meta.ID != observationID ||
			*p.Attempt.Observation != o.Meta.Ref() ||
			o.AdmissionReceipt.StableKeyDigest != stable ||
			o.ProviderReceipt.StableKeyDigest != stable {
			return p, ports.ErrConflict
		}
		p.Observation = &o
		provider := o.ProviderReceipt
		p.ProviderReceipt = &provider
	}
	return p, p.ValidateShape()
}

var _ ports.WorkspaceReadOwnerStoreV1 = (*Store)(nil)

func minWorkspaceReadStoreExpiryV1(values ...int64) int64 {
	if len(values) == 0 {
		return 0
	}
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}
