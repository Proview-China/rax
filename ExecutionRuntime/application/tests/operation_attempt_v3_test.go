package application_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestGovernedOperationAttemptV3NormalPathAndTypedSettlementGuard(t *testing.T) {
	chain := normalOperationAttemptChainV3(t, time.Unix(1_800_000_000, 0), "custom.ninth/process")
	for i, f := range chain {
		if err := f.Validate(); err != nil {
			t.Fatalf("stage %d invalid: %v", i, err)
		}
		if i > 0 {
			if err := contract.ValidateGovernedOperationAttemptTransitionV3(chain[i-1], f); err != nil {
				t.Fatalf("transition %d rejected: %v", i, err)
			}
		}
	}
	ref, err := chain[len(chain)-1].RefV3()
	if err != nil || ref.Settlement == nil || ref.DispatchUnknown {
		t.Fatalf("normal typed ref invalid: %#v %v", ref, err)
	}
	if err := ref.ValidateSettledForV3(*chain[len(chain)-1].Settlement); err != nil {
		t.Fatal(err)
	}
	wrong := cloneSettlementForTestV3(chain[len(chain)-1].Settlement)
	wrong.Disposition = runtimeports.OperationSettlementFailedV3
	if ref.ValidateSettledForV3(*wrong) == nil {
		t.Fatal("typed guard accepted another Settlement")
	}
}

func TestGovernedOperationAttemptV3UnknownBranchesNeverRequireObservationOrRedispatch(t *testing.T) {
	normal := normalOperationAttemptChainV3(t, time.Unix(1_800_000_000, 0), "custom.future/process")
	for _, from := range []int{3, 4, 5, 6} {
		t.Run(string(normal[from].State), func(t *testing.T) {
			unknown := unknownAfterV3(t, normal[from])
			if err := contract.ValidateGovernedOperationAttemptTransitionV3(normal[from], unknown); err != nil {
				t.Fatalf("unknown branch rejected: %v", err)
			}
			if unknown.Observation != nil || unknown.State != contract.OperationDispatchUnknownV3 {
				t.Fatal("unknown branch claimed Observation")
			}
			settled := settleUnknownV3(t, unknown)
			if err := contract.ValidateGovernedOperationAttemptTransitionV3(unknown, settled); err != nil {
				t.Fatalf("unknown settlement rejected: %v", err)
			}
			ref, err := settled.RefV3()
			if err != nil || !ref.DispatchUnknown || ref.Settlement.Observation != nil {
				t.Fatalf("unknown typed ref invalid: %#v %v", ref, err)
			}
			redispatch := unknown
			redispatch.Revision++
			redispatch.State = contract.OperationDelegationDeclaredV3
			redispatch.UpdatedUnixNano++
			if contract.ValidateGovernedOperationAttemptTransitionV3(unknown, redispatch) == nil {
				t.Fatal("dispatch_unknown allowed blind redispatch")
			}
		})
	}
}

func TestGovernedOperationAttemptV3PostPreparedUnknownRequiresExactEnforcementRevision(t *testing.T) {
	chain := normalOperationAttemptChainV3(t, time.Unix(1_800_000_000, 0), "custom.post-prepared-unknown/process")
	prepared := chain[6]
	unknown := unknownAfterV3(t, prepared)
	if unknown.UnknownAuthorization.PermitFactRevision != prepared.BegunAuthorization.PermitFactRevision+1 || unknown.Enforcement.RecordedRevision != unknown.UnknownAuthorization.PermitFactRevision {
		t.Fatalf("post-prepared unknown did not preserve the exact Enforcement revision: begun=%d enforcement=%d unknown=%d", prepared.BegunAuthorization.PermitFactRevision, unknown.Enforcement.RecordedRevision, unknown.UnknownAuthorization.PermitFactRevision)
	}
	if err := contract.ValidateGovernedOperationAttemptTransitionV3(prepared, unknown); err != nil {
		t.Fatalf("exact post-prepared unknown transition rejected: %v", err)
	}

	for name, mutate := range map[string]func(*contract.GovernedOperationAttemptFactV3){
		"same-as-begun": func(f *contract.GovernedOperationAttemptFactV3) {
			f.UnknownAuthorization.PermitFactRevision = f.BegunAuthorization.PermitFactRevision
		},
		"jump-more-than-one": func(f *contract.GovernedOperationAttemptFactV3) {
			f.UnknownAuthorization.PermitFactRevision++
		},
		"unrelated-recorded-revision": func(f *contract.GovernedOperationAttemptFactV3) {
			f.Enforcement.RecordedRevision++
		},
		"wrong-enforcement-permit": func(f *contract.GovernedOperationAttemptFactV3) {
			f.Enforcement.PermitID = "other-permit"
		},
		"wrong-enforcement-attempt": func(f *contract.GovernedOperationAttemptFactV3) {
			f.Enforcement.AttemptID = "other-attempt"
		},
		"wrong-enforcement-operation": func(f *contract.GovernedOperationAttemptFactV3) {
			f.Enforcement.OperationDigest = core.DigestBytes([]byte("other-operation"))
		},
		"wrong-authorization-effect": func(f *contract.GovernedOperationAttemptFactV3) {
			f.UnknownAuthorization.Attempt.EffectID = "other-effect"
		},
		"wrong-authorization-permit": func(f *contract.GovernedOperationAttemptFactV3) {
			f.UnknownAuthorization.Attempt.PermitID = "other-permit"
		},
		"wrong-authorization-attempt": func(f *contract.GovernedOperationAttemptFactV3) {
			f.UnknownAuthorization.Attempt.AttemptID = "other-attempt"
		},
		"wrong-authorization-operation": func(f *contract.GovernedOperationAttemptFactV3) {
			f.UnknownAuthorization.Attempt.OperationDigest = core.DigestBytes([]byte("other-operation"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			forged := unknown
			authorization := *unknown.UnknownAuthorization
			forged.UnknownAuthorization = &authorization
			preparedRef := *unknown.Prepared
			forged.Prepared = &preparedRef
			enforcement := *unknown.Enforcement
			forged.Enforcement = &enforcement
			mutate(&forged)
			if forged.Validate() == nil {
				t.Fatal("post-prepared unknown accepted revision without one exact persisted Enforcement cause")
			}
		})
	}

	prePrepared := unknownAfterV3(t, chain[5])
	prePrepared.UnknownAuthorization.PermitFactRevision++
	if prePrepared.Validate() == nil {
		t.Fatal("pre-prepared unknown accepted an uncaused Permit fact revision advance")
	}
}

func TestGovernedOperationAttemptV3IssuedWorkerAcceptsOnlyExactRuntimeUnknown(t *testing.T) {
	chain := normalOperationAttemptChainV3(t, time.Unix(1_800_000_000, 0), "custom.two-workers/process")
	issued := chain[3]
	unknown := unknownAfterV3(t, issued)
	if unknown.BegunAuthorization != nil {
		t.Fatal("issued worker forged a Begun Authorization")
	}
	if err := contract.ValidateGovernedOperationAttemptTransitionV3(issued, unknown); err != nil {
		t.Fatalf("exact Runtime unknown rejected: %v", err)
	}
	for name, mutate := range map[string]func(*contract.GovernedOperationAttemptFactV3){"effect-revision": func(f *contract.GovernedOperationAttemptFactV3) { f.UnknownAuthorization.EffectFactRevision-- }, "permit-revision": func(f *contract.GovernedOperationAttemptFactV3) { f.UnknownAuthorization.PermitFactRevision-- }, "permit": func(f *contract.GovernedOperationAttemptFactV3) { f.UnknownAuthorization.Permit.ID = "other-permit" }, "fence": func(f *contract.GovernedOperationAttemptFactV3) {
		f.UnknownAuthorization.Fence.ExpiresAt = f.UnknownAuthorization.Fence.ExpiresAt.Add(-time.Nanosecond)
	}} {
		t.Run(name, func(t *testing.T) {
			forged := unknown
			a := *unknown.UnknownAuthorization
			forged.UnknownAuthorization = &a
			mutate(&forged)
			if forged.Validate() == nil {
				t.Fatal("issued-to-unknown accepted drift")
			}
		})
	}
}

func TestGovernedOperationAttemptV3UnknownSettlementMayPersistExactDomainResult(t *testing.T) {
	chain := normalOperationAttemptChainV3(t, time.Unix(1_800_000_000, 0), "custom.unknown-result/process")
	unknown := unknownAfterV3(t, chain[6])
	settled := settleUnknownV3(t, unknown)
	payload := settlementDomainPayloadV3("unknown-result")
	schema := payload.Schema
	settled.Settlement.DomainResultSchema = &schema
	settled.Settlement.DomainResultDigest = payload.ContentDigest
	settled.SettlementDomainResult = &payload
	if err := settled.Validate(); err != nil {
		t.Fatalf("unknown exact DomainResult rejected: %v", err)
	}
	if err := contract.ValidateGovernedOperationAttemptTransitionV3(unknown, settled); err != nil {
		t.Fatalf("unknown Settlement and DomainResult were not one CAS transition: %v", err)
	}
	missing := settled
	missing.SettlementDomainResult = nil
	if missing.Validate() == nil {
		t.Fatal("Settlement DomainResult metadata was accepted without its payload")
	}
	forged := settled
	bad := payload
	bad.ContentDigest = core.DigestBytes([]byte("different-result"))
	forged.SettlementDomainResult = &bad
	if forged.Validate() == nil {
		t.Fatal("unknown Settlement accepted mismatched DomainResult")
	}
}

func TestGovernedOperationAttemptV3UnknownSettlementDelegationMatchesPreparationBoundary(t *testing.T) {
	chain := normalOperationAttemptChainV3(t, time.Unix(1_800_000_000, 0), "custom.unknown-delegation/process")
	prePrepared := unknownAfterV3(t, chain[5])
	settledPre := settleUnknownV3(t, prePrepared)
	d := *chain[6].PreparedDelegation
	settledPre.Settlement.Attempt.Delegation = &d
	if settledPre.Validate() == nil {
		t.Fatal("pre-prepared unknown Settlement smuggled Delegation")
	}
	prepared := unknownAfterV3(t, chain[6])
	settledPrepared := settleUnknownV3(t, prepared)
	if settledPrepared.Settlement.Attempt.Delegation == nil || *settledPrepared.Settlement.Attempt.Delegation != *prepared.PreparedDelegation {
		t.Fatal("prepared unknown Settlement omitted exact Delegation")
	}
	missing := settledPrepared
	missing.Settlement = cloneSettlementForTestV3(settledPrepared.Settlement)
	missing.Settlement.Attempt.Delegation = nil
	if missing.Validate() == nil {
		t.Fatal("prepared unknown Settlement accepted missing Delegation")
	}
	wrong := settledPrepared
	wrong.Settlement = cloneSettlementForTestV3(settledPrepared.Settlement)
	wrong.Settlement.Attempt.Delegation = &runtimeports.ExecutionDelegationRefV2{ID: "other-delegation", Revision: 2, Digest: core.DigestBytes([]byte("other-delegation"))}
	if wrong.Validate() == nil {
		t.Fatal("prepared unknown Settlement accepted another Delegation")
	}
}

func TestGovernedOperationAttemptV3RejectsSkippedStageRewritesAndObservationMismatch(t *testing.T) {
	chain := normalOperationAttemptChainV3(t, time.Unix(1_800_000_000, 0), "custom.future-module/process")
	if err := contract.ValidateGovernedOperationAttemptTransitionV3(chain[0], chain[2]); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("skip accepted: %v", err)
	}
	forged := chain[5]
	forged.IntentValue.Target = "different/provider"
	if err := contract.ValidateGovernedOperationAttemptTransitionV3(chain[4], forged); !core.HasReason(err, core.ReasonEffectStateConflict) {
		t.Fatalf("immutable Intent rewrite accepted: %v", err)
	}
	forged = chain[8]
	forged.Settlement = cloneSettlementForTestV3(chain[8].Settlement)
	forged.Settlement.Observation = cloneObservationForTestV3(chain[7].Observation)
	forged.Settlement.Observation.SourceSequence++
	if forged.Validate() == nil {
		t.Fatal("normal Settlement accepted another Observation")
	}
}

func TestGovernedOperationAttemptV3ConstructorPersistsRestartInputsForCustomModule(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	fixture := operationAttemptFixtureV3(t, now, "user.module-eleven/process")
	f := fixture.base
	if f.IntentValue.ID != f.Intent.EffectID || f.DispatchPlan.PermitID == "" || f.DelegationPlan.DelegationID == "" || len(f.DelegationPlan.RelayHops) == 0 || f.StepKind != "user.module-eleven/process" || f.DomainAdapter.ComponentID == "" || f.DomainAdapter == f.PlannedProvider {
		t.Fatalf("rev1 omitted restart input: %#v", f)
	}
	changed := f
	changed.DelegationPlan.RelayHops = append([]runtimeports.ExecutionRelayHopV2(nil), f.DelegationPlan.RelayHops...)
	changed.DelegationPlan.RelayHops[0].Relay.ArtifactDigest = core.DigestBytes([]byte("drift"))
	if changed.Validate() == nil {
		t.Fatal("Delegation plan allowed host relay drift")
	}
	legacyPlan := fixture.bundle.Plan
	legacyPlan.Steps = append([]contract.WorkflowStepV2(nil), fixture.bundle.Plan.Steps...)
	legacyPlan.Steps[0].DomainAdapter = nil
	if _, err := contract.NewWorkflowJournalV2("journal-operation-without-domain-adapter", legacyPlan, now.UnixNano()); !core.HasReason(err, core.ReasonProviderBindingStale) {
		t.Fatalf("workflow admission accepted a governed step without DomainAdapter: %v", err)
	}
}

func TestGovernedOperationAttemptV3RejectsPlanProviderAndAuthorityDrift(t *testing.T) {
	base := operationAttemptFixtureV3(t, time.Unix(1_800_000_000, 0), "custom.plan-binding/process").base
	cases := map[string]func(*contract.GovernedOperationAttemptFactV3){"provider-capability": func(f *contract.GovernedOperationAttemptFactV3) {
		f.PlannedProvider.Capability = "custom.plan-binding/other"
	}, "binding-revision": func(f *contract.GovernedOperationAttemptFactV3) { f.PlannedProvider.BindingSetRevision++ }, "authority-epoch": func(f *contract.GovernedOperationAttemptFactV3) { f.PlanAuthority.Epoch++ }, "authority-digest": func(f *contract.GovernedOperationAttemptFactV3) {
		f.PlanAuthority.Digest = core.DigestBytes([]byte("other-authority"))
	}, "domain-binding-set": func(f *contract.GovernedOperationAttemptFactV3) {
		f.DomainAdapter.BindingSetID = "binding-set-other"
	}, "domain-binding-revision": func(f *contract.GovernedOperationAttemptFactV3) {
		f.DomainAdapter.BindingSetRevision++
	}, "domain-component": func(f *contract.GovernedOperationAttemptFactV3) {
		f.DomainAdapter.ComponentID = "custom.other/domain-adapter"
	}, "domain-manifest": func(f *contract.GovernedOperationAttemptFactV3) {
		f.DomainAdapter.ManifestDigest = core.DigestBytes([]byte("other-domain-manifest"))
	}, "domain-artifact": func(f *contract.GovernedOperationAttemptFactV3) {
		f.DomainAdapter.ArtifactDigest = core.DigestBytes([]byte("other-domain-artifact"))
	}, "domain-capability": func(f *contract.GovernedOperationAttemptFactV3) {
		f.DomainAdapter.Capability = "custom.other/apply-settlement"
	}}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			forged := base
			mutate(&forged)
			if forged.Validate() == nil {
				t.Fatal("Plan binding drift accepted")
			}
		})
	}
}

func TestGovernedOperationAttemptV3RejectsDomainAdapterTransitionDrift(t *testing.T) {
	chain := normalOperationAttemptChainV3(t, time.Unix(1_800_000_000, 0), "custom.domain-transition/process")
	current, admitted := chain[0], chain[1]
	cases := map[string]func(*contract.GovernedOperationAttemptFactV3){
		"binding-set":      func(f *contract.GovernedOperationAttemptFactV3) { f.DomainAdapter.BindingSetID = "binding-set-other" },
		"binding-revision": func(f *contract.GovernedOperationAttemptFactV3) { f.DomainAdapter.BindingSetRevision++ },
		"component": func(f *contract.GovernedOperationAttemptFactV3) {
			f.DomainAdapter.ComponentID = "custom.other/domain-adapter"
		},
		"manifest": func(f *contract.GovernedOperationAttemptFactV3) {
			f.DomainAdapter.ManifestDigest = core.DigestBytes([]byte("other-domain-manifest"))
		},
		"artifact": func(f *contract.GovernedOperationAttemptFactV3) {
			f.DomainAdapter.ArtifactDigest = core.DigestBytes([]byte("other-domain-artifact"))
		},
		"capability": func(f *contract.GovernedOperationAttemptFactV3) {
			f.DomainAdapter.Capability = "custom.other/apply-settlement"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			forged := admitted
			mutate(&forged)
			if err := contract.ValidateGovernedOperationAttemptTransitionV3(current, forged); !core.HasReason(err, core.ReasonEffectStateConflict) {
				t.Fatalf("DomainAdapter drift crossed an attempt CAS: %v", err)
			}
		})
	}
}

func TestGovernedOperationAttemptRefV3RejectsForgedDomainRoutingBindings(t *testing.T) {
	base := operationAttemptFixtureV3(t, time.Unix(1_800_000_000, 0), "custom.domain-route/process").base
	ref, err := base.RefV3()
	if err != nil {
		t.Fatal(err)
	}
	if ref.StepKind != base.StepKind || ref.Descriptor != base.Descriptor || ref.PlannedProvider != base.PlannedProvider || ref.DomainAdapter != base.DomainAdapter || ref.PlanAuthority != base.PlanAuthority {
		t.Fatal("Attempt Ref omitted persisted routing bindings")
	}
	cases := map[string]func(*contract.GovernedOperationAttemptRefV3){"step-kind": func(r *contract.GovernedOperationAttemptRefV3) { r.StepKind = "custom.other/process" }, "descriptor": func(r *contract.GovernedOperationAttemptRefV3) {
		r.Descriptor.Digest = core.DigestBytes([]byte("other-descriptor"))
	}, "provider": func(r *contract.GovernedOperationAttemptRefV3) {
		r.PlannedProvider.Capability = "custom.other/capability"
	}, "domain-binding-set": func(r *contract.GovernedOperationAttemptRefV3) {
		r.DomainAdapter.BindingSetID = "binding-set-other"
	}, "domain-binding-revision": func(r *contract.GovernedOperationAttemptRefV3) {
		r.DomainAdapter.BindingSetRevision++
	}, "domain-component": func(r *contract.GovernedOperationAttemptRefV3) {
		r.DomainAdapter.ComponentID = "custom.other/domain-adapter"
	}, "domain-manifest": func(r *contract.GovernedOperationAttemptRefV3) {
		r.DomainAdapter.ManifestDigest = core.DigestBytes([]byte("other-domain-manifest"))
	}, "domain-artifact": func(r *contract.GovernedOperationAttemptRefV3) {
		r.DomainAdapter.ArtifactDigest = core.DigestBytes([]byte("other-domain-artifact"))
	}, "domain-capability": func(r *contract.GovernedOperationAttemptRefV3) {
		r.DomainAdapter.Capability = "custom.other/apply-settlement"
	}, "authority": func(r *contract.GovernedOperationAttemptRefV3) { r.PlanAuthority.Epoch++ }}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			forged := ref
			mutate(&forged)
			if forged.Validate() == nil {
				t.Fatal("forged Domain routing binding accepted")
			}
		})
	}
}

func TestGovernedOperationAttemptStoreV3LostRepliesConflictIsolationAndDeepCopy(t *testing.T) {
	chain := normalOperationAttemptChainV3(t, time.Unix(1_800_000_000, 0), "custom.ninth/process")
	store := fakes.NewGovernedOperationAttemptStoreV3()
	store.LoseNextCreateReply = true
	if _, err := store.CreateGovernedOperationAttemptV3(context.Background(), chain[0]); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("create loss missing: %v", err)
	}
	got, err := store.InspectGovernedOperationAttemptV3(context.Background(), chain[0].Scope, chain[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := chain[0].DigestV3()
	actual, _ := got.DigestV3()
	if want != actual {
		t.Fatal("lost create reply persisted different content")
	}
	got.IntentValue.Payload.Inline[0] ^= 0xff
	got.DelegationPlan.RelayHops[0].Relay.ComponentID = "mutated/client"
	again, _ := store.InspectGovernedOperationAttemptV3(context.Background(), chain[0].Scope, chain[0].ID)
	againDigest, _ := again.DigestV3()
	if againDigest != want {
		t.Fatal("fake leaked nested mutable fields")
	}
	changed := chain[0]
	changed.DispatchPlan.AttemptID = "changed-attempt"
	if _, err := store.CreateGovernedOperationAttemptV3(context.Background(), changed); !core.HasReason(err, core.ReasonAlreadyExists) {
		t.Fatalf("same ID changed content accepted: %v", err)
	}
	store.LoseNextCASReply = true
	if _, err := store.CompareAndSwapGovernedOperationAttemptV3(context.Background(), applicationports.GovernedOperationAttemptCASRequestV3{Scope: chain[1].Scope, ID: chain[1].ID, ExpectedRevision: 1, Next: chain[1]}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("CAS loss missing: %v", err)
	}
	got, _ = store.InspectGovernedOperationAttemptV3(context.Background(), chain[1].Scope, chain[1].ID)
	if got.Revision != 2 || got.State != contract.OperationDomainReservedV3 {
		t.Fatal("lost CAS not recoverable by Inspect")
	}
	other := chain[0]
	other.Scope.Identity.TenantID = "tenant-other"
	other.Operation.ExecutionScope = other.Scope
	other.IntentValue.Operation.ExecutionScope = other.Scope
	other.ScopeDigest, _ = runtimeports.ExecutionScopeDigestV2(other.Scope)
	other.Operation.ExecutionScopeDigest = other.ScopeDigest
	other.IntentValue.Operation.ExecutionScopeDigest = other.ScopeDigest
	other.IntentValue.ConflictDomain.ScopeDigest = runtimeports.StableTenantScopeDigestV2(other.Scope.Identity.TenantID)
	other.IntentValue.Idempotency.ScopeDigest = other.IntentValue.ConflictDomain.ScopeDigest
	other.Intent.OperationDigest, _ = other.Operation.DigestV3()
	other.IntentValue.Budget.SubjectDigest = other.Intent.OperationDigest
	other.IntentValue.Policy.SubjectDigest = other.Intent.OperationDigest
	other.Intent, _ = contract.NewOperationIntentRefV3(other.IntentValue)
	if _, err := store.CreateGovernedOperationAttemptV3(context.Background(), other); err != nil {
		t.Fatalf("tenant isolation failed: %v", err)
	}
}

func TestGovernedOperationAttemptStoreV3ConcurrentCASLinearizesOnce(t *testing.T) {
	chain := normalOperationAttemptChainV3(t, time.Unix(1_800_000_000, 0), "custom.tenth/process")
	store := fakes.NewGovernedOperationAttemptStoreV3()
	if _, err := store.CreateGovernedOperationAttemptV3(context.Background(), chain[0]); err != nil {
		t.Fatal(err)
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.CompareAndSwapGovernedOperationAttemptV3(context.Background(), applicationports.GovernedOperationAttemptCASRequestV3{Scope: chain[1].Scope, ID: chain[1].ID, ExpectedRevision: 1, Next: chain[1]})
			if err == nil {
				wins.Add(1)
			} else if !core.HasReason(err, core.ReasonRevisionConflict) {
				t.Errorf("unexpected CAS error: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("CAS linearized %d winners", wins.Load())
	}
}

func TestGovernedOperationAttemptStoreV3TwoWorkerIssuedToUnknownLostCASOnlyInspects(t *testing.T) {
	chain := normalOperationAttemptChainV3(t, time.Unix(1_800_000_000, 0), "custom.unknown-recovery/process")
	store := fakes.NewGovernedOperationAttemptStoreV3()
	if _, err := store.CreateGovernedOperationAttemptV3(context.Background(), chain[0]); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		if _, err := store.CompareAndSwapGovernedOperationAttemptV3(context.Background(), applicationports.GovernedOperationAttemptCASRequestV3{Scope: chain[i].Scope, ID: chain[i].ID, ExpectedRevision: chain[i-1].Revision, Next: chain[i]}); err != nil {
			t.Fatal(err)
		}
	}
	unknown := unknownAfterV3(t, chain[3])
	store.LoseNextCASReply = true
	if _, err := store.CompareAndSwapGovernedOperationAttemptV3(context.Background(), applicationports.GovernedOperationAttemptCASRequestV3{Scope: unknown.Scope, ID: unknown.ID, ExpectedRevision: chain[3].Revision, Next: unknown}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("unknown CAS loss not injected: %v", err)
	}
	inspected, err := store.InspectGovernedOperationAttemptV3(context.Background(), unknown.Scope, unknown.ID)
	if err != nil || inspected.State != contract.OperationDispatchUnknownV3 || inspected.UnknownAuthorization == nil {
		t.Fatalf("unknown CAS not recoverable by Inspect: %#v %v", inspected, err)
	}
	blind := chain[4]
	blind.Revision = inspected.Revision + 1
	blind.UpdatedUnixNano = inspected.UpdatedUnixNano + 1
	if _, err := store.CompareAndSwapGovernedOperationAttemptV3(context.Background(), applicationports.GovernedOperationAttemptCASRequestV3{Scope: blind.Scope, ID: blind.ID, ExpectedRevision: inspected.Revision, Next: blind}); err == nil {
		t.Fatal("persisted unknown outcome was blindly redispatched")
	}
}

func TestGovernedOperationAttemptStoreV3LostSettledCASReplaysExactDomainResult(t *testing.T) {
	chain := normalOperationAttemptChainV3(t, time.Unix(1_800_000_000, 0), "custom.result-replay/process")
	store := fakes.NewGovernedOperationAttemptStoreV3()
	if _, err := store.CreateGovernedOperationAttemptV3(context.Background(), chain[0]); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(chain)-1; i++ {
		if _, err := store.CompareAndSwapGovernedOperationAttemptV3(context.Background(), applicationports.GovernedOperationAttemptCASRequestV3{Scope: chain[i].Scope, ID: chain[i].ID, ExpectedRevision: chain[i-1].Revision, Next: chain[i]}); err != nil {
			t.Fatal(err)
		}
	}
	store.LoseNextCASReply = true
	last := chain[len(chain)-1]
	if _, err := store.CompareAndSwapGovernedOperationAttemptV3(context.Background(), applicationports.GovernedOperationAttemptCASRequestV3{Scope: last.Scope, ID: last.ID, ExpectedRevision: chain[len(chain)-2].Revision, Next: last}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("settled CAS loss missing: %v", err)
	}
	got, err := store.InspectGovernedOperationAttemptV3(context.Background(), last.Scope, last.ID)
	if err != nil || got.SettlementDomainResult == nil {
		t.Fatalf("settled payload not recoverable: %#v %v", got.SettlementDomainResult, err)
	}
	want := settlementPayloadDigestForTestV3(*last.SettlementDomainResult)
	actual := settlementPayloadDigestForTestV3(*got.SettlementDomainResult)
	if want != actual {
		t.Fatal("replayed DomainResult differs")
	}
	got.SettlementDomainResult.Inline[0] ^= 0xff
	again, _ := store.InspectGovernedOperationAttemptV3(context.Background(), last.Scope, last.ID)
	againDigest := settlementPayloadDigestForTestV3(*again.SettlementDomainResult)
	if againDigest != want {
		t.Fatal("fake leaked mutable DomainResult bytes")
	}
}

type operationAttemptFixtureForTestV3 struct {
	base       contract.GovernedOperationAttemptFactV3
	bundle     contract.SubmissionBundleV2
	journal    contract.WorkflowJournalV2
	operation  runtimeports.OperationSubjectV3
	intent     runtimeports.OperationEffectIntentV3
	dispatch   contract.OperationDispatchPlanV3
	delegation contract.ExecutionDelegationPlanV3
	now        time.Time
	provider   runtimeports.ProviderBindingRefV2
}

func operationAttemptFixtureV3(t *testing.T, now time.Time, kind runtimeports.NamespacedNameV2) operationAttemptFixtureForTestV3 {
	t.Helper()
	bundle, _ := applicationFixtureV2(t, now, kind, true)
	journal, err := contract.NewWorkflowJournalV2("journal-operation", bundle.Plan, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	scopeDigest, _ := runtimeports.ExecutionScopeDigestV2(bundle.Plan.Target)
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeRunV3, ExecutionScope: bundle.Plan.Target, ExecutionScopeDigest: scopeDigest, RunID: "run-operation", SubjectRevision: 1, CurrentProjectionRef: "projection-operation", CurrentProjectionDigest: core.DigestBytes([]byte("projection-operation")), CurrentProjectionRevision: 1}
	intent := validApplicationOperationIntentV3(t, bundle, operation, now)
	provider := *bundle.Plan.Steps[0].Provider
	host := runtimeports.ProviderBindingRefV2{BindingSetID: provider.BindingSetID, BindingSetRevision: provider.BindingSetRevision, ComponentID: "application.host/relay", ManifestDigest: core.DigestBytes([]byte("host-manifest")), ArtifactDigest: core.DigestBytes([]byte("host-artifact")), Capability: "application.host/relay"}
	dispatch := contract.OperationDispatchPlanV3{PermitID: "permit-operation", AttemptID: "provider-attempt", PermitTTLNanos: int64(20 * time.Second)}
	delegation := contract.ExecutionDelegationPlanV3{ContractVersion: contract.GovernedOperationAttemptContractVersionV3, DelegationID: "delegation-operation", HostAdapter: host, RelayHops: []runtimeports.ExecutionRelayHopV2{{Sequence: 1, Relay: host}}, EndpointID: "endpoint-operation", RuntimeSessionRef: "session-operation", HostBindingExpiresUnixNano: now.Add(time.Hour).UnixNano(), ProviderBindingExpiresUnixNano: now.Add(time.Hour).UnixNano(), DelegationTTLNanos: int64(10 * time.Second)}
	base, err := contract.NewGovernedOperationAttemptFactV3("operation-attempt-shared", bundle.Plan, journal, bundle.Plan.Steps[0].ID, 1, operation, intent, dispatch, delegation, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	return operationAttemptFixtureForTestV3{base: base, bundle: bundle, journal: journal, operation: operation, intent: intent, dispatch: dispatch, delegation: delegation, now: now, provider: provider}
}

func normalOperationAttemptChainV3(t *testing.T, now time.Time, kind runtimeports.NamespacedNameV2) []contract.GovernedOperationAttemptFactV3 {
	t.Helper()
	fx := operationAttemptFixtureV3(t, now, kind)
	chain := []contract.GovernedOperationAttemptFactV3{fx.base}
	next := func(state contract.GovernedOperationAttemptStateV3) contract.GovernedOperationAttemptFactV3 {
		v := chain[len(chain)-1]
		v.Revision++
		v.State = state
		v.UpdatedUnixNano += int64(time.Millisecond)
		return v
	}
	reserved := next(contract.OperationDomainReservedV3)
	reservation := operationDomainReservationForTestV3(t, fx.base, fx.now)
	reserved.DomainReservation = &reservation
	chain = append(chain, reserved)
	admitted := next(contract.OperationEffectAdmittedV3)
	admitted.Admission = &runtimeports.OperationEffectAdmissionReceiptV3{OperationDigest: admitted.Intent.OperationDigest, EffectID: admitted.Intent.EffectID, IntentRevision: admitted.Intent.IntentRevision, IntentDigest: admitted.Intent.IntentDigest, FactRevision: 1, State: "accepted"}
	chain = append(chain, admitted)
	issued := next(contract.OperationPermitIssuedV3)
	authorization := validAuthorizationV3(t, issued, fx.now)
	issued.IssuedAuthorization = &authorization
	chain = append(chain, issued)
	begun := next(contract.OperationPermitBegunV3)
	begunAuth := authorization
	begunAuth.State = runtimeports.OperationDispatchAuthorizationBegunV3
	begunAuth.PermitFactRevision++
	begun.BegunAuthorization = &begunAuth
	chain = append(chain, begun)
	declared := next(contract.OperationDelegationDeclaredV3)
	fact := validDelegationFactV3(t, declared, fx.provider, fx.now)
	declared.DelegationFact = &fact
	ref, _ := fact.RefV2()
	declared.DeclaredDelegation = &ref
	chain = append(chain, declared)
	prepared := next(contract.OperationExecutionPreparedV3)
	preparedRef := runtimeports.ExecutionDelegationRefV2{ID: ref.ID, Revision: ref.Revision + 1, Digest: core.DigestBytes([]byte("prepared-delegation"))}
	prepared.PreparedDelegation = &preparedRef
	raw := runtimeports.PreparedProviderAttemptRefV2{ID: fact.PreparedAttemptID, Revision: 1, DeclaredDelegation: ref, OperationDigest: prepared.Intent.OperationDigest, IntentID: prepared.Intent.EffectID, IntentRevision: prepared.Intent.IntentRevision, IntentDigest: prepared.Intent.IntentDigest, PermitID: begunAuth.Attempt.PermitID, PermitRevision: begunAuth.Attempt.PermitRevision, PermitDigest: begunAuth.Attempt.PermitDigest, AttemptID: begunAuth.Attempt.AttemptID, Provider: fx.provider, PayloadSchema: prepared.IntentValue.Payload.Schema, PayloadDigest: prepared.IntentValue.Payload.ContentDigest, PayloadRevision: prepared.IntentValue.PayloadRevision, PreparedUnixNano: fx.now.Add(4 * time.Millisecond).UnixNano(), ExpiresUnixNano: fx.now.Add(15 * time.Second).UnixNano()}
	sealed, err := runtimeports.SealPreparedProviderAttemptRefV2(raw)
	if err != nil {
		t.Fatal(err)
	}
	prepared.Prepared = &sealed
	prepared.Enforcement = &runtimeports.PersistedOperationEnforcementRefV3{PermitID: sealed.PermitID, PermitRevision: sealed.PermitRevision, PermitDigest: sealed.PermitDigest, AttemptID: sealed.AttemptID, OperationDigest: sealed.OperationDigest, Provider: sealed.Provider, ReceiptDigest: core.DigestBytes([]byte("enforcement")), RecordedRevision: 3}
	chain = append(chain, prepared)
	observed := next(contract.OperationProviderObservedV3)
	evidence := runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: core.DigestBytes([]byte("ledger")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("evidence"))}
	observed.Observation = &runtimeports.ProviderAttemptObservationRefV2{Delegation: preparedRef, PreparedAttemptID: sealed.ID, ProviderOperationRef: "provider-operation", Revision: 1, State: runtimeports.ProviderAttemptObservedV2, Digest: core.DigestBytes([]byte("observation")), PayloadDigest: sealed.PayloadDigest, PayloadRevision: 1, SourceRegistrationID: "source-registration", SourceEpoch: 1, SourceSequence: 1, Evidence: evidence, ObservedUnixNano: fx.now.Add(5 * time.Millisecond).UnixNano()}
	chain = append(chain, observed)
	settled := next(contract.OperationSettledV3)
	delegation := preparedRef
	observation := *observed.Observation
	result := settlementDomainPayloadV3("normal-result")
	resultSchema := result.Schema
	settled.Settlement = &runtimeports.OperationSettlementRefV3{ID: "settlement-operation", Revision: 1, Digest: core.DigestBytes([]byte("settlement")), Attempt: runtimeports.OperationDispatchAttemptRefV3{OperationDigest: settled.Intent.OperationDigest, EffectID: settled.Intent.EffectID, IntentRevision: settled.Intent.IntentRevision, IntentDigest: settled.Intent.IntentDigest, PermitID: begunAuth.Attempt.PermitID, PermitRevision: begunAuth.Attempt.PermitRevision, PermitDigest: begunAuth.Attempt.PermitDigest, AttemptID: begunAuth.Attempt.AttemptID, Delegation: &delegation}, Disposition: runtimeports.OperationSettlementAppliedV3, Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: fx.provider.ComponentID, ManifestDigest: fx.provider.ManifestDigest}, Observation: &observation, Evidence: []runtimeports.EvidenceRecordRefV2{evidence}, DomainResultSchema: &resultSchema, DomainResultDigest: result.ContentDigest}
	settled.SettlementDomainResult = &result
	chain = append(chain, settled)
	return chain
}

func operationDomainReservationForTestV3(t *testing.T, initial contract.GovernedOperationAttemptFactV3, now time.Time) contract.OperationDomainReservationRefV3 {
	t.Helper()
	ref, err := initial.RefV3()
	if err != nil {
		t.Fatal(err)
	}
	operationDigest, _ := initial.Operation.DigestV3()
	reservation, err := contract.SealOperationDomainReservationRefV3(contract.OperationDomainReservationRefV3{
		ContractVersion: contract.GovernedOperationAttemptContractVersionV3,
		ID:              "domain-reservation:" + initial.ID, Revision: 1,
		StepKind: initial.StepKind, Descriptor: initial.Descriptor, DomainAdapter: initial.DomainAdapter,
		AttemptID: ref.ID, AttemptRevision: ref.Revision, AttemptDigest: ref.Digest, IntentDigest: initial.Intent.IntentDigest,
		DomainSubjectDigest: operationDigest, SessionRef: initial.DelegationPlan.RuntimeSessionRef,
		CandidateDigest: initial.IntentValue.Payload.ContentDigest, ReservedUnixNano: initial.IntentValue.ExpiresUnixNano - int64(30*time.Second),
		ExpiresUnixNano: initial.IntentValue.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	return reservation
}

func unknownAfterV3(t *testing.T, from contract.GovernedOperationAttemptFactV3) contract.GovernedOperationAttemptFactV3 {
	t.Helper()
	v := from
	v.Revision++
	v.State = contract.OperationDispatchUnknownV3
	v.UpdatedUnixNano++
	base := from.BegunAuthorization
	if base == nil {
		base = from.IssuedAuthorization
	}
	a := *base
	a.State = runtimeports.OperationDispatchAuthorizationUnknownV3
	if from.BegunAuthorization == nil {
		a.EffectFactRevision += 2
		a.PermitFactRevision++
	} else {
		a.EffectFactRevision++
		if from.Prepared != nil {
			a.PermitFactRevision++
		}
	}
	v.UnknownAuthorization = &a
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	return v
}
func settleUnknownV3(t *testing.T, unknown contract.GovernedOperationAttemptFactV3) contract.GovernedOperationAttemptFactV3 {
	t.Helper()
	v := unknown
	v.Revision++
	v.State = contract.OperationSettledV3
	v.UpdatedUnixNano++
	a := unknown.UnknownAuthorization.Attempt
	if unknown.PreparedDelegation != nil {
		delegation := *unknown.PreparedDelegation
		a.Delegation = &delegation
	}
	e := runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: core.DigestBytes([]byte("unknown-ledger")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("unknown-evidence"))}
	v.Settlement = &runtimeports.OperationSettlementRefV3{ID: "settlement-unknown", Revision: 1, Digest: core.DigestBytes([]byte("settlement-unknown")), Attempt: a, Disposition: runtimeports.OperationSettlementNotAppliedV3, Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: unknown.IntentValue.Provider.ComponentID, ManifestDigest: unknown.IntentValue.Provider.ManifestDigest}, Evidence: []runtimeports.EvidenceRecordRefV2{e}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	return v
}

func validAuthorizationV3(t *testing.T, f contract.GovernedOperationAttemptFactV3, now time.Time) runtimeports.OperationDispatchAuthorizationV3 {
	t.Helper()
	d := func(s string) core.Digest { return core.DigestBytes([]byte(s)) }
	expires := now.Add(20 * time.Second)
	governance := func(id string) runtimeports.OperationGovernanceFactRefV3 {
		return runtimeports.OperationGovernanceFactRefV3{Ref: id, Revision: 1, Digest: d(id), ExpiresUnixNano: expires.UnixNano()}
	}
	review := runtimeports.OperationReviewAuthorizationV3{Case: governance(f.IntentValue.Review.CaseRef), CandidateDigest: f.IntentValue.Review.CandidateDigest, CandidateRevision: f.IntentValue.Review.CandidateRevision, Verdict: governance("verdict-operation"), ReviewerAuthority: governance("reviewer-authority"), PolicyDigest: f.IntentValue.Review.PolicyDigest, ExpiresUnixNano: expires.UnixNano()}
	capability := d("capability")
	fence := core.ExecutionFence{BoundaryScope: core.FenceBoundaryInstance, Scope: f.Scope, CapabilityGrantDigest: capability, EffectIntentID: f.Intent.EffectID, EffectIntentRevision: f.Intent.IntentRevision, CanonicalPayloadDigest: f.IntentValue.Payload.ContentDigest, ExpiresAt: expires}
	fenceDigest, err := runtimeports.DigestOperationExecutionFenceV3(fence, f.Operation)
	if err != nil {
		t.Fatal(err)
	}
	permit := runtimeports.OperationDispatchPermitV3{ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: f.DispatchPlan.PermitID, Revision: 1, AttemptID: f.DispatchPlan.AttemptID, IntentID: f.Intent.EffectID, IntentRevision: f.Intent.IntentRevision, IntentDigest: f.Intent.IntentDigest, Operation: f.Operation, PayloadSchema: f.IntentValue.Payload.Schema, PayloadDigest: f.IntentValue.Payload.ContentDigest, PayloadRevision: f.IntentValue.PayloadRevision, ConflictDomain: f.IntentValue.ConflictDomain, Provider: f.IntentValue.Provider, EnforcementPoint: f.IntentValue.Provider, Authority: f.IntentValue.Authority, Review: f.IntentValue.Review, ReviewAuthorization: review, Budget: f.IntentValue.Budget, Policy: f.IntentValue.Policy, CapabilityGrantDigest: capability, CredentialGrantDigest: d("credentials"), GovernanceSnapshotDigest: d("governance"), FenceDigest: fenceDigest, Idempotency: f.IntentValue.Idempotency, IssuedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()}
	permitDigest, err := permit.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	attempt := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: f.Intent.OperationDigest, EffectID: f.Intent.EffectID, IntentRevision: f.Intent.IntentRevision, IntentDigest: f.Intent.IntentDigest, PermitID: permit.ID, PermitRevision: permit.Revision, PermitDigest: permitDigest, AttemptID: permit.AttemptID}
	a := runtimeports.OperationDispatchAuthorizationV3{Attempt: attempt, Permit: permit, EffectFactRevision: 2, PermitFactRevision: 1, State: runtimeports.OperationDispatchAuthorizationIssuedV3, Fence: fence, ExpiresUnixNano: expires.UnixNano()}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	return a
}

func validDelegationFactV3(t *testing.T, f contract.GovernedOperationAttemptFactV3, provider runtimeports.ProviderBindingRefV2, now time.Time) runtimeports.ExecutionDelegationFactV2 {
	t.Helper()
	p := f.DelegationPlan
	a := f.BegunAuthorization.Attempt
	preparedID, err := runtimeports.DerivePreparedProviderAttemptIDV2(p.DelegationID, a.PermitID, a.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	fact := runtimeports.ExecutionDelegationFactV2{ContractVersion: runtimeports.ExecutionGovernanceContractVersionV2, ID: p.DelegationID, Revision: 1, State: runtimeports.ExecutionDelegationDeclaredV2, BindingSetID: provider.BindingSetID, BindingSetRevision: provider.BindingSetRevision, Operation: f.Operation, HostAdapter: p.HostAdapter, DataProvider: provider, RelayHops: append([]runtimeports.ExecutionRelayHopV2(nil), p.RelayHops...), EndpointID: p.EndpointID, RuntimeSessionRef: p.RuntimeSessionRef, PayloadSchema: f.IntentValue.Payload.Schema, PayloadDigest: f.IntentValue.Payload.ContentDigest, PayloadRevision: f.IntentValue.PayloadRevision, IntentID: a.EffectID, IntentRevision: a.IntentRevision, IntentDigest: a.IntentDigest, ProviderPermitID: a.PermitID, ProviderPermitRevision: a.PermitRevision, ProviderPermitDigest: a.PermitDigest, ProviderAttemptID: a.AttemptID, PreparedAttemptID: preparedID, OperationExpiresUnixNano: f.IntentValue.ExpiresUnixNano, PermitExpiresUnixNano: f.BegunAuthorization.Permit.ExpiresUnixNano, HostBindingExpiresUnixNano: p.HostBindingExpiresUnixNano, ProviderBindingExpiresUnixNano: p.ProviderBindingExpiresUnixNano, CreatedUnixNano: now.Add(3 * time.Millisecond).UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano()}
	if err := fact.Validate(); err != nil {
		t.Fatal(err)
	}
	return fact
}

func validApplicationOperationIntentV3(t *testing.T, bundle contract.SubmissionBundleV2, operation runtimeports.OperationSubjectV3, now time.Time) runtimeports.OperationEffectIntentV3 {
	t.Helper()
	d := func(s string) core.Digest { return core.DigestBytes([]byte(s)) }
	subjectDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	provider := *bundle.Plan.Steps[0].Provider
	manifest := provider.ManifestDigest
	intent := runtimeports.OperationEffectIntentV3{ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: "effect-operation", Revision: 1, Operation: operation, Kind: "user.module-eleven/effect", RiskClass: "user.module-eleven/controlled", ActionScopeDigest: d("action-scope"), Payload: bundle.Plan.Steps[0].Payload, PayloadRevision: 1, Target: "user.module-eleven/provider", ConflictDomain: runtimeports.ConflictDomainBindingV2{Domain: "user.module-eleven/conflict", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(bundle.Plan.Target.Identity.TenantID)}, Owners: []runtimeports.EffectOwnerRefV2{{Role: runtimeports.OwnerCleanup, ComponentID: provider.ComponentID, ManifestDigest: manifest}, {Role: runtimeports.OwnerEffect, ComponentID: provider.ComponentID, ManifestDigest: manifest}, {Role: runtimeports.OwnerSettlement, ComponentID: provider.ComponentID, ManifestDigest: manifest}}, Provider: provider, Authority: bundle.Plan.Authority, Review: runtimeports.OperationReviewBindingRefV3{CaseRef: "review-operation", CandidateDigest: d("candidate"), CandidateRevision: 1, PolicyDigest: d("review-policy")}, Budget: runtimeports.OperationBudgetBindingRefV3{Ref: "budget-operation", Digest: d("budget"), Revision: 1, PolicyDigest: d("budget-policy"), SubjectDigest: subjectDigest}, Policy: runtimeports.OperationPolicyBindingRefV3{Ref: "policy-operation", Digest: d("policy"), Revision: 1, SubjectDigest: subjectDigest}, Idempotency: runtimeports.IdempotencyBindingV2{Key: "idem-operation", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(bundle.Plan.Target.Identity.TenantID), Class: core.IdempotencyQueryable}, CredentialLeases: []runtimeports.CredentialLeaseRefV2{}, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	if err := intent.Validate(); err != nil {
		t.Fatal(err)
	}
	return intent
}

func cloneSettlementForTestV3(value *runtimeports.OperationSettlementRefV3) *runtimeports.OperationSettlementRefV3 {
	v := *value
	v.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), value.Evidence...)
	if value.Attempt.Delegation != nil {
		d := *value.Attempt.Delegation
		v.Attempt.Delegation = &d
	}
	if value.Observation != nil {
		o := *value.Observation
		v.Observation = &o
	}
	return &v
}
func cloneObservationForTestV3(value *runtimeports.ProviderAttemptObservationRefV2) *runtimeports.ProviderAttemptObservationRefV2 {
	v := *value
	return &v
}

func settlementDomainPayloadV3(value string) runtimeports.OpaquePayloadV2 {
	bytes := []byte(`{"result":"` + value + `"}`)
	schema := runtimeports.SchemaRefV2{Namespace: "custom.result", Name: "settlement", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("settlement-result-schema"))}
	return runtimeports.OpaquePayloadV2{Schema: schema, ContentDigest: core.DigestBytes(bytes), Length: uint64(len(bytes)), Inline: bytes, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "custom.result/limit", Digest: core.DigestBytes([]byte("settlement-result-limit"))}}
}

func settlementPayloadDigestForTestV3(value runtimeports.OpaquePayloadV2) core.Digest {
	digest, _ := core.CanonicalJSONDigest("praxis.application.test", contract.GovernedOperationAttemptContractVersionV3, "SettlementDomainResult", value)
	return digest
}
