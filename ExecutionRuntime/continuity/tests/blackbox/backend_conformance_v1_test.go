package blackbox_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/sqlite"
)

func TestSQLiteMetadataAndReferenceContentPassBackendSuiteV1(t *testing.T) {
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "continuity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	content := memory.New()
	report, err := conformance.RunBackendSuiteV1(context.Background(), conformance.BackendSuiteRequestV1{
		Namespace:   "sqlite-blackbox",
		NowUnixNano: 1_900_000_000_000_000_000,
		Metadata:    store,
		Content:     content,
		Retention:   store,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
}
