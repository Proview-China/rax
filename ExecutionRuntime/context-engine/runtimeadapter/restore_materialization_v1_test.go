package runtimeadapter_test

import (
	"context"
	"strings"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	contextcontract "github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	contextkernel "github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/restorestore"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/runtimeadapter"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type restoreAdapterRequirementReaderV1 struct{ now time.Time }

func (r restoreAdapterRequirementReaderV1) InspectRestoreContextRequirementsCurrentV1(_ context.Context, request contextcontract.RestoreContextMaterializationRequestV1) (contextcontract.RestoreContextRequirementsCurrentV1, error) {
	digest, err := request.DigestValue()
	if err != nil {
		return contextcontract.RestoreContextRequirementsCurrentV1{}, err
	}
	return contextcontract.SealRestoreContextRequirementsCurrentV1(contextcontract.RestoreContextRequirementsCurrentV1{
		RequestDigest: digest, Proofs: []contextcontract.FactRef{{ID: "restore-context-requirements-proof", Revision: 1, Digest: contextcontract.DigestBytes([]byte("restore-context-requirements-proof"))}},
		CheckedUnixNano: r.now.UnixNano(), ExpiresUnixNano: r.now.Add(45 * time.Second).UnixNano(),
	})
}

func TestRestoreContextMaterializationAdapterV1ExactRoutesLostReplyAndInspect(t *testing.T) {
	fixture := newRestoreContextAdapterFixtureV1(t, true)
	if err := fixture.request.Stage.Validate(fixture.now); err != nil {
		t.Fatalf("returned fixture Stage drifted: %+v err=%v", fixture.request.Stage, err)
	}
	if err := fixture.request.ValidateCurrent(fixture.now); err != nil {
		t.Fatalf("returned fixture request drifted: err=%v", err)
	}
	projection, err := fixture.adapter.MaterializeRestoreContextV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Fact.Attempt != fixture.request.Materialization.Attempt || projection.Fact.Identity != fixture.request.Materialization.Identity || projection.Fact.SourceGeneration != fixture.request.Materialization.ContextGeneration || len(projection.Fact.TargetFrames) != 1 || len(projection.Residuals) != 0 {
		t.Fatalf("Context adapter lost exact Restore closure: %+v", projection)
	}
	inspected, err := fixture.adapter.InspectRestoreContextMaterializationCurrentV1(context.Background(), projection.Fact)
	if err != nil || inspected.ProjectionDigest != projection.ProjectionDigest {
		t.Fatalf("exact Inspect did not recover the same projection: got=%+v err=%v", inspected, err)
	}
	projection.Fact.TargetFrames[0].ID = "caller-alias"
	again, err := fixture.adapter.InspectRestoreContextMaterializationCurrentV1(context.Background(), inspected.Fact)
	if err != nil || again.Fact.TargetFrames[0].ID == "caller-alias" {
		t.Fatalf("Context adapter leaked caller alias: got=%+v err=%v", again, err)
	}
}

func TestRestoreContextMaterializationAdapterV1RejectsOwnerRouteAndStageSplice(t *testing.T) {
	for name, mutate := range map[string]func(*restoreContextAdapterFixtureV1){
		"source owner route": func(f *restoreContextAdapterFixtureV1) {
			f.request.Materialization.ContextGeneration.Owner.BindingRevision++
			f.reseal(t)
		},
		"stage target identity": func(f *restoreContextAdapterFixtureV1) {
			f.request.Materialization.Identity.TargetInstance.ID = "spliced-target-instance"
			f.reseal(t)
		},
		"requirement tenant": func(f *restoreContextAdapterFixtureV1) {
			f.request.Requirements[0].Ref.TenantID = "tenant-other"
			f.reseal(t)
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newRestoreContextAdapterFixtureV1(t, false)
			mutate(&fixture)
			if _, err := fixture.adapter.MaterializeRestoreContextV1(context.Background(), fixture.request); err == nil {
				t.Fatal("spliced Restore Context request was accepted")
			}
		})
	}
}

type restoreContextAdapterFixtureV1 struct {
	now     time.Time
	adapter *runtimeadapter.RestoreContextMaterializationAdapterV1
	request applicationcontract.RestoreContextMaterializationRequestV1
}

func (f *restoreContextAdapterFixtureV1) reseal(t *testing.T) {
	t.Helper()
	materialization, err := runtimeports.SealRestoreMaterializationCurrentProjectionV1(f.request.Materialization, f.now)
	if err == nil {
		f.request.Materialization = materialization
	}
	f.request.Digest = ""
	sealed, sealErr := applicationcontract.SealRestoreContextMaterializationRequestV1(f.request)
	if sealErr == nil {
		f.request = sealed
	}
}

func newRestoreContextAdapterFixtureV1(t *testing.T, loseReply bool) restoreContextAdapterFixtureV1 {
	t.Helper()
	now := time.Unix(1_780_000_000, 0)
	plan, err := runtimefakes.BuildRestorePlanCurrentFixtureV2("context-adapter", now)
	if err != nil {
		t.Fatal(err)
	}
	tenant := runtimecore.TenantID(plan.RestorePlan.TenantID)
	attempt := runtimeports.RestoreAttemptRefV2{TenantID: tenant, ID: "restore-attempt-context-adapter", Revision: 2, Digest: runtimecore.DigestBytes([]byte("restore-attempt-context-adapter"))}
	eligibility := runtimeports.RestoreEligibilityRefV2{TenantID: tenant, ID: "restore-eligibility-context-adapter", Revision: 1, Digest: runtimecore.DigestBytes([]byte("restore-eligibility-context-adapter")), ExpiresUnixNano: now.Add(2 * time.Minute).UnixNano()}
	scope := runtimecore.ExecutionScope{Identity: runtimecore.AgentIdentityRef{TenantID: tenant, ID: "identity-context-adapter", Epoch: 1}, Lineage: runtimecore.LineageRef{ID: "lineage-context-adapter", PlanDigest: runtimecore.DigestBytes([]byte("lineage-context-adapter"))}, Instance: plan.IdentityProposal.TargetInstance, SandboxLease: &plan.IdentityProposal.TargetLease, AuthorityEpoch: plan.IdentityProposal.TargetFenceEpoch}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := runtimeports.OperationSubjectV3{Kind: runtimeports.RestoreStageOperationKindV1, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, CustomOperationID: attempt.ID, SubjectRevision: 1, CurrentProjectionRef: "restore-operation-context-adapter", CurrentProjectionDigest: runtimecore.DigestBytes([]byte("restore-operation-context-adapter")), CurrentProjectionRevision: 1}
	opDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	dispatch := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: opDigest, EffectID: "restore-effect-context-adapter", IntentRevision: 1, IntentDigest: runtimecore.DigestBytes([]byte("intent-context-adapter")), PermitID: "permit-context-adapter", PermitRevision: 1, PermitDigest: runtimecore.DigestBytes([]byte("permit-context-adapter")), AttemptID: "dispatch-context-adapter"}
	domain := runtimeports.RestoreStageDomainResultFactRefV1{Owner: restoreContextProviderOwnerV1("praxis/sandbox", "workspace-restore-stage"), Kind: runtimeports.RestoreStageDomainResultKindV1, ID: "stage-domain-context-adapter", Revision: 1, Digest: runtimecore.DigestBytes([]byte("stage-domain-context-adapter")), TenantID: tenant, Operation: operation, OperationDigest: opDigest, EffectID: dispatch.EffectID, EffectRevision: 1, Attempt: dispatch, RestoreAttempt: attempt, Eligibility: eligibility, PayloadSchema: runtimeports.SchemaRefV2{Namespace: "praxis.sandbox", Name: "workspace-restore-stage", Version: "1.0.0", MediaType: "application/json", ContentDigest: runtimecore.DigestBytes([]byte("stage-schema"))}, PayloadDigest: runtimecore.DigestBytes([]byte("stage-payload")), PayloadRevision: 1, AuthoritativeTime: now.UnixNano()}
	stage, err := runtimeports.SealRestoreStageDomainResultCurrentProjectionV1(runtimeports.RestoreStageDomainResultCurrentProjectionV1{Fact: domain, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}, now)
	if err != nil {
		t.Fatalf("seal stage: %v; domain=%v operation=%v dispatch=%v", err, domain.Validate(), operation.Validate(), dispatch.Validate())
	}
	settlement := runtimeports.RestoreStageSettlementRefV1{ID: "runtime-stage-settlement-context-adapter", Revision: 1, Digest: runtimecore.DigestBytes([]byte("runtime-stage-settlement-context-adapter")), OperationDigest: opDigest, EffectID: dispatch.EffectID, DomainResult: domain}
	applyRef := runtimeports.RestoreStageApplySettlementRefV1{Owner: restoreContextProviderOwnerV1("praxis/sandbox", "workspace-restore-apply"), ID: "sandbox-stage-apply-context-adapter", Revision: 1, Digest: runtimecore.DigestBytes([]byte("sandbox-stage-apply-context-adapter")), TenantID: tenant, DomainResult: domain, RuntimeSettlement: settlement}
	apply, err := runtimeports.SealRestoreStageApplySettlementCurrentProjectionV1(runtimeports.RestoreStageApplySettlementCurrentProjectionV1{Fact: applyRef, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}

	sourceGenerationRoute := restoreContextRouteV1("source-context-generation")
	sourceFrameRoute := restoreContextRouteV1("source-context-frame")
	targetGenerationRoute := restoreContextRouteV1("target-context-generation")
	targetFrameRoute := restoreContextRouteV1("target-context-frame")
	residualRoute := restoreContextRouteV1("restore-context-residual")
	metadata := testkit.NewMetadataStoreV1()
	frame := contextcontract.ContextFrame{ContractVersion: contextcontract.Version, ID: "source-frame-context-adapter", Revision: 1, Execution: contextcontract.ExecutionBinding{ScopeDigest: contextcontract.Digest(plan.SourceScopeDigest), RunID: "source-run-context-adapter", Turn: 1, AuthorityDigest: contextcontract.DigestBytes([]byte("source-authority-context-adapter"))}, ManifestRef: contextFactRefV1("source-manifest-context-adapter"), GenerationID: "source-generation-context-adapter", Generation: 1, StablePrefix: contextContentRefV1("stable"), DynamicTail: contextContentRefV1("dynamic"), Rendered: contextContentRefV1("rendered"), SourceSetDigest: contextcontract.DigestBytes([]byte("source-set-context-adapter")), CreatedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(2 * time.Minute).UnixNano()}
	frameDigest, err := frame.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	frameRef := contextcontract.FactRef{ID: frame.ID, Revision: frame.Revision, Digest: frameDigest}
	generation := contextcontract.ContextGeneration{ContractVersion: contextcontract.Version, ID: frame.GenerationID, Revision: 1, Ordinal: 1, RootFrame: frameRef, CreatedUnixNano: now.Add(-time.Minute).UnixNano()}
	generationDigest, err := generation.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	if err = metadata.PutFrame(frame); err != nil {
		t.Fatal(err)
	}
	if err = metadata.PutGeneration(contextcontract.Digest(plan.SourceScopeDigest), generation); err != nil {
		t.Fatal(err)
	}

	store := restorestore.NewMemory()
	if loseReply {
		store.LoseNextReplyV1()
	}
	service, err := contextkernel.NewRestoreContextMaterializationServiceV1(metadata, metadata, restoreAdapterRequirementReaderV1{now: now}, store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	factOwner := restoreContextProviderOwnerV1("praxis/context", "restore-context-materialization")
	adapter, err := runtimeadapter.NewRestoreContextMaterializationAdapterV1(service, factOwner, sourceGenerationRoute, sourceFrameRoute, targetGenerationRoute, targetFrameRoute, residualRoute, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	sourceGeneration := restoreContextExternalV1(sourceGenerationRoute, tenant, plan.SourceScopeDigest, generation.ID, generation.Revision, generationDigest)
	sourceFrame := restoreContextExternalV1(sourceFrameRoute, tenant, plan.SourceScopeDigest, frame.ID, frame.Revision, frameDigest)
	extra := func(id string) runtimeports.CheckpointExternalExactFactRefV2 {
		return restoreContextExternalV1(restoreContextRouteV1(id), tenant, plan.SourceScopeDigest, id, 1, contextcontract.DigestBytes([]byte(id)))
	}
	materialization, err := runtimeports.SealRestoreMaterializationCurrentProjectionV1(runtimeports.RestoreMaterializationCurrentProjectionV1{Attempt: attempt, Eligibility: eligibility, RestorePlan: plan.RestorePlan, Consistency: plan.CheckpointConsistency.Ref, ManifestSeal: plan.ManifestSeal, SourceScopeDigest: plan.SourceScopeDigest, Identity: plan.IdentityProposal, ContextGeneration: sourceGeneration, ContextFrames: []runtimeports.CheckpointExternalExactFactRefV2{sourceFrame}, Memory: []runtimeports.CheckpointExternalExactFactRefV2{extra("memory")}, Knowledge: []runtimeports.CheckpointExternalExactFactRefV2{extra("knowledge")}, Snapshots: []runtimeports.CheckpointExternalExactFactRefV2{extra("snapshot")}, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	kinds := []applicationcontract.RestoreContextRequirementKindV1{applicationcontract.RestoreContextRequirementProfileV1, applicationcontract.RestoreContextRequirementToolV1, applicationcontract.RestoreContextRequirementMCPV1, applicationcontract.RestoreContextRequirementReviewV1, applicationcontract.RestoreContextRequirementAuthorityV1, applicationcontract.RestoreContextRequirementBudgetV1, applicationcontract.RestoreContextRequirementBindingV1}
	requirements := make([]applicationcontract.RestoreContextRequirementCoordinateV1, len(kinds))
	for index, kind := range kinds {
		requirements[index] = applicationcontract.RestoreContextRequirementCoordinateV1{Kind: kind, Ref: extra("requirement-" + string(kind))}
	}
	request, err := applicationcontract.SealRestoreContextMaterializationRequestV1(applicationcontract.RestoreContextMaterializationRequestV1{ID: "restore-context-materialization-context-adapter", IdempotencyKey: "restore-context-materialization-key-context-adapter", Materialization: materialization, Stage: stage, SandboxSettlement: apply, Requirements: requirements, RequestedUnixNano: now.UnixNano(), NotAfterUnixNano: now.Add(30 * time.Second).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if err = request.ValidateCurrent(now); err != nil {
		t.Fatalf("request current: %v; stage version=%q checked=%d expires=%d now=%d fact=%v digest=%q", err, request.Stage.ContractVersion, request.Stage.CheckedUnixNano, request.Stage.ExpiresUnixNano, now.UnixNano(), request.Stage.Fact.Validate(), request.Stage.ProjectionDigest)
	}
	return restoreContextAdapterFixtureV1{now: now, adapter: adapter, request: request}
}

func restoreContextProviderOwnerV1(component, capability string) runtimeports.ProviderBindingRefV2 {
	if !strings.Contains(capability, "/") {
		capability = "restore/" + capability
	}
	return runtimeports.ProviderBindingRefV2{BindingSetID: "binding-set-" + capability, BindingSetRevision: 1, ComponentID: runtimeports.ComponentIDV2(component), ManifestDigest: runtimecore.DigestBytes([]byte("manifest-" + capability)), ArtifactDigest: runtimecore.DigestBytes([]byte("artifact-" + capability)), Capability: runtimeports.CapabilityNameV2(capability)}
}

func restoreContextRouteV1(kind string) runtimeadapter.RestoreContextExactRouteV1 {
	owner := restoreContextProviderOwnerV1("praxis/context", kind+"-current")
	return runtimeadapter.RestoreContextExactRouteV1{ContractVersion: "praxis.context/" + kind + "/v1", SchemaRef: "praxis.context/" + kind + "-fact/v1", Owner: runtimeports.CheckpointManifestSealOwnerBindingV2{BindingSetID: owner.BindingSetID, BindingRevision: owner.BindingSetRevision, ComponentID: string(owner.ComponentID), ManifestDigest: string(owner.ManifestDigest), ArtifactDigest: string(owner.ArtifactDigest), Capability: string(owner.Capability), FactKind: kind}}
}

func restoreContextExternalV1(route runtimeadapter.RestoreContextExactRouteV1, tenant runtimecore.TenantID, scope runtimecore.Digest, id string, revision uint64, digest contextcontract.Digest) runtimeports.CheckpointExternalExactFactRefV2 {
	return runtimeports.CheckpointExternalExactFactRefV2{ContractVersion: route.ContractVersion, SchemaRef: route.SchemaRef, Owner: route.Owner, TenantID: string(tenant), ID: id, Revision: runtimecore.Revision(revision), Digest: string(digest), ScopeDigest: string(scope)}
}

func contextFactRefV1(id string) contextcontract.FactRef {
	return contextcontract.FactRef{ID: id, Revision: 1, Digest: contextcontract.DigestBytes([]byte(id))}
}

func contextContentRefV1(id string) contextcontract.ContentRef {
	return contextcontract.ContentRef{Ref: "content:" + id, Digest: contextcontract.DigestBytes([]byte(id)), Length: uint64(len(id))}
}
