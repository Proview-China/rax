package contract_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
)

func TestRestorePlanV2CanonicalShapeAndCurrentTTL(t *testing.T) {
	now := time.Unix(1_752_577_200, 0)
	plan := restorePlanV2(t, now)
	if err := plan.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if err := plan.ValidateCurrent(now); err != nil {
		t.Fatalf("ValidateCurrent() error = %v", err)
	}
	if err := plan.ValidateCurrent(time.Unix(0, plan.ExpiresUnixNano)); !contract.HasCode(err, contract.ErrRestoreIncompatible) {
		t.Fatalf("expiry boundary error = %v, want restore incompatible", err)
	}

	reordered := plan.Clone()
	reordered.ContextFrameRefs[0], reordered.ContextFrameRefs[1] = reordered.ContextFrameRefs[1], reordered.ContextFrameRefs[0]
	digest, err := reordered.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	if digest != plan.Digest {
		t.Fatal("set ordering changed canonical digest")
	}
}

func TestRestorePlanV2RejectsScopeSpliceAndOldInstance(t *testing.T) {
	now := time.Unix(1_752_577_200, 0)
	tests := []struct {
		name   string
		mutate func(*contract.RestorePlanFactV2)
	}{
		{name: "cross tenant", mutate: func(p *contract.RestorePlanFactV2) { p.CurrentnessRefs[0].TenantID = "tenant-other" }},
		{name: "cross scope", mutate: func(p *contract.RestorePlanFactV2) { p.ReviewRequirementRefs[0].ScopeDigest = "scope-other" }},
		{name: "same instance", mutate: func(p *contract.RestorePlanFactV2) { p.ProposedInstance.InstanceID = p.SourceInstanceRef.ID }},
		{name: "old epoch", mutate: func(p *contract.RestorePlanFactV2) { p.ProposedInstance.Epoch = p.SourceInstanceEpoch }},
		{name: "foreign conflict domain", mutate: func(p *contract.RestorePlanFactV2) { p.ConflictDomain = "tenant/tenant-other/continuity/restore" }},
		{name: "missing review", mutate: func(p *contract.RestorePlanFactV2) { p.ReviewRequirementRefs = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := restorePlanV2(t, now)
			tt.mutate(&plan)
			refreshRestorePlanV2(t, &plan)
			if err := plan.Validate(); err == nil {
				t.Fatal("Validate() succeeded, want fail closed")
			}
		})
	}
}

func TestRestorePlanV2CloneHasNoAliasAndCarriesNoExecutionAuthority(t *testing.T) {
	plan := restorePlanV2(t, time.Unix(1_752_577_200, 0))
	clone := plan.Clone()
	clone.ContextFrameRefs[0].ID = "mutated"
	clone.ResidualRefs[0].ID = "mutated-residual"
	if plan.ContextFrameRefs[0].ID == "mutated" || plan.ResidualRefs[0].ID == "mutated-residual" {
		t.Fatal("Clone() aliases input slices")
	}
	for _, forbidden := range []string{"Eligibility", "Authorization", "Permit", "Execute", "Provider", "Activation", "RuntimeOutcome"} {
		if _, ok := reflect.TypeOf(plan).FieldByName(forbidden); ok {
			t.Fatalf("RestorePlanFactV2 must not carry %s", forbidden)
		}
	}
}

func TestRestorePlanV2StateMachineAndExpiry(t *testing.T) {
	now := time.Unix(1_752_577_200, 0)
	plan := restorePlanV2(t, now)
	sequence := []contract.RestorePlanStateV2{
		contract.RestorePlanCheckpointInspectedV2,
		contract.RestorePlanCompatibilityInspectedV2,
		contract.RestorePlanAdmittedV2,
		contract.RestorePlanSubmittedV2,
	}
	for _, next := range sequence {
		if err := contract.AdvanceRestorePlanStateV2(plan, next, now); err != nil {
			t.Fatalf("%s -> %s: %v", plan.State, next, err)
		}
		plan.State = next
	}
	if err := contract.AdvanceRestorePlanStateV2(plan, contract.RestorePlanAdmittedV2, now); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("terminal replay error = %v", err)
	}

	plan = restorePlanV2(t, now)
	if err := contract.AdvanceRestorePlanStateV2(plan, contract.RestorePlanExpiredV2, now); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("early expiry error = %v", err)
	}
	atExpiry := time.Unix(0, plan.ExpiresUnixNano)
	if err := contract.AdvanceRestorePlanStateV2(plan, contract.RestorePlanExpiredV2, atExpiry); err != nil {
		t.Fatalf("expiry transition error = %v", err)
	}
}

func restorePlanV2(t *testing.T, now time.Time) contract.RestorePlanFactV2 {
	t.Helper()
	ref := func(id, component, kind string) contract.ExactFactRefV2 {
		return testkit.ExactRefV2(id, component, kind)
	}
	manifest := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
	seal := testkit.SealV2(manifest)
	plan := contract.RestorePlanFactV2{
		ContractVersion: contract.RestorePlanGovernanceContractV2,
		SchemaRef:       contract.RestorePlanFactSchemaV2,
		PlanID:          "restore-plan-v2-1",
		Revision:        1,
		Owner: contract.OwnerBinding{
			BindingSetID: "binding-set-restore-plan-v2", BindingRevision: 2,
			ComponentID: contract.ContinuityComponentID, ManifestDigest: "continuity-manifest-digest-v2",
			ArtifactDigest: "continuity-artifact-digest-v2", Capability: contract.RestorePlanCapabilityV2,
			FactKind: "restore_plan_fact_v2",
		},
		Scope:                    testkit.Scope(),
		State:                    contract.RestorePlanDraftV2,
		IdempotencyKey:           "restore-plan-request-v2-1",
		CheckpointConsistencyRef: ref("checkpoint-consistency-1", "praxis/runtime", "checkpoint_consistency_fact_v2"),
		ManifestSealRef:          seal.Ref(),
		FrozenRefSetDigest:       manifest.FrozenRefSetDigest,
		SourceInstanceRef:        ref("instance-source-1", "praxis/runtime", "instance_fact_v2"),
		SourceInstanceEpoch:      7,
		ProposedInstance: contract.RestoreInstanceProposalV2{
			InstanceID: "instance-restored-1", Epoch: 8, LeaseID: "lease-restored-1", LeaseEpoch: 1, FenceEpoch: 8,
		},
		RequiredParticipantSetDigest: manifest.RequiredParticipantSetDigest,
		ContextGenerationRef:         ref("context-generation-7", "praxis/context", "context_generation_fact_v2"),
		ContextFrameRefs: []contract.ExactFactRefV2{
			ref("context-frame-7", "praxis/context", "context_frame_fact_v2"),
			ref("context-frame-8", "praxis/context", "context_frame_fact_v2"),
		},
		CompatibilityRefs:        []contract.ExactFactRefV2{ref("compatibility-1", "praxis/sandbox", "restore_compatibility_fact_v1")},
		CurrentnessRefs:          []contract.ExactFactRefV2{ref("currentness-1", "praxis/runtime", "restore_currentness_requirement_v1")},
		ReviewRequirementRefs:    []contract.ExactFactRefV2{ref("review-requirement-1", "praxis/review", "review_requirement_fact_v2")},
		AuthorityRequirementRefs: []contract.ExactFactRefV2{ref("authority-requirement-1", "praxis/runtime", "authority_requirement_fact_v2")},
		BudgetRequirementRefs:    []contract.ExactFactRefV2{ref("budget-requirement-1", "praxis/runtime", "budget_requirement_fact_v2")},
		BindingRequirementRefs:   []contract.ExactFactRefV2{ref("binding-requirement-1", "praxis/runtime", "binding_requirement_fact_v2")},
		ConflictDomain:           "tenant/tenant-1/continuity/restore",
		ResidualPolicyRef:        ref("residual-policy-1", "praxis/continuity", "restore_residual_policy_v1"),
		ResidualRefs:             []contract.ExactFactRefV2{ref("residual-1", "praxis/continuity", "restore_residual_fact_v1")},
		RecoveryCredentialRef:    ref("recovery-credential-1", "praxis/continuity", "recovery_credential_fact_v1"),
		CreatedUnixNano:          now.UnixNano(),
		UpdatedUnixNano:          now.UnixNano(),
		ExpiresUnixNano:          now.Add(time.Hour).UnixNano(),
	}
	refreshRestorePlanV2(t, &plan)
	return plan
}

func refreshRestorePlanV2(t *testing.T, plan *contract.RestorePlanFactV2) {
	t.Helper()
	digest, err := plan.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	plan.Digest = digest
}
