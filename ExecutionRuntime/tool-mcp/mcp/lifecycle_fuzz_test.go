package mcp_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
)

func FuzzMCPLifecycleStateMachineV1(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3})
	f.Add([]byte{5, 6, 7, 0xff})
	f.Add([]byte{2, 3, 4, 1, 0})
	f.Fuzz(func(t *testing.T, operations []byte) {
		if len(operations) > 64 {
			operations = operations[:64]
		}
		manager, connection, initial := bindActiveSnapshot(t)
		for index, operation := range operations {
			before, ok := manager.Inspect(connection.ID)
			if !ok || before.Snapshot == nil {
				t.Fatal("lifecycle record or exact Snapshot disappeared")
			}
			expected := before.Revision
			if operation&0x80 != 0 {
				expected++
			}
			now := testkit.FixedTime.Add(time.Duration(index+1) * time.Second)
			if operation&0x40 != 0 {
				now = testkit.FixedTime.Add(-time.Duration(index+1) * time.Second)
			}

			var err error
			switch operation & 0x0f {
			case 0:
				_, err = manager.Transition(connection.ID, expected, mcp.ConnectionDegraded, now)
			case 1:
				_, err = manager.Transition(connection.ID, expected, mcp.ConnectionBound, now)
			case 2:
				_, err = manager.Transition(connection.ID, expected, mcp.ConnectionDraining, now)
			case 3:
				_, err = manager.Transition(connection.ID, expected, mcp.ConnectionClosed, now)
			case 4:
				_, err = manager.Transition(connection.ID, expected, mcp.ConnectionUnknown, now)
			case 5:
				_, err = manager.TransitionSnapshot(connection.ID, expected, before.Snapshot.Digest, mcp.SnapshotExpired, now)
			case 6:
				_, err = manager.TransitionSnapshot(connection.ID, expected, before.Snapshot.Digest, mcp.SnapshotRevoked, now)
			case 7:
				_, err = manager.TransitionSnapshot(connection.ID, expected, before.Snapshot.Digest, mcp.SnapshotSuperseded, now)
			default:
				_, err = manager.TransitionSnapshot(connection.ID, expected, testkit.Digest("wrong-snapshot"), mcp.SnapshotRevoked, now)
			}

			after, ok := manager.Inspect(connection.ID)
			if !ok || after.Snapshot == nil {
				t.Fatal("lifecycle operation removed its exact record")
			}
			if after.Connection != initial.Connection || after.Snapshot.Digest != initial.Snapshot.Digest {
				t.Fatal("lifecycle operation rewrote immutable Connection or Snapshot content")
			}
			if after.Revision < before.Revision || after.Revision > before.Revision+1 {
				t.Fatalf("lifecycle revision was not monotonic: before=%d after=%d", before.Revision, after.Revision)
			}
			if err != nil && !reflect.DeepEqual(after, before) {
				t.Fatalf("failed lifecycle operation changed state: before=%+v after=%+v err=%v", before, after, err)
			}
			if before.State == mcp.ConnectionClosed && after.State != mcp.ConnectionClosed {
				t.Fatal("closed MCP Connection was revived")
			}
			switch before.SnapshotState {
			case mcp.SnapshotExpired, mcp.SnapshotRevoked, mcp.SnapshotSuperseded:
				if after.SnapshotState != before.SnapshotState {
					t.Fatal("terminal MCP Snapshot state changed")
				}
			}
		}
	})
}
