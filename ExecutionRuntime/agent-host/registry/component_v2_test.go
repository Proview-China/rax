package registry_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/registry"
)

type factoryV2 struct {
	descriptor contract.ComponentFactoryDescriptorV2
	starts     atomic.Int64
}

func (factory *factoryV2) DescriptorV2() contract.ComponentFactoryDescriptorV2 {
	return factory.descriptor
}
func (factory *factoryV2) StartOrInspectComponentV2(context.Context, contract.ComponentStartRequestV2) (ports.ComponentHandleV2, error) {
	factory.starts.Add(1)
	return nil, nil
}
func (factory *factoryV2) InspectComponentV2(context.Context, contract.ComponentFactoryAttemptRefV2) (ports.ComponentHandleV2, error) {
	factory.starts.Add(1)
	return nil, nil
}

func TestComponentRegistryV2SealsExactMetadataWithoutInvokingFactory(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	descriptor, conformance := componentFactoryFixtureV2(t, now)
	factory := &factoryV2{descriptor: descriptor}
	value := registry.NewComponentV2()
	registration, err := value.RegisterComponentFactoryV2(context.Background(), factory, conformance)
	if err != nil {
		t.Fatal(err)
	}
	if factory.starts.Load() != 0 {
		t.Fatal("registration invoked component factory")
	}
	if err = value.SealComponentFactoryRegistryV2(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := value.InspectComponentFactoryRegistrationV2(context.Background(), registration.Key)
	if err != nil || got != registration {
		t.Fatalf("registration=%+v err=%v", got, err)
	}
	resolved, err := value.ResolveComponentFactoryV2(context.Background(), registration.Key)
	if err != nil || resolved != factory {
		t.Fatalf("factory=%T err=%v", resolved, err)
	}
	if factory.starts.Load() != 0 {
		t.Fatal("metadata inspection invoked component factory")
	}
	if registration.ProductionEligible || got.ProductionEligible {
		t.Fatal("structural registration claimed production eligibility")
	}
	if _, err = value.RegisterComponentFactoryV2(context.Background(), factory, conformance); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("sealed registry accepted registration: %v", err)
	}
}

func TestComponentRegistryV2AdmitsSelfDeclaredTestFactoryButNotProduction(t *testing.T) {
	descriptor, conformance := componentFactoryFixtureV2(t, time.Unix(2_400_000_000, 0))
	localTestFactory := &factoryV2{descriptor: descriptor}
	value := registry.NewComponentV2()
	registration, err := value.RegisterComponentFactoryV2(context.Background(), localTestFactory, conformance)
	if err != nil {
		t.Fatalf("structurally valid self-declared test factory was not admitted: %v", err)
	}
	if registration.ProductionEligible {
		t.Fatal("self-declared metadata was treated as verified production provenance")
	}
	if localTestFactory.starts.Load() != 0 {
		t.Fatalf("structural admission invoked local test factory: %d", localTestFactory.starts.Load())
	}
	if err = value.SealComponentFactoryRegistryV2(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := value.InspectComponentFactoryRegistrationV2(context.Background(), registration.Key)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductionEligible {
		t.Fatal("sealed registry upgraded self-declared metadata to production eligibility")
	}
	upgraded := registration
	upgraded.ProductionEligible = true
	if err = upgraded.Validate(); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("registration metadata accepted caller production upgrade: %v", err)
	}
	if localTestFactory.starts.Load() != 0 {
		t.Fatalf("sealed metadata inspection invoked local test factory: %d", localTestFactory.starts.Load())
	}
}

func TestComponentRegistryV2RejectsTypedNilAndDuplicates(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	descriptor, conformance := componentFactoryFixtureV2(t, now)
	value := registry.NewComponentV2()
	var nilFactory *factoryV2
	if _, err := value.RegisterComponentFactoryV2(context.Background(), nilFactory, conformance); !contract.HasCode(err, contract.ErrorInvalidArgument) {
		t.Fatalf("typed nil accepted: %v", err)
	}
	factory := &factoryV2{descriptor: descriptor}
	if _, err := value.RegisterComponentFactoryV2(nil, factory, conformance); !contract.HasCode(err, contract.ErrorInvalidArgument) {
		t.Fatalf("nil context accepted: %v", err)
	}
	if _, err := value.RegisterComponentFactoryV2(context.Background(), factory, conformance); err != nil {
		t.Fatal(err)
	}
	if _, err := value.RegisterComponentFactoryV2(context.Background(), factory, conformance); !contract.HasCode(err, contract.ErrorConflict) {
		t.Fatalf("duplicate accepted: %v", err)
	}
}

func TestComponentRegistryV2ResolveRejectsMutableDescriptorDrift(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	descriptor, conformance := componentFactoryFixtureV2(t, now)
	factory := &factoryV2{descriptor: descriptor}
	value := registry.NewComponentV2()
	registration, err := value.RegisterComponentFactoryV2(context.Background(), factory, conformance)
	if err != nil {
		t.Fatal(err)
	}
	if err = value.SealComponentFactoryRegistryV2(context.Background()); err != nil {
		t.Fatal(err)
	}
	factory.descriptor.ModuleRef = "praxis.component/alias-module"
	if _, err = value.ResolveComponentFactoryV2(context.Background(), registration.Key); err == nil {
		t.Fatal("mutable descriptor alias escaped sealed registry")
	}
	if factory.starts.Load() != 0 {
		t.Fatal("descriptor drift invoked component factory")
	}
}

func TestComponentFactoryConformanceV2RejectsCheckedTimeBeforeEvidence(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	descriptor, conformance := componentFactoryFixtureV2(t, now)
	conformance.Ref.Digest, conformance.ProjectionDigest = "", ""
	conformance.CheckedUnixNano = now.Add(-time.Nanosecond).UnixNano()
	if _, err := contract.SealComponentFactoryConformanceCurrentV2(conformance); !contract.HasCode(err, contract.ErrorPrecondition) {
		t.Fatalf("conformance checked-time rollback accepted: descriptor=%s err=%v", descriptor.Ref.FactoryID, err)
	}
}

func TestComponentRegistryV2RejectsNonExecutableDescriptorAndConformanceAxes(t *testing.T) {
	now := time.Unix(2_400_000_000, 0)
	tests := []struct {
		name              string
		mutateDescriptor  func(*contract.ComponentFactoryDescriptorV2)
		mutateConformance func(*contract.ComponentFactoryConformanceCurrentV2)
	}{
		{
			name: "reference_only",
			mutateDescriptor: func(value *contract.ComponentFactoryDescriptorV2) {
				value.ReferenceOnly = true
			},
			mutateConformance: func(value *contract.ComponentFactoryConformanceCurrentV2) {
				value.ReferenceOnly = true
			},
		},
		{
			name: "raw_provider_access",
			mutateDescriptor: func(value *contract.ComponentFactoryDescriptorV2) {
				value.ProviderAccess = "raw_provider"
			},
			mutateConformance: func(value *contract.ComponentFactoryConformanceCurrentV2) {
				value.ProviderAccess = "raw_provider"
			},
		},
		{
			name: "non_owner_implementation",
			mutateDescriptor: func(value *contract.ComponentFactoryDescriptorV2) {
				value.Implementation = "fixture"
			},
			mutateConformance: func(value *contract.ComponentFactoryConformanceCurrentV2) {
				value.Implementation = "fixture"
			},
		},
	}
	for _, test := range tests {
		t.Run("descriptor/"+test.name, func(t *testing.T) {
			descriptor, conformance := componentFactoryFixtureV2(t, now)
			test.mutateDescriptor(&descriptor)
			if err := descriptor.Validate(); !contract.HasCode(err, contract.ErrorPrecondition) {
				t.Fatalf("public Validate accepted non-executable descriptor: %v", err)
			}
			sealCandidate := descriptor
			sealCandidate.Ref.Digest, sealCandidate.DescriptorDigest = "", ""
			if _, err := contract.SealComponentFactoryDescriptorV2(sealCandidate); !contract.HasCode(err, contract.ErrorPrecondition) {
				t.Fatalf("public Seal accepted non-executable descriptor: %v", err)
			}

			factory := &factoryV2{descriptor: descriptor}
			if _, err := registry.NewComponentV2().RegisterComponentFactoryV2(context.Background(), factory, conformance); !contract.HasCode(err, contract.ErrorPrecondition) {
				t.Fatalf("registry accepted non-executable descriptor: %v", err)
			}
			if factory.starts.Load() != 0 {
				t.Fatalf("descriptor rejection invoked factory: %d", factory.starts.Load())
			}
		})
		t.Run("conformance/"+test.name, func(t *testing.T) {
			descriptor, conformance := componentFactoryFixtureV2(t, now)
			test.mutateConformance(&conformance)
			if err := conformance.Validate(); !contract.HasCode(err, contract.ErrorPrecondition) {
				t.Fatalf("public Validate accepted non-executable conformance: %v", err)
			}
			sealCandidate := conformance
			sealCandidate.Ref.Digest, sealCandidate.ProjectionDigest = "", ""
			if _, err := contract.SealComponentFactoryConformanceCurrentV2(sealCandidate); !contract.HasCode(err, contract.ErrorPrecondition) {
				t.Fatalf("public Seal accepted non-executable conformance: %v", err)
			}

			factory := &factoryV2{descriptor: descriptor}
			if _, err := registry.NewComponentV2().RegisterComponentFactoryV2(context.Background(), factory, conformance); !contract.HasCode(err, contract.ErrorPrecondition) {
				t.Fatalf("registry accepted non-executable conformance: %v", err)
			}
			if factory.starts.Load() != 0 {
				t.Fatalf("conformance rejection invoked factory: %d", factory.starts.Load())
			}
		})
	}
}

func componentFactoryFixtureV2(t *testing.T, now time.Time) (contract.ComponentFactoryDescriptorV2, contract.ComponentFactoryConformanceCurrentV2) {
	t.Helper()
	digest := func(value string) contract.DigestV1 {
		result, err := contract.DigestJSONV1(value)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	schema := func(name string) contract.ComponentSchemaRefV2 {
		return contract.ComponentSchemaRefV2{
			Namespace: "praxis.component", Name: name, Version: "v1",
			MediaType: "application/schema+json", ContentDigest: digest(name),
		}
	}
	cleanup, err := contract.SealComponentCleanupContractV2(contract.ComponentCleanupContractV2{
		Ref:             contract.ExactRefV1{Kind: "praxis.component/cleanup-contract", ID: "cleanup", Revision: 1, Digest: digest("cleanup-ref")},
		OwnerCapability: "praxis.component/cleanup", RequestSchema: schema("cleanup-request"), ResultSchema: schema("cleanup-result"),
	})
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := contract.SealComponentFactoryDescriptorV2(contract.ComponentFactoryDescriptorV2{
		Ref:       contract.ComponentFactoryRefV2{FactoryID: "praxis.component/factory", Revision: 1},
		ModuleRef: "praxis.component/module", ArtifactDigest: digest("artifact"),
		ConstructionMode: contract.ComponentFactoryConstructionTrustedGoV2,
		InputSchema:      schema("input"), OutputCapability: "praxis.component/output",
		Lifecycle: "host", CleanupContract: cleanup,
		TrustRef:       contract.ExactRefV1{Kind: "praxis.component/trust", ID: "trust", Revision: 1, Digest: digest("trust")},
		Implementation: contract.ComponentFactoryImplementationOwnerV2,
		ProviderAccess: contract.ComponentFactoryProviderAccessNoneV2,
	})
	if err != nil {
		t.Fatal(err)
	}
	expires := now.Add(time.Hour).UnixNano()
	evidence := func(name string) contract.ComponentFactoryEvidenceCurrentV2 {
		value, sealErr := contract.SealComponentFactoryEvidenceCurrentV2(contract.ComponentFactoryEvidenceCurrentV2{
			Ref:             contract.ExactRefV1{Kind: "praxis.component/evidence", ID: name, Revision: 1, Digest: digest(name + "-ref")},
			CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
		})
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		return value
	}
	conformance, err := contract.SealComponentFactoryConformanceCurrentV2(contract.ComponentFactoryConformanceCurrentV2{
		Ref:        contract.ComponentFactoryConformanceCurrentRefV2{ConformanceID: "praxis.component/conformance", Revision: 1, ExpiresUnixNano: expires},
		FactoryRef: descriptor.Ref, DescriptorDigest: descriptor.DescriptorDigest,
		Certification: evidence("certification"), StaticImportEvidence: evidence("static"),
		NoRawProviderEvidence: evidence("provider"), ZeroEffectEvidence: evidence("effect"),
		Implementation:  contract.ComponentFactoryImplementationOwnerV2,
		ProviderAccess:  contract.ComponentFactoryProviderAccessNoneV2,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return descriptor, conformance
}
