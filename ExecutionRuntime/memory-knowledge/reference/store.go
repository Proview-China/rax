package reference

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

// Store is an in-memory, content-addressed reference store for Wave 1. It is a
// testable local backend, not a production persistence or SLA claim.
type Store struct {
	mu    sync.RWMutex
	items map[string][]byte
	meta  map[string]contract.ContentRef
}

func NewStore() *Store {
	return &Store{items: make(map[string][]byte), meta: make(map[string]contract.ContentRef)}
}

func (s *Store) Put(content []byte, mediaType string) (contract.ContentRef, error) {
	if strings.TrimSpace(mediaType) == "" {
		return contract.ContentRef{}, fmt.Errorf("%w: media type required", contract.ErrInvalidArgument)
	}
	ref := contentRef(content, mediaType)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.items[ref.ID]; ok {
		if !bytes.Equal(existing, content) || s.meta[ref.ID] != ref {
			return contract.ContentRef{}, contract.ErrEvidenceConflict
		}
		return s.meta[ref.ID], nil
	}
	s.items[ref.ID] = append([]byte(nil), content...)
	s.meta[ref.ID] = ref
	return ref, nil
}

func (s *Store) Get(ref contract.ContentRef) ([]byte, error) {
	if err := validateContentRef(ref); err != nil {
		return nil, err
	}
	s.mu.RLock()
	content, ok := s.items[ref.ID]
	meta := s.meta[ref.ID]
	content = append([]byte(nil), content...)
	s.mu.RUnlock()
	if !ok {
		return nil, contract.ErrNotFound
	}
	if err := validateContentRef(meta); err != nil || meta != ref || contentRef(content, meta.MediaType) != meta {
		return nil, contract.ErrEvidenceConflict
	}
	return content, nil
}

func (s *Store) Has(ref contract.ContentRef) bool {
	_, err := s.Get(ref)
	return err == nil
}

func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

func contentRef(content []byte, mediaType string) contract.ContentRef {
	sum := sha256.Sum256(content)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	return contract.ContentRef{ID: digest, Digest: digest, Length: int64(len(content)), MediaType: mediaType}
}

func validateContentRef(ref contract.ContentRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if ref.ID != ref.Digest || !strings.HasPrefix(ref.Digest, "sha256:") {
		return fmt.Errorf("%w: non-canonical content ref", contract.ErrInvalidArgument)
	}
	raw := strings.TrimPrefix(ref.Digest, "sha256:")
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("%w: invalid sha256 digest", contract.ErrInvalidArgument)
	}
	return nil
}
