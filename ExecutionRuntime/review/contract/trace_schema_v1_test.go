package contract

import (
	"encoding/json"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestTraceFactSchemaRefV1BindsExactDocumentBytes(t *testing.T) {
	document := TraceFactJSONSchemaV1()
	if !json.Valid(document) {
		t.Fatal("Trace Fact schema document is not valid JSON")
	}
	ref := TraceFactSchemaRefV1()
	if err := ref.Validate(); err != nil {
		t.Fatal(err)
	}
	if ref.ContentDigest != core.DigestBytes(document) {
		t.Fatal("Trace Fact schema ref does not bind the exact document")
	}
	document[0] = ' '
	if TraceFactJSONSchemaV1()[0] != '{' {
		t.Fatal("Trace Fact schema document leaked a mutable alias")
	}
}
