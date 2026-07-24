package fakes

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ReviewDecisionGovernanceCurrentStoreV1 is a deterministic reference journal.
// It is a test/conformance fake, not production persistence or an SLA claim.
type ReviewDecisionGovernanceCurrentStoreV1 struct {
	mu               sync.RWMutex
	policyHistory    map[string]ports.ReviewDecisionPolicyCurrentProjectionV1
	policyCurrent    map[string]ports.ReviewDecisionPolicyCurrentProjectionRefV1
	authorityHistory map[string]ports.ReviewDecisionAuthorityCurrentProjectionV1
	authorityCurrent map[string]ports.ReviewDecisionAuthorityCurrentProjectionRefV1
	scopeHistory     map[string]ports.ReviewDecisionScopeCurrentProjectionV1
	scopeCurrent     map[string]ports.ReviewDecisionScopeCurrentProjectionRefV1
	afterCommit      func(string) error
}

func NewReviewDecisionGovernanceCurrentStoreV1() *ReviewDecisionGovernanceCurrentStoreV1 {
	return &ReviewDecisionGovernanceCurrentStoreV1{
		policyHistory: make(map[string]ports.ReviewDecisionPolicyCurrentProjectionV1), policyCurrent: make(map[string]ports.ReviewDecisionPolicyCurrentProjectionRefV1),
		authorityHistory: make(map[string]ports.ReviewDecisionAuthorityCurrentProjectionV1), authorityCurrent: make(map[string]ports.ReviewDecisionAuthorityCurrentProjectionRefV1),
		scopeHistory: make(map[string]ports.ReviewDecisionScopeCurrentProjectionV1), scopeCurrent: make(map[string]ports.ReviewDecisionScopeCurrentProjectionRefV1),
	}
}

func (s *ReviewDecisionGovernanceCurrentStoreV1) SetAfterCommitHookV1(hook func(string) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.afterCommit = hook
}

func (s *ReviewDecisionGovernanceCurrentStoreV1) ResolvePolicyV1(ctx context.Context, subject ports.ReviewDecisionPolicyCurrentSubjectV1) (ports.ReviewDecisionPolicyCurrentProjectionRefV1, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV1{}, err
	}
	id, err := ports.DeriveReviewDecisionPolicyCurrentProjectionIDV1(subject, subject.Policy.Ref)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ref, ok := s.policyCurrent[id]
	if !ok {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV1{}, fakeGovernanceNotFoundV1("Policy current projection")
	}
	return ref, nil
}
func (s *ReviewDecisionGovernanceCurrentStoreV1) InspectCurrentPolicyV1(ctx context.Context, subject ports.ReviewDecisionPolicyCurrentSubjectV1, expected ports.ReviewDecisionPolicyCurrentProjectionRefV1) (ports.ReviewDecisionPolicyCurrentProjectionV1, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	id, err := ports.DeriveReviewDecisionPolicyCurrentProjectionIDV1(subject, subject.Policy.Ref)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	current, ok := s.policyCurrent[id]
	if !ok {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, fakeGovernanceNotFoundV1("Policy current projection")
	}
	if current != expected {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, fakeGovernanceConflictV1("Policy current index drifted")
	}
	value, ok := s.policyHistory[fakeGovernanceRefKeyV1(expected.ID, expected.Revision, expected.Digest)]
	if !ok {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, fakeGovernanceConflictV1("Policy current history is incomplete")
	}
	return clonePolicyProjectionV1(value), nil
}
func (s *ReviewDecisionGovernanceCurrentStoreV1) InspectHistoricalPolicyV1(ctx context.Context, ref ports.ReviewDecisionPolicyCurrentProjectionRefV1) (ports.ReviewDecisionPolicyCurrentProjectionV1, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.policyHistory[fakeGovernanceRefKeyV1(ref.ID, ref.Revision, ref.Digest)]
	if !ok {
		return ports.ReviewDecisionPolicyCurrentProjectionV1{}, fakeGovernanceNotFoundV1("Policy historical projection")
	}
	return clonePolicyProjectionV1(value), nil
}
func (s *ReviewDecisionGovernanceCurrentStoreV1) CommitPolicyV1(ctx context.Context, request ports.ReviewDecisionPolicyCurrentPublishRequestV1) (ports.ReviewDecisionPolicyCurrentPublishReceiptV1, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
	}
	s.mu.Lock()
	key := fakeGovernanceRefKeyV1(request.Value.Ref.ID, request.Value.Ref.Revision, request.Value.Ref.Digest)
	if existing, ok := s.policyHistory[key]; ok {
		s.mu.Unlock()
		if reflect.DeepEqual(existing, request.Value) {
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{Ref: request.Value.Ref, Created: false}, nil
		}
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, fakeGovernanceConflictV1("Policy same Ref changed content")
	}
	if err := validatePolicyCASV1(s.policyCurrent, request); err != nil {
		s.mu.Unlock()
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
	}
	s.policyHistory[key] = clonePolicyProjectionV1(request.Value)
	s.policyCurrent[request.Value.Ref.ID] = request.Value.Ref
	hook := s.afterCommit
	s.mu.Unlock()
	if hook != nil {
		if err := hook("policy"); err != nil {
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{}, err
		}
	}
	return ports.ReviewDecisionPolicyCurrentPublishReceiptV1{Ref: request.Value.Ref, Created: true}, nil
}

func (s *ReviewDecisionGovernanceCurrentStoreV1) ResolveAuthorityV1(ctx context.Context, subject ports.ReviewDecisionAuthorityCurrentSubjectV1) (ports.ReviewDecisionAuthorityCurrentProjectionRefV1, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionRefV1{}, err
	}
	id, err := ports.DeriveReviewDecisionAuthorityCurrentProjectionIDV1(subject, subject.Authority.Ref)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionRefV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ref, ok := s.authorityCurrent[id]
	if !ok {
		return ports.ReviewDecisionAuthorityCurrentProjectionRefV1{}, fakeGovernanceNotFoundV1("Authority current projection")
	}
	return ref, nil
}
func (s *ReviewDecisionGovernanceCurrentStoreV1) InspectCurrentAuthorityV1(ctx context.Context, subject ports.ReviewDecisionAuthorityCurrentSubjectV1, expected ports.ReviewDecisionAuthorityCurrentProjectionRefV1) (ports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	id, err := ports.DeriveReviewDecisionAuthorityCurrentProjectionIDV1(subject, subject.Authority.Ref)
	if err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	current, ok := s.authorityCurrent[id]
	if !ok {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, fakeGovernanceNotFoundV1("Authority current projection")
	}
	if current != expected {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, fakeGovernanceConflictV1("Authority current index drifted")
	}
	value, ok := s.authorityHistory[fakeGovernanceRefKeyV1(expected.ID, expected.Revision, expected.Digest)]
	if !ok {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, fakeGovernanceConflictV1("Authority current history is incomplete")
	}
	return cloneAuthorityProjectionV1(value), nil
}
func (s *ReviewDecisionGovernanceCurrentStoreV1) InspectHistoricalAuthorityV1(ctx context.Context, ref ports.ReviewDecisionAuthorityCurrentProjectionRefV1) (ports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.authorityHistory[fakeGovernanceRefKeyV1(ref.ID, ref.Revision, ref.Digest)]
	if !ok {
		return ports.ReviewDecisionAuthorityCurrentProjectionV1{}, fakeGovernanceNotFoundV1("Authority historical projection")
	}
	return cloneAuthorityProjectionV1(value), nil
}
func (s *ReviewDecisionGovernanceCurrentStoreV1) CommitAuthorityV1(ctx context.Context, request ports.ReviewDecisionAuthorityCurrentPublishRequestV1) (ports.ReviewDecisionAuthorityCurrentPublishReceiptV1, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
	}
	s.mu.Lock()
	key := fakeGovernanceRefKeyV1(request.Value.Ref.ID, request.Value.Ref.Revision, request.Value.Ref.Digest)
	if existing, ok := s.authorityHistory[key]; ok {
		s.mu.Unlock()
		if reflect.DeepEqual(existing, request.Value) {
			return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{Ref: request.Value.Ref, Created: false}, nil
		}
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, fakeGovernanceConflictV1("Authority same Ref changed content")
	}
	if err := validateAuthorityCASV1(s.authorityCurrent, request); err != nil {
		s.mu.Unlock()
		return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
	}
	s.authorityHistory[key] = cloneAuthorityProjectionV1(request.Value)
	s.authorityCurrent[request.Value.Ref.ID] = request.Value.Ref
	hook := s.afterCommit
	s.mu.Unlock()
	if hook != nil {
		if err := hook("authority"); err != nil {
			return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{}, err
		}
	}
	return ports.ReviewDecisionAuthorityCurrentPublishReceiptV1{Ref: request.Value.Ref, Created: true}, nil
}

func (s *ReviewDecisionGovernanceCurrentStoreV1) ResolveScopeV1(ctx context.Context, subject ports.ReviewDecisionScopeCurrentSubjectV1) (ports.ReviewDecisionScopeCurrentProjectionRefV1, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionRefV1{}, err
	}
	id, err := ports.DeriveReviewDecisionScopeCurrentProjectionIDV1(subject, subject.CurrentScope.Ref)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionRefV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ref, ok := s.scopeCurrent[id]
	if !ok {
		return ports.ReviewDecisionScopeCurrentProjectionRefV1{}, fakeGovernanceNotFoundV1("Scope current projection")
	}
	return ref, nil
}
func (s *ReviewDecisionGovernanceCurrentStoreV1) InspectCurrentScopeV1(ctx context.Context, subject ports.ReviewDecisionScopeCurrentSubjectV1, expected ports.ReviewDecisionScopeCurrentProjectionRefV1) (ports.ReviewDecisionScopeCurrentProjectionV1, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	id, err := ports.DeriveReviewDecisionScopeCurrentProjectionIDV1(subject, subject.CurrentScope.Ref)
	if err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	current, ok := s.scopeCurrent[id]
	if !ok {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, fakeGovernanceNotFoundV1("Scope current projection")
	}
	if current != expected {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, fakeGovernanceConflictV1("Scope current index drifted")
	}
	value, ok := s.scopeHistory[fakeGovernanceRefKeyV1(expected.ID, expected.Revision, expected.Digest)]
	if !ok {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, fakeGovernanceConflictV1("Scope current history is incomplete")
	}
	return cloneScopeProjectionV1(value), nil
}
func (s *ReviewDecisionGovernanceCurrentStoreV1) InspectHistoricalScopeV1(ctx context.Context, ref ports.ReviewDecisionScopeCurrentProjectionRefV1) (ports.ReviewDecisionScopeCurrentProjectionV1, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.scopeHistory[fakeGovernanceRefKeyV1(ref.ID, ref.Revision, ref.Digest)]
	if !ok {
		return ports.ReviewDecisionScopeCurrentProjectionV1{}, fakeGovernanceNotFoundV1("Scope historical projection")
	}
	return cloneScopeProjectionV1(value), nil
}
func (s *ReviewDecisionGovernanceCurrentStoreV1) CommitScopeV1(ctx context.Context, request ports.ReviewDecisionScopeCurrentPublishRequestV1) (ports.ReviewDecisionScopeCurrentPublishReceiptV1, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
	}
	s.mu.Lock()
	key := fakeGovernanceRefKeyV1(request.Value.Ref.ID, request.Value.Ref.Revision, request.Value.Ref.Digest)
	if existing, ok := s.scopeHistory[key]; ok {
		s.mu.Unlock()
		if reflect.DeepEqual(existing, request.Value) {
			return ports.ReviewDecisionScopeCurrentPublishReceiptV1{Ref: request.Value.Ref, Created: false}, nil
		}
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, fakeGovernanceConflictV1("Scope same Ref changed content")
	}
	if err := validateScopeCASV1(s.scopeCurrent, request); err != nil {
		s.mu.Unlock()
		return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
	}
	s.scopeHistory[key] = cloneScopeProjectionV1(request.Value)
	s.scopeCurrent[request.Value.Ref.ID] = request.Value.Ref
	hook := s.afterCommit
	s.mu.Unlock()
	if hook != nil {
		if err := hook("scope"); err != nil {
			return ports.ReviewDecisionScopeCurrentPublishReceiptV1{}, err
		}
	}
	return ports.ReviewDecisionScopeCurrentPublishReceiptV1{Ref: request.Value.Ref, Created: true}, nil
}

// ReviewDecisionGovernanceSourceStoreV1 is a mutable test source/proof reader.
// Mutations let fault tests prove S1/S2 drift detection; it is not production.
type ReviewDecisionGovernanceSourceStoreV1 struct {
	mu          sync.RWMutex
	targets     map[string]ports.ReviewDecisionTargetRefV1
	assignments map[string]ports.ReviewDecisionAssignmentRefV1
	policies    map[string]ports.ReviewPolicyFactV2
	authorities map[string]ports.DispatchAuthorityFactV2
	scopes      map[string]ports.ExecutionScopeCurrentFactV2
}

func NewReviewDecisionGovernanceSourceStoreV1() *ReviewDecisionGovernanceSourceStoreV1 {
	return &ReviewDecisionGovernanceSourceStoreV1{targets: map[string]ports.ReviewDecisionTargetRefV1{}, assignments: map[string]ports.ReviewDecisionAssignmentRefV1{}, policies: map[string]ports.ReviewPolicyFactV2{}, authorities: map[string]ports.DispatchAuthorityFactV2{}, scopes: map[string]ports.ExecutionScopeCurrentFactV2{}}
}
func (s *ReviewDecisionGovernanceSourceStoreV1) PutTargetV1(v ports.ReviewDecisionTargetRefV1) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targets[v.ID] = v
}
func (s *ReviewDecisionGovernanceSourceStoreV1) PutAssignmentV1(v ports.ReviewDecisionAssignmentRefV1) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assignments[string(v.TenantID)+"\x00"+v.ID] = v
}
func (s *ReviewDecisionGovernanceSourceStoreV1) PutPolicyV1(v ports.ReviewPolicyFactV2) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policies[v.Ref] = clonePolicyFactV1(v)
}
func (s *ReviewDecisionGovernanceSourceStoreV1) PutAuthorityV1(v ports.DispatchAuthorityFactV2) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authorities[v.Ref] = cloneAuthorityFactV1(v)
}
func (s *ReviewDecisionGovernanceSourceStoreV1) PutScopeV1(v ports.ExecutionScopeCurrentFactV2) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scopes[v.Ref] = cloneScopeFactV1(v)
}
func (s *ReviewDecisionGovernanceSourceStoreV1) InspectReviewDecisionTargetProofV1(ctx context.Context, expected ports.ReviewDecisionTargetRefV1) (ports.ReviewDecisionTargetRefV1, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionTargetRefV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.targets[expected.ID]
	if !ok {
		return ports.ReviewDecisionTargetRefV1{}, fakeGovernanceNotFoundV1("Review Target proof")
	}
	return v, nil
}
func (s *ReviewDecisionGovernanceSourceStoreV1) InspectReviewDecisionAssignmentProofV1(ctx context.Context, expected ports.ReviewDecisionAssignmentRefV1) (ports.ReviewDecisionAssignmentRefV1, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionAssignmentRefV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.assignments[string(expected.TenantID)+"\x00"+expected.ID]
	if !ok {
		return ports.ReviewDecisionAssignmentRefV1{}, fakeGovernanceNotFoundV1("Review Assignment proof")
	}
	return v, nil
}
func (s *ReviewDecisionGovernanceSourceStoreV1) InspectReviewPolicy(ctx context.Context, ref string) (ports.ReviewPolicyFactV2, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewPolicyFactV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.policies[ref]
	if !ok {
		return ports.ReviewPolicyFactV2{}, fakeGovernanceNotFoundV1("Review Policy fact")
	}
	return clonePolicyFactV1(v), nil
}
func (s *ReviewDecisionGovernanceSourceStoreV1) InspectDispatchAuthority(ctx context.Context, ref string) (ports.DispatchAuthorityFactV2, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.DispatchAuthorityFactV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.authorities[ref]
	if !ok {
		return ports.DispatchAuthorityFactV2{}, fakeGovernanceNotFoundV1("Authority fact")
	}
	return cloneAuthorityFactV1(v), nil
}
func (s *ReviewDecisionGovernanceSourceStoreV1) InspectCurrentExecutionScope(ctx context.Context, ref string) (ports.ExecutionScopeCurrentFactV2, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ExecutionScopeCurrentFactV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.scopes[ref]
	if !ok {
		return ports.ExecutionScopeCurrentFactV2{}, fakeGovernanceNotFoundV1("Scope fact")
	}
	return cloneScopeFactV1(v), nil
}

func validatePolicyCASV1(index map[string]ports.ReviewDecisionPolicyCurrentProjectionRefV1, r ports.ReviewDecisionPolicyCurrentPublishRequestV1) error {
	current, exists := index[r.Value.Ref.ID]
	if r.Previous == nil {
		if exists {
			return fakeGovernanceConflictV1("Policy current already exists")
		}
		return nil
	}
	if !exists || current != *r.Previous {
		return fakeGovernanceConflictV1("Policy current CAS drifted")
	}
	return nil
}
func validateAuthorityCASV1(index map[string]ports.ReviewDecisionAuthorityCurrentProjectionRefV1, r ports.ReviewDecisionAuthorityCurrentPublishRequestV1) error {
	current, exists := index[r.Value.Ref.ID]
	if r.Previous == nil {
		if exists {
			return fakeGovernanceConflictV1("Authority current already exists")
		}
		return nil
	}
	if !exists || current != *r.Previous {
		return fakeGovernanceConflictV1("Authority current CAS drifted")
	}
	return nil
}
func validateScopeCASV1(index map[string]ports.ReviewDecisionScopeCurrentProjectionRefV1, r ports.ReviewDecisionScopeCurrentPublishRequestV1) error {
	current, exists := index[r.Value.Ref.ID]
	if r.Previous == nil {
		if exists {
			return fakeGovernanceConflictV1("Scope current already exists")
		}
		return nil
	}
	if !exists || current != *r.Previous {
		return fakeGovernanceConflictV1("Scope current CAS drifted")
	}
	return nil
}

func fakeGovernanceRefKeyV1(id string, revision core.Revision, digest core.Digest) string {
	return fmt.Sprintf("%s\x00%d\x00%s", id, revision, digest)
}
func fakeGovernanceNotFoundV1(what string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, what+" not found")
}
func fakeGovernanceConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, message)
}
func fakeGovernanceContextV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "context is required")
	}
	if ctx.Err() != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "context ended before exact governance inspection")
	}
	return nil
}
func clonePolicyProjectionV1(v ports.ReviewDecisionPolicyCurrentProjectionV1) ports.ReviewDecisionPolicyCurrentProjectionV1 {
	v.Fact = clonePolicyFactV1(v.Fact)
	return v
}
func cloneAuthorityProjectionV1(v ports.ReviewDecisionAuthorityCurrentProjectionV1) ports.ReviewDecisionAuthorityCurrentProjectionV1 {
	v.Fact = cloneAuthorityFactV1(v.Fact)
	return v
}
func cloneScopeProjectionV1(v ports.ReviewDecisionScopeCurrentProjectionV1) ports.ReviewDecisionScopeCurrentProjectionV1 {
	v.Subject.Scope = cloneExecutionScopeV1(v.Subject.Scope)
	v.Fact = cloneScopeFactV1(v.Fact)
	return v
}
func clonePolicyFactV1(v ports.ReviewPolicyFactV2) ports.ReviewPolicyFactV2 {
	v.Scope = cloneExecutionScopeV1(v.Scope)
	return v
}
func cloneAuthorityFactV1(v ports.DispatchAuthorityFactV2) ports.DispatchAuthorityFactV2 {
	v.Scope = cloneExecutionScopeV1(v.Scope)
	return v
}
func cloneScopeFactV1(v ports.ExecutionScopeCurrentFactV2) ports.ExecutionScopeCurrentFactV2 {
	v.Scope = cloneExecutionScopeV1(v.Scope)
	if v.SandboxSource != nil {
		copy := *v.SandboxSource
		v.SandboxSource = &copy
	}
	return v
}
func cloneExecutionScopeV1(v core.ExecutionScope) core.ExecutionScope {
	if v.SandboxLease != nil {
		copy := *v.SandboxLease
		v.SandboxLease = &copy
	}
	return v
}

var _ control.ReviewDecisionGovernanceCurrentFactPortV1 = (*ReviewDecisionGovernanceCurrentStoreV1)(nil)
var _ control.ReviewDecisionSubjectProofReaderV1 = (*ReviewDecisionGovernanceSourceStoreV1)(nil)
var _ ports.ReviewPolicyFactReaderV2 = (*ReviewDecisionGovernanceSourceStoreV1)(nil)
var _ ports.AuthorityFactReaderV2 = (*ReviewDecisionGovernanceSourceStoreV1)(nil)
var _ ports.ExecutionScopeFactReaderV2 = (*ReviewDecisionGovernanceSourceStoreV1)(nil)
