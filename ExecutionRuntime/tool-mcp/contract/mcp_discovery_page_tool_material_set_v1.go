package contract

import (
	"context"
	"sort"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const MCPDiscoveryPageToolMaterialSetContractVersionV1 = "praxis.tool-mcp.mcp-discovery-page-tool-material-set/v1"

type MCPDiscoveryPageToolMaterialEntryV1 struct {
	Source   MCPToolObservationV2          `json:"source"`
	Material MCPToolDiscoveryMaterialRefV1 `json:"material"`
}

func (e MCPDiscoveryPageToolMaterialEntryV1) Validate() error {
	if e.Source.Validate() != nil || e.Material.Validate() != nil {
		return invalid("MCP Discovery Page Tool Material entry is invalid")
	}
	return nil
}

// MCPDiscoveryPageToolMaterialSetV1 is an immutable, derived exact projection
// from one observed Tools page. It exposes no latest/name lookup and grants no
// semantic mapping or execution authority.
type MCPDiscoveryPageToolMaterialSetV1 struct {
	ContractVersion    string                                `json:"contract_version"`
	Ref                ObjectRef                             `json:"ref"`
	Receipt            ObjectRef                             `json:"receipt"`
	Command            ObjectRef                             `json:"command"`
	Connection         MCPConnectionFactRefV2                `json:"connection"`
	ResponsePageDigest core.Digest                           `json:"response_page_digest"`
	Entries            []MCPDiscoveryPageToolMaterialEntryV1 `json:"entries"`
}

func DeriveMCPDiscoveryPageToolMaterialSetIDV1(receipt ObjectRef) (string, error) {
	if receipt.Validate() != nil {
		return "", invalid("MCP Discovery Page Tool Material Set receipt is invalid")
	}
	return StableID("mcp-page-materials", receipt.ID, string(receipt.Digest))
}

func (s MCPDiscoveryPageToolMaterialSetV1) Validate() error {
	if s.ContractVersion != MCPDiscoveryPageToolMaterialSetContractVersionV1 || s.Ref.Validate() != nil || s.Receipt.Validate() != nil || s.Command.Validate() != nil || s.Connection.Validate() != nil || s.ResponsePageDigest.Validate() != nil || len(s.Entries) > MaxMCPDiscoveryToolsV2 {
		return invalid("MCP Discovery Page Tool Material Set is invalid")
	}
	for index, entry := range s.Entries {
		if entry.Validate() != nil || index > 0 && s.Entries[index-1].Source.Name >= entry.Source.Name {
			return invalid("MCP Discovery Page Tool Material Set entries are invalid or not unique")
		}
	}
	id, err := DeriveMCPDiscoveryPageToolMaterialSetIDV1(s.Receipt)
	if err != nil || s.Ref.ID != id || s.Ref.Revision != 1 {
		return conflict("MCP Discovery Page Tool Material Set identity drifted")
	}
	digest, err := s.ComputeDigest()
	if err != nil || digest != s.Ref.Digest {
		return conflict("MCP Discovery Page Tool Material Set digest drifted")
	}
	return nil
}

func (s MCPDiscoveryPageToolMaterialSetV1) ComputeDigest() (core.Digest, error) {
	if s.Receipt.Validate() != nil || s.Command.Validate() != nil || s.Connection.Validate() != nil || s.ResponsePageDigest.Validate() != nil || len(s.Entries) > MaxMCPDiscoveryToolsV2 {
		return "", invalid("MCP Discovery Page Tool Material Set source is invalid")
	}
	s = CloneMCPDiscoveryPageToolMaterialSetV1(s)
	s.Ref.Digest = ""
	return Seal("praxis.tool-mcp.mcp-discovery-page-tool-material-set", MCPDiscoveryPageToolMaterialSetContractVersionV1, "MCPDiscoveryPageToolMaterialSetV1", s)
}

func SealMCPDiscoveryPageToolMaterialSetV1(s MCPDiscoveryPageToolMaterialSetV1) (MCPDiscoveryPageToolMaterialSetV1, error) {
	s = CloneMCPDiscoveryPageToolMaterialSetV1(s)
	s.ContractVersion = MCPDiscoveryPageToolMaterialSetContractVersionV1
	sort.Slice(s.Entries, func(i, j int) bool { return s.Entries[i].Source.Name < s.Entries[j].Source.Name })
	id, err := DeriveMCPDiscoveryPageToolMaterialSetIDV1(s.Receipt)
	if err != nil {
		return MCPDiscoveryPageToolMaterialSetV1{}, err
	}
	if s.Ref.ID != "" && s.Ref.ID != id {
		return MCPDiscoveryPageToolMaterialSetV1{}, conflict("supplied MCP Discovery Page Tool Material Set ID drifted")
	}
	s.Ref = ObjectRef{ID: id, Revision: 1}
	digest, err := s.ComputeDigest()
	if err != nil {
		return MCPDiscoveryPageToolMaterialSetV1{}, err
	}
	s.Ref.Digest = digest
	return s, s.Validate()
}

func CloneMCPDiscoveryPageToolMaterialSetV1(s MCPDiscoveryPageToolMaterialSetV1) MCPDiscoveryPageToolMaterialSetV1 {
	s.Entries = append([]MCPDiscoveryPageToolMaterialEntryV1(nil), s.Entries...)
	return s
}

type MCPDiscoveryPageToolMaterialSetExactReaderV1 interface {
	InspectMCPDiscoveryPageToolMaterialSetV1(context.Context, ObjectRef) (MCPDiscoveryPageToolMaterialSetV1, error)
}
