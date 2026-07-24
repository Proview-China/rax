package refstore

import (
	"sync"
	"testing"
)

func TestMemoryContentAddressedAndDefensiveCopies(t *testing.T) {
	store := NewMemory()
	original := []byte("immutable")
	ref1, err := store.Put(original)
	if err != nil {
		t.Fatal(err)
	}
	original[0] = 'X'
	ref2, err := store.Put([]byte("immutable"))
	if err != nil {
		t.Fatal(err)
	}
	if ref1 != ref2 || store.Len() != 1 {
		t.Fatalf("content addressing not idempotent: %#v %#v len=%d", ref1, ref2, store.Len())
	}
	got, err := store.Get(ref1)
	if err != nil {
		t.Fatal(err)
	}
	got[0] = 'Y'
	again, err := store.Get(ref1)
	if err != nil || string(again) != "immutable" {
		t.Fatalf("store leaked mutable bytes: %q %v", again, err)
	}
}

func TestMemoryConcurrentPutGet(t *testing.T) {
	store := NewMemory()
	ref, err := store.Put([]byte("shared"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if _, err := store.Put([]byte("shared")); err != nil {
					t.Error(err)
				}
				if got, err := store.Get(ref); err != nil || string(got) != "shared" {
					t.Errorf("get=%q err=%v", got, err)
				}
			}
		}()
	}
	wg.Wait()
}
