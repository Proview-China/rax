package applicationadapter

import (
	"errors"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
)

// WorkspaceRestoreDurableStoreV1 is the Sandbox State Plane surface required
// by the production Restore composition. Implementations must be durable and
// create-once/CAS safe; the SQLite Store is the bundled implementation.
type WorkspaceRestoreDurableStoreV1 interface {
	sandboxports.WorkspaceRestoreStoreV1
	sandboxports.WorkspaceRestoreSettlementStoreV1
	runtimeadapter.WorkspaceRestorePreparedRuntimeBindingStoreV1
	runtimeadapter.WorkspaceRestoreStageRuntimeBindingStoreV1
}

// WorkspaceRestoreProductionConfigV1 accepts only public Runtime readers and
// Sandbox-owned ports. Runtime remains the sole owner of Restore Attempt,
// eligibility, fresh Instance/Lease/Fence, governance, and Settlement facts.
type WorkspaceRestoreProductionConfigV1 struct {
	Store                WorkspaceRestoreDurableStoreV1
	Bundles              sandboxports.WorkspaceRestoreBundleCurrentReaderV1
	Coordinates          runtimeadapter.RestoreStageCoordinateReaderV1
	RuntimeCurrent       runtimeports.RestoreStageGovernanceCurrentPortV1
	RuntimeSettlements   runtimeadapter.WorkspaceRestoreRuntimeSettlementReaderV1
	Provider             sandboxports.WorkspaceRestoreProviderV1
	DomainResultOwner    runtimeports.ProviderBindingRefV2
	DomainResultSchema   runtimeports.SchemaRefV2
	ApplySettlementOwner runtimeports.ProviderBindingRefV2
	Clock                func() time.Time
	Limits               kernel.WorkspaceRestoreOwnerLimitsV1
}

// WorkspaceRestoreProductionCompositionV1 exposes the Sandbox-owned Restore
// surfaces. The host registers PreparedCurrent and DomainResultCurrent with
// Runtime and invokes Settlement only with an exact Runtime Settlement ref.
// No Runtime fact is written directly by this composition.
type WorkspaceRestoreProductionCompositionV1 struct {
	Restore                sandboxports.WorkspaceRestoreOwnerPortV1
	PreparedCurrent        *runtimeadapter.WorkspaceRestorePreparedCurrentAdapterV1
	DomainResultCurrent    *runtimeadapter.WorkspaceRestoreStageDomainResultAdapterV1
	Settlement             *runtimeadapter.WorkspaceRestoreSettlementAdapterV1
	SettlementOwner        sandboxports.WorkspaceRestoreSettlementOwnerPortV1
	ApplySettlementCurrent *runtimeadapter.WorkspaceRestoreApplySettlementCurrentAdapterV1
}

func NewWorkspaceRestoreProductionCompositionV1(config WorkspaceRestoreProductionConfigV1) (*WorkspaceRestoreProductionCompositionV1, error) {
	if nilLike(config.Store) || nilLike(config.Bundles) || nilLike(config.Coordinates) ||
		nilLike(config.RuntimeCurrent) || nilLike(config.RuntimeSettlements) || nilLike(config.Provider) || nilLike(config.Clock) {
		return nil, errors.New("production workspace Restore requires durable Sandbox stores, exact Runtime readers, Provider, and clock")
	}
	if config.DomainResultOwner.Validate() != nil || config.DomainResultSchema.Validate() != nil || config.ApplySettlementOwner.Validate() != nil {
		return nil, errors.New("production workspace Restore requires exact DomainResult owner and schema refs")
	}

	governance, err := runtimeadapter.NewWorkspaceRestoreGovernanceReaderV1(config.Coordinates, config.RuntimeCurrent, config.Clock)
	if err != nil {
		return nil, err
	}
	restore, err := kernel.NewWorkspaceRestoreOwnerV1(config.Store, config.Bundles, governance, config.Provider, config.Clock, config.Limits)
	if err != nil {
		return nil, err
	}
	prepared, err := runtimeadapter.NewWorkspaceRestorePreparedCurrentAdapterV1(config.Store, config.Store, config.Clock)
	if err != nil {
		return nil, err
	}
	domain, err := runtimeadapter.NewWorkspaceRestoreStageDomainResultAdapterV1(config.Store, config.Store, config.RuntimeCurrent, config.DomainResultOwner, config.DomainResultSchema, config.Clock)
	if err != nil {
		return nil, err
	}
	settlementOwner, err := kernel.NewWorkspaceRestoreSettlementOwnerV1(config.Store, config.Store, config.Clock)
	if err != nil {
		return nil, err
	}
	settlement, err := runtimeadapter.NewWorkspaceRestoreSettlementAdapterV1(config.RuntimeSettlements, settlementOwner)
	if err != nil {
		return nil, err
	}
	applyCurrent, err := runtimeadapter.NewWorkspaceRestoreApplySettlementCurrentAdapterV1(settlement, settlementOwner, config.ApplySettlementOwner, config.Clock)
	if err != nil {
		return nil, err
	}

	return &WorkspaceRestoreProductionCompositionV1{
		Restore:                restore,
		PreparedCurrent:        prepared,
		DomainResultCurrent:    domain,
		Settlement:             settlement,
		SettlementOwner:        settlementOwner,
		ApplySettlementCurrent: applyCurrent,
	}, nil
}
