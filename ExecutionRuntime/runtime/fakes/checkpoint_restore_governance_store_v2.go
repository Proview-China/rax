package fakes

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RestoreGovernanceStoreV2 is a deterministic in-memory reference Owner. It
// is test-only and provides no production persistence, Provider, or SLA.
type RestoreGovernanceStoreV2 struct {
	mu sync.Mutex

	attemptCurrent      map[string]ports.RestoreAttemptFactV2
	attemptHistory      map[string]map[core.Revision]ports.RestoreAttemptFactV2
	eligibilityCurrent  map[string]ports.RestoreEligibilityFactV2
	eligibilityHistory  map[string]map[core.Revision]ports.RestoreEligibilityFactV2
	reservedInstances   map[string]string
	reservedLeases      map[string]string
	activationByID      map[string]ports.RestoreActivationFactV1
	activationByAttempt map[string]ports.RestoreActivationFactV1
	activationByStable  map[string]ports.RestoreActivationFactV1
	loseNextReply       bool
	failNextWrite       bool
}

func NewRestoreGovernanceStoreV2() *RestoreGovernanceStoreV2 {
	return &RestoreGovernanceStoreV2{
		attemptCurrent: map[string]ports.RestoreAttemptFactV2{}, attemptHistory: map[string]map[core.Revision]ports.RestoreAttemptFactV2{},
		eligibilityCurrent: map[string]ports.RestoreEligibilityFactV2{}, eligibilityHistory: map[string]map[core.Revision]ports.RestoreEligibilityFactV2{},
		reservedInstances: map[string]string{}, reservedLeases: map[string]string{},
		activationByID: map[string]ports.RestoreActivationFactV1{}, activationByAttempt: map[string]ports.RestoreActivationFactV1{}, activationByStable: map[string]ports.RestoreActivationFactV1{},
	}
}

func (s *RestoreGovernanceStoreV2) LoseNextRestoreReplyV2() {
	s.mu.Lock()
	s.loseNextReply = true
	s.mu.Unlock()
}

func (s *RestoreGovernanceStoreV2) FailNextRestoreWriteV2() {
	s.mu.Lock()
	s.failNextWrite = true
	s.mu.Unlock()
}

func (s *RestoreGovernanceStoreV2) CreateRestoreAttemptV2(ctx context.Context, candidate ports.RestoreAttemptFactV2) (ports.RestoreAttemptFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	if err := candidate.Validate(); err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	key := restoreOwnerKeyV2(candidate.Ref.TenantID, candidate.Ref.ID)
	instanceKey := restoreOwnerKeyV2(candidate.Ref.TenantID, string(candidate.OperationScope.Identity.TargetInstance.ID))
	leaseKey := restoreOwnerKeyV2(candidate.Ref.TenantID, string(candidate.OperationScope.Identity.TargetLease.ID))
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.attemptCurrent[key]; ok {
		if history := s.attemptHistory[key][candidate.Ref.Revision]; history.Ref == candidate.Ref && sameRestoreAttemptFactV2(history, candidate) {
			return history.Clone(), nil
		}
		if existing.Ref == candidate.Ref && sameRestoreAttemptFactV2(existing, candidate) {
			return existing.Clone(), nil
		}
		return ports.RestoreAttemptFactV2{}, restoreStoreConflictV2("Restore Attempt ID binds different reservation")
	}
	if owner, exists := s.reservedInstances[instanceKey]; exists && owner != key {
		return ports.RestoreAttemptFactV2{}, restoreStoreConflictV2("target Instance is already reserved by another Restore Attempt")
	}
	if owner, exists := s.reservedLeases[leaseKey]; exists && owner != key {
		return ports.RestoreAttemptFactV2{}, restoreStoreConflictV2("target Sandbox Lease is already reserved by another Restore Attempt")
	}
	if s.failNextWrite {
		s.failNextWrite = false
		return ports.RestoreAttemptFactV2{}, restoreStoreUnavailableV2("injected Restore Attempt write failure")
	}
	s.attemptCurrent[key] = candidate.Clone()
	s.attemptHistory[key] = map[core.Revision]ports.RestoreAttemptFactV2{candidate.Ref.Revision: candidate.Clone()}
	s.reservedInstances[instanceKey] = key
	s.reservedLeases[leaseKey] = key
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.RestoreAttemptFactV2{}, restoreStoreUnavailableV2("injected Restore Attempt reply loss")
	}
	return candidate.Clone(), nil
}

func (s *RestoreGovernanceStoreV2) InspectRestoreAttemptCurrentV2(ctx context.Context, request ports.InspectRestoreAttemptRequestV2) (ports.RestoreAttemptFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.attemptCurrent[restoreOwnerKeyV2(request.TenantID, request.AttemptID)]
	if !ok {
		return ports.RestoreAttemptFactV2{}, restoreStoreNotFoundV2("Restore Attempt current not found")
	}
	return fact.Clone(), nil
}

func (s *RestoreGovernanceStoreV2) InspectRestoreAttemptHistoricalV2(ctx context.Context, ref ports.RestoreAttemptRefV2) (ports.RestoreAttemptFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.attemptHistory[restoreOwnerKeyV2(ref.TenantID, ref.ID)][ref.Revision]
	if !ok {
		return ports.RestoreAttemptFactV2{}, restoreStoreNotFoundV2("Restore Attempt history not found")
	}
	if fact.Ref != ref {
		return ports.RestoreAttemptFactV2{}, restoreStoreConflictV2("Restore Attempt historical ref drifted")
	}
	return fact.Clone(), nil
}

func (s *RestoreGovernanceStoreV2) BindRestoreEligibilityV2(ctx context.Context, request ports.RestoreEligibilityBindCommitRequestV2) (ports.RestoreEligibilityBindBundleV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	if request.ExpectedAttempt.Validate() != nil || request.Bundle.Validate() != nil {
		return ports.RestoreEligibilityBindBundleV2{}, restoreStoreConflictV2("Restore Eligibility bind request is invalid")
	}
	attemptKey := restoreOwnerKeyV2(request.ExpectedAttempt.TenantID, request.ExpectedAttempt.ID)
	eligibilityKey := restoreOwnerKeyV2(request.Bundle.Eligibility.Ref.TenantID, request.Bundle.Eligibility.Ref.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.attemptCurrent[attemptKey]
	if !ok {
		return ports.RestoreEligibilityBindBundleV2{}, restoreStoreNotFoundV2("Restore Attempt current not found for Eligibility")
	}
	if existing, ok := s.eligibilityCurrent[eligibilityKey]; ok {
		historicalAttempt, found := s.attemptHistory[attemptKey][request.Bundle.Attempt.Ref.Revision]
		if found && existing.Ref == request.Bundle.Eligibility.Ref && sameRestoreEligibilityFactV2(existing, request.Bundle.Eligibility) && historicalAttempt.Ref == request.Bundle.Attempt.Ref && sameRestoreAttemptFactV2(historicalAttempt, request.Bundle.Attempt) {
			return cloneOSE(request.Bundle), nil
		}
		return ports.RestoreEligibilityBindBundleV2{}, restoreStoreConflictV2("Restore Eligibility ID binds different content")
	}
	if current.Ref != request.ExpectedAttempt || control.ValidateRestoreAttemptTransitionV2(current, request.Bundle.Attempt) != nil {
		return ports.RestoreEligibilityBindBundleV2{}, restoreStoreConflictV2("Restore Eligibility Attempt CAS drifted")
	}
	if s.failNextWrite {
		s.failNextWrite = false
		return ports.RestoreEligibilityBindBundleV2{}, restoreStoreUnavailableV2("injected Restore Eligibility bind failure")
	}
	s.attemptCurrent[attemptKey] = request.Bundle.Attempt.Clone()
	s.attemptHistory[attemptKey][request.Bundle.Attempt.Ref.Revision] = request.Bundle.Attempt.Clone()
	s.eligibilityCurrent[eligibilityKey] = request.Bundle.Eligibility.Clone()
	s.eligibilityHistory[eligibilityKey] = map[core.Revision]ports.RestoreEligibilityFactV2{request.Bundle.Eligibility.Ref.Revision: request.Bundle.Eligibility.Clone()}
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.RestoreEligibilityBindBundleV2{}, restoreStoreUnavailableV2("injected Restore Eligibility bind reply loss")
	}
	return cloneOSE(request.Bundle), nil
}

func (s *RestoreGovernanceStoreV2) InspectRestoreEligibilityHistoricalV2(ctx context.Context, ref ports.RestoreEligibilityRefV2) (ports.RestoreEligibilityFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.eligibilityHistory[restoreOwnerKeyV2(ref.TenantID, ref.ID)][ref.Revision]
	if !ok {
		return ports.RestoreEligibilityFactV2{}, restoreStoreNotFoundV2("Restore Eligibility history not found")
	}
	if fact.Ref != ref {
		return ports.RestoreEligibilityFactV2{}, restoreStoreConflictV2("Restore Eligibility historical ref drifted")
	}
	return fact.Clone(), nil
}

func (s *RestoreGovernanceStoreV2) InspectRestoreEligibilityCurrentV2(ctx context.Context, request ports.InspectRestoreEligibilityCurrentRequestV2) (ports.RestoreEligibilityFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.eligibilityCurrent[restoreOwnerKeyV2(request.ExpectedEligibility.TenantID, request.ExpectedEligibility.ID)]
	if !ok {
		return ports.RestoreEligibilityFactV2{}, restoreStoreNotFoundV2("Restore Eligibility current not found")
	}
	if fact.Ref != request.ExpectedEligibility || fact.Attempt.ID != request.Attempt.ID || fact.Attempt.TenantID != request.Attempt.TenantID || fact.Attempt.Revision > request.Attempt.Revision {
		return ports.RestoreEligibilityFactV2{}, restoreStoreConflictV2("Restore Eligibility is no longer exact current for Attempt")
	}
	return fact.Clone(), nil
}

func (s *RestoreGovernanceStoreV2) CompareAndSwapRestoreEligibilityV2(ctx context.Context, request ports.RestoreEligibilityCASRequestV2) (ports.RestoreEligibilityFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	if request.Expected.Validate() != nil || request.Next.Validate() != nil {
		return ports.RestoreEligibilityFactV2{}, restoreStoreConflictV2("Restore Eligibility CAS request is invalid")
	}
	key := restoreOwnerKeyV2(request.Expected.TenantID, request.Expected.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.eligibilityCurrent[key]
	if !ok {
		return ports.RestoreEligibilityFactV2{}, restoreStoreNotFoundV2("Restore Eligibility current not found for CAS")
	}
	if current.Ref != request.Expected {
		return ports.RestoreEligibilityFactV2{}, restoreStoreConflictV2("Restore Eligibility expected current drifted")
	}
	if err := control.ValidateRestoreEligibilityTransitionV2(current, request.Next, time.Unix(0, request.Next.UpdatedUnixNano)); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	if s.failNextWrite {
		s.failNextWrite = false
		return ports.RestoreEligibilityFactV2{}, restoreStoreUnavailableV2("injected Restore Eligibility CAS failure")
	}
	s.eligibilityCurrent[key] = request.Next.Clone()
	s.eligibilityHistory[key][request.Next.Ref.Revision] = request.Next.Clone()
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.RestoreEligibilityFactV2{}, restoreStoreUnavailableV2("injected Restore Eligibility CAS reply loss")
	}
	return request.Next.Clone(), nil
}

func (s *RestoreGovernanceStoreV2) CommitRestoreActivationV1(ctx context.Context, request ports.RestoreActivationCommitRequestV1) (ports.RestoreActivationFactV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.RestoreActivationFactV1{}, err
	}
	if request.ExpectedAttempt.Validate() != nil || request.NextAttempt.Validate() != nil || request.Activation.Validate() != nil {
		return ports.RestoreActivationFactV1{}, restoreStoreConflictV2("Restore Activation commit is invalid")
	}
	attemptKey := restoreOwnerKeyV2(request.ExpectedAttempt.TenantID, request.ExpectedAttempt.ID)
	activationKey := restoreOwnerKeyV2(request.Activation.Ref.Attempt.TenantID, request.Activation.Ref.ID)
	activatedAttemptKey := restoreActivationAttemptKeyV1(request.Activation.Ref.Attempt)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.activationByAttempt[activatedAttemptKey]; ok {
		if existing.Ref == request.Activation.Ref && sameRestoreActivationFactV1(existing, request.Activation) {
			return cloneRestoreActivationFactV1(existing), nil
		}
		return ports.RestoreActivationFactV1{}, restoreStoreConflictV2("Restore Attempt already binds another Activation")
	}
	if existing, ok := s.activationByID[activationKey]; ok {
		if existing.Ref == request.Activation.Ref && sameRestoreActivationFactV1(existing, request.Activation) {
			return cloneRestoreActivationFactV1(existing), nil
		}
		return ports.RestoreActivationFactV1{}, restoreStoreConflictV2("Restore Activation ID binds different content")
	}
	current, ok := s.attemptCurrent[attemptKey]
	if !ok || current.Ref != request.ExpectedAttempt || !validRestoreActivationAttemptTransitionV1(current, request.NextAttempt) || request.Activation.Submission.Attempt != current.Ref || request.Activation.Ref.Attempt != request.NextAttempt.Ref {
		return ports.RestoreActivationFactV1{}, restoreStoreConflictV2("Restore Activation Attempt CAS drifted")
	}
	if s.failNextWrite {
		s.failNextWrite = false
		return ports.RestoreActivationFactV1{}, restoreStoreUnavailableV2("injected Restore Activation commit failure")
	}
	s.attemptCurrent[attemptKey] = request.NextAttempt.Clone()
	s.attemptHistory[attemptKey][request.NextAttempt.Ref.Revision] = request.NextAttempt.Clone()
	s.activationByID[activationKey] = cloneRestoreActivationFactV1(request.Activation)
	s.activationByAttempt[activatedAttemptKey] = cloneRestoreActivationFactV1(request.Activation)
	s.activationByStable[restoreOwnerKeyV2(request.Activation.Ref.Attempt.TenantID, request.Activation.Ref.Attempt.ID)] = cloneRestoreActivationFactV1(request.Activation)
	if s.loseNextReply {
		s.loseNextReply = false
		return ports.RestoreActivationFactV1{}, restoreStoreUnavailableV2("injected Restore Activation reply loss")
	}
	return cloneRestoreActivationFactV1(request.Activation), nil
}

func (s *RestoreGovernanceStoreV2) InspectRestoreActivationV1(ctx context.Context, ref ports.RestoreActivationRefV1) (ports.RestoreActivationFactV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.RestoreActivationFactV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.RestoreActivationFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.activationByID[restoreOwnerKeyV2(ref.Attempt.TenantID, ref.ID)]
	if !ok {
		return ports.RestoreActivationFactV1{}, restoreStoreNotFoundV2("Restore Activation not found")
	}
	if fact.Ref != ref {
		return ports.RestoreActivationFactV1{}, restoreStoreConflictV2("Restore Activation exact ref drifted")
	}
	return cloneRestoreActivationFactV1(fact), nil
}

func (s *RestoreGovernanceStoreV2) InspectRestoreActivationByAttemptV1(ctx context.Context, attempt ports.RestoreAttemptRefV2) (ports.RestoreActivationFactV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.RestoreActivationFactV1{}, err
	}
	if err := attempt.Validate(); err != nil {
		return ports.RestoreActivationFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.activationByAttempt[restoreActivationAttemptKeyV1(attempt)]
	if !ok {
		return ports.RestoreActivationFactV1{}, restoreStoreNotFoundV2("Restore Activation by Attempt not found")
	}
	return cloneRestoreActivationFactV1(fact), nil
}

func (s *RestoreGovernanceStoreV2) InspectRestoreActivationByStableAttemptV1(ctx context.Context, tenantID core.TenantID, attemptID string) (ports.RestoreActivationFactV1, error) {
	if err := contextError(ctx); err != nil {
		return ports.RestoreActivationFactV1{}, err
	}
	if tenantID == "" || attemptID == "" {
		return ports.RestoreActivationFactV1{}, restoreStoreConflictV2("Restore Activation stable Attempt coordinate is incomplete")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.activationByStable[restoreOwnerKeyV2(tenantID, attemptID)]
	if !ok {
		return ports.RestoreActivationFactV1{}, restoreStoreNotFoundV2("Restore Activation by stable Attempt not found")
	}
	return cloneRestoreActivationFactV1(fact), nil
}

func validRestoreActivationAttemptTransitionV1(current, next ports.RestoreAttemptFactV2) bool {
	return current.State == ports.RestoreAttemptEligibilityBoundV2 && next.State == ports.RestoreAttemptActivatedV2 &&
		next.Ref.TenantID == current.Ref.TenantID && next.Ref.ID == current.Ref.ID && next.Ref.Revision == current.Ref.Revision+1 &&
		next.OperationScope == current.OperationScope && next.IdempotencyKey == current.IdempotencyKey && next.RequestedNotAfter == current.RequestedNotAfter && next.CreatedUnixNano == current.CreatedUnixNano && next.UpdatedUnixNano >= current.UpdatedUnixNano &&
		current.Eligibility != nil && next.Eligibility != nil && *current.Eligibility == *next.Eligibility
}

func restoreActivationAttemptKeyV1(ref ports.RestoreAttemptRefV2) string {
	return restoreOwnerKeyV2(ref.TenantID, ref.ID) + "\x00" + strconv.FormatUint(uint64(ref.Revision), 10) + "\x00" + string(ref.Digest)
}

func cloneRestoreActivationFactV1(value ports.RestoreActivationFactV1) ports.RestoreActivationFactV1 {
	value.Submission.Context = value.Submission.Context.Clone()
	return value
}

func sameRestoreActivationFactV1(left, right ports.RestoreActivationFactV1) bool {
	ld, le := core.CanonicalJSONDigest("praxis.runtime.restore-activation-fake", ports.RestoreActivationContractVersionV1, "RestoreActivationFactV1", left)
	rd, re := core.CanonicalJSONDigest("praxis.runtime.restore-activation-fake", ports.RestoreActivationContractVersionV1, "RestoreActivationFactV1", right)
	return le == nil && re == nil && ld == rd
}

func restoreOwnerKeyV2(tenant core.TenantID, id string) string { return string(tenant) + "\x00" + id }

func sameRestoreAttemptFactV2(a, b ports.RestoreAttemptFactV2) bool {
	return a.Ref == b.Ref && a.OperationScope == b.OperationScope && a.State == b.State && a.IdempotencyKey == b.IdempotencyKey && a.RequestedNotAfter == b.RequestedNotAfter && a.CreatedUnixNano == b.CreatedUnixNano && a.UpdatedUnixNano == b.UpdatedUnixNano && ((a.Eligibility == nil && b.Eligibility == nil) || (a.Eligibility != nil && b.Eligibility != nil && *a.Eligibility == *b.Eligibility))
}

func sameRestoreEligibilityFactV2(a, b ports.RestoreEligibilityFactV2) bool {
	return a.Ref == b.Ref && a.State == b.State && a.Attempt == b.Attempt && a.OperationScopeDigest == b.OperationScopeDigest && a.RestorePlan == b.RestorePlan && a.CheckpointConsistency == b.CheckpointConsistency && a.Identity == b.Identity && a.ReviewTarget == b.ReviewTarget && a.InputsProjectionDigest == b.InputsProjectionDigest && a.CreatedUnixNano == b.CreatedUnixNano && a.UpdatedUnixNano == b.UpdatedUnixNano && a.InvalidationReason == b.InvalidationReason && restoreRefSlicesEqualV2(a.ReviewRequirementRefs, b.ReviewRequirementRefs) && restoreRefSlicesEqualV2(a.PolicyBasisRefs, b.PolicyBasisRefs) && restoreRefSlicesEqualV2(a.AuthorityRequirementRefs, b.AuthorityRequirementRefs) && restoreRefSlicesEqualV2(a.ScopeRequirementRefs, b.ScopeRequirementRefs) && restoreRefSlicesEqualV2(a.BudgetRequirementRefs, b.BudgetRequirementRefs) && restoreRefSlicesEqualV2(a.BindingRequirementRefs, b.BindingRequirementRefs) && restoreRefSlicesEqualV2(a.ContextRequirementRefs, b.ContextRequirementRefs)
}

func restoreRefSlicesEqualV2(a, b []ports.CheckpointExternalExactFactRefV2) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

func restoreStoreConflictV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, message)
}
func restoreStoreNotFoundV2(message string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonRestoreIncompatible, message)
}
func restoreStoreUnavailableV2(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonRestoreIncompatible, message)
}

var _ ports.RestoreGovernanceFactPortV2 = (*RestoreGovernanceStoreV2)(nil)
var _ ports.RestoreActivationFactPortV1 = (*RestoreGovernanceStoreV2)(nil)
