package sdk

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// MCPDiscoveryV2 is a read-only SDK projection over Tool/MCP Owner snapshots.
// Discovery execution and production composition remain outside this SDK.
type MCPDiscoveryV2 struct {
	reader toolcontract.MCPCapabilitySnapshotExactReaderV2
	clock  ClockV1
}

func NewMCPDiscoveryV2(reader toolcontract.MCPCapabilitySnapshotExactReaderV2, clock ClockV1) (*MCPDiscoveryV2, error) {
	if nilLikeV1(reader) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Discovery SDK dependencies are required")
	}
	return &MCPDiscoveryV2{reader: reader, clock: clock}, nil
}

func (s *MCPDiscoveryV2) InspectCurrentMCPCapabilitySnapshotV2(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV2, error) {
	firstTime, err := s.nowV2(ctx)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	first, err := s.reader.InspectMCPCapabilitySnapshotV2(ctx, exact)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	if first.ObjectRef() != exact || first.ValidateCurrent(firstTime) != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery SDK Snapshot is not exact-current")
	}
	second, err := s.reader.InspectMCPCapabilitySnapshotV2(ctx, exact)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	secondTime, err := s.nowV2(ctx)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	if secondTime.Before(firstTime) {
		return toolcontract.MCPCapabilitySnapshotV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Discovery SDK clock regressed")
	}
	if second.ObjectRef() != exact || second.ValidateCurrent(secondTime) != nil || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPCapabilitySnapshotV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery SDK Snapshot drifted during inspection")
	}
	return toolcontract.CloneMCPCapabilitySnapshotV2(second), nil
}

func (s *MCPDiscoveryV2) nowV2(ctx context.Context) (time.Time, error) {
	if s == nil || nilLikeV1(s.reader) || s.clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Discovery SDK is unavailable")
	}
	if ctx == nil {
		return time.Time{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Discovery SDK context is required")
	}
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	now := s.clock()
	if now.IsZero() {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Discovery SDK clock is unavailable")
	}
	return now, nil
}
