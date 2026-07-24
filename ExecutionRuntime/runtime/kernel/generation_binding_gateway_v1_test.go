package kernel

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	runtimesqlite "github.com/Proview-China/rax/ExecutionRuntime/runtime/storage/sqlite"
)

func TestGenerationBindingAssociationGatewayV1HappyLostReplyAndConcurrentReplay(t *testing.T) {
	now := time.Unix(90_000, 0)
	fixture := generationBindingGatewayFixtureV1(t, now)
	fixture.store.LoseNextCreateReplyV1()
	fact, err := fixture.gateway.AssociateGenerationBindingV1(context.Background(), fixture.candidate)
	if err != nil {
		t.Fatal(err)
	}
	if fact.CandidateDigest != fixture.candidate.Digest || fixture.store.CreateCommitCountV1() != 1 {
		t.Fatalf("lost create reply did not recover exact Fact: %+v commits=%d", fact, fixture.store.CreateCommitCountV1())
	}

	const workers = 32
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			current, callErr := fixture.gateway.AssociateGenerationBindingV1(context.Background(), fixture.candidate)
			if callErr == nil && current.Digest != fact.Digest {
				callErr = core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "concurrent replay returned different Fact")
			}
			errs <- callErr
		}()
	}
	wg.Wait()
	close(errs)
	for callErr := range errs {
		if callErr != nil {
			t.Fatal(callErr)
		}
	}
	if fixture.store.CreateCommitCountV1() != 1 {
		t.Fatalf("same candidate linearized %d creates", fixture.store.CreateCommitCountV1())
	}

	conflict := fixture.candidate
	conflict.Generation.Generation.GraphDigest = generationBindingKernelDigestV1(t, "other-graph")
	conflict.Generation, _ = ports.SealGenerationCurrentProjectionV1(conflict.Generation)
	conflict, _ = ports.SealGenerationBindingAssociationCandidateV1(conflict)
	if _, err := fixture.gateway.AssociateGenerationBindingV1(context.Background(), conflict); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same ID changed generation must conflict: %v", err)
	}
}

func TestGenerationBindingAssociationGatewayV1SQLiteConcurrent64AndStrictCAS(t *testing.T) {
	now := time.Unix(90_500, 0)
	fixture := generationBindingGatewayFixtureV1(t, now)
	path := filepath.Join(t.TempDir(), "generation-binding.db")
	const workers = 64
	stores := make([]*runtimesqlite.Store, 0, workers)
	gateways := make([]GenerationBindingAssociationGatewayV1, 0, workers)
	var createWinners atomic.Int64
	for range workers {
		store, err := runtimesqlite.Open(context.Background(), runtimesqlite.Config{Path: path, BusyTimeout: 5 * time.Second, MaxOpenConns: 2, Clock: func() time.Time { return now.Add(time.Second) }})
		if err != nil {
			t.Fatal(err)
		}
		stores = append(stores, store)
		facts := &countingGenerationBindingFactPortV1{GenerationBindingAssociationFactPortV1: store, createWinners: &createWinners}
		gateway := fixture.gateway
		gateway.Facts = facts
		gateways = append(gateways, gateway)
	}
	t.Cleanup(func() {
		for _, store := range stores {
			_ = store.Close()
		}
	})
	var wait sync.WaitGroup
	wait.Add(workers)
	for _, gateway := range gateways {
		go func(g GenerationBindingAssociationGatewayV1) {
			defer wait.Done()
			fact, err := g.AssociateGenerationBindingV1(context.Background(), fixture.candidate)
			if err != nil || fact.CandidateDigest != fixture.candidate.Digest {
				t.Errorf("sqlite association failed: fact=%+v err=%v", fact, err)
			}
		}(gateway)
	}
	wait.Wait()
	if createWinners.Load() != 1 {
		t.Fatalf("sqlite association granted %d normal Create responses", createWinners.Load())
	}
	active, err := stores[0].InspectGenerationBindingAssociationV1(context.Background(), fixture.candidate.AssociationID)
	if err != nil || active.State != ports.GenerationBindingAssociationActiveV1 {
		t.Fatalf("sqlite current association is absent: %+v err=%v", active, err)
	}
	if _, err := stores[1].CreateGenerationBindingAssociationV1(context.Background(), active); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same-content Create replay did not require Inspect: %v", err)
	}
	revoked, err := ports.NextGenerationBindingAssociationStateV1(active, ports.GenerationBindingAssociationRevokedV1, core.ReasonBindingDrift, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stores[2].CompareAndSwapGenerationBindingAssociationV1(context.Background(), ports.GenerationBindingAssociationCASRequestV1{ExpectedRevision: active.Revision, Next: revoked}); err != nil {
		t.Fatal(err)
	}
	if _, err := stores[3].CompareAndSwapGenerationBindingAssociationV1(context.Background(), ports.GenerationBindingAssociationCASRequestV1{ExpectedRevision: active.Revision, Next: revoked}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale exact CAS replay bypassed expected revision: %v", err)
	}
	aba := revoked
	aba.Revision++
	aba.State = ports.GenerationBindingAssociationActiveV1
	aba.InvalidationReason = ""
	aba.UpdatedUnixNano = now.Add(2 * time.Second).UnixNano()
	aba.Digest = ""
	aba, err = ports.SealGenerationBindingAssociationFactV1(aba)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stores[4].CompareAndSwapGenerationBindingAssociationV1(context.Background(), ports.GenerationBindingAssociationCASRequestV1{ExpectedRevision: revoked.Revision, Next: aba}); err == nil {
		t.Fatalf("revoked-to-active ABA was not rejected: %v", err)
	}
	current, err := stores[5].InspectGenerationBindingAssociationV1(context.Background(), active.ID)
	if err != nil || current.Digest != revoked.Digest {
		t.Fatalf("restart Inspect did not preserve terminal current: %+v err=%v", current, err)
	}
	if err := stores[0].IntegrityCheckV1(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type countingGenerationBindingFactPortV1 struct {
	ports.GenerationBindingAssociationFactPortV1
	createWinners *atomic.Int64
}

func (p *countingGenerationBindingFactPortV1) CreateGenerationBindingAssociationV1(ctx context.Context, fact ports.GenerationBindingAssociationFactV1) (ports.GenerationBindingAssociationFactV1, error) {
	created, err := p.GenerationBindingAssociationFactPortV1.CreateGenerationBindingAssociationV1(ctx, fact)
	if err == nil {
		p.createWinners.Add(1)
	}
	return created, err
}

func TestGenerationBindingAssociationGatewayV1FailsClosedOnEachCurrentSourceDrift(t *testing.T) {
	tests := []struct {
		name  string
		drift func(*generationBindingGatewayFixture)
	}{
		{"generation", func(f *generationBindingGatewayFixture) {
			f.generation.Current = false
			f.generation, _ = ports.SealGenerationCurrentProjectionV1(f.generation)
		}},
		{"binding", func(f *generationBindingGatewayFixture) {
			f.bindingSet.State = control.BindingSetRevoked
			f.bindingSet.InvalidationReason = core.ReasonBindingDrift
		}},
		{"binding-member", func(f *generationBindingGatewayFixture) { f.bindingFact.Revision++ }},
		{"activation", func(f *generationBindingGatewayFixture) {
			f.activation.Active = false
			f.activation, _ = ports.SealGenerationActivationCurrentProjectionV1(f.activation)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Unix(91_000, 0)
			fixture := generationBindingGatewayFixtureV1(t, now)
			test.drift(fixture)
			if _, err := fixture.gateway.AssociateGenerationBindingV1(context.Background(), fixture.candidate); err == nil {
				t.Fatal("drifted current source created an association")
			}
			if fixture.store.CreateCommitCountV1() != 0 {
				t.Fatal("Fact owner changed after preflight failed")
			}
		})
	}
}

func TestGenerationBindingAssociationGatewayV1CurrentInspectDetectsPostCreateDrift(t *testing.T) {
	now := time.Unix(92_000, 0)
	fixture := generationBindingGatewayFixtureV1(t, now)
	if _, err := fixture.gateway.AssociateGenerationBindingV1(context.Background(), fixture.candidate); err != nil {
		t.Fatal(err)
	}
	fixture.activation.Active = false
	fixture.activation, _ = ports.SealGenerationActivationCurrentProjectionV1(fixture.activation)
	if _, err := fixture.gateway.InspectCurrentGenerationBindingAssociationV1(context.Background(), fixture.candidate.AssociationID); !core.HasReason(err, core.ReasonActivationFactDrift) {
		t.Fatalf("post-create activation drift must fail current Inspect: %v", err)
	}
	// Idempotency recovery remains historical and does not become a currentness grant.
	if _, err := fixture.gateway.AssociateGenerationBindingV1(context.Background(), fixture.candidate); err != nil {
		t.Fatalf("exact create replay must remain inspectable: %v", err)
	}
}

func TestGenerationBindingAssociationStoreV1CASLostReplyAndDeepClone(t *testing.T) {
	now := time.Unix(93_000, 0)
	fixture := generationBindingGatewayFixtureV1(t, now)
	fact, err := fixture.gateway.AssociateGenerationBindingV1(context.Background(), fixture.candidate)
	if err != nil {
		t.Fatal(err)
	}
	read, err := fixture.store.InspectGenerationBindingAssociationV1(context.Background(), fact.ID)
	if err != nil {
		t.Fatal(err)
	}
	read.Candidate.Activation.Operation.ExecutionScope.SandboxLease.Epoch++
	again, err := fixture.store.InspectGenerationBindingAssociationV1(context.Background(), fact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Candidate.Activation.Operation.ExecutionScope.SandboxLease.Epoch != 1 {
		t.Fatal("returned SandboxLease pointer aliased persisted association")
	}
	forged := fact
	forged.Candidate.Generation.Generation.GraphDigest = generationBindingKernelDigestV1(t, "forged-graph")
	if _, err := fixture.store.CreateGenerationBindingAssociationV1(context.Background(), forged); err == nil {
		t.Fatal("same digest field with changed create content was accepted as idempotent")
	}
	revoked, err := ports.NextGenerationBindingAssociationStateV1(fact, ports.GenerationBindingAssociationRevokedV1, core.ReasonBindingDrift, now)
	if err != nil {
		t.Fatal(err)
	}
	fixture.store.LoseNextCASReplyV1()
	if _, err := fixture.store.CompareAndSwapGenerationBindingAssociationV1(context.Background(), ports.GenerationBindingAssociationCASRequestV1{ExpectedRevision: fact.Revision, Next: revoked}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("injected CAS reply loss was not observed: %v", err)
	}
	recovered, err := fixture.store.InspectGenerationBindingAssociationV1(context.Background(), fact.ID)
	if err != nil || recovered.Digest != revoked.Digest {
		t.Fatalf("CAS reply loss did not recover by Inspect: fact=%+v err=%v", recovered, err)
	}
	if replayed, err := fixture.store.CompareAndSwapGenerationBindingAssociationV1(context.Background(), ports.GenerationBindingAssociationCASRequestV1{ExpectedRevision: fact.Revision, Next: revoked}); err != nil || replayed.Digest != revoked.Digest {
		t.Fatalf("exact stale CAS replay must be idempotent: fact=%+v err=%v", replayed, err)
	}
	forgedCAS := revoked
	forgedCAS.InvalidationReason = core.ReasonBindingExpired
	if _, err := fixture.store.CompareAndSwapGenerationBindingAssociationV1(context.Background(), ports.GenerationBindingAssociationCASRequestV1{ExpectedRevision: fact.Revision, Next: forgedCAS}); err == nil {
		t.Fatal("same digest field with changed CAS content was accepted as idempotent")
	}
}

func TestGenerationBindingAssociationConformanceV1NeverClaimsBindingOrProduction(t *testing.T) {
	now := time.Unix(94_000, 0)
	fixture := generationBindingGatewayFixtureV1(t, now)
	report, err := conformance.CheckGenerationBindingAssociationV1(context.Background(), conformance.GenerationBindingAssociationCaseV1{Gateway: fixture.gateway, Candidate: fixture.candidate})
	if err != nil {
		t.Fatal(err)
	}
	if !report.RuntimeFactOwnerObserved || !report.CurrentInspectObserved || report.CandidateIsBindingFact || report.ProductionClaimEligible {
		t.Fatalf("conformance report exceeded its authority: %+v", report)
	}
}

type generationBindingGatewayFixture struct {
	store       *fakes.GenerationBindingAssociationStoreV1
	gateway     GenerationBindingAssociationGatewayV1
	candidate   ports.GenerationBindingAssociationCandidateV1
	generation  ports.GenerationCurrentProjectionV1
	activation  ports.GenerationActivationCurrentProjectionV1
	bindingSet  control.BindingSetFactV2
	bindingFact control.BindingFactV2
}

func generationBindingGatewayFixtureV1(t *testing.T, now time.Time) *generationBindingGatewayFixture {
	t.Helper()
	manifest := ports.ComponentManifestV2{
		ContractVersion: ports.BindingContractVersionV2,
		ComponentID:     "vendor/component", Kind: "vendor/component-kind", GovernanceCategory: "vendor/execution",
		SemanticVersion: "1.0.0", ArtifactDigest: generationBindingKernelDigestV1(t, "artifact"),
		Contract: ports.ContractBindingV2{Name: "vendor/execution-contract", Version: "2.0.0", Compatible: ports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}},
		Schemas:  []ports.SchemaRefV2{}, Locality: ports.LocalityHostControlPlane,
		Dependencies: []ports.ComponentDependencyV2{}, RequiredCapabilities: []ports.CapabilityRequirementV2{},
		ProvidedCapabilities: []ports.ProvidedCapabilityV2{{Capability: "vendor/execute", TTLSeconds: 300, Schemas: []ports.SchemaRefV2{}}},
		Conformance:          ports.ConformanceFullyControlled, ResidualClass: ports.ResidualInspectable,
		Owners:      []ports.OwnerAssignmentV2{{Role: ports.OwnerEffect, OwnerComponentID: "vendor/component"}, {Role: ports.OwnerSettlement, OwnerComponentID: "vendor/component"}, {Role: ports.OwnerCleanup, OwnerComponentID: "vendor/component"}},
		Credentials: []ports.CredentialRequirementV2{}, OfflinePolicy: ports.OfflineDenied,
		Extensions: []ports.GovernanceExtensionV2{}, Annotations: []ports.DisplayAnnotationV2{},
	}
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	expires := now.Add(5 * time.Minute).UnixNano()
	grant := ports.CapabilityGrantV2{Capability: "vendor/execute", EvidenceDigest: generationBindingKernelDigestV1(t, "grant"), ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}
	fact := control.BindingFactV2{
		ID: "binding-1", ComponentID: manifest.ComponentID, Manifest: manifest, ManifestDigest: manifestDigest,
		GovernanceDigest: generationBindingKernelDigestV1(t, "governance"), State: control.BindingBound, Revision: 4,
		Grants: []ports.CapabilityGrantV2{grant}, ProbedUnixNano: now.Add(-3 * time.Second).UnixNano(), CertifiedUnixNano: now.Add(-2 * time.Second).UnixNano(),
		ConformanceEvidenceDigest: generationBindingKernelDigestV1(t, "conformance"), ExpiresUnixNano: expires, BindingSetID: "binding-set-1", RenewalEvidence: []ports.EvidenceRecordRefV2{},
	}
	if err := fact.Validate(); err != nil {
		t.Fatal(err)
	}
	set := control.BindingSetFactV2{
		ID: "binding-set-1", PlanID: "binding-plan-1", PlanDigest: generationBindingKernelDigestV1(t, "plan"), GovernanceDigest: fact.GovernanceDigest,
		State: control.BindingSetActive, Revision: 1,
		Members:          []control.BindingMemberV2{{BindingID: fact.ID, BindingRevision: fact.Revision, ComponentID: fact.ComponentID, Kind: manifest.Kind, ManifestDigest: manifestDigest, ArtifactDigest: manifest.ArtifactDigest, Contract: manifest.Contract, Owners: manifest.Owners, Grants: fact.Grants}},
		TopologicalOrder: []ports.ComponentIDV2{fact.ComponentID}, Residuals: []control.BindingResidualV2{}, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: expires,
	}
	if err := set.Validate(); err != nil {
		t.Fatal(err)
	}
	bindingPort := &staticGenerationBindingPortV1{set: &set, fact: &fact}
	component := ports.GenerationComponentManifestRefV1{ComponentID: manifest.ComponentID, ManifestDigest: manifestDigest, ArtifactDigest: manifest.ArtifactDigest}
	generation, err := ports.SealGenerationCurrentProjectionV1(ports.GenerationCurrentProjectionV1{
		Generation:         ports.GenerationArtifactRefV1{ID: "generation-1", Revision: 1, Digest: generationBindingKernelDigestV1(t, "generation"), InputDigest: generationBindingKernelDigestV1(t, "input"), ManifestDigest: generationBindingKernelDigestV1(t, "assembly-manifest"), GraphDigest: generationBindingKernelDigestV1(t, "graph"), CatalogDigest: generationBindingKernelDigestV1(t, "catalog")},
		ComponentManifests: []ports.GenerationComponentManifestRefV1{component},
		Extension: ports.GenerationGovernanceExtensionRefV1{Kind: "praxis.harness/assembly-generation", Contract: ports.SchemaRefV2{
			Namespace: "praxis.harness", Name: "assembly-generation", Version: "1.0.0", MediaType: "application/json", ContentDigest: generationBindingKernelDigestV1(t, "extension-schema"),
		}, Digest: generationBindingKernelDigestV1(t, "extension")},
		State: ports.GenerationCurrentSealedV1, Current: true, Watermark: 1, ExpiresUnixNano: now.Add(4 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := BuildGenerationBindingSetCurrentProjectionV1(context.Background(), bindingPort, generation.ComponentManifests, set.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "identity-1", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-1", PlanDigest: generationBindingKernelDigestV1(t, "lineage")}, Instance: core.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "lease-1", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, _ := ports.ExecutionScopeDigestV2(scope)
	operation := ports.OperationSubjectV3{Kind: ports.OperationScopeActivationV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, ActivationAttemptID: "activation-1", SubjectRevision: 1, CurrentProjectionRef: "activation-projection-1", CurrentProjectionDigest: generationBindingKernelDigestV1(t, "activation-current"), CurrentProjectionRevision: 1}
	operationDigest, _ := operation.DigestV3()
	activation, err := ports.SealGenerationActivationCurrentProjectionV1(ports.GenerationActivationCurrentProjectionV1{Operation: operation, OperationDigest: operationDigest, Active: true, Watermark: 1, CurrentnessDigest: generationBindingKernelDigestV1(t, "activation-watermark"), ExpiresUnixNano: now.Add(3 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := ports.SealGenerationBindingAssociationCandidateV1(ports.GenerationBindingAssociationCandidateV1{AssociationID: "association-1", Generation: generation, Binding: binding, Activation: activation, RequestedExpiresUnixNano: now.Add(2 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	store := fakes.NewGenerationBindingAssociationStoreV1(func() time.Time { return now })
	fixture := &generationBindingGatewayFixture{store: store, candidate: candidate, generation: generation, activation: activation, bindingSet: set, bindingFact: fact}
	bindingPort.set = &fixture.bindingSet
	bindingPort.fact = &fixture.bindingFact
	fixture.gateway = GenerationBindingAssociationGatewayV1{Facts: store, Generations: generationProjectionReaderV1{value: &fixture.generation}, Activations: activationProjectionReaderV1{value: &fixture.activation}, Bindings: bindingPort, Clock: func() time.Time { return now }}
	return fixture
}

type generationProjectionReaderV1 struct {
	value *ports.GenerationCurrentProjectionV1
}

func (r generationProjectionReaderV1) InspectGenerationCurrentV1(context.Context, ports.GenerationArtifactRefV1) (ports.GenerationCurrentProjectionV1, error) {
	return *r.value, nil
}

type activationProjectionReaderV1 struct {
	value *ports.GenerationActivationCurrentProjectionV1
}

func (r activationProjectionReaderV1) InspectGenerationActivationCurrentV1(context.Context, ports.OperationSubjectV3) (ports.GenerationActivationCurrentProjectionV1, error) {
	return *r.value, nil
}

type staticGenerationBindingPortV1 struct {
	set  *control.BindingSetFactV2
	fact *control.BindingFactV2
}

func (s *staticGenerationBindingPortV1) InspectBindingSet(context.Context, string) (control.BindingSetFactV2, error) {
	return *s.set, nil
}
func (s *staticGenerationBindingPortV1) InspectBinding(context.Context, string) (control.BindingFactV2, error) {
	return *s.fact, nil
}
func (s *staticGenerationBindingPortV1) CreateBinding(context.Context, control.BindingFactV2) (control.BindingFactV2, error) {
	return control.BindingFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonInvalidTransition, "read only")
}
func (s *staticGenerationBindingPortV1) CompareAndSwapBinding(context.Context, control.BindingFactCASRequestV2) (control.BindingFactV2, error) {
	return control.BindingFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonInvalidTransition, "read only")
}
func (s *staticGenerationBindingPortV1) CommitBindingSet(context.Context, control.CommitBindingSetRequestV2) (control.BindingSetFactV2, error) {
	return control.BindingSetFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonInvalidTransition, "read only")
}
func (s *staticGenerationBindingPortV1) CompareAndSwapBindingSet(context.Context, control.BindingSetCASRequestV2) (control.BindingSetFactV2, error) {
	return control.BindingSetFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonInvalidTransition, "read only")
}

func generationBindingKernelDigestV1(t *testing.T, value string) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
