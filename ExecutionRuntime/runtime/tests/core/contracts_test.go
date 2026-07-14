package core_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestDigestJSONIsDeterministic(t *testing.T) {
	t.Parallel()
	left, err := core.DigestJSON(map[string]any{"b": 2, "a": 1})
	if err != nil {
		t.Fatal(err)
	}
	right, err := core.DigestJSON(map[string]any{"a": 1, "b": 2})
	if err != nil {
		t.Fatal(err)
	}
	if left != right {
		t.Fatalf("digest drift: %q != %q", left, right)
	}
	if err := left.Validate(); err != nil {
		t.Fatalf("generated digest must validate: %v", err)
	}
}

func TestStateRejectsFencedRunning(t *testing.T) {
	t.Parallel()
	state := core.InstanceState{
		Phase: core.PhaseRunning, Certainty: core.CertaintyFenced,
		Cleanup: core.CleanupPending, HasCleanupObligations: true,
	}
	assertReason(t, state.Validate(), core.ReasonInvalidState)
}

func TestPreflightCannotRetainUnknownState(t *testing.T) {
	t.Parallel()
	state := core.InstanceState{
		Phase: core.PhasePreflighting, Certainty: core.CertaintyUnknown,
		Cleanup: core.CleanupIndeterminate, HasCleanupObligations: true,
	}
	assertReason(t, state.Validate(), core.ReasonInvalidState)
}

func TestTerminalWithPendingCleanupIsValid(t *testing.T) {
	t.Parallel()
	state := core.InstanceState{
		Phase: core.PhaseTerminal, Certainty: core.CertaintyLost,
		Cleanup: core.CleanupPending, HasCleanupObligations: true,
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("terminal and pending cleanup must remain representable: %v", err)
	}
}

func TestCleanupCompleteRequiresIndependentEvidence(t *testing.T) {
	t.Parallel()
	state := core.InstanceState{
		Phase: core.PhaseTerminal, Certainty: core.CertaintyConfirmed,
		Cleanup: core.CleanupComplete, HasCleanupObligations: true,
	}
	assertReason(t, state.Validate(), core.ReasonCleanupEvidenceIncomplete)
}

func TestTerminalLifecycleCannotRevive(t *testing.T) {
	t.Parallel()
	from := core.InstanceState{Phase: core.PhaseTerminal, Certainty: core.CertaintyLost, Cleanup: core.CleanupPending, HasCleanupObligations: true}
	to := core.InstanceState{Phase: core.PhaseRunning, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true}
	assertReason(t, core.ValidateStateTransition(from, to, core.TransitionContext{InspectCoverageComplete: true}), core.ReasonTerminalInstance)
}

func TestUnknownCertaintyConvergesOnlyWithCompleteCoverage(t *testing.T) {
	t.Parallel()
	from := core.InstanceState{Phase: core.PhaseStopping, Certainty: core.CertaintyUnknown, Cleanup: core.CleanupPending, HasCleanupObligations: true}
	to := core.InstanceState{Phase: core.PhaseStopping, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true}
	assertReason(t, core.ValidateStateTransition(from, to, core.TransitionContext{}), core.ReasonInspectCoverageIncomplete)
	if err := core.ValidateStateTransition(from, to, core.TransitionContext{InspectCoverageComplete: true}); err != nil {
		t.Fatalf("complete inspect coverage should allow certainty convergence: %v", err)
	}
}

func TestStaleInstanceEpochIsRejected(t *testing.T) {
	t.Parallel()
	facts := currentFacts(t, true)
	leaseEpoch := core.Epoch(2)
	expected := core.ExecutionPreconditions{
		IdentityEpoch: 4, InstanceEpoch: 8, LeaseEpoch: &leaseEpoch,
		AuthorityEpoch: 3, Revision: 9,
	}
	assertReason(t, core.CheckExecutionPreconditions(expected, facts), core.ReasonStaleInstanceEpoch)
}

func TestActivationFenceDoesNotRequireFutureSandboxLease(t *testing.T) {
	t.Parallel()
	intent, fence, current := validEffectFixture(t, false)
	if err := core.ValidateEffectDispatch(intent, fence, current, time.Now()); err != nil {
		t.Fatalf("activation effect must be fenceable before sandbox allocation: %v", err)
	}
}

func TestInstanceFenceRequiresSandboxLease(t *testing.T) {
	t.Parallel()
	_, fence, _ := validEffectFixture(t, false)
	fence.BoundaryScope = core.FenceBoundaryInstance
	assertReason(t, fence.Validate(), core.ReasonInvalidReference)
}

func TestEffectRequiresWriteAheadIntent(t *testing.T) {
	t.Parallel()
	intent, _, _ := validEffectFixture(t, true)
	intent.PersistedAt = time.Time{}
	assertReason(t, intent.Validate(), core.ReasonEvidenceUnavailable)
}

func TestRecoveryCompensationCannotGuessUnknownEffect(t *testing.T) {
	t.Parallel()
	request := core.RecoveryEffectRequest{
		Kind: core.RecoveryCompensation, HasFreshIntent: true, HasCurrentFence: true,
		HasCurrentAuthority: true, HasApplicableBudget: true, HasWriteAheadEvidence: true,
		CompensationAuthorized: true,
	}
	assertReason(t, core.ValidateRecoveryEffect(core.CertaintyUnknown, request), core.ReasonRecoveryEffectNotPermitted)
	request.OriginalEffectConfirmed = true
	if err := core.ValidateRecoveryEffect(core.CertaintyUnknown, request); err != nil {
		t.Fatalf("independently confirmed and authorized compensation should be admitted: %v", err)
	}
}

func TestExternalRecoveryInspectRequiresFreshGuards(t *testing.T) {
	t.Parallel()
	request := core.RecoveryEffectRequest{Kind: core.RecoveryInspect, ExternalEffect: true}
	assertReason(t, core.ValidateRecoveryEffect(core.CertaintyLost, request), core.ReasonRecoveryEffectNotPermitted)
	request.HasFreshIntent = true
	request.HasCurrentFence = true
	request.HasCurrentAuthority = true
	request.HasApplicableBudget = true
	request.HasWriteAheadEvidence = true
	if err := core.ValidateRecoveryEffect(core.CertaintyLost, request); err != nil {
		t.Fatalf("external inspect with fresh guards should be permitted: %v", err)
	}
}

func TestOfflineEffectRequiresExplicitRevocationBounds(t *testing.T) {
	t.Parallel()
	policy := core.RevocationPolicy{
		RiskClass: "external_write", ValidationMode: core.ValidationLeasedOffline,
		ConflictEffectDomain: "workspace:one",
	}
	assertReason(t, policy.Validate(), core.ReasonOfflineRevocationPolicyMissing)
}

func validEffectFixture(t *testing.T, withLease bool) (core.EffectIntent, core.ExecutionFence, core.CurrentFenceFacts) {
	t.Helper()
	scope := currentFacts(t, withLease).Scope
	payloadDigest := digest(t, map[string]string{"action": "allocate"})
	capabilityDigest := digest(t, map[string]string{"capability": "sandbox.allocate"})
	intent := core.EffectIntent{
		ID: "effect-1", Revision: 1, Kind: core.EffectKindResourceLifecycle,
		RiskClass: "resource", CanonicalPayloadDigest: payloadDigest,
		Target: "sandbox-provider", ConflictEffectDomain: "instance:7",
		Ownership: core.EffectOwnership{
			IntentOwner:     core.OwnerRef{Domain: "runtime-intent", ID: "intent-store"},
			SettlementOwner: core.OwnerRef{Domain: "sandbox", ID: "sandbox-provider"},
		},
		AuthorizationRef: "authority:3", IdempotencyClass: core.IdempotencyQueryable,
		PersistedAt: time.Now().Add(-time.Second),
	}
	boundary := core.FenceBoundaryActivation
	if withLease {
		boundary = core.FenceBoundaryInstance
	}
	fence := core.ExecutionFence{
		BoundaryScope: boundary, Scope: scope, CapabilityGrantDigest: capabilityDigest,
		EffectIntentID: intent.ID, EffectIntentRevision: intent.Revision,
		CanonicalPayloadDigest: payloadDigest, ExpiresAt: time.Now().Add(time.Minute),
	}
	return intent, fence, core.CurrentFenceFacts{Scope: scope, CapabilityGrantDigest: capabilityDigest}
}

func currentFacts(t *testing.T, withLease bool) core.CurrentExecutionFacts {
	t.Helper()
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 4},
		Lineage:  core.LineageRef{ID: "lineage-1", PlanDigest: digest(t, "plan")},
		Instance: core.InstanceRef{ID: "instance-1", Epoch: 7}, AuthorityEpoch: 3,
	}
	if withLease {
		scope.SandboxLease = &core.SandboxLeaseRef{ID: "lease-1", Epoch: 2}
	}
	return core.CurrentExecutionFacts{Scope: scope, Revision: 9}
}

func digest(t *testing.T, value any) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func assertReason(t *testing.T, err error, reason core.ReasonCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s error", reason)
	}
	if !core.HasReason(err, reason) {
		t.Fatalf("expected reason %s, got %v", reason, err)
	}
}
