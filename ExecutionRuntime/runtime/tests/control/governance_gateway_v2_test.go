package control_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestGovernanceGatewayV2RevalidatesCurrentFactsAndDerivesOwnersFromBindingSet(t *testing.T) {
	t.Parallel()
	now := time.Unix(28_000, 0)
	gateway, effect := gatewayFixtureV2(t, now, false)
	result, err := gateway.Issue(context.Background(), control.IssueGovernedDispatchRequestV2{EffectID: effect.Intent.ID, ExpectedEffectRevision: effect.Revision, PermitID: "permit-gateway", AttemptID: "attempt-gateway", PermitTTL: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if result.Effect.State != control.EffectDispatchIntent || result.Permit.State != control.DispatchPermitIssued || result.Permit.Permit.Provider != effect.Intent.Provider || result.Permit.Permit.Budget != effect.Intent.Budget || result.Permit.Permit.Review != effect.Intent.Review {
		t.Fatalf("gateway must atomically bind one current effect/binding/review/budget/provider attempt: %+v", result)
	}
	if result.Permit.Permit.ExpiresUnixNano != now.Add(4*time.Second).UnixNano() {
		t.Fatalf("permit TTL must be capped by the shortest credential lease: got %d", result.Permit.Permit.ExpiresUnixNano)
	}

	wrongOwnerGateway, wrongOwnerEffect := gatewayFixtureV2(t, now, true)
	if _, err := wrongOwnerGateway.Issue(context.Background(), control.IssueGovernedDispatchRequestV2{EffectID: wrongOwnerEffect.Intent.ID, ExpectedEffectRevision: wrongOwnerEffect.Revision, PermitID: "permit-wrong-owner", AttemptID: "attempt-wrong-owner", PermitTTL: 5 * time.Second}); !core.HasReason(err, core.ReasonProviderBindingStale) {
		t.Fatalf("caller-provided owner refs must not override current binding assignments: %v", err)
	}
}

func TestGovernanceGatewayV2UsesAtomicallyIndexedPartitionedRunEffect(t *testing.T) {
	t.Parallel()
	now := time.Unix(28_500, 0)
	gateway, legacyAccepted := gatewayFixtureV2(t, now, false)
	store := gateway.Effects.(*fakes.EffectStoreV2)
	runs := fakes.NewRunSettlementStoreV2(func() time.Time { return now })
	run := core.AgentRunRecord{ID: legacyAccepted.Intent.RunID, Scope: legacyAccepted.Intent.Scope, Status: core.RunRunning, Revision: 1, SessionRef: "runtime-session-partitioned", StartedAt: now.Add(-time.Second)}
	if _, err := runs.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	store.SetRunFacts(runs)
	runIdentity, _ := ports.RunIdentityDigestV2(run)
	scopeDigest, _ := ports.ExecutionScopeDigestV2(run.Scope)
	partition := control.RunEffectPartitionV2{ExecutionScope: run.Scope, ExecutionScopeDigest: scopeDigest, RunID: run.ID, RunIdentityDigest: runIdentity}
	index := control.RunEffectIndexFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: "effect-index-partitioned", Revision: 1, RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScope: run.Scope, ExecutionScopeDigest: scopeDigest, State: control.RunEffectIndexOpen, HeadSegmentDigest: ports.EvidenceGenesisDigestV2, Watermark: 1, CreatedUnixNano: now.UnixNano()}
	if _, err := store.CreateRunEffectIndexV2(context.Background(), index); err != nil {
		t.Fatal(err)
	}
	proposed, err := control.NewProposedEffectFactV2(legacyAccepted.Intent, now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateEffectForRunV2(context.Background(), control.CreateRunEffectRequestV2{Partition: partition, ExpectedIndexRevision: 1, Effect: proposed})
	if err != nil {
		t.Fatal(err)
	}
	accepted := created.Effect
	accepted.State, accepted.Revision = control.EffectAccepted, accepted.Revision+1
	accepted, err = store.CompareAndSwapRunEffectV2(context.Background(), partition, control.EffectFactCASRequestV2{ExpectedRevision: proposed.Revision, Next: accepted})
	if err != nil {
		t.Fatal(err)
	}
	gateway.Effects = control.RunEffectGovernanceAdapterV2{Partition: partition, Facts: store}
	issued, err := gateway.Issue(context.Background(), control.IssueGovernedDispatchRequestV2{EffectID: accepted.Intent.ID, ExpectedEffectRevision: accepted.Revision, PermitID: "permit-partitioned", AttemptID: "attempt-partitioned", PermitTTL: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	replayedIssue, err := store.IssueRunDispatchPermitV2(
		context.Background(),
		partition,
		control.IssueDispatchPermitRequestV2{
			EffectID:               accepted.Intent.ID,
			ExpectedEffectRevision: accepted.Revision,
			Permit:                 issued.Permit.Permit,
			Fence:                  issued.Permit.Fence,
		},
	)
	if err != nil || replayedIssue.Permit.PermitDigest != issued.Permit.PermitDigest || replayedIssue.Effect.Revision != issued.Effect.Revision {
		t.Fatalf("lost partitioned Issue reply did not recover by exact idempotent Inspect semantics: %+v %v", replayedIssue, err)
	}
	forgedPermit := issued.Permit.Permit
	forgedPermit.ExpiresUnixNano++
	if _, err := store.IssueRunDispatchPermitV2(
		context.Background(),
		partition,
		control.IssueDispatchPermitRequestV2{
			EffectID:               accepted.Intent.ID,
			ExpectedEffectRevision: accepted.Revision,
			Permit:                 forgedPermit,
			Fence:                  issued.Permit.Fence,
		},
	); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same partitioned Permit identity accepted different TTL/content: %v", err)
	}
	begun, err := gateway.Begin(context.Background(), control.BeginGovernedDispatchRequestV2{EffectID: accepted.Intent.ID, ExpectedEffectRevision: issued.Effect.Revision, PermitID: issued.Permit.Permit.ID, ExpectedPermitRevision: issued.Permit.Revision})
	if err != nil {
		t.Fatal(err)
	}
	receipt := ports.EnforcementReceiptV2{ContractVersion: ports.EffectContractVersionV2, PermitID: begun.Permit.ID, PermitRevision: begun.Permit.Revision, AttemptID: begun.Permit.AttemptID, PermitDigest: begun.PermitDigest, Verifier: begun.Permit.EnforcementPoint, ValidatedAt: now.UnixNano()}
	recorded, err := store.RecordRunEnforcementReceiptV2(context.Background(), partition, control.RecordEnforcementReceiptRequestV2{PermitID: begun.Permit.ID, ExpectedPermitRevision: begun.Revision, Receipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	dispatched := issued.Effect
	dispatched.State = control.EffectDispatched
	dispatched.Revision++
	dispatched.UpdatedUnixNano = now.UnixNano()
	dispatched.DispatchReceipt = &control.ProviderDispatchReceiptV2{PermitID: begun.Permit.ID, PermitDigest: begun.PermitDigest, AttemptID: begun.Permit.AttemptID, IntentID: begun.Permit.IntentID, IntentRevision: begun.Permit.IntentRevision, Provider: begun.Permit.Provider, ProviderOperationRef: "operation-partitioned", ReceiptRef: "receipt-partitioned", ObservationDigest: controlEffectDigestV2(t, "partitioned-provider-receipt"), ObservedUnixNano: now.UnixNano()}
	if recorded.Enforcement == nil {
		t.Fatal("execution-point enforcement was not persisted")
	}
	dispatched, err = store.CompareAndSwapRunEffectV2(context.Background(), partition, control.EffectFactCASRequestV2{ExpectedRevision: issued.Effect.Revision, Next: dispatched})
	if err != nil {
		t.Fatal(err)
	}
	settled := dispatched
	settled.State = control.EffectSettled
	settled.Revision++
	settled.UpdatedUnixNano = now.UnixNano()
	settled.Settlement = &control.EffectSettlementFactV2{Owner: settled.Intent.Owners[2], Disposition: control.SettlementConfirmedApplied, ReceiptRef: "settlement-partitioned", EvidenceDigest: controlEffectDigestV2(t, "partitioned-settlement"), SettledUnixNano: now.UnixNano()}
	settled, err = store.CompareAndSwapRunEffectV2(context.Background(), partition, control.EffectFactCASRequestV2{ExpectedRevision: dispatched.Revision, Next: settled})
	if err != nil || settled.State != control.EffectSettled {
		t.Fatalf("partitioned governed Effect did not settle: %+v %v", settled, err)
	}
	if _, err := store.InspectEffect(context.Background(), accepted.Intent.ID); err != nil { // legacy fixture remains separate; assert adapter returns indexed fact instead.
		t.Fatal(err)
	}
	indexed, err := gateway.Effects.InspectEffect(context.Background(), accepted.Intent.ID)
	if err != nil || indexed.State != control.EffectSettled {
		t.Fatalf("governed adapter did not remain partition-scoped: %+v %v", indexed, err)
	}
}

func TestGovernanceGatewayV2RejectsCurrentFactDrift(t *testing.T) {
	t.Parallel()
	now := time.Unix(29_000, 0)
	gateway, effect := gatewayFixtureV2(t, now, false)
	review := gateway.Review.(staticReviewReaderV2)
	review.fact.Revision++
	gateway.Review = review
	if _, err := gateway.Issue(context.Background(), control.IssueGovernedDispatchRequestV2{EffectID: effect.Intent.ID, ExpectedEffectRevision: effect.Revision, PermitID: "permit-stale-review", AttemptID: "attempt-stale-review", PermitTTL: 5 * time.Second}); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("review revision drift must fail closed immediately before dispatch: %v", err)
	}

	gateway, effect = gatewayFixtureV2(t, now, false)
	review = gateway.Review.(staticReviewReaderV2)
	review.fact.PolicyDecisionRef = ""
	gateway.Review = review
	if _, err := gateway.Issue(context.Background(), control.IssueGovernedDispatchRequestV2{EffectID: effect.Intent.ID, ExpectedEffectRevision: effect.Revision, PermitID: "permit-review-without-policy", AttemptID: "attempt-review-without-policy", PermitTTL: 5 * time.Second}); !core.HasReason(err, core.ReasonReviewVerdictMissing) {
		t.Fatalf("operation_not_required review must cite an explicit policy fact: %v", err)
	}

	gateway, effect = gatewayFixtureV2(t, now, false)
	if _, err := gateway.Issue(context.Background(), control.IssueGovernedDispatchRequestV2{EffectID: effect.Intent.ID, ExpectedEffectRevision: effect.Revision, PermitID: "permit-policy-ttl", AttemptID: "attempt-policy-ttl", PermitTTL: 11 * time.Second}); !core.HasReason(err, core.ReasonEffectAuthorizationMissing) {
		t.Fatalf("caller cannot exceed the current dispatch policy fact TTL: %v", err)
	}

	gateway, effect = gatewayFixtureV2(t, now, false)
	current := gateway.CurrentScopes.(staticScopeReaderV2)
	current.fact.Scope.Instance.Epoch++
	gateway.CurrentScopes = current
	if _, err := gateway.Issue(context.Background(), control.IssueGovernedDispatchRequestV2{EffectID: effect.Intent.ID, ExpectedEffectRevision: effect.Revision, PermitID: "permit-stale-scope", AttemptID: "attempt-stale-scope", PermitTTL: 5 * time.Second}); !core.HasReason(err, core.ReasonEffectFenceStale) {
		t.Fatalf("current instance epoch drift must fail closed: %v", err)
	}

	gateway, effect = gatewayFixtureV2(t, now, false)
	credentials := gateway.Credentials.(staticCredentialReaderV2)
	credentials.fact.ExpiresUnixNano = now.UnixNano()
	gateway.Credentials = credentials
	if _, err := gateway.Issue(context.Background(), control.IssueGovernedDispatchRequestV2{EffectID: effect.Intent.ID, ExpectedEffectRevision: effect.Revision, PermitID: "permit-expired-credential", AttemptID: "attempt-expired-credential", PermitTTL: 5 * time.Second}); !core.HasReason(err, core.ReasonCredentialLeaseMissing) {
		t.Fatalf("credential lease must be inactive at its exact expiry boundary: %v", err)
	}
}

func TestGovernanceGatewayV2BeginRevalidatesAllCurrentAuthority(t *testing.T) {
	t.Parallel()
	now := time.Unix(29_500, 0)
	gateway, effect := gatewayFixtureV2(t, now, false)
	issued, err := gateway.Issue(context.Background(), control.IssueGovernedDispatchRequestV2{EffectID: effect.Intent.ID, ExpectedEffectRevision: effect.Revision, PermitID: "permit-begin", AttemptID: "attempt-begin", PermitTTL: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	begun, err := gateway.Begin(context.Background(), control.BeginGovernedDispatchRequestV2{EffectID: effect.Intent.ID, ExpectedEffectRevision: issued.Effect.Revision, PermitID: issued.Permit.Permit.ID, ExpectedPermitRevision: issued.Permit.Revision})
	if err != nil || begun.State != control.DispatchPermitBegun {
		t.Fatalf("all-current governed begin should linearize once: %v %+v", err, begun)
	}

	for _, testCase := range []struct {
		name   string
		mutate func(*control.GovernanceDispatchGatewayV2)
		reason core.ReasonCode
	}{
		{name: "review_revoked", reason: core.ReasonReviewVerdictStale, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.Review.(staticReviewReaderV2)
			reader.fact.Decision = ports.ReviewDecisionRevoked
			g.Review = reader
		}},
		{name: "identity_revoked", reason: core.ReasonStaleIdentityEpoch, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.IdentityLeases.(staticIdentityPortV2)
			reader.lease.State = control.IdentityLeaseRevoked
			g.IdentityLeases = reader
		}},
		{name: "sandbox_replaced", reason: core.ReasonEffectFenceStale, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.CurrentScopes.(staticScopeReaderV2)
			lease := *reader.fact.Scope.SandboxLease
			lease.Epoch++
			reader.fact.Scope.SandboxLease = &lease
			g.CurrentScopes = reader
		}},
		{name: "run_stopping", reason: core.ReasonEffectFenceStale, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.CurrentScopes.(staticScopeReaderV2)
			reader.fact.RunState = "stopping"
			g.CurrentScopes = reader
		}},
		{name: "policy_revoked", reason: core.ReasonEffectAuthorizationMissing, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.Policies.(staticPolicyReaderV2)
			reader.fact.Active = false
			g.Policies = reader
		}},
		{name: "binding_revision", reason: core.ReasonProviderBindingStale, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.Bindings.(staticBindingPortV2)
			reader.set.Revision++
			g.Bindings = reader
		}},
		{name: "binding_revoked", reason: core.ReasonProviderBindingStale, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.Bindings.(staticBindingPortV2)
			reader.set.State = control.BindingSetRevoked
			g.Bindings = reader
		}},
		{name: "binding_exact_expiry", reason: core.ReasonProviderBindingStale, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.Bindings.(staticBindingPortV2)
			reader.set.ExpiresUnixNano = now.UnixNano()
			g.Bindings = reader
		}},
		{name: "authority_revoked", reason: core.ReasonEffectAuthorizationMissing, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.Authority.(staticAuthorityReaderV2)
			reader.fact.State = ports.AuthorityFactRevoked
			g.Authority = reader
		}},
		{name: "authority_epoch", reason: core.ReasonEffectAuthorizationMissing, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.Authority.(staticAuthorityReaderV2)
			reader.fact.Scope.AuthorityEpoch++
			g.Authority = reader
		}},
		{name: "authority_revision", reason: core.ReasonEffectAuthorizationMissing, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.Authority.(staticAuthorityReaderV2)
			reader.fact.Revision++
			g.Authority = reader
		}},
		{name: "budget_consumed", reason: core.ReasonBudgetBindingStale, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.Budgets.(staticBudgetPortV2)
			reader.fact.State = control.BudgetFactConsumed
			g.Budgets = reader
		}},
		{name: "budget_revoked", reason: core.ReasonBudgetBindingStale, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.Budgets.(staticBudgetPortV2)
			reader.fact.State = control.BudgetFactRevoked
			g.Budgets = reader
		}},
		{name: "budget_exact_expiry", reason: core.ReasonBudgetBindingStale, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.Budgets.(staticBudgetPortV2)
			reader.fact.ExpiresUnixNano = now.UnixNano()
			g.Budgets = reader
		}},
		{name: "credential_revoked", reason: core.ReasonCredentialLeaseMissing, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.Credentials.(staticCredentialReaderV2)
			reader.fact.Active = false
			g.Credentials = reader
		}},
		{name: "credential_epoch", reason: core.ReasonCredentialLeaseMissing, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.Credentials.(staticCredentialReaderV2)
			reader.fact.Epoch++
			g.Credentials = reader
		}},
		{name: "credential_exact_expiry", reason: core.ReasonCredentialLeaseMissing, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.Credentials.(staticCredentialReaderV2)
			reader.fact.ExpiresUnixNano = now.UnixNano()
			g.Credentials = reader
		}},
		{name: "current_scope_inactive", reason: core.ReasonEffectFenceStale, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.CurrentScopes.(staticScopeReaderV2)
			reader.fact.State = ports.ExecutionScopeFactRevoked
			g.CurrentScopes = reader
		}},
		{name: "current_scope_revision", reason: core.ReasonEffectFenceStale, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.CurrentScopes.(staticScopeReaderV2)
			reader.fact.Revision++
			g.CurrentScopes = reader
		}},
		{name: "current_scope_digest", reason: core.ReasonEffectFenceStale, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.CurrentScopes.(staticScopeReaderV2)
			reader.fact.Digest = controlEffectDigestV2(t, "different-current-scope")
			g.CurrentScopes = reader
		}},
		{name: "current_scope_exact_expiry", reason: core.ReasonEffectFenceStale, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			reader := g.CurrentScopes.(staticScopeReaderV2)
			reader.fact.ExpiresUnixNano = now.UnixNano()
			g.CurrentScopes = reader
		}},
		{name: "permit_exact_expiry", reason: core.ReasonDispatchPermitInvalid, mutate: func(g *control.GovernanceDispatchGatewayV2) {
			g.Clock = func() time.Time { return now.Add(4 * time.Second) }
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			gateway, effect := gatewayFixtureV2(t, now, false)
			issued, err := gateway.Issue(context.Background(), control.IssueGovernedDispatchRequestV2{EffectID: effect.Intent.ID, ExpectedEffectRevision: effect.Revision, PermitID: "permit-" + testCase.name, AttemptID: "attempt-" + testCase.name, PermitTTL: 5 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			testCase.mutate(&gateway)
			if _, err := gateway.Begin(context.Background(), control.BeginGovernedDispatchRequestV2{EffectID: effect.Intent.ID, ExpectedEffectRevision: issued.Effect.Revision, PermitID: issued.Permit.Permit.ID, ExpectedPermitRevision: issued.Permit.Revision}); !core.HasReason(err, testCase.reason) {
				t.Fatalf("final Begin gate must reject drift, got %v", err)
			}
			inspected, err := gateway.Effects.InspectDispatchPermit(context.Background(), issued.Permit.Permit.ID)
			if err != nil || inspected.State != control.DispatchPermitIssued {
				t.Fatalf("failed authorization must not consume permit or reach provider: %v %+v", err, inspected)
			}
		})
	}
}

func TestGovernanceGatewayV2ConditionalReviewCannotDispatch(t *testing.T) {
	t.Parallel()
	now := time.Unix(29_800, 0)
	gateway, effect := gatewayFixtureV2(t, now, false)
	review := gateway.Review.(staticReviewReaderV2)
	review.fact.Decision = ports.ReviewDecisionConditional
	review.fact.ConditionsDigest = controlEffectDigestV2(t, "conditions-only")
	gateway.Review = review
	if _, err := gateway.Issue(context.Background(), control.IssueGovernedDispatchRequestV2{EffectID: effect.Intent.ID, ExpectedEffectRevision: effect.Revision, PermitID: "permit-conditional", AttemptID: "attempt-conditional", PermitTTL: 5 * time.Second}); !core.HasReason(err, core.ReasonReviewConditionUnsatisfied) {
		t.Fatalf("conditions digest alone is not a satisfaction fact: %v", err)
	}
}

func gatewayFixtureV2(t *testing.T, now time.Time, wrongOwner bool) (control.GovernanceDispatchGatewayV2, control.EffectFactV2) {
	t.Helper()
	intent := controlEffectFactV2(t, now, false, false).Intent
	manifestDigest := intent.Provider.ManifestDigest
	credential := ports.CredentialLeaseRefV2{Ref: "credential-lease-1", Class: "vendor/model-provider", ScopeDigest: controlEffectDigestV2(t, "credential-scope"), Epoch: 1}
	intent.CredentialLeases = []ports.CredentialLeaseRefV2{credential}
	if wrongOwner {
		intent.Owners[1].ComponentID = "vendor/caller-selected-owner"
	}
	budget := control.BudgetBindingFactV2{Ref: "budget-1", IntentID: intent.ID, IntentRevision: intent.Revision, Scope: intent.Scope, Mode: control.BudgetOperationNotRequired, PolicyDigest: intent.Budget.PolicyDigest, PolicyDecisionRef: "budget-policy-fact-1", PolicyEvidenceDigest: controlEffectDigestV2(t, "budget-evidence"), State: control.BudgetFactActive, Revision: 1, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	budgetRef, err := budget.BindingRefV2()
	if err != nil {
		t.Fatal(err)
	}
	intent.Budget = budgetRef
	grant := ports.CapabilityGrantV2{Capability: intent.Provider.Capability, EvidenceDigest: controlEffectDigestV2(t, "grant-evidence"), ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	owners := []ports.OwnerAssignmentV2{{Role: ports.OwnerEffect, OwnerComponentID: intent.Provider.ComponentID}, {Role: ports.OwnerSettlement, OwnerComponentID: intent.Provider.ComponentID}, {Role: ports.OwnerCleanup, OwnerComponentID: intent.Provider.ComponentID}}
	set := control.BindingSetFactV2{ID: intent.Provider.BindingSetID, PlanID: "plan-1", PlanDigest: controlEffectDigestV2(t, "plan-binding"), GovernanceDigest: controlEffectDigestV2(t, "governance"), State: control.BindingSetActive, Revision: intent.Provider.BindingSetRevision, Members: []control.BindingMemberV2{{BindingID: "binding-provider", BindingRevision: 3, ComponentID: intent.Provider.ComponentID, Kind: "vendor/provider-kind", ManifestDigest: manifestDigest, ArtifactDigest: intent.Provider.ArtifactDigest, Contract: ports.ContractBindingV2{Name: "vendor/provider-contract", Version: "2.0.0", Compatible: ports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}}, Owners: owners, Grants: []ports.CapabilityGrantV2{grant}}}, TopologicalOrder: []ports.ComponentIDV2{intent.Provider.ComponentID}, Residuals: []control.BindingResidualV2{}, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	grantDigest, err := set.CapabilityGrantDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	source := func(name string) ports.GovernanceSourceFactRefV2 {
		return ports.GovernanceSourceFactRefV2{Ref: name, Revision: 1, Digest: controlEffectDigestV2(t, name)}
	}
	sandboxSource := source("sandbox-source")
	currentScope := ports.ExecutionScopeCurrentFactV2{Ref: "scope-current-1", Revision: 1, Scope: intent.Scope, CapabilityGrantDigest: grantDigest, ActivationSource: source("activation-source"), InstanceSource: source("instance-source"), SandboxSource: &sandboxSource, AuthoritySource: source("authority-source"), BindingSource: source("binding-source"), RunSource: source("run-source"), ActiveRunID: intent.RunID, RunState: "running", ProjectionWatermark: 1, State: ports.ExecutionScopeFactActive, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	currentScopeDigest, err := currentScope.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	currentScope.Digest = currentScopeDigest
	intent.CurrentScope = ports.ExecutionScopeBindingRefV2{Ref: currentScope.Ref, Digest: currentScopeDigest, Revision: currentScope.Revision}
	policyCandidateDigest, err := intent.PolicyCandidateDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	policy := ports.DispatchPolicyFactV2{Ref: intent.Policy.Ref, Revision: intent.Policy.Revision, IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: policyCandidateDigest, Scope: intent.Scope, EffectKind: intent.Kind, RiskClass: intent.RiskClass, ActionScopeDigest: intent.ActionScopeDigest, MaximumPermitTTL: 10 * time.Second, Active: true, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	policyDigest, err := policy.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	policy.Digest = policyDigest
	intent.Policy.Digest = policyDigest
	proposed, err := control.NewProposedEffectFactV2(intent, now)
	if err != nil {
		t.Fatal(err)
	}
	store := fakes.NewEffectStoreV2(func() time.Time { return now })
	if _, err := store.CreateBudgetBinding(context.Background(), budget); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateEffect(context.Background(), proposed); err != nil {
		t.Fatal(err)
	}
	accepted := proposed
	accepted.State, accepted.Revision = control.EffectAccepted, proposed.Revision+1
	accepted, err = store.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: proposed.Revision, Next: accepted})
	if err != nil {
		t.Fatal(err)
	}
	authority := ports.DispatchAuthorityFactV2{Ref: intent.Authority.Ref, Digest: intent.Authority.Digest, Revision: intent.Authority.Revision, Scope: intent.Scope, ActionScopeDigest: intent.ActionScopeDigest, State: ports.AuthorityFactActive, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	subjectDigest, err := intent.ReviewSubjectDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	review := ports.DispatchReviewFactV2{Ref: intent.Review.Ref, Digest: intent.Review.Digest, Revision: intent.Review.Revision, IntentID: intent.ID, IntentRevision: intent.Revision, SubjectDigest: subjectDigest, CandidateDigest: intent.Review.Digest, VerdictDigest: controlEffectDigestV2(t, "review-verdict"), VerdictRevision: 1, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision, ScopeDigest: intent.ActionScopeDigest, PolicyDigest: intent.Review.PolicyDigest, PolicyDecisionRef: "review-policy-fact-1", ActorAuthorityDigest: intent.Authority.Digest, ReviewerAuthorityDigest: controlEffectDigestV2(t, "reviewer-authority"), EvidenceDigest: controlEffectDigestV2(t, "review-evidence"), Decision: ports.ReviewDecisionOperationNotRequired, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	identityLease := control.IdentityExecutionLease{ID: "identity-lease-1", Identity: intent.Scope.Identity, Lineage: intent.Scope.Lineage, ActivationAttemptID: "activation-1", State: control.IdentityLeaseActive, AuthorityEpoch: intent.Scope.AuthorityEpoch, ExpiresAt: now.Add(time.Minute), Revision: 2}
	credentialFact := ports.CredentialLeaseFactV2{Ref: credential.Ref, Class: credential.Class, ScopeDigest: credential.ScopeDigest, Epoch: credential.Epoch, Active: true, ExpiresUnixNano: now.Add(4 * time.Second).UnixNano()}
	gateway := control.GovernanceDispatchGatewayV2{Effects: store, Bindings: staticBindingPortV2{set: set}, Budgets: staticBudgetPortV2{base: store, fact: budget}, IdentityLeases: staticIdentityPortV2{lease: identityLease}, Authority: staticAuthorityReaderV2{fact: authority}, Policies: staticPolicyReaderV2{fact: policy}, CurrentScopes: staticScopeReaderV2{fact: currentScope}, Review: staticReviewReaderV2{fact: review}, Credentials: staticCredentialReaderV2{fact: credentialFact}, Clock: func() time.Time { return now }}
	return gateway, accepted
}

type staticBindingPortV2 struct{ set control.BindingSetFactV2 }

type staticBudgetPortV2 struct {
	base control.BudgetFactPortV2
	fact control.BudgetBindingFactV2
}

func (s staticBudgetPortV2) CreateBudgetBinding(ctx context.Context, fact control.BudgetBindingFactV2) (control.BudgetBindingFactV2, error) {
	return s.base.CreateBudgetBinding(ctx, fact)
}
func (s staticBudgetPortV2) InspectBudgetBinding(context.Context, string) (control.BudgetBindingFactV2, error) {
	return s.fact, nil
}
func (s staticBudgetPortV2) CompareAndSwapBudgetBinding(ctx context.Context, request control.BudgetFactCASRequestV2) (control.BudgetBindingFactV2, error) {
	return s.base.CompareAndSwapBudgetBinding(ctx, request)
}

func (s staticBindingPortV2) CreateBinding(context.Context, control.BindingFactV2) (control.BindingFactV2, error) {
	return control.BindingFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticBindingPortV2) InspectBinding(context.Context, string) (control.BindingFactV2, error) {
	return control.BindingFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticBindingPortV2) CompareAndSwapBinding(context.Context, control.BindingFactCASRequestV2) (control.BindingFactV2, error) {
	return control.BindingFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticBindingPortV2) CommitBindingSet(context.Context, control.CommitBindingSetRequestV2) (control.BindingSetFactV2, error) {
	return control.BindingSetFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticBindingPortV2) InspectBindingSet(context.Context, string) (control.BindingSetFactV2, error) {
	return s.set, nil
}
func (s staticBindingPortV2) CompareAndSwapBindingSet(context.Context, control.BindingSetCASRequestV2) (control.BindingSetFactV2, error) {
	return control.BindingSetFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}

type staticIdentityPortV2 struct {
	lease control.IdentityExecutionLease
}

func (s staticIdentityPortV2) ReserveIdentityLease(context.Context, control.ReserveIdentityLeaseRequest) (control.IdentityExecutionLease, error) {
	return control.IdentityExecutionLease{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticIdentityPortV2) RenewIdentityLease(context.Context, control.RenewIdentityLeaseRequest) (control.IdentityExecutionLease, error) {
	return control.IdentityExecutionLease{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticIdentityPortV2) RevokeIdentityLease(context.Context, control.EndIdentityLeaseRequest) (control.IdentityExecutionLease, error) {
	return control.IdentityExecutionLease{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticIdentityPortV2) ReleaseIdentityLease(context.Context, control.EndIdentityLeaseRequest) (control.IdentityExecutionLease, error) {
	return control.IdentityExecutionLease{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticIdentityPortV2) InspectIdentityLease(context.Context, core.TenantID, core.AgentIdentityID) (control.IdentityExecutionLease, error) {
	return s.lease, nil
}

type staticAuthorityReaderV2 struct{ fact ports.DispatchAuthorityFactV2 }

func (s staticAuthorityReaderV2) InspectDispatchAuthority(context.Context, string) (ports.DispatchAuthorityFactV2, error) {
	return s.fact, nil
}

type staticPolicyReaderV2 struct{ fact ports.DispatchPolicyFactV2 }

func (s staticPolicyReaderV2) InspectDispatchPolicy(context.Context, string) (ports.DispatchPolicyFactV2, error) {
	return s.fact, nil
}

type staticScopeReaderV2 struct {
	fact ports.ExecutionScopeCurrentFactV2
}

func (s staticScopeReaderV2) InspectCurrentExecutionScope(context.Context, string) (ports.ExecutionScopeCurrentFactV2, error) {
	return s.fact, nil
}

type staticReviewReaderV2 struct{ fact ports.DispatchReviewFactV2 }

func (s staticReviewReaderV2) InspectDispatchReview(context.Context, string) (ports.DispatchReviewFactV2, error) {
	return s.fact, nil
}

type staticCredentialReaderV2 struct{ fact ports.CredentialLeaseFactV2 }

func (s staticCredentialReaderV2) InspectCredentialLease(_ context.Context, ref string) (ports.CredentialLeaseFactV2, error) {
	if s.fact.Ref != ref {
		return ports.CredentialLeaseFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonCredentialLeaseMissing, "credential lease not found")
	}
	return s.fact, nil
}
