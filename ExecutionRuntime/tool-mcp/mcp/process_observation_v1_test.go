package mcp

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func processObservationInputV1(kind toolcontract.MCPProcessObservationKindV1) toolcontract.MCPProcessObservationInputV1 {
	input := toolcontract.MCPProcessObservationInputV1{Kind: kind, CorrelationDigest: testkit.Digest("process-correlation"), PayloadDigest: testkit.Digest("process-payload")}
	if kind == toolcontract.MCPProcessProgressV1 {
		input.Progress, input.Total = 1, 2
	} else {
		input.LoggingLevel, input.Logger = "info", "mcp-test"
	}
	return input
}

func TestMCPProcessObservationJournalV1ExactHistoryAndKinds(t *testing.T) {
	now := testkit.FixedTime
	journal, err := NewInMemoryMCPProcessObservationJournalV1(func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	connection := testkit.MCPConnection()
	snapshotValue := testkit.MCPSnapshot()
	snapshot := toolcontract.ObjectRef{ID: snapshotValue.ID, Revision: snapshotValue.Revision, Digest: snapshotValue.Digest}
	progress, err := journal.RecordMCPProcessObservationV1(context.Background(), connection, snapshot, processObservationInputV1(toolcontract.MCPProcessProgressV1))
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Nanosecond)
	logging, err := journal.RecordMCPProcessObservationV1(context.Background(), connection, snapshot, processObservationInputV1(toolcontract.MCPProcessLoggingV1))
	if err != nil {
		t.Fatal(err)
	}
	if progress.SourceSequence != 1 || logging.SourceSequence != 2 || progress.Ref == logging.Ref {
		t.Fatalf("process sequence drifted: progress=%+v logging=%+v", progress, logging)
	}
	inspected, err := journal.InspectMCPProcessObservationV1(context.Background(), progress.Ref)
	if err != nil || inspected.Ref != progress.Ref || inspected.Kind != toolcontract.MCPProcessProgressV1 {
		t.Fatalf("exact Inspect=%+v err=%v", inspected, err)
	}
	wrong := progress.Ref
	wrong.Digest = testkit.Digest("wrong-process-observation")
	if _, err = journal.InspectMCPProcessObservationV1(context.Background(), wrong); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("wrong digest error=%v", err)
	}
	if err = progress.ValidateCurrent(time.Unix(0, connection.ExpiresUnixNano)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("expired process Observation error=%v", err)
	}
}

func TestMCPProcessObservationJournalV1ConcurrentSequenceAndFailClosed(t *testing.T) {
	now := testkit.FixedTime
	journal, _ := NewInMemoryMCPProcessObservationJournalV1(func() time.Time { return now })
	connection := testkit.MCPConnection()
	snapshotValue := testkit.MCPSnapshot()
	snapshot := toolcontract.ObjectRef{ID: snapshotValue.ID, Revision: snapshotValue.Revision, Digest: snapshotValue.Digest}
	const workers = 64
	var wg sync.WaitGroup
	sequences := make(chan uint64, workers)
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := journal.RecordMCPProcessObservationV1(context.Background(), connection, snapshot, processObservationInputV1(toolcontract.MCPProcessProgressV1))
			sequences <- value.SourceSequence
			errs <- err
		}()
	}
	wg.Wait()
	close(sequences)
	close(errs)
	seen := make(map[uint64]struct{}, workers)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for sequence := range sequences {
		if _, ok := seen[sequence]; ok || sequence == 0 {
			t.Fatalf("duplicate process sequence %d", sequence)
		}
		seen[sequence] = struct{}{}
	}
	if len(seen) != workers {
		t.Fatalf("process sequence count=%d", len(seen))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := journal.RecordMCPProcessObservationV1(ctx, connection, snapshot, processObservationInputV1(toolcontract.MCPProcessLoggingV1)); err != context.Canceled {
		t.Fatalf("canceled context error=%v", err)
	}
	rollback, _ := NewInMemoryMCPProcessObservationJournalV1(func() time.Time { return time.Unix(0, connection.CreatedUnixNano-1) })
	if _, err := rollback.RecordMCPProcessObservationV1(context.Background(), connection, snapshot, processObservationInputV1(toolcontract.MCPProcessLoggingV1)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback error=%v", err)
	}
}

func TestMCPProcessObservationJournalV1BoundedPullPage(t *testing.T) {
	now := testkit.FixedTime
	journal, _ := NewInMemoryMCPProcessObservationJournalV1(func() time.Time { return now })
	connection := testkit.MCPConnection()
	snapshotValue := testkit.MCPSnapshot()
	snapshot := toolcontract.ObjectRef{ID: snapshotValue.ID, Revision: snapshotValue.Revision, Digest: snapshotValue.Digest}
	for index := range 3 {
		now = now.Add(time.Nanosecond)
		kind := toolcontract.MCPProcessProgressV1
		if index == 1 {
			kind = toolcontract.MCPProcessLoggingV1
		}
		if _, err := journal.RecordMCPProcessObservationV1(context.Background(), connection, snapshot, processObservationInputV1(kind)); err != nil {
			t.Fatal(err)
		}
	}
	request := toolcontract.MCPProcessObservationPageRequestV1{
		Connection:      toolcontract.ObjectRef{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest},
		ConnectionEpoch: connection.Epoch, Snapshot: snapshot, Limit: 2,
	}
	first, err := journal.ReadMCPProcessObservationPageV1(context.Background(), request)
	if err != nil || first.Validate() != nil || len(first.Observations) != 2 || !first.HasMore || first.NextAfterSourceSequence != 2 || first.UpperBoundSourceSequence != 3 {
		t.Fatalf("first process page=%+v err=%v", first, err)
	}
	request.AfterSourceSequence = first.NextAfterSourceSequence
	second, err := journal.ReadMCPProcessObservationPageV1(context.Background(), request)
	if err != nil || second.Validate() != nil || len(second.Observations) != 1 || second.HasMore || second.NextAfterSourceSequence != 3 || second.UpperBoundSourceSequence != 3 {
		t.Fatalf("second process page=%+v err=%v", second, err)
	}
	second.Observations[0].PayloadDigest = testkit.Digest("tampered-process-page")
	if second.Validate() == nil {
		t.Fatal("tampered process page was accepted")
	}
	other := request
	other.Snapshot = toolcontract.ObjectRef{ID: "other-snapshot", Revision: 1, Digest: testkit.Digest("other-process-snapshot")}
	empty, err := journal.ReadMCPProcessObservationPageV1(context.Background(), other)
	if err != nil || len(empty.Observations) != 0 || empty.NextAfterSourceSequence != other.AfterSourceSequence || empty.HasMore {
		t.Fatalf("empty exact stream page=%+v err=%v", empty, err)
	}
}

func TestMCPProcessObservationJournalV1PageConcurrentReadAndContext(t *testing.T) {
	journal, _ := NewInMemoryMCPProcessObservationJournalV1(func() time.Time { return testkit.FixedTime })
	connection := testkit.MCPConnection()
	snapshotValue := testkit.MCPSnapshot()
	snapshot := toolcontract.ObjectRef{ID: snapshotValue.ID, Revision: snapshotValue.Revision, Digest: snapshotValue.Digest}
	request := toolcontract.MCPProcessObservationPageRequestV1{
		Connection:      toolcontract.ObjectRef{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest},
		ConnectionEpoch: connection.Epoch, Snapshot: snapshot, Limit: 64,
	}
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := journal.RecordMCPProcessObservationV1(context.Background(), connection, snapshot, processObservationInputV1(toolcontract.MCPProcessProgressV1))
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
	page, err := journal.ReadMCPProcessObservationPageV1(context.Background(), request)
	if err != nil || page.Validate() != nil || len(page.Observations) != workers || page.HasMore {
		t.Fatalf("concurrent process page count=%d err=%v", len(page.Observations), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := journal.ReadMCPProcessObservationPageV1(ctx, request); err != context.Canceled {
		t.Fatalf("canceled page read error=%v", err)
	}
	request.Limit = toolcontract.MaxMCPProcessObservationPageSizeV1 + 1
	if _, err := journal.ReadMCPProcessObservationPageV1(context.Background(), request); err == nil {
		t.Fatal("oversized process page was accepted")
	}
}

func TestOfficialSDKProcessObservationBridgeV1MapsOfficialNotifications(t *testing.T) {
	fixture := newMCPCallFixtureV1(t)
	journal, _ := NewInMemoryMCPProcessObservationJournalV1(func() time.Time { return fixture.now })
	snapshot := toolcontract.ObjectRef{ID: fixture.command.Snapshot.ID, Revision: fixture.command.Snapshot.Revision, Digest: fixture.command.Snapshot.Digest}
	bridge, err := NewOfficialSDKProcessObservationBridgeV1(fixture.command.Connection, snapshot, journal)
	if err != nil {
		t.Fatal(err)
	}
	options := &officialmcp.ClientOptions{}
	if err := bridge.InstallClientOptionsV1(options); err != nil {
		t.Fatal(err)
	}
	if err := bridge.InstallClientOptionsV1(options); err == nil {
		t.Fatal("existing official SDK process handlers were overwritten")
	}
	server := officialmcp.NewServer(&officialmcp.Implementation{Name: "process-server", Version: "1.0.0"}, nil)
	serverTransport, clientTransport := officialmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := officialmcp.NewClient(&officialmcp.Implementation{Name: "process-client", Version: "1.0.0"}, options)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	if err := bridge.BindInitializedSessionV1(context.Background(), clientSession); err != nil {
		t.Fatal(err)
	}
	options.ProgressNotificationHandler(context.Background(), &officialmcp.ProgressNotificationClientRequest{Session: clientSession, Params: &officialmcp.ProgressNotificationParams{ProgressToken: "attempt-1", Message: "half", Progress: 1, Total: 2}})
	state := bridge.InspectStateV1()
	if state.LastError != nil || state.LastObservation == nil {
		t.Fatalf("progress bridge state=%+v", state)
	}
	progress, err := journal.InspectMCPProcessObservationV1(context.Background(), *state.LastObservation)
	if err != nil || progress.Kind != toolcontract.MCPProcessProgressV1 || progress.Progress != 1 || progress.Total != 2 {
		t.Fatalf("progress Observation=%+v err=%v", progress, err)
	}
	options.LoggingMessageHandler(context.Background(), &officialmcp.LoggingMessageRequest{Session: clientSession, Params: &officialmcp.LoggingMessageParams{Level: "warning", Logger: "remote", Data: map[string]any{"message": "bounded"}}})
	state = bridge.InspectStateV1()
	if state.LastError != nil || state.LastObservation == nil {
		t.Fatalf("logging bridge state=%+v", state)
	}
	logging, err := journal.InspectMCPProcessObservationV1(context.Background(), *state.LastObservation)
	if err != nil || logging.Kind != toolcontract.MCPProcessLoggingV1 || logging.LoggingLevel != "warning" || logging.Logger != "remote" || logging.SourceSequence != progress.SourceSequence+1 {
		t.Fatalf("logging Observation=%+v err=%v", logging, err)
	}
}

func TestOfficialSDKProcessObservationBridgeV1RejectsTypedNilWrongSessionAndPayload(t *testing.T) {
	fixture := newMCPCallFixtureV1(t)
	snapshot := toolcontract.ObjectRef{ID: fixture.command.Snapshot.ID, Revision: fixture.command.Snapshot.Revision, Digest: fixture.command.Snapshot.Digest}
	var typedNil *InMemoryMCPProcessObservationJournalV1
	if _, err := NewOfficialSDKProcessObservationBridgeV1(fixture.command.Connection, snapshot, typedNil); err == nil {
		t.Fatal("typed-nil process sink was accepted")
	}
	journal, _ := NewInMemoryMCPProcessObservationJournalV1(func() time.Time { return fixture.now })
	bridge, _ := NewOfficialSDKProcessObservationBridgeV1(fixture.command.Connection, snapshot, journal)
	options := &officialmcp.ClientOptions{}
	_ = bridge.InstallClientOptionsV1(options)
	options.ProgressNotificationHandler(context.Background(), nil)
	if state := bridge.InspectStateV1(); state.LastError == nil {
		t.Fatal("nil progress notification was accepted")
	}
	options.ProgressNotificationHandler(context.Background(), &officialmcp.ProgressNotificationClientRequest{Params: &officialmcp.ProgressNotificationParams{ProgressToken: map[string]any{"bad": true}, Progress: 1}})
	if state := bridge.InspectStateV1(); state.LastError == nil {
		t.Fatal("non-scalar progress token was accepted")
	}
	options.LoggingMessageHandler(context.Background(), &officialmcp.LoggingMessageRequest{Params: &officialmcp.LoggingMessageParams{Level: "info", Data: strings.Repeat("x", MaxMessageBytes+1)}})
	if state := bridge.InspectStateV1(); !core.HasReason(state.LastError, core.ReasonCanonicalLimitExceeded) {
		t.Fatalf("oversized logging payload error=%v", state.LastError)
	}
	options.LoggingMessageHandler(context.Background(), &officialmcp.LoggingMessageRequest{Params: &officialmcp.LoggingMessageParams{Level: "made-up", Data: "x"}})
	if state := bridge.InspectStateV1(); state.LastError == nil {
		t.Fatal("wrong-session/invalid-level logging notification was accepted")
	}
}
