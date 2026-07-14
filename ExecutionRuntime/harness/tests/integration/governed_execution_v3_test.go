package integration_test

import (
	"context"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	harnesscontract "github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCertifiedModelTurnRunClosesAcrossRuntimeApplicationHarnessV3(t *testing.T) {
	fixture := newIntegrationFixtureV3(t)
	ctx := context.Background()

	started, err := fixture.coordinator.StartGovernedOperationV3(ctx, application.StartGovernedOperationRequestV3{Plan: fixture.plan, Attempt: fixture.initial})
	if err != nil {
		t.Fatal(err)
	}
	if started.Attempt.State != applicationcontract.OperationProviderObservedV3 || started.Domain == nil || started.Domain.State != applicationports.OperationDomainObservedV3 {
		t.Fatalf("Application/Harness did not converge on the exact provider observation: %#v", started)
	}
	fixture.provider.mu.Lock()
	executeEntries, logicalEffects := fixture.provider.executeEntries, fixture.provider.logicalEffects
	fixture.provider.mu.Unlock()
	if executeEntries != 1 || logicalEffects != 1 {
		t.Fatalf("provider execution was not one logical effect: entries=%d logical=%d", executeEntries, logicalEffects)
	}
	session, err := fixture.harnessStore.InspectSessionV2(ctx, fixture.candidate.Run, fixture.candidate.SessionRef)
	if err != nil || session.Phase != harnesscontract.SessionWaitingSettlementV2 {
		t.Fatalf("Harness did not persist waiting_settlement: %#v err=%v", session, err)
	}

	candidateRef, _ := fixture.candidate.RefV2()
	domainResult, err := harnesscontract.NewSettledTurnDomainResultV2(harnesscontract.SettledTurnResultV2{ContractVersion: harnesscontract.SettledTurnResultContractV2, Candidate: candidateRef, State: harnesscontract.SettledTurnCompletedV2, Output: &fixture.candidate.Input})
	if err != nil {
		t.Fatal(err)
	}
	settlementEvidence := integrationSettlementEvidenceV3(t, fixture, started)
	fixture.evidence.add(settlementEvidence)
	delegation := *started.Attempt.PreparedDelegation
	submission := runtimeports.OperationSettlementSubmissionV3{
		ID: "operation-settlement-integration", Revision: 1,
		Attempt: runtimeports.OperationDispatchAttemptRefV3{OperationDigest: started.Attempt.Intent.OperationDigest, EffectID: started.Attempt.Intent.EffectID, IntentRevision: started.Attempt.Intent.IntentRevision, IntentDigest: started.Attempt.Intent.IntentDigest, PermitID: started.Attempt.BegunAuthorization.Permit.ID, PermitRevision: started.Attempt.BegunAuthorization.Permit.Revision, PermitDigest: started.Attempt.BegunAuthorization.Attempt.PermitDigest, AttemptID: started.Attempt.BegunAuthorization.Attempt.AttemptID, Delegation: &delegation},
		Owner:   started.Attempt.IntentValue.Owners[2], Disposition: runtimeports.OperationSettlementAppliedV3,
		Observation: started.Attempt.Observation, Evidence: []runtimeports.EvidenceRecordRefV2{settlementEvidence.Ref}, DomainResult: &domainResult,
		SettledUnixNano: fixture.operationNow().UnixNano(),
	}
	settled, err := fixture.coordinator.SettleGovernedOperationV3(ctx, application.SettleGovernedOperationRequestV3{Plan: fixture.plan, AttemptID: fixture.initial.ID, Submission: submission})
	if err != nil {
		t.Fatal(err)
	}
	if settled.Attempt.State != applicationcontract.OperationSettledV3 || settled.Domain == nil || settled.Domain.State != applicationports.OperationDomainSettledV3 {
		t.Fatalf("Application/Harness settlement did not converge: %#v", settled)
	}
	session, err = fixture.harnessStore.InspectSessionV2(ctx, fixture.candidate.Run, fixture.candidate.SessionRef)
	if err != nil || session.Phase != harnesscontract.SessionTerminalV2 || session.CompletionClaim != harnesscontract.ClaimCompleted {
		t.Fatalf("Harness terminal claim was not persisted: %#v err=%v", session, err)
	}

	fixture.claimEvidence.loseNext = true
	fixture.claimStore.LoseNextReply()
	claim, err := fixture.claimGateway.IngestRunClaimV3(ctx, runtimeports.RunClaimIngestRequestV2{ExpectedRunRevision: fixture.run.Revision, Candidate: integrationClaimCandidateV3(fixture.run, core.RunClaimCompleted, 1)})
	if err != nil {
		t.Fatal(err)
	}
	certificationRef, _ := fixture.certification.RefV3()
	if claim.Certification.Certification != certificationRef || claim.Plan.ID != fixture.runPlan.ID || claim.Association.RunID != fixture.run.ID {
		t.Fatalf("certified Claim V3 drifted from immutable Run bundle: %#v", claim)
	}
	stillRunning, err := fixture.runFacts.InspectRun(ctx, fixture.run.Scope, fixture.run.ID)
	if err != nil || stillRunning.Status != core.RunRunning || stillRunning.Outcome != "" {
		t.Fatalf("Completion Claim improperly authored Runtime outcome: %#v err=%v", stillRunning, err)
	}

	closed := settleCertifiedRunV3(t, fixture)
	if closed.Phase != runtimeports.RunLifecycleTerminationClosedV3 || closed.Run.Status != core.RunTerminal || closed.Run.Outcome != core.OutcomeCompleted || closed.Report == nil || closed.Progress == nil || closed.Progress.UnresolvedCount != 0 {
		t.Fatalf("independent Runtime settlement did not close terminal cleanup: %#v", closed)
	}
}

func TestCertifiedModelTurnUnknownRecoveryInspectsAndNeverRedispatchesV3(t *testing.T) {
	fixture := newIntegrationFixtureV3(t)
	ctx := context.Background()
	fixture.provider.mu.Lock()
	fixture.provider.loseExecuteReply = true
	fixture.provider.inspectUnavailable = true
	fixture.provider.mu.Unlock()

	unknown, err := fixture.coordinator.StartGovernedOperationV3(ctx, application.StartGovernedOperationRequestV3{Plan: fixture.plan, Attempt: fixture.initial})
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Attempt.State != applicationcontract.OperationDispatchUnknownV3 || unknown.Domain == nil || unknown.Domain.State != applicationports.OperationDomainUnknownV3 || unknown.Attempt.PreparedDelegation == nil || unknown.Attempt.Prepared == nil {
		t.Fatalf("lost Execute reply did not preserve the exact unknown attempt: %#v", unknown)
	}
	fixture.provider.mu.Lock()
	executeEntries, logicalEffects, inspectEntries := fixture.provider.executeEntries, fixture.provider.logicalEffects, fixture.provider.inspectEntries
	fixture.provider.mu.Unlock()
	if executeEntries != 1 || logicalEffects != 1 || inspectEntries < 2 {
		t.Fatalf("unknown boundary was not Execute-once then Inspect-only: execute=%d logical=%d inspect=%d", executeEntries, logicalEffects, inspectEntries)
	}
	session, err := fixture.harnessStore.InspectSessionV2(ctx, fixture.candidate.Run, fixture.candidate.SessionRef)
	if err != nil || session.Phase != harnesscontract.SessionReconcilingV2 {
		t.Fatalf("Harness did not preserve reconciling after unknown: %#v err=%v", session, err)
	}

	for range 20 {
		replayed, resumeErr := fixture.coordinator.ResumeGovernedOperationV3(ctx, application.ResumeGovernedOperationRequestV3{Plan: fixture.plan, AttemptID: fixture.initial.ID})
		if resumeErr != nil {
			t.Fatal(resumeErr)
		}
		if replayed.Attempt.State != applicationcontract.OperationDispatchUnknownV3 || replayed.Domain == nil || replayed.Domain.State != applicationports.OperationDomainUnknownV3 {
			t.Fatalf("unknown replay changed authority state: %#v", replayed)
		}
	}
	fixture.provider.mu.Lock()
	executeEntries, logicalEffects = fixture.provider.executeEntries, fixture.provider.logicalEffects
	fixture.provider.inspectUnavailable = false
	fixture.provider.mu.Unlock()
	if executeEntries != 1 || logicalEffects != 1 {
		t.Fatalf("unknown recovery blindly redispatched Execute: execute=%d logical=%d", executeEntries, logicalEffects)
	}

	inspected, err := fixture.relay.RelayInspectLocalAttempt(ctx, runtimeports.InspectLocalProviderAttemptRequestV2{Delegation: *unknown.Attempt.PreparedDelegation, Prepared: *unknown.Attempt.Prepared})
	if err != nil {
		t.Fatal(err)
	}
	if inspected.State != runtimeports.ProviderAttemptObservedV2 || inspected.Prepared != *unknown.Attempt.Prepared {
		t.Fatalf("independent Inspect did not recover the exact provider observation: %#v", inspected)
	}
	fixture.provider.mu.Lock()
	executeEntries, logicalEffects = fixture.provider.executeEntries, fixture.provider.logicalEffects
	fixture.provider.mu.Unlock()
	if executeEntries != 1 || logicalEffects != 1 {
		t.Fatalf("independent Inspect redispatched provider work: execute=%d logical=%d", executeEntries, logicalEffects)
	}
}

func integrationSettlementEvidenceV3(t *testing.T, fixture *integrationFixtureV3, started application.GovernedOperationResultV3) runtimeports.EvidenceLedgerRecordV2 {
	t.Helper()
	observation := started.Attempt.Observation
	ledger := runtimeports.EvidenceLedgerScopeV2{Partition: runtimeports.EvidencePartitionRun, TenantID: fixture.run.Scope.Identity.TenantID, IdentityID: fixture.run.Scope.Identity.ID, LineageID: fixture.run.Scope.Lineage.ID, InstanceID: fixture.run.Scope.Instance.ID, RunID: fixture.run.ID}
	ledgerDigest, _ := ledger.DigestV2()
	owner := runtimeports.EvidenceProducerBindingRefV2{BindingSetID: fixture.intent.Provider.BindingSetID, BindingSetRevision: fixture.intent.Provider.BindingSetRevision, ComponentID: fixture.intent.Provider.ComponentID, ManifestDigest: fixture.intent.Provider.ManifestDigest, ArtifactDigest: fixture.intent.Provider.ArtifactDigest, Capability: fixture.intent.Provider.Capability}
	resultPayload := integrationPayloadV3("model-result")
	candidate := runtimeports.EvidenceEventCandidateV2{ContractVersion: runtimeports.EvidenceContractVersionV2, LedgerScope: ledger, EventID: "operation-settlement-evidence", RegistrationID: "operation-settlement-source", RegistrationRevision: 1, SourceConfigurationDigest: integrationDigestV3("settlement-source-config"), SourcePolicy: runtimeports.EvidenceSourcePolicyBindingRefV2{Ref: "settlement-source-policy", Revision: 1, Digest: integrationDigestV3("settlement-source-policy")}, SourceID: "praxis.model/settlement-owner", SourceEpoch: 1, SourceSequence: 2, TrustClass: runtimeports.EvidenceTrustAttestation, EventKind: "praxis.model/settlement", CustomClass: "praxis.model/settlement", ExecutionScope: fixture.run.Scope, Payload: runtimeports.EvidencePayloadRefV2{Schema: resultPayload.Schema, ContentDigest: observation.PayloadDigest, Revision: observation.PayloadRevision, Length: resultPayload.Length, Ref: "memory://operation-settlement"}, Causation: []runtimeports.EvidenceCausationRefV2{{LedgerScopeDigest: ledgerDigest, EventID: observation.ProviderOperationRef}}, CorrelationID: fixture.initial.ID, Producer: owner, Authority: runtimeports.AuthorityBindingRefV2{Ref: "settlement-evidence-authority", Revision: 1, Digest: integrationDigestV3("settlement-evidence-authority"), Epoch: fixture.run.Scope.AuthorityEpoch}, ObservedUnixNano: fixture.operationNow().UnixNano()}
	record, err := controlNewEvidenceRecordV3(candidate, 2, fixture.operationNow())
	if err != nil {
		t.Fatal(err)
	}
	return record
}

// Kept behind a local helper so the test reads as a public Evidence-owner
// interaction rather than a direct construction of record internals.
func controlNewEvidenceRecordV3(candidate runtimeports.EvidenceEventCandidateV2, sequence uint64, now time.Time) (runtimeports.EvidenceLedgerRecordV2, error) {
	return control.NewEvidenceLedgerRecordV2(candidate, sequence, integrationDigestV3("previous-operation-evidence"), now)
}

type integrationAuthorityReaderV3 map[string]runtimeports.DispatchAuthorityFactV2

func (r integrationAuthorityReaderV3) InspectDispatchAuthority(_ context.Context, ref string) (runtimeports.DispatchAuthorityFactV2, error) {
	fact, ok := r[ref]
	if !ok {
		return runtimeports.DispatchAuthorityFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonEffectAuthorizationMissing, "settlement authority missing")
	}
	return fact, nil
}

type integrationExecutionInspectorV3 struct {
	fixture *integrationFixtureV3
	inputs  *runtimefakes.RunSettlementInputStoreV2
}

func (i integrationExecutionInspectorV3) InspectRunExecutionV2(_ context.Context, request runtimeports.RunExecutionInspectionRequestV2) (runtimeports.ExecutionSettlementInspectionV2, error) {
	requirement, ok := integrationRequirementV3(i.fixture.runPlan, runtimeports.RunRequirementExecutionTruth)
	if !ok {
		return runtimeports.ExecutionSettlementInspectionV2{}, core.NewError(core.ErrorNotFound, core.ReasonRunSettlementRequirementInvalid, "execution requirement missing")
	}
	inspection := runtimeports.ExecutionSettlementInspectionV2{ContractVersion: runtimeports.RunSettlementContractVersionV2, ID: "execution-inspection-integration", Revision: 1, RunID: request.RunID, RunIdentityDigest: request.RunIdentityDigest, RunRevision: request.ExpectedRunRevision, ExecutionScope: request.ExecutionScope, ExecutionScopeDigest: i.fixture.runPlan.ExecutionScopeDigest, Subject: request.Subject, Truth: runtimeports.RunExecutionTerminalCompleted, SourceEpoch: request.ExecutionScope.Instance.Epoch, SourceSequence: 10, InspectedUnixNano: i.fixture.now.Add(5 * time.Second).UnixNano(), ExpiresUnixNano: i.fixture.now.Add(time.Hour).UnixNano()}
	inspection.PayloadDigest, _ = inspection.EvidenceSubjectDigestV2()
	correlation, _ := runtimeports.RunSettlementEvidenceCorrelationIDV2(i.fixture.runPlan.ID, i.fixture.runPlan.RunID, requirement.ID, requirement.SubjectDigest)
	causation, _ := runtimeports.RunExecutionInspectionEvidenceCausationIDV2(inspection)
	ledger := runtimeports.EvidenceLedgerScopeV2{Partition: runtimeports.EvidencePartitionRun, TenantID: request.ExecutionScope.Identity.TenantID, IdentityID: request.ExecutionScope.Identity.ID, LineageID: request.ExecutionScope.Lineage.ID, InstanceID: request.ExecutionScope.Instance.ID, RunID: request.RunID}
	ledgerDigest, _ := ledger.DigestV2()
	candidate := runtimeports.EvidenceEventCandidateV2{ContractVersion: runtimeports.EvidenceContractVersionV2, LedgerScope: ledger, EventID: "execution-inspection-event", RegistrationID: "execution-inspection-source", RegistrationRevision: 1, SourceConfigurationDigest: integrationDigestV3("execution-source-config"), SourcePolicy: runtimeports.EvidenceSourcePolicyBindingRefV2{Ref: "execution-source-policy", Revision: 1, Digest: integrationDigestV3("execution-source-policy")}, SourceID: "praxis.runtime/execution-inspector", SourceEpoch: inspection.SourceEpoch, SourceSequence: inspection.SourceSequence, TrustClass: requirement.EvidenceTrust, EventKind: requirement.EvidenceKind, CustomClass: "runtime/settlement", ExecutionScope: request.ExecutionScope, Payload: runtimeports.EvidencePayloadRefV2{Schema: requirement.Schema, ContentDigest: inspection.PayloadDigest, Revision: inspection.Revision, Length: 32, Ref: "memory://execution-inspection"}, Causation: []runtimeports.EvidenceCausationRefV2{{LedgerScopeDigest: ledgerDigest, EventID: causation}}, CorrelationID: correlation, Producer: request.Subject.Binding, Authority: runtimeports.AuthorityBindingRefV2{Ref: "execution-inspection-authority", Revision: 1, Digest: integrationDigestV3("execution-inspection-authority"), Epoch: request.ExecutionScope.AuthorityEpoch}, ObservedUnixNano: inspection.InspectedUnixNano}
	record, err := control.NewEvidenceLedgerRecordV2(candidate, inspection.SourceSequence, integrationDigestV3("execution-previous"), i.fixture.now.Add(5*time.Second))
	if err != nil {
		return runtimeports.ExecutionSettlementInspectionV2{}, err
	}
	i.fixture.evidence.add(record)
	inspection.Evidence = record.Ref
	return inspection, inspection.Validate()
}

func settleCertifiedRunV3(t *testing.T, fixture *integrationFixtureV3) runtimeports.RunLifecycleEnvelopeV3 {
	t.Helper()
	ctx := context.Background()
	inputs := runtimefakes.NewRunSettlementInputStoreV2()
	authorities := integrationAuthorityReaderV3{}
	planRef, _ := fixture.runPlan.RefV2()
	for _, requirement := range fixture.runPlan.Requirements {
		policy, ok := fixture.runPolicies[requirement.ID]
		if !ok || policy.Digest != requirement.Policy.Digest || policy.SemanticDigest != requirement.Policy.SemanticDigest {
			t.Fatalf("certified policy ref does not match its fact for %s", requirement.ID)
		}
		if err := inputs.PutPolicy(policy); err != nil {
			t.Fatal(err)
		}
		authorities[policy.PolicyAuthority.Ref] = runtimeports.DispatchAuthorityFactV2{Ref: policy.PolicyAuthority.Ref, Digest: policy.PolicyAuthority.Digest, Revision: policy.PolicyAuthority.Revision, Scope: policy.ExecutionScope, ActionScopeDigest: policy.ActionScopeDigest, State: runtimeports.AuthorityFactActive, ExpiresUnixNano: fixture.now.Add(time.Hour).UnixNano()}
		if requirement.Kind == runtimeports.RunRequirementExecutionTruth || requirement.Kind == runtimeports.RunRequirementEffects {
			continue
		}
		requirementDigest, _ := requirement.DigestV2()
		participant := runtimeports.RunSettlementParticipantFactV2{ContractVersion: runtimeports.RunSettlementContractVersionV2, ID: "participant-" + string(requirement.ID[8:]), Revision: 1, RunID: fixture.run.ID, RunIdentityDigest: fixture.runPlan.RunIdentityDigest, ExecutionScope: fixture.run.Scope, ExecutionScopeDigest: fixture.runPlan.ExecutionScopeDigest, Plan: planRef, RequirementID: requirement.ID, RequirementDigest: requirementDigest, SubjectDigest: requirement.SubjectDigest, Owner: requirement.Owner, Disposition: runtimeports.RunSettlementOperationNotRequired, Policy: &requirement.Policy, Evidence: []runtimeports.EvidenceRecordRefV2{}, CreatedUnixNano: fixture.now.Add(5 * time.Second).UnixNano(), ExpiresUnixNano: fixture.now.Add(time.Hour).UnixNano()}
		if err := inputs.PutParticipant(participant); err != nil {
			t.Fatal(err)
		}
	}
	gateway := fixture.runGateway
	gateway.Claims = fixture.claimStore
	gateway.Evidence = fixture.evidence
	gateway.Execution = integrationExecutionInspectorV3{fixture: fixture, inputs: inputs}
	gateway.Participants = inputs
	gateway.Policies = inputs
	gateway.Bindings = fixture.bindings.store
	gateway.Authority = authorities
	gateway.PlanAdmissions = fixture.planGateway
	fixture.runFacts.LoseNextCommitReply()
	closed, err := gateway.StopAndSettleRunV3(ctx, runtimeports.BeginStopRunRequestV3{ExecutionScope: fixture.run.Scope, RunID: fixture.run.ID, ExpectedRunRevision: fixture.run.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if closed.Phase == runtimeports.RunLifecycleTerminalCleanupV3 {
		closed, err = gateway.ReconcileRunTerminationV3(ctx, runtimeports.RunTerminationRequestV3{ExecutionScope: fixture.run.Scope, RunID: fixture.run.ID})
		if err != nil {
			t.Fatal(err)
		}
	}
	return closed
}

func integrationRequirementV3(plan runtimeports.RunSettlementPlanFactV2, kind runtimeports.NamespacedNameV2) (runtimeports.RunSettlementRequirementV2, bool) {
	for _, requirement := range plan.Requirements {
		if requirement.Kind == kind {
			return requirement, true
		}
	}
	return runtimeports.RunSettlementRequirementV2{}, false
}
