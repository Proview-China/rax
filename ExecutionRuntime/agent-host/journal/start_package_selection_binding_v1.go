package journal

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

// MemoryHostStartPackageSelectionStoreV1 is a same-lock reference
// implementation. It makes no production durability claim.
type MemoryHostStartPackageSelectionStoreV1 struct {
	claims   *MemoryHostStartClaimStoreV3
	bindings map[string]contract.HostStartPackageSelectionBindingV1
}

func NewMemoryHostStartPackageSelectionStoreV1() *MemoryHostStartPackageSelectionStoreV1 {
	return &MemoryHostStartPackageSelectionStoreV1{
		claims:   NewMemoryHostStartClaimStoreV3(),
		bindings: map[string]contract.HostStartPackageSelectionBindingV1{},
	}
}

func (store *MemoryHostStartPackageSelectionStoreV1) ClaimOrInspectHostStartPackageSelectionV1(
	ctx context.Context,
	desired contract.HostStartClaimV1,
	input contract.HostStartClaimInputV3,
	desiredBinding contract.HostStartPackageSelectionBindingV1,
) (contract.HostStartPackageSelectionBindingV1, error) {
	if store == nil || store.claims == nil {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorUnavailable, "host_start_package_selection_store_missing", "HostStart Package Selection reference store is unavailable")
	}
	if ctx == nil {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if ctx.Err() != nil {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorUnknownOutcome, "context_ended", "HostStart Package Selection mutation outcome is unknown")
	}
	inputBinding, err := contract.NewHostStartClaimInputBindingV3(desired, input)
	if err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	if err = desiredBinding.ValidateAgainstClaimInputV1(desired, inputBinding); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	key := startClaimKeyV3(desired.HostID, desired.StartID)
	store.claims.mu.Lock()
	defer store.claims.mu.Unlock()

	currentClaim, claimExists := store.claims.claims[key]
	currentInput, inputExists := store.claims.bindings[key]
	currentBinding, bindingExists := store.bindings[key]
	if claimExists {
		if !contract.SameHostStartClaimV1(currentClaim, desired) {
			return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorConflict, "host_start_claim_conflict", "HostID and StartID are permanently bound to another exact Claim")
		}
		if !inputExists || !bindingExists {
			return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorPrecondition, "host_start_package_selection_atomic_proof_missing", "existing HostStart Claim was not created with the atomic Package Selection proof")
		}
		if currentInput.BindingDigest != inputBinding.BindingDigest ||
			currentBinding.Ref != desiredBinding.Ref ||
			!reflect.DeepEqual(currentBinding, desiredBinding) {
			return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorConflict, "host_start_package_selection_binding_conflict", "HostStart Claim is permanently bound to another Package Selection association")
		}
		return currentBinding, nil
	}
	if inputExists || bindingExists {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorConflict, "host_start_package_selection_orphan", "HostStart Package Selection sidecar exists without its Claim")
	}
	store.claims.claims[key] = desired
	store.claims.bindings[key] = inputBinding
	store.bindings[key] = desiredBinding
	return desiredBinding, nil
}

func (store *MemoryHostStartPackageSelectionStoreV1) InspectHostStartClaimCurrentV1(
	ctx context.Context,
	expected contract.HostStartClaimRefV1,
) (contract.HostStartClaimV1, error) {
	if store == nil || store.claims == nil {
		return contract.HostStartClaimV1{}, contract.NewError(contract.ErrorUnavailable, "host_start_package_selection_store_missing", "HostStart Package Selection reference store is unavailable")
	}
	return store.claims.InspectHostStartClaimCurrentV1(ctx, expected)
}

func (store *MemoryHostStartPackageSelectionStoreV1) InspectHostStartClaimInputV3(
	ctx context.Context,
	expected contract.HostStartClaimRefV1,
) (contract.HostStartClaimInputBindingV3, error) {
	if store == nil || store.claims == nil {
		return contract.HostStartClaimInputBindingV3{}, contract.NewError(contract.ErrorUnavailable, "host_start_package_selection_store_missing", "HostStart Package Selection reference store is unavailable")
	}
	return store.claims.InspectHostStartClaimInputV3(ctx, expected)
}

func (store *MemoryHostStartPackageSelectionStoreV1) InspectHostStartPackageSelectionBindingV1(
	ctx context.Context,
	expected contract.HostStartPackageSelectionBindingRefV1,
) (contract.HostStartPackageSelectionBindingV1, error) {
	if err := expected.Validate(); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	actual, err := store.InspectHostStartPackageSelectionBindingForClaimV1(ctx, expected.ClaimRef)
	if err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	if actual.Ref != expected {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorConflict, "host_start_package_selection_binding_ref_drift", "HostStart Package Selection binding exact Ref drifted")
	}
	return actual, nil
}

func (store *MemoryHostStartPackageSelectionStoreV1) InspectHostStartPackageSelectionBindingForClaimV1(
	ctx context.Context,
	expected contract.HostStartClaimRefV1,
) (contract.HostStartPackageSelectionBindingV1, error) {
	if store == nil || store.claims == nil {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorUnavailable, "host_start_package_selection_store_missing", "HostStart Package Selection reference store is unavailable")
	}
	if ctx == nil {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if ctx.Err() != nil {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorUnavailable, "context_ended", "HostStart Package Selection Inspect context ended")
	}
	if err := expected.Validate(); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	store.claims.mu.RLock()
	defer store.claims.mu.RUnlock()
	key := startClaimKeyV3(expected.HostID, expected.StartID)
	claim, claimExists := store.claims.claims[key]
	input, inputExists := store.claims.bindings[key]
	binding, bindingExists := store.bindings[key]
	if !claimExists {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorNotFound, "host_start_claim_missing", "HostStart Claim does not exist")
	}
	actualClaimRef, err := claim.CurrentRefV1()
	if err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	if actualClaimRef != expected {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorConflict, "host_start_claim_ref_drift", "HostStart Claim exact Ref drifted")
	}
	if !inputExists || !bindingExists {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorPrecondition, "host_start_package_selection_atomic_proof_missing", "HostStart Claim lacks its atomic Package Selection proof")
	}
	if err = binding.ValidateAgainstClaimInputV1(claim, input); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	return binding, nil
}

type HostStartPackageSelectionAdmissionV1 struct {
	facts hostports.HostStartClaimPackageSelectionPortV1
	now   func() time.Time
}

func NewHostStartPackageSelectionAdmissionV1(
	facts hostports.HostStartClaimPackageSelectionPortV1,
	now func() time.Time,
) (*HostStartPackageSelectionAdmissionV1, error) {
	if contract.IsTypedNilV1(facts) {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "host_start_package_selection_port_missing", "HostStart Package Selection composite port is required")
	}
	if now == nil {
		now = time.Now
	}
	return &HostStartPackageSelectionAdmissionV1{facts: facts, now: now}, nil
}

func (admission *HostStartPackageSelectionAdmissionV1) ClaimV1(
	ctx context.Context,
	claim contract.HostStartClaimV1,
	input contract.HostStartClaimInputV3,
	binding contract.HostStartPackageSelectionBindingV1,
) (contract.HostStartPackageSelectionBindingV1, error) {
	if admission == nil || contract.IsTypedNilV1(admission.facts) || admission.now == nil {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorUnavailable, "host_start_package_selection_admission_missing", "HostStart Package Selection admission is unavailable")
	}
	if ctx == nil {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	now := admission.now()
	if err := binding.ValidateCurrentV1(binding.Ref, now); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	actual, writeErr := safeClaimHostStartPackageSelectionV1(ctx, admission.facts, claim, input, binding)
	if writeErr == nil {
		return exactHostStartPackageSelectionResultV1(actual, binding)
	}
	if !contract.HasCode(writeErr, contract.ErrorUnknownOutcome) {
		return contract.HostStartPackageSelectionBindingV1{}, writeErr
	}
	recovered, inspectErr := safeInspectHostStartPackageSelectionV1(context.WithoutCancel(ctx), admission.facts, binding.Ref)
	if inspectErr != nil {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorUnknownOutcome, "host_start_package_selection_outcome_unknown", "HostStart Package Selection outcome could not be proven by exact Inspect")
	}
	return exactHostStartPackageSelectionResultV1(recovered, binding)
}

func safeClaimHostStartPackageSelectionV1(
	ctx context.Context,
	facts hostports.HostStartClaimPackageSelectionPortV1,
	claim contract.HostStartClaimV1,
	input contract.HostStartClaimInputV3,
	binding contract.HostStartPackageSelectionBindingV1,
) (result contract.HostStartPackageSelectionBindingV1, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = contract.NewError(contract.ErrorUnknownOutcome, "host_start_package_selection_mutation_panic", fmt.Sprintf("HostStart Package Selection mutation panicked: %v", recovered))
		}
	}()
	return facts.ClaimOrInspectHostStartPackageSelectionV1(ctx, claim, input, binding)
}

func safeInspectHostStartPackageSelectionV1(
	ctx context.Context,
	facts hostports.HostStartClaimPackageSelectionPortV1,
	expected contract.HostStartPackageSelectionBindingRefV1,
) (result contract.HostStartPackageSelectionBindingV1, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = contract.NewError(contract.ErrorUnavailable, "host_start_package_selection_inspect_panic", fmt.Sprintf("HostStart Package Selection Inspect panicked: %v", recovered))
		}
	}()
	return facts.InspectHostStartPackageSelectionBindingV1(ctx, expected)
}

func exactHostStartPackageSelectionResultV1(
	actual contract.HostStartPackageSelectionBindingV1,
	expected contract.HostStartPackageSelectionBindingV1,
) (contract.HostStartPackageSelectionBindingV1, error) {
	if err := actual.ValidateHistoricalV1(); err != nil {
		return contract.HostStartPackageSelectionBindingV1{}, err
	}
	if actual.Ref != expected.Ref || !reflect.DeepEqual(actual, expected) {
		return contract.HostStartPackageSelectionBindingV1{}, contract.NewError(contract.ErrorConflict, "host_start_package_selection_exact_drift", "HostStart Package Selection exact result drifted")
	}
	return actual, nil
}

var _ hostports.HostStartClaimPackageSelectionPortV1 = (*MemoryHostStartPackageSelectionStoreV1)(nil)
