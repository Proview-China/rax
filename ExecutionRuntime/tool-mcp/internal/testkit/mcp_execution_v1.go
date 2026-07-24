package testkit

import (
	"encoding/json"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPExecutionFixtureV1 struct {
	Now               time.Time
	Command           toolcontract.MCPExecutionCommandFactV1
	Association       runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1
	Authorization     runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3
	ProviderTransport runtimeports.ProviderBindingRefV2
	Provider          runtimeports.ProviderBindingRefV2
}

func MCPProtocolReceiptV1(fixture MCPExecutionFixtureV1, observed time.Time) toolcontract.MCPProtocolReceiptV1 {
	admission, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{
		ID: "mcp-admission-" + fixture.Command.Ref.ID, Revision: 1, StableKeyDigest: fixture.Authorization.StableKeyDigest, Admitted: true,
	})
	if err != nil {
		panic(err)
	}
	response, err := json.Marshal(map[string]any{"content": []any{map[string]any{"type": "text", "text": "ok"}}})
	if err != nil {
		panic(err)
	}
	receipt, err := toolcontract.SealMCPProtocolReceiptV1(toolcontract.MCPProtocolReceiptV1{
		Command: fixture.Command.Ref, StableKeyDigest: fixture.Authorization.StableKeyDigest, AdmissionReceipt: admission,
		JSONRPCRequestID: fixture.Command.JSONRPCRequestID, CanonicalResponse: response, ObservedUnixNano: observed.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return receipt
}

func MCPExecutionV1(now time.Time) MCPExecutionFixtureV1 {
	return MCPExecutionWithProviderSessionV1(now, "mcp-session-call-v1")
}

// MCPExecutionWithProviderSessionV1 builds the same exact governed call
// fixture while binding the Tool-owned Connection fact to the session ID
// observed from a real MCP transport. An empty Provider session ID remains
// valid for transports that do not publish one.
func MCPExecutionWithProviderSessionV1(now time.Time, providerSessionID string) MCPExecutionFixtureV1 {
	provider := ProviderBinding()
	transport := provider
	transport.ComponentID, transport.ManifestDigest, transport.ArtifactDigest, transport.Capability = "tool-mcp/transport", Digest("transport-manifest"), Digest("transport-artifact"), runtimeports.ControlledOperationProviderTransportCapabilityV2
	boundary := BoundaryFixture(now)
	projection := ModelProjection(1)
	source, err := toolcontract.SealModelSourceCandidateHistoricalRefV1(projection)
	if err != nil {
		panic(err)
	}
	capability := Capability()
	tool := Tool()
	tool.Mechanism, tool.ArtifactDigest, tool.Digest = toolcontract.MechanismMCP, provider.ArtifactDigest, ""
	tool, err = toolcontract.SealTool(tool)
	if err != nil {
		panic(err)
	}
	capRef := toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
	toolRef := toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}
	surfaceRef := toolcontract.ToolSurfaceManifestCurrentRefV1{ContractVersion: toolcontract.ToolSurfaceManifestCurrentContractVersionV1, ID: "surface-mcp-call-v1", Revision: 1, Digest: Digest("surface-mcp-call-v1")}
	capCurrentID, _ := toolcontract.DeriveToolRegistryObjectCurrentIDV1(toolcontract.ToolRegistryCapabilityCurrentKindV1, capRef, capability.Owner)
	toolCurrentID, _ := toolcontract.DeriveToolRegistryObjectCurrentIDV1(toolcontract.ToolRegistryDescriptorCurrentKindV1, toolRef, tool.Owner)
	capCurrent := toolcontract.ToolRegistryObjectCurrentRefV1{Kind: toolcontract.ToolRegistryCapabilityCurrentKindV1, ID: capCurrentID, Revision: 1, Digest: Digest("cap-current")}
	toolCurrent := toolcontract.ToolRegistryObjectCurrentRefV1{Kind: toolcontract.ToolRegistryDescriptorCurrentKindV1, ID: toolCurrentID, Revision: 1, Digest: Digest("tool-current")}
	schemaCurrent, err := toolcontract.SealToolInputSchemaCurrentRefV1(toolcontract.ToolInputSchemaCurrentRefV1{InputSchema: tool.InputSchema, Authority: toolCurrent, RegistryOwner: tool.Owner, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(12 * time.Second).UnixNano()})
	if err != nil {
		panic(err)
	}
	inputRef := toolcontract.ToolInputContractCurrentRefV1{ID: "input-contract-mcp-call-v1", Revision: 1, Digest: Digest("input-contract")}
	arguments := append([]byte(nil), projection.Observation.Calls[0].CanonicalArguments...)
	payload := runtimeports.OpaquePayloadV2{Schema: tool.InputSchema, ContentDigest: core.DigestBytes(arguments), Length: uint64(len(arguments)), Inline: arguments, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: toolcontract.ToolInputLimitPolicyV1, Digest: Digest("input-limit")}}
	candidate, err := toolcontract.SealActionCandidateV3(toolcontract.ActionCandidateV3{
		TenantID: "tenant-v2", RunID: "run-v2", SessionID: "session-v2", TurnID: "1", PendingAction: toolcontract.PendingActionExactRefV2{ID: "pending-mcp-call-v1", Revision: 1, RequestDigest: Digest("pending")}, SourceCandidate: source,
		Surface: toolcontract.ObjectRef{ID: surfaceRef.ID, Revision: surfaceRef.Revision, Digest: surfaceRef.Digest}, Capability: capRef, Tool: toolRef, InputSchema: tool.InputSchema, Payload: payload, PayloadRevision: 1, LimitPolicy: payload.LimitPolicy,
		InputContractCurrentRef: inputRef, SurfaceCurrent: surfaceRef, CapabilityCurrent: capCurrent, ToolCurrent: toolCurrent, InputSchemaCurrent: schemaCurrent,
		OperationScopeDigest: boundary.Operation.ExecutionScopeDigest, EffectKind: runtimeports.OperationScopeEvidenceActionEffectKindV3, ExpectedOwner: SettlementOwner(), ConflictDomain: "tenant/tenant-v2/tool/example", IdempotencyKey: "mcp-call-v1", CreatedUnixNano: now.UnixNano(), RequestedExpiresUnixNano: now.Add(10 * time.Second).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	server := MCPServer()
	connectionID, _ := toolcontract.StableID("mcp-connection", server.ID, "tenant-v2", "session-v2")
	connection, err := toolcontract.SealMCPConnection(toolcontract.MCPConnectionRef{ID: connectionID, Revision: 1, Epoch: 1, Server: toolcontract.ObjectRef{ID: server.ID, Revision: server.Revision, Digest: server.Digest}, TenantID: "tenant-v2", IdentityID: "identity-v2", PlanDigest: Digest("plan-v2"), InstanceID: "instance-v2", RunID: "run-v2", NegotiatedProtocol: toolcontract.MCPStableProtocolVersion, SessionID: providerSessionID, CreatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()})
	if err != nil {
		panic(err)
	}
	snapshotTool := toolcontract.MCPToolObservationV2{Name: source.CallName, ObjectDigest: Digest("snapshot-tool"), DescriptionDigest: Digest("description"), InputSchemaDigest: tool.InputSchema.ContentDigest, OutputSchemaDigest: tool.OutputSchema.ContentDigest, AnnotationsDigest: Digest("annotations"), MetaDigest: Digest("meta")}
	snapshot, err := toolcontract.SealMCPCapabilitySnapshotV2(toolcontract.MCPCapabilitySnapshotV2{Revision: 1, Server: connection.Server, Connection: toolcontract.ObjectRef{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest}, ConnectionEpoch: connection.Epoch, ProtocolVersion: connection.NegotiatedProtocol, ServerInfoDigest: Digest("server-info"), ServerCapabilitiesDigest: Digest("server-capabilities"), InstructionsDigest: Digest("instructions"), Tools: []toolcontract.MCPToolObservationV2{snapshotTool}, SourceDigest: Digest("source"), Conformance: "mcp/official-go-sdk-v1", CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(15 * time.Second).UnixNano()})
	if err != nil {
		panic(err)
	}
	prepared := PreparedAttemptFor(now, boundary, provider, payload.Schema, payload.ContentDigest, 1)
	command, err := toolcontract.SealMCPExecutionCommandFactV1(toolcontract.MCPExecutionCommandFactV1{Owner: SettlementOwner(), BindingCurrent: toolcontract.SingleCallToolActionBindingCurrentRefV2{ID: "binding-mcp-call-v1", Revision: 1, Digest: Digest("binding")}, Candidate: candidate, InputContractCurrent: inputRef, Capability: capability, Tool: tool, Server: server, Connection: connection, Snapshot: snapshot, SnapshotTool: snapshotTool, Method: toolcontract.MCPToolsCallMethodV1, Params: payload, ParamsRevision: 1, Operation: boundary.Operation, OperationDigest: boundary.Attempt.OperationDigest, EffectID: boundary.Attempt.EffectID, EffectRevision: boundary.Attempt.IntentRevision, IntentDigest: boundary.Attempt.IntentDigest, Prepared: prepared, Attempt: boundary.Attempt, Provider: provider, CreatedUnixNano: now.UnixNano(), NotAfterUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		panic(err)
	}
	association, err := runtimeports.SealPreparedDomainCommandAssociationCurrentProjectionV1(runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1{Operation: boundary.Operation, OperationDigest: boundary.Attempt.OperationDigest, EffectID: boundary.Attempt.EffectID, EffectRevision: boundary.Attempt.IntentRevision, IntentDigest: boundary.Attempt.IntentDigest, Prepared: prepared, Attempt: boundary.Attempt, Provider: provider, PayloadSchema: payload.Schema, PayloadDigest: payload.ContentDigest, PayloadRevision: 1, DomainCommand: command.RuntimeDomainCommandRefV1(), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		panic(err)
	}
	authorization, err := runtimeports.SealControlledOperationPhysicalExecutionAuthorizationV3(runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3{UnifiedNotAfterUnixNano: now.Add(10 * time.Second).UnixNano(), ProviderTransport: transport, Provider: provider, Operation: boundary.Operation, OperationDigest: boundary.Attempt.OperationDigest, OperationScopeDigest: boundary.Operation.ExecutionScopeDigest, EffectKind: runtimeports.OperationScopeEvidenceActionEffectKindV3, Prepared: prepared, Attempt: boundary.Attempt, ExecuteEnforcement: boundary.Enforcement, ExecuteEvidenceHandoff: boundary.Handoff.RefV3(), Boundary: runtimeports.OperationProviderBoundaryRefV1{ID: "boundary-mcp-call-v1", Revision: 1, Digest: Digest("boundary")}, Association: association.Ref, DomainCommand: command.RuntimeDomainCommandRefV1()})
	if err != nil {
		panic(err)
	}
	return MCPExecutionFixtureV1{Now: now, Command: command, Association: association, Authorization: authorization, ProviderTransport: transport, Provider: provider}
}
