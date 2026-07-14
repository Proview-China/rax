package admission_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/admission"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestRecoveryPlannerResumesEveryDurableActivationStage(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 15, 0, 0, time.UTC)
	tests := []struct {
		stage admission.ActivationStage
		want  admission.RecoveryAction
	}{
		{admission.StageProposed, admission.ActionResumePreflight},
		{admission.StagePreflighting, admission.ActionInspectPreflight},
		{admission.StagePreflightPassed, admission.ActionFreezeSnapshot},
		{admission.StageSnapshotFrozen, admission.ActionReserveIdentityLease},
		{admission.StageIdentityLeaseReserved, admission.ActionResolveBudget},
		{admission.StageBudgetResolved, admission.ActionReserveSandbox},
		{admission.StageSandboxReserved, admission.ActionCommitActivation},
		{admission.StageCommitted, admission.ActionActivateSandbox},
		{admission.StageSandboxActive, admission.ActionResumeBinding},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.stage), func(t *testing.T) {
			t.Parallel()
			decision, err := admission.PlanRecovery(attemptAtStage(t, now, test.stage), now)
			if err != nil {
				t.Fatal(err)
			}
			if decision.Action != test.want {
				t.Fatalf("want %s, got %+v", test.want, decision)
			}
		})
	}
}

func TestUnknownSandboxReservationIsInspectedAndNeverRedispatched(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 16, 0, 0, time.UTC)
	attempt := attemptAtStage(t, now, admission.StageBudgetResolved)
	attempt.Recovery = admission.RecoveryQuarantined
	attempt.FailureReason = "sandbox allocation response lost"
	attempt.SandboxReservation = admission.ActivationOperation{State: admission.OperationUnknownOutcome, IntentID: "effect-sandbox-reserve"}
	decision, err := admission.PlanRecovery(attempt, now)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != admission.ActionInspectSandboxReservation || decision.AutomaticSafe {
		t.Fatalf("unknown allocation must require authoritative inspection, got %+v", decision)
	}
}

func TestCommittedActivationWithLostProviderReplyInspectsBeforeRetry(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 17, 0, 0, time.UTC)
	attempt := attemptAtStage(t, now, admission.StageCommitted)
	attempt.SandboxActivation = admission.ActivationOperation{State: admission.OperationIntentRecorded, IntentID: "effect-sandbox-activate"}
	decision, err := admission.PlanRecovery(attempt, now)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != admission.ActionInspectSandboxActivation || decision.AutomaticSafe {
		t.Fatalf("lost activation reply must inspect instead of duplicate dispatch, got %+v", decision)
	}
}

func TestRecordedSandboxIntentIsInspectedBeforeAnyRedispatch(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 17, 30, 0, time.UTC)
	attempt := attemptAtStage(t, now, admission.StageBudgetResolved)
	attempt.SandboxReservation = admission.ActivationOperation{State: admission.OperationIntentRecorded, IntentID: "effect-sandbox-reserve"}
	decision, err := admission.PlanRecovery(attempt, now)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != admission.ActionInspectSandboxReservation || decision.AutomaticSafe {
		t.Fatalf("write-ahead intent with no durable dispatch outcome must inspect, got %+v", decision)
	}
}

func TestExpiredSnapshotBeginsAbortInsteadOfAllocatingResources(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 17, 45, 0, time.UTC)
	attempt := attemptAtStage(t, now, admission.StageSnapshotFrozen)
	attempt.Snapshot.EvidenceExpiresAt = now.Add(-time.Second)
	decision, err := admission.PlanRecovery(attempt, now)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != admission.ActionBeginAbort {
		t.Fatalf("expired snapshot must not reserve identity authority, got %+v", decision)
	}
}

func TestAbortCleanupOrderPreservesFenceAndReverseRelease(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 18, 0, 0, time.UTC)
	attempt := attemptAtStage(t, now, admission.StageSandboxReserved)
	attempt.Recovery = admission.RecoveryAborting
	attempt.FailureReason = "activation commit rejected"
	attempt.Budget = appliedOperation(t, "effect-budget", "budget-1")

	assertAction(t, attempt, now, admission.ActionFenceSandbox)
	attempt.SandboxFenced = true
	assertAction(t, attempt, now, admission.ActionReleaseSandbox)
	attempt.SandboxReservation.State = admission.OperationReleased
	assertAction(t, attempt, now, admission.ActionReleaseBudget)
	attempt.Budget.State = admission.OperationReleased
	assertAction(t, attempt, now, admission.ActionRevokeIdentityLease)
	attempt.IdentityLeaseState = control.IdentityLeaseRevoked
	assertAction(t, attempt, now, admission.ActionReleaseIdentityLease)
	attempt.IdentityLeaseState = control.IdentityLeaseReleased
	assertAction(t, attempt, now, admission.ActionMarkAborted)
}

func TestQuarantineCannotClearWithoutAuthoritativeInspect(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 19, 0, 0, time.UTC)
	from := attemptAtStage(t, now, admission.StageBudgetResolved)
	from.Recovery = admission.RecoveryQuarantined
	from.FailureReason = "allocation unknown"
	from.SandboxReservation = admission.ActivationOperation{State: admission.OperationUnknownOutcome, IntentID: "effect-sandbox"}
	to := from
	to.Recovery = admission.RecoveryNormal
	to.FailureReason = ""
	to.SandboxReservation = appliedOperation(t, "effect-sandbox", "sandbox-1")
	to.Revision++
	to.UpdatedAt = to.UpdatedAt.Add(time.Second)
	if err := admission.ValidateTransition(from, to, admission.TransitionContext{}, now); !core.HasReason(err, core.ReasonInspectCoverageIncomplete) {
		t.Fatalf("quarantine cleared without inspect coverage: %v", err)
	}
	if err := admission.ValidateTransition(from, to, admission.TransitionContext{UnknownResolved: true}, now); err != nil {
		t.Fatalf("authoritative inspect should clear quarantine: %v", err)
	}
}

func assertAction(t *testing.T, attempt admission.ActivationAttempt, now time.Time, want admission.RecoveryAction) {
	t.Helper()
	decision, err := admission.PlanRecovery(attempt, now)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != want {
		t.Fatalf("want %s, got %+v", want, decision)
	}
}

func attemptAtStage(t *testing.T, now time.Time, stage admission.ActivationStage) admission.ActivationAttempt {
	t.Helper()
	plan := mustDigest(t, "plan")
	requirement := mustDigest(t, "sandbox-requirement")
	attempt := admission.ActivationAttempt{
		ID: "attempt-1",
		Scope: core.ExecutionScope{
			Identity:       core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 4},
			Lineage:        core.LineageRef{ID: "lineage-1", PlanDigest: plan},
			Instance:       core.InstanceRef{ID: "instance-1", Epoch: 7},
			AuthorityEpoch: 3,
		},
		ExpectedIdentityEpoch: 3,
		RequirementDigest:     requirement,
		Stage:                 stage,
		Recovery:              admission.RecoveryNormal,
		Budget:                admission.ActivationOperation{State: admission.OperationNotStarted},
		SandboxReservation:    admission.ActivationOperation{State: admission.OperationNotStarted},
		SandboxActivation:     admission.ActivationOperation{State: admission.OperationNotStarted},
		Revision:              1,
		CreatedAt:             now.Add(-time.Minute),
		UpdatedAt:             now.Add(-time.Minute),
	}
	if stageRank(stage) >= stageRank(admission.StageSnapshotFrozen) {
		attempt.Snapshot = activationSnapshot(t, now, requirement)
	}
	if stageRank(stage) >= stageRank(admission.StageIdentityLeaseReserved) {
		attempt.IdentityLeaseID = "identity-lease-4"
		attempt.IdentityLeaseState = control.IdentityLeaseReserved
		attempt.IdentityLeaseRevision = 1
	}
	if stageRank(stage) >= stageRank(admission.StageBudgetResolved) {
		attempt.Budget = admission.ActivationOperation{State: admission.OperationNotRequired}
	}
	if stageRank(stage) >= stageRank(admission.StageSandboxReserved) {
		attempt.SandboxReservation = appliedOperation(t, "effect-sandbox-reserve", "sandbox-1")
	}
	if stageRank(stage) >= stageRank(admission.StageCommitted) {
		lease := core.SandboxLeaseRef{ID: "sandbox-1", Epoch: 1}
		attempt.Scope.SandboxLease = &lease
		attempt.IdentityLeaseState = control.IdentityLeaseActive
		attempt.IdentityLeaseRevision = 2
	}
	if stage == admission.StageSandboxActive {
		attempt.SandboxActivation = appliedOperation(t, "effect-sandbox-activate", "sandbox-1")
	}
	if err := attempt.Validate(now); err != nil {
		t.Fatalf("invalid %s fixture: %v", stage, err)
	}
	return attempt
}

func activationSnapshot(t *testing.T, now time.Time, requirement core.Digest) *admission.ActivationSnapshot {
	t.Helper()
	return &admission.ActivationSnapshot{
		Digest: mustDigest(t, "activation-snapshot"), AuthorityEpoch: 3,
		EntitlementDigest: mustDigest(t, "entitlement"), RouteDigest: mustDigest(t, "route"),
		CapabilityEvidenceDigest: mustDigest(t, "capability"), PolicyDigest: mustDigest(t, "policy"),
		BudgetPolicyDigest: mustDigest(t, "budget-policy"), SandboxRequirementDigest: requirement,
		EvidenceExpiresAt: now.Add(time.Hour),
	}
}

func appliedOperation(t *testing.T, intent, reference string) admission.ActivationOperation {
	t.Helper()
	return admission.ActivationOperation{State: admission.OperationConfirmedApplied, IntentID: core.EffectIntentID(intent), Reference: reference, EvidenceDigest: mustDigest(t, intent+reference)}
}

func stageRank(stage admission.ActivationStage) int {
	order := []admission.ActivationStage{
		admission.StageProposed, admission.StagePreflighting, admission.StagePreflightPassed,
		admission.StageSnapshotFrozen, admission.StageIdentityLeaseReserved, admission.StageBudgetResolved,
		admission.StageSandboxReserved, admission.StageCommitted, admission.StageSandboxActive,
	}
	for index, candidate := range order {
		if candidate == stage {
			return index
		}
	}
	return -1
}

func mustDigest(t *testing.T, value string) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
