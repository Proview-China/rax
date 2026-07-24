package decisioncurrent

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// HumanReviewFactReaderV5 is the Review-owned, read-only slice required to
// assemble one Human quorum cut. Implementations may expose broader mutation
// ports elsewhere; this component never receives them.
type HumanReviewFactReaderV5 interface {
	InspectTargetExactV1(context.Context, core.TenantID, reviewport.ExactFactRefV1) (contract.TargetSnapshotV1, error)
	InspectCaseV1(context.Context, core.TenantID, string) (contract.ReviewCaseV1, error)
	InspectCaseExactV1(context.Context, core.TenantID, reviewport.ExactFactRefV1) (contract.ReviewCaseV1, error)
	InspectRoundExactV1(context.Context, core.TenantID, reviewport.ExactFactRefV1) (contract.ReviewRoundV1, error)
	InspectHumanPanelCurrentV2(context.Context, core.TenantID, string) (contract.HumanReviewPanelV2, error)
	InspectHumanPanelExactV2(context.Context, contract.HumanPanelExactRefV2) (contract.HumanReviewPanelV2, error)
	InspectHumanPanelAssignmentExactV2(context.Context, contract.HumanPanelAssignmentExactRefV2) (contract.HumanPanelAssignmentV2, error)
	InspectHumanAttestationExactV2(context.Context, contract.HumanAttestationExactRefV2) (contract.HumanAttestationV2, error)
	InspectHumanQuorumDecisionExactV2(context.Context, contract.HumanQuorumDecisionExactRefV2) (contract.HumanQuorumDecisionV2, error)
	InspectHumanVerdictExactV2(context.Context, contract.HumanVerdictExactRefV2) (contract.HumanVerdictV2, error)
}

type HumanConditionSatisfactionReaderV5 interface {
	InspectConditionSatisfactionByVerdict(context.Context, string) (runtimeports.ConditionSatisfactionFactV2, error)
}

// HumanOrganizationSubjectBindingV5 is immutable host composition data. It is
// lookup material only and is always checked against the exact Assignment.
type HumanOrganizationSubjectBindingV5 struct {
	Assignment         contract.HumanPanelAssignmentExactRefV2 `json:"assignment"`
	ReviewerSubjectID  string                                  `json:"reviewer_subject_id"`
	DelegatorSubjectID string                                  `json:"delegator_subject_id,omitempty"`
}

type HumanCurrentSourceDependenciesV5 struct {
	Facts        HumanReviewFactReaderV5
	Organization reviewport.HumanOrganizationCurrentReaderV2
	Binding      runtimeports.ReviewBindingAuthoritativeCurrentReaderV1
	Evidence     runtimeports.ReviewEvidenceApplicabilityCurrentReaderV1
	Policy       runtimeports.HumanQuorumPolicyCurrentReaderV2
	Authority    runtimeports.ReviewDecisionAuthorityCurrentReaderV1
	Scope        runtimeports.ReviewDecisionScopeCurrentReaderV1
	Satisfaction HumanConditionSatisfactionReaderV5
	Subjects     []HumanOrganizationSubjectBindingV5
	Bypass       *BypassCurrentSourceDependenciesV5
	Clock        func() time.Time
}

// CurrentFactSourceV5 is the Review-owned production assembler for Human
// quorum currentness. The policy-not-required branch remains fail-closed until
// its three missing Owner exact Readers are public; it never type-puns Human
// Assignment or Review Binding coordinates.
type CurrentFactSourceV5 struct {
	facts        HumanReviewFactReaderV5
	organization reviewport.HumanOrganizationCurrentReaderV2
	binding      runtimeports.ReviewBindingAuthoritativeCurrentReaderV1
	evidence     runtimeports.ReviewEvidenceApplicabilityCurrentReaderV1
	policy       runtimeports.HumanQuorumPolicyCurrentReaderV2
	authority    runtimeports.ReviewDecisionAuthorityCurrentReaderV1
	scope        runtimeports.ReviewDecisionScopeCurrentReaderV1
	satisfaction HumanConditionSatisfactionReaderV5
	subjects     map[string]HumanOrganizationSubjectBindingV5
	bypass       *BypassCurrentFactSourceV5
	clock        func() time.Time
}

func NewCurrentFactSourceV5(d HumanCurrentSourceDependenciesV5) (*CurrentFactSourceV5, error) {
	if nilcheck.IsNil(d.Facts) || nilcheck.IsNil(d.Organization) || nilcheck.IsNil(d.Binding) || nilcheck.IsNil(d.Evidence) || nilcheck.IsNil(d.Policy) || nilcheck.IsNil(d.Authority) || nilcheck.IsNil(d.Scope) || nilcheck.IsNil(d.Satisfaction) || d.Clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review V5 Human current source requires all read-only Owner dependencies")
	}
	subjects := make(map[string]HumanOrganizationSubjectBindingV5, len(d.Subjects))
	for _, value := range d.Subjects {
		if err := value.Assignment.Validate(); err != nil {
			return nil, err
		}
		if strings.TrimSpace(value.ReviewerSubjectID) == "" {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Human Organization subject binding is incomplete")
		}
		key := humanAssignmentKeyV5(value.Assignment)
		if previous, ok := subjects[key]; ok && previous != value {
			return nil, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Human Organization subject binding conflicts")
		}
		subjects[key] = value
	}
	var bypass *BypassCurrentFactSourceV5
	if d.Bypass != nil {
		dependencies := *d.Bypass
		if dependencies.Clock == nil {
			dependencies.Clock = d.Clock
		}
		var err error
		bypass, err = NewBypassCurrentFactSourceV5(dependencies)
		if err != nil {
			return nil, err
		}
	}
	return &CurrentFactSourceV5{facts: d.Facts, organization: d.Organization, binding: d.Binding, evidence: d.Evidence, policy: d.Policy, authority: d.Authority, scope: d.Scope, satisfaction: d.Satisfaction, subjects: subjects, bypass: bypass, clock: d.Clock}, nil
}

func (s *CurrentFactSourceV5) InspectReviewCurrentFactsV5(ctx context.Context, request runtimeadapter.ExactCurrentRequestV5) (runtimeadapter.CurrentFactSnapshotV5, error) {
	if err := request.Validate(); err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	if request.Basis == runtimeports.OperationReviewBasisPolicyNotRequiredV5 {
		if s.bypass == nil {
			return runtimeadapter.CurrentFactSnapshotV5{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "policy-not-required production source requires public PolicyDecision, actor Authority, Scope and Provider Binding exact Readers")
		}
		return s.bypass.InspectBypassCurrentFactsV5(ctx, request)
	}
	if request.Basis != runtimeports.OperationReviewBasisAcceptedQuorumV5 && request.Basis != runtimeports.OperationReviewBasisConditionalQuorumSatisfiedV5 {
		return runtimeadapter.CurrentFactSnapshotV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Review V5 current basis is unsupported")
	}
	baseline := s.clock()
	if baseline.IsZero() {
		return runtimeadapter.CurrentFactSnapshotV5{}, clockRegressionV1("Review V5 source baseline clock is unavailable")
	}
	recoveryCtx, recoveryCancel, recoveryReady := boundedDetachedRecoveryV1(ctx, baseline, request.Intent.ExpiresUnixNano)
	if !recoveryReady {
		return runtimeadapter.CurrentFactSnapshotV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review V5 source recovery window is unavailable or expired")
	}
	defer recoveryCancel()
	first, err := s.readHumanCutV5(ctx, recoveryCtx, request)
	if err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	afterS1 := s.clock()
	if afterS1.IsZero() || afterS1.Before(baseline) {
		return runtimeadapter.CurrentFactSnapshotV5{}, clockRegressionV1("Review V5 source clock regressed across S1")
	}
	s2Ctx, s2Cancel, s2Ready := tightenDetachedRecoveryV1(recoveryCtx, afterS1, minimumHumanCutExpiryV5(first))
	if !s2Ready {
		return runtimeadapter.CurrentFactSnapshotV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review V5 Human S1 snapshot expired before S2")
	}
	defer s2Cancel()
	second, err := s.readHumanCutV5(s2Ctx, s2Ctx, request)
	if err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	now := s.clock()
	if now.IsZero() || now.Before(afterS1) {
		return runtimeadapter.CurrentFactSnapshotV5{}, clockRegressionV1("Review V5 source clock regressed across S2")
	}
	if !reflect.DeepEqual(first.review, second.review) || !reflect.DeepEqual(first.external, second.external) {
		return runtimeadapter.CurrentFactSnapshotV5{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Review V5 exact/current cut drifted between S1 and S2")
	}
	return s.projectHumanCutV5(request, second, now)
}

var _ runtimeadapter.CurrentFactSourceV5 = (*CurrentFactSourceV5)(nil)

type humanReviewCutV5 struct {
	target        contract.TargetSnapshotV1
	decisionCase  contract.ReviewCaseV1
	currentCase   contract.ReviewCaseV1
	round         contract.ReviewRoundV1
	decisionPanel contract.HumanReviewPanelV2
	currentPanel  contract.HumanReviewPanelV2
	quorum        contract.HumanQuorumDecisionV2
	verdict       contract.HumanVerdictV2
	assignments   []contract.HumanPanelAssignmentV2
	attestations  []contract.HumanAttestationV2
	caseHistory   []contract.ReviewCaseV1
	panelHistory  []contract.HumanReviewPanelV2
}

type humanExternalItemV5 struct {
	assignment contract.HumanPanelAssignmentExactRefV2
	actor      runtimeports.ReviewDecisionAuthorityCurrentProjectionV1
	reviewer   runtimeports.ReviewDecisionAuthorityCurrentProjectionV1
	binding    runtimeports.ReviewBindingCurrentProjectionV1
}

type humanExternalCutV5 struct {
	policy       runtimeports.HumanQuorumPolicyCurrentProjectionV2
	scope        runtimeports.ReviewDecisionScopeCurrentProjectionV1
	items        []humanExternalItemV5
	evidence     []runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1
	organization reviewport.HumanOrganizationCurrentCutV2
	satisfaction *runtimeports.ConditionSatisfactionFactV2
}

type humanCompleteCutV5 struct {
	review   humanReviewCutV5
	external humanExternalCutV5
}

func (s *CurrentFactSourceV5) readHumanCutV5(ctx, recoveryCtx context.Context, request runtimeadapter.ExactCurrentRequestV5) (humanCompleteCutV5, error) {
	review, err := s.readHumanReviewFactsV5(ctx, recoveryCtx, request)
	if err != nil {
		return humanCompleteCutV5{}, err
	}
	external, err := s.readHumanExternalV5(ctx, recoveryCtx, review, request.Basis)
	if err != nil {
		return humanCompleteCutV5{}, err
	}
	return humanCompleteCutV5{review: review, external: external}, nil
}

func (s *CurrentFactSourceV5) readHumanReviewFactsV5(ctx, recoveryCtx context.Context, request runtimeadapter.ExactCurrentRequestV5) (humanReviewCutV5, error) {
	intent := request.Intent
	tenant := intent.Operation.ExecutionScope.Identity.TenantID
	target, _, err := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (contract.TargetSnapshotV1, error) {
		return s.facts.InspectTargetExactV1(call, tenant, reviewport.ExactV1(intent.Target, intent.Review.CandidateRevision, intent.Review.CandidateDigest))
	})
	if err != nil {
		return humanReviewCutV5{}, err
	}
	currentCase, _, err := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (contract.ReviewCaseV1, error) {
		return s.facts.InspectCaseV1(call, tenant, intent.Review.CaseRef)
	})
	if err != nil {
		return humanReviewCutV5{}, err
	}
	if currentCase.VerdictID == "" || currentCase.VerdictRevision == 0 || currentCase.VerdictDigest == "" {
		return humanReviewCutV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictMissing, "current Human Case has no exact Verdict")
	}
	verdict, _, err := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (contract.HumanVerdictV2, error) {
		return s.facts.InspectHumanVerdictExactV2(call, contract.HumanVerdictExactRefV2{TenantID: tenant, ID: currentCase.VerdictID, Revision: currentCase.VerdictRevision, Digest: currentCase.VerdictDigest})
	})
	if err != nil {
		return humanReviewCutV5{}, err
	}
	decisionCase, _, err := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (contract.ReviewCaseV1, error) {
		return s.facts.InspectCaseExactV1(call, tenant, reviewport.ExactV1(verdict.Case.ID, verdict.Case.Revision, verdict.Case.Digest))
	})
	if err != nil {
		return humanReviewCutV5{}, err
	}
	decisionPanel, _, err := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (contract.HumanReviewPanelV2, error) {
		return s.facts.InspectHumanPanelExactV2(call, verdict.Panel)
	})
	if err != nil {
		return humanReviewCutV5{}, err
	}
	currentPanel, _, err := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (contract.HumanReviewPanelV2, error) {
		return s.facts.InspectHumanPanelCurrentV2(call, tenant, verdict.Panel.ID)
	})
	if err != nil {
		return humanReviewCutV5{}, err
	}
	quorum, _, err := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (contract.HumanQuorumDecisionV2, error) {
		return s.facts.InspectHumanQuorumDecisionExactV2(call, verdict.QuorumDecision)
	})
	if err != nil {
		return humanReviewCutV5{}, err
	}
	round, _, err := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (contract.ReviewRoundV1, error) {
		return s.facts.InspectRoundExactV1(call, verdict.Round.TenantID, reviewport.ExactV1(verdict.Round.ID, verdict.Round.Revision, verdict.Round.Digest))
	})
	if err != nil {
		return humanReviewCutV5{}, err
	}
	assignments := make([]contract.HumanPanelAssignmentV2, 0, len(decisionPanel.AssignmentRefs))
	for _, ref := range decisionPanel.AssignmentRefs {
		value, _, readErr := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (contract.HumanPanelAssignmentV2, error) {
			return s.facts.InspectHumanPanelAssignmentExactV2(call, ref)
		})
		if readErr != nil {
			return humanReviewCutV5{}, readErr
		}
		assignments = append(assignments, value)
	}
	attestationRefs := append(append([]contract.HumanAttestationExactRefV2(nil), quorum.AcceptedAttestationRefs...), quorum.OtherAttestationRefs...)
	attestations := make([]contract.HumanAttestationV2, 0, len(attestationRefs))
	for _, ref := range attestationRefs {
		value, _, readErr := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (contract.HumanAttestationV2, error) {
			return s.facts.InspectHumanAttestationExactV2(call, ref)
		})
		if readErr != nil {
			return humanReviewCutV5{}, readErr
		}
		attestations = append(attestations, value)
	}
	cases, panels, err := s.readHumanHistoryV5(ctx, recoveryCtx, decisionCase, decisionPanel, quorum, assignments, attestations)
	if err != nil {
		return humanReviewCutV5{}, err
	}
	return humanReviewCutV5{target: target, decisionCase: decisionCase, currentCase: currentCase, round: round, decisionPanel: decisionPanel, currentPanel: currentPanel, quorum: quorum, verdict: verdict, assignments: assignments, attestations: attestations, caseHistory: cases, panelHistory: panels}, nil
}

func retryValueV5[T any](ctx, recoveryCtx context.Context, clock func() time.Time, read func(context.Context) (T, error)) (T, bool, error) {
	return retryExactReadV1(ctx, recoveryCtx, clock, read)
}

// resolveValueV5 retries a weak current lookup only as a new S1. Because no
// expected full Ref exists, it never describes the second result as recovery
// of the unknown first result. A failed new S1 preserves the original Unknown;
// only a detected clock rollback takes precedence.
func resolveValueV5[T any](ctx, recoveryCtx context.Context, clock func() time.Time, read func(context.Context) (T, error)) (T, error) {
	value, err := singleReadV1(ctx, clock, read)
	if err == nil || !unknownReadV1(err) {
		return value, err
	}
	originalUnknown := err
	if recoveryCtx == nil {
		var zero T
		return zero, originalUnknown
	}
	value, err = singleReadV1(recoveryCtx, clock, read)
	if err != nil || recoveryCtx.Err() != nil {
		if core.HasReason(err, core.ReasonClockRegression) {
			var zero T
			return zero, err
		}
		var zero T
		return zero, originalUnknown
	}
	return value, nil
}

func humanAssignmentKeyV5(ref contract.HumanPanelAssignmentExactRefV2) string {
	return string(ref.TenantID) + "\x00" + ref.ID + "\x00" + string(ref.Digest)
}

func (s *CurrentFactSourceV5) readHumanHistoryV5(ctx, recoveryCtx context.Context, decisionCase contract.ReviewCaseV1, decisionPanel contract.HumanReviewPanelV2, quorum contract.HumanQuorumDecisionV2, assignments []contract.HumanPanelAssignmentV2, attestations []contract.HumanAttestationV2) ([]contract.ReviewCaseV1, []contract.HumanReviewPanelV2, error) {
	caseRefs := map[string]contract.HumanCaseExactRefV2{}
	panelRefs := map[string]contract.HumanPanelExactRefV2{}
	addCase := func(ref contract.HumanCaseExactRefV2) { caseRefs[string(ref.Digest)] = ref }
	addPanel := func(ref contract.HumanPanelExactRefV2) { panelRefs[string(ref.Digest)] = ref }
	addCase(contract.HumanCaseExactRefV2{TenantID: decisionCase.TenantID, ID: decisionCase.ID, Revision: decisionCase.Revision, Digest: decisionCase.Digest})
	addPanel(decisionPanel.ExactRef())
	addPanel(quorum.Panel)
	for _, value := range assignments {
		addCase(value.Case)
		addPanel(value.Panel)
	}
	for _, value := range attestations {
		addCase(value.Case)
		addPanel(value.Panel)
	}
	cases := make([]contract.ReviewCaseV1, 0, len(caseRefs))
	for _, ref := range caseRefs {
		value, _, err := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (contract.ReviewCaseV1, error) {
			return s.facts.InspectCaseExactV1(call, ref.TenantID, reviewport.ExactV1(ref.ID, ref.Revision, ref.Digest))
		})
		if err != nil {
			return nil, nil, err
		}
		cases = append(cases, value)
	}
	panels := make([]contract.HumanReviewPanelV2, 0, len(panelRefs))
	for _, ref := range panelRefs {
		value, _, err := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (contract.HumanReviewPanelV2, error) {
			return s.facts.InspectHumanPanelExactV2(call, ref)
		})
		if err != nil {
			return nil, nil, err
		}
		panels = append(panels, value)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Revision < cases[j].Revision })
	sort.Slice(panels, func(i, j int) bool { return panels[i].Revision < panels[j].Revision })
	return cases, panels, nil
}

func (s *CurrentFactSourceV5) readHumanExternalV5(ctx, recoveryCtx context.Context, review humanReviewCutV5, basis runtimeports.OperationReviewAuthorizationBasisV5) (humanExternalCutV5, error) {
	targetRef := humanDecisionTargetV5(review.target)
	policySubject := runtimeports.HumanQuorumPolicyCurrentSubjectV2{TenantID: review.decisionPanel.TenantID, Domain: review.decisionPanel.QuorumPolicy.Domain}
	policyRef, err := resolveValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.HumanQuorumPolicyCurrentProjectionRefV2, error) {
		return s.policy.ResolveCurrentHumanQuorumPolicyV2(call, runtimeports.HumanQuorumPolicyCurrentResolveRequestV2{Subject: policySubject})
	})
	if err != nil {
		return humanExternalCutV5{}, err
	}
	wantPolicy := runtimeports.HumanQuorumPolicyCurrentProjectionRefV2{ID: review.decisionPanel.QuorumPolicy.Ref, Revision: review.decisionPanel.QuorumPolicy.Revision, Digest: review.decisionPanel.QuorumPolicy.Digest}
	if policyRef != wantPolicy {
		return humanExternalCutV5{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Human quorum Policy current ref drifted from Panel")
	}
	policy, _, err := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.HumanQuorumPolicyCurrentProjectionV2, error) {
		return s.policy.InspectCurrentHumanQuorumPolicyV2(call, policySubject, policyRef)
	})
	if err != nil {
		return humanExternalCutV5{}, err
	}
	scopeSubject := runtimeports.ReviewDecisionScopeCurrentSubjectV1{TenantID: review.target.TenantID, Target: targetRef, RunID: review.target.RunID, Scope: review.target.Scope, CurrentScope: review.target.CurrentScope, ActionScopeDigest: review.target.ActionScopeDigest}
	scopeRef, err := resolveValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ReviewDecisionScopeCurrentProjectionRefV1, error) {
		return s.scope.ResolveCurrentReviewDecisionScopeV1(call, runtimeports.ReviewDecisionScopeCurrentResolveRequestV1{Subject: scopeSubject})
	})
	if err != nil {
		return humanExternalCutV5{}, err
	}
	scope, _, err := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ReviewDecisionScopeCurrentProjectionV1, error) {
		return s.scope.InspectCurrentReviewDecisionScopeV1(call, scopeSubject, scopeRef)
	})
	if err != nil {
		return humanExternalCutV5{}, err
	}
	items := make([]humanExternalItemV5, 0, len(review.assignments))
	organizationRequests := make([]reviewport.HumanOrganizationCurrentRequestV2, 0, len(review.assignments))
	for _, assignment := range review.assignments {
		assignmentRef := humanDecisionAssignmentV5(assignment)
		actorSubject := runtimeports.ReviewDecisionAuthorityCurrentSubjectV1{Role: runtimeports.ReviewDecisionAuthorityActorV1, Target: targetRef, Assignment: assignmentRef, Authority: review.target.ActorAuthority, ActionScopeDigest: review.target.ActionScopeDigest}
		actor, readErr := s.readAuthorityV5(ctx, recoveryCtx, actorSubject)
		if readErr != nil {
			return humanExternalCutV5{}, readErr
		}
		reviewerSubject := runtimeports.ReviewDecisionAuthorityCurrentSubjectV1{Role: runtimeports.ReviewDecisionAuthorityReviewerV1, Target: targetRef, Assignment: assignmentRef, Authority: assignment.ReviewerAuthority, ActionScopeDigest: review.target.ActionScopeDigest}
		reviewer, readErr := s.readAuthorityV5(ctx, recoveryCtx, reviewerSubject)
		if readErr != nil {
			return humanExternalCutV5{}, readErr
		}
		bindingSubject := runtimeports.ReviewBindingSubjectV1{TenantID: review.target.TenantID, AssignmentID: assignment.ID, AssignmentRevision: assignment.Revision, AssignmentDigest: assignment.Digest, ReviewerID: assignment.ReviewerIdentity.Ref, TargetID: review.target.ID, TargetRevision: review.target.Revision, TargetDigest: review.target.Digest}
		bindingRef, readErr := resolveValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ReviewBindingProjectionRefV1, error) {
			return s.binding.ResolveCurrentReviewBindingV1(call, runtimeports.ResolveReviewBindingCurrentRequestV1{Source: assignment.ReviewerBinding, Subject: bindingSubject})
		})
		if readErr != nil {
			return humanExternalCutV5{}, readErr
		}
		binding, _, readErr := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
			return s.binding.InspectCurrentReviewBindingV1(call, runtimeports.InspectCurrentReviewBindingRequestV1{ExpectedRef: bindingRef, ExpectedSource: assignment.ReviewerBinding, ExpectedSubject: bindingSubject})
		})
		if readErr != nil {
			return humanExternalCutV5{}, readErr
		}
		items = append(items, humanExternalItemV5{assignment: assignment.ExactRef(), actor: actor, reviewer: reviewer, binding: binding})
		subjectBinding, ok := s.subjects[humanAssignmentKeyV5(assignment.ExactRef())]
		if !ok {
			return humanExternalCutV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "Human Organization subject binding is absent")
		}
		organizationRequests = append(organizationRequests, reviewport.HumanOrganizationCurrentRequestV2{Panel: review.decisionPanel, Assignment: assignment, ReviewerSubjectID: subjectBinding.ReviewerSubjectID, DelegatorSubjectID: subjectBinding.DelegatorSubjectID, ActionScopeDigest: review.target.ActionScopeDigest})
	}
	organization, _, err := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (reviewport.HumanOrganizationCurrentCutV2, error) {
		return s.organization.InspectHumanOrganizationCurrentV2(call, organizationRequests)
	})
	if err != nil {
		return humanExternalCutV5{}, err
	}
	var satisfaction *runtimeports.ConditionSatisfactionFactV2
	evidenceRefs := append([]runtimeports.ReviewEvidenceRefV2(nil), review.verdict.Evidence...)
	if basis == runtimeports.OperationReviewBasisConditionalQuorumSatisfiedV5 {
		value, _, readErr := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ConditionSatisfactionFactV2, error) {
			return s.satisfaction.InspectConditionSatisfactionByVerdict(call, review.verdict.ID)
		})
		if readErr != nil {
			return humanExternalCutV5{}, readErr
		}
		satisfaction = &value
		for _, proof := range value.Proofs {
			evidenceRefs = append(evidenceRefs, proof.Evidence)
		}
	}
	evidenceRefs = uniqueReviewEvidenceV5(evidenceRefs)
	evidence := make([]runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, 0, len(evidenceRefs))
	for _, value := range evidenceRefs {
		subject := runtimeports.ReviewEvidenceApplicabilitySubjectV1{TenantID: review.target.TenantID, Target: runtimeports.ReviewEvidenceTargetRefV1{ID: review.target.ID, Revision: review.target.Revision, Digest: review.target.Digest}, RunID: review.target.RunID, Scope: review.target.Scope, ActionScopeDigest: review.target.ActionScopeDigest, ReviewEvidence: value}
		snapshot, readErr := resolveValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
			return s.evidence.ResolveReviewEvidenceApplicabilityCurrentV1(call, runtimeports.ResolveReviewEvidenceApplicabilityCurrentRequestV1{ContractVersion: runtimeports.ReviewEvidenceCurrentContractVersionV1, Subject: subject})
		})
		if readErr != nil {
			return humanExternalCutV5{}, readErr
		}
		evidence = append(evidence, snapshot)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].assignment.ID < items[j].assignment.ID })
	sort.Slice(evidence, func(i, j int) bool {
		return evidence[i].Projection.Subject.ReviewEvidence.Ref < evidence[j].Projection.Subject.ReviewEvidence.Ref
	})
	return humanExternalCutV5{policy: policy, scope: scope, items: items, evidence: evidence, organization: organization, satisfaction: satisfaction}, nil
}

func (s *CurrentFactSourceV5) readAuthorityV5(ctx, recoveryCtx context.Context, subject runtimeports.ReviewDecisionAuthorityCurrentSubjectV1) (runtimeports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
	ref, err := resolveValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1, error) {
		return s.authority.ResolveCurrentReviewDecisionAuthorityV1(call, runtimeports.ReviewDecisionAuthorityCurrentResolveRequestV1{Subject: subject})
	})
	if err != nil {
		return runtimeports.ReviewDecisionAuthorityCurrentProjectionV1{}, err
	}
	value, _, err := retryValueV5(ctx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
		return s.authority.InspectCurrentReviewDecisionAuthorityV1(call, subject, ref)
	})
	return value, err
}

func humanDecisionTargetV5(target contract.TargetSnapshotV1) runtimeports.ReviewDecisionTargetRefV1 {
	return runtimeports.ReviewDecisionTargetRefV1{TenantID: target.TenantID, ID: target.ID, Revision: target.Revision, Digest: target.Digest, RunID: target.RunID}
}

func humanDecisionAssignmentV5(assignment contract.HumanPanelAssignmentV2) runtimeports.ReviewDecisionAssignmentRefV1 {
	return runtimeports.ReviewDecisionAssignmentRefV1{TenantID: assignment.TenantID, ID: assignment.ID, Revision: assignment.Revision, Digest: assignment.Digest, ReviewerID: assignment.ReviewerIdentity.Ref}
}

func uniqueReviewEvidenceV5(values []runtimeports.ReviewEvidenceRefV2) []runtimeports.ReviewEvidenceRefV2 {
	byRef := make(map[string]runtimeports.ReviewEvidenceRefV2, len(values))
	for _, value := range values {
		if previous, ok := byRef[value.Ref]; ok && previous != value {
			// Preserve both so the downstream exact validation rejects the drift.
			return append([]runtimeports.ReviewEvidenceRefV2(nil), values...)
		}
		byRef[value.Ref] = value
	}
	out := make([]runtimeports.ReviewEvidenceRefV2, 0, len(byRef))
	for _, value := range byRef {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out
}

func (s *CurrentFactSourceV5) projectHumanCutV5(request runtimeadapter.ExactCurrentRequestV5, cut humanCompleteCutV5, now time.Time) (runtimeadapter.CurrentFactSnapshotV5, error) {
	r, x := cut.review, cut.external
	if err := validateHumanExternalCutV5(r, x, request.Basis, now, s.subjects); err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	expires := minimumHumanCutExpiryV5(cut)
	if expires <= now.UnixNano() {
		return runtimeadapter.CurrentFactSnapshotV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review V5 Human completed cut expired")
	}
	targetRef := contract.HumanTargetExactRefV2{TenantID: r.target.TenantID, ID: r.target.ID, Revision: r.target.Revision, Digest: r.target.Digest}
	policy, err := sealHumanPolicyReceiptFromProjectionV5(targetRef, r.verdict.Policy, x.policy, expires)
	if err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	scope, err := sealOwnerReceiptFromProjectionV5("scope", targetRef, nil, r.verdict.CurrentScope.Ref, r.verdict.CurrentScope.Revision, r.verdict.CurrentScope.Digest, x.scope.Ref.ID, x.scope.Ref.Revision, x.scope.Ref.Digest, "", false, x.scope.CheckedUnixNano, x.scope.ExpiresUnixNano, expires)
	if err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	actorReceipts := make([]runtimeadapter.OwnerCurrentReceiptV5, 0, len(x.items))
	reviewerReceipts := make([]runtimeadapter.OwnerCurrentReceiptV5, 0, len(x.items))
	bindingReceipts := make([]runtimeadapter.OwnerCurrentReceiptV5, 0, len(x.items))
	assignments := make(map[string]contract.HumanPanelAssignmentV2, len(r.assignments))
	for _, assignment := range r.assignments {
		assignments[assignment.ID] = assignment
	}
	for _, item := range x.items {
		assignment := assignments[item.assignment.ID]
		assignmentRef := assignment.ExactRef()
		actor, sealErr := sealOwnerReceiptFromProjectionV5("actor_authority", targetRef, &assignmentRef, r.target.ActorAuthority.Ref, r.target.ActorAuthority.Revision, r.target.ActorAuthority.Digest, item.actor.Ref.ID, item.actor.Ref.Revision, item.actor.Ref.Digest, "", false, item.actor.CheckedUnixNano, item.actor.ExpiresUnixNano, expires)
		if sealErr != nil {
			return runtimeadapter.CurrentFactSnapshotV5{}, sealErr
		}
		reviewer, sealErr := sealOwnerReceiptFromProjectionV5("reviewer_authority", targetRef, &assignmentRef, assignment.ReviewerAuthority.Ref, assignment.ReviewerAuthority.Revision, assignment.ReviewerAuthority.Digest, item.reviewer.Ref.ID, item.reviewer.Ref.Revision, item.reviewer.Ref.Digest, "", false, item.reviewer.CheckedUnixNano, item.reviewer.ExpiresUnixNano, expires)
		if sealErr != nil {
			return runtimeadapter.CurrentFactSnapshotV5{}, sealErr
		}
		binding, sealErr := sealBindingReceiptFromProjectionV5(targetRef, assignment, item.binding, expires)
		if sealErr != nil {
			return runtimeadapter.CurrentFactSnapshotV5{}, sealErr
		}
		actorReceipts = append(actorReceipts, actor)
		reviewerReceipts = append(reviewerReceipts, reviewer)
		bindingReceipts = append(bindingReceipts, binding)
	}
	evidenceByRef := make(map[string]runtimeadapter.EvidenceCurrentReceiptV5, len(x.evidence))
	for _, value := range x.evidence {
		projection := value.Projection
		receipt, sealErr := runtimeadapter.SealEvidenceCurrentReceiptV5(runtimeadapter.EvidenceCurrentReceiptV5{Target: targetRef, Review: projection.Subject.ReviewEvidence, Applicability: projection.Ref, Ledger: projection.Record, Current: true, CheckedUnixNano: projection.CheckedUnixNano, SourceExpiresUnixNano: projection.ExpiresUnixNano, ExpiresUnixNano: expires})
		if sealErr != nil {
			return runtimeadapter.CurrentFactSnapshotV5{}, sealErr
		}
		evidenceByRef[receipt.Review.Ref] = receipt
	}
	decisionEvidence, err := selectEvidenceReceiptsV5(r.verdict.Evidence, evidenceByRef)
	if err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	var satisfactionEvidence []runtimeadapter.EvidenceCurrentReceiptV5
	if x.satisfaction != nil {
		refs := make([]runtimeports.ReviewEvidenceRefV2, 0, len(x.satisfaction.Proofs))
		for _, proof := range x.satisfaction.Proofs {
			refs = append(refs, proof.Evidence)
		}
		satisfactionEvidence, err = selectEvidenceReceiptsV5(refs, evidenceByRef)
		if err != nil {
			return runtimeadapter.CurrentFactSnapshotV5{}, err
		}
	}
	quorum := runtimeadapter.QuorumCurrentSnapshotV5{DecisionCase: r.decisionCase, CurrentCase: r.currentCase, CaseHistory: append([]contract.ReviewCaseV1(nil), r.caseHistory...), Round: r.round, DecisionPanel: r.decisionPanel.Clone(), CurrentPanel: r.currentPanel.Clone(), PanelHistory: clonePanelsV5(r.panelHistory), Quorum: r.quorum.Clone(), Verdict: r.verdict.Clone(), Assignments: cloneAssignmentsV5(r.assignments), Attestations: cloneAttestationsV5(r.attestations), OrganizationCut: x.organization.Clone(), Policy: policy, Scope: scope, ActorAuthorities: actorReceipts, ReviewerAuthorities: reviewerReceipts, Bindings: bindingReceipts, Evidence: decisionEvidence, Satisfaction: cloneSatisfactionV5(x.satisfaction), SatisfactionEvidence: satisfactionEvidence}
	snapshot := runtimeadapter.CurrentFactSnapshotV5{Revision: r.currentCase.Revision, Basis: request.Basis, Target: r.target, Quorum: &quorum, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}
	return runtimeadapter.SealCurrentFactSnapshotV5(snapshot)
}

func sealHumanPolicyReceiptFromProjectionV5(target contract.HumanTargetExactRefV2, source contract.HumanQuorumPolicyBindingV2, projection runtimeports.HumanQuorumPolicyCurrentProjectionV2, cutExpires int64) (runtimeadapter.OwnerCurrentReceiptV5, error) {
	projectionRef := projection.Ref
	return runtimeadapter.SealOwnerCurrentReceiptV5(runtimeadapter.OwnerCurrentReceiptV5{Kind: "policy", Target: target, HumanQuorumPolicySource: &source, HumanQuorumPolicyProjection: &projectionRef, SourceRef: source.Ref, SourceRevision: source.Revision, SourceDigest: source.Digest, Projection: runtimeports.OperationGovernanceFactRefV3{Ref: projection.Ref.ID, Revision: projection.Ref.Revision, Digest: projection.Ref.Digest, ExpiresUnixNano: cutExpires}, Current: true, CheckedUnixNano: projection.CheckedUnixNano, SourceExpiresUnixNano: projection.ExpiresUnixNano, ExpiresUnixNano: cutExpires})
}

func sealOwnerReceiptFromProjectionV5(kind string, target contract.HumanTargetExactRefV2, assignment *contract.HumanPanelAssignmentExactRefV2, sourceRef string, sourceRevision core.Revision, sourceDigest core.Digest, projectionRef string, projectionRevision core.Revision, projectionDigest core.Digest, policyDecision string, operationNotRequired bool, checked, sourceExpires, cutExpires int64) (runtimeadapter.OwnerCurrentReceiptV5, error) {
	return runtimeadapter.SealOwnerCurrentReceiptV5(runtimeadapter.OwnerCurrentReceiptV5{Kind: kind, Target: target, Assignment: assignment, SourceRef: sourceRef, SourceRevision: sourceRevision, SourceDigest: sourceDigest, PolicyDecisionRef: policyDecision, PolicyOperationNotRequired: operationNotRequired, Projection: runtimeports.OperationGovernanceFactRefV3{Ref: projectionRef, Revision: projectionRevision, Digest: projectionDigest, ExpiresUnixNano: cutExpires}, Current: true, CheckedUnixNano: checked, SourceExpiresUnixNano: sourceExpires, ExpiresUnixNano: cutExpires})
}

func sealBindingReceiptFromProjectionV5(target contract.HumanTargetExactRefV2, assignment contract.HumanPanelAssignmentV2, projection runtimeports.ReviewBindingCurrentProjectionV1, cutExpires int64) (runtimeadapter.OwnerCurrentReceiptV5, error) {
	source := assignment.ReviewerBinding
	sourceDigest, err := core.CanonicalJSONDigest("praxis.review.runtime-current", "praxis.review.runtime-current/v5", "ReviewComponentBindingRefV2", source)
	if err != nil {
		return runtimeadapter.OwnerCurrentReceiptV5{}, err
	}
	assignmentRef, projectionRef := assignment.ExactRef(), projection.Ref
	return runtimeadapter.SealOwnerCurrentReceiptV5(runtimeadapter.OwnerCurrentReceiptV5{Kind: "binding", Target: target, Assignment: &assignmentRef, ReviewBindingSource: &source, ReviewBindingProjection: &projectionRef, SourceRef: source.BindingSetID, SourceRevision: source.BindingSetRevision, SourceDigest: sourceDigest, Projection: runtimeports.OperationGovernanceFactRefV3{Ref: projection.Ref.ID, Revision: projection.Ref.Revision, Digest: projection.Ref.Digest, ExpiresUnixNano: cutExpires}, Current: true, CheckedUnixNano: projection.CheckedUnixNano, SourceExpiresUnixNano: projection.ExpiresUnixNano, ExpiresUnixNano: cutExpires})
}

func validateHumanExternalCutV5(review humanReviewCutV5, external humanExternalCutV5, basis runtimeports.OperationReviewAuthorizationBasisV5, now time.Time, subjects map[string]HumanOrganizationSubjectBindingV5) error {
	targetRef := humanDecisionTargetV5(review.target)
	policySubject := runtimeports.HumanQuorumPolicyCurrentSubjectV2{TenantID: review.decisionPanel.TenantID, Domain: review.decisionPanel.QuorumPolicy.Domain}
	if err := external.policy.ValidateCurrent(external.policy.Ref, policySubject, now); err != nil {
		return err
	}
	wantPolicy := runtimeports.HumanQuorumPolicyCurrentProjectionRefV2{ID: review.verdict.Policy.Ref, Revision: review.verdict.Policy.Revision, Digest: review.verdict.Policy.Digest}
	if external.policy.Ref != wantPolicy || external.policy.CheckedUnixNano != review.verdict.Policy.CheckedUnixNano || external.policy.ExpiresUnixNano != review.verdict.Policy.ExpiresUnixNano || external.policy.AcceptThreshold != review.decisionPanel.AcceptThreshold || external.policy.MaximumPanelSize != review.decisionPanel.MaximumPanelSize || external.policy.DelegationRequired != review.decisionPanel.DelegationRequired || external.policy.ProductionSelfReviewAllowed != review.decisionPanel.ProductionSelfReviewAllowed || external.policy.MaxPanelDurationNanos != review.decisionPanel.MaxPanelDurationNanos || external.policy.MaxVoteTTLNanos != review.decisionPanel.MaxVoteTTLNanos || !sameHumanPolicyRolesV5(external.policy.RoleRequirements, review.decisionPanel.RoleRequirements) || !reflect.DeepEqual(external.policy.RejectVetoRoles, review.decisionPanel.RejectVetoRoles) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Human quorum Policy projection drifted from Panel and Verdict")
	}
	scopeSubject := runtimeports.ReviewDecisionScopeCurrentSubjectV1{TenantID: review.target.TenantID, Target: targetRef, RunID: review.target.RunID, Scope: review.target.Scope, CurrentScope: review.target.CurrentScope, ActionScopeDigest: review.target.ActionScopeDigest}
	if err := external.scope.ValidateCurrent(external.scope.Ref, scopeSubject, now); err != nil {
		return err
	}
	if err := external.organization.Validate(now); err != nil {
		return err
	}
	assignments := make(map[string]contract.HumanPanelAssignmentV2, len(review.assignments))
	for _, assignment := range review.assignments {
		assignments[assignment.ID] = assignment
	}
	if len(external.items) != len(assignments) || len(external.organization.Items) != len(assignments) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Human external Owner cut cardinality drifted")
	}
	organizationByAssignment := make(map[string]reviewport.HumanOrganizationAssignmentCurrentV2, len(external.organization.Items))
	for _, item := range external.organization.Items {
		organizationByAssignment[item.Assignment.ID] = item
	}
	for _, item := range external.items {
		assignment, ok := assignments[item.assignment.ID]
		if !ok || assignment.ExactRef() != item.assignment {
			return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Human external cut Assignment drifted")
		}
		assignmentRef := humanDecisionAssignmentV5(assignment)
		actorSubject := runtimeports.ReviewDecisionAuthorityCurrentSubjectV1{Role: runtimeports.ReviewDecisionAuthorityActorV1, Target: targetRef, Assignment: assignmentRef, Authority: review.target.ActorAuthority, ActionScopeDigest: review.target.ActionScopeDigest}
		if err := item.actor.ValidateCurrent(item.actor.Ref, actorSubject, now); err != nil {
			return err
		}
		reviewerSubject := runtimeports.ReviewDecisionAuthorityCurrentSubjectV1{Role: runtimeports.ReviewDecisionAuthorityReviewerV1, Target: targetRef, Assignment: assignmentRef, Authority: assignment.ReviewerAuthority, ActionScopeDigest: review.target.ActionScopeDigest}
		if err := item.reviewer.ValidateCurrent(item.reviewer.Ref, reviewerSubject, now); err != nil {
			return err
		}
		bindingSubject := runtimeports.ReviewBindingSubjectV1{TenantID: review.target.TenantID, AssignmentID: assignment.ID, AssignmentRevision: assignment.Revision, AssignmentDigest: assignment.Digest, ReviewerID: assignment.ReviewerIdentity.Ref, TargetID: review.target.ID, TargetRevision: review.target.Revision, TargetDigest: review.target.Digest}
		if err := item.binding.ValidateCurrent(item.binding.Ref, assignment.ReviewerBinding, bindingSubject, now); err != nil {
			return err
		}
		subject, ok := subjects[humanAssignmentKeyV5(assignment.ExactRef())]
		if !ok {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "Human Organization subject binding is absent")
		}
		request := reviewport.HumanOrganizationCurrentRequestV2{Panel: review.decisionPanel, Assignment: assignment, ReviewerSubjectID: subject.ReviewerSubjectID, DelegatorSubjectID: subject.DelegatorSubjectID, ActionScopeDigest: review.target.ActionScopeDigest}
		organizationItem, ok := organizationByAssignment[assignment.ID]
		if !ok {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Human Organization current item is absent")
		}
		if err := organizationItem.Validate(request, now); err != nil {
			return err
		}
	}
	for _, value := range external.evidence {
		if err := value.ValidateCurrent(value.Projection.Ref, now); err != nil {
			return err
		}
		if value.Projection.Subject.Target.ID != review.target.ID || value.Projection.Subject.Target.Revision != review.target.Revision || value.Projection.Subject.Target.Digest != review.target.Digest || value.Projection.Subject.RunID != review.target.RunID || !runtimeports.SameExecutionScopeV2(value.Projection.Subject.Scope, review.target.Scope) || value.Projection.Subject.ActionScopeDigest != review.target.ActionScopeDigest {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Human Evidence applicability drifted from Target")
		}
	}
	if basis == runtimeports.OperationReviewBasisConditionalQuorumSatisfiedV5 {
		if external.satisfaction == nil || external.satisfaction.Validate() != nil || external.satisfaction.VerdictID != review.verdict.ID || external.satisfaction.State != runtimeports.ConditionSatisfied {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "conditional Human quorum lacks current Satisfaction")
		}
	} else if external.satisfaction != nil {
		return core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "accepted Human quorum carried Satisfaction")
	}
	return nil
}

func sameHumanPolicyRolesV5(left []runtimeports.HumanQuorumRoleRequirementV2, right []contract.HumanRoleRequirementV2) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Role != right[index].Role || left[index].Minimum != right[index].Minimum {
			return false
		}
	}
	return true
}

func minimumHumanCutExpiryV5(cut humanCompleteCutV5) int64 {
	r, x := cut.review, cut.external
	values := []int64{r.target.ExpiresUnixNano, r.decisionCase.ExpiresUnixNano, r.currentCase.ExpiresUnixNano, r.round.ExpiresUnixNano, r.decisionPanel.ExpiresUnixNano, r.currentPanel.ExpiresUnixNano, r.quorum.ExpiresUnixNano, r.verdict.ExpiresUnixNano, x.policy.ExpiresUnixNano, x.scope.ExpiresUnixNano, x.organization.ExpiresUnixNano}
	for _, value := range r.assignments {
		values = append(values, value.ExpiresUnixNano, value.LeaseExpiresUnixNano)
	}
	for _, value := range r.attestations {
		values = append(values, value.ExpiresUnixNano)
	}
	for _, value := range r.caseHistory {
		values = append(values, value.ExpiresUnixNano)
	}
	for _, value := range r.panelHistory {
		values = append(values, value.ExpiresUnixNano)
	}
	for _, item := range x.items {
		values = append(values, item.actor.ExpiresUnixNano, item.reviewer.ExpiresUnixNano, item.binding.ExpiresUnixNano)
	}
	for _, value := range x.evidence {
		values = append(values, value.Projection.ExpiresUnixNano)
	}
	if x.satisfaction != nil {
		values = append(values, x.satisfaction.ExpiresUnixNano)
		for _, proof := range x.satisfaction.Proofs {
			values = append(values, proof.ExpiresUnixNano)
		}
	}
	return minimumPositive(values...)
}

func selectEvidenceReceiptsV5(refs []runtimeports.ReviewEvidenceRefV2, byRef map[string]runtimeadapter.EvidenceCurrentReceiptV5) ([]runtimeadapter.EvidenceCurrentReceiptV5, error) {
	out := make([]runtimeadapter.EvidenceCurrentReceiptV5, 0, len(refs))
	for _, ref := range refs {
		value, ok := byRef[ref.Ref]
		if !ok || value.Review != ref {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Review Evidence exact current receipt is absent")
		}
		out = append(out, value)
	}
	return out, nil
}

func clonePanelsV5(values []contract.HumanReviewPanelV2) []contract.HumanReviewPanelV2 {
	out := append([]contract.HumanReviewPanelV2(nil), values...)
	for index := range out {
		out[index] = out[index].Clone()
	}
	return out
}
func cloneAssignmentsV5(values []contract.HumanPanelAssignmentV2) []contract.HumanPanelAssignmentV2 {
	out := append([]contract.HumanPanelAssignmentV2(nil), values...)
	for index := range out {
		out[index] = out[index].Clone()
	}
	return out
}
func cloneAttestationsV5(values []contract.HumanAttestationV2) []contract.HumanAttestationV2 {
	out := append([]contract.HumanAttestationV2(nil), values...)
	for index := range out {
		out[index] = out[index].Clone()
	}
	return out
}
func cloneSatisfactionV5(value *runtimeports.ConditionSatisfactionFactV2) *runtimeports.ConditionSatisfactionFactV2 {
	if value == nil {
		return nil
	}
	out := *value
	out.Proofs = append([]runtimeports.ReviewConditionProofV2(nil), value.Proofs...)
	return &out
}
