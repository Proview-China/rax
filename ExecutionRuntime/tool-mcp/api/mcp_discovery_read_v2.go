package api

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// MCPDiscoveryReadV2 exposes only an exact-current Tool/MCP Owner Capability
// Snapshot. It has no discovery writer, Provider Session, transport, Runtime
// settlement, or production composition method.
type MCPDiscoveryReadV2 struct {
	snapshots toolcontract.MCPCapabilitySnapshotExactReaderV2
	clock     func() time.Time
}

func NewMCPDiscoveryReadV2(snapshots toolcontract.MCPCapabilitySnapshotExactReaderV2, clock func() time.Time) (*MCPDiscoveryReadV2, error) {
	if nilLikeMCPDiscoveryReadV2(snapshots) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Discovery read API dependencies are required")
	}
	return &MCPDiscoveryReadV2{snapshots: snapshots, clock: clock}, nil
}

func (a *MCPDiscoveryReadV2) InspectCurrentMCPCapabilitySnapshotV2(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV2, error) {
	if err := a.readyV2(ctx); err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	firstTime := a.clock()
	first, err := a.snapshots.InspectMCPCapabilitySnapshotV2(ctx, exact)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	if first.ObjectRef() != exact || first.ValidateCurrent(firstTime) != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, driftMCPDiscoveryReadV2()
	}
	second, err := a.snapshots.InspectMCPCapabilitySnapshotV2(ctx, exact)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	secondTime := a.clock()
	if secondTime.IsZero() || secondTime.Before(firstTime) {
		return toolcontract.MCPCapabilitySnapshotV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Discovery read API clock regressed")
	}
	if second.ObjectRef() != exact || second.ValidateCurrent(secondTime) != nil || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPCapabilitySnapshotV2{}, driftMCPDiscoveryReadV2()
	}
	return toolcontract.CloneMCPCapabilitySnapshotV2(second), nil
}

func (a *MCPDiscoveryReadV2) readyV2(ctx context.Context) error {
	if a == nil || nilLikeMCPDiscoveryReadV2(a.snapshots) || a.clock == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Discovery read API is unavailable")
	}
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Discovery read API context is required")
	}
	return ctx.Err()
}

func driftMCPDiscoveryReadV2() error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery read API Snapshot differs from exact-current Ref")
}

func nilLikeMCPDiscoveryReadV2(value any) bool {
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
