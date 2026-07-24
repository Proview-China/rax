package journal

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

// MemoryHostJournalStoreV2 is a deterministic reference-test CAS store. It is
// not a durable production State Plane.
type MemoryHostJournalStoreV2 struct {
	mu     sync.RWMutex
	values map[string]contract.HostJournalV2
}

func NewMemoryHostJournalStoreV2() *MemoryHostJournalStoreV2 {
	return &MemoryHostJournalStoreV2{values: map[string]contract.HostJournalV2{}}
}

func (s *MemoryHostJournalStoreV2) CreateHostJournalV2(ctx context.Context, desired contract.HostJournalV2) (contract.HostJournalV2, error) {
	if s == nil { return contract.HostJournalV2{}, contract.NewError(contract.ErrorUnavailable, "host_journal_store_missing", "HostV2 Journal store is nil") }
	if ctx == nil { return contract.HostJournalV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required") }
	if err := desired.Validate(); err != nil { return contract.HostJournalV2{}, err }
	key := desired.HostID + "\x00" + desired.StartID
	s.mu.Lock(); defer s.mu.Unlock()
	if current, ok := s.values[key]; ok {
		if current.Digest != desired.Digest { return contract.HostJournalV2{}, contract.NewError(contract.ErrorConflict, "host_journal_v2_conflict", "HostV2 Journal already exists with different content") }
		return cloneHostJournalV2(current), nil
	}
	s.values[key] = cloneHostJournalV2(desired)
	return cloneHostJournalV2(desired), nil
}

func (s *MemoryHostJournalStoreV2) CompareAndSwapHostJournalV2(ctx context.Context, expected contract.ExactRefV1, next contract.HostJournalV2) (contract.HostJournalV2, error) {
	if s == nil { return contract.HostJournalV2{}, contract.NewError(contract.ErrorUnavailable, "host_journal_store_missing", "HostV2 Journal store is nil") }
	if ctx == nil { return contract.HostJournalV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required") }
	if err := expected.Validate(); err != nil { return contract.HostJournalV2{}, err }; if err := next.Validate(); err != nil { return contract.HostJournalV2{}, err }
	key := next.HostID + "\x00" + next.StartID
	s.mu.Lock(); defer s.mu.Unlock()
	current, ok := s.values[key]
	if !ok { return contract.HostJournalV2{}, contract.NewError(contract.ErrorNotFound, "host_journal_v2_missing", "HostV2 Journal does not exist") }
	currentRef, _ := current.RefV2()
	if currentRef != expected { return contract.HostJournalV2{}, contract.NewError(contract.ErrorConflict, "host_journal_v2_cas_conflict", "HostV2 Journal expected Ref drifted") }
	if err := contract.ValidateHostJournalSuccessorV2(current, next); err != nil { return contract.HostJournalV2{}, err }
	s.values[key] = cloneHostJournalV2(next)
	return cloneHostJournalV2(next), nil
}

func (s *MemoryHostJournalStoreV2) InspectHostJournalV2(ctx context.Context, hostID, startID string) (contract.HostJournalV2, error) {
	if s == nil { return contract.HostJournalV2{}, contract.NewError(contract.ErrorUnavailable, "host_journal_store_missing", "HostV2 Journal store is nil") }
	if ctx == nil { return contract.HostJournalV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required") }
	if err := contract.ValidateIdentifierV1("host id", hostID); err != nil { return contract.HostJournalV2{}, err }; if err := contract.ValidateIdentifierV1("start id", startID); err != nil { return contract.HostJournalV2{}, err }
	s.mu.RLock(); defer s.mu.RUnlock()
	value, ok := s.values[hostID+"\x00"+startID]
	if !ok { return contract.HostJournalV2{}, contract.NewError(contract.ErrorNotFound, "host_journal_v2_missing", "HostV2 Journal does not exist") }
	return cloneHostJournalV2(value), nil
}

func cloneHostJournalV2(value contract.HostJournalV2) contract.HostJournalV2 {
	value.Operations = append([]contract.HostOperationAttemptV2(nil), value.Operations...)
	for index := range value.Operations {
		value.Operations[index].Inputs = append([]contract.HostOperationCoordinateV2(nil), value.Operations[index].Inputs...)
		if value.Operations[index].Result != nil { result := *value.Operations[index].Result; value.Operations[index].Result = &result }
	}
	return value
}
