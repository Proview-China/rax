package conformance_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestConditionV2ConformanceHumanCanonicalUnion(t *testing.T) {
	now := time.Unix(1_980_000_000, 0)
	first := conformanceHumanAttestation(t, now, "a", "review/a")
	second := conformanceHumanAttestation(t, now, "b", "review/b")
	conditions, digest, err := contract.CanonicalAcceptedConditionsV2([]contract.HumanAttestationV2{first, second}, []contract.HumanAttestationExactRefV2{first.ExactRef(), second.ExactRef()})
	if err != nil || len(conditions) != 2 {
		t.Fatalf("canonical union failed: %v", err)
	}
	q := conformanceHumanQuorum(t, now, first, second, conditions, digest)
	v := conformanceHumanVerdict(t, now, q, conditions, digest)
	if err := conformance.CheckConditionChainV2(conformance.ConditionChainFixtureV2{HumanVotes: []contract.HumanAttestationV2{first, second}, HumanQuorum: &q, HumanVerdict: &v}); err != nil {
		t.Fatal(err)
	}
	drifted := v.Clone()
	drifted.Conditions[0].ConstraintDigest = core.DigestBytes([]byte("drift"))
	drifted.Digest = ""
	if sealed, e := contract.SealHumanVerdictV2(drifted); e == nil {
		if e = conformance.CheckConditionChainV2(conformance.ConditionChainFixtureV2{HumanVotes: []contract.HumanAttestationV2{first, second}, HumanQuorum: &q, HumanVerdict: &sealed}); !core.HasReason(e, core.ReasonReviewConditionUnsatisfied) {
			t.Fatalf("condition drift was accepted: %v", e)
		}
	}
	sameA := conformanceHumanAttestation(t, now, "same-a", "review/shared")
	sameB := conformanceHumanAttestation(t, now, "same-b", "review/shared")
	if union, _, e := contract.CanonicalAcceptedConditionsV2([]contract.HumanAttestationV2{sameA, sameB}, []contract.HumanAttestationExactRefV2{sameA.ExactRef(), sameB.ExactRef()}); e != nil || len(union) != 1 {
		t.Fatalf("identical shared condition did not deduplicate canonically: %+v err=%v", union, e)
	}
	driftVote := sameB.Clone()
	driftVote.Conditions[0].ConstraintDigest = core.DigestBytes([]byte("same-id-different-constraint"))
	driftVote.ConditionsDigest, _ = runtimeports.DigestReviewConditionsV2(driftVote.Conditions)
	driftVote.Digest = ""
	driftVote, err = contract.SealHumanAttestationV2(driftVote)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, e := contract.CanonicalAcceptedConditionsV2([]contract.HumanAttestationV2{sameA, driftVote}, []contract.HumanAttestationExactRefV2{sameA.ExactRef(), driftVote.ExactRef()}); !core.HasReason(e, core.ReasonReviewConditionUnsatisfied) {
		t.Fatalf("same condition ID field drift was accepted: %v", e)
	}
}

func conformanceCondition(now time.Time, id string) runtimeports.ReviewConditionV2 {
	d := func(v string) core.Digest { return core.DigestBytes([]byte(v)) }
	return runtimeports.ReviewConditionV2{ID: runtimeports.NamespacedNameV2(id), Revision: 1, Schema: runtimeports.SchemaRefV2{Namespace: "review", Name: "condition", Version: "1.0.0", MediaType: "application/json", ContentDigest: d("schema")}, ConstraintDigest: d("constraint-" + id), SatisfactionOwner: runtimeports.ReviewComponentBindingRefV2{BindingSetID: "set", BindingSetRevision: 1, ComponentID: "review/owner", ManifestDigest: d("manifest"), ArtifactDigest: d("artifact"), Capability: "review/satisfy"}, ScopeDigest: d("scope"), Authority: runtimeports.AuthorityBindingRefV2{Ref: "authority", Revision: 1, Digest: d("authority"), Epoch: 1}, ExpiresUnixNano: now.Add(5 * time.Minute).UnixNano()}
}

func conformanceHumanAttestation(t *testing.T, now time.Time, suffix, conditionID string) contract.HumanAttestationV2 {
	t.Helper()
	d := func(v string) core.Digest { return core.DigestBytes([]byte(v)) }
	condition := conformanceCondition(now, conditionID)
	conditions := []runtimeports.ReviewConditionV2{condition}
	cd, _ := runtimeports.DigestReviewConditionsV2(conditions)
	evidence := []runtimeports.ReviewEvidenceRefV2{{Ref: "evidence-" + suffix, Classification: "review/evidence", Digest: d("evidence-" + suffix)}}
	ed, _ := contract.ComputeReviewEvidenceDigestV1(evidence)
	ref := func(id string) contract.FactIdentityV1 {
		return contract.FactIdentityV1{TenantID: "tenant", ID: id, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}
	}
	identity := contract.HumanIdentityProofRefV2{TenantID: "tenant", Ref: "identity-" + suffix, Revision: 1, Digest: d("identity-" + suffix)}
	value := contract.HumanAttestationV2{FactIdentityV1: ref("att-" + suffix), IdempotencyKey: "idem-" + suffix, Panel: contract.HumanPanelExactRefV2{TenantID: "tenant", ID: "panel", Revision: 1, Digest: d("panel")}, Assignment: contract.HumanPanelAssignmentExactRefV2{TenantID: "tenant", ID: "assignment-" + suffix, Revision: 1, Digest: d("assignment-" + suffix)}, Case: contract.HumanCaseExactRefV2{TenantID: "tenant", ID: "case", Revision: 1, Digest: d("case")}, Round: contract.HumanRoundExactRefV2{TenantID: "tenant", ID: "round", Revision: 1, Digest: d("round")}, Target: contract.HumanTargetExactRefV2{TenantID: "tenant", ID: "target", Revision: 1, Digest: d("target")}, Policy: contract.HumanQuorumPolicyBindingV2{TenantID: "tenant", Ref: "policy", Revision: 1, Digest: d("policy"), Domain: "review", CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Minute).UnixNano()}, ResponsibilitySubject: contract.HumanResponsibilitySubjectRefV2{TenantID: "tenant", Ref: "responsibility", Revision: 1, Digest: d("responsibility"), IdentityProof: contract.HumanIdentityProofRefV2{TenantID: "tenant", Ref: "owner", Revision: 1, Digest: d("owner")}}, ReviewerIdentity: identity, ReviewerAuthority: condition.Authority, ReviewerBinding: condition.SatisfactionOwner, Resolution: contract.ResolutionConditionalV1, ReasonCodes: []string{"review/conditional"}, Evidence: evidence, EvidenceDigest: ed, Conditions: conditions, ConditionsDigest: cd, ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(4 * time.Minute).UnixNano()}
	sealed, err := contract.SealHumanAttestationV2(value)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func conformanceHumanQuorum(t *testing.T, now time.Time, first, second contract.HumanAttestationV2, conditions []runtimeports.ReviewConditionV2, digest core.Digest) contract.HumanQuorumDecisionV2 {
	t.Helper()
	d := func(v string) core.Digest { return core.DigestBytes([]byte(v)) }
	refs := []contract.HumanAttestationExactRefV2{first.ExactRef(), second.ExactRef()}
	ids := []contract.HumanIdentityProofRefV2{first.ReviewerIdentity, second.ReviewerIdentity}
	rs, _ := contract.ComputeHumanReviewerSetDigestV2(ids)
	evidence := append(append([]runtimeports.ReviewEvidenceRefV2{}, first.Evidence...), second.Evidence...)
	ed, _ := contract.ComputeReviewEvidenceDigestV1(evidence)
	q, err := contract.SealHumanQuorumDecisionV2(contract.HumanQuorumDecisionV2{FactIdentityV1: contract.FactIdentityV1{TenantID: "tenant", ID: "quorum", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, Panel: first.Panel, Policy: first.Policy, Resolution: contract.ResolutionConditionalV1, AcceptedAttestationRefs: refs, DistinctReviewerIdentityRefs: ids, AcceptCount: 2, Threshold: 2, SatisfiedRoleCounts: []contract.HumanSatisfiedRoleCountV2{{Role: "reviewer", DistinctCurrentCount: 2}}, Conditions: conditions, ConditionsDigest: digest, EvidenceSetDigest: ed, ReviewerSetDigest: rs, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(3 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	_ = d
	return q
}

func conformanceHumanVerdict(t *testing.T, now time.Time, q contract.HumanQuorumDecisionV2, conditions []runtimeports.ReviewConditionV2, digest core.Digest) contract.HumanVerdictV2 {
	t.Helper()
	d := func(v string) core.Digest { return core.DigestBytes([]byte(v)) }
	first := q.AcceptedAttestationRefs[0]
	v, err := contract.SealHumanVerdictV2(contract.HumanVerdictV2{FactIdentityV1: contract.FactIdentityV1{TenantID: "tenant", ID: "verdict", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, Case: contract.HumanCaseExactRefV2{TenantID: "tenant", ID: "case", Revision: 1, Digest: d("case")}, Target: contract.HumanTargetExactRefV2{TenantID: "tenant", ID: "target", Revision: 1, Digest: d("target")}, Round: contract.HumanRoundExactRefV2{TenantID: "tenant", ID: "round", Revision: 1, Digest: d("round")}, Panel: q.Panel, QuorumDecision: q.ExactRef(), Policy: q.Policy, Scope: testkit.Scope(), CurrentScope: runtimeports.ExecutionScopeBindingRefV2{Ref: "scope", Revision: 1, Digest: d("scope")}, ReviewerSetDigest: q.ReviewerSetDigest, ReviewerAuthorityRefs: []runtimeports.AuthorityBindingRefV2{{Ref: "authority", Revision: 1, Digest: d("authority"), Epoch: 1}}, BindingClosures: []runtimeports.ReviewComponentBindingRefV2{{BindingSetID: "set", BindingSetRevision: 1, ComponentID: "review/owner", ManifestDigest: d("manifest"), ArtifactDigest: d("artifact"), Capability: "review/satisfy"}}, AttestationRefs: append([]contract.HumanAttestationExactRefV2(nil), q.AcceptedAttestationRefs...), Evidence: []runtimeports.ReviewEvidenceRefV2{{Ref: "evidence-a", Classification: "review/evidence", Digest: d("evidence-a")}, {Ref: "evidence-b", Classification: "review/evidence", Digest: d("evidence-b")}}, EvidenceSetDigest: q.EvidenceSetDigest, Conditions: conditions, ConditionsDigest: digest, ReasonCodes: []string{"review/conditional"}, State: contract.HumanVerdictConditionalV2, ExpiresUnixNano: now.Add(2 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	_ = first
	return v
}
