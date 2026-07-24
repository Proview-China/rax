package blackbox_test

import (
	"encoding/json"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func TestBlackboxMCPToolDiscoveryMaterialJSONRoundTripV1(t *testing.T) {
	object := map[string]any{
		"description": "Echo one value.",
		"inputSchema": map[string]any{
			"properties": map[string]any{"value": map[string]any{"type": "string"}},
			"required":   []any{"value"},
			"type":       "object",
		},
		"name": "echo",
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	digest := func(discriminator string, value any) core.Digest {
		result, digestErr := core.CanonicalJSONDigest("praxis.tool-mcp.mcp.official-sdk", "praxis.tool-mcp.official-sdk-discovery/v1", discriminator, value)
		if digestErr != nil {
			t.Fatal(digestErr)
		}
		return result
	}
	source := toolcontract.MCPToolObservationV2{
		Name:               "echo",
		ObjectDigest:       digest("MCPToolObjectV1", object),
		DescriptionDigest:  core.DigestBytes([]byte("Echo one value.")),
		InputSchemaDigest:  digest("MCPToolInputSchemaV1", object["inputSchema"]),
		OutputSchemaDigest: digest("MCPToolOutputSchemaV1", nil),
		AnnotationsDigest:  digest("MCPToolAnnotationsV1", nil),
		MetaDigest:         digest("MCPToolMetaV1", nil),
	}
	material, err := toolcontract.SealMCPToolDiscoveryMaterialV1(toolcontract.MCPToolDiscoveryMaterialV1{
		Command:         toolcontract.ObjectRef{ID: "page-command-one", Revision: 1, Digest: core.DigestBytes([]byte("command"))},
		Connection:      toolcontract.MCPConnectionFactRefV2{ID: "connection-one", Revision: 1, Digest: core.DigestBytes([]byte("connection"))},
		Source:          source,
		CanonicalObject: canonical,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	var decoded toolcontract.MCPToolDiscoveryMaterialV1
	if err = json.Unmarshal(payload, &decoded); err != nil || decoded.Validate() != nil || decoded.Ref != material.Ref {
		t.Fatalf("MCP Tool Discovery Material round trip drifted: material=%#v err=%v", decoded, err)
	}
	decoded.CanonicalObject = []byte(`{"description":"Echo one value.","inputSchema":{"type":"object"},"name":"other"}`)
	if decoded.Validate() == nil {
		t.Fatal("MCP Tool Discovery Material accepted canonical-object tampering")
	}
	invalidSchema := object
	invalidSchema["inputSchema"] = map[string]any{"type": "object", "required": []any{"missing"}}
	invalidBytes, _ := json.Marshal(invalidSchema)
	if _, err = toolcontract.SealMCPToolDiscoveryMaterialV1(toolcontract.MCPToolDiscoveryMaterialV1{
		Command: material.Command, Connection: material.Connection, Source: source, CanonicalObject: invalidBytes,
	}); err == nil {
		t.Fatal("MCP Tool Discovery Material accepted a required field absent from properties")
	}
	receipt := toolcontract.ObjectRef{ID: "page-receipt-one", Revision: 1, Digest: core.DigestBytes([]byte("receipt"))}
	set, err := toolcontract.SealMCPDiscoveryPageToolMaterialSetV1(toolcontract.MCPDiscoveryPageToolMaterialSetV1{
		Receipt: receipt, Command: material.Command, Connection: material.Connection,
		ResponsePageDigest: core.DigestBytes([]byte("page")),
		Entries:            []toolcontract.MCPDiscoveryPageToolMaterialEntryV1{{Source: material.Source, Material: material.Ref}},
	})
	if err != nil || set.Validate() != nil || set.Entries[0].Material != material.Ref {
		t.Fatalf("MCP Discovery Page Tool Material Set drifted: set=%#v err=%v", set, err)
	}
	if _, err = toolcontract.SealMCPDiscoveryPageToolMaterialSetV1(toolcontract.MCPDiscoveryPageToolMaterialSetV1{
		Receipt: receipt, Command: material.Command, Connection: material.Connection,
		ResponsePageDigest: core.DigestBytes([]byte("page")),
		Entries: []toolcontract.MCPDiscoveryPageToolMaterialEntryV1{
			{Source: material.Source, Material: material.Ref},
			{Source: material.Source, Material: material.Ref},
		},
	}); err == nil {
		t.Fatal("MCP Discovery Page Tool Material Set accepted duplicate Tool names")
	}
}
