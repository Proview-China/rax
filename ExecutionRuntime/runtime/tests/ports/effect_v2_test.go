package ports_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestEffectIntentV2RequiresExplicitGovernanceFactsAndSupportsCustomKind(t *testing.T) {
	t.Parallel()
	now := time.Unix(23_000, 0)
	intent, _, _, _, _ := effectPortFixtureV2(t, now)
	if err := intent.Validate(); err != nil {
		t.Fatalf("namespaced custom effect kind must remain extensible: %v", err)
	}
	withoutReview := intent
	withoutReview.Review = ports.ReviewBindingRefV2{}
	if err := withoutReview.Validate(); !core.HasReason(err, core.ReasonReviewVerdictMissing) {
		t.Fatalf("empty review ref must not mean not-required: %v", err)
	}
	withoutBudget := intent
	withoutBudget.Budget = ports.BudgetBindingRefV2{}
	if err := withoutBudget.Validate(); !core.HasReason(err, core.ReasonBudgetBindingMissing) {
		t.Fatalf("empty budget ref must not mean not-required: %v", err)
	}
	related := intent
	related.ID = "effect-related"
	related.Relation = ports.EffectRelationV2{CompensatesEffectID: intent.ID, CompensatesEffectRevision: intent.Revision, InspectsEffectID: intent.ID, InspectsEffectRevision: intent.Revision}
	if err := related.Validate(); !core.HasReason(err, core.ReasonInvalidReference) {
		t.Fatalf("one effect cannot ambiguously be both compensation and inspection: %v", err)
	}
}

func TestPermitVerifierRequiresBegunFactAndExactCurrentFence(t *testing.T) {
	t.Parallel()
	now := time.Unix(24_000, 0)
	intent, permit, fence, current, _ := effectPortFixtureV2(t, now)
	issued := ports.PermitVerificationRequestV2{Permit: permit, PermitFactRevision: 1, PermitFactState: "issued", Intent: intent, Fence: fence, Current: current}
	if err := issued.Validate(now); !core.HasReason(err, core.ReasonDispatchPermitConsumed) {
		t.Fatalf("execution point must reject an unconsumed bearer permit: %v", err)
	}
	begun := issued
	begun.PermitFactRevision = 2
	begun.PermitFactState = "begun"
	if err := begun.Validate(now); err != nil {
		t.Fatalf("exact begun permit and current facts should verify: %v", err)
	}
	drifted := begun
	drifted.Current.Budget.Revision++
	if err := drifted.Validate(now); !core.HasReason(err, core.ReasonDispatchPermitInvalid) {
		t.Fatalf("current budget revision drift must fail at actual execution point: %v", err)
	}
	if err := begun.Validate(time.Unix(0, permit.ExpiresUnixNano)); !core.HasReason(err, core.ReasonDispatchPermitExpired) {
		t.Fatalf("permit must fail closed at its exact TTL boundary: %v", err)
	}
	staleFence := begun
	staleFence.Fence.Scope.AuthorityEpoch++
	if err := staleFence.Validate(now); !core.HasReason(err, core.ReasonEffectFenceStale) {
		t.Fatalf("stale authority epoch in the fence must fail closed: %v", err)
	}
}

func effectPortFixtureV2(t *testing.T, now time.Time) (ports.EffectIntentV2, ports.DispatchPermitV2, core.ExecutionFence, ports.DispatchCurrentFactsV2, control.BindingSetFactV2) {
	t.Helper()
	payload := []byte("payload")
	manifest := portEffectDigestV2(t, "manifest")
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-1", PlanDigest: portEffectDigestV2(t, "plan")}, Instance: core.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox-1", Epoch: 1}, AuthorityEpoch: 1}
	stableScope := ports.StableTenantScopeDigestV2(scope.Identity.TenantID)
	intent := ports.EffectIntentV2{ContractVersion: ports.EffectContractVersionV2, ID: "effect-1", Revision: 1, Scope: scope, RunID: "run-1", Kind: "custom.vendor/send", RiskClass: "praxis/high", ActionScopeDigest: portEffectDigestV2(t, "action"), Payload: ports.OpaquePayloadV2{Schema: ports.SchemaRefV2{Namespace: "custom.vendor", Name: "send", Version: "1.0.0", MediaType: "application/octet-stream", ContentDigest: portEffectDigestV2(t, "schema")}, ContentDigest: core.DigestBytes(payload), Length: uint64(len(payload)), Inline: payload, LimitPolicy: ports.OpaqueLimitPolicyRefV2{Policy: "praxis/default-limit", Digest: portEffectDigestV2(t, "limit")}}, PayloadRevision: 1, Target: "remote://target", ConflictDomain: ports.ConflictDomainBindingV2{Domain: "remote/target", ScopeClass: ports.EffectStableScopeTenantV2, ScopeDigest: stableScope}, Owners: []ports.EffectOwnerRefV2{{Role: ports.OwnerCleanup, ComponentID: "custom.vendor/provider", ManifestDigest: manifest}, {Role: ports.OwnerEffect, ComponentID: "custom.vendor/provider", ManifestDigest: manifest}, {Role: ports.OwnerSettlement, ComponentID: "custom.vendor/provider", ManifestDigest: manifest}}, Provider: ports.ProviderBindingRefV2{BindingSetID: "set-1", BindingSetRevision: 7, ComponentID: "custom.vendor/provider", ManifestDigest: manifest, ArtifactDigest: portEffectDigestV2(t, "artifact"), Capability: "custom.vendor/send"}, Authority: ports.AuthorityBindingRefV2{Ref: "authority-1", Digest: portEffectDigestV2(t, "authority"), Revision: 3, Epoch: 1}, Review: ports.ReviewBindingRefV2{Ref: "review-1", Digest: portEffectDigestV2(t, "review"), Revision: 5, PolicyDigest: portEffectDigestV2(t, "review-policy")}, Budget: ports.BudgetBindingRefV2{Ref: "budget-1", Digest: portEffectDigestV2(t, "budget"), Revision: 2, PolicyDigest: portEffectDigestV2(t, "budget-policy")}, Policy: ports.DispatchPolicyBindingRefV2{Ref: "dispatch-policy-1", Digest: portEffectDigestV2(t, "dispatch-policy"), Revision: 4}, CurrentScope: ports.ExecutionScopeBindingRefV2{Ref: "scope-current-1", Digest: portEffectDigestV2(t, "scope-current"), Revision: 3}, Idempotency: ports.IdempotencyBindingV2{Key: "idem-1", ScopeClass: ports.EffectStableScopeTenantV2, ScopeDigest: stableScope, Class: core.IdempotencyQueryable}, CredentialLeases: []ports.CredentialLeaseRefV2{}, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	grant := ports.CapabilityGrantV2{Capability: intent.Provider.Capability, EvidenceDigest: portEffectDigestV2(t, "grant-evidence"), ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	set := control.BindingSetFactV2{ID: intent.Provider.BindingSetID, PlanID: "plan-1", PlanDigest: portEffectDigestV2(t, "binding-plan"), GovernanceDigest: portEffectDigestV2(t, "binding-governance"), State: control.BindingSetActive, Revision: intent.Provider.BindingSetRevision, Members: []control.BindingMemberV2{{BindingID: "binding-provider", BindingRevision: 1, ComponentID: intent.Provider.ComponentID, Kind: "custom.vendor/provider-kind", ManifestDigest: intent.Provider.ManifestDigest, ArtifactDigest: intent.Provider.ArtifactDigest, Contract: ports.ContractBindingV2{Name: "custom.vendor/provider-contract", Version: "2.0.0", Compatible: ports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}}, Owners: []ports.OwnerAssignmentV2{{Role: ports.OwnerEffect, OwnerComponentID: intent.Provider.ComponentID}, {Role: ports.OwnerSettlement, OwnerComponentID: intent.Provider.ComponentID}, {Role: ports.OwnerCleanup, OwnerComponentID: intent.Provider.ComponentID}}, Grants: []ports.CapabilityGrantV2{grant}}}, TopologicalOrder: []ports.ComponentIDV2{intent.Provider.ComponentID}, Residuals: []control.BindingResidualV2{}, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	grantDigest, err := set.CapabilityGrantDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	scopeFact := portCurrentScopeFactV2(t, intent, grantDigest, now)
	intent.CurrentScope, _ = scopeFact.BindingRefV2()
	budget := portBudgetFactV2(t, intent, now)
	intent.Budget, _ = budget.BindingRefV2()
	policy := portPolicyFactV2(t, intent, now)
	intent.Policy = ports.DispatchPolicyBindingRefV2{Ref: policy.Ref, Digest: policy.Digest, Revision: policy.Revision}
	intentDigest, err := intent.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	fence := core.ExecutionFence{BoundaryScope: core.FenceBoundaryInstance, Scope: scope, CapabilityGrantDigest: grantDigest, EffectIntentID: intent.ID, EffectIntentRevision: intent.Revision, CanonicalPayloadDigest: intent.Payload.ContentDigest, ExpiresAt: now.Add(20 * time.Second)}
	fenceDigest, err := ports.DigestExecutionFenceV2(fence)
	if err != nil {
		t.Fatal(err)
	}
	credentialDigest, _ := ports.DigestCredentialLeaseFactsV2(nil)
	reviewVerdictDigest := portEffectDigestV2(t, "review-verdict")
	permit := ports.DispatchPermitV2{ContractVersion: ports.EffectContractVersionV2, ID: "permit-1", Revision: 1, AttemptID: "attempt-1", IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest, PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision, Scope: scope, RunID: intent.RunID, ConflictDomain: intent.ConflictDomain, Provider: intent.Provider, EnforcementPoint: intent.Provider, Authority: intent.Authority, Review: intent.Review, ReviewVerdictDigest: reviewVerdictDigest, ReviewVerdictRevision: 1, Budget: intent.Budget, Policy: intent.Policy, CurrentScope: intent.CurrentScope, CapabilityGrantDigest: grantDigest, CredentialGrantDigest: credentialDigest, FenceDigest: fenceDigest, Idempotency: intent.Idempotency, IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}
	current := ports.DispatchCurrentFactsV2{Scope: scope, CapabilityGrantDigest: grantDigest, CredentialGrantDigest: credentialDigest, Provider: intent.Provider, EnforcementPoint: intent.Provider, Authority: intent.Authority, Review: intent.Review, ReviewVerdictDigest: reviewVerdictDigest, ReviewVerdictRevision: 1, Budget: intent.Budget, Policy: intent.Policy, CurrentScope: intent.CurrentScope, FenceDigest: fenceDigest}
	return intent, permit, fence, current, set
}

func portBudgetFactV2(t *testing.T, intent ports.EffectIntentV2, now time.Time) control.BudgetBindingFactV2 {
	t.Helper()
	return control.BudgetBindingFactV2{Ref: intent.Budget.Ref, IntentID: intent.ID, IntentRevision: intent.Revision, Scope: intent.Scope, Mode: control.BudgetOperationNotRequired, PolicyDigest: intent.Budget.PolicyDigest, PolicyDecisionRef: "budget-policy-decision-1", PolicyEvidenceDigest: portEffectDigestV2(t, "budget-policy-evidence"), State: control.BudgetFactActive, Revision: intent.Budget.Revision, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
}

func portPolicyFactV2(t *testing.T, intent ports.EffectIntentV2, now time.Time) ports.DispatchPolicyFactV2 {
	t.Helper()
	candidateDigest, err := intent.PolicyCandidateDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	fact := ports.DispatchPolicyFactV2{Ref: intent.Policy.Ref, Revision: intent.Policy.Revision, IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: candidateDigest, Scope: intent.Scope, EffectKind: intent.Kind, RiskClass: intent.RiskClass, ActionScopeDigest: intent.ActionScopeDigest, MaximumPermitTTL: 20 * time.Second, Active: true, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	fact.Digest, err = fact.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func portCurrentScopeFactV2(t *testing.T, intent ports.EffectIntentV2, grantDigest core.Digest, now time.Time) ports.ExecutionScopeCurrentFactV2 {
	t.Helper()
	source := func(name string) ports.GovernanceSourceFactRefV2 {
		return ports.GovernanceSourceFactRefV2{Ref: name, Revision: 1, Digest: portEffectDigestV2(t, name)}
	}
	sandbox := source("sandbox-source")
	fact := ports.ExecutionScopeCurrentFactV2{Ref: "scope-current-1", Revision: 3, Scope: intent.Scope, CapabilityGrantDigest: grantDigest, ActivationSource: source("activation-source"), InstanceSource: source("instance-source"), SandboxSource: &sandbox, AuthoritySource: source("authority-source"), BindingSource: source("binding-source"), RunSource: source("run-source"), ActiveRunID: intent.RunID, RunState: "running", ProjectionWatermark: 1, State: ports.ExecutionScopeFactActive, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	fact.Digest, _ = fact.DigestV2()
	return fact
}

func portEffectDigestV2(t *testing.T, value any) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
