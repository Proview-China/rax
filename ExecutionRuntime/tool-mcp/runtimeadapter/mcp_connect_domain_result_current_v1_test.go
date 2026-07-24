package runtimeadapter_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/runtimeadapter"
)

func TestMCPConnectDomainResultCurrentAdapterV1ExactProjection(t *testing.T) {
	now := time.Now().UTC()
	fact, current, owner := mcpConnectDomainResultAdapterFixtureV1(t, now)
	source := &mcpConnectDomainResultSourceV1{fact: fact, current: current}
	adapter, err := runtimeadapter.NewMCPConnectDomainResultCurrentAdapterV1(source, owner, func() time.Time { return now }, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	exact := runtimeadapter.MCPConnectDomainResultRuntimeRefV1(owner, fact)
	projection, err := adapter.InspectOperationSettlementDomainResultCurrentV4(context.Background(), toolcontract.MCPConnectEffectKindV1, exact)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Fact != exact || projection.EffectKind != toolcontract.MCPConnectEffectKindV1 || projection.CheckedUnixNano != current.CheckedUnixNano || projection.ExpiresUnixNano != current.ExpiresUnixNano {
		t.Fatalf("Runtime DomainResult projection lost exact Tool fact: %#v", projection)
	}
}

func TestMCPConnectDomainResultCurrentAdapterV1FailsClosed(t *testing.T) {
	now := time.Now().UTC()
	fact, current, owner := mcpConnectDomainResultAdapterFixtureV1(t, now)

	t.Run("typed_nil", func(t *testing.T) {
		var source *mcpConnectDomainResultSourceV1
		if _, err := runtimeadapter.NewMCPConnectDomainResultCurrentAdapterV1(source, owner, func() time.Time { return now }, time.Second); err == nil {
			t.Fatal("typed-nil DomainResult source was accepted")
		}
	})

	t.Run("nil_context", func(t *testing.T) {
		adapter, _ := runtimeadapter.NewMCPConnectDomainResultCurrentAdapterV1(&mcpConnectDomainResultSourceV1{fact: fact, current: current}, owner, func() time.Time { return now }, 5*time.Second)
		exact := runtimeadapter.MCPConnectDomainResultRuntimeRefV1(owner, fact)
		if _, err := adapter.InspectOperationSettlementDomainResultCurrentV4(nil, toolcontract.MCPConnectEffectKindV1, exact); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("nil context error=%v", err)
		}
	})

	t.Run("owner_drift", func(t *testing.T) {
		adapter, _ := runtimeadapter.NewMCPConnectDomainResultCurrentAdapterV1(&mcpConnectDomainResultSourceV1{fact: fact, current: current}, owner, func() time.Time { return now }, 5*time.Second)
		exact := runtimeadapter.MCPConnectDomainResultRuntimeRefV1(owner, fact)
		exact.Owner.ManifestDigest = testkit.Digest("another-owner")
		if _, err := adapter.InspectOperationSettlementDomainResultCurrentV4(context.Background(), toolcontract.MCPConnectEffectKindV1, exact); !core.HasReason(err, core.ReasonSettlementOwnerMismatch) {
			t.Fatalf("owner drift error=%v", err)
		}
	})

	t.Run("current_drift", func(t *testing.T) {
		drifted := current
		drifted.Connection.Digest = testkit.Digest("another-connection")
		drifted.Digest = ""
		drifted, _ = toolcontract.SealMCPConnectDomainResultCurrentProjectionV1(drifted, now)
		adapter, _ := runtimeadapter.NewMCPConnectDomainResultCurrentAdapterV1(&mcpConnectDomainResultSourceV1{fact: fact, current: drifted}, owner, func() time.Time { return now }, 5*time.Second)
		exact := runtimeadapter.MCPConnectDomainResultRuntimeRefV1(owner, fact)
		if _, err := adapter.InspectOperationSettlementDomainResultCurrentV4(context.Background(), toolcontract.MCPConnectEffectKindV1, exact); !core.HasReason(err, core.ReasonSettlementOwnerMismatch) {
			t.Fatalf("current drift error=%v", err)
		}
	})
}

func mcpConnectDomainResultAdapterFixtureV1(t *testing.T, now time.Time) (toolcontract.MCPConnectDomainResultFactV1, toolcontract.MCPConnectDomainResultCurrentProjectionV1, runtimeports.ProviderBindingRefV2) {
	t.Helper()
	fixture := testkit.MCPConnectControlledV1(now, toolcontract.MCPTransportStreamableHTTPV1)
	receipt := testkit.MCPConnectReceiptV1(fixture, []byte(`{"protocolVersion":"2025-03-26"}`), now)
	connection, err := toolcontract.SealMCPConnectionFactV2(toolcontract.MCPConnectionFactV2{Owner: fixture.Connect.Intent.Owner, Coordinate: fixture.Connect.Intent.Coordinate, Intent: fixture.Connect.Intent.Ref, TransportConfig: fixture.Connect.Config.Ref, Server: fixture.Connect.Intent.Server, ProtocolReceipt: receipt.Ref, ProviderTransport: fixture.Connect.Intent.ProviderTransport, Provider: fixture.Connect.Intent.Provider, NegotiatedProtocol: receipt.NegotiatedProtocol, ProviderSessionID: receipt.ProviderSessionID, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	observation := runtimeports.ProviderAttemptObservationRefV2{Delegation: *fixture.Authorization.Attempt.Delegation, PreparedAttemptID: fixture.Authorization.Prepared.ID, ProviderOperationRef: receipt.Ref.ID, Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Digest: testkit.Digest("mcp-connect-domain-observation"), PayloadDigest: receipt.ResponseDigest, PayloadRevision: 1, SourceRegistrationID: "mcp-connect-domain-source", SourceEpoch: 1, SourceSequence: 1, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: testkit.Digest("mcp-connect-domain-ledger"), Sequence: 1, RecordDigest: testkit.Digest("mcp-connect-domain-record")}, ObservedUnixNano: now.UnixNano()}
	if err = observation.Validate(); err != nil {
		t.Fatal(err)
	}
	prepare := fixture.Authorization.ExecuteEnforcement
	prepare.Phase = runtimeports.OperationDispatchEnforcementPrepareV4
	prepare.ReceiptDigest = fixture.Authorization.ExecuteEnforcement.PrepareReceiptDigest
	prepare.JournalRevision = 1
	prepare.PrepareReceiptDigest = ""
	prepare.PreparedAttemptDigest = ""
	prepareRecord := runtimeports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: testkit.Digest("mcp-connect-prepare-ledger"), Sequence: 1, RecordDigest: testkit.Digest("mcp-connect-prepare-record")}
	executeRecord := runtimeports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: testkit.Digest("mcp-connect-execute-ledger"), Sequence: 1, RecordDigest: testkit.Digest("mcp-connect-execute-record")}
	prepareConsumption := runtimeports.OperationScopeEvidenceConsumptionRefV3{ID: "mcp-connect-prepare-consumption-adapter", Revision: 1, Digest: testkit.Digest("mcp-connect-prepare-consumption-adapter"), Record: prepareRecord}
	executeConsumption := runtimeports.OperationScopeEvidenceConsumptionRefV3{ID: "mcp-connect-execute-consumption-adapter", Revision: 1, Digest: testkit.Digest("mcp-connect-execute-consumption-adapter"), Record: executeRecord}
	schema := runtimeports.SchemaRefV2{Namespace: "praxis.tool-mcp", Name: "mcp-connection-fact", Version: "2.0.0", MediaType: "application/json", ContentDigest: testkit.Digest("mcp-connect-domain-schema")}
	fact, err := toolcontract.SealMCPConnectDomainResultFactV1(toolcontract.MCPConnectDomainResultFactV1{TenantID: fixture.Connect.Intent.Operation.ExecutionScope.Identity.TenantID, Operation: fixture.Connect.Intent.Operation, OperationScopeDigest: fixture.Connect.Intent.Operation.ExecutionScopeDigest, Connection: connection.Ref, Intent: fixture.Connect.Intent.Ref, ProtocolReceipt: receipt.Ref, PreparedAttempt: fixture.Authorization.Prepared, Attempt: fixture.Authorization.Attempt, Observation: observation, PrepareEnforcement: prepare, ExecuteEnforcement: fixture.Authorization.ExecuteEnforcement, PrepareConsumption: prepareConsumption, ExecuteConsumption: executeConsumption, Schema: schema, PayloadDigest: connection.Ref.Digest, PayloadRevision: 1, Owner: fixture.Connect.Intent.Owner, Outcome: toolcontract.ToolOutcomeSucceededV2, Disposition: toolcontract.ToolDispositionConfirmedAppliedV2, CreatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	current, err := toolcontract.SealMCPConnectDomainResultCurrentProjectionV1(toolcontract.MCPConnectDomainResultCurrentProjectionV1{Fact: fact.ObjectRef(), Connection: fact.Connection, Observation: fact.Observation, PrepareEnforcement: fact.PrepareEnforcement, ExecuteEnforcement: fact.ExecuteEnforcement, PrepareConsumption: fact.PrepareConsumption, ExecuteConsumption: fact.ExecuteConsumption, Owner: fact.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	return fact, current, fixture.Connect.Intent.Provider
}

type mcpConnectDomainResultSourceV1 struct {
	fact    toolcontract.MCPConnectDomainResultFactV1
	current toolcontract.MCPConnectDomainResultCurrentProjectionV1
}

func (s *mcpConnectDomainResultSourceV1) InspectMCPConnectDomainResultV1(_ context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPConnectDomainResultFactV1, error) {
	if exact != s.fact.ObjectRef() {
		return toolcontract.MCPConnectDomainResultFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Connect DomainResult not found")
	}
	return s.fact, nil
}

func (s *mcpConnectDomainResultSourceV1) InspectCurrentMCPConnectDomainResultV1(_ context.Context, exact toolcontract.ObjectRef, _ time.Duration) (toolcontract.MCPConnectDomainResultCurrentProjectionV1, error) {
	if exact != s.fact.ObjectRef() {
		return toolcontract.MCPConnectDomainResultCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Connect DomainResult current not found")
	}
	return s.current, nil
}

var _ runtimeports.OperationSettlementDomainResultCurrentReaderV4 = (*runtimeadapter.MCPConnectDomainResultCurrentAdapterV1)(nil)
