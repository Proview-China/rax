package conformance_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func TestHistoryDerivationCandidateConformanceIsCandidateOnly(t *testing.T) {
	manifest := conformance.Wave1Manifest()
	if !containsCapabilityV1(manifest.Supported, conformance.CapabilityHistoryDerivationV1) {
		t.Fatal("History Derivation reference capability is missing")
	}
	port := reflect.TypeOf((*ports.HistoryDerivationCandidateGovernancePortV1)(nil)).Elem()
	for i := 0; i < port.NumMethod(); i++ {
		name := strings.ToLower(port.Method(i).Name)
		for _, forbidden := range []string{"publish", "current", "execute", "compact", "replace", "mutate", "delete", "purge", "provider"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("Port exposes effect method %s", port.Method(i).Name)
			}
		}
	}
	request := reflect.TypeOf(ports.CreateHistoryDerivationCandidateRequestV1{})
	for i := 0; i < request.NumField(); i++ {
		name := strings.ToLower(request.Field(i).Name)
		for _, forbidden := range []string{"payload", "authority", "current", "correct", "summarybytes", "delete", "purge"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("request exposes derived field %s", request.Field(i).Name)
			}
		}
	}
	for _, forbidden := range []string{"continuity/history-derivation-execute", "continuity/physical-purge", "continuity/remote-archive", "continuity/remote-blob"} {
		if !containsCapabilityV1(manifest.Unsupported, forbidden) {
			t.Fatalf("History Derivation silently unlocked %s", forbidden)
		}
	}
}
