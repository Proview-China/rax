package fakes_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationReviewAuthorizationV5QuorumLostReplyConcurrentAndConformance(t *testing.T) {
	fixture, gateway, store, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
	store.LoseNextCreateReplyV5()
	request := operationReviewAuthorizationRequestV5(fixture, "authorization-v5", ports.OperationReviewBasisAcceptedQuorumV5)
	fact, err := gateway.CreateOperationReviewAuthorizationV5(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if fact.Review.Quorum == nil || fact.Review.PolicyNotRequired != nil || store.CreateCommitCountV5() != 1 {
		t.Fatalf("quorum union or create-once drifted: %+v commits=%d", fact.Review, store.CreateCommitCountV5())
	}

	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, callErr := gateway.CreateOperationReviewAuthorizationV5(context.Background(), request)
			if callErr == nil && got.Digest != fact.Digest {
				callErr = fmt.Errorf("same canonical replay returned another fact")
			}
			errs <- callErr
		}()
	}
	wg.Wait()
	close(errs)
	for callErr := range errs {
		if callErr != nil {
			t.Fatal(callErr)
		}
	}
	if store.CreateCommitCountV5() != 1 {
		t.Fatalf("same canonical request committed %d times", store.CreateCommitCountV5())
	}

	report, err := conformance.CheckOperationReviewAuthorizationV5(context.Background(), conformance.OperationReviewAuthorizationCaseV5{Gateway: gateway, Facts: store, Request: request})
	if err != nil {
		t.Fatal(err)
	}
	if !report.RuntimeOwnerObserved || !report.CurrentInspectObserved || !report.HistoricalExactObserved || report.AuthorizationIsPermit || report.ReviewOwnedAuthorization || report.ProductionClaimEligible {
		t.Fatalf("conformance exceeded authority: %+v", report)
	}
}

func TestOperationReviewAuthorizationV5PolicyNotRequiredHasNoVerdictOrAttestation(t *testing.T) {
	fixture, gateway, _, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisPolicyNotRequiredV5)
	fact, err := gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(fixture, "authorization-not-required-v5", ports.OperationReviewBasisPolicyNotRequiredV5))
	if err != nil {
		t.Fatal(err)
	}
	if fact.Review.PolicyNotRequired == nil || fact.Review.Quorum != nil {
		t.Fatalf("not-required used quorum/Verdict branch: %+v", fact.Review)
	}
	if fact.Review.PolicyNotRequired.BypassDecision.ID == "" || fact.Review.PolicyNotRequired.PolicyDecisionRef.Ref == "" {
		t.Fatal("not-required lost exact Bypass/Policy facts")
	}
}

func TestOperationReviewAuthorizationV5ConditionalQuorumRequiresExactSatisfaction(t *testing.T) {
	fixture, gateway, store, review := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisConditionalQuorumSatisfiedV5)
	fact, err := gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(fixture, "authorization-conditional-v5", ports.OperationReviewBasisConditionalQuorumSatisfiedV5))
	if err != nil || fact.Review.Quorum == nil || fact.Review.Quorum.Satisfaction == nil {
		t.Fatalf("conditional quorum was not authorized with exact satisfaction: %+v err=%v", fact, err)
	}
	review.mutate(func(p *ports.OperationReviewCurrentProjectionV5) { p.Quorum.Satisfaction = nil })
	if _, err := gateway.InspectCurrentOperationReviewAuthorizationV5(context.Background(), fixture.intent.Operation, fixture.intent.ID, fact.ID); err == nil {
		t.Fatal("removed condition satisfaction left Authorization current")
	}
	if store.CreateCommitCountV5() != 1 {
		t.Fatal("current inspection changed the Fact owner")
	}
}

func TestOperationReviewAuthorizationV5S1S2TTLAndClockFailClosedZeroWrite(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*operationFixtureV3, *kernel.OperationReviewAuthorizationGatewayV5, *operationReviewReaderV5)
	}{
		{"governance-drift", func(f *operationFixtureV3, _ *kernel.OperationReviewAuthorizationGatewayV5, r *operationReviewReaderV5) {
			r.after = func() {
				f.current.mutate(func(s *ports.OperationGovernanceSnapshotV3) {
					s.ProjectionWatermark++
					s.Identity.Digest = core.DigestBytes([]byte("changed-identity"))
				})
			}
		}},
		{"clock-rollback", func(f *operationFixtureV3, g *kernel.OperationReviewAuthorizationGatewayV5, _ *operationReviewReaderV5) {
			calls := 0
			g.Clock = func() time.Time {
				calls++
				if calls == 1 {
					return f.now
				}
				return f.now.Add(-time.Nanosecond)
			}
		}},
		{"ttl-crossing", func(f *operationFixtureV3, g *kernel.OperationReviewAuthorizationGatewayV5, r *operationReviewReaderV5) {
			r.mutateAndSeal(t, f.now, func(p *ports.OperationReviewCurrentProjectionV5) {
				setReviewV5Expiry(p, f.now.Add(time.Nanosecond).UnixNano())
			})
			calls := 0
			g.Clock = func() time.Time {
				calls++
				if calls == 1 {
					return f.now
				}
				return f.now.Add(time.Nanosecond)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, gateway, store, review := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
			test.prepare(fixture, &gateway, review)
			if _, err := gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(fixture, "authorization-fail-v5", ports.OperationReviewBasisAcceptedQuorumV5)); err == nil {
				t.Fatal("invalid current cut produced Authorization")
			}
			if store.CreateCommitCountV5() != 0 {
				t.Fatal("failed current cut leaked a Fact")
			}
		})
	}
}

func TestOperationReviewAuthorizationV5EveryBranchDriftStalesCurrent(t *testing.T) {
	tests := []struct {
		name   string
		change func(*testing.T, *operationFixtureV3, *operationReviewReaderV5)
	}{
		{"quorum", func(t *testing.T, f *operationFixtureV3, r *operationReviewReaderV5) {
			r.mutateAndSeal(t, f.now, func(p *ports.OperationReviewCurrentProjectionV5) {
				p.Quorum.QuorumDecision.Revision++
				p.Quorum.QuorumDecision.Digest = core.DigestBytes([]byte("changed-quorum"))
			})
		}},
		{"role-authority", func(t *testing.T, f *operationFixtureV3, r *operationReviewReaderV5) {
			r.mutateAndSeal(t, f.now, func(p *ports.OperationReviewCurrentProjectionV5) {
				p.Quorum.ReviewerAuthorityRefs[0].Digest = core.DigestBytes([]byte("changed-reviewer-authority"))
			})
		}},
		{"binding", func(t *testing.T, f *operationFixtureV3, r *operationReviewReaderV5) {
			r.mutateAndSeal(t, f.now, func(p *ports.OperationReviewCurrentProjectionV5) {
				p.Quorum.BindingRefs[0].Digest = core.DigestBytes([]byte("changed-review-binding"))
			})
		}},
		{"evidence", func(t *testing.T, f *operationFixtureV3, r *operationReviewReaderV5) {
			r.mutateAndSeal(t, f.now, func(p *ports.OperationReviewCurrentProjectionV5) {
				p.Quorum.DecisionEvidence[0].RecordDigest = core.DigestBytes([]byte("changed-evidence"))
			})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, gateway, _, review := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
			fact, err := gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(fixture, "authorization-drift-v5", ports.OperationReviewBasisAcceptedQuorumV5))
			if err != nil {
				t.Fatal(err)
			}
			test.change(t, fixture, review)
			if _, err := gateway.InspectCurrentOperationReviewAuthorizationV5(context.Background(), fixture.intent.Operation, fixture.intent.ID, fact.ID); err == nil {
				t.Fatal("drifted current remained authorized")
			}
		})
	}
}

func TestOperationReviewAuthorizationV5NotRequiredExactDriftStalesCurrent(t *testing.T) {
	tests := []struct {
		name   string
		change func(*ports.OperationReviewPolicyNotRequiredCurrentProjectionV5)
	}{
		{"bypass", func(p *ports.OperationReviewPolicyNotRequiredCurrentProjectionV5) {
			p.BypassDecision.Revision++
			p.BypassDecision.Digest = core.DigestBytes([]byte("changed-bypass"))
		}},
		{"policy", func(p *ports.OperationReviewPolicyNotRequiredCurrentProjectionV5) {
			p.PolicyDecisionRef.Revision++
			p.PolicyDecisionRef.Digest = core.DigestBytes([]byte("changed-policy"))
		}},
		{"scope", func(p *ports.OperationReviewPolicyNotRequiredCurrentProjectionV5) {
			p.ScopeRef.Revision++
			p.ScopeRef.Digest = core.DigestBytes([]byte("changed-scope"))
		}},
		{"binding", func(p *ports.OperationReviewPolicyNotRequiredCurrentProjectionV5) {
			p.BindingRef.Revision++
			p.BindingRef.Digest = core.DigestBytes([]byte("changed-binding"))
		}},
		{"authority", func(p *ports.OperationReviewPolicyNotRequiredCurrentProjectionV5) {
			p.ActorAuthorityRef.Revision++
			p.ActorAuthorityRef.Digest = core.DigestBytes([]byte("changed-authority"))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, gateway, _, review := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisPolicyNotRequiredV5)
			fact, err := gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(fixture, "authorization-not-required-drift-v5", ports.OperationReviewBasisPolicyNotRequiredV5))
			if err != nil {
				t.Fatal(err)
			}
			review.mutateAndSeal(t, fixture.now, func(p *ports.OperationReviewCurrentProjectionV5) { test.change(p.PolicyNotRequired) })
			if _, err := gateway.InspectCurrentOperationReviewAuthorizationV5(context.Background(), fixture.intent.Operation, fixture.intent.ID, fact.ID); err == nil {
				t.Fatal("not-required drift remained current")
			}
		})
	}
}

func TestOperationReviewAuthorizationV5DoesNotTypePunLegacyV3Review(t *testing.T) {
	fixture, gateway, _, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
	fact, err := gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(fixture, "authorization-no-v3-type-pun-v5", ports.OperationReviewBasisAcceptedQuorumV5))
	if err != nil {
		t.Fatal(err)
	}
	fixture.current.mutate(func(s *ports.OperationGovernanceSnapshotV3) {
		s.Review.Verdict.Revision++
		s.Review.Verdict.Digest = core.DigestBytes([]byte("legacy-v3-verdict-drift"))
	})
	current, err := gateway.InspectCurrentOperationReviewAuthorizationV5(context.Background(), fixture.intent.Operation, fixture.intent.ID, fact.ID)
	if err != nil || current.Digest != fact.Digest {
		t.Fatalf("legacy V3 Review was treated as V5 quorum authority: %+v err=%v", current, err)
	}
}

func TestOperationReviewAuthorizationV5RejectsTypedNilDependenciesAndChangedBasisReplay(t *testing.T) {
	fixture, gateway, _, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
	var nilStore *fakes.OperationReviewAuthorizationStoreV5
	gateway.Facts = nilStore
	if _, err := gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(fixture, "authorization-typed-nil-v5", ports.OperationReviewBasisAcceptedQuorumV5)); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Fact dependency accepted: %v", err)
	}
	fixture, gateway, _, _ = newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
	request := operationReviewAuthorizationRequestV5(fixture, "authorization-basis-replay-v5", ports.OperationReviewBasisAcceptedQuorumV5)
	if _, err := gateway.CreateOperationReviewAuthorizationV5(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Basis = ports.OperationReviewBasisPolicyNotRequiredV5
	if _, err := gateway.CreateOperationReviewAuthorizationV5(context.Background(), request); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("changed Basis replay did not conflict: %v", err)
	}
}

func TestOperationReviewAuthorizationV5StoreHistoryCASLostReplyAndDeepClone(t *testing.T) {
	fixture, gateway, store, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
	fact, err := gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(fixture, "authorization-history-v5", ports.OperationReviewBasisAcceptedQuorumV5))
	if err != nil {
		t.Fatal(err)
	}
	returned, err := store.InspectOperationReviewAuthorizationExactV5(context.Background(), fact.RefV5())
	if err != nil {
		t.Fatal(err)
	}
	returned.Review.Quorum.DecisionEvidence[0].RecordDigest = core.DigestBytes([]byte("mutated-alias"))
	again, err := store.InspectOperationReviewAuthorizationExactV5(context.Background(), fact.RefV5())
	if err != nil || again.Digest != fact.Digest || again.Review.Quorum.DecisionEvidence[0] != fact.Review.Quorum.DecisionEvidence[0] {
		t.Fatalf("stored history was aliased: %+v err=%v", again, err)
	}
	next := fact
	next.Revision++
	next.State = ports.OperationReviewAuthorizationRevokedV5
	next.InvalidationReason = core.ReasonReviewVerdictStale
	next.UpdatedUnixNano = fixture.now.UnixNano()
	next, err = ports.SealOperationReviewAuthorizationFactV5(next)
	if err != nil {
		t.Fatal(err)
	}
	store.LoseNextCASReplyV5()
	recoveredByGateway, err := gateway.CompareAndSwapOperationReviewAuthorizationV5(context.Background(), ports.OperationReviewAuthorizationCASRequestV5{ExpectedRevision: fact.Revision, Next: next})
	if err != nil || recoveredByGateway.Digest != next.Digest {
		t.Fatalf("lost CAS was not recovered by exact Inspect: %+v err=%v", recoveredByGateway, err)
	}
	current, err := store.InspectOperationReviewAuthorizationV5(context.Background(), fact.ID)
	if err != nil || current.Digest != next.Digest {
		t.Fatalf("CAS recovery did not Inspect current: %+v err=%v", current, err)
	}
	historical, err := store.InspectOperationReviewAuthorizationExactV5(context.Background(), fact.RefV5())
	if err != nil || historical.Digest != fact.Digest {
		t.Fatalf("terminal CAS overwrote history: %+v err=%v", historical, err)
	}
	if _, err := gateway.InspectCurrentOperationReviewAuthorizationV5(context.Background(), fixture.intent.Operation, fixture.intent.ID, fact.ID); err == nil {
		t.Fatal("terminal Authorization remained current")
	}
}

func TestOperationReviewAuthorizationV5ConcurrentDifferentIDsOneCurrentEffect(t *testing.T) {
	fixture, gateway, store, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
	const workers = 64
	var wg sync.WaitGroup
	successes := make(chan ports.OperationReviewAuthorizationFactV5, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fact, err := gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(fixture, fmt.Sprintf("authorization-race-v5-%d", i), ports.OperationReviewBasisAcceptedQuorumV5))
			if err == nil {
				successes <- fact
			} else {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(successes)
	close(errs)
	if len(successes) != 1 || store.CreateCommitCountV5() != 1 {
		t.Fatalf("different IDs produced successes=%d commits=%d", len(successes), store.CreateCommitCountV5())
	}
	for err := range errs {
		if !core.HasReason(err, core.ReasonEffectConflictDomainOccupied) {
			t.Fatalf("unexpected loser error: %v", err)
		}
	}
}

func TestOperationReviewAuthorizationV5SharedV4V5GuardLinearizesOneCurrentEffect(t *testing.T) {
	v4Fixture, v4Gateway, _, _ := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
	v5Fixture, v5Gateway, _, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
	shared := fakes.NewOperationReviewAuthorizationSharedStoreV45(func() time.Time { return v5Fixture.now })
	v4Gateway.Facts = shared
	v5Gateway.Facts = shared
	const workers = 64
	var wg sync.WaitGroup
	successes := make(chan string, workers)
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				_, err := v4Gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(v4Fixture, fmt.Sprintf("authorization-shared-v4-%d", i)))
				if err == nil {
					successes <- "v4"
				} else {
					errs <- err
				}
				return
			}
			_, err := v5Gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(v5Fixture, fmt.Sprintf("authorization-shared-v5-%d", i), ports.OperationReviewBasisAcceptedQuorumV5))
			if err == nil {
				successes <- "v5"
			} else {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(successes)
	close(errs)
	if len(successes) != 1 {
		t.Fatalf("V4/V5 shared guard produced %d current authorizations", len(successes))
	}
	for err := range errs {
		if !core.HasReason(err, core.ReasonEffectConflictDomainOccupied) {
			t.Fatalf("unexpected shared-guard loser: %v", err)
		}
	}
}

func TestOperationReviewAuthorizationV5SharedGuardReconcilesLostCreateReply(t *testing.T) {
	t.Run("v5-first", func(t *testing.T) {
		v4Fixture, v4Gateway, _, _ := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
		v5Fixture, v5Gateway, _, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
		shared := fakes.NewOperationReviewAuthorizationSharedStoreV45(func() time.Time { return v5Fixture.now })
		v4Gateway.Facts, v5Gateway.Facts = shared, shared

		shared.LoseNextCreateReplyV5()
		if _, err := v5Gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(v5Fixture, "authorization-lost-create-v5", ports.OperationReviewBasisAcceptedQuorumV5)); err != nil {
			t.Fatalf("V5 lost create did not recover exact Fact: %v", err)
		}
		if _, err := v4Gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(v4Fixture, "authorization-after-lost-v5")); !core.HasReason(err, core.ReasonEffectConflictDomainOccupied) {
			t.Fatalf("V5 lost create left shared guard unoccupied: %v", err)
		}
	})

	t.Run("v4-first", func(t *testing.T) {
		v4Fixture, v4Gateway, _, _ := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
		v5Fixture, v5Gateway, _, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
		shared := fakes.NewOperationReviewAuthorizationSharedStoreV45(func() time.Time { return v5Fixture.now })
		v4Gateway.Facts, v5Gateway.Facts = shared, shared

		shared.LoseNextCreateReplyV4()
		if _, err := v4Gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(v4Fixture, "authorization-lost-create-v4")); err != nil {
			t.Fatalf("V4 lost create did not recover exact Fact: %v", err)
		}
		if _, err := v5Gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(v5Fixture, "authorization-after-lost-v4", ports.OperationReviewBasisAcceptedQuorumV5)); !core.HasReason(err, core.ReasonEffectConflictDomainOccupied) {
			t.Fatalf("V4 lost create left shared guard unoccupied: %v", err)
		}
	})
}

func TestOperationReviewAuthorizationV5SharedGuardReconcilesLostTerminalCAS(t *testing.T) {
	t.Run("v5-terminal", func(t *testing.T) {
		v4Fixture, v4Gateway, _, _ := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
		v5Fixture, v5Gateway, _, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
		shared := fakes.NewOperationReviewAuthorizationSharedStoreV45(func() time.Time { return v5Fixture.now })
		v4Gateway.Facts, v5Gateway.Facts = shared, shared

		fact, err := v5Gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(v5Fixture, "authorization-lost-cas-v5", ports.OperationReviewBasisAcceptedQuorumV5))
		if err != nil {
			t.Fatal(err)
		}
		next := fact
		next.Revision++
		next.State = ports.OperationReviewAuthorizationRevokedV5
		next.InvalidationReason = core.ReasonReviewVerdictStale
		next.UpdatedUnixNano = v5Fixture.now.UnixNano()
		next, err = ports.SealOperationReviewAuthorizationFactV5(next)
		if err != nil {
			t.Fatal(err)
		}
		shared.LoseNextCASReplyV5()
		if recovered, err := v5Gateway.CompareAndSwapOperationReviewAuthorizationV5(context.Background(), ports.OperationReviewAuthorizationCASRequestV5{ExpectedRevision: fact.Revision, Next: next}); err != nil || recovered.Digest != next.Digest {
			t.Fatalf("V5 lost terminal CAS did not recover exact revision: %+v err=%v", recovered, err)
		}
		if _, err := v4Gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(v4Fixture, "authorization-after-terminal-v5")); err != nil {
			t.Fatalf("V5 lost terminal CAS left shared guard occupied: %v", err)
		}
	})

	t.Run("v4-terminal", func(t *testing.T) {
		v4Fixture, v4Gateway, _, _ := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
		v5Fixture, v5Gateway, _, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
		shared := fakes.NewOperationReviewAuthorizationSharedStoreV45(func() time.Time { return v5Fixture.now })
		v4Gateway.Facts, v5Gateway.Facts = shared, shared

		fact, err := v4Gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(v4Fixture, "authorization-lost-cas-v4"))
		if err != nil {
			t.Fatal(err)
		}
		next := fact
		next.Revision++
		next.State = ports.OperationReviewAuthorizationRevokedV4
		next.InvalidationReason = core.ReasonReviewVerdictStale
		next.UpdatedUnixNano = v4Fixture.now.UnixNano()
		next, err = ports.SealOperationReviewAuthorizationFactV4(next)
		if err != nil {
			t.Fatal(err)
		}
		shared.LoseNextCASReplyV4()
		if _, err := shared.CompareAndSwapOperationReviewAuthorizationV4(context.Background(), ports.OperationReviewAuthorizationCASRequestV4{ExpectedRevision: fact.Revision, Next: next}); !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("V4 lost terminal CAS was not surfaced: %v", err)
		}
		if recovered, err := shared.InspectOperationReviewAuthorizationV4(context.Background(), fact.ID); err != nil || recovered.Digest != next.Digest {
			t.Fatalf("V4 lost terminal CAS did not retain exact revision: %+v err=%v", recovered, err)
		}
		if _, err := v5Gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(v5Fixture, "authorization-after-terminal-v4", ports.OperationReviewBasisAcceptedQuorumV5)); err != nil {
			t.Fatalf("V4 lost terminal CAS left shared guard occupied: %v", err)
		}
	})
}

func TestOperationReviewAuthorizationV5SharedGuardStagedFailureLeaksNothing(t *testing.T) {
	t.Run("v5", func(t *testing.T) {
		v4Fixture, v4Gateway, _, _ := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
		v5Fixture, v5Gateway, _, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
		shared := fakes.NewOperationReviewAuthorizationSharedStoreV45(func() time.Time { return v5Fixture.now })
		v4Gateway.Facts, v5Gateway.Facts = shared, shared

		shared.FailNextCreateBeforeCommitV5()
		if _, err := v5Gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(v5Fixture, "authorization-staged-v5", ports.OperationReviewBasisAcceptedQuorumV5)); !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("V5 staged failure was not surfaced: %v", err)
		}
		if _, err := shared.InspectOperationReviewAuthorizationV5(context.Background(), "authorization-staged-v5"); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("V5 staged failure leaked history: %v", err)
		}
		if _, err := v4Gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(v4Fixture, "authorization-after-staged-v5")); err != nil {
			t.Fatalf("V5 staged failure leaked shared guard: %v", err)
		}
	})

	t.Run("v4", func(t *testing.T) {
		v4Fixture, v4Gateway, _, _ := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
		v5Fixture, v5Gateway, _, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
		shared := fakes.NewOperationReviewAuthorizationSharedStoreV45(func() time.Time { return v5Fixture.now })
		v4Gateway.Facts, v5Gateway.Facts = shared, shared

		shared.FailNextCreateBeforeCommitV4()
		if _, err := v4Gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(v4Fixture, "authorization-staged-v4")); !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("V4 staged failure was not surfaced: %v", err)
		}
		if _, err := shared.InspectOperationReviewAuthorizationV4(context.Background(), "authorization-staged-v4"); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("V4 staged failure leaked history: %v", err)
		}
		if _, err := v5Gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(v5Fixture, "authorization-after-staged-v4", ports.OperationReviewBasisAcceptedQuorumV5)); err != nil {
			t.Fatalf("V4 staged failure leaked shared guard: %v", err)
		}
	})
}

func TestOperationReviewAuthorizationV5SharedMutationsRejectNilContextWithoutPanic(t *testing.T) {
	t.Run("v4", func(t *testing.T) {
		fixture, gateway, _, _ := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
		fact, err := gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(fixture, "authorization-nil-context-v4"))
		if err != nil {
			t.Fatal(err)
		}
		shared := fakes.NewOperationReviewAuthorizationSharedStoreV45(func() time.Time { return fixture.now })
		if _, err := shared.CreateOperationReviewAuthorizationV4(nil, fact); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("V4 nil Create context did not fail closed: %v", err)
		}
		if _, err := shared.InspectOperationReviewAuthorizationV4(context.Background(), fact.ID); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("V4 nil Create context leaked history: %v", err)
		}
		if _, err := shared.CreateOperationReviewAuthorizationV4(context.Background(), fact); err != nil {
			t.Fatal(err)
		}
		next := fact
		next.Revision++
		next.State = ports.OperationReviewAuthorizationRevokedV4
		next.InvalidationReason = core.ReasonReviewVerdictStale
		next.UpdatedUnixNano = fixture.now.UnixNano()
		next, err = ports.SealOperationReviewAuthorizationFactV4(next)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := shared.CompareAndSwapOperationReviewAuthorizationV4(nil, ports.OperationReviewAuthorizationCASRequestV4{ExpectedRevision: fact.Revision, Next: next}); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("V4 nil CAS context did not fail closed: %v", err)
		}
		if current, err := shared.InspectOperationReviewAuthorizationV4(context.Background(), fact.ID); err != nil || current.Digest != fact.Digest {
			t.Fatalf("V4 nil CAS context changed current Fact: %+v err=%v", current, err)
		}
	})

	t.Run("v5", func(t *testing.T) {
		fixture, gateway, _, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
		fact, err := gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(fixture, "authorization-nil-context-v5", ports.OperationReviewBasisAcceptedQuorumV5))
		if err != nil {
			t.Fatal(err)
		}
		shared := fakes.NewOperationReviewAuthorizationSharedStoreV45(func() time.Time { return fixture.now })
		if _, err := shared.CreateOperationReviewAuthorizationV5(nil, fact); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("V5 nil Create context did not fail closed: %v", err)
		}
		if _, err := shared.InspectOperationReviewAuthorizationExactV5(context.Background(), fact.RefV5()); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("V5 nil Create context leaked history: %v", err)
		}
		if _, err := shared.CreateOperationReviewAuthorizationV5(context.Background(), fact); err != nil {
			t.Fatal(err)
		}
		next := fact
		next.Revision++
		next.State = ports.OperationReviewAuthorizationRevokedV5
		next.InvalidationReason = core.ReasonReviewVerdictStale
		next.UpdatedUnixNano = fixture.now.UnixNano()
		next, err = ports.SealOperationReviewAuthorizationFactV5(next)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := shared.CompareAndSwapOperationReviewAuthorizationV5(nil, ports.OperationReviewAuthorizationCASRequestV5{ExpectedRevision: fact.Revision, Next: next}); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("V5 nil CAS context did not fail closed: %v", err)
		}
		if current, err := shared.InspectOperationReviewAuthorizationExactV5(context.Background(), fact.RefV5()); err != nil || current.Digest != fact.Digest {
			t.Fatalf("V5 nil CAS context changed current Fact: %+v err=%v", current, err)
		}
	})
}

func TestOperationReviewAuthorizationV5LostInspectRetryUsesExactDetachedRead(t *testing.T) {
	fixture, gateway, store, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
	store.LoseNextInspectReplyV5()
	fact, err := gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(fixture, "authorization-inspect-retry-v5", ports.OperationReviewBasisAcceptedQuorumV5))
	if err != nil {
		t.Fatal(err)
	}
	store.LoseNextInspectReplyV5()
	current, err := gateway.InspectCurrentOperationReviewAuthorizationV5(context.Background(), fixture.intent.Operation, fixture.intent.ID, fact.ID)
	if err != nil || current.Digest != fact.Digest {
		t.Fatalf("exact Inspect retry failed: %+v err=%v", current, err)
	}
}

func TestOperationReviewAuthorizationV5InspectRetryStillUsesFreshTTLClock(t *testing.T) {
	fixture, gateway, store, _ := newOperationReviewAuthorizationFixtureV5(t, ports.OperationReviewBasisAcceptedQuorumV5)
	fact, err := gateway.CreateOperationReviewAuthorizationV5(context.Background(), operationReviewAuthorizationRequestV5(fixture, "authorization-retry-ttl-v5", ports.OperationReviewBasisAcceptedQuorumV5))
	if err != nil {
		t.Fatal(err)
	}
	store.LoseNextInspectReplyV5()
	calls := 0
	gateway.Clock = func() time.Time {
		calls++
		if calls == 1 {
			return fixture.now
		}
		return time.Unix(0, fact.ExpiresUnixNano)
	}
	if _, err := gateway.InspectCurrentOperationReviewAuthorizationV5(context.Background(), fixture.intent.Operation, fixture.intent.ID, fact.ID); err == nil {
		t.Fatal("lost Inspect retry crossed TTL but remained current")
	}
}

type operationReviewReaderV5 struct {
	mu    sync.Mutex
	value ports.OperationReviewCurrentProjectionV5
	after func()
}

func (r *operationReviewReaderV5) mutate(change func(*ports.OperationReviewCurrentProjectionV5)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	change(&r.value)
}

func (r *operationReviewReaderV5) InspectOperationReviewCurrentV5(_ context.Context, _ ports.OperationReviewCurrentRequestV5) (ports.OperationReviewCurrentProjectionV5, error) {
	r.mu.Lock()
	value := cloneReviewCurrentProjectionV5(r.value)
	after := r.after
	r.after = nil
	r.mu.Unlock()
	if after != nil {
		after()
	}
	return value, nil
}
func (r *operationReviewReaderV5) mutateAndSeal(t *testing.T, now time.Time, change func(*ports.OperationReviewCurrentProjectionV5)) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	change(&r.value)
	var err error
	if r.value.Quorum != nil {
		q, sealErr := ports.SealOperationReviewQuorumCurrentProjectionV5(*r.value.Quorum, now)
		err = sealErr
		r.value.Quorum = &q
	} else {
		n, sealErr := ports.SealOperationReviewPolicyNotRequiredCurrentProjectionV5(*r.value.PolicyNotRequired, now)
		err = sealErr
		r.value.PolicyNotRequired = &n
	}
	if err != nil {
		t.Fatal(err)
	}
	r.value, err = ports.SealOperationReviewCurrentProjectionV5(r.value, now)
	if err != nil {
		t.Fatal(err)
	}
}

func newOperationReviewAuthorizationFixtureV5(t *testing.T, basis ports.OperationReviewAuthorizationBasisV5) (*operationFixtureV3, kernel.OperationReviewAuthorizationGatewayV5, *fakes.OperationReviewAuthorizationStoreV5, *operationReviewReaderV5) {
	t.Helper()
	fixture := newOperationFixtureV3(t)
	snapshot := fixture.current.snapshot
	intentDigest, err := fixture.intent.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	expires := fixture.now.Add(30 * time.Second).UnixNano()
	tenant := fixture.intent.Operation.ExecutionScope.Identity.TenantID
	ref := func(id string, digest core.Digest) ports.OperationGovernanceFactRefV3 {
		return ports.OperationGovernanceFactRefV3{Ref: id, Revision: 1, Digest: digest, ExpiresUnixNano: expires}
	}
	caseRef := ports.OperationReviewCaseRefV5{TenantID: tenant, ID: fixture.intent.Review.CaseRef, Revision: 1, Digest: core.DigestBytes([]byte("case-v5")), ExpiresUnixNano: expires}
	target := ports.OperationReviewTargetRefV4{Ref: fixture.intent.Target, Revision: fixture.intent.Review.CandidateRevision, Digest: fixture.intent.Review.CandidateDigest}
	var projection ports.OperationReviewCurrentProjectionV5
	if basis == ports.OperationReviewBasisPolicyNotRequiredV5 {
		n, sealErr := ports.SealOperationReviewPolicyNotRequiredCurrentProjectionV5(ports.OperationReviewPolicyNotRequiredCurrentProjectionV5{Operation: fixture.intent.Operation, IntentID: fixture.intent.ID, IntentRevision: fixture.intent.Revision, IntentDigest: intentDigest, PayloadSchema: fixture.intent.Payload.Schema, PayloadDigest: fixture.intent.Payload.ContentDigest, PayloadRevision: fixture.intent.PayloadRevision, Target: target, Case: caseRef, BypassDecision: ports.OperationReviewBypassDecisionRefV5{TenantID: tenant, ID: "bypass-v5", Revision: 1, Digest: core.DigestBytes([]byte("bypass-v5")), ExpiresUnixNano: expires}, PolicyCurrentProjection: ref("review-policy-current-v5", fixture.intent.Review.PolicyDigest), PolicyDecisionRef: ref("policy-decision-v5", core.DigestBytes([]byte("policy-decision-v5"))), ScopeRef: snapshot.CurrentScope, BindingRef: snapshot.Binding, ActorAuthorityRef: snapshot.Authority, Current: true, CurrentnessDigest: core.DigestBytes([]byte("not-required-current-v5")), CheckedUnixNano: fixture.now.UnixNano(), ExpiresUnixNano: expires}, fixture.now)
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		projection = ports.OperationReviewCurrentProjectionV5{Basis: basis, PolicyNotRequired: &n}
	} else {
		evidence := []ports.EvidenceRecordRefV2{{LedgerScopeDigest: core.DigestBytes([]byte("quorum-ledger-v5")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("quorum-evidence-v5"))}}
		quorumInput := ports.OperationReviewQuorumCurrentProjectionV5{Operation: fixture.intent.Operation, IntentID: fixture.intent.ID, IntentRevision: fixture.intent.Revision, IntentDigest: intentDigest, PayloadSchema: fixture.intent.Payload.Schema, PayloadDigest: fixture.intent.Payload.ContentDigest, PayloadRevision: fixture.intent.PayloadRevision, Target: target, Case: caseRef, Panel: ports.OperationReviewPanelRefV5{TenantID: tenant, ID: "panel-v5", Revision: 1, Digest: core.DigestBytes([]byte("panel-v5")), ExpiresUnixNano: expires}, QuorumDecision: ports.OperationReviewQuorumDecisionRefV5{TenantID: tenant, ID: "quorum-v5", Revision: 1, Digest: core.DigestBytes([]byte("quorum-v5")), ExpiresUnixNano: expires}, Verdict: ports.OperationReviewVerdictRefV5{TenantID: tenant, ID: "verdict-v5", Revision: 1, Digest: core.DigestBytes([]byte("verdict-v5")), ExpiresUnixNano: expires}, QuorumPolicy: ref("quorum-policy-v5", fixture.intent.Review.PolicyDigest), ReviewerSetDigest: core.DigestBytes([]byte("reviewer-set-v5")), AcceptCount: 2, Threshold: 2, SatisfiedRoleCounts: []ports.OperationReviewRoleCountV5{{Role: "security", Count: 1, Required: 1}}, ReviewerAuthorityRefs: []ports.OperationGovernanceFactRefV3{ref("reviewer-authority-a-v5", core.DigestBytes([]byte("reviewer-authority-a-v5"))), ref("reviewer-authority-b-v5", core.DigestBytes([]byte("reviewer-authority-b-v5")))}, BindingRefs: []ports.OperationGovernanceFactRefV3{ref("review-binding-v5", core.DigestBytes([]byte("review-binding-v5")))}, ScopeRef: snapshot.CurrentScope, DecisionEvidence: evidence, Basis: basis, Current: true, CurrentnessDigest: core.DigestBytes([]byte("quorum-current-v5")), CheckedUnixNano: fixture.now.UnixNano(), ExpiresUnixNano: expires}
		if basis == ports.OperationReviewBasisConditionalQuorumSatisfiedV5 {
			quorumInput.Satisfaction = &ports.OperationReviewConditionSatisfactionV4{Fact: ref("conditional-satisfaction-v5", core.DigestBytes([]byte("conditional-satisfaction-v5"))), ConditionsDigest: core.DigestBytes([]byte("conditions-v5")), Evidence: []ports.EvidenceRecordRefV2{{LedgerScopeDigest: core.DigestBytes([]byte("condition-ledger-v5")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("condition-evidence-v5"))}}}
		}
		q, sealErr := ports.SealOperationReviewQuorumCurrentProjectionV5(quorumInput, fixture.now)
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		projection = ports.OperationReviewCurrentProjectionV5{Basis: basis, Quorum: &q}
	}
	projection, err = ports.SealOperationReviewCurrentProjectionV5(projection, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	reader := &operationReviewReaderV5{value: projection}
	store := fakes.NewOperationReviewAuthorizationStoreV5(func() time.Time { return fixture.now })
	gateway := kernel.OperationReviewAuthorizationGatewayV5{Facts: store, Effects: fixture.store, Governance: fixture.current, Reviews: reader, Clock: func() time.Time { return fixture.now }}
	return fixture, gateway, store, reader
}

func operationReviewAuthorizationRequestV5(f *operationFixtureV3, id string, basis ports.OperationReviewAuthorizationBasisV5) ports.CreateOperationReviewAuthorizationRequestV5 {
	return ports.CreateOperationReviewAuthorizationRequestV5{AuthorizationID: id, Operation: f.intent.Operation, EffectID: f.intent.ID, ExpectedEffectRevision: f.accepted.Revision, Basis: basis, RequestedTTL: 20 * time.Second}
}

func setReviewV5Expiry(p *ports.OperationReviewCurrentProjectionV5, expires int64) {
	if p.Quorum != nil {
		p.Quorum.ExpiresUnixNano = expires
		p.Quorum.Case.ExpiresUnixNano = expires
		p.Quorum.Panel.ExpiresUnixNano = expires
		p.Quorum.QuorumDecision.ExpiresUnixNano = expires
		p.Quorum.Verdict.ExpiresUnixNano = expires
		p.Quorum.QuorumPolicy.ExpiresUnixNano = expires
		p.Quorum.ScopeRef.ExpiresUnixNano = expires
		for i := range p.Quorum.ReviewerAuthorityRefs {
			p.Quorum.ReviewerAuthorityRefs[i].ExpiresUnixNano = expires
		}
		for i := range p.Quorum.BindingRefs {
			p.Quorum.BindingRefs[i].ExpiresUnixNano = expires
		}
	}
}
func cloneReviewCurrentProjectionV5(p ports.OperationReviewCurrentProjectionV5) ports.OperationReviewCurrentProjectionV5 {
	if p.Quorum != nil {
		q := *p.Quorum
		q.SatisfiedRoleCounts = append([]ports.OperationReviewRoleCountV5{}, q.SatisfiedRoleCounts...)
		q.ReviewerAuthorityRefs = append([]ports.OperationGovernanceFactRefV3{}, q.ReviewerAuthorityRefs...)
		q.BindingRefs = append([]ports.OperationGovernanceFactRefV3{}, q.BindingRefs...)
		q.DecisionEvidence = append([]ports.EvidenceRecordRefV2{}, q.DecisionEvidence...)
		if q.Satisfaction != nil {
			satisfaction := *q.Satisfaction
			satisfaction.Evidence = append([]ports.EvidenceRecordRefV2{}, satisfaction.Evidence...)
			q.Satisfaction = &satisfaction
		}
		p.Quorum = &q
	}
	if p.PolicyNotRequired != nil {
		n := *p.PolicyNotRequired
		p.PolicyNotRequired = &n
	}
	return p
}
