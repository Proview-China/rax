package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestMCPConnectionCoordinateRunIsolationV1(t *testing.T) {
	fixture := testkit.MCPConnectV1(testkit.FixedTime, contract.MCPTransportStdioV1)
	other := testkit.MCPConnectCoordinateForRunV1(fixture.Coordinate, "run-other", 1)
	if fixture.Coordinate.ID == other.ID {
		t.Fatal("different Runs shared one MCP Connection coordinate")
	}
	if fixture.Coordinate.Validate() != nil || other.Validate() != nil {
		t.Fatal("valid Run-scoped MCP Connection coordinate was rejected")
	}
}

func TestMCPTransportConfigStrictOneOfAndEndpointV1(t *testing.T) {
	fixture := testkit.MCPConnectV1(testkit.FixedTime, contract.MCPTransportStreamableHTTPV1)
	if fixture.Config.Validate() != nil {
		t.Fatal("valid Streamable HTTP config was rejected")
	}
	bad := fixture.Config
	bad.Ref.Digest = ""
	bad.StreamableHTTP.Endpoint = "http://example.com/mcp"
	if _, err := contract.SealMCPTransportConfigV1(bad); err == nil {
		t.Fatal("non-loopback plain HTTP endpoint was admitted")
	}
	bad = fixture.Config
	bad.Ref.Digest = ""
	bad.Stdio = &contract.MCPStdioTransportConfigV1{Executable: "/bin/false"}
	if _, err := contract.SealMCPTransportConfigV1(bad); err == nil {
		t.Fatal("two transport variants were admitted")
	}
}

func TestMCPConnectIntentExactOwnerRunAndConfigV1(t *testing.T) {
	fixture := testkit.MCPConnectV1(testkit.FixedTime, contract.MCPTransportStdioV1)
	if fixture.Intent.Validate() != nil || fixture.Intent.RuntimeDomainCommandRefV1().Validate() != nil {
		t.Fatal("valid MCP Connect Intent was rejected")
	}
	tests := []struct {
		name   string
		mutate func(*contract.MCPConnectIntentV1)
	}{
		{"run", func(v *contract.MCPConnectIntentV1) {
			v.Coordinate = testkit.MCPConnectCoordinateForRunV1(v.Coordinate, "run-other", 1)
		}},
		{"transport", func(v *contract.MCPConnectIntentV1) { v.TransportConfig.Digest = testkit.Digest("other-config") }},
		{"owner", func(v *contract.MCPConnectIntentV1) { v.Owner.ManifestDigest = testkit.Digest("other-owner") }},
		{"not-after", func(v *contract.MCPConnectIntentV1) { v.NotAfterUnixNano = v.CreatedUnixNano }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := fixture.Intent
			test.mutate(&value)
			if value.Validate() == nil {
				t.Fatal("drifted MCP Connect Intent remained valid")
			}
		})
	}
}

func TestMCPStdioConfigRejectsSecretShapedPlaceholderAndNULV1(t *testing.T) {
	fixture := testkit.MCPConnectV1(time.Now().UTC(), contract.MCPTransportStdioV1)
	bad := fixture.Config
	bad.Ref.Digest = ""
	bad.Stdio.CredentialPlaceholders = []string{"token-value"}
	if _, err := contract.SealMCPTransportConfigV1(bad); err == nil {
		t.Fatal("non-nominal credential placeholder was admitted")
	}
	bad = fixture.Config
	bad.Ref.Digest = ""
	bad.Stdio.Arguments = []string{"bad\x00argument"}
	if _, err := contract.SealMCPTransportConfigV1(bad); err == nil {
		t.Fatal("NUL-bearing stdio argument was admitted")
	}
}
