package repository

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type Memory struct {
	mu                 sync.RWMutex
	plans              map[planKey]contract.ResolvedAgentPlanV1
	current            map[string]contract.CurrentResolvedPlanV1
	currentPlanHistory map[string]map[contract.ResolvedAgentPlanRefV1]struct{}
}

type planKey struct {
	ID       string
	Revision core.Revision
}

func NewMemory() *Memory {
	return &Memory{
		plans:              map[planKey]contract.ResolvedAgentPlanV1{},
		current:            map[string]contract.CurrentResolvedPlanV1{},
		currentPlanHistory: map[string]map[contract.ResolvedAgentPlanRefV1]struct{}{},
	}
}

func (m *Memory) EnsureExactResolvedAgentPlanV1(ctx context.Context, candidate contract.ResolvedAgentPlanV1) (contract.ResolvedAgentPlanV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ResolvedAgentPlanV1{}, err
	}
	if err := candidate.Validate(); err != nil {
		return contract.ResolvedAgentPlanV1{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := planKey{ID: candidate.PlanID, Revision: candidate.Revision}
	if existing, ok := m.plans[key]; ok {
		if existing.Digest != candidate.Digest {
			return contract.ResolvedAgentPlanV1{}, core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "resolved plan identity is already occupied by different exact content")
		}
		return contract.CloneResolvedAgentPlanV1(existing), nil
	}
	m.plans[key] = contract.CloneResolvedAgentPlanV1(candidate)
	return contract.CloneResolvedAgentPlanV1(candidate), nil
}

func (m *Memory) InspectExactResolvedAgentPlanV1(ctx context.Context, ref contract.ResolvedAgentPlanRefV1) (contract.ResolvedAgentPlanV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ResolvedAgentPlanV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.ResolvedAgentPlanV1{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.plans[planKey{ID: ref.PlanID, Revision: ref.Revision}]
	if !ok {
		return contract.ResolvedAgentPlanV1{}, core.NewError(core.ErrorNotFound, core.ReasonPlanInvalid, "resolved plan is absent")
	}
	if value.Digest != ref.Digest {
		return contract.ResolvedAgentPlanV1{}, core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "resolved plan exact ref drifted")
	}
	return contract.CloneResolvedAgentPlanV1(value), nil
}

func (m *Memory) InspectCurrentResolvedAgentPlanV1(ctx context.Context, definitionID string) (contract.CurrentResolvedPlanV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.CurrentResolvedPlanV1{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.current[definitionID]
	if !ok {
		return contract.CurrentResolvedPlanV1{}, core.NewError(core.ErrorNotFound, core.ReasonPlanInvalid, "resolved plan current pointer is absent")
	}
	return contract.CloneCurrentResolvedPlanV1(value), nil
}

func (m *Memory) CompareAndSwapCurrentResolvedAgentPlanV1(ctx context.Context, expected *contract.CurrentResolvedPlanRefV1, next contract.CurrentResolvedPlanV1) (contract.CurrentResolvedPlanV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.CurrentResolvedPlanV1{}, err
	}
	if err := next.Validate(); err != nil {
		return contract.CurrentResolvedPlanV1{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current, exists := m.current[next.DefinitionID]
	if expected == nil {
		if exists {
			return contract.CurrentResolvedPlanV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "resolved plan current pointer already exists")
		}
		if next.Revision != 1 || next.PreviousRef != nil {
			return contract.CurrentResolvedPlanV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "initial current projection must be revision one without a predecessor")
		}
	} else {
		if err := expected.Validate(); err != nil {
			return contract.CurrentResolvedPlanV1{}, err
		}
		if !exists || current.RefV1() != *expected {
			return contract.CurrentResolvedPlanV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "resolved plan current pointer changed")
		}
		if next.Revision != current.Revision+1 || next.PreviousRef == nil || *next.PreviousRef != *expected || next.UpdatedUnixNano < current.UpdatedUnixNano {
			return contract.CurrentResolvedPlanV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "next current projection does not extend the exact current projection")
		}
	}
	stored, ok := m.plans[planKey{ID: next.PlanRef.PlanID, Revision: next.PlanRef.Revision}]
	if !ok || stored.RefV1() != next.PlanRef || stored.DefinitionRef.DefinitionID != next.DefinitionID {
		return contract.CurrentResolvedPlanV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "resolved plan current pointer target is absent or drifted")
	}
	history := m.currentPlanHistory[next.DefinitionID]
	if history == nil {
		history = map[contract.ResolvedAgentPlanRefV1]struct{}{}
		m.currentPlanHistory[next.DefinitionID] = history
	}
	if _, seen := history[next.PlanRef]; seen {
		return contract.CurrentResolvedPlanV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "resolved plan current projection cannot revisit a historical plan")
	}
	storedCurrent := contract.CloneCurrentResolvedPlanV1(next)
	m.current[next.DefinitionID] = storedCurrent
	history[next.PlanRef] = struct{}{}
	return contract.CloneCurrentResolvedPlanV1(storedCurrent), nil
}
