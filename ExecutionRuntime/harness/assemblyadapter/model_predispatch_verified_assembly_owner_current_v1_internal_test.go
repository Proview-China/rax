package assemblyadapter

import (
	"context"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
	runtimecontrol "github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimekernel "github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func TestModelPreDispatchVerifiedAssemblyCanonicalGoldenV1(t *testing.T) {
	id, err := DeriveModelPreDispatchVerifiedAssemblyOwnerCurrentIDV1("golden-generation", "golden-generation/handoff")
	if err != nil {
		t.Fatal(err)
	}
	const wantID = "mpva-owner-current:v1:sha256:c05c7aab1acd177e819a951287120e5cbab0859d8c3eee9d8478cdab6a45f68c"
	if id != wantID {
		t.Fatalf("ID = %q, want %q", id, wantID)
	}
	compileDigest, err := ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileDigestV1(assemblycontract.CompileResultV1{})
	if err != nil {
		t.Fatal(err)
	}
	const wantCompile = core.Digest("sha256:a833f14f767cd6083cfde17198423cdf4cd0cfdb323a40fe95db69ed0465b455")
	if compileDigest != wantCompile {
		t.Fatalf("CompileDigest = %q, want %q", compileDigest, wantCompile)
	}
	projectionDigest, err := ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionDigestV1(ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{
		ContractVersion: ModelPreDispatchVerifiedAssemblyOwnerCurrentContractVersionV1,
		Ref:             ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1{ID: wantID, Revision: 1},
		CompileDigest:   wantCompile, CheckedUnixNano: 1, ExpiresUnixNano: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	const wantProjection = core.Digest("sha256:93c8ffb4f5aeb21685f1a1eee9c32b156b4d28b1ddff9772e6869e468a8013e9")
	if projectionDigest != wantProjection {
		t.Fatalf("ProjectionDigest = %q, want %q", projectionDigest, wantProjection)
	}
}

func TestModelPreDispatchVerifiedAssemblyOwnerStoreLifecycleDeepCloneAndLostReplyV1(t *testing.T) {
	now := assemblytestkit.Now
	first := modelPreDispatchVerifiedAssemblyProjectionV1(t, now, 1)
	store, err := NewInMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1(func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	first.Compile.Manifest.CurrentFacts[0].ID = "caller-mutated"
	created.Compile.Manifest.CurrentFacts[0].ID = "return-mutated"
	recovered, err := store.InspectCurrentModelPreDispatchVerifiedAssemblyOwnerV1(context.Background(), created.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Compile.Manifest.CurrentFacts[0].ID == "caller-mutated" || recovered.Compile.Manifest.CurrentFacts[0].ID == "return-mutated" {
		t.Fatal("input or returned projection aliased the Store")
	}
	replayed, err := store.EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(context.Background(), recovered)
	if err != nil || !reflect.DeepEqual(replayed, recovered) {
		t.Fatalf("lost-reply replay = %#v, %v", replayed, err)
	}

	now = now.Add(time.Second)
	second := modelPreDispatchVerifiedAssemblyProjectionV1(t, now, 2)
	updated, err := store.CompareAndSwapModelPreDispatchVerifiedAssemblyOwnerCurrentV1(context.Background(), recovered.Ref, second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectCurrentModelPreDispatchVerifiedAssemblyOwnerV1(context.Background(), recovered.Ref); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("old Ref remained current: %v", err)
	}
	if historical, err := store.InspectHistoricalModelPreDispatchVerifiedAssemblyOwnerV1(context.Background(), recovered.Ref); err != nil || historical.Ref != recovered.Ref {
		t.Fatalf("historical = %#v, %v", historical, err)
	}
	if replay, err := store.CompareAndSwapModelPreDispatchVerifiedAssemblyOwnerCurrentV1(context.Background(), recovered.Ref, updated); err != nil || replay.Ref != updated.Ref {
		t.Fatalf("lost successor reply replay = %#v, %v", replay, err)
	}
	if _, err := store.CompareAndSwapModelPreDispatchVerifiedAssemblyOwnerCurrentV1(context.Background(), updated.Ref, recovered); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("ABA replay = %v", err)
	}
}

func TestModelPreDispatchVerifiedAssemblyOwnerStoreRejectsLineageAndSameRevisionDriftV1(t *testing.T) {
	now := assemblytestkit.Now
	projection := modelPreDispatchVerifiedAssemblyProjectionV1(t, now, 1)
	store, _ := NewInMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1(func() time.Time { return now })
	if _, err := store.EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(context.Background(), projection); err != nil {
		t.Fatal(err)
	}

	drift := projection
	drift.Compile = cloneVerifiedAssemblyForTestV1(t, projection.Compile)
	drift.Compile.Diagnostics = append(drift.Compile.Diagnostics, assemblycontract.AssemblyDiagnosticV1{Severity: assemblycontract.DiagnosticInfoV1, Code: "drift", ObjectPath: "manifest", FieldPath: "digest", Owner: "harness", Remediation: "reject"})
	drift.CompileDigest, _ = ModelPreDispatchVerifiedAssemblyOwnerCurrentCompileDigestV1(drift.Compile)
	drift.Ref.Digest = ""
	drift.ProjectionDigest = ""
	drift.ProjectionDigest, _ = ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionDigestV1(drift)
	drift.Ref.Digest = drift.ProjectionDigest
	if _, err := store.EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(context.Background(), drift); err == nil {
		t.Fatal("same revision different content was accepted")
	}

	wrongHandoff := cloneVerifiedAssemblyForTestV1(t, projection)
	wrongHandoff.Conformance.HandoffRef.ID += "/wrong"
	wrongHandoff.Conformance.Digest, _ = assemblycontract.BindingConformanceDigestV1(wrongHandoff.Conformance)
	wrongHandoff.Ref.Digest = ""
	wrongHandoff.ProjectionDigest = ""
	wrongHandoff.ProjectionDigest, _ = ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionDigestV1(wrongHandoff)
	wrongHandoff.Ref.Digest = wrongHandoff.ProjectionDigest
	if _, err := store.EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(context.Background(), wrongHandoff); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("wrong HandoffRef = %v", err)
	}
}

func TestModelPreDispatchVerifiedAssemblyOwnerStoreConcurrentSingleWinnerV1(t *testing.T) {
	now := assemblytestkit.Now
	store, _ := NewInMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1(func() time.Time { return now })
	base := modelPreDispatchVerifiedAssemblyProjectionV1(t, now, 1)
	const workers = 64
	start := make(chan struct{})
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			<-start
			_, err := store.EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(context.Background(), base)
			errors <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.InspectCurrentModelPreDispatchVerifiedAssemblyOwnerV1(context.Background(), base.Ref); err != nil {
		t.Fatal(err)
	}
}

func TestModelPreDispatchVerifiedAssemblyOwnerStoreConcurrentDifferentContentSingleWinnerV1(t *testing.T) {
	baseNow := assemblytestkit.Now
	clockNow := baseNow.Add(time.Second)
	store, _ := NewInMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1(func() time.Time { return clockNow })
	base := modelPreDispatchVerifiedAssemblyProjectionV1(t, baseNow, 1)
	const workers = 64
	values := make([]ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, workers)
	for index := range values {
		candidate := cloneVerifiedAssemblyForTestV1(t, base)
		candidate.Conformance.ObservedUnixNano += int64(index)
		candidate.Conformance.Digest = ""
		sealedConformance, err := assemblycontract.SealBindingConformanceV1(candidate.Conformance, clockNow.UnixNano())
		if err != nil {
			t.Fatal(err)
		}
		candidate.Conformance = sealedConformance
		candidate.Ref.Digest = ""
		candidate.CompileDigest = ""
		candidate.CheckedUnixNano = 0
		candidate.ExpiresUnixNano = 0
		candidate.ProjectionDigest = ""
		values[index], err = SealModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1(candidate, clockNow)
		if err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for index := range values {
		go func(value ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1) {
			defer wait.Done()
			<-start
			_, err := store.EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(context.Background(), value)
			results <- err
		}(values[index])
	}
	close(start)
	wait.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if core.HasCategory(err, core.ErrorConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != workers-1 || len(store.history) != 1 {
		t.Fatalf("successes=%d conflicts=%d history=%d", successes, conflicts, len(store.history))
	}
}

func modelPreDispatchVerifiedAssemblyProjectionV1(t *testing.T, now time.Time, revision core.Revision) ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1 {
	return modelPreDispatchVerifiedAssemblyFixtureV1(t, now, revision).projection
}

type modelPreDispatchVerifiedAssemblyFixtureDataV1 struct {
	projection  ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1
	association runtimeports.GenerationBindingAssociationFactV1
	tool        toolcontract.ToolSurfaceManifestCurrentProjectionV1
	bindingPort *modelPreDispatchRuntimeBindingPortV1
}

func modelPreDispatchVerifiedAssemblyFixtureV1(t *testing.T, now time.Time, revision core.Revision) modelPreDispatchVerifiedAssemblyFixtureDataV1 {
	t.Helper()
	input := assemblytestkit.ValidInput()
	toolManifest, err := toolcontract.SealSurface(toolcontract.ToolSurfaceManifest{
		ID: "tool-surface", Revision: 1, Owner: core.OwnerRef{Domain: "praxis.tool-mcp", ID: "tool-surface-owner"},
		ResolvedPlanDigest: input.Plan.ResolvedAgentPlan.Digest, ProfileDigest: input.Plan.Profile.Digest,
		CapabilityGrantDigest: input.Plan.CapabilityGrant.Digest, RegistrySnapshotDigest: assemblytestkit.Digest("registry"),
		Entries: []toolcontract.ToolSurfaceEntry{{
			Capability: toolcontract.ObjectRef{ID: "capability/tool", Revision: 1, Digest: assemblytestkit.Digest("tool-capability")},
			Tool:       toolcontract.ObjectRef{ID: "tool/example", Revision: 1, Digest: assemblytestkit.Digest("tool-example")},
			ModelName:  "tool.example", InputSchema: assemblytestkit.Schema("tool-input"), DescriptionDigest: assemblytestkit.Digest("tool-description"),
			Visibility: toolcontract.SurfaceVisible, Allowed: true, Admission: toolcontract.AdmissionRequired,
			MechanismDigest: assemblytestkit.Digest("tool-mechanism"), EffectKinds: []runtimeports.NamespacedNameV2{"praxis.tool/execute"},
		}},
		Dialect: "praxis.model/default", CreatedUnixNano: assemblytestkit.Now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: assemblytestkit.Now.Add(7 * 24 * time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	input.Plan.ToolSurface = assemblycontract.ObjectRefV1{ID: toolManifest.ID, Revision: toolManifest.Revision, Digest: toolManifest.Digest}
	input.Plan.ExpectedInjectionManifest.Digest = toolManifest.ExpectedInjectionDigest
	payload := []byte(`{"assembly_generation":"v1"}`)
	input.ComponentManifests[0].Extensions = []runtimeports.GovernanceExtensionV2{{
		Key: "praxis.harness/assembly-generation", Required: true,
		Payload: runtimeports.OpaquePayloadV2{
			Schema: assemblytestkit.Schema("assembly-generation-extension"), ContentDigest: core.DigestBytes(payload), Length: uint64(len(payload)), Inline: payload,
			LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.runtime/default-limit", Digest: assemblytestkit.Digest("extension-limit")},
		},
	}}
	manifestDigest, err := input.ComponentManifests[0].BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	input.Modules[0].ComponentManifestRef.Digest = manifestDigest
	input, err = assemblycontract.SealAssemblyInputV1(input)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := assemblycompiler.New().Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	compiled.Generation.Revision = revision
	compiled.Generation.Digest, _ = assemblycontract.GenerationDigestV1(*compiled.Generation)
	compiled.Handoff.GenerationRef = assemblycontract.ObjectRefV1{ID: compiled.Generation.GenerationID, Revision: revision, Digest: compiled.Generation.Digest}
	compiled.Handoff.Digest, _ = assemblycontract.HandoffDigestV1(*compiled.Handoff)

	components := make([]runtimeports.GenerationComponentManifestRefV1, 0, len(compiled.Manifest.ComponentManifests))
	for _, component := range compiled.Manifest.ComponentManifests {
		digest, digestErr := component.BindingDigestV2()
		if digestErr != nil {
			t.Fatal(digestErr)
		}
		components = append(components, runtimeports.GenerationComponentManifestRefV1{ComponentID: component.ComponentID, ManifestDigest: digest, ArtifactDigest: component.ArtifactDigest})
	}
	bindingPort := modelPreDispatchRuntimeBindingFixtureV1(t, compiled.Manifest.ComponentManifests, compiled.Manifest.Plan.ResolvedAgentPlan.Digest, revision, now)
	binding, err := runtimekernel.BuildGenerationBindingSetCurrentProjectionV1(context.Background(), bindingPort, components, bindingPort.set.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	operation := modelPreDispatchActivationSubjectV1(t, compiled.Manifest.Plan.ResolvedAgentPlan.Digest, int(revision))
	operationDigest, _ := operation.DigestV3()
	activation, err := runtimeports.SealGenerationActivationCurrentProjectionV1(runtimeports.GenerationActivationCurrentProjectionV1{
		Operation: operation, OperationDigest: operationDigest, Active: true, Watermark: revision, CurrentnessDigest: assemblytestkit.Digest("activation-currentness"), ExpiresUnixNano: now.Add(4 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := AssociationRequestV1{
		ContractVersion: ContractVersionV1, AssociationID: "association/a2", Generation: *compiled.Generation, Manifest: *compiled.Manifest, Graph: *compiled.Graph, Handoff: *compiled.Handoff,
		GenerationCurrentness: GenerationCurrentnessV1{Current: true, Watermark: revision, ExpiresUnixNano: now.Add(6 * time.Minute).UnixNano()},
		ExpectedBindingSet:    BindingSetExpectationV1{ID: binding.BindingSetID, Revision: binding.BindingSetRevision}, Binding: binding,
		ExpectedActivation: operation, Activation: activation, RequestedExpiresUnixNano: now.Add(3 * time.Minute).UnixNano(),
	}
	candidate, err := BuildCandidateV1(request, now)
	if err != nil {
		t.Fatal(err)
	}
	fact, err := runtimeports.SealGenerationBindingAssociationFactV1(runtimeports.GenerationBindingAssociationFactV1{
		ID: candidate.AssociationID, Revision: revision, State: runtimeports.GenerationBindingAssociationActiveV1, Candidate: candidate, CandidateDigest: candidate.Digest,
		CreatedUnixNano: now.Add(-time.Minute).UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: candidate.RequestedExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	conformance, err := BuildBindingConformanceV1(*compiled.Handoff, fact, now)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := SealModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1(ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{
		Ref: ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1{Revision: revision}, Compile: compiled, Conformance: conformance,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := toolcontract.SealToolSurfaceManifestCurrentV1(toolcontract.ToolSurfaceManifestCurrentProjectionV1{
		Manifest: toolManifest, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: toolManifest.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	return modelPreDispatchVerifiedAssemblyFixtureDataV1{projection: sealed, association: fact, tool: tool, bindingPort: bindingPort}
}

func cloneVerifiedAssemblyForTestV1[T any](t *testing.T, value T) T {
	t.Helper()
	clone, err := cloneModelPreDispatchVerifiedAssemblyV1(value)
	if err != nil {
		t.Fatal(err)
	}
	return clone
}

type modelPreDispatchRuntimeBindingPortV1 struct {
	set   runtimecontrol.BindingSetFactV2
	facts map[string]runtimecontrol.BindingFactV2
}

func modelPreDispatchRuntimeBindingFixtureV1(t *testing.T, manifests []runtimeports.ComponentManifestV2, planDigest core.Digest, revision core.Revision, now time.Time) *modelPreDispatchRuntimeBindingPortV1 {
	t.Helper()
	governance := assemblytestkit.Digest("binding-governance")
	expires := now.Add(4 * time.Minute).UnixNano()
	port := &modelPreDispatchRuntimeBindingPortV1{facts: make(map[string]runtimecontrol.BindingFactV2, len(manifests))}
	members := make([]runtimecontrol.BindingMemberV2, 0, len(manifests))
	order := make([]runtimeports.ComponentIDV2, 0, len(manifests))
	for index, manifest := range manifests {
		manifestDigest, err := manifest.BindingDigestV2()
		if err != nil {
			t.Fatal(err)
		}
		grants := make([]runtimeports.CapabilityGrantV2, 0, len(manifest.ProvidedCapabilities))
		for _, capability := range manifest.ProvidedCapabilities {
			grants = append(grants, runtimeports.CapabilityGrantV2{
				Capability: capability.Capability, EvidenceDigest: assemblytestkit.Digest("grant-" + string(capability.Capability)),
				ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
			})
		}
		sort.Slice(grants, func(i, j int) bool { return grants[i].Capability < grants[j].Capability })
		bindingID := "binding-a2-" + string(rune('a'+index))
		fact := runtimecontrol.BindingFactV2{
			ID: bindingID, ComponentID: manifest.ComponentID, Manifest: manifest, ManifestDigest: manifestDigest, GovernanceDigest: governance,
			State: runtimecontrol.BindingBound, Revision: revision, Grants: grants,
			ProbedUnixNano: now.Add(-2 * time.Minute).UnixNano(), CertifiedUnixNano: now.Add(-time.Minute).UnixNano(),
			ConformanceEvidenceDigest: assemblytestkit.Digest("binding-conformance"), ExpiresUnixNano: expires, BindingSetID: "binding-set/a2", RenewalEvidence: []runtimeports.EvidenceRecordRefV2{},
		}
		if err := fact.Validate(); err != nil {
			t.Fatal(err)
		}
		port.facts[bindingID] = fact
		members = append(members, runtimecontrol.BindingMemberV2{
			BindingID: bindingID, BindingRevision: fact.Revision, ComponentID: fact.ComponentID, Kind: manifest.Kind,
			ManifestDigest: manifestDigest, ArtifactDigest: manifest.ArtifactDigest, Contract: manifest.Contract,
			Owners: append([]runtimeports.OwnerAssignmentV2(nil), manifest.Owners...), Grants: append([]runtimeports.CapabilityGrantV2(nil), grants...),
		})
		order = append(order, manifest.ComponentID)
	}
	port.set = runtimecontrol.BindingSetFactV2{
		ID: "binding-set/a2", PlanID: "resolved-plan/a2", PlanDigest: planDigest, GovernanceDigest: governance,
		State: runtimecontrol.BindingSetActive, Revision: revision, Members: members, TopologicalOrder: order,
		Residuals: []runtimecontrol.BindingResidualV2{}, CreatedUnixNano: now.Add(-3 * time.Minute).UnixNano(), ExpiresUnixNano: expires,
	}
	if err := port.set.Validate(); err != nil {
		t.Fatal(err)
	}
	return port
}

func (p *modelPreDispatchRuntimeBindingPortV1) InspectBindingSet(context.Context, string) (runtimecontrol.BindingSetFactV2, error) {
	return p.set, nil
}

func (p *modelPreDispatchRuntimeBindingPortV1) InspectBinding(_ context.Context, id string) (runtimecontrol.BindingFactV2, error) {
	value, ok := p.facts[id]
	if !ok {
		return runtimecontrol.BindingFactV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "binding fixture is absent")
	}
	return value, nil
}

func (*modelPreDispatchRuntimeBindingPortV1) CreateBinding(context.Context, runtimecontrol.BindingFactV2) (runtimecontrol.BindingFactV2, error) {
	return runtimecontrol.BindingFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonInvalidTransition, "read-only binding fixture")
}

func (*modelPreDispatchRuntimeBindingPortV1) CompareAndSwapBinding(context.Context, runtimecontrol.BindingFactCASRequestV2) (runtimecontrol.BindingFactV2, error) {
	return runtimecontrol.BindingFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonInvalidTransition, "read-only binding fixture")
}

func (*modelPreDispatchRuntimeBindingPortV1) CommitBindingSet(context.Context, runtimecontrol.CommitBindingSetRequestV2) (runtimecontrol.BindingSetFactV2, error) {
	return runtimecontrol.BindingSetFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonInvalidTransition, "read-only binding fixture")
}

func (*modelPreDispatchRuntimeBindingPortV1) CompareAndSwapBindingSet(context.Context, runtimecontrol.BindingSetCASRequestV2) (runtimecontrol.BindingSetFactV2, error) {
	return runtimecontrol.BindingSetFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonInvalidTransition, "read-only binding fixture")
}
