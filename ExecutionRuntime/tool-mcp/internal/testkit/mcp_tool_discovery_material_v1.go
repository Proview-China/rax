package testkit

import (
	"encoding/json"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func MCPToolDiscoveryMaterialV1() toolcontract.MCPToolDiscoveryMaterialV1 {
	object := map[string]any{
		"description": "Echo one value.",
		"inputSchema": map[string]any{
			"properties": map[string]any{"value": map[string]any{"type": "string"}},
			"required":   []any{"value"},
			"type":       "object",
		},
		"name": "echo",
	}
	canonical, _ := json.Marshal(object)
	digest := func(discriminator string, value any) core.Digest {
		result, _ := core.CanonicalJSONDigest("praxis.tool-mcp.mcp.official-sdk", "praxis.tool-mcp.official-sdk-discovery/v1", discriminator, value)
		return result
	}
	material, err := toolcontract.SealMCPToolDiscoveryMaterialV1(toolcontract.MCPToolDiscoveryMaterialV1{
		Command:    toolcontract.ObjectRef{ID: "test-page-command", Revision: 1, Digest: Digest("test-page-command")},
		Connection: toolcontract.MCPConnectionFactRefV2{ID: "test-connection", Revision: 1, Digest: Digest("test-connection")},
		Source: toolcontract.MCPToolObservationV2{
			Name:               "echo",
			ObjectDigest:       digest("MCPToolObjectV1", object),
			DescriptionDigest:  core.DigestBytes([]byte("Echo one value.")),
			InputSchemaDigest:  digest("MCPToolInputSchemaV1", object["inputSchema"]),
			OutputSchemaDigest: digest("MCPToolOutputSchemaV1", nil),
			AnnotationsDigest:  digest("MCPToolAnnotationsV1", nil),
			MetaDigest:         digest("MCPToolMetaV1", nil),
		},
		CanonicalObject: canonical,
	})
	if err != nil {
		panic(err)
	}
	return material
}

func MCPDiscoveryPageToolMaterialSetV1() toolcontract.MCPDiscoveryPageToolMaterialSetV1 {
	material := MCPToolDiscoveryMaterialV1()
	receipt := toolcontract.ObjectRef{ID: "test-page-receipt", Revision: 1, Digest: Digest("test-page-receipt")}
	set, err := toolcontract.SealMCPDiscoveryPageToolMaterialSetV1(toolcontract.MCPDiscoveryPageToolMaterialSetV1{
		Receipt: receipt, Command: material.Command, Connection: material.Connection,
		ResponsePageDigest: Digest("test-page-response"),
		Entries:            []toolcontract.MCPDiscoveryPageToolMaterialEntryV1{{Source: material.Source, Material: material.Ref}},
	})
	if err != nil {
		panic(err)
	}
	return set
}
