package decisioncurrent

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	organizationcontract "github.com/Proview-China/rax/ExecutionRuntime/organization-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/multisigcurrent"
	"github.com/Proview-China/rax/ExecutionRuntime/review/multisigowner"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestHumanMultiSignExternalCurrentV2PanelS1S2MinimumTTLAndConcurrency(t *testing.T) {
	fixture := newHumanExternalFixtureV2(t)
	wantSubject, err := multisigowner.PanelCurrentSubjectDigestV2(fixture.proposed, []contract.HumanPanelAssignmentV2{fixture.assignment}, fixture.open)
	if err != nil {
		t.Fatal(err)
	}
	wantExpiry := fixture.base.Add(3 * time.Minute).UnixNano()
	for range 64 {
		proof, readErr := fixture.source.ValidatePanelCurrentV2(context.Background(), fixture.proposed, []contract.HumanPanelAssignmentV2{fixture.assignment.Clone()}, fixture.open, fixture.base)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if proof.SubjectDigest != wantSubject || proof.ExpiresUnixNano != wantExpiry || proof.TenantID != fixture.target.TenantID {
			t.Fatalf("external proof drifted: %+v", proof)
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			proof, readErr := fixture.source.ValidatePanelCurrentV2(context.Background(), fixture.proposed, []contract.HumanPanelAssignmentV2{fixture.assignment.Clone()}, fixture.open, fixture.base)
			if readErr == nil && (proof.SubjectDigest != wantSubject || proof.ExpiresUnixNano != wantExpiry) {
				readErr = core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "concurrent proof drifted")
			}
			errs <- readErr
		}()
	}
	wg.Wait()
	close(errs)
	for readErr := range errs {
		if readErr != nil {
			t.Fatal(readErr)
		}
	}
}

func TestHumanMultiSignExternalCurrentV2AttestationAndDecisionEvidence(t *testing.T) {
	fixture := newHumanExternalFixtureV2(t)
	attestation, quorumPanel, decidingPanel, quorum, verdict := humanDecisionValuesV2(t, fixture)
	request := reviewport.DecisionExternalCurrentRequestV1{Target: fixture.target, Assignment: contract.ReviewerAssignmentV1{FactIdentityV1: contract.FactIdentityV1{TenantID: fixture.assignment.TenantID, ID: fixture.assignment.ID, Revision: fixture.assignment.Revision, Digest: fixture.assignment.Digest}, ReviewerID: fixture.assignment.ReviewerIdentity.Ref}, Evidence: append([]runtimeports.ReviewEvidenceRefV2(nil), attestation.Evidence...)}
	evidence := newExternalEvidenceReaderWithExpiryV1(t, request, attestation.Evidence, fixture.base, fixture.base.Add(150*time.Second))
	fixture.source, _ = multisigcurrent.NewExternalSourceV2(fixture.review, fixture.organization, fixture.coordinates, fixture.policy, fixture.authority, fixture.binding, fixture.scope, evidence, fixture.clock.Now)

	fixture.review.assignments = []contract.HumanPanelAssignmentV2{fixture.assignment.Clone()}
	proof, err := fixture.source.ValidateAttestationCurrentV2(context.Background(), fixture.open, fixture.assignment, attestation, fixture.base)
	if err != nil {
		t.Fatal(err)
	}
	if proof.ExpiresUnixNano != evidence.minimumExpiry {
		t.Fatalf("Attestation Evidence was omitted from min TTL: got=%d want=%d", proof.ExpiresUnixNano, evidence.minimumExpiry)
	}

	fixture.review.assignments = []contract.HumanPanelAssignmentV2{fixture.assignment.Clone()}
	proof, err = fixture.source.ValidateDecisionCurrentV2(context.Background(), decidingPanel, quorum, verdict, fixture.base)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := multisigowner.DecisionCurrentSubjectDigestV2(decidingPanel, quorum, verdict)
	if proof.SubjectDigest != want || proof.ExpiresUnixNano != evidence.minimumExpiry || quorum.Panel != quorumPanel.ExactRef() {
		t.Fatalf("Decision external proof drifted: %+v", proof)
	}
}

func TestHumanMultiSignExternalCurrentV2FailClosedMatrix(t *testing.T) {
	tests := map[string]func(*humanExternalFixtureV2){
		"policy-current-drift":       func(f *humanExternalFixtureV2) { f.policy.driftAt = 2 },
		"authority-current-drift":    func(f *humanExternalFixtureV2) { f.authority.driftAt = 2 },
		"scope-current-drift":        func(f *humanExternalFixtureV2) { f.scope.driftAt = 2 },
		"binding-current-drift":      func(f *humanExternalFixtureV2) { f.binding.driftAt = 2 },
		"organization-current-drift": func(f *humanExternalFixtureV2) { f.organization.driftAt = 2 },
		"policy-ttl-crossing": func(f *humanExternalFixtureV2) {
			f.clock.jumpAt = 20
			f.clock.jumpTo = time.Unix(0, f.policy.value.ExpiresUnixNano)
		},
		"clock-rollback":       func(f *humanExternalFixtureV2) { f.clock.rollbackAt = 20 },
		"coordinate-set-drift": func(f *humanExternalFixtureV2) { f.coordinates.omit = true },
		"coordinate-panel-exact-drift": func(f *humanExternalFixtureV2) {
			f.coordinates.panelExactDrift = true
		},
		"target-drift": func(f *humanExternalFixtureV2) { f.review.target.Revision++ },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newHumanExternalFixtureV2(t)
			mutate(&fixture)
			got, err := fixture.source.ValidatePanelCurrentV2(context.Background(), fixture.proposed, []contract.HumanPanelAssignmentV2{fixture.assignment}, fixture.open, fixture.base)
			if err == nil || !reflect.DeepEqual(got, multisigowner.ExternalCurrentProofV2{}) {
				t.Fatalf("%s reached a proof: value=%+v err=%v", name, got, err)
			}
		})
	}
}

func TestHumanMultiSignExternalCurrentV2LostExactReplyUsesOriginalCoordinate(t *testing.T) {
	fixture := newHumanExternalFixtureV2(t)
	ctx, cancel := context.WithCancel(context.Background())
	fixture.policy.loseInspect = true
	fixture.policy.cancel = cancel
	proof, err := fixture.source.ValidatePanelCurrentV2(ctx, fixture.proposed, []contract.HumanPanelAssignmentV2{fixture.assignment}, fixture.open, fixture.base)
	if err != nil {
		t.Fatal(err)
	}
	if proof.Digest == "" || fixture.policy.inspectCalls != 3 || fixture.policy.resolveCalls != 1 {
		t.Fatalf("lost exact read did not stay on the original ref: proof=%+v resolve=%d inspect=%d", proof, fixture.policy.resolveCalls, fixture.policy.inspectCalls)
	}
}

func TestHumanMultiSignExternalCurrentV2CoordinateLostReplyAndDeepClone(t *testing.T) {
	fixture := newHumanExternalFixtureV2(t)
	ctx, cancel := context.WithCancel(context.Background())
	fixture.coordinates.lose = true
	fixture.coordinates.cancel = cancel
	fixture.coordinates.mutateInput = true
	proof, err := fixture.source.ValidatePanelCurrentV2(ctx, fixture.proposed, []contract.HumanPanelAssignmentV2{fixture.assignment}, fixture.open, fixture.base)
	if err != nil {
		t.Fatal(err)
	}
	if proof.Digest == "" || fixture.coordinates.calls != 2 {
		t.Fatalf("exact coordinate read was not recovered once: proof=%+v calls=%d", proof, fixture.coordinates.calls)
	}
	if fixture.assignment.Roles[0] != "technical" || fixture.proposed.RoleRequirements[0].Role != "technical" {
		t.Fatal("coordinate resolver mutated caller-owned Review facts")
	}
}

func TestHumanMultiSignExternalCurrentV2RejectsRollbackAtEveryOwnerBoundary(t *testing.T) {
	baseline := newHumanExternalFixtureV2(t)
	if _, err := baseline.source.ValidatePanelCurrentV2(context.Background(), baseline.proposed, []contract.HumanPanelAssignmentV2{baseline.assignment}, baseline.open, baseline.base); err != nil {
		t.Fatal(err)
	}
	baseline.clock.mu.Lock()
	totalCalls := baseline.clock.calls
	baseline.clock.mu.Unlock()
	if totalCalls < 10 {
		t.Fatalf("fixture did not exercise a multi-Owner cut: calls=%d", totalCalls)
	}
	for rollbackAt := 1; rollbackAt <= totalCalls; rollbackAt++ {
		t.Run(fmt.Sprintf("clock-%02d", rollbackAt), func(t *testing.T) {
			fixture := newHumanExternalFixtureV2(t)
			fixture.clock.rollbackAt = rollbackAt
			proof, err := fixture.source.ValidatePanelCurrentV2(context.Background(), fixture.proposed, []contract.HumanPanelAssignmentV2{fixture.assignment}, fixture.open, fixture.base)
			if err == nil || proof.Digest != "" {
				t.Fatalf("cross-Owner rollback at clock call %d reached a proof: %+v err=%v", rollbackAt, proof, err)
			}
		})
	}
}

func TestHumanMultiSignExternalCurrentV2RejectsTypedNilDependencies(t *testing.T) {
	fixture := newHumanExternalFixtureV2(t)
	var policy *humanPolicyReaderV2
	if _, err := multisigcurrent.NewExternalSourceV2(fixture.review, fixture.organization, fixture.coordinates, policy, fixture.authority, fixture.binding, fixture.scope, noEvidenceReaderV1{}, fixture.clock.Now); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Policy reader was accepted: %v", err)
	}
}

type humanExternalFixtureV2 struct {
	base         time.Time
	clock        *humanExternalClockV2
	target       contract.TargetSnapshotV1
	proposed     contract.HumanReviewPanelV2
	open         contract.HumanReviewPanelV2
	assignment   contract.HumanPanelAssignmentV2
	review       *humanReviewReaderV2
	coordinates  *humanCoordinateReaderV2
	organization *humanOrganizationReaderV2
	policy       *humanPolicyReaderV2
	authority    *humanAuthorityReaderV2
	binding      *humanBindingReaderV2
	scope        *humanScopeReaderV2
	source       *multisigcurrent.ExternalSourceV2
}

func newHumanExternalFixtureV2(t *testing.T) humanExternalFixtureV2 {
	t.Helper()
	base := time.Unix(2_300_000_000, 0)
	clock := &humanExternalClockV2{value: base.Add(time.Second)}
	target := testkit.Target(base)

	scopeFact := runtimeports.ExecutionScopeCurrentFactV2{Ref: "scope-human-v2", Revision: 1, Scope: target.Scope, CapabilityGrantDigest: externalDigestV1("scope-grant-human"), ActivationSource: externalGovernanceSourceV1("activation-human"), InstanceSource: externalGovernanceSourceV1("instance-human"), AuthoritySource: externalGovernanceSourceV1("authority-human"), BindingSource: externalGovernanceSourceV1("binding-human"), RunSource: externalGovernanceSourceV1("run-human"), ActiveRunID: target.RunID, RunState: "running", ProjectionWatermark: 1, State: runtimeports.ExecutionScopeFactActive, ExpiresUnixNano: base.Add(9 * time.Minute).UnixNano()}
	scopeFact.Digest, _ = scopeFact.DigestV2()
	target.CurrentScope, _ = scopeFact.BindingRefV2()
	actorBinding, actorFact := externalAuthorityV1(t, "actor-human-v2", target.Scope, target.ActionScopeDigest, base.Add(8*time.Minute))
	target.ActorAuthority = actorBinding
	target.ExpiresUnixNano = base.Add(10 * time.Minute).UnixNano()
	target.Digest = ""
	var err error
	target, err = contract.SealTargetSnapshotV1(target)
	if err != nil {
		t.Fatal(err)
	}

	policySubject := runtimeports.HumanQuorumPolicyCurrentSubjectV2{TenantID: target.TenantID, Domain: "review/human"}
	policyValue, err := runtimeports.SealHumanQuorumPolicyCurrentProjectionV2(runtimeports.HumanQuorumPolicyCurrentProjectionV2{Ref: runtimeports.HumanQuorumPolicyCurrentProjectionRefV2{Revision: 1}, Subject: policySubject, State: runtimeports.HumanQuorumPolicyProjectionActiveV2, Current: true, AcceptThreshold: 1, MaximumPanelSize: 1, RoleRequirements: []runtimeports.HumanQuorumRoleRequirementV2{{Role: "technical", Minimum: 1}}, RejectVetoRoles: []string{"security"}, DelegationRequired: true, ProductionSelfReviewAllowed: false, MaxPanelDurationNanos: int64(10 * time.Minute), MaxVoteTTLNanos: int64(5 * time.Minute), CheckedUnixNano: base.UnixNano(), ExpiresUnixNano: base.Add(6 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	policyBinding := contract.HumanQuorumPolicyBindingV2{TenantID: target.TenantID, Ref: policyValue.Ref.ID, Revision: policyValue.Ref.Revision, Digest: policyValue.Ref.Digest, Domain: policySubject.Domain, CheckedUnixNano: policyValue.CheckedUnixNano, ExpiresUnixNano: policyValue.ExpiresUnixNano}
	reviewerSubject, managerSubject, authorSubject := "reviewer-human-v2", "manager-human-v2", "author-human-v2"
	reviewerID, _ := organizationcontract.DeriveIdentityIDV1(target.TenantID, reviewerSubject)
	managerID, _ := organizationcontract.DeriveIdentityIDV1(target.TenantID, managerSubject)
	authorID, _ := organizationcontract.DeriveIdentityIDV1(target.TenantID, authorSubject)
	responsibilityID, _ := organizationcontract.DeriveResponsibilityIDV1(target.TenantID, "review-target", target.ID)
	responsibility := contract.HumanResponsibilitySubjectRefV2{TenantID: target.TenantID, Ref: responsibilityID, Revision: 1, Digest: externalDigestV1("responsibility-human"), IdentityProof: contract.HumanIdentityProofRefV2{TenantID: target.TenantID, Ref: authorID, Revision: 1, Digest: externalDigestV1("author-human")}}
	proposed, err := contract.SealHumanReviewPanelV2(contract.HumanReviewPanelV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, Revision: 1, CreatedUnixNano: base.Add(time.Second).UnixNano(), UpdatedUnixNano: base.Add(time.Second).UnixNano()}, Case: contract.HumanCaseExactRefV2{TenantID: target.TenantID, ID: "case-human-external", Revision: 1, Digest: externalDigestV1("case-human")}, Target: contract.HumanTargetExactRefV2{TenantID: target.TenantID, ID: target.ID, Revision: target.Revision, Digest: target.Digest}, Round: contract.HumanRoundExactRefV2{TenantID: target.TenantID, ID: "round-human-external", Revision: 1, Digest: externalDigestV1("round-human")}, QuorumPolicy: policyBinding, ResponsibilitySubject: responsibility, State: contract.HumanPanelProposedV2, AcceptThreshold: 1, MaximumPanelSize: 1, RoleRequirements: []contract.HumanRoleRequirementV2{{Role: "technical", Minimum: 1}}, RejectVetoRoles: []string{"security"}, DelegationRequired: true, ProductionSelfReviewAllowed: false, MaxPanelDurationNanos: int64(10 * time.Minute), MaxVoteTTLNanos: int64(5 * time.Minute), ExpiresUnixNano: base.Add(6 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}

	bindingStore := fakes.NewReviewBindingCurrentStoreV1(clock.Now)
	sourceSet, sourceFact := commitExternalBindingComponentV1(t, bindingStore, base, "human-source-set", "human-source-binding", "review/human", "review/attest")
	consumerSet, consumerFact := commitExternalBindingComponentV1(t, bindingStore, base, "human-consumer-set", "human-consumer-binding", "review/verdict-owner", "runtime/read-review-binding-current")
	bindingSource := runtimeports.ReviewComponentBindingRefV2{BindingSetID: sourceSet.ID, BindingSetRevision: sourceSet.Revision, ComponentID: sourceFact.ComponentID, ManifestDigest: sourceFact.ManifestDigest, ArtifactDigest: sourceFact.Manifest.ArtifactDigest, Capability: "review/attest"}
	consumer := runtimeports.ProviderBindingRefV2{BindingSetID: consumerSet.ID, BindingSetRevision: consumerSet.Revision, ComponentID: consumerFact.ComponentID, ManifestDigest: consumerFact.ManifestDigest, ArtifactDigest: consumerFact.Manifest.ArtifactDigest, Capability: "runtime/read-review-binding-current"}
	association, err := runtimeports.SealReviewBindingConsumerAssociationCurrentProjectionV1(runtimeports.ReviewBindingConsumerAssociationCurrentProjectionV1{Ref: runtimeports.ReviewBindingConsumerAssociationRefV1{Revision: 1}, Consumer: consumer, Source: bindingSource, Current: true, CheckedUnixNano: base.UnixNano(), ExpiresUnixNano: base.Add(4 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bindingStore.CreateReviewBindingConsumerAssociationV1(context.Background(), association); err != nil {
		t.Fatal(err)
	}
	reviewerBinding, reviewerFact := externalAuthorityV1(t, "reviewer-human-v2", target.Scope, target.ActionScopeDigest, base.Add(7*time.Minute))
	delegationID, _ := organizationcontract.DeriveDelegationIDV1(target.TenantID, managerSubject, reviewerSubject, "technical", target.ActionScopeDigest)
	assignment, err := contract.SealHumanPanelAssignmentV2(contract.HumanPanelAssignmentV2{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "assignment-human-external", Revision: 1, CreatedUnixNano: base.Add(time.Second).UnixNano(), UpdatedUnixNano: base.Add(time.Second).UnixNano()}, Panel: proposed.ExactRef(), Case: proposed.Case, Round: proposed.Round, Target: proposed.Target, ReviewerIdentity: contract.HumanIdentityProofRefV2{TenantID: target.TenantID, Ref: reviewerID, Revision: 1, Digest: externalDigestV1("reviewer-identity-human")}, ReviewerAuthority: reviewerBinding, ReviewerBinding: bindingSource, Roles: []string{"technical"}, Delegated: true, DelegatorIdentity: contract.HumanIdentityProofRefV2{TenantID: target.TenantID, Ref: managerID, Revision: 1, Digest: externalDigestV1("manager-identity-human")}, DelegateIdentity: contract.HumanIdentityProofRefV2{TenantID: target.TenantID, Ref: reviewerID, Revision: 1, Digest: externalDigestV1("reviewer-identity-human")}, DelegationFact: contract.HumanDelegationFactRefV2{TenantID: target.TenantID, Ref: delegationID, Revision: 1, Digest: externalDigestV1("delegation-human")}, DelegatedRole: "technical", DelegationScopeDigest: target.ActionScopeDigest, State: contract.HumanAssignmentOfferedV2, ExpiresUnixNano: base.Add(6 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	open := proposed.Clone()
	open.Revision = 2
	open.State = contract.HumanPanelOpenV2
	open.UpdatedUnixNano++
	open.AssignmentRefs = []contract.HumanPanelAssignmentExactRefV2{assignment.ExactRef()}
	open.Digest = ""
	open, err = contract.SealHumanReviewPanelV2(open)
	if err != nil {
		t.Fatal(err)
	}
	targetRef := runtimeports.ReviewDecisionTargetRefV1{TenantID: target.TenantID, ID: target.ID, Revision: target.Revision, Digest: target.Digest, RunID: target.RunID}
	assignmentRef := runtimeports.ReviewDecisionAssignmentRefV1{TenantID: assignment.TenantID, ID: assignment.ID, Revision: assignment.Revision, Digest: assignment.Digest, ReviewerID: assignment.ReviewerIdentity.Ref}
	bindingSubject := runtimeports.ReviewBindingSubjectV1{TenantID: target.TenantID, AssignmentID: assignment.ID, AssignmentRevision: assignment.Revision, AssignmentDigest: assignment.Digest, ReviewerID: assignment.ReviewerIdentity.Ref, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest}
	input := runtimeports.CreateReviewBindingProjectionCommandInputV1{Source: bindingSource, Subject: bindingSubject, Association: association.Ref}
	publishRef, _ := runtimeports.DeriveCreateReviewBindingProjectionPublishRefV1(input)
	if _, err = bindingStore.CreateReviewBindingProjectionV1(context.Background(), runtimeports.CreateReviewBindingProjectionRequestV1{PublishRef: publishRef, Input: input}); err != nil {
		t.Fatal(err)
	}
	actorSubject := runtimeports.ReviewDecisionAuthorityCurrentSubjectV1{Role: runtimeports.ReviewDecisionAuthorityActorV1, Target: targetRef, Assignment: assignmentRef, Authority: actorBinding, ActionScopeDigest: target.ActionScopeDigest}
	reviewerAuthoritySubject := runtimeports.ReviewDecisionAuthorityCurrentSubjectV1{Role: runtimeports.ReviewDecisionAuthorityReviewerV1, Target: targetRef, Assignment: assignmentRef, Authority: reviewerBinding, ActionScopeDigest: target.ActionScopeDigest}
	authorityValues := map[runtimeports.ReviewDecisionAuthorityCurrentSubjectV1]runtimeports.ReviewDecisionAuthorityCurrentProjectionV1{actorSubject: sealExternalAuthorityV1(t, actorSubject, actorFact, base, base.Add(8*time.Minute)), reviewerAuthoritySubject: sealExternalAuthorityV1(t, reviewerAuthoritySubject, reviewerFact, base, base.Add(7*time.Minute))}
	scopeSubject := runtimeports.ReviewDecisionScopeCurrentSubjectV1{TenantID: target.TenantID, Target: targetRef, RunID: target.RunID, Scope: target.Scope, CurrentScope: target.CurrentScope, ActionScopeDigest: target.ActionScopeDigest}
	scopeValue := sealExternalScopeV1(t, scopeSubject, scopeFact, base, base.Add(9*time.Minute))
	request := reviewport.HumanOrganizationCurrentRequestV2{Panel: proposed, Assignment: assignment, ReviewerSubjectID: reviewerSubject, DelegatorSubjectID: managerSubject, ActionScopeDigest: target.ActionScopeDigest}
	review := &humanReviewReaderV2{target: target, assignments: []contract.HumanPanelAssignmentV2{assignment}}
	coordinates := &humanCoordinateReaderV2{requests: []reviewport.HumanOrganizationCurrentRequestV2{request}}
	organization := &humanOrganizationReaderV2{expires: base.Add(3 * time.Minute).UnixNano(), checked: base.UnixNano()}
	policy := &humanPolicyReaderV2{value: policyValue}
	authority := &humanAuthorityReaderV2{values: authorityValues}
	binding := &humanBindingReaderV2{inner: bindingStore}
	scope := &humanScopeReaderV2{value: scopeValue}
	source, err := multisigcurrent.NewExternalSourceV2(review, organization, coordinates, policy, authority, binding, scope, noEvidenceReaderV1{}, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	return humanExternalFixtureV2{base: base, clock: clock, target: target, proposed: proposed, open: open, assignment: assignment, review: review, coordinates: coordinates, organization: organization, policy: policy, authority: authority, binding: binding, scope: scope, source: source}
}

func humanDecisionValuesV2(t *testing.T, fixture humanExternalFixtureV2) (contract.HumanAttestationV2, contract.HumanReviewPanelV2, contract.HumanReviewPanelV2, contract.HumanQuorumDecisionV2, contract.HumanVerdictV2) {
	t.Helper()
	evidence := []runtimeports.ReviewEvidenceRefV2{testkit.Evidence("human-external")}
	evidenceDigest, _ := contract.ComputeReviewEvidenceDigestV1(evidence)
	attestation, err := contract.SealHumanAttestationV2(contract.HumanAttestationV2{FactIdentityV1: contract.FactIdentityV1{TenantID: fixture.target.TenantID, ID: "attestation-human-external", Revision: 1, CreatedUnixNano: fixture.base.Add(2 * time.Second).UnixNano(), UpdatedUnixNano: fixture.base.Add(2 * time.Second).UnixNano()}, IdempotencyKey: "idem-human-external", Panel: fixture.open.ExactRef(), Assignment: fixture.assignment.ExactRef(), Case: fixture.open.Case, Round: fixture.open.Round, Target: fixture.open.Target, Policy: fixture.open.QuorumPolicy, ResponsibilitySubject: fixture.open.ResponsibilitySubject, ReviewerIdentity: fixture.assignment.ReviewerIdentity, ReviewerAuthority: fixture.assignment.ReviewerAuthority, Delegation: &fixture.assignment.DelegationFact, ReviewerBinding: fixture.assignment.ReviewerBinding, Resolution: contract.ResolutionAcceptV1, ReasonCodes: []string{"review/verified"}, Evidence: evidence, EvidenceDigest: evidenceDigest, ObservedUnixNano: fixture.base.Add(2 * time.Second).UnixNano(), ExpiresUnixNano: fixture.base.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	quorumPanel := fixture.open.Clone()
	quorumPanel.Revision++
	quorumPanel.State = contract.HumanPanelQuorumSatisfiedV2
	quorumPanel.UpdatedUnixNano = fixture.base.Add(3 * time.Second).UnixNano()
	quorumPanel.Digest = ""
	quorumPanel, err = contract.SealHumanReviewPanelV2(quorumPanel)
	if err != nil {
		t.Fatal(err)
	}
	reviewerSet, _ := contract.ComputeHumanReviewerSetDigestV2([]contract.HumanIdentityProofRefV2{fixture.assignment.ReviewerIdentity})
	quorum, err := contract.SealHumanQuorumDecisionV2(contract.HumanQuorumDecisionV2{FactIdentityV1: contract.FactIdentityV1{TenantID: fixture.target.TenantID, ID: "quorum-human-external", Revision: 1, CreatedUnixNano: fixture.base.Add(3 * time.Second).UnixNano(), UpdatedUnixNano: fixture.base.Add(3 * time.Second).UnixNano()}, Panel: quorumPanel.ExactRef(), Policy: fixture.open.QuorumPolicy, AcceptedAttestationRefs: []contract.HumanAttestationExactRefV2{attestation.ExactRef()}, DistinctReviewerIdentityRefs: []contract.HumanIdentityProofRefV2{fixture.assignment.ReviewerIdentity}, SatisfiedRoleCounts: []contract.HumanSatisfiedRoleCountV2{{Role: "technical", DistinctCurrentCount: 1}}, AcceptCount: 1, Threshold: 1, Resolution: contract.ResolutionAcceptV1, EvidenceSetDigest: evidenceDigest, ReviewerSetDigest: reviewerSet, CheckedUnixNano: fixture.base.Add(3 * time.Second).UnixNano(), ExpiresUnixNano: fixture.base.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	deciding := quorumPanel.Clone()
	deciding.Revision++
	deciding.State = contract.HumanPanelDecidingV2
	deciding.UpdatedUnixNano = fixture.base.Add(4 * time.Second).UnixNano()
	deciding.Digest = ""
	deciding, err = contract.SealHumanReviewPanelV2(deciding)
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := contract.SealHumanVerdictV2(contract.HumanVerdictV2{FactIdentityV1: contract.FactIdentityV1{TenantID: fixture.target.TenantID, ID: "verdict-human-external", Revision: 1, CreatedUnixNano: fixture.base.Add(4 * time.Second).UnixNano(), UpdatedUnixNano: fixture.base.Add(4 * time.Second).UnixNano()}, Case: deciding.Case, Target: deciding.Target, Round: deciding.Round, Panel: deciding.ExactRef(), QuorumDecision: quorum.ExactRef(), Policy: deciding.QuorumPolicy, Scope: fixture.target.Scope, CurrentScope: fixture.target.CurrentScope, ReviewerSetDigest: reviewerSet, ReviewerAuthorityRefs: []runtimeports.AuthorityBindingRefV2{fixture.assignment.ReviewerAuthority}, BindingClosures: []runtimeports.ReviewComponentBindingRefV2{fixture.assignment.ReviewerBinding}, AttestationRefs: []contract.HumanAttestationExactRefV2{attestation.ExactRef()}, Evidence: evidence, EvidenceSetDigest: evidenceDigest, ReasonCodes: []string{"review/quorum"}, State: contract.HumanVerdictAcceptedV2, ExpiresUnixNano: fixture.base.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return attestation, quorumPanel, deciding, quorum, verdict
}

type humanExternalClockV2 struct {
	mu         sync.Mutex
	value      time.Time
	calls      int
	rollbackAt int
	jumpAt     int
	jumpTo     time.Time
}

func (c *humanExternalClockV2) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.jumpAt > 0 && c.calls >= c.jumpAt {
		c.value = c.jumpTo
		c.jumpAt = 0
	}
	if c.rollbackAt > 0 && c.calls >= c.rollbackAt {
		return c.value.Add(-time.Minute)
	}
	c.value = c.value.Add(time.Nanosecond)
	return c.value
}

type humanReviewReaderV2 struct {
	mu          sync.Mutex
	target      contract.TargetSnapshotV1
	assignments []contract.HumanPanelAssignmentV2
}

func (r *humanReviewReaderV2) InspectTargetExactV1(_ context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.TargetSnapshotV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value := r.target
	value.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), value.Evidence...)
	if value.TenantID != tenant || value.ID != ref.ID || value.Revision != ref.Revision || value.Digest != ref.Digest {
		return contract.TargetSnapshotV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Target exact ref drifted")
	}
	return value, nil
}
func (r *humanReviewReaderV2) ListHumanPanelAssignmentsV2(context.Context, contract.HumanPanelExactRefV2) ([]contract.HumanPanelAssignmentV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneHumanAssignmentsV2(r.assignments), nil
}

type humanCoordinateReaderV2 struct {
	mu              sync.Mutex
	requests        []reviewport.HumanOrganizationCurrentRequestV2
	omit            bool
	lose            bool
	mutateInput     bool
	panelExactDrift bool
	calls           int
	cancel          context.CancelFunc
}

func (r *humanCoordinateReaderV2) InspectHumanOrganizationCurrentRequestsV2(_ context.Context, panel contract.HumanReviewPanelV2, assignments []contract.HumanPanelAssignmentV2) ([]reviewport.HumanOrganizationCurrentRequestV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.lose && r.calls == 1 {
		if r.cancel != nil {
			r.cancel()
		}
		return nil, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "lost exact Organization coordinate reply")
	}
	if r.omit {
		return []reviewport.HumanOrganizationCurrentRequestV2{}, nil
	}
	out := make([]reviewport.HumanOrganizationCurrentRequestV2, len(r.requests))
	for i := range r.requests {
		out[i] = r.requests[i].Clone()
		out[i].Panel = panel.Clone()
		out[i].Assignment = assignments[i].Clone()
		if r.panelExactDrift {
			drifted := out[i].Panel.Clone()
			drifted.Revision++
			drifted.UpdatedUnixNano++
			drifted.Digest = ""
			sealed, err := contract.SealHumanReviewPanelV2(drifted)
			if err != nil {
				return nil, err
			}
			out[i].Panel = sealed
		}
	}
	if r.mutateInput {
		assignments[0].Roles[0] = "mutated"
		panel.RoleRequirements[0].Role = "mutated"
	}
	return out, nil
}

type humanOrganizationReaderV2 struct {
	mu      sync.Mutex
	calls   int
	driftAt int
	checked int64
	expires int64
}

func (r *humanOrganizationReaderV2) InspectHumanOrganizationCurrentV2(_ context.Context, requests []reviewport.HumanOrganizationCurrentRequestV2) (reviewport.HumanOrganizationCurrentCutV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	items := make([]reviewport.HumanOrganizationAssignmentCurrentV2, 0, len(requests))
	for _, request := range requests {
		source, err := request.OrganizationSourceV1()
		if err != nil {
			return reviewport.HumanOrganizationCurrentCutV2{}, err
		}
		requestDigest, _ := request.Digest()
		owner := organizationcontract.ReviewEligibilityProjectionRefV1{TenantID: request.Panel.TenantID, ID: "org-current-" + request.Assignment.ID, Source: source, Identity: organizationcontract.IdentityRefV1{TenantID: request.Panel.TenantID, ID: request.Assignment.ReviewerIdentity.Ref, Revision: request.Assignment.ReviewerIdentity.Revision, Digest: request.Assignment.ReviewerIdentity.Digest}, Roles: []organizationcontract.RoleGrantRefV1{{TenantID: request.Panel.TenantID, ID: "role-" + request.Assignment.ID, Revision: 1, Digest: externalDigestV1("role-" + request.Assignment.ID)}}, Delegation: &organizationcontract.DelegationRefV1{TenantID: request.Panel.TenantID, ID: request.Assignment.DelegationFact.Ref, Revision: request.Assignment.DelegationFact.Revision, Digest: request.Assignment.DelegationFact.Digest}, Responsibility: organizationcontract.ResponsibilityRefV1{TenantID: request.Panel.TenantID, ID: request.Panel.ResponsibilitySubject.Ref, Revision: request.Panel.ResponsibilitySubject.Revision, Digest: request.Panel.ResponsibilitySubject.Digest}, Digest: externalDigestV1("org-current-" + request.Assignment.ID)}
		if r.driftAt > 0 && r.calls >= r.driftAt {
			owner.Digest = externalDigestV1("drifted-org")
		}
		items = append(items, reviewport.HumanOrganizationAssignmentCurrentV2{RequestDigest: requestDigest, Assignment: request.Assignment.ExactRef(), ReviewerIdentity: request.Assignment.ReviewerIdentity, OwnerProjectionRef: owner, CheckedUnixNano: r.checked, ExpiresUnixNano: r.expires, ProjectionDigest: owner.Digest})
	}
	return reviewport.SealHumanOrganizationCurrentCutV2(reviewport.HumanOrganizationCurrentCutV2{TenantID: requests[0].Panel.TenantID, Items: items, CheckedUnixNano: r.checked, ExpiresUnixNano: r.expires})
}

type humanPolicyReaderV2 struct {
	mu           sync.Mutex
	value        runtimeports.HumanQuorumPolicyCurrentProjectionV2
	resolveCalls int
	inspectCalls int
	driftAt      int
	loseInspect  bool
	cancel       context.CancelFunc
}

func (r *humanPolicyReaderV2) ResolveCurrentHumanQuorumPolicyV2(_ context.Context, request runtimeports.HumanQuorumPolicyCurrentResolveRequestV2) (runtimeports.HumanQuorumPolicyCurrentProjectionRefV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolveCalls++
	if request.Subject != r.value.Subject {
		return runtimeports.HumanQuorumPolicyCurrentProjectionRefV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Policy subject drifted")
	}
	return r.value.Ref, nil
}
func (r *humanPolicyReaderV2) InspectCurrentHumanQuorumPolicyV2(ctx context.Context, subject runtimeports.HumanQuorumPolicyCurrentSubjectV2, ref runtimeports.HumanQuorumPolicyCurrentProjectionRefV2) (runtimeports.HumanQuorumPolicyCurrentProjectionV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inspectCalls++
	if r.loseInspect && r.inspectCalls == 1 {
		if r.cancel != nil {
			r.cancel()
		}
		return runtimeports.HumanQuorumPolicyCurrentProjectionV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "lost Policy exact reply")
	}
	if err := ctx.Err(); err != nil {
		return runtimeports.HumanQuorumPolicyCurrentProjectionV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "Policy context ended")
	}
	value := r.value.Clone()
	if r.driftAt > 0 && r.inspectCalls >= r.driftAt {
		value.Ref.Revision++
	}
	if value.Subject != subject || value.Ref != ref {
		return runtimeports.HumanQuorumPolicyCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Policy current drifted")
	}
	return value, nil
}
func (r *humanPolicyReaderV2) InspectHistoricalHumanQuorumPolicyV2(context.Context, runtimeports.HumanQuorumPolicyCurrentProjectionRefV2) (runtimeports.HumanQuorumPolicyCurrentProjectionV2, error) {
	return r.value.Clone(), nil
}

type humanAuthorityReaderV2 struct {
	mu      sync.Mutex
	values  map[runtimeports.ReviewDecisionAuthorityCurrentSubjectV1]runtimeports.ReviewDecisionAuthorityCurrentProjectionV1
	calls   int
	driftAt int
}

func (r *humanAuthorityReaderV2) ResolveCurrentReviewDecisionAuthorityV1(_ context.Context, request runtimeports.ReviewDecisionAuthorityCurrentResolveRequestV1) (runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.values[request.Subject].Ref, nil
}
func (r *humanAuthorityReaderV2) InspectCurrentReviewDecisionAuthorityV1(_ context.Context, subject runtimeports.ReviewDecisionAuthorityCurrentSubjectV1, ref runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1) (runtimeports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	value := r.values[subject]
	if r.driftAt > 0 && r.calls >= r.driftAt {
		value.Ref.Revision++
	}
	if value.Ref != ref {
		return runtimeports.ReviewDecisionAuthorityCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Authority current drifted")
	}
	return value, nil
}
func (r *humanAuthorityReaderV2) InspectHistoricalReviewDecisionAuthorityV1(context.Context, runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1) (runtimeports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
	return runtimeports.ReviewDecisionAuthorityCurrentProjectionV1{}, nil
}

type humanBindingReaderV2 struct {
	mu             sync.Mutex
	inner          runtimeports.ReviewBindingAuthoritativeCurrentReaderV1
	calls, driftAt int
}

func (r *humanBindingReaderV2) ResolveCurrentReviewBindingV1(ctx context.Context, request runtimeports.ResolveReviewBindingCurrentRequestV1) (runtimeports.ReviewBindingProjectionRefV1, error) {
	return r.inner.ResolveCurrentReviewBindingV1(ctx, request)
}
func (r *humanBindingReaderV2) InspectReviewBindingProjectionV1(ctx context.Context, request runtimeports.InspectReviewBindingProjectionRequestV1) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
	return r.inner.InspectReviewBindingProjectionV1(ctx, request)
}
func (r *humanBindingReaderV2) InspectCurrentReviewBindingV1(ctx context.Context, request runtimeports.InspectCurrentReviewBindingRequestV1) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
	r.mu.Lock()
	r.calls++
	drift := r.driftAt > 0 && r.calls >= r.driftAt
	r.mu.Unlock()
	if drift {
		return runtimeports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Binding current drifted")
	}
	return r.inner.InspectCurrentReviewBindingV1(ctx, request)
}

type humanScopeReaderV2 struct {
	mu             sync.Mutex
	value          runtimeports.ReviewDecisionScopeCurrentProjectionV1
	calls, driftAt int
}

func (r *humanScopeReaderV2) ResolveCurrentReviewDecisionScopeV1(context.Context, runtimeports.ReviewDecisionScopeCurrentResolveRequestV1) (runtimeports.ReviewDecisionScopeCurrentProjectionRefV1, error) {
	return r.value.Ref, nil
}
func (r *humanScopeReaderV2) InspectCurrentReviewDecisionScopeV1(_ context.Context, subject runtimeports.ReviewDecisionScopeCurrentSubjectV1, ref runtimeports.ReviewDecisionScopeCurrentProjectionRefV1) (runtimeports.ReviewDecisionScopeCurrentProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	value := r.value
	if r.driftAt > 0 && r.calls >= r.driftAt {
		value.Ref.Revision++
	}
	if value.Subject != subject || value.Ref != ref {
		return runtimeports.ReviewDecisionScopeCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Scope current drifted")
	}
	return value, nil
}
func (r *humanScopeReaderV2) InspectHistoricalReviewDecisionScopeV1(context.Context, runtimeports.ReviewDecisionScopeCurrentProjectionRefV1) (runtimeports.ReviewDecisionScopeCurrentProjectionV1, error) {
	return r.value, nil
}

func cloneHumanAssignmentsV2(values []contract.HumanPanelAssignmentV2) []contract.HumanPanelAssignmentV2 {
	out := append([]contract.HumanPanelAssignmentV2(nil), values...)
	for i := range out {
		out[i] = out[i].Clone()
	}
	return out
}
