package testkit

import (
	"encoding/json"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

const officialSDKDiscoveryVersionV1 = "praxis.tool-mcp.official-sdk-discovery/v1"

func MCPResourceDiscoveryMaterialV1() toolcontract.MCPResourceDiscoveryMaterialV1 {
	object := map[string]any{
		"description": "workspace readme", "mimeType": "text/markdown", "name": "readme",
		"size": float64(128), "title": "README", "uri": "file:///workspace/readme.md",
	}
	payload, _ := json.Marshal(object)
	source := toolcontract.MCPResourceObservationV2{
		URI: "file:///workspace/readme.md", Name: "readme", Title: "README", MIMEType: "text/markdown", Size: 128,
		ObjectDigest:      officialSDKDiscoveryDigestV1("MCPResourceObjectV1", object),
		DescriptionDigest: core.DigestBytes([]byte("workspace readme")),
		AnnotationsDigest: officialSDKDiscoveryDigestV1("MCPResourceAnnotationsV1", nil),
		MetaDigest:        officialSDKDiscoveryDigestV1("MCPResourceMetaV1", nil),
	}
	material, err := toolcontract.SealMCPResourceDiscoveryMaterialV1(toolcontract.MCPResourceDiscoveryMaterialV1{
		Command:    toolcontract.ObjectRef{ID: "mcp-resource-page", Revision: 1, Digest: Digest("mcp-resource-page")},
		Connection: toolcontract.MCPConnectionFactRefV2{ID: "mcp-resource-connection", Revision: 1, Digest: Digest("mcp-resource-connection")},
		Source:     source, CanonicalObject: payload,
	})
	if err != nil {
		panic(err)
	}
	return material
}

func MCPPromptDiscoveryMaterialV1() toolcontract.MCPPromptDiscoveryMaterialV1 {
	arguments := []any{map[string]any{"name": "scope", "required": true}}
	object := map[string]any{
		"arguments": arguments, "description": "review changes", "name": "review", "title": "Review",
	}
	payload, _ := json.Marshal(object)
	source := toolcontract.MCPPromptObservationV2{
		Name: "review", Title: "Review",
		ObjectDigest:      officialSDKDiscoveryDigestV1("MCPPromptObjectV1", object),
		DescriptionDigest: core.DigestBytes([]byte("review changes")),
		ArgumentsDigest:   officialSDKDiscoveryDigestV1("MCPPromptArgumentsV1", arguments),
		MetaDigest:        officialSDKDiscoveryDigestV1("MCPPromptMetaV1", nil),
	}
	material, err := toolcontract.SealMCPPromptDiscoveryMaterialV1(toolcontract.MCPPromptDiscoveryMaterialV1{
		Command:    toolcontract.ObjectRef{ID: "mcp-prompt-page", Revision: 1, Digest: Digest("mcp-prompt-page")},
		Connection: toolcontract.MCPConnectionFactRefV2{ID: "mcp-prompt-connection", Revision: 1, Digest: Digest("mcp-prompt-connection")},
		Source:     source, CanonicalObject: payload,
	})
	if err != nil {
		panic(err)
	}
	return material
}

func MCPDiscoveryPageResourceMaterialSetV1() toolcontract.MCPDiscoveryPageResourceMaterialSetV1 {
	material := MCPResourceDiscoveryMaterialV1()
	receipt := toolcontract.ObjectRef{ID: "test-resource-page-receipt", Revision: 1, Digest: Digest("test-resource-page-receipt")}
	set, err := toolcontract.SealMCPDiscoveryPageResourceMaterialSetV1(toolcontract.MCPDiscoveryPageResourceMaterialSetV1{
		Receipt: receipt, Command: material.Command, Connection: material.Connection,
		ResponsePageDigest: Digest("test-resource-page-response"),
		Entries:            []toolcontract.MCPDiscoveryPageResourceMaterialEntryV1{{Source: material.Source, Material: material.Ref}},
	})
	if err != nil {
		panic(err)
	}
	return set
}

func MCPDiscoveryPagePromptMaterialSetV1() toolcontract.MCPDiscoveryPagePromptMaterialSetV1 {
	material := MCPPromptDiscoveryMaterialV1()
	receipt := toolcontract.ObjectRef{ID: "test-prompt-page-receipt", Revision: 1, Digest: Digest("test-prompt-page-receipt")}
	set, err := toolcontract.SealMCPDiscoveryPagePromptMaterialSetV1(toolcontract.MCPDiscoveryPagePromptMaterialSetV1{
		Receipt: receipt, Command: material.Command, Connection: material.Connection,
		ResponsePageDigest: Digest("test-prompt-page-response"),
		Entries:            []toolcontract.MCPDiscoveryPagePromptMaterialEntryV1{{Source: material.Source, Material: material.Ref}},
	})
	if err != nil {
		panic(err)
	}
	return set
}

func officialSDKDiscoveryDigestV1(discriminator string, value any) core.Digest {
	digest, err := core.CanonicalJSONDigest("praxis.tool-mcp.mcp.official-sdk", officialSDKDiscoveryVersionV1, discriminator, value)
	if err != nil {
		panic(err)
	}
	return digest
}
