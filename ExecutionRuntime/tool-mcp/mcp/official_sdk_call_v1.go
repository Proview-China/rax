package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const OfficialSDKCallContractVersionV1 = "praxis.tool-mcp.official-sdk-call/v1"

type OfficialSDKCallSessionV1 interface {
	InitializeResult() *officialmcp.InitializeResult
	ID() string
	CallTool(context.Context, *officialmcp.CallToolParams) (*officialmcp.CallToolResult, error)
}

type OfficialSDKCallSessionBindingV1 struct {
	Connection        toolcontract.MCPConnectionRef        `json:"connection"`
	Snapshot          toolcontract.MCPCapabilitySnapshotV2 `json:"snapshot"`
	ProviderTransport runtimeports.ProviderBindingRefV2    `json:"provider_transport"`
	Provider          runtimeports.ProviderBindingRefV2    `json:"provider"`
	CheckedUnixNano   int64                                `json:"checked_unix_nano"`
	ExpiresUnixNano   int64                                `json:"expires_unix_nano"`
	BindingDigest     core.Digest                          `json:"binding_digest"`
	Session           OfficialSDKCallSessionV1             `json:"-"`
}

func (b OfficialSDKCallSessionBindingV1) ValidateCurrent(connection toolcontract.MCPConnectionRef, snapshot toolcontract.ObjectRef, transport, provider runtimeports.ProviderBindingRefV2, now time.Time) error {
	if b.Connection.Validate() != nil || b.Snapshot.Validate() != nil || b.ProviderTransport.Validate() != nil || b.Provider.Validate() != nil || b.ProviderTransport == b.Provider || b.CheckedUnixNano <= 0 || b.ExpiresUnixNano <= b.CheckedUnixNano || b.BindingDigest.Validate() != nil || nilLikeOfficialSDKV1(b.Session) {
		return invalid("official MCP SDK call Session binding is invalid")
	}
	if b.Connection != connection || b.Snapshot.ID != snapshot.ID || b.Snapshot.Revision != snapshot.Revision || b.Snapshot.Digest != snapshot.Digest || b.Snapshot.Connection != (toolcontract.ObjectRef{ID: b.Connection.ID, Revision: b.Connection.Revision, Digest: b.Connection.Digest}) || b.Snapshot.ConnectionEpoch != b.Connection.Epoch || b.ProviderTransport != transport || b.Provider != provider {
		return mcpCallConflictV1("official MCP SDK call Session exact binding drifted")
	}
	digest, err := b.ComputeDigest()
	if err != nil || digest != b.BindingDigest {
		return mcpCallConflictV1("official MCP SDK call Session binding digest drifted")
	}
	if now.IsZero() || now.UnixNano() < b.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "official MCP SDK call Session clock regressed")
	}
	if !now.Before(time.Unix(0, b.ExpiresUnixNano)) || !now.Before(time.Unix(0, b.Connection.ExpiresUnixNano)) || !now.Before(time.Unix(0, b.Snapshot.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "official MCP SDK call Session binding expired")
	}
	initialize := b.Session.InitializeResult()
	if initialize == nil || initialize.Capabilities == nil || initialize.ProtocolVersion != b.Connection.NegotiatedProtocol || initialize.ProtocolVersion != toolcontract.MCPStableProtocolVersion {
		return mcpCallConflictV1("official MCP SDK call Session initialize result drifted")
	}
	if sessionID := b.Session.ID(); sessionID != "" && sessionID != b.Connection.SessionID {
		return mcpCallConflictV1("official MCP SDK call Session ID drifted")
	}
	return nil
}

func (b OfficialSDKCallSessionBindingV1) ComputeDigest() (core.Digest, error) {
	b.Snapshot = toolcontract.CloneMCPCapabilitySnapshotV2(b.Snapshot)
	b.BindingDigest = ""
	b.Session = nil
	return core.CanonicalJSONDigest("praxis.tool-mcp.official-sdk-call", OfficialSDKCallContractVersionV1, "OfficialSDKCallSessionBindingV1", b)
}

func SealOfficialSDKCallSessionBindingV1(b OfficialSDKCallSessionBindingV1, now time.Time) (OfficialSDKCallSessionBindingV1, error) {
	b.Snapshot = toolcontract.CloneMCPCapabilitySnapshotV2(b.Snapshot)
	provided := b.BindingDigest
	b.BindingDigest = ""
	digest, err := b.ComputeDigest()
	if err != nil {
		return OfficialSDKCallSessionBindingV1{}, err
	}
	if provided != "" && provided != digest {
		return OfficialSDKCallSessionBindingV1{}, mcpCallConflictV1("supplied official MCP SDK call Session binding digest drifted")
	}
	b.BindingDigest = digest
	snapshot := toolcontract.ObjectRef{ID: b.Snapshot.ID, Revision: b.Snapshot.Revision, Digest: b.Snapshot.Digest}
	return b, b.ValidateCurrent(b.Connection, snapshot, b.ProviderTransport, b.Provider, now)
}

type OfficialSDKCallSessionCurrentReaderV1 interface {
	InspectCurrentOfficialSDKCallSessionV1(context.Context, toolcontract.MCPConnectionRef, toolcontract.ObjectRef, runtimeports.ProviderBindingRefV2, runtimeports.ProviderBindingRefV2) (OfficialSDKCallSessionBindingV1, error)
}

// InMemoryOfficialSDKCallSessionRepositoryV1 is a production-neutral exact
// Session repository. It accepts an already initialized official SDK Session
// only together with the full Connection/Snapshot/Transport/Provider binding.
type InMemoryOfficialSDKCallSessionRepositoryV1 struct {
	mu       sync.RWMutex
	bindings map[string]OfficialSDKCallSessionBindingV1
	clock    func() time.Time
}

func NewInMemoryOfficialSDKCallSessionRepositoryV1(clock func() time.Time) (*InMemoryOfficialSDKCallSessionRepositoryV1, error) {
	if clock == nil {
		return nil, invalid("official MCP SDK call Session repository clock is missing")
	}
	return &InMemoryOfficialSDKCallSessionRepositoryV1{bindings: make(map[string]OfficialSDKCallSessionBindingV1), clock: clock}, nil
}

func (r *InMemoryOfficialSDKCallSessionRepositoryV1) BindInitializedOfficialSDKSessionV1(ctx context.Context, binding OfficialSDKCallSessionBindingV1) (OfficialSDKCallSessionBindingV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return OfficialSDKCallSessionBindingV1{}, err
	}
	if r == nil || r.clock == nil {
		return OfficialSDKCallSessionBindingV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "official MCP SDK call Session repository is unavailable")
	}
	now := r.clock()
	sealed, err := SealOfficialSDKCallSessionBindingV1(binding, now)
	if err != nil {
		return OfficialSDKCallSessionBindingV1{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.bindings[sealed.Connection.ID]; ok {
		if existing.BindingDigest != sealed.BindingDigest || !sameOfficialSDKSessionV1(existing.Session, sealed.Session) {
			return OfficialSDKCallSessionBindingV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "official MCP SDK call Session ID already binds another exact Session")
		}
		return cloneOfficialSDKCallSessionBindingV1(existing), nil
	}
	r.bindings[sealed.Connection.ID] = cloneOfficialSDKCallSessionBindingV1(sealed)
	return cloneOfficialSDKCallSessionBindingV1(sealed), nil
}

func (r *InMemoryOfficialSDKCallSessionRepositoryV1) InspectCurrentOfficialSDKCallSessionV1(ctx context.Context, connection toolcontract.MCPConnectionRef, snapshot toolcontract.ObjectRef, transport, provider runtimeports.ProviderBindingRefV2) (OfficialSDKCallSessionBindingV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return OfficialSDKCallSessionBindingV1{}, err
	}
	if r == nil || r.clock == nil {
		return OfficialSDKCallSessionBindingV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "official MCP SDK call Session repository is unavailable")
	}
	r.mu.RLock()
	binding, ok := r.bindings[connection.ID]
	r.mu.RUnlock()
	if !ok {
		return OfficialSDKCallSessionBindingV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "official MCP SDK call Session not found")
	}
	now := r.clock()
	if err := binding.ValidateCurrent(connection, snapshot, transport, provider, now); err != nil {
		return OfficialSDKCallSessionBindingV1{}, err
	}
	return cloneOfficialSDKCallSessionBindingV1(binding), nil
}

type MCPPhysicalExecutionStateV1 string

const (
	MCPPhysicalExecutionAdmittedV1 MCPPhysicalExecutionStateV1 = "admitted"
	MCPPhysicalExecutionObservedV1 MCPPhysicalExecutionStateV1 = "observed"
	MCPPhysicalExecutionUnknownV1  MCPPhysicalExecutionStateV1 = "unknown"
)

type MCPPhysicalExecutionEntryV1 struct {
	ID                  string                                                        `json:"id"`
	Revision            core.Revision                                                 `json:"revision"`
	StableKeyDigest     core.Digest                                                   `json:"stable_key_digest"`
	AuthorizationDigest core.Digest                                                   `json:"authorization_digest"`
	Command             toolcontract.MCPExecutionCommandRefV1                         `json:"command"`
	AdmissionReceipt    runtimeports.ControlledOperationProviderAdmissionReceiptRefV2 `json:"admission_receipt"`
	State               MCPPhysicalExecutionStateV1                                   `json:"state"`
	ProtocolReceipt     *toolcontract.MCPProtocolReceiptV1                            `json:"protocol_receipt,omitempty"`
	UnknownReasonDigest core.Digest                                                   `json:"unknown_reason_digest,omitempty"`
	UpdatedUnixNano     int64                                                         `json:"updated_unix_nano"`
}

type InMemoryMCPPhysicalExecutionStoreV1 struct {
	mu      sync.RWMutex
	entries map[string]MCPPhysicalExecutionEntryV1
}

func NewInMemoryMCPPhysicalExecutionStoreV1() *InMemoryMCPPhysicalExecutionStoreV1 {
	return &InMemoryMCPPhysicalExecutionStoreV1{entries: make(map[string]MCPPhysicalExecutionEntryV1)}
}

func (s *InMemoryMCPPhysicalExecutionStoreV1) beginV1(ctx context.Context, authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3, command toolcontract.MCPExecutionCommandRefV1, now time.Time) (MCPPhysicalExecutionEntryV1, bool, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return MCPPhysicalExecutionEntryV1{}, false, err
	}
	if s == nil {
		return MCPPhysicalExecutionEntryV1{}, false, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP physical execution store is unavailable")
	}
	if err := authorization.ValidateCurrent(now); err != nil || command.Validate() != nil {
		if err != nil {
			return MCPPhysicalExecutionEntryV1{}, false, err
		}
		return MCPPhysicalExecutionEntryV1{}, false, invalid("MCP physical execution command Ref is invalid")
	}
	id := "mcp-physical-entry-" + strings.TrimPrefix(string(authorization.StableKeyDigest), "sha256:")
	receipt, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{ID: "mcp-admission-" + strings.TrimPrefix(string(authorization.StableKeyDigest), "sha256:"), Revision: 1, StableKeyDigest: authorization.StableKeyDigest, Admitted: true})
	if err != nil {
		return MCPPhysicalExecutionEntryV1{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.entries[id]; ok {
		if existing.StableKeyDigest != authorization.StableKeyDigest || existing.AuthorizationDigest != authorization.AuthorizationDigest || existing.Command != command {
			return MCPPhysicalExecutionEntryV1{}, false, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP physical execution stable key binds another authorization or command")
		}
		return cloneMCPPhysicalExecutionEntryV1(existing), false, nil
	}
	entry := MCPPhysicalExecutionEntryV1{ID: id, Revision: 1, StableKeyDigest: authorization.StableKeyDigest, AuthorizationDigest: authorization.AuthorizationDigest, Command: command, AdmissionReceipt: receipt, State: MCPPhysicalExecutionAdmittedV1, UpdatedUnixNano: now.UnixNano()}
	s.entries[id] = entry
	return cloneMCPPhysicalExecutionEntryV1(entry), true, nil
}

func (s *InMemoryMCPPhysicalExecutionStoreV1) completeV1(ctx context.Context, stable core.Digest, receipt toolcontract.MCPProtocolReceiptV1, now time.Time) (MCPPhysicalExecutionEntryV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return MCPPhysicalExecutionEntryV1{}, err
	}
	if s == nil || receipt.Validate() != nil || now.IsZero() {
		return MCPPhysicalExecutionEntryV1{}, invalid("MCP physical execution completion is invalid")
	}
	id := "mcp-physical-entry-" + strings.TrimPrefix(string(stable), "sha256:")
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[id]
	if !ok {
		return MCPPhysicalExecutionEntryV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP physical execution Entry not found")
	}
	if receipt.StableKeyDigest != stable || receipt.Command != entry.Command || receipt.AdmissionReceipt != entry.AdmissionReceipt {
		return MCPPhysicalExecutionEntryV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Protocol Receipt belongs to another physical Entry")
	}
	if entry.State == MCPPhysicalExecutionObservedV1 {
		if entry.ProtocolReceipt == nil || entry.ProtocolReceipt.Ref != receipt.Ref {
			return MCPPhysicalExecutionEntryV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP physical Entry already observed another response")
		}
		return cloneMCPPhysicalExecutionEntryV1(entry), nil
	}
	if entry.State != MCPPhysicalExecutionAdmittedV1 {
		return MCPPhysicalExecutionEntryV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "unknown MCP physical Entry cannot be upgraded without Inspect")
	}
	receipt = toolcontract.CloneMCPProtocolReceiptV1(receipt)
	entry.Revision++
	entry.State, entry.ProtocolReceipt, entry.UpdatedUnixNano = MCPPhysicalExecutionObservedV1, &receipt, now.UnixNano()
	s.entries[id] = entry
	return cloneMCPPhysicalExecutionEntryV1(entry), nil
}

func (s *InMemoryMCPPhysicalExecutionStoreV1) markUnknownV1(stable core.Digest, reason core.Digest, now time.Time) {
	if s == nil || stable.Validate() != nil || reason.Validate() != nil || now.IsZero() {
		return
	}
	id := "mcp-physical-entry-" + strings.TrimPrefix(string(stable), "sha256:")
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[id]
	if !ok || entry.State != MCPPhysicalExecutionAdmittedV1 {
		return
	}
	entry.Revision++
	entry.State, entry.UnknownReasonDigest, entry.UpdatedUnixNano = MCPPhysicalExecutionUnknownV1, reason, now.UnixNano()
	s.entries[id] = entry
}

func (s *InMemoryMCPPhysicalExecutionStoreV1) InspectMCPPhysicalExecutionV1(ctx context.Context, stable core.Digest) (MCPPhysicalExecutionEntryV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return MCPPhysicalExecutionEntryV1{}, err
	}
	if s == nil || stable.Validate() != nil {
		return MCPPhysicalExecutionEntryV1{}, invalid("MCP physical execution Inspect key is invalid")
	}
	id := "mcp-physical-entry-" + strings.TrimPrefix(string(stable), "sha256:")
	s.mu.RLock()
	entry, ok := s.entries[id]
	s.mu.RUnlock()
	if !ok {
		return MCPPhysicalExecutionEntryV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP physical execution Entry not found")
	}
	return cloneMCPPhysicalExecutionEntryV1(entry), nil
}

func (s *InMemoryMCPPhysicalExecutionStoreV1) InspectMCPProtocolReceiptV1(ctx context.Context, exact toolcontract.MCPProtocolReceiptRefV1) (toolcontract.MCPProtocolReceiptV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPProtocolReceiptV1{}, err
	}
	if s == nil || exact.Validate() != nil {
		return toolcontract.MCPProtocolReceiptV1{}, invalid("MCP Protocol Receipt exact Inspect key is invalid")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, entry := range s.entries {
		if entry.ProtocolReceipt == nil || entry.ProtocolReceipt.Ref.ID != exact.ID {
			continue
		}
		if entry.ProtocolReceipt.Ref != exact {
			return toolcontract.MCPProtocolReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Protocol Receipt exact Ref drifted")
		}
		return toolcontract.CloneMCPProtocolReceiptV1(*entry.ProtocolReceipt), nil
	}
	return toolcontract.MCPProtocolReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Protocol Receipt not found")
}

func (s *InMemoryMCPPhysicalExecutionStoreV1) InspectMCPProtocolReceiptByIDV1(ctx context.Context, id string) (toolcontract.MCPProtocolReceiptV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPProtocolReceiptV1{}, err
	}
	if s == nil || toolcontract.ValidateStableID(id) != nil {
		return toolcontract.MCPProtocolReceiptV1{}, invalid("MCP Protocol Receipt ID lookup is invalid")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var found *toolcontract.MCPProtocolReceiptV1
	for _, entry := range s.entries {
		if entry.ProtocolReceipt == nil || entry.ProtocolReceipt.Ref.ID != id {
			continue
		}
		if found != nil && found.Ref != entry.ProtocolReceipt.Ref {
			return toolcontract.MCPProtocolReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Protocol Receipt ID is not unique")
		}
		copy := toolcontract.CloneMCPProtocolReceiptV1(*entry.ProtocolReceipt)
		found = &copy
	}
	if found == nil {
		return toolcontract.MCPProtocolReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Protocol Receipt not found")
	}
	return toolcontract.CloneMCPProtocolReceiptV1(*found), nil
}

var _ toolcontract.MCPProtocolReceiptIDReaderV1 = (*InMemoryMCPPhysicalExecutionStoreV1)(nil)

type OfficialSDKPhysicalExecutorV1 struct {
	commands     toolcontract.MCPExecutionCommandCurrentReaderV1
	associations runtimeports.PreparedDomainCommandAssociationCurrentReaderV1
	sessions     OfficialSDKCallSessionCurrentReaderV1
	entries      *InMemoryMCPPhysicalExecutionStoreV1
	clock        func() time.Time
}

func NewOfficialSDKPhysicalExecutorV1(commands toolcontract.MCPExecutionCommandCurrentReaderV1, associations runtimeports.PreparedDomainCommandAssociationCurrentReaderV1, sessions OfficialSDKCallSessionCurrentReaderV1, entries *InMemoryMCPPhysicalExecutionStoreV1, clock func() time.Time) (*OfficialSDKPhysicalExecutorV1, error) {
	if nilLikeCallDependencyV1(commands) || nilLikeCallDependencyV1(associations) || nilLikeCallDependencyV1(sessions) || entries == nil || clock == nil {
		return nil, invalid("official MCP SDK physical executor dependencies are incomplete")
	}
	return &OfficialSDKPhysicalExecutorV1{commands: commands, associations: associations, sessions: sessions, entries: entries, clock: clock}, nil
}

func (e *OfficialSDKPhysicalExecutorV1) ExecuteControlledOperationPhysicalV3(ctx context.Context, authorization runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3) (runtimeports.ControlledOperationProviderAdmissionReceiptRefV2, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	if e == nil || nilLikeCallDependencyV1(e.commands) || nilLikeCallDependencyV1(e.associations) || nilLikeCallDependencyV1(e.sessions) || e.entries == nil || e.clock == nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "official MCP SDK physical executor is unavailable")
	}
	if err := authorization.Validate(); err != nil || authorization.DomainCommand.Kind != toolcontract.MCPExecutionCommandKindV1 {
		if err != nil {
			return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
		}
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "physical executor only accepts exact MCP execution commands")
	}
	previous, err := e.freshV1(time.Time{})
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	association, err := e.associations.InspectCurrentPreparedDomainCommandAssociationV1(ctx, authorization.Association)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	previous, err = e.freshV1(previous)
	if err != nil || association.ValidateCurrent(authorization.Association, previous) != nil {
		if err != nil {
			return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
		}
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, association.ValidateCurrent(authorization.Association, previous)
	}
	if err = validateAssociationAgainstAuthorizationV1(association, authorization); err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	commandRef := toolcontract.MCPExecutionCommandRefV1{ID: authorization.DomainCommand.ID, Revision: authorization.DomainCommand.Revision, Digest: authorization.DomainCommand.Digest}
	projection, err := e.commands.InspectCurrentMCPExecutionCommandV1(ctx, commandRef)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	previous, err = e.freshV1(previous)
	if err != nil || projection.ValidateCurrent(commandRef, previous) != nil {
		if err != nil {
			return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
		}
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, projection.ValidateCurrent(commandRef, previous)
	}
	command := projection.Fact
	if err = validateCommandAgainstAuthorizationV1(command, association, authorization); err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	snapshotRef := toolcontract.ObjectRef{ID: command.Snapshot.ID, Revision: command.Snapshot.Revision, Digest: command.Snapshot.Digest}
	session, err := e.sessions.InspectCurrentOfficialSDKCallSessionV1(ctx, command.Connection, snapshotRef, authorization.ProviderTransport, authorization.Provider)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	previous, err = e.freshV1(previous)
	if err != nil || session.ValidateCurrent(command.Connection, snapshotRef, authorization.ProviderTransport, authorization.Provider, previous) != nil {
		if err != nil {
			return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
		}
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, session.ValidateCurrent(command.Connection, snapshotRef, authorization.ProviderTransport, authorization.Provider, previous)
	}
	arguments, err := decodeMCPCallArgumentsV1(command.Params.Inline)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	actual, err := e.freshV1(previous)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	if err = validateActualPointV1(actual, authorization, association, projection, session, command); err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	entry, created, err := e.entries.beginV1(ctx, authorization, command.Ref, actual)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	if !created {
		return entry.AdmissionReceipt, nil
	}
	result, callErr := session.Session.CallTool(ctx, &officialmcp.CallToolParams{Name: command.SnapshotTool.Name, Arguments: arguments})
	observed, clockErr := e.freshV1(actual)
	if callErr != nil || clockErr != nil || result == nil {
		reason := "nil-result"
		if callErr != nil {
			reason = callErr.Error()
		} else if clockErr != nil {
			reason = clockErr.Error()
		}
		e.entries.markUnknownV1(authorization.StableKeyDigest, core.DigestBytes([]byte(reason)), nonZeroExecutionTimeV1(observed, actual))
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP tools/call outcome requires exact Inspect")
	}
	response, err := canonicalizeOfficialSDKCallResultV1(result, command.Tool.ResultLimitBytes)
	if err != nil {
		e.entries.markUnknownV1(authorization.StableKeyDigest, core.DigestBytes([]byte("response-canonicalization-failed")), observed)
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP tools/call response could not be recorded; exact Inspect is required")
	}
	receipt, err := toolcontract.SealMCPProtocolReceiptV1(toolcontract.MCPProtocolReceiptV1{Command: command.Ref, StableKeyDigest: authorization.StableKeyDigest, AdmissionReceipt: entry.AdmissionReceipt, JSONRPCRequestID: command.JSONRPCRequestID, ToolError: result.IsError, CanonicalResponse: response, ObservedUnixNano: observed.UnixNano()})
	if err != nil {
		e.entries.markUnknownV1(authorization.StableKeyDigest, core.DigestBytes([]byte("receipt-seal-failed")), observed)
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Protocol Receipt could not be sealed; exact Inspect is required")
	}
	if _, err = e.entries.completeV1(ctx, authorization.StableKeyDigest, receipt, observed); err != nil {
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Protocol Receipt persistence requires exact Inspect")
	}
	return entry.AdmissionReceipt, nil
}

func (e *OfficialSDKPhysicalExecutorV1) InspectMCPPhysicalExecutionV1(ctx context.Context, stable core.Digest) (MCPPhysicalExecutionEntryV1, error) {
	if e == nil || e.entries == nil {
		return MCPPhysicalExecutionEntryV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "official MCP SDK physical execution Inspect is unavailable")
	}
	return e.entries.InspectMCPPhysicalExecutionV1(ctx, stable)
}

func (e *OfficialSDKPhysicalExecutorV1) freshV1(previous time.Time) (time.Time, error) {
	now := e.clock()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "official MCP SDK physical execution clock regressed")
	}
	return now, nil
}

func validateAssociationAgainstAuthorizationV1(p runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1, a runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3) error {
	if p.Ref != a.Association || p.DomainCommand != a.DomainCommand || p.Operation != a.Operation || p.OperationDigest != a.OperationDigest || p.Prepared != a.Prepared || p.Attempt != a.Attempt || p.Provider != a.Provider {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Prepared-domain-command Association differs from physical authorization")
	}
	return nil
}

func validateCommandAgainstAuthorizationV1(c toolcontract.MCPExecutionCommandFactV1, p runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1, a runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3) error {
	if c.RuntimeDomainCommandRefV1() != a.DomainCommand || c.Operation != a.Operation || c.OperationDigest != a.OperationDigest || c.Prepared != a.Prepared || c.Attempt != a.Attempt || c.Provider != a.Provider || c.Params.Schema != p.PayloadSchema || c.Params.ContentDigest != p.PayloadDigest || c.ParamsRevision != p.PayloadRevision || c.NotAfterUnixNano > a.UnifiedNotAfterUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP execution command differs from Runtime authorization or Association")
	}
	return nil
}

func validateActualPointV1(now time.Time, a runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3, p runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1, command toolcontract.MCPExecutionCommandCurrentProjectionV1, session OfficialSDKCallSessionBindingV1, fact toolcontract.MCPExecutionCommandFactV1) error {
	if err := a.ValidateCurrent(now); err != nil {
		return err
	}
	if err := p.ValidateCurrent(a.Association, now); err != nil {
		return err
	}
	if err := command.ValidateCurrent(fact.Ref, now); err != nil {
		return err
	}
	snapshot := toolcontract.ObjectRef{ID: fact.Snapshot.ID, Revision: fact.Snapshot.Revision, Digest: fact.Snapshot.Digest}
	return session.ValidateCurrent(fact.Connection, snapshot, a.ProviderTransport, a.Provider, now)
}

func decodeMCPCallArgumentsV1(value []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var arguments map[string]any
	if err := decoder.Decode(&arguments); err != nil || arguments == nil {
		return nil, invalid("MCP tools/call arguments are not one canonical JSON object")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, invalid("MCP tools/call arguments contain trailing JSON")
	}
	return arguments, nil
}

func cloneOfficialSDKCallSessionBindingV1(b OfficialSDKCallSessionBindingV1) OfficialSDKCallSessionBindingV1 {
	b.Snapshot = toolcontract.CloneMCPCapabilitySnapshotV2(b.Snapshot)
	return b
}

func cloneMCPPhysicalExecutionEntryV1(e MCPPhysicalExecutionEntryV1) MCPPhysicalExecutionEntryV1 {
	if e.ProtocolReceipt != nil {
		copy := toolcontract.CloneMCPProtocolReceiptV1(*e.ProtocolReceipt)
		e.ProtocolReceipt = &copy
	}
	return e
}

func sameOfficialSDKSessionV1(left, right OfficialSDKCallSessionV1) bool {
	if nilLikeOfficialSDKV1(left) || nilLikeOfficialSDKV1(right) {
		return false
	}
	l, r := reflect.ValueOf(left), reflect.ValueOf(right)
	return l.Type() == r.Type() && l.Kind() == reflect.Pointer && l.Pointer() == r.Pointer()
}

func nilLikeCallDependencyV1(value any) bool {
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

func nonZeroExecutionTimeV1(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value
}

func mcpCallConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}
