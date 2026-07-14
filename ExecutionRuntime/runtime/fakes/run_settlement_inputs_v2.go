package fakes

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RunSettlementInputStoreV2 is a deterministic test-only collection of
// independent owner projections. Storing them together here does not make it a
// production multi-domain owner and does not grant any component authority.
type RunSettlementInputStoreV2 struct {
	mu           sync.Mutex
	policies     map[string]ports.RunSettlementPolicyFactV2
	participants map[string]ports.RunSettlementParticipantFactV2
	executions   map[core.AgentRunID]ports.ExecutionSettlementInspectionV2
}

func NewRunSettlementInputStoreV2() *RunSettlementInputStoreV2 {
	return &RunSettlementInputStoreV2{policies: map[string]ports.RunSettlementPolicyFactV2{}, participants: map[string]ports.RunSettlementParticipantFactV2{}, executions: map[core.AgentRunID]ports.ExecutionSettlementInspectionV2{}}
}

func (s *RunSettlementInputStoreV2) PutPolicy(fact ports.RunSettlementPolicyFactV2) error {
	digest, err := fact.DigestV2()
	if err != nil {
		return err
	}
	fact.Digest = digest
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.policies[fact.Ref]; exists && current.Digest != fact.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementRequirementInvalid, "test policy ref already binds different content")
	}
	s.policies[fact.Ref] = fact
	return nil
}

func (s *RunSettlementInputStoreV2) PutParticipant(fact ports.RunSettlementParticipantFactV2) error {
	if err := fact.Validate(); err != nil {
		return err
	}
	key := participantInputKeyV2(fact.RunID, fact.RequirementID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.participants[key]; exists {
		left, _ := current.DigestV2()
		right, _ := fact.DigestV2()
		if left == right {
			return nil
		}
		if fact.Revision != current.Revision+1 || fact.ID != current.ID || fact.RunID != current.RunID || fact.RunIdentityDigest != current.RunIdentityDigest || fact.ExecutionScopeDigest != current.ExecutionScopeDigest || fact.Plan != current.Plan || fact.RequirementID != current.RequirementID || fact.RequirementDigest != current.RequirementDigest || fact.SubjectDigest != current.SubjectDigest || fact.Owner != current.Owner || fact.CreatedUnixNano != current.CreatedUnixNano {
			return core.NewError(core.ErrorConflict, core.ReasonRunSettlementParticipantStale, "test participant transition changed immutable identity or skipped a revision")
		}
		if current.Disposition != ports.RunSettlementUnknown {
			return core.NewError(core.ErrorConflict, core.ReasonRunSettlementParticipantStale, "resolved participant disposition is immutable")
		}
	}
	s.participants[key] = cloneRunSettlementParticipantV2(fact)
	return nil
}

func (s *RunSettlementInputStoreV2) PutExecution(fact ports.ExecutionSettlementInspectionV2) error {
	if err := fact.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executions[fact.RunID] = cloneExecutionSettlementInspectionV2(fact)
	return nil
}

func (s *RunSettlementInputStoreV2) InspectRunSettlementPolicy(ctx context.Context, ref string) (ports.RunSettlementPolicyFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.RunSettlementPolicyFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, exists := s.policies[ref]
	if !exists {
		return ports.RunSettlementPolicyFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonRunSettlementRequirementInvalid, "Run settlement policy does not exist")
	}
	return fact, nil
}

func (s *RunSettlementInputStoreV2) InspectRunSettlementParticipant(ctx context.Context, request ports.RunSettlementParticipantInspectRequestV2) (ports.RunSettlementParticipantFactV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.RunSettlementParticipantFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, exists := s.participants[participantInputKeyV2(request.RunID, request.RequirementID)]
	if !exists {
		return ports.RunSettlementParticipantFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonRunSettlementParticipantMissing, "Run settlement participant does not exist")
	}
	if fact.RunIdentityDigest != request.RunIdentityDigest || fact.ExecutionScopeDigest != request.ExecutionScopeDigest || fact.Plan != request.Plan || fact.RequirementDigest != request.RequirementDigest || fact.SubjectDigest != request.SubjectDigest || fact.Owner != request.Owner || !ports.SameExecutionScopeV2(fact.ExecutionScope, request.ExecutionScope) {
		return ports.RunSettlementParticipantFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementParticipantStale, "Run settlement participant inspect request does not match")
	}
	return cloneRunSettlementParticipantV2(fact), nil
}

func (s *RunSettlementInputStoreV2) InspectRunExecutionV2(ctx context.Context, request ports.RunExecutionInspectionRequestV2) (ports.ExecutionSettlementInspectionV2, error) {
	if err := contextError(ctx); err != nil {
		return ports.ExecutionSettlementInspectionV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, exists := s.executions[request.RunID]
	if !exists {
		return ports.ExecutionSettlementInspectionV2{}, core.NewError(core.ErrorNotFound, core.ReasonExecutionInspectionInvalid, "Execution inspection does not exist")
	}
	if fact.RunIdentityDigest != request.RunIdentityDigest || fact.RunRevision != request.ExpectedRunRevision || !ports.SameExecutionScopeV2(fact.ExecutionScope, request.ExecutionScope) || fact.Subject != request.Subject {
		return ports.ExecutionSettlementInspectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonExecutionInspectionInvalid, "Execution inspection request does not match")
	}
	return cloneExecutionSettlementInspectionV2(fact), nil
}

func participantInputKeyV2(runID core.AgentRunID, requirement ports.NamespacedNameV2) string {
	return string(runID) + "\x00" + string(requirement)
}

func cloneRunSettlementParticipantV2(f ports.RunSettlementParticipantFactV2) ports.RunSettlementParticipantFactV2 {
	f.ExecutionScope = cloneScope(f.ExecutionScope)
	f.Evidence = append([]ports.EvidenceRecordRefV2{}, f.Evidence...)
	if f.Policy != nil {
		policy := *f.Policy
		f.Policy = &policy
	}
	if f.Payload != nil {
		payload := *f.Payload
		payload.Inline = append([]byte{}, f.Payload.Inline...)
		f.Payload = &payload
	}
	return f
}

func cloneExecutionSettlementInspectionV2(f ports.ExecutionSettlementInspectionV2) ports.ExecutionSettlementInspectionV2 {
	f.ExecutionScope = cloneScope(f.ExecutionScope)
	return f
}

var _ ports.RunSettlementPolicyReaderV2 = (*RunSettlementInputStoreV2)(nil)
var _ ports.RunSettlementParticipantPortV2 = (*RunSettlementInputStoreV2)(nil)
var _ ports.RunExecutionSettlementInspectorV2 = (*RunSettlementInputStoreV2)(nil)
