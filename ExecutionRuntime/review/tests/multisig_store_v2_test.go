package review_test

import (
	"context"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/multisigowner"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	reviewsqlite "github.com/Proview-China/rax/ExecutionRuntime/review/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type multiStore interface {
	reviewport.StoreV1
	reviewport.StoreV2
	reviewport.CaseTransitionStoreV2
}

func hd(s string) core.Digest { return testkit.Digest("human-v2-" + s) }
func hcase(c contract.ReviewCaseV1) contract.HumanCaseExactRefV2 {
	return contract.HumanCaseExactRefV2{TenantID: c.TenantID, ID: c.ID, Revision: c.Revision, Digest: c.Digest}
}
func htarget(t contract.TargetSnapshotV1) contract.HumanTargetExactRefV2 {
	return contract.HumanTargetExactRefV2{TenantID: t.TenantID, ID: t.ID, Revision: t.Revision, Digest: t.Digest}
}
func hround(r contract.ReviewRoundV1) contract.HumanRoundExactRefV2 {
	return contract.HumanRoundExactRefV2{TenantID: r.TenantID, ID: r.ID, Revision: r.Revision, Digest: r.Digest}
}
func hid(t core.TenantID, id string) contract.HumanIdentityProofRefV2 {
	return contract.HumanIdentityProofRefV2{TenantID: t, Ref: id, Revision: 1, Digest: hd(id)}
}
func hauth(id string) runtimeports.AuthorityBindingRefV2 {
	return runtimeports.AuthorityBindingRefV2{Ref: id, Revision: 1, Digest: hd(id), Epoch: 1}
}
func hbind(id string) runtimeports.ReviewComponentBindingRefV2 {
	return runtimeports.ReviewComponentBindingRefV2{BindingSetID: "set-" + id, BindingSetRevision: 1, ComponentID: "review/human", ManifestDigest: hd("manifest-" + id), ArtifactDigest: hd("artifact-" + id), Capability: "review/attest"}
}

type multiFixture struct {
	now                                                                  time.Time
	target                                                               contract.TargetSnapshotV1
	round                                                                contract.ReviewRoundV1
	caseWaiting, caseReviewing, caseAttested, caseDeciding, caseResolved contract.ReviewCaseV1
	create                                                               reviewport.CreateHumanPanelMutationV2
	vote1, vote2                                                         reviewport.RecordHumanAttestationMutationV2
	begin                                                                reviewport.BeginHumanPanelDecisionMutationV2
	decide                                                               reviewport.DecideHumanPanelMutationV2
}

func sealCaseState(t *testing.T, current contract.ReviewCaseV1, state contract.CaseStateV1, at time.Time, verdict *contract.HumanVerdictV2) contract.ReviewCaseV1 {
	t.Helper()
	v := current
	v.Revision++
	v.State = state
	v.UpdatedUnixNano = at.UnixNano()
	v.Digest = ""
	if verdict != nil {
		v.VerdictID = verdict.ID
		v.VerdictRevision = verdict.Revision
		v.VerdictDigest = verdict.Digest
	}
	sealed, err := contract.SealReviewCaseV1(v)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}
func nextPanel(t *testing.T, p contract.HumanReviewPanelV2, state contract.HumanPanelStateV2, at time.Time) contract.HumanReviewPanelV2 {
	t.Helper()
	v := p.Clone()
	v.Revision++
	v.State = state
	v.UpdatedUnixNano = at.UnixNano()
	v.Digest = ""
	sealed, err := contract.SealHumanReviewPanelV2(v)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func humanMutationTrace(t *testing.T, at time.Time, c contract.ReviewCaseV1, target contract.TargetSnapshotV1, event contract.TraceEventV1, id, source string, sequence uint64, causation, correlation string, refs ...string) contract.TraceFactV1 {
	t.Helper()
	refs = append([]string(nil), refs...)
	sort.Strings(refs)
	trace, err := contract.SealTraceFactV1(contract.TraceFactV1{
		FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: c.TenantID, ID: id, Revision: 1, CreatedUnixNano: at.UnixNano(), UpdatedUnixNano: at.UnixNano()},
		CaseID:         c.ID, CaseRevision: c.Revision, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest,
		Event: event, SourceID: source, SourceEpoch: 1, SourceSequence: sequence, CausationID: causation, CorrelationID: correlation, FactRefs: refs,
	})
	if err != nil {
		t.Fatal(err)
	}
	return trace
}

func prepareFixture(t *testing.T, store multiStore) multiFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	target := testkit.Target(now)
	rubric := testkit.PublishRubric(ctx, store, now, target.TenantID)
	rubricRef := rubric.ExactRef()
	c, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "case-human-v2", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, Rubric: &rubricRef, State: contract.CaseRequestedV1, ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateTargetCaseV1(ctx, reviewport.CreateTargetCaseMutationV1{Target: target, Case: c}); err != nil {
		t.Fatal(err)
	}
	admitted := sealCaseState(t, c, contract.CaseAdmittedV1, now.Add(time.Nanosecond), nil)
	if _, err = store.TransitionCaseWithTraceV2(ctx, reviewport.TransitionCaseWithTraceMutationV2{Expected: reviewport.ExpectedV1(c.Revision, c.Digest), Next: admitted, Trace: testkit.TransitionTrace(now.Add(time.Nanosecond), c, contract.CaseAdmittedV1)}); err != nil {
		t.Fatal(err)
	}
	routed := sealCaseState(t, admitted, contract.CaseRoutedV1, now.Add(2*time.Nanosecond), nil)
	if _, err = store.TransitionCaseWithTraceV2(ctx, reviewport.TransitionCaseWithTraceMutationV2{Expected: reviewport.ExpectedV1(admitted.Revision, admitted.Digest), Next: routed, Trace: testkit.TransitionTrace(now.Add(2*time.Nanosecond), admitted, contract.CaseRoutedV1)}); err != nil {
		t.Fatal(err)
	}
	round := testkit.Round(now.Add(3*time.Nanosecond), routed, contract.RouteHumanV1)
	a1 := testkit.Assignment(now.Add(3*time.Nanosecond), routed, round, contract.RouteHumanV1)
	trace := testkit.Trace(now.Add(3*time.Nanosecond), routed, contract.TraceAssignedV1, 1, round.ID, a1.ID)
	waiting, _, _, err := store.StartRoundV1(ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(routed.Revision, routed.Digest), Round: round, Assignment: a1, Trace: trace, RubricCheckedUnixNano: now.Add(3 * time.Nanosecond).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	policy := contract.HumanQuorumPolicyBindingV2{TenantID: target.TenantID, Ref: "policy-human-v2", Revision: 1, Digest: hd("policy"), Domain: "tenant-review", CheckedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	resp := contract.HumanResponsibilitySubjectRefV2{TenantID: target.TenantID, Ref: "responsibility-human-v2", Revision: 1, Digest: hd("responsibility"), IdentityProof: hid(target.TenantID, "author")}
	proposed, err := contract.SealHumanReviewPanelV2(contract.HumanReviewPanelV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, Revision: 1, CreatedUnixNano: now.Add(time.Second).UnixNano(), UpdatedUnixNano: now.Add(time.Second).UnixNano()}, Case: hcase(waiting), Target: htarget(target), Round: hround(round), QuorumPolicy: policy, ResponsibilitySubject: resp, State: contract.HumanPanelProposedV2, AcceptThreshold: 2, MaximumPanelSize: 2, RoleRequirements: []contract.HumanRoleRequirementV2{{Role: "security", Minimum: 1}, {Role: "technical", Minimum: 1}}, RejectVetoRoles: []string{"security"}, DelegationRequired: true, ProductionSelfReviewAllowed: false, MaxPanelDurationNanos: int64(30 * time.Minute), MaxVoteTTLNanos: int64(10 * time.Minute), ExpiresUnixNano: now.Add(20 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	makeAssignment := func(id, role string) contract.HumanPanelAssignmentV2 {
		reviewer := hid(target.TenantID, id)
		v, err := contract.SealHumanPanelAssignmentV2(contract.HumanPanelAssignmentV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "assignment-" + id, Revision: 1, CreatedUnixNano: now.Add(time.Second).UnixNano(), UpdatedUnixNano: now.Add(time.Second).UnixNano()}, Panel: proposed.ExactRef(), Case: proposed.Case, Round: proposed.Round, Target: proposed.Target, ReviewerIdentity: reviewer, ReviewerAuthority: hauth("authority-" + id), ReviewerBinding: hbind(id), Roles: []string{role}, CanVeto: role == "security", Delegated: true, DelegatorIdentity: hid(target.TenantID, "manager"), DelegateIdentity: reviewer, DelegationFact: contract.HumanDelegationFactRefV2{TenantID: target.TenantID, Ref: "delegation-" + id, Revision: 1, Digest: hd("delegation-" + id)}, DelegatedRole: role, DelegationScopeDigest: hd("scope"), State: contract.HumanAssignmentOfferedV2, ExpiresUnixNano: now.Add(20 * time.Minute).UnixNano()})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	a := []contract.HumanPanelAssignmentV2{makeAssignment("reviewer-a", "security"), makeAssignment("reviewer-b", "technical")}
	open := proposed.Clone()
	open.Revision = 2
	open.State = contract.HumanPanelOpenV2
	open.UpdatedUnixNano = now.Add(time.Second + time.Nanosecond).UnixNano()
	open.AssignmentRefs = []contract.HumanPanelAssignmentExactRefV2{a[0].ExactRef(), a[1].ExactRef()}
	open.Digest = ""
	open, err = contract.SealHumanReviewPanelV2(open)
	if err != nil {
		t.Fatal(err)
	}
	makeAtt := func(panel contract.HumanReviewPanelV2, as contract.HumanPanelAssignmentV2, id string, at time.Time) contract.HumanAttestationV2 {
		ev := []runtimeports.ReviewEvidenceRefV2{{Ref: "evidence-" + id, Classification: "review/human", Digest: hd("evidence-" + id)}}
		ed, _ := contract.ComputeReviewEvidenceDigestV1(ev)
		v, err := contract.SealHumanAttestationV2(contract.HumanAttestationV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "attestation-" + id, Revision: 1, CreatedUnixNano: at.UnixNano(), UpdatedUnixNano: at.UnixNano()}, IdempotencyKey: "idem-" + id, Panel: panel.ExactRef(), Assignment: as.ExactRef(), Case: panel.Case, Round: panel.Round, Target: panel.Target, Policy: panel.QuorumPolicy, ResponsibilitySubject: panel.ResponsibilitySubject, ReviewerIdentity: as.ReviewerIdentity, ReviewerAuthority: as.ReviewerAuthority, Delegation: &as.DelegationFact, ReviewerBinding: as.ReviewerBinding, Resolution: contract.ResolutionAcceptV1, ReasonCodes: []string{"review/verified"}, Evidence: ev, EvidenceDigest: ed, ObservedUnixNano: at.UnixNano(), ExpiresUnixNano: at.Add(10 * time.Minute).UnixNano()})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	reviewing := sealCaseState(t, waiting, contract.CaseReviewingV1, now.Add(2*time.Second), nil)
	att1 := makeAtt(open, a[0], "reviewer-a", now.Add(2*time.Second))
	p3 := nextPanel(t, open, contract.HumanPanelOpenV2, now.Add(2*time.Second))
	vote1Trace := humanMutationTrace(t, now.Add(2*time.Second), waiting, target, contract.TraceAttestedV1, "trace-human-attested-a", "review.test/human-vote", 1, att1.ID, p3.ID, att1.ID)
	vote1 := reviewport.RecordHumanAttestationMutationV2{ExpectedPanel: open.ExactRef(), Attestation: att1, NextPanel: p3, ExpectedCase: ptrCase(hcase(waiting)), NextCase: &reviewing, Trace: vote1Trace}
	att2 := makeAtt(p3, a[1], "reviewer-b", now.Add(3*time.Second))
	p4 := nextPanel(t, p3, contract.HumanPanelQuorumSatisfiedV2, now.Add(3*time.Second))
	allEvidence := append(append([]runtimeports.ReviewEvidenceRefV2{}, att1.Evidence...), att2.Evidence...)
	evidenceDigest, _ := contract.ComputeReviewEvidenceDigestV1(allEvidence)
	ids := []contract.HumanIdentityProofRefV2{a[0].ReviewerIdentity, a[1].ReviewerIdentity}
	reviewerDigest, _ := contract.ComputeHumanReviewerSetDigestV2(ids)
	q, err := contract.SealHumanQuorumDecisionV2(contract.HumanQuorumDecisionV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "quorum-human-v2", Revision: 1, CreatedUnixNano: now.Add(3 * time.Second).UnixNano(), UpdatedUnixNano: now.Add(3 * time.Second).UnixNano()}, Panel: p4.ExactRef(), Policy: policy, AcceptedAttestationRefs: []contract.HumanAttestationExactRefV2{att1.ExactRef(), att2.ExactRef()}, DistinctReviewerIdentityRefs: ids, SatisfiedRoleCounts: []contract.HumanSatisfiedRoleCountV2{{Role: "security", DistinctCurrentCount: 1}, {Role: "technical", DistinctCurrentCount: 1}}, AcceptCount: 2, Threshold: 2, Resolution: contract.ResolutionAcceptV1, EvidenceSetDigest: evidenceDigest, ReviewerSetDigest: reviewerDigest, CheckedUnixNano: now.Add(3 * time.Second).UnixNano(), ExpiresUnixNano: now.Add(10*time.Minute + 2*time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	attested := sealCaseState(t, reviewing, contract.CaseAttestedV1, now.Add(3*time.Second), nil)
	vote2Trace := humanMutationTrace(t, now.Add(3*time.Second), waiting, target, contract.TraceAttestedV1, "trace-human-attested-b", "review.test/human-vote", 2, att2.ID, p4.ID, att2.ID, q.ID)
	vote2 := reviewport.RecordHumanAttestationMutationV2{ExpectedPanel: p3.ExactRef(), Attestation: att2, NextPanel: p4, Quorum: &q, ExpectedCase: ptrCase(hcase(reviewing)), NextCase: &attested, Trace: vote2Trace}
	p5 := nextPanel(t, p4, contract.HumanPanelDecidingV2, now.Add(4*time.Second))
	deciding := sealCaseState(t, attested, contract.CaseDecidingV1, now.Add(4*time.Second), nil)
	beginTrace := humanMutationTrace(t, now.Add(4*time.Second), deciding, target, contract.TraceStartedV1, "trace-human-decision-started", reviewport.HumanDecisionTraceSourceV2, uint64(p5.Revision), p5.ID, deciding.ID, p5.ID, deciding.ID, target.ID, q.ID)
	begin := reviewport.BeginHumanPanelDecisionMutationV2{ExpectedPanel: p4.ExactRef(), NextPanel: p5, ExpectedCase: hcase(attested), NextCase: deciding, Trace: beginTrace}
	v, err := contract.SealHumanVerdictV2(contract.HumanVerdictV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "verdict-human-v2", Revision: 1, CreatedUnixNano: now.Add(5 * time.Second).UnixNano(), UpdatedUnixNano: now.Add(5 * time.Second).UnixNano()}, Case: hcase(deciding), Target: htarget(target), Round: hround(round), Panel: p5.ExactRef(), QuorumDecision: q.ExactRef(), Policy: policy, Scope: target.Scope, CurrentScope: target.CurrentScope, ReviewerSetDigest: reviewerDigest, ReviewerAuthorityRefs: []runtimeports.AuthorityBindingRefV2{a[0].ReviewerAuthority, a[1].ReviewerAuthority}, BindingClosures: []runtimeports.ReviewComponentBindingRefV2{a[0].ReviewerBinding, a[1].ReviewerBinding}, AttestationRefs: []contract.HumanAttestationExactRefV2{att1.ExactRef(), att2.ExactRef()}, Evidence: allEvidence, EvidenceSetDigest: evidenceDigest, ReasonCodes: []string{"review/quorum-satisfied"}, State: contract.HumanVerdictAcceptedV2, ExpiresUnixNano: now.Add(10*time.Minute + 2*time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	p6 := nextPanel(t, p5, contract.HumanPanelDecidedV2, now.Add(5*time.Second))
	resolved := sealCaseState(t, deciding, contract.CaseResolvedV1, now.Add(5*time.Second), &v)
	verdictTrace := humanMutationTrace(t, now.Add(5*time.Second), deciding, target, contract.TraceVerdictV1, "trace-human-verdict", "review.test/human-decision", 1, v.ID, p6.ID, v.ID, q.ID)
	resolvedTrace := humanMutationTrace(t, now.Add(5*time.Second), resolved, target, contract.TraceResolvedV1, "trace-human-resolved", "review.test/human-resolved", 1, v.ID, p6.ID, v.ID)
	decide := reviewport.DecideHumanPanelMutationV2{ExpectedPanel: p5.ExactRef(), ExpectedCase: hcase(deciding), Quorum: q.ExactRef(), Verdict: v, NextPanel: p6, NextCase: resolved, Trace: verdictTrace, AdditionalTraces: []contract.TraceFactV1{resolvedTrace}}
	assignedRefs := []string{open.ID, a[0].ID, a[1].ID}
	assignedTrace := humanMutationTrace(t, now.Add(time.Second+time.Nanosecond), waiting, target, contract.TraceAssignedV1, "trace-human-assigned", "review.test/human-panel", 1, open.ID, waiting.ID, assignedRefs...)
	return multiFixture{now: now, target: target, round: round, caseWaiting: waiting, caseReviewing: reviewing, caseAttested: attested, caseDeciding: deciding, caseResolved: resolved, create: reviewport.CreateHumanPanelMutationV2{ExpectedCase: hcase(waiting), ProposedPanel: proposed, Assignments: a, OpenPanel: open, Trace: assignedTrace}, vote1: vote1, vote2: vote2, begin: begin, decide: decide}
}
func ptrCase(v contract.HumanCaseExactRefV2) *contract.HumanCaseExactRefV2 { return &v }

func conditionV2Fixture(t *testing.T, f multiFixture) multiFixture {
	t.Helper()
	condition := runtimeports.ReviewConditionV2{ID: "review/human-followup", Revision: 1, Schema: testkit.Schema("human-condition"), ConstraintDigest: hd("human-condition"), SatisfactionOwner: f.vote2.Attestation.ReviewerBinding, ScopeDigest: f.target.ActionScopeDigest, Authority: f.vote2.Attestation.ReviewerAuthority, ExpiresUnixNano: f.vote2.Attestation.ExpiresUnixNano}
	conditions := []runtimeports.ReviewConditionV2{condition}
	digest, err := runtimeports.DigestReviewConditionsV2(conditions)
	if err != nil {
		t.Fatal(err)
	}
	attestation := f.vote2.Attestation.Clone()
	attestation.Resolution, attestation.Conditions, attestation.ConditionsDigest, attestation.Digest = contract.ResolutionConditionalV1, conditions, digest, ""
	attestation, err = contract.SealHumanAttestationV2(attestation)
	if err != nil {
		t.Fatal(err)
	}
	quorum := f.vote2.Quorum.Clone()
	quorum.AcceptedAttestationRefs[1] = attestation.ExactRef()
	quorum.Resolution, quorum.Conditions, quorum.ConditionsDigest, quorum.Digest = contract.ResolutionConditionalV1, conditions, digest, ""
	quorum, err = contract.SealHumanQuorumDecisionV2(quorum)
	if err != nil {
		t.Fatal(err)
	}
	f.vote2.Attestation, f.vote2.Quorum = attestation, &quorum
	verdict := f.decide.Verdict.Clone()
	verdict.QuorumDecision = quorum.ExactRef()
	verdict.AttestationRefs[1] = attestation.ExactRef()
	verdict.State, verdict.Conditions, verdict.ConditionsDigest, verdict.Digest = contract.HumanVerdictConditionalV2, conditions, digest, ""
	verdict, err = contract.SealHumanVerdictV2(verdict)
	if err != nil {
		t.Fatal(err)
	}
	f.caseResolved = sealCaseState(t, f.caseDeciding, contract.CaseResolvedV1, time.Unix(0, verdict.UpdatedUnixNano), &verdict)
	f.decide.Quorum, f.decide.Verdict, f.decide.NextCase = quorum.ExactRef(), verdict, f.caseResolved
	return f
}

func TestConditionV2HumanStoreConformanceMemoryAndSQLite(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			var store multiStore
			var closeFn func()
			if backend == "memory" {
				store = storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
				closeFn = func() {}
			} else {
				sqlStore, err := reviewsqlite.Open(context.Background(), reviewsqlite.Config{Path: filepath.Join(t.TempDir(), "condition-v2.sqlite"), Clock: func() time.Time { return time.Unix(1_900_000_010, 0) }})
				if err != nil {
					t.Fatal(err)
				}
				store = sqlStore
				closeFn = func() { _ = sqlStore.Close() }
			}
			defer closeFn()
			fixture := conditionV2Fixture(t, prepareFixture(t, store))
			if err := conformance.CheckHumanMultiSignStoreV2(context.Background(), store, conformance.HumanMultiSignStoreFixtureV2{Create: fixture.create, Votes: []reviewport.RecordHumanAttestationMutationV2{fixture.vote1, fixture.vote2}, Begin: &fixture.begin, Decide: &fixture.decide}); err != nil {
				t.Fatal(err)
			}
			stored, err := store.InspectHumanVerdictExactV2(context.Background(), fixture.decide.Verdict.ExactRef())
			if err != nil || len(stored.Conditions) != 1 || stored.Conditions[0] != fixture.decide.Verdict.Conditions[0] {
				t.Fatalf("stored Human Verdict lost exact conditions: %+v err=%v", stored, err)
			}
		})
	}
}

type acceptingCut struct{}

func cutProof(tenant core.TenantID, subject core.Digest, now time.Time) (multisigowner.ExternalCurrentProofV2, error) {
	return multisigowner.SealExternalCurrentProofV2(multisigowner.ExternalCurrentProofV2{TenantID: tenant, SubjectDigest: subject, CheckedUnixNano: now.Add(-time.Nanosecond).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
}
func (acceptingCut) ValidatePanelCurrentV2(_ context.Context, p contract.HumanReviewPanelV2, a []contract.HumanPanelAssignmentV2, open contract.HumanReviewPanelV2, now time.Time) (multisigowner.ExternalCurrentProofV2, error) {
	d, err := multisigowner.PanelCurrentSubjectDigestV2(p, a, open)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	return cutProof(p.TenantID, d, now)
}
func (acceptingCut) ValidateAttestationCurrentV2(_ context.Context, p contract.HumanReviewPanelV2, a contract.HumanPanelAssignmentV2, v contract.HumanAttestationV2, now time.Time) (multisigowner.ExternalCurrentProofV2, error) {
	d, err := multisigowner.AttestationCurrentSubjectDigestV2(p, a, v)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	return cutProof(p.TenantID, d, now)
}
func (acceptingCut) ValidateDecisionCurrentV2(_ context.Context, p contract.HumanReviewPanelV2, q contract.HumanQuorumDecisionV2, v contract.HumanVerdictV2, now time.Time) (multisigowner.ExternalCurrentProofV2, error) {
	d, err := multisigowner.DecisionCurrentSubjectDigestV2(p, q, v)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	return cutProof(p.TenantID, d, now)
}

func runMultiSignStoreFlow(t *testing.T, store multiStore, reopen func() multiStore) {
	t.Helper()
	ctx := context.Background()
	f := prepareFixture(t, store)
	owner, err := multisigowner.New(store, acceptingCut{}, func() time.Time { return f.now.Add(6 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.OpenPanelV2(ctx, f.create); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := owner.SubmitAttestationV2(ctx, f.vote1); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	p, err := store.InspectHumanPanelCurrentV2(ctx, f.create.OpenPanel.TenantID, f.create.OpenPanel.ID)
	if err != nil || p.Revision != f.vote1.NextPanel.Revision {
		t.Fatalf("concurrent vote current drift: %v rev=%d", err, p.Revision)
	}
	if _, err = owner.SubmitAttestationV2(ctx, f.vote2); err != nil {
		t.Fatal(err)
	}
	if _, _, err = owner.BeginDecisionV2(ctx, f.begin); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.DecideV2(ctx, f.decide); err != nil {
		t.Fatal(err)
	}
	if reopen != nil {
		store = reopen()
	}
	if _, err = store.InspectHumanPanelExactV2(ctx, f.create.ProposedPanel.ExactRef()); err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectHumanAttestationExactV2(ctx, f.vote1.Attestation.ExactRef()); err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectHumanVerdictExactV2(ctx, f.decide.Verdict.ExactRef()); err != nil {
		t.Fatal(err)
	}
	current, err := store.InspectHumanPanelCurrentV2(ctx, f.decide.NextPanel.TenantID, f.decide.NextPanel.ID)
	if err != nil || current.Digest != f.decide.NextPanel.Digest {
		t.Fatalf("terminal Panel not restored: %v", err)
	}
}

func TestHumanMultiSignV2MemoryOwnerConcurrentFlow(t *testing.T) {
	runMultiSignStoreFlow(t, storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) }), nil)
}
func TestHumanMultiSignV2SQLiteRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.sqlite")
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0)
	store, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: func() time.Time { return now.Add(time.Hour) }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runMultiSignStoreFlow(t, store, func() multiStore {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		next, e := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: func() time.Time { return now.Add(time.Hour) }})
		if e != nil {
			t.Fatal(e)
		}
		store = next
		return next
	})
}

func TestHumanMultiSignV2CreateStagedFailureZeroWrite(t *testing.T) {
	s := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
	f := prepareFixture(t, s)
	bad := f.create
	bad.Assignments = append([]contract.HumanPanelAssignmentV2(nil), bad.Assignments...)
	bad.Assignments[1] = bad.Assignments[0]
	if _, err := s.CreateHumanPanelV2(context.Background(), bad); err == nil {
		t.Fatal("duplicate staged Assignment accepted")
	}
	if _, err := s.InspectHumanPanelCurrentV2(context.Background(), f.create.OpenPanel.TenantID, f.create.OpenPanel.ID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged failure leaked Panel: %v", err)
	}
}

func TestHumanMultiSignV2DuplicateIdentityAndTTLDriftZeroWrite(t *testing.T) {
	t.Run("duplicate identity across assignments", func(t *testing.T) {
		s := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
		f := prepareFixture(t, s)
		bad := f.create
		bad.Assignments = append([]contract.HumanPanelAssignmentV2(nil), bad.Assignments...)
		a := bad.Assignments[1].Clone()
		a.Digest = ""
		a.ReviewerIdentity = bad.Assignments[0].ReviewerIdentity
		a.DelegateIdentity = a.ReviewerIdentity
		a, err := contract.SealHumanPanelAssignmentV2(a)
		if err != nil {
			t.Fatal(err)
		}
		bad.Assignments[1] = a
		open := bad.OpenPanel.Clone()
		open.Digest = ""
		open.AssignmentRefs = []contract.HumanPanelAssignmentExactRefV2{bad.Assignments[0].ExactRef(), a.ExactRef()}
		open, err = contract.SealHumanReviewPanelV2(open)
		if err != nil {
			t.Fatal(err)
		}
		bad.OpenPanel = open
		if _, err := s.CreateHumanPanelV2(context.Background(), bad); !core.HasReason(err, core.ReasonDuplicateCanonicalKey) {
			t.Fatalf("duplicate identity accepted: %v", err)
		}
		if _, err := s.InspectHumanPanelCurrentV2(context.Background(), open.TenantID, open.ID); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("duplicate identity leaked Panel: %v", err)
		}
	})
	t.Run("attestation expiry not exact minimum", func(t *testing.T) {
		s := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
		f := prepareFixture(t, s)
		owner, err := multisigowner.New(s, acceptingCut{}, func() time.Time { return f.now.Add(6 * time.Second) })
		if err != nil {
			t.Fatal(err)
		}
		if _, err = owner.OpenPanelV2(context.Background(), f.create); err != nil {
			t.Fatal(err)
		}
		bad := f.vote1
		att := bad.Attestation.Clone()
		att.Digest = ""
		att.ExpiresUnixNano++
		att, err = contract.SealHumanAttestationV2(att)
		if err != nil {
			t.Fatal(err)
		}
		bad.Attestation = att
		if _, err = owner.SubmitAttestationV2(context.Background(), bad); !core.HasReason(err, core.ReasonReviewVerdictStale) {
			t.Fatalf("TTL drift accepted: %v", err)
		}
		current, err := s.InspectHumanPanelCurrentV2(context.Background(), f.create.OpenPanel.TenantID, f.create.OpenPanel.ID)
		if err != nil || current.Digest != f.create.OpenPanel.Digest {
			t.Fatalf("TTL drift advanced Panel: %v", err)
		}
		if _, err = s.InspectHumanAttestationExactV2(context.Background(), att.ExactRef()); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("TTL drift wrote Attestation: %v", err)
		}
	})
}

type lostReplyMultiStore struct {
	multiStore
	create, vote, begin, decide bool
}

func indeterminateReply() error {
	return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected lost reply")
}
func (s *lostReplyMultiStore) CreateHumanPanelV2(ctx context.Context, m reviewport.CreateHumanPanelMutationV2) (reviewport.CreateHumanPanelResultV2, error) {
	v, e := s.multiStore.CreateHumanPanelV2(ctx, m)
	if e == nil && s.create {
		s.create = false
		return reviewport.CreateHumanPanelResultV2{}, indeterminateReply()
	}
	return v, e
}
func (s *lostReplyMultiStore) RecordHumanAttestationV2(ctx context.Context, m reviewport.RecordHumanAttestationMutationV2) (reviewport.RecordHumanAttestationResultV2, error) {
	v, e := s.multiStore.RecordHumanAttestationV2(ctx, m)
	if e == nil && s.vote {
		s.vote = false
		return reviewport.RecordHumanAttestationResultV2{}, indeterminateReply()
	}
	return v, e
}
func (s *lostReplyMultiStore) BeginHumanPanelDecisionV2(ctx context.Context, m reviewport.BeginHumanPanelDecisionMutationV2) (contract.HumanReviewPanelV2, contract.ReviewCaseV1, error) {
	p, c, e := s.multiStore.BeginHumanPanelDecisionV2(ctx, m)
	if e == nil && s.begin {
		s.begin = false
		return contract.HumanReviewPanelV2{}, contract.ReviewCaseV1{}, indeterminateReply()
	}
	return p, c, e
}
func (s *lostReplyMultiStore) DecideHumanPanelV2(ctx context.Context, m reviewport.DecideHumanPanelMutationV2) (reviewport.DecideHumanPanelResultV2, error) {
	v, e := s.multiStore.DecideHumanPanelV2(ctx, m)
	if e == nil && s.decide {
		s.decide = false
		return reviewport.DecideHumanPanelResultV2{}, indeterminateReply()
	}
	return v, e
}

func TestHumanMultiSignV2LostReplyExactInspectRecovery(t *testing.T) {
	base := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
	f := prepareFixture(t, base)
	lost := &lostReplyMultiStore{multiStore: base, create: true, vote: true, begin: true, decide: true}
	owner, err := multisigowner.New(lost, acceptingCut{}, func() time.Time { return f.now.Add(6 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if _, err = owner.OpenPanelV2(ctx, f.create); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.SubmitAttestationV2(ctx, f.vote1); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.SubmitAttestationV2(ctx, f.vote2); err != nil {
		t.Fatal(err)
	}
	if _, _, err = owner.BeginDecisionV2(ctx, f.begin); err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err = owner.DecideV2(ctx, f.decide); err == nil {
		t.Fatal("cancelled original context unexpectedly executed Decide")
	}
	ctx = context.Background()
	lost.decide = true
	if _, err = owner.DecideV2(ctx, f.decide); err != nil {
		t.Fatal(err)
	}
	if _, err = base.InspectHumanVerdictExactV2(ctx, f.decide.Verdict.ExactRef()); err != nil {
		t.Fatal(err)
	}
	for _, event := range append([]contract.TraceFactV1{f.decide.Trace}, f.decide.AdditionalTraces...) {
		if _, err = base.InspectTraceExactV1(ctx, event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest)); err != nil {
			t.Fatalf("lost-reply recovery missed exact %s: %v", event.Event, err)
		}
	}
}

func TestHumanMultiSignV2ReusableConformance(t *testing.T) {
	s := storetestkit.NewMemoryStoreV1(func() time.Time { return time.Unix(1_900_000_010, 0) })
	f := prepareFixture(t, s)
	if err := conformance.CheckHumanMultiSignStoreV2(context.Background(), s, conformance.HumanMultiSignStoreFixtureV2{Create: f.create, Votes: []reviewport.RecordHumanAttestationMutationV2{f.vote1, f.vote2}, Begin: &f.begin, Decide: &f.decide}); err != nil {
		t.Fatal(err)
	}
}
