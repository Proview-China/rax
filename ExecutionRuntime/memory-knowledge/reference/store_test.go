package reference

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestStoreDeepCopiesAndIsIdempotent(t *testing.T) {
	t.Parallel()
	s := NewStore()
	input := []byte("authoritative content")
	ref, err := s.Put(input, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	input[0] = 'X'
	got, err := s.Get(ref)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "authoritative content" {
		t.Fatalf("input alias mutated store: %q", got)
	}
	got[0] = 'Y'
	again, err := s.Get(ref)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != "authoritative content" {
		t.Fatalf("output alias mutated store: %q", again)
	}
	second, err := s.Put([]byte("authoritative content"), "text/plain")
	if err != nil || second != ref || s.Len() != 1 {
		t.Fatalf("content-addressed put is not exact-idempotent: ref=%+v err=%v len=%d", second, err, s.Len())
	}
	if _, err := s.Put([]byte("authoritative content"), "application/octet-stream"); err == nil {
		t.Fatal("same bytes under a conflicting media type must not be exact-idempotent")
	}
}

func TestStoreConcurrentPut(t *testing.T) {
	t.Parallel()
	s := NewStore()
	const workers = 100
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			if _, err := s.Put([]byte("same"), "text/plain"); err != nil {
				t.Errorf("put: %v", err)
			}
		}()
	}
	wg.Wait()
	if s.Len() != 1 {
		t.Fatalf("want one content object, got %d", s.Len())
	}
}

func TestStoreRejectsCallerReferenceDrift(t *testing.T) {
	t.Parallel()
	s := NewStore()
	ref, err := s.Put([]byte("immutable"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		edit func(*contract.ContentRef)
	}{
		{name: "digest", edit: func(got *contract.ContentRef) { got.Digest = "sha256:" + string(make([]byte, 64)) }},
		{name: "length", edit: func(got *contract.ContentRef) { got.Length++ }},
		{name: "media type", edit: func(got *contract.ContentRef) { got.MediaType = "application/octet-stream" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drifted := ref
			tt.edit(&drifted)
			if _, err := s.Get(drifted); err == nil {
				t.Fatal("drifted reference was accepted")
			}
		})
	}
}

func TestStoreDetectsBackendTamper(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		tamper func(*Store, contract.ContentRef)
	}{
		{
			name: "content bytes",
			tamper: func(s *Store, ref contract.ContentRef) {
				s.items[ref.ID][0] ^= 0xff
			},
		},
		{
			name: "metadata length",
			tamper: func(s *Store, ref contract.ContentRef) {
				meta := s.meta[ref.ID]
				meta.Length++
				s.meta[ref.ID] = meta
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			ref, err := s.Put([]byte("immutable"), "text/plain")
			if err != nil {
				t.Fatal(err)
			}
			s.mu.Lock()
			tt.tamper(s, ref)
			s.mu.Unlock()
			if _, err := s.Get(ref); !errors.Is(err, contract.ErrEvidenceConflict) {
				t.Fatalf("tamper must fail closed with evidence conflict: %v", err)
			}
			if s.Has(ref) {
				t.Fatal("tampered content reported present")
			}
		})
	}
}

func TestStoreDoesNotConferCurrentnessOnExpiredFact(t *testing.T) {
	t.Parallel()
	s := NewStore()
	ref, err := s.Put([]byte("still retained"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	envelope := contract.Envelope{
		ContractVersion:   contract.VersionV1,
		SchemaRef:         "test/expired-v1",
		ID:                "expired",
		Revision:          1,
		TenantID:          "tenant",
		IdentityID:        "identity",
		IdentityEpoch:     1,
		AuthorityRef:      testRef("authority"),
		AuthorityEpoch:    1,
		PolicyRef:         testRef("policy"),
		Purpose:           "test",
		ActionScopeDigest: "sha256:scope",
		CreatedAt:         now.Add(-2 * time.Hour),
		UpdatedAt:         now.Add(-time.Hour),
		ExpiresAt:         now,
	}
	if _, err := s.Get(ref); err != nil {
		t.Fatalf("retained content unavailable: %v", err)
	}
	if err := envelope.ValidateCurrent(now); !errors.Is(err, contract.ErrNotCurrent) {
		t.Fatalf("backend availability must not make expired fact current: %v", err)
	}
}

func TestStoreMalformedUntrustedInputDoesNotPanic(t *testing.T) {
	t.Parallel()
	s := NewStore()
	malformed := []contract.ContentRef{
		{},
		{ID: "sha256:x", Digest: "sha256:x", MediaType: "text/plain"},
		{ID: "different", Digest: "sha256:" + string(make([]byte, 64)), MediaType: "text/plain"},
		{ID: "sha256:" + string(make([]byte, 64)), Digest: "sha256:" + string(make([]byte, 64)), Length: -1, MediaType: "text/plain"},
	}
	for i, ref := range malformed {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if _, err := s.Get(ref); err == nil {
				t.Fatal("malformed untrusted reference was accepted")
			}
		})
	}
	if _, err := s.Put(nil, " \t"); !errors.Is(err, contract.ErrInvalidArgument) {
		t.Fatalf("blank media type accepted: %v", err)
	}
}

func TestStoreConcurrentDistinctPutAndGet(t *testing.T) {
	t.Parallel()
	s := NewStore()
	const workers = 64
	refs := make([]contract.ContentRef, workers)
	for i := range workers {
		ref, err := s.Put([]byte(fmt.Sprintf("content-%03d", i)), "text/plain")
		if err != nil {
			t.Fatal(err)
		}
		refs[i] = ref
	}

	var wg sync.WaitGroup
	wg.Add(workers * 2)
	for i := range workers {
		i := i
		go func() {
			defer wg.Done()
			body, err := s.Get(refs[i])
			if err != nil || string(body) != fmt.Sprintf("content-%03d", i) {
				t.Errorf("get %d: body=%q err=%v", i, body, err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := s.Put([]byte(fmt.Sprintf("content-%03d", i)), "text/plain"); err != nil {
				t.Errorf("idempotent put %d: %v", i, err)
			}
		}()
	}
	wg.Wait()
	if s.Len() != workers {
		t.Fatalf("want %d immutable objects, got %d", workers, s.Len())
	}
}

func testRef(id string) contract.Ref {
	return contract.Ref{ID: id, Revision: 1, Digest: "sha256:" + id}
}
