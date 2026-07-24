package runtimeadapter

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	continuityports "github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimekernel "github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRestorePlanCurrentReaderV2ProjectsExactSubmittedPlan(t *testing.T) {
	fixture := newRestorePlanAdapterFixtureV2(t)
	projection, err := fixture.adapter.InspectRestorePlanCurrentV2(context.Background(), fixture.expected)
	if err != nil {
		t.Fatal(err)
	}
	if projection.RestorePlan != fixture.expected || projection.CheckpointConsistency.Ref != fixture.consistency.Ref || projection.ManifestSeal != fixture.consistency.ManifestSeal {
		t.Fatalf("projection lost exact refs: %+v", projection)
	}
	if projection.IdentityProposal.SourceInstance.ID == projection.IdentityProposal.TargetInstance.ID || projection.IdentityProposal.TargetInstance.Epoch <= projection.IdentityProposal.SourceInstance.Epoch {
		t.Fatalf("projection did not preserve fresh Instance semantics: %+v", projection.IdentityProposal)
	}
}

func TestRestorePlanCurrentReaderV2FeedsRuntimeReservationAndEligibility(t *testing.T) {
	fixture := newRestorePlanAdapterFixtureV2(t)
	store := runtimefakes.NewRestoreGovernanceStoreV2()
	gateway := runtimekernel.RestoreGovernanceGatewayV2{
		Facts: store, Plans: fixture.adapter,
		Inputs: restoreEligibilityInputsAdapterV2{now: fixture.now, tenant: runtimecore.TenantID(fixture.expected.TenantID), scope: runtimecore.Digest(fixture.expected.ScopeDigest)},
		Clock:  func() time.Time { return fixture.now },
	}
	attempt, err := gateway.CreateRestoreAttemptV2(context.Background(), runtimeports.CreateRestoreAttemptRequestV2{AttemptID: "restore-attempt-adapter-e2e", IdempotencyKey: "restore-attempt-adapter-idempotency", RestorePlan: fixture.expected, RequestedNotAfter: fixture.now.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	store.LoseNextRestoreReplyV2()
	bundle, err := gateway.IssueRestoreEligibilityV2(context.Background(), runtimeports.IssueRestoreEligibilityRequestV2{EligibilityID: "restore-eligibility-adapter-e2e", Attempt: attempt.Ref, RequestedTTL: time.Minute})
	if err != nil || bundle.Eligibility.Identity.TargetInstance.ID != runtimecore.AgentInstanceID(fixture.plan.plan.ProposedInstance.InstanceID) {
		t.Fatalf("Continuity Plan did not feed exact Runtime reservation: %+v err=%v", bundle, err)
	}
}

func TestRestorePlanCurrentReaderV2RejectsCurrentAndClosureDrift(t *testing.T) {
	for name, mutate := range map[string]func(*restorePlanAdapterFixtureV2){
		"not-submitted": func(f *restorePlanAdapterFixtureV2) {
			f.plan.plan.State = contract.RestorePlanAdmittedV2
			refreshRestorePlanAdapterV2(t, &f.plan.plan)
		},
		"wrong-current-ref": func(f *restorePlanAdapterFixtureV2) {
			f.plan.plan.Revision++
			refreshRestorePlanAdapterV2(t, &f.plan.plan)
		},
		"seal-drift": func(f *restorePlanAdapterFixtureV2) {
			f.seals.seal.FrozenRefSetDigest = contract.DigestBytes([]byte("drift"))
			testkit.RefreshSealV2(&f.seals.seal)
		},
		"consistency-drift": func(f *restorePlanAdapterFixtureV2) {
			f.consistencyReader.fact.ParticipantSetDigest = runtimecore.DigestBytes([]byte("drift"))
		},
		"source-instance-drift": func(f *restorePlanAdapterFixtureV2) {
			f.plan.plan.SourceInstanceRef.ID = "another-source-instance"
			refreshRestorePlanAdapterV2(t, &f.plan.plan)
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newRestorePlanAdapterFixtureV2(t)
			mutate(&fixture)
			if _, err := fixture.adapter.InspectRestorePlanCurrentV2(context.Background(), fixture.expected); err == nil {
				t.Fatal("drifted Restore Plan input was accepted")
			}
		})
	}
}

func TestRestorePlanCurrentReaderV2ExpiryAndTypedNil(t *testing.T) {
	fixture := newRestorePlanAdapterFixtureV2(t)
	fixture.now = time.Unix(0, fixture.plan.plan.ExpiresUnixNano)
	fixture.adapter.Clock = func() time.Time { return fixture.now }
	if _, err := fixture.adapter.InspectRestorePlanCurrentV2(context.Background(), fixture.expected); err == nil {
		t.Fatal("expired submitted Plan was accepted as current")
	}
	fixture = newRestorePlanAdapterFixtureV2(t)
	var typedNil *restorePlanReaderAdapterV2
	fixture.adapter.Plans = typedNil
	if _, err := fixture.adapter.InspectRestorePlanCurrentV2(context.Background(), fixture.expected); !runtimecore.HasCategory(err, runtimecore.ErrorUnavailable) {
		t.Fatalf("typed-nil Plan Reader was accepted: %v", err)
	}
}

type restorePlanAdapterFixtureV2 struct {
	now               time.Time
	plan              *restorePlanReaderAdapterV2
	seals             *checkpointManifestExactReaderV2
	consistencyReader *restoreConsistencyReaderAdapterV2
	adapter           RestorePlanCurrentReaderV2
	expected          runtimeports.CheckpointExternalExactFactRefV2
	consistency       runtimeports.CheckpointConsistencyFactV2
}

func newRestorePlanAdapterFixtureV2(t *testing.T) restorePlanAdapterFixtureV2 {
	t.Helper()
	// Checkpoint fixtures seal immutable facts a few nanoseconds after their
	// base timestamp; consumers therefore inspect them from a later instant.
	now := time.Unix(1_752_577_200, 0).UTC().Add(time.Second)
	manifest := newCheckpointManifestAdapterFixtureV2(t)
	consistency, err := runtimeports.SealCheckpointConsistencyFactV2(runtimeports.CheckpointConsistencyFactV2{
		Ref:     runtimeports.CheckpointConsistencyRefV2{ID: "checkpoint-consistency-restore-adapter", Revision: 1, Attempt: manifest.request.Ref.Attempt},
		Barrier: manifest.request.Ref.Barrier, EffectCut: manifest.request.Ref.EffectCut, ManifestSeal: manifest.request.Ref,
		ParticipantClosures:  []runtimeports.CheckpointParticipantClosureRefV2{manifest.closure},
		ParticipantSetDigest: manifest.request.ExpectedParticipantSetDigest, ParticipantRootDigest: runtimecore.DigestBytes([]byte("restore-adapter-participant-root")),
		ParticipantWatermark: 1, ParticipantCount: 1, FrozenRefSetDigest: manifest.request.Ref.FrozenRefSetDigest, CreatedUnixNano: now.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := testkit.RestorePlanV2(now)
	plan.Scope.TenantID = string(consistency.Ref.Attempt.TenantID)
	plan.Scope.ExecutionScopeDigest = manifest.seal.ScopeDigest
	plan.Scope.InstanceID = "source-instance-restore-adapter"
	plan.Scope.InstanceEpoch = 7
	plan.State = contract.RestorePlanSubmittedV2
	plan.CheckpointConsistencyRef = runtimeExactForRestorePlanAdapterV2("checkpoint-consistency-restore-adapter", "checkpoint_consistency_fact_v2", uint64(consistency.Ref.Revision), string(consistency.Ref.Digest), plan.Scope.TenantID, plan.Scope.ExecutionScopeDigest)
	plan.ManifestSealRef = manifest.seal.Ref()
	plan.FrozenRefSetDigest = manifest.seal.FrozenRefSetDigest
	plan.SourceInstanceRef = runtimeExactForRestorePlanAdapterV2(plan.Scope.InstanceID, "instance_fact_v2", 1, string(runtimecore.DigestBytes([]byte("source-instance-fact"))), plan.Scope.TenantID, plan.Scope.ExecutionScopeDigest)
	plan.SourceInstanceEpoch = plan.Scope.InstanceEpoch
	plan.ProposedInstance = contract.RestoreInstanceProposalV2{InstanceID: "target-instance-restore-adapter", Epoch: 8, LeaseID: "target-lease-restore-adapter", LeaseEpoch: 8, FenceEpoch: 8}
	plan.RequiredParticipantSetDigest = manifest.seal.RequiredParticipantSetDigest
	plan.ContextGenerationRef = manifest.reader.manifest.ContextGenerationRef
	plan.ContextFrameRefs = append([]contract.ExactFactRefV2{}, manifest.reader.manifest.ContextFrameRefs...)
	plan.ConflictDomain = "tenant/" + plan.Scope.TenantID + "/restore-adapter"
	retargetRestorePlanRefsAdapterV2(&plan)
	plan.CreatedUnixNano = now.UnixNano()
	plan.UpdatedUnixNano = now.UnixNano()
	plan.ExpiresUnixNano = now.Add(10 * time.Minute).UnixNano()
	refreshRestorePlanAdapterV2(t, &plan)
	if err := plan.ValidateCurrent(now); err != nil {
		t.Fatal(err)
	}
	expected := exactContinuityToRuntimeAdapterV2(plan.Ref().Exact())
	planReader := &restorePlanReaderAdapterV2{plan: plan}
	consistencyReader := &restoreConsistencyReaderAdapterV2{fact: consistency}
	adapter := RestorePlanCurrentReaderV2{Plans: planReader, ManifestSeals: manifest.reader, Consistency: consistencyReader, Clock: func() time.Time { return now }}
	return restorePlanAdapterFixtureV2{now: now, plan: planReader, seals: manifest.reader, consistencyReader: consistencyReader, adapter: adapter, expected: expected, consistency: consistency}
}

type restorePlanReaderAdapterV2 struct{ plan contract.RestorePlanFactV2 }

func (r *restorePlanReaderAdapterV2) InspectRestorePlanV2(_ context.Context, request continuityports.InspectRestorePlanRequestV2) (contract.RestorePlanFactV2, error) {
	if request.Ref != r.plan.Ref() {
		return contract.RestorePlanFactV2{}, contract.NewError(contract.ErrRevisionConflict, "restore_plan_ref", "test exact ref drift")
	}
	return r.plan.Clone(), nil
}

func (r *restorePlanReaderAdapterV2) InspectCurrentRestorePlanV2(_ context.Context, request continuityports.InspectCurrentRestorePlanRequestV2) (contract.RestorePlanFactV2, error) {
	if request.TenantID != r.plan.Scope.TenantID || request.ScopeDigest != r.plan.Scope.ExecutionScopeDigest || request.PlanID != r.plan.PlanID || request.Owner != r.plan.Owner {
		return contract.RestorePlanFactV2{}, contract.NewError(contract.ErrRevisionConflict, "restore_plan_current", "test current coordinate drift")
	}
	return r.plan.Clone(), nil
}

type restoreConsistencyReaderAdapterV2 struct {
	fact runtimeports.CheckpointConsistencyFactV2
}

func (r *restoreConsistencyReaderAdapterV2) InspectCheckpointConsistencyV2(_ context.Context, ref runtimeports.CheckpointConsistencyRefV2) (runtimeports.CheckpointConsistencyFactV2, error) {
	if ref != r.fact.Ref {
		return runtimeports.CheckpointConsistencyFactV2{}, runtimecore.NewError(runtimecore.ErrorConflict, runtimecore.ReasonCheckpointInconsistent, "test Consistency exact ref drift")
	}
	return r.fact, nil
}

type restoreEligibilityInputsAdapterV2 struct {
	now    time.Time
	tenant runtimecore.TenantID
	scope  runtimecore.Digest
}

func (r restoreEligibilityInputsAdapterV2) InspectRestoreEligibilityInputsCurrentV2(_ context.Context, attempt runtimeports.RestoreAttemptFactV2) (runtimeports.RestoreEligibilityInputsCurrentProjectionV2, error) {
	refs := func(kind string) []runtimeports.CheckpointExternalExactFactRefV2 {
		return []runtimeports.CheckpointExternalExactFactRefV2{restoreRuntimeInputRefAdapterV2(r.tenant, r.scope, kind)}
	}
	return runtimeports.SealRestoreEligibilityInputsCurrentProjectionV2(runtimeports.RestoreEligibilityInputsCurrentProjectionV2{
		Attempt: attempt.Ref, OperationScopeDigest: attempt.OperationScope.Digest, SourceScopeDigest: r.scope,
		ReviewTarget:          runtimeports.OperationReviewTargetRefV4{Ref: "restore-review-target-adapter", Revision: 1, Digest: runtimecore.DigestBytes([]byte("restore-review-target-adapter"))},
		ReviewRequirementRefs: refs("review-requirement"), PolicyBasisRefs: refs("policy-basis"), AuthorityRequirementRefs: refs("authority"), ScopeRequirementRefs: refs("scope"), BudgetRequirementRefs: refs("budget"), BindingRequirementRefs: refs("binding"), ContextRequirementRefs: refs("context"),
		CheckedUnixNano: r.now.UnixNano(), ExpiresUnixNano: r.now.Add(3 * time.Minute).UnixNano(),
	}, r.now)
}

func restoreRuntimeInputRefAdapterV2(tenant runtimecore.TenantID, scope runtimecore.Digest, kind string) runtimeports.CheckpointExternalExactFactRefV2 {
	digest := runtimecore.DigestBytes([]byte("restore-input-" + kind))
	return runtimeports.CheckpointExternalExactFactRefV2{
		ContractVersion: "praxis.test/" + kind + "/v1", SchemaRef: "praxis.test/" + kind + "-fact/v1",
		Owner:    runtimeports.CheckpointManifestSealOwnerBindingV2{BindingSetID: "binding-set-" + kind, BindingRevision: 1, ComponentID: "praxis/" + kind, ManifestDigest: string(runtimecore.DigestBytes([]byte("manifest-" + kind))), ArtifactDigest: string(runtimecore.DigestBytes([]byte("artifact-" + kind))), Capability: kind + "-current", FactKind: kind + "-fact"},
		TenantID: string(tenant), ID: kind + "-fact", Revision: 1, Digest: string(digest), ScopeDigest: string(scope),
	}
}

func retargetRestorePlanRefsAdapterV2(plan *contract.RestorePlanFactV2) {
	sets := [][]contract.ExactFactRefV2{plan.ContextFrameRefs, plan.CompatibilityRefs, plan.CurrentnessRefs, plan.ReviewRequirementRefs, plan.AuthorityRequirementRefs, plan.BudgetRequirementRefs, plan.BindingRequirementRefs, plan.ResidualRefs}
	for _, values := range sets {
		for index := range values {
			values[index].TenantID = plan.Scope.TenantID
			values[index].ScopeDigest = plan.Scope.ExecutionScopeDigest
		}
	}
	for _, ref := range []*contract.ExactFactRefV2{&plan.ContextGenerationRef, &plan.ResidualPolicyRef, &plan.RecoveryCredentialRef} {
		ref.TenantID = plan.Scope.TenantID
		ref.ScopeDigest = plan.Scope.ExecutionScopeDigest
	}
}

func runtimeExactForRestorePlanAdapterV2(id, factKind string, revision uint64, digest, tenant, scope string) contract.ExactFactRefV2 {
	ref := testkit.ExactRefV2(id, "praxis/runtime", factKind)
	ref.TenantID, ref.ScopeDigest, ref.Revision, ref.Digest = tenant, scope, revision, digest
	return ref
}

func exactContinuityToRuntimeAdapterV2(ref contract.ExactFactRefV2) runtimeports.CheckpointExternalExactFactRefV2 {
	return runtimeports.CheckpointExternalExactFactRefV2{
		ContractVersion: ref.ContractVersion, SchemaRef: ref.SchemaRef,
		Owner:    runtimeports.CheckpointManifestSealOwnerBindingV2{BindingSetID: ref.Owner.BindingSetID, BindingRevision: runtimecore.Revision(ref.Owner.BindingRevision), ComponentID: ref.Owner.ComponentID, ManifestDigest: ref.Owner.ManifestDigest, ArtifactDigest: ref.Owner.ArtifactDigest, Capability: ref.Owner.Capability, FactKind: ref.Owner.FactKind},
		TenantID: ref.TenantID, ID: ref.ID, Revision: runtimecore.Revision(ref.Revision), Digest: ref.Digest, ScopeDigest: ref.ScopeDigest,
	}
}

func refreshRestorePlanAdapterV2(t *testing.T, plan *contract.RestorePlanFactV2) {
	t.Helper()
	plan.Digest = ""
	digest, err := plan.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	plan.Digest = digest
}
