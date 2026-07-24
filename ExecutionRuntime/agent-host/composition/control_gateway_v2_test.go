package composition_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/composition"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type controlReadersV2 struct {
	conformance contract.ControlAdapterConformanceV2
	resources   runtimeports.ResourceBindingSetV1
	handles     map[string]runtimeports.ResourceHandleCurrentV1
	driftAfter  int64
	reads       atomic.Int64
}

func (r *controlReadersV2) InspectControlAdapterConformanceV2(context.Context, contract.ControlAdapterFactoryRefV2) (contract.ControlAdapterConformanceV2, error) {
	value := r.conformance
	if r.driftAfter > 0 && r.reads.Add(1) > r.driftAfter {
		value.Digest = controlDigestV2("drift")
	}
	return value, nil
}
func (r *controlReadersV2) InspectResourceBindingSetCurrentV1(context.Context, runtimeports.ResourceBindingSetRefV1) (runtimeports.ResourceBindingSetV1, error) {
	return r.resources, nil
}
func (r *controlReadersV2) InspectResourceHandleCurrentV1(_ context.Context, expected runtimeports.ResourceHandleRefV1) (runtimeports.ResourceHandleCurrentV1, error) {
	return r.handles[expected.ID], nil
}

type controlHandleV2 struct {
	instance contract.ControlAdapterInstanceV2
}

func (h *controlHandleV2) InstanceV2() contract.ControlAdapterInstanceV2 { return h.instance }

type controlFactoryV2 struct {
	descriptor contract.ControlAdapterFactoryDescriptorV2
	instance   contract.ControlAdapterInstanceV2
	starts     atomic.Int64
	inspects   atomic.Int64
	lostOnce   atomic.Bool
}

func (f *controlFactoryV2) DescriptorV2() contract.ControlAdapterFactoryDescriptorV2 {
	return f.descriptor
}
func (f *controlFactoryV2) StartOrInspectControlAdapterV2(context.Context, contract.ControlAdapterConstructRequestV2) (ports.ControlAdapterHandleV2, error) {
	f.starts.Add(1)
	if f.lostOnce.CompareAndSwap(true, false) {
		return nil, contract.NewError(contract.ErrorUnknownOutcome, "lost_reply", "control adapter reply was lost")
	}
	return &controlHandleV2{instance: f.instance}, nil
}
func (f *controlFactoryV2) InspectControlAdapterV2(context.Context, contract.ControlAdapterConstructRequestV2) (ports.ControlAdapterHandleV2, error) {
	f.inspects.Add(1)
	return &controlHandleV2{instance: f.instance}, nil
}

func TestControlGatewayV2RereadsExactInputsAndRecoversLostReply(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	descriptor, conformance, resources, handles, request, instance := controlGatewayFixtureV2(t, now)
	factory := &controlFactoryV2{descriptor: descriptor, instance: instance}
	factory.lostOnce.Store(true)
	reg := registry.NewControlV2()
	if err := reg.RegisterControlAdapterFactoryV2(factory); err != nil {
		t.Fatal(err)
	}
	if err := reg.SealControlAdapterRegistryV2(); err != nil {
		t.Fatal(err)
	}
	readers := &controlReadersV2{conformance: conformance, resources: resources, handles: handles}
	clock := monotonicClockV2(now, time.Millisecond)
	gateway, err := composition.NewControlGatewayV2(reg, readers, readers, clock)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := gateway.StartOrInspectControlAdapterConstructionV2(context.Background(), request)
	if err != nil || actual.Digest != instance.Digest {
		t.Fatalf("actual=%+v err=%v", actual, err)
	}
	if factory.starts.Load() != 1 || factory.inspects.Load() != 1 {
		t.Fatalf("starts=%d inspects=%d", factory.starts.Load(), factory.inspects.Load())
	}
}

func TestControlGatewayV2S2DriftIsFailClosed(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	descriptor, conformance, resources, handles, request, instance := controlGatewayFixtureV2(t, now)
	factory := &controlFactoryV2{descriptor: descriptor, instance: instance}
	reg := registry.NewControlV2()
	_ = reg.RegisterControlAdapterFactoryV2(factory)
	_ = reg.SealControlAdapterRegistryV2()
	readers := &controlReadersV2{conformance: conformance, resources: resources, handles: handles, driftAfter: 1}
	gateway, _ := composition.NewControlGatewayV2(reg, readers, readers, monotonicClockV2(now, time.Millisecond))
	if _, err := gateway.StartOrInspectControlAdapterConstructionV2(context.Background(), request); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("S2 drift error=%v", err)
	}
}

func TestControlRegistryV2LinearizesOneOf64ExactRegistrations(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	descriptor, _, _, _, _, instance := controlGatewayFixtureV2(t, now)
	reg := registry.NewControlV2()
	var success atomic.Int64
	var conflict atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			factory := &controlFactoryV2{descriptor: descriptor, instance: instance}
			if err := reg.RegisterControlAdapterFactoryV2(factory); err == nil {
				success.Add(1)
			} else if contract.HasCode(err, contract.ErrorConflict) {
				conflict.Add(1)
			} else {
				t.Errorf("register error=%v", err)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 1 || conflict.Load() != 63 {
		t.Fatalf("success=%d conflict=%d", success.Load(), conflict.Load())
	}
}

func controlGatewayFixtureV2(t *testing.T, now time.Time) (contract.ControlAdapterFactoryDescriptorV2, contract.ControlAdapterConformanceV2, runtimeports.ResourceBindingSetV1, map[string]runtimeports.ResourceHandleCurrentV1, contract.ControlAdapterConstructRequestV2, contract.ControlAdapterInstanceV2) {
	t.Helper()
	expires := now.Add(time.Hour)
	owner := core.OwnerRef{Domain: "fixture.resources", ID: "owner-1"}
	ownerCurrent := func(id string) runtimeports.OwnerCurrentRefV1 {
		return runtimeports.OwnerCurrentRefV1{Owner: owner, ContractVersion: "1.0.0", ID: id, Revision: 1, Digest: controlDigestV2(id), ExpiresUnixNano: expires.UnixNano()}
	}
	cleanup, deployment := ownerCurrent("cleanup-1"), ownerCurrent("deployment-1")
	handle, err := runtimeports.SealResourceHandleCurrentV1(runtimeports.ResourceHandleCurrentV1{Ref: runtimeports.ResourceHandleRefV1{Owner: owner, ID: "resource/db-1", Revision: 1, Kind: "fixture/sqlite", ScopeDigest: controlDigestV2("scope")}, CleanupContract: cleanup, DeploymentAttestation: deployment, CheckedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	resources, err := runtimeports.SealResourceBindingSetV1(runtimeports.ResourceBindingSetV1{Ref: runtimeports.ResourceBindingSetRefV1{ID: "resource-set-1", Revision: 1}, Bindings: []runtimeports.ResourceBindingV1{{ComponentID: "fixture/control-adapter", Handle: handle.Ref, CleanupContract: cleanup, DeploymentAttestation: deployment}}, CheckedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := contract.SealControlAdapterFactoryDescriptorV2(contract.ControlAdapterFactoryDescriptorV2{Ref: contract.ControlAdapterFactoryRefV2{FactoryID: "factory/control-1", Revision: 1}, ComponentID: "fixture/control-adapter", ArtifactDigest: controlDigestV2("artifact"), ComponentContract: "1.0.0", Capability: "fixture/control", Binding: runtimeports.BindingAdmissionBindingRefV1{ComponentID: "fixture/control-adapter", ID: "binding-1", Revision: 1, Digest: controlDigestV2("binding"), ExpiresUnixNano: expires.UnixNano()}, Generation: runtimeports.OwnerCurrentRefV1{Owner: core.OwnerRef{Domain: "fixture.assembly", ID: "owner-2"}, ContractVersion: "1.0.0", ID: "generation-1", Revision: 1, Digest: controlDigestV2("generation"), ExpiresUnixNano: expires.UnixNano()}, ResourceBindingSet: resources.Ref, ResourceHandles: []runtimeports.ResourceHandleRefV1{handle.Ref}, OutputPortCapabilities: []runtimeports.CapabilityNameV2{"fixture/control-current"}, EffectClass: contract.ControlAdapterEffectNoneV2})
	if err != nil {
		t.Fatal(err)
	}
	evidenceOwner := core.OwnerRef{Domain: "fixture.certification", ID: "owner-3"}
	evidence := func(id string) runtimeports.OwnerCurrentRefV1 {
		return runtimeports.OwnerCurrentRefV1{Owner: evidenceOwner, ContractVersion: "1.0.0", ID: id, Revision: 1, Digest: controlDigestV2(id), ExpiresUnixNano: expires.UnixNano()}
	}
	conformance, err := contract.SealControlAdapterConformanceV2(contract.ControlAdapterConformanceV2{ConformanceID: "conformance-1", Revision: 1, DescriptorRef: descriptor.Ref, CertificationCurrent: evidence("certification"), StaticImportEvidence: evidence("imports"), NoRawProviderEvidence: evidence("no-provider"), ZeroEffectEvidence: evidence("zero-effect"), CheckedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	request, err := contract.SealControlAdapterConstructRequestV2(contract.ControlAdapterConstructRequestV2{HostID: "host-1", StartID: "start-1", AttemptID: "attempt-1", Descriptor: descriptor, Conformance: conformance, ResourceBindings: resources, RequestedNotAfterUnixNano: now.Add(45 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := contract.SealControlAdapterInstanceV2(contract.ControlAdapterInstanceV2{InstanceRef: contract.ExactRefV1{Kind: "praxis.agent-host/control-adapter-instance", ID: "instance-1", Revision: 1, Digest: contract.DigestV1(controlDigestV2("instance"))}, AttemptID: request.AttemptID, RequestDigest: request.RequestDigest, DescriptorRef: descriptor.Ref, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return descriptor, conformance, resources, map[string]runtimeports.ResourceHandleCurrentV1{handle.Ref.ID: handle}, request, instance
}

func monotonicClockV2(start time.Time, step time.Duration) func() time.Time {
	var mu sync.Mutex
	now := start
	return func() time.Time { mu.Lock(); defer mu.Unlock(); current := now; now = now.Add(step); return current }
}

func controlDigestV2(value string) core.Digest {
	digest, _ := core.CanonicalJSONDigest("fixture", "1.0.0", "Fixture", value)
	return digest
}
