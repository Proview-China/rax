package sdk_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

// packageAssemblyRegistryV1 is a test-only RegistryPort. It lets this SDK read
// projection keep testing exact active Package snapshots without reopening the
// production Registry's verification-aware Admission and governed Enable
// gates. It is not a production backend or an enablement implementation.
type packageAssemblyRegistryV1 struct {
	base      *registry.Registry
	mu        sync.RWMutex
	overrides map[string]registry.Record
	revision  core.Revision
}

func newPackageAssemblyRegistryV1(base *registry.Registry) *packageAssemblyRegistryV1 {
	return &packageAssemblyRegistryV1{base: base, overrides: make(map[string]registry.Record)}
}

func (r *packageAssemblyRegistryV1) SubmitCapability(value contract.CapabilityDescriptor, now time.Time) (registry.Record, error) {
	return r.base.SubmitCapability(value, now)
}

func (r *packageAssemblyRegistryV1) SubmitTool(value contract.ToolDescriptor, now time.Time) (registry.Record, error) {
	return r.base.SubmitTool(value, now)
}

func (r *packageAssemblyRegistryV1) SubmitPackage(value contract.ToolPackageManifest, now time.Time) (registry.Record, error) {
	return r.base.SubmitPackage(value, now)
}

func (r *packageAssemblyRegistryV1) SubmitToolAlias(value contract.ToolAliasV1, expected *contract.ToolAliasRefV1, now time.Time) (registry.Record, error) {
	return r.base.SubmitToolAlias(value, expected, now)
}

func (r *packageAssemblyRegistryV1) SubmitMCPToolMapping(value contract.MCPToolMappingManifestV1, now time.Time) (registry.Record, error) {
	return r.base.SubmitMCPToolMapping(value, now)
}

func (r *packageAssemblyRegistryV1) ResolveCapability(id string) (contract.CapabilityDescriptor, registry.Record, bool) {
	return r.base.ResolveCapability(id)
}

func (r *packageAssemblyRegistryV1) ResolveTool(id string) (contract.ToolDescriptor, registry.Record, bool) {
	return r.base.ResolveTool(id)
}

func (r *packageAssemblyRegistryV1) ResolvePackage(id string) (contract.ToolPackageManifest, registry.Record, bool) {
	manifest, record, ok := r.base.ResolvePackage(id)
	if !ok {
		return manifest, record, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if override, exists := r.overrides[id]; exists {
		record = override
	}
	return manifest, record, true
}

func (r *packageAssemblyRegistryV1) InspectToolAlias(exact contract.ToolAliasRefV1) (contract.ToolAliasV1, registry.Record, bool) {
	return r.base.InspectToolAlias(exact)
}

func (r *packageAssemblyRegistryV1) ResolveToolAlias(id string) (contract.ToolAliasV1, registry.Record, bool) {
	return r.base.ResolveToolAlias(id)
}

func (r *packageAssemblyRegistryV1) InspectMCPToolMapping(exact contract.MCPToolMappingManifestRefV1) (contract.MCPToolMappingManifestV1, registry.Record, bool) {
	return r.base.InspectMCPToolMapping(exact)
}

func (r *packageAssemblyRegistryV1) Snapshot() (registry.Snapshot, error) {
	snapshot, err := r.base.Snapshot()
	if err != nil {
		return registry.Snapshot{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.overrides) == 0 {
		return snapshot, nil
	}
	for i := range snapshot.Records {
		if snapshot.Records[i].Kind == "package" {
			if override, ok := r.overrides[snapshot.Records[i].ID]; ok {
				snapshot.Records[i] = override
			}
		}
	}
	snapshot.Revision = r.revision
	snapshot.Digest, err = contract.Seal("praxis.tool-mcp.registry", "v1", "Snapshot", snapshot.Records)
	return snapshot, err
}

func (r *packageAssemblyRegistryV1) Transition(kind, id string, expected core.Revision, target registry.State, now time.Time) (registry.Record, error) {
	if kind != "package" {
		return r.base.Transition(kind, id, expected, target, now)
	}
	manifest, baseRecord, ok := r.base.ResolvePackage(id)
	if !ok || manifest.Validate() != nil {
		return registry.Record{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "test Package is absent")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	current := baseRecord
	if override, exists := r.overrides[id]; exists {
		current = override
	}
	if current.RegistryRevision != expected {
		return registry.Record{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "test Package CAS revision differs")
	}
	allowed := current.State == registry.StateSubmitted && target == registry.StateAdmitted ||
		current.State == registry.StateAdmitted && target == registry.StateActive ||
		current.State == registry.StateActive && target == registry.StateRevoked
	if !allowed {
		return registry.Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidTransition, "test Package transition is not monotonic")
	}
	if now.IsZero() || now.UnixNano() < current.UpdatedUnixNano {
		return registry.Record{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "test Package clock regressed")
	}
	baseSnapshot, err := r.base.Snapshot()
	if err != nil {
		return registry.Record{}, err
	}
	if r.revision < baseSnapshot.Revision {
		r.revision = baseSnapshot.Revision
	}
	r.revision++
	current.State = target
	current.RegistryRevision = r.revision
	current.UpdatedUnixNano = now.UnixNano()
	r.overrides[id] = current
	return current, nil
}

type packageAssemblyFixtureV1 struct {
	client     *sdk.SDKV1
	registry   *packageAssemblyRegistryV1
	capability contract.CapabilityDescriptor
	tool       contract.ToolDescriptor
}

func activePackageAssemblyFixtureV1(t *testing.T) (packageAssemblyFixtureV1, sdk.RegistrySnapshotRefV1) {
	t.Helper()
	baseFixture := activeSDKFixtureV1(t)
	store := newPackageAssemblyRegistryV1(baseFixture.registry)
	client, err := sdk.NewV1(store, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	fixture := packageAssemblyFixtureV1{client: client, registry: store, capability: baseFixture.capability, tool: baseFixture.tool}
	record, err := fixture.client.RegisterPackageV1(context.Background(), testkit.Package())
	if err != nil {
		t.Fatal(err)
	}
	record, err = fixture.registry.Transition("package", string(testkit.Package().ID), record.RegistryRevision, registry.StateAdmitted, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.registry.Transition("package", string(testkit.Package().ID), record.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	snapshot, err := fixture.client.InspectRegistrySnapshotV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return fixture, sdk.RegistrySnapshotRefV1{Revision: snapshot.Revision, Digest: snapshot.Digest}
}

func TestPackageAssemblyV1ResolvesExactActiveClosureAndDeepCopies(t *testing.T) {
	fixture, snapshot := activePackageAssemblyFixtureV1(t)
	got, err := fixture.client.ResolvePackageForAssemblyV1(context.Background(), testkit.Package().ID, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err = got.Validate(); err != nil || got.Package.Digest != testkit.Package().Digest || len(got.Tools) != 1 || got.Tools[0].Digest != fixture.tool.Digest || len(got.Capabilities) != 1 || got.Capabilities[0].Digest != fixture.capability.Digest {
		t.Fatalf("Package assembly lost exact closure: %v %+v", err, got)
	}
	got.Package.Descriptors[0].Digest = testkit.Digest("mutated-package-descriptor")
	got.Tools[0].EffectKinds[0] = "praxis.tool/mutated"
	got.Capabilities[0].EffectKinds[0] = "praxis.tool/mutated"
	again, err := fixture.client.ResolvePackageForAssemblyV1(context.Background(), testkit.Package().ID, snapshot)
	if err != nil || again.Package.Descriptors[0].Digest != testkit.Package().Descriptors[0].Digest || again.Tools[0].EffectKinds[0] != "praxis.tool/execute" || again.Capabilities[0].EffectKinds[0] != "praxis.tool/execute" {
		t.Fatalf("Package assembly exposed mutable Registry state: %v %+v", err, again)
	}
}

func TestPackageAssemblyV1FailsClosedOnInactiveStaleAndUnderDeclaredEffect(t *testing.T) {
	baseFixture := activeSDKFixtureV1(t)
	fixture := packageAssemblyFixtureV1{registry: newPackageAssemblyRegistryV1(baseFixture.registry), capability: baseFixture.capability, tool: baseFixture.tool}
	var err error
	fixture.client, err = sdk.NewV1(fixture.registry, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	manifest := testkit.Package()
	record, err := fixture.client.RegisterPackageV1(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := fixture.client.InspectRegistrySnapshotV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ref := sdk.RegistrySnapshotRefV1{Revision: snapshot.Revision, Digest: snapshot.Digest}
	if _, err = fixture.client.ResolvePackageForAssemblyV1(context.Background(), manifest.ID, ref); err == nil {
		t.Fatal("submitted Package was assembled")
	}
	record, err = fixture.registry.Transition("package", string(manifest.ID), record.RegistryRevision, registry.StateAdmitted, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.registry.Transition("package", string(manifest.ID), record.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.client.ResolvePackageForAssemblyV1(context.Background(), manifest.ID, ref); err == nil {
		t.Fatal("stale Registry Snapshot assembled a Package")
	}

	otherBase := activeSDKFixtureV1(t)
	other := packageAssemblyFixtureV1{registry: newPackageAssemblyRegistryV1(otherBase.registry), capability: otherBase.capability, tool: otherBase.tool}
	other.client, err = sdk.NewV1(other.registry, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	underDeclared := testkit.Package()
	underDeclared.EffectKinds = []runtimeports.NamespacedNameV2{"praxis.tool/other"}
	underDeclared.Digest = ""
	underDeclared, err = contract.SealPackage(underDeclared)
	if err != nil {
		t.Fatal(err)
	}
	record, err = other.client.RegisterPackageV1(context.Background(), underDeclared)
	if err != nil {
		t.Fatal(err)
	}
	record, err = other.registry.Transition("package", string(underDeclared.ID), record.RegistryRevision, registry.StateAdmitted, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = other.registry.Transition("package", string(underDeclared.ID), record.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	otherSnapshot, err := other.client.InspectRegistrySnapshotV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = other.client.ResolvePackageForAssemblyV1(context.Background(), underDeclared.ID, sdk.RegistrySnapshotRefV1{Revision: otherSnapshot.Revision, Digest: otherSnapshot.Digest}); err == nil {
		t.Fatal("Package under-declared a Tool effect and was assembled")
	}
}

func TestPackageAssemblyV1RejectsNilCanceledAndClockRegression(t *testing.T) {
	fixture, snapshot := activePackageAssemblyFixtureV1(t)
	if _, err := fixture.client.ResolvePackageForAssemblyV1(nil, testkit.Package().ID, snapshot); err == nil {
		t.Fatal("nil context was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.client.ResolvePackageForAssemblyV1(ctx, testkit.Package().ID, snapshot); err != context.Canceled {
		t.Fatalf("canceled context was not preserved: %v", err)
	}
	values := []time.Time{testkit.FixedTime, testkit.FixedTime.Add(-time.Nanosecond)}
	var mu sync.Mutex
	index := 0
	client, err := sdk.NewV1(fixture.registry, func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.ResolvePackageForAssemblyV1(context.Background(), testkit.Package().ID, snapshot); err == nil {
		t.Fatal("Package assembly accepted a regressing clock")
	}
}

func TestPackageAssemblyV1ConcurrentReadsAreDeterministic(t *testing.T) {
	fixture, snapshot := activePackageAssemblyFixtureV1(t)
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	digests := make(chan string, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := fixture.client.ResolvePackageForAssemblyV1(context.Background(), testkit.Package().ID, snapshot)
			errs <- err
			if err == nil {
				digests <- string(value.Package.Digest) + ":" + string(value.Tools[0].Digest) + ":" + string(value.Capabilities[0].Digest)
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(digests)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := ""
	for digest := range digests {
		if want == "" {
			want = digest
		} else if digest != want {
			t.Fatal("concurrent Package assembly returned different exact closures")
		}
	}
}

func TestPackageAssemblyV1RevocationInvalidatesOldAndCurrentSnapshots(t *testing.T) {
	fixture, oldSnapshot := activePackageAssemblyFixtureV1(t)
	if _, err := fixture.client.ResolvePackageForAssemblyV1(context.Background(), testkit.Package().ID, oldSnapshot); err != nil {
		t.Fatalf("active Package did not assemble before revocation: %v", err)
	}
	_, record, ok := fixture.registry.ResolvePackage(string(testkit.Package().ID))
	if !ok || record.State != registry.StateActive {
		t.Fatalf("active Package record is absent before revocation: %+v", record)
	}
	revoked, err := fixture.registry.Transition("package", string(testkit.Package().ID), record.RegistryRevision, registry.StateRevoked, testkit.FixedTime.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if revoked.State != registry.StateRevoked {
		t.Fatalf("Package did not reach the revoked terminal state: %+v", revoked)
	}

	if _, err = fixture.client.ResolvePackageForAssemblyV1(context.Background(), testkit.Package().ID, oldSnapshot); err == nil {
		t.Fatal("revoked Package assembled through its stale pre-revocation Snapshot")
	}
	current, err := fixture.client.InspectRegistrySnapshotV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	currentRef := sdk.RegistrySnapshotRefV1{Revision: current.Revision, Digest: current.Digest}
	if _, err = fixture.client.ResolvePackageForAssemblyV1(context.Background(), testkit.Package().ID, currentRef); err == nil {
		t.Fatal("revoked Package assembled through the current Registry Snapshot")
	}
	if _, err = fixture.registry.Transition("package", string(testkit.Package().ID), revoked.RegistryRevision, registry.StateActive, testkit.FixedTime.Add(2*time.Second)); err == nil {
		t.Fatal("revoked Package was reactivated")
	}
	_, unchanged, ok := fixture.registry.ResolvePackage(string(testkit.Package().ID))
	if !ok || unchanged.State != registry.StateRevoked || unchanged.RegistryRevision != revoked.RegistryRevision {
		t.Fatalf("failed reactivation changed the revoked Package: %+v", unchanged)
	}
}
