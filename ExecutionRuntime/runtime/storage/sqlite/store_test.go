package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestStoreWALMigrationIntegrityRestartAndStrictJSON(t *testing.T) {
	path := testDBPath(t)
	base := time.Unix(2_300_000_000, 0)
	store := openTestStore(t, path, func() time.Time { return base.Add(2 * time.Second) })
	fact, _ := certifiedBinding(t, store, base, "set-a", "binding-a", "review/worker", "review/attest")
	if err := store.IntegrityCheckV1(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path, func() time.Time { return base.Add(2 * time.Second) })
	got, err := reopened.InspectBinding(context.Background(), fact.ID)
	if err != nil || got.Revision != fact.Revision {
		t.Fatalf("restart lost Binding Fact: %+v %v", got, err)
	}
	if _, err := reopened.db.Exec(`UPDATE runtime_binding_facts SET canonical_json=? WHERE id=?`, []byte(`{"id":"binding-a","id":"binding-b"}`), fact.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.InspectBinding(context.Background(), fact.ID); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("strict JSON corruption was accepted: %v", err)
	}
}

func TestStoreCanceledContextAndUnsupportedRenewalFailClosed(t *testing.T) {
	store := openTestStore(t, testDBPath(t), time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.InspectBinding(ctx, "missing"); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("canceled read was downgraded to another category: %v", err)
	}
	if _, err := store.RenewBindingSetV2(context.Background(), control.RenewBindingSetRequestV2{}); !core.HasCategory(err, core.ErrorCapabilityUnavailable) {
		t.Fatalf("cross-Owner renewal was not unsupported: %v", err)
	}
}

func TestStoreMigrationDigestDriftFailsReopen(t *testing.T) {
	path := testDBPath(t)
	store := openTestStore(t, path, time.Now)
	if _, err := store.db.Exec(`UPDATE runtime_binding_schema SET digest='sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' WHERE version=1`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := Open(context.Background(), Config{Path: path, Clock: time.Now}); !core.HasReason(err, core.ReasonInvalidDigest) {
		if reopened != nil {
			_ = reopened.Close()
		}
		t.Fatalf("schema digest drift did not fail reopen: %v", err)
	}
}

func TestStorePublicInterfaceShape(t *testing.T) {
	var _ control.BindingFactPortV2 = (*Store)(nil)
	var _ control.BindingRenewalPortV2 = (*Store)(nil)
	var _ ports.ReviewBindingAuthoritativeCurrentReaderV1 = (*Store)(nil)
	var _ ports.ReviewBindingConsumerAssociationCurrentReaderV1 = (*Store)(nil)
	var _ ports.ReviewBindingProjectionPublisherV1 = (*Store)(nil)
	var _ control.ReviewBindingAssociationProjectionPublisherV1 = (*Store)(nil)
}
