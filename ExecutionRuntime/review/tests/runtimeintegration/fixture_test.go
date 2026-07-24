package runtimeintegration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type fixtureV4 struct {
	now      time.Time
	intent   runtimeports.OperationEffectIntentV3
	snapshot runtimeadapter.CurrentFactSnapshotV4
}

func digest(label string) core.Digest { return core.DigestBytes([]byte(label)) }

func schema() runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "review.test", Name: "operation", Version: "1.0.0", MediaType: "application/json", ContentDigest: digest("schema")}
}

func authority(label string) runtimeports.AuthorityBindingRefV2 {
	return runtimeports.AuthorityBindingRefV2{Ref: label, Revision: 1, Digest: digest(label), Epoch: 1}
}

func reviewerBinding() runtimeports.ReviewComponentBindingRefV2 {
	return runtimeports.ReviewComponentBindingRefV2{BindingSetID: "review-binding", BindingSetRevision: 1, ComponentID: "review.test/reviewer", ManifestDigest: digest("review-manifest"), ArtifactDigest: digest("review-artifact"), Capability: "review.test/attest"}
}

func newFixtureV4(t *testing.T, state contract.VerdictStateV1, notRequired bool) fixtureV4 {
	t.Helper()
	now := time.Unix(910_000, 0)
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-review", ID: "agent-review", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-review", PlanDigest: digest("plan")}, Instance: core.InstanceRef{ID: "instance-review", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	currentScope := runtimeports.ExecutionScopeBindingRefV2{Ref: "operation-current", Revision: 1, Digest: digest("operation-current")}
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeRunV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, RunID: "run-review", SubjectRevision: 1, CurrentProjectionRef: currentScope.Ref, CurrentProjectionRevision: currentScope.Revision, CurrentProjectionDigest: currentScope.Digest}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	actor := authority("actor-authority")
	reviewer := authority("reviewer-authority")
	subjectDigest := digest("review-subject")
	policy := runtimeports.ReviewPolicyFactV2{Ref: "review-policy", Revision: 1, SubjectDigest: subjectDigest, Scope: scope, RunID: operation.RunID, CurrentScope: currentScope, RiskClass: "review.test/controlled", ActorAuthorityRef: actor.Ref, ReviewerAuthorityRef: reviewer.Ref, OperationNotRequired: notRequired, PolicyDecisionRef: "review-policy-decision", Active: true, ExpiresUnixNano: now.Add(25 * time.Minute).UnixNano()}
	policyDigest, err := policy.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	policy.Digest = policyDigest
	policyBinding := runtimeports.ReviewPolicyBindingRefV2{Ref: policy.Ref, Revision: policy.Revision, Digest: policy.Digest}
	payload := []byte(`{"operation":"review"}`)
	payloadDigest := core.DigestBytes(payload)
	targetEvidence := []runtimeports.ReviewEvidenceRefV2{{Ref: "review-evidence://target", Classification: "review.test/target", Digest: digest("target-evidence")}}
	targetEvidenceDigest, err := contract.ComputeReviewEvidenceDigestV1(targetEvidence)
	if err != nil {
		t.Fatal(err)
	}
	target, err := contract.SealTargetSnapshotV1(contract.TargetSnapshotV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: "tenant-review", ID: "review-target", Revision: 2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, Kind: contract.TargetEffectV1, PayloadSchema: schema(), PayloadDigest: payloadDigest, PayloadRevision: 3, Scope: scope, RunID: operation.RunID, ActionScopeDigest: digest("action-scope"), IntentID: "operation-effect", IntentRevision: 4, SubjectDigest: subjectDigest, Policy: policyBinding, ActorAuthority: actor, CurrentScope: currentScope, Evidence: targetEvidence, EvidenceSetDigest: targetEvidenceDigest, ContextFrameDigest: digest("context-frame"), ExpiresUnixNano: now.Add(60 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	round, err := contract.SealReviewRoundV1(contract.ReviewRoundV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: target.TenantID, ID: "review-round", Revision: 1, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), UpdatedUnixNano: now.Add(-time.Minute).UnixNano()}, CaseID: "review-case", CaseRevision: 1, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, Route: contract.RouteHumanV1, State: contract.RoundAttestedV1, AssignmentID: "assignment-current", ContextFrameDigest: target.ContextFrameDigest, RubricDigest: digest("rubric"), ExpiresUnixNano: now.Add(20 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	assignment, err := contract.SealReviewerAssignmentV1(contract.ReviewerAssignmentV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: target.TenantID, ID: "assignment-current", Revision: 2, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), UpdatedUnixNano: now.UnixNano()}, CaseID: "review-case", CaseRevision: round.CaseRevision, RoundID: round.ID, RoundRevision: round.Revision, RoundDigest: round.Digest, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, Route: contract.RouteHumanV1, ReviewerID: "human-reviewer", ReviewerAuthority: reviewer, ReviewerBinding: reviewerBinding(), Capability: "review.test/attest", State: contract.AssignmentClaimedV1, LeaseHolder: "human-reviewer", LeaseExpiresUnixNano: now.Add(10 * time.Minute).UnixNano(), ExpiresUnixNano: now.Add(20 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	decisionReviewEvidence := runtimeports.ReviewEvidenceRefV2{Ref: "review-evidence://decision", Classification: "review.test/decision", Digest: digest("decision-evidence")}
	decisionEvidenceDigest, err := contract.ComputeReviewEvidenceDigestV1([]runtimeports.ReviewEvidenceRefV2{decisionReviewEvidence})
	if err != nil {
		t.Fatal(err)
	}
	resolution := contract.ResolutionAcceptV1
	conditionsDigest := core.Digest("")
	var conditions []runtimeports.ReviewConditionV2
	if state == contract.VerdictConditionalV1 {
		resolution = contract.ResolutionConditionalV1
		conditions = []runtimeports.ReviewConditionV2{{ID: "review.test/condition", Revision: 1, Schema: schema(), ConstraintDigest: digest("condition-constraint"), SatisfactionOwner: reviewerBinding(), ScopeDigest: target.ActionScopeDigest, Authority: target.ActorAuthority, ExpiresUnixNano: now.Add(20 * time.Minute).UnixNano()}}
		conditionsDigest, err = runtimeports.DigestReviewConditionsV2(conditions)
		if err != nil {
			t.Fatal(err)
		}
	} else if state == contract.VerdictRejectedV1 {
		resolution = contract.ResolutionRejectV1
	}
	attestation, err := contract.SealAttestationV1(contract.AttestationV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: target.TenantID, ID: "attestation-current", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, IdempotencyKey: "attestation-idempotency", CaseID: "review-case", CaseRevision: 2, RoundID: assignment.RoundID, RoundRevision: round.Revision, RoundDigest: round.Digest, AssignmentID: assignment.ID, AssignmentRevision: assignment.Revision, AssignmentDigest: assignment.Digest, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, ContextFrameDigest: target.ContextFrameDigest, Route: contract.RouteHumanV1, ReviewerID: assignment.ReviewerID, ReviewerAuthority: reviewer, ReviewerBinding: assignment.ReviewerBinding, Resolution: resolution, ReasonCodes: []string{"review.test/checked"}, Evidence: []runtimeports.ReviewEvidenceRefV2{decisionReviewEvidence}, EvidenceDigest: decisionEvidenceDigest, Conditions: conditions, ConditionsDigest: conditionsDigest, ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(15 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	preCase, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: target.TenantID, ID: "review-case", Revision: 3, CreatedUnixNano: now.Add(-2 * time.Minute).UnixNano(), UpdatedUnixNano: now.UnixNano()}, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, State: contract.CaseAttestedV1, CurrentRoundID: assignment.RoundID, CurrentAssignment: assignment.ID, ExpiresUnixNano: now.Add(30 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	verdict := contract.VerdictV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: target.TenantID, ID: "review-verdict", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, CaseID: "review-case", CaseRevision: preCase.Revision, CaseDigest: preCase.Digest, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, PayloadRevision: target.PayloadRevision, PayloadDigest: target.PayloadDigest, Scope: target.Scope, ActionScopeDigest: target.ActionScopeDigest, TargetEvidenceSetDigest: target.EvidenceSetDigest, ContextFrameDigest: target.ContextFrameDigest, IntentID: target.IntentID, IntentRevision: target.IntentRevision, SubjectDigest: target.SubjectDigest, Policy: target.Policy, ActorAuthority: actor, ReviewerAuthority: reviewer, CurrentScope: target.CurrentScope, RoundID: round.ID, RoundRevision: round.Revision, RoundDigest: round.Digest, AssignmentID: assignment.ID, AssignmentRevision: assignment.Revision, AssignmentDigest: assignment.Digest, ReviewerID: assignment.ReviewerID, ReviewerBinding: assignment.ReviewerBinding, State: state, AttestationRefs: []string{attestation.ID}, ReasonCodes: []string{"review.test/decided"}, FindingDigest: digest("finding-set"), EvidenceDigest: attestation.EvidenceDigest, Conditions: conditions, ConditionsDigest: conditionsDigest, ExpiresUnixNano: now.Add(18 * time.Minute).UnixNano()}
	if state == contract.VerdictExpiredV1 || state == contract.VerdictRevokedV1 || state == contract.VerdictSupersededV1 {
		verdict.InvalidationReason = core.ReasonReviewVerdictStale
	}
	sealedVerdict, err := contract.SealVerdictV1(verdict)
	if err != nil {
		t.Fatal(err)
	}
	caseFact := preCase
	caseFact.Revision++
	caseFact.State = contract.CaseResolvedV1
	caseFact.VerdictID, caseFact.VerdictRevision, caseFact.VerdictDigest = sealedVerdict.ID, sealedVerdict.Revision, sealedVerdict.Digest
	caseFact.Digest = ""
	caseFact, err = contract.SealReviewCaseV1(caseFact)
	if err != nil {
		t.Fatal(err)
	}
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "operation-binding", BindingSetRevision: 1, ComponentID: "review.test/provider", ManifestDigest: digest("provider-manifest"), ArtifactDigest: digest("provider-artifact"), Capability: "review.test/execute"}
	ownerManifest := digest("owner-manifest")
	intent := runtimeports.OperationEffectIntentV3{ContractVersion: runtimeports.OperationEffectContractVersionV3, ID: target.IntentID, Revision: target.IntentRevision, Operation: operation, Kind: "review.test/effect", RiskClass: policy.RiskClass, ActionScopeDigest: target.ActionScopeDigest, Payload: runtimeports.OpaquePayloadV2{Schema: target.PayloadSchema, ContentDigest: payloadDigest, Length: uint64(len(payload)), Inline: payload, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "review.test/payload-limit", Digest: digest("payload-limit")}}, PayloadRevision: target.PayloadRevision, Target: target.ID, ConflictDomain: runtimeports.ConflictDomainBindingV2{Domain: "review.test/conflict", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(scope.Identity.TenantID)}, Owners: []runtimeports.EffectOwnerRefV2{{Role: runtimeports.OwnerCleanup, ComponentID: "review.test/owner", ManifestDigest: ownerManifest}, {Role: runtimeports.OwnerEffect, ComponentID: "review.test/owner", ManifestDigest: ownerManifest}, {Role: runtimeports.OwnerSettlement, ComponentID: "review.test/owner", ManifestDigest: ownerManifest}}, Provider: provider, Authority: actor, Review: runtimeports.OperationReviewBindingRefV3{CaseRef: caseFact.ID, CandidateDigest: target.Digest, CandidateRevision: target.Revision, PolicyDigest: target.Policy.Digest}, Budget: runtimeports.OperationBudgetBindingRefV3{Ref: "operation-budget", Revision: 1, Digest: digest("operation-budget"), PolicyDigest: digest("budget-policy"), SubjectDigest: operationDigest}, Policy: runtimeports.OperationPolicyBindingRefV3{Ref: "operation-policy", Revision: 1, Digest: digest("operation-policy"), SubjectDigest: operationDigest}, Idempotency: runtimeports.IdempotencyBindingV2{Key: "review-operation-idempotency", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(scope.Identity.TenantID), Class: core.IdempotencyQueryable}, CredentialLeases: []runtimeports.CredentialLeaseRefV2{}, ExpiresUnixNano: now.Add(35 * time.Minute).UnixNano()}
	if err := intent.Validate(); err != nil {
		t.Fatal(err)
	}
	decisionBinding := runtimeadapter.EvidenceBindingV4{Review: decisionReviewEvidence, Ledger: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: digest("decision-ledger"), Sequence: 1, RecordDigest: digest("decision-record")}}
	snapshot := runtimeadapter.CurrentFactSnapshotV4{Revision: 1, Target: target, Case: caseFact, Verdict: sealedVerdict, Rounds: []contract.ReviewRoundV1{round}, Assignments: []contract.ReviewerAssignmentV1{assignment}, Attestations: []contract.AttestationV1{attestation}, Policy: policy, DecisionEvidence: []runtimeadapter.EvidenceBindingV4{decisionBinding}, ReviewerAuthority: governanceRef(reviewer.Ref, reviewer.Revision, reviewer.Digest, now.Add(22*time.Minute)), Scope: governanceRef(currentScope.Ref, currentScope.Revision, currentScope.Digest, now.Add(22*time.Minute)), Binding: governanceRef(provider.BindingSetID, provider.BindingSetRevision, digest("operation-binding"), now.Add(22*time.Minute)), Current: true}
	if state == contract.VerdictConditionalV1 {
		proofEvidence := runtimeports.ReviewEvidenceRefV2{Ref: "review-evidence://condition", Classification: "review.test/condition", Digest: digest("condition-evidence")}
		proof := runtimeports.ReviewConditionProofV2{ConditionID: "review.test/condition", ConditionRevision: 1, ConstraintDigest: digest("condition-constraint"), Owner: reviewerBinding(), ScopeDigest: target.ActionScopeDigest, Authority: target.ActorAuthority, Evidence: proofEvidence, ExpiresUnixNano: now.Add(9 * time.Minute).UnixNano()}
		proofsDigest, err := runtimeports.DigestConditionProofsV2([]runtimeports.ReviewConditionProofV2{proof})
		if err != nil {
			t.Fatal(err)
		}
		satisfaction := runtimeports.ConditionSatisfactionFactV2{ID: "review-satisfaction", VerdictID: sealedVerdict.ID, VerdictRevision: sealedVerdict.Revision, VerdictDigest: sealedVerdict.Digest, CandidateDigest: target.Digest, IntentID: intent.ID, IntentRevision: intent.Revision, SubjectDigest: target.SubjectDigest, ConditionsDigest: sealedVerdict.ConditionsDigest, Policy: target.Policy, Scope: target.Scope, RunID: target.RunID, ActionScopeDigest: target.ActionScopeDigest, CurrentScope: target.CurrentScope, Proofs: []runtimeports.ReviewConditionProofV2{proof}, ProofsDigest: proofsDigest, State: runtimeports.ConditionSatisfied, Revision: 2, SatisfiedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(12 * time.Minute).UnixNano()}
		if err := satisfaction.Validate(); err != nil {
			t.Fatal(err)
		}
		snapshot.Satisfaction = &satisfaction
		snapshot.SatisfactionEvidence = []runtimeadapter.EvidenceBindingV4{{Review: proofEvidence, Ledger: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: digest("condition-ledger"), Sequence: 1, RecordDigest: digest("condition-record")}}}
	}
	return fixtureV4{now: now, intent: intent, snapshot: resealSnapshot(t, snapshot)}
}

func governanceRef(ref string, revision core.Revision, value core.Digest, expires time.Time) runtimeports.OperationGovernanceFactRefV3 {
	return runtimeports.OperationGovernanceFactRefV3{Ref: ref, Revision: revision, Digest: value, ExpiresUnixNano: expires.UnixNano()}
}

func resealSnapshot(t *testing.T, value runtimeadapter.CurrentFactSnapshotV4) runtimeadapter.CurrentFactSnapshotV4 {
	t.Helper()
	value.ExpiresUnixNano = minimumSnapshotExpiry(value)
	value.Digest = ""
	sealed, err := runtimeadapter.SealCurrentFactSnapshotV4(value)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func minimumSnapshotExpiry(value runtimeadapter.CurrentFactSnapshotV4) int64 {
	values := []int64{value.Target.ExpiresUnixNano, value.Case.ExpiresUnixNano, value.Verdict.ExpiresUnixNano, value.Policy.ExpiresUnixNano, value.ReviewerAuthority.ExpiresUnixNano, value.Scope.ExpiresUnixNano, value.Binding.ExpiresUnixNano}
	for _, round := range value.Rounds {
		values = append(values, round.ExpiresUnixNano)
	}
	for _, assignment := range value.Assignments {
		values = append(values, assignment.ExpiresUnixNano, assignment.LeaseExpiresUnixNano)
	}
	for _, attestation := range value.Attestations {
		values = append(values, attestation.ExpiresUnixNano)
		for _, condition := range attestation.Conditions {
			values = append(values, condition.ExpiresUnixNano)
		}
	}
	for _, condition := range value.Verdict.Conditions {
		values = append(values, condition.ExpiresUnixNano)
	}
	if value.Satisfaction != nil {
		values = append(values, value.Satisfaction.ExpiresUnixNano)
		for _, proof := range value.Satisfaction.Proofs {
			values = append(values, proof.ExpiresUnixNano)
		}
	}
	minimum := int64(0)
	for _, candidate := range values {
		if candidate > 0 && (minimum == 0 || candidate < minimum) {
			minimum = candidate
		}
	}
	return minimum
}

type atomicSourceV4 struct {
	mu            sync.RWMutex
	expected      runtimeadapter.ExactCurrentRequestV4
	snapshot      runtimeadapter.CurrentFactSnapshotV4
	loseReplies   int
	alwaysUnknown bool
	calls         int
}

func newAtomicSourceV4(t *testing.T, fixture fixtureV4) *atomicSourceV4 {
	t.Helper()
	intentDigest, err := fixture.intent.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	return &atomicSourceV4{expected: runtimeadapter.ExactCurrentRequestV4{Operation: fixture.intent.Operation, IntentID: fixture.intent.ID, IntentRevision: fixture.intent.Revision, IntentDigest: intentDigest, TargetID: fixture.intent.Target, TargetRevision: fixture.intent.Review.CandidateRevision, TargetDigest: fixture.intent.Review.CandidateDigest, CaseID: fixture.intent.Review.CaseRef}, snapshot: cloneTestSnapshot(fixture.snapshot)}
}

func (s *atomicSourceV4) InspectReviewCurrentFactsV4(_ context.Context, request runtimeadapter.ExactCurrentRequestV4) (runtimeadapter.CurrentFactSnapshotV4, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if !sameExactRequest(request, s.expected) {
		return runtimeadapter.CurrentFactSnapshotV4{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "exact Inspect key drifted")
	}
	value := cloneTestSnapshot(s.snapshot)
	if s.alwaysUnknown || s.loseReplies > 0 {
		if s.loseReplies > 0 {
			s.loseReplies--
		}
		return runtimeadapter.CurrentFactSnapshotV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Inspect reply loss")
	}
	return value, nil
}

func (s *atomicSourceV4) replace(value runtimeadapter.CurrentFactSnapshotV4) {
	s.mu.Lock()
	s.snapshot = cloneTestSnapshot(value)
	s.mu.Unlock()
}

func (s *atomicSourceV4) callCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.calls
}

func sameExactRequest(left, right runtimeadapter.ExactCurrentRequestV4) bool {
	return runtimeports.SameOperationSubjectV3(left.Operation, right.Operation) && left.IntentID == right.IntentID && left.IntentRevision == right.IntentRevision && left.IntentDigest == right.IntentDigest && left.TargetID == right.TargetID && left.TargetRevision == right.TargetRevision && left.TargetDigest == right.TargetDigest && left.CaseID == right.CaseID
}

func cloneTestSnapshot(value runtimeadapter.CurrentFactSnapshotV4) runtimeadapter.CurrentFactSnapshotV4 {
	value.Rounds = append([]contract.ReviewRoundV1(nil), value.Rounds...)
	value.Assignments = append([]contract.ReviewerAssignmentV1(nil), value.Assignments...)
	value.Attestations = append([]contract.AttestationV1(nil), value.Attestations...)
	for index := range value.Attestations {
		value.Attestations[index].ReasonCodes = append([]string(nil), value.Attestations[index].ReasonCodes...)
		value.Attestations[index].FindingRefs = append([]string(nil), value.Attestations[index].FindingRefs...)
		value.Attestations[index].Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), value.Attestations[index].Evidence...)
	}
	value.DecisionEvidence = append([]runtimeadapter.EvidenceBindingV4(nil), value.DecisionEvidence...)
	value.SatisfactionEvidence = append([]runtimeadapter.EvidenceBindingV4(nil), value.SatisfactionEvidence...)
	value.Target.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), value.Target.Evidence...)
	value.Verdict.AttestationRefs = append([]string(nil), value.Verdict.AttestationRefs...)
	value.Verdict.ReasonCodes = append([]string(nil), value.Verdict.ReasonCodes...)
	if value.Satisfaction != nil {
		fact := *value.Satisfaction
		fact.Proofs = append([]runtimeports.ReviewConditionProofV2(nil), fact.Proofs...)
		value.Satisfaction = &fact
	}
	return value
}
