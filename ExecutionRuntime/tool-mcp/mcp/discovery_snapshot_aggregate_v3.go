package mcp

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type AggregateMCPDiscoverySnapshotRequestV3 struct {
	Connection               toolcontract.MCPConnectionFactRefV2 `json:"connection"`
	AppliedCommands          []toolcontract.ObjectRef            `json:"applied_commands"`
	SnapshotRevision         core.Revision                       `json:"snapshot_revision"`
	Conformance              runtimeports.NamespacedNameV2       `json:"conformance"`
	RequestedExpiresUnixNano int64                               `json:"requested_expires_unix_nano"`
}

func (r AggregateMCPDiscoverySnapshotRequestV3) Validate() error {
	return (AggregateMCPDiscoverySnapshotRequestV2{
		Connection:               r.Connection,
		AppliedCommands:          append([]toolcontract.ObjectRef(nil), r.AppliedCommands...),
		SnapshotRevision:         r.SnapshotRevision,
		Conformance:              r.Conformance,
		RequestedExpiresUnixNano: r.RequestedExpiresUnixNano,
	}).Validate()
}

func (a *MCPDiscoverySnapshotAggregatorV2) AggregateMCPDiscoverySnapshotV3(ctx context.Context, request AggregateMCPDiscoverySnapshotRequestV3) (toolcontract.MCPCapabilitySnapshotV3, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, err
	}
	if a == nil || request.Validate() != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, invalid("MCP Discovery Snapshot V3 aggregate call is invalid")
	}
	v2Request := AggregateMCPDiscoverySnapshotRequestV2{
		Connection:               request.Connection,
		AppliedCommands:          append([]toolcontract.ObjectRef(nil), request.AppliedCommands...),
		SnapshotRevision:         request.SnapshotRevision,
		Conformance:              request.Conformance,
		RequestedExpiresUnixNano: request.RequestedExpiresUnixNano,
	}
	s1 := a.clock()
	if s1.IsZero() {
		return toolcontract.MCPCapabilitySnapshotV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Discovery Snapshot V3 clock is zero")
	}
	c1, err := a.inspectAggregateClosureV2(ctx, v2Request, s1)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, err
	}
	s2 := a.clock()
	if s2.IsZero() || s2.Before(s1) {
		return toolcontract.MCPCapabilitySnapshotV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Discovery Snapshot V3 clock regressed")
	}
	c2, err := a.inspectAggregateClosureV2(ctx, v2Request, s2)
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, err
	}
	if !sameMCPDiscoveryAggregateClosureV2(c1, c2) {
		return toolcontract.MCPCapabilitySnapshotV3{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Snapshot V3 closure drifted between S1 and S2")
	}
	expires := request.RequestedExpiresUnixNano
	for _, bound := range []int64{c1.expires, c2.expires} {
		if bound < expires {
			expires = bound
		}
	}
	if !s2.Before(time.Unix(0, expires)) {
		return toolcontract.MCPCapabilitySnapshotV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Discovery Snapshot V3 closure expired")
	}
	pages := make([]toolcontract.MCPDiscoveryPageProvenanceV3, 0, len(c2.pages))
	tools := make([]toolcontract.MCPToolMaterialProvenanceV3, 0, len(c2.tools))
	resources := make([]toolcontract.MCPResourceMaterialProvenanceV3, 0, len(c2.resources))
	prompts := make([]toolcontract.MCPPromptMaterialProvenanceV3, 0, len(c2.prompts))
	for _, page := range c2.pages {
		pages = append(pages, toolcontract.MCPDiscoveryPageProvenanceV3{
			Namespace: page.Command.Namespace, PageOrdinal: page.Command.PageOrdinal,
			Command: page.Command.Ref, ProtocolReceipt: page.Receipt.Ref,
			ApplySettlement:    page.Applied.ApplySettlement,
			ResponsePageDigest: page.Receipt.ResponsePageDigest, MaterialSet: page.MaterialSet,
		})
		for _, entry := range page.ToolMaterials {
			tools = append(tools, toolcontract.MCPToolMaterialProvenanceV3{Source: entry.Source, PageReceipt: page.Receipt.Ref, Material: entry.Material})
		}
		for _, entry := range page.ResourceMaterials {
			resources = append(resources, toolcontract.MCPResourceMaterialProvenanceV3{Source: entry.Source, PageReceipt: page.Receipt.Ref, Material: entry.Material})
		}
		for _, entry := range page.PromptMaterials {
			prompts = append(prompts, toolcontract.MCPPromptMaterialProvenanceV3{Source: entry.Source, PageReceipt: page.Receipt.Ref, Material: entry.Material})
		}
	}
	source, err := core.CanonicalJSONDigest("praxis.tool-mcp.mcp-discovery-provenance", toolcontract.MCPDiscoveryContractVersionV3, "MCPDiscoveryProvenanceSourceV3", struct {
		Connect           toolcontract.MCPConnectProtocolReceiptRefV1    `json:"connect_receipt"`
		Pages             []toolcontract.MCPDiscoveryPageProvenanceV3    `json:"pages"`
		ToolMaterials     []toolcontract.MCPToolMaterialProvenanceV3     `json:"tool_materials"`
		ResourceMaterials []toolcontract.MCPResourceMaterialProvenanceV3 `json:"resource_materials"`
		PromptMaterials   []toolcontract.MCPPromptMaterialProvenanceV3   `json:"prompt_materials"`
	}{c2.connect.Ref, pages, tools, resources, prompts})
	if err != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, err
	}
	return toolcontract.SealMCPCapabilitySnapshotV3(toolcontract.MCPCapabilitySnapshotV3{
		Revision: request.SnapshotRevision, Server: c2.connection.Server,
		Connection:      toolcontract.ObjectRef{ID: c2.connection.Ref.ID, Revision: c2.connection.Ref.Revision, Digest: c2.connection.Ref.Digest},
		ConnectionEpoch: c2.connection.Coordinate.Epoch, ProtocolVersion: c2.initialize.ProtocolVersion,
		ServerInfoDigest: c2.serverInfoDigest, ServerCapabilitiesDigest: c2.serverCapabilitiesDigest,
		InstructionsDigest: c2.instructionsDigest, Tools: c2.tools, Resources: c2.resources, Prompts: c2.prompts,
		Pages: pages, ToolMaterials: tools, ResourceMaterials: resources, PromptMaterials: prompts,
		SourceDigest: source, Conformance: request.Conformance, CreatedUnixNano: s2.UnixNano(), ExpiresUnixNano: expires,
	})
}

type InMemoryMCPCapabilitySnapshotRepositoryV3 struct {
	mu      sync.RWMutex
	history map[string]map[core.Revision]toolcontract.MCPCapabilitySnapshotV3
	current map[string]toolcontract.ObjectRef
}

func NewInMemoryMCPCapabilitySnapshotRepositoryV3() *InMemoryMCPCapabilitySnapshotRepositoryV3 {
	return &InMemoryMCPCapabilitySnapshotRepositoryV3{history: map[string]map[core.Revision]toolcontract.MCPCapabilitySnapshotV3{}, current: map[string]toolcontract.ObjectRef{}}
}

func (r *InMemoryMCPCapabilitySnapshotRepositoryV3) EnsureMCPCapabilitySnapshotV3(ctx context.Context, snapshot toolcontract.MCPCapabilitySnapshotV3) (toolcontract.MCPCapabilitySnapshotV3, error) {
	return r.EnsureMCPCapabilitySnapshotRevisionV3(ctx, snapshot, nil)
}

func (r *InMemoryMCPCapabilitySnapshotRepositoryV3) EnsureMCPCapabilitySnapshotRevisionV3(ctx context.Context, snapshot toolcontract.MCPCapabilitySnapshotV3, expectedCurrent *toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return snapshot, err
	}
	if r == nil || snapshot.Validate() != nil || expectedCurrent != nil && expectedCurrent.Validate() != nil {
		return snapshot, invalid("MCP Capability Snapshot V3 Ensure is invalid")
	}
	snapshot = toolcontract.CloneMCPCapabilitySnapshotV3(snapshot)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return snapshot, err
	}
	lineage := r.history[snapshot.ID]
	if lineage == nil {
		lineage = map[core.Revision]toolcontract.MCPCapabilitySnapshotV3{}
		r.history[snapshot.ID] = lineage
	}
	if winner, ok := lineage[snapshot.Revision]; ok {
		if !reflect.DeepEqual(winner, snapshot) {
			return snapshot, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Capability Snapshot V3 revision binds another snapshot")
		}
		if r.current[snapshot.ID] != winner.ObjectRef() {
			return snapshot, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "historical MCP Capability Snapshot V3 cannot replace current")
		}
		return toolcontract.CloneMCPCapabilitySnapshotV3(winner), nil
	}
	current, exists := r.current[snapshot.ID]
	if !exists {
		if snapshot.Revision != 1 || expectedCurrent != nil {
			return snapshot, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Capability Snapshot V3 lineage must start at revision 1")
		}
	} else if expectedCurrent == nil || *expectedCurrent != current || snapshot.Revision != current.Revision+1 {
		return snapshot, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Capability Snapshot V3 successor CAS differs from current")
	}
	lineage[snapshot.Revision] = snapshot
	r.current[snapshot.ID] = snapshot.ObjectRef()
	return toolcontract.CloneMCPCapabilitySnapshotV3(snapshot), nil
}

func (r *InMemoryMCPCapabilitySnapshotRepositoryV3) InspectMCPCapabilitySnapshotV3(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, err
	}
	if r == nil || exact.Validate() != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, invalid("MCP Capability Snapshot V3 exact Inspect is invalid")
	}
	r.mu.RLock()
	value, ok := r.history[exact.ID][exact.Revision]
	r.mu.RUnlock()
	if !ok {
		return value, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Capability Snapshot V3 not found")
	}
	if value.ObjectRef() != exact {
		return value, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Capability Snapshot V3 exact Ref drifted")
	}
	return toolcontract.CloneMCPCapabilitySnapshotV3(value), nil
}

func (r *InMemoryMCPCapabilitySnapshotRepositoryV3) InspectCurrentMCPCapabilitySnapshotV3(ctx context.Context, id string) (toolcontract.MCPCapabilitySnapshotV3, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, err
	}
	if r == nil || toolcontract.ValidateStableID(id) != nil {
		return toolcontract.MCPCapabilitySnapshotV3{}, invalid("MCP Capability Snapshot V3 current Inspect is invalid")
	}
	r.mu.RLock()
	current, ok := r.current[id]
	value := r.history[id][current.Revision]
	r.mu.RUnlock()
	if !ok || value.ObjectRef() != current {
		return toolcontract.MCPCapabilitySnapshotV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "current MCP Capability Snapshot V3 not found")
	}
	return toolcontract.CloneMCPCapabilitySnapshotV3(value), nil
}

var _ toolcontract.MCPCapabilitySnapshotExactReaderV3 = (*InMemoryMCPCapabilitySnapshotRepositoryV3)(nil)
var _ toolcontract.MCPCapabilitySnapshotCurrentReaderV3 = (*InMemoryMCPCapabilitySnapshotRepositoryV3)(nil)
