package conformance_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/runtimeadapter"
)

func TestCheckpointManifestV2CapabilityDoesNotExpandCheckpointOrRestoreExecution(t *testing.T) {
	manifest := conformance.Wave1Manifest()
	if !containsCapability(manifest.Supported, conformance.CapabilityCheckpointManifestV2) {
		t.Fatal("CheckpointManifestGovernancePortV2 capability is not declared")
	}
	for _, forbidden := range []string{"continuity/checkpoint-capture", "continuity/restore-execute", "continuity/remote-blob"} {
		if !containsCapability(manifest.Unsupported, forbidden) || containsCapability(manifest.Supported, forbidden) {
			t.Fatalf("forbidden capability drifted: %s", forbidden)
		}
	}
	if err := conformance.Validate(manifest); err != nil {
		t.Fatalf("manifest invalid: %v", err)
	}

	portType := reflect.TypeOf((*ports.CheckpointManifestGovernancePortV2)(nil)).Elem()
	for index := 0; index < portType.NumMethod(); index++ {
		method := portType.Method(index)
		for _, forbidden := range []string{"restore", "activate", "execute", "capture", "consistency"} {
			if strings.Contains(strings.ToLower(method.Name), forbidden) {
				t.Fatalf("public Continuity V2 port contains forbidden method %s", method.Name)
			}
		}
	}
	sealType := reflect.TypeOf(contract.CheckpointManifestSealFactV2{})
	for _, forbidden := range []string{"Consistent", "RestoreEligible", "RuntimeOutcome", "ExpiresUnixNano"} {
		if _, exists := sealType.FieldByName(forbidden); exists {
			t.Fatalf("immutable seal contains foreign/current semantic %s", forbidden)
		}
	}
}

func TestCheckpointManifestRuntimeAdapterV2IsReadOnlyAndOneWay(t *testing.T) {
	adapterType := reflect.TypeOf(runtimeadapter.CheckpointManifestSealReaderV2{})
	if adapterType.NumField() != 1 || adapterType.Field(0).Name != "Manifests" || adapterType.Field(0).Type != reflect.TypeOf((*ports.CheckpointManifestReaderV2)(nil)).Elem() {
		t.Fatalf("Runtime adapter acquired a non-Reader dependency: %v", adapterType)
	}
	readerType := reflect.TypeOf((*runtimeadapter.CheckpointManifestSealReaderV2)(nil))
	for index := 0; index < readerType.NumMethod(); index++ {
		name := strings.ToLower(readerType.Method(index).Name)
		for _, forbidden := range []string{"create", "compare", "capture", "restore", "execute", "activate", "provider"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("Runtime adapter exposes forbidden method %s", readerType.Method(index).Name)
			}
		}
	}
}
