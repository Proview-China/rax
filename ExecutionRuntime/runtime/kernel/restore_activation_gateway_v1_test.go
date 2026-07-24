package kernel

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRestoreActivationGatewayV1AtomicLostReplyAndConcurrent(t *testing.T) {
	fixture := newRestoreActivationFixtureV1(t)
	fixture.store.LoseNextRestoreReplyV2()
	const workers = 64
	refs := make(chan ports.RestoreActivationRefV1, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ref, err := fixture.gateway.ActivateRestoreV1(context.Background(), fixture.submission)
			refs <- ref
			errs <- err
		}()
	}
	wait.Wait()
	close(refs)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent exact Activation failed: %v", err)
		}
	}
	var winner ports.RestoreActivationRefV1
	for ref := range refs {
		if winner == (ports.RestoreActivationRefV1{}) {
			winner = ref
		} else if ref != winner {
			t.Fatal("concurrent Activation returned different exact refs")
		}
	}
	current, err := fixture.restore.InspectRestoreAttemptV2(context.Background(), ports.InspectRestoreAttemptRequestV2{TenantID: fixture.submission.Attempt.TenantID, AttemptID: fixture.submission.Attempt.ID})
	if err != nil || current.State != ports.RestoreAttemptActivatedV2 || current.Ref != winner.Attempt {
		t.Fatalf("Activation did not atomically publish activated Attempt: current=%+v err=%v", current, err)
	}
	stable, err := fixture.gateway.InspectRestoreActivationByStableAttemptV1(context.Background(), fixture.submission.Attempt.TenantID, fixture.submission.Attempt.ID)
	if err != nil || stable.Validate() != nil || stable.Ref != winner || stable.Submission.IdempotencyKey != fixture.submission.IdempotencyKey {
		t.Fatalf("stable lost-reply Inspect returned another Activation: fact=%+v err=%v", stable, err)
	}
	if _, err := fixture.gateway.InspectRestoreActivationByStableAttemptV1(context.Background(), "other-tenant", fixture.submission.Attempt.ID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("cross-tenant stable Inspect did not fail closed: %v", err)
	}
}

func TestRestoreActivationGatewayV1RejectsResidualStaleAndSplice(t *testing.T) {
	for name, mutate := range map[string]func(*restoreActivationFixtureV1){
		"context residual": func(f *restoreActivationFixtureV1) {
			f.context.value.Residuals = []ports.CheckpointExternalExactFactRefV2{restoreActivationExternalRefV1(f.submission.Attempt.TenantID, f.submission.Context.TargetScopeDigest, "residual", "residual")}
			f.context.value, _ = ports.SealRestoreContextMaterializationCurrentProjectionV1(f.context.value, f.now)
		},
		"context scope": func(f *restoreActivationFixtureV1) {
			f.submission.Context.TargetScopeDigest = core.DigestBytes([]byte("other-target-scope"))
		},
		"sandbox settlement": func(f *restoreActivationFixtureV1) {
			f.submission.SandboxSettlement.Digest = core.DigestBytes([]byte("other-apply"))
		},
		"expired eligibility": func(f *restoreActivationFixtureV1) {
			f.now = time.Unix(0, f.submission.Eligibility.ExpiresUnixNano)
			f.gateway.Clock = func() time.Time { return f.now }
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newRestoreActivationFixtureV1(t)
			mutate(&fixture)
			if _, err := fixture.gateway.ActivateRestoreV1(context.Background(), fixture.submission); err == nil {
				t.Fatal("unsafe Restore Activation was accepted")
			}
			current, err := fixture.restore.InspectRestoreAttemptV2(context.Background(), ports.InspectRestoreAttemptRequestV2{TenantID: fixture.submission.Attempt.TenantID, AttemptID: fixture.submission.Attempt.ID})
			if err != nil || current.State == ports.RestoreAttemptActivatedV2 {
				t.Fatalf("failed Activation changed Attempt: current=%+v err=%v", current, err)
			}
		})
	}
}

func TestRestoreActivationSubmissionV1BindsTargetScopeAndDefersSourceToAttempt(t *testing.T) {
	for name, mutate := range map[string]func(*ports.RestoreActivationSubmissionV1){
		"target instance": func(submission *ports.RestoreActivationSubmissionV1) {
			submission.Context.Identity.TargetInstance.ID = "other-target-instance"
		},
		"target lease": func(submission *ports.RestoreActivationSubmissionV1) {
			submission.Context.Identity.TargetLease.ID = "other-target-lease"
		},
		"target fence": func(submission *ports.RestoreActivationSubmissionV1) {
			submission.Context.Identity.TargetFenceEpoch++
		},
		"target scope digest": func(submission *ports.RestoreActivationSubmissionV1) {
			digest := core.DigestBytes([]byte("other-target-scope"))
			submission.Context.TargetScopeDigest = digest
			submission.Context.TargetGeneration.ScopeDigest = string(digest)
			for index := range submission.Context.TargetFrames {
				submission.Context.TargetFrames[index].ScopeDigest = string(digest)
			}
		},
		"missing target lease": func(submission *ports.RestoreActivationSubmissionV1) {
			submission.Stage.Operation.ExecutionScope.SandboxLease = nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newRestoreActivationFixtureV1(t)
			mutate(&fixture.submission)
			if err := fixture.submission.Validate(); err == nil {
				t.Fatal("Restore Activation submission accepted a drifted target scope")
			}
		})
	}

	fixture := newRestoreActivationFixtureV1(t)
	fixture.submission.Context.Identity.SourceInstance.ID = "other-source-instance"
	if err := fixture.submission.Validate(); err != nil {
		t.Fatalf("submission-local target closure incorrectly interpreted SourceInstance: %v", err)
	}
	if _, err := fixture.gateway.ActivateRestoreV1(context.Background(), fixture.submission); err == nil {
		t.Fatal("Restore Activation gateway accepted a SourceInstance that differed from the current Attempt")
	}
}

type restoreActivationFixtureV1 struct {
	now        time.Time
	store      *fakes.RestoreGovernanceStoreV2
	restore    RestoreGovernanceGatewayV2
	stage      *restoreActivationStageReaderV1
	settlement *restoreActivationSettlementReaderV1
	sandbox    *restoreActivationSandboxReaderV1
	context    *restoreActivationContextReaderV1
	gateway    RestoreActivationGatewayV1
	submission ports.RestoreActivationSubmissionV1
}

func newRestoreActivationFixtureV1(t *testing.T) restoreActivationFixtureV1 {
	t.Helper()
	now := time.Unix(1_770_000_000, 0)
	plan, err := fakes.BuildRestorePlanCurrentFixtureV2("activation", now)
	if err != nil {
		t.Fatal(err)
	}
	store := fakes.NewRestoreGovernanceStoreV2()
	restore := RestoreGovernanceGatewayV2{Facts: store, Plans: restoreActivationPlanReaderV1{value: plan}, Inputs: restoreActivationInputsReaderV1{now: now}, Clock: func() time.Time { return now }}
	reserved, err := restore.CreateRestoreAttemptV2(context.Background(), ports.CreateRestoreAttemptRequestV2{AttemptID: "restore-attempt-activation", IdempotencyKey: "restore-attempt-activation-key", RestorePlan: plan.RestorePlan, RequestedNotAfter: now.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := restore.IssueRestoreEligibilityV2(context.Background(), ports.IssueRestoreEligibilityRequestV2{EligibilityID: "restore-eligibility-activation", Attempt: reserved.Ref, RequestedTTL: 3 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	operation := restoreActivationOperationV1(t, bundle.Attempt.OperationScope.Identity, bundle.Attempt.Ref)
	opDigest, _ := operation.DigestV3()
	dispatch := ports.OperationDispatchAttemptRefV3{OperationDigest: opDigest, EffectID: "restore-stage-effect-activation", IntentRevision: 1, IntentDigest: core.DigestBytes([]byte("intent-activation")), PermitID: "restore-permit-activation", PermitRevision: 1, PermitDigest: core.DigestBytes([]byte("permit-activation")), AttemptID: "restore-dispatch-activation"}
	domain := ports.RestoreStageDomainResultFactRefV1{
		Owner: restoreActivationOwnerV1("praxis/sandbox", "restore/workspace-stage"), Kind: ports.RestoreStageDomainResultKindV1,
		ID: "restore-stage-domain-activation", Revision: 1, Digest: core.DigestBytes([]byte("restore-stage-domain-activation")), TenantID: bundle.Attempt.Ref.TenantID,
		Operation: operation, OperationDigest: opDigest, EffectID: dispatch.EffectID, EffectRevision: 1, Attempt: dispatch,
		RestoreAttempt: bundle.Attempt.Ref, Eligibility: bundle.Eligibility.Ref,
		PayloadSchema: ports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "workspace-restore-stage", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("workspace-restore-stage-schema"))}, PayloadDigest: core.DigestBytes([]byte("stage-payload")), PayloadRevision: 1, AuthoritativeTime: now.UnixNano(),
	}
	if err := domain.Validate(); err != nil {
		t.Fatal(err)
	}
	stageProjection, err := ports.SealRestoreStageDomainResultCurrentProjectionV1(ports.RestoreStageDomainResultCurrentProjectionV1{Fact: domain, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(2 * time.Minute).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	settlement := ports.RestoreStageSettlementRefV1{ID: "restore-stage-settlement-activation", Revision: 1, Digest: core.DigestBytes([]byte("restore-stage-settlement-activation")), OperationDigest: opDigest, EffectID: dispatch.EffectID, DomainResult: domain}
	apply := ports.RestoreStageApplySettlementRefV1{Owner: restoreActivationOwnerV1("praxis/sandbox", "restore/workspace-apply"), ID: "restore-stage-apply-activation", Revision: 1, Digest: core.DigestBytes([]byte("restore-stage-apply-activation")), TenantID: bundle.Attempt.Ref.TenantID, DomainResult: domain, RuntimeSettlement: settlement}
	applyProjection, err := ports.SealRestoreStageApplySettlementCurrentProjectionV1(ports.RestoreStageApplySettlementCurrentProjectionV1{Fact: apply, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(2 * time.Minute).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	targetScope := operation.ExecutionScopeDigest
	sourceGeneration := restoreActivationExternalRefV1(bundle.Attempt.Ref.TenantID, plan.SourceScopeDigest, "source-generation", "context-generation")
	targetGeneration := restoreActivationExternalRefV1(bundle.Attempt.Ref.TenantID, targetScope, "target-generation", "context-generation")
	contextRef := ports.RestoreContextMaterializationRefV1{Owner: restoreActivationOwnerV1("praxis/context", "restore/context-materialization"), ID: "restore-context-materialization-activation", Revision: 1, Digest: core.DigestBytes([]byte("restore-context-materialization-activation")), TenantID: bundle.Attempt.Ref.TenantID, Attempt: bundle.Attempt.Ref, Eligibility: bundle.Eligibility.Ref, Identity: bundle.Attempt.OperationScope.Identity, SourceScopeDigest: plan.SourceScopeDigest, TargetScopeDigest: targetScope, SourceGeneration: sourceGeneration, TargetGeneration: targetGeneration, TargetFrames: []ports.CheckpointExternalExactFactRefV2{restoreActivationExternalRefV1(bundle.Attempt.Ref.TenantID, targetScope, "target-frame", "context-frame")}, CurrentDigest: core.DigestBytes([]byte("context-current"))}
	contextProjection, err := ports.SealRestoreContextMaterializationCurrentProjectionV1(ports.RestoreContextMaterializationCurrentProjectionV1{Fact: contextRef, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(2 * time.Minute).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	stageReader := &restoreActivationStageReaderV1{value: stageProjection}
	settlementReader := &restoreActivationSettlementReaderV1{value: settlement}
	sandboxReader := &restoreActivationSandboxReaderV1{value: applyProjection}
	contextReader := &restoreActivationContextReaderV1{value: contextProjection}
	gateway := RestoreActivationGatewayV1{Restore: restore, Stage: stageReader, Settlements: settlementReader, Sandbox: sandboxReader, Context: contextReader, Facts: store, Clock: func() time.Time { return now }}
	submission := ports.RestoreActivationSubmissionV1{Attempt: bundle.Attempt.Ref, Eligibility: bundle.Eligibility.Ref, Stage: domain, RuntimeSettlement: settlement, SandboxSettlement: apply, Context: contextRef, IdempotencyKey: "restore-activation-submission"}
	if err := submission.Validate(); err != nil {
		t.Fatal(err)
	}
	return restoreActivationFixtureV1{now: now, store: store, restore: restore, stage: stageReader, settlement: settlementReader, sandbox: sandboxReader, context: contextReader, gateway: gateway, submission: submission}
}

type restoreActivationPlanReaderV1 struct {
	value ports.RestorePlanCurrentProjectionV2
}

func (r restoreActivationPlanReaderV1) InspectRestorePlanCurrentV2(context.Context, ports.CheckpointExternalExactFactRefV2) (ports.RestorePlanCurrentProjectionV2, error) {
	return r.value, nil
}

type restoreActivationInputsReaderV1 struct{ now time.Time }

func (r restoreActivationInputsReaderV1) InspectRestoreEligibilityInputsCurrentV2(_ context.Context, attempt ports.RestoreAttemptFactV2) (ports.RestoreEligibilityInputsCurrentProjectionV2, error) {
	ref := func(kind string) []ports.CheckpointExternalExactFactRefV2 {
		return []ports.CheckpointExternalExactFactRefV2{restoreActivationExternalRefV1(attempt.Ref.TenantID, attempt.OperationScope.SourceScopeDigest, kind, kind)}
	}
	return ports.SealRestoreEligibilityInputsCurrentProjectionV2(ports.RestoreEligibilityInputsCurrentProjectionV2{Attempt: attempt.Ref, OperationScopeDigest: attempt.OperationScope.Digest, SourceScopeDigest: attempt.OperationScope.SourceScopeDigest, ReviewTarget: ports.OperationReviewTargetRefV4{Ref: "restore-review-target-activation", Revision: 1, Digest: core.DigestBytes([]byte("restore-review-target-activation"))}, ReviewRequirementRefs: ref("review"), PolicyBasisRefs: ref("policy"), AuthorityRequirementRefs: ref("authority"), ScopeRequirementRefs: ref("scope"), BudgetRequirementRefs: ref("budget"), BindingRequirementRefs: ref("binding"), ContextRequirementRefs: ref("context"), CheckedUnixNano: r.now.UnixNano(), ExpiresUnixNano: r.now.Add(4 * time.Minute).UnixNano()}, r.now)
}

type restoreActivationStageReaderV1 struct {
	value ports.RestoreStageDomainResultCurrentProjectionV1
}

func (r *restoreActivationStageReaderV1) InspectRestoreStageDomainResultCurrentV1(context.Context, ports.RestoreStageDomainResultFactRefV1) (ports.RestoreStageDomainResultCurrentProjectionV1, error) {
	return r.value, nil
}

type restoreActivationSettlementReaderV1 struct {
	value ports.RestoreStageSettlementRefV1
}

func (r *restoreActivationSettlementReaderV1) SettleRestoreStageV1(context.Context, ports.RestoreStageSettlementSubmissionV1) (ports.RestoreStageSettlementRefV1, error) {
	panic("read-only test")
}
func (r *restoreActivationSettlementReaderV1) InspectRestoreStageSettlementV1(context.Context, string) (ports.RestoreStageSettlementFactV1, error) {
	panic("read-only test")
}
func (r *restoreActivationSettlementReaderV1) InspectCurrentRestoreStageSettlementV1(context.Context, ports.OperationSubjectV3, core.EffectIntentID) (ports.RestoreStageSettlementRefV1, error) {
	return r.value, nil
}

type restoreActivationSandboxReaderV1 struct {
	value ports.RestoreStageApplySettlementCurrentProjectionV1
}

func (r *restoreActivationSandboxReaderV1) InspectRestoreStageApplySettlementCurrentV1(context.Context, ports.RestoreStageApplySettlementRefV1) (ports.RestoreStageApplySettlementCurrentProjectionV1, error) {
	return r.value, nil
}

type restoreActivationContextReaderV1 struct {
	value ports.RestoreContextMaterializationCurrentProjectionV1
}

func (r *restoreActivationContextReaderV1) InspectRestoreContextMaterializationCurrentV1(context.Context, ports.RestoreContextMaterializationRefV1) (ports.RestoreContextMaterializationCurrentProjectionV1, error) {
	return r.value, nil
}

func restoreActivationOperationV1(t *testing.T, identity ports.RestoreIdentityReservationV2, attempt ports.RestoreAttemptRefV2) ports.OperationSubjectV3 {
	t.Helper()
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: attempt.TenantID, ID: "identity-activation", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-activation", PlanDigest: core.DigestBytes([]byte("lineage-activation"))}, Instance: identity.TargetInstance, SandboxLease: &identity.TargetLease, AuthorityEpoch: identity.TargetFenceEpoch}
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := ports.OperationSubjectV3{Kind: ports.RestoreStageOperationKindV1, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, CustomOperationID: attempt.ID, SubjectRevision: 1, CurrentProjectionRef: "restore-operation-current-activation", CurrentProjectionDigest: core.DigestBytes([]byte("restore-operation-current-activation")), CurrentProjectionRevision: 1}
	if err := operation.Validate(); err != nil {
		t.Fatal(err)
	}
	return operation
}

func restoreActivationOwnerV1(component, capability string) ports.ProviderBindingRefV2 {
	return ports.ProviderBindingRefV2{BindingSetID: "binding-set-" + component, BindingSetRevision: 1, ComponentID: ports.ComponentIDV2(component), ManifestDigest: core.DigestBytes([]byte("manifest-" + component)), ArtifactDigest: core.DigestBytes([]byte("artifact-" + component)), Capability: ports.CapabilityNameV2(capability)}
}

func restoreActivationExternalRefV1(tenant core.TenantID, scope core.Digest, id, kind string) ports.CheckpointExternalExactFactRefV2 {
	owner := restoreActivationOwnerV1("praxis/"+kind, kind+"-reader")
	return ports.CheckpointExternalExactFactRefV2{ContractVersion: "praxis.test/" + kind + "/v1", SchemaRef: "praxis.test/" + kind + "-fact/v1", Owner: ports.CheckpointManifestSealOwnerBindingV2{BindingSetID: owner.BindingSetID, BindingRevision: owner.BindingSetRevision, ComponentID: string(owner.ComponentID), ManifestDigest: string(owner.ManifestDigest), ArtifactDigest: string(owner.ArtifactDigest), Capability: string(owner.Capability), FactKind: kind}, TenantID: string(tenant), ID: id, Revision: 1, Digest: string(core.DigestBytes([]byte(id))), ScopeDigest: string(scope)}
}
