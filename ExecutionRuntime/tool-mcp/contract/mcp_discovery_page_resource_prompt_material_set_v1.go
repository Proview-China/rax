package contract

import (
	"context"
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	MCPDiscoveryPageResourceMaterialSetContractVersionV1 = "praxis.tool-mcp.mcp-discovery-page-resource-material-set/v1"
	MCPDiscoveryPagePromptMaterialSetContractVersionV1   = "praxis.tool-mcp.mcp-discovery-page-prompt-material-set/v1"
)

type MCPDiscoveryPageResourceMaterialEntryV1 struct {
	Source   MCPResourceObservationV2          `json:"source"`
	Material MCPResourceDiscoveryMaterialRefV1 `json:"material"`
}

func (e MCPDiscoveryPageResourceMaterialEntryV1) Validate() error {
	if e.Source.Validate() != nil || e.Material.Validate() != nil {
		return invalid("MCP Discovery Page Resource Material entry is invalid")
	}
	return nil
}

type MCPDiscoveryPageResourceMaterialSetV1 struct {
	ContractVersion    string                                    `json:"contract_version"`
	Ref                ObjectRef                                 `json:"ref"`
	Receipt            ObjectRef                                 `json:"receipt"`
	Command            ObjectRef                                 `json:"command"`
	Connection         MCPConnectionFactRefV2                    `json:"connection"`
	ResponsePageDigest core.Digest                               `json:"response_page_digest"`
	Entries            []MCPDiscoveryPageResourceMaterialEntryV1 `json:"entries"`
}

func DeriveMCPDiscoveryPageResourceMaterialSetIDV1(receipt ObjectRef) (string, error) {
	if receipt.Validate() != nil {
		return "", invalid("MCP Discovery Page Resource Material Set receipt is invalid")
	}
	return StableID("mcp-resource-page-materials", receipt.ID, string(receipt.Digest))
}

func (s MCPDiscoveryPageResourceMaterialSetV1) Validate() error {
	if s.ContractVersion != MCPDiscoveryPageResourceMaterialSetContractVersionV1 || s.Ref.Validate() != nil || s.Receipt.Validate() != nil || s.Command.Validate() != nil || s.Connection.Validate() != nil || s.ResponsePageDigest.Validate() != nil || len(s.Entries) > MaxMCPDiscoveryResourcesV2 {
		return invalid("MCP Discovery Page Resource Material Set is invalid")
	}
	for index, entry := range s.Entries {
		if entry.Validate() != nil || index > 0 && s.Entries[index-1].Source.URI >= entry.Source.URI {
			return invalid("MCP Discovery Page Resource Material Set entries are invalid or not unique")
		}
	}
	id, err := DeriveMCPDiscoveryPageResourceMaterialSetIDV1(s.Receipt)
	if err != nil || s.Ref.ID != id || s.Ref.Revision != 1 {
		return conflict("MCP Discovery Page Resource Material Set identity drifted")
	}
	digest, err := s.ComputeDigest()
	if err != nil || digest != s.Ref.Digest {
		return conflict("MCP Discovery Page Resource Material Set digest drifted")
	}
	return nil
}

func (s MCPDiscoveryPageResourceMaterialSetV1) ComputeDigest() (core.Digest, error) {
	if s.Receipt.Validate() != nil || s.Command.Validate() != nil || s.Connection.Validate() != nil || s.ResponsePageDigest.Validate() != nil || len(s.Entries) > MaxMCPDiscoveryResourcesV2 {
		return "", invalid("MCP Discovery Page Resource Material Set source is invalid")
	}
	s = CloneMCPDiscoveryPageResourceMaterialSetV1(s)
	s.Ref.Digest = ""
	return Seal("praxis.tool-mcp.mcp-discovery-page-resource-material-set", MCPDiscoveryPageResourceMaterialSetContractVersionV1, "MCPDiscoveryPageResourceMaterialSetV1", s)
}

func SealMCPDiscoveryPageResourceMaterialSetV1(s MCPDiscoveryPageResourceMaterialSetV1) (MCPDiscoveryPageResourceMaterialSetV1, error) {
	s = CloneMCPDiscoveryPageResourceMaterialSetV1(s)
	s.ContractVersion = MCPDiscoveryPageResourceMaterialSetContractVersionV1
	sort.Slice(s.Entries, func(i, j int) bool { return s.Entries[i].Source.URI < s.Entries[j].Source.URI })
	id, err := DeriveMCPDiscoveryPageResourceMaterialSetIDV1(s.Receipt)
	if err != nil {
		return MCPDiscoveryPageResourceMaterialSetV1{}, err
	}
	if s.Ref.ID != "" && s.Ref.ID != id {
		return MCPDiscoveryPageResourceMaterialSetV1{}, conflict("supplied MCP Discovery Page Resource Material Set ID drifted")
	}
	s.Ref = ObjectRef{ID: id, Revision: 1}
	s.Ref.Digest, err = s.ComputeDigest()
	if err != nil {
		return MCPDiscoveryPageResourceMaterialSetV1{}, err
	}
	return s, s.Validate()
}

func CloneMCPDiscoveryPageResourceMaterialSetV1(s MCPDiscoveryPageResourceMaterialSetV1) MCPDiscoveryPageResourceMaterialSetV1 {
	s.Entries = append([]MCPDiscoveryPageResourceMaterialEntryV1(nil), s.Entries...)
	return s
}

type MCPDiscoveryPageResourceMaterialSetExactReaderV1 interface {
	InspectMCPDiscoveryPageResourceMaterialSetV1(context.Context, ObjectRef) (MCPDiscoveryPageResourceMaterialSetV1, error)
}

type MCPDiscoveryPagePromptMaterialEntryV1 struct {
	Source   MCPPromptObservationV2          `json:"source"`
	Material MCPPromptDiscoveryMaterialRefV1 `json:"material"`
}

func (e MCPDiscoveryPagePromptMaterialEntryV1) Validate() error {
	if e.Source.Validate() != nil || e.Material.Validate() != nil {
		return invalid("MCP Discovery Page Prompt Material entry is invalid")
	}
	return nil
}

type MCPDiscoveryPagePromptMaterialSetV1 struct {
	ContractVersion    string                                  `json:"contract_version"`
	Ref                ObjectRef                               `json:"ref"`
	Receipt            ObjectRef                               `json:"receipt"`
	Command            ObjectRef                               `json:"command"`
	Connection         MCPConnectionFactRefV2                  `json:"connection"`
	ResponsePageDigest core.Digest                             `json:"response_page_digest"`
	Entries            []MCPDiscoveryPagePromptMaterialEntryV1 `json:"entries"`
}

func DeriveMCPDiscoveryPagePromptMaterialSetIDV1(receipt ObjectRef) (string, error) {
	if receipt.Validate() != nil {
		return "", invalid("MCP Discovery Page Prompt Material Set receipt is invalid")
	}
	return StableID("mcp-prompt-page-materials", receipt.ID, string(receipt.Digest))
}

func (s MCPDiscoveryPagePromptMaterialSetV1) Validate() error {
	if s.ContractVersion != MCPDiscoveryPagePromptMaterialSetContractVersionV1 || s.Ref.Validate() != nil || s.Receipt.Validate() != nil || s.Command.Validate() != nil || s.Connection.Validate() != nil || s.ResponsePageDigest.Validate() != nil || len(s.Entries) > MaxMCPDiscoveryPromptsV2 {
		return invalid("MCP Discovery Page Prompt Material Set is invalid")
	}
	for index, entry := range s.Entries {
		if entry.Validate() != nil || index > 0 && s.Entries[index-1].Source.Name >= entry.Source.Name {
			return invalid("MCP Discovery Page Prompt Material Set entries are invalid or not unique")
		}
	}
	id, err := DeriveMCPDiscoveryPagePromptMaterialSetIDV1(s.Receipt)
	if err != nil || s.Ref.ID != id || s.Ref.Revision != 1 {
		return conflict("MCP Discovery Page Prompt Material Set identity drifted")
	}
	digest, err := s.ComputeDigest()
	if err != nil || digest != s.Ref.Digest {
		return conflict("MCP Discovery Page Prompt Material Set digest drifted")
	}
	return nil
}

func (s MCPDiscoveryPagePromptMaterialSetV1) ComputeDigest() (core.Digest, error) {
	if s.Receipt.Validate() != nil || s.Command.Validate() != nil || s.Connection.Validate() != nil || s.ResponsePageDigest.Validate() != nil || len(s.Entries) > MaxMCPDiscoveryPromptsV2 {
		return "", invalid("MCP Discovery Page Prompt Material Set source is invalid")
	}
	s = CloneMCPDiscoveryPagePromptMaterialSetV1(s)
	s.Ref.Digest = ""
	return Seal("praxis.tool-mcp.mcp-discovery-page-prompt-material-set", MCPDiscoveryPagePromptMaterialSetContractVersionV1, "MCPDiscoveryPagePromptMaterialSetV1", s)
}

func SealMCPDiscoveryPagePromptMaterialSetV1(s MCPDiscoveryPagePromptMaterialSetV1) (MCPDiscoveryPagePromptMaterialSetV1, error) {
	s = CloneMCPDiscoveryPagePromptMaterialSetV1(s)
	s.ContractVersion = MCPDiscoveryPagePromptMaterialSetContractVersionV1
	sort.Slice(s.Entries, func(i, j int) bool { return s.Entries[i].Source.Name < s.Entries[j].Source.Name })
	id, err := DeriveMCPDiscoveryPagePromptMaterialSetIDV1(s.Receipt)
	if err != nil {
		return MCPDiscoveryPagePromptMaterialSetV1{}, err
	}
	if s.Ref.ID != "" && s.Ref.ID != id {
		return MCPDiscoveryPagePromptMaterialSetV1{}, conflict("supplied MCP Discovery Page Prompt Material Set ID drifted")
	}
	s.Ref = ObjectRef{ID: id, Revision: 1}
	s.Ref.Digest, err = s.ComputeDigest()
	if err != nil {
		return MCPDiscoveryPagePromptMaterialSetV1{}, err
	}
	return s, s.Validate()
}

func CloneMCPDiscoveryPagePromptMaterialSetV1(s MCPDiscoveryPagePromptMaterialSetV1) MCPDiscoveryPagePromptMaterialSetV1 {
	s.Entries = append([]MCPDiscoveryPagePromptMaterialEntryV1(nil), s.Entries...)
	return s
}

type MCPDiscoveryPagePromptMaterialSetExactReaderV1 interface {
	InspectMCPDiscoveryPagePromptMaterialSetV1(context.Context, ObjectRef) (MCPDiscoveryPagePromptMaterialSetV1, error)
}
