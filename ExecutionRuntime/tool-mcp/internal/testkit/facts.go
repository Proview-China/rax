package testkit

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

var FixedTime = time.Unix(1_800_000_000, 123).UTC()

func Digest(label string) core.Digest { return core.DigestBytes([]byte(label)) }

func Owner() core.OwnerRef { return core.OwnerRef{Domain: "tool-mcp", ID: "engine"} }

func SettlementOwner() runtimeports.EffectOwnerRefV2 {
	return runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: "tool-mcp/engine", ManifestDigest: Digest("manifest")}
}

func Schema(name string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "tool", Name: name, Version: "1.0.0", MediaType: "application/json", ContentDigest: Digest("schema:" + name)}
}

func Payload(body string) runtimeports.OpaquePayloadV2 {
	data := []byte(body)
	return runtimeports.OpaquePayloadV2{
		Schema: Schema("payload"), ContentDigest: core.DigestBytes(data), Length: uint64(len(data)), Inline: data,
		LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "tool/standard", Digest: Digest("limit")},
	}
}

func Capability() contract.CapabilityDescriptor {
	value, err := contract.SealCapability(contract.CapabilityDescriptor{
		ID: "tool/example", SemanticVersion: "1.0.0", Revision: 1, Owner: Owner(), InputSchema: Schema("input"), OutputSchema: Schema("output"), ActionScopeSchema: Schema("scope"),
		EffectKinds: []runtimeports.NamespacedNameV2{"praxis.tool/execute"}, Risk: contract.RiskModerate, ReviewProfile: "review/standard", AuthorityRequirement: "authority/tool",
		BudgetRequirement: "budget/standard", SandboxRequirement: "sandbox/tool", EvidenceRequirement: "evidence/receipt", Compatibility: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, CreatedUnixNano: FixedTime.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return value
}

func Tool() contract.ToolDescriptor {
	capability := Capability()
	value, err := contract.SealTool(contract.ToolDescriptor{
		ID: "tool/example-local", SemanticVersion: "1.0.0", Revision: 1, Owner: Owner(), Capability: contract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest},
		ArtifactDigest: Digest("tool-artifact"), Mechanism: contract.MechanismLocal, InputSchema: capability.InputSchema, OutputSchema: capability.OutputSchema, EffectKinds: capability.EffectKinds,
		TimeoutMillis: 5000, ConcurrencyLimit: 4, CancellationSupported: true, Idempotency: "tool/provider-key", ConflictDomain: "tenant/{tenant}/workspace/{workspace}", ResultLimitBytes: 1 << 20,
		Conformance: "tool/wave1", CreatedUnixNano: FixedTime.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return value
}

func Package() contract.ToolPackageManifest {
	tool := Tool()
	value, err := contract.SealPackage(contract.ToolPackageManifest{
		ID: "package/example", SemanticVersion: "1.0.0", Revision: 1, Publisher: Owner(), ArtifactDigest: Digest("package-artifact"), Signatures: []core.Digest{Digest("signature")},
		Descriptors: []contract.PackageDescriptorRef{{ToolID: tool.ID, Revision: tool.Revision, Digest: tool.Digest}}, EffectKinds: tool.EffectKinds, ReviewRequirement: "review/package",
		SandboxRequirement: "sandbox/package", ProvenanceDigest: Digest("provenance"), CreatedUnixNano: FixedTime.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return value
}

func Candidate() contract.ActionCandidate {
	capability, tool := Capability(), Tool()
	id, err := contract.StableID("action", "run-1", "pending-1", "candidate-1")
	if err != nil {
		panic(err)
	}
	value, err := contract.SealActionCandidate(contract.ActionCandidate{
		ID: id, Revision: 1, RunID: "run-1", SessionID: "session-1", PendingActionRef: "pending-1", PendingActionDigest: Digest("pending-action-request"), SourceCandidateDigest: Digest("candidate-source"),
		Capability: contract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}, Tool: contract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest},
		Payload: Payload(`{"value":1}`), PayloadRevision: 1, ActionScopeDigest: Digest("scope"), EffectKinds: []runtimeports.NamespacedNameV2{"praxis.tool/execute"}, Risk: contract.RiskModerate,
		ExpectedOwner: SettlementOwner(), ConflictDomain: "tenant/t1/workspace/w1", IdempotencyKey: "action-1", CreatedUnixNano: FixedTime.UnixNano(), ExpiresUnixNano: FixedTime.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return value
}

func Settlement(attemptID string, payload runtimeports.OpaquePayloadV2) runtimeports.OperationSettlementRefV3 {
	return runtimeports.OperationSettlementRefV3{
		ID: "settlement-1", Revision: 1, Digest: Digest("settlement"),
		Attempt:     runtimeports.OperationDispatchAttemptRefV3{OperationDigest: Digest("operation"), EffectID: "effect-1", IntentRevision: 1, IntentDigest: Digest("intent"), PermitID: "permit-1", PermitRevision: 1, PermitDigest: Digest("permit"), AttemptID: attemptID},
		Disposition: runtimeports.OperationSettlementAppliedV3, Owner: SettlementOwner(), Evidence: []runtimeports.EvidenceRecordRefV2{{LedgerScopeDigest: Digest("ledger"), Sequence: 1, RecordDigest: Digest("record")}},
		DomainResultSchema: &payload.Schema, DomainResultDigest: payload.ContentDigest,
	}
}

func MCPServer() contract.MCPServerDescriptor {
	id, _ := contract.StableID("mcp-server", "local-test")
	value, err := contract.SealMCPServer(contract.MCPServerDescriptor{
		ID: id, Revision: 1, Owner: Owner(), Source: "mcp/local-test", MinimumProtocol: "2025-06-18", MaximumProtocol: contract.MCPStableProtocolVersion,
		Transports: []runtimeports.NamespacedNameV2{"mcp/local"}, AuthRequirement: "auth/test", TrustClass: "trust/test", NetworkScopeDigest: Digest("network"), ArtifactDigest: Digest("mcp-artifact"), ConfigDigest: Digest("mcp-config"), Conformance: "mcp/wave1", CreatedUnixNano: FixedTime.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return value
}

func MCPConnection() contract.MCPConnectionRef {
	server := MCPServer()
	id, _ := contract.StableID("mcp-connection", server.ID, "tenant-1", "session-1")
	value, err := contract.SealMCPConnection(contract.MCPConnectionRef{
		ID: id, Revision: 1, Epoch: 1, Server: contract.ObjectRef{ID: server.ID, Revision: server.Revision, Digest: server.Digest}, TenantID: "tenant-1", IdentityID: "identity-1",
		PlanDigest: Digest("plan"), InstanceID: "instance-1", RunID: "run-1", NegotiatedProtocol: contract.MCPStableProtocolVersion, SessionID: "mcp-session-1", CreatedUnixNano: FixedTime.UnixNano(), ExpiresUnixNano: FixedTime.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return value
}

func MCPSnapshot() contract.MCPCapabilitySnapshot {
	server, connection := MCPServer(), MCPConnection()
	id, _ := contract.StableID("mcp-snapshot", connection.ID, "1")
	value, err := contract.SealMCPSnapshot(contract.MCPCapabilitySnapshot{
		ID: id, Revision: 1, Server: contract.ObjectRef{ID: server.ID, Revision: server.Revision, Digest: server.Digest}, Connection: contract.ObjectRef{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest},
		ConnectionEpoch: connection.Epoch, ProtocolVersion: contract.MCPStableProtocolVersion, Tools: []contract.MCPToolObservation{{Name: "example", DescriptionDigest: Digest("description"), InputSchemaDigest: Digest("input")}},
		SourceDigest: Digest("source"), ValidationDigest: Digest("validation"), Conformance: "mcp/wave1", CreatedUnixNano: FixedTime.UnixNano(), ExpiresUnixNano: FixedTime.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return value
}
