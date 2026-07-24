package sqlite

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestResourceBindingSQLiteLostRepliesInspectAndAtomicSet(t *testing.T) {
	now := time.Unix(2_430_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	handles, set := resourceBindingSQLiteFixtureV1(t, now)
	if _, err := store.EnsureResourceBindingSetCurrentV1(context.Background(), set); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("set without all exact handles was accepted: %v", err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT count(*) FROM runtime_resource_binding_set_history`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed set leaked history: count=%d err=%v", count, err)
	}
	store.loseNextReplyForTest()
	if _, err := store.EnsureResourceHandleCurrentV1(context.Background(), handles[0]); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost handle reply was not indeterminate: %v", err)
	}
	if current, err := store.InspectResourceHandleCurrentV1(context.Background(), handles[0].Ref); err != nil || current.ProjectionDigest != handles[0].ProjectionDigest {
		t.Fatalf("lost handle reply did not recover by exact Inspect: %+v err=%v", current, err)
	}
	if _, err := store.EnsureResourceHandleCurrentV1(context.Background(), handles[1]); err != nil {
		t.Fatal(err)
	}
	store.loseNextReplyForTest()
	if _, err := store.EnsureResourceBindingSetCurrentV1(context.Background(), set); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost set reply was not indeterminate: %v", err)
	}
	if current, err := store.InspectResourceBindingSetCurrentV1(context.Background(), set.Ref); err != nil || current.ProjectionDigest != set.ProjectionDigest {
		t.Fatalf("lost set reply did not recover by exact Inspect: %+v err=%v", current, err)
	}
	changed := handles[0]
	changed.CleanupContract.Digest = core.DigestBytes([]byte("changed-cleanup"))
	changed.Ref.Digest, changed.ProjectionDigest = "", ""
	changed, err := ports.SealResourceHandleCurrentV1(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureResourceHandleCurrentV1(context.Background(), changed); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same handle identity changed content was accepted: %v", err)
	}
}

func TestResourceBindingSQLiteConcurrent64StoresOneHistory(t *testing.T) {
	now := time.Unix(2_440_000_000, 0)
	handles, set := resourceBindingSQLiteFixtureV1(t, now)
	path := filepath.Join(t.TempDir(), "resources.db")
	const workers = 64
	stores := make([]*Store, 0, workers)
	for range workers {
		store, err := Open(context.Background(), Config{Path: path, BusyTimeout: 5 * time.Second, MaxOpenConns: 2, Clock: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		stores = append(stores, store)
	}
	t.Cleanup(func() {
		for _, store := range stores {
			_ = store.Close()
		}
	})
	var wait sync.WaitGroup
	wait.Add(workers)
	for _, store := range stores {
		go func(s *Store) {
			defer wait.Done()
			for _, handle := range handles {
				if _, err := s.EnsureResourceHandleCurrentV1(context.Background(), handle); err != nil {
					t.Errorf("concurrent handle Ensure failed: %v", err)
					return
				}
			}
			if _, err := s.EnsureResourceBindingSetCurrentV1(context.Background(), set); err != nil {
				t.Errorf("concurrent set Ensure failed: %v", err)
			}
		}(store)
	}
	wait.Wait()
	var handleHistory, setHistory int
	if err := stores[0].db.QueryRow(`SELECT count(*) FROM runtime_resource_handle_history`).Scan(&handleHistory); err != nil {
		t.Fatal(err)
	}
	if err := stores[0].db.QueryRow(`SELECT count(*) FROM runtime_resource_binding_set_history`).Scan(&setHistory); err != nil {
		t.Fatal(err)
	}
	if handleHistory != len(handles) || setHistory != 1 {
		t.Fatalf("concurrent stores duplicated history: handles=%d sets=%d", handleHistory, setHistory)
	}
	if err := stores[0].IntegrityCheckV1(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestResourceBindingSQLiteTTLClockNoRawHandleAndCreateOnce(t *testing.T) {
	now := time.Unix(2_450_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	handles, _ := resourceBindingSQLiteFixtureV1(t, now)
	future := handles[0]
	future.CheckedUnixNano = now.Add(time.Second).UnixNano()
	future.Ref.Digest, future.ProjectionDigest = "", ""
	future, err := ports.SealResourceHandleCurrentV1(future)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureResourceHandleCurrentV1(context.Background(), future); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("future current projection was accepted: %v", err)
	}
	if _, ok := reflect.TypeOf((*ports.ResourceOwnerRepositoryV1)(nil)).Elem().MethodByName("CompareAndSwapResourceHandleV1"); ok {
		t.Fatal("create-once Resource Owner repository unexpectedly exposes CAS")
	}
	if _, err := store.EnsureResourceHandleCurrentV1(context.Background(), handles[0]); err != nil {
		t.Fatal(err)
	}
	var payload string
	if err := store.db.QueryRow(`SELECT canonical_json FROM runtime_resource_handle_history LIMIT 1`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"raw_handle", "credential_value", "secret_value", "private_key"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("serialized public resource ref contains forbidden field %q", forbidden)
		}
	}
	differentRevision := handles[0]
	differentRevision.Ref.Revision++
	differentRevision.Ref.Digest, differentRevision.ProjectionDigest = "", ""
	differentRevision, err = ports.SealResourceHandleCurrentV1(differentRevision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureResourceHandleCurrentV1(context.Background(), differentRevision); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("create-once identity accepted a revision advance without a contract: %v", err)
	}
}

func TestResourceBindingSQLiteTypedNilFailsClosed(t *testing.T) {
	var store *Store
	now := time.Unix(2_460_000_000, 0)
	handles, set := resourceBindingSQLiteFixtureV1(t, now)
	if _, err := store.EnsureResourceHandleCurrentV1(context.Background(), handles[0]); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("typed nil handle repository did not fail closed: %v", err)
	}
	if _, err := store.EnsureResourceBindingSetCurrentV1(context.Background(), set); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("typed nil set repository did not fail closed: %v", err)
	}
}

func resourceBindingSQLiteFixtureV1(t *testing.T, now time.Time) ([]ports.ResourceHandleCurrentV1, ports.ResourceBindingSetV1) {
	t.Helper()
	expires := now.Add(5 * time.Minute)
	handles := make([]ports.ResourceHandleCurrentV1, 0, 2)
	bindings := make([]ports.ResourceBindingV1, 0, 2)
	for index, component := range []ports.ComponentIDV2{"praxis/context", "praxis/runtime"} {
		owner := core.OwnerRef{Domain: "praxis.resource", ID: core.OwnerID("owner-" + string(component))}
		cleanup := ports.OwnerCurrentRefV1{Owner: owner, ContractVersion: "1.0.0", ID: "cleanup", Revision: 1, Digest: core.DigestBytes([]byte("cleanup-" + string(component))), ExpiresUnixNano: expires.UnixNano()}
		deployment := ports.OwnerCurrentRefV1{Owner: owner, ContractVersion: "1.0.0", ID: "deployment", Revision: 1, Digest: core.DigestBytes([]byte("deployment-" + string(component))), ExpiresUnixNano: expires.UnixNano()}
		handle, err := ports.SealResourceHandleCurrentV1(ports.ResourceHandleCurrentV1{Ref: ports.ResourceHandleRefV1{Owner: owner, ID: "resource-" + string(rune('a'+index)), Revision: 1, Kind: "praxis/sqlite", ScopeDigest: core.DigestBytes([]byte("scope-" + string(component)))}, CleanupContract: cleanup, DeploymentAttestation: deployment, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
		if err != nil {
			t.Fatal(err)
		}
		handles = append(handles, handle)
		bindings = append(bindings, ports.ResourceBindingV1{ComponentID: component, Handle: handle.Ref, CleanupContract: cleanup, DeploymentAttestation: deployment})
	}
	set, err := ports.SealResourceBindingSetV1(ports.ResourceBindingSetV1{Ref: ports.ResourceBindingSetRefV1{ID: "resource-set", Revision: 1}, Bindings: bindings, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return handles, set
}
