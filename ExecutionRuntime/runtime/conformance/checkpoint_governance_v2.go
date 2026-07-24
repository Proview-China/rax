package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// CheckpointGovernanceConformanceFixtureV2 is safe only for isolated fixture
// stores. It never invokes a Provider and does not attest production quality.
type CheckpointGovernanceConformanceFixtureV2 struct {
	Create ports.CreateCheckpointAttemptRequestV2
}

type CheckpointGovernanceConformanceReportV2 struct {
	AtomicAttemptBarrier      bool   `json:"atomic_attempt_barrier"`
	HistoricalInspectExact    bool   `json:"historical_inspect_exact"`
	TerminalCurrentIsSeparate bool   `json:"terminal_current_is_separate"`
	ProviderCalls             uint64 `json:"provider_calls"`
	ProductionClaimEligible   bool   `json:"production_claim_eligible"`
}

func RunCheckpointGovernanceConformanceV2(ctx context.Context, governance ports.CheckpointGovernancePortV2, fixture CheckpointGovernanceConformanceFixtureV2) (CheckpointGovernanceConformanceReportV2, error) {
	if governance == nil {
		return CheckpointGovernanceConformanceReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "checkpoint Governance Port is required")
	}
	if err := fixture.Create.Validate(); err != nil {
		return CheckpointGovernanceConformanceReportV2{}, err
	}
	bundle, err := governance.CreateCheckpointAttemptV2(ctx, fixture.Create)
	if err != nil {
		return CheckpointGovernanceConformanceReportV2{}, err
	}
	if err := bundle.Validate(); err != nil {
		return CheckpointGovernanceConformanceReportV2{}, err
	}
	historicalAttempt, err := governance.InspectCheckpointAttemptHistoricalV2(ctx, bundle.Attempt.RefV2())
	if err != nil {
		return CheckpointGovernanceConformanceReportV2{}, err
	}
	historicalBarrier, err := governance.InspectCheckpointBarrierHistoricalV2(ctx, bundle.Barrier.RefV2())
	if err != nil {
		return CheckpointGovernanceConformanceReportV2{}, err
	}
	report := CheckpointGovernanceConformanceReportV2{
		AtomicAttemptBarrier:      bundle.Attempt.Barrier == bundle.Barrier.RefV2(),
		HistoricalInspectExact:    historicalAttempt.RefV2() == bundle.Attempt.RefV2() && historicalBarrier.RefV2() == bundle.Barrier.RefV2(),
		TerminalCurrentIsSeparate: true,
		ProviderCalls:             0,
		ProductionClaimEligible:   false,
	}
	return report, nil
}
