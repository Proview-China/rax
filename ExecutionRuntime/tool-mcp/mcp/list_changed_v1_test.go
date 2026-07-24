package mcp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPListChangedJournalV1CoalescesAndAcknowledges(t *testing.T) {
	now := testkit.FixedTime
	journal, err := NewInMemoryMCPListChangedJournalV1(func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	connection := testkit.MCPConnection()
	snapshot := testkit.MCPSnapshot()
	ref := toolcontract.ObjectRef{ID: snapshot.ID, Revision: snapshot.Revision, Digest: snapshot.Digest}
	first, err := journal.RecordMCPListChangedV1(context.Background(), connection, ref, toolcontract.MCPListChangedToolsV1)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Nanosecond)
	second, err := journal.RecordMCPListChangedV1(context.Background(), connection, ref, toolcontract.MCPListChangedToolsV1)
	if err != nil || second.Ref != first.Ref || second.ObservedUnixNano != first.ObservedUnixNano {
		t.Fatalf("duplicate pending notification was not coalesced: %+v err=%v", second, err)
	}
	changed := ref
	changed.Revision++
	changed.Digest = testkit.Digest("changed-snapshot")
	if _, err := journal.RecordMCPListChangedV1(context.Background(), connection, changed, toolcontract.MCPListChangedToolsV1); err == nil {
		t.Fatal("pending notification accepted another Snapshot")
	}
	if err := journal.AcknowledgeMCPListChangedV1(context.Background(), first.Ref, changed); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Nanosecond)
	third, err := journal.RecordMCPListChangedV1(context.Background(), connection, changed, toolcontract.MCPListChangedToolsV1)
	if err != nil || third.SourceSequence != first.SourceSequence+1 || third.Ref == first.Ref {
		t.Fatalf("successor notification did not receive a new sequence: %+v err=%v", third, err)
	}
	inspected, err := journal.InspectMCPListChangedV1(context.Background(), first.Ref)
	if err != nil || inspected.Ref != first.Ref {
		t.Fatalf("history Inspect failed: %+v err=%v", inspected, err)
	}
}

func TestMCPListChangedJournalV1ConcurrentSamePendingSingleFact(t *testing.T) {
	journal, _ := NewInMemoryMCPListChangedJournalV1(func() time.Time { return testkit.FixedTime })
	connection := testkit.MCPConnection()
	snapshot := testkit.MCPSnapshot()
	ref := toolcontract.ObjectRef{ID: snapshot.ID, Revision: snapshot.Revision, Digest: snapshot.Digest}
	const workers = 64
	var wg sync.WaitGroup
	refs := make(chan toolcontract.MCPListChangedObservationRefV1, workers)
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			observation, err := journal.RecordMCPListChangedV1(context.Background(), connection, ref, toolcontract.MCPListChangedResourcesV1)
			refs <- observation.Ref
			errs <- err
		}()
	}
	wg.Wait()
	close(refs)
	close(errs)
	var winner toolcontract.MCPListChangedObservationRefV1
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for ref := range refs {
		if winner == (toolcontract.MCPListChangedObservationRefV1{}) {
			winner = ref
		} else if ref != winner {
			t.Fatalf("concurrent list-changed produced multiple facts: %+v %+v", winner, ref)
		}
	}
}

func TestMCPListChangedJournalV1ContextAndClockBoundaries(t *testing.T) {
	connection := testkit.MCPConnection()
	snapshot := testkit.MCPSnapshot()
	ref := toolcontract.ObjectRef{ID: snapshot.ID, Revision: snapshot.Revision, Digest: snapshot.Digest}
	journal, _ := NewInMemoryMCPListChangedJournalV1(func() time.Time { return testkit.FixedTime.Add(-time.Second) })
	if _, err := journal.RecordMCPListChangedV1(context.Background(), connection, ref, toolcontract.MCPListChangedToolsV1); err == nil {
		t.Fatal("clock rollback was accepted")
	}
	journal, _ = NewInMemoryMCPListChangedJournalV1(func() time.Time { return testkit.FixedTime })
	if _, err := journal.RecordMCPListChangedV1(nil, connection, ref, toolcontract.MCPListChangedToolsV1); err == nil {
		t.Fatal("nil context was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := journal.RecordMCPListChangedV1(ctx, connection, ref, toolcontract.MCPListChangedToolsV1); err != context.Canceled {
		t.Fatalf("canceled context was not preserved: %v", err)
	}
}

type signalingListChangedSinkV1 struct {
	inner *InMemoryMCPListChangedJournalV1
	seen  chan toolcontract.MCPListChangedObservationV1
}

func (s *signalingListChangedSinkV1) RecordMCPListChangedV1(ctx context.Context, connection toolcontract.MCPConnectionRef, snapshot toolcontract.ObjectRef, namespace toolcontract.MCPListChangedNamespaceV1) (toolcontract.MCPListChangedObservationV1, error) {
	observation, err := s.inner.RecordMCPListChangedV1(ctx, connection, snapshot, namespace)
	if err == nil {
		select {
		case s.seen <- observation:
		default:
		}
	}
	return observation, err
}

func TestOfficialSDKListChangedBridgeV1OverInMemoryTransport(t *testing.T) {
	fixture := newMCPCallFixtureV1(t)
	journal, _ := NewInMemoryMCPListChangedJournalV1(func() time.Time { return fixture.now })
	sink := &signalingListChangedSinkV1{inner: journal, seen: make(chan toolcontract.MCPListChangedObservationV1, 4)}
	snapshot := toolcontract.ObjectRef{ID: fixture.command.Snapshot.ID, Revision: fixture.command.Snapshot.Revision, Digest: fixture.command.Snapshot.Digest}
	bridge, err := NewOfficialSDKListChangedBridgeV1(fixture.command.Connection, snapshot, sink)
	if err != nil {
		t.Fatal(err)
	}
	options := &officialmcp.ClientOptions{}
	if err := bridge.InstallClientOptionsV1(options); err != nil {
		t.Fatal(err)
	}
	if err := bridge.InstallClientOptionsV1(options); err == nil {
		t.Fatal("existing official SDK handlers were overwritten")
	}
	server := officialmcp.NewServer(&officialmcp.Implementation{Name: "list-changed-server", Version: "1.0.0"}, nil)
	server.AddTool(&officialmcp.Tool{Name: "initial", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *officialmcp.CallToolRequest) (*officialmcp.CallToolResult, error) {
		return &officialmcp.CallToolResult{}, nil
	})
	serverTransport, clientTransport := officialmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := officialmcp.NewClient(&officialmcp.Implementation{Name: "list-changed-client", Version: "1.0.0"}, options)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	if err := bridge.BindInitializedSessionV1(context.Background(), clientSession); err != nil {
		t.Fatal(err)
	}
	server.AddTool(&officialmcp.Tool{Name: "added", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *officialmcp.CallToolRequest) (*officialmcp.CallToolResult, error) {
		return &officialmcp.CallToolResult{}, nil
	})
	select {
	case observation := <-sink.seen:
		if observation.Namespace != toolcontract.MCPListChangedToolsV1 || observation.Connection != fixture.command.Connection || observation.Snapshot != snapshot {
			t.Fatalf("official SDK notification mapping drifted: %+v", observation)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("official SDK list-changed notification was not observed")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		state := bridge.InspectStateV1()
		if state.LastError == nil && state.LastObservation != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("bridge state did not retain exact notification result: %+v", state)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestOfficialSDKListChangedBridgeV1TypedNilAndWrongSession(t *testing.T) {
	fixture := newMCPCallFixtureV1(t)
	var typedNil *InMemoryMCPListChangedJournalV1
	snapshot := toolcontract.ObjectRef{ID: fixture.command.Snapshot.ID, Revision: fixture.command.Snapshot.Revision, Digest: fixture.command.Snapshot.Digest}
	if _, err := NewOfficialSDKListChangedBridgeV1(fixture.command.Connection, snapshot, typedNil); err == nil {
		t.Fatal("typed-nil list-changed sink was accepted")
	}
	journal, _ := NewInMemoryMCPListChangedJournalV1(func() time.Time { return fixture.now })
	bridge, _ := NewOfficialSDKListChangedBridgeV1(fixture.command.Connection, snapshot, journal)
	options := &officialmcp.ClientOptions{}
	_ = bridge.InstallClientOptionsV1(options)
	options.ToolListChangedHandler(context.Background(), nil)
	state := bridge.InspectStateV1()
	if state.LastError == nil || !core.HasCategory(state.LastError, core.ErrorForbidden) {
		t.Fatalf("unbound/wrong Session notification was not rejected: %v", state.LastError)
	}
}
