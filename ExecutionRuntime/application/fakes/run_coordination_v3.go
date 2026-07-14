package fakes

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RunCoordinationStoreV3 is a deterministic fault-injection fixture. It does
// not claim production durability or conformance.
type RunCoordinationStoreV3 struct {
	mu                  sync.Mutex
	facts               map[string]contract.RunCoordinationFactV3
	LoseNextCreateReply bool
	LoseNextCASReply    bool
	LoseCASReplies      int
}

func NewRunCoordinationStoreV3() *RunCoordinationStoreV3 {
	return &RunCoordinationStoreV3{facts: make(map[string]contract.RunCoordinationFactV3)}
}

var _ applicationports.RunCoordinationFactPortV3 = (*RunCoordinationStoreV3)(nil)

func (s *RunCoordinationStoreV3) CreateRunCoordinationV3(_ context.Context, fact contract.RunCoordinationFactV3) (contract.RunCoordinationFactV3, error) {
	if err := fact.Validate(); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if fact.Revision != 1 || fact.State != contract.RunCoordinationCreatePlannedV3 {
		return contract.RunCoordinationFactV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "new run coordination must begin at create_planned")
	}
	digest, err := fact.DigestV3()
	if err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	key := runCoordinationKeyV3(fact.Scope, fact.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.facts[key]; ok {
		currentDigest, _ := current.DigestV3()
		if currentDigest == digest {
			return cloneRunCoordinationV3(current), nil
		}
		return contract.RunCoordinationFactV3{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "run coordination ID already binds different content")
	}
	s.facts[key] = cloneRunCoordinationV3(fact)
	if s.LoseNextCreateReply {
		s.LoseNextCreateReply = false
		return contract.RunCoordinationFactV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected run coordination create reply loss")
	}
	return cloneRunCoordinationV3(fact), nil
}

func (s *RunCoordinationStoreV3) InspectRunCoordinationV3(_ context.Context, scope core.ExecutionScope, id string) (contract.RunCoordinationFactV3, error) {
	if err := scope.Validate(); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.facts[runCoordinationKeyV3(scope, id)]
	if !ok {
		return contract.RunCoordinationFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "run coordination not found")
	}
	return cloneRunCoordinationV3(current), nil
}

func (s *RunCoordinationStoreV3) CompareAndSwapRunCoordinationV3(_ context.Context, request applicationports.RunCoordinationCASRequestV3) (contract.RunCoordinationFactV3, error) {
	if err := request.Scope.Validate(); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if request.ExpectedRevision == 0 || request.Next.ID != request.ID || request.Next.Revision != request.ExpectedRevision+1 || !runtimeports.SameExecutionScopeV2(request.Scope, request.Next.Scope) {
		return contract.RunCoordinationFactV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "run coordination CAS key/revisions are incomplete")
	}
	key := runCoordinationKeyV3(request.Scope, request.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.facts[key]
	if !ok {
		return contract.RunCoordinationFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "run coordination not found")
	}
	if current.Revision != request.ExpectedRevision {
		return contract.RunCoordinationFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "run coordination revision changed")
	}
	if err := contract.ValidateRunCoordinationTransitionV3(current, request.Next); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	s.facts[key] = cloneRunCoordinationV3(request.Next)
	if s.LoseNextCASReply || s.LoseCASReplies > 0 {
		s.LoseNextCASReply = false
		if s.LoseCASReplies > 0 {
			s.LoseCASReplies--
		}
		return contract.RunCoordinationFactV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected run coordination CAS reply loss")
	}
	return cloneRunCoordinationV3(request.Next), nil
}

func runCoordinationKeyV3(scope core.ExecutionScope, id string) string {
	digest, _ := runtimeports.ExecutionScopeDigestV2(scope)
	return string(digest) + "\x00" + id
}

func cloneRunCoordinationV3(value contract.RunCoordinationFactV3) contract.RunCoordinationFactV3 {
	clone := value
	clone.Scope = cloneScopeV2(value.Scope)
	clone.CreateRequest.Run.Scope = cloneScopeV2(value.CreateRequest.Run.Scope)
	clone.CreateRequest.Plan.ExecutionScope = cloneScopeV2(value.CreateRequest.Plan.ExecutionScope)
	clone.CreateRequest.Plan.Requirements = append([]runtimeports.RunSettlementRequirementV2(nil), value.CreateRequest.Plan.Requirements...)
	if value.Lifecycle != nil {
		v := *value.Lifecycle
		v.Run.Scope = cloneScopeV2(value.Lifecycle.Run.Scope)
		if value.Lifecycle.Run.CompletionClaim != nil {
			claim := *value.Lifecycle.Run.CompletionClaim
			v.Run.CompletionClaim = &claim
		}
		if value.Lifecycle.Closure != nil {
			closure := *value.Lifecycle.Closure
			v.Closure = &closure
		}
		if value.Lifecycle.Decision != nil {
			decision := *value.Lifecycle.Decision
			v.Decision = &decision
		}
		if value.Lifecycle.Progress != nil {
			progress := *value.Lifecycle.Progress
			v.Progress = &progress
		}
		if value.Lifecycle.Report != nil {
			report := *value.Lifecycle.Report
			v.Report = &report
		}
		clone.Lifecycle = &v
	}
	if value.StartAttempt != nil {
		v := cloneOperationAttemptV3(*value.StartAttempt)
		clone.StartAttempt = &v
	}
	if value.StartOperation != nil {
		v := *value.StartOperation
		v.ExecutionScope = cloneScopeV2(value.StartOperation.ExecutionScope)
		clone.StartOperation = &v
	}
	if value.StartRuntimeAttempt != nil {
		v := cloneGovernedExecutionAttemptRefsV3(*value.StartRuntimeAttempt)
		clone.StartRuntimeAttempt = &v
	}
	if value.StartConfirmation != nil {
		v := *value.StartConfirmation
		v.ExecutionScope = cloneScopeV2(value.StartConfirmation.ExecutionScope)
		v.Attempt = cloneGovernedExecutionAttemptRefsV3(value.StartConfirmation.Attempt)
		clone.StartConfirmation = &v
	}
	if value.ClaimCandidate != nil {
		v := *value.ClaimCandidate
		v.ExecutionScope = cloneScopeV2(value.ClaimCandidate.ExecutionScope)
		v.Causation = append([]runtimeports.EvidenceCausationRefV2(nil), value.ClaimCandidate.Causation...)
		if value.ClaimCandidate.OwnerFact != nil {
			owner := *value.ClaimCandidate.OwnerFact
			v.OwnerFact = &owner
		}
		if value.ClaimCandidate.HistoricalSource != nil {
			historical := *value.ClaimCandidate.HistoricalSource
			v.HistoricalSource = &historical
		}
		clone.ClaimCandidate = &v
	}
	if value.ClaimResult != nil {
		v := *value.ClaimResult
		v.Run.Scope = cloneScopeV2(value.ClaimResult.Run.Scope)
		if value.ClaimResult.Run.CompletionClaim != nil {
			claim := *value.ClaimResult.Run.CompletionClaim
			v.Run.CompletionClaim = &claim
		}
		v.Evidence.Candidate.ExecutionScope = cloneScopeV2(value.ClaimResult.Evidence.Candidate.ExecutionScope)
		v.Evidence.Candidate.Causation = append([]runtimeports.EvidenceCausationRefV2(nil), value.ClaimResult.Evidence.Candidate.Causation...)
		if value.ClaimResult.Evidence.Candidate.OwnerFact != nil {
			owner := *value.ClaimResult.Evidence.Candidate.OwnerFact
			v.Evidence.Candidate.OwnerFact = &owner
		}
		if value.ClaimResult.Evidence.Candidate.HistoricalSource != nil {
			historical := *value.ClaimResult.Evidence.Candidate.HistoricalSource
			v.Evidence.Candidate.HistoricalSource = &historical
		}
		v.Association.ExecutionScope = cloneScopeV2(value.ClaimResult.Association.ExecutionScope)
		clone.ClaimResult = &v
	}
	return clone
}

func cloneGovernedExecutionAttemptRefsV3(value runtimeports.GovernedExecutionAttemptRefsV2) runtimeports.GovernedExecutionAttemptRefsV2 {
	clone := value
	if value.Observation != nil {
		o := *value.Observation
		clone.Observation = &o
	}
	if value.Settlement != nil {
		settlement := *value.Settlement
		settlement.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), value.Settlement.Evidence...)
		if value.Settlement.Attempt.Delegation != nil {
			delegation := *value.Settlement.Attempt.Delegation
			settlement.Attempt.Delegation = &delegation
		}
		if value.Settlement.Observation != nil {
			observation := *value.Settlement.Observation
			settlement.Observation = &observation
		}
		if value.Settlement.DomainResultSchema != nil {
			schema := *value.Settlement.DomainResultSchema
			settlement.DomainResultSchema = &schema
		}
		if value.Settlement.InspectionEffect != nil {
			effect := *value.Settlement.InspectionEffect
			if effect.Delegation != nil {
				delegation := *effect.Delegation
				effect.Delegation = &delegation
			}
			settlement.InspectionEffect = &effect
		}
		if value.Settlement.InspectionSettlement != nil {
			inspection := *value.Settlement.InspectionSettlement
			settlement.InspectionSettlement = &inspection
		}
		clone.Settlement = &settlement
	}
	return clone
}
