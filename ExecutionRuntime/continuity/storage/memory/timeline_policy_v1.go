package memory

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

type timelinePolicyKeyV1 struct {
	ScopeDigest string
	PolicyID    string
}

func (b *Backend) CreateTimelineProjectionPolicyV1(_ context.Context, candidate contract.TimelineProjectionPolicyCurrentV1) (contract.TimelineProjectionPolicyCurrentV1, bool, error) {
	if err := candidate.Validate(); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, false, err
	}
	if candidate.Ref.Revision != 1 || candidate.State != contract.TimelineProjectionPolicyActiveV1 {
		return contract.TimelineProjectionPolicyCurrentV1{}, false, contract.NewError(contract.ErrRevisionConflict, "policy_create", "create requires active revision one")
	}
	key := timelinePolicyKeyV1{ScopeDigest: candidate.Ref.ScopeDigest, PolicyID: candidate.Ref.PolicyID}
	b.mu.Lock()
	defer b.mu.Unlock()
	if revision, ok := b.timelinePolicyCurrentV1[key]; ok {
		existing := b.timelinePoliciesV1[key][revision]
		if existing.Ref.Digest == candidate.Ref.Digest {
			return existing, true, nil
		}
		return contract.TimelineProjectionPolicyCurrentV1{}, false, contract.NewError(contract.ErrRevisionConflict, "policy_id", "create-once policy changed content")
	}
	b.timelinePoliciesV1[key] = map[uint64]contract.TimelineProjectionPolicyCurrentV1{1: candidate}
	b.timelinePolicyCurrentV1[key] = 1
	return candidate, false, nil
}

func (b *Backend) InspectTimelineProjectionPolicyV1(_ context.Context, ref contract.TimelineProjectionPolicyRefV1) (contract.TimelineProjectionPolicyCurrentV1, error) {
	if err := ref.Validate(); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	key := timelinePolicyKeyV1{ScopeDigest: ref.ScopeDigest, PolicyID: ref.PolicyID}
	b.mu.RLock()
	defer b.mu.RUnlock()
	value, ok := b.timelinePoliciesV1[key][ref.Revision]
	if !ok {
		return contract.TimelineProjectionPolicyCurrentV1{}, contract.NewError(contract.ErrNotFound, "policy_ref", "policy revision not found")
	}
	if value.Ref != ref {
		return contract.TimelineProjectionPolicyCurrentV1{}, contract.NewError(contract.ErrRevisionConflict, "policy_ref", "policy digest drifted")
	}
	return value, nil
}

func (b *Backend) InspectTimelineProjectionPolicyCurrentV1(_ context.Context, policyID, scopeDigest string) (contract.TimelineProjectionPolicyCurrentV1, error) {
	if err := contract.ValidateToken("policy_id", policyID); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	if err := contract.ValidateToken("scope_digest", scopeDigest); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	key := timelinePolicyKeyV1{ScopeDigest: scopeDigest, PolicyID: policyID}
	b.mu.RLock()
	defer b.mu.RUnlock()
	revision, ok := b.timelinePolicyCurrentV1[key]
	if !ok {
		return contract.TimelineProjectionPolicyCurrentV1{}, contract.NewError(contract.ErrNotFound, "policy_id", "current policy not found")
	}
	return b.timelinePoliciesV1[key][revision], nil
}

func (b *Backend) ValidateTimelineProjectionPolicyCurrentV1(ctx context.Context, expected contract.TimelineProjectionPolicyCurrentV1) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	current, err := b.InspectTimelineProjectionPolicyCurrentV1(ctx, expected.Ref.PolicyID, expected.Ref.ScopeDigest)
	if err != nil {
		return err
	}
	if current.Ref != expected.Ref {
		return contract.NewError(contract.ErrRevisionConflict, "policy_ref", "policy current index advanced")
	}
	return current.ValidateCurrent(expected.Ref, b.clock())
}

func (b *Backend) CompareAndSwapTimelineProjectionPolicyV1(_ context.Context, expected contract.TimelineProjectionPolicyRefV1, next contract.TimelineProjectionPolicyCurrentV1) (contract.TimelineProjectionPolicyCurrentV1, error) {
	if err := expected.Validate(); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	if err := next.Validate(); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	key := timelinePolicyKeyV1{ScopeDigest: expected.ScopeDigest, PolicyID: expected.PolicyID}
	b.mu.Lock()
	defer b.mu.Unlock()
	revision, ok := b.timelinePolicyCurrentV1[key]
	if !ok {
		return contract.TimelineProjectionPolicyCurrentV1{}, contract.NewError(contract.ErrNotFound, "policy_id", "current policy not found")
	}
	current := b.timelinePoliciesV1[key][revision]
	if current.Ref != expected {
		return contract.TimelineProjectionPolicyCurrentV1{}, contract.NewError(contract.ErrRevisionConflict, "policy_ref", "CAS expected ref is stale")
	}
	if err := contract.ValidateTimelineProjectionPolicySuccessorV1(current, next); err != nil {
		return contract.TimelineProjectionPolicyCurrentV1{}, err
	}
	b.timelinePoliciesV1[key][next.Ref.Revision] = next
	b.timelinePolicyCurrentV1[key] = next.Ref.Revision
	return next, nil
}
