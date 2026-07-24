package contract_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestToolAliasV1CanonicalIdentityAndDigest(t *testing.T) {
	tool := testkit.Tool()
	target := toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}
	alias := testkit.ToolAliasV1(1, target, testkit.FixedTime)
	if err := alias.ValidateAt(testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	id, err := toolcontract.DeriveToolAliasIDV1(testkit.Owner(), testkit.ToolAliasNameV1())
	if err != nil || id != alias.Ref.ID {
		t.Fatalf("Tool Alias stable ID=%q err=%v", id, err)
	}
	changedOwner := testkit.Owner()
	changedOwner.ID = "another-engine"
	otherID, err := toolcontract.DeriveToolAliasIDV1(changedOwner, testkit.ToolAliasNameV1())
	if err != nil || otherID == id {
		t.Fatalf("cross-owner Tool Alias ID was not isolated: %q %v", otherID, err)
	}
	tampered := alias
	tampered.Tool.Digest = testkit.Digest("alias-target-tamper")
	if tampered.Validate() == nil {
		t.Fatal("Tool Alias target drift did not invalidate its canonical digest")
	}
	tampered = alias
	tampered.Ref.Digest = testkit.Digest("alias-digest-tamper")
	if tampered.Validate() == nil {
		t.Fatal("Tool Alias Ref digest drift was accepted")
	}
}

func TestToolAliasV1SealRejectsRevisionClockAndIdentityDrift(t *testing.T) {
	tool := testkit.Tool()
	target := toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}
	if _, err := toolcontract.SealToolAliasV1(toolcontract.ToolAliasV1{Alias: testkit.ToolAliasNameV1(), Owner: testkit.Owner(), Tool: target, CreatedUnixNano: testkit.FixedTime.UnixNano()}); err == nil {
		t.Fatal("Tool Alias without revision was sealed")
	}
	alias := testkit.ToolAliasV1(1, target, testkit.FixedTime)
	alias.Ref.ID = "wrong-alias-id"
	alias.Ref.Digest = ""
	if _, err := toolcontract.SealToolAliasV1(alias); err == nil {
		t.Fatal("supplied drifting Tool Alias ID was accepted")
	}
	alias = testkit.ToolAliasV1(1, target, testkit.FixedTime)
	if err := alias.ValidateAt(testkit.FixedTime.Add(-1)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("Tool Alias clock regression error=%v", err)
	}
}
