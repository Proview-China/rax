package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestBindingAdmissionSchemaV4PreservesPriorSchemaProofs(t *testing.T) {
	store := openTestStore(t, testDBPath(t), func() time.Time { return time.Unix(2_400_000_000, 0) })
	want := map[int]core.Digest{
		schemaVersionV1: core.DigestBytes([]byte(schemaV1)),
		schemaVersionV2: core.DigestBytes([]byte(schemaV2)),
		schemaVersionV3: core.DigestBytes([]byte(schemaV3)),
		schemaVersionV4: core.DigestBytes([]byte(schemaV4)),
		schemaVersionV5: core.DigestBytes([]byte(schemaV5)),
		schemaVersionV6: core.DigestBytes([]byte(schemaV6)),
	}
	for version, expected := range want {
		var digest string
		if err := store.db.QueryRowContext(context.Background(), `SELECT digest FROM runtime_binding_schema WHERE version=?`, version).Scan(&digest); err != nil {
			t.Fatal(err)
		}
		if core.Digest(digest) != expected {
			t.Fatalf("schema %d proof drifted: got=%s want=%s", version, digest, expected)
		}
	}
	var table string
	if err := store.db.QueryRowContext(context.Background(), `SELECT name FROM sqlite_master WHERE type='table' AND name='runtime_binding_admission_attempts'`).Scan(&table); err != nil || table != "runtime_binding_admission_attempts" {
		t.Fatalf("Binding admission Attempt table is absent: table=%q err=%v", table, err)
	}
}
