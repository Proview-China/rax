package applicationadapter

import (
	"context"
	"errors"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
)

// CheckpointDurableStoreV2 is the exact Sandbox-owned State Plane slice used
// by the checkpoint production composition. Runtime and Provider facts are
// never copied into it; only exact association refs are retained.
type CheckpointDurableStoreV2 interface {
	sandboxports.CheckpointPhaseStore
	sandboxports.CheckpointPhaseResultStoreV2
	CheckpointProviderResultBindingStoreV2
	CreateCheckpointParticipant(context.Context, contract.CheckpointParticipantFact) error
}

type CheckpointProductionConfigV2 struct {
	Store               CheckpointDurableStoreV2
	RuntimeReservations runtimeports.CheckpointParticipantPhaseReservationCurrentReaderV2
	RuntimeEnforcement  runtimeports.CheckpointRestoreDispatchEnforcementGovernancePortV1
	RuntimeEvidence     runtimeports.CheckpointRestoreEvidenceGovernancePortV1
	EvidenceLedger      runtimeports.EvidenceGovernancePortV2
	RuntimeSettlements  runtimeports.OperationCheckpointRestoreSettlementGovernancePortV5
	DataPlane           DataPlanePortV1
	Clock               func() time.Time
	DomainResultMaxTTL  time.Duration
}

// CheckpointProductionCompositionV2 closes the Sandbox-owned actual-point,
// DomainResult-current, phase-result-current, Runtime Settlement-current, and
// ApplySettlement pieces. Runtime still owns Admission/Review/Permit/Begin,
// Enforcement, Evidence, and Settlement; the host supplies those gateways.
type CheckpointProductionCompositionV2 struct {
	Store                 CheckpointDurableStoreV2
	Controller            *kernel.CheckpointController
	ProviderBoundary      *CheckpointProviderBoundaryV1
	ActualPoint           *CheckpointActualPointOwnerV2
	DomainResultOwner     *kernel.CheckpointPhaseResultOwnerV2
	DomainResultCurrent   *runtimeadapter.CheckpointDomainResultCurrentAdapterV2
	PhaseResultCurrent    *runtimeadapter.CheckpointPhaseResultCurrentAdapterV2
	RuntimeSettlementRead *runtimeadapter.CheckpointSettlementCurrentAdapterV2
	RuntimeReservations   runtimeports.CheckpointParticipantPhaseReservationCurrentReaderV2
	RuntimeSettlements    runtimeports.OperationCheckpointRestoreSettlementGovernancePortV5
	Clock                 func() time.Time
}

func NewCheckpointProductionCompositionV2(config CheckpointProductionConfigV2) (*CheckpointProductionCompositionV2, error) {
	if nilLike(config.Store) || nilLike(config.RuntimeReservations) || nilLike(config.RuntimeEnforcement) || nilLike(config.RuntimeEvidence) || nilLike(config.EvidenceLedger) || nilLike(config.RuntimeSettlements) || nilLike(config.DataPlane) || nilLike(config.Clock) || config.DomainResultMaxTTL <= 0 {
		return nil, errors.New("checkpoint production composition requires durable Sandbox stores and all exact Runtime actual-point gateways")
	}
	boundary, err := NewCheckpointProviderBoundaryV1(config.RuntimeEnforcement, config.RuntimeEvidence, config.EvidenceLedger, config.DataPlane, config.Clock)
	if err != nil {
		return nil, err
	}
	actualPoint, err := NewCheckpointActualPointOwnerV2(boundary, config.Store)
	if err != nil {
		return nil, err
	}
	controller, err := kernel.NewCheckpointController(config.Store, config.Clock)
	if err != nil {
		return nil, err
	}
	settlementCurrent, err := runtimeadapter.NewCheckpointSettlementCurrentAdapterV2(config.Store, config.Store, config.RuntimeReservations, config.RuntimeSettlements, config.Clock)
	if err != nil {
		return nil, err
	}
	owner, err := kernel.NewCheckpointPhaseResultOwnerV2(config.Store, config.Store, actualPoint, settlementCurrent, config.Clock, config.DomainResultMaxTTL)
	if err != nil {
		return nil, err
	}
	domainCurrent, err := runtimeadapter.NewCheckpointDomainResultCurrentAdapterV2(config.Store, config.Store, config.RuntimeReservations, config.Clock)
	if err != nil {
		return nil, err
	}
	phaseCurrent, err := runtimeadapter.NewCheckpointPhaseResultCurrentAdapterV2(config.Store, config.Store, config.RuntimeReservations, config.Clock)
	if err != nil {
		return nil, err
	}
	return &CheckpointProductionCompositionV2{Store: config.Store, Controller: controller, ProviderBoundary: boundary, ActualPoint: actualPoint, DomainResultOwner: owner, DomainResultCurrent: domainCurrent, PhaseResultCurrent: phaseCurrent, RuntimeSettlementRead: settlementCurrent, RuntimeReservations: config.RuntimeReservations, RuntimeSettlements: config.RuntimeSettlements, Clock: config.Clock}, nil
}
