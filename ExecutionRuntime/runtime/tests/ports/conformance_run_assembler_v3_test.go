package ports_test

import (
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestTrustedRunAssemblerV3IsNotAnApplicationOrComponentCapability(t *testing.T) {
	lifecycle := reflect.TypeOf((*ports.RunLifecycleGovernancePortV3)(nil)).Elem()
	if _, exists := lifecycle.MethodByName("CreatePendingRunV3"); exists {
		t.Fatal("ordinary Application lifecycle still exposes pending Run/Plan creation")
	}
	assembler := reflect.TypeOf((*ports.TrustedRunAssemblerPortV3)(nil)).Elem()
	if _, exists := assembler.MethodByName("CreatePendingRunV3"); !exists {
		t.Fatal("trusted host assembler lost its restricted create method")
	}
	for _, subject := range []conformance.RunAssemblerSubjectV3{
		conformance.RunAssemblerApplicationV3,
		conformance.RunAssemblerComponentAdapterV3,
		conformance.RunAssemblerTestFixtureV3,
	} {
		if report, err := conformance.CheckTrustedRunAssemblerAccessV3(subject, true); err == nil || report.AssemblerEligible || report.ProductionPlanEligible {
			t.Fatalf("%s obtained trusted or production Plan authority: report=%+v err=%v", subject, report, err)
		}
	}
	report, err := conformance.CheckTrustedRunAssemblerAccessV3(conformance.RunAssemblerHostControlPlaneV3, true)
	if err != nil || !report.AssemblerEligible || report.ProductionPlanEligible || report.AssemblerCarriesCertificationProof {
		t.Fatalf("unexpected restricted host assembler report: %+v err=%v", report, err)
	}
	if _, err := conformance.CheckTrustedRunAssemblerAccessV3(conformance.RunAssemblerHostControlPlaneV3, false); err == nil {
		t.Fatal("host subject obtained assembler without explicit dependency injection")
	}
}
