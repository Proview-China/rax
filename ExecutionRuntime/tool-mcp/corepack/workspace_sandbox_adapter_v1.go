package corepack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	sandboxcontract "github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

const (
	WorkspaceReadSandboxRequestContractVersionV1 = "praxis.tool-mcp/workspace-read-sandbox-request/v1"
	WorkspaceReadMinimumRecoveryTimeoutV1        = 5 * time.Second
	WorkspaceReadMaximumRecoveryTimeoutV1        = 30 * time.Second
)

type WorkspaceReadSandboxRequestV1 struct {
	ContractVersion string                            `json:"contract_version"`
	SourceCommand   toolcontract.ObjectRef            `json:"source_command"`
	PayloadSchema   runtimeports.SchemaRefV2          `json:"payload_schema"`
	PayloadDigest   runtimecore.Digest                `json:"payload_digest"`
	PayloadRevision runtimecore.Revision              `json:"payload_revision"`
	Input           toolcontract.WorkspaceReadInputV1 `json:"input"`
	CanonicalInput  []byte                            `json:"canonical_input"`
	Digest          runtimecore.Digest                `json:"digest"`
}

func SealWorkspaceReadSandboxRequestV1(request WorkspaceReadSandboxRequestV1) (WorkspaceReadSandboxRequestV1, error) {
	request.ContractVersion = WorkspaceReadSandboxRequestContractVersionV1
	request.CanonicalInput = append([]byte(nil), request.CanonicalInput...)
	if len(request.CanonicalInput) == 0 {
		body, err := json.Marshal(request.Input)
		if err != nil {
			return WorkspaceReadSandboxRequestV1{}, workspaceReadInvalidV1("workspace.read canonical input cannot be encoded")
		}
		request.CanonicalInput = body
	}
	request.Digest = ""
	digest, err := request.computeDigestV1()
	if err != nil {
		return WorkspaceReadSandboxRequestV1{}, err
	}
	request.Digest = digest
	if err = request.ValidateCurrent(time.Unix(0, request.Input.RequestedNotAfter-1)); err != nil {
		return WorkspaceReadSandboxRequestV1{}, err
	}
	return request.Clone(), nil
}

func (request WorkspaceReadSandboxRequestV1) Clone() WorkspaceReadSandboxRequestV1 {
	request.CanonicalInput = append([]byte(nil), request.CanonicalInput...)
	return request
}

func (request WorkspaceReadSandboxRequestV1) ValidateCurrent(now time.Time) error {
	if request.ContractVersion != WorkspaceReadSandboxRequestContractVersionV1 ||
		request.SourceCommand.Validate() != nil ||
		!workspaceReadSchemaValidV1(request.PayloadSchema) ||
		request.PayloadDigest.Validate() != nil ||
		request.PayloadRevision == 0 ||
		len(request.CanonicalInput) == 0 ||
		request.Digest.Validate() != nil {
		return workspaceReadInvalidV1("workspace.read Sandbox request envelope is incomplete")
	}
	if err := request.Input.ValidateCurrent(now); err != nil {
		return err
	}
	decoded, err := toolcontract.DecodeWorkspaceReadInputV1(request.CanonicalInput)
	if err != nil || !reflect.DeepEqual(decoded, request.Input) {
		return workspaceReadConflictV1("workspace.read canonical input differs from the typed input")
	}
	canonical, err := json.Marshal(decoded)
	if err != nil || !bytes.Equal(canonical, request.CanonicalInput) {
		return workspaceReadConflictV1("workspace.read input bytes are not canonical")
	}
	if runtimecore.DigestBytes(request.CanonicalInput) != request.PayloadDigest {
		return workspaceReadConflictV1("workspace.read canonical input differs from the exact payload digest")
	}
	digest, err := request.computeDigestV1()
	if err != nil || digest != request.Digest {
		return workspaceReadConflictV1("workspace.read Sandbox request envelope digest drifted")
	}
	return nil
}

func (request WorkspaceReadSandboxRequestV1) computeDigestV1() (runtimecore.Digest, error) {
	request.Digest = ""
	return runtimecore.CanonicalJSONDigest(
		"praxis.tool-mcp.workspace-read-sandbox", WorkspaceReadSandboxRequestContractVersionV1,
		"WorkspaceReadSandboxRequestV1", request,
	)
}

type WorkspaceReadRuntimeAdmissionInspectionV1 = toolcontract.WorkspaceReadRuntimeAdmissionInspectionV1
type WorkspaceReadRuntimeAdmissionCurrentReaderV1 = toolcontract.WorkspaceReadRuntimeAdmissionCurrentReaderV1

func SealWorkspaceReadRuntimeAdmissionInspectionV1(
	inspection WorkspaceReadRuntimeAdmissionInspectionV1,
) (WorkspaceReadRuntimeAdmissionInspectionV1, error) {
	return toolcontract.SealWorkspaceReadRuntimeAdmissionInspectionV1(inspection)
}

// UnsupportedWorkspaceReadRuntimeAdmissionCurrentReaderV1 is the zero-call
// production placeholder while Runtime has no public Attempt-to-Admission
// exact reader. It never executes or synthesizes a Runtime fact.
type UnsupportedWorkspaceReadRuntimeAdmissionCurrentReaderV1 struct{}

func (UnsupportedWorkspaceReadRuntimeAdmissionCurrentReaderV1) InspectWorkspaceReadAdmissionForRuntimeAttemptV1(
	ctx context.Context,
	attempt runtimeports.OperationDispatchAttemptRefV3,
) (WorkspaceReadRuntimeAdmissionInspectionV1, error) {
	if ctx == nil {
		return WorkspaceReadRuntimeAdmissionInspectionV1{}, workspaceReadInvalidV1("workspace.read Runtime inspection context is nil")
	}
	if err := ctx.Err(); err != nil {
		return WorkspaceReadRuntimeAdmissionInspectionV1{}, err
	}
	if err := attempt.Validate(); err != nil {
		return WorkspaceReadRuntimeAdmissionInspectionV1{}, err
	}
	return WorkspaceReadRuntimeAdmissionInspectionV1{}, workspaceReadUnavailableV1("Runtime does not expose an exact Attempt-to-Admission reader")
}

// WorkspaceReadSandboxAdapterV1 is a Tool-owned Start-or-Inspect adapter. It
// never reads the host filesystem and never chooses a Sandbox attempt. Runtime
// authorization starts the Sandbox-owned effect; the exact Admission receipt
// is the sole lookup coordinate for recovering its original attempt.
type WorkspaceReadSandboxAdapterV1 struct {
	execution        sandboxports.WorkspaceReadExecutionPortV2
	commands         sandboxports.WorkspaceReadCommandCurrentReaderV1
	commandHistory   sandboxports.WorkspaceReadCommandExactReaderV1
	runtimeAdmission WorkspaceReadRuntimeAdmissionCurrentReaderV1
	clock            func() time.Time
	recoveryTimeout  time.Duration
}

func NewWorkspaceReadSandboxAdapterV1(
	execution sandboxports.WorkspaceReadExecutionPortV2,
	commands sandboxports.WorkspaceReadCommandCurrentReaderV1,
	commandHistory sandboxports.WorkspaceReadCommandExactReaderV1,
	runtimeAdmission WorkspaceReadRuntimeAdmissionCurrentReaderV1,
	clock func() time.Time,
	recoveryTimeout time.Duration,
) (*WorkspaceReadSandboxAdapterV1, error) {
	if workspaceReadNilV1(execution) ||
		workspaceReadNilV1(commands) ||
		workspaceReadNilV1(commandHistory) ||
		workspaceReadNilV1(runtimeAdmission) ||
		workspaceReadNilV1(clock) ||
		recoveryTimeout < WorkspaceReadMinimumRecoveryTimeoutV1 ||
		recoveryTimeout > WorkspaceReadMaximumRecoveryTimeoutV1 {
		return nil, runtimecore.NewError(runtimecore.ErrorInvalidArgument, runtimecore.ReasonComponentMissing, "workspace.read Sandbox adapter dependencies are incomplete")
	}
	return &WorkspaceReadSandboxAdapterV1{
		execution: execution, commands: commands, commandHistory: commandHistory, runtimeAdmission: runtimeAdmission,
		clock: clock, recoveryTimeout: recoveryTimeout,
	}, nil
}

// StartOrInspectWorkspaceReadV1 executes one already-governed Runtime physical
// authorization or inspects the exact Sandbox attempt selected by its
// Admission receipt. A caller retry does not authorize a second physical read:
// Sandbox owns create-once reservation and attempt state.
func (a *WorkspaceReadSandboxAdapterV1) StartOrInspectWorkspaceReadV1(
	ctx context.Context,
	request WorkspaceReadSandboxRequestV1,
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
) (toolcontract.WorkspaceReadOutputV1, error) {
	if ctx == nil {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadInvalidV1("workspace.read context is nil")
	}
	if a == nil ||
		workspaceReadNilV1(a.execution) ||
		workspaceReadNilV1(a.commands) ||
		workspaceReadNilV1(a.commandHistory) ||
		workspaceReadNilV1(a.runtimeAdmission) ||
		workspaceReadNilV1(a.clock) {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadUnavailableV1("workspace.read Sandbox adapter is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	authorization = cloneWorkspaceReadAuthorizationV1(authorization)
	request = request.Clone()

	s1 := a.clock()
	if err := validateWorkspaceReadAdapterRequestV1(request, authorization, s1); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	commandRef, err := workspaceReadCommandRefV1(authorization.DomainCommand)
	if err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	commandS1, err := a.commands.InspectWorkspaceReadCommandCurrentV1(ctx, commandRef)
	if err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	commandS1 = cloneWorkspaceReadCommandV1(commandS1)
	if err = validateWorkspaceReadCommandInputV1(commandS1, commandRef, request, authorization, s1); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}

	commandS2, err := a.commands.InspectWorkspaceReadCommandCurrentV1(ctx, commandRef)
	if err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	commandS2 = cloneWorkspaceReadCommandV1(commandS2)
	s2 := a.clock()
	if s2.Before(s1) {
		return toolcontract.WorkspaceReadOutputV1{}, runtimecore.NewError(runtimecore.ErrorPreconditionFailed, runtimecore.ReasonClockRegression, "workspace.read Tool clock regressed before Sandbox entry")
	}
	if !reflect.DeepEqual(commandS1, commandS2) {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadConflictV1("workspace.read Sandbox Command drifted before physical entry")
	}
	if err = validateWorkspaceReadAdapterRequestV1(request, authorization, s2); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	if err = validateWorkspaceReadCommandInputV1(commandS2, commandRef, request, authorization, s2); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}

	entryAt := a.clock()
	if entryAt.Before(s2) {
		return toolcontract.WorkspaceReadOutputV1{}, runtimecore.NewError(runtimecore.ErrorPreconditionFailed, runtimecore.ReasonClockRegression, "workspace.read Tool clock regressed at Sandbox physical entry")
	}
	if err = validateWorkspaceReadAdapterRequestV1(request, authorization, entryAt); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	if err = validateWorkspaceReadCommandInputV1(commandS2, commandRef, request, authorization, entryAt); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	if err = ctx.Err(); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	admission, executeErr := a.execution.ExecuteControlledOperationPhysicalV3(ctx, authorization)
	if admission.Validate() != nil ||
		!admission.Admitted ||
		admission.NoEffect ||
		admission.StableKeyDigest != authorization.StableKeyDigest {
		return a.InspectWorkspaceReadRecoveryV1(ctx, request, authorization)
	}

	recovery, cancel := workspaceReadRecoveryContextV1(ctx, a.recoveryTimeout)
	defer cancel()
	return a.inspectWorkspaceReadAdmissionV1(recovery, request, authorization, commandRef, &commandS2, admission, executeErr)
}

// InspectWorkspaceReadRecoveryV1 is the independent inspect-only recovery
// entry. It never invokes ExecuteControlledOperationPhysicalV3 and therefore
// may inspect the original attempt after execution eligibility has expired.
func (a *WorkspaceReadSandboxAdapterV1) InspectWorkspaceReadRecoveryV1(
	ctx context.Context,
	request WorkspaceReadSandboxRequestV1,
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
) (toolcontract.WorkspaceReadOutputV1, error) {
	if ctx == nil {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadInvalidV1("workspace.read recovery context is nil")
	}
	if a == nil ||
		workspaceReadNilV1(a.execution) ||
		workspaceReadNilV1(a.commandHistory) ||
		workspaceReadNilV1(a.runtimeAdmission) ||
		workspaceReadNilV1(a.clock) {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadUnavailableV1("workspace.read Sandbox recovery is unavailable")
	}
	request = request.Clone()
	authorization = cloneWorkspaceReadAuthorizationV1(authorization)
	if err := validateWorkspaceReadRecoveryRequestV1(request, authorization); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	commandRef, err := workspaceReadCommandRefV1(authorization.DomainCommand)
	if err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	recovery, cancel := workspaceReadRecoveryContextV1(ctx, a.recoveryTimeout)
	defer cancel()
	runtimeInspection, err := a.runtimeAdmission.InspectWorkspaceReadAdmissionForRuntimeAttemptV1(
		recovery,
		authorization.Attempt,
	)
	if err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadUnknownV1("workspace.read Runtime Attempt inspection failed", err)
	}
	now := a.clock()
	if err = runtimeInspection.ValidateCurrent(authorization.Attempt, now); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	if runtimeInspection.Admission.StableKeyDigest != authorization.StableKeyDigest {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadConflictV1("workspace.read Runtime Admission differs from the exact authorization")
	}
	return a.inspectWorkspaceReadAdmissionV1(
		recovery,
		request,
		authorization,
		commandRef,
		nil,
		runtimeInspection.Admission,
		nil,
	)
}

func (a *WorkspaceReadSandboxAdapterV1) inspectWorkspaceReadAdmissionV1(
	recovery context.Context,
	request WorkspaceReadSandboxRequestV1,
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
	commandRef sandboxcontract.Ref,
	currentCommand *sandboxcontract.WorkspaceReadCommandV1,
	admission runtimeports.ControlledOperationProviderAdmissionReceiptRefV2,
	executeErr error,
) (toolcontract.WorkspaceReadOutputV1, error) {
	binding, err := a.execution.InspectWorkspaceReadAttemptForAdmissionV1(recovery, admission)
	if err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadUnknownV1("workspace.read Admission-to-Attempt lookup failed", err)
	}
	if err = validateWorkspaceReadAdmissionBindingV1(binding, admission, commandRef, authorization); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	var verifiedCommand sandboxcontract.WorkspaceReadCommandV1
	if currentCommand == nil {
		verifiedCommand, err = a.commandHistory.InspectWorkspaceReadCommandExactV1(recovery, binding.Command)
		inspectErr := err
		if inspectErr != nil {
			return toolcontract.WorkspaceReadOutputV1{}, workspaceReadUnknownV1("workspace.read historical Sandbox Command inspection failed", inspectErr)
		}
	} else {
		verifiedCommand = *currentCommand
	}
	verifiedCommand = cloneWorkspaceReadCommandV1(verifiedCommand)
	if err = validateWorkspaceReadCommandHistoricalV1(
		verifiedCommand,
		binding.Command,
		request,
		authorization,
	); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	envelope, err := a.execution.InspectBoundedWorkspaceReadV2(recovery, binding.Attempt)
	if err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadUnknownV1("workspace.read exact Attempt inspection failed", err)
	}
	now := a.clock()
	if err = envelope.ValidateCurrent(now); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadConflictV1("workspace.read Sandbox inspection envelope is invalid or expired")
	}
	if envelope.RequestedOriginAttemptRef != binding.Attempt {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadConflictV1("workspace.read Sandbox inspection changed the exact requested origin Attempt")
	}
	projection := envelope.CurrentProjection
	if err = projection.ValidateShape(); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadConflictV1("workspace.read Sandbox projection is invalid")
	}
	if !sandboxcontract.SameRef(projection.Attempt.Reservation, projection.Reservation.Meta.Ref()) ||
		projection.Attempt.StableKeyDigest != string(binding.StableKeyDigest) ||
		projection.Reservation.Command != binding.Command ||
		projection.Reservation.AuthorizationDigest != string(binding.AuthorizationDigest) ||
		projection.Reservation.PayloadDigest != string(request.PayloadDigest) ||
		projection.AdmissionReceipt.ID != admission.ID ||
		projection.AdmissionReceipt.Revision != uint64(admission.Revision) ||
		projection.AdmissionReceipt.Digest != string(admission.Digest) {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadConflictV1("workspace.read Sandbox Attempt closure drifted")
	}
	if err = validateWorkspaceReadReservationTTLClosureV1(
		projection.Reservation,
		projection,
		binding,
		verifiedCommand,
		authorization,
	); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}

	switch projection.Attempt.State {
	case sandboxcontract.WorkspaceReadObservedV1:
		output, outputErr := a.workspaceReadOutputV1(request.Input, verifiedCommand, binding, envelope, now)
		if outputErr != nil {
			return toolcontract.WorkspaceReadOutputV1{}, outputErr
		}
		return output, nil
	case sandboxcontract.WorkspaceReadStartedV1, sandboxcontract.WorkspaceReadUnknownV1:
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadUnknownV1("workspace.read original Attempt is not settled", executeErr)
	case sandboxcontract.WorkspaceReadFailedV1:
		return toolcontract.WorkspaceReadOutputV1{}, runtimecore.NewError(
			runtimecore.ErrorPreconditionFailed,
			runtimecore.ReasonEffectStateConflict,
			"workspace.read exact Sandbox Attempt settled as a deterministic failure",
		)
	default:
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadConflictV1("workspace.read Sandbox Attempt state is invalid")
	}
}

// cloneWorkspaceReadAuthorizationV1 freezes every pointer reachable from the
// public V3 authorization. Its remaining fields are scalar/array-free value
// structs, so all subsequent validation and execution consume one snapshot.
func cloneWorkspaceReadAuthorizationV1(
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
) runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3 {
	if authorization.Operation.ExecutionScope.SandboxLease != nil {
		lease := *authorization.Operation.ExecutionScope.SandboxLease
		authorization.Operation.ExecutionScope.SandboxLease = &lease
	}
	if authorization.Attempt.Delegation != nil {
		delegation := *authorization.Attempt.Delegation
		authorization.Attempt.Delegation = &delegation
	}
	return authorization
}

func cloneWorkspaceReadCommandV1(
	command sandboxcontract.WorkspaceReadCommandV1,
) sandboxcontract.WorkspaceReadCommandV1 {
	if command.ExpectedFileRef != nil {
		expected := *command.ExpectedFileRef
		command.ExpectedFileRef = &expected
	}
	return command
}

func (a *WorkspaceReadSandboxAdapterV1) workspaceReadOutputV1(
	input toolcontract.WorkspaceReadInputV1,
	command sandboxcontract.WorkspaceReadCommandV1,
	binding sandboxports.WorkspaceReadAdmissionAttemptBindingV1,
	envelope sandboxports.WorkspaceReadInspectionEnvelopeV2,
	previous time.Time,
) (toolcontract.WorkspaceReadOutputV1, error) {
	now := a.clock()
	if now.Before(previous) {
		return toolcontract.WorkspaceReadOutputV1{}, runtimecore.NewError(runtimecore.ErrorPreconditionFailed, runtimecore.ReasonClockRegression, "workspace.read Tool clock regressed after Sandbox inspection")
	}
	if err := envelope.ValidateCurrent(now); err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadConflictV1("workspace.read Sandbox inspection envelope expired before Tool materialization")
	}
	projection := envelope.CurrentProjection
	observation := projection.Observation
	if observation == nil || projection.ProviderReceipt == nil {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadConflictV1("workspace.read observed Attempt lacks an authoritative Observation")
	}
	expectedFileID, err := sandboxcontract.WorkspaceReadFileIDV1(observation.WorkspaceView.ID, observation.RelativePath)
	if err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadConflictV1("workspace.read Sandbox File identity cannot be derived")
	}
	workspace, err := workspaceReadSandboxRefV1(input.WorkspaceRoot)
	if err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	if !sandboxcontract.SameRef(observation.Command, binding.Command) ||
		!sandboxcontract.SameRef(observation.Reservation, projection.Reservation.Meta.Ref()) ||
		!sandboxcontract.SameRef(projection.Reservation.WorkspaceView, workspace) ||
		!sandboxcontract.SameRef(observation.WorkspaceView, workspace) ||
		observation.File.ID != expectedFileID ||
		observation.File.Revision != observation.WorkspaceView.Revision ||
		observation.RelativePath != input.RelativePath ||
		observation.StartByte != input.StartByte ||
		observation.ReturnedBytes > input.MaxBytes ||
		observation.ContentDigest != sandboxcontract.WorkspaceReadContentDigestV1(
			[]byte(observation.Content), observation.StartByte, observation.TotalBytes, observation.Complete,
		) {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadConflictV1("workspace.read Sandbox Observation differs from the Tool request")
	}
	if command.ExpectedFileRef != nil &&
		!sandboxcontract.SameRef(observation.File, *command.ExpectedFileRef) {
		return toolcontract.WorkspaceReadOutputV1{}, workspaceReadConflictV1("workspace.read Sandbox Observation differs from the exact expected File")
	}
	if observation.ReturnedBytes > runtimeports.MaxOpaqueInlineBytes ||
		len(observation.Content) > runtimeports.MaxOpaqueInlineBytes {
		return toolcontract.WorkspaceReadOutputV1{}, runtimecore.NewError(
			runtimecore.ErrorPreconditionFailed,
			runtimecore.ReasonCanonicalLimitExceeded,
			"workspace.read output requires a Tool-owned Artifact and cannot be returned inline",
		)
	}
	digest, err := workspaceReadRuntimeDigestV1(observation.File.Digest)
	if err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	content := observation.Content
	output, err := toolcontract.SealWorkspaceReadOutputV1(toolcontract.WorkspaceReadOutputV1{
		File: toolcontract.WorkspaceFileRefV1{
			Path: observation.RelativePath, Revision: runtimecore.Revision(observation.File.Revision), Digest: digest,
		},
		StartByte:       observation.StartByte,
		BytesReturned:   observation.ReturnedBytes,
		TotalBytes:      observation.TotalBytes,
		Complete:        observation.Complete,
		Content:         &content,
		CheckedUnixNano: now.UnixNano(),
		ExpiresUnixNano: envelope.ExpiresUnixNano,
	})
	if err != nil {
		return toolcontract.WorkspaceReadOutputV1{}, err
	}
	return output.Clone(), nil
}

func validateWorkspaceReadRecoveryRequestV1(
	request WorkspaceReadSandboxRequestV1,
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
) error {
	if request.Input.RequestedNotAfter <= 1 {
		return workspaceReadInvalidV1("workspace.read recovery request has no historical validation point")
	}
	if err := request.ValidateCurrent(time.Unix(0, request.Input.RequestedNotAfter-1)); err != nil {
		return err
	}
	if err := authorization.Validate(); err != nil {
		return err
	}
	if string(authorization.DomainCommand.Kind) != sandboxcontract.WorkspaceReadCommandKindV1 ||
		request.Input.RequestedNotAfter > authorization.UnifiedNotAfterUnixNano ||
		authorization.Prepared.PayloadSchema != request.PayloadSchema ||
		authorization.Prepared.PayloadDigest != request.PayloadDigest ||
		authorization.Prepared.PayloadRevision != request.PayloadRevision {
		return workspaceReadConflictV1("workspace.read recovery request differs from the historical Runtime authorization")
	}
	return nil
}

func validateWorkspaceReadAdapterRequestV1(
	request WorkspaceReadSandboxRequestV1,
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
	now time.Time,
) error {
	if err := request.ValidateCurrent(now); err != nil {
		return err
	}
	if err := authorization.ValidateCurrent(now); err != nil {
		return err
	}
	if string(authorization.DomainCommand.Kind) != sandboxcontract.WorkspaceReadCommandKindV1 {
		return runtimecore.NewError(runtimecore.ErrorForbidden, runtimecore.ReasonUnknownGovernanceCategory, "workspace.read Tool adapter accepts only the Sandbox workspace-read command kind")
	}
	if request.Input.RequestedNotAfter > authorization.UnifiedNotAfterUnixNano {
		return runtimecore.NewError(runtimecore.ErrorConflict, runtimecore.ReasonBindingDrift, "workspace.read Tool request exceeds Runtime authorization")
	}
	if authorization.Prepared.PayloadSchema != request.PayloadSchema ||
		authorization.Prepared.PayloadDigest != request.PayloadDigest ||
		authorization.Prepared.PayloadRevision != request.PayloadRevision {
		return workspaceReadConflictV1("workspace.read request payload differs from the prepared Runtime attempt")
	}
	return nil
}

func validateWorkspaceReadCommandInputV1(
	command sandboxcontract.WorkspaceReadCommandV1,
	exact sandboxcontract.Ref,
	request WorkspaceReadSandboxRequestV1,
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
	now time.Time,
) error {
	if err := command.ValidateCurrent(now); err != nil {
		return err
	}
	return validateWorkspaceReadCommandHistoricalV1(command, exact, request, authorization)
}

func validateWorkspaceReadCommandHistoricalV1(
	command sandboxcontract.WorkspaceReadCommandV1,
	exact sandboxcontract.Ref,
	request WorkspaceReadSandboxRequestV1,
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
) error {
	if err := command.ValidateShape(); err != nil {
		return workspaceReadConflictV1("workspace.read historical Sandbox Command is not an exact sealed fact")
	}
	workspace, err := workspaceReadSandboxRefV1(request.Input.WorkspaceRoot)
	if err != nil {
		return err
	}
	sourceCommand, err := workspaceReadToolCommandRefV1(request.SourceCommand)
	if err != nil {
		return err
	}
	dispatchDigest, err := runtimecore.CanonicalJSONDigest(
		"praxis.sandbox.workspace-read",
		"1.0.0",
		"OperationDispatchAttemptRefV3",
		authorization.Attempt,
	)
	if err != nil {
		return workspaceReadInvalidV1("workspace.read Runtime dispatch digest cannot be computed")
	}
	if !sandboxcontract.SameRef(command.Meta.Ref(), exact) ||
		!sandboxcontract.SameRef(command.SourceToolCommand, sourceCommand) ||
		command.SourceToolPayloadSchema != request.PayloadSchema.Key() ||
		command.SourceToolPayloadDigest != string(request.PayloadDigest) ||
		command.SourceToolPayloadRevision != uint64(request.PayloadRevision) ||
		!sandboxcontract.SameRef(command.WorkspaceView, workspace) ||
		command.RelativePath != request.Input.RelativePath ||
		command.StartByte != request.Input.StartByte ||
		command.MaxBytes != request.Input.MaxBytes ||
		command.RequestedNotAfterUnixNano != request.Input.RequestedNotAfter ||
		command.OperationDigest != string(authorization.OperationDigest) ||
		command.EffectID != string(authorization.Attempt.EffectID) ||
		command.IntentRevision != uint64(authorization.Attempt.IntentRevision) ||
		command.IntentDigest != string(authorization.Attempt.IntentDigest) ||
		command.AttemptID != authorization.Attempt.AttemptID ||
		command.PreparedDigest != string(authorization.Prepared.Digest) ||
		command.DispatchDigest != string(dispatchDigest) ||
		command.TenantID != string(authorization.Operation.ExecutionScope.Identity.TenantID) ||
		command.ProviderComponent != string(authorization.Provider.ComponentID) ||
		command.ProviderManifest != string(authorization.Provider.ManifestDigest) {
		return workspaceReadConflictV1("workspace.read Tool input, Sandbox Command, and Runtime authorization differ")
	}
	return nil
}

func validateWorkspaceReadAdmissionBindingV1(
	binding sandboxports.WorkspaceReadAdmissionAttemptBindingV1,
	admission runtimeports.ControlledOperationProviderAdmissionReceiptRefV2,
	command sandboxcontract.Ref,
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
) error {
	if err := binding.Validate(); err != nil {
		return workspaceReadConflictV1("workspace.read Admission-to-Attempt binding is invalid")
	}
	if binding.AdmissionReceipt != admission ||
		!sandboxcontract.SameRef(binding.Command, command) ||
		binding.AuthorizationDigest != authorization.AuthorizationDigest ||
		binding.StableKeyDigest != authorization.StableKeyDigest ||
		binding.Association != authorization.Association ||
		binding.DomainCommand != authorization.DomainCommand {
		return workspaceReadConflictV1("workspace.read Admission-to-Attempt binding drifted")
	}
	return nil
}

func validateWorkspaceReadReservationTTLClosureV1(
	reservation sandboxcontract.WorkspaceReadReservationV1,
	projection sandboxcontract.WorkspaceReadExecutionProjectionV1,
	binding sandboxports.WorkspaceReadAdmissionAttemptBindingV1,
	command sandboxcontract.WorkspaceReadCommandV1,
	authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3,
) error {
	ttl := reservation.TTLClosure
	expectedAttemptExpiry := ttl.EffectiveExpiresUnixNano
	if projection.AdmissionReceipt.ExpiresUnixNano < expectedAttemptExpiry {
		expectedAttemptExpiry = projection.AdmissionReceipt.ExpiresUnixNano
	}
	if ttl.UnifiedNotAfterUnixNano != authorization.UnifiedNotAfterUnixNano ||
		ttl.RuntimeEnforcementExpiresNano > authorization.ExecuteEnforcement.ExpiresUnixNano ||
		ttl.CommandRequestedNotAfterNano != command.RequestedNotAfterUnixNano ||
		ttl.CommandExpiresUnixNano != command.Meta.ExpiresUnixNano ||
		ttl.EffectiveExpiresUnixNano != reservation.Meta.ExpiresUnixNano ||
		ttl.EffectiveExpiresUnixNano != binding.ExpiresUnixNano ||
		reservation.StableKeyDigest != string(binding.StableKeyDigest) ||
		reservation.StableKeyDigest != string(authorization.StableKeyDigest) ||
		reservation.RequestDigest != command.Meta.Digest ||
		reservation.AttemptID != binding.Attempt.ID ||
		projection.AdmissionReceipt.CheckedUnixNano != binding.CreatedUnixNano ||
		projection.AdmissionReceipt.ExpiresUnixNano != binding.ExpiresUnixNano ||
		projection.Attempt.Meta.ExpiresUnixNano != expectedAttemptExpiry {
		return workspaceReadConflictV1("workspace.read Sandbox Reservation lifetime differs from the historical causal closure")
	}
	return nil
}

func workspaceReadCommandRefV1(ref runtimeports.OperationDomainCommandRefV1) (sandboxcontract.Ref, error) {
	if ref.Validate() != nil || string(ref.Kind) != sandboxcontract.WorkspaceReadCommandKindV1 {
		return sandboxcontract.Ref{}, workspaceReadInvalidV1("workspace.read Runtime Domain Command Ref is invalid")
	}
	digest, err := workspaceReadSandboxDigestV1(ref.Digest)
	if err != nil {
		return sandboxcontract.Ref{}, err
	}
	result := sandboxcontract.Ref{ID: ref.ID, Revision: uint64(ref.Revision), Digest: digest}
	if err = result.ValidateShape("workspace read command"); err != nil {
		return sandboxcontract.Ref{}, workspaceReadInvalidV1("workspace.read Sandbox Command Ref is invalid")
	}
	return result, nil
}

func workspaceReadSandboxRefV1(ref toolcontract.WorkspaceExactRefV1) (sandboxcontract.Ref, error) {
	digest, err := workspaceReadSandboxDigestV1(ref.Digest)
	if err != nil {
		return sandboxcontract.Ref{}, err
	}
	result := sandboxcontract.Ref{ID: ref.ID, Revision: uint64(ref.Revision), Digest: digest}
	if err = result.ValidateShape("workspace view"); err != nil {
		return sandboxcontract.Ref{}, workspaceReadInvalidV1("workspace.read Workspace Ref is invalid")
	}
	return result, nil
}

func workspaceReadToolCommandRefV1(ref toolcontract.ObjectRef) (sandboxcontract.Ref, error) {
	if err := ref.Validate(); err != nil {
		return sandboxcontract.Ref{}, err
	}
	digest, err := workspaceReadSandboxDigestV1(ref.Digest)
	if err != nil {
		return sandboxcontract.Ref{}, err
	}
	result := sandboxcontract.Ref{ID: ref.ID, Revision: uint64(ref.Revision), Digest: digest}
	if err = result.ValidateShape("source Tool command"); err != nil {
		return sandboxcontract.Ref{}, workspaceReadInvalidV1("workspace.read source Tool Command Ref is invalid")
	}
	return result, nil
}

func workspaceReadSchemaValidV1(schema runtimeports.SchemaRefV2) bool {
	return schema.Validate() == nil
}

func workspaceReadSandboxDigestV1(value runtimecore.Digest) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	raw := strings.TrimPrefix(string(value), "sha256:")
	if raw == string(value) || !sandboxcontract.ValidDigest(raw) {
		return "", workspaceReadInvalidV1("workspace.read Runtime digest cannot map to the Sandbox exact Ref")
	}
	return raw, nil
}

func workspaceReadRuntimeDigestV1(value string) (runtimecore.Digest, error) {
	if !sandboxcontract.ValidDigest(value) {
		return "", workspaceReadInvalidV1("workspace.read Sandbox digest cannot map to the Tool exact Ref")
	}
	result := runtimecore.Digest("sha256:" + value)
	return result, result.Validate()
}

func workspaceReadRecoveryContextV1(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

func workspaceReadUnknownV1(message string, cause error) error {
	if cause == nil {
		cause = sandboxports.ErrWorkspaceReadUnknown
	}
	return fmt.Errorf("%w: %s: %v", sandboxports.ErrWorkspaceReadUnknown, message, cause)
}

func workspaceReadInvalidV1(message string) error {
	return runtimecore.NewError(runtimecore.ErrorInvalidArgument, runtimecore.ReasonInvalidReference, message)
}

func workspaceReadConflictV1(message string) error {
	return runtimecore.NewError(runtimecore.ErrorConflict, runtimecore.ReasonBindingDrift, message)
}

func workspaceReadUnavailableV1(message string) error {
	return runtimecore.NewError(runtimecore.ErrorUnavailable, runtimecore.ReasonComponentMissing, message)
}

func workspaceReadNilV1(value any) bool {
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
