package mcp

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPProcessObservationReaderV1 interface {
	toolcontract.MCPProcessObservationReadPortV1
}

type MCPProcessObservationSinkV1 interface {
	RecordMCPProcessObservationV1(context.Context, toolcontract.MCPConnectionRef, toolcontract.ObjectRef, toolcontract.MCPProcessObservationInputV1) (toolcontract.MCPProcessObservationV1, error)
}

type InMemoryMCPProcessObservationJournalV1 struct {
	mu      sync.RWMutex
	clock   func() time.Time
	next    map[mcpProcessConnectionKeyV1]uint64
	records map[string]toolcontract.MCPProcessObservationV1
	streams map[mcpProcessStreamKeyV1][]toolcontract.MCPProcessObservationRefV1
}

type mcpProcessConnectionKeyV1 struct {
	ID       string
	Revision core.Revision
	Digest   core.Digest
	Epoch    core.Epoch
}

type mcpProcessStreamKeyV1 struct {
	Connection mcpProcessConnectionKeyV1
	Snapshot   toolcontract.ObjectRef
}

func NewInMemoryMCPProcessObservationJournalV1(clock func() time.Time) (*InMemoryMCPProcessObservationJournalV1, error) {
	if clock == nil {
		return nil, invalid("MCP process Observation journal clock is missing")
	}
	return &InMemoryMCPProcessObservationJournalV1{
		clock: clock, next: make(map[mcpProcessConnectionKeyV1]uint64),
		records: make(map[string]toolcontract.MCPProcessObservationV1), streams: make(map[mcpProcessStreamKeyV1][]toolcontract.MCPProcessObservationRefV1),
	}, nil
}

func (j *InMemoryMCPProcessObservationJournalV1) RecordMCPProcessObservationV1(ctx context.Context, connection toolcontract.MCPConnectionRef, snapshot toolcontract.ObjectRef, input toolcontract.MCPProcessObservationInputV1) (toolcontract.MCPProcessObservationV1, error) {
	if err := officialSDKContextV1(ctx); err != nil {
		return toolcontract.MCPProcessObservationV1{}, err
	}
	if j == nil || j.clock == nil {
		return toolcontract.MCPProcessObservationV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP process Observation journal is unavailable")
	}
	if connection.Validate() != nil || snapshot.Validate() != nil || input.Validate() != nil {
		return toolcontract.MCPProcessObservationV1{}, invalid("MCP process Observation record request is invalid")
	}
	now := j.clock()
	if now.IsZero() || now.UnixNano() < connection.CreatedUnixNano {
		return toolcontract.MCPProcessObservationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP process Observation journal clock regressed")
	}
	if !now.Before(time.Unix(0, connection.ExpiresUnixNano)) {
		return toolcontract.MCPProcessObservationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP process Observation Connection expired")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	connectionKey := mcpProcessConnectionKeyV1{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest, Epoch: connection.Epoch}
	sequence := j.next[connectionKey] + 1
	observation, err := toolcontract.SealMCPProcessObservationV1(toolcontract.MCPProcessObservationV1{
		Connection: connection, Snapshot: snapshot, Kind: input.Kind, SourceSequence: sequence,
		CorrelationDigest: input.CorrelationDigest, PayloadDigest: input.PayloadDigest,
		LoggingLevel: input.LoggingLevel, Logger: input.Logger, Progress: input.Progress, Total: input.Total,
		ObservedUnixNano: now.UnixNano(),
	})
	if err != nil {
		return toolcontract.MCPProcessObservationV1{}, err
	}
	j.next[connectionKey] = sequence
	j.records[observation.Ref.ID] = observation
	streamKey := mcpProcessStreamKeyV1{Connection: connectionKey, Snapshot: snapshot}
	j.streams[streamKey] = append(j.streams[streamKey], observation.Ref)
	return observation, nil
}

func (j *InMemoryMCPProcessObservationJournalV1) InspectMCPProcessObservationV1(ctx context.Context, exact toolcontract.MCPProcessObservationRefV1) (toolcontract.MCPProcessObservationV1, error) {
	if err := officialSDKContextV1(ctx); err != nil {
		return toolcontract.MCPProcessObservationV1{}, err
	}
	if j == nil || exact.Validate() != nil {
		return toolcontract.MCPProcessObservationV1{}, invalid("MCP process Observation exact Inspect is invalid")
	}
	j.mu.RLock()
	value, ok := j.records[exact.ID]
	j.mu.RUnlock()
	if !ok {
		return toolcontract.MCPProcessObservationV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP process Observation not found")
	}
	if value.Ref != exact {
		return toolcontract.MCPProcessObservationV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "MCP process Observation exact Ref drifted")
	}
	return value, nil
}

func (j *InMemoryMCPProcessObservationJournalV1) ReadMCPProcessObservationPageV1(ctx context.Context, request toolcontract.MCPProcessObservationPageRequestV1) (toolcontract.MCPProcessObservationPageV1, error) {
	if err := officialSDKContextV1(ctx); err != nil {
		return toolcontract.MCPProcessObservationPageV1{}, err
	}
	if j == nil || request.Validate() != nil {
		return toolcontract.MCPProcessObservationPageV1{}, invalid("MCP process Observation page read is invalid")
	}
	streamKey := mcpProcessStreamKeyV1{
		Connection: mcpProcessConnectionKeyV1{ID: request.Connection.ID, Revision: request.Connection.Revision, Digest: request.Connection.Digest, Epoch: request.ConnectionEpoch},
		Snapshot:   request.Snapshot,
	}
	j.mu.RLock()
	refs := j.streams[streamKey]
	observations := make([]toolcontract.MCPProcessObservationV1, 0, min(len(refs), int(request.Limit)))
	upper := request.AfterSourceSequence
	for _, ref := range refs {
		value, ok := j.records[ref.ID]
		if !ok || value.Ref != ref {
			j.mu.RUnlock()
			return toolcontract.MCPProcessObservationPageV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP process Observation journal index drifted")
		}
		if value.SourceSequence > upper {
			upper = value.SourceSequence
		}
		if value.SourceSequence > request.AfterSourceSequence && len(observations) < int(request.Limit) {
			observations = append(observations, value)
		}
	}
	j.mu.RUnlock()
	next := request.AfterSourceSequence
	if len(observations) != 0 {
		next = observations[len(observations)-1].SourceSequence
	}
	return toolcontract.SealMCPProcessObservationPageV1(toolcontract.MCPProcessObservationPageV1{
		Request: request, Observations: observations, NextAfterSourceSequence: next,
		UpperBoundSourceSequence: upper, HasMore: next < upper,
	})
}

type OfficialSDKProcessObservationBridgeStateV1 struct {
	LastObservation *toolcontract.MCPProcessObservationRefV1
	LastError       error
}

// OfficialSDKProcessObservationBridgeV1 installs official SDK handlers. It
// records bounded observations only and never creates Evidence or ToolResult.
type OfficialSDKProcessObservationBridgeV1 struct {
	mu         sync.RWMutex
	connection toolcontract.MCPConnectionRef
	snapshot   toolcontract.ObjectRef
	sink       MCPProcessObservationSinkV1
	session    *officialmcp.ClientSession
	state      OfficialSDKProcessObservationBridgeStateV1
}

func NewOfficialSDKProcessObservationBridgeV1(connection toolcontract.MCPConnectionRef, snapshot toolcontract.ObjectRef, sink MCPProcessObservationSinkV1) (*OfficialSDKProcessObservationBridgeV1, error) {
	if connection.Validate() != nil || snapshot.Validate() != nil || nilLikeMCPProcessObservationV1(sink) {
		return nil, invalid("official MCP SDK process Observation bridge dependencies are invalid")
	}
	return &OfficialSDKProcessObservationBridgeV1{connection: connection, snapshot: snapshot, sink: sink}, nil
}

func (b *OfficialSDKProcessObservationBridgeV1) InstallClientOptionsV1(options *officialmcp.ClientOptions) error {
	if b == nil || options == nil || options.ProgressNotificationHandler != nil || options.LoggingMessageHandler != nil {
		return invalid("official MCP SDK process Observation handlers cannot be installed")
	}
	options.ProgressNotificationHandler = b.handleProgressV1
	options.LoggingMessageHandler = b.handleLoggingV1
	return nil
}

func (b *OfficialSDKProcessObservationBridgeV1) BindInitializedSessionV1(ctx context.Context, session *officialmcp.ClientSession) error {
	if err := officialSDKContextV1(ctx); err != nil {
		return err
	}
	if b == nil || session == nil || session.InitializeResult() == nil || session.InitializeResult().ProtocolVersion != b.connection.NegotiatedProtocol || session.InitializeResult().Capabilities == nil || session.ID() != "" && session.ID() != b.connection.SessionID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "official MCP SDK process Observation Session drifted")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.session != nil && b.session != session {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "official MCP SDK process Observation bridge already binds another Session")
	}
	b.session = session
	return nil
}

func (b *OfficialSDKProcessObservationBridgeV1) InspectStateV1() OfficialSDKProcessObservationBridgeStateV1 {
	if b == nil {
		return OfficialSDKProcessObservationBridgeStateV1{LastError: core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "official MCP SDK process Observation bridge is unavailable")}
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	state := b.state
	if state.LastObservation != nil {
		copy := *state.LastObservation
		state.LastObservation = &copy
	}
	return state
}

func (b *OfficialSDKProcessObservationBridgeV1) handleProgressV1(ctx context.Context, request *officialmcp.ProgressNotificationClientRequest) {
	if request == nil || request.Params == nil {
		b.recordProcessResultV1(toolcontract.MCPProcessObservationRefV1{}, invalid("official MCP SDK progress notification is incomplete"))
		return
	}
	correlation, err := mcpProcessCorrelationDigestV1(request.Params.ProgressToken, true)
	if err != nil {
		b.recordProcessResultV1(toolcontract.MCPProcessObservationRefV1{}, err)
		return
	}
	payload, err := digestOfficialSDKProcessValueV1("MCPProgressNotificationV1", request.Params)
	if err != nil {
		b.recordProcessResultV1(toolcontract.MCPProcessObservationRefV1{}, err)
		return
	}
	b.recordOfficialSDKProcessV1(ctx, request.GetSession(), toolcontract.MCPProcessObservationInputV1{Kind: toolcontract.MCPProcessProgressV1, CorrelationDigest: correlation, PayloadDigest: payload, Progress: request.Params.Progress, Total: request.Params.Total})
}

func (b *OfficialSDKProcessObservationBridgeV1) handleLoggingV1(ctx context.Context, request *officialmcp.LoggingMessageRequest) {
	if request == nil || request.Params == nil {
		b.recordProcessResultV1(toolcontract.MCPProcessObservationRefV1{}, invalid("official MCP SDK logging notification is incomplete"))
		return
	}
	correlation, err := mcpProcessCorrelationDigestV1(request.Params.GetProgressToken(), false)
	if err != nil {
		b.recordProcessResultV1(toolcontract.MCPProcessObservationRefV1{}, err)
		return
	}
	payload, err := digestOfficialSDKProcessValueV1("MCPLoggingNotificationV1", request.Params)
	if err != nil {
		b.recordProcessResultV1(toolcontract.MCPProcessObservationRefV1{}, err)
		return
	}
	b.recordOfficialSDKProcessV1(ctx, request.GetSession(), toolcontract.MCPProcessObservationInputV1{Kind: toolcontract.MCPProcessLoggingV1, CorrelationDigest: correlation, PayloadDigest: payload, LoggingLevel: string(request.Params.Level), Logger: request.Params.Logger})
}

func (b *OfficialSDKProcessObservationBridgeV1) recordOfficialSDKProcessV1(ctx context.Context, session officialmcp.Session, input toolcontract.MCPProcessObservationInputV1) {
	if b == nil {
		return
	}
	b.mu.RLock()
	expected, connection, snapshot, sink := b.session, b.connection, b.snapshot, b.sink
	b.mu.RUnlock()
	if ctx == nil || expected == nil || session != expected || nilLikeMCPProcessObservationV1(sink) {
		b.recordProcessResultV1(toolcontract.MCPProcessObservationRefV1{}, core.NewError(core.ErrorForbidden, core.ReasonBindingDrift, "official MCP SDK process notification Session is not exact"))
		return
	}
	observation, err := sink.RecordMCPProcessObservationV1(ctx, connection, snapshot, input)
	b.recordProcessResultV1(observation.Ref, err)
}

func (b *OfficialSDKProcessObservationBridgeV1) recordProcessResultV1(ref toolcontract.MCPProcessObservationRefV1, err error) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state.LastError = err
	if err == nil {
		copy := ref
		b.state.LastObservation = &copy
	}
}

func digestOfficialSDKProcessValueV1(discriminator string, value any) (core.Digest, error) {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) == 0 || len(payload) > MaxMessageBytes {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "official MCP SDK process payload exceeds its canonical limit")
	}
	var decoded any
	if err := core.DecodeStrictJSON(payload, &decoded); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.tool-mcp.mcp-process-observation", toolcontract.MCPProcessObservationContractVersionV1, discriminator, decoded)
}

func mcpProcessCorrelationDigestV1(value any, required bool) (core.Digest, error) {
	if value == nil {
		if required {
			return "", invalid("official MCP SDK progress token is required")
		}
		return core.CanonicalJSONDigest("praxis.tool-mcp.mcp-process-observation", toolcontract.MCPProcessObservationContractVersionV1, "MCPNoCorrelationV1", struct{}{})
	}
	var normalized any
	switch token := value.(type) {
	case string:
		if token == "" || len(token) > toolcontract.MaxStringBytes {
			return "", invalid("official MCP SDK progress token is invalid")
		}
		normalized = token
	case int:
		normalized = int64(token)
	case int32:
		normalized = int64(token)
	case int64:
		normalized = token
	case float64:
		if math.IsNaN(token) || math.IsInf(token, 0) || token != math.Trunc(token) {
			return "", invalid("official MCP SDK progress token is not an integer")
		}
		normalized = int64(token)
	case json.Number:
		integer, err := token.Int64()
		if err != nil {
			return "", invalid("official MCP SDK progress token is not an integer")
		}
		normalized = integer
	default:
		return "", invalid("official MCP SDK progress token type is invalid")
	}
	return core.CanonicalJSONDigest("praxis.tool-mcp.mcp-process-observation", toolcontract.MCPProcessObservationContractVersionV1, "MCPProgressCorrelationV1", normalized)
}

func nilLikeMCPProcessObservationV1(value any) bool {
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
