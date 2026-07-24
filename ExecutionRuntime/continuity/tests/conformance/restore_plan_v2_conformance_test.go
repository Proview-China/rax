package conformance_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func TestRestorePlanV2CapabilityIsShapeOnlyAndHasNoExecutionPort(t *testing.T) {
	manifest := conformance.Wave1Manifest()
	if !containsCapability(manifest.Supported, conformance.CapabilityRestorePlanV2) {
		t.Fatal("Restore Plan V2 shape-only capability is not declared")
	}
	if !containsCapability(manifest.Unsupported, "continuity/restore-execute") {
		t.Fatal("Restore execution must remain explicitly unsupported")
	}
	portType := reflect.TypeOf((*ports.RestorePlanGovernancePortV2)(nil)).Elem()
	for i := 0; i < portType.NumMethod(); i++ {
		method := strings.ToLower(portType.Method(i).Name)
		for _, forbidden := range []string{"eligibility", "authorize", "permit", "execute", "stage", "activate", "provider"} {
			if strings.Contains(method, forbidden) {
				t.Fatalf("Restore Plan Owner port contains forbidden method %s", portType.Method(i).Name)
			}
		}
	}
}
