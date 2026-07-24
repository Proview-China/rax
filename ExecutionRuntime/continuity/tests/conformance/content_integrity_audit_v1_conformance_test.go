package conformance_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func TestContentIntegrityAuditConformanceIsDiagnosticOnly(t *testing.T) {
	manifest := conformance.Wave1Manifest()
	found := false
	for _, capability := range manifest.Supported {
		if capability == conformance.CapabilityContentIntegrityV1 {
			found = true
		}
	}
	if !found {
		t.Fatal("Content Integrity Audit diagnostic capability is missing")
	}
	port := reflect.TypeOf((*ports.ContentIntegrityAuditGovernancePortV1)(nil)).Elem()
	for i := 0; i < port.NumMethod(); i++ {
		name := strings.ToLower(port.Method(i).Name)
		for _, forbidden := range []string{"delete", "purge", "cleanup", "retention", "provider", "archive", "reclaim"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("diagnostic Port exposes governed effect method %s", port.Method(i).Name)
			}
		}
	}
	for _, forbidden := range []string{"continuity/physical-purge", "continuity/remote-archive", "continuity/remote-blob"} {
		if !containsCapabilityV1(manifest.Unsupported, forbidden) {
			t.Fatalf("diagnostic capability silently unlocked %s", forbidden)
		}
	}
	request := reflect.TypeOf(ports.CreateContentIntegrityAuditRequestV1{})
	for i := 0; i < request.NumField(); i++ {
		name := strings.ToLower(request.Field(i).Name)
		for _, forbidden := range []string{"healthy", "classification", "visibility", "journalstate", "chunkstate", "cleanup", "purge", "provider"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("caller request exposes trusted result field %s", request.Field(i).Name)
			}
		}
	}
}
