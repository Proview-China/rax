package testkit

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPConnectFixtureV1 struct {
	Now        time.Time
	Coordinate toolcontract.MCPConnectionCoordinateV1
	Config     toolcontract.MCPTransportConfigV1
	Intent     toolcontract.MCPConnectIntentV1
}

func MCPConnectV1(now time.Time, kind runtimeports.NamespacedNameV2) MCPConnectFixtureV1 {
	server := MCPServer()
	serverRef := toolcontract.ObjectRef{ID: server.ID, Revision: server.Revision, Digest: server.Digest}
	coordinate, err := toolcontract.SealMCPConnectionCoordinateV1(toolcontract.MCPConnectionCoordinateV1{
		TenantID: "tenant-v2", IdentityID: "identity-v2", IdentityEpoch: 1, PlanDigest: Digest("plan-v2"),
		InstanceID: "instance-v2", InstanceEpoch: 1, RunID: "run-v2",
		Session: toolcontract.ObjectRef{ID: "session-v2", Revision: 1, Digest: Digest("session-v2")}, Server: serverRef, Epoch: 1,
	})
	if err != nil {
		panic(err)
	}
	provider := ProviderBinding()
	provider.Capability = runtimeports.CapabilityNameV2(toolcontract.MCPConnectEffectKindV1)
	transport := provider
	transport.ComponentID = "tool-mcp/mcp-transport"
	transport.ManifestDigest = Digest("mcp-transport-manifest")
	transport.ArtifactDigest = Digest("mcp-transport-artifact")
	transport.Capability = "praxis.mcp/controlled-transport-v1"
	config := toolcontract.MCPTransportConfigV1{
		Ref: toolcontract.MCPTransportConfigRefV1{Revision: 1}, Owner: Owner(), Server: serverRef, Kind: kind,
		ProviderTransport: transport, ArtifactDigest: server.ArtifactDigest, ConfigDigest: server.ConfigDigest,
		NetworkScopeDigest: server.NetworkScopeDigest, SandboxRequirementDigest: Digest("mcp-sandbox"), CreatedUnixNano: now.UnixNano(),
	}
	if kind == toolcontract.MCPTransportStdioV1 {
		config.Stdio = &toolcontract.MCPStdioTransportConfigV1{Executable: "/usr/bin/mcp-test", Arguments: []string{"--stdio"}, CredentialPlaceholders: []string{"MCP_TOKEN"}}
	} else {
		config.StreamableHTTP = &toolcontract.MCPStreamableHTTPTransportConfigV1{Endpoint: "http://127.0.0.1:8123/mcp"}
	}
	config, err = toolcontract.SealMCPTransportConfigV1(config)
	if err != nil {
		panic(err)
	}
	boundary := BoundaryFixture(now)
	intent, err := toolcontract.SealMCPConnectIntentV1(toolcontract.MCPConnectIntentV1{
		Ref: toolcontract.ObjectRef{Revision: 1}, Owner: SettlementOwner(), Coordinate: coordinate, Server: serverRef,
		TransportConfig: config.Ref, Operation: boundary.Operation, OperationDigest: boundary.Attempt.OperationDigest,
		EffectID: boundary.Attempt.EffectID, EffectRevision: boundary.Attempt.IntentRevision, EffectKind: toolcontract.MCPConnectEffectKindV1,
		PolicyProfile: toolcontract.MCPConnectPolicyProfileV1, IntentDigest: boundary.Attempt.IntentDigest, Attempt: boundary.Attempt,
		CredentialLeases: []runtimeports.CredentialLeaseRefV2{{Ref: "credential-mcp-v1", Class: "bearer", ScopeDigest: Digest("credential-scope"), Epoch: 1}},
		Provider:         provider, ProviderTransport: transport, NetworkScopeDigest: config.NetworkScopeDigest,
		SandboxRequirementDigest: config.SandboxRequirementDigest, CreatedUnixNano: now.UnixNano(), NotAfterUnixNano: now.Add(20 * time.Second).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return MCPConnectFixtureV1{Now: now, Coordinate: coordinate, Config: config, Intent: intent}
}

func MCPConnectCoordinateForRunV1(base toolcontract.MCPConnectionCoordinateV1, runID string, epoch core.Epoch) toolcontract.MCPConnectionCoordinateV1 {
	base.ID = ""
	base.RunID = runID
	base.Epoch = epoch
	value, err := toolcontract.SealMCPConnectionCoordinateV1(base)
	if err != nil {
		panic(err)
	}
	return value
}
