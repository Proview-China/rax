package api_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestMCPReadAPIV1ExactServerAndProcessObservation(t *testing.T) {
	serverRepository := mcp.NewInMemoryMCPServerDescriptorRepositoryV1()
	serverSDK, err := sdk.NewMCPServerRegistryV1(serverRepository, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	server := testkit.MCPServer()
	if _, err = serverSDK.RegisterMCPServerV1(context.Background(), server, nil); err != nil {
		t.Fatal(err)
	}
	journal, _ := mcp.NewInMemoryMCPProcessObservationJournalV1(func() time.Time { return testkit.FixedTime })
	connection := testkit.MCPConnection()
	snapshotValue := testkit.MCPSnapshot()
	snapshot := toolcontract.ObjectRef{ID: snapshotValue.ID, Revision: snapshotValue.Revision, Digest: snapshotValue.Digest}
	observation, err := journal.RecordMCPProcessObservationV1(context.Background(), connection, snapshot, toolcontract.MCPProcessObservationInputV1{Kind: toolcontract.MCPProcessProgressV1, CorrelationDigest: testkit.Digest("api-process-correlation"), PayloadDigest: testkit.Digest("api-process-payload"), Progress: 1, Total: 2})
	if err != nil {
		t.Fatal(err)
	}
	read, err := api.NewMCPReadV1(serverSDK, journal)
	if err != nil {
		t.Fatal(err)
	}
	exact := toolcontract.ObjectRef{ID: server.ID, Revision: server.Revision, Digest: server.Digest}
	gotServer, err := read.InspectMCPServerV1(context.Background(), exact)
	if err != nil || gotServer.Digest != server.Digest {
		t.Fatalf("server=%+v err=%v", gotServer, err)
	}
	gotServer.Transports[0] = "praxis.test/tampered"
	current, err := read.InspectCurrentMCPServerV1(context.Background(), server.ID)
	if err != nil || current.Transports[0] != server.Transports[0] {
		t.Fatalf("current=%+v err=%v", current, err)
	}
	gotObservation, err := read.InspectMCPProcessObservationV1(context.Background(), observation.Ref)
	if err != nil || gotObservation.Ref != observation.Ref {
		t.Fatalf("observation=%+v err=%v", gotObservation, err)
	}
	pageRequest := toolcontract.MCPProcessObservationPageRequestV1{
		Connection:      toolcontract.ObjectRef{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest},
		ConnectionEpoch: connection.Epoch, Snapshot: snapshot, Limit: 10,
	}
	page, err := read.ReadMCPProcessObservationPageV1(context.Background(), pageRequest)
	if err != nil || page.Validate() != nil || len(page.Observations) != 1 || page.Observations[0].Ref != observation.Ref {
		t.Fatalf("process page=%+v err=%v", page, err)
	}
	page.Observations[0].PayloadDigest = testkit.Digest("api-page-copy-tamper")
	again, err := read.ReadMCPProcessObservationPageV1(context.Background(), pageRequest)
	if err != nil || again.Observations[0].PayloadDigest != observation.PayloadDigest {
		t.Fatalf("process page aliased journal: %+v err=%v", again, err)
	}
}

func TestMCPReadAPIV1FailClosedDependenciesContextAndDigest(t *testing.T) {
	var nilServers *sdk.MCPServerRegistryV1
	var nilProcess *mcp.InMemoryMCPProcessObservationJournalV1
	if _, err := api.NewMCPReadV1(nilServers, nilProcess); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil error=%v", err)
	}
	serverRepository := mcp.NewInMemoryMCPServerDescriptorRepositoryV1()
	serverSDK, _ := sdk.NewMCPServerRegistryV1(serverRepository, func() time.Time { return testkit.FixedTime })
	journal, _ := mcp.NewInMemoryMCPProcessObservationJournalV1(func() time.Time { return testkit.FixedTime })
	read, _ := api.NewMCPReadV1(serverSDK, journal)
	server := testkit.MCPServer()
	if _, err := serverSDK.RegisterMCPServerV1(context.Background(), server, nil); err != nil {
		t.Fatal(err)
	}
	exact := toolcontract.ObjectRef{ID: server.ID, Revision: server.Revision, Digest: server.Digest}
	if _, err := read.InspectMCPServerV1(nil, exact); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := read.InspectMCPServerV1(ctx, exact); err != context.Canceled {
		t.Fatalf("canceled context error=%v", err)
	}
	exact.Digest = testkit.Digest("wrong-api-server")
	if _, err := read.InspectMCPServerV1(context.Background(), exact); err == nil {
		t.Fatal("wrong exact Server digest was accepted")
	}
}
