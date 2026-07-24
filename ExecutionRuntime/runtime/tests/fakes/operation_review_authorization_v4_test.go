package fakes_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationReviewAuthorizationV4AcceptedLostReplyConcurrentAndCompatibility(t *testing.T) {
	fixture, gateway, store, _ := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
	store.LoseNextCreateReplyV4()
	request := operationReviewAuthorizationRequestV4(fixture, "authorization-1")
	fact, err := gateway.CreateOperationReviewAuthorizationV4(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if store.CreateCommitCountV4() != 1 {
		t.Fatalf("lost create reply committed %d facts", store.CreateCommitCountV4())
	}
	legacy, err := fact.CompatibilityProjectionV3(fixture.now)
	if err != nil || legacy.Verdict.Ref != fact.Review.Verdict.Ref || legacy.Satisfaction != nil {
		t.Fatalf("V3 compatibility projection drifted: %+v err=%v", legacy, err)
	}

	const workers = 32
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			current, callErr := gateway.CreateOperationReviewAuthorizationV4(context.Background(), request)
			if callErr == nil && current.Digest != fact.Digest {
				callErr = core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "concurrent replay returned another Authorization")
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
	if store.CreateCommitCountV4() != 1 {
		t.Fatalf("same request linearized %d Facts", store.CreateCommitCountV4())
	}
	conflict := request
	conflict.RequestedTTL++
	if _, err := gateway.CreateOperationReviewAuthorizationV4(context.Background(), conflict); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same ID changed request must conflict: %v", err)
	}
}

func TestOperationReviewAuthorizationV4OnlySealedBasesAuthorize(t *testing.T) {
	invalid := []ports.OperationReviewAuthorizationBasisV4{"pending", "rejected", "expired", "revoked", "superseded", "unknown", "conditional"}
	for _, basis := range invalid {
		t.Run(string(basis), func(t *testing.T) {
			fixture, gateway, store, review := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
			review.mutate(func(value *ports.OperationReviewCurrentProjectionV4) { value.Basis = basis })
			if _, err := gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(fixture, "authorization-invalid")); err == nil {
				t.Fatal("non-authorizing Review basis produced an Authorization")
			}
			if store.CreateCommitCountV4() != 0 {
				t.Fatal("invalid basis changed the Fact owner")
			}
		})
	}
}

func TestOperationReviewAuthorizationV4ConditionalRequiresExactSatisfactionEvidence(t *testing.T) {
	fixture, gateway, store, review := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisConditionalSatisfiedV4)
	review.mutate(func(value *ports.OperationReviewCurrentProjectionV4) { value.Satisfaction = nil })
	if _, err := gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(fixture, "authorization-unsatisfied")); !core.HasReason(err, core.ReasonReviewConditionUnsatisfied) {
		t.Fatalf("conditional without satisfaction was not rejected: %v", err)
	}
	if store.CreateCommitCountV4() != 0 {
		t.Fatal("unsatisfied conditional changed the Fact owner")
	}

	fixture, gateway, _, review = newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisConditionalSatisfiedV4)
	fact, err := gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(fixture, "authorization-satisfied"))
	if err != nil {
		t.Fatal(err)
	}
	if fact.Review.Satisfaction == nil {
		t.Fatal("conditional Authorization lost satisfaction provenance")
	}
	review.mutateAndSeal(t, fixture.now, func(value *ports.OperationReviewCurrentProjectionV4) {
		value.Satisfaction.Evidence[0].RecordDigest = core.DigestBytes([]byte("new-condition-evidence"))
	})
	if _, err := gateway.InspectCurrentOperationReviewAuthorizationV4(context.Background(), fixture.intent.Operation, fixture.intent.ID, fact.ID); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("changed satisfaction evidence did not stale old Authorization: %v", err)
	}
}

func TestOperationReviewAuthorizationV4EveryCurrentDriftRequiresNewAuthorization(t *testing.T) {
	tests := []struct {
		name   string
		change func(*testing.T, *operationFixtureV3, *operationReviewReaderV4)
	}{
		{"target", func(t *testing.T, f *operationFixtureV3, r *operationReviewReaderV4) {
			r.mutateAndSeal(t, f.now, func(p *ports.OperationReviewCurrentProjectionV4) {
				p.Target.Digest = core.DigestBytes([]byte("new-target"))
			})
		}},
		{"case", func(t *testing.T, f *operationFixtureV3, r *operationReviewReaderV4) {
			r.mutateAndSeal(t, f.now, func(p *ports.OperationReviewCurrentProjectionV4) { p.Case.Revision++ })
		}},
		{"verdict", func(t *testing.T, f *operationFixtureV3, r *operationReviewReaderV4) {
			r.mutateAndSeal(t, f.now, func(p *ports.OperationReviewCurrentProjectionV4) {
				p.Verdict.Digest = core.DigestBytes([]byte("new-verdict"))
			})
		}},
		{"review-policy", func(t *testing.T, f *operationFixtureV3, r *operationReviewReaderV4) {
			r.mutateAndSeal(t, f.now, func(p *ports.OperationReviewCurrentProjectionV4) { p.Policy.Revision++ })
		}},
		{"evidence", func(t *testing.T, f *operationFixtureV3, r *operationReviewReaderV4) {
			r.mutateAndSeal(t, f.now, func(p *ports.OperationReviewCurrentProjectionV4) {
				p.DecisionEvidence[0].RecordDigest = core.DigestBytes([]byte("new-evidence"))
			})
		}},
		{"payload", func(t *testing.T, f *operationFixtureV3, r *operationReviewReaderV4) {
			r.mutateAndSeal(t, f.now, func(p *ports.OperationReviewCurrentProjectionV4) {
				p.PayloadDigest = core.DigestBytes([]byte("new-payload"))
			})
		}},
		{"intent-revision", func(t *testing.T, f *operationFixtureV3, r *operationReviewReaderV4) {
			r.mutateAndSeal(t, f.now, func(p *ports.OperationReviewCurrentProjectionV4) { p.IntentRevision++ })
		}},
		{"currentness", func(t *testing.T, f *operationFixtureV3, r *operationReviewReaderV4) {
			r.mutateAndSeal(t, f.now, func(p *ports.OperationReviewCurrentProjectionV4) {
				p.CurrentnessDigest = core.DigestBytes([]byte("new-currentness"))
			})
		}},
		{"authority", func(_ *testing.T, f *operationFixtureV3, _ *operationReviewReaderV4) {
			f.current.mutate(func(p *ports.OperationGovernanceSnapshotV3) {
				p.Authority.Digest = core.DigestBytes([]byte("new-authority"))
			})
		}},
		{"scope", func(_ *testing.T, f *operationFixtureV3, _ *operationReviewReaderV4) {
			f.current.mutate(func(p *ports.OperationGovernanceSnapshotV3) { p.CurrentScope.Revision++ })
		}},
		{"binding", func(_ *testing.T, f *operationFixtureV3, _ *operationReviewReaderV4) {
			f.current.mutate(func(p *ports.OperationGovernanceSnapshotV3) {
				p.Binding.Digest = core.DigestBytes([]byte("new-binding"))
			})
		}},
		{"fence-capability", func(_ *testing.T, f *operationFixtureV3, _ *operationReviewReaderV4) {
			f.current.mutate(func(p *ports.OperationGovernanceSnapshotV3) {
				p.CapabilityGrantDigest = core.DigestBytes([]byte("new-capability"))
			})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, gateway, _, review := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
			fact, err := gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(fixture, "authorization-drift"))
			if err != nil {
				t.Fatal(err)
			}
			test.change(t, fixture, review)
			if _, err := gateway.InspectCurrentOperationReviewAuthorizationV4(context.Background(), fixture.intent.Operation, fixture.intent.ID, fact.ID); err == nil {
				t.Fatal("old Authorization remained current after drift")
			}
			// Create replay remains an historical idempotency recovery, not a current grant.
			if replay, err := gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(fixture, "authorization-drift")); err != nil || replay.Digest != fact.Digest {
				t.Fatalf("historical create replay was not inspectable: fact=%+v err=%v", replay, err)
			}
		})
	}
}

func TestOperationReviewAuthorizationV4VerdictDriftCreatesNewIDAndNeverRevivesOldFact(t *testing.T) {
	fixture, gateway, _, review := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
	oldFact, err := gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(fixture, "authorization-old"))
	if err != nil {
		t.Fatal(err)
	}
	review.mutateAndSeal(t, fixture.now, func(value *ports.OperationReviewCurrentProjectionV4) {
		value.Verdict.Revision++
		value.Verdict.Digest = core.DigestBytes([]byte("replacement-verdict"))
		value.CurrentnessDigest = core.DigestBytes([]byte("replacement-currentness"))
	})
	if _, err := gateway.InspectCurrentOperationReviewAuthorizationV4(context.Background(), fixture.intent.Operation, fixture.intent.ID, oldFact.ID); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("old Verdict Authorization remained executable: %v", err)
	}
	newFact, err := gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(fixture, "authorization-new"))
	if err != nil {
		t.Fatal(err)
	}
	if newFact.Digest == oldFact.Digest || newFact.Review.Verdict == oldFact.Review.Verdict {
		t.Fatal("replacement Verdict did not produce a distinct immutable Authorization")
	}
	if _, err := oldFact.CompatibilityProjectionV3(fixture.now); err != nil {
		// Historical projection can be represented, but only Gateway current
		// Inspect decides whether it may be consumed. The call itself remaining
		// structurally valid is intentional compatibility behavior.
		t.Fatal(err)
	}
}

func TestOperationReviewAuthorizationV4CurrentAndTTLBoundariesFailClosed(t *testing.T) {
	fixture, gateway, store, review := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
	review.mutate(func(value *ports.OperationReviewCurrentProjectionV4) { value.Current = false })
	if _, err := gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(fixture, "authorization-inactive")); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("inactive Review projection was accepted: %v", err)
	}
	if store.CreateCommitCountV4() != 0 {
		t.Fatal("inactive Review projection changed Fact owner")
	}

	fixture, gateway, store, review = newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
	review.mu.Lock()
	exactExpiry := review.value.ExpiresUnixNano
	review.mu.Unlock()
	gateway.Clock = func() time.Time { return time.Unix(0, exactExpiry) }
	if _, err := gateway.CreateOperationReviewAuthorizationV4(context.Background(), operationReviewAuthorizationRequestV4(fixture, "authorization-expired")); err == nil {
		t.Fatal("exact Review TTL boundary authorized an operation")
	}
	if store.CreateCommitCountV4() != 0 {
		t.Fatal("expired Review projection changed Fact owner")
	}
}

func TestOperationReviewAuthorizationV4CASLostReplyAndConformanceAuthorityBoundary(t *testing.T) {
	fixture, gateway, store, _ := newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisNotRequiredV4)
	request := operationReviewAuthorizationRequestV4(fixture, "authorization-cas")
	fact, err := gateway.CreateOperationReviewAuthorizationV4(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	forged := fact
	forged.Intent.Target = "other/target"
	if _, err := store.CreateOperationReviewAuthorizationV4(context.Background(), forged); err == nil {
		t.Fatal("same digest with changed intent/review relation was treated as idempotent")
	}
	changed := fact
	changed.Review.Verdict.Revision++
	changed.Review.Verdict.Digest = core.DigestBytes([]byte("changed-verdict-content"))
	changed.Review.CurrentnessDigest = core.DigestBytes([]byte("changed-currentness-content"))
	changed.Review, err = ports.SealOperationReviewCurrentProjectionV4(changed.Review, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	changed, err = ports.SealOperationReviewAuthorizationFactV4(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateOperationReviewAuthorizationV4(context.Background(), changed); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same ID changed valid Authorization must conflict: %v", err)
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
	store.LoseNextCASReplyV4()
	if _, err := store.CompareAndSwapOperationReviewAuthorizationV4(context.Background(), ports.OperationReviewAuthorizationCASRequestV4{ExpectedRevision: fact.Revision, Next: next}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("injected CAS reply loss was not observed: %v", err)
	}
	recovered, err := store.InspectOperationReviewAuthorizationV4(context.Background(), fact.ID)
	if err != nil || recovered.Digest != next.Digest {
		t.Fatalf("CAS reply loss did not recover by Inspect: %+v err=%v", recovered, err)
	}
	if _, err := gateway.InspectCurrentOperationReviewAuthorizationV4(context.Background(), fixture.intent.Operation, fixture.intent.ID, fact.ID); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("revoked Authorization remained usable: %v", err)
	}

	fixture, gateway, _, _ = newOperationReviewAuthorizationFixtureV4(t, ports.OperationReviewBasisAcceptedV4)
	report, err := conformance.CheckOperationReviewAuthorizationV4(context.Background(), conformance.OperationReviewAuthorizationCaseV4{Gateway: gateway, Request: operationReviewAuthorizationRequestV4(fixture, "authorization-conformance")})
	if err != nil {
		t.Fatal(err)
	}
	if !report.RuntimeOwnerObserved || !report.CurrentInspectObserved || report.AuthorizationIsPermit || report.ReviewOwnedAuthorization || report.ProductionClaimEligible {
		t.Fatalf("conformance report exceeded its authority: %+v", report)
	}
}

type operationReviewReaderV4 struct {
	mu    sync.Mutex
	value ports.OperationReviewCurrentProjectionV4
}

func (r *operationReviewReaderV4) InspectOperationReviewCurrentV4(_ context.Context, _ ports.OperationEffectIntentV3) (ports.OperationReviewCurrentProjectionV4, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.value, nil
}

func (r *operationReviewReaderV4) mutate(change func(*ports.OperationReviewCurrentProjectionV4)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	change(&r.value)
}

func (r *operationReviewReaderV4) mutateAndSeal(t *testing.T, now time.Time, change func(*ports.OperationReviewCurrentProjectionV4)) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	change(&r.value)
	sealed, err := ports.SealOperationReviewCurrentProjectionV4(r.value, now)
	if err != nil {
		t.Fatal(err)
	}
	r.value = sealed
}

func newOperationReviewAuthorizationFixtureV4(t *testing.T, basis ports.OperationReviewAuthorizationBasisV4) (*operationFixtureV3, kernel.OperationReviewAuthorizationGatewayV4, *fakes.OperationReviewAuthorizationStoreV4, *operationReviewReaderV4) {
	t.Helper()
	fixture := newOperationFixtureV3(t)
	snapshot := fixture.current.snapshot
	intentDigest, err := fixture.intent.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	evidence := []ports.EvidenceRecordRefV2{{LedgerScopeDigest: core.DigestBytes([]byte("review-ledger")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("review-evidence"))}}
	review := ports.OperationReviewCurrentProjectionV4{
		Operation: fixture.intent.Operation, IntentID: fixture.intent.ID, IntentRevision: fixture.intent.Revision, IntentDigest: intentDigest,
		PayloadSchema: fixture.intent.Payload.Schema, PayloadDigest: fixture.intent.Payload.ContentDigest, PayloadRevision: fixture.intent.PayloadRevision,
		Target: ports.OperationReviewTargetRefV4{Ref: fixture.intent.Target, Revision: fixture.intent.Review.CandidateRevision, Digest: fixture.intent.Review.CandidateDigest},
		Case:   snapshot.Review.Case, Verdict: snapshot.Review.Verdict, Basis: basis,
		Policy:            ports.OperationGovernanceFactRefV3{Ref: "review-policy-operation", Revision: 1, Digest: fixture.intent.Review.PolicyDigest, ExpiresUnixNano: fixture.now.Add(30 * time.Second).UnixNano()},
		ReviewerAuthority: snapshot.Review.ReviewerAuthority, Scope: snapshot.CurrentScope, Binding: snapshot.Binding,
		DecisionEvidence: evidence, Current: true, CurrentnessDigest: core.DigestBytes([]byte("review-currentness")), ExpiresUnixNano: fixture.now.Add(30 * time.Second).UnixNano(),
	}
	if basis == ports.OperationReviewBasisConditionalSatisfiedV4 {
		review.Satisfaction = &ports.OperationReviewConditionSatisfactionV4{
			Fact:             ports.OperationGovernanceFactRefV3{Ref: "review-satisfaction-operation", Revision: 1, Digest: core.DigestBytes([]byte("review-satisfaction")), ExpiresUnixNano: fixture.now.Add(25 * time.Second).UnixNano()},
			ConditionsDigest: core.DigestBytes([]byte("review-conditions")),
			Evidence:         []ports.EvidenceRecordRefV2{{LedgerScopeDigest: core.DigestBytes([]byte("condition-ledger")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("condition-evidence"))}},
		}
	}
	review, err = ports.SealOperationReviewCurrentProjectionV4(review, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	reviewReader := &operationReviewReaderV4{value: review}
	store := fakes.NewOperationReviewAuthorizationStoreV4(func() time.Time { return fixture.now })
	gateway := kernel.OperationReviewAuthorizationGatewayV4{Facts: store, Effects: fixture.store, Governance: fixture.current, Reviews: reviewReader, Clock: func() time.Time { return fixture.now }}
	return fixture, gateway, store, reviewReader
}

func operationReviewAuthorizationRequestV4(fixture *operationFixtureV3, id string) ports.CreateOperationReviewAuthorizationRequestV4 {
	return ports.CreateOperationReviewAuthorizationRequestV4{AuthorizationID: id, Operation: fixture.intent.Operation, EffectID: fixture.intent.ID, ExpectedEffectRevision: fixture.accepted.Revision, RequestedTTL: 20 * time.Second}
}
