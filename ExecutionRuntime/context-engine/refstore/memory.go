package refstore

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type Memory struct {
	mu      sync.RWMutex
	content map[string][]byte
}

func NewMemory() *Memory {
	return &Memory{content: make(map[string][]byte)}
}

func (m *Memory) Put(value []byte) (contract.ContentRef, error) {
	if len(value) == 0 {
		return contract.ContentRef{}, fmt.Errorf("%w: empty content", contract.ErrInvalid)
	}
	digest := contract.DigestBytes(value)
	ref := contract.ContentRef{Ref: string(digest), Digest: digest, Length: uint64(len(value))}
	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.content[ref.Ref]; ok && !bytes.Equal(current, value) {
		return contract.ContentRef{}, fmt.Errorf("%w: content address collision", contract.ErrConflict)
	}
	m.content[ref.Ref] = append([]byte(nil), value...)
	return ref, nil
}

func (m *Memory) Get(ref contract.ContentRef) ([]byte, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	value, ok := m.content[ref.Ref]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: content reference", contract.ErrUnknown)
	}
	if uint64(len(value)) != ref.Length || contract.DigestBytes(value) != ref.Digest {
		return nil, fmt.Errorf("%w: content reference mismatch", contract.ErrConflict)
	}
	return append([]byte(nil), value...), nil
}

func (m *Memory) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.content)
}
