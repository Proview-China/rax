package repository

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ReleaseMemory struct {
	mu       sync.RWMutex
	releases map[releaseKey]contract.ComponentReleaseV1
}

type releaseKey struct {
	ID       string
	Revision core.Revision
}

func NewReleaseMemory() *ReleaseMemory {
	return &ReleaseMemory{releases: map[releaseKey]contract.ComponentReleaseV1{}}
}

func (m *ReleaseMemory) EnsureExactComponentReleaseV1(ctx context.Context, candidate contract.ComponentReleaseV1) (contract.ComponentReleaseV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ComponentReleaseV1{}, err
	}
	if err := candidate.Validate(); err != nil {
		return contract.ComponentReleaseV1{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := releaseKey{ID: candidate.ReleaseID, Revision: candidate.Revision}
	if existing, ok := m.releases[key]; ok {
		if existing.ReleaseDigest != candidate.ReleaseDigest {
			return contract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "component release identity contains different content")
		}
		return contract.CloneComponentReleaseV1(existing), nil
	}
	m.releases[key] = contract.CloneComponentReleaseV1(candidate)
	return contract.CloneComponentReleaseV1(candidate), nil
}

func (m *ReleaseMemory) InspectExactComponentReleaseV1(ctx context.Context, ref contract.ComponentReleaseRefV1) (contract.ComponentReleaseV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ComponentReleaseV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.ComponentReleaseV1{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.releases[releaseKey{ID: ref.ReleaseID, Revision: ref.Revision}]
	if !ok {
		return contract.ComponentReleaseV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "component release is absent")
	}
	if value.RefV1() != ref {
		return contract.ComponentReleaseV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "component release exact ref drifted")
	}
	return contract.CloneComponentReleaseV1(value), nil
}
