package journal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

// MemoryHostStartClaimStoreV3 is a deterministic, same-lock reference store.
// It demonstrates the required V1/V2/V3 conflict domain and Claim+Input atomic
// visibility, but makes no production durability claim.
type MemoryHostStartClaimStoreV3 struct {
	mu       sync.RWMutex
	claims   map[string]contract.HostStartClaimV1
	bindings map[string]contract.HostStartClaimInputBindingV3
}

func NewMemoryHostStartClaimStoreV3() *MemoryHostStartClaimStoreV3 {
	return &MemoryHostStartClaimStoreV3{claims: map[string]contract.HostStartClaimV1{}, bindings: map[string]contract.HostStartClaimInputBindingV3{}}
}

func (s *MemoryHostStartClaimStoreV3) ClaimOrInspectHostStartV1(ctx context.Context, desired contract.HostStartClaimV1) (contract.HostStartClaimV1, error) {
	if s == nil {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorUnavailable, "host_start_claim_store_missing", "HostStart V3 reference store is nil")
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
	key := startClaimKeyV3(desired.HostID, desired.StartID)
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

func (s *MemoryHostStartClaimStoreV3) ClaimOrInspectHostStartV3(ctx context.Context, desired contract.HostStartClaimV1, input contract.HostStartClaimInputV3) (contract.HostStartClaimInputBindingV3, error) {
	if s == nil {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorUnavailable, "host_start_claim_store_missing", "HostStart V3 reference store is nil")
	}
	if ctx == nil {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	binding, err := contract.NewHostStartClaimInputBindingV3(desired, input)
	if err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	key := startClaimKeyV3(desired.HostID, desired.StartID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.claims[key]; ok {
		if !contract.SameHostStartClaimV1(current, desired) {
			return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorConflict, "host_start_claim_conflict", "HostID and StartID are permanently bound to another exact claim")
		}
	} else {
		s.claims[key] = desired
	}
	if current, ok := s.bindings[key]; ok {
		if current.BindingDigest != binding.BindingDigest {
			return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorConflict, "host_start_input_binding_v3_conflict", "HostStart InputV3 sidecar drifted")
		}
		return current, nil
	}
	s.bindings[key] = binding
	return binding, nil
}

func (s *MemoryHostStartClaimStoreV3) InspectHostStartClaimV1(ctx context.Context, hostID, startID string) (contract.HostStartClaimV1, error) {
	if s == nil {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorUnavailable, "host_start_claim_store_missing", "HostStart V3 reference store is nil")
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
	value, ok := s.claims[startClaimKeyV3(hostID, startID)]
	if !ok {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorNotFound, "host_start_claim_missing", "HostStart Claim does not exist")
	}
	return value, nil
}

func (s *MemoryHostStartClaimStoreV3) InspectHostStartClaimCurrentV1(ctx context.Context, expected contract.HostStartClaimRefV1) (contract.HostStartClaimV1, error) {
	if err := expected.Validate(); err != nil {
		return contract.HostStartClaimV1{}, err
	}
	actual, err := s.InspectHostStartClaimV1(ctx, expected.HostID, expected.StartID)
	if err != nil {
		return contract.HostStartClaimV1{}, err
	}
	ref, err := actual.CurrentRefV1()
	if err != nil {
		return contract.HostStartClaimV1{}, err
	}
	if ref != expected {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorConflict, "host_start_claim_ref_drift", "HostStart Claim exact Ref drifted")
	}
	return actual, nil
}

func (s *MemoryHostStartClaimStoreV3) InspectHostStartClaimInputV3(ctx context.Context, expected contract.HostStartClaimRefV1) (contract.HostStartClaimInputBindingV3, error) {
	if s == nil {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorUnavailable, "host_start_claim_store_missing", "HostStart V3 reference store is nil")
	}
	if ctx == nil {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := expected.Validate(); err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := startClaimKeyV3(expected.HostID, expected.StartID)
	claim, claimOK := s.claims[key]
	binding, bindingOK := s.bindings[key]
	if !claimOK {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorNotFound, "host_start_claim_missing", "HostStart Claim does not exist")
	}
	actualRef, err := claim.CurrentRefV1()
	if err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	if actualRef != expected {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorConflict, "host_start_claim_ref_drift", "HostStart Claim exact Ref drifted")
	}
	if !bindingOK {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorUnknownOutcome, "host_start_input_binding_v3_missing", "HostStart Claim exists without its required InputV3 sidecar")
	}
	if err := exactClaimInputBindingV3(binding, claim); err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	return binding, nil
}

type HostStartAdmissionV3 struct {
	facts ports.HostStartClaimPortV3
	now   func() time.Time
}

func NewHostStartAdmissionV3(facts ports.HostStartClaimPortV3, now func() time.Time) (*HostStartAdmissionV3, error) {
	if contract.IsTypedNilV1(facts) {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "host_start_claim_v3_port_missing", "HostStart Claim V3 port is required")
	}
	if now == nil {
		now = time.Now
	}
	return &HostStartAdmissionV3{facts: facts, now: now}, nil
}

func (a *HostStartAdmissionV3) ClaimV3(ctx context.Context, input contract.HostStartClaimInputV3) (contract.HostStartClaimInputBindingV3, error) {
	if a == nil || contract.IsTypedNilV1(a.facts) {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorUnavailable, "host_start_admission_v3_missing", "HostStart Admission V3 is unavailable")
	}
	if ctx == nil {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := input.ValidateV3(); err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	now, err := safeNowV1(a.now)
	if err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	if now.UnixNano() < input.CreatedUnixNano {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "HostStart InputV3 clock regressed")
	}
	if !now.Before(time.Unix(0, input.ExpiresUnixNano)) {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorPrecondition, "host_start_input_v3_expired", "HostStart InputV3 expired")
	}
	claim, err := input.ClaimV1()
	if err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	actual, writeErr := safeClaimHostStartV3(ctx, a.facts, claim, input)
	if writeErr == nil {
		return exactClaimInputBindingV3Result(actual, claim)
	}
	if !contract.HasCode(writeErr, contract.ErrorConflict) && !contract.HasCode(writeErr, contract.ErrorUnavailable) && !contract.HasCode(writeErr, contract.ErrorUnknownOutcome) {
		return contract.HostStartClaimInputBindingV3{}, writeErr
	}
	ref, _ := claim.CurrentRefV1()
	recovered, inspectErr := safeInspectHostStartV3(context.WithoutCancel(ctx), a.facts, ref)
	if inspectErr != nil {
		if contract.HasCode(inspectErr, contract.ErrorConflict) {
			return contract.HostStartClaimInputBindingV3{}, inspectErr
		}
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorUnknownOutcome, "host_start_claim_v3_outcome_unknown", "HostStart Claim/InputV3 outcome could not be proven by exact Inspect")
	}
	return exactClaimInputBindingV3Result(recovered, claim)
}

func safeClaimHostStartV3(ctx context.Context, facts ports.HostStartClaimPortV3, claim contract.HostStartClaimV1, input contract.HostStartClaimInputV3) (result contract.HostStartClaimInputBindingV3, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = contract.NewError(contract.ErrorUnknownOutcome, "host_start_claim_v3_panic", fmt.Sprintf("HostStart Claim/InputV3 mutation panicked: %v", recovered))
		}
	}()
	return facts.ClaimOrInspectHostStartV3(ctx, claim, input)
}
func safeInspectHostStartV3(ctx context.Context, facts ports.HostStartClaimPortV3, ref contract.HostStartClaimRefV1) (result contract.HostStartClaimInputBindingV3, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = contract.NewError(contract.ErrorUnavailable, "host_start_claim_v3_inspect_panic", fmt.Sprintf("HostStart InputV3 Inspect panicked: %v", recovered))
		}
	}()
	return facts.InspectHostStartClaimInputV3(ctx, ref)
}
func exactClaimInputBindingV3Result(binding contract.HostStartClaimInputBindingV3, desired contract.HostStartClaimV1) (contract.HostStartClaimInputBindingV3, error) {
	if err := binding.ValidateV3(); err != nil {
		return contract.HostStartClaimInputBindingV3{}, err
	}
	if binding.ClaimRef.HostID != desired.HostID || binding.ClaimRef.StartID != desired.StartID || binding.ClaimRef.Digest != desired.Digest {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorConflict, "host_start_claim_v3_exact_drift", "HostStart Claim/InputV3 exact result drifted")
	}
	return binding, nil
}
func exactClaimInputBindingV3(binding contract.HostStartClaimInputBindingV3, claim contract.HostStartClaimV1) error {
	if err := binding.ValidateV3(); err != nil {
		return err
	}
	expected, err := binding.Input.ClaimV1()
	if err != nil {
		return err
	}
	if !contract.SameHostStartClaimV1(expected, claim) {
		return contract.NewError(contract.ErrorConflict, "host_start_input_binding_v3_claim_drift", "HostStart InputV3 sidecar no longer matches its Claim")
	}
	return nil
}
func startClaimKeyV3(hostID, startID string) string { return hostID + "\x00" + startID }

var _ ports.HostStartClaimPortV3 = (*MemoryHostStartClaimStoreV3)(nil)
