package domain

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

type RetentionManager struct {
	store ports.RetentionStore
	clock Clock
}

func NewRetentionManager(store ports.RetentionStore, clock Clock) (*RetentionManager, error) {
	if nilInterfaceV1(store) || nilInterfaceV1(clock) {
		return nil, contract.NewError(contract.ErrInvalidArgument, "retention_manager", "store and clock are required")
	}
	return &RetentionManager{store: store, clock: clock}, nil
}

func (m *RetentionManager) Create(ctx context.Context, objectID, policyRef, classification string) (contract.RetentionFact, error) {
	fact := contract.RetentionFact{
		ObjectID: objectID, PolicyRef: policyRef, Classification: classification,
		State: contract.RetentionActive, Revision: 1, UpdatedUnixNano: m.clock.Now().UnixNano(),
	}
	if err := fact.Validate(); err != nil {
		return contract.RetentionFact{}, err
	}
	if createErr := m.store.CreateRetention(ctx, fact); createErr != nil {
		inspected, inspectErr := m.store.InspectRetention(context.WithoutCancel(ctx), objectID)
		if inspectErr != nil {
			return contract.RetentionFact{}, createErr
		}
		if inspected != fact {
			return contract.RetentionFact{}, contract.NewError(contract.ErrRevisionConflict, "retention", "retention create recovery found different content")
		}
		return inspected, nil
	}
	return fact, nil
}

func (m *RetentionManager) Transition(ctx context.Context, objectID string, next contract.RetentionState, evidenceRef string) (contract.RetentionFact, error) {
	current, err := m.store.InspectRetention(ctx, objectID)
	if err != nil {
		return contract.RetentionFact{}, err
	}
	updated, err := contract.AdvanceRetention(current, next, evidenceRef)
	if err != nil {
		return contract.RetentionFact{}, err
	}
	updated.UpdatedUnixNano = m.clock.Now().UnixNano()
	if casErr := m.store.CASRetention(ctx, current.Revision, updated); casErr != nil {
		inspected, inspectErr := m.store.InspectRetention(context.WithoutCancel(ctx), objectID)
		if inspectErr != nil {
			return contract.RetentionFact{}, casErr
		}
		if inspected != updated {
			return contract.RetentionFact{}, contract.NewError(contract.ErrRevisionConflict, "retention", "retention transition recovery found another revision")
		}
		return inspected, nil
	}
	return updated, nil
}

func (m *RetentionManager) Inspect(ctx context.Context, objectID string) (contract.RetentionFact, error) {
	return m.store.InspectRetention(ctx, objectID)
}

func (m *RetentionManager) PhysicalPurge(context.Context, string) error {
	return contract.NewError(contract.ErrUnsupported, "physical_purge", "real purge is outside Wave 1 and requires the governed Runtime operation chain")
}
