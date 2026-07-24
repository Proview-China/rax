package sdk_test

import (
	"context"
	"sync"
	"testing"
	"time"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestMCPStatusSDKV1ExactReadAndDeepCopy(t *testing.T) {
	manager := mcp.NewManager()
	connection := testkit.MCPConnection()
	if _, err := manager.Register(connection, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	client, err := sdk.NewMCPStatusV1(manager, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	exact := toolcontract.ObjectRef{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest}
	record, err := client.InspectMCPConnectionStatusV1(context.Background(), exact)
	if err != nil || record.Connection.Digest != connection.Digest || record.State != mcp.ConnectionRegistered {
		t.Fatalf("exact MCP status read failed: %+v %v", record, err)
	}
	record.Connection.SessionID = "changed"
	again, err := client.InspectMCPConnectionStatusV1(context.Background(), exact)
	if err != nil || again.Connection.SessionID != connection.SessionID {
		t.Fatalf("MCP status result aliased Lifecycle state: %v", err)
	}
}

func TestMCPStatusSDKV1FailsClosed(t *testing.T) {
	manager := mcp.NewManager()
	connection := testkit.MCPConnection()
	if _, err := manager.Register(connection, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	exact := toolcontract.ObjectRef{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest}
	var typedNil *mcp.Manager
	if _, err := sdk.NewMCPStatusV1(typedNil, func() time.Time { return testkit.FixedTime }); err == nil {
		t.Fatal("typed-nil MCP Lifecycle Reader was accepted")
	}
	client, err := sdk.NewMCPStatusV1(manager, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	wrong := exact
	wrong.Digest = testkit.Digest("wrong")
	if _, err := client.InspectMCPConnectionStatusV1(context.Background(), wrong); err == nil {
		t.Fatal("MCP status exact digest drift was accepted")
	}
	if _, err := client.InspectMCPConnectionStatusV1(nil, exact); err == nil {
		t.Fatal("nil MCP status context was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.InspectMCPConnectionStatusV1(ctx, exact); err != context.Canceled {
		t.Fatalf("canceled MCP status context was not preserved: %v", err)
	}

	clock := statusSequenceClockV1(testkit.FixedTime.Add(time.Second), testkit.FixedTime)
	client, _ = sdk.NewMCPStatusV1(manager, clock)
	if _, err := client.InspectMCPConnectionStatusV1(context.Background(), exact); err == nil {
		t.Fatal("MCP status clock rollback was accepted")
	}
}

type driftingMCPStatusReaderV1 struct {
	mu      sync.Mutex
	records []mcp.ConnectionRecord
}

func (r *driftingMCPStatusReaderV1) Inspect(string) (mcp.ConnectionRecord, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.records) == 0 {
		return mcp.ConnectionRecord{}, false
	}
	value := r.records[0]
	if len(r.records) > 1 {
		r.records = r.records[1:]
	}
	return value, true
}

func TestMCPStatusSDKV1RejectsS1S2Drift(t *testing.T) {
	manager := mcp.NewManager()
	connection := testkit.MCPConnection()
	first, err := manager.Register(connection, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Revision++
	second.State = mcp.ConnectionResolving
	reader := &driftingMCPStatusReaderV1{records: []mcp.ConnectionRecord{first, second}}
	client, err := sdk.NewMCPStatusV1(reader, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	exact := toolcontract.ObjectRef{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest}
	if _, err := client.InspectMCPConnectionStatusV1(context.Background(), exact); err == nil {
		t.Fatal("MCP status S1/S2 drift was accepted")
	}
}

func TestMCPStatusSDKV1ConcurrentExactReads(t *testing.T) {
	manager := mcp.NewManager()
	connection := testkit.MCPConnection()
	if _, err := manager.Register(connection, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	client, err := sdk.NewMCPStatusV1(manager, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	exact := toolcontract.ObjectRef{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest}
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.InspectMCPConnectionStatusV1(context.Background(), exact)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func statusSequenceClockV1(values ...time.Time) func() time.Time {
	var mu sync.Mutex
	index := 0
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}
