package testkit

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ManualClock struct {
	mu    sync.RWMutex
	value time.Time
}

func NewClock(value time.Time) *ManualClock    { return &ManualClock{value: value} }
func (c *ManualClock) Now() time.Time          { c.mu.RLock(); defer c.mu.RUnlock(); return c.value }
func (c *ManualClock) Advance(d time.Duration) { c.mu.Lock(); c.value = c.value.Add(d); c.mu.Unlock() }
func (c *ManualClock) Set(v time.Time)         { c.mu.Lock(); c.value = v; c.mu.Unlock() }

func Digest(label string) core.Digest { return core.DigestBytes([]byte(label)) }

func Scope() core.ExecutionScope {
	return core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-a", ID: "agent-a", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-a", PlanDigest: Digest("plan")}, Instance: core.InstanceRef{ID: "instance-a", Epoch: 1}, AuthorityEpoch: 1}
}
func Schema(name string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "review.test", Name: name, Version: "1.0.0", MediaType: "application/json", ContentDigest: Digest("schema-" + name)}
}
func Authority(label string) runtimeports.AuthorityBindingRefV2 {
	return runtimeports.AuthorityBindingRefV2{Ref: label, Digest: Digest(label), Revision: 1, Epoch: 1}
}
func CurrentScope() runtimeports.ExecutionScopeBindingRefV2 {
	return runtimeports.ExecutionScopeBindingRefV2{Ref: "scope-current", Digest: Digest("scope-current"), Revision: 1}
}
func policyFact(now time.Time, scope core.ExecutionScope, runID core.AgentRunID, currentScope runtimeports.ExecutionScopeBindingRefV2, actor, reviewer runtimeports.AuthorityBindingRefV2, expires int64) runtimeports.ReviewPolicyFactV2 {
	fact := runtimeports.ReviewPolicyFactV2{Ref: "policy-a", Revision: 1, Scope: scope, RunID: runID, CurrentScope: currentScope, RiskClass: "review.test/risk", ActorAuthorityRef: actor.Ref, ReviewerAuthorityRef: reviewer.Ref, PolicyDecisionRef: "policy-decision-a", Active: true, ExpiresUnixNano: expires}
	digest, err := fact.DigestV2()
	if err != nil {
		panic(err)
	}
	fact.Digest = digest
	return fact
}
func ReviewerBinding() runtimeports.ReviewComponentBindingRefV2 {
	return runtimeports.ReviewComponentBindingRefV2{BindingSetID: "binding-a", BindingSetRevision: 1, ComponentID: "review.test/reviewer", ManifestDigest: Digest("reviewer-manifest"), ArtifactDigest: Digest("reviewer-artifact"), Capability: "review.test/attest"}
}
func Evidence(label string) runtimeports.ReviewEvidenceRefV2 {
	return runtimeports.ReviewEvidenceRefV2{Ref: "evidence://" + label, Classification: "review.test/observation", Digest: Digest("evidence-" + label)}
}

func Rubric(now time.Time, tenant core.TenantID) contract.RubricDefinitionV1 {
	value, err := contract.NewBaselineRubricDefinitionV1(contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: tenant, ID: "rubric-a", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, contract.RubricActionSafetyV1, now.Add(2*time.Hour).UnixNano())
	if err != nil {
		panic(err)
	}
	return value
}

func PublishRubric(ctx context.Context, store reviewport.RubricStoreV1, now time.Time, tenant core.TenantID) contract.RubricDefinitionV1 {
	value := Rubric(now, tenant)
	published, err := store.PublishRubricV1(ctx, reviewport.PublishRubricMutationV1{Next: value})
	if err != nil {
		panic(err)
	}
	return published
}

func Target(now time.Time) contract.TargetSnapshotV1 {
	return TargetWithPolicyExpiry(now, now.Add(time.Hour).UnixNano())
}

func Request(now time.Time, target contract.TargetSnapshotV1, caseID string) contract.ReviewRequestV1 {
	evidence := []runtimeports.ReviewEvidenceRefV2{}
	evidenceDigest, err := contract.ComputeReviewEvidenceDigestV1(evidence)
	if err != nil {
		panic(err)
	}
	value, err := contract.SealReviewRequestV1(contract.ReviewRequestV1{
		FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: target.TenantID, ID: "request-" + caseID, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		IdempotencyKey: "idem-" + caseID, CaseID: caseID, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest,
		Delivery: contract.DeliveryDetachedV1, Profile: contract.ProfileStandardV1, Rubric: Rubric(now, target.TenantID).ExactRef(), RequesterID: "requester-a", RequesterAuthority: Authority("requester-authority"),
		AttachmentEvidence: evidence, AttachmentEvidenceDigest: evidenceDigest, RequestedVerdictTTL: int64(10 * time.Minute), BudgetDigest: Digest("review-budget"), ExpiresUnixNano: now.Add(30 * time.Minute).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return value
}

func ResultBundle(now time.Time, tenant core.TenantID, id string) contract.ReviewResultBundleV1 {
	artifact := contract.ExactResourceRefV1{ID: "artifact-" + id, Revision: 1, Digest: Digest("artifact-" + id)}
	evidence := []runtimeports.ReviewEvidenceRefV2{Evidence("result-" + id)}
	evidenceDigest, err := contract.ComputeReviewEvidenceDigestV1(evidence)
	if err != nil {
		panic(err)
	}
	value, err := contract.SealReviewResultBundleV1(contract.ReviewResultBundleV1{
		FactIdentityV1:     contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: tenant, ID: id, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		OriginalTaskDigest: Digest("task-" + id), AcceptanceDigest: Digest("acceptance-" + id), Artifacts: []contract.ExactResourceRefV1{artifact},
		Claims:            []contract.ResultClaimV1{{ID: "claim-" + id, Statement: "the exact artifact satisfies the declared acceptance", Artifact: artifact, Anchor: "artifact/root", Evidence: evidence}},
		EnvironmentDigest: Digest("environment-" + id), ValidationScopeDigest: Digest("scope-" + id), EvidenceSetDigest: evidenceDigest, ExpiresUnixNano: now.Add(30 * time.Minute).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return value
}

func TargetWithPolicyExpiry(now time.Time, policyExpiresUnixNano int64) contract.TargetSnapshotV1 {
	evidence := []runtimeports.ReviewEvidenceRefV2{Evidence("target")}
	evidenceDigest, _ := contract.ComputeReviewEvidenceDigestV1(evidence)
	expires := now.Add(time.Hour).UnixNano()
	actor, reviewer, scope, currentScope := Authority("actor-authority"), Authority("reviewer-authority"), Scope(), CurrentScope()
	policy := policyFact(now, scope, "run-a", currentScope, actor, reviewer, policyExpiresUnixNano)
	value, err := contract.SealTargetSnapshotV1(contract.TargetSnapshotV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: "tenant-a", ID: "target-a", Revision: 7, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, Kind: contract.TargetActionV1, PayloadSchema: Schema("target"), PayloadDigest: Digest("target-payload"), PayloadRevision: 3, Scope: scope, RunID: "run-a", ActionScopeDigest: Digest("action-scope"), Policy: runtimeports.ReviewPolicyBindingRefV2{Ref: policy.Ref, Revision: policy.Revision, Digest: policy.Digest}, ActorAuthority: actor, CurrentScope: currentScope, Evidence: evidence, EvidenceSetDigest: evidenceDigest, ContextFrameDigest: Digest("context-frame"), ExpiresUnixNano: expires})
	if err != nil {
		panic(err)
	}
	return value
}

func Trace(now time.Time, c contract.ReviewCaseV1, event contract.TraceEventV1, sequence uint64, ref ...string) contract.TraceFactV1 {
	sort.Strings(ref)
	causation := c.ID
	if len(ref) > 0 {
		causation = ref[0]
	}
	value, err := contract.SealTraceFactV1(contract.TraceFactV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: c.TenantID, ID: "trace-" + eventString(event) + "-" + time.Unix(0, int64(sequence)).Format("150405.000000000"), Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, CaseID: c.ID, CaseRevision: c.Revision, TargetID: c.TargetID, TargetRevision: c.TargetRevision, TargetDigest: c.TargetDigest, Event: event, SourceID: "review.test/source", SourceEpoch: 1, SourceSequence: sequence, CausationID: causation, CorrelationID: c.ID, FactRefs: ref})
	if err != nil {
		panic(err)
	}
	return value
}
func StartedTrace(now time.Time, current contract.ReviewCaseV1, assignmentID string) contract.TraceFactV1 {
	successor := current
	successor.Revision++
	return Trace(now, successor, contract.TraceStartedV1, 1_000_000+uint64(current.Revision), assignmentID)
}

// AttestedTrace preserves the Attestation as the causal fact even when the
// canonical provenance set contains IDs that sort before it.
func AttestedTrace(now time.Time, c contract.ReviewCaseV1, sequence uint64, attestationID string, refs ...string) contract.TraceFactV1 {
	refs = append([]string(nil), refs...)
	sort.Strings(refs)
	value, err := contract.SealTraceFactV1(contract.TraceFactV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: c.TenantID, ID: "trace-" + eventString(contract.TraceAttestedV1) + "-" + time.Unix(0, int64(sequence)).Format("150405.000000000"), Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, CaseID: c.ID, CaseRevision: c.Revision, TargetID: c.TargetID, TargetRevision: c.TargetRevision, TargetDigest: c.TargetDigest, Event: contract.TraceAttestedV1, SourceID: "review.test/source", SourceEpoch: 1, SourceSequence: sequence, CausationID: attestationID, CorrelationID: c.ID, FactRefs: refs})
	if err != nil {
		panic(err)
	}
	return value
}
func TraceForTarget(now time.Time, caseID string, target contract.TargetSnapshotV1, event contract.TraceEventV1, sequence uint64, ref ...string) contract.TraceFactV1 {
	c := contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: caseID, Revision: 1}, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest}
	if event == contract.TraceRequestedV1 {
		causation := target.ID
		if len(ref) > 0 {
			causation = ref[0]
		}
		ref = append(ref, caseID, target.ID)
		sort.Strings(ref)
		unique := ref[:0]
		for _, value := range ref {
			if len(unique) == 0 || unique[len(unique)-1] != value {
				unique = append(unique, value)
			}
		}
		value, err := contract.SealTraceFactV1(contract.TraceFactV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: target.TenantID, ID: "trace-" + eventString(event) + "-" + time.Unix(0, int64(sequence)).Format("150405.000000000"), Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, CaseID: caseID, CaseRevision: 1, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, Event: event, SourceID: "review.test/request", SourceEpoch: 1, SourceSequence: sequence, CausationID: causation, CorrelationID: caseID, FactRefs: unique})
		if err != nil {
			panic(err)
		}
		return value
	}
	return Trace(now, c, event, sequence, ref...)
}
func TransitionTrace(now time.Time, current contract.ReviewCaseV1, next contract.CaseStateV1) contract.TraceFactV1 {
	event := contract.TraceEventV1("")
	switch next {
	case contract.CaseAdmittedV1:
		event = contract.TraceAdmittedV1
	case contract.CaseRoutedV1:
		event = contract.TraceRoutedV1
	default:
		panic("testkit.TransitionTrace only supports admitted/routed")
	}
	successor := current
	successor.Revision++
	refs := []string{successor.ID}
	value, err := contract.SealTraceFactV1(contract.TraceFactV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: successor.TenantID, ID: "trace-" + eventString(event) + "-" + time.Unix(0, int64(successor.Revision)).Format("150405.000000000"), Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, CaseID: successor.ID, CaseRevision: successor.Revision, TargetID: successor.TargetID, TargetRevision: successor.TargetRevision, TargetDigest: successor.TargetDigest, Event: event, SourceID: "review.test/case-transition", SourceEpoch: 1, SourceSequence: uint64(successor.Revision), CausationID: successor.ID, CorrelationID: successor.ID, FactRefs: refs})
	if err != nil {
		panic(err)
	}
	return value
}
func CaseSuccessor(now time.Time, current contract.ReviewCaseV1, next contract.CaseStateV1) contract.ReviewCaseV1 {
	value := current
	value.Revision++
	value.State = next
	value.UpdatedUnixNano = now.UnixNano()
	value.Digest = ""
	sealed, err := contract.SealReviewCaseV1(value)
	if err != nil {
		panic(err)
	}
	return sealed
}
func eventString(e contract.TraceEventV1) string { return string(e) }

func Round(now time.Time, c contract.ReviewCaseV1, route contract.RouteV1) contract.ReviewRoundV1 {
	rubric := c.Rubric
	if rubric == nil {
		exact := Rubric(now, c.TenantID).ExactRef()
		rubric = &exact
	}
	value, err := contract.SealReviewRoundV1(contract.ReviewRoundV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: c.TenantID, ID: "round-a", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, CaseID: c.ID, CaseRevision: c.Revision, TargetID: c.TargetID, TargetRevision: c.TargetRevision, TargetDigest: c.TargetDigest, Route: route, State: contract.RoundPreparedV1, AssignmentID: "assignment-a", ContextFrameDigest: Digest("context-frame"), Rubric: rubric, RubricDigest: rubric.Digest, ExpiresUnixNano: now.Add(20 * time.Minute).UnixNano()})
	if err != nil {
		panic(err)
	}
	return value
}
func Assignment(now time.Time, c contract.ReviewCaseV1, round contract.ReviewRoundV1, route contract.RouteV1) contract.ReviewerAssignmentV1 {
	reviewerID := "reviewer-a"
	if route == contract.RouteAutoV1 {
		reviewerID = "auto-reviewer-a"
	}
	value, err := contract.SealReviewerAssignmentV1(contract.ReviewerAssignmentV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: c.TenantID, ID: "assignment-a", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, CaseID: c.ID, CaseRevision: c.Revision, RoundID: round.ID, RoundRevision: round.Revision, RoundDigest: round.Digest, TargetID: c.TargetID, TargetRevision: c.TargetRevision, TargetDigest: c.TargetDigest, Route: route, ReviewerID: reviewerID, ReviewerAuthority: Authority("reviewer-authority"), ReviewerBinding: ReviewerBinding(), Capability: "review.test/attest", State: contract.AssignmentOfferedV1, ExpiresUnixNano: now.Add(20 * time.Minute).UnixNano()})
	if err != nil {
		panic(err)
	}
	return value
}

func HumanAttestation(now time.Time, c contract.ReviewCaseV1, round contract.ReviewRoundV1, assignment contract.ReviewerAssignmentV1, resolution contract.ResolutionV1, idem string) contract.AttestationV1 {
	ev := []runtimeports.ReviewEvidenceRefV2{Evidence("attestation")}
	evidenceDigest, _ := contract.ComputeReviewEvidenceDigestV1(ev)
	var conditions []runtimeports.ReviewConditionV2
	conditionsDigest := core.Digest("")
	if resolution == contract.ResolutionConditionalV1 {
		conditions = []runtimeports.ReviewConditionV2{{ID: "review.test/followup", Revision: 1, Schema: Schema("review-condition"), ConstraintDigest: Digest("condition-constraint"), SatisfactionOwner: ReviewerBinding(), ScopeDigest: Digest("action-scope"), Authority: Authority("actor-authority"), ExpiresUnixNano: now.Add(10 * time.Minute).UnixNano()}}
		conditionsDigest, _ = runtimeports.DigestReviewConditionsV2(conditions)
	}
	value, err := contract.SealAttestationV1(contract.AttestationV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: c.TenantID, ID: "attestation-" + idem, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, IdempotencyKey: idem, CaseID: c.ID, CaseRevision: c.Revision, RoundID: round.ID, RoundRevision: round.Revision, RoundDigest: round.Digest, AssignmentID: assignment.ID, AssignmentRevision: assignment.Revision, AssignmentDigest: assignment.Digest, TargetID: c.TargetID, TargetRevision: c.TargetRevision, TargetDigest: c.TargetDigest, ContextFrameDigest: Digest("context-frame"), Route: contract.RouteHumanV1, ReviewerID: assignment.ReviewerID, ReviewerAuthority: assignment.ReviewerAuthority, ReviewerBinding: assignment.ReviewerBinding, Resolution: resolution, ReasonCodes: []string{"review.test/checked"}, Evidence: ev, EvidenceDigest: evidenceDigest, Conditions: conditions, ConditionsDigest: conditionsDigest, ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Minute).UnixNano()})
	if err != nil {
		panic(err)
	}
	return value
}

func DomainResult(now time.Time, c contract.ReviewCaseV1, round contract.ReviewRoundV1, assignment contract.ReviewerAssignmentV1) contract.ReviewerInvocationResultFactV1 {
	value, err := contract.SealReviewerInvocationResultFactV1(contract.ReviewerInvocationResultFactV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: c.TenantID, ID: "domain-result-a", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, CaseID: c.ID, CaseRevision: c.Revision, RoundID: round.ID, RoundRevision: round.Revision, RoundDigest: round.Digest, AssignmentID: assignment.ID, AssignmentRevision: assignment.Revision, AssignmentDigest: assignment.Digest, TargetID: c.TargetID, TargetRevision: c.TargetRevision, TargetDigest: c.TargetDigest, AttemptID: "attempt-a", ResultSchema: Schema("reviewer-result"), ResultDigest: Digest("reviewer-result-payload"), ObservationRefs: []string{"observation-a"}})
	if err != nil {
		panic(err)
	}
	return value
}

func RuntimeSettlement(result contract.ReviewerInvocationResultFactV1) runtimeports.OperationSettlementRefV3 {
	return runtimeports.OperationSettlementRefV3{ID: "operation-settlement-a", Revision: 1, Digest: Digest("operation-settlement-a"), Attempt: runtimeports.OperationDispatchAttemptRefV3{OperationDigest: Digest("operation"), EffectID: "effect-a", IntentRevision: 1, IntentDigest: Digest("intent"), PermitID: "permit-a", PermitRevision: 1, PermitDigest: Digest("permit"), AttemptID: "attempt-a"}, Disposition: runtimeports.OperationSettlementAppliedV3, Owner: runtimeports.EffectOwnerRefV2{Role: runtimeports.OwnerSettlement, ComponentID: "review.test/settlement", ManifestDigest: Digest("settlement-manifest")}, Evidence: []runtimeports.EvidenceRecordRefV2{{LedgerScopeDigest: Digest("ledger"), Sequence: 1, RecordDigest: Digest("record")}}, DomainResultSchema: &result.ResultSchema, DomainResultDigest: result.Digest}
}

func AutoAttestation(now time.Time, c contract.ReviewCaseV1, round contract.ReviewRoundV1, assignment contract.ReviewerAssignmentV1, result contract.ReviewerInvocationResultFactV1, apply contract.DomainApplySettlementFactV1) contract.AttestationV1 {
	ev := []runtimeports.ReviewEvidenceRefV2{Evidence("auto")}
	evidenceDigest, _ := contract.ComputeReviewEvidenceDigestV1(ev)
	ref := apply.Ref()
	value, err := contract.SealAttestationV1(contract.AttestationV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: c.TenantID, ID: "attestation-auto", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, IdempotencyKey: "idem-auto", CaseID: c.ID, CaseRevision: c.Revision, RoundID: round.ID, RoundRevision: round.Revision, RoundDigest: round.Digest, AssignmentID: assignment.ID, AssignmentRevision: assignment.Revision, AssignmentDigest: assignment.Digest, TargetID: c.TargetID, TargetRevision: c.TargetRevision, TargetDigest: c.TargetDigest, ContextFrameDigest: Digest("context-frame"), Route: contract.RouteAutoV1, ReviewerID: assignment.ReviewerID, ReviewerAuthority: assignment.ReviewerAuthority, ReviewerBinding: assignment.ReviewerBinding, Resolution: contract.ResolutionAcceptV1, ReasonCodes: []string{"review.test/auto-checked"}, Evidence: ev, EvidenceDigest: evidenceDigest, DomainApplySettlement: &ref, ReviewerAttemptID: result.AttemptID, ReviewerResultDigest: result.ResultDigest, ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Minute).UnixNano()})
	if err != nil {
		panic(err)
	}
	return value
}

type ExternalCurrentReader struct {
	Mutate func(*reviewport.DecisionExternalCurrentProjectionV1)
}

func (r *ExternalCurrentReader) InspectDecisionExternalCurrentV1(_ context.Context, request reviewport.DecisionExternalCurrentRequestV1) (reviewport.DecisionExternalCurrentProjectionV1, error) {
	expires := request.Attestation.ExpiresUnixNano
	if request.Assignment.LeaseExpiresUnixNano < expires {
		expires = request.Assignment.LeaseExpiresUnixNano
	}
	policy := policyFact(time.Unix(0, request.Target.CreatedUnixNano), request.Target.Scope, request.Target.RunID, request.Target.CurrentScope, request.Target.ActorAuthority, request.Assignment.ReviewerAuthority, request.Target.ExpiresUnixNano)
	projection := reviewport.DecisionExternalCurrentProjectionV1{Policy: policy, ActorAuthority: runtimeports.OperationGovernanceFactRefV3{Ref: request.Target.ActorAuthority.Ref, Revision: request.Target.ActorAuthority.Revision, Digest: request.Target.ActorAuthority.Digest, ExpiresUnixNano: expires}, ReviewerAuthority: runtimeports.OperationGovernanceFactRefV3{Ref: request.Assignment.ReviewerAuthority.Ref, Revision: request.Assignment.ReviewerAuthority.Revision, Digest: request.Assignment.ReviewerAuthority.Digest, ExpiresUnixNano: expires}, Scope: runtimeports.OperationGovernanceFactRefV3{Ref: request.Target.CurrentScope.Ref, Revision: request.Target.CurrentScope.Revision, Digest: request.Target.CurrentScope.Digest, ExpiresUnixNano: expires}, Binding: contract.ReviewerBindingCurrentV1{Binding: request.Assignment.ReviewerBinding, Current: true, ExpiresUnixNano: expires}, Current: true, ExpiresUnixNano: expires}
	for _, evidence := range request.Evidence {
		owner := runtimeports.EvidenceProducerBindingRefV2{BindingSetID: request.Assignment.ReviewerBinding.BindingSetID, BindingSetRevision: request.Assignment.ReviewerBinding.BindingSetRevision, ComponentID: request.Assignment.ReviewerBinding.ComponentID, ManifestDigest: request.Assignment.ReviewerBinding.ManifestDigest, ArtifactDigest: request.Assignment.ReviewerBinding.ArtifactDigest, Capability: request.Assignment.ReviewerBinding.Capability}
		projection.Evidence = append(projection.Evidence, contract.DecisionEvidenceCurrentV1{Review: evidence, OwnerFact: runtimeports.EvidenceOwnerFactRefV2{Owner: owner, FactKind: evidence.Classification, FactID: evidence.Ref, Revision: 1, FactDigest: evidence.Digest, PayloadSchema: Schema("evidence"), PayloadDigest: evidence.Digest, PayloadRevision: 1}, Current: true, ExpiresUnixNano: expires})
	}
	if r != nil && r.Mutate != nil {
		r.Mutate(&projection)
	}
	return projection, nil
}
