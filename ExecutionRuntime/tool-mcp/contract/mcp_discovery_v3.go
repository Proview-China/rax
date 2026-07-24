package contract

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const MCPDiscoveryContractVersionV3 = "praxis.tool-mcp.mcp-discovery/v3"

// MCPDiscoveryPageProvenanceV3 binds one governed page to the immutable
// protocol receipt, Tool-owned settlement and the corresponding material set.
// MaterialSet is interpreted only under the closed Namespace discriminator.
type MCPDiscoveryPageProvenanceV3 struct {
	Namespace          runtimeports.NamespacedNameV2 `json:"namespace"`
	PageOrdinal        uint32                        `json:"page_ordinal"`
	Command            ObjectRef                     `json:"command"`
	ProtocolReceipt    ObjectRef                     `json:"protocol_receipt"`
	ApplySettlement    ObjectRef                     `json:"apply_settlement"`
	ResponsePageDigest core.Digest                   `json:"response_page_digest"`
	MaterialSet        ObjectRef                     `json:"material_set"`
}

func (p MCPDiscoveryPageProvenanceV3) Validate() error {
	if !runtimeports.IsMCPDiscoveryPageNamespaceV1(p.Namespace) || p.Command.Validate() != nil || p.ProtocolReceipt.Validate() != nil || p.ApplySettlement.Validate() != nil || p.ResponsePageDigest.Validate() != nil || p.MaterialSet.Validate() != nil {
		return invalid("MCP Discovery Page provenance V3 is invalid")
	}
	return nil
}

type MCPToolMaterialProvenanceV3 struct {
	Source      MCPToolObservationV2          `json:"source"`
	PageReceipt ObjectRef                     `json:"page_receipt"`
	Material    MCPToolDiscoveryMaterialRefV1 `json:"material"`
}

func (p MCPToolMaterialProvenanceV3) Validate() error {
	if p.Source.Validate() != nil || p.PageReceipt.Validate() != nil || p.Material.Validate() != nil {
		return invalid("MCP Tool Material provenance V3 is invalid")
	}
	return nil
}

type MCPResourceMaterialProvenanceV3 struct {
	Source      MCPResourceObservationV2          `json:"source"`
	PageReceipt ObjectRef                         `json:"page_receipt"`
	Material    MCPResourceDiscoveryMaterialRefV1 `json:"material"`
}

func (p MCPResourceMaterialProvenanceV3) Validate() error {
	if p.Source.Validate() != nil || p.PageReceipt.Validate() != nil || p.Material.Validate() != nil {
		return invalid("MCP Resource Material provenance V3 is invalid")
	}
	return nil
}

type MCPPromptMaterialProvenanceV3 struct {
	Source      MCPPromptObservationV2          `json:"source"`
	PageReceipt ObjectRef                       `json:"page_receipt"`
	Material    MCPPromptDiscoveryMaterialRefV1 `json:"material"`
}

func (p MCPPromptMaterialProvenanceV3) Validate() error {
	if p.Source.Validate() != nil || p.PageReceipt.Validate() != nil || p.Material.Validate() != nil {
		return invalid("MCP Prompt Material provenance V3 is invalid")
	}
	return nil
}

// MCPCapabilitySnapshotV3 is the first snapshot whose canonical body includes
// the complete governed page and raw discovery material provenance closure.
// It remains an observation and grants no mapping, admission or execution
// authority.
type MCPCapabilitySnapshotV3 struct {
	ContractVersion          string                            `json:"contract_version"`
	ID                       string                            `json:"id"`
	Revision                 core.Revision                     `json:"revision"`
	Digest                   core.Digest                       `json:"digest"`
	Server                   ObjectRef                         `json:"server"`
	Connection               ObjectRef                         `json:"connection"`
	ConnectionEpoch          core.Epoch                        `json:"connection_epoch"`
	ProtocolVersion          string                            `json:"protocol_version"`
	ServerInfoDigest         core.Digest                       `json:"server_info_digest"`
	ServerCapabilitiesDigest core.Digest                       `json:"server_capabilities_digest"`
	InstructionsDigest       core.Digest                       `json:"instructions_digest"`
	Tools                    []MCPToolObservationV2            `json:"tools"`
	Resources                []MCPResourceObservationV2        `json:"resources"`
	Prompts                  []MCPPromptObservationV2          `json:"prompts"`
	Pages                    []MCPDiscoveryPageProvenanceV3    `json:"pages"`
	ToolMaterials            []MCPToolMaterialProvenanceV3     `json:"tool_materials"`
	ResourceMaterials        []MCPResourceMaterialProvenanceV3 `json:"resource_materials"`
	PromptMaterials          []MCPPromptMaterialProvenanceV3   `json:"prompt_materials"`
	SourceDigest             core.Digest                       `json:"source_digest"`
	ValidationDigest         core.Digest                       `json:"validation_digest"`
	Conformance              runtimeports.NamespacedNameV2     `json:"conformance"`
	Residuals                []Residual                        `json:"residuals,omitempty"`
	CreatedUnixNano          int64                             `json:"created_unix_nano"`
	ExpiresUnixNano          int64                             `json:"expires_unix_nano"`
}

func (s MCPCapabilitySnapshotV3) validateShape() error {
	if s.ContractVersion != MCPDiscoveryContractVersionV3 || ValidateStableID(s.ID) != nil || s.Revision == 0 || s.Server.Validate() != nil || s.Connection.Validate() != nil || s.ConnectionEpoch == 0 || !validProtocolVersion(s.ProtocolVersion) || s.ProtocolVersion > MCPStableProtocolVersion || s.ServerInfoDigest.Validate() != nil || s.ServerCapabilitiesDigest.Validate() != nil || s.InstructionsDigest.Validate() != nil || s.SourceDigest.Validate() != nil || s.ValidationDigest.Validate() != nil || runtimeports.ValidateNamespacedNameV2(s.Conformance) != nil || s.CreatedUnixNano <= 0 || s.ExpiresUnixNano <= s.CreatedUnixNano {
		return invalid("MCP Capability Snapshot V3 is incomplete")
	}
	if len(s.Tools) > MaxMCPDiscoveryToolsV2 || len(s.Resources) > MaxMCPDiscoveryResourcesV2 || len(s.Prompts) > MaxMCPDiscoveryPromptsV2 || len(s.Pages) > MaxMCPDiscoveryToolsV2+MaxMCPDiscoveryResourcesV2+MaxMCPDiscoveryPromptsV2 || len(s.ToolMaterials) != len(s.Tools) || len(s.ResourceMaterials) != len(s.Resources) || len(s.PromptMaterials) != len(s.Prompts) || len(s.Residuals) > MaxResiduals {
		return invalid("MCP Capability Snapshot V3 exceeds limits or has incomplete provenance")
	}
	pageReceipts := make(map[ObjectRef]runtimeports.NamespacedNameV2, len(s.Pages))
	lastOrdinal := map[runtimeports.NamespacedNameV2]uint32{}
	seenNamespace := map[runtimeports.NamespacedNameV2]bool{}
	for i, page := range s.Pages {
		if page.Validate() != nil || i > 0 && (s.Pages[i-1].Namespace > page.Namespace || s.Pages[i-1].Namespace == page.Namespace && s.Pages[i-1].PageOrdinal >= page.PageOrdinal) {
			return invalid("MCP Capability Snapshot V3 Pages are invalid or not unique")
		}
		if !seenNamespace[page.Namespace] {
			if page.PageOrdinal != 0 {
				return conflict("MCP Capability Snapshot V3 Page ordinal does not start at zero")
			}
			seenNamespace[page.Namespace] = true
		} else if page.PageOrdinal != lastOrdinal[page.Namespace]+1 {
			return conflict("MCP Capability Snapshot V3 Page ordinal is discontinuous")
		}
		lastOrdinal[page.Namespace] = page.PageOrdinal
		if _, exists := pageReceipts[page.ProtocolReceipt]; exists {
			return conflict("MCP Capability Snapshot V3 repeats a Page Receipt")
		}
		pageReceipts[page.ProtocolReceipt] = page.Namespace
	}
	for i, tool := range s.Tools {
		if tool.Validate() != nil || i > 0 && s.Tools[i-1].Name >= tool.Name {
			return invalid("MCP Capability Snapshot V3 Tools are invalid or not unique")
		}
		material := s.ToolMaterials[i]
		if material.Validate() != nil || material.Source != tool || pageReceipts[material.PageReceipt] != runtimeports.MCPDiscoveryPageToolsNamespaceV1 || i > 0 && s.ToolMaterials[i-1].Source.Name >= material.Source.Name {
			return conflict("MCP Capability Snapshot V3 Tool provenance drifted")
		}
	}
	for i, resource := range s.Resources {
		if resource.Validate() != nil || i > 0 && s.Resources[i-1].URI >= resource.URI {
			return invalid("MCP Capability Snapshot V3 Resources are invalid or not unique")
		}
		material := s.ResourceMaterials[i]
		if material.Validate() != nil || material.Source != resource || pageReceipts[material.PageReceipt] != runtimeports.MCPDiscoveryPageResourcesNamespaceV1 || i > 0 && s.ResourceMaterials[i-1].Source.URI >= material.Source.URI {
			return conflict("MCP Capability Snapshot V3 Resource provenance drifted")
		}
	}
	for i, prompt := range s.Prompts {
		if prompt.Validate() != nil || i > 0 && s.Prompts[i-1].Name >= prompt.Name {
			return invalid("MCP Capability Snapshot V3 Prompts are invalid or not unique")
		}
		material := s.PromptMaterials[i]
		if material.Validate() != nil || material.Source != prompt || pageReceipts[material.PageReceipt] != runtimeports.MCPDiscoveryPagePromptsNamespaceV1 || i > 0 && s.PromptMaterials[i-1].Source.Name >= material.Source.Name {
			return conflict("MCP Capability Snapshot V3 Prompt provenance drifted")
		}
	}
	for _, residual := range s.Residuals {
		if residual.Validate() != nil {
			return invalid("MCP Capability Snapshot V3 Residual is invalid")
		}
	}
	return nil
}

func (s MCPCapabilitySnapshotV3) Validate() error {
	if err := s.validateShape(); err != nil {
		return err
	}
	id, err := DeriveMCPCapabilitySnapshotIDV3(s.Server, s.Connection, s.ConnectionEpoch)
	if err != nil || id != s.ID {
		return conflict("MCP Capability Snapshot V3 ID drifted")
	}
	validation, err := s.ComputeValidationDigest()
	if err != nil || validation != s.ValidationDigest {
		return conflict("MCP Capability Snapshot V3 validation digest drifted")
	}
	digest, err := s.ComputeDigest()
	if err != nil || digest != s.Digest {
		return conflict("MCP Capability Snapshot V3 digest drifted")
	}
	return nil
}

func (s MCPCapabilitySnapshotV3) ValidateCurrent(now time.Time) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < s.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Capability Snapshot V3 clock regressed")
	}
	if !now.Before(time.Unix(0, s.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "MCP Capability Snapshot V3 expired")
	}
	return nil
}

func (s MCPCapabilitySnapshotV3) ComputeValidationDigest() (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.tool-mcp.mcp", MCPDiscoveryContractVersionV3, "MCPCapabilitySnapshotValidationV3", struct {
		Tools             []MCPToolObservationV2            `json:"tools"`
		Resources         []MCPResourceObservationV2        `json:"resources"`
		Prompts           []MCPPromptObservationV2          `json:"prompts"`
		Pages             []MCPDiscoveryPageProvenanceV3    `json:"pages"`
		ToolMaterials     []MCPToolMaterialProvenanceV3     `json:"tool_materials"`
		ResourceMaterials []MCPResourceMaterialProvenanceV3 `json:"resource_materials"`
		PromptMaterials   []MCPPromptMaterialProvenanceV3   `json:"prompt_materials"`
	}{s.Tools, s.Resources, s.Prompts, s.Pages, s.ToolMaterials, s.ResourceMaterials, s.PromptMaterials})
}

func (s MCPCapabilitySnapshotV3) ComputeDigest() (core.Digest, error) {
	if err := s.validateShape(); err != nil {
		return "", err
	}
	s = CloneMCPCapabilitySnapshotV3(s)
	s.Digest = ""
	return Seal("praxis.tool-mcp.mcp", MCPDiscoveryContractVersionV3, "MCPCapabilitySnapshotV3", s)
}

func SealMCPCapabilitySnapshotV3(s MCPCapabilitySnapshotV3) (MCPCapabilitySnapshotV3, error) {
	s = CloneMCPCapabilitySnapshotV3(s)
	s.ContractVersion = MCPDiscoveryContractVersionV3
	sort.Slice(s.Tools, func(i, j int) bool { return s.Tools[i].Name < s.Tools[j].Name })
	sort.Slice(s.Resources, func(i, j int) bool { return s.Resources[i].URI < s.Resources[j].URI })
	sort.Slice(s.Prompts, func(i, j int) bool { return s.Prompts[i].Name < s.Prompts[j].Name })
	sort.Slice(s.Pages, func(i, j int) bool {
		if s.Pages[i].Namespace != s.Pages[j].Namespace {
			return s.Pages[i].Namespace < s.Pages[j].Namespace
		}
		return s.Pages[i].PageOrdinal < s.Pages[j].PageOrdinal
	})
	sort.Slice(s.ToolMaterials, func(i, j int) bool { return s.ToolMaterials[i].Source.Name < s.ToolMaterials[j].Source.Name })
	sort.Slice(s.ResourceMaterials, func(i, j int) bool { return s.ResourceMaterials[i].Source.URI < s.ResourceMaterials[j].Source.URI })
	sort.Slice(s.PromptMaterials, func(i, j int) bool { return s.PromptMaterials[i].Source.Name < s.PromptMaterials[j].Source.Name })
	id, err := DeriveMCPCapabilitySnapshotIDV3(s.Server, s.Connection, s.ConnectionEpoch)
	if err != nil {
		return MCPCapabilitySnapshotV3{}, err
	}
	if s.ID != "" && s.ID != id {
		return MCPCapabilitySnapshotV3{}, conflict("supplied MCP Capability Snapshot V3 ID drifted")
	}
	s.ID = id
	validation, err := s.ComputeValidationDigest()
	if err != nil {
		return MCPCapabilitySnapshotV3{}, err
	}
	if s.ValidationDigest != "" && s.ValidationDigest != validation {
		return MCPCapabilitySnapshotV3{}, conflict("supplied MCP Capability Snapshot V3 validation digest drifted")
	}
	s.ValidationDigest = validation
	provided := s.Digest
	s.Digest = ""
	digest, err := s.ComputeDigest()
	if err != nil {
		return MCPCapabilitySnapshotV3{}, err
	}
	if provided != "" && provided != digest {
		return MCPCapabilitySnapshotV3{}, conflict("supplied MCP Capability Snapshot V3 digest drifted")
	}
	s.Digest = digest
	return s, s.Validate()
}

func DeriveMCPCapabilitySnapshotIDV3(server, connection ObjectRef, epoch core.Epoch) (string, error) {
	if server.Validate() != nil || connection.Validate() != nil || epoch == 0 {
		return "", invalid("MCP Capability Snapshot V3 identity inputs are invalid")
	}
	digest, err := core.CanonicalJSONDigest("praxis.tool-mcp.mcp", MCPDiscoveryContractVersionV3, "MCPCapabilitySnapshotIdentityV3", struct {
		Server     ObjectRef  `json:"server"`
		Connection ObjectRef  `json:"connection"`
		Epoch      core.Epoch `json:"epoch"`
	}{server, connection, epoch})
	if err != nil {
		return "", err
	}
	return "mcp-snapshot-v3-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func CloneMCPCapabilitySnapshotV3(s MCPCapabilitySnapshotV3) MCPCapabilitySnapshotV3 {
	s.Tools = append([]MCPToolObservationV2(nil), s.Tools...)
	s.Resources = append([]MCPResourceObservationV2(nil), s.Resources...)
	s.Prompts = append([]MCPPromptObservationV2(nil), s.Prompts...)
	s.Pages = append([]MCPDiscoveryPageProvenanceV3(nil), s.Pages...)
	s.ToolMaterials = append([]MCPToolMaterialProvenanceV3(nil), s.ToolMaterials...)
	s.ResourceMaterials = append([]MCPResourceMaterialProvenanceV3(nil), s.ResourceMaterials...)
	s.PromptMaterials = append([]MCPPromptMaterialProvenanceV3(nil), s.PromptMaterials...)
	s.Residuals = append([]Residual(nil), s.Residuals...)
	return s
}

func (s MCPCapabilitySnapshotV3) ObjectRef() ObjectRef {
	return ObjectRef{ID: s.ID, Revision: s.Revision, Digest: s.Digest}
}

type MCPCapabilitySnapshotExactReaderV3 interface {
	InspectMCPCapabilitySnapshotV3(context.Context, ObjectRef) (MCPCapabilitySnapshotV3, error)
}

type MCPCapabilitySnapshotCurrentReaderV3 interface {
	InspectCurrentMCPCapabilitySnapshotV3(context.Context, string) (MCPCapabilitySnapshotV3, error)
}
