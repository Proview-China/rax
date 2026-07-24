package applicationadapter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

func TestToolInputContractCurrentResolverV1ResolveInspectAndFailClosed(t *testing.T) {
	resolver, clock, store, request := inputContractResolverFixtureV1(t)
	ctx := context.Background()
	winner, err := resolver.ResolveToolInputContractCurrentV1(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if err = winner.ValidateAgainst(request, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	byIssuance, err := resolver.InspectToolInputContractCurrentByIssuanceV1(ctx, toolcontract.ToolInputContractInspectByIssuanceRequestV1{ResolveRequest: request})
	if err != nil || byIssuance.Ref != winner.Ref || byIssuance.CheckedUnixNano != winner.CheckedUnixNano {
		t.Fatalf("inspect by issuance did not return immutable winner: %v", err)
	}
	exact, err := resolver.InspectExactToolInputContractCurrentV1(ctx, toolcontract.ToolInputContractInspectExactRequestV1{ResolveRequest: request, Expected: winner.Ref})
	if err != nil || exact.ProjectionDigest != winner.ProjectionDigest {
		t.Fatalf("exact inspect did not return winner: %v", err)
	}

	attack := request
	attack.CallName = "tool.other"
	if _, err = resolver.ResolveToolInputContractCurrentV1(ctx, attack); err == nil {
		t.Fatal("unknown Surface call name was accepted")
	}
	_, attackID, err := inputContractIssuanceIDV1(attack)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.InspectToolInputContractCurrentByIssuanceIDV1(ctx, attackID); err == nil || !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed resolve wrote a lease: %v", err)
	}

	clock.Set(time.Unix(0, winner.ExpiresUnixNano))
	if _, err = resolver.InspectExactToolInputContractCurrentV1(ctx, toolcontract.ToolInputContractInspectExactRequestV1{ResolveRequest: request, Expected: winner.Ref}); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("expired immutable lease remained current: %v", err)
	}
}

func TestToolInputContractCurrentResolverV1LostCreateReplyInspectsWinner(t *testing.T) {
	base, clock, _, request := inputContractResolverFixtureV1(t)
	fault := &lostReplyInputContractStoreV1{inner: NewInMemoryToolInputContractLeaseStoreV1()}
	resolver, err := NewToolInputContractCurrentResolverV1(base.surface, base.registry, fault, clock)
	if err != nil {
		t.Fatal(err)
	}
	winner, err := resolver.ResolveToolInputContractCurrentV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if fault.creates.Load() != 1 || fault.inspects.Load() == 0 || winner.Ref.ID == "" {
		t.Fatalf("lost reply recovery did not inspect the one winner: creates=%d inspects=%d", fault.creates.Load(), fault.inspects.Load())
	}
}

func TestToolInputContractCurrentResolverV1ConcurrentSameIssuance(t *testing.T) {
	resolver, _, _, request := inputContractResolverFixtureV1(t)
	const workers = 64
	var wg sync.WaitGroup
	refs := make(chan toolcontract.ToolInputContractCurrentRefV1, workers)
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			winner, err := resolver.ResolveToolInputContractCurrentV1(context.Background(), request)
			if err == nil {
				refs <- winner.Ref
			}
			errs <- err
		}()
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
			t.Fatalf("same issuance returned multiple refs: %#v != %#v", ref, winner)
		}
	}
}

func TestToolInputContractCurrentResolverV1TypedNilAndCanceled(t *testing.T) {
	resolver, _, _, request := inputContractResolverFixtureV1(t)
	var nilSurface *surface.InMemoryToolSurfaceManifestCurrentRepositoryV1
	if _, err := NewToolInputContractCurrentResolverV1(nilSurface, resolver.registry, resolver.store, resolver.clock); err == nil {
		t.Fatal("typed-nil Surface Reader was accepted")
	}
	if _, err := resolver.ResolveToolInputContractCurrentV1(nil, request); err == nil {
		t.Fatal("nil context was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver.ResolveToolInputContractCurrentV1(ctx, request); err != context.Canceled {
		t.Fatalf("canceled context was not preserved: %v", err)
	}
}

func inputContractResolverFixtureV1(t *testing.T) (*ToolInputContractCurrentResolverV1, *testkit.ManualClock, *InMemoryToolInputContractLeaseStoreV1, toolcontract.ToolInputContractResolveRequestV1) {
	t.Helper()
	clock := testkit.NewManualClock(testkit.FixedTime)
	surfaceRepository, err := surface.NewInMemoryToolSurfaceManifestCurrentRepositoryV1(clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	surfaceCurrent, err := surfaceRepository.EnsureExactToolSurfaceManifestCurrentV1(context.Background(), testkit.ToolSurfaceManifestCurrentRequestV1(1))
	if err != nil {
		t.Fatal(err)
	}
	registryReader, _, capability, tool := registryCurrentFixtureV1(t)
	store := NewInMemoryToolInputContractLeaseStoreV1()
	resolver, err := NewToolInputContractCurrentResolverV1(surfaceRepository, registryReader, store, clock)
	if err != nil {
		t.Fatal(err)
	}
	provider := runtimeports.ProviderBindingRefV2{
		BindingSetID: "binding-set-1", BindingSetRevision: 1,
		ComponentID: testkit.SettlementOwner().ComponentID, ManifestDigest: testkit.SettlementOwner().ManifestDigest,
		ArtifactDigest: tool.ArtifactDigest, Capability: runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3),
	}
	request := toolcontract.ToolInputContractResolveRequestV1{
		ApplicationRequestID: "application-request-1", ApplicationRequestRevision: 1, ApplicationRequestDigest: testkit.Digest("application-request"),
		PendingAction:        toolcontract.PendingActionExactRefV2{ID: "pending-action-1", Revision: 1, RequestDigest: testkit.Digest("pending-action")},
		OperationScopeDigest: testkit.Digest("operation-scope"), ProviderBinding: provider, ExpectedOwner: testkit.SettlementOwner(),
		Surface:    toolcontract.ObjectRef{ID: surfaceCurrent.Manifest.ID, Revision: surfaceCurrent.Manifest.Revision, Digest: surfaceCurrent.Manifest.Digest},
		CallName:   surfaceCurrent.Manifest.Entries[0].ModelName,
		Capability: toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest},
		Tool:       toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}, InputSchema: tool.InputSchema,
		RequestedExpiresUnixNano: testkit.FixedTime.Add(10 * time.Second).UnixNano(),
	}
	return resolver, clock, store, request
}

type lostReplyInputContractStoreV1 struct {
	inner    *InMemoryToolInputContractLeaseStoreV1
	creates  atomic.Uint64
	inspects atomic.Uint64
}

func (s *lostReplyInputContractStoreV1) CreateToolInputContractCurrentOnceV1(ctx context.Context, projection toolcontract.ToolInputContractCurrentProjectionV1) (toolcontract.ToolInputContractCurrentProjectionV1, error) {
	s.creates.Add(1)
	if _, err := s.inner.CreateToolInputContractCurrentOnceV1(ctx, projection); err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	return toolcontract.ToolInputContractCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "simulated lost create reply")
}

func (s *lostReplyInputContractStoreV1) InspectToolInputContractCurrentByIssuanceIDV1(ctx context.Context, id string) (toolcontract.ToolInputContractCurrentProjectionV1, error) {
	s.inspects.Add(1)
	return s.inner.InspectToolInputContractCurrentByIssuanceIDV1(ctx, id)
}

func (s *lostReplyInputContractStoreV1) InspectExactToolInputContractCurrentV1(ctx context.Context, ref toolcontract.ToolInputContractCurrentRefV1) (toolcontract.ToolInputContractCurrentProjectionV1, error) {
	return s.inner.InspectExactToolInputContractCurrentV1(ctx, ref)
}
