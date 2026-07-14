package fakes

import (
	"context"
	"sync"

	bridgecontract "github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ModelTurnOperationBindingStoreV3 is deterministic and process-local. It is
// only a Fact Port test double and makes no production durability claim.
type ModelTurnOperationBindingStoreV3 struct {
	mu                  sync.Mutex
	facts               map[string]bridgecontract.ModelTurnOperationBindingFactV3
	LoseNextCreateReply bool
	LoseNextCASReply    bool
}

func NewModelTurnOperationBindingStoreV3() *ModelTurnOperationBindingStoreV3 {
	return &ModelTurnOperationBindingStoreV3{facts: make(map[string]bridgecontract.ModelTurnOperationBindingFactV3)}
}

var _ harnessports.ModelTurnOperationBindingFactPortV3 = (*ModelTurnOperationBindingStoreV3)(nil)

func (s *ModelTurnOperationBindingStoreV3) CreateModelTurnOperationBindingV3(_ context.Context, fact bridgecontract.ModelTurnOperationBindingFactV3) (bridgecontract.ModelTurnOperationBindingFactV3, error) {
	if err := fact.Validate(); err != nil {
		return bridgecontract.ModelTurnOperationBindingFactV3{}, err
	}
	if fact.Revision != 1 || fact.State != bridgecontract.ModelTurnOperationPreparedV3 && fact.State != bridgecontract.ModelTurnOperationSettledV3 {
		return bridgecontract.ModelTurnOperationBindingFactV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "new model-turn binding must be prepared or an undispatched terminal settlement at revision one")
	}
	key, err := modelTurnBindingKeyV3(fact.Scope, fact.StepKind, fact.ID)
	if err != nil {
		return bridgecontract.ModelTurnOperationBindingFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.facts == nil {
		s.facts = make(map[string]bridgecontract.ModelTurnOperationBindingFactV3)
	}
	if current, ok := s.facts[key]; ok {
		if sameModelTurnBindingV3(current, fact) {
			return cloneModelTurnBindingV3(current), nil
		}
		return bridgecontract.ModelTurnOperationBindingFactV3{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "model-turn operation attempt already binds different content")
	}
	s.facts[key] = cloneModelTurnBindingV3(fact)
	if s.LoseNextCreateReply {
		s.LoseNextCreateReply = false
		return bridgecontract.ModelTurnOperationBindingFactV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected model-turn binding create reply loss")
	}
	return cloneModelTurnBindingV3(fact), nil
}

func (s *ModelTurnOperationBindingStoreV3) InspectModelTurnOperationBindingV3(_ context.Context, scope core.ExecutionScope, step runtimeports.NamespacedNameV2, id string) (bridgecontract.ModelTurnOperationBindingFactV3, error) {
	key, err := modelTurnBindingKeyV3(scope, step, id)
	if err != nil {
		return bridgecontract.ModelTurnOperationBindingFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.facts[key]
	if !ok {
		return bridgecontract.ModelTurnOperationBindingFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "model-turn operation binding not found")
	}
	return cloneModelTurnBindingV3(current), nil
}

func (s *ModelTurnOperationBindingStoreV3) CompareAndSwapModelTurnOperationBindingV3(_ context.Context, request harnessports.ModelTurnOperationBindingCASRequestV3) (bridgecontract.ModelTurnOperationBindingFactV3, error) {
	if err := request.Validate(); err != nil {
		return bridgecontract.ModelTurnOperationBindingFactV3{}, err
	}
	key, err := modelTurnBindingKeyV3(request.Scope, request.StepKind, request.ID)
	if err != nil {
		return bridgecontract.ModelTurnOperationBindingFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.facts[key]
	if !ok {
		return bridgecontract.ModelTurnOperationBindingFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "model-turn operation binding not found")
	}
	if current.Revision != request.ExpectedRevision {
		return bridgecontract.ModelTurnOperationBindingFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "model-turn operation binding revision changed")
	}
	if err := bridgecontract.ValidateModelTurnOperationBindingTransitionV3(current, request.Next); err != nil {
		return bridgecontract.ModelTurnOperationBindingFactV3{}, err
	}
	s.facts[key] = cloneModelTurnBindingV3(request.Next)
	if s.LoseNextCASReply {
		s.LoseNextCASReply = false
		return bridgecontract.ModelTurnOperationBindingFactV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected model-turn binding CAS reply loss")
	}
	return cloneModelTurnBindingV3(request.Next), nil
}

func modelTurnBindingKeyV3(scope core.ExecutionScope, step runtimeports.NamespacedNameV2, id string) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	if runtimeports.ValidateNamespacedNameV2(step) != nil || id == "" {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "model-turn binding key is incomplete")
	}
	digest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		return "", err
	}
	return string(digest) + "\x00" + string(step) + "\x00" + id, nil
}

func sameModelTurnBindingV3(left, right bridgecontract.ModelTurnOperationBindingFactV3) bool {
	ld, le := left.DigestV3()
	rd, re := right.DigestV3()
	return le == nil && re == nil && ld == rd
}

func cloneModelTurnBindingV3(value bridgecontract.ModelTurnOperationBindingFactV3) bridgecontract.ModelTurnOperationBindingFactV3 {
	clone := value
	clone.Scope = cloneGovernedScopeV2(value.Scope)
	clone.Run.Scope = cloneGovernedScopeV2(value.Run.Scope)
	clone.ApplicationAttempt = value.ApplicationAttempt
	if value.ApplicationAttempt.DomainReservation != nil {
		reservation := *value.ApplicationAttempt.DomainReservation
		clone.ApplicationAttempt.DomainReservation = &reservation
	}
	if value.ApplicationAttempt.Settlement != nil {
		settlement := cloneOperationSettlementRefV3(*value.ApplicationAttempt.Settlement)
		clone.ApplicationAttempt.Settlement = &settlement
	}
	if value.DelegationFact != nil {
		delegation := *value.DelegationFact
		delegation.Operation.ExecutionScope = cloneGovernedScopeV2(value.DelegationFact.Operation.ExecutionScope)
		delegation.RelayHops = append([]runtimeports.ExecutionRelayHopV2(nil), value.DelegationFact.RelayHops...)
		if value.DelegationFact.Preparation != nil {
			preparation := *value.DelegationFact.Preparation
			delegation.Preparation = &preparation
		}
		clone.DelegationFact = &delegation
	}
	if value.UnknownAuthorization != nil {
		authorization := cloneOperationAuthorizationV3(*value.UnknownAuthorization)
		clone.UnknownAuthorization = &authorization
	}
	if value.RuntimeAttempt != nil {
		runtimeAttempt := *value.RuntimeAttempt
		if value.RuntimeAttempt.Observation != nil {
			observation := *value.RuntimeAttempt.Observation
			runtimeAttempt.Observation = &observation
		}
		if value.RuntimeAttempt.Settlement != nil {
			settlement := cloneOperationSettlementRefV3(*value.RuntimeAttempt.Settlement)
			runtimeAttempt.Settlement = &settlement
		}
		clone.RuntimeAttempt = &runtimeAttempt
	}
	if value.Settlement != nil {
		settlement := cloneOperationSettlementRefV3(*value.Settlement)
		clone.Settlement = &settlement
	}
	if value.DomainResult != nil {
		result := *value.DomainResult
		result.Inline = append([]byte(nil), value.DomainResult.Inline...)
		clone.DomainResult = &result
	}
	return clone
}

func cloneOperationSettlementRefV3(value runtimeports.OperationSettlementRefV3) runtimeports.OperationSettlementRefV3 {
	clone := value
	clone.Attempt = cloneOperationDispatchAttemptRefV3(value.Attempt)
	if value.Observation != nil {
		observation := *value.Observation
		clone.Observation = &observation
	}
	if value.InspectionEffect != nil {
		inspection := cloneOperationDispatchAttemptRefV3(*value.InspectionEffect)
		clone.InspectionEffect = &inspection
	}
	if value.InspectionSettlement != nil {
		inspection := cloneOperationInspectionSettlementRefV3(*value.InspectionSettlement)
		clone.InspectionSettlement = &inspection
	}
	clone.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), value.Evidence...)
	if value.DomainResultSchema != nil {
		schema := *value.DomainResultSchema
		clone.DomainResultSchema = &schema
	}
	return clone
}

func cloneOperationInspectionSettlementRefV3(value runtimeports.OperationInspectionSettlementRefV3) runtimeports.OperationInspectionSettlementRefV3 {
	clone := value
	clone.Attempt = cloneOperationDispatchAttemptRefV3(value.Attempt)
	if value.Observation != nil {
		observation := *value.Observation
		clone.Observation = &observation
	}
	clone.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), value.Evidence...)
	if value.DomainResultSchema != nil {
		schema := *value.DomainResultSchema
		clone.DomainResultSchema = &schema
	}
	return clone
}

func cloneOperationDispatchAttemptRefV3(value runtimeports.OperationDispatchAttemptRefV3) runtimeports.OperationDispatchAttemptRefV3 {
	clone := value
	if value.Delegation != nil {
		delegation := *value.Delegation
		clone.Delegation = &delegation
	}
	return clone
}

func cloneOperationAuthorizationV3(value runtimeports.OperationDispatchAuthorizationV3) runtimeports.OperationDispatchAuthorizationV3 {
	clone := value
	clone.Attempt = cloneOperationDispatchAttemptRefV3(value.Attempt)
	clone.Permit.Operation.ExecutionScope = cloneGovernedScopeV2(value.Permit.Operation.ExecutionScope)
	if value.Permit.ReviewAuthorization.Satisfaction != nil {
		satisfaction := *value.Permit.ReviewAuthorization.Satisfaction
		clone.Permit.ReviewAuthorization.Satisfaction = &satisfaction
	}
	clone.Fence.Scope = cloneGovernedScopeV2(value.Fence.Scope)
	return clone
}
