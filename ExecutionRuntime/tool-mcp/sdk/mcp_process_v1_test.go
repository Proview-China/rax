package sdk_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestMCPProcessSDKV1ExactAndBoundedPage(t *testing.T) {
	journal, _ := mcp.NewInMemoryMCPProcessObservationJournalV1(func() time.Time { return testkit.FixedTime })
	connection := testkit.MCPConnection()
	snapshotValue := testkit.MCPSnapshot()
	snapshot := toolcontract.ObjectRef{ID: snapshotValue.ID, Revision: snapshotValue.Revision, Digest: snapshotValue.Digest}
	observation, err := journal.RecordMCPProcessObservationV1(context.Background(), connection, snapshot, toolcontract.MCPProcessObservationInputV1{
		Kind: toolcontract.MCPProcessProgressV1, CorrelationDigest: testkit.Digest("sdk-process-correlation"), PayloadDigest: testkit.Digest("sdk-process-payload"), Progress: 1, Total: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := sdk.NewMCPProcessV1(journal)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.InspectMCPProcessObservationV1(context.Background(), observation.Ref)
	if err != nil || got.Ref != observation.Ref {
		t.Fatalf("exact process Observation=%+v err=%v", got, err)
	}
	request := toolcontract.MCPProcessObservationPageRequestV1{
		Connection:      toolcontract.ObjectRef{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest},
		ConnectionEpoch: connection.Epoch, Snapshot: snapshot, Limit: 10,
	}
	page, err := client.ReadMCPProcessObservationPageV1(context.Background(), request)
	if err != nil || page.Validate() != nil || len(page.Observations) != 1 {
		t.Fatalf("SDK process page=%+v err=%v", page, err)
	}
	page.Observations[0].PayloadDigest = testkit.Digest("sdk-process-tamper")
	again, err := client.ReadMCPProcessObservationPageV1(context.Background(), request)
	if err != nil || again.Observations[0].PayloadDigest != observation.PayloadDigest {
		t.Fatalf("SDK process page aliased source: %+v err=%v", again, err)
	}
}

func TestMCPProcessSDKV1FailClosed(t *testing.T) {
	var typedNil *mcp.InMemoryMCPProcessObservationJournalV1
	if _, err := sdk.NewMCPProcessV1(typedNil); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil process Reader error=%v", err)
	}
	journal, _ := mcp.NewInMemoryMCPProcessObservationJournalV1(func() time.Time { return testkit.FixedTime })
	client, _ := sdk.NewMCPProcessV1(journal)
	if _, err := client.ReadMCPProcessObservationPageV1(nil, toolcontract.MCPProcessObservationPageRequestV1{}); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil process context error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.ReadMCPProcessObservationPageV1(ctx, toolcontract.MCPProcessObservationPageRequestV1{}); err != context.Canceled {
		t.Fatalf("canceled process context error=%v", err)
	}
}
