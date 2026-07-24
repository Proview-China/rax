package repository

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type Snapshots struct {
	mu       sync.RWMutex
	facts    map[factsKey]contract.ResolutionFactsSnapshotV1
	catalogs map[catalogKey]contract.ComponentReleaseCatalogSnapshotV1
}

type factsKey struct {
	ID       string
	Revision core.Revision
}

type catalogKey struct {
	ID       string
	Revision core.Revision
}

func NewSnapshots() *Snapshots {
	return &Snapshots{facts: map[factsKey]contract.ResolutionFactsSnapshotV1{}, catalogs: map[catalogKey]contract.ComponentReleaseCatalogSnapshotV1{}}
}

func (s *Snapshots) PutFacts(value contract.ResolutionFactsSnapshotV1) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := factsKey{ID: value.FactsID, Revision: value.Revision}
	if existing, ok := s.facts[key]; ok && existing.Digest != value.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "resolution facts id is occupied")
	}
	s.facts[key] = contract.CloneResolutionFactsV1(value)
	return nil
}

func (s *Snapshots) PutCatalog(value contract.ComponentReleaseCatalogSnapshotV1) error {
	if err := value.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := catalogKey{ID: value.CatalogID, Revision: value.Revision}
	if existing, ok := s.catalogs[key]; ok && existing.Digest != value.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "component catalog id is occupied")
	}
	s.catalogs[key] = contract.CloneCatalogV1(value)
	return nil
}

func (s *Snapshots) InspectExactResolutionFactsV1(ctx context.Context, ref contract.ResolutionFactsRefV1) (contract.ResolutionFactsSnapshotV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ResolutionFactsSnapshotV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.facts[factsKey{ID: ref.FactsID, Revision: ref.Revision}]
	if !ok {
		return contract.ResolutionFactsSnapshotV1{}, core.NewError(core.ErrorNotFound, core.ReasonPlanInvalid, "resolution facts are absent")
	}
	if value.RefV1() != ref {
		return contract.ResolutionFactsSnapshotV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "resolution facts exact ref drifted")
	}
	return contract.CloneResolutionFactsV1(value), nil
}

func (s *Snapshots) InspectExactComponentReleaseCatalogV1(ctx context.Context, ref contract.ComponentReleaseCatalogRefV1) (contract.ComponentReleaseCatalogSnapshotV1, error) {
	if err := ctx.Err(); err != nil {
		return contract.ComponentReleaseCatalogSnapshotV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.catalogs[catalogKey{ID: ref.CatalogID, Revision: ref.Revision}]
	if !ok {
		return contract.ComponentReleaseCatalogSnapshotV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "component release catalog is absent")
	}
	if value.RefV1() != ref {
		return contract.ComponentReleaseCatalogSnapshotV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "component release catalog exact ref drifted")
	}
	return contract.CloneCatalogV1(value), nil
}
