//go:build cgo && continuity_rocksdb

package rocksdb_test

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/rocksdb"
)

func TestRocksDBContentPassesBackendSuiteV1(t *testing.T) {
	content, err := rocksdb.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer content.Close()
	metadata := memory.New()
	report, err := conformance.RunBackendSuiteV1(context.Background(), conformance.BackendSuiteRequestV1{
		Namespace:   "rocksdb-conformance",
		NowUnixNano: 1_900_000_000_000_000_000,
		Metadata:    metadata,
		Content:     content,
		Retention:   metadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
}
