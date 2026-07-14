package ports_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestBindingV2ConformanceMatrixSeparatesRegistrationFromDispatch(t *testing.T) {
	t.Parallel()
	levels := []ports.ConformanceLevel{ports.ConformanceFullyControlled, ports.ConformanceRestrictedControlled, ports.ConformanceContainedObserveOnly, ports.ConformanceRejected}
	for _, level := range levels {
		level := level
		t.Run(string(level), func(t *testing.T) {
			t.Parallel()
			manifest, catalog := bindingV2Fixture(t, "vendor/component-"+string(level), "vendor/kind-"+string(level))
			manifest.Conformance = level
			report, err := conformance.CheckBindingAdapterV2(context.Background(), conformance.BindingAdapterCaseV2{SubjectClass: conformance.SubjectExternalAdapter, Adapter: staticDescriberV2{manifest: manifest}, Catalog: catalog, Clock: func() time.Time { return time.Unix(100, 0) }})
			if err != nil {
				t.Fatal(err)
			}
			if !report.Registered || !report.Probed || report.DispatchEligible {
				t.Fatalf("registration/probe must never imply dispatch: %+v", report)
			}
			if (level != ports.ConformanceRejected) != report.CertificationCandidate {
				t.Fatalf("unexpected certification eligibility: %+v", report)
			}
			if report.BindingEligible || report.ProductionClaimEligible {
				t.Fatalf("self-declared conformance must remain only a certification candidate: %+v", report)
			}
		})
	}
}

func TestExternalFullyControlledSelfDeclarationHasNoAuthority(t *testing.T) {
	t.Parallel()
	manifest, catalog := bindingV2Fixture(t, "custom/component", "custom/kind")
	manifest.Conformance = ports.ConformanceFullyControlled
	report, err := conformance.CheckBindingAdapterV2(context.Background(), conformance.BindingAdapterCaseV2{SubjectClass: conformance.SubjectExternalAdapter, Adapter: staticDescriberV2{manifest: manifest}, Catalog: catalog, Clock: func() time.Time { return time.Unix(100, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	if !report.CertificationCandidate || report.BindingEligible || report.ProductionClaimEligible || report.DispatchEligible {
		t.Fatalf("external self-declaration is not certification, binding, production or dispatch authority: %+v", report)
	}
	imports := conformance.AdapterAllowedImportsV2()
	imports[0] = "attacker/runtime-internal"
	if conformance.AdapterAllowedImportsV2()[0] == imports[0] {
		t.Fatal("adapter import allowlist accessor must return an isolated copy")
	}
	if err := conformance.CheckAdapterRuntimeImportsV2(conformance.AdapterAllowedImportsV2()); err != nil {
		t.Fatalf("documented public adapter imports were rejected: %v", err)
	}
	for _, forbidden := range []string{
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/control",
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel",
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/foundation",
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes",
	} {
		if err := conformance.CheckAdapterRuntimeImportsV2([]string{forbidden}); err == nil {
			t.Fatalf("adapter import scanner allowed Runtime owner package %s", forbidden)
		}
	}
}

func TestBindingV2ConformanceFakeCannotClaimProduction(t *testing.T) {
	t.Parallel()
	manifest, catalog := bindingV2Fixture(t, "vendor/fixture", "vendor/fixture-kind")
	report, err := conformance.CheckBindingAdapterV2(context.Background(), conformance.BindingAdapterCaseV2{SubjectClass: conformance.SubjectTestFixture, Adapter: staticDescriberV2{manifest: manifest}, Catalog: catalog, Clock: func() time.Time { return time.Unix(100, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	if report.ProductionClaimEligible || report.DispatchEligible {
		t.Fatalf("test fixture must never advertise production conformance or dispatch authority: %+v", report)
	}
}
