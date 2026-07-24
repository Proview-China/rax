package apihandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/api"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/sdk"
)

type ControllerSDKV1 interface {
	DescribeBackends() ([]contract.BackendDescriptor, error)
	MatchRequirement(contract.ExecutionRequirement, contract.PolicyProjection, []sdk.PlacementInput) ([]sdk.PlacementResult, error)
	StartLifecycle(context.Context, applicationcontract.SandboxLifecycleRequestV4) (applicationcontract.SandboxLifecycleResultV4, error)
	InspectLifecycle(context.Context, applicationcontract.SandboxLifecycleRequestV4) (applicationcontract.SandboxLifecycleResultV4, error)
	CaptureWorkspaceChangeSet(context.Context, ports.CaptureWorkspaceChangeSetRequest) (contract.WorkspaceChangeSet, error)
	CompleteCheckpointParticipant(context.Context, applicationcontract.CheckpointParticipantWorkRequestV1) (applicationcontract.CheckpointParticipantCommitV1, error)
	InspectCheckpointParticipant(context.Context, runtimeports.CheckpointAttemptRefV2, runtimeports.CheckpointParticipantRefV2) (applicationcontract.CheckpointParticipantCommitV1, error)
	StageWorkspaceRestore(context.Context, *contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreStageFactV1, error)
	ReconcileWorkspaceRestore(context.Context, *contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreStageFactV1, error)
	ComposeWorkspaceRewindV1(context.Context, contract.ComposeWorkspaceRewindRequestV1) (ports.WorkspaceRewindCompositionResultV1, error)
	InspectWorkspaceRewindV1(context.Context, contract.ComposeWorkspaceRewindRequestV1) (ports.WorkspaceRewindCompositionResultV1, error)
	InspectReservation(context.Context, string) (contract.DomainReservation, error)
	InspectDomainResult(context.Context, string) (contract.SandboxDomainResultFact, error)
	InspectEnvironment(context.Context, string) (contract.EnvironmentProjection, error)
	InspectOperation(context.Context, string) (contract.InspectionFact, error)
	InspectCleanup(context.Context, string) (contract.CleanupReport, error)
	InspectResiduals(context.Context, string) ([]contract.Residual, error)
}

type SDKHandlerV1 struct {
	client ControllerSDKV1
}

func NewSDKHandlerV1(client ControllerSDKV1) (*SDKHandlerV1, error) {
	if nilLikeV1(client) {
		return nil, errors.New("Sandbox API SDK handler requires a governed Controller SDK")
	}
	return &SDKHandlerV1{client: client}, nil
}

func (h *SDKHandlerV1) Execute(ctx context.Context, request api.OperationRequestV1) (api.HandlerOutcomeV1, error) {
	if err := request.ValidateShape(); err != nil {
		return failedV1("invalid_request", err), nil
	}
	var value any
	var err error
	switch request.Action {
	case api.ActionDescribeBackendsV1:
		if err = decodePayloadV1(request, &struct{}{}); err == nil {
			value, err = h.client.DescribeBackends()
		}
	case api.ActionMatchRequirementV1:
		var input MatchRequirementRequestV1
		if err = decodePayloadV1(request, &input); err == nil {
			value, err = h.client.MatchRequirement(input.Requirement, input.Policy, input.Inputs)
		}
	case api.ActionInspectV1:
		var input InspectRequestV1
		if err = decodePayloadV1(request, &input); err == nil {
			value, err = h.inspect(ctx, input)
		}
	case api.ActionWorkspaceDiffV1:
		var input ports.CaptureWorkspaceChangeSetRequest
		if err = decodePayloadV1(request, &input); err == nil {
			value, err = h.client.CaptureWorkspaceChangeSet(ctx, input)
		}
	case api.ActionLifecycleV1, api.ActionFenceV1, api.ActionWorkspaceCommitV1, api.ActionReleaseV1, api.ActionCleanupV1:
		var input applicationcontract.SandboxLifecycleRequestV4
		if err = decodePayloadV1(request, &input); err == nil {
			value, err = h.client.StartLifecycle(ctx, input)
		}
	case api.ActionCheckpointV1:
		var input applicationcontract.CheckpointParticipantWorkRequestV1
		if err = decodePayloadV1(request, &input); err == nil {
			value, err = h.client.CompleteCheckpointParticipant(ctx, input)
		}
	case api.ActionWorkspaceRestoreV1:
		var input contract.WorkspaceRestoreStageRequestV1
		if err = decodePayloadV1(request, &input); err == nil {
			value, err = h.client.StageWorkspaceRestore(ctx, &input)
		}
	case api.ActionWorkspaceRewindV1:
		var input contract.ComposeWorkspaceRewindRequestV1
		if err = decodePayloadV1(request, &input); err == nil {
			value, err = h.client.ComposeWorkspaceRewindV1(ctx, input)
		}
	default:
		err = fmt.Errorf("unsupported API action %q", request.Action)
	}
	if err != nil {
		if payloadContractErrorV1(err) {
			return failedV1("invalid_payload", err), nil
		}
		return api.HandlerOutcomeV1{}, err
	}
	return succeededV1(request.Action, value)
}

func (h *SDKHandlerV1) Reconcile(ctx context.Context, request api.OperationRequestV1) (api.HandlerOutcomeV1, error) {
	if err := request.ValidateShape(); err != nil {
		return failedV1("invalid_request", err), nil
	}
	var value any
	var err error
	switch request.Action {
	case api.ActionLifecycleV1, api.ActionFenceV1, api.ActionWorkspaceCommitV1, api.ActionReleaseV1, api.ActionCleanupV1:
		var input applicationcontract.SandboxLifecycleRequestV4
		if err = decodePayloadV1(request, &input); err == nil {
			value, err = h.client.InspectLifecycle(ctx, input)
		}
	case api.ActionCheckpointV1:
		var input applicationcontract.CheckpointParticipantWorkRequestV1
		if err = decodePayloadV1(request, &input); err == nil {
			value, err = h.client.InspectCheckpointParticipant(ctx, input.Attempt, input.Participant)
		}
	case api.ActionWorkspaceRestoreV1:
		var input contract.WorkspaceRestoreStageRequestV1
		if err = decodePayloadV1(request, &input); err == nil {
			value, err = h.client.ReconcileWorkspaceRestore(ctx, &input)
		}
	case api.ActionWorkspaceRewindV1:
		var input contract.ComposeWorkspaceRewindRequestV1
		if err = decodePayloadV1(request, &input); err == nil {
			value, err = h.client.InspectWorkspaceRewindV1(ctx, input)
		}
	default:
		return api.HandlerOutcomeV1{}, fmt.Errorf("API action %q has no authoritative recovery Inspect", request.Action)
	}
	if err != nil {
		return api.HandlerOutcomeV1{}, err
	}
	return succeededV1(request.Action, value)
}

type MatchRequirementRequestV1 struct {
	Requirement contract.ExecutionRequirement `json:"requirement"`
	Policy      contract.PolicyProjection     `json:"policy"`
	Inputs      []sdk.PlacementInput          `json:"inputs"`
}

type InspectKindV1 string

const (
	InspectReservationV1  InspectKindV1 = "reservation"
	InspectDomainResultV1 InspectKindV1 = "domain_result"
	InspectEnvironmentV1  InspectKindV1 = "environment"
	InspectOperationV1    InspectKindV1 = "operation"
	InspectCleanupV1      InspectKindV1 = "cleanup"
	InspectResidualsV1    InspectKindV1 = "residuals"
)

type InspectRequestV1 struct {
	Kind InspectKindV1 `json:"kind"`
	ID   string        `json:"id"`
}

func (h *SDKHandlerV1) inspect(ctx context.Context, input InspectRequestV1) (any, error) {
	if input.ID == "" {
		return nil, errors.New("inspect ID is required")
	}
	switch input.Kind {
	case InspectReservationV1:
		return h.client.InspectReservation(ctx, input.ID)
	case InspectDomainResultV1:
		return h.client.InspectDomainResult(ctx, input.ID)
	case InspectEnvironmentV1:
		return h.client.InspectEnvironment(ctx, input.ID)
	case InspectOperationV1:
		return h.client.InspectOperation(ctx, input.ID)
	case InspectCleanupV1:
		return h.client.InspectCleanup(ctx, input.ID)
	case InspectResidualsV1:
		return h.client.InspectResiduals(ctx, input.ID)
	default:
		return nil, fmt.Errorf("unsupported inspect kind %q", input.Kind)
	}
}

func decodePayloadV1(request api.OperationRequestV1, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(request.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("payload decode: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("payload contains trailing JSON")
	}
	return nil
}

func succeededV1(action api.ActionV1, value any) (api.HandlerOutcomeV1, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return api.HandlerOutcomeV1{}, err
	}
	result, err := api.SealResultV1(api.ResultV1{Schema: string(action) + "/result-v1", Revision: 1, Payload: payload})
	if err != nil {
		return api.HandlerOutcomeV1{}, err
	}
	return api.HandlerOutcomeV1{State: api.OperationSucceededV1, Result: &result}, nil
}

func failedV1(reason string, err error) api.HandlerOutcomeV1 {
	return api.HandlerOutcomeV1{State: api.OperationFailedV1, Error: &api.ClosedErrorV1{Category: "invalid_argument", Reason: reason, Message: err.Error()}}
}

func payloadContractErrorV1(err error) bool {
	message := err.Error()
	return strings.Contains(message, "payload") || strings.Contains(message, "unsupported API action") || strings.Contains(message, "inspect ID") || strings.Contains(message, "inspect kind")
}

func nilLikeV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

var _ api.GovernedHandlerV1 = (*SDKHandlerV1)(nil)
