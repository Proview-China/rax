package control_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewGovernanceV2EffectWriteAheadThenCaseVerdictPermitAndRevoke(t *testing.T) {
	t.Parallel()
	fixture := newReviewGovernanceFixtureV2(t, time.Unix(40_000, 0), ports.ReviewInvocationHuman, false)
	originalIntent := fixture.effect.Intent
	fixture.now = fixture.now.Add(time.Second)
	caseFact, err := fixture.review.CreateCase(context.Background(), control.CreateGovernedReviewCaseRequestV2{EffectID: fixture.effect.Intent.ID, ExpectedEffectRevision: fixture.effect.Revision, Candidate: fixture.candidate})
	if err != nil {
		t.Fatal(err)
	}
	if caseFact.UpdatedUnixNano != fixture.now.UnixNano() || caseFact.Candidate.RequestedUnixNano >= caseFact.UpdatedUnixNano {
		t.Fatalf("case must use gateway time after the write-ahead Effect: %+v", caseFact)
	}
	fixture.now = fixture.now.Add(time.Second)
	observation := acceptedObservationV2(t, caseFact, fixture.now)
	decided, err := fixture.review.Decide(context.Background(), control.DecideGovernedReviewRequestV2{CaseID: caseFact.Candidate.ID, ExpectedCaseRevision: caseFact.Revision, Observation: observation, RequestedTTL: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	for _, drift := range []func(*ports.EffectIntentV2){
		func(v *ports.EffectIntentV2) { v.Revision++ },
		func(v *ports.EffectIntentV2) {
			v.Payload.Inline = []byte("new-effect-payload")
			v.Payload.Length = uint64(len(v.Payload.Inline))
			v.Payload.ContentDigest = core.DigestBytes(v.Payload.Inline)
		},
		func(v *ports.EffectIntentV2) { v.Target = "provider://new-subject" },
	} {
		changed := originalIntent
		drift(&changed)
		if _, err := control.BuildDispatchReviewProjectionV2(decided.Case, decided.Verdict, nil, changed, fixture.now); !core.HasReason(err, core.ReasonReviewCandidateConflict) {
			t.Fatalf("old verdict cannot authorize a changed Effect revision, payload or subject: %v", err)
		}
	}
	driftedVerdict := decided.Verdict
	driftedVerdict.DecisionEvidence = append([]ports.ReviewEvidenceRefV2{}, decided.Verdict.DecisionEvidence...)
	driftedVerdict.DecisionEvidence[0].Digest = controlEffectDigestV2(t, "changed-verdict-evidence")
	if err := driftedVerdict.Validate(); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("verdict evidence content cannot retain an old canonical digest: %v", err)
	}
	accepted := fixture.effect
	accepted.State, accepted.Revision, accepted.UpdatedUnixNano = control.EffectAccepted, fixture.effect.Revision+1, fixture.now.UnixNano()
	accepted, err = fixture.effects.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: fixture.effect.Revision, Next: accepted})
	if err != nil {
		t.Fatal(err)
	}
	originalDigest, _ := originalIntent.DigestV2()
	acceptedDigest, _ := accepted.Intent.DigestV2()
	if originalIntent.ID != accepted.Intent.ID || originalIntent.Revision != accepted.Intent.Revision || originalDigest != acceptedDigest {
		t.Fatal("Review verdict must not mutate the canonical write-ahead Effect intent")
	}
	issued, err := fixture.dispatch.Issue(context.Background(), control.IssueGovernedDispatchRequestV2{EffectID: accepted.Intent.ID, ExpectedEffectRevision: accepted.Revision, PermitID: "permit-reviewed", AttemptID: "attempt-reviewed", PermitTTL: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if issued.Permit.Permit.ReviewVerdictDigest == "" || issued.Permit.Permit.ReviewVerdictRevision != decided.Verdict.Revision {
		t.Fatalf("permit must bind the current authoritative verdict: %+v", issued.Permit.Permit)
	}
	fixture.now = fixture.now.Add(time.Second)
	revoked := decided.Verdict
	revoked.State, revoked.Revision, revoked.UpdatedUnixNano, revoked.InvalidationReason = ports.ReviewVerdictRevoked, decided.Verdict.Revision+1, fixture.now.UnixNano(), core.ReasonReviewVerdictStale
	if _, err := fixture.reviewFacts.CompareAndSwapReviewVerdict(context.Background(), ports.ReviewVerdictCASRequestV2{VerdictID: revoked.ID, ExpectedRevision: decided.Verdict.Revision, Next: revoked}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.dispatch.Begin(context.Background(), control.BeginGovernedDispatchRequestV2{EffectID: accepted.Intent.ID, ExpectedEffectRevision: issued.Effect.Revision, PermitID: issued.Permit.Permit.ID, ExpectedPermitRevision: issued.Permit.Revision}); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("verdict revoked after Issue must block final Begin: %v", err)
	}
	permit, err := fixture.effects.InspectDispatchPermit(context.Background(), issued.Permit.Permit.ID)
	if err != nil || permit.State != control.DispatchPermitIssued {
		t.Fatalf("failed Begin must leave permit unconsumed: %v %+v", err, permit)
	}
}

func TestReviewGovernanceV2CurrentVerdictAllowsNormalIssueThenBegin(t *testing.T) {
	t.Parallel()
	fixture := decidedAcceptedReviewFixtureV2(t, time.Unix(40_500, 0))
	issued, err := fixture.dispatch.Issue(context.Background(), control.IssueGovernedDispatchRequestV2{EffectID: fixture.effect.Intent.ID, ExpectedEffectRevision: fixture.effect.Revision, PermitID: "permit-normal-reviewed", AttemptID: "attempt-normal-reviewed", PermitTTL: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	fixture.now = fixture.now.Add(time.Second)
	begun, err := fixture.dispatch.Begin(context.Background(), control.BeginGovernedDispatchRequestV2{EffectID: fixture.effect.Intent.ID, ExpectedEffectRevision: issued.Effect.Revision, PermitID: issued.Permit.Permit.ID, ExpectedPermitRevision: issued.Permit.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if begun.State != control.DispatchPermitBegun {
		t.Fatalf("current Review authority must allow the normal Begin path: %+v", begun)
	}
}

func TestReviewGovernanceV2CreateCaseTrustsOnlyPersistedPredispatchEffect(t *testing.T) {
	t.Parallel()
	fixture := newReviewGovernanceFixtureV2(t, time.Unix(41_000, 0), ports.ReviewInvocationHuman, false)
	fixture.now = fixture.now.Add(time.Second)
	if _, err := fixture.review.CreateCase(context.Background(), control.CreateGovernedReviewCaseRequestV2{EffectID: "missing-effect", ExpectedEffectRevision: fixture.effect.Revision, Candidate: fixture.candidate}); err == nil {
		t.Fatal("an unpersisted caller-supplied intent must never create a case")
	}
	if _, err := fixture.review.CreateCase(context.Background(), control.CreateGovernedReviewCaseRequestV2{EffectID: fixture.effect.Intent.ID, ExpectedEffectRevision: fixture.effect.Revision + 1, Candidate: fixture.candidate}); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("stale Effect fact revision must fail: %v", err)
	}
	caseFact, err := fixture.review.CreateCase(context.Background(), control.CreateGovernedReviewCaseRequestV2{EffectID: fixture.effect.Intent.ID, ExpectedEffectRevision: fixture.effect.Revision, Candidate: fixture.candidate})
	if err != nil {
		t.Fatal(err)
	}
	replayedCase := caseFact
	replayedCase.Candidate.Evidence = []ports.ReviewEvidenceRefV2{}
	if replayed, err := fixture.reviewFacts.CreateReviewCase(context.Background(), replayedCase); err != nil || replayed.CandidateDigest != caseFact.CandidateDigest {
		t.Fatalf("Case store round-trip must preserve canonical digest across nil/empty: %v %+v", err, replayed)
	}
	differentCandidate := fixture.candidate
	differentCandidate.Policy.Digest = controlEffectDigestV2(t, "different-review-policy")
	differentCase, err := control.NewPendingReviewCaseV2(differentCandidate, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.reviewFacts.CreateReviewCase(context.Background(), differentCase); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same Case id with different candidate must conflict: %v", err)
	}
	dispatching := fixture.effect
	dispatching.State, dispatching.Revision, dispatching.DispatchPermitID, dispatching.UpdatedUnixNano = control.EffectDispatchIntent, fixture.effect.Revision+1, "permit-existing", fixture.now.UnixNano()
	// A structurally incomplete direct transition is intentionally rejected by
	// the Effect store; replace the reader only to prove the Review gateway's
	// own pre-dispatch guard is independent of caller booleans.
	fixture.review.Effects = staticEffectFactReaderV2{fact: dispatching}
	if _, err := fixture.review.CreateCase(context.Background(), control.CreateGovernedReviewCaseRequestV2{EffectID: fixture.effect.Intent.ID, ExpectedEffectRevision: dispatching.Revision, Candidate: fixture.candidate}); !core.HasReason(err, core.ReasonInvalidTransition) {
		t.Fatalf("post-begin or dispatch-intent Effect cannot enter review: %v", err)
	}
}

func TestReviewVerdictV2ExpireBoundaryClockRegressionAndLostCASReply(t *testing.T) {
	t.Parallel()
	fixture := decidedAcceptedReviewFixtureV2(t, time.Unix(41_500, 0))
	caseFact, err := fixture.reviewFacts.InspectReviewCase(context.Background(), fixture.candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := fixture.reviewFacts.InspectReviewVerdict(context.Background(), caseFact.VerdictID)
	if err != nil {
		t.Fatal(err)
	}
	expired := verdict
	expired.State, expired.Revision, expired.UpdatedUnixNano, expired.InvalidationReason = ports.ReviewVerdictExpired, verdict.Revision+1, fixture.now.UnixNano(), core.ReasonReviewVerdictStale
	if _, err := fixture.reviewFacts.CompareAndSwapReviewVerdict(context.Background(), ports.ReviewVerdictCASRequestV2{VerdictID: verdict.ID, ExpectedRevision: verdict.Revision, Next: expired}); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("verdict cannot expire before exact TTL boundary: %v", err)
	}
	rolledBack := verdict
	rolledBack.State, rolledBack.Revision, rolledBack.UpdatedUnixNano, rolledBack.InvalidationReason = ports.ReviewVerdictRevoked, verdict.Revision+1, verdict.UpdatedUnixNano, core.ReasonReviewVerdictStale
	fixture.now = time.Unix(0, verdict.UpdatedUnixNano-1)
	if _, err := fixture.reviewFacts.CompareAndSwapReviewVerdict(context.Background(), ports.ReviewVerdictCASRequestV2{VerdictID: verdict.ID, ExpectedRevision: verdict.Revision, Next: rolledBack}); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("verdict invalidation must fail closed on clock rollback: %v", err)
	}
	fixture.now = time.Unix(0, verdict.ExpiresUnixNano)
	expired.UpdatedUnixNano = fixture.now.UnixNano()
	fixture.reviewFacts.LoseNextCASReply()
	if _, err := fixture.reviewFacts.CompareAndSwapReviewVerdict(context.Background(), ports.ReviewVerdictCASRequestV2{VerdictID: verdict.ID, ExpectedRevision: verdict.Revision, Next: expired}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("exact-boundary expiry CAS reply loss must occur after persistence: %v", err)
	}
	current, err := fixture.reviewFacts.InspectReviewVerdict(context.Background(), verdict.ID)
	if err != nil || current.State != ports.ReviewVerdictExpired || current.DecisionEvidenceDigest != verdict.DecisionEvidenceDigest {
		t.Fatalf("expired verdict must preserve decision and recover by Inspect: %v %+v", err, current)
	}
	current.DecisionEvidence = append([]ports.ReviewEvidenceRefV2{}, current.DecisionEvidence...)
	if replayed, err := fixture.reviewFacts.CompareAndSwapReviewVerdict(context.Background(), ports.ReviewVerdictCASRequestV2{VerdictID: verdict.ID, ExpectedRevision: verdict.Revision, Next: current}); err != nil || replayed.State != ports.ReviewVerdictExpired {
		t.Fatalf("canonical lost expiry replay must be idempotent: %v %+v", err, replayed)
	}
}

func TestReviewGovernanceV2ProjectionRereadsCurrentFactsAndCapsTTL(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name   string
		reason core.ReasonCode
		mutate func(*reviewGovernanceFixtureV2)
	}{
		{name: "policy_revoked", reason: core.ReasonReviewVerdictStale, mutate: func(f *reviewGovernanceFixtureV2) { f.reviewPolicy.fact.Active = false }},
		{name: "actor_revoked", reason: core.ReasonReviewVerdictStale, mutate: func(f *reviewGovernanceFixtureV2) {
			fact := f.authority.facts[f.candidate.ActorAuthority.Ref]
			fact.State = ports.AuthorityFactRevoked
			f.authority.facts[f.candidate.ActorAuthority.Ref] = fact
		}},
		{name: "reviewer_revoked", reason: core.ReasonReviewVerdictStale, mutate: func(f *reviewGovernanceFixtureV2) {
			fact := f.authority.facts[f.candidate.ReviewerAuthority.Ref]
			fact.State = ports.AuthorityFactRevoked
			f.authority.facts[f.candidate.ReviewerAuthority.Ref] = fact
		}},
		{name: "binding_drift", reason: core.ReasonProviderBindingStale, mutate: func(f *reviewGovernanceFixtureV2) { f.bindings.set.Revision++ }},
		{name: "run_scope_revoked", reason: core.ReasonEffectFenceStale, mutate: func(f *reviewGovernanceFixtureV2) { f.scope.fact.State = ports.ExecutionScopeFactRevoked }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := decidedAcceptedReviewFixtureV2(t, time.Unix(42_000, 0))
			testCase.mutate(fixture)
			if _, err := fixture.review.InspectDispatchReview(context.Background(), fixture.candidate.ID); !core.HasReason(err, testCase.reason) {
				t.Fatalf("projection must re-read current authority rather than cache verdict TTL: %v", err)
			}
		})
	}
	fixture := decidedAcceptedReviewFixtureV2(t, time.Unix(42_500, 0))
	// Authority expiry is current mutable state and does not rewrite the
	// immutable Review candidate identity.
	actor := fixture.authority.facts[fixture.candidate.ActorAuthority.Ref]
	actor.ExpiresUnixNano = fixture.now.Add(time.Second).UnixNano()
	fixture.authority.facts[fixture.candidate.ActorAuthority.Ref] = actor
	projection, err := fixture.review.InspectDispatchReview(context.Background(), fixture.candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.ExpiresUnixNano != actor.ExpiresUnixNano {
		t.Fatalf("projection TTL must be capped by freshly read current facts: got %d want %d", projection.ExpiresUnixNano, actor.ExpiresUnixNano)
	}
}

func TestReviewGovernanceV2LostRepliesRecoverByInspectAndDecisionCASLinearizesOnce(t *testing.T) {
	t.Parallel()
	fixture := newReviewGovernanceFixtureV2(t, time.Unix(43_000, 0), ports.ReviewInvocationHuman, false)
	fixture.now = fixture.now.Add(time.Second)
	fixture.reviewFacts.LoseNextCreateReply()
	request := control.CreateGovernedReviewCaseRequestV2{EffectID: fixture.effect.Intent.ID, ExpectedEffectRevision: fixture.effect.Revision, Candidate: fixture.candidate}
	if _, err := fixture.review.CreateCase(context.Background(), request); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("injected Create reply loss must surface after persistence: %v", err)
	}
	caseFact, err := fixture.reviewFacts.InspectReviewCase(context.Background(), fixture.candidate.ID)
	if err != nil || caseFact.State != ports.ReviewCasePending {
		t.Fatalf("lost Create reply must recover by Inspect: %v %+v", err, caseFact)
	}
	fixture.now = fixture.now.Add(time.Second)
	accepted := acceptedObservationV2(t, caseFact, fixture.now)
	rejected := accepted
	rejected.ProposedState = ports.ReviewVerdictRejected
	rejected.Evidence = []ports.ReviewEvidenceRefV2{{Ref: "review-evidence-rejected", Classification: "custom/reviewer-attestation", Digest: controlEffectDigestV2(t, "review-evidence-rejected")}}
	var successes atomic.Int32
	var conflicts atomic.Int32
	var wait sync.WaitGroup
	for _, observation := range []ports.ReviewAttestationObservationV2{accepted, rejected} {
		observation := observation
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := fixture.review.Decide(context.Background(), control.DecideGovernedReviewRequestV2{CaseID: caseFact.Candidate.ID, ExpectedCaseRevision: caseFact.Revision, Observation: observation, RequestedTTL: 10 * time.Second})
			if err == nil {
				successes.Add(1)
				return
			}
			if core.HasReason(err, core.ReasonReviewCandidateConflict) || core.HasReason(err, core.ReasonRevisionConflict) {
				conflicts.Add(1)
				return
			}
			t.Errorf("unexpected concurrent decision error: %v", err)
		}()
	}
	wait.Wait()
	if successes.Load() != 1 || conflicts.Load() != 1 {
		t.Fatalf("opposite decisions must linearize exactly once: success=%d conflict=%d", successes.Load(), conflicts.Load())
	}
	verdict, err := fixture.reviewFacts.InspectReviewVerdict(context.Background(), caseFact.Candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	fixture2 := newReviewGovernanceFixtureV2(t, time.Unix(43_500, 0), ports.ReviewInvocationHuman, false)
	fixture2.now = fixture2.now.Add(time.Second)
	case2, err := fixture2.review.CreateCase(context.Background(), control.CreateGovernedReviewCaseRequestV2{EffectID: fixture2.effect.Intent.ID, ExpectedEffectRevision: fixture2.effect.Revision, Candidate: fixture2.candidate})
	if err != nil {
		t.Fatal(err)
	}
	fixture2.now = fixture2.now.Add(time.Second)
	fixture2.reviewFacts.LoseNextDecisionReply()
	observation2 := acceptedObservationV2(t, case2, fixture2.now)
	if _, err := fixture2.review.Decide(context.Background(), control.DecideGovernedReviewRequestV2{CaseID: case2.Candidate.ID, ExpectedCaseRevision: case2.Revision, Observation: observation2, RequestedTTL: 10 * time.Second}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("injected Decide reply loss must surface after atomic persistence: %v", err)
	}
	recovered, err := fixture2.reviewFacts.InspectReviewVerdict(context.Background(), case2.Candidate.ID)
	if err != nil || recovered.State != ports.ReviewVerdictAccepted {
		t.Fatalf("lost Decide reply must recover by Inspect: %v %+v", err, recovered)
	}
	_ = verdict
}

func TestReviewGovernanceV2ConditionalSatisfactionBindsPermitAndRevocationBlocksBegin(t *testing.T) {
	t.Parallel()
	fixture := newReviewGovernanceFixtureV2(t, time.Unix(44_000, 0), ports.ReviewInvocationHuman, false)
	fixture.now = fixture.now.Add(time.Second)
	caseFact, err := fixture.review.CreateCase(context.Background(), control.CreateGovernedReviewCaseRequestV2{EffectID: fixture.effect.Intent.ID, ExpectedEffectRevision: fixture.effect.Revision, Candidate: fixture.candidate})
	if err != nil {
		t.Fatal(err)
	}
	condition := ports.ReviewConditionV2{ID: "custom/require-owner-proof", Revision: 1, Schema: ports.SchemaRefV2{Namespace: "custom", Name: "condition", Version: "1.0.0", MediaType: "application/octet-stream", ContentDigest: controlEffectDigestV2(t, "condition-schema")}, ConstraintDigest: controlEffectDigestV2(t, "condition-constraint"), SatisfactionOwner: satisfactionOwnerFromFixtureV2(t, fixture), ScopeDigest: fixture.candidate.ActionScopeDigest, Authority: fixture.candidate.ActorAuthority, ExpiresUnixNano: fixture.now.Add(8 * time.Second).UnixNano()}
	fixture.now = fixture.now.Add(time.Second)
	observation := acceptedObservationV2(t, caseFact, fixture.now)
	observation.ProposedState, observation.Conditions = ports.ReviewVerdictConditional, []ports.ReviewConditionV2{condition}
	decided, err := fixture.review.Decide(context.Background(), control.DecideGovernedReviewRequestV2{CaseID: caseFact.Candidate.ID, ExpectedCaseRevision: caseFact.Revision, Observation: observation, RequestedTTL: 12 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	accepted := fixture.effect
	accepted.State, accepted.Revision, accepted.UpdatedUnixNano = control.EffectAccepted, fixture.effect.Revision+1, fixture.now.UnixNano()
	accepted, err = fixture.effects.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: fixture.effect.Revision, Next: accepted})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.review.InspectDispatchReview(context.Background(), fixture.candidate.ID); !core.HasReason(err, core.ReasonReviewConditionUnsatisfied) {
		t.Fatalf("conditional verdict without satisfaction must fail closed: %v", err)
	}
	pending, err := fixture.review.CreateSatisfaction(context.Background(), decided.Verdict.ID, "satisfaction-1")
	if err != nil {
		t.Fatal(err)
	}
	pendingReplay := pending
	pendingReplay.Proofs = []ports.ReviewConditionProofV2{}
	if replayed, err := fixture.reviewFacts.CreateConditionSatisfaction(context.Background(), pendingReplay); err != nil || replayed.ID != pending.ID {
		t.Fatalf("pending satisfaction nil/empty persistence replay must be idempotent: %v %+v", err, replayed)
	}
	changedPending := pendingReplay
	changedPending.Policy.Digest = controlEffectDigestV2(t, "different-satisfaction-policy")
	if _, err := fixture.reviewFacts.CreateConditionSatisfaction(context.Background(), changedPending); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same satisfaction id with changed governed subject must conflict: %v", err)
	}
	fixture.now = fixture.now.Add(time.Second)
	proof := ports.ReviewConditionProofV2{ConditionID: condition.ID, ConditionRevision: condition.Revision, ConstraintDigest: condition.ConstraintDigest, Owner: condition.SatisfactionOwner, ScopeDigest: condition.ScopeDigest, Authority: condition.Authority, Evidence: ports.ReviewEvidenceRefV2{Ref: "condition-proof-1", Classification: "custom/condition-proof", Digest: controlEffectDigestV2(t, "condition-proof-1")}, ExpiresUnixNano: fixture.now.Add(5 * time.Second).UnixNano()}
	for _, testCase := range []struct {
		name   string
		mutate func(*ports.ReviewConditionProofV2)
	}{
		{name: "wrong_owner", mutate: func(v *ports.ReviewConditionProofV2) { v.Owner.ComponentID = "custom/other-satisfier" }},
		{name: "wrong_authority_revision", mutate: func(v *ports.ReviewConditionProofV2) { v.Authority.Revision++ }},
		{name: "wrong_scope", mutate: func(v *ports.ReviewConditionProofV2) { v.ScopeDigest = controlEffectDigestV2(t, "wrong-proof-scope") }},
		{name: "wrong_condition_revision", mutate: func(v *ports.ReviewConditionProofV2) { v.ConditionRevision++ }},
		{name: "expired", mutate: func(v *ports.ReviewConditionProofV2) { v.ExpiresUnixNano = fixture.now.UnixNano() }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			wrong := proof
			testCase.mutate(&wrong)
			if _, err := fixture.review.Satisfy(context.Background(), control.SatisfyGovernedConditionsRequestV2{SatisfactionID: pending.ID, ExpectedRevision: pending.Revision, Proofs: []ports.ReviewConditionProofV2{wrong}, RequestedTTL: 5 * time.Second}); !core.HasReason(err, core.ReasonReviewConditionUnsatisfied) {
				t.Fatalf("wrong condition proof must fail closed: %v", err)
			}
		})
	}
	fixture.reviewFacts.LoseNextCASReply()
	if _, err := fixture.review.Satisfy(context.Background(), control.SatisfyGovernedConditionsRequestV2{SatisfactionID: pending.ID, ExpectedRevision: pending.Revision, Proofs: []ports.ReviewConditionProofV2{proof}, RequestedTTL: 5 * time.Second}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("satisfaction CAS reply loss must occur after persistence: %v", err)
	}
	satisfied, err := fixture.reviewFacts.InspectConditionSatisfaction(context.Background(), pending.ID)
	if err != nil || satisfied.State != ports.ConditionSatisfied {
		t.Fatalf("satisfaction CAS loss must recover by Inspect: %v %+v", err, satisfied)
	}
	if replayed, err := fixture.reviewFacts.CompareAndSwapConditionSatisfaction(context.Background(), ports.ConditionSatisfactionCASRequestV2{SatisfactionID: pending.ID, ExpectedRevision: pending.Revision, Next: satisfied}); err != nil || replayed.Revision != satisfied.Revision {
		t.Fatalf("canonical stale satisfaction replay must return current: %v %+v", err, replayed)
	}
	changedSatisfied := satisfied
	changedSatisfied.Proofs = append([]ports.ReviewConditionProofV2{}, satisfied.Proofs...)
	changedSatisfied.Proofs[0].Evidence.Digest = controlEffectDigestV2(t, "changed-proof")
	changedSatisfied.ProofsDigest, _ = ports.DigestConditionProofsV2(changedSatisfied.Proofs)
	if _, err := fixture.reviewFacts.CompareAndSwapConditionSatisfaction(context.Background(), ports.ConditionSatisfactionCASRequestV2{SatisfactionID: pending.ID, ExpectedRevision: pending.Revision, Next: changedSatisfied}); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("same stale revision with changed proof must conflict: %v", err)
	}
	projection, err := fixture.review.InspectDispatchReview(context.Background(), fixture.candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.SatisfactionRef != satisfied.ID || projection.SatisfactionRevision != satisfied.Revision {
		t.Fatalf("conditional projection must bind exact satisfaction: %+v", projection)
	}
	issued, err := fixture.dispatch.Issue(context.Background(), control.IssueGovernedDispatchRequestV2{EffectID: accepted.Intent.ID, ExpectedEffectRevision: accepted.Revision, PermitID: "permit-conditional-reviewed", AttemptID: "attempt-conditional-reviewed", PermitTTL: 4 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	fixture.now = fixture.now.Add(time.Second)
	expired := satisfied
	expired.State, expired.Revision, expired.UpdatedUnixNano, expired.InvalidationReason = ports.ConditionSatisfactionExpired, satisfied.Revision+1, fixture.now.UnixNano(), core.ReasonReviewConditionUnsatisfied
	if err := control.ValidateConditionSatisfactionTransitionV2(satisfied, expired, decided.Verdict, fixture.now); !core.HasReason(err, core.ReasonReviewConditionUnsatisfied) {
		t.Fatalf("satisfaction cannot expire before exact TTL boundary: %v", err)
	}
	exactExpiry := time.Unix(0, satisfied.ExpiresUnixNano)
	expired.UpdatedUnixNano = exactExpiry.UnixNano()
	if err := control.ValidateConditionSatisfactionTransitionV2(satisfied, expired, decided.Verdict, exactExpiry); err != nil {
		t.Fatalf("satisfaction may expire exactly at TTL boundary without rewriting proof: %v", err)
	}
	rolledBack := expired
	rolledBack.State, rolledBack.UpdatedUnixNano = ports.ConditionSatisfactionRevoked, satisfied.UpdatedUnixNano-1
	if err := control.ValidateConditionSatisfactionTransitionV2(satisfied, rolledBack, decided.Verdict, time.Unix(0, satisfied.UpdatedUnixNano-1)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("satisfaction transition must fail closed on clock rollback: %v", err)
	}
	revoked := satisfied
	revoked.State, revoked.Revision, revoked.UpdatedUnixNano, revoked.InvalidationReason = ports.ConditionSatisfactionRevoked, satisfied.Revision+1, fixture.now.UnixNano(), core.ReasonReviewConditionUnsatisfied
	fixture.reviewFacts.LoseNextCASReply()
	if _, err := fixture.reviewFacts.CompareAndSwapConditionSatisfaction(context.Background(), ports.ConditionSatisfactionCASRequestV2{SatisfactionID: satisfied.ID, ExpectedRevision: satisfied.Revision, Next: revoked}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("revocation CAS reply loss must occur after persistence: %v", err)
	}
	revokedCurrent, err := fixture.reviewFacts.InspectConditionSatisfaction(context.Background(), satisfied.ID)
	if err != nil || revokedCurrent.State != ports.ConditionSatisfactionRevoked || revokedCurrent.ProofsDigest != satisfied.ProofsDigest {
		t.Fatalf("revocation must preserve proof and recover by Inspect: %v %+v", err, revokedCurrent)
	}
	if replayed, err := fixture.reviewFacts.CompareAndSwapConditionSatisfaction(context.Background(), ports.ConditionSatisfactionCASRequestV2{SatisfactionID: satisfied.ID, ExpectedRevision: satisfied.Revision, Next: revoked}); err != nil || replayed.State != ports.ConditionSatisfactionRevoked {
		t.Fatalf("lost revocation reply must be canonical-idempotent: %v %+v", err, replayed)
	}
	if _, err := fixture.dispatch.Begin(context.Background(), control.BeginGovernedDispatchRequestV2{EffectID: accepted.Intent.ID, ExpectedEffectRevision: issued.Effect.Revision, PermitID: issued.Permit.Permit.ID, ExpectedPermitRevision: issued.Permit.Revision}); !core.HasReason(err, core.ReasonReviewConditionUnsatisfied) {
		t.Fatalf("satisfaction revoked after Issue must require a new permit: %v", err)
	}
}

func TestReviewGovernanceV2SelfReviewRequiresExplicitCurrentPolicy(t *testing.T) {
	t.Parallel()
	denied := newReviewGovernanceFixtureConfiguredV2(t, time.Unix(44_500, 0), ports.ReviewInvocationHuman, true, false, false)
	denied.now = denied.now.Add(time.Second)
	if _, err := denied.review.CreateCase(context.Background(), control.CreateGovernedReviewCaseRequestV2{EffectID: denied.effect.Intent.ID, ExpectedEffectRevision: denied.effect.Revision, Candidate: denied.candidate}); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("same authority subject must be denied by default: %v", err)
	}
	allowed := newReviewGovernanceFixtureConfiguredV2(t, time.Unix(44_700, 0), ports.ReviewInvocationHuman, true, true, false)
	allowed.now = allowed.now.Add(time.Second)
	caseFact, err := allowed.review.CreateCase(context.Background(), control.CreateGovernedReviewCaseRequestV2{EffectID: allowed.effect.Intent.ID, ExpectedEffectRevision: allowed.effect.Revision, Candidate: allowed.candidate})
	if err != nil {
		t.Fatal(err)
	}
	allowed.now = allowed.now.Add(time.Second)
	if _, err := allowed.review.Decide(context.Background(), control.DecideGovernedReviewRequestV2{CaseID: caseFact.Candidate.ID, ExpectedCaseRevision: caseFact.Revision, Observation: acceptedObservationV2(t, caseFact, allowed.now), RequestedTTL: 5 * time.Second}); err != nil {
		t.Fatalf("exact current allow_self_review policy may authorize the owner CAS: %v", err)
	}
}

func TestReviewGovernanceV2OperationNotRequiredNeedsExplicitPolicyAndNoInvocation(t *testing.T) {
	t.Parallel()
	for _, explicit := range []bool{false, true} {
		fixture := newReviewGovernanceFixtureConfiguredV2(t, time.Unix(45_000+int64(map[bool]int{false: 0, true: 100}[explicit]), 0), ports.ReviewInvocationHuman, false, false, explicit)
		fixture.now = fixture.now.Add(time.Second)
		caseFact, err := fixture.review.CreateCase(context.Background(), control.CreateGovernedReviewCaseRequestV2{EffectID: fixture.effect.Intent.ID, ExpectedEffectRevision: fixture.effect.Revision, Candidate: fixture.candidate})
		if err != nil {
			t.Fatal(err)
		}
		fixture.now = fixture.now.Add(time.Second)
		observation := acceptedObservationV2(t, caseFact, fixture.now)
		observation.Basis = ports.ReviewBasisPolicyNotRequired
		result, err := fixture.review.Decide(context.Background(), control.DecideGovernedReviewRequestV2{CaseID: caseFact.Candidate.ID, ExpectedCaseRevision: caseFact.Revision, Observation: observation, RequestedTTL: 5 * time.Second})
		if !explicit {
			if !core.HasReason(err, core.ReasonReviewVerdictStale) {
				t.Fatalf("empty or ordinary policy cannot mean operation_not_required: %v", err)
			}
			continue
		}
		if err != nil || result.Verdict.InvocationEffect != nil || result.Verdict.Basis != ports.ReviewBasisPolicyNotRequired {
			t.Fatalf("explicit policy decision must be authoritative without reviewer invocation: %v %+v", err, result)
		}
	}
}

func TestReviewGovernanceV2AutomaticReviewerRequiresExactSettledEffect(t *testing.T) {
	t.Parallel()
	for _, mode := range []ports.ReviewInvocationModeV2{ports.ReviewInvocationAutomaticLocal, ports.ReviewInvocationAutomaticRemote} {
		t.Run(string(mode), func(t *testing.T) {
			fixture := newReviewGovernanceFixtureV2(t, time.Unix(45_500, 0), mode, false)
			fixture.now = fixture.now.Add(time.Second)
			caseFact, err := fixture.review.CreateCase(context.Background(), control.CreateGovernedReviewCaseRequestV2{EffectID: fixture.effect.Intent.ID, ExpectedEffectRevision: fixture.effect.Revision, Candidate: fixture.candidate})
			if err != nil {
				t.Fatal(err)
			}
			fixture.now = fixture.now.Add(time.Second)
			observation := acceptedObservationV2(t, caseFact, fixture.now)
			observation.Basis = ports.ReviewBasisAutomatic
			if _, err := fixture.review.Decide(context.Background(), control.DecideGovernedReviewRequestV2{CaseID: caseFact.Candidate.ID, ExpectedCaseRevision: caseFact.Revision, Observation: observation, RequestedTTL: 5 * time.Second}); !core.HasReason(err, core.ReasonReviewRemoteEffectRequired) {
				t.Fatalf("automatic reviewer observation alone cannot become verdict: %v", err)
			}
			expected := fixture.candidate.InvocationEffect
			invocationIntent := fixture.effect.Intent
			invocationIntent.ID, invocationIntent.Revision, invocationIntent.Kind = expected.EffectID, expected.EffectRevision, expected.EffectKind
			invocationIntent.Payload.ContentDigest, invocationIntent.Provider = expected.PayloadDigest, expected.Provider
			invocationIntent.Relation = ports.EffectRelationV2{ReviewsCaseID: fixture.candidate.ID, ReviewsCandidateRevision: fixture.candidate.Revision}
			settlement := &control.EffectSettlementFactV2{Owner: invocationIntent.Owners[2], Disposition: control.SettlementConfirmedApplied, ReceiptRef: "review-invocation-settlement", EvidenceDigest: controlEffectDigestV2(t, "review-invocation-settlement"), SettledUnixNano: fixture.now.UnixNano()}
			invocationFact := control.EffectFactV2{Intent: invocationIntent, State: control.EffectUnknownOutcome, Revision: 5, Settlement: nil}
			overlay := &overlayEffectFactPortV2{base: fixture.effects, facts: map[core.EffectIntentID]control.EffectFactV2{expected.EffectID: invocationFact}}
			fixture.review.Effects = overlay
			if _, err := fixture.review.Decide(context.Background(), control.DecideGovernedReviewRequestV2{CaseID: caseFact.Candidate.ID, ExpectedCaseRevision: caseFact.Revision, Observation: observation, RequestedTTL: 5 * time.Second}); !core.HasReason(err, core.ReasonReviewRemoteEffectRequired) {
				t.Fatalf("unknown automatic reviewer Effect must only be inspected: %v", err)
			}
			invocationFact.State, invocationFact.Settlement = control.EffectSettled, settlement
			for _, drift := range []struct {
				name   string
				mutate func(*control.EffectFactV2)
			}{
				{name: "kind", mutate: func(v *control.EffectFactV2) { v.Intent.Kind = "custom/wrong-review-kind" }},
				{name: "payload", mutate: func(v *control.EffectFactV2) {
					v.Intent.Payload.ContentDigest = controlEffectDigestV2(t, "wrong-review-payload")
				}},
				{name: "provider", mutate: func(v *control.EffectFactV2) { v.Intent.Provider.ComponentID = "custom/wrong-review-provider" }},
				{name: "candidate_relation", mutate: func(v *control.EffectFactV2) { v.Intent.Relation.ReviewsCaseID = "other-review-case" }},
				{name: "candidate_revision", mutate: func(v *control.EffectFactV2) { v.Intent.Relation.ReviewsCandidateRevision++ }},
			} {
				t.Run("wrong_"+drift.name, func(t *testing.T) {
					wrong := invocationFact
					drift.mutate(&wrong)
					overlay.facts[expected.EffectID] = wrong
					if _, err := fixture.review.Decide(context.Background(), control.DecideGovernedReviewRequestV2{CaseID: caseFact.Candidate.ID, ExpectedCaseRevision: caseFact.Revision, Observation: observation, RequestedTTL: 5 * time.Second}); !core.HasReason(err, core.ReasonReviewRemoteEffectRequired) {
						t.Fatalf("automatic Review Effect binding drift must fail before Verdict CAS: %v", err)
					}
				})
			}
			overlay.facts[expected.EffectID] = invocationFact
			result, err := fixture.review.Decide(context.Background(), control.DecideGovernedReviewRequestV2{CaseID: caseFact.Candidate.ID, ExpectedCaseRevision: caseFact.Revision, Observation: observation, RequestedTTL: 5 * time.Second})
			if err != nil || result.Verdict.InvocationEffect == nil || result.Verdict.InvocationSettlementDigest == "" {
				t.Fatalf("only exact settled automatic Review Effect may support owner CAS: %v %+v", err, result)
			}
		})
	}
}

func satisfactionOwnerFromFixtureV2(t *testing.T, fixture *reviewGovernanceFixtureV2) ports.ReviewComponentBindingRefV2 {
	t.Helper()
	for _, member := range fixture.bindings.set.Members {
		if member.ComponentID == "custom/satisfier" {
			return ports.ReviewComponentBindingRefV2{BindingSetID: fixture.bindings.set.ID, BindingSetRevision: fixture.bindings.set.Revision, ComponentID: member.ComponentID, ManifestDigest: member.ManifestDigest, ArtifactDigest: member.ArtifactDigest, Capability: member.Grants[0].Capability}
		}
	}
	t.Fatal("satisfaction owner missing")
	return ports.ReviewComponentBindingRefV2{}
}

type reviewGovernanceFixtureV2 struct {
	now          time.Time
	effect       control.EffectFactV2
	candidate    ports.ReviewCandidateV2
	effects      *fakes.EffectStoreV2
	reviewFacts  *fakes.ReviewStoreV2
	bindings     *reviewBindingReaderV2
	authority    *reviewAuthorityReaderV2
	scope        *reviewScopeReaderV2
	reviewPolicy *reviewPolicyReaderV2
	review       control.ReviewGovernanceGatewayV2
	dispatch     control.GovernanceDispatchGatewayV2
}

func newReviewGovernanceFixtureV2(t *testing.T, requested time.Time, mode ports.ReviewInvocationModeV2, selfReview bool) *reviewGovernanceFixtureV2 {
	return newReviewGovernanceFixtureConfiguredV2(t, requested, mode, selfReview, selfReview, false)
}

func newReviewGovernanceFixtureConfiguredV2(t *testing.T, requested time.Time, mode ports.ReviewInvocationModeV2, sameReviewer, allowSelfReview, operationNotRequired bool) *reviewGovernanceFixtureV2 {
	t.Helper()
	current := requested.Add(time.Second)
	base := controlEffectFactV2(t, requested, false, false).Intent
	base.ID = "effect-review-1"
	base.RunID = "run-review-1"
	base.Review = ports.ReviewBindingRefV2{Ref: "review-case-1", Revision: 1}
	base.Policy = ports.DispatchPolicyBindingRefV2{Ref: "dispatch-policy-review-1", Revision: 1}
	base.ExpiresUnixNano = requested.Add(time.Minute).UnixNano()
	grant := func(capability ports.CapabilityNameV2) ports.CapabilityGrantV2 {
		return ports.CapabilityGrantV2{Capability: capability, EvidenceDigest: controlEffectDigestV2(t, "grant-"+string(capability)), ObservedUnixNano: requested.UnixNano(), ExpiresUnixNano: requested.Add(time.Minute).UnixNano()}
	}
	member := func(id ports.ComponentIDV2, capability ports.CapabilityNameV2, manifest, artifact core.Digest, owners []ports.OwnerAssignmentV2) control.BindingMemberV2 {
		if len(owners) == 0 {
			owners = []ports.OwnerAssignmentV2{{Role: ports.OwnerEffect, OwnerComponentID: id}, {Role: ports.OwnerSettlement, OwnerComponentID: id}, {Role: ports.OwnerCleanup, OwnerComponentID: id}}
		}
		return control.BindingMemberV2{BindingID: "binding-" + string(id), BindingRevision: 1, ComponentID: id, Kind: ports.ComponentKindV2(string(id) + "-kind"), ManifestDigest: manifest, ArtifactDigest: artifact, Contract: ports.ContractBindingV2{Name: ports.NamespacedNameV2(string(id) + "-contract"), Version: "2.0.0", Compatible: ports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}}, Owners: owners, Grants: []ports.CapabilityGrantV2{grant(capability)}}
	}
	reviewOwner := ports.ReviewComponentBindingRefV2{BindingSetID: base.Provider.BindingSetID, BindingSetRevision: base.Provider.BindingSetRevision, ComponentID: "custom/review-owner", ManifestDigest: controlEffectDigestV2(t, "review-owner-manifest"), ArtifactDigest: controlEffectDigestV2(t, "review-owner-artifact"), Capability: "custom/review-own"}
	reviewer := ports.ReviewComponentBindingRefV2{BindingSetID: base.Provider.BindingSetID, BindingSetRevision: base.Provider.BindingSetRevision, ComponentID: "custom/reviewer", ManifestDigest: controlEffectDigestV2(t, "reviewer-manifest"), ArtifactDigest: controlEffectDigestV2(t, "reviewer-artifact"), Capability: "custom/review-attest"}
	satisfier := ports.ReviewComponentBindingRefV2{BindingSetID: base.Provider.BindingSetID, BindingSetRevision: base.Provider.BindingSetRevision, ComponentID: "custom/satisfier", ManifestDigest: controlEffectDigestV2(t, "satisfier-manifest"), ArtifactDigest: controlEffectDigestV2(t, "satisfier-artifact"), Capability: "custom/condition-satisfy"}
	providerOwners := []ports.OwnerAssignmentV2{{Role: ports.OwnerEffect, OwnerComponentID: base.Provider.ComponentID}, {Role: ports.OwnerSettlement, OwnerComponentID: base.Provider.ComponentID}, {Role: ports.OwnerCleanup, OwnerComponentID: base.Provider.ComponentID}}
	members := []control.BindingMemberV2{member(base.Provider.ComponentID, base.Provider.Capability, base.Provider.ManifestDigest, base.Provider.ArtifactDigest, providerOwners), member(reviewOwner.ComponentID, reviewOwner.Capability, reviewOwner.ManifestDigest, reviewOwner.ArtifactDigest, nil), member(reviewer.ComponentID, reviewer.Capability, reviewer.ManifestDigest, reviewer.ArtifactDigest, nil), member(satisfier.ComponentID, satisfier.Capability, satisfier.ManifestDigest, satisfier.ArtifactDigest, nil)}
	set := control.BindingSetFactV2{ID: base.Provider.BindingSetID, PlanID: "review-plan", PlanDigest: controlEffectDigestV2(t, "review-plan"), GovernanceDigest: controlEffectDigestV2(t, "review-governance"), State: control.BindingSetActive, Revision: base.Provider.BindingSetRevision, Members: members, TopologicalOrder: []ports.ComponentIDV2{base.Provider.ComponentID, reviewOwner.ComponentID, reviewer.ComponentID, satisfier.ComponentID}, Residuals: []control.BindingResidualV2{}, CreatedUnixNano: requested.UnixNano(), ExpiresUnixNano: requested.Add(time.Minute).UnixNano()}
	grantDigest, err := set.CapabilityGrantDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	source := func(name string) ports.GovernanceSourceFactRefV2 {
		return ports.GovernanceSourceFactRefV2{Ref: name, Revision: 1, Digest: controlEffectDigestV2(t, name)}
	}
	sandbox := source("review-sandbox-source")
	scopeFact := ports.ExecutionScopeCurrentFactV2{Ref: "review-current-scope", Revision: 1, Scope: base.Scope, CapabilityGrantDigest: grantDigest, ActivationSource: source("review-activation-source"), InstanceSource: source("review-instance-source"), SandboxSource: &sandbox, AuthoritySource: source("review-authority-source"), BindingSource: source("review-binding-source"), RunSource: source("review-run-source"), ActiveRunID: base.RunID, RunState: "running", ProjectionWatermark: 1, State: ports.ExecutionScopeFactActive, ExpiresUnixNano: requested.Add(time.Minute).UnixNano()}
	scopeFact.Digest, err = scopeFact.DigestV2()
	if err != nil {
		t.Fatalf("current scope digest: %v", err)
	}
	base.CurrentScope = ports.ExecutionScopeBindingRefV2{Ref: scopeFact.Ref, Digest: scopeFact.Digest, Revision: scopeFact.Revision}
	budget := control.BudgetBindingFactV2{Ref: base.Budget.Ref, IntentID: base.ID, IntentRevision: base.Revision, Scope: base.Scope, Mode: control.BudgetOperationNotRequired, PolicyDigest: base.Budget.PolicyDigest, PolicyDecisionRef: "review-budget-policy", PolicyEvidenceDigest: controlEffectDigestV2(t, "review-budget-evidence"), State: control.BudgetFactActive, Revision: 1, ExpiresUnixNano: requested.Add(time.Minute).UnixNano()}
	base.Budget, err = budget.BindingRefV2()
	if err != nil {
		t.Fatalf("budget binding: %v", err)
	}
	actor := ports.DispatchAuthorityFactV2{Ref: base.Authority.Ref, Digest: base.Authority.Digest, Revision: base.Authority.Revision, Scope: base.Scope, ActionScopeDigest: base.ActionScopeDigest, State: ports.AuthorityFactActive, ExpiresUnixNano: requested.Add(time.Minute).UnixNano()}
	reviewerAuthority := ports.AuthorityBindingRefV2{Ref: "reviewer-authority-1", Digest: controlEffectDigestV2(t, "reviewer-authority-1"), Revision: 1, Epoch: base.Scope.AuthorityEpoch}
	if sameReviewer {
		reviewerAuthority = base.Authority
	}
	reviewerAuthorityFact := ports.DispatchAuthorityFactV2{Ref: reviewerAuthority.Ref, Digest: reviewerAuthority.Digest, Revision: reviewerAuthority.Revision, Scope: base.Scope, ActionScopeDigest: base.ActionScopeDigest, State: ports.AuthorityFactActive, ExpiresUnixNano: requested.Add(time.Minute).UnixNano()}
	subjectDigest, err := base.ReviewSubjectDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	policyFact := ports.ReviewPolicyFactV2{Ref: "review-policy-1", Revision: 1, SubjectDigest: subjectDigest, Scope: base.Scope, RunID: base.RunID, CurrentScope: base.CurrentScope, RiskClass: base.RiskClass, ActorAuthorityRef: base.Authority.Ref, ReviewerAuthorityRef: reviewerAuthority.Ref, AllowSelfReview: allowSelfReview, OperationNotRequired: operationNotRequired, PolicyDecisionRef: "review-policy-decision-1", Active: true, ExpiresUnixNano: requested.Add(40 * time.Second).UnixNano()}
	policyFact.Digest, err = policyFact.DigestV2()
	if err != nil {
		t.Fatalf("review policy digest: %v", err)
	}
	candidate := ports.ReviewCandidateV2{ContractVersion: ports.ReviewContractVersionV2, ID: base.Review.Ref, Revision: base.Review.Revision, IntentID: base.ID, IntentRevision: base.Revision, SubjectDigest: subjectDigest, CandidateKind: "custom/effect-review", RiskClass: base.RiskClass, PayloadSchema: base.Payload.Schema, PayloadDigest: base.Payload.ContentDigest, PayloadRevision: base.PayloadRevision, Scope: base.Scope, RunID: base.RunID, ActionScopeDigest: base.ActionScopeDigest, SubjectProvider: base.Provider, ReviewOwnerBinding: reviewOwner, ReviewerBinding: reviewer, CurrentScope: base.CurrentScope, ActorAuthority: base.Authority, ReviewerAuthority: reviewerAuthority, Policy: ports.ReviewPolicyBindingRefV2{Ref: policyFact.Ref, Digest: policyFact.Digest, Revision: policyFact.Revision}, Evidence: []ports.ReviewEvidenceRefV2{}, InvocationMode: mode, RequestedUnixNano: requested.UnixNano(), ExpiresUnixNano: requested.Add(30 * time.Second).UnixNano()}
	if mode != ports.ReviewInvocationHuman {
		candidate.InvocationEffect = &ports.ReviewInvocationEffectRefV2{EffectID: "review-invocation-effect", EffectRevision: 1, EffectKind: "custom/review-invoke", PayloadDigest: controlEffectDigestV2(t, "review-invocation-payload"), Provider: base.Provider}
	}
	candidateDigest, err := candidate.DigestV2()
	if err != nil {
		t.Fatalf("review candidate digest: %v", err)
	}
	base.Review = ports.ReviewBindingRefV2{Ref: candidate.ID, Digest: candidateDigest, Revision: candidate.Revision, PolicyDigest: policyFact.Digest}
	dispatchPolicy := ports.DispatchPolicyFactV2{Ref: base.Policy.Ref, Revision: base.Policy.Revision, IntentID: base.ID, IntentRevision: base.Revision, Scope: base.Scope, EffectKind: base.Kind, RiskClass: base.RiskClass, ActionScopeDigest: base.ActionScopeDigest, MaximumPermitTTL: 10 * time.Second, Active: true, ExpiresUnixNano: requested.Add(time.Minute).UnixNano()}
	dispatchPolicy.IntentDigest, err = base.PolicyCandidateDigestV2()
	if err != nil {
		t.Fatalf("dispatch policy candidate digest: %v", err)
	}
	dispatchPolicy.Digest, err = dispatchPolicy.DigestV2()
	if err != nil {
		t.Fatalf("dispatch policy digest: %v", err)
	}
	base.Policy.Digest = dispatchPolicy.Digest
	effect, err := control.NewProposedEffectFactV2(base, current)
	if err != nil {
		t.Fatalf("persisted review Effect: %v", err)
	}
	effects := fakes.NewEffectStoreV2(func() time.Time { return current })
	if _, err := effects.CreateBudgetBinding(context.Background(), budget); err != nil {
		t.Fatal(err)
	}
	if _, err := effects.CreateEffect(context.Background(), effect); err != nil {
		t.Fatal(err)
	}
	reviewFacts := fakes.NewReviewStoreV2(func() time.Time { return current })
	bindings := &reviewBindingReaderV2{set: set}
	authority := &reviewAuthorityReaderV2{facts: map[string]ports.DispatchAuthorityFactV2{actor.Ref: actor, reviewerAuthorityFact.Ref: reviewerAuthorityFact}}
	scope := &reviewScopeReaderV2{fact: scopeFact}
	reviewPolicy := &reviewPolicyReaderV2{fact: policyFact}
	reviewGateway := control.ReviewGovernanceGatewayV2{Facts: reviewFacts, Policies: reviewPolicy, Authority: authority, CurrentScopes: scope, Bindings: bindings, Effects: effects, Clock: func() time.Time { return current }}
	identity := control.IdentityExecutionLease{ID: "review-identity-lease", Identity: base.Scope.Identity, Lineage: base.Scope.Lineage, ActivationAttemptID: "review-activation", State: control.IdentityLeaseActive, AuthorityEpoch: base.Scope.AuthorityEpoch, ExpiresAt: requested.Add(time.Minute), Revision: 1}
	dispatchGateway := control.GovernanceDispatchGatewayV2{Effects: effects, Bindings: bindings, Budgets: effects, IdentityLeases: staticIdentityPortV2{lease: identity}, Authority: authority, Policies: &reviewDispatchPolicyReaderV2{fact: dispatchPolicy}, CurrentScopes: scope, Review: reviewGateway, Credentials: staticCredentialReaderV2{}, Clock: func() time.Time { return current }}
	fixture := &reviewGovernanceFixtureV2{now: current, effect: effect, candidate: candidate, effects: effects, reviewFacts: reviewFacts, bindings: bindings, authority: authority, scope: scope, reviewPolicy: reviewPolicy, review: reviewGateway, dispatch: dispatchGateway}
	fixture.review.Clock, fixture.dispatch.Clock = func() time.Time { return fixture.now }, func() time.Time { return fixture.now }
	effects.SetClock(func() time.Time { return fixture.now })
	reviewFacts.SetClock(func() time.Time { return fixture.now })
	return fixture
}

func acceptedObservationV2(t *testing.T, caseFact ports.ReviewCaseFactV2, now time.Time) ports.ReviewAttestationObservationV2 {
	t.Helper()
	return ports.ReviewAttestationObservationV2{CaseID: caseFact.Candidate.ID, CandidateDigest: caseFact.CandidateDigest, ReviewerBinding: caseFact.Candidate.ReviewerBinding, ReviewerAuthority: caseFact.Candidate.ReviewerAuthority, ProposedState: ports.ReviewVerdictAccepted, Basis: ports.ReviewBasisHuman, Evidence: []ports.ReviewEvidenceRefV2{{Ref: "review-evidence-1", Classification: "custom/reviewer-attestation", Digest: controlEffectDigestV2(t, "review-evidence-1")}}, Conditions: []ports.ReviewConditionV2{}, ObservedUnixNano: now.UnixNano()}
}

func decidedAcceptedReviewFixtureV2(t *testing.T, requested time.Time) *reviewGovernanceFixtureV2 {
	t.Helper()
	fixture := newReviewGovernanceFixtureV2(t, requested, ports.ReviewInvocationHuman, false)
	fixture.now = fixture.now.Add(time.Second)
	caseFact, err := fixture.review.CreateCase(context.Background(), control.CreateGovernedReviewCaseRequestV2{EffectID: fixture.effect.Intent.ID, ExpectedEffectRevision: fixture.effect.Revision, Candidate: fixture.candidate})
	if err != nil {
		t.Fatal(err)
	}
	fixture.now = fixture.now.Add(time.Second)
	if _, err := fixture.review.Decide(context.Background(), control.DecideGovernedReviewRequestV2{CaseID: caseFact.Candidate.ID, ExpectedCaseRevision: caseFact.Revision, Observation: acceptedObservationV2(t, caseFact, fixture.now), RequestedTTL: 20 * time.Second}); err != nil {
		t.Fatal(err)
	}
	accepted := fixture.effect
	accepted.State, accepted.Revision, accepted.UpdatedUnixNano = control.EffectAccepted, fixture.effect.Revision+1, fixture.now.UnixNano()
	accepted, err = fixture.effects.CompareAndSwapEffect(context.Background(), control.EffectFactCASRequestV2{ExpectedRevision: fixture.effect.Revision, Next: accepted})
	if err != nil {
		t.Fatal(err)
	}
	fixture.effect = accepted
	return fixture
}

type reviewBindingReaderV2 struct{ set control.BindingSetFactV2 }
type reviewAuthorityReaderV2 struct {
	facts map[string]ports.DispatchAuthorityFactV2
}
type reviewScopeReaderV2 struct {
	fact ports.ExecutionScopeCurrentFactV2
}
type reviewPolicyReaderV2 struct{ fact ports.ReviewPolicyFactV2 }
type reviewDispatchPolicyReaderV2 struct{ fact ports.DispatchPolicyFactV2 }
type staticEffectFactReaderV2 struct{ fact control.EffectFactV2 }
type overlayEffectFactPortV2 struct {
	base  control.EffectFactPortV2
	facts map[core.EffectIntentID]control.EffectFactV2
}

func (s *overlayEffectFactPortV2) InspectEffect(ctx context.Context, id core.EffectIntentID) (control.EffectFactV2, error) {
	if fact, ok := s.facts[id]; ok {
		return fact, nil
	}
	return s.base.InspectEffect(ctx, id)
}
func (s *overlayEffectFactPortV2) CreateEffect(ctx context.Context, fact control.EffectFactV2) (control.EffectFactV2, error) {
	return s.base.CreateEffect(ctx, fact)
}
func (s *overlayEffectFactPortV2) InspectEffectByIdempotency(ctx context.Context, class ports.EffectStableScopeClassV2, digest core.Digest, key string) (control.EffectFactV2, error) {
	return s.base.InspectEffectByIdempotency(ctx, class, digest, key)
}
func (s *overlayEffectFactPortV2) InspectConflictDomain(ctx context.Context, binding ports.ConflictDomainBindingV2) (control.EffectFactV2, error) {
	return s.base.InspectConflictDomain(ctx, binding)
}
func (s *overlayEffectFactPortV2) CompareAndSwapEffect(ctx context.Context, request control.EffectFactCASRequestV2) (control.EffectFactV2, error) {
	return s.base.CompareAndSwapEffect(ctx, request)
}
func (s *overlayEffectFactPortV2) IssueDispatchPermit(ctx context.Context, request control.IssueDispatchPermitRequestV2) (control.IssueDispatchPermitResultV2, error) {
	return s.base.IssueDispatchPermit(ctx, request)
}
func (s *overlayEffectFactPortV2) InspectDispatchPermit(ctx context.Context, id string) (control.DispatchPermitFactV2, error) {
	return s.base.InspectDispatchPermit(ctx, id)
}
func (s *overlayEffectFactPortV2) BeginDispatch(ctx context.Context, request control.BeginDispatchRequestV2) (control.DispatchPermitFactV2, error) {
	return s.base.BeginDispatch(ctx, request)
}
func (s *overlayEffectFactPortV2) RecordEnforcementReceipt(ctx context.Context, request control.RecordEnforcementReceiptRequestV2) (control.DispatchPermitFactV2, error) {
	return s.base.RecordEnforcementReceipt(ctx, request)
}
func (s *overlayEffectFactPortV2) CompareAndSwapDispatchPermit(ctx context.Context, request control.DispatchPermitFactCASRequestV2) (control.DispatchPermitFactV2, error) {
	return s.base.CompareAndSwapDispatchPermit(ctx, request)
}

func (s staticEffectFactReaderV2) InspectEffect(context.Context, core.EffectIntentID) (control.EffectFactV2, error) {
	return s.fact, nil
}
func (s staticEffectFactReaderV2) InspectEffectByIdempotency(context.Context, ports.EffectStableScopeClassV2, core.Digest, string) (control.EffectFactV2, error) {
	return control.EffectFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticEffectFactReaderV2) InspectConflictDomain(context.Context, ports.ConflictDomainBindingV2) (control.EffectFactV2, error) {
	return control.EffectFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticEffectFactReaderV2) CreateEffect(context.Context, control.EffectFactV2) (control.EffectFactV2, error) {
	return control.EffectFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticEffectFactReaderV2) CompareAndSwapEffect(context.Context, control.EffectFactCASRequestV2) (control.EffectFactV2, error) {
	return control.EffectFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticEffectFactReaderV2) IssueDispatchPermit(context.Context, control.IssueDispatchPermitRequestV2) (control.IssueDispatchPermitResultV2, error) {
	return control.IssueDispatchPermitResultV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticEffectFactReaderV2) InspectDispatchPermit(context.Context, string) (control.DispatchPermitFactV2, error) {
	return control.DispatchPermitFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticEffectFactReaderV2) CompareAndSwapDispatchPermit(context.Context, control.DispatchPermitFactCASRequestV2) (control.DispatchPermitFactV2, error) {
	return control.DispatchPermitFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticEffectFactReaderV2) BeginDispatch(context.Context, control.BeginDispatchRequestV2) (control.DispatchPermitFactV2, error) {
	return control.DispatchPermitFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s staticEffectFactReaderV2) RecordEnforcementReceipt(context.Context, control.RecordEnforcementReceiptRequestV2) (control.DispatchPermitFactV2, error) {
	return control.DispatchPermitFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}

func (s *reviewBindingReaderV2) InspectBindingSet(context.Context, string) (control.BindingSetFactV2, error) {
	return s.set, nil
}
func (s *reviewBindingReaderV2) CreateBinding(context.Context, control.BindingFactV2) (control.BindingFactV2, error) {
	return control.BindingFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s *reviewBindingReaderV2) InspectBinding(context.Context, string) (control.BindingFactV2, error) {
	return control.BindingFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s *reviewBindingReaderV2) CompareAndSwapBinding(context.Context, control.BindingFactCASRequestV2) (control.BindingFactV2, error) {
	return control.BindingFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s *reviewBindingReaderV2) CommitBindingSet(context.Context, control.CommitBindingSetRequestV2) (control.BindingSetFactV2, error) {
	return control.BindingSetFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s *reviewBindingReaderV2) CompareAndSwapBindingSet(context.Context, control.BindingSetCASRequestV2) (control.BindingSetFactV2, error) {
	return control.BindingSetFactV2{}, core.NewError(core.ErrorInternal, core.ReasonComponentMissing, "unused")
}
func (s *reviewAuthorityReaderV2) InspectDispatchAuthority(_ context.Context, ref string) (ports.DispatchAuthorityFactV2, error) {
	fact, ok := s.facts[ref]
	if !ok {
		return ports.DispatchAuthorityFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonEffectAuthorizationMissing, "authority missing")
	}
	return fact, nil
}
func (s *reviewScopeReaderV2) InspectCurrentExecutionScope(context.Context, string) (ports.ExecutionScopeCurrentFactV2, error) {
	return s.fact, nil
}
func (s *reviewPolicyReaderV2) InspectReviewPolicy(context.Context, string) (ports.ReviewPolicyFactV2, error) {
	return s.fact, nil
}
func (s *reviewDispatchPolicyReaderV2) InspectDispatchPolicy(context.Context, string) (ports.DispatchPolicyFactV2, error) {
	return s.fact, nil
}
