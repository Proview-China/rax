package sdk

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// MCPDiscoveryV3 provides an exact, double-read SDK view over provenance-bound
// snapshots. It does not execute discovery or grant mapping/admission rights.
type MCPDiscoveryV3 struct {
	reader toolcontract.MCPCapabilitySnapshotExactReaderV3
	clock  ClockV1
}

func NewMCPDiscoveryV3(reader toolcontract.MCPCapabilitySnapshotExactReaderV3, clock ClockV1) (*MCPDiscoveryV3, error) {
	if nilLikeV1(reader) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Discovery V3 SDK dependencies are required")
	}
	return &MCPDiscoveryV3{reader: reader, clock: clock}, nil
}

func (s *MCPDiscoveryV3) InspectCurrentMCPCapabilitySnapshotV3(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error) {
	firstTime, err := s.nowV3(ctx)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, err
	}
	first, err := s.reader.InspectMCPCapabilitySnapshotV3(ctx, exact)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, err
	}
	if first.ObjectRef() != exact || first.ValidateCurrent(firstTime) != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery V3 SDK Snapshot is not exact-current")
	}
	second, err := s.reader.InspectMCPCapabilitySnapshotV3(ctx, exact)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, err
	}
	secondTime, err := s.nowV3(ctx)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, err
	}
	if secondTime.Before(firstTime) {
		return toolcontract.MCPCapabilitySnapshotV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Discovery V3 SDK clock regressed")
	}
	if second.ObjectRef() != exact || second.ValidateCurrent(secondTime) != nil || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPCapabilitySnapshotV3{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery V3 SDK Snapshot drifted during inspection")
	}
	return toolcontract.CloneMCPCapabilitySnapshotV3(second), nil
}

func (s *MCPDiscoveryV3) nowV3(ctx context.Context) (time.Time, error) {
	if s == nil || nilLikeV1(s.reader) || s.clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Discovery V3 SDK is unavailable")
	}
	if ctx == nil {
		return time.Time{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Discovery V3 SDK context is required")
	}
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	now := s.clock()
	if now.IsZero() {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Discovery V3 SDK clock is unavailable")
	}
	return now, nil
}
