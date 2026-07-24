package mcp

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPListChangedObservationReaderV1 interface {
	InspectPendingMCPListChangedV1(context.Context, toolcontract.MCPConnectionRef, toolcontract.MCPListChangedNamespaceV1) (toolcontract.MCPListChangedObservationV1, error)
	InspectMCPListChangedV1(context.Context, toolcontract.MCPListChangedObservationRefV1) (toolcontract.MCPListChangedObservationV1, error)
}

type MCPListChangedObservationSinkV1 interface {
	RecordMCPListChangedV1(context.Context, toolcontract.MCPConnectionRef, toolcontract.ObjectRef, toolcontract.MCPListChangedNamespaceV1) (toolcontract.MCPListChangedObservationV1, error)
}

type InMemoryMCPListChangedJournalV1 struct {
	mu      sync.RWMutex
	clock   func() time.Time
	next    map[string]uint64
	pending map[string]toolcontract.MCPListChangedObservationRefV1
	records map[string]toolcontract.MCPListChangedObservationV1
}

func NewInMemoryMCPListChangedJournalV1(clock func() time.Time) (*InMemoryMCPListChangedJournalV1, error) {
	if clock == nil {
		return nil, invalid("MCP list-changed journal clock is missing")
	}
	return &InMemoryMCPListChangedJournalV1{clock: clock, next: make(map[string]uint64), pending: make(map[string]toolcontract.MCPListChangedObservationRefV1), records: make(map[string]toolcontract.MCPListChangedObservationV1)}, nil
}

func (j *InMemoryMCPListChangedJournalV1) RecordMCPListChangedV1(ctx context.Context, connection toolcontract.MCPConnectionRef, snapshot toolcontract.ObjectRef, namespace toolcontract.MCPListChangedNamespaceV1) (toolcontract.MCPListChangedObservationV1, error) {
	if err := officialSDKContextV1(ctx); err != nil {
		return toolcontract.MCPListChangedObservationV1{}, err
	}
	if j == nil || j.clock == nil {
		return toolcontract.MCPListChangedObservationV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP list-changed journal is unavailable")
	}
	if connection.Validate() != nil || snapshot.Validate() != nil || namespace.Validate() != nil {
		return toolcontract.MCPListChangedObservationV1{}, invalid("MCP list-changed record request is invalid")
	}
	now := j.clock()
	if now.IsZero() || now.UnixNano() < connection.CreatedUnixNano {
		return toolcontract.MCPListChangedObservationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP list-changed journal clock regressed")
	}
	if !now.Before(time.Unix(0, connection.ExpiresUnixNano)) {
		return toolcontract.MCPListChangedObservationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP list-changed Connection expired")
	}
	key := listChangedPendingKeyV1(connection, namespace)
	j.mu.Lock()
	defer j.mu.Unlock()
	if ref, ok := j.pending[key]; ok {
		existing := j.records[ref.ID]
		if existing.Connection != connection || existing.Snapshot != snapshot || existing.Namespace != namespace {
			return toolcontract.MCPListChangedObservationV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "pending MCP list-changed Observation binds another current Snapshot")
		}
		return existing, nil
	}
	sequence := j.next[connection.ID] + 1
	observation, err := toolcontract.SealMCPListChangedObservationV1(toolcontract.MCPListChangedObservationV1{Connection: connection, Snapshot: snapshot, Namespace: namespace, SourceSequence: sequence, ObservedUnixNano: now.UnixNano()})
	if err != nil {
		return toolcontract.MCPListChangedObservationV1{}, err
	}
	j.next[connection.ID] = sequence
	j.records[observation.Ref.ID] = observation
	j.pending[key] = observation.Ref
	return observation, nil
}

func (j *InMemoryMCPListChangedJournalV1) InspectPendingMCPListChangedV1(ctx context.Context, connection toolcontract.MCPConnectionRef, namespace toolcontract.MCPListChangedNamespaceV1) (toolcontract.MCPListChangedObservationV1, error) {
	if err := officialSDKContextV1(ctx); err != nil {
		return toolcontract.MCPListChangedObservationV1{}, err
	}
	if j == nil || connection.Validate() != nil || namespace.Validate() != nil {
		return toolcontract.MCPListChangedObservationV1{}, invalid("MCP list-changed pending Inspect is invalid")
	}
	key := listChangedPendingKeyV1(connection, namespace)
	j.mu.RLock()
	ref, ok := j.pending[key]
	value := j.records[ref.ID]
	j.mu.RUnlock()
	if !ok {
		return toolcontract.MCPListChangedObservationV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "pending MCP list-changed Observation not found")
	}
	if value.Connection != connection || value.Namespace != namespace {
		return toolcontract.MCPListChangedObservationV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "pending MCP list-changed Observation drifted")
	}
	return value, nil
}

func (j *InMemoryMCPListChangedJournalV1) InspectMCPListChangedV1(ctx context.Context, ref toolcontract.MCPListChangedObservationRefV1) (toolcontract.MCPListChangedObservationV1, error) {
	if err := officialSDKContextV1(ctx); err != nil {
		return toolcontract.MCPListChangedObservationV1{}, err
	}
	if j == nil || ref.Validate() != nil {
		return toolcontract.MCPListChangedObservationV1{}, invalid("MCP list-changed exact Inspect is invalid")
	}
	j.mu.RLock()
	value, ok := j.records[ref.ID]
	j.mu.RUnlock()
	if !ok {
		return toolcontract.MCPListChangedObservationV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP list-changed Observation not found")
	}
	if value.Ref != ref {
		return toolcontract.MCPListChangedObservationV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "MCP list-changed Observation exact Ref drifted")
	}
	return value, nil
}

// AcknowledgeMCPListChangedV1 only releases the coalescing index after a new
// exact Snapshot exists. It does not create or mutate that Snapshot.
func (j *InMemoryMCPListChangedJournalV1) AcknowledgeMCPListChangedV1(ctx context.Context, exact toolcontract.MCPListChangedObservationRefV1, replacement toolcontract.ObjectRef) error {
	if err := officialSDKContextV1(ctx); err != nil {
		return err
	}
	if j == nil || exact.Validate() != nil || replacement.Validate() != nil {
		return invalid("MCP list-changed acknowledgement is invalid")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	observation, ok := j.records[exact.ID]
	if !ok {
		return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP list-changed Observation not found")
	}
	if observation.Ref != exact || replacement.ID != observation.Snapshot.ID || replacement.Revision <= observation.Snapshot.Revision || replacement.Digest == observation.Snapshot.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP list-changed acknowledgement lacks a successor Snapshot")
	}
	key := listChangedPendingKeyV1(observation.Connection, observation.Namespace)
	if current, ok := j.pending[key]; !ok || current != exact {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP list-changed pending index changed")
	}
	delete(j.pending, key)
	return nil
}

type OfficialSDKListChangedBridgeStateV1 struct {
	LastObservation *toolcontract.MCPListChangedObservationRefV1
	LastError       error
}

// OfficialSDKListChangedBridgeV1 installs official SDK notification handlers.
// It never calls Discovery, changes a Snapshot, or contacts a Provider.
type OfficialSDKListChangedBridgeV1 struct {
	mu         sync.RWMutex
	connection toolcontract.MCPConnectionRef
	snapshot   toolcontract.ObjectRef
	sink       MCPListChangedObservationSinkV1
	session    *officialmcp.ClientSession
	state      OfficialSDKListChangedBridgeStateV1
}

func NewOfficialSDKListChangedBridgeV1(connection toolcontract.MCPConnectionRef, snapshot toolcontract.ObjectRef, sink MCPListChangedObservationSinkV1) (*OfficialSDKListChangedBridgeV1, error) {
	if connection.Validate() != nil || snapshot.Validate() != nil || nilLikeListChangedV1(sink) {
		return nil, invalid("official MCP SDK list-changed bridge dependencies are invalid")
	}
	return &OfficialSDKListChangedBridgeV1{connection: connection, snapshot: snapshot, sink: sink}, nil
}

func (b *OfficialSDKListChangedBridgeV1) InstallClientOptionsV1(options *officialmcp.ClientOptions) error {
	if b == nil || options == nil || options.ToolListChangedHandler != nil || options.ResourceListChangedHandler != nil || options.PromptListChangedHandler != nil {
		return invalid("official MCP SDK list-changed handlers cannot be installed")
	}
	options.ToolListChangedHandler = func(ctx context.Context, request *officialmcp.ToolListChangedRequest) {
		b.handleV1(ctx, request, toolcontract.MCPListChangedToolsV1)
	}
	options.ResourceListChangedHandler = func(ctx context.Context, request *officialmcp.ResourceListChangedRequest) {
		b.handleV1(ctx, request, toolcontract.MCPListChangedResourcesV1)
	}
	options.PromptListChangedHandler = func(ctx context.Context, request *officialmcp.PromptListChangedRequest) {
		b.handleV1(ctx, request, toolcontract.MCPListChangedPromptsV1)
	}
	return nil
}

func (b *OfficialSDKListChangedBridgeV1) BindInitializedSessionV1(ctx context.Context, session *officialmcp.ClientSession) error {
	if err := officialSDKContextV1(ctx); err != nil {
		return err
	}
	if b == nil || session == nil || session.InitializeResult() == nil || session.InitializeResult().ProtocolVersion != b.connection.NegotiatedProtocol || session.InitializeResult().Capabilities == nil || session.ID() != "" && session.ID() != b.connection.SessionID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "official MCP SDK list-changed Session drifted")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.session != nil && b.session != session {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "official MCP SDK list-changed bridge already binds another Session")
	}
	b.session = session
	return nil
}

func (b *OfficialSDKListChangedBridgeV1) InspectStateV1() OfficialSDKListChangedBridgeStateV1 {
	if b == nil {
		return OfficialSDKListChangedBridgeStateV1{LastError: core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "official MCP SDK list-changed bridge is unavailable")}
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

func (b *OfficialSDKListChangedBridgeV1) handleV1(ctx context.Context, request interface{ GetSession() officialmcp.Session }, namespace toolcontract.MCPListChangedNamespaceV1) {
	if b == nil {
		return
	}
	b.mu.RLock()
	session, connection, snapshot, sink := b.session, b.connection, b.snapshot, b.sink
	b.mu.RUnlock()
	if ctx == nil || request == nil || session == nil || request.GetSession() != session || nilLikeListChangedV1(sink) {
		b.recordHandlerResultV1(toolcontract.MCPListChangedObservationRefV1{}, core.NewError(core.ErrorForbidden, core.ReasonBindingDrift, "official MCP SDK list-changed notification Session is not exact"))
		return
	}
	observation, err := sink.RecordMCPListChangedV1(ctx, connection, snapshot, namespace)
	b.recordHandlerResultV1(observation.Ref, err)
}

func (b *OfficialSDKListChangedBridgeV1) recordHandlerResultV1(ref toolcontract.MCPListChangedObservationRefV1, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state.LastError = err
	if err == nil {
		copy := ref
		b.state.LastObservation = &copy
	}
}

func listChangedPendingKeyV1(connection toolcontract.MCPConnectionRef, namespace toolcontract.MCPListChangedNamespaceV1) string {
	return connection.ID + "\x00" + string(namespace)
}

func nilLikeListChangedV1(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
