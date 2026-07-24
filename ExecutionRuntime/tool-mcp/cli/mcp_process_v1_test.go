package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/cli"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
)

func TestRunnerV1MCPProcessBoundedPage(t *testing.T) {
	fixture := newCLIFixtureV1(t)
	journal, _ := mcp.NewInMemoryMCPProcessObservationJournalV1(func() time.Time { return testkit.FixedTime })
	connection := testkit.MCPConnection()
	snapshotValue := testkit.MCPSnapshot()
	snapshot := toolcontract.ObjectRef{ID: snapshotValue.ID, Revision: snapshotValue.Revision, Digest: snapshotValue.Digest}
	observation, err := journal.RecordMCPProcessObservationV1(context.Background(), connection, snapshot, toolcontract.MCPProcessObservationInputV1{
		Kind: toolcontract.MCPProcessLoggingV1, CorrelationDigest: testkit.Digest("cli-process-correlation"), PayloadDigest: testkit.Digest("cli-process-payload"), LoggingLevel: "info", Logger: "remote",
	})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := cli.NewRunnerWithMCPProcessV1(fixture.catalog, fixture.inspector, journal)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{
		"mcp", "process",
		"--connection-id=" + connection.ID,
		"--connection-revision=" + strconv.FormatUint(uint64(connection.Revision), 10),
		"--connection-digest=" + string(connection.Digest),
		"--connection-epoch=" + strconv.FormatUint(uint64(connection.Epoch), 10),
		"--snapshot-id=" + snapshot.ID,
		"--snapshot-revision=" + strconv.FormatUint(uint64(snapshot.Revision), 10),
		"--snapshot-digest=" + string(snapshot.Digest),
		"--limit=10",
	}
	var output bytes.Buffer
	if err := runner.RunV1(context.Background(), args, &output); err != nil {
		t.Fatal(err)
	}
	var result cli.MCPProcessPageOutputV1
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.ContractVersion != cli.ContractVersionV1 || len(result.Page.Observations) != 1 || result.Page.Observations[0].Ref != observation.Ref {
		t.Fatalf("mcp process output=%+v err=%v", result, err)
	}
	output.Reset()
	args[len(args)-1] = "--limit=0"
	if err := runner.RunV1(context.Background(), args, &output); err == nil || output.Len() != 0 {
		t.Fatalf("invalid process page wrote output: err=%v output=%q", err, output.String())
	}
}
