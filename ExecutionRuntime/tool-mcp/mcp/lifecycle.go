package mcp

import (
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type ConnectionState string

const (
	ConnectionRegistered   ConnectionState = "registered"
	ConnectionResolving    ConnectionState = "resolving"
	ConnectionConnecting   ConnectionState = "connecting"
	ConnectionInitializing ConnectionState = "initializing"
	ConnectionDiscovering  ConnectionState = "discovering"
	ConnectionBound        ConnectionState = "bound"
	ConnectionDegraded     ConnectionState = "degraded"
	ConnectionDraining     ConnectionState = "draining"
	ConnectionClosed       ConnectionState = "closed"
	ConnectionUnknown      ConnectionState = "outcome_unknown"
)

type SnapshotState string

const (
	SnapshotObserved   SnapshotState = "observed"
	SnapshotValidated  SnapshotState = "validated"
	SnapshotAdmitted   SnapshotState = "admitted"
	SnapshotActive     SnapshotState = "active"
	SnapshotSuperseded SnapshotState = "superseded"
	SnapshotRevoked    SnapshotState = "revoked"
	SnapshotExpired    SnapshotState = "expired"
)

type ConnectionRecord struct {
	Connection      contract.MCPConnectionRef       `json:"connection"`
	State           ConnectionState                 `json:"state"`
	Revision        core.Revision                   `json:"revision"`
	Snapshot        *contract.MCPCapabilitySnapshot `json:"snapshot,omitempty"`
	SnapshotState   SnapshotState                   `json:"snapshot_state,omitempty"`
	UpdatedUnixNano int64                           `json:"updated_unix_nano"`
}

type Manager struct {
	mu       sync.RWMutex
	records  map[string]ConnectionRecord
	sessions map[string]string
}

func NewManager() *Manager {
	return &Manager{records: make(map[string]ConnectionRecord), sessions: make(map[string]string)}
}

func sessionKey(ref contract.MCPConnectionRef) string {
	return ref.TenantID + "\x00" + ref.IdentityID + "\x00" + string(ref.PlanDigest) + "\x00" + ref.InstanceID + "\x00" + ref.RunID + "\x00" + ref.SessionID + "\x00" + ref.Server.ID + "\x00" + string(ref.Server.Digest)
}

func (m *Manager) Register(ref contract.MCPConnectionRef, now time.Time) (ConnectionRecord, error) {
	if err := ref.Validate(); err != nil {
		return ConnectionRecord{}, err
	}
	if now.IsZero() {
		return ConnectionRecord{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "MCP lifecycle time is required")
	}
	currentUnixNano := now.UTC().UnixNano()
	if currentUnixNano < ref.CreatedUnixNano {
		return ConnectionRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP connection registration precedes its creation time")
	}
	if currentUnixNano >= ref.ExpiresUnixNano {
		return ConnectionRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "expired MCP connection cannot be registered")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.records[ref.ID]; ok {
		if existing.Connection.Digest == ref.Digest {
			return cloneConnectionRecord(existing), nil
		}
		return ConnectionRecord{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP connection id already binds another connection")
	}
	key := sessionKey(ref)
	if existingID, exists := m.sessions[key]; exists && existingID != ref.ID {
		existing := m.records[existingID]
		if existing.State != ConnectionClosed && existing.State != ConnectionUnknown {
			return ConnectionRecord{}, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "MCP Run/Session/Server scope has an active connection")
		}
		if ref.Epoch != existing.Connection.Epoch+1 {
			return ConnectionRecord{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP reconnect must use the next connection epoch")
		}
	}
	record := ConnectionRecord{Connection: ref, State: ConnectionRegistered, Revision: 1, UpdatedUnixNano: now.UTC().UnixNano()}
	m.records[ref.ID] = record
	m.sessions[key] = ref.ID
	return cloneConnectionRecord(record), nil
}

func (m *Manager) Transition(id string, expected core.Revision, target ConnectionState, now time.Time) (ConnectionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.records[id]
	if !ok {
		return ConnectionRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP connection not found")
	}
	if record.Revision != expected {
		return ConnectionRecord{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP connection CAS revision differs")
	}
	if now.IsZero() || now.UTC().UnixNano() < record.UpdatedUnixNano || now.UTC().UnixNano() < record.Connection.CreatedUnixNano {
		return ConnectionRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP lifecycle clock regressed")
	}
	if now.UTC().UnixNano() >= record.Connection.ExpiresUnixNano && target != ConnectionDraining && target != ConnectionClosed {
		return ConnectionRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "expired MCP connection cannot advance its active lifecycle")
	}
	if record.State == target {
		return cloneConnectionRecord(record), nil
	}
	if !allowedConnectionTransition(record.State, target) {
		return ConnectionRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "MCP connection transition is invalid")
	}
	record.State = target
	record.Revision++
	record.UpdatedUnixNano = now.UTC().UnixNano()
	m.records[id] = record
	return cloneConnectionRecord(record), nil
}

// TransitionSnapshot applies one terminal lifecycle fact to the exact active
// snapshot. Supersede, revoke and expiry are CAS-protected and never replace
// the snapshot content in place.
func (m *Manager) TransitionSnapshot(id string, expected core.Revision, snapshotDigest core.Digest, target SnapshotState, now time.Time) (ConnectionRecord, error) {
	if snapshotDigest.Validate() != nil {
		return ConnectionRecord{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidDigest, "exact MCP snapshot digest is required")
	}
	if target != SnapshotExpired && target != SnapshotRevoked && target != SnapshotSuperseded {
		return ConnectionRecord{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "MCP snapshot target must be expired, revoked or superseded")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.records[id]
	if !ok || record.Snapshot == nil {
		return ConnectionRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "active MCP snapshot not found")
	}
	if record.Revision != expected {
		return ConnectionRecord{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP snapshot CAS revision differs")
	}
	if record.Snapshot.Digest != snapshotDigest {
		return ConnectionRecord{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP snapshot differs from exact lifecycle target")
	}
	currentUnixNano := now.UTC().UnixNano()
	if now.IsZero() || currentUnixNano < record.UpdatedUnixNano || currentUnixNano < record.Snapshot.CreatedUnixNano {
		return ConnectionRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP snapshot lifecycle clock regressed")
	}
	if record.SnapshotState == target {
		return cloneConnectionRecord(record), nil
	}
	if record.SnapshotState != SnapshotActive {
		return ConnectionRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "terminal MCP snapshot cannot transition again")
	}
	switch target {
	case SnapshotExpired:
		if currentUnixNano < record.Snapshot.ExpiresUnixNano {
			return ConnectionRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "current MCP snapshot cannot be expired early")
		}
	case SnapshotRevoked, SnapshotSuperseded:
		if currentUnixNano >= record.Connection.ExpiresUnixNano || currentUnixNano >= record.Snapshot.ExpiresUnixNano {
			return ConnectionRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "expired MCP facts cannot be rewritten as revoked or superseded")
		}
	}
	record.SnapshotState = target
	if record.State == ConnectionBound {
		record.State = ConnectionDegraded
	}
	record.Revision++
	record.UpdatedUnixNano = currentUnixNano
	m.records[id] = record
	return cloneConnectionRecord(record), nil
}

func allowedConnectionTransition(from, to ConnectionState) bool {
	if to == ConnectionUnknown {
		return from == ConnectionConnecting || from == ConnectionInitializing || from == ConnectionDiscovering || from == ConnectionDraining
	}
	if to == ConnectionDraining {
		return from != ConnectionClosed && from != ConnectionRegistered
	}
	if to == ConnectionClosed {
		return from == ConnectionDraining || from == ConnectionRegistered || from == ConnectionUnknown
	}
	switch from {
	case ConnectionRegistered:
		return to == ConnectionResolving
	case ConnectionResolving:
		return to == ConnectionConnecting
	case ConnectionConnecting:
		return to == ConnectionInitializing
	case ConnectionInitializing:
		return to == ConnectionDiscovering
	case ConnectionDiscovering:
		return to == ConnectionBound || to == ConnectionDegraded
	case ConnectionBound:
		return to == ConnectionDegraded
	case ConnectionDegraded:
		return to == ConnectionBound
	default:
		return false
	}
}

func (m *Manager) BindSnapshot(id string, expected core.Revision, snapshot contract.MCPCapabilitySnapshot, now time.Time) (ConnectionRecord, error) {
	if err := snapshot.Validate(); err != nil {
		return ConnectionRecord{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.records[id]
	if !ok {
		return ConnectionRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP connection not found")
	}
	if record.Revision != expected || record.State != ConnectionDiscovering {
		return ConnectionRecord{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP snapshot requires exact discovering revision")
	}
	currentUnixNano := now.UTC().UnixNano()
	if now.IsZero() || currentUnixNano < record.Connection.CreatedUnixNano || currentUnixNano < snapshot.CreatedUnixNano {
		return ConnectionRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP snapshot binding precedes connection or snapshot creation")
	}
	if currentUnixNano >= record.Connection.ExpiresUnixNano || currentUnixNano >= snapshot.ExpiresUnixNano {
		return ConnectionRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "expired MCP connection or snapshot cannot be bound")
	}
	if snapshot.Connection.ID != record.Connection.ID || snapshot.Connection.Revision != record.Connection.Revision || snapshot.Connection.Digest != record.Connection.Digest || snapshot.ConnectionEpoch != record.Connection.Epoch || snapshot.Server != record.Connection.Server {
		return ConnectionRecord{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP snapshot belongs to another server, connection or epoch")
	}
	if currentUnixNano < record.UpdatedUnixNano {
		return ConnectionRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP lifecycle clock regressed")
	}
	value := snapshot
	record.Snapshot = &value
	record.SnapshotState = SnapshotActive
	record.State = ConnectionBound
	record.Revision++
	record.UpdatedUnixNano = now.UTC().UnixNano()
	m.records[id] = record
	return cloneConnectionRecord(record), nil
}

func (m *Manager) Inspect(id string) (ConnectionRecord, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record, ok := m.records[id]
	return cloneConnectionRecord(record), ok
}

// InspectSnapshot is the exact, read-only recovery path after an uncertain
// snapshot lifecycle reply.
func (m *Manager) InspectSnapshot(id string, snapshotDigest core.Digest) (ConnectionRecord, error) {
	if snapshotDigest.Validate() != nil {
		return ConnectionRecord{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidDigest, "exact MCP snapshot digest is required")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	record, ok := m.records[id]
	if !ok || record.Snapshot == nil {
		return ConnectionRecord{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP snapshot not found")
	}
	if record.Snapshot.Digest != snapshotDigest {
		return ConnectionRecord{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP snapshot differs from exact inspect key")
	}
	return cloneConnectionRecord(record), nil
}

func cloneConnectionRecord(record ConnectionRecord) ConnectionRecord {
	if record.Snapshot != nil {
		value := *record.Snapshot
		value.Tools = append([]contract.MCPToolObservation(nil), value.Tools...)
		value.Residuals = append([]contract.Residual(nil), value.Residuals...)
		record.Snapshot = &value
	}
	return record
}
