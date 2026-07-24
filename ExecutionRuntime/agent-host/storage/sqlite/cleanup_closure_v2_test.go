package sqlite

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCleanupClosureV2EnsureLostReplyRestartAndEmbeddedPlan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "closure.db")
	store := openTestStore(t, path)
	claim := claimFixture(t, "host-closure", "start-closure", "config-closure")
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	fact := cleanupClosureFixtureV2(t, claim)
	store.loseNextReplyForTest()
	if _, err := store.EnsureHostCleanupClosureV2(context.Background(), fact); !contract.HasCode(err, contract.ErrorUnknownOutcome) {
		t.Fatalf("lost reply error=%v", err)
	}
	ref, _ := fact.RefV2()
	got, err := store.InspectHostCleanupClosureV2(context.Background(), ref)
	if err != nil || got.ContentDigest != fact.ContentDigest {
		t.Fatalf("inspect after lost reply: %+v %v", got, err)
	}
	byStart, err := store.InspectHostCleanupClosureForStartV2(context.Background(), fact.Plan.HostID, fact.Plan.StartID)
	if err != nil || byStart.ContentDigest != fact.ContentDigest {
		t.Fatalf("by-start inspect: %+v %v", byStart, err)
	}
	planRef, _ := fact.Plan.RefV2()
	plan, err := store.InspectCleanupPlanV2(context.Background(), planRef)
	if err != nil || plan.Digest != fact.Plan.Digest {
		t.Fatalf("embedded Plan inspect: %+v %v", plan, err)
	}
	got.Coverage[0].SourceID = "mutated-return"
	again, err := store.InspectHostCleanupClosureV2(context.Background(), ref)
	if err != nil || again.Coverage[0].SourceID == "mutated-return" {
		t.Fatalf("store returned aliased Closure: %+v %v", again, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path)
	defer reopened.Close()
	if recovered, err := reopened.InspectHostCleanupClosureV2(context.Background(), ref); err != nil || recovered.ContentDigest != fact.ContentDigest {
		t.Fatalf("restart recovery: %+v %v", recovered, err)
	}
}

func TestCleanupClosureV2ChangedContentAndExactInspectConflict(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "closure-conflict.db"))
	defer store.Close()
	claim := claimFixture(t, "host-closure-conflict", "start-closure-conflict", "config")
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	fact := cleanupClosureFixtureV2(t, claim)
	if _, err := store.EnsureHostCleanupClosureV2(context.Background(), fact); err != nil {
		t.Fatal(err)
	}
	changed := fact
	changed.CreatedUnixNano++
	changed.ContentDigest = ""
	changed, err := contract.SealHostCleanupClosureFactV2(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.EnsureHostCleanupClosureV2(context.Background(), changed); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("changed ensure error=%v", err)
	}
	ref, _ := fact.RefV2()
	ref.Digest = digestV1(t, "wrong")
	if _, err = store.InspectHostCleanupClosureV2(context.Background(), ref); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("wrong exact Ref error=%v", err)
	}
	actual, err := store.InspectHostCleanupClosureForStartV2(context.Background(), fact.Plan.HostID, fact.Plan.StartID)
	if err != nil || actual.ContentDigest != fact.ContentDigest {
		t.Fatalf("original changed: %+v %v", actual, err)
	}
}

func TestCleanupClosureV2Across64StoreHandlesLinearizesOneFact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "closure-64.db")
	seed := openTestStore(t, path)
	claim := claimFixture(t, "host-closure-64", "start-closure-64", "config")
	if _, err := seed.ClaimOrInspectHostStartV1(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	fact := cleanupClosureFixtureV2(t, claim)
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int64
	var failures atomic.Int64
	var wg sync.WaitGroup
	ref, _ := fact.RefV2()
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store := openTestStore(t, path)
			defer store.Close()
			for attempt := 0; attempt < 64; attempt++ {
				got, err := store.EnsureHostCleanupClosureV2(context.Background(), fact)
				if err == nil {
					if got.ContentDigest == fact.ContentDigest { successes.Add(1) } else { failures.Add(1) }
					return
				}
				if !contract.HasCode(err, contract.ErrorUnavailable) && !contract.HasCode(err, contract.ErrorUnknownOutcome) {
					failures.Add(1)
					return
				}
				// A transient write failure or unknown reply is recovered by exact
				// Inspect first. Only authoritative NotFound/Unavailable permits a
				// bounded replay of the same canonical Ensure command.
				inspected, inspectErr := store.InspectHostCleanupClosureV2(context.Background(), ref)
				if inspectErr == nil {
					if inspected.ContentDigest == fact.ContentDigest { successes.Add(1) } else { failures.Add(1) }
					return
				}
				if !contract.HasCode(inspectErr, contract.ErrorNotFound) && !contract.HasCode(inspectErr, contract.ErrorUnavailable) {
					failures.Add(1)
					return
				}
				time.Sleep(time.Duration(attempt+1) * time.Millisecond)
			}
			failures.Add(1)
		}()
	}
	wg.Wait()
	if successes.Load() != 64 || failures.Load() != 0 {
		t.Fatalf("idempotent successes=%d failures=%d want 64/0", successes.Load(), failures.Load())
	}
	store := openTestStore(t, path)
	defer store.Close()
	got, err := store.InspectHostCleanupClosureForStartV2(context.Background(), fact.Plan.HostID, fact.Plan.StartID)
	if err != nil || got.ContentDigest != fact.ContentDigest {
		t.Fatalf("final Closure=%+v %v", got, err)
	}
}

func TestCleanupClosureV2RequiresExactPermanentClaim(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "closure-claim.db"))
	defer store.Close()
	claim := claimFixture(t, "host-closure-claim", "start-closure-claim", "config")
	fact := cleanupClosureFixtureV2(t, claim)
	if _, err := store.EnsureHostCleanupClosureV2(context.Background(), fact); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("missing claim error=%v", err)
	}
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	fact.StartClaimRef.Digest = digestV1(t, "wrong-claim")
	fact.ContentDigest = ""
	fact, err := contract.SealHostCleanupClosureFactV2(fact)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.EnsureHostCleanupClosureV2(context.Background(), fact); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("wrong claim error=%v", err)
	}
}

func cleanupClosureFixtureV2(t *testing.T, claim contract.HostStartClaimV1) contract.HostCleanupClosureFactV2 {
	t.Helper()
	now := testNow
	expires := now.Add(time.Hour).UnixNano()
	claimRef, err := claim.CurrentRefV1()
	if err != nil {
		t.Fatal(err)
	}
	ownerCurrent := func(domain, id string) runtimeports.OwnerCurrentRefV1 {
		return runtimeports.OwnerCurrentRefV1{Owner: core.OwnerRef{Domain: domain, ID: core.OwnerID("owner-" + id)}, ContractVersion: "1.0.0", ID: id, Revision: 1, Digest: coreDigest(t, domain+":"+id), ExpiresUnixNano: expires}
	}
	component := runtimeports.ComponentIDV2("fixture/control")
	binding := runtimeports.BindingAdmissionBindingRefV1{ComponentID: component, ID: "binding-control", Revision: 1, Digest: coreDigest(t, "binding"), ExpiresUnixNano: expires}
	resourceSet := runtimeports.ResourceBindingSetRefV1{ID: "resource-set", Revision: 1, Digest: coreDigest(t, "resource-set"), ExpiresUnixNano: expires}
	handle := runtimeports.ResourceHandleRefV1{Owner: core.OwnerRef{Domain: "fixture.resources", ID: "owner-resource"}, ID: "resource-control", Revision: 1, Digest: coreDigest(t, "resource"), Kind: "fixture/resource", ScopeDigest: coreDigest(t, "scope"), ExpiresUnixNano: expires}
	generation := ownerCurrent("fixture.assembly", "generation")
	factory := contract.ControlAdapterFactoryRefV2{FactoryID: "factory/control", Revision: 1, Digest: coreDigest(t, "factory")}
	controlNode := cleanupClosureNodeV2(t, "control-cleanup", contract.CleanupOwnerNodeV2, string(component), contract.CleanupLiveExecutionV2)
	harness := cleanupClosureNodeV2(t, contract.CleanupBarrierHarnessCloseV2, contract.CleanupBarrierNodeV2, contract.CleanupBarrierOwnerHarnessV2, contract.CleanupLiveExecutionV2, controlNode.NodeID)
	fence := cleanupClosureNodeV2(t, contract.CleanupBarrierSandboxFenceV2, contract.CleanupBarrierNodeV2, contract.CleanupBarrierOwnerSandboxV2, contract.CleanupFencedSandboxLeaseV2, harness.NodeID)
	release := cleanupClosureNodeV2(t, contract.CleanupBarrierSandboxReleaseV2, contract.CleanupBarrierNodeV2, contract.CleanupBarrierOwnerSandboxV2, contract.CleanupFencedSandboxLeaseV2, fence.NodeID)
	aggregate := cleanupClosureNodeV2(t, contract.CleanupBarrierRuntimeCleanupAggregateV2, contract.CleanupBarrierNodeV2, contract.CleanupBarrierOwnerRuntimeV2, contract.CleanupHostControlHandleV2, release.NodeID)
	plan, err := contract.SealCleanupPlanV2(contract.CleanupPlanV2{ContractVersion: contract.CleanupContractVersionV2, PlanID: "plan/" + claim.StartID, Revision: 1, HostID: claim.HostID, StartID: claim.StartID, Nodes: []contract.CleanupNodeV2{controlNode, harness, fence, release, aggregate}})
	if err != nil {
		t.Fatal(err)
	}
	route := contract.HostCleanupPlanTemplateRouteV2{NodeID: controlNode.NodeID, FactoryRef: factory, ComponentID: component, ArtifactDigest: coreDigest(t, "artifact"), Capability: "fixture/control-cleanup", Binding: binding, CleanupContractRef: controlNode.CleanupContractRef, InspectPortBinding: controlNode.InspectPortBinding, RequestSchemaDigest: controlNode.RequestSchemaDigest, ResultSchemaDigest: controlNode.ResultSchemaDigest, ResourceClass: controlNode.ResourceClass}
	template, err := contract.SealHostCleanupPlanTemplateCurrentV2(contract.HostCleanupPlanTemplateCurrentV2{TemplateRef: contract.ExactRefV1{Kind: contract.HostCleanupPlanTemplateRefKindV2, ID: "template/" + claim.StartID, Revision: 1}, Routes: []contract.HostCleanupPlanTemplateRouteV2{route}, FixedBarriers: []contract.CleanupNodeV2{harness, fence, release, aggregate}, ResourceBindingSet: resourceSet, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	control := contract.HostCleanupClosureControlCoverageV2{FactoryRef: factory, ComponentID: component, ArtifactDigest: route.ArtifactDigest, Capability: route.Capability, Binding: binding, Generation: generation, ResourceBindingSet: resourceSet, ResourceHandles: []runtimeports.ResourceHandleRefV1{handle}, CleanupNodeIDs: []string{controlNode.NodeID}}
	coverage := []contract.HostCleanupClosureCoverageEntryV2{
		{SourceKind: contract.HostCleanupCoverageBindingV2, SourceID: binding.ID, SourceRevision: uint64(binding.Revision), SourceDigest: contract.DigestV1(binding.Digest), ComponentID: string(component), ResourceClass: controlNode.ResourceClass, CleanupNodeID: controlNode.NodeID},
		{SourceKind: contract.HostCleanupCoverageControlV2, SourceID: factory.FactoryID, SourceRevision: uint64(factory.Revision), SourceDigest: contract.DigestV1(factory.Digest), ComponentID: string(component), ResourceClass: controlNode.ResourceClass, CleanupNodeID: controlNode.NodeID},
		{SourceKind: contract.HostCleanupCoverageResourceV2, SourceID: handle.ID, SourceRevision: uint64(handle.Revision), SourceDigest: contract.DigestV1(handle.Digest), ComponentID: string(component), ResourceClass: controlNode.ResourceClass, CleanupNodeID: controlNode.NodeID},
	}
	for _, node := range []contract.CleanupNodeV2{harness, fence, release, aggregate} {
		coverage = append(coverage, contract.HostCleanupClosureCoverageEntryV2{SourceKind: contract.HostCleanupCoverageBarrierV2, SourceID: node.NodeID, SourceRevision: 1, SourceDigest: node.Digest, ComponentID: node.OwnerComponentID, ResourceClass: node.ResourceClass, CleanupNodeID: node.NodeID})
	}
	assembly := contract.HostCleanupClosureAssemblyCoordinateV2{ScopeRef: "scope/" + claim.StartID, AssemblyInput: exactRef(t, "praxis.harness/assembly-input", "input"), Publication: exactRef(t, "praxis.harness/publication", "publication"), Generation: exactRef(t, "praxis.harness/generation", "generation"), Manifest: exactRef(t, "praxis.harness/manifest", "manifest"), Graph: exactRef(t, "praxis.harness/graph", "graph"), Handoff: exactRef(t, "praxis.harness/handoff", "handoff"), OwnerCurrent: generation}
	fact, err := contract.SealHostCleanupClosureFactV2(contract.HostCleanupClosureFactV2{Revision: 1, StartClaimRef: claimRef, Assembly: assembly, Binding: contract.HostCleanupClosureBindingCoordinateV2{AttemptID: "binding-attempt", RequestDigest: coreDigest(t, "binding-request"), BindingSet: runtimeports.BindingAdmissionBindingSetRefV1{ID: "binding-set", Revision: 1, Digest: coreDigest(t, "binding-set"), ExpiresUnixNano: expires}, Bindings: []runtimeports.BindingAdmissionBindingRefV1{binding}, ResourceBindingSet: resourceSet, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires, ResultDigest: coreDigest(t, "binding-result")}, PlanTemplate: template, Controls: []contract.HostCleanupClosureControlCoverageV2{control}, Plan: plan, Coverage: coverage, CreatedUnixNano: now.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func cleanupClosureNodeV2(t *testing.T, id string, kind contract.CleanupNodeKindV2, owner string, class contract.CleanupResourceClassV2, deps ...string) contract.CleanupNodeV2 {
	t.Helper()
	node, err := contract.SealCleanupNodeV2(contract.CleanupNodeV2{NodeID: id, Kind: kind, OwnerComponentID: owner, CleanupContractRef: exactRef(t, "fixture/cleanup-contract", "cleanup-"+id), ResourceClass: class, RequiredBarrierIDs: deps, InspectPortBinding: exactRef(t, "fixture/inspect-port", "inspect-"+id), RequestSchemaDigest: digestV1(t, "request-"+id), ResultSchemaDigest: digestV1(t, "result-"+id)})
	if err != nil {
		t.Fatal(err)
	}
	return node
}
