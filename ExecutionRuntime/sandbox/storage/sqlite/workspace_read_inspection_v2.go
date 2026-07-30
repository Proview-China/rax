package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// InspectBoundedWorkspaceReadV2 proves, in one read transaction, that the
// latest current attempt descends from the caller's exact immutable origin.
// It never calls a Provider, reads workspace bytes, or renews execution
// eligibility.
func (s *Store) InspectBoundedWorkspaceReadV2(
	ctx context.Context,
	exact contract.WorkspaceReadAttemptRefV1,
) (ports.WorkspaceReadInspectionEnvelopeV2, error) {
	if s == nil || s.db == nil || s.clock == nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, errors.New("Sandbox workspace read inspection store is unavailable")
	}
	if ctx == nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, errors.New("Sandbox workspace read inspection context is required")
	}
	if err := exact.Validate(); err != nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, err
	}
	initial := s.clock()
	if initial.IsZero() || initial.UnixNano() <= 0 {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, ports.ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, err
	}
	defer tx.Rollback()

	origin, stable, err := inspectWorkspaceReadOriginV2Tx(ctx, tx, exact)
	if err != nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, err
	}
	reservation, err := inspectWorkspaceReadReservationV2Tx(ctx, tx, stable)
	if err != nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, err
	}
	command, err := inspectWorkspaceReadCommandV2Tx(ctx, tx, reservation.Command)
	if err != nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, err
	}
	workspace, err := inspectWorkspaceReadViewV2Tx(ctx, tx, reservation.WorkspaceView)
	if err != nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, err
	}
	binding, err := inspectWorkspaceReadAdmissionBindingV2Tx(ctx, tx, exact)
	if err != nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, err
	}
	current, err := inspectWorkspaceReadCurrentV2Tx(ctx, tx, stable, exact.ID)
	if err != nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, err
	}
	if err := validateWorkspaceReadInspectionClosureV2(
		exact,
		origin,
		reservation,
		command,
		binding,
		current,
		initial,
	); err != nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, err
	}
	projection, err := s.inspectWorkspaceReadTx(ctx, tx, stable)
	if err != nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, ports.ErrConflict
	}
	if err := validateWorkspaceReadInspectionProjectionV2(current, reservation, command, workspace, origin, projection); err != nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, err
	}
	fresh := s.clock()
	if fresh.IsZero() || fresh.UnixNano() <= 0 || fresh.UnixNano() < initial.UnixNano() {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, ports.ErrConflict
	}
	expiresUnixNano, err := workspaceReadInspectionExpiryV2(
		fresh,
		origin,
		reservation,
		command,
		binding,
		current,
		projection,
	)
	if err != nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, err
	}

	envelope, err := ports.SealWorkspaceReadInspectionEnvelopeV2(
		ports.WorkspaceReadInspectionEnvelopeV2{
			RequestedOriginAttemptRef: exact,
			CurrentProjection:         projection,
			CheckedUnixNano:           fresh.UnixNano(),
			ExpiresUnixNano:           expiresUnixNano,
		},
	)
	if err != nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, err
	}
	if err := envelope.ValidateCurrent(fresh); err != nil {
		return ports.WorkspaceReadInspectionEnvelopeV2{}, err
	}
	return envelope, nil
}

func workspaceReadInspectionExpiryV2(
	fresh time.Time,
	origin contract.WorkspaceReadAttemptV1,
	reservation contract.WorkspaceReadReservationV1,
	command contract.WorkspaceReadCommandV1,
	binding ports.WorkspaceReadAdmissionAttemptBindingV1,
	current contract.WorkspaceReadAttemptV1,
	projection contract.WorkspaceReadExecutionProjectionV1,
) (int64, error) {
	envelopeExpiry := fresh.Add(ports.WorkspaceReadInspectionMaxTTLV2).UnixNano()
	if envelopeExpiry <= fresh.UnixNano() {
		return 0, ports.ErrConflict
	}
	// Terminal facts remain inspectable after execution eligibility expires.
	// Their historical TTL is not renewed: only this newly checked, read-only
	// envelope receives the independent bounded inspection lifetime.
	if current.State != contract.WorkspaceReadStartedV1 {
		return envelopeExpiry, nil
	}
	executionExpiry := minWorkspaceReadStoreExpiryV1(
		origin.Meta.ExpiresUnixNano,
		origin.AdmissionReceipt.ExpiresUnixNano,
		reservation.Meta.ExpiresUnixNano,
		reservation.TTLClosure.EffectiveExpiresUnixNano,
		command.Meta.ExpiresUnixNano,
		command.RequestedNotAfterUnixNano,
		binding.ExpiresUnixNano,
		current.Meta.ExpiresUnixNano,
		current.AdmissionReceipt.ExpiresUnixNano,
		projection.Attempt.Meta.ExpiresUnixNano,
		projection.Reservation.Meta.ExpiresUnixNano,
		projection.AdmissionReceipt.ExpiresUnixNano,
	)
	if executionExpiry <= fresh.UnixNano() {
		return 0, ports.ErrConflict
	}
	if executionExpiry < envelopeExpiry {
		envelopeExpiry = executionExpiry
	}
	return envelopeExpiry, nil
}

func inspectWorkspaceReadOriginV2Tx(
	ctx context.Context,
	tx *sql.Tx,
	exact contract.WorkspaceReadAttemptRefV1,
) (contract.WorkspaceReadAttemptV1, string, error) {
	var stable string
	var revision uint64
	var digest string
	var body []byte
	err := tx.QueryRowContext(
		ctx,
		`SELECT stable_digest,revision,digest,body
		   FROM workspace_read_attempt_origin
		  WHERE attempt_id=?`,
		exact.ID,
	).Scan(&stable, &revision, &digest, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.WorkspaceReadAttemptV1{}, "", ports.ErrNotFound
	}
	if err != nil {
		return contract.WorkspaceReadAttemptV1{}, "", err
	}
	if revision != exact.Revision || digest != exact.Digest || !contract.ValidDigest(stable) {
		return contract.WorkspaceReadAttemptV1{}, "", ports.ErrConflict
	}
	var origin contract.WorkspaceReadAttemptV1
	if err := decode(body, &origin); err != nil {
		return contract.WorkspaceReadAttemptV1{}, "", err
	}
	if origin.ValidateShape() != nil ||
		origin.Meta.Ref() != exact.OwnerRef() ||
		origin.State != contract.WorkspaceReadStartedV1 ||
		origin.StableKeyDigest != stable {
		return contract.WorkspaceReadAttemptV1{}, "", ports.ErrConflict
	}
	return origin, stable, nil
}

func inspectWorkspaceReadReservationV2Tx(
	ctx context.Context,
	tx *sql.Tx,
	stable string,
) (contract.WorkspaceReadReservationV1, error) {
	var reservationID string
	var body []byte
	err := tx.QueryRowContext(
		ctx,
		`SELECT reservation_id,body
		   FROM workspace_read_reservation
		  WHERE stable_digest=?`,
		stable,
	).Scan(&reservationID, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.WorkspaceReadReservationV1{}, ports.ErrNotFound
	}
	if err != nil {
		return contract.WorkspaceReadReservationV1{}, err
	}
	var reservation contract.WorkspaceReadReservationV1
	if err := decode(body, &reservation); err != nil {
		return contract.WorkspaceReadReservationV1{}, err
	}
	if reservation.ValidateShape() != nil ||
		reservation.Meta.ID != reservationID ||
		reservation.StableKeyDigest != stable {
		return contract.WorkspaceReadReservationV1{}, ports.ErrConflict
	}
	return reservation, nil
}

func inspectWorkspaceReadCommandV2Tx(
	ctx context.Context,
	tx *sql.Tx,
	exact contract.Ref,
) (contract.WorkspaceReadCommandV1, error) {
	var revision uint64
	var digest string
	var body []byte
	err := tx.QueryRowContext(
		ctx,
		`SELECT revision,digest,body
		   FROM workspace_read_command_current
		  WHERE command_id=?`,
		exact.ID,
	).Scan(&revision, &digest, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.WorkspaceReadCommandV1{}, ports.ErrNotFound
	}
	if err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	if revision != exact.Revision || digest != exact.Digest {
		return contract.WorkspaceReadCommandV1{}, ports.ErrConflict
	}
	var command contract.WorkspaceReadCommandV1
	if err := decode(body, &command); err != nil {
		return contract.WorkspaceReadCommandV1{}, err
	}
	if command.ValidateShape() != nil || command.Meta.Ref() != exact {
		return contract.WorkspaceReadCommandV1{}, ports.ErrConflict
	}
	return command, nil
}

func inspectWorkspaceReadViewV2Tx(
	ctx context.Context,
	tx *sql.Tx,
	exact contract.Ref,
) (contract.WorkspaceView, error) {
	var revision uint64
	var digest string
	var body []byte
	err := tx.QueryRowContext(
		ctx,
		`SELECT revision,digest,body
		   FROM workspace_view_history
		  WHERE view_id=? AND revision=?`,
		exact.ID,
		exact.Revision,
	).Scan(&revision, &digest, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.WorkspaceView{}, ports.ErrNotFound
	}
	if err != nil {
		return contract.WorkspaceView{}, err
	}
	if revision != exact.Revision || digest != exact.Digest {
		return contract.WorkspaceView{}, ports.ErrConflict
	}
	var workspace contract.WorkspaceView
	if err := decode(body, &workspace); err != nil {
		return contract.WorkspaceView{}, err
	}
	if workspace.ValidateShape() != nil || workspace.Meta.Ref() != exact {
		return contract.WorkspaceView{}, ports.ErrConflict
	}
	return workspace, nil
}

func inspectWorkspaceReadAdmissionBindingV2Tx(
	ctx context.Context,
	tx *sql.Tx,
	exact contract.WorkspaceReadAttemptRefV1,
) (ports.WorkspaceReadAdmissionAttemptBindingV1, error) {
	var admissionID string
	var admissionRevision uint64
	var admissionDigest string
	var body []byte
	err := tx.QueryRowContext(
		ctx,
		`SELECT admission_id,admission_revision,admission_digest,body
		   FROM workspace_read_admission_attempt_binding
		  WHERE attempt_id=? AND attempt_revision=? AND attempt_digest=?`,
		exact.ID,
		exact.Revision,
		exact.Digest,
	).Scan(&admissionID, &admissionRevision, &admissionDigest, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.WorkspaceReadAdmissionAttemptBindingV1{}, ports.ErrNotFound
	}
	if err != nil {
		return ports.WorkspaceReadAdmissionAttemptBindingV1{}, err
	}
	var binding ports.WorkspaceReadAdmissionAttemptBindingV1
	if err := decode(body, &binding); err != nil {
		return ports.WorkspaceReadAdmissionAttemptBindingV1{}, err
	}
	if binding.Validate() != nil ||
		binding.Attempt != exact ||
		binding.AdmissionReceipt.ID != admissionID ||
		uint64(binding.AdmissionReceipt.Revision) != admissionRevision ||
		string(binding.AdmissionReceipt.Digest) != admissionDigest {
		return ports.WorkspaceReadAdmissionAttemptBindingV1{}, ports.ErrConflict
	}
	return binding, nil
}

func inspectWorkspaceReadCurrentV2Tx(
	ctx context.Context,
	tx *sql.Tx,
	stable string,
	attemptID string,
) (contract.WorkspaceReadAttemptV1, error) {
	var storedStable string
	var revision uint64
	var digest string
	var body []byte
	err := tx.QueryRowContext(
		ctx,
		`SELECT stable_digest,revision,digest,body
		   FROM workspace_read_attempt_current
		  WHERE attempt_id=?`,
		attemptID,
	).Scan(&storedStable, &revision, &digest, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.WorkspaceReadAttemptV1{}, ports.ErrNotFound
	}
	if err != nil {
		return contract.WorkspaceReadAttemptV1{}, err
	}
	var current contract.WorkspaceReadAttemptV1
	if err := decode(body, &current); err != nil {
		return contract.WorkspaceReadAttemptV1{}, err
	}
	if current.ValidateShape() != nil ||
		storedStable != stable ||
		current.StableKeyDigest != stable ||
		current.Meta.ID != attemptID ||
		current.Meta.Revision != revision ||
		current.Meta.Digest != digest {
		return contract.WorkspaceReadAttemptV1{}, ports.ErrConflict
	}
	return current, nil
}

func validateWorkspaceReadInspectionClosureV2(
	exact contract.WorkspaceReadAttemptRefV1,
	origin contract.WorkspaceReadAttemptV1,
	reservation contract.WorkspaceReadReservationV1,
	command contract.WorkspaceReadCommandV1,
	binding ports.WorkspaceReadAdmissionAttemptBindingV1,
	current contract.WorkspaceReadAttemptV1,
	now time.Time,
) error {
	if now.UnixNano() < origin.Meta.UpdatedUnixNano ||
		now.UnixNano() < reservation.Meta.UpdatedUnixNano ||
		now.UnixNano() < command.Meta.UpdatedUnixNano ||
		now.UnixNano() < binding.CreatedUnixNano ||
		now.UnixNano() < current.Meta.UpdatedUnixNano {
		return ports.ErrConflict
	}
	if reservation.AttemptID != exact.ID ||
		reservation.StableKeyDigest != origin.StableKeyDigest ||
		reservation.RequestDigest != origin.RequestDigest ||
		reservation.PayloadDigest != origin.PayloadDigest ||
		reservation.Meta.Ref() != origin.Reservation ||
		reservation.Command != command.Meta.Ref() ||
		reservation.RequestDigest != command.Meta.Digest ||
		reservation.PayloadDigest != command.SourceToolPayloadDigest ||
		reservation.WorkspaceView != command.WorkspaceView ||
		command.AttemptID != exact.ID {
		return ports.ErrConflict
	}
	if binding.Attempt != exact ||
		binding.Command != command.Meta.Ref() ||
		string(binding.AuthorizationDigest) != reservation.AuthorizationDigest ||
		string(binding.StableKeyDigest) != reservation.StableKeyDigest ||
		binding.CreatedUnixNano != origin.Meta.CreatedUnixNano ||
		binding.ExpiresUnixNano != origin.Meta.ExpiresUnixNano ||
		binding.DomainCommand.ID != command.Meta.ID ||
		uint64(binding.DomainCommand.Revision) != command.Meta.Revision ||
		binding.DomainCommand.Digest != runtimecore.Digest("sha256:"+command.Meta.Digest) ||
		binding.AdmissionReceipt.ID != origin.AdmissionReceipt.ID ||
		uint64(binding.AdmissionReceipt.Revision) != origin.AdmissionReceipt.Revision ||
		string(binding.AdmissionReceipt.Digest) != origin.AdmissionReceipt.Digest ||
		string(binding.AdmissionReceipt.StableKeyDigest) != origin.AdmissionReceipt.StableKeyDigest {
		return ports.ErrConflict
	}
	if current.Meta.ID != origin.Meta.ID ||
		current.StableKeyDigest != origin.StableKeyDigest ||
		current.RequestDigest != origin.RequestDigest ||
		current.PayloadDigest != origin.PayloadDigest ||
		current.Reservation != origin.Reservation ||
		current.AdmissionReceipt != origin.AdmissionReceipt ||
		current.Meta.ExpiresUnixNano != origin.Meta.ExpiresUnixNano {
		return ports.ErrConflict
	}
	switch current.State {
	case contract.WorkspaceReadStartedV1:
		if current.Meta.Ref() != origin.Meta.Ref() {
			return ports.ErrConflict
		}
	case contract.WorkspaceReadObservedV1, contract.WorkspaceReadFailedV1, contract.WorkspaceReadUnknownV1:
		if current.Meta.Revision != origin.Meta.Revision+1 ||
			current.Meta.CreatedUnixNano < origin.Meta.UpdatedUnixNano ||
			current.Meta.UpdatedUnixNano < origin.Meta.UpdatedUnixNano {
			return ports.ErrConflict
		}
	default:
		return ports.ErrConflict
	}
	return nil
}

func validateWorkspaceReadInspectionProjectionV2(
	current contract.WorkspaceReadAttemptV1,
	reservation contract.WorkspaceReadReservationV1,
	command contract.WorkspaceReadCommandV1,
	workspace contract.WorkspaceView,
	origin contract.WorkspaceReadAttemptV1,
	projection contract.WorkspaceReadExecutionProjectionV1,
) error {
	if projection.Attempt.Meta.Ref() != current.Meta.Ref() ||
		projection.Reservation.Meta.Ref() != reservation.Meta.Ref() ||
		projection.Reservation.StableKeyDigest != reservation.StableKeyDigest ||
		projection.Reservation.RequestDigest != reservation.RequestDigest ||
		projection.Reservation.PayloadDigest != reservation.PayloadDigest ||
		projection.Reservation.Command != reservation.Command ||
		projection.Reservation.WorkspaceView != reservation.WorkspaceView ||
		projection.AdmissionReceipt != origin.AdmissionReceipt ||
		command.Meta.Ref() != reservation.Command ||
		command.WorkspaceView != reservation.WorkspaceView ||
		workspace.Meta.Ref() != reservation.WorkspaceView ||
		workspace.FileScopeDigest != command.FileScopeDigest ||
		!workspaceReadStoreAllowedV1(workspace, command.RelativePath) {
		return ports.ErrConflict
	}
	if current.State != contract.WorkspaceReadObservedV1 {
		return nil
	}
	observation := projection.Observation
	providerReceipt := projection.ProviderReceipt
	if observation == nil || providerReceipt == nil {
		return ports.ErrConflict
	}
	fileID, err := contract.WorkspaceReadFileIDV1(workspace.Meta.ID, command.RelativePath)
	if err != nil ||
		observation.Reservation != reservation.Meta.Ref() ||
		observation.Command != command.Meta.Ref() ||
		observation.WorkspaceView != workspace.Meta.Ref() ||
		observation.RelativePath != command.RelativePath ||
		observation.File.ID != fileID ||
		observation.File.Revision != workspace.Meta.Revision ||
		observation.StartByte != command.StartByte ||
		observation.ReturnedBytes != uint64(len([]byte(observation.Content))) ||
		observation.ReturnedBytes > command.MaxBytes ||
		observation.StartByte > observation.TotalBytes ||
		observation.ReturnedBytes > observation.TotalBytes-observation.StartByte ||
		observation.Complete != (observation.StartByte+observation.ReturnedBytes == observation.TotalBytes) ||
		observation.ContentDigest != contract.WorkspaceReadContentDigestV1(
			[]byte(observation.Content),
			observation.StartByte,
			observation.TotalBytes,
			observation.Complete,
		) ||
		current.Observation == nil ||
		*current.Observation != observation.Meta.Ref() ||
		observation.AdmissionReceipt != origin.AdmissionReceipt ||
		observation.AdmissionReceipt != current.AdmissionReceipt ||
		observation.ProviderReceipt != *providerReceipt ||
		providerReceipt.StableKeyDigest != reservation.StableKeyDigest ||
		observation.Meta.CreatedUnixNano < origin.Meta.UpdatedUnixNano ||
		observation.Meta.UpdatedUnixNano < origin.Meta.UpdatedUnixNano ||
		current.Meta.CreatedUnixNano < observation.Meta.UpdatedUnixNano ||
		current.Meta.UpdatedUnixNano < observation.Meta.UpdatedUnixNano ||
		observation.S1CheckedUnixNano < origin.Meta.CreatedUnixNano ||
		observation.S2CheckedUnixNano < observation.S1CheckedUnixNano ||
		observation.ProviderReceipt.CheckedUnixNano < origin.Meta.CreatedUnixNano ||
		observation.ProviderReceipt.CheckedUnixNano < observation.S1CheckedUnixNano ||
		observation.ProviderReceipt.CheckedUnixNano > observation.S2CheckedUnixNano {
		return ports.ErrConflict
	}
	if command.ExpectedFileRef != nil && observation.File != *command.ExpectedFileRef {
		return ports.ErrConflict
	}
	return nil
}

var _ ports.WorkspaceReadInspectionReaderV2 = (*Store)(nil)
