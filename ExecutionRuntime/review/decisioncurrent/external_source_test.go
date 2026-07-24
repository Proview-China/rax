package decisioncurrent

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestExternalSourceV1ProjectsExactOwnerCutAndShortestTTL(t *testing.T) {
	fixture := newExternalSourceFixtureV1(t)
	got, err := fixture.source.InspectDecisionExternalCurrentV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Current || got.ExternalProof == nil || got.ExternalProof.Validate() != nil {
		t.Fatalf("external current proof is incomplete: %+v", got)
	}
	if got.ExpiresUnixNano != fixture.policy.value.ExpiresUnixNano {
		t.Fatalf("shortest Owner TTL was not preserved: got=%d want=%d", got.ExpiresUnixNano, fixture.policy.value.ExpiresUnixNano)
	}
	if got.Policy != fixture.policy.value.Fact || got.Binding.Binding != fixture.request.Assignment.ReviewerBinding {
		t.Fatal("external projection drifted from exact Policy or Binding")
	}
}

func TestExternalSourceV1EachOwnerCanBeTheUniqueShortestTTL(t *testing.T) {
	for _, name := range []string{"policy", "actor_authority", "reviewer_authority", "scope", "binding"} {
		t.Run(name, func(t *testing.T) {
			fixture := newExternalSourceFixtureV1(t)
			checked := time.Unix(0, fixture.policy.value.CheckedUnixNano)
			long := checked.Add(6 * time.Minute)
			short := checked.Add(time.Minute)
			fixture.policy.value = sealExternalPolicyV1(t, fixture.policy.value.Subject, fixture.policy.value.Fact, checked, long)
			for role, value := range fixture.authority.values {
				fixture.authority.values[role] = sealExternalAuthorityV1(t, value.Subject, value.Fact, checked, long)
			}
			fixture.scope.value = sealExternalScopeV1(t, fixture.scope.value.Subject, fixture.scope.value.Fact, checked, long)
			want := short.UnixNano()
			switch name {
			case "policy":
				fixture.policy.value = sealExternalPolicyV1(t, fixture.policy.value.Subject, fixture.policy.value.Fact, checked, short)
			case "actor_authority":
				value := fixture.authority.values[runtimeports.ReviewDecisionAuthorityActorV1]
				fixture.authority.values[runtimeports.ReviewDecisionAuthorityActorV1] = sealExternalAuthorityV1(t, value.Subject, value.Fact, checked, short)
			case "reviewer_authority":
				value := fixture.authority.values[runtimeports.ReviewDecisionAuthorityReviewerV1]
				fixture.authority.values[runtimeports.ReviewDecisionAuthorityReviewerV1] = sealExternalAuthorityV1(t, value.Subject, value.Fact, checked, short)
			case "scope":
				fixture.scope.value = sealExternalScopeV1(t, fixture.scope.value.Subject, fixture.scope.value.Fact, checked, short)
			case "binding":
				projection, err := fixture.binding.inner.InspectCurrentReviewBindingV1(context.Background(), runtimeports.InspectCurrentReviewBindingRequestV1{ExpectedRef: fixture.binding.ref, ExpectedSource: fixture.request.Assignment.ReviewerBinding, ExpectedSubject: fixture.binding.subject})
				if err != nil {
					t.Fatal(err)
				}
				want = projection.ExpiresUnixNano
			}
			got, err := fixture.source.InspectDecisionExternalCurrentV1(context.Background(), fixture.request)
			if err != nil {
				t.Fatal(err)
			}
			if got.ExpiresUnixNano != want {
				t.Fatalf("%s was not the unique shortest TTL: got=%d want=%d", name, got.ExpiresUnixNano, want)
			}
		})
	}
}

func TestExternalSourceV1EachGovernanceOwnerDriftFailsClosed(t *testing.T) {
	for _, name := range []string{"policy", "actor_authority", "reviewer_authority", "scope", "binding"} {
		t.Run(name, func(t *testing.T) {
			fixture := newExternalSourceFixtureV1(t)
			switch name {
			case "policy":
				fixture.policy.driftOnInspect = 2
			case "actor_authority":
				fixture.authority.driftRole = runtimeports.ReviewDecisionAuthorityActorV1
			case "reviewer_authority":
				fixture.authority.driftRole = runtimeports.ReviewDecisionAuthorityReviewerV1
			case "scope":
				fixture.scope.driftOnInspect = 2
			case "binding":
				fixture.binding.driftOnInspect = 2
			}
			got, err := fixture.source.InspectDecisionExternalCurrentV1(context.Background(), fixture.request)
			if err == nil || !reflect.DeepEqual(got, reviewport.DecisionExternalCurrentProjectionV1{}) {
				t.Fatalf("%s drift reached a current projection: value=%+v err=%v", name, got, err)
			}
		})
	}
}

func TestExternalSourceV1ExactInspectLostReplyIgnoresCanceledOriginalContext(t *testing.T) {
	fixture := newExternalSourceFixtureV1(t)
	ctx, cancel := context.WithCancel(context.Background())
	fixture.policy.cancel = cancel
	fixture.policy.loseInspectReply = true
	got, err := fixture.source.InspectDecisionExternalCurrentV1(ctx, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Current || fixture.policy.inspectCalls < 3 || ctx.Err() != context.Canceled {
		t.Fatalf("exact lost reply was not recovered under a detached context: calls=%d ctx=%v", fixture.policy.inspectCalls, ctx.Err())
	}
}

func TestExternalSourceV1ResolveUnknownStartsNewS1OutsideCanceledContext(t *testing.T) {
	fixture := newExternalSourceFixtureV1(t)
	ctx, cancel := context.WithCancel(context.Background())
	fixture.policy.cancel = cancel
	fixture.policy.loseResolveReply = true
	got, err := fixture.source.InspectDecisionExternalCurrentV1(ctx, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Current || fixture.policy.resolveCalls != 2 || ctx.Err() != context.Canceled {
		t.Fatalf("Resolve unknown did not start one new detached S1: calls=%d ctx=%v", fixture.policy.resolveCalls, ctx.Err())
	}
}

func TestExternalSourceV1ExactRecoveryIsBoundedAndPreservesOriginalUnknown(t *testing.T) {
	t.Run("blocking_inspect_uses_target_ttl", func(t *testing.T) {
		fixture := newExternalSourceFixtureV1(t)
		fixture.request.Target.ExpiresUnixNano = fixture.now.value.Add(20 * time.Millisecond).UnixNano()
		fixture.policy.loseInspectReply = true
		fixture.policy.blockInspectAfter = 2
		started := time.Now()
		got, err := fixture.source.InspectDecisionExternalCurrentV1(context.Background(), fixture.request)
		if elapsed := time.Since(started); elapsed >= time.Second {
			t.Fatalf("blocking exact recovery exceeded bounded target TTL: %v", elapsed)
		}
		if !core.HasCategory(err, core.ErrorIndeterminate) || !core.HasReason(err, core.ReasonInspectCoverageIncomplete) || fixture.policy.inspectCalls != 2 || !reflect.DeepEqual(got, reviewport.DecisionExternalCurrentProjectionV1{}) {
			t.Fatalf("blocking recovery replaced the original Unknown or retried more than once: calls=%d value=%+v err=%v", fixture.policy.inspectCalls, got, err)
		}
	})

	t.Run("authoritative_not_found_does_not_replace_unknown", func(t *testing.T) {
		fixture := newExternalSourceFixtureV1(t)
		fixture.policy.loseInspectReply = true
		fixture.policy.notFoundInspectAfter = 2
		got, err := fixture.source.InspectDecisionExternalCurrentV1(context.Background(), fixture.request)
		if !core.HasCategory(err, core.ErrorIndeterminate) || !core.HasReason(err, core.ReasonInspectCoverageIncomplete) || fixture.policy.inspectCalls != 2 || !reflect.DeepEqual(got, reviewport.DecisionExternalCurrentProjectionV1{}) {
			t.Fatalf("NotFound recovery replaced the original Unknown: calls=%d value=%+v err=%v", fixture.policy.inspectCalls, got, err)
		}
	})

	t.Run("expired_subject_skips_detached_retry", func(t *testing.T) {
		fixture := newExternalSourceFixtureV1(t)
		fixture.request.Target.ExpiresUnixNano = fixture.now.value.UnixNano()
		fixture.policy.loseInspectReply = true
		got, err := fixture.source.InspectDecisionExternalCurrentV1(context.Background(), fixture.request)
		if !core.HasCategory(err, core.ErrorIndeterminate) || fixture.policy.inspectCalls != 1 || !reflect.DeepEqual(got, reviewport.DecisionExternalCurrentProjectionV1{}) {
			t.Fatalf("expired subject started detached recovery: calls=%d value=%+v err=%v", fixture.policy.inspectCalls, got, err)
		}
	})
}

func TestExternalSourceV1ClockRollbackDuringS2FailsClosed(t *testing.T) {
	fixture := newExternalSourceFixtureV1(t)
	fixture.policy.rollbackClockOnInspect = 2
	got, err := fixture.source.InspectDecisionExternalCurrentV1(context.Background(), fixture.request)
	if !core.HasReason(err, core.ReasonClockRegression) || !reflect.DeepEqual(got, reviewport.DecisionExternalCurrentProjectionV1{}) {
		t.Fatalf("clock rollback reached a projection: value=%+v err=%v", got, err)
	}
}

type externalSourceFixtureV1 struct {
	now       *externalClockV1
	request   reviewport.DecisionExternalCurrentRequestV1
	policy    *policyReaderV1
	authority *authorityReaderV1
	scope     *scopeReaderV1
	binding   *bindingReaderV1
	source    *ExternalSourceV1
}

func newExternalSourceFixtureV1(t *testing.T) externalSourceFixtureV1 {
	t.Helper()
	base := time.Unix(2_200_000_000, 0)
	clock := &externalClockV1{value: base.Add(time.Second)}
	tenant := core.TenantID("tenant-review-external")
	scopeValue := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: tenant, ID: "agent-review-external", Epoch: 2},
		Lineage:        core.LineageRef{ID: "lineage-review-external", PlanDigest: externalDigestV1("plan")},
		Instance:       core.InstanceRef{ID: "instance-review-external", Epoch: 3},
		AuthorityEpoch: 4,
	}
	targetRef := runtimeports.ReviewDecisionTargetRefV1{TenantID: tenant, ID: "target-review-external", Revision: 7, Digest: externalDigestV1("target"), RunID: "run-review-external"}
	assignmentRef := runtimeports.ReviewDecisionAssignmentRefV1{TenantID: tenant, ID: "assignment-review-external", Revision: 2, Digest: externalDigestV1("assignment"), ReviewerID: "reviewer-review-external"}
	actionScopeDigest := externalDigestV1("action-scope")

	scopeFact := runtimeports.ExecutionScopeCurrentFactV2{
		Ref: "scope-review-external", Revision: 5, Scope: scopeValue,
		CapabilityGrantDigest: externalDigestV1("scope-grant"),
		ActivationSource:      externalGovernanceSourceV1("activation"),
		InstanceSource:        externalGovernanceSourceV1("instance"),
		AuthoritySource:       externalGovernanceSourceV1("authority"),
		BindingSource:         externalGovernanceSourceV1("binding"),
		RunSource:             externalGovernanceSourceV1("run"),
		ActiveRunID:           targetRef.RunID, RunState: "running", ProjectionWatermark: 9,
		State: runtimeports.ExecutionScopeFactActive, ExpiresUnixNano: base.Add(10 * time.Minute).UnixNano(),
	}
	scopeDigest, err := scopeFact.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	scopeFact.Digest = scopeDigest
	currentScope, err := scopeFact.BindingRefV2()
	if err != nil {
		t.Fatal(err)
	}

	actorBinding, actorFact := externalAuthorityV1(t, "actor-authority-review-external", scopeValue, actionScopeDigest, base.Add(9*time.Minute))
	reviewerBinding, reviewerFact := externalAuthorityV1(t, "reviewer-authority-review-external", scopeValue, actionScopeDigest, base.Add(8*time.Minute))

	policyFact := runtimeports.ReviewPolicyFactV2{
		Ref: "policy-review-external", Revision: 3, SubjectDigest: targetRef.Digest,
		Scope: scopeValue, RunID: targetRef.RunID, CurrentScope: currentScope,
		RiskClass: "review/high", ActorAuthorityRef: actorBinding.Ref, ReviewerAuthorityRef: reviewerBinding.Ref,
		PolicyDecisionRef: "policy-decision-review-external", Active: true,
		ExpiresUnixNano: base.Add(7 * time.Minute).UnixNano(),
	}
	policyDigest, err := policyFact.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	policyFact.Digest = policyDigest
	policyBinding := runtimeports.ReviewPolicyBindingRefV2{Ref: policyFact.Ref, Revision: policyFact.Revision, Digest: policyFact.Digest}

	policySubject := runtimeports.ReviewDecisionPolicyCurrentSubjectV1{Target: targetRef, Policy: policyBinding}
	policyProjection := sealExternalPolicyV1(t, policySubject, policyFact, base, base.Add(30*time.Second))
	actorSubject := runtimeports.ReviewDecisionAuthorityCurrentSubjectV1{Role: runtimeports.ReviewDecisionAuthorityActorV1, Target: targetRef, Assignment: assignmentRef, Authority: actorBinding, ActionScopeDigest: actionScopeDigest}
	reviewerSubject := runtimeports.ReviewDecisionAuthorityCurrentSubjectV1{Role: runtimeports.ReviewDecisionAuthorityReviewerV1, Target: targetRef, Assignment: assignmentRef, Authority: reviewerBinding, ActionScopeDigest: actionScopeDigest}
	actorProjection := sealExternalAuthorityV1(t, actorSubject, actorFact, base, base.Add(45*time.Second))
	reviewerProjection := sealExternalAuthorityV1(t, reviewerSubject, reviewerFact, base, base.Add(50*time.Second))
	scopeSubject := runtimeports.ReviewDecisionScopeCurrentSubjectV1{TenantID: tenant, Target: targetRef, RunID: targetRef.RunID, Scope: scopeValue, CurrentScope: currentScope, ActionScopeDigest: actionScopeDigest}
	scopeProjection := sealExternalScopeV1(t, scopeSubject, scopeFact, base, base.Add(55*time.Second))

	bindingStore, sourceBinding, subjectBinding := externalBindingStoreV1(t, base, clock.Now, tenant, targetRef, assignmentRef)
	request := reviewport.DecisionExternalCurrentRequestV1{
		Target:     contract.TargetSnapshotV1{FactIdentityV1: contract.FactIdentityV1{TenantID: tenant, ID: targetRef.ID, Revision: targetRef.Revision, Digest: targetRef.Digest}, Scope: scopeValue, RunID: targetRef.RunID, ActionScopeDigest: actionScopeDigest, Policy: policyBinding, ActorAuthority: actorBinding, CurrentScope: currentScope},
		Assignment: contract.ReviewerAssignmentV1{FactIdentityV1: contract.FactIdentityV1{TenantID: tenant, ID: assignmentRef.ID, Revision: assignmentRef.Revision, Digest: assignmentRef.Digest}, ReviewerID: assignmentRef.ReviewerID, ReviewerAuthority: reviewerBinding, ReviewerBinding: sourceBinding},
	}
	if subjectBinding != (runtimeports.ReviewBindingSubjectV1{TenantID: tenant, AssignmentID: assignmentRef.ID, AssignmentRevision: assignmentRef.Revision, AssignmentDigest: assignmentRef.Digest, ReviewerID: assignmentRef.ReviewerID, TargetID: targetRef.ID, TargetRevision: targetRef.Revision, TargetDigest: targetRef.Digest}) {
		t.Fatal("binding fixture drifted from Review request")
	}
	bindingRef, err := bindingStore.ResolveCurrentReviewBindingV1(context.Background(), runtimeports.ResolveReviewBindingCurrentRequestV1{Source: sourceBinding, Subject: subjectBinding})
	if err != nil {
		t.Fatal(err)
	}
	binding := &bindingReaderV1{inner: bindingStore, ref: bindingRef, subject: subjectBinding}
	policy := &policyReaderV1{value: policyProjection, clock: clock}
	authority := &authorityReaderV1{values: map[runtimeports.ReviewDecisionAuthorityRoleV1]runtimeports.ReviewDecisionAuthorityCurrentProjectionV1{runtimeports.ReviewDecisionAuthorityActorV1: actorProjection, runtimeports.ReviewDecisionAuthorityReviewerV1: reviewerProjection}}
	scopeReader := &scopeReaderV1{value: scopeProjection}
	external, err := NewExternalSourceV1(binding, noEvidenceReaderV1{}, policy, authority, scopeReader, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	return externalSourceFixtureV1{now: clock, request: request, policy: policy, authority: authority, scope: scopeReader, binding: binding, source: external}
}

type externalClockV1 struct {
	mu       sync.Mutex
	value    time.Time
	rollback bool
}

func (c *externalClockV1) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rollback {
		return c.value.Add(-time.Second)
	}
	c.value = c.value.Add(time.Nanosecond)
	return c.value
}

type policyReaderV1 struct {
	value                  runtimeports.ReviewDecisionPolicyCurrentProjectionV1
	resolveCalls           int
	inspectCalls           int
	driftOnInspect         int
	loseResolveReply       bool
	loseInspectReply       bool
	cancel                 context.CancelFunc
	rollbackClockOnInspect int
	blockInspectAfter      int
	notFoundInspectAfter   int
	clock                  *externalClockV1
}

func (r *policyReaderV1) ResolveCurrentReviewDecisionPolicyV1(ctx context.Context, request runtimeports.ReviewDecisionPolicyCurrentResolveRequestV1) (runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1, error) {
	r.resolveCalls++
	if r.loseResolveReply && r.resolveCalls == 1 {
		if r.cancel != nil {
			r.cancel()
		}
		return runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "policy Resolve reply was unknown")
	}
	if err := ctx.Err(); err != nil {
		return runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "policy Resolve context ended")
	}
	if request.Subject != r.value.Subject {
		return runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "policy subject drifted")
	}
	return r.value.Ref, nil
}
func (r *policyReaderV1) InspectCurrentReviewDecisionPolicyV1(ctx context.Context, subject runtimeports.ReviewDecisionPolicyCurrentSubjectV1, expected runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1) (runtimeports.ReviewDecisionPolicyCurrentProjectionV1, error) {
	r.inspectCalls++
	if r.loseInspectReply && r.inspectCalls == 1 {
		if r.cancel != nil {
			r.cancel()
		}
		return runtimeports.ReviewDecisionPolicyCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "policy exact Inspect reply was lost")
	}
	if r.blockInspectAfter > 0 && r.inspectCalls >= r.blockInspectAfter {
		<-ctx.Done()
		return runtimeports.ReviewDecisionPolicyCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "policy exact Inspect recovery timed out")
	}
	if r.notFoundInspectAfter > 0 && r.inspectCalls >= r.notFoundInspectAfter {
		return runtimeports.ReviewDecisionPolicyCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "policy projection is not found")
	}
	if err := ctx.Err(); err != nil {
		return runtimeports.ReviewDecisionPolicyCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "policy Inspect context ended")
	}
	value := r.value
	if r.driftOnInspect > 0 && r.inspectCalls >= r.driftOnInspect {
		value.Ref.Revision++
	}
	if r.rollbackClockOnInspect > 0 && r.inspectCalls >= r.rollbackClockOnInspect {
		r.clock.mu.Lock()
		r.clock.rollback = true
		r.clock.mu.Unlock()
	}
	if subject != value.Subject || expected != value.Ref {
		return runtimeports.ReviewDecisionPolicyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "policy current ref drifted")
	}
	return value, nil
}
func (r *policyReaderV1) InspectHistoricalReviewDecisionPolicyV1(context.Context, runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1) (runtimeports.ReviewDecisionPolicyCurrentProjectionV1, error) {
	return r.value, nil
}

type authorityReaderV1 struct {
	values    map[runtimeports.ReviewDecisionAuthorityRoleV1]runtimeports.ReviewDecisionAuthorityCurrentProjectionV1
	calls     map[runtimeports.ReviewDecisionAuthorityRoleV1]int
	driftRole runtimeports.ReviewDecisionAuthorityRoleV1
}

func (r *authorityReaderV1) ResolveCurrentReviewDecisionAuthorityV1(_ context.Context, request runtimeports.ReviewDecisionAuthorityCurrentResolveRequestV1) (runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1, error) {
	return r.values[request.Subject.Role].Ref, nil
}
func (r *authorityReaderV1) InspectCurrentReviewDecisionAuthorityV1(_ context.Context, subject runtimeports.ReviewDecisionAuthorityCurrentSubjectV1, expected runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1) (runtimeports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
	if r.calls == nil {
		r.calls = make(map[runtimeports.ReviewDecisionAuthorityRoleV1]int)
	}
	r.calls[subject.Role]++
	value := r.values[subject.Role]
	if r.driftRole == subject.Role && r.calls[subject.Role] >= 2 {
		value.Ref.Revision++
	}
	if subject != value.Subject || expected != value.Ref {
		return runtimeports.ReviewDecisionAuthorityCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "authority current ref drifted")
	}
	return value, nil
}
func (r *authorityReaderV1) InspectHistoricalReviewDecisionAuthorityV1(context.Context, runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1) (runtimeports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
	return runtimeports.ReviewDecisionAuthorityCurrentProjectionV1{}, nil
}

type scopeReaderV1 struct {
	value          runtimeports.ReviewDecisionScopeCurrentProjectionV1
	inspectCalls   int
	driftOnInspect int
}

func (r *scopeReaderV1) ResolveCurrentReviewDecisionScopeV1(context.Context, runtimeports.ReviewDecisionScopeCurrentResolveRequestV1) (runtimeports.ReviewDecisionScopeCurrentProjectionRefV1, error) {
	return r.value.Ref, nil
}
func (r *scopeReaderV1) InspectCurrentReviewDecisionScopeV1(_ context.Context, subject runtimeports.ReviewDecisionScopeCurrentSubjectV1, expected runtimeports.ReviewDecisionScopeCurrentProjectionRefV1) (runtimeports.ReviewDecisionScopeCurrentProjectionV1, error) {
	r.inspectCalls++
	value := r.value
	if r.driftOnInspect > 0 && r.inspectCalls >= r.driftOnInspect {
		value.Ref.Revision++
	}
	if subject != value.Subject || expected != value.Ref {
		return runtimeports.ReviewDecisionScopeCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "scope current ref drifted")
	}
	return value, nil
}
func (r *scopeReaderV1) InspectHistoricalReviewDecisionScopeV1(context.Context, runtimeports.ReviewDecisionScopeCurrentProjectionRefV1) (runtimeports.ReviewDecisionScopeCurrentProjectionV1, error) {
	return r.value, nil
}

type noEvidenceReaderV1 struct{}

func (noEvidenceReaderV1) ResolveReviewEvidenceApplicabilityCurrentV1(context.Context, runtimeports.ResolveReviewEvidenceApplicabilityCurrentRequestV1) (runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	return runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, core.NewError(core.ErrorInternal, core.ReasonInvalidState, "empty Evidence fixture must not resolve")
}

type bindingReaderV1 struct {
	inner          runtimeports.ReviewBindingAuthoritativeCurrentReaderV1
	ref            runtimeports.ReviewBindingProjectionRefV1
	subject        runtimeports.ReviewBindingSubjectV1
	inspectCalls   int
	driftOnInspect int
}

func (r *bindingReaderV1) ResolveCurrentReviewBindingV1(ctx context.Context, request runtimeports.ResolveReviewBindingCurrentRequestV1) (runtimeports.ReviewBindingProjectionRefV1, error) {
	return r.inner.ResolveCurrentReviewBindingV1(ctx, request)
}
func (r *bindingReaderV1) InspectReviewBindingProjectionV1(ctx context.Context, request runtimeports.InspectReviewBindingProjectionRequestV1) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
	return r.inner.InspectReviewBindingProjectionV1(ctx, request)
}
func (r *bindingReaderV1) InspectCurrentReviewBindingV1(ctx context.Context, request runtimeports.InspectCurrentReviewBindingRequestV1) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
	r.inspectCalls++
	if r.driftOnInspect > 0 && r.inspectCalls >= r.driftOnInspect {
		return runtimeports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "binding current index drifted")
	}
	return r.inner.InspectCurrentReviewBindingV1(ctx, request)
}
func (noEvidenceReaderV1) InspectCurrentReviewEvidenceApplicabilityV1(context.Context, runtimeports.ReviewEvidenceApplicabilityRefV1) (runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	return runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, core.NewError(core.ErrorInternal, core.ReasonInvalidState, "empty Evidence fixture must not inspect")
}
func (noEvidenceReaderV1) InspectHistoricalReviewEvidenceApplicabilityV1(context.Context, runtimeports.ReviewEvidenceApplicabilityRefV1) (runtimeports.ReviewEvidenceApplicabilityProjectionV1, error) {
	return runtimeports.ReviewEvidenceApplicabilityProjectionV1{}, core.NewError(core.ErrorInternal, core.ReasonInvalidState, "empty Evidence fixture must not inspect history")
}

func externalAuthorityV1(t *testing.T, id string, scope core.ExecutionScope, action core.Digest, expires time.Time) (runtimeports.AuthorityBindingRefV2, runtimeports.DispatchAuthorityFactV2) {
	t.Helper()
	fact := runtimeports.DispatchAuthorityFactV2{Ref: id, Revision: 1, Scope: scope, ActionScopeDigest: action, State: runtimeports.AuthorityFactActive, ExpiresUnixNano: expires.UnixNano()}
	fact.Digest = externalDigestV1(id)
	return runtimeports.AuthorityBindingRefV2{Ref: id, Revision: fact.Revision, Digest: fact.Digest, Epoch: scope.AuthorityEpoch}, fact
}

func sealExternalPolicyV1(t *testing.T, subject runtimeports.ReviewDecisionPolicyCurrentSubjectV1, fact runtimeports.ReviewPolicyFactV2, checked, expires time.Time) runtimeports.ReviewDecisionPolicyCurrentProjectionV1 {
	t.Helper()
	id, err := runtimeports.DeriveReviewDecisionPolicyCurrentProjectionIDV1(subject, fact.Ref)
	if err != nil {
		t.Fatal(err)
	}
	value := runtimeports.ReviewDecisionPolicyCurrentProjectionV1{ContractVersion: runtimeports.ReviewDecisionGovernanceCurrentContractVersionV1, Ref: runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1{ID: id, Revision: 1}, Subject: subject, Fact: fact, State: runtimeports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires.UnixNano()}
	digest, err := runtimeports.DigestReviewDecisionPolicyCurrentProjectionV1(value)
	if err != nil {
		t.Fatal(err)
	}
	value.ProjectionDigest = digest
	value.Ref.Digest = value.ProjectionDigest
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	return value
}

func sealExternalAuthorityV1(t *testing.T, subject runtimeports.ReviewDecisionAuthorityCurrentSubjectV1, fact runtimeports.DispatchAuthorityFactV2, checked, expires time.Time) runtimeports.ReviewDecisionAuthorityCurrentProjectionV1 {
	t.Helper()
	id, err := runtimeports.DeriveReviewDecisionAuthorityCurrentProjectionIDV1(subject, fact.Ref)
	if err != nil {
		t.Fatal(err)
	}
	value := runtimeports.ReviewDecisionAuthorityCurrentProjectionV1{ContractVersion: runtimeports.ReviewDecisionGovernanceCurrentContractVersionV1, Ref: runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1{ID: id, Revision: 1}, Subject: subject, Fact: fact, State: runtimeports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires.UnixNano()}
	digest, err := runtimeports.DigestReviewDecisionAuthorityCurrentProjectionV1(value)
	if err != nil {
		t.Fatal(err)
	}
	value.ProjectionDigest = digest
	value.Ref.Digest = value.ProjectionDigest
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	return value
}

func sealExternalScopeV1(t *testing.T, subject runtimeports.ReviewDecisionScopeCurrentSubjectV1, fact runtimeports.ExecutionScopeCurrentFactV2, checked, expires time.Time) runtimeports.ReviewDecisionScopeCurrentProjectionV1 {
	t.Helper()
	id, err := runtimeports.DeriveReviewDecisionScopeCurrentProjectionIDV1(subject, fact.Ref)
	if err != nil {
		t.Fatal(err)
	}
	value := runtimeports.ReviewDecisionScopeCurrentProjectionV1{ContractVersion: runtimeports.ReviewDecisionGovernanceCurrentContractVersionV1, Ref: runtimeports.ReviewDecisionScopeCurrentProjectionRefV1{ID: id, Revision: 1}, Subject: subject, Fact: fact, State: runtimeports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires.UnixNano()}
	digest, err := runtimeports.DigestReviewDecisionScopeCurrentProjectionV1(value)
	if err != nil {
		t.Fatal(err)
	}
	value.ProjectionDigest = digest
	value.Ref.Digest = value.ProjectionDigest
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	return value
}

func externalBindingStoreV1(t *testing.T, base time.Time, clock func() time.Time, tenant core.TenantID, target runtimeports.ReviewDecisionTargetRefV1, assignment runtimeports.ReviewDecisionAssignmentRefV1) (*fakes.ReviewBindingCurrentStoreV1, runtimeports.ReviewComponentBindingRefV2, runtimeports.ReviewBindingSubjectV1) {
	t.Helper()
	store := fakes.NewReviewBindingCurrentStoreV1(clock)
	sourceSet, sourceFact := commitExternalBindingComponentV1(t, store, base, "source-set", "source-binding", "review/auto-worker", "review/attest")
	consumerSet, consumerFact := commitExternalBindingComponentV1(t, store, base, "consumer-set", "consumer-binding", "review/verdict-owner", "runtime/read-review-binding-current")
	source := runtimeports.ReviewComponentBindingRefV2{BindingSetID: sourceSet.ID, BindingSetRevision: sourceSet.Revision, ComponentID: sourceFact.ComponentID, ManifestDigest: sourceFact.ManifestDigest, ArtifactDigest: sourceFact.Manifest.ArtifactDigest, Capability: "review/attest"}
	consumer := runtimeports.ProviderBindingRefV2{BindingSetID: consumerSet.ID, BindingSetRevision: consumerSet.Revision, ComponentID: consumerFact.ComponentID, ManifestDigest: consumerFact.ManifestDigest, ArtifactDigest: consumerFact.Manifest.ArtifactDigest, Capability: "runtime/read-review-binding-current"}
	association, err := runtimeports.SealReviewBindingConsumerAssociationCurrentProjectionV1(runtimeports.ReviewBindingConsumerAssociationCurrentProjectionV1{Ref: runtimeports.ReviewBindingConsumerAssociationRefV1{Revision: 1}, Consumer: consumer, Source: source, Current: true, CheckedUnixNano: base.UnixNano(), ExpiresUnixNano: base.Add(4 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateReviewBindingConsumerAssociationV1(context.Background(), association); err != nil {
		t.Fatal(err)
	}
	subject := runtimeports.ReviewBindingSubjectV1{TenantID: tenant, AssignmentID: assignment.ID, AssignmentRevision: assignment.Revision, AssignmentDigest: assignment.Digest, ReviewerID: assignment.ReviewerID, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest}
	input := runtimeports.CreateReviewBindingProjectionCommandInputV1{Source: source, Subject: subject, Association: association.Ref}
	publishRef, err := runtimeports.DeriveCreateReviewBindingProjectionPublishRefV1(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateReviewBindingProjectionV1(context.Background(), runtimeports.CreateReviewBindingProjectionRequestV1{PublishRef: publishRef, Input: input}); err != nil {
		t.Fatal(err)
	}
	return store, source, subject
}

func commitExternalBindingComponentV1(t *testing.T, store *fakes.ReviewBindingCurrentStoreV1, base time.Time, setID, bindingID string, component runtimeports.ComponentIDV2, capability runtimeports.CapabilityNameV2) (control.BindingSetFactV2, control.BindingFactV2) {
	t.Helper()
	artifact := externalDigestV1("artifact-" + string(component))
	manifest := runtimeports.ComponentManifestV2{ContractVersion: runtimeports.BindingContractVersionV2, ComponentID: component, Kind: "runtime/component", GovernanceCategory: "runtime/review", SemanticVersion: "1.0.0", ArtifactDigest: artifact, Contract: runtimeports.ContractBindingV2{Name: "runtime/review-binding", Version: "1.0.0", Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}, Schemas: []runtimeports.SchemaRefV2{}, Locality: runtimeports.LocalityHostControlPlane, Dependencies: []runtimeports.ComponentDependencyV2{}, RequiredCapabilities: []runtimeports.CapabilityRequirementV2{}, ProvidedCapabilities: []runtimeports.ProvidedCapabilityV2{{Capability: capability, TTLSeconds: 300, Schemas: []runtimeports.SchemaRefV2{}}}, Conformance: runtimeports.ConformanceFullyControlled, ResidualClass: runtimeports.ResidualInspectable, Owners: []runtimeports.OwnerAssignmentV2{{Role: runtimeports.OwnerEffect, OwnerComponentID: component}, {Role: runtimeports.OwnerSettlement, OwnerComponentID: component}, {Role: runtimeports.OwnerCleanup, OwnerComponentID: component}}, Credentials: []runtimeports.CredentialRequirementV2{}, OfflinePolicy: runtimeports.OfflineDenied, Extensions: []runtimeports.GovernanceExtensionV2{}, Annotations: []runtimeports.DisplayAnnotationV2{}}
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	governance := externalDigestV1("governance-" + setID)
	expires := base.Add(5 * time.Minute).UnixNano()
	grant := runtimeports.CapabilityGrantV2{Capability: capability, EvidenceDigest: externalDigestV1("grant-" + bindingID), ObservedUnixNano: base.UnixNano(), ExpiresUnixNano: expires}
	certified := control.BindingFactV2{ID: bindingID, ComponentID: component, Manifest: manifest, ManifestDigest: manifestDigest, GovernanceDigest: governance, State: control.BindingCertified, Revision: 3, Grants: []runtimeports.CapabilityGrantV2{grant}, ProbedUnixNano: base.UnixNano(), CertifiedUnixNano: base.Add(time.Second).UnixNano(), ConformanceEvidenceDigest: externalDigestV1("conformance-" + bindingID), ExpiresUnixNano: expires}
	declared := certified
	declared.State, declared.Revision, declared.Grants = control.BindingDeclared, 1, []runtimeports.CapabilityGrantV2{}
	declared.ProbedUnixNano, declared.CertifiedUnixNano, declared.ExpiresUnixNano, declared.ConformanceEvidenceDigest = 0, 0, 0, ""
	if _, err = store.CreateBinding(context.Background(), declared); err != nil {
		t.Fatal(err)
	}
	probed := certified
	probed.State, probed.Revision, probed.CertifiedUnixNano, probed.ConformanceEvidenceDigest = control.BindingProbed, 2, 0, ""
	if _, err = store.CompareAndSwapBinding(context.Background(), control.BindingFactCASRequestV2{ExpectedRevision: 1, Next: probed}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompareAndSwapBinding(context.Background(), control.BindingFactCASRequestV2{ExpectedRevision: 2, Next: certified}); err != nil {
		t.Fatal(err)
	}
	set := control.BindingSetFactV2{ID: setID, PlanID: "plan-" + setID, PlanDigest: externalDigestV1("plan-" + setID), GovernanceDigest: governance, State: control.BindingSetActive, Revision: 1, Members: []control.BindingMemberV2{{BindingID: bindingID, BindingRevision: certified.Revision, ComponentID: component, Kind: manifest.Kind, ManifestDigest: manifestDigest, ArtifactDigest: artifact, Contract: manifest.Contract, Owners: append([]runtimeports.OwnerAssignmentV2(nil), manifest.Owners...), Grants: []runtimeports.CapabilityGrantV2{grant}}}, TopologicalOrder: []runtimeports.ComponentIDV2{component}, Residuals: []control.BindingResidualV2{}, CreatedUnixNano: base.Add(time.Second).UnixNano(), ExpiresUnixNano: expires}
	committed, err := store.CommitBindingSet(context.Background(), control.CommitBindingSetRequestV2{Set: set, Expected: []control.ExpectedBindingRevisionV2{{BindingID: bindingID, ExpectedRevision: certified.Revision}}})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := store.InspectBinding(context.Background(), bindingID)
	if err != nil {
		t.Fatal(err)
	}
	return committed, bound
}

func externalGovernanceSourceV1(ref string) runtimeports.GovernanceSourceFactRefV2 {
	return runtimeports.GovernanceSourceFactRefV2{Ref: ref, Revision: 1, Digest: externalDigestV1(ref)}
}
func externalDigestV1(value string) core.Digest { return core.DigestBytes([]byte(value)) }
