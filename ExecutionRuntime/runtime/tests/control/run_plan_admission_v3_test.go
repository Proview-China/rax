package control_test

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type bindingFactsV3 struct {
	control.BindingFactPortV2
	set   control.BindingSetFactV2
	facts map[string]control.BindingFactV2
}

func (b *bindingFactsV3) InspectBinding(_ context.Context, id string) (control.BindingFactV2, error) {
	fact, ok := b.facts[id]
	if !ok {
		return control.BindingFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonBindingDrift, "Binding Fact missing")
	}
	return fact, nil
}

func (b *bindingFactsV3) InspectBindingSet(_ context.Context, id string) (control.BindingSetFactV2, error) {
	if id != b.set.ID {
		return control.BindingSetFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonBindingDrift, "BindingSet missing")
	}
	return b.set, nil
}

type planAdmissionFixtureV3 struct {
	now         time.Time
	run         core.AgentRunRecord
	plan        ports.RunSettlementPlanFactV2
	owner       ports.EvidenceProducerBindingRefV2
	baseline    ports.RunSettlementBaselinePolicyFactV3
	declaration ports.RunSettlementDeclarationFactV3
	bindings    *bindingFactsV3
	store       *fakes.RunPlanAdmissionStoreV3
	gateway     control.RunSettlementPlanAdmissionGatewayV3
}

func newPlanAdmissionFixtureV3(t *testing.T) *planAdmissionFixtureV3 {
	t.Helper()
	now := time.Unix(700_000, 0)
	manifest, catalog := controlBindingFixture(t, "runtime/host-assembler", "runtime/host-assembler-kind", nil, nil)
	manifest.ProvidedCapabilities = []ports.ProvidedCapabilityV2{{Capability: control.RunSettlementPlanCertifyCapabilityV3, TTLSeconds: 300, Schemas: []ports.SchemaRefV2{}}, {Capability: "runtime/settle-run", TTLSeconds: 300, Schemas: []ports.SchemaRefV2{}}}
	catalog.Registrations[0].Capabilities = []ports.CapabilityNameV2{control.RunSettlementPlanCertifyCapabilityV3, "runtime/settle-run"}
	governance, _ := catalog.DigestV2()
	declared := declaredBindingWithDigestV2(t, "binding-host-assembler", manifest, governance)
	probed := probedBindingV2(t, declared, now)
	certified := certifiedBindingV2(t, probed, now.Add(time.Second))
	planBinding := sealedBindingPlanV2(t, ports.BindingPlanV2{ID: "binding-plan-run-admission", GovernanceDigest: governance, Requirements: []ports.BindingRequirementV2{bindingRequirementV2(manifest)}})
	set, err := control.BuildBindingSetV2("binding-set-run-admission", planBinding, catalog, []control.BindingFactV2{certified}, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	bound := certified
	bound.State, bound.Revision, bound.BindingSetID = control.BindingBound, certified.Revision+1, set.ID
	set.Members[0].BindingRevision = bound.Revision
	set.ExpiresUnixNano = bound.ExpiresUnixNano
	setDigest, _ := control.BindingSetDigestV2(set)
	setSemantic, _ := control.BindingSetSemanticDigestV2(set)
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-plan-admission", ID: "identity-plan-admission", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-plan-admission", PlanDigest: controlDigestV2(t, "lineage")}, Instance: core.InstanceRef{ID: "instance-plan-admission", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "lease-plan-admission", Epoch: 1}, AuthorityEpoch: 1}
	session, err := ports.DeriveRuntimeExecutionSessionRefV2("endpoint-plan-admission", "run-plan-admission")
	if err != nil {
		t.Fatal(err)
	}
	run := core.AgentRunRecord{ID: "run-plan-admission", Scope: scope, Status: core.RunPending, Revision: 1, SessionRef: session}
	runIdentity, _ := ports.RunIdentityDigestV2(run)
	scopeDigest, _ := ports.ExecutionScopeDigestV2(scope)
	owner := ports.EvidenceProducerBindingRefV2{BindingSetID: set.ID, BindingSetRevision: set.Revision, ComponentID: manifest.ComponentID, ManifestDigest: bound.ManifestDigest, ArtifactDigest: manifest.ArtifactDigest, Capability: control.RunSettlementPlanCertifyCapabilityV3}
	settlementOwner := owner
	settlementOwner.Capability = "runtime/settle-run"
	execution := ports.RunExecutionSubjectV2{EndpointID: "endpoint-plan-admission", EndpointDigest: controlDigestV2(t, "endpoint"), SessionRef: run.SessionRef, Binding: settlementOwner}
	execution.SubjectDigest, _ = execution.DigestV2()
	schema := ports.SchemaRefV2{Namespace: "runtime", Name: "settlement", Version: "2.0.0", MediaType: "application/octet-stream", ContentDigest: controlDigestV2(t, "settlement-schema")}
	kinds := []struct {
		kind  ports.NamespacedNameV2
		phase ports.RunSettlementRequirementPhaseV2
	}{{ports.RunRequirementExecutionTruth, ports.RunSettlementPhaseCompletion}, {ports.RunRequirementEffects, ports.RunSettlementPhaseCompletion}, {ports.RunRequirementRemoteContinuations, ports.RunSettlementPhaseCompletion}, {ports.RunRequirementDomainCommits, ports.RunSettlementPhaseCompletion}, {ports.RunRequirementBudget, ports.RunSettlementPhaseCompletion}, {ports.RunRequirementCleanup, ports.RunSettlementPhaseTerminationReport}, {ports.RunRequirementResidual, ports.RunSettlementPhaseTerminationReport}, {ports.RunRequirementProviderRetention, ports.RunSettlementPhaseTerminationReport}}
	requirements := make([]ports.RunSettlementRequirementV2, 0, len(kinds))
	for _, item := range kinds {
		subject := controlDigestV2(t, "subject-"+string(item.kind))
		if item.kind == ports.RunRequirementExecutionTruth {
			subject = execution.SubjectDigest
		}
		requirements = append(requirements, ports.RunSettlementRequirementV2{ID: item.kind, Kind: item.kind, Phase: item.phase, Owner: settlementOwner, Schema: schema, SubjectSelector: "runtime/run", SubjectDigest: subject, Policy: ports.RunSettlementPolicyBindingRefV2{Ref: "policy-" + string(item.kind[8:]), Revision: 1, Digest: controlDigestV2(t, "policy-"+string(item.kind)), SemanticDigest: controlDigestV2(t, "policy-semantic-"+string(item.kind))}, EvidenceTrust: ports.EvidenceTrustAttestation, EvidenceKind: "runtime/settlement-attestation"})
	}
	ports.SortRunSettlementRequirementsV2(requirements)
	plan := ports.RunSettlementPlanFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: "plan-run-admission", Revision: 1, RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, SessionRef: run.SessionRef, LineagePlanDigest: scope.Lineage.PlanDigest, BindingSet: ports.RunBindingSetRefV2{ID: set.ID, Revision: set.Revision, Digest: setDigest, SemanticDigest: setSemantic}, Execution: execution, Claim: ports.RunClaimRequirementV2{Mode: ports.RunClaimRequiredV2}, Requirements: requirements, CreatedUnixNano: now.UnixNano()}
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	baseline, err := ports.SealRunSettlementBaselinePolicyFactV3(ports.RunSettlementBaselinePolicyFactV3{ContractVersion: ports.RunSettlementPlanAdmissionContractVersionV3, ID: "baseline-run-admission", Revision: 1, RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScopeDigest: scopeDigest, Requirements: append([]ports.RunSettlementRequirementV2{}, requirements...), PolicyOwner: owner, ExpiresUnixNano: now.Add(4 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	declaration, err := ports.SealRunSettlementDeclarationFactV3(ports.RunSettlementDeclarationFactV3{ContractVersion: ports.RunSettlementPlanAdmissionContractVersionV3, ID: "declaration-run-admission", Revision: 1, BindingSetID: set.ID, BindingSetRevision: set.Revision, BindingRevision: bound.Revision, ComponentID: manifest.ComponentID, BindingID: bound.ID, ManifestDigest: bound.ManifestDigest, ArtifactDigest: manifest.ArtifactDigest, Requirements: []ports.RunSettlementRequirementV2{}, ExpiresUnixNano: now.Add(3 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	store := fakes.NewRunPlanAdmissionStoreV3(func() time.Time { return now })
	if _, err := store.CreateRunSettlementBaselinePolicyV3(context.Background(), baseline); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateRunSettlementDeclarationV3(context.Background(), declaration); err != nil {
		t.Fatal(err)
	}
	bindings := &bindingFactsV3{set: set, facts: map[string]control.BindingFactV2{bound.ID: bound}}
	gateway := control.RunSettlementPlanAdmissionGatewayV3{Bindings: bindings, Declarations: store, Baselines: store, Certifications: store, Clock: func() time.Time { return now }}
	return &planAdmissionFixtureV3{now: now, run: run, plan: plan, owner: owner, baseline: baseline, declaration: declaration, bindings: bindings, store: store, gateway: gateway}
}

func (f *planAdmissionFixtureV3) request() ports.CertifyRunSettlementPlanRequestV3 {
	baseline, _ := f.baseline.RefV3()
	return ports.CertifyRunSettlementPlanRequestV3{CertificationID: "certification-run-admission", Run: f.run, Plan: f.plan, BaselinePolicy: baseline, Owner: f.owner, TTL: 5 * time.Minute}
}

func addSecondPlanAdmissionMemberV3(t *testing.T, f *planAdmissionFixtureV3, capabilities []ports.ProvidedCapabilityV2, includeDeclaration bool, includeCustomRequirement bool) (control.BindingFactV2, ports.RunSettlementDeclarationFactV3) {
	t.Helper()
	manifest, _ := controlBindingFixture(t, "custom/second-settlement", "custom/settlement-provider", nil, nil)
	manifest.ProvidedCapabilities = append([]ports.ProvidedCapabilityV2{}, capabilities...)
	declared := declaredBindingWithDigestV2(t, "binding-second-settlement", manifest, f.bindings.set.GovernanceDigest)
	probed := probedBindingV2(t, declared, f.now)
	certified := certifiedBindingV2(t, probed, f.now.Add(time.Second))
	bound := certified
	bound.State = control.BindingBound
	bound.Revision = certified.Revision + 1
	bound.BindingSetID = f.bindings.set.ID
	member := control.BindingMemberV2{
		BindingID:       bound.ID,
		BindingRevision: bound.Revision,
		ComponentID:     bound.ComponentID,
		Kind:            bound.Manifest.Kind,
		ManifestDigest:  bound.ManifestDigest,
		ArtifactDigest:  bound.Manifest.ArtifactDigest,
		Contract:        bound.Manifest.Contract,
		Owners:          append([]ports.OwnerAssignmentV2{}, bound.Manifest.Owners...),
		Grants:          append([]ports.CapabilityGrantV2{}, bound.Grants...),
	}
	f.bindings.set.Members = append(f.bindings.set.Members, member)
	sort.Slice(f.bindings.set.Members, func(i, j int) bool {
		return f.bindings.set.Members[i].ComponentID < f.bindings.set.Members[j].ComponentID
	})
	f.bindings.set.TopologicalOrder = append(f.bindings.set.TopologicalOrder, member.ComponentID)
	sort.Slice(f.bindings.set.TopologicalOrder, func(i, j int) bool { return f.bindings.set.TopologicalOrder[i] < f.bindings.set.TopologicalOrder[j] })
	for _, grant := range member.Grants {
		if grant.ExpiresUnixNano < f.bindings.set.ExpiresUnixNano {
			f.bindings.set.ExpiresUnixNano = grant.ExpiresUnixNano
		}
	}
	if err := f.bindings.set.Validate(); err != nil {
		t.Fatal(err)
	}
	f.bindings.facts[bound.ID] = bound
	setDigest, _ := control.BindingSetDigestV2(f.bindings.set)
	setSemantic, _ := control.BindingSetSemanticDigestV2(f.bindings.set)
	f.plan.BindingSet.Digest = setDigest
	f.plan.BindingSet.SemanticDigest = setSemantic

	owner := ports.EvidenceProducerBindingRefV2{
		BindingSetID:       f.bindings.set.ID,
		BindingSetRevision: f.bindings.set.Revision,
		ComponentID:        member.ComponentID,
		ManifestDigest:     member.ManifestDigest,
		ArtifactDigest:     member.ArtifactDigest,
		Capability:         "runtime/settle-run",
	}
	requirements := []ports.RunSettlementRequirementV2{}
	if includeCustomRequirement {
		requirement := ports.RunSettlementRequirementV2{
			ID: "custom/second-domain-commit", Kind: "custom/domain-commit", Phase: ports.RunSettlementPhaseCompletion,
			Owner:           owner,
			Schema:          ports.SchemaRefV2{Namespace: "custom", Name: "second-settlement", Version: "2.0.0", MediaType: "application/octet-stream", ContentDigest: controlDigestV2(t, "custom-second-schema")},
			SubjectSelector: "custom/run-subject", SubjectDigest: controlDigestV2(t, "custom-second-subject"),
			Policy:        ports.RunSettlementPolicyBindingRefV2{Ref: "custom-second-policy", Revision: 1, Digest: controlDigestV2(t, "custom-second-policy"), SemanticDigest: controlDigestV2(t, "custom-second-policy-semantic")},
			EvidenceTrust: ports.EvidenceTrustAttestation, EvidenceKind: "custom/settlement-attestation",
		}
		requirements = append(requirements, requirement)
		f.plan.Requirements = append(f.plan.Requirements, requirement)
		ports.SortRunSettlementRequirementsV2(f.plan.Requirements)
	}
	declaration, err := ports.SealRunSettlementDeclarationFactV3(ports.RunSettlementDeclarationFactV3{
		ContractVersion: ports.RunSettlementPlanAdmissionContractVersionV3,
		ID:              "declaration-second-settlement", Revision: 1,
		BindingSetID: f.bindings.set.ID, BindingSetRevision: f.bindings.set.Revision, BindingRevision: bound.Revision,
		ComponentID: member.ComponentID, BindingID: bound.ID, ManifestDigest: bound.ManifestDigest, ArtifactDigest: bound.Manifest.ArtifactDigest,
		Requirements: requirements, ExpiresUnixNano: f.now.Add(2 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if includeDeclaration {
		if _, err := f.store.CreateRunSettlementDeclarationV3(context.Background(), declaration); err != nil {
			t.Fatal(err)
		}
	}
	return bound, declaration
}

func TestRunSettlementPlanAdmissionV3ExactAggregateExplicitEmptyAndLostReply(t *testing.T) {
	fixture := newPlanAdmissionFixtureV3(t)
	fixture.store.LoseNextCertificationReply()
	fact, err := fixture.gateway.CertifyRunSettlementPlanV3(context.Background(), fixture.request())
	if err != nil {
		t.Fatal(err)
	}
	if len(fact.Declarations) != 1 || fixture.declaration.Requirements == nil || len(fixture.declaration.Requirements) != 0 {
		t.Fatal("every Binding member must contribute an explicit declaration, including an explicit empty set")
	}
	if fact.ExpiresUnixNano != fixture.declaration.ExpiresUnixNano {
		t.Fatalf("certification TTL did not take the minimum declaration watermark: got %d want %d", fact.ExpiresUnixNano, fixture.declaration.ExpiresUnixNano)
	}
	ref, _ := fact.RefV3()
	if err := fixture.gateway.ValidateRunSettlementPlanCertificationV3(context.Background(), ref, fixture.run, fixture.plan); err != nil {
		t.Fatal(err)
	}
}

func TestRunSettlementPlanAdmissionV3RejectsMissingBaselineOwnerAndBindingDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*planAdmissionFixtureV3, *ports.CertifyRunSettlementPlanRequestV3)
	}{
		{"wrong owner capability", func(_ *planAdmissionFixtureV3, request *ports.CertifyRunSettlementPlanRequestV3) {
			request.Owner.Capability = "custom/foo"
		}},
		{"baseline misses reserved", func(f *planAdmissionFixtureV3, request *ports.CertifyRunSettlementPlanRequestV3) {
			baseline := f.baseline
			baseline.Requirements = append([]ports.RunSettlementRequirementV2{}, baseline.Requirements[1:]...)
			baseline, _ = ports.SealRunSettlementBaselinePolicyFactV3(baseline)
			store := fakes.NewRunPlanAdmissionStoreV3(func() time.Time { return f.now })
			store.CreateRunSettlementBaselinePolicyV3(context.Background(), baseline)
			store.CreateRunSettlementDeclarationV3(context.Background(), f.declaration)
			f.gateway.Baselines, f.gateway.Declarations, f.gateway.Certifications = store, store, store
			request.BaselinePolicy, _ = baseline.RefV3()
		}},
		{"member revoked", func(f *planAdmissionFixtureV3, _ *ports.CertifyRunSettlementPlanRequestV3) {
			fact := f.bindings.facts[f.declaration.BindingID]
			fact.State = control.BindingRevoked
			fact.InvalidationReason = core.ReasonBindingDrift
			f.bindings.facts[fact.ID] = fact
		}},
		{"member grant drift", func(f *planAdmissionFixtureV3, _ *ports.CertifyRunSettlementPlanRequestV3) {
			fact := f.bindings.facts[f.declaration.BindingID]
			fact.Grants = append([]ports.CapabilityGrantV2{}, fact.Grants...)
			fact.Grants[0].EvidenceDigest = controlDigestV2(t, "drift")
			f.bindings.facts[fact.ID] = fact
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPlanAdmissionFixtureV3(t)
			request := fixture.request()
			test.mutate(fixture, &request)
			if _, err := fixture.gateway.CertifyRunSettlementPlanV3(context.Background(), request); err == nil {
				t.Fatal("invalid Plan certification input was accepted")
			}
		})
	}
}

func TestRunSettlementPlanAdmissionV3ConflictingCreateLinearizesOnce(t *testing.T) {
	fixture := newPlanAdmissionFixtureV3(t)
	request := fixture.request()
	var successes int
	var mu sync.Mutex
	var wait sync.WaitGroup
	for i := 0; i < 32; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := fixture.gateway.CertifyRunSettlementPlanV3(context.Background(), request); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wait.Wait()
	if successes != 32 {
		t.Fatalf("same certification replay must be idempotent for every caller, successes=%d", successes)
	}
	stored, err := fixture.store.InspectRunSettlementPlanCertificationV3(context.Background(), fixture.run.Scope, fixture.run.ID)
	if err != nil || stored.Revision != 1 {
		t.Fatalf("certification did not linearize exactly once: %#v %v", stored, err)
	}
}

func TestRunSettlementPlanAdmissionV3RequiresEveryMemberDeclarationAndExactCustomAggregate(t *testing.T) {
	capabilities := []ports.ProvidedCapabilityV2{{Capability: "runtime/settle-run", TTLSeconds: 300, Schemas: []ports.SchemaRefV2{}}}

	t.Run("missing second member declaration", func(t *testing.T) {
		fixture := newPlanAdmissionFixtureV3(t)
		addSecondPlanAdmissionMemberV3(t, fixture, capabilities, false, false)
		if _, err := fixture.gateway.CertifyRunSettlementPlanV3(context.Background(), fixture.request()); err == nil {
			t.Fatal("Binding member without an explicit declaration was certified")
		}
	})

	t.Run("custom requirement exact aggregate", func(t *testing.T) {
		fixture := newPlanAdmissionFixtureV3(t)
		_, declaration := addSecondPlanAdmissionMemberV3(t, fixture, capabilities, true, true)
		fact, err := fixture.gateway.CertifyRunSettlementPlanV3(context.Background(), fixture.request())
		if err != nil {
			t.Fatal(err)
		}
		if len(fact.Declarations) != 2 || len(declaration.Requirements) != 1 {
			t.Fatalf("two-member custom declaration was not frozen exactly: %#v", fact.Declarations)
		}
	})

	for name, mutate := range map[string]func(*planAdmissionFixtureV3, *ports.RunSettlementDeclarationFactV3){
		"plan deletes declared requirement": func(f *planAdmissionFixtureV3, _ *ports.RunSettlementDeclarationFactV3) {
			f.plan.Requirements = f.plan.Requirements[:len(f.plan.Requirements)-1]
		},
		"plan replaces declared policy": func(f *planAdmissionFixtureV3, _ *ports.RunSettlementDeclarationFactV3) {
			for index := range f.plan.Requirements {
				if f.plan.Requirements[index].ID == "custom/second-domain-commit" {
					f.plan.Requirements[index].Policy.Digest = controlDigestV2(t, "changed-policy")
				}
			}
		},
		"declaration claims another owner": func(f *planAdmissionFixtureV3, declaration *ports.RunSettlementDeclarationFactV3) {
			declaration.Requirements[0].Owner = f.plan.Execution.Binding
			*declaration, _ = ports.SealRunSettlementDeclarationFactV3(*declaration)
			store := fakes.NewRunPlanAdmissionStoreV3(func() time.Time { return f.now })
			store.CreateRunSettlementBaselinePolicyV3(context.Background(), f.baseline)
			store.CreateRunSettlementDeclarationV3(context.Background(), f.declaration)
			store.CreateRunSettlementDeclarationV3(context.Background(), *declaration)
			f.gateway.Declarations, f.gateway.Baselines, f.gateway.Certifications = store, store, store
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newPlanAdmissionFixtureV3(t)
			_, declaration := addSecondPlanAdmissionMemberV3(t, fixture, capabilities, true, true)
			mutate(fixture, &declaration)
			if _, err := fixture.gateway.CertifyRunSettlementPlanV3(context.Background(), fixture.request()); err == nil {
				t.Fatal("non-exact custom requirement aggregate was certified")
			}
		})
	}
}

func TestRunSettlementPlanAdmissionV3RejectsSecondHostCertifier(t *testing.T) {
	fixture := newPlanAdmissionFixtureV3(t)
	capabilities := []ports.ProvidedCapabilityV2{
		{Capability: control.RunSettlementPlanCertifyCapabilityV3, TTLSeconds: 300, Schemas: []ports.SchemaRefV2{}},
		{Capability: "runtime/settle-run", TTLSeconds: 300, Schemas: []ports.SchemaRefV2{}},
	}
	addSecondPlanAdmissionMemberV3(t, fixture, capabilities, true, false)
	if _, err := fixture.gateway.CertifyRunSettlementPlanV3(context.Background(), fixture.request()); !core.HasReason(err, core.ReasonOwnerMissing) {
		t.Fatalf("second current Plan certifier was not rejected: %v", err)
	}
}

func TestRunSettlementPlanAdmissionV3CertificationTTLUsesEveryCurrentWatermark(t *testing.T) {
	tests := []struct {
		name   string
		cut    time.Duration
		mutate func(*testing.T, *planAdmissionFixtureV3, time.Duration)
	}{
		{
			name: "baseline",
			cut:  45 * time.Second,
			mutate: func(t *testing.T, f *planAdmissionFixtureV3, cut time.Duration) {
				baseline := f.baseline
				baseline.ExpiresUnixNano = f.now.Add(cut).UnixNano()
				baseline, _ = ports.SealRunSettlementBaselinePolicyFactV3(baseline)
				store := fakes.NewRunPlanAdmissionStoreV3(func() time.Time { return f.now })
				store.CreateRunSettlementBaselinePolicyV3(context.Background(), baseline)
				store.CreateRunSettlementDeclarationV3(context.Background(), f.declaration)
				f.baseline = baseline
				f.gateway.Declarations, f.gateway.Baselines, f.gateway.Certifications = store, store, store
			},
		},
		{
			name: "certification owner grant",
			cut:  40 * time.Second,
			mutate: func(_ *testing.T, f *planAdmissionFixtureV3, cut time.Duration) {
				setPlanAdmissionGrantExpiryV3(f, control.RunSettlementPlanCertifyCapabilityV3, f.now.Add(cut).UnixNano())
			},
		},
		{
			name: "requirement owner grant",
			cut:  35 * time.Second,
			mutate: func(_ *testing.T, f *planAdmissionFixtureV3, cut time.Duration) {
				setPlanAdmissionGrantExpiryV3(f, "runtime/settle-run", f.now.Add(cut).UnixNano())
			},
		},
		{
			name: "non-target member fact",
			cut:  30 * time.Second,
			mutate: func(t *testing.T, f *planAdmissionFixtureV3, cut time.Duration) {
				bound, _ := addSecondPlanAdmissionMemberV3(t, f, []ports.ProvidedCapabilityV2{{Capability: "runtime/settle-run", TTLSeconds: 300, Schemas: []ports.SchemaRefV2{}}}, true, false)
				setSpecificPlanAdmissionFactExpiryV3(f, bound.ID, f.now.Add(cut).UnixNano())
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPlanAdmissionFixtureV3(t)
			test.mutate(t, fixture, test.cut)
			fact, err := fixture.gateway.CertifyRunSettlementPlanV3(context.Background(), fixture.request())
			if err != nil {
				t.Fatal(err)
			}
			want := fixture.now.Add(test.cut).UnixNano()
			if fact.ExpiresUnixNano != want {
				t.Fatalf("certification TTL ignored %s watermark: got=%d want=%d", test.name, fact.ExpiresUnixNano, want)
			}
		})
	}
}

func TestRunSettlementPlanAdmissionV3DifferentCertificationContentConflicts(t *testing.T) {
	fixture := newPlanAdmissionFixtureV3(t)
	first := fixture.request()
	second := fixture.request()
	second.CertificationID = "certification-run-admission-conflict"
	start := make(chan struct{})
	errors := make(chan error, 2)
	for _, request := range []ports.CertifyRunSettlementPlanRequestV3{first, second} {
		request := request
		go func() {
			<-start
			_, err := fixture.gateway.CertifyRunSettlementPlanV3(context.Background(), request)
			errors <- err
		}()
	}
	close(start)
	var success, conflict int
	for range 2 {
		err := <-errors
		if err == nil {
			success++
		} else if core.HasCategory(err, core.ErrorConflict) {
			conflict++
		} else {
			t.Fatalf("unexpected conflicting certification result: %v", err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("different certification contents did not linearize once: success=%d conflict=%d", success, conflict)
	}
}

func setPlanAdmissionGrantExpiryV3(f *planAdmissionFixtureV3, capability ports.CapabilityNameV2, expiry int64) {
	for memberIndex := range f.bindings.set.Members {
		member := &f.bindings.set.Members[memberIndex]
		fact := f.bindings.facts[member.BindingID]
		for grantIndex := range member.Grants {
			if member.Grants[grantIndex].Capability == capability {
				member.Grants[grantIndex].ExpiresUnixNano = expiry
				fact.Grants[grantIndex].ExpiresUnixNano = expiry
			}
		}
		fact.ExpiresUnixNano = minimumGrantExpiryV3(fact.Grants)
		f.bindings.facts[fact.ID] = fact
	}
	f.bindings.set.ExpiresUnixNano = minimumSetGrantExpiryV3(f.bindings.set)
	refreshPlanAdmissionBindingRefV3(f)
}

func setSpecificPlanAdmissionFactExpiryV3(f *planAdmissionFixtureV3, bindingID string, expiry int64) {
	fact := f.bindings.facts[bindingID]
	for index := range fact.Grants {
		fact.Grants[index].ExpiresUnixNano = expiry
	}
	fact.ExpiresUnixNano = expiry
	f.bindings.facts[bindingID] = fact
	for memberIndex := range f.bindings.set.Members {
		if f.bindings.set.Members[memberIndex].BindingID == bindingID {
			f.bindings.set.Members[memberIndex].Grants = append([]ports.CapabilityGrantV2{}, fact.Grants...)
		}
	}
	f.bindings.set.ExpiresUnixNano = minimumSetGrantExpiryV3(f.bindings.set)
	refreshPlanAdmissionBindingRefV3(f)
}

func refreshPlanAdmissionBindingRefV3(f *planAdmissionFixtureV3) {
	digest, _ := control.BindingSetDigestV2(f.bindings.set)
	semantic, _ := control.BindingSetSemanticDigestV2(f.bindings.set)
	f.plan.BindingSet = ports.RunBindingSetRefV2{ID: f.bindings.set.ID, Revision: f.bindings.set.Revision, Digest: digest, SemanticDigest: semantic}
}

func minimumGrantExpiryV3(grants []ports.CapabilityGrantV2) int64 {
	minimum := grants[0].ExpiresUnixNano
	for _, grant := range grants[1:] {
		if grant.ExpiresUnixNano < minimum {
			minimum = grant.ExpiresUnixNano
		}
	}
	return minimum
}

func minimumSetGrantExpiryV3(set control.BindingSetFactV2) int64 {
	minimum := set.Members[0].Grants[0].ExpiresUnixNano
	for _, member := range set.Members {
		if current := minimumGrantExpiryV3(member.Grants); current < minimum {
			minimum = current
		}
	}
	return minimum
}

func TestProviderBindingCurrentnessV2RejectsDriftAndSealsProjection(t *testing.T) {
	fixture := newPlanAdmissionFixtureV3(t)
	expected := ports.ProviderBindingRefV2(fixture.owner)
	adapter := control.ProviderBindingCurrentnessAdapterV2{Bindings: fixture.bindings, Clock: func() time.Time { return fixture.now }}
	projection, err := adapter.InspectProviderBindingCurrentV2(context.Background(), expected)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ValidateCurrent(expected, fixture.now); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ports.ProviderBindingCurrentProjectionV2){
		"state": func(value *ports.ProviderBindingCurrentProjectionV2) {
			value.State = ports.ProviderBindingCurrentRevokedV2
		},
		"ttl": func(value *ports.ProviderBindingCurrentProjectionV2) { value.ExpiresUnixNano++ },
		"grant": func(value *ports.ProviderBindingCurrentProjectionV2) {
			value.GrantDigest = controlDigestV2(t, "other-grant")
		},
	} {
		forged := projection
		mutate(&forged)
		if err := forged.ValidateCurrent(expected, fixture.now); err == nil {
			t.Fatalf("projection accepted forged %s under the old self digest", name)
		}
	}
	fact := fixture.bindings.facts[fixture.declaration.BindingID]
	fact.Grants = append([]ports.CapabilityGrantV2{}, fact.Grants...)
	sort.Slice(fact.Grants, func(i, j int) bool { return fact.Grants[i].Capability < fact.Grants[j].Capability })
	fact.Grants[0].EvidenceDigest = controlDigestV2(t, "fact-grant-drift")
	fixture.bindings.facts[fact.ID] = fact
	if _, err := adapter.InspectProviderBindingCurrentV2(context.Background(), expected); err == nil {
		t.Fatal("BindingSet embedded grant was trusted over the exact Binding Fact")
	}
}

func TestProviderBindingCurrentnessV2FailsClosedOnNonTargetMemberDrift(t *testing.T) {
	capabilities := []ports.ProvidedCapabilityV2{{Capability: "runtime/settle-run", TTLSeconds: 300, Schemas: []ports.SchemaRefV2{}}}
	tests := []struct {
		name   string
		mutate func(*testing.T, *planAdmissionFixtureV3, control.BindingFactV2)
	}{
		{
			name: "non-target revoked",
			mutate: func(_ *testing.T, f *planAdmissionFixtureV3, second control.BindingFactV2) {
				second.State = control.BindingRevoked
				second.InvalidationReason = core.ReasonBindingDrift
				f.bindings.facts[second.ID] = second
			},
		},
		{
			name: "non-target grant drift",
			mutate: func(t *testing.T, f *planAdmissionFixtureV3, second control.BindingFactV2) {
				second.Grants = append([]ports.CapabilityGrantV2{}, second.Grants...)
				second.Grants[0].EvidenceDigest = controlDigestV2(t, "non-target-grant-drift")
				f.bindings.facts[second.ID] = second
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPlanAdmissionFixtureV3(t)
			second, _ := addSecondPlanAdmissionMemberV3(t, fixture, capabilities, true, false)
			test.mutate(t, fixture, second)
			adapter := control.ProviderBindingCurrentnessAdapterV2{Bindings: fixture.bindings, Clock: func() time.Time { return fixture.now }}
			if _, err := adapter.InspectProviderBindingCurrentV2(context.Background(), ports.ProviderBindingRefV2(fixture.owner)); err == nil {
				t.Fatalf("target provider remained current while %s", test.name)
			}
		})
	}
}
