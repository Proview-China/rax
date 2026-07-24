package sqlite

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestStrictSnapshotJSONRejectsDuplicateUnknownAndTrailingV1(t *testing.T) {
	cases := []string{`{"contract_version":"a","contract_version":"a"}`, `{"unknown":true}`, `{} {}`}
	for _, payload := range cases {
		var target memory.SnapshotV1
		if err := decodeSnapshotStrictV1([]byte(payload), &target); err == nil {
			t.Fatalf("unsafe snapshot JSON accepted: %s", payload)
		}
	}
}
func TestStrictSnapshotJSONPreservesTypedDuplicateErrorV1(t *testing.T) {
	var target memory.SnapshotV1
	err := decodeSnapshotStrictV1([]byte(`{"tenant_id":"a","tenant_id":"a"}`), &target)
	if !core.HasReason(err, core.ReasonDuplicateCanonicalKey) {
		t.Fatalf("duplicate key classification lost: %v", err)
	}
}
