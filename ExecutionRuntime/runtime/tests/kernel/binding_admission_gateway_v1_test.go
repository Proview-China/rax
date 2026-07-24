package kernel_test

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	runtimesqlite "github.com/Proview-China/rax/ExecutionRuntime/runtime/storage/sqlite"
)

func TestBindingAdmissionGatewayV1HappyPathAndLostCommitReply(t *testing.T) {
	t.Parallel()
	for _, lostReply := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "lost_reply"}[lostReply], func(t *testing.T) {
			fixture := newBindingAdmissionGatewayFixtureV1(t)
			if lostReply {
				fixture.factStore.LoseNextCommitReply()
			}
			result, err := fixture.gateway(t).StartOrInspectBindingAdmissionV1(context.Background(), fixture.request)
			if err != nil {
				t.Fatal(err)
			}
			if err := result.ValidateCurrent(fixture.request, fixture.now); err != nil {
				t.Fatal(err)
			}
			if fixture.facts.commitCalls.Load() != 1 {
				t.Fatalf("expected one atomic BindingSet commit, got %d", fixture.facts.commitCalls.Load())
			}
			inspected, err := fixture.gateway(t).InspectBindingAdmissionV1(context.Background(), ports.BindingAdmissionInspectRequestV1{AttemptID: fixture.request.AttemptID, RequestDigest: fixture.request.RequestDigest})
			if err != nil || inspected.ResultDigest != result.ResultDigest {
				t.Fatalf("exact result was not recoverable: result=%+v err=%v", inspected, err)
			}
		})
	}
}

func TestBindingAdmissionGatewayV1LostAttemptCreateReplyNeverDispatches(t *testing.T) {
	t.Parallel()
	fixture := newBindingAdmissionGatewayFixtureV1(t)
	fixture.attempts.LoseNextCreateReplyV1()
	_, err := fixture.gateway(t).StartOrInspectBindingAdmissionV1(context.Background(), fixture.request)
	if !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost Attempt create must stop at reconciliation: %v", err)
	}
	if fixture.facts.createCalls.Load() != 0 || fixture.facts.casCalls.Load() != 0 || fixture.facts.commitCalls.Load() != 0 {
		t.Fatalf("lost Attempt create dispatched Binding writes: create=%d cas=%d commit=%d", fixture.facts.createCalls.Load(), fixture.facts.casCalls.Load(), fixture.facts.commitCalls.Load())
	}
	_, err = fixture.gateway(t).StartOrInspectBindingAdmissionV1(context.Background(), fixture.request)
	if !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("restart must remain inspect-only: %v", err)
	}
	if fixture.facts.createCalls.Load() != 0 || fixture.facts.casCalls.Load() != 0 || fixture.facts.commitCalls.Load() != 0 {
		t.Fatal("restart redispatched a Binding prefix")
	}
}

func TestBindingAdmissionGatewayV1CrashPrefixIsInspectOnly(t *testing.T) {
	t.Parallel()
	fixture := newBindingAdmissionGatewayFixtureV1(t)
	inputs, err := fixture.reader.snapshot(fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	intent := bindingAdmissionIntentFromGatewayFixtureV1(t, fixture, inputs)
	if _, err := fixture.attempts.CreateBindingAdmissionAttemptV1(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.factStore.CreateBinding(context.Background(), intent.DeclaredCandidates[0]); err != nil {
		t.Fatal(err)
	}
	fixture.facts.createCalls.Store(0)
	_, err = fixture.gateway(t).StartOrInspectBindingAdmissionV1(context.Background(), fixture.request)
	if !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("crash prefix must require reconciliation: %v", err)
	}
	if fixture.facts.createCalls.Load() != 0 || fixture.facts.casCalls.Load() != 0 || fixture.facts.commitCalls.Load() != 0 {
		t.Fatalf("crash prefix was advanced blindly: create=%d cas=%d commit=%d", fixture.facts.createCalls.Load(), fixture.facts.casCalls.Load(), fixture.facts.commitCalls.Load())
	}
}

func TestBindingAdmissionGatewayV1Concurrent64HasOneExecutionOwner(t *testing.T) {
	fixture := newBindingAdmissionGatewayFixtureV1(t)
	const workers = 64
	var wait sync.WaitGroup
	var success atomic.Int64
	var indeterminate atomic.Int64
	wait.Add(workers)
	for range workers {
		go func() {
			defer wait.Done()
			result, err := fixture.gateway(t).StartOrInspectBindingAdmissionV1(context.Background(), fixture.request)
			if err == nil {
				if result.ValidateCurrent(fixture.request, fixture.now) != nil {
					t.Errorf("invalid concurrent result: %+v", result)
				}
				success.Add(1)
				return
			}
			if core.HasCategory(err, core.ErrorIndeterminate) || core.HasCategory(err, core.ErrorConflict) {
				indeterminate.Add(1)
				return
			}
			t.Errorf("unexpected concurrent error: %v", err)
		}()
	}
	wait.Wait()
	if fixture.facts.createCalls.Load() != 1 || fixture.facts.casCalls.Load() != 2 || fixture.facts.commitCalls.Load() != 1 {
		t.Fatalf("Binding lifecycle was not single-owner: create=%d cas=%d commit=%d successes=%d indeterminate=%d", fixture.facts.createCalls.Load(), fixture.facts.casCalls.Load(), fixture.facts.commitCalls.Load(), success.Load(), indeterminate.Load())
	}
	result, err := fixture.gateway(t).InspectBindingAdmissionV1(context.Background(), ports.BindingAdmissionInspectRequestV1{AttemptID: fixture.request.AttemptID, RequestDigest: fixture.request.RequestDigest})
	if err != nil || result.ValidateCurrent(fixture.request, fixture.now) != nil {
		t.Fatalf("concurrent closure did not preserve one result: %+v err=%v", result, err)
	}
}

func TestBindingAdmissionGatewayV1SQLiteConcurrent64StoresOneExecution(t *testing.T) {
	fixture := newBindingAdmissionGatewayFixtureV1(t)
	path := filepath.Join(t.TempDir(), "binding-admission.db")
	const workers = 64
	stores := make([]*runtimesqlite.Store, 0, workers)
	gateways := make([]*kernel.BindingAdmissionGatewayV1, 0, workers)
	var createCalls, casCalls, commitCalls atomic.Int64
	for range workers {
		store, err := runtimesqlite.Open(context.Background(), runtimesqlite.Config{Path: path, BusyTimeout: 5 * time.Second, MaxOpenConns: 2, Clock: func() time.Time { return fixture.now }})
		if err != nil {
			t.Fatal(err)
		}
		stores = append(stores, store)
		facts := &sharedCountingBindingFactPortV2{BindingFactPortV2: store, createCalls: &createCalls, casCalls: &casCalls, commitCalls: &commitCalls}
		gateway, err := kernel.NewBindingAdmissionGatewayV1(facts, store, fixture.reader, func() time.Time { return fixture.now })
		if err != nil {
			t.Fatal(err)
		}
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
		go func(g *kernel.BindingAdmissionGatewayV1) {
			defer wait.Done()
			_, err := g.StartOrInspectBindingAdmissionV1(context.Background(), fixture.request)
			if err != nil && !core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorConflict) && !core.HasCategory(err, core.ErrorUnavailable) {
				t.Errorf("unexpected sqlite concurrent error: %v", err)
			}
		}(gateway)
	}
	wait.Wait()
	if createCalls.Load() != 1 || casCalls.Load() != 2 || commitCalls.Load() != 1 {
		t.Fatalf("sqlite gateways did not preserve one execution token: create=%d cas=%d commit=%d", createCalls.Load(), casCalls.Load(), commitCalls.Load())
	}
	result, err := gateways[0].InspectBindingAdmissionV1(context.Background(), ports.BindingAdmissionInspectRequestV1{AttemptID: fixture.request.AttemptID, RequestDigest: fixture.request.RequestDigest})
	if err != nil || result.ValidateCurrent(fixture.request, fixture.now) != nil {
		t.Fatalf("sqlite admission result is not durable: %+v err=%v", result, err)
	}
	if err := stores[0].IntegrityCheckV1(context.Background()); err != nil {
		t.Fatal(err)
	}
	terminal, err := stores[0].InspectBindingAdmissionAttemptV1(context.Background(), fixture.request.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	initial := terminal.CloneV1()
	initial.Revision = 1
	initial.State = control.BindingAdmissionIntentRecordedV1
	initial.Result = nil
	initial.UpdatedUnixNano = initial.CreatedUnixNano
	initial.Digest = ""
	initial, err = control.SealBindingAdmissionAttemptFactV1(initial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stores[1].CreateBindingAdmissionAttemptV1(context.Background(), initial); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same-content sqlite Create replay granted another execution token: %v", err)
	}
	unknown := initial.CloneV1()
	unknown.Revision = 2
	unknown.State = control.BindingAdmissionOutcomeUnknownV1
	unknown.UpdatedUnixNano++
	unknown.Digest = ""
	unknown, err = control.SealBindingAdmissionAttemptFactV1(unknown)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stores[2].CompareAndSwapBindingAdmissionAttemptV1(context.Background(), control.BindingAdmissionAttemptCASRequestV1{ExpectedRevision: initial.Revision, ExpectedDigest: initial.Digest, Next: unknown}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale sqlite CAS precondition was not a hard Conflict: %v", err)
	}
}

func TestBindingAdmissionGatewayV1S2DriftAndClockRollbackFailClosed(t *testing.T) {
	t.Run("s2_drift", func(t *testing.T) {
		fixture := newBindingAdmissionGatewayFixtureV1(t)
		fixture.reader.driftAfter.Store(9) // nine S1 reads; first S2 read drifts.
		_, err := fixture.gateway(t).StartOrInspectBindingAdmissionV1(context.Background(), fixture.request)
		if err == nil {
			t.Fatal("S2 drift was accepted")
		}
		if fixture.facts.commitCalls.Load() != 0 {
			t.Fatal("S2 drift reached BindingSet commit")
		}
	})
	t.Run("clock_rollback", func(t *testing.T) {
		fixture := newBindingAdmissionGatewayFixtureV1(t)
		var calls atomic.Int64
		clock := func() time.Time {
			if calls.Add(1) == 1 {
				return fixture.now
			}
			return fixture.now.Add(-time.Nanosecond)
		}
		gateway, err := kernel.NewBindingAdmissionGatewayV1(fixture.facts, fixture.attempts, fixture.reader, clock)
		if err != nil {
			t.Fatal(err)
		}
		_, err = gateway.StartOrInspectBindingAdmissionV1(context.Background(), fixture.request)
		if !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock rollback was not rejected: %v", err)
		}
		if fixture.facts.commitCalls.Load() != 0 {
			t.Fatal("clock rollback reached commit")
		}
	})
}

func TestBindingAdmissionGatewayV1RejectsTypedNilAndProjectionSplice(t *testing.T) {
	fixture := newBindingAdmissionGatewayFixtureV1(t)
	var nilReader *bindingAdmissionInputReaderFixtureV1
	if _, err := kernel.NewBindingAdmissionGatewayV1(fixture.facts, fixture.attempts, nilReader, func() time.Time { return fixture.now }); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil reader was accepted: %v", err)
	}
	fixture.reader.plan.Ref.Digest = digestBindingAdmissionGatewayFixtureV1("spliced-plan-ref")
	fixture.reader.plan, _ = ports.SealBindingAdmissionPlanCurrentV1(fixture.reader.plan)
	if _, err := fixture.gateway(t).StartOrInspectBindingAdmissionV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("same-ID projection splice was accepted: %v", err)
	}
}

type bindingAdmissionGatewayFixtureV1 struct {
	now       time.Time
	request   ports.BindingAdmissionRequestV1
	reader    *bindingAdmissionInputReaderFixtureV1
	factStore *fakes.BindingStoreV2
	facts     *countingBindingFactPortV2
	attempts  *fakes.BindingAdmissionAttemptStoreV1
}

func (f *bindingAdmissionGatewayFixtureV1) gateway(t *testing.T) *kernel.BindingAdmissionGatewayV1 {
	t.Helper()
	gateway, err := kernel.NewBindingAdmissionGatewayV1(f.facts, f.attempts, f.reader, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	return gateway
}

type countingBindingFactPortV2 struct {
	control.BindingFactPortV2
	createCalls atomic.Int64
	casCalls    atomic.Int64
	commitCalls atomic.Int64
}

type sharedCountingBindingFactPortV2 struct {
	control.BindingFactPortV2
	createCalls *atomic.Int64
	casCalls    *atomic.Int64
	commitCalls *atomic.Int64
}

func (p *sharedCountingBindingFactPortV2) CreateBinding(ctx context.Context, fact control.BindingFactV2) (control.BindingFactV2, error) {
	p.createCalls.Add(1)
	return p.BindingFactPortV2.CreateBinding(ctx, fact)
}
func (p *sharedCountingBindingFactPortV2) CompareAndSwapBinding(ctx context.Context, r control.BindingFactCASRequestV2) (control.BindingFactV2, error) {
	p.casCalls.Add(1)
	return p.BindingFactPortV2.CompareAndSwapBinding(ctx, r)
}
func (p *sharedCountingBindingFactPortV2) CommitBindingSet(ctx context.Context, r control.CommitBindingSetRequestV2) (control.BindingSetFactV2, error) {
	p.commitCalls.Add(1)
	return p.BindingFactPortV2.CommitBindingSet(ctx, r)
}

func (p *countingBindingFactPortV2) CreateBinding(ctx context.Context, fact control.BindingFactV2) (control.BindingFactV2, error) {
	p.createCalls.Add(1)
	return p.BindingFactPortV2.CreateBinding(ctx, fact)
}
func (p *countingBindingFactPortV2) CompareAndSwapBinding(ctx context.Context, r control.BindingFactCASRequestV2) (control.BindingFactV2, error) {
	p.casCalls.Add(1)
	return p.BindingFactPortV2.CompareAndSwapBinding(ctx, r)
}
func (p *countingBindingFactPortV2) CommitBindingSet(ctx context.Context, r control.CommitBindingSetRequestV2) (control.BindingSetFactV2, error) {
	p.commitCalls.Add(1)
	return p.BindingFactPortV2.CommitBindingSet(ctx, r)
}

type bindingAdmissionInputReaderFixtureV1 struct {
	definition, resolution, authority, policy ports.OwnerCurrentRefV1
	plan                                      ports.BindingAdmissionPlanCurrentV1
	assembly                                  ports.BindingAdmissionAssemblyCurrentV1
	catalog                                   ports.BindingAdmissionCatalogCurrentV1
	release                                   ports.BindingAdmissionReleaseCurrentV1
	resources                                 ports.ResourceBindingSetV1
	reads                                     atomic.Int64
	driftAfter                                atomic.Int64
}

func (r *bindingAdmissionInputReaderFixtureV1) drift() bool {
	threshold := r.driftAfter.Load()
	return threshold > 0 && r.reads.Add(1) > threshold
}
func (r *bindingAdmissionInputReaderFixtureV1) InspectBindingAdmissionDefinitionCurrentV1(context.Context, ports.OwnerCurrentRefV1) (ports.OwnerCurrentRefV1, error) {
	v := r.definition
	if r.drift() {
		v.Digest = digestBindingAdmissionGatewayFixtureV1("drift")
	}
	return v, nil
}
func (r *bindingAdmissionInputReaderFixtureV1) InspectBindingAdmissionPlanCurrentV1(context.Context, ports.OwnerCurrentRefV1) (ports.BindingAdmissionPlanCurrentV1, error) {
	r.drift()
	return r.plan, nil
}
func (r *bindingAdmissionInputReaderFixtureV1) InspectBindingAdmissionAssemblyCurrentV1(context.Context, ports.OwnerCurrentRefV1) (ports.BindingAdmissionAssemblyCurrentV1, error) {
	r.drift()
	return r.assembly, nil
}
func (r *bindingAdmissionInputReaderFixtureV1) InspectBindingAdmissionCatalogCurrentV1(context.Context, ports.OwnerCurrentRefV1) (ports.BindingAdmissionCatalogCurrentV1, error) {
	r.drift()
	return r.catalog, nil
}
func (r *bindingAdmissionInputReaderFixtureV1) InspectBindingAdmissionResolutionCurrentV1(context.Context, ports.OwnerCurrentRefV1) (ports.OwnerCurrentRefV1, error) {
	r.drift()
	return r.resolution, nil
}
func (r *bindingAdmissionInputReaderFixtureV1) InspectBindingAdmissionReleaseCurrentV1(context.Context, ports.PreBindingComponentReleaseV1) (ports.BindingAdmissionReleaseCurrentV1, error) {
	r.drift()
	return r.release, nil
}
func (r *bindingAdmissionInputReaderFixtureV1) InspectBindingAdmissionResourceBindingSetCurrentV1(context.Context, ports.ResourceBindingSetRefV1) (ports.ResourceBindingSetV1, error) {
	r.drift()
	return r.resources, nil
}
func (r *bindingAdmissionInputReaderFixtureV1) InspectBindingAdmissionAuthorityCurrentV1(context.Context, ports.OwnerCurrentRefV1) (ports.OwnerCurrentRefV1, error) {
	r.drift()
	return r.authority, nil
}
func (r *bindingAdmissionInputReaderFixtureV1) InspectBindingAdmissionPolicyCurrentV1(context.Context, ports.OwnerCurrentRefV1) (ports.OwnerCurrentRefV1, error) {
	r.drift()
	return r.policy, nil
}

func (r *bindingAdmissionInputReaderFixtureV1) snapshot(req ports.BindingAdmissionRequestV1) (control.BindingAdmissionInputSnapshotV1, error) {
	return control.SealBindingAdmissionInputSnapshotV1(control.BindingAdmissionInputSnapshotV1{Definition: r.definition, Plan: r.plan, Assembly: r.assembly, Catalog: r.catalog, Resolution: r.resolution, Releases: []ports.BindingAdmissionReleaseCurrentV1{r.release}, Resources: r.resources, Authority: r.authority, Policy: r.policy})
}

func newBindingAdmissionGatewayFixtureV1(t *testing.T) *bindingAdmissionGatewayFixtureV1 {
	t.Helper()
	now := time.Unix(203_000, 0)
	expires := now.Add(5 * time.Minute)
	component := ports.ComponentIDV2("praxis/component")
	manifest := ports.ComponentManifestV2{ContractVersion: ports.BindingContractVersionV2, ComponentID: component, Kind: "praxis/component-kind", GovernanceCategory: "praxis/execution", SemanticVersion: "1.0.0", ArtifactDigest: digestBindingAdmissionGatewayFixtureV1("artifact"), Contract: ports.ContractBindingV2{Name: "praxis/component-contract", Version: "2.0.0", Compatible: ports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}}, Schemas: []ports.SchemaRefV2{}, Locality: ports.LocalityHostControlPlane, Dependencies: []ports.ComponentDependencyV2{}, RequiredCapabilities: []ports.CapabilityRequirementV2{}, ProvidedCapabilities: []ports.ProvidedCapabilityV2{{Capability: "praxis/execute", TTLSeconds: 300, Schemas: []ports.SchemaRefV2{}}}, Conformance: ports.ConformanceFullyControlled, ResidualClass: ports.ResidualInspectable, Owners: []ports.OwnerAssignmentV2{{Role: ports.OwnerEffect, OwnerComponentID: component}, {Role: ports.OwnerSettlement, OwnerComponentID: component}, {Role: ports.OwnerCleanup, OwnerComponentID: component}}, Credentials: []ports.CredentialRequirementV2{}, OfflinePolicy: ports.OfflineDenied, Extensions: []ports.GovernanceExtensionV2{}, Annotations: []ports.DisplayAnnotationV2{}}
	catalog := ports.GovernanceCatalogV2{Registrations: []ports.GovernanceRegistrationV2{{Kind: manifest.Kind, Category: manifest.GovernanceCategory, Capabilities: []ports.CapabilityNameV2{"praxis/execute"}, Schemas: []ports.SchemaRefV2{}, ExtensionPolicies: []ports.ExtensionPolicyV2{}, AllowedLocalities: []ports.LocalityV2{ports.LocalityHostControlPlane}, AllowedConformance: []ports.ConformanceLevel{ports.ConformanceFullyControlled}}}}
	catalogDigest, err := catalog.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := ports.SealBindingPlanV2(ports.BindingPlanV2{ID: "plan-a", GovernanceDigest: catalogDigest, Requirements: []ports.BindingRequirementV2{{ComponentID: component, Kind: manifest.Kind, SemanticVersion: ports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, ContractName: manifest.Contract.Name, Contract: ports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}, ArtifactDigest: manifest.ArtifactDigest, RequiredCapabilities: []ports.CapabilityNameV2{"praxis/execute"}, Required: true}}})
	if err != nil {
		t.Fatal(err)
	}
	owner := core.OwnerRef{Domain: "praxis.owner", ID: "owner-a"}
	current := func(id string) ports.OwnerCurrentRefV1 {
		return ports.OwnerCurrentRefV1{Owner: owner, ContractVersion: "1.0.0", ID: id, Revision: 1, Digest: digestBindingAdmissionGatewayFixtureV1(id), ExpiresUnixNano: expires.UnixNano()}
	}
	releaseRef := ports.PreBindingComponentReleaseV1{ComponentID: component, Release: current("release"), Certification: current("certification"), DeploymentReadiness: current("deployment")}
	grant := ports.CapabilityGrantV2{Capability: "praxis/execute", EvidenceDigest: digestBindingAdmissionGatewayFixtureV1("grant"), ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()}
	release, err := ports.SealBindingAdmissionReleaseCurrentV1(ports.BindingAdmissionReleaseCurrentV1{Expected: releaseRef, ManifestDigest: manifestDigest, Grants: []ports.CapabilityGrantV2{grant}, ConformanceEvidenceDigest: digestBindingAdmissionGatewayFixtureV1("conformance"), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	handleOwner := core.OwnerRef{Domain: "praxis.resource", ID: "resource-owner"}
	cleanup := ports.OwnerCurrentRefV1{Owner: handleOwner, ContractVersion: "1.0.0", ID: "cleanup", Revision: 1, Digest: digestBindingAdmissionGatewayFixtureV1("cleanup"), ExpiresUnixNano: expires.UnixNano()}
	deployment := ports.OwnerCurrentRefV1{Owner: handleOwner, ContractVersion: "1.0.0", ID: "resource-deployment", Revision: 1, Digest: digestBindingAdmissionGatewayFixtureV1("resource-deployment"), ExpiresUnixNano: expires.UnixNano()}
	handle, err := ports.SealResourceHandleCurrentV1(ports.ResourceHandleCurrentV1{Ref: ports.ResourceHandleRefV1{Owner: handleOwner, ID: "resource-a", Revision: 1, Kind: "praxis/sqlite", ScopeDigest: digestBindingAdmissionGatewayFixtureV1("scope")}, CleanupContract: cleanup, DeploymentAttestation: deployment, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	resources, err := ports.SealResourceBindingSetV1(ports.ResourceBindingSetV1{Ref: ports.ResourceBindingSetRefV1{ID: "resource-set", Revision: 1}, Bindings: []ports.ResourceBindingV1{{ComponentID: component, Handle: handle.Ref, CleanupContract: cleanup, DeploymentAttestation: deployment}}, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	definition, planRef, assemblyRef, catalogRef, resolution, authority, policy := current("definition"), current("plan"), current("assembly"), current("catalog"), current("resolution"), current("authority"), current("policy")
	planCurrent, err := ports.SealBindingAdmissionPlanCurrentV1(ports.BindingAdmissionPlanCurrentV1{Ref: planRef, Plan: plan, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	assemblyCurrent, err := ports.SealBindingAdmissionAssemblyCurrentV1(ports.BindingAdmissionAssemblyCurrentV1{Ref: assemblyRef, Manifests: []ports.ComponentManifestV2{manifest}, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	catalogCurrent, err := ports.SealBindingAdmissionCatalogCurrentV1(ports.BindingAdmissionCatalogCurrentV1{Ref: catalogRef, Catalog: catalog, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	request, err := ports.SealBindingAdmissionRequestV1(ports.BindingAdmissionRequestV1{AttemptID: "binding-attempt", DefinitionCurrent: definition, PlanCurrent: planRef, AssemblyCurrent: assemblyRef, CatalogCurrent: catalogRef, ResolutionCurrent: resolution, Releases: []ports.PreBindingComponentReleaseV1{releaseRef}, ResourceBindingSet: resources.Ref, AuthorityCurrent: authority, PolicyCurrent: policy, ExpectedBindingSetID: "binding-set", RequestedNotAfterUnixNano: expires.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	reader := &bindingAdmissionInputReaderFixtureV1{definition: definition, plan: planCurrent, assembly: assemblyCurrent, catalog: catalogCurrent, resolution: resolution, release: release, resources: resources, authority: authority, policy: policy}
	store := fakes.NewBindingStoreV2(func() time.Time { return now })
	counting := &countingBindingFactPortV2{BindingFactPortV2: store}
	return &bindingAdmissionGatewayFixtureV1{now: now, request: request, reader: reader, factStore: store, facts: counting, attempts: fakes.NewBindingAdmissionAttemptStoreV1()}
}

func bindingAdmissionIntentFromGatewayFixtureV1(t *testing.T, fixture *bindingAdmissionGatewayFixtureV1, inputs control.BindingAdmissionInputSnapshotV1) control.BindingAdmissionAttemptFactV1 {
	t.Helper()
	// Build the deterministic candidates through an isolated normal gateway and
	// copy only the owner-generated intent before any lifecycle write.
	tempAttempts := fakes.NewBindingAdmissionAttemptStoreV1()
	tempAttempts.LoseNextCreateReplyV1()
	tempFacts := &countingBindingFactPortV2{BindingFactPortV2: fakes.NewBindingStoreV2(func() time.Time { return fixture.now })}
	gateway, err := kernel.NewBindingAdmissionGatewayV1(tempFacts, tempAttempts, fixture.reader, func() time.Time { return fixture.now })
	if err != nil {
		t.Fatal(err)
	}
	_, _ = gateway.StartOrInspectBindingAdmissionV1(context.Background(), fixture.request)
	intent, err := tempAttempts.InspectBindingAdmissionAttemptV1(context.Background(), fixture.request.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	intent.Revision = 1
	intent.State = control.BindingAdmissionIntentRecordedV1
	intent.Result = nil
	intent.UpdatedUnixNano = intent.CreatedUnixNano
	intent.Digest = ""
	intent, err = control.SealBindingAdmissionAttemptFactV1(intent)
	if err != nil {
		t.Fatal(err)
	}
	if intent.Inputs.SnapshotDigest != inputs.SnapshotDigest {
		t.Fatal("isolated intent snapshot drifted")
	}
	return intent
}

func digestBindingAdmissionGatewayFixtureV1(value string) core.Digest {
	return core.DigestBytes([]byte(value))
}
