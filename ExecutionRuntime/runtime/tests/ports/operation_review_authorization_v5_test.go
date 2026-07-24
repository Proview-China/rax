package ports_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationReviewAuthorizationV5UnionIsNominalAndExclusive(t *testing.T) {
	now := time.Unix(410_000, 0)
	projection := operationReviewUnionShapeV5(t, now)
	if err := projection.Validate(now); err != nil {
		t.Fatal(err)
	}
	changed := projection
	changed.Basis = ports.OperationReviewBasisPolicyNotRequiredV5
	if err := changed.Validate(now); !core.HasReason(err, core.ReasonReviewCandidateConflict) {
		t.Fatalf("basis type-pun did not fail closed: %v", err)
	}
	changed = projection
	other := *projection.Quorum
	changed.PolicyNotRequired = &ports.OperationReviewPolicyNotRequiredCurrentProjectionV5{Operation: other.Operation}
	if err := changed.Validate(now); !core.HasReason(err, core.ReasonReviewCandidateConflict) {
		t.Fatalf("two active union branches did not fail closed: %v", err)
	}
}

func TestOperationReviewAuthorizationV5ExactRefsRequireTenantTTLAndDigest(t *testing.T) {
	now := time.Unix(410_000, 0)
	ref := ports.OperationReviewCaseRefV5{TenantID: "tenant", ID: "case", Revision: 1, Digest: core.DigestBytes([]byte("case")), ExpiresUnixNano: now.Add(time.Second).UnixNano()}
	if err := ref.Validate(now); err != nil {
		t.Fatal(err)
	}
	ref.TenantID = ""
	if err := ref.Validate(now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("tenant-less exact ref accepted: %v", err)
	}
}

func TestOperationReviewAuthorizationV5QuorumHardNegatives(t *testing.T) {
	now := time.Unix(410_000, 0)
	base := operationReviewUnionShapeV5(t, now)
	tests := []struct {
		name   string
		change func(*ports.OperationReviewQuorumCurrentProjectionV5)
	}{
		{"role-shortfall", func(q *ports.OperationReviewQuorumCurrentProjectionV5) { q.SatisfiedRoleCounts[0].Count = 0 }},
		{"duplicate-authority", func(q *ports.OperationReviewQuorumCurrentProjectionV5) {
			q.ReviewerAuthorityRefs[1].Ref = q.ReviewerAuthorityRefs[0].Ref
			q.ReviewerAuthorityRefs[1].Revision++
			q.ReviewerAuthorityRefs[1].Digest = core.DigestBytes([]byte("same-ref-different-content"))
		}},
		{"cross-tenant", func(q *ports.OperationReviewQuorumCurrentProjectionV5) { q.Panel.TenantID = "other-tenant" }},
		{"ttl-not-minimum", func(q *ports.OperationReviewQuorumCurrentProjectionV5) {
			q.Case.ExpiresUnixNano = now.Add(30 * time.Second).UnixNano()
		}},
		{"evidence-coordinate-conflict", func(q *ports.OperationReviewQuorumCurrentProjectionV5) {
			conflict := q.DecisionEvidence[0]
			conflict.RecordDigest = core.DigestBytes([]byte("different-record"))
			q.DecisionEvidence = append(q.DecisionEvidence, conflict)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			q := *base.Quorum
			q.SatisfiedRoleCounts = append([]ports.OperationReviewRoleCountV5{}, q.SatisfiedRoleCounts...)
			q.ReviewerAuthorityRefs = append([]ports.OperationGovernanceFactRefV3{}, q.ReviewerAuthorityRefs...)
			test.change(&q)
			if _, err := ports.SealOperationReviewQuorumCurrentProjectionV5(q, now); err == nil {
				t.Fatal("invalid quorum projection was sealed")
			}
		})
	}
}

func operationReviewUnionShapeV5(t *testing.T, now time.Time) ports.OperationReviewCurrentProjectionV5 {
	t.Helper()
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant", ID: "identity", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: core.DigestBytes([]byte("lineage"))}, Instance: core.InstanceRef{ID: "instance", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, _ := ports.ExecutionScopeDigestV2(scope)
	operation := ports.OperationSubjectV3{Kind: ports.OperationScopeActivationV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, ActivationAttemptID: "attempt", SubjectRevision: 1, CurrentProjectionRef: "scope", CurrentProjectionRevision: 1, CurrentProjectionDigest: core.DigestBytes([]byte("scope"))}
	expires := now.Add(time.Minute).UnixNano()
	ref := func(id string) ports.OperationGovernanceFactRefV3 {
		return ports.OperationGovernanceFactRefV3{Ref: id, Revision: 1, Digest: core.DigestBytes([]byte(id)), ExpiresUnixNano: expires}
	}
	evidence := []ports.EvidenceRecordRefV2{{LedgerScopeDigest: core.DigestBytes([]byte("ledger")), Sequence: 1, RecordDigest: core.DigestBytes([]byte("evidence"))}}
	q, err := ports.SealOperationReviewQuorumCurrentProjectionV5(ports.OperationReviewQuorumCurrentProjectionV5{
		Operation: operation, IntentID: "effect", IntentRevision: 1, IntentDigest: core.DigestBytes([]byte("intent")),
		PayloadSchema: ports.SchemaRefV2{Namespace: "custom", Name: "payload", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("schema"))}, PayloadDigest: core.DigestBytes([]byte("payload")), PayloadRevision: 1,
		Target:         ports.OperationReviewTargetRefV4{Ref: "target", Revision: 1, Digest: core.DigestBytes([]byte("target"))},
		Case:           ports.OperationReviewCaseRefV5{TenantID: "tenant", ID: "case", Revision: 1, Digest: core.DigestBytes([]byte("case")), ExpiresUnixNano: expires},
		Panel:          ports.OperationReviewPanelRefV5{TenantID: "tenant", ID: "panel", Revision: 1, Digest: core.DigestBytes([]byte("panel")), ExpiresUnixNano: expires},
		QuorumDecision: ports.OperationReviewQuorumDecisionRefV5{TenantID: "tenant", ID: "quorum", Revision: 1, Digest: core.DigestBytes([]byte("quorum")), ExpiresUnixNano: expires},
		Verdict:        ports.OperationReviewVerdictRefV5{TenantID: "tenant", ID: "verdict", Revision: 1, Digest: core.DigestBytes([]byte("verdict")), ExpiresUnixNano: expires},
		QuorumPolicy:   ref("policy"), ReviewerSetDigest: core.DigestBytes([]byte("reviewers")), AcceptCount: 2, Threshold: 2,
		SatisfiedRoleCounts:   []ports.OperationReviewRoleCountV5{{Role: "security", Count: 1, Required: 1}},
		ReviewerAuthorityRefs: []ports.OperationGovernanceFactRefV3{ref("authority-a"), ref("authority-b")}, BindingRefs: []ports.OperationGovernanceFactRefV3{ref("binding-a")}, ScopeRef: ref("scope"),
		DecisionEvidence: evidence, Basis: ports.OperationReviewBasisAcceptedQuorumV5, Current: true, CurrentnessDigest: core.DigestBytes([]byte("current")), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ports.SealOperationReviewCurrentProjectionV5(ports.OperationReviewCurrentProjectionV5{Basis: ports.OperationReviewBasisAcceptedQuorumV5, Quorum: &q}, now)
	if err != nil {
		t.Fatal(err)
	}
	return projection
}
