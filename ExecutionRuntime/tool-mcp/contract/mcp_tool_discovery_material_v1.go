package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"unicode/utf8"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	MCPToolDiscoveryMaterialContractVersionV1 = "praxis.tool-mcp.mcp-tool-discovery-material/v1"
	MaxMCPToolDiscoveryCanonicalBytesV1       = 1 << 20
)

const mcpToolDiscoveryMaterialCanonicalDomainV1 = "praxis.tool-mcp.mcp-tool-discovery-material"

// MCPToolDiscoveryMaterialRefV1 identifies one exact Tool object observed in
// one governed MCP Discovery Page. It is an observation coordinate only: it
// grants no Capability, Authority, Review verdict, Permit, or execution right.
type MCPToolDiscoveryMaterialRefV1 struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

func (r MCPToolDiscoveryMaterialRefV1) Validate() error {
	if r.ContractVersion != MCPToolDiscoveryMaterialContractVersionV1 || ValidateStableID(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil {
		return invalid("MCP Tool Discovery Material Ref is invalid")
	}
	return nil
}

// MCPToolDiscoveryMaterialV1 preserves the canonical MCP Tool JSON returned
// by the provider. CanonicalObject is untrusted provider material; Source is a
// bounded digest projection and neither value is a Tool/Capability fact.
type MCPToolDiscoveryMaterialV1 struct {
	Ref             MCPToolDiscoveryMaterialRefV1 `json:"ref"`
	Command         ObjectRef                     `json:"command"`
	Connection      MCPConnectionFactRefV2        `json:"connection"`
	Source          MCPToolObservationV2          `json:"source"`
	CanonicalObject json.RawMessage               `json:"canonical_object"`
}

func DeriveMCPToolDiscoveryMaterialIDV1(command ObjectRef, source MCPToolObservationV2) (string, error) {
	if command.Validate() != nil || source.Validate() != nil {
		return "", invalid("MCP Tool Discovery Material identity source is invalid")
	}
	return StableID("mcp-tool-material", command.ID, strconv.FormatUint(uint64(command.Revision), 10), string(command.Digest), source.Name, string(source.ObjectDigest))
}

func (m MCPToolDiscoveryMaterialV1) Validate() error {
	if m.Ref.Validate() != nil || m.Command.Validate() != nil || m.Connection.Validate() != nil || m.Source.Validate() != nil {
		return invalid("MCP Tool Discovery Material coordinates are invalid")
	}
	id, err := DeriveMCPToolDiscoveryMaterialIDV1(m.Command, m.Source)
	if err != nil || m.Ref.ID != id {
		return conflict("MCP Tool Discovery Material ID drifted")
	}
	object, err := validateMCPToolDiscoveryCanonicalObjectV1(m.CanonicalObject, m.Source)
	if err != nil {
		return err
	}
	digest, err := m.computeDigestWithObjectV1(object)
	if err != nil || digest != m.Ref.Digest {
		return conflict("MCP Tool Discovery Material digest drifted")
	}
	return nil
}

func (m MCPToolDiscoveryMaterialV1) ComputeDigest() (core.Digest, error) {
	if m.Command.Validate() != nil || m.Connection.Validate() != nil || m.Source.Validate() != nil {
		return "", invalid("MCP Tool Discovery Material coordinates are invalid")
	}
	object, err := validateMCPToolDiscoveryCanonicalObjectV1(m.CanonicalObject, m.Source)
	if err != nil {
		return "", err
	}
	return m.computeDigestWithObjectV1(object)
}

func (m MCPToolDiscoveryMaterialV1) computeDigestWithObjectV1(object any) (core.Digest, error) {
	copy := struct {
		ContractVersion string                 `json:"contract_version"`
		ID              string                 `json:"id"`
		Revision        core.Revision          `json:"revision"`
		Command         ObjectRef              `json:"command"`
		Connection      MCPConnectionFactRefV2 `json:"connection"`
		Source          MCPToolObservationV2   `json:"source"`
		CanonicalObject any                    `json:"canonical_object"`
	}{MCPToolDiscoveryMaterialContractVersionV1, m.Ref.ID, 1, m.Command, m.Connection, m.Source, object}
	return core.CanonicalJSONDigest(mcpToolDiscoveryMaterialCanonicalDomainV1, MCPToolDiscoveryMaterialContractVersionV1, "MCPToolDiscoveryMaterialV1", copy)
}

func SealMCPToolDiscoveryMaterialV1(m MCPToolDiscoveryMaterialV1) (MCPToolDiscoveryMaterialV1, error) {
	m = m.Clone()
	if m.Command.Validate() != nil || m.Connection.Validate() != nil || m.Source.Validate() != nil {
		return MCPToolDiscoveryMaterialV1{}, invalid("MCP Tool Discovery Material source is invalid")
	}
	id, err := DeriveMCPToolDiscoveryMaterialIDV1(m.Command, m.Source)
	if err != nil {
		return MCPToolDiscoveryMaterialV1{}, err
	}
	if m.Ref.ID != "" && m.Ref.ID != id {
		return MCPToolDiscoveryMaterialV1{}, conflict("supplied MCP Tool Discovery Material ID drifted")
	}
	m.Ref = MCPToolDiscoveryMaterialRefV1{ContractVersion: MCPToolDiscoveryMaterialContractVersionV1, ID: id, Revision: 1}
	digest, err := m.ComputeDigest()
	if err != nil {
		return MCPToolDiscoveryMaterialV1{}, err
	}
	m.Ref.Digest = digest
	return m, m.Validate()
}

func (m MCPToolDiscoveryMaterialV1) Clone() MCPToolDiscoveryMaterialV1 {
	m.CanonicalObject = append(json.RawMessage(nil), m.CanonicalObject...)
	return m
}

type MCPToolDiscoveryMaterialExactReaderV1 interface {
	InspectExactMCPToolDiscoveryMaterialV1(context.Context, MCPToolDiscoveryMaterialRefV1) (MCPToolDiscoveryMaterialV1, error)
}

func validateMCPToolDiscoveryCanonicalObjectV1(payload []byte, source MCPToolObservationV2) (any, error) {
	if len(payload) == 0 || len(payload) > MaxMCPToolDiscoveryCanonicalBytesV1 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "MCP Tool Discovery canonical object is empty or exceeds limit")
	}
	var object any
	if err := core.DecodeStrictJSON(payload, &object); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(object)
	if err != nil || !bytes.Equal(canonical, payload) {
		return nil, conflict("MCP Tool Discovery object is not canonical JSON")
	}
	fields, ok := object.(map[string]any)
	if !ok {
		return nil, invalid("MCP Tool Discovery object must be one JSON object")
	}
	name, _ := fields["name"].(string)
	title, _ := fields["title"].(string)
	description, _ := fields["description"].(string)
	if name != source.Name || title != source.Title || !utf8.ValidString(description) || len(description) > MaxStringBytes || core.DigestBytes([]byte(description)) != source.DescriptionDigest {
		return nil, conflict("MCP Tool Discovery text fields drifted")
	}
	input, exists := fields["inputSchema"]
	if !exists {
		return nil, invalid("MCP Tool Discovery inputSchema is absent")
	}
	inputObject, ok := input.(map[string]any)
	if !ok || inputObject["type"] != "object" {
		return nil, invalid("MCP Tool Discovery inputSchema must be one object schema")
	}
	nodes := 0
	if err := validateToolDefinitionSchemaValueV1(input, 0, &nodes); err != nil {
		return nil, err
	}
	properties, _ := inputObject["properties"].(map[string]any)
	if required, exists := inputObject["required"]; exists {
		items, ok := required.([]any)
		if !ok {
			return nil, invalid("MCP Tool Discovery inputSchema required must be an array")
		}
		seen := make(map[string]struct{}, len(items))
		for _, item := range items {
			name, ok := item.(string)
			if !ok || name == "" {
				return nil, invalid("MCP Tool Discovery inputSchema required entries must be names")
			}
			if _, exists := properties[name]; !exists {
				return nil, invalid("MCP Tool Discovery inputSchema required name is absent from properties")
			}
			if _, duplicate := seen[name]; duplicate {
				return nil, invalid("MCP Tool Discovery inputSchema required names must be unique")
			}
			seen[name] = struct{}{}
		}
	}
	checks := []struct {
		discriminator string
		value         any
		expected      core.Digest
	}{
		{"MCPToolObjectV1", object, source.ObjectDigest},
		{"MCPToolInputSchemaV1", input, source.InputSchemaDigest},
		{"MCPToolOutputSchemaV1", fields["outputSchema"], source.OutputSchemaDigest},
		{"MCPToolAnnotationsV1", fields["annotations"], source.AnnotationsDigest},
		{"MCPToolMetaV1", fields["_meta"], source.MetaDigest},
	}
	for _, check := range checks {
		digest, err := core.CanonicalJSONDigest("praxis.tool-mcp.mcp.official-sdk", "praxis.tool-mcp.official-sdk-discovery/v1", check.discriminator, check.value)
		if err != nil || digest != check.expected {
			return nil, conflict("MCP Tool Discovery canonical field digest drifted")
		}
	}
	return object, nil
}
