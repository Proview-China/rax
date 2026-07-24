package api

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestMCPConnectReadV1ExactFactsAndDeepCopy(t *testing.T) {
	now := time.Now().UTC()
	source := mcpConnectReadFixtureV1(t, now)
	api, err := NewMCPConnectReadV1(source, source, source, source, source, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if value, err := api.InspectMCPConnectIntentV1(context.Background(), source.intent.Ref); err != nil || value.Ref != source.intent.Ref {
		t.Fatalf("Intent Inspect=%#v err=%v", value, err)
	}
	receipt, err := api.InspectMCPConnectProtocolReceiptV1(context.Background(), source.receipt.Ref)
	if err != nil {
		t.Fatal(err)
	}
	receipt.InitializeResponse[0] ^= 0xff
	again, err := api.InspectMCPConnectProtocolReceiptV1(context.Background(), source.receipt.Ref)
	if err != nil || again.InitializeResponse[0] != source.receipt.InitializeResponse[0] {
		t.Fatal("MCP Connect read API leaked mutable Receipt bytes")
	}
	if value, err := api.InspectMCPConnectionFactV2(context.Background(), source.connection.Ref); err != nil || value.Ref != source.connection.Ref {
		t.Fatalf("Connection Inspect=%#v err=%v", value, err)
	}
	if value, err := api.InspectMCPConnectDomainResultV1(context.Background(), source.domain.ObjectRef()); err != nil || value.ObjectRef() != source.domain.ObjectRef() {
		t.Fatalf("DomainResult Inspect=%#v err=%v", value, err)
	}
	if value, err := api.InspectMCPConnectApplySettlementV1(context.Background(), source.apply.Ref); err != nil || value.Ref != source.apply.Ref {
		t.Fatalf("ApplySettlement Inspect=%#v err=%v", value, err)
	}
	if value, err := api.InspectCurrentMCPConnectionAvailabilityV1(context.Background(), source.connection.Ref, 5*time.Second); err != nil || value.Connection != source.connection.Ref {
		t.Fatalf("Availability Inspect=%#v err=%v", value, err)
	}
}

func TestMCPConnectReadV1FailsClosed(t *testing.T) {
	now := time.Now().UTC()
	source := mcpConnectReadFixtureV1(t, now)
	var typedNil *mcpConnectReadSourceV1
	if _, err := NewMCPConnectReadV1(typedNil, source, source, source, source, func() time.Time { return now }); err == nil {
		t.Fatal("typed-nil MCP Connect reader was accepted")
	}
	api, _ := NewMCPConnectReadV1(source, source, source, source, source, func() time.Time { return now })
	if _, err := api.InspectMCPConnectIntentV1(nil, source.intent.Ref); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
	wrong := source.connection.Ref
	wrong.Digest = testkit.Digest("another-connection")
	if _, err := api.InspectMCPConnectionFactV2(context.Background(), wrong); err == nil {
		t.Fatal("wrong exact Connection Ref was accepted")
	}
	source.availability.Connection.Digest = testkit.Digest("drifted-availability")
	if _, err := api.InspectCurrentMCPConnectionAvailabilityV1(context.Background(), source.connection.Ref, time.Second); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("availability drift error=%v", err)
	}
}

type mcpConnectReadSourceV1 struct {
	intent       toolcontract.MCPConnectIntentV1
	receipt      toolcontract.MCPConnectProtocolReceiptV1
	connection   toolcontract.MCPConnectionFactV2
	domain       toolcontract.MCPConnectDomainResultFactV1
	apply        toolcontract.MCPConnectApplySettlementFactV1
	availability toolcontract.MCPConnectionAvailabilityCurrentProjectionV1
}

func mcpConnectReadFixtureV1(t *testing.T, now time.Time) *mcpConnectReadSourceV1 {
	t.Helper()
	fixture := testkit.MCPConnectControlledV1(now, toolcontract.MCPTransportStreamableHTTPV1)
	receipt := testkit.MCPConnectReceiptV1(fixture, []byte(`{"protocolVersion":"2025-03-26"}`), now)
	connection, err := toolcontract.SealMCPConnectionFactV2(toolcontract.MCPConnectionFactV2{Owner: fixture.Connect.Intent.Owner, Coordinate: fixture.Connect.Intent.Coordinate, Intent: fixture.Connect.Intent.Ref, TransportConfig: fixture.Connect.Config.Ref, Server: fixture.Connect.Intent.Server, ProtocolReceipt: receipt.Ref, ProviderTransport: fixture.Connect.Intent.ProviderTransport, Provider: fixture.Connect.Intent.Provider, NegotiatedProtocol: receipt.NegotiatedProtocol, ProviderSessionID: receipt.ProviderSessionID, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	observation := runtimeports.ProviderAttemptObservationRefV2{Delegation: *fixture.Authorization.Attempt.Delegation, PreparedAttemptID: fixture.Authorization.Prepared.ID, ProviderOperationRef: receipt.Ref.ID, Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Digest: testkit.Digest("mcp-connect-read-observation"), PayloadDigest: receipt.ResponseDigest, PayloadRevision: 1, SourceRegistrationID: "mcp-connect-read-source", SourceEpoch: 1, SourceSequence: 1, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: testkit.Digest("mcp-connect-read-ledger"), Sequence: 1, RecordDigest: testkit.Digest("mcp-connect-read-record")}, ObservedUnixNano: now.UnixNano()}
	prepare := fixture.Authorization.ExecuteEnforcement
	prepare.Phase, prepare.ReceiptDigest, prepare.JournalRevision = runtimeports.OperationDispatchEnforcementPrepareV4, fixture.Authorization.ExecuteEnforcement.PrepareReceiptDigest, 1
	prepare.PrepareReceiptDigest, prepare.PreparedAttemptDigest = "", ""
	consumption := func(label string, sequence uint64) runtimeports.OperationScopeEvidenceConsumptionRefV3 {
		return runtimeports.OperationScopeEvidenceConsumptionRefV3{ID: "mcp-connect-read-" + label, Revision: 1, Digest: testkit.Digest("mcp-connect-read-" + label), Record: runtimeports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: testkit.Digest("mcp-connect-read-ledger-" + label), Sequence: sequence, RecordDigest: testkit.Digest("mcp-connect-read-record-" + label)}}
	}
	domain, err := toolcontract.SealMCPConnectDomainResultFactV1(toolcontract.MCPConnectDomainResultFactV1{TenantID: fixture.Connect.Intent.Operation.ExecutionScope.Identity.TenantID, Operation: fixture.Connect.Intent.Operation, OperationScopeDigest: fixture.Connect.Intent.Operation.ExecutionScopeDigest, Connection: connection.Ref, Intent: fixture.Connect.Intent.Ref, ProtocolReceipt: receipt.Ref, PreparedAttempt: fixture.Authorization.Prepared, Attempt: fixture.Authorization.Attempt, Observation: observation, PrepareEnforcement: prepare, ExecuteEnforcement: fixture.Authorization.ExecuteEnforcement, PrepareConsumption: consumption("prepare", 1), ExecuteConsumption: consumption("execute", 2), Schema: testkit.Schema("mcp-connect-read-domain"), PayloadDigest: connection.Ref.Digest, PayloadRevision: 1, Owner: fixture.Connect.Intent.Owner, Outcome: toolcontract.ToolOutcomeSucceededV2, Disposition: toolcontract.ToolDispositionConfirmedAppliedV2, CreatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	runtimeDomain := runtimeports.OperationSettlementDomainResultFactRefV4{Owner: fixture.Connect.Intent.Provider, Kind: toolcontract.MCPConnectDomainResultRuntimeKindV1, ID: domain.ID, Revision: domain.Revision, Digest: domain.Digest, TenantID: domain.TenantID, EffectID: domain.Attempt.EffectID, EffectRevision: domain.Attempt.IntentRevision, Operation: domain.Operation, OperationDigest: domain.Attempt.OperationDigest, Attempt: domain.Attempt, Schema: domain.Schema, PayloadDigest: domain.PayloadDigest, PayloadRevision: domain.PayloadRevision, AuthoritativeTime: domain.CreatedUnixNano}
	settlement := runtimeports.OperationSettlementRefV4{ID: "mcp-connect-read-settlement", Revision: 1, Digest: testkit.Digest("mcp-connect-read-settlement"), OperationDigest: domain.Attempt.OperationDigest, EffectID: domain.Attempt.EffectID, DomainResult: runtimeDomain}
	association := runtimeports.OperationSettlementEvidenceAssociationRefV4{ID: "mcp-connect-read-association", Revision: 1, Digest: testkit.Digest("mcp-connect-read-association"), Settlement: settlement, OperationDigest: settlement.OperationDigest, EffectID: settlement.EffectID}
	guard := runtimeports.OperationSettlementTerminalGuardRefV4{ID: "mcp-connect-read-guard", TenantID: domain.TenantID, EffectID: settlement.EffectID, OperationDigest: settlement.OperationDigest, Revision: 1, Digest: testkit.Digest("mcp-connect-read-guard"), Settlement: settlement}
	projection := runtimeports.OperationSettlementTerminalProjectionRefV4{ID: "mcp-connect-read-projection", Revision: 1, Digest: testkit.Digest("mcp-connect-read-projection"), TenantID: domain.TenantID, OperationDigest: settlement.OperationDigest, EffectID: settlement.EffectID, Settlement: settlement, Association: association, Guard: guard}
	inspection, err := runtimeports.SealOperationInspectionSettlementRefV4(runtimeports.OperationInspectionSettlementRefV4{Settlement: settlement, Association: association, Guard: guard, Projection: projection, DomainResult: runtimeDomain, EffectFactRevision: 2, Owner: domain.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(8 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	apply, err := toolcontract.SealMCPConnectApplySettlementFactV1(toolcontract.MCPConnectApplySettlementFactV1{Connection: connection.Ref, DomainResult: domain.ObjectRef(), Inspection: inspection, Owner: domain.Owner, AppliedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	availability, err := toolcontract.SealMCPConnectionAvailabilityCurrentProjectionV1(toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{Connection: connection.Ref, ApplySettlement: apply.Ref, DomainResult: domain.ObjectRef(), Owner: domain.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	return &mcpConnectReadSourceV1{intent: fixture.Connect.Intent, receipt: receipt, connection: connection, domain: domain, apply: apply, availability: availability}
}

func (s *mcpConnectReadSourceV1) InspectMCPConnectIntentV1(_ context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPConnectIntentV1, error) {
	if exact != s.intent.Ref {
		return toolcontract.MCPConnectIntentV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Intent not found")
	}
	return s.intent, nil
}
func (s *mcpConnectReadSourceV1) InspectMCPConnectProtocolReceiptV1(_ context.Context, exact toolcontract.MCPConnectProtocolReceiptRefV1) (toolcontract.MCPConnectProtocolReceiptV1, error) {
	if exact != s.receipt.Ref {
		return toolcontract.MCPConnectProtocolReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Receipt not found")
	}
	return toolcontract.CloneMCPConnectProtocolReceiptV1(s.receipt), nil
}
func (s *mcpConnectReadSourceV1) InspectMCPConnectionFactV2(_ context.Context, exact toolcontract.MCPConnectionFactRefV2) (toolcontract.MCPConnectionFactV2, error) {
	if exact != s.connection.Ref {
		return toolcontract.MCPConnectionFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Connection not found")
	}
	return s.connection, nil
}
func (s *mcpConnectReadSourceV1) InspectMCPConnectDomainResultV1(_ context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPConnectDomainResultFactV1, error) {
	if exact != s.domain.ObjectRef() {
		return toolcontract.MCPConnectDomainResultFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "DomainResult not found")
	}
	return s.domain, nil
}
func (s *mcpConnectReadSourceV1) InspectMCPConnectApplySettlementV1(_ context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPConnectApplySettlementFactV1, error) {
	if exact != s.apply.Ref {
		return toolcontract.MCPConnectApplySettlementFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "ApplySettlement not found")
	}
	return s.apply, nil
}
func (s *mcpConnectReadSourceV1) InspectCurrentMCPConnectionAvailabilityV1(_ context.Context, _ toolcontract.MCPConnectionFactRefV2, _ time.Duration) (toolcontract.MCPConnectionAvailabilityCurrentProjectionV1, error) {
	return s.availability, nil
}
