package mcp

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func FuzzValidateMCPNegotiatedProtocolV1(f *testing.F) {
	for _, seed := range []string{
		toolcontract.MCPStableProtocolVersion,
		"2025-06-18",
		"2026-01-01",
		"",
		"not-a-version",
		"2025-11-25\x00",
	} {
		f.Add(seed)
	}
	server, err := toolcontract.SealMCPServer(toolcontract.MCPServerDescriptor{
		ID:                 "mcp-server-connect-fuzz-v1",
		Revision:           1,
		Owner:              core.OwnerRef{Domain: "praxis.tool-mcp", ID: "registry"},
		Source:             "praxis.mcp/source-fuzz",
		MinimumProtocol:    toolcontract.MCPStableProtocolVersion,
		MaximumProtocol:    toolcontract.MCPStableProtocolVersion,
		Transports:         []runtimeports.NamespacedNameV2{toolcontract.MCPTransportStdioV1},
		AuthRequirement:    "praxis.mcp.auth/none",
		TrustClass:         "praxis.mcp.trust/test",
		NetworkScopeDigest: core.DigestBytes([]byte("network")),
		ArtifactDigest:     core.DigestBytes([]byte("artifact")),
		ConfigDigest:       core.DigestBytes([]byte("config")),
		Conformance:        "praxis.mcp.conformance/official-go-sdk",
		CreatedUnixNano:    time.Unix(1, 0).UnixNano(),
	})
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, negotiated string) {
		err := validateMCPNegotiatedProtocolV1(server, negotiated)
		if negotiated == toolcontract.MCPStableProtocolVersion && err != nil {
			t.Fatalf("stable negotiated protocol rejected: %v", err)
		}
		if negotiated != toolcontract.MCPStableProtocolVersion && err == nil {
			t.Fatalf("out-of-range negotiated protocol accepted: %q", negotiated)
		}
	})
}
