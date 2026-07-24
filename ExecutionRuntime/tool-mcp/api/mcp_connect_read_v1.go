package api

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPConnectIntentReadPortV1 interface {
	InspectMCPConnectIntentV1(context.Context, toolcontract.ObjectRef) (toolcontract.MCPConnectIntentV1, error)
}

type MCPConnectReceiptReadPortV1 interface {
	InspectMCPConnectProtocolReceiptV1(context.Context, toolcontract.MCPConnectProtocolReceiptRefV1) (toolcontract.MCPConnectProtocolReceiptV1, error)
}

type MCPConnectionFactReadPortV2 interface {
	InspectMCPConnectionFactV2(context.Context, toolcontract.MCPConnectionFactRefV2) (toolcontract.MCPConnectionFactV2, error)
}

type MCPConnectDomainResultReadPortV1 interface {
	InspectMCPConnectDomainResultV1(context.Context, toolcontract.ObjectRef) (toolcontract.MCPConnectDomainResultFactV1, error)
}

type MCPConnectApplyReadPortV1 interface {
	InspectMCPConnectApplySettlementV1(context.Context, toolcontract.ObjectRef) (toolcontract.MCPConnectApplySettlementFactV1, error)
	InspectCurrentMCPConnectionAvailabilityV1(context.Context, toolcontract.MCPConnectionFactRefV2, time.Duration) (toolcontract.MCPConnectionAvailabilityCurrentProjectionV1, error)
}

// MCPConnectReadV1 exposes only Tool/MCP Owner exact facts. It has no writer,
// Runtime settlement, raw SDK Session, transport, credential, or Provider
// execution method.
type MCPConnectReadV1 struct {
	intents     MCPConnectIntentReadPortV1
	receipts    MCPConnectReceiptReadPortV1
	connections MCPConnectionFactReadPortV2
	domains     MCPConnectDomainResultReadPortV1
	applies     MCPConnectApplyReadPortV1
	clock       func() time.Time
}

func NewMCPConnectReadV1(intents MCPConnectIntentReadPortV1, receipts MCPConnectReceiptReadPortV1, connections MCPConnectionFactReadPortV2, domains MCPConnectDomainResultReadPortV1, applies MCPConnectApplyReadPortV1, clock func() time.Time) (*MCPConnectReadV1, error) {
	for _, dependency := range []any{intents, receipts, connections, domains, applies} {
		if nilLikeMCPConnectReadV1(dependency) {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Connect read API dependencies are required")
		}
	}
	if clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Connect read API clock is required")
	}
	return &MCPConnectReadV1{intents: intents, receipts: receipts, connections: connections, domains: domains, applies: applies, clock: clock}, nil
}

func (a *MCPConnectReadV1) InspectMCPConnectIntentV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPConnectIntentV1, error) {
	if err := a.readyV1(ctx); err != nil {
		return toolcontract.MCPConnectIntentV1{}, err
	}
	value, err := a.intents.InspectMCPConnectIntentV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPConnectIntentV1{}, err
	}
	if value.Validate() != nil || value.Ref != exact {
		return toolcontract.MCPConnectIntentV1{}, driftMCPConnectReadV1("Intent")
	}
	return value, nil
}

func (a *MCPConnectReadV1) InspectMCPConnectProtocolReceiptV1(ctx context.Context, exact toolcontract.MCPConnectProtocolReceiptRefV1) (toolcontract.MCPConnectProtocolReceiptV1, error) {
	if err := a.readyV1(ctx); err != nil {
		return toolcontract.MCPConnectProtocolReceiptV1{}, err
	}
	value, err := a.receipts.InspectMCPConnectProtocolReceiptV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPConnectProtocolReceiptV1{}, err
	}
	if value.Validate() != nil || value.Ref != exact {
		return toolcontract.MCPConnectProtocolReceiptV1{}, driftMCPConnectReadV1("Protocol Receipt")
	}
	return toolcontract.CloneMCPConnectProtocolReceiptV1(value), nil
}

func (a *MCPConnectReadV1) InspectMCPConnectionFactV2(ctx context.Context, exact toolcontract.MCPConnectionFactRefV2) (toolcontract.MCPConnectionFactV2, error) {
	if err := a.readyV1(ctx); err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	value, err := a.connections.InspectMCPConnectionFactV2(ctx, exact)
	if err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	if value.Validate() != nil || value.Ref != exact {
		return toolcontract.MCPConnectionFactV2{}, driftMCPConnectReadV1("Connection Fact")
	}
	return value, nil
}

func (a *MCPConnectReadV1) InspectMCPConnectDomainResultV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPConnectDomainResultFactV1, error) {
	if err := a.readyV1(ctx); err != nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, err
	}
	value, err := a.domains.InspectMCPConnectDomainResultV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPConnectDomainResultFactV1{}, err
	}
	if value.Validate() != nil || value.ObjectRef() != exact {
		return toolcontract.MCPConnectDomainResultFactV1{}, driftMCPConnectReadV1("DomainResult")
	}
	return value, nil
}

func (a *MCPConnectReadV1) InspectMCPConnectApplySettlementV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPConnectApplySettlementFactV1, error) {
	if err := a.readyV1(ctx); err != nil {
		return toolcontract.MCPConnectApplySettlementFactV1{}, err
	}
	value, err := a.applies.InspectMCPConnectApplySettlementV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPConnectApplySettlementFactV1{}, err
	}
	if value.Validate() != nil || value.Ref != exact {
		return toolcontract.MCPConnectApplySettlementFactV1{}, driftMCPConnectReadV1("ApplySettlement")
	}
	return value, nil
}

func (a *MCPConnectReadV1) InspectCurrentMCPConnectionAvailabilityV1(ctx context.Context, exact toolcontract.MCPConnectionFactRefV2, ttl time.Duration) (toolcontract.MCPConnectionAvailabilityCurrentProjectionV1, error) {
	if err := a.readyV1(ctx); err != nil {
		return toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{}, err
	}
	value, err := a.applies.InspectCurrentMCPConnectionAvailabilityV1(ctx, exact, ttl)
	if err != nil {
		return toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{}, err
	}
	now := a.clock()
	if value.Connection != exact || value.Validate(now) != nil {
		return toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{}, driftMCPConnectReadV1("Connection Availability")
	}
	return value, nil
}

func (a *MCPConnectReadV1) readyV1(ctx context.Context) error {
	if a == nil || nilLikeMCPConnectReadV1(a.intents) || nilLikeMCPConnectReadV1(a.receipts) || nilLikeMCPConnectReadV1(a.connections) || nilLikeMCPConnectReadV1(a.domains) || nilLikeMCPConnectReadV1(a.applies) || a.clock == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Connect read API is unavailable")
	}
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Connect read API context is required")
	}
	return ctx.Err()
}

func driftMCPConnectReadV1(kind string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connect read API "+kind+" differs from exact Ref")
}

func nilLikeMCPConnectReadV1(value any) bool {
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
