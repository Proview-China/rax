package applicationadapter

import (
	"strings"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
)

func TestLifecyclePlanV4AcceptsEveryIndependentImplementedEffect(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for _, kind := range []contract.EffectKind{
		contract.EffectAllocate, contract.EffectActivate, contract.EffectOpen,
		contract.EffectCancel, contract.EffectClose, contract.EffectFence,
		contract.EffectRelease, contract.EffectCleanup,
	} {
		t.Run(string(kind), func(t *testing.T) {
			plan, reservation := lifecyclePlanValidationFixtureV4(t, now, kind)
			if err := validateLifecyclePlanV4(plan, reservation); err != nil {
				t.Fatalf("implemented independent effect rejected: %v", err)
			}
		})
	}
}

func TestLifecyclePlanV4KeepsWorkspaceCommitOnDedicatedClosure(t *testing.T) {
	plan, reservation := lifecyclePlanValidationFixtureV4(t, time.Unix(1_800_000_000, 0), contract.EffectWorkspaceCommit)
	if err := validateLifecyclePlanV4(plan, reservation); err == nil || !strings.Contains(err.Error(), "dedicated governed commit closure") {
		t.Fatalf("workspace commit entered generic lifecycle: %v", err)
	}
}

func lifecyclePlanValidationFixtureV4(t *testing.T, now time.Time, kind contract.EffectKind) (LifecyclePlanV4, contract.DomainReservation) {
	t.Helper()
	prepare := providerPhasePlanFixtureV4(t, now)
	reservation := testkit.Reservation(kind, 1, "lifecycle-plan")
	reservation.EffectID = string(prepare.Enforcement.EffectID)
	reservation.AttemptID = prepare.Enforcement.AttemptID
	reservation.IntentRevision = uint64(prepare.Attempt.IntentRevision)
	reservation.IntentDigest = strings.TrimPrefix(string(prepare.Attempt.IntentDigest), "sha256:")
	reservation.Kind = kind
	reservation.ConflictDomain = kind.ConflictDomain()
	if kind == contract.EffectCancel || kind == contract.EffectWorkspaceCommit {
		reservation.RunID = "run-1"
	} else {
		reservation.RunID = ""
	}
	prepare.EffectKind = string(kind)
	prepare.Evidence.Scope.EffectKind = runtimeports.EffectKindV2(kind)
	operation := prepare.Enforcement.Operation
	operation.ActivationAttemptID = ""
	switch kind {
	case contract.EffectAllocate, contract.EffectActivate, contract.EffectOpen:
		operation.Kind = runtimeports.OperationScopeActivationV3
		operation.ActivationAttemptID = "activation-1"
	case contract.EffectCancel, contract.EffectWorkspaceCommit:
		operation.Kind = runtimeports.OperationScopeRunV3
		operation.RunID = "run-1"
	case contract.EffectClose, contract.EffectRelease, contract.EffectFence, contract.EffectCleanup:
		operation.Kind = runtimeports.OperationScopeTerminationV3
		operation.TerminationAttemptID = "termination-1"
	}
	prepare.Enforcement.Operation = operation
	prepare.Evidence.Scope.Operation = operation
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	prepare.Evidence.Scope.OperationDigest = operationDigest

	execute := prepare
	execute.RequestID = "provider-request-execute"
	execute.Enforcement.Phase = runtimeports.OperationDispatchEnforcementExecuteV4
	execute.Evidence.QualificationID = "qualification-execute"
	execute.Evidence.HandoffID = "handoff-execute"
	execute.Evidence.ConsumptionID = "consumption-execute"
	execute.Evidence.Reservation.EventID = "event-execute"
	execute.Evidence.Reservation.Source.SourceSequence++

	digest := func(value string) runtimecore.Digest { return runtimecore.DigestBytes([]byte(value)) }
	return LifecyclePlanV4{
		ReservationID: reservation.Meta.ID,
		Prepare:       prepare,
		Execute:       execute,
		DeclaredDelegation: runtimeports.ExecutionDelegationRefV2{
			ID: "delegation-1", Revision: 1, Digest: digest("delegation"),
		},
		ResultID: "result-lifecycle-plan",
		Settlement: LifecycleSettlementPlanV4{
			ID: "settlement-1",
			Owner: runtimeports.EffectOwnerRefV2{
				Role: runtimeports.OwnerSettlement, ComponentID: "praxis.sandbox/controller", ManifestDigest: digest("manifest"),
			},
			ExpectedEffectRevision: 1, ExpectedTerminalGuardRevision: 1,
			IdempotencyKey: "settlement-key", ConflictDomain: digest("conflict-domain"),
		},
		Inspection: GovernedInspectionPlanV4{
			ReservationID: "inspect-reservation", FinalInspectionID: "inspect-fact",
			Prepare: ProviderPhasePlanV4{EffectKind: string(contract.EffectInspect)},
			Execute: ProviderPhasePlanV4{EffectKind: string(contract.EffectInspect)},
		},
	}, reservation
}
