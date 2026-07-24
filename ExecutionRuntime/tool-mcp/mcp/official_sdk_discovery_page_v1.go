package mcp

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPDiscoveryPagePhysicalStateV1 string

const (
	MCPDiscoveryPagePhysicalAdmittedV1 MCPDiscoveryPagePhysicalStateV1 = "admitted"
	MCPDiscoveryPagePhysicalObservedV1 MCPDiscoveryPagePhysicalStateV1 = "observed"
	MCPDiscoveryPagePhysicalUnknownV1  MCPDiscoveryPagePhysicalStateV1 = "unknown"
)

type MCPDiscoveryPageObservationV1 struct {
	Namespace  runtimeports.NamespacedNameV2           `json:"namespace"`
	Tools      []toolcontract.MCPToolObservationV2     `json:"tools,omitempty"`
	Resources  []toolcontract.MCPResourceObservationV2 `json:"resources,omitempty"`
	Prompts    []toolcontract.MCPPromptObservationV2   `json:"prompts,omitempty"`
	NextCursor []byte                                  `json:"next_cursor"`
	Digest     core.Digest                             `json:"digest"`
}

func (o MCPDiscoveryPageObservationV1) computeDigestV1() (core.Digest, error) {
	o.Digest = ""
	return core.CanonicalJSONDigest("praxis.tool-mcp.mcp-discovery-page-observation", toolcontract.MCPDiscoveryPageReceiptContractVersionV1, "MCPDiscoveryPageObservationV1", o)
}

type MCPDiscoveryPagePhysicalEntryV1 struct {
	ID                  string                                                         `json:"id"`
	Revision            core.Revision                                                  `json:"revision"`
	Authorization       runtimeports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1 `json:"authorization"`
	Command             toolcontract.ObjectRef                                         `json:"command"`
	Connection          toolcontract.MCPConnectionFactRefV2                            `json:"connection"`
	AdmissionReceipt    runtimeports.ControlledOperationProviderAdmissionReceiptRefV2  `json:"admission_receipt"`
	State               MCPDiscoveryPagePhysicalStateV1                                `json:"state"`
	ProtocolReceipt     *toolcontract.MCPDiscoveryPageProtocolReceiptV1                `json:"protocol_receipt,omitempty"`
	Observation         *MCPDiscoveryPageObservationV1                                 `json:"observation,omitempty"`
	ToolMaterials       []toolcontract.MCPToolDiscoveryMaterialV1                      `json:"tool_materials,omitempty"`
	ResourceMaterials   []toolcontract.MCPResourceDiscoveryMaterialV1                  `json:"resource_materials,omitempty"`
	PromptMaterials     []toolcontract.MCPPromptDiscoveryMaterialV1                    `json:"prompt_materials,omitempty"`
	UnknownReasonDigest core.Digest                                                    `json:"unknown_reason_digest,omitempty"`
	UpdatedUnixNano     int64                                                          `json:"updated_unix_nano"`
}

type InMemoryMCPDiscoveryPagePhysicalRepositoryV1 struct {
	mu      sync.RWMutex
	entries map[string]MCPDiscoveryPagePhysicalEntryV1
}

func NewInMemoryMCPDiscoveryPagePhysicalRepositoryV1() *InMemoryMCPDiscoveryPagePhysicalRepositoryV1 {
	return &InMemoryMCPDiscoveryPagePhysicalRepositoryV1{entries: make(map[string]MCPDiscoveryPagePhysicalEntryV1)}
}

func (r *InMemoryMCPDiscoveryPagePhysicalRepositoryV1) beginV1(ctx context.Context, authorization runtimeports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1, command toolcontract.MCPDiscoveryPageCommandV1, now time.Time) (MCPDiscoveryPagePhysicalEntryV1, bool, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return MCPDiscoveryPagePhysicalEntryV1{}, false, err
	}
	if r == nil || authorization.ValidateCurrent(now) != nil || command.Validate() != nil {
		return MCPDiscoveryPagePhysicalEntryV1{}, false, invalid("MCP Discovery Page physical admission is invalid")
	}
	id := "mcp-discovery-page-entry-" + strings.TrimPrefix(string(authorization.StableKeyDigest), "sha256:")
	admission, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{ID: id + "-admission", Revision: 1, StableKeyDigest: authorization.StableKeyDigest, Admitted: true})
	if err != nil {
		return MCPDiscoveryPagePhysicalEntryV1{}, false, err
	}
	entry := MCPDiscoveryPagePhysicalEntryV1{ID: id, Revision: 1, Authorization: authorization, Command: command.Ref, Connection: command.Connection, AdmissionReceipt: admission, State: MCPDiscoveryPagePhysicalAdmittedV1, UpdatedUnixNano: now.UnixNano()}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return MCPDiscoveryPagePhysicalEntryV1{}, false, err
	}
	if winner, ok := r.entries[id]; ok {
		if winner.Authorization.Digest != authorization.Digest || winner.Command != command.Ref || winner.Connection != command.Connection {
			return MCPDiscoveryPagePhysicalEntryV1{}, false, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Discovery Page stable key binds another command")
		}
		return cloneMCPDiscoveryPagePhysicalEntryV1(winner), false, nil
	}
	r.entries[id] = entry
	return cloneMCPDiscoveryPagePhysicalEntryV1(entry), true, nil
}

func (r *InMemoryMCPDiscoveryPagePhysicalRepositoryV1) completeV1(ctx context.Context, stable core.Digest, receipt toolcontract.MCPDiscoveryPageProtocolReceiptV1, observation MCPDiscoveryPageObservationV1, toolMaterials []toolcontract.MCPToolDiscoveryMaterialV1, resourceMaterials []toolcontract.MCPResourceDiscoveryMaterialV1, promptMaterials []toolcontract.MCPPromptDiscoveryMaterialV1, now time.Time) error {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return err
	}
	id := "mcp-discovery-page-entry-" + strings.TrimPrefix(string(stable), "sha256:")
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[id]
	if !ok || receipt.Validate() != nil || observation.Digest != receipt.ResponsePageDigest || receipt.Command != entry.Command || receipt.AdmissionReceipt != entry.AdmissionReceipt || validateMCPDiscoveryPageToolMaterialsV1(receipt, entry.Connection, observation, toolMaterials) != nil || validateMCPDiscoveryPageResourceMaterialsV1(receipt, entry.Connection, observation, resourceMaterials) != nil || validateMCPDiscoveryPagePromptMaterialsV1(receipt, entry.Connection, observation, promptMaterials) != nil {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Discovery Page completion closure drifted")
	}
	if entry.State == MCPDiscoveryPagePhysicalObservedV1 {
		if entry.ProtocolReceipt == nil || entry.ProtocolReceipt.Ref != receipt.Ref || !reflect.DeepEqual(entry.ToolMaterials, toolMaterials) || !reflect.DeepEqual(entry.ResourceMaterials, resourceMaterials) || !reflect.DeepEqual(entry.PromptMaterials, promptMaterials) {
			return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Discovery Page already observed another response")
		}
		return nil
	}
	if entry.State != MCPDiscoveryPagePhysicalAdmittedV1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "unknown MCP Discovery Page is inspect-only")
	}
	entry.Revision++
	entry.State, entry.ProtocolReceipt, entry.Observation = MCPDiscoveryPagePhysicalObservedV1, &receipt, &observation
	entry.ToolMaterials = cloneMCPToolDiscoveryMaterialsV1(toolMaterials)
	entry.ResourceMaterials = cloneMCPResourceDiscoveryMaterialsV1(resourceMaterials)
	entry.PromptMaterials = cloneMCPPromptDiscoveryMaterialsV1(promptMaterials)
	entry.UpdatedUnixNano = now.UnixNano()
	r.entries[id] = cloneMCPDiscoveryPagePhysicalEntryV1(entry)
	return nil
}

func (r *InMemoryMCPDiscoveryPagePhysicalRepositoryV1) markUnknownV1(stable core.Digest, reason core.Digest, now time.Time) {
	id := "mcp-discovery-page-entry-" + strings.TrimPrefix(string(stable), "sha256:")
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[id]
	if ok && entry.State == MCPDiscoveryPagePhysicalAdmittedV1 {
		entry.Revision++
		entry.State, entry.UnknownReasonDigest, entry.UpdatedUnixNano = MCPDiscoveryPagePhysicalUnknownV1, reason, now.UnixNano()
		r.entries[id] = entry
	}
}

func (r *InMemoryMCPDiscoveryPagePhysicalRepositoryV1) InspectMCPDiscoveryPagePhysicalV1(ctx context.Context, stable core.Digest) (MCPDiscoveryPagePhysicalEntryV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return MCPDiscoveryPagePhysicalEntryV1{}, err
	}
	id := "mcp-discovery-page-entry-" + strings.TrimPrefix(string(stable), "sha256:")
	r.mu.RLock()
	entry, ok := r.entries[id]
	r.mu.RUnlock()
	if !ok {
		return MCPDiscoveryPagePhysicalEntryV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Discovery Page Entry not found")
	}
	return cloneMCPDiscoveryPagePhysicalEntryV1(entry), nil
}

func (r *InMemoryMCPDiscoveryPagePhysicalRepositoryV1) InspectMCPDiscoveryPagePhysicalByCommandV1(ctx context.Context, exact toolcontract.ObjectRef) (MCPDiscoveryPagePhysicalEntryV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return MCPDiscoveryPagePhysicalEntryV1{}, err
	}
	if r == nil || exact.Validate() != nil {
		return MCPDiscoveryPagePhysicalEntryV1{}, invalid("MCP Discovery Page physical Command Inspect is invalid")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.entries {
		if entry.Command.ID == exact.ID {
			if entry.Command != exact {
				return MCPDiscoveryPagePhysicalEntryV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page physical Command Ref drifted")
			}
			return cloneMCPDiscoveryPagePhysicalEntryV1(entry), nil
		}
	}
	return MCPDiscoveryPagePhysicalEntryV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Discovery Page physical Command not found")
}

func (r *InMemoryMCPDiscoveryPagePhysicalRepositoryV1) InspectMCPDiscoveryPageProtocolReceiptV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageProtocolReceiptV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPDiscoveryPageProtocolReceiptV1{}, err
	}
	if r == nil || exact.Validate() != nil {
		return toolcontract.MCPDiscoveryPageProtocolReceiptV1{}, invalid("MCP Discovery Page Receipt exact Inspect is invalid")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.entries {
		if entry.ProtocolReceipt != nil && entry.ProtocolReceipt.Ref.ID == exact.ID {
			if entry.ProtocolReceipt.Ref != exact {
				return toolcontract.MCPDiscoveryPageProtocolReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page Receipt exact Ref drifted")
			}
			return *cloneMCPDiscoveryPagePhysicalEntryV1(entry).ProtocolReceipt, nil
		}
	}
	return toolcontract.MCPDiscoveryPageProtocolReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Discovery Page Receipt not found")
}

func (r *InMemoryMCPDiscoveryPagePhysicalRepositoryV1) InspectMCPDiscoveryPageObservationV1(ctx context.Context, exact toolcontract.ObjectRef) (MCPDiscoveryPageObservationV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return MCPDiscoveryPageObservationV1{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.entries {
		if entry.ProtocolReceipt != nil && entry.ProtocolReceipt.Ref == exact && entry.Observation != nil {
			return *cloneMCPDiscoveryPagePhysicalEntryV1(entry).Observation, nil
		}
	}
	return MCPDiscoveryPageObservationV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Discovery Page Observation not found")
}

func (r *InMemoryMCPDiscoveryPagePhysicalRepositoryV1) InspectExactMCPToolDiscoveryMaterialV1(ctx context.Context, exact toolcontract.MCPToolDiscoveryMaterialRefV1) (toolcontract.MCPToolDiscoveryMaterialV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPToolDiscoveryMaterialV1{}, err
	}
	if r == nil || exact.Validate() != nil {
		return toolcontract.MCPToolDiscoveryMaterialV1{}, invalid("MCP Tool Discovery Material exact Inspect is invalid")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.entries {
		for _, material := range entry.ToolMaterials {
			if material.Ref.ID != exact.ID {
				continue
			}
			if material.Ref != exact {
				return toolcontract.MCPToolDiscoveryMaterialV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Tool Discovery Material exact Ref drifted")
			}
			if err := material.Validate(); err != nil {
				return toolcontract.MCPToolDiscoveryMaterialV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "stored MCP Tool Discovery Material is invalid")
			}
			return material.Clone(), nil
		}
	}
	return toolcontract.MCPToolDiscoveryMaterialV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Tool Discovery Material not found")
}

func (r *InMemoryMCPDiscoveryPagePhysicalRepositoryV1) InspectExactMCPResourceDiscoveryMaterialV1(ctx context.Context, exact toolcontract.MCPResourceDiscoveryMaterialRefV1) (toolcontract.MCPResourceDiscoveryMaterialV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPResourceDiscoveryMaterialV1{}, err
	}
	if r == nil || exact.Validate() != nil {
		return toolcontract.MCPResourceDiscoveryMaterialV1{}, invalid("MCP Resource Discovery Material exact Inspect is invalid")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.entries {
		for _, material := range entry.ResourceMaterials {
			if material.Ref.ID != exact.ID {
				continue
			}
			if material.Ref != exact {
				return toolcontract.MCPResourceDiscoveryMaterialV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Resource Discovery Material exact Ref drifted")
			}
			if err := material.Validate(); err != nil {
				return toolcontract.MCPResourceDiscoveryMaterialV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "stored MCP Resource Discovery Material is invalid")
			}
			return material.Clone(), nil
		}
	}
	return toolcontract.MCPResourceDiscoveryMaterialV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Resource Discovery Material not found")
}

func (r *InMemoryMCPDiscoveryPagePhysicalRepositoryV1) InspectExactMCPPromptDiscoveryMaterialV1(ctx context.Context, exact toolcontract.MCPPromptDiscoveryMaterialRefV1) (toolcontract.MCPPromptDiscoveryMaterialV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPPromptDiscoveryMaterialV1{}, err
	}
	if r == nil || exact.Validate() != nil {
		return toolcontract.MCPPromptDiscoveryMaterialV1{}, invalid("MCP Prompt Discovery Material exact Inspect is invalid")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.entries {
		for _, material := range entry.PromptMaterials {
			if material.Ref.ID != exact.ID {
				continue
			}
			if material.Ref != exact {
				return toolcontract.MCPPromptDiscoveryMaterialV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Prompt Discovery Material exact Ref drifted")
			}
			if err := material.Validate(); err != nil {
				return toolcontract.MCPPromptDiscoveryMaterialV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "stored MCP Prompt Discovery Material is invalid")
			}
			return material.Clone(), nil
		}
	}
	return toolcontract.MCPPromptDiscoveryMaterialV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Prompt Discovery Material not found")
}

func (r *InMemoryMCPDiscoveryPagePhysicalRepositoryV1) InspectMCPDiscoveryPageToolMaterialSetV1(ctx context.Context, exactReceipt toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageToolMaterialSetV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, err
	}
	if r == nil || exactReceipt.Validate() != nil {
		return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, invalid("MCP Discovery Page Tool Material Set exact Inspect is invalid")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.entries {
		if entry.ProtocolReceipt == nil || entry.ProtocolReceipt.Ref.ID != exactReceipt.ID {
			continue
		}
		if entry.ProtocolReceipt.Ref != exactReceipt {
			return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page Tool Material Set receipt Ref drifted")
		}
		if entry.State != MCPDiscoveryPagePhysicalObservedV1 || entry.Observation == nil || entry.ProtocolReceipt.Namespace != runtimeports.MCPDiscoveryPageToolsNamespaceV1 || entry.Observation.Namespace != runtimeports.MCPDiscoveryPageToolsNamespaceV1 {
			return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "MCP Discovery Page is not one observed Tools page")
		}
		entries := make([]toolcontract.MCPDiscoveryPageToolMaterialEntryV1, 0, len(entry.ToolMaterials))
		for _, material := range entry.ToolMaterials {
			if material.Validate() != nil || material.Command != entry.Command || material.Connection != entry.Connection {
				return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "stored MCP Discovery Page Tool Material closure drifted")
			}
			entries = append(entries, toolcontract.MCPDiscoveryPageToolMaterialEntryV1{Source: material.Source, Material: material.Ref})
		}
		set, err := toolcontract.SealMCPDiscoveryPageToolMaterialSetV1(toolcontract.MCPDiscoveryPageToolMaterialSetV1{Receipt: exactReceipt, Command: entry.Command, Connection: entry.Connection, ResponsePageDigest: entry.ProtocolReceipt.ResponsePageDigest, Entries: entries})
		if err != nil {
			return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, err
		}
		return toolcontract.CloneMCPDiscoveryPageToolMaterialSetV1(set), nil
	}
	return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Discovery Page Tool Material Set receipt not found")
}

func (r *InMemoryMCPDiscoveryPagePhysicalRepositoryV1) InspectMCPDiscoveryPageResourceMaterialSetV1(ctx context.Context, exactReceipt toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageResourceMaterialSetV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, err
	}
	if r == nil || exactReceipt.Validate() != nil {
		return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, invalid("MCP Discovery Page Resource Material Set exact Inspect is invalid")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.entries {
		if entry.ProtocolReceipt == nil || entry.ProtocolReceipt.Ref.ID != exactReceipt.ID {
			continue
		}
		if entry.ProtocolReceipt.Ref != exactReceipt {
			return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page Resource Material Set receipt Ref drifted")
		}
		if entry.State != MCPDiscoveryPagePhysicalObservedV1 || entry.Observation == nil || entry.ProtocolReceipt.Namespace != runtimeports.MCPDiscoveryPageResourcesNamespaceV1 || entry.Observation.Namespace != runtimeports.MCPDiscoveryPageResourcesNamespaceV1 {
			return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "MCP Discovery Page is not one observed Resources page")
		}
		entries := make([]toolcontract.MCPDiscoveryPageResourceMaterialEntryV1, 0, len(entry.ResourceMaterials))
		for _, material := range entry.ResourceMaterials {
			if material.Validate() != nil || material.Command != entry.Command || material.Connection != entry.Connection {
				return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "stored MCP Discovery Page Resource Material closure drifted")
			}
			entries = append(entries, toolcontract.MCPDiscoveryPageResourceMaterialEntryV1{Source: material.Source, Material: material.Ref})
		}
		set, err := toolcontract.SealMCPDiscoveryPageResourceMaterialSetV1(toolcontract.MCPDiscoveryPageResourceMaterialSetV1{Receipt: exactReceipt, Command: entry.Command, Connection: entry.Connection, ResponsePageDigest: entry.ProtocolReceipt.ResponsePageDigest, Entries: entries})
		if err != nil {
			return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, err
		}
		return toolcontract.CloneMCPDiscoveryPageResourceMaterialSetV1(set), nil
	}
	return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Discovery Page Resource Material Set receipt not found")
}

func (r *InMemoryMCPDiscoveryPagePhysicalRepositoryV1) InspectMCPDiscoveryPagePromptMaterialSetV1(ctx context.Context, exactReceipt toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPagePromptMaterialSetV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, err
	}
	if r == nil || exactReceipt.Validate() != nil {
		return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, invalid("MCP Discovery Page Prompt Material Set exact Inspect is invalid")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.entries {
		if entry.ProtocolReceipt == nil || entry.ProtocolReceipt.Ref.ID != exactReceipt.ID {
			continue
		}
		if entry.ProtocolReceipt.Ref != exactReceipt {
			return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page Prompt Material Set receipt Ref drifted")
		}
		if entry.State != MCPDiscoveryPagePhysicalObservedV1 || entry.Observation == nil || entry.ProtocolReceipt.Namespace != runtimeports.MCPDiscoveryPagePromptsNamespaceV1 || entry.Observation.Namespace != runtimeports.MCPDiscoveryPagePromptsNamespaceV1 {
			return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "MCP Discovery Page is not one observed Prompts page")
		}
		entries := make([]toolcontract.MCPDiscoveryPagePromptMaterialEntryV1, 0, len(entry.PromptMaterials))
		for _, material := range entry.PromptMaterials {
			if material.Validate() != nil || material.Command != entry.Command || material.Connection != entry.Connection {
				return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "stored MCP Discovery Page Prompt Material closure drifted")
			}
			entries = append(entries, toolcontract.MCPDiscoveryPagePromptMaterialEntryV1{Source: material.Source, Material: material.Ref})
		}
		set, err := toolcontract.SealMCPDiscoveryPagePromptMaterialSetV1(toolcontract.MCPDiscoveryPagePromptMaterialSetV1{Receipt: exactReceipt, Command: entry.Command, Connection: entry.Connection, ResponsePageDigest: entry.ProtocolReceipt.ResponsePageDigest, Entries: entries})
		if err != nil {
			return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, err
		}
		return toolcontract.CloneMCPDiscoveryPagePromptMaterialSetV1(set), nil
	}
	return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Discovery Page Prompt Material Set receipt not found")
}

type OfficialSDKDiscoveryPageExecutorV1 struct {
	commands     toolcontract.MCPDiscoveryPageCommandExactReaderV1
	connections  toolcontract.MCPConnectionFactCurrentReaderV2
	availability runtimeports.MCPConnectionAvailabilityNeutralCurrentReaderV1
	sessions     MCPDiscoveryPageSessionReaderV1
	entries      *InMemoryMCPDiscoveryPagePhysicalRepositoryV1
	clock        func() time.Time
}

func NewOfficialSDKDiscoveryPageExecutorV1(commands toolcontract.MCPDiscoveryPageCommandExactReaderV1, connections toolcontract.MCPConnectionFactCurrentReaderV2, availability runtimeports.MCPConnectionAvailabilityNeutralCurrentReaderV1, sessions MCPDiscoveryPageSessionReaderV1, entries *InMemoryMCPDiscoveryPagePhysicalRepositoryV1, clock func() time.Time) (*OfficialSDKDiscoveryPageExecutorV1, error) {
	if nilLikeOfficialSDKConnectV1(commands) || nilLikeOfficialSDKConnectV1(connections) || nilLikeOfficialSDKConnectV1(availability) || nilLikeOfficialSDKConnectV1(sessions) || nilLikeOfficialSDKConnectV1(entries) || clock == nil {
		return nil, invalid("official MCP SDK Discovery Page executor dependencies are incomplete")
	}
	return &OfficialSDKDiscoveryPageExecutorV1{commands: commands, connections: connections, availability: availability, sessions: sessions, entries: entries, clock: clock}, nil
}

func (e *OfficialSDKDiscoveryPageExecutorV1) DiscoverControlledMCPPageV1(ctx context.Context, authorization runtimeports.ControlledMCPDiscoveryPagePhysicalAuthorizationV1) (runtimeports.ControlledOperationProviderAdmissionReceiptRefV2, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	if e == nil || e.clock == nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "official MCP SDK Discovery Page executor is unavailable")
	}
	now := e.clock()
	if err := authorization.ValidateCurrent(now); err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	commandRef := toolcontract.ObjectRef{ID: authorization.DomainCommand.ID, Revision: authorization.DomainCommand.Revision, Digest: authorization.DomainCommand.Digest}
	command, err := e.commands.InspectMCPDiscoveryPageCommandV1(ctx, commandRef)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	availability, err := e.availability.InspectCurrentMCPConnectionAvailabilityNeutralV1(ctx, command.Availability)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	connection, err := e.connections.InspectCurrentMCPConnectionFactV2(ctx, command.Connection)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	session, err := e.sessions.InspectMCPDiscoveryPageSessionV1(ctx, connection.ProtocolReceipt)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	actual := e.clock()
	if actual.IsZero() || actual.Before(now) || authorization.ValidateCurrent(actual) != nil || availability.ValidateCurrent(command.Availability, actual) != nil || command.RuntimeDomainCommandRefV1() != authorization.DomainCommand || command.Namespace != authorization.Namespace || command.CursorDigest != authorization.CursorDigest || command.PageOrdinal != authorization.PageOrdinal || command.Provider != authorization.Provider || connection.Provider != authorization.Provider || connection.Ref != command.Connection || !actual.Before(time.Unix(0, command.NotAfterUnixNano)) {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page actual-point closure drifted")
	}
	entry, created, err := e.entries.beginV1(ctx, authorization, command, actual)
	if err != nil {
		return runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{}, err
	}
	if !created {
		if entry.State == MCPDiscoveryPagePhysicalObservedV1 {
			return entry.AdmissionReceipt, nil
		}
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Discovery Page was already admitted and requires exact Inspect")
	}
	capture, callErr := captureOfficialSDKDiscoveryPageV1(ctx, session, command)
	observation := capture.Observation
	observed := e.clock()
	if callErr != nil || observed.IsZero() || observed.Before(actual) {
		reason := "MCP Discovery Page returned an unknown outcome"
		if callErr != nil {
			reason = callErr.Error()
		}
		e.entries.markUnknownV1(authorization.StableKeyDigest, core.DigestBytes([]byte(reason)), nonZeroExecutionTimeV1(observed, actual))
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Discovery Page outcome requires exact Inspect")
	}
	receipt, err := toolcontract.SealMCPDiscoveryPageProtocolReceiptV1(toolcontract.MCPDiscoveryPageProtocolReceiptV1{Command: command.Ref, StableKeyDigest: authorization.StableKeyDigest, AdmissionReceipt: entry.AdmissionReceipt, Namespace: command.Namespace, CursorDigest: command.CursorDigest, PageOrdinal: command.PageOrdinal, ResponsePageDigest: observation.Digest, NextCursor: observation.NextCursor, ItemCount: uint32(len(observation.Tools) + len(observation.Resources) + len(observation.Prompts)), ObservedUnixNano: observed.UnixNano()})
	if err != nil {
		e.entries.markUnknownV1(authorization.StableKeyDigest, core.DigestBytes([]byte("MCP Discovery Page Receipt seal failed")), observed)
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Discovery Page Receipt persistence requires exact Inspect")
	}
	toolMaterials, err := sealMCPToolDiscoveryMaterialsV1(command, capture.ToolObjects)
	if err != nil {
		e.entries.markUnknownV1(authorization.StableKeyDigest, core.DigestBytes([]byte("MCP Tool Discovery Material seal failed")), observed)
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Tool Discovery Material persistence requires exact Inspect")
	}
	resourceMaterials, err := sealMCPResourceDiscoveryMaterialsV1(command, capture.ResourceObjects)
	if err != nil {
		e.entries.markUnknownV1(authorization.StableKeyDigest, core.DigestBytes([]byte("MCP Resource Discovery Material seal failed")), observed)
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Resource Discovery Material persistence requires exact Inspect")
	}
	promptMaterials, err := sealMCPPromptDiscoveryMaterialsV1(command, capture.PromptObjects)
	if err != nil {
		e.entries.markUnknownV1(authorization.StableKeyDigest, core.DigestBytes([]byte("MCP Prompt Discovery Material seal failed")), observed)
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Prompt Discovery Material persistence requires exact Inspect")
	}
	if err = e.entries.completeV1(ctx, authorization.StableKeyDigest, receipt, observation, toolMaterials, resourceMaterials, promptMaterials, observed); err != nil {
		e.entries.markUnknownV1(authorization.StableKeyDigest, core.DigestBytes([]byte(err.Error())), observed)
		return entry.AdmissionReceipt, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "MCP Discovery Page Receipt persistence requires exact Inspect")
	}
	return entry.AdmissionReceipt, nil
}

type mcpToolDiscoveryObjectCaptureV1 struct {
	Observation     toolcontract.MCPToolObservationV2
	CanonicalObject json.RawMessage
}

type mcpResourceDiscoveryObjectCaptureV1 struct {
	Observation     toolcontract.MCPResourceObservationV2
	CanonicalObject json.RawMessage
}

type mcpPromptDiscoveryObjectCaptureV1 struct {
	Observation     toolcontract.MCPPromptObservationV2
	CanonicalObject json.RawMessage
}

type mcpDiscoveryPageCaptureV1 struct {
	Observation     MCPDiscoveryPageObservationV1
	ToolObjects     []mcpToolDiscoveryObjectCaptureV1
	ResourceObjects []mcpResourceDiscoveryObjectCaptureV1
	PromptObjects   []mcpPromptDiscoveryObjectCaptureV1
}

func callOfficialSDKDiscoveryPageV1(ctx context.Context, session OfficialSDKDiscoveryPageSessionV1, command toolcontract.MCPDiscoveryPageCommandV1) (MCPDiscoveryPageObservationV1, error) {
	capture, err := captureOfficialSDKDiscoveryPageV1(ctx, session, command)
	return capture.Observation, err
}

func captureOfficialSDKDiscoveryPageV1(ctx context.Context, session OfficialSDKDiscoveryPageSessionV1, command toolcontract.MCPDiscoveryPageCommandV1) (mcpDiscoveryPageCaptureV1, error) {
	value := mcpDiscoveryPageCaptureV1{Observation: MCPDiscoveryPageObservationV1{Namespace: command.Namespace}}
	cursor := string(command.Cursor)
	switch command.Namespace {
	case runtimeports.MCPDiscoveryPageToolsNamespaceV1:
		page, err := session.ListTools(ctx, &officialmcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return value, err
		}
		if page == nil {
			return value, invalid("official MCP SDK returned a nil Tools page")
		}
		for _, item := range page.Tools {
			observation, err := toolObservationFromOfficialSDKV1(item)
			if err != nil {
				return value, err
			}
			object, err := canonicalOfficialSDKValueV1(item)
			if err != nil {
				return value, err
			}
			value.ToolObjects = append(value.ToolObjects, mcpToolDiscoveryObjectCaptureV1{Observation: observation, CanonicalObject: object})
		}
		sort.Slice(value.ToolObjects, func(i, j int) bool {
			return value.ToolObjects[i].Observation.Name < value.ToolObjects[j].Observation.Name
		})
		for index, item := range value.ToolObjects {
			if index > 0 && value.ToolObjects[index-1].Observation.Name >= item.Observation.Name {
				return value, invalid("official MCP SDK Tools page names are not unique")
			}
			value.Observation.Tools = append(value.Observation.Tools, item.Observation)
		}
		value.Observation.NextCursor = []byte(page.NextCursor)
	case runtimeports.MCPDiscoveryPageResourcesNamespaceV1:
		page, err := session.ListResources(ctx, &officialmcp.ListResourcesParams{Cursor: cursor})
		if err != nil {
			return value, err
		}
		if page == nil {
			return value, invalid("official MCP SDK returned a nil Resources page")
		}
		for _, item := range page.Resources {
			observation, err := resourceObservationFromOfficialSDKV1(item)
			if err != nil {
				return value, err
			}
			object, err := canonicalOfficialSDKValueV1(item)
			if err != nil {
				return value, err
			}
			value.ResourceObjects = append(value.ResourceObjects, mcpResourceDiscoveryObjectCaptureV1{Observation: observation, CanonicalObject: object})
		}
		sort.Slice(value.ResourceObjects, func(i, j int) bool {
			return value.ResourceObjects[i].Observation.URI < value.ResourceObjects[j].Observation.URI
		})
		for index, item := range value.ResourceObjects {
			if index > 0 && value.ResourceObjects[index-1].Observation.URI >= item.Observation.URI {
				return value, invalid("official MCP SDK Resources page URIs are not unique")
			}
			value.Observation.Resources = append(value.Observation.Resources, item.Observation)
		}
		value.Observation.NextCursor = []byte(page.NextCursor)
	case runtimeports.MCPDiscoveryPagePromptsNamespaceV1:
		page, err := session.ListPrompts(ctx, &officialmcp.ListPromptsParams{Cursor: cursor})
		if err != nil {
			return value, err
		}
		if page == nil {
			return value, invalid("official MCP SDK returned a nil Prompts page")
		}
		for _, item := range page.Prompts {
			observation, err := promptObservationFromOfficialSDKV1(item)
			if err != nil {
				return value, err
			}
			object, err := canonicalOfficialSDKValueV1(item)
			if err != nil {
				return value, err
			}
			value.PromptObjects = append(value.PromptObjects, mcpPromptDiscoveryObjectCaptureV1{Observation: observation, CanonicalObject: object})
		}
		sort.Slice(value.PromptObjects, func(i, j int) bool {
			return value.PromptObjects[i].Observation.Name < value.PromptObjects[j].Observation.Name
		})
		for index, item := range value.PromptObjects {
			if index > 0 && value.PromptObjects[index-1].Observation.Name >= item.Observation.Name {
				return value, invalid("official MCP SDK Prompts page names are not unique")
			}
			value.Observation.Prompts = append(value.Observation.Prompts, item.Observation)
		}
		value.Observation.NextCursor = []byte(page.NextCursor)
	default:
		return value, invalid("unsupported MCP Discovery Page namespace")
	}
	if len(value.Observation.NextCursor) > toolcontract.MaxMCPDiscoveryCursorBytesV1 {
		return value, invalid("MCP Discovery next cursor exceeds limit")
	}
	value.Observation.Digest, _ = value.Observation.computeDigestV1()
	return value, nil
}

func sealMCPToolDiscoveryMaterialsV1(command toolcontract.MCPDiscoveryPageCommandV1, objects []mcpToolDiscoveryObjectCaptureV1) ([]toolcontract.MCPToolDiscoveryMaterialV1, error) {
	if command.Namespace != runtimeports.MCPDiscoveryPageToolsNamespaceV1 {
		if len(objects) != 0 {
			return nil, invalid("non-Tool MCP Discovery Page returned Tool material")
		}
		return nil, nil
	}
	materials := make([]toolcontract.MCPToolDiscoveryMaterialV1, 0, len(objects))
	for _, object := range objects {
		material, err := toolcontract.SealMCPToolDiscoveryMaterialV1(toolcontract.MCPToolDiscoveryMaterialV1{Command: command.Ref, Connection: command.Connection, Source: object.Observation, CanonicalObject: object.CanonicalObject})
		if err != nil {
			return nil, err
		}
		materials = append(materials, material)
	}
	return materials, nil
}

func sealMCPResourceDiscoveryMaterialsV1(command toolcontract.MCPDiscoveryPageCommandV1, objects []mcpResourceDiscoveryObjectCaptureV1) ([]toolcontract.MCPResourceDiscoveryMaterialV1, error) {
	if command.Namespace != runtimeports.MCPDiscoveryPageResourcesNamespaceV1 {
		if len(objects) != 0 {
			return nil, invalid("non-Resource MCP Discovery Page returned Resource material")
		}
		return nil, nil
	}
	materials := make([]toolcontract.MCPResourceDiscoveryMaterialV1, 0, len(objects))
	for _, object := range objects {
		material, err := toolcontract.SealMCPResourceDiscoveryMaterialV1(toolcontract.MCPResourceDiscoveryMaterialV1{Command: command.Ref, Connection: command.Connection, Source: object.Observation, CanonicalObject: object.CanonicalObject})
		if err != nil {
			return nil, err
		}
		materials = append(materials, material)
	}
	return materials, nil
}

func sealMCPPromptDiscoveryMaterialsV1(command toolcontract.MCPDiscoveryPageCommandV1, objects []mcpPromptDiscoveryObjectCaptureV1) ([]toolcontract.MCPPromptDiscoveryMaterialV1, error) {
	if command.Namespace != runtimeports.MCPDiscoveryPagePromptsNamespaceV1 {
		if len(objects) != 0 {
			return nil, invalid("non-Prompt MCP Discovery Page returned Prompt material")
		}
		return nil, nil
	}
	materials := make([]toolcontract.MCPPromptDiscoveryMaterialV1, 0, len(objects))
	for _, object := range objects {
		material, err := toolcontract.SealMCPPromptDiscoveryMaterialV1(toolcontract.MCPPromptDiscoveryMaterialV1{Command: command.Ref, Connection: command.Connection, Source: object.Observation, CanonicalObject: object.CanonicalObject})
		if err != nil {
			return nil, err
		}
		materials = append(materials, material)
	}
	return materials, nil
}

func validateMCPDiscoveryPageToolMaterialsV1(receipt toolcontract.MCPDiscoveryPageProtocolReceiptV1, connection toolcontract.MCPConnectionFactRefV2, observation MCPDiscoveryPageObservationV1, materials []toolcontract.MCPToolDiscoveryMaterialV1) error {
	if observation.Namespace != runtimeports.MCPDiscoveryPageToolsNamespaceV1 {
		if len(materials) != 0 {
			return invalid("non-Tool MCP Discovery Page contains Tool material")
		}
		return nil
	}
	if len(materials) != len(observation.Tools) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Tool Discovery material set is incomplete")
	}
	for index, material := range materials {
		if material.Validate() != nil || material.Command != receipt.Command || material.Connection != connection || material.Source != observation.Tools[index] {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Tool Discovery material set drifted")
		}
	}
	return nil
}

func validateMCPDiscoveryPageResourceMaterialsV1(receipt toolcontract.MCPDiscoveryPageProtocolReceiptV1, connection toolcontract.MCPConnectionFactRefV2, observation MCPDiscoveryPageObservationV1, materials []toolcontract.MCPResourceDiscoveryMaterialV1) error {
	if observation.Namespace != runtimeports.MCPDiscoveryPageResourcesNamespaceV1 {
		if len(materials) != 0 {
			return invalid("non-Resource MCP Discovery Page contains Resource material")
		}
		return nil
	}
	if len(materials) != len(observation.Resources) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Resource Discovery material set is incomplete")
	}
	for index, material := range materials {
		if material.Validate() != nil || material.Command != receipt.Command || material.Connection != connection || material.Source != observation.Resources[index] {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Resource Discovery material set drifted")
		}
	}
	return nil
}

func validateMCPDiscoveryPagePromptMaterialsV1(receipt toolcontract.MCPDiscoveryPageProtocolReceiptV1, connection toolcontract.MCPConnectionFactRefV2, observation MCPDiscoveryPageObservationV1, materials []toolcontract.MCPPromptDiscoveryMaterialV1) error {
	if observation.Namespace != runtimeports.MCPDiscoveryPagePromptsNamespaceV1 {
		if len(materials) != 0 {
			return invalid("non-Prompt MCP Discovery Page contains Prompt material")
		}
		return nil
	}
	if len(materials) != len(observation.Prompts) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Prompt Discovery material set is incomplete")
	}
	for index, material := range materials {
		if material.Validate() != nil || material.Command != receipt.Command || material.Connection != connection || material.Source != observation.Prompts[index] {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "MCP Prompt Discovery material set drifted")
		}
	}
	return nil
}

func cloneMCPToolDiscoveryMaterialsV1(values []toolcontract.MCPToolDiscoveryMaterialV1) []toolcontract.MCPToolDiscoveryMaterialV1 {
	result := make([]toolcontract.MCPToolDiscoveryMaterialV1, len(values))
	for index := range values {
		result[index] = values[index].Clone()
	}
	return result
}

func cloneMCPResourceDiscoveryMaterialsV1(values []toolcontract.MCPResourceDiscoveryMaterialV1) []toolcontract.MCPResourceDiscoveryMaterialV1 {
	result := make([]toolcontract.MCPResourceDiscoveryMaterialV1, len(values))
	for index := range values {
		result[index] = values[index].Clone()
	}
	return result
}

func cloneMCPPromptDiscoveryMaterialsV1(values []toolcontract.MCPPromptDiscoveryMaterialV1) []toolcontract.MCPPromptDiscoveryMaterialV1 {
	result := make([]toolcontract.MCPPromptDiscoveryMaterialV1, len(values))
	for index := range values {
		result[index] = values[index].Clone()
	}
	return result
}

func cloneMCPDiscoveryPagePhysicalEntryV1(value MCPDiscoveryPagePhysicalEntryV1) MCPDiscoveryPagePhysicalEntryV1 {
	data, _ := json.Marshal(value)
	var clone MCPDiscoveryPagePhysicalEntryV1
	_ = json.Unmarshal(data, &clone)
	return clone
}

var _ toolcontract.MCPDiscoveryPageProtocolReceiptExactReaderV1 = (*InMemoryMCPDiscoveryPagePhysicalRepositoryV1)(nil)
var _ toolcontract.MCPToolDiscoveryMaterialExactReaderV1 = (*InMemoryMCPDiscoveryPagePhysicalRepositoryV1)(nil)
var _ toolcontract.MCPResourceDiscoveryMaterialExactReaderV1 = (*InMemoryMCPDiscoveryPagePhysicalRepositoryV1)(nil)
var _ toolcontract.MCPPromptDiscoveryMaterialExactReaderV1 = (*InMemoryMCPDiscoveryPagePhysicalRepositoryV1)(nil)
var _ toolcontract.MCPDiscoveryPageToolMaterialSetExactReaderV1 = (*InMemoryMCPDiscoveryPagePhysicalRepositoryV1)(nil)
var _ toolcontract.MCPDiscoveryPageResourceMaterialSetExactReaderV1 = (*InMemoryMCPDiscoveryPagePhysicalRepositoryV1)(nil)
var _ toolcontract.MCPDiscoveryPagePromptMaterialSetExactReaderV1 = (*InMemoryMCPDiscoveryPagePhysicalRepositoryV1)(nil)
