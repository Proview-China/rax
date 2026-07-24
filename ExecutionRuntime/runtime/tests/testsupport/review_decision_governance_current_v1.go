package testsupport

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ReviewDecisionGovernanceFixtureV1 struct {
	Now        time.Time
	Target     ports.ReviewDecisionTargetRefV1
	Assignment ports.ReviewDecisionAssignmentRefV1
	Policy     ports.ReviewDecisionPolicyCurrentProjectionV1
	Authority  ports.ReviewDecisionAuthorityCurrentProjectionV1
	Scope      ports.ReviewDecisionScopeCurrentProjectionV1
}

func SeedReviewDecisionGovernanceSourcesV1(source *fakes.ReviewDecisionGovernanceSourceStoreV1, fixture ReviewDecisionGovernanceFixtureV1) {
	source.PutTargetV1(fixture.Target)
	source.PutAssignmentV1(fixture.Assignment)
	source.PutPolicyV1(fixture.Policy.Fact)
	source.PutAuthorityV1(fixture.Authority.Fact)
	source.PutScopeV1(fixture.Scope.Fact)
}

func ReviewDecisionGovernanceFixture() ReviewDecisionGovernanceFixtureV1 {
	now := time.Unix(1_810_000_000, 0)
	sandbox := core.SandboxLeaseRef{ID: "sandbox-gov", Epoch: 5}
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-gov", ID: "agent-gov", Epoch: 2}, Lineage: core.LineageRef{ID: "lineage-gov", PlanDigest: reviewGovernanceDigest("plan")}, Instance: core.InstanceRef{ID: "instance-gov", Epoch: 3}, SandboxLease: &sandbox, AuthorityEpoch: 4}
	target := ports.ReviewDecisionTargetRefV1{TenantID: "tenant-gov", ID: "target-gov", Revision: 7, Digest: reviewGovernanceDigest("target"), RunID: "run-gov"}
	assignment := ports.ReviewDecisionAssignmentRefV1{TenantID: target.TenantID, ID: "assignment-gov", Revision: 2, Digest: reviewGovernanceDigest("assignment"), ReviewerID: "reviewer-gov"}

	policyRef := ports.ReviewPolicyBindingRefV2{Ref: "policy-gov", Revision: 3, Digest: reviewGovernanceDigest("pending")}
	policyFact := ports.ReviewPolicyFactV2{Ref: policyRef.Ref, Revision: policyRef.Revision, SubjectDigest: target.Digest, Scope: scope, RunID: target.RunID, CurrentScope: ports.ExecutionScopeBindingRefV2{Ref: "scope-gov", Revision: 4, Digest: reviewGovernanceDigest("pending-scope")}, RiskClass: "praxis/high", ActorAuthorityRef: "actor-gov", ReviewerAuthorityRef: "reviewer-authority-gov", PolicyDecisionRef: "decision-gov", Active: true, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	policyFact.Digest = mustReviewGovernanceDigest(policyFact.DigestV2())
	policyRef.Digest = policyFact.Digest
	policySubject := ports.ReviewDecisionPolicyCurrentSubjectV1{Target: target, Policy: policyRef}
	policyID := mustReviewGovernanceID(ports.DeriveReviewDecisionPolicyCurrentProjectionIDV1(policySubject, policyFact.Ref))
	policy := ports.ReviewDecisionPolicyCurrentProjectionV1{ContractVersion: ports.ReviewDecisionGovernanceCurrentContractVersionV1, Ref: ports.ReviewDecisionPolicyCurrentProjectionRefV1{ID: policyID, Revision: 1}, Subject: policySubject, Fact: policyFact, State: ports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()}
	policy.ProjectionDigest = mustReviewGovernanceDigest(ports.DigestReviewDecisionPolicyCurrentProjectionV1(policy))
	policy.Ref.Digest = policy.ProjectionDigest

	action := reviewGovernanceDigest("action")
	authorityBinding := ports.AuthorityBindingRefV2{Ref: "reviewer-authority-gov", Revision: 5, Digest: reviewGovernanceDigest("authority"), Epoch: scope.AuthorityEpoch}
	authoritySubject := ports.ReviewDecisionAuthorityCurrentSubjectV1{Role: ports.ReviewDecisionAuthorityReviewerV1, Target: target, Assignment: assignment, Authority: authorityBinding, ActionScopeDigest: action}
	authorityFact := ports.DispatchAuthorityFactV2{Ref: authorityBinding.Ref, Digest: authorityBinding.Digest, Revision: authorityBinding.Revision, Scope: scope, ActionScopeDigest: action, State: ports.AuthorityFactActive, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	authorityID := mustReviewGovernanceID(ports.DeriveReviewDecisionAuthorityCurrentProjectionIDV1(authoritySubject, authorityFact.Ref))
	authority := ports.ReviewDecisionAuthorityCurrentProjectionV1{ContractVersion: ports.ReviewDecisionGovernanceCurrentContractVersionV1, Ref: ports.ReviewDecisionAuthorityCurrentProjectionRefV1{ID: authorityID, Revision: 1}, Subject: authoritySubject, Fact: authorityFact, State: ports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()}
	authority.ProjectionDigest = mustReviewGovernanceDigest(ports.DigestReviewDecisionAuthorityCurrentProjectionV1(authority))
	authority.Ref.Digest = authority.ProjectionDigest

	sandboxSource := ports.GovernanceSourceFactRefV2{Ref: "sandbox-source", Revision: 1, Digest: reviewGovernanceDigest("sandbox-source")}
	scopeFact := ports.ExecutionScopeCurrentFactV2{Ref: "scope-gov", Revision: 4, Scope: scope, CapabilityGrantDigest: reviewGovernanceDigest("grant"), ActivationSource: reviewGovernanceSource("activation"), InstanceSource: reviewGovernanceSource("instance"), SandboxSource: &sandboxSource, AuthoritySource: reviewGovernanceSource("authority"), BindingSource: reviewGovernanceSource("binding"), RunSource: reviewGovernanceSource("run"), ActiveRunID: target.RunID, RunState: "running", ProjectionWatermark: 9, State: ports.ExecutionScopeFactActive, ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	scopeFact.Digest = mustReviewGovernanceDigest(scopeFact.DigestV2())
	currentScope, err := scopeFact.BindingRefV2()
	if err != nil {
		panic(err)
	}
	scopeSubject := ports.ReviewDecisionScopeCurrentSubjectV1{TenantID: target.TenantID, Target: target, RunID: target.RunID, Scope: scope, CurrentScope: currentScope, ActionScopeDigest: action}
	scopeID := mustReviewGovernanceID(ports.DeriveReviewDecisionScopeCurrentProjectionIDV1(scopeSubject, scopeFact.Ref))
	scopeProjection := ports.ReviewDecisionScopeCurrentProjectionV1{ContractVersion: ports.ReviewDecisionGovernanceCurrentContractVersionV1, Ref: ports.ReviewDecisionScopeCurrentProjectionRefV1{ID: scopeID, Revision: 1}, Subject: scopeSubject, Fact: scopeFact, State: ports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano()}
	scopeProjection.ProjectionDigest = mustReviewGovernanceDigest(ports.DigestReviewDecisionScopeCurrentProjectionV1(scopeProjection))
	scopeProjection.Ref.Digest = scopeProjection.ProjectionDigest
	return ReviewDecisionGovernanceFixtureV1{Now: now, Target: target, Assignment: assignment, Policy: policy, Authority: authority, Scope: scopeProjection}
}

func (f ReviewDecisionGovernanceFixtureV1) NextPolicy(checked time.Time) ports.ReviewDecisionPolicyCurrentProjectionV1 {
	next := f.Policy
	next.Ref.Revision++
	next.CheckedUnixNano = checked.UnixNano()
	next.Ref.Digest, next.ProjectionDigest = "", ""
	next.ProjectionDigest = mustReviewGovernanceDigest(ports.DigestReviewDecisionPolicyCurrentProjectionV1(next))
	next.Ref.Digest = next.ProjectionDigest
	return next
}
func reviewGovernanceSource(ref string) ports.GovernanceSourceFactRefV2 {
	return ports.GovernanceSourceFactRefV2{Ref: ref, Revision: 1, Digest: reviewGovernanceDigest(ref)}
}
func reviewGovernanceDigest(v string) core.Digest { return core.DigestBytes([]byte(v)) }
func mustReviewGovernanceDigest(v core.Digest, err error) core.Digest {
	if err != nil {
		panic(err)
	}
	return v
}
func mustReviewGovernanceID(v string, err error) string {
	if err != nil {
		panic(err)
	}
	return v
}
