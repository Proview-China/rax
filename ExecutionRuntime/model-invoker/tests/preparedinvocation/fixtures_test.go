package preparedinvocation_test

import (
	"context"
	"sync/atomic"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func digest(label string) core.Digest { return core.DigestBytes([]byte(label)) }

func owner(domain, id string) core.OwnerRef {
	return core.OwnerRef{Domain: domain, ID: core.OwnerID(id)}
}

func registryRef() runtimeports.RegistrySnapshotRefV1 {
	return runtimeports.RegistrySnapshotRefV1{
		Owner:           owner("registry", "registry-owner"),
		ContractVersion: "1.0.0",
		ID:              "registry-snapshot-1",
		Revision:        1,
		Digest:          digest("registry"),
	}
}

func draftFact() modelinvoker.PreparedModelInvocationFactV1 {
	request := digest("unified-request")
	return modelinvoker.PreparedModelInvocationFactV1{
		InvocationID:                  "invocation-1",
		InvocationDigest:              request,
		UnifiedRequestDigest:          request,
		RequestToolsDigest:            digest("request-tools"),
		PreparedPlanDigest:            digest("plan"),
		RouteDigest:                   digest("route"),
		ProfileDigest:                 digest("profile"),
		ActualToolSurfaceDigest:       digest("surface"),
		ActualProviderInjectionDigest: digest("provider-injection"),
		CapabilitySnapshotRef: modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{
			ContractVersion: "1.0.0",
			ID:              "capability-snapshot-1",
			Revision:        1,
			Digest:          digest("capability"),
		},
		RegistrySnapshotRef: registryRef(),
		CreatedUnixNano:     1_000,
		NotAfterUnixNano:    10_000,
	}
}

func sealedFact() modelinvoker.PreparedModelInvocationFactV1 {
	fact, err := modelinvoker.SealPreparedModelInvocationFactV1(draftFact())
	if err != nil {
		panic(err)
	}
	return fact
}

func sealedCurrent(fact modelinvoker.PreparedModelInvocationFactV1) modelinvoker.PreparedModelInvocationCurrentProjectionV1 {
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(modelinvoker.PreparedModelInvocationCurrentProjectionV1{
		Prepared:                      fact.Ref(),
		CapabilitySnapshotRef:         fact.CapabilitySnapshotRef,
		RegistrySnapshotRef:           fact.RegistrySnapshotRef,
		ActualToolSurfaceDigest:       fact.ActualToolSurfaceDigest,
		ActualProviderInjectionDigest: fact.ActualProviderInjectionDigest,
		CheckedUnixNano:               2_000,
		ExpiresUnixNano:               8_000,
		NotAfterUnixNano:              fact.NotAfterUnixNano,
	})
	if err != nil {
		panic(err)
	}
	return current
}

func sealedAck(fact modelinvoker.PreparedModelInvocationFactV1, current modelinvoker.PreparedModelInvocationCurrentProjectionV1) modelinvoker.PreparedModelInvocationCommitAckV1 {
	ack, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(modelinvoker.PreparedModelInvocationCommitAckV1{
		PreparedRef: fact.Ref(),
		CurrentRef:  current.Ref(),
		GateImplementationRef: modelinvoker.PreparedModelInvocationGateImplementationRefV1{
			Owner:           owner("harness", "gate-owner"),
			ContractVersion: "1.0.0",
			ID:              "gate-implementation-1",
			Revision:        1,
			Digest:          digest("gate"),
		},
		SurfaceBindingRef: modelinvoker.PreparedModelInvocationSurfaceBindingRefV1{
			Owner:           owner("tool", "surface-owner"),
			ContractVersion: "1.0.0",
			ID:              "surface-binding-1",
			Revision:        1,
			Digest:          digest("binding"),
		},
		CheckedUnixNano:  3_000,
		ExpiresUnixNano:  7_000,
		NotAfterUnixNano: fact.NotAfterUnixNano,
	})
	if err != nil {
		panic(err)
	}
	return ack
}

type exactRegistryReader struct {
	ref   runtimeports.RegistrySnapshotRefV1
	err   error
	calls atomic.Uint64
}

func (r *exactRegistryReader) InspectExactRegistrySnapshotV1(context.Context, runtimeports.RegistrySnapshotRefV1) (runtimeports.RegistrySnapshotRefV1, error) {
	r.calls.Add(1)
	return r.ref, r.err
}
