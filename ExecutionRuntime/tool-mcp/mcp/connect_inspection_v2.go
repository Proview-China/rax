package mcp

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPConnectionFactRepositoryV2 interface {
	toolcontract.MCPConnectionFactExactReaderV2
	toolcontract.MCPConnectionFactCurrentReaderV2
	CreateMCPConnectionFactV2(context.Context, toolcontract.MCPConnectionFactV2) (toolcontract.MCPConnectionFactV2, error)
}

type InMemoryMCPConnectionFactRepositoryV2 struct {
	mu     sync.RWMutex
	values map[string]toolcontract.MCPConnectionFactV2
	clock  func() time.Time
}

func NewInMemoryMCPConnectionFactRepositoryV2(clock func() time.Time) (*InMemoryMCPConnectionFactRepositoryV2, error) {
	if clock == nil {
		return nil, invalid("MCP Connection Fact V2 repository clock is missing")
	}
	return &InMemoryMCPConnectionFactRepositoryV2{values: make(map[string]toolcontract.MCPConnectionFactV2), clock: clock}, nil
}

func (r *InMemoryMCPConnectionFactRepositoryV2) CreateMCPConnectionFactV2(ctx context.Context, fact toolcontract.MCPConnectionFactV2) (toolcontract.MCPConnectionFactV2, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	if r == nil || r.clock == nil || fact.Validate() != nil {
		return toolcontract.MCPConnectionFactV2{}, invalid("MCP Connection Fact V2 create is invalid")
	}
	now := r.clock()
	if now.IsZero() || now.UnixNano() < fact.CreatedUnixNano {
		return toolcontract.MCPConnectionFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Connection Fact V2 create clock regressed")
	}
	if !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return toolcontract.MCPConnectionFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "expired MCP Connection Fact V2 cannot be created")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	if winner, ok := r.values[fact.Ref.ID]; ok {
		if !reflect.DeepEqual(winner, fact) {
			return toolcontract.MCPConnectionFactV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Connection Fact V2 ID binds another fact")
		}
		return winner, nil
	}
	r.values[fact.Ref.ID] = fact
	return fact, nil
}

func (r *InMemoryMCPConnectionFactRepositoryV2) InspectMCPConnectionFactV2(ctx context.Context, exact toolcontract.MCPConnectionFactRefV2) (toolcontract.MCPConnectionFactV2, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	if r == nil || exact.Validate() != nil {
		return toolcontract.MCPConnectionFactV2{}, invalid("MCP Connection Fact V2 exact Inspect is invalid")
	}
	r.mu.RLock()
	fact, ok := r.values[exact.ID]
	r.mu.RUnlock()
	if !ok {
		return toolcontract.MCPConnectionFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Connection Fact V2 not found")
	}
	if fact.Ref != exact {
		return toolcontract.MCPConnectionFactV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connection Fact V2 exact Ref drifted")
	}
	return fact, nil
}

func (r *InMemoryMCPConnectionFactRepositoryV2) InspectCurrentMCPConnectionFactV2(ctx context.Context, exact toolcontract.MCPConnectionFactRefV2) (toolcontract.MCPConnectionFactV2, error) {
	fact, err := r.InspectMCPConnectionFactV2(ctx, exact)
	if err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	if r.clock == nil {
		return toolcontract.MCPConnectionFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Connection Fact V2 current clock is unavailable")
	}
	now := r.clock()
	if now.IsZero() || now.UnixNano() < fact.CreatedUnixNano {
		return toolcontract.MCPConnectionFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Connection Fact V2 current clock regressed")
	}
	if !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return toolcontract.MCPConnectionFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Connection Fact V2 expired")
	}
	return fact, nil
}

type InspectMCPConnectReceiptRequestV2 struct {
	Receipt                  toolcontract.MCPConnectProtocolReceiptRefV1 `json:"receipt"`
	RequestedExpiresUnixNano int64                                       `json:"requested_expires_unix_nano"`
}

type MCPConnectReceiptInspectorV2 struct {
	physical *InMemoryMCPConnectPhysicalRepositoryV1
	intents  MCPConnectIntentReaderV1
	configs  MCPTransportConfigReaderV1
	servers  MCPServerDescriptorReaderV1
	facts    MCPConnectionFactRepositoryV2
	clock    func() time.Time
}

func NewMCPConnectReceiptInspectorV2(physical *InMemoryMCPConnectPhysicalRepositoryV1, intents MCPConnectIntentReaderV1, configs MCPTransportConfigReaderV1, servers MCPServerDescriptorReaderV1, facts MCPConnectionFactRepositoryV2, clock func() time.Time) (*MCPConnectReceiptInspectorV2, error) {
	if physical == nil || nilLikeOfficialSDKConnectV1(intents) || nilLikeOfficialSDKConnectV1(configs) || nilLikeOfficialSDKConnectV1(servers) || nilLikeOfficialSDKConnectV1(facts) || clock == nil {
		return nil, invalid("MCP Connect Receipt Inspector V2 dependencies are incomplete")
	}
	return &MCPConnectReceiptInspectorV2{physical: physical, intents: intents, configs: configs, servers: servers, facts: facts, clock: clock}, nil
}

func (i *MCPConnectReceiptInspectorV2) InspectAndCreateMCPConnectionFactV2(ctx context.Context, request InspectMCPConnectReceiptRequestV2) (toolcontract.MCPConnectionFactV2, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	if i == nil || i.physical == nil || nilLikeOfficialSDKConnectV1(i.intents) || nilLikeOfficialSDKConnectV1(i.configs) || nilLikeOfficialSDKConnectV1(i.servers) || nilLikeOfficialSDKConnectV1(i.facts) || i.clock == nil {
		return toolcontract.MCPConnectionFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Connect Receipt Inspector V2 is unavailable")
	}
	if request.Receipt.Validate() != nil || request.RequestedExpiresUnixNano <= 0 {
		return toolcontract.MCPConnectionFactV2{}, invalid("MCP Connect Receipt Inspect request is invalid")
	}
	previous, err := i.freshV2(time.Time{})
	if err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	entry, session, err := i.physical.inspectMCPConnectSessionV1(ctx, request.Receipt)
	if err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	if entry.ProtocolReceipt == nil {
		return toolcontract.MCPConnectionFactV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Connect Receipt closure is absent")
	}
	receipt := toolcontract.CloneMCPConnectProtocolReceiptV1(*entry.ProtocolReceipt)
	intent, config, server, err := i.inspectExactCurrentV2(ctx, receipt)
	if err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	afterReads, err := i.freshV2(previous)
	if err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	if err = validateMCPConnectReceiptClosureV2(entry, session, receipt, intent, config, server, afterReads); err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	entry2, session2, err := i.physical.inspectMCPConnectSessionV1(ctx, request.Receipt)
	if err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	intent2, config2, server2, err := i.inspectExactCurrentV2(ctx, receipt)
	if err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	actual, err := i.freshV2(afterReads)
	if err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	if !reflect.DeepEqual(entry, entry2) || !sameOfficialSDKConnectSessionV1(session, session2) || !reflect.DeepEqual(intent, intent2) || !reflect.DeepEqual(config, config2) || !reflect.DeepEqual(server, server2) {
		return toolcontract.MCPConnectionFactV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connect Receipt closure drifted between S1 and S2")
	}
	if err = validateMCPConnectReceiptClosureV2(entry2, session2, receipt, intent2, config2, server2, actual); err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	expires := minUnixNanoV2(request.RequestedExpiresUnixNano, intent2.NotAfterUnixNano, entry2.NotAfterUnixNano)
	if !actual.Before(time.Unix(0, expires)) {
		return toolcontract.MCPConnectionFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Connection Fact V2 requested window expired")
	}
	fact, err := toolcontract.SealMCPConnectionFactV2(toolcontract.MCPConnectionFactV2{
		Owner: intent2.Owner, Coordinate: intent2.Coordinate, Intent: intent2.Ref,
		TransportConfig: config2.Ref, Server: intent2.Server, ProtocolReceipt: receipt.Ref,
		ProviderTransport: intent2.ProviderTransport, Provider: intent2.Provider,
		NegotiatedProtocol: receipt.NegotiatedProtocol, ProviderSessionID: receipt.ProviderSessionID,
		CreatedUnixNano: receipt.ObservedUnixNano, ExpiresUnixNano: expires,
	})
	if err != nil {
		return toolcontract.MCPConnectionFactV2{}, err
	}
	return i.facts.CreateMCPConnectionFactV2(ctx, fact)
}

func (i *MCPConnectReceiptInspectorV2) inspectExactCurrentV2(ctx context.Context, receipt toolcontract.MCPConnectProtocolReceiptV1) (toolcontract.MCPConnectIntentV1, toolcontract.MCPTransportConfigV1, toolcontract.MCPServerDescriptor, error) {
	intent, err := i.intents.InspectMCPConnectIntentV1(ctx, receipt.Intent)
	if err != nil {
		return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, err
	}
	intentCurrent, err := i.intents.InspectCurrentMCPConnectIntentV1(ctx, intent.Ref.ID)
	if err != nil || !reflect.DeepEqual(intent, intentCurrent) {
		if err != nil {
			return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, err
		}
		return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Connect Intent drifted during Receipt Inspect")
	}
	config, err := i.configs.InspectMCPTransportConfigV1(ctx, receipt.TransportConfig)
	if err != nil {
		return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, err
	}
	configCurrent, err := i.configs.InspectCurrentMCPTransportConfigV1(ctx, config.Ref.ID)
	if err != nil || !reflect.DeepEqual(config, configCurrent) {
		if err != nil {
			return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, err
		}
		return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Transport Config drifted during Receipt Inspect")
	}
	server, err := i.servers.InspectMCPServerDescriptorV1(ctx, intent.Server)
	if err != nil {
		return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, err
	}
	serverCurrent, err := i.servers.InspectCurrentMCPServerDescriptorV1(ctx, server.ID)
	if err != nil || !reflect.DeepEqual(server, serverCurrent) {
		if err != nil {
			return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, err
		}
		return toolcontract.MCPConnectIntentV1{}, toolcontract.MCPTransportConfigV1{}, toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Server Descriptor drifted during Receipt Inspect")
	}
	return intent, config, server, nil
}

func (i *MCPConnectReceiptInspectorV2) freshV2(previous time.Time) (time.Time, error) {
	now := i.clock()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "MCP Connect Receipt Inspector V2 clock regressed")
	}
	return now, nil
}

func validateMCPConnectReceiptClosureV2(entry MCPConnectPhysicalEntryV1, session OfficialSDKConnectSessionV1, receipt toolcontract.MCPConnectProtocolReceiptV1, intent toolcontract.MCPConnectIntentV1, config toolcontract.MCPTransportConfigV1, server toolcontract.MCPServerDescriptor, now time.Time) error {
	if entry.State != MCPConnectPhysicalObservedV1 || entry.ProtocolReceipt == nil || entry.ProtocolReceipt.Ref != receipt.Ref || receipt.Validate() != nil || nilLikeOfficialSDKConnectV1(session) || intent.Validate() != nil || config.Validate() != nil || server.Validate() != nil {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Connect Receipt closure is invalid")
	}
	if now.IsZero() || now.UnixNano() < receipt.ObservedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Connect Receipt Inspect clock regressed")
	}
	if !now.Before(time.Unix(0, intent.NotAfterUnixNano)) || !now.Before(time.Unix(0, entry.NotAfterUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Connect Receipt closure expired")
	}
	initialize := session.InitializeResult()
	if initialize == nil || initialize.Capabilities == nil || initialize.ProtocolVersion != receipt.NegotiatedProtocol || session.ID() != receipt.ProviderSessionID || receipt.Intent != intent.Ref || receipt.TransportConfig != config.Ref || intent.TransportConfig != config.Ref || intent.Server != config.Server || intent.Server != (toolcontract.ObjectRef{ID: server.ID, Revision: server.Revision, Digest: server.Digest}) || receipt.TransportKind != config.Kind {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Connect Receipt exact causal bindings drifted")
	}
	if err := validateMCPNegotiatedProtocolV1(server, receipt.NegotiatedProtocol); err != nil {
		return err
	}
	response, err := json.Marshal(initialize)
	if err != nil || core.DigestBytes(response) != receipt.ResponseDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "MCP Connect initialize observation digest drifted")
	}
	return nil
}

func minUnixNanoV2(values ...int64) int64 {
	result := int64(0)
	for _, value := range values {
		if value > 0 && (result == 0 || value < result) {
			result = value
		}
	}
	return result
}
