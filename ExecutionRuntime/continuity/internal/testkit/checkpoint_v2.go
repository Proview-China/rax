package testkit

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func CheckpointOwnerV2(factKind string) contract.OwnerBinding {
	return contract.OwnerBinding{
		BindingSetID: "binding-set-checkpoint-v2", BindingRevision: 2,
		ComponentID: contract.ContinuityComponentID, ManifestDigest: "continuity-manifest-digest-v2",
		ArtifactDigest: "continuity-artifact-digest-v2", Capability: contract.CheckpointManifestCapabilityV2,
		FactKind: factKind,
	}
}

func RestorePlanOwnerV2() contract.OwnerBinding {
	return contract.OwnerBinding{
		BindingSetID: "binding-set-restore-plan-v2", BindingRevision: 2,
		ComponentID: contract.ContinuityComponentID, ManifestDigest: "continuity-manifest-digest-v2",
		ArtifactDigest: "continuity-artifact-digest-v2", Capability: contract.RestorePlanCapabilityV2,
		FactKind: "restore_plan_fact_v2",
	}
}

func RewindPlanOwnerV2() contract.OwnerBinding {
	return contract.OwnerBinding{
		BindingSetID: "binding-set-rewind-plan-v2", BindingRevision: 2,
		ComponentID: contract.ContinuityComponentID, ManifestDigest: "continuity-manifest-digest-v2",
		ArtifactDigest: "continuity-artifact-digest-v2", Capability: contract.RewindPlanCapabilityV2,
		FactKind: "rewind_plan_fact_v2",
	}
}

func ExactRefV2(id, component, factKind string) contract.ExactFactRefV2 {
	return contract.ExactFactRefV2{
		ContractVersion: "praxis." + component + "/v2", SchemaRef: "schema/" + factKind + "/v2",
		Owner: contract.OwnerBinding{
			BindingSetID: "binding-set-" + component, BindingRevision: 2, ComponentID: component,
			ManifestDigest: component + "-manifest-digest", ArtifactDigest: component + "-artifact-digest",
			Capability: factKind, FactKind: factKind,
		},
		TenantID: "tenant-1", ID: id, Revision: 1, Digest: id + "-digest", ScopeDigest: "execution-scope-digest",
	}
}

func ManifestV2(state contract.ManifestState, revision uint64) contract.CheckpointManifestFactV2 {
	created := int64(1_752_577_200_000_000_000)
	participantFact := ExactRefV2("participant-fact-sandbox", "praxis/sandbox", "checkpoint-participant")
	runtimeClosure := ExactRefV2("runtime-participant-closure-sandbox", "praxis/runtime", "checkpoint-participant-closure-ref-v2")
	runtimeClosure.ContractVersion = "2.0.0"
	runtimeClosure.SchemaRef = "praxis.runtime/checkpoint-participant-closure-ref/v2"
	runtimeClosure.Owner.FactKind = "checkpoint_participant_closure_ref_v2"
	snapshot := ExactRefV2("snapshot-sandbox", "praxis/sandbox", "snapshot")
	coverage := ExactRefV2("coverage-sandbox", "praxis/sandbox", "coverage")
	evidence := ExactRefV2("evidence-sandbox", "praxis/runtime", "evidence-record")
	settlement := ExactRefV2("settlement-tool-1", "praxis/runtime", "operation-settlement")
	fact := contract.CheckpointManifestFactV2{
		ContractVersion:      contract.CheckpointManifestGovernanceContractV2,
		SchemaRef:            contract.CheckpointManifestFactSchemaV2,
		ManifestID:           "checkpoint-manifest-v2-1",
		Revision:             revision,
		Owner:                CheckpointOwnerV2("checkpoint_manifest_fact_v2"),
		Scope:                Scope(),
		State:                state,
		IdempotencyKey:       "checkpoint-manifest-request-1",
		CheckpointAttemptRef: ExactRefV2("checkpoint-attempt-1", "praxis/runtime", "checkpoint-attempt"),
		BarrierRef:           ExactRefV2("checkpoint-barrier-1", "praxis/runtime", "checkpoint-barrier"),
		EffectCutRef:         ExactRefV2("effect-cut-1", "praxis/runtime", "checkpoint-effect-cut"),
		TimelineCut: contract.TimelineCutV2{
			LedgerScopeDigest: "ledger-scope-digest", LedgerSequence: 42,
			EvidenceRecordRef: ExactRefV2("timeline-evidence-42", "praxis/runtime", "evidence-record"),
		},
		ContextGenerationRef: ExactRefV2("context-generation-7", "praxis/context", "context-generation"),
		ContextFrameRefs: []contract.ExactFactRefV2{
			ExactRefV2("context-frame-7", "praxis/context", "context-frame"),
		},
		AttemptSettlementClosures: []contract.AttemptSettlementClosureV2{{
			AttemptRef: ExactRefV2("tool-attempt-1", "praxis/runtime", "operation-attempt"),
			Begun:      true, SettlementRef: &settlement,
		}},
		MemoryRefs:                  []contract.ExactFactRefV2{ExactRefV2("memory-watermark-1", "praxis/memory", "memory-watermark")},
		KnowledgeRefs:               []contract.ExactFactRefV2{ExactRefV2("knowledge-snapshot-1", "praxis/knowledge", "knowledge-snapshot")},
		RuntimeParticipantSetDigest: "runtime-participant-set-digest",
		ParticipantClosures: []contract.ParticipantClosureRefV2{{
			ParticipantID: "praxis/sandbox", Required: true, RuntimeClosureRef: runtimeClosure, ParticipantFactRef: participantFact,
			SnapshotRef: &snapshot, CoverageRef: &coverage,
			EvidenceRefs: []contract.ExactFactRefV2{evidence},
		}},
		CreatedUnixNano: created,
		UpdatedUnixNano: created + int64(revision),
	}
	if state == contract.ManifestDiagnosticPartial || state == contract.ManifestDiagnosticIndeterminate || state == contract.ManifestRejected {
		inspection := ExactRefV2("inspection-tool-1", "praxis/runtime", "operation-inspection")
		residual := ResidualV2("residual-tool-1", "operation_unknown", "tool-attempt-1-digest")
		fact.Diagnostics = []contract.CheckpointManifestDiagnosticV2{{
			DiagnosticID: "diagnostic-tool-1", DiagnosticRef: ExactRefV2("diagnostic-tool-1", "praxis/continuity", "checkpoint-diagnostic"), Code: "attempt_unsettled",
			Severity: contract.ManifestDiagnosticBlockingV2, InspectionRef: &inspection,
			ResidualRefs: []contract.ExactFactRefV2{residual},
		}}
		fact.ResidualRefs = []contract.ExactFactRefV2{residual}
	}
	if state == contract.ManifestDiagnosticIndeterminate {
		fact.AttemptSettlementClosures[0].SettlementRef = nil
		inspection := ExactRefV2("inspection-tool-1", "praxis/runtime", "operation-inspection")
		fact.AttemptSettlementClosures[0].InspectionRef = &inspection
		fact.AttemptSettlementClosures[0].ResidualRefs = append([]contract.ExactFactRefV2{}, fact.ResidualRefs...)
	}
	requiredDigest, err := contract.RequiredParticipantSetDigestV2(fact.ParticipantClosures)
	if err != nil {
		panic(err)
	}
	fact.RequiredParticipantSetDigest = requiredDigest
	frozenDigest, err := contract.FrozenRefSetDigestV2(fact)
	if err != nil {
		panic(err)
	}
	fact.FrozenRefSetDigest = frozenDigest
	digest, err := fact.CanonicalDigest()
	if err != nil {
		panic(err)
	}
	fact.Digest = digest
	return fact
}

func SealV2(manifest contract.CheckpointManifestFactV2) contract.CheckpointManifestSealFactV2 {
	copy := manifest.Clone()
	seal := contract.CheckpointManifestSealFactV2{
		ContractVersion:              contract.CheckpointManifestGovernanceContractV2,
		SchemaRef:                    contract.CheckpointManifestSealSchemaV2,
		SealID:                       "checkpoint-manifest-seal-v2-1",
		Revision:                     1,
		Owner:                        CheckpointOwnerV2("checkpoint_manifest_seal_fact_v2"),
		TenantID:                     manifest.Scope.TenantID,
		ScopeDigest:                  manifest.Scope.ExecutionScopeDigest,
		IdempotencyKey:               "checkpoint-manifest-seal-request-1",
		ManifestRef:                  manifest.Ref(),
		CheckpointAttemptRef:         manifest.CheckpointAttemptRef,
		BarrierRef:                   manifest.BarrierRef,
		EffectCutRef:                 manifest.EffectCutRef,
		FrozenRefSetDigest:           manifest.FrozenRefSetDigest,
		RequiredParticipantSetDigest: manifest.RequiredParticipantSetDigest,
		RuntimeParticipantSetDigest:  manifest.RuntimeParticipantSetDigest,
		ParticipantClosures:          copy.ParticipantClosures,
		CreatedUnixNano:              manifest.UpdatedUnixNano + 1,
	}
	var err error
	seal.ContextClosureDigest, err = contract.ContextClosureDigestV2(manifest)
	if err != nil {
		panic(err)
	}
	seal.ArtifactClosureDigest, err = contract.ArtifactClosureDigestV2(manifest)
	if err != nil {
		panic(err)
	}
	digest, err := seal.CanonicalDigest()
	if err != nil {
		panic(err)
	}
	seal.Digest = digest
	return seal
}

func RestorePlanV2(now time.Time) contract.RestorePlanFactV2 {
	ref := func(id, component, kind string) contract.ExactFactRefV2 { return ExactRefV2(id, component, kind) }
	manifest := ManifestV2(contract.ManifestVerifiedCandidate, 2)
	plan := contract.RestorePlanFactV2{
		ContractVersion: contract.RestorePlanGovernanceContractV2,
		SchemaRef:       contract.RestorePlanFactSchemaV2, PlanID: "restore-plan-v2-1", Revision: 1,
		Owner: RestorePlanOwnerV2(), Scope: Scope(), State: contract.RestorePlanDraftV2,
		IdempotencyKey:           "restore-plan-request-v2-1",
		CheckpointConsistencyRef: ref("checkpoint-consistency-1", "praxis/runtime", "checkpoint_consistency_fact_v2"),
		ManifestSealRef:          SealV2(manifest).Ref(), FrozenRefSetDigest: manifest.FrozenRefSetDigest,
		SourceInstanceRef: ref("instance-source-1", "praxis/runtime", "instance_fact_v2"), SourceInstanceEpoch: 7,
		ProposedInstance:             contract.RestoreInstanceProposalV2{InstanceID: "instance-restored-1", Epoch: 8, LeaseID: "lease-restored-1", LeaseEpoch: 1, FenceEpoch: 8},
		RequiredParticipantSetDigest: manifest.RequiredParticipantSetDigest,
		ContextGenerationRef:         ref("context-generation-7", "praxis/context", "context_generation_fact_v2"),
		ContextFrameRefs:             []contract.ExactFactRefV2{ref("context-frame-7", "praxis/context", "context_frame_fact_v2"), ref("context-frame-8", "praxis/context", "context_frame_fact_v2")},
		CompatibilityRefs:            []contract.ExactFactRefV2{ref("compatibility-1", "praxis/sandbox", "restore_compatibility_fact_v1")},
		CurrentnessRefs:              []contract.ExactFactRefV2{ref("currentness-1", "praxis/runtime", "restore_currentness_requirement_v1")},
		ReviewRequirementRefs:        []contract.ExactFactRefV2{ref("review-requirement-1", "praxis/review", "review_requirement_fact_v2")},
		AuthorityRequirementRefs:     []contract.ExactFactRefV2{ref("authority-requirement-1", "praxis/runtime", "authority_requirement_fact_v2")},
		BudgetRequirementRefs:        []contract.ExactFactRefV2{ref("budget-requirement-1", "praxis/runtime", "budget_requirement_fact_v2")},
		BindingRequirementRefs:       []contract.ExactFactRefV2{ref("binding-requirement-1", "praxis/runtime", "binding_requirement_fact_v2")},
		ConflictDomain:               "tenant/tenant-1/continuity/restore",
		ResidualPolicyRef:            ref("residual-policy-1", "praxis/continuity", "restore_residual_policy_v1"),
		ResidualRefs:                 []contract.ExactFactRefV2{ref("residual-1", "praxis/continuity", "restore_residual_fact_v1")},
		RecoveryCredentialRef:        ref("recovery-credential-1", "praxis/continuity", "recovery_credential_fact_v1"),
		CreatedUnixNano:              now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	}
	RefreshRestorePlanV2(&plan)
	return plan
}

func RefreshRestorePlanV2(plan *contract.RestorePlanFactV2) {
	digest, err := plan.CanonicalDigest()
	if err != nil {
		panic(err)
	}
	plan.Digest = digest
}

func RewindPlanV2(now time.Time) contract.RewindPlanFactV2 {
	ref := func(id, component, kind string) contract.ExactFactRefV2 { return ExactRefV2(id, component, kind) }
	manifest := ManifestV2(contract.ManifestVerifiedCandidate, 2)
	plan := contract.RewindPlanFactV2{
		ContractVersion: contract.RewindPlanGovernanceContractV2,
		SchemaRef:       contract.RewindPlanFactSchemaV2, PlanID: "rewind-plan-v2-1", Revision: 1,
		Owner: RewindPlanOwnerV2(), Scope: Scope(), State: contract.RewindPlanDraftV2,
		IdempotencyKey:            "rewind-plan-request-v2-1",
		CheckpointConsistencyRef:  ref("checkpoint-consistency-1", "praxis/runtime", "checkpoint_consistency_fact_v2"),
		ManifestSealRef:           SealV2(manifest).Ref(),
		SourceWorkspaceViewRef:    ref("workspace-view-7", "praxis/sandbox", "workspace_view_v1"),
		ExpectedWorkspaceRevision: "workspace-revision-7-digest",
		FileScopeDigest:           "workspace-file-scope-digest",
		KeepChangeSetRefs:         []contract.ExactFactRefV2{ref("workspace-change-keep-1", "praxis/sandbox", "workspace_change_set_v1")},
		DropChangeSetRefs:         []contract.ExactFactRefV2{ref("workspace-change-drop-1", "praxis/sandbox", "workspace_change_set_v1")},
		PlannedChangeSetRef:       ref("workspace-change-rewind-1", "praxis/sandbox", "workspace_change_set_v1"),
		DependencyInspectionRefs:  []contract.ExactFactRefV2{ref("rewind-dependency-inspection-1", "praxis/continuity", "rewind_dependency_inspection_v1")},
		ReviewRequirementRefs:     []contract.ExactFactRefV2{ref("rewind-review-requirement-1", "praxis/review", "review_requirement_fact_v2")},
		IrreversibleEffectRefs:    []contract.ExactFactRefV2{ref("sent-mail-effect-1", "praxis/runtime", "operation_settlement_fact_v4")},
		ConflictDomain:            "tenant/tenant-1/sandbox/workspace",
		CreatedUnixNano:           now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	}
	RefreshRewindPlanV2(&plan)
	return plan
}

func RefreshRewindPlanV2(plan *contract.RewindPlanFactV2) {
	selection, err := plan.CanonicalWorkspaceSelectionDigest()
	if err != nil {
		panic(err)
	}
	plan.WorkspaceSelectionDigest = selection
	digest, err := plan.CanonicalDigest()
	if err != nil {
		panic(err)
	}
	plan.Digest = digest
}

func ResidualV2(id, kind, _ string) contract.ExactFactRefV2 {
	return ExactRefV2(id, "praxis/continuity", "checkpoint-residual-"+kind)
}

func InspectManifestRequestV2(ref contract.CheckpointManifestRefV2) ports.InspectCheckpointManifestRequestV2 {
	return ports.InspectCheckpointManifestRequestV2{Ref: ref}
}

func CurrentManifestRequestV2(manifest contract.CheckpointManifestFactV2) ports.InspectCurrentCheckpointManifestRequestV2 {
	return ports.InspectCurrentCheckpointManifestRequestV2{
		TenantID: manifest.Scope.TenantID, ScopeDigest: manifest.Scope.ExecutionScopeDigest,
		ManifestID: manifest.ManifestID, Owner: manifest.Owner,
	}
}

func InspectSealRequestV2(ref contract.CheckpointManifestSealRefV2) ports.InspectCheckpointManifestSealRequestV2 {
	return ports.InspectCheckpointManifestSealRequestV2{Ref: ref}
}

func RefreshManifestV2(fact *contract.CheckpointManifestFactV2) {
	requiredDigest, err := contract.RequiredParticipantSetDigestV2(fact.ParticipantClosures)
	if err != nil {
		panic(err)
	}
	fact.RequiredParticipantSetDigest = requiredDigest
	frozenDigest, err := contract.FrozenRefSetDigestV2(*fact)
	if err != nil {
		panic(err)
	}
	fact.FrozenRefSetDigest = frozenDigest
	digest, err := fact.CanonicalDigest()
	if err != nil {
		panic(err)
	}
	fact.Digest = digest
}

func RefreshSealV2(seal *contract.CheckpointManifestSealFactV2) {
	digest, err := seal.CanonicalDigest()
	if err != nil {
		panic(err)
	}
	seal.Digest = digest
}

func RetargetManifestScopeV2(fact contract.CheckpointManifestFactV2, tenantID, scopeDigest string) contract.CheckpointManifestFactV2 {
	result := fact.Clone()
	result.Scope.TenantID = tenantID
	result.Scope.ExecutionScopeDigest = scopeDigest
	retarget := func(ref *contract.ExactFactRefV2) {
		if ref != nil {
			ref.TenantID = tenantID
			ref.ScopeDigest = scopeDigest
		}
	}
	retarget(&result.CheckpointAttemptRef)
	retarget(&result.BarrierRef)
	retarget(&result.EffectCutRef)
	retarget(&result.TimelineCut.EvidenceRecordRef)
	retarget(&result.ContextGenerationRef)
	for i := range result.ContextFrameRefs {
		retarget(&result.ContextFrameRefs[i])
	}
	for i := range result.MemoryRefs {
		retarget(&result.MemoryRefs[i])
	}
	for i := range result.KnowledgeRefs {
		retarget(&result.KnowledgeRefs[i])
	}
	for i := range result.AttemptSettlementClosures {
		closure := &result.AttemptSettlementClosures[i]
		retarget(&closure.AttemptRef)
		retarget(closure.SettlementRef)
		retarget(closure.InspectionRef)
		for j := range closure.ResidualRefs {
			retarget(&closure.ResidualRefs[j])
		}
	}
	for i := range result.ParticipantClosures {
		closure := &result.ParticipantClosures[i]
		retarget(&closure.RuntimeClosureRef)
		retarget(&closure.ParticipantFactRef)
		retarget(closure.SnapshotRef)
		retarget(closure.CoverageRef)
		for j := range closure.EvidenceRefs {
			retarget(&closure.EvidenceRefs[j])
		}
		for j := range closure.ResidualRefs {
			retarget(&closure.ResidualRefs[j])
		}
	}
	for i := range result.Diagnostics {
		diagnostic := &result.Diagnostics[i]
		retarget(&diagnostic.DiagnosticRef)
		retarget(diagnostic.SubjectRef)
		retarget(diagnostic.InspectionRef)
		for j := range diagnostic.ResidualRefs {
			retarget(&diagnostic.ResidualRefs[j])
		}
	}
	for i := range result.ResidualRefs {
		retarget(&result.ResidualRefs[i])
	}
	RefreshManifestV2(&result)
	return result
}
