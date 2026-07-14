package conformance

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RunSettlementParticipantCaseV2 checks only the public read seam. Passing
// never grants Binding, policy, dispatch, trust, Outcome or another Fact Owner.
type RunSettlementParticipantCaseV2 struct {
	Participant ports.RunSettlementParticipantPortV2
	Request     ports.RunSettlementParticipantInspectRequestV2
	Expected    ports.RunSettlementParticipantFactV2
}

type RunSettlementParticipantReportV2 struct {
	ExactFactRead         bool `json:"exact_fact_read"`
	CanonicalReplayStable bool `json:"canonical_replay_stable"`
	BindingEligible       bool `json:"binding_eligible"`
	DispatchEligible      bool `json:"dispatch_eligible"`
	OutcomeCommitEligible bool `json:"outcome_commit_eligible"`
	ProductionEligible    bool `json:"production_eligible"`
}

func CheckRunSettlementParticipantV2(ctx context.Context, testCase RunSettlementParticipantCaseV2) (RunSettlementParticipantReportV2, error) {
	if testCase.Participant == nil {
		return RunSettlementParticipantReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "participant public Port is required")
	}
	if err := testCase.Expected.Validate(); err != nil {
		return RunSettlementParticipantReportV2{}, err
	}
	first, err := testCase.Participant.InspectRunSettlementParticipant(ctx, testCase.Request)
	if err != nil {
		return RunSettlementParticipantReportV2{}, err
	}
	second, err := testCase.Participant.InspectRunSettlementParticipant(ctx, testCase.Request)
	if err != nil {
		return RunSettlementParticipantReportV2{}, err
	}
	expectedDigest, _ := testCase.Expected.DigestV2()
	firstDigest, firstErr := first.DigestV2()
	secondDigest, secondErr := second.DigestV2()
	if firstErr != nil || secondErr != nil || firstDigest != expectedDigest || secondDigest != expectedDigest {
		return RunSettlementParticipantReportV2{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementParticipantStale, "participant did not return one exact canonical Fact")
	}
	return RunSettlementParticipantReportV2{ExactFactRead: true, CanonicalReplayStable: true, BindingEligible: false, DispatchEligible: false, OutcomeCommitEligible: false, ProductionEligible: false}, nil
}

type RunSettlementBackendCaseV2 struct {
	Facts  control.RunSettlementFactPortV2
	Bundle control.RunBundleCreateRequestV2
}
type RunSettlementBackendReportV2 struct {
	AtomicBundleVisible       bool `json:"atomic_bundle_visible"`
	IdempotentReplay          bool `json:"idempotent_replay"`
	ProductionDurabilityClaim bool `json:"production_durability_claim"`
	SLAClaim                  bool `json:"sla_claim"`
}

func CheckRunSettlementBackendV2(ctx context.Context, testCase RunSettlementBackendCaseV2) (RunSettlementBackendReportV2, error) {
	if testCase.Facts == nil {
		return RunSettlementBackendReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Run settlement Fact Port is required")
	}
	created, err := testCase.Facts.CreateRunBundleV2(ctx, testCase.Bundle)
	if err != nil {
		return RunSettlementBackendReportV2{}, err
	}
	run, runErr := testCase.Facts.InspectRun(ctx, testCase.Bundle.Run.Scope, testCase.Bundle.Run.ID)
	plan, planErr := testCase.Facts.InspectRunSettlementPlanV2(ctx, testCase.Bundle.Run.Scope, testCase.Bundle.Run.ID)
	createdPlanDigest, _ := created.Plan.DigestV2()
	inspectedPlanDigest, _ := plan.DigestV2()
	if runErr != nil || planErr != nil || run.ID != created.Run.ID || createdPlanDigest != inspectedPlanDigest {
		return RunSettlementBackendReportV2{}, core.NewError(core.ErrorInternal, core.ReasonRunCompletionAtomicityBroken, "Run and Plan bundle is not atomically inspectable")
	}
	replayed, err := testCase.Facts.CreateRunBundleV2(ctx, testCase.Bundle)
	if err != nil {
		return RunSettlementBackendReportV2{}, err
	}
	replayDigest, _ := replayed.Plan.DigestV2()
	if replayDigest != createdPlanDigest {
		return RunSettlementBackendReportV2{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "exact Run bundle replay drifted")
	}
	return RunSettlementBackendReportV2{AtomicBundleVisible: true, IdempotentReplay: true, ProductionDurabilityClaim: false, SLAClaim: false}, nil
}
