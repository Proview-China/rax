package journal

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

// MemoryHostStartClaimStoreV1 is a deterministic reference-test store. It does
// not claim durable production conformance.
type MemoryHostStartClaimStoreV1 struct {
	mu     sync.RWMutex
	claims map[string]contract.HostStartClaimV1
}

func NewMemoryHostStartClaimStoreV1() *MemoryHostStartClaimStoreV1 {
	return &MemoryHostStartClaimStoreV1{claims: map[string]contract.HostStartClaimV1{}}
}

func (s *MemoryHostStartClaimStoreV1) ClaimOrInspectHostStartV1(ctx context.Context, desired contract.HostStartClaimV1) (contract.HostStartClaimV1, error) {
	if s == nil {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorUnavailable, "host_start_claim_store_missing", "host start claim store is nil")
	}
	if ctx == nil {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := desired.ValidateHistoricalV1(); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	if desired.HostContractVersion == contract.HostLifecycleContractVersionV3 {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorPrecondition, "host_start_v3_atomic_port_required", "HostStart V3 must create its Claim and Input sidecar through the atomic V3 port")
	}
	key := desired.HostID + "\x00" + desired.StartID
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.claims[key]; ok {
		if !contract.SameHostStartClaimV1(current, desired) {
			return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorConflict, "host_start_claim_conflict", "HostID and StartID are permanently bound to another exact claim")
		}
		return current, nil
	}
	s.claims[key] = desired
	return desired, nil
}

func (s *MemoryHostStartClaimStoreV1) InspectHostStartClaimV1(ctx context.Context, hostID, startID string) (contract.HostStartClaimV1, error) {
	if s == nil {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorUnavailable, "host_start_claim_store_missing", "host start claim store is nil")
	}
	if ctx == nil {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := contract.ValidateIdentifierV1("host id", hostID); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	if err := contract.ValidateIdentifierV1("start id", startID); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.claims[hostID+"\x00"+startID]
	if !ok {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorNotFound, "host_start_claim_missing", "host start claim does not exist")
	}
	return value, nil
}

func (s *MemoryHostStartClaimStoreV1) InspectHostStartClaimCurrentV1(ctx context.Context, expected contract.HostStartClaimRefV1) (contract.HostStartClaimV1, error) {
	if s == nil {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorUnavailable, "host_start_claim_store_missing", "host start claim store is nil")
	}
	if ctx == nil {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := expected.Validate(); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	actual, err := s.InspectHostStartClaimV1(ctx, expected.HostID, expected.StartID)
	if err != nil {
		return contract.HostStartClaimV1{}, err
	}
	actualRef, err := actual.CurrentRefV1()
	if err != nil {
		return contract.HostStartClaimV1{}, err
	}
	if actualRef != expected {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorConflict, "host_start_claim_ref_drift", "HostStart Claim exact Ref drifted")
	}
	return actual, nil
}
