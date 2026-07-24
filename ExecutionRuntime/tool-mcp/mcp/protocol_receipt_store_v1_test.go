package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestMCPProtocolReceiptStoreExactInspect(t *testing.T) {
	now := testkit.FixedTime
	fixture := testkit.MCPExecutionV1(now)
	store := NewInMemoryMCPPhysicalExecutionStoreV1()
	entry, created, err := store.beginV1(context.Background(), fixture.Authorization, fixture.Command.Ref, now)
	if err != nil || !created {
		t.Fatalf("begin: created=%v err=%v", created, err)
	}
	response := []byte(`{"content":[{"type":"text","text":"ok"}]}`)
	receipt, err := toolcontract.SealMCPProtocolReceiptV1(toolcontract.MCPProtocolReceiptV1{Command: fixture.Command.Ref, StableKeyDigest: fixture.Authorization.StableKeyDigest, AdmissionReceipt: entry.AdmissionReceipt, JSONRPCRequestID: fixture.Command.JSONRPCRequestID, CanonicalResponse: response, ResponseDigest: core.DigestBytes(response), ObservedUnixNano: now.Add(time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.completeV1(context.Background(), fixture.Authorization.StableKeyDigest, receipt, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	got, err := store.InspectMCPProtocolReceiptV1(context.Background(), receipt.Ref)
	if err != nil || got.Ref != receipt.Ref || string(got.CanonicalResponse) != string(receipt.CanonicalResponse) {
		t.Fatalf("exact Inspect drifted: got=%+v err=%v", got.Ref, err)
	}
	drift := receipt.Ref
	drift.Digest = testkit.Digest("wrong-receipt")
	if _, err := store.InspectMCPProtocolReceiptV1(context.Background(), drift); err == nil {
		t.Fatal("same receipt ID with another digest passed")
	}
}
