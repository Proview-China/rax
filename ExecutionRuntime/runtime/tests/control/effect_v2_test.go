package control_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestEffectTransitionSeparatesPreDispatchRejectionFromPostBeginUnknown(t *testing.T) {
	t.Parallel()
	now := time.Unix(25_000, 0)
	proposed := controlEffectFactV2(t, now, false, false)
	accepted := proposed
	accepted.State, accepted.Revision = control.EffectAccepted, proposed.Revision+1
	if err := control.ValidateEffectFactTransitionV2(proposed, accepted, control.EffectTransitionContextV2{}, now); err != nil {
		t.Fatal(err)
	}
	intent := accepted
	intent.State, intent.Revision = control.EffectDispatchIntent, accepted.Revision+1
	intent.DispatchPermitID = "permit-1"
	intent.DispatchPermitDigest = controlEffectDigestV2(t, "permit")
	if err := control.ValidateEffectFactTransitionV2(accepted, intent, control.EffectTransitionContextV2{}, now); err != nil {
		t.Fatal(err)
	}
	preDispatchRejected := intent
	preDispatchRejected.State, preDispatchRejected.Revision = control.EffectRejected, intent.Revision+1
	preDispatchRejected.RejectionReason = core.ReasonEffectAuthorizationMissing
	if err := control.ValidateEffectFactTransitionV2(intent, preDispatchRejected, control.EffectTransitionContextV2{PermitBegun: false}, now); err != nil {
		t.Fatalf("pre-dispatch rejection before provider reach must be terminally safe: %v", err)
	}
	if err := control.ValidateEffectFactTransitionV2(intent, preDispatchRejected, control.EffectTransitionContextV2{PermitBegun: true}, now); !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("after Begin, rejection must become unknown/inspect rather than safe retry: %v", err)
	}
	unknown := intent
	unknown.State, unknown.Revision = control.EffectUnknownOutcome, intent.Revision+1
	if err := control.ValidateEffectFactTransitionV2(intent, unknown, control.EffectTransitionContextV2{PermitBegun: true}, now); err != nil {
		t.Fatalf("post-begin crash must admit unknown_outcome: %v", err)
	}
}

func TestEffectClosureAndCompensationRequireIndependentSettledEffects(t *testing.T) {
	t.Parallel()
	now := time.Unix(26_000, 0)
	unknown := controlEffectFactV2(t, now, true, false)
	unknown.State, unknown.Revision = control.EffectUnknownOutcome, 4
	unknown.DispatchPermitID = "permit-1"
	unknown.DispatchPermitDigest = controlEffectDigestV2(t, "permit")
	settled := unknown
	settled.State, settled.Revision = control.EffectSettled, unknown.Revision+1
	settled.Settlement = &control.EffectSettlementFactV2{Owner: unknown.Intent.Owners[2], Disposition: control.SettlementConfirmedApplied, ReceiptRef: "settlement-1", EvidenceDigest: controlEffectDigestV2(t, "settlement-evidence"), InspectionIntentID: "inspect-1", InspectionIntentRevision: 1, InspectionSettlementDigest: controlEffectDigestV2(t, "inspect-settlement"), SettledUnixNano: now.UnixNano()}
	settled.RemoteResidual = control.RemoteResidualConfirmedAbsent
	settled.ResidualResolution = &control.EffectResolutionCompletionV2{EffectID: "inspect-1", EffectRevision: 1, SettlementDigest: controlEffectDigestV2(t, "inspect-settlement")}
	if err := control.ValidateEffectFactTransitionV2(unknown, settled, control.EffectTransitionContextV2{SettlementOwnerMatched: true, UnknownInspectionSettled: true}, now); !core.HasReason(err, core.ReasonRemoteResidualUnresolved) {
		t.Fatalf("residual conclusion without independently settled inspect effect must fail: %v", err)
	}
	if err := control.ValidateEffectFactTransitionV2(unknown, settled, control.EffectTransitionContextV2{SettlementOwnerMatched: true, UnknownInspectionSettled: true, ResidualInspectSettled: true}, now); err != nil {
		t.Fatalf("settlement may close residual only with independently settled inspect effect: %v", err)
	}
	compensated := settled
	compensated.State, compensated.Revision = control.EffectCompensated, settled.Revision+1
	compensated.Compensation = &control.CompensationCompletionV2{EffectID: "compensate-1", EffectRevision: 1, SettlementDigest: controlEffectDigestV2(t, "compensation-settlement")}
	if err := control.ValidateEffectFactTransitionV2(settled, compensated, control.EffectTransitionContextV2{}, now); !core.HasReason(err, core.ReasonCompensationIncomplete) {
		t.Fatalf("original effect cannot jump directly to compensated: %v", err)
	}
	if err := control.ValidateEffectFactTransitionV2(settled, compensated, control.EffectTransitionContextV2{CompensationSettled: true}, now); err != nil {
		t.Fatalf("independent settled compensation effect may close original: %v", err)
	}
}

func TestBudgetBindingRequiresExplicitPolicyAndHasExactExpiryBoundary(t *testing.T) {
	t.Parallel()
	now := time.Unix(27_000, 0)
	intent := controlEffectFactV2(t, now, false, false).Intent
	fact := control.BudgetBindingFactV2{Ref: "budget-1", IntentID: intent.ID, IntentRevision: intent.Revision, Scope: intent.Scope, Mode: control.BudgetOperationNotRequired, PolicyDigest: controlEffectDigestV2(t, "policy"), PolicyDecisionRef: "policy-fact-1", PolicyEvidenceDigest: controlEffectDigestV2(t, "policy-evidence"), State: control.BudgetFactActive, Revision: 1, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	if err := fact.Validate(); err != nil {
		t.Fatal(err)
	}
	missingPolicy := fact
	missingPolicy.PolicyDecisionRef = ""
	if err := missingPolicy.Validate(); !core.HasReason(err, core.ReasonBudgetBindingMissing) {
		t.Fatalf("operation_not_required must cite an explicit policy fact: %v", err)
	}
	ref, err := fact.BindingRefV2()
	if err != nil {
		t.Fatal(err)
	}
	if err := fact.ValidateCurrent(ref, intent, time.Unix(0, fact.ExpiresUnixNano)); !core.HasReason(err, core.ReasonBudgetBindingStale) {
		t.Fatalf("budget binding must be stale at exact TTL boundary: %v", err)
	}
	consumed := fact
	consumed.State, consumed.Revision = control.BudgetFactConsumed, fact.Revision+1
	if err := control.ValidateBudgetFactTransitionV2(fact, consumed, now); !core.HasReason(err, core.ReasonBudgetBindingStale) {
		t.Fatalf("not-required policy cannot be silently converted into consumption: %v", err)
	}
}

func TestEffectSettlementResidualCleanupAndCompensationAdvanceOrthogonally(t *testing.T) {
	t.Parallel()
	now := time.Unix(27_500, 0)
	settled := controlEffectFactV2(t, now, true, true)
	settled.State = control.EffectSettled
	settled.Revision = 5
	settled.DispatchPermitID = "permit-1"
	settled.DispatchPermitDigest = controlEffectDigestV2(t, "permit")
	settled.Settlement = &control.EffectSettlementFactV2{Owner: settled.Intent.Owners[2], Disposition: control.SettlementConfirmedApplied, ReceiptRef: "settlement-1", EvidenceDigest: controlEffectDigestV2(t, "settlement"), SettledUnixNano: now.UnixNano()}
	if err := settled.Validate(); err != nil {
		t.Fatal(err)
	}
	residualClosed := settled
	residualClosed.Revision++
	residualClosed.RemoteResidual = control.RemoteResidualConfirmedAbsent
	residualClosed.ResidualResolution = &control.EffectResolutionCompletionV2{EffectID: "inspect-1", EffectRevision: 1, SettlementDigest: controlEffectDigestV2(t, "inspect-settlement")}
	if err := control.ValidateEffectFactTransitionV2(settled, residualClosed, control.EffectTransitionContextV2{ResidualInspectSettled: true}, now); err != nil {
		t.Fatalf("residual must be closable after the main settlement: %v", err)
	}
	cleanupClosed := residualClosed
	cleanupClosed.Revision++
	cleanupClosed.Cleanup = control.EffectCleanupComplete
	cleanupClosed.CleanupResolution = &control.EffectResolutionCompletionV2{EffectID: "cleanup-1", EffectRevision: 1, SettlementDigest: controlEffectDigestV2(t, "cleanup-settlement")}
	if err := control.ValidateEffectFactTransitionV2(residualClosed, cleanupClosed, control.EffectTransitionContextV2{CleanupEffectSettled: true}, now); err != nil {
		t.Fatalf("cleanup must be closable after main settlement and residual: %v", err)
	}
	compensatedPendingCleanup := residualClosed
	compensatedPendingCleanup.State = control.EffectCompensated
	compensatedPendingCleanup.Revision++
	compensatedPendingCleanup.Compensation = &control.CompensationCompletionV2{EffectID: "compensation-1", EffectRevision: 1, SettlementDigest: controlEffectDigestV2(t, "compensation-settlement")}
	if err := control.ValidateEffectFactTransitionV2(residualClosed, compensatedPendingCleanup, control.EffectTransitionContextV2{CompensationSettled: true}, now); err != nil {
		t.Fatalf("compensation must not require cleanup to finish first: %v", err)
	}
	compensatedCleanupClosed := compensatedPendingCleanup
	compensatedCleanupClosed.Revision++
	compensatedCleanupClosed.Cleanup = control.EffectCleanupComplete
	compensatedCleanupClosed.CleanupResolution = &control.EffectResolutionCompletionV2{EffectID: "cleanup-after-compensation", EffectRevision: 1, SettlementDigest: controlEffectDigestV2(t, "cleanup-after-compensation")}
	if err := control.ValidateEffectFactTransitionV2(compensatedPendingCleanup, compensatedCleanupClosed, control.EffectTransitionContextV2{CleanupEffectSettled: true}, now); err != nil {
		t.Fatalf("cleanup must remain independently closable after compensation: %v", err)
	}
	conflictingResolution := residualClosed
	conflictingResolution.Revision++
	conflictingResolution.ResidualResolution = &control.EffectResolutionCompletionV2{EffectID: "inspect-other", EffectRevision: 1, SettlementDigest: controlEffectDigestV2(t, "other")}
	if err := control.ValidateEffectFactTransitionV2(residualClosed, conflictingResolution, control.EffectTransitionContextV2{ResidualInspectSettled: true}, now); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("a different resolution cannot replace the first linearized one: %v", err)
	}
}

func controlEffectFactV2(t *testing.T, now time.Time, residual, cleanup bool) control.EffectFactV2 {
	t.Helper()
	payload := []byte("payload")
	manifest := controlEffectDigestV2(t, "manifest")
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-1", PlanDigest: controlEffectDigestV2(t, "plan")}, Instance: core.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox-1", Epoch: 1}, AuthorityEpoch: 1}
	stableScope := ports.StableTenantScopeDigestV2(scope.Identity.TenantID)
	intent := ports.EffectIntentV2{ContractVersion: ports.EffectContractVersionV2, ID: "effect-1", Revision: 1, Scope: scope, RunID: "run-1", Kind: "vendor/execute", RiskClass: "vendor/high", ActionScopeDigest: controlEffectDigestV2(t, "action"), Payload: ports.OpaquePayloadV2{Schema: ports.SchemaRefV2{Namespace: "vendor", Name: "effect", Version: "1.0.0", MediaType: "application/octet-stream", ContentDigest: controlEffectDigestV2(t, "schema")}, ContentDigest: core.DigestBytes(payload), Length: uint64(len(payload)), Inline: payload, LimitPolicy: ports.OpaqueLimitPolicyRefV2{Policy: "vendor/limit", Digest: controlEffectDigestV2(t, "limit")}}, PayloadRevision: 1, Target: "provider://target", ConflictDomain: ports.ConflictDomainBindingV2{Domain: "domain/one", ScopeClass: ports.EffectStableScopeTenantV2, ScopeDigest: stableScope}, Owners: []ports.EffectOwnerRefV2{{Role: ports.OwnerCleanup, ComponentID: "vendor/provider", ManifestDigest: manifest}, {Role: ports.OwnerEffect, ComponentID: "vendor/provider", ManifestDigest: manifest}, {Role: ports.OwnerSettlement, ComponentID: "vendor/provider", ManifestDigest: manifest}}, Provider: ports.ProviderBindingRefV2{BindingSetID: "set-1", BindingSetRevision: 1, ComponentID: "vendor/provider", ManifestDigest: manifest, ArtifactDigest: controlEffectDigestV2(t, "artifact"), Capability: "vendor/execute"}, Authority: ports.AuthorityBindingRefV2{Ref: "authority-1", Digest: controlEffectDigestV2(t, "authority"), Revision: 1, Epoch: 1}, Review: ports.ReviewBindingRefV2{Ref: "review-1", Digest: controlEffectDigestV2(t, "review"), Revision: 1, PolicyDigest: controlEffectDigestV2(t, "review-policy")}, Budget: ports.BudgetBindingRefV2{Ref: "budget-1", Digest: controlEffectDigestV2(t, "budget"), Revision: 1, PolicyDigest: controlEffectDigestV2(t, "budget-policy")}, Policy: ports.DispatchPolicyBindingRefV2{Ref: "dispatch-policy-1", Digest: controlEffectDigestV2(t, "dispatch-policy"), Revision: 1}, CurrentScope: ports.ExecutionScopeBindingRefV2{Ref: "scope-current-1", Digest: controlEffectDigestV2(t, "scope-current"), Revision: 1}, Idempotency: ports.IdempotencyBindingV2{Key: "idem-1", ScopeClass: ports.EffectStableScopeTenantV2, ScopeDigest: stableScope, Class: core.IdempotencyQueryable}, CredentialLeases: []ports.CredentialLeaseRefV2{}, MayLeaveRemoteResidual: residual, RequiresCleanup: cleanup, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	fact, err := control.NewProposedEffectFactV2(intent, now)
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func controlEffectDigestV2(t *testing.T, value any) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
