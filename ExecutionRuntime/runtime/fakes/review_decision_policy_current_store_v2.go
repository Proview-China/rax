package fakes

import (
	"context"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ReviewDecisionPolicyCurrentStoreV2 is a deterministic test/conformance
// journal. It is not production persistence or a production root.
type ReviewDecisionPolicyCurrentStoreV2 struct {
	mu          sync.RWMutex
	history     map[string]ports.ReviewDecisionPolicyCurrentProjectionV2
	current     map[string]ports.ReviewDecisionPolicyCurrentProjectionRefV2
	highest     map[string]core.Revision
	afterCommit func() error
}

func NewReviewDecisionPolicyCurrentStoreV2() *ReviewDecisionPolicyCurrentStoreV2 {
	return &ReviewDecisionPolicyCurrentStoreV2{history: map[string]ports.ReviewDecisionPolicyCurrentProjectionV2{}, current: map[string]ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, highest: map[string]core.Revision{}}
}
func (s *ReviewDecisionPolicyCurrentStoreV2) SetAfterCommitHookV2(hook func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.afterCommit = hook
}

func (s *ReviewDecisionPolicyCurrentStoreV2) ResolvePolicyV2(ctx context.Context, subject ports.ReviewDecisionPolicyApplicabilitySubjectV2) (ports.ReviewDecisionPolicyCurrentProjectionRefV2, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, err
	}
	id, err := ports.DeriveReviewDecisionPolicyCurrentProjectionIDV2(subject)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ref, ok := s.current[id]
	if !ok {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, fakeGovernanceNotFoundV1("Review decision policy V2 current projection")
	}
	if s.highest[id] != ref.Revision {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, fakeGovernanceConflictV1("Review decision policy V2 current/highest drifted")
	}
	value, ok := s.history[fakeGovernanceRefKeyV1(ref.ID, ref.Revision, ref.Digest)]
	if !ok || !ports.SameReviewDecisionPolicyApplicabilitySubjectV2(value.Subject, subject) {
		return ports.ReviewDecisionPolicyCurrentProjectionRefV2{}, fakeGovernanceConflictV1("Review decision policy V2 current subject drifted")
	}
	return ref, nil
}

func (s *ReviewDecisionPolicyCurrentStoreV2) InspectCurrentPolicyV2(ctx context.Context, subject ports.ReviewDecisionPolicyApplicabilitySubjectV2, expected ports.ReviewDecisionPolicyCurrentProjectionRefV2) (ports.ReviewDecisionPolicyCurrentProjectionV2, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	id, err := ports.DeriveReviewDecisionPolicyCurrentProjectionIDV2(subject)
	if err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	current, ok := s.current[id]
	if !ok {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, fakeGovernanceNotFoundV1("Review decision policy V2 current projection")
	}
	if current != expected || s.highest[id] != expected.Revision {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, fakeGovernanceConflictV1("Review decision policy V2 current full Ref drifted")
	}
	value, ok := s.history[fakeGovernanceRefKeyV1(expected.ID, expected.Revision, expected.Digest)]
	if !ok {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, fakeGovernanceConflictV1("Review decision policy V2 history is incomplete")
	}
	if !ports.SameReviewDecisionPolicyApplicabilitySubjectV2(value.Subject, subject) {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, fakeGovernanceConflictV1("Review decision policy V2 subject drifted")
	}
	return value.Clone(), nil
}

func (s *ReviewDecisionPolicyCurrentStoreV2) InspectHistoricalPolicyV2(ctx context.Context, ref ports.ReviewDecisionPolicyCurrentProjectionRefV2) (ports.ReviewDecisionPolicyCurrentProjectionV2, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.history[fakeGovernanceRefKeyV1(ref.ID, ref.Revision, ref.Digest)]
	if !ok {
		return ports.ReviewDecisionPolicyCurrentProjectionV2{}, fakeGovernanceNotFoundV1("Review decision policy V2 historical projection")
	}
	return value.Clone(), nil
}

func (s *ReviewDecisionPolicyCurrentStoreV2) CommitPolicyV2(ctx context.Context, request ports.ReviewDecisionPolicyCurrentPublishRequestV2) (ports.ReviewDecisionPolicyCurrentPublishReceiptV2, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
	}
	s.mu.Lock()
	key := fakeGovernanceRefKeyV1(request.Value.Ref.ID, request.Value.Ref.Revision, request.Value.Ref.Digest)
	if existing, ok := s.history[key]; ok {
		s.mu.Unlock()
		if reflect.DeepEqual(existing, request.Value) {
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{Ref: request.Value.Ref, Created: false}, nil
		}
		return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, fakeGovernanceConflictV1("Review decision policy V2 exact Ref changed content")
	}
	id := request.Value.Ref.ID
	if request.Previous == nil {
		if _, ok := s.current[id]; ok || s.highest[id] != 0 {
			s.mu.Unlock()
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, fakeGovernanceConflictV1("Review decision policy V2 create-once conflict")
		}
	} else {
		current, ok := s.current[id]
		if !ok || current != *request.Previous || s.highest[id] != request.Previous.Revision {
			s.mu.Unlock()
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, fakeGovernanceConflictV1("Review decision policy V2 full-ref CAS conflict")
		}
		prior, ok := s.history[fakeGovernanceRefKeyV1(request.Previous.ID, request.Previous.Revision, request.Previous.Digest)]
		if !ok || !ports.SameReviewDecisionPolicyProjectionIdentityV2(prior.Subject, request.Value.Subject) || request.Value.CheckedUnixNano < prior.CheckedUnixNano {
			s.mu.Unlock()
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review decision policy V2 stable identity or clock regressed")
		}
	}
	s.history[key] = request.Value.Clone()
	s.current[id] = request.Value.Ref
	s.highest[id] = request.Value.Ref.Revision
	hook := s.afterCommit
	s.mu.Unlock()
	if hook != nil {
		if err := hook(); err != nil {
			return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{}, err
		}
	}
	return ports.ReviewDecisionPolicyCurrentPublishReceiptV2{Ref: request.Value.Ref, Created: true}, nil
}

var _ control.ReviewDecisionPolicyCurrentFactPortV2 = (*ReviewDecisionPolicyCurrentStoreV2)(nil)
