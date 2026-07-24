package hostlocal

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

func TestStoreV2EncryptedCreateOnceInspectAndNoAlias(t *testing.T) {
	now := time.Unix(1_900_400_000, 0).UTC()
	root := t.TempDir()
	store := hostLocalTestStoreV2(t, root, func() time.Time { return now })
	request := hostLocalPutRequestV2(now, []byte("workspace secret payload"))
	first, err := store.PutSnapshotContentV2(context.Background(), &request)
	if err != nil || !first.Created {
		t.Fatalf("first put=%#v err=%v", first, err)
	}
	second, err := store.PutSnapshotContentV2(context.Background(), &request)
	if err != nil || second.Created || second.StorageRef != first.StorageRef {
		t.Fatalf("replay put=%#v err=%v", second, err)
	}
	files, err := filepath.Glob(filepath.Join(root, "objects", "*", "*.snapshot"))
	if err != nil || len(files) != 1 {
		t.Fatalf("snapshot files=%v err=%v", files, err)
	}
	onDisk, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(onDisk, request.Content) {
		t.Fatal("host-local store persisted plaintext snapshot bytes")
	}
	inspected, err := store.InspectSnapshotContentV2(context.Background(), &contract.InspectSnapshotContentRequestV2{ExpectedRef: first.StorageRef})
	if err != nil || !bytes.Equal(inspected.Content, request.Content) {
		t.Fatalf("inspect=%#v err=%v", inspected, err)
	}
	inspected.Content[0] ^= 0xff
	again, err := store.InspectSnapshotContentV2(context.Background(), &contract.InspectSnapshotContentRequestV2{ExpectedRef: first.StorageRef})
	if err != nil || !bytes.Equal(again.Content, request.Content) {
		t.Fatal("host-local inspect leaked a mutable alias")
	}
}

func TestStoreV2TamperAndExactExpiryFailClosed(t *testing.T) {
	now := time.Unix(1_900_400_000, 0).UTC()
	clock := now
	root := t.TempDir()
	store := hostLocalTestStoreV2(t, root, func() time.Time { return clock })
	request := hostLocalPutRequestV2(now, []byte("tamper target"))
	result, err := store.PutSnapshotContentV2(context.Background(), &request)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(root, "objects", "*", "*.snapshot"))
	payload, _ := os.ReadFile(files[0])
	payload[len(payload)-3] ^= 0x1
	if err := os.WriteFile(files[0], payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectSnapshotContentV2(context.Background(), &contract.InspectSnapshotContentRequestV2{ExpectedRef: result.StorageRef}); err == nil {
		t.Fatal("tampered encrypted snapshot passed Inspect")
	}
	clock = time.Unix(0, result.StorageRef.ExpiresUnixNano)
	if _, err := store.InspectSnapshotContentV2(context.Background(), &contract.InspectSnapshotContentRequestV2{ExpectedRef: result.StorageRef}); err == nil {
		t.Fatal("snapshot remained current at exact expiry")
	}
}

func TestStoreV2MalformedExpectedIDFailsWithoutPanic(t *testing.T) {
	now := time.Unix(1_900_400_000, 0).UTC()
	store := hostLocalTestStoreV2(t, t.TempDir(), func() time.Time { return now })
	request := hostLocalPutRequestV2(now, []byte("valid payload"))
	result, err := store.PutSnapshotContentV2(context.Background(), &request)
	if err != nil {
		t.Fatal(err)
	}
	malformed := result.StorageRef
	malformed.StorageArtifactID = "x"
	malformed, err = contract.SealSnapshotStorageArtifactRefV2(malformed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectSnapshotContentV2(context.Background(), &contract.InspectSnapshotContentRequestV2{ExpectedRef: malformed}); err == nil {
		t.Fatal("malformed host-local storage ID passed Inspect")
	}
}

func TestStoreV2ConcurrentSameContentHasOneCreateAndOneExactRef(t *testing.T) {
	now := time.Unix(1_900_400_000, 0).UTC()
	store := hostLocalTestStoreV2(t, t.TempDir(), func() time.Time { return now })
	request := hostLocalPutRequestV2(now, []byte("concurrent workspace snapshot"))
	var wg sync.WaitGroup
	results := make(chan contract.PutSnapshotContentResultV2, 64)
	errs := make(chan error, 64)
	for index := 0; index < 64; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := store.PutSnapshotContentV2(context.Background(), &request)
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	created := 0
	var exact contract.SnapshotStorageArtifactRefV2
	for result := range results {
		if result.Created {
			created++
		}
		if exact.StorageArtifactID == "" {
			exact = result.StorageRef
		} else if result.StorageRef != exact {
			t.Fatalf("concurrent refs drifted: %#v != %#v", result.StorageRef, exact)
		}
	}
	if created != 1 {
		t.Fatalf("created=%d want=1", created)
	}
}

func hostLocalTestStoreV2(t *testing.T, root string, clock func() time.Time) *StoreV2 {
	t.Helper()
	now := clock()
	namespace := contract.SnapshotArtifactExactRefV2{TypeURL: "praxis.sandbox/host-local-namespace/v1", Version: 1, ID: "host-local-namespace", Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: "praxis.sandbox/host-local-namespace/body/v1", Digest: hostLocalRefV2("namespace").Digest, ExpiresUnixNano: now.Add(4 * time.Hour).UnixNano()}
	store, err := NewStoreV2(ConfigV2{Root: root, Key: bytes.Repeat([]byte{0x5a}, 32), Namespace: namespace, Clock: clock, MaxContentBytes: 1 << 20, MaxArtifactTTL: 2 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func hostLocalPutRequestV2(now time.Time, content []byte) contract.PutSnapshotContentRequestV2 {
	return contract.PutSnapshotContentRequestV2{TenantID: "tenant-1", DataDomain: "workspace-checkpoint", Content: append([]byte(nil), content...), SchemaRef: hostLocalRefV2("workspace-snapshot-schema"), EncryptionFactRef: hostLocalRefV2("encryption-fact"), ResidencyFactRef: hostLocalRefV2("residency-fact"), RequestedNotAfter: now.Add(3 * time.Hour).UnixNano()}
}

func hostLocalRefV2(id string) contract.Ref {
	digest, _ := contract.Digest("host-local-snapshot-test-ref-v2", id)
	return contract.Ref{ID: id, Revision: 1, Digest: digest}
}
