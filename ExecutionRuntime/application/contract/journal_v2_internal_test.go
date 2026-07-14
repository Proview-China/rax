package contract

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestWorkflowStepProgressV2FieldMatrix(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UnixNano()
	digest := core.DigestBytes([]byte("fact"))
	effect := &ApplicationFactRefV2{Ref: "effect-1", Revision: 1, Digest: digest}
	settlement := &ApplicationFactRefV2{Ref: "settlement-1", Revision: 1, Digest: digest}
	valid := []WorkflowStepProgressV2{
		{StepID: "step", State: StepPendingV2, UpdatedUnixNano: now},
		{StepID: "step", State: StepReadyV2, UpdatedUnixNano: now},
		{StepID: "step", State: StepDispatchIntentV2, Attempt: 1, Effect: effect, UpdatedUnixNano: now},
		{StepID: "step", State: StepWaitingInspectV2, Attempt: 1, Effect: effect, UpdatedUnixNano: now},
		{StepID: "step", State: StepCompletedV2, Attempt: 1, Settlement: settlement, UpdatedUnixNano: now},
		{StepID: "step", State: StepSkippedV2, LastError: "optional", UpdatedUnixNano: now},
		{StepID: "step", State: StepIndeterminateV2, Attempt: 1, Effect: effect, LastError: "unknown", UpdatedUnixNano: now},
		{StepID: "step", State: StepBlockedV2, LastError: "blocked", UpdatedUnixNano: now},
	}
	for _, progress := range valid {
		if err := progress.Validate(); err != nil {
			t.Fatalf("valid %s rejected: %v", progress.State, err)
		}
	}
	for _, progress := range valid {
		progress.Attempt, progress.Effect, progress.Settlement, progress.LastError = 0, nil, nil, ""
		if progress.State == StepPendingV2 || progress.State == StepReadyV2 {
			progress.Effect = effect
		}
		if err := progress.Validate(); err == nil {
			t.Fatalf("invalid %s fields accepted", progress.State)
		}
	}
}

func workflowContractFixtureV2(t *testing.T, now time.Time, class StepExecutionClassV2) WorkflowPlanV2 {
	t.Helper()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant", ID: "agent", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: digest("lineage")}, Instance: core.InstanceRef{ID: "instance", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "sandbox", Epoch: 1}, AuthorityEpoch: 1}
	schema := runtimeports.SchemaRefV2{Namespace: "custom", Name: "payload", Version: "1.0.0", MediaType: "application/octet-stream", ContentDigest: digest("schema")}
	bytes := []byte("payload")
	payload := runtimeports.OpaquePayloadV2{Schema: schema, ContentDigest: core.DigestBytes(bytes), Length: uint64(len(bytes)), Inline: bytes, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "custom/limit", Digest: digest("limit")}}
	step := WorkflowStepV2{ID: "step-a", Kind: "custom/step", Descriptor: StepDescriptorRefV2{Kind: "custom/step", Revision: 1, Digest: digest("descriptor"), ExpiresUnixNano: now.Add(2 * time.Hour).UnixNano()}, ExecutionClass: class, Required: true, Dependencies: []string{}, Payload: payload}
	if class == StepGovernedEffectV2 {
		step.Provider = &runtimeports.ProviderBindingRefV2{BindingSetID: "binding", BindingSetRevision: 1, ComponentID: "custom/provider", ManifestDigest: digest("manifest"), ArtifactDigest: digest("artifact"), Capability: "custom/execute"}
		step.DomainAdapter = &runtimeports.ProviderBindingRefV2{BindingSetID: "binding", BindingSetRevision: 1, ComponentID: "custom/domain-adapter", ManifestDigest: digest("domain-manifest"), ArtifactDigest: digest("domain-artifact"), Capability: "custom/domain-state"}
	}
	plan := WorkflowPlanV2{ContractVersion: WorkflowContractVersionV2, ID: "plan", Revision: 1, CommandID: "command", CommandPayloadDigest: digest("command-payload"), Target: scope, Authority: runtimeports.AuthorityBindingRefV2{Ref: "authority", Digest: digest("authority"), Revision: 1, Epoch: 1}, Steps: []WorkflowStepV2{step}, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	if err := plan.Validate(now); err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestDeriveWorkflowStatusV2Precedence(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UnixNano()
	digest := core.DigestBytes([]byte("fact"))
	effect := &ApplicationFactRefV2{Ref: "effect", Revision: 1, Digest: digest}
	settlement := &ApplicationFactRefV2{Ref: "settlement", Revision: 1, Digest: digest}
	if got := DeriveWorkflowStatusV2([]WorkflowStepProgressV2{{StepID: "a", State: StepCompletedV2, Attempt: 1, Settlement: settlement, UpdatedUnixNano: now}, {StepID: "b", State: StepSkippedV2, LastError: "optional", UpdatedUnixNano: now}}); got != WorkflowCompletedV2 {
		t.Fatalf("terminal status=%s", got)
	}
	if got := DeriveWorkflowStatusV2([]WorkflowStepProgressV2{{StepID: "a", State: StepWaitingInspectV2, Attempt: 1, Effect: effect, UpdatedUnixNano: now}, {StepID: "b", State: StepIndeterminateV2, Attempt: 1, Effect: effect, LastError: "unknown", UpdatedUnixNano: now}}); got != WorkflowIndeterminateV2 {
		t.Fatalf("indeterminate did not dominate waiting: %s", got)
	}
}

func TestWorkflowJournalTransitionV2EnforcesExecutionClassPath(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	governed := workflowContractFixtureV2(t, now, StepGovernedEffectV2)
	journal, err := NewWorkflowJournalV2("journal-governed", governed, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	settlement := &ApplicationFactRefV2{Ref: "settlement-1", Revision: 1, Digest: core.DigestBytes([]byte("settlement"))}
	forged := journal
	forged.Revision++
	forged.UpdatedUnixNano++
	forged.Steps = append([]WorkflowStepProgressV2(nil), journal.Steps...)
	forged.Steps[0] = WorkflowStepProgressV2{StepID: journal.Steps[0].StepID, State: StepCompletedV2, Attempt: 1, Settlement: settlement, UpdatedUnixNano: forged.UpdatedUnixNano}
	forged.Status = DeriveWorkflowStatusV2(forged.Steps)
	if err := ValidateWorkflowJournalTransitionV2(governed, journal, forged); err == nil {
		t.Fatal("governed effect completed directly from ready")
	}

	coordination := workflowContractFixtureV2(t, now, StepCoordinationV2)
	journal, err = NewWorkflowJournalV2("journal-coordination", coordination, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	effect := &ApplicationFactRefV2{Ref: "effect-1", Revision: 1, Digest: core.DigestBytes([]byte("effect"))}
	forged = journal
	forged.Revision++
	forged.UpdatedUnixNano++
	forged.Steps = append([]WorkflowStepProgressV2(nil), journal.Steps...)
	forged.Steps[0] = WorkflowStepProgressV2{StepID: journal.Steps[0].StepID, State: StepDispatchIntentV2, Attempt: 1, Effect: effect, UpdatedUnixNano: forged.UpdatedUnixNano}
	forged.Status = DeriveWorkflowStatusV2(forged.Steps)
	if err := ValidateWorkflowJournalTransitionV2(coordination, journal, forged); err == nil {
		t.Fatal("coordination step entered provider dispatch")
	}
}
