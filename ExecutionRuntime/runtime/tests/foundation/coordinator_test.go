package foundation_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/admission"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/foundation"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestFoundationStartRunV2PersistsBundleBeforeExecutionAndRecoversReplies(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	coordinator, activation, _, _ := foundationFixture(t, now, ports.ConformanceFullyControlled, false)
	instance, err := coordinator.Activate(context.Background(), activation)
	if err != nil {
		t.Fatal(err)
	}
	runStore := fakes.NewRunSettlementStoreV2(func() time.Time { return now })
	effectStore := fakes.NewEffectStoreV2(func() time.Time { return now })
	effectStore.SetRunFacts(runStore)
	coordinator.RunSettlements = runStore
	coordinator.RunEffects = effectStore
	coordinator.RunSettlement = &kernel.RunSettlementGatewayV2{}
	scope := instance.Snapshot().Kernel.Scope
	session, _ := ports.DeriveRuntimeExecutionSessionRefV2("foundation-endpoint", "run-foundation-v2")
	run := core.AgentRunRecord{ID: "run-foundation-v2", Scope: scope, Status: core.RunPending, Revision: 1, SessionRef: session}
	plan := foundationRunSettlementPlanV2(t, run, now)
	index := control.RunEffectIndexFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: "effect-index-foundation-v2", Revision: 1, RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, State: control.RunEffectIndexOpen, HeadSegmentDigest: ports.EvidenceGenesisDigestV2, Watermark: 1, CreatedUnixNano: now.UnixNano()}
	runStore.LoseNextBundleReply()
	effectStore.LoseNextRunEffectReply()
	result, err := coordinator.StartRunV2(context.Background(), instance, foundation.StartRunRequestV2{Bundle: control.RunBundleCreateRequestV2{Run: run, Plan: plan}, EffectIndex: index})
	if err != nil {
		t.Fatal(err)
	}
	if result.Bundle.Run.Status != core.RunPending || result.EffectIndex.State != control.RunEffectIndexOpen {
		t.Fatalf("persistent V2 preparation did not reach recoverable boundary: %+v", result)
	}
	// A fresh coordinator has no RunRegistry state, yet the Run and Plan remain
	// recoverable solely through the persistent V2 owner.
	restarted := &foundation.Coordinator{RunSettlements: runStore, RunEffects: effectStore, RunSettlement: &kernel.RunSettlementGatewayV2{}}
	inspected, err := restarted.RunSettlements.InspectRun(context.Background(), scope, run.ID)
	if err != nil || inspected.ID != run.ID {
		t.Fatalf("restart could not recover persistent Run: %+v %v", inspected, err)
	}
	if _, err := restarted.RunSettlements.InspectRunSettlementPlanV2(context.Background(), scope, run.ID); err != nil {
		t.Fatalf("restart could not recover create-time Plan: %v", err)
	}
}

func TestFoundationStartRunV2RejectsConflictingFactsAfterUnknownReplies(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 9, 5, 0, 0, time.UTC)
	coordinator, activation, _, _ := foundationFixture(t, now, ports.ConformanceFullyControlled, false)
	instance, err := coordinator.Activate(context.Background(), activation)
	if err != nil {
		t.Fatal(err)
	}
	scope := instance.Snapshot().Kernel.Scope
	session, _ := ports.DeriveRuntimeExecutionSessionRefV2("foundation-endpoint", "run-foundation-conflict")
	run := core.AgentRunRecord{ID: "run-foundation-conflict", Scope: scope, Status: core.RunPending, Revision: 1, SessionRef: session}
	plan := foundationRunSettlementPlanV2(t, run, now)
	index := control.RunEffectIndexFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: "effect-index-foundation-conflict", Revision: 1, RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, State: control.RunEffectIndexOpen, HeadSegmentDigest: ports.EvidenceGenesisDigestV2, Watermark: 1, CreatedUnixNano: now.UnixNano()}

	conflictingRun := run
	conflictingRun.SessionRef, _ = ports.DeriveRuntimeExecutionSessionRefV2("foundation-endpoint", "run-foundation-conflict-other")
	coordinator.RunSettlements = conflictingRunSettlementPortV2{run: conflictingRun, plan: plan}
	coordinator.RunEffects = conflictingRunEffectPortV2{index: index}
	coordinator.RunSettlement = &kernel.RunSettlementGatewayV2{}
	request := foundation.StartRunRequestV2{Bundle: control.RunBundleCreateRequestV2{Run: run, Plan: plan}, EffectIndex: index}
	if _, err := coordinator.StartRunV2(context.Background(), instance, request); !core.HasReason(err, core.ReasonRunSettlementPlanConflict) {
		t.Fatalf("unknown bundle reply accepted conflicting inspected Run: %v", err)
	}

	realRuns := fakes.NewRunSettlementStoreV2(func() time.Time { return now })
	coordinator.RunSettlements = realRuns
	conflictingIndex := index
	conflictingIndex.ID = "effect-index-conflicting-persisted"
	coordinator.RunEffects = conflictingRunEffectPortV2{index: conflictingIndex}
	if _, err := coordinator.StartRunV2(context.Background(), instance, request); !core.HasReason(err, core.ReasonRunEffectIndexConflict) {
		t.Fatalf("unknown index reply accepted conflicting inspected index: %v", err)
	}
}

type conflictingRunSettlementPortV2 struct {
	control.RunSettlementFactPortV2
	run  core.AgentRunRecord
	plan ports.RunSettlementPlanFactV2
}

func (p conflictingRunSettlementPortV2) CreateRunBundleV2(context.Context, control.RunBundleCreateRequestV2) (control.RunBundleV2, error) {
	return control.RunBundleV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected unknown bundle result")
}

func (p conflictingRunSettlementPortV2) InspectRun(context.Context, core.ExecutionScope, core.AgentRunID) (core.AgentRunRecord, error) {
	return p.run, nil
}

func (p conflictingRunSettlementPortV2) InspectRunSettlementPlanV2(context.Context, core.ExecutionScope, core.AgentRunID) (ports.RunSettlementPlanFactV2, error) {
	return p.plan, nil
}

type conflictingRunEffectPortV2 struct {
	control.RunEffectFactPortV2
	index control.RunEffectIndexFactV2
}

func (p conflictingRunEffectPortV2) CreateRunEffectIndexV2(context.Context, control.RunEffectIndexFactV2) (control.RunEffectIndexFactV2, error) {
	return control.RunEffectIndexFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected unknown index result")
}

func (p conflictingRunEffectPortV2) InspectRunEffectIndexV2(context.Context, control.RunEffectPartitionV2) (control.RunEffectIndexFactV2, error) {
	return p.index, nil
}

func TestFoundationLifecycleAcrossExecutionConformanceLevels(t *testing.T) {
	for _, conformance := range []ports.ConformanceLevel{
		ports.ConformanceFullyControlled,
		ports.ConformanceRestrictedControlled,
	} {
		conformance := conformance
		t.Run(string(conformance), func(t *testing.T) {
			t.Parallel()
			now := time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)
			coordinator, request, store, _ := foundationFixture(t, now, conformance, false)
			ctx := context.Background()

			instance, err := coordinator.Activate(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			activated := instance.Snapshot()
			if activated.Kernel.State.Phase != core.PhaseReady || activated.Activation.Stage != admission.StageSandboxActive || activated.IdentityLease.State != control.IdentityLeaseActive {
				t.Fatalf("activation did not establish ready authority: %+v", activated)
			}

			run, err := coordinator.StartRun(ctx, instance, "run-1", "session-1")
			if err != nil {
				t.Fatal(err)
			}
			if run.Status != core.RunRunning || instance.Snapshot().Kernel.State.Phase != core.PhaseRunning {
				t.Fatalf("run did not enter running: %+v", run)
			}
			if _, err := coordinator.StartRun(ctx, instance, "run-2", "session-2"); !core.HasReason(err, core.ReasonRunConflict) {
				t.Fatalf("second active run must conflict: %v", err)
			}

			checkpoint, err := coordinator.Checkpoint(ctx, instance, checkpointRequest(now))
			if err != nil {
				t.Fatal(err)
			}
			if checkpoint.Consistency != core.CheckpointConsistent || len(checkpoint.Participants) != 1 {
				t.Fatalf("checkpoint is not consistent: %+v", checkpoint)
			}
			restore, err := coordinator.Restore(ctx, core.RestoreRequest{
				Checkpoint:            checkpoint,
				NewInstance:           core.InstanceRef{ID: "instance-restored", Epoch: checkpoint.Scope.Instance.Epoch + 1},
				CurrentPlanDigest:     checkpoint.PlanDigest,
				CurrentProfileDigest:  checkpoint.ProfileDigest,
				CurrentAuthorityEpoch: checkpoint.AuthorityEpoch + 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if restore.ProposedScope.Instance.ID == checkpoint.Scope.Instance.ID || restore.ProposedScope.SandboxLease != nil || len(restore.Reports) != 1 {
				t.Fatalf("restore must propose a fresh lease-free instance: %+v", restore)
			}

			report, err := coordinator.Stop(ctx, instance, foundation.StopRequest{
				Outcome: core.OutcomeCompleted, Reason: "test complete",
				CloseIntent:   effectIntent(t, "close", request.ProposedScope),
				ReleaseIntent: effectIntent(t, "release", request.ProposedScope),
			})
			if err != nil {
				t.Fatal(err)
			}
			if report.State.Phase != core.PhaseTerminal || report.State.Cleanup != core.CleanupComplete {
				t.Fatalf("termination did not close every owned obligation: %+v", report)
			}
			lease, err := store.InspectIdentityLease(ctx, request.ProposedScope.Identity.TenantID, request.ProposedScope.Identity.ID)
			if err != nil {
				t.Fatal(err)
			}
			if lease.State != control.IdentityLeaseReleased {
				t.Fatalf("identity lease was not released: %+v", lease)
			}
		})
	}
}

func TestFoundationRejectsIndependentReadyConflict(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 10, 0, 0, time.UTC)
	coordinator, request, _, _ := foundationFixture(t, now, ports.ConformanceFullyControlled, true)
	if _, err := coordinator.Activate(context.Background(), request); !core.HasReason(err, core.ReasonReadyEvidenceIncomplete) {
		t.Fatalf("self-reported endpoint without independent ready evidence must fail: %v", err)
	}
}

func TestFoundationCheckpointPrepareFailureReturnsPartialAndAbortsPreparedParticipants(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 20, 0, 0, time.UTC)
	coordinator, request, _, _ := foundationFixture(t, now, ports.ConformanceFullyControlled, false)
	failing, err := fakes.NewFakeCheckpointParticipant("checkpoint-failing", ports.ComponentCheckpoint, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	failing.FailPrepare = true
	coordinator.CheckpointParticipants = append(coordinator.CheckpointParticipants, foundation.CheckpointBinding{Port: failing, Required: true})
	instance, err := coordinator.Activate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	partial, err := coordinator.Checkpoint(context.Background(), instance, checkpointRequest(now))
	if !core.HasReason(err, core.ReasonCheckpointInconsistent) {
		t.Fatalf("injected prepare failure must surface: %v", err)
	}
	if partial.Consistency != core.CheckpointPartial || len(partial.Participants) != 1 {
		t.Fatalf("prepare failure must return the partial checkpoint evidence: %+v", partial)
	}
	if partial.Participants[0].State != core.CheckpointParticipantAborted {
		t.Fatalf("prepared participants must be durably aborted after peer failure: %+v", partial.Participants)
	}
}

func TestFoundationLostAllocateReplyQuarantinesAndOnlyPlansInspect(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 30, 0, 0, time.UTC)
	coordinator, request, store, environment := foundationFixture(t, now, ports.ConformanceFullyControlled, false)
	environment.SetFaults(fakes.FakeEnvironmentFaults{LoseAllocateReply: true})
	if _, err := coordinator.Activate(context.Background(), request); err == nil {
		t.Fatal("lost allocation reply must not be reported as activation success")
	}
	// Recovery planning needs only the durable journal, so a freshly restarted
	// process does not need to reconstruct every component before deciding.
	restarted := &foundation.Coordinator{ActivationFacts: store, Clock: func() time.Time { return now }}
	attempt, decision, err := restarted.RecoverActivation(context.Background(), request.ActivationAttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.Recovery != admission.RecoveryQuarantined || attempt.SandboxReservation.State != admission.OperationUnknownOutcome || decision.Action != admission.ActionInspectSandboxReservation || decision.AutomaticSafe {
		t.Fatalf("unknown allocation must remain quarantined for inspect: attempt=%+v decision=%+v", attempt, decision)
	}
	allocate, activate, _ := environment.OperationCounts()
	if allocate != 1 || activate != 0 {
		t.Fatalf("recovery planning must not redispatch external effects: allocate=%d activate=%d", allocate, activate)
	}
}

func TestFoundationLostActivateReplyQuarantinesAndOnlyPlansInspect(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 40, 0, 0, time.UTC)
	coordinator, request, _, environment := foundationFixture(t, now, ports.ConformanceFullyControlled, false)
	environment.SetFaults(fakes.FakeEnvironmentFaults{LoseActivateReply: true})
	if _, err := coordinator.Activate(context.Background(), request); err == nil {
		t.Fatal("lost activation reply must not be reported as ready")
	}
	attempt, decision, err := coordinator.RecoverActivation(context.Background(), request.ActivationAttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.Stage != admission.StageCommitted || attempt.Recovery != admission.RecoveryQuarantined || attempt.SandboxActivation.State != admission.OperationUnknownOutcome || decision.Action != admission.ActionInspectSandboxActivation || decision.AutomaticSafe {
		t.Fatalf("unknown sandbox activation must remain quarantined for inspect: attempt=%+v decision=%+v", attempt, decision)
	}
	allocate, activate, _ := environment.OperationCounts()
	if allocate != 1 || activate != 1 {
		t.Fatalf("recovery planning must not redispatch activation: allocate=%d activate=%d", allocate, activate)
	}
}

func TestFoundationLostReleaseReplyCannotClaimTerminalCleanup(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 50, 0, 0, time.UTC)
	coordinator, request, store, environment := foundationFixture(t, now, ports.ConformanceFullyControlled, false)
	instance, err := coordinator.Activate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	environment.SetFaults(fakes.FakeEnvironmentFaults{LoseReleaseReply: true})
	_, err = coordinator.Stop(context.Background(), instance, foundation.StopRequest{
		Outcome: core.OutcomeCancelled, Reason: "fault injection",
		CloseIntent:   effectIntent(t, "close-lost-release", request.ProposedScope),
		ReleaseIntent: effectIntent(t, "release-lost-release", request.ProposedScope),
	})
	if !core.HasReason(err, core.ReasonCleanupEvidenceIncomplete) {
		t.Fatalf("lost cleanup reply must remain indeterminate: %v", err)
	}
	if snapshot := instance.Snapshot(); snapshot.Kernel.State.Phase == core.PhaseTerminal || snapshot.Kernel.State.Cleanup == core.CleanupComplete {
		t.Fatalf("incomplete cleanup was falsely reported terminal: %+v", snapshot)
	}
	lease, err := store.InspectIdentityLease(context.Background(), request.ProposedScope.Identity.TenantID, request.ProposedScope.Identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lease.State != control.IdentityLeaseActive {
		t.Fatalf("identity authority must remain held until cleanup is authoritatively settled: %+v", lease)
	}
}

func foundationFixture(t *testing.T, now time.Time, conformance ports.ConformanceLevel, conflictReady bool) (*foundation.Coordinator, foundation.ActivationRequest, *fakes.FactStore, *fakes.FakeEnvironment) {
	t.Helper()
	clock := func() time.Time { return now }
	execution, err := fakes.NewFakeExecution("execution-"+string(conformance), conformance, clock)
	if err != nil {
		t.Fatal(err)
	}
	if conflictReady {
		execution.SetInspectState("not_ready")
	}
	environment, err := fakes.NewFakeEnvironment("sandbox", clock)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := fakes.NewFakeEvidence("evidence", clock)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := fakes.NewFakeCheckpointParticipant("checkpoint", ports.ComponentCheckpoint, clock)
	if err != nil {
		t.Fatal(err)
	}
	registry := ports.NewComponentRegistry()
	adapters := []ports.Describer{execution, environment, evidence, checkpoint}
	descriptors := make([]ports.ComponentDescriptor, 0, len(adapters))
	for _, adapter := range adapters {
		if err := registry.Register(context.Background(), adapter); err != nil {
			t.Fatal(err)
		}
		descriptor, err := adapter.Describe(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		descriptors = append(descriptors, descriptor)
	}
	planDigest := digest(t, "foundation-plan-"+string(conformance))
	plan := ports.ResolvedAgentPlan{
		ID: "foundation-plan", Digest: planDigest,
		ProfileDigest: digest(t, "profile"), ContextDigest: digest(t, "context"), AuthorityDigest: digest(t, "authority"),
	}
	for _, descriptor := range descriptors {
		plan.Requirements = append(plan.Requirements, ports.ComponentRequirement{
			ID: descriptor.ID, Kind: descriptor.Kind, Version: descriptor.Version,
			ArtifactDigest: descriptor.ArtifactDigest, Required: true,
		})
	}
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-1", ID: core.AgentIdentityID("agent-" + string(conformance)), Epoch: 1},
		Lineage:        core.LineageRef{ID: core.InstanceLineageID("lineage-" + string(conformance)), PlanDigest: planDigest},
		Instance:       core.InstanceRef{ID: core.AgentInstanceID("instance-" + string(conformance)), Epoch: 1},
		AuthorityEpoch: 1,
	}
	store := fakes.NewFactStore(clock)
	coordinator := &foundation.Coordinator{
		Registry: registry, Execution: execution, Environment: environment, Evidence: evidence,
		IdentityLeases: store, ActivationFacts: store, Clock: clock,
		CheckpointParticipants: []foundation.CheckpointBinding{{Port: checkpoint, Required: true}},
	}
	request := foundation.ActivationRequest{
		Plan: plan, ProposedScope: scope,
		ActivationAttemptID: fmt.Sprintf("activation-%s-%t", conformance, conflictReady),
		RequirementDigest:   digest(t, "sandbox-requirement"), CapabilityGrantDigest: digest(t, "capability-grant"),
		ProbeBudget:            ports.ProbeBudget{MaxRequests: 1, MaxDuration: time.Second},
		IdentityLeaseExpiresAt: now.Add(2 * time.Hour), FenceTTL: time.Minute,
		AllocateIntent: effectIntent(t, "allocate", scope),
		ActivateIntent: effectIntent(t, "activate", scope),
		OpenIntent:     effectIntent(t, "open", scope),
	}
	return coordinator, request, store, environment
}

func checkpointRequest(now time.Time) foundation.CheckpointRequest {
	return foundation.CheckpointRequest{
		ID: "checkpoint-1", Epoch: 1, BarrierID: "barrier-1",
		Effects: core.EffectWatermarks{Accepted: 3, Dispatched: 3, Settled: 2, Remote: 0},
		EventWatermark: core.TimelinePoint{
			LedgerScope: "tenant-1", LedgerSequence: 7, SourceID: "execution", SourceEpoch: 1,
			SourceSequence: 5, EventID: "event-5", CausationID: "run-1", CorrelationID: "session-1",
			ObservedAt: now, IngestedAt: now,
		},
	}
}

func effectIntent(t *testing.T, id string, scope core.ExecutionScope) core.EffectIntent {
	t.Helper()
	return core.EffectIntent{
		ID: core.EffectIntentID("effect-" + id), Revision: 1, Kind: core.EffectKindResourceLifecycle,
		RiskClass: "foundation-test", CanonicalPayloadDigest: digest(t, id), Target: id,
		ConflictEffectDomain: "foundation/" + id,
		Ownership: core.EffectOwnership{
			IntentOwner:     core.OwnerRef{Domain: "runtime", ID: "foundation"},
			SettlementOwner: core.OwnerRef{Domain: "runtime", ID: "foundation"},
		},
		AuthorizationRef: "authorization-1", IdempotencyClass: core.IdempotencyQueryable,
	}
}

func digest(t *testing.T, value string) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func foundationRunSettlementPlanV2(t *testing.T, run core.AgentRunRecord, now time.Time) ports.RunSettlementPlanFactV2 {
	t.Helper()
	runIdentity, _ := ports.RunIdentityDigestV2(run)
	scopeDigest, _ := ports.ExecutionScopeDigestV2(run.Scope)
	owner := ports.EvidenceProducerBindingRefV2{BindingSetID: "binding-set-foundation", BindingSetRevision: 1, ComponentID: "runtime/foundation", ManifestDigest: digest(t, "foundation-manifest"), ArtifactDigest: digest(t, "foundation-artifact"), Capability: "runtime/settle-run"}
	schema := ports.SchemaRefV2{Namespace: "runtime", Name: "settlement", Version: "2.0.0", MediaType: "application/octet-stream", ContentDigest: digest(t, "foundation-settlement-schema")}
	kinds := []struct {
		kind  ports.NamespacedNameV2
		phase ports.RunSettlementRequirementPhaseV2
	}{{ports.RunRequirementExecutionTruth, ports.RunSettlementPhaseCompletion}, {ports.RunRequirementEffects, ports.RunSettlementPhaseCompletion}, {ports.RunRequirementRemoteContinuations, ports.RunSettlementPhaseCompletion}, {ports.RunRequirementDomainCommits, ports.RunSettlementPhaseCompletion}, {ports.RunRequirementBudget, ports.RunSettlementPhaseCompletion}, {ports.RunRequirementCleanup, ports.RunSettlementPhaseTerminationReport}, {ports.RunRequirementResidual, ports.RunSettlementPhaseTerminationReport}, {ports.RunRequirementProviderRetention, ports.RunSettlementPhaseTerminationReport}}
	requirements := make([]ports.RunSettlementRequirementV2, 0, len(kinds))
	for _, item := range kinds {
		policy := ports.RunSettlementPolicyBindingRefV2{Ref: "foundation-policy-" + string(item.kind[8:]), Revision: 1, Digest: digest(t, "foundation-policy-"+string(item.kind)), SemanticDigest: digest(t, "foundation-policy-semantic-"+string(item.kind))}
		requirements = append(requirements, ports.RunSettlementRequirementV2{ID: item.kind, Kind: item.kind, Phase: item.phase, Owner: owner, Schema: schema, SubjectSelector: "runtime/run", SubjectDigest: digest(t, "foundation-subject-"+string(item.kind)), Policy: policy, EvidenceTrust: ports.EvidenceTrustAttestation, EvidenceKind: "runtime/settlement-attestation"})
	}
	execution := ports.RunExecutionSubjectV2{EndpointID: "foundation-endpoint", EndpointDigest: digest(t, "foundation-endpoint"), SessionRef: run.SessionRef, Binding: owner}
	execution.SubjectDigest, _ = execution.DigestV2()
	for index := range requirements {
		if requirements[index].Kind == ports.RunRequirementExecutionTruth {
			requirements[index].SubjectDigest = execution.SubjectDigest
		}
	}
	ports.SortRunSettlementRequirementsV2(requirements)
	claimPolicy := ports.RunSettlementPolicyBindingRefV2{Ref: "foundation-policy-claim", Revision: 1, Digest: digest(t, "foundation-policy-claim"), SemanticDigest: digest(t, "foundation-policy-claim-semantic")}
	return ports.RunSettlementPlanFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: "foundation-settlement-plan", Revision: 1, RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScope: run.Scope, ExecutionScopeDigest: scopeDigest, SessionRef: run.SessionRef, LineagePlanDigest: run.Scope.Lineage.PlanDigest, BindingSet: ports.RunBindingSetRefV2{ID: owner.BindingSetID, Revision: 1, Digest: digest(t, "foundation-binding-set"), SemanticDigest: digest(t, "foundation-binding-set-semantic")}, Execution: execution, Claim: ports.RunClaimRequirementV2{Mode: ports.RunClaimOptionalByPolicyV2, OmissionPolicy: &claimPolicy}, Requirements: requirements, CreatedUnixNano: now.UnixNano()}
}
