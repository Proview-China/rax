package kernel_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type staticClaimBundlePortV3 struct {
	bundle       control.RunBundleV3
	err          error
	inspectCalls atomic.Int64
}

func (s *staticClaimBundlePortV3) CreateRunBundleV3(context.Context, control.RunBundleCreateRequestV3) (control.RunBundleV3, error) {
	panic("unused")
}

func (s *staticClaimBundlePortV3) InspectRunBundleV3(context.Context, core.ExecutionScope, core.AgentRunID) (control.RunBundleV3, error) {
	s.inspectCalls.Add(1)
	return s.bundle, s.err
}

type staticClaimPlanAdmissionV3 struct {
	fact ports.RunSettlementPlanCertificationFactV3
	err  error
}

func (s staticClaimPlanAdmissionV3) CertifyRunSettlementPlanV3(context.Context, ports.CertifyRunSettlementPlanRequestV3) (ports.RunSettlementPlanCertificationFactV3, error) {
	panic("unused")
}

func (s staticClaimPlanAdmissionV3) InspectCertifiedRunSettlementPlanV3(context.Context, core.ExecutionScope, core.AgentRunID) (ports.RunSettlementPlanCertificationFactV3, error) {
	return s.fact, s.err
}

func (s staticClaimPlanAdmissionV3) ValidateRunSettlementPlanCertificationV3(context.Context, ports.RunSettlementPlanCertificationRefV3, core.AgentRunRecord, ports.RunSettlementPlanFactV2) error {
	panic("unused")
}

type certifiedClaimFixtureV3 struct {
	now           time.Time
	run           core.AgentRunRecord
	plan          ports.RunSettlementPlanFactV2
	certification ports.RunSettlementPlanCertificationFactV3
	bundle        control.RunBundleV3
	runs          *fakes.FactStore
	evidence      *governedEvidenceStubV2
	associations  *fakes.RunClaimAssociationStoreV2
	gateway       kernel.RunClaimGatewayV3
}

func newCertifiedClaimFixtureV3(t *testing.T, runID core.AgentRunID) *certifiedClaimFixtureV3 {
	t.Helper()
	now := time.Unix(130_000, 0)
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-claim-v3", ID: "identity-claim-v3", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-claim-v3", PlanDigest: claimDigestV2(t, "lineage-plan-v3")},
		Instance:       core.InstanceRef{ID: "instance-claim-v3", Epoch: 1},
		AuthorityEpoch: 1,
	}
	session, err := ports.DeriveRuntimeExecutionSessionRefV2("endpoint-claim-v3", runID)
	if err != nil {
		t.Fatal(err)
	}
	run := core.AgentRunRecord{ID: runID, Scope: scope, Status: core.RunRunning, Revision: 1, SessionRef: session, StartedAt: now.Add(-time.Second)}
	plan := claimPlanV3(t, run, now)
	certification := claimCertificationV3(t, run, plan, now)
	certificationRef, err := certification.RefV3()
	if err != nil {
		t.Fatal(err)
	}
	association, err := ports.NewRunSettlementPlanCertificationAssociationV3(run, plan, certificationRef)
	if err != nil {
		t.Fatal(err)
	}
	bundle := control.RunBundleV3{Run: run, Plan: plan, Certification: association}
	runs := fakes.NewFactStore(func() time.Time { return now })
	if _, err := runs.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	evidence := newGovernedEvidenceStubV2(now)
	associations := fakes.NewRunClaimAssociationStoreV2()
	legacy, err := kernel.NewRunClaimGatewayV2(evidence, associations, runs, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	fixture := &certifiedClaimFixtureV3{
		now:           now,
		run:           run,
		plan:          plan,
		certification: certification,
		bundle:        bundle,
		runs:          runs,
		evidence:      evidence,
		associations:  associations,
	}
	fixture.gateway = kernel.RunClaimGatewayV3{
		Legacy:         legacy,
		Bundles:        &staticClaimBundlePortV3{bundle: bundle},
		PlanAdmissions: staticClaimPlanAdmissionV3{fact: certification},
	}
	return fixture
}

func TestRunClaimGatewayV3RejectsLegacyOrForgedCertificationBeforeEvidenceMutation(t *testing.T) {
	fixture := newCertifiedClaimFixtureV3(t, "run-claim-v3-preflight")
	request := ports.RunClaimIngestRequestV2{
		ExpectedRunRevision: fixture.run.Revision,
		Candidate:           claimCandidateV2(t, fixture.run, 1, "event-claim-v3-preflight", core.RunClaimCompleted),
	}

	tests := []struct {
		name      string
		bundles   control.RunBundleFactPortV3
		admission ports.RunSettlementPlanAdmissionPortV3
	}{
		{
			name:      "legacy V2 bundle has no certified V3 projection",
			bundles:   &staticClaimBundlePortV3{err: core.NewError(core.ErrorNotFound, core.ReasonRunConflict, "missing V3 bundle")},
			admission: staticClaimPlanAdmissionV3{fact: fixture.certification},
		},
		{
			name:      "historical certification fact is missing",
			bundles:   &staticClaimBundlePortV3{bundle: fixture.bundle},
			admission: staticClaimPlanAdmissionV3{err: core.NewError(core.ErrorNotFound, core.ReasonRunSettlementPlanConflict, "missing certification")},
		},
		{
			name: "bundle association belongs to another Run",
			bundles: func() control.RunBundleFactPortV3 {
				forged := fixture.bundle
				forged.Certification.RunID = "another-run"
				return &staticClaimBundlePortV3{bundle: forged}
			}(),
			admission: staticClaimPlanAdmissionV3{fact: fixture.certification},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gateway := fixture.gateway
			gateway.Bundles = test.bundles
			gateway.PlanAdmissions = test.admission
			if _, err := gateway.IngestRunClaimV3(context.Background(), request); err == nil {
				t.Fatal("uncertified or forged Run was allowed to append claim evidence")
			}
			if fixture.evidence.count() != 0 {
				t.Fatal("certification preflight failure mutated the Evidence ledger")
			}
			scopeDigest, _ := ports.ExecutionScopeDigestV2(fixture.run.Scope)
			if _, err := fixture.associations.InspectRunClaimAssociation(context.Background(), scopeDigest, fixture.run.ID); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("certification preflight failure created an association: %v", err)
			}
		})
	}
}

func TestRunClaimGatewayV3CertifiedIngestRecoversLostRepliesAndHistoricalCertification(t *testing.T) {
	fixture := newCertifiedClaimFixtureV3(t, "run-claim-v3-recovery")
	fixture.evidence.loseNext = true
	fixture.associations.LoseNextReply()
	request := ports.RunClaimIngestRequestV2{
		ExpectedRunRevision: fixture.run.Revision,
		Candidate:           claimCandidateV2(t, fixture.run, 1, "event-claim-v3-recovery", core.RunClaimCompleted),
	}
	result, err := fixture.gateway.IngestRunClaimV3(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil || result.Certification != fixture.bundle.Certification {
		t.Fatalf("certified claim result did not preserve the exact persisted association: %v %+v", err, result)
	}
	forged := result
	forgedDigest := claimDigestV2(t, "another-valid-scope-digest")
	forged.Certification.ExecutionScopeDigest = forgedDigest
	forged.Plan.ExecutionScopeDigest = forgedDigest
	if err := forged.Validate(); !core.HasReason(err, core.ReasonRunClaimUnverified) {
		t.Fatalf("result cannot self-authorize by making Plan and certification share a forged scope digest: %v", err)
	}
	if fixture.evidence.count() != 1 {
		t.Fatalf("lost replies must recover one ledger record, got %d", fixture.evidence.count())
	}
	inspected, err := fixture.gateway.InspectRunClaimV3(context.Background(), fixture.run.Scope, fixture.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := inspected.Validate(); err != nil || inspected.Certification != result.Certification || inspected.Plan != result.Plan || inspected.Evidence.Ref != result.Evidence.Ref || inspected.Association.ID != result.Association.ID {
		t.Fatalf("restart Inspect did not recover the exact certified claim chain: %v %+v", err, inspected)
	}

	// Expiry is a dispatch-currentness concern. An already-persisted bundle may
	// still ingest its terminal Claim using the immutable historical fact.
	expiredRelativeToRecovery := fixture.certification
	expiredRelativeToRecovery.ExpiresUnixNano = fixture.now.Add(-time.Second).UnixNano()
	expiredRelativeToRecovery, err = ports.SealRunSettlementPlanCertificationFactV3(expiredRelativeToRecovery)
	if err != nil {
		t.Fatal(err)
	}
	expiredRef, _ := expiredRelativeToRecovery.RefV3()
	expiredAssociation, _ := ports.NewRunSettlementPlanCertificationAssociationV3(fixture.run, fixture.plan, expiredRef)
	expiredBundle := fixture.bundle
	expiredBundle.Certification = expiredAssociation
	fixture.gateway.Bundles = &staticClaimBundlePortV3{bundle: expiredBundle}
	fixture.gateway.PlanAdmissions = staticClaimPlanAdmissionV3{fact: expiredRelativeToRecovery}
	second := request
	second.Candidate = claimCandidateV2(t, fixture.run, 2, "event-claim-v3-historical", core.RunClaimCompleted)
	// The first association remains authoritative, so a different terminal
	// claim is preserved as Evidence but cannot overwrite it.
	if _, err := fixture.gateway.IngestRunClaimV3(context.Background(), second); !core.HasReason(err, core.ReasonRunClaimConflict) {
		t.Fatalf("historical certification should permit ingest but preserve create-once association: %v", err)
	}
	if fixture.evidence.count() != 2 {
		t.Fatalf("historically certified late terminal evidence was not preserved: %d", fixture.evidence.count())
	}
}

func TestRunClaimGatewayV3ConcurrentReplayCreatesOneLogicalEvidenceAndAssociation(t *testing.T) {
	fixture := newCertifiedClaimFixtureV3(t, "run-claim-v3-concurrent")
	request := ports.RunClaimIngestRequestV2{
		ExpectedRunRevision: fixture.run.Revision,
		Candidate:           claimCandidateV2(t, fixture.run, 1, "event-claim-v3-concurrent", core.RunClaimCompleted),
	}
	const callers = 64
	results := make(chan ports.RunClaimIngestResultV3, callers)
	errors := make(chan error, callers)
	var wait sync.WaitGroup
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := fixture.gateway.IngestRunClaimV3(context.Background(), request)
			if err != nil {
				errors <- err
				return
			}
			results <- result
		}()
	}
	wait.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatalf("same-content concurrent replay failed: %v", err)
	}
	var first *ports.RunClaimIngestResultV3
	count := 0
	for result := range results {
		copy := result
		if first == nil {
			first = &copy
		} else if result.Association.ID != first.Association.ID || result.Evidence.Ref != first.Evidence.Ref || result.Certification != first.Certification {
			t.Fatal("concurrent replay returned different authoritative facts")
		}
		count++
	}
	if count != callers || fixture.evidence.count() != 1 {
		t.Fatalf("concurrent replay did not linearize one logical claim: results=%d evidence=%d", count, fixture.evidence.count())
	}
}

func TestRunClaimGatewayV3InspectValidatesInputBeforeBackend(t *testing.T) {
	bundles := &staticClaimBundlePortV3{}
	gateway := kernel.RunClaimGatewayV3{Bundles: bundles}
	if _, err := gateway.InspectRunClaimV3(context.Background(), core.ExecutionScope{}, ""); err == nil {
		t.Fatal("invalid inspect input was accepted")
	}
	if bundles.inspectCalls.Load() != 0 {
		t.Fatal("invalid public inspect input reached the backend")
	}
}

func claimPlanV3(t *testing.T, run core.AgentRunRecord, now time.Time) ports.RunSettlementPlanFactV2 {
	t.Helper()
	identity, _ := ports.RunIdentityDigestV2(run)
	scopeDigest, _ := ports.ExecutionScopeDigestV2(run.Scope)
	owner := ports.EvidenceProducerBindingRefV2{
		BindingSetID:       "binding-set-claim-v3",
		BindingSetRevision: 1,
		ComponentID:        "runtime/claim-owner",
		ManifestDigest:     claimDigestV2(t, "claim-v3-manifest"),
		ArtifactDigest:     claimDigestV2(t, "claim-v3-artifact"),
		Capability:         "runtime/settle-run",
	}
	execution := ports.RunExecutionSubjectV2{
		EndpointID:     "endpoint-claim-v3",
		EndpointDigest: claimDigestV2(t, "claim-v3-endpoint"),
		SessionRef:     run.SessionRef,
		Binding:        owner,
	}
	execution.SubjectDigest, _ = execution.DigestV2()
	schema := ports.SchemaRefV2{Namespace: "runtime", Name: "claim-settlement", Version: "2.0.0", MediaType: "application/octet-stream", ContentDigest: claimDigestV2(t, "claim-v3-schema")}
	reserved := []struct {
		kind  ports.NamespacedNameV2
		phase ports.RunSettlementRequirementPhaseV2
	}{
		{ports.RunRequirementExecutionTruth, ports.RunSettlementPhaseCompletion},
		{ports.RunRequirementEffects, ports.RunSettlementPhaseCompletion},
		{ports.RunRequirementRemoteContinuations, ports.RunSettlementPhaseCompletion},
		{ports.RunRequirementDomainCommits, ports.RunSettlementPhaseCompletion},
		{ports.RunRequirementBudget, ports.RunSettlementPhaseCompletion},
		{ports.RunRequirementCleanup, ports.RunSettlementPhaseTerminationReport},
		{ports.RunRequirementResidual, ports.RunSettlementPhaseTerminationReport},
		{ports.RunRequirementProviderRetention, ports.RunSettlementPhaseTerminationReport},
	}
	requirements := make([]ports.RunSettlementRequirementV2, 0, len(reserved))
	for _, item := range reserved {
		subject := claimDigestV2(t, "claim-v3-subject-"+string(item.kind))
		if item.kind == ports.RunRequirementExecutionTruth {
			subject = execution.SubjectDigest
		}
		requirements = append(requirements, ports.RunSettlementRequirementV2{
			ID: item.kind, Kind: item.kind, Phase: item.phase, Owner: owner, Schema: schema,
			SubjectSelector: "runtime/run", SubjectDigest: subject,
			Policy:        ports.RunSettlementPolicyBindingRefV2{Ref: "policy-" + string(item.kind[8:]), Revision: 1, Digest: claimDigestV2(t, "policy-"+string(item.kind)), SemanticDigest: claimDigestV2(t, "policy-semantic-"+string(item.kind))},
			EvidenceTrust: ports.EvidenceTrustAttestation, EvidenceKind: "runtime/settlement-attestation",
		})
	}
	ports.SortRunSettlementRequirementsV2(requirements)
	return ports.RunSettlementPlanFactV2{
		ContractVersion: ports.RunSettlementContractVersionV2,
		ID:              "claim-plan-v3-" + string(run.ID), Revision: 1, RunID: run.ID,
		RunIdentityDigest: identity, ExecutionScope: run.Scope, ExecutionScopeDigest: scopeDigest,
		SessionRef: run.SessionRef, LineagePlanDigest: run.Scope.Lineage.PlanDigest,
		BindingSet:   ports.RunBindingSetRefV2{ID: owner.BindingSetID, Revision: 1, Digest: claimDigestV2(t, "claim-v3-set"), SemanticDigest: claimDigestV2(t, "claim-v3-set-semantic")},
		Execution:    execution,
		Claim:        ports.RunClaimRequirementV2{Mode: ports.RunClaimOptionalByPolicyV2, OmissionPolicy: &ports.RunSettlementPolicyBindingRefV2{Ref: "claim-omission", Revision: 1, Digest: claimDigestV2(t, "claim-omission"), SemanticDigest: claimDigestV2(t, "claim-omission-semantic")}},
		Requirements: requirements, CreatedUnixNano: now.UnixNano(),
	}
}

func claimCertificationV3(t *testing.T, run core.AgentRunRecord, plan ports.RunSettlementPlanFactV2, now time.Time) ports.RunSettlementPlanCertificationFactV3 {
	t.Helper()
	planRef, _ := plan.RefV2()
	declaration := ports.RunSettlementDeclarationRefV3{
		ID: "claim-declaration-v3", Revision: 1, Digest: claimDigestV2(t, "claim-declaration-v3"),
		BindingSetID: plan.BindingSet.ID, BindingSetRevision: plan.BindingSet.Revision, BindingRevision: 1,
		ComponentID: plan.Execution.Binding.ComponentID,
	}
	fact, err := ports.SealRunSettlementPlanCertificationFactV3(ports.RunSettlementPlanCertificationFactV3{
		ContractVersion: ports.RunSettlementPlanAdmissionContractVersionV3,
		ID:              "claim-certification-v3-" + string(run.ID), Revision: 1, RunID: run.ID,
		RunIdentityDigest: plan.RunIdentityDigest, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest,
		Plan: planRef, BindingSet: plan.BindingSet,
		BaselinePolicy: ports.RunSettlementBaselinePolicyRefV3{ID: "claim-baseline-v3", Revision: 1, Digest: claimDigestV2(t, "claim-baseline-v3")},
		Declarations:   []ports.RunSettlementDeclarationRefV3{declaration}, CertificationOwner: plan.Execution.Binding,
		CreatedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}
