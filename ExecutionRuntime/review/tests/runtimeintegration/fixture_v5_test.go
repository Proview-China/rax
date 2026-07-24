package runtimeintegration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	organizationcontract "github.com/Proview-China/rax/ExecutionRuntime/organization-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type fixtureV5 struct {
	now      time.Time
	request  runtimeports.OperationReviewCurrentRequestV5
	snapshot runtimeadapter.CurrentFactSnapshotV5
}

func humanCaseRefV5(c contract.ReviewCaseV1) contract.HumanCaseExactRefV2 {
	return contract.HumanCaseExactRefV2{TenantID: c.TenantID, ID: c.ID, Revision: c.Revision, Digest: c.Digest}
}
func humanTargetRefV5(v contract.TargetSnapshotV1) contract.HumanTargetExactRefV2 {
	return contract.HumanTargetExactRefV2{TenantID: v.TenantID, ID: v.ID, Revision: v.Revision, Digest: v.Digest}
}
func humanRoundRefV5(v contract.ReviewRoundV1) contract.HumanRoundExactRefV2 {
	return contract.HumanRoundExactRefV2{TenantID: v.TenantID, ID: v.ID, Revision: v.Revision, Digest: v.Digest}
}
func humanIdentityRefV5(tenant core.TenantID, id string) contract.HumanIdentityProofRefV2 {
	ref, _ := organizationcontract.DeriveIdentityIDV1(tenant, id)
	return contract.HumanIdentityProofRefV2{TenantID: tenant, Ref: ref, Revision: 1, Digest: digest("identity-" + id)}
}
func nextCaseV5(t *testing.T, c contract.ReviewCaseV1, state contract.CaseStateV1, at time.Time, verdict *contract.HumanVerdictV2) contract.ReviewCaseV1 {
	t.Helper()
	c.Revision++
	c.State = state
	c.UpdatedUnixNano = at.UnixNano()
	c.Digest = ""
	c.VerdictID, c.VerdictRevision, c.VerdictDigest = "", 0, ""
	if verdict != nil {
		c.VerdictID, c.VerdictRevision, c.VerdictDigest = verdict.ID, verdict.Revision, verdict.Digest
	}
	v, err := contract.SealReviewCaseV1(c)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func nextPanelV5(t *testing.T, p contract.HumanReviewPanelV2, state contract.HumanPanelStateV2, at time.Time) contract.HumanReviewPanelV2 {
	t.Helper()
	p = p.Clone()
	p.Revision++
	p.State = state
	p.UpdatedUnixNano = at.UnixNano()
	p.Digest = ""
	v, err := contract.SealHumanReviewPanelV2(p)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func ownerReceiptV5(t *testing.T, target contract.TargetSnapshotV1, assignment *contract.HumanPanelAssignmentExactRefV2, kind, id string, revision core.Revision, sourceDigest core.Digest, policyDecision string, operationNotRequired bool, checked time.Time, expires int64) runtimeadapter.OwnerCurrentReceiptV5 {
	t.Helper()
	v, err := runtimeadapter.SealOwnerCurrentReceiptV5(runtimeadapter.OwnerCurrentReceiptV5{Kind: kind, Target: humanTargetRefV5(target), Assignment: assignment, SourceRef: id, SourceRevision: revision, SourceDigest: sourceDigest, PolicyDecisionRef: policyDecision, PolicyOperationNotRequired: operationNotRequired, Projection: runtimeports.OperationGovernanceFactRefV3{Ref: id, Revision: revision, Digest: sourceDigest, ExpiresUnixNano: expires}, Current: true, CheckedUnixNano: checked.UnixNano(), SourceExpiresUnixNano: expires, ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func bindingReceiptV5(t *testing.T, target contract.TargetSnapshotV1, assignment contract.HumanPanelAssignmentV2, checked time.Time, expires int64) runtimeadapter.OwnerCurrentReceiptV5 {
	t.Helper()
	source := assignment.ReviewerBinding
	nominalID := source.BindingSetID + "/" + string(source.ComponentID) + "/" + string(source.Capability)
	nominalDigest, _ := core.CanonicalJSONDigest("praxis.review.runtime-current", "praxis.review.runtime-current/v5", "ReviewComponentBindingRefV2", source)
	projection := runtimeports.ReviewBindingProjectionRefV1{ID: "binding-current-" + assignment.ID, Revision: 1, Digest: digest("binding-current-" + assignment.ID)}
	assignmentRef := assignment.ExactRef()
	v, err := runtimeadapter.SealOwnerCurrentReceiptV5(runtimeadapter.OwnerCurrentReceiptV5{Kind: "binding", Target: humanTargetRefV5(target), Assignment: &assignmentRef, ReviewBindingSource: &source, ReviewBindingProjection: &projection, SourceRef: nominalID, SourceRevision: source.BindingSetRevision, SourceDigest: nominalDigest, Projection: runtimeports.OperationGovernanceFactRefV3{Ref: projection.ID, Revision: projection.Revision, Digest: projection.Digest, ExpiresUnixNano: expires}, Current: true, CheckedUnixNano: checked.UnixNano(), SourceExpiresUnixNano: expires, ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func evidenceReceiptV5(t *testing.T, target contract.TargetSnapshotV1, review runtimeports.ReviewEvidenceRefV2, sequence uint64, checked time.Time, expires int64) runtimeadapter.EvidenceCurrentReceiptV5 {
	t.Helper()
	subjectDigest := digest("applicability-subject-" + review.Ref)
	projectionID, err := runtimeports.DeriveReviewEvidenceApplicabilityProjectionIDV1(subjectDigest)
	if err != nil {
		t.Fatal(err)
	}
	applicability := runtimeports.ReviewEvidenceApplicabilityRefV1{ProjectionID: projectionID, Revision: 1, SubjectDigest: subjectDigest, Digest: digest("applicability-" + review.Ref)}
	v, err := runtimeadapter.SealEvidenceCurrentReceiptV5(runtimeadapter.EvidenceCurrentReceiptV5{Target: humanTargetRefV5(target), Review: review, Applicability: applicability, Ledger: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: digest("ledger-scope"), Sequence: sequence, RecordDigest: digest("record-" + review.Ref)}, Current: true, CheckedUnixNano: checked.UnixNano(), SourceExpiresUnixNano: expires, ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func newQuorumFixtureV5(t *testing.T) fixtureV5 {
	t.Helper()
	base := newFixtureV4(t, contract.VerdictAcceptedV1, false)
	now, target, intent := base.now, base.snapshot.Target, base.intent
	cutExpiry := now.Add(7 * time.Minute).UnixNano()
	target.ExpiresUnixNano = cutExpiry
	target.Digest = ""
	var err error
	target, err = contract.SealTargetSnapshotV1(target)
	if err != nil {
		t.Fatal(err)
	}
	intent.Review.CandidateDigest = target.Digest
	c1, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: intent.Review.CaseRef, Revision: 1, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), UpdatedUnixNano: now.Add(-time.Minute).UnixNano()}, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, State: contract.CaseRoutedV1, ExpiresUnixNano: cutExpiry})
	if err != nil {
		t.Fatal(err)
	}
	c2 := nextCaseV5(t, c1, contract.CaseReviewingV1, now.Add(time.Second), nil)
	c3 := nextCaseV5(t, c2, contract.CaseAttestedV1, now.Add(2*time.Second), nil)
	c4 := nextCaseV5(t, c3, contract.CaseDecidingV1, now.Add(3*time.Second), nil)
	round, err := contract.SealReviewRoundV1(contract.ReviewRoundV1{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "round-v5", Revision: 1, CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.UnixNano()}, CaseID: c1.ID, CaseRevision: c1.Revision, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, Route: contract.RouteHumanV1, State: contract.RoundAttestedV1, AssignmentID: "panel-v5", ContextFrameDigest: target.ContextFrameDigest, RubricDigest: digest("rubric-v5"), ExpiresUnixNano: cutExpiry})
	if err != nil {
		t.Fatal(err)
	}
	policy := contract.HumanQuorumPolicyBindingV2{TenantID: target.TenantID, Ref: target.Policy.Ref, Revision: target.Policy.Revision, Digest: target.Policy.Digest, Domain: "review/quorum", CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: cutExpiry}
	authorIdentity := humanIdentityRefV5(target.TenantID, "author-v5")
	responsibilityID, _ := organizationcontract.DeriveResponsibilityIDV1(target.TenantID, "review-target", target.ID)
	responsibility := contract.HumanResponsibilitySubjectRefV2{TenantID: target.TenantID, Ref: responsibilityID, Revision: 1, Digest: digest("responsibility-v5"), IdentityProof: authorIdentity}
	p1, err := contract.SealHumanReviewPanelV2(contract.HumanReviewPanelV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, Case: humanCaseRefV5(c1), Target: humanTargetRefV5(target), Round: humanRoundRefV5(round), QuorumPolicy: policy, ResponsibilitySubject: responsibility, State: contract.HumanPanelProposedV2, AcceptThreshold: 2, MaximumPanelSize: 2, RoleRequirements: []contract.HumanRoleRequirementV2{{Role: "security", Minimum: 1}, {Role: "technical", Minimum: 1}}, RejectVetoRoles: []string{"security"}, DelegationRequired: true, MaxPanelDurationNanos: int64(10 * time.Minute), MaxVoteTTLNanos: int64(7 * time.Minute), ExpiresUnixNano: cutExpiry})
	if err != nil {
		t.Fatal(err)
	}
	makeAssignment := func(name, role string) contract.HumanPanelAssignmentV2 {
		identity := humanIdentityRefV5(target.TenantID, name)
		delegationID, _ := organizationcontract.DeriveDelegationIDV1(target.TenantID, "manager-v5", name, role, target.ActionScopeDigest)
		binding := runtimeports.ReviewComponentBindingRefV2{BindingSetID: "set-" + name, BindingSetRevision: 1, ComponentID: "review.test/human", ManifestDigest: digest("manifest-" + name), ArtifactDigest: digest("artifact-" + name), Capability: "review.test/attest"}
		v, e := contract.SealHumanPanelAssignmentV2(contract.HumanPanelAssignmentV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "assignment-" + name, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, Panel: p1.ExactRef(), Case: p1.Case, Round: p1.Round, Target: p1.Target, ReviewerIdentity: identity, ReviewerAuthority: authority("authority-" + name), ReviewerBinding: binding, Roles: []string{role}, CanVeto: role == "security", Delegated: true, DelegatorIdentity: humanIdentityRefV5(target.TenantID, "manager-v5"), DelegateIdentity: identity, DelegationFact: contract.HumanDelegationFactRefV2{TenantID: target.TenantID, Ref: delegationID, Revision: 1, Digest: digest("delegation-" + name)}, DelegatedRole: role, DelegationScopeDigest: target.ActionScopeDigest, State: contract.HumanAssignmentClaimedV2, LeaseHolder: name, LeaseExpiresUnixNano: cutExpiry, ExpiresUnixNano: cutExpiry})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	assignments := []contract.HumanPanelAssignmentV2{makeAssignment("reviewer-a-v5", "security"), makeAssignment("reviewer-b-v5", "technical")}
	p2 := p1.Clone()
	p2.Revision = 2
	p2.State = contract.HumanPanelOpenV2
	p2.UpdatedUnixNano = now.Add(time.Second).UnixNano()
	p2.AssignmentRefs = []contract.HumanPanelAssignmentExactRefV2{assignments[0].ExactRef(), assignments[1].ExactRef()}
	p2.Digest = ""
	p2, err = contract.SealHumanReviewPanelV2(p2)
	if err != nil {
		t.Fatal(err)
	}
	p3 := nextPanelV5(t, p2, contract.HumanPanelOpenV2, now.Add(2*time.Second))
	p4 := nextPanelV5(t, p3, contract.HumanPanelQuorumSatisfiedV2, now.Add(3*time.Second))
	p5 := nextPanelV5(t, p4, contract.HumanPanelDecidingV2, now.Add(4*time.Second))
	makeAtt := func(panel contract.HumanReviewPanelV2, a contract.HumanPanelAssignmentV2, seq int) contract.HumanAttestationV2 {
		ev := []runtimeports.ReviewEvidenceRefV2{{Ref: "evidence-v5-" + a.ID, Classification: "review.test/human", Digest: digest("evidence-v5-" + a.ID)}}
		ed, _ := contract.ComputeReviewEvidenceDigestV1(ev)
		v, e := contract.SealHumanAttestationV2(contract.HumanAttestationV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "attestation-v5-" + a.ID, Revision: 1, CreatedUnixNano: now.Add(time.Duration(seq) * time.Second).UnixNano(), UpdatedUnixNano: now.Add(time.Duration(seq) * time.Second).UnixNano()}, IdempotencyKey: "idem-v5-" + a.ID, Panel: panel.ExactRef(), Assignment: a.ExactRef(), Case: a.Case, Round: a.Round, Target: a.Target, Policy: policy, ResponsibilitySubject: responsibility, ReviewerIdentity: a.ReviewerIdentity, ReviewerAuthority: a.ReviewerAuthority, Delegation: &a.DelegationFact, ReviewerBinding: a.ReviewerBinding, Resolution: contract.ResolutionAcceptV1, ReasonCodes: []string{"review/verified"}, Evidence: ev, EvidenceDigest: ed, ObservedUnixNano: now.Add(time.Duration(seq) * time.Second).UnixNano(), ExpiresUnixNano: cutExpiry})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	attestations := []contract.HumanAttestationV2{makeAtt(p2, assignments[0], 1), makeAtt(p3, assignments[1], 2)}
	evidence := append(append([]runtimeports.ReviewEvidenceRefV2{}, attestations[0].Evidence...), attestations[1].Evidence...)
	evidenceDigest, _ := contract.ComputeReviewEvidenceDigestV1(evidence)
	ids := []contract.HumanIdentityProofRefV2{assignments[0].ReviewerIdentity, assignments[1].ReviewerIdentity}
	reviewerSet, _ := contract.ComputeHumanReviewerSetDigestV2(ids)
	quorum, err := contract.SealHumanQuorumDecisionV2(contract.HumanQuorumDecisionV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "quorum-v5", Revision: 1, CreatedUnixNano: now.Add(3 * time.Second).UnixNano(), UpdatedUnixNano: now.Add(3 * time.Second).UnixNano()}, Panel: p4.ExactRef(), Policy: policy, AcceptedAttestationRefs: []contract.HumanAttestationExactRefV2{attestations[0].ExactRef(), attestations[1].ExactRef()}, DistinctReviewerIdentityRefs: ids, SatisfiedRoleCounts: []contract.HumanSatisfiedRoleCountV2{{Role: "security", DistinctCurrentCount: 1}, {Role: "technical", DistinctCurrentCount: 1}}, AcceptCount: 2, Threshold: 2, Resolution: contract.ResolutionAcceptV1, EvidenceSetDigest: evidenceDigest, ReviewerSetDigest: reviewerSet, CheckedUnixNano: now.Add(3 * time.Second).UnixNano(), ExpiresUnixNano: cutExpiry})
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := contract.SealHumanVerdictV2(contract.HumanVerdictV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "human-verdict-v5", Revision: 1, CreatedUnixNano: now.Add(5 * time.Second).UnixNano(), UpdatedUnixNano: now.Add(5 * time.Second).UnixNano()}, Case: humanCaseRefV5(c4), Target: humanTargetRefV5(target), Round: humanRoundRefV5(round), Panel: p5.ExactRef(), QuorumDecision: quorum.ExactRef(), Policy: policy, Scope: target.Scope, CurrentScope: target.CurrentScope, ReviewerSetDigest: reviewerSet, ReviewerAuthorityRefs: []runtimeports.AuthorityBindingRefV2{assignments[0].ReviewerAuthority, assignments[1].ReviewerAuthority}, BindingClosures: []runtimeports.ReviewComponentBindingRefV2{assignments[0].ReviewerBinding, assignments[1].ReviewerBinding}, AttestationRefs: []contract.HumanAttestationExactRefV2{attestations[0].ExactRef(), attestations[1].ExactRef()}, Evidence: evidence, EvidenceSetDigest: evidenceDigest, ReasonCodes: []string{"review/quorum"}, State: contract.HumanVerdictAcceptedV2, ExpiresUnixNano: cutExpiry})
	if err != nil {
		t.Fatal(err)
	}
	p6 := nextPanelV5(t, p5, contract.HumanPanelDecidedV2, now.Add(5*time.Second))
	c5 := nextCaseV5(t, c4, contract.CaseResolvedV1, now.Add(5*time.Second), &verdict)
	orgItems := make([]reviewport.HumanOrganizationAssignmentCurrentV2, 0, 2)
	for _, a := range assignments {
		source := organizationcontract.ReviewEligibilitySourceV1{TenantID: target.TenantID, ReviewerSubjectID: a.LeaseHolder, RequiredRoles: append([]string(nil), a.Roles...), ScopeDigest: target.ActionScopeDigest, ResponsibilitySubjectKind: "review-target", ResponsibilitySubjectID: target.ID, ResponsibilitySubjectDigest: target.Digest, DelegatorSubjectID: "manager-v5", DelegatedRole: a.DelegatedRole, RequireDelegation: true, Production: true}
		owner := organizationcontract.ReviewEligibilityProjectionRefV1{TenantID: target.TenantID, ID: "org-projection-" + a.ID, Source: source, Identity: organizationcontract.IdentityRefV1{TenantID: target.TenantID, ID: a.ReviewerIdentity.Ref, Revision: a.ReviewerIdentity.Revision, Digest: a.ReviewerIdentity.Digest}, Roles: []organizationcontract.RoleGrantRefV1{{TenantID: target.TenantID, ID: "role-" + a.ID, Revision: 1, Digest: digest("role-" + a.ID)}}, Delegation: &organizationcontract.DelegationRefV1{TenantID: target.TenantID, ID: a.DelegationFact.Ref, Revision: a.DelegationFact.Revision, Digest: a.DelegationFact.Digest}, Responsibility: organizationcontract.ResponsibilityRefV1{TenantID: target.TenantID, ID: responsibility.Ref, Revision: responsibility.Revision, Digest: responsibility.Digest}, Digest: digest("org-projection-" + a.ID)}
		orgItems = append(orgItems, reviewport.HumanOrganizationAssignmentCurrentV2{RequestDigest: digest("org-request-" + a.ID), Assignment: a.ExactRef(), ReviewerIdentity: a.ReviewerIdentity, OwnerProjectionRef: owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: cutExpiry, ProjectionDigest: owner.Digest})
	}
	orgCut, err := reviewport.SealHumanOrganizationCurrentCutV2(reviewport.HumanOrganizationCurrentCutV2{TenantID: target.TenantID, Items: orgItems, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: cutExpiry})
	if err != nil {
		t.Fatal(err)
	}
	policyReceipt := ownerReceiptV5(t, target, nil, "policy", policy.Ref, policy.Revision, policy.Digest, "quorum-policy-decision-v5", false, now, cutExpiry)
	scopeReceipt := ownerReceiptV5(t, target, nil, "scope", target.CurrentScope.Ref, target.CurrentScope.Revision, target.CurrentScope.Digest, "", false, now, cutExpiry)
	authReceipts := make([]runtimeadapter.OwnerCurrentReceiptV5, 0, 2)
	actorReceipts := make([]runtimeadapter.OwnerCurrentReceiptV5, 0, 2)
	bindingReceipts := make([]runtimeadapter.OwnerCurrentReceiptV5, 0, 2)
	evidenceReceipts := make([]runtimeadapter.EvidenceCurrentReceiptV5, 0, 2)
	for i, a := range assignments {
		assignmentRef := a.ExactRef()
		actorReceipts = append(actorReceipts, ownerReceiptV5(t, target, &assignmentRef, "actor_authority", target.ActorAuthority.Ref, target.ActorAuthority.Revision, target.ActorAuthority.Digest, "", false, now, cutExpiry))
		authReceipts = append(authReceipts, ownerReceiptV5(t, target, &assignmentRef, "reviewer_authority", a.ReviewerAuthority.Ref, a.ReviewerAuthority.Revision, a.ReviewerAuthority.Digest, "", false, now, cutExpiry))
		bindingReceipts = append(bindingReceipts, bindingReceiptV5(t, target, a, now, cutExpiry))
		evidenceReceipts = append(evidenceReceipts, evidenceReceiptV5(t, target, attestations[i].Evidence[0], uint64(i+1), now, cutExpiry))
	}
	qSnapshot := runtimeadapter.QuorumCurrentSnapshotV5{DecisionCase: c4, CurrentCase: c5, CaseHistory: []contract.ReviewCaseV1{c1, c2, c3, c4}, Round: round, DecisionPanel: p5, CurrentPanel: p6, PanelHistory: []contract.HumanReviewPanelV2{p1, p2, p3, p4, p5}, Quorum: quorum, Verdict: verdict, Assignments: assignments, Attestations: attestations, OrganizationCut: orgCut, Policy: policyReceipt, Scope: scopeReceipt, ActorAuthorities: actorReceipts, ReviewerAuthorities: authReceipts, Bindings: bindingReceipts, Evidence: evidenceReceipts}
	snapshot := runtimeadapter.CurrentFactSnapshotV5{Revision: 1, Basis: runtimeports.OperationReviewBasisAcceptedQuorumV5, Target: target, Quorum: &qSnapshot, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: cutExpiry}
	snapshot, err = runtimeadapter.SealCurrentFactSnapshotV5(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return fixtureV5{now: now, request: runtimeports.OperationReviewCurrentRequestV5{Intent: intent, Basis: runtimeports.OperationReviewBasisAcceptedQuorumV5}, snapshot: snapshot}
}

func newBypassFixtureV5(t *testing.T) fixtureV5 {
	t.Helper()
	base := newFixtureV4(t, contract.VerdictAcceptedV1, false)
	now, target, intent := base.now, base.snapshot.Target, base.intent
	cutExpiry := now.Add(6 * time.Minute).UnixNano()
	target.ExpiresUnixNano = cutExpiry
	target.Digest = ""
	var err error
	target, err = contract.SealTargetSnapshotV1(target)
	if err != nil {
		t.Fatal(err)
	}
	intent.Review.CandidateDigest = target.Digest
	caseFact, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: intent.Review.CaseRef, Revision: 3, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), UpdatedUnixNano: now.UnixNano()}, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, State: contract.CaseRoutedV1, ExpiresUnixNano: cutExpiry})
	if err != nil {
		t.Fatal(err)
	}
	policyProjection := runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1{ID: "policy-current-v5", Revision: 1, Digest: target.Policy.Digest}
	proof, err := contract.SealBypassExternalCurrentProofV1(contract.BypassExternalCurrentProofV1{Policy: policyProjection, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: cutExpiry})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := contract.SealBypassDecisionV1(contract.BypassDecisionV1{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "bypass-v5", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, Target: target.BypassExactRefV1(), Case: caseFact.BypassExactRefV1(), IntentID: intent.ID, IntentRevision: intent.Revision, SubjectDigest: target.SubjectDigest, PayloadRevision: target.PayloadRevision, PayloadDigest: target.PayloadDigest, Scope: target.Scope, RunID: target.RunID, ActionScopeDigest: target.ActionScopeDigest, Policy: target.Policy, PolicyCurrentProjection: policyProjection, PolicyDecisionRef: "policy-decision-v5", ActorAuthority: target.ActorAuthority, CurrentScope: target.CurrentScope, TargetEvidenceSetDigest: target.EvidenceSetDigest, Profile: contract.ProfileYOLOV1, Risk: contract.RiskLowV1, EffectClass: contract.EffectObserveOnlyV1, Environment: contract.EnvironmentProductionV1, RouteDecisionDigest: digest("bypass-route-v5"), ExternalProof: proof, State: contract.BypassDecisionActiveV1, ExpiresUnixNano: cutExpiry})
	if err != nil {
		t.Fatal(err)
	}
	providerDigest, _ := core.CanonicalJSONDigest("praxis.review.runtime-current", "praxis.review.runtime-current/v5", "ProviderBindingRefV2", intent.Provider)
	b := runtimeadapter.BypassCurrentSnapshotV5{CurrentCase: caseFact, Decision: decision, Policy: ownerReceiptV5(t, target, nil, "policy", policyProjection.ID, policyProjection.Revision, policyProjection.Digest, decision.PolicyDecisionRef, true, now, cutExpiry), PolicyDecision: ownerReceiptV5(t, target, nil, "policy_decision", decision.PolicyDecisionRef, decision.Policy.Revision, decision.Policy.Digest, "", false, now, cutExpiry), Authority: ownerReceiptV5(t, target, nil, "actor_authority", target.ActorAuthority.Ref, target.ActorAuthority.Revision, target.ActorAuthority.Digest, "", false, now, cutExpiry), Scope: ownerReceiptV5(t, target, nil, "scope", target.CurrentScope.Ref, target.CurrentScope.Revision, target.CurrentScope.Digest, "", false, now, cutExpiry), Binding: ownerReceiptV5(t, target, nil, "binding", intent.Provider.BindingSetID, intent.Provider.BindingSetRevision, providerDigest, "", false, now, cutExpiry)}
	snapshot := runtimeadapter.CurrentFactSnapshotV5{Revision: 1, Basis: runtimeports.OperationReviewBasisPolicyNotRequiredV5, Target: target, PolicyNotRequired: &b, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: cutExpiry}
	snapshot, err = runtimeadapter.SealCurrentFactSnapshotV5(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return fixtureV5{now: now, request: runtimeports.OperationReviewCurrentRequestV5{Intent: intent, Basis: runtimeports.OperationReviewBasisPolicyNotRequiredV5}, snapshot: snapshot}
}

func newConditionalQuorumFixtureV5(t *testing.T) fixtureV5 {
	t.Helper()
	f := newQuorumFixtureV5(t)
	q := f.snapshot.Quorum
	condition := runtimeports.ReviewConditionV2{ID: "review.test/conditional-v5", Revision: 1, Schema: schema(), ConstraintDigest: digest("condition-constraint-v5"), SatisfactionOwner: q.Assignments[0].ReviewerBinding, ScopeDigest: f.snapshot.Target.ActionScopeDigest, Authority: f.snapshot.Target.ActorAuthority, ExpiresUnixNano: f.snapshot.ExpiresUnixNano}
	conditions := []runtimeports.ReviewConditionV2{condition}
	conditionsDigest, err := runtimeports.DigestReviewConditionsV2(conditions)
	if err != nil {
		t.Fatal(err)
	}
	att := q.Attestations[0].Clone()
	att.Resolution = contract.ResolutionConditionalV1
	att.Conditions, att.ConditionsDigest, att.Digest = conditions, conditionsDigest, ""
	att, err = contract.SealHumanAttestationV2(att)
	if err != nil {
		t.Fatal(err)
	}
	q.Attestations[0] = att
	quorum := q.Quorum.Clone()
	quorum.AcceptedAttestationRefs[0] = att.ExactRef()
	quorum.Resolution, quorum.Conditions, quorum.ConditionsDigest, quorum.Digest = contract.ResolutionConditionalV1, conditions, conditionsDigest, ""
	quorum, err = contract.SealHumanQuorumDecisionV2(quorum)
	if err != nil {
		t.Fatal(err)
	}
	q.Quorum = quorum
	verdict := q.Verdict.Clone()
	verdict.QuorumDecision = quorum.ExactRef()
	verdict.AttestationRefs[0] = att.ExactRef()
	verdict.State, verdict.Conditions, verdict.ConditionsDigest, verdict.Digest = contract.HumanVerdictConditionalV2, conditions, conditionsDigest, ""
	verdict, err = contract.SealHumanVerdictV2(verdict)
	if err != nil {
		t.Fatal(err)
	}
	q.Verdict = verdict
	q.CurrentCase = nextCaseV5(t, q.DecisionCase, contract.CaseResolvedV1, f.now.Add(5*time.Second), &verdict)
	proofEvidence := runtimeports.ReviewEvidenceRefV2{Ref: "condition-evidence-v5", Classification: "review.test/condition", Digest: digest("condition-evidence-v5")}
	proof := runtimeports.ReviewConditionProofV2{ConditionID: condition.ID, ConditionRevision: condition.Revision, ConstraintDigest: condition.ConstraintDigest, Owner: condition.SatisfactionOwner, ScopeDigest: condition.ScopeDigest, Authority: condition.Authority, Evidence: proofEvidence, ExpiresUnixNano: f.snapshot.ExpiresUnixNano}
	proofsDigest, err := runtimeports.DigestConditionProofsV2([]runtimeports.ReviewConditionProofV2{proof})
	if err != nil {
		t.Fatal(err)
	}
	satisfaction := runtimeports.ConditionSatisfactionFactV2{ID: "satisfaction-v5", VerdictID: verdict.ID, VerdictRevision: verdict.Revision, VerdictDigest: verdict.Digest, CandidateDigest: f.snapshot.Target.Digest, IntentID: f.request.Intent.ID, IntentRevision: f.request.Intent.Revision, SubjectDigest: f.snapshot.Target.SubjectDigest, ConditionsDigest: conditionsDigest, Policy: f.snapshot.Target.Policy, Scope: f.snapshot.Target.Scope, RunID: f.snapshot.Target.RunID, ActionScopeDigest: f.snapshot.Target.ActionScopeDigest, CurrentScope: f.snapshot.Target.CurrentScope, Proofs: []runtimeports.ReviewConditionProofV2{proof}, ProofsDigest: proofsDigest, State: runtimeports.ConditionSatisfied, Revision: 1, SatisfiedUnixNano: f.now.Add(5 * time.Second).UnixNano(), UpdatedUnixNano: f.now.Add(5 * time.Second).UnixNano(), ExpiresUnixNano: f.snapshot.ExpiresUnixNano}
	if err = satisfaction.Validate(); err != nil {
		t.Fatal(err)
	}
	q.Satisfaction = &satisfaction
	q.SatisfactionEvidence = []runtimeadapter.EvidenceCurrentReceiptV5{evidenceReceiptV5(t, f.snapshot.Target, proofEvidence, 3, f.now, f.snapshot.ExpiresUnixNano)}
	f.snapshot.Basis = runtimeports.OperationReviewBasisConditionalQuorumSatisfiedV5
	f.snapshot.Quorum = q
	f.snapshot.Digest = ""
	f.snapshot, err = runtimeadapter.SealCurrentFactSnapshotV5(f.snapshot)
	if err != nil {
		t.Fatal(err)
	}
	f.request.Basis = runtimeports.OperationReviewBasisConditionalQuorumSatisfiedV5
	return f
}

type atomicSourceV5 struct {
	mu          sync.RWMutex
	snapshot    runtimeadapter.CurrentFactSnapshotV5
	requests    []runtimeadapter.ExactCurrentRequestV5
	calls       int
	lose        int
	always      bool
	cancel      context.CancelFunc
	recoveryErr error
}

func (s *atomicSourceV5) InspectReviewCurrentFactsV5(ctx context.Context, request runtimeadapter.ExactCurrentRequestV5) (runtimeadapter.CurrentFactSnapshotV5, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.requests = append(s.requests, request)
	if s.calls == 1 && s.cancel != nil {
		s.cancel()
	}
	if s.calls > 1 {
		s.recoveryErr = ctx.Err()
	}
	if s.always || s.lose > 0 {
		if s.lose > 0 {
			s.lose--
		}
		return runtimeadapter.CurrentFactSnapshotV5{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost read reply")
	}
	return s.snapshot, nil
}
func (s *atomicSourceV5) count() int { s.mu.RLock(); defer s.mu.RUnlock(); return s.calls }
func (s *atomicSourceV5) exactRequests() []runtimeadapter.ExactCurrentRequestV5 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]runtimeadapter.ExactCurrentRequestV5(nil), s.requests...)
}
