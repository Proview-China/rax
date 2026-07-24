package testkit

import (
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPConnectControlledFixtureV1 struct {
	Connect       MCPConnectFixtureV1
	Authorization runtimeports.ControlledMCPConnectPhysicalAuthorizationV1
}

func MCPConnectControlledV1(now time.Time, kind runtimeports.NamespacedNameV2) MCPConnectControlledFixtureV1 {
	connect := MCPConnectV1(now, kind)
	intent := connect.Intent
	declared := runtimeports.ExecutionDelegationRefV2{ID: intent.Attempt.Delegation.ID, Revision: intent.Attempt.Delegation.Revision - 1, Digest: Digest("mcp-connect-declared")}
	preparedID, err := runtimeports.DerivePreparedProviderAttemptIDV2(declared.ID, intent.Attempt.PermitID, intent.Attempt.AttemptID)
	if err != nil {
		panic(err)
	}
	prepared, err := runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{ID: preparedID, Revision: 1, DeclaredDelegation: declared, OperationDigest: intent.OperationDigest, IntentID: intent.EffectID, IntentRevision: intent.Attempt.IntentRevision, IntentDigest: intent.IntentDigest, PermitID: intent.Attempt.PermitID, PermitRevision: intent.Attempt.PermitRevision, PermitDigest: intent.Attempt.PermitDigest, AttemptID: intent.Attempt.AttemptID, Provider: intent.Provider, PayloadSchema: Schema("mcp-connect-intent"), PayloadDigest: intent.Ref.Digest, PayloadRevision: 1, PreparedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		panic(err)
	}
	enforcement := BoundaryFixture(now).Enforcement
	declaration := runtimeports.ControlledOperationProviderRouteDeclarationRefV2{RouteID: "mcp-connect-route-testkit", Revision: 1, PublisherComponentID: "praxis.harness/assembly", DeclarationDigest: Digest("mcp-connect-declaration")}
	conformance := runtimeports.ControlledOperationProviderRouteConformanceRefV2{ConformanceID: "mcp-connect-conformance-testkit", Revision: 1, DeclarationRef: declaration, ConformanceDigest: Digest("mcp-connect-conformance")}
	route, err := runtimeports.SealControlledMCPConnectRouteCurrentProjectionV1(runtimeports.ControlledMCPConnectRouteCurrentProjectionV1{Ref: runtimeports.ControlledMCPConnectRouteCurrentRefV1{Revision: 1, DeclarationRef: declaration, ConformanceRef: conformance}, Generation: runtimeports.GenerationArtifactRefV1{ID: "mcp-connect-generation", Revision: 1, Digest: Digest("mcp-connect-generation"), InputDigest: Digest("mcp-connect-generation-input"), ManifestDigest: Digest("mcp-connect-generation-manifest"), GraphDigest: Digest("mcp-connect-generation-graph"), CatalogDigest: Digest("mcp-connect-generation-catalog")}, Assembly: runtimeports.GenerationBindingAssociationRefV1{ID: "mcp-connect-assembly", Revision: 1, Digest: Digest("mcp-connect-assembly")}, HandoffID: "mcp-connect-route-handoff", HandoffRevision: 1, HandoffDigest: Digest("mcp-connect-route-handoff"), BindingSetID: intent.Provider.BindingSetID, BindingSetRevision: intent.Provider.BindingSetRevision, BindingSetDigest: Digest("mcp-connect-binding-set"), BindingSetSemanticDigest: Digest("mcp-connect-binding-semantic"), BindingSetCurrentnessDigest: Digest("mcp-connect-binding-current"), ActiveRouteID: "mcp-connect-active-route", ActiveRouteRevision: 1, ActiveRouteDigest: Digest("mcp-connect-active-route"), ProviderTransport: intent.ProviderTransport, Provider: intent.Provider, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		panic(err)
	}
	association, err := runtimeports.SealPreparedDomainCommandAssociationCurrentProjectionV1(runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1{Operation: intent.Operation, OperationDigest: intent.OperationDigest, EffectID: intent.EffectID, EffectRevision: intent.Attempt.IntentRevision, IntentDigest: intent.IntentDigest, Prepared: prepared, Attempt: intent.Attempt, Provider: intent.Provider, PayloadSchema: prepared.PayloadSchema, PayloadDigest: prepared.PayloadDigest, PayloadRevision: prepared.PayloadRevision, DomainCommand: intent.RuntimeDomainCommandRefV1(), CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		panic(err)
	}
	record := runtimeports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: Digest("mcp-connect-ledger"), Sequence: 1, RecordDigest: Digest("mcp-connect-record")}
	prepareConsumption := runtimeports.OperationScopeEvidenceConsumptionRefV3{ID: "mcp-connect-prepare-consumption", Revision: 1, Digest: Digest("mcp-connect-prepare-consumption"), Record: record}
	qualification := runtimeports.OperationScopeEvidenceQualificationRefV3{ID: "mcp-connect-execute-qualification", Revision: 1, Digest: Digest("mcp-connect-execute-qualification"), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}
	handoff, err := runtimeports.SealOperationScopeEvidenceProviderHandoffFactV3(runtimeports.OperationScopeEvidenceProviderHandoffFactV3{ID: "mcp-connect-execute-handoff", Revision: 1, Qualification: qualification, Phase: enforcement, CheckedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		panic(err)
	}
	authorization, err := runtimeports.SealControlledMCPConnectPhysicalAuthorizationV1(runtimeports.ControlledMCPConnectPhysicalAuthorizationV1{UnifiedNotAfterUnixNano: now.Add(8 * time.Second).UnixNano(), Route: route.Ref, ProviderTransport: intent.ProviderTransport, Provider: intent.Provider, Operation: intent.Operation, OperationDigest: intent.OperationDigest, OperationScopeDigest: intent.Operation.ExecutionScopeDigest, EffectID: intent.EffectID, EffectRevision: intent.Attempt.IntentRevision, EffectFactRevision: intent.EffectRevision, IntentDigest: intent.IntentDigest, Prepared: prepared, Attempt: intent.Attempt, ExecuteEnforcement: enforcement, PrepareConsumption: prepareConsumption, ExecuteHandoff: handoff.RefV3(), SandboxProjectionDigest: Digest("mcp-connect-sandbox-projection"), CredentialFactsDigest: Digest("mcp-connect-credential-facts"), Association: association.Ref, DomainCommand: intent.RuntimeDomainCommandRefV1(), IssuedUnixNano: now.Add(-time.Second).UnixNano()})
	if err != nil {
		panic(err)
	}
	return MCPConnectControlledFixtureV1{Connect: connect, Authorization: authorization}
}

func MCPConnectReceiptV1(f MCPConnectControlledFixtureV1, response []byte, observed time.Time) toolcontract.MCPConnectProtocolReceiptV1 {
	admission, err := runtimeports.SealControlledOperationProviderAdmissionReceiptRefV2(runtimeports.ControlledOperationProviderAdmissionReceiptRefV2{ID: "mcp-connect-admission-testkit", Revision: 1, StableKeyDigest: f.Authorization.StableKeyDigest, Admitted: true})
	if err != nil {
		panic(err)
	}
	receipt, err := toolcontract.SealMCPConnectProtocolReceiptV1(toolcontract.MCPConnectProtocolReceiptV1{Intent: f.Connect.Intent.Ref, TransportConfig: f.Connect.Config.Ref, StableKeyDigest: f.Authorization.StableKeyDigest, AdmissionReceipt: admission, TransportKind: f.Connect.Config.Kind, NegotiatedProtocol: toolcontract.MCPStableProtocolVersion, ProviderSessionID: "mcp-provider-session-testkit", InitializeResponse: append([]byte(nil), response...), ObservedUnixNano: observed.UnixNano()})
	if err != nil {
		panic(err)
	}
	return receipt
}
