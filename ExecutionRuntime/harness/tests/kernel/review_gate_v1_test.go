package kernel_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewGateV1AllowBindsExactActionTargetAndAuthorization(t *testing.T) {
	fixture := newReviewGateFixtureV1(t)
	result, err := fixture.controller(t).EvaluateReviewGateV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != contract.ReviewGateAllowV1 || result.Receipt.Authorization == nil || *result.Receipt.Authorization != fixture.fact.RefV5() || result.Receipt.Target != fixture.request.Target || result.Receipt.ActionRequestDigest != fixture.action.PendingAction.RequestDigest || result.Receipt.ReviewProjectionDigest != fixture.fact.Review.ProjectionDigest {
		t.Fatalf("unexpected allow result: %+v", result)
	}
	if fixture.auth.exactCalls.Load() != 2 || fixture.auth.currentCalls.Load() != 2 || fixture.actions.calls.Load() != 2 {
		t.Fatalf("S1/S2 reads = action:%d exact:%d current:%d", fixture.actions.calls.Load(), fixture.auth.exactCalls.Load(), fixture.auth.currentCalls.Load())
	}
}

func TestReviewGateV1FirstGateWithoutAuthorizationAsksAfterActionS1S2(t *testing.T) {
	fixture := newReviewGateFixtureV1(t)
	fixture.request.Authorization = nil
	result, err := fixture.controller(t).EvaluateReviewGateV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != contract.ReviewGateAskV1 || result.Receipt.Authorization != nil || result.Receipt.ErrorCategory != core.ErrorNotFound || result.Receipt.Reason != core.ReasonReviewVerdictMissing {
		t.Fatalf("unexpected first-review result: %+v", result)
	}
	if fixture.actions.calls.Load() != 2 || fixture.auth.exactCalls.Load() != 0 || fixture.auth.currentCalls.Load() != 0 {
		t.Fatalf("nil Authorization read behavior action=%d exact=%d current=%d", fixture.actions.calls.Load(), fixture.auth.exactCalls.Load(), fixture.auth.currentCalls.Load())
	}
}

func TestReviewGateV1FailClosedDecisionMatrix(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		decision contract.ReviewGatePhaseDecisionV1
	}{
		{"pending", core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "pending"), contract.ReviewGateAskV1},
		{"condition-unsatisfied", core.NewError(core.ErrorForbidden, core.ReasonReviewConditionUnsatisfied, "pending condition"), contract.ReviewGateAskV1},
		{"rejected", core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "rejected"), contract.ReviewGateDenyV1},
		{"expired", core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "expired"), contract.ReviewGateDeferV1},
		{"revoked", core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "revoked"), contract.ReviewGateDeferV1},
		{"superseded", core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "superseded"), contract.ReviewGateDeferV1},
		{"unknown", core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "unknown"), contract.ReviewGateDeferV1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newReviewGateFixtureV1(t)
			fixture.auth.currentErrors = []error{test.err, test.err}
			result, err := fixture.controller(t).EvaluateReviewGateV1(context.Background(), fixture.request)
			if err != nil {
				t.Fatal(err)
			}
			if result.Decision != test.decision || result.Receipt.ErrorCategory == "" || result.Receipt.Reason == "" {
				t.Fatalf("decision = %q receipt=%+v", result.Decision, result.Receipt)
			}
		})
	}
}

func TestReviewGateV1LostReadRetriesOnlySameExactInspectWithoutCanceledContext(t *testing.T) {
	fixture := newReviewGateFixtureV1(t)
	fixture.auth.exactErrors = []error{context.Canceled, nil}
	fixture.auth.currentErrors = []error{core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost"), nil}
	result, err := fixture.controller(t).EvaluateReviewGateV1(context.Background(), fixture.request)
	if err != nil || result.Decision != contract.ReviewGateAllowV1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if fixture.auth.exactCalls.Load() != 3 || fixture.auth.currentCalls.Load() != 3 || !fixture.auth.retryExactUncanceled.Load() || !fixture.auth.retryCurrentUncanceled.Load() {
		t.Fatalf("lost read recovery did not preserve exact uncancelled inspect: exact=%d current=%d", fixture.auth.exactCalls.Load(), fixture.auth.currentCalls.Load())
	}
	for _, ref := range fixture.auth.refs {
		if ref != fixture.fact.RefV5() {
			t.Fatalf("exact retry changed ref: %+v", ref)
		}
	}
}

func TestReviewGateV1CurrentDriftTTLAndClockRollbackNeverAllow(t *testing.T) {
	t.Run("current-drift", func(t *testing.T) {
		fixture := newReviewGateFixtureV1(t)
		drift := fixture.fact
		drift.Revision++
		drift.Digest = testkit.Digest("drift")
		fixture.auth.currentValues = []runtimeports.OperationReviewAuthorizationFactV5{fixture.fact, drift}
		result, err := fixture.controller(t).EvaluateReviewGateV1(context.Background(), fixture.request)
		if err != nil || result.Decision != contract.ReviewGateDeferV1 {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	})
	t.Run("ttl-crossing", func(t *testing.T) {
		fixture := newReviewGateFixtureV1(t)
		expires := time.Unix(0, fixture.fact.ExpiresUnixNano)
		fixture.clock = &reviewGateClockV1{values: []time.Time{fixture.now, fixture.now.Add(time.Millisecond), expires}}
		result, err := fixture.controller(t).EvaluateReviewGateV1(context.Background(), fixture.request)
		if !core.HasReason(err, core.ReasonReviewVerdictStale) || result != (contract.ReviewGateResultV1{}) {
			t.Fatalf("TTL crossing emitted or extended a receipt: result=%+v err=%v", result, err)
		}
	})
	t.Run("rollback", func(t *testing.T) {
		fixture := newReviewGateFixtureV1(t)
		fixture.clock = &reviewGateClockV1{values: []time.Time{fixture.now.Add(time.Second), fixture.now}}
		if _, err := fixture.controller(t).EvaluateReviewGateV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("expected clock regression, got %v", err)
		}
	})
}

func TestReviewGateV1Concurrent64IsReadOnlyAndDeterministic(t *testing.T) {
	fixture := newReviewGateFixtureV1(t)
	controller := fixture.controller(t)
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := controller.EvaluateReviewGateV1(context.Background(), fixture.request)
			if err != nil {
				errs <- err
				return
			}
			if result.Decision != contract.ReviewGateAllowV1 {
				errs <- core.NewError(core.ErrorInternal, core.ReasonReviewVerdictStale, "concurrent Gate did not allow")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if fixture.actions.calls.Load() != 128 || fixture.auth.exactCalls.Load() != 128 || fixture.auth.currentCalls.Load() != 128 {
		t.Fatalf("unexpected read counts action=%d exact=%d current=%d", fixture.actions.calls.Load(), fixture.auth.exactCalls.Load(), fixture.auth.currentCalls.Load())
	}
}

func TestReviewGateV1ConstructorsRejectTypedNil(t *testing.T) {
	var actions *reviewGateActionReaderV1
	var authorizations *reviewGateAuthorizationReaderV1
	if _, err := kernel.NewReviewGateControllerV1(actions, &reviewGateAuthorizationReaderV1{}, time.Now); err == nil {
		t.Fatal("typed-nil action reader accepted")
	}
	if _, err := kernel.NewReviewGateControllerV1(&reviewGateActionReaderV1{}, authorizations, time.Now); err == nil {
		t.Fatal("typed-nil authorization reader accepted")
	}
}

func TestReviewGateV1ReusableConformanceDoesNotGrantOwnerAuthority(t *testing.T) {
	fixture := newReviewGateFixtureV1(t)
	report, err := conformance.CheckReviewGateV1(context.Background(), conformance.ReviewGateCaseV1{Gate: fixture.controller(t), Request: fixture.request, Parallelism: 64})
	if err != nil {
		t.Fatal(err)
	}
	if !report.ConcurrentExact || !report.ExactRefDriftFailClosed || !report.ActionDriftFailClosed || !report.TargetDriftFailClosed || !report.ReceiptObservationOnly || report.VerdictAuthority || report.AuthorizationAuthority || report.DispatchAuthority || report.ProductionRootProven {
		t.Fatalf("unexpected conformance report: %+v", report)
	}
}

type reviewGateFixtureV1 struct {
	now     time.Time
	request contract.ReviewGateRequestV1
	action  contract.CommittedPendingActionCurrentV3
	fact    runtimeports.OperationReviewAuthorizationFactV5
	actions *reviewGateActionReaderV1
	auth    *reviewGateAuthorizationReaderV1
	clock   *reviewGateClockV1
}

func newReviewGateFixtureV1(t *testing.T) *reviewGateFixtureV1 {
	t.Helper()
	base := newPendingActionReaderV3Fixture(t, pendingActionReaderV3Options{callerTTL: 20 * time.Second})
	action, err := base.newReader(t).InspectCommittedPendingActionCurrentV3(context.Background(), base.request)
	mustNoErrorV3(t, err)
	now := base.now.Add(2 * time.Second)
	intent, target := reviewGateIntentV1(t, now, action)
	fact := reviewGateAuthorizationFactV1(t, now, intent, target)
	authorization := fact.RefV5()
	request := contract.ReviewGateRequestV1{ContractVersion: contract.ReviewGateContractVersionV1, Action: base.request, Target: target, Intent: intent, Authorization: &authorization, Basis: runtimeports.OperationReviewBasisAcceptedQuorumV5, RequestedNotAfterUnixNano: base.request.RequestedNotAfterUnixNano}
	mustNoErrorV3(t, request.Validate(now))
	return &reviewGateFixtureV1{now: now, request: request, action: action, fact: fact, actions: &reviewGateActionReaderV1{value: action}, auth: &reviewGateAuthorizationReaderV1{exact: fact, current: fact}, clock: &reviewGateClockV1{values: []time.Time{now, now.Add(time.Millisecond), now.Add(2 * time.Millisecond)}}}
}

func (f *reviewGateFixtureV1) controller(t *testing.T) *kernel.ReviewGateControllerV1 {
	t.Helper()
	controller, err := kernel.NewReviewGateControllerV1(f.actions, f.auth, f.clock.Now)
	mustNoErrorV3(t, err)
	return controller
}

func reviewGateIntentV1(t *testing.T, now time.Time, action contract.CommittedPendingActionCurrentV3) (runtimeports.OperationEffectIntentV3, runtimeports.OperationReviewTargetRefV4) {
	t.Helper()
	prepare, _, _, _ := testkit.GovernedProviderFixtureV2(now)
	intent := prepare.Intent
	intent.ID = "effect-review-gate-v1"
	intent.Operation = action.ApplicationBinding.OwnerCurrentInputs.ModelTurnOperation
	intent.Operation.SubjectRevision++
	intent.Operation.CurrentProjectionRef = "review-gate-action-operation-current"
	intent.Operation.CurrentProjectionRevision = 1
	intent.Operation.CurrentProjectionDigest = testkit.Digest("review-gate-action-operation-current")
	intent.Payload = action.PendingAction.Payload
	intent.Kind = "praxis.tool/execute"
	intent.RiskClass = "praxis.review/gated"
	intent.Target = action.PendingAction.Ref
	intent.ActionScopeDigest = testkit.Digest("review-gate-action-scope")
	intent.Provider.BindingSetID = "review-gate-binding"
	intent.Provider.BindingSetRevision = 1
	intent.Provider.ComponentID = "praxis.tool/provider"
	intent.Provider.Capability = "praxis.tool/execute"
	intent.Provider.ManifestDigest = testkit.Digest("review-gate-provider-manifest")
	intent.Provider.ArtifactDigest = testkit.Digest("review-gate-provider-artifact")
	intent.Authority = runtimeports.AuthorityBindingRefV2{Ref: "review-gate-authority", Revision: 1, Digest: testkit.Digest("review-gate-authority"), Epoch: intent.Operation.ExecutionScope.AuthorityEpoch}
	target := runtimeports.OperationReviewTargetRefV4{Ref: intent.Target, Revision: 1, Digest: testkit.Digest("review-gate-target")}
	intent.Review = runtimeports.OperationReviewBindingRefV3{CaseRef: "review-gate-case", CandidateRevision: target.Revision, CandidateDigest: target.Digest, PolicyDigest: testkit.Digest("review-gate-review-policy")}
	operationDigest, err := intent.Operation.DigestV3()
	mustNoErrorV3(t, err)
	intent.Budget = runtimeports.OperationBudgetBindingRefV3{Ref: "review-gate-budget", Revision: 1, Digest: testkit.Digest("review-gate-budget"), PolicyDigest: testkit.Digest("review-gate-budget-policy"), SubjectDigest: operationDigest}
	intent.Policy = runtimeports.OperationPolicyBindingRefV3{Ref: "review-gate-policy", Revision: 1, Digest: testkit.Digest("review-gate-policy"), SubjectDigest: operationDigest}
	intent.ConflictDomain = runtimeports.ConflictDomainBindingV2{Domain: "praxis.tool/execute", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(intent.Operation.ExecutionScope.Identity.TenantID)}
	intent.Idempotency = runtimeports.IdempotencyBindingV2{Key: "review-gate-effect", ScopeClass: runtimeports.EffectStableScopeTenantV2, ScopeDigest: runtimeports.StableTenantScopeDigestV2(intent.Operation.ExecutionScope.Identity.TenantID), Class: core.IdempotencyQueryable}
	intent.Owners = []runtimeports.EffectOwnerRefV2{{Role: runtimeports.OwnerCleanup, ComponentID: intent.Provider.ComponentID, ManifestDigest: intent.Provider.ManifestDigest}, {Role: runtimeports.OwnerEffect, ComponentID: intent.Provider.ComponentID, ManifestDigest: intent.Provider.ManifestDigest}, {Role: runtimeports.OwnerSettlement, ComponentID: intent.Provider.ComponentID, ManifestDigest: intent.Provider.ManifestDigest}}
	intent.CredentialLeases = []runtimeports.CredentialLeaseRefV2{}
	intent.ExpiresUnixNano = now.Add(8 * time.Second).UnixNano()
	mustNoErrorV3(t, intent.Validate())
	return intent, target
}

func reviewGateAuthorizationFactV1(t *testing.T, now time.Time, intent runtimeports.OperationEffectIntentV3, target runtimeports.OperationReviewTargetRefV4) runtimeports.OperationReviewAuthorizationFactV5 {
	t.Helper()
	expires := intent.ExpiresUnixNano
	intentDigest, err := intent.DigestV3()
	mustNoErrorV3(t, err)
	ref := func(id string, digest core.Digest) runtimeports.OperationGovernanceFactRefV3 {
		return runtimeports.OperationGovernanceFactRefV3{Ref: id, Revision: 1, Digest: digest, ExpiresUnixNano: expires}
	}
	governance := runtimeports.OperationReviewGovernanceBindingV4{SnapshotDigest: testkit.Digest("review-gate-snapshot"), ProjectionWatermark: 1, Identity: ref("review-gate-identity", testkit.Digest("review-gate-identity")), Binding: ref(intent.Provider.BindingSetID, testkit.Digest("review-gate-binding")), CurrentScope: ref(intent.Operation.CurrentProjectionRef, intent.Operation.CurrentProjectionDigest), Authority: ref(intent.Authority.Ref, intent.Authority.Digest), Policy: ref(intent.Policy.Ref, intent.Policy.Digest), Budget: ref(intent.Budget.Ref, intent.Budget.Digest), CapabilityGrantDigest: testkit.Digest("review-gate-capability"), CredentialGrantDigest: testkit.Digest("review-gate-credentials"), ExpiresUnixNano: expires}
	tenant := intent.Operation.ExecutionScope.Identity.TenantID
	evidence := []runtimeports.EvidenceRecordRefV2{{LedgerScopeDigest: testkit.Digest("review-gate-ledger"), Sequence: 1, RecordDigest: testkit.Digest("review-gate-evidence")}}
	quorum, err := runtimeports.SealOperationReviewQuorumCurrentProjectionV5(runtimeports.OperationReviewQuorumCurrentProjectionV5{Operation: intent.Operation, IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest, PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision, Target: target, Case: runtimeports.OperationReviewCaseRefV5{TenantID: tenant, ID: intent.Review.CaseRef, Revision: 1, Digest: testkit.Digest("review-gate-case"), ExpiresUnixNano: expires}, Panel: runtimeports.OperationReviewPanelRefV5{TenantID: tenant, ID: "review-gate-panel", Revision: 1, Digest: testkit.Digest("review-gate-panel"), ExpiresUnixNano: expires}, QuorumDecision: runtimeports.OperationReviewQuorumDecisionRefV5{TenantID: tenant, ID: "review-gate-quorum", Revision: 1, Digest: testkit.Digest("review-gate-quorum"), ExpiresUnixNano: expires}, Verdict: runtimeports.OperationReviewVerdictRefV5{TenantID: tenant, ID: "review-gate-verdict", Revision: 1, Digest: testkit.Digest("review-gate-verdict"), ExpiresUnixNano: expires}, QuorumPolicy: ref("review-gate-quorum-policy", intent.Review.PolicyDigest), ReviewerSetDigest: testkit.Digest("review-gate-reviewers"), AcceptCount: 2, Threshold: 2, SatisfiedRoleCounts: []runtimeports.OperationReviewRoleCountV5{{Role: "security", Count: 1, Required: 1}}, ReviewerAuthorityRefs: []runtimeports.OperationGovernanceFactRefV3{ref("review-gate-reviewer-a", testkit.Digest("review-gate-reviewer-a")), ref("review-gate-reviewer-b", testkit.Digest("review-gate-reviewer-b"))}, BindingRefs: []runtimeports.OperationGovernanceFactRefV3{ref("review-gate-review-binding", testkit.Digest("review-gate-review-binding"))}, ScopeRef: governance.CurrentScope, DecisionEvidence: evidence, Basis: runtimeports.OperationReviewBasisAcceptedQuorumV5, Current: true, CurrentnessDigest: testkit.Digest("review-gate-current"), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}, now)
	mustNoErrorV3(t, err)
	review, err := runtimeports.SealOperationReviewCurrentProjectionV5(runtimeports.OperationReviewCurrentProjectionV5{Basis: runtimeports.OperationReviewBasisAcceptedQuorumV5, Quorum: &quorum}, now)
	mustNoErrorV3(t, err)
	binding := runtimeports.OperationReviewIntentBindingV4{Operation: intent.Operation, IntentID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest, EffectFactRevision: 1, Target: intent.Target, PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision, Provider: intent.Provider, Authority: intent.Authority, ReviewBinding: intent.Review, DispatchPolicy: intent.Policy, IntentExpires: intent.ExpiresUnixNano}
	fence := core.ExecutionFence{BoundaryScope: core.FenceBoundaryInstance, Scope: intent.Operation.ExecutionScope, CapabilityGrantDigest: governance.CapabilityGrantDigest, EffectIntentID: intent.ID, EffectIntentRevision: intent.Revision, CanonicalPayloadDigest: intent.Payload.ContentDigest, ExpiresAt: time.Unix(0, expires)}
	fenceDigest, err := runtimeports.DigestOperationExecutionFenceV3(fence, intent.Operation)
	mustNoErrorV3(t, err)
	fact, err := runtimeports.SealOperationReviewAuthorizationFactV5(runtimeports.OperationReviewAuthorizationFactV5{ID: "review-gate-authorization", Revision: 1, State: runtimeports.OperationReviewAuthorizationActiveV5, Intent: binding, Review: review, Governance: governance, Fence: fence, FenceDigest: fenceDigest, RequestedTTLUnixNano: expires - now.UnixNano(), CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	mustNoErrorV3(t, err)
	return fact
}

type reviewGateActionReaderV1 struct {
	value contract.CommittedPendingActionCurrentV3
	calls atomic.Int64
}

func (r *reviewGateActionReaderV1) InspectCommittedPendingActionCurrentV3(context.Context, contract.CommittedPendingActionCurrentRequestV3) (contract.CommittedPendingActionCurrentV3, error) {
	r.calls.Add(1)
	return r.value.Clone(), nil
}

type reviewGateAuthorizationReaderV1 struct {
	mu                     sync.Mutex
	exact                  runtimeports.OperationReviewAuthorizationFactV5
	current                runtimeports.OperationReviewAuthorizationFactV5
	exactValues            []runtimeports.OperationReviewAuthorizationFactV5
	currentValues          []runtimeports.OperationReviewAuthorizationFactV5
	exactErrors            []error
	currentErrors          []error
	refs                   []runtimeports.OperationReviewAuthorizationRefV5
	exactCalls             atomic.Int64
	currentCalls           atomic.Int64
	retryExactUncanceled   atomic.Bool
	retryCurrentUncanceled atomic.Bool
}

func (r *reviewGateAuthorizationReaderV1) InspectOperationReviewAuthorizationExactV5(ctx context.Context, ref runtimeports.OperationReviewAuthorizationRefV5) (runtimeports.OperationReviewAuthorizationFactV5, error) {
	r.exactCalls.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refs = append(r.refs, ref)
	if len(r.exactErrors) > 0 {
		err := r.exactErrors[0]
		r.exactErrors = r.exactErrors[1:]
		if err != nil {
			return runtimeports.OperationReviewAuthorizationFactV5{}, err
		}
		r.retryExactUncanceled.Store(ctx.Err() == nil)
	}
	if len(r.exactValues) > 0 {
		value := r.exactValues[0]
		r.exactValues = r.exactValues[1:]
		return value, nil
	}
	return r.exact, nil
}

func (r *reviewGateAuthorizationReaderV1) InspectCurrentOperationReviewAuthorizationV5(ctx context.Context, operation runtimeports.OperationSubjectV3, effect core.EffectIntentID, id string) (runtimeports.OperationReviewAuthorizationFactV5, error) {
	r.currentCalls.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if !runtimeports.SameOperationSubjectV3(operation, r.current.Intent.Operation) || effect != r.current.Intent.IntentID || id != r.current.ID {
		return runtimeports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "current read request drifted")
	}
	if len(r.currentErrors) > 0 {
		err := r.currentErrors[0]
		r.currentErrors = r.currentErrors[1:]
		if err != nil {
			return runtimeports.OperationReviewAuthorizationFactV5{}, err
		}
		r.retryCurrentUncanceled.Store(ctx.Err() == nil)
	}
	if len(r.currentValues) > 0 {
		value := r.currentValues[0]
		r.currentValues = r.currentValues[1:]
		return value, nil
	}
	return r.current, nil
}

type reviewGateClockV1 struct {
	mu     sync.Mutex
	values []time.Time
	index  int
}

func (c *reviewGateClockV1) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.values) == 0 {
		return time.Time{}
	}
	if c.index >= len(c.values) {
		return c.values[len(c.values)-1]
	}
	value := c.values[c.index]
	c.index++
	return value
}
