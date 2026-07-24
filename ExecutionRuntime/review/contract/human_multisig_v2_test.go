package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const humanTenantV2 core.TenantID = "tenant-a"

func hv2Digest(value string) core.Digest { return core.DigestBytes([]byte(value)) }

func hv2FactRef(id string) contract.HumanCaseExactRefV2 {
	return contract.HumanCaseExactRefV2{TenantID: humanTenantV2, ID: id, Revision: 1, Digest: hv2Digest(id)}
}

func hv2TargetRef(id string) contract.HumanTargetExactRefV2 {
	return contract.HumanTargetExactRefV2{TenantID: humanTenantV2, ID: id, Revision: 1, Digest: hv2Digest(id)}
}

func hv2RoundRef(id string) contract.HumanRoundExactRefV2 {
	return contract.HumanRoundExactRefV2{TenantID: humanTenantV2, ID: id, Revision: 1, Digest: hv2Digest(id)}
}

func hv2Identity(id string) contract.HumanIdentityProofRefV2 {
	return contract.HumanIdentityProofRefV2{TenantID: humanTenantV2, Ref: id, Revision: 1, Digest: hv2Digest(id)}
}

func hv2Policy() contract.HumanQuorumPolicyBindingV2 {
	return contract.HumanQuorumPolicyBindingV2{TenantID: humanTenantV2, Ref: "policy-a", Revision: 1, Digest: hv2Digest("policy"), Domain: "tenant-review", CheckedUnixNano: time.Unix(1_800_000_000, 0).Add(-time.Minute).UnixNano(), ExpiresUnixNano: time.Unix(1_800_000_000, 0).Add(2 * time.Hour).UnixNano()}
}

func hv2Authority(id string) runtimeports.AuthorityBindingRefV2 {
	return runtimeports.AuthorityBindingRefV2{Ref: id, Revision: 1, Digest: hv2Digest(id), Epoch: 1}
}

func hv2Binding(id string) runtimeports.ReviewComponentBindingRefV2 {
	return runtimeports.ReviewComponentBindingRefV2{BindingSetID: id, BindingSetRevision: 1, ComponentID: "review/human", ManifestDigest: hv2Digest(id + "-manifest"), ArtifactDigest: hv2Digest(id + "-artifact"), Capability: "review/attest"}
}

func hv2Evidence(id string) runtimeports.ReviewEvidenceRefV2 {
	return runtimeports.ReviewEvidenceRefV2{Ref: id, Classification: "review/evidence", Digest: hv2Digest(id)}
}

func hv2Panel(t *testing.T, now time.Time) contract.HumanReviewPanelV2 {
	t.Helper()
	responsibility := contract.HumanResponsibilitySubjectRefV2{TenantID: humanTenantV2, Ref: "responsibility-a", Revision: 1, Digest: hv2Digest("responsibility"), IdentityProof: hv2Identity("author-a")}
	assignments := []contract.HumanPanelAssignmentExactRefV2{
		{TenantID: humanTenantV2, ID: "assignment-reviewer-a", Revision: 1, Digest: hv2Digest("assignment-reviewer-a")},
		{TenantID: humanTenantV2, ID: "assignment-reviewer-b", Revision: 1, Digest: hv2Digest("assignment-reviewer-b")},
		{TenantID: humanTenantV2, ID: "assignment-reviewer-c", Revision: 1, Digest: hv2Digest("assignment-reviewer-c")},
	}
	panel, err := contract.SealHumanReviewPanelV2(contract.HumanReviewPanelV2{
		FactIdentityV1: contract.FactIdentityV1{TenantID: humanTenantV2, Revision: 2, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		Case:           hv2FactRef("case-a"), Target: hv2TargetRef("target-a"), Round: hv2RoundRef("round-a"), QuorumPolicy: hv2Policy(), ResponsibilitySubject: responsibility,
		State: contract.HumanPanelOpenV2, AssignmentRefs: assignments, AcceptThreshold: 2, MaximumPanelSize: 3,
		RoleRequirements: []contract.HumanRoleRequirementV2{{Role: "security", Minimum: 1}, {Role: "technical", Minimum: 1}}, RejectVetoRoles: []string{"security"},
		DelegationRequired: true, ProductionSelfReviewAllowed: false, MaxPanelDurationNanos: int64(time.Hour), MaxVoteTTLNanos: int64(10 * time.Minute), ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return panel
}

func hv2Assignment(t *testing.T, now time.Time, panel contract.HumanReviewPanelV2, reviewer string) contract.HumanPanelAssignmentV2 {
	t.Helper()
	delegate := hv2Identity(reviewer)
	assignment, err := contract.SealHumanPanelAssignmentV2(contract.HumanPanelAssignmentV2{
		FactIdentityV1: contract.FactIdentityV1{TenantID: humanTenantV2, ID: "assignment-" + reviewer, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		Panel:          panel.ExactRef(), Case: panel.Case, Round: panel.Round, Target: panel.Target,
		ReviewerIdentity: delegate, ReviewerAuthority: hv2Authority("authority-" + reviewer), ReviewerBinding: hv2Binding("binding-" + reviewer), Roles: []string{"security", "technical"}, CanVeto: true,
		Delegated: true, DelegatorIdentity: hv2Identity("manager-a"), DelegateIdentity: delegate, DelegationFact: contract.HumanDelegationFactRefV2{TenantID: humanTenantV2, Ref: "delegation-" + reviewer, Revision: 1, Digest: hv2Digest("delegation-" + reviewer)}, DelegatedRole: "security", DelegationScopeDigest: hv2Digest("scope"),
		State: contract.HumanAssignmentClaimedV2, LeaseHolder: reviewer, LeaseExpiresUnixNano: now.Add(5 * time.Minute).UnixNano(), ExpiresUnixNano: now.Add(10 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return assignment
}

func hv2Attestation(t *testing.T, now time.Time, panel contract.HumanReviewPanelV2, assignment contract.HumanPanelAssignmentV2, resolution contract.ResolutionV1) contract.HumanAttestationV2 {
	t.Helper()
	evidence := []runtimeports.ReviewEvidenceRefV2{hv2Evidence("evidence-a")}
	evidenceDigest, err := contract.ComputeReviewEvidenceDigestV1(evidence)
	if err != nil {
		t.Fatal(err)
	}
	value := contract.HumanAttestationV2{
		FactIdentityV1: contract.FactIdentityV1{TenantID: humanTenantV2, ID: "attestation-" + assignment.ID, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, IdempotencyKey: "idem-" + assignment.ID,
		Panel: panel.ExactRef(), Assignment: assignment.ExactRef(), Case: panel.Case, Round: panel.Round, Target: panel.Target, Policy: panel.QuorumPolicy, ResponsibilitySubject: panel.ResponsibilitySubject,
		ReviewerIdentity: assignment.ReviewerIdentity, ReviewerAuthority: assignment.ReviewerAuthority, Delegation: &assignment.DelegationFact, ReviewerBinding: assignment.ReviewerBinding,
		Resolution: resolution, ReasonCodes: []string{"review/verified"}, Evidence: evidence, EvidenceDigest: evidenceDigest, ObservedUnixNano: now.Add(time.Second).UnixNano(), ExpiresUnixNano: now.Add(9 * time.Minute).UnixNano(),
	}
	if resolution == contract.ResolutionConditionalV1 {
		value.Conditions = []runtimeports.ReviewConditionV2{hv2Condition(now)}
		value.ConditionsDigest, err = runtimeports.DigestReviewConditionsV2(value.Conditions)
		if err != nil {
			t.Fatal(err)
		}
	}
	sealed, err := contract.SealHumanAttestationV2(value)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func hv2Condition(now time.Time) runtimeports.ReviewConditionV2 {
	return runtimeports.ReviewConditionV2{ID: "review/followup", Revision: 1, Schema: runtimeports.SchemaRefV2{Namespace: "review", Name: "condition", Version: "1.0.0", MediaType: "application/json", ContentDigest: hv2Digest("condition-schema")}, ConstraintDigest: hv2Digest("constraint"), SatisfactionOwner: hv2Binding("condition-owner"), ScopeDigest: hv2Digest("scope"), Authority: hv2Authority("condition-authority"), ExpiresUnixNano: now.Add(10 * time.Minute).UnixNano()}
}

func TestHumanPanelV2CanonicalStableIDAndCurrentness(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	panel := hv2Panel(t, now)
	wantID, err := contract.DeriveHumanPanelIDV2(humanTenantV2, panel.Case, panel.Round, panel.QuorumPolicy)
	if err != nil || panel.ID != wantID {
		t.Fatalf("stable panel id mismatch: %s %v", panel.ID, err)
	}
	if panel.ContractVersion != contract.HumanMultiSignContractV2 {
		t.Fatalf("wrong V2 version %q", panel.ContractVersion)
	}
	if err := panel.ValidateCurrent(panel.ExactRef(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	drift := panel.ExactRef()
	drift.Digest = hv2Digest("drift")
	if err := panel.ValidateCurrent(drift, now.Add(time.Minute)); !core.HasReason(err, core.ReasonReviewCandidateConflict) {
		t.Fatalf("expected exact drift conflict, got %v", err)
	}
	if err := panel.ValidateCurrent(panel.ExactRef(), now.Add(time.Hour)); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("expected expiry, got %v", err)
	}
	if err := panel.ValidateCurrent(panel.ExactRef(), now.Add(-time.Nanosecond)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("expected clock rollback rejection, got %v", err)
	}
	clone := panel.Clone()
	clone.RoleRequirements[0].Role = "changed"
	if panel.RoleRequirements[0].Role == "changed" {
		t.Fatal("panel clone aliases role requirements")
	}
	if !contract.CanTransitionHumanPanelV2(contract.HumanPanelOpenV2, contract.HumanPanelQuorumSatisfiedV2) || contract.CanTransitionHumanPanelV2(contract.HumanPanelDecidedV2, contract.HumanPanelOpenV2) {
		t.Fatal("panel transition truth table drifted")
	}
}

func TestHumanPanelV2HardNegatives(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := hv2Panel(t, now)
	tests := map[string]func(*contract.HumanReviewPanelV2){
		"K zero":      func(v *contract.HumanReviewPanelV2) { v.AcceptThreshold = 0 },
		"K greater N": func(v *contract.HumanReviewPanelV2) { v.AcceptThreshold = 4 },
		"role duplicate": func(v *contract.HumanReviewPanelV2) {
			v.RoleRequirements = []contract.HumanRoleRequirementV2{{Role: "security", Minimum: 1}, {Role: "security", Minimum: 1}}
		},
		"role minima greater N": func(v *contract.HumanReviewPanelV2) {
			v.RoleRequirements = []contract.HumanRoleRequirementV2{{Role: "security", Minimum: 2}, {Role: "technical", Minimum: 2}}
		},
		"no delegation policy":   func(v *contract.HumanReviewPanelV2) { v.DelegationRequired = false },
		"production self review": func(v *contract.HumanReviewPanelV2) { v.ProductionSelfReviewAllowed = true },
		"duration exceeds policy": func(v *contract.HumanReviewPanelV2) {
			v.ExpiresUnixNano = v.CreatedUnixNano + v.MaxPanelDurationNanos + 1
		},
		"expiry exceeds policy":       func(v *contract.HumanReviewPanelV2) { v.ExpiresUnixNano = v.QuorumPolicy.ExpiresUnixNano + 1 },
		"invalid policy checked time": func(v *contract.HumanReviewPanelV2) { v.QuorumPolicy.CheckedUnixNano = 0 },
		"cross tenant policy":         func(v *contract.HumanReviewPanelV2) { v.QuorumPolicy.TenantID = "tenant-b" },
		"incomplete assignment set":   func(v *contract.HumanReviewPanelV2) { v.AssignmentRefs = v.AssignmentRefs[:2] },
		"duplicate assignment":        func(v *contract.HumanReviewPanelV2) { v.AssignmentRefs[1] = v.AssignmentRefs[0] },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			v := base.Clone()
			v.Digest = ""
			mutate(&v)
			if _, err := contract.SealHumanReviewPanelV2(v); err == nil {
				t.Fatal("expected fail closed")
			}
		})
	}
}

func TestHumanPanelV2TerminalRevisionNeverBecomesCurrentByTime(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	panel := hv2Panel(t, now)
	terminal := panel.Clone()
	terminal.Digest = ""
	terminal.Revision++
	terminal.UpdatedUnixNano++
	terminal.State = contract.HumanPanelExpiredV2
	sealed, err := contract.SealHumanReviewPanelV2(terminal)
	if err != nil {
		t.Fatal(err)
	}
	if err := sealed.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := sealed.ValidateCurrent(sealed.ExactRef(), now.Add(time.Minute)); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("terminal panel became current: %v", err)
	}
	if panel.Revision+1 != sealed.Revision {
		t.Fatal("explicit terminal revision did not advance exactly once")
	}
}

func TestHumanAssignmentV2DelegationLeaseAndDeepClone(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	panel := hv2Panel(t, now)
	assignment := hv2Assignment(t, now, panel, "reviewer-a")
	if err := assignment.ValidateCurrent(assignment.ExactRef(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := assignment.ValidateCurrent(assignment.ExactRef(), now.Add(6*time.Minute)); !core.HasReason(err, core.ReasonStaleLeaseRevision) {
		t.Fatalf("expected lease expiry, got %v", err)
	}
	clone := assignment.Clone()
	clone.Roles[0] = "changed"
	if assignment.Roles[0] == "changed" {
		t.Fatal("assignment clone aliases roles")
	}
	bad := assignment.Clone()
	bad.Digest = ""
	bad.DelegateIdentity = hv2Identity("other")
	if _, err := contract.SealHumanPanelAssignmentV2(bad); err == nil {
		t.Fatal("delegate/reviewer mismatch accepted")
	}
	bad = assignment.Clone()
	bad.Digest = ""
	bad.Delegated = false
	if _, err := contract.SealHumanPanelAssignmentV2(bad); err == nil {
		t.Fatal("direct assignment retained delegation")
	}
	bad = assignment.Clone()
	bad.Digest = ""
	bad.Roles = []string{"technical"}
	if _, err := contract.SealHumanPanelAssignmentV2(bad); err == nil {
		t.Fatal("delegated role absent from roles accepted")
	}
}

func TestHumanAttestationV2ExactBindingsConditionalAndSelfReview(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	panel := hv2Panel(t, now)
	assignment := hv2Assignment(t, now, panel, "reviewer-a")
	att := hv2Attestation(t, now, panel, assignment, contract.ResolutionConditionalV1)
	if err := att.ValidateCurrent(att.ExactRef(), now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	clone := att.Clone()
	clone.Evidence[0].Ref = "changed"
	if att.Evidence[0].Ref == "changed" {
		t.Fatal("attestation clone aliases evidence")
	}
	bad := att.Clone()
	bad.Digest = ""
	bad.ResponsibilitySubject.IdentityProof = contract.HumanIdentityProofRefV2{TenantID: humanTenantV2, Ref: att.ReviewerIdentity.Ref, Revision: 2, Digest: hv2Digest("new-revision")}
	if _, err := contract.SealHumanAttestationV2(bad); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("self-review not rejected: %v", err)
	}
	bad = att.Clone()
	bad.Digest = ""
	bad.Conditions = nil
	bad.ConditionsDigest = ""
	if _, err := contract.SealHumanAttestationV2(bad); !core.HasReason(err, core.ReasonReviewConditionUnsatisfied) {
		t.Fatalf("missing conditional conditions accepted: %v", err)
	}
	bad = att.Clone()
	bad.Digest = ""
	bad.EvidenceDigest = hv2Digest("drift")
	if _, err := contract.SealHumanAttestationV2(bad); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("evidence drift accepted: %v", err)
	}
	bad = att.Clone()
	bad.Digest = ""
	bad.Assignment.TenantID = "tenant-b"
	if _, err := contract.SealHumanAttestationV2(bad); err == nil {
		t.Fatal("cross-tenant assignment accepted")
	}
}

func TestHumanQuorumV2ThresholdDistinctIdentityVetoAndDigest(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	panel := hv2Panel(t, now)
	a1 := hv2Assignment(t, now, panel, "reviewer-a")
	a2 := hv2Assignment(t, now, panel, "reviewer-b")
	att1 := hv2Attestation(t, now, panel, a1, contract.ResolutionAcceptV1)
	att2 := hv2Attestation(t, now, panel, a2, contract.ResolutionAcceptV1)
	identities := []contract.HumanIdentityProofRefV2{a2.ReviewerIdentity, a1.ReviewerIdentity}
	reviewerDigest, err := contract.ComputeHumanReviewerSetDigestV2(identities)
	if err != nil {
		t.Fatal(err)
	}
	q, err := contract.SealHumanQuorumDecisionV2(contract.HumanQuorumDecisionV2{FactIdentityV1: contract.FactIdentityV1{TenantID: humanTenantV2, ID: "quorum-a", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, Panel: panel.ExactRef(), Policy: panel.QuorumPolicy, AcceptedAttestationRefs: []contract.HumanAttestationExactRefV2{att2.ExactRef(), att1.ExactRef()}, DistinctReviewerIdentityRefs: identities, SatisfiedRoleCounts: []contract.HumanSatisfiedRoleCountV2{{Role: "security", DistinctCurrentCount: 2}, {Role: "technical", DistinctCurrentCount: 2}}, AcceptCount: 2, Threshold: 2, Resolution: contract.ResolutionAcceptV1, EvidenceSetDigest: att1.EvidenceDigest, ReviewerSetDigest: reviewerDigest, CheckedUnixNano: now.Add(time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.ValidateCurrent(q.ExactRef(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	bad := q.Clone()
	bad.Digest = ""
	bad.AcceptCount = 1
	if _, err := contract.SealHumanQuorumDecisionV2(bad); err == nil {
		t.Fatal("accept count drift accepted")
	}
	bad = q.Clone()
	bad.Digest = ""
	bad.DistinctReviewerIdentityRefs[1] = bad.DistinctReviewerIdentityRefs[0]
	if _, err := contract.SealHumanQuorumDecisionV2(bad); err == nil {
		t.Fatal("duplicate identity counted twice")
	}
	bad = q.Clone()
	bad.Digest = ""
	bad.DistinctReviewerIdentityRefs[1] = bad.DistinctReviewerIdentityRefs[0]
	bad.DistinctReviewerIdentityRefs[1].Revision++
	bad.DistinctReviewerIdentityRefs[1].Digest = hv2Digest("renewed")
	if _, err := contract.SealHumanQuorumDecisionV2(bad); err == nil {
		t.Fatal("same actual identity with a renewed exact proof counted twice")
	}
	bad = q.Clone()
	bad.Digest = ""
	bad.OtherAttestationRefs = []contract.HumanAttestationExactRefV2{bad.AcceptedAttestationRefs[0]}
	bad.DistinctReviewerIdentityRefs = append(bad.DistinctReviewerIdentityRefs, hv2Identity("reviewer-c"))
	bad.ReviewerSetDigest, _ = contract.ComputeHumanReviewerSetDigestV2(bad.DistinctReviewerIdentityRefs)
	if _, err := contract.SealHumanQuorumDecisionV2(bad); err == nil {
		t.Fatal("attestation counted in both sets")
	}
	bad = q.Clone()
	bad.Digest = ""
	bad.ReviewerSetDigest = hv2Digest("drift")
	if _, err := contract.SealHumanQuorumDecisionV2(bad); err == nil {
		t.Fatal("reviewer set digest drift accepted")
	}
	vetoAtt := hv2Attestation(t, now, panel, a1, contract.ResolutionRejectV1)
	vetoRef := vetoAtt.ExactRef()
	vetoIdentities := []contract.HumanIdentityProofRefV2{a1.ReviewerIdentity}
	vetoDigest, _ := contract.ComputeHumanReviewerSetDigestV2(vetoIdentities)
	veto, err := contract.SealHumanQuorumDecisionV2(contract.HumanQuorumDecisionV2{FactIdentityV1: contract.FactIdentityV1{TenantID: humanTenantV2, ID: "quorum-veto", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, Panel: panel.ExactRef(), Policy: panel.QuorumPolicy, OtherAttestationRefs: []contract.HumanAttestationExactRefV2{vetoRef}, DistinctReviewerIdentityRefs: vetoIdentities, SatisfiedRoleCounts: []contract.HumanSatisfiedRoleCountV2{{Role: "security", DistinctCurrentCount: 1}}, Threshold: 2, Resolution: contract.ResolutionRejectV1, Vetoed: true, VetoAttestationRef: &vetoRef, EvidenceSetDigest: vetoAtt.EvidenceDigest, ReviewerSetDigest: vetoDigest, CheckedUnixNano: now.Add(time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if err := veto.Validate(); err != nil {
		t.Fatal(err)
	}
	bad = veto.Clone()
	bad.Digest = ""
	bad.Vetoed = false
	bad.VetoAttestationRef = nil
	if _, err := contract.SealHumanQuorumDecisionV2(bad); err == nil {
		t.Fatal("reject without veto proof accepted")
	}
}

func TestHumanVerdictV2ExactQuorumConditionalTerminalAndClone(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	panel := hv2Panel(t, now)
	a := hv2Assignment(t, now, panel, "reviewer-a")
	att := hv2Attestation(t, now, panel, a, contract.ResolutionAcceptV1)
	ids := []contract.HumanIdentityProofRefV2{a.ReviewerIdentity}
	rd, _ := contract.ComputeHumanReviewerSetDigestV2(ids)
	q, err := contract.SealHumanQuorumDecisionV2(contract.HumanQuorumDecisionV2{FactIdentityV1: contract.FactIdentityV1{TenantID: humanTenantV2, ID: "quorum", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, Panel: panel.ExactRef(), Policy: panel.QuorumPolicy, AcceptedAttestationRefs: []contract.HumanAttestationExactRefV2{att.ExactRef()}, DistinctReviewerIdentityRefs: ids, SatisfiedRoleCounts: []contract.HumanSatisfiedRoleCountV2{{Role: "security", DistinctCurrentCount: 1}}, AcceptCount: 1, Threshold: 1, Resolution: contract.ResolutionAcceptV1, EvidenceSetDigest: att.EvidenceDigest, ReviewerSetDigest: rd, CheckedUnixNano: now.Add(time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: humanTenantV2, ID: "agent", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: hv2Digest("plan")}, Instance: core.InstanceRef{ID: "instance", Epoch: 1}, AuthorityEpoch: 1}
	currentScope := runtimeports.ExecutionScopeBindingRefV2{Ref: "scope", Revision: 1, Digest: hv2Digest("scope")}
	v, err := contract.SealHumanVerdictV2(contract.HumanVerdictV2{FactIdentityV1: contract.FactIdentityV1{TenantID: humanTenantV2, ID: "verdict", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, Case: panel.Case, Target: panel.Target, Round: panel.Round, Panel: panel.ExactRef(), QuorumDecision: q.ExactRef(), Policy: panel.QuorumPolicy, Scope: scope, CurrentScope: currentScope, ReviewerSetDigest: rd, ReviewerAuthorityRefs: []runtimeports.AuthorityBindingRefV2{a.ReviewerAuthority}, BindingClosures: []runtimeports.ReviewComponentBindingRefV2{a.ReviewerBinding}, AttestationRefs: []contract.HumanAttestationExactRefV2{att.ExactRef()}, Evidence: att.Evidence, EvidenceSetDigest: att.EvidenceDigest, ReasonCodes: []string{"review/quorum-satisfied"}, State: contract.HumanVerdictAcceptedV2, ExpiresUnixNano: now.Add(4 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.ValidateCurrent(v.ExactRef(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	clone := v.Clone()
	clone.ReasonCodes[0] = "changed"
	if v.ReasonCodes[0] == "changed" {
		t.Fatal("verdict clone aliases reason codes")
	}
	bad := v.Clone()
	bad.Digest = ""
	bad.State = contract.HumanVerdictConditionalV2
	if _, err := contract.SealHumanVerdictV2(bad); err == nil {
		t.Fatal("conditional verdict without conditions accepted")
	}
	bad = v.Clone()
	bad.Digest = ""
	bad.State = contract.HumanVerdictRevokedV2
	if _, err := contract.SealHumanVerdictV2(bad); err == nil {
		t.Fatal("terminal verdict without invalidation reason accepted")
	}
	bad = v.Clone()
	bad.Digest = ""
	bad.AttestationRefs[0].TenantID = "tenant-b"
	if _, err := contract.SealHumanVerdictV2(bad); err == nil {
		t.Fatal("cross-tenant attestation accepted")
	}
}

func TestHumanMultiSignV2LiteralDigestGolden(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	panel := hv2Panel(t, now)
	const want = "sha256:5497f935f8d6f98bed026dc6fee173d131fb6708e7e0bd19caf1459f2b9ab073"
	if string(panel.Digest) != want {
		t.Fatalf("literal digest drift: got %s want %s", panel.Digest, want)
	}
}
