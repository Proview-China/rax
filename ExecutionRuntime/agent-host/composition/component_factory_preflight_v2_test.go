package composition_test

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	buildercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/composition"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var preflightNowV2 = time.Unix(2_401_000_000, 0)

type preflightFactoryV2 struct {
	descriptor contract.ComponentFactoryDescriptorV2
	calls      atomic.Int64
}

func (factory *preflightFactoryV2) DescriptorV2() contract.ComponentFactoryDescriptorV2 {
	return factory.descriptor
}
func (factory *preflightFactoryV2) StartOrInspectComponentV2(context.Context, contract.ComponentStartRequestV2) (ports.ComponentHandleV2, error) {
	factory.calls.Add(1)
	return nil, nil
}
func (factory *preflightFactoryV2) InspectComponentV2(context.Context, contract.ComponentFactoryAttemptRefV2) (ports.ComponentHandleV2, error) {
	factory.calls.Add(1)
	return nil, nil
}

func TestComponentFactoryPreflightV2UsesAuthoritativeBuilderClosureAndCallsNoFactory(t *testing.T) {
	fixture := newPreflightFixtureV2(t)
	receipt, err := fixture.preflight.PreflightComponentFactoryV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.PackageRef != fixture.closure.Package.RefV1() ||
		receipt.PublicationRef != fixture.closure.PublicationRefV2() ||
		receipt.ClosureDigest != contract.DigestV1(fixture.closure.ClosureDigest) ||
		receipt.PackageDescriptor.FactoryID != fixture.request.RegistryKey.FactoryRef.FactoryID {
		t.Fatalf("receipt did not seal authoritative Builder closure: %+v", receipt)
	}
	if !reflect.DeepEqual(receipt.Selection, fixture.selection) {
		t.Fatalf("receipt did not seal the complete authoritative Builder selection: %+v", receipt.Selection)
	}
	if fixture.factory.calls.Load() != 0 {
		t.Fatalf("preflight invoked executable factory: %d", fixture.factory.calls.Load())
	}
	spliced := receipt
	spliced.PackageDescriptor.ModuleRef = "praxis.fixture/other-module"
	if err = spliced.Validate(); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("receipt accepted package descriptor splice: %v", err)
	}
	resource := func(id string) contract.ComponentResourceRefV2 {
		return contract.ComponentResourceRefV2{
			OwnerDomain: "praxis.fixture", OwnerID: "resource-owner", ID: id, Revision: 1,
			Digest: contract.DigestV1(core.DigestBytes([]byte("resource-" + id))),
			Kind:   "praxis.fixture/resource", ScopeDigest: contract.DigestV1(core.DigestBytes([]byte("scope-" + id))),
			ExpiresUnixNano: receipt.ExpiresUnixNano,
		}
	}
	withResources := contract.CloneComponentFactoryPreflightRequestV2(receipt.Request)
	withResources.ResourceRefs = []contract.ComponentResourceRefV2{resource("b"), resource("a")}
	withResources.AttemptID, withResources.RequestDigest = "", ""
	withResources, err = contract.SealComponentFactoryPreflightRequestV2(withResources)
	if err != nil {
		t.Fatal(err)
	}
	nonCanonical := withResources
	nonCanonical.ResourceRefs[0], nonCanonical.ResourceRefs[1] = nonCanonical.ResourceRefs[1], nonCanonical.ResourceRefs[0]
	if err = nonCanonical.Validate(); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("request accepted non-canonical resource wire: %v", err)
	}
	duplicate := contract.CloneComponentFactoryPreflightRequestV2(receipt.Request)
	duplicate.AttemptID, duplicate.RequestDigest = "", ""
	duplicate.ResourceRefs = []contract.ComponentResourceRefV2{resource("a"), resource("a")}
	if _, err = contract.SealComponentFactoryPreflightRequestV2(duplicate); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("request accepted duplicate resource: %v", err)
	}
}

func TestComponentFactoryPreflightV2RejectsS1S2SelectionDriftAndExpiryEquality(t *testing.T) {
	t.Run("selection drift", func(t *testing.T) {
		fixture := newPreflightFixtureV2(t)
		drift := fixture.selection
		drift.Ref.Revision = 2
		drift.Ref.Digest, drift.ProjectionDigest = "", ""
		var err error
		drift, err = buildercontract.SealAgentPackageSelectionCurrentV1(drift)
		if err != nil {
			t.Fatal(err)
		}
		fixture.builder.sequence = []buildercontract.AgentPackageSelectionCurrentV1{
			fixture.selection, fixture.selection, drift,
		}
		if _, err = fixture.preflight.PreflightComponentFactoryV2(context.Background(), fixture.request); err == nil {
			t.Fatal("S1/S2 selection drift was accepted")
		}
		if fixture.factory.calls.Load() != 0 {
			t.Fatal("selection drift invoked factory")
		}
	})
	t.Run("now equals expiry", func(t *testing.T) {
		fixture := newPreflightFixtureV2(t)
		fixture.preflight, _ = composition.NewComponentFactoryPreflightV2(composition.ComponentFactoryPreflightConfigV2{
			Deployments: fixture.deployment, Builder: fixture.builder, Resources: noResourceReaderV2{},
			Registry: fixture.registry, Conformance: fixture.conformance, Dependencies: noDependencyReaderV2{},
			Clock: func() time.Time { return time.Unix(0, fixture.request.RequestedNotAfterUnixNano) },
		})
		if _, err := fixture.preflight.PreflightComponentFactoryV2(context.Background(), fixture.request); !contract.HasCode(err, contract.ErrorPrecondition) {
			t.Fatalf("now==expiry accepted: %v", err)
		}
	})
}

func TestComponentFactoryPreflightV2NonEmptyInputsCloseCheckedAndExpiry(t *testing.T) {
	fixture := newPreflightFixtureWithInputsV2(t, true)
	receipt, err := fixture.preflight.PreflightComponentFactoryV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.ResourceCurrents) != 1 || len(receipt.Dependencies) != 1 {
		t.Fatalf("non-empty Resource/Dependency currents were not sealed: resources=%d dependencies=%d", len(receipt.ResourceCurrents), len(receipt.Dependencies))
	}
	expectedChecked := fixture.request.RequestedUnixNano
	for _, checked := range []int64{
		fixture.deployment.value.CheckedUnixNano,
		fixture.selection.CheckedUnixNano,
		fixture.conformance.value.CheckedUnixNano,
		fixture.resourceCurrent.CheckedUnixNano,
		fixture.dependencyCurrent.CheckedUnixNano,
	} {
		if checked > expectedChecked {
			expectedChecked = checked
		}
	}
	expectedExpires := fixture.request.RequestedNotAfterUnixNano
	for _, expires := range []int64{
		fixture.deployment.value.ExpiresUnixNano,
		fixture.selection.ExpiresUnixNano,
		fixture.conformance.value.ExpiresUnixNano,
		fixture.resourceCurrent.ExpiresUnixNano,
		fixture.dependencyCurrent.ExpiresUnixNano,
	} {
		if expires < expectedExpires {
			expectedExpires = expires
		}
	}
	if receipt.CheckedUnixNano != expectedChecked ||
		receipt.CheckedUnixNano != fixture.resourceCurrent.CheckedUnixNano {
		t.Fatalf("receipt Checked is not max(all authoritative currents): got=%d want=%d", receipt.CheckedUnixNano, expectedChecked)
	}
	if receipt.ExpiresUnixNano != expectedExpires ||
		receipt.ExpiresUnixNano != fixture.dependencyCurrent.ExpiresUnixNano {
		t.Fatalf("receipt Expires is not min(all authoritative currents): got=%d want=%d", receipt.ExpiresUnixNano, expectedExpires)
	}

	atExpiry, err := composition.NewComponentFactoryPreflightV2(composition.ComponentFactoryPreflightConfigV2{
		Deployments: fixture.deployment, Builder: fixture.builder,
		Resources: fixture.resources, Registry: fixture.registry,
		Conformance: fixture.conformance, Dependencies: fixture.dependencies,
		Clock: func() time.Time { return time.Unix(0, expectedExpires) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = atExpiry.PreflightComponentFactoryV2(context.Background(), fixture.request); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("now==minimum authoritative expiry was accepted: %v", err)
	}
	if fixture.factory.calls.Load() != 0 {
		t.Fatalf("non-empty preflight or expiry failure invoked factory/provider/effect: %d", fixture.factory.calls.Load())
	}
}

func TestComponentFactoryPreflightV2RejectsEveryS1S2AuthoritativeDriftAxis(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, *preflightFixtureV2)
	}{
		{"deployment", func(t *testing.T, fixture *preflightFixtureV2) {
			drift := contract.CloneHostDeploymentCurrentV2(fixture.deployment.value)
			drift.Ref.Revision++
			drift.Ref.Digest, drift.ProjectionDigest = "", ""
			var err error
			drift, err = contract.SealHostDeploymentCurrentV2(drift)
			if err != nil {
				t.Fatal(err)
			}
			fixture.deployment.sequence = []contract.HostDeploymentCurrentV2{fixture.deployment.value, drift}
		}},
		{"selection", func(t *testing.T, fixture *preflightFixtureV2) {
			drift := buildercontract.CloneAgentPackageSelectionCurrentV1(fixture.selection)
			drift.Ref.Revision++
			drift.Ref.Digest, drift.ProjectionDigest = "", ""
			var err error
			drift, err = buildercontract.SealAgentPackageSelectionCurrentV1(drift)
			if err != nil {
				t.Fatal(err)
			}
			fixture.builder.sequence = []buildercontract.AgentPackageSelectionCurrentV1{
				fixture.selection, fixture.selection, drift,
			}
		}},
		{"closure", func(t *testing.T, fixture *preflightFixtureV2) {
			lock := buildercontract.CloneLockManifestV1(fixture.closure.Package.Lock)
			lock.DefinitionRef.Digest = core.DigestBytes([]byte("s2-closure-definition"))
			lock.Digest = ""
			lock, err := buildercontract.SealLockManifestV1(lock)
			if err != nil {
				t.Fatal(err)
			}
			pkg, err := buildercontract.SealPackageV1(buildercontract.AgentPackageV1{Lock: lock})
			if err != nil {
				t.Fatal(err)
			}
			drift, err := buildercontract.SealVerifiedAgentPackageClosureV1(pkg, fixture.closure.Publication)
			if err != nil {
				t.Fatal(err)
			}
			fixture.builder.closures = []buildercontract.VerifiedAgentPackageClosureV1{fixture.closure, drift}
		}},
		{"registry_registration", func(t *testing.T, fixture *preflightFixtureV2) {
			packaged := fixture.closure.Publication.Manifest.Factories[0]
			packaged.ModuleRef = "praxis.fixture/s2-registry-module"
			descriptor := hostFactoryDescriptorV2(t, packaged)
			conformance := hostFactoryConformanceAtV2(
				t,
				descriptor,
				time.Unix(0, fixture.conformance.value.CheckedUnixNano),
				fixture.conformance.value.ExpiresUnixNano,
			)
			drift, err := contract.SealComponentFactoryRegistrationV2(descriptor, conformance)
			if err != nil {
				t.Fatal(err)
			}
			fixture.registry.sequence = []contract.ComponentFactoryRegistrationV2{fixture.registration, drift}
		}},
		{"conformance", func(t *testing.T, fixture *preflightFixtureV2) {
			drift := fixture.conformance.value
			drift.Ref.Revision++
			drift.Ref.Digest, drift.ProjectionDigest = "", ""
			var err error
			drift, err = contract.SealComponentFactoryConformanceCurrentV2(drift)
			if err != nil {
				t.Fatal(err)
			}
			fixture.conformance.sequence = []contract.ComponentFactoryConformanceCurrentV2{fixture.conformance.value, drift}
		}},
		{"resource", func(t *testing.T, fixture *preflightFixtureV2) {
			drift := fixture.resourceCurrent
			drift.CheckedUnixNano--
			drift.Ref.Digest, drift.ProjectionDigest = "", ""
			var err error
			drift, err = runtimeports.SealResourceHandleCurrentV1(drift)
			if err != nil {
				t.Fatal(err)
			}
			fixture.resources.sequence = []runtimeports.ResourceHandleCurrentV1{fixture.resourceCurrent, drift}
		}},
		{"dependency", func(t *testing.T, fixture *preflightFixtureV2) {
			drift := fixture.dependencyCurrent
			drift.CheckedUnixNano--
			drift.Ref.Digest, drift.ProjectionDigest = "", ""
			var err error
			drift, err = contract.SealComponentDependencyCurrentV2(drift)
			if err != nil {
				t.Fatal(err)
			}
			fixture.dependencies.sequence = []contract.ComponentDependencyCurrentV2{fixture.dependencyCurrent, drift}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPreflightFixtureWithInputsV2(t, true)
			test.setup(t, fixture)
			if _, err := fixture.preflight.PreflightComponentFactoryV2(context.Background(), fixture.request); err == nil {
				t.Fatalf("S1/S2 %s drift was accepted", test.name)
			}
			if fixture.factory.calls.Load() != 0 {
				t.Fatalf("S1/S2 %s drift invoked factory/provider/effect: %d", test.name, fixture.factory.calls.Load())
			}
			configType := reflect.TypeOf(composition.ComponentFactoryPreflightConfigV2{})
			for _, forbidden := range []string{"Factory", "Store", "Database", "File", "Socket", "Provider", "Network", "Effect"} {
				if _, exists := configType.FieldByName(forbidden); exists {
					t.Fatalf("S1/S2 %s drift exposes forbidden side-effect dependency %s", test.name, forbidden)
				}
			}
		})
	}
}

func TestComponentFactoryPreflightV2ConcurrentDeterminismAndNoEffectDependencies(t *testing.T) {
	fixture := newPreflightFixtureV2(t)
	type resultV2 struct {
		receipt contract.ComponentFactoryPreflightReceiptV2
		err     error
	}
	results := make(chan resultV2, 64)
	for range 64 {
		go func() {
			receipt, err := fixture.preflight.PreflightComponentFactoryV2(context.Background(), fixture.request)
			results <- resultV2{receipt: receipt, err: err}
		}()
	}
	var winner contract.ComponentFactoryPreflightReceiptV2
	for range 64 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if winner.Ref.Digest == "" {
			winner = result.receipt
		} else if !reflect.DeepEqual(winner, result.receipt) {
			t.Fatal("same exact preflight request produced different receipt")
		}
	}
	differentPayload := contract.CloneComponentFactoryPreflightRequestV2(fixture.request)
	differentPayload.StartID = "start-component-v2-conflict"
	conflicts := make(chan error, 64)
	for range 64 {
		go func() {
			_, err := fixture.preflight.PreflightComponentFactoryV2(context.Background(), differentPayload)
			conflicts <- err
		}()
	}
	for range 64 {
		if err := <-conflicts; !contract.HasCode(err, contract.ErrorConflict) {
			t.Fatalf("same AttemptID accepted a different concurrent payload: %v", err)
		}
	}
	if fixture.factory.calls.Load() != 0 {
		t.Fatalf("concurrent preflight invoked factory: %d", fixture.factory.calls.Load())
	}
	configType := reflect.TypeOf(composition.ComponentFactoryPreflightConfigV2{})
	for _, forbidden := range []string{"Factory", "Store", "Database", "Provider", "Network", "Effect"} {
		if _, exists := configType.FieldByName(forbidden); exists {
			t.Fatalf("preflight config exposes forbidden effect dependency %s", forbidden)
		}
	}
}

func TestComponentFactoryPreflightV2RejectsEmptyMediaTypeTokensBeforeFactoryEffect(t *testing.T) {
	for _, test := range []struct {
		name      string
		mediaType string
	}{
		{"empty_type", "/json"},
		{"empty_subtype", "application/"},
		{"empty_type_and_subtype", "/"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPreflightFixtureV2(t)
			descriptor := fixture.registration.Descriptor
			descriptor.InputSchema.MediaType = test.mediaType
			if err := descriptor.Validate(); !contract.HasCode(err, contract.ErrorInvalidArgument) {
				t.Fatalf("public Validate accepted media type %q: %v", test.mediaType, err)
			}
			descriptor.Ref.Digest, descriptor.DescriptorDigest = "", ""
			if _, err := contract.SealComponentFactoryDescriptorV2(descriptor); !contract.HasCode(err, contract.ErrorInvalidArgument) {
				t.Fatalf("public Seal accepted media type %q: %v", test.mediaType, err)
			}

			fixture.registry.value.Descriptor.InputSchema.MediaType = test.mediaType
			if _, err := fixture.preflight.PreflightComponentFactoryV2(context.Background(), fixture.request); !contract.HasCode(err, contract.ErrorInvalidArgument) {
				t.Fatalf("preflight accepted media type %q: %v", test.mediaType, err)
			}
			if fixture.factory.calls.Load() != 0 {
				t.Fatalf("invalid media type %q invoked factory/provider/effect: %d", test.mediaType, fixture.factory.calls.Load())
			}
		})
	}
}

func TestComponentFactoryPreflightV2SameRequestIsStableAcrossClockProgress(t *testing.T) {
	fixture := newPreflightFixtureV2(t)
	var ticks atomic.Int64
	preflight, err := composition.NewComponentFactoryPreflightV2(composition.ComponentFactoryPreflightConfigV2{
		Deployments: fixture.deployment, Builder: fixture.builder, Resources: noResourceReaderV2{},
		Registry: fixture.registry, Conformance: fixture.conformance, Dependencies: noDependencyReaderV2{},
		Clock: func() time.Time {
			return preflightNowV2.Add(time.Duration(ticks.Add(1)) * time.Nanosecond)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := preflight.PreflightComponentFactoryV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := preflight.PreflightComponentFactoryV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.Ref.Digest != second.Ref.Digest {
		t.Fatalf("same Request/Attempt changed receipt across clock progress: first=%+v second=%+v", first.Ref, second.Ref)
	}
	if first.CheckedUnixNano != preflightNowV2.UnixNano() {
		t.Fatalf("receipt checked epoch is not the deterministic authoritative maximum: got=%d want=%d", first.CheckedUnixNano, preflightNowV2.UnixNano())
	}
}

func TestComponentFactoryPreflightV2CheckedEpochIncludesLaterOwnerCurrent(t *testing.T) {
	fixture := newPreflightFixtureV2(t)
	request := contract.CloneComponentFactoryPreflightRequestV2(fixture.request)
	request.AttemptID, request.RequestDigest = "", ""
	request.RequestedUnixNano = preflightNowV2.Add(-time.Minute).UnixNano()
	request, err := contract.SealComponentFactoryPreflightRequestV2(request)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := fixture.preflight.PreflightComponentFactoryV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.CheckedUnixNano != preflightNowV2.UnixNano() ||
		receipt.CheckedUnixNano <= request.RequestedUnixNano {
		t.Fatalf("later Owner current was omitted from checked maximum: receipt=%d request=%d", receipt.CheckedUnixNano, request.RequestedUnixNano)
	}
	tampered := contract.CloneComponentFactoryPreflightReceiptV2(receipt)
	tampered.Ref.Digest, tampered.ProjectionDigest = "", ""
	tampered.CheckedUnixNano = request.RequestedUnixNano
	if _, err = contract.SealComponentFactoryPreflightReceiptV2(tampered); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("receipt accepted checked epoch before authoritative Owner current: %v", err)
	}
}

func TestComponentFactoryPreflightV2AttemptIdentityRejectsDifferentPayload(t *testing.T) {
	fixture := newPreflightFixtureV2(t)
	expires := fixture.request.RequestedNotAfterUnixNano
	resource := contract.ComponentResourceRefV2{
		OwnerDomain: "praxis.fixture", OwnerID: "resource-owner", ID: "resource-a", Revision: 1,
		Digest: contract.DigestV1(core.DigestBytes([]byte("resource-a"))), Kind: "praxis.fixture/resource",
		ScopeDigest: contract.DigestV1(core.DigestBytes([]byte("scope-a"))), ExpiresUnixNano: expires,
	}
	dependency := contract.ComponentInstanceRefV2{
		OwnerID: "praxis.fixture/component-owner", InstanceID: "dependency-a", Revision: 1,
		Digest:     contract.DigestV1(core.DigestBytes([]byte("dependency-a"))),
		Capability: "praxis.fixture/dependency", ExpiresUnixNano: expires,
	}
	module := fixture.closure.Publication.Manifest.Factories[0]
	module.ModuleRef = "praxis.fixture/alternate-module"
	alternateDescriptor := hostFactoryDescriptorV2(t, module)
	alternateConformance := hostFactoryConformanceV2(t, alternateDescriptor, expires)
	alternateKey, err := contract.SealComponentFactoryRegistryKeyV2(alternateDescriptor, alternateConformance)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*contract.ComponentFactoryPreflightRequestV2)
	}{
		{"host", func(value *contract.ComponentFactoryPreflightRequestV2) {
			value.HostID = "host-component-v2-alternate"
			value.DeploymentRef.HostID = value.HostID
		}},
		{"start", func(value *contract.ComponentFactoryPreflightRequestV2) {
			value.StartID = "start-component-v2-alternate"
		}},
		{"deployment", func(value *contract.ComponentFactoryPreflightRequestV2) {
			value.DeploymentRef.DeploymentID = "deployment-component-v2-alternate"
		}},
		{"resource", func(value *contract.ComponentFactoryPreflightRequestV2) {
			value.ResourceRefs = []contract.ComponentResourceRefV2{resource}
		}},
		{"dependency", func(value *contract.ComponentFactoryPreflightRequestV2) {
			value.DependencyRefs = []contract.ComponentInstanceRefV2{dependency}
		}},
		{"not_after", func(value *contract.ComponentFactoryPreflightRequestV2) {
			value.RequestedNotAfterUnixNano--
		}},
		{"requested_at", func(value *contract.ComponentFactoryPreflightRequestV2) {
			value.RequestedUnixNano--
		}},
		{"registry", func(value *contract.ComponentFactoryPreflightRequestV2) {
			value.RegistryKey = alternateKey
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			alternate := contract.CloneComponentFactoryPreflightRequestV2(fixture.request)
			alternate.AttemptID, alternate.RequestDigest = "", ""
			test.mutate(&alternate)
			sealedAlternate, sealErr := contract.SealComponentFactoryPreflightRequestV2(alternate)
			if sealErr != nil {
				t.Fatalf("alternate payload is not independently valid: %v", sealErr)
			}
			if sealedAlternate.AttemptID == fixture.request.AttemptID {
				t.Fatal("different payload derived the same AttemptID")
			}
			alternate.AttemptID = fixture.request.AttemptID
			if _, sealErr = contract.SealComponentFactoryPreflightRequestV2(alternate); !contract.HasCode(sealErr, contract.ErrorConflict) {
				t.Fatalf("same AttemptID accepted different %s payload: %v", test.name, sealErr)
			}
		})
	}
}

func TestComponentFactoryPreflightRequestV2CanonicalAliasesHaveOneIdentity(t *testing.T) {
	fixture := newPreflightFixtureV2(t)
	expires := fixture.request.RequestedNotAfterUnixNano
	resource := func(id string) contract.ComponentResourceRefV2 {
		return contract.ComponentResourceRefV2{
			OwnerDomain: "praxis.fixture", OwnerID: "resource-owner", ID: id, Revision: 1,
			Digest: contract.DigestV1(core.DigestBytes([]byte("resource-" + id))), Kind: "praxis.fixture/resource",
			ScopeDigest: contract.DigestV1(core.DigestBytes([]byte("scope-" + id))), ExpiresUnixNano: expires,
		}
	}
	left := contract.CloneComponentFactoryPreflightRequestV2(fixture.request)
	left.AttemptID, left.RequestDigest = "", ""
	left.ResourceRefs = []contract.ComponentResourceRefV2{resource("b"), resource("a")}
	right := contract.CloneComponentFactoryPreflightRequestV2(fixture.request)
	right.AttemptID, right.RequestDigest = "", ""
	right.ResourceRefs = []contract.ComponentResourceRefV2{resource("a"), resource("b")}
	left, err := contract.SealComponentFactoryPreflightRequestV2(left)
	if err != nil {
		t.Fatal(err)
	}
	right, err = contract.SealComponentFactoryPreflightRequestV2(right)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("canonical resource aliases produced different request identities: left=%+v right=%+v", left, right)
	}
}

func TestComponentFactoryPreflightV2RejectsTypedNilReaders(t *testing.T) {
	fixture := newPreflightFixtureV2(t)
	base := composition.ComponentFactoryPreflightConfigV2{
		Deployments: fixture.deployment, Builder: fixture.builder, Resources: &noResourceReaderV2{},
		Registry: fixture.registry, Conformance: fixture.conformance, Dependencies: &noDependencyReaderV2{},
	}
	tests := []struct {
		name   string
		mutate func(*composition.ComponentFactoryPreflightConfigV2)
	}{
		{"deployment", func(value *composition.ComponentFactoryPreflightConfigV2) {
			var reader *deploymentReaderV2
			value.Deployments = reader
		}},
		{"builder", func(value *composition.ComponentFactoryPreflightConfigV2) {
			var reader *preflightBuilderV2
			value.Builder = reader
		}},
		{"resource", func(value *composition.ComponentFactoryPreflightConfigV2) {
			var reader *noResourceReaderV2
			value.Resources = reader
		}},
		{"registry", func(value *composition.ComponentFactoryPreflightConfigV2) {
			var reader *registry.ComponentRegistryV2
			value.Registry = reader
		}},
		{"conformance", func(value *composition.ComponentFactoryPreflightConfigV2) {
			var reader *conformanceReaderV2
			value.Conformance = reader
		}},
		{"dependency", func(value *composition.ComponentFactoryPreflightConfigV2) {
			var reader *noDependencyReaderV2
			value.Dependencies = reader
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := base
			test.mutate(&config)
			if _, err := composition.NewComponentFactoryPreflightV2(config); !contract.HasCode(err, contract.ErrorInvalidArgument) {
				t.Fatalf("typed nil %s reader accepted: %v", test.name, err)
			}
		})
	}
}

func TestComponentFactoryPreflightReceiptV2RejectsRequestDigestSplice(t *testing.T) {
	fixture := newPreflightFixtureV2(t)
	receipt, err := fixture.preflight.PreflightComponentFactoryV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	spliced := receipt
	spliced.Ref.Digest, spliced.ProjectionDigest = "", ""
	spliced.RequestDigest = contract.DigestV1(core.DigestBytes([]byte("another-request")))
	if _, err = contract.SealComponentFactoryPreflightReceiptV2(spliced); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("receipt request digest splice was resealed: %v", err)
	}
}

func TestComponentFactoryPreflightReceiptV2RejectsSelectionClosureAxisSplices(t *testing.T) {
	fixture := newPreflightFixtureV2(t)
	receipt, err := fixture.preflight.PreflightComponentFactoryV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*buildercontract.AgentPackageSelectionCurrentV1)
	}{
		{"package", func(value *buildercontract.AgentPackageSelectionCurrentV1) {
			value.PackageRef.PackageID = "package/component-v2-splice"
		}},
		{"publication", func(value *buildercontract.AgentPackageSelectionCurrentV1) {
			value.PublicationRef.PublicationID = "publication/component-v2-splice"
		}},
		{"closure", func(value *buildercontract.AgentPackageSelectionCurrentV1) {
			value.ClosureDigest = core.DigestBytes([]byte("closure-component-v2-splice"))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spliced := contract.CloneComponentFactoryPreflightReceiptV2(receipt)
			selection := buildercontract.CloneAgentPackageSelectionCurrentV1(spliced.Selection)
			selection.Ref.Digest, selection.ProjectionDigest = "", ""
			test.mutate(&selection)
			selection, sealErr := buildercontract.SealAgentPackageSelectionCurrentV1(selection)
			if sealErr != nil {
				t.Fatalf("selection splice is not independently valid: %v", sealErr)
			}
			spliced.Selection = selection
			spliced.PackageRef = selection.PackageRef
			spliced.PublicationRef = selection.PublicationRef
			spliced.ClosureDigest = contract.DigestV1(selection.ClosureDigest)
			deployment := contract.CloneHostDeploymentCurrentV2(spliced.Deployment)
			deployment.Ref.PackageSelectionRef = selection.Ref
			deployment.Ref.Digest, deployment.ProjectionDigest = "", ""
			deployment, sealErr = contract.SealHostDeploymentCurrentV2(deployment)
			if sealErr != nil {
				t.Fatal(sealErr)
			}
			spliced.Deployment = deployment
			spliced.DeploymentRef = deployment.Ref
			spliced.Request.DeploymentRef = spliced.DeploymentRef
			spliced.Request.AttemptID, spliced.Request.RequestDigest = "", ""
			spliced.Request, sealErr = contract.SealComponentFactoryPreflightRequestV2(spliced.Request)
			if sealErr != nil {
				t.Fatal(sealErr)
			}
			spliced.Ref.AttemptID = spliced.Request.AttemptID
			spliced.RequestDigest = spliced.Request.RequestDigest
			spliced.Ref.Digest, spliced.ProjectionDigest = "", ""
			if _, sealErr = contract.SealComponentFactoryPreflightReceiptV2(spliced); !contract.HasCode(sealErr, contract.ErrorConflict) {
				t.Fatalf("%s selection closure splice was resealed: %v", test.name, sealErr)
			}
		})
	}
}

func TestComponentFactoryPreflightReceiptV2RejectsDescriptorAxisSplices(t *testing.T) {
	fixture := newPreflightFixtureV2(t)
	receipt, err := fixture.preflight.PreflightComponentFactoryV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*assemblycontract.ModuleFactoryDescriptorV1)
	}{
		{"artifact", func(value *assemblycontract.ModuleFactoryDescriptorV1) {
			value.ArtifactDigest = core.DigestBytes([]byte("artifact-splice"))
		}},
		{"module", func(value *assemblycontract.ModuleFactoryDescriptorV1) {
			value.ModuleRef = "praxis.fixture/module-splice"
		}},
		{"capability", func(value *assemblycontract.ModuleFactoryDescriptorV1) {
			value.OutputCapability = "praxis.fixture/capability-splice"
		}},
		{"schema", func(value *assemblycontract.ModuleFactoryDescriptorV1) {
			value.InputSchema.Name = "schema-splice"
		}},
		{"cleanup", func(value *assemblycontract.ModuleFactoryDescriptorV1) {
			value.CleanupContractRef.OwnerCapability = "praxis.fixture/cleanup-splice"
		}},
		{"trust", func(value *assemblycontract.ModuleFactoryDescriptorV1) {
			value.TrustRef.ID = "trust-splice"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			packaged := receipt.PackageDescriptor
			test.mutate(&packaged)
			descriptor := hostFactoryDescriptorV2(t, packaged)
			conformance := hostFactoryConformanceV2(t, descriptor, receipt.ExpiresUnixNano)
			registration, sealErr := contract.SealComponentFactoryRegistrationV2(descriptor, conformance)
			if sealErr != nil {
				t.Fatalf("descriptor splice is not independently valid: %v", sealErr)
			}
			spliced := contract.CloneComponentFactoryPreflightReceiptV2(receipt)
			spliced.PackageDescriptor = packaged
			spliced.Registration = registration
			spliced.ConformanceRef = registration.ConformanceRef
			spliced.Ref.Digest, spliced.ProjectionDigest = "", ""
			if _, sealErr = contract.SealComponentFactoryPreflightReceiptV2(spliced); !contract.HasCode(sealErr, contract.ErrorConflict) {
				t.Fatalf("%s descriptor splice was resealed: %v", test.name, sealErr)
			}
		})
	}
}

func TestComponentFactoryPreflightReceiptV2BodySpliceCannotBeResealed(t *testing.T) {
	fixture := newPreflightFixtureV2(t)
	receipt, err := fixture.preflight.PreflightComponentFactoryV2(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	expires := receipt.ExpiresUnixNano
	resource := contract.ComponentResourceRefV2{
		OwnerDomain: "praxis.fixture", OwnerID: "resource-owner", ID: "resource-splice", Revision: 1,
		Digest: contract.DigestV1(core.DigestBytes([]byte("resource-splice"))), Kind: "praxis.fixture/resource",
		ScopeDigest: contract.DigestV1(core.DigestBytes([]byte("scope-splice"))), ExpiresUnixNano: expires,
	}
	dependency, err := contract.SealComponentDependencyCurrentV2(contract.ComponentDependencyCurrentV2{
		Ref: contract.ComponentInstanceRefV2{
			OwnerID: "praxis.fixture/component-owner", InstanceID: "dependency-splice", Revision: 1,
			Capability: "praxis.fixture/dependency", ExpiresUnixNano: expires,
		},
		InspectBinding: contract.ExactRefV1{
			Kind: "praxis.fixture/inspect", ID: "dependency-inspect", Revision: 1,
			Digest: contract.DigestV1(core.DigestBytes([]byte("dependency-inspect"))),
		},
		CleanupBinding: contract.ExactRefV1{
			Kind: "praxis.fixture/cleanup", ID: "dependency-cleanup", Revision: 1,
			Digest: contract.DigestV1(core.DigestBytes([]byte("dependency-cleanup"))),
		},
		CheckedUnixNano: preflightNowV2.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	module := fixture.closure.Publication.Manifest.Factories[0]
	module.ModuleRef = "praxis.fixture/spliced-module"
	alternateDescriptor := hostFactoryDescriptorV2(t, module)
	alternateConformance := hostFactoryConformanceV2(t, alternateDescriptor, expires)
	alternateRegistration, err := contract.SealComponentFactoryRegistrationV2(alternateDescriptor, alternateConformance)
	if err != nil {
		t.Fatal(err)
	}
	resealRequest := func(value contract.ComponentFactoryPreflightRequestV2, mutate func(*contract.ComponentFactoryPreflightRequestV2)) contract.ComponentFactoryPreflightRequestV2 {
		t.Helper()
		value = contract.CloneComponentFactoryPreflightRequestV2(value)
		value.AttemptID, value.RequestDigest = "", ""
		mutate(&value)
		sealed, sealErr := contract.SealComponentFactoryPreflightRequestV2(value)
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		return sealed
	}
	tests := []struct {
		name   string
		mutate func(*contract.ComponentFactoryPreflightReceiptV2)
	}{
		{"host", func(value *contract.ComponentFactoryPreflightReceiptV2) { value.HostID = "host-splice" }},
		{"start", func(value *contract.ComponentFactoryPreflightReceiptV2) { value.StartID = "start-splice" }},
		{"deployment", func(value *contract.ComponentFactoryPreflightReceiptV2) {
			value.DeploymentRef.DeploymentID = "deployment-splice"
		}},
		{"registration", func(value *contract.ComponentFactoryPreflightReceiptV2) {
			value.Registration = alternateRegistration
			value.ConformanceRef = alternateRegistration.ConformanceRef
		}},
		{"resources", func(value *contract.ComponentFactoryPreflightReceiptV2) {
			value.ResourceRefs = []contract.ComponentResourceRefV2{resource}
		}},
		{"dependencies", func(value *contract.ComponentFactoryPreflightReceiptV2) {
			value.Dependencies = []contract.ComponentDependencyCurrentV2{dependency}
		}},
		{"request_window", func(value *contract.ComponentFactoryPreflightReceiptV2) {
			value.Request = resealRequest(value.Request, func(request *contract.ComponentFactoryPreflightRequestV2) {
				request.RequestedNotAfterUnixNano--
			})
		}},
		{"request_body", func(value *contract.ComponentFactoryPreflightReceiptV2) {
			value.Request = resealRequest(value.Request, func(request *contract.ComponentFactoryPreflightRequestV2) {
				request.ResourceRefs = []contract.ComponentResourceRefV2{resource}
			})
		}},
		{"package_ref", func(value *contract.ComponentFactoryPreflightReceiptV2) {
			value.PackageRef.PackageID = "package-splice"
		}},
		{"publication_ref", func(value *contract.ComponentFactoryPreflightReceiptV2) {
			value.PublicationRef.PublicationID = "publication-splice"
		}},
		{"closure_digest", func(value *contract.ComponentFactoryPreflightReceiptV2) {
			value.ClosureDigest = contract.DigestV1(core.DigestBytes([]byte("closure-splice")))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spliced := contract.CloneComponentFactoryPreflightReceiptV2(receipt)
			spliced.Ref.Digest, spliced.ProjectionDigest = "", ""
			test.mutate(&spliced)
			if _, sealErr := contract.SealComponentFactoryPreflightReceiptV2(spliced); sealErr == nil {
				t.Fatalf("%s splice was publicly resealed", test.name)
			}
		})
	}
}

type preflightFixtureV2 struct {
	closure           buildercontract.VerifiedAgentPackageClosureV1
	selection         buildercontract.AgentPackageSelectionCurrentV1
	registration      contract.ComponentFactoryRegistrationV2
	resourceCurrent   runtimeports.ResourceHandleCurrentV1
	dependencyCurrent contract.ComponentDependencyCurrentV2
	builder           *preflightBuilderV2
	deployment        *deploymentReaderV2
	conformance       *conformanceReaderV2
	registry          *registrationReaderV2
	resources         *resourceReaderV2
	dependencies      *dependencyReaderV2
	factory           *preflightFactoryV2
	preflight         *composition.ComponentFactoryPreflightV2
	request           contract.ComponentFactoryPreflightRequestV2
}

func newPreflightFixtureV2(t *testing.T) *preflightFixtureV2 {
	return newPreflightFixtureWithInputsV2(t, false)
}

func newPreflightFixtureWithInputsV2(t *testing.T, withInputs bool) *preflightFixtureV2 {
	t.Helper()
	closure := preflightClosureV2(t)
	requested := preflightNowV2
	selectionChecked := preflightNowV2
	deploymentChecked := preflightNowV2
	conformanceChecked := preflightNowV2
	resourceChecked := preflightNowV2
	dependencyChecked := preflightNowV2
	requestExpires := preflightNowV2.Add(time.Hour).UnixNano()
	selectionExpires := requestExpires
	deploymentExpires := requestExpires
	conformanceExpires := requestExpires
	resourceExpires := requestExpires
	dependencyExpires := requestExpires
	if withInputs {
		requested = preflightNowV2.Add(-10 * time.Minute)
		deploymentChecked = preflightNowV2.Add(-5 * time.Minute)
		selectionChecked = preflightNowV2.Add(-4 * time.Minute)
		conformanceChecked = preflightNowV2.Add(-3 * time.Minute)
		dependencyChecked = preflightNowV2.Add(-2 * time.Minute)
		resourceChecked = preflightNowV2.Add(-time.Minute)
		selectionExpires = preflightNowV2.Add(55 * time.Minute).UnixNano()
		resourceExpires = preflightNowV2.Add(55 * time.Minute).UnixNano()
		deploymentExpires = preflightNowV2.Add(50 * time.Minute).UnixNano()
		conformanceExpires = preflightNowV2.Add(45 * time.Minute).UnixNano()
		dependencyExpires = preflightNowV2.Add(30 * time.Minute).UnixNano()
	}
	selection, err := buildercontract.SealAgentPackageSelectionCurrentV1(buildercontract.AgentPackageSelectionCurrentV1{
		Ref: buildercontract.AgentPackageSelectionCurrentRefV1{
			SelectionID: "selection/component-factory-v2", Revision: 1, ExpiresUnixNano: selectionExpires,
		},
		PackageRef: closure.Package.RefV1(), PublicationRef: closure.PublicationRefV2(), ClosureDigest: closure.ClosureDigest,
		CheckedUnixNano: selectionChecked.UnixNano(), ExpiresUnixNano: selectionExpires,
	})
	if err != nil {
		t.Fatal(err)
	}
	resourceCurrent := runtimeports.ResourceHandleCurrentV1{}
	dependencyCurrent := contract.ComponentDependencyCurrentV2{}
	resourceRefs := []contract.ComponentResourceRefV2{}
	dependencyRefs := []contract.ComponentInstanceRefV2{}
	deploymentResources := []runtimeports.ResourceHandleRefV1{}
	if withInputs {
		resourceCurrent = preflightResourceCurrentV2(t, resourceChecked, resourceExpires)
		dependencyCurrent = preflightDependencyCurrentV2(t, dependencyChecked, dependencyExpires)
		resourceRefs = []contract.ComponentResourceRefV2{componentResourceRefV2(resourceCurrent)}
		dependencyRefs = []contract.ComponentInstanceRefV2{dependencyCurrent.Ref}
		deploymentResources = []runtimeports.ResourceHandleRefV1{resourceCurrent.Ref}
	}
	deployment, err := contract.SealHostDeploymentCurrentV2(contract.HostDeploymentCurrentV2{
		Ref: contract.HostDeploymentCurrentRefV2{
			HostID: "host-component-v2", DeploymentID: "deployment-component-v2", Revision: 1,
			BootstrapDigest: contract.DigestV1(core.DigestBytes([]byte("bootstrap"))), PackageSelectionRef: selection.Ref, ExpiresUnixNano: deploymentExpires,
		},
		ResourceHandles: deploymentResources, ServiceBindings: []contract.HostServiceBindingRefV1{},
		CheckedUnixNano: deploymentChecked.UnixNano(), ExpiresUnixNano: deploymentExpires,
	})
	if err != nil {
		t.Fatal(err)
	}
	module := closure.Publication.Manifest.Factories[0]
	descriptor := hostFactoryDescriptorV2(t, module)
	conformance := hostFactoryConformanceAtV2(t, descriptor, conformanceChecked, conformanceExpires)
	factory := &preflightFactoryV2{descriptor: descriptor}
	registryV2 := registry.NewComponentV2()
	registration, err := registryV2.RegisterComponentFactoryV2(context.Background(), factory, conformance)
	if err != nil {
		t.Fatal(err)
	}
	if err = registryV2.SealComponentFactoryRegistryV2(context.Background()); err != nil {
		t.Fatal(err)
	}
	builder := &preflightBuilderV2{selection: selection, closure: closure}
	deploymentReader := &deploymentReaderV2{value: deployment}
	registrationReader := &registrationReaderV2{value: registration}
	conformanceReader := &conformanceReaderV2{value: conformance}
	resourceReader := &resourceReaderV2{value: resourceCurrent}
	dependencyReader := &dependencyReaderV2{value: dependencyCurrent}
	preflight, err := composition.NewComponentFactoryPreflightV2(composition.ComponentFactoryPreflightConfigV2{
		Deployments: deploymentReader, Builder: builder,
		Resources: resourceReader, Registry: registrationReader,
		Conformance: conformanceReader, Dependencies: dependencyReader,
		Clock: func() time.Time { return preflightNowV2 },
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := contract.SealComponentFactoryPreflightRequestV2(contract.ComponentFactoryPreflightRequestV2{
		HostID: "host-component-v2", StartID: "start-component-v2", DeploymentRef: deployment.Ref,
		RegistryKey: registration.Key, ResourceRefs: resourceRefs,
		DependencyRefs:    dependencyRefs,
		RequestedUnixNano: requested.UnixNano(), RequestedNotAfterUnixNano: requestExpires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &preflightFixtureV2{
		closure: closure, selection: selection, registration: registration,
		resourceCurrent: resourceCurrent, dependencyCurrent: dependencyCurrent,
		builder: builder, deployment: deploymentReader, conformance: conformanceReader,
		registry: registrationReader, resources: resourceReader, dependencies: dependencyReader,
		factory: factory, preflight: preflight, request: request,
	}
}

type preflightBuilderV2 struct {
	selection buildercontract.AgentPackageSelectionCurrentV1
	closure   buildercontract.VerifiedAgentPackageClosureV1
	sequence  []buildercontract.AgentPackageSelectionCurrentV1
	closures  []buildercontract.VerifiedAgentPackageClosureV1
}

func (reader *preflightBuilderV2) InspectAgentPackageSelectionExactV1(context.Context, buildercontract.AgentPackageSelectionCurrentRefV1) (buildercontract.AgentPackageSelectionCurrentV1, error) {
	return reader.nextSelectionV2(), nil
}
func (reader *preflightBuilderV2) InspectAgentPackageSelectionCurrentV1(context.Context, string) (buildercontract.AgentPackageSelectionCurrentV1, error) {
	return reader.nextSelectionV2(), nil
}
func (reader *preflightBuilderV2) LoadVerifiedAgentPackageClosureV1(context.Context, buildercontract.AgentPackageRefV1) (buildercontract.VerifiedAgentPackageClosureV1, error) {
	if len(reader.closures) == 0 {
		return buildercontract.CloneVerifiedAgentPackageClosureV1(reader.closure), nil
	}
	value := reader.closures[0]
	reader.closures = reader.closures[1:]
	return buildercontract.CloneVerifiedAgentPackageClosureV1(value), nil
}
func (reader *preflightBuilderV2) nextSelectionV2() buildercontract.AgentPackageSelectionCurrentV1 {
	if len(reader.sequence) == 0 {
		return buildercontract.CloneAgentPackageSelectionCurrentV1(reader.selection)
	}
	value := reader.sequence[0]
	reader.sequence = reader.sequence[1:]
	return buildercontract.CloneAgentPackageSelectionCurrentV1(value)
}

type deploymentReaderV2 struct {
	value    contract.HostDeploymentCurrentV2
	sequence []contract.HostDeploymentCurrentV2
}

func (reader *deploymentReaderV2) InspectHostDeploymentCurrentV2(context.Context, contract.HostDeploymentCurrentRefV2) (contract.HostDeploymentCurrentV2, error) {
	if len(reader.sequence) == 0 {
		return contract.CloneHostDeploymentCurrentV2(reader.value), nil
	}
	value := reader.sequence[0]
	reader.sequence = reader.sequence[1:]
	return contract.CloneHostDeploymentCurrentV2(value), nil
}

type conformanceReaderV2 struct {
	value    contract.ComponentFactoryConformanceCurrentV2
	sequence []contract.ComponentFactoryConformanceCurrentV2
}

func (reader *conformanceReaderV2) InspectComponentFactoryConformanceCurrentV2(context.Context, contract.ComponentFactoryConformanceCurrentRefV2) (contract.ComponentFactoryConformanceCurrentV2, error) {
	if len(reader.sequence) == 0 {
		return reader.value, nil
	}
	value := reader.sequence[0]
	reader.sequence = reader.sequence[1:]
	return value, nil
}

type registrationReaderV2 struct {
	value    contract.ComponentFactoryRegistrationV2
	sequence []contract.ComponentFactoryRegistrationV2
}

func (reader *registrationReaderV2) InspectComponentFactoryRegistrationV2(context.Context, contract.ComponentFactoryRegistryKeyV2) (contract.ComponentFactoryRegistrationV2, error) {
	if len(reader.sequence) == 0 {
		return reader.value, nil
	}
	value := reader.sequence[0]
	reader.sequence = reader.sequence[1:]
	return value, nil
}

type resourceReaderV2 struct {
	value    runtimeports.ResourceHandleCurrentV1
	sequence []runtimeports.ResourceHandleCurrentV1
}

func (reader *resourceReaderV2) InspectResourceHandleCurrentV1(context.Context, runtimeports.ResourceHandleRefV1) (runtimeports.ResourceHandleCurrentV1, error) {
	if len(reader.sequence) == 0 {
		return reader.value, nil
	}
	value := reader.sequence[0]
	reader.sequence = reader.sequence[1:]
	return value, nil
}

type dependencyReaderV2 struct {
	value    contract.ComponentDependencyCurrentV2
	sequence []contract.ComponentDependencyCurrentV2
}

func (reader *dependencyReaderV2) InspectComponentDependencyCurrentV2(context.Context, contract.ComponentInstanceRefV2) (contract.ComponentDependencyCurrentV2, error) {
	if len(reader.sequence) == 0 {
		return reader.value, nil
	}
	value := reader.sequence[0]
	reader.sequence = reader.sequence[1:]
	return value, nil
}

type noResourceReaderV2 struct{}

func (noResourceReaderV2) InspectResourceHandleCurrentV1(context.Context, runtimeports.ResourceHandleRefV1) (runtimeports.ResourceHandleCurrentV1, error) {
	return runtimeports.ResourceHandleCurrentV1{}, core.NewError(core.ErrorNotFound, core.ReasonComponentMissing, "unexpected resource read")
}

type noDependencyReaderV2 struct{}

func (noDependencyReaderV2) InspectComponentDependencyCurrentV2(context.Context, contract.ComponentInstanceRefV2) (contract.ComponentDependencyCurrentV2, error) {
	return contract.ComponentDependencyCurrentV2{}, contract.NewError(contract.ErrorNotFound, "dependency_missing", "unexpected dependency read")
}

func preflightClosureV2(t *testing.T) buildercontract.VerifiedAgentPackageClosureV1 {
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
		PublicationID: publication.Publication.PublicationID, Revision: publication.Publication.Revision, Digest: publication.Publication.Digest,
	}
	lock, err := buildercontract.SealLockManifestV1(buildercontract.AgentPackageLockManifestV1{
		DefinitionRef:      definitioncontract.AgentDefinitionRefV1{DefinitionID: "agent/component-v2", Revision: 1, Digest: core.DigestBytes([]byte("definition"))},
		ResolvedPlanRef:    assemblercontract.ResolvedAgentPlanRefV1{PlanID: "plan/component-v2", Revision: 1, Digest: core.DigestBytes([]byte("plan"))},
		ResolutionFactsRef: assemblercontract.ResolutionFactsRefV1{FactsID: "facts/component-v2", Revision: 1, Digest: core.DigestBytes([]byte("facts"))},
		CatalogRef:         assemblercontract.ComponentReleaseCatalogRefV1{CatalogID: "catalog/component-v2", Revision: 1, Digest: core.DigestBytes([]byte("catalog"))},
		ComponentReleaseRefs: []assemblercontract.ComponentReleaseRefV1{{
			ReleaseID: "release/component-v2", Revision: 1, Digest: core.DigestBytes([]byte("release")), ComponentID: "praxis/component-v2",
		}},
		BindingPlanDigest: core.DigestBytes([]byte("binding")), AssemblyInputDigest: input.Digest,
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

func hostFactoryDescriptorV2(t *testing.T, value assemblycontract.ModuleFactoryDescriptorV1) contract.ComponentFactoryDescriptorV2 {
	t.Helper()
	schema := func(value runtimeports.SchemaRefV2) contract.ComponentSchemaRefV2 {
		return contract.ComponentSchemaRefV2{
			Namespace: value.Namespace, Name: value.Name, Version: value.Version,
			MediaType: value.MediaType, ContentDigest: contract.DigestV1(value.ContentDigest),
		}
	}
	cleanup, err := contract.SealComponentCleanupContractV2(contract.ComponentCleanupContractV2{
		Ref:             contract.ExactRefV1{Kind: "praxis.component/cleanup-contract", ID: value.CleanupContractRef.Ref.ID, Revision: uint64(value.CleanupContractRef.Ref.Revision), Digest: contract.DigestV1(value.CleanupContractRef.Ref.Digest)},
		OwnerCapability: string(value.CleanupContractRef.OwnerCapability),
		RequestSchema:   schema(value.CleanupContractRef.RequestSchema), ResultSchema: schema(value.CleanupContractRef.ResultSchema),
	})
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := contract.SealComponentFactoryDescriptorV2(contract.ComponentFactoryDescriptorV2{
		Ref:       contract.ComponentFactoryRefV2{FactoryID: value.FactoryID, Revision: 1},
		ModuleRef: value.ModuleRef, ArtifactDigest: contract.DigestV1(value.ArtifactDigest),
		ConstructionMode: string(value.ConstructionMode), InputSchema: schema(value.InputSchema),
		OutputCapability: string(value.OutputCapability), Lifecycle: string(value.Lifecycle),
		CleanupContract: cleanup,
		TrustRef:        contract.ExactRefV1{Kind: "praxis.component/trust", ID: value.TrustRef.ID, Revision: uint64(value.TrustRef.Revision), Digest: contract.DigestV1(value.TrustRef.Digest)},
		Implementation:  contract.ComponentFactoryImplementationOwnerV2,
		ProviderAccess:  contract.ComponentFactoryProviderAccessNoneV2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}

func hostFactoryConformanceV2(t *testing.T, descriptor contract.ComponentFactoryDescriptorV2, expires int64) contract.ComponentFactoryConformanceCurrentV2 {
	return hostFactoryConformanceAtV2(t, descriptor, preflightNowV2, expires)
}

func hostFactoryConformanceAtV2(
	t *testing.T,
	descriptor contract.ComponentFactoryDescriptorV2,
	checked time.Time,
	expires int64,
) contract.ComponentFactoryConformanceCurrentV2 {
	t.Helper()
	evidence := func(name string) contract.ComponentFactoryEvidenceCurrentV2 {
		value, err := contract.SealComponentFactoryEvidenceCurrentV2(contract.ComponentFactoryEvidenceCurrentV2{
			Ref:             contract.ExactRefV1{Kind: "praxis.component/evidence", ID: name, Revision: 1, Digest: contract.DigestV1(core.DigestBytes([]byte(name)))},
			CheckedUnixNano: checked.Add(-time.Minute).UnixNano(), ExpiresUnixNano: expires,
		})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	value, err := contract.SealComponentFactoryConformanceCurrentV2(contract.ComponentFactoryConformanceCurrentV2{
		Ref:        contract.ComponentFactoryConformanceCurrentRefV2{ConformanceID: "praxis.component/conformance", Revision: 1, ExpiresUnixNano: expires},
		FactoryRef: descriptor.Ref, DescriptorDigest: descriptor.DescriptorDigest,
		Certification: evidence("certification"), StaticImportEvidence: evidence("static"),
		NoRawProviderEvidence: evidence("provider"), ZeroEffectEvidence: evidence("effect"),
		Implementation: contract.ComponentFactoryImplementationOwnerV2, ProviderAccess: contract.ComponentFactoryProviderAccessNoneV2,
		CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func preflightResourceCurrentV2(t *testing.T, checked time.Time, expires int64) runtimeports.ResourceHandleCurrentV1 {
	t.Helper()
	owner := core.OwnerRef{Domain: "praxis.fixture", ID: "resource-owner"}
	ownerCurrent := func(id string) runtimeports.OwnerCurrentRefV1 {
		return runtimeports.OwnerCurrentRefV1{
			Owner: owner, ContractVersion: "praxis.fixture/current/v1", ID: id,
			Revision: 1, Digest: core.DigestBytes([]byte(id)), ExpiresUnixNano: expires,
		}
	}
	value, err := runtimeports.SealResourceHandleCurrentV1(runtimeports.ResourceHandleCurrentV1{
		Ref: runtimeports.ResourceHandleRefV1{
			Owner: owner, ID: "resource-component-v2", Revision: 1,
			Kind: "praxis.fixture/resource", ScopeDigest: core.DigestBytes([]byte("resource-scope")),
		},
		CleanupContract:       ownerCurrent("resource-cleanup"),
		DeploymentAttestation: ownerCurrent("resource-deployment"),
		CheckedUnixNano:       checked.UnixNano(),
		ExpiresUnixNano:       expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func componentResourceRefV2(value runtimeports.ResourceHandleCurrentV1) contract.ComponentResourceRefV2 {
	return contract.ComponentResourceRefV2{
		OwnerDomain:     string(value.Ref.Owner.Domain),
		OwnerID:         string(value.Ref.Owner.ID),
		ID:              value.Ref.ID,
		Revision:        uint64(value.Ref.Revision),
		Digest:          contract.DigestV1(value.Ref.Digest),
		Kind:            string(value.Ref.Kind),
		ScopeDigest:     contract.DigestV1(value.Ref.ScopeDigest),
		ExpiresUnixNano: value.Ref.ExpiresUnixNano,
	}
}

func preflightDependencyCurrentV2(t *testing.T, checked time.Time, expires int64) contract.ComponentDependencyCurrentV2 {
	t.Helper()
	value, err := contract.SealComponentDependencyCurrentV2(contract.ComponentDependencyCurrentV2{
		Ref: contract.ComponentInstanceRefV2{
			OwnerID: "praxis.fixture/component-owner", InstanceID: "dependency-component-v2",
			Revision: 1, Capability: "praxis.fixture/dependency", ExpiresUnixNano: expires,
		},
		InspectBinding: contract.ExactRefV1{
			Kind: "praxis.fixture/inspect", ID: "dependency-inspect", Revision: 1,
			Digest: contract.DigestV1(core.DigestBytes([]byte("dependency-inspect"))),
		},
		CleanupBinding: contract.ExactRefV1{
			Kind: "praxis.fixture/cleanup", ID: "dependency-cleanup", Revision: 1,
			Digest: contract.DigestV1(core.DigestBytes([]byte("dependency-cleanup"))),
		},
		CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
