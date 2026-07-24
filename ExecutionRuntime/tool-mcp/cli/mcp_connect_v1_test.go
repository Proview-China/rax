package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/cli"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestRunnerV1MCPConnectInspectAndAvailabilityAreReadOnly(t *testing.T) {
	fixture := newCLIFixtureV1(t)
	manager := mcp.NewManager()
	legacy := testkit.MCPConnection()
	if _, err := manager.Register(legacy, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	status, err := sdk.NewMCPStatusV1(manager, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	connect := newCLIConnectReadV1(t)
	runner, err := cli.NewRunnerWithMCPConnectV1(fixture.catalog, fixture.inspector, status, connect)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	args := []string{"mcp", "inspect", "--kind=receipt", "--id=" + connect.receipt.Ref.ID, "--revision=1", "--digest=" + string(connect.receipt.Ref.Digest)}
	if err := runner.RunV1(context.Background(), args, &output); err != nil {
		t.Fatal(err)
	}
	var inspected struct {
		ContractVersion string         `json:"contract_version"`
		Kind            string         `json:"kind"`
		Object          map[string]any `json:"object"`
	}
	if err := json.Unmarshal(output.Bytes(), &inspected); err != nil || inspected.ContractVersion != cli.ContractVersionV1 || inspected.Kind != "receipt" || inspected.Object["ref"] == nil {
		t.Fatalf("mcp inspect output=%s err=%v", output.String(), err)
	}
	output.Reset()
	args = []string{"mcp", "availability", "--id=" + connect.connection.ID, "--revision=1", "--digest=" + string(connect.connection.Digest), "--ttl=5s"}
	if err := runner.RunV1(context.Background(), args, &output); err != nil {
		t.Fatal(err)
	}
	var available cli.MCPConnectionAvailabilityOutputV1
	if err := json.Unmarshal(output.Bytes(), &available); err != nil || available.Availability.Connection != connect.connection {
		t.Fatalf("mcp availability output=%s err=%v", output.String(), err)
	}
	output.Reset()
	if err := runner.RunV1(context.Background(), []string{"mcp", "discover"}, &output); err == nil || output.Len() != 0 {
		t.Fatalf("mcp discover bypassed its missing Runtime page gate: err=%v output=%q", err, output.String())
	}
}

func TestRunnerV1RejectsTypedNilMCPConnectReadPort(t *testing.T) {
	fixture := newCLIFixtureV1(t)
	manager := mcp.NewManager()
	status, _ := sdk.NewMCPStatusV1(manager, func() time.Time { return testkit.FixedTime })
	var connect *fakeCLIConnectReadV1
	if _, err := cli.NewRunnerWithMCPConnectV1(fixture.catalog, fixture.inspector, status, connect); err == nil {
		t.Fatal("typed-nil MCP Connect read Port was accepted")
	}
}

type fakeCLIConnectReadV1 struct {
	receipt      toolcontract.MCPConnectProtocolReceiptV1
	connection   toolcontract.MCPConnectionFactRefV2
	availability toolcontract.MCPConnectionAvailabilityCurrentProjectionV1
}

func newCLIConnectReadV1(t *testing.T) *fakeCLIConnectReadV1 {
	t.Helper()
	fixture := testkit.MCPConnectControlledV1(testkit.FixedTime, toolcontract.MCPTransportStreamableHTTPV1)
	receipt := testkit.MCPConnectReceiptV1(fixture, []byte(`{"protocolVersion":"2025-03-26"}`), testkit.FixedTime)
	connection := toolcontract.MCPConnectionFactRefV2{ID: "mcp-connection-cli-v2", Revision: 1, Digest: testkit.Digest("mcp-connection-cli-v2")}
	availability, err := toolcontract.SealMCPConnectionAvailabilityCurrentProjectionV1(toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{Connection: connection, ApplySettlement: toolcontract.ObjectRef{ID: "mcp-connect-cli-apply", Revision: 1, Digest: testkit.Digest("mcp-connect-cli-apply")}, DomainResult: toolcontract.ObjectRef{ID: "mcp-connect-cli-domain", Revision: 1, Digest: testkit.Digest("mcp-connect-cli-domain")}, Owner: fixture.Connect.Intent.Owner, CheckedUnixNano: testkit.FixedTime.UnixNano(), ExpiresUnixNano: testkit.FixedTime.Add(10 * time.Second).UnixNano()}, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeCLIConnectReadV1{receipt: receipt, connection: connection, availability: availability}
}

func (f *fakeCLIConnectReadV1) InspectMCPConnectIntentV1(context.Context, toolcontract.ObjectRef) (toolcontract.MCPConnectIntentV1, error) {
	return toolcontract.MCPConnectIntentV1{}, nil
}
func (f *fakeCLIConnectReadV1) InspectMCPConnectProtocolReceiptV1(_ context.Context, exact toolcontract.MCPConnectProtocolReceiptRefV1) (toolcontract.MCPConnectProtocolReceiptV1, error) {
	if exact != f.receipt.Ref {
		return toolcontract.MCPConnectProtocolReceiptV1{}, context.Canceled
	}
	return f.receipt, nil
}
func (f *fakeCLIConnectReadV1) InspectMCPConnectionFactV2(context.Context, toolcontract.MCPConnectionFactRefV2) (toolcontract.MCPConnectionFactV2, error) {
	return toolcontract.MCPConnectionFactV2{}, nil
}
func (f *fakeCLIConnectReadV1) InspectMCPConnectDomainResultV1(context.Context, toolcontract.ObjectRef) (toolcontract.MCPConnectDomainResultFactV1, error) {
	return toolcontract.MCPConnectDomainResultFactV1{}, nil
}
func (f *fakeCLIConnectReadV1) InspectMCPConnectApplySettlementV1(context.Context, toolcontract.ObjectRef) (toolcontract.MCPConnectApplySettlementFactV1, error) {
	return toolcontract.MCPConnectApplySettlementFactV1{}, nil
}
func (f *fakeCLIConnectReadV1) InspectCurrentMCPConnectionAvailabilityV1(_ context.Context, exact toolcontract.MCPConnectionFactRefV2, _ time.Duration) (toolcontract.MCPConnectionAvailabilityCurrentProjectionV1, error) {
	if exact != f.connection {
		return toolcontract.MCPConnectionAvailabilityCurrentProjectionV1{}, context.Canceled
	}
	return f.availability, nil
}
