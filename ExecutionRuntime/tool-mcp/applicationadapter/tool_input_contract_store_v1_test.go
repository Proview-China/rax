package applicationadapter

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestToolInputContractLeaseStoreV1CreateInspectAndDeepCopy(t *testing.T) {
	store := NewInMemoryToolInputContractLeaseStoreV1()
	projection := toolInputContractProjectionFixtureV1(t, testkit.FixedTime)
	created, err := store.CreateToolInputContractCurrentOnceV1(context.Background(), projection)
	if err != nil {
		t.Fatal(err)
	}
	created.SurfaceCurrent.Manifest.Entries[0].ModelName = "tampered"
	inspected, err := store.InspectExactToolInputContractCurrentV1(context.Background(), projection.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if inspected.SurfaceCurrent.Manifest.Entries[0].ModelName != projection.SurfaceCurrent.Manifest.Entries[0].ModelName {
		t.Fatal("store leaked a caller-owned nested Surface entry")
	}
	drift := projection.Ref
	drift.Digest = testkit.Digest("same-id-drift")
	if _, err = store.InspectExactToolInputContractCurrentV1(context.Background(), drift); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same ID with another digest was not Conflict: %v", err)
	}
}

func TestToolInputContractLeaseStoreV1ConcurrentSameIssuanceSingleWinner(t *testing.T) {
	store := NewInMemoryToolInputContractLeaseStoreV1()
	const workers = 64
	projections := make([]toolcontract.ToolInputContractCurrentProjectionV1, workers)
	for index := range workers {
		projections[index] = toolInputContractProjectionFixtureV1(t, testkit.FixedTime.Add(time.Duration(index)*time.Microsecond))
	}
	refs := make(chan toolcontract.ToolInputContractCurrentRefV1, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for index := range workers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			winner, err := store.CreateToolInputContractCurrentOnceV1(context.Background(), projections[index])
			if err == nil {
				refs <- winner.Ref
			}
			errs <- err
		}(index)
	}
	wg.Wait()
	close(refs)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var winner toolcontract.ToolInputContractCurrentRefV1
	for ref := range refs {
		if winner == (toolcontract.ToolInputContractCurrentRefV1{}) {
			winner = ref
		} else if ref != winner {
			t.Fatalf("same issuance returned multiple winners: %#v != %#v", ref, winner)
		}
	}
	if winner.ID == "" {
		t.Fatal("no create-once winner")
	}
}

func TestToolInputContractLeaseStoreV1FailClosedBoundaries(t *testing.T) {
	projection := toolInputContractProjectionFixtureV1(t, testkit.FixedTime)
	store := NewInMemoryToolInputContractLeaseStoreV1()
	if _, err := store.CreateToolInputContractCurrentOnceV1(nil, projection); err == nil {
		t.Fatal("nil context was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.CreateToolInputContractCurrentOnceV1(ctx, projection); err != context.Canceled {
		t.Fatalf("canceled context was not preserved: %v", err)
	}
	var typedNil *InMemoryToolInputContractLeaseStoreV1
	if _, err := typedNil.CreateToolInputContractCurrentOnceV1(context.Background(), projection); err == nil || !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("typed-nil store was not unavailable: %v", err)
	}
	if _, err := store.InspectToolInputContractCurrentByIssuanceIDV1(context.Background(), projection.Ref.ID); err == nil || !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("absent issuance was not authoritative NotFound: %v", err)
	}
}

func toolInputContractProjectionFixtureV1(t *testing.T, checked time.Time) toolcontract.ToolInputContractCurrentProjectionV1 {
	t.Helper()
	reader, _, capability, tool := registryCurrentFixtureV1(t)
	capObject := toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
	toolObject := toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}
	_, capCurrent, err := reader.ResolveExactToolCapabilityCurrentV1(context.Background(), capObject)
	if err != nil {
		t.Fatal(err)
	}
	_, toolCurrent, err := reader.ResolveExactToolDescriptorCurrentV1(context.Background(), toolObject)
	if err != nil {
		t.Fatal(err)
	}
	surface := testkit.ToolSurfaceManifestCurrentProjectionV1(1)
	entry := surface.Manifest.Entries[0]
	policy, err := toolcontract.DeriveToolInputLimitPolicyV1(capObjectForSurfaceV1(surface), entry.Order, entry, capObject, toolObject, tool.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	provider := runtimeports.ProviderBindingRefV2{
		BindingSetID: "binding-set-1", BindingSetRevision: 1,
		ComponentID: testkit.SettlementOwner().ComponentID, ManifestDigest: testkit.SettlementOwner().ManifestDigest,
		ArtifactDigest: tool.ArtifactDigest, Capability: runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3),
	}
	subject, err := toolcontract.SealToolInputContractBindingSubjectV1(toolcontract.ToolInputContractBindingSubjectV1{
		ApplicationRequestID: "application-request-1", ApplicationRequestRevision: 1, ApplicationRequestDigest: testkit.Digest("application-request"),
		PendingAction:        toolcontract.PendingActionExactRefV2{ID: "pending-action-1", Revision: 1, RequestDigest: testkit.Digest("pending-action")},
		OperationScopeDigest: testkit.Digest("operation-scope"), ProviderBinding: provider, ExpectedOwner: testkit.SettlementOwner(),
		SurfaceOwner: surface.Owner, CapabilityRegistryOwner: capability.Owner, ToolRegistryOwner: tool.Owner,
		Surface: capObjectForSurfaceV1(surface), SurfaceEntryOrdinal: entry.Order, SurfaceEntry: entry,
		Capability: capObject, Tool: toolObject, ToolArtifactDigest: tool.ArtifactDigest, InputSchema: tool.InputSchema, LimitPolicy: policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	requested := testkit.FixedTime.Add(10 * time.Second).UnixNano()
	resolveRequest := toolcontract.ToolInputContractResolveRequestV1{
		ApplicationRequestID: subject.ApplicationRequestID, ApplicationRequestRevision: subject.ApplicationRequestRevision, ApplicationRequestDigest: subject.ApplicationRequestDigest,
		PendingAction: subject.PendingAction, OperationScopeDigest: subject.OperationScopeDigest, ProviderBinding: subject.ProviderBinding, ExpectedOwner: subject.ExpectedOwner,
		Surface: subject.Surface, CallName: subject.SurfaceEntry.ModelName, Capability: subject.Capability, Tool: subject.Tool, InputSchema: subject.InputSchema,
		RequestedExpiresUnixNano: requested,
	}
	issuance, err := toolcontract.ToolInputContractIssuanceFromResolveRequestV1(resolveRequest)
	if err != nil {
		t.Fatal(err)
	}
	expires := requested
	if upper := checked.Add(toolcontract.MaxToolInputContractCurrentTTLV1).UnixNano(); upper < expires {
		expires = upper
	}
	schemaCurrent, err := toolcontract.SealToolInputSchemaCurrentRefV1(toolcontract.ToolInputSchemaCurrentRefV1{
		InputSchema: tool.InputSchema, Authority: toolCurrent.Ref, RegistryOwner: tool.Owner,
		CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := toolcontract.SealToolInputContractCurrentV1(toolcontract.ToolInputContractCurrentProjectionV1{
		IssuanceSubject: issuance, BindingSubject: subject, SurfaceCurrent: surface, CapabilityCurrent: capCurrent, ToolCurrent: toolCurrent,
		InputSchemaCurrent: schemaCurrent, RequestedExpiresUnixNano: requested, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

func capObjectForSurfaceV1(surface toolcontract.ToolSurfaceManifestCurrentProjectionV1) toolcontract.ObjectRef {
	return toolcontract.ObjectRef{ID: surface.Manifest.ID, Revision: surface.Manifest.Revision, Digest: surface.Manifest.Digest}
}
