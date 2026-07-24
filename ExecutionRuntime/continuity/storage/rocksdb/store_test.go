//go:build cgo && continuity_rocksdb

package rocksdb_test

import (
	"context"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	continuityrocks "github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/rocksdb"
)

func TestStoreDurableCreateOnceIntegrityAndClone(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "content")
	store := openStore(t, path)
	data := []byte("durable-content-addressed-chunk")
	ref := chunkRef(data)
	if err := store.PutChunk(ctx, ref, data); err != nil {
		t.Fatal(err)
	}
	if err := store.PutChunk(ctx, ref, data); err != nil {
		t.Fatalf("lost put reply was not exact-idempotent: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openStore(t, path)
	defer store.Close()
	got, err := store.GetChunk(ctx, ref)
	if err != nil || string(got) != string(data) {
		t.Fatalf("reopen read: got=%q err=%v", got, err)
	}
	got[0] ^= 0xff
	again, err := store.GetChunk(ctx, ref)
	if err != nil || string(again) != string(data) {
		t.Fatalf("read leaked a mutable alias: got=%q err=%v", again, err)
	}
	wrongLength := ref
	wrongLength.Length++
	if _, err := store.GetChunk(ctx, wrongLength); !contract.HasCode(err, contract.ErrContentDigestMismatch) {
		t.Fatalf("same key with drifted exact ref did not fail closed: %v", err)
	}
	changed := append([]byte{}, data...)
	changed[0] ^= 0xff
	if err := store.PutChunk(ctx, ref, changed); !contract.HasCode(err, contract.ErrContentDigestMismatch) {
		t.Fatalf("same ref changed content was accepted: %v", err)
	}
	metrics := store.Metrics()
	if metrics.LatestSequenceNumber == 0 {
		t.Fatalf("write sequence was not observable: %#v", metrics)
	}
}

func TestStoreConcurrentSameContentIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, filepath.Join(t.TempDir(), "content"))
	defer store.Close()
	data := make([]byte, 64<<10)
	for i := range data {
		data[i] = byte(i)
	}
	ref := chunkRef(data)
	var wg sync.WaitGroup
	errorsByWorker := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errorsByWorker <- store.PutChunk(ctx, ref, data)
		}()
	}
	wg.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		if err != nil {
			t.Fatal(err)
		}
	}
	present, err := store.HasChunk(ctx, ref)
	if err != nil || !present {
		t.Fatalf("concurrent put left no inspectable value: present=%v err=%v", present, err)
	}
}

func TestStoreClosedAndCancelledFailWithoutIO(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "content"))
	data := []byte("chunk")
	ref := chunkRef(data)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.PutChunk(ctx, ref, data); !isContextError(err) {
		t.Fatalf("cancelled request reached storage: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.PutChunk(context.Background(), ref, data); !contract.HasCode(err, contract.ErrUnavailable) {
		t.Fatalf("closed store did not fail unavailable: %v", err)
	}
}

func BenchmarkContentStore(b *testing.B) {
	for _, size := range []int{4 << 10, 1 << 20} {
		size := size
		b.Run(fmt.Sprintf("put-sync/%d", size), func(b *testing.B) {
			store := openStore(b, filepath.Join(b.TempDir(), "content"))
			defer store.Close()
			ctx := context.Background()
			payload := make([]byte, size)
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				binary.LittleEndian.PutUint64(payload[:8], uint64(i))
				ref := chunkRef(payload)
				if err := store.PutChunk(ctx, ref, payload); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("get-verified/%d", size), func(b *testing.B) {
			store := openStore(b, filepath.Join(b.TempDir(), "content"))
			defer store.Close()
			ctx := context.Background()
			payload := make([]byte, size)
			ref := chunkRef(payload)
			if err := store.PutChunk(ctx, ref, payload); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := store.GetChunk(ctx, ref); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

type testingTB interface {
	Helper()
	TempDir() string
	Fatal(args ...any)
}

func openStore(tb testingTB, path string) *continuityrocks.Store {
	tb.Helper()
	store, err := continuityrocks.Open(path)
	if err != nil {
		tb.Fatal(err)
	}
	return store
}

func chunkRef(data []byte) contract.ChunkRef {
	return contract.ChunkRef{
		SchemaVersion: "content/v1", Digest: contract.DigestBytes(data), Length: int64(len(data)),
	}
}

func isContextError(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}
