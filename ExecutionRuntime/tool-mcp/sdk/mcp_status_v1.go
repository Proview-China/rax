package sdk

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
)

type MCPLifecycleReaderV1 interface {
	Inspect(string) (mcp.ConnectionRecord, bool)
}

type MCPStatusV1 struct {
	reader MCPLifecycleReaderV1
	clock  ClockV1
}

func NewMCPStatusV1(reader MCPLifecycleReaderV1, clock ClockV1) (*MCPStatusV1, error) {
	if nilLikeV1(reader) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Status SDK dependencies are required")
	}
	return &MCPStatusV1{reader: reader, clock: clock}, nil
}

func (s *MCPStatusV1) InspectMCPConnectionStatusV1(ctx context.Context, exact toolcontract.ObjectRef) (mcp.ConnectionRecord, error) {
	firstTime, err := s.nowV1(ctx)
	if err != nil {
		return mcp.ConnectionRecord{}, err
	}
	if err := exact.Validate(); err != nil {
		return mcp.ConnectionRecord{}, err
	}
	first, ok := s.reader.Inspect(exact.ID)
	if !ok {
		return mcp.ConnectionRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Connection status is absent")
	}
	if err := validateMCPStatusRecordV1(first, exact, firstTime); err != nil {
		return mcp.ConnectionRecord{}, err
	}
	if err := ctx.Err(); err != nil {
		return mcp.ConnectionRecord{}, err
	}
	secondTime, err := s.nowV1(ctx)
	if err != nil {
		return mcp.ConnectionRecord{}, err
	}
	if secondTime.Before(firstTime) {
		return mcp.ConnectionRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Status SDK clock regressed")
	}
	second, ok := s.reader.Inspect(exact.ID)
	if !ok || !reflect.DeepEqual(first, second) {
		return mcp.ConnectionRecord{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connection status changed during exact inspection")
	}
	if err := validateMCPStatusRecordV1(second, exact, secondTime); err != nil {
		return mcp.ConnectionRecord{}, err
	}
	return cloneMCPStatusRecordV1(second), nil
}

func (s *MCPStatusV1) nowV1(ctx context.Context) (time.Time, error) {
	if s == nil || nilLikeV1(s.reader) || s.clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Status SDK is unavailable")
	}
	if ctx == nil {
		return time.Time{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Status SDK context is required")
	}
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	now := s.clock()
	if now.IsZero() {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Status SDK clock is unavailable")
	}
	return now.UTC(), nil
}

func validateMCPStatusRecordV1(record mcp.ConnectionRecord, exact toolcontract.ObjectRef, now time.Time) error {
	if err := record.Connection.Validate(); err != nil {
		return err
	}
	if record.Connection.ID != exact.ID || record.Connection.Revision != exact.Revision || record.Connection.Digest != exact.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connection status differs from exact Ref")
	}
	if record.Revision == 0 || record.UpdatedUnixNano < record.Connection.CreatedUnixNano || now.UnixNano() < record.UpdatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Connection status time or revision is invalid")
	}
	if !validMCPConnectionStateV1(record.State) {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "MCP Connection status state is invalid")
	}
	if record.Snapshot == nil {
		if record.SnapshotState != "" {
			return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connection status has Snapshot state without Snapshot")
		}
		return nil
	}
	if err := record.Snapshot.Validate(); err != nil || !validMCPSnapshotStateV1(record.SnapshotState) {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connection status Snapshot is non-canonical")
	}
	return nil
}

func validMCPConnectionStateV1(state mcp.ConnectionState) bool {
	switch state {
	case mcp.ConnectionRegistered, mcp.ConnectionResolving, mcp.ConnectionConnecting, mcp.ConnectionInitializing, mcp.ConnectionDiscovering, mcp.ConnectionBound, mcp.ConnectionDegraded, mcp.ConnectionDraining, mcp.ConnectionClosed, mcp.ConnectionUnknown:
		return true
	default:
		return false
	}
}

func validMCPSnapshotStateV1(state mcp.SnapshotState) bool {
	switch state {
	case mcp.SnapshotObserved, mcp.SnapshotValidated, mcp.SnapshotAdmitted, mcp.SnapshotActive, mcp.SnapshotSuperseded, mcp.SnapshotRevoked, mcp.SnapshotExpired:
		return true
	default:
		return false
	}
}

func cloneMCPStatusRecordV1(record mcp.ConnectionRecord) mcp.ConnectionRecord {
	if record.Snapshot != nil {
		value := *record.Snapshot
		value.Tools = append([]toolcontract.MCPToolObservation(nil), value.Tools...)
		value.Residuals = append([]toolcontract.Residual(nil), value.Residuals...)
		record.Snapshot = &value
	}
	return record
}
