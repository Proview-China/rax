package deployment_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	buildercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/deployment"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var ownerNowV2 = time.Unix(2_300_100_000, 0)

func TestHostDeploymentCurrentV2PublishAdvanceAndSupersededInspect(t *testing.T) {
	fixture := newOwnerFixtureV2(t)
	first, err := fixture.owner.PublishHostDeploymentCurrentV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Ref.PackageSelectionRef != fixture.selection.Ref {
		t.Fatal("Host did not persist the exact Builder selection Ref")
	}

	secondSelection := fixture.nextSelectionV2(t, 2)
	nextRequest := fixture.request
	nextRequest.ExpectedCurrent = first.Ref
	nextRequest.PackageSelectionRef = secondSelection.Ref
	nextRequest.RequestDigest = ""
	nextRequest, err = contract.SealPublishHostDeploymentCurrentRequestV2(nextRequest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.owner.PublishHostDeploymentCurrentV2(context.Background(), nextRequest)
	if err != nil || second.Ref.Revision != 2 {
		t.Fatalf("advance=%+v err=%v", second, err)
	}
	if _, err = fixture.owner.InspectHostDeploymentCurrentV2(context.Background(), first.Ref); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("superseded historical value reported current: %v", err)
	}
	if got, err := fixture.owner.InspectHostDeploymentCurrentV2(context.Background(), second.Ref); err != nil || !reflect.DeepEqual(got, second) {
		t.Fatalf("current Inspect=%+v err=%v", got, err)
	}
}

func TestHostDeploymentCurrentV2UnknownRecoveryRequiresCurrentWinner(t *testing.T) {
	t.Run("winner", func(t *testing.T) {
		fixture := newOwnerFixtureV2(t)
		fixture.repository.unknown = true
		got, err := fixture.owner.PublishHostDeploymentCurrentV2(context.Background(), fixture.request)
		if err != nil || got.Ref.Revision != 1 || fixture.repository.writes != 1 {
			t.Fatalf("lost reply recovery=%+v writes=%d err=%v", got, fixture.repository.writes, err)
		}
	})
	t.Run("pointer advanced", func(t *testing.T) {
		fixture := newOwnerFixtureV2(t)
		fixture.repository.unknownAndAdvance = true
		if _, err := fixture.owner.PublishHostDeploymentCurrentV2(context.Background(), fixture.request); !contract.HasCode(err, contract.ErrorConflict) {
			t.Fatalf("unknown recovery accepted superseded history: %v", err)
		}
		if fixture.repository.writes != 1 {
			t.Fatalf("unknown recovery retried mutation: %d", fixture.repository.writes)
		}
	})
}

func TestHostDeploymentCurrentV2ExpiryEqualityAndTypedNilFailClosed(t *testing.T) {
	fixture := newOwnerFixtureV2(t)
	fixture.clock = time.Unix(0, fixture.request.RequestedNotAfterUnixNano)
	fixture.owner, _ = deployment.NewCurrentOwnerV2(deployment.CurrentOwnerConfigV2{
		Builder: fixture.builder, Resources: fixture.resources, Services: fixture.services,
		Repository: fixture.repository, Clock: func() time.Time { return fixture.clock },
	})
	if _, err := fixture.owner.PublishHostDeploymentCurrentV2(context.Background(), fixture.request); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("now==expiry accepted: %v", err)
	}
	var nilRepository *ownerRepositoryV2
	if _, err := deployment.NewCurrentOwnerV2(deployment.CurrentOwnerConfigV2{
		Builder: fixture.builder, Resources: fixture.resources, Services: fixture.services, Repository: nilRepository,
	}); !contract.HasCode(err, contract.ErrorInvalidArgument) {
		t.Fatalf("typed nil repository accepted: %v", err)
	}
	for _, forbidden := range []string{"Factory", "Provider", "Runtime", "Application", "HostV3"} {
		if _, exists := reflect.TypeOf(deployment.CurrentOwnerConfigV2{}).FieldByName(forbidden); exists {
			t.Fatalf("Owner config exposes forbidden %s dependency", forbidden)
		}
	}
}

func TestHostDeploymentCurrentV2SelectionDriftBetweenS1AndS2WritesZero(t *testing.T) {
	fixture := newOwnerFixtureV2(t)
	next := selectionFixtureV2(t, fixture.closure, 2, fixture.request.RequestedNotAfterUnixNano)
	fixture.builder.mu.Lock()
	fixture.builder.currentSequence = []buildercontract.AgentPackageSelectionCurrentV1{fixture.selection, next}
	fixture.builder.mu.Unlock()
	if _, err := fixture.owner.PublishHostDeploymentCurrentV2(context.Background(), fixture.request); err == nil {
		t.Fatal("Selection current drift between S1 and S2 was accepted")
	}
	if fixture.repository.writes != 0 {
		t.Fatalf("S1/S2 drift reached durable CAS: %d", fixture.repository.writes)
	}
}

func TestHostDeploymentCurrentV2SnapshotsCallerSlicesBeforeValidation(t *testing.T) {
	fixture := newOwnerFixtureV2(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	fixture.builder.mu.Lock()
	fixture.builder.exactEntered = entered
	fixture.builder.exactRelease = release
	fixture.builder.mu.Unlock()
	request := fixture.request
	result := make(chan error, 1)
	go func() {
		_, err := fixture.owner.PublishHostDeploymentCurrentV2(context.Background(), request)
		result <- err
	}()
	<-entered
	request.ResourceHandles[0].ID = "caller-mutated-resource"
	request.ServiceBindings[0].ConfiguredID = "caller-mutated-service"
	request.Bootstrap.StatePlaneBindingIDs[0] = "caller-mutated-state"
	request.Bootstrap.RuntimeServiceBindingIDs[0] = "caller-mutated-runtime"
	request.Bootstrap.ApplicationServiceBindingIDs[0] = "caller-mutated-application"
	request.Bootstrap.HarnessServiceBindingIDs[0] = "caller-mutated-harness"
	request.Bootstrap.EnabledControlAPISurfaces[0] = "caller-mutated-surface"
	close(release)
	if err := <-result; err != nil {
		t.Fatalf("caller mutation changed the published snapshot: %v", err)
	}
	if fixture.repository.writes != 1 {
		t.Fatalf("snapshot publish writes=%d", fixture.repository.writes)
	}
}

func TestHostDeploymentCurrentV2FinalPointerReadRejectsConcurrentAdvance(t *testing.T) {
	t.Run("normal publish", func(t *testing.T) {
		fixture := newOwnerFixtureV2(t)
		fixture.repository.mu.Lock()
		fixture.repository.advanceOnCurrentRead = 1
		fixture.repository.mu.Unlock()
		if _, err := fixture.owner.PublishHostDeploymentCurrentV2(context.Background(), fixture.request); !contract.HasCode(err, contract.ErrorConflict) {
			t.Fatalf("normal publish returned a deployment superseded after CAS: %v", err)
		}
		if fixture.repository.writes != 1 {
			t.Fatalf("normal publish final pointer check retried CAS: %d", fixture.repository.writes)
		}
	})
	t.Run("public Inspect", func(t *testing.T) {
		fixture := newOwnerFixtureV2(t)
		current, err := fixture.owner.PublishHostDeploymentCurrentV2(context.Background(), fixture.request)
		if err != nil {
			t.Fatal(err)
		}
		fixture.repository.mu.Lock()
		fixture.repository.currentReads = 0
		fixture.repository.advanceOnCurrentRead = 2
		fixture.repository.mu.Unlock()
		if _, err = fixture.owner.InspectHostDeploymentCurrentV2(context.Background(), current.Ref); !contract.HasCode(err, contract.ErrorConflict) {
			t.Fatalf("Inspect returned a deployment superseded during source validation: %v", err)
		}
	})
	t.Run("unknown recovery", func(t *testing.T) {
		fixture := newOwnerFixtureV2(t)
		fixture.repository.mu.Lock()
		fixture.repository.unknown = true
		fixture.repository.advanceOnCurrentRead = 2
		fixture.repository.mu.Unlock()
		if _, err := fixture.owner.PublishHostDeploymentCurrentV2(context.Background(), fixture.request); !contract.HasCode(err, contract.ErrorConflict) {
			t.Fatalf("unknown recovery returned a deployment superseded during source validation: %v", err)
		}
		if fixture.repository.writes != 1 {
			t.Fatalf("unknown final pointer recovery retried CAS: %d", fixture.repository.writes)
		}
	})
}

func TestHostDeploymentCurrentV2SourceDriftBetweenS1AndS2WritesZero(t *testing.T) {
	tests := []struct {
		name  string
		drift func(*ownerFixtureV2)
	}{
		{"closure", func(fixture *ownerFixtureV2) {
			fixture.builder.mu.Lock()
			fixture.builder.closureSequence = []buildercontract.VerifiedAgentPackageClosureV1{fixture.closure, {}}
			fixture.builder.mu.Unlock()
		}},
		{"resource", func(fixture *ownerFixtureV2) {
			ref := fixture.request.ResourceHandles[0]
			fixture.resources.sequences[ref] = []runtimeports.ResourceHandleCurrentV1{fixture.resources.values[ref], {}}
		}},
		{"service", func(fixture *ownerFixtureV2) {
			ref := fixture.request.ServiceBindings[0]
			fixture.services.sequences[ref] = []contract.HostServiceBindingCurrentV2{fixture.services.values[ref], {}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newOwnerFixtureV2(t)
			test.drift(fixture)
			if _, err := fixture.owner.PublishHostDeploymentCurrentV2(context.Background(), fixture.request); err == nil {
				t.Fatal("S1/S2 source drift was accepted")
			}
			if fixture.repository.writes != 0 {
				t.Fatalf("S1/S2 source drift reached CAS: %d", fixture.repository.writes)
			}
		})
	}
}

type ownerFixtureV2 struct {
	clock      time.Time
	closure    buildercontract.VerifiedAgentPackageClosureV1
	selection  buildercontract.AgentPackageSelectionCurrentV1
	builder    *builderEvidenceV2
	resources  *resourceReaderV2
	services   *serviceReaderV2
	repository *ownerRepositoryV2
	owner      *deployment.CurrentOwnerV2
	request    contract.PublishHostDeploymentCurrentRequestV2
}

func newOwnerFixtureV2(t *testing.T) *ownerFixtureV2 {
	t.Helper()
	closure := verifiedClosureFixtureV2(t)
	expires := ownerNowV2.Add(time.Hour).UnixNano()
	selection := selectionFixtureV2(t, closure, 1, expires)
	bootstrap := bootstrapFixtureV2(t, expires)
	resources := &resourceReaderV2{
		values:    map[runtimeports.ResourceHandleRefV1]runtimeports.ResourceHandleCurrentV1{},
		sequences: map[runtimeports.ResourceHandleRefV1][]runtimeports.ResourceHandleCurrentV1{},
	}
	resourceRefs := make([]runtimeports.ResourceHandleRefV1, 0, len(bootstrap.StatePlaneBindingIDs))
	for _, id := range bootstrap.StatePlaneBindingIDs {
		current := resourceCurrentFixtureV2(t, id, expires)
		resources.values[current.Ref] = current
		resourceRefs = append(resourceRefs, current.Ref)
	}
	services := &serviceReaderV2{
		values:    map[contract.HostServiceBindingRefV1]contract.HostServiceBindingCurrentV2{},
		sequences: map[contract.HostServiceBindingRefV1][]contract.HostServiceBindingCurrentV2{},
	}
	serviceRefs := serviceRefsFixtureV2(t, bootstrap, expires)
	for _, ref := range serviceRefs {
		current, err := contract.SealHostServiceBindingCurrentV2(contract.HostServiceBindingCurrentV2{
			Ref: ref, CheckedUnixNano: ownerNowV2.UnixNano(), ExpiresUnixNano: expires,
		})
		if err != nil {
			t.Fatal(err)
		}
		services.values[ref] = current
	}
	request, err := contract.SealPublishHostDeploymentCurrentRequestV2(contract.PublishHostDeploymentCurrentRequestV2{
		Bootstrap: bootstrap, DeploymentID: "deployment-owner-v2", PackageSelectionRef: selection.Ref,
		ResourceHandles: resourceRefs, ServiceBindings: serviceRefs,
		RequestedUnixNano: ownerNowV2.UnixNano(), RequestedNotAfterUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &ownerFixtureV2{
		clock: ownerNowV2, closure: closure, selection: selection,
		builder: &builderEvidenceV2{exact: map[buildercontract.AgentPackageSelectionCurrentRefV1]buildercontract.AgentPackageSelectionCurrentV1{
			selection.Ref: selection,
		}, current: selection, closure: closure},
		resources: resources, services: services, repository: newOwnerRepositoryV2(), request: request,
	}
	fixture.owner, err = deployment.NewCurrentOwnerV2(deployment.CurrentOwnerConfigV2{
		Builder: fixture.builder, Resources: fixture.resources, Services: fixture.services,
		Repository: fixture.repository, Clock: func() time.Time { return fixture.clock },
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (fixture *ownerFixtureV2) nextSelectionV2(t *testing.T, revision core.Revision) buildercontract.AgentPackageSelectionCurrentV1 {
	t.Helper()
	value := selectionFixtureV2(t, fixture.closure, revision, fixture.request.RequestedNotAfterUnixNano)
	fixture.builder.mu.Lock()
	fixture.builder.exact[value.Ref] = value
	fixture.builder.current = value
	fixture.builder.mu.Unlock()
	return value
}

type builderEvidenceV2 struct {
	mu              sync.Mutex
	exact           map[buildercontract.AgentPackageSelectionCurrentRefV1]buildercontract.AgentPackageSelectionCurrentV1
	current         buildercontract.AgentPackageSelectionCurrentV1
	currentSequence []buildercontract.AgentPackageSelectionCurrentV1
	closure         buildercontract.VerifiedAgentPackageClosureV1
	closureSequence []buildercontract.VerifiedAgentPackageClosureV1
	exactEntered    chan struct{}
	exactRelease    chan struct{}
}

func (reader *builderEvidenceV2) InspectAgentPackageSelectionExactV1(_ context.Context, ref buildercontract.AgentPackageSelectionCurrentRefV1) (buildercontract.AgentPackageSelectionCurrentV1, error) {
	reader.mu.Lock()
	value, ok := reader.exact[ref]
	entered, release := reader.exactEntered, reader.exactRelease
	reader.exactEntered, reader.exactRelease = nil, nil
	reader.mu.Unlock()
	if entered != nil {
		close(entered)
		<-release
	}
	if !ok {
		return buildercontract.AgentPackageSelectionCurrentV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "selection missing")
	}
	return buildercontract.CloneAgentPackageSelectionCurrentV1(value), nil
}

func (reader *builderEvidenceV2) InspectAgentPackageSelectionCurrentV1(_ context.Context, _ string) (buildercontract.AgentPackageSelectionCurrentV1, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if len(reader.currentSequence) != 0 {
		value := reader.currentSequence[0]
		reader.currentSequence = reader.currentSequence[1:]
		return buildercontract.CloneAgentPackageSelectionCurrentV1(value), nil
	}
	return buildercontract.CloneAgentPackageSelectionCurrentV1(reader.current), nil
}

func (reader *builderEvidenceV2) LoadVerifiedAgentPackageClosureV1(_ context.Context, _ buildercontract.AgentPackageRefV1) (buildercontract.VerifiedAgentPackageClosureV1, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if len(reader.closureSequence) != 0 {
		value := reader.closureSequence[0]
		reader.closureSequence = reader.closureSequence[1:]
		return buildercontract.CloneVerifiedAgentPackageClosureV1(value), nil
	}
	return buildercontract.CloneVerifiedAgentPackageClosureV1(reader.closure), nil
}

type resourceReaderV2 struct {
	values    map[runtimeports.ResourceHandleRefV1]runtimeports.ResourceHandleCurrentV1
	sequences map[runtimeports.ResourceHandleRefV1][]runtimeports.ResourceHandleCurrentV1
}

func (reader *resourceReaderV2) InspectResourceHandleCurrentV1(_ context.Context, ref runtimeports.ResourceHandleRefV1) (runtimeports.ResourceHandleCurrentV1, error) {
	if values := reader.sequences[ref]; len(values) != 0 {
		value := values[0]
		reader.sequences[ref] = values[1:]
		return value, nil
	}
	value, ok := reader.values[ref]
	if !ok {
		return runtimeports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "resource missing")
	}
	return value, nil
}

type serviceReaderV2 struct {
	values    map[contract.HostServiceBindingRefV1]contract.HostServiceBindingCurrentV2
	sequences map[contract.HostServiceBindingRefV1][]contract.HostServiceBindingCurrentV2
}

func (reader *serviceReaderV2) InspectHostServiceBindingCurrentV2(_ context.Context, ref contract.HostServiceBindingRefV1) (contract.HostServiceBindingCurrentV2, error) {
	if values := reader.sequences[ref]; len(values) != 0 {
		value := values[0]
		reader.sequences[ref] = values[1:]
		return value, nil
	}
	value, ok := reader.values[ref]
	if !ok {
		return contract.HostServiceBindingCurrentV2{}, contract.NewError(contract.ErrorNotFound, "service_missing", "service missing")
	}
	return value, nil
}

type ownerRepositoryV2 struct {
	mu                   sync.Mutex
	history              map[contract.HostDeploymentCurrentRefV2]contract.HostDeploymentCurrentV2
	current              map[string]contract.HostDeploymentCurrentV2
	writes               int
	unknown              bool
	unknownAndAdvance    bool
	currentReads         int
	advanceOnCurrentRead int
}

func newOwnerRepositoryV2() *ownerRepositoryV2 {
	return &ownerRepositoryV2{
		history: map[contract.HostDeploymentCurrentRefV2]contract.HostDeploymentCurrentV2{},
		current: map[string]contract.HostDeploymentCurrentV2{},
	}
}

func deploymentKeyV2(hostID, deploymentID string) string { return hostID + "\x00" + deploymentID }

func (repository *ownerRepositoryV2) CompareAndSwapStoredHostDeploymentCurrentV2(_ context.Context, expected contract.HostDeploymentCurrentRefV2, next contract.HostDeploymentCurrentV2) (contract.HostDeploymentCurrentV2, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.writes++
	key := deploymentKeyV2(next.Ref.HostID, next.Ref.DeploymentID)
	actual, exists := repository.current[key]
	if (exists && actual.Ref != expected) || (!exists && !expected.IsZero()) {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorConflict, "cas_conflict", "CAS conflict")
	}
	repository.history[next.Ref] = contract.CloneHostDeploymentCurrentV2(next)
	repository.current[key] = contract.CloneHostDeploymentCurrentV2(next)
	if repository.unknownAndAdvance {
		advanced := contract.CloneHostDeploymentCurrentV2(next)
		advanced.Ref.Revision++
		advanced.Ref.Digest, advanced.ProjectionDigest = "", ""
		advanced, _ = contract.SealHostDeploymentCurrentV2(advanced)
		repository.history[advanced.Ref] = advanced
		repository.current[key] = advanced
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorUnknownOutcome, "lost_reply", "lost reply")
	}
	if repository.unknown {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorUnknownOutcome, "lost_reply", "lost reply")
	}
	return contract.CloneHostDeploymentCurrentV2(next), nil
}

func (repository *ownerRepositoryV2) InspectStoredHostDeploymentExactV2(_ context.Context, ref contract.HostDeploymentCurrentRefV2) (contract.HostDeploymentCurrentV2, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	value, ok := repository.history[ref]
	if !ok {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorNotFound, "deployment_missing", "deployment missing")
	}
	return contract.CloneHostDeploymentCurrentV2(value), nil
}

func (repository *ownerRepositoryV2) InspectStoredHostDeploymentCurrentV2(_ context.Context, hostID, deploymentID string) (contract.HostDeploymentCurrentV2, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	key := deploymentKeyV2(hostID, deploymentID)
	value, ok := repository.current[key]
	if !ok {
		return contract.HostDeploymentCurrentV2{}, contract.NewError(contract.ErrorNotFound, "deployment_missing", "deployment missing")
	}
	repository.currentReads++
	if repository.advanceOnCurrentRead != 0 && repository.currentReads == repository.advanceOnCurrentRead {
		advanced := contract.CloneHostDeploymentCurrentV2(value)
		advanced.Ref.Revision++
		advanced.Ref.Digest, advanced.ProjectionDigest = "", ""
		advanced, _ = contract.SealHostDeploymentCurrentV2(advanced)
		repository.history[advanced.Ref] = advanced
		repository.current[key] = advanced
		value = advanced
	}
	return contract.CloneHostDeploymentCurrentV2(value), nil
}

func verifiedClosureFixtureV2(t *testing.T) buildercontract.VerifiedAgentPackageClosureV1 {
	t.Helper()
	input := assemblytestkit.ValidInput()
	compiled, err := assemblycompiler.New().Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := assemblycontract.NewAssemblyPublicationBundleV2(input.ScopeRef, compiled)
	if err != nil {
		t.Fatal(err)
	}
	publicationRef := assemblycontract.AssemblyPublicationRefV2{
		PublicationID: publication.Publication.PublicationID,
		Revision:      publication.Publication.Revision,
		Digest:        publication.Publication.Digest,
	}
	lock, err := buildercontract.SealLockManifestV1(buildercontract.AgentPackageLockManifestV1{
		DefinitionRef:      definitioncontract.AgentDefinitionRefV1{DefinitionID: "agent/host-v2", Revision: 1, Digest: core.DigestBytes([]byte("definition"))},
		ResolvedPlanRef:    assemblercontract.ResolvedAgentPlanRefV1{PlanID: "plan/host-v2", Revision: 1, Digest: core.DigestBytes([]byte("plan"))},
		ResolutionFactsRef: assemblercontract.ResolutionFactsRefV1{FactsID: "facts/host-v2", Revision: 1, Digest: core.DigestBytes([]byte("facts"))},
		CatalogRef:         assemblercontract.ComponentReleaseCatalogRefV1{CatalogID: "catalog/host-v2", Revision: 1, Digest: core.DigestBytes([]byte("catalog"))},
		ComponentReleaseRefs: []assemblercontract.ComponentReleaseRefV1{{
			ReleaseID: "release/host-v2", Revision: 1, Digest: core.DigestBytes([]byte("release")), ComponentID: "praxis/host-v2",
		}},
		BindingPlanDigest: core.DigestBytes([]byte("binding-plan")), AssemblyInputDigest: input.Digest,
		FrozenUnixNano: input.CreatedUnixNano, HarnessCompilerVersion: assemblycontract.CompilerVersionV1,
		PublicationRef: publicationRef, GenerationRef: publication.Publication.Artifacts.Generation,
		ManifestRef: publication.Publication.Artifacts.Manifest, GraphRef: publication.Publication.Artifacts.Graph,
		HandoffRef: publication.Publication.Artifacts.Handoff,
	})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := buildercontract.SealPackageV1(buildercontract.AgentPackageV1{Lock: lock})
	if err != nil {
		t.Fatal(err)
	}
	closure, err := buildercontract.SealVerifiedAgentPackageClosureV1(pkg, publication)
	if err != nil {
		t.Fatal(err)
	}
	return closure
}

func selectionFixtureV2(t *testing.T, closure buildercontract.VerifiedAgentPackageClosureV1, revision core.Revision, expires int64) buildercontract.AgentPackageSelectionCurrentV1 {
	t.Helper()
	value, err := buildercontract.SealAgentPackageSelectionCurrentV1(buildercontract.AgentPackageSelectionCurrentV1{
		Ref: buildercontract.AgentPackageSelectionCurrentRefV1{
			SelectionID: "selection/host-v2", Revision: revision, ExpiresUnixNano: expires,
		},
		PackageRef: closure.Package.RefV1(), PublicationRef: closure.PublicationRefV2(), ClosureDigest: closure.ClosureDigest,
		CheckedUnixNano: ownerNowV2.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func bootstrapFixtureV2(t *testing.T, expires int64) contract.HostBootstrapConfigV1 {
	t.Helper()
	value, err := contract.SealHostBootstrapConfigV1(contract.HostBootstrapConfigV1{
		HostID: "host-owner-v2", StatePlaneBindingIDs: []string{"state-a", "state-b"},
		DefinitionSourceBindingID: "definition-source", CatalogBindingID: "catalog",
		ResolutionFactsBindingID: "resolution", SecretBrokerBindingID: "secret-broker",
		CredentialRegistryBindingID: "credentials", ProviderEndpointRegistryBindingID: "providers",
		RuntimeServiceBindingIDs: []string{"runtime"}, ApplicationServiceBindingIDs: []string{"application"},
		HarnessServiceBindingIDs: []string{"harness"}, ListenBindingID: "listen",
		DiagnosticsPolicyBindingID: "diagnostics", ShutdownPolicyBindingID: "shutdown",
		EnabledControlAPISurfaces: []string{"inspect", "run", "stop"},
		CreatedUnixNano:           ownerNowV2.Add(-time.Minute).UnixNano(), NotAfterUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func resourceCurrentFixtureV2(t *testing.T, id string, expires int64) runtimeports.ResourceHandleCurrentV1 {
	t.Helper()
	owner := core.OwnerRef{Domain: "praxis.host-test", ID: "resource-owner"}
	currentRef := func(name string) runtimeports.OwnerCurrentRefV1 {
		return runtimeports.OwnerCurrentRefV1{
			Owner: owner, ContractVersion: "praxis.host-test/current/v1", ID: name,
			Revision: 1, Digest: core.DigestBytes([]byte(name)), ExpiresUnixNano: expires,
		}
	}
	value, err := runtimeports.SealResourceHandleCurrentV1(runtimeports.ResourceHandleCurrentV1{
		Ref: runtimeports.ResourceHandleRefV1{
			Owner: owner, ID: id, Revision: 1, Kind: "praxis/sqlite",
			ScopeDigest: core.DigestBytes([]byte("scope-" + id)),
		},
		CleanupContract: currentRef("cleanup-" + id), DeploymentAttestation: currentRef("attestation-" + id),
		CheckedUnixNano: ownerNowV2.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func serviceRefsFixtureV2(t *testing.T, bootstrap contract.HostBootstrapConfigV1, expires int64) []contract.HostServiceBindingRefV1 {
	t.Helper()
	items := []struct {
		role contract.HostServiceBindingRoleV1
		id   string
	}{
		{contract.HostServiceDefinitionSourceV1, bootstrap.DefinitionSourceBindingID},
		{contract.HostServiceCatalogV1, bootstrap.CatalogBindingID},
		{contract.HostServiceResolutionFactsV1, bootstrap.ResolutionFactsBindingID},
		{contract.HostServiceSecretBrokerV1, bootstrap.SecretBrokerBindingID},
		{contract.HostServiceCredentialRegistryV1, bootstrap.CredentialRegistryBindingID},
		{contract.HostServiceProviderRegistryV1, bootstrap.ProviderEndpointRegistryBindingID},
		{contract.HostServiceRuntimeV1, bootstrap.RuntimeServiceBindingIDs[0]},
		{contract.HostServiceApplicationV1, bootstrap.ApplicationServiceBindingIDs[0]},
		{contract.HostServiceHarnessV1, bootstrap.HarnessServiceBindingIDs[0]},
		{contract.HostServiceListenV1, bootstrap.ListenBindingID},
		{contract.HostServiceDiagnosticsV1, bootstrap.DiagnosticsPolicyBindingID},
		{contract.HostServiceShutdownV1, bootstrap.ShutdownPolicyBindingID},
	}
	result := make([]contract.HostServiceBindingRefV1, 0, len(items))
	for _, item := range items {
		result = append(result, contract.HostServiceBindingRefV1{
			Role: item.role, ConfiguredID: item.id,
			BindingRef: contract.ExactRefV1{
				Kind: "praxis.agent-host/service-binding", ID: string(item.role) + "-" + item.id,
				Revision: 1, Digest: contract.DigestV1(core.DigestBytes([]byte(string(item.role) + item.id))),
			},
			Capability: "praxis.host/" + string(item.role), ExpiresUnixNano: expires,
		})
	}
	return result
}
