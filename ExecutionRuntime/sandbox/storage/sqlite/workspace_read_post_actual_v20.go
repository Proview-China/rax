package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	ownerworkspaceread "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

var _ ownerworkspaceread.PostActualRepositoryV2 = (*Store)(nil)
var _ ports.WorkspaceReadTerminalExactReaderV2 = (*Store)(nil)
var _ ports.WorkspaceReadTerminalOriginReaderV2 = (*Store)(nil)

func (s *Store) EnsureAuthorizedExecutionQualificationV2(
	ctx context.Context,
	capability ownerworkspaceread.AuthorizedExecutionQualificationV2,
) (contract.WorkspaceReadExecutionQualificationV2, bool, error) {
	if ctx == nil || s == nil || s.db == nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, false, ports.ErrConflict
	}
	fact, err := capability.FactV2()
	if err != nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, false, ports.ErrConflict
	}
	for attempt := 0; attempt < 64; attempt++ {
		winner, created, err := s.ensureWorkspaceReadExecutionQualificationOnceV2(ctx, fact)
		if err == nil {
			return winner, created, nil
		}
		if !workspaceReadCommandPublicationSQLiteBusyV2(err) {
			return contract.WorkspaceReadExecutionQualificationV2{}, false, workspaceReadPostActualStorageErrorV20(err)
		}
		if waitErr := waitWorkspaceReadPostActualRetryV20(ctx, attempt); waitErr != nil {
			return contract.WorkspaceReadExecutionQualificationV2{}, false, waitErr
		}
	}
	return contract.WorkspaceReadExecutionQualificationV2{}, false,
		workspaceReadPostActualStorageErrorV20(errors.New("workspace read qualification retry limit reached"))
}

func (s *Store) ensureWorkspaceReadExecutionQualificationOnceV2(
	ctx context.Context,
	fact contract.WorkspaceReadExecutionQualificationV2,
) (contract.WorkspaceReadExecutionQualificationV2, bool, error) {
	mutationNow := s.clock()
	if mutationNow.IsZero() || mutationNow.UnixNano() < fact.S1CheckedUnixNano || mutationNow.UnixNano() >= fact.ExpiresUnixNano {
		return contract.WorkspaceReadExecutionQualificationV2{}, false, ports.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, false, err
	}
	defer tx.Rollback()
	binding, err := inspectWorkspaceReadAdmissionClosureForRuntimeAttemptTxV2(ctx, tx, fact.RuntimeAttempt)
	if err != nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, false, referencedWorkspaceReadPostActualStorageErrorV20(err)
	}
	if binding.Digest != fact.AdmissionAttemptBindingDigest ||
		binding.WorkspaceReadAttempt != fact.OriginAttempt ||
		binding.AuthorizationDigest != fact.AuthorizationDigest ||
		binding.Association != fact.Association ||
		binding.WorkspaceReadCommand.Meta.Ref() != fact.Command ||
		binding.AdmissionReceipt != fact.RuntimeAdmissionReceipt ||
		binding.AdmissionBinding.AdmissionReceipt.ID != fact.AdmissionReceipt.ID ||
		uint64(binding.AdmissionBinding.AdmissionReceipt.Revision) != fact.AdmissionReceipt.Revision ||
		string(binding.AdmissionBinding.AdmissionReceipt.Digest) != fact.AdmissionReceipt.Digest {
		return contract.WorkspaceReadExecutionQualificationV2{}, false, ports.ErrConflict
	}
	if err = validateWorkspaceReadQualificationDurableClosureV20(ctx, tx, fact, binding, mutationNow); err != nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, false, err
	}
	count, err := countWorkspaceReadQualificationCollisionsV20(ctx, tx, fact)
	if err != nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, false, err
	}
	if count != 0 {
		if count != 1 {
			return contract.WorkspaceReadExecutionQualificationV2{}, false, ports.ErrConflict
		}
		winner, inspectErr := inspectWorkspaceReadExecutionQualificationExactV20(ctx, tx, fact.Ref)
		if inspectErr != nil || !reflect.DeepEqual(winner, fact) {
			return contract.WorkspaceReadExecutionQualificationV2{}, false, ports.ErrConflict
		}
		return winner, false, nil
	}
	body, err := encode(fact)
	if err != nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, false, err
	}
	rowDigest, err := workspaceReadQualificationRowDigestV20(fact, body)
	if err != nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, false, err
	}
	precommitNow := s.clock()
	if precommitNow.IsZero() || precommitNow.Before(mutationNow) || precommitNow.UnixNano() >= fact.ExpiresUnixNano {
		return contract.WorkspaceReadExecutionQualificationV2{}, false, ports.ErrConflict
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO workspace_read_execution_qualification_history_v2(
		qualification_id,revision,digest,expires_unix_nano,
		origin_attempt_id,origin_attempt_revision,origin_attempt_digest,
		reservation_id,reservation_revision,reservation_digest,
		admission_id,admission_revision,admission_digest,
		runtime_admission_id,runtime_admission_revision,runtime_admission_digest,
		runtime_attempt_digest,admission_attempt_binding_digest,authorization_digest,
		association_id,association_revision,association_digest,
		command_id,command_revision,command_digest,
		publication_id,publication_revision,publication_digest,
		owner_current_id,owner_current_revision,owner_current_digest,
		workspace_view_id,workspace_view_revision,workspace_view_digest,
		workspace_lease_digest,
		current_query_digest,expected_runtime_current_digest,actual_request_digest,payload_digest,
		s1_checked_unix_nano,body,row_digest)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		fact.Ref.ID, fact.Ref.Revision, fact.Ref.Digest, fact.Ref.ExpiresUnixNano,
		fact.OriginAttempt.ID, fact.OriginAttempt.Revision, fact.OriginAttempt.Digest,
		fact.Reservation.ID, fact.Reservation.Revision, fact.Reservation.Digest,
		fact.AdmissionReceipt.ID, fact.AdmissionReceipt.Revision, fact.AdmissionReceipt.Digest,
		fact.RuntimeAdmissionReceipt.ID, fact.RuntimeAdmissionReceipt.Revision, fact.RuntimeAdmissionReceipt.Digest,
		fact.RuntimeAttemptDigest, fact.AdmissionAttemptBindingDigest, fact.AuthorizationDigest,
		fact.Association.ID, fact.Association.Revision, fact.Association.Digest,
		fact.Command.ID, fact.Command.Revision, fact.Command.Digest,
		fact.CommandPublication.ID, fact.CommandPublication.Revision, fact.CommandPublication.Digest,
		fact.CommandOwnerCurrent.ID, fact.CommandOwnerCurrent.Revision, fact.CommandOwnerCurrent.Digest,
		fact.WorkspaceView.ID, fact.WorkspaceView.Revision, fact.WorkspaceView.Digest,
		fact.WorkspaceLeaseDigest,
		fact.CurrentQueryDigest, fact.ExpectedRuntimeCurrentDigest, fact.ActualRequestDigest, fact.PayloadDigest,
		fact.S1CheckedUnixNano, body, rowDigest,
	)
	if err != nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, false, classifyWrite(err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		if err != nil {
			return contract.WorkspaceReadExecutionQualificationV2{}, false, err
		}
		return contract.WorkspaceReadExecutionQualificationV2{}, false, ports.ErrConflict
	}
	commitNow := s.clock()
	if commitNow.IsZero() || commitNow.Before(precommitNow) || commitNow.UnixNano() >= fact.ExpiresUnixNano {
		return contract.WorkspaceReadExecutionQualificationV2{}, false, ports.ErrConflict
	}
	if err = tx.Commit(); err != nil {
		winner, inspectErr := s.InspectWorkspaceReadExecutionQualificationExactV2(ctx, fact.Ref)
		if inspectErr == nil && reflect.DeepEqual(winner, fact) {
			return winner, false, nil
		}
		return contract.WorkspaceReadExecutionQualificationV2{}, false,
			fmt.Errorf("%w: commit workspace read execution qualification: %v", ports.ErrUnknownOutcome, err)
	}
	return fact, true, nil
}

func validateWorkspaceReadQualificationDurableClosureV20(
	ctx context.Context,
	tx *sql.Tx,
	fact contract.WorkspaceReadExecutionQualificationV2,
	binding ports.WorkspaceReadAdmissionAttemptBindingV2,
	now time.Time,
) error {
	reservation, origin, err := inspectWorkspaceReadQualificationAdmissionCurrentClosureV20(ctx, tx, fact, binding, now)
	if err != nil {
		return err
	}
	publication, err := inspectStoredWorkspaceReadCommandPublicationExactTxV2(ctx, tx, fact.CommandPublication)
	if err != nil {
		return referencedWorkspaceReadPostActualStorageErrorV20(err)
	}
	current, err := inspectStoredWorkspaceReadCommandOwnerCurrentExactTxV2(ctx, tx, fact.CommandOwnerCurrent)
	if err != nil {
		return referencedWorkspaceReadPostActualStorageErrorV20(err)
	}
	workspace, err := inspectWorkspaceReadViewExactV20(ctx, tx, fact.WorkspaceView)
	if err != nil {
		return referencedWorkspaceReadPostActualStorageErrorV20(err)
	}
	leaseDigest, err := contract.WorkspaceReadRuntimeLeaseDigestV2(workspace.Lease)
	if err != nil {
		return ports.ErrConflict
	}
	if publication.Meta.Ref() != fact.CommandPublication ||
		publication.Command != fact.Command ||
		publication.Semantic.RuntimeAttemptDigest != fact.RuntimeAttemptDigest ||
		publication.Semantic.Workspace.Meta.Ref() != fact.WorkspaceView ||
		!reflect.DeepEqual(publication.Semantic.Workspace, workspace) ||
		current.Meta.Ref() != fact.CommandOwnerCurrent ||
		current.Command != fact.Command ||
		current.Publication != fact.CommandPublication ||
		current.WorkspaceView != fact.WorkspaceView ||
		current.PublicationSemanticDigest != publication.Semantic.Digest ||
		workspace.Meta.Ref() != fact.WorkspaceView ||
		binding.WorkspaceReadCommand.WorkspaceView != fact.WorkspaceView ||
		fact.WorkspaceLeaseDigest != leaseDigest ||
		fact.S1CheckedUnixNano < reservation.Meta.UpdatedUnixNano ||
		fact.S1CheckedUnixNano < origin.Meta.UpdatedUnixNano ||
		fact.S1CheckedUnixNano < binding.AdmissionBinding.CreatedUnixNano ||
		fact.S1CheckedUnixNano < fact.AdmissionReceipt.CheckedUnixNano ||
		fact.S1CheckedUnixNano < binding.WorkspaceReadCommand.Meta.UpdatedUnixNano ||
		fact.S1CheckedUnixNano < publication.Meta.UpdatedUnixNano ||
		fact.S1CheckedUnixNano < current.CheckedUnixNano ||
		fact.S1CheckedUnixNano < current.Meta.UpdatedUnixNano ||
		fact.S1CheckedUnixNano < workspace.Meta.UpdatedUnixNano ||
		current.ValidateCurrent(now) != nil ||
		workspace.ValidateCurrent(now) != nil ||
		publication.Meta.ValidateCurrent(now) != nil ||
		fact.ExpiresUnixNano > current.ExpiresUnixNano ||
		fact.ExpiresUnixNano > publication.Meta.ExpiresUnixNano ||
		fact.ExpiresUnixNano > workspace.Meta.ExpiresUnixNano ||
		fact.ExpiresUnixNano > workspace.Lease.ExpiresUnixNano ||
		fact.ExpiresUnixNano > reservation.Meta.ExpiresUnixNano ||
		fact.ExpiresUnixNano > origin.Meta.ExpiresUnixNano ||
		fact.ExpiresUnixNano > binding.AdmissionBinding.ExpiresUnixNano ||
		fact.ExpiresUnixNano > fact.AdmissionReceipt.ExpiresUnixNano {
		return ports.ErrConflict
	}
	return nil
}

func inspectWorkspaceReadQualificationAdmissionCurrentClosureV20(
	ctx context.Context,
	tx *sql.Tx,
	fact contract.WorkspaceReadExecutionQualificationV2,
	binding ports.WorkspaceReadAdmissionAttemptBindingV2,
	now time.Time,
) (contract.WorkspaceReadReservationV1, contract.WorkspaceReadAttemptV1, error) {
	var reservationBody []byte
	if err := tx.QueryRowContext(ctx,
		`SELECT body FROM workspace_read_reservation WHERE stable_digest=?`,
		binding.AdmissionBinding.StableKeyDigest,
	).Scan(&reservationBody); err != nil {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, referencedWorkspaceReadPostActualStorageErrorV20(err)
	}
	var reservation contract.WorkspaceReadReservationV1
	if err := decode(reservationBody, &reservation); err != nil {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, ports.ErrConflict
	}
	canonicalReservation, err := encode(reservation)
	if err != nil || !bytes.Equal(canonicalReservation, reservationBody) ||
		reservation.ValidateCurrent(now) != nil ||
		reservation.Meta.Ref() != fact.Reservation ||
		reservation.Command != fact.Command ||
		reservation.WorkspaceView != fact.WorkspaceView ||
		reservation.AttemptID != fact.OriginAttempt.ID {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, ports.ErrConflict
	}
	var originRevision uint64
	var originDigest string
	var originBody []byte
	if err = tx.QueryRowContext(ctx,
		`SELECT revision,digest,body FROM workspace_read_attempt_origin WHERE attempt_id=?`,
		fact.OriginAttempt.ID,
	).Scan(&originRevision, &originDigest, &originBody); err != nil {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, referencedWorkspaceReadPostActualStorageErrorV20(err)
	}
	var origin contract.WorkspaceReadAttemptV1
	if err = decode(originBody, &origin); err != nil {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, ports.ErrConflict
	}
	canonicalOrigin, err := encode(origin)
	if err != nil || !bytes.Equal(canonicalOrigin, originBody) ||
		origin.ValidateCurrent(now) != nil ||
		origin.Meta.Revision != originRevision || origin.Meta.Digest != originDigest ||
		origin.Meta.Ref() != fact.OriginAttempt.OwnerRef() ||
		origin.Reservation != fact.Reservation ||
		origin.AdmissionReceipt != fact.AdmissionReceipt ||
		origin.State != contract.WorkspaceReadStartedV1 {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, ports.ErrConflict
	}
	if binding.AdmissionBinding.Validate() != nil ||
		binding.AdmissionBinding.Attempt != fact.OriginAttempt ||
		binding.AdmissionBinding.Command != fact.Command ||
		binding.AdmissionBinding.ExpiresUnixNano <= now.UnixNano() {
		return contract.WorkspaceReadReservationV1{}, contract.WorkspaceReadAttemptV1{}, ports.ErrConflict
	}
	return reservation, origin, nil
}

func inspectWorkspaceReadViewExactV20(ctx context.Context, source queryer, exact contract.Ref) (contract.WorkspaceView, error) {
	if err := exact.ValidateShape("workspace read Workspace View"); err != nil {
		return contract.WorkspaceView{}, ports.ErrConflict
	}
	var revision uint64
	var digest string
	var body []byte
	if err := source.QueryRowContext(ctx,
		`SELECT revision,digest,body FROM workspace_view_history WHERE view_id=? AND revision=?`,
		exact.ID, exact.Revision,
	).Scan(&revision, &digest, &body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceView{}, ports.ErrNotFound
		}
		return contract.WorkspaceView{}, workspaceReadPostActualStorageErrorV20(err)
	}
	var workspace contract.WorkspaceView
	if err := decode(body, &workspace); err != nil {
		return contract.WorkspaceView{}, ports.ErrConflict
	}
	canonical, err := encode(workspace)
	if err != nil || !bytes.Equal(canonical, body) || workspace.ValidateShape() != nil ||
		workspace.Meta.Ref() != exact || revision != exact.Revision || digest != exact.Digest {
		return contract.WorkspaceView{}, ports.ErrConflict
	}
	return workspace, nil
}

func (s *Store) InspectWorkspaceReadExecutionQualificationExactV2(
	ctx context.Context,
	exact contract.WorkspaceReadExecutionQualificationRefV2,
) (contract.WorkspaceReadExecutionQualificationV2, error) {
	if ctx == nil || s == nil || s.db == nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, ports.ErrConflict
	}
	return inspectWorkspaceReadExecutionQualificationExactV20(ctx, s.db, exact)
}

func inspectWorkspaceReadExecutionQualificationExactV20(
	ctx context.Context,
	source queryer,
	exact contract.WorkspaceReadExecutionQualificationRefV2,
) (contract.WorkspaceReadExecutionQualificationV2, error) {
	if err := exact.Validate(); err != nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, ports.ErrConflict
	}
	var row workspaceReadQualificationRowV20
	if err := source.QueryRowContext(ctx, workspaceReadQualificationSelectV20+` WHERE qualification_id=?`, exact.ID).Scan(row.scanTargets()...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceReadExecutionQualificationV2{}, ports.ErrNotFound
		}
		return contract.WorkspaceReadExecutionQualificationV2{}, workspaceReadPostActualStorageErrorV20(err)
	}
	fact, err := row.factV20()
	if err != nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, err
	}
	if fact.Ref != exact {
		return contract.WorkspaceReadExecutionQualificationV2{}, ports.ErrConflict
	}
	return fact, nil
}

// InspectWorkspaceReadExecutionQualificationByOriginV2 is the Sandbox Owner's
// recovery lookup. It accepts only the full immutable origin coordinate; it is
// not a stable-key or caller-selected current lookup.
func (s *Store) InspectWorkspaceReadExecutionQualificationByOriginV2(
	ctx context.Context,
	origin contract.WorkspaceReadAttemptRefV1,
) (contract.WorkspaceReadExecutionQualificationV2, error) {
	if ctx == nil || s == nil || s.db == nil || origin.Validate() != nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, ports.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, workspaceReadPostActualStorageErrorV20(err)
	}
	defer tx.Rollback()
	var row workspaceReadQualificationRowV20
	if err := tx.QueryRowContext(ctx, workspaceReadQualificationSelectV20+
		` WHERE origin_attempt_id=? AND origin_attempt_revision=? AND origin_attempt_digest=?`,
		origin.ID, origin.Revision, origin.Digest,
	).Scan(row.scanTargets()...); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceReadExecutionQualificationV2{}, workspaceReadPostActualStorageErrorV20(err)
		}
		var sameID int
		if countErr := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM workspace_read_execution_qualification_history_v2 WHERE origin_attempt_id=?`,
			origin.ID,
		).Scan(&sameID); countErr != nil {
			return contract.WorkspaceReadExecutionQualificationV2{}, workspaceReadPostActualStorageErrorV20(countErr)
		}
		if sameID != 0 {
			return contract.WorkspaceReadExecutionQualificationV2{}, ports.ErrConflict
		}
		return contract.WorkspaceReadExecutionQualificationV2{}, ports.ErrNotFound
	}
	fact, err := row.factV20()
	if err != nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, err
	}
	if fact.OriginAttempt != origin {
		return contract.WorkspaceReadExecutionQualificationV2{}, ports.ErrConflict
	}
	return fact, nil
}

func (s *Store) CreateOrInspectKernelTerminalV2(
	ctx context.Context,
	capability kernel.WorkspaceReadAuthorizedTerminalV2,
) (contract.WorkspaceReadTerminalFactV2, bool, error) {
	if ctx == nil || s == nil || s.db == nil {
		return contract.WorkspaceReadTerminalFactV2{}, false, ports.ErrConflict
	}
	fact, err := capability.FactV2()
	if err != nil {
		return contract.WorkspaceReadTerminalFactV2{}, false, ports.ErrConflict
	}
	for attempt := 0; attempt < 64; attempt++ {
		winner, created, err := s.createOrInspectWorkspaceReadTerminalOnceV2(ctx, fact)
		if err == nil {
			return winner, created, nil
		}
		if !workspaceReadCommandPublicationSQLiteBusyV2(err) {
			return contract.WorkspaceReadTerminalFactV2{}, false, workspaceReadPostActualStorageErrorV20(err)
		}
		if waitErr := waitWorkspaceReadPostActualRetryV20(ctx, attempt); waitErr != nil {
			return contract.WorkspaceReadTerminalFactV2{}, false, waitErr
		}
	}
	return contract.WorkspaceReadTerminalFactV2{}, false,
		workspaceReadPostActualStorageErrorV20(errors.New("workspace read terminal retry limit reached"))
}

func (s *Store) createOrInspectWorkspaceReadTerminalOnceV2(
	ctx context.Context,
	fact contract.WorkspaceReadTerminalFactV2,
) (contract.WorkspaceReadTerminalFactV2, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.WorkspaceReadTerminalFactV2{}, false, err
	}
	defer tx.Rollback()
	qualification, err := inspectWorkspaceReadExecutionQualificationExactV20(ctx, tx, fact.Qualification)
	if err != nil {
		return contract.WorkspaceReadTerminalFactV2{}, false, referencedWorkspaceReadPostActualStorageErrorV20(err)
	}
	if qualification.OriginAttempt != fact.OriginAttempt ||
		qualification.RuntimeAttemptDigest != fact.RuntimeAttemptDigest ||
		!reflect.DeepEqual(qualification.RuntimeAttempt, fact.RuntimeAttempt) ||
		qualification.ActualRequestDigest != fact.ActualRequestDigest ||
		qualification.PayloadDigest != fact.Journal.PayloadDigest ||
		qualification.ExpiresUnixNano != fact.QualificationExpiresUnixNano {
		return contract.WorkspaceReadTerminalFactV2{}, false, ports.ErrConflict
	}
	if fact.Observed != nil {
		if validateErr := validateWorkspaceReadObservedTerminalSourcesV20(ctx, tx, fact, qualification); validateErr != nil {
			return contract.WorkspaceReadTerminalFactV2{}, false, ports.ErrConflict
		}
	}
	count, err := countWorkspaceReadTerminalCollisionsV20(ctx, tx, fact)
	if err != nil {
		return contract.WorkspaceReadTerminalFactV2{}, false, err
	}
	if count != 0 {
		if count != 1 {
			return contract.WorkspaceReadTerminalFactV2{}, false, ports.ErrConflict
		}
		winner, inspectErr := inspectWorkspaceReadTerminalExactV20(ctx, tx, fact.Ref)
		if inspectErr != nil || !reflect.DeepEqual(winner, fact) {
			return contract.WorkspaceReadTerminalFactV2{}, false, ports.ErrConflict
		}
		return winner, false, nil
	}
	body, err := encode(fact)
	if err != nil {
		return contract.WorkspaceReadTerminalFactV2{}, false, err
	}
	rowDigest, err := workspaceReadTerminalRowDigestV20(fact, body)
	if err != nil {
		return contract.WorkspaceReadTerminalFactV2{}, false, err
	}
	observationID, observationDigest, providerID, providerDigest, s2Proof := "", "", "", "", ""
	indeterminateBoundary, indeterminateStage, indeterminateClass := "", "", ""
	indeterminateError, indeterminateEvidence, indeterminateFact := "", "", ""
	var s2Checked int64
	var observationRevision, providerRevision uint64
	if fact.Observed != nil {
		proof := fact.Observed.S2Proof
		observationID, observationRevision, observationDigest = proof.Observation.ID, proof.Observation.Revision, proof.Observation.Digest
		providerID, providerRevision, providerDigest = proof.ProviderReceipt.ID, proof.ProviderReceipt.Revision, proof.ProviderReceipt.Digest
		s2Proof = proof.Digest
		s2Checked = proof.S2CheckedUnixNano
	} else {
		evidence := fact.Indeterminate.Evidence
		indeterminateBoundary = evidence.Boundary
		indeterminateStage = string(evidence.Stage)
		indeterminateClass = string(evidence.ErrorClass)
		indeterminateError = evidence.ErrorDigest
		indeterminateEvidence = evidence.Digest
		indeterminateFact = fact.Ref.Digest
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO workspace_read_terminal_history_v2(
		terminal_id,revision,digest,qualification_id,qualification_revision,qualification_digest,
		qualification_expires_unix_nano,origin_attempt_id,origin_attempt_revision,origin_attempt_digest,
		runtime_attempt_digest,actual_request_digest,
		journal_attempt_id,journal_request_digest,journal_payload_digest,journal_phase,journal_state,
		journal_revision,journal_recorded_unix_nano,journal_record_digest,outcome,
		observation_id,observation_revision,observation_digest,
		provider_receipt_id,provider_receipt_revision,provider_receipt_digest,s2_proof_digest,
		s2_checked_unix_nano,indeterminate_boundary,indeterminate_stage,indeterminate_error_class,
		indeterminate_error_digest,indeterminate_evidence_digest,indeterminate_fact_digest,
		outcome_checked_unix_nano,recorded_unix_nano,body,row_digest)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		fact.Ref.ID, fact.Ref.Revision, fact.Ref.Digest,
		fact.Qualification.ID, fact.Qualification.Revision, fact.Qualification.Digest, fact.Qualification.ExpiresUnixNano,
		fact.OriginAttempt.ID, fact.OriginAttempt.Revision, fact.OriginAttempt.Digest,
		fact.RuntimeAttemptDigest, fact.ActualRequestDigest,
		fact.Journal.AttemptID, fact.Journal.RequestDigest, fact.Journal.PayloadDigest,
		fact.Journal.Phase, fact.Journal.State, fact.Journal.Revision, fact.Journal.RecordedUnixNano, fact.Journal.RecordDigest,
		fact.Outcome, observationID, observationRevision, observationDigest,
		providerID, providerRevision, providerDigest, s2Proof, s2Checked,
		indeterminateBoundary, indeterminateStage, indeterminateClass,
		indeterminateError, indeterminateEvidence, indeterminateFact,
		fact.OutcomeCheckedUnixNano, fact.RecordedUnixNano, body, rowDigest,
	)
	if err != nil {
		return contract.WorkspaceReadTerminalFactV2{}, false, classifyWrite(err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		if err != nil {
			return contract.WorkspaceReadTerminalFactV2{}, false, err
		}
		return contract.WorkspaceReadTerminalFactV2{}, false, ports.ErrConflict
	}
	if err = tx.Commit(); err != nil {
		winner, inspectErr := s.InspectWorkspaceReadTerminalExactV2(ctx, fact.Ref)
		if inspectErr == nil && reflect.DeepEqual(winner, fact) {
			return winner, false, nil
		}
		return contract.WorkspaceReadTerminalFactV2{}, false,
			fmt.Errorf("%w: commit workspace read terminal fact: %v", ports.ErrUnknownOutcome, err)
	}
	return fact, true, nil
}

func validateWorkspaceReadObservedTerminalSourcesV20(
	ctx context.Context,
	tx *sql.Tx,
	fact contract.WorkspaceReadTerminalFactV2,
	qualification contract.WorkspaceReadExecutionQualificationV2,
) error {
	proof := fact.Observed.S2Proof
	if err := proof.ValidateQualificationV2(qualification); err != nil {
		return ports.ErrConflict
	}
	publication, err := inspectStoredWorkspaceReadCommandPublicationExactTxV2(ctx, tx, proof.CommandPublication)
	if err != nil {
		return referencedWorkspaceReadPostActualStorageErrorV20(err)
	}
	current, err := inspectStoredWorkspaceReadCommandOwnerCurrentExactTxV2(ctx, tx, proof.CommandOwnerCurrent)
	if err != nil {
		return referencedWorkspaceReadPostActualStorageErrorV20(err)
	}
	workspace, err := inspectWorkspaceReadViewExactV20(ctx, tx, proof.WorkspaceView)
	if err != nil {
		return referencedWorkspaceReadPostActualStorageErrorV20(err)
	}
	leaseDigest, err := contract.WorkspaceReadRuntimeLeaseDigestV2(workspace.Lease)
	if err != nil {
		return ports.ErrConflict
	}
	observation, err := inspectWorkspaceReadObservationExactV20(ctx, tx, proof.Observation)
	if err != nil {
		return referencedWorkspaceReadPostActualStorageErrorV20(err)
	}
	if publication.Meta.Ref() != proof.CommandPublication ||
		publication.Command != qualification.Command ||
		publication.Semantic.Workspace.Meta.Ref() != proof.WorkspaceView ||
		!reflect.DeepEqual(publication.Semantic.Workspace, workspace) ||
		current.Meta.Ref() != proof.CommandOwnerCurrent ||
		current.Command != qualification.Command ||
		current.Publication != proof.CommandPublication ||
		current.WorkspaceView != proof.WorkspaceView ||
		workspace.Meta.Ref() != proof.WorkspaceView ||
		leaseDigest != proof.WorkspaceLeaseDigest ||
		proof.Association != qualification.Association ||
		proof.RuntimeCurrentDigest != qualification.ExpectedRuntimeCurrentDigest ||
		observation.Meta.Ref() != proof.Observation ||
		observation.ProviderReceipt != proof.ProviderReceipt ||
		observation.Reservation != qualification.Reservation ||
		observation.Command != qualification.Command ||
		observation.WorkspaceView != qualification.WorkspaceView {
		return ports.ErrConflict
	}
	return nil
}

func (s *Store) InspectWorkspaceReadTerminalExactV2(
	ctx context.Context,
	exact contract.WorkspaceReadTerminalRefV2,
) (contract.WorkspaceReadTerminalFactV2, error) {
	if ctx == nil || s == nil || s.db == nil {
		return contract.WorkspaceReadTerminalFactV2{}, ports.ErrConflict
	}
	return inspectWorkspaceReadTerminalExactV20(ctx, s.db, exact)
}

func inspectWorkspaceReadTerminalExactV20(ctx context.Context, source queryer, exact contract.WorkspaceReadTerminalRefV2) (contract.WorkspaceReadTerminalFactV2, error) {
	if err := exact.Validate(); err != nil {
		return contract.WorkspaceReadTerminalFactV2{}, ports.ErrConflict
	}
	var row workspaceReadTerminalRowV20
	if err := source.QueryRowContext(ctx, workspaceReadTerminalSelectV20+` WHERE terminal_id=?`, exact.ID).Scan(row.scanTargets()...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceReadTerminalFactV2{}, ports.ErrNotFound
		}
		return contract.WorkspaceReadTerminalFactV2{}, workspaceReadPostActualStorageErrorV20(err)
	}
	fact, err := row.factV20()
	if err != nil || fact.Ref != exact {
		return contract.WorkspaceReadTerminalFactV2{}, ports.ErrConflict
	}
	return fact, nil
}

func (s *Store) InspectWorkspaceReadTerminalByOriginV2(ctx context.Context, origin contract.WorkspaceReadAttemptRefV1) (contract.WorkspaceReadTerminalFactV2, error) {
	if ctx == nil || s == nil || s.db == nil || origin.Validate() != nil {
		return contract.WorkspaceReadTerminalFactV2{}, ports.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return contract.WorkspaceReadTerminalFactV2{}, workspaceReadPostActualStorageErrorV20(err)
	}
	defer tx.Rollback()
	var row workspaceReadTerminalRowV20
	if err := tx.QueryRowContext(ctx, workspaceReadTerminalSelectV20+` WHERE origin_attempt_id=? AND origin_attempt_revision=? AND origin_attempt_digest=?`, origin.ID, origin.Revision, origin.Digest).Scan(row.scanTargets()...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			var sameID int
			if countErr := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_read_terminal_history_v2 WHERE origin_attempt_id=?`, origin.ID).Scan(&sameID); countErr != nil {
				return contract.WorkspaceReadTerminalFactV2{}, workspaceReadPostActualStorageErrorV20(countErr)
			}
			if sameID != 0 {
				return contract.WorkspaceReadTerminalFactV2{}, ports.ErrConflict
			}
			return contract.WorkspaceReadTerminalFactV2{}, ports.ErrNotFound
		}
		return contract.WorkspaceReadTerminalFactV2{}, workspaceReadPostActualStorageErrorV20(err)
	}
	fact, err := row.factV20()
	if err != nil || fact.OriginAttempt != origin {
		return contract.WorkspaceReadTerminalFactV2{}, ports.ErrConflict
	}
	return fact, nil
}

func countWorkspaceReadQualificationCollisionsV20(ctx context.Context, tx *sql.Tx, fact contract.WorkspaceReadExecutionQualificationV2) (int, error) {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_read_execution_qualification_history_v2
		WHERE qualification_id=?
		   OR (origin_attempt_id=? AND origin_attempt_revision=? AND origin_attempt_digest=?)
		   OR runtime_attempt_digest=? OR current_query_digest=? OR actual_request_digest=?`,
		fact.Ref.ID, fact.OriginAttempt.ID, fact.OriginAttempt.Revision, fact.OriginAttempt.Digest,
		fact.RuntimeAttemptDigest, fact.CurrentQueryDigest, fact.ActualRequestDigest,
	).Scan(&count)
	return count, err
}

func countWorkspaceReadTerminalCollisionsV20(ctx context.Context, tx *sql.Tx, fact contract.WorkspaceReadTerminalFactV2) (int, error) {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_read_terminal_history_v2
		WHERE terminal_id=?
		   OR (qualification_id=? AND qualification_revision=? AND qualification_digest=?)
		   OR (origin_attempt_id=? AND origin_attempt_revision=? AND origin_attempt_digest=?)
		   OR journal_record_digest=?`,
		fact.Ref.ID, fact.Qualification.ID, fact.Qualification.Revision, fact.Qualification.Digest,
		fact.OriginAttempt.ID, fact.OriginAttempt.Revision, fact.OriginAttempt.Digest, fact.Journal.RecordDigest,
	).Scan(&count)
	return count, err
}

func inspectWorkspaceReadObservationExactV20(ctx context.Context, source queryer, exact contract.Ref) (contract.WorkspaceReadObservationV1, error) {
	if err := exact.ValidateShape("workspace read observation"); err != nil {
		return contract.WorkspaceReadObservationV1{}, ports.ErrConflict
	}
	var body []byte
	if err := source.QueryRowContext(ctx, `SELECT body FROM workspace_read_observation WHERE observation_id=?`, exact.ID).Scan(&body); err != nil {
		return contract.WorkspaceReadObservationV1{}, referencedWorkspaceReadPostActualStorageErrorV20(err)
	}
	var observation contract.WorkspaceReadObservationV1
	if err := decode(body, &observation); err != nil {
		return contract.WorkspaceReadObservationV1{}, ports.ErrConflict
	}
	canonical, err := encode(observation)
	if err != nil || !bytes.Equal(canonical, body) || observation.ValidateShape() != nil || observation.Meta.Ref() != exact {
		return contract.WorkspaceReadObservationV1{}, ports.ErrConflict
	}
	return observation, nil
}

func workspaceReadQualificationRowDigestV20(fact contract.WorkspaceReadExecutionQualificationV2, body []byte) (string, error) {
	return contract.Digest("workspace-read-execution-qualification-row-v2", struct {
		Ref                  contract.WorkspaceReadExecutionQualificationRefV2 `json:"ref"`
		OriginAttempt        contract.WorkspaceReadAttemptRefV1                `json:"origin_attempt"`
		RuntimeAttemptDigest string                                            `json:"runtime_attempt_digest"`
		CurrentQueryDigest   string                                            `json:"current_query_digest"`
		ActualRequestDigest  string                                            `json:"actual_request_digest"`
		CanonicalBody        string                                            `json:"canonical_body"`
	}{fact.Ref, fact.OriginAttempt, string(fact.RuntimeAttemptDigest), fact.CurrentQueryDigest, fact.ActualRequestDigest, string(body)})
}

func workspaceReadTerminalRowDigestV20(fact contract.WorkspaceReadTerminalFactV2, body []byte) (string, error) {
	return contract.Digest("workspace-read-terminal-row-v2", struct {
		Ref           contract.WorkspaceReadTerminalRefV2               `json:"ref"`
		Qualification contract.WorkspaceReadExecutionQualificationRefV2 `json:"qualification"`
		OriginAttempt contract.WorkspaceReadAttemptRefV1                `json:"origin_attempt"`
		JournalDigest string                                            `json:"journal_digest"`
		CanonicalBody string                                            `json:"canonical_body"`
	}{fact.Ref, fact.Qualification, fact.OriginAttempt, fact.Journal.RecordDigest, string(body)})
}

func waitWorkspaceReadPostActualRetryV20(ctx context.Context, attempt int) error {
	delay := time.Duration(attempt+1) * time.Millisecond
	if delay > 10*time.Millisecond {
		delay = 10 * time.Millisecond
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func referencedWorkspaceReadPostActualStorageErrorV20(err error) error {
	if errors.Is(err, ports.ErrNotFound) {
		return ports.ErrConflict
	}
	return workspaceReadPostActualStorageErrorV20(err)
}

func workspaceReadPostActualStorageErrorV20(err error) error {
	return workspaceReadCommandPublicationStorageErrorV2(err)
}

const workspaceReadQualificationSelectV20 = `SELECT
	qualification_id,revision,digest,expires_unix_nano,
	origin_attempt_id,origin_attempt_revision,origin_attempt_digest,
	reservation_id,reservation_revision,reservation_digest,
	admission_id,admission_revision,admission_digest,
	runtime_admission_id,runtime_admission_revision,runtime_admission_digest,
	runtime_attempt_digest,admission_attempt_binding_digest,authorization_digest,
	association_id,association_revision,association_digest,
	command_id,command_revision,command_digest,
	publication_id,publication_revision,publication_digest,
	owner_current_id,owner_current_revision,owner_current_digest,
	workspace_view_id,workspace_view_revision,workspace_view_digest,
	workspace_lease_digest,
	current_query_digest,expected_runtime_current_digest,actual_request_digest,payload_digest,
	s1_checked_unix_nano,body,row_digest
	FROM workspace_read_execution_qualification_history_v2`

type workspaceReadQualificationRowV20 struct {
	qualificationID, digest                                                              string
	revision                                                                             uint64
	expires                                                                              int64
	originID, originDigest                                                               string
	originRevision                                                                       uint64
	reservationID, reservationDigest                                                     string
	reservationRevision                                                                  uint64
	admissionID, admissionDigest                                                         string
	admissionRevision                                                                    uint64
	runtimeAdmissionID, runtimeAdmissionDigest                                           string
	runtimeAdmissionRevision                                                             uint64
	runtimeAttemptDigest, bindingDigest, authorizationDigest                             string
	associationID, associationDigest                                                     string
	associationRevision                                                                  uint64
	commandID, commandDigest                                                             string
	commandRevision                                                                      uint64
	publicationID, publicationDigest                                                     string
	publicationRevision                                                                  uint64
	ownerCurrentID, ownerCurrentDigest                                                   string
	ownerCurrentRevision                                                                 uint64
	workspaceViewID, workspaceViewDigest                                                 string
	workspaceViewRevision                                                                uint64
	workspaceLeaseDigest                                                                 string
	currentQueryDigest, expectedRuntimeCurrentDigest, actualRequestDigest, payloadDigest string
	s1Checked                                                                            int64
	body                                                                                 []byte
	rowDigest                                                                            string
}

func (r *workspaceReadQualificationRowV20) scanTargets() []any {
	return []any{&r.qualificationID, &r.revision, &r.digest, &r.expires,
		&r.originID, &r.originRevision, &r.originDigest,
		&r.reservationID, &r.reservationRevision, &r.reservationDigest,
		&r.admissionID, &r.admissionRevision, &r.admissionDigest,
		&r.runtimeAdmissionID, &r.runtimeAdmissionRevision, &r.runtimeAdmissionDigest,
		&r.runtimeAttemptDigest, &r.bindingDigest, &r.authorizationDigest,
		&r.associationID, &r.associationRevision, &r.associationDigest,
		&r.commandID, &r.commandRevision, &r.commandDigest,
		&r.publicationID, &r.publicationRevision, &r.publicationDigest,
		&r.ownerCurrentID, &r.ownerCurrentRevision, &r.ownerCurrentDigest,
		&r.workspaceViewID, &r.workspaceViewRevision, &r.workspaceViewDigest,
		&r.workspaceLeaseDigest,
		&r.currentQueryDigest, &r.expectedRuntimeCurrentDigest, &r.actualRequestDigest, &r.payloadDigest,
		&r.s1Checked, &r.body, &r.rowDigest}
}

func (r workspaceReadQualificationRowV20) factV20() (contract.WorkspaceReadExecutionQualificationV2, error) {
	var fact contract.WorkspaceReadExecutionQualificationV2
	if err := decode(r.body, &fact); err != nil {
		return fact, fmt.Errorf("%w: decode workspace read Qualification body", ports.ErrConflict)
	}
	canonical, err := encode(fact)
	if err != nil || !bytes.Equal(canonical, r.body) {
		return contract.WorkspaceReadExecutionQualificationV2{}, fmt.Errorf("%w: workspace read Qualification body is not canonical", ports.ErrConflict)
	}
	if err = fact.Validate(); err != nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, fmt.Errorf("%w: validate workspace read Qualification body: %v", ports.ErrConflict, err)
	}
	if fact.Ref.ID != r.qualificationID || fact.Ref.Revision != r.revision || fact.Ref.Digest != r.digest || fact.Ref.ExpiresUnixNano != r.expires ||
		fact.OriginAttempt.ID != r.originID || fact.OriginAttempt.Revision != r.originRevision || fact.OriginAttempt.Digest != r.originDigest ||
		fact.Reservation.ID != r.reservationID || fact.Reservation.Revision != r.reservationRevision || fact.Reservation.Digest != r.reservationDigest ||
		fact.AdmissionReceipt.ID != r.admissionID || fact.AdmissionReceipt.Revision != r.admissionRevision || fact.AdmissionReceipt.Digest != r.admissionDigest ||
		fact.RuntimeAdmissionReceipt.ID != r.runtimeAdmissionID || uint64(fact.RuntimeAdmissionReceipt.Revision) != r.runtimeAdmissionRevision || string(fact.RuntimeAdmissionReceipt.Digest) != r.runtimeAdmissionDigest ||
		string(fact.RuntimeAttemptDigest) != r.runtimeAttemptDigest || fact.AdmissionAttemptBindingDigest != r.bindingDigest || string(fact.AuthorizationDigest) != r.authorizationDigest ||
		fact.Association.ID != r.associationID || uint64(fact.Association.Revision) != r.associationRevision || string(fact.Association.Digest) != r.associationDigest ||
		fact.Command.ID != r.commandID || fact.Command.Revision != r.commandRevision || fact.Command.Digest != r.commandDigest ||
		fact.CommandPublication.ID != r.publicationID || fact.CommandPublication.Revision != r.publicationRevision || fact.CommandPublication.Digest != r.publicationDigest ||
		fact.CommandOwnerCurrent.ID != r.ownerCurrentID || fact.CommandOwnerCurrent.Revision != r.ownerCurrentRevision || fact.CommandOwnerCurrent.Digest != r.ownerCurrentDigest ||
		fact.WorkspaceView.ID != r.workspaceViewID || fact.WorkspaceView.Revision != r.workspaceViewRevision || fact.WorkspaceView.Digest != r.workspaceViewDigest ||
		fact.WorkspaceLeaseDigest != r.workspaceLeaseDigest ||
		fact.CurrentQueryDigest != r.currentQueryDigest || fact.ExpectedRuntimeCurrentDigest != runtimecore.Digest(r.expectedRuntimeCurrentDigest) || fact.ActualRequestDigest != r.actualRequestDigest || fact.PayloadDigest != r.payloadDigest ||
		fact.S1CheckedUnixNano != r.s1Checked {
		return contract.WorkspaceReadExecutionQualificationV2{}, fmt.Errorf("%w: workspace read Qualification denormalized coordinate drift", ports.ErrConflict)
	}
	want, err := workspaceReadQualificationRowDigestV20(fact, canonical)
	if err != nil || want != r.rowDigest {
		return contract.WorkspaceReadExecutionQualificationV2{}, fmt.Errorf("%w: workspace read Qualification row digest drift", ports.ErrConflict)
	}
	return fact, nil
}

const workspaceReadTerminalSelectV20 = `SELECT
	terminal_id,revision,digest,qualification_id,qualification_revision,qualification_digest,
	qualification_expires_unix_nano,origin_attempt_id,origin_attempt_revision,origin_attempt_digest,
	runtime_attempt_digest,actual_request_digest,
	journal_attempt_id,journal_request_digest,journal_payload_digest,journal_phase,journal_state,
	journal_revision,journal_recorded_unix_nano,journal_record_digest,outcome,
	observation_id,observation_revision,observation_digest,
	provider_receipt_id,provider_receipt_revision,provider_receipt_digest,s2_proof_digest,
	s2_checked_unix_nano,indeterminate_boundary,indeterminate_stage,indeterminate_error_class,
	indeterminate_error_digest,indeterminate_evidence_digest,indeterminate_fact_digest,
	outcome_checked_unix_nano,recorded_unix_nano,body,row_digest
	FROM workspace_read_terminal_history_v2`

type workspaceReadTerminalRowV20 struct {
	terminalID, digest                                                                       string
	revision                                                                                 uint64
	qualificationID, qualificationDigest                                                     string
	qualificationRevision                                                                    uint64
	qualificationExpires                                                                     int64
	originID, originDigest                                                                   string
	originRevision                                                                           uint64
	runtimeAttemptDigest, actualRequestDigest                                                string
	journalAttemptID, journalRequestDigest, journalPayloadDigest, journalPhase, journalState string
	journalRevision                                                                          uint64
	journalRecorded                                                                          int64
	journalDigest, outcome                                                                   string
	observationID, observationDigest                                                         string
	observationRevision                                                                      uint64
	providerID, providerDigest                                                               string
	providerRevision                                                                         uint64
	s2Proof                                                                                  string
	s2Checked                                                                                int64
	indeterminateBoundary, indeterminateStage, indeterminateClass                            string
	indeterminateError, indeterminateEvidence, indeterminateFact                             string
	outcomeChecked, recorded                                                                 int64
	body                                                                                     []byte
	rowDigest                                                                                string
}

func (r *workspaceReadTerminalRowV20) scanTargets() []any {
	return []any{&r.terminalID, &r.revision, &r.digest, &r.qualificationID, &r.qualificationRevision, &r.qualificationDigest,
		&r.qualificationExpires, &r.originID, &r.originRevision, &r.originDigest,
		&r.runtimeAttemptDigest, &r.actualRequestDigest,
		&r.journalAttemptID, &r.journalRequestDigest, &r.journalPayloadDigest, &r.journalPhase, &r.journalState,
		&r.journalRevision, &r.journalRecorded, &r.journalDigest, &r.outcome,
		&r.observationID, &r.observationRevision, &r.observationDigest,
		&r.providerID, &r.providerRevision, &r.providerDigest, &r.s2Proof,
		&r.s2Checked, &r.indeterminateBoundary, &r.indeterminateStage, &r.indeterminateClass,
		&r.indeterminateError, &r.indeterminateEvidence, &r.indeterminateFact,
		&r.outcomeChecked, &r.recorded, &r.body, &r.rowDigest}
}

func (r workspaceReadTerminalRowV20) factV20() (contract.WorkspaceReadTerminalFactV2, error) {
	var fact contract.WorkspaceReadTerminalFactV2
	if err := decode(r.body, &fact); err != nil {
		return fact, ports.ErrConflict
	}
	canonical, err := encode(fact)
	if err != nil || !bytes.Equal(canonical, r.body) || fact.Validate() != nil {
		return contract.WorkspaceReadTerminalFactV2{}, ports.ErrConflict
	}
	observationID, observationDigest, providerID, providerDigest, s2Proof := "", "", "", "", ""
	var s2Checked int64
	boundary, stage, errorClass, errorDigest, evidence, evidenceFact := "", "", "", "", "", ""
	var observationRevision, providerRevision uint64
	if fact.Observed != nil {
		proof := fact.Observed.S2Proof
		observationID, observationRevision, observationDigest = proof.Observation.ID, proof.Observation.Revision, proof.Observation.Digest
		providerID, providerRevision, providerDigest = proof.ProviderReceipt.ID, proof.ProviderReceipt.Revision, proof.ProviderReceipt.Digest
		s2Proof = proof.Digest
		s2Checked = proof.S2CheckedUnixNano
	} else {
		proof := fact.Indeterminate.Evidence
		boundary = proof.Boundary
		stage = string(proof.Stage)
		errorClass = string(proof.ErrorClass)
		errorDigest = proof.ErrorDigest
		evidence = proof.Digest
		evidenceFact = fact.Ref.Digest
	}
	if fact.Ref.ID != r.terminalID || fact.Ref.Revision != r.revision || fact.Ref.Digest != r.digest ||
		fact.Qualification.ID != r.qualificationID || fact.Qualification.Revision != r.qualificationRevision || fact.Qualification.Digest != r.qualificationDigest || fact.Qualification.ExpiresUnixNano != r.qualificationExpires ||
		fact.OriginAttempt.ID != r.originID || fact.OriginAttempt.Revision != r.originRevision || fact.OriginAttempt.Digest != r.originDigest ||
		string(fact.RuntimeAttemptDigest) != r.runtimeAttemptDigest || fact.ActualRequestDigest != r.actualRequestDigest ||
		fact.Journal.AttemptID != r.journalAttemptID || fact.Journal.RequestDigest != r.journalRequestDigest || fact.Journal.PayloadDigest != r.journalPayloadDigest || fact.Journal.Phase != r.journalPhase || string(fact.Journal.State) != r.journalState || fact.Journal.Revision != r.journalRevision || fact.Journal.RecordedUnixNano != r.journalRecorded || fact.Journal.RecordDigest != r.journalDigest ||
		string(fact.Outcome) != r.outcome || observationID != r.observationID || observationRevision != r.observationRevision || observationDigest != r.observationDigest || providerID != r.providerID || providerRevision != r.providerRevision || providerDigest != r.providerDigest || s2Proof != r.s2Proof || s2Checked != r.s2Checked || boundary != r.indeterminateBoundary || stage != r.indeterminateStage || errorClass != r.indeterminateClass || errorDigest != r.indeterminateError || evidence != r.indeterminateEvidence || evidenceFact != r.indeterminateFact ||
		fact.OutcomeCheckedUnixNano != r.outcomeChecked || fact.RecordedUnixNano != r.recorded {
		return contract.WorkspaceReadTerminalFactV2{}, ports.ErrConflict
	}
	want, err := workspaceReadTerminalRowDigestV20(fact, canonical)
	if err != nil || want != r.rowDigest {
		return contract.WorkspaceReadTerminalFactV2{}, ports.ErrConflict
	}
	return fact, nil
}
