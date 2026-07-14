package fakes_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRunSettlementV2ConformanceNeverSelfGrantsProductionOrOutcome(t *testing.T) {
	t.Parallel()
	fixture := newRunSettlementGatewayFixtureV2(t, ports.RunExecutionTerminalCompleted)
	backend := fakes.NewRunSettlementStoreV2(func() time.Time { return fixture.now })
	pending := fixture.run
	pending.Status = "pending"
	pending.Revision = 1
	pending.StartedAt = time.Time{}
	backendReport, err := conformance.CheckRunSettlementBackendV2(
		context.Background(),
		conformance.RunSettlementBackendCaseV2{
			Facts: backend,
			Bundle: control.RunBundleCreateRequestV2{
				Run:  pending,
				Plan: fixture.plan,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !backendReport.AtomicBundleVisible || !backendReport.IdempotentReplay || backendReport.ProductionDurabilityClaim || backendReport.SLAClaim {
		t.Fatalf("test backend conformance overclaimed production properties: %+v", backendReport)
	}
	var requirement ports.RunSettlementRequirementV2
	for _, candidate := range fixture.plan.Requirements {
		if candidate.Kind == ports.RunRequirementDomainCommits {
			requirement = candidate
			break
		}
	}
	fact := fixture.participantsFor[requirement.ID]
	requirementDigest, _ := requirement.DigestV2()
	planRef, _ := fixture.plan.RefV2()
	request := ports.RunSettlementParticipantInspectRequestV2{
		RunID:                fixture.run.ID,
		RunIdentityDigest:    fixture.plan.RunIdentityDigest,
		ExecutionScope:       fixture.run.Scope,
		ExecutionScopeDigest: fixture.plan.ExecutionScopeDigest,
		Plan:                 planRef,
		RequirementID:        requirement.ID,
		RequirementDigest:    requirementDigest,
		SubjectDigest:        requirement.SubjectDigest,
		Owner:                requirement.Owner,
	}
	report, err := conformance.CheckRunSettlementParticipantV2(
		context.Background(),
		conformance.RunSettlementParticipantCaseV2{
			Participant: fixture.inputs,
			Request:     request,
			Expected:    fact,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.ExactFactRead || !report.CanonicalReplayStable || report.BindingEligible || report.DispatchEligible || report.OutcomeCommitEligible || report.ProductionEligible {
		t.Fatalf("custom participant conformance self-granted authority: %+v", report)
	}
}
