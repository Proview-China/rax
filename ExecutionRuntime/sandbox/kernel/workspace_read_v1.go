package kernel

import (
	"context"
	"errors"
	"path"
	"reflect"
	"strings"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	ownerworkspaceread "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// WorkspaceReadActualPointV1 is an internal Sandbox composition seam. It is
// exported only so dataplaneadapter can implement it without becoming a public
// Runtime port; callers still enter through ControlledOperationPhysicalExecutionPortV3.
type WorkspaceReadActualPointV1 interface {
	ReadWorkspaceFileV1(context.Context, WorkspaceReadActualPointRequestV1) (WorkspaceReadActualPointResultV1, error)
}

type WorkspaceReadActualPointBoundaryV1 string

const (
	WorkspaceReadEffectNotStartedV1     WorkspaceReadActualPointBoundaryV1 = "effect_not_started"
	WorkspaceReadEffectStartedUnknownV1 WorkspaceReadActualPointBoundaryV1 = "effect_started_unknown"
)

type WorkspaceReadActualPointErrorV1 struct {
	Boundary WorkspaceReadActualPointBoundaryV1
	Cause    error
}

func (e *WorkspaceReadActualPointErrorV1) Error() string {
	if e == nil || e.Cause == nil {
		return "workspace read actual-point failure"
	}
	return e.Cause.Error()
}
func (e *WorkspaceReadActualPointErrorV1) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
func NewWorkspaceReadActualPointErrorV1(boundary WorkspaceReadActualPointBoundaryV1, cause error) error {
	if cause == nil {
		cause = errors.New("workspace read actual-point failure")
	}
	return &WorkspaceReadActualPointErrorV1{Boundary: boundary, Cause: cause}
}

type WorkspaceReadActualPointRequestV1 struct {
	Reservation       contract.WorkspaceReadReservationV1
	Command           contract.WorkspaceReadCommandV1
	Workspace         contract.WorkspaceView
	RuntimeCurrent    runtimeports.CurrentOperationDispatchEnforcementV4
	CurrentQuery      sandboxports.WorkspaceReadCurrentQueryV2
	S1CheckedUnixNano int64
	ExpiresUnixNano   int64
}

type WorkspaceReadActualPointResultV1 struct {
	File              contract.Ref
	Content           string
	ContentDigest     string
	StartByte         uint64
	ReturnedBytes     uint64
	TotalBytes        uint64
	Complete          bool
	ProviderS1Checked bool
	ProviderS2Checked bool
	PhysicalReadCount uint64
	ProviderReceipt   contract.WorkspaceReadReceiptBindingV1
}

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

type WorkspaceReadPhysicalExecutorV1 struct {
	commands        sandboxports.WorkspaceReadPublishedCommandCurrentReaderV2
	associations    runtimeports.PreparedDomainCommandAssociationCurrentReaderV1
	workspaces      sandboxports.WorkspaceCurrentReaderV1
	sandboxCurrent  runtimeports.OperationDispatchSandboxCurrentReaderV4
	enforcement     runtimeports.OperationDispatchEnforcementGovernancePortV4
	store           sandboxports.WorkspaceReadOwnerStoreV1
	authorizedStore workspaceReadAuthorizedOwnerStoreV2
	actualPoint     WorkspaceReadActualPointV1
	clock           func() time.Time
}

func NewWorkspaceReadPhysicalExecutorV1(commands sandboxports.WorkspaceReadPublishedCommandCurrentReaderV2, associations runtimeports.PreparedDomainCommandAssociationCurrentReaderV1, workspaces sandboxports.WorkspaceCurrentReaderV1, sandboxCurrent runtimeports.OperationDispatchSandboxCurrentReaderV4, enforcement runtimeports.OperationDispatchEnforcementGovernancePortV4, store sandboxports.WorkspaceReadOwnerStoreV1, actualPoint WorkspaceReadActualPointV1, clock func() time.Time) (*WorkspaceReadPhysicalExecutorV1, error) {
	if commands == nil || associations == nil || workspaces == nil || sandboxCurrent == nil || enforcement == nil || store == nil || actualPoint == nil || clock == nil {
		return nil, runtimecore.NewError(runtimecore.ErrorInvalidArgument, runtimecore.ReasonInvalidReference, "workspace read physical executor dependencies are incomplete")
	}
	authorizedStore, ok := store.(workspaceReadAuthorizedOwnerStoreV2)
	if !ok || nilLikeWorkspaceReadInspectionV2(authorizedStore) {
		return nil, runtimecore.NewError(runtimecore.ErrorInvalidArgument, runtimecore.ReasonInvalidReference, "workspace read Runtime-attempt history store is incomplete")
	}
	return &WorkspaceReadPhysicalExecutorV1{commands: commands, associations: associations, workspaces: workspaces, sandboxCurrent: sandboxCurrent, enforcement: enforcement, store: store, authorizedStore: authorizedStore, actualPoint: actualPoint, clock: clock}, nil
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

	association, command, workspace, s1, err := e.readCurrentClosureV1(ctx, authorization)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	runtimeCurrent, err := e.readRuntimeCurrentV1(ctx, authorization, s1)
	if err != nil {
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
		workspace.Meta.ExpiresUnixNano,
		workspace.Lease.ExpiresUnixNano,
	)
	expires := time.Unix(0, expiresNano)
	requestDigest := command.Meta.Digest
	payloadDigest := command.SourceToolPayloadDigest
	factTime := time.Unix(0, association.CheckedUnixNano)
	receipt, err := workspaceReadAdmissionReceiptV1(authorization.StableKeyDigest)
	if err != nil {
		return receipt, err
	}
	admissionBinding := contract.WorkspaceReadReceiptBindingV1{
		ID: receipt.ID, Revision: uint64(receipt.Revision), Digest: string(receipt.Digest),
		StableKeyDigest: string(receipt.StableKeyDigest), CheckedUnixNano: factTime.UnixNano(), ExpiresUnixNano: expiresNano,
	}
	ttlClosure, err := contract.SealWorkspaceReadTTLClosureV1(contract.WorkspaceReadTTLClosureV1{
		UnifiedNotAfterUnixNano:       authorization.UnifiedNotAfterUnixNano,
		RuntimeEnforcementExpiresNano: runtimeCurrent.ExpiresUnixNano,
		AssociationExpiresUnixNano:    association.ExpiresUnixNano,
		CommandRequestedNotAfterNano:  command.RequestedNotAfterUnixNano,
		CommandExpiresUnixNano:        command.Meta.ExpiresUnixNano,
		WorkspaceViewExpiresUnixNano:  workspace.Meta.ExpiresUnixNano,
		WorkspaceLeaseExpiresUnixNano: workspace.Lease.ExpiresUnixNano,
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
			return receipt, runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read requires exact Inspect of the original attempt")
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
	currentQuery, err := workspaceReadCurrentQueryV2(authorization, association, command, workspace, reservation, attempt, admissionBinding, runtimeCurrent, s1, expiresNano)
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

	result, readErr := e.actualPoint.ReadWorkspaceFileV1(ctx, WorkspaceReadActualPointRequestV1{
		Reservation: reservation, Command: command, Workspace: workspace,
		RuntimeCurrent: runtimeCurrent, CurrentQuery: currentQuery,
		S1CheckedUnixNano: s1.UnixNano(), ExpiresUnixNano: expiresNano,
	})
	if readErr != nil {
		var actualPointError *WorkspaceReadActualPointErrorV1
		if errors.As(readErr, &actualPointError) && actualPointError.Boundary == WorkspaceReadEffectNotStartedV1 {
			failureDigest, digestErr := contract.Digest("workspace-read-failed", struct{ Cause string }{readErr.Error()})
			if digestErr != nil {
				return receipt, digestErr
			}
			if failErr := e.failWorkspaceReadAuthorizedV2(ctx, transitionAuthority, failureDigest); failErr != nil {
				return receipt, failErr
			}
			return receipt, readErr
		}
		return receipt, e.markWorkspaceReadUnknownV1(ctx, transitionAuthority, "actual-point", readErr)
	}

	// S2 is a full current re-read after Rust crossed the physical actual point.
	_, commandS2, workspaceS2, s2, err := e.readCurrentClosureV1(ctx, authorization)
	if err != nil || !contract.SameRef(commandS2.Meta.Ref(), command.Meta.Ref()) || !contract.SameRef(workspaceS2.Meta.Ref(), workspace.Meta.Ref()) {
		if err == nil {
			err = errors.New("workspace read current closure drifted at S2")
		}
		return receipt, e.markWorkspaceReadUnknownV1(ctx, transitionAuthority, "s2-current", err)
	}
	runtimeCurrentS2, err := e.readRuntimeCurrentV1(ctx, authorization, s2)
	if err != nil || runtimeCurrentS2.Digest != runtimeCurrent.Digest || runtimeCurrentS2.ExpiresUnixNano != runtimeCurrent.ExpiresUnixNano {
		if err == nil {
			err = runtimecore.NewError(runtimecore.ErrorConflict, runtimecore.ReasonBindingDrift, "workspace read Runtime current drifted at S2")
		}
		return receipt, e.markWorkspaceReadUnknownV1(ctx, transitionAuthority, "s2-runtime-current", err)
	}
	if err = validateWorkspaceReadRuntimeLeaseV1(workspaceS2.Lease, runtimeCurrentS2); err != nil {
		return receipt, e.markWorkspaceReadUnknownV1(ctx, transitionAuthority, "s2-workspace-lease", err)
	}
	if err = validateWorkspaceReadActualPointResultV1(result, reservation, command, workspace, s2); err != nil {
		return receipt, e.markWorkspaceReadUnknownV1(ctx, transitionAuthority, "provider-result", err)
	}

	observationExpiresNano := minWorkspaceReadExpiryV1(expiresNano, runtimeCurrentS2.ExpiresUnixNano, reservation.Meta.ExpiresUnixNano, attempt.Meta.ExpiresUnixNano, admissionBinding.ExpiresUnixNano, result.ProviderReceipt.ExpiresUnixNano)
	observation, err := contract.SealWorkspaceReadObservationV1(contract.WorkspaceReadObservationV1{
		Reservation: reservation.Meta.Ref(), Command: command.Meta.Ref(), WorkspaceView: workspace.Meta.Ref(), File: result.File,
		RelativePath: command.RelativePath, StartByte: result.StartByte, ReturnedBytes: result.ReturnedBytes, TotalBytes: result.TotalBytes,
		Complete: result.Complete, Content: result.Content, ContentDigest: result.ContentDigest,
		S1CheckedUnixNano: s1.UnixNano(), S2CheckedUnixNano: s2.UnixNano(),
		AdmissionReceipt: admissionBinding, ProviderReceipt: result.ProviderReceipt,
	}, "workspace-read-observation-"+trimRuntimeDigestV1(string(authorization.StableKeyDigest)), s2, time.Unix(0, observationExpiresNano))
	if err != nil {
		return receipt, e.markWorkspaceReadUnknownV1(ctx, transitionAuthority, "observation-seal", err)
	}
	transition, err := transitionAuthority.Observed(observation, s2)
	if err != nil {
		return receipt, e.markWorkspaceReadUnknownV1(ctx, transitionAuthority, "observation-authority", err)
	}
	if _, err = authorizedStore.TransitionWorkspaceReadAuthorizedV2(ctx, transition); err != nil {
		return receipt, runtimecore.NewError(runtimecore.ErrorIndeterminate, runtimecore.ReasonEffectUnknownOutcome, "workspace read observation persistence requires exact Inspect")
	}
	return receipt, nil
}

func workspaceReadCurrentQueryV2(
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
	association runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1,
	command contract.WorkspaceReadCommandV1,
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
		Authorization:       authorization,
		StableKeyDigest:     authorization.StableKeyDigest,
		AuthorizationDigest: authorization.AuthorizationDigest,
		Association:         association.Ref,
		DomainCommand:       association.DomainCommand,
		Command:             command.Meta.Ref(),
		WorkspaceView:       workspace.Meta.Ref(),
		FileScopeDigest:     command.FileScopeDigest,
		RelativePath:        command.RelativePath,
		CheckedUnixNano:     checked.UnixNano(),
		ExpiresUnixNano:     expiresUnixNano,
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

func (e *WorkspaceReadPhysicalExecutorV1) readCurrentClosureV1(ctx context.Context, authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3) (runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1, contract.WorkspaceReadCommandV1, contract.WorkspaceView, time.Time, error) {
	now := e.clock()
	if err := authorization.ValidateCurrent(now); err != nil {
		return runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1{}, contract.WorkspaceReadCommandV1{}, contract.WorkspaceView{}, now, err
	}
	association, err := e.associations.InspectCurrentPreparedDomainCommandAssociationV1(ctx, authorization.Association)
	if err != nil || association.ValidateCurrent(authorization.Association, now) != nil {
		if err == nil {
			err = association.ValidateCurrent(authorization.Association, now)
		}
		return association, contract.WorkspaceReadCommandV1{}, contract.WorkspaceView{}, now, err
	}
	if association.DomainCommand != authorization.DomainCommand || association.Operation != authorization.Operation || association.Prepared != authorization.Prepared || association.Attempt != authorization.Attempt || association.Provider != authorization.Provider {
		return association, contract.WorkspaceReadCommandV1{}, contract.WorkspaceView{}, now, runtimecore.NewError(runtimecore.ErrorConflict, runtimecore.ReasonBindingDrift, "workspace read Association drifted from authorization")
	}
	commandDigest, err := runtimeWorkspaceReadDigestToSandboxV1(authorization.DomainCommand.Digest)
	if err != nil {
		return runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1{}, contract.WorkspaceReadCommandV1{}, contract.WorkspaceView{}, now, err
	}
	commandRef := contract.Ref{ID: authorization.DomainCommand.ID, Revision: uint64(authorization.DomainCommand.Revision), Digest: commandDigest}
	command, err := e.commands.InspectWorkspaceReadPublishedCommandCurrentV2(ctx, commandRef)
	if err != nil {
		return association, command, contract.WorkspaceView{}, now, err
	}
	if err = validateWorkspaceReadCommandAuthorizationV1(command, association, authorization); err != nil || command.ValidateCurrent(now) != nil {
		if err == nil {
			err = command.ValidateCurrent(now)
		}
		return association, command, contract.WorkspaceView{}, now, err
	}
	workspace, err := e.workspaces.InspectWorkspaceViewCurrentV1(ctx, command.WorkspaceView)
	if err != nil || workspace.ValidateCurrent(now) != nil {
		if err == nil {
			err = workspace.ValidateCurrent(now)
		}
		return association, command, workspace, now, err
	}
	if !contract.SameRef(workspace.Meta.Ref(), command.WorkspaceView) || workspace.FileScopeDigest != command.FileScopeDigest || !workspaceReadAllowedV1(workspace, command.RelativePath) {
		return association, command, workspace, now, runtimecore.NewError(runtimecore.ErrorForbidden, runtimecore.ReasonEvidenceScopeConflict, "workspace read path is outside the exact read scope")
	}
	return association, command, workspace, now, nil
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
