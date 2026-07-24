package runtimeadapter_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	toolmcp "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/runtimeadapter"
)

func TestMCPConnectReceiptObservationReaderV1ExactProjection(t *testing.T) {
	now := time.Now().UTC()
	fixture := testkit.MCPConnectControlledV1(now, toolcontract.MCPTransportStreamableHTTPV1)
	receipt := testkit.MCPConnectReceiptV1(fixture, []byte(`{"protocolVersion":"2025-03-26"}`), now)
	entry := toolmcp.MCPConnectPhysicalEntryV1{ID: "mcp-connect-entry-test", Revision: 2, StableKeyDigest: fixture.Authorization.StableKeyDigest, AuthorizationDigest: fixture.Authorization.Digest, Authorization: fixture.Authorization, Intent: fixture.Connect.Intent.Ref, TransportConfig: fixture.Connect.Config.Ref, AdmissionReceipt: receipt.AdmissionReceipt, NotAfterUnixNano: fixture.Authorization.UnifiedNotAfterUnixNano, State: toolmcp.MCPConnectPhysicalObservedV1, ProtocolReceipt: &receipt, UpdatedUnixNano: now.UnixNano()}
	reader, err := runtimeadapter.NewMCPConnectReceiptObservationReaderV1(connectReceiptReaderV1{receipt}, connectEntryReaderV1{entry}, fixture.Connect.Intent.Owner, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	exact, err := runtimeadapter.MCPConnectProtocolReceiptRuntimeRefV1(fixture.Connect.Intent.Owner, receipt.Ref)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := reader.InspectOperationProviderReceiptV1(context.Background(), exact)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Ref != exact || projection.Operation != fixture.Authorization.Operation || projection.Prepared != fixture.Authorization.Prepared || projection.Attempt != fixture.Authorization.Attempt || projection.Provider != fixture.Authorization.Provider || projection.Payload.ContentDigest != receipt.ResponseDigest || len(projection.Payload.Inline) != 0 {
		t.Fatalf("Runtime receipt projection lost exact closure or exposed raw bytes: %#v", projection)
	}
}

func TestMCPConnectReceiptObservationReaderV1FailsClosed(t *testing.T) {
	now := time.Now().UTC()
	fixture := testkit.MCPConnectControlledV1(now, toolcontract.MCPTransportStreamableHTTPV1)
	receipt := testkit.MCPConnectReceiptV1(fixture, []byte(`{"protocolVersion":"2025-03-26"}`), now)
	entry := toolmcp.MCPConnectPhysicalEntryV1{ID: "mcp-connect-entry-test", Revision: 2, StableKeyDigest: fixture.Authorization.StableKeyDigest, AuthorizationDigest: fixture.Authorization.Digest, Authorization: fixture.Authorization, Intent: fixture.Connect.Intent.Ref, TransportConfig: fixture.Connect.Config.Ref, AdmissionReceipt: receipt.AdmissionReceipt, NotAfterUnixNano: fixture.Authorization.UnifiedNotAfterUnixNano, State: toolmcp.MCPConnectPhysicalObservedV1, ProtocolReceipt: &receipt, UpdatedUnixNano: now.UnixNano()}

	t.Run("typed_nil", func(t *testing.T) {
		var receipts *connectReceiptReaderV1
		if _, err := runtimeadapter.NewMCPConnectReceiptObservationReaderV1(receipts, connectEntryReaderV1{entry}, fixture.Connect.Intent.Owner, func() time.Time { return now }); err == nil {
			t.Fatal("typed-nil receipt reader was accepted")
		}
	})

	t.Run("nil_context", func(t *testing.T) {
		reader, _ := runtimeadapter.NewMCPConnectReceiptObservationReaderV1(connectReceiptReaderV1{receipt}, connectEntryReaderV1{entry}, fixture.Connect.Intent.Owner, func() time.Time { return now })
		exact, _ := runtimeadapter.MCPConnectProtocolReceiptRuntimeRefV1(fixture.Connect.Intent.Owner, receipt.Ref)
		if _, err := reader.InspectOperationProviderReceiptV1(nil, exact); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("nil context error=%v", err)
		}
	})

	t.Run("authorization_drift", func(t *testing.T) {
		drifted := entry
		drifted.AuthorizationDigest = testkit.Digest("another-authorization")
		reader, _ := runtimeadapter.NewMCPConnectReceiptObservationReaderV1(connectReceiptReaderV1{receipt}, connectEntryReaderV1{drifted}, fixture.Connect.Intent.Owner, func() time.Time { return now })
		exact, _ := runtimeadapter.MCPConnectProtocolReceiptRuntimeRefV1(fixture.Connect.Intent.Owner, receipt.Ref)
		if _, err := reader.InspectOperationProviderReceiptV1(context.Background(), exact); !core.HasReason(err, core.ReasonEvidenceConflict) {
			t.Fatalf("authorization drift error=%v", err)
		}
	})
}

type connectReceiptReaderV1 struct {
	receipt toolcontract.MCPConnectProtocolReceiptV1
}

func (r connectReceiptReaderV1) InspectMCPConnectProtocolReceiptV1(_ context.Context, exact toolcontract.MCPConnectProtocolReceiptRefV1) (toolcontract.MCPConnectProtocolReceiptV1, error) {
	if exact != r.receipt.Ref {
		return toolcontract.MCPConnectProtocolReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "receipt not found")
	}
	return toolcontract.CloneMCPConnectProtocolReceiptV1(r.receipt), nil
}

type connectEntryReaderV1 struct {
	entry toolmcp.MCPConnectPhysicalEntryV1
}

func (r connectEntryReaderV1) InspectMCPConnectPhysicalV1(_ context.Context, stable core.Digest) (toolmcp.MCPConnectPhysicalEntryV1, error) {
	if stable != r.entry.StableKeyDigest {
		return toolmcp.MCPConnectPhysicalEntryV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "entry not found")
	}
	return r.entry, nil
}

var _ runtimeports.OperationProviderReceiptReaderV1 = (*runtimeadapter.MCPConnectReceiptObservationReaderV1)(nil)
