package fakes_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type staticPlanAdmissionV3 struct {
	fact ports.RunSettlementPlanCertificationFactV3
}

func (s staticPlanAdmissionV3) CertifyRunSettlementPlanV3(context.Context, ports.CertifyRunSettlementPlanRequestV3) (ports.RunSettlementPlanCertificationFactV3, error) {
	return s.fact, nil
}

func (s staticPlanAdmissionV3) InspectCertifiedRunSettlementPlanV3(context.Context, core.ExecutionScope, core.AgentRunID) (ports.RunSettlementPlanCertificationFactV3, error) {
	return s.fact, nil
}

func (s staticPlanAdmissionV3) ValidateRunSettlementPlanCertificationV3(_ context.Context, expected ports.RunSettlementPlanCertificationRefV3, run core.AgentRunRecord, plan ports.RunSettlementPlanFactV2) error {
	ref, err := s.fact.RefV3()
	planRef, planErr := plan.RefV2()
	identity, identityErr := ports.RunIdentityDigestV2(run)
	if err != nil || planErr != nil || identityErr != nil || ref != expected || s.fact.RunID != run.ID || s.fact.RunIdentityDigest != identity || s.fact.Plan != planRef {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "static Plan certification drifted")
	}
	return nil
}

func staticPlanCertificationV3(t *testing.T, run core.AgentRunRecord, plan ports.RunSettlementPlanFactV2, now time.Time) staticPlanAdmissionV3 {
	t.Helper()
	planRef, _ := plan.RefV2()
	declaration := ports.RunSettlementDeclarationRefV3{ID: "declaration-" + string(run.ID), Revision: 1, Digest: runSettlementDigestV2(t, "declaration-"+string(run.ID)), BindingSetID: plan.BindingSet.ID, BindingSetRevision: plan.BindingSet.Revision, BindingRevision: 1, ComponentID: plan.Execution.Binding.ComponentID}
	fact, err := ports.SealRunSettlementPlanCertificationFactV3(ports.RunSettlementPlanCertificationFactV3{ContractVersion: ports.RunSettlementPlanAdmissionContractVersionV3, ID: "certification-" + string(run.ID), Revision: 1, RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, Plan: planRef, BindingSet: plan.BindingSet, BaselinePolicy: ports.RunSettlementBaselinePolicyRefV3{ID: "baseline-" + string(run.ID), Revision: 1, Digest: runSettlementDigestV2(t, "baseline-"+string(run.ID))}, Declarations: []ports.RunSettlementDeclarationRefV3{declaration}, CertificationOwner: plan.Execution.Binding, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return staticPlanAdmissionV3{fact: fact}
}

func persistRunningRunBundleV3(t *testing.T, store *fakes.RunSettlementStoreV2, desired core.AgentRunRecord, plan ports.RunSettlementPlanFactV2, admission staticPlanAdmissionV3) core.AgentRunRecord {
	t.Helper()
	pending := desired
	pending.Status, pending.Revision, pending.StartedAt, pending.EndedAt, pending.Outcome, pending.CompletionClaim = core.RunPending, 1, time.Time{}, time.Time{}, "", nil
	ref, _ := admission.fact.RefV3()
	association, _ := ports.NewRunSettlementPlanCertificationAssociationV3(pending, plan, ref)
	if _, err := store.CreateRunBundleV3(context.Background(), control.RunBundleCreateRequestV3{Run: pending, Plan: plan, Certification: association}); err != nil {
		t.Fatal(err)
	}
	running := desired
	running.Status, running.Revision = core.RunRunning, 2
	stored, err := store.CompareAndSwapRun(context.Background(), control.RunFactCASRequest{ExpectedRevision: 1, Next: running})
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func TestRunSettlementGatewayV2DerivesOutcomeAndRecoversLostCommit(t *testing.T) {
	t.Parallel()
	fixture := newRunSettlementGatewayFixtureV2(t, ports.RunExecutionTerminalCompleted)
	fixture.facts.LoseNextCommitReply()
	result, err := fixture.gateway.StopAndSettleRunV2(context.Background(), kernel.StopAndSettleRunRequestV2{Scope: fixture.run.Scope, RunID: fixture.run.ID, ExpectedRunRevision: fixture.run.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.Status != core.RunTerminal || result.Run.Outcome != core.OutcomeCompleted || result.Decision.Outcome != core.OutcomeCompleted {
		t.Fatalf("Gateway did not derive and atomically recover Completed outcome: %+v", result)
	}
	if result.Run.CompletionClaim != nil {
		t.Fatal("V2 settlement must not write the legacy CompletionClaim into the authoritative Run")
	}
	if len(result.Progress.Items) != 3 {
		t.Fatalf("termination progress must remain a separate three-dimensional fact: %+v", result.Progress)
	}
	restarted := fixture.gateway
	replayed, err := restarted.StopAndSettleRunV2(context.Background(), kernel.StopAndSettleRunRequestV2{Scope: fixture.run.Scope, RunID: fixture.run.ID, ExpectedRunRevision: fixture.run.Revision})
	if err != nil || replayed.Decision.ID != result.Decision.ID || replayed.Closure.Attempt != result.Closure.Attempt {
		t.Fatalf("whole settlement operation did not recover after terminal commit: %+v %v", replayed, err)
	}
}

func TestRunSettlementGatewayV2MissingClaimNeverSelectsOutcome(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		truth   ports.RunExecutionTruthV2
		outcome core.ExecutionOutcome
	}{{ports.RunExecutionTerminalCancelled, core.OutcomeCancelled}, {ports.RunExecutionTerminalFailed, core.OutcomeFailed}} {
		test := test
		t.Run(string(test.truth), func(t *testing.T) {
			fixture := newRunSettlementGatewayFixtureV2(t, test.truth)
			result, err := fixture.gateway.StopAndSettleRunV2(context.Background(), kernel.StopAndSettleRunRequestV2{Scope: fixture.run.Scope, RunID: fixture.run.ID, ExpectedRunRevision: fixture.run.Revision})
			if err != nil {
				t.Fatal(err)
			}
			if result.Run.Outcome != test.outcome {
				t.Fatalf("outcome must come from independent Execution inspect, got %s want %s", result.Run.Outcome, test.outcome)
			}
		})
	}
}

func TestRunSettlementGatewayV2ClaimOmissionRevalidatesPolicyGovernance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*runSettlementGatewayFixtureV2)
	}{
		{
			name: "authority_revoked",
			mutate: func(fixture *runSettlementGatewayFixtureV2) {
				reader := fixture.gateway.Authority.(settlementAuthorityReaderV2)
				policy := fixture.plan.Claim.OmissionPolicy
				for _, fact := range fixture.policies {
					if policy != nil && fact.Ref == policy.Ref {
						authority := reader[fact.PolicyAuthority.Ref]
						authority.State = ports.AuthorityFactRevoked
						reader[fact.PolicyAuthority.Ref] = authority
					}
				}
			},
		},
		{
			name: "authority_exact_expiry",
			mutate: func(fixture *runSettlementGatewayFixtureV2) {
				reader := fixture.gateway.Authority.(settlementAuthorityReaderV2)
				policy := fixture.plan.Claim.OmissionPolicy
				for _, fact := range fixture.policies {
					if policy != nil && fact.Ref == policy.Ref {
						authority := reader[fact.PolicyAuthority.Ref]
						authority.ExpiresUnixNano = fixture.now.UnixNano()
						reader[fact.PolicyAuthority.Ref] = authority
					}
				}
			},
		},
		{
			name: "binding_revoked",
			mutate: func(fixture *runSettlementGatewayFixtureV2) {
				reader := fixture.gateway.Bindings.(*settlementBindingReaderV2)
				reader.set.State = control.BindingSetRevoked
			},
		},
		{
			name: "binding_member_revision_drift",
			mutate: func(fixture *runSettlementGatewayFixtureV2) {
				reader := fixture.gateway.Bindings.(*settlementBindingReaderV2)
				reader.set.Members[0].BindingRevision++
			},
		},
		{
			name: "binding_manifest_drift",
			mutate: func(fixture *runSettlementGatewayFixtureV2) {
				reader := fixture.gateway.Bindings.(*settlementBindingReaderV2)
				reader.set.Members[0].ManifestDigest = runSettlementDigestV2(t, "forged-claim-policy-manifest")
			},
		},
		{
			name: "binding_capability_drift",
			mutate: func(fixture *runSettlementGatewayFixtureV2) {
				reader := fixture.gateway.Bindings.(*settlementBindingReaderV2)
				reader.set.Members[0].Grants[0].Capability = "runtime/forged-claim-policy"
			},
		},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newRunSettlementGatewayFixtureV2(t, ports.RunExecutionTerminalCompleted)
			testCase.mutate(fixture)
			_, err := fixture.gateway.StopAndSettleRunV2(context.Background(), kernel.StopAndSettleRunRequestV2{Scope: fixture.run.Scope, RunID: fixture.run.ID, ExpectedRunRevision: fixture.run.Revision})
			if err == nil || !(core.HasReason(err, core.ReasonRunClaimUnverified) || core.HasReason(err, core.ReasonBindingDrift) || core.HasReason(err, core.ReasonUnknownCapability)) {
				t.Fatalf("stale Claim omission governance did not fail closed: %v", err)
			}
		})
	}

	self := newRunSettlementGatewayFixtureWithPoliciesV2(t, ports.RunExecutionTerminalCompleted, nil, false)
	if _, err := self.gateway.StopAndSettleRunV2(context.Background(), kernel.StopAndSettleRunRequestV2{Scope: self.run.Scope, RunID: self.run.ID, ExpectedRunRevision: self.run.Revision}); !core.HasReason(err, core.ReasonRunClaimUnverified) {
		t.Fatalf("Execution owner self-authorized Claim omission without explicit policy: %v", err)
	}
}

func TestRunSettlementGatewayV2ExecutionInspectionRejectsEveryGovernanceDrift(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*runSettlementGatewayFixtureV2, *ports.ExecutionSettlementInspectionV2)
	}{
		{
			name: "old_instance_epoch",
			mutate: func(_ *runSettlementGatewayFixtureV2, inspection *ports.ExecutionSettlementInspectionV2) {
				inspection.ExecutionScope.Instance.Epoch--
				inspection.ExecutionScopeDigest, _ = ports.ExecutionScopeDigestV2(inspection.ExecutionScope)
				inspection.SourceEpoch = inspection.ExecutionScope.Instance.Epoch
			},
		},
		{
			name: "wrong_endpoint",
			mutate: func(_ *runSettlementGatewayFixtureV2, inspection *ports.ExecutionSettlementInspectionV2) {
				inspection.Subject.EndpointID = "endpoint-forged"
				inspection.Subject.SubjectDigest, _ = inspection.Subject.DigestV2()
			},
		},
		{
			name: "wrong_runtime_session",
			mutate: func(fixture *runSettlementGatewayFixtureV2, inspection *ports.ExecutionSettlementInspectionV2) {
				inspection.Subject.SessionRef, _ = ports.DeriveRuntimeExecutionSessionRefV2(inspection.Subject.EndpointID, fixture.run.ID+"-forged")
				inspection.Subject.SubjectDigest, _ = inspection.Subject.DigestV2()
			},
		},
		{
			name: "wrong_run",
			mutate: func(_ *runSettlementGatewayFixtureV2, inspection *ports.ExecutionSettlementInspectionV2) {
				inspection.RunID = "run-forged"
			},
		},
		{
			name: "wrong_binding",
			mutate: func(_ *runSettlementGatewayFixtureV2, inspection *ports.ExecutionSettlementInspectionV2) {
				inspection.Subject.Binding.ComponentID = "runtime/forged-executor"
				inspection.Subject.SubjectDigest, _ = inspection.Subject.DigestV2()
			},
		},
		{
			name: "wrong_source_sequence",
			mutate: func(_ *runSettlementGatewayFixtureV2, inspection *ports.ExecutionSettlementInspectionV2) {
				inspection.SourceSequence++
			},
		},
		{
			name: "wrong_payload",
			mutate: func(_ *runSettlementGatewayFixtureV2, inspection *ports.ExecutionSettlementInspectionV2) {
				inspection.PayloadDigest = runSettlementDigestV2(t, "forged-execution-payload")
			},
		},
		{
			name: "truth_not_bound_by_evidence",
			mutate: func(_ *runSettlementGatewayFixtureV2, inspection *ports.ExecutionSettlementInspectionV2) {
				inspection.Truth = ports.RunExecutionTerminalFailed
				inspection.PayloadDigest, _ = inspection.EvidenceSubjectDigestV2()
			},
		},
		{
			name: "wrong_evidence_ref",
			mutate: func(_ *runSettlementGatewayFixtureV2, inspection *ports.ExecutionSettlementInspectionV2) {
				inspection.Evidence.RecordDigest = runSettlementDigestV2(t, "forged-execution-record")
			},
		},
		{
			name: "exact_expiry",
			mutate: func(fixture *runSettlementGatewayFixtureV2, inspection *ports.ExecutionSettlementInspectionV2) {
				inspection.ExpiresUnixNano = fixture.now.UnixNano()
			},
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newRunSettlementGatewayFixtureV2(t, ports.RunExecutionTerminalCompleted)
			inspection := fixture.execution
			testCase.mutate(fixture, &inspection)
			fixture.gateway.Execution = staticExecutionSettlementInspectorV2{fact: inspection}
			_, err := fixture.gateway.StopAndSettleRunV2(
				context.Background(),
				kernel.StopAndSettleRunRequestV2{
					Scope:               fixture.run.Scope,
					RunID:               fixture.run.ID,
					ExpectedRunRevision: fixture.run.Revision,
				},
			)
			if !core.HasReason(err, core.ReasonExecutionInspectionInvalid) {
				t.Fatalf("execution inspection drift did not fail closed: %v", err)
			}
		})
	}
}

func TestRunSettlementGatewayV2RejectsInspectionRefDriftAfterClosure(t *testing.T) {
	t.Parallel()
	fixture := newRunSettlementGatewayFixtureV2(t, ports.RunExecutionTerminalCompleted)
	drifted := fixture.execution
	drifted.ID = "inspection-replaced"
	drifted.Revision++
	fixture.gateway.Execution = &changingExecutionSettlementInspectorV2{
		first: fixture.execution,
		next:  drifted,
	}
	_, err := fixture.gateway.StopAndSettleRunV2(
		context.Background(),
		kernel.StopAndSettleRunRequestV2{
			Scope:               fixture.run.Scope,
			RunID:               fixture.run.ID,
			ExpectedRunRevision: fixture.run.Revision,
		},
	)
	if !core.HasReason(err, core.ReasonExecutionInspectionInvalid) {
		t.Fatalf("replacement inspection ref after Closure did not fail closed: %v", err)
	}
}

func TestRunSettlementGatewayV2RejectsExecutionEvidenceFromAnotherInspection(t *testing.T) {
	t.Parallel()
	fixture := newRunSettlementGatewayFixtureV2(t, ports.RunExecutionTerminalCompleted)
	original := fixture.evidence.records[fixture.execution.Evidence]
	other := fixture.execution
	other.ID = "inspection-other-same-run"
	other.Revision++
	other.PayloadDigest, _ = other.EvidenceSubjectDigestV2()
	causation, err := ports.RunExecutionInspectionEvidenceCausationIDV2(other)
	if err != nil {
		t.Fatal(err)
	}
	forgedCandidate := original.Candidate
	ledgerScopeDigest, _ := forgedCandidate.LedgerScope.DigestV2()
	forgedCandidate.Causation = []ports.EvidenceCausationRefV2{{LedgerScopeDigest: ledgerScopeDigest, EventID: causation}}
	forged, err := control.NewEvidenceLedgerRecordV2(forgedCandidate, original.Ref.Sequence, original.PreviousRecordDigest, time.Unix(0, original.IngestedUnixNano))
	if err != nil {
		t.Fatal(err)
	}
	fixture.evidence.records[forged.Ref] = forged
	inspection := fixture.execution
	inspection.Evidence = forged.Ref
	fixture.gateway.Execution = staticExecutionSettlementInspectorV2{fact: inspection}
	_, err = fixture.gateway.StopAndSettleRunV2(context.Background(), kernel.StopAndSettleRunRequestV2{Scope: fixture.run.Scope, RunID: fixture.run.ID, ExpectedRunRevision: fixture.run.Revision})
	if !core.HasReason(err, core.ReasonExecutionInspectionInvalid) {
		t.Fatalf("same payload/correlation Evidence from another inspection identity was reused: %v", err)
	}
}

func TestRunSettlementGatewayV2RevalidatesPresentClaimWithoutUsingItAsOutcome(t *testing.T) {
	t.Parallel()
	fixture := newRunSettlementGatewayFixtureV2(t, ports.RunExecutionTerminalCancelled)
	association, record := attachRunSettlementClaimV2(t, fixture, core.RunClaimCompleted)

	result, err := fixture.gateway.StopAndSettleRunV2(
		context.Background(),
		kernel.StopAndSettleRunRequestV2{
			Scope:               fixture.run.Scope,
			RunID:               fixture.run.ID,
			ExpectedRunRevision: fixture.run.Revision,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.Outcome != core.OutcomeCancelled {
		t.Fatalf("completed Claim selected Outcome over cancelled Execution truth: %+v", result.Run)
	}
	if result.Decision.Claim == nil || result.Decision.Claim.ID != association.ID {
		t.Fatalf("Decision did not retain the exact verified Claim provenance: %+v", result.Decision.Claim)
	}

	conflict := association
	conflict.Evidence.RecordDigest = runSettlementDigestV2(t, "late-conflicting-claim")
	conflict.ID, _ = ports.RunClaimAssociationIDV2(conflict.RunID, conflict.Evidence)
	conflict.CandidateDigest = record.CandidateDigest
	if _, err := fixture.gateway.Claims.CreateRunClaimAssociation(context.Background(), conflict); !core.HasReason(err, core.ReasonRunClaimConflict) {
		t.Fatalf("late conflicting Claim replaced terminal provenance: %v", err)
	}
	replayed, err := fixture.gateway.StopAndSettleRunV2(
		context.Background(),
		kernel.StopAndSettleRunRequestV2{
			Scope:               fixture.run.Scope,
			RunID:               fixture.run.ID,
			ExpectedRunRevision: fixture.run.Revision,
		},
	)
	if err != nil || replayed.Decision.Claim == nil || replayed.Decision.Claim.ID != association.ID {
		t.Fatalf("late Claim changed immutable terminal Decision: %+v %v", replayed, err)
	}
}

func TestRunSettlementGatewayV2RejectsForgedClaimAssociationCoordinates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*ports.RunClaimAssociationFactV2)
	}{
		{"run", func(f *ports.RunClaimAssociationFactV2) { f.RunID = "run-forged" }},
		{"scope", func(f *ports.RunClaimAssociationFactV2) { f.ExecutionScope.Identity.TenantID = "tenant-forged" }},
		{"claim_kind", func(f *ports.RunClaimAssociationFactV2) { f.ClaimKind = core.RunClaimFailed }},
		{"evidence_ref", func(f *ports.RunClaimAssociationFactV2) {
			f.Evidence.RecordDigest = runSettlementDigestV2(t, "claim-ref-forged")
		}},
		{"candidate_digest", func(f *ports.RunClaimAssociationFactV2) {
			f.CandidateDigest = runSettlementDigestV2(t, "claim-candidate-forged")
		}},
		{"association_id", func(f *ports.RunClaimAssociationFactV2) {
			f.ID = "association:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newRunSettlementGatewayFixtureV2(t, ports.RunExecutionTerminalCompleted)
			association, _ := runSettlementClaimAssociationV2(t, fixture, core.RunClaimCompleted)
			testCase.mutate(&association)
			fixture.gateway.Claims = staticRunClaimAssociationPortV2{fact: association}
			_, err := fixture.gateway.StopAndSettleRunV2(
				context.Background(),
				kernel.StopAndSettleRunRequestV2{
					Scope:               fixture.run.Scope,
					RunID:               fixture.run.ID,
					ExpectedRunRevision: fixture.run.Revision,
				},
			)
			if !core.HasReason(err, core.ReasonRunClaimUnverified) {
				t.Fatalf("forged Claim association did not fail closed: %v", err)
			}
		})
	}
}

func TestRunSettlementGatewayV2RejectsParticipantEvidenceWithWrongCausation(t *testing.T) {
	t.Parallel()
	fixture := newRunSettlementGatewayFixtureV2(t, ports.RunExecutionTerminalCompleted)
	var requirement ports.RunSettlementRequirementV2
	for _, candidate := range fixture.plan.Requirements {
		if candidate.Kind == ports.RunRequirementDomainCommits {
			requirement = candidate
			break
		}
	}
	fact := fixture.participantsFor[requirement.ID]
	original := fixture.evidence.records[fact.Evidence[0]]
	forgedCandidate := original.Candidate
	forgedCandidate.Causation = append([]ports.EvidenceCausationRefV2{}, forgedCandidate.Causation...)
	forgedCandidate.Causation[0].EventID = "settlement-cause:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	forged, err := control.NewEvidenceLedgerRecordV2(forgedCandidate, original.Ref.Sequence, original.PreviousRecordDigest, time.Unix(0, original.IngestedUnixNano))
	if err != nil {
		t.Fatal(err)
	}
	fixture.evidence.records[forged.Ref] = forged
	fact.Evidence = []ports.EvidenceRecordRefV2{forged.Ref}
	inputs := fakes.NewRunSettlementInputStoreV2()
	for _, policy := range fixture.policies {
		if err := inputs.PutPolicy(policy); err != nil {
			t.Fatal(err)
		}
	}
	for id, current := range fixture.participantsFor {
		if id == requirement.ID {
			current = fact
		}
		if err := inputs.PutParticipant(current); err != nil {
			t.Fatal(err)
		}
	}
	if err := inputs.PutExecution(fixture.execution); err != nil {
		t.Fatal(err)
	}
	fixture.gateway.Participants, fixture.gateway.Policies, fixture.gateway.Execution = inputs, inputs, inputs
	if _, err := fixture.gateway.StopAndSettleRunV2(context.Background(), kernel.StopAndSettleRunRequestV2{Scope: fixture.run.Scope, RunID: fixture.run.ID, ExpectedRunRevision: fixture.run.Revision}); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("same-correlation Evidence with another causation must fail closed: %v", err)
	}
}

func TestRunSettlementGatewayV2UnknownRequiredDimensionBlocksWithoutPolicy(t *testing.T) {
	t.Parallel()
	fixture := newRunSettlementGatewayFixtureV2(t, ports.RunExecutionTerminalCompleted)
	requirement := fixture.plan.Requirements[0]
	for _, candidate := range fixture.plan.Requirements {
		if candidate.Kind == ports.RunRequirementDomainCommits {
			requirement = candidate
			break
		}
	}
	participant := fixture.participantsFor[requirement.ID]
	participant.Disposition = ports.RunSettlementUnknown
	participant.Revision++
	participant.ID += "-unknown"
	if err := fixture.inputs.PutParticipant(participant); err == nil {
		t.Fatal("test fixture correctly prevents replacing an authoritative participant identity; expected conflict")
	}
	// A fresh authoritative input store can expose the unknown current fact.
	inputs := fakes.NewRunSettlementInputStoreV2()
	for _, policy := range fixture.policies {
		if err := inputs.PutPolicy(policy); err != nil {
			t.Fatal(err)
		}
	}
	for id, fact := range fixture.participantsFor {
		if id == requirement.ID {
			fact.Disposition = ports.RunSettlementUnknown
		}
		if err := inputs.PutParticipant(fact); err != nil {
			t.Fatal(err)
		}
	}
	if err := inputs.PutExecution(fixture.execution); err != nil {
		t.Fatal(err)
	}
	fixture.gateway.Participants, fixture.gateway.Policies, fixture.gateway.Execution = inputs, inputs, inputs
	if _, err := fixture.gateway.StopAndSettleRunV2(context.Background(), kernel.StopAndSettleRunRequestV2{Scope: fixture.run.Scope, RunID: fixture.run.ID, ExpectedRunRevision: fixture.run.Revision}); !core.HasReason(err, core.ReasonRunSettlementBlocked) {
		t.Fatalf("unknown required domain fact without explicit terminalize policy must block: %v", err)
	}
	run, err := fixture.facts.InspectRun(context.Background(), fixture.run.Scope, fixture.run.ID)
	if err != nil || run.Status == core.RunTerminal {
		t.Fatalf("blocked settlement terminalized the Run: %+v %v", run, err)
	}
}

func TestRunSettlementGatewayV2DispositionPolicyMatrix(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name        string
		disposition ports.RunSettlementDispositionV2
		allow       bool
	}{
		{"unknown_block", ports.RunSettlementUnknown, false}, {"unknown_reconcile", ports.RunSettlementUnknown, true},
		{"failed_block", ports.RunSettlementConfirmedFailed, false}, {"failed_reconcile", ports.RunSettlementConfirmedFailed, true},
		{"not_applied_block", ports.RunSettlementConfirmedNotApplied, false}, {"not_applied_reconcile", ports.RunSettlementConfirmedNotApplied, true},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newRunSettlementGatewayFixtureWithPolicyV2(t, ports.RunExecutionTerminalCompleted, func(requirement ports.RunSettlementRequirementV2, policy *ports.RunSettlementPolicyFactV2) {
				if requirement.Kind != ports.RunRequirementDomainCommits || !testCase.allow {
					return
				}
				switch testCase.disposition {
				case ports.RunSettlementUnknown:
					policy.UnknownMode = ports.RunUnknownTerminalizeReconciliation
				case ports.RunSettlementConfirmedFailed:
					policy.FailureMode = ports.RunClosedFailureReconcile
				case ports.RunSettlementConfirmedNotApplied:
					policy.NotAppliedMode = ports.RunClosedFailureReconcile
				}
			})
			inputs := fakes.NewRunSettlementInputStoreV2()
			for _, policy := range fixture.policies {
				if err := inputs.PutPolicy(policy); err != nil {
					t.Fatal(err)
				}
			}
			for id, fact := range fixture.participantsFor {
				if fact.RequirementID == ports.RunRequirementDomainCommits {
					fact.Disposition = testCase.disposition
				}
				if err := inputs.PutParticipant(fact); err != nil {
					t.Fatal(err)
				}
				fixture.participantsFor[id] = fact
			}
			if err := inputs.PutExecution(fixture.execution); err != nil {
				t.Fatal(err)
			}
			fixture.gateway.Participants, fixture.gateway.Policies, fixture.gateway.Execution = inputs, inputs, inputs
			result, err := fixture.gateway.StopAndSettleRunV2(context.Background(), kernel.StopAndSettleRunRequestV2{Scope: fixture.run.Scope, RunID: fixture.run.ID, ExpectedRunRevision: fixture.run.Revision})
			if testCase.allow {
				if err != nil || result.Run.Outcome != core.OutcomeNeedsReconciliation {
					t.Fatalf("explicit reconcile policy did not derive NeedsReconciliation: %+v %v", result, err)
				}
			} else if !core.HasReason(err, core.ReasonRunSettlementBlocked) {
				t.Fatalf("missing explicit mode did not block: %v", err)
			}
		})
	}
}

func TestRunSettlementGatewayV2ExecutionUnknownAndLostPolicyMatrix(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name    string
		truth   ports.RunExecutionTruthV2
		allow   bool
		outcome core.ExecutionOutcome
	}{
		{name: "unknown_blocks", truth: ports.RunExecutionUnknown},
		{name: "unknown_indeterminate", truth: ports.RunExecutionUnknown, allow: true, outcome: core.OutcomeIndeterminate},
		{name: "lost_blocks", truth: ports.RunExecutionConfirmedLost},
		{name: "lost_confirmed", truth: ports.RunExecutionConfirmedLost, allow: true, outcome: core.OutcomeLost},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newRunSettlementGatewayFixtureWithPolicyV2(t, testCase.truth, func(requirement ports.RunSettlementRequirementV2, policy *ports.RunSettlementPolicyFactV2) {
				if requirement.Kind == ports.RunRequirementExecutionTruth && testCase.allow {
					policy.UnknownMode = ports.RunUnknownTerminalizeIndeterminate
					policy.AllowConfirmedLost = testCase.truth == ports.RunExecutionConfirmedLost
				}
			})
			result, err := fixture.gateway.StopAndSettleRunV2(context.Background(), kernel.StopAndSettleRunRequestV2{Scope: fixture.run.Scope, RunID: fixture.run.ID, ExpectedRunRevision: fixture.run.Revision})
			if testCase.allow {
				if err != nil || result.Run.Outcome != testCase.outcome {
					t.Fatalf("authorized execution truth did not derive expected Outcome: %+v %v", result, err)
				}
			} else if !core.HasReason(err, core.ReasonRunSettlementBlocked) {
				t.Fatalf("execution truth terminalized without exact policy: %v", err)
			}
		})
	}
}

func TestRunSettlementGatewayV2EffectSettlementDispositionMatrix(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name        string
		disposition control.EffectSettlementDispositionV2
		unknown     bool
		allow       bool
	}{
		{name: "applied", disposition: control.SettlementConfirmedApplied, allow: true},
		{name: "not_applied_blocks", disposition: control.SettlementConfirmedNotApplied},
		{name: "not_applied_reconciles", disposition: control.SettlementConfirmedNotApplied, allow: true},
		{name: "failed_blocks", disposition: control.SettlementConfirmedFailed},
		{name: "failed_reconciles", disposition: control.SettlementConfirmedFailed, allow: true},
		{name: "unknown_blocks", unknown: true},
		{name: "unknown_reconciles", unknown: true, allow: true},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newRunSettlementGatewayFixtureWithPolicyV2(t, ports.RunExecutionTerminalCompleted, func(requirement ports.RunSettlementRequirementV2, policy *ports.RunSettlementPolicyFactV2) {
				if requirement.Kind != ports.RunRequirementEffects || !testCase.allow {
					return
				}
				if testCase.unknown {
					policy.UnknownMode = ports.RunUnknownTerminalizeReconciliation
				}
				if testCase.disposition == control.SettlementConfirmedFailed {
					policy.FailureMode = ports.RunClosedFailureReconcile
				}
				if testCase.disposition == control.SettlementConfirmedNotApplied {
					policy.NotAppliedMode = ports.RunClosedFailureReconcile
				}
			})
			createRunEffectSettlementFixtureV2(t, fixture, testCase.disposition, testCase.unknown)
			result, err := fixture.gateway.StopAndSettleRunV2(context.Background(), kernel.StopAndSettleRunRequestV2{Scope: fixture.run.Scope, RunID: fixture.run.ID, ExpectedRunRevision: fixture.run.Revision})
			expectCompleted := testCase.disposition == control.SettlementConfirmedApplied
			if expectCompleted {
				if err != nil || result.Run.Outcome != core.OutcomeCompleted {
					t.Fatalf("applied Effect did not close normally: %+v %v", result, err)
				}
				return
			}
			if testCase.allow {
				if err != nil || result.Run.Outcome != core.OutcomeNeedsReconciliation {
					t.Fatalf("Effect disposition did not follow explicit reconcile mode: %+v %v", result, err)
				}
			} else if !core.HasReason(err, core.ReasonRunSettlementBlocked) {
				t.Fatalf("Effect disposition bypassed block policy: %v", err)
			}
		})
	}
}

func TestRunSettlementGatewayV2TerminationProgressIsOrthogonalToOutcome(t *testing.T) {
	t.Parallel()
	fixture := newRunSettlementGatewayFixtureV2(t, ports.RunExecutionTerminalCompleted)
	var terminationID ports.NamespacedNameV2
	inputs := fakes.NewRunSettlementInputStoreV2()
	for _, policy := range fixture.policies {
		if err := inputs.PutPolicy(policy); err != nil {
			t.Fatal(err)
		}
	}
	for id, fact := range fixture.participantsFor {
		if fact.RequirementID == ports.RunRequirementCleanup {
			terminationID = id
			fact.Disposition = ports.RunSettlementUnknown
			fixture.participantsFor[id] = fact
		}
		if err := inputs.PutParticipant(fact); err != nil {
			t.Fatal(err)
		}
	}
	if err := inputs.PutExecution(fixture.execution); err != nil {
		t.Fatal(err)
	}
	fixture.inputs = inputs
	fixture.gateway.Participants, fixture.gateway.Policies, fixture.gateway.Execution = inputs, inputs, inputs
	result, err := fixture.gateway.StopAndSettleRunV2(context.Background(), kernel.StopAndSettleRunRequestV2{Scope: fixture.run.Scope, RunID: fixture.run.ID, ExpectedRunRevision: fixture.run.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.Outcome != core.OutcomeCompleted {
		t.Fatalf("termination_report barrier must not rewrite execution outcome: %+v", result.Run)
	}
	if _, err := fixture.gateway.BuildTerminationReportV2(context.Background(), fixture.run.Scope, fixture.run.ID); !core.HasReason(err, core.ReasonTerminationReportIncomplete) {
		t.Fatalf("TerminationReport was produced before cleanup closed: %v", err)
	}
	for index := 0; index < 100; index++ {
		unchanged, err := fixture.gateway.ReconcileTerminationProgressV2(context.Background(), fixture.run.Scope, fixture.run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if unchanged.Revision != result.Progress.Revision {
			t.Fatalf("unknown participant created false termination progress at iteration %d: %+v", index, unchanged)
		}
	}
	resolved := fixture.participantsFor[terminationID]
	resolved.Revision++
	resolved.Disposition = ports.RunSettlementConfirmedSatisfied
	requirement, _ := findSettlementRequirementFixtureV2(fixture.plan, terminationID)
	record := settlementEvidenceRecordRevisionV2(t, fixture.plan, requirement, "termination-resolved", 100, runSettlementDigestV2(t, "termination-previous"), requirement.SubjectDigest, resolved.Revision)
	fixture.evidence.records[record.Ref] = record
	resolved.Evidence = []ports.EvidenceRecordRefV2{record.Ref}
	if err := inputs.PutParticipant(resolved); err != nil {
		t.Fatal(err)
	}
	progress, err := fixture.gateway.ReconcileTerminationProgressV2(context.Background(), fixture.run.Scope, fixture.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if progress.Revision != 2 {
		t.Fatalf("termination progress did not advance independently: %+v", progress)
	}
	fixture.facts.LoseNextReportReply()
	report, err := fixture.gateway.BuildTerminationReportV2(context.Background(), fixture.run.Scope, fixture.run.ID)
	if err != nil || report.Outcome != core.OutcomeCompleted {
		t.Fatalf("closed termination barriers did not produce a report: %+v %v", report, err)
	}
	replayed, err := fixture.gateway.BuildTerminationReportV2(context.Background(), fixture.run.Scope, fixture.run.ID)
	left, _ := report.DigestV2()
	right, _ := replayed.DigestV2()
	if err != nil || left != right || replayed.CompletedUnixNano != progress.UpdatedUnixNano {
		t.Fatalf("termination Report is not create-once/deterministic: %+v %v", replayed, err)
	}
}

type runSettlementGatewayFixtureV2 struct {
	now             time.Time
	run             core.AgentRunRecord
	plan            ports.RunSettlementPlanFactV2
	execution       ports.ExecutionSettlementInspectionV2
	participantsFor map[ports.NamespacedNameV2]ports.RunSettlementParticipantFactV2
	policies        []ports.RunSettlementPolicyFactV2
	facts           *fakes.RunSettlementStoreV2
	effects         *fakes.EffectStoreV2
	inputs          *fakes.RunSettlementInputStoreV2
	evidence        *settlementEvidenceReaderV2
	gateway         kernel.RunSettlementGatewayV2
}

func newRunSettlementGatewayFixtureV2(t *testing.T, truth ports.RunExecutionTruthV2) *runSettlementGatewayFixtureV2 {
	return newRunSettlementGatewayFixtureWithPolicyV2(t, truth, nil)
}

func newRunSettlementGatewayFixtureWithPolicyV2(t *testing.T, truth ports.RunExecutionTruthV2, mutate func(ports.RunSettlementRequirementV2, *ports.RunSettlementPolicyFactV2)) *runSettlementGatewayFixtureV2 {
	return newRunSettlementGatewayFixtureWithPoliciesV2(t, truth, mutate, true)
}

func newRunSettlementGatewayFixtureWithPoliciesV2(t *testing.T, truth ports.RunExecutionTruthV2, mutate func(ports.RunSettlementRequirementV2, *ports.RunSettlementPolicyFactV2), allowClaimSelfPolicy bool) *runSettlementGatewayFixtureV2 {
	t.Helper()
	now := time.Date(2026, 7, 14, 14, 30, 0, 0, time.UTC)
	run := runningRecord(runScope(t), core.AgentRunID("run-gateway-"+string(truth)), now)
	plan := runSettlementPlanFixtureV2(t, run, now)
	binding, set := settlementBindingProjectionV2(t, plan, now)
	setDigest, _ := control.BindingSetDigestV2(set)
	setSemantic, _ := control.BindingSetSemanticDigestV2(set)
	plan.BindingSet = ports.RunBindingSetRefV2{ID: set.ID, Revision: set.Revision, Digest: setDigest, SemanticDigest: setSemantic}
	for index := range plan.Requirements {
		plan.Requirements[index].Owner.BindingSetID = set.ID
		plan.Requirements[index].Owner.BindingSetRevision = set.Revision
		plan.Requirements[index].Owner.ComponentID = binding.ComponentID
		plan.Requirements[index].Owner.ManifestDigest = binding.ManifestDigest
		plan.Requirements[index].Owner.ArtifactDigest = binding.Manifest.ArtifactDigest
		plan.Requirements[index].Owner.Capability = "runtime/settle-run"
		plan.Requirements[index].Schema = binding.Manifest.Schemas[0]
	}
	plan.Execution.Binding = plan.Requirements[0].Owner
	plan.Execution.SubjectDigest, _ = plan.Execution.DigestV2()
	for index := range plan.Requirements {
		if plan.Requirements[index].Kind == ports.RunRequirementExecutionTruth {
			plan.Requirements[index].SubjectDigest = plan.Execution.SubjectDigest
		}
	}

	inputs := fakes.NewRunSettlementInputStoreV2()
	policies := make([]ports.RunSettlementPolicyFactV2, 0, len(plan.Requirements)+1)
	for index := range plan.Requirements {
		requirement := &plan.Requirements[index]
		policy := settlementPolicyFixtureV2(t, plan, *requirement, now, false)
		if mutate != nil {
			mutate(*requirement, &policy)
			sealed, sealErr := ports.SealRunSettlementPolicyFactV2(policy)
			if sealErr != nil {
				t.Fatal(sealErr)
			}
			policy = sealed
		}
		requirement.Policy = ports.RunSettlementPolicyBindingRefV2{Ref: policy.Ref, Revision: policy.Revision, Digest: policy.Digest, SemanticDigest: policy.SemanticDigest}
		policies = append(policies, policy)
	}
	claimRequirement := plan.Requirements[0]
	claimRequirement.ID = ports.RunRequirementClaimAssociation
	claimPolicy := settlementPolicyFixtureV2(t, plan, claimRequirement, now, true)
	claimPolicy.AllowSelfPolicy = allowClaimSelfPolicy
	claimPolicy, _ = ports.SealRunSettlementPolicyFactV2(claimPolicy)
	plan.Claim.OmissionPolicy = &ports.RunSettlementPolicyBindingRefV2{Ref: claimPolicy.Ref, Revision: 1, Digest: claimPolicy.Digest, SemanticDigest: claimPolicy.SemanticDigest}
	policies = append(policies, claimPolicy)
	for _, policy := range policies {
		if err := inputs.PutPolicy(policy); err != nil {
			t.Fatal(err)
		}

	}
	facts := fakes.NewRunSettlementStoreV2(func() time.Time { return now })
	planAdmission := staticPlanCertificationV3(t, run, plan, now)
	run = persistRunningRunBundleV3(t, facts, run, plan, planAdmission)
	effects := fakes.NewEffectStoreV2(func() time.Time { return now })
	effects.SetRunFacts(facts)
	index := control.RunEffectIndexFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: "effect-index-" + string(run.ID), Revision: 1, RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, State: control.RunEffectIndexOpen, HeadSegmentDigest: ports.EvidenceGenesisDigestV2, Watermark: 1, CreatedUnixNano: now.UnixNano()}
	if _, err := effects.CreateRunEffectIndexV2(context.Background(), index); err != nil {
		t.Fatal(err)
	}
	evidence := &settlementEvidenceReaderV2{records: map[ports.EvidenceRecordRefV2]ports.EvidenceLedgerRecordV2{}}
	executionRequirement, _ := findSettlementRequirementFixtureV2(plan, ports.RunRequirementExecutionTruth)
	execution := ports.ExecutionSettlementInspectionV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: "inspection-" + string(run.ID), Revision: 1, RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest, RunRevision: run.Revision + 1, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, Subject: plan.Execution, Truth: truth, SourceEpoch: run.Scope.Instance.Epoch, SourceSequence: 1, InspectedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	execution.PayloadDigest, _ = execution.EvidenceSubjectDigestV2()
	executionEvidence := settlementExecutionEvidenceRecordV2(t, plan, executionRequirement, execution)
	execution.Evidence = executionEvidence.Ref
	evidence.records[executionEvidence.Ref] = executionEvidence
	if err := inputs.PutExecution(execution); err != nil {
		t.Fatal(err)
	}
	participants := map[ports.NamespacedNameV2]ports.RunSettlementParticipantFactV2{}
	planRef, _ := plan.RefV2()
	sequence := uint64(2)
	previous := executionEvidence.Ref.RecordDigest
	for _, requirement := range plan.Requirements {
		if requirement.Kind == ports.RunRequirementExecutionTruth || requirement.Kind == ports.RunRequirementEffects {
			continue
		}
		record := settlementEvidenceRecordV2(t, plan, requirement, string(requirement.ID), sequence, previous, requirement.SubjectDigest)
		sequence++
		previous = record.Ref.RecordDigest
		evidence.records[record.Ref] = record
		requirementDigest, _ := requirement.DigestV2()
		participant := ports.RunSettlementParticipantFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: "participant-" + string(requirement.ID[8:]), Revision: 1, RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, Plan: planRef, RequirementID: requirement.ID, RequirementDigest: requirementDigest, SubjectDigest: requirement.SubjectDigest, Owner: requirement.Owner, Disposition: ports.RunSettlementConfirmedSatisfied, Evidence: []ports.EvidenceRecordRefV2{record.Ref}, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
		if err := inputs.PutParticipant(participant); err != nil {
			t.Fatal(err)
		}
		participants[requirement.ID] = participant
	}
	bindings := &settlementBindingReaderV2{binding: binding, set: set}
	authorities := settlementAuthorityReaderV2{}
	for _, policy := range policies {
		authorities[policy.PolicyAuthority.Ref] = ports.DispatchAuthorityFactV2{Ref: policy.PolicyAuthority.Ref, Digest: policy.PolicyAuthority.Digest, Revision: policy.PolicyAuthority.Revision, Scope: policy.ExecutionScope, ActionScopeDigest: policy.ActionScopeDigest, State: ports.AuthorityFactActive, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	}
	claims := fakes.NewRunClaimAssociationStoreV2()
	gateway := kernel.RunSettlementGatewayV2{Facts: facts, Effects: effects, Claims: claims, Evidence: evidence, Execution: inputs, Participants: inputs, Policies: inputs, Bindings: bindings, Authority: authorities, PlanAdmissions: planAdmission, Clock: func() time.Time { return now }}
	return &runSettlementGatewayFixtureV2{now: now, run: run, plan: plan, execution: execution, participantsFor: participants, policies: policies, facts: facts, effects: effects, inputs: inputs, evidence: evidence, gateway: gateway}
}

func TestRunLifecycleGovernancePortV3CreatePendingLostRepliesAndExactInspect(t *testing.T) {
	now := time.Date(2026, 7, 14, 17, 0, 0, 0, time.UTC)
	run := runningRecord(runScope(t), "run-lifecycle-public", now)
	run.Status = core.RunPending
	run.Revision = 1
	run.StartedAt = time.Time{}
	plan := runSettlementPlanFixtureV2(t, run, now)
	planAdmission := staticPlanCertificationV3(t, run, plan, now)
	certificationRef, err := planAdmission.fact.RefV3()
	if err != nil {
		t.Fatal(err)
	}
	certification, err := ports.NewRunSettlementPlanCertificationAssociationV3(run, plan, certificationRef)
	if err != nil {
		t.Fatal(err)
	}
	facts := fakes.NewRunSettlementStoreV2(func() time.Time { return now })
	effects := fakes.NewEffectStoreV2(func() time.Time { return now })
	effects.SetRunFacts(facts)
	gateway := kernel.RunSettlementGatewayV2{Facts: facts, Effects: effects, PlanAdmissions: planAdmission, Clock: func() time.Time { return now }}
	facts.LoseNextBundleReply()
	effects.LoseNextRunEffectReply()
	request := ports.CreatePendingRunRequestV3{Run: run, Plan: plan, Certification: certification, EffectIndexID: "effect-index-lifecycle-public"}
	envelope, err := gateway.CreatePendingRunV3(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Phase != ports.RunLifecyclePendingPreparedV3 || envelope.Run.Status != core.RunPending || envelope.EffectIndex.EffectCount != 0 || envelope.EffectIndex.Frozen {
		t.Fatalf("unexpected pending lifecycle envelope: %#v", envelope)
	}
	if err := envelope.Validate(); err != nil {
		t.Fatalf("public pending lifecycle envelope is not independently valid: %v", err)
	}
	for name, mutate := range map[string]func(*ports.RunLifecycleEnvelopeV3){
		"plan run": func(value *ports.RunLifecycleEnvelopeV3) { value.Plan.RunID = "other-run" },
		"index scope": func(value *ports.RunLifecycleEnvelopeV3) {
			value.EffectIndex.ExecutionScopeDigest = runSettlementDigestV2(t, "other-scope")
		},
		"phase": func(value *ports.RunLifecycleEnvelopeV3) { value.Phase = ports.RunLifecycleRunningV3 },
	} {
		forged := envelope
		mutate(&forged)
		if err := forged.Validate(); err == nil {
			t.Fatalf("public lifecycle accepted forged %s relation", name)
		}
	}
	replayed, err := gateway.CreatePendingRunV3(context.Background(), request)
	if err != nil || replayed.Plan != envelope.Plan || replayed.EffectIndex.Digest != envelope.EffectIndex.Digest {
		t.Fatalf("pending lifecycle create did not recover exactly: %#v err=%v", replayed, err)
	}
	changed := request
	changed.Plan.ID = "different-plan"
	if _, err := gateway.CreatePendingRunV3(context.Background(), changed); err == nil {
		t.Fatal("same Run with a different Plan was accepted")
	}
}

func TestRunLifecycleGovernancePortV3TerminalCleanupResumesByInspect(t *testing.T) {
	fixture := newRunSettlementGatewayFixtureV2(t, ports.RunExecutionTerminalCompleted)
	inputs := fakes.NewRunSettlementInputStoreV2()
	for _, policy := range fixture.policies {
		if err := inputs.PutPolicy(policy); err != nil {
			t.Fatal(err)
		}
	}
	if err := inputs.PutExecution(fixture.execution); err != nil {
		t.Fatal(err)
	}
	cleanup := fixture.participantsFor[ports.RunRequirementCleanup]
	cleanup.Disposition = ports.RunSettlementUnknown
	for requirementID, participant := range fixture.participantsFor {
		if requirementID == ports.RunRequirementCleanup {
			participant = cleanup
		}
		if err := inputs.PutParticipant(participant); err != nil {
			t.Fatal(err)
		}
	}
	fixture.inputs = inputs
	fixture.gateway.Execution = inputs
	fixture.gateway.Participants = inputs
	fixture.gateway.Policies = inputs
	stop := ports.BeginStopRunRequestV3{ExecutionScope: fixture.run.Scope, RunID: fixture.run.ID, ExpectedRunRevision: fixture.run.Revision}
	fixture.facts.LoseNextCommitReply()
	terminal, err := fixture.gateway.StopAndSettleRunV3(context.Background(), stop)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.Phase != ports.RunLifecycleTerminalCleanupV3 || terminal.Decision == nil || terminal.Progress == nil || terminal.Report != nil {
		t.Fatalf("terminal Run did not expose interim cleanup watermarks: %#v", terminal)
	}
	if err := terminal.Validate(); err != nil {
		t.Fatalf("terminal cleanup envelope is not independently valid: %v", err)
	}
	for name, mutate := range map[string]func(*ports.RunLifecycleEnvelopeV3){
		"old closure attempt": func(value *ports.RunLifecycleEnvelopeV3) {
			decision := *value.Decision
			decision.Closure.Attempt++
			value.Decision = &decision
		},
		"foreign progress decision": func(value *ports.RunLifecycleEnvelopeV3) {
			progress := *value.Progress
			progress.Decision.Digest = runSettlementDigestV2(t, "other-decision")
			value.Progress = &progress
		},
	} {
		forged := terminal
		mutate(&forged)
		if err := forged.Validate(); err == nil {
			t.Fatalf("terminal lifecycle accepted forged %s chain", name)
		}
	}
	replayed, err := fixture.gateway.StopAndSettleRunV3(context.Background(), stop)
	if err != nil || replayed.Decision == nil || replayed.Decision.Digest != terminal.Decision.Digest {
		t.Fatalf("terminal Stop replay did not Inspect exact Decision: %#v err=%v", replayed, err)
	}
	progressRevision := terminal.Progress.Revision
	for index := 0; index < 100; index++ {
		inspected, reconcileErr := fixture.gateway.ReconcileRunTerminationV3(context.Background(), ports.RunTerminationRequestV3{ExecutionScope: fixture.run.Scope, RunID: fixture.run.ID})
		if reconcileErr != nil {
			t.Fatal(reconcileErr)
		}
		if inspected.Report != nil || inspected.Progress == nil || inspected.Progress.Revision != progressRevision || inspected.Progress.UnresolvedCount == 0 {
			t.Fatalf("unknown cleanup was retried or reported closed at iteration %d: %#v", index, inspected)
		}
	}
	cleanup.Disposition = ports.RunSettlementConfirmedSatisfied
	cleanup.Revision++
	cleanup.CreatedUnixNano = fixture.now.UnixNano()
	cleanupRequirement, _ := findSettlementRequirementFixtureV2(fixture.plan, ports.RunRequirementCleanup)
	cleanupEvidence := settlementEvidenceRecordRevisionV2(t, fixture.plan, cleanupRequirement, "cleanup-resolved", 10_000, runSettlementDigestV2(t, "cleanup-previous"), cleanupRequirement.SubjectDigest, cleanup.Revision)
	fixture.evidence.records[cleanupEvidence.Ref] = cleanupEvidence
	cleanup.Evidence = []ports.EvidenceRecordRefV2{cleanupEvidence.Ref}
	if err := inputs.PutParticipant(cleanup); err != nil {
		t.Fatal(err)
	}
	closed, err := fixture.gateway.ReconcileRunTerminationV3(context.Background(), ports.RunTerminationRequestV3{ExecutionScope: fixture.run.Scope, RunID: fixture.run.ID})
	if err != nil {
		t.Fatal(err)
	}
	if closed.Phase != ports.RunLifecycleTerminationClosedV3 || closed.Report == nil || closed.Progress == nil || closed.Progress.UnresolvedCount != 0 {
		t.Fatalf("settled cleanup did not produce exact termination report: %#v", closed)
	}
	if err := closed.Validate(); err != nil {
		t.Fatalf("closed lifecycle envelope is not independently valid: %v", err)
	}
	forgedReport := closed
	report := *forgedReport.Report
	report.Progress.Revision++
	forgedReport.Report = &report
	if err := forgedReport.Validate(); err == nil {
		t.Fatal("closed lifecycle accepted a Report linked to stale Progress")
	}
}

func TestRunLifecycleGovernanceV3RejectsLegacyBundleBeforeAnyMutation(t *testing.T) {
	now := time.Unix(720_000, 0)
	run := runningRecord(runScope(t), "run-legacy-lifecycle-v3", now)
	run.Status, run.Revision, run.StartedAt = core.RunPending, 1, time.Time{}
	plan := runSettlementPlanFixtureV2(t, run, now)
	facts := fakes.NewRunSettlementStoreV2(func() time.Time { return now })
	if _, err := facts.CreateRunBundleV2(context.Background(), control.RunBundleCreateRequestV2{Run: run, Plan: plan}); err != nil {
		t.Fatal(err)
	}
	effects := fakes.NewEffectStoreV2(func() time.Time { return now })
	effects.SetRunFacts(facts)
	admission := staticPlanCertificationV3(t, run, plan, now)
	gateway := kernel.RunSettlementGatewayV2{Facts: facts, Effects: effects, PlanAdmissions: admission, Clock: func() time.Time { return now }}
	request := ports.BeginStopRunRequestV3{ExecutionScope: run.Scope, RunID: run.ID, ExpectedRunRevision: run.Revision}
	if _, err := gateway.BeginStopRunV3(context.Background(), request); err == nil {
		t.Fatal("legacy uncertified bundle entered BeginStop")
	}
	if _, err := gateway.StopAndSettleRunV3(context.Background(), request); err == nil {
		t.Fatal("legacy uncertified bundle entered settlement")
	}
	if _, err := gateway.ReconcileRunTerminationV3(context.Background(), ports.RunTerminationRequestV3{ExecutionScope: run.Scope, RunID: run.ID}); err == nil {
		t.Fatal("legacy uncertified bundle entered termination reconciliation")
	}
	current, err := facts.InspectRun(context.Background(), run.Scope, run.ID)
	if err != nil || current.Status != core.RunPending || current.Revision != 1 {
		t.Fatalf("failed certified preflight mutated the Run: %#v %v", current, err)
	}
	if _, err := facts.InspectCurrentRunSettlementClosureV2(context.Background(), run.Scope, run.ID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed certified preflight created a Closure: %v", err)
	}
	if _, err := facts.InspectRunTerminationProgressV2(context.Background(), run.Scope, run.ID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed certified preflight created termination Progress: %v", err)
	}
	if _, err := facts.InspectRunTerminationReportV2(context.Background(), run.Scope, run.ID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed certified preflight created termination Report: %v", err)
	}
}

func settlementPolicyFixtureV2(t *testing.T, plan ports.RunSettlementPlanFactV2, requirement ports.RunSettlementRequirementV2, now time.Time, allowMissingClaim bool) ports.RunSettlementPolicyFactV2 {
	t.Helper()
	action := runSettlementDigestV2(t, "settlement-action-"+string(requirement.ID))
	policy := ports.RunSettlementPolicyFactV2{Ref: "settlement-policy-" + string(requirement.ID[8:]), Revision: 1, RunID: plan.RunID, PlanID: plan.ID, PlanRevision: plan.Revision, RequirementID: requirement.ID, ExecutionScopeDigest: plan.ExecutionScopeDigest, ExecutionScope: plan.ExecutionScope, ActionScopeDigest: action, PolicyOwner: requirement.Owner, PolicyAuthority: ports.AuthorityBindingRefV2{Ref: "authority-settlement-" + string(requirement.ID), Digest: runSettlementDigestV2(t, "authority-settlement-"+string(requirement.ID)), Revision: 1, Epoch: plan.ExecutionScope.AuthorityEpoch}, UnknownMode: ports.RunUnknownBlock, FailureMode: ports.RunClosedFailureBlock, NotAppliedMode: ports.RunClosedFailureBlock, AllowMissingClaim: allowMissingClaim, AllowSelfPolicy: true, State: ports.RunSettlementPolicyActive, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	sealed, err := ports.SealRunSettlementPolicyFactV2(policy)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

type settlementEvidenceReaderV2 struct {
	ports.EvidenceLedgerFactPortV2
	records map[ports.EvidenceRecordRefV2]ports.EvidenceLedgerRecordV2
}

type staticExecutionSettlementInspectorV2 struct {
	fact ports.ExecutionSettlementInspectionV2
}

func (s staticExecutionSettlementInspectorV2) InspectRunExecutionV2(context.Context, ports.RunExecutionInspectionRequestV2) (ports.ExecutionSettlementInspectionV2, error) {
	return s.fact, nil
}

type changingExecutionSettlementInspectorV2 struct {
	mu    sync.Mutex
	reads uint64
	first ports.ExecutionSettlementInspectionV2
	next  ports.ExecutionSettlementInspectionV2
}

func (s *changingExecutionSettlementInspectorV2) InspectRunExecutionV2(context.Context, ports.RunExecutionInspectionRequestV2) (ports.ExecutionSettlementInspectionV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	if s.reads == 1 {
		return s.first, nil
	}
	return s.next, nil
}

type staticRunClaimAssociationPortV2 struct {
	ports.RunClaimAssociationPortV2
	fact ports.RunClaimAssociationFactV2
}

func (s staticRunClaimAssociationPortV2) InspectRunClaimAssociation(context.Context, core.Digest, core.AgentRunID) (ports.RunClaimAssociationFactV2, error) {
	return s.fact, nil
}

func attachRunSettlementClaimV2(t *testing.T, fixture *runSettlementGatewayFixtureV2, kind core.RunCompletionClaimKind) (ports.RunClaimAssociationFactV2, ports.EvidenceLedgerRecordV2) {
	t.Helper()
	association, record := runSettlementClaimAssociationV2(t, fixture, kind)
	store := fakes.NewRunClaimAssociationStoreV2()
	if _, err := store.CreateRunClaimAssociation(context.Background(), association); err != nil {
		t.Fatal(err)
	}
	fixture.evidence.records[record.Ref] = record
	fixture.gateway.Claims = store
	return association, record
}

func runSettlementClaimAssociationV2(t *testing.T, fixture *runSettlementGatewayFixtureV2, kind core.RunCompletionClaimKind) (ports.RunClaimAssociationFactV2, ports.EvidenceLedgerRecordV2) {
	t.Helper()
	sequence := uint64(1_000)
	candidate := ports.EvidenceEventCandidateV2{
		ContractVersion: ports.EvidenceContractVersionV2,
		LedgerScope: ports.EvidenceLedgerScopeV2{
			Partition:  ports.EvidencePartitionRun,
			TenantID:   fixture.run.Scope.Identity.TenantID,
			IdentityID: fixture.run.Scope.Identity.ID,
			LineageID:  fixture.run.Scope.Lineage.ID,
			InstanceID: fixture.run.Scope.Instance.ID,
			RunID:      fixture.run.ID,
		},
		EventID:                   "event-settlement-claim",
		RegistrationID:            "registration-settlement-claim",
		RegistrationRevision:      1,
		SourceConfigurationDigest: runSettlementDigestV2(t, "claim-source-configuration"),
		SourcePolicy: ports.EvidenceSourcePolicyBindingRefV2{
			Ref:      "policy-settlement-claim",
			Digest:   runSettlementDigestV2(t, "claim-source-policy"),
			Revision: 1,
		},
		SourceID:       "runtime/run-claim",
		SourceEpoch:    fixture.run.Scope.Instance.Epoch,
		SourceSequence: sequence,
		TrustClass:     ports.EvidenceTrustClaim,
		ClaimKind:      kind,
		EventKind:      "runtime/run-completion",
		CustomClass:    "runtime/claim",
		ExecutionScope: fixture.run.Scope,
		Payload: ports.EvidencePayloadRefV2{
			Schema: ports.SchemaRefV2{
				Namespace:     "runtime",
				Name:          "run-claim",
				Version:       "2.0.0",
				MediaType:     "application/octet-stream",
				ContentDigest: runSettlementDigestV2(t, "claim-schema"),
			},
			ContentDigest: runSettlementDigestV2(t, "claim-payload"),
			Revision:      1,
			Length:        32,
			Ref:           "evidence://settlement/claim",
		},
		Causation:     []ports.EvidenceCausationRefV2{},
		CorrelationID: string(fixture.run.ID),
		Producer:      fixture.plan.Execution.Binding,
		Authority: ports.AuthorityBindingRefV2{
			Ref:      "authority-settlement-claim",
			Digest:   runSettlementDigestV2(t, "claim-authority"),
			Revision: 1,
			Epoch:    fixture.run.Scope.AuthorityEpoch,
		},
		ObservedUnixNano: fixture.now.UnixNano(),
	}
	record, err := control.NewEvidenceLedgerRecordV2(candidate, sequence, runSettlementDigestV2(t, "claim-previous-record"), fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	runIdentity, _ := ports.RunIdentityDigestV2(fixture.run)
	scopeDigest, _ := ports.ExecutionScopeDigestV2(fixture.run.Scope)
	id, _ := ports.RunClaimAssociationIDV2(fixture.run.ID, record.Ref)
	association := ports.RunClaimAssociationFactV2{
		ContractVersion:          ports.RunClaimAssociationContractVersionV2,
		ID:                       id,
		Revision:                 1,
		State:                    ports.RunClaimAssociatedV2,
		RunID:                    fixture.run.ID,
		RunRevisionAtAssociation: fixture.run.Revision,
		RunIdentityDigest:        runIdentity,
		ExecutionScope:           fixture.run.Scope,
		ExecutionScopeDigest:     scopeDigest,
		LineagePlanDigest:        fixture.run.Scope.Lineage.PlanDigest,
		ClaimKind:                kind,
		RegistrationID:           candidate.RegistrationID,
		SourceID:                 candidate.SourceID,
		SourceEpoch:              candidate.SourceEpoch,
		SourceSequence:           candidate.SourceSequence,
		EventID:                  candidate.EventID,
		Evidence:                 record.Ref,
		CandidateDigest:          record.CandidateDigest,
		PayloadDigest:            candidate.Payload.ContentDigest,
		ObservedUnixNano:         candidate.ObservedUnixNano,
		EvidenceIngestedUnixNano: record.IngestedUnixNano,
		CreatedUnixNano:          record.IngestedUnixNano,
	}
	if err := association.Validate(); err != nil {
		t.Fatal(err)
	}
	return association, record
}

func (s *settlementEvidenceReaderV2) InspectRecord(_ context.Context, ref ports.EvidenceRecordRefV2) (ports.EvidenceLedgerRecordV2, error) {
	record, exists := s.records[ref]
	if !exists {
		return ports.EvidenceLedgerRecordV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "settlement Evidence record does not exist")
	}
	return record, nil
}

type settlementBindingReaderV2 struct {
	control.BindingFactPortV2
	binding control.BindingFactV2
	set     control.BindingSetFactV2
}

type settlementAuthorityReaderV2 map[string]ports.DispatchAuthorityFactV2

func (r settlementAuthorityReaderV2) InspectDispatchAuthority(_ context.Context, ref string) (ports.DispatchAuthorityFactV2, error) {
	fact, ok := r[ref]
	if !ok {
		return ports.DispatchAuthorityFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonEffectAuthorizationMissing, "settlement authority does not exist")
	}
	return fact, nil
}

func (s *settlementBindingReaderV2) InspectBinding(_ context.Context, id string) (control.BindingFactV2, error) {
	if id != s.binding.ID {
		return control.BindingFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Binding does not exist")
	}
	return s.binding, nil
}

func (s *settlementBindingReaderV2) InspectBindingSet(_ context.Context, id string) (control.BindingSetFactV2, error) {
	if id != s.set.ID {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "BindingSet does not exist")
	}
	return s.set, nil
}

func settlementBindingProjectionV2(t *testing.T, plan ports.RunSettlementPlanFactV2, now time.Time) (control.BindingFactV2, control.BindingSetFactV2) {
	t.Helper()
	schema := plan.Requirements[0].Schema
	artifact := runSettlementDigestV2(t, "gateway-artifact")
	manifest := ports.ComponentManifestV2{ContractVersion: ports.BindingContractVersionV2, ComponentID: "runtime/settlement-owner", Kind: "runtime/settlement-adapter", GovernanceCategory: "runtime/execution", SemanticVersion: "1.0.0", ArtifactDigest: artifact, Contract: ports.ContractBindingV2{Name: "runtime/run-settlement", Version: "2.0.0", Compatible: ports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}}, Schemas: []ports.SchemaRefV2{schema}, Locality: ports.LocalityHostControlPlane, Dependencies: []ports.ComponentDependencyV2{}, RequiredCapabilities: []ports.CapabilityRequirementV2{}, ProvidedCapabilities: []ports.ProvidedCapabilityV2{{Capability: "runtime/settle-run", TTLSeconds: 3600, Schemas: []ports.SchemaRefV2{schema}}}, Conformance: ports.ConformanceFullyControlled, ResidualClass: ports.ResidualInspectable, Owners: []ports.OwnerAssignmentV2{{Role: ports.OwnerEffect, OwnerComponentID: "runtime/settlement-owner"}, {Role: ports.OwnerSettlement, OwnerComponentID: "runtime/settlement-owner"}, {Role: ports.OwnerCleanup, OwnerComponentID: "runtime/settlement-owner"}}, Credentials: []ports.CredentialRequirementV2{}, OfflinePolicy: ports.OfflineDenied, Extensions: []ports.GovernanceExtensionV2{}, Annotations: []ports.DisplayAnnotationV2{}}
	manifestDigest, _ := manifest.BindingDigestV2()
	grant := ports.CapabilityGrantV2{Capability: "runtime/settle-run", EvidenceDigest: runSettlementDigestV2(t, "gateway-grant"), ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	governance := runSettlementDigestV2(t, "gateway-governance")
	binding := control.BindingFactV2{ID: "binding-settlement-owner", ComponentID: manifest.ComponentID, Manifest: manifest, ManifestDigest: manifestDigest, GovernanceDigest: governance, State: control.BindingBound, Revision: 4, Grants: []ports.CapabilityGrantV2{grant}, ProbedUnixNano: now.UnixNano(), CertifiedUnixNano: now.UnixNano(), ConformanceEvidenceDigest: runSettlementDigestV2(t, "gateway-conformance"), ExpiresUnixNano: now.Add(time.Hour).UnixNano(), BindingSetID: "binding-set-run"}
	member := control.BindingMemberV2{BindingID: binding.ID, BindingRevision: binding.Revision, ComponentID: binding.ComponentID, Kind: manifest.Kind, ManifestDigest: manifestDigest, ArtifactDigest: artifact, Contract: manifest.Contract, Owners: manifest.Owners, Grants: []ports.CapabilityGrantV2{grant}}
	set := control.BindingSetFactV2{ID: binding.BindingSetID, PlanID: "binding-plan", PlanDigest: runSettlementDigestV2(t, "binding-plan"), GovernanceDigest: governance, State: control.BindingSetActive, Revision: 1, Members: []control.BindingMemberV2{member}, TopologicalOrder: []ports.ComponentIDV2{binding.ComponentID}, Residuals: []control.BindingResidualV2{}, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	return binding, set
}

func settlementEvidenceRecordV2(t *testing.T, plan ports.RunSettlementPlanFactV2, requirement ports.RunSettlementRequirementV2, suffix string, sequence uint64, previous core.Digest, payload core.Digest) ports.EvidenceLedgerRecordV2 {
	return settlementEvidenceRecordRevisionV2(t, plan, requirement, suffix, sequence, previous, payload, 1)
}

func settlementExecutionEvidenceRecordV2(t *testing.T, plan ports.RunSettlementPlanFactV2, requirement ports.RunSettlementRequirementV2, inspection ports.ExecutionSettlementInspectionV2) ports.EvidenceLedgerRecordV2 {
	t.Helper()
	record := settlementEvidenceRecordRevisionV2(t, plan, requirement, "execution", inspection.SourceSequence, ports.EvidenceGenesisDigestV2, inspection.PayloadDigest, inspection.Revision)
	causation, err := ports.RunExecutionInspectionEvidenceCausationIDV2(inspection)
	if err != nil {
		t.Fatal(err)
	}
	ledgerScopeDigest, _ := record.Candidate.LedgerScope.DigestV2()
	record.Candidate.Causation = []ports.EvidenceCausationRefV2{{LedgerScopeDigest: ledgerScopeDigest, EventID: causation}}
	record, err = control.NewEvidenceLedgerRecordV2(record.Candidate, record.Ref.Sequence, record.PreviousRecordDigest, time.Unix(0, record.IngestedUnixNano))
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func settlementEvidenceRecordRevisionV2(t *testing.T, plan ports.RunSettlementPlanFactV2, requirement ports.RunSettlementRequirementV2, suffix string, sequence uint64, previous core.Digest, payload core.Digest, payloadRevision core.Revision) ports.EvidenceLedgerRecordV2 {
	t.Helper()
	correlation, _ := ports.RunSettlementEvidenceCorrelationIDV2(plan.ID, plan.RunID, requirement.ID, requirement.SubjectDigest)
	ledgerScope := ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionRun, TenantID: plan.ExecutionScope.Identity.TenantID, IdentityID: plan.ExecutionScope.Identity.ID, LineageID: plan.ExecutionScope.Lineage.ID, InstanceID: plan.ExecutionScope.Instance.ID, RunID: plan.RunID}
	ledgerScopeDigest, _ := ledgerScope.DigestV2()
	participantID := "participant-" + string(requirement.ID)
	if len(requirement.ID) > 8 {
		participantID = "participant-" + string(requirement.ID[8:])
	}
	causation, _ := ports.RunSettlementEvidenceCausationEventIDV2(plan.ID, plan.RunID, requirement.ID, participantID, payloadRevision)
	candidate := ports.EvidenceEventCandidateV2{ContractVersion: ports.EvidenceContractVersionV2, LedgerScope: ledgerScope, EventID: "event-settlement-" + suffix, RegistrationID: "registration-settlement-" + string(plan.RunID), RegistrationRevision: 1, SourceConfigurationDigest: runSettlementDigestV2(t, "source-config"), SourcePolicy: ports.EvidenceSourcePolicyBindingRefV2{Ref: "source-policy-settlement", Digest: runSettlementDigestV2(t, "source-policy"), Revision: 1}, SourceID: "runtime/settlement-source", SourceEpoch: plan.ExecutionScope.Instance.Epoch, SourceSequence: sequence, TrustClass: requirement.EvidenceTrust, EventKind: requirement.EvidenceKind, CustomClass: "runtime/settlement", ExecutionScope: plan.ExecutionScope, Payload: ports.EvidencePayloadRefV2{Schema: requirement.Schema, ContentDigest: payload, Revision: payloadRevision, Length: 32, Ref: "evidence://settlement/" + suffix}, Causation: []ports.EvidenceCausationRefV2{{LedgerScopeDigest: ledgerScopeDigest, EventID: causation}}, CorrelationID: correlation, Producer: requirement.Owner, Authority: ports.AuthorityBindingRefV2{Ref: "authority-settlement", Digest: runSettlementDigestV2(t, "authority-settlement"), Revision: 1, Epoch: plan.ExecutionScope.AuthorityEpoch}, ObservedUnixNano: time.Unix(0, plan.CreatedUnixNano).UnixNano()}
	record, err := control.NewEvidenceLedgerRecordV2(candidate, sequence, previous, time.Unix(0, plan.CreatedUnixNano))
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func createRunEffectSettlementFixtureV2(t *testing.T, fixture *runSettlementGatewayFixtureV2, disposition control.EffectSettlementDispositionV2, unknown bool) {
	t.Helper()
	partition := control.RunEffectPartitionV2{ExecutionScope: fixture.run.Scope, ExecutionScopeDigest: fixture.plan.ExecutionScopeDigest, RunID: fixture.run.ID, RunIdentityDigest: fixture.plan.RunIdentityDigest}
	index, err := fixture.effects.InspectRunEffectIndexV2(context.Background(), partition)
	if err != nil {
		t.Fatal(err)
	}
	intent := effectIntentV2(t, fixture.now, "effect-settlement-"+string(disposition), "idem-settlement", "domain/settlement")
	if unknown {
		intent.ID = "effect-settlement-unknown"
	}
	intent.Scope, intent.RunID = fixture.run.Scope, fixture.run.ID
	tenantDigest := ports.StableTenantScopeDigestV2(fixture.run.Scope.Identity.TenantID)
	intent.ConflictDomain.ScopeDigest = tenantDigest
	intent.Idempotency.ScopeDigest = tenantDigest
	proposed, err := control.NewProposedEffectFactV2(intent, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := fixture.effects.CreateEffectForRunV2(context.Background(), control.CreateRunEffectRequestV2{Partition: partition, ExpectedIndexRevision: index.Revision, Effect: proposed})
	if err != nil {
		t.Fatal(err)
	}
	accepted := created.Effect
	accepted.State = control.EffectAccepted
	accepted.Revision++
	accepted, err = fixture.effects.CompareAndSwapRunEffectV2(context.Background(), partition, control.EffectFactCASRequestV2{ExpectedRevision: proposed.Revision, Next: accepted})
	if err != nil {
		t.Fatal(err)
	}
	permit := permitForIntentV2(t, intent, "permit-settlement-"+string(intent.ID), "attempt-settlement-"+string(intent.ID), fixture.now)
	fence := core.ExecutionFence{BoundaryScope: core.FenceBoundaryInstance, Scope: permit.Scope, CapabilityGrantDigest: permit.CapabilityGrantDigest, EffectIntentID: permit.IntentID, EffectIntentRevision: permit.IntentRevision, CanonicalPayloadDigest: permit.PayloadDigest, ExpiresAt: fixture.now.Add(10 * time.Second)}
	fenceDigest, fenceErr := ports.DigestExecutionFenceV2(fence)
	if fenceErr != nil || fenceDigest != permit.FenceDigest {
		t.Fatalf("test fence reconstruction drifted: got=%s want=%s err=%v", fenceDigest, permit.FenceDigest, fenceErr)
	}
	if permit.PayloadSchema != intent.Payload.Schema || permit.PayloadRevision != intent.PayloadRevision || permit.ConflictDomain != intent.ConflictDomain || permit.Provider != intent.Provider || permit.EnforcementPoint != intent.Provider || permit.Authority != intent.Authority || permit.Review != intent.Review || permit.Budget != intent.Budget || permit.Policy != intent.Policy || permit.CurrentScope != intent.CurrentScope || permit.Idempotency != intent.Idempotency || permit.RunID != intent.RunID {
		t.Fatal("test permit helper drifted from the exact Effect governance projection")
	}
	issued, err := fixture.effects.IssueRunDispatchPermitV2(context.Background(), partition, control.IssueDispatchPermitRequestV2{EffectID: intent.ID, ExpectedEffectRevision: accepted.Revision, Permit: permit, Fence: fence})
	if err != nil {
		t.Fatal(err)
	}
	begun, err := fixture.effects.BeginRunDispatchV2(context.Background(), partition, control.BeginDispatchRequestV2{EffectID: intent.ID, ExpectedEffectRevision: issued.Effect.Revision, PermitID: permit.ID, ExpectedPermitRevision: issued.Permit.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if unknown {
		next := issued.Effect
		next.State = control.EffectUnknownOutcome
		next.Revision++
		next.UpdatedUnixNano = fixture.now.UnixNano()
		if _, err := fixture.effects.CompareAndSwapRunEffectV2(context.Background(), partition, control.EffectFactCASRequestV2{ExpectedRevision: issued.Effect.Revision, Next: next}); err != nil {
			t.Fatal(err)
		}
		return
	}
	if _, err := fixture.effects.RecordRunEnforcementReceiptV2(context.Background(), partition, control.RecordEnforcementReceiptRequestV2{PermitID: permit.ID, ExpectedPermitRevision: begun.Revision, Receipt: enforcementReceiptV2(t, begun, fixture.now)}); err != nil {
		t.Fatal(err)
	}
	dispatched := issued.Effect
	dispatched.State = control.EffectDispatched
	dispatched.Revision++
	dispatched.UpdatedUnixNano = fixture.now.UnixNano()
	dispatched.DispatchReceipt = &control.ProviderDispatchReceiptV2{PermitID: permit.ID, PermitDigest: begun.PermitDigest, AttemptID: permit.AttemptID, IntentID: intent.ID, IntentRevision: intent.Revision, Provider: intent.Provider, ProviderOperationRef: "operation-settlement", ReceiptRef: "receipt-settlement", ObservationDigest: runSettlementDigestV2(t, "provider-settlement"), ObservedUnixNano: fixture.now.UnixNano()}
	dispatched, err = fixture.effects.CompareAndSwapRunEffectV2(context.Background(), partition, control.EffectFactCASRequestV2{ExpectedRevision: issued.Effect.Revision, Next: dispatched})
	if err != nil {
		t.Fatal(err)
	}
	settled := dispatched
	settled.State = control.EffectSettled
	settled.Revision++
	settled.UpdatedUnixNano = fixture.now.UnixNano()
	settled.Settlement = &control.EffectSettlementFactV2{Owner: intent.Owners[2], Disposition: disposition, ReceiptRef: "settlement-receipt", EvidenceDigest: runSettlementDigestV2(t, "effect-settlement"), SettledUnixNano: fixture.now.UnixNano()}
	if _, err := fixture.effects.CompareAndSwapRunEffectV2(context.Background(), partition, control.EffectFactCASRequestV2{ExpectedRevision: dispatched.Revision, Next: settled}); err != nil {
		t.Fatal(err)
	}
}

func findSettlementRequirementFixtureV2(plan ports.RunSettlementPlanFactV2, kind ports.NamespacedNameV2) (ports.RunSettlementRequirementV2, bool) {
	for _, requirement := range plan.Requirements {
		if requirement.Kind == kind {
			return requirement, true
		}
	}
	return ports.RunSettlementRequirementV2{}, false
}
