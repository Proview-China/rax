package sdk

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// MCPProcessV1 exposes bounded, digest-only process telemetry. It never
// starts a watch goroutine, contacts a Provider, or upgrades an Observation
// into Evidence, Timeline, ToolResult, Review, or authority.
type MCPProcessV1 struct {
	reader toolcontract.MCPProcessObservationReadPortV1
}

func NewMCPProcessV1(reader toolcontract.MCPProcessObservationReadPortV1) (*MCPProcessV1, error) {
	if nilLikeV1(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Process SDK Reader is required")
	}
	return &MCPProcessV1{reader: reader}, nil
}

func (s *MCPProcessV1) InspectMCPProcessObservationV1(ctx context.Context, exact toolcontract.MCPProcessObservationRefV1) (toolcontract.MCPProcessObservationV1, error) {
	if err := s.readyMCPProcessV1(ctx); err != nil {
		return toolcontract.MCPProcessObservationV1{}, err
	}
	if exact.Validate() != nil {
		return toolcontract.MCPProcessObservationV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Process SDK exact Ref is invalid")
	}
	first, err := s.reader.InspectMCPProcessObservationV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPProcessObservationV1{}, err
	}
	second, err := s.reader.InspectMCPProcessObservationV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPProcessObservationV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Ref != exact || second.Ref != exact || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPProcessObservationV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Process SDK exact Observation drifted")
	}
	return second, nil
}

func (s *MCPProcessV1) ReadMCPProcessObservationPageV1(ctx context.Context, request toolcontract.MCPProcessObservationPageRequestV1) (toolcontract.MCPProcessObservationPageV1, error) {
	if err := s.readyMCPProcessV1(ctx); err != nil {
		return toolcontract.MCPProcessObservationPageV1{}, err
	}
	if request.Validate() != nil {
		return toolcontract.MCPProcessObservationPageV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Process SDK page request is invalid")
	}
	page, err := s.reader.ReadMCPProcessObservationPageV1(ctx, request)
	if err != nil {
		return toolcontract.MCPProcessObservationPageV1{}, err
	}
	if page.Validate() != nil || page.Request != request {
		return toolcontract.MCPProcessObservationPageV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Process SDK page drifted")
	}
	return toolcontract.CloneMCPProcessObservationPageV1(page), nil
}

func (s *MCPProcessV1) readyMCPProcessV1(ctx context.Context) error {
	if s == nil || nilLikeV1(s.reader) {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Process SDK is unavailable")
	}
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Process SDK context is required")
	}
	return ctx.Err()
}
