package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type AggregateMCPDiscoverySnapshotRequestV2 struct {
	Connection               toolcontract.MCPConnectionFactRefV2 `json:"connection"`
	AppliedCommands          []toolcontract.ObjectRef            `json:"applied_commands"`
	SnapshotRevision         core.Revision                       `json:"snapshot_revision"`
	Conformance              runtimeports.NamespacedNameV2       `json:"conformance"`
	RequestedExpiresUnixNano int64                               `json:"requested_expires_unix_nano"`
}

func (r AggregateMCPDiscoverySnapshotRequestV2) Validate() error {
	if r.Connection.Validate() != nil || len(r.AppliedCommands) > 192 || r.SnapshotRevision == 0 || runtimeports.ValidateNamespacedNameV2(r.Conformance) != nil || r.RequestedExpiresUnixNano <= 0 {
		return invalid("MCP Discovery Snapshot aggregate request is invalid")
	}
	seen := map[string]struct{}{}
	for _, ref := range r.AppliedCommands {
		if ref.Validate() != nil {
			return invalid("MCP Discovery Snapshot command Ref is invalid")
		}
		if _, ok := seen[ref.ID]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Discovery Snapshot repeats a command")
		}
		seen[ref.ID] = struct{}{}
	}
	return nil
}

type MCPDiscoverySnapshotAggregatorV2 struct {
	connections     toolcontract.MCPConnectionFactCurrentReaderV2
	connectReceipts toolcontract.MCPConnectProtocolReceiptExactReaderV1
	commands        toolcontract.MCPDiscoveryPageCommandExactReaderV1
	applied         toolcontract.MCPDiscoveryPageApplyExactReaderV1
	physical        *InMemoryMCPDiscoveryPagePhysicalRepositoryV1
	clock           func() time.Time
}

func NewMCPDiscoverySnapshotAggregatorV2(connections toolcontract.MCPConnectionFactCurrentReaderV2, connectReceipts toolcontract.MCPConnectProtocolReceiptExactReaderV1, commands toolcontract.MCPDiscoveryPageCommandExactReaderV1, applied toolcontract.MCPDiscoveryPageApplyExactReaderV1, physical *InMemoryMCPDiscoveryPagePhysicalRepositoryV1, clock func() time.Time) (*MCPDiscoverySnapshotAggregatorV2, error) {
	if nilLikeOfficialSDKConnectV1(connections) || nilLikeOfficialSDKConnectV1(connectReceipts) || nilLikeOfficialSDKConnectV1(commands) || nilLikeOfficialSDKConnectV1(applied) || physical == nil || clock == nil {
		return nil, invalid("MCP Discovery Snapshot aggregator dependencies are incomplete")
	}
	return &MCPDiscoverySnapshotAggregatorV2{connections: connections, connectReceipts: connectReceipts, commands: commands, applied: applied, physical: physical, clock: clock}, nil
}

func (a *MCPDiscoverySnapshotAggregatorV2) AggregateMCPDiscoverySnapshotV2(ctx context.Context, request AggregateMCPDiscoverySnapshotRequestV2) (toolcontract.MCPCapabilitySnapshotV2, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	if a == nil || request.Validate() != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, invalid("MCP Discovery Snapshot aggregate call is invalid")
	}
	s1 := a.clock()
	if s1.IsZero() {
		return toolcontract.MCPCapabilitySnapshotV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Discovery Snapshot clock is zero")
	}
	c1, err := a.inspectAggregateClosureV2(ctx, request, s1)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	s2 := a.clock()
	if s2.IsZero() || s2.Before(s1) {
		return toolcontract.MCPCapabilitySnapshotV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Discovery Snapshot clock regressed")
	}
	c2, err := a.inspectAggregateClosureV2(ctx, request, s2)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	if !sameMCPDiscoveryAggregateClosureV2(c1, c2) {
		return toolcontract.MCPCapabilitySnapshotV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Snapshot closure drifted between S1 and S2")
	}
	expires := request.RequestedExpiresUnixNano
	for _, bound := range []int64{c1.expires, c2.expires} {
		if bound < expires {
			expires = bound
		}
	}
	if !s2.Before(time.Unix(0, expires)) {
		return toolcontract.MCPCapabilitySnapshotV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Discovery Snapshot closure expired")
	}
	source, err := core.CanonicalJSONDigest("praxis.tool-mcp.mcp-discovery-applied-pages", toolcontract.MCPDiscoveryContractVersionV2, "MCPDiscoveryAppliedPageSourceV2", struct {
		Connect toolcontract.MCPConnectProtocolReceiptRefV1 `json:"connect_receipt"`
		Pages   []mcpDiscoveryAppliedPageClosureV2          `json:"pages"`
	}{c2.connect.Ref, c2.pages})
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	return toolcontract.SealMCPCapabilitySnapshotV2(toolcontract.MCPCapabilitySnapshotV2{Revision: request.SnapshotRevision, Server: c2.connection.Server, Connection: toolcontract.ObjectRef{ID: c2.connection.Ref.ID, Revision: c2.connection.Ref.Revision, Digest: c2.connection.Ref.Digest}, ConnectionEpoch: c2.connection.Coordinate.Epoch, ProtocolVersion: c2.initialize.ProtocolVersion, ServerInfoDigest: c2.serverInfoDigest, ServerCapabilitiesDigest: c2.serverCapabilitiesDigest, InstructionsDigest: c2.instructionsDigest, Tools: c2.tools, Resources: c2.resources, Prompts: c2.prompts, SourceDigest: source, Conformance: request.Conformance, CreatedUnixNano: s2.UnixNano(), ExpiresUnixNano: expires})
}

type mcpDiscoveryAppliedPageClosureV2 struct {
	Command           toolcontract.MCPDiscoveryPageCommandV1                  `json:"command"`
	Applied           toolcontract.MCPDiscoveryPageAppliedCurrentProjectionV1 `json:"applied"`
	Receipt           toolcontract.MCPDiscoveryPageProtocolReceiptV1          `json:"receipt"`
	Observation       MCPDiscoveryPageObservationV1                           `json:"observation"`
	MaterialSet       toolcontract.ObjectRef                                  `json:"material_set"`
	ToolMaterials     []toolcontract.MCPDiscoveryPageToolMaterialEntryV1      `json:"tool_materials,omitempty"`
	ResourceMaterials []toolcontract.MCPDiscoveryPageResourceMaterialEntryV1  `json:"resource_materials,omitempty"`
	PromptMaterials   []toolcontract.MCPDiscoveryPagePromptMaterialEntryV1    `json:"prompt_materials,omitempty"`
}
type mcpDiscoveryAggregateClosureV2 struct {
	connection                                                     toolcontract.MCPConnectionFactV2
	connect                                                        toolcontract.MCPConnectProtocolReceiptV1
	initialize                                                     officialmcp.InitializeResult
	pages                                                          []mcpDiscoveryAppliedPageClosureV2
	tools                                                          []toolcontract.MCPToolObservationV2
	resources                                                      []toolcontract.MCPResourceObservationV2
	prompts                                                        []toolcontract.MCPPromptObservationV2
	serverInfoDigest, serverCapabilitiesDigest, instructionsDigest core.Digest
	expires                                                        int64
}

func (a *MCPDiscoverySnapshotAggregatorV2) inspectAggregateClosureV2(ctx context.Context, request AggregateMCPDiscoverySnapshotRequestV2, now time.Time) (mcpDiscoveryAggregateClosureV2, error) {
	var c mcpDiscoveryAggregateClosureV2
	var err error
	c.connection, err = a.connections.InspectCurrentMCPConnectionFactV2(ctx, request.Connection)
	if err != nil {
		return c, err
	}
	if c.connection.Validate() != nil || c.connection.Ref != request.Connection || now.UnixNano() < c.connection.CreatedUnixNano || !now.Before(time.Unix(0, c.connection.ExpiresUnixNano)) {
		return c, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Discovery Snapshot connection is not exact-current")
	}
	c.connect, err = a.connectReceipts.InspectMCPConnectProtocolReceiptV1(ctx, c.connection.ProtocolReceipt)
	if err != nil {
		return c, err
	}
	if c.connect.Validate() != nil || c.connect.Ref != c.connection.ProtocolReceipt || c.connect.NegotiatedProtocol != c.connection.NegotiatedProtocol || c.connect.ObservedUnixNano > now.UnixNano() {
		return c, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Snapshot Connect receipt drifted")
	}
	if err = json.Unmarshal(c.connect.InitializeResponse, &c.initialize); err != nil || c.initialize.ServerInfo == nil || c.initialize.Capabilities == nil || c.initialize.ProtocolVersion != c.connection.NegotiatedProtocol {
		return c, core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "MCP Connect initialize receipt cannot seed Discovery Snapshot")
	}
	c.serverInfoDigest, err = digestOfficialSDKValueV1("MCPServerInfoV1", c.initialize.ServerInfo)
	if err != nil {
		return c, err
	}
	c.serverCapabilitiesDigest, err = digestOfficialSDKValueV1("MCPServerCapabilitiesV1", c.initialize.Capabilities)
	if err != nil {
		return c, err
	}
	c.instructionsDigest = core.DigestBytes([]byte(c.initialize.Instructions))
	c.expires = c.connection.ExpiresUnixNano
	for _, ref := range request.AppliedCommands {
		command, err := a.commands.InspectMCPDiscoveryPageCommandV1(ctx, ref)
		if err != nil {
			return c, err
		}
		applied, err := a.applied.InspectCurrentMCPDiscoveryPageAppliedV1(ctx, ref, 5*time.Second)
		if err != nil {
			return c, err
		}
		if applied.Validate(now) != nil || applied.Command != ref {
			return c, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery applied current is not exact")
		}
		receipt, err := a.physical.InspectMCPDiscoveryPageProtocolReceiptV1(ctx, applied.ProtocolReceipt)
		if err != nil {
			return c, err
		}
		observation, err := a.physical.InspectMCPDiscoveryPageObservationV1(ctx, applied.ProtocolReceipt)
		if err != nil {
			return c, err
		}
		observationDigest, digestErr := observation.computeDigestV1()
		itemCount := uint32(len(observation.Tools) + len(observation.Resources) + len(observation.Prompts))
		if command.Validate() != nil || command.Ref != ref || command.Connection != c.connection.Ref || command.Namespace != applied.Namespace || command.PageOrdinal != applied.PageOrdinal {
			return c, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery command and Applied current drifted")
		}
		if receipt.Validate() != nil || receipt.Ref != applied.ProtocolReceipt || receipt.Command != command.Ref || receipt.Namespace != command.Namespace || receipt.PageOrdinal != command.PageOrdinal || receipt.CursorDigest != command.CursorDigest || receipt.NextCursorDigest != applied.NextCursorDigest || !bytes.Equal(receipt.NextCursor, applied.NextCursor) || receipt.ResponsePageDigest != observation.Digest || receipt.ItemCount != itemCount {
			return c, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery protocol Receipt drifted")
		}
		if observation.Namespace != command.Namespace || digestErr != nil || observationDigest != observation.Digest || !bytes.Equal(observation.NextCursor, applied.NextCursor) {
			return c, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery provider Observation drifted")
		}
		if applied.ExpiresUnixNano < c.expires {
			c.expires = applied.ExpiresUnixNano
		}
		page := mcpDiscoveryAppliedPageClosureV2{Command: command, Applied: applied, Receipt: receipt, Observation: observation}
		switch command.Namespace {
		case runtimeports.MCPDiscoveryPageToolsNamespaceV1:
			set, err := a.physical.InspectMCPDiscoveryPageToolMaterialSetV1(ctx, receipt.Ref)
			if err != nil {
				return c, err
			}
			if set.Validate() != nil || set.Receipt != receipt.Ref || set.Command != command.Ref || set.Connection != command.Connection || set.ResponsePageDigest != observation.Digest || len(set.Entries) != len(observation.Tools) {
				return c, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Tool Material Set drifted")
			}
			for index := range set.Entries {
				if set.Entries[index].Source != observation.Tools[index] {
					return c, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Tool Material source drifted")
				}
			}
			page.MaterialSet = set.Ref
			page.ToolMaterials = append([]toolcontract.MCPDiscoveryPageToolMaterialEntryV1(nil), set.Entries...)
		case runtimeports.MCPDiscoveryPageResourcesNamespaceV1:
			set, err := a.physical.InspectMCPDiscoveryPageResourceMaterialSetV1(ctx, receipt.Ref)
			if err != nil {
				return c, err
			}
			if set.Validate() != nil || set.Receipt != receipt.Ref || set.Command != command.Ref || set.Connection != command.Connection || set.ResponsePageDigest != observation.Digest || len(set.Entries) != len(observation.Resources) {
				return c, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Resource Material Set drifted")
			}
			for index := range set.Entries {
				if set.Entries[index].Source != observation.Resources[index] {
					return c, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Resource Material source drifted")
				}
			}
			page.MaterialSet = set.Ref
			page.ResourceMaterials = append([]toolcontract.MCPDiscoveryPageResourceMaterialEntryV1(nil), set.Entries...)
		case runtimeports.MCPDiscoveryPagePromptsNamespaceV1:
			set, err := a.physical.InspectMCPDiscoveryPagePromptMaterialSetV1(ctx, receipt.Ref)
			if err != nil {
				return c, err
			}
			if set.Validate() != nil || set.Receipt != receipt.Ref || set.Command != command.Ref || set.Connection != command.Connection || set.ResponsePageDigest != observation.Digest || len(set.Entries) != len(observation.Prompts) {
				return c, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Prompt Material Set drifted")
			}
			for index := range set.Entries {
				if set.Entries[index].Source != observation.Prompts[index] {
					return c, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Prompt Material source drifted")
				}
			}
			page.MaterialSet = set.Ref
			page.PromptMaterials = append([]toolcontract.MCPDiscoveryPagePromptMaterialEntryV1(nil), set.Entries...)
		}
		c.pages = append(c.pages, page)
	}
	sort.Slice(c.pages, func(i, j int) bool {
		if c.pages[i].Command.Namespace == c.pages[j].Command.Namespace {
			return c.pages[i].Command.PageOrdinal < c.pages[j].Command.PageOrdinal
		}
		return c.pages[i].Command.Namespace < c.pages[j].Command.Namespace
	})
	expected := map[runtimeports.NamespacedNameV2]bool{runtimeports.MCPDiscoveryPageToolsNamespaceV1: c.initialize.Capabilities.Tools != nil, runtimeports.MCPDiscoveryPageResourcesNamespaceV1: c.initialize.Capabilities.Resources != nil, runtimeports.MCPDiscoveryPagePromptsNamespaceV1: c.initialize.Capabilities.Prompts != nil}
	for namespace, required := range expected {
		pages := make([]mcpDiscoveryAppliedPageClosureV2, 0)
		for _, page := range c.pages {
			if page.Command.Namespace == namespace {
				pages = append(pages, page)
			}
		}
		if required && len(pages) == 0 || !required && len(pages) != 0 {
			return c, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "MCP Discovery Snapshot namespace page set is incomplete")
		}
		cursor := []byte(nil)
		for ordinal, page := range pages {
			if page.Command.PageOrdinal != uint32(ordinal) || page.Command.CursorDigest != core.DigestBytes(cursor) || string(page.Command.Cursor) != string(cursor) {
				return c, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Snapshot page cursor chain drifted")
			}
			cursor = append([]byte(nil), page.Applied.NextCursor...)
			c.tools = append(c.tools, page.Observation.Tools...)
			c.resources = append(c.resources, page.Observation.Resources...)
			c.prompts = append(c.prompts, page.Observation.Prompts...)
		}
		if len(cursor) != 0 {
			return c, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "MCP Discovery Snapshot namespace has no settled terminal page")
		}
	}
	if !now.Before(time.Unix(0, c.expires)) {
		return c, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Discovery Snapshot source closure expired")
	}
	return c, nil
}

func sameMCPDiscoveryAggregateClosureV2(left, right mcpDiscoveryAggregateClosureV2) bool {
	if left.connection != right.connection || left.connect.Ref != right.connect.Ref || !reflect.DeepEqual(left.connect, right.connect) || !reflect.DeepEqual(left.initialize, right.initialize) || left.serverInfoDigest != right.serverInfoDigest || left.serverCapabilitiesDigest != right.serverCapabilitiesDigest || left.instructionsDigest != right.instructionsDigest || len(left.pages) != len(right.pages) {
		return false
	}
	for index := range left.pages {
		l, r := left.pages[index], right.pages[index]
		if l.Command.Ref != r.Command.Ref || !reflect.DeepEqual(l.Command, r.Command) || !reflect.DeepEqual(l.Receipt, r.Receipt) || !reflect.DeepEqual(l.Observation, r.Observation) || l.MaterialSet != r.MaterialSet || !reflect.DeepEqual(l.ToolMaterials, r.ToolMaterials) || !reflect.DeepEqual(l.ResourceMaterials, r.ResourceMaterials) || !reflect.DeepEqual(l.PromptMaterials, r.PromptMaterials) {
			return false
		}
		l.Applied.CheckedUnixNano, l.Applied.ExpiresUnixNano, l.Applied.Digest = 0, 0, ""
		r.Applied.CheckedUnixNano, r.Applied.ExpiresUnixNano, r.Applied.Digest = 0, 0, ""
		if !reflect.DeepEqual(l.Applied, r.Applied) || right.pages[index].Applied.CheckedUnixNano < left.pages[index].Applied.CheckedUnixNano {
			return false
		}
	}
	return reflect.DeepEqual(left.tools, right.tools) && reflect.DeepEqual(left.resources, right.resources) && reflect.DeepEqual(left.prompts, right.prompts)
}

type InMemoryMCPCapabilitySnapshotRepositoryV2 struct {
	mu      sync.RWMutex
	history map[string]map[core.Revision]toolcontract.MCPCapabilitySnapshotV2
	current map[string]toolcontract.ObjectRef
}

func NewInMemoryMCPCapabilitySnapshotRepositoryV2() *InMemoryMCPCapabilitySnapshotRepositoryV2 {
	return &InMemoryMCPCapabilitySnapshotRepositoryV2{history: map[string]map[core.Revision]toolcontract.MCPCapabilitySnapshotV2{}, current: map[string]toolcontract.ObjectRef{}}
}
func (r *InMemoryMCPCapabilitySnapshotRepositoryV2) EnsureMCPCapabilitySnapshotV2(ctx context.Context, snapshot toolcontract.MCPCapabilitySnapshotV2) (toolcontract.MCPCapabilitySnapshotV2, error) {
	return r.EnsureMCPCapabilitySnapshotRevisionV2(ctx, snapshot, nil)
}

// EnsureMCPCapabilitySnapshotRevisionV2 creates revision 1 or atomically
// advances current by exactly one revision. A retry of the canonical winner is
// idempotent; an old historical revision can never roll current back.
func (r *InMemoryMCPCapabilitySnapshotRepositoryV2) EnsureMCPCapabilitySnapshotRevisionV2(ctx context.Context, snapshot toolcontract.MCPCapabilitySnapshotV2, expectedCurrent *toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV2, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return snapshot, err
	}
	if r == nil || snapshot.Validate() != nil || expectedCurrent != nil && expectedCurrent.Validate() != nil {
		return snapshot, invalid("MCP Capability Snapshot Ensure is invalid")
	}
	snapshot = toolcontract.CloneMCPCapabilitySnapshotV2(snapshot)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return snapshot, err
	}
	lineage := r.history[snapshot.ID]
	if lineage == nil {
		lineage = map[core.Revision]toolcontract.MCPCapabilitySnapshotV2{}
		r.history[snapshot.ID] = lineage
	}
	if winner, ok := lineage[snapshot.Revision]; ok {
		if !reflect.DeepEqual(winner, snapshot) {
			return snapshot, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Capability Snapshot revision binds another snapshot")
		}
		if r.current[snapshot.ID] != winner.ObjectRef() {
			return snapshot, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "historical MCP Capability Snapshot cannot replace current")
		}
		return toolcontract.CloneMCPCapabilitySnapshotV2(winner), nil
	}
	current, exists := r.current[snapshot.ID]
	if !exists {
		if snapshot.Revision != 1 || expectedCurrent != nil {
			return snapshot, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Capability Snapshot lineage must start at revision 1")
		}
	} else if expectedCurrent == nil || *expectedCurrent != current || snapshot.Revision != current.Revision+1 {
		return snapshot, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Capability Snapshot successor CAS differs from current")
	}
	lineage[snapshot.Revision] = snapshot
	r.current[snapshot.ID] = snapshot.ObjectRef()
	return toolcontract.CloneMCPCapabilitySnapshotV2(snapshot), nil
}
func (r *InMemoryMCPCapabilitySnapshotRepositoryV2) InspectMCPCapabilitySnapshotV2(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV2, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	if r == nil || exact.Validate() != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, invalid("MCP Capability Snapshot exact Inspect is invalid")
	}
	r.mu.RLock()
	value, ok := r.history[exact.ID][exact.Revision]
	r.mu.RUnlock()
	if !ok {
		return value, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Capability Snapshot not found")
	}
	if value.ObjectRef() != exact {
		return value, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Capability Snapshot exact Ref drifted")
	}
	return toolcontract.CloneMCPCapabilitySnapshotV2(value), nil
}

func (r *InMemoryMCPCapabilitySnapshotRepositoryV2) InspectCurrentMCPCapabilitySnapshotV2(ctx context.Context, id string) (toolcontract.MCPCapabilitySnapshotV2, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, err
	}
	if r == nil || toolcontract.ValidateStableID(id) != nil {
		return toolcontract.MCPCapabilitySnapshotV2{}, invalid("MCP Capability Snapshot current Inspect is invalid")
	}
	r.mu.RLock()
	current, ok := r.current[id]
	value := r.history[id][current.Revision]
	r.mu.RUnlock()
	if !ok || value.ObjectRef() != current {
		return toolcontract.MCPCapabilitySnapshotV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "current MCP Capability Snapshot not found")
	}
	return toolcontract.CloneMCPCapabilitySnapshotV2(value), nil
}

var _ toolcontract.MCPCapabilitySnapshotExactReaderV2 = (*InMemoryMCPCapabilitySnapshotRepositoryV2)(nil)
var _ toolcontract.MCPCapabilitySnapshotCurrentReaderV2 = (*InMemoryMCPCapabilitySnapshotRepositoryV2)(nil)

var _ toolcontract.MCPCapabilitySnapshotExactReaderV2 = (*InMemoryMCPCapabilitySnapshotRepositoryV2)(nil)
