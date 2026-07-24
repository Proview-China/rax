package fakes

import (
	"context"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// DispatchAuthorityCurrentStoreV3 is a deterministic test/conformance fake.
// It is not production persistence, composition or an SLA claim.
type DispatchAuthorityCurrentStoreV3 struct {
	mu          sync.RWMutex
	history     map[string]ports.DispatchAuthorityFactV3
	current     map[string]ports.AuthorityBindingRefV2
	highest     map[string]core.Revision
	afterCommit func() error
}

func NewDispatchAuthorityCurrentStoreV3() *DispatchAuthorityCurrentStoreV3 {
	return &DispatchAuthorityCurrentStoreV3{history: map[string]ports.DispatchAuthorityFactV3{}, current: map[string]ports.AuthorityBindingRefV2{}, highest: map[string]core.Revision{}}
}
func (s *DispatchAuthorityCurrentStoreV3) SetAfterCommitHookV3(hook func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.afterCommit = hook
}

func (s *DispatchAuthorityCurrentStoreV3) InspectCurrentAuthorityFactV3(ctx context.Context, expected ports.AuthorityBindingRefV2) (ports.DispatchAuthorityFactV3, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	current, ok := s.current[expected.Ref]
	if !ok {
		return ports.DispatchAuthorityFactV3{}, fakeGovernanceNotFoundV1("dispatch authority V3 current fact")
	}
	if current != expected || s.highest[expected.Ref] != expected.Revision {
		return ports.DispatchAuthorityFactV3{}, fakeGovernanceConflictV1("dispatch authority V3 current full Ref drifted")
	}
	value, ok := s.history[dispatchAuthorityKeyV3(expected)]
	if !ok || value.Ref != expected {
		return ports.DispatchAuthorityFactV3{}, fakeGovernanceConflictV1("dispatch authority V3 current history is incomplete")
	}
	return value.Clone(), nil
}
func (s *DispatchAuthorityCurrentStoreV3) InspectHistoricalAuthorityFactV3(ctx context.Context, expected ports.AuthorityBindingRefV2) (ports.DispatchAuthorityFactV3, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.DispatchAuthorityFactV3{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.history[dispatchAuthorityKeyV3(expected)]
	if !ok || value.Ref != expected {
		return ports.DispatchAuthorityFactV3{}, fakeGovernanceNotFoundV1("dispatch authority V3 historical fact")
	}
	return value.Clone(), nil
}
func (s *DispatchAuthorityCurrentStoreV3) CommitAuthorityFactV3(ctx context.Context, request ports.DispatchAuthorityFactPublishRequestV3) (ports.DispatchAuthorityFactPublishReceiptV3, error) {
	if err := fakeGovernanceContextV1(ctx); err != nil {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.DispatchAuthorityFactPublishReceiptV3{}, err
	}
	value := request.Value.Clone()
	s.mu.Lock()
	key := dispatchAuthorityKeyV3(value.Ref)
	if existing, ok := s.history[key]; ok {
		s.mu.Unlock()
		if reflect.DeepEqual(existing, value) {
			return ports.DispatchAuthorityFactPublishReceiptV3{Ref: value.Ref, Created: false}, nil
		}
		return ports.DispatchAuthorityFactPublishReceiptV3{}, fakeGovernanceConflictV1("dispatch authority V3 exact Ref changed content")
	}
	if request.Previous == nil {
		if _, ok := s.current[value.Ref.Ref]; ok || s.highest[value.Ref.Ref] != 0 {
			s.mu.Unlock()
			return ports.DispatchAuthorityFactPublishReceiptV3{}, fakeGovernanceConflictV1("dispatch authority V3 create-once conflict")
		}
	} else {
		current, ok := s.current[value.Ref.Ref]
		if !ok || current != *request.Previous || s.highest[value.Ref.Ref] != request.Previous.Revision {
			s.mu.Unlock()
			return ports.DispatchAuthorityFactPublishReceiptV3{}, fakeGovernanceConflictV1("dispatch authority V3 full-ref CAS conflict")
		}
		prior, ok := s.history[dispatchAuthorityKeyV3(*request.Previous)]
		if !ok || !ports.SameDispatchAuthorityStableIdentityV3(prior, value) || value.CheckedUnixNano < prior.CheckedUnixNano || prior.State != ports.AuthorityFactActive {
			s.mu.Unlock()
			return ports.DispatchAuthorityFactPublishReceiptV3{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "dispatch authority V3 identity, state or sealed clock regressed")
		}
	}
	s.history[key] = value.Clone()
	s.current[value.Ref.Ref] = value.Ref
	s.highest[value.Ref.Ref] = value.Ref.Revision
	hook := s.afterCommit
	s.mu.Unlock()
	if hook != nil {
		if err := hook(); err != nil {
			return ports.DispatchAuthorityFactPublishReceiptV3{}, err
		}
	}
	return ports.DispatchAuthorityFactPublishReceiptV3{Ref: value.Ref, Created: true}, nil
}
func dispatchAuthorityKeyV3(ref ports.AuthorityBindingRefV2) string {
	return fakeGovernanceRefKeyV1(ref.Ref, ref.Revision, ref.Digest)
}

var _ control.DispatchAuthorityCurrentFactPortV3 = (*DispatchAuthorityCurrentStoreV3)(nil)
