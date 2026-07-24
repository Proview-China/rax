package owneradapter

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// AvailabilityReaderV1 projects Host-owned SystemReady Current through the
// Runtime-neutral reader contract. It owns no second fact or current index.
type AvailabilityReaderV1 struct {
	owner    core.OwnerRef
	currents hostports.SystemReadyAvailabilitySourceV2
}

func NewAvailabilityReaderV1(owner core.OwnerRef, currents hostports.SystemReadyAvailabilitySourceV2) (*AvailabilityReaderV1, error) {
	if err := owner.Validate(); err != nil {
		return nil, err
	}
	if contract.IsTypedNilV1(currents) {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "system_ready_current_reader_missing", "SystemReady Current reader is required")
	}
	return &AvailabilityReaderV1{owner: owner, currents: currents}, nil
}

func (r *AvailabilityReaderV1) InspectAgentExecutionAvailabilityCurrentV1(ctx context.Context, expected runtimeports.AgentExecutionAvailabilityRefV1) (runtimeports.AgentExecutionAvailabilityProjectionV1, error) {
	if r == nil || contract.IsTypedNilV1(r.currents) {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, contract.NewError(contract.ErrorUnavailable, "availability_reader_missing", "availability reader is unavailable")
	}
	if ctx == nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := expected.Validate(); err != nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	if expected.Owner != r.owner {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, contract.NewError(contract.ErrorConflict, "availability_owner_drift", "availability exact Ref belongs to another Owner")
	}
	current, err := r.currents.InspectSystemReadyCurrentForAvailabilityV2(ctx, expected)
	if err != nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	projected, err := current.ToAgentExecutionAvailabilityV1(r.owner)
	if err != nil {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, err
	}
	if projected.Ref != expected {
		return runtimeports.AgentExecutionAvailabilityProjectionV1{}, contract.NewError(contract.ErrorConflict, "availability_projection_drift", "availability projection does not match the exact requested Ref")
	}
	return projected, nil
}

var _ runtimeports.AgentExecutionAvailabilityCurrentReaderV1 = (*AvailabilityReaderV1)(nil)
