package workspaceread

import (
	"context"
	"errors"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type PreparedWorkspaceReadRequestProofV2 struct {
	runtimeAttemptID string
	requestDigest    string
	payloadDigest    string
	expiresUnixNano  int64
}

func AuthorizePreparedWorkspaceReadRequestProofV2(
	runtimeAttemptID string,
	requestDigest string,
	payloadDigest string,
	expiresUnixNano int64,
) (PreparedWorkspaceReadRequestProofV2, error) {
	proof := PreparedWorkspaceReadRequestProofV2{
		runtimeAttemptID: runtimeAttemptID, requestDigest: requestDigest,
		payloadDigest: payloadDigest, expiresUnixNano: expiresUnixNano,
	}
	if err := proof.validateV2(); err != nil {
		return PreparedWorkspaceReadRequestProofV2{}, err
	}
	return proof, nil
}

func (p PreparedWorkspaceReadRequestProofV2) validateV2() error {
	if p.runtimeAttemptID == "" ||
		!contract.ValidDigest(p.requestDigest) ||
		!contract.ValidDigest(p.payloadDigest) ||
		p.expiresUnixNano <= 0 {
		return errors.New("workspace read prepared request proof is incomplete")
	}
	return nil
}

// AuthorizedExecutionQualificationV2 is a Sandbox-internal write capability.
// External Owners cannot import this package or submit caller-sealed facts to
// the SQLite owner.
type AuthorizedExecutionQualificationV2 struct {
	fact contract.WorkspaceReadExecutionQualificationV2
}

func AuthorizeExecutionQualificationV2(
	fact contract.WorkspaceReadExecutionQualificationV2,
	binding sandboxports.WorkspaceReadAdmissionAttemptBindingV2,
	reservation contract.WorkspaceReadReservationV1,
	attempt contract.WorkspaceReadAttemptV1,
	publication contract.WorkspaceReadCommandPublicationV2,
	ownerCurrent contract.WorkspaceReadCommandOwnerCurrentV2,
	workspace contract.WorkspaceView,
	query sandboxports.WorkspaceReadCurrentQueryV2,
	runtimeCurrent runtimeports.CurrentOperationDispatchEnforcementV4,
	prepared PreparedWorkspaceReadRequestProofV2,
) (AuthorizedExecutionQualificationV2, error) {
	if err := fact.Validate(); err != nil {
		return AuthorizedExecutionQualificationV2{}, err
	}
	if err := binding.Validate(); err != nil {
		return AuthorizedExecutionQualificationV2{}, err
	}
	if err := reservation.ValidateCurrent(time.Unix(0, fact.S1CheckedUnixNano)); err != nil {
		return AuthorizedExecutionQualificationV2{}, err
	}
	if err := attempt.ValidateCurrent(time.Unix(0, fact.S1CheckedUnixNano)); err != nil {
		return AuthorizedExecutionQualificationV2{}, err
	}
	if err := publication.ValidateShape(); err != nil {
		return AuthorizedExecutionQualificationV2{}, err
	}
	if err := ownerCurrent.ValidateShape(); err != nil {
		return AuthorizedExecutionQualificationV2{}, err
	}
	if err := workspace.ValidateShape(); err != nil {
		return AuthorizedExecutionQualificationV2{}, err
	}
	if err := query.Validate(); err != nil {
		return AuthorizedExecutionQualificationV2{}, err
	}
	if err := runtimeCurrent.Validate(); err != nil {
		return AuthorizedExecutionQualificationV2{}, err
	}
	if err := prepared.validateV2(); err != nil {
		return AuthorizedExecutionQualificationV2{}, err
	}
	leaseDigest, err := contract.WorkspaceReadRuntimeLeaseDigestV2(workspace.Lease)
	if err != nil {
		return AuthorizedExecutionQualificationV2{}, err
	}
	bindingAttemptDigest, err := contract.WorkspaceReadSourceRuntimeAttemptDigestV2(binding.RuntimeAttempt)
	if err != nil ||
		fact.AdmissionAttemptBindingDigest != binding.Digest ||
		fact.Reservation != reservation.Meta.Ref() ||
		reservation.Command != fact.Command ||
		reservation.WorkspaceView != fact.WorkspaceView ||
		reservation.AttemptID != fact.OriginAttempt.ID ||
		fact.OriginAttempt != (contract.WorkspaceReadAttemptRefV1{
			ID: attempt.Meta.ID, Revision: attempt.Meta.Revision, Digest: attempt.Meta.Digest,
		}) ||
		attempt.Reservation != fact.Reservation ||
		attempt.AdmissionReceipt != fact.AdmissionReceipt ||
		attempt.State != contract.WorkspaceReadStartedV1 ||
		fact.OriginAttempt != binding.WorkspaceReadAttempt ||
		fact.RuntimeAttemptDigest != bindingAttemptDigest ||
		fact.AuthorizationDigest != binding.AuthorizationDigest ||
		fact.Association != binding.Association ||
		fact.Command != binding.WorkspaceReadCommand.Meta.Ref() ||
		fact.RuntimeAdmissionReceipt != binding.AdmissionReceipt ||
		fact.AdmissionReceipt.ID != binding.AdmissionBinding.AdmissionReceipt.ID ||
		fact.AdmissionReceipt.Revision != uint64(binding.AdmissionBinding.AdmissionReceipt.Revision) ||
		fact.AdmissionReceipt.Digest != string(binding.AdmissionBinding.AdmissionReceipt.Digest) ||
		fact.CommandPublication != publication.Meta.Ref() ||
		publication.Command != fact.Command ||
		fact.CommandOwnerCurrent != ownerCurrent.Meta.Ref() ||
		ownerCurrent.Command != fact.Command ||
		ownerCurrent.Publication != fact.CommandPublication ||
		fact.WorkspaceView != workspace.Meta.Ref() ||
		fact.WorkspaceLeaseDigest != leaseDigest ||
		fact.CurrentQueryDigest != query.Digest ||
		query.Base.Association != fact.Association ||
		query.Base.Command != fact.Command ||
		query.Base.PublishedCommandCurrent == nil ||
		*query.Base.PublishedCommandCurrent != fact.CommandOwnerCurrent ||
		query.Base.WorkspaceView != fact.WorkspaceView ||
		fact.ExpectedRuntimeCurrentDigest != runtimeCurrent.Digest ||
		fact.ActualRequestDigest != prepared.requestDigest ||
		fact.PayloadDigest != prepared.payloadDigest ||
		fact.RuntimeAttempt.AttemptID != prepared.runtimeAttemptID ||
		fact.ExpiresUnixNano > prepared.expiresUnixNano ||
		fact.ExpiresUnixNano > query.Base.ExpiresUnixNano ||
		fact.ExpiresUnixNano > runtimeCurrent.ExpiresUnixNano ||
		fact.ExpiresUnixNano > ownerCurrent.ExpiresUnixNano ||
		fact.ExpiresUnixNano > workspace.Lease.ExpiresUnixNano ||
		fact.ExpiresUnixNano > reservation.Meta.ExpiresUnixNano ||
		fact.ExpiresUnixNano > attempt.Meta.ExpiresUnixNano ||
		fact.ExpiresUnixNano > binding.AdmissionBinding.ExpiresUnixNano ||
		fact.ExpiresUnixNano > fact.AdmissionReceipt.ExpiresUnixNano ||
		!qualificationS1CheckedClosesSourcesV2(fact, binding, reservation, attempt, publication, ownerCurrent, workspace, query, runtimeCurrent) {
		return AuthorizedExecutionQualificationV2{}, sandboxports.ErrConflict
	}
	return AuthorizedExecutionQualificationV2{fact: fact}, nil
}

func qualificationS1CheckedClosesSourcesV2(
	fact contract.WorkspaceReadExecutionQualificationV2,
	binding sandboxports.WorkspaceReadAdmissionAttemptBindingV2,
	reservation contract.WorkspaceReadReservationV1,
	attempt contract.WorkspaceReadAttemptV1,
	publication contract.WorkspaceReadCommandPublicationV2,
	ownerCurrent contract.WorkspaceReadCommandOwnerCurrentV2,
	workspace contract.WorkspaceView,
	query sandboxports.WorkspaceReadCurrentQueryV2,
	runtimeCurrent runtimeports.CurrentOperationDispatchEnforcementV4,
) bool {
	checked := fact.S1CheckedUnixNano
	return checked >= fact.AdmissionReceipt.CheckedUnixNano &&
		checked >= binding.AdmissionBinding.CreatedUnixNano &&
		checked >= binding.WorkspaceReadCommand.Meta.UpdatedUnixNano &&
		checked >= publication.Meta.UpdatedUnixNano &&
		checked >= ownerCurrent.CheckedUnixNano &&
		checked >= ownerCurrent.Meta.UpdatedUnixNano &&
		checked >= workspace.Meta.UpdatedUnixNano &&
		checked >= reservation.Meta.UpdatedUnixNano &&
		checked >= attempt.Meta.UpdatedUnixNano &&
		checked >= query.Base.CheckedUnixNano &&
		checked >= runtimeCurrent.CheckedUnixNano
}

func (a AuthorizedExecutionQualificationV2) FactV2() (contract.WorkspaceReadExecutionQualificationV2, error) {
	if err := a.fact.Validate(); err != nil {
		return contract.WorkspaceReadExecutionQualificationV2{}, err
	}
	return a.fact, nil
}

// PostActualRepositoryV2 is the only Sandbox Owner write surface. An expired
// Qualification remains exact-readable but cannot authorize another dispatch.
type PostActualRepositoryV2 interface {
	EnsureAuthorizedExecutionQualificationV2(
		context.Context,
		AuthorizedExecutionQualificationV2,
	) (contract.WorkspaceReadExecutionQualificationV2, bool, error)
	InspectWorkspaceReadExecutionQualificationExactV2(
		context.Context,
		contract.WorkspaceReadExecutionQualificationRefV2,
	) (contract.WorkspaceReadExecutionQualificationV2, error)
	InspectWorkspaceReadExecutionQualificationByOriginV2(
		context.Context,
		contract.WorkspaceReadAttemptRefV1,
	) (contract.WorkspaceReadExecutionQualificationV2, error)
	InspectWorkspaceReadTerminalExactV2(
		context.Context,
		contract.WorkspaceReadTerminalRefV2,
	) (contract.WorkspaceReadTerminalFactV2, error)
	InspectWorkspaceReadTerminalByOriginV2(
		context.Context,
		contract.WorkspaceReadAttemptRefV1,
	) (contract.WorkspaceReadTerminalFactV2, error)
}
