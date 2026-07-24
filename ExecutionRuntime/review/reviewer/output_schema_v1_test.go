package reviewer_test

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/reviewer"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestBuiltinOutputSchemaReaderV1ExactCloneAndClosedErrors(t *testing.T) {
	reader, err := reviewer.NewBuiltinOutputSchemaReaderV1()
	if err != nil {
		t.Fatal(err)
	}
	first, err := reader.InspectAutoReviewerOutputSchemaV1(context.Background(), mustBuiltinSchemaV1(t))
	if err != nil {
		t.Fatal(err)
	}
	first.Document[0] = '['
	second, err := reader.InspectAutoReviewerOutputSchemaV1(context.Background(), mustBuiltinSchemaV1(t))
	if err != nil || second.Document[0] != '{' {
		t.Fatalf("reader leaked schema bytes: %v", err)
	}

	wrong := second.Schema
	wrong.ContentDigest = core.DigestBytes([]byte("other"))
	if _, err := reader.InspectAutoReviewerOutputSchemaV1(context.Background(), wrong); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("wrong exact schema ref was not NotFound: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.InspectAutoReviewerOutputSchemaV1(cancelled, second.Schema); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("cancelled schema read was not indeterminate: %v", err)
	}
}

func mustBuiltinSchemaV1(t *testing.T) runtimeports.SchemaRefV2 {
	t.Helper()
	document, err := contract.BuiltinAutoReviewerOutputSchemaDocumentV1()
	if err != nil {
		t.Fatal(err)
	}
	return document.Schema
}
