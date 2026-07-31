package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"reflect"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	ownerworkspaceread "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// ReserveWorkspaceReadAuthorizedV2 is an internal Sandbox-owner writer. Its
// nominal request type cannot be imported by Tool, Runtime, Application, or
// another module under Go's internal-package rule. Public consumers only get
// WorkspaceReadRuntimeAttemptAdmissionReaderV2.
func (s *Store) ReserveWorkspaceReadAuthorizedV2(
	ctx context.Context,
	request ownerworkspaceread.AuthorizedReservationV2,
) (contract.WorkspaceReadExecutionProjectionV1, bool, error) {
	if ctx == nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, errors.New("workspace read authorized reservation context is required")
	}
	if err := ctx.Err(); err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, err
	}
	if s == nil || s.db == nil || s.clock == nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, errors.New("workspace read authorized reservation store is unavailable")
	}
	now := s.clock()
	reservation, attempt, binding, authorization, err := request.Open(now)
	if err != nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, false, ports.ErrConflict
	}
	return s.reserveWorkspaceReadV1(ctx, reservation, attempt, binding, &authorization)
}

func sealWorkspaceReadAdmissionAttemptBindingV2(
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
	binding ports.WorkspaceReadAdmissionAttemptBindingV1,
	command contract.WorkspaceReadCommandV1,
) (ports.WorkspaceReadAdmissionAttemptBindingV2, error) {
	if err := authorization.Validate(); err != nil {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, err
	}
	authorizationDigest, err := authorization.DigestV3()
	if err != nil ||
		authorizationDigest != authorization.AuthorizationDigest ||
		authorization.AuthorizationDigest != binding.AuthorizationDigest ||
		authorization.Association != binding.Association ||
		authorization.DomainCommand != binding.DomainCommand ||
		command.Meta.Ref() != binding.Command ||
		command.SourceToolPayloadSchema != authorization.Prepared.PayloadSchema.Key() ||
		command.SourceToolPayloadDigest != string(authorization.Prepared.PayloadDigest) ||
		command.SourceToolPayloadRevision != uint64(authorization.Prepared.PayloadRevision) {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, ports.ErrConflict
	}
	return ports.SealWorkspaceReadAdmissionAttemptBindingV2(ports.WorkspaceReadAdmissionAttemptBindingV2{
		RuntimeAttempt:       authorization.Attempt,
		AdmissionBinding:     binding,
		AuthorizationDigest:  authorization.AuthorizationDigest,
		Association:          authorization.Association,
		DomainCommand:        authorization.DomainCommand,
		WorkspaceReadCommand: command,
		AdmissionReceipt:     binding.AdmissionReceipt,
		WorkspaceReadAttempt: binding.Attempt,
	})
}

func validateWorkspaceReadRuntimeAttemptAdmissionClosureV2(
	reservation contract.WorkspaceReadReservationV1,
	origin contract.WorkspaceReadAttemptV1,
	command contract.WorkspaceReadCommandV1,
	binding ports.WorkspaceReadAdmissionAttemptBindingV2,
) error {
	if err := reservation.ValidateShape(); err != nil {
		return err
	}
	if err := origin.ValidateShape(); err != nil {
		return err
	}
	if err := command.ValidateShape(); err != nil {
		return err
	}
	if err := binding.Validate(); err != nil {
		return err
	}
	// The exact immutable Command ref binds its payload schema and revision into
	// Meta.Digest. Creation additionally compares those axes with the Runtime
	// authorization's Prepared payload before the V2 fact is sealed.
	base := binding.AdmissionBinding
	if reservation.Meta.Ref() != origin.Reservation ||
		reservation.Command != command.Meta.Ref() ||
		reservation.Command != base.Command ||
		reservation.WorkspaceView != command.WorkspaceView ||
		reservation.RequestDigest != command.Meta.Digest ||
		reservation.PayloadDigest != command.SourceToolPayloadDigest ||
		reservation.AttemptID != origin.Meta.ID ||
		reservation.AuthorizationDigest != string(base.AuthorizationDigest) ||
		reservation.StableKeyDigest != string(base.StableKeyDigest) ||
		reservation.StableKeyDigest != origin.StableKeyDigest ||
		reservation.RequestDigest != origin.RequestDigest ||
		reservation.PayloadDigest != origin.PayloadDigest ||
		base.Attempt.OwnerRef() != origin.Meta.Ref() ||
		base.AdmissionReceipt.ID != origin.AdmissionReceipt.ID ||
		uint64(base.AdmissionReceipt.Revision) != origin.AdmissionReceipt.Revision ||
		string(base.AdmissionReceipt.Digest) != origin.AdmissionReceipt.Digest ||
		string(base.AdmissionReceipt.StableKeyDigest) != origin.AdmissionReceipt.StableKeyDigest ||
		base.DomainCommand.ID != command.Meta.ID ||
		uint64(base.DomainCommand.Revision) != command.Meta.Revision ||
		string(base.DomainCommand.Digest) != "sha256:"+command.Meta.Digest ||
		binding.AuthorizationDigest != base.AuthorizationDigest ||
		binding.Association != base.Association ||
		binding.DomainCommand != base.DomainCommand ||
		binding.WorkspaceReadCommand.Meta.Ref() != command.Meta.Ref() ||
		binding.AdmissionReceipt != base.AdmissionReceipt ||
		binding.WorkspaceReadAttempt != base.Attempt {
		return ports.ErrConflict
	}
	return nil
}

func (s *Store) InspectWorkspaceReadAdmissionForRuntimeAttemptV2(
	ctx context.Context,
	exact runtimeports.OperationDispatchAttemptRefV3,
) (ports.WorkspaceReadAdmissionAttemptBindingV2, error) {
	if ctx == nil {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, errors.New("workspace read Runtime-attempt Inspect context is required")
	}
	if err := ctx.Err(); err != nil {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, err
	}
	if s == nil || s.db == nil {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, errors.New("workspace read Runtime-attempt reader is unavailable")
	}
	if err := exact.Validate(); err != nil {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, err
	}
	defer tx.Rollback()
	binding, err := inspectWorkspaceReadAdmissionForRuntimeAttemptTxV2(ctx, tx, exact)
	if err != nil {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, err
	}
	command, err := inspectWorkspaceReadCommandExactTxV1(ctx, tx, binding.WorkspaceReadCommand.Meta.Ref())
	if err != nil || !reflect.DeepEqual(command, binding.WorkspaceReadCommand) {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, ports.ErrConflict
	}
	var (
		stable            string
		originRevision    uint64
		originDigest      string
		originBody        []byte
		reservationStable string
		reservationID     string
		reservationBody   []byte
		v1AttemptID       string
		v1AttemptRevision uint64
		v1AttemptDigest   string
		v1Body            []byte
	)
	if err = tx.QueryRowContext(
		ctx,
		`SELECT stable_digest,revision,digest,body
		   FROM workspace_read_attempt_origin WHERE attempt_id=?`,
		binding.WorkspaceReadAttempt.ID,
	).Scan(&stable, &originRevision, &originDigest, &originBody); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ports.WorkspaceReadAdmissionAttemptBindingV2{}, ports.ErrConflict
		}
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, err
	}
	if originRevision != binding.WorkspaceReadAttempt.Revision ||
		originDigest != binding.WorkspaceReadAttempt.Digest {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, ports.ErrConflict
	}
	var origin contract.WorkspaceReadAttemptV1
	if err = decode(originBody, &origin); err != nil {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, ports.ErrConflict
	}
	if stable != origin.StableKeyDigest {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, ports.ErrConflict
	}
	originCanonical, canonicalErr := encode(origin)
	if canonicalErr != nil || !bytes.Equal(originCanonical, originBody) {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, ports.ErrConflict
	}
	if err = tx.QueryRowContext(
		ctx,
		`SELECT stable_digest,reservation_id,body
		   FROM workspace_read_reservation WHERE stable_digest=?`,
		stable,
	).Scan(&reservationStable, &reservationID, &reservationBody); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ports.WorkspaceReadAdmissionAttemptBindingV2{}, ports.ErrConflict
		}
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, err
	}
	var reservation contract.WorkspaceReadReservationV1
	if err = decode(reservationBody, &reservation); err != nil ||
		reservationStable != stable ||
		reservationStable != reservation.StableKeyDigest ||
		reservationID != reservation.Meta.ID {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, ports.ErrConflict
	}
	reservationCanonical, canonicalErr := encode(reservation)
	if canonicalErr != nil || !bytes.Equal(reservationCanonical, reservationBody) {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, ports.ErrConflict
	}
	if err = tx.QueryRowContext(
		ctx,
		`SELECT attempt_id,attempt_revision,attempt_digest,body
		   FROM workspace_read_admission_attempt_binding
		  WHERE admission_id=? AND admission_revision=? AND admission_digest=?`,
		binding.AdmissionReceipt.ID,
		binding.AdmissionReceipt.Revision,
		binding.AdmissionReceipt.Digest,
	).Scan(&v1AttemptID, &v1AttemptRevision, &v1AttemptDigest, &v1Body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ports.WorkspaceReadAdmissionAttemptBindingV2{}, ports.ErrConflict
		}
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, err
	}
	var v1 ports.WorkspaceReadAdmissionAttemptBindingV1
	v1Canonical := []byte(nil)
	if err = decode(v1Body, &v1); err != nil ||
		func() bool {
			v1Canonical, canonicalErr = encode(v1)
			return canonicalErr != nil || !bytes.Equal(v1Canonical, v1Body)
		}() ||
		v1.Validate() != nil ||
		v1 != binding.AdmissionBinding ||
		v1AttemptID != v1.Attempt.ID ||
		v1AttemptRevision != v1.Attempt.Revision ||
		v1AttemptDigest != v1.Attempt.Digest ||
		validateWorkspaceReadRuntimeAttemptAdmissionClosureV2(reservation, origin, command, binding) != nil {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, ports.ErrConflict
	}
	return binding, nil
}

func inspectWorkspaceReadAdmissionForRuntimeAttemptTxV2(
	ctx context.Context,
	source queryer,
	exact runtimeports.OperationDispatchAttemptRefV3,
) (ports.WorkspaceReadAdmissionAttemptBindingV2, error) {
	runtimeAttemptDigest, err := ports.WorkspaceReadRuntimeAttemptDigestV2(exact)
	if err != nil {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, err
	}
	var (
		storedRuntimeDigest      string
		operationDigest          string
		effectID                 string
		intentRevision           uint64
		intentDigest             string
		permitID                 string
		permitRevision           uint64
		permitDigest             string
		runtimeAttemptID         string
		delegationPresent        int
		delegationID             string
		delegationRevision       uint64
		delegationDigest         string
		authorizationDigest      string
		associationID            string
		associationRevision      uint64
		associationDigest        string
		domainCommandID          string
		domainCommandRevision    uint64
		domainCommandDigest      string
		commandID                string
		commandRevision          uint64
		commandDigest            string
		admissionID              string
		admissionRevision        uint64
		admissionDigest          string
		workspaceAttemptID       string
		workspaceAttemptRevision uint64
		workspaceAttemptDigest   string
		bindingDigest            string
		body                     []byte
	)
	if err = source.QueryRowContext(
		ctx,
		`SELECT runtime_attempt_digest,operation_digest,effect_id,intent_revision,intent_digest,
		        permit_id,permit_revision,permit_digest,runtime_attempt_id,
		        delegation_present,delegation_id,delegation_revision,delegation_digest,
		        authorization_digest,
		        association_id,association_revision,association_digest,
		        domain_command_id,domain_command_revision,domain_command_digest,
		        command_id,command_revision,command_digest,
		        admission_id,admission_revision,admission_digest,
		        workspace_attempt_id,workspace_attempt_revision,workspace_attempt_digest,
		        binding_digest,body
		   FROM workspace_read_runtime_attempt_admission_binding_v2
		  WHERE runtime_attempt_digest=?`,
		runtimeAttemptDigest,
	).Scan(
		&storedRuntimeDigest,
		&operationDigest,
		&effectID,
		&intentRevision,
		&intentDigest,
		&permitID,
		&permitRevision,
		&permitDigest,
		&runtimeAttemptID,
		&delegationPresent,
		&delegationID,
		&delegationRevision,
		&delegationDigest,
		&authorizationDigest,
		&associationID,
		&associationRevision,
		&associationDigest,
		&domainCommandID,
		&domainCommandRevision,
		&domainCommandDigest,
		&commandID,
		&commandRevision,
		&commandDigest,
		&admissionID,
		&admissionRevision,
		&admissionDigest,
		&workspaceAttemptID,
		&workspaceAttemptRevision,
		&workspaceAttemptDigest,
		&bindingDigest,
		&body,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ports.WorkspaceReadAdmissionAttemptBindingV2{}, ports.ErrNotFound
		}
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, err
	}
	var binding ports.WorkspaceReadAdmissionAttemptBindingV2
	if err = decode(body, &binding); err != nil {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, ports.ErrConflict
	}
	expectedDelegationPresent := 0
	expectedDelegationID := ""
	var expectedDelegationRevision uint64
	expectedDelegationDigest := ""
	if binding.RuntimeAttempt.Delegation != nil {
		expectedDelegationPresent = 1
		expectedDelegationID = binding.RuntimeAttempt.Delegation.ID
		expectedDelegationRevision = uint64(binding.RuntimeAttempt.Delegation.Revision)
		expectedDelegationDigest = string(binding.RuntimeAttempt.Delegation.Digest)
	}
	canonical, err := encode(binding)
	if err != nil ||
		!bytes.Equal(canonical, body) ||
		binding.Validate() != nil ||
		!reflect.DeepEqual(binding.RuntimeAttempt, exact) ||
		storedRuntimeDigest != string(runtimeAttemptDigest) ||
		operationDigest != string(binding.RuntimeAttempt.OperationDigest) ||
		effectID != string(binding.RuntimeAttempt.EffectID) ||
		intentRevision != uint64(binding.RuntimeAttempt.IntentRevision) ||
		intentDigest != string(binding.RuntimeAttempt.IntentDigest) ||
		permitID != binding.RuntimeAttempt.PermitID ||
		permitRevision != uint64(binding.RuntimeAttempt.PermitRevision) ||
		permitDigest != string(binding.RuntimeAttempt.PermitDigest) ||
		runtimeAttemptID != binding.RuntimeAttempt.AttemptID ||
		delegationPresent != expectedDelegationPresent ||
		delegationID != expectedDelegationID ||
		delegationRevision != expectedDelegationRevision ||
		delegationDigest != expectedDelegationDigest ||
		authorizationDigest != string(binding.AuthorizationDigest) ||
		associationID != binding.Association.ID ||
		associationRevision != uint64(binding.Association.Revision) ||
		associationDigest != string(binding.Association.Digest) ||
		domainCommandID != binding.DomainCommand.ID ||
		domainCommandRevision != uint64(binding.DomainCommand.Revision) ||
		domainCommandDigest != string(binding.DomainCommand.Digest) ||
		commandID != binding.WorkspaceReadCommand.Meta.ID ||
		commandRevision != binding.WorkspaceReadCommand.Meta.Revision ||
		commandDigest != binding.WorkspaceReadCommand.Meta.Digest ||
		admissionID != binding.AdmissionReceipt.ID ||
		admissionRevision != uint64(binding.AdmissionReceipt.Revision) ||
		admissionDigest != string(binding.AdmissionReceipt.Digest) ||
		workspaceAttemptID != binding.WorkspaceReadAttempt.ID ||
		workspaceAttemptRevision != binding.WorkspaceReadAttempt.Revision ||
		workspaceAttemptDigest != binding.WorkspaceReadAttempt.Digest ||
		bindingDigest != binding.Digest {
		return ports.WorkspaceReadAdmissionAttemptBindingV2{}, ports.ErrConflict
	}
	return binding, nil
}

var _ ports.WorkspaceReadRuntimeAttemptAdmissionReaderV2 = (*Store)(nil)
