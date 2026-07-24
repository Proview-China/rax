package assemblyadapter

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimekernel "github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	toolsurface "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

type modelPreDispatchVerifiedReaderStubV1 struct {
	mu     sync.Mutex
	values []ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1
	err    error
	calls  int
}

func (r *modelPreDispatchVerifiedReaderStubV1) InspectCurrentModelPreDispatchVerifiedAssemblyOwnerV1(context.Context, ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, r.err
	}
	return r.values[min(r.calls-1, len(r.values)-1)], nil
}

func (*modelPreDispatchVerifiedReaderStubV1) modelPreDispatchVerifiedAssemblyOwnerCurrentReaderV1() {}

type modelPreDispatchAssociationReaderStubV1 struct {
	mu     sync.Mutex
	values []runtimeports.GenerationBindingAssociationFactV1
	err    error
	calls  int
}

func (r *modelPreDispatchAssociationReaderStubV1) InspectCurrentGenerationBindingAssociationV1(context.Context, string) (runtimeports.GenerationBindingAssociationFactV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.err != nil {
		return runtimeports.GenerationBindingAssociationFactV1{}, r.err
	}
	return r.values[min(r.calls-1, len(r.values)-1)], nil
}

type modelPreDispatchHandoffReaderStubV1 struct {
	mu     sync.Mutex
	values []ModelPreDispatchAssemblyHandoffCurrentProjectionV1
	err    error
	calls  int
}

func (r *modelPreDispatchHandoffReaderStubV1) InspectCurrentModelPreDispatchAssemblyHandoffV1(context.Context, runtimeports.ModelPreDispatchAssemblyExactRefV1) (ModelPreDispatchAssemblyHandoffCurrentProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.err != nil {
		return ModelPreDispatchAssemblyHandoffCurrentProjectionV1{}, r.err
	}
	return r.values[min(r.calls-1, len(r.values)-1)], nil
}

type modelPreDispatchRegistryReaderStubV1 struct {
	mu     sync.Mutex
	values []runtimeports.RegistrySnapshotRefV1
	err    error
	calls  int
}

type modelPreDispatchToolReaderStubV1 struct {
	mu     sync.Mutex
	values []toolcontract.ToolSurfaceManifestCurrentProjectionV1
	err    error
	calls  int
}

func (r *modelPreDispatchToolReaderStubV1) InspectExactToolSurfaceManifestCurrentV1(context.Context, toolcontract.ToolSurfaceManifestCurrentRefV1) (toolcontract.ToolSurfaceManifestCurrentProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.err != nil {
		return toolcontract.ToolSurfaceManifestCurrentProjectionV1{}, r.err
	}
	return r.values[min(r.calls-1, len(r.values)-1)], nil
}

func (r *modelPreDispatchRegistryReaderStubV1) InspectExactRegistrySnapshotV1(context.Context, runtimeports.RegistrySnapshotRefV1) (runtimeports.RegistrySnapshotRefV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.err != nil {
		return runtimeports.RegistrySnapshotRefV1{}, r.err
	}
	return r.values[min(r.calls-1, len(r.values)-1)], nil
}

type modelPreDispatchFixtureV1 struct {
	now         time.Time
	request     ModelPreDispatchAssemblyPublishRequestV1
	verified    ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1
	association runtimeports.GenerationBindingAssociationFactV1
	handoff     ModelPreDispatchAssemblyHandoffCurrentProjectionV1
	tool        toolcontract.ToolSurfaceManifestCurrentProjectionV1
	registry    runtimeports.RegistrySnapshotRefV1
	bindingPort *modelPreDispatchRuntimeBindingPortV1
}

func newModelPreDispatchFixtureV1(t *testing.T, version int) modelPreDispatchFixtureV1 {
	t.Helper()
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC).Add(time.Duration(version) * time.Second)
	a2 := modelPreDispatchVerifiedAssemblyFixtureV1(t, now, core.Revision(version))
	compile := a2.projection.Compile
	generationRef := a2.association.Candidate.Generation.Generation
	handoffRef := runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: a2.projection.Conformance.HandoffRef.ID, Revision: a2.projection.Conformance.HandoffRef.Revision, Digest: a2.projection.Conformance.HandoffRef.Digest}
	handoff, err := SealModelPreDispatchAssemblyHandoffCurrentProjectionV1(ModelPreDispatchAssemblyHandoffCurrentProjectionV1{
		Ref: handoffRef, CurrentnessDigest: assemblytestkit.Digest("handoff-currentness"), CheckedUnixNano: now.Add(-30 * time.Second).UnixNano(), ExpiresUnixNano: now.Add(140 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := runtimeports.RegistrySnapshotRefV1{Owner: core.OwnerRef{Domain: "praxis.registry", ID: "registry-owner"}, ContractVersion: "1.0.0", ID: "registry/surface", Revision: core.Revision(version), Digest: a2.tool.Manifest.RegistrySnapshotDigest}
	request := ModelPreDispatchAssemblyPublishRequestV1{
		ContractVersion: ModelPreDispatchAssemblyPublisherContractVersionV1, VerifiedAssembly: a2.projection.Ref,
		Association: a2.association.RefV1(), Generation: generationRef, Handoff: handoffRef,
		Manifest:      runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: compile.Generation.GenerationID + "/manifest", Revision: compile.Generation.Revision, Digest: compile.Manifest.Digest},
		Conformance:   runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: compile.Generation.GenerationID + "/conformance", Revision: compile.Generation.Revision, Digest: a2.projection.Conformance.Digest},
		ToolSurface:   runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: compile.Manifest.Plan.ToolSurface.ID, Revision: compile.Manifest.Plan.ToolSurface.Revision, Digest: compile.Manifest.Plan.ToolSurface.Digest},
		ProfileDigest: compile.Manifest.Plan.Profile.Digest, RegistrySnapshot: registry,
	}
	return modelPreDispatchFixtureV1{now: now, request: request, verified: a2.projection, association: a2.association, handoff: handoff, tool: a2.tool, registry: registry, bindingPort: a2.bindingPort}
}

func modelPreDispatchActivationSubjectV1(t *testing.T, planDigest core.Digest, version int) runtimeports.OperationSubjectV3 {
	t.Helper()
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant", ID: "agent", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage", PlanDigest: planDigest},
		Instance: core.InstanceRef{ID: "instance", Epoch: 1}, AuthorityEpoch: 1,
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	return runtimeports.OperationSubjectV3{
		Kind: runtimeports.OperationScopeActivationV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest,
		ActivationAttemptID: "activation-attempt", SubjectRevision: core.Revision(version), CurrentProjectionRef: "activation/current",
		CurrentProjectionDigest: assemblytestkit.Digest("activation-projection"), CurrentProjectionRevision: core.Revision(version),
	}
}

func newModelPreDispatchPublisherV1(t *testing.T, fixture modelPreDispatchFixtureV1, store ModelPreDispatchAssemblyCurrentStoreV1, clock func() time.Time) (*ModelPreDispatchAssemblyCurrentPublisherV1, *modelPreDispatchVerifiedReaderStubV1, *modelPreDispatchAssociationReaderStubV1, *modelPreDispatchHandoffReaderStubV1, *modelPreDispatchToolReaderStubV1, *modelPreDispatchRegistryReaderStubV1) {
	t.Helper()
	verified := &modelPreDispatchVerifiedReaderStubV1{values: []ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{fixture.verified}}
	association := &modelPreDispatchAssociationReaderStubV1{values: []runtimeports.GenerationBindingAssociationFactV1{fixture.association}}
	handoff := &modelPreDispatchHandoffReaderStubV1{values: []ModelPreDispatchAssemblyHandoffCurrentProjectionV1{fixture.handoff}}
	tool := &modelPreDispatchToolReaderStubV1{values: []toolcontract.ToolSurfaceManifestCurrentProjectionV1{fixture.tool}}
	registry := &modelPreDispatchRegistryReaderStubV1{values: []runtimeports.RegistrySnapshotRefV1{fixture.registry}}
	publisher, err := NewModelPreDispatchAssemblyCurrentPublisherV1(store, verified, association, handoff, tool, registry, clock)
	if err != nil {
		t.Fatal(err)
	}
	return publisher, verified, association, handoff, tool, registry
}

type modelPreDispatchRuntimeGenerationReaderV1 struct {
	value runtimeports.GenerationCurrentProjectionV1
}

func (r modelPreDispatchRuntimeGenerationReaderV1) InspectGenerationCurrentV1(context.Context, runtimeports.GenerationArtifactRefV1) (runtimeports.GenerationCurrentProjectionV1, error) {
	return r.value, nil
}

type modelPreDispatchRuntimeActivationReaderV1 struct {
	value runtimeports.GenerationActivationCurrentProjectionV1
}

func (r modelPreDispatchRuntimeActivationReaderV1) InspectGenerationActivationCurrentV1(context.Context, runtimeports.OperationSubjectV3) (runtimeports.GenerationActivationCurrentProjectionV1, error) {
	return r.value, nil
}

type modelPreDispatchAssociationFactStoreV1 struct {
	value runtimeports.GenerationBindingAssociationFactV1
}

func (s modelPreDispatchAssociationFactStoreV1) InspectGenerationBindingAssociationV1(context.Context, string) (runtimeports.GenerationBindingAssociationFactV1, error) {
	return s.value, nil
}

func (modelPreDispatchAssociationFactStoreV1) CreateGenerationBindingAssociationV1(context.Context, runtimeports.GenerationBindingAssociationFactV1) (runtimeports.GenerationBindingAssociationFactV1, error) {
	return runtimeports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorForbidden, core.ReasonInvalidTransition, "read-only association fixture")
}

func (modelPreDispatchAssociationFactStoreV1) CompareAndSwapGenerationBindingAssociationV1(context.Context, runtimeports.GenerationBindingAssociationCASRequestV1) (runtimeports.GenerationBindingAssociationFactV1, error) {
	return runtimeports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorForbidden, core.ReasonInvalidTransition, "read-only association fixture")
}

func TestModelPreDispatchAssemblyRealRuntimeGatewayAndToolC2FixtureV1(t *testing.T) {
	fixture := newModelPreDispatchFixtureV1(t, 1)
	clock := func() time.Time { return fixture.now }
	facts := modelPreDispatchAssociationFactStoreV1{value: fixture.association}
	gateway := runtimekernel.GenerationBindingAssociationGatewayV1{
		Facts:       facts,
		Generations: modelPreDispatchRuntimeGenerationReaderV1{value: fixture.association.Candidate.Generation},
		Activations: modelPreDispatchRuntimeActivationReaderV1{value: fixture.association.Candidate.Activation},
		Bindings:    fixture.bindingPort,
		Clock:       clock,
	}
	providerType := reflect.TypeOf(gateway)
	if providerType.PkgPath() != "github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel" || providerType.Name() != "GenerationBindingAssociationGatewayV1" {
		t.Fatalf("Runtime Gateway provider identity = %s.%s", providerType.PkgPath(), providerType.Name())
	}
	currentFact, err := gateway.InspectCurrentGenerationBindingAssociationV1(context.Background(), fixture.association.ID)
	if err != nil || currentFact.RefV1() != fixture.request.Association || fixture.verified.Conformance.Association == nil || *fixture.verified.Conformance.Association != currentFact.RefV1() {
		t.Fatalf("Runtime Gateway exact association = %#v, %v", currentFact, err)
	}
	binding := currentFact.Candidate.Binding
	if fixture.verified.Conformance.BindingSetID != binding.BindingSetID || fixture.verified.Conformance.BindingSetRevision != binding.BindingSetRevision || fixture.verified.Conformance.BindingSetDigest != binding.BindingSetDigest || fixture.verified.Conformance.BindingSetSemanticDigest != binding.BindingSetSemanticDigest || fixture.verified.Conformance.BindingSetCurrentnessDigest != binding.CurrentnessDigest || fixture.verified.Conformance.BindingSetProjectionDigest != binding.ProjectionDigest {
		t.Fatal("A2 conformance did not bind the concrete Gateway BindingSet")
	}
	if len(fixture.verified.Compile.Handoff.ProviderCandidates) != 1 || len(fixture.bindingPort.set.Members) != 1 {
		t.Fatal("fixture does not expose one exact Provider candidate/member mapping")
	}
	providerCandidate := fixture.verified.Compile.Handoff.ProviderCandidates[0]
	if err := providerCandidate.Validate(); err != nil {
		t.Fatal(err)
	}
	var module assemblycontract.ModuleDescriptorV1
	for _, candidate := range fixture.verified.Compile.Manifest.Modules {
		if candidate.ModuleID == providerCandidate.ModuleRef {
			module = candidate
			break
		}
	}
	var contribution assemblycontract.SlotContributionV1
	for _, candidate := range fixture.verified.Compile.Manifest.SlotContributions {
		if candidate.ProviderCandidateRef == providerCandidate.CandidateID {
			contribution = candidate
			break
		}
	}
	member := fixture.bindingPort.set.Members[0]
	providerRef := runtimeports.ProviderBindingRefV2{
		BindingSetID: fixture.bindingPort.set.ID, BindingSetRevision: fixture.bindingPort.set.Revision,
		ComponentID: member.ComponentID, ManifestDigest: member.ManifestDigest, ArtifactDigest: member.ArtifactDigest,
		Capability: contribution.CapabilityRef,
	}
	if err := providerRef.Validate(); err != nil || module.ComponentManifestRef.ID != string(member.ComponentID) || module.ComponentManifestRef.Digest != member.ManifestDigest || module.ArtifactDigest != member.ArtifactDigest || contribution.ModuleRef != providerCandidate.ModuleRef || contribution.SlotRef != providerCandidate.SlotRef || contribution.PortSpecRef != providerCandidate.PortSpecRef {
		t.Fatalf("Provider candidate did not map to full Runtime member: ref=%#v err=%v", providerRef, err)
	}

	a2Store, _ := NewInMemoryModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1(clock)
	if _, err := a2Store.EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(context.Background(), fixture.verified); err != nil {
		t.Fatal(err)
	}
	toolRepo, err := toolsurface.NewInMemoryToolSurfaceManifestCurrentRepositoryV1(clock)
	if err != nil {
		t.Fatal(err)
	}
	toolCurrent, err := toolRepo.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), toolcontract.ToolSurfaceManifestCurrentEnsureRequestV1{
		ContractVersion: toolcontract.ToolSurfaceManifestCurrentContractVersionV1, Manifest: fixture.tool.Manifest,
	})
	if err != nil || toolCurrent.Ref != fixture.tool.Ref {
		t.Fatalf("Tool C2 fixture = %#v, %v", toolCurrent, err)
	}
	store, _ := NewInMemoryModelPreDispatchAssemblyCurrentStoreV1(clock)
	handoff := &modelPreDispatchHandoffReaderStubV1{values: []ModelPreDispatchAssemblyHandoffCurrentProjectionV1{fixture.handoff}}
	registry := &modelPreDispatchRegistryReaderStubV1{values: []runtimeports.RegistrySnapshotRefV1{fixture.registry}}
	publisher, err := NewModelPreDispatchAssemblyCurrentPublisherV1(store, a2Store, gateway, handoff, toolRepo, registry, clock)
	if err != nil {
		t.Fatal(err)
	}
	current, err := publisher.PublishModelPreDispatchAssemblyCurrentV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.InspectCurrentModelPreDispatchAssemblyV1(context.Background(), current.Ref); err != nil {
		t.Fatal(err)
	}

	for id, fact := range fixture.bindingPort.facts {
		fact.Grants[0].EvidenceDigest = assemblytestkit.Digest("drifted-runtime-grant")
		fixture.bindingPort.facts[id] = fact
		break
	}
	if _, err := gateway.InspectCurrentGenerationBindingAssociationV1(context.Background(), fixture.association.ID); err == nil {
		t.Fatal("concrete Runtime Gateway accepted drifted Binding member")
	}
}

func TestModelPreDispatchAssemblyPublisherLifecycleV1(t *testing.T) {
	first := newModelPreDispatchFixtureV1(t, 1)
	clockNow := first.now
	clock := func() time.Time { return clockNow }
	store, err := NewInMemoryModelPreDispatchAssemblyCurrentStoreV1(clock)
	if err != nil {
		t.Fatal(err)
	}
	publisher, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, first, store, clock)
	one, err := publisher.PublishModelPreDispatchAssemblyCurrentV1(context.Background(), first.request)
	if err != nil {
		t.Fatal(err)
	}
	if one.Ref.Revision != 1 || one.Ref.Digest != one.ProjectionDigest || one.Ref.ProjectionDigest != one.ProjectionDigest {
		t.Fatalf("initial projection = %#v", one)
	}
	current, err := publisher.InspectCurrentModelPreDispatchAssemblyV1(context.Background(), one.Ref)
	if err != nil || current != one {
		t.Fatalf("current = %#v, %v", current, err)
	}

	second := newModelPreDispatchFixtureV1(t, 2)
	clockNow = second.now
	second.request.ExpectedCurrent = one.Ref
	publisher, _, _, _, _, _ = newModelPreDispatchPublisherV1(t, second, store, clock)
	two, err := publisher.PublishModelPreDispatchAssemblyCurrentV1(context.Background(), second.request)
	if err != nil {
		t.Fatal(err)
	}
	if two.Ref.ID != one.Ref.ID || two.Ref.Revision != 2 || two == one {
		t.Fatalf("successor = %#v", two)
	}
	historical, err := store.InspectHistoricalModelPreDispatchAssemblyV1(context.Background(), one.Ref)
	if err != nil || historical != one {
		t.Fatalf("historical = %#v, %v", historical, err)
	}
	if _, err := store.InspectCurrentModelPreDispatchAssemblyV1(context.Background(), one.Ref); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("old current err = %v", err)
	}
}

type lostReplyModelPreDispatchStoreV1 struct {
	inner    *InMemoryModelPreDispatchAssemblyCurrentStoreV1
	casCalls atomic.Uint64
}

func (s *lostReplyModelPreDispatchStoreV1) CompareAndSwapModelPreDispatchAssemblyCurrentV1(ctx context.Context, expected runtimeports.ModelPreDispatchAssemblyCurrentRefV1, next runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error) {
	s.casCalls.Add(1)
	if _, err := s.inner.CompareAndSwapModelPreDispatchAssemblyCurrentV1(ctx, expected, next); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "reply lost")
}
func (s *lostReplyModelPreDispatchStoreV1) InspectHistoricalModelPreDispatchAssemblyV1(ctx context.Context, ref runtimeports.ModelPreDispatchAssemblyCurrentRefV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error) {
	return s.inner.InspectHistoricalModelPreDispatchAssemblyV1(ctx, ref)
}
func (s *lostReplyModelPreDispatchStoreV1) InspectCurrentModelPreDispatchAssemblyV1(ctx context.Context, ref runtimeports.ModelPreDispatchAssemblyCurrentRefV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error) {
	return s.inner.InspectCurrentModelPreDispatchAssemblyV1(ctx, ref)
}

func TestModelPreDispatchAssemblyLostReplyUsesExactInspectV1(t *testing.T) {
	fixture := newModelPreDispatchFixtureV1(t, 1)
	clockNow := fixture.now
	clock := func() time.Time { return clockNow }
	inner, _ := NewInMemoryModelPreDispatchAssemblyCurrentStoreV1(clock)
	store := &lostReplyModelPreDispatchStoreV1{inner: inner}
	publisher, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, fixture, store, clock)
	got, err := publisher.PublishModelPreDispatchAssemblyCurrentV1(context.Background(), fixture.request)
	if err != nil || got.Ref.Revision != 1 || store.casCalls.Load() != 1 {
		t.Fatalf("lost reply = %#v, %v, CAS=%d", got, err, store.casCalls.Load())
	}
}

func TestModelPreDispatchAssemblyRejectsOwnerDriftAndTTLSpliceV1(t *testing.T) {
	fixture := newModelPreDispatchFixtureV1(t, 1)
	clock := func() time.Time { return fixture.now }
	cases := map[string]func(*modelPreDispatchVerifiedReaderStubV1, *modelPreDispatchAssociationReaderStubV1, *modelPreDispatchHandoffReaderStubV1, *modelPreDispatchToolReaderStubV1, *modelPreDispatchRegistryReaderStubV1){
		"verified S2 drift": func(reader *modelPreDispatchVerifiedReaderStubV1, _ *modelPreDispatchAssociationReaderStubV1, _ *modelPreDispatchHandoffReaderStubV1, _ *modelPreDispatchToolReaderStubV1, _ *modelPreDispatchRegistryReaderStubV1) {
			drift := fixture.verified
			drift.ExpiresUnixNano--
			drift.Conformance.ExpiresUnixNano--
			drift.Conformance.Digest, _ = assemblycontract.BindingConformanceDigestV1(drift.Conformance)
			drift.Ref.Digest = ""
			drift.ProjectionDigest = ""
			drift.ProjectionDigest, _ = ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionDigestV1(drift)
			drift.Ref.Digest = drift.ProjectionDigest
			reader.values = []ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{fixture.verified, drift}
		},
		"registry exact drift": func(_ *modelPreDispatchVerifiedReaderStubV1, _ *modelPreDispatchAssociationReaderStubV1, _ *modelPreDispatchHandoffReaderStubV1, _ *modelPreDispatchToolReaderStubV1, reader *modelPreDispatchRegistryReaderStubV1) {
			drift := fixture.registry
			drift.Revision++
			drift.Digest = assemblytestkit.Digest("registry-drift")
			reader.values = []runtimeports.RegistrySnapshotRefV1{drift}
		},
		"handoff expired": func(_ *modelPreDispatchVerifiedReaderStubV1, _ *modelPreDispatchAssociationReaderStubV1, reader *modelPreDispatchHandoffReaderStubV1, _ *modelPreDispatchToolReaderStubV1, _ *modelPreDispatchRegistryReaderStubV1) {
			drift := fixture.handoff
			drift.CheckedUnixNano = fixture.now.Add(-2 * time.Minute).UnixNano()
			drift.ExpiresUnixNano = fixture.now.UnixNano()
			drift, _ = SealModelPreDispatchAssemblyHandoffCurrentProjectionV1(drift)
			reader.values = []ModelPreDispatchAssemblyHandoffCurrentProjectionV1{drift}
		},
		"binding digest splice": func(_ *modelPreDispatchVerifiedReaderStubV1, reader *modelPreDispatchAssociationReaderStubV1, _ *modelPreDispatchHandoffReaderStubV1, _ *modelPreDispatchToolReaderStubV1, _ *modelPreDispatchRegistryReaderStubV1) {
			drift := fixture.association
			drift.Candidate.Binding.BindingSetDigest = assemblytestkit.Digest("binding-splice")
			reader.values = []runtimeports.GenerationBindingAssociationFactV1{drift}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			store, _ := NewInMemoryModelPreDispatchAssemblyCurrentStoreV1(clock)
			publisher, verified, association, handoff, tool, registry := newModelPreDispatchPublisherV1(t, fixture, store, clock)
			mutate(verified, association, handoff, tool, registry)
			if _, err := publisher.PublishModelPreDispatchAssemblyCurrentV1(context.Background(), fixture.request); err == nil {
				t.Fatal("expected fail closed")
			}
			if len(store.current) != 0 || len(store.history) != 0 {
				t.Fatal("failed publish mutated Store")
			}
		})
	}
}

func TestModelPreDispatchAssemblyRejectsRawCallerSpliceBeforeDownstreamReadersV1(t *testing.T) {
	fixture := newModelPreDispatchFixtureV1(t, 1)
	clock := func() time.Time { return fixture.now }
	store, _ := NewInMemoryModelPreDispatchAssemblyCurrentStoreV1(clock)
	publisher, verified, association, handoff, tool, registry := newModelPreDispatchPublisherV1(t, fixture, store, clock)
	drift := fixture.request
	drift.Manifest.Digest = assemblytestkit.Digest("caller-manifest-splice")
	if _, err := publisher.PublishModelPreDispatchAssemblyCurrentV1(context.Background(), drift); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("raw caller splice = %v", err)
	}
	if verified.calls != 1 || association.calls != 0 || handoff.calls != 0 || tool.calls != 0 || registry.calls != 0 || len(store.history) != 0 || len(store.current) != 0 {
		t.Fatalf("raw splice reached downstream: A=%d B=%d H=%d C=%d R=%d", verified.calls, association.calls, handoff.calls, tool.calls, registry.calls)
	}
}

func TestModelPreDispatchAssemblyNilTypedNilAndCanceledV1(t *testing.T) {
	fixture := newModelPreDispatchFixtureV1(t, 1)
	clock := func() time.Time { return fixture.now }
	store, _ := NewInMemoryModelPreDispatchAssemblyCurrentStoreV1(clock)
	verified := &modelPreDispatchVerifiedReaderStubV1{values: []ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{fixture.verified}}
	association := &modelPreDispatchAssociationReaderStubV1{values: []runtimeports.GenerationBindingAssociationFactV1{fixture.association}}
	handoff := &modelPreDispatchHandoffReaderStubV1{values: []ModelPreDispatchAssemblyHandoffCurrentProjectionV1{fixture.handoff}}
	tool := &modelPreDispatchToolReaderStubV1{values: []toolcontract.ToolSurfaceManifestCurrentProjectionV1{fixture.tool}}
	registry := &modelPreDispatchRegistryReaderStubV1{values: []runtimeports.RegistrySnapshotRefV1{fixture.registry}}
	var nilVerified *modelPreDispatchVerifiedReaderStubV1
	var nilAssociation *modelPreDispatchAssociationReaderStubV1
	var nilHandoff *modelPreDispatchHandoffReaderStubV1
	var nilTool *modelPreDispatchToolReaderStubV1
	var nilRegistry *modelPreDispatchRegistryReaderStubV1
	var nilStore *lostReplyModelPreDispatchStoreV1
	cases := []struct {
		store    ModelPreDispatchAssemblyCurrentStoreV1
		verified ModelPreDispatchVerifiedAssemblyOwnerCurrentReaderV1
		assoc    runtimeports.GenerationBindingAssociationCurrentReaderV1
		hand     ModelPreDispatchAssemblyHandoffCurrentReaderV1
		tool     toolcontract.ToolSurfaceManifestCurrentReaderV1
		reg      runtimeports.RegistrySnapshotExactReaderV1
		clock    func() time.Time
	}{{nilStore, verified, association, handoff, tool, registry, clock}, {store, nilVerified, association, handoff, tool, registry, clock}, {store, verified, nilAssociation, handoff, tool, registry, clock}, {store, verified, association, nilHandoff, tool, registry, clock}, {store, verified, association, handoff, nilTool, registry, clock}, {store, verified, association, handoff, tool, nilRegistry, clock}, {store, verified, association, handoff, tool, registry, nil}}
	for index, testCase := range cases {
		if _, err := NewModelPreDispatchAssemblyCurrentPublisherV1(testCase.store, testCase.verified, testCase.assoc, testCase.hand, testCase.tool, testCase.reg, testCase.clock); err == nil || !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("case %d = %v", index, err)
		}
	}
	publisher, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, fixture, store, clock)
	if _, err := publisher.PublishModelPreDispatchAssemblyCurrentV1(nil, fixture.request); err == nil {
		t.Fatal("nil context accepted")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := publisher.PublishModelPreDispatchAssemblyCurrentV1(canceled, fixture.request); err == nil || !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("canceled = %v", err)
	}
	if verified.calls != 0 || association.calls != 0 || handoff.calls != 0 || tool.calls != 0 || registry.calls != 0 {
		t.Fatal("invalid contexts reached Owner Readers")
	}
}

func TestModelPreDispatchAssemblyStoreChecksCancellationAfterLockV1(t *testing.T) {
	fixture := newModelPreDispatchFixtureV1(t, 1)
	clockNow := fixture.now
	clock := func() time.Time { return clockNow }
	store, _ := NewInMemoryModelPreDispatchAssemblyCurrentStoreV1(clock)
	publisher, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, fixture, store, clock)
	next, err := publisher.PublishModelPreDispatchAssemblyCurrentV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	second := newModelPreDispatchFixtureV1(t, 2)
	clockNow = second.now
	second.request.ExpectedCurrent = next.Ref
	pub2, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, second, store, clock)
	// Build the valid successor with a separate Store, then block this Store's CAS.
	temp, _ := NewInMemoryModelPreDispatchAssemblyCurrentStoreV1(clock)
	firstTemp, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, fixture, temp, clock)
	tempOne, _ := firstTemp.PublishModelPreDispatchAssemblyCurrentV1(context.Background(), fixture.request)
	second.request.ExpectedCurrent = tempOne.Ref
	secondTemp, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, second, temp, clock)
	successor, err := secondTemp.PublishModelPreDispatchAssemblyCurrentV1(context.Background(), second.request)
	if err != nil {
		t.Fatal(err)
	}
	_ = pub2
	ctx, cancel := context.WithCancel(context.Background())
	store.mu.Lock()
	done := make(chan error, 1)
	go func() {
		_, err := store.CompareAndSwapModelPreDispatchAssemblyCurrentV1(ctx, next.Ref, successor)
		done <- err
	}()
	time.Sleep(time.Millisecond)
	cancel()
	store.mu.Unlock()
	if err := <-done; err == nil || !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("post-lock cancel = %v", err)
	}
	if store.current[next.Ref.ID] != next.Ref || len(store.history[next.Ref.ID]) != 1 {
		t.Fatal("post-lock cancellation mutated Store")
	}
}

func TestModelPreDispatchAssemblyFullRefCASAndABAV1(t *testing.T) {
	first := newModelPreDispatchFixtureV1(t, 1)
	clockNow := first.now
	clock := func() time.Time { return clockNow }
	store, _ := NewInMemoryModelPreDispatchAssemblyCurrentStoreV1(clock)
	publisher, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, first, store, clock)
	one, _ := publisher.PublishModelPreDispatchAssemblyCurrentV1(context.Background(), first.request)
	second := newModelPreDispatchFixtureV1(t, 2)
	clockNow = second.now
	second.request.ExpectedCurrent = one.Ref
	pub2, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, second, store, clock)
	two, err := pub2.PublishModelPreDispatchAssemblyCurrentV1(context.Background(), second.request)
	if err != nil {
		t.Fatal(err)
	}
	wrong := two.Ref
	wrong.RegistrySnapshot.Digest = assemblytestkit.Digest("wrong-registry")
	wrong.Digest = two.Ref.Digest
	third := newModelPreDispatchFixtureV1(t, 3)
	third.request.ExpectedCurrent = wrong
	pub3, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, third, store, clock)
	if _, err := pub3.PublishModelPreDispatchAssemblyCurrentV1(context.Background(), third.request); err == nil {
		t.Fatal("forged full expected Ref accepted")
	}
	// Explicitly prove the Store rejects a previously seen watermark.
	store.watermarks[two.Ref.ID][assemblytestkit.Digest("future-watermark")] = struct{}{}
	if store.current[two.Ref.ID] != two.Ref {
		t.Fatal("ABA preparation changed current")
	}
}

func TestModelPreDispatchAssemblyConcurrentPublishV1(t *testing.T) {
	fixture := newModelPreDispatchFixtureV1(t, 1)
	clock := func() time.Time { return fixture.now }
	store, _ := NewInMemoryModelPreDispatchAssemblyCurrentStoreV1(clock)
	publisher, _, _, _, _, _ := newModelPreDispatchPublisherV1(t, fixture, store, clock)
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	results := make(chan runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := publisher.PublishModelPreDispatchAssemblyCurrentV1(context.Background(), fixture.request)
			if err != nil {
				errs <- err
				return
			}
			results <- value
		}()
	}
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		t.Fatal(err)
	}
	var first runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1
	for value := range results {
		if first == (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}) {
			first = value
		} else if value != first {
			t.Fatal("same canonical concurrent publish returned different projections")
		}
	}
	if len(store.history[first.Ref.ID]) != 1 || store.current[first.Ref.ID] != first.Ref {
		t.Fatal("concurrent publish created duplicate history")
	}
}

func TestModelPreDispatchAssemblyCanonicalBodiesAreFieldSensitiveV1(t *testing.T) {
	fixture := newModelPreDispatchFixtureV1(t, 1)
	binding := fixture.association.Candidate.Binding
	ref := runtimeports.ModelPreDispatchAssemblyBindingSetRefV1{ID: binding.BindingSetID, Revision: binding.BindingSetRevision, Digest: binding.BindingSetDigest, SemanticDigest: binding.BindingSetSemanticDigest, CurrentnessDigest: binding.CurrentnessDigest, ProjectionDigest: binding.ProjectionDigest, ExpiresUnixNano: binding.ExpiresUnixNano}
	semantic, err := digestModelPreDispatchAssemblySemanticV1(fixture.request, ref)
	if err != nil {
		t.Fatal(err)
	}
	changed := fixture.request
	changed.RegistrySnapshot.Digest = assemblytestkit.Digest("changed-registry")
	other, err := digestModelPreDispatchAssemblySemanticV1(changed, ref)
	if err != nil || semantic == other {
		t.Fatalf("registry digest sensitivity = %s/%s, %v", semantic, other, err)
	}
	snapshot := modelPreDispatchAssemblyOwnerSnapshotV1{verified: fixture.verified, association: fixture.association, handoff: fixture.handoff, tool: fixture.tool, registry: fixture.registry}
	currentness, _ := digestModelPreDispatchAssemblyCurrentnessV1(fixture.request, snapshot)
	snapshot.handoff.CurrentnessDigest = assemblytestkit.Digest("changed-handoff-currentness")
	otherCurrentness, _ := digestModelPreDispatchAssemblyCurrentnessV1(fixture.request, snapshot)
	if currentness == otherCurrentness {
		t.Fatal("Handoff currentness splice did not change digest")
	}
}

func TestModelPreDispatchAssemblyRuntimeReaderMethodSetV1(t *testing.T) {
	reader := reflect.TypeOf((*runtimeports.ModelPreDispatchAssemblyCurrentReaderV1)(nil)).Elem()
	if reader.NumMethod() != 1 || reader.Method(0).Name != "InspectCurrentModelPreDispatchAssemblyV1" {
		t.Fatalf("Runtime Reader method set = %v", reader)
	}
	if !reflect.TypeOf((*ModelPreDispatchAssemblyCurrentPublisherV1)(nil)).Implements(reader) {
		t.Fatal("Publisher does not implement Runtime Reader")
	}
}

func TestModelPreDispatchAssemblyHandoffSealRejectsWrongVersionV1(t *testing.T) {
	fixture := newModelPreDispatchFixtureV1(t, 1)
	drift := fixture.handoff
	drift.ContractVersion = "praxis.harness.model-predispatch-assembly-current-publisher/v2"
	if _, err := SealModelPreDispatchAssemblyHandoffCurrentProjectionV1(drift); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("wrong nonzero Handoff version = %v", err)
	}
}

func TestModelPreDispatchAssemblyRecoveryHasIndependentHardCapV1(t *testing.T) {
	now := time.Now()
	ctx, cancel := boundedModelPreDispatchAssemblyRecoveryContextV1(context.Background(), now, now.Add(time.Hour).UnixNano())
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("recovery context has no deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > modelPreDispatchAssemblyRecoveryHardCapV1+100*time.Millisecond {
		t.Fatalf("recovery deadline = %s", remaining)
	}
}

var _ = errors.Is
