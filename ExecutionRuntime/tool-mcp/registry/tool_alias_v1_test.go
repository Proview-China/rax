package registry_test

import (
	"sync"
	"testing"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

func activeToolAliasRegistryV1(t *testing.T) (*registry.Registry, toolcontract.ToolDescriptor) {
	t.Helper()
	store := registry.New()
	capability := testkit.Capability()
	capRecord, err := store.SubmitCapability(capability, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	capRecord, err = store.Transition("capability", string(capability.ID), capRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Transition("capability", string(capability.ID), capRecord.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	tool := testkit.Tool()
	toolRecord, err := store.SubmitTool(tool, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	toolRecord, err = store.Transition("tool", string(tool.ID), toolRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Transition("tool", string(tool.ID), toolRecord.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	return store, tool
}

func secondActiveAliasToolV1(t *testing.T, store *registry.Registry) toolcontract.ToolDescriptor {
	t.Helper()
	tool := testkit.Tool()
	tool.ID = "tool/example-second"
	tool.ArtifactDigest = testkit.Digest("tool-second-artifact")
	tool.Digest = ""
	var err error
	tool, err = toolcontract.SealTool(tool)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.SubmitTool(tool, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	record, err = store.Transition("tool", string(tool.ID), record.RegistryRevision, registry.StateAdmitted, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Transition("tool", string(tool.ID), record.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	return tool
}

func TestToolAliasRegistryV1CreateSuccessorHistoryAndRevoke(t *testing.T) {
	store, firstTool := activeToolAliasRegistryV1(t)
	first := testkit.ToolAliasV1(1, toolcontract.ObjectRef{ID: string(firstTool.ID), Revision: firstTool.Revision, Digest: firstTool.Digest}, testkit.FixedTime)
	firstRecord, err := store.SubmitToolAlias(first, nil, testkit.FixedTime)
	if err != nil || firstRecord.State != registry.StateActive {
		t.Fatalf("Tool Alias create record=%+v err=%v", firstRecord, err)
	}
	if repeated, err := store.SubmitToolAlias(first, nil, testkit.FixedTime); err != nil || repeated != firstRecord {
		t.Fatalf("same canonical Tool Alias was not idempotent: %+v %v", repeated, err)
	}
	secondTool := secondActiveAliasToolV1(t, store)
	second := testkit.ToolAliasV1(2, toolcontract.ObjectRef{ID: string(secondTool.ID), Revision: secondTool.Revision, Digest: secondTool.Digest}, testkit.FixedTime)
	expected := first.Ref
	secondRecord, err := store.SubmitToolAlias(second, &expected, testkit.FixedTime)
	if err != nil || secondRecord.ObjectRevision != 2 {
		t.Fatalf("Tool Alias successor record=%+v err=%v", secondRecord, err)
	}
	historical, historicalRecord, ok := store.InspectToolAlias(first.Ref)
	if !ok || historical.Ref != first.Ref || historicalRecord != firstRecord {
		t.Fatalf("Tool Alias history drifted: %+v %+v %v", historical, historicalRecord, ok)
	}
	current, currentRecord, ok := store.ResolveToolAlias(second.Ref.ID)
	if !ok || current.Ref != second.Ref || currentRecord != secondRecord {
		t.Fatalf("Tool Alias current drifted: %+v %+v %v", current, currentRecord, ok)
	}
	if _, err := store.SubmitToolAlias(first, nil, testkit.FixedTime); err == nil {
		t.Fatal("historical Tool Alias revision rolled current back")
	}
	revoked, err := store.Transition("tool-alias", second.Ref.ID, secondRecord.RegistryRevision, registry.StateRevoked, testkit.FixedTime)
	if err != nil || revoked.State != registry.StateRevoked {
		t.Fatalf("Tool Alias revoke=%+v err=%v", revoked, err)
	}
	third := testkit.ToolAliasV1(3, second.Tool, testkit.FixedTime)
	expected = second.Ref
	if _, err := store.SubmitToolAlias(third, &expected, testkit.FixedTime); err == nil {
		t.Fatal("revoked Tool Alias was repointed")
	}
}

func TestToolAliasRegistryV1FailsClosedOnTargetAndCASDrift(t *testing.T) {
	store, tool := activeToolAliasRegistryV1(t)
	target := toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}
	alias := testkit.ToolAliasV1(1, target, testkit.FixedTime)
	wrongTarget := alias
	wrongTarget.Tool.Digest = testkit.Digest("wrong-alias-target")
	wrongTarget.Ref.Digest = ""
	wrongTarget, _ = toolcontract.SealToolAliasV1(wrongTarget)
	if _, err := store.SubmitToolAlias(wrongTarget, nil, testkit.FixedTime); err == nil {
		t.Fatal("Tool Alias targeted a drifting Tool")
	}
	wrongExpected := alias.Ref
	wrongExpected.Digest = testkit.Digest("wrong-alias-expected")
	if _, err := store.SubmitToolAlias(alias, &wrongExpected, testkit.FixedTime); err == nil {
		t.Fatal("Tool Alias create accepted an expected-current Ref")
	}
	if _, err := store.SubmitToolAlias(alias, nil, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	successor := testkit.ToolAliasV1(3, target, testkit.FixedTime)
	expected := alias.Ref
	if _, err := store.SubmitToolAlias(successor, &expected, testkit.FixedTime); err == nil {
		t.Fatal("Tool Alias skipped a revision")
	}
}

func TestToolAliasRegistryV1ConcurrentSameCanonicalSingleRevision(t *testing.T) {
	store, tool := activeToolAliasRegistryV1(t)
	alias := testkit.ToolAliasV1(1, toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}, testkit.FixedTime)
	const workers = 64
	var wg sync.WaitGroup
	records := make(chan registry.Record, workers)
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			record, err := store.SubmitToolAlias(alias, nil, testkit.FixedTime)
			records <- record
			errs <- err
		}()
	}
	wg.Wait()
	close(records)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := registry.Record{}
	for record := range records {
		if want.RegistryRevision == 0 {
			want = record
		} else if record != want {
			t.Fatalf("concurrent Tool Alias created multiple records: %+v != %+v", record, want)
		}
	}
}
