package kernel

import (
	"context"
	"errors"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
	ownerworkspaceread "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type WorkspaceReadActualPointRequestV1 = sandboxports.WorkspaceReadActualPointRequestV1
type WorkspaceReadActualPointBoundaryV1 = sandboxports.WorkspaceReadActualPointBoundaryV1
type WorkspaceReadActualPointErrorV1 = sandboxports.WorkspaceReadActualPointErrorV1

const (
	WorkspaceReadEffectNotStartedV1     = sandboxports.WorkspaceReadEffectNotStartedV1
	WorkspaceReadEffectStartedUnknownV1 = sandboxports.WorkspaceReadEffectStartedUnknownV1
)

var (
	NewWorkspaceReadActualPointErrorV1            = sandboxports.NewWorkspaceReadActualPointErrorV1
	NewWorkspaceReadActualPointErrorWithJournalV2 = sandboxports.NewWorkspaceReadActualPointErrorWithJournalV2
)

// workspaceReadAuthorizedOwnerStoreV2 is an internal Sandbox composition seam.
// It is intentionally absent from public ports: only the Sandbox kernel may
// submit the complete Runtime authorization that lets SQLite construct the V2
// historical Owner fact.
type workspaceReadAuthorizedOwnerStoreV2 interface {
	sandboxports.WorkspaceReadOwnerStoreV1
	sandboxports.WorkspaceReadRuntimeAttemptAdmissionReaderV2
	ReserveWorkspaceReadAuthorizedV2(
		context.Context,
		ownerworkspaceread.AuthorizedReservationV2,
	) (contract.WorkspaceReadExecutionProjectionV1, bool, error)
	TransitionWorkspaceReadAuthorizedV2(
		context.Context,
		ownerworkspaceread.AuthorizedTransitionV2,
	) (contract.WorkspaceReadExecutionProjectionV1, error)
}

type workspaceReadPublishedCommandCurrentReaderV2 interface {
	inspectWorkspaceReadPublishedCommandCurrentV2(
		context.Context,
		contract.Ref,
	) (contract.WorkspaceReadCommandV1, contract.WorkspaceReadCommandOwnerCurrentV2, error)
	InspectWorkspaceReadCommandPublicationExactV2(
		context.Context,
		contract.Ref,
	) (contract.WorkspaceReadCommandPublicationV2, error)
}

type WorkspaceReadPhysicalExecutorV1 struct {
	commands        workspaceReadPublishedCommandCurrentReaderV2
	associations    runtimeports.PreparedDomainCommandAssociationCurrentReaderV1
	workspaces      sandboxports.WorkspaceCurrentReaderV1
	sandboxCurrent  runtimeports.OperationDispatchSandboxCurrentReaderV4
	enforcement     runtimeports.OperationDispatchEnforcementGovernancePortV4
	store           sandboxports.WorkspaceReadOwnerStoreV1
	authorizedStore workspaceReadAuthorizedOwnerStoreV2
	postActual      workspaceReadPostActualRepositoryV2
	actualPoint     WorkspaceReadActualPointV2
	clock           func() time.Time
	dispatchMu      sync.Mutex
	dispatchClaims  map[string]struct{}
}

func NewWorkspaceReadPhysicalExecutorV1(commands workspaceReadPublishedCommandCurrentReaderV2, associations runtimeports.PreparedDomainCommandAssociationCurrentReaderV1, workspaces sandboxports.WorkspaceCurrentReaderV1, sandboxCurrent runtimeports.OperationDispatchSandboxCurrentReaderV4, enforcement runtimeports.OperationDispatchEnforcementGovernancePortV4, store sandboxports.WorkspaceReadOwnerStoreV1, client dataplaneadapter.Client, clock func() time.Time) (*WorkspaceReadPhysicalExecutorV1, error) {
	actualPoint, err := newWorkspaceReadActualPointAdapterV2(client)
	if err != nil {
		return nil, runtimecore.NewError(runtimecore.ErrorInvalidArgument, runtimecore.ReasonInvalidReference, "workspace read private Data Plane bridge is incomplete")
	}
	return newWorkspaceReadPhysicalExecutorV1(commands, associations, workspaces, sandboxCurrent, enforcement, store, actualPoint, clock)
}

func newWorkspaceReadPhysicalExecutorV1(commands workspaceReadPublishedCommandCurrentReaderV2, associations runtimeports.PreparedDomainCommandAssociationCurrentReaderV1, workspaces sandboxports.WorkspaceCurrentReaderV1, sandboxCurrent runtimeports.OperationDispatchSandboxCurrentReaderV4, enforcement runtimeports.OperationDispatchEnforcementGovernancePortV4, store sandboxports.WorkspaceReadOwnerStoreV1, actualPoint WorkspaceReadActualPointV1, clock func() time.Time) (*WorkspaceReadPhysicalExecutorV1, error) {
	if nilLikeWorkspaceReadInspectionV2(commands) || associations == nil || workspaces == nil || sandboxCurrent == nil || enforcement == nil || store == nil || actualPoint == nil || clock == nil {
		return nil, runtimecore.NewError(runtimecore.ErrorInvalidArgument, runtimecore.ReasonInvalidReference, "workspace read physical executor dependencies are incomplete")
	}
	authorizedStore, ok := store.(workspaceReadAuthorizedOwnerStoreV2)
	if !ok || nilLikeWorkspaceReadInspectionV2(authorizedStore) {
		return nil, runtimecore.NewError(runtimecore.ErrorInvalidArgument, runtimecore.ReasonInvalidReference, "workspace read Runtime-attempt history store is incomplete")
	}
	postActual, ok := store.(workspaceReadPostActualRepositoryV2)
	if !ok || nilLikeWorkspaceReadInspectionV2(postActual) {
		return nil, runtimecore.NewError(runtimecore.ErrorInvalidArgument, runtimecore.ReasonInvalidReference, "workspace read post-actual Owner repository is incomplete")
	}
	actualPointV2, ok := actualPoint.(WorkspaceReadActualPointV2)
	if !ok || nilLikeWorkspaceReadInspectionV2(actualPointV2) {
		return nil, runtimecore.NewError(runtimecore.ErrorInvalidArgument, runtimecore.ReasonInvalidReference, "workspace read physical execution requires the V2 qualified actual-point adapter")
	}
	return &WorkspaceReadPhysicalExecutorV1{commands: commands, associations: associations, workspaces: workspaces, sandboxCurrent: sandboxCurrent, enforcement: enforcement, store: store, authorizedStore: authorizedStore, postActual: postActual, actualPoint: actualPointV2, clock: clock, dispatchClaims: make(map[string]struct{})}, nil
}

func (e *WorkspaceReadPhysicalExecutorV1) ExecuteControlledOperationPhysicalV3(ctx context.Context, authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3) (runtimeports.ControlledOperationProviderAdmissionReceiptRefV2, error) {
	if e == nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, runtimecore.NewError(runtimecore.ErrorUnavailable, runtimecore.ReasonComponentMissing, "workspace read executor is unavailable")
	}
	if err := authorization.Validate(); err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	if string(authorization.DomainCommand.Kind) != contract.WorkspaceReadCommandKindV1 {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, runtimecore.NewError(runtimecore.ErrorForbidden, runtimecore.ReasonUnknownGovernanceCategory, "workspace read executor accepts only exact workspace.read commands")
	}
	receipt, err := workspaceReadAdmissionReceiptV1(authorization.StableKeyDigest)
	if err != nil {
		return receipt, err
	}

	// Historical recovery is deliberately before every mutable current read.
	// Once the Sandbox has durably bound this Runtime Attempt, current expiry or
	// unavailability must not hide a crossed physical boundary or cause a reread.
	var (
		historicalQualification *contract.WorkspaceReadExecutionQualificationV2
		historicalInspection    *WorkspaceReadActualPointInspectionV2
	)
	if binding, inspectErr := e.authorizedStore.InspectWorkspaceReadAdmissionForRuntimeAttemptV2(ctx, authorization.Attempt); inspectErr == nil {
		origin := binding.WorkspaceReadAttempt
		if terminal, terminalErr := e.postActual.InspectWorkspaceReadTerminalByOriginV2(ctx, origin); terminalErr == nil {
			return receipt, workspaceReadTerminalOutcomeErrorV2(terminal)
		} else if !errors.Is(terminalErr, sandboxports.ErrNotFound) {
			return receipt, terminalErr
		}
		qualification, qualificationErr := e.postActual.InspectWorkspaceReadExecutionQualificationByOriginV2(ctx, origin)
		if errors.Is(qualificationErr, sandboxports.ErrNotFound) {
			return receipt, runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read reservation exists without a durable execution Qualification; physical dispatch is forbidden")
		}
		if qualificationErr != nil {
			return receipt, errors.Join(qualificationErr, errors.New("workspace read historical Qualification lookup failed"))
		}
		historicalQualification = &qualification
		inspection, journalErr := e.actualPoint.InspectWorkspaceReadJournalV2(ctx, qualification)
		if journalErr == nil {
			journal, evidenceErr := inspection.JournalEvidence.JournalV2()
			if evidenceErr != nil || journal != inspection.Journal {
				return receipt, sandboxports.ErrConflict
			}
			if journal.State == contract.WorkspaceReadPhysicalJournalStartedV2 || inspection.Result == nil {
				class := contract.WorkspaceReadIndeterminateErrorActualPointUnknownV2
				if journal.State == contract.WorkspaceReadPhysicalJournalCompletedV2 {
					class = contract.WorkspaceReadIndeterminateErrorRecoveryUnknownV2
				}
				return receipt, e.persistWorkspaceReadIndeterminateV2(ctx, nil, qualification, inspection.JournalEvidence, class)
			}
			historicalInspection = &inspection
		} else if !workspaceReadJournalAbsentV2(journalErr) {
			return receipt, runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read exact historical journal recovery is unavailable")
		}
	} else if !errors.Is(inspectErr, sandboxports.ErrNotFound) {
		return receipt, inspectErr
	}

	association, command, commandCurrent, workspace, s1, err := e.readCurrentClosureV1(ctx, authorization)
	if err != nil {
		if historicalQualification != nil && historicalInspection != nil {
			return receipt, e.persistWorkspaceReadIndeterminateV2(ctx, nil, *historicalQualification, historicalInspection.JournalEvidence, workspaceReadS2ErrorClassV2(err))
		}
		if historicalQualification != nil {
			return receipt, runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read post-actual recovery cannot re-establish current authority; physical dispatch is forbidden")
		}
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	runtimeCurrent, err := e.readRuntimeCurrentV1(ctx, authorization, s1)
	if err != nil {
		if historicalQualification != nil && historicalInspection != nil {
			return receipt, e.persistWorkspaceReadIndeterminateV2(ctx, nil, *historicalQualification, historicalInspection.JournalEvidence, workspaceReadS2ErrorClassV2(err))
		}
		if historicalQualification != nil {
			return receipt, runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read post-actual recovery cannot re-establish Runtime current; physical dispatch is forbidden")
		}
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	if err = validateWorkspaceReadRuntimeLeaseV1(workspace.Lease, runtimeCurrent); err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	expiresNano := minWorkspaceReadExpiryV1(
		authorization.UnifiedNotAfterUnixNano,
		runtimeCurrent.ExpiresUnixNano,
		association.ExpiresUnixNano,
		command.RequestedNotAfterUnixNano,
		command.Meta.ExpiresUnixNano,
		commandCurrent.Meta.ExpiresUnixNano,
		workspace.Meta.ExpiresUnixNano,
		workspace.Lease.ExpiresUnixNano,
	)
	expires := time.Unix(0, expiresNano)
	requestDigest := command.Meta.Digest
	payloadDigest := command.SourceToolPayloadDigest
	factTime := time.Unix(0, association.CheckedUnixNano)
	admissionBinding := contract.WorkspaceReadReceiptBindingV1{
		ID: receipt.ID, Revision: uint64(receipt.Revision), Digest: string(receipt.Digest),
		StableKeyDigest: string(receipt.StableKeyDigest), CheckedUnixNano: factTime.UnixNano(), ExpiresUnixNano: expiresNano,
	}
	ttlClosure, err := contract.SealWorkspaceReadTTLClosureV1(contract.WorkspaceReadTTLClosureV1{
		UnifiedNotAfterUnixNano:                authorization.UnifiedNotAfterUnixNano,
		RuntimeEnforcementExpiresNano:          runtimeCurrent.ExpiresUnixNano,
		AssociationExpiresUnixNano:             association.ExpiresUnixNano,
		CommandRequestedNotAfterNano:           command.RequestedNotAfterUnixNano,
		CommandExpiresUnixNano:                 command.Meta.ExpiresUnixNano,
		PublishedCommandCurrentExpiresUnixNano: commandCurrent.Meta.ExpiresUnixNano,
		WorkspaceViewExpiresUnixNano:           workspace.Meta.ExpiresUnixNano,
		WorkspaceLeaseExpiresUnixNano:          workspace.Lease.ExpiresUnixNano,
	})
	if err != nil {
		return receipt, err
	}
	reservation, err := contract.SealWorkspaceReadReservationV1(contract.WorkspaceReadReservationV1{
		StableKeyDigest:     string(authorization.StableKeyDigest),
		AuthorizationDigest: string(authorization.AuthorizationDigest),
		RequestDigest:       requestDigest,
		PayloadDigest:       payloadDigest,
		Command:             command.Meta.Ref(),
		WorkspaceView:       workspace.Meta.Ref(),
		AttemptID:           "workspace-read-attempt-" + trimRuntimeDigestV1(string(authorization.StableKeyDigest)),
		TTLClosure:          ttlClosure,
	}, "workspace-read-reservation-"+trimRuntimeDigestV1(string(authorization.StableKeyDigest)), factTime, expires)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	attempt, err := contract.SealWorkspaceReadAttemptV1(contract.WorkspaceReadAttemptV1{
		StableKeyDigest:  string(authorization.StableKeyDigest),
		RequestDigest:    requestDigest,
		PayloadDigest:    payloadDigest,
		Reservation:      reservation.Meta.Ref(),
		AdmissionReceipt: admissionBinding,
		State:            contract.WorkspaceReadStartedV1,
	}, "workspace-read-attempt-"+trimRuntimeDigestV1(string(authorization.StableKeyDigest)), 1, factTime, expires)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	handoff, err := sandboxports.SealWorkspaceReadAdmissionAttemptBindingV1(sandboxports.WorkspaceReadAdmissionAttemptBindingV1{
		AdmissionReceipt:    receipt,
		Attempt:             workspaceReadAttemptRefV1(attempt),
		Command:             command.Meta.Ref(),
		AuthorizationDigest: authorization.AuthorizationDigest,
		StableKeyDigest:     authorization.StableKeyDigest,
		Association:         association.Ref,
		DomainCommand:       association.DomainCommand,
		CreatedUnixNano:     attempt.Meta.CreatedUnixNano,
		ExpiresUnixNano:     attempt.Meta.ExpiresUnixNano,
	})
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	authorizedStore := e.authorizedStore
	if authorizedStore == nil {
		authorizedStore, _ = e.store.(workspaceReadAuthorizedOwnerStoreV2)
	}
	if nilLikeWorkspaceReadInspectionV2(authorizedStore) {
		return receipt, runtimecore.NewError(runtimecore.ErrorUnavailable, runtimecore.ReasonComponentMissing, "workspace read Runtime-attempt history store is unavailable")
	}
	ownerRequest, err := ownerworkspaceread.NewAuthorizedReservationV2(reservation, attempt, handoff, authorization, s1)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	projection, created, err := authorizedStore.ReserveWorkspaceReadAuthorizedV2(ctx, ownerRequest)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	if !created {
		switch projection.Attempt.State {
		case contract.WorkspaceReadObservedV1:
			return receipt, nil
		case contract.WorkspaceReadStartedV1, contract.WorkspaceReadUnknownV1:
			// Continue only into the historical Qualification/journal recovery
			// path below. A replay never enters Prepare or physical dispatch.
		case contract.WorkspaceReadFailedV1:
			return receipt, runtimecore.NewError(runtimecore.ErrorPreconditionFailed, runtimecore.ReasonEffectStateConflict, "workspace read deterministically failed before its actual point: "+projection.Attempt.FailureDigest)
		default:
			return receipt, runtimecore.NewError(runtimecore.ErrorInternal, runtimecore.ReasonInvalidReference, "workspace read projection state is invalid")
		}
	}
	transitionAuthority, err := ownerworkspaceread.NewAuthorizedExecutionV2(
		workspaceReadAttemptRefV1(projection.Attempt), authorization, s1,
	)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	queryChecked, err := workspaceReadCurrentQueryWatermarkV2(
		association, command, commandCurrent, workspace, reservation, attempt,
		admissionBinding, runtimeCurrent, s1,
	)
	if err != nil {
		return receipt, err
	}
	currentQuery, err := workspaceReadCurrentQueryV2(authorization, association, command, commandCurrent, workspace, reservation, attempt, admissionBinding, runtimeCurrent, queryChecked, expiresNano)
	if err != nil {
		failureDigest, digestErr := contract.Digest("workspace-read-failed", struct {
			Stage string
			Cause string
		}{"exact-current-query", err.Error()})
		if digestErr != nil {
			return receipt, digestErr
		}
		if failErr := e.failWorkspaceReadAuthorizedV2(ctx, transitionAuthority, failureDigest); failErr != nil {
			return receipt, failErr
		}
		return receipt, NewWorkspaceReadActualPointErrorV1(WorkspaceReadEffectNotStartedV1, err)
	}

	publication, err := e.commands.InspectWorkspaceReadCommandPublicationExactV2(ctx, commandCurrent.Publication)
	if err != nil || publication.ValidateShape() != nil || publication.Meta.Ref() != commandCurrent.Publication || publication.Command != command.Meta.Ref() {
		if err == nil {
			err = sandboxports.ErrConflict
		}
		failureDigest, digestErr := contract.Digest("workspace-read-failed", struct{ Cause string }{err.Error()})
		if digestErr != nil {
			return receipt, digestErr
		}
		if failErr := e.failWorkspaceReadAuthorizedV2(ctx, transitionAuthority, failureDigest); failErr != nil {
			return receipt, failErr
		}
		return receipt, NewWorkspaceReadActualPointErrorV1(WorkspaceReadEffectNotStartedV1, err)
	}
	var replayQualification *contract.WorkspaceReadExecutionQualificationV2
	if !created {
		origin := workspaceReadAttemptRefV1(projection.Attempt)
		if terminal, inspectErr := e.postActual.InspectWorkspaceReadTerminalByOriginV2(ctx, origin); inspectErr == nil {
			return receipt, workspaceReadTerminalOutcomeErrorV2(terminal)
		} else if !errors.Is(inspectErr, sandboxports.ErrNotFound) {
			return receipt, inspectErr
		}
		qualification, inspectErr := e.postActual.InspectWorkspaceReadExecutionQualificationByOriginV2(ctx, origin)
		if errors.Is(inspectErr, sandboxports.ErrNotFound) {
			return receipt, runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read reservation exists without a durable execution Qualification; physical dispatch is forbidden")
		}
		if inspectErr != nil {
			return receipt, errors.Join(inspectErr, errors.New("workspace read replay Qualification lookup failed"))
		}
		if historicalQualification != nil && historicalInspection != nil {
			if !reflect.DeepEqual(*historicalQualification, qualification) {
				return receipt, sandboxports.ErrConflict
			}
			return receipt, e.recoverWorkspaceReadPostActualV2(ctx, authorization, association, command, commandCurrent, publication, workspace, reservation, attempt, admissionBinding, runtimeCurrent, currentQuery, transitionAuthority, qualification, *historicalInspection)
		}
		inspection, inspectErr := e.actualPoint.InspectWorkspaceReadJournalV2(ctx, qualification)
		if inspectErr == nil {
			return receipt, e.recoverWorkspaceReadPostActualV2(ctx, authorization, association, command, commandCurrent, publication, workspace, reservation, attempt, admissionBinding, runtimeCurrent, currentQuery, transitionAuthority, qualification, inspection)
		}
		if !workspaceReadJournalAbsentV2(inspectErr) {
			return receipt, runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read replay can only inspect the original Qualification journal")
		}
		fresh := e.clock()
		if fresh.IsZero() || fresh.Before(s1) || !fresh.Before(time.Unix(0, qualification.ExpiresUnixNano)) {
			return receipt, runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read Qualification has no durable journal and is no longer eligible for physical dispatch")
		}
		replayQualification = &qualification
	}
	actualRequest := WorkspaceReadActualPointRequestV1{
		Reservation: reservation, Command: command, Workspace: workspace,
		RuntimeCurrent: runtimeCurrent, CurrentQuery: currentQuery,
		S1CheckedUnixNano: queryChecked.UnixNano(), ExpiresUnixNano: expiresNano,
	}
	prepared, err := e.actualPoint.PrepareWorkspaceReadV2(ctx, actualRequest)
	if err != nil {
		failureDigest, digestErr := contract.Digest("workspace-read-failed", struct{ Cause string }{err.Error()})
		if digestErr != nil {
			return receipt, digestErr
		}
		if failErr := e.failWorkspaceReadAuthorizedV2(ctx, transitionAuthority, failureDigest); failErr != nil {
			return receipt, failErr
		}
		return receipt, NewWorkspaceReadActualPointErrorV1(WorkspaceReadEffectNotStartedV1, err)
	}
	binding, err := authorizedStore.InspectWorkspaceReadAdmissionForRuntimeAttemptV2(ctx, authorization.Attempt)
	if err != nil {
		return receipt, err
	}
	preparedProof, err := ownerworkspaceread.AuthorizePreparedWorkspaceReadRequestProofV2(
		prepared.ActualAttemptIDV2(), prepared.ActualRequestDigestV2(), prepared.ActualPayloadDigestV2(), prepared.ActualExpiresUnixNanoV2(),
	)
	if err != nil {
		return receipt, err
	}
	leaseDigest, err := contract.WorkspaceReadRuntimeLeaseDigestV2(workspace.Lease)
	if err != nil {
		return receipt, err
	}
	runtimeAttemptDigest, err := contract.WorkspaceReadSourceRuntimeAttemptDigestV2(authorization.Attempt)
	if err != nil {
		return receipt, err
	}
	qualificationChecked := e.clock()
	if qualificationChecked.IsZero() || qualificationChecked.Before(s1) || !qualificationChecked.Before(time.Unix(0, expiresNano)) {
		return receipt, NewWorkspaceReadActualPointErrorV1(WorkspaceReadEffectNotStartedV1, sandboxports.ErrConflict)
	}
	qualificationExpires := minWorkspaceReadExpiryV1(expiresNano, prepared.ActualExpiresUnixNanoV2())
	if replayQualification != nil {
		qualificationChecked = time.Unix(0, replayQualification.S1CheckedUnixNano)
		qualificationExpires = replayQualification.ExpiresUnixNano
	}
	qualification, err := contract.SealWorkspaceReadExecutionQualificationV2(contract.WorkspaceReadExecutionQualificationV2{
		OriginAttempt: workspaceReadAttemptRefV1(projection.Attempt), Reservation: reservation.Meta.Ref(),
		AdmissionReceipt: admissionBinding, RuntimeAdmissionReceipt: receipt,
		AdmissionAttemptBindingDigest: binding.Digest, RuntimeAttempt: authorization.Attempt,
		RuntimeAttemptDigest: runtimeAttemptDigest, AuthorizationDigest: authorization.AuthorizationDigest,
		Association: association.Ref, Command: command.Meta.Ref(), CommandPublication: publication.Meta.Ref(),
		CommandOwnerCurrent: commandCurrent.Meta.Ref(), WorkspaceView: workspace.Meta.Ref(), WorkspaceLeaseDigest: leaseDigest,
		CurrentQueryDigest: currentQuery.Digest, ExpectedRuntimeCurrentDigest: runtimeCurrent.Digest,
		ActualRequestDigest: prepared.ActualRequestDigestV2(), PayloadDigest: prepared.ActualPayloadDigestV2(),
		S1CheckedUnixNano: qualificationChecked.UnixNano(), ExpiresUnixNano: qualificationExpires,
	})
	if err != nil {
		return receipt, err
	}
	qualificationAuthority, err := ownerworkspaceread.AuthorizeExecutionQualificationV2(
		qualification, binding, reservation, attempt, publication, commandCurrent, workspace, currentQuery, runtimeCurrent, preparedProof,
	)
	if err != nil {
		return receipt, err
	}
	qualificationCreated := false
	if replayQualification != nil {
		if !reflect.DeepEqual(*replayQualification, qualification) {
			return receipt, errors.Join(sandboxports.ErrConflict, errors.New("workspace read replay Qualification closure drifted"))
		}
		qualification = *replayQualification
	} else {
		qualification, qualificationCreated, err = e.postActual.EnsureAuthorizedExecutionQualificationV2(ctx, qualificationAuthority)
		if err != nil {
			return receipt, errors.Join(err, errors.New("workspace read Qualification persistence failed"))
		}
	}
	if terminal, inspectErr := e.postActual.InspectWorkspaceReadTerminalByOriginV2(ctx, qualification.OriginAttempt); inspectErr == nil {
		return receipt, workspaceReadTerminalOutcomeErrorV2(terminal)
	} else if !errors.Is(inspectErr, sandboxports.ErrNotFound) {
		return receipt, inspectErr
	}
	if !qualificationCreated {
		inspection, inspectErr := e.actualPoint.InspectWorkspaceReadJournalV2(ctx, qualification)
		if inspectErr == nil {
			return receipt, e.recoverWorkspaceReadPostActualV2(ctx, authorization, association, command, commandCurrent, publication, workspace, reservation, attempt, admissionBinding, runtimeCurrent, currentQuery, transitionAuthority, qualification, inspection)
		}
		if !workspaceReadJournalAbsentV2(inspectErr) {
			return receipt, runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read exact journal recovery is unavailable")
		}
		fresh := e.clock()
		if fresh.IsZero() || fresh.Before(qualificationChecked) || !fresh.Before(time.Unix(0, qualification.ExpiresUnixNano)) {
			return receipt, runtimecore.NewError(runtimecore.ErrorPreconditionFailed, runtimecore.ReasonBindingExpired, "workspace read Qualification expired before the physical dispatch")
		}
	}
	if !e.claimWorkspaceReadPhysicalDispatchV2(qualification.Ref) {
		return receipt, runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read Qualification already has an in-process physical dispatcher; inspect the original journal")
	}
	dispatchNow := e.clock()
	if dispatchNow.IsZero() || dispatchNow.Before(qualificationChecked) || !dispatchNow.Before(time.Unix(0, qualification.ExpiresUnixNano)) {
		return receipt, NewWorkspaceReadActualPointErrorV1(WorkspaceReadEffectNotStartedV1, runtimecore.NewError(runtimecore.ErrorPreconditionFailed, runtimecore.ReasonBindingExpired, "workspace read Qualification expired before the physical dispatch"))
	}

	result, readErr := e.actualPoint.DispatchPreparedWorkspaceReadV2(ctx, prepared)
	if readErr != nil {
		inspection, inspectErr := e.actualPoint.InspectWorkspaceReadJournalV2(ctx, qualification)
		if inspectErr == nil {
			return receipt, e.recoverWorkspaceReadPostActualV2(ctx, authorization, association, command, commandCurrent, publication, workspace, reservation, attempt, admissionBinding, runtimeCurrent, currentQuery, transitionAuthority, qualification, inspection)
		}
		var actualPointError *WorkspaceReadActualPointErrorV1
		if errors.As(readErr, &actualPointError) && actualPointError.Boundary == WorkspaceReadEffectNotStartedV1 && workspaceReadJournalAbsentV2(inspectErr) {
			failureDigest, digestErr := contract.Digest("workspace-read-failed", struct{ Cause string }{readErr.Error()})
			if digestErr != nil {
				return receipt, digestErr
			}
			if failErr := e.failWorkspaceReadAuthorizedV2(ctx, transitionAuthority, failureDigest); failErr != nil {
				return receipt, failErr
			}
			return receipt, readErr
		}
		return receipt, runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read crossed or may have crossed its actual point; inspect the exact Qualification and journal")
	}
	inspection := WorkspaceReadActualPointInspectionV2{Journal: result.Journal, JournalEvidence: result.JournalEvidence, Result: &result}
	return receipt, e.recoverWorkspaceReadPostActualV2(ctx, authorization, association, command, commandCurrent, publication, workspace, reservation, attempt, admissionBinding, runtimeCurrent, currentQuery, transitionAuthority, qualification, inspection)
}

func (e *WorkspaceReadPhysicalExecutorV1) claimWorkspaceReadPhysicalDispatchV2(ref contract.WorkspaceReadExecutionQualificationRefV2) bool {
	if e == nil || ref.Validate() != nil {
		return false
	}
	e.dispatchMu.Lock()
	defer e.dispatchMu.Unlock()
	if e.dispatchClaims == nil {
		e.dispatchClaims = make(map[string]struct{})
	}
	key := ref.ID + "/" + ref.Digest
	if _, exists := e.dispatchClaims[key]; exists {
		return false
	}
	e.dispatchClaims[key] = struct{}{}
	return true
}

func workspaceReadTerminalOutcomeErrorV2(terminal contract.WorkspaceReadTerminalFactV2) error {
	if err := terminal.Validate(); err != nil {
		return sandboxports.ErrConflict
	}
	switch terminal.Outcome {
	case contract.WorkspaceReadTerminalObservedV2:
		return nil
	case contract.WorkspaceReadTerminalIndeterminateV2:
		return runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read has a durable indeterminate post-actual terminal; inspect the original terminal")
	default:
		return sandboxports.ErrConflict
	}
}

func workspaceReadJournalAbsentV2(err error) bool {
	return errors.Is(err, ErrWorkspaceReadPhysicalJournalNotFoundV2)
}

func (e *WorkspaceReadPhysicalExecutorV1) recoverWorkspaceReadPostActualV2(
	ctx context.Context,
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
	association runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1,
	command contract.WorkspaceReadCommandV1,
	commandCurrent contract.WorkspaceReadCommandOwnerCurrentV2,
	publication contract.WorkspaceReadCommandPublicationV2,
	workspace contract.WorkspaceView,
	reservation contract.WorkspaceReadReservationV1,
	attempt contract.WorkspaceReadAttemptV1,
	admissionBinding contract.WorkspaceReadReceiptBindingV1,
	runtimeCurrent runtimeports.CurrentOperationDispatchEnforcementV4,
	currentQuery sandboxports.WorkspaceReadCurrentQueryV2,
	transitionAuthority ownerworkspaceread.AuthorizedExecutionV2,
	qualification contract.WorkspaceReadExecutionQualificationV2,
	inspection WorkspaceReadActualPointInspectionV2,
) error {
	if err := qualification.Validate(); err != nil {
		return sandboxports.ErrConflict
	}
	journal, err := inspection.JournalEvidence.JournalV2()
	if err != nil || journal != inspection.Journal || journal.AttemptID != qualification.RuntimeAttempt.AttemptID || journal.RequestDigest != qualification.ActualRequestDigest || journal.PayloadDigest != qualification.PayloadDigest {
		return sandboxports.ErrConflict
	}
	if terminal, inspectErr := e.postActual.InspectWorkspaceReadTerminalByOriginV2(ctx, qualification.OriginAttempt); inspectErr == nil {
		return workspaceReadTerminalOutcomeErrorV2(terminal)
	} else if !errors.Is(inspectErr, sandboxports.ErrNotFound) {
		return inspectErr
	}
	if journal.State == contract.WorkspaceReadPhysicalJournalStartedV2 || inspection.Result == nil {
		class := contract.WorkspaceReadIndeterminateErrorActualPointUnknownV2
		if journal.State == contract.WorkspaceReadPhysicalJournalCompletedV2 {
			class = contract.WorkspaceReadIndeterminateErrorRecoveryUnknownV2
		}
		return e.persistWorkspaceReadIndeterminateV2(ctx, &transitionAuthority, qualification, inspection.JournalEvidence, class)
	}

	result := *inspection.Result
	s2Association, s2Command, s2CommandCurrent, s2Workspace, s2Now, s2Err := e.readCurrentClosureV1(ctx, authorization)
	var s2Runtime runtimeports.CurrentOperationDispatchEnforcementV4
	if s2Err == nil {
		var runtimeErr error
		s2Runtime, runtimeErr = e.readRuntimeCurrentV1(ctx, authorization, s2Now)
		if runtimeErr != nil {
			s2Err = runtimeErr
		} else {
			s2Publication, publicationErr := e.commands.InspectWorkspaceReadCommandPublicationExactV2(ctx, qualification.CommandPublication)
			leaseDigest, leaseErr := contract.WorkspaceReadRuntimeLeaseDigestV2(s2Workspace.Lease)
			s2Query, queryErr := workspaceReadCurrentQueryV2(authorization, s2Association, s2Command, s2CommandCurrent, s2Workspace, reservation, attempt, admissionBinding, s2Runtime, time.Unix(0, currentQuery.Base.CheckedUnixNano), currentQuery.Base.ExpiresUnixNano)
			if publicationErr != nil {
				s2Err = publicationErr
			} else if queryErr != nil {
				s2Err = queryErr
			} else if currentErr := s2Query.ValidateCurrent(s2Now); currentErr != nil {
				s2Err = currentErr
			} else if leaseErr != nil {
				s2Err = errors.New("workspace read S2 lease digest failed")
			} else if !reflect.DeepEqual(s2Association, association) {
				s2Err = errors.New("workspace read S2 association drifted")
			} else if !reflect.DeepEqual(s2Command, command) {
				s2Err = errors.New("workspace read S2 command drifted")
			} else if !reflect.DeepEqual(s2CommandCurrent, commandCurrent) {
				s2Err = errors.New("workspace read S2 command current drifted")
			} else if !reflect.DeepEqual(s2Publication, publication) {
				s2Err = errors.New("workspace read S2 publication drifted")
			} else if !reflect.DeepEqual(s2Workspace, workspace) {
				s2Err = errors.New("workspace read S2 workspace drifted")
			} else if !reflect.DeepEqual(s2Runtime, runtimeCurrent) {
				s2Err = errors.New("workspace read S2 Runtime current drifted")
			} else if !reflect.DeepEqual(s2Query, currentQuery) {
				s2Err = errors.New("workspace read S2 current query drifted")
			} else if s2Runtime.Digest != qualification.ExpectedRuntimeCurrentDigest || leaseDigest != qualification.WorkspaceLeaseDigest || s2Query.Digest != qualification.CurrentQueryDigest {
				s2Err = errors.New("workspace read S2 qualification digest closure drifted")
			} else if !s2Now.Before(time.Unix(0, qualification.ExpiresUnixNano)) {
				s2Err = runtimecore.NewError(runtimecore.ErrorPreconditionFailed, runtimecore.ReasonBindingExpired, "workspace read Qualification expired before outcome S2")
			} else if validateErr := validateWorkspaceReadRuntimeLeaseV1(s2Workspace.Lease, s2Runtime); validateErr != nil {
				s2Err = validateErr
			} else if validateErr := validateWorkspaceReadActualPointResultV1(result, reservation, command, workspace, s2Now); validateErr != nil {
				s2Err = validateErr
			}
		}
	}
	if s2Err != nil {
		return e.persistWorkspaceReadIndeterminateV2(ctx, &transitionAuthority, qualification, inspection.JournalEvidence, workspaceReadS2ErrorClassV2(s2Err))
	}
	if s2Now.UnixNano() < journal.RecordedUnixNano || s2Now.UnixNano() < result.ProviderReceipt.CheckedUnixNano {
		return e.persistWorkspaceReadIndeterminateV2(ctx, &transitionAuthority, qualification, inspection.JournalEvidence, contract.WorkspaceReadIndeterminateErrorS2DriftedV2)
	}
	observationExpiry := minWorkspaceReadExpiryV1(qualification.ExpiresUnixNano, attempt.Meta.ExpiresUnixNano, reservation.Meta.ExpiresUnixNano, admissionBinding.ExpiresUnixNano, result.ProviderReceipt.ExpiresUnixNano)
	if !s2Now.Before(time.Unix(0, observationExpiry)) {
		return e.persistWorkspaceReadIndeterminateV2(ctx, &transitionAuthority, qualification, inspection.JournalEvidence, contract.WorkspaceReadIndeterminateErrorS2ExpiredV2)
	}
	observation, err := contract.SealWorkspaceReadObservationV1(contract.WorkspaceReadObservationV1{
		Reservation: reservation.Meta.Ref(), Command: command.Meta.Ref(), WorkspaceView: workspace.Meta.Ref(), File: result.File,
		RelativePath: command.RelativePath, StartByte: result.StartByte, ReturnedBytes: result.ReturnedBytes,
		TotalBytes: result.TotalBytes, Complete: result.Complete, Content: result.Content, ContentDigest: result.ContentDigest,
		S1CheckedUnixNano: qualification.S1CheckedUnixNano, S2CheckedUnixNano: s2Now.UnixNano(),
		AdmissionReceipt: admissionBinding, ProviderReceipt: result.ProviderReceipt,
	}, "workspace-read-observation-"+trimRuntimeDigestV1(qualification.ActualRequestDigest), s2Now, time.Unix(0, observationExpiry))
	if err != nil {
		return e.persistWorkspaceReadIndeterminateV2(ctx, &transitionAuthority, qualification, inspection.JournalEvidence, contract.WorkspaceReadIndeterminateErrorS2DriftedV2)
	}
	transition, err := transitionAuthority.Observed(observation, s2Now)
	if err == nil {
		_, err = e.authorizedStore.TransitionWorkspaceReadAuthorizedV2(ctx, transition)
	}
	if err != nil {
		stored, inspectErr := e.store.InspectBoundedWorkspaceReadV1(ctx, qualification.OriginAttempt)
		if inspectErr != nil || stored.Attempt.State != contract.WorkspaceReadObservedV1 || stored.Observation == nil || stored.Observation.Meta.Ref() != observation.Meta.Ref() || !reflect.DeepEqual(*stored.Observation, observation) {
			return e.persistWorkspaceReadIndeterminateV2(ctx, &transitionAuthority, qualification, inspection.JournalEvidence, contract.WorkspaceReadIndeterminateErrorRecoveryUnknownV2)
		}
	}
	s2Authority, err := authorizeWorkspaceReadOutcomeS2V2(qualification, s2Association, publication, s2CommandCurrent, s2Workspace, s2Runtime, inspection.JournalEvidence, observation.Meta.Ref(), result.ProviderReceipt, s2Now.UnixNano())
	if err != nil {
		return e.persistWorkspaceReadIndeterminateV2(ctx, &transitionAuthority, qualification, inspection.JournalEvidence, contract.WorkspaceReadIndeterminateErrorS2DriftedV2)
	}
	recorded := e.clock()
	if recorded.IsZero() || recorded.Before(s2Now) {
		return e.persistWorkspaceReadIndeterminateV2(ctx, &transitionAuthority, qualification, inspection.JournalEvidence, contract.WorkspaceReadIndeterminateErrorRecoveryUnknownV2)
	}
	terminalAuthority, err := buildWorkspaceReadObservedTerminalV2(qualification, inspection.JournalEvidence, s2Authority, s2Now.UnixNano(), recorded.UnixNano())
	if err != nil {
		return err
	}
	terminal, _, err := e.postActual.CreateOrInspectKernelTerminalV2(ctx, terminalAuthority)
	if err != nil {
		if recovered, inspectErr := e.postActual.InspectWorkspaceReadTerminalByOriginV2(ctx, qualification.OriginAttempt); inspectErr == nil {
			return workspaceReadTerminalOutcomeErrorV2(recovered)
		}
		return runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read terminal commit outcome is unknown")
	}
	return workspaceReadTerminalOutcomeErrorV2(terminal)
}

func workspaceReadS2ErrorClassV2(err error) contract.WorkspaceReadIndeterminateErrorClassV2 {
	if runtimecore.HasReason(err, runtimecore.ReasonBindingExpired) || runtimecore.HasReason(err, runtimecore.ReasonCapabilityExpired) || runtimecore.HasReason(err, runtimecore.ReasonDispatchPermitExpired) {
		return contract.WorkspaceReadIndeterminateErrorS2ExpiredV2
	}
	if runtimecore.HasCategory(err, runtimecore.ErrorUnavailable) {
		return contract.WorkspaceReadIndeterminateErrorS2UnavailableV2
	}
	return contract.WorkspaceReadIndeterminateErrorS2DriftedV2
}

func (e *WorkspaceReadPhysicalExecutorV1) persistWorkspaceReadIndeterminateV2(ctx context.Context, transitionAuthority *ownerworkspaceread.AuthorizedExecutionV2, qualification contract.WorkspaceReadExecutionQualificationV2, evidence workspaceReadPhysicalJournalEvidenceV2, class contract.WorkspaceReadIndeterminateErrorClassV2) error {
	journal, err := evidence.JournalV2()
	if err != nil {
		return sandboxports.ErrConflict
	}
	errorDigest, err := contract.Digest("workspace-read-post-actual-indeterminate-v2", struct {
		Qualification contract.WorkspaceReadExecutionQualificationRefV2 `json:"qualification"`
		Journal       contract.WorkspaceReadPhysicalJournalRefV2        `json:"journal"`
		Class         contract.WorkspaceReadIndeterminateErrorClassV2   `json:"class"`
	}{qualification.Ref, journal, class})
	if err != nil {
		return err
	}
	checked := e.clock()
	if checked.IsZero() || checked.UnixNano() < journal.RecordedUnixNano {
		checked = time.Unix(0, journal.RecordedUnixNano)
	}
	unknown, err := authorizeWorkspaceReadIndeterminateV2(qualification, evidence, class, errorDigest, checked.UnixNano())
	if err != nil {
		return err
	}
	recorded := e.clock()
	if recorded.IsZero() || recorded.Before(checked) {
		recorded = checked
	}
	terminalAuthority, err := buildWorkspaceReadIndeterminateTerminalV2(qualification, evidence, unknown, checked.UnixNano(), recorded.UnixNano())
	if err != nil {
		return err
	}
	terminal, _, createErr := e.postActual.CreateOrInspectKernelTerminalV2(ctx, terminalAuthority)
	if createErr != nil {
		if recovered, inspectErr := e.postActual.InspectWorkspaceReadTerminalByOriginV2(ctx, qualification.OriginAttempt); inspectErr == nil {
			return workspaceReadTerminalOutcomeErrorV2(recovered)
		}
		return runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read indeterminate terminal commit outcome is unknown")
	}
	if terminal.Outcome != contract.WorkspaceReadTerminalIndeterminateV2 {
		return sandboxports.ErrConflict
	}
	// V20 is authoritative. The V1 transition is compatibility-only and cannot
	// override or delay the durable terminal.
	if transitionAuthority != nil {
		if transition, transitionErr := transitionAuthority.Unknown(errorDigest, checked); transitionErr == nil {
			_, _ = e.authorizedStore.TransitionWorkspaceReadAuthorizedV2(ctx, transition)
		}
	}
	return runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read post-actual outcome is indeterminate ("+string(class)+"); inspect the original terminal")
}

// workspaceReadCurrentQueryWatermarkV2 derives request identity only from the
// authoritative source facts. The caller's wall clock is a currentness gate,
// not part of the replay identity; otherwise a crash would change the exact
// Data Plane request even when every Owner fact stayed unchanged.
func workspaceReadCurrentQueryWatermarkV2(
	association runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1,
	command contract.WorkspaceReadCommandV1,
	ownerCurrent contract.WorkspaceReadCommandOwnerCurrentV2,
	workspace contract.WorkspaceView,
	reservation contract.WorkspaceReadReservationV1,
	attempt contract.WorkspaceReadAttemptV1,
	admission contract.WorkspaceReadReceiptBindingV1,
	runtimeCurrent runtimeports.CurrentOperationDispatchEnforcementV4,
	currentNow time.Time,
) (time.Time, error) {
	watermark := int64(0)
	for _, value := range []int64{
		association.CheckedUnixNano,
		command.Meta.UpdatedUnixNano,
		ownerCurrent.Meta.UpdatedUnixNano,
		ownerCurrent.CheckedUnixNano,
		workspace.Meta.UpdatedUnixNano,
		reservation.Meta.UpdatedUnixNano,
		attempt.Meta.UpdatedUnixNano,
		admission.CheckedUnixNano,
		runtimeCurrent.CheckedUnixNano,
	} {
		if value <= 0 {
			return time.Time{}, sandboxports.ErrConflict
		}
		if value > watermark {
			watermark = value
		}
	}
	checked := time.Unix(0, watermark)
	if currentNow.IsZero() || checked.After(currentNow) {
		return time.Time{}, runtimecore.NewError(runtimecore.ErrorPreconditionFailed, runtimecore.ReasonClockRegression, "workspace read request watermark is ahead of the current owner clock")
	}
	return checked, nil
}

func workspaceReadCurrentQueryV2(
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
	association runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1,
	command contract.WorkspaceReadCommandV1,
	commandCurrent contract.WorkspaceReadCommandOwnerCurrentV2,
	workspace contract.WorkspaceView,
	reservation contract.WorkspaceReadReservationV1,
	attempt contract.WorkspaceReadAttemptV1,
	admission contract.WorkspaceReadReceiptBindingV1,
	current runtimeports.CurrentOperationDispatchEnforcementV4,
	checked time.Time,
	expiresUnixNano int64,
) (sandboxports.WorkspaceReadCurrentQueryV2, error) {
	if checked.IsZero() || expiresUnixNano <= checked.UnixNano() {
		return sandboxports.WorkspaceReadCurrentQueryV2{}, runtimecore.NewError(runtimecore.ErrorPreconditionFailed, runtimecore.ReasonBindingExpired, "workspace read exact current query TTL is invalid")
	}
	base, err := sandboxports.SealWorkspaceReadCurrentQueryV1(sandboxports.WorkspaceReadCurrentQueryV1{
		RuntimeInspect: runtimeports.InspectCurrentOperationDispatchEnforcementRequestV4{
			Inspect: runtimeports.InspectOperationDispatchEnforcementRequestV4{
				Operation: authorization.Operation,
				EffectID:  authorization.Attempt.EffectID,
				PermitID:  current.Phase.PermitID,
				Phase:     runtimeports.OperationDispatchEnforcementExecuteV4,
			},
			PermitDigest:            current.Phase.PermitDigest,
			AdmissionDigest:         current.Phase.AdmissionDigest,
			ReviewAuthorization:     current.Phase.ReviewAuthorization,
			SandboxAttempt:          current.Phase.SandboxAttempt,
			SandboxProjectionDigest: current.Sandbox.ProjectionDigest,
		},
		Authorization:           authorization,
		StableKeyDigest:         authorization.StableKeyDigest,
		AuthorizationDigest:     authorization.AuthorizationDigest,
		Association:             association.Ref,
		DomainCommand:           association.DomainCommand,
		Command:                 command.Meta.Ref(),
		PublishedCommandCurrent: func() *contract.Ref { ref := commandCurrent.Meta.Ref(); return &ref }(),
		WorkspaceView:           workspace.Meta.Ref(),
		FileScopeDigest:         command.FileScopeDigest,
		RelativePath:            command.RelativePath,
		CheckedUnixNano:         checked.UnixNano(),
		ExpiresUnixNano:         expiresUnixNano,
	})
	if err != nil {
		return sandboxports.WorkspaceReadCurrentQueryV2{}, err
	}
	sealed, err := sandboxports.SealWorkspaceReadCurrentQueryV2(sandboxports.WorkspaceReadCurrentQueryV2{
		Base:             base,
		Reservation:      reservation.Meta.Ref(),
		Attempt:          workspaceReadAttemptRefV1(attempt),
		AdmissionReceipt: admission,
	})
	if err != nil {
		return sandboxports.WorkspaceReadCurrentQueryV2{}, err
	}
	if err := sealed.ValidateCurrent(checked); err != nil {
		return sandboxports.WorkspaceReadCurrentQueryV2{}, err
	}
	return sealed, nil
}

func (e *WorkspaceReadPhysicalExecutorV1) readRuntimeCurrentV1(ctx context.Context, authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3, now time.Time) (runtimeports.CurrentOperationDispatchEnforcementV4, error) {
	sandboxCurrent, err := e.sandboxCurrent.InspectOperationDispatchSandboxCurrentV4(ctx, authorization.Operation, authorization.Attempt.EffectID, authorization.ExecuteEnforcement.SandboxAttempt)
	if err != nil {
		return runtimeports.CurrentOperationDispatchEnforcementV4{}, err
	}
	if err = sandboxCurrent.ValidateCurrent(authorization.Operation, authorization.Attempt.EffectID, authorization.Attempt.IntentRevision, authorization.Attempt.IntentDigest, authorization.Attempt.AttemptID, authorization.Provider, now); err != nil {
		return runtimeports.CurrentOperationDispatchEnforcementV4{}, err
	}
	current, err := e.enforcement.InspectCurrentOperationDispatchEnforcementV4(ctx, runtimeports.InspectCurrentOperationDispatchEnforcementRequestV4{
		Inspect: runtimeports.InspectOperationDispatchEnforcementRequestV4{
			Operation: authorization.Operation,
			EffectID:  authorization.Attempt.EffectID,
			PermitID:  authorization.ExecuteEnforcement.PermitID,
			Phase:     runtimeports.OperationDispatchEnforcementExecuteV4,
		},
		PermitDigest:            authorization.ExecuteEnforcement.PermitDigest,
		AdmissionDigest:         authorization.ExecuteEnforcement.AdmissionDigest,
		ReviewAuthorization:     authorization.ExecuteEnforcement.ReviewAuthorization,
		SandboxAttempt:          authorization.ExecuteEnforcement.SandboxAttempt,
		SandboxProjectionDigest: sandboxCurrent.ProjectionDigest,
	})
	if err != nil {
		return runtimeports.CurrentOperationDispatchEnforcementV4{}, err
	}
	if err = current.Validate(); err != nil || !now.Before(time.Unix(0, current.ExpiresUnixNano)) || current.Sandbox.OperationDigest != authorization.OperationDigest || current.Sandbox.Attempt != authorization.ExecuteEnforcement.SandboxAttempt || current.Sandbox.ProviderBinding != authorization.Provider || current.Phase != authorization.ExecuteEnforcement {
		if err == nil {
			err = runtimecore.NewError(runtimecore.ErrorConflict, runtimecore.ReasonBindingDrift, "workspace read Runtime current differs from physical authorization")
		}
		return runtimeports.CurrentOperationDispatchEnforcementV4{}, err
	}
	return current, nil
}

func (e *WorkspaceReadPhysicalExecutorV1) InspectBoundedWorkspaceReadV1(ctx context.Context, ref contract.WorkspaceReadAttemptRefV1) (contract.WorkspaceReadExecutionProjectionV1, error) {
	if e == nil || e.store == nil {
		return contract.WorkspaceReadExecutionProjectionV1{}, runtimecore.NewError(runtimecore.ErrorUnavailable, runtimecore.ReasonComponentMissing, "workspace read Inspect is unavailable")
	}
	return e.store.InspectBoundedWorkspaceReadV1(ctx, ref)
}

func (e *WorkspaceReadPhysicalExecutorV1) InspectBoundedWorkspaceReadV2(ctx context.Context, ref contract.WorkspaceReadAttemptRefV1) (sandboxports.WorkspaceReadInspectionEnvelopeV2, error) {
	if e == nil || e.store == nil {
		return sandboxports.WorkspaceReadInspectionEnvelopeV2{}, runtimecore.NewError(runtimecore.ErrorUnavailable, runtimecore.ReasonComponentMissing, "workspace read exact Inspect v2 is unavailable")
	}
	reader, ok := e.store.(sandboxports.WorkspaceReadInspectionReaderV2)
	if !ok || nilLikeWorkspaceReadInspectionV2(reader) {
		return sandboxports.WorkspaceReadInspectionEnvelopeV2{}, runtimecore.NewError(runtimecore.ErrorUnavailable, runtimecore.ReasonComponentMissing, "workspace read exact Inspect v2 is unavailable")
	}
	return reader.InspectBoundedWorkspaceReadV2(ctx, ref)
}

func (e *WorkspaceReadPhysicalExecutorV1) InspectWorkspaceReadCommandExactV1(ctx context.Context, ref contract.Ref) (contract.WorkspaceReadCommandV1, error) {
	if e == nil || e.store == nil {
		return contract.WorkspaceReadCommandV1{}, runtimecore.NewError(runtimecore.ErrorUnavailable, runtimecore.ReasonComponentMissing, "workspace read exact Command Inspect is unavailable")
	}
	reader, ok := e.store.(sandboxports.WorkspaceReadCommandExactReaderV1)
	if !ok || nilLikeWorkspaceReadInspectionV2(reader) {
		return contract.WorkspaceReadCommandV1{}, runtimecore.NewError(runtimecore.ErrorUnavailable, runtimecore.ReasonComponentMissing, "workspace read exact Command Inspect is unavailable")
	}
	return reader.InspectWorkspaceReadCommandExactV1(ctx, ref)
}

func (e *WorkspaceReadPhysicalExecutorV1) InspectWorkspaceReadAttemptForAdmissionV1(ctx context.Context, receipt runtimeports.ControlledOperationProviderAdmissionReceiptRefV2) (sandboxports.WorkspaceReadAdmissionAttemptBindingV1, error) {
	if e == nil || e.store == nil {
		return sandboxports.WorkspaceReadAdmissionAttemptBindingV1{}, runtimecore.NewError(runtimecore.ErrorUnavailable, runtimecore.ReasonComponentMissing, "workspace read admission handoff Inspect is unavailable")
	}
	return e.store.InspectWorkspaceReadAttemptForAdmissionV1(ctx, receipt)
}

func (e *WorkspaceReadPhysicalExecutorV1) InspectWorkspaceReadAdmissionForRuntimeAttemptV2(ctx context.Context, attempt runtimeports.OperationDispatchAttemptRefV3) (sandboxports.WorkspaceReadAdmissionAttemptBindingV2, error) {
	if e == nil || e.store == nil {
		return sandboxports.WorkspaceReadAdmissionAttemptBindingV2{}, runtimecore.NewError(runtimecore.ErrorUnavailable, runtimecore.ReasonComponentMissing, "workspace read Runtime-attempt Inspect is unavailable")
	}
	reader, ok := e.store.(sandboxports.WorkspaceReadRuntimeAttemptAdmissionReaderV2)
	if !ok || nilLikeWorkspaceReadInspectionV2(reader) {
		return sandboxports.WorkspaceReadAdmissionAttemptBindingV2{}, runtimecore.NewError(runtimecore.ErrorUnavailable, runtimecore.ReasonComponentMissing, "workspace read Runtime-attempt reader is unavailable")
	}
	return reader.InspectWorkspaceReadAdmissionForRuntimeAttemptV2(ctx, attempt)
}

func (e *WorkspaceReadPhysicalExecutorV1) readCurrentClosureV1(ctx context.Context, authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3) (runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1, contract.WorkspaceReadCommandV1, contract.WorkspaceReadCommandOwnerCurrentV2, contract.WorkspaceView, time.Time, error) {
	now := e.clock()
	if err := authorization.ValidateCurrent(now); err != nil {
		return runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1{}, contract.WorkspaceReadCommandV1{}, contract.WorkspaceReadCommandOwnerCurrentV2{}, contract.WorkspaceView{}, now, err
	}
	association, err := e.associations.InspectCurrentPreparedDomainCommandAssociationV1(ctx, authorization.Association)
	if err != nil || association.ValidateCurrent(authorization.Association, now) != nil {
		if err == nil {
			err = association.ValidateCurrent(authorization.Association, now)
		}
		return association, contract.WorkspaceReadCommandV1{}, contract.WorkspaceReadCommandOwnerCurrentV2{}, contract.WorkspaceView{}, now, err
	}
	if association.DomainCommand != authorization.DomainCommand || association.Operation != authorization.Operation || association.Prepared != authorization.Prepared || association.Attempt != authorization.Attempt || association.Provider != authorization.Provider {
		return association, contract.WorkspaceReadCommandV1{}, contract.WorkspaceReadCommandOwnerCurrentV2{}, contract.WorkspaceView{}, now, runtimecore.NewError(runtimecore.ErrorConflict, runtimecore.ReasonBindingDrift, "workspace read Association drifted from authorization")
	}
	commandDigest, err := runtimeWorkspaceReadDigestToSandboxV1(authorization.DomainCommand.Digest)
	if err != nil {
		return runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1{}, contract.WorkspaceReadCommandV1{}, contract.WorkspaceReadCommandOwnerCurrentV2{}, contract.WorkspaceView{}, now, err
	}
	commandRef := contract.Ref{ID: authorization.DomainCommand.ID, Revision: uint64(authorization.DomainCommand.Revision), Digest: commandDigest}
	command, commandCurrent, err := e.commands.inspectWorkspaceReadPublishedCommandCurrentV2(ctx, commandRef)
	if err != nil {
		return association, command, commandCurrent, contract.WorkspaceView{}, now, err
	}
	if err = validateWorkspaceReadCommandAuthorizationV1(command, association, authorization); err != nil || command.ValidateCurrent(now) != nil || commandCurrent.ValidateCurrent(now) != nil || commandCurrent.Command != commandRef {
		if err == nil {
			err = command.ValidateCurrent(now)
			if err == nil {
				err = commandCurrent.ValidateCurrent(now)
			}
			if err == nil && commandCurrent.Command != commandRef {
				err = sandboxports.ErrConflict
			}
		}
		return association, command, commandCurrent, contract.WorkspaceView{}, now, err
	}
	workspace, err := e.workspaces.InspectWorkspaceViewCurrentV1(ctx, command.WorkspaceView)
	if err != nil || workspace.ValidateCurrent(now) != nil {
		if err == nil {
			err = workspace.ValidateCurrent(now)
		}
		return association, command, commandCurrent, workspace, now, err
	}
	if !contract.SameRef(workspace.Meta.Ref(), command.WorkspaceView) || workspace.FileScopeDigest != command.FileScopeDigest || !workspaceReadAllowedV1(workspace, command.RelativePath) {
		return association, command, commandCurrent, workspace, now, runtimecore.NewError(runtimecore.ErrorForbidden, runtimecore.ReasonEvidenceScopeConflict, "workspace read path is outside the exact read scope")
	}
	finalCommand, finalCommandCurrent, err := e.commands.inspectWorkspaceReadPublishedCommandCurrentV2(ctx, commandRef)
	if err != nil {
		return association, command, commandCurrent, workspace, now, err
	}
	if !reflect.DeepEqual(finalCommand, command) || finalCommandCurrent.Meta.Ref() != commandCurrent.Meta.Ref() {
		return association, command, commandCurrent, workspace, now, sandboxports.ErrConflict
	}
	command = finalCommand
	commandCurrent = finalCommandCurrent
	finalNow := e.clock()
	if finalNow.IsZero() || finalNow.Before(now) {
		return association, command, commandCurrent, workspace, finalNow, sandboxports.ErrConflict
	}
	if err = authorization.ValidateCurrent(finalNow); err != nil {
		return association, command, commandCurrent, workspace, finalNow, err
	}
	if err = association.ValidateCurrent(authorization.Association, finalNow); err != nil {
		return association, command, commandCurrent, workspace, finalNow, err
	}
	if err = command.ValidateCurrent(finalNow); err != nil {
		return association, command, commandCurrent, workspace, finalNow, err
	}
	if err = commandCurrent.ValidateCurrent(finalNow); err != nil {
		return association, command, commandCurrent, workspace, finalNow, err
	}
	if err = workspace.ValidateCurrent(finalNow); err != nil {
		return association, command, commandCurrent, workspace, finalNow, err
	}
	return association, command, commandCurrent, workspace, finalNow, nil
}

func (e *WorkspaceReadPhysicalExecutorV1) markWorkspaceReadUnknownV1(ctx context.Context, authority ownerworkspaceread.AuthorizedExecutionV2, stage string, cause error) error {
	digest, digestErr := contract.Digest("workspace-read-indeterminate", struct{ Stage, Cause string }{stage, cause.Error()})
	if digestErr == nil {
		if transition, transitionErr := authority.Unknown(digest, e.clock()); transitionErr == nil {
			_, _ = e.authorizedStore.TransitionWorkspaceReadAuthorizedV2(ctx, transition)
		}
	}
	return runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read crossed its actual point; inspect the original attempt")
}

func (e *WorkspaceReadPhysicalExecutorV1) failWorkspaceReadAuthorizedV2(ctx context.Context, authority ownerworkspaceread.AuthorizedExecutionV2, failure string) error {
	transition, err := authority.Failed(failure, e.clock())
	if err != nil {
		return err
	}
	_, err = e.authorizedStore.TransitionWorkspaceReadAuthorizedV2(ctx, transition)
	return err
}

func validateWorkspaceReadCommandAuthorizationV1(c contract.WorkspaceReadCommandV1, p runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1, a runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3) error {
	runtimeDigest, err := sandboxWorkspaceReadDigestToRuntimeV1(c.Meta.Digest)
	if err != nil {
		return err
	}
	ref := runtimeports.OperationDomainCommandRefV1{Owner: a.DomainCommand.Owner, Kind: a.DomainCommand.Kind, ID: c.Meta.ID, Revision: runtimecore.Revision(c.Meta.Revision), Digest: runtimeDigest}
	attemptDigest, err := runtimecore.CanonicalJSONDigest("praxis.sandbox.workspace-read", "1.0.0", "OperationDispatchAttemptRefV3", a.Attempt)
	if err != nil {
		return err
	}
	if ref != a.DomainCommand || c.TenantID != string(a.Operation.ExecutionScope.Identity.TenantID) || c.OperationDigest != string(a.OperationDigest) || c.EffectID != string(a.Attempt.EffectID) || c.IntentRevision != uint64(a.Attempt.IntentRevision) || c.IntentDigest != string(a.Attempt.IntentDigest) || c.AttemptID != a.Attempt.AttemptID || c.PreparedDigest != string(a.Prepared.Digest) || c.DispatchDigest != string(attemptDigest) || c.ProviderComponent != string(a.Provider.ComponentID) || c.ProviderManifest != string(a.Provider.ManifestDigest) || c.SourceToolPayloadSchema != p.PayloadSchema.Key() || c.SourceToolPayloadDigest != string(p.PayloadDigest) || c.SourceToolPayloadRevision != uint64(p.PayloadRevision) || c.RequestedNotAfterUnixNano > a.UnifiedNotAfterUnixNano {
		return runtimecore.NewError(runtimecore.ErrorConflict, runtimecore.ReasonBindingDrift, "workspace read command differs from Runtime authorization")
	}
	return nil
}

func sandboxWorkspaceReadDigestToRuntimeV1(value string) (runtimecore.Digest, error) {
	if !contract.ValidDigest(value) {
		return "", runtimecore.NewError(runtimecore.ErrorInvalidArgument, runtimecore.ReasonInvalidDigest, "Sandbox workspace-read digest is invalid")
	}
	result := runtimecore.Digest("sha256:" + value)
	if err := result.Validate(); err != nil {
		return "", err
	}
	return result, nil
}

func runtimeWorkspaceReadDigestToSandboxV1(value runtimecore.Digest) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	const prefix = "sha256:"
	raw := string(value)
	if len(raw) <= len(prefix) || raw[:len(prefix)] != prefix || !contract.ValidDigest(raw[len(prefix):]) {
		return "", runtimecore.NewError(runtimecore.ErrorInvalidArgument, runtimecore.ReasonInvalidDigest, "Runtime workspace-read digest cannot be decoded as a Sandbox digest")
	}
	return raw[len(prefix):], nil
}

func validateWorkspaceReadActualPointResultV1(r WorkspaceReadActualPointResultV1, reservation contract.WorkspaceReadReservationV1, command contract.WorkspaceReadCommandV1, workspace contract.WorkspaceView, now time.Time) error {
	if r.File.ValidateShape("file") != nil || r.StartByte != command.StartByte || r.ReturnedBytes != uint64(len([]byte(r.Content))) || r.ReturnedBytes > command.MaxBytes || r.TotalBytes > contract.WorkspaceReadMaxBytesV1 || r.StartByte > r.TotalBytes || r.ReturnedBytes > r.TotalBytes-r.StartByte || r.Complete != (r.StartByte+r.ReturnedBytes == r.TotalBytes) || r.ContentDigest != contract.WorkspaceReadContentDigestV1([]byte(r.Content), r.StartByte, r.TotalBytes, r.Complete) || !r.ProviderS1Checked || !r.ProviderS2Checked || r.PhysicalReadCount != 1 || r.ProviderReceipt.Validate() != nil || r.ProviderReceipt.StableKeyDigest != reservation.StableKeyDigest || now.UnixNano() >= r.ProviderReceipt.ExpiresUnixNano {
		return errors.New("workspace read actual-point result is incomplete or stale")
	}
	if r.File.Revision != workspace.Meta.Revision || command.ExpectedFileRef != nil && !contract.SameRef(r.File, *command.ExpectedFileRef) {
		return errors.New("workspace read exact file ref drifted")
	}
	return nil
}

func validateWorkspaceReadRuntimeLeaseV1(binding contract.RuntimeLeaseBinding, current runtimeports.CurrentOperationDispatchEnforcementV4) error {
	runtimeLease := current.Sandbox.RuntimeLease
	scopeDigest, err := runtimeWorkspaceReadDigestToSandboxV1(runtimeLease.ScopeDigest)
	if err != nil {
		return runtimecore.NewError(runtimecore.ErrorConflict, runtimecore.ReasonBindingDrift, "workspace read Runtime scope digest is invalid")
	}
	if binding.TenantID != string(current.Sandbox.Operation.ExecutionScope.Identity.TenantID) ||
		binding.InstanceID != string(runtimeLease.Instance.ID) ||
		binding.InstanceEpoch != uint64(runtimeLease.Instance.Epoch) ||
		binding.LeaseID != string(runtimeLease.Lease.ID) ||
		binding.LeaseEpoch != uint64(runtimeLease.Lease.Epoch) ||
		binding.FenceEpoch != uint64(runtimeLease.FenceEpoch) ||
		binding.ScopeDigest != scopeDigest ||
		binding.ObservedRevision != uint64(runtimeLease.ObservedRevision) ||
		binding.ExpiresUnixNano != runtimeLease.Ref.ExpiresUnixNano {
		return runtimecore.NewError(runtimecore.ErrorConflict, runtimecore.ReasonBindingDrift, "workspace read WorkspaceView lease differs from Runtime current")
	}
	return nil
}

func workspaceReadAllowedV1(workspace contract.WorkspaceView, relative string) bool {
	allowed := false
	for _, scope := range workspace.ReadScopes {
		if relative == scope || strings.HasPrefix(relative, scope+"/") {
			allowed = true
			break
		}
	}
	if !allowed {
		return false
	}
	for _, scope := range workspace.HiddenScopes {
		if relative == scope || strings.HasPrefix(relative, scope+"/") {
			return false
		}
	}
	return path.Clean(relative) == relative
}

func workspaceReadAdmissionReceiptV1(stable runtimecore.Digest) (runtimeports.ControlledOperationProviderAdmissionReceiptRefV2, error) {
	return runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{ID: "workspace-read-admission-" + trimRuntimeDigestV1(string(stable)), Revision: 1, StableKeyDigest: stable, Admitted: true})
}

func workspaceReadAttemptRefV1(attempt contract.WorkspaceReadAttemptV1) contract.WorkspaceReadAttemptRefV1 {
	return contract.WorkspaceReadAttemptRefV1{
		ID: attempt.Meta.ID, Revision: attempt.Meta.Revision, Digest: attempt.Meta.Digest,
	}
}
func trimRuntimeDigestV1(value string) string { return strings.TrimPrefix(value, "sha256:") }
func minWorkspaceReadExpiryV1(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func nilLikeWorkspaceReadInspectionV2(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

var _ sandboxports.WorkspaceReadExecutionPortV1 = (*WorkspaceReadPhysicalExecutorV1)(nil)
var _ sandboxports.WorkspaceReadExecutionPortV2 = (*WorkspaceReadPhysicalExecutorV1)(nil)
var _ sandboxports.WorkspaceReadExecutionPortV3 = (*WorkspaceReadPhysicalExecutorV1)(nil)
var _ sandboxports.WorkspaceReadCommandExactReaderV1 = (*WorkspaceReadPhysicalExecutorV1)(nil)
