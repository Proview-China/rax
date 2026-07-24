package testkit

import (
	"time"

	modelcontract "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func SurfaceInvocationRegistrySnapshotV1() runtimeports.RegistrySnapshotRefV1 {
	return runtimeports.RegistrySnapshotRefV1{
		Owner: core.OwnerRef{Domain: "registry", ID: "registry-owner"}, ContractVersion: "1.0.0",
		ID: "surface-registry-snapshot", Revision: 1, Digest: Digest("surface-registry"),
	}
}

func PreparedSurfaceInvocationFactV1() modelcontract.PreparedModelInvocationFactV1 {
	surface := ToolSurfaceManifestCurrentProjectionV1(1)
	request := Digest("surface-invocation-request")
	fact, err := modelcontract.SealPreparedModelInvocationFactV1(modelcontract.PreparedModelInvocationFactV1{
		InvocationID: "surface-invocation-1", InvocationDigest: request, UnifiedRequestDigest: request,
		RequestToolsDigest: Digest("surface-request-tools"), PreparedPlanDigest: Digest("surface-prepared-plan"),
		RouteDigest: Digest("surface-route"), ProfileDigest: surface.Manifest.ProfileDigest,
		ActualToolSurfaceDigest: surface.Manifest.ExpectedInjectionDigest, ActualProviderInjectionDigest: Digest("provider-injection"),
		CapabilitySnapshotRef: modelcontract.PreparedModelInvocationCapabilitySnapshotRefV1{
			ContractVersion: "1.0.0", ID: "surface-capability-snapshot", Revision: 1, Digest: Digest("capability-snapshot"),
		},
		RegistrySnapshotRef: SurfaceInvocationRegistrySnapshotV1(),
		CreatedUnixNano:     FixedTime.Add(-time.Minute).UnixNano(), NotAfterUnixNano: FixedTime.Add(30 * time.Minute).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return fact
}

func PreparedSurfaceInvocationCurrentV1(fact modelcontract.PreparedModelInvocationFactV1) modelcontract.PreparedModelInvocationCurrentProjectionV1 {
	current, err := modelcontract.SealPreparedModelInvocationCurrentV1(modelcontract.PreparedModelInvocationCurrentProjectionV1{
		Prepared: fact.Ref(), CapabilitySnapshotRef: fact.CapabilitySnapshotRef, RegistrySnapshotRef: fact.RegistrySnapshotRef,
		ActualToolSurfaceDigest: fact.ActualToolSurfaceDigest, ActualProviderInjectionDigest: fact.ActualProviderInjectionDigest,
		CheckedUnixNano: FixedTime.UnixNano(), ExpiresUnixNano: FixedTime.Add(20 * time.Minute).UnixNano(), NotAfterUnixNano: fact.NotAfterUnixNano,
	})
	if err != nil {
		panic(err)
	}
	return current
}

func ModelPreDispatchAssemblyCurrentV1(surface toolcontract.ToolSurfaceManifestCurrentProjectionV1, fact modelcontract.PreparedModelInvocationFactV1) runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1 {
	now := FixedTime
	projection, err := runtimeports.SealModelPreDispatchAssemblyCurrentProjectionV1(runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{
		Ref: runtimeports.ModelPreDispatchAssemblyCurrentRefV1{Revision: 1},
		Generation: runtimeports.GenerationArtifactRefV1{
			ID: "surface-generation", Revision: 1, Digest: Digest("generation"), InputDigest: Digest("generation-input"),
			ManifestDigest: Digest("generation-manifest"), GraphDigest: Digest("generation-graph"), CatalogDigest: Digest("generation-catalog"),
		},
		Handoff: runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: "surface-handoff", Revision: 1, Digest: Digest("handoff")},
		BindingSet: runtimeports.ModelPreDispatchAssemblyBindingSetRefV1{
			ID: "surface-binding-set", Revision: 1, Digest: Digest("binding-set"), SemanticDigest: Digest("binding-semantic"),
			CurrentnessDigest: Digest("binding-currentness"), ProjectionDigest: Digest("binding-projection"), ExpiresUnixNano: now.Add(25 * time.Minute).UnixNano(),
		},
		Manifest:      runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: "surface-assembly-manifest", Revision: 1, Digest: Digest("assembly-manifest")},
		Conformance:   runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: "surface-conformance", Revision: 1, Digest: Digest("conformance")},
		ToolSurface:   runtimeports.ModelPreDispatchAssemblyExactRefV1{ID: surface.Ref.ID, Revision: surface.Ref.Revision, Digest: surface.Ref.Digest},
		ProfileDigest: fact.ProfileDigest, RegistrySnapshot: fact.RegistrySnapshotRef,
		SemanticDigest: Digest("assembly-semantic"), CurrentnessDigest: Digest("assembly-currentness"),
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(15 * time.Minute).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return projection
}

func ToolSurfaceInvocationBindingRequestV1() toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1 {
	surface := ToolSurfaceManifestCurrentProjectionV1(1)
	fact := PreparedSurfaceInvocationFactV1()
	current := PreparedSurfaceInvocationCurrentV1(fact)
	assembly := ModelPreDispatchAssemblyCurrentV1(surface, fact)
	return toolcontract.ToolSurfaceInvocationBindingEnsureRequestV1{
		Invocation:      toolcontract.ToolSurfaceInvocationCoordinateV1{InvocationID: fact.InvocationID, InvocationDigest: fact.InvocationDigest},
		PreparedFactRef: fact.Ref(), PreparedHistoricalFact: fact, PreparedCurrentRef: current.Ref(), PreparedCurrent: current,
		SurfaceCurrent: surface, AssemblyCurrentRef: assembly.Ref, AssemblyRegistrySnapshot: fact.RegistrySnapshotRef,
		AssemblyCurrent: assembly, RequestedNotAfterUnixNano: FixedTime.Add(10 * time.Minute).UnixNano(),
	}
}
