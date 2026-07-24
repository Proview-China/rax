package conformance_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func TestContentDeltaConformanceIsReferenceOnly(t *testing.T) {
	manifest := conformance.Wave1Manifest()
	if !containsCapabilityV1(manifest.Supported, conformance.CapabilityContentDeltaV1) {
		t.Fatal("Content Delta reference capability is missing")
	}
	port := reflect.TypeOf((*ports.ContentDeltaGovernancePortV1)(nil)).Elem()
	for i := 0; i < port.NumMethod(); i++ {
		name := strings.ToLower(port.Method(i).Name)
		for _, forbidden := range []string{"execute", "apply", "compact", "delete", "purge", "reclaim", "provider"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("Content Delta Port exposes effect method %s", port.Method(i).Name)
			}
		}
	}
	request := reflect.TypeOf(ports.CreateContentDeltaRequestV1{})
	for i := 0; i < request.NumField(); i++ {
		name := strings.ToLower(request.Field(i).Name)
		for _, forbidden := range []string{"chunk", "reuse", "added", "removed", "payload", "bytes", "fact"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("caller request exposes derived field %s", request.Field(i).Name)
			}
		}
	}
	for _, forbidden := range []string{"continuity/physical-purge", "continuity/remote-archive", "continuity/remote-blob"} {
		if !containsCapabilityV1(manifest.Unsupported, forbidden) {
			t.Fatalf("Content Delta silently unlocked %s", forbidden)
		}
	}
}
