package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationfakes "github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimekernel "github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRestoreExecutionCoordinatorV1LostStageReplyInspectsAndActivatesFreshInstance(t *testing.T) {
	fixture := newRestoreExecutionFixtureV1(t)
	fixture.stage.loseNext = true
	result, err := fixture.coordinator.ExecuteRestoreV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Activation.Attempt.ID != result.Attempt.ID || result.Activation.Attempt.Revision != result.Attempt.Revision+1 || result.Context.Fact.Identity.TargetInstance == result.Context.Fact.Identity.SourceInstance || result.Context.Fact.Identity.TargetInstance.Epoch <= result.Context.Fact.Identity.SourceInstance.Epoch {
		t.Fatalf("Restore did not activate the fresh reserved identity: %+v", result)
	}
	if fixture.stage.providerCalls != 1 || fixture.stage.inspectCalls != 1 || fixture.activation.calls != 1 {
		t.Fatalf("lost reply path did not Inspect-only: provider=%d inspect=%d activation=%d", fixture.stage.providerCalls, fixture.stage.inspectCalls, fixture.activation.calls)
	}
	if err := result.ValidateFor(fixture.request, fixture.now); err != nil {
		t.Fatal(err)
	}
	replayed, err := fixture.coordinator.ExecuteRestoreV1(context.Background(), fixture.request)
	if err != nil || replayed.Digest != result.Digest || fixture.stage.providerCalls != 1 {
		t.Fatalf("replay repeated Provider or changed result: replay=%+v err=%v provider=%d", replayed, err, fixture.stage.providerCalls)
	}
}

func TestRestoreExecutionCoordinatorV1LostActivationReplyInspectsOriginalAttempt(t *testing.T) {
	fixture := newRestoreExecutionFixtureV1(t)
	fixture.activation.loseNext = true
	result, err := fixture.coordinator.ExecuteRestoreV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.activation.calls != 1 || fixture.activation.inspectCalls != 1 {
		t.Fatalf("lost Activation reply did not Inspect-only: activate=%d inspect=%d", fixture.activation.calls, fixture.activation.inspectCalls)
	}
	if err := result.ValidateFor(fixture.request, fixture.now); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreExecutionCoordinatorV1ResultLostReplyReplayAndNoAlias(t *testing.T) {
	fixture := newRestoreExecutionFixtureV1(t)
	fixture.results.LoseNextReplyForTestV1()
	result, err := fixture.coordinator.ExecuteRestoreV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	result.Context.Fact.TargetFrames[0].ID = "mutated-caller-frame"
	replayed, err := fixture.coordinator.ExecuteRestoreV1(context.Background(), fixture.request)
	if err != nil || replayed.Context.Fact.TargetFrames[0].ID == "mutated-caller-frame" || fixture.stage.providerCalls != 1 || fixture.activation.calls != 1 {
		t.Fatalf("result lost-reply/replay/clone failed: replay=%+v err=%v provider=%d activation=%d", replayed, err, fixture.stage.providerCalls, fixture.activation.calls)
	}
}

func TestRestoreExecutionCoordinatorV1ConcurrentSingleStageAndResult(t *testing.T) {
	fixture := newRestoreExecutionFixtureV1(t)
	const workers = 64
	results := make(chan applicationcontract.RestoreExecutionResultV1, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := fixture.coordinator.ExecuteRestoreV1(context.Background(), fixture.request)
			results <- result
			errs <- err
		}()
	}
	wait.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var digest core.Digest
	for result := range results {
		if digest == "" {
			digest = result.Digest
		} else if result.Digest != digest {
			t.Fatal("concurrent Restore returned different results")
		}
	}
	if fixture.stage.providerCalls != 1 {
		t.Fatalf("concurrent Restore invoked Stage Provider %d times", fixture.stage.providerCalls)
	}
}

func TestRestoreExecutionCoordinatorV1ResidualAndTypedNilFailClosed(t *testing.T) {
	fixture := newRestoreExecutionFixtureV1(t)
	fixture.context.residual = true
	if _, err := fixture.coordinator.ExecuteRestoreV1(context.Background(), fixture.request); err == nil {
		t.Fatal("Context residual was accepted for Activation")
	}
	if fixture.activation.calls != 0 {
		t.Fatal("Activation was invoked despite Context residual")
	}
	var typedNil *restoreStageActionFakeV1
	if _, err := NewRestoreExecutionCoordinatorV1(RestoreExecutionCoordinatorConfigV1{Intents: fixture.results, Results: fixture.results, Restore: fixture.restore, Materialization: fixture.materialization, Stage: typedNil, Context: fixture.context, Activation: fixture.activation, Clock: func() time.Time { return fixture.now }}); err == nil {
		t.Fatal("typed-nil Restore Stage Action Port was accepted")
	}
}

func TestRestoreExecutionCoordinatorV1IntentLostReplyAndPrecedesRuntimeReservation(t *testing.T) {
	fixture := newRestoreExecutionFixtureV1(t)
	fixture.results.LoseNextIntentReplyForTestV1()
	result, err := fixture.coordinator.ExecuteRestoreV1(context.Background(), fixture.request)
	if err != nil || result.ValidateFor(fixture.request, fixture.now) != nil {
		t.Fatalf("Intent lost-reply recovery failed: result=%+v err=%v", result, err)
	}
	intent, err := fixture.results.InspectRestoreExecutionIntentV1(context.Background(), core.TenantID(fixture.request.RestorePlan.TenantID), fixture.request.ID)
	if err != nil || intent.RequestDigest != fixture.request.Digest || intent.ValidateCurrent(fixture.now) != nil {
		t.Fatalf("persisted Intent is not exact: intent=%+v err=%v", intent, err)
	}

	blocked := newRestoreExecutionFixtureV1(t)
	coordinator, err := NewRestoreExecutionCoordinatorV1(RestoreExecutionCoordinatorConfigV1{Intents: restoreExecutionIntentRejectV1{}, Results: blocked.results, Restore: blocked.restore, Materialization: blocked.materialization, Stage: blocked.stage, Context: blocked.context, Activation: blocked.activation, Clock: func() time.Time { return blocked.now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.ExecuteRestoreV1(context.Background(), blocked.request); err == nil {
		t.Fatal("Restore proceeded without durable Application Intent")
	}
	if _, err := blocked.restore.InspectRestoreAttemptV2(context.Background(), runtimeports.InspectRestoreAttemptRequestV2{TenantID: core.TenantID(blocked.request.RestorePlan.TenantID), AttemptID: blocked.request.RestoreAttemptID}); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("Runtime RestoreAttempt exists before Application Intent: %v", err)
	}
}

type restoreExecutionIntentRejectV1 struct{}

func (restoreExecutionIntentRejectV1) CreateRestoreExecutionIntentV1(context.Context, applicationcontract.RestoreExecutionIntentFactV1) (applicationcontract.RestoreExecutionIntentFactV1, error) {
	return applicationcontract.RestoreExecutionIntentFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Intent persistence failure")
}

func (restoreExecutionIntentRejectV1) InspectRestoreExecutionIntentV1(context.Context, core.TenantID, string) (applicationcontract.RestoreExecutionIntentFactV1, error) {
	return applicationcontract.RestoreExecutionIntentFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Intent absent")
}

type restoreExecutionFixtureV1 struct {
	now             time.Time
	plan            runtimeports.RestorePlanCurrentProjectionV2
	results         *applicationfakes.RestoreExecutionResultStoreV1
	store           *fakes.RestoreGovernanceStoreV2
	restore         runtimekernel.RestoreGovernanceGatewayV2
	materialization *restoreMaterializationReaderFakeV1
	stage           *restoreStageActionFakeV1
	context         *restoreContextPortFakeV1
	activation      *restoreActivationFakeV1
	coordinator     *RestoreExecutionCoordinatorV1
	request         applicationcontract.RestoreExecutionRequestV1
}

func newRestoreExecutionFixtureV1(t *testing.T) *restoreExecutionFixtureV1 {
	t.Helper()
	now := time.Unix(1_790_000_000, 0)
	plan, err := fakes.BuildRestorePlanCurrentFixtureV2("application-restore", now)
	if err != nil {
		t.Fatal(err)
	}
	store := fakes.NewRestoreGovernanceStoreV2()
	results := applicationfakes.NewRestoreExecutionResultStoreV1()
	restore := runtimekernel.RestoreGovernanceGatewayV2{Facts: store, Plans: restorePlanCurrentFakeV1{value: plan}, Inputs: restoreEligibilityInputsFakeV1{now: now}, Clock: func() time.Time { return now }}
	materialization := &restoreMaterializationReaderFakeV1{now: now, plan: plan}
	stage := &restoreStageActionFakeV1{now: now}
	contextPort := &restoreContextPortFakeV1{now: now}
	activation := &restoreActivationFakeV1{}
	coordinator, err := NewRestoreExecutionCoordinatorV1(RestoreExecutionCoordinatorConfigV1{Intents: results, Results: results, Restore: restore, Materialization: materialization, Stage: stage, Context: contextPort, Activation: activation, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	kinds := []applicationcontract.RestoreContextRequirementKindV1{applicationcontract.RestoreContextRequirementProfileV1, applicationcontract.RestoreContextRequirementToolV1, applicationcontract.RestoreContextRequirementMCPV1, applicationcontract.RestoreContextRequirementReviewV1, applicationcontract.RestoreContextRequirementAuthorityV1, applicationcontract.RestoreContextRequirementBudgetV1, applicationcontract.RestoreContextRequirementBindingV1}
	requirements := make([]applicationcontract.RestoreContextRequirementCoordinateV1, len(kinds))
	for index, kind := range kinds {
		requirements[index] = applicationcontract.RestoreContextRequirementCoordinateV1{Kind: kind, Ref: restoreExecutionExternalRefV1(core.TenantID(plan.RestorePlan.TenantID), plan.SourceScopeDigest, "requirement-"+string(kind), string(kind))}
	}
	request, err := applicationcontract.SealRestoreExecutionRequestV1(applicationcontract.RestoreExecutionRequestV1{
		ID: "restore-execution-application", IdempotencyKey: "restore-execution-key-application", RestorePlan: plan.RestorePlan,
		RestoreAttemptID: "restore-attempt-application", RestoreEligibilityID: "restore-eligibility-application",
		StageActionID: "restore-stage-action-application", StageIdempotencyKey: "restore-stage-action-key-application",
		ContextID: "restore-context-application", ContextIdempotencyKey: "restore-context-key-application", ActivationIdempotencyKey: "restore-activation-key-application",
		Requirements: requirements, EligibilityTTL: 2 * time.Minute, RequestedUnixNano: now.UnixNano(), NotAfterUnixNano: now.Add(4 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &restoreExecutionFixtureV1{now: now, plan: plan, results: results, store: store, restore: restore, materialization: materialization, stage: stage, context: contextPort, activation: activation, coordinator: coordinator, request: request}
}

type restorePlanCurrentFakeV1 struct {
	value runtimeports.RestorePlanCurrentProjectionV2
}

func (r restorePlanCurrentFakeV1) InspectRestorePlanCurrentV2(context.Context, runtimeports.CheckpointExternalExactFactRefV2) (runtimeports.RestorePlanCurrentProjectionV2, error) {
	return r.value, nil
}

type restoreEligibilityInputsFakeV1 struct{ now time.Time }

func (r restoreEligibilityInputsFakeV1) InspectRestoreEligibilityInputsCurrentV2(_ context.Context, attempt runtimeports.RestoreAttemptFactV2) (runtimeports.RestoreEligibilityInputsCurrentProjectionV2, error) {
	refs := func(kind string) []runtimeports.CheckpointExternalExactFactRefV2 {
		return []runtimeports.CheckpointExternalExactFactRefV2{restoreExecutionExternalRefV1(attempt.Ref.TenantID, attempt.OperationScope.SourceScopeDigest, "eligibility-"+kind, kind)}
	}
	return runtimeports.SealRestoreEligibilityInputsCurrentProjectionV2(runtimeports.RestoreEligibilityInputsCurrentProjectionV2{Attempt: attempt.Ref, OperationScopeDigest: attempt.OperationScope.Digest, SourceScopeDigest: attempt.OperationScope.SourceScopeDigest, ReviewTarget: runtimeports.OperationReviewTargetRefV4{Ref: "restore-review-target-application", Revision: 1, Digest: core.DigestBytes([]byte("restore-review-target-application"))}, ReviewRequirementRefs: refs("review"), PolicyBasisRefs: refs("policy"), AuthorityRequirementRefs: refs("authority"), ScopeRequirementRefs: refs("scope"), BudgetRequirementRefs: refs("budget"), BindingRequirementRefs: refs("binding"), ContextRequirementRefs: refs("context"), CheckedUnixNano: r.now.UnixNano(), ExpiresUnixNano: r.now.Add(3 * time.Minute).UnixNano()}, r.now)
}

type restoreMaterializationReaderFakeV1 struct {
	now  time.Time
	plan runtimeports.RestorePlanCurrentProjectionV2
}

func (r *restoreMaterializationReaderFakeV1) InspectRestoreMaterializationCurrentV1(_ context.Context, request runtimeports.InspectRestoreMaterializationCurrentRequestV1) (runtimeports.RestoreMaterializationCurrentProjectionV1, error) {
	ref := func(id, kind string) runtimeports.CheckpointExternalExactFactRefV2 {
		return restoreExecutionExternalRefV1(request.Attempt.TenantID, r.plan.SourceScopeDigest, id, kind)
	}
	return runtimeports.SealRestoreMaterializationCurrentProjectionV1(runtimeports.RestoreMaterializationCurrentProjectionV1{Attempt: request.Attempt, Eligibility: request.Eligibility, RestorePlan: r.plan.RestorePlan, Consistency: r.plan.CheckpointConsistency.Ref, ManifestSeal: r.plan.ManifestSeal, SourceScopeDigest: r.plan.SourceScopeDigest, Identity: r.plan.IdentityProposal, ContextGeneration: ref("source-context-generation", "context-generation"), ContextFrames: []runtimeports.CheckpointExternalExactFactRefV2{ref("source-context-frame", "context-frame")}, Memory: []runtimeports.CheckpointExternalExactFactRefV2{ref("source-memory", "memory")}, Knowledge: []runtimeports.CheckpointExternalExactFactRefV2{ref("source-knowledge", "knowledge")}, Snapshots: []runtimeports.CheckpointExternalExactFactRefV2{ref("source-snapshot", "snapshot")}, CheckedUnixNano: r.now.UnixNano(), ExpiresUnixNano: r.now.Add(time.Minute).UnixNano()}, r.now)
}

type restoreStageActionFakeV1 struct {
	mu            sync.Mutex
	now           time.Time
	result        *applicationcontract.RestoreStageActionResultV1
	loseNext      bool
	providerCalls int
	inspectCalls  int
}

func (s *restoreStageActionFakeV1) ExecuteRestoreStageActionV1(_ context.Context, request applicationcontract.RestoreStageActionRequestV1) (applicationcontract.RestoreStageActionResultV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.result != nil {
		return *s.result, nil
	}
	s.providerCalls++
	result, err := buildRestoreStageActionResultV1(request, s.now)
	if err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	s.result = &result
	if s.loseNext {
		s.loseNext = false
		return applicationcontract.RestoreStageActionResultV1{}, errors.New("injected Restore Stage lost reply")
	}
	return result, nil
}

func (s *restoreStageActionFakeV1) InspectRestoreStageActionV1(_ context.Context, key applicationcontract.RestoreStageActionInspectKeyV1) (applicationcontract.RestoreStageActionResultV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inspectCalls++
	if key.Validate() != nil || s.result == nil || s.result.RequestDigest != key.RequestDigest {
		return applicationcontract.RestoreStageActionResultV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Restore Stage result not found")
	}
	return *s.result, nil
}

func buildRestoreStageActionResultV1(request applicationcontract.RestoreStageActionRequestV1, now time.Time) (applicationcontract.RestoreStageActionResultV1, error) {
	identity := request.Materialization.Identity
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: request.Attempt.TenantID, ID: "identity-restore-stage-application", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-restore-stage-application", PlanDigest: core.DigestBytes([]byte("lineage-restore-stage-application"))}, Instance: identity.TargetInstance, SandboxLease: &identity.TargetLease, AuthorityEpoch: identity.TargetFenceEpoch}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.RestoreStageOperationKindV1, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, CustomOperationID: request.Attempt.ID, SubjectRevision: 1, CurrentProjectionRef: "restore-operation-application", CurrentProjectionDigest: core.DigestBytes([]byte("restore-operation-application")), CurrentProjectionRevision: 1}
	opDigest, _ := operation.DigestV3()
	dispatch := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: opDigest, EffectID: "restore-effect-application", IntentRevision: 1, IntentDigest: core.DigestBytes([]byte("restore-intent-application")), PermitID: "restore-permit-application", PermitRevision: 1, PermitDigest: core.DigestBytes([]byte("restore-permit-application")), AttemptID: "restore-dispatch-application"}
	domain := runtimeports.RestoreStageDomainResultFactRefV1{Owner: restoreExecutionProviderV1("praxis/sandbox", "sandbox/workspace-restore-stage"), Kind: runtimeports.RestoreStageDomainResultKindV1, ID: "restore-stage-domain-application", Revision: 1, Digest: core.DigestBytes([]byte("restore-stage-domain-application")), TenantID: request.Attempt.TenantID, Operation: operation, OperationDigest: opDigest, EffectID: dispatch.EffectID, EffectRevision: 1, Attempt: dispatch, RestoreAttempt: request.Attempt, Eligibility: request.Eligibility, PayloadSchema: runtimeports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "workspace-restore-stage", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("restore-stage-schema"))}, PayloadDigest: core.DigestBytes([]byte("restore-stage-payload")), PayloadRevision: 1, AuthoritativeTime: now.UnixNano()}
	stage, err := runtimeports.SealRestoreStageDomainResultCurrentProjectionV1(runtimeports.RestoreStageDomainResultCurrentProjectionV1{Fact: domain, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(45 * time.Second).UnixNano()}, now)
	if err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	settlement := runtimeports.RestoreStageSettlementRefV1{ID: "restore-runtime-settlement-application", Revision: 1, Digest: core.DigestBytes([]byte("restore-runtime-settlement-application")), OperationDigest: opDigest, EffectID: dispatch.EffectID, DomainResult: domain}
	apply := runtimeports.RestoreStageApplySettlementRefV1{Owner: restoreExecutionProviderV1("praxis/sandbox", "sandbox/workspace-restore-apply"), ID: "restore-sandbox-apply-application", Revision: 1, Digest: core.DigestBytes([]byte("restore-sandbox-apply-application")), TenantID: request.Attempt.TenantID, DomainResult: domain, RuntimeSettlement: settlement}
	sandbox, err := runtimeports.SealRestoreStageApplySettlementCurrentProjectionV1(runtimeports.RestoreStageApplySettlementCurrentProjectionV1{Fact: apply, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(45 * time.Second).UnixNano()}, now)
	if err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	return applicationcontract.SealRestoreStageActionResultV1(applicationcontract.RestoreStageActionResultV1{RequestDigest: request.Digest, Stage: stage, RuntimeSettlement: settlement, SandboxSettlement: sandbox, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(45 * time.Second).UnixNano()})
}

type restoreContextPortFakeV1 struct {
	now      time.Time
	residual bool
}

func (c *restoreContextPortFakeV1) MaterializeRestoreContextV1(_ context.Context, request applicationcontract.RestoreContextMaterializationRequestV1) (runtimeports.RestoreContextMaterializationCurrentProjectionV1, error) {
	targetScope := request.Stage.Fact.Operation.ExecutionScopeDigest
	targetGeneration := restoreExecutionExternalRefV1(request.Materialization.Attempt.TenantID, targetScope, "target-context-generation", "context-generation")
	targetFrame := restoreExecutionExternalRefV1(request.Materialization.Attempt.TenantID, targetScope, "target-context-frame", "context-frame")
	ref := runtimeports.RestoreContextMaterializationRefV1{Owner: restoreExecutionProviderV1("praxis/context", "context/restore-materialization"), ID: request.ID, Revision: 1, Digest: core.DigestBytes([]byte(request.ID)), TenantID: request.Materialization.Attempt.TenantID, Attempt: request.Materialization.Attempt, Eligibility: request.Materialization.Eligibility, Identity: request.Materialization.Identity, SourceScopeDigest: request.Materialization.SourceScopeDigest, TargetScopeDigest: targetScope, SourceGeneration: request.Materialization.ContextGeneration, TargetGeneration: targetGeneration, TargetFrames: []runtimeports.CheckpointExternalExactFactRefV2{targetFrame}, CurrentDigest: core.DigestBytes([]byte("restore-context-current"))}
	residuals := []runtimeports.CheckpointExternalExactFactRefV2(nil)
	if c.residual {
		residuals = []runtimeports.CheckpointExternalExactFactRefV2{restoreExecutionExternalRefV1(request.Materialization.Attempt.TenantID, targetScope, "restore-context-residual", "residual")}
	}
	return runtimeports.SealRestoreContextMaterializationCurrentProjectionV1(runtimeports.RestoreContextMaterializationCurrentProjectionV1{Fact: ref, Residuals: residuals, CheckedUnixNano: c.now.UnixNano(), ExpiresUnixNano: c.now.Add(30 * time.Second).UnixNano()}, c.now)
}

func (c *restoreContextPortFakeV1) InspectRestoreContextMaterializationV1(context.Context, runtimeports.RestoreContextMaterializationRefV1) (runtimeports.RestoreContextMaterializationCurrentProjectionV1, error) {
	return runtimeports.RestoreContextMaterializationCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "unused")
}

type restoreActivationFakeV1 struct {
	mu           sync.Mutex
	calls        int
	inspectCalls int
	loseNext     bool
	ref          runtimeports.RestoreActivationRefV1
	fact         runtimeports.RestoreActivationFactV1
}

func (a *restoreActivationFakeV1) ActivateRestoreV1(_ context.Context, submission runtimeports.RestoreActivationSubmissionV1) (runtimeports.RestoreActivationRefV1, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := submission.Validate(); err != nil {
		return runtimeports.RestoreActivationRefV1{}, err
	}
	a.calls++
	if a.ref == (runtimeports.RestoreActivationRefV1{}) {
		attempt := runtimeports.RestoreAttemptRefV2{TenantID: submission.Attempt.TenantID, ID: submission.Attempt.ID, Revision: submission.Attempt.Revision + 1, Digest: core.DigestBytes([]byte("restore-activated-attempt-application"))}
		fact, err := runtimeports.SealRestoreActivationFactV1(runtimeports.RestoreActivationFactV1{Ref: runtimeports.RestoreActivationRefV1{ID: "restore-activation-application", Attempt: attempt}, Submission: submission, Identity: submission.Context.Identity, ActivatedUnixNano: time.Unix(1_790_000_000, 0).UnixNano()})
		if err != nil {
			return runtimeports.RestoreActivationRefV1{}, err
		}
		a.fact = fact
		a.ref = fact.Ref
	}
	if a.loseNext {
		a.loseNext = false
		return runtimeports.RestoreActivationRefV1{}, errors.New("injected Restore Activation lost reply")
	}
	return a.ref, nil
}

func (a *restoreActivationFakeV1) InspectRestoreActivationV1(context.Context, runtimeports.RestoreActivationRefV1) (runtimeports.RestoreActivationFactV1, error) {
	return runtimeports.RestoreActivationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "unused")
}

func (a *restoreActivationFakeV1) InspectRestoreActivationByAttemptV1(context.Context, runtimeports.RestoreAttemptRefV2) (runtimeports.RestoreActivationFactV1, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.inspectCalls++
	if a.fact.Ref.ID == "" {
		return runtimeports.RestoreActivationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Restore Activation absent")
	}
	return a.fact, nil
}

func (a *restoreActivationFakeV1) InspectRestoreActivationByStableAttemptV1(_ context.Context, tenantID core.TenantID, attemptID string) (runtimeports.RestoreActivationFactV1, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.inspectCalls++
	if a.fact.Ref.ID == "" {
		return runtimeports.RestoreActivationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Restore Activation absent")
	}
	if a.fact.Ref.Attempt.TenantID != tenantID || a.fact.Ref.Attempt.ID != attemptID {
		return runtimeports.RestoreActivationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "Restore Activation stable Attempt drifted")
	}
	return a.fact, nil
}

func restoreExecutionProviderV1(component, capability string) runtimeports.ProviderBindingRefV2 {
	return runtimeports.ProviderBindingRefV2{BindingSetID: "binding-set-" + capability, BindingSetRevision: 1, ComponentID: runtimeports.ComponentIDV2(component), ManifestDigest: core.DigestBytes([]byte("manifest-" + capability)), ArtifactDigest: core.DigestBytes([]byte("artifact-" + capability)), Capability: runtimeports.CapabilityNameV2(capability)}
}

func restoreExecutionExternalRefV1(tenant core.TenantID, scope core.Digest, id, kind string) runtimeports.CheckpointExternalExactFactRefV2 {
	owner := restoreExecutionProviderV1("praxis/"+kind, kind+"/current")
	return runtimeports.CheckpointExternalExactFactRefV2{ContractVersion: "praxis.test/" + kind + "/v1", SchemaRef: "praxis.test/" + kind + "-fact/v1", Owner: runtimeports.CheckpointManifestSealOwnerBindingV2{BindingSetID: owner.BindingSetID, BindingRevision: owner.BindingSetRevision, ComponentID: string(owner.ComponentID), ManifestDigest: string(owner.ManifestDigest), ArtifactDigest: string(owner.ArtifactDigest), Capability: string(owner.Capability), FactKind: kind}, TenantID: string(tenant), ID: id, Revision: 1, Digest: string(core.DigestBytes([]byte(id))), ScopeDigest: string(scope)}
}
