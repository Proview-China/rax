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
	MCPResourceDiscoveryMaterialContractVersionV1 = "praxis.tool-mcp.mcp-resource-discovery-material/v1"
	MCPPromptDiscoveryMaterialContractVersionV1   = "praxis.tool-mcp.mcp-prompt-discovery-material/v1"
	MaxMCPDiscoveryMaterialCanonicalBytesV1       = 1 << 20
)

const (
	mcpResourceDiscoveryMaterialCanonicalDomainV1 = "praxis.tool-mcp.mcp-resource-discovery-material"
	mcpPromptDiscoveryMaterialCanonicalDomainV1   = "praxis.tool-mcp.mcp-prompt-discovery-material"
	officialSDKDiscoveryCanonicalDomainV1         = "praxis.tool-mcp.mcp.official-sdk"
	officialSDKDiscoveryContractVersionV1         = "praxis.tool-mcp.official-sdk-discovery/v1"
)

// MCPResourceDiscoveryMaterialRefV1 identifies one exact untrusted Resource
// object returned by one governed MCP Discovery Page.
type MCPResourceDiscoveryMaterialRefV1 struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

func (r MCPResourceDiscoveryMaterialRefV1) Validate() error {
	if r.ContractVersion != MCPResourceDiscoveryMaterialContractVersionV1 || ValidateStableID(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil {
		return invalid("MCP Resource Discovery Material Ref is invalid")
	}
	return nil
}

// MCPResourceDiscoveryMaterialV1 preserves provider Resource JSON without
// converting it into Context authority, a read permit, or a Tool Capability.
type MCPResourceDiscoveryMaterialV1 struct {
	Ref             MCPResourceDiscoveryMaterialRefV1 `json:"ref"`
	Command         ObjectRef                         `json:"command"`
	Connection      MCPConnectionFactRefV2            `json:"connection"`
	Source          MCPResourceObservationV2          `json:"source"`
	CanonicalObject json.RawMessage                   `json:"canonical_object"`
}

func DeriveMCPResourceDiscoveryMaterialIDV1(command ObjectRef, source MCPResourceObservationV2) (string, error) {
	if command.Validate() != nil || source.Validate() != nil {
		return "", invalid("MCP Resource Discovery Material identity source is invalid")
	}
	return StableID("mcp-resource-material", command.ID, strconv.FormatUint(uint64(command.Revision), 10), string(command.Digest), source.URI, string(source.ObjectDigest))
}

func (m MCPResourceDiscoveryMaterialV1) Validate() error {
	if m.Ref.Validate() != nil || m.Command.Validate() != nil || m.Connection.Validate() != nil || m.Source.Validate() != nil {
		return invalid("MCP Resource Discovery Material coordinates are invalid")
	}
	id, err := DeriveMCPResourceDiscoveryMaterialIDV1(m.Command, m.Source)
	if err != nil || m.Ref.ID != id {
		return conflict("MCP Resource Discovery Material ID drifted")
	}
	object, err := validateMCPResourceDiscoveryCanonicalObjectV1(m.CanonicalObject, m.Source)
	if err != nil {
		return err
	}
	digest, err := m.computeDigestWithObjectV1(object)
	if err != nil || digest != m.Ref.Digest {
		return conflict("MCP Resource Discovery Material digest drifted")
	}
	return nil
}

func (m MCPResourceDiscoveryMaterialV1) ComputeDigest() (core.Digest, error) {
	if m.Command.Validate() != nil || m.Connection.Validate() != nil || m.Source.Validate() != nil {
		return "", invalid("MCP Resource Discovery Material coordinates are invalid")
	}
	object, err := validateMCPResourceDiscoveryCanonicalObjectV1(m.CanonicalObject, m.Source)
	if err != nil {
		return "", err
	}
	return m.computeDigestWithObjectV1(object)
}

func (m MCPResourceDiscoveryMaterialV1) computeDigestWithObjectV1(object any) (core.Digest, error) {
	body := struct {
		ContractVersion string                   `json:"contract_version"`
		ID              string                   `json:"id"`
		Revision        core.Revision            `json:"revision"`
		Command         ObjectRef                `json:"command"`
		Connection      MCPConnectionFactRefV2   `json:"connection"`
		Source          MCPResourceObservationV2 `json:"source"`
		CanonicalObject any                      `json:"canonical_object"`
	}{MCPResourceDiscoveryMaterialContractVersionV1, m.Ref.ID, 1, m.Command, m.Connection, m.Source, object}
	return core.CanonicalJSONDigest(mcpResourceDiscoveryMaterialCanonicalDomainV1, MCPResourceDiscoveryMaterialContractVersionV1, "MCPResourceDiscoveryMaterialV1", body)
}

func SealMCPResourceDiscoveryMaterialV1(m MCPResourceDiscoveryMaterialV1) (MCPResourceDiscoveryMaterialV1, error) {
	m = m.Clone()
	if m.Command.Validate() != nil || m.Connection.Validate() != nil || m.Source.Validate() != nil {
		return MCPResourceDiscoveryMaterialV1{}, invalid("MCP Resource Discovery Material source is invalid")
	}
	id, err := DeriveMCPResourceDiscoveryMaterialIDV1(m.Command, m.Source)
	if err != nil {
		return MCPResourceDiscoveryMaterialV1{}, err
	}
	if m.Ref.ID != "" && m.Ref.ID != id {
		return MCPResourceDiscoveryMaterialV1{}, conflict("supplied MCP Resource Discovery Material ID drifted")
	}
	m.Ref = MCPResourceDiscoveryMaterialRefV1{ContractVersion: MCPResourceDiscoveryMaterialContractVersionV1, ID: id, Revision: 1}
	m.Ref.Digest, err = m.ComputeDigest()
	if err != nil {
		return MCPResourceDiscoveryMaterialV1{}, err
	}
	return m, m.Validate()
}

func (m MCPResourceDiscoveryMaterialV1) Clone() MCPResourceDiscoveryMaterialV1 {
	m.CanonicalObject = append(json.RawMessage(nil), m.CanonicalObject...)
	return m
}

type MCPResourceDiscoveryMaterialExactReaderV1 interface {
	InspectExactMCPResourceDiscoveryMaterialV1(context.Context, MCPResourceDiscoveryMaterialRefV1) (MCPResourceDiscoveryMaterialV1, error)
}

// MCPPromptDiscoveryMaterialRefV1 identifies one exact untrusted Prompt
// object returned by one governed MCP Discovery Page.
type MCPPromptDiscoveryMaterialRefV1 struct {
	ContractVersion string        `json:"contract_version"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
}

func (r MCPPromptDiscoveryMaterialRefV1) Validate() error {
	if r.ContractVersion != MCPPromptDiscoveryMaterialContractVersionV1 || ValidateStableID(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil {
		return invalid("MCP Prompt Discovery Material Ref is invalid")
	}
	return nil
}

// MCPPromptDiscoveryMaterialV1 preserves provider Prompt JSON without
// converting it into Context authority, a prompt instruction, or a Tool.
type MCPPromptDiscoveryMaterialV1 struct {
	Ref             MCPPromptDiscoveryMaterialRefV1 `json:"ref"`
	Command         ObjectRef                       `json:"command"`
	Connection      MCPConnectionFactRefV2          `json:"connection"`
	Source          MCPPromptObservationV2          `json:"source"`
	CanonicalObject json.RawMessage                 `json:"canonical_object"`
}

func DeriveMCPPromptDiscoveryMaterialIDV1(command ObjectRef, source MCPPromptObservationV2) (string, error) {
	if command.Validate() != nil || source.Validate() != nil {
		return "", invalid("MCP Prompt Discovery Material identity source is invalid")
	}
	return StableID("mcp-prompt-material", command.ID, strconv.FormatUint(uint64(command.Revision), 10), string(command.Digest), source.Name, string(source.ObjectDigest))
}

func (m MCPPromptDiscoveryMaterialV1) Validate() error {
	if m.Ref.Validate() != nil || m.Command.Validate() != nil || m.Connection.Validate() != nil || m.Source.Validate() != nil {
		return invalid("MCP Prompt Discovery Material coordinates are invalid")
	}
	id, err := DeriveMCPPromptDiscoveryMaterialIDV1(m.Command, m.Source)
	if err != nil || m.Ref.ID != id {
		return conflict("MCP Prompt Discovery Material ID drifted")
	}
	object, err := validateMCPPromptDiscoveryCanonicalObjectV1(m.CanonicalObject, m.Source)
	if err != nil {
		return err
	}
	digest, err := m.computeDigestWithObjectV1(object)
	if err != nil || digest != m.Ref.Digest {
		return conflict("MCP Prompt Discovery Material digest drifted")
	}
	return nil
}

func (m MCPPromptDiscoveryMaterialV1) ComputeDigest() (core.Digest, error) {
	if m.Command.Validate() != nil || m.Connection.Validate() != nil || m.Source.Validate() != nil {
		return "", invalid("MCP Prompt Discovery Material coordinates are invalid")
	}
	object, err := validateMCPPromptDiscoveryCanonicalObjectV1(m.CanonicalObject, m.Source)
	if err != nil {
		return "", err
	}
	return m.computeDigestWithObjectV1(object)
}

func (m MCPPromptDiscoveryMaterialV1) computeDigestWithObjectV1(object any) (core.Digest, error) {
	body := struct {
		ContractVersion string                 `json:"contract_version"`
		ID              string                 `json:"id"`
		Revision        core.Revision          `json:"revision"`
		Command         ObjectRef              `json:"command"`
		Connection      MCPConnectionFactRefV2 `json:"connection"`
		Source          MCPPromptObservationV2 `json:"source"`
		CanonicalObject any                    `json:"canonical_object"`
	}{MCPPromptDiscoveryMaterialContractVersionV1, m.Ref.ID, 1, m.Command, m.Connection, m.Source, object}
	return core.CanonicalJSONDigest(mcpPromptDiscoveryMaterialCanonicalDomainV1, MCPPromptDiscoveryMaterialContractVersionV1, "MCPPromptDiscoveryMaterialV1", body)
}

func SealMCPPromptDiscoveryMaterialV1(m MCPPromptDiscoveryMaterialV1) (MCPPromptDiscoveryMaterialV1, error) {
	m = m.Clone()
	if m.Command.Validate() != nil || m.Connection.Validate() != nil || m.Source.Validate() != nil {
		return MCPPromptDiscoveryMaterialV1{}, invalid("MCP Prompt Discovery Material source is invalid")
	}
	id, err := DeriveMCPPromptDiscoveryMaterialIDV1(m.Command, m.Source)
	if err != nil {
		return MCPPromptDiscoveryMaterialV1{}, err
	}
	if m.Ref.ID != "" && m.Ref.ID != id {
		return MCPPromptDiscoveryMaterialV1{}, conflict("supplied MCP Prompt Discovery Material ID drifted")
	}
	m.Ref = MCPPromptDiscoveryMaterialRefV1{ContractVersion: MCPPromptDiscoveryMaterialContractVersionV1, ID: id, Revision: 1}
	m.Ref.Digest, err = m.ComputeDigest()
	if err != nil {
		return MCPPromptDiscoveryMaterialV1{}, err
	}
	return m, m.Validate()
}

func (m MCPPromptDiscoveryMaterialV1) Clone() MCPPromptDiscoveryMaterialV1 {
	m.CanonicalObject = append(json.RawMessage(nil), m.CanonicalObject...)
	return m
}

type MCPPromptDiscoveryMaterialExactReaderV1 interface {
	InspectExactMCPPromptDiscoveryMaterialV1(context.Context, MCPPromptDiscoveryMaterialRefV1) (MCPPromptDiscoveryMaterialV1, error)
}

func validateMCPResourceDiscoveryCanonicalObjectV1(payload []byte, source MCPResourceObservationV2) (any, error) {
	fields, object, err := decodeCanonicalMCPDiscoveryObjectV1(payload, "Resource")
	if err != nil {
		return nil, err
	}
	uri, _ := fields["uri"].(string)
	name, _ := fields["name"].(string)
	title, _ := fields["title"].(string)
	mimeType, _ := fields["mimeType"].(string)
	description, _ := fields["description"].(string)
	if uri != source.URI || name != source.Name || title != source.Title || mimeType != source.MIMEType || !validMCPDiscoveryMaterialTextV1(description) || core.DigestBytes([]byte(description)) != source.DescriptionDigest {
		return nil, conflict("MCP Resource Discovery text fields drifted")
	}
	size := int64(0)
	if raw, exists := fields["size"]; exists {
		number, ok := raw.(json.Number)
		if !ok {
			return nil, invalid("MCP Resource Discovery size is not an integer")
		}
		size, err = strconv.ParseInt(number.String(), 10, 64)
		if err != nil || size < 0 {
			return nil, invalid("MCP Resource Discovery size is invalid")
		}
	}
	if size != source.Size {
		return nil, conflict("MCP Resource Discovery size drifted")
	}
	checks := []struct {
		discriminator string
		value         any
		expected      core.Digest
	}{
		{"MCPResourceObjectV1", object, source.ObjectDigest},
		{"MCPResourceAnnotationsV1", fields["annotations"], source.AnnotationsDigest},
		{"MCPResourceMetaV1", fields["_meta"], source.MetaDigest},
	}
	if err := validateOfficialSDKDiscoveryDigestsV1(checks); err != nil {
		return nil, err
	}
	return object, nil
}

func validateMCPPromptDiscoveryCanonicalObjectV1(payload []byte, source MCPPromptObservationV2) (any, error) {
	fields, object, err := decodeCanonicalMCPDiscoveryObjectV1(payload, "Prompt")
	if err != nil {
		return nil, err
	}
	name, _ := fields["name"].(string)
	title, _ := fields["title"].(string)
	description, _ := fields["description"].(string)
	if name != source.Name || title != source.Title || !validMCPDiscoveryMaterialTextV1(description) || core.DigestBytes([]byte(description)) != source.DescriptionDigest {
		return nil, conflict("MCP Prompt Discovery text fields drifted")
	}
	checks := []struct {
		discriminator string
		value         any
		expected      core.Digest
	}{
		{"MCPPromptObjectV1", object, source.ObjectDigest},
		{"MCPPromptArgumentsV1", fields["arguments"], source.ArgumentsDigest},
		{"MCPPromptMetaV1", fields["_meta"], source.MetaDigest},
	}
	if err := validateOfficialSDKDiscoveryDigestsV1(checks); err != nil {
		return nil, err
	}
	return object, nil
}

func decodeCanonicalMCPDiscoveryObjectV1(payload []byte, kind string) (map[string]any, any, error) {
	if len(payload) == 0 || len(payload) > MaxMCPDiscoveryMaterialCanonicalBytesV1 {
		return nil, nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "MCP "+kind+" Discovery canonical object is empty or exceeds limit")
	}
	var raw json.RawMessage
	if err := core.DecodeStrictJSON(payload, &raw); err != nil {
		return nil, nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var object any
	if err := decoder.Decode(&object); err != nil {
		return nil, nil, invalid("MCP " + kind + " Discovery object is invalid JSON")
	}
	canonical, err := json.Marshal(object)
	if err != nil || !bytes.Equal(canonical, payload) {
		return nil, nil, conflict("MCP " + kind + " Discovery object is not canonical JSON")
	}
	fields, ok := object.(map[string]any)
	if !ok {
		return nil, nil, invalid("MCP " + kind + " Discovery object must be one JSON object")
	}
	return fields, object, nil
}

func validateOfficialSDKDiscoveryDigestsV1(checks []struct {
	discriminator string
	value         any
	expected      core.Digest
}) error {
	for _, check := range checks {
		digest, err := core.CanonicalJSONDigest(officialSDKDiscoveryCanonicalDomainV1, officialSDKDiscoveryContractVersionV1, check.discriminator, check.value)
		if err != nil || digest != check.expected {
			return conflict("MCP Discovery canonical field digest drifted")
		}
	}
	return nil
}

func validMCPDiscoveryMaterialTextV1(value string) bool {
	return utf8.ValidString(value) && len(value) <= MaxStringBytes
}
