package fakes

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// GovernedOperationAttemptStoreV3 is deterministic and process-local. It is a
// fault-injection fixture, never a production durability claim.
type GovernedOperationAttemptStoreV3 struct {
	mu                  sync.Mutex
	facts               map[string]contract.GovernedOperationAttemptFactV3
	LoseNextCreateReply bool
	LoseNextCASReply    bool
}

func NewGovernedOperationAttemptStoreV3() *GovernedOperationAttemptStoreV3 {
	return &GovernedOperationAttemptStoreV3{facts: make(map[string]contract.GovernedOperationAttemptFactV3)}
}

var _ applicationports.GovernedOperationAttemptFactPortV3 = (*GovernedOperationAttemptStoreV3)(nil)

func (s *GovernedOperationAttemptStoreV3) CreateGovernedOperationAttemptV3(_ context.Context, fact contract.GovernedOperationAttemptFactV3) (contract.GovernedOperationAttemptFactV3, error) {
	if err := fact.Validate(); err != nil {
		return contract.GovernedOperationAttemptFactV3{}, err
	}
	if fact.Revision != 1 || fact.State != contract.OperationIntentRecordedV3 {
		return contract.GovernedOperationAttemptFactV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "new operation attempt must begin at intent_recorded revision 1")
	}
	digest, err := fact.DigestV3()
	if err != nil {
		return contract.GovernedOperationAttemptFactV3{}, err
	}
	key := operationAttemptKeyV3(fact.Scope, fact.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.facts == nil {
		s.facts = make(map[string]contract.GovernedOperationAttemptFactV3)
	}
	if current, ok := s.facts[key]; ok {
		currentDigest, _ := current.DigestV3()
		if currentDigest == digest {
			return cloneOperationAttemptV3(current), nil
		}
		return contract.GovernedOperationAttemptFactV3{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "operation attempt ID already binds different content")
	}
	s.facts[key] = cloneOperationAttemptV3(fact)
	if s.LoseNextCreateReply {
		s.LoseNextCreateReply = false
		return contract.GovernedOperationAttemptFactV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected operation attempt create reply loss")
	}
	return cloneOperationAttemptV3(fact), nil
}

func (s *GovernedOperationAttemptStoreV3) InspectGovernedOperationAttemptV3(_ context.Context, scope core.ExecutionScope, id string) (contract.GovernedOperationAttemptFactV3, error) {
	if err := scope.Validate(); err != nil {
		return contract.GovernedOperationAttemptFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.facts[operationAttemptKeyV3(scope, id)]
	if !ok {
		return contract.GovernedOperationAttemptFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "operation attempt not found")
	}
	return cloneOperationAttemptV3(current), nil
}

func (s *GovernedOperationAttemptStoreV3) CompareAndSwapGovernedOperationAttemptV3(_ context.Context, request applicationports.GovernedOperationAttemptCASRequestV3) (contract.GovernedOperationAttemptFactV3, error) {
	if err := request.Scope.Validate(); err != nil {
		return contract.GovernedOperationAttemptFactV3{}, err
	}
	if request.ExpectedRevision == 0 || request.Next.ID != request.ID || request.Next.Revision != request.ExpectedRevision+1 || !runtimeports.SameExecutionScopeV2(request.Scope, request.Next.Scope) {
		return contract.GovernedOperationAttemptFactV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "operation attempt CAS key/revisions are incomplete")
	}
	key := operationAttemptKeyV3(request.Scope, request.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.facts[key]
	if !ok {
		return contract.GovernedOperationAttemptFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "operation attempt not found")
	}
	if current.Revision != request.ExpectedRevision {
		return contract.GovernedOperationAttemptFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "operation attempt revision changed")
	}
	if err := contract.ValidateGovernedOperationAttemptTransitionV3(current, request.Next); err != nil {
		return contract.GovernedOperationAttemptFactV3{}, err
	}
	s.facts[key] = cloneOperationAttemptV3(request.Next)
	if s.LoseNextCASReply {
		s.LoseNextCASReply = false
		return contract.GovernedOperationAttemptFactV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected operation attempt CAS reply loss")
	}
	return cloneOperationAttemptV3(request.Next), nil
}

func operationAttemptKeyV3(scope core.ExecutionScope, id string) string {
	digest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	return string(digest) + "\x00" + id
}

func cloneOperationAttemptV3(value contract.GovernedOperationAttemptFactV3) contract.GovernedOperationAttemptFactV3 {
	clone := value
	clone.Scope = cloneScopeV2(value.Scope)
	clone.Operation.ExecutionScope = cloneScopeV2(value.Operation.ExecutionScope)
	clone.IntentValue.Operation.ExecutionScope = cloneScopeV2(value.IntentValue.Operation.ExecutionScope)
	clone.IntentValue.Payload.Inline = append([]byte(nil), value.IntentValue.Payload.Inline...)
	if value.IntentValue.Owners != nil {
		clone.IntentValue.Owners = append([]runtimeports.EffectOwnerRefV2{}, value.IntentValue.Owners...)
	}
	if value.IntentValue.CredentialLeases != nil {
		clone.IntentValue.CredentialLeases = append([]runtimeports.CredentialLeaseRefV2{}, value.IntentValue.CredentialLeases...)
	}
	if value.DelegationPlan.RelayHops != nil {
		clone.DelegationPlan.RelayHops = append([]runtimeports.ExecutionRelayHopV2{}, value.DelegationPlan.RelayHops...)
	}
	if value.DomainReservation != nil {
		reservation := *value.DomainReservation
		clone.DomainReservation = &reservation
	}
	if value.Admission != nil {
		v := *value.Admission
		clone.Admission = &v
	}
	if value.IssuedAuthorization != nil {
		v := cloneAuthorizationV3(*value.IssuedAuthorization)
		clone.IssuedAuthorization = &v
	}
	if value.BegunAuthorization != nil {
		v := cloneAuthorizationV3(*value.BegunAuthorization)
		clone.BegunAuthorization = &v
	}
	if value.UnknownAuthorization != nil {
		v := cloneAuthorizationV3(*value.UnknownAuthorization)
		clone.UnknownAuthorization = &v
	}
	if value.DelegationFact != nil {
		v := *value.DelegationFact
		v.Operation.ExecutionScope = cloneScopeV2(value.DelegationFact.Operation.ExecutionScope)
		if value.DelegationFact.RelayHops != nil {
			v.RelayHops = append([]runtimeports.ExecutionRelayHopV2{}, value.DelegationFact.RelayHops...)
		}
		clone.DelegationFact = &v
	}
	if value.DeclaredDelegation != nil {
		v := *value.DeclaredDelegation
		clone.DeclaredDelegation = &v
	}
	if value.PreparedDelegation != nil {
		v := *value.PreparedDelegation
		clone.PreparedDelegation = &v
	}
	if value.Prepared != nil {
		v := *value.Prepared
		clone.Prepared = &v
	}
	if value.Enforcement != nil {
		v := *value.Enforcement
		clone.Enforcement = &v
	}
	if value.Observation != nil {
		v := *value.Observation
		clone.Observation = &v
	}
	if value.Settlement != nil {
		v := *value.Settlement
		v.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), value.Settlement.Evidence...)
		if value.Settlement.Attempt.Delegation != nil {
			delegation := *value.Settlement.Attempt.Delegation
			v.Attempt.Delegation = &delegation
		}
		if value.Settlement.Observation != nil {
			observation := *value.Settlement.Observation
			v.Observation = &observation
		}
		if value.Settlement.DomainResultSchema != nil {
			schema := *value.Settlement.DomainResultSchema
			v.DomainResultSchema = &schema
		}
		clone.Settlement = &v
	}
	if value.SettlementDomainResult != nil {
		v := *value.SettlementDomainResult
		v.Inline = append([]byte(nil), value.SettlementDomainResult.Inline...)
		clone.SettlementDomainResult = &v
	}
	return clone
}

func cloneAuthorizationV3(value runtimeports.OperationDispatchAuthorizationV3) runtimeports.OperationDispatchAuthorizationV3 {
	clone := value
	if value.Attempt.Delegation != nil {
		delegation := *value.Attempt.Delegation
		clone.Attempt.Delegation = &delegation
	}
	clone.Permit.Operation.ExecutionScope = cloneScopeV2(value.Permit.Operation.ExecutionScope)
	if value.Permit.ReviewAuthorization.Satisfaction != nil {
		satisfaction := *value.Permit.ReviewAuthorization.Satisfaction
		clone.Permit.ReviewAuthorization.Satisfaction = &satisfaction
	}
	clone.Fence.Scope = cloneScopeV2(value.Fence.Scope)
	return clone
}
