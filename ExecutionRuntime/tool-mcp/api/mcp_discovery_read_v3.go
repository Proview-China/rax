package api

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPDiscoveryReadV3 struct {
	snapshots toolcontract.MCPCapabilitySnapshotExactReaderV3
	clock     func() time.Time
}

func NewMCPDiscoveryReadV3(snapshots toolcontract.MCPCapabilitySnapshotExactReaderV3, clock func() time.Time) (*MCPDiscoveryReadV3, error) {
	if nilLikeMCPDiscoveryReadV2(snapshots) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Discovery V3 read dependencies are required")
	}
	return &MCPDiscoveryReadV3{snapshots: snapshots, clock: clock}, nil
}

func (a *MCPDiscoveryReadV3) InspectCurrentMCPCapabilitySnapshotV3(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error) {
	if ctx == nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Discovery V3 API context is required")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, err
	}
	if a == nil || nilLikeMCPDiscoveryReadV2(a.snapshots) || a.clock == nil || exact.Validate() != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Discovery V3 read API is unavailable or invalid")
	}
	firstTime := a.clock()
	if firstTime.IsZero() {
		return toolcontract.MCPCapabilitySnapshotV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Discovery V3 read API clock is unavailable")
	}
	first, err := a.snapshots.InspectMCPCapabilitySnapshotV3(ctx, exact)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, err
	}
	if first.ObjectRef() != exact || first.ValidateCurrent(firstTime) != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery V3 read API Snapshot is not exact-current")
	}
	second, err := a.snapshots.InspectMCPCapabilitySnapshotV3(ctx, exact)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, err
	}
	secondTime := a.clock()
	if secondTime.IsZero() || secondTime.Before(firstTime) {
		return toolcontract.MCPCapabilitySnapshotV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Discovery V3 read API clock regressed")
	}
	if second.ObjectRef() != exact || second.ValidateCurrent(secondTime) != nil || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPCapabilitySnapshotV3{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery V3 read API Snapshot drifted")
	}
	return toolcontract.CloneMCPCapabilitySnapshotV3(second), nil
}
