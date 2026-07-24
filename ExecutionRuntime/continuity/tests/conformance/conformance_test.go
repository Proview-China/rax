package conformance_test

import (
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func TestWave1ManifestIsRestrictedAndExplicitlyUnsupported(t *testing.T) {
	manifest := conformance.Wave1Manifest()
	if err := conformance.Validate(manifest); err != nil {
		t.Fatalf("default manifest failed conformance: %v", err)
	}
	if manifest.ProductionSLA || !manifest.ReferenceOnly || manifest.Level != conformance.RestrictedControlled {
		t.Fatalf("reference backend overclaimed capability: %#v", manifest)
	}
}

func TestTimelineStoreHasNoLegacyBulkReplaceOrEventMutationWrite(t *testing.T) {
	typeOf := reflect.TypeOf((*ports.TimelineProjectionStore)(nil)).Elem()
	for _, forbidden := range []string{"ReplaceLedgerScope", "TombstoneProjection"} {
		if _, ok := typeOf.MethodByName(forbidden); ok {
			t.Fatalf("legacy production bypass %s remains public", forbidden)
		}
	}
}

func TestConformanceRejectsExternalCapabilityOverclaim(t *testing.T) {
	manifest := conformance.Wave1Manifest()
	manifest.Supported = append(manifest.Supported, "continuity/restore-execute")
	if err := conformance.Validate(manifest); err == nil {
		t.Fatal("restore execute overclaim was accepted")
	}
	manifest = conformance.Wave1Manifest()
	manifest.Unsupported = manifest.Unsupported[1:]
	if err := conformance.Validate(manifest); err == nil {
		t.Fatal("missing unsupported declaration was accepted")
	}
}

func TestConformanceCapabilitySetsAreExactAndAdaptersDoNotOverclaimProduction(t *testing.T) {
	baseline := conformance.Wave1Manifest()
	for _, stale := range []string{"continuity/runtime-adapter", "continuity/application-adapter"} {
		if containsConformanceValue(baseline.Unsupported, stale) {
			t.Fatalf("implemented reference adapter is declared wholly unsupported: %s", stale)
		}
	}
	for _, required := range []string{"continuity/production-runtime-root", "continuity/production-application-root"} {
		if !containsConformanceValue(baseline.Unsupported, required) {
			t.Fatalf("production root NO-GO is not explicit: %s", required)
		}
	}

	mutations := []struct {
		name   string
		mutate func(*conformance.Manifest)
	}{
		{"unknown-supported", func(m *conformance.Manifest) { m.Supported = append(m.Supported, "continuity/caller-overclaim") }},
		{"missing-supported", func(m *conformance.Manifest) { m.Supported = m.Supported[1:] }},
		{"duplicate-supported", func(m *conformance.Manifest) { m.Supported = append(m.Supported, m.Supported[0]) }},
		{"unknown-unsupported", func(m *conformance.Manifest) {
			m.Unsupported = append(m.Unsupported, "continuity/caller-invented-no-go")
		}},
		{"duplicate-unsupported", func(m *conformance.Manifest) { m.Unsupported = append(m.Unsupported, m.Unsupported[0]) }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			manifest := conformance.Wave1Manifest()
			mutation.mutate(&manifest)
			if err := conformance.Validate(manifest); err == nil {
				t.Fatal("non-exact capability declaration was accepted")
			}
		})
	}

	for _, level := range []conformance.Level{conformance.FullyControlled, conformance.ContainedObserveOnly, conformance.Rejected} {
		manifest := conformance.Wave1Manifest()
		manifest.Level = level
		if err := conformance.Validate(manifest); err == nil {
			t.Fatalf("Wave1 accepted undeclared conformance level %s", level)
		}
	}
}

func containsConformanceValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
