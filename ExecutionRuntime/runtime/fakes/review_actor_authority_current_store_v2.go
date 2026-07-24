package fakes

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"reflect"
	"sync"
)

type ReviewActorAuthorityCurrentStoreV2 struct {
	mu          sync.RWMutex
	history     map[string]ports.ReviewActorAuthorityCurrentProjectionV2
	current     map[string]ports.ReviewActorAuthorityCurrentProjectionRefV2
	highest     map[string]core.Revision
	afterCommit func() error
}

func NewReviewActorAuthorityCurrentStoreV2() *ReviewActorAuthorityCurrentStoreV2 {
	return &ReviewActorAuthorityCurrentStoreV2{history: map[string]ports.ReviewActorAuthorityCurrentProjectionV2{}, current: map[string]ports.ReviewActorAuthorityCurrentProjectionRefV2{}, highest: map[string]core.Revision{}}
}
func (s *ReviewActorAuthorityCurrentStoreV2) SetAfterCommitHookV2(hook func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.afterCommit = hook
}
func (s *ReviewActorAuthorityCurrentStoreV2) ResolveActorAuthorityV2(ctx context.Context, subject ports.ReviewActorAuthorityCurrentSubjectV2) (ports.ReviewActorAuthorityCurrentProjectionRefV2, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, err
	}
	id, err := ports.DeriveReviewActorAuthorityCurrentProjectionIDV2(subject)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ref, ok := s.current[id]
	if !ok {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, fakeGovernanceNotFoundV1("Review actor authority V2 current projection")
	}
	if s.highest[id] != ref.Revision {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, fakeGovernanceConflictV1("Review actor authority V2 current/highest drifted")
	}
	value, ok := s.history[fakeGovernanceRefKeyV1(ref.ID, ref.Revision, ref.Digest)]
	if !ok || value.Subject != subject {
		return ports.ReviewActorAuthorityCurrentProjectionRefV2{}, fakeGovernanceConflictV1("Review actor authority V2 current subject drifted")
	}
	return ref, nil
}
func (s *ReviewActorAuthorityCurrentStoreV2) InspectCurrentActorAuthorityV2(ctx context.Context, subject ports.ReviewActorAuthorityCurrentSubjectV2, expected ports.ReviewActorAuthorityCurrentProjectionRefV2) (ports.ReviewActorAuthorityCurrentProjectionV2, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	if err := subject.Validate(); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	id, err := ports.DeriveReviewActorAuthorityCurrentProjectionIDV2(subject)
	if err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	current, ok := s.current[id]
	if !ok {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, fakeGovernanceNotFoundV1("Review actor authority V2 current projection")
	}
	if current != expected || s.highest[id] != expected.Revision {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, fakeGovernanceConflictV1("Review actor authority V2 current full Ref drifted")
	}
	value, ok := s.history[fakeGovernanceRefKeyV1(expected.ID, expected.Revision, expected.Digest)]
	if !ok || value.Subject != subject {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, fakeGovernanceConflictV1("Review actor authority V2 history or subject drifted")
	}
	return value.Clone(), nil
}
func (s *ReviewActorAuthorityCurrentStoreV2) InspectHistoricalActorAuthorityV2(ctx context.Context, ref ports.ReviewActorAuthorityCurrentProjectionRefV2) (ports.ReviewActorAuthorityCurrentProjectionV2, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.history[fakeGovernanceRefKeyV1(ref.ID, ref.Revision, ref.Digest)]
	if !ok {
		return ports.ReviewActorAuthorityCurrentProjectionV2{}, fakeGovernanceNotFoundV1("Review actor authority V2 historical projection")
	}
	return value.Clone(), nil
}
func (s *ReviewActorAuthorityCurrentStoreV2) CommitActorAuthorityV2(ctx context.Context, request ports.ReviewActorAuthorityCurrentPublishRequestV2) (ports.ReviewActorAuthorityCurrentPublishReceiptV2, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
	}
	value := request.Value.Clone()
	s.mu.Lock()
	key := fakeGovernanceRefKeyV1(value.Ref.ID, value.Ref.Revision, value.Ref.Digest)
	if existing, ok := s.history[key]; ok {
		s.mu.Unlock()
		if reflect.DeepEqual(existing, value) {
			return ports.ReviewActorAuthorityCurrentPublishReceiptV2{Ref: value.Ref, Created: false}, nil
		}
		return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, fakeGovernanceConflictV1("Review actor authority V2 exact Ref changed content")
	}
	if request.Previous == nil {
		if _, ok := s.current[value.Ref.ID]; ok || s.highest[value.Ref.ID] != 0 {
			s.mu.Unlock()
			return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, fakeGovernanceConflictV1("Review actor authority V2 create-once conflict")
		}
	} else {
		current, ok := s.current[value.Ref.ID]
		if !ok || current != *request.Previous || s.highest[value.Ref.ID] != request.Previous.Revision {
			s.mu.Unlock()
			return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, fakeGovernanceConflictV1("Review actor authority V2 full-ref CAS conflict")
		}
		prior, ok := s.history[fakeGovernanceRefKeyV1(request.Previous.ID, request.Previous.Revision, request.Previous.Digest)]
		if !ok || !ports.SameReviewActorAuthorityStableIdentityV2(prior.Subject, value.Subject) || value.CheckedUnixNano < prior.CheckedUnixNano || prior.State != ports.ReviewDecisionGovernanceProjectionActiveV1 {
			s.mu.Unlock()
			return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review actor authority V2 identity, state or clock regressed")
		}
	}
	s.history[key] = value.Clone()
	s.current[value.Ref.ID] = value.Ref
	s.highest[value.Ref.ID] = value.Ref.Revision
	hook := s.afterCommit
	s.mu.Unlock()
	if hook != nil {
		if err := hook(); err != nil {
			return ports.ReviewActorAuthorityCurrentPublishReceiptV2{}, err
		}
	}
	return ports.ReviewActorAuthorityCurrentPublishReceiptV2{Ref: value.Ref, Created: true}, nil
}

var _ control.ReviewActorAuthorityCurrentFactPortV2 = (*ReviewActorAuthorityCurrentStoreV2)(nil)
