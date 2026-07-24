package blackbox_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/action"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

func TestBlackboxRegistrySurfaceAndActionSettlement(t *testing.T) {
	r := registry.New()
	capability, tool := testkit.Capability(), testkit.Tool()
	capRecord, err := r.SubmitCapability(capability, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	capRecord, err = r.Transition("capability", string(capability.ID), capRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Transition("capability", string(capability.ID), capRecord.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	toolRecord, err := r.SubmitTool(tool, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	toolRecord, err = r.Transition("tool", string(tool.ID), toolRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Transition("tool", string(tool.ID), toolRecord.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	snapshot, err := r.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := surface.Compile(surface.CompileRequest{
		Owner: testkit.Owner(), ResolvedPlanDigest: testkit.Digest("plan"), ProfileDigest: testkit.Digest("profile"), CapabilityGrantDigest: testkit.Digest("grant"), RegistrySnapshotDigest: snapshot.Digest,
		Dialect: "model/default", Selections: []surface.Selection{{Capability: capability, Tool: tool, ModelName: "example", DescriptionDigest: testkit.Digest("description"), Visible: true, Allowed: true}},
		Revision: 1, CreatedAt: testkit.FixedTime, ExpiresAt: testkit.FixedTime.Add(time.Hour),
	})
	if err != nil || manifest.Validate() != nil {
		t.Fatalf("compile surface: %v", err)
	}
	controller := action.NewController()
	candidate := testkit.Candidate()
	if _, err = controller.PutCandidate(candidate, candidate.PendingActionDigest); err != nil {
		t.Fatal(err)
	}
	if _, err = controller.Reserve(candidate.ID, testkit.Digest("application-attempt"), testkit.Digest("intent"), testkit.Digest("subject"), candidate.SessionID, testkit.FixedTime, testkit.FixedTime.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	payload := testkit.Payload(`{"ok":true}`)
	if _, err = controller.RecordDomainResult(candidate.ID, "attempt-1", testkit.Digest("observation"), payload, nil, testkit.FixedTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	record, err := controller.ApplySettlement(candidate.ID, testkit.Settlement("attempt-1", payload), testkit.FixedTime.Add(2*time.Second))
	if err != nil || record.State != action.StateSettled || record.Result == nil {
		t.Fatalf("settle action: %v %+v", err, record)
	}
}

func TestBlackboxExpiredConnectionRegistrationRejected(t *testing.T) {
	connection := testkit.MCPConnection()
	manager := mcp.NewManager()
	if _, err := manager.Register(connection, time.Unix(0, connection.ExpiresUnixNano)); err == nil {
		t.Fatal("expired MCP connection was registered")
	}
	if _, ok := manager.Inspect(connection.ID); ok {
		t.Fatal("expired MCP connection left a registration fact")
	}
}
