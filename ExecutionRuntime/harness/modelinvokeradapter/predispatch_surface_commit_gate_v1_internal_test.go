package modelinvokeradapter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type gatePreparedReaderV1 struct {
	value modelinvoker.PreparedModelInvocationFactV1
	calls atomic.Int64
}

func (r *gatePreparedReaderV1) InspectExactPreparedModelInvocationV1(context.Context, modelinvoker.PreparedModelInvocationRefV1) (modelinvoker.PreparedModelInvocationFactV1, error) {
	r.calls.Add(1)
	return r.value.Clone(), nil
}

type gateCurrentReaderV1 struct {
	value modelinvoker.PreparedModelInvocationCurrentProjectionV1
	calls atomic.Int64
}

func (r *gateCurrentReaderV1) InspectExactPreparedModelInvocationCurrentV1(context.Context, modelinvoker.PreparedModelInvocationCurrentRefV1) (modelinvoker.PreparedModelInvocationCurrentProjectionV1, error) {
	r.calls.Add(1)
	return r.value.Clone(), nil
}

type gateAssemblyReaderV1 struct {
	value runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1
	calls atomic.Int64
}

func (r *gateAssemblyReaderV1) InspectCurrentModelPreDispatchAssemblyV1(context.Context, runtimeports.ModelPreDispatchAssemblyCurrentRefV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error) {
	r.calls.Add(1)
	return r.value, nil
}

type gateSurfaceReaderV1 struct {
	value toolcontract.ToolSurfaceManifestCurrentProjectionV1
	calls atomic.Int64
}

func (r *gateSurfaceReaderV1) InspectExactToolSurfaceManifestCurrentV1(context.Context, toolcontract.ToolSurfaceManifestCurrentRefV1) (toolcontract.ToolSurfaceManifestCurrentProjectionV1, error) {
	r.calls.Add(1)
	return r.value, nil
}

type gateBindingRepositoryV1 struct {
	mu              sync.Mutex
	owner           core.OwnerRef
	now             time.Time
	binding         toolcontract.ToolSurfaceInvocationBindingV1
	ack             toolcontract.ToolSurfaceInvocationBindingAckV1
	hasBinding      bool
	inspectCalls    atomic.Int64
	ensureCalls     atomic.Int64
	loseEnsureReply bool
}

func (r *gateBindingRepositoryV1) EnsureToolSurfaceInvocationBindingV1(ctx context.Context, request toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1) (toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	r.ensureCalls.Add(1)
	if ctx == nil || ctx.Err() != nil {
		return toolcontract.ToolSurfaceInvocationBindingV1{}, toolcontract.ToolSurfaceInvocationBindingAckV1{}, core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "fixture context unavailable")
	}
	subject, err := toolcontract.SealToolSurfaceInvocationBindingSubjectV1(request)
	if err != nil {
		return toolcontract.ToolSurfaceInvocationBindingV1{}, toolcontract.ToolSurfaceInvocationBindingAckV1{}, err
	}
	binding, err := toolcontract.SealToolSurfaceInvocationBindingV1(toolcontract.ToolSurfaceInvocationBindingV1{
		Ref: toolcontract.ToolSurfaceInvocationBindingRefV1{Owner: r.owner}, Subject: subject,
		CreatedUnixNano: r.now.UnixNano(), NotAfterUnixNano: toolcontract.ToolSurfaceInvocationBindingNotAfterV1(subject, time.Time{}),
	})
	if err != nil {
		return toolcontract.ToolSurfaceInvocationBindingV1{}, toolcontract.ToolSurfaceInvocationBindingAckV1{}, err
	}
	ack, err := toolcontract.SealToolSurfaceInvocationBindingAckV1(toolcontract.ToolSurfaceInvocationBindingAckV1{
		BindingRef: binding.Ref, Invocation: request.Invocation, PreparedFactRef: request.PreparedFactRef,
		PreparedCurrentRef: request.PreparedCurrentRef, CheckedUnixNano: r.now.UnixNano(), NotAfterUnixNano: binding.NotAfterUnixNano,
	})
	if err != nil {
		return toolcontract.ToolSurfaceInvocationBindingV1{}, toolcontract.ToolSurfaceInvocationBindingAckV1{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hasBinding {
		if r.binding.Digest != binding.Digest {
			return toolcontract.ToolSurfaceInvocationBindingV1{}, toolcontract.ToolSurfaceInvocationBindingAckV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "fixture binding drift")
		}
		return r.binding, r.ack, nil
	}
	r.binding, r.ack, r.hasBinding = binding, ack, true
	if r.loseEnsureReply {
		return toolcontract.ToolSurfaceInvocationBindingV1{}, toolcontract.ToolSurfaceInvocationBindingAckV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEffectUnknownOutcome, "fixture lost Ensure reply")
	}
	return binding, ack, nil
}

func (r *gateBindingRepositoryV1) InspectToolSurfaceInvocationBindingByInvocationV1(context.Context, toolcontract.ToolSurfaceInvocationCoordinateV1) (toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	r.inspectCalls.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.hasBinding {
		return toolcontract.ToolSurfaceInvocationBindingV1{}, toolcontract.ToolSurfaceInvocationBindingAckV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "fixture binding absent")
	}
	return r.binding, r.ack, nil
}

func (r *gateBindingRepositoryV1) InspectExactToolSurfaceInvocationBindingV1(_ context.Context, ref toolcontract.ToolSurfaceInvocationBindingRefV1) (toolcontract.ToolSurfaceInvocationBindingV1, toolcontract.ToolSurfaceInvocationBindingAckV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.binding.Ref != ref {
		return toolcontract.ToolSurfaceInvocationBindingV1{}, toolcontract.ToolSurfaceInvocationBindingAckV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "fixture exact binding absent")
	}
	return r.binding, r.ack, nil
}

type gateFixtureV1 struct {
	now      time.Time
	fact     modelinvoker.PreparedModelInvocationFactV1
	current  modelinvoker.PreparedModelInvocationCurrentProjectionV1
	assembly runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1
	surface  toolcontract.ToolSurfaceManifestCurrentProjectionV1
}

func newGateFixtureV1(t *testing.T) gateFixtureV1 {
	t.Helper()
	now := time.Unix(1_900_000_000, 0).UTC()
	digest := func(label string) core.Digest { return core.DigestBytes([]byte(label)) }
	registry := runtimeports.RegistrySnapshotRefV1{
		Owner: core.OwnerRef{Domain: "registry", ID: "registry-owner"}, ContractVersion: "1.0.0",
		ID: "surface-registry-snapshot", Revision: 1, Digest: digest("surface-registry"),
	}
	surfaceManifest, err := toolcontract.SealSurface(toolcontract.ToolSurfaceManifest{
		ID: "surface-example", Revision: 1, Owner: core.OwnerRef{Domain: "praxis.tool", ID: "surface-owner"},
		ResolvedPlanDigest: digest("surface-plan"), ProfileDigest: digest("surface-profile"),
		CapabilityGrantDigest: digest("surface-grant"), RegistrySnapshotDigest: registry.Digest,
		Entries: []toolcontract.ToolSurfaceEntry{{
			Capability: toolcontract.ObjectRef{ID: "capability-example", Revision: 1, Digest: digest("capability")},
			Tool:       toolcontract.ObjectRef{ID: "tool-example", Revision: 1, Digest: digest("tool")},
			ModelName:  "tool.example", InputSchema: runtimeports.SchemaRefV2{Namespace: "praxis.tool", Name: "input", Version: "1.0.0", MediaType: "application/json", ContentDigest: digest("schema")},
			DescriptionDigest: digest("description"), Visibility: toolcontract.SurfaceVisible, Allowed: true,
			Admission: toolcontract.AdmissionRequired, MechanismDigest: digest("mechanism"), EffectKinds: []runtimeports.NamespacedNameV2{"praxis.tool/execute"},
		}},
		Dialect: "model/default", CreatedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	surface, err := toolcontract.SealToolSurfaceManifestCurrentV1(toolcontract.ToolSurfaceManifestCurrentProjectionV1{
		Manifest: surfaceManifest, Owner: surfaceManifest.Owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: surfaceManifest.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	requestDigest := digest("prepared-request")
	fact, err := modelinvoker.SealPreparedModelInvocationFactV1(modelinvoker.PreparedModelInvocationFactV1{
		InvocationID: "invocation-1", InvocationDigest: requestDigest, UnifiedRequestDigest: requestDigest,
		RequestToolsDigest: digest("request-tools"), PreparedPlanDigest: digest("prepared-plan"), RouteDigest: digest("route"),
		ProfileDigest: surfaceManifest.ProfileDigest, ActualToolSurfaceDigest: surfaceManifest.ExpectedInjectionDigest,
		ActualProviderInjectionDigest: digest("provider-injection"),
		CapabilitySnapshotRef:         modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{ContractVersion: "1.0.0", ID: "capability-snapshot", Revision: 1, Digest: digest("capability-snapshot")},
		RegistrySnapshotRef:           registry, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), NotAfterUnixNano: now.Add(30 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(modelinvoker.PreparedModelInvocationCurrentProjectionV1{
		Prepared: fact.Ref(), CapabilitySnapshotRef: fact.CapabilitySnapshotRef, RegistrySnapshotRef: registry,
		ActualToolSurfaceDigest: fact.ActualToolSurfaceDigest, ActualProviderInjectionDigest: fact.ActualProviderInjectionDigest,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(20 * time.Minute).UnixNano(), NotAfterUnixNano: fact.NotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	assembly, err := runtimeports.SealModelPreDispatchAssemblyCurrentProjectionV1(runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{
		Ref:           runtimeports.ModelPreDispatchAssemblyCurrentRefV1{Revision: 1},
		Generation:    runtimeports.GenerationArtifactRefV1{ID: "generation-1", Revision: 1, Digest: digest("generation"), InputDigest: digest("input"), ManifestDigest: digest("manifest"), GraphDigest: digest("graph"), CatalogDigest: digest("catalog")},
		Handoff:       runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: "handoff-1", Revision: 1, Digest: digest("handoff")},
		BindingSet:    runtimeports.ModelPreDispatchAssemblyBindingSetRefV1{ID: "binding-set-1", Revision: 1, Digest: digest("binding-set"), SemanticDigest: digest("binding-semantic"), CurrentnessDigest: digest("binding-current"), ProjectionDigest: digest("binding-projection"), ExpiresUnixNano: now.Add(25 * time.Minute).UnixNano()},
		Manifest:      runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: "assembly-manifest", Revision: 1, Digest: digest("assembly-manifest")},
		Conformance:   runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: "conformance", Revision: 1, Digest: digest("conformance")},
		ToolSurface:   runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: surface.Ref.ID, Revision: surface.Ref.Revision, Digest: surface.Ref.Digest},
		ProfileDigest: fact.ProfileDigest, RegistrySnapshot: registry, SemanticDigest: digest("assembly-semantic"), CurrentnessDigest: digest("assembly-currentness"),
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(15 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return gateFixtureV1{now: now, fact: fact, current: current, assembly: assembly, surface: surface}
}

func newGateV1(t *testing.T, fixture gateFixtureV1) (*PreparedModelInvocationSurfaceCommitGateV1, *gatePreparedReaderV1, *gateCurrentReaderV1, *gateAssemblyReaderV1, *gateSurfaceReaderV1, *gateBindingRepositoryV1) {
	t.Helper()
	prepared := &gatePreparedReaderV1{value: fixture.fact}
	current := &gateCurrentReaderV1{value: fixture.current}
	assembly := &gateAssemblyReaderV1{value: fixture.assembly}
	surface := &gateSurfaceReaderV1{value: fixture.surface}
	binding := &gateBindingRepositoryV1{owner: core.OwnerRef{Domain: "praxis.tool", ID: "binding-owner"}, now: fixture.now.Add(time.Second)}
	var mu sync.Mutex
	next := fixture.now.Add(2 * time.Second)
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		value := next
		next = next.Add(time.Second)
		return value
	}
	gate, err := NewPreparedModelInvocationSurfaceCommitGateV1(
		fixture.assembly.Ref,
		modelinvoker.PreparedModelInvocationGateImplementationRefV1{Owner: core.OwnerRef{Domain: "praxis.harness", ID: "gate-owner"}, ContractVersion: "praxis.harness/model-predispatch-surface-commit-gate/v1", ID: "gate-1", Revision: 1, Digest: core.DigestBytes([]byte("gate"))},
		prepared, current, assembly, surface, binding, NewInMemoryPreparedModelInvocationAckRepositoryV1(), clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	return gate, prepared, current, assembly, surface, binding
}

func TestPreparedModelInvocationSurfaceCommitGateV1CommitAndStoredRecovery(t *testing.T) {
	fixture := newGateFixtureV1(t)
	gate, prepared, current, assembly, surface, binding := newGateV1(t, fixture)
	ack, err := gate.Commit(context.Background(), fixture.fact.Ref(), fixture.current.Ref())
	if err != nil {
		t.Fatal(err)
	}
	if err := ack.Validate(); err != nil {
		t.Fatal(err)
	}
	if prepared.calls.Load() != 2 || current.calls.Load() != 2 || assembly.calls.Load() != 2 || surface.calls.Load() != 2 || binding.inspectCalls.Load() != 1 || binding.ensureCalls.Load() != 1 {
		t.Fatalf("unexpected S1/S2 call counts: prepared=%d current=%d assembly=%d surface=%d inspect=%d ensure=%d", prepared.calls.Load(), current.calls.Load(), assembly.calls.Load(), surface.calls.Load(), binding.inspectCalls.Load(), binding.ensureCalls.Load())
	}
	second, err := gate.Commit(context.Background(), fixture.fact.Ref(), fixture.current.Ref())
	if err != nil || second != ack {
		t.Fatalf("stored ACK recovery drifted: ack=%+v err=%v", second, err)
	}
	if prepared.calls.Load() != 2 || current.calls.Load() != 3 || assembly.calls.Load() != 2 || surface.calls.Load() != 2 || binding.inspectCalls.Load() != 1 || binding.ensureCalls.Load() != 1 {
		t.Fatal("stored ACK path touched Tool or non-Current Owner readers")
	}
	read, err := gate.InspectExactAck(context.Background(), ack.Ref())
	if err != nil || read != ack {
		t.Fatalf("exact ACK read drifted: ack=%+v err=%v", read, err)
	}
}

func TestPreparedModelInvocationSurfaceCommitGateV1FailClosedBeforeOwnerReads(t *testing.T) {
	fixture := newGateFixtureV1(t)
	gate, prepared, current, assembly, surface, binding := newGateV1(t, fixture)
	if _, err := gate.Commit(nil, fixture.fact.Ref(), fixture.current.Ref()); err == nil {
		t.Fatal("nil context must fail")
	}
	if _, err := (*PreparedModelInvocationSurfaceCommitGateV1)(nil).Commit(context.Background(), fixture.fact.Ref(), fixture.current.Ref()); err == nil {
		t.Fatal("nil receiver must fail")
	}
	drifted := fixture.current.Ref()
	drifted.Prepared.InvocationID = "other"
	if _, err := gate.Commit(context.Background(), fixture.fact.Ref(), drifted); err == nil {
		t.Fatal("spliced Current must fail")
	}
	if prepared.calls.Load()+current.calls.Load()+assembly.calls.Load()+surface.calls.Load()+binding.inspectCalls.Load()+binding.ensureCalls.Load() != 0 {
		t.Fatal("invalid inputs reached an Owner reader")
	}
}

func TestPreparedModelInvocationSurfaceCommitGateV1HardFailuresAndLostReply(t *testing.T) {
	t.Run("surface drift", func(t *testing.T) {
		fixture := newGateFixtureV1(t)
		gate, _, _, _, surface, binding := newGateV1(t, fixture)
		surface.value.Ref.Digest = core.DigestBytes([]byte("other-surface"))
		if _, err := gate.Commit(context.Background(), fixture.fact.Ref(), fixture.current.Ref()); err == nil {
			t.Fatal("surface exact drift must fail")
		}
		if binding.inspectCalls.Load()+binding.ensureCalls.Load() != 0 {
			t.Fatal("surface drift reached Tool Binding Repository")
		}
	})

	t.Run("ttl expired", func(t *testing.T) {
		fixture := newGateFixtureV1(t)
		prepared := &gatePreparedReaderV1{value: fixture.fact}
		current := &gateCurrentReaderV1{value: fixture.current}
		assembly := &gateAssemblyReaderV1{value: fixture.assembly}
		surface := &gateSurfaceReaderV1{value: fixture.surface}
		binding := &gateBindingRepositoryV1{owner: core.OwnerRef{Domain: "praxis.tool", ID: "binding-owner"}, now: fixture.now}
		gate, err := NewPreparedModelInvocationSurfaceCommitGateV1(
			fixture.assembly.Ref,
			modelinvoker.PreparedModelInvocationGateImplementationRefV1{Owner: core.OwnerRef{Domain: "praxis.harness", ID: "gate-owner"}, ContractVersion: "praxis.harness/model-predispatch-surface-commit-gate/v1", ID: "gate-1", Revision: 1, Digest: core.DigestBytes([]byte("gate"))},
			prepared, current, assembly, surface, binding, NewInMemoryPreparedModelInvocationAckRepositoryV1(),
			func() time.Time { return time.Unix(0, fixture.current.ExpiresUnixNano) },
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := gate.Commit(context.Background(), fixture.fact.Ref(), fixture.current.Ref()); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
			t.Fatalf("expired Current must fail closed: %v", err)
		}
		if binding.inspectCalls.Load()+binding.ensureCalls.Load() != 0 {
			t.Fatal("expired Current reached Tool Binding Repository")
		}
	})

	t.Run("tool ensure lost reply", func(t *testing.T) {
		fixture := newGateFixtureV1(t)
		gate, _, _, _, _, binding := newGateV1(t, fixture)
		binding.loseEnsureReply = true
		ack, err := gate.Commit(context.Background(), fixture.fact.Ref(), fixture.current.Ref())
		if err != nil {
			t.Fatal(err)
		}
		if err := ack.Validate(); err != nil {
			t.Fatal(err)
		}
		if binding.ensureCalls.Load() != 1 || binding.inspectCalls.Load() != 2 {
			t.Fatalf("lost reply did not Inspect original Invocation: ensure=%d inspect=%d", binding.ensureCalls.Load(), binding.inspectCalls.Load())
		}
	})
}

var _ modelinvoker.PreparedModelInvocationCommitGateV1 = (*PreparedModelInvocationSurfaceCommitGateV1)(nil)
var _ toolcontract.ToolSurfaceInvocationBindingRepositoryV1 = (*gateBindingRepositoryV1)(nil)
