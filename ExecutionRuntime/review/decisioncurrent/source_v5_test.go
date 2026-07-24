package decisioncurrent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCurrentFactSourceV5HumanQuorumUsesPublicOwnerReaders(t *testing.T) {
	fixture := newCurrentSourceFixtureV5(t)
	snapshot, err := fixture.source.InspectReviewCurrentFactsV5(context.Background(), runtimeadapter.ExactCurrentRequestV5{Intent: fixture.intent, Basis: runtimeports.OperationReviewBasisAcceptedQuorumV5})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Quorum == nil || snapshot.PolicyNotRequired != nil || snapshot.Quorum.Policy.HumanQuorumPolicySource == nil || snapshot.Quorum.Policy.HumanQuorumPolicyProjection == nil || snapshot.Quorum.Policy.PolicyDecisionRef != "" {
		t.Fatalf("Human source did not preserve the exact Policy Owner boundary: %+v", snapshot.Quorum)
	}
	reader, err := runtimeadapter.NewReaderV5(fixture.source, fixture.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := reader.InspectOperationReviewCurrentV5(context.Background(), runtimeports.OperationReviewCurrentRequestV5{Intent: fixture.intent, Basis: runtimeports.OperationReviewBasisAcceptedQuorumV5})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Quorum == nil || projection.Quorum.QuorumPolicy.Ref != fixture.policy.value.Ref.ID || projection.Quorum.Verdict.ID != fixture.verdict.ID {
		t.Fatalf("Human V5 production-shaped projection drifted: %+v", projection)
	}
}

func TestCurrentFactSourceV5HumanQuorumFailsClosedOnPolicyS2Drift(t *testing.T) {
	fixture := newCurrentSourceFixtureV5(t)
	fixture.policy.driftAt = 2
	snapshot, err := fixture.source.InspectReviewCurrentFactsV5(context.Background(), runtimeadapter.ExactCurrentRequestV5{Intent: fixture.intent, Basis: runtimeports.OperationReviewBasisAcceptedQuorumV5})
	if err == nil || snapshot.Digest != "" {
		t.Fatalf("Policy S2 drift reached a sealed Review snapshot: value=%+v err=%v", snapshot, err)
	}
}

func TestCurrentFactSourceV5HumanQuorumLostReplyUsesDetachedExactInspect(t *testing.T) {
	fixture := newCurrentSourceFixtureV5(t)
	ctx, cancel := context.WithCancel(context.Background())
	fixture.policy.loseInspect, fixture.policy.cancel = true, cancel
	snapshot, err := fixture.source.InspectReviewCurrentFactsV5(ctx, runtimeadapter.ExactCurrentRequestV5{Intent: fixture.intent, Basis: runtimeports.OperationReviewBasisAcceptedQuorumV5})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Digest == "" || ctx.Err() == nil || fixture.policy.resolveCalls != 2 || fixture.policy.inspectCalls != 3 {
		t.Fatalf("lost exact Policy reply did not recover on the same current coordinate: digest=%s ctx=%v resolve=%d inspect=%d", snapshot.Digest, ctx.Err(), fixture.policy.resolveCalls, fixture.policy.inspectCalls)
	}
}

func TestCurrentFactSourceV5HumanQuorumClockRollbackAndBypassFailClosed(t *testing.T) {
	t.Run("clock rollback", func(t *testing.T) {
		fixture := newCurrentSourceFixtureV5(t)
		fixture.clock.rollbackAt = fixture.clock.calls + 2
		snapshot, err := fixture.source.InspectReviewCurrentFactsV5(context.Background(), runtimeadapter.ExactCurrentRequestV5{Intent: fixture.intent, Basis: runtimeports.OperationReviewBasisAcceptedQuorumV5})
		if !core.HasReason(err, core.ReasonClockRegression) || snapshot.Digest != "" {
			t.Fatalf("clock rollback reached a sealed Review snapshot: value=%+v err=%v", snapshot, err)
		}
	})
	t.Run("policy not required", func(t *testing.T) {
		fixture := newCurrentSourceFixtureV5(t)
		snapshot, err := fixture.source.InspectReviewCurrentFactsV5(context.Background(), runtimeadapter.ExactCurrentRequestV5{Intent: fixture.intent, Basis: runtimeports.OperationReviewBasisPolicyNotRequiredV5})
		if !core.HasCategory(err, core.ErrorUnavailable) || snapshot.Digest != "" {
			t.Fatalf("production Bypass was enabled without its exact Owner readers: value=%+v err=%v", snapshot, err)
		}
	})
}

type currentSourceFixtureV5 struct {
	clock   *humanExternalClockV2
	source  *CurrentFactSourceV5
	policy  *humanPolicyReaderV2
	intent  runtimeports.OperationEffectIntentV3
	verdict contract.HumanVerdictV2
}

func newCurrentSourceFixtureV5(t *testing.T) currentSourceFixtureV5 {
	t.Helper()
	external := newHumanExternalFixtureV2(t)
	base := external.base
	payload := []byte(`{"operation":"human-review-v5"}`)
	target := external.target
	target.Kind = contract.TargetEffectV1
	target.PayloadDigest = core.DigestBytes(payload)
	target.PayloadRevision = 4
	target.IntentID = "effect-human-source-v5"
	target.IntentRevision = 2
	target.SubjectDigest = externalDigestV1("effect-subject-human-source-v5")
	target.ExpiresUnixNano = base.Add(10 * time.Minute).UnixNano()
	target.Digest = ""
	var err error
	target, err = contract.SealTargetSnapshotV1(target)
	if err != nil {
		t.Fatal(err)
	}

	caseDecision, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "case-human-source-v5", Revision: 4, CreatedUnixNano: base.UnixNano(), UpdatedUnixNano: base.Add(5 * time.Second).UnixNano()}, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, State: contract.CaseDecidingV1, CurrentRoundID: "round-human-source-v5", CurrentAssignment: "assignment-human-source-v5", ExpiresUnixNano: base.Add(8 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	round, err := contract.SealReviewRoundV1(contract.ReviewRoundV1{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "round-human-source-v5", Revision: 1, CreatedUnixNano: base.Add(time.Second).UnixNano(), UpdatedUnixNano: base.Add(4 * time.Second).UnixNano()}, CaseID: caseDecision.ID, CaseRevision: caseDecision.Revision, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, Route: contract.RouteHumanV1, State: contract.RoundAttestedV1, AssignmentID: "assignment-human-source-v5", ContextFrameDigest: target.ContextFrameDigest, RubricDigest: externalDigestV1("rubric-human-source-v5"), ExpiresUnixNano: base.Add(8 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	policy := external.proposed.QuorumPolicy
	responsibility := external.proposed.ResponsibilitySubject
	proposed, err := contract.SealHumanReviewPanelV2(contract.HumanReviewPanelV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, Revision: 1, CreatedUnixNano: base.Add(2 * time.Second).UnixNano(), UpdatedUnixNano: base.Add(2 * time.Second).UnixNano()}, Case: contract.HumanCaseExactRefV2{TenantID: caseDecision.TenantID, ID: caseDecision.ID, Revision: caseDecision.Revision, Digest: caseDecision.Digest}, Target: contract.HumanTargetExactRefV2{TenantID: target.TenantID, ID: target.ID, Revision: target.Revision, Digest: target.Digest}, Round: contract.HumanRoundExactRefV2{TenantID: round.TenantID, ID: round.ID, Revision: round.Revision, Digest: round.Digest}, QuorumPolicy: policy, ResponsibilitySubject: responsibility, State: contract.HumanPanelProposedV2, AcceptThreshold: 1, MaximumPanelSize: 1, RoleRequirements: []contract.HumanRoleRequirementV2{{Role: "technical", Minimum: 1}}, RejectVetoRoles: []string{"security"}, DelegationRequired: true, ProductionSelfReviewAllowed: false, MaxPanelDurationNanos: int64(10 * time.Minute), MaxVoteTTLNanos: int64(5 * time.Minute), ExpiresUnixNano: base.Add(6 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	assignment := external.assignment.Clone()
	assignment.ID, assignment.Revision = "assignment-human-source-v5", 1
	assignment.Panel, assignment.Case, assignment.Round, assignment.Target = proposed.ExactRef(), proposed.Case, proposed.Round, proposed.Target
	assignment.CreatedUnixNano, assignment.UpdatedUnixNano = base.Add(2*time.Second).UnixNano(), base.Add(2*time.Second).UnixNano()
	assignment.Digest = ""
	assignment, err = contract.SealHumanPanelAssignmentV2(assignment)
	if err != nil {
		t.Fatal(err)
	}
	open := proposed.Clone()
	open.Revision++
	open.State, open.UpdatedUnixNano = contract.HumanPanelOpenV2, base.Add(3*time.Second).UnixNano()
	open.AssignmentRefs = []contract.HumanPanelAssignmentExactRefV2{assignment.ExactRef()}
	open.Digest = ""
	open, err = contract.SealHumanReviewPanelV2(open)
	if err != nil {
		t.Fatal(err)
	}
	evidence := []runtimeports.ReviewEvidenceRefV2{{Ref: "review-evidence://human-source-v5", Classification: "review/human", Digest: externalDigestV1("human-source-v5")}}
	evidenceDigest, _ := contract.ComputeReviewEvidenceDigestV1(evidence)
	attestation, err := contract.SealHumanAttestationV2(contract.HumanAttestationV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "attestation-human-source-v5", Revision: 1, CreatedUnixNano: base.Add(3 * time.Second).UnixNano(), UpdatedUnixNano: base.Add(3 * time.Second).UnixNano()}, IdempotencyKey: "idem-human-source-v5", Panel: open.ExactRef(), Assignment: assignment.ExactRef(), Case: open.Case, Round: open.Round, Target: open.Target, Policy: open.QuorumPolicy, ResponsibilitySubject: open.ResponsibilitySubject, ReviewerIdentity: assignment.ReviewerIdentity, ReviewerAuthority: assignment.ReviewerAuthority, Delegation: &assignment.DelegationFact, ReviewerBinding: assignment.ReviewerBinding, Resolution: contract.ResolutionAcceptV1, ReasonCodes: []string{"review/verified"}, Evidence: evidence, EvidenceDigest: evidenceDigest, ObservedUnixNano: base.Add(3 * time.Second).UnixNano(), ExpiresUnixNano: base.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	quorumPanel := nextSourcePanelV5(t, open, contract.HumanPanelQuorumSatisfiedV2, base.Add(4*time.Second))
	reviewerSet, _ := contract.ComputeHumanReviewerSetDigestV2([]contract.HumanIdentityProofRefV2{assignment.ReviewerIdentity})
	quorum, err := contract.SealHumanQuorumDecisionV2(contract.HumanQuorumDecisionV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "quorum-human-source-v5", Revision: 1, CreatedUnixNano: base.Add(4 * time.Second).UnixNano(), UpdatedUnixNano: base.Add(4 * time.Second).UnixNano()}, Panel: quorumPanel.ExactRef(), Policy: policy, AcceptedAttestationRefs: []contract.HumanAttestationExactRefV2{attestation.ExactRef()}, DistinctReviewerIdentityRefs: []contract.HumanIdentityProofRefV2{assignment.ReviewerIdentity}, SatisfiedRoleCounts: []contract.HumanSatisfiedRoleCountV2{{Role: "technical", DistinctCurrentCount: 1}}, AcceptCount: 1, Threshold: 1, Resolution: contract.ResolutionAcceptV1, EvidenceSetDigest: evidenceDigest, ReviewerSetDigest: reviewerSet, CheckedUnixNano: base.Add(4 * time.Second).UnixNano(), ExpiresUnixNano: base.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	deciding := nextSourcePanelV5(t, quorumPanel, contract.HumanPanelDecidingV2, base.Add(5*time.Second))
	verdict, err := contract.SealHumanVerdictV2(contract.HumanVerdictV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "verdict-human-source-v5", Revision: 1, CreatedUnixNano: base.Add(5 * time.Second).UnixNano(), UpdatedUnixNano: base.Add(5 * time.Second).UnixNano()}, Case: proposed.Case, Target: proposed.Target, Round: proposed.Round, Panel: deciding.ExactRef(), QuorumDecision: quorum.ExactRef(), Policy: policy, Scope: target.Scope, CurrentScope: target.CurrentScope, ReviewerSetDigest: reviewerSet, ReviewerAuthorityRefs: []runtimeports.AuthorityBindingRefV2{assignment.ReviewerAuthority}, BindingClosures: []runtimeports.ReviewComponentBindingRefV2{assignment.ReviewerBinding}, AttestationRefs: []contract.HumanAttestationExactRefV2{attestation.ExactRef()}, Evidence: evidence, EvidenceSetDigest: evidenceDigest, ReasonCodes: []string{"review/quorum"}, State: contract.HumanVerdictAcceptedV2, ExpiresUnixNano: base.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	currentPanel := nextSourcePanelV5(t, deciding, contract.HumanPanelDecidedV2, base.Add(6*time.Second))
	currentCase := caseDecision
	currentCase.Revision++
	currentCase.State, currentCase.UpdatedUnixNano = contract.CaseResolvedV1, base.Add(6*time.Second).UnixNano()
	currentCase.VerdictID, currentCase.VerdictRevision, currentCase.VerdictDigest, currentCase.Digest = verdict.ID, verdict.Revision, verdict.Digest, ""
	currentCase, err = contract.SealReviewCaseV1(currentCase)
	if err != nil {
		t.Fatal(err)
	}

	facts := newCurrentFactsReaderV5(target, caseDecision, currentCase, round, []contract.HumanReviewPanelV2{proposed, open, quorumPanel, deciding, currentPanel}, assignment, attestation, quorum, verdict)
	targetRef := humanDecisionTargetV5(target)
	assignmentRef := humanDecisionAssignmentV5(assignment)
	actorFact, reviewerFact := authorityFactsFromFixtureV5(t, external)
	actorSubject := runtimeports.ReviewDecisionAuthorityCurrentSubjectV1{Role: runtimeports.ReviewDecisionAuthorityActorV1, Target: targetRef, Assignment: assignmentRef, Authority: target.ActorAuthority, ActionScopeDigest: target.ActionScopeDigest}
	reviewerSubject := runtimeports.ReviewDecisionAuthorityCurrentSubjectV1{Role: runtimeports.ReviewDecisionAuthorityReviewerV1, Target: targetRef, Assignment: assignmentRef, Authority: assignment.ReviewerAuthority, ActionScopeDigest: target.ActionScopeDigest}
	authority := &humanAuthorityReaderV2{values: map[runtimeports.ReviewDecisionAuthorityCurrentSubjectV1]runtimeports.ReviewDecisionAuthorityCurrentProjectionV1{actorSubject: sealExternalAuthorityV1(t, actorSubject, actorFact, base, base.Add(8*time.Minute)), reviewerSubject: sealExternalAuthorityV1(t, reviewerSubject, reviewerFact, base, base.Add(7*time.Minute))}}
	scopeSubject := runtimeports.ReviewDecisionScopeCurrentSubjectV1{TenantID: target.TenantID, Target: targetRef, RunID: target.RunID, Scope: target.Scope, CurrentScope: target.CurrentScope, ActionScopeDigest: target.ActionScopeDigest}
	scope := &humanScopeReaderV2{value: sealExternalScopeV1(t, scopeSubject, external.scope.value.Fact, base, base.Add(9*time.Minute))}
	binding := newFixedHumanBindingReaderV5(t, external, assignment, target)
	decisionRequest := reviewport.DecisionExternalCurrentRequestV1{Target: target, Evidence: evidence}
	evidenceReader := newExternalEvidenceReaderWithExpiryV1(t, decisionRequest, evidence, base, base.Add(4*time.Minute))
	intent := currentSourceIntentV5(t, target, payload, caseDecision.ID)
	satisfaction := noSatisfactionReaderV5{}
	external.clock.value = base.Add(10 * time.Second)
	source, err := NewCurrentFactSourceV5(HumanCurrentSourceDependenciesV5{Facts: facts, Organization: external.organization, Binding: binding, Evidence: evidenceReader, Policy: external.policy, Authority: authority, Scope: scope, Satisfaction: satisfaction, Subjects: []HumanOrganizationSubjectBindingV5{{Assignment: assignment.ExactRef(), ReviewerSubjectID: "reviewer-human-v2", DelegatorSubjectID: "manager-human-v2"}}, Clock: external.clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	return currentSourceFixtureV5{clock: external.clock, source: source, policy: external.policy, intent: intent, verdict: verdict}
}

func nextSourcePanelV5(t *testing.T, value contract.HumanReviewPanelV2, state contract.HumanPanelStateV2, at time.Time) contract.HumanReviewPanelV2 {
	t.Helper()
	value = value.Clone()
	value.Revision++
	value.State, value.UpdatedUnixNano, value.Digest = state, at.UnixNano(), ""
	sealed, err := contract.SealHumanReviewPanelV2(value)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

type currentFactsReaderV5 struct {
	mu           sync.Mutex
	target       contract.TargetSnapshotV1
	currentCase  contract.ReviewCaseV1
	cases        map[reviewport.ExactFactRefV1]contract.ReviewCaseV1
	rounds       map[reviewport.ExactFactRefV1]contract.ReviewRoundV1
	panels       map[contract.HumanPanelExactRefV2]contract.HumanReviewPanelV2
	currentPanel contract.HumanReviewPanelV2
	assignment   contract.HumanPanelAssignmentV2
	attestation  contract.HumanAttestationV2
	quorum       contract.HumanQuorumDecisionV2
	verdict      contract.HumanVerdictV2
}

func newCurrentFactsReaderV5(target contract.TargetSnapshotV1, decisionCase, currentCase contract.ReviewCaseV1, round contract.ReviewRoundV1, panels []contract.HumanReviewPanelV2, assignment contract.HumanPanelAssignmentV2, attestation contract.HumanAttestationV2, quorum contract.HumanQuorumDecisionV2, verdict contract.HumanVerdictV2) *currentFactsReaderV5 {
	r := &currentFactsReaderV5{target: target, currentCase: currentCase, cases: map[reviewport.ExactFactRefV1]contract.ReviewCaseV1{}, rounds: map[reviewport.ExactFactRefV1]contract.ReviewRoundV1{}, panels: map[contract.HumanPanelExactRefV2]contract.HumanReviewPanelV2{}, currentPanel: panels[len(panels)-1], assignment: assignment, attestation: attestation, quorum: quorum, verdict: verdict}
	r.cases[reviewport.ExactV1(decisionCase.ID, decisionCase.Revision, decisionCase.Digest)] = decisionCase
	r.cases[reviewport.ExactV1(currentCase.ID, currentCase.Revision, currentCase.Digest)] = currentCase
	r.rounds[reviewport.ExactV1(round.ID, round.Revision, round.Digest)] = round
	for _, panel := range panels {
		r.panels[panel.ExactRef()] = panel.Clone()
	}
	return r
}

func (r *currentFactsReaderV5) InspectTargetExactV1(_ context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.TargetSnapshotV1, error) {
	if tenant != r.target.TenantID || ref != reviewport.ExactV1(r.target.ID, r.target.Revision, r.target.Digest) {
		return contract.TargetSnapshotV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Target exact ref drifted")
	}
	value := r.target
	value.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), value.Evidence...)
	return value, nil
}
func (r *currentFactsReaderV5) InspectCaseV1(context.Context, core.TenantID, string) (contract.ReviewCaseV1, error) {
	return r.currentCase, nil
}
func (r *currentFactsReaderV5) InspectCaseExactV1(_ context.Context, _ core.TenantID, ref reviewport.ExactFactRefV1) (contract.ReviewCaseV1, error) {
	value, ok := r.cases[ref]
	if !ok {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Case absent")
	}
	return value, nil
}
func (r *currentFactsReaderV5) InspectRoundExactV1(_ context.Context, _ core.TenantID, ref reviewport.ExactFactRefV1) (contract.ReviewRoundV1, error) {
	value, ok := r.rounds[ref]
	if !ok {
		return contract.ReviewRoundV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Round absent")
	}
	return value, nil
}
func (r *currentFactsReaderV5) InspectHumanPanelCurrentV2(context.Context, core.TenantID, string) (contract.HumanReviewPanelV2, error) {
	return r.currentPanel.Clone(), nil
}
func (r *currentFactsReaderV5) InspectHumanPanelExactV2(_ context.Context, ref contract.HumanPanelExactRefV2) (contract.HumanReviewPanelV2, error) {
	value, ok := r.panels[ref]
	if !ok {
		return contract.HumanReviewPanelV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Panel absent")
	}
	return value.Clone(), nil
}
func (r *currentFactsReaderV5) InspectHumanPanelAssignmentExactV2(_ context.Context, ref contract.HumanPanelAssignmentExactRefV2) (contract.HumanPanelAssignmentV2, error) {
	if ref != r.assignment.ExactRef() {
		return contract.HumanPanelAssignmentV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Assignment drifted")
	}
	return r.assignment.Clone(), nil
}
func (r *currentFactsReaderV5) InspectHumanAttestationExactV2(_ context.Context, ref contract.HumanAttestationExactRefV2) (contract.HumanAttestationV2, error) {
	if ref != r.attestation.ExactRef() {
		return contract.HumanAttestationV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Attestation drifted")
	}
	return r.attestation.Clone(), nil
}
func (r *currentFactsReaderV5) InspectHumanQuorumDecisionExactV2(_ context.Context, ref contract.HumanQuorumDecisionExactRefV2) (contract.HumanQuorumDecisionV2, error) {
	if ref != r.quorum.ExactRef() {
		return contract.HumanQuorumDecisionV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Quorum drifted")
	}
	return r.quorum.Clone(), nil
}
func (r *currentFactsReaderV5) InspectHumanVerdictExactV2(_ context.Context, ref contract.HumanVerdictExactRefV2) (contract.HumanVerdictV2, error) {
	if ref != r.verdict.ExactRef() {
		return contract.HumanVerdictV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Verdict drifted")
	}
	return r.verdict.Clone(), nil
}

type fixedHumanBindingReaderV5 struct {
	value runtimeports.ReviewBindingCurrentProjectionV1
}

func newFixedHumanBindingReaderV5(t *testing.T, external humanExternalFixtureV2, assignment contract.HumanPanelAssignmentV2, target contract.TargetSnapshotV1) *fixedHumanBindingReaderV5 {
	t.Helper()
	oldSubject := runtimeports.ReviewBindingSubjectV1{TenantID: external.target.TenantID, AssignmentID: external.assignment.ID, AssignmentRevision: external.assignment.Revision, AssignmentDigest: external.assignment.Digest, ReviewerID: external.assignment.ReviewerIdentity.Ref, TargetID: external.target.ID, TargetRevision: external.target.Revision, TargetDigest: external.target.Digest}
	oldRef, err := external.binding.inner.ResolveCurrentReviewBindingV1(context.Background(), runtimeports.ResolveReviewBindingCurrentRequestV1{Source: external.assignment.ReviewerBinding, Subject: oldSubject})
	if err != nil {
		t.Fatal(err)
	}
	old, err := external.binding.inner.InspectCurrentReviewBindingV1(context.Background(), runtimeports.InspectCurrentReviewBindingRequestV1{ExpectedRef: oldRef, ExpectedSource: external.assignment.ReviewerBinding, ExpectedSubject: oldSubject})
	if err != nil {
		t.Fatal(err)
	}
	value := old.CloneV1()
	value.Subject = runtimeports.ReviewBindingSubjectV1{TenantID: target.TenantID, AssignmentID: assignment.ID, AssignmentRevision: assignment.Revision, AssignmentDigest: assignment.Digest, ReviewerID: assignment.ReviewerIdentity.Ref, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest}
	value.Ref = runtimeports.ReviewBindingProjectionRefV1{Revision: 1}
	value.ClosureDigest, value.ProjectionDigest = "", ""
	value, err = runtimeports.SealReviewBindingCurrentProjectionV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return &fixedHumanBindingReaderV5{value: value}
}
func (r *fixedHumanBindingReaderV5) ResolveCurrentReviewBindingV1(_ context.Context, request runtimeports.ResolveReviewBindingCurrentRequestV1) (runtimeports.ReviewBindingProjectionRefV1, error) {
	if request.Source != r.value.Source || request.Subject != r.value.Subject {
		return runtimeports.ReviewBindingProjectionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Binding resolve drifted")
	}
	return r.value.Ref, nil
}
func (r *fixedHumanBindingReaderV5) InspectReviewBindingProjectionV1(context.Context, runtimeports.InspectReviewBindingProjectionRequestV1) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
	return r.value.CloneV1(), nil
}
func (r *fixedHumanBindingReaderV5) InspectCurrentReviewBindingV1(_ context.Context, request runtimeports.InspectCurrentReviewBindingRequestV1) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
	if request.ExpectedRef != r.value.Ref || request.ExpectedSource != r.value.Source || request.ExpectedSubject != r.value.Subject {
		return runtimeports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Binding inspect drifted")
	}
	return r.value.CloneV1(), nil
}

func authorityFactsFromFixtureV5(t *testing.T, fixture humanExternalFixtureV2) (runtimeports.DispatchAuthorityFactV2, runtimeports.DispatchAuthorityFactV2) {
	t.Helper()
	var actor, reviewer runtimeports.DispatchAuthorityFactV2
	for subject, projection := range fixture.authority.values {
		if subject.Role == runtimeports.ReviewDecisionAuthorityActorV1 {
			actor = projection.Fact
		} else {
			reviewer = projection.Fact
		}
	}
	if actor.Ref == "" || reviewer.Ref == "" {
		t.Fatal("authority fixture is incomplete")
	}
	return actor, reviewer
}

type noSatisfactionReaderV5 struct{}

func (noSatisfactionReaderV5) InspectConditionSatisfactionByVerdict(context.Context, string) (runtimeports.ConditionSatisfactionFactV2, error) {
	return runtimeports.ConditionSatisfactionFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Satisfaction absent")
}

func currentSourceIntentV5(t *testing.T, target contract.TargetSnapshotV1, payload []byte, caseID string) runtimeports.OperationEffectIntentV3 {
	t.Helper()
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(target.Scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeRunV3, ExecutionScope: target.Scope, ExecutionScopeDigest: scopeDigest, RunID: target.RunID, SubjectRevision: 1, CurrentProjectionRef: target.CurrentScope.Ref, CurrentProjectionRevision: target.CurrentScope.Revision, CurrentProjectionDigest: target.CurrentScope.Digest}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	ownerDigest := externalDigestV1("owner-human-source-v5")
	intent := runtimeports.OperationEffectIntentV3{ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: target.IntentID, Revision: target.IntentRevision, Operation: operation, Kind: "review.test/human-source-v5", RiskClass: "review.test/controlled", ActionScopeDigest: target.ActionScopeDigest, Payload: runtimeports.OpaquePayloadV2{Schema: target.PayloadSchema, ContentDigest: target.PayloadDigest, Length: uint64(len(payload)), Inline: append([]byte(nil), payload...), LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "review.test/payload-limit", Digest: externalDigestV1("payload-limit-human-source-v5")}}, PayloadRevision: target.PayloadRevision, Target: target.ID, ConflictDomain: runtimeports.ConflictDomainBindingV2{Domain: "review.test/human-source-v5", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(target.TenantID)}, Owners: []runtimeports.EffectOwnerRefV2{{Role: runtimeports.OwnerCleanup, ComponentID: "review.test/owner", ManifestDigest: ownerDigest}, {Role: runtimeports.OwnerEffect, ComponentID: "review.test/owner", ManifestDigest: ownerDigest}, {Role: runtimeports.OwnerSettlement, ComponentID: "review.test/owner", ManifestDigest: ownerDigest}}, Provider: runtimeports.ProviderBindingRefV2{BindingSetID: "provider-human-source-v5", BindingSetRevision: 1, ComponentID: "review.test/provider", ManifestDigest: externalDigestV1("provider-manifest-human-source-v5"), ArtifactDigest: externalDigestV1("provider-artifact-human-source-v5"), Capability: "review.test/execute"}, Authority: target.ActorAuthority, Review: runtimeports.OperationReviewBindingRefV3{CaseRef: caseID, CandidateDigest: target.Digest, CandidateRevision: target.Revision, PolicyDigest: target.Policy.Digest}, Budget: runtimeports.OperationBudgetBindingRefV3{Ref: "budget-human-source-v5", Revision: 1, Digest: externalDigestV1("budget-human-source-v5"), PolicyDigest: externalDigestV1("budget-policy-human-source-v5"), SubjectDigest: operationDigest}, Policy: runtimeports.OperationPolicyBindingRefV3{Ref: "operation-policy-human-source-v5", Revision: 1, Digest: externalDigestV1("operation-policy-human-source-v5"), SubjectDigest: operationDigest}, Idempotency: runtimeports.IdempotencyBindingV2{Key: "idempotency-human-source-v5", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(target.TenantID), Class: core.IdempotencyQueryable}, CredentialLeases: []runtimeports.CredentialLeaseRefV2{}, ExpiresUnixNano: target.ExpiresUnixNano}
	if err = intent.Validate(); err != nil {
		t.Fatal(err)
	}
	return intent
}
