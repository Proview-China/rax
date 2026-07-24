package mcp_test

import (
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
)

func bindActiveSnapshot(t *testing.T) (*mcp.Manager, contract.MCPConnectionRef, mcp.ConnectionRecord) {
	t.Helper()
	manager := mcp.NewManager()
	connection := testkit.MCPConnection()
	record, err := manager.Register(connection, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []mcp.ConnectionState{mcp.ConnectionResolving, mcp.ConnectionConnecting, mcp.ConnectionInitializing, mcp.ConnectionDiscovering} {
		record, err = manager.Transition(connection.ID, record.Revision, state, time.Unix(0, record.UpdatedUnixNano).Add(time.Nanosecond))
		if err != nil {
			t.Fatal(err)
		}
	}
	record, err = manager.BindSnapshot(connection.ID, record.Revision, testkit.MCPSnapshot(), time.Unix(0, record.UpdatedUnixNano).Add(time.Nanosecond))
	if err != nil {
		t.Fatal(err)
	}
	return manager, connection, record
}

func TestWhiteboxConnectionLifecycleAndSnapshotBinding(t *testing.T) {
	manager := mcp.NewManager()
	connection := testkit.MCPConnection()
	record, err := manager.Register(connection, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []mcp.ConnectionState{mcp.ConnectionResolving, mcp.ConnectionConnecting, mcp.ConnectionInitializing, mcp.ConnectionDiscovering} {
		record, err = manager.Transition(connection.ID, record.Revision, state, time.Unix(0, record.UpdatedUnixNano).Add(time.Nanosecond))
		if err != nil {
			t.Fatalf("transition to %s: %v", state, err)
		}
	}
	record, err = manager.BindSnapshot(connection.ID, record.Revision, testkit.MCPSnapshot(), time.Unix(0, record.UpdatedUnixNano).Add(time.Nanosecond))
	if err != nil {
		t.Fatal(err)
	}
	if record.State != mcp.ConnectionBound || record.SnapshotState != mcp.SnapshotActive || record.Snapshot == nil {
		t.Fatalf("snapshot did not bind connection: %+v", record)
	}
	copy, ok := manager.Inspect(connection.ID)
	if !ok || copy.Snapshot.Digest != record.Snapshot.Digest {
		t.Fatal("connection inspection lost snapshot")
	}
}

func TestWhiteboxSessionIsolationAndUnknown(t *testing.T) {
	manager := mcp.NewManager()
	connection := testkit.MCPConnection()
	record, err := manager.Register(connection, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	other := connection
	other.ID = "mcp-connection_other0000000000000000"
	other.Digest = ""
	other, err = contract.SealMCPConnection(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Register(other, testkit.FixedTime); err == nil {
		t.Fatal("same tenant/identity/session was rebound to another connection")
	}
	record, err = manager.Transition(connection.ID, record.Revision, mcp.ConnectionResolving, testkit.FixedTime.Add(time.Nanosecond))
	if err != nil {
		t.Fatal(err)
	}
	record, err = manager.Transition(connection.ID, record.Revision, mcp.ConnectionConnecting, testkit.FixedTime.Add(2*time.Nanosecond))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Transition(connection.ID, record.Revision, mcp.ConnectionUnknown, testkit.FixedTime.Add(3*time.Nanosecond)); err != nil {
		t.Fatal(err)
	}
}

func TestWhiteboxExpiredConnectionCannotAdvance(t *testing.T) {
	manager := mcp.NewManager()
	connection := testkit.MCPConnection()
	record, err := manager.Register(connection, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Transition(connection.ID, record.Revision, mcp.ConnectionResolving, time.Unix(0, connection.ExpiresUnixNano)); err == nil {
		t.Fatal("expired connection advanced its active lifecycle")
	}
	inspected, _ := manager.Inspect(connection.ID)
	if inspected.Revision != record.Revision || inspected.State != mcp.ConnectionRegistered {
		t.Fatal("expired connection transition changed state")
	}
}

func TestWhiteboxSnapshotExpiryRequiresCurrentTime(t *testing.T) {
	manager, connection, record := bindActiveSnapshot(t)
	digest := record.Snapshot.Digest
	if _, err := manager.TransitionSnapshot(connection.ID, record.Revision, digest, mcp.SnapshotExpired, testkit.FixedTime.Add(30*time.Minute)); err == nil {
		t.Fatal("current snapshot was expired early")
	}
	unchanged, err := manager.InspectSnapshot(connection.ID, digest)
	if err != nil || unchanged.Revision != record.Revision || unchanged.SnapshotState != mcp.SnapshotActive {
		t.Fatalf("early expiry changed snapshot state: %v %+v", err, unchanged)
	}
	expired, err := manager.TransitionSnapshot(connection.ID, record.Revision, digest, mcp.SnapshotExpired, time.Unix(0, record.Snapshot.ExpiresUnixNano))
	if err != nil {
		t.Fatal(err)
	}
	if expired.SnapshotState != mcp.SnapshotExpired || expired.State != mcp.ConnectionDegraded || expired.Snapshot.Digest != digest {
		t.Fatalf("snapshot expiry did not preserve exact fact: %+v", expired)
	}
}

func TestWhiteboxSnapshotRevokeAndSupersede(t *testing.T) {
	for _, target := range []mcp.SnapshotState{mcp.SnapshotRevoked, mcp.SnapshotSuperseded} {
		t.Run(string(target), func(t *testing.T) {
			manager, connection, record := bindActiveSnapshot(t)
			updated, err := manager.TransitionSnapshot(connection.ID, record.Revision, record.Snapshot.Digest, target, testkit.FixedTime.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			if updated.SnapshotState != target || updated.State != mcp.ConnectionDegraded || updated.Snapshot.Digest != record.Snapshot.Digest {
				t.Fatalf("snapshot terminal transition rewrote content: %+v", updated)
			}
			if _, err := manager.TransitionSnapshot(connection.ID, updated.Revision, updated.Snapshot.Digest, mcp.SnapshotActive, testkit.FixedTime.Add(2*time.Second)); err == nil {
				t.Fatal("terminal snapshot was reactivated")
			}
		})
	}
}

func TestWhiteboxSnapshotLostReplyExactInspect(t *testing.T) {
	manager, connection, record := bindActiveSnapshot(t)
	updated, err := manager.TransitionSnapshot(connection.ID, record.Revision, record.Snapshot.Digest, mcp.SnapshotRevoked, testkit.FixedTime.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := manager.InspectSnapshot(connection.ID, record.Snapshot.Digest)
	if err != nil || inspected.Revision != updated.Revision || inspected.SnapshotState != mcp.SnapshotRevoked {
		t.Fatalf("lost snapshot reply could not be recovered exactly: %v %+v", err, inspected)
	}
	if _, err := manager.InspectSnapshot(connection.ID, testkit.Digest("wrong-snapshot")); err == nil {
		t.Fatal("exact snapshot Inspect accepted wrong digest")
	}
}

func TestWhiteboxConcurrentSnapshotTerminalSingleWinner(t *testing.T) {
	manager, connection, record := bindActiveSnapshot(t)
	const workers = 64
	var wg sync.WaitGroup
	results := make(chan error, workers)
	for i := range workers {
		wg.Add(1)
		go func(target mcp.SnapshotState) {
			defer wg.Done()
			_, err := manager.TransitionSnapshot(connection.ID, record.Revision, record.Snapshot.Digest, target, testkit.FixedTime.Add(time.Second))
			results <- err
		}([]mcp.SnapshotState{mcp.SnapshotRevoked, mcp.SnapshotSuperseded}[i%2])
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("want one snapshot terminal CAS winner, got %d", successes)
	}
	inspected, err := manager.InspectSnapshot(connection.ID, record.Snapshot.Digest)
	if err != nil || inspected.Revision != record.Revision+1 || inspected.SnapshotState != mcp.SnapshotRevoked && inspected.SnapshotState != mcp.SnapshotSuperseded {
		t.Fatalf("concurrent snapshot terminal state is invalid: %v %+v", err, inspected)
	}
}

func TestWhiteboxRunSessionAllowsDistinctServerAndNextClosedEpoch(t *testing.T) {
	manager := mcp.NewManager()
	first := testkit.MCPConnection()
	firstRecord, err := manager.Register(first, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	server := testkit.MCPServer()
	server.ID, _ = contract.StableID("mcp-server", "second-server")
	server.Revision, server.Digest = 1, ""
	server, err = contract.SealMCPServer(server)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Server = contract.ObjectRef{ID: server.ID, Revision: server.Revision, Digest: server.Digest}
	second.ID, _ = contract.StableID("mcp-connection", server.ID, second.RunID, second.SessionID, "1")
	second.Digest = ""
	second, err = contract.SealMCPConnection(second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Register(second, testkit.FixedTime); err != nil {
		t.Fatalf("same Run/Session could not bind a distinct MCP Server: %v", err)
	}
	firstRecord, err = manager.Transition(first.ID, firstRecord.Revision, mcp.ConnectionClosed, testkit.FixedTime.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	reconnect := first
	reconnect.Epoch++
	reconnect.ID, _ = contract.StableID("mcp-connection", reconnect.Server.ID, reconnect.RunID, reconnect.SessionID, "2")
	reconnect.Digest = ""
	reconnect.CreatedUnixNano = testkit.FixedTime.Add(2 * time.Second).UnixNano()
	reconnect.ExpiresUnixNano = testkit.FixedTime.Add(time.Hour).UnixNano()
	reconnect, err = contract.SealMCPConnection(reconnect)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Register(reconnect, testkit.FixedTime.Add(2*time.Second)); err != nil {
		t.Fatalf("closed MCP Connection could not advance to next epoch: %v", err)
	}
	if firstRecord.State != mcp.ConnectionClosed {
		t.Fatal("old epoch history was rewritten")
	}
}

func TestWhiteboxRunSessionRejectsEpochGapAndSeparatesRuns(t *testing.T) {
	manager := mcp.NewManager()
	first := testkit.MCPConnection()
	record, err := manager.Register(first, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	record, err = manager.Transition(first.ID, record.Revision, mcp.ConnectionClosed, testkit.FixedTime.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	gap := first
	gap.Epoch += 2
	gap.ID, _ = contract.StableID("mcp-connection", gap.Server.ID, gap.RunID, gap.SessionID, "3")
	gap.Digest = ""
	gap.CreatedUnixNano = testkit.FixedTime.Add(2 * time.Second).UnixNano()
	gap, err = contract.SealMCPConnection(gap)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Register(gap, testkit.FixedTime.Add(2*time.Second)); err == nil {
		t.Fatal("MCP Connection epoch gap was admitted")
	}
	crossRun := first
	crossRun.RunID = "run-other"
	crossRun.ID, _ = contract.StableID("mcp-connection", crossRun.Server.ID, crossRun.RunID, crossRun.SessionID, "1")
	crossRun.Digest = ""
	crossRun.CreatedUnixNano = testkit.FixedTime.Add(2 * time.Second).UnixNano()
	crossRun, err = contract.SealMCPConnection(crossRun)
	if err != nil {
		t.Fatal(err)
	}
	if crossRun.ID == first.ID {
		t.Fatal("cross-Run MCP connection reused the original identity")
	}
	if _, err := manager.Register(crossRun, testkit.FixedTime.Add(2*time.Second)); err != nil {
		t.Fatalf("distinct Run could not create its own MCP connection: %v", err)
	}
	_ = record
}
